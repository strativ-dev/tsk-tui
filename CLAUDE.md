# tsk — terminal task manager

A keyboard-only task manager for the terminal. Vim-style modal focus, a search field
for filtering, task lines that expand into a per-task time table.

Written in Go with [Bubble Tea](https://github.com/charmbracelet/bubbletea).
Reference UI prototype: `Task TUI Interactive.dc.html` / spec PDF in the design project.

## Stack

- `github.com/charmbracelet/bubbletea` — runtime
- `github.com/charmbracelet/bubbles` — `textinput`, `viewport`, `key`, `help`, `spinner`
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
internal/api/          ERP tasks API client (GET /api/v1/tasks/my), Odoo JSON-RPC,
                       the dashboard's monthly hour log (api/hours.go), and the year's
                       leave, balances and public holidays (api/timeoff.go)
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

## Tabs

`Model.tab` is the screen; `Model.mode` is the focus inside it. Three so far:

| Tab | Keys | What |
|---|---|---|
| `TabTasks` | `t` · `1` | the task list and everything reached from it (**default at launch**) |
| `TabDash` | `d` · `2` | this month's hours per day, and the ERP's clock (`c`) |
| `TabTime` | `o` · `3` | this year's time off: the calendar, the balances, the holidays |
| `TabMeal` | `m` · `4` | this month's canteen meals, one bar per meal per day |

- The tab bar is the **first line of every screen** (`view.go: tabBar`), the active tab
  reversed out in the **accent** (`theme.Pill`) — on this line the primary colour marks
  one thing, which screen you are on: `¹tasks  ²dashboard  ³timeoff  ⁴meal`. Each tab carries **its position as a
  raised digit** at the top-left of the label, btop style, and the **letter picked out
  inside the word itself** (`hinted`) — accent on an inactive tab, dark ink underlined on
  the pill, where accent on light would fail contrast. Both come off the binding, so a
  rebind shows in the bar with nothing else to edit.
- **No footer names a tab key.** The bar picks each tab's letter out of its own label, so a
  `t tasks` or `d dashboard` hint below spends a footer slot saying the same thing twice — the
  meal tab never had one, and the task list, the chart and the calendar have given theirs up.
- **Digits are aliases in bar order** (`1`, `2`, `3`, `4`). They are matched in the same place as
  the letters, so the excluded typing modes protect them: a query of `2` and a `/12` date
  prompt both keep their digits.
- Tab keys are matched **before** the mode handlers, but **not** while a field is taking
  letters (`ModeSearch`, `ModeInsert`, `ModeJump`, `ModeAuth`) — `t`, `d` and `o` have to stay
  typeable in the query box and in a description. A modal (`ModeConfirm`) keeps the
  keyboard whichever tab is behind it.
- **`x` deletes a row, not `d`.** `d` means one thing everywhere, and a key that unlinks
  an hour log must not be one keystroke from a tab switch.
- **The time off tab is `o`, not `t`.** The spec's own footer asks for `t today`, and `t`
  is the tasks tab from every screen: a tab key that means one thing everywhere is worth
  more than a motion the calendar can do without, since it opens on this month anyway.
- The query field renders on `TabTasks` only: it filters tasks, so it belongs to that
  tab. The dashboard's and the calendar's bodies replace the list body; the footer and
  status line are shared.

## Dashboard (`TabDash`)

One chart: hours logged per day this month, `d` to open, `t` to go back.

- Data comes from `account.analytic.line.get_employee_hour_logs`
  (`api.FetchHourLogs`), one call for the whole month. The browser reaches it over
  `/web/dataset/call_kw` with a session cookie; we have an API key, so it goes through
  the same `execute_kw` path as the rest — an **empty ids list** (it is an `@api.model`
  method) and any timestamp inside the month to report on.
- The bar is **`actual`**, the track behind it **`expected`**. `logged_this_day` is the
  raw sum of that day's lines and disagrees — 06/08/26 has `actual` 8 and
  `logged_this_day` 16 — so the chart would show a 16h day. `office_hours + home_hours`
  is attendance, a different question from timesheet hours.
- **The whole month is on it**, the days still to come included — that is where this
  month's holidays and weekends are, and they are worth seeing before the week starts. A
  working day that has not happened draws the **bare track and no number**: the ERP reports
  8 expected hours for it, and an empty red `0:00` bar read as a day the hours were missed
  on rather than one nobody could have worked.
- **The target is the whole month** (`dashTotals`): `logged  ███┈┈┈  80:00 / 152:00`, since
  152:00 over 22 days is what the month is billed against and what a bar is worth filling
  towards. Hours logged and the `N of M days` count still stop at today — they can only be
  counted where the days exist.
- **The number beside it is today's own hours** (`view.go: todayLogged`), not the month's and
  not a gap: `today 0:00` on a day nothing has been logged to yet, coloured on the chart's
  thresholds. It printed the shortfall first — `today −8:00` — and opening the chart in the
  morning to a red minus eight read as hours already missed rather than a day not yet worked.
  The colour carries the shortfall instead, and today's own bar says the same thing against
  the 8h tick. A day nothing was expected of reads `today off`, and a month the ERP has not
  reported today in reads `today —` rather than implying a full day owed.
- Weekends, holidays and leave say so instead of drawing an empty bar (`dayNote`); a
  working day with nothing on it draws an empty track, which is the point. The band
  spans the full track deliberately — width itself says "nothing was expected".
- **The bar** (`view.go: dashBar`) is exactly `barCells` wide whatever it holds: a **dark wash**
  of the threshold colour (`theme.BandLow`/`BandMid`/`BandHigh`), **edged and lettered in the
  light one**, then the dotted remainder. Details that are each there for a reason:
  - **Ends and an underline separate the bands**: `▏` and `▕` at each end
    (`barEdgeL`/`barEdgeR`) plus the light **underline** the `*Fill` styles carry along the
    bottom. Nothing along the top — a rule there crowded the band and read as a second bar.
    Once the month is in columns the rows sit directly against each other, and a bare wash of
    colour merged a run of eight-hour days into one rectangle; a *block* inside the band read as
    a pattern rather than a bar, and a half block hung at the bottom of the cell, under its own
    row. This costs no row, where a rule between days would cost one per day — which is what the
    columns exist to save.
  - **Working days are white, quiet ones dim** (`theme.DayLabel`): a weekend or a holiday is
    not a day you can act on, so the dates carry that without the working days needing a
    colour of their own. Today's date keeps the accent.
  - **The month is laid out in columns** (`dashGrid`), days running down one and on into the
    next, so all 31 of them are one screen on a terminal that could never stack them: at
    100×24 that is four columns of eight rows, at 120×30 three, at 100×70 one. Every day
    keeps its own label and its own printed hours — that is the point of the columns rather
    than a per-day glyph. The count comes from the rows left over (`dashChrome`) capped by
    what the width holds at `minBar` cells a bar, and by `maxDashCols`, since more columns
    only make the bars shorter. Below roughly 80 cells the month no longer fits a 24-row
    terminal and the window takes over again.
  - `barCells` and `dashGrid` both size themselves from **one estimate** of the body's rows,
    not from the budget `View` measured, so the bars, the columns and the axis cannot
    disagree with each other; being a row out only changes how early a column is added.
  - **A rule between days** in the one-column case, so each bar sits against its own row
    rather than in a column of colour. Ruled, a month costs `2n-1` rows, so it is the first
    thing given up as the terminal shortens — then the columns take over from it entirely.
  - **A ruler under every column** (`dashFoot`): the columns share one scale, and a bar with
    no axis beneath it is measured against nothing. `dashAxis` is exactly as wide as a day's
    row — the corner on the bar's first cell, the rule ending on its last — since one cell
    over ran four columns past the right edge.
  - **The axis is only pinned when the days scroll.** A month that fits gets its ruler back
    in the body, directly under the last day where it is read; pinned, it sat at the bottom of
    the screen with the body's padding between it and the bars (`View`, where `dFoot` moves
    from the tail into the body).
  - **It moves in screenfuls, not days.** There is no cursor — the chart is a picture, not
    a list — so `j`/`k` are unbound here and `Model.dashHold` only says which day the
    window is built around: `-1` follows **today**, where it opens and where a re-read puts
    it back; `g`/`G` pin it to the month's ends; `ctrl+f`/`ctrl+b` move half a screen of
    days (`halfPage(dashRowLines)`), clamped. `window` holds that row in view and counts
    what is off screen.
  - **`<`/`>` move between months**, not days: `Model.dashOffset` is the viewed month in
    months from the current one — `0` is this month, `-1` last month — and `<`/`>` step it,
    clearing `dashMonth` so the new month is re-read (`updateDash`). `>` refuses past `0`:
    the ERP has nothing to report on a month that has not happened. Both reset `dashHold` to
    `-1`, so a navigated-to past month opens on its last day (`dashDayIndex`'s fallback, since
    "today" is never among a past month's days) rather than wherever the previous month left
    the window. Going back and forward again re-reads rather than caching — one month in
    hand at a time, the same as the single-month cache `dashMonth` already was.
  - **Only the days move.** The month, its totals and the tick legend are laid out with
    the header (`dashHead`) and the axis with the footer (`dashFoot`), so the figures the
    bars are read against cannot leave the screen with them. A terminal too short for both
    gives the rows back — the axis first, then the head (`minDayRows`), since a chart with
    two visible days is not a chart.
  - Today's bar says **`today`** after its hours, inside the band.
  - **The label sits at the end of the run**, not beyond it, so it costs the bar no width and
    the bar keeps meaning what the axis says. A bar too short to hold its own number spills it
    into the track, in the bar's colour, eating the dots it covers so the row stays the same
    width.
  - **`┆` ticks at 4h and 8h, only past the fill.** A bar sitting exactly on 8h has
    reached that threshold, so a tick at its edge would read as part of the bar.
- **Colours by threshold** (`hourBand`/`hourFill`): under 4h `#E0574F`, under 8h
  `#E0A030`, 8h or more `#5FBF7F`, from the spec's dashboard palette. The spec draws
  one-colour bars and tints only the number, warning that red-amber-green is the
  colourblind trap — the colour is on the bars here by choice, so the signals it relies
  on stay: bar length, the visible ticks, the labelled axis, the printed number.
- Hours read as **`h:mm`** everywhere (`hoursLabel` → `parse.FormatHM`): the ERP sends
  decimal hours, and 8.25 means nothing to anyone logging time — 8:15 does.
- The `logged` figure is a **bar** too (`monthBar`), against the month's target. A 14h
  shortfall stated as a number alone read like a rounding error.
- The month is read **once** per session (`dashMonth`), and `r` re-reads it.
- Opening it needs the key owner's **email**, which only arrives with the REST day-total
  answer (`DayHoursMsg.UserEmail`). Pressing `d` before the first sync lands sets
  `dashWanted`, fetches the day total, and continues into the chart when the email
  arrives — otherwise the first `d` of a session failed with `unknown Odoo login`.
- Totals are derived on render (`dashTotals`), never stored.

### The clock (`c`)

Check in and out without leaving the terminal — top-right of the chart, `WFH  11:05 AM
(5:12)` over a button (`view.go: clockStatus`, `clockButton`).

- **The button is a box** (`theme.ClockIn`/`ClockOut`), three lines of the head, because it
  is the one thing on this screen you can press and it should look like it. The **border**
  carries the state, in the chart's own threshold colours: green invites an action not yet
  taken, amber says a clock is running and wants closing, a plain rule is the state we have
  not read yet. The **words are white** (`theme.ClockText`) and only the **`c` is the accent**,
  so the colour that means "this is the key" everywhere else is not competing with the
  state's colour for the same cells.
- **The footer names the action, not the thing**: `c check in` / `c check out`
  (`view.go: clockHelp`), and `c checking out…` while a request is out. `c clock` left you to
  guess which way the key would go. The key comes off the binding, so a rebind follows.
- **One key, both directions.** `c` toggles, because `hr.employee.attendance_manual` is one
  toggle — the same button the web client shows. Checking out opens the modal
  (`confirmCheckOut`) and takes **`y` only** (`keys.YesOnly`): it closes a session the ERP
  then bills. The spec asks for two keys, `c` and `C`, so a stray `c` cannot close the day;
  the modal buys that with the guard the app already has; `C` is the month's own confirm, below.
- **The prompt states facts, not a prediction**: `Check out now? (in since 11:05 AM, 5:12)`.
  `cPrompt` is frozen when it is built and the server stamps its own time, so a promised
  "check out at 4:17 PM" would be a lie two minutes later.
- **`c` needs a state it has read.** A toggle fired against an unknown state could check you
  out when you meant in, so `Model.toggleClock` — the one path both the key and the modal go
  through — refuses until `attKnown` and `attEmp` are set, and says `reading attendance…`
  rather than doing nothing. Until then the button is **dim**: a green one that swallowed
  the key would be a lie.
- **The button only flips on confirmed state.** `api.ToggleAttendance` presses the button and
  **re-reads in the same Cmd**, so the screen never shows a guess. A `{"warning": …}` answer
  is not an error — "already checked in from another device" means the truth moved without
  us, and the snapshot beside it is what fixes the screen. An answer that disagrees with the
  intent (`Want`) is applied as-is and says so; nothing retries, since a retry ping-pongs.
- **The open session is read by `check_out = false`**, never `last_attendance_id`: after a
  check-out that field still points at the closed record, whose `check_in` is populated, so
  the screen would draw a running clock beside a `check in` button. One `hr.attendance`
  `search_read` answers both questions — a row exists, and it says since when.
- **`hr.employee` is read with exactly `["id", "attendance_state"]`.** This user cannot read
  `hours_last_month` (Attendances officer group) and one refused field fails the whole call,
  so a widened list breaks the feature. `TestAttendanceFieldsStayMinimal` holds it.
- **Elapsed is derived on render**, `now - check_in`: Odoo leaves `worked_hours` at 0 until
  the session closes. A `clockTickMsg` every 30s only repaints (`clockTick`), scheduled from
  the same one place in `Update` as the spinner and guarded by `Model.ticking`, so a late or
  dropped tick costs freshness and nothing else. It keeps ticking on the task list: gating it
  per tab would mean a second scheduling site, which is what the flag exists to avoid.
- A **read that was already in flight when a toggle started is dropped** (`!msg.Toggled &&
  m.clocking`), and `loadDash` will not start one while `clocking` — two answers to the same
  question must not land in the wrong order.
- `m.clocking` is in `busy()`, or the loader in the button's place would never animate.
- **`WFH` comes from the month, not the employee.** `hr.employee.work_location_id` is a place
  ("Bangladesh"); `get_employee_hour_logs` reports `work_location: "office" | "home" | null`
  per day, so `DayLog.WorkLocation` reads today's. It is `odooText`: Odoo sends `false` for an
  empty char field, and `false` into a `string` fails the **whole month**, not the one day.
- Times print as `3:04 PM` (`view.go: clockTime`). The API layer keeps Odoo's UTC and this is
  the only place that localises it. Wall-clock formatting lives in `view.go`, not
  `internal/parse`, which is about the input grammar.

### Confirming the month (`C`)

A second box on the button band, left of the clock: `Confirm hour logs`, and the ERP is told the
month is done.

- **`account.analytic.line.confirm_hour_logs` is a recordset method taking no arguments** — the
  lines go in `execute_kw`'s ids slot — and it sets each line's `confirmed` boolean. So
  `api.ConfirmHours` reads the ids first: the key owner's own lines (`user_id = uid`, the same
  clause the table's read uses), inside the month, `confirmed = false`. There is no monthly
  sheet model on this database and no `@api.model` entry point; the MCP exposes none of this.
- **The answer is not a success signal.** It replies with a bare `false` on lines it has in fact
  just written, so only an RPC error is a failure here and the month is **re-read** rather than
  trusted. Nothing retries.
- **A month with nothing left to confirm costs no write** and is not an error — it says
  `August 2026 was already confirmed`, the same way the meal line refuses before the round trip.
- **`C`, not `c`.** The clock's `c` closes a session and this closes a month; the pair reads as
  one idea, and the spec's own reason for keeping `C` free was exactly this kind of key.
- **It asks first and takes `y` or `n`** (`confirmHourLogs`, `keys.Yes`): the prompt is
  `Have you logged all hours of August 2026 ?` and it names the **viewed** month, so `<` then `C`
  asks about July. It is a claim about every day on the chart behind the modal, which is why the
  month is in the sentence rather than only in the button.
- **The box is green and stays green**, the colour that invites an action not yet taken: nothing
  is running here, there is only something to do, so the clock's amber has no meaning for it. The
  words are white and only the `C` is the accent, exactly as on the clock. It goes dim while the
  WFH line holds the keyboard, and the loader takes the label's place while the call is out.
- **While the modal is up the box is pressed**: green fill, white words, and the `C` no longer
  picked out, since it has been pressed and the modal in front of it is what the keyboard is
  answering. The fill stops **inside the frame** (`theme.ClockOn`, `ClockOnText`) — the border
  cells keep their green glyph and no background, so the box measures and reads as the same
  button it is unpressed; painted onto the border rows as well, the block of colour looked bigger
  than the button, which is the same reason the ✓ and ✕ on the request lines stop short of their
  own frames. The label carries the fill on its own span, since a foreground set inside resets
  the background the box put behind it.
- **It shares the clock's three rows** (`dashHead`, through `clockLine`): the clock is on the
  right edge where its own status line sits, the confirm on the left, so the chart pays no rows
  for it.

### The WFH request line (`ModeWFH`)

One row over the chart, opened by a refusal rather than by a key:
`wfh request  │21/08/26 │ → │21/08/26 │ │reason…│ │✓│ │✕│`.

- **The refusal is what opens it.** `attendance_manual` declines a check in from home once the
  free days are used up — *"You have exceeded the number of days available for WFH. Please
  submit a WFH request."* — and the line is the shortest way from that sentence to the request,
  so `needsWFH` matches the ERP's own word for the thing and `openWFH` runs there. Any other
  refusal is reported and nothing opens: a form in front of an unrelated error is a form that
  cannot help.
- **The model is `serp_attendance.wfh_request`, not `hr.leave`.** There is no work-from-home
  leave type on this database; the serp attendance module keeps its own model, and
  `hr.attendance.wfh_request_id` points at it. Fields are `start_date` / `end_date` /
  `description` (labelled *Reason*, and **required**, so a blank one is refused here rather
  than one round trip later), `employee_id` defaults to the caller, and `number_of_days` and
  `name` are computed.
- **Two calls, because create alone is not a request.** `state` defaults to `draft` — the ERP
  calls it *"To Submit"* — so `api.RequestWFH` follows the create with **`action_confirm`**,
  which is what moves it to `confirm` (*To Approve*). Read off `default_get` and `fields_get`
  over RPC; the MCP does not expose this model at all, so it was probed there.
- **A create that landed with a submit that did not still carries its id back**
  (`WFHRequestedMsg.ID`): re-filing would ask HR for the same days twice, so the line closes on
  it and says what happened. **Nothing retries**, for the same reason.
- **The answer retries the check in** — that is what the request was for — and the ERP has the
  last word on whether a submitted request is enough. If it refuses again with the same words,
  `wfhFiled` keeps the line shut: reopening there would loop, filing the same days on every
  attempt. A check in that lands clears the flag.
- **✓ does not ask.** It asks a manager for days, which destroys nothing, and the line already
  states everything a modal would repeat. `esc` and `✕` close it outright for the same reason —
  nothing has been filed, and the refusal is still in the status line to open it again.
- **The line owns the keyboard while it is open** (`ModeWFH` excluded from the tab-key and `?`
  block, routed before the tab handlers), so a reason can hold a `t`, a `d` and an `m` while the
  chart's own month keys sit behind it.
- It renders **under the check in button, on the same right edge** (`dashHead`, through
  `clockLine`): the button is what refused, so the line belongs to it. The rows come out of the
  head, so the days window into what is left, and a chart too short for both **gives up days
  rather than the head** while the line is open — a field you cannot see is a field you cannot
  fill. The lines go in bare: `clockLine` is what right-aligns them and adds the gutter, and a
  second one pushed the row off the edge.
- **The button goes dim and gives up its accent** while the line is open (`clockButton`): `c`
  cannot fire, since the line holds the keyboard, which is the same rule the meal tab's inactive
  label follows.
- The reason is capped at `wfhMaxReason` cells so the row stays a cluster under the button
  rather than a band across the screen; the input scrolls, so the cap costs visible characters
  and nothing else. Dates normalize on exit and the end follows the start past it
  (`normalizeWFHDates`, `before`), the same rules the other two lines have; a selected value is
  padded to the **full** `dateWidth`, so tabbing onto a date does not shrink its box and slide
  the buttons along the row. The reason's width is measured **after** the form is in the model:
  an empty `wfhForm` draws no date fields, so sizing against it left the row too wide by nine
  cells and `clockLine` truncated the buttons off.

## Time off (`TabTime`)

A year calendar of the days you took off, the leave balances above it, the public holidays
beside it. `o` to open, `t` to go back. **Read only** — see the spec (`docs/timeoff.md`)
for the request form, which is not built.

- Four RPC reads, **one message** (`api.FetchTimeOff` → `TimeOffMsg`), because they are one
  screen: half of them landing would draw a calendar whose days disagree with its own
  totals. `internal/api/timeoff.go`:
  - `hr.leave.type.get_days_all_request` — the balance cards. An `@api.model` method that
    takes **no arguments at all**, not even an empty ids list: passing one fails the read
    with `takes 1 positional argument but 2 were given`. It answers with one array per type,
    `[name, {figures}, "yes", id]` — the **figures are strings** (`"8.5"`), and the **id is
    the last element**, which is what lets a filter name a type without matching its name.
  - `hr.leave.search_read` — mine, for the year. The dates are
    **`request_date_from` / `request_date_to`**, Odoo's own `Date` fields: `date_from` is a
    UTC datetime for the same request, and reading a day out of that puts a 10am-Dhaka
    morning on 04:00 UTC of a day that may not be the same one. The domain **overlaps** the
    year rather than sitting inside it, so a request across New Year is on both calendars,
    and `refuse`/`cancel` are dropped — a red day nobody is taking off reads as one that was.
  - `hr.employee.search_read` for `resource_calendar_id`, then
    `resource.calendar.leaves` with `resource_id = false` — the public holidays. **Per
    office**: Dhaka keeps 17 in 2026 and Sweden 12, on separate calendars, so whose applies
    is a property of the employee. Company-wide closures (`calendar_id = false`) count too.
    This is a **separate read from `employeeOf`**, whose field list is two fields on purpose
    (see the clock) — widening that one to save a round trip here would break attendance.
  - Holiday rows are datetimes spanning the working day (04:00–13:00 UTC in Dhaka,
    07:00–16:00 in Sweden), so both ends fall on their own local date and the date part is
    the day itself.
- **The filters are the leave types' own initials** — `s`, `c`, `a`, `p` here, but nothing
  is hardcoded: `Model.filterKind` takes the first type whose name starts with the letter,
  and the chips render the same initial in the accent. They are matched **after** every
  bound key, so a type named `Toil` cannot shadow the tasks tab. The same letter again
  clears the filter, and so does `esc`.
- **A day off is its type's colour as a band** with the date reversed out of it
  (`view.go: dayCellOf`) — the spec's filled circle, in the two cells a terminal gives a
  date. A **half day bands one of those two cells**: the left for a morning, the right for
  an afternoon, which is the same half-filled square and says which half as well. A single
  digit is right-aligned, so its banded half is a space — still half the square.
- **Pending is an underline**, and the head says `pending underlined` **only on a year that
  has one**: an underline nobody can see needs no legend.
- **The visual spec is `docs/timeoff-styles.md`** — surfaces, the text ramp, and the
  translation notes for the parts a terminal cannot draw. What it says goes; the notes below
  are only what that meant in cells.
- **A day is a four-cell badge, `" 21 "`**, filled when the day carries anything and plain on
  the month's own surface when it does not, with the date reversed out in dark ink — never
  white on a colour. It sits in a **five-cell column** (`dayCell`, `badgeCell`): the fifth is
  the gap the design leaves between its days, and without it a run of leave days read as one
  long bar rather than as days.
- **The measurements come from the design's own month cell** — 304px at 38 columns. A week is
  35 cells and a month 39 with the week-number column (`weekCol`) and the cell's left padding
  (`monthPad`) in front of it, so **three months want 121 cells and three months with the
  holiday panel about 148**; below that the width buys months one at a time.
- **The body is cut by the row, never through a month** (`timeLines` takes the budget, as
  `dashLines` does, and `timeWindow` picks the rows): the generic line window sliced a row
  through its third week and reported `↓ 11 more` in lines, which is a count nobody thinks in.
  Whole rows only, the caret's row always among them, growing forward first — a year is read Jan
  to Dec — and what is left out says so in **months** (`↑ 6 more months`).
- **A screenful is two rows of three months** — half a year — and that outranks a month cell's
  own air (`timeTier`). A week row gets a line under it, which is what makes a month read as a
  calendar rather than as a table and is the design's own proportion (a day nearly as tall as it
  is wide), and the design's padding around the cell comes on top of that; both are spent only
  when two rows still fit. So a month is **8 rows bare, 13 with the air, 16 padded as well**, and
  a 130×32 terminal shows six months where the aired version showed three and a half. The budget
  is `rows() - timeChrome` less the two lines the window spends on its own `↑ N more` /
  `↓ N more` — an estimate, like `dashChrome`, since being a row out only moves where a tier
  changes.
