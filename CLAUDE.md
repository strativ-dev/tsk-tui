# tsk — terminal task manager

A keyboard-only task manager for the terminal. Vim-style modal focus, a search field
for filtering, task lines that expand into a per-task time table.

Written in Go with [Bubble Tea](https://github.com/charmbracelet/bubbletea).
Reference UI prototype: `Task TUI Interactive.dc.html` / spec PDF in the design project.

## Stack

- `github.com/charmbracelet/bubbletea` — runtime
- `github.com/charmbracelet/bubbles` — `textinput`, `viewport`, `key`, `help`
- `github.com/charmbracelet/lipgloss` — styling
- `github.com/BurntSushi/toml` — the config file. TOML over YAML because this file is
  all short bare-looking scalars (`y`, `n`, `on`, `j`) and quoting is mandatory here;
  over stdlib JSON because a hand-edited keymap wants comments
- Go 1.22+, no CGO

## Layout

```
cmd/tsk/main.go        program entry, tea.NewProgram(..., tea.WithAltScreen())
internal/model/        root model, per-mode Update handlers, View
internal/model/keys.go key.Binding sets per mode (footer help renders from these),
                       plus ApplyKeys/KeysTOML for the config file
internal/config/       ~/.config/tsk/config.toml — [keys] overrides, validated
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
| `ModeSearch` | search input |
| `ModeList` | task list (**default at launch**) |
| `ModeTable` | rows of an expanded task |
| `ModeInsert` | add/edit entry inputs |
| `ModeJump` | `/dd` date prompt |
| `ModeDay` | modal listing a date's entries across all tasks; `esc` closes |
| `ModeConfirm` | modal; swallows everything except `y` / `n` / `esc` |
| `ModeAuth` | API key prompt; opens when no key is stored or a fetch returns 401 |

## Keymap

Search
- any key — filter tasks by title and tag, live
- `ctrl+u` — clear query **and collapse all expanded tasks**. `ctrl+l` was tried
  first and abandoned: tmux and vim both grab it before the app sees it
- `esc` / `enter` — focus the task list

List (`ModeList`) — the mode the app starts in
- `j` / `k` — next / previous task
- `g` / `G` — first / last task
- `ctrl+f` / `ctrl+b` — half a screen down / up (`Model.halfPage`, sized from the
  terminal height, so it moves by half of what is actually visible). Half-up is
  `ctrl+b`, not vim's `ctrl+u`, because `ctrl+u` clears the query in every mode, and
  half-down is `ctrl+f` so the pair reads as one forward/back set
- `l` — expand task, focus its first row (→ `ModeTable`); a task with no
  entries still opens, so `a` has somewhere to add the first one
- `h` — collapse task
- `/` — date jump (→ `ModeJump`); from here it lists the whole day in a modal
  (→ `ModeDay`), so it needs neither this task open nor any rows in it
- `r` — fetch tasks from the API
- `K` — replace the stored API key (→ `ModeAuth`)
- `i` / `esc` — focus the search input
- `ctrl+u` — clear the query **and** focus the search input (does not collapse)
- `q` — ask before quitting (→ `ModeConfirm`); `ctrl+c` quits at once

Table (`ModeTable`)
- `j` / `k` — next / previous row
- `g` / `G` — first / last row; `ctrl+f` / `ctrl+b` — half a screen
- `enter` — edit the focused row in place (→ `ModeInsert`, kind=edit)
- `a` — new entry at the top, prefilled with today's date (→ `ModeInsert`, kind=new)
- `d` — delete the focused row (→ `ModeConfirm`). The modal names it —
  `Delete this entry "Retry backoff" of 11/08/26?` — since an unlink cannot be undone;
  a row Odoo returned with no name reads `Delete the entry of 11/08/26?`
- `/` — date jump prompt (→ `ModeJump`); inside a task it moves the cursor rather than
  opening the day modal, and matches part by part — `/12` is the 12th of any month
- `h` — collapse the task, focus the task line (→ `ModeList`)
- `esc` — focus the task line without collapsing
- `i` — **collapse the task** and focus the search input
- `ctrl+u` — collapse the task, clear the query, focus the search input

Insert (`ModeInsert`)
- `tab` / `shift+tab` — date → description → hours → ✓ → ✕
- `ctrl+u` — clear the focused field
- `enter` on a text field — same as tab
- `enter` on ✓ — commit; for a new entry prepend it to `Rows`, inputs vanish
- `enter` on ✕ — discard (→ `ModeConfirm`)
- `esc` — **edit**: cancel immediately, no modal. **New entry**: → `ModeConfirm`

Jump (`ModeJump`)
- digits and `/` build the date. **`enter` does one of two things, depending on where
  `/` was pressed** (`Model.jumpInTask`) — the two situations want different answers,
  and read the same input differently:
  - **Inside a task's rows** (`ModeTable`) it is a **move**, and the query is matched
    **part by part** (`parse.DateMatches`, `Model.jumpQuery`): `12` is the 12th of
    **any month in any year**, `12/7` any 12th of July, `12/7/26` that date alone. A
    task's rows span months, so resolving `12` against today would skip most of them.
    The cursor walks to the first row that matches; those rows are already on screen,
    so covering them with a modal would hide the thing you asked about. No tasks are
    pulled — an open task's lines are already read
  - **From the list** it is a **report on one day**, so the query is **resolved** with
    the insert field's grammar (`parse.Date` against today, `Model.jumpDate`): `12` is
    the 12th of *this* month. It opens the **day modal** (`ModeDay`,
    `view.go: dayModal`): one line per entry logged on that date in any task — task
    key, description, hours — with the day's total in its head. **Nothing in the list
    opens or moves**; the modal is the answer. `esc` (or `enter`) closes it, no
    confirmation, since it destroys nothing
- Either way the matched rows are **marked**, by whichever of the two rules applied
  (`Model.onJumpDate`): date reversed out (`theme.Match`), description in accent
  (`theme.MatchText`). So `/12` inside a task marks every 12th it can see. Marks
  **stand** after the prompt closes, so they survive scrolling and editing; `enter` on
  an empty prompt clears them, and so does `ctrl+u` wherever it is pressed
- Both the marks and the modal's contents are **derived on render** (`onJumpDate`,
  `Model.dayRows`), never captured, so a line that arrives later joins on its own
- A jump from the list **pulls the tasks it has never read**: a pull returns a task's
  whole history, so one with rows on disk has nothing to add, while one with none was
  never opened and its hours would silently miss the day. The status line counts them
  (`12/08/26 — 2 entries, reading 3 more tasks…`) and the modal grows as answers land
- The modal lists at most 12 entries and says `… N more`; its head counts them all
- An impossible date (`31/02`) keeps the prompt open with the error rather than
  quietly doing nothing
- `esc` — cancel, back to the rows if the task under the cursor is open, else the list

Confirm
- `y` / `enter` — proceed; `n` / `esc` — cancel
- destructive prompts — **quit** and **delete row** — take `y` only
  (`keys.YesOnly`, chosen by `Model.confirmKeys`), so a stray `enter` cannot fire
  them. Discarding an entry still being typed keeps `y`/`enter`. The footer and the
  modal hint both render from that same choice.

Auth (`ModeAuth`)
- typing is echoed as `•` — the key is never rendered
- `enter` — store the key in `pass` and fetch; `esc` — work offline on the local file

## Configuration (`internal/config`)

Every binding above is a **default**, not a fact: `~/.config/tsk/config.toml` can
rebind any of them. No file, or a file with no `[keys]` table, means the compiled-in
defaults stand — that is the normal case, not an error.

```toml
# Only the lines you want to change.
[keys]
half_down = ["ctrl+d"]   # back to vim's key
down      = ["j", "down", "n"]
quit      = []           # an empty list unbinds; ctrl+c always quits
```

- Action names are the `keyMap` field names in snake_case (`HalfDown` → `half_down`),
  resolved by reflection in `model.ApplyKeys`, so the accepted names cannot drift from
  the struct. `model.Actions()` lists them and every error message offers them.
- `tsk --print-keys` writes the live keymap, commented, as a `[keys]` table:
  `mkdir -p ~/.config/tsk && tsk --print-keys > ~/.config/tsk/config.toml`. **No
  example file is checked in** — a second copy of the keymap in the repo is a copy to
  fall out of date, and the binary already knows its own defaults. Its output loads
  back as a no-op, which is what keeps the documented defaults honest.
- A rebound action keeps its help **description** and takes its help **key** from the
  first key given, so the footer follows a rebind with nothing else to edit. An action
  rebound to the key it already had keeps its label verbatim — that is what preserves
  the paired hints (`g/G`, `ctrl+f/b`, held on the first of each pair) for a config
  seeded from `--print-keys`, which necessarily lists every action. Move the primary
  key of a pair and the footer shows that single key instead.
- Key spellings are bubbletea's: a single character, a `ctrl+`/`alt+`/`shift+`
  compound, or one of `enter esc tab shift+tab space backspace delete up down left
  right home end pgup pgdown`. A misspelling (`ctrl-d`) is **refused at startup** —
  it would otherwise just never match, which reads as "rebinding is broken". So is an
  unknown action name. Either prints to stderr and exits 1 rather than entering the
  alt screen with a keymap you cannot drive.
- Two actions on one key resolve by the order of the `switch` in the handler, which is
  deterministic and immediately visible; nothing validates it.
- `keys` is a package var and `ApplyKeys` is its only writer, called once from
  `main` before `tea.NewProgram`. Loading deliberately does **not** happen in
  `model.New`: `New` runs in every test, and a test must not read the developer's
  config. Tests that rebind restore `keys = defaultKeys()` in `t.Cleanup`.
- The modal's `y / n` hint is read off `confirmKeys().Help()`, not spelled out, or a
  rebind would leave a destructive prompt advertising a key that no longer accepts it.

## Credentials and sync

The task list comes from `GET /api/v1/tasks/my` (see `my-tasks.md`) with a native
Odoo API key as a Bearer token.

What the ERP REST API actually offers (probed against erp360, Odoo 16):

| Route | Use |
|---|---|
| `GET /api/v1/tasks/my` | task list (no timesheet lines) |
| `GET /api/v1/timesheets/hour-log-summary?start_date&end_date` | **day totals only** — feeds the progress bar |
| `POST /api/v1/timesheets/log` | write one entry — `api.LogHours`, fired by ✓ on a new entry |

The REST API has no line-level read, so timesheet rows come from Odoo's JSON-RPC
instead (`internal/api/rpc.go`):

1. `common.login(db, email, key)` → `uid`
2. `object.execute_kw(db, uid, key, "account.analytic.line", "search_read",
   [[["task_id","=",<id>],["user_id","=",<uid>]]],
   {fields: [date,name,unit_amount], order: "date desc, id desc"})`

The `user_id` clause matters: Odoo's own task form reads the same model **unfiltered**,
so it lists every employee's lines (AI-286 has 8 across two people). Unfiltered here
would put a colleague's hours in your table, in the task total, and in the 8h day
bar — and you cannot edit their lines anyway. `uid` comes from `common.login`, and
matches `user_id` (res.users), not `employee_id`.

- The database name is a **secret** and never lives in this repo: it is half of what
  someone needs to reach the ERP over JSON-RPC. It goes in the same `pass` entry as
  the key, following pass's convention of secret-on-line-one and metadata below:

  ```
  <odoo-api-key>
  db: <database>
  ```

  `store.LoadKey` reads line one as the key and the `db:` line via `passField`;
  `$TSK_ODOO_DB` overrides it. `store.SaveKey(key, db)` writes both back, so
  re-entering a key through `ModeAuth` does not lose the db. With neither, rows stay
  local and the status line says so (`api.ErrNoDB`). The name cannot be discovered
  at runtime — the server refuses `db.list` and the selector page is filtered.
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
## Writing hours

✓ on a **new** entry posts it: `api.LogHours` → `POST /api/v1/timesheets/log`.

- The endpoint identifies the task by **key** (`SE360-1372`), not by id, which is why
  `Task.Key` is its own field (`store.Task`) rather than baked into `Title`. The view
  prefixes it for display. A task whose `key` is null cannot be logged against.
- Body is `{task_key, date: YYYY-MM-DD, hours: decimal, description}`; `Entry.Minutes`
  is divided by 60 on the way out and the reply's `hours` multiplied back.
- The row is written to disk **and** kept `Local` until the server answers. On
  `LoggedMsg` success it takes the returned `account.analytic.line` id, drops `Local`
  (so it stops counting as unsynced), and today's total is re-fetched. On failure the
  row stays put and the status line says it was kept locally — a failed write must
  never lose typed hours.
- Client-side refusals before the request: no key, no task key, blank description,
  minutes outside 0–24h, unreadable date. The documented `code` values from
  `log-hours.md` (`daily_cap_exceeded`, `task_not_found`, `task_ambiguous`,
  `no_employee`, `no_hourly_cost`) become sentences in `logError`.
**Edits and deletes go through RPC, not REST.** The REST API only creates, but
`account.analytic.line` reports `read/write/create/unlink` all true for our user
(checked with `check_access_rights`), so:

- `enter` on a pulled row commits an `execute_kw` **`write`** with
  `{name, date, unit_amount}` (`api.UpdateEntry`). The row stays `Local` until the
  server agrees; a refusal keeps the edit and says the ERP is unchanged.
- `d` on a pulled row commits an `execute_kw` **`unlink`** (`api.DeleteEntry`). The
  row stays on screen until the server confirms — a refused delete must not hide
  hours that still exist. A local row (negative id) is dropped immediately.
- Odoo answers `write`/`unlink` with a bare `true`/`false`, so `false` is treated as
  failure rather than success. A record rule can still refuse someone else's line.
- The MCP at `/mcp` blocks `unlink` on this model; RPC does not. That is the reason
  this path talks RPC rather than MCP.
- Every RPC call logs in first, so each write costs two round trips. Fine at this
  volume; cache the uid if it ever matters.
- One entry per request, and a timed-out retry can double-log (`log-hours.md`), so
  nothing retries automatically.
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
7h30m → 450    7.5  → 450    90m → 90    7:30 → 450    7 → 420    :30 → 30
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

`DateMatches` is the other reading of the same input, for a jump inside a task — it
compares only the parts that were typed instead of filling the rest in:

```
DateMatches("12/07/26", "12")      → true   (any month, any year)
DateMatches("12/07/26", "12/7")    → true
DateMatches("12/07/26", "12/7/26") → true
DateMatches("13/08/26", "12")      → false
```

## UI rules

Layout follows `Pictures/screenshots/tsk.png`:

- An accent `❯` in the left gutter, then the search field inside a rounded box
  spanning the width. The list holds focus at launch, so the caret only appears
  once `i` or `ctrl+u` moves the cursor into the field.
- Right of the box, a two-line progress cluster, right-aligned: bar (`█` / `░`)
  plus bold `1h45m` and dim `/ 8h`, then `TODAY 22% · 6/6 tasks`. Turns green at
  100%. Recomputed live.
- A dim rule under the header, then the list, one blank line between tasks.
- Task line: caret (`▸`/`▾`), title, tag chip (uppercase, `│ BACKEND │`), and on
  the right edge the entry count and the bold task total. The caret and the right
  cluster are fixed; what is left goes to the chip (a third of the screen at most,
  measured from `cols()` so the same tag is cut to the same width on every row) and
  then to the title, which loses cells last because it identifies the task. Odoo
  project names run to 50 cells — `VALUE-DRIVEN ENGAGEMENT, INTERNAL MEETINGS & TASKS`
  — and an uncapped chip pushed the line off an 80-cell screen.
- **Everything the ERP wrote goes through `oneLine`** — task names, tags, timesheet
  descriptions, error and status text. VD-427's Odoo name contains a newline, which
  rendered the task as two lines: the list grew past the terminal, the terminal
  scrolled, and the search box went off the top. `TestErpTextCannotGrowTheView` holds
  the line count and the width.
- The query field is sized by `searchFieldWidth()`, against the **widest** the progress
  cluster gets (`progReserve`), not against the cluster as rendered. The box shrinks
  when the TODAY line gains `+2h30m unsynced`, and a field wider than its box wraps
  inside it, growing the header from three lines to four and shoving the list down.
- Every line is `Blur`/`Focus`-wrapped, both two cells wide, so focus never shifts
  a row. `internal/model/view_test.go` asserts that and that nothing exceeds the
  terminal width.
- Expanded table columns: DATE · DESCRIPTION · HOURS, all left-aligned in their
  column. HOURS was right-aligned in the head and the rows, but the insert row puts a
  `textinput` there and an input fills its column and left-aligns its own text, so
  `h:mm` sat two cells off `HOURS`. One offset per column, checked by
  `TestHoursColumnStartsAtOneOffset`. DESCRIPTION
  is capped at 48 cells — stretched to the edge, the hours end up a screen away
  from the text they belong to.
- `dateWidth` and `hoursWidth` are **one cell wider than the value they hold** (9 and
  6). The insert row puts a `textinput` in each column and an input always draws a
  cursor cell after its `Width` (`fieldWidth(col) = col-1`), so a column sized exactly
  to `dd/mm/yy` cannot show a normalized date: the field scrolled and `12/08/26` read
  as `2/08/26`.
- The header and footer are laid out first and the list is windowed into the rows
  left over (`view.go: window`), so the search field can never scroll off the top.
  Hidden lines are announced as `↑ N more` / `↓ N more`, and the window follows
  the cursor.
- Focus is shown by a **left border + dim row background**, never color alone. On top
  of that the accent colors whatever holds the keys: the title of the task the cursor
  is in — kept once it is expanded and focus has moved into its rows, so you can still
  see which task you are inside (`theme.TitleFocus`) — and the search frame while the
  field has the cursor
  (`theme.SearchBoxFocus`). Colors are additive here, not the signal on their own —
  `TestFocusIsAccentColored` covers both.
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
