package workspace

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

const workspaceVersion = 1
const configByteLimit = 4 << 20

var errInvalidConfig = errors.New("Workspace configuration is damaged or invalid. Open Workspace health to review recovery options.")
var errFutureConfig = errors.New("This workspace was saved by a newer app version. Open it with a compatible version; no files have been changed.")
var errReadConfig = errors.New("Workspace configuration could not be read. Check file access, then retry in Workspace health.")
var errLargeConfig = errors.New("Workspace configuration exceeds 4 MiB. Preserve the file and reduce its size before retrying.")

func defaultState() diskState {
	return diskState{Version: workspaceVersion, Connections: []Connection{}, AI: AISettings{Endpoint: "https://api.openai.com/v1"}}
}

func decodeState(b []byte) (diskState, error) {
	var header map[string]json.RawMessage
	if json.Unmarshal(b, &header) != nil || header == nil {
		return diskState{}, errInvalidConfig
	}
	var version int
	if raw, ok := header["version"]; ok {
		if string(raw) == "null" || json.Unmarshal(raw, &version) != nil || version < 0 {
			return diskState{}, errInvalidConfig
		}
	}
	if version > workspaceVersion {
		return diskState{}, errFutureConfig
	}
	if len(header["connections"]) == 0 || bytes.Equal(header["connections"], []byte("null")) ||
		len(header["ai"]) == 0 || bytes.Equal(header["ai"], []byte("null")) {
		return diskState{}, errInvalidConfig
	}
	state := defaultState()
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&state) != nil {
		return diskState{}, errInvalidConfig
	}
	ids := map[string]bool{}
	for _, c := range state.Connections {
		if _, err := uuid.Parse(c.ID); err != nil || ids[c.ID] || Validate(c) != nil ||
			c.Password != "" || c.SSH.Password != "" || c.SSH.Passphrase != "" {
			return diskState{}, errInvalidConfig
		}
		ids[c.ID] = true
	}
	// An unconfigured provider has no model yet, but its endpoint must still be safe.
	if state.AI.APIKey != "" || validateAI(AISettings{Endpoint: state.AI.Endpoint, Model: "validation"}) != nil {
		return diskState{}, errInvalidConfig
	}
	state.Version = workspaceVersion
	return state, nil
}

func readConfig(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errReadConfig
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, configByteLimit+1))
	if err != nil {
		return nil, err
	}
	if len(b) > configByteLimit {
		return nil, errLargeConfig
	}
	return b, nil
}

func atomicWrite(path string, b []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".workspace-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func (s *Store) backupPath() string { return s.path + ".backup" }

// Caller holds s.mu (except during construction). Never replace unreadable,
// invalid or newer-version metadata with an empty workspace.
func (s *Store) loadLocked() {
	s.loadErr, s.loadCode = nil, "ready"
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		s.loadErr, s.loadCode = errReadConfig, "unreadable"
		return
	}
	b, err := readConfig(s.path)
	if errors.Is(err, os.ErrNotExist) {
		// A missing primary with a backup is recoverable, not a new installation.
		if _, backupErr := os.Lstat(s.backupPath()); !errors.Is(backupErr, os.ErrNotExist) {
			s.loadErr, s.loadCode = errInvalidConfig, "missing"
			return
		}
		s.state = defaultState()
		return
	}
	if err != nil {
		s.loadErr, s.loadCode = errReadConfig, "unreadable"
		if errors.Is(err, errLargeConfig) {
			s.loadErr, s.loadCode = errLargeConfig, "too_large"
		}
		return
	}
	state, err := decodeState(b)
	if err != nil {
		s.loadErr, s.loadCode = err, "invalid"
		if errors.Is(err, errFutureConfig) {
			s.loadCode = "newer_version"
		}
		return
	}
	s.state = state
	// Legacy metadata is migrated in memory and saved in versioned form on the
	// next edit. The validated primary becomes the latest recovery copy.
	s.backupFailed = atomicWrite(s.backupPath(), b) != nil
}

func (s *Store) Ready() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadErr
}

type StoreHealth struct {
	Status          string `json:"status"`
	Message         string `json:"message"`
	CanRestore      bool   `json:"canRestore"`
	BackupAvailable bool   `json:"backupAvailable"`
	BackupUpdatedAt string `json:"backupUpdatedAt"`
	BackupWarning   bool   `json:"backupWarning"`
	ConfigVersion   int    `json:"configVersion"`
}

func (s *Store) Health() StoreHealth {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := StoreHealth{Status: s.loadCode, ConfigVersion: workspaceVersion, BackupWarning: s.backupFailed}
	if s.loadErr != nil {
		h.Message = s.loadErr.Error()
	}
	if b, err := readConfig(s.backupPath()); err == nil {
		if _, err = decodeState(b); err == nil {
			h.BackupAvailable = true
			if info, err := os.Stat(s.backupPath()); err == nil {
				h.BackupUpdatedAt = info.ModTime().UTC().Format(time.RFC3339)
			}
		}
	}
	h.CanRestore = h.BackupAvailable && (s.loadCode == "invalid" || s.loadCode == "missing")
	return h
}

func (s *Store) RetryLoad() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadErr == nil {
		return nil
	}
	s.loadLocked()
	return s.loadErr
}

// Restore is an explicit action for a blocked workspace. The backup is metadata
// only: keychain values are neither read nor overwritten, and may need re-entry.
func (s *Store) RestoreBackup() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadCode != "invalid" && s.loadCode != "missing" {
		return errors.New("Backup recovery is not available for this workspace state")
	}
	b, err := readConfig(s.backupPath())
	if err != nil {
		return errors.New("Recovery copy could not be read; no files were changed")
	}
	state, err := decodeState(b)
	if err != nil {
		return errors.New("Recovery copy is invalid or incompatible; no files were changed")
	}
	original, err := readConfig(s.path)
	if err == nil {
		if _, decodeErr := decodeState(original); !errors.Is(decodeErr, errInvalidConfig) {
			return errors.New("Workspace changed since startup. Retry loading it before recovery.")
		}
		// Keep the exact damaged bytes in a new owner-only archive before replacing.
		archive := filepath.Join(filepath.Dir(s.path), "workspace.damaged-"+uuid.NewString()+".json")
		if err = atomicWrite(archive, original); err != nil {
			return errors.New("Could not preserve the damaged file; recovery was not applied")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("Could not preserve the original file; recovery was not applied")
	}
	if err = atomicWrite(s.path, b); err != nil {
		return errors.New("Could not restore the workspace; the recovery copy is unchanged")
	}
	s.state, s.loadErr, s.loadCode, s.backupFailed = state, nil, "ready", false
	return nil
}
