package workspace

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
)

//go:embed capabilities.json
var capabilitiesJSON []byte

type Capabilities struct {
	Language          string   `json:"language"`
	ResourceLabel     string   `json:"resourceLabel"`
	ScopeLabel        string   `json:"scopeLabel"`
	CanSelectDatabase bool     `json:"canSelectDatabase"`
	CanInspect        bool     `json:"canInspect"`
	CanPageResources  bool     `json:"canPageResources"`
	Features          []string `json:"features"`
	Limitations       []string `json:"limitations"`
}

func CapabilityCatalog() map[string]Capabilities {
	var catalog map[string]Capabilities
	if err := json.Unmarshal(capabilitiesJSON, &catalog); err != nil {
		panic("invalid built-in capability catalog")
	}
	return catalog
}

type BrowseRequest struct {
	Database string `json:"database"`
	Cursor   string `json:"cursor"`
	Filter   string `json:"filter"`
}
type ResourcePage struct {
	Resources  []Resource `json:"resources"`
	NextCursor string     `json:"nextCursor"`
	HasMore    bool       `json:"hasMore"`
	Message    string     `json:"message"`
}
type Property struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type ResourceDetails struct {
	Name       string     `json:"name"`
	Database   string     `json:"database"`
	Kind       string     `json:"kind"`
	Properties []Property `json:"properties"`
	Columns    Result     `json:"columns"`
	Indexes    Result     `json:"indexes"`
	Preview    Result     `json:"preview"`
	Definition string     `json:"definition"`
}

