package workspace

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ego-component/egorm"
	"github.com/ego-component/eredis"
	mysql "github.com/go-sql-driver/mysql"
	"github.com/gotomicro/ego/core/econf"
	redis "github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/plain"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const RowLimit = 500

var componentMu sync.Mutex

func address(c Connection) string { return net.JoinHostPort(c.Host, strconv.Itoa(c.Port)) }
func secureTLS(c Connection) *tls.Config {
	if c.TLS {
		return &tls.Config{MinVersion: tls.VersionTLS12, ServerName: c.Host}
	}
	return nil
}
func sqlClient(c Connection) (db *sql.DB, err error) {

	// Ego containers use global configuration during construction. Serialize that phase,
	// remove the DSN immediately, and never expose initialization panics to the desktop.
	componentMu.Lock()
	defer componentMu.Unlock()
	defer func() {
		if recover() != nil {
			err = errors.New("MySQL connection failed; check the address, credentials, database, and TLS settings")
		}
	}()
	cfg := mysql.NewConfig()
	cfg.User = c.Username
	cfg.Passwd = c.Password
	cfg.Net = "tcp"
	cfg.Addr = address(c)
	cfg.DBName = c.Database
	cfg.Timeout = 5 * time.Second
	cfg.ReadTimeout = 15 * time.Second
	cfg.WriteTimeout = 5 * time.Second
	cfg.ParseTime = true
	if c.TLS {
		cfg.TLSConfig = "true"
	}
	if c.tunnel != nil {
		cfg.Net = "infradex-ssh"
		connector, e := mysql.NewConnector(cfg)
		if e != nil {
			return nil, e
		}
		db := sql.OpenDB(sshSQLConnector{Connector: connector, tunnel: c.tunnel})
		db.SetMaxOpenConns(2)
		db.SetMaxIdleConns(1)
		return db, nil
	}
	econf.Set("infradex.mysql", map[string]any{"dsn": cfg.FormatDSN(), "onFail": "error", "maxOpenConns": 2, "maxIdleConns": 1, "enableMetricInterceptor": false, "enableTraceInterceptor": false})
	defer econf.Set("infradex.mysql", map[string]any{})
	component := egorm.Load("infradex.mysql").Build()
	if component == nil {
		return nil, errors.New("Unable to open MySQL connection")
	}
	return component.DB()
}
func mongoClient(ctx context.Context, c Connection) (*mongo.Client, error) {
	opts := options.Client().SetHosts([]string{address(c)}).SetConnectTimeout(5 * time.Second).SetServerSelectionTimeout(5 * time.Second).SetTLSConfig(secureTLS(c))
	if c.tunnel != nil {
		opts.SetDialer(c.tunnel)
	}
	if c.Username != "" {
		opts.SetAuth(options.Credential{Username: c.Username, Password: c.Password, AuthSource: mongoAuthSource(c)})
	}
	return mongo.Connect(ctx, opts)
}
func kafkaClient(c Connection) *kafka.Client {
	t := &kafka.Transport{TLS: secureTLS(c), DialTimeout: 5 * time.Second}
	if c.tunnel != nil {
		t.Dial = c.tunnel.DialContext
	}
	if t.TLS != nil {
		t.TLS.ServerName = ""
	} // Each advertised broker retains its own TLS identity.
	if c.Username != "" {
		t.SASL = plain.Mechanism{Username: c.Username, Password: c.Password}
	}
	return &kafka.Client{Addr: kafka.TCP(address(c)), Timeout: 10 * time.Second, Transport: t}
}
func kubeConfig(c Connection) (*rest.Config, error) {
	config, e := clientcmd.LoadFromFile(c.Kubeconfig)
	if e != nil {
		return nil, errors.New("Cannot read kubeconfig file")
	}
	if c.Context != "" {
		config.CurrentContext = c.Context
	}
	selected := config.Contexts[config.CurrentContext]
	if selected == nil {
		return nil, errors.New("Kubeconfig context was not found")
	}
	if auth := config.AuthInfos[selected.AuthInfo]; auth != nil && auth.Exec != nil {
		return nil, errors.New("Exec-based kubeconfig authentication is not supported yet; use a token or client certificate")
	}
	rest, e := clientcmd.NewDefaultClientConfig(*config, &clientcmd.ConfigOverrides{}).ClientConfig()
	if e != nil {
		return nil, e
	}
	if c.tunnel != nil {
		rest.Dial = c.tunnel.DialContext
		// Never route the tunneled API through an environment/kubeconfig proxy.
		rest.Proxy = func(*http.Request) (*url.URL, error) { return nil, nil }
	}
	rest.Timeout = 15 * time.Second
	return rest, nil
}
func kubeClient(c Connection) (dynamic.Interface, error) {
	cfg, e := kubeConfig(c)
	if e != nil {
		return nil, e
	}
	return dynamic.NewForConfig(cfg)
}
func Test(ctx context.Context, c Connection) (err error) {
	if err = Validate(c); err != nil {
		return err
	}
	c, closeTransport, transportErr := prepareTransport(ctx, c)
	if transportErr != nil {
		return transportErr
	}
	defer closeTransport()

	switch c.Kind {
	case "clickhouse":
		_, e := clickhouseQuery(ctx, c, "SELECT 1")
		return e
	case "mysql":
		db, e := sqlClient(c)
		if e != nil {
			return e
		}
		defer db.Close()
		return db.PingContext(ctx)
	case "redis":
		_, e := redisCommand(ctx, c, []string{"PING"})
		return e
	case "mongo":
		cl, e := mongoClient(ctx, c)
		if e != nil {
			return e
		}
		defer cl.Disconnect(context.Background())
		return cl.Ping(ctx, nil)
	case "kafka":
		cl := kafkaClient(c)
		defer cl.Transport.(*kafka.Transport).CloseIdleConnections()
		_, e := cl.Metadata(ctx, &kafka.MetadataRequest{})
		return e
	default:
		cl, e := kubeClient(c)
		if e != nil {
			return e
		}
		_, e = cl.Resource(schema.GroupVersionResource{Version: "v1", Resource: "pods"}).Namespace(strings.TrimSuffix(c.Database, "*")).List(ctx, metav1.ListOptions{Limit: 1})
		return e
	}
}
func redisCommand(ctx context.Context, c Connection, args []string) (value any, err error) {
	dbIndex := 0
	if c.Database != "" {
		dbIndex, err = strconv.Atoi(c.Database)
		if err != nil || dbIndex < 0 {
			return nil, errors.New("Invalid Redis database number")
		}
	}
	argv := make([]any, len(args))
	for i, s := range args {
		argv[i] = s
	}
	// eredis v1 does not expose a context dialer, forward ACL usernames, or reliably
	// preserve verified TLS configs. Use the upstream driver for SSH/ACL/TLS.
	if c.tunnel != nil || c.TLS || (c.Username != "" && c.Username != "default") {
		opts := &redis.Options{Addr: address(c), Username: c.Username, Password: c.Password, DB: dbIndex, TLSConfig: secureTLS(c), DialTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, MaxRetries: -1}
		if c.tunnel != nil {
			opts.Dialer = func(ctx context.Context, network, addr string) (net.Conn, error) {
				conn, e := c.tunnel.DialContext(ctx, network, addr)
				if e != nil || !c.TLS {
					return conn, e
				}
				secure := tls.Client(conn, secureTLS(c))
				handshakeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
				if e = secure.HandshakeContext(handshakeCtx); e != nil {
					conn.Close()
					return nil, e
				}
				return secure, nil
			}
		}
		cl := redis.NewClient(opts)
		defer cl.Close()
		return cl.Do(ctx, argv...).Result()
	}
	componentMu.Lock()
	var cl *eredis.Component
	func() {
		defer componentMu.Unlock()
		defer func() {
			if recover() != nil {
				err = errors.New("Unable to initialize Redis connection")
			}
		}()
		econf.Set("infradex.redis", map[string]any{"addr": address(c), "db": dbIndex, "password": c.Password, "onFail": "error", "maxRetries": -1, "minIdleConns": 0, "poolSize": 2, "dialTimeout": "5s", "readTimeout": "10s", "enableMetricInterceptor": false, "enableTraceInterceptor": false})
		defer econf.Set("infradex.redis", map[string]any{})
		cl = eredis.Load("infradex.redis").Build()
	}()
	if err != nil {
		return nil, err
	}
	defer cl.Stub().Close()
	return cl.Stub().Do(ctx, argv...).Result()
}
func Browse(ctx context.Context, c Connection) ([]Resource, error) {
	c, closeTransport, transportErr := prepareTransport(ctx, c)
	if transportErr != nil {
		return nil, transportErr
	}
	defer closeTransport()

	resources := []Resource{}
	switch c.Kind {
	case "mysql", "clickhouse":
		q := "SHOW TABLES"
		r, e := Execute(ctx, c, q)
		if e != nil {
			return nil, e
		}
		for _, row := range r.Rows {
			if len(row) > 0 {
				name := fmt.Sprint(row[0])
				resources = append(resources, Resource{name, "table", "SELECT * FROM `" + strings.ReplaceAll(name, "`", "``") + "` LIMIT 100;"})
			}
		}
	case "redis":
		v, e := redisCommand(ctx, c, []string{"SCAN", "0", "COUNT", "100"})
		if e != nil {
			return nil, e
		}
		items, ok := v.([]any)
		if ok && len(items) == 2 {
			if keys, ok := items[1].([]any); ok {
				for _, key := range keys {
					name := fmt.Sprint(key)
					encoded, _ := json.Marshal([]string{"TYPE", name})
					resources = append(resources, Resource{name, "key", string(encoded)})
				}
			}
		}
	case "mongo":
		cl, e := mongoClient(ctx, c)
		if e != nil {
			return nil, e
		}
		defer cl.Disconnect(context.Background())
		names, e := cl.Database(c.Database).ListCollectionNames(ctx, bson.D{})
		if e != nil {
			return nil, e
		}
		for _, name := range names {
			q, _ := json.MarshalIndent(map[string]any{"collection": name, "filter": map[string]any{}, "limit": 100}, "", "  ")
			resources = append(resources, Resource{name, "collection", string(q)})
		}
	case "kafka":
		cl := kafkaClient(c)
		defer cl.Transport.(*kafka.Transport).CloseIdleConnections()
		r, e := cl.Metadata(ctx, &kafka.MetadataRequest{})
		if e != nil {
			return nil, e
		}
		for _, t := range r.Topics {
			resources = append(resources, Resource{t.Name, "topic", t.Name})
		}
	default:
		for _, name := range []string{"pods", "deployments", "services", "namespaces", "nodes", "configmaps", "events"} {
			resources = append(resources, Resource{name, "resource", "get " + name})
		}
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Name < resources[j].Name })
	return resources, nil
}
func Execute(ctx context.Context, c Connection, q string) (result Result, err error) {
	c, closeTransport, transportErr := prepareTransport(ctx, c)
	if transportErr != nil {
		return Result{}, transportErr
	}
	defer closeTransport()

	result = Result{Columns: []string{}, Rows: [][]any{}}
	start := time.Now()
	defer func() { result.Duration = time.Since(start).Milliseconds() }()
	if strings.TrimSpace(q) == "" {
		return result, errors.New("Enter a query first")
	}
	switch c.Kind {
	case "clickhouse":
		return clickhouseQuery(ctx, c, q)
	case "mysql":
		db, e := sqlClient(c)
		if e != nil {
			return result, e
		}
		defer db.Close()
		tx, e := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if e != nil {
			return result, e
		}
		defer tx.Rollback()
		rows, e := tx.QueryContext(ctx, q)
		if e != nil {
			return result, e
		}
		defer rows.Close()
		return readSQLRows(rows)
	case "redis":
		args := strings.Fields(q)
		if strings.HasPrefix(strings.TrimSpace(q), "[") {
			if e := json.Unmarshal([]byte(q), &args); e != nil {
				return result, errors.New("Use a JSON array of command arguments")
			}
		}
		if len(args) == 0 {
			return result, errors.New("Enter a Redis command")
		}
		args[0] = strings.ToUpper(args[0])
		allowed := map[string]bool{"GETRANGE": true, "LRANGE": true, "XRANGE": true, "PING": true, "GET": true, "MGET": true, "TYPE": true, "TTL": true, "PTTL": true, "SCAN": true, "HSCAN": true, "SSCAN": true, "ZSCAN": true, "HLEN": true, "HGET": true, "LLEN": true, "SCARD": true, "ZCARD": true, "DBSIZE": true, "INFO": true}
		if !allowed[args[0]] {
			return result, errors.New("This workspace accepts read-only Redis commands: GET, MGET, TYPE, TTL, SCAN, HSCAN, SSCAN, ZSCAN, HGET, HLEN, LLEN, SCARD, ZCARD, DBSIZE, INFO, PING")
		}
		value, e := redisCommand(ctx, c, args)
		if e != nil {
			return result, e
		}
		result.Columns = []string{"response"}
		result.Rows = [][]any{{value}}
		encoded, e := json.Marshal(result)
		if e != nil || len(encoded) > ResultByteLimit {
			return emptyResult(), errors.New("Redis response exceeds the 8 MiB preview limit; use GETRANGE, bounded ranges, or SCAN with a smaller COUNT")
		}
	case "mongo":
		return mongoFind(ctx, c, q)
	case "kafka":
		cl := kafkaClient(c)
		defer cl.Transport.(*kafka.Transport).CloseIdleConnections()
		req := &kafka.MetadataRequest{}
		if strings.TrimSpace(q) != "topics" {
			req.Topics = []string{strings.TrimSpace(q)}
		}
		r, e := cl.Metadata(ctx, req)
		if e != nil {
			return result, e
		}
		result.Columns = []string{"topic", "partition", "leader", "replicas", "in_sync_replicas"}
		for _, t := range r.Topics {
			if t.Error != nil {
				return result, t.Error
			}
			for _, p := range t.Partitions {
				if p.Error != nil {
					return result, p.Error
				}
				if len(result.Rows) >= RowLimit {
					result.Truncated = true
					return result, nil
				}
				result.Rows = append(result.Rows, []any{t.Name, p.ID, p.Leader.ID, len(p.Replicas), len(p.Isr)})
			}
		}
	case "k8s", "k3s":
		parts := strings.Fields(q)
		if len(parts) < 2 || len(parts) > 3 || parts[0] != "get" {
			return result, errors.New("Use: get <resource> [namespace]")
		}
		group, version := "", "v1"
		switch parts[1] {
		case "deployments":
			group = "apps"
		case "pods", "services", "namespaces", "nodes", "configmaps", "events":
		default:
			return result, errors.New("Supported: pods, deployments, services, namespaces, nodes, configmaps, events")
		}
		namespace := c.Database
		if len(parts) == 3 {
			namespace = parts[2]
		}
		if namespace == "*" || parts[1] == "nodes" || parts[1] == "namespaces" {
			namespace = ""
		}
		cl, e := kubeClient(c)
		if e != nil {
			return result, e
		}
		r, e := cl.Resource(schema.GroupVersionResource{Group: group, Version: version, Resource: parts[1]}).Namespace(namespace).List(ctx, metav1.ListOptions{Limit: RowLimit})
		if e != nil {
			return result, e
		}
		result.ResourceType = parts[1]
		result.Columns = []string{"name", "namespace", "created", "resource"}
		budget := newResultBudget(result.Columns)
		for _, item := range r.Items {
			item.SetManagedFields(nil)
			if !budget.append(&result, []any{item.GetName(), item.GetNamespace(), item.GetCreationTimestamp().UTC().Format(time.RFC3339), item.Object}) {
				break
			}
		}
		result.Truncated = result.Truncated || r.GetContinue() != ""
	default:
		return result, errors.New("Unsupported connection type")
	}
	return result, nil
}
