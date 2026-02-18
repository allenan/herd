# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is Herd

Herd is a TUI for managing multiple Claude Code sessions, grouped by project. It uses tmux as its PTY multiplexing backend (an implementation detail — users interact only with the herd TUI). Ships as a single Go binary.

## Build & Run

```bash
make build     # builds ./herd binary
make run       # builds + runs
make reload    # builds + hot-swaps sidebar (keeps all sessions alive)
make vet       # go vet ./...
make kill      # kills the herd tmux server
make clean     # removes binary
```

Profile support in the Makefile:
```bash
make run PROFILE=work       # launches with --profile work
make kill PROFILE=work      # kills only work profile's server
make reload PROFILE=work    # hot-swaps work profile's sidebar
```

There are no tests yet. The project uses Go 1.24.

## Architecture

### Two-process model

When the user runs `herd`, the main process:
1. Resolves the profile (default or named via `--profile`)
2. Creates/attaches to a dedicated tmux server via a Unix socket at `~/.herd/tmux.sock` (or `~/.herd/profiles/<name>/tmux.sock` for named profiles)
3. Sets up a two-pane layout in tmux (sidebar 32 chars left, viewport right)
4. The sidebar pane re-executes itself as `herd --sidebar` (with `--profile` if applicable) to run the Bubble Tea TUI
5. Attaches the user's terminal to the tmux session (blocks until detach)

The sidebar subprocess (`herd --sidebar`) runs the interactive TUI. It communicates with tmux to create/switch/kill sessions and persists state to `~/.herd/state.json`.

### Session lifecycle (pane-swapping)

Sessions are tmux windows created with `tmux new-window -d` running either `claude` (Claude sessions) or `$SHELL` (terminal sessions). Both types share the same CRUD, pane-swap, and reconciliation lifecycle — distinguished by a `Type` field on the Session struct (`""` for Claude, `"terminal"` for terminals). Switching sessions uses `tmux swap-pane` to move the selected session's pane into the viewport position. The viewport pane is resolved dynamically via `resolveViewportPane()` rather than relying on a stored pane ID.

### Key packages

- **`cmd/`** — Cobra commands. `root.go` handles main launch + `--sidebar` flag dispatch + profile resolution. `sidebar.go` starts the Bubble Tea program. `popup.go` and `popup_worktree.go` are hidden subcommands spawned by the TUI for tmux `display-popup` flows.
- **`internal/tmux/`** — All tmux interaction. `server.go` manages the socket/server lifecycle (profile-aware). `layout.go` creates the two-pane split with terminal capability negotiation. `manager.go` contains session CRUD (create, switch, kill) using pane-swap strategy — includes `CreateTerminal()` for terminal sessions and split `RefreshStatus()` dispatching to `refreshClaudeStatus()`/`refreshTerminalStatus()`. `capture.go` handles pane content/title capture, Claude status detection, and terminal status detection (`DetectTerminalStatus`). `portdetect.go` detects listening TCP ports via process tree walking (`pgrep`) + `lsof`. `popup.go` provides tmux `display-popup` helpers.
- **`internal/tui/`** — Bubble Tea UI. `app.go` is the top-level model with normal/prompt modes, popup launching, and help overlay. `sidebar.go` renders the tree view with project grouping. `prompt.go` handles legacy inline prompts (fallback for tmux < 3.2). `popup.go` contains the directory picker and worktree branch picker popup models. `dirpicker.go` provides tab-completion utilities. `keybindings.go` and `styles.go` define key mappings and lipgloss styling.
- **`internal/session/`** — Data types and persistence. `store.go` handles JSON state read/write with atomic rename and backup. `project.go` detects git project name and branch from a directory. `reconcile.go` adopts orphan tmux panes and prunes dead sessions.
- **`internal/profile/`** — Profile management. `profile.go` resolves profile by name, creates directory structure, and manages per-profile config (including `CLAUDE_CONFIG_DIR`).
- **`internal/worktree/`** — Git worktree operations. `worktree.go` wraps `git worktree add/remove` with branch handling and path sanitization.

### State files

