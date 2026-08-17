# Hour Logger TUI — Design Spec

Terminal UI in Go for logging work hours and booking canteen meals.
This document is the visual and interaction contract. It does not
prescribe file layout or state management — those are yours.

## Stack

- **Bubble Tea v2** (`github.com/charmbracelet/bubbletea/v2`) — Elm/MVU
- **Lip Gloss v2** — styling and layout
- **Bubbles v2** — `viewport` (scrolling panes), `key` (bindings), `help`
- **Bubblezone** (`github.com/lrstanley/bubblezone`) — mouse hit-testing
- Keybind config in **TOML**, `[[bind]]` array-of-tables

v2 API notes: `Init()` returns `(Model, Cmd)`; key events arrive as
`tea.KeyPressMsg`; module paths are `/v2`-suffixed. If a tutorial shows
`tea.KeyMsg`, it is v1 — see `UPGRADE_GUIDE_V2.md` in the bubbletea repo.

## Hard rules

1. **Measure every string with `lipgloss.Width`, never `len` or
   `utf8.RuneCountInString`.** All layout is fixed-column; one width
   miscalculation skews every cell to its right.
2. **No emoji, no Nerd Font glyphs in aligned regions.** Only
   single-width box-drawing and block characters. Ambiguous-width
   codepoints (including `৳` U+09F3) are banned in grid rows — currency
   is written `Tk`.
3. **Colour is never the only channel.** Every state also differs by
   position, glyph, text weight, or a label.
4. **Fixed content width: 69 columns.** Every row in a pane renders to
   exactly this width, padded if needed.
5. **Every mouse target is also key-reachable.** Enabling mouse
   reporting costs the user their terminal text selection, so keys are
   the primary path and clicks are additive.

---

## Global shell

Five tabs, always visible, always in this order:

```
  alt   projects   tasks   dashboard   meal   timeoff
```

Tab switching is on a **modifier**: `alt+p` `alt+t` `alt+d` `alt+m`
`alt+o`. Aliases `alt+1`..`alt+5` in bar order.

**Why alt and not bare letters:** `t`, `d` and `m` are already bound
inside the Meal tab to *today*, *delete day* and *tomorrow*. Bare
letters cannot serve both layers — `d` would sometimes navigate and
sometimes destroy a day's bookings. Never resolve this by renaming the
tab-local bindings; the modifier layer is the fix.

Timeoff takes `o` (tim**e**o**ff**) because Tasks holds `t`.

**Key hints** are rendered btop-style: the hint character is highlighted
*inside the word itself*, in hint blue. No brackets, no separate legend
to maintain. On the active tab (light pill background) blue-on-light
fails contrast, so the hint switches channel there: dark ink, underlined.

Hint-letter selection rule: **take the first character not already
claimed by a sibling in the same menu.** Never a character that repeats
across siblings — `tomorrow` gets `m`, not `o`, because `o` is also
`today`'s second letter and a hint you have to count characters to find
is not a hint.

Note: some macOS terminals bind alt to composed characters. Ghostty
passes alt through as ESC-prefixed sequences. In Bubble Tea v2 read
these as `tea.KeyPressMsg` with the Alt modifier set, not as a distinct
rune. The numeric aliases exist as the fallback.

---

## Palette

### Shared chrome

| Token | Hex | 256 | Use |
|---|---|---|---|
| Terminal bg | `#16161D` | — | pane background |
| Foreground | `#C8C8D0` | 252 | dates, labels, values |
| Bright | `#E4E4E8` | 255 | today, selection rail, active pill — used sparingly |
| Muted | `#6B6B78` | 243 | secondary labels, help text |
| Hint blue | `#6FA8D4` | 110 | key hint characters only |
| Destructive | `#E05252` | 203 | delete confirmation only |

### Meal tab

| Token | Hex | 256 | Reasoning |
|---|---|---|---|
| Breakfast | `#E8A33D` | 179 | warm amber, morning |
| Lunch | `#DD5F45` | 167 | paprika, the hot main meal |
| Snacks | `#93C572` | 149 | pistachio, nuts and tea |
| Breakfast staged | `#8A6428` | 136 | same hue, ~45% toward bg |
| Lunch staged | `#85392A` | 130 | same hue, ~45% toward bg |
| Snacks staged | `#587643` | 65 | same hue, ~45% toward bg |
| Open slot | `#3E3E47` | 238 | faint, deliberately hueless |
| Selection band | `#262633` | 236 | background only, no hue |
| Weekend / locked | `#55555F` | 240 | present but unbookable |

