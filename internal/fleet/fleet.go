package fleet

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

const maxMachines = 32

var (
	fleetName   = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	machineName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
)

// File is the local fleet inventory. It stores connection IDs, not secrets.
type File struct {
	Fleets []Fleet `json:"fleets"`
}

type Fleet struct {
	Name     string    `json:"name"`
	Machines []Machine `json:"machines"`
}

type Machine struct {
	Name         string   `json:"name"`
	ConnectionID string   `json:"connectionId"`
	Roles        []string `json:"roles"`
}

func Load(configDir string, hasConnection func(id string) bool) (File, error) {
	path := filepath.Join(configDir, "fleet.json")
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return File{}, nil
	}
	if err != nil {
		return File{}, err
	}
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		return File{}, fmt.Errorf("read fleet file: %w", err)
	}
	if err := validate(f, hasConnection); err != nil {
		return File{}, err
	}
	return f, nil
}

func Save(configDir string, f File) error {
	if err := validate(f, func(string) bool { return true }); err != nil {
		return err
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return err
	}
	return atomicWrite(filepath.Join(configDir, "fleet.json"), b)
}

func validate(f File, hasConnection func(id string) bool) error {
	if hasConnection == nil {
		return errors.New("connection check is required")
	}
	seenFleet := map[string]struct{}{}
	for _, fl := range f.Fleets {
		if !fleetName.MatchString(fl.Name) {
			return fmt.Errorf("fleet name %q is invalid", fl.Name)
		}
		if _, ok := seenFleet[fl.Name]; ok {
			return fmt.Errorf("duplicate fleet %s", fl.Name)
		}
		seenFleet[fl.Name] = struct{}{}
		if len(fl.Machines) > maxMachines {
			return fmt.Errorf("fleet %s exceeds 32 machines", fl.Name)
		}
		seenMachine := map[string]struct{}{}
		for _, m := range fl.Machines {
			if !machineName.MatchString(m.Name) {
				return fmt.Errorf("machine name %q is invalid", m.Name)
			}
			if _, ok := seenMachine[m.Name]; ok {
				return fmt.Errorf("duplicate machine %s", m.Name)
			}
			seenMachine[m.Name] = struct{}{}
			if m.ConnectionID == "" || hasPathChars(m.ConnectionID) {
				return fmt.Errorf("machine %s has an invalid connection", m.Name)
			}
			if !hasConnection(m.ConnectionID) {
				return fmt.Errorf("machine %s references a missing connection", m.Name)
			}
			if err := validateRoles(m.Roles); err != nil {
				return fmt.Errorf("machine %s: %w", m.Name, err)
			}
		}
	}
	return nil
}

func validateRoles(roles []string) error {
	if len(roles) == 0 {
		return errors.New("needs a cluster or edge role")
	}
	seen := map[string]struct{}{}
	for _, role := range roles {
		if role != "cluster" && role != "edge" {
			return fmt.Errorf("role %q is invalid", role)
		}
		if _, ok := seen[role]; ok {
			return fmt.Errorf("duplicate role %s", role)
		}
		seen[role] = struct{}{}
	}
	return nil
}

func hasPathChars(s string) bool {
	for _, r := range s {
		if r == '/' || r == '\\' || r == 0 || r == '\n' || r == '\r' {
			return true
		}
	}
	return false
}

func atomicWrite(path string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".fleet-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
