<p align="center">
  <img src="herd.png" alt="Herd" width="400" />
</p>

<!-- <h1 align="center">Herd</h1> -->

<p align="center">
  <strong>Run multiple Claude Code sessions. Stay on top of all of them.</strong>
</p>

<p align="center">
  <a href="#install">Install</a> &middot;
  <a href="#features">Features</a> &middot;
  <a href="#keybindings">Keybindings</a> &middot;
  <a href="#git-worktrees">Git worktrees</a> &middot;
  <a href="#how-it-works">How it works</a>
</p>

<!-- --- -->

<!-- TODO: Replace with VHS recording -->
<!-- <p align="center">
  <img src="https://vhs.charm.sh/vhs-placeholder.gif" alt="Herd demo" width="720" />
</p> -->

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

| Indicator     | Meaning     |
| ------------- | ----------- |
| `●` (spinner) | Running     |
| `!`           | Needs input |
| `●` (gray)    | Idle        |
| `✓`           | Done        |
| `x`           | Exited      |

**Sessions survive everything** &mdash; Quit herd, close your terminal, reboot your machine. Your Claude Code sessions keep running. Relaunch `herd` and they're all still there.

**Git worktree integration** &mdash; Spin up a session on an isolated branch with `w`. Herd creates the worktree and launches Claude Code in it.

## Keybindings

### Sidebar

| Key       | Action                        |
| --------- | ----------------------------- |
| `j` / `k` | Navigate up/down              |
| `Enter`   | Switch to session             |
| `Space`   | Collapse/expand project       |
| `n`       | New session                   |
| `N`       | New session (pick directory)  |
| `w`       | New session with git worktree |
| `d`       | Delete session                |
| `q`       | Quit (sessions keep running)  |

### Pane navigation

| Key                        | Action                |
| -------------------------- | --------------------- |
| `Ctrl-h` / `Ctrl-Left`    | Focus sidebar         |
| `Ctrl-l` / `Ctrl-Right`   | Focus viewport        |

Mouse click also switches focus. Everything in the viewport passes through to Claude Code.

## Git worktrees

Git normally only lets you have one branch checked out at a time. If you're working on a feature and need to switch to a hotfix, you have to stash or commit your work, switch branches, then switch back when you're done. Git worktrees solve this by letting you check out multiple branches simultaneously, each in its own directory — so you can work on `feature/auth` and `hotfix/login` at the same time without touching each other.

Herd makes this a one-keystroke operation. Press `w`, type a branch name, and herd creates the worktree and launches a Claude Code session in it. When you're done, delete the session with `d` and herd cleans up the worktree too.

### Creating a worktree session

1. Select any session or project header in the sidebar
2. Press `w`
3. Type a branch name (e.g. `feature/auth`) and press Enter

Herd will:

- Create a worktree at `<repo>/.worktrees/<branch>` with that branch checked out
- Launch a new Claude Code session in that directory
- Switch you to the new session immediately

The new session appears in the sidebar under its project with a `⎇` prefix to distinguish it from regular sessions:

```
▼ myproject           (3)
    main session       ●
  ⎇ feature/auth       ●
  ⎇ hotfix/login       ✓
```

If the branch already exists (e.g. a remote branch), herd checks it out. If it doesn't exist, herd creates it from the current HEAD.

### Working in a worktree session

Once created, a worktree session works exactly like any other herd session. Claude Code runs in the worktree directory and sees the branch you specified. Changes you make are completely isolated from your main checkout and from other worktrees.

You can switch between worktree sessions and regular sessions freely — they're all just entries in the sidebar.

### Cleaning up

When you're done with a worktree session, select it and press `d`. Herd will:

1. Kill the Claude Code session
2. Remove the git worktree (`git worktree remove`)
3. Clean up stale worktree references (`git worktree prune`)

The branch itself is **not** deleted — only the worktree directory. If you've merged or pushed the branch, you can delete it through git as you normally would.

> **Note:** If herd is killed unexpectedly, orphaned worktree directories may be left behind in `<repo>/.worktrees/`. You can clean these up manually with `git worktree prune` from the repo root.

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
