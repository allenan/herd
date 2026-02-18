package notify

import "github.com/allenan/herd/internal/session"

// Event describes a session status transition worth notifying about.
type Event struct {
	SessionName string
	ProjectName string
	Status      session.Status
}

// Notifier sends desktop notifications and plays sounds.
type Notifier interface {
	Notify(event Event)
	SetMuted(muted bool)
	IsMuted() bool
}
