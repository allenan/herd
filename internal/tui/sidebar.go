package tui

import (
	"fmt"

	"github.com/allenan/herd/internal/session"
)

type itemKind int

const (
	itemProject itemKind = iota
	itemSession
)

type visibleItem struct {
	kind    itemKind
	project string
	session *session.Session // nil for project headers
}

type SidebarModel struct {
	sessions  []session.Session
	items     []visibleItem
	collapsed map[string]bool
	cursor    int
	activeID  string
}

func NewSidebarModel() SidebarModel {
	return SidebarModel{
		collapsed: make(map[string]bool),
	}
}

func (m *SidebarModel) SetSessions(sessions []session.Session) {
	m.sessions = sessions

	// Prune collapsed entries for projects that no longer exist
	projects := make(map[string]bool)
	for _, s := range sessions {
		projects[s.Project] = true
	}
	for p := range m.collapsed {
		if !projects[p] {
			delete(m.collapsed, p)
		}
	}

	m.rebuildItems()

	if m.cursor >= len(m.items) && len(m.items) > 0 {
		m.cursor = len(m.items) - 1
	}
}

func (m *SidebarModel) SetActive(id string) {
	m.activeID = id

	// Auto-expand collapsed project containing the active session
	for _, s := range m.sessions {
		if s.ID == id && m.collapsed[s.Project] {
			m.collapsed[s.Project] = false
			m.rebuildItems()
			break
		}
	}

	for i, item := range m.items {
		if item.kind == itemSession && item.session != nil && item.session.ID == id {
			m.cursor = i
			return
		}
	}
}

func (m *SidebarModel) MoveUp() {
	if m.cursor > 0 {
		m.cursor--
	}
}

func (m *SidebarModel) MoveDown() {
	if m.cursor < len(m.items)-1 {
		m.cursor++
	}
}

func (m *SidebarModel) Selected() *session.Session {
	if len(m.items) == 0 || m.cursor >= len(m.items) {
		return nil
	}
	item := m.items[m.cursor]
	if item.kind == itemSession {
		return item.session
	}
	return nil
}

func (m *SidebarModel) IsOnProject() bool {
	if len(m.items) == 0 || m.cursor >= len(m.items) {
		return false
	}
	return m.items[m.cursor].kind == itemProject
}

func (m *SidebarModel) ToggleCollapse() {
	if len(m.items) == 0 || m.cursor >= len(m.items) {
		return
	}
	item := m.items[m.cursor]
	project := item.project
	wasCollapsed := m.collapsed[project]
	m.collapsed[project] = !wasCollapsed

	if !wasCollapsed {
		// Collapsing — move cursor to the project header
		m.rebuildItems()
		for i, it := range m.items {
			if it.kind == itemProject && it.project == project {
				m.cursor = i
				return
			}
		}
	} else {
		m.rebuildItems()
	}
}

func (m *SidebarModel) rebuildItems() {
	m.items = nil

	// Group sessions by project in encounter order
	type projectGroup struct {
		name     string
		sessions []*session.Session
	}
	var groups []projectGroup
	seen := make(map[string]int)

	for i := range m.sessions {
		s := &m.sessions[i]
		if idx, ok := seen[s.Project]; ok {
			groups[idx].sessions = append(groups[idx].sessions, s)
		} else {
			seen[s.Project] = len(groups)
			groups = append(groups, projectGroup{
				name:     s.Project,
				sessions: []*session.Session{s},
			})
		}
	}

	for _, g := range groups {
		m.items = append(m.items, visibleItem{
			kind:    itemProject,
			project: g.name,
		})
		if !m.collapsed[g.name] {
			// Claude sessions first, then terminals
			for _, s := range g.sessions {
				if s.Type != session.TypeTerminal {
					m.items = append(m.items, visibleItem{
						kind:    itemSession,
						project: g.name,
						session: s,
					})
				}
			}
			for _, s := range g.sessions {
				if s.Type == session.TypeTerminal {
					m.items = append(m.items, visibleItem{
						kind:    itemSession,
						project: g.name,
						session: s,
					})
				}
			}
		}
	}
}

// sessionCount returns the total number of sessions in a project group.
func (m *SidebarModel) sessionCount(project string) int {
	count := 0
	for _, s := range m.sessions {
		if s.Project == project {
			count++
		}
	}
	return count
}

