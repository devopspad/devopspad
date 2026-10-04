package workspace

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

const inspectorPod = `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"api","namespace":"team","uid":"pod-uid","labels":{"app":"api"},"managedFields":[{"manager":"test"}],"creationTimestamp":"2026-10-01T00:00:00Z"},"spec":{"nodeName":"node-1","containers":[{"name":"app","image":"example/api:1"}],"initContainers":[{"name":"init","image":"example/init:1"}],"ephemeralContainers":[{"name":"debug","image":"example/debug:1"}]},"status":{"phase":"Running","containerStatuses":[{"name":"app","ready":true,"restartCount":2,"state":{"running":{}}}]}}`

func inspectorFixture(t *testing.T, handler http.HandlerFunc) (Connection, ClusterRef) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("unexpected mutation: %s", r.Method)
		}
		if r.URL.Path == "/api/v1/namespaces/team/pods/api" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, inspectorPod)
			return
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	path := filepath.Join(t.TempDir(), "config")
	cfg := fmt.Sprintf("apiVersion: v1\nkind: Config\ncurrent-context: test\nclusters:\n- name: test\n  cluster:\n    server: %s\ncontexts:\n- name: test\n  context:\n    cluster: test\n", server.URL)
	if e := os.WriteFile(path, []byte(cfg), 0600); e != nil {
		t.Fatal(e)
	}
	return Connection{Kind: "k8s", Kubeconfig: path, Context: "test"}, ClusterRef{Context: "test", Namespace: "team", Resource: "pods", Name: "api", UID: "pod-uid"}
}
func TestClusterInspectorAndUIDScopedEvents(t *testing.T) {
	c, ref := inspectorFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/namespaces/team/events" || r.URL.Query().Get("fieldSelector") != "involvedObject.uid=pod-uid" || r.URL.Query().Get("limit") != "200" {
			t.Errorf("event scope: %s", r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"apiVersion":"v1","kind":"EventList","metadata":{"continue":"more"},"items":[{"lastTimestamp":"2026-09-30T00:00:00Z","type":"Normal","reason":"Scheduled","message":"Scheduled","count":1},{"lastTimestamp":"2026-10-01T00:00:00Z","type":"Warning","reason":"BackOff","message":"Restarting","count":2}]}`)
	})
	details, e := InspectClusterResource(context.Background(), c, ref)
	if e != nil || len(details.Containers) != 3 || details.Containers[0].Restarts != 2 || !details.Containers[0].Ready || details.Containers[2].Kind != "Ephemeral" {
		t.Fatalf("details %+v %v", details, e)
	}
	if strings.Contains(details.Definition, "managedFields") || details.Labels["app"] != "api" || details.Ref.UID != "pod-uid" {
		t.Fatal(details)
	}
	events, e := ClusterEvents(context.Background(), c, details.Ref)
	if e != nil || len(events.Rows) != 2 || events.Rows[0][2] != "BackOff" || !events.Truncated {
		t.Fatalf("events %+v %v", events, e)
	}
	ref.UID = "previous-pod"
	if _, e = InspectClusterResource(context.Background(), c, ref); e == nil || !strings.Contains(e.Error(), "replaced") {
		t.Fatal("replacement not detected", e)
	}
}
func TestPodLogOptionsAndByteBound(t *testing.T) {
	var calls atomic.Int32
	c, ref := inspectorFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		q := r.URL.Query()
		if r.URL.Path != "/api/v1/namespaces/team/pods/api/log" || q.Get("container") != "init" || q.Get("tailLines") != "500" || q.Get("previous") != "true" || q.Get("timestamps") != "true" || q.Get("follow") == "true" || q.Get("limitBytes") != fmt.Sprint(LogByteLimit+1) {
			t.Errorf("log options: %s", r.URL)
		}
		fmt.Fprint(w, strings.Repeat("x", LogByteLimit+100))
	})
	request := PodLogRequest{Ref: ref, Container: "init", TailLines: 500, Previous: true}
	logs, e := ReadPodLogs(context.Background(), c, request)
	if e != nil || !logs.Truncated || len(logs.Text) != LogByteLimit {
		t.Fatalf("log bounds %d %v %v", len(logs.Text), logs.Truncated, e)
	}
	request.Ref.UID = "old"
	if _, e = ReadPodLogs(context.Background(), c, request); e == nil || calls.Load() != 1 {
		t.Fatal("read logs from replaced Pod", e)
	}
	request.Ref = ref
	request.Container = "unknown"
	if _, e = ReadPodLogs(context.Background(), c, request); e == nil || calls.Load() != 1 {
		t.Fatal("unknown container", e)
	}
	for _, limit := range []int64{-1, 2001} {
		request.TailLines = limit
		if _, e = ReadPodLogs(context.Background(), c, request); e == nil {
			t.Fatal("unbounded tail accepted")
		}
	}
}
func TestPodLogForbiddenAndCancellation(t *testing.T) {
	t.Run("permission", func(t *testing.T) {
		c, ref := inspectorFixture(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(403)
			fmt.Fprint(w, `{"apiVersion":"v1","kind":"Status","reason":"Forbidden","code":403,"message":"pods/log is forbidden"}`)
		})
		if _, e := ReadPodLogs(context.Background(), c, PodLogRequest{Ref: ref, Container: "app"}); e == nil || !strings.Contains(e.Error(), "get pods/log") {
			t.Fatal(e)
		}
		if _, e := InspectClusterResource(context.Background(), c, ref); e != nil {
			t.Fatal("log permissions blocked details", e)
		}
	})
	t.Run("cancel", func(t *testing.T) {
		c, ref := inspectorFixture(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		})
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		if _, e := ReadPodLogs(ctx, c, PodLogRequest{Ref: ref, Container: "app"}); e == nil {
			t.Fatal("expected cancellation")
		}
	})
}
func TestClusterRefValidation(t *testing.T) {
	c := Connection{Kind: "k8s"}
	for _, ref := range []ClusterRef{
		{Resource: "secrets", Name: "x", Namespace: "team"}, {Resource: "pods", Name: "x", Namespace: "*"},
		{Resource: "pods", Name: "../logs", Namespace: "team"}, {Resource: "nodes", Name: "node", Namespace: "team"},
	} {
		if _, _, e := clusterTarget(c, ref); e == nil {
			t.Fatalf("accepted invalid reference %+v", ref)
		}
	}
}

func TestPodLogsInvalidUTF8StaysWithinDisplayBudget(t *testing.T) {
	c, ref := inspectorFixture(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, strings.Repeat("a\xff", LogByteLimit/2)) })
	logs, e := ReadPodLogs(context.Background(), c, PodLogRequest{Ref: ref, Container: "app"})
	if e != nil || !utf8.ValidString(logs.Text) || len(logs.Text) > LogByteLimit || !logs.Truncated {
		t.Fatalf("invalid UTF8/budget: length=%d truncated=%v error=%v", len(logs.Text), logs.Truncated, e)
	}
}

func TestClusterEventPreviewBudgets(t *testing.T) {
	for _, mode := range []string{"count", "bytes"} {
		t.Run(mode, func(t *testing.T) {
			c, ref := inspectorFixture(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"apiVersion":"v1","kind":"EventList","items":[`)
				if mode == "count" {
					for i := 0; i < 201; i++ {
						if i > 0 {
							fmt.Fprint(w, ",")
						}
						fmt.Fprint(w, `{"reason":"Sample","message":"test"}`)
					}
				} else {
					fmt.Fprintf(w, `{"message":%q}`, strings.Repeat("x", (1<<20)+1))
				}
				fmt.Fprint(w, `]}`)
			})
			result, e := ClusterEvents(context.Background(), c, ref)
			if mode == "count" {
				if e != nil || !result.Truncated || len(result.Rows) != 200 {
					t.Fatalf("event count budget: %d %v %v", len(result.Rows), result.Truncated, e)
				}
			} else if e == nil || !strings.Contains(e.Error(), "preview budget") {
				t.Fatalf("large event must not appear as an empty list: %v", e)
			}
		})
	}
}
