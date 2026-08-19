# Time Off page — spec

A second full-screen view inside `tsk` (see `CLAUDE.md`): a year calendar of my time off,
four balance boxes, a public-holiday panel, and a single-line request form.

Reference prototype: `Time Off Calendar.dc.html` in the design project.

## Layout

```
┌────────────────────────────────────────────────────────────────────────────┐
│ tsk timeoff — 2026                                        -- CALENDAR --   │  title bar
├──────────────┬──────────────┬──────────────┬──────────────────────────────┤
│ sick Time Off│casual TimeOff│annual TimeOff│ paternity Time Off           │  balances
│      9       │      6       │     8.5      │        0                     │
│ DAYS AVAILABLE …                                                          │
├────────────────────────────────────────────────────────────────────────────┤
│ new timeoff   [fields appear here only after `n`]                    ✓  ✕ │  request line (fixed height)
├──────────────────────────────────────────────┬─────────────────────────────┤
│ Jan   Feb   Mar        (scrollable, 3 cols)  │ PUBLIC HOLIDAYS             │
│ Apr   May   Jun                              │ Feb 4  : Shab e-barat*      │
│ …                                            │ …                           │
├──────────────────────────────────────────────┴─────────────────────────────┤
│ ● 21/01/26  Wed   CASUAL   Family errand            22 days taken in 2026  │  detail line
├────────────────────────────────────────────────────────────────────────────┤
│ n new timeoff   ctrl+f/ctrl+b scroll half page   t today   s c a p filter   │  help footer
└────────────────────────────────────────────────────────────────────────────┘
```

Only the calendar grid and the holiday list scroll. Balance boxes, request line,
detail line, and footer are fixed. The request line keeps its height whether or not
the form is open, so revealing the form must not shift the calendar.

## Domain

```go
type Kind int // Sick, Casual, Annual, Paternity

type Leave struct {
    Date   string // "2026-01-21"  (yyyy-mm-dd, one record per day)
    Kind   Kind
    Desc   string
    Half   string // "" | "morning" | "afternoon"
}

type Holiday struct {
    From, To string // inclusive range, yyyy-mm-dd
    Name     string
}

type Balance map[Kind]float64 // days available, e.g. Annual: 8.5
```

A multi-day request expands into one `Leave` per day at apply time.

## Colors (`internal/theme`)

| Role | Color |
|---|---|
| sick | `#E13400` |
| casual | `#01B9AE` |
| annual | `#7C6BE8` (violet — deliberately not accent) |
| paternity | `#12CC63` |
| accent / focus | `#FFC000` |
| chrome | `#151520` / panel `#12121C` |
| muted text | `#8A8F99`, dim day `#5E636E` |

Annual must not use the accent color — focus rings are accent and would be ambiguous.

## Calendar rendering

- 12 months, 3 per row, Monday-first, ISO week number in a leading `wk` column.
- Weekends **and public holidays** render dim: muted text on a faint
  `rgba(255,255,255,0.045)` background. Holidays are visually identical to weekends.
- A day with leave renders as a filled circle in the leave-type color with dark text.
- A **half day** renders as a half-filled circle (left half color, right half faint).
- The month containing the current selection is tinted and its label shown in accent
  with a `▸` caret; the month header also shows a leave-day count (`3d`).
- Days already taken but filtered out render dim, not hidden.

## Keymap — calendar mode

- `n` — open the new-timeoff form, focus leave type
- `ctrl+f` / `ctrl+b` — scroll the calendar half a page down / up
- `t` — jump to today (scrolls that month into view)
- `s` `c` `a` `p` — toggle filter by sick / casual / annual / paternity; pressing the
  same letter again clears it (`esc` also clears). Each balance box shows its first
  letter lowercased and drawn in accent (**s**ick Time Off, **c**asual Time Off,
  **a**nnual Time Off, **p**aternity Time Off)
- `q` / `ctrl+c` — quit

No per-day or per-month cursor keys. There is no year switching.

Filtering only changes which leave circles are drawn; the mode indicator becomes
`-- ANNUAL TIME OFF --` and the detail line's right side reads `FILTER: …`.

## Request line

Hidden until `n`. Label is always visible: `new timeoff` with the `n` in accent.
When open, all controls sit on one line:

```
new timeoff  [Annual ▾] [full day ▾] [21/01/26] → [23/01/26] [description…]   ✓  ✕
```

Fields, in tab order:

