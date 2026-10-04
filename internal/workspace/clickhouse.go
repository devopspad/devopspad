package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// egorm imports clickhouse-go v1 and registers database/sql's "clickhouse" name.
// A separate HTTP adapter avoids duplicate driver registration with v2 and works
// with both self-hosted ClickHouse and ClickHouse Cloud.
func clickhouseQuery(ctx context.Context, c Connection, q string) (Result, error) {
	result := Result{Columns: []string{}, Rows: [][]any{}}
	params := url.Values{"database": {c.Database}, "default_format": {"JSONCompact"}, "readonly": {"1"}, "max_execution_time": {"15"}, "max_result_rows": {"500"}, "result_overflow_mode": {"break"}}
	scheme := "http"
	if c.TLS {
		scheme = "https"
	}
	endpoint := url.URL{Scheme: scheme, Host: address(c), Path: "/", RawQuery: params.Encode()}
	req, e := http.NewRequestWithContext(ctx, "POST", endpoint.String(), strings.NewReader(q))
	if e != nil {
		return result, e
	}
	req.SetBasicAuth(c.Username, c.Password)
	req.Header.Set("Content-Type", "text/plain")
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = secureTLS(c)
	if c.tunnel != nil {
		transport.DialContext = c.tunnel.DialContext
		transport.Proxy = nil
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 20 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	start := time.Now()
	resp, e := client.Do(req)
	if e != nil {
		return result, errors.New("Cannot reach ClickHouse HTTP interface; check the host, port, and TLS setting")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return result, fmt.Errorf("ClickHouse HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var body struct {
		Meta []struct {
			Name string `json:"name"`
		} `json:"meta"`
		Data            [][]any `json:"data"`
		Rows            int     `json:"rows"`
		RowsBeforeLimit int     `json:"rows_before_limit_at_least"`
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 8<<20))
	decoder.UseNumber()
	if e = decoder.Decode(&body); e != nil {
		return result, errors.New("ClickHouse result exceeded 8 MB or was not JSONCompact; remove an explicit FORMAT clause or narrow the query")
	}
	for _, col := range body.Meta {
		result.Columns = append(result.Columns, col.Name)
	}
	result.Rows = body.Data
	for ri, row := range result.Rows {
		for ci, value := range row {
			result.Rows[ri][ci] = preserveJSONNumbers(value)
		}
	}
	if result.Rows == nil {
		result.Rows = [][]any{}
	}
	if len(result.Rows) > RowLimit {
		result.Rows = result.Rows[:RowLimit]
	}
	result.Truncated = body.Rows >= RowLimit || body.RowsBeforeLimit > body.Rows
	result.Duration = time.Since(start).Milliseconds()
	return result, nil
}

// Wails marshals Go results to JavaScript; keep integer precision across that boundary.
func preserveJSONNumbers(v any) any {
	switch value := v.(type) {
	case json.Number:
		return value.String()
	case []any:
		for i, item := range value {
			value[i] = preserveJSONNumbers(item)
		}
		return value
	case map[string]any:
		for k, item := range value {
			value[k] = preserveJSONNumbers(item)
		}
		return value
	default:
		return v
	}
}