- **The days the request line covers are reversed out in the accent** — the same mark a date
  jump leaves on the rows it found. The type it is for is on the request line itself, in its
  own colour, so the day does not have to carry both.
- **Months first, then the holidays** (`timeLayout`): a full row of three is what makes a
  screenful half a year, so the panel **never costs a month its column** — it takes what three
  months leave when that is enough to read a holiday on (`panelMin` to `panelMax`), which is
  about 148 cells, and is dropped below that. It used to have a column from about 130 at the
  price of the third month; the holidays are still on the calendar there as dimmed days, where a
  month is not recoverable from a list.
- **The months are cells of one surface, divided by hairlines** — `│` between the columns and
  a `─` across the grid between the rows, never a gap, which is what the spec asks for
  everywhere. The month in view is **tinted with the accent at 4.5%**
  (`theme.PanelHold` against `theme.Surface`), the only thing that colour does there.
- The surface colour is on **every span**, never wrapped around the line: a background set
  around a whole line dies at the first span that sets its own — a weekend badge, a leave
  day — and never comes back, which drew the month as a stripe that stopped at the first
  coloured day (`theme.OnPanel`, and `monthPanel.filler` for the line a five-week month pads
  a six-week row with). `TestMonthPanelIsRectangular` holds every line at exactly `monthCols`.