1. **leave type** — dropdown: Sick / Casual / Annual / Paternity (`j`/`k` cycles)
2. **duration** — dropdown: `full day` / `half day` (`j`/`k` or `space` toggles)
3. **date 1** — text field
4. **date 2** (full day) — text field, the range end
   **period** (half day) — dropdown: `morning` / `afternoon`
5. **description** — text field
6. **✓** — review
7. **✕** — reset the line to a blank request

Half day replaces the second date picker with the period dropdown, and the request
covers exactly one day.

### Keys — form mode

- `tab` / `shift+tab` — move between fields
- `enter` on a text or dropdown field — same as tab
- `j` / `k` / `space` — change the focused dropdown
- `ctrl+u` — clear the focused text field
- `enter` on ✓ — confirm modal
- `enter` on ✕ — reset all fields
- `esc` — discard modal if anything was changed, otherwise close the line

### Date entry

Dates are `dd/mm/yy`. Typing is normalized **on field exit**, relative to the
currently focused calendar date (the form's base date):

```
21       → 21/01/26   (day only: keep month + year)
21/3     → 21/03/26   (day + month: keep year)
21/3/26  → 21/03/26
```

Tabbing into a date field shows the current value **selected** (accent background);
the first keystroke replaces the whole value, so `21` shows just `21`. `tab` with no
input keeps the previous value. So `n` → tab → `21` → tab → `23` → tab lands on
description with `21/01/26` and `23/01/26` filled in.

The second date is normalized against the **first date**, not today, so a range that
crosses a month works: `21/3` then `2/4`.

### Live preview

While the form is open, every day the request covers is highlighted in the selected
leave-type color with an accent ring, and the calendar scrolls to that month. Partial
input counts: typing `21` already highlights. When editing the second date, the view
follows the second date.

Reversed ranges are accepted and normalized (`to` before `from` swaps).

## Confirm modals

Both are the same modal shell; they swallow all keys except `y` / `n` / `enter` / `esc`.

Apply (from ✓) — header `CONFIRM TIME OFF` in accent:

```
TYPE         ANNUAL TIME OFF
DATES        21/01/26  →  23/01/26
DURATION     3 days                 (or "half day · morning")
DESCRIPTION  Coast trip
                                    [ yes ]  [ no ]
```

Discard (from `esc` with unsaved input) — header `DISCARD TIME OFF REQUEST` in
`#E13400`, same four rows.

Actions are **yes then no**, in that order, with the `y` and `n` letters in accent.
`y` / `enter` proceeds; `n` / `esc` returns to the form with everything intact.

Applying writes one `Leave` per day in the range, closes the form, and moves the
selection to the first day of the range.

## Public holidays panel

Right of the calendar, 268px, own scroll, header `PUBLIC HOLIDAYS` + `16 in 2026`.
One compact line per holiday, date column left-aligned and fixed width:

```
Aug 5  : July Uprising day
Mar 18-23 : Eid-ul-Fitar* & Jumatul Bidah
```

Single-month ranges collapse to `Mar 18-23`; cross-month to `Mar 30-Apr 2`. Long
names truncate with an ellipsis. The holiday whose month is the active month is
highlighted (accent left border and date).

Holidays feed the calendar dim-day set. Source them from a config file
(`~/.config/tsk/holidays.json`) rather than hardcoding.

## Detail line

Left: selected date + weekday, a leave-type chip (`CASUAL`, `ANNUAL · MORNING`, or
`NO LEAVE`), and the description — or the holiday name, or `working day`.
Right: `22 days taken in 2026`, or `FILTER: ANNUAL TIME OFF` when filtered.

## Implementation notes

- Balances are read from the store and displayed as-is (they can be fractional, `8.5`);
  the number is drawn in the leave-type color, greyed out when zero.
- Holiday and weekend lookups are precomputed into a `map[string]bool` once per render
  of the year, not per cell.
- The date parser is a pure function with a table-driven test covering every case above,
  including reversed ranges and the second-date-relative-to-first rule.
- Form state is one struct on the root model (`kind`, `half`, `period`, `d1`, `d2`,
  `desc`, `field`, `fresh`); `fresh` marks a date field whose value is selected and
  will be replaced by the next keystroke.
- The confirm modal is a field on the root model, not a separate program.
- `ctrl+f`/`ctrl+b` operate on the calendar `viewport.Model` only; the panels are
  separate viewports.
