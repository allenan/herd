package session

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// LivePane is a tmux-agnostic representation of a running pane,
// passed from the manager to avoid importing tmux in this package.
type LivePane struct {
	PaneID         string
	CurrentCommand string
	StartCommand   string
	CurrentPath    string
	Title          string
	WindowIndex    int
}

// Reconcile compares state sessions against live tmux panes.
// It prunes dead sessions and adopts orphan claude panes.
// layoutPaneIDs is the set of pane IDs belonging to the layout (sidebar, viewport placeholder).
// Returns true if state was modified.
func (s *State) Reconcile(livePanes []LivePane, layoutPaneIDs map[string]bool) bool {
	changed := false

	// Build lookup of live pane IDs
	liveSet := make(map[string]bool, len(livePanes))
	for _, lp := range livePanes {
		liveSet[lp.PaneID] = true
	}

	// Prune: remove sessions whose pane is no longer alive
	valid := s.Sessions[:0]
	for _, sess := range s.Sessions {
		if liveSet[sess.TmuxPaneID] {
			valid = append(valid, sess)
		} else {
			changed = true
		}
	}
	s.Sessions = valid

	// Build set of tracked pane IDs
	tracked := make(map[string]bool, len(s.Sessions))
	for _, sess := range s.Sessions {
		tracked[sess.TmuxPaneID] = true
	}

	// Adopt: find untracked claude panes outside window 0
	for _, lp := range livePanes {
		if tracked[lp.PaneID] || layoutPaneIDs[lp.PaneID] || lp.WindowIndex == 0 {
			continue
		}
		if !isClaudePane(lp) {
			continue
		}
		sess := Session{
			ID:         uuid.New().String(),
			TmuxPaneID: lp.PaneID,
			Project:    DetectProject(lp.CurrentPath),
			Name:       deriveName(lp),
			Dir:        lp.CurrentPath,
			CreatedAt:  time.Now(),
			Status:     StatusIdle,
		}
		s.Sessions = append(s.Sessions, sess)
		changed = true
	}

	return changed
}

// isClaudePane checks whether a live pane is running claude.
func isClaudePane(lp LivePane) bool {
	for _, cmd := range []string{lp.CurrentCommand, lp.StartCommand} {
		if strings.Contains(strings.ToLower(cmd), "claude") {
			return true
		}
	}
	return false
}

// deriveName produces a session name from a live pane's title or path.
func deriveName(lp LivePane) string {
	if lp.Title != "" {
		return lp.Title
	}
	if lp.CurrentPath != "" {
		return filepath.Base(lp.CurrentPath)
	}
	return "Recovered Session"
}
