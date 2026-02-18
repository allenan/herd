package cmd

import (
	"fmt"
	"os"

	"github.com/allenan/herd/internal/profile"
	"github.com/allenan/herd/internal/session"
	htmux "github.com/allenan/herd/internal/tmux"
	"github.com/allenan/herd/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
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
	manager.Reconcile()

	// Default directory for new sessions: use the directory herd was launched from
	defaultDir, err := os.Getwd()
	if err != nil {
		defaultDir = os.Getenv("HOME")
	}

	app := tui.NewApp(manager, defaultDir, prof.Name)
	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithReportFocus())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}
