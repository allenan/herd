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
	ID          string    `json:"id"`
	TmuxPaneID  string    `json:"tmux_pane_id"`
	Project     string    `json:"project"`
	Name        string    `json:"name"`
	Dir         string    `json:"dir"`
	CreatedAt   time.Time `json:"created_at"`
	Status      Status    `json:"status"`
}
