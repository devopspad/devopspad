package archive

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"devopspad/internal/workspace"
)

const maxKubeconfig = 1 << 20

// sshReadFile is replaced in tests.
var sshReadFile = workspace.SSHReadFile

// hostSSH turns a Host's ssh block into a tunnel configuration with its
// secrets resolved on this machine. Secrets stay in memory.
func (a *Archive) hostSSH(ref string) (workspace.SSHConfig, error) {
	r := a.Get(ref)
	if r == nil || r.Kind != "Host" {
		return workspace.SSHConfig{}, fmt.Errorf("%s is not a host", ref)
	}
	s := r.Object.(*Host).Spec.SSH
	if s == nil {
		return workspace.SSHConfig{}, fmt.Errorf("%s has no ssh block", ref)
	}
	port := s.Port
	if port == 0 {
		port = 22
	}
	cfg := workspace.SSHConfig{Enabled: true, Host: s.Host, Port: port, Username: s.User, AuthMethod: s.Auth, HostKeyFingerprint: s.HostKey, KnownHostsPath: s.KnownHosts}
	switch s.Auth {
	case "key":
		path, err := expandHome(strings.TrimPrefix(s.KeyRef, "file:"))
		if err != nil {
			return cfg, err
		}
		if !strings.HasPrefix(s.KeyRef, "file:") {
			return cfg, fmt.Errorf("%s: keyRef must be file:<private key path>", ref)
		}
		cfg.PrivateKeyPath = path
		if s.PassphraseRef != "" {
			v, err := ResolveSecret(s.PassphraseRef)
			cfg.Passphrase = v
			return cfg, err // cfg is complete except for a missing secret
		}
	case "password":
		v, err := ResolveSecret(s.PasswordRef)
		cfg.Password = v
		return cfg, err
	}
	return cfg, nil
}

// HostConnection is an SSH-only connection used to read facts from a host.
func (a *Archive) HostConnection(ref string) (workspace.Connection, error) {
	ssh, err := a.hostSSH(ref)
	if err != nil {
		return workspace.Connection{}, err
	}
	return workspace.Connection{Name: strings.TrimPrefix(ref, "host:"), SSH: ssh}, nil
}

// ClusterConnection builds a Kubernetes connection for a cluster. A remote
// kubeconfig is read over SSH and written to kubeDir (mode 0600); its API
// address is then reached through that host unless spec.tunnel says otherwise.
// fetch=false plans without network access.
func (a *Archive) ClusterConnection(ctx context.Context, ref, kubeDir string, fetch bool) (workspace.Connection, error) {
	r := a.Get(ref)
	if r == nil || r.Kind != "Cluster" {
		return workspace.Connection{}, fmt.Errorf("%s is not a cluster", ref)
	}
	spec := r.Object.(*Cluster).Spec
	c := workspace.Connection{Name: r.Name, Kind: spec.Distribution, Context: spec.Kubeconfig.Context, Environment: r.Header().Metadata.Labels["env"]}
	tunnel := spec.Tunnel
	switch spec.Kubeconfig.Source {
	case "local":
		p, err := expandHome(strings.TrimPrefix(spec.Kubeconfig.PathRef, "file:"))
		if err != nil {
			return c, err
		}
		c.Kubeconfig = p
	case "remote":
		if tunnel == "" {
			tunnel = spec.Kubeconfig.Host
		}
		c.Kubeconfig = filepath.Join(kubeDir, r.Name+".yaml")
		if fetch {
			host, err := a.HostConnection(spec.Kubeconfig.Host)
			if err != nil { // %v: without SSH there is no kubeconfig, so this is never "just a missing secret"
				return c, fmt.Errorf("read kubeconfig over SSH from %s: %v", spec.Kubeconfig.Host, err)
			}
			b, err := sshReadFile(ctx, host, spec.Kubeconfig.Path, maxKubeconfig)
			if err != nil {
				return c, fmt.Errorf("read kubeconfig from %s: %w", spec.Kubeconfig.Host, err)
			}
			if err := os.MkdirAll(kubeDir, 0o700); err != nil {
				return c, err
			}
			if err := writeAtomic(c.Kubeconfig, b, 0o600); err != nil {
				return c, err
			}
		}
	}
	if tunnel != "" {
		ssh, err := a.hostSSH(tunnel)
		c.SSH = ssh
		return c, err
	}
	return c, nil
}

var connectionKinds = map[string]string{"mysql": "mysql", "redis": "redis", "mongodb": "mongo", "clickhouse": "clickhouse", "kafka": "kafka"}

