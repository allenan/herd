package tmux

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/allenan/herd/internal/session"
	gotmux "github.com/GianlucaP106/gotmux/gotmux"
	"github.com/google/uuid"
)

var debugLog *log.Logger

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	logPath := filepath.Join(home, ".herd", "debug.log")
	os.MkdirAll(filepath.Dir(logPath), 0o755)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		debugLog = log.New(os.Stderr, "[herd] ", log.LstdFlags)
		return
	}
	debugLog = log.New(f, "[herd] ", log.LstdFlags)
}

type Manager struct {
	Client    *gotmux.Tmux
	State     *session.State
	StatePath string
}

func NewManager(client *gotmux.Tmux, state *session.State, statePath string) *Manager {
	return &Manager{
		Client:    client,
		State:     state,
		StatePath: statePath,
	}
}

// paneExists checks whether a tmux pane ID is still valid.
func paneExists(paneID string) bool {
	return TmuxRun("display-message", "-p", "-t", paneID, "") == nil
}

// reloadState re-reads state from disk, merging pane IDs that the main
// process may have written after the sidebar subprocess started.
func (m *Manager) reloadState() {
	fresh, err := session.LoadState(m.StatePath)
	if err != nil {
		debugLog.Printf("reloadState: failed to load state: %v", err)
		return
	}
	// Always pick up layout pane IDs from disk (set by main process)
	if fresh.ViewportPaneID != "" {
		m.State.ViewportPaneID = fresh.ViewportPaneID
	}
	if fresh.SidebarPaneID != "" {
		m.State.SidebarPaneID = fresh.SidebarPaneID
	}

	// Validate that the viewport pane still exists in tmux
	if m.State.ViewportPaneID != "" && !paneExists(m.State.ViewportPaneID) {
		debugLog.Printf("reloadState: viewport pane %s no longer exists, clearing", m.State.ViewportPaneID)
		m.State.ViewportPaneID = ""
	}

	// Prune sessions whose tmux panes no longer exist
	valid := m.State.Sessions[:0]
	for _, s := range m.State.Sessions {
		if paneExists(s.TmuxPaneID) {
			valid = append(valid, s)
		} else {
			debugLog.Printf("reloadState: pruning session %s (%s), pane %s dead", s.ID, s.Name, s.TmuxPaneID)
		}
	}
	if len(valid) != len(m.State.Sessions) {
		m.State.Sessions = valid
		m.State.Save(m.StatePath)
	}
}

func (m *Manager) CreateSession(dir, name string) (*session.Session, error) {
	m.reloadState()

	project := session.DetectProject(dir)
	windowName := fmt.Sprintf("%s/%s", project, name)

	debugLog.Printf("CreateSession: name=%s dir=%s window=%s", name, dir, windowName)

	if err := TmuxRun(
		"new-window", "-d",
		"-t", "herd-main",
		"-n", windowName,
		"-c", dir,
		"claude",
	); err != nil {
		debugLog.Printf("CreateSession: new-window failed: %v", err)
		return nil, fmt.Errorf("failed to create tmux window: %w", err)
	}

	// Find the new window's pane ID
	sess, err := m.Client.GetSessionByName("herd-main")
	if err != nil || sess == nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	windows, err := sess.ListWindows()
	if err != nil {
		return nil, fmt.Errorf("failed to list windows: %w", err)
	}

	if len(windows) == 0 {
		return nil, fmt.Errorf("no windows found after creation")
	}

	// The new window should be the last one
	newWindow := windows[len(windows)-1]
	panes, err := newWindow.ListPanes()
	if err != nil || len(panes) == 0 {
		return nil, fmt.Errorf("failed to get pane for new window")
	}

	newSession := session.Session{
		ID:         uuid.New().String(),
		TmuxPaneID: panes[0].Id,
		Project:    project,
		Name:       name,
		Dir:        dir,
		CreatedAt:  time.Now(),
		Status:     session.StatusRunning,
	}

	debugLog.Printf("CreateSession: created session %s pane=%s", newSession.ID, newSession.TmuxPaneID)

	m.State.AddSession(newSession)

	// Auto-switch if this is the first session
	if len(m.State.Sessions) == 1 {
		m.SwitchTo(newSession.ID)
	}

	m.State.Save(m.StatePath)
	return &newSession, nil
}

