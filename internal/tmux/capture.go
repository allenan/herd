package tmux

import (
	"strings"
	"unicode"

	"github.com/allenan/herd/internal/session"
)

// runningPatterns are strings that reliably indicate Claude Code is actively
// working (e.g. the "esc to interrupt" prompt shown during generation).
var runningPatterns = []string{
	"esc to interrupt",
}

// inputPatterns are strings that indicate Claude Code is waiting for user
// confirmation (permission prompts, yes/no questions, etc.).
var inputPatterns = []string{
	"Do you want to",
	"[Y/n]",
	"[y/N]",
	"Allow once",
	"Yes, allow",
	"Allow all",
	"trust this tool",
	"approve this",
}

// idlePatterns are strings that indicate Claude Code is at its input prompt,
// waiting for the user to type a task.
var idlePatterns = []string{
	"? for shortcuts",
	"? for help",
}

// CapturePaneContent returns the visible text of a tmux pane.
func CapturePaneContent(paneID string) (string, error) {
	return TmuxRunOutput("capture-pane", "-t", paneID, "-p")
}

// DetectStatus classifies a session's current state by inspecting its pane.
func DetectStatus(paneID string) session.Status {
	if !paneExists(paneID) {
		return session.StatusExited
	}

	content, err := CapturePaneContent(paneID)
	if err != nil {
		debugLog.Printf("DetectStatus: capture failed for %s: %v", paneID, err)
		return session.StatusExited
	}

	// Check for active generation first — "esc to interrupt" is the most
	// reliable indicator that Claude is actively working.
	for _, pattern := range runningPatterns {
		if strings.Contains(content, pattern) {
			return session.StatusRunning
		}
	}

	// Check for permission/input prompts (highest priority after running)
	for _, pattern := range inputPatterns {
		if strings.Contains(content, pattern) {
			return session.StatusInput
		}
	}

	// Check for idle prompt — Claude Code shows "? for shortcuts" at the
	// bottom when waiting for the user to type a task.
	for _, pattern := range idlePatterns {
		if strings.Contains(content, pattern) {
			return session.StatusIdle
		}
	}

	// Fallback: no recognized pattern — show as idle since we only want
	// the running indicator when "esc to interrupt" is explicitly visible.
	return session.StatusIdle
}

// DetectAllStatuses updates the status of every session and returns true
// if any status changed.
func DetectAllStatuses(sessions []session.Session) bool {
	changed := false
	for i := range sessions {
		newStatus := DetectStatus(sessions[i].TmuxPaneID)
		if sessions[i].Status != newStatus {
			sessions[i].Status = newStatus
			changed = true
		}
	}
	return changed
}

// noisePatterns are pane titles that carry no useful information and should
// be treated as empty (so DisplayName falls back to the session Name).
var noisePatterns = []string{
	"bash", "zsh", "sh", "fish",
	"claude", "claude-code",
	"tmux",
}

// CapturePaneTitle returns the pane title set via OSC escape sequences.
func CapturePaneTitle(paneID string) (string, error) {
	return TmuxRunOutput("display-message", "-p", "-t", paneID, "#{pane_title}")
}

// CleanPaneTitle filters out noise titles (hostnames, shell names, bare
// process names). Returns empty string for these so DisplayName() falls
// back to the session Name.
func CleanPaneTitle(raw string) string {
	title := strings.TrimSpace(raw)
	if title == "" {
		return ""
	}
	lower := strings.ToLower(title)
	for _, noise := range noisePatterns {
		if lower == noise {
			return ""
		}
	}
	// Strip leading status characters that Claude Code prepends to the
	// tab title (e.g. "* task name", "✳ task name") — we show our own
	// status icons and don't want duplicates. Use a unicode-aware trim
	// to catch all variants (✳, ●, *, ·, etc.) rather than enumerating.
	title = strings.TrimLeftFunc(title, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	if title == "" {
		return ""
	}

	// Filter hostnames (common default pane title is user@host)
	if strings.Contains(title, "@") && !strings.Contains(title, " ") {
		return ""
	}
	return title
}
