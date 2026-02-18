package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	gotmux "github.com/GianlucaP106/gotmux/gotmux"
)

const herdEnvVar = "HERD_ACTIVE"

func SocketPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".herd", "tmux.sock")
}

// IsInsideHerd returns true if we're already running inside a herd tmux session.
func IsInsideHerd() bool {
	return os.Getenv(herdEnvVar) == "1"
}

// tmuxCmd creates an exec.Cmd for tmux with $TMUX unset and HERD_ACTIVE=1 set.
func tmuxCmd(args ...string) *exec.Cmd {
	cmd := exec.Command("tmux", args...)
	for _, e := range os.Environ() {
		if len(e) < 5 || e[:5] != "TMUX=" {
			cmd.Env = append(cmd.Env, e)
		}
	}
	cmd.Env = append(cmd.Env, herdEnvVar+"=1")
	return cmd
}

func IsInstalled() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

func ServerRunning() bool {
	return tmuxCmd("-S", SocketPath(), "list-sessions").Run() == nil
}

func EnsureServer() (*gotmux.Tmux, error) {
	sockPath := SocketPath()

	if !ServerRunning() {
		if err := os.MkdirAll(filepath.Dir(sockPath), 0o755); err != nil {
			return nil, fmt.Errorf("failed to create socket directory: %w", err)
		}

		cmd := tmuxCmd("-S", sockPath, "new-session", "-d", "-s", "herd-main")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("failed to create tmux server: %w", err)
		}
	}
	return GetClient()
}

func GetClient() (*gotmux.Tmux, error) {
	return gotmux.NewTmux(SocketPath())
}

// TmuxRun executes a raw tmux command against our socket with $TMUX stripped.
// Use this instead of gotmux's Command for operations that must work reliably
// from inside a herd tmux pane (where tmux sets $TMUX on child processes).
func TmuxRun(args ...string) error {
	all := append([]string{"-S", SocketPath()}, args...)
	return tmuxCmd(all...).Run()
}

// TmuxRunOutput executes a raw tmux command and returns its stdout.
// Like TmuxRun, it strips $TMUX so it works from inside a herd tmux pane.
func TmuxRunOutput(args ...string) (string, error) {
	all := append([]string{"-S", SocketPath()}, args...)
	out, err := tmuxCmd(all...).Output()
	return string(out), err
}

func Attach() error {
	cmd := tmuxCmd("-S", SocketPath(), "attach-session", "-t", "herd-main")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
