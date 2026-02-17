package tui

import (
	"github.com/allenan/herd/internal/session"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type promptStep int

const (
	stepDir promptStep = iota
	stepName
)

type PromptModel struct {
	active    bool
	step      promptStep
	dirInput  textinput.Model
	nameInput textinput.Model
	dir       string
	name      string
}

func NewPromptModel() PromptModel {
	di := textinput.New()
	di.Placeholder = "directory path"
	di.CharLimit = 256

	ni := textinput.New()
	ni.Placeholder = "session name"
	ni.CharLimit = 64

	return PromptModel{
		dirInput:  di,
		nameInput: ni,
	}
}

type PromptResult struct {
	Dir  string
	Name string
}

func (m *PromptModel) Start(defaultDir string) {
	m.active = true
	m.step = stepDir
	m.dirInput.SetValue(defaultDir)
	m.nameInput.SetValue("")
	m.dir = ""
	m.name = ""
	m.dirInput.Focus()
}

func (m *PromptModel) Cancel() {
	m.active = false
	m.dirInput.Blur()
	m.nameInput.Blur()
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
			switch m.step {
			case stepDir:
				m.dir = m.dirInput.Value()
				if m.dir == "" {
					return nil, nil
				}
				m.step = stepName
				m.dirInput.Blur()
				// Default name to git branch
				defaultName := session.DetectBranch(m.dir)
				if defaultName == "" {
					defaultName = "main"
				}
				m.nameInput.SetValue(defaultName)
				m.nameInput.Focus()
				return nil, nil
			case stepName:
				m.name = m.nameInput.Value()
				if m.name == "" {
					return nil, nil
				}
				m.active = false
				m.nameInput.Blur()
				return &PromptResult{Dir: m.dir, Name: m.name}, nil
			}
		}
	}

	var cmd tea.Cmd
	switch m.step {
	case stepDir:
		m.dirInput, cmd = m.dirInput.Update(msg)
	case stepName:
		m.nameInput, cmd = m.nameInput.Update(msg)
	}
	return nil, cmd
}

func (m PromptModel) View() string {
	if !m.active {
		return ""
	}

	switch m.step {
	case stepDir:
		return promptLabelStyle.Render("Directory:") + "\n" + "  " + m.dirInput.View()
	case stepName:
		return promptLabelStyle.Render("Name:") + "\n" + "  " + m.nameInput.View()
	}
	return ""
}
