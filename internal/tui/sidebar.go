package tui

import (
	"fmt"

	"github.com/allenan/herd/internal/session"
)

type SidebarModel struct {
	sessions []session.Session
	cursor   int
	activeID string
}

func NewSidebarModel() SidebarModel {
	return SidebarModel{}
}

func (m *SidebarModel) SetSessions(sessions []session.Session) {
	m.sessions = sessions
	if m.cursor >= len(sessions) && len(sessions) > 0 {
		m.cursor = len(sessions) - 1
	}
}

func (m *SidebarModel) SetActive(id string) {
	m.activeID = id
}

func (m *SidebarModel) MoveUp() {
	if m.cursor > 0 {
		m.cursor--
	}
}

func (m *SidebarModel) MoveDown() {
	if m.cursor < len(m.sessions)-1 {
		m.cursor++
	}
}

func (m *SidebarModel) Selected() *session.Session {
	if len(m.sessions) == 0 {
		return nil
	}
	return &m.sessions[m.cursor]
}

func (m SidebarModel) View(width, height int) string {
	if len(m.sessions) == 0 {
		return normalStyle.Render("  No sessions yet.\n  Press n to create one.")
	}

	var s string
	for i, sess := range m.sessions {
		indicator := statusIndicator(sess.Status)
		name := fmt.Sprintf("%s/%s", sess.Project, sess.Name)

		if i == m.cursor {
			s += selectedStyle.Render(fmt.Sprintf("> %s %s", indicator, name)) + "\n"
		} else {
			s += normalStyle.Render(fmt.Sprintf("  %s %s", indicator, name)) + "\n"
		}
	}
	return s
}

func statusIndicator(status session.Status) string {
	switch status {
	case session.StatusRunning:
		return statusRunning
	case session.StatusIdle:
		return statusIdle
	case session.StatusExited:
		return statusExited
	default:
		return statusRunning
	}
}
