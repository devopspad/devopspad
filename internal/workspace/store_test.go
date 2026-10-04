package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type memoryVault struct {
	data map[string]string
	fail bool
}

func (v *memoryVault) Set(k, s string) error {
	if v.fail {
		return errors.New("vault unavailable")
	}
	v.data[k] = s
	return nil
}
func (v *memoryVault) Get(k string) (string, error) {
	if s, ok := v.data[k]; ok {
		return s, nil
	}
	return "", errors.New("not found")
}
func (v *memoryVault) Delete(k string) error {
	if v.fail {
		return errors.New("vault unavailable")
	}
	delete(v.data, k)
	return nil
}
func testStore(t *testing.T) (*Store, *memoryVault) {
	t.Helper()
	v := &memoryVault{data: map[string]string{}}
	s, e := NewStore(t.TempDir(), v)
	if e != nil {
		t.Fatal(e)
	}
	return s, v
}
func testConnection() Connection {
	return Connection{Name: "Local", Kind: "mysql", Host: "localhost", Port: 3306, Password: "do-not-persist", Database: "test"}
}
func TestSecretsNeverPersistInMetadata(t *testing.T) {
	s, v := testStore(t)
	c, e := s.Save(testConnection())
	if e != nil {
		t.Fatal(e)
	}
	if c.Password != "" || !c.HasPassword {
		t.Fatal("returned secret or missing flag")
	}
	b, _ := os.ReadFile(s.path)
	if strings.Contains(string(b), "do-not-persist") {
		t.Fatal("secret persisted in plaintext")
	}
	if v.data[c.ID] != "do-not-persist" {
		t.Fatal("secret missing from vault")
	}
	reopened, e := NewStore(filepath.Dir(s.path), v)
	if e != nil {
		t.Fatal(e)
	}
	live, e := reopened.Get(c.ID)
	if e != nil || live.Password != "do-not-persist" {
		t.Fatalf("resolve: %v", e)
	}
	c.Name = "Renamed"
	c, e = s.Save(c)
	if e != nil {
		t.Fatal(e)
	}
	live, _ = s.Get(c.ID)
	if live.Password != "do-not-persist" {
		t.Fatal("blank update must preserve secret")
	}
	if e = s.Delete(c.ID); e != nil {
		t.Fatal(e)
	}
	if len(s.List()) != 0 || len(v.data) != 0 {
		t.Fatal("deletion left profile or credentials")
	}
}
func TestFailedWritesRollBackSecrets(t *testing.T) {
	s, v := testStore(t)
	c, e := s.Save(testConnection())
	if e != nil {
		t.Fatal(e)
	}
	s.path = filepath.Join(t.TempDir(), "missing", "workspace.json")
	c.Password = "replacement"
	c.Name = "Changed"
	if _, e = s.Save(c); e == nil {
		t.Fatal("expected disk failure")
	}
	if v.data[c.ID] != "do-not-persist" || s.List()[0].Name != "Local" {
		t.Fatal("update was not rolled back")
	}
	if e = s.Delete(c.ID); e == nil {
		t.Fatal("expected deletion failure")
	}
	if v.data[c.ID] != "do-not-persist" || len(s.List()) != 1 {
		t.Fatal("delete was not rolled back")
	}
}
func TestVaultFailureDoesNotSaveConnection(t *testing.T) {
	s, v := testStore(t)
	v.fail = true
	if _, e := s.Save(testConnection()); e == nil {
		t.Fatal("expected vault error")
	}
	if len(s.List()) != 0 {
		t.Fatal("saved partial profile")
	}
}
func TestChangingAIEndpointDoesNotReuseKey(t *testing.T) {
	s, v := testStore(t)
	if e := s.SaveAI(AISettings{Endpoint: "https://one.example/v1", Model: "one", APIKey: "provider-one-key"}); e != nil {
		t.Fatal(e)
	}
	if e := s.SaveAI(AISettings{Endpoint: "https://two.example/v1", Model: "two"}); e != nil {
		t.Fatal(e)
	}
	if s.AI().HasKey || v.data["ai-api-key"] != "" {
		t.Fatal("reused a key across providers")
	}
}
func TestValidation(t *testing.T) {
	for _, kind := range []string{"mysql", "redis", "mongo", "kafka", "clickhouse", "k8s", "k3s"} {
		c := testConnection()
		c.Kind = kind
		c.Database = "0"
		c.Kubeconfig = "/tmp/config"
		if e := Validate(c); e != nil {
			t.Fatalf("%s: %v", kind, e)
		}
	}
	for _, database := range []string{"-1", "1x", "1.2"} {
		c := testConnection()
		c.Kind = "redis"
		c.Database = database
		if Validate(c) == nil {
			t.Fatalf("accepted invalid redis database %s", database)
		}
	}
	c := testConnection()
	c.Port = 0
	if Validate(c) == nil {
		t.Fatal("accepted invalid port")
	}
	c = testConnection()
	c.Kind = "unsupported"
	if Validate(c) == nil {
		t.Fatal("accepted unknown engine")
	}
}

