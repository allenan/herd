# Herd — Claude Code Manager

A TUI for managing multiple Claude Code sessions and scratch terminals, grouped by project, with input notifications and git worktree support. Ships as a single Go binary via Homebrew.

```
+-------------------+---------------------------------------+
|  v project-1  (3) |                                       |
|    * feature-a    |   $ claude                            |
|    ! feature-b    |   > I've updated the pricing page...  |
|    ● $ shell      |                                       |
|  v project-2  (2) |                                       |
|    * feature-c    |                                       |
|    o feature-d    |                                       |
|  > project-3  (1) |                                       |
|                   |                                       |
|  [n]ew [t]erm [q] |                                       |
+-------------------+---------------------------------------+
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
|  | (tree UI)|  | running claude or $SHELL | |
|  |          |  |                          | |
|  +----------+  +--------------------------+ |
|         |               |                   |
|         |  tmux control protocol (CLI)      |
|         +---------------+                   |
|                                             |
|  ~/.herd/state.json       <- session meta   |
|  tmux capture-pane        <- notifications  |
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
$ herd --profile work    # isolated instance with separate config
```

On first run:
1. Check tmux is installed (error with install instructions if not)
2. Resolve profile (default `~/.herd/` or named `~/.herd/profiles/<name>/`)
3. Create a dedicated tmux server via Unix socket at `~/.herd/tmux.sock`
4. Create the two-pane layout: sidebar (left, 32 chars) + main pane (right, flexible)
5. Sidebar pane runs the Bubble Tea TUI (re-execs itself: `herd --sidebar`)
6. Load state from `~/.herd/state.json` if it exists (recover from previous run)
7. Reconcile state against live tmux panes (adopt orphans, prune dead)

On subsequent runs, if the tmux server is already running, reattach to it.

### 2. User creates a new Claude Code session

User presses `n` (new) in the sidebar. Herd opens a tmux popup (or inline prompt for tmux < 3.2):

```
Directory: ~/code/project-1      (tab-complete, auto-detects project)
```

The session name is auto-derived from Claude Code's terminal title via OSC polling. For worktrees, `w` opens a branch name picker with path preview.

Then Herd does:

```go
func (m *Manager) CreateSession(dir, name string) {
    project := detectProject(dir) // git rev-parse --show-toplevel | basename

    // Create a new tmux window running claude
    paneID := tmux.NewWindow(...)

    // Store metadata
    session := Session{
        ID:         uuid.New(),
        TmuxPaneID: paneID,
        Project:    project,
        Name:       name,
        Dir:        dir,
        CreatedAt:  time.Now(),
        Status:     StatusRunning,
    }
    m.state.Sessions = append(m.state.Sessions, session)
    m.state.Save()
}

func (m *Manager) CreateWorktreeSession(repoRoot, project, branch string) {
    // git worktree add -b <branch> <repo>/.worktrees/<branch>
    wtDir := worktree.Create(repoRoot, branch)

    // Create session in worktree directory
    // Tags session with IsWorktree=true, WorktreeBranch=branch
}
```

### 3. Input detection

Herd polls pane content every 2 seconds via `tmux capture-pane`:

```go
func DetectStatus(content string) Status {
    // Priority order:
    // 1. "esc to interrupt" → Running (Claude is generating)
    // 2. "Do you want to", "[Y/n]", "Allow once", etc. → Input (permission prompt)
    // 3. "? for shortcuts" → Idle (waiting for user task)
    // 4. Running→Idle while not in viewport → Done (finished unattended)
}
```

Additionally, `StatusDone` is set when a session transitions from Running to Idle while not in the viewport — "Claude finished while you weren't looking".

### 4. Switching sessions

`j`/`k` to navigate, `Enter` to switch. Herd swaps the selected session's pane into the viewport:

