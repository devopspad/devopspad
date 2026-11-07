package workspace

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"
)

//go:embed nginx_remote.py
var nginxRemoteScript string

type NginxConfig struct {
	Path     string `json:"path"`
	Text     string `json:"text"`
	Revision string `json:"revision"`
	Message  string `json:"message,omitempty"`
	Error    string `json:"error,omitempty"`
}

type NginxUpdate struct {
	Revision string `json:"revision"`
	Text     string `json:"text"`
}

var nginxPathPattern = regexp.MustCompile(`^/etc/nginx/conf\.d/[a-zA-Z0-9][a-zA-Z0-9._-]*\.conf$`)
var nginxRevisionPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// Nginx operates on the selected environment's existing, verified SSH host.
// Configuration travels over stdin, never in command arguments or local logs.
func ReadNginx(ctx context.Context, c Connection, path string) (NginxConfig, error) {
	return nginxOperation(ctx, c, path, nil)
}

func SaveNginx(ctx context.Context, c Connection, path string, update NginxUpdate) (NginxConfig, error) {
	if !nginxRevisionPattern.MatchString(update.Revision) || len(update.Text) > 1<<20 || !utf8.ValidString(update.Text) || strings.ContainsRune(update.Text, 0) {
		return NginxConfig{}, errors.New("Invalid Nginx config or revision; keep your edits, refresh and try again")
	}
	return nginxOperation(ctx, c, path, &update)
}

type nginxOutput struct{ bytes.Buffer }

func (b *nginxOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 8<<20 {
		return 0, errors.New("The Nginx response exceeds the size limit")
	}
	return b.Buffer.Write(p)
}

func nginxOperation(ctx context.Context, c Connection, path string, update *NginxUpdate) (NginxConfig, error) {
	var result NginxConfig
	if len(path) > 240 || !nginxPathPattern.MatchString(path) {
		return result, errors.New("Only .conf files under /etc/nginx/conf.d/ are supported")
	}
	if !c.SSH.Enabled {
		return result, errors.New("Host Nginx needs SSH configured on the selected environment's cluster connection")
	}
	c, closeTransport, err := prepareTransport(ctx, c)
	if err != nil {
		return result, err
	}
	defer closeTransport()
	session, err := c.tunnel.client.NewSession()
	if err != nil {
		return result, err
	}
	defer session.Close()
	stop := context.AfterFunc(ctx, func() { session.Close() })
	defer stop()
	payload, _ := json.Marshal(struct {
		Path   string       `json:"path"`
		Update *NginxUpdate `json:"update,omitempty"`
	}{path, update})
	session.Stdin = bytes.NewReader(payload)
	var out nginxOutput
	session.Stdout = &out
	session.Stderr = io.Discard // Never return diagnostics containing configuration values.
	command := "python3 -c '" + strings.ReplaceAll(nginxRemoteScript, "'", "'\"'\"'") + "'"
	if err = session.Run(command); err != nil {
		return result, errors.New("The Nginx operation was not confirmed; refresh to check. The host needs Python 3 and access to the config file")
	}
	if err = json.Unmarshal(out.Bytes(), &result); err != nil {
		return result, errors.New("Cannot read the remote Nginx result; refresh to check")
	}
	if result.Error != "" {
		return result, errors.New(result.Error)
	}
	if result.Path != path || !nginxRevisionPattern.MatchString(result.Revision) {
		return result, errors.New("Invalid remote Nginx file identity")
	}
	return result, nil
}
