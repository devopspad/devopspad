package workspace

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/zalando/go-keyring"
)

type Vault interface {
	Set(string, string) error
	Get(string) (string, error)
	Delete(string) error
}
type Keychain struct{}

func (Keychain) Set(k, v string) error        { return keyring.Set("devopspad", k, v) }
func (Keychain) Get(k string) (string, error) { return keyring.Get("devopspad", k) }
func (Keychain) Delete(k string) error        { return keyring.Delete("devopspad", k) }

type diskState struct {
	Version     int          `json:"version"`
	Connections []Connection `json:"connections"`
	AI          AISettings   `json:"ai"`
}
type Store struct {
	mu           sync.Mutex
	path         string
	vault        Vault
	state        diskState
	loadErr      error
	loadCode     string
	backupFailed bool
}

func NewStore(dir string, vault Vault) (*Store, error) {
	s := &Store{path: filepath.Join(dir, "workspace.json"), vault: vault, state: defaultState()}
	s.loadLocked()
	return s, s.loadErr
}
func (s *Store) persist() error {
	if s.loadErr != nil {
		return s.loadErr
	}
	s.state.Version = workspaceVersion
	b, e := json.MarshalIndent(s.state, "", "  ")
	if e != nil {
		return e
	}
	if len(b) > configByteLimit {
		return errors.New("Workspace configuration exceeds 4 MiB")
	}
	if e = atomicWrite(s.path, b); e != nil {
		return e
	}
	// The primary file is already committed. A failed backup must not roll back
	// keychain changes for a successful primary write; report degraded protection.
	s.backupFailed = atomicWrite(s.backupPath(), b) != nil
	return nil
}

