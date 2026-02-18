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
			for _, s := range g.sessions {
				m.items = append(m.items, visibleItem{
					kind:    itemSession,
					project: g.name,
					session: s,
				})
			}
		}
	}
}

// sessionCount returns the number of sessions in a project group.
func (m *SidebarModel) sessionCount(project string) int {
	count := 0
	for _, s := range m.sessions {
		if s.Project == project {
			count++
		}
	}
	return count
}

func (m SidebarModel) View(width, height int, focused bool, spinnerFrame string) string {
	if len(m.sessions) == 0 {
		if focused {
			return normalStyle.Render("  No sessions yet.\n  Press n to create one.")
		}
		return normalBlurredStyle.Render("  No sessions yet.")
	}

	var s string
	for i, item := range m.items {
		isCursor := i == m.cursor

		switch item.kind {
		case itemProject:
			s += m.renderProject(item.project, isCursor, focused) + "\n"
		case itemSession:
			isActive := item.session != nil && item.session.ID == m.activeID
			s += m.renderSession(item.session, isCursor, focused, isActive, spinnerFrame) + "\n"
		}
	}
	return s
}

func (m SidebarModel) renderProject(project string, isCursor, focused bool) string {
	chevron := chevronStyle.Render("▼")
	if m.collapsed[project] {
		chevron = chevronStyle.Render("▶")
	}
	count := fmt.Sprintf("(%d)", m.sessionCount(project))

	var glyph string
	if isCursor {
		glyph = cursorGlyph + " "
	} else {
		glyph = "  "
	}

	if focused {
		if isCursor {
			countStr := sessionCountStyle.Render(count)
			name := selectedStyle.Render(project)
			return fmt.Sprintf("  %s %s %s %s", glyph, chevron, name, countStr)
		}
		countStr := sessionCountStyle.Render(count)
		name := projectHeaderStyle.Render(project)
		return fmt.Sprintf("  %s %s %s %s", glyph, chevron, name, countStr)
	}

	countStr := sessionCountBlurredStyle.Render(count)
	name := projectHeaderBlurredStyle.Render(project)
	return fmt.Sprintf("  %s %s %s %s", glyph, chevron, name, countStr)
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "~"
}

func (m SidebarModel) renderSession(sess *session.Session, isCursor, focused, isActive bool, spinnerFrame string) string {
	indicator := statusIndicator(sess.Status, spinnerFrame)
	display := truncate(sess.DisplayName(), 24)

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

	var name string
	if focused {
		if isCursor {
			name = selectedStyle.Render(display)
		} else {
			name = normalStyle.Render(display)
		}
	} else {
		if isActive {
			name = activeStyle.Render(display)
		} else {
			name = normalBlurredStyle.Render(display)
		}
	}

	return fmt.Sprintf("  %s %s %s", glyph, indicator, name)
}

func statusIndicator(status session.Status, spinnerFrame string) string {
	switch status {
	case session.StatusRunning:
		return spinnerFrame
	case session.StatusIdle:
		return statusIdle
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
