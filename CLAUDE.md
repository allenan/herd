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

There are no tests yet. The project uses Go 1.24.

## Architecture

### Two-process model

When the user runs `herd`, the main process:
1. Creates/attaches to a dedicated tmux server via a Unix socket at `~/.herd/tmux.sock`
2. Sets up a two-pane layout in tmux (20% sidebar, 80% viewport)
3. The sidebar pane re-executes itself as `herd --sidebar` to run the Bubble Tea TUI
4. Attaches the user's terminal to the tmux session (blocks until detach)

The sidebar subprocess (`herd --sidebar`) runs the interactive TUI. It communicates with tmux to create/switch/kill sessions and persists state to `~/.herd/state.json`.

### Session lifecycle (pane-swapping)

Sessions are tmux windows created with `tmux new-window -d` running `claude`. Switching sessions uses `tmux swap-pane` to move the selected session's pane into the viewport position. The `ViewportPaneID` in state tracks which pane currently occupies the right side.

### Key packages

- **`cmd/`** — Cobra commands. `root.go` handles main launch + `--sidebar` flag dispatch. `sidebar.go` starts the Bubble Tea program.
- **`internal/tmux/`** — All tmux interaction. `server.go` manages the socket/server lifecycle. `layout.go` creates the two-pane split. `manager.go` contains session CRUD (create, switch, kill) using pane-swap strategy.
- **`internal/tui/`** — Bubble Tea UI. `app.go` is the top-level model with normal/prompt modes. `sidebar.go` renders the session list. `prompt.go` handles inline new-session creation (dir → name two-step flow).
- **`internal/session/`** — Data types and persistence. `store.go` handles JSON state read/write with atomic rename and backup. `project.go` detects git project name and branch from a directory. `reconcile.go` adopts orphan tmux panes and prunes dead sessions.

### State files

- `~/.herd/state.json` — Session metadata, pane IDs, active session
- `~/.herd/state.json.bak` — Backup created on every save (used for recovery)
- `~/.herd/state.json.corrupt.<timestamp>` — Archived corrupt state files (for inspection)
- `~/.herd/tmux.sock` — Tmux socket (dedicated server, isolated from user's tmux)
- `~/.herd/debug.log` — Debug logging from the manager

### tmux interaction pattern

The codebase uses two methods to talk to tmux:
- **`gotmux` client** (`*gotmux.Tmux`) — for queries (list windows/panes, get session by name)
- **`TmuxRun()` helper** — for mutations (new-window, swap-pane, kill-pane, select-pane). This is necessary because it strips `$TMUX` from the environment, which gotmux doesn't handle when running inside a herd tmux pane.

### Import alias

The `internal/tmux` package is imported as `htmux` in `cmd/` to avoid collision with the `gotmux` package name.

## Status indicators

`*` running, `!` needs input, `o` idle, `x` exited — defined as styled strings in `internal/tui/styles.go`.

## Key invariants

Window 0 must always have exactly two panes: sidebar (left) + viewport (right). Operations that could break this invariant (killing panes, quitting) need special care:
- To replace viewport content without destroying the pane, use `respawn-pane -k` (not kill + recreate). `split-window -l` sizes are relative to the pane being split, not the window.
- When killing a session that's in the viewport, swap the replacement in first, then kill the old pane.
- The sidebar subprocess must stay alive across detach/re-attach cycles (no `tea.Quit` on detach). If the sidebar pane is destroyed, `HasLayout` fails and `SetupLayout` re-runs.
- The tmux socket is at `~/.herd/tmux.sock`. Always use `-S path` (not `-L name`) in raw tmux commands — `-L herd` targets a different socket in `/tmp` and silently does nothing.
- Pane sizes set via `split-window -l` on a detached session are proportionally scaled when a client attaches. Use tmux hooks (`client-attached`, `client-resized`) to enforce fixed pane widths.
- When debugging visual anomalies (duplicate UI, wrong sizes), check `ps aux | grep herd` for orphan processes first — `make kill` failures silently leave servers and sidebars running.

## State resilience

Sessions survive any state failure. Defense in depth:

1. **Atomic writes** — `Save()` writes to `.tmp` then renames (existing).
2. **Backup on every save** — `state.json` is copied to `state.json.bak` before each write (read+write, not rename, so the primary is never missing).
3. **Graceful corrupt recovery** — `LoadState()` falls back: primary → `.bak` → empty state. Corrupt files are archived as `state.json.corrupt.<timestamp>`.
4. **Reconciliation** — `Manager.Reconcile()` compares state against live tmux panes. Prunes dead sessions, adopts orphan claude panes (detected via `CurrentCommand`/`StartCommand` containing "claude"). Runs on startup and every 2s in the polling loop.
5. **`reloadState()`** — re-reads state from disk and prunes sessions with dead panes before every manager operation (existing).

Net effect: deleting `state.json` while herd is running recovers all sessions within 2s.

## Implementation status

See `PLAN.md` for the full roadmap. Current status by phase:

**Phase 1 (Walking skeleton) — Complete.** Tmux server bootstrap, two-pane layout, session CRUD, sidebar TUI, JSON state persistence, `herd` / `herd --sidebar` commands.

**Phase 2 (Grouping + notifications) — Effectively complete.** Done: project detection, tree view with collapse/expand, pane polling via `capture-pane` every 2s, status indicators. Deferred (nice-to-have): `internal/hooks/` (installer + socket listener), `internal/notify/` (desktop notifications), `cmd/notify.go` (`herd notify` subcommand) — the hook-based approach would be faster/more reliable than polling, but polling works well enough and desktop notifications can be configured independently in `~/.claude/settings.json`.

**Phase 3 (Worktrees + polish) — Partially complete.** Done: delete session (`d` key, cleans up pane + state), state reconciliation (orphan adoption, dead session pruning, backup/corrupt recovery). Not needed: rename session (names auto-derived from Claude Code tab title). Missing: `internal/worktree/`, `cmd/new.go`, `cmd/list.go`, `cmd/cleanup.go`, keybindings for `w` (worktree), `/` (fuzzy search), `?` (help). Nice-to-have: delete confirmation prompt.

**Phase 4 (Ship) — Not started.** Missing: `.goreleaser.yml`, `.github/workflows/`, `README.md`, Homebrew tap, `--version` flag.

### Divergences from PLAN.md

- **State path**: Plan says `~/.config/herd/`, implementation uses `~/.herd/`.
- **Session naming**: Plan had a 3-step prompt (dir → name → worktree). Implementation uses a single-step directory prompt with auto-naming via OSC terminal title polling (`CapturePaneTitle` + `CleanPaneTitle`).
- **Extra status**: `StatusDone` (`✓`, blue) was added beyond the plan's four states — represents "Claude finished while you weren't looking".
- **Status glyphs differ from plan**: Plan said `o` for idle, implementation uses `●` (gray). Plan said `*` for running, implementation uses an animated spinner.
- **No worktree fields on Session struct** — `IsWorktree`/`WorktreeBranch` omitted since worktrees aren't implemented yet.
