package archive

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/zalando/go-keyring"
	"gopkg.in/yaml.v3"
)

// KeychainService is the OS credential store service shared with the desktop app.
const KeychainService = "devopspad"

var (
	envNameRE     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	placeholderRE = regexp.MustCompile(`\$\{secret:([^}]*)\}`)
	literalKeyRE  = regexp.MustCompile(`(?i)^(password|passwd|pwd|passphrase|secret|secret[_-]?key|client[_-]?secret|token|api[_-]?token|access[_-]?token|api[_-]?key|access[_-]?key|private[_-]?key)$`)
	// Matches KEY = value where KEY ends in a secret-like word, including
	// prefixed names such as DB_PASSWORD or stripe-api-key.
	assignRE = regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_-])([A-Za-z0-9]+[_.-])*(password|passwd|pwd|passphrase|secret|token|api[_-]?key|access[_-]?key|secret[_-]?key|private[_-]?key|client[_-]?secret)["']?\s*[:=]\s*["']?([^\s"'#;,$<{][^\s"'#;,]{5,})`)
)

type secretPattern struct {
	name string
	re   *regexp.Regexp
}

// Content patterns that indicate a secret value. Matches never echo the value.
var secretPatterns = []secretPattern{
	{"private key", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
	{"kubeconfig client key", regexp.MustCompile(`client-key-data:\s*\S`)},
	{"token value", regexp.MustCompile(`^\s*token:\s*[A-Za-z0-9._-]{20,}`)},
	{"AWS access key", regexp.MustCompile(`\b(AKIA|ASIA)[0-9A-Z]{16}\b`)},
	{"GitHub token", regexp.MustCompile(`\b(gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{20,})`)},
	{"GitLab token", regexp.MustCompile(`\bglpat-[A-Za-z0-9_-]{20,}`)},
	{"API secret key", regexp.MustCompile(`\b(sk|rk)[-_](live|test|ant|proj)?[-_]?[A-Za-z0-9_-]{20,}`)},
	{"Slack token", regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}`)},
	{"JWT", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`)},
	{"URL with password", regexp.MustCompile(`[a-z][a-z0-9+.-]*://[^/\s:@'"]+:([^/\s@'"$<{][^/\s@'"]*)@`)},
	{"MySQL DSN with password", regexp.MustCompile(`[^\s:/'"]+:[^\s@'"$<{][^\s@'"]*@tcp\(`)},
}

const allowMarker = "devopspad:allow-secret"

// ParseSecretRef validates keychain:<account>, env:<NAME> or file:<path>.
func ParseSecretRef(s string) error {
	scheme, rest, ok := strings.Cut(s, ":")
	if !ok || rest == "" {
		return fmt.Errorf("%q is not a secret reference; use keychain:<account>, env:<NAME> or file:<path>", s)
	}
	switch scheme {
	case "keychain":
		if strings.ContainsAny(rest, " \t\r\n") {
			return fmt.Errorf("%q: keychain accounts cannot contain whitespace", s)
		}
	case "env":
		if !envNameRE.MatchString(rest) {
			return fmt.Errorf("%q: env references name an environment variable", s)
		}
	case "file":
		if !strings.HasPrefix(rest, "/") && !strings.HasPrefix(rest, "~/") {
			return fmt.Errorf("%q: file references are absolute paths or start with ~/", s)
		}
	default:
		return fmt.Errorf("%q: unsupported secret store %q; use keychain:, env: or file:", s, scheme)
	}
	return nil
}

// SecretStatus reports whether a reference resolves on this machine, without the value.
type SecretStatus struct {
	Ref    string   `json:"ref"`
	OK     bool     `json:"ok"`
	Reason string   `json:"reason,omitempty"`
	UsedBy []string `json:"usedBy"`
}

// keychainGet is replaced in tests.
var keychainGet = func(account string) (string, error) { return keyring.Get(KeychainService, account) }

// KeychainSet stores a secret for keychain:<account>.
var KeychainSet = func(account, value string) error { return keyring.Set(KeychainService, account, value) }

// CheckSecret resolves one reference and discards the value.
func CheckSecret(ref string) (bool, string) {
	if err := ParseSecretRef(ref); err != nil {
		return false, err.Error()
	}
	scheme, rest, _ := strings.Cut(ref, ":")
	switch scheme {
	case "keychain":
		v, err := keychainGet(rest)
		if errors.Is(err, keyring.ErrNotFound) || (err == nil && v == "") {
			return false, "not in the OS keychain; store it with devopspadcli secrets set " + ref
		} else if err != nil {
			return false, "keychain unavailable: " + err.Error()
		}
	case "env":
		if v, ok := os.LookupEnv(rest); !ok || v == "" {
			return false, "environment variable is not set"
		}
	case "file":
		p, err := expandHome(rest)
		if err != nil {
			return false, err.Error()
		}
		if info, err := os.Stat(p); err != nil || !info.Mode().IsRegular() {
			return false, "file does not exist"
		}
	}
	return true, ""
}

