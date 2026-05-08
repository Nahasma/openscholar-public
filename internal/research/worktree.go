package research

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Worktree 表示一个 git worktree 实验隔离环境
type Worktree struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Branch    string `json:"branch"`
	NodeID    string `json:"node_id"`
	Status    string `json:"status"` // active | kept | removed
	CreatedAt int64  `json:"created_at"`
}

// WorktreeManager 管理实验 worktrees
type WorktreeManager struct {
	repoRoot string // git repo root
	baseDir  string // .worktrees/ directory inside repoRoot
}

var worktreeNameRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,40}$`)

// NewWorktreeManager 创建 WorktreeManager，验证 git 可用且当前目录是 git 仓库
func NewWorktreeManager(repoRoot string) (*WorktreeManager, error) {
	// Verify git is available
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("git not found in PATH: %w", err)
	}

	// Verify repoRoot is a git repository
	cmd := exec.Command("git", "-C", repoRoot, "rev-parse", "--git-dir")
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("not a git repository (%s): %s", repoRoot, strings.TrimSpace(string(out)))
	}

	baseDir := filepath.Join(repoRoot, ".worktrees")
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("create worktrees dir: %w", err)
	}

	return &WorktreeManager{
		repoRoot: repoRoot,
		baseDir:  baseDir,
	}, nil
}

// Create 创建一个新的 worktree 及对应分支
func (m *WorktreeManager) Create(name, baseRef string) (*Worktree, error) {
	if !worktreeNameRe.MatchString(name) {
		return nil, fmt.Errorf("invalid worktree name %q: must match ^[A-Za-z0-9._-]{1,40}$", name)
	}

	// Check for dirty repo
	statusCmd := exec.Command("git", "-C", m.repoRoot, "status", "--porcelain")
	statusOut, err := statusCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w", err)
	}
	if len(bytes.TrimSpace(statusOut)) > 0 {
		return nil, fmt.Errorf("repo has uncommitted changes; commit or stash before creating worktree")
	}

	branch := "wt/" + name
	wtPath := filepath.Join(m.baseDir, name)

	// Check if branch already exists
	checkCmd := exec.Command("git", "-C", m.repoRoot, "rev-parse", "--verify", branch)
	if err := checkCmd.Run(); err == nil {
		return nil, fmt.Errorf("branch %q already exists", branch)
	}

	// Create worktree with new branch
	addCmd := exec.Command("git", "-C", m.repoRoot, "worktree", "add", "-b", branch, wtPath, baseRef)
	if out, err := addCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git worktree add failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	wt := &Worktree{
		Name:      name,
		Path:      wtPath,
		Branch:    branch,
		Status:    "active",
		CreatedAt: time.Now().Unix(),
	}
	return wt, nil
}

// Remove 删除 worktree 并清理对应分支
func (m *WorktreeManager) Remove(name string, force bool) error {
	wtPath := filepath.Join(m.baseDir, name)
	branch := "wt/" + name

	// Remove worktree
	args := []string{"-C", m.repoRoot, "worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, wtPath)
	rmCmd := exec.Command("git", args...)
	if out, err := rmCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree remove failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	// Delete branch
	deleteBranchArgs := []string{"-C", m.repoRoot, "branch", "-d", branch}
	if force {
		deleteBranchArgs[4] = "-D"
	}
	delCmd := exec.Command("git", deleteBranchArgs...)
	if out, err := delCmd.CombinedOutput(); err != nil {
		// Non-fatal: branch may already be gone
		_ = out
	}

	return nil
}

// List 列出已注册的 worktrees（过滤 .worktrees/ 下的条目）
func (m *WorktreeManager) List() ([]Worktree, error) {
	cmd := exec.Command("git", "-C", m.repoRoot, "worktree", "list", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git worktree list failed: %w", err)
	}

	var result []Worktree
	// Parse porcelain output: blocks separated by blank lines
	blocks := strings.Split(string(out), "\n\n")
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		var wtPath, branch string
		for _, line := range strings.Split(block, "\n") {
			if strings.HasPrefix(line, "worktree ") {
				wtPath = strings.TrimPrefix(line, "worktree ")
			} else if strings.HasPrefix(line, "branch ") {
				branch = strings.TrimPrefix(line, "branch ")
				// branch is refs/heads/wt/<name>
				branch = strings.TrimPrefix(branch, "refs/heads/")
			}
		}
		// Only include worktrees under our baseDir
		if wtPath == "" || !strings.HasPrefix(wtPath, m.baseDir) {
			continue
		}
		name := filepath.Base(wtPath)
		result = append(result, Worktree{
			Name:   name,
			Path:   wtPath,
			Branch: branch,
			Status: "active",
		})
	}
	return result, nil
}
