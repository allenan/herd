package tmux

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// PopupResultPath returns the path for the popup result file.
func PopupResultPath() string {
	if baseDir != "" {
		return filepath.Join(baseDir, "popup-result.json")
	}
	// Fallback if Init not called
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".herd", "popup-result.json")
}

// TmuxSupportsPopup checks whether the tmux server supports display-popup (>= 3.2).
func TmuxSupportsPopup() bool {
	out, err := TmuxRunOutput("display-message", "-p", "#{version}")
	if err != nil {
		return false
	}
	version := strings.TrimSpace(out)
	// Strip trailing letter suffixes like "3.4a"
	numStr := strings.TrimRight(version, "abcdefghijklmnopqrstuvwxyz-")
	parts := strings.SplitN(numStr, ".", 2)
	if len(parts) < 2 {
		return false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	return major > 3 || (major == 3 && minor >= 2)
}

// PopupOpts configures the tmux display-popup.
type PopupOpts struct {
	Title  string
	Width  int
	Height int
}

// ShowPopup launches a tmux display-popup with the given command.
// It returns immediately; the popup runs asynchronously in the tmux server.
func ShowPopup(opts PopupOpts, command ...string) error {
	args := []string{"display-popup", "-EE", "-b", "rounded"}

	if opts.Title != "" {
		args = append(args, "-T", fmt.Sprintf(" %s ", opts.Title))
	}
	if opts.Width > 0 {
		args = append(args, "-w", strconv.Itoa(opts.Width))
	}
	if opts.Height > 0 {
		args = append(args, "-h", strconv.Itoa(opts.Height))
	}

	// Style the popup border
	args = append(args, "-S", "fg=colour205")

	args = append(args, command...)
	return TmuxRun(args...)
}
