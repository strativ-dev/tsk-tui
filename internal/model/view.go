package model

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"

	"github.com/tasnimAlam/tsk/internal/api"
	"github.com/tasnimAlam/tsk/internal/parse"
	"github.com/tasnimAlam/tsk/internal/store"
	"github.com/tasnimAlam/tsk/internal/theme"
)

const (
	barWidth = 12
	// caretCol is the gutter in front of the search box: " ❯ " when the field has
	// focus, three spaces when it does not, so the header never shifts sideways.
	caretCol = 3
	// progReserve is the widest the progress cluster gets — "TODAY 100% · +12h30m
	// unsynced · 22/22 tasks" and a little room. The search field is sized against
	// this rather than against the cluster as currently rendered: the box shrinks
	// when the TODAY line grows, and a field wider than the box wraps inside it,
	// which grows the header a line and shoves the whole list down.
	progReserve = 46
	// gutter is the indent every line shares, one cell of which the focus border
	// or the blur padding occupies.
	gutter = 2
	// Used until the first WindowSizeMsg lands.
	fallbackWidth  = 100
	fallbackHeight = 24

	// Screen rows a collapsed task and a table row each cost, and the rows the
	// header, footer and status take away from the list. ctrl+f / ctrl+b use these
	// to jump by half of what is actually on screen.
	taskLines  = 2 // the task line plus the blank one after it
	entryLines = 1
	chromeRows = 8

	// Table columns, shared by the head, the rows and the insert line.
	tableIndent = "    "
	colGap      = "   "
	// One cell wider than the value they hold: the insert row puts a textinput in
	// each of these columns, and an input always draws a cursor cell after its
	// Width. At dateWidth 8 a normalized 12/08/26 did not fit its own input, so the
	// field scrolled and showed 2/08/26.
	dateWidth  = 9 // dd/mm/yy
	hoursWidth = 6 // h:mm, or hh:mm

	// Dashboard chart. dashScale is the hours the full bar stands for — 10, so an 8h
	// day sits short of the end and overtime still has somewhere to go. dashBars caps
	// the bar on a wide terminal; dashLabel holds "mon 17".
	dashScale = 10.0
	dashBars  = 34
	dashLabel = 6
	// Screen rows one day costs when the chart is ruled: its bar and the rule under it.
	// ctrl+f / ctrl+b move by half a screen of these.
	dashRowLines = 2
	// minDayRows is how few rows of days the chart will squeeze into before it gives up
	// its pinned head and axis: two days and the rule between them.
	minDayRows = 3
	descCap    = 48
)

// View stacks a fixed header, a windowed list, and a fixed footer. The header and
// footer are laid out first and the list gets whatever rows are left, so the
// search field can never be pushed off the top of the screen.
func (m Model) View() string {
	head := []string{m.tabBar()}
	if m.tab == TabTasks {
		// The query field filters tasks, so it belongs to that tab and nowhere else.
		head = append(head, strings.Split(m.header(), "\n")...)
	}
	head = append(head, theme.Sep.Render(strings.Repeat("─", m.cols())), "")

	var tail []string
	switch m.mode {
	case ModeConfirm:
		// Read off the bindings themselves. Spelling the keys out here meant a rebind
		// could leave a destructive prompt advertising a key that no longer accepts it.
		hint := m.confirmKeys().Help().Key + " / " + keys.No.Help().Key
		tail = append(tail, strings.Split(
			theme.Modal.Render(m.cPrompt+"  "+theme.Dim.Render(hint)), "\n")...)
	case ModeAuth:
		tail = append(tail, strings.Split(m.authModal(), "\n")...)
	case ModeJump:
		// Above the status line rather than inside a task: a jump reaches every task,
		// so it does not belong to the one under the cursor.
		tail = append(tail, theme.Blur.Render(
			theme.Prompt.Render("jump to date ")+m.jump.View()))
	case ModeDay:
		tail = append(tail, strings.Split(m.dayModal(), "\n")...)
	}
	// Flattened and cut to the width: a server message can arrive with newlines in it,
	// and a status line that wraps costs the list a row it was not given.
	if m.err != nil {
		tail = append(tail, theme.Blur.Render(theme.Err.Render(
			trunc("! "+oneLine(m.err.Error()), m.cols()-gutter))))
	}
	if m.status != "" {
		// The spinner goes in front of the status line while a request is out, so a wait
		// on a screen that already has content still shows as movement rather than as a
		// sentence that might be stale.
		lead, room := "", m.cols()-gutter
		if m.busy() {
			lead, room = m.spin.View()+" ", room-2
		}
		tail = append(tail, theme.Blur.Render(
			lead+theme.Dim.Render(trunc(oneLine(m.status), room))))
	}
	tail = append(tail, strings.Split(m.footer(), "\n")...)

	// The chart's month totals and its axis are pinned above and below the days rather
	// than scrolling with them: they are what the bars are read against, so scrolling
	// them off the screen loses the scale the colours lean on. A terminal too short for
	// both gives the days back their rows — the axis first, then the totals, since a
	// chart with two visible days is not a chart.
	if m.tab == TabDash {
		dHead, dFoot := m.dashHead(), m.dashFoot()
		for _, frame := range []*[]string{&dFoot, &dHead} {
			if m.rows()-len(head)-len(tail)-len(dHead)-len(dFoot) >= minDayRows {
				break
			}
			*frame = nil
		}
		head = append(head, dHead...)
		tail = append(dFoot, tail...)
	}

	// The body takes the rows left between header and footer, and is padded out to
	// them, which pins the status line and the key hints to the bottom of the screen.
	budget := m.rows() - len(head) - len(tail)
	body, focus := m.listLines()
	if m.tab == TabDash {
		body, focus = m.dashLines(budget)
	}
	body = window(body, focus, budget)
	for len(body) < budget {
		body = append(body, "")
	}

	out := make([]string, 0, len(head)+len(body)+len(tail))
	out = append(out, head...)
	out = append(out, body...)
	out = append(out, tail...)
	return strings.Join(out, "\n")
}

