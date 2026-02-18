package tui

import (
	"encoding/json"
	"os"
	"time"

	"github.com/allenan/herd/internal/tmux"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type mode int

const (
	modeNormal mode = iota
	modePrompt
)

// claudeSpinner uses the same animation sequence as Claude Code's spinner,
// with first/last frames duplicated to create a hold/easing effect.
var claudeSpinner = spinner.Spinner{
	Frames: []string{"·", "·", "✢", "✳", "✶", "✻", "✽", "✽", "✻", "✶", "✳", "✢"},
	FPS:    150 * time.Millisecond,
}

type App struct {
	mode         mode
	sidebar      SidebarModel
	prompt       PromptModel
	spinner      spinner.Model
	manager      *tmux.Manager
	width        int
	height       int
	defaultDir   string
	err          string
	focused      bool
	waitingPopup bool
}

func NewApp(manager *tmux.Manager, defaultDir string) App {
	sidebar := NewSidebarModel()
	sidebar.SetSessions(manager.ListSessions())
	sidebar.SetActive(manager.State.LastActiveSession)

	s := spinner.New()
	s.Spinner = claudeSpinner
	s.Style = statusRunningStyle

	return App{
		mode:       modeNormal,
		sidebar:    sidebar,
		prompt:     NewPromptModel(),
		spinner:    s,
		manager:    manager,
		defaultDir: defaultDir,
		focused:    true,
	}
}

type statusTickMsg time.Time

func statusTick() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return statusTickMsg(t)
	})
}

// popupResultMsg is sent when the popup process writes a result file.
type popupResultMsg struct {
	Dir  string
	Mode string
}

// popupCheckMsg triggers re-checking for the popup result file.
type popupCheckMsg struct{}

// checkPopupResult returns a command that polls for the popup result file.
func checkPopupResult(resultPath string) tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
		data, err := os.ReadFile(resultPath)
		if err != nil {
			return popupCheckMsg{} // keep polling
		}
		os.Remove(resultPath)
		var result PopupResult
		if err := json.Unmarshal(data, &result); err != nil {
			return popupCheckMsg{}
		}
		return popupResultMsg{Dir: result.Dir, Mode: result.Mode}
	})
}

func (a App) Init() tea.Cmd {
	return tea.Batch(tea.EnableReportFocus, statusTick(), a.spinner.Tick)
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
	case statusTickMsg:
		changed := a.manager.RefreshStatus()
		if changed {
			a.sidebar.SetSessions(a.manager.ListSessions())
		}
		return a, statusTick()
	case spinner.TickMsg:
		var cmd tea.Cmd
		a.spinner, cmd = a.spinner.Update(msg)
		return a, cmd
	case popupCheckMsg:
		if a.waitingPopup {
			return a, checkPopupResult(tmux.PopupResultPath())
		}
		return a, nil
	case popupResultMsg:
		if !a.waitingPopup {
			return a, nil
		}
		a.waitingPopup = false
		a.manager.CreateSession(msg.Dir, "New Session")
		a.sidebar.SetSessions(a.manager.ListSessions())
		a.sidebar.SetActive(a.manager.State.LastActiveSession)
		a.err = ""
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
			if a.sidebar.IsOnProject() {
				a.sidebar.ToggleCollapse()
			} else if sel := a.sidebar.Selected(); sel != nil {
				if err := a.manager.SwitchTo(sel.ID); err != nil {
					a.err = err.Error()
				} else {
					a.err = ""
					a.sidebar.SetActive(sel.ID)
				}
			}
		case key.Matches(msg, keys.Space):
			a.sidebar.ToggleCollapse()
		case key.Matches(msg, keys.NewProject):
			return a.launchPopup("new_project", a.defaultDir, "")
		case key.Matches(msg, keys.New):
			if !a.sidebar.HasSessions() {
				// No sessions — behave like N (new project)
				return a.launchPopup("new_project", a.defaultDir, "")
			}
			// On a project or session — add session to that project
			project, dir := a.sidebar.CurrentProjectInfo()
			if dir == "" {
				dir = a.defaultDir
			}
			return a.launchPopup("add_session", dir, project)
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
		// Prompt completed — create session with placeholder name.
		// The real name will be populated from Claude Code's terminal
		// title via the polling loop in RefreshStatus.
		a.manager.CreateSession(result.Dir, "New Session")
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

func (a App) launchPopup(mode, dir, project string) (tea.Model, tea.Cmd) {
	if a.waitingPopup {
		return a, nil
	}

	if !tmux.TmuxSupportsPopup() {
		// Fallback to inline prompt for old tmux
		a.mode = modePrompt
		a.prompt.Start(dir)
		return a, a.prompt.dirInput.Focus()
	}

	resultPath := tmux.PopupResultPath()
	// Remove any stale result file
	os.Remove(resultPath)

	executable, err := os.Executable()
	if err != nil {
		a.err = "failed to find executable"
		return a, nil
	}

	popupArgs := []string{
		executable, "popup-new",
		"--mode", mode,
		"--dir", dir,
		"--result-path", resultPath,
	}
	if project != "" {
		popupArgs = append(popupArgs, "--project", project)
	}

	title := "New Project"
	if mode == "add_session" && project != "" {
		title = "New Session in " + project
	}

	opts := tmux.PopupOpts{
		Title:  title,
		Width:  60,
		Height: 18,
	}

	if err := tmux.ShowPopup(opts, popupArgs...); err != nil {
		a.err = "failed to open popup"
		return a, nil
	}

	a.waitingPopup = true
	a.err = ""
	return a, checkPopupResult(resultPath)
}

func (a App) View() string {
	var title string
	if a.focused {
		title = titleStyle.Render("🐕 herd")
	} else {
		title = titleBlurredStyle.Render("🐕 herd")
	}

	spinnerFrame := a.spinner.View()

	var body string
	if a.mode == modePrompt {
		body = a.sidebar.View(a.width, a.height, a.focused, spinnerFrame) + "\n\n" + a.prompt.View()
	} else {
		body = a.sidebar.View(a.width, a.height, a.focused, spinnerFrame)
	}

	var statusLine string
	if a.focused {
		if a.err != "" {
			statusLine = errStyle.Render("err: "+a.err) + "\n" + statusBarStyle.Render("[n]ew [N]ew project [d]el [q]uit")
		} else {
			statusLine = statusBarStyle.Render("[n]ew [N]ew project [d]el [q]uit")
		}
	} else {
		if a.err != "" {
			statusLine = errBlurredStyle.Render("err: "+a.err) + "\n" + statusBarBlurredStyle.Render("ctrl-h sidebar")
		} else {
			statusLine = statusBarBlurredStyle.Render("ctrl-h sidebar")
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
