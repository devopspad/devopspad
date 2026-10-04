package workspace

import (
	"errors"
	"strconv"
	"strings"
)

type Connection struct {
	SSH         SSHConfig `json:"ssh"`
	tunnel      *sshTunnel
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password,omitempty"`
	Database    string `json:"database"`
	AuthSource  string `json:"authSource,omitempty"`
	Environment string `json:"environment"`
	TLS         bool   `json:"tls"`
	Kubeconfig  string `json:"kubeconfig"`
	Context     string `json:"context"`
	HasPassword bool   `json:"hasPassword"`
}
type Resource struct {
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	Query string `json:"query"`
}
type Result struct {
	ResourceType  string   `json:"resourceType,omitempty"`
	Columns       []string `json:"columns"`
	Rows          [][]any  `json:"rows"`
	Duration      int64    `json:"duration"`
	Truncated     bool     `json:"truncated"`
	Message       string   `json:"message"`
	NextQuery     string   `json:"nextQuery,omitempty"`
	PreviousQuery string   `json:"previousQuery,omitempty"`
	Offset        int64    `json:"offset"`
}
type AISettings struct {
	Endpoint string `json:"endpoint"`
	Model    string `json:"model"`
	APIKey   string `json:"apiKey,omitempty"`
	HasKey   bool   `json:"hasKey"`
}
type AIRequest struct {
	Prompt string   `json:"prompt"`
	Kind   string   `json:"kind"`
	Schema []string `json:"schema"`
}

func Validate(c Connection) error {
	if err := validateSSH(c.SSH); err != nil {
		return err
	}
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("Connection name is required")
	}
	switch c.Kind {
	case "mysql", "redis", "mongo", "kafka", "clickhouse", "k8s", "k3s":
	default:
		return errors.New("Unsupported connection type")
	}
	if c.Kind == "k8s" || c.Kind == "k3s" {
		if strings.TrimSpace(c.Kubeconfig) == "" {
			return errors.New("Kubeconfig path is required")
		}
		return nil
	}
	if strings.TrimSpace(c.Host) == "" || strings.ContainsAny(c.Host, "/ @?#") {
		return errors.New("Enter a hostname or IP address without a URL scheme")
	}
	if c.Port < 1 || c.Port > 65535 {
		return errors.New("Port must be between 1 and 65535")
	}
	if c.Kind == "redis" && c.Database != "" {
		n, e := strconv.Atoi(c.Database)
		if e != nil || n < 0 {
			return errors.New("Redis database must be a non-negative integer")
		}
	}
	return nil
}
