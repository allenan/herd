package tui

import (
	"encoding/json"
	"os"
	"time"

	"github.com/allenan/herd/internal/session"
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
	modeSearch
)

// claudeSpinner uses the same animation sequence as Claude Code's spinner,
// with first/last frames duplicated to create a hold/easing effect.
var claudeSpinner = spinner.Spinner{
	Frames: []string{"·", "·", "✢", "✳", "✶", "✻", "✽", "✽", "✻", "✶", "✳", "✢"},
	FPS:    150 * time.Millisecond,
}

// terminalSpinner is a subtler animation for active terminal sessions,
// reserving the asterisk animation for Claude Code sessions.
var terminalSpinner = spinner.Spinner{
	Frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
	FPS:    100 * time.Millisecond,
}

type App struct {
	mode         mode
	sidebar      SidebarModel
	prompt       PromptModel
	spinner      spinner.Model
	termSpinner  spinner.Model
	manager      *tmux.Manager
	width        int
	height       int
	defaultDir   string
	profileName  string
	err            string
	focused        bool
	waitingPopup   bool
	showHelp       bool
	pendingDelete  *session.Session
	searchText     string
}

func NewApp(manager *tmux.Manager, defaultDir, profileName string) App {
	sidebar := NewSidebarModel()
	sidebar.SetSessions(manager.ListSessions())
	sidebar.SetActive(manager.State.LastActiveSession)

	s := spinner.New()
	s.Spinner = claudeSpinner
	s.Style = statusRunningStyle

	ts := spinner.New()
	ts.Spinner = terminalSpinner
	ts.Style = statusRunningStyle

	return App{
		mode:        modeNormal,
		sidebar:     sidebar,
		prompt:      NewPromptModel(),
		spinner:     s,
		termSpinner: ts,
		manager:     manager,
		defaultDir:  defaultDir,
		profileName: profileName,
		focused:     true,
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
	Dir    string
	Mode   string
	Branch string
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
		return popupResultMsg{Dir: result.Dir, Mode: result.Mode, Branch: result.Branch}
	})
}

func (a App) Init() tea.Cmd {
	return tea.Batch(tea.EnableReportFocus, statusTick(), a.spinner.Tick, a.termSpinner.Tick)
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
		reconciled := a.manager.Reconcile()
		refreshed := a.manager.RefreshStatus()
		if reconciled || refreshed {
			a.sidebar.SetSessions(a.manager.ListSessions())
		}
		return a, statusTick()
	case spinner.TickMsg:
		var cmd1, cmd2 tea.Cmd
		a.spinner, cmd1 = a.spinner.Update(msg)
		a.termSpinner, cmd2 = a.termSpinner.Update(msg)
		return a, tea.Batch(cmd1, cmd2)
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
		if msg.Mode == "canceled" {
			return a, nil
		}
		if msg.Mode == "worktree" {
			if _, err := a.manager.CreateWorktreeSession(msg.Dir, msg.Branch); err != nil {
				a.err = err.Error()
			} else {
				a.err = ""
			}
		} else {
			a.manager.CreateSession(msg.Dir, "New Session")
			a.err = ""
		}
		a.sidebar.SetSessions(a.manager.ListSessions())
		a.sidebar.SetActive(a.manager.State.LastActiveSession)
		return a, nil
	}

	switch a.mode {
	case modePrompt:
		return a.updatePrompt(msg)
	case modeSearch:
		return a.updateSearch(msg)
	default:
		return a.updateNormal(msg)
	}
}

func (a App) updateNormal(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle pending delete confirmation first
		if a.pendingDelete != nil {
			switch msg.String() {
			case "y", "enter":
				a.manager.KillSession(a.pendingDelete.ID)
				a.sidebar.SetFilter("")
				a.sidebar.SetSessions(a.manager.ListSessions())
				a.sidebar.SetActive(a.manager.State.LastActiveSession)
				a.err = ""
			}
			a.pendingDelete = nil
			return a, nil
		}

		// Dismiss help on any key except ? itself
		if a.showHelp && !key.Matches(msg, keys.Help) {
			a.showHelp = false
		}
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
			// On a project or session — instantly create session in that project
			_, dir := a.sidebar.CurrentProjectInfo()
			if dir == "" {
				dir = a.defaultDir
			}
			a.manager.CreateSession(dir, "New Session")
			a.sidebar.SetFilter("")
			a.sidebar.SetSessions(a.manager.ListSessions())
			a.sidebar.SetActive(a.manager.State.LastActiveSession)
			a.err = ""
		case key.Matches(msg, keys.Worktree):
			return a.handleWorktree()
		case key.Matches(msg, keys.Terminal):
			return a.handleNewTerminal()
		case key.Matches(msg, keys.Delete):
			if sel := a.sidebar.Selected(); sel != nil {
				a.pendingDelete = sel
			}
		case key.Matches(msg, keys.Search):
			a.mode = modeSearch
			a.searchText = ""
			a.sidebar.SetFilter("")
		case key.Matches(msg, keys.Mute):
			if a.manager.Notifier != nil {
				a.manager.Notifier.SetMuted(!a.manager.Notifier.IsMuted())
			}
		case key.Matches(msg, keys.Help):
			a.showHelp = !a.showHelp
		}
	}
	return a, nil
}

