package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestRedisDriverReadsAndRejectsWrites(t *testing.T) {
	server := miniredis.RunT(t)
	server.Set("hello world", "sample")
	port, _ := strconv.Atoi(server.Port())
	c := Connection{Kind: "redis", Host: server.Host(), Port: port, Database: "0"}
	r, e := Execute(context.Background(), c, `["GET","hello world"]`)
	if e != nil {
		t.Fatal(e)
	}
	if r.Rows[0][0] != "sample" {
		t.Fatal(r)
	}
	if _, e = Execute(context.Background(), c, "FLUSHALL"); e == nil {
		t.Fatal("accepted destructive Redis command")
	}
	if !server.Exists("hello world") {
		t.Fatal("data deleted")
	}
	resources, e := Browse(context.Background(), c)
	if e != nil || len(resources) != 1 {
		t.Fatalf("browse: %v %v", resources, e)
	}
	if _, e = Execute(context.Background(), c, resources[0].Query); e != nil {
		t.Fatalf("generated query cannot address a key with spaces: %v", e)
	}
}
func TestKubernetesDriverUsesNamespaceAndReadMethods(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != "GET" || r.URL.Path != "/api/v1/namespaces/team/pods" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"apiVersion":"v1","kind":"PodList","metadata":{},"items":[{"apiVersion":"v1","kind":"Pod","metadata":{"name":"api-123","namespace":"team","creationTimestamp":"2026-01-01T00:00:00Z"}}]}`)
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "config")
	config := fmt.Sprintf("apiVersion: v1\nkind: Config\nclusters:\n- name: test\n  cluster:\n    server: %s\ncontexts:\n- name: test\n  context:\n    cluster: test\n    user: test\ncurrent-context: test\nusers:\n- name: test\n  user: {}\n", server.URL)
	os.WriteFile(path, []byte(config), 0600)
	for _, kind := range []string{"k8s", "k3s"} {
		r, e := Execute(context.Background(), Connection{Kind: kind, Kubeconfig: path, Database: "team"}, "get pods")
		if e != nil {
			t.Fatal(e)
		}
		if !called || r.Rows[0][0] != "api-123" {
			t.Fatal(r)
		}
	}
	if _, e := Execute(context.Background(), Connection{Kind: "k8s", Kubeconfig: path}, "delete pods api-123"); e == nil {
		t.Fatal("accepted a write command")
	}
}
func TestAIRequestSendsOnlyExplicitContext(t *testing.T) {
	s, _ := testStore(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected provider request")
		}
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if len(body.Messages) != 2 || !strings.Contains(body.Messages[0].Content, "orders") || body.Messages[1].Content != "Count orders" {
			t.Errorf("unexpected context: %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"SELECT COUNT(*) FROM orders;"}}]}`)
	}))
	defer server.Close()
	if e := s.SaveAI(AISettings{Endpoint: server.URL + "/v1", Model: "test-model", APIKey: "test-key"}); e != nil {
		t.Fatal(e)
	}
	reply, e := s.Ask(context.Background(), AIRequest{Prompt: "Count orders", Kind: "mysql", Schema: []string{"orders"}})
	if e != nil || reply != "SELECT COUNT(*) FROM orders;" {
		t.Fatalf("%q %v", reply, e)
	}
}
func TestAIValidationAndCancellation(t *testing.T) {
	for _, endpoint := range []string{"http://remote.example/v1", "https://user:pass@example.com/v1", "https://example.com/v1?api_key=x"} {
		if validateAI(AISettings{Endpoint: endpoint, Model: "test"}) == nil {
			t.Fatalf("accepted %s", endpoint)
		}
	}
	s, _ := testStore(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer server.Close()
	s.SaveAI(AISettings{Endpoint: server.URL, Model: "test"})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, e := s.Ask(ctx, AIRequest{Prompt: "test"}); e == nil {
		t.Fatal("expected cancellation")
	}
}

func TestClickHouseAdapterUsesReadOnlySettingsAndPreservesLargeNumbers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Query().Get("readonly") != "1" || r.URL.Query().Get("max_result_rows") != "500" {
			t.Errorf("missing read-only limits: %s", r.URL.String())
		}
		user, pass, _ := r.BasicAuth()
		if user != "analyst" || pass != "secret" {
			t.Error("missing basic auth")
		}
		fmt.Fprint(w, `{"meta":[{"name":"id","type":"UInt64"}],"data":[[9007199254740993]],"rows":1}`)
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	r, e := Execute(context.Background(), Connection{Kind: "clickhouse", Host: u.Hostname(), Port: port, Username: "analyst", Password: "secret"}, "SELECT id FROM events")
	if e != nil {
		t.Fatal(e)
	}
	if fmt.Sprint(r.Rows[0][0]) != "9007199254740993" {
		t.Fatal("lost numeric precision")
	}
}
