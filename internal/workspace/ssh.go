package workspace

import (
	"context"
	"crypto/subtle"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// SSHConfig contains metadata only after saving. Password and passphrase are
// resolved from separate OS keychain entries immediately before an operation.
type SSHConfig struct {
	Enabled            bool   `json:"enabled"`
	Host               string `json:"host"`
	Port               int    `json:"port"`
	Username           string `json:"username"`
	AuthMethod         string `json:"authMethod"`
	Password           string `json:"password,omitempty"`
	PrivateKeyPath     string `json:"privateKeyPath"`
	Passphrase         string `json:"passphrase,omitempty"`
	KnownHostsPath     string `json:"knownHostsPath"`
	HostKeyFingerprint string `json:"hostKeyFingerprint"`
	HasPassword        bool   `json:"hasPassword"`
	HasPassphrase      bool   `json:"hasPassphrase"`
}

func validateSSH(s SSHConfig) error {
	if !s.Enabled {
		return nil
	}
	if strings.TrimSpace(s.Host) == "" || strings.ContainsAny(s.Host, "/ @?#\t\r\n") {
		return errors.New("SSH host must be a hostname or IP address")
	}
	if s.Port < 1 || s.Port > 65535 {
		return errors.New("SSH port must be between 1 and 65535")
	}
	if strings.TrimSpace(s.Username) == "" {
		return errors.New("SSH username is required")
	}
	switch s.AuthMethod {
	case "password", "agent":
	case "key":
		if strings.TrimSpace(s.PrivateKeyPath) == "" {
			return errors.New("SSH private key file is required")
		}
	default:
		return errors.New("Choose an SSH authentication method")
	}
	if s.HostKeyFingerprint != "" && (!strings.HasPrefix(s.HostKeyFingerprint, "SHA256:") || len(s.HostKeyFingerprint) != 50) {
		return errors.New("SSH host fingerprint must use SHA256 format")
	}
	return nil
}
func sshFilePath(path, fallback string) (string, error) {
	if path == "" {
		path = fallback
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path, nil
}
func hostKeyCallback(s SSHConfig) (ssh.HostKeyCallback, error) {
	if s.HostKeyFingerprint != "" {
		return func(_ string, _ net.Addr, key ssh.PublicKey) error {
			if subtle.ConstantTimeCompare([]byte(s.HostKeyFingerprint), []byte(ssh.FingerprintSHA256(key))) != 1 {
				return errors.New("SSH host key does not match the configured fingerprint")
			}
			return nil
		}, nil
	}
	path, err := sshFilePath(s.KnownHostsPath, "~/.ssh/known_hosts")
	if err != nil {
		return nil, err
	}
	callback, err := knownhosts.New(path)
	if err != nil {
		return nil, errors.New("Cannot read SSH known_hosts; choose a file or enter a verified SHA256 host fingerprint")
	}
	return func(host string, remote net.Addr, key ssh.PublicKey) error {
		if err := callback(host, remote, key); err != nil {
			return fmt.Errorf("SSH host key verification failed; verify the host in known_hosts or configure its trusted SHA256 fingerprint: %w", err)
		}
		return nil
	}, nil
}

type sshTunnel struct {
	client *ssh.Client
	ctx    context.Context
}

// prepareTransport opens one authenticated session per operation. Nested reads
// reuse it. Both explicit completion and cancellation close all its channels.
func prepareTransport(ctx context.Context, c Connection) (Connection, func(), error) {
	noop := func() {}
	if !c.SSH.Enabled || c.tunnel != nil {
		return c, noop, nil
	}
	if err := validateSSH(c.SSH); err != nil {
		return c, noop, err
	}
	callback, err := hostKeyCallback(c.SSH)
	if err != nil {
		return c, noop, err
	}
	var auth ssh.AuthMethod
	switch c.SSH.AuthMethod {
	case "password":
		if c.SSH.Password == "" {
			return c, noop, errors.New("SSH password is required")
		}
		auth = ssh.Password(c.SSH.Password)
	case "key":
		path, e := sshFilePath(c.SSH.PrivateKeyPath, "")
		if e != nil {
			return c, noop, e
		}
		data, e := os.ReadFile(path)
		if e != nil {
			return c, noop, errors.New("Cannot read SSH private key file")
		}
		var signer ssh.Signer
		if c.SSH.Passphrase != "" {
			signer, e = ssh.ParsePrivateKeyWithPassphrase(data, []byte(c.SSH.Passphrase))
		} else {
			signer, e = ssh.ParsePrivateKey(data)
		}
		if e != nil {
			return c, noop, errors.New("Cannot unlock SSH private key; check the key format and passphrase")
		}
		auth = ssh.PublicKeys(signer)
	case "agent":
		socket := os.Getenv("SSH_AUTH_SOCK")
		if socket == "" {
			return c, noop, errors.New("SSH Agent is unavailable; choose a private key or password")
		}
		conn, e := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", socket)
		if e != nil {
			return c, noop, errors.New("Cannot connect to SSH Agent")
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		stop := context.AfterFunc(ctx, func() { conn.Close() })
		defer stop()
		auth = ssh.PublicKeysCallback(agent.NewClient(conn).Signers)
	}
	addr := net.JoinHostPort(c.SSH.Host, fmt.Sprint(c.SSH.Port))
	raw, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return c, noop, fmt.Errorf("Cannot reach SSH host: %w", err)
	}
	stop := context.AfterFunc(ctx, func() { raw.Close() })
	raw.SetDeadline(time.Now().Add(10 * time.Second))
	sc, chans, reqs, err := ssh.NewClientConn(raw, addr, &ssh.ClientConfig{User: c.SSH.Username, Auth: []ssh.AuthMethod{auth}, HostKeyCallback: callback})
	if err != nil {
		stop()
		raw.Close()
		return c, noop, fmt.Errorf("SSH connection failed: %w", err)
	}
	raw.SetDeadline(time.Time{})
	sessionCtx, endSession := context.WithCancel(ctx)
	tunnel := &sshTunnel{client: ssh.NewClient(sc, chans, reqs), ctx: sessionCtx}
	c.tunnel = tunnel
	return c, func() { endSession(); stop(); tunnel.client.Close() }, nil
}

// The in-process pipe adds real read/write deadlines to SSH channels (which do
// not implement deadlines). DNS resolution happens at the SSH server. No local
// listening socket is exposed, and no direct-network fallback is possible.
func (t *sshTunnel) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, errors.New("SSH tunnels only support TCP")
	}
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	remote, err := t.client.DialContext(dialCtx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("SSH forwarding to %s failed: %w", addr, err)
	}
	local, bridge := net.Pipe()
	stop := context.AfterFunc(t.ctx, func() { bridge.Close(); remote.Close() })
	copyStream := func(dst io.Writer, src io.Reader) { io.Copy(dst, src); stop(); bridge.Close(); remote.Close() }
	go copyStream(remote, bridge)
	go copyStream(bridge, remote)
	return local, nil
}

type sshContextKey struct{}
type sshSQLConnector struct {
	driver.Connector
	tunnel *sshTunnel
}

func (c sshSQLConnector) Connect(ctx context.Context) (driver.Conn, error) {
	return c.Connector.Connect(context.WithValue(ctx, sshContextKey{}, c.tunnel))
}
func init() {
	// One permanent registration, with the session carried in the connection
	// context. No global per-profile registrations or credential-bearing DSNs.
	mysql.RegisterDialContext("infradex-ssh", func(ctx context.Context, addr string) (net.Conn, error) {
		tunnel, ok := ctx.Value(sshContextKey{}).(*sshTunnel)
		if !ok {
			return nil, errors.New("SSH session is unavailable")
		}
		return tunnel.DialContext(ctx, "tcp", addr)
	})
}
