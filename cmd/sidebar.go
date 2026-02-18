package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/allenan/herd/internal/notify"
	"github.com/allenan/herd/internal/profile"
	"github.com/allenan/herd/internal/session"
	htmux "github.com/allenan/herd/internal/tmux"
	"github.com/allenan/herd/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func runSidebar() error {
	prof, err := profile.Resolve(profileName)
	if err != nil {
		return fmt.Errorf("failed to resolve profile: %w", err)
	}
	htmux.Init(prof)

	client, err := htmux.GetClient()
	if err != nil {
		return fmt.Errorf("failed to connect to tmux: %w", err)
	}

	statePath := prof.StatePath()
	state, err := session.LoadState(statePath)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	manager := htmux.NewManager(client, state, statePath)
	manager.Notifier = notify.New()
	manager.Reconcile()

	// Default directory for new sessions: use the directory herd was launched from
	defaultDir, err := os.Getwd()
	if err != nil {
		defaultDir = os.Getenv("HOME")
	}

	// Allow overriding light/dark detection (OSC 11 can be unreliable inside tmux)
	switch strings.ToLower(os.Getenv("HERD_THEME")) {
	case "light":
		lipgloss.SetHasDarkBackground(false)
	case "dark":
		lipgloss.SetHasDarkBackground(true)
	}

	app := tui.NewApp(manager, defaultDir, prof.Name)
	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithReportFocus())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}
