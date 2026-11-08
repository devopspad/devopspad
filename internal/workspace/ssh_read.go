package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		return 0, errors.New("remote output exceeds the size limit")
	}
	return b.Buffer.Write(p)
}

// SSHOutput runs one read-only command on the connection's SSH host, using the
// same authentication and host-key verification as tunnels, and returns at
// most limit bytes of stdout. Callers must pass constant commands; never build
// command from user or archive input (use SSHReadFile for paths).
func SSHOutput(ctx context.Context, c Connection, command string, limit int) ([]byte, error) {
	if !c.SSH.Enabled {
		return nil, errors.New("SSH is not configured")
	}
	c, closeTransport, err := prepareTransport(ctx, c)
	if err != nil {
		return nil, err
	}
	defer closeTransport()
	session, err := c.tunnel.client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()
	stop := context.AfterFunc(ctx, func() { session.Close() })
	defer stop()
	out := &limitedBuffer{limit: limit}
	var errOut limitedBuffer
	errOut.limit = 4 << 10
	session.Stdout, session.Stderr = out, &errOut
	if err := session.Run(command); err != nil {
		msg := strings.TrimSpace(errOut.String())
		if msg == "" {
			msg = err.Error()
		}
		return out.Bytes(), fmt.Errorf("remote command failed: %s", firstLine(msg))
	}
	return out.Bytes(), nil
}

// SSHReadFile reads one absolute path on the SSH host (at most limit bytes).
func SSHReadFile(ctx context.Context, c Connection, path string, limit int) ([]byte, error) {
	if !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "\x00\n") {
		return nil, errors.New("remote path must be absolute")
	}
	b, err := SSHOutput(ctx, c, "cat -- '"+strings.ReplaceAll(path, "'", `'\''`)+"'", limit+1)
	if err != nil {
		return nil, err
	}
	if len(b) > limit {
		return nil, fmt.Errorf("%s exceeds %d KiB", path, limit>>10)
	}
	return b, nil
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

var _ io.Writer = (*limitedBuffer)(nil)