- **The text ramp is the spec's** (`theme.DayInk` for working days, `QuietInk` for weekend
  numbers, `WeekInk` for week numbers, `Muted` for the weekday heads, and `theme.Ink` —
  `#0B0B10`, never white — for anything sitting on the accent or on a leave colour).
- **Today is accent ink with no pill.** A filled accent cell is what the days being typed on
  the request line take; today is not a cursor.
- Weekends and public holidays are a **filled badge** (`theme.WeekendBand`, white at 4.5%
  over the surface), lighter than the surface so it reads as a day nobody works rather than as a gap in the month, and
  deliberately identical to each other — neither is a day you can act on. It is also the
  faint half of a half day. **Weekend is Saturday and Sunday**: both offices' calendars run
  Monday to Friday (`resource.calendar.attendance`, `dayofweek` 0–4), and reading that per
  employee would be a fifth call to say the same thing.
- **Annual is its own violet** (`theme.LeaveAnnual`), not the accent: focus is accent
  everywhere else, and a calendar full of accent days beside an accent cursor says nothing.
  `theme.LeaveColor` matches on the type's name — the ids are per database, the palette is
  per meaning — and an unknown type gets **white**, since a leave day in muted ink on a dim
  band is exactly what a weekend looks like.
- **A month's name is white** (`theme.Title` over `theme.White`), not the muted head style the
  weekday letters take: the names are how the year is read, and twelve dim ones beside a single
  accented month read as eleven months switched off. The month in view keeps the accent and the
  caret.
