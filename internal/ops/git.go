package ops

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Git operates only on an explicit local checkout. Pull never resets local work
// or resolves conflicts automatically. Remote authentication is owned by Git.
func (s *Store) Git(ctx context.Context, action string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, lockErr := s.projectLock()
	if lockErr != nil {
		return "", lockErr
	}
	defer unlock()
	run := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", s.Root}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never")
		b, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("Git %s failed; check the Git status or remote authentication in the project directory (%v)", args[0], err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	top, err := run("rev-parse", "--show-toplevel")
	if err != nil {
		return "", errors.New("This directory is not a Git repository; clone the shared repository first, then choose its root")
	}
	// Prevent accidentally staging another application's parent repository.
	if top != s.Root {
		return "", errors.New("Choose the Git repository root as the project directory")
	}
	switch action {
	case "status":
		return run("status", "--short", "--branch", "--", ".", ":!.devopspad-write.lock")
	case "pull":
		status, err := run("status", "--porcelain", "--", ".", ":!.devopspad-write.lock")
		if err != nil {
			return "", err
		}
		if status != "" {
			return "", errors.New("There are uncommitted changes; commit them before pulling")
		}
		if _, err = run("pull", "--ff-only"); err != nil {
			return "", errors.New("Cannot fast-forward. Run git fetch in the project directory, merge the divergence and try again; local changes were not reset")
		}
		if _, err = s.read(); err != nil {
			return "", fmt.Errorf("Pulled, but the project failed validation: %w", err)
		}
		return "Pulled the shared project", nil
	case "push":
		snapshot, readErr := s.read()
		if readErr != nil {
			return "", readErr
		}
		paths := []string{"project.yaml"}
		for _, artifact := range snapshot.Artifacts {
			if artifact.RunID == "" {
				paths = append(paths, "artifacts/"+artifact.ID+".yaml")
			}
		}
		for _, record := range snapshot.Releases {
			paths = append(paths, "releases/"+record.ID+".json")
		}
		for _, record := range snapshot.Runs {
			paths = append(paths, "runs/"+record.ID+".json")
		}
		deleted, err := run("ls-files", "--deleted", "-z", "--", "artifacts", "releases", "runs")
		if err != nil {
			return "", err
		}
		for _, path := range strings.Split(strings.TrimSuffix(deleted, "\x00"), "\x00") {
			dir := filepath.ToSlash(filepath.Dir(path))
			ext := filepath.Ext(path)
			_, idErr := uuid.Parse(strings.TrimSuffix(filepath.Base(path), ext))
			if idErr == nil && ((dir == "artifacts" && ext == ".yaml") || ((dir == "releases" || dir == "runs") && ext == ".json")) {
				paths = append(paths, path)
			}
		}
		if _, err = run(append([]string{"add", "--"}, paths...)...); err != nil {
			return "", err
		}
		changed, err := run(append([]string{"diff", "--cached", "--name-only", "-z", "--"}, paths...)...)
		if err != nil {
			return "", err
		}
		if changed != "" {
			files := strings.Split(strings.TrimSuffix(changed, "\x00"), "\x00")
			args := append([]string{"commit", "-m", "Update DevOpsPad project", "--only", "--"}, files...)
			if _, err = run(args...); err != nil {
				return "", err
			}
		}
		if _, err = run("push"); err != nil {
			return "", errors.New("The local commit is kept, but the push failed. Check authentication, or merge remote changes and try again")
		}
		return "Pushed the shared project", nil
	default:
		return "", errors.New("Unsupported Git operation")
	}
}
