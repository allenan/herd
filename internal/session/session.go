package session

import (
	"fmt"
	"time"
)

type Status string

const (
	StatusRunning Status = "running"
	StatusInput   Status = "input"
	StatusIdle    Status = "idle"
	StatusDone    Status = "done"
	StatusExited  Status = "exited"
	StatusShell   Status = "shell"
	StatusService Status = "service"
)

type SessionType string

const (
	TypeClaude   SessionType = ""         // zero value = backward compat
	TypeTerminal SessionType = "terminal"
)

type Session struct {
	ID             string      `json:"id"`
	TmuxPaneID     string      `json:"tmux_pane_id"`
	Project        string      `json:"project"`
	Name           string      `json:"name"`
	Title          string      `json:"title,omitempty"`
	Dir            string      `json:"dir"`
	CreatedAt      time.Time   `json:"created_at"`
	Status         Status      `json:"status"`
	Type           SessionType `json:"type,omitempty"`
	ServicePort    int         `json:"service_port,omitempty"`
	IsWorktree     bool        `json:"is_worktree,omitempty"`
	WorktreeBranch string      `json:"worktree_branch,omitempty"`
}

// DisplayName returns a human-readable name for the session.
// For terminals: port number if a service is detected, pane_current_command
// when running, or "shell" when idle. For Claude sessions: Title if set
// (from Claude Code's terminal title), otherwise the static Name.
func (s *Session) DisplayName() string {
	if s.Type == TypeTerminal {
		if s.ServicePort > 0 {
			return fmt.Sprintf(":%d", s.ServicePort)
		}
		if s.Title != "" {
			return s.Title
		}
		return "shell"
	}
	if s.Title != "" {
		return s.Title
	}
	return s.Name
}
