package tmux

import (
	"fmt"
	"os"
	"strings"
)

// FindSidebarPane discovers the sidebar pane ID by scanning all panes in the
// herd session for one whose start command contains "--sidebar".
func FindSidebarPane() (string, error) {
	out, err := TmuxRunOutput(
		"list-panes", "-s", "-t", SessionName(),
		"-F", "#{pane_id}\t#{pane_start_command}",
	)
	if err != nil {
		return "", fmt.Errorf("list-panes failed: %w", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 && strings.Contains(parts[1], "--sidebar") {
			return parts[0], nil
		}
	}
	return "", fmt.Errorf("no sidebar pane found in session %s", SessionName())
}

// ReloadSidebar respawns the sidebar pane with the current on-disk binary.
// The pane is killed and replaced in one atomic tmux operation, so all
// Claude Code sessions (which live in separate windows) are unaffected.
func ReloadSidebar(sidebarPaneID, profileName string) error {
	bin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to resolve executable: %w", err)
	}

	args := []string{"respawn-pane", "-k", "-t", sidebarPaneID, bin, "--sidebar"}
	if profileName != "" {
		args = append(args, "--profile", profileName)
	}
	return TmuxRun(args...)
}
