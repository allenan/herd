package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var validName = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$`)

// Profile represents an isolated herd environment with its own state,
// tmux server, and claude config directory.
type Profile struct {
	Name           string // "" for default
	BaseDir        string // resolved directory (e.g. ~/.herd or ~/.herd/profiles/work)
	ClaudeConfigDir string // if set, CLAUDE_CONFIG_DIR env var for tmux server
}

// Config is the JSON config stored in a profile directory.
type Config struct {
	ClaudeConfigDir string `json:"claude_config_dir,omitempty"`
}

// Resolve returns a Profile for the given name. An empty name returns the
// default profile using ~/.herd/ directly (fully backward compatible).
func Resolve(name string) (*Profile, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}

	if name == "" {
		return &Profile{
			Name:    "",
			BaseDir: filepath.Join(home, ".herd"),
		}, nil
	}

	if !validName.MatchString(name) {
		return nil, fmt.Errorf("invalid profile name %q: must be alphanumeric with hyphens", name)
	}

	baseDir := filepath.Join(home, ".herd", "profiles", name)

	p := &Profile{
		Name:           name,
		BaseDir:        baseDir,
		ClaudeConfigDir: filepath.Join(home, ".claude-"+name),
	}

	if err := p.EnsureDir(); err != nil {
		return nil, err
	}

	// Load config if it exists
	configPath := filepath.Join(baseDir, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Write default config on first use
			if err := p.SaveDefaultConfig(); err != nil {
				return nil, err
			}
			return p, nil
		}
		return nil, fmt.Errorf("failed to read profile config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse profile config: %w", err)
	}
	if cfg.ClaudeConfigDir != "" {
		p.ClaudeConfigDir = cfg.ClaudeConfigDir
	}

	return p, nil
}

func (p *Profile) StatePath() string {
	return filepath.Join(p.BaseDir, "state.json")
}

func (p *Profile) PopupResultPath() string {
	return filepath.Join(p.BaseDir, "popup-result.json")
}

func (p *Profile) TmuxSessionName() string {
	if p.Name == "" {
		return "herd-main"
	}
	return "herd-" + p.Name + "-main"
}

func (p *Profile) SocketPath() string {
	return filepath.Join(p.BaseDir, "tmux.sock")
}

func (p *Profile) LogPath() string {
	return filepath.Join(p.BaseDir, "debug.log")
}

func (p *Profile) EnsureDir() error {
	return os.MkdirAll(p.BaseDir, 0o755)
}

func (p *Profile) SaveDefaultConfig() error {
	cfg := Config{ClaudeConfigDir: p.ClaudeConfigDir}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(p.BaseDir, "config.json"), data, 0o644)
}
