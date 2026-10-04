package workspace

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

type sshFixture struct {
	config  SSHConfig
	private ed25519.PrivateKey
	signer  ssh.Signer
	mu      sync.Mutex
	targets []string
	active  atomic.Int32
}

// An actual SSH server, accepting only direct-tcpip requests. Mapping private
// names here proves that adapters send DNS names across SSH, not to local DNS.
func newSSHFixture(t *testing.T, route func(string) (string, error)) *sshFixture {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	f := &sshFixture{private: key, signer: signer}
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(meta ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if meta.User() == "fixture" && string(pass) == "ssh-secret" {
				return nil, nil
			}
			return nil, errors.New("denied")
		},
		PublicKeyCallback: func(meta ssh.ConnMetadata, pub ssh.PublicKey) (*ssh.Permissions, error) {
			if meta.User() == "fixture" && string(pub.Marshal()) == string(signer.PublicKey().Marshal()) {
				return nil, nil
			}
			return nil, errors.New("denied")
		},
	}
	cfg.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	n, _ := strconv.Atoi(port)
	f.config = SSHConfig{Enabled: true, Host: host, Port: n, Username: "fixture", AuthMethod: "password", Password: "ssh-secret", HostKeyFingerprint: ssh.FingerprintSHA256(signer.PublicKey())}
	var mu sync.Mutex
	conns := map[net.Conn]bool{}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			raw, e := listener.Accept()
			if e != nil {
				return
			}
			mu.Lock()
			conns[raw] = true
			mu.Unlock()
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer raw.Close()
				defer func() { mu.Lock(); delete(conns, raw); mu.Unlock() }()
				sc, channels, requests, e := ssh.NewServerConn(raw, cfg)
				if e != nil {
					return
				}
				defer sc.Close()
				f.active.Add(1)
				defer f.active.Add(-1)
				go ssh.DiscardRequests(requests)
				for channel := range channels {
					if channel.ChannelType() != "direct-tcpip" {
						channel.Reject(ssh.UnknownChannelType, "unsupported")
						continue
					}
					var target struct {
						Host       string
						Port       uint32
						Origin     string
						OriginPort uint32
					}
					if ssh.Unmarshal(channel.ExtraData(), &target) != nil {
						channel.Reject(ssh.ConnectionFailed, "bad request")
						continue
					}
					addr := net.JoinHostPort(target.Host, fmt.Sprint(target.Port))
					f.mu.Lock()
					f.targets = append(f.targets, addr)
					f.mu.Unlock()
					mapped, e := route(addr)
					if e != nil {
						channel.Reject(ssh.ConnectionFailed, "fixture denied target")
						continue
					}
					upstream, e := net.DialTimeout("tcp", mapped, time.Second)
					if e != nil {
						channel.Reject(ssh.ConnectionFailed, "unreachable")
						continue
					}
					ch, reqs, e := channel.Accept()
					if e != nil {
						upstream.Close()
						continue
					}
					go ssh.DiscardRequests(reqs)
					wg.Add(2)
					go func() { defer wg.Done(); defer ch.Close(); defer upstream.Close(); io.Copy(ch, upstream) }()
					go func() { defer wg.Done(); defer ch.Close(); defer upstream.Close(); io.Copy(upstream, ch) }()
				}
			}()
		}
	}()
	t.Cleanup(func() {
		listener.Close()
		mu.Lock()
		for c := range conns {
			c.Close()
		}
		mu.Unlock()
		wg.Wait()
	})
	return f
}
func (f *sshFixture) saw(addr string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.targets {
		if s == addr {
			return true
		}
	}
	return false
}
func (f *sshFixture) connection(kind string) Connection {
	return Connection{Name: "SSH fixture", Kind: kind, Host: "private.invalid", Port: 1234, SSH: f.config}
}
func TestSSHRedisOperationsAndDeadlines(t *testing.T) {
	server := miniredis.RunT(t)
	server.Set("key", "value")
	f := newSSHFixture(t, func(addr string) (string, error) {
		if addr != "private.invalid:1234" {
			return "", errors.New("wrong target")
		}
		return server.Addr(), nil
	})
	c := f.connection("redis")
	ctx := context.Background()
	if err := Test(ctx, c); err != nil {
		t.Fatal(err)
	}
	page, err := BrowsePage(ctx, c, BrowseRequest{})
	if err != nil || len(page.Resources) != 1 {
		t.Fatalf("browse %v %v", page, err)
	}
	detail, err := InspectResource(ctx, c, "", "key")
	if err != nil || len(detail.Preview.Rows) != 1 {
		t.Fatalf("inspect %v %v", detail, err)
	}
	result, err := Execute(ctx, c, "GET key")
	if err != nil || !strings.Contains(fmt.Sprint(result.Rows), "value") {
		t.Fatalf("query %v %v", result, err)
	}
	if !f.saw("private.invalid:1234") {
		t.Fatal("did not forward target")
	}
	live, closeTunnel, err := prepareTransport(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	defer closeTunnel()
	conn, err := live.tunnel.DialContext(ctx, "tcp", "private.invalid:1234")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err = conn.SetReadDeadline(time.Now().Add(30 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	_, err = conn.Read(make([]byte, 1))
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("deadline: %v", err)
	}
	closeTunnel()
	deadline := time.Now().Add(time.Second)
	for f.active.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if f.active.Load() != 0 {
		t.Fatal("SSH session leaked after operation")
	}
}
func TestSSHHostVerificationAndAuthentication(t *testing.T) {
	f := newSSHFixture(t, func(string) (string, error) { return "", errors.New("unused") })
	for _, method := range []string{"password", "key", "encrypted-key", "agent", "known-hosts"} {
		t.Run(method, func(t *testing.T) {
			c := f.connection("mysql")
			if method == "key" || method == "encrypted-key" {
				c.SSH.AuthMethod = "key"
				var block *pem.Block
				var err error
				if method == "encrypted-key" {
					c.SSH.Passphrase = "key-secret"
					block, err = ssh.MarshalPrivateKeyWithPassphrase(f.private, "fixture", []byte(c.SSH.Passphrase))
				} else {
					block, err = ssh.MarshalPrivateKey(f.private, "fixture")
				}
				if err != nil {
					t.Fatal(err)
				}
				c.SSH.PrivateKeyPath = filepath.Join(t.TempDir(), "id_ed25519")
				if err = os.WriteFile(c.SSH.PrivateKeyPath, pem.EncodeToMemory(block), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if method == "agent" {
				c.SSH.AuthMethod = "agent"
				dir, err := os.MkdirTemp("/tmp", "ix-agent-")
				if err != nil {
					t.Fatal(err)
				}
				defer os.RemoveAll(dir)
				socket := filepath.Join(dir, "agent.sock")
				listener, err := net.Listen("unix", socket)
				if err != nil {
					t.Fatal(err)
				}
				defer listener.Close()
				ring := agent.NewKeyring()
				if err = ring.Add(agent.AddedKey{PrivateKey: f.private}); err != nil {
					t.Fatal(err)
				}
				go func() {
					conn, e := listener.Accept()
					if e == nil {
						defer conn.Close()
						agent.ServeAgent(ring, conn)
					}
				}()
				t.Setenv("SSH_AUTH_SOCK", socket)
			}
			if method == "known-hosts" {
				c.SSH.HostKeyFingerprint = ""
				c.SSH.KnownHostsPath = filepath.Join(t.TempDir(), "known_hosts")
				line := knownhosts.Line([]string{net.JoinHostPort(c.SSH.Host, fmt.Sprint(c.SSH.Port))}, f.signer.PublicKey()) + "\n"
				if err := os.WriteFile(c.SSH.KnownHostsPath, []byte(line), 0600); err != nil {
					t.Fatal(err)
				}
			}
			_, closeTunnel, err := prepareTransport(context.Background(), c)
			if err != nil {
				t.Fatal(err)
			}
			closeTunnel()
			if method == "encrypted-key" {
				c.SSH.Passphrase = "incorrect"
				if _, _, err = prepareTransport(context.Background(), c); err == nil {
					t.Fatal("accepted wrong passphrase")
				}
			}
		})
	}
	for _, mode := range []string{"wrong-password", "changed-key", "unknown-key"} {
		t.Run(mode, func(t *testing.T) {
			c := f.connection("mysql")
			if mode == "wrong-password" {
				c.SSH.Password = "wrong"
			} else if mode == "changed-key" {
				c.SSH.HostKeyFingerprint = "SHA256:" + strings.Repeat("A", 43)
			} else {
				c.SSH.HostKeyFingerprint = ""
				c.SSH.KnownHostsPath = filepath.Join(t.TempDir(), "known_hosts")
				os.WriteFile(c.SSH.KnownHostsPath, nil, 0600)
			}
			if _, closeTunnel, err := prepareTransport(context.Background(), c); err == nil {
				closeTunnel()
				t.Fatal("accepted untrusted connection")
			}
		})
	}
}
func TestSSHCancelledHandshakeAndForwarding(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, e := listener.Accept()
		if e == nil {
			defer conn.Close()
			io.Copy(io.Discard, conn)
		}
	}()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	n, _ := strconv.Atoi(port)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	c := Connection{SSH: SSHConfig{Enabled: true, Host: host, Port: n, Username: "x", AuthMethod: "password", Password: "x", HostKeyFingerprint: "SHA256:" + strings.Repeat("A", 43)}}
	start := time.Now()
	if _, closeTunnel, e := prepareTransport(ctx, c); e == nil {
		closeTunnel()
		t.Fatal("accepted stalled server")
	}
	if time.Since(start) > time.Second {
		t.Fatal("handshake ignored cancellation")
	}
	f := newSSHFixture(t, func(string) (string, error) { return "", errors.New("denied") })
	c = f.connection("clickhouse")
	if _, e := Execute(context.Background(), c, "SELECT 1"); e == nil {
		t.Fatal("forward denial ignored")
	}
}
func TestSSHClickHouseAndKubernetes(t *testing.T) {
	t.Run("clickhouse", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Host != "private.invalid:1234" {
				t.Errorf("host rewritten %s", r.Host)
			}
			fmt.Fprint(w, `{"meta":[{"name":"n"}],"data":[[1]],"rows":1}`)
		}))
		defer server.Close()
		f := newSSHFixture(t, func(string) (string, error) { return strings.TrimPrefix(server.URL, "http://"), nil })
		c := f.connection("clickhouse")
		if err := Test(context.Background(), c); err != nil {
			t.Fatal(err)
		}
	})
	for _, kind := range []string{"k8s", "k3s"} {
		t.Run(kind, func(t *testing.T) {
			c, ref := inspectorFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/log") {
					fmt.Fprint(w, "log through SSH\n")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"apiVersion":"v1","kind":"EventList","items":[]}`)
			})
			config, err := os.ReadFile(c.Kubeconfig)
			if err != nil {
				t.Fatal(err)
			}
			var original string
			for _, line := range strings.Split(string(config), "\n") {
				if strings.Contains(line, "server: ") {
					original = strings.TrimSpace(strings.SplitN(line, "server: ", 2)[1])
				}
			}
			f := newSSHFixture(t, func(string) (string, error) { return strings.TrimPrefix(original, "http://"), nil })
			os.WriteFile(c.Kubeconfig, []byte(strings.ReplaceAll(string(config), original, "http://private.invalid:1234")), 0600)
			c.Kind = kind
			c.SSH = f.config
			if _, err = InspectClusterResource(context.Background(), c, ref); err != nil {
				t.Fatal(err)
			}
			if _, err = ClusterEvents(context.Background(), c, ref); err != nil {
				t.Fatal(err)
			}
			logs, err := ReadPodLogs(context.Background(), c, PodLogRequest{Ref: ref, Container: "app"})
			if err != nil || logs.Text != "log through SSH\n" {
				t.Fatalf("logs %v %v", logs, err)
			}
			if !f.saw("private.invalid:1234") {
				t.Fatal("API target did not use SSH")
			}
		})
	}
}
func TestSSHTLSStillRejectsUntrustedServer(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("untrusted TLS request reached handler") }))
	defer server.Close()
	f := newSSHFixture(t, func(string) (string, error) { return strings.TrimPrefix(server.URL, "https://"), nil })
	for _, kind := range []string{"redis", "clickhouse", "mongo", "kafka"} {
		t.Run(kind, func(t *testing.T) {
			c := f.connection(kind)
			c.TLS = true
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := Test(ctx, c); err == nil {
				t.Fatal("TLS verification bypassed")
			}
		})
	}
}

func TestSSHForwardDenialNeverFallsBackToDirect(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		fmt.Fprint(w, `{"meta":[],"data":[],"rows":0}`)
	}))
	defer server.Close()
	host, port, _ := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	n, _ := strconv.Atoi(port)
	f := newSSHFixture(t, func(string) (string, error) { return "", errors.New("forwarding denied") })
	c := f.connection("clickhouse")
	c.Host = host
	c.Port = n
	if err := Test(context.Background(), c); err == nil || requests.Load() != 0 {
		t.Fatal("fell back to direct access", err)
	}
}
func TestSSHCancellationClosesOpenChannel(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, e := listener.Accept()
		if e == nil {
			defer conn.Close()
			io.Copy(io.Discard, conn)
		}
	}()
	f := newSSHFixture(t, func(string) (string, error) { return listener.Addr().String(), nil })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, closeTunnel, err := prepareTransport(ctx, f.connection("mysql"))
	if err != nil {
		t.Fatal(err)
	}
	defer closeTunnel()
	conn, err := c.tunnel.DialContext(ctx, "tcp", "private.invalid:1234")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	read := make(chan error, 1)
	go func() { _, err := conn.Read(make([]byte, 1)); read <- err }()
	cancel()
	select {
	case err = <-read:
		if err == nil {
			t.Fatal("read succeeded after cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("SSH read stuck after cancellation")
	}
}