func TestSSHSecretsPersistenceRollbackAndRemoval(t *testing.T) {
	s, v := testStore(t)
	input := testConnection()
	input.SSH = SSHConfig{Enabled: true, Host: "bastion", Port: 22, Username: "user", AuthMethod: "password", Password: "ssh-password-secret"}
	c, e := s.Save(input)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := os.ReadFile(s.path)
	if c.SSH.Password != "" || !c.SSH.HasPassword || strings.Contains(string(raw), "ssh-password-secret") {
		t.Fatal("SSH secret exposed")
	}
	c.Password = "new-database-password"
	resolved, e := s.Resolve(c)
	if e != nil || resolved.SSH.Password != "ssh-password-secret" {
		t.Fatal("database password must not bypass SSH resolution", e)
	}
	c.Password = ""
	path := s.path
	s.path = filepath.Join(t.TempDir(), "missing", "state.json")
	replacement := c
	replacement.SSH.AuthMethod = "key"
	replacement.SSH.PrivateKeyPath = "/key"
	replacement.SSH.Passphrase = "key-passphrase-secret"
	replacement.Password = "replacement"
	if _, e = s.Save(replacement); e == nil {
		t.Fatal("expected persist failure")
	}
	if v.data[c.ID] != input.Password || v.data[c.ID+":ssh-password"] != "ssh-password-secret" || v.data[c.ID+":ssh-passphrase"] != "" {
		t.Fatal("multi-secret rollback failed")
	}
	if e = s.Delete(c.ID); e == nil || v.data[c.ID+":ssh-password"] != "ssh-password-secret" {
		t.Fatal("failed delete lost SSH password")
	}
	s.path = path
	c, e = s.Save(replacement)
	if e != nil {
		t.Fatal(e)
	}
	if c.SSH.HasPassword || !c.SSH.HasPassphrase || v.data[c.ID+":ssh-password"] != "" {
		t.Fatal("unused authentication secret retained")
	}
	c, e = s.Save(c)
	if e != nil {
		t.Fatal(e)
	}
	reopened, e := NewStore(filepath.Dir(path), v)
	if e != nil {
		t.Fatal(e)
	}
	resolved, e = reopened.Get(c.ID)
	if e != nil || resolved.SSH.Passphrase != "key-passphrase-secret" {
		t.Fatal("passphrase not preserved", e)
	}
	raw, _ = os.ReadFile(path)
	if strings.Contains(string(raw), "key-passphrase-secret") || c.SSH.Passphrase != "" {
		t.Fatal("passphrase exposed")
	}
	c.SSH.Enabled = false
	c, e = s.Save(c)
	if e != nil {
		t.Fatal(e)
	}
	if c.SSH.HasPassphrase || v.data[c.ID+":ssh-passphrase"] != "" {
		t.Fatal("disabled SSH retained secret")
	}
	if e = s.Delete(c.ID); e != nil || len(v.data) != 0 {
		t.Fatal("delete left credentials", e)
	}
}