- **A month is titled `Jan 26`** and its weekday heads are single letters, right-aligned
  over the dates, per the design. `T`/`T` and `S`/`S` are told apart by their column, which
  is the only thing a date under them is read by anyway.
- **The months size themselves to what they span.** A month is its name, the weekday heads
  and four to six week rows; the months in one row are padded to the tallest of them and no
  further, since padding all twelve to six costs the year lines it does not have. Three
  months a row fits 80 cells (`monthCols` = 23); the count comes from the width
  (`timeCols`), capped at `maxTimeCols`.
- **The holiday panel is a pinned column flush with the right edge**, as wide as the months
  leave it
  (`view.go: withHolidayPanel`), on its own raised surface (`theme.Raised`), with a dim rule
  between it and the months and the dates in their own column before a `:`, as the design
  draws it. A one-day holiday **says which weekday it takes** — `Aug 5 (Wed)` — which is the
  question a holiday raises; a run of days (`Mar 18-23`, `Mar 30-Apr 2`) answers it by being
  a run, and `holidaySpan` stays inside `spanCells` either way. The rows of the **month in view** are marked down the panel's own edge — a `▎` in
  the accent, the date in the accent, the name in white — so the list answers the part of the
  calendar you are looking at. Its header gives up the year and then the count itself rather
  than pushing the column wider than the one it was handed. It is a column of the screen, not of
  the calendar, so it does not move when the months do or when one of them is a week shorter. It is composed **after** `window` has cut the body — the months scroll
  under it and the holidays stay where they are read. Zipped into the body before the
  window instead, the header and the first holidays scrolled away with January on the first
  keypress. A list taller than the body ends in `… N more` rather than stopping mid-year;
  the spec gives the panel its own scroll, which is what that line stands in for.
- It gets its column only when **a full row of three months still fits beside it**
  (`timeLayout`): from about 148 cells, and below that the dimmed days on the months are the
  whole answer.
### The new-timeoff line (`n`)

One row between the balances and the calendar, and the whole request is on it:
`new timeoff  │Annual ▾│ │full day ▾│ │21/01/26 │ → │23/01/26 │ │description…│ │✓│ │✕│`.

- **The row is there whether or not the line is open.** Closed it is the label alone, with
  `n` in the accent; `n` reveals the fields and focuses the leave type — and **the label gives up
  the accent on its key** while it is open, since the line owns the keyboard there and `n` types
  an `n` into the description. The same rule the meal labels and the clock button follow. Nothing above or
  below moves, which is the point of keeping the row — `TestNewLeaveOpensWithoutShifting`
  holds the calendar's first line at the same index either way.
- **Every field is a rounded box** (`theme.Field`), as the design draws them, which makes
  the row **three lines tall** — and it is three lines closed as well, the label on the
  middle one, so revealing the fields moves nothing (`leaveBand`). The parts are joined with
  `lipgloss.JoinHorizontal(lipgloss.Center, …)`, which is what puts the one-line label beside
  the three-line boxes rather than on their top rule.
- **The ✓ and ✕ fill when the keys are on them** — green and red, the mark reversed out in
  white (`theme.FieldOkOn`/`FieldDropOn`, `OnOk`/`OnDrop`) — rather than taking the accent
  frame the fields take: these two are pressed, not typed into, so a fill says what a
  hovered button says everywhere else. The fill stops **inside** the frame — on the border
  cells too it spread a cell past the box on every side — and the mark carries the fill on
  its own span,
  since a foreground set inside resets the background the box put behind it.
- **Two marks, two meanings.** The accent **frame** says which field has the keys; the accent
  **fill** says the value is selected and the next keystroke replaces it, which is only ever a
  date field just tabbed into (`leaveDate`, `theme.Match`). The selected value is rendered
  without the input's cursor — the whole value is the selection — and padded to `dateWidth`,
  so it measures the same as the input it stands in for.
- **The leave type reads in its own colour**, the one its days are drawn in on the calendar
  below, and takes the accent while it holds the keys.
- Tab order is left to right, which is the only order the line reads in: type, duration,
  the dates, what it is for, then ✓ and ✕ (`leaveFieldCount`). `tab`/`shift+tab` move,
  `enter` on a field is a tab, `enter` on ✓ asks, `enter` on ✕ starts over.
- **Duration is a dropdown, not a checkbox** — `full day` / `half day` — and choosing half a
  day replaces the range's end with the **morning/afternoon** dropdown, since a half day is
  one day and has no end to give. `j`/`k`/`space` change whichever dropdown has the keys;
  they are letters everywhere else, so they only do that on a dropdown.
- **A leave type's own initial picks it** on the type dropdown — `s`, `c`, `a`, `p` here,
  the same letters the filter chips take, matched on the type's name
  (`Model.kindByLetter`) rather than hardcoded. It is matched **after** the bound keys, so
  `j`/`k` keep stepping and a type named `Kayak` cannot shadow them. `filterKind` returns
  the type's id, which is what a filter holds; this returns its index, which is what the
  form holds.
