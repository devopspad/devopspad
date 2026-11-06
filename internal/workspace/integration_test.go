//go:build integration

package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	redis "github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const fixturePassword = "devopspad-fixture-only"

func integrationEnabled(t *testing.T, kind string) {
	t.Helper()
	if !strings.Contains(","+os.Getenv("DEVOPSPAD_TEST_ENGINES")+",", ","+kind+",") {
		t.Skip("Enable this fixture with DEVOPSPAD_TEST_ENGINES")
	}
}
func fixture(kind string, port int) Connection {
	return Connection{Name: "Integration " + kind, Kind: kind, Host: "127.0.0.1", Port: port, Username: "root", Password: fixturePassword, Database: "devopspad_test"}
}
func TestIntegrationMySQL(t *testing.T) {
	integrationEnabled(t, "mysql")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, e := sql.Open("mysql", "root:"+fixturePassword+"@tcp(127.0.0.1:49306)/?timeout=3s&multiStatements=true")
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	_, e = db.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS devopspad_test; CREATE DATABASE IF NOT EXISTS devopspad_other; CREATE TABLE IF NOT EXISTS devopspad_test.orders (id BIGINT PRIMARY KEY, email VARCHAR(100) NOT NULL, total DECIMAL(10,2), INDEX idx_email(email)); DELETE FROM devopspad_test.orders; INSERT INTO devopspad_test.orders VALUES(9007199254740993,'fixture@example.test',49.90); CREATE TABLE IF NOT EXISTS devopspad_other.orders (id INT PRIMARY KEY); DELETE FROM devopspad_other.orders; INSERT INTO devopspad_other.orders VALUES(2);")
	if e != nil {
		t.Fatal(e)
	}
	c := fixture("mysql", 49306)
	if e = Test(ctx, c); e != nil {
		t.Fatal(e)
	}
	names, e := ListDatabases(ctx, c)
	if e != nil || !contains(names, "devopspad_other") {
		t.Fatalf("databases: %v %v", names, e)
	}
	page, e := BrowsePage(ctx, c, BrowseRequest{Database: "devopspad_other"})
	if e != nil || len(page.Resources) != 1 {
		t.Fatalf("browse: %+v %v", page, e)
	}
	details, e := InspectResource(ctx, c, "devopspad_test", "orders")
	if e != nil {
		t.Fatal(e)
	}
	if len(details.Columns.Rows) != 3 || len(details.Indexes.Rows) != 2 || !strings.Contains(details.Definition, "CREATE TABLE") {
		t.Fatalf("schema: %+v", details)
	}
	r, e := Execute(ctx, c, "SELECT id,total FROM orders")
	if e != nil || fmt.Sprint(r.Rows[0][0]) != "9007199254740993" {
		t.Fatalf("read/precision: %+v %v", r, e)
	}
	other := c
	other.Database = "devopspad_other"
	r, e = Execute(ctx, other, "SELECT id FROM orders")
	if e != nil || fmt.Sprint(r.Rows[0][0]) != "2" {
		t.Fatalf("wrong database: %+v %v", r, e)
	}
	if _, e = Execute(ctx, c, "DELETE FROM orders"); e == nil {
		t.Fatal("read-only transaction accepted DELETE")
	}
	for _, count := range []int{RowLimit, RowLimit + 1} {
		query := fmt.Sprintf("WITH RECURSIVE numbers AS (SELECT 1 AS n UNION ALL SELECT n+1 FROM numbers WHERE n < %d) SELECT n FROM numbers", count)
		preview, err := Execute(ctx, c, query)
		if err != nil || len(preview.Rows) != RowLimit || preview.Truncated != (count > RowLimit) {
			t.Fatalf("row boundary %d: rows=%d truncated=%v error=%v", count, len(preview.Rows), preview.Truncated, err)
		}
	}
	preview, err := Execute(ctx, c, "SELECT REPEAT('<', 1500000) AS large_value")
	if err != nil || len(preview.Rows) != 0 || !preview.Truncated || !strings.Contains(preview.Message, "8 MiB") {
		t.Fatalf("encoded result budget: rows=%d truncated=%v error=%v", len(preview.Rows), preview.Truncated, err)
	}
	bad := c
	bad.Password = "wrong-fixture-password"
	if e = Test(ctx, bad); e == nil {
		t.Fatal("accepted invalid credentials")
	}
	short, stop := context.WithTimeout(ctx, 200*time.Millisecond)
	defer stop()
	start := time.Now()
	if _, e = Execute(short, c, "SELECT SLEEP(10)"); e == nil {
		t.Fatal("expected cancellation")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("query cancellation exceeded 3 seconds")
	}
	// A table name containing SQL punctuation is always quoted as an identifier.
	if _, e = db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS devopspad_test.`odd``name` (id INT)"); e != nil {
		t.Fatal(e)
	}
	if _, e = InspectResource(ctx, c, "devopspad_test", "odd`name"); e != nil {
		t.Fatalf("quoted table: %v", e)
	}
	if _, e = db.ExecContext(ctx, "CREATE OR REPLACE VIEW devopspad_test.order_emails AS SELECT id,email FROM devopspad_test.orders"); e != nil {
		t.Fatal(e)
	}
	view, e := InspectResource(ctx, c, "devopspad_test", "order_emails")
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(view.Definition, "VIEW") || len(view.Columns.Rows) != 2 || len(view.Indexes.Rows) != 0 {
		t.Fatalf("view inspection: %+v", view)
	}
	t.Log("MySQL authentication, scopes, columns, indexes, DDL, precision, read-only enforcement, cancellation and identifier quoting verified")
}
func contains(items []string, want string) bool {
	for _, s := range items {
		if s == want {
			return true
		}
	}
	return false
}
func TestIntegrationRedis(t *testing.T) {
	integrationEnabled(t, "redis")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cl := redis.NewClient(&redis.Options{Addr: "127.0.0.1:49379", Password: fixturePassword, DB: 14})
	defer cl.Close()
	defer cl.FlushDB(context.Background())
	if e := cl.FlushDB(ctx).Err(); e != nil {
		t.Fatal(e)
	}
	cl.Set(ctx, "fixture:string", "hello", time.Minute)
	cl.HSet(ctx, "fixture:hash", "name", "Ada", "plan", "pro")
	cl.RPush(ctx, "fixture:list", "a", "b")
	cl.SAdd(ctx, "fixture:set", "a", "b")
	cl.ZAdd(ctx, "fixture:zset", redis.Z{Score: 1, Member: "a"})
	cl.XAdd(ctx, &redis.XAddArgs{Stream: "fixture:stream", Values: map[string]any{"event": "created"}})
	for i := 0; i < 310; i++ {
		cl.Set(ctx, fmt.Sprintf("fixture:page:%03d", i), i, 0)
	}
	c := fixture("redis", 49379)
	c.Username = ""
	c.Database = "14"
	if e := Test(ctx, c); e != nil {
		t.Fatal(e)
	}
	cursor := ""
	seen := map[string]bool{}
	batches := 0
	for {
		p, e := BrowsePage(ctx, c, BrowseRequest{Cursor: cursor, Filter: "fixture:page:*"})
		if e != nil {
			t.Fatal(e)
		}
		for _, r := range p.Resources {
			seen[r.Name] = true
		}
		batches++
		if !p.HasMore {
			break
		}
		cursor = p.NextCursor
		if batches > 100 {
			t.Fatal("scan did not terminate")
		}
	}
	if len(seen) != 310 || batches < 2 {
		t.Fatalf("pagination: %d keys in %d batches", len(seen), batches)
	}
	for _, kind := range []string{"string", "hash", "list", "set", "zset", "stream"} {
		t.Run(kind, func(t *testing.T) {
			d, e := InspectResource(ctx, c, "14", "fixture:"+kind)
			if e != nil {
				t.Fatal(e)
			}
			if len(d.Preview.Rows) == 0 || d.Properties[0].Value != kind {
				t.Fatalf("preview: %+v", d)
			}
		})
	}
	if _, e := Execute(ctx, c, "FLUSHDB"); e == nil {
		t.Fatal("accepted mutation")
	}
	c.Username = "default"
	c.TLS = false
	bad := c
	bad.Password = "wrong"
	if e := Test(ctx, bad); e == nil {
		t.Fatal("accepted invalid credentials")
	}
	t.Log("Redis auth, cursor pagination, filtering and six key type previews verified")
}
func TestIntegrationClickHouse(t *testing.T) {
	integrationEnabled(t, "clickhouse")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, q := range []string{"CREATE DATABASE IF NOT EXISTS devopspad_test", "CREATE TABLE IF NOT EXISTS devopspad_test.orders (id UInt64, total Decimal(10,2), INDEX idx_total total TYPE minmax GRANULARITY 1) ENGINE = MergeTree ORDER BY id", "TRUNCATE TABLE devopspad_test.orders", "INSERT INTO devopspad_test.orders VALUES (9007199254740993,49.90)"} {
		req, _ := http.NewRequestWithContext(ctx, "POST", "http://127.0.0.1:48123/", strings.NewReader(q))
		req.SetBasicAuth("default", fixturePassword)
		resp, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("seed: %s", b)
		}
	}
	c := fixture("clickhouse", 48123)
	c.Username = "default"
	if e := Test(ctx, c); e != nil {
		t.Fatal(e)
	}
	if _, e := ListDatabases(ctx, c); e != nil {
		t.Fatal(e)
	}
	d, e := InspectResource(ctx, c, "devopspad_test", "orders")
	if e != nil {
		t.Fatal(e)
	}
	if len(d.Columns.Rows) != 2 || len(d.Indexes.Rows) != 1 || d.Definition == "" {
		t.Fatalf("schema: %+v", d)
	}
	r, e := Execute(ctx, c, "SELECT id FROM orders")
	if e != nil || fmt.Sprint(r.Rows[0][0]) != "9007199254740993" {
		t.Fatalf("query: %+v %v", r, e)
	}
	if _, e = Execute(ctx, c, "TRUNCATE TABLE orders"); e == nil {
		t.Fatal("accepted write")
	}
	t.Log("ClickHouse authentication, database navigation, columns, skipping index, DDL, precision and read-only enforcement verified")
}
func TestIntegrationMongo(t *testing.T) {
	integrationEnabled(t, "mongo")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c := fixture("mongo", 49017)
	cl, e := mongoClient(ctx, c)
	if e != nil {
		t.Fatal(e)
	}
	defer cl.Disconnect(context.Background())
	col := cl.Database(c.Database).Collection("orders")
	if _, e = col.DeleteMany(ctx, bson.M{}); e != nil {
		t.Fatal(e)
	}
	if _, e = col.InsertOne(ctx, bson.M{"id": int64(9007199254740993), "status": "completed"}); e != nil {
		t.Fatal(e)
	}
	if e = Test(ctx, c); e != nil {
		t.Fatal(e)
	}
	names, e := ListDatabases(ctx, c)
	if e != nil || !contains(names, c.Database) {
		t.Fatalf("db list: %v %v", names, e)
	}
	r, e := Execute(ctx, c, `{"collection":"orders","filter":{"status":"completed"},"limit":10}`)
	if e != nil || len(r.Rows) != 1 {
		t.Fatalf("find: %+v %v", r, e)
	}
	user := "reader_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	authDB := cl.Database(c.Database)
	if e = authDB.RunCommand(ctx, bson.D{{Key: "createUser", Value: user}, {Key: "pwd", Value: fixturePassword}, {Key: "roles", Value: bson.A{bson.M{"role": "read", "db": c.Database}}}}).Err(); e != nil {
		t.Fatal(e)
	}
	defer authDB.RunCommand(context.Background(), bson.D{{Key: "dropUser", Value: user}})
	reader := c
	reader.Username = user
	reader.AuthSource = c.Database
	if e = Test(ctx, reader); e != nil {
		t.Fatal("custom authSource", e)
	}
	wrong := reader
	wrong.AuthSource = "admin"
	if e = Test(ctx, wrong); e == nil {
		t.Fatal("wrong authentication database unexpectedly succeeded")
	}
	ids := []primitive.ObjectID{primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()}
	paged := authDB.Collection("paged")
	if _, e = paged.DeleteMany(ctx, bson.M{}); e != nil {
		t.Fatal(e)
	}
	for i, id := range ids {
		if _, e = paged.InsertOne(ctx, bson.M{"_id": id, "rank": i + 1, "category": "same", "secret": "exclude", "big": int64(9007199254740993)}); e != nil {
			t.Fatal(e)
		}
	}
	query := `{"collection":"paged","filter":{},"projection":{"secret":0},"sort":{"category":1,"rank":-1},"limit":2}`
	first, e := Execute(ctx, reader, query)
	if e != nil || len(first.Rows) != 2 || first.NextQuery == "" || !first.Truncated {
		t.Fatalf("first batch: %+v %v", first, e)
	}
	doc := first.Rows[0][0].(map[string]any)
	if doc["secret"] != nil || doc["rank"].(map[string]any)["$numberInt"] != "3" || doc["big"].(map[string]any)["$numberLong"] != "9007199254740993" {
		t.Fatal("projection, sort or precision lost", doc)
	}
	second, e := Execute(ctx, reader, first.NextQuery)
	if e != nil || len(second.Rows) != 1 || second.Truncated || second.NextQuery != "" || second.PreviousQuery == "" || second.Offset != 2 {
		t.Fatalf("last batch: %+v %v", second, e)
	}
	previous, e := Execute(ctx, reader, second.PreviousQuery)
	if e != nil || fmt.Sprint(previous.Rows) != fmt.Sprint(first.Rows) {
		t.Fatal("previous batch differs", e)
	}
	exact, e := Execute(ctx, reader, `{"collection":"paged","filter":{"rank":{"$gte":2}},"limit":2}`)
	if e != nil || len(exact.Rows) != 2 || exact.Truncated || exact.NextQuery != "" {
		t.Fatalf("exact limit false positive: %+v %v", exact, e)
	}
	typedQuery := fmt.Sprintf(`{"collection":"paged","filter":{"_id":{"$oid":"%s"},"big":{"$numberLong":"9007199254740993"}}}`, ids[0].Hex())
	typed, e := Execute(ctx, reader, typedQuery)
	if e != nil || len(typed.Rows) != 1 {
		t.Fatalf("Extended JSON: %+v %v", typed, e)
	}
	t.Log("MongoDB authSource, ordered sort, projection, Extended JSON and next/previous batches verified")
}
func TestIntegrationKafka(t *testing.T) {
	integrationEnabled(t, "kafka")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c := fixture("kafka", 49092)
	c.Username = ""
	c.Password = ""
	cl := kafkaClient(c)
	defer cl.Transport.(*kafka.Transport).CloseIdleConnections()
	r, e := cl.CreateTopics(ctx, &kafka.CreateTopicsRequest{Topics: []kafka.TopicConfig{{Topic: "devopspad-fixture", NumPartitions: 2, ReplicationFactor: 1}}})
	if e != nil {
		t.Fatal(e)
	}
	_ = r
	if e = Test(ctx, c); e != nil {
		t.Fatal(e)
	}
	result, e := Execute(ctx, c, "devopspad-fixture")
	if e != nil || len(result.Rows) != 2 {
		t.Fatalf("metadata: %+v %v", result, e)
	}
	t.Log("Kafka topic discovery and partition metadata verified")
}
func TestIntegrationClusters(t *testing.T) {
	path := os.Getenv("DEVOPSPAD_TEST_KUBECONFIG")
	if path == "" {
		t.Skip("Set DEVOPSPAD_TEST_KUBECONFIG to the isolated fixture kubeconfig")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for _, kind := range []string{"k8s", "k3s"} {
		t.Run(kind, func(t *testing.T) {
			c := Connection{Name: "Fixture cluster", Kind: kind, Kubeconfig: path, Database: "default"}
			if e := Test(ctx, c); e != nil {
				t.Fatal(e)
			}
			r, e := Execute(ctx, c, "get namespaces")
			if e != nil || len(r.Rows) == 0 {
				t.Fatalf("namespaces: %+v %v", r, e)
			}
		})
	}
	cfg, e := clientcmd.LoadFromFile(path)
	if e != nil {
		t.Fatal(e)
	}
	base := cfg.Contexts[cfg.CurrentContext]
	cfg.Contexts["fixture-default"] = &clientcmdapi.Context{Cluster: base.Cluster, AuthInfo: base.AuthInfo, Namespace: "default"}
	cfg.Contexts["fixture-system"] = &clientcmdapi.Context{Cluster: base.Cluster, AuthInfo: base.AuthInfo, Namespace: "kube-system"}
	cfg.CurrentContext = "fixture-default"
	localPath := filepath.Join(t.TempDir(), "contexts")
	if e = clientcmd.WriteToFile(*cfg, localPath); e != nil {
		t.Fatal(e)
	}
	info, e := ReadKubeconfig(localPath)
	if e != nil || len(info.Contexts) < 2 {
		t.Fatalf("contexts %+v %v", info, e)
	}
	c := Connection{Name: "Fixture", Kind: "k8s", Kubeconfig: localPath, Context: "fixture-default", Database: "default"}
	names, e := ListDatabases(ctx, c)
	if e != nil || !contains(names, "kube-system") || !contains(names, "*") {
		t.Fatalf("namespace discovery %v %v", names, e)
	}
	cl, e := kubeClient(c)
	if e != nil {
		t.Fatal(e)
	}
	maps := schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
	for _, ns := range []string{"default", "kube-system"} {
		obj := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": "devopspad-" + ns, "namespace": ns}, "data": map[string]any{"fixture": "true"}}}
		if _, e = cl.Resource(maps).Namespace(ns).Create(ctx, obj, metav1.CreateOptions{}); e != nil {
			t.Fatal(e)
		}
		defer cl.Resource(maps).Namespace(ns).Delete(context.Background(), obj.GetName(), metav1.DeleteOptions{})
		scoped := c
		scoped.Context = "fixture-" + map[string]string{"default": "default", "kube-system": "system"}[ns]
		scoped.Database = ns
		r, e := Execute(ctx, scoped, "get configmaps")
		if e != nil {
			t.Fatal(e)
		}
		found := false
		for _, row := range r.Rows {
			if row[1] != ns {
				t.Fatalf("escaped namespace: %v", row)
			}
			if row[0] == obj.GetName() {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing namespace fixture %s", ns)
		}
	}
	pods := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	pod := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]any{"name": "devopspad-inspector", "namespace": "default", "labels": map[string]any{"app": "devopspad-fixture"}}, "spec": map[string]any{"automountServiceAccountToken": false, "containers": []any{map[string]any{"name": "app", "image": "registry.k8s.io/pause:3.10"}}}}}
	created, e := cl.Resource(pods).Namespace("default").Create(ctx, pod, metav1.CreateOptions{})
	if e != nil {
		t.Fatal(e)
	}
	defer cl.Resource(pods).Namespace("default").Delete(context.Background(), pod.GetName(), metav1.DeleteOptions{})
	ref := ClusterRef{Context: c.Context, Namespace: "default", Resource: "pods", Name: pod.GetName(), UID: string(created.GetUID())}
	detail, e := InspectClusterResource(ctx, c, ref)
	if e != nil || len(detail.Containers) != 1 || detail.Containers[0].Name != "app" || detail.Ref.UID != ref.UID {
		t.Fatalf("resource details %+v %v", detail, e)
	}
	eventResource := schema.GroupVersionResource{Version: "v1", Resource: "events"}
	for i, uid := range []string{ref.UID, "previous-instance"} {
		event := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Event", "metadata": map[string]any{"name": fmt.Sprintf("devopspad-event-%d", i), "namespace": "default"}, "involvedObject": map[string]any{"apiVersion": "v1", "kind": "Pod", "name": ref.Name, "namespace": "default", "uid": uid}, "reason": "DevOpsPadFixture", "message": "Inspector fixture", "type": "Normal", "count": int64(1)}}
		if _, e = cl.Resource(eventResource).Namespace("default").Create(ctx, event, metav1.CreateOptions{}); e != nil {
			t.Fatal(e)
		}
		defer cl.Resource(eventResource).Namespace("default").Delete(context.Background(), event.GetName(), metav1.DeleteOptions{})
	}
	events, e := ClusterEvents(ctx, c, ref)
	if e != nil || len(events.Rows) != 1 || events.Rows[0][2] != "DevOpsPadFixture" {
		t.Fatalf("UID event filter %+v %v", events, e)
	}
	// K3s can return HTTP 200 with an empty stream before a Pod is scheduled.
	pendingLogs, pendingError := ReadPodLogs(ctx, c, PodLogRequest{Ref: ref, Container: "app"})
	if pendingError == nil && (pendingLogs.Text != "" || pendingLogs.Truncated) {
		t.Fatal("unscheduled fixture Pod must not contain runtime log output")
	}
	// Restrict an impersonated fixture identity without touching any user cluster.
	roleResource := schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"}
	bindingResource := schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"}
	for _, fixture := range []struct {
		gvr schema.GroupVersionResource
		raw string
	}{
		{roleResource, `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"Role","metadata":{"name":"devopspad-reader","namespace":"default"},"rules":[{"apiGroups":[""],"resources":["pods","configmaps"],"verbs":["get","list"]}]}`},
		{bindingResource, `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"name":"devopspad-reader","namespace":"default"},"subjects":[{"kind":"User","name":"devopspad-fixture-reader","apiGroup":"rbac.authorization.k8s.io"}],"roleRef":{"kind":"Role","name":"devopspad-reader","apiGroup":"rbac.authorization.k8s.io"}}`},
	} {
		obj := &unstructured.Unstructured{}
		if e = json.Unmarshal([]byte(fixture.raw), &obj.Object); e != nil {
			t.Fatal(e)
		}
		if _, e = cl.Resource(fixture.gvr).Namespace("default").Create(ctx, obj, metav1.CreateOptions{}); e != nil {
			t.Fatal(e)
		}
		defer cl.Resource(fixture.gvr).Namespace("default").Delete(context.Background(), "devopspad-reader", metav1.DeleteOptions{})
	}
	readerAuth := cfg.AuthInfos[base.AuthInfo].DeepCopy()
	readerAuth.Impersonate = "devopspad-fixture-reader"
	readerAuth.ImpersonateGroups = nil
	cfg.AuthInfos["reader"] = readerAuth
	cfg.Contexts["reader"] = &clientcmdapi.Context{Cluster: base.Cluster, AuthInfo: "reader", Namespace: "default"}
	if e = clientcmd.WriteToFile(*cfg, localPath); e != nil {
		t.Fatal(e)
	}
	c.Context = "reader"
	if _, e = ListDatabases(ctx, c); e == nil || !strings.Contains(e.Error(), "not permitted") {
		t.Fatal("expected explained RBAC error", e)
	}
	if _, e = BrowsePage(ctx, c, BrowseRequest{Database: "default"}); e != nil {
		t.Fatal("permitted namespace must remain usable", e)
	}
	if _, e = Execute(ctx, c, "get configmaps"); e != nil {
		t.Fatal("permitted resource read", e)
	}
	if _, e = Execute(ctx, c, "get configmaps kube-system"); e == nil {
		t.Fatal("namespace RBAC bypassed")
	}
	ref.Context = "reader"
	if _, e = InspectClusterResource(ctx, c, ref); e != nil {
		t.Fatal("reader should inspect Pod", e)
	}
	if _, e = ClusterEvents(ctx, c, ref); e == nil || !strings.Contains(e.Error(), "Access denied") {
		t.Fatal("events RBAC was not explained", e)
	}
	if _, e = ReadPodLogs(ctx, c, PodLogRequest{Ref: ref, Container: "app"}); e == nil || !strings.Contains(e.Error(), "get pods/log") {
		t.Fatal("log subresource RBAC was not explained", e)
	}
	t.Log("Kubernetes/K3s contexts, namespace isolation, resource inspection, UID events and log RBAC verified; fixture has no running kubelet for positive log output")
}
func TestIntegrationKeychain(t *testing.T) {
	if os.Getenv("DEVOPSPAD_TEST_KEYCHAIN") != "1" {
		t.Skip("Set DEVOPSPAD_TEST_KEYCHAIN=1")
	}
	vault := Keychain{}
	key := "integration-" + uuid.NewString()
	value := uuid.NewString()
	if e := vault.Set(key, value); e != nil {
		t.Fatal(e)
	}
	defer vault.Delete(key)
	got, e := vault.Get(key)
	if e != nil || got != value {
		t.Fatal("keychain round trip failed")
	}
	if e = vault.Delete(key); e != nil {
		t.Fatal(e)
	}
	if _, e = vault.Get(key); e == nil {
		t.Fatal("keychain deletion failed")
	}
	t.Log("Real OS keychain create/read/delete verified using a disposable random entry")
}
