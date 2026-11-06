//go:build integration

package workspace

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"testing"
	"time"

	"k8s.io/client-go/tools/clientcmd"
)

func TestIntegrationSSHEngines(t *testing.T) {
	ports := map[string]int{"mysql": 49306, "redis": 49379, "mongo": 49017, "clickhouse": 48123, "kafka": 49092}
	for _, kind := range []string{"mysql", "redis", "mongo", "clickhouse", "kafka", "k8s", "k3s"} {
		t.Run(kind, func(t *testing.T) {
			cluster := kind == "k8s" || kind == "k3s"
			if !cluster {
				integrationEnabled(t, kind)
			} else if os.Getenv("DEVOPSPAD_TEST_KUBECONFIG") == "" {
				t.Skip("No isolated cluster fixture")
			}
			f := newSSHFixture(t, func(addr string) (string, error) {
				host, port, e := net.SplitHostPort(addr)
				if e != nil {
					return "", e
				}
				if host == "private.invalid" {
					host = "127.0.0.1"
				}
				return net.JoinHostPort(host, port), nil
			})
			c := fixture(kind, ports[kind])
			c.Host = "private.invalid"
			c.Database = ""
			c.SSH = f.config
			query := "SELECT 1"
			switch kind {
			case "mysql":
				c.Database = "mysql"
			case "redis":
				c.Username = ""
				query = "PING"
			case "mongo":
				c.Database = "devopspad_test"
				query = `{"collection":"orders","filter":{},"limit":1}`
			case "clickhouse":
				c.Username = "default"
			case "kafka":
				c.Username = ""
				c.Password = ""
				query = "topics"
			case "k8s", "k3s":
				cfg, e := clientcmd.LoadFromFile(os.Getenv("DEVOPSPAD_TEST_KUBECONFIG"))
				if e != nil {
					t.Fatal(e)
				}
				for _, cl := range cfg.Clusters {
					u, e := url.Parse(cl.Server)
					if e != nil {
						t.Fatal(e)
					}
					cl.TLSServerName = u.Hostname() // Retain the certificate identity across remote DNS aliasing.
					u.Host = net.JoinHostPort("private.invalid", u.Port())
					cl.Server = u.String()
				}
				c.Kubeconfig = t.TempDir() + "/config"
				if e := clientcmd.WriteToFile(*cfg, c.Kubeconfig); e != nil {
					t.Fatal(e)
				}
				c.Database = "default"
				query = "get pods"
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if e := Test(ctx, c); e != nil {
				t.Fatal("test", e)
			}
			if _, e := Execute(ctx, c, query); e != nil {
				t.Fatal("query", e)
			}
			if _, e := BrowsePage(ctx, c, BrowseRequest{Database: c.Database}); e != nil {
				t.Fatal("browse", e)
			}
			if CapabilityCatalog()[kind].CanSelectDatabase {
				if _, e := ListDatabases(ctx, c); e != nil {
					t.Fatal("scope discovery", e)
				}
			}
			if !cluster && !f.saw(net.JoinHostPort("private.invalid", fmt.Sprint(c.Port))) {
				t.Fatal("did not use remote DNS target")
			}
			if kind == "mysql" {
				short, stop := context.WithTimeout(ctx, 100*time.Millisecond)
				start := time.Now()
				_, err := Execute(short, c, "SELECT SLEEP(10)")
				stop()
				if err == nil || time.Since(start) > 2*time.Second {
					t.Fatal("SSH query cancellation failed", err)
				}
			}
			t.Log("Live engine test, query, browse and available scope discovery passed through authenticated SSH")
		})
	}
}