- **The description sizes itself from the row that is actually drawn** (`leaveSkeleton`
  renders the line with an empty description and measures it), so it cannot disagree with
  what surrounds it by a cell. It measures **both durations and takes the wider**, or
  choosing half day would push the buttons off the row. Narrow terminals give up the label
  first, then the spaces between the boxes, then the space inside them (`leaveTier`,
  `compact`) — the boxes stay boxes — so the whole request still fits 60 cells.
- **The end date follows the start when the start passes it** (`normalizeLeaveDates`, `before`).
  Left behind, the range reads backwards and quietly covers the days between — a request for
  the 20th with the 19th still in the end field booked both, and the ERP refused the pair over
  a leave that was already on the 19th. `leaveRange` still swaps a reversed pair as a backstop.
- **A refused request re-reads the year**, and **the reason goes in the status line**, not
  only in `err`: what refused it is usually a leave this screen has not seen — someone can
  file one for you in the web client, so the calendar can never be fresh by itself — but the
  answer to that re-read clears `err`, which left a refusal on screen as the word "refused"
  and nothing else. Whatever the ERP said is the one thing worth keeping, verbatim.
- **Dates are normalized on exit, never per keystroke**, against today for the start and
  against the start for the end: `21` `tab` `23` `tab` lands on the description with
  `21/08/26` and `23/08/26` in the fields. A freshly focused date field is **selected** —
  the first keystroke replaces the whole value, so `21` is the 21st rather than appended to
  what was there (`leaveForm.fresh`, the same rule the entry row has).
- **The days the line covers are marked on the calendar as they are typed** — both ends and
  everything between — reversed out in the accent (`theme.Match`, the same mark a date jump
  leaves), and the month they are in **comes into view** (`followLeaveDates` sets
  `timeHold`, so the caret moves to it). Partial input counts: `21` already marks a day.
  The mark outranks whatever else the day holds, because it is what the keys are about — the
  design rings the day instead, keeping the type's colour inside it, which a terminal cell
  cannot draw; the form line above it is already showing the type in its own colour.
- **✓ asks first** (`confirmApplyLeave`), with a modal that states the type, the dates, the
  duration and the description, and takes **`y` only** — it files a request the ERP then
  routes to a manager. `n` returns to the line with everything in it. A prompt of several
  lines puts its `y / n` hint on a line of its own, or "Coast trip  y / n" reads as the
  description.
- **`esc` asks too** (`confirmDropLeave`): everything typed goes with the line. `✕` does not
  ask — nothing has been filed, so there is nothing to lose — and it **closes the line**,
  back to the `new timeoff` label the tab opened on; `n` opens a fresh one. It reset in
  place first, which left you on a form you had just said you were done with, and `esc` was
  then the only way off the row.
- **The line stays exactly as typed until the ERP answers.** A refusal keeps it on screen to
  fix; only `LeaveRequestedMsg` with an id closes it, and that re-reads the year so the days
  appear where the calendar says they are rather than where this screen guessed.
- **The line owns the keyboard while it is open**: `ModeForm` is excluded from the tab-key
  and `?` block and routed before the tab handlers, so a description can hold a `t`, a `d`,
  an `o`, an `n` and a `?`.
- The write is `hr.leave` `create` (`api.RequestLeave`), with **`request_date_from` /
  `request_date_to`, never `date_from`**: those are computed from the request dates and the
  employee's own working calendar, which is the only thing that knows when their day starts —
  an approved leave reads back `date_from 2026-01-21 04:00:00`, which is 10am in Dhaka.
  `employee_id` comes from the same read that found the calendar (`TimeOffMsg.Employee`);
  with no employee record it is left out and Odoo works out who is asking. A half day sends
  `request_unit_half` and the period and one date. **Nothing retries** — a timed-out create
  that in fact landed would book the leave twice, and a duplicate request is a conversation
  with HR.
- What the ERP itself says about that write, checked against the MCP (`hr.leave` is exposed
  there with `create: true`, `unlink: false`) and `default_get`/`fields_get` over RPC:
  - **`state` defaults to `confirm`**, so a created record is *submitted*, not a draft — there
    is no `action_confirm` to call afterwards, and the status line saying it is waiting on
    approval is the truth. Most types here are `validation_type: both`, two approvers.
  - `holiday_type` defaults to `employee`, so it is left out; `notes` is **readonly** (the
    description is `name`); `number_of_days` and `date_from`/`date_to` are computed.
  - **One leave per day** is a hard constraint, so `Model.leaveClash` refuses a range that
    covers a day already taken before the round trip, naming the day — the same way the hour
    log refuses what the endpoint would.
  - The **balance is stated, not enforced** (`BALANCE  8.5 left, this takes 3  — more than you
    have`): some types are allowed to run negative, and refusing a request the ERP would have
    accepted is worse than warning about one it will not.
  - The MCP is a Claude-side server, not something this binary can call, so the write stays on
    RPC; the MCP is only how the model's own rules were read.

