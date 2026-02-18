package tui

import "github.com/charmbracelet/lipgloss"

// Claude Code color palette — adapts to terminal background automatically.
// Override with HERD_THEME=light or HERD_THEME=dark.
var (
	colorClaude   = lipgloss.AdaptiveColor{Dark: "#D77757", Light: "#D77757"} // brand terracotta
	colorText     = lipgloss.AdaptiveColor{Dark: "#FFFFFF", Light: "#000000"} // primary text
	colorInactive = lipgloss.AdaptiveColor{Dark: "#999999", Light: "#666666"} // dimmed/muted
	colorSubtle   = lipgloss.AdaptiveColor{Dark: "#505050", Light: "#AFAFAF"} // very dim elements
	colorAccent   = lipgloss.AdaptiveColor{Dark: "#B1B9F9", Light: "#5769F7"} // suggestion/permission blue
	colorSuccess  = lipgloss.AdaptiveColor{Dark: "#4EBA65", Light: "#2C7A39"} // success green
	colorError    = lipgloss.AdaptiveColor{Dark: "#FF6B80", Light: "#AB2B3F"} // error pink/red
	colorWarning  = lipgloss.AdaptiveColor{Dark: "#FFC107", Light: "#966C1E"} // warning amber
	colorTeal     = lipgloss.AdaptiveColor{Dark: "#48968C", Light: "#006666"} // plan mode teal
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorClaude).
			PaddingLeft(1).
			PaddingBottom(1)

	selectedStyle = lipgloss.NewStyle().
			Foreground(colorClaude).
			Bold(true)

	normalStyle = lipgloss.NewStyle().
			Foreground(colorText)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(colorInactive).
			PaddingTop(1).
			PaddingLeft(1)

	promptLabelStyle = lipgloss.NewStyle().
				Foreground(colorAccent).
				Bold(true).
				PaddingLeft(1)

	errStyle = lipgloss.NewStyle().
			Foreground(colorError).
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

	statusRunningStyle = lipgloss.NewStyle().Foreground(colorClaude)
	statusInput        = lipgloss.NewStyle().Foreground(colorWarning).Render("!")
	statusIdle         = lipgloss.NewStyle().Foreground(colorInactive).Render("●")
	statusDone         = lipgloss.NewStyle().Foreground(colorTeal).Render("✓")
	statusPlanReady    = lipgloss.NewStyle().Foreground(colorTeal).Render("◆")
	statusExited       = lipgloss.NewStyle().Foreground(colorError).Render("x")
	statusService      = lipgloss.NewStyle().Foreground(colorSuccess).Render("◉")

	// Project header styles
	projectHeaderStyle = lipgloss.NewStyle().
				Foreground(colorAccent).
				Bold(true)

	projectHeaderBlurredStyle = lipgloss.NewStyle().
					Faint(true)

	// Session count shown after project name
	sessionCountStyle = lipgloss.NewStyle().
				Foreground(colorInactive)

	sessionCountBlurredStyle = lipgloss.NewStyle().
					Faint(true)

	// Chevron styles
	chevronStyle = lipgloss.NewStyle().Foreground(colorSubtle)

	// Cursor indicator
	cursorGlyph = lipgloss.NewStyle().Foreground(colorClaude).Bold(true).Render("▸")

	// Active session indicator (when cursor is elsewhere)
	activeGlyph = lipgloss.NewStyle().Foreground(colorInactive).Render("▸")

	// Active session name style when sidebar is unfocused
	activeStyle = lipgloss.NewStyle().
			Foreground(colorText)

	// Search prompt style
	searchStyle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true).
			PaddingLeft(1)

	// Delete confirmation style
	deleteConfirmStyle = lipgloss.NewStyle().
				Foreground(colorWarning).
				Bold(true).
				PaddingLeft(1)
)