func (m *Manager) SwitchTo(sessionID string) error {
	m.reloadState()

	sess := m.State.FindByID(sessionID)
	if sess == nil {
		debugLog.Printf("SwitchTo: session %s not found in state", sessionID)
		return fmt.Errorf("session %s not found", sessionID)
	}

	debugLog.Printf("SwitchTo: session=%s tmuxPane=%s viewportPane=%s", sessionID, sess.TmuxPaneID, m.State.ViewportPaneID)

	if m.State.ViewportPaneID == "" {
		debugLog.Printf("SwitchTo: no viewport pane configured")
		return fmt.Errorf("no viewport pane configured")
	}

	if !paneExists(sess.TmuxPaneID) {
		debugLog.Printf("SwitchTo: session pane %s no longer exists", sess.TmuxPaneID)
		return fmt.Errorf("session pane %s no longer exists (stale state)", sess.TmuxPaneID)
	}

	// Already in viewport — just focus it
	if sess.TmuxPaneID == m.State.ViewportPaneID {
		debugLog.Printf("SwitchTo: pane %s already in viewport, focusing", sess.TmuxPaneID)
		m.State.LastActiveSession = sessionID
		TmuxRun("select-pane", "-t", sess.TmuxPaneID)
		m.State.Save(m.StatePath)
		return nil
	}

	// Swap the session pane into the viewport
	if err := TmuxRun("swap-pane", "-s", sess.TmuxPaneID, "-t", m.State.ViewportPaneID); err != nil {
		debugLog.Printf("SwitchTo: swap-pane failed: %v", err)
		return fmt.Errorf("failed to swap pane: %w", err)
	}
	debugLog.Printf("SwitchTo: swap-pane succeeded")

	// After swap: the session pane is now in the viewport position,
	// and the old viewport pane moved to where the session pane was.
	m.State.ViewportPaneID = sess.TmuxPaneID
	m.State.LastActiveSession = sessionID

	// Focus the viewport pane so keyboard input goes to the session
	if err := TmuxRun("select-pane", "-t", sess.TmuxPaneID); err != nil {
		debugLog.Printf("SwitchTo: select-pane failed: %v", err)
	} else {
		debugLog.Printf("SwitchTo: select-pane succeeded, viewportPane now=%s", m.State.ViewportPaneID)
	}

	m.State.Save(m.StatePath)
	return nil
}

func (m *Manager) KillSession(sessionID string) error {
	m.reloadState()

	sess := m.State.FindByID(sessionID)
	if sess == nil {
		debugLog.Printf("KillSession: session %s not found", sessionID)
		return fmt.Errorf("session %s not found", sessionID)
	}

	debugLog.Printf("KillSession: session=%s pane=%s viewportPane=%s", sessionID, sess.TmuxPaneID, m.State.ViewportPaneID)

	isInViewport := sess.TmuxPaneID == m.State.ViewportPaneID
	paneID := sess.TmuxPaneID

	// Remove from state first, before killing the pane
	m.State.RemoveSession(sessionID)

	// Only kill the pane if no other session references it (safety check)
	otherUsesPane := false
	for _, s := range m.State.Sessions {
		if s.TmuxPaneID == paneID {
			otherUsesPane = true
			debugLog.Printf("KillSession: pane %s still used by session %s, not killing", paneID, s.ID)
			break
		}
	}

	if !otherUsesPane {
		if err := TmuxRun("kill-pane", "-t", paneID); err != nil {
			debugLog.Printf("KillSession: kill-pane %s failed: %v", paneID, err)
		} else {
			debugLog.Printf("KillSession: killed pane %s", paneID)
		}
	}

	// If the killed pane was in the viewport, find a replacement
	if isInViewport {
		m.State.ViewportPaneID = ""

		if len(m.State.Sessions) > 0 {
			// Use the next session's pane as the new viewport by switching to it.
			// We need a valid viewport target first — find any non-sidebar pane
			// in window 0 (the main window).
			tmuxSess, err := m.Client.GetSessionByName("herd-main")
			if err == nil && tmuxSess != nil {
				windows, err := tmuxSess.ListWindows()
				if err == nil {
					for _, w := range windows {
						panes, err := w.ListPanes()
						if err != nil {
							continue
						}
						for _, p := range panes {
							if p.Id != m.State.SidebarPaneID {
								m.State.ViewportPaneID = p.Id
								debugLog.Printf("KillSession: found replacement viewport pane %s", p.Id)
								break
							}
						}
						if m.State.ViewportPaneID != "" {
							break
						}
					}
				}
			}

			if m.State.ViewportPaneID != "" {
				debugLog.Printf("KillSession: switching to session %s", m.State.Sessions[0].ID)
				m.SwitchTo(m.State.Sessions[0].ID)
			} else {
				debugLog.Printf("KillSession: no replacement viewport pane found")
			}
		} else {
			debugLog.Printf("KillSession: no sessions left")
		}
	}

	m.State.Save(m.StatePath)
	return nil
}

func (m *Manager) ListSessions() []session.Session {
	return m.State.Sessions
}

func (m *Manager) RefreshStatus() {
	for i := range m.State.Sessions {
		sess := &m.State.Sessions[i]
		output, err := m.Client.Command("display-message", "-p", "-t", sess.TmuxPaneID, "#{pane_pid}")
		if err != nil || strings.TrimSpace(output) == "" {
			sess.Status = session.StatusExited
		}
	}
}
