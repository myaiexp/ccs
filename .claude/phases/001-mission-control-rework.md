# AI Status Summaries & UX Polish — In Progress

> Added AI-powered periodic status summaries for active sessions, 2-column detail pane, cursor fixes, and stable ordering. 2026-03-19.

## What Changed

### AI Status Summaries (new)
- Periodic haiku calls (every 2 min) generate one-line status summaries for active sessions
- Up to 5 summaries displayed per session with fading colors (newest = brightest)
- Content source: pane capture preferred, JSONL conversation text fallback
- Skips haiku call if input unchanged (no wasted API calls for idle sessions)
- `N` key manually triggers status summary on selected session
- All haiku calls logged to `~/.cache/ccs/naming.log` (prompt, response, timing, errors)
- First summary fires 5s after startup (not 2 min)

### Transition Summaries (new)
- When session goes inactive: haiku condenses status history → short name + comprehensive multi-line summary
- Comprehensive summary shown in detail pane left column
- Falls back to JSONL conversation text if no status history exists

### Detail Pane Rework
- 2-column layout: left = AI summary, right = conversation text (no tool calls)
- Conversation text uses `›` (cyan) for user, `»` (purple) for assistant — no verbose "Claude:" prefix
- `ExtractConversationText()` in activity.go extracts only text blocks from JSONL

### UX Fixes
- `▸` cursor indicator on selected rows (active, done, search results)
- Active rows don't pad to 2 lines when empty — just the header
- Active section order is stable (no re-sorting on refresh, new sessions insert at top)
- Open section detail pane properly truncates both columns

### Security Fix
- Old naming sent raw JSONL (including settings.json with API keys) to haiku
- All paths now use `statusContent()` which extracts conversation text only

## Known Issues / Tech Debt

### Pane capture reliability
- `TICK active=N with_pane=0` is common — active sessions frequently lack pane content
- Root cause: tracker doesn't always have tmux window IDs for sessions started outside ccs
- JSONL conversation text fallback mitigates but is lower quality than pane capture
- Needs investigation: why does pane content disappear after initial capture?

### AI summary quality
- Haiku sometimes produces vague summaries ("Working on code improvements")
- Prompt tuning needed — current prompt is generic
- Status summaries from JSONL conversation text are lower quality than from pane capture
- No retry on SKIP — if first summary fails, nothing until next content change

### Cursor/navigation feel
- User reports "getting lost" — stable ordering helps but may not fully solve
- `▸` cursor is small; no background highlight (lipgloss ANSI nesting prevents it)
- No visual feedback when list reorders (sessions appearing/disappearing)
- Follow mode cursor in compressed list doesn't have `▸`

### Detail pane
- Right column (conversation text) can be empty for sessions with only tool calls
- Summary column shows "No summary yet" until transition — no in-progress summary for open sessions
- Body height calculation is fixed, not responsive to terminal height changes

### State model
- `status_history` grows unbounded in state.json (capped at 20 per session in memory, but all persisted)
- No cleanup of status history for deleted sessions
- `lastSummaryInput` map in model isn't persisted — resets on ccs restart

## Design Spec
- `.claude/plans/2026-03-18-mission-control-rework-design.md` — original mission control design
- `.claude/plans/2026-03-18-mission-control-rework-plan.md` — implementation plan
