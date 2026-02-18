package notify

import (
	"fmt"
	"os/exec"
	"sync"

	"github.com/allenan/herd/internal/session"
)

const (
	soundInput = "/System/Library/Sounds/Blow.aiff"
	soundDone  = "/System/Library/Sounds/Glass.aiff"
)

type darwinNotifier struct {
	mu    sync.Mutex
	muted bool
}

// New creates a macOS notifier that uses osascript for desktop
// notifications and afplay for system sounds.
func New() Notifier {
	return &darwinNotifier{}
}

func (n *darwinNotifier) Notify(event Event) {
	n.mu.Lock()
	muted := n.muted
	n.mu.Unlock()
	if muted {
		return
	}

	var title, subtitle, body, sound string
	switch event.Status {
	case session.StatusInput:
		title = "Herd: Needs Input"
		subtitle = event.SessionName
		body = event.ProjectName
		sound = soundInput
	case session.StatusPlanReady:
		title = "Herd: Plan Ready"
		subtitle = event.SessionName
		body = event.ProjectName
		sound = soundInput
	case session.StatusDone:
		title = "Herd: Task Complete"
		subtitle = event.SessionName
		body = event.ProjectName
		sound = soundDone
	default:
		return
	}

	go func() {
		script := fmt.Sprintf(`display notification %q with title %q subtitle %q`, body, title, subtitle)
		exec.Command("osascript", "-e", script).Run()
		if sound != "" {
			exec.Command("afplay", sound).Run()
		}
	}()
}

func (n *darwinNotifier) SetMuted(muted bool) {
	n.mu.Lock()
	n.muted = muted
	n.mu.Unlock()
}

func (n *darwinNotifier) IsMuted() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.muted
}