// Secret changes and metadata writes either succeed together or restore the prior
// vault value. The caller holds s.mu and restores the in-memory state on failure.
func (s *Store) withSecret(key, value string, hadOld, change bool, commit func() error) error {
	if !change {
		return commit()
	}
	old := ""
	var e error
	if hadOld {
		old, e = s.vault.Get(key)
		if e != nil {
			return errors.New("Could not access the saved secret in the OS keychain")
		}
	}
	if value == "" {
		if hadOld {
			e = s.vault.Delete(key)
		}
	} else {
		e = s.vault.Set(key, value)
	}
	if e != nil {
		return errors.New("Could not update the OS keychain: " + e.Error())
	}
	if e = commit(); e != nil {
		var rollback error
		if hadOld {
			rollback = s.vault.Set(key, old)
		} else if value != "" {
			rollback = s.vault.Delete(key)
		}
		return errors.Join(e, rollback)
	}
	return nil
}
func (s *Store) List() []Connection {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Connection{}, s.state.Connections...)
}
func (s *Store) Save(c Connection) (Connection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadErr != nil {
		return Connection{}, s.loadErr
	}
	if e := Validate(c); e != nil {
		return Connection{}, e
	}
	idx := -1
	hadOld := false
	var oldSSH SSHConfig
	if c.ID == "" {
		c.ID = uuid.NewString()
	} else {
		for i, old := range s.state.Connections {
			if old.ID == c.ID {
				idx = i
				hadOld = old.HasPassword
				oldSSH = old.SSH
			}
		}
		if idx < 0 {
			return Connection{}, errors.New("Connection not found")
		}
	}
	secret := c.Password
	c.Password = ""
	c.HasPassword = hadOld || secret != ""
	sshPassword, sshPassphrase := c.SSH.Password, c.SSH.Passphrase
	c.SSH.Password, c.SSH.Passphrase = "", ""
	keepPassword := c.SSH.Enabled && c.SSH.AuthMethod == "password"
	keepPassphrase := c.SSH.Enabled && c.SSH.AuthMethod == "key"
	if !keepPassword {
		sshPassword = ""
	}
	if !keepPassphrase {
		sshPassphrase = ""
	}
	c.SSH.HasPassword = keepPassword && (oldSSH.HasPassword || sshPassword != "")
	c.SSH.HasPassphrase = keepPassphrase && (oldSSH.HasPassphrase || sshPassphrase != "")
	previous := append([]Connection{}, s.state.Connections...)
	err := s.withSecret(c.ID, secret, hadOld, secret != "", func() error {
		return s.withSecret(c.ID+":ssh-password", sshPassword, oldSSH.HasPassword, sshPassword != "" || (oldSSH.HasPassword && !keepPassword), func() error {
			return s.withSecret(c.ID+":ssh-passphrase", sshPassphrase, oldSSH.HasPassphrase, sshPassphrase != "" || (oldSSH.HasPassphrase && !keepPassphrase), func() error {
				if idx < 0 {
					s.state.Connections = append(s.state.Connections, c)
				} else {
					s.state.Connections[idx] = c
				}
				return s.persist()
			})
		})
	})
	if err != nil {
		s.state.Connections = previous
		return Connection{}, err
	}
	return c, nil
}
func (s *Store) Resolve(c Connection) (Connection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resolveLocked(c)
}
func (s *Store) resolveLocked(c Connection) (Connection, error) {
	if s.loadErr != nil {
		return Connection{}, s.loadErr
	}
	if c.ID == "" {
		return c, nil
	}
	for _, saved := range s.state.Connections {
		if saved.ID == c.ID {
			if saved.HasPassword && c.Password == "" {
				p, e := s.vault.Get(c.ID)
				if e != nil {
					return c, errors.New("Could not read password from OS keychain")
				}
				c.Password = p
			}
			if c.SSH.Enabled {
				if c.SSH.AuthMethod == "password" && c.SSH.Password == "" && saved.SSH.HasPassword {
					p, e := s.vault.Get(c.ID + ":ssh-password")
					if e != nil {
						return c, errors.New("Could not read SSH password from OS keychain")
					}
					c.SSH.Password = p
				}
				if c.SSH.AuthMethod == "key" && c.SSH.Passphrase == "" && saved.SSH.HasPassphrase {
					p, e := s.vault.Get(c.ID + ":ssh-passphrase")
					if e != nil {
						return c, errors.New("Could not read SSH key passphrase from OS keychain")
					}
					c.SSH.Passphrase = p
				}
			}
			return c, nil
		}
	}
	return Connection{}, errors.New("Connection not found")
}
func (s *Store) Get(id string) (Connection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadErr != nil {
		return Connection{}, s.loadErr
	}
	for _, c := range s.state.Connections {
		if c.ID == id {
			return s.resolveLocked(c)
		}
	}
	return Connection{}, errors.New("Connection not found")
}
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadErr != nil {
		return s.loadErr
	}
	for i, c := range s.state.Connections {
		if c.ID == id {
			old := append([]Connection{}, s.state.Connections...)
			e := s.withSecret(id, "", c.HasPassword, c.HasPassword, func() error {
				return s.withSecret(id+":ssh-password", "", c.SSH.HasPassword, c.SSH.HasPassword, func() error {
					return s.withSecret(id+":ssh-passphrase", "", c.SSH.HasPassphrase, c.SSH.HasPassphrase, func() error {
						s.state.Connections = append(s.state.Connections[:i], s.state.Connections[i+1:]...)
						return s.persist()
					})
				})
			})
			if e != nil {
				s.state.Connections = old
			}
			return e
		}
	}
	return errors.New("Connection not found")
}
func (s *Store) AI() AISettings { s.mu.Lock(); defer s.mu.Unlock(); return s.state.AI }
func (s *Store) SaveAI(c AISettings) error {
	if e := validateAI(c); e != nil {
		return e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadErr != nil {
		return s.loadErr
	}
	old := s.state.AI
	secret := c.APIKey
	c.APIKey = ""
	changedEndpoint := strings.TrimRight(c.Endpoint, "/") != strings.TrimRight(old.Endpoint, "/")
	c.HasKey = secret != "" || (old.HasKey && !changedEndpoint)
	e := s.withSecret("ai-api-key", secret, old.HasKey, secret != "" || (changedEndpoint && old.HasKey), func() error { s.state.AI = c; return s.persist() })
	if e != nil {
		s.state.AI = old
	}
	return e
}
