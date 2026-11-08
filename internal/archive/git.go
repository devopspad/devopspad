package archive

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// managedPaths are the only paths archive commit stages.
// Observed snapshots stay local in .devopspad/observed and are never committed.
var managedPaths = []string{RootFile, "AGENTS.md", "INDEX.md", ".gitignore", ".claude", "inventory", "products", "notes", "records"}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never")
	b, err := cmd.CombinedOutput()
	out := strings.TrimRight(string(b), "\n")
	if err != nil {
		return out, fmt.Errorf("git %s: %s", args[0], firstLine(out, err.Error()))
	}
	return out, nil
}

func firstLine(s, fallback string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return fallback
	}
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// repoRoot fails unless dir is the top level of a Git repository.
func repoRoot(dir string) error {
	top, err := git(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("%s is not a Git repository; run git init or devopspadcli archive init", dir)
	}
	a, _ := filepath.EvalSymlinks(top)
	b, _ := filepath.EvalSymlinks(dir)
	if a != b {
		return fmt.Errorf("the archive must be the root of its own Git repository (found parent repository %s)", top)
	}
	return nil
}

// Check loads the archive and adds history checks. staged validates the Git
// index (what the next commit will contain) instead of the working tree.
func Check(dir string, staged bool) (*Archive, error) {
	inRepo := repoRoot(dir) == nil
	if !staged {
		a, err := Load(dir)
		if err == nil && inRepo {
			a.checkRecordHistory(dir, false)
			sortIssues(a.Errors)
		}
		return a, err
	}
	if !inRepo {
		return nil, repoRoot(dir)
	}
	tmp, err := os.MkdirTemp("", "devopspad-staged-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	if _, err := git(dir, "checkout-index", "-a", "-f", "--prefix="+tmp+string(filepath.Separator)); err != nil {
		return nil, err
	}
	a, err := Load(tmp)
	if err != nil {
		return nil, err
	}
	a.Dir = dir
	a.checkRecordHistory(dir, true)
	sortIssues(a.Errors)
	return a, nil
}

// Revision returns the short HEAD commit, or "" before the first commit.
func Revision(dir string) string {
	out, err := git(dir, "rev-parse", "--short", "HEAD")
	if err != nil {
		return ""
	}
	return out
}

// GitUser returns user.name for human actors.
func GitUser(dir string) string {
	out, _ := git(dir, "config", "user.name")
	return strings.TrimSpace(out)
}

type Status struct {
	Dir      string   `json:"dir"`
	Branch   string   `json:"branch"`
	Upstream string   `json:"upstream,omitempty"`
	Ahead    int      `json:"ahead"`
	Behind   int      `json:"behind"`
	Fetched  bool     `json:"fetched"`
	Changes  []string `json:"changes"`
	Errors   int      `json:"errors"`
	Warnings int      `json:"warnings"`
}

// GitStatus summarizes the repository. fetch contacts the remote first.
func GitStatus(dir string, fetch bool) (Status, error) {
	s := Status{Dir: dir, Changes: []string{}}
	if err := repoRoot(dir); err != nil {
		return s, err
	}
	if fetch {
		if _, err := git(dir, "fetch", "--quiet"); err != nil {
			return s, fmt.Errorf("fetch failed; check the remote and its authentication (%v)", err)
		}
		s.Fetched = true
	}
	out, err := git(dir, "status", "--porcelain=v1", "--branch")
	if err != nil {
		return s, err
	}
	for i, line := range strings.Split(out, "\n") {
		if i == 0 {
			s.Branch, s.Upstream, s.Ahead, s.Behind = parseBranch(strings.TrimPrefix(line, "## "))
		} else if line != "" {
			s.Changes = append(s.Changes, line)
		}
	}
	return s, nil
}

var aheadRE = regexp.MustCompile(`(ahead|behind) (\d+)`)

func parseBranch(line string) (branch, upstream string, ahead, behind int) {
	line = strings.TrimPrefix(line, "No commits yet on ")
	head, rest, _ := strings.Cut(line, " ")
	branch, upstream, _ = strings.Cut(head, "...")
	for _, m := range aheadRE.FindAllStringSubmatch(rest, -1) {
		n, _ := strconv.Atoi(m[2])
		if m[1] == "ahead" {
			ahead = n
		} else {
			behind = n
		}
	}
	return
}

var commitRE = regexp.MustCompile(`^(archive|config|record|scan|note|migrate|skill)(\([^)\s]+\))?: \S`)

type CommitInput struct {
	Message string
	Actor   string // human/<name> or ai/<name>
	Session string
}

