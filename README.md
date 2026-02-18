<p align="center">
  <img src="herd.png" alt="Herd" width="128" />
</p>

<h1 align="center">Herd</h1>

<p align="center">
  <strong>Run multiple Claude Code sessions. Stay on top of all of them.</strong>
</p>

<p align="center">
  <a href="#install">Install</a> &middot;
  <a href="#features">Features</a> &middot;
  <a href="#keybindings">Keybindings</a> &middot;
  <a href="#how-it-works">How it works</a>
</p>

---

<!-- TODO: Replace with VHS recording -->
<p align="center">
  <img src="https://vhs.charm.sh/vhs-placeholder.gif" alt="Herd demo" width="720" />
</p>

---

Herd is a terminal UI for running multiple [Claude Code](https://docs.anthropic.com/en/docs/claude-code) sessions side by side, grouped by project. Switch between them instantly, see which ones need your attention, and never lose a session to a crashed terminal again.

Ships as a single binary. No config files, no daemon, no setup.

## Install

### Homebrew

```bash
brew tap allenan/tap
brew install herd
```

### From source

```bash
git clone https://github.com/allenan/herd.git
cd herd
make build
./herd
```

Requires Go 1.24+ and tmux.

## Features

**Instant session switching** &mdash; Create Claude Code sessions and flip between them with a keystroke. Each session runs in its own isolated pane.

**Project grouping** &mdash; Sessions are automatically grouped by git repo. Collapse and expand project groups to keep the sidebar manageable.

**Status at a glance** &mdash; See which sessions are running, idle, waiting for input, or finished without switching to them.

| Indicator | Meaning |
|-----------|---------|
| `●` (spinner) | Running |
| `!` | Needs input |
| `●` (gray) | Idle |
| `✓` | Done |
| `x` | Exited |

**Sessions survive everything** &mdash; Quit herd, close your terminal, reboot your machine. Your Claude Code sessions keep running. Relaunch `herd` and they're all still there.

**Git worktree integration** &mdash; Spin up a session on an isolated branch with `w`. Herd creates the worktree and launches Claude Code in it.

## Keybindings

### Sidebar

| Key | Action |
|-----|--------|
| `j` / `k` | Navigate up/down |
| `Enter` | Switch to session |
| `Space` | Collapse/expand project |
| `n` | New session |
| `N` | New session (pick directory) |
| `w` | New session with git worktree |
| `d` | Delete session |
| `q` | Quit (sessions keep running) |

### Viewport

| Key | Action |
|-----|--------|
| `Ctrl-]` | Return focus to sidebar |

Everything else passes through to Claude Code.

## How it works

Herd uses tmux as a PTY multiplexer behind the scenes &mdash; you never interact with tmux directly. When you run `herd`:

1. A dedicated tmux server starts on its own socket (`~/.herd/tmux.sock`), isolated from your normal tmux
2. A two-pane layout is created: sidebar (left) + viewport (right)
3. The sidebar runs a [Bubble Tea](https://github.com/charmbracelet/bubbletea) TUI for navigation
4. Each Claude Code session is a tmux window that gets swapped into the viewport when selected

State is persisted to `~/.herd/state.json`. A reconciliation loop runs every 2 seconds to sync state with live tmux panes &mdash; if state gets corrupted or deleted, sessions are automatically recovered.

## Requirements

- **tmux** &mdash; installed automatically via Homebrew, or `apt install tmux` / `dnf install tmux` on Linux
- **Claude Code** &mdash; [install instructions](https://docs.anthropic.com/en/docs/claude-code)

## License

MIT
