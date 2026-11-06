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
			return "", fmt.Errorf("Git %s 失败，请在项目目录检查 Git 状态或远端认证（%v）", args[0], err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	top, err := run("rev-parse", "--show-toplevel")
	if err != nil {
		return "", errors.New("此目录不是 Git 仓库；请先克隆共享仓库，再选择其根目录")
	}
	// Prevent accidentally staging another application's parent repository.
	if top != s.Root {
		return "", errors.New("请选择 Git 仓库根目录作为项目目录")
	}
	switch action {
	case "status":
		return run("status", "--short", "--branch", "--", ".", ":!.infradex-write.lock")
	case "pull":
		status, err := run("status", "--porcelain", "--", ".", ":!.infradex-write.lock")
		if err != nil {
			return "", err
		}
		if status != "" {
			return "", errors.New("有未提交改动，请先提交后再拉取")
		}
		if _, err = run("pull", "--ff-only"); err != nil {
			return "", errors.New("无法快进同步。请在项目目录执行 git fetch，检查并合并分叉后重试；本地改动未被重置")
		}
		if _, err = s.read(); err != nil {
			return "", fmt.Errorf("已拉取，但项目校验失败: %w", err)
		}
		return "已拉取共享项目", nil
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
			args := append([]string{"commit", "-m", "Update Infradex project", "--only", "--"}, files...)
			if _, err = run(args...); err != nil {
				return "", err
			}
		}
		if _, err = run("push"); err != nil {
			return "", errors.New("本地提交已保留，但推送失败。请检查认证，或合并远端改动后重试")
		}
		return "已推送共享项目", nil
	default:
		return "", errors.New("不支持的 Git 操作")
	}
}
