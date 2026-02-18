package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/allenan/herd/internal/session"
	"github.com/allenan/herd/internal/worktree"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const maxVisibleSuggestions = 8

// PopupResult is the JSON structure written to the result file.
type PopupResult struct {
	Dir    string `json:"dir"`
	Mode   string `json:"mode"`
	Branch string `json:"branch,omitempty"`
}

// WriteCanceledResult writes a canceled result file if no result was already written.
// Called after the popup process exits to signal cancellation to the sidebar.
func WriteCanceledResult(resultPath string) {
	if _, err := os.Stat(resultPath); err == nil {
		return // result already written (successful submission)
	}
	result := PopupResult{Mode: "canceled"}
	data, _ := json.Marshal(result)
	tmp := resultPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	os.Rename(tmp, resultPath)
}

// PopupModel is the Bubble Tea model for the popup directory picker.
type PopupModel struct {
	mode        string // "new_project" or "add_session"
	projectName string // for add_session mode, the project name in the title
	dirInput    textinput.Model
	suggestions []Completion
	selectedIdx int // highlighted suggestion (-1 = none)
	scrollOff   int // scroll offset for suggestions list
	project     string // auto-detected project name (live preview)
	err         string
	width       int
	height      int
	resultPath  string
}

// NewPopupModel creates a new popup model with the given parameters.
func NewPopupModel(mode, dir, projectName, resultPath string) PopupModel {
	ti := textinput.New()
	ti.Placeholder = "~/projects/my-app"
	ti.CharLimit = 512
	ti.Width = 50
	ti.SetValue(dir)
	ti.Focus()
	ti.CursorEnd()

	m := PopupModel{
		mode:        mode,
		projectName: projectName,
		dirInput:    ti,
		selectedIdx: -1,
		resultPath:  resultPath,
	}

	// Compute initial suggestions and project preview
	m.refreshSuggestions()
	m.refreshProject()

	return m
}

func (m PopupModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m PopupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.width > 4 {
			m.dirInput.Width = m.width - 8
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, tea.Quit

		case "enter":
			dir := ExpandPath(m.dirInput.Value())
			dir = strings.TrimRight(dir, "/")
			info, err := os.Stat(dir)
			if err != nil || !info.IsDir() {
				m.err = "not a valid directory"
				return m, nil
			}
			m.err = ""
			if err := m.writeResult(dir); err != nil {
				m.err = "failed to write result"
				return m, nil
			}
			return m, tea.Quit

		case "tab":
			m.applyCompletion()
			m.refreshSuggestions()
			m.refreshProject()
			return m, nil

		case "up":
			if m.selectedIdx > 0 {
				m.selectedIdx--
				if m.selectedIdx < m.scrollOff {
					m.scrollOff = m.selectedIdx
				}
			} else if m.selectedIdx == -1 && len(m.suggestions) > 0 {
				m.selectedIdx = 0
			}
			return m, nil

		case "down":
			if m.selectedIdx < len(m.suggestions)-1 {
				m.selectedIdx++
				if m.selectedIdx >= m.scrollOff+maxVisibleSuggestions {
					m.scrollOff = m.selectedIdx - maxVisibleSuggestions + 1
				}
			}
			return m, nil
		}
	}

	// Delegate to text input
	prevVal := m.dirInput.Value()
	var cmd tea.Cmd
	m.dirInput, cmd = m.dirInput.Update(msg)

	// If value changed, refresh suggestions
	if m.dirInput.Value() != prevVal {
		m.selectedIdx = -1
		m.scrollOff = 0
		m.refreshSuggestions()
		m.refreshProject()
		m.err = ""
	}

	return m, cmd
}

func (m *PopupModel) applyCompletion() {
	if len(m.suggestions) == 0 {
		return
	}

	if m.selectedIdx >= 0 && m.selectedIdx < len(m.suggestions) {
		// Complete to highlighted item
		comp := m.suggestions[m.selectedIdx]
		m.setInputToCompletion(comp.FullPath)
		m.selectedIdx = -1
		m.scrollOff = 0
		return
	}

	// Complete to common prefix
	if len(m.suggestions) == 1 {
		m.setInputToCompletion(m.suggestions[0].FullPath)
		m.selectedIdx = -1
		m.scrollOff = 0
		return
	}

	names := make([]string, len(m.suggestions))
	for i, c := range m.suggestions {
		names[i] = c.Name
	}
	common := CommonPrefix(names)
	if common == "" {
		return
	}

	// Build the completed path
	expanded := ExpandPath(m.dirInput.Value())
	var parentDir string
	if strings.HasSuffix(expanded, "/") {
		parentDir = expanded
	} else {
		parentDir = filepath.Dir(expanded)
	}
	completed := filepath.Join(parentDir, common)
	m.setInputToCompletion(completed)
}

func (m *PopupModel) setInputToCompletion(fullPath string) {
	display := ContractPath(fullPath) + "/"
	// Avoid double trailing slash
	display = strings.TrimSuffix(display, "//") + "/"
	// Normalize
	display = strings.ReplaceAll(display, "//", "/")
	m.dirInput.SetValue(display)
	m.dirInput.CursorEnd()
}

func (m *PopupModel) refreshSuggestions() {
	val := m.dirInput.Value()
	if val == "" {
		m.suggestions = nil
		return
	}
	completions, err := ListCompletions(val)
	if err != nil {
		m.suggestions = nil
		return
	}
	m.suggestions = completions
}

func (m *PopupModel) refreshProject() {
	val := m.dirInput.Value()
	if val == "" {
		m.project = ""
		return
	}
	dir := ExpandPath(val)
	dir = strings.TrimRight(dir, "/")
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		m.project = session.DetectProject(dir)
	} else {
		m.project = filepath.Base(dir)
	}
}

