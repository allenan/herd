package tmux

import (
	"fmt"
	"os"

	gotmux "github.com/GianlucaP106/gotmux/gotmux"
)

func SetupLayout(client *gotmux.Tmux, profileName string) (sidebarPaneID string, viewportPaneID string, err error) {
	session, err := client.GetSessionByName(SessionName())
	if err != nil {
		return "", "", fmt.Errorf("failed to get %s session: %w", SessionName(), err)
	}
	if session == nil {
		return "", "", fmt.Errorf("%s session not found", SessionName())
	}

	windows, err := session.ListWindows()
	if err != nil {
		return "", "", fmt.Errorf("failed to list windows: %w", err)
	}
	if len(windows) == 0 {
		return "", "", fmt.Errorf("no windows found in %s", SessionName())
	}

	panes, err := windows[0].ListPanes()
	if err != nil {
		return "", "", fmt.Errorf("failed to list panes: %w", err)
	}
	if len(panes) == 0 {
		return "", "", fmt.Errorf("no panes found")
	}

	viewportPaneID = panes[0].Id

	// --- Terminal capability settings (BEFORE creating panes) ---
	// These must be set before split-window / new-window so that new
	// panes inherit the correct TERM and terminal features.

	// Use xterm-256color so programs (especially Claude Code) get full
	// capability support instead of tmux's default "screen" TERM.
	// default-terminal is a server option, so use -g (global).
	client.Command("set-option", "-g", "default-terminal", "xterm-256color")

	// Advertise true-color (24-bit) support to the outer terminal.
	client.Command("set-option", "-g", "terminal-overrides", ",xterm-256color:Tc")

	// Reduce escape-time from the 500ms default so TUI programs respond
	// instantly to Escape and don't confuse escape sequences.
	client.Command("set-option", "-g", "escape-time", "10")

	// Allow DCS passthrough so programs can communicate directly with the
	// outer terminal (clipboard, Kitty graphics, etc.).
	client.Command("set-option", "-g", "allow-passthrough", "on")

	// Allow programs to set pane titles via OSC escape sequences.
	// Claude Code sets terminal titles to describe what it's working on;
	// herd captures these via #{pane_title} to auto-name sessions.
	client.Command("set-option", "-g", "allow-rename", "on")
	client.Command("set-option", "-g", "set-titles", "on")

	// Enable extended key encoding (CSI u / modifyOtherKeys) so modern
	// TUI programs can distinguish key combinations correctly.
	client.Command("set-option", "-g", "extended-keys", "on")

	// Set COLORTERM so programs detect true-color support.
	client.Command("set-environment", "-g", "COLORTERM", "truecolor")

	// --- Layout ---

	selfBin, err := os.Executable()
	if err != nil {
		return "", "", fmt.Errorf("failed to get executable path: %w", err)
	}

	// Build sidebar command with optional --profile flag
	sidebarArgs := []string{selfBin, "--sidebar"}
	if profileName != "" {
		sidebarArgs = append(sidebarArgs, "--profile", profileName)
	}

	// Split window: -h horizontal, -b before (left side), -l size
	splitArgs := []string{
		"split-window", "-h", "-b", "-l", "28",
		"-t", viewportPaneID,
	}
	splitArgs = append(splitArgs, sidebarArgs...)
	_, err = client.Command(splitArgs...)
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

	sn := SessionName()

	// --- Session/window options ---

	// Disable status bar for clean look
	client.Command("set-option", "-t", sn, "status", "off")

	// Visible but subtle pane border between sidebar and viewport
	client.Command("set-option", "-t", sn, "pane-border-style", "fg=colour240")
	client.Command("set-option", "-t", sn, "pane-active-border-style", "fg=colour240")

	// Enable focus events so panes receive focus-in/out escape sequences
	client.Command("set-option", "-t", sn, "focus-events", "on")

	// Bind Ctrl-h / Ctrl-Left to focus sidebar, Ctrl-l / Ctrl-Right to focus viewport
	client.Command("bind-key", "-n", "C-h", "select-pane", "-L")
	client.Command("bind-key", "-n", "C-Left", "select-pane", "-L")
	client.Command("bind-key", "-n", "C-l", "select-pane", "-R")
	client.Command("bind-key", "-n", "C-Right", "select-pane", "-R")

	// Enable mouse mode for click-to-focus and scroll
	client.Command("set-option", "-t", sn, "mouse", "on")

	// Scrollback buffer for copy-mode
	client.Command("set-option", "-t", sn, "history-limit", "50000")

	// Subtle copy-mode styling (avoids jarring yellow highlight)
	client.Command("set-option", "-t", sn, "mode-style", "bg=colour236,fg=colour248")

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
	resizeCmd := fmt.Sprintf("resize-pane -t %s -x 32", sidebarPaneID)
	client.Command("set-hook", "-t", sn, "client-attached[0]", resizeCmd)
	client.Command("set-hook", "-t", sn, "client-resized[0]", resizeCmd)

	return sidebarPaneID, viewportPaneID, nil
}

// ShowPlaceholder replaces the viewport pane content with the welcome message.
func ShowPlaceholder(paneID string) {
	placeholderCmd := "printf '\\033[?25l\\n\\n        \\033[1;38;5;205m🐕 herd\\033[0m\\n\\n    \\033[38;5;241mCreate a session to get started.\\n    Press n in the sidebar.\\033[0m\\n'; exec cat"
	TmuxRun("respawn-pane", "-k", "-t", paneID, "sh", "-c", placeholderCmd)
}

func HasLayout(client *gotmux.Tmux) bool {
	session, err := client.GetSessionByName(SessionName())
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
