package fleet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFleetRejectsMissingConnection(t *testing.T) {
	dir := t.TempDir()
	raw := []byte(`{"fleets":[{"name":"hk","machines":[{"name":"hk-1","connectionId":"missing","roles":["cluster"]}]}]}`)
	if err := os.WriteFile(filepath.Join(dir, "fleet.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir, func(id string) bool { return false })
	if err == nil || !strings.Contains(err.Error(), "hk-1") {
		t.Fatalf("%v", err)
	}
}

func TestFleetAcceptsClusterAndEdgeRoles(t *testing.T) {
	dir := t.TempDir()
	f := File{Fleets: []Fleet{{
		Name: "hk",
		Machines: []Machine{
			{Name: "hk-1", ConnectionID: "c1", Roles: []string{"cluster", "edge"}},
			{Name: "edge-2", ConnectionID: "c2", Roles: []string{"edge"}},
		},
	}}}
	if err := Save(dir, f); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "fleet.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	got, err := Load(dir, func(id string) bool { return id == "c1" || id == "c2" })
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Fleets) != 1 || len(got.Fleets[0].Machines) != 2 {
		t.Fatalf("%+v", got)
	}
}

func TestFleetRejectsInvalidShape(t *testing.T) {
	dir := t.TempDir()
	base := Machine{Name: "hk-1", ConnectionID: "c1", Roles: []string{"cluster"}}
	machines := make([]Machine, 33)
	for i := range machines {
		machines[i] = Machine{Name: "m" + string(rune('a'+i%26)) + string(rune('0'+i/26)), ConnectionID: "c1", Roles: []string{"cluster"}}
	}
	cases := []File{
		{Fleets: []Fleet{{Name: "hk", Machines: []Machine{{Name: "hk-1", ConnectionID: "c1"}}}}},
		{Fleets: []Fleet{{Name: "hk", Machines: machines}}},
		{Fleets: []Fleet{{Name: "hk", Machines: []Machine{base, {Name: "hk-1", ConnectionID: "c2", Roles: []string{"edge"}}}}}},
	}
	for _, f := range cases {
		if err := Save(dir, f); err == nil {
			t.Fatalf("saved %+v", f)
		}
	}
}

func TestFleetCorruptJSONDoesNotReplaceLastGoodFile(t *testing.T) {
	dir := t.TempDir()
	good := File{Fleets: []Fleet{{Name: "hk", Machines: []Machine{{Name: "hk-1", ConnectionID: "c1", Roles: []string{"cluster"}}}}}}
	if err := Save(dir, good); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "fleet.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir, func(string) bool { return true }); err == nil {
		t.Fatal("accepted corrupt fleet")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != "{" {
		t.Fatalf("load rewrote corrupt file: %s", after)
	}
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	bad := good
	bad.Fleets[0].Machines[0].Roles = nil
	if err := Save(dir, bad); err == nil {
		t.Fatal("saved invalid fleet")
	}
	still, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(still) != string(before) {
		t.Fatalf("invalid save replaced the last good file")
	}
}

func TestFleetMissingFileIsEmpty(t *testing.T) {
	got, err := Load(t.TempDir(), func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Fleets) != 0 {
		t.Fatalf("%+v", got)
	}
}
