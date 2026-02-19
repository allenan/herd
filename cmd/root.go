package cmd

import (
	"fmt"
	"os"
	"runtime"

	"github.com/allenan/herd/internal/profile"
	"github.com/allenan/herd/internal/session"
	htmux "github.com/allenan/herd/internal/tmux"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var sidebarFlag bool
var profileName string

// Version is set at build time via ldflags.
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:   "herd",
	Short: "TUI manager for multiple Claude Code sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		if sidebarFlag {
			return runSidebar()
		}
		return runMain()
	},
}

func init() {
	rootCmd.Version = Version
	rootCmd.Flags().BoolVar(&sidebarFlag, "sidebar", false, "run sidebar TUI (internal)")
	rootCmd.Flags().MarkHidden("sidebar")
	rootCmd.PersistentFlags().StringVar(&profileName, "profile", "", "named profile for isolated sessions")
}

func Execute() {
	// Unset TMUX so herd works when launched from inside another tmux session.
	// We use our own dedicated socket, so nesting is safe.
	os.Unsetenv("TMUX")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func printTmuxMissing() {
	colorClaude := lipgloss.AdaptiveColor{Dark: "#D77757", Light: "#D77757"}
	colorText := lipgloss.AdaptiveColor{Dark: "#FFFFFF", Light: "#000000"}
	colorAccent := lipgloss.AdaptiveColor{Dark: "#B1B9F9", Light: "#5769F7"}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorClaude).
		Padding(1, 2)

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorClaude).
		Render("herd requires tmux")

	body := lipgloss.NewStyle().
		Foreground(colorText).
		Render("tmux was not found in your PATH.\nInstall it and try again:")

	var cmd string
	if runtime.GOOS == "darwin" {
		cmd = "brew install tmux"
	} else {
		cmd = "sudo apt install tmux   # Debian/Ubuntu\nsudo dnf install tmux   # Fedora"
	}
	code := lipgloss.NewStyle().
		Foreground(colorAccent).
		Bold(true).
		Render(cmd)

	fmt.Fprintln(os.Stderr, box.Render(title+"\n\n"+body+"\n\n"+code))
}

func runMain() error {
	if htmux.IsInsideHerd() {
		return fmt.Errorf("already running inside herd")
	}

	if !htmux.IsInstalled() {
		printTmuxMissing()
		os.Exit(1)
	}

	prof, err := profile.Resolve(profileName)
	if err != nil {
		return fmt.Errorf("failed to resolve profile: %w", err)
	}
	htmux.Init(prof)

	statePath := prof.StatePath()
	state, err := session.LoadState(statePath)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	alreadyRunning := htmux.ServerRunning()

	client, err := htmux.EnsureServer()
	if err != nil {
		return fmt.Errorf("failed to start tmux server: %w", err)
	}

	htmux.ApplyEnv()

	if !alreadyRunning {
		// Fresh server — old session panes are gone, clear stale entries
		state.Sessions = nil
		state.LastActiveSession = ""
	}

	if !alreadyRunning || !htmux.HasLayout(client) {
		sidebarPaneID, viewportPaneID, err := htmux.SetupLayout(client, prof.Name)
		if err != nil {
			return fmt.Errorf("failed to setup layout: %w", err)
		}
		state.SidebarPaneID = sidebarPaneID
		state.ViewportPaneID = viewportPaneID

		// Only show the welcome placeholder on a fresh server.
		// On re-attach (sidebar crashed), the viewport may hold
		// an active session that we must not overwrite.
		if !alreadyRunning {
			htmux.ShowPlaceholder(viewportPaneID)
			htmux.TmuxRun("select-pane", "-t", state.SidebarPaneID)
			htmux.PlaceholderGuardOn(viewportPaneID, state.SidebarPaneID)
		}

		if err := state.Save(statePath); err != nil {
			return fmt.Errorf("failed to save state: %w", err)
		}
	}

	// Attach to the tmux session (blocks until detach)
	return htmux.Attach()
}
