package workspace

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestLegacyWorkspaceLoadsWithoutRewritingAndSavesVersioned(t *testing.T) {
	s, v := testStore(t)
	c, err := s.Save(testConnection())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(s.path)
	var old map[string]any
	json.Unmarshal(b, &old)
	delete(old, "version")
	legacy, _ := json.Marshal(old)
	os.WriteFile(s.path, legacy, 0600)
	s, err = NewStore(filepath.Dir(s.path), v)
	if err != nil {
		t.Fatal(err)
	}
	unchanged, _ := os.ReadFile(s.path)
	if !bytes.Equal(legacy, unchanged) {
		t.Fatal("opening legacy metadata modified the primary")
	}
	if _, err = s.Save(c); err != nil {
		t.Fatal(err)
	}
	current, _ := os.ReadFile(s.path)
	backup, _ := os.ReadFile(s.backupPath())
	if !bytes.Contains(current, []byte(`"version": 1`)) || !bytes.Equal(current, backup) {
		t.Fatal("missing current recovery copy")
	}
	for _, path := range []string{s.path, s.backupPath()} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm()&0077 != 0 {
			t.Fatal("metadata must be owner-only", err)
		}
	}
}

func TestCorruptWorkspaceBlocksWritesAndRecoversWithoutLosingOriginal(t *testing.T) {
	s, v := testStore(t)
	c, err := s.Save(testConnection())
	if err != nil {
		t.Fatal(err)
	}
	broken := []byte(`{"connections": [THIS-IS-DAMAGED`)
	os.WriteFile(s.path, broken, 0600)
	blocked, err := NewStore(filepath.Dir(s.path), v)
	if err == nil || blocked == nil || blocked.Ready() == nil {
		t.Fatal("corrupt metadata accepted")
	}
	if !blocked.Health().CanRestore {
		t.Fatal("valid backup not offered")
	}
	if _, err = blocked.Save(testConnection()); err == nil {
		t.Fatal("saved over corrupt metadata")
	}
	if err = blocked.SaveAI(AISettings{Endpoint: "https://example.test", Model: "test", APIKey: "new-key"}); err == nil {
		t.Fatal("changed keychain while blocked")
	}
	if len(v.data) != 1 || v.data[c.ID] != "do-not-persist" {
		t.Fatal("blocked operations changed secrets")
	}
	if err = blocked.RestoreBackup(); err != nil {
		t.Fatal(err)
	}
	archives, _ := filepath.Glob(filepath.Join(filepath.Dir(s.path), "workspace.damaged-*.json"))
	if len(archives) != 1 {
		t.Fatal("original was not preserved")
	}
	original, _ := os.ReadFile(archives[0])
	if !bytes.Equal(original, broken) {
		t.Fatal("original bytes changed")
	}
	got, err := blocked.Get(c.ID)
	if err != nil || got.Password != "do-not-persist" || blocked.Health().Status != "ready" {
		t.Fatal("recovered workspace unusable", err)
	}
	if err = blocked.RestoreBackup(); err == nil {
		t.Fatal("recovery can replace a healthy workspace")
	}
}

