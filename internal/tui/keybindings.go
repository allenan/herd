package tui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Up         key.Binding
	Down       key.Binding
	Enter      key.Binding
	Space      key.Binding
	New        key.Binding
	NewProject key.Binding
	Worktree   key.Binding
	Terminal   key.Binding
	Delete     key.Binding
	Search     key.Binding
	MoveUp     key.Binding
	MoveDown   key.Binding
	Mute       key.Binding
	Reload     key.Binding
	Quit       key.Binding
	Help       key.Binding
}

var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("k", "up"),
		key.WithHelp("k/up", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("j", "down"),
		key.WithHelp("j/down", "down"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "switch"),
	),
	Space: key.NewBinding(
		key.WithKeys(" "),
		key.WithHelp("space", "toggle"),
	),
	New: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "new"),
	),
	NewProject: key.NewBinding(
		key.WithKeys("N"),
		key.WithHelp("N", "new project"),
	),
	Worktree: key.NewBinding(
		key.WithKeys("w"),
		key.WithHelp("w", "worktree"),
	),
	Terminal: key.NewBinding(
		key.WithKeys("t"),
		key.WithHelp("t", "terminal"),
	),
	Delete: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "delete"),
	),
	Search: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "search"),
	),
	MoveUp: key.NewBinding(
		key.WithKeys("K"),
		key.WithHelp("K", "move up"),
	),
	MoveDown: key.NewBinding(
		key.WithKeys("J"),
		key.WithHelp("J", "move down"),
	),
	Mute: key.NewBinding(
		key.WithKeys("m"),
		key.WithHelp("m", "mute"),
	),
	Reload: key.NewBinding(
		key.WithKeys("R"),
		key.WithHelp("R", "reload sidebar"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q"),
		key.WithHelp("q", "quit"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "shortcuts"),
	),
}
