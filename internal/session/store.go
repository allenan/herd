package session

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
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
			return loadFromBackup(path)
		}
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		// Archive the corrupt file and try backup
		archiveCorrupt(path)
		return loadFromBackup(path)
	}
	return &s, nil
}

// loadFromBackup tries to load state from the .bak file, falling back to empty state.
func loadFromBackup(path string) (*State, error) {
	bakPath := path + ".bak"
	data, err := os.ReadFile(bakPath)
	if err != nil {
		return &State{TmuxSocket: "herd"}, nil
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return &State{TmuxSocket: "herd"}, nil
	}
	return &s, nil
}

// archiveCorrupt renames a corrupt state file to state.json.corrupt.<timestamp>
// so it can be inspected later. Best-effort; errors are ignored.
func archiveCorrupt(path string) {
	ts := time.Now().Format("20060102-150405")
	os.Rename(path, fmt.Sprintf("%s.corrupt.%s", path, ts))
}

func (s *State) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// Best-effort backup before writing
	backupState(path)
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

// backupState copies state.json to state.json.bak using read+write
// (not rename) so state.json is never missing during the backup window.
func backupState(path string) {
	src, err := os.Open(path)
	if err != nil {
		return
	}
	defer src.Close()
	dst, err := os.Create(path + ".bak")
	if err != nil {
		return
	}
	defer dst.Close()
	io.Copy(dst, src)
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
