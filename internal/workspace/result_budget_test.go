package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

type previewRows struct {
	values [][]any
	index  int
	err    error
}

func (r *previewRows) Columns() ([]string, error) { return []string{"value"}, nil }
func (r *previewRows) Next() bool                 { r.index++; return r.index <= len(r.values) }
func (r *previewRows) Scan(dest ...any) error {
	for i, value := range r.values[r.index-1] {
		*dest[i].(*any) = value
	}
	return nil
}
func (r *previewRows) Err() error { return r.err }

func TestSQLPreviewBoundsAndPrecision(t *testing.T) {
	for _, count := range []int{0, RowLimit - 1, RowLimit, RowLimit + 1} {
		rows := &previewRows{}
		for i := 0; i < count; i++ {
			rows.values = append(rows.values, []any{int64(9007199254740993)})
		}
		result, err := readSQLRows(rows)
		if err != nil || len(result.Rows) != min(count, RowLimit) || result.Truncated != (count > RowLimit) {
			t.Fatalf("count=%d rows=%d truncated=%v err=%v", count, len(result.Rows), result.Truncated, err)
		}
		if count > 0 && result.Rows[0][0] != "9007199254740993" {
			t.Fatal("lost integer precision")
		}
	}
	rows := &previewRows{values: [][]any{{"small"}, {strings.Repeat("<", 2<<20)}, {"must not be scanned"}}}
	result, err := readSQLRows(rows)
	if err != nil || len(result.Rows) != 1 || !result.Truncated || !strings.Contains(result.Message, "8 MiB") || rows.index != 2 {
		t.Fatalf("encoded budget not enforced: rows=%d index=%d error=%v", len(result.Rows), rows.index, err)
	}
	encoded, _ := json.Marshal(result)
	if len(encoded) > ResultByteLimit {
		t.Fatal("preview exceeds bridge budget")
	}
	result, err = readSQLRows(&previewRows{values: [][]any{{strings.Repeat("x", ResultByteLimit)}}})
	if err != nil || !result.Truncated || len(result.Rows) != 0 {
		t.Fatal("oversized first row not reported")
	}
	want := errors.New("connection interrupted")
	if _, err = readSQLRows(&previewRows{err: want}); !errors.Is(err, want) {
		t.Fatal("lost driver error")
	}
}

func TestRedisOversizedResponseCanBeRetriedWithRange(t *testing.T) {
	server := miniredis.RunT(t)
	server.Set("large", strings.Repeat("x", ResultByteLimit))
	port, _ := strconv.Atoi(server.Port())
	c := Connection{Kind: "redis", Host: server.Host(), Port: port}
	result, err := Execute(context.Background(), c, "GET large")
	if err == nil || !strings.Contains(err.Error(), "8 MiB") || len(result.Rows) != 0 {
		t.Fatal("oversized response crossed bridge")
	}
	result, err = Execute(context.Background(), c, "GETRANGE large 0 99")
	if err != nil || result.Rows[0][0] != strings.Repeat("x", 100) {
		t.Fatal("bounded retry failed", err)
	}
}

func TestClusterListBoundsEncodedPreviewAndOmitsManagedFields(t *testing.T) {
	c, _ := inspectorFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/configmaps" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"apiVersion": "v1", "kind": "ConfigMapList", "metadata": map[string]any{},
			"items": []any{
				map[string]any{"metadata": map[string]any{"name": "small", "managedFields": []any{map[string]any{"manager": "test"}}}},
				map[string]any{"metadata": map[string]any{"name": "large"}, "data": map[string]any{"value": strings.Repeat("<", 1500000)}},
			},
		})
	})
	result, err := Execute(context.Background(), c, "get configmaps")
	if err != nil || len(result.Rows) != 1 || !result.Truncated || !strings.Contains(result.Message, "8 MiB") {
		t.Fatalf("cluster budget: rows=%d truncated=%v err=%v", len(result.Rows), result.Truncated, err)
	}
	encoded, _ := json.Marshal(result)
	if len(encoded) > ResultByteLimit || strings.Contains(string(encoded), "managedFields") {
		t.Fatal("unbounded resource metadata")
	}
}