// errSecretMissing marks a reference that does not resolve on this machine.
var errSecretMissing = errors.New("secret not available on this machine")

// ResolveSecret returns the value of a reference on this machine. Callers
// must not store or print it.
func ResolveSecret(ref string) (string, error) {
	if err := ParseSecretRef(ref); err != nil {
		return "", err
	}
	scheme, rest, _ := strings.Cut(ref, ":")
	var v string
	switch scheme {
	case "keychain":
		val, err := keychainGet(rest)
		if err != nil {
			return "", fmt.Errorf("%w: %s is not in the OS keychain; store it with devopspadcli secrets set %s", errSecretMissing, ref, ref)
		}
		v = val
	case "env":
		v = os.Getenv(rest)
	case "file":
		p, err := expandHome(rest)
		if err != nil {
			return "", err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return "", fmt.Errorf("%w: %s: %v", errSecretMissing, ref, err)
		}
		v = string(b)
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return "", fmt.Errorf("%w: %s is empty or unset", errSecretMissing, ref)
	}
	return v, nil
}

func expandHome(p string) (string, error) {
	if !strings.HasPrefix(p, "~/") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, p[2:]), nil
}

// SecretReport groups every secretRef of the archive and checks each once.
func (a *Archive) SecretReport() []SecretStatus {
	byRef := map[string]*SecretStatus{}
	for _, u := range a.Secrets {
		s := byRef[u.Ref]
		if s == nil {
			s = &SecretStatus{Ref: u.Ref}
			s.OK, s.Reason = CheckSecret(u.Ref)
			byRef[u.Ref] = s
		}
		s.UsedBy = append(s.UsedBy, u.From+" "+u.Field)
	}
	out := make([]SecretStatus, 0, len(byRef))
	for _, s := range byRef {
		sort.Strings(s.UsedBy)
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ref < out[j].Ref })
	return out
}

// scanSecrets rejects literal credentials in every text file of the archive.
func (a *Archive) scanSecrets() {
	// The root file and files that failed to decode are scanned in decode.
	for _, r := range a.Resources {
		a.scanResourceKeys(r.File, r.node)
		a.scanText(r.File, r.raw, false)
	}
	files := make([]string, 0, len(a.textFiles))
	for f := range a.textFiles {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		config := strings.Contains(f, "/configs/")
		a.scanText(f, a.textFiles[f], config)
		if config {
			for _, m := range placeholderRE.FindAllStringSubmatch(string(a.textFiles[f]), -1) {
				if err := ParseSecretRef(m[1]); err != nil {
					a.errorf(f, 0, "", "secret.ref", "placeholder ${secret:%s}: %v", m[1], err)
				} else {
					a.Secrets = append(a.Secrets, SecretUse{From: f, Field: "${secret}", Ref: m[1], File: f})
				}
			}
		}
	}
	for _, rec := range a.Records {
		a.scanText(rec.file, rec.raw, false)
	}
}

// scanResourceKeys finds keys such as password: or token: holding a value
// instead of a reference.
func (a *Archive) scanResourceKeys(file string, node *yaml.Node) {
	var walk func(n *yaml.Node)
	walk = func(n *yaml.Node) {
		if n == nil {
			return
		}
		if n.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(n.Content); i += 2 {
				k, v := n.Content[i], n.Content[i+1]
				if literalKeyRE.MatchString(k.Value) && v.Kind == yaml.ScalarNode && v.Value != "" && ParseSecretRef(v.Value) != nil && !strings.Contains(v.LineComment, allowMarker) {
					a.errorf(file, k.Line, "", "secret.literal", "%s holds a literal value; store the secret with devopspadcli secrets set keychain:<account> and write a reference in a *Ref field or credentials[].ref", k.Value)
				}
				walk(v)
			}
			return
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(node)
}

func (a *Archive) scanText(file string, b []byte, config bool) {
	for i, line := range strings.Split(string(b), "\n") {
		if strings.Contains(line, allowMarker) {
			continue
		}
		for _, p := range secretPatterns {
			if p.re.MatchString(line) {
				a.errorf(file, i+1, "", "secret.pattern", "looks like a %s; remove it and use a reference (keychain:, env:, file:)%s. Append the comment %q to the line if this is a false positive", p.name, placeholderHint(config), allowMarker)
				break
			}
		}
		if config && assignRE.MatchString(placeholderRE.ReplaceAllString(line, "")) {
			a.errorf(file, i+1, "", "secret.literal", "assigns a literal secret; use a placeholder such as ${secret:keychain:<account>}. Append the comment %q to the line if this is a false positive", allowMarker)
		}
	}
}

func placeholderHint(config bool) string {
	if config {
		return " or a ${secret:<ref>} placeholder"
	}
	return ""
}