- **`enter` lists the month's own time off** in a modal (`ModeLeaves`, `view.go:
  leavesModal`): one line a day — `19 Aug (Wed)  casual : Baby got sick` — with the type in
  the colour its days are drawn in behind, the date as a person says it, and what the request
  said it was for. `esc` closes it and nothing else in it needs a key, since it destroys
  nothing.
  - **A day, not a request**: the calendar above already reads a range as the days it covers,
    so collapsing `19-21 Aug` into one row would answer a different question. A half day says
    which half (`18 Feb (Wed, morning)`), and anything not yet `validate` says `pending` —
    the underline the calendar marks that with has no room in a list.
  - It **follows the filter** and names it in the head (`sick only`), so the list and the
    calendar under it can never say different things. Both columns are sized from the rows,
    so a month with no half day in it does not pay for the word "afternoon", and the rows are
    derived on render (`monthLeaves`) like every other figure here.
  - Routed **before the tab handlers** (`Model.updateLeaves`), the way the new-timeoff line
    is: otherwise `j`/`k` would walk the months behind a modal whose head names the month it
    is listing. `?` still reaches the help toggle, as on every other modal that is not a
    confirm.
- **It moves in months, not days.** No cursor — a year is a picture — so `Model.timeHold`
  only says which month the window is built around: `-1` follows **this** month, `h`/`j`/`k`/`l`
  step **one month** — Jan, Feb, Mar, which is the sequence a year is read in — `g`/`G` pin it to
  the ends, and `ctrl+f`/`ctrl+b` move a row of months, the one place a width-dependent jump
  still says something. They are the **same bindings the task list moves by**, so a rebind moves
  both (`view.go: monthMoveHelp` reads their keys back for the footer).
  - **`h`/`l` are aliases of `j`/`k`, not a second distance.** They stepped one month while
    `j`/`k` stepped a row, and a row is two, three or four months depending on the width — so
    the same key covered a different jump on every terminal, and the calendar read as a grid
    rather than as twelve months in order. Now whichever hand you reach with lands on the next
    month, and the footer lists all four.
  - This month carries the **caret** (`▸Aug`), which is the only thing that says where today is
    once a leave has taken the cells.
- **The balances are four boxes** (`view.go: balanceCards`), as the design draws them: the
  type's name with its filter key picked out, the days left, and `DAYS AVAILABLE` under it,
  divided by verticals and ruled above and below — the rule above is the screen's own, under
  the tab bar, which is why `View` gives `TabTime` no blank line there. The dividers are what
  makes them boxes; a border per card would cost two more rows to say the same thing.
  - **The figure is double width** (`wide`), in the fullwidth digits: the one way a terminal
    has of drawing a bigger number without spending a second row on it. Two rows of quadrant
    blocks was bigger still and read worse — a balance is a number, and a number drawn out of
    quadrants stops looking like one.
  - The year and the days taken ride on the **tab bar's own row** (`timeSummary`), which is
    half empty, rather than costing the calendar a title line. It gives up its parts from
    the tail in when the tab bar leaves it less room (`fits`).
  - The cards **split the width evenly and the remainder goes to the right-hand ones**, or
    the row would stop short of its own rules.
  - A name too wide for its card falls back to its **first word** — the word carrying the
    key is the one that cannot be cut.
  - **Nothing left is dim**, not the type's own colour, and the card being **filtered by is
    reversed out** so the calendar and the card that explains it read together.
- The initial inside a card is **its own span**, not part of a style wrapping the label: a
  colour nested inside another does not survive the inner reset.
- **One year in hand at a time** (`Model.timeYear`, the cache key as well as the year on
  screen), and `r` re-reads it. `r` does **not** clear `timeYear` — `loadTime` is called
  outright, so nothing needs it cleared, and clearing it blanks the calendar and its totals
  for as long as the read takes. The loader sits beside the title instead, as on the chart.
- **No year switching**, per the spec: `<`/`>` are the chart's months and nothing here.
- Opening it needs the key owner's **email**, exactly as the chart does: `timeWanted` is set,
  the day total is fetched, and the calendar continues when `DayHoursMsg.UserEmail` lands.
  Both flags can be set at once — `d` then `o` before the first sync — so the handler starts
  whichever asked.
- The rest of the keys are the shared ones: `r` re-reads, `q` asks before quitting, `i` and
  `ctrl+u` leave for the query field (which filters tasks, so it takes you to that tab), `?`
  toggles the key list. There is no `j`/`k` and no `x` — nothing here is a list, and nothing
  here writes.
- `timeLoading` is in `busy()`, and every figure — the days taken, the per-month counts, the
  marks map — is **derived on render** (`timeMarks`, `timeTaken`), never stored.

## Meals (`TabMeal`)

This month's canteen bookings: a Monday-first month grid where every day carries one short
bar per meal type, the week's menu down the right, `b` to book and `x` to cancel. `m` to open,
`t` to go back. The design (`docs/meal-calendar-palette.html`) also has staged edits and a
save modal, which are not built — `b` books outright instead.

- Four RPC reads, **one message** (`api.FetchMeals` → `MealMsg`), for the same reason the
  time off screen reads four in one: the types landing without the bookings would draw a
  month of empty slots that only looked like a month nobody ate in.
  `internal/api/meal.go`:
  - `serp.meal.type` `search_read` on `active`, ordered by `sequence` — **Breakfast** (id 2),
    **Lunch** (1), **Snacks** (6) on this database, but nothing about the three is
    hardcoded: the bars on a day are whatever the office serves, in the order it returns
    them, so a fourth meal gets a fourth bar and a fourth legend swatch with nothing to edit.
    `serving_time` and `allow_booking_before_hours` come along as decimal hours, so the
    cutoff a later booking step needs is already in hand (`serving − before`, Dhaka).
  - `serp.meal.booking` `search_read`, **`user_id` = the uid, always**: a key in the
    meal-admin group sees the whole office, and a canteen list with everyone's meals on it
    says nothing about what you are eating. `state = booked`, since a cancelled row would
    draw as one that is on. `date` is Odoo's own **Date** field — no zone conversion, unlike
    the leave datetimes.
  - `serp.meal.booking` `get_unusual_days` — the days the canteen is shut. An `@api.model`
    method taking **the two ends and no ids list** (the same shape as the leave balances),
    answering `{"2026-08-01": true, …}`: **weekends and public holidays together**, which is
    exactly the question a meal calendar asks. 5 Aug 2026 is a Wednesday and comes back
    `true`. Working that out here would mean reading the office calendar and its holidays to
    say what one call already says, and getting it wrong on a holiday nobody told us about.
  - `serp.meal.menu` `search_read` over the month — what is **on offer**, whether or not it
    was booked, which is the question the bars cannot answer. `common_items_display` is what
    everyone gets and `options_display` the pick, `/`-separated — the same split the booking
    rows carry as `common_items` / `available_options`. No `user_id` in this domain: a menu is
    the same for everyone. The **month**, not the week, because the panel follows the cursor
    and the cursor walks the month — one call of ~60 rows instead of one a week.
- **The week's menu is a pinned column down the right** (`view.go: mealMenuPanel`,
  `withMealPanel`), composed **after** `window` exactly as the holiday panel is, so the weeks
  scroll under a list that stays where it is read. It follows the cursor: `mealWeekStart` is
  the Monday of the week the cursor is in, so walking to next week brings next week's menu.
  - **The cursor's whole block is the accent** — the heading `Thu 27` and its dishes — and
    nothing else on the panel is: it is the day the grid bands and the day `x` and the two lines
    act on, so the panel answers the part of the calendar you are pointing at. Marking only the
    heading left four lines of the same weight as every other day. **Today keeps `· today` and
    gives up the accent** when the cursor is elsewhere: it already says itself on the grid with a
    bright underlined date, which is the same division of labour there — the band is the cursor,
    today is a date. The
    swatches keep their meal colours, since that is what says which meal a line is. A day with no menu rows is left out, which is what the weekend is; a day the ERP has a
    menu on but calls shut is **dimmed rather than dropped**, since hiding the odd one out
    hides a fact.
  - **The menus are cut by runes as well as by cells** (`truncShaped`). They are written in
    Bangla, and lipgloss counts its matras and hasantas as **zero** cells — `পরোটা, অমলেট, মুগ ডাল`
    measures 18 while being 21 runes — so a line cut to the measured width came out wider than
    its column in a terminal without Bengali shaping, wrapped, and pushed the grid's own last
    columns onto the next screen row, which read as the calendar printing its dates twice.
    Cutting to whichever is smaller is safe either way. `TestMealPanelFitsUnshapedText` holds
    every line inside the width **by rune count**, which is the stricter of the two.
  - **One line per meal, cut rather than wrapped**: the swatch in the meal's own colour then
    the dish, the choice first and the common items after a `·`. Three meals across five days
    has to fit beside a month — wrapped, the panel ran twice the height of the body — and the
    swatch carries which meal it is, since the legend above the grid already says which colour
    is which.
  - It gets its column only when the grid keeps **all** its cells, measured at a gap of 2 and
    not at the narrowest one: at a single cell between days a week of bars runs together into
    one stripe, which is what the gap is for. So `mealPanelMin` 28 to `mealPanelMax` 44, and
    the panel shows from about 92 cells; the 80-cell month is untouched.
- **The bar vocabulary is the design's own** (`view.go: mealDay`): a booked meal is `━━` in
  its type's colour, an open slot is `──` and hueless, and a day the canteen is shut carries
  **no bars at all** — the empty row is what says nothing was on offer, the same way the
  hour chart's band does for a day nothing was expected of.
- **A day already eaten keeps its meal's hue, dimmed** (`theme.MealPastColor`, the palette's
  same three hues at ~45% toward the background). Drawn in the weekend grey instead, a month
  whose bookings are all behind it read as a month nobody ate in — which is every month by
  the time you look back at it. The palette earmarks those dims for staged edits; those are
  told apart by their dashed glyphs, not by hue.
- **`is_locked_for_user` is not what greys a bar.** Locked means the booking can no longer be
  changed, which is true of tomorrow's lunch after this morning's cutoff — greying that read
  as a meal that had already happened. It decides what `x` may cancel, nothing about colour.
- **Colours by type name** (`theme.MealColor`), as `LeaveColor` does and for the same
  reason — the ids are per database, the palette is per meaning: breakfast `#E8A33D`, lunch
  `#DD5F45`, snacks `#93C572`, iftar the violet, anything else white.
- **The band marks the cursor, not today** (`theme.MealBand`): `x` acts on the cursor, so
  that is what has to be visible; today says itself with a bright underlined date, and the
  cursor's own date is bold. `Model.mealHold` is the day it is pinned to — 0 follows today,
  or the 1st in a month today is not in — and `h`/`l` walk a day, `j`/`k` and `ctrl+f`/`b` a
  week, `g`/`G` the ends of the month, clamped, since wrapping would land on a month this
  screen has not read. They are the same four bindings the task list moves by.
- **`x` clears the cursor's day** — every meal on it — and the footer says `x clear day`, off
  the same binding since a rebind has to follow it. It names the **scope** on purpose: `c`
  cancels the meals it is told to over the days it is given, and this takes one day whole. Two
  keys both reading `cancel meal` said nothing about which was which. The modal **names what
  goes** — `Cancel 3 meals on Mon 3
  Aug?  breakfast · lunch · snacks` — and takes **`y` only** (`keys.YesOnly`): an unlink
  cannot be undone. `x`, not `d`: `d` is the dashboard from every screen, and the design's
  own `d delete day` cannot have that key.
