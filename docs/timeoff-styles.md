# Time off calendar — visual style spec

Extracted from `Time Off Calendar.dc.html`. Web values first, terminal translation
second, since the target is a Bubble Tea TUI rendered with lipgloss.

## Palette

| Role | Hex | Where |
|---|---|---|
| page / desk | `#0B0B10` | outside the app frame |
| frame surface | `#151520` | calendar body, modal body |
| chrome bar | `#101018` | title bar, footer hint bar |
| raised strip | `#12121C` | request line, holidays panel |
| balance strip | `#14141F` | four balance boxes, detail line |
| input well | `#0E0E16` | unfocused field background |
| hairline | `rgba(255,255,255,0.07)` | horizontal section dividers |
| grid hairline | `rgba(255,255,255,0.06)` | between month cells |
| weekend / holiday fill | `rgba(255,255,255,0.045)` | dimmed day background |
| accent (focus) | `#FFC000` | cursor, focus ring, active month, hotkey letters |
| sick | `#E13400` | |
| casual | `#01B9AE` | |
| annual | `#7C6BE8` | violet — deliberately NOT accent |
| paternity | `#12CC63` | |

Text ramp, brightest to dimmest:

```
#FFFFFF  focused value, active month name, selected date
#E6E7EA  default body text
#C9CCD3  day numbers (working days), field values, holiday names
#9AA0AA  inactive month names, holiday dates
#8A8F99  weekday heads, footer hint labels, meta
#6C717C  unit labels (DAYS AVAILABLE), title-bar text, mode indicator
#5E636E  weekend/holiday day numbers, placeholder text, dropdown arrows
#4E525B  filtered-out leave days, the `:` separator
#3E424B  week numbers, empty state dot
#2A2A38  progress-bar track, scrollbar thumb
```

Anything sitting on accent or on a leave color uses `#0B0B10`, never white.

## Frame

- outer radius `10px`, border `1px solid rgba(255,255,255,0.09)`,
  shadow `0 24px 60px rgba(0,0,0,0.55)`
- title bar: three 10px dots (`#E13400` `#FFC000` `#12CC63`), centered
  `tsk timeoff — 2026` at 12px `#6C717C`, right-aligned mode indicator
  `-- CALENDAR --` at 11px, letter-spacing `0.08em`
- every section is separated by a single `rgba(255,255,255,0.07)` rule, never a gap

## Balance boxes

Four equal columns on `#14141F`, `1px` vertical dividers between them, no divider after
the last. Each column is centered, `14px 16px 15px` padding, `2px` gap:

1. name at 12px `#9AA0AA` — first letter lowercase and `#FFC000` bold (**s**ick Time Off)
2. number at 26px bold, letter-spacing `-0.02em`, line-height `1.25`, colored in that
   leave type's color; `#5E636E` when the balance is 0
3. `DAYS AVAILABLE` at 11px `#6C717C`, letter-spacing `0.08em`

Active filter adds a `2px` top border in the type color and a
`rgba(255,255,255,0.04)` background.

## Request line

Fixed `52px` height so revealing it never shifts the calendar. Padding `0 16px`,
`14px` gap, `nowrap`, `overflow: hidden`.

Field chip, resting and focused:

```
font-size    12px
padding      4px 9px
radius       5px
gap          6px
resting      bg #0E0E16   border 1px rgba(255,255,255,0.09)   text #C9CCD3 (#5E636E if empty)
focused      bg rgba(255,192,0,0.12)   border 1px #FFC000   text #FFFFFF
selected     bg #FFC000   text #0B0B10 bold        (date field just tabbed into)
```

- dropdown arrow `▾` at 9px, `#FFC000` when focused else `#6C717C`
- leave-type chip renders its label in that type's color, weight 500
- description chip: `min-width: 180px`, `flex: 1 1 auto`
- range separator `→` at 12px `#5E636E`
- cursor is a `█` in accent, `blink 1s steps(1) infinite`
- ✓ / ✕ pushed right with `margin-left: auto`, `8px` apart, `4px 10px` padding,
  radius `5px`, `1px` border at 50% alpha of `#12CC63` / `#E13400`; focused inverts to
  a solid fill with `#0B0B10` (✓) or `#FFFFFF` (✕) text

## Month grid

Three columns, whole grid capped at `560px` with `overflow-y: auto`
(`scrollbar-color: #2A2A38 #151520`). Each month cell:

```
padding        14px 18px 16px
border-right   1px rgba(255,255,255,0.06)   (columns 1-2 only)
border-bottom  1px rgba(255,255,255,0.06)   (rows 1-3 only)
active month   background rgba(255,192,0,0.045)
```