func (a App) updateSearch(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEscape:
			a.mode = modeNormal
			a.searchText = ""
			a.sidebar.SetFilter("")
			return a, nil
		case tea.KeyEnter:
			a.mode = modeNormal
			return a, nil
		case tea.KeyBackspace:
			if len(a.searchText) > 0 {
				a.searchText = a.searchText[:len(a.searchText)-1]
				a.sidebar.SetFilter(a.searchText)
			}
			return a, nil
		case tea.KeyRunes:
			a.searchText += string(msg.Runes)
			a.sidebar.SetFilter(a.searchText)
			return a, nil
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
	if a.profileName != "" {
		popupArgs = append(popupArgs, "--profile", a.profileName)
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

func (a App) handleWorktree() (tea.Model, tea.Cmd) {
	if !a.sidebar.HasSessions() {
		a.err = "no project context for worktree"
		return a, nil
	}

	_, dir := a.sidebar.CurrentProjectInfo()
	if dir == "" {
		a.err = "no project context for worktree"
		return a, nil
	}

	repoRoot := session.DetectRepoRoot(dir)
	if repoRoot == "" {
		a.err = "not a git repository"
		return a, nil
	}

	project := session.DetectProject(dir)
	return a.launchWorktreePopup(project, repoRoot)
}

func (a App) handleNewTerminal() (tea.Model, tea.Cmd) {
	if !a.sidebar.HasSessions() {
		a.err = "no project context for terminal"
		return a, nil
	}

	project, dir := a.sidebar.CurrentProjectInfo()
	if dir == "" {
		a.err = "no project context for terminal"
		return a, nil
	}

	if _, err := a.manager.CreateTerminal(dir, project); err != nil {
		a.err = err.Error()
	} else {
		a.err = ""
	}
	a.sidebar.SetSessions(a.manager.ListSessions())
	a.sidebar.SetActive(a.manager.State.LastActiveSession)
	return a, nil
}

func (a App) launchWorktreePopup(project, repoRoot string) (tea.Model, tea.Cmd) {
	if a.waitingPopup {
		return a, nil
	}

	if !tmux.TmuxSupportsPopup() {
		a.err = "worktrees require tmux >= 3.2"
		return a, nil
	}

	resultPath := tmux.PopupResultPath()
	os.Remove(resultPath)

	executable, err := os.Executable()
	if err != nil {
		a.err = "failed to find executable"
		return a, nil
	}

	popupArgs := []string{
		executable, "popup-worktree",
		"--project", project,
		"--repo-root", repoRoot,
		"--result-path", resultPath,
	}
	if a.profileName != "" {
		popupArgs = append(popupArgs, "--profile", a.profileName)
	}

	opts := tmux.PopupOpts{
		Title:  "New Worktree in " + project,
		Width:  60,
		Height: 10,
	}

	if err := tmux.ShowPopup(opts, popupArgs...); err != nil {
		a.err = "failed to open popup"
		return a, nil
	}

	a.waitingPopup = true
	a.err = ""
	return a, checkPopupResult(resultPath)
}

func (a App) renderHelp() string {
	hintStyle := statusBarStyle.PaddingTop(0)
	header := statusBarStyle.Render("shortcuts")
	lines := []string{
		header,
		hintStyle.Render("j/k    navigate"),
		hintStyle.Render("enter  switch"),
		hintStyle.Render("space  collapse"),
		hintStyle.Render("/      search"),
		hintStyle.Render("n      new session"),
		hintStyle.Render("N      new project"),
		hintStyle.Render("w      worktree"),
		hintStyle.Render("t      terminal"),
		hintStyle.Render("d      delete (confirms)"),
		hintStyle.Render("m      mute"),
		hintStyle.Render("q      quit"),
		hintStyle.Render("?      close"),
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (a App) View() string {
	titleText := "🐕 herd"
	if a.profileName != "" {
		titleText = "🐕 herd (" + a.profileName + ")"
	}
	if a.manager.Notifier != nil && a.manager.Notifier.IsMuted() {
		titleText += " 🔇"
	}
	var title string
	if a.focused {
		title = titleStyle.Render(titleText)
	} else {
		title = titleBlurredStyle.Render(titleText)
	}

	spinnerFrame := a.spinner.View()
	termSpinnerFrame := a.termSpinner.View()

	var body string
	if a.mode == modePrompt {
		body = a.sidebar.View(a.width, a.height, a.focused, spinnerFrame, termSpinnerFrame) + "\n\n" + a.prompt.View()
	} else {
		body = a.sidebar.View(a.width, a.height, a.focused, spinnerFrame, termSpinnerFrame)
	}

	var statusLine string
	if a.focused {
		if a.pendingDelete != nil {
			name := a.pendingDelete.DisplayName()
			statusLine = deleteConfirmStyle.Render("delete \""+name+"\"? y/n")
		} else if a.mode == modeSearch {
			statusLine = searchStyle.Render("/ " + a.searchText + "█")
		} else if a.showHelp {
			statusLine = a.renderHelp()
		} else if a.sidebar.Filter() != "" {
			statusLine = searchStyle.Render("/ "+a.sidebar.Filter()) + "  " + statusBarStyle.PaddingTop(0).Render("? shortcuts")
		} else {
			statusLine = statusBarStyle.Render("? shortcuts")
		}
		if a.err != "" {
			statusLine = errStyle.Render("err: "+a.err) + "\n" + statusLine
		}
	} else {
		if a.err != "" {
			statusLine = errBlurredStyle.Render("err: "+a.err) + "\n" + statusBarBlurredStyle.Render("← ctrl-h · ctrl-l →")
		} else {
			statusLine = statusBarBlurredStyle.Render("← ctrl-h · ctrl-l →")
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
