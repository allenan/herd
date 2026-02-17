# Herd — Claude Code Manager

A TUI for managing multiple Claude Code sessions, grouped by project, with input notifications and git worktree support. Ships as a single Go binary via Homebrew.

```
+----------------+---------------------------------------+
|  v omix        |                                       |
|    * refactor  |   $ claude                            |
|    ! mktg      |   > I've updated the pricing page...  |
|  v mtrail      |                                       |
|    * combat    |                                       |
|    o tiles     |                                       |
|  > helium      |                                       |
|                |                                       |
|  [n]ew [q]uit  |                                       |
+----------------+---------------------------------------+
```

---

## Architecture

### Single binary, tmux backend

Herd is a **single Go binary** that orchestrates everything. It uses **tmux** as its PTY multiplexing backend — not as something the user interacts with directly, but as plumbing. The user launches `herd` and gets the full TUI experience; tmux is an implementation detail.

```
+---------------------------------------------+
|  herd binary (Go / Bubble Tea)               |
|  +----------+  +--------------------------+ |
|  | Sidebar  |  | Active pane (tmux pane)  | |
|  | (tree UI)|  | running `claude`         | |
|  |          |  |                          | |
|  +----------+  +--------------------------+ |
|         |               |                   |
|         |  tmux control protocol (CLI)      |
|         +---------------+                   |
|                                             |
|  ~/.config/herd/state.json  <- session meta  |
|  Claude Code hooks         <- notifications |
+---------------------------------------------+
```

### Why tmux (and not raw PTYs or Zellij)?

- **PTY multiplexing is hard** — tmux handles resize, detach/reattach, scroll, and signal forwarding. Reimplementing this is months of work.
- **`tmux capture-pane`** gives us screen-scraping for free as a fallback.
- **Detach/reattach** means sessions survive `herd` crashes. You can kill the TUI, restart it, and all your Claude sessions are still running.
- **Zellij plugins** (WASM) can't programmatically control other panes yet. Their plugin API is still maturing.
- **tmux is ubiquitous** — already installed or trivially installable on macOS/Linux.

### Key dependency: `gotmux`