Amber and paprika hold the warm half, pistachio sits outside it. The
tight pair is breakfast/lunch — under deuteranopia those soften into
each other. Position covers it (bar 1 is always breakfast). If you want
lightness separation too, lift breakfast to `#F0C15C`.

### Dashboard tab

| Token | Hex | Use |
|---|---|---|
| Bar | `#4FA89C` | single colour for all hour bars |
| Bar ink | `#0C201E` | label printed inside the bar |
| Today bar | `#2F6E68` | in-progress fill |
| Today ink | `#DFF2EF` | label inside in-progress bar |
| Off-day band | `#1E1E26` | weekend/holiday, full track width |
| Track & ticks | `#2A2A33` | unfilled remainder, threshold ticks |
| Check in | `#5FBF7F` on ink `#10231A` | button, when checked out |
| Check out | `#E0A030` on ink `#241A08` | button, when checked in |
| Over target | `#5FBF7F` | optional: number tint, ≥8h |
| Under target | `#E0A030` | optional: number tint, 4–8h |
| Well under | `#E0574F` | optional: number tint, <4h |

**Unresolved decision, flagged deliberately:** dashboard amber
(`#E0A030`) is close to Meal breakfast amber (`#E8A33D`), and
dashboard red is close to destructive red. They never share a screen,
so this is safe today — but decide whether these are one shared token
set or two independent ones before adding a third tab that uses colour.
"Roughly the same orange" in two files is how a palette drifts.

---

## Dashboard tab

Hours worked per day, month to date. Horizontal bar chart.

```
  alt   projects   tasks  [dashboard]  meal   timeoff

AUGUST 2026 · month to date                        ⏸ cheCk out
                                              since 09:12 · 5h 28m

  logged  64.2h   target 72h   −7.8h   avg 7.1h   9 of 20 days


  HOURS PER DAY                              ┆ 4h · 8h

  sat  1   [              weekend               ]
  sun  2   [              weekend               ]
  mon  3   ██████████████████ 8.2h┈┈┈┈┈┈┈┈┈┈┈┈
  tue  4   ████████████████ 7.5h┈┈┆┈┈┈┈┈┈┈┈┈┈
  wed  5   ████████████████████ 9.0h┈┈┈┈┈┈┈┈┈┈
  thu  6   ██████████ 6.0h┈┈┈┈┈┈┈┆┈┈┈┈┈┈┈┈┈┈
  fri  7   ██████████████████ 8.0h┈┈┈┈┈┈┈┈┈┈┈┈
  sat  8   [              weekend               ]
  sun  9   [              weekend               ]
  mon 10   ███████████████████ 8.5h┈┈┈┈┈┈┈┈┈┈┈
  tue 11   █████ 3.5h┈┆┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┆┈┈┈
  wed 12   ██████████████████ 8.0h┈┈┈┈┈┈┈┈┈┈┈┈
  thu 13   ████████ 5.5h ▸┈┈┈┈┈┈┈┈┈┆┈┈┈┈┈┈┈┈
  fri 14   [              holiday               ]
  sat 15   [              weekend               ]
  sun 16   [              weekend               ]
           └─────────────────────────────────
           0     2     4     6     8     10
```

(The `[...]` brackets above are ASCII stand-ins for a filled background
band — render them as `#1E1E26` background across the full track, no
bracket characters.)

### Bar rendering

- Scale **3 cells per hour**, track width 30 cells (0–10h).
- Bars are **one colour**. Threshold is not encoded in the bar.
- **The hours label prints inside the filled region**, right-aligned
  against its end, in bar ink on bar colour. Draw the bar to its true
  length, then overwrite its last cells with the label — the label costs
  no width and the bar length stays truthful.
- Reserve ~6 cells for the label. Below roughly 2h there is no room:
  flip the number outside the bar and render it in bar colour as
  foreground.
- **Threshold ticks `┆` at 4h and 8h appear only in the unfilled
  remainder.** Once a bar passes a threshold the tick is redundant, and
  painting it over the fill reads as damage.
- **Today's bar is the darker fill with light ink and a trailing `▸`.**
  A running total must not look identical to a finished one.

### Off days

Weekends and holidays are **rows in the chart, not filtered out** — the
gap between Friday and Monday should not look like two missed days.

- Full-track background band, reason centred inside (`weekend`,
  `holiday`).
- **The band spans the entire width deliberately.** A band stopping
  short would read as hours; a real bar reaching the end would have
  printed a number in it. Width itself distinguishes "no expectation"
  from "worked".