Header row: month name `Jan 26` at 12.5px — active month is bold `#FFC000` prefixed
`▸ `, inactive is weight 500 `#9AA0AA` prefixed with two spaces so nothing shifts. Right
side shows the leave count for that month (`3d`) at 10.5px `#5E636E`.

Day grid is `26px repeat(7, 1fr)` with `1px` gap:

- `wk` head at 9.5px `#4E525B`; week numbers same color, `line-height: 22px`
- weekday heads `M T W T F S S` at 10.5px, `#8A8F99` weekdays / `#5E636E` weekend,
  `3px` bottom padding
- day cell: `22px` tall flex-centered, 11.5px, `border-radius: 999px`,
  `font-variant-numeric: tabular-nums`

Day cell states, in priority order:

| State | Style |
|---|---|
| pending (typing a date) | fill = leave-type color, `#0B0B10` bold, ring `0 0 0 2px #FFC000, 0 0 0 4px #151520` |
| leave taken | fill = leave-type color, `#0B0B10` bold |
| half day | same fill, but only half the cell — see terminal note |
| cursor | fill `#FFC000`, `#0B0B10` bold |
| weekend or public holiday | text `#5E636E`, background `rgba(255,255,255,0.045)` |
| working day | text `#C9CCD3`, transparent |
| leave hidden by a filter | text `#4E525B`, transparent |

## Public holidays panel

`268px` fixed width to the right of the grid, `border-left: 1px rgba(255,255,255,0.08)`,
background `#12121C`, capped at the same `560px` with its own scroll.

- header: `PUBLIC HOLIDAYS` 12px bold letter-spacing `0.08em` `#C9CCD3`, right side
  `16 in 2026` at 10.5px `#5E636E`, bottom hairline
- rows are one line: `padding 3px 14px`, `6px` gap, baseline-aligned
  - date at 10.5px, fixed `62px` column, `nowrap` — `Aug 5`, `Mar 18-23`
  - `:` at 10.5px `#4E525B`
  - name at 10.5px `#C9CCD3`, truncated with ellipsis
- row for the active month: `2px` left border `#FFC000`, background
  `rgba(255,192,0,0.05)`, date `#FFC000`, name `#FFFFFF`

## Detail line

On `#14141F`, `10px 16px`, `14px` gap: an 8px dot in the leave color (`#3E424B` when
none), the date + weekday at 12.5px `#FFFFFF` weight 500, then a type chip — 10px bold,
letter-spacing `0.08em`, `2px 7px`, radius `4px`, filled with the leave color and
`#0B0B10` text, or `#6C717C` on a `rgba(255,255,255,0.12)` outline for `NO LEAVE`. Note
text 12px `#8A8F99`. Right side: the summary/filter string at 11px `#6C717C`.

## Footer hints

On `#101018`, `10px 16px`, `16px` gap. Each hint is a key badge plus a label: badge is
`#23232F` background, `#FFC000` bold, `2px 6px`, radius `4px`; label 11px `#8A8F99`.
The set swaps entirely per mode — never show a key the current mode ignores.

## Modals

Centered on `rgba(8,8,12,0.72)`. Width `460px`, background `#151520`, radius `8px`,
border `1px solid rgba(255,192,0,0.4)`, shadow `0 24px 60px rgba(0,0,0,0.6)`.

- header strip: `#1C1C29`, 11px letter-spacing `0.12em`, `#FFC000` for confirm /
  `#E13400` for discard
- body: `96px 1fr` grid, `8px 12px` gap, 12.5px — labels `#6C717C`, values `#FFFFFF`,
  the type value in its leave color bold
- footer: right-aligned, **yes then no**, each `6px 14px` on `#23232F` radius `6px`,
  with the `y` / `n` letter in `#FFC000` bold

## Terminal translation notes

- Rounded day chips become a padded cell: render ` 21 ` with
  `lipgloss.NewStyle().Background(color).Foreground(lipgloss.Color("#0B0B10")).Bold(true)`.
  Terminal cells can't be circular — don't try to fake it with `()`.
- The focus ring has no terminal equivalent. Use reverse video, or wrap the cursor day
  in `[21]` while keeping the leave background.
- Half days can't be a split background. Use `◐` after the number, or underline the cell.
- Weekend/holiday dimming: a slightly lighter background works in truecolor terminals;
  fall back to `Faint(true)` on the number when only 256 colors are available.
- Keep every section width-fixed and pad to a constant height — the calendar must not
  reflow when the request line opens or the mode changes.
- Blinking cursor: `lipgloss` has no animation; drive it from a `tea.Tick` every 500ms
  and swap `█` for a space.
