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

- The tab bar is the **first line of every screen** (`view.go: tabBar`), the active tab
  reversed out in the **accent** (`theme.Pill`) — on this line the primary colour marks
  one thing, which screen you are on: `¹tasks  ²dashboard  ³timeoff`. Each tab carries **its position as a
  raised digit** at the top-left of the label, btop style, and the **letter picked out
  inside the word itself** (`hinted`) — accent on an inactive tab, dark ink underlined on
  the pill, where accent on light would fail contrast. Both come off the binding, so a
  rebind shows in the bar with nothing else to edit.
- **Digits are aliases in bar order** (`1`, `2`, `3`). They are matched in the same place as
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
  the modal buys that with the guard the app already has, and leaves `C` free.
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
- **Every week row has a line under it.** That air is what makes a month read as a calendar
  rather than as a table, and it is the design's own proportion — a day there is nearly as tall
  as it is wide. The padding above the month's name and under its weekday heads comes too on a
  terminal tall enough to spend it (`roomyRows`).
- **The days the request line covers are reversed out in the accent** — the same mark a date
  jump leaves on the rows it found. The type it is for is on the request line itself, in its
  own colour, so the day does not have to carry both.
- **Months first, then the holidays** (`timeLayout`): three months outrank the list, and the
  panel takes what is left when that is enough to read a holiday on (`panelMin` to
  `panelMax`). A panel that would leave only one month beside it is dropped instead — that is
  a list with a calendar attached, not a calendar.
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
- It gets its column whenever **two months still fit beside it** (`timePanel`): with one,
  the screen is a list with a calendar attached rather than a calendar. So 88 cells and up
  — three months beside it from about 114 — and below that the dimmed days on the months
  are the whole answer.
### The new-timeoff line (`n`)

One row between the balances and the calendar, and the whole request is on it:
`new timeoff  │Annual ▾│ │full day ▾│ │21/01/26 │ → │23/01/26 │ │description…│ │✓│ │✕│`.

- **The row is there whether or not the line is open.** Closed it is the label alone, with
  `n` in the accent; `n` reveals the fields and focuses the leave type. Nothing above or
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

- **It moves in months, not days.** No cursor — a year is a picture — so `Model.timeHold`
  only says which month the window is built around: `-1` follows **this** month, `h`/`l` step
  one month, `j`/`k` a row of them, `g`/`G` pin it to the ends, and `ctrl+f`/`ctrl+b` move a
  row like `j`/`k`. The four motions are the **same bindings the task list moves by**, so a
  rebind moves both, and a row is however many months the width is showing
  (`view.go: monthMoveHelp` reads their keys back for the footer). This month carries the **caret**
  (`▸Aug`), which is the only thing that says where today is once a leave has taken the
  cells.
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