// DatastoreConnection builds a database connection with its password resolved.
func (a *Archive) DatastoreConnection(ref string) (workspace.Connection, error) {
	r := a.Get(ref)
	if r == nil || r.Kind != "Datastore" {
		return workspace.Connection{}, fmt.Errorf("%s is not a datastore", ref)
	}
	spec := r.Object.(*Datastore).Spec
	kind, ok := connectionKinds[spec.Engine]
	if !ok {
		return workspace.Connection{}, fmt.Errorf("DevOpsPad has no %s connection type", spec.Engine)
	}
	host, port, err := net.SplitHostPort(spec.Endpoint)
	if err != nil {
		return workspace.Connection{}, fmt.Errorf("%s: endpoint must be host:port", ref)
	}
	c := workspace.Connection{Name: r.Name, Kind: kind, Host: host, Username: spec.Username, TLS: spec.TLS, Environment: r.Header().Metadata.Labels["env"]}
	c.Port, _ = strconv.Atoi(port)
	if len(spec.Parts) > 0 && kind != "redis" {
		c.Database = spec.Parts[0]
	}
	var missing error
	if spec.Tunnel != "" {
		if c.SSH, err = a.hostSSH(spec.Tunnel); err != nil {
			if !errors.Is(err, errSecretMissing) {
				return c, err
			}
			missing = err
		}
	}
	for _, cr := range spec.Credentials {
		if cr.Name == "password" || len(spec.Credentials) == 1 {
			if c.Password, err = ResolveSecret(cr.Ref); err != nil && missing == nil {
				missing = err
			}
			break
		}
	}
	return c, missing
}

type SyncItem struct {
	Ref    string `json:"ref"`
	Name   string `json:"name"`
	Action string `json:"action"` // create | update | unchanged | skip
	Reason string `json:"reason,omitempty"`
}

// ConnectSync creates or updates desktop connections from the archive's
// clusters and datastores. Connections are matched by the
// devopspad.io/connection-id annotation, then by name. Secrets go to the OS
// keychain through the store, as in the desktop.
func (a *Archive) ConnectSync(ctx context.Context, store *workspace.Store, kubeDir string, dryRun bool) ([]SyncItem, error) {
	if err := store.Ready(); err != nil {
		return nil, err
	}
	existing := store.List()
	var items []SyncItem
	for _, r := range a.Resources {
		if r.Kind != "Cluster" && r.Kind != "Datastore" {
			continue
		}
		item := SyncItem{Ref: r.Ref, Name: r.Name}
		var want workspace.Connection
		var err error
		if r.Kind == "Cluster" {
			want, err = a.ClusterConnection(ctx, r.Ref, kubeDir, !dryRun)
		} else {
			want, err = a.DatastoreConnection(r.Ref)
		}
		var missingSecret error
		if err != nil {
			if !errors.Is(err, errSecretMissing) {
				item.Action, item.Reason = "skip", err.Error()
				items = append(items, item)
				continue
			}
			missingSecret = err
		}
		var cur *workspace.Connection
		id := r.Header().Metadata.Annotations[ConnectionAnnotation]
		for i := range existing {
			if (id != "" && existing[i].ID == id) || (cur == nil && existing[i].Name == want.Name && existing[i].Kind == want.Kind) {
				cur = &existing[i]
				if existing[i].ID == id {
					break
				}
			}
		}
		if cur == nil {
			item.Action = "create"
		} else {
			want.ID = cur.ID
			want.Name = cur.Name // keep the user's display name
			if sameConnection(*cur, want) {
				item.Action = "unchanged"
				items = append(items, item)
				continue
			}
			item.Action = "update"
		}
		if missingSecret != nil {
			item.Reason = "saved without a secret: " + missingSecret.Error()
		}
		if !dryRun {
			if _, err := store.Save(want); err != nil {
				item.Action, item.Reason = "skip", err.Error()
			}
		}
		items = append(items, item)
	}
	return items, nil
}

func sameConnection(a, b workspace.Connection) bool {
	sa, sb := a.SSH, b.SSH
	return a.Kind == b.Kind && a.Host == b.Host && a.Port == b.Port && a.Username == b.Username &&
		a.Database == b.Database && a.Environment == b.Environment && a.TLS == b.TLS &&
		a.Kubeconfig == b.Kubeconfig && a.Context == b.Context &&
		sa.Enabled == sb.Enabled && sa.Host == sb.Host && sa.Port == sb.Port && sa.Username == sb.Username &&
		sa.AuthMethod == sb.AuthMethod && sa.PrivateKeyPath == sb.PrivateKeyPath &&
		sa.KnownHostsPath == sb.KnownHostsPath && sa.HostKeyFingerprint == sb.HostKeyFingerprint &&
		(b.Password == "" || a.HasPassword) && (sb.Password == "" || sa.HasPassword) && (sb.Passphrase == "" || sa.HasPassphrase)
}