// window trims lines to n, keeping the focused line in view and saying how many
// lines it hid — a silently cut list reads as a complete one.
func window(lines []string, focus, n int) []string {
	if n < 1 {
		return nil
	}
	if len(lines) <= n {
		return lines
	}

	start := 0
	if focus >= 0 {
		start = focus - n/2 // keep the cursor near the middle once scrolling starts
	}
	if start > len(lines)-n {
		start = len(lines) - n
	}
	if start < 0 {
		start = 0
	}

	out := make([]string, n)
	copy(out, lines[start:start+n])
	if start > 0 {
		out[0] = theme.Blur.Render(theme.Dim.Render(fmt.Sprintf("↑ %d more", start)))
	}
	if hidden := len(lines) - start - n; hidden > 0 {
		out[n-1] = theme.Blur.Render(theme.Dim.Render(fmt.Sprintf("↓ %d more", hidden)))
	}
	return out
}

func (m Model) cols() int {
	if m.width < 40 {
		return fallbackWidth
	}
	return m.width
}

func (m Model) rows() int {
	if m.height < 10 {
		return fallbackHeight
	}
	return m.height
}

// halfPage is how far ctrl+f / ctrl+b move: half of what fits on screen, in items
// of the given height. Never zero, or the keys would look broken.
func (m Model) halfPage(linesPerItem int) int {
	fits := (m.rows() - chromeRows) / linesPerItem
	if fits < 4 {
		return 1
	}
	return fits / 2
}

// tabBar is the row of screens across the top, the active one reversed out. The key
// that reaches a tab is highlighted inside the word itself — btop style, no brackets
// and no separate legend to keep in step. On the pill, accent on light would fail
// contrast, so the hint switches to dark ink and an underline.
func (m Model) tabBar() string {
	tabs := []struct {
		tab   Tab
		label string
		key   key.Binding
	}{
		{TabTasks, "tasks", keys.TasksTab},
		{TabDash, "dashboard", keys.DashTab},
	}

	var b strings.Builder
	b.WriteString("  ")
	for i, t := range tabs {
		active := t.tab == m.tab
		body, hint := theme.Dim, theme.HintKey
		if active {
			body, hint = theme.Pill, theme.PillKey
		}
		// A superscript index sits at the top-left of the label, btop style, and is a key
		// in its own right: 1 and 2 in bar order, alongside the letter.
		b.WriteString(body.Render(" ") + hint.Render(superscript(i+1)) +
			hinted(t.label, t.key, body, hint) + body.Render(" "))
		b.WriteString("  ")
	}
	return b.String()
}

// hinted renders label with the binding's key picked out inside it. A key that is not
// a letter of the word — a rebind to alt+d, say — is spelled out after it instead,
// since there is nothing in the word to highlight.
func hinted(label string, b key.Binding, body, hint lipgloss.Style) string {
	k := b.Help().Key
	if len([]rune(k)) == 1 {
		if i := strings.Index(label, k); i >= 0 {
			return body.Render(label[:i]) + hint.Render(k) + body.Render(label[i+len(k):])
		}
	}
	return body.Render(label) + theme.Dim.Render(" "+k)
}