func (m *PopupModel) writeResult(dir string) error {
	result := PopupResult{
		Dir:  dir,
		Mode: m.mode,
	}
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	tmp := m.resultPath + ".tmp"
	if err := os.MkdirAll(filepath.Dir(m.resultPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, m.resultPath)
}

func (m PopupModel) View() string {
	w := m.width
	if w <= 0 {
		w = 60
	}
	innerW := w - 4 // padding

	// Directory input
	dirLabel := popupLabelStyle.Render("Directory")
	dirInput := m.dirInput.View()

	// Suggestions
	var sugView string
	if len(m.suggestions) > 0 {
		visible := m.suggestions
		start := m.scrollOff
		end := start + maxVisibleSuggestions
		if end > len(visible) {
			end = len(visible)
		}
		if start < len(visible) {
			visible = visible[start:end]
		}

		var lines []string
		for i, comp := range visible {
			realIdx := start + i
			marker := "  "
			style := popupSuggestionStyle
			if realIdx == m.selectedIdx {
				marker = "> "
				style = popupSuggestionSelectedStyle
			}
			name := comp.Name
			maxLen := innerW - 4
			if maxLen > 0 && len(name) > maxLen {
				name = name[:maxLen-1] + "~"
			}
			lines = append(lines, marker+style.Render(name))
		}
		sugView = strings.Join(lines, "\n")
	}

	// Pad suggestions area to fixed height
	sugLines := strings.Count(sugView, "\n") + 1
	if sugView == "" {
		sugLines = 0
	}
	for sugLines < maxVisibleSuggestions {
		sugView += "\n"
		sugLines++
	}

	// Project preview
	var projectLine string
	if m.project != "" {
		projectLine = popupProjectLabelStyle.Render("Project: ") + popupProjectNameStyle.Render(m.project)
	}

	// Error
	var errLine string
	if m.err != "" {
		errLine = popupErrStyle.Render(m.err)
	}

	// Hints
	hints := popupHintStyle.Render("tab complete \u00b7 enter create \u00b7 esc cancel")

	// Assemble
	var sections []string
	sections = append(sections, "")
	sections = append(sections, "  "+dirLabel)
	sections = append(sections, "  "+dirInput)
	sections = append(sections, "")
	sections = append(sections, sugView)
	if errLine != "" {
		sections = append(sections, "  "+errLine)
	}
	if projectLine != "" {
		sections = append(sections, "  "+projectLine)
	}
	sections = append(sections, "")
	sections = append(sections, "  "+hints)

	return strings.Join(sections, "\n")
}

// Popup-specific styles
var (
	popupLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Bold(true)

	popupSuggestionStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	popupSuggestionSelectedStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("170")).
					Bold(true)

	popupProjectLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241"))

	popupProjectNameStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("39"))

	popupErrStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	popupHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
)

// WorktreePopupModel is the Bubble Tea model for the worktree branch popup.
type WorktreePopupModel struct {
	projectName string
	repoRoot    string
	branchInput textinput.Model
	err         string
	width       int
	height      int
	resultPath  string
}

// NewWorktreePopupModel creates a new worktree popup model.
func NewWorktreePopupModel(projectName, repoRoot, resultPath string) WorktreePopupModel {
	ti := textinput.New()
	ti.Placeholder = "feature/my-branch"
	ti.CharLimit = 256
	ti.Width = 50
	ti.Focus()

	return WorktreePopupModel{
		projectName: projectName,
		repoRoot:    repoRoot,
		branchInput: ti,
		resultPath:  resultPath,
	}
}

func (m WorktreePopupModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m WorktreePopupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.width > 4 {
			m.branchInput.Width = m.width - 8
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, tea.Quit

		case "enter":
			branch := strings.TrimSpace(m.branchInput.Value())
			if branch == "" {
				m.err = "branch name required"
				return m, nil
			}
			m.err = ""
			if err := m.writeResult(branch); err != nil {
				m.err = "failed to write result"
				return m, nil
			}
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.branchInput, cmd = m.branchInput.Update(msg)
	return m, cmd
}

func (m *WorktreePopupModel) writeResult(branch string) error {
	result := PopupResult{
		Dir:    m.repoRoot,
		Mode:   "worktree",
		Branch: branch,
	}
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	tmp := m.resultPath + ".tmp"
	if err := os.MkdirAll(filepath.Dir(m.resultPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, m.resultPath)
}

func (m WorktreePopupModel) View() string {
	w := m.width
	if w <= 0 {
		w = 60
	}

	branchLabel := popupLabelStyle.Render("Branch")
	branchInput := m.branchInput.View()

	// Path preview
	branch := strings.TrimSpace(m.branchInput.Value())
	var pathLine string
	if branch != "" {
		wtDir := worktree.WorktreeDir(m.repoRoot, branch)
		pathLine = popupProjectLabelStyle.Render("Path: ") + popupProjectNameStyle.Render(ContractPath(wtDir))
	}

	var errLine string
	if m.err != "" {
		errLine = popupErrStyle.Render(m.err)
	}

	hints := popupHintStyle.Render("enter create \u00b7 esc cancel")

	var sections []string
	sections = append(sections, "")
	sections = append(sections, "  "+branchLabel)
	sections = append(sections, "  "+branchInput)
	sections = append(sections, "")
	if errLine != "" {
		sections = append(sections, "  "+errLine)
	}
	if pathLine != "" {
		sections = append(sections, "  "+pathLine)
	}
	sections = append(sections, "")
	sections = append(sections, "  "+hints)

	return strings.Join(sections, "\n")
}
