package ops

import (
	"gopkg.in/yaml.v3"
	"strings"
	"testing"
)

func TestNginxProjectScopeAndReattach(t *testing.T) {
	s := runTestStore(t)
	snapshot, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	p := snapshot.Project
	p.Applications[0].NginxPath = "/etc/nginx/conf.d/app.conf"
	data, err := yaml.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = s.Save(string(data), snapshot.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Attach(p.Environments[0], Application{Name: p.Applications[0].Name, Selector: p.Applications[0].Selector}, snapshot.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err = Validate(strings.Replace(string(data), "/etc/nginx/conf.d/app.conf", "/etc/passwd", 1)); err == nil {
		t.Fatal("accepted outside Nginx path")
	}
}
