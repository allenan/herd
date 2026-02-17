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
	focused    bool
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
		focused:    true,
	}
}

func (a App) Init() tea.Cmd {
	return tea.EnableReportFocus
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		return a, nil
	case tea.FocusMsg:
		a.focused = true
		return a, nil
	case tea.BlurMsg:
		a.focused = false
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
			// Detach the user's terminal. The sidebar process keeps
			// running inside tmux so state is preserved on re-attach.
			tmux.TmuxRun("detach-client")
			return a, nil
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
				a.sidebar.SetActive(a.manager.State.LastActiveSession)
				a.err = ""
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
	var title string
	if a.focused {
		title = titleStyle.Render("🐕 herd")
	} else {
		title = titleBlurredStyle.Render("🐕 herd")
	}

	var body string
	if a.mode == modePrompt {
		body = a.sidebar.View(a.width, a.height, a.focused) + "\n\n" + a.prompt.View()
	} else {
		body = a.sidebar.View(a.width, a.height, a.focused)
	}

	var statusLine string
	if a.focused {
		if a.err != "" {
			statusLine = errStyle.Render("err: "+a.err) + "\n" + statusBarStyle.Render("[n]ew [d]el [q]uit")
		} else {
			statusLine = statusBarStyle.Render("[n]ew [d]el [q]uit")
		}
	} else {
		if a.err != "" {
			statusLine = errBlurredStyle.Render("err: "+a.err) + "\n" + statusBarBlurredStyle.Render("ctrl-] sidebar")
		} else {
			statusLine = statusBarBlurredStyle.Render("ctrl-] sidebar")
		}
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

	output := content + padding + hints

	// Constrain output to pane dimensions to prevent line wrapping artifacts
	if a.width > 0 {
		output = lipgloss.NewStyle().MaxWidth(a.width).Render(output)
	}

	return output
}