func TestWorkspaceValidationNeverExposesPlaintextOrOverwritesFutureFormats(t *testing.T) {
	for name, change := range map[string]func(map[string]any){
		"future":    func(m map[string]any) { m["version"] = 99 },
		"null":      func(m map[string]any) { m["connections"] = nil },
		"duplicate": func(m map[string]any) { cs := m["connections"].([]any); m["connections"] = append(cs, cs[0]) },
		"password": func(m map[string]any) {
			m["connections"].([]any)[0].(map[string]any)["password"] = "plaintext-sentinel"
		},
		"apikey":  func(m map[string]any) { m["ai"].(map[string]any)["apiKey"] = "plaintext-sentinel" },
		"unknown": func(m map[string]any) { m["unexpected"] = "preserve-me" },
	} {
		t.Run(name, func(t *testing.T) {
			s, v := testStore(t)
			s.Save(testConnection())
			b, _ := os.ReadFile(s.path)
			var obj map[string]any
			json.Unmarshal(b, &obj)
			change(obj)
			modified, _ := json.Marshal(obj)
			os.WriteFile(s.path, modified, 0600)
			blocked, err := NewStore(filepath.Dir(s.path), v)
			if err == nil {
				t.Fatal("invalid metadata accepted")
			}
			if strings.Contains(err.Error(), "plaintext-sentinel") || len(blocked.List()) != 0 || blocked.AI().APIKey != "" {
				t.Fatal("invalid metadata leaked")
			}
			if name == "future" {
				if blocked.Health().CanRestore || blocked.Health().Status != "newer_version" || blocked.RestoreBackup() == nil {
					t.Fatal("future format may be downgraded")
				}
			}
			after, _ := os.ReadFile(s.path)
			if !bytes.Equal(modified, after) {
				t.Fatal("invalid original overwritten")
			}
		})
	}
}

func TestMissingConfigRequiresExplicitBackupRecoveryAndRevalidatesOnClick(t *testing.T) {
	s, v := testStore(t)
	s.Save(testConnection())
	valid, _ := os.ReadFile(s.path)
	os.Remove(s.path)
	blocked, err := NewStore(filepath.Dir(s.path), v)
	if err == nil || !blocked.Health().CanRestore || blocked.Health().Status != "missing" {
		t.Fatal("missing primary silently reset")
	}
	// Another process repairs the primary after the health dialog was opened.
	os.WriteFile(s.path, valid, 0600)
	if err = blocked.RestoreBackup(); err == nil {
		t.Fatal("overwrote a repaired workspace")
	}
	if err = blocked.RetryLoad(); err != nil || len(blocked.List()) != 1 {
		t.Fatal("retry did not reload", err)
	}
	os.WriteFile(s.path, []byte("broken"), 0600)
	blocked, _ = NewStore(filepath.Dir(s.path), v)
	os.WriteFile(s.backupPath(), []byte("broken backup"), 0600)
	if blocked.Health().CanRestore || blocked.RestoreBackup() == nil {
		t.Fatal("used an invalid backup")
	}
	stillBroken, _ := os.ReadFile(s.path)
	if string(stillBroken) != "broken" {
		t.Fatal("failed restore changed original")
	}
}

func TestBackupFailureDoesNotRollBackCommittedSecrets(t *testing.T) {
	s, v := testStore(t)
	c, _ := s.Save(testConnection())
	os.Remove(s.backupPath())
	os.Mkdir(s.backupPath(), 0700)
	c.Name, c.Password = "updated", "new-password"
	if _, err := s.Save(c); err != nil {
		t.Fatal("backup error incorrectly failed primary commit", err)
	}
	if v.data[c.ID] != "new-password" || !s.Health().BackupWarning {
		t.Fatal("committed credentials rolled back or warning missing")
	}
	os.Remove(s.backupPath())
	reopened, err := NewStore(filepath.Dir(s.path), v)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Get(c.ID)
	if err != nil || got.Name != "updated" || got.Password != "new-password" || !reopened.Health().BackupAvailable {
		t.Fatal("primary and keychain disagree", err)
	}
}

func TestConcurrentProfileUpdatesKeepConnectionAndSecretPaired(t *testing.T) {
	s, _ := testStore(t)
	input := testConnection()
	input.Name, input.Password = "first", "first"
	c, _ := s.Save(input)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			c.Name, c.Password = []string{"first", "second"}[i%2], []string{"first", "second"}[i%2]
			if _, err := s.Save(c); err != nil {
				t.Error(err)
			}
		}
	}()
	id := c.ID
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			got, err := s.Get(id)
			if err != nil || got.Name != got.Password {
				t.Error("profile and credential came from different revisions", err)
			}
		}
	}()
	wg.Wait()
}