- Two refusals happen **before** the round trip (`askCancelMeals`), the way the hour log
  refuses what the endpoint would: a day with nothing booked, and a day every booking on
  which is `Locked` — the ERP reports that per booking, so there is no cutoff arithmetic here.
  A day with a mix cancels what it can.
- The write is `serp.meal.booking` **`unlink`** (`api.CancelMeals`): an employee cancels by
  deleting the row, since only a meal admin may do it by setting `state`. Odoo answers with a
  bare `true`/`false`, so `false` is a refusal, not a success. **Nothing retries**, and the
  answer **re-reads the month** rather than dropping the rows locally — someone can book or
  cancel for you in the web client, so the month is only ever what the ERP says it is. A
  refusal keeps the day on screen with what the ERP said in the status line.
- The band is set on **every span** of the cursor's cell and stops before the gap after it:
  a background wrapped around the line dies at the first span that sets its own, which is the
  same trap the month panels on the time off calendar avoid.
- **The cell arithmetic is the design's**: a bar is two cells, one space between bars, so a
  day is `━━ ━━ ━━` at three meals, and the **gap after it is what stops a week of booked
  days reading as one long stripe**. Five weekday columns of eleven and two weekend columns
  of **seven** — a closed day never holds bars, so it needs only its date — is 69 cells, and
  that is what fits the month on an 80-cell terminal. Narrower ones shed the gap to 2 then 1
  (`mealGapFor`); the bars never lose a cell, or a booked meal and an open one would measure
  the same.
### The book-meal and cancel-meal lines (`b`, `c`)

Two labels under the calendar, a line each, and the whole request is on whichever one is open:

```
  book meal   │ today ▾ │ │✓│ │✕│
   ☑ breakfast
   ☑ lunch
   ☑ snacks
  cancel meal                        ← dim while the other line has the keyboard
```

- **One form, two verbs** (`mealForm.drop`): `b` books what `c` cancels, over a scope and a set
  of meals chosen exactly the same way. One struct for both, since a row that can hold only one
  of them cannot hold two states — and opening either **replaces** whatever was there.
- **The label that is not on the row stays, and goes fully dim, key and all**: its key does
  nothing while the other line owns the keyboard, and an accent on a key that does nothing is
  the accent lying. Closed, both carry their key in the accent.
- **No switching while a line is open.** `b`, `l` and `s` are the meals' own ticks there, so `b`
  meaning breakfast on one line and "start booking instead" on the other would be one key with
  two jobs. `esc` closes the row; then `b` or `c` opens the one you want.
- The cancel line's ✓ takes **`y` only** (`confirmDropForm`) — an unlink cannot be undone, the
  same rule `x` follows on a single day — where the booking line's takes `y`/`n`. It drops what
  the ERP has already **locked** before asking (`dropWanted`), since it refuses to change those.
- **✓ is green on both lines.** It means "commit this row", and the row already says which verb
  it is; red there made the cancel line's commit look like its discard, which are the two things
  a reader most needs to tell apart. ✕ carries the red.
- **The calendar previews the day as it will be**, not what is going: on the cancel line a
  ticked meal that is booked is drawn as the **open slot it is about to become**, and one left
  unticked keeps its own colour because it is staying. Ticking lunch takes the lunch bar off
  the day, which is the whole question the tick is answering.
- **A meal with nothing to cancel in the scope is disabled** (`dropAvailable`): dim box, dim
  name, no accent on its letter, and `none` beside it. Its key says `no lunch booked on those
  days` rather than moving a tick that could not act on anything. The line **opens ticking only
  what it can cancel**, and the ticks are re-derived whenever the days change — a new scope or a
  typed date is a new set of bookings (`retickForScope`), so a tick can never be left behind on
  a meal the scope no longer holds.
- **The tick map is copied on write** (`ticksWith`). The model is a value everywhere else in
  this app, but a map inside it is a reference: toggling in place reached every copy that still
  held the old form.

- **The row is there whether or not the line is open** — closed it is the label alone with
  `b` in the accent. `TestBookMealLineOpensWithoutShifting` holds the view's line count.
- It sits **directly under the calendar**, one blank line below the last week, not pinned to
  the bottom of the screen: it is about the days on the grid, and a row of fields a screen away
  from them reads as belonging to nothing. It is appended **before the padding** that fills the
  body — after it, the line drifted to the bottom of the screen, which is exactly what it is
  not for — and its rows come out of the weeks' budget, so the month is windowed into what is
  left rather than the line being pushed off.
- The menu column is zipped over the **whole** body, this row included, so it keeps its own
  length instead of being cut to however tall the month happens to be. Which means the row has
  to fit what that column leaves: `bookCompact` measures against `cols - panel - 3`, and
  `withMealPanel` **trims** as well as pads, so nothing beside the column can push it past the
  terminal. `monthCells` is the one place a month's width is worked out, so the two-up test and
  the layout that puts two months side by side cannot disagree by the cell that wraps a row.
- **One meal a line**, under the boxes and indented to them: three ticks in a row read as three
  more fields to tab through, and they are not.
- **The ticks are not tab stops.** `tab` runs scope → the two dates → ✓ → ✕, so it lands where
  the booking is actually pressed; each meal is toggled by its own letter instead, which is
  what makes a stop on it pointless.
- **The scope is a dropdown and nothing more**: `j`/`k`/`space` step it — today, tomorrow,
  week, custom — wherever the cursor is on the line, since it is the only dropdown there.
  It has **no letter of its own**, and nothing in its label is picked out: `t` and `c` are the
  tasks tab and a leave filter everywhere else, and a line that quietly took them back for one
  screen is a key meaning two things.
- **`custom` is the only scope with dates**, and turning it off takes its two fields away —
  `clampBookField` keeps the cursor on a field that still exists. The dates normalize on exit
  and the end follows the start past it, the same rules the time off line has
  (`normalizeBookDates`, `before`).
- **A meal's own initial ticks it** — `b`, `l`, `s` here, off the ERP's own names
  (`mealByLetter`), so an office that starts serving dinner gets `d` with nothing to edit. All
  of them open **ticked**: booking every meal is the common case, and unticking one is a
  keystroke where ticking three is three.
- **The line owns the keyboard while it is open** (`ModeBook`, excluded from the tab-key block
  and routed before the tab handlers): `t` is tomorrow here, `w` and `c` are scopes, and `b`
  is breakfast rather than the key that opened the line.
- **`✕` and `esc` close it outright, with no confirm** — nothing has been filed, so there is
  nothing to lose, and the meals it would have booked are one `b` away again.
- **`✓` asks first** (`confirmBookMeals`), with a modal that states what it is about to book —
  the days, then the meals — and takes `y` or `n`: a scope key can turn one day into thirty, so
  the count is worth reading before it is filed. `n` comes back to the line with everything
  still on it. It is **`y`/`n`, not `y` alone**, since a booking is reversible: `x` cancels the
  day, which is the destructive half and takes `y` only.
- **What the ERP would refuse is dropped before the round trip** (`bookDays`, `bookMeals`),
  the way the hour log refuses what the endpoint would: a past date, a day the canteen is shut
  (`mealClosed`), anything past the ERP's 30-day ceiling, and a meal already booked on that day
  — it holds one booking per meal per day. Nothing ticked says `tick a meal first`.
- The write is `serp.meal.booking` **`create`, one row at a time** (`api.BookMeals`). Odoo's
  create takes a list, but one refused row would roll the whole list back, and the refusals
  here are ordinary — a cutoff that passed while the line was open. Booking four days and
  being told the fifth was full beats booking nothing, so the message carries **counts**:
  `booked 6 meals`, or `booked 2 meals, 1 refused: Booking is closed`, the ERP's own words.
- **`user_id` is left out** — it defaults to the caller, and naming it would let a meal-admin
  key book for somebody else by accident — and so are `menu_id` / `menu_item_id`, which
  resolve themselves from the date and the type. `TestBookMeals` holds all three out.
- **The range is marked on the calendar as it is typed** (`bookCovers`, `bookBar`): every day
  the ✓ would book is reversed out in the accent — the same mark a date jump leaves — with the
  ticked meals drawn in their own colours on it, so the ticks read on the grid as well as on
  the row. It comes from the same `bookDays` the request is built from, so the calendar cannot
  mark a day the ✓ would skip.
