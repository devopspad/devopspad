package workspace

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

func TestCapabilityCatalogIsCompleteAndGatesUnsupportedOperations(t *testing.T) {
	catalog := CapabilityCatalog()
	if len(catalog) != 7 {
		t.Fatal("expected seven engines")
	}
	for _, kind := range []string{"mysql", "redis", "mongo", "clickhouse", "kafka", "k8s", "k3s"} {
		c, ok := catalog[kind]
		if !ok || c.Language == "" || len(c.Features) == 0 || len(c.Limitations) == 0 {
			t.Fatalf("incomplete capability description for %s", kind)
		}
	}
	if _, e := ListDatabases(context.Background(), Connection{Kind: "redis"}); e == nil {
		t.Fatal("unsupported database discovery must fail")
	}
	if _, e := InspectResource(context.Background(), Connection{Kind: "kafka"}, "", "topic"); e == nil {
		t.Fatal("unsupported inspector must fail")
	}
}
func TestRedisScanCursorFilterAndMissingKey(t *testing.T) {
	server := miniredis.RunT(t)
	port, _ := strconv.Atoi(server.Port())
	c := Connection{Kind: "redis", Host: server.Host(), Port: port}
	server.Set("session:one", "one")
	server.Set("cache:one", "two")
	p, e := BrowsePage(context.Background(), c, BrowseRequest{Filter: "session:*"})
	if e != nil || len(p.Resources) != 1 || p.Resources[0].Name != "session:one" {
		t.Fatalf("filtered scan: %+v %v", p, e)
	}
	for _, cursor := range []string{"-1", "x", "1.2", "18446744073709551616"} {
		if _, e := BrowsePage(context.Background(), c, BrowseRequest{Cursor: cursor}); e == nil {
			t.Fatalf("accepted invalid cursor %s", cursor)
		}
	}
	if _, e := InspectResource(context.Background(), c, "", "missing"); e == nil || !strings.Contains(e.Error(), "expired") {
		t.Fatalf("missing key: %v", e)
	}
}
func TestIdentifierQuoting(t *testing.T) {
	if got := quoteIdentifier("mysql", "sales`archive"); got != "`sales``archive`" {
		t.Fatal(got)
	}
	if got := quoteIdentifier("clickhouse", "sales\\`archive"); got != "`sales\\\\\\`archive`" {
		t.Fatal(got)
	}
	if got := quoteClickHouseString("user's\\table"); got != "'user\\'s\\\\table'" {
		t.Fatal(got)
	}
	if _, e := InspectResource(context.Background(), Connection{Kind: "mysql"}, "db", "a\x00b"); e == nil {
		t.Fatal("accepted NUL in table name")
	}
}

func TestDDLExtractionUsesNamedColumn(t *testing.T) {
	view := Result{Columns: []string{"View", "Create View", "character_set_client", "collation_connection"}, Rows: [][]any{{"v", "CREATE VIEW v AS SELECT 1", "utf8mb4", "utf8mb4_0900_ai_ci"}}}
	if definitionText(view) != "CREATE VIEW v AS SELECT 1" {
		t.Fatal("view charset metadata was mistaken for DDL")
	}
	if definitionText(Result{Columns: []string{"statement"}, Rows: [][]any{{"CREATE TABLE t (id Int64)"}}}) != "CREATE TABLE t (id Int64)" {
		t.Fatal("ClickHouse definition missing")
	}
}