// superscript is a tab's position as a raised digit, so the number reads as a label on
// the word rather than another word beside it. Beyond nine it falls back to the digit.
func superscript(n int) string {
	const raised = "⁰¹²³⁴⁵⁶⁷⁸⁹"
	if n < 0 || n > 9 {
		return fmt.Sprint(n)
	}
	return string([]rune(raised)[n])
}

// dashHead is the chart's frame above the days: the month, its totals against the
// target, and what a tick inside a bar means. It is laid out with the header rather than
// with the body, so the days scroll under numbers that stay put.
func (m Model) dashHead() []string {
	if len(m.dashDays) == 0 {
		return nil
	}

	logged, target, worked, workdays := m.dashTotals()
	month := time.Now()
	if t, err := time.Parse("2006-01-02", m.dashMonth); err == nil {
		month = t
	}

	return []string{
		// No "month to date" note: the target beside `logged` is the whole month, and the
		// last row is today, marked as today, which says where the days stop.
		theme.Blur.Render(theme.Title.Render(strings.ToUpper(month.Format("January 2006")))),
		"",
		theme.Blur.Render(theme.Dim.Render("logged  ") + monthBar(logged, target) +
			"  " + theme.Total.Render(hoursLabel(logged)) +
			theme.Dim.Render(" / "+hoursLabel(target)) + "   " + m.todayDelta() +
			theme.Dim.Render(fmt.Sprintf("   %d of %d days", worked, workdays))),
		"",
		theme.Blur.Render(theme.Header.Render("HOURS PER DAY") +
			theme.Dim.Render("   ") + theme.Track.Render("┆") + theme.Dim.Render(" 4h · 8h")),
		"",
	}
}

// todayDelta is where today stands against the hours expected of it — the one figure on
// that line you can still do something about. The month's own gap is left to the bar and
// the two numbers beside it: a shortfall spread over three weeks is not a number anybody
// acts on before the end of the day.
func (m Model) todayDelta() string {
	today := time.Now().Format("2006-01-02")
	for _, d := range m.dashDays {
		if d.Date != today {
			continue
		}
		switch {
		case d.Expected == 0:
			// Weekend, holiday, leave: nothing was expected, so there is no gap.
			return theme.Dim.Render("today off")
		case d.Actual >= d.Expected:
			return theme.Dim.Render("today ") + theme.High.Render("+"+hoursLabel(d.Actual-d.Expected))
		default:
			return theme.Dim.Render("today ") + theme.Low.Render("−"+hoursLabel(d.Expected-d.Actual))
		}
	}
	// The ERP did not report today at all — say so rather than implying a full day owed.
	return theme.Dim.Render("today —")
}

// dashFoot is the ruler under the bars, pinned to the bottom of the chart for the same
// reason as the head: the colours are read against a labelled axis, which says nothing
// once it has scrolled off screen.
//
// The two lines are handed over separately — window() and the row budget count elements,
// so one element holding a newline would make the view a row taller than the terminal.
func (m Model) dashFoot() []string {
	if len(m.dashDays) == 0 {
		return nil
	}
	out := []string{""}
	for _, l := range strings.Split(dashAxis(m.barCells()), "\n") {
		out = append(out, theme.Blur.Render(l))
	}
	return out
}

// dashLines is the body of the chart: one row per day of the **whole** month, so the
// holidays and weekends still to come are on it, with the index of today's row — the row
// the window is built around when the month is taller than the terminal. Derived from
// dashDays on every render, like every other body.
//
// budget is the rows it has. The rule between days is what it gives up first when the
// month does not fit: separation is worth a line each until the lines are what is scarce.
func (m Model) dashLines(budget int) ([]string, int) {
	if len(m.dashDays) == 0 {
		if m.dashLoading {
			return []string{theme.Blur.Render(
				m.spin.View() + theme.Dim.Render(" reading this month's hour log…"))}, -1
		}
		return []string{theme.Blur.Render(
			theme.Dim.Render("no hour log yet — r to read this month"))}, -1
	}

	// One rule between days, none after the last: a bar sits against its own row rather
	// than in a column of colour, which is the only way a stack of full days reads as
	// separate days. Ruled, a month costs 2n-1 rows — dropped, it may fit whole, and a
	// whole month on screen beats a windowed one now that the chart takes no motions.
	barCells := m.barCells()
	ruled := 2*len(m.dashDays)-1 <= budget || len(m.dashDays) > budget
	rule := theme.Blur.Render(theme.Track.Render(
		strings.Repeat("─", dashLabel+2+barCells)))
	today, hold, focus := time.Now().Format("2006-01-02"), m.dashDayIndex(), -1
	var lines []string
	for i, d := range m.dashDays {
		if i > 0 && ruled {
			lines = append(lines, rule)
		}
		lines = append(lines, theme.Blur.Render(m.dashRow(d, barCells, today)))
		if i == hold {
			focus = len(lines) - 1 // the row the window is built around
		}
	}
	return lines, focus
}