[`gotmux`](https://github.com/GianlucaP106/gotmux) is a comprehensive Go library for controlling tmux programmatically — sessions, windows, panes, option getting/setting. Wraps the tmux CLI with type-safe Go structs.

---

## How it works, end to end

### 1. User runs `herd`

```bash
$ herd
```

On first run:
1. Check tmux is installed (error with install instructions if not)
2. Create a dedicated tmux server: `tmux -L herd new-session -d -s herd-main`
   - Using `-L herd` gives us our own tmux socket, isolated from the user's normal tmux usage
3. Create the two-pane layout: sidebar (left, 20%) + main pane (right, 80%)
4. Sidebar pane runs the Bubble Tea TUI (re-execs itself: `herd --sidebar`)
5. Main pane is empty, waiting for session selection or creation
6. Load state from `~/.config/herd/state.json` if it exists (recover from previous run)

On subsequent runs, if the tmux server `herd` is already running, reattach to it.

### 2. User creates a new Claude Code session

User presses `n` (new) in the sidebar. Herd prompts inline:

```
Directory: ~/code/omix      (default: cwd, tab-complete)
Name: marketing-page        (default: current git branch)
Worktree? [y/N]
```

Then Herd does:

```go
func (m *Manager) CreateSession(dir, name string, worktree bool) {
    project := detectProject(dir) // git rev-parse --show-toplevel | basename

    if worktree {
        wtDir := filepath.Join(dir, ".worktrees", name)
        exec("git", "worktree", "add", "-b", name, wtDir)
        dir = wtDir
    }

    // Create a new tmux window in our server
    windowID := tmux.NewWindow(tmux.WindowOpts{
        Name:           fmt.Sprintf("%s/%s", project, name),
        StartDirectory: dir,
        Command:        "claude",
    })

    // Store metadata
    session := Session{
        ID:         uuid.New(),
        WindowID:   windowID,
        Project:    project,
        Name:       name,
        Dir:        dir,
        IsWorktree: worktree,
        CreatedAt:  time.Now(),
        Status:     StatusRunning,
    }
    m.state.Sessions = append(m.state.Sessions, session)
    m.state.Save()

    m.installHookIfNeeded()
}
```

### 3. Input detection (the bell icon)

Two complementary approaches — Herd uses both:

#### Primary: Claude Code Notification Hook

Claude Code has a built-in `Notification` hook event that fires when Claude is waiting for user input (permission prompts, idle prompts, questions). Herd installs a global hook that writes to a **Unix domain socket** that the Herd process listens on.

Installed by `herd hook install` into `~/.claude/settings.json`:

```json
{
  "hooks": {
    "Notification": [
      {
        "type": "command",
        "command": "herd notify",
        "timeout": 5
      }
    ]
  }
}
```

`herd notify` is a subcommand in the same binary:

```go
func notifyCmd() {
    input := readStdin()  // Claude Code pipes JSON with session info
    parsed := parseHookInput(input)

    conn, _ := net.Dial("unix", socketPath())
    msg := NotifyMessage{
        Type:    parsed.Type, // "permission" | "idle_prompt" | "ask_user"
        Dir:     parsed.SessionCwd,
        Message: parsed.Title,
    }
    json.NewEncoder(conn).Encode(msg)
}
```

Herd matches the notification to a session by working directory and shows the bell icon. Response time: near-instant.

#### Fallback: Screen scraping via `tmux capture-pane`

For resilience, Herd also polls pane content every 2 seconds:

```go
func needsInput(output string) bool {
    patterns := []string{
        "Do you want to",
        "[Y/n]", "[y/N]",
        "Allow once", "Allow always",
    }
    for _, p := range patterns {
        if strings.Contains(output, p) {
            return true
        }
    }
    return false
}
```

### 4. Switching sessions

`j`/`k` to navigate, `Enter` to switch. Herd tells tmux to display the selected window:

```go
func (m *Manager) switchTo(session Session) {
    tmux.SelectWindow(session.WindowID)
    m.activeSession = session.ID
    session.Status = StatusRunning // clear notification
    m.state.Save()
}
```

### 5. Auto-grouping by project

Derived from git automatically:

```go
func detectProject(dir string) string {
    out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
    if err != nil {
        return filepath.Base(dir)
    }
    return filepath.Base(strings.TrimSpace(string(out)))
}
```

Sidebar tree:
```
v omix           (2 sessions)
  * refactor     Running
  ! marketing   Needs input
v monster-trail  (2 sessions)
  * combat       Running
  o tiles        Idle
> helium         (1 session, collapsed)
```

Status indicators: `*` Running, `!` Needs input, `o` Idle, `x` Exited/crashed

---

## Project structure

```
herd/
  main.go                    # Entry point, Cobra root command
  cmd/
    root.go                  # `herd` - TUI launch (default)
    sidebar.go               # `herd --sidebar` - runs Bubble Tea in left pane
    notify.go                # `herd notify` - hook callback subcommand
    new.go                   # `herd new` - create session headlessly
    list.go                  # `herd list` - list sessions for scripting
    hook.go                  # `herd hook install|uninstall`
    cleanup.go               # `herd cleanup` - prune dead sessions
  internal/
    tui/
      app.go                 # Top-level Bubble Tea model
      sidebar.go             # Tree view component
      prompt.go              # Inline prompts (new session, rename)
      keybindings.go         # Key mapping definitions
      styles.go              # Lip Gloss theme
    tmux/
      server.go              # Server bootstrap (create/find -L herd server)
      manager.go             # Window lifecycle (create, kill, select)
      capture.go             # Pane capture + input pattern matching
      layout.go              # Two-pane layout setup
    session/
      session.go             # Session struct, status enum
      store.go               # JSON persistence (~/.config/herd/state.json)
      project.go             # Git-based project detection
      reconcile.go           # Sync state.json with live tmux state
    worktree/
      worktree.go            # Git worktree create/remove
      cleanup.go             # Orphan worktree pruning
    hooks/
      installer.go           # Read/modify ~/.claude/settings.json
      listener.go            # Unix socket server for hook notifications
    notify/
      desktop.go             # OS-native notifications (osascript / notify-send)
  .goreleaser.yml
  .github/workflows/
    release.yml              # GoReleaser on tag push
  go.mod
  go.sum
  README.md
```

---

## State management

`~/.config/herd/state.json`:

```json
{
  "sessions": [
    {
      "id": "a1b2c3d4",
      "tmux_window_id": "@3",
      "project": "omix",
      "name": "marketing-page",
      "dir": "/Users/andrew/code/omix",
      "is_worktree": false,
      "worktree_branch": "",
      "created_at": "2026-02-17T10:30:00Z",
      "status": "running"
    }
  ],
  "tmux_socket": "herd",
  "last_active_session": "a1b2c3d4"
}
```

**Reconciliation on startup**: Reads state, queries tmux for live windows. Dead windows get marked exited. Orphan windows get adopted with best-effort project detection.

---

## Git worktree integration

```go
func CreateWorktree(repoDir, branchName string) (string, error) {
    wtDir := filepath.Join(repoDir, ".worktrees", branchName)
    cmd := exec.Command("git", "-C", repoDir, "worktree", "add", "-b", branchName, wtDir)
    if err := cmd.Run(); err != nil {
        return "", fmt.Errorf("git worktree add: %w", err)
    }
    return wtDir, nil
}

func RemoveWorktree(repoDir, wtDir string) error {
    exec.Command("git", "-C", repoDir, "worktree", "prune").Run()
    return exec.Command("git", "-C", repoDir, "worktree", "remove", wtDir).Run()
}
```

Deleting a worktree session (`d`): kill tmux window -> `git worktree remove` -> prompt to delete branch -> remove from state.

---

## Key bindings

```
Sidebar focused:
  j/k, up/down    Navigate
  Enter            Switch to session (focuses main pane)
  n                New session
  w                New session with git worktree
  d                Delete session (confirmation prompt)
  r                Rename session
  /                Fuzzy search
  Space            Collapse/expand project group
  ?                Help overlay
  q                Quit Herd (tmux sessions keep running)

Main pane focused:
  Ctrl-]           Return focus to sidebar
                   (only Herd keybinding - everything else passes to Claude Code)
```

---

## Packaging and distribution

### GoReleaser + Homebrew Tap

```yaml
# .goreleaser.yml
project_name: herd

builds:
  - main: ./main.go
    binary: herd
    goos: [darwin, linux]
    goarch: [amd64, arm64]
    ldflags:
      - -s -w
      - -X main.version={{.Version}}
      - -X main.commit={{.Commit}}

brews:
  - repository:
      owner: yourusername
      name: homebrew-tap
    directory: Formula
    homepage: "https://github.com/yourusername/herd"
    description: "TUI manager for multiple Claude Code sessions"
    license: "MIT"
    dependencies:
      - name: tmux
    install: |
      bin.install "herd"
    caveats: |
      Quick start:
        herd                # launch the TUI
        herd new            # create a session from any directory
        herd hook install   # set up Claude Code notifications
```

### GitHub Actions (.github/workflows/release.yml)

```yaml
name: Release
on:
  push:
    tags: ['v*']
jobs:
  release:
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: actions/setup-go@v5
        with: { go-version: '1.22' }
      - uses: goreleaser/goreleaser-action@v6
        with: { args: 'release --clean' }
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### End user experience

```bash
brew tap yourusername/tap
brew install herd
herd
```

tmux installed automatically as dependency. First run auto-installs Claude Code hook. Done.

---

## Implementation phases

### Phase 1: Walking skeleton (3-4 days)

**Delivers**: Launch herd, sidebar appears, create sessions, switch between them.

| Task | File(s) | Notes |
|------|---------|-------|
| Project init | go.mod, main.go | bubbletea, lipgloss, bubbles, cobra, gotmux |
| Tmux server bootstrap | internal/tmux/server.go | Create/find `-L herd` server |
| Two-pane layout | internal/tmux/layout.go | Left 20% sidebar, right 80% main |
| Session struct + store | internal/session/ | JSON read/write to ~/.config/herd/ |
| Bubble Tea skeleton | internal/tui/app.go | Model with sidebar list, status bar |
| Sidebar (flat list) | internal/tui/sidebar.go | No grouping yet |
| Create session | internal/tmux/manager.go | New tmux window running `claude` |
| Switch session | internal/tmux/manager.go | select-window on Enter |
| CLI commands | cmd/root.go, cmd/sidebar.go | herd launches tmux, herd --sidebar runs TUI |

**Test**: Open herd, press `n` three times in different repos, j/k + Enter to switch.

### Phase 2: Grouping + notifications (2-3 days)

**Delivers**: Project tree in sidebar. Bell when Claude needs input.

| Task | File(s) | Notes |
|------|---------|-------|
| Project detection | internal/session/project.go | git rev-parse --show-toplevel |
| Tree view | internal/tui/sidebar.go | Group by project, collapse/expand |
| Pane polling | internal/tmux/capture.go | capture-pane every 2s, pattern match |
| Hook installer | internal/hooks/installer.go | Merge into ~/.claude/settings.json |
| Socket listener | internal/hooks/listener.go | Listen on /tmp/herd.sock |
| Notify subcommand | cmd/notify.go | Read stdin JSON, write to socket |
| Status indicators | internal/tui/sidebar.go | Based on session state |
| Desktop notifications | internal/notify/desktop.go | osascript on mac, notify-send on linux |

**Test**: Two sessions. Trigger permission prompt in one. Bell appears in sidebar within 1s. Desktop notification fires. Switch to it, approve, bell clears.

### Phase 3: Worktrees + polish (2-3 days)

**Delivers**: Git worktree sessions. Full session lifecycle. Fuzzy search.

| Task | File(s) | Notes |
|------|---------|-------|
| Worktree create/remove | internal/worktree/ | git worktree add/remove |
| `w` keybinding | keybindings.go | New-with-worktree flow |
| Delete session (`d`) | tui/prompt.go | Confirmation, worktree cleanup |
| Rename session (`r`) | tui/prompt.go | Inline text input |
| Fuzzy search (`/`) | tui/sidebar.go | Filter tree with text input |
| State reconciliation | session/reconcile.go | Sync state.json with live tmux |
| `herd list --json` | cmd/list.go | For status bar integration |
| `herd new` | cmd/new.go | Headless session creation |

**Test**: Create worktree session, verify isolated branch. Delete, verify cleanup. Kill herd, restart, sessions reconnect.

### Phase 4: Ship (1-2 days)

**Delivers**: brew install works. README is good.

| Task | Notes |
|------|-------|
| .goreleaser.yml | Test with `goreleaser release --snapshot` |
| homebrew-tap repo | Create on GitHub |
| Release workflow | .github/workflows/release.yml |
| README | Usage, screenshots, demo GIF (use `vhs` for recording) |
| First-run experience | Auto hook install, welcome text |
| --version, --help | Polish |
| Tag v0.1.0 | Push, verify full pipeline |

---

## Open questions

1. **tmux prefix key**: Dedicated socket isolates from user's tmux, but still need sidebar/main switching. Plan: Ctrl-]. Configurable.

2. **Hook scope**: Global (~/.claude/settings.json) vs per-project. Going with global — one install covers everything, uses $PWD to match sessions.

3. **Session limit / scrolling**: Bubble Tea viewport handles scrolling natively. Fine up to ~100 sessions.

4. **Config file**: Skip at Phase 1. Add ~/.config/herd/config.toml when customization is requested.

5. **Multi-repo worktrees**: Default puts worktrees in `<repo>/.worktrees/`. Make configurable later.