func emptyResult() Result { return Result{Columns: []string{}, Rows: [][]any{}} }
func scoped(c Connection, database string) (Connection, error) {
	if len(database) > 256 || strings.ContainsRune(database, 0) {
		return c, errors.New("Invalid database name")
	}
	if database != "" {
		c.Database = database
	}
	return c, nil
}
func ListDatabases(ctx context.Context, c Connection) ([]string, error) {
	c, closeTransport, transportErr := prepareTransport(ctx, c)
	if transportErr != nil {
		return nil, transportErr
	}
	defer closeTransport()

	if !CapabilityCatalog()[c.Kind].CanSelectDatabase {
		return nil, errors.New("This engine does not support database navigation")
	}
	if c.Kind == "k8s" || c.Kind == "k3s" {
		return listNamespaces(ctx, c)
	}
	if c.Kind == "mongo" {
		cl, e := mongoClient(ctx, c)
		if e != nil {
			return nil, e
		}
		defer cl.Disconnect(context.Background())
		names, e := cl.ListDatabaseNames(ctx, bson.D{})
		sort.Strings(names)
		return names, e
	}
	// Do not select a saved database during discovery: it may have been removed.
	c.Database = ""
	r, e := Execute(ctx, c, "SHOW DATABASES")
	if e != nil {
		return nil, e
	}
	names := []string{}
	for _, row := range r.Rows {
		if len(row) > 0 {
			names = append(names, fmt.Sprint(row[0]))
		}
	}
	sort.Strings(names)
	return names, nil
}
func BrowsePage(ctx context.Context, c Connection, request BrowseRequest) (ResourcePage, error) {
	c, closeTransport, transportErr := prepareTransport(ctx, c)
	if transportErr != nil {
		return ResourcePage{}, transportErr
	}
	defer closeTransport()

	page := ResourcePage{Resources: []Resource{}}
	var e error
	c, e = scoped(c, request.Database)
	if e != nil {
		return page, e
	}
	if c.Kind != "redis" {
		if c.Kind == "k8s" || c.Kind == "k3s" {
			if e = Test(ctx, c); e != nil {
				return page, e
			}
		}
		page.Resources, e = Browse(ctx, c)
		return page, e
	}
	cursor := request.Cursor
	if cursor == "" {
		cursor = "0"
	}
	if _, e = strconv.ParseUint(cursor, 10, 64); e != nil {
		return page, errors.New("Invalid Redis scan cursor")
	}
	if len(request.Filter) > 1024 {
		return page, errors.New("Key pattern is too long")
	}
	pattern := request.Filter
	if pattern == "" {
		pattern = "*"
	}
	value, e := redisCommand(ctx, c, []string{"SCAN", cursor, "MATCH", pattern, "COUNT", "100"})
	if e != nil {
		return page, e
	}
	next, items, e := scanResponse(value)
	if e != nil {
		return page, e
	}
	page.NextCursor = next
	page.HasMore = next != "0"
	seen := map[string]bool{}
	for _, item := range items {
		name := fmt.Sprint(item)
		if seen[name] {
			continue
		}
		seen[name] = true
		q, _ := json.Marshal([]string{"TYPE", name})
		page.Resources = append(page.Resources, Resource{Name: name, Kind: "key", Query: string(q)})
	}
	sort.Slice(page.Resources, func(i, j int) bool { return page.Resources[i].Name < page.Resources[j].Name })
	page.Message = "SCAN may return an empty batch before it reaches the end."
	return page, nil
}
func scanResponse(value any) (string, []any, error) {
	outer, ok := value.([]any)
	if !ok || len(outer) != 2 {
		return "", nil, errors.New("Unexpected Redis scan response")
	}
	cursor := fmt.Sprint(outer[0])
	if _, e := strconv.ParseUint(cursor, 10, 64); e != nil {
		return "", nil, e
	}
	items, ok := outer[1].([]any)
	if !ok {
		if stringsList, yes := outer[1].([]string); yes {
			items = make([]any, len(stringsList))
			for i, s := range stringsList {
				items[i] = s
			}
		} else {
			return "", nil, errors.New("Unexpected Redis scan values")
		}
	}
	return cursor, items, nil
}
func quoteIdentifier(kind, name string) string {
	if kind == "clickhouse" {
		name = strings.ReplaceAll(name, "\\", "\\\\")
		name = strings.ReplaceAll(name, "`", "\\`")
	} else {
		name = strings.ReplaceAll(name, "`", "``")
	}
	return "`" + name + "`"
}
func quoteClickHouseString(value string) string {
	return "'" + strings.ReplaceAll(strings.ReplaceAll(value, "\\", "\\\\"), "'", "\\'") + "'"
}
func InspectResource(ctx context.Context, c Connection, database, name string) (ResourceDetails, error) {
	c, closeTransport, transportErr := prepareTransport(ctx, c)
	if transportErr != nil {
		return ResourceDetails{}, transportErr
	}
	defer closeTransport()

	d := ResourceDetails{Name: name, Kind: c.Kind, Properties: []Property{}, Columns: emptyResult(), Indexes: emptyResult(), Preview: emptyResult()}
	if name == "" || len(name) > 4096 || strings.ContainsRune(name, 0) {
		return d, errors.New("Invalid resource name")
	}
	if !CapabilityCatalog()[c.Kind].CanInspect {
		return d, errors.New("Resource inspection is not available for this engine")
	}
	var e error
	c, e = scoped(c, database)
	if e != nil {
		return d, e
	}
	d.Database = c.Database
	if c.Kind == "redis" {
		return inspectRedis(ctx, c, name, d)
	}
	if c.Database == "" {
		return d, errors.New("Select a database before inspecting a table")
	}
	qualified := quoteIdentifier(c.Kind, c.Database) + "." + quoteIdentifier(c.Kind, name)
	ddl, e := Execute(ctx, c, "SHOW CREATE TABLE "+qualified)
	if e != nil {
		return d, e
	}
	d.Definition = definitionText(ddl)
	isView := false
	for _, column := range ddl.Columns {
		if strings.EqualFold(column, "Create View") {
			isView = true
		}
	}
	if c.Kind == "mysql" {
		d.Columns, e = Execute(ctx, c, "SHOW FULL COLUMNS FROM "+qualified)
		if e != nil {
			return d, e
		}
		if !isView {
			d.Indexes, e = Execute(ctx, c, "SHOW INDEX FROM "+qualified)
		}
	} else {
		d.Columns, e = Execute(ctx, c, "DESCRIBE TABLE "+qualified)
		if e != nil {
			return d, e
		}
		d.Indexes, e = Execute(ctx, c, "SELECT name, type, expr, granularity FROM system.data_skipping_indices WHERE database = "+quoteClickHouseString(c.Database)+" AND table = "+quoteClickHouseString(name))
	}
	if e != nil {
		return d, e
	}

	d.Properties = []Property{{"Database", c.Database}, {"Columns", strconv.Itoa(len(d.Columns.Rows))}, {"Index entries", strconv.Itoa(len(d.Indexes.Rows))}}
	return d, nil
}
func definitionText(result Result) string {
	if len(result.Rows) == 0 {
		return ""
	}
	for i, column := range result.Columns {
		if (strings.HasPrefix(strings.ToLower(column), "create ") || strings.EqualFold(column, "statement")) && i < len(result.Rows[0]) {
			return fmt.Sprint(result.Rows[0][i])
		}
	}
	return ""
}