- Holiday labels come from config, not hardcoded. Weekend days are also
  configurable — Sat/Sun here, but Fri/Sat elsewhere in the region, and
  the layout does not care which two go quiet.

### Thresholds

`< 4h` well under · `4h to < 8h` under · `>= 8h` on target.

With single-colour bars, shortfall is carried by **length against the
visible 8h tick**, plus the printed number. Optional enhancement that
preserves the one-colour rule: **tint the number only** — green/amber/red
by threshold, bar stays teal.

Red-amber-green is the classic colourblind trap, so if you adopt the
number tint, keep it redundant: bar length, the visible tick, and the
labelled axis each already carry the same fact. Never reduce this to a
flat row of coloured squares.

### Check in / check out

Top-right corner. Two **distinct** keys, not a toggle:

- `c` — check in. Button reads `⏵ che`**`c`**`k in`, green.
- `C` (shift+c) — check out. Button reads `⏸ che`**`C`**`k out`, amber.

Shift on checkout makes ending a session deliberate; a stray `c` cannot
close the day. Green invites an action not yet taken; amber says a clock
is running and wants closing. Second line under the button shows
`since HH:MM · Xh Ym` when checked in, `last out HH:MM` when out.

Pressing the wrong one is a **no-op with feedback**, never silence:
`already checked in since 09:12`. Silent no-ops make people press harder.

Also required: weekly totals (per week vs 40h), and the button plus
every tab as Bubblezone click targets.

---

## Meal tab

Booking breakfast / lunch / snacks per working day.

### Menus are fixed per weekday

Monday is always the same menu. This is reference data, shown **once**
at the top, never repeated per date:

```
     mon           tue           wed           thu           fri

  ━━ banana·bread  khichuri·egg  ruti·sabji    paratha·egg   nan·dal
  ━━ fried rice    murgi·bhat    mach·dal      beef tehari   biriyani
  ━━ fruits        biscuit·cha   piyaju·cha    shingara·cha  jilapi·cha
```

Consequence: calendar cells hold **no menu text at all** — bar position
already identifies the meal. The grid stores fifteen booleans a week and
nothing more. This is the single largest simplification in the design;
do not reintroduce per-date menu strings.

### Calendar grid

Borderless. Alignment and a blank line between weeks imply the grid — do
not draw a box. Weekday columns 11 cells wide, weekend columns narrowed
and recessed at the right.

```
AUGUST 2026                     ━━ breakfast  ━━ lunch  ━━ snacks  • unsaved

  scope    today    tomorrow    week

  mon        tue        wed        thu        fri        sat    sun

  3          4          5          6          7          8      9
  ━━ ━━ ━━   ━━ ━━ ──   ── ━━ ──   ━━ ━━ ━━   ━━ ── ──

  10         11         12       ▎ 13         14         15     16
  ━━ ━━ ──   ━━ ── ──   ── ━━ ── ▎ ━━ ┄┄ ╌╌ •  ── ── ──
```

Each day cell carries three 2-cell bars in **fixed order: breakfast,
lunch, snacks**. Order never varies — that is what makes colour
confirmation rather than the sole signal.

**Weekends carry no bars at all.** Dates remain for orientation, dimmed.

### Bar states

| State | Glyph | Channel | Meaning |
|---|---|---|---|
| Booked, saved | `━━` | solid, full hue | committed to backend |
| Will add | `┄┄` | fine dash, dimmed hue | staged, unsaved |
| Will remove | `╌╌` | coarse dash, dimmed hue | staged removal, unsaved |
| Open | `──` | thin, hueless | bookable, not taken |
| Locked / past | `━━` | grey, hue removed | past cutoff or past date |
| Weekend | none | no bars rendered | no service |
| Day has staged edits | `•` | bright dot after the cell | unsaved marker |

Dash weight is **reserved for save state**. If a team/headcount view is
added later, express headcount with a trailing digit or a separate row —
not with dash weights.

### Three overlapping cell states

These can all land on the same cell and must stay individually readable,
each using a different channel:

- **Today** — bold + underline on the date number. Fixed, never moves.
- **Selection** — background band plus a bright `▎` left rail. Moves
  with the scope selector.
- **Booked** — the meal-coloured bars. Independent of both.

This separation is precisely why the three meal hues never had to be
compromised. Preserve it.

### Scope selector

`t` today · `m` tomorrow · `w` week. The selected scope **bands the
matching dates in the calendar**:

- **today** / **tomorrow** — band exactly one column.
- **week** — band Mon→Fri as one continuous block with the left rail, so
  it reads as a range rather than five separate picks.

