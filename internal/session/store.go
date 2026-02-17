package session

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type State struct {
	Sessions         []Session `json:"sessions"`
	TmuxSocket       string    `json:"tmux_socket"`
	LastActiveSession string   `json:"last_active_session"`
	ViewportPaneID   string    `json:"viewport_pane_id"`
	SidebarPaneID    string    `json:"sidebar_pane_id"`
}

func DefaultStatePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".herd", "state.json")
}

func LoadState(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &State{TmuxSocket: "herd"}, nil
		}
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (s *State) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *State) AddSession(sess Session) {
	s.Sessions = append(s.Sessions, sess)
}

func (s *State) RemoveSession(id string) {
	for i, sess := range s.Sessions {
		if sess.ID == id {
			s.Sessions = append(s.Sessions[:i], s.Sessions[i+1:]...)
			return
		}
	}
}

func (s *State) FindByID(id string) *Session {
	for i := range s.Sessions {
		if s.Sessions[i].ID == id {
			return &s.Sessions[i]
		}
	}
	return nil
}

func (s *State) FindByPaneID(paneID string) *Session {
	for i := range s.Sessions {
		if s.Sessions[i].TmuxPaneID == paneID {
			return &s.Sessions[i]
		}
	}
	return nil
}