```go
func (m *Manager) SwitchTo(session Session) {
    viewportPane := m.resolveViewportPane()  // dynamically discover
    tmux.SwapPane(session.TmuxPaneID, viewportPane)
    m.state.LastActiveSession = session.ID
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
▼ project-1                     (4)
    ● feature-a                       ← Running (animated spinner)
  ! feature-b                         ← Needs input
  ● $ shell                           ← Terminal (idle)
  ◉ $ :3000                           ← Terminal (service on port)
▼ project-2                     (1)
  ✓ feature-c                         ← Done
> project-3                     (1)   ← Collapsed
```

Claude status indicators: spinner Running, `!` Needs input, `●` Idle, `✓` Done, `x` Exited/crashed

Terminal status indicators: `●` Shell (idle), spinner Running (command executing), `◉` Service (listening on port), `x` Exited

---

## Project structure

```
herd/
  main.go                    # Entry point, Cobra root command
  cmd/
    root.go                  # `herd` - TUI launch + --sidebar dispatch + profile resolution
    sidebar.go               # `herd --sidebar` - runs Bubble Tea in left pane
    popup.go                 # `herd popup-new` - directory picker popup (hidden)
    popup_worktree.go        # `herd popup-worktree` - branch picker popup (hidden)
  internal/
    profile/
      profile.go             # Profile resolution, config, path management
    tui/
      app.go                 # Top-level Bubble Tea model (normal/prompt modes, popup launching)
      sidebar.go             # Tree view component (project grouping, collapse/expand)
      prompt.go              # Legacy inline prompts (fallback for tmux < 3.2)
      popup.go               # Directory picker + worktree popup models
      dirpicker.go           # Tab-completion utilities
      keybindings.go         # Key mapping definitions
      styles.go              # Lip Gloss theme + status indicator styles
    tmux/
      server.go              # Server bootstrap (profile-aware socket management)
      manager.go             # Session lifecycle (create, switch, kill, reconcile, status refresh)
      capture.go             # Pane capture + status pattern matching + title cleaning + terminal status
      portdetect.go          # TCP port detection for terminal sessions (pgrep + lsof)
      layout.go              # Two-pane layout setup + terminal capabilities
      popup.go               # tmux display-popup helpers (version check, spawn)
    session/
      session.go             # Session struct, status enum, DisplayName()
      store.go               # JSON persistence (~/.herd/state.json) with atomic writes
      project.go             # Git-based project/branch detection
      reconcile.go           # Sync state.json with live tmux state (orphan adoption, pruning)
    worktree/
      worktree.go            # Git worktree create/remove/detect
  go.mod
  go.sum
  Makefile                   # Profile-aware build targets
  README.md
  PLAN.md
  CLAUDE.md
```

---

## State management

`~/.herd/state.json`:

```json
{
  "sessions": [
    {
      "id": "a1b2c3d4",
      "tmux_pane_id": "%5",
      "project": "project-1",
      "name": "feature-a",
      "title": "feature-a",
      "dir": "/home/user/code/project-1",
      "created_at": "2026-02-17T10:30:00Z",
      "status": "running"
    },
    {
      "id": "e5f6g7h8",
      "tmux_pane_id": "%7",
      "project": "project-1",
      "name": "shell",
      "dir": "/home/user/code/project-1",
      "created_at": "2026-02-17T11:00:00Z",
      "status": "service",
      "type": "terminal",
      "service_port": 3000
    }
  ],
  "tmux_socket": "herd",
  "last_active_session": "a1b2c3d4",
  "viewport_pane_id": "%2",
  "sidebar_pane_id": "%1"
}
```

**Reconciliation on startup**: Reads state, queries tmux for live panes. Dead panes get pruned. Orphan claude panes get adopted with best-effort project detection. Worktree sessions auto-tagged.

---

## Git worktree integration

```go
func Create(repoDir, branchName string) (string, error) {
    wtDir := WorktreeDir(repoDir, branchName) // <repo>/.worktrees/<sanitized-branch>
    // Try git worktree add -b <branch> <dir> (new branch)
    // Fallback to git worktree add <dir> <branch> (existing branch)
    return wtDir, nil
}

func Remove(repoDir, wtDir string) error {
    exec.Command("git", "-C", repoDir, "worktree", "remove", "--force", wtDir).Run()
    exec.Command("git", "-C", repoDir, "worktree", "prune").Run()
    return nil
}
```

