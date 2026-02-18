package tmux

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/allenan/herd/internal/session"
	"github.com/allenan/herd/internal/worktree"
	gotmux "github.com/GianlucaP106/gotmux/gotmux"
	"github.com/google/uuid"
)

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
	sess, err := m.Client.GetSessionByName(SessionName())
	if err != nil || sess == nil {
		return "", fmt.Errorf("resolveViewportPane: failed to get %s session: %w", SessionName(), err)
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
		"-t", SessionName(),
		"-n", windowName,
		"-c", dir,
		"claude",
	); err != nil {
		debugLog.Printf("CreateSession: new-window failed: %v", err)
		return nil, fmt.Errorf("failed to create tmux window: %w", err)
	}

	// Find the new window's pane ID
	sess, err := m.Client.GetSessionByName(SessionName())
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

// CreateWorktreeSession creates a git worktree and launches a Claude Code session in it.
func (m *Manager) CreateWorktreeSession(repoRoot, branch string) (*session.Session, error) {
	m.reloadState()

	debugLog.Printf("CreateWorktreeSession: repoRoot=%s branch=%s", repoRoot, branch)

	wtDir, err := worktree.Create(repoRoot, branch)
	if err != nil {
		debugLog.Printf("CreateWorktreeSession: worktree create failed: %v", err)
		return nil, fmt.Errorf("failed to create worktree: %w", err)
	}

	project := session.DetectProject(repoRoot)
	windowName := fmt.Sprintf("%s/%s", project, branch)

	if err := TmuxRun(
		"new-window", "-d",
		"-t", SessionName(),
		"-n", windowName,
		"-c", wtDir,
		"claude",
	); err != nil {
		debugLog.Printf("CreateWorktreeSession: new-window failed: %v, rolling back worktree", err)
		worktree.Remove(repoRoot, wtDir)
		return nil, fmt.Errorf("failed to create tmux window: %w", err)
	}

	// Find the new window's pane ID
	sess, err := m.Client.GetSessionByName(SessionName())
	if err != nil || sess == nil {
		worktree.Remove(repoRoot, wtDir)
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	windows, err := sess.ListWindows()
	if err != nil {
		worktree.Remove(repoRoot, wtDir)
		return nil, fmt.Errorf("failed to list windows: %w", err)
	}

	if len(windows) == 0 {
		worktree.Remove(repoRoot, wtDir)
		return nil, fmt.Errorf("no windows found after creation")
	}

	newWindow := windows[len(windows)-1]
	panes, err := newWindow.ListPanes()
	if err != nil || len(panes) == 0 {
		worktree.Remove(repoRoot, wtDir)
		return nil, fmt.Errorf("failed to get pane for new window")
	}

	newSession := session.Session{
		ID:             uuid.New().String(),
		TmuxPaneID:     panes[0].Id,
		Project:        project,
		Name:           branch,
		Dir:            wtDir,
		CreatedAt:      time.Now(),
		Status:         session.StatusRunning,
		IsWorktree:     true,
		WorktreeBranch: branch,
	}

	debugLog.Printf("CreateWorktreeSession: created session %s pane=%s worktree=%s", newSession.ID, newSession.TmuxPaneID, wtDir)

	m.State.AddSession(newSession)
	m.SwitchTo(newSession.ID)
	m.State.Save(m.StatePath)
	return &newSession, nil
}

// CreateTerminal creates a new terminal session running $SHELL in the given directory.
func (m *Manager) CreateTerminal(dir, project string) (*session.Session, error) {
	m.reloadState()

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	id := uuid.New().String()
	windowName := fmt.Sprintf("%s/term-%s", project, id[:8])

	debugLog.Printf("CreateTerminal: dir=%s project=%s window=%s", dir, project, windowName)

	if err := TmuxRun(
		"new-window", "-d",
		"-t", SessionName(),
		"-n", windowName,
		"-c", dir,
		shell,
	); err != nil {
		debugLog.Printf("CreateTerminal: new-window failed: %v", err)
		return nil, fmt.Errorf("failed to create tmux window: %w", err)
	}

	// Find the new window's pane ID
	sess, err := m.Client.GetSessionByName(SessionName())
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

	newWindow := windows[len(windows)-1]
	panes, err := newWindow.ListPanes()
	if err != nil || len(panes) == 0 {
		return nil, fmt.Errorf("failed to get pane for new window")
	}

	newSession := session.Session{
		ID:         id,
		TmuxPaneID: panes[0].Id,
		Project:    project,
		Name:       "shell",
		Dir:        dir,
		CreatedAt:  time.Now(),
		Status:     session.StatusShell,
		Type:       session.TypeTerminal,
	}

	debugLog.Printf("CreateTerminal: created session %s pane=%s", newSession.ID, newSession.TmuxPaneID)

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
		if sess.Status == session.StatusDone || sess.Status == session.StatusPlanReady {
			sess.Status = session.StatusIdle
		}
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
	if sess.Status == session.StatusDone || sess.Status == session.StatusPlanReady {
		sess.Status = session.StatusIdle
	}

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
	isWorktree := sess.IsWorktree
	sessDir := sess.Dir

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

	// Clean up git worktree if this was a worktree session
	if isWorktree && sessDir != "" {
		repoRoot := worktree.RepoRootFromWorktreeDir(sessDir)
		if repoRoot != "" {
			if err := worktree.Remove(repoRoot, sessDir); err != nil {
				debugLog.Printf("KillSession: worktree remove failed (non-fatal): %v", err)
			} else {
				debugLog.Printf("KillSession: removed worktree %s", sessDir)
			}
		}
	}

	m.State.Save(m.StatePath)
	return nil
}

func (m *Manager) ListSessions() []session.Session {
	return m.State.Sessions
}

// CollectLivePanes queries tmux for all panes in herd-main, returning them
// as session.LivePane values with pre-cleaned titles.
func (m *Manager) CollectLivePanes() ([]session.LivePane, error) {
	// List all panes across all windows in herd-main.
	// Format: pane_id, window_index, pane_current_command, pane_start_command, pane_current_path, pane_title
	out, err := TmuxRunOutput(
		"list-panes", "-s", "-t", SessionName(),
		"-F", "#{pane_id}\t#{window_index}\t#{pane_current_command}\t#{pane_start_command}\t#{pane_current_path}\t#{pane_title}",
	)
	if err != nil {
		return nil, fmt.Errorf("list-panes failed: %w", err)
	}
	var panes []session.LivePane
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 6)
		if len(parts) < 6 {
			continue
		}
		var winIdx int
		fmt.Sscanf(parts[1], "%d", &winIdx)
		panes = append(panes, session.LivePane{
			PaneID:         parts[0],
			WindowIndex:    winIdx,
			CurrentCommand: parts[2],
			StartCommand:   parts[3],
			CurrentPath:    parts[4],
			Title:          CleanPaneTitle(parts[5]),
		})
	}
	return panes, nil
}