Default profile paths (named profiles use `~/.herd/profiles/<name>/` instead):
- `~/.herd/state.json` — Session metadata, pane IDs, active session
- `~/.herd/state.json.bak` — Backup created on every save (used for recovery)
- `~/.herd/state.json.corrupt.<timestamp>` — Archived corrupt state files (for inspection)
- `~/.herd/tmux.sock` — Tmux socket (dedicated server, isolated from user's tmux)
- `~/.herd/debug.log` — Debug logging from the manager
- `~/.herd/config.json` — Profile config (currently stores `CLAUDE_CONFIG_DIR`)

### tmux interaction pattern

The codebase uses two methods to talk to tmux:
- **`gotmux` client** (`*gotmux.Tmux`) — for queries (list windows/panes, get session by name)
- **`TmuxRun()` helper** — for mutations (new-window, swap-pane, kill-pane, select-pane). This is necessary because it strips `$TMUX` from the environment, which gotmux doesn't handle when running inside a herd tmux pane.

### Popup communication pattern

New-session and worktree flows use tmux `display-popup` to spawn a separate `herd popup-new` or `herd popup-worktree` process. The popup writes its result (directory, project, branch) to a JSON file at a known path. The main TUI polls for this file every 200ms (`popupCheckMsg`). If tmux < 3.2 (no popup support), falls back to inline prompts.

### Import alias

The `internal/tmux` package is imported as `htmux` in `cmd/` to avoid collision with the `gotmux` package name.

## Status indicators

**Claude sessions**: Animated spinner (running), `!` (needs input), `●` (idle), `✓` (done), `x` (exited).

**Terminal sessions**: `●` gray (shell idle), animated spinner (command running), `◉` green (service listening on a TCP port), `x` (exited).

All defined as styled strings in `internal/tui/styles.go`. Terminal status is detected via `pane_current_command` (process name) rather than content pattern-matching. Port detection walks the pane's process tree with `pgrep` and checks `lsof` for TCP LISTEN sockets.

## Key invariants

Window 0 must always have exactly two panes: sidebar (left) + viewport (right). Operations that could break this invariant (killing panes, quitting) need special care:
- To replace viewport content without destroying the pane, use `respawn-pane -k` (not kill + recreate). `split-window -l` sizes are relative to the pane being split, not the window.
- When killing a session that's in the viewport, swap the replacement in first, then kill the old pane.
- The sidebar subprocess must stay alive across detach/re-attach cycles (no `tea.Quit` on detach). If the sidebar pane is destroyed, `HasLayout` fails and `SetupLayout` re-runs.
- The tmux socket is at `~/.herd/tmux.sock`. Always use `-S path` (not `-L name`) in raw tmux commands — `-L herd` targets a different socket in `/tmp` and silently does nothing.
- Pane sizes set via `split-window -l` on a detached session are proportionally scaled when a client attaches. Use tmux hooks (`client-attached`, `client-resized`) to enforce fixed pane widths (sidebar pinned to 32 chars).
- When debugging visual anomalies (duplicate UI, wrong sizes), check `ps aux | grep herd` for orphan processes first — `make kill` failures silently leave servers and sidebars running.

## State resilience

Sessions survive any state failure. Defense in depth:

1. **Atomic writes** — `Save()` writes to `.tmp` then renames (existing).
2. **Backup on every save** — `state.json` is copied to `state.json.bak` before each write (read+write, not rename, so the primary is never missing).
3. **Graceful corrupt recovery** — `LoadState()` falls back: primary → `.bak` → empty state. Corrupt files are archived as `state.json.corrupt.<timestamp>`.
4. **Reconciliation** — `Manager.Reconcile()` compares state against live tmux panes. Prunes dead sessions, adopts orphan claude panes (detected via `CurrentCommand`/`StartCommand` containing "claude"), auto-tags worktree sessions. Runs on startup and every 2s in the polling loop.
5. **`reloadState()`** — re-reads state from disk and prunes sessions with dead panes before every manager operation (existing).

Net effect: deleting `state.json` while herd is running recovers all sessions within 2s.

## Implementation status

See `PLAN.md` for the full roadmap. Current status by phase:

**Phase 1 (Walking skeleton) — Complete.** Tmux server bootstrap, two-pane layout, session CRUD, sidebar TUI, JSON state persistence, `herd` / `herd --sidebar` commands.

**Phase 2 (Grouping + notifications) — Complete.** Project detection, tree view with collapse/expand, pane polling via `capture-pane` every 2s, status indicators, title capture via OSC escape sequences. Deferred (nice-to-have): `internal/hooks/` (installer + socket listener), `internal/notify/` (desktop notifications), `cmd/notify.go` (`herd notify` subcommand) — polling works well enough and desktop notifications can be configured independently in `~/.claude/settings.json`.

**Phase 3 (Worktrees + polish) — Substantially complete.** Done: git worktree create/remove (`internal/worktree/`), `CreateWorktreeSession()` with cleanup on `KillSession()`, worktree popup UI (`w` key), worktree session tagging (`IsWorktree`/`WorktreeBranch` on Session struct), auto-tagging in `Reconcile()`, delete session (`d` key), state reconciliation (orphan adoption, dead session pruning, backup/corrupt recovery), help overlay (`?` key). Not needed: rename session (names auto-derived from Claude Code tab title). Deferred: `/` (fuzzy search), `herd list --json`, `herd new`.

**Phase 3.5 (Profiles) — Complete.** Multiple isolated herd instances via `--profile <name>`. Each profile gets its own tmux server, state file, socket, and optionally a custom `CLAUDE_CONFIG_DIR`. Default (no profile flag) is backward compatible with `~/.herd/`.

**Phase 3.6 (Terminals) — Complete.** Project-scoped terminal sessions (`t` key). Session struct extended with `Type` field (`""`/`"terminal"`) and `ServicePort`. Terminal status detected via `pane_current_command` (shell idle vs command running) + port detection via `pgrep`/`lsof` (service listening). Sidebar shows terminals after Claude sessions with `$ ` prefix and total counts. No popup needed — instant shell creation in project directory. Orphan terminals are not auto-adopted (indistinguishable from random tmux panes); dead panes still pruned by existing logic.

**Phase 4 (Ship) — Partially started.** Done: `README.md` (comprehensive, includes features, keybindings, worktrees, profiles). Missing: `.goreleaser.yml`, `.github/workflows/`, Homebrew tap, `--version` flag.

### Divergences from PLAN.md

- **State path**: Plan says `~/.config/herd/`, implementation uses `~/.herd/`.
- **Session naming**: Plan had a 3-step prompt (dir → name → worktree). Implementation uses a popup directory picker with tab-completion and auto-naming via OSC terminal title polling (`CapturePaneTitle` + `CleanPaneTitle`).
- **Extra status**: `StatusDone` (`✓`, cyan) was added beyond the plan's four states — represents "Claude finished while you weren't looking".
- **Status glyphs differ from plan**: Plan said `o` for idle, implementation uses `●` (gray). Plan said `*` for running, implementation uses an animated spinner.
- **Popup UI**: Plan had inline prompts. Implementation uses tmux `display-popup` with a fallback to inline prompts for tmux < 3.2.
- **Profiles**: Not in the original plan. Fully implemented as Phase 3.5 — enables multi-account/multi-config isolated instances.
- **Terminals**: Not in the original plan. Fully implemented as Phase 3.6 — project-scoped scratch terminals with smart status detection (idle/running/service).
