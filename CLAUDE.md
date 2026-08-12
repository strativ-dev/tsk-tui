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
internal/store/        persistence (JSON on disk), API key via pass, load/save commands
internal/api/          ERP tasks API client (GET /api/v1/tasks/my)
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
| `ModeAuth` | API key prompt; opens when no key is stored or a fetch returns 401 |

## Keymap

Search
- any key — filter tasks by title and tag, live
- `ctrl+u` — clear query **and collapse all expanded tasks**
- `esc` / `enter` — focus the task list

List (`ModeList`)
- `j` / `k` — next / previous task
- `l` / `enter` — expand task, focus its first row (→ `ModeTable`); a task with no
  entries still opens, so `a` has somewhere to add the first one
- `h` — collapse task
- `/` — expand the task and open the date jump (→ `ModeJump`)
- `r` — fetch tasks from the API
- `K` — replace the stored API key (→ `ModeAuth`)
- `i` / `esc` — focus the search input
- `ctrl+l` — clear the query **and** focus the search input (does not collapse)
- `q` — ask before quitting (→ `ModeConfirm`); `ctrl+c` quits at once

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
- the **quit** prompt takes `y` only (`keys.YesOnly`, chosen by `Model.confirmKeys`),
  so a stray `enter` cannot end the session. The footer and the modal hint both
  render from that same choice.

Auth (`ModeAuth`)
- typing is echoed as `•` — the key is never rendered
- `enter` — store the key in `pass` and fetch; `esc` — work offline on the local file

## Credentials and sync

The task list comes from `GET /api/v1/tasks/my` (see `my-tasks.md`) with a native
Odoo API key as a Bearer token.

What the ERP REST API actually offers (probed against erp360, Odoo 16):

| Route | Use |
|---|---|
| `GET /api/v1/tasks/my` | task list (no timesheet lines) |
| `GET /api/v1/timesheets/hour-log-summary?start_date&end_date` | **day totals only** — feeds the progress bar |
| `POST /api/v1/timesheets/log` | write one entry (not wired up yet) |

The REST API has no line-level read, so timesheet rows come from Odoo's JSON-RPC
instead (`internal/api/rpc.go`):

1. `common.login(db, email, key)` → `uid`
2. `object.execute_kw(db, uid, key, "account.analytic.line", "search_read",
   [[["task_id","=",<id>]]], {fields: [date,name,unit_amount], order: "date desc, id desc"})`

- `$TSK_ODOO_DB` must hold the database name; the server refuses `db.list` and the
  selector page is filtered, so it cannot be discovered. Without it, rows stay
  local and the status line says so (`api.ErrNoDB`).
- The login is the key owner's email, which arrives in the `hour-log-summary`
  response (`DayHoursMsg.UserEmail`) — nothing else exposes it.
- `task_id` is the numeric `project.task` id from `/tasks/my`, not the `AI-283`
  key. Filtering on the key string returns nothing.
- Lines are read lazily: expanding a task pulls it once (`Model.pulled`), and `r`
  clears that so open tasks re-read.
- A pull **merges**, never assigns (`store.MergeRows`). `Entry.Local` marks what the
  ERP has no matching copy of — an entry typed with `a`, or a pulled line edited
  here — and app-created entries carry **negative** ids so they can never collide
  with an `account.analytic.line` id. A local edit beats the pulled copy; a row
  that is neither takes Odoo's version. Assigning `msg.Rows` directly deleted every
  locally added row, which is the bug this guards.
- Nothing writes back yet (`POST /api/v1/timesheets/log` is unwired), so local rows
  stay local, and deleting a pulled row with `d` only hides it until the next pull.
- Empty Odoo char fields decode as `false`, hence `odooText`.
- The bar reads `Model.todayMinutes()`: the ERP's own day total (`Model.erpToday`,
  `-1` until a sync answers) **plus** `store.PendingMinutesOn` — the app-created
  entries the ERP has never seen, shown as `+2h30m unsynced` on the TODAY line.
  Local edits of pulled lines are excluded from that sum, since the ERP total
  already counts their original hours. Before a sync answers, local rows are the
  only source.

- Key resolution: `$TSK_API_KEY` first, then `pass show $TSK_PASS_NAME`
  (default entry `tsk/api-key`). No plaintext fallback file — if `pass` is
  missing, the app says so and stays offline.
- `pass show` runs through `tea.ExecProcess`, so a tty pinentry gets the terminal
  instead of corrupting the alt screen; stdout is captured, stderr is the user's.
  `pass insert -m -f` takes the key on stdin (no pinentry: encrypt only).
- The key lives in the `Authorization` header, the `Model.key` field, and nowhere
  else: not in a URL, not in a log line, not in an error string, never in
  `tasks.json`. Screens show `store.MaskKey` (`••••1234`); the prompt echoes `•`.
- Base URL: `$TSK_API_URL`, default `https://erp360.strativ.se`.
- `store.Merge` keeps remote titles and tags, keeps every local entry, and keeps
  local-only tasks that still hold hours — a sync never drops logged time.
- 401 clears the in-memory key and reopens `ModeAuth`; any other failure keeps the
  disk copy of the list and says so in the status line.

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

Layout follows `Pictures/screenshots/tsk.png`:

- An accent `❯` in the left gutter, then the search field inside a rounded box
  spanning the width, focused on launch.
- Right of the box, a two-line progress cluster, right-aligned: bar (`█` / `░`)
  plus bold `1h45m` and dim `/ 8h`, then `TODAY 22% · 6/6 tasks`. Turns green at
  100%. Recomputed live.
- A dim rule under the header, then the list, one blank line between tasks.
- Task line: caret (`▸`/`▾`), title, tag chip (uppercase, `│ BACKEND │`), and on
  the right edge the entry count and the bold task total.
- Every line is `Blur`/`Focus`-wrapped, both two cells wide, so focus never shifts
  a row. `internal/model/view_test.go` asserts that and that nothing exceeds the
  terminal width.
- Expanded table columns: DATE · DESCRIPTION · HOURS (right-aligned). DESCRIPTION
  is capped at 48 cells — stretched to the edge, the hours end up a screen away
  from the text they belong to.
- The header and footer are laid out first and the list is windowed into the rows
  left over (`view.go: window`), so the search field can never scroll off the top.
  Hidden lines are announced as `↑ N more` / `↓ N more`, and the window follows
  the cursor.
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