// CurrentProjectInfo returns the project name and directory for the item at the cursor.
// For project headers, it uses the first session's dir. For sessions, uses that session's dir.
func (m *SidebarModel) CurrentProjectInfo() (project, dir string) {
	if len(m.items) == 0 || m.cursor >= len(m.items) {
		return "", ""
	}
	item := m.items[m.cursor]
	projectName := item.project

	// Find the first session in this project to get the directory
	for _, s := range m.sessions {
		if s.Project == projectName {
			return projectName, s.Dir
		}
	}

	return projectName, ""
}

// HasSessions returns true if there are any sessions.
func (m *SidebarModel) HasSessions() bool {
	return len(m.sessions) > 0
}

func (m SidebarModel) View(width, height int, focused bool, spinnerFrame string) string {
	if len(m.sessions) == 0 {
		if focused {
			return normalStyle.Render(" No sessions yet.\n Press n to create one.")
		}
		return normalBlurredStyle.Render(" No sessions yet.")
	}

	var s string
	for i, item := range m.items {
		isCursor := i == m.cursor

		switch item.kind {
		case itemProject:
			if i > 0 {
				s += "\n"
			}
			s += m.renderProject(item.project, isCursor, focused) + "\n"
		case itemSession:
			isActive := item.session != nil && item.session.ID == m.activeID
			s += m.renderSession(item.session, isCursor, focused, isActive, spinnerFrame) + "\n"
		}
	}
	return s
}

func (m SidebarModel) renderProject(project string, isCursor, focused bool) string {
	chevronChar := "▼"
	if m.collapsed[project] {
		chevronChar = "▶"
	}
	count := fmt.Sprintf("(%d)", m.sessionCount(project))

	if focused {
		if isCursor {
			chevron := selectedStyle.Render(chevronChar)
			countStr := sessionCountStyle.Render(count)
			name := selectedStyle.Render(project)
			return fmt.Sprintf(" %s %s %s", chevron, name, countStr)
		}
		chevron := chevronStyle.Render(chevronChar)
		countStr := sessionCountStyle.Render(count)
		name := projectHeaderStyle.Render(project)
		return fmt.Sprintf(" %s %s %s", chevron, name, countStr)
	}

	chevron := chevronStyle.Render(chevronChar)
	countStr := sessionCountBlurredStyle.Render(count)
	name := projectHeaderBlurredStyle.Render(project)
	return fmt.Sprintf(" %s %s %s", chevron, name, countStr)
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "~"
}

func (m SidebarModel) renderSession(sess *session.Session, isCursor, focused, isActive bool, spinnerFrame string) string {
	indicator := statusIndicator(sess, spinnerFrame)
	name := sess.DisplayName()
	if sess.Type == session.TypeTerminal {
		name = "$ " + name
	} else if sess.IsWorktree {
		name = "\u2387 " + name // ⎇ prefix
	}
	display := truncate(name, 24)

	// All sessions use the same layout: " GG  I name"
	// where GG = 2-char glyph column (▸ + space, or 2 spaces),
	// I = status indicator. This keeps everything vertically aligned
	// regardless of cursor/active state.
	var glyph string
	if isCursor {
		glyph = cursorGlyph + " "
	} else if isActive {
		glyph = activeGlyph + " "
	} else {
		glyph = "  "
	}

	var styledName string
	if focused {
		if isCursor {
			styledName = selectedStyle.Render(display)
		} else {
			styledName = normalStyle.Render(display)
		}
	} else {
		if isActive {
			styledName = activeStyle.Render(display)
		} else {
			styledName = normalBlurredStyle.Render(display)
		}
	}

	return fmt.Sprintf(" %s %s %s", glyph, indicator, styledName)
}

func statusIndicator(sess *session.Session, spinnerFrame string) string {
	switch sess.Status {
	case session.StatusRunning:
		return spinnerFrame
	case session.StatusIdle:
		return statusIdle
	case session.StatusShell:
		return statusIdle
	case session.StatusService:
		return statusService
	case session.StatusDone:
		return statusDone
	case session.StatusInput:
		return statusInput
	case session.StatusExited:
		return statusExited
	default:
		return spinnerFrame
	}
}
