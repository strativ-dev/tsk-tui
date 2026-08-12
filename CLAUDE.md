# tsk — terminal task manager

A keyboard-only task manager for the terminal. Vim-style modal focus, a search field
that owns focus on launch, task lines that expand into a per-task time table.

Written in Go with [Bubble Tea](https://github.com/charmbracelet/bubbletea).
Reference UI prototype: `Task TUI Interactive.dc.html` / spec PDF in the design project.

## Stack

- `github.com/charmbracelet/bubbletea` — runtime
- `github.com/charmbracelet/bubbles` — `textinput`, `viewport`, `key`, `help`
- `github.com/charmbracelet/lipgloss` — styling
- Go 1.22+, no CGO

## Layout

```
cmd/tsk/main.go        program entry, tea.NewProgram(..., tea.WithAltScreen())
internal/model/        root model, per-mode Update handlers, View
internal/model/keys.go key.Binding sets per mode (footer help renders from these)
internal/parse/        pure parsing: hours, dates  (unit-tested, no tea imports)
internal/store/        persistence (JSON on disk), load/save commands
internal/theme/        lipgloss styles, all colors in one place
```

## Domain

```go
type Task struct {
    ID    int
    Title string
    Tag   string   // "backend", "ui", ...
    Rows  []Entry  // newest first
}

type Entry struct {
    ID      int
    Date    string // "02/01/06" — dd/mm/yy
    Desc    string
    Minutes int    // stored as minutes; rendered "7:30"
}
```

Store minutes, never a formatted string. Totals and the daily progress bar are
**derived on every render** — never cached in the model.

## Modes

One `Mode` field on the root model. Only the active mode consumes keys.

| Mode | Focus |
|---|---|
| `ModeSearch` | search input (default at launch) |
| `ModeList` | task list |
| `ModeTable` | rows of an expanded task |
| `ModeInsert` | add/edit entry inputs |
| `ModeJump` | `/dd` date prompt inside a table |
| `ModeConfirm` | modal; swallows everything except `y` / `n` / `esc` |

## Keymap

Search
- any key — filter tasks by title and tag, live
- `ctrl+u` — clear query **and collapse all expanded tasks**
- `esc` / `enter` — focus the task list

List (`ModeList`)
- `j` / `k` — next / previous task
- `l` — expand task, focus its first row (→ `ModeTable`)
- `h` — collapse task
- `/` — expand the task and open the date jump (→ `ModeJump`)
- `i` / `esc` — focus the search input
- `q` / `ctrl+c` — quit

Table (`ModeTable`)
- `j` / `k` — next / previous row
- `enter` — edit the focused row in place (→ `ModeInsert`, kind=edit)
- `a` — new entry at the top, prefilled with today's date (→ `ModeInsert`, kind=new)
- `d` — delete the focused row (→ `ModeConfirm`)
- `/` — date jump prompt (→ `ModeJump`)
- `h` — collapse the task, focus the task line (→ `ModeList`)
- `esc` — focus the task line without collapsing
- `i` — **collapse the task** and focus the search input

Insert (`ModeInsert`)
- `tab` / `shift+tab` — date → description → hours → ✓ → ✕
- `ctrl+u` — clear the focused field
- `enter` on a text field — same as tab
- `enter` on ✓ — commit; for a new entry prepend it to `Rows`, inputs vanish
- `enter` on ✕ — discard (→ `ModeConfirm`)
- `esc` — **edit**: cancel immediately, no modal. **New entry**: → `ModeConfirm`

Jump (`ModeJump`)
- digits and `/` build the query; `enter` jumps to the first row whose day matches
- `esc` — cancel, back to `ModeTable`

Confirm
- `y` / `enter` — proceed; `n` / `esc` — cancel

## Input parsing (`internal/parse`)

Pure functions, normalized **on field exit** — not per keystroke.

Hours → minutes:
```
7h30m → 450    7.5  → 450    90m → 90    7:30 → 450    7 → 420
```
Render minutes as `h:mm` in the table, `6h45m` for task totals.

Date, relative to the row's existing date (or today for a new entry):
```
8      → 08/08/26   (day only: keep month + year)
8/9    → 08/09/26   (day + month: keep year)
8/9/26 → 08/09/26
```
When editing a row, the date field opens **pre-filled and visually selected**; the first
keystroke replaces it, `tab` with no input keeps the original.

## UI rules

- Search bar at the top, focused on launch. Right of it: a today-progress bar,
  hours logged today against an **8h goal**, block chars (`█` / `░`), label `6h45m / 8h`
  and a percentage. Turns green at 100%. Recomputed live.
- Task line: caret (`▸`/`▾`), title, tag chip, entry count, task total.
- Expanded table columns: DATE · DESCRIPTION · HOURS (right-aligned).
- Focus is shown by a **left border + dim row background**, never color alone.
- Mode indicator top-right (`-- SEARCH --`), contextual key hints in the footer,
  rendered from the mode's `key.Binding` set so help can never drift from behavior.

## Theme (`internal/theme`)

| Role | Color |
|---|---|
| accent / focus | `#FFC000` |
| chrome | `#151520` |
| destructive | `#E13400` |
| confirm / complete | `#12CC63` |
| link / tag | `#01B9AE` |
| muted text | `#8A8F99` |

## Conventions

- `Update` returns `(tea.Model, tea.Cmd)` — never mutate through a pointer receiver
  stored elsewhere; keep the model a value type.
- No I/O in `Update` or `View`. Disk reads/writes go through `tea.Cmd`.
- Filtering is derived from the query on each update; don't keep a second slice in sync.
- Every parsing rule above has a table-driven test in `internal/parse`.
- `go test ./... && go vet ./...` before considering a change done.
