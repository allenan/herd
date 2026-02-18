package worktree

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// WorktreeDir computes the worktree path for a branch under the repo root.
// Slashes in the branch name are replaced with hyphens.
func WorktreeDir(repoRoot, branch string) string {
	sanitized := strings.ReplaceAll(branch, "/", "-")
	return filepath.Join(repoRoot, ".worktrees", sanitized)
}

// Create creates a git worktree for the given branch under repoRoot/.worktrees/.
// If the branch already exists, it checks it out; otherwise creates a new branch.
// Returns the worktree directory path.
func Create(repoRoot, branch string) (string, error) {
	wtDir := WorktreeDir(repoRoot, branch)

	// Try creating with a new branch first
	cmd := exec.Command("git", "-C", repoRoot, "worktree", "add", "-b", branch, wtDir)
	if err := cmd.Run(); err != nil {
		// Branch may already exist — try without -b
		cmd = exec.Command("git", "-C", repoRoot, "worktree", "add", wtDir, branch)
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("git worktree add failed: %w", err)
		}
	}

	return wtDir, nil
}

// Remove force-removes a git worktree and prunes stale entries.
func Remove(repoRoot, wtDir string) error {
	cmd := exec.Command("git", "-C", repoRoot, "worktree", "remove", "--force", wtDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git worktree remove failed: %w", err)
	}
	// Prune any stale worktree entries
	exec.Command("git", "-C", repoRoot, "worktree", "prune").Run()
	return nil
}

// IsWorktreeDir checks if the given path contains /.worktrees/ indicating
// it's a herd-managed worktree directory.
func IsWorktreeDir(dir string) bool {
	return strings.Contains(dir, "/.worktrees/")
}

// DetectBranchFromDir runs git rev-parse in the worktree dir to find the current branch.
func DetectBranchFromDir(dir string) string {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// RepoRootFromWorktreeDir extracts the repo root from a worktree path
// by taking everything before /.worktrees/.
func RepoRootFromWorktreeDir(dir string) string {
	idx := strings.Index(dir, "/.worktrees/")
	if idx < 0 {
		return ""
	}
	return dir[:idx]
}
