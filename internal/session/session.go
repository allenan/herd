package session

import "time"

type Status string

const (
	StatusRunning Status = "running"
	StatusInput   Status = "input"
	StatusIdle    Status = "idle"
	StatusDone    Status = "done"
	StatusExited  Status = "exited"
)

type Session struct {
	ID             string    `json:"id"`
	TmuxPaneID     string    `json:"tmux_pane_id"`
	Project        string    `json:"project"`
	Name           string    `json:"name"`
	Title          string    `json:"title,omitempty"`
	Dir            string    `json:"dir"`
	CreatedAt      time.Time `json:"created_at"`
	Status         Status    `json:"status"`
	IsWorktree     bool      `json:"is_worktree,omitempty"`
	WorktreeBranch string    `json:"worktree_branch,omitempty"`
}

// DisplayName returns the Title if set (from Claude Code's terminal title),
// otherwise falls back to the static Name.
func (s *Session) DisplayName() string {
	if s.Title != "" {
		return s.Title
	}
	return s.Name
}
