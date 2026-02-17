package tui

import (
	"github.com/allenan/herd/internal/tmux"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type mode int

const (
	modeNormal mode = iota
	modePrompt
)

type App struct {
	mode       mode
	sidebar    SidebarModel
	prompt     PromptModel
	manager    *tmux.Manager
	width      int
	height     int
	defaultDir string
	err        string
}

func NewApp(manager *tmux.Manager, defaultDir string) App {
	sidebar := NewSidebarModel()
	sidebar.SetSessions(manager.ListSessions())
	sidebar.SetActive(manager.State.LastActiveSession)

	return App{
		mode:       modeNormal,
		sidebar:    sidebar,
		prompt:     NewPromptModel(),
		manager:    manager,
		defaultDir: defaultDir,
	}
}

func (a App) Init() tea.Cmd {
	return nil
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		return a, nil
	}

	switch a.mode {
	case modePrompt:
		return a.updatePrompt(msg)
	default:
		return a.updateNormal(msg)
	}
}

func (a App) updateNormal(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			// Detach the user from the herd tmux session before quitting.
			// Sessions keep running in the background.
			tmux.TmuxRun("detach-client")
			return a, tea.Quit
		case key.Matches(msg, keys.Up):
			a.sidebar.MoveUp()
		case key.Matches(msg, keys.Down):
			a.sidebar.MoveDown()
		case key.Matches(msg, keys.Enter):
			if sel := a.sidebar.Selected(); sel != nil {
				if err := a.manager.SwitchTo(sel.ID); err != nil {
					a.err = err.Error()
				} else {
					a.err = ""
					a.sidebar.SetActive(sel.ID)
				}
			}
		case key.Matches(msg, keys.New):
			a.mode = modePrompt
			a.prompt.Start(a.defaultDir)
			return a, a.prompt.dirInput.Focus()
		case key.Matches(msg, keys.Delete):
			if sel := a.sidebar.Selected(); sel != nil {
				a.manager.KillSession(sel.ID)
				a.sidebar.SetSessions(a.manager.ListSessions())
			}
		}
	}
	return a, nil
}

func (a App) updatePrompt(msg tea.Msg) (tea.Model, tea.Cmd) {
	result, cmd := a.prompt.Update(msg)
	if result != nil {
		// Prompt completed — create the session
		a.manager.CreateSession(result.Dir, result.Name)
		a.sidebar.SetSessions(a.manager.ListSessions())
		a.sidebar.SetActive(a.manager.State.LastActiveSession)
		a.mode = modeNormal
		return a, nil
	}
	if !a.prompt.IsActive() {
		// Prompt was cancelled
		a.mode = modeNormal
		return a, nil
	}
	return a, cmd
}

func (a App) View() string {
	title := titleStyle.Render("herd")

	var body string
	if a.mode == modePrompt {
		body = a.sidebar.View(a.width, a.height) + "\n\n" + a.prompt.View()
	} else {
		body = a.sidebar.View(a.width, a.height)
	}

	var statusLine string
	if a.err != "" {
		statusLine = errStyle.Render("err: "+a.err) + "\n" + statusBarStyle.Render("[n]ew [d]el [q]uit")
	} else {
		statusLine = statusBarStyle.Render("[n]ew [d]el [q]uit")
	}
	hints := statusLine

	// Fill remaining height
	content := lipgloss.JoinVertical(lipgloss.Left, title, body)
	contentHeight := lipgloss.Height(content)
	hintsHeight := lipgloss.Height(hints)
	gap := a.height - contentHeight - hintsHeight
	if gap < 0 {
		gap = 0
	}
	padding := ""
	for i := 0; i < gap; i++ {
		padding += "\n"
	}

	return content + padding + hints
}