// barCells is the width of the bar area: what the day label and the hours leave over,
// capped so a wide terminal does not stretch one day across the screen. The bars and the
// axis under them read it, so they cannot disagree.
func (m Model) barCells() int {
	n := m.cols() - gutter - dashLabel - 8
	if n > dashBars {
		n = dashBars
	}
	if n < 8 {
		n = 8
	}
	return n
}

// dashRow is one day: weekday and date, then either why nothing was expected of it or
// a bar of the hours that counted against the hours that were due.
func (m Model) dashRow(d api.DayLog, barCells int, today string) string {
	day, err := time.Parse("2006-01-02", d.Date)
	label := pad(d.Date, dashLabel)
	if err == nil {
		label = pad(strings.ToLower(day.Format("Mon"))+" "+day.Format("_2"), dashLabel)
	}

	// Days the ERP expected nothing of say so instead of drawing an empty bar.
	if note := dayNote(d); note != "" {
		return theme.Dim.Render(label) + "  " +
			theme.Off.Render(center(note, barCells))
	}

	// A working day that has not happened yet draws the bare track and no number: the ERP
	// reports 8 expected hours for it, and an empty red 0:00 bar would read as a day the
	// hours were missed on rather than one nobody could have worked.
	if d.Date > today && d.Actual == 0 { // ISO dates sort as strings
		return theme.Dim.Render(label) + "  " +
			theme.Track.Render(strings.Repeat("┈", barCells))
	}

	style := theme.Dim
	if d.Date == today {
		style = theme.TitleFocus // the day being logged into right now
	}
	return style.Render(label) + "  " + dashBar(d.Actual, barCells, d.Date == today)
}

// dashBar is one day's bar, exactly barCells wide however long the bar is: a solid band
// of colour, the hours printed inside it at its right end in dark ink, then the dotted
// track and the 4h / 8h ticks in whatever it did not fill.
//
// The label sits inside the band rather than after it so it costs no width and the bar
// keeps meaning what the axis says. The band carries no pattern — the rule between days
// is what tells one bar from the next.
func dashBar(hours float64, cells int, today bool) string {
	fill, band, fillStyle := cellsFor(hours, cells), hourBand(hours), hourFill(hours)

	label := hoursLabel(hours)
	if today {
		// Say it, rather than leaving a glyph to be decoded: a running total must not
		// look like a finished one.
		label += " today"
	}

	// The unfilled remainder, with a tick where a threshold falls beyond the fill.
	rest := []rune(strings.Repeat("┈", cells-fill))
	for _, h := range []float64{4, 8} {
		// Strictly past the fill: a bar sitting exactly on 8h has reached that
		// threshold, so a tick at its edge would read as part of the bar.
		if at := cellsFor(h, cells) - fill; at > 0 && at < len(rest) {
			rest[at] = '┆'
		}
	}

	// Room for the label inside the band, a space either side of it?
	if inside := len(label) + 2; fill >= inside {
		return fillStyle.Render(strings.Repeat(" ", fill-inside)+" "+label+" ") +
			theme.Track.Render(string(rest))
	}
	// Too short to hold its own number: the number moves into the track, in the band's
	// colour, and eats the dots it covers so the row stays the same width.
	n := min(len(label)+2, len(rest))
	return fillStyle.Render(strings.Repeat(" ", fill)) +
		band.Render(" "+label+" ") + theme.Track.Render(string(rest[n:]))
}

// hourFill is the band a bar is drawn on: same thresholds as hourBand, as a background.
func hourFill(hours float64) lipgloss.Style {
	switch {
	case hours >= 8:
		return theme.HighFill
	case hours >= 4:
		return theme.MidFill
	default:
		return theme.LowFill
	}
}

// hourBand is the colour for a day's hours: under 4h, under 8h, on target. Deliberately
// not read off Expected — a day the ERP expects 4h of is still a short day.
func hourBand(hours float64) lipgloss.Style {
	switch {
	case hours >= 8:
		return theme.High
	case hours >= 4:
		return theme.Mid
	default:
		return theme.Low
	}
}

// cellsFor is how much of a barCells-wide track a number of hours fills.
func cellsFor(hours float64, barCells int) int {
	n := int(math.Round(hours / dashScale * float64(barCells)))
	if n > barCells {
		n = barCells
	}
	if n < 0 {
		n = 0
	}
	return n
}

