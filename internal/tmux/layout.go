package tmux

import (
	"fmt"
	"os"

	gotmux "github.com/GianlucaP106/gotmux/gotmux"
)

func SetupLayout(client *gotmux.Tmux) (sidebarPaneID string, viewportPaneID string, err error) {
	session, err := client.GetSessionByName("herd-main")
	if err != nil {
		return "", "", fmt.Errorf("failed to get herd-main session: %w", err)
	}
	if session == nil {
		return "", "", fmt.Errorf("herd-main session not found")
	}

	windows, err := session.ListWindows()
	if err != nil {
		return "", "", fmt.Errorf("failed to list windows: %w", err)
	}
	if len(windows) == 0 {
		return "", "", fmt.Errorf("no windows found in herd-main")
	}

	panes, err := windows[0].ListPanes()
	if err != nil {
		return "", "", fmt.Errorf("failed to list panes: %w", err)
	}
	if len(panes) == 0 {
		return "", "", fmt.Errorf("no panes found")
	}

	viewportPaneID = panes[0].Id

	selfBin, err := os.Executable()
	if err != nil {
		return "", "", fmt.Errorf("failed to get executable path: %w", err)
	}

	// Split window: -h horizontal, -b before (left side), -l size
	_, err = client.Command(
		"split-window", "-h", "-b", "-l", "20%",
		"-t", viewportPaneID,
		selfBin, "--sidebar",
	)
	if err != nil {
		return "", "", fmt.Errorf("failed to create sidebar split: %w", err)
	}

	// Re-list panes to find the sidebar pane ID
	panes, err = windows[0].ListPanes()
	if err != nil {
		return "", "", fmt.Errorf("failed to re-list panes: %w", err)
	}

	for _, p := range panes {
		if p.Id != viewportPaneID {
			sidebarPaneID = p.Id
			break
		}
	}

	if sidebarPaneID == "" {
		return "", "", fmt.Errorf("could not identify sidebar pane")
	}

	// Disable status bar for clean look
	client.Command("set-option", "-t", "herd-main", "status", "off")

	// Bind Ctrl-] to jump back to the sidebar pane (no prefix needed)
	client.Command("bind-key", "-n", "C-]", "select-pane", "-t", sidebarPaneID)

	return sidebarPaneID, viewportPaneID, nil
}

func HasLayout(client *gotmux.Tmux) bool {
	session, err := client.GetSessionByName("herd-main")
	if err != nil || session == nil {
		return false
	}
	windows, err := session.ListWindows()
	if err != nil || len(windows) == 0 {
		return false
	}
	panes, err := windows[0].ListPanes()
	if err != nil {
		return false
	}
	return len(panes) >= 2
}
