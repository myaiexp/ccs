# Project Grid: `ls`-style Columnar Layout

## Problem

Projects render as a flow-wrapped tag cloud with " · " separators. Lines fill independently, nothing aligns vertically, and it's hard to scan — especially with wildly varying name lengths.

## Design

### Layout Algorithm

1. Truncate project names beyond `project_name_max` (default 16). Append `…` when truncated.
2. Try N columns starting high, work down. Items flow **left-to-right, top-to-bottom** (reading order). This matches the activity-based sort — most active projects fill the first row.
3. Each column width = max truncated name width among items in that column + 2 (gap).
4. Use the highest column count where total width fits `m.width - 4`.
5. No separators — alignment and gaps provide visual structure.

### Truncation

- Default: 16 characters
- Cycleable in prefs popup: 12 / 16 / 20 / 24 / 30
- Config key: `project_name_max`
- Full name visible in detail pane when selected (already works)

### Navigation

Arrow keys map to the grid — left/right across columns, up/down across rows. Same `projectGrid()` logic but now computed from fixed column layout instead of flow-wrap.

### Example (16-char max, wide terminal)

```
PROJECTS
  ccs               spot-price        explorer          superpowers       vps
  ~                 central-hub       factorio-solv…    mase.fi
  poe-crafting      .openclaw         cloned-claude-…   singlepagers      tracker
```

## Changes

- `renderProjects()` — replace flow-wrap with column grid render
- `projectGrid()` — rewrite to match new column layout
- `types.go` — add `ProjectNameMax` to Config
- `config.go` — add `project_name_max` field, default 16
- `model.go` prefs — add "Name length" cycle (12/16/20/24/30)