Detail pane below reflects the scope. Single-day scope shows that day's
menu with per-meal on/off. Week scope lists all five days with their
menus and states.

Keys: `b` `l` `s` toggle one meal. `B` `L` `S` toggle that meal across
the whole week — the reason a week scope exists at all.

This retires any separate "go to today" binding; `t` already does it.

### Staged edits and save

**Nothing writes to the backend on toggle.** Toggles stage. Footer shows
`N unsaved`. Days with pending changes get the bright `•`, so an unsaved
calendar is visibly unsaved at a glance.

`enter` opens the save confirmation. It shows a **diff, not a
restatement** — only what changes, signed `+` and `−`, with the cutoff
time inside the modal because that is the fact that makes the decision
urgent.

```
┌ confirm save ──────────────────────────┐
│                                        │
│  thu 13 aug  ·  cutoff 21:00, 6h 12m   │
│                                        │
│  +  ━━  lunch      beef tehari         │
│  −  ━━  snacks     shingara·cha        │
│                                        │
│     enter save        esc cancel       │
└────────────────────────────────────────┘
```

Modals are the **one place a drawn border is justified** — they must
visibly float above a grid still partly visible behind them. `esc`
returns to the staged state without discarding it.

### Delete

Two different operations, deliberately different weight:

- **Turning one meal off** is an ordinary staged edit (`╌╌`),
  reversible, saved with everything else.
- **Clearing a whole day** is destructive: own modal, own key `d`.

```
┌ delete all meals ──────────────────────┐
│                                        │
│  thu 13 aug — 3 booked meals           │
│                                        │
│  ━━ breakfast   ━━ lunch   ━━ snacks   │
│                                        │
│  after 21:00 this cannot be undone     │
│                                        │
│     y delete          esc keep         │
└────────────────────────────────────────┘
```

Destructive confirm takes **`y`, not `enter`** — muscle memory from the
save dialog must not be able to wipe a day. It lists the three named
meals that would be lost rather than asking "are you sure": three named
meals is information, a yes/no question is not. Destructive red appears
here and nowhere else, which is what makes it mean something.

### Meal statistics

The meal charts (take rate per meal, daily matrix, by-weekday pattern,
weekly spend) belong **inside the Meal tab as a sub-view**, not on the
Dashboard. Dashboard is hours only.

Daily meal history is a **matrix, not a stacked bar** — a day can have
snacks without breakfast, and stacking would hide that.

---

## Keymap

### Global (modifier layer)

| Key | Action |
|---|---|
| `alt+p` / `alt+1` | Projects tab |
| `alt+t` / `alt+2` | Tasks tab |
| `alt+d` / `alt+3` | Dashboard tab |
| `alt+m` / `alt+4` | Meal tab |
| `alt+o` / `alt+5` | Timeoff tab |
| `q` | quit |
| `?` | help |

### Dashboard

| Key | Action |
|---|---|
| `c` | check in |
| `C` | check out |
| `n` / `p` | next / previous month |

### Meal

| Key | Action |
|---|---|
| `t` | scope: today |
| `m` | scope: tomorrow |
| `w` | scope: week |
| `b` `l` `s` | toggle breakfast / lunch / snacks |
| `B` `L` `S` | toggle that meal across the whole week |
| `j` `k` | move day (week scope) |
| `enter` | save staged changes |
| `esc` | discard staged changes |
| `d` | delete whole day (confirm with `y`) |

Bindings live in TOML using `[[bind]]` array-of-tables records, which
avoids special characters as map keys entirely:

```toml
[[bind]]
key = "b"
action = "toggle_breakfast"

[[bind]]
key = "C"
action = "check_out"
```

TOML rather than YAML deliberately: YAML coerces `y`/`n` to booleans and
treats `>`, `*`, `#` as reserved — both fatal for a keybind file.

---

## Build order

1. Model, tab shell, tab switching, key hints. No data.
2. Dashboard read-only: bar rows, off-day bands, axis, inside-bar labels.
   Get `lipgloss.Width` discipline right here; everything later inherits it.
3. Check in/out with live elapsed timer.
4. Meal calendar read-only: grid, bars, today, weekends.
5. Scope selector and banding.
6. Staged edits, `•` markers, save modal with diff.
7. Delete modal.
8. Bubblezone mouse targets.
9. Meal statistics sub-view.

Verify at every step in a real terminal at 80 columns, and check one row
of each pane renders to exactly 69 cells. Alignment bugs compound
silently.
