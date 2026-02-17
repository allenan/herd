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

	// Sanity check: viewport and sidebar must never be the same pane
	if m.State.ViewportPaneID != "" && m.State.ViewportPaneID == m.State.SidebarPaneID {
		debugLog.Printf("reloadState: ViewportPaneID == SidebarPaneID (%s), clearing viewport", m.State.ViewportPaneID)
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

// resolveViewportPane dynamically discovers the viewport pane by querying
// tmux for actual panes in window 0. The sidebar pane is identified by its
// StartCommand (contains "--sidebar") or by matching SidebarPaneID; the
// other pane in window 0 is the viewport.
func (m *Manager) resolveViewportPane() (string, error) {
	sess, err := m.Client.GetSessionByName("herd-main")
	if err != nil || sess == nil {
		return "", fmt.Errorf("resolveViewportPane: failed to get herd-main session: %w", err)
	}

	win, err := sess.GetWindowByIndex(0)
	if err != nil || win == nil {
		return "", fmt.Errorf("resolveViewportPane: failed to get window 0: %w", err)
	}

	panes, err := win.ListPanes()
	if err != nil {
		return "", fmt.Errorf("resolveViewportPane: failed to list panes: %w", err)
	}

	if len(panes) < 2 {
		return "", fmt.Errorf("resolveViewportPane: expected >= 2 panes in window 0, got %d", len(panes))
	}

	var sidebarID string
	for _, p := range panes {
		if p.Id == m.State.SidebarPaneID || strings.Contains(p.StartCommand, "--sidebar") {
			sidebarID = p.Id
			break
		}
	}

	if sidebarID == "" {
		return "", fmt.Errorf("resolveViewportPane: could not identify sidebar pane among %d panes", len(panes))
	}

	// Correct SidebarPaneID if it was wrong
	if m.State.SidebarPaneID != sidebarID {
		debugLog.Printf("resolveViewportPane: correcting SidebarPaneID from %s to %s", m.State.SidebarPaneID, sidebarID)
		m.State.SidebarPaneID = sidebarID
	}

	// The viewport is the non-sidebar pane in window 0
	for _, p := range panes {
		if p.Id != sidebarID {
			if m.State.ViewportPaneID != p.Id {
				debugLog.Printf("resolveViewportPane: correcting ViewportPaneID from %s to %s", m.State.ViewportPaneID, p.Id)
				m.State.ViewportPaneID = p.Id
			}
			return p.Id, nil
		}
	}

	return "", fmt.Errorf("resolveViewportPane: no non-sidebar pane found in window 0")
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

	m.SwitchTo(newSession.ID)

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

	// Dynamically resolve the viewport pane instead of trusting stored ID
	viewportPaneID, err := m.resolveViewportPane()
	if err != nil {
		debugLog.Printf("SwitchTo: resolveViewportPane failed: %v", err)
		return fmt.Errorf("no viewport pane: %w", err)
	}

	debugLog.Printf("SwitchTo: session=%s tmuxPane=%s viewportPane=%s (resolved)", sessionID, sess.TmuxPaneID, viewportPaneID)

	if !paneExists(sess.TmuxPaneID) {
		debugLog.Printf("SwitchTo: session pane %s no longer exists", sess.TmuxPaneID)
		return fmt.Errorf("session pane %s no longer exists (stale state)", sess.TmuxPaneID)
	}

	// Guard: refuse to swap if the session pane IS the sidebar
	if sess.TmuxPaneID == m.State.SidebarPaneID {
		debugLog.Printf("SwitchTo: refusing to swap sidebar pane %s into viewport", sess.TmuxPaneID)
		return fmt.Errorf("session pane %s is the sidebar pane, refusing swap", sess.TmuxPaneID)
	}

	// Already in viewport — just focus it
	if sess.TmuxPaneID == viewportPaneID {
		debugLog.Printf("SwitchTo: pane %s already in viewport, focusing", sess.TmuxPaneID)
		m.State.LastActiveSession = sessionID
		TmuxRun("select-pane", "-t", sess.TmuxPaneID)
		m.State.Save(m.StatePath)
		return nil
	}

	// Swap the session pane into the viewport
	if err := TmuxRun("swap-pane", "-s", sess.TmuxPaneID, "-t", viewportPaneID); err != nil {
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

	// Remove from state first
	m.State.RemoveSession(sessionID)

	// Check if another session still references this pane
	otherUsesPane := false
	for _, s := range m.State.Sessions {
		if s.TmuxPaneID == paneID {
			otherUsesPane = true
			debugLog.Printf("KillSession: pane %s still used by session %s, not killing", paneID, s.ID)
			break
		}
	}

	if otherUsesPane {
		m.State.Save(m.StatePath)
		return nil
	}

	if isInViewport && len(m.State.Sessions) > 0 {
		// Swap a replacement session into the viewport BEFORE killing,
		// so window 0 always keeps two panes (sidebar + viewport).
		replacement := &m.State.Sessions[0]
		if err := TmuxRun("swap-pane", "-s", replacement.TmuxPaneID, "-t", paneID); err != nil {
			debugLog.Printf("KillSession: swap replacement failed: %v", err)
		} else {
			debugLog.Printf("KillSession: swapped replacement %s into viewport", replacement.TmuxPaneID)
		}
		// Kill the old pane (now swapped out of viewport)
		if err := TmuxRun("kill-pane", "-t", paneID); err != nil {
			debugLog.Printf("KillSession: kill-pane %s failed: %v", paneID, err)
		} else {
			debugLog.Printf("KillSession: killed pane %s", paneID)
		}
		m.State.ViewportPaneID = replacement.TmuxPaneID
		m.State.LastActiveSession = replacement.ID
		TmuxRun("select-pane", "-t", replacement.TmuxPaneID)
		debugLog.Printf("KillSession: switched to replacement session %s pane=%s", replacement.ID, replacement.TmuxPaneID)
	} else if isInViewport {
		// No sessions left — respawn the viewport pane with placeholder
		// instead of killing it, so the two-pane layout stays intact.
		placeholderCmd := `printf '\033[?25l\n\n        \033[1;38;5;205m🐕 herd\033[0m\n\n    \033[38;5;241mCreate a session to get started.\n    Press n in the sidebar.\033[0m\n'; exec cat`
		if err := TmuxRun("respawn-pane", "-k", "-t", paneID, "sh", "-c", placeholderCmd); err != nil {
			debugLog.Printf("KillSession: respawn-pane placeholder failed: %v", err)
		} else {
			debugLog.Printf("KillSession: respawned viewport pane %s with placeholder", paneID)
		}
		m.State.LastActiveSession = ""
		TmuxRun("select-pane", "-t", m.State.SidebarPaneID)
	} else {
		// Not in viewport — just kill the pane
		if err := TmuxRun("kill-pane", "-t", paneID); err != nil {
			debugLog.Printf("KillSession: kill-pane %s failed: %v", paneID, err)
		} else {
			debugLog.Printf("KillSession: killed pane %s", paneID)
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
