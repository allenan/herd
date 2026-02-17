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
		"split-window", "-h", "-b", "-l", "24",
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

	// Enable focus events so panes receive focus-in/out escape sequences
	client.Command("set-option", "-t", "herd-main", "focus-events", "on")

	// Bind Ctrl-] to jump back to the sidebar pane (no prefix needed)
	client.Command("bind-key", "-n", "C-]", "select-pane", "-t", sidebarPaneID)

	// Enable mouse mode for click-to-focus and scroll
	client.Command("set-option", "-t", "herd-main", "mouse", "on")

	// Scrollback buffer for copy-mode
	client.Command("set-option", "-t", "herd-main", "history-limit", "50000")

	// Subtle copy-mode styling (avoids jarring yellow highlight)
	client.Command("set-option", "-t", "herd-main", "mode-style", "bg=colour236,fg=colour248")

	// Scroll into copy-mode with -e so scrolling back to bottom auto-exits
	client.Command("bind-key", "-T", "root", "WheelUpPane",
		"if-shell", "-Ft=", "#{mouse_any_flag}",
		"send-keys -M",
		"if-shell -Ft= '#{pane_in_mode}' 'send-keys -M' 'copy-mode -e'",
	)

	// Escape and q exit copy-mode cleanly
	client.Command("bind-key", "-T", "copy-mode", "Escape", "send-keys", "-X", "cancel")
	client.Command("bind-key", "-T", "copy-mode", "q", "send-keys", "-X", "cancel")

	// Override default mouse behavior in copy-mode to stay in copy-mode (allows text selection)
	client.Command("bind-key", "-T", "copy-mode", "MouseDown1Pane", "select-pane")
	client.Command("bind-key", "-T", "copy-mode", "MouseDragEnd1Pane", "send-keys", "-X", "copy-pipe-no-clear", "pbcopy")

	// Pin sidebar to fixed width. The initial split happens on a detached
	// session (small default size); tmux proportionally scales panes when a
	// client attaches or the terminal is resized. These hooks correct it.
	resizeCmd := fmt.Sprintf("resize-pane -t %s -x 28", sidebarPaneID)
	client.Command("set-hook", "-t", "herd-main", "client-attached[0]", resizeCmd)
	client.Command("set-hook", "-t", "herd-main", "client-resized[0]", resizeCmd)

	return sidebarPaneID, viewportPaneID, nil
}

// ShowPlaceholder replaces the viewport pane content with the welcome message.
func ShowPlaceholder(paneID string) {
	placeholderCmd := "printf '\\033[?25l\\n\\n        \\033[1;38;5;205m🐕 herd\\033[0m\\n\\n    \\033[38;5;241mCreate a session to get started.\\n    Press n in the sidebar.\\033[0m\\n'; exec cat"
	TmuxRun("respawn-pane", "-k", "-t", paneID, "sh", "-c", placeholderCmd)
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