// monthBar is the month's hours against its target so far, in the same block glyphs as
// the day bars — the number alone made a 14h shortfall look like a rounding error.
func monthBar(logged, target float64) string {
	const w = 20
	filled := w
	if target > 0 {
		filled = int(math.Round(logged / target * float64(w)))
	}
	if filled > w {
		filled = w
	}
	if filled < 0 {
		filled = 0
	}
	style := theme.Mid
	if logged >= target && target > 0 {
		style = theme.High
	}
	return style.Render(strings.Repeat("█", filled)) +
		theme.Track.Render(strings.Repeat("┈", w-filled))
}

// dayNote is why a day has no hours to show, or "" when hours were expected.
func dayNote(d api.DayLog) string {
	switch {
	case d.Holiday:
		return "holiday"
	case d.OnLeave && !d.HalfDay:
		return "leave"
	case d.Weekend:
		return "weekend"
	case d.Expected == 0:
		return "no hours expected"
	}
	return ""
}

// dashAxis is the ruler under the bars, labelled every second hour. A label that would
// run past the end of the bar is left out rather than pushing the line wider.
func dashAxis(barCells int) string {
	marks := []byte(strings.Repeat(" ", barCells+1))
	for h := 0; h <= int(dashScale); h += 2 {
		at := int(math.Round(float64(h) / dashScale * float64(barCells)))
		label := fmt.Sprint(h)
		if at+len(label) > len(marks) {
			continue
		}
		copy(marks[at:], label)
	}
	indent := strings.Repeat(" ", dashLabel+2)
	return indent + theme.Track.Render("└"+strings.Repeat("─", barCells)) + "\n" +
		indent + " " + theme.Dim.Render(strings.TrimRight(string(marks), " "))
}

// hoursLabel turns the ERP's decimal hours into the h:mm the rest of the app uses:
// 8.25 -> "8:15". A tenth of an hour is not a unit anybody logs in.
func hoursLabel(h float64) string {
	return parse.FormatHM(int(math.Round(h * 60)))
}

// center pads s into w cells, for the weekend and holiday blocks.
func center(s string, w int) string {
	if lipgloss.Width(s) >= w {
		return trunc(s, w)
	}
	left := (w - lipgloss.Width(s)) / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", w-left-lipgloss.Width(s))
}

// header is the caret, the boxed query field, and the day's progress cluster.
func (m Model) header() string {
	// The caret marks focus, so it goes away with focus — but keeps its cells, or
	// the whole header would shift.
	caret := "   "
	if m.mode == ModeSearch {
		caret = theme.Prompt.Render(" ❯ ")
	}
	prog := m.progress()

	field := m.search.View()
	if m.mode != ModeSearch && m.search.Value() != "" {
		field = theme.Dim.Render(m.search.Value())
	}
	boxWidth := m.cols() - lipgloss.Width(caret) - lipgloss.Width(prog) - 6
	if boxWidth < 24 {
		boxWidth = 24
	}
	frame := theme.SearchBox
	if m.mode == ModeSearch {
		frame = theme.SearchBoxFocus
	}
	box := frame.Width(boxWidth).Render(field)

	return lipgloss.JoinHorizontal(lipgloss.Center, caret, box, "  ", prog)
}

// searchFieldWidth is the textinput.Width the query field gets: the narrowest the
// box can be — the progress cluster at its widest — less the box's two padding cells
// and the cursor cell the input draws after its Width. Sized against the worst case
// on purpose, so no query can wrap the box onto a second line and shove the list
// down; the box itself still hugs the cluster as actually rendered.
func (m Model) searchFieldWidth() int {
	if w := m.cols() - caretCol - progReserve - 9; w > 16 {
		return w
	}
	return 16
}

// progress is two right-aligned lines: the bar with today's total, and the
// TODAY line with the percentage and how many tasks are in view.
func (m Model) progress() string {
	done, pending := m.todayMinutes()
	pct := float64(done) / DailyGoal
	if pct > 1 {
		pct = 1
	}

	style := theme.Bar
	if done >= DailyGoal {
		style = theme.BarDone
	}
	full := int(pct * barWidth)
	bar := style.Render(strings.Repeat("█", full)) +
		theme.BarRest.Render(strings.Repeat("░", barWidth-full))

	top := bar + "  " + theme.Total.Render(parse.FormatTotal(done)) + theme.Dim.Render(" / 8h")
	if m.syncing {
		top += theme.Dim.Render(" ⟳")
	}
	line := fmt.Sprintf("TODAY %.0f%% · %d/%d tasks", pct*100, len(m.filtered()), len(m.tasks))
	if pending > 0 {
		line = fmt.Sprintf("TODAY %.0f%% · +%s unsynced · %d/%d tasks",
			pct*100, parse.FormatTotal(pending), len(m.filtered()), len(m.tasks))
	}

	return lipgloss.JoinVertical(lipgloss.Right, top, theme.Dim.Render(line))
}

