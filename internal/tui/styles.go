package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			PaddingLeft(1).
			PaddingBottom(1)

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("170")).
			Bold(true)

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			PaddingTop(1).
			PaddingLeft(1)

	promptLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("39")).
				Bold(true).
				PaddingLeft(1)

	errStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			PaddingLeft(1)

	titleBlurredStyle = lipgloss.NewStyle().
				Faint(true).
				PaddingLeft(1).
				PaddingBottom(1)

	selectedBlurredStyle = lipgloss.NewStyle().
				Faint(true).
				PaddingLeft(1)

	normalBlurredStyle = lipgloss.NewStyle().
				Faint(true)

	statusBarBlurredStyle = lipgloss.NewStyle().
				Faint(true).
				PaddingTop(1).
				PaddingLeft(1)

	errBlurredStyle = lipgloss.NewStyle().
			Faint(true).
			PaddingLeft(1)

	statusRunningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	statusInput        = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("!")
	statusIdle         = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render("●")
	statusDone         = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render("✓")
	statusExited       = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("x")
	statusService      = lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Render("◉")

	// Project header styles
	projectHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("39")).
				Bold(true)

	projectHeaderBlurredStyle = lipgloss.NewStyle().
					Faint(true)

	// Session count shown after project name
	sessionCountStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241"))

	sessionCountBlurredStyle = lipgloss.NewStyle().
					Faint(true)

	// Chevron styles
	chevronStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	// Cursor indicator
	cursorGlyph = lipgloss.NewStyle().Foreground(lipgloss.Color("170")).Bold(true).Render("▸")

	// Active session indicator (when cursor is elsewhere)
	activeGlyph = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("▸")

	// Active session name style when sidebar is unfocused
	activeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))
)