- **A range that runs past the end of the month brings the next month with it** (`bookSpill`,
  `monthGrid`): side by side where two grids fit, stacked underneath where they do not
  (`mealTwoUp`), and the **menu column gives up its cells first** — seeing the days a booking
  covers beats knowing what is on the menu. The spill month carries no cursor and says its own
  name, since the header names the other. `FetchMeals` reads **two months** in its one set of
  calls for exactly this: fetching the second on demand would cost three more round trips and
  a screen that disagreed with itself until they landed.
- **Nothing a day draws may exceed its own column** (`fitCell`): the weekend columns are
  narrower, since a day the canteen is shut carries no bars, and a Saturday the ERP does serve
  on would otherwise run a cell past its neighbour and wrap the row.
- Anything booked **closes the line and re-reads the month**: what was booked is on the
  calendar behind it, which is a better answer than a form still asking the same question.
- **The head's legend gives up its parts from the tail in** — the `N of M days` count, then
  the meals' names, leaving the swatches the bars are read by — and each tier is measured **on
  the line as it will be drawn** rather than by adding up its parts, since the padding, the
  gutter and the trailing space are each a cell that arithmetic forgets.
- **Below 61 cells there is no month**: seven columns at the narrowest grid is what a week
  costs, so the body says `a week needs 61 cells — widen the terminal` and the weekday row
  goes with it, rather than both of them overflowing.
- **A week the canteen served nothing in costs no row for bars** — the weekend tail of a
  month — so the grid is 2 rows a week plus the blank, not 3 everywhere.
- **It moves in months, nothing else**: `<` / `>` step `Model.mealOffset`, and `>` refuses
  past `0` — the canteen has nothing to report on a month that has not happened. Stepping
  clears `mealMonth`, so the new month is read; **one month in hand at a time**, the same as
  the chart. `r` re-reads without clearing it, so the month on screen stays up with the
  loader beside its title.
- The month, the step keys and the legend are laid out with the **header** (`mealHead`), so
  the weeks scroll under the figures they are read against.
- Opening it needs the key owner's **email**, exactly as the chart and the calendar do:
  `mealWanted` is set, the day total is fetched, and the month continues when
  `DayHoursMsg.UserEmail` lands. All three flags can be set at once.
- `mealLoading` is in `busy()`, and every figure — the day's bookings (`mealsOn`), the
  `N of M days` count (`mealDaysBooked`) — is **derived on render**, never stored.

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
| `ModeForm` | the new-timeoff line on `TabTime`; owns every key, so a description can hold `t` |
| `ModeWFH` | the WFH request line on `TabDash`, opened by the ERP refusing a check in |

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
- `d` — the dashboard tab; `o` — the time off tab; `t` comes back here
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
- `x` — delete the focused row (→ `ModeConfirm`). `d` is the dashboard tab, and a key
  that unlinks an hour log must not sit one keystroke from a tab switch. The modal
  names it —
  `Delete this entry "Retry backoff" of 11/08/26?` — since an unlink cannot be undone;
  a row Odoo returned with no name reads `Delete the entry of 11/08/26?`
- `/` — date jump prompt (→ `ModeJump`); inside a task it moves the cursor rather than
  opening the day modal, and matches part by part — `/12` is the 12th of any month
- `h` — collapse the task, focus the task line (→ `ModeList`)
- `esc` — focus the task line without collapsing
- `q` — ask before quitting, as in the list (→ `ModeConfirm`); `n` comes back to these
  rows rather than collapsing the task
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

- **A `[keys.<tab>]` table rebinds an action on one screen only** — `tasks`, `dash`, `time`,
  `meal` (`model.TabNames`). Each screen reads `keysFor(tab)`: the global map with its own
  overrides on top, resolved through `Model.k()` — a **method, not a field**, so there is no
  copy to keep in step with `m.tab` and the handlers and the footer cannot disagree about
  which key does what.

  ```toml
  [keys]
  delete = ["x"]     # everywhere

  [keys.meal]
  delete = ["d"]     # ...but d cancels the day's meals on the meal tab
  ```

  - **A screen's own binding is matched before the tab keys** (`claims`), so a table like
    that trades the dashboard shortcut for it **on that screen and nowhere else**. That is
    what asking for it means; `tabClaimed` remembers which actions a tab named, so only those
    jump the queue.
  - **Globally, a collision with a tab key is refused at startup** (`model.CheckKeys`, called
    from `main` after `ApplyKeys`): the tab keys are matched first, so `delete = ["d"]` in the
    global table put the dashboard on the key and left both delete-a-row and cancel-a-meal
    unreachable while the footer honestly advertised `d` for them. A keymap you cannot drive
    should not reach the alt screen, which is the same reason a misspelled key is refused.
  - `config.LoadKeys` returns **both** maps, and reads `[keys]` as `map[string]any` because
    the table holds two kinds of entry: an action, whose value is a list, and a screen, whose
    value is a sub-table. `--print-keys` writes the global table and the per-screen ones
    commented, so the file documents them without changing a binding.
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
  rebind would leave a destructive prompt advertising a key that no longer accepts it. The
  **keys take the accent** and the slash between them stays dim: accent means "this is what
  to press" everywhere else, and the punctuation is not a key.

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
- `x` on a pulled row commits an `execute_kw` **`unlink`** (`api.DeleteEntry`). The
  row stays on screen until the server confirms — a refused delete must not hide
  hours that still exist. A local row (negative id) is dropped immediately.
- Odoo answers `write`/`unlink` with a bare `true`/`false`, so `false` is treated as
  failure rather than success. A record rule can still refuse someone else's line.
- The MCP at `/mcp` blocks `unlink` on this model; RPC does not. That is the reason
  this path talks RPC rather than MCP.
- Attendance is RPC too (`internal/api/attendance.go`): `hr.employee` `search_read` for the
  employee behind the uid, `hr.attendance` `search_read` for the open session, and
  `hr.employee` `attendance_manual` to toggle. See **The clock** above.
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
- Mode indicator top-right (`-- SEARCH --`), and the mode's keys in the footer, rendered
  from its `key.Binding` set so help can never drift from behavior. The list is **closed by
  default** and toggled with `?` (`Model.showHelp`): open, the list mode's keys wrap to
  three lines, which is worth a screen when you want them and not on every screen forever.
  Closed, the footer still advertises `? keys` — one keystroke, nothing to discover. The
  footer keeps its line either way, so opening the list windows the body rather than moving
  it. `?` is matched where the tab keys are, so the typing modes protect it: `why?` in a
  description stays typeable. A **modal is the exception** — it holds the keyboard, so `?`
  cannot reach the toggle and its `y`/`n` hint shows regardless.
- **A spinner marks every wait** (`bubbles/spinner`, `Model.spin`): in front of the status
  line, and in place of the body's empty state while the first answer is outstanding
  (`reading your tasks…`, `reading this month's hour log…`) — an empty list mid-sync is not
  the answer "no tasks". A re-read that already has a month on screen (`r`, `<`/`>`) leaves
  that month's header and rows up rather than blanking them, so the same spinner sits beside
  the month title instead (`dashHead`) — the chart stays coherent (one answer's header with
  its own rows) while it waits for the next one to replace both at once. It runs while `Model.busy()` — a task sync or write, the month's
  hour log, or a task's lines — and is started in **one place**: `Update` wraps the mode
  handlers and batches `spin.Tick` whenever work went in flight and no tick is scheduled
  (`Model.spinning`), so a new kind of request cannot forget to animate, and two overlapping
  ones cannot double the frame rate. The tick stops itself once nothing is outstanding.

## Theme (`internal/theme`)

| Role | Color |
|---|---|
| accent / focus | `#FFC000` |
| chrome | `#151520` |
| destructive | `#E13400` |
| confirm / complete | `#12CC63` |
| link / tag | `#01B9AE` |
| muted text | `#8A8F99` |
| sick / casual / annual / paternity | `#E13400` · `#01B9AE` · `#7C6BE8` · `#12CC63` |

## Conventions

- `Update` returns `(tea.Model, tea.Cmd)` — never mutate through a pointer receiver
  stored elsewhere; keep the model a value type.
- No I/O in `Update` or `View`. Disk reads/writes go through `tea.Cmd`.
- Filtering is derived from the query on each update; don't keep a second slice in sync.
- Every parsing rule above has a table-driven test in `internal/parse`.
- `go test ./... && go vet ./...` before considering a change done.