// todayMinutes is what the bar measures: the ERP's own total for today once a sync
// has answered, plus the entries typed here that the ERP has never seen. Before a
// sync answers, the local rows are all there is to go on.
func (m Model) todayMinutes() (total, pending int) {
	today := parse.Today()
	pending = store.PendingMinutesOn(m.tasks, today)
	if m.erpToday >= 0 {
		return m.erpToday + pending, pending
	}
	return store.MinutesOn(m.tasks, today), pending
}

// listLines renders the list one line per slice entry, and reports which line
// holds the cursor so the window can keep it on screen.
func (m Model) listLines() ([]string, int) {
	tasks := m.filtered()
	if len(tasks) == 0 {
		// An empty list mid-sync is not an answer yet, so it says it is still reading
		// rather than "no tasks match", which reads as the ERP having none.
		if m.busy() && m.search.Value() == "" {
			return []string{theme.Blur.Render(
				m.spin.View() + theme.Dim.Render(" reading your tasks…"))}, -1
		}
		return []string{theme.Blur.Render(theme.Dim.Render("no tasks match"))}, -1
	}

	var lines []string
	focus := -1
	add := func(s string) { lines = append(lines, s) }
	mark := func() { focus = len(lines) - 1 }

	// A modal opened from the list keeps the task highlighted behind it.
	listFocus := m.mode == ModeList || m.mode == ModeSearch ||
		(m.mode == ModeConfirm && m.prev == ModeList) ||
		// The day modal is opened from the list and covers nothing of it, so the task
		// the cursor was on keeps saying so behind the modal.
		(m.mode == ModeDay && !m.jumpInTask)

	for i, t := range tasks {
		onTask := i == m.cursor
		focused := onTask && listFocus
		// The title stays accent-colored for the task the cursor is in, even once focus
		// has moved down into its rows — otherwise expanding a task drops the only
		// mark of which one you are inside. The border and background still track the
		// focused line.
		add(row(m.taskLine(t, onTask), focused))
		if focused {
			mark()
		}

		if !m.expanded[t.ID] {
			add("") // a collapsed list breathes, as in the design
			continue
		}

		add(theme.Blur.Render(m.tableHead()))

		editing := m.mode == ModeInsert && onTask
		// A new entry is typed at the top of the table, where it will land.
		if editing && m.kind == insertNew {
			add(row(m.insertLine(), true))
			mark()
		}

		inTable := onTask && !listFocus &&
			(m.mode == ModeTable || m.mode == ModeJump || m.mode == ModeConfirm ||
				m.mode == ModeDay)
		for j, e := range t.Rows {
			// An edit happens in place: the inputs stand where the row was.
			if editing && m.kind == insertEdit && j == m.editRow {
				add(row(m.insertLine(), true))
				mark()
				continue
			}
			add(row(m.entryLine(e), inTable && j == m.row))
			if inTable && j == m.row {
				mark()
			}
		}
		if len(t.Rows) == 0 && !editing {
			add(theme.Blur.Render(theme.Dim.Render("    no entries yet — a to add one")))
		}
		add("")
	}
	return lines, focus
}

// row shows focus with a left border plus a dim background — never color alone.
func row(s string, focused bool) string {
	if focused {
		return theme.Focus.Render(s)
	}
	return theme.Blur.Render(s)
}

