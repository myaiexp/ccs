# Tabbed Session Wrapper — Debugging Done

> Tabbed session wrapper architecture implemented and debugged. 2026-03-21.

## Bugs Fixed

1. **TUI lag** — `handleRefresh()` was synchronous, blocking bubbletea's Update loop. Fixed with `asyncRefreshCmd()` goroutine + `RefreshDoneMsg`. Pane captures batched into single message per tick (was N messages). Tick intervals increased to 3s. Conv text loading made async via `ConvTextLoadMsg`.

2. **tmux keybindings** — `parseBindingLine()` destroyed quoted strings via `strings.Fields()`. Added persistent `original-bindings.json` for crash recovery, `filterCCSBindings()` to detect stale if-shell bindings, raw command preservation in parser.

3. **Status line not visible** — `SetStatusLines/SetStatusFormat` used `-s` (server scope instead of session). Also `status 1` is invalid in tmux (only accepts `on`/`off`/`2`-`5`).

4. **Multi-line active rows** — restored `statusFadeColors`, `maxActiveStatusLines()`, variable `activeRowLines()`. Kept attention badges.

5. **Attention detection** — `stripStatusBar` cut from FIRST separator (removing CC's `❯` prompt). Changed to cut from LAST separator, preserving the prompt for idle detection. Also wired `UpdateAttention()` from `PaneCaptureMsg` handler to tab manager.

6. **Crash safety** — `ScanAndAdopt()` removed (was moving windows destructively). Added `cleanupStaleCCSState()` on startup and `ccs cleanup` subcommand for manual recovery.

## Remaining Known Issues

- Keybindings (`§ space`, `§ 1`, `§ 2`) not yet verified working in practice
- Window adoption removed — sessions from other tmux sessions won't appear as tabs (they still show as active in dashboard)
- User considering rewrite in another language
