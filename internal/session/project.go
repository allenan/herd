package session

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// DetectRepoRoot returns the absolute path of the git repository root,
// or an empty string if dir is not inside a git repo.
func DetectRepoRoot(dir string) string {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func DetectProject(dir string) string {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return filepath.Base(dir)
	}
	return filepath.Base(strings.TrimSpace(string(out)))
}

func DetectBranch(dir string) string {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
