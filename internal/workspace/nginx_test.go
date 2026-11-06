package workspace

import (
	"context"
	"strings"
	"testing"
)

func TestNginxRejectsInvalidScopeBeforeSSH(t *testing.T) {
	for _, path := range []string{"/etc/passwd", "/etc/nginx/conf.d/../nginx.conf", "/etc/nginx/conf.d/a.conf;id", "/etc/nginx/conf.d/a.conf\n"} {
		if _, err := ReadNginx(context.Background(), Connection{}, path); err == nil {
			t.Fatal("accepted", path)
		}
	}
	if _, err := ReadNginx(context.Background(), Connection{}, "/etc/nginx/conf.d/app.conf"); err == nil || !strings.Contains(err.Error(), "SSH") {
		t.Fatal("missing SSH was not rejected", err)
	}
	if _, err := SaveNginx(context.Background(), Connection{}, "/etc/nginx/conf.d/app.conf", NginxUpdate{Revision: "invalid", Text: "x"}); err == nil {
		t.Fatal("accepted missing version")
	}
}