// taskLine is caret · title · tag chip ......... entry count · total.
func (m Model) taskLine(t store.Task, focused bool) string {
	caret := "▸"
	if m.expanded[t.ID] {
		caret = "▾"
	}

	title := oneLine(t.Title)
	if t.Key != "" {
		title = t.Key + " " + title
	}

	count := fmt.Sprintf("%d entries", len(t.Rows))
	if len(t.Rows) == 1 {
		count = "1 entry"
	}
	right := theme.Dim.Render(count) + "   " + theme.Total.Render(fmt.Sprintf("%7s",
		parse.FormatTotal(store.Total(t.Rows))))

	// What the caret, the right cluster and spread's two-cell gap leave over is split
	// between the two variable parts, measured rather than guessed. The chip is cut
	// first and to no more than a third: tags are Odoo project names, and
	// "VALUE-DRIVEN ENGAGEMENT, INTERNAL MEETINGS & TASKS" is 50 cells on its own,
	// which used to push the line clean off an 80-cell screen. The title takes the
	// rest — it identifies the task, so it loses cells last.
	room := m.cols() - gutter - lipgloss.Width(caret) - 2 - lipgloss.Width(right) - 2

	chip := ""
	if tag := oneLine(t.Tag); tag != "" {
		const chrome = 6 // the two spaces in front, two borders, two padding cells
		// A third of the screen, not of what this row has left, so the same tag is cut
		// to the same width on every line. Clamped to the row's own space as a floor,
		// which only bites when the entry count and total are unusually wide.
		w := m.cols()/3 - chrome
		if w > room-4 {
			w = room - 4
		}
		if w > 3 {
			chip = "  " + theme.Chip.Render(strings.ToUpper(trunc(tag, w)))
			room -= lipgloss.Width(chip)
		}
	}

	style := theme.Title
	if focused {
		style = theme.TitleFocus
	}
	left := theme.Dim.Render(caret) + "  " + style.Render(trunc(title, room)) + chip
	return spread(left, right, m.cols()-gutter)
}

func (m Model) tableHead() string {
	return theme.Header.Render(cells(
		pad("DATE", dateWidth),
		pad("DESCRIPTION", m.descWidth()),
		// Left-aligned, like the hours input on the insert row: a textinput fills its
		// column and draws its cursor after the text, so it cannot be right-aligned.
		// Right-aligning only the head and the rows left h:mm two cells adrift.
		pad("HOURS", hoursWidth)))
}

func (m Model) entryLine(e store.Entry) string {
	desc := m.descWidth()
	date, text := theme.Dim, lipgloss.NewStyle()
	if m.onJumpDate(e) {
		// The row a jump was looking for, in any task: the date and what was done on
		// it both carry the accent, so a marked row reads at a glance while scrolling.
		date, text = theme.Match, theme.MatchText
	}
	// Pad first, style second: fmt counts the bytes of an ANSI escape as width.
	return cells(
		date.Render(pad(e.Date, dateWidth)),
		text.Render(pad(trunc(oneLine(e.Desc), desc), desc)),
		theme.Total.Render(pad(parse.FormatHM(e.Minutes), hoursWidth)))
}

// cells lays out one table row: a shared indent, then columns two cells apart.
func cells(date, desc, hours string) string {
	return tableIndent + date + colGap + desc + colGap + hours
}

// fieldWidth is the textinput.Width that fits a column: the input renders its
// width plus one cell for the cursor.
func fieldWidth(col int) int {
	if col < 2 {
		return 1
	}
	return col - 1
}