Deleting a worktree session (`d`): swap replacement into viewport (if active) → kill tmux pane → `git worktree remove --force` → prune → remove from state.

---

## Profiles

Profiles enable multiple isolated herd instances (e.g., personal vs work Claude accounts):

```bash
herd                    # default profile (~/.herd/)
herd --profile work     # named profile (~/.herd/profiles/work/)
```

Each profile gets:
- Its own tmux server and socket
- Its own state.json
- Its own session name (e.g., `herd-work-main`)
- Optionally, a custom `CLAUDE_CONFIG_DIR` (for separate Claude accounts)

Profiles are managed by `internal/profile/profile.go`. Names are validated with `^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$`.

---

## Key bindings

```
Sidebar focused:
  j/k, up/down    Navigate
  Enter            Switch to session / expand project
  n                New session (in current project or pick directory)
  N                New session (always picks directory)
  w                New session with git worktree
  t                New terminal in current project
  d                Delete session
  Space            Collapse/expand project group
  ?                Help overlay
  q                Quit Herd (tmux sessions keep running)

Pane navigation (works from either pane):
  Ctrl-h / Ctrl-Left     Focus sidebar
  Ctrl-l / Ctrl-Right    Focus viewport
  Mouse click            Focus clicked pane
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
        with: { go-version: '1.24' }
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

tmux installed automatically as dependency. Done.

---

## Implementation phases

### Phase 1: Walking skeleton — Complete

**Delivers**: Launch herd, sidebar appears, create sessions, switch between them.

| Task | File(s) | Status |
|------|---------|--------|
| Project init | go.mod, main.go | ✅ Done |
| Tmux server bootstrap | internal/tmux/server.go | ✅ Done |
| Two-pane layout | internal/tmux/layout.go | ✅ Done |
| Session struct + store | internal/session/ | ✅ Done |
| Bubble Tea skeleton | internal/tui/app.go | ✅ Done |
| Sidebar (flat list) | internal/tui/sidebar.go | ✅ Done |
| Create session | internal/tmux/manager.go | ✅ Done |
| Switch session | internal/tmux/manager.go | ✅ Done |
| CLI commands | cmd/root.go, cmd/sidebar.go | ✅ Done |

### Phase 2: Grouping + notifications — Complete

**Delivers**: Project tree in sidebar. Bell when Claude needs input.

| Task | File(s) | Status |
|------|---------|--------|
| Project detection | internal/session/project.go | ✅ Done |
| Tree view | internal/tui/sidebar.go | ✅ Done |
| Pane polling | internal/tmux/capture.go | ✅ Done (every 2s) |
| Status indicators | internal/tui/sidebar.go, styles.go | ✅ Done |
| Title capture | internal/tmux/capture.go | ✅ Done (OSC polling) |
| ~~Hook installer~~ | ~~internal/hooks/installer.go~~ | Deferred — polling works well enough |
| ~~Socket listener~~ | ~~internal/hooks/listener.go~~ | Deferred — nice-to-have for lower latency |
| ~~Notify subcommand~~ | ~~cmd/notify.go~~ | Deferred |
| ~~Desktop notifications~~ | ~~internal/notify/desktop.go~~ | Deferred — users can configure independently in ~/.claude/settings.json |

### Phase 3: Worktrees + polish — Substantially complete

**Delivers**: Git worktree sessions. Full session lifecycle. Popup UIs.

| Task | File(s) | Status |
|------|---------|--------|
| Worktree create/remove | internal/worktree/worktree.go | ✅ Done |
| `w` keybinding | tui/keybindings.go, tui/app.go | ✅ Done |
| Worktree popup UI | tui/popup.go, cmd/popup_worktree.go | ✅ Done |
| Delete session (`d`) | tui/app.go, tmux/manager.go | ✅ Done (includes worktree cleanup) |
| State reconciliation | session/reconcile.go | ✅ Done (orphan adoption, dead pruning, worktree tagging) |
| Popup directory picker | tui/popup.go, cmd/popup.go | ✅ Done (with tab-completion, inline fallback) |
| `N` keybinding (new project) | tui/keybindings.go | ✅ Done |
| Help overlay (`?`) | tui/app.go | ✅ Done |
| ~~Rename session (`r`)~~ | — | Not needed — names auto-derived from Claude Code tab title |
| Fuzzy search (`/`) | tui/sidebar.go | Deferred (nice-to-have) |
| `herd list --json` | cmd/list.go | Deferred |
| `herd new` | cmd/new.go | Deferred |

### Phase 3.5: Profiles — Complete

**Delivers**: Multiple isolated herd instances for different Claude accounts/configs.

| Task | File(s) | Status |
|------|---------|--------|
| Profile struct + resolution | internal/profile/profile.go | ✅ Done |
| Per-profile paths (state, socket, log) | internal/profile/profile.go | ✅ Done |
| Per-profile tmux session name | internal/profile/profile.go | ✅ Done |
| CLAUDE_CONFIG_DIR support | internal/profile/profile.go, tmux/server.go | ✅ Done |
| --profile flag on all commands | cmd/root.go, cmd/sidebar.go, cmd/popup*.go | ✅ Done |
| Makefile PROFILE variable | Makefile | ✅ Done |

### Phase 3.6: Terminals — Complete

**Delivers**: Project-scoped scratch terminals alongside Claude sessions, with smart status detection.

| Task | File(s) | Status |
|------|---------|--------|
| Session type + terminal fields | internal/session/session.go | ✅ Done (`Type`, `ServicePort`, `StatusShell`, `StatusService`) |
| Terminal status detection | internal/tmux/capture.go | ✅ Done (`DetectTerminalStatus`, `shellCommands` map) |
| Port/service detection | internal/tmux/portdetect.go | ✅ Done (process tree walk + lsof) |
| `CreateTerminal()` | internal/tmux/manager.go | ✅ Done (spawns `$SHELL`, no popup needed) |
| `RefreshStatus()` refactor | internal/tmux/manager.go | ✅ Done (split into `refreshClaudeStatus` + `refreshTerminalStatus`) |
| Sidebar rendering | internal/tui/sidebar.go | ✅ Done (`$ ` prefix, sorted after Claude sessions) |
| Service status style | internal/tui/styles.go | ✅ Done (`◉` fisheye, green) |
| `t` keybinding | tui/keybindings.go, tui/app.go | ✅ Done |

### Phase 4: Ship — Partially started

**Delivers**: brew install works. README is good.

| Task | Status |
|------|--------|
| README | ✅ Done (comprehensive, covers features/keybindings/worktrees/profiles) |
| .goreleaser.yml | Not started |
| homebrew-tap repo | Not started |
| Release workflow | Not started |
| --version, --help | Not started |
| Tag v0.1.0 | Not started |

---

## Open questions

1. ~~**tmux prefix key**~~: Resolved — Ctrl-h/Ctrl-Left (sidebar) and Ctrl-l/Ctrl-Right (viewport), plus mouse click. Dedicated socket isolates from user's tmux.

2. **Hook scope**: Global (~/.claude/settings.json) vs per-project. Going with global — one install covers everything, uses $PWD to match sessions. (Hooks not yet implemented — polling is sufficient.)

3. **Session limit / scrolling**: Bubble Tea viewport handles scrolling natively. Fine up to ~100 sessions.

4. ~~**Config file**~~: Resolved — `~/.herd/config.json` stores per-profile config (currently `CLAUDE_CONFIG_DIR`).

5. ~~**Multi-repo worktrees**~~: Resolved — worktrees go in `<repo>/.worktrees/<branch>`. Branch names with `/` are sanitized to `-`.