// Reconcile reloads state, collects live panes, and reconciles them.
// Returns true if state was modified.
func (m *Manager) Reconcile() bool {
	m.reloadState()

	livePanes, err := m.CollectLivePanes()
	if err != nil {
		debugLog.Printf("Reconcile: CollectLivePanes failed: %v", err)
		return false
	}

	// Build layout exclusion set
	layoutPaneIDs := make(map[string]bool)
	if m.State.SidebarPaneID != "" {
		layoutPaneIDs[m.State.SidebarPaneID] = true
	}
	// Exclude the viewport pane only when it's NOT a session pane
	// (i.e., it's the welcome placeholder)
	if m.State.ViewportPaneID != "" && m.State.FindByPaneID(m.State.ViewportPaneID) == nil {
		layoutPaneIDs[m.State.ViewportPaneID] = true
	}

	changed := m.State.Reconcile(livePanes, layoutPaneIDs)

	// Post-process: tag untagged worktree sessions
	for i := range m.State.Sessions {
		s := &m.State.Sessions[i]
		if !s.IsWorktree && worktree.IsWorktreeDir(s.Dir) {
			s.IsWorktree = true
			s.WorktreeBranch = worktree.DetectBranchFromDir(s.Dir)
			if s.WorktreeBranch != "" {
				s.Name = s.WorktreeBranch
			}
			debugLog.Printf("Reconcile: tagged session %s as worktree (branch=%s)", s.ID, s.WorktreeBranch)
			changed = true
		}
	}

	if changed {
		debugLog.Printf("Reconcile: state changed, saving")
		m.State.Save(m.StatePath)
	}
	return changed
}

func (m *Manager) RefreshStatus() bool {
	changed := false
	for i := range m.State.Sessions {
		s := &m.State.Sessions[i]
		if s.Type == session.TypeTerminal {
			changed = m.refreshTerminalStatus(s) || changed
		} else {
			changed = m.refreshClaudeStatus(s) || changed
		}
	}
	if changed {
		m.State.Save(m.StatePath)
	}
	return changed
}

func (m *Manager) refreshClaudeStatus(s *session.Session) bool {
	changed := false
	raw := DetectStatus(s.TmuxPaneID)
	prev := s.Status

	var next session.Status
	switch {
	// Running → Idle while not in viewport → mark done
	case prev == session.StatusRunning && raw == session.StatusIdle && s.TmuxPaneID != m.State.ViewportPaneID:
		next = session.StatusDone
	// Running → PlanReady while not in viewport → keep PlanReady
	case prev == session.StatusRunning && raw == session.StatusPlanReady && s.TmuxPaneID != m.State.ViewportPaneID:
		next = session.StatusPlanReady
	// Already done and still idle → keep done (don't let polling overwrite)
	case prev == session.StatusDone && raw == session.StatusIdle:
		next = session.StatusDone
	// Done but raw changed to something else → use raw
	case prev == session.StatusDone:
		next = raw
	// PlanReady + Idle → keep PlanReady (plan path scrolled off visible area)
	case prev == session.StatusPlanReady && raw == session.StatusIdle:
		next = session.StatusPlanReady
	// PlanReady + other → use raw (user accepted/rejected, Claude moved on)
	case prev == session.StatusPlanReady:
		next = raw
	// All other transitions → use raw
	default:
		next = raw
	}

	if s.Status != next {
		s.Status = next
		changed = true
	}

	// Capture pane title set by Claude Code via OSC sequences
	if rawTitle, err := CapturePaneTitle(s.TmuxPaneID); err == nil {
		title := CleanPaneTitle(rawTitle)
		if title != s.Title {
			s.Title = title
			changed = true
		}
	}
	return changed
}

func (m *Manager) refreshTerminalStatus(s *session.Session) bool {
	changed := false

	status, cmdName := DetectTerminalStatus(s.TmuxPaneID)

	// If a command is running, check for listening ports
	if status == session.StatusRunning {
		port := DetectListeningPort(s.TmuxPaneID)
		if port > 0 {
			status = session.StatusService
			if s.ServicePort != port {
				s.ServicePort = port
				changed = true
			}
		} else if s.ServicePort != 0 {
			s.ServicePort = 0
			changed = true
		}
	} else if s.ServicePort != 0 {
		s.ServicePort = 0
		changed = true
	}

	if s.Status != status {
		s.Status = status
		changed = true
	}

	// Update Title with the running command name (empty for idle shells)
	if s.Title != cmdName {
		s.Title = cmdName
		changed = true
	}

	return changed
}
