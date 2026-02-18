package tui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type PromptModel struct {
	active   bool
	dirInput textinput.Model
	dir      string
}

func NewPromptModel() PromptModel {
	di := textinput.New()
	di.Placeholder = "directory path"
	di.CharLimit = 256

	return PromptModel{
		dirInput: di,
	}
}

type PromptResult struct {
	Dir string
}

func (m *PromptModel) Start(defaultDir string) {
	m.active = true
	m.dirInput.SetValue(defaultDir)
	m.dir = ""
	m.dirInput.Focus()
}

func (m *PromptModel) Cancel() {
	m.active = false
	m.dirInput.Blur()
}

func (m *PromptModel) IsActive() bool {
	return m.active
}

func (m *PromptModel) Update(msg tea.Msg) (*PromptResult, tea.Cmd) {
	if !m.active {
		return nil, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.Cancel()
			return nil, nil
		case "enter":
			m.dir = m.dirInput.Value()
			if m.dir == "" {
				return nil, nil
			}
			m.active = false
			m.dirInput.Blur()
			return &PromptResult{Dir: m.dir}, nil
		}
	}

	var cmd tea.Cmd
	m.dirInput, cmd = m.dirInput.Update(msg)
	return nil, cmd
}

func (m PromptModel) View() string {
	if !m.active {
		return ""
	}

	return promptLabelStyle.Render("Directory:") + "\n" + "  " + m.dirInput.View()
}