// pad measures with lipgloss, so styled text lands in the right column.
func pad(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

// descWidth caps the description column: stretching it to the right edge leaves
// the hours marooned a screen away from the text they belong to.
func (m Model) descWidth() int {
	// insertWidth is reserved on every row, not just the insert one, so the columns
	// hold still when the ✓ / ✕ buttons appear.
	const insertWidth = 10
	w := m.cols() - gutter - len(tableIndent) - dateWidth - hoursWidth - 2*len(colGap) - insertWidth
	if w > descCap {
		w = descCap
	}
	if w < 20 {
		w = 20
	}
	return w
}

func (m Model) insertLine() string {
	date := m.fields[fieldDate].View()
	if m.datePristine && m.focus == fieldDate {
		// "selected": the whole value is highlighted until the first keystroke.
		date = lipgloss.NewStyle().Foreground(theme.Chrome).Background(theme.Accent).
			Render(m.fields[fieldDate].Value())
	}

	ok, cancel := " ✓ ", " ✕ "
	okStyle := lipgloss.NewStyle().Foreground(theme.Complete)
	cancelStyle := lipgloss.NewStyle().Foreground(theme.Destructive)
	if m.focus == fieldAccept {
		okStyle = okStyle.Reverse(true)
	}
	if m.focus == fieldReject {
		cancelStyle = cancelStyle.Reverse(true)
	}

	return cells(
		pad(date, dateWidth),
		pad(m.fields[fieldDesc].View(), m.descWidth()),
		pad(m.fields[fieldHours].View(), hoursWidth)) +
		"  " + okStyle.Render(ok) + " " + cancelStyle.Render(cancel)
}

// authModal asks for the key. The input echoes dots, and the only thing shown
// about a stored key is its last four characters.
func (m Model) authModal() string {
	lines := []string{
		theme.Title.Render("ERP API key") + theme.Dim.Render("  "+api.BaseURL()),
		m.auth.View(),
		theme.Dim.Render("in memory " + store.MaskKey(m.key) + " · saved to pass: " + store.PassName()),
		theme.Dim.Render("or export " + store.KeyEnv + " to skip pass"),
	}
	return theme.Modal.Render(strings.Join(lines, "\n"))
}

// dayModal lists what a date jump found: one line per entry, with the task it belongs
// to and its hours, then the day's total. It answers "where did the day go" without
// opening a single task, which is the whole point of asking.
func (m Model) dayModal() string {
	rows, total := m.dayRows()

	// Nothing wider than the screen, and the description gives up cells first.
	keyW, hoursW := 0, hoursWidth
	for _, r := range rows {
		if w := lipgloss.Width(r.key); w > keyW {
			keyW = w
		}
	}
	descW := m.cols() - gutter - 8 - keyW - hoursW // 8: borders, padding, two gaps
	if descW > descCap {
		descW = descCap
	}

	head := theme.Title.Render(m.jumpDate) + theme.Dim.Render(
		fmt.Sprintf("   %s in %d %s", parse.FormatTotal(total), len(rows), plural(len(rows), "entry", "entries")))
	lines := []string{head}
	if len(rows) == 0 {
		lines = append(lines, theme.Dim.Render("nothing logged on this date"))
	}

	// A day with more entries than this is not a day worth reading in a modal; the
	// count in the head still tells the truth about what was left out.
	const most = 12
	for i, r := range rows {
		if i == most {
			lines = append(lines, theme.Dim.Render(fmt.Sprintf("… %d more", len(rows)-most)))
			break
		}
		desc := trunc(oneLine(r.desc), descW)
		if desc == "" {
			desc = "—"
		}
		hours := parse.FormatHM(r.mins)
		if r.local {
			hours += "*" // typed here, not in the ERP yet
		}
		lines = append(lines, theme.Dim.Render(pad(r.key, keyW))+"  "+
			pad(desc, descW)+"  "+theme.Total.Render(padLeftCell(hours, hoursW+1)))
	}
	return theme.Modal.Render(strings.Join(lines, "\n"))
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// padLeftCell right-aligns in w cells, measuring with lipgloss like pad does. Hours
// are the one column in the modal that reads better against its right edge, since
// nothing follows them.
func padLeftCell(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return strings.Repeat(" ", d) + s
	}
	return s
}

// footer is the mode indicator and, when the key list is open, the keys the mode takes.
// Closed it advertises one key — ? — which is what buys the list back; the screen keeps
// the line either way, so opening it does not shove the body around.
func (m Model) footer() string {
	label := modeLabel(m.mode)
	if m.tab == TabDash && m.mode != ModeConfirm && m.mode != ModeAuth {
		label = "-- DASHBOARD --"
	}

	// A modal is the exception: whatever accepts it has to be on screen, since it is
	// holding the keyboard and ? will not reach the toggle.
	help := []key.Binding{keys.Help}
	switch {
	case m.mode == ModeConfirm:
		// Which key accepts depends on what is being confirmed.
		help = []key.Binding{m.confirmKeys(), keys.No}
	case !m.showHelp:
	case m.tab == TabDash:
		// It moves in screenfuls, not days, and there is no i: the query field filters
		// tasks and this is not that tab.
		help = []key.Binding{keys.Top, keys.HalfDown, keys.TasksTab, keys.Refresh,
			keys.Quit, keys.Help}
	default:
		help = append(keys.help(m.mode), keys.Help)
	}

	parts := []string{theme.Mode.Render(label)}
	for _, b := range help {
		h := b.Help()
		parts = append(parts, theme.Mode.Render(h.Key)+theme.Hint.Render(" "+h.Desc))
	}
	// Width wraps rather than truncates: help that hides keys drifts from behavior.
	return theme.Blur.Width(m.cols()).Render(strings.Join(parts, theme.Hint.Render("  ·  ")))
}

// spread pushes right to the far edge of width, keeping at least two cells of gap.
func spread(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 2 {
		gap = 2
	}
	return left + strings.Repeat(" ", gap) + right
}

// oneLine flattens text that came from the ERP. An Odoo task name or timesheet
// description can hold newlines and tabs — VD-427's name does — and one newline in a
// task line renders it as two, so every row below slides down and the header scrolls
// off the top of the screen. Runs of whitespace collapse to a single space.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func trunc(s string, n int) string {
	if lipgloss.Width(s) <= n {
		return s
	}
	// Cramped is still better than overflowing: a line wider than the terminal wraps,
	// and the whole layout below it slides.
	switch {
	case n <= 0:
		return ""
	case n == 1:
		return "…"
	}
	return string([]rune(s)[:n-1]) + "…"
}