// Commit validates, regenerates INDEX.md and AGENTS.md, stages the archive
// paths and commits with DevOpsPad trailers. It never pushes.
func Commit(dir string, in CommitInput) (string, error) {
	if err := repoRoot(dir); err != nil {
		return "", err
	}
	subject, _, _ := strings.Cut(in.Message, "\n")
	if !commitRE.MatchString(subject) {
		return "", errors.New(`-m must start with <type>(<scope>): <summary>; types: archive, config, record, scan, note, migrate, skill (e.g. "archive(domain:foo.app): register domain")`)
	}
	if !actorRE.MatchString(in.Actor) {
		return "", errors.New("--actor must be human/<name> or ai/<name>")
	}
	a, err := Load(dir)
	if err != nil {
		return "", err
	}
	if err := WriteGenerated(a); err != nil {
		return "", err
	}
	if a, err = Check(dir, false); err != nil {
		return "", err
	}
	if len(a.Errors) > 0 {
		return "", fmt.Errorf("the archive has %d validation errors; run devopspadcli archive validate", len(a.Errors))
	}
	var paths []string
	for _, p := range managedPaths {
		_, statErr := os.Lstat(filepath.Join(dir, p))
		tracked, _ := git(dir, "ls-files", "--", p)
		if statErr == nil || tracked != "" {
			paths = append(paths, p)
		}
	}
	if _, err := git(dir, append([]string{"add", "-A", "--"}, paths...)...); err != nil {
		return "", err
	}
	staged, err := git(dir, "diff", "--cached", "--name-only")
	if err != nil {
		return "", err
	}
	if staged == "" {
		return "", errors.New("nothing to commit")
	}
	var other []string
	for _, f := range strings.Split(staged, "\n") {
		if !underManaged(f) {
			other = append(other, f)
		}
	}
	if len(other) > 0 {
		sort.Strings(other)
		return "", fmt.Errorf("other files are staged (%s); commit or unstage them with git first", strings.Join(other, ", "))
	}
	msg := strings.TrimSpace(in.Message) + "\n\nDevOpsPad-Actor: " + in.Actor + "\n"
	if in.Session != "" {
		msg += "DevOpsPad-Session: " + in.Session + "\n"
	}
	cmd := exec.Command("git", "-C", dir, "commit", "-q", "-F", "-")
	cmd.Stdin = strings.NewReader(msg)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git commit failed: %s", strings.TrimSpace(string(out)))
	}
	return Revision(dir), nil
}

func underManaged(f string) bool {
	for _, p := range managedPaths {
		if f == p || strings.HasPrefix(f, p+"/") {
			return true
		}
	}
	return false
}

// Pull fast-forwards a clean archive and reports the new validation state.
func Pull(dir string) (*Archive, error) {
	s, err := GitStatus(dir, false)
	if err != nil {
		return nil, err
	}
	if len(s.Changes) > 0 {
		return nil, errors.New("there are uncommitted changes; commit them (devopspadcli archive commit) before pulling")
	}
	if _, err := git(dir, "pull", "--ff-only", "--quiet"); err != nil {
		return nil, fmt.Errorf("cannot fast-forward; fetch and merge in a terminal, local work was not changed (%v)", err)
	}
	return Load(dir)
}

func Push(dir string) error {
	if err := repoRoot(dir); err != nil {
		return err
	}
	if _, err := git(dir, "push", "--quiet"); err != nil {
		return fmt.Errorf("push failed; local commits are kept. Set an upstream with git push -u origin <branch>, or pull first (%v)", err)
	}
	return nil
}

const hookMarker = "# devopspadcli archive hook"

// InstallHook adds a pre-commit hook that validates the staged archive.
// An existing foreign hook is left alone.
func InstallHook(dir string) (string, error) {
	p, err := git(dir, "rev-parse", "--git-path", "hooks/pre-commit")
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	if b, err := os.ReadFile(p); err == nil && !strings.Contains(string(b), hookMarker) {
		return "", fmt.Errorf("%s already exists; add `devopspadcli archive validate --staged` to it yourself", p)
	}
	exe, _ := os.Executable()
	script := "#!/bin/sh\n" + hookMarker + "\n" +
		"# Rejects commits whose staged archive fails validation (including literal secrets).\n" +
		"cli=$(command -v devopspadcli || echo " + shellQuote(exe) + ")\n" +
		"exec \"$cli\" archive validate --staged --archive \"$(git rev-parse --show-toplevel)\"\n"
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	return p, os.WriteFile(p, []byte(script), 0o755)
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