func inspectRedis(ctx context.Context, c Connection, name string, d ResourceDetails) (ResourceDetails, error) {
	kind, e := redisCommand(ctx, c, []string{"TYPE", name})
	if e != nil {
		return d, e
	}
	keyType := fmt.Sprint(kind)
	if keyType == "none" {
		return d, errors.New("This key has expired or no longer exists")
	}
	ttl, e := redisCommand(ctx, c, []string{"PTTL", name})
	if e != nil {
		return d, e
	}
	ttlText := fmt.Sprint(ttl) + " ms"
	if fmt.Sprint(ttl) == "-1" {
		ttlText = "No expiration"
	}
	if fmt.Sprint(ttl) == "-2" {
		return d, errors.New("This key expired while it was being inspected")
	}
	d.Properties = []Property{{"Type", keyType}, {"TTL", ttlText}, {"Database", c.Database}}
	var args []string
	switch keyType {
	case "string":
		args = []string{"GETRANGE", name, "0", "65535"}
		d.Preview.Columns = []string{"value"}
	case "hash":
		args = []string{"HSCAN", name, "0", "COUNT", "100"}
		d.Preview.Columns = []string{"field", "value"}
	case "set":
		args = []string{"SSCAN", name, "0", "COUNT", "100"}
		d.Preview.Columns = []string{"member"}
	case "zset":
		args = []string{"ZSCAN", name, "0", "COUNT", "100"}
		d.Preview.Columns = []string{"member", "score"}
	case "list":
		args = []string{"LRANGE", name, "0", "99"}
		d.Preview.Columns = []string{"index", "value"}
	case "stream":
		args = []string{"XRANGE", name, "-", "+", "COUNT", "100"}
		d.Preview.Columns = []string{"id", "fields"}
	default:
		d.Preview.Message = "Preview is not available for this Redis module type."
		return d, nil
	}
	value, e := redisCommand(ctx, c, args)
	if e != nil {
		return d, e
	}
	if keyType == "string" {
		text := fmt.Sprint(value)
		d.Preview.Rows = [][]any{{text}}
		d.Preview.Truncated = len(text) >= 65536
		d.Preview.Message = "String preview is limited to 64 KiB."
		return d, nil
	}
	var items []any
	if keyType == "hash" || keyType == "set" || keyType == "zset" {
		var cursor string
		cursor, items, e = scanResponse(value)
		if e != nil {
			return d, e
		}
		d.Preview.Truncated = cursor != "0"
		d.Preview.Message = "First scan batch. Use the query editor with the next cursor to continue: " + cursor
	} else {
		var ok bool
		items, ok = value.([]any)
		if !ok {
			return d, errors.New("Unexpected Redis collection response")
		}
		d.Preview.Truncated = len(items) >= 100
	}
	for i := 0; i < len(items) && len(d.Preview.Rows) < 100; i++ {
		switch keyType {
		case "hash", "zset":
			if i+1 >= len(items) {
				return d, errors.New("Incomplete Redis field/value pair")
			}
			d.Preview.Rows = append(d.Preview.Rows, []any{items[i], items[i+1]})
			i++
		case "list":
			d.Preview.Rows = append(d.Preview.Rows, []any{i, items[i]})
		case "stream":
			pair, ok := items[i].([]any)
			if !ok || len(pair) != 2 {
				return d, errors.New("Invalid Redis stream entry")
			}
			d.Preview.Rows = append(d.Preview.Rows, pair)
		default:
			d.Preview.Rows = append(d.Preview.Rows, []any{items[i]})
		}
	}
	limit := 100
	if keyType == "hash" || keyType == "zset" {
		limit = 200
	}
	if len(items) > limit {
		d.Preview.Truncated = true
	}
	return d, nil
}
