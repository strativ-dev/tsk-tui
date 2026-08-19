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
	// The band's ends. With the underline the *Fill styles carry along the bottom, they are
	// what separate a bar from the day stacked directly under it: a bare wash of colour, row
	// after row, merged into one rectangle, and the month is laid out in columns precisely so
	// no row can be spent on a rule between days. Nothing is drawn along the top — a rule there
	// crowded the band and read as a second bar.
	barEdgeL = "▏"
	barEdgeR = "▕"
	// minBar is the narrowest a bar may get before the chart stops adding columns: below
	// this the hours no longer fit beside it and the bar stops being readable.
	minBar = 14
	// maxDashCols caps the columns even on a very wide terminal: four columns of eight days
	// is the whole month, and more would only shrink the bars.
	maxDashCols = 4
	// dashChrome is the rows the chart's own furniture costs — tab bar, rule, the clock box
	// and the month's totals above, the axis, status and footer below. The grid sizes itself
	// against what is left.
	dashChrome = 16
	descCap    = 48

	// The year calendar, measured off the design's own month cell — 304px at 38 columns. A
	// day is a four-cell badge, " 21 ", in a five-cell column: the fifth is the gap the design
	// leaves between its days, and without it a run of leave days reads as one long bar rather
	// than as days. So a week is 35 cells and a month 39 with the week-number column and the
	// cell's own left padding in front of it — three months and the holiday panel want about
	// 148 cells, three months alone 121.
	dayCell   = 5
	badgeCell = 4
	weekCol   = 3
	monthPad  = 1
	monthCols = monthPad + weekCol + 7*dayCell
	// A week row always has a line under it: that air is what makes the month read as a
	// calendar rather than as a table, and it is the design's own proportion — a day is
	// nearly as tall as it is wide there. A roomy terminal also gets the padding the design
	// puts above the month's name and under its weekday heads; a short one spends those two
	// rows on days.
	roomyRows = 34
	// A month is its name and its weekday heads above however many week rows it spans —
	// four to six — and the months in one row are padded to the tallest of them and no
	// further: six rows everywhere would cost the year lines it does not have to spend.
	//
	// maxTimeCols caps the months per row: more than three and the calendar reads as a
	// wall rather than as a year, and the holiday panel loses its room.
	maxTimeCols = 3
	// The holidays get whatever is left beside the months, between these: below panelMin the
	// names stop being readable, and above panelMax the list is just wide.
	panelMin  = 24
	panelMax  = 34
	spanCells = 12 // "Mar 30-Apr 2", the longest a span gets
)

// View stacks a fixed header, a windowed list, and a fixed footer. The header and
// footer are laid out first and the list gets whatever rows are left, so the
// search field can never be pushed off the top of the screen.
func (m Model) View() string {
	top := m.tabBar()
	if m.tab == TabTime {
		// The year and what has been taken out of it ride here: the tab bar's row is half
		// empty, and a title line of its own would cost the calendar a row.
		top = spread(top, m.timeSummary(m.cols()-lipgloss.Width(top)-2)+" ", m.cols())
	}
	head := []string{top}
	if m.tab == TabTasks {
		// The query field filters tasks, so it belongs to that tab and nowhere else.
		head = append(head, strings.Split(m.header(), "\n")...)
	}
	head = append(head, theme.Sep.Render(strings.Repeat("─", m.cols())))
	if m.tab != TabTime {
		// On the calendar that rule is the top edge of the balance boxes, so nothing sits
		// between them.
		head = append(head, "")
	}

	var tail []string
	switch m.mode {
	case ModeConfirm:
		// Read off the bindings themselves. Spelling the keys out here meant a rebind
		// could leave a destructive prompt advertising a key that no longer accepts it.
		hint := m.confirmKeys().Help().Key + " / " + keys.No.Help().Key
		// A prompt of several lines puts its keys on a line of their own: appended to the
		// last one they read as part of it, and "Coast trip  y / n" is not a description.
		prompt := m.cPrompt + "  " + theme.Dim.Render(hint)
		if strings.Contains(m.cPrompt, "\n") {
			prompt = m.cPrompt + "\n\n" + theme.Dim.Render(hint)
		}
		tail = append(tail, strings.Split(theme.Modal.Render(prompt), "\n")...)
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
	var dFoot []string
	if m.tab == TabDash {
		dHead := m.dashHead()
		dFoot = m.dashFoot()
		for _, frame := range []*[]string{&dFoot, &dHead} {
			if m.rows()-len(head)-len(tail)-len(dHead)-len(dFoot) >= minDayRows {
				break
			}
			*frame = nil
		}
		head = append(head, dHead...)
		tail = append(dFoot, tail...)
	}
	if m.tab == TabTime {
		head = append(head, m.timeHead()...)
	}

	// The body takes the rows left between header and footer, and is padded out to
	// them, which pins the status line and the key hints to the bottom of the screen.
	budget := m.rows() - len(head) - len(tail)
	body, focus := m.listLines()
	if m.tab == TabTime {
		body, focus = m.timeLines()
	}
	if m.tab == TabDash {
		body, focus = m.dashLines(budget)
		// A month that fits needs no pinned axis: the ruler goes back under the last day,
		// where it is read, rather than at the bottom of the screen with the padding
		// between. Pinning is only worth it once the days scroll under it.
		if len(dFoot) > 0 && len(body) <= budget {
			tail = tail[len(dFoot):]
			budget += len(dFoot)
			body = append(body, dFoot...)
		}
	}
	body = window(body, focus, budget)
	for len(body) < budget {
		body = append(body, "")
	}
	if m.tab == TabTime {
		// After the window, not before it: the holiday list is a pinned column, so the
		// months scroll under it rather than taking it with them.
		body = m.withHolidayPanel(body)
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
		{TabTime, "timeoff", keys.TimeTab},
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

// dashHead is the chart's frame above the days: the ERP's clock in the top-right corner,
// then the month, its totals against the target, and what a tick inside a bar means. It is
// laid out with the header rather than with the body, so the days scroll under numbers that
// stay put.
//
// The clock does not wait for a month. Its two lines are built first and unconditionally —
// line two is the spacer that was already there, so a loaded month still costs six lines —
// and the totals join only once there are days to total.
func (m Model) dashHead() []string {
	month := time.Now()
	if t, err := time.Parse("2006-01-02", m.dashMonth); err == nil {
		month = t
	}
	title := ""
	if len(m.dashDays) > 0 || m.dashMonth != "" {
		// No "month to date" note: the target beside `logged` is the whole month, and the
		// last row is today, marked as today, which says where the days stop.
		title = theme.HintKey.Render("< ") + theme.Title.Render(strings.ToUpper(month.Format("January 2006")))
		if m.dashOffset < 0 {
			// > only past the current month: the ERP has nothing to report on one that
			// has not happened, so a key that does nothing does not appear.
			title += theme.HintKey.Render(" >")
		}
		if m.dashLoading {
			// The month on screen is still the last one answered — r or a month step
			// left it up rather than blanking it — so the loader sits beside its title
			// instead of replacing it. The empty-chart case has its own spinner, in
			// dashLines, since there is no title yet to sit beside.
			title += " " + m.spin.View()
		}
	}

	// The button is a box, three lines of it, so the one thing on this screen you can press
	// looks like it. Each line goes in on its own: View budgets rows by counting elements.
	out := []string{m.clockLine(title, m.clockStatus())}
	for _, l := range strings.Split(m.clockButton(), "\n") {
		out = append(out, m.clockLine("", l))
	}
	if len(m.dashDays) == 0 {
		return out
	}

	logged, target, worked, workdays := m.dashTotals()
	return append(out,
		theme.Blur.Render(theme.Dim.Render("logged  ")+monthBar(logged, target)+
			"  "+theme.Total.Render(hoursLabel(logged))+
			theme.Dim.Render(" / "+hoursLabel(target))+"   "+m.todayLogged()+
			theme.Dim.Render(fmt.Sprintf("   %d of %d days", worked, workdays))),
		"",
		theme.Blur.Render(theme.Header.Render("HOURS PER DAY")+
			theme.Dim.Render("   ")+theme.Track.Render("┆")+theme.Dim.Render(" 4h · 8h")),
		"",
	)
}

// clockHelp is the footer's entry for the clock key, named after what pressing it will do
// rather than after the thing it belongs to: "c clock" left you to guess which way it would
// go. It takes its key off the binding, so a rebind still follows.
func (m Model) clockHelp() key.Binding {
	what := "check in"
	switch {
	case m.clocking && m.att.CheckedIn:
		what = "checking out…"
	case m.clocking:
		what = "checking in…"
	case m.attKnown && m.att.CheckedIn:
		what = "check out"
	}
	return key.NewBinding(key.WithKeys(keys.Clock.Keys()...),
		key.WithHelp(keys.Clock.Help().Key, what))
}

// clockLine puts the clock cluster at the right edge. spread does not truncate — it clamps
// the gap and lets the line overflow — so the cluster is cut to what the left side leaves,
// the way a task line measures its own right cluster first.
func (m Model) clockLine(left, right string) string {
	room := m.cols() - gutter - lipgloss.Width(left) - 2
	return theme.Blur.Render(spread(left, trunc(right, room), m.cols()-gutter))
}

// clockStatus is the line above the button: where you are working, when the session
// started, and how long it has run. Nothing before the first answer lands — an invented
// clock is worse than no clock.
func (m Model) clockStatus() string {
	if !m.attKnown {
		return ""
	}
	if !m.att.CheckedIn {
		return theme.Dim.Render("checked out")
	}

	where := ""
	switch m.todayLocation() {
	case "home":
		where = "WFH  "
	case "office":
		where = "OFFICE  "
	}
	// Elapsed is derived here, never stored: Odoo leaves worked_hours at 0 until the
	// session closes, so the running figure can only be now minus the check in.
	elapsed := parse.FormatHM(int(time.Since(m.att.Since).Minutes()))
	return theme.Dim.Render(where) + theme.Total.Render(clockTime(m.att.Since)) +
		theme.Dim.Render(" ("+elapsed+")")
}

// clockButton is the ERP's own check in / check out control, boxed: three lines, and the
// only thing on the chart you can press. The border carries the state — green invites an
// action not yet taken, amber says a clock is running and wants closing, and a plain rule says
// we have not read the state yet, since a green button that swallowed the key would be a lie.
// The words themselves are white and the c is the accent, so the one colour that means "this
// is the key" is not competing with the state's colour for the same cells.
func (m Model) clockButton() string {
	box, label := theme.ClockIn, hinted("check in", keys.Clock, theme.ClockText, theme.HintKey)
	switch {
	case m.clocking:
		// The loader takes the label's place, so the box does not move while the ERP thinks
		// about it, and the border still says which way it is going.
		what := "checking in…"
		if m.att.CheckedIn {
			box, what = theme.ClockOut, "checking out…"
		}
		label = m.spin.View() + " " + theme.ClockText.Render(what)
	case !m.attKnown:
		box, label = theme.ClockOff, theme.Dim.Render("check in")
	case m.att.CheckedIn:
		box = theme.ClockOut
		label = hinted("check out", keys.Clock, theme.ClockText, theme.HintKey)
	}
	return box.Render(label)
}

// todayLocation is what the ERP said about today in the month it already sent: "home",
// "office" or "". hr.employee's own work_location_id is a place ("Bangladesh"), which
// answers a different question.
func (m Model) todayLocation() string {
	today := time.Now().Format("2006-01-02")
	for _, d := range m.dashDays {
		if d.Date == today {
			return string(d.WorkLocation)
		}
	}
	return ""
}

// clockTime renders a wall-clock time the way a person reads one. Odoo keeps UTC; this is
// the only place that turns it into the terminal's own zone.
func clockTime(t time.Time) string { return t.Local().Format("3:04 PM") }

// checkedLabel names a state for a sentence.
func checkedLabel(in bool) string {
	if in {
		return "checked in"
	}
	return "checked out"
}

// todayLogged is the hours on today, beside the month's own totals — the one figure on that
// line you can still do something about.
//
// It is what has been logged, not the gap against the eight hours due: a day that has barely
// started owes nothing yet, and opening the chart to a red "−8:00" read as hours already
// missed rather than a day not yet worked. The colour carries the shortfall instead, on the
// chart's own thresholds, and today's bar shows the same fact against the 8h tick.
func (m Model) todayLogged() string {
	today := time.Now().Format("2006-01-02")
	for _, d := range m.dashDays {
		if d.Date != today {
			continue
		}
		if d.Expected == 0 {
			// Weekend, holiday, leave: nothing was expected, so nothing is owed.
			return theme.Dim.Render("today off")
		}
		return theme.Dim.Render("today ") + hourBand(d.Actual).Render(hoursLabel(d.Actual))
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
	// One ruler under every column: the columns share a scale, and a bar with no ruler
	// beneath it is measured against nothing.
	cols, _ := m.dashGrid()
	// No blank line above it: the ruler and its numbers belong to the bars, and a gap read
	// as the chart ending before the axis did.
	var out []string
	for _, l := range strings.Split(dashAxis(m.barCells()), "\n") {
		row := make([]string, cols)
		for i := range row {
			row[i] = l
		}
		out = append(out, theme.Blur.Render(strings.Join(row, colGap)))
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

	// The month is laid out in as many columns as it takes to fit the rows on offer, days
	// running down each column and on into the next, so every day keeps its own label and
	// its own printed hours. One column is the roomy case and keeps the rule between days.
	today, hold, focus := time.Now().Format("2006-01-02"), m.dashDayIndex(), -1
	cols, rows := m.dashGrid()
	barCells := m.barCells()
	ruled := cols == 1 && (2*len(m.dashDays)-1 <= budget || len(m.dashDays) > budget)
	rule := theme.Blur.Render(theme.Track.Render(
		strings.Repeat("─", dashLabel+2+barCells)))

	lines := make([]string, 0, rows*2)
	for r := range rows {
		if r > 0 && ruled {
			lines = append(lines, rule)
		}
		cells := make([]string, 0, cols)
		for c := range cols {
			i := c*rows + r
			if i >= len(m.dashDays) {
				// A short last column is padded, or the columns beside it would shift left
				// on the rows it does not reach.
				cells = append(cells, strings.Repeat(" ", dashLabel+2+barCells))
				continue
			}
			cells = append(cells, m.dashRow(m.dashDays[i], barCells, today))
			if i == hold {
				focus = len(lines) // the row the window is built around
			}
		}
		lines = append(lines, theme.Blur.Render(strings.Join(cells, colGap)))
	}
	return lines, focus
}

// dashGrid is how the month is arranged: how many columns of days, and how many rows in
// each. Enough columns to fit the rows on offer, capped by what the width can hold, so all
// 31 days are one screen on a terminal that could never show them stacked.
//
// It reads its own estimate of the rows rather than the budget View measured, because the
// bars and the axis are sized from this too: one estimate everywhere keeps them agreeing,
// and being a row out only changes how early a column is added.
func (m Model) dashGrid() (cols, rows int) {
	days := len(m.dashDays)
	if days == 0 {
		return 1, 0
	}

	// The widest the terminal can hold, and the fewest columns the rows need.
	wide := max((m.cols()-gutter+len(colGap))/(dashLabel+2+minBar+len(colGap)), 1)
	budget := max(m.rows()-dashChrome, 1)
	cols = min(max((days+budget-1)/budget, 1), min(wide, maxDashCols))
	return cols, (days + cols - 1) / cols
}

// barCells is the width of one day's bar: what the label, the hours and the other columns
// leave over, capped so a wide terminal does not stretch one day across the screen. The
// bars and the axis under them read it, so they cannot disagree.
func (m Model) barCells() int {
	cols, _ := m.dashGrid()
	room := m.cols() - gutter - cols*(dashLabel+2) - (cols-1)*len(colGap)
	n := room / cols
	if cols == 1 {
		n -= 6 // one column leaves the right edge alone rather than filling it
	}
	if n > dashBars {
		n = dashBars
	}
	if n < minBar {
		n = minBar
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

	// Days the ERP expected nothing of say so instead of drawing an empty bar, and their
	// dates stay dim: a weekend is not a day you can act on.
	if note := dayNote(d); note != "" {
		return theme.Dim.Render(label) + "  " +
			theme.Off.Render(center(note, barCells))
	}

	// A working day that has not happened yet draws the bare track and no number: the ERP
	// reports 8 expected hours for it, and an empty red 0:00 bar would read as a day the
	// hours were missed on rather than one nobody could have worked.
	if d.Date > today && d.Actual == 0 { // ISO dates sort as strings
		return theme.DayLabel.Render(label) + "  " +
			theme.Track.Render(strings.Repeat("┈", barCells))
	}

	// White for a working day, so the weekends beside it read as the quiet ones; the accent
	// for today, which is the day being logged into right now.
	style := theme.DayLabel
	if d.Date == today {
		style = theme.TitleFocus
	}
	return style.Render(label) + "  " + dashBar(d.Actual, barCells, d.Date == today)
}

// dashBar is one day's bar, exactly barCells wide however long the bar is: a dark wash of
// the day's threshold colour, outlined and lettered in the light one, then the dotted track
// and the 4h / 8h ticks in whatever it did not fill.
//
// The ends and the style's own underline are what tell one bar from the day stacked on top of
// it, which matters now that the month is laid out in columns and the rows sit directly against
// each other. Nothing is drawn along the top: a rule there crowded the band.
//
// The hours print inside the band at its right end, so they cost the bar no width and the bar
// keeps meaning what the axis says.
func dashBar(hours float64, cells int, today bool) string {
	fill, band, bar := cellsFor(hours, cells), hourBand(hours), hourFill(hours)

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
	track := theme.Track.Render(string(rest))

	// Room inside the band for both edges, the number, and a space either side of it?
	if inside := len(label) + 4; fill >= inside {
		return bar.Render(barEdgeL+strings.Repeat(" ", fill-inside)+" "+label+" "+barEdgeR) + track
	}
	// Too short to hold its own number: the band keeps its outline and the number moves into
	// the track, in the bar's colour, eating the dots it covers so the row stays the same
	// width.
	body := ""
	if fill >= 2 {
		body = bar.Render(barEdgeL + strings.Repeat(" ", fill-2) + barEdgeR)
	} else if fill == 1 {
		body = bar.Render(barEdgeL)
	}
	n := min(len(label)+2, len(rest))
	return body + band.Render(" "+label+" ") + theme.Track.Render(string(rest[n:]))
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
	// Exactly as wide as a day's row: the corner sits on the bar's first cell and the rule
	// ends on its last. A cell over ran the columns past the right edge.
	marks := []byte(strings.Repeat(" ", barCells))
	for h := 0; h <= int(dashScale); h += 2 {
		at := int(math.Round(float64(h) / dashScale * float64(barCells)))
		label := fmt.Sprint(h)
		if at+len(label) > len(marks) {
			continue
		}
		copy(marks[at:], label)
	}
	indent := strings.Repeat(" ", dashLabel+2)
	return indent + theme.Track.Render("└"+strings.Repeat("─", barCells-1)) + "\n" +
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
// --- time off ----------------------------------------------------------------

// timeYearOf is the year on screen: the one the ERP answered for, or this one before any
// answer has landed, so the calendar draws itself either way.
func (m Model) timeYearOf() int {
	if m.timeYear != 0 {
		return m.timeYear
	}
	return time.Now().Year()
}

// timeLayout splits the width between the months and the holidays: **months first**, up to
// three, and the panel takes what is left when that is enough to read a holiday on.
//
// The months are what this screen is, so three of them outrank the list — but a panel that
// leaves only one month beside it is a list with a calendar attached, so it is dropped before
// the second month is. panel is 0 when there is no column for it.
func (m Model) timeLayout() (cols, panel int) {
	room := m.cols() - gutter
	// One hairline between each pair of months, and no gap: they are cells of one grid.
	fit := min(max((room+1)/(monthCols+1), 1), maxTimeCols)
	if len(m.timeHolidays) == 0 {
		return fit, 0
	}
	for c := fit; c >= 2; c-- {
		if left := room - (c*monthCols + c - 1) - len(colGap); left >= panelMin {
			return c, min(left, panelMax)
		}
	}
	return fit, 0
}

// timeCols is how many months go side by side. ctrl+f / ctrl+b move by one row of them, so
// the handler reads it too.
func (m Model) timeCols() int {
	cols, _ := m.timeLayout()
	return cols
}

// panelCells is the holiday column's width, 0 when it has none.
func (m Model) panelCells() int {
	_, panel := m.timeLayout()
	return panel
}

// timeHead is the frame above the calendar: the year, what has been taken out of it, and
// one chip per leave type with its balance and the key that filters by it. Laid out with
// the header, so the months scroll under figures that stay put.
func (m Model) timeHead() []string {
	title := theme.Title.Render(fmt.Sprintf("TIME OFF %d", m.timeYearOf()))
	if m.timeLoading {
		// The year on screen is still the last one answered — loadTime left it up rather
		// than blanking it — so the loader sits beside its title instead of replacing it.
		title += " " + m.spin.View()
	}

	right := ""
	if m.timeYear != 0 {
		what := "days taken"
		if k, ok := m.timeKind(m.timeFilter); ok {
			what = strings.ToLower(firstWord(k.Name)) + " days taken"
		}
		right = theme.Dim.Render(days(m.timeTaken()) + " " + what)
		if m.timePending() {
			// The underline needs saying once, and only on a year that has one.
			right += theme.Dim.Render("  ·  pending underlined")
		}
	}

	out := m.balanceCards()
	// The line the request is typed on, ruled off from the calendar under it. It is here
	// whether or not it is open — closed it is a label and nothing else — so pressing n
	// cannot move a single row of what is below.
	out = append(out, theme.Blur.Render(theme.Sep.Render(strings.Repeat("─", m.cols()-gutter))))
	out = append(out, m.leaveBand()...)
	return append(out,
		theme.Blur.Render(theme.Sep.Render(strings.Repeat("─", m.cols()-gutter))),
		"")
}

// timeSummary is the year and what has been taken out of it, which rides on the tab bar's
// own row rather than costing the calendar one of its own.
//
// room is what the tab bar leaves. It gives up its parts from the least useful in: the note
// about the underline, then the count, then the year itself — the tab bar is what that row
// is for, and a summary that overflows it wraps the whole screen a line.
func (m Model) timeSummary(room int) string {
	year := theme.Title.Render(fmt.Sprintf("TIME OFF %d", m.timeYearOf()))
	if m.timeLoading {
		// The year on screen is still the last one answered — loadTime left it up rather
		// than blanking it — so the loader sits beside its title instead of replacing it.
		year += " " + m.spin.View()
	}
	if m.timeYear == 0 {
		return fits(room, year, "")
	}

	what := "days taken"
	if k, ok := m.timeKind(m.timeFilter); ok {
		what = strings.ToLower(firstWord(k.Name)) + " days taken"
	}
	count := theme.Dim.Render("   " + days(m.timeTaken()) + " " + what)
	note := ""
	if m.timePending() {
		// The underline needs saying once, and only on a year that has one.
		note = theme.Dim.Render("  ·  pending underlined")
	}
	return fits(room, year, count, note)
}

// fits is the longest prefix of parts that stays inside room, so a cluster gives up its
// tail rather than overflowing its row.
func fits(room int, parts ...string) string {
	out := ""
	for _, p := range parts {
		if lipgloss.Width(out+p) > room {
			return out
		}
		out += p
	}
	return out
}

// leaveLine is the new-timeoff row: a label with its key picked out, and — once n has been
// pressed — the whole request on the same line. Everything is a one-row field, so opening
// the form adds no row and the calendar under it does not move.
//
// Tab order is left to right, which is the only order the line can be read in: type,
// duration, the dates, what it is for, then ✓ and ✕.
// leaveBand is the new-timeoff row: three lines, because each field is a box and a box is
// three lines. Closed it is the label on the middle line and two blanks, so the band is the
// same height either way and revealing the fields cannot move the calendar.
func (m Model) leaveBand() []string {
	if !m.form.open {
		return []string{"",
			theme.Blur.Render(hinted("new timeoff", keys.NewLeave, theme.Dim, theme.HintKey)),
			""}
	}
	label, compact := m.leaveTier()
	lines := strings.Split(m.leaveRow(label, compact, m.form.desc.View()), "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, theme.Blur.Render(l))
	}
	return out
}

// leaveRow draws the line with the description given, so the same code both renders it and
// measures what is left for that description — the two cannot disagree about a cell.
//
// compact is the narrow terminal's version: no spaces between the frames, and the duration
// and the period said in as few letters as they can be. Everything is still there and still
// in the same order.
func (m Model) leaveRow(label, compact bool, desc string) string {
	kind := "—"
	if k, ok := m.leaveKind(); ok {
		kind = firstWord(oneLine(k.Name))
	}
	dur, period, sep := "full day", m.leavePeriodName(), " "
	// The mark between the dates says which two things they are: a range runs from one to
	// the other, a half day has only the one and the half of it.
	arrow := " → "
	if m.form.half {
		dur, arrow = "half day", " · "
	}
	if compact {
		dur, period, sep = dur[:4], m.leavePeriod(), ""
		arrow = strings.TrimSpace(arrow)
	}

	var parts []string
	if label {
		parts = append(parts, hinted("new timeoff", keys.NewLeave, theme.DayLabel, theme.HintKey))
	}
	// The type reads in its own colour — the same one its days are drawn in on the calendar
	// below — and keeps it while it holds the keys, only bolder: cycling the dropdown is
	// exactly when that colour has something to say, and the accent frame already says which
	// field the keys are going into.
	kindInk := theme.LeaveInk(theme.LeaveColor(kind))
	if m.form.field == leaveKindField {
		kindInk = kindInk.Bold(true)
	}
	parts = append(parts,
		m.leaveField(kindInk.Render(pad(kind, m.leaveKindWidth()))+theme.Dim.Render(" ▾"), leaveKindField, compact),
		m.leaveField(theme.DayLabel.Render(dur)+theme.Dim.Render(" ▾"), leaveDurField, compact),
		m.leaveField(m.leaveDate(0), leaveFromField, compact))
	if m.form.half {
		// A half day is one day: the range's end has nothing to say, so the slot asks which
		// half instead.
		parts = append(parts, theme.Dim.Render(arrow),
			m.leaveField(theme.DayLabel.Render(pad(period, m.leavePeriodWidth(compact)))+
				theme.Dim.Render(" ▾"), leaveToField, compact))
	} else {
		parts = append(parts, theme.Dim.Render(arrow),
			m.leaveField(m.leaveDate(1), leaveToField, compact))
	}

	// The two buttons frame themselves in what they do — green commits, red starts over —
	// and take the accent only while the keys are on them.
	ok, drop := theme.FieldOk, theme.FieldDrop
	if m.form.field == leaveOKField {
		ok = theme.FieldFocus
	}
	if m.form.field == leaveXField {
		drop = theme.FieldFocus
	}
	if compact {
		ok, drop = ok.Padding(0), drop.Padding(0)
	}
	parts = append(parts,
		m.leaveField(desc, leaveDescField, compact),
		ok.Render(theme.Ok.Render("✓")),
		drop.Render(theme.Err.Render("✕")))

	// Joined as a block, with the gaps as parts of their own: the boxes are three lines and
	// the label is one, so it is centred against them rather than sitting on the top rule.
	if sep != "" {
		spaced := make([]string, 0, 2*len(parts))
		for i, p := range parts {
			if i > 0 {
				spaced = append(spaced, sep)
			}
			spaced = append(spaced, p)
		}
		parts = spaced
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, parts...)
}

// leaveDate draws one of the two date fields. A field just tabbed into shows its value
// **selected** — reversed out in the accent — because the next keystroke replaces the whole
// thing; that is what the accent fill means here, and the accent frame is what says which
// field has the keys. Once anything is typed the selection is gone and the input draws its
// own cursor again.
//
// Either way it is exactly dateWidth cells: an input renders its Width plus a cursor cell,
// and a selection that measured differently would move everything to its right.
func (m Model) leaveDate(i int) string {
	in := m.form.from
	field := leaveFromField
	if i == 1 {
		in, field = m.form.to, leaveToField
	}
	if m.form.field == field && m.form.fresh[i] {
		return theme.Match.Render(pad(in.Value(), dateWidth))
	}
	return in.View()
}

// leavePeriodWidth keeps the period dropdown one width whichever half is chosen, so cycling
// it does not move the fields to its right.
func (m Model) leavePeriodWidth(compact bool) int {
	if compact {
		return 2
	}
	return len("afternoon")
}

// leaveField frames one field, accent while it holds the keys. A compact row gives up the
// space inside the frames as well as the ones between them — the box is still a box.
func (m Model) leaveField(s string, field int, compact bool) string {
	box := theme.Field
	if m.form.field == field {
		box = theme.FieldFocus
	}
	if compact {
		box = box.Padding(0)
	}
	return box.Render(s)
}

// leaveKindWidth is the widest leave type's first word, so the dropdown does not resize as
// it is cycled — a field that changes width moves everything to its right.
func (m Model) leaveKindWidth() int {
	w := 6
	for _, k := range m.timeKinds {
		w = max(w, lipgloss.Width(firstWord(oneLine(k.Name))))
	}
	return w
}

// leaveSkeleton is the line's width with nothing in the description: every frame, both
// dropdowns, the dates or the period, the arrow and the two buttons, measured off the render
// itself rather than added up by hand.
//
// It measures both durations and takes the wider: the period dropdown is wider than the
// range's end it replaces, and a description sized for a full day would push the buttons off
// the row the moment half day was chosen.
func (m Model) leaveSkeleton(label, compact bool) int {
	full, half := m, m
	full.form.half, half.form.half = false, true
	return max(lipgloss.Width(full.leaveRow(label, compact, "")),
		lipgloss.Width(half.leaveRow(label, compact, "")))
}

// leaveMinDesc is the narrowest description worth showing. The line gives up its label to
// keep it — the mode indicator already says NEW TIMEOFF — and then its spacing, before it
// gives up the description itself.
const leaveMinDesc = 8

// leaveTier is how much of the line's furniture the width can hold: the label, and the
// spaces between the fields.
func (m Model) leaveTier() (label, compact bool) {
	switch {
	case m.leaveDescRoom(true, false) >= leaveMinDesc:
		return true, false
	case m.leaveDescRoom(false, false) >= leaveMinDesc:
		return false, false
	}
	return false, true
}

// leaveDescRoom is what is left for the description's own text: the row, less the skeleton,
// less the cursor cell an input always draws after it.
func (m Model) leaveDescRoom(label, compact bool) int {
	return m.cols() - gutter - m.leaveSkeleton(label, compact) - 1
}

// leaveDescWidth is what the description gets: the row less everything that is fixed, less
// its own frame and the cursor cell after its text. The input scrolls what it holds, so a
// narrow terminal costs visible characters and nothing else.
func (m Model) leaveDescWidth() int {
	label, compact := m.leaveTier()
	return max(m.leaveDescRoom(label, compact), 1)
}

// balanceCards is the design's row of boxes, one per leave type: the name with its filter
// key picked out, the days left, and what that figure is. Ruled above and below and divided
// by verticals, which is the box the design draws without spending a row on a border of its
// own per card.
//
// Three lines and two rules is four rows the calendar does not get, and the figures are
// worth them: how much leave is left is the question the screen answers second, after which
// days are gone.
func (m Model) balanceCards() []string {
	if len(m.timeKinds) == 0 {
		// Three blank rows, so the line below them sits where it will sit once the balances
		// land: an answer must not shove the calendar down.
		return []string{"", "", ""}
	}
	room := m.cols() - gutter
	div := theme.Sep.Render(" │ ")
	// An equal share each, less the dividers between them, with the cells that do not
	// divide evenly handed out from the right — the row has to end exactly where its own
	// rules do.
	n := len(m.timeKinds)
	widths := make([]int, n)
	share := max((room-(n-1)*3)/n, 8)
	for i := range widths {
		widths[i] = share
	}
	for i := 0; i < room-(n-1)*3-share*n && i < n; i++ {
		widths[n-1-i]++
	}

	names, figures, units := make([]string, 0, 4), make([]string, 0, 4), make([]string, 0, 4)
	for i, k := range m.timeKinds {
		w := widths[i]
		c := theme.LeaveColor(k.Name)
		// The full name when the card holds it, the first word when it does not: the word
		// carrying the key is the one that cannot be cut.
		label := oneLine(k.Name)
		if lipgloss.Width(label) > w {
			label = firstWord(label)
		}
		lower := strings.ToLower(label)
		// The initial is a span of its own — accent, because it is the key that filters by
		// this type — and a colour nested inside another does not survive the inner reset.
		names = append(names, center(theme.HintKey.Render(lower[:1])+
			lipgloss.NewStyle().Foreground(theme.DayInk).Render(label[1:]), w))

		figure := theme.LeaveInk(c).Bold(true)
		switch {
		case k.ID == m.timeFilter:
			// The filtering card is reversed out, so the calendar and the card that explains
			// it are never read apart.
			figure = theme.LeaveDay(c)
		case k.Available == 0:
			// Nothing left is not news in the type's own colour.
			figure = lipgloss.NewStyle().Foreground(theme.QuietInk)
		}
		// Double-width digits: the balance is the figure on this row worth reading from across
		// the desk, and a terminal makes a number bigger by making it wider.
		figures = append(figures, center(figure.Render(" "+wide(days(k.Available))+" "), w))
		units = append(units, center(
			lipgloss.NewStyle().Foreground(theme.UnitInk).Render("DAYS AVAILABLE"), w))
	}

	return []string{
		theme.Blur.Render(strings.Join(names, div)),
		theme.Blur.Render(strings.Join(figures, div)),
		theme.Blur.Render(strings.Join(units, div)),
	}
}

// wide renders a figure at double width, in the fullwidth digits: the one way a terminal has
// of drawing a bigger number without spending a second row on it.
//
// Two rows of block glyphs was bigger still and read worse — a balance is a number, and a
// number drawn out of quadrants stops looking like one.
func wide(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune('０' + (r - '0'))
		case r == '.':
			b.WriteRune('．')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// timePending is whether anything on the calendar// timePending is whether anything on the calendar is still waiting on approval.
func (m Model) timePending() bool {
	for _, l := range m.timeLeaves {
		if l.State != "validate" && (m.timeFilter == 0 || l.KindID == m.timeFilter) {
			return true
		}
	}
	return false
}

// timeLines is the body: the twelve months, laid out in rows of as many as fit, with the
// holiday panel beside them when there is room, and the index of the line the month in
// view starts on. Derived from the answer on every render, like every other body.
func (m Model) timeLines() ([]string, int) {
	if m.timeYear == 0 {
		if m.timeLoading {
			return []string{theme.Blur.Render(
				m.spin.View() + theme.Dim.Render(" reading this year's time off…"))}, -1
		}
		return []string{theme.Blur.Render(
			theme.Dim.Render("no time off yet — r to read this year"))}, -1
	}

	year, marks, cols := m.timeYearOf(), m.timeMarks(), m.timeCols()
	hold, focus := m.timeMonth(), -1
	blank := monthPanel{filler: theme.OnPanel(theme.Surface).Render(strings.Repeat(" ", monthCols))}
	// The months are cells of one surface, divided by hairlines rather than by gaps — the
	// style spec is explicit that every section is separated by a rule, never a space.
	vRule := theme.Sep.Background(theme.Surface).Render("│")
	hRule := theme.Sep.Render(strings.Repeat("─", cols*monthCols+cols-1))

	var lines []string
	for r := 0; r < (12+cols-1)/cols; r++ {
		if r > 0 {
			lines = append(lines, hRule)
		}
		blocks, tall := make([]monthPanel, 0, cols), 0
		for c := range cols {
			mon := r*cols + c
			if mon > 11 {
				blocks = append(blocks, blank) // a short last row is padded, not shifted
				continue
			}
			if mon == hold {
				focus = len(lines)
			}
			b := m.monthBlock(year, time.Month(mon+1), marks)
			tall = max(tall, len(b.lines))
			blocks = append(blocks, b)
		}
		// Every month in the row is padded to the tallest of them, on its own panel, and no
		// further: a month that spans five weeks beside one that spans six costs one line,
		// where padding all twelve to six would cost the year four.
		for i := range tall {
			row := make([]string, 0, cols)
			for _, b := range blocks {
				if i >= len(b.lines) {
					row = append(row, b.filler)
					continue
				}
				row = append(row, b.lines[i])
			}
			lines = append(lines, strings.Join(row, vRule))
		}
	}

	for i, l := range lines {
		lines[i] = theme.Blur.Render(l)
	}
	return lines, focus
}

// withHolidayPanel puts the public holidays down the right of the calendar, in their own
// column with a rule between.
//
// It runs on the **windowed** body, after View has cut it to the rows it has, so the panel
// is pinned: the months scroll under it and the holidays stay where they are read. Zipped
// into the body before that, the list scrolled away with January and the header of it went
// with the first keypress.
func (m Model) withHolidayPanel(body []string) []string {
	panel := m.holidayPanel(m.timeYearOf())
	if len(panel) == 0 {
		return body
	}
	// Flush to the right edge, with the rule against it: the panel is a column of the
	// screen, not a column of the calendar, so it does not move when the months do or when
	// one of them is a week shorter.
	rule := theme.Sep.Render("│")
	grid := m.cols() - m.panelCells() - 3

	out := make([]string, len(body))
	for i, l := range body {
		out[i] = pad(l, grid) + " " + rule + " "
		if i < len(panel) {
			out[i] += panel[i]
		}
		out[i] = strings.TrimRight(out[i], " ")
	}
	// A panel taller than the body says so on its last line rather than stopping mid-year.
	if hidden := len(panel) - len(body); hidden > 0 && len(out) > 0 {
		out[len(out)-1] = strings.Repeat(" ", grid) + " " + rule + " " +
			theme.Dim.Render(fmt.Sprintf("… %d more", hidden+1))
	}
	return out
}

// monthPanel is one month drawn on its own panel: the lines it occupies, and the blank
// panel line the row pads short months with, so a five-week month beside a six-week one is
// still a panel all the way down.
type monthPanel struct {
	lines  []string
	filler string
}

// monthBlock is one month: its name and how many days off it holds, the weekday heads, and
// its four to six rows of dates, all on the month's own panel.
//
// The panel colour is on every span rather than wrapped around each line: a background set
// around a whole line dies at the first span that sets its own — a weekend badge, a leave
// day — and never comes back, which drew the panel as a stripe that stopped at the first
// coloured day.
func (m Model) monthBlock(year int, mon time.Month, marks map[string]dayMark) monthPanel {
	// The colour the days being typed are filled with: the type the request line has picked.
	var want lipgloss.Color
	if k, ok := m.leaveKind(); ok && m.form.open {
		want = theme.LeaveColor(k.Name)
	}
	first := time.Date(year, mon, 1, 0, 0, 0, 0, time.UTC)
	// Monday-first, so the two weekend columns sit together at the end of the week.
	lead := (int(first.Weekday()) + 6) % 7
	last := first.AddDate(0, 1, -1).Day()

	taken := 0.0
	for d := 1; d <= last; d++ {
		if mk := marks[fmt.Sprintf("%04d-%02d-%02d", year, int(mon), d)]; mk.kind != "" {
			if mk.half {
				taken += 0.5
				continue
			}
			taken++
		}
	}

	// The month the window is built around is the tinted panel and takes the caret and the
	// accent; the rest are quiet. That is today's month when nothing has moved it, and the
	// month a typed date lands in while the new-timeoff line is open — either way it says
	// which month the screen is answering for. The year rides along two digits, as in the
	// design: a month on its own could be any year's.
	bg := theme.Surface
	label := fmt.Sprintf("%s %02d", mon.String()[:3], year%100)
	name, style := "  "+label, theme.Header
	if int(mon)-1 == m.timeMonth() {
		bg = theme.PanelHold
		name, style = "▸ "+label, theme.TitleFocus
	}
	on := theme.OnPanel(bg)
	fill := func(n int) string {
		if n <= 0 {
			return ""
		}
		return on.Render(strings.Repeat(" ", n))
	}

	count := ""
	if taken > 0 {
		count = lipgloss.NewStyle().Foreground(theme.QuietInk).Background(bg).
			Render(days(taken) + "d ")
	}
	head := fill(monthPad) + style.Background(bg).Render(name)
	head += fill(monthCols-lipgloss.Width(head)-lipgloss.Width(count)) + count

	// The design pads the month cell — a line above its name and one under the weekday
	// heads — and a terminal too short for the year spends those two rows on days instead.
	roomy := m.rows() >= roomyRows
	var out []string
	if roomy {
		out = append(out, fill(monthCols))
	}
	out = append(out, head)
	if roomy {
		out = append(out, fill(monthCols))
	}
	out = append(out, m.weekdayHead(bg))
	today := time.Now().Format("2006-01-02")
	for r := range (lead + last + 6) / 7 {
		if r > 0 {
			out = append(out, fill(monthCols)) // air under the week above
		}
		// The ISO week of the row's Monday, which in the first row can belong to the month
		// before — or to the year before, which ISOWeek answers for correctly.
		monday := first.AddDate(0, 0, r*7-lead)
		_, week := monday.ISOWeek()
		cells := make([]string, 0, 9)
		cells = append(cells, fill(monthPad),
			lipgloss.NewStyle().Foreground(theme.WeekInk).Background(bg).
				Render(fmt.Sprintf("%2d ", week)))
		for c := range 7 {
			d := r*7 + c - lead + 1
			if d < 1 || d > last {
				cells = append(cells, fill(dayCell))
				continue
			}
			date := fmt.Sprintf("%04d-%02d-%02d", year, int(mon), d)
			cells = append(cells,
				dayCellOf(d, marks[date], c >= 5, date == today, bg, want)+fill(dayCell-badgeCell))
		}
		out = append(out, strings.Join(cells, ""))
	}
	if roomy {
		out = append(out, fill(monthCols))
	}
	return monthPanel{lines: out, filler: fill(monthCols)}
}

// weekdayHead is the wk column and Monday-first weekday initials, right-aligned over the
// dates they head, and the weekend pair dim so the two columns nobody works read as the
// quiet ones. One letter each, as in the design: T and T are told apart by their column,
// which is the only thing a date under them is read by anyway.
func (m Model) weekdayHead(bg lipgloss.Color) string {
	cells := []string{
		theme.OnPanel(bg).Render(strings.Repeat(" ", monthPad)),
		lipgloss.NewStyle().Foreground(theme.WeekInk).Background(bg).Render("wk "),
	}
	for i, d := range []string{"M", "T", "W", "T", "F", "S", "S"} {
		ink := theme.Muted
		if i >= 5 {
			ink = theme.QuietInk
		}
		cells = append(cells, lipgloss.NewStyle().Foreground(ink).Background(bg).
			Render(center(d, dayCell)))
	}
	return strings.Join(cells, "")
}

// dayCellOf is one day: a four-cell badge, " 21 ", filled when the day carries anything and
// plain on the month's own surface when it does not. The date is reversed out of the fill in
// dark ink — never white on a colour — and the fifth cell of the column, added by the caller,
// is the gap that keeps two filled days from reading as one bar.
//
// Priority, from the style spec: the day being typed, then leave taken, then a weekend or a
// holiday, then today, then a plain working day.
func dayCellOf(day int, mk dayMark, weekend, today bool, bg, want lipgloss.Color) string {
	num := fmt.Sprintf(" %2d ", day)
	badge := func(fill lipgloss.Color, ink lipgloss.Color, underline bool) lipgloss.Style {
		st := lipgloss.NewStyle().Foreground(ink).Background(fill).Bold(true)
		if underline {
			st = st.Underline(true)
		}
		return st
	}

	switch {
	case mk.selected:
		// The days the request line covers, reversed out in the accent — the same mark a date
		// jump leaves on the rows it found, and the one thing on this screen the keys are
		// about. The type it is for is on the request line itself, in its own colour.
		return badge(theme.Accent, theme.Ink, false).Render(num)

	case mk.kind != "":
		fill := theme.LeaveColor(mk.kind)
		if !mk.half {
			return badge(fill, theme.Ink, mk.pending).Render(num)
		}
		// A half day fills half the badge: the morning is its left half, the afternoon its
		// right, which says both that it is half a day and which half.
		quiet := badge(theme.WeekendBand, theme.QuietInk, mk.pending)
		full := badge(fill, theme.Ink, mk.pending)
		if mk.period == "pm" {
			return quiet.Render(num[:2]) + full.Render(num[2:])
		}
		return full.Render(num[:2]) + quiet.Render(num[2:])

	case mk.holiday != "", weekend:
		// A day nobody works is the faint fill with its number dimmed rather than reversed
		// out: it is not an event, it is a day off the calendar.
		return lipgloss.NewStyle().Foreground(theme.QuietInk).Background(theme.WeekendBand).
			Render(num)
	}

	// Nothing on it: the month's own surface, today in the accent's ink — a filled accent cell
	// is what the days being typed take, and today is not a cursor.
	ink := lipgloss.NewStyle().Foreground(theme.DayInk).Background(bg)
	if today {
		ink = theme.HintKey.Background(bg)
	}
	return ink.Render(num)
}

// holidayPanel is the public holidays, one line each: the dates in their own column, then
// a colon, then the name. Nil when the width cannot hold the column, in which case the
// calendar's own dimmed days are the answer.
func (m Model) holidayPanel(year int) []string {
	panelCells := m.panelCells()
	if panelCells == 0 {
		return nil
	}
	// The panel is a raised surface of its own, as the style spec has it, so the colour is on
	// every span: one wrapped around the line would stop at the first accent date.
	on := theme.OnPanel(theme.Raised)
	ink := lipgloss.NewStyle().Foreground(theme.DayInk).Background(theme.Raised)
	fill := func(s string, w int) string {
		if n := w - lipgloss.Width(s); n > 0 {
			return s + on.Render(strings.Repeat(" ", n))
		}
		return s
	}
	head := lipgloss.NewStyle().Foreground(theme.DayInk).Background(theme.Raised).Bold(true).
		Render(" PUBLIC HOLIDAYS")
	// The count gives up its year, and then itself, rather than pushing the column wider
	// than the one the layout handed out.
	quiet := lipgloss.NewStyle().Foreground(theme.QuietInk).Background(theme.Raised)
	count := ""
	for _, try := range []string{
		fmt.Sprintf("%d in %d ", len(m.timeHolidays), year),
		fmt.Sprintf("%d ", len(m.timeHolidays)),
	} {
		if lipgloss.Width(head)+len(try) <= panelCells {
			count = quiet.Render(try)
			break
		}
	}
	out := []string{
		head + on.Render(strings.Repeat(" ",
			max(panelCells-lipgloss.Width(head)-lipgloss.Width(count), 0))) + count,
		on.Render(strings.Repeat(" ", panelCells)),
	}

	hold := time.Month(m.timeMonth() + 1)
	for _, h := range m.timeHolidays {
		mark, date, name := on.Render(" "), theme.Dim.Background(theme.Raised), ink
		// The month in view is picked out down the panel's own edge, so the list answers the
		// part of the calendar you are looking at rather than the whole year at once.
		if t, err := time.Parse("2006-01-02", h.From); err == nil && t.Month() == hold {
			mark = theme.HintKey.Background(theme.Raised).Render("▎")
			date = theme.HintKey.Background(theme.Raised)
			name = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).
				Background(theme.Raised)
		}
		out = append(out, fill(mark+date.Render(pad(holidaySpan(h), spanCells))+
			lipgloss.NewStyle().Foreground(theme.WeekInk).Background(theme.Raised).Render(": ")+
			// oneLine like every other string the ERP wrote: a newline in a holiday name
			// renders the panel a row taller than the column it was given.
			name.Render(trunc(oneLine(h.Name), panelCells-spanCells-3)), panelCells))
	}
	return out
}

// holidaySpan is a holiday's dates, as short as they can be said: one day is "Aug 5", a
// run inside one month collapses to "Mar 18-23", and one that crosses a month keeps both
// names.
func holidaySpan(h api.Holiday) string {
	from, err := time.Parse("2006-01-02", h.From)
	if err != nil {
		return h.From
	}
	to, err := time.Parse("2006-01-02", h.To)
	if err != nil || !to.After(from) {
		return from.Format("Jan 2")
	}
	if to.Month() == from.Month() {
		return from.Format("Jan 2") + "-" + to.Format("2")
	}
	return from.Format("Jan 2") + "-" + to.Format("Jan 2")
}

// monthMoveHelp is the footer's entry for the four motions, named after what they move —
// months, not rows or days — with the keys read off the bindings themselves so a rebind
// follows.
func monthMoveHelp() key.Binding {
	keysOf := []string{keys.Collapse.Help().Key, keys.Down.Help().Key,
		keys.Up.Help().Key, keys.Expand.Help().Key}
	return key.NewBinding(key.WithHelp(strings.Join(keysOf, "/"), "month"))
}

// filterHelp is the footer's entry for the filters: the leave types' own initials, in the
// order the chips are in, since that is where they are read off. Nothing to show before
// the types have landed.
func (m Model) filterHelp() key.Binding {
	if len(m.timeKinds) == 0 {
		return key.NewBinding()
	}
	letters := make([]string, 0, len(m.timeKinds))
	for _, k := range m.timeKinds {
		letters = append(letters, strings.ToLower(firstWord(k.Name)[:1]))
	}
	return key.NewBinding(key.WithHelp(strings.Join(letters, " "), "filter"))
}

// timeLabel is the mode indicator on this tab: the filter, when there is one, since a
// calendar showing one type of leave and one showing all of them look alike.
func (m Model) timeLabel() string {
	if k, ok := m.timeKind(m.timeFilter); ok {
		return "-- " + strings.ToUpper(oneLine(k.Name)) + " --"
	}
	return "-- TIME OFF --"
}

// days renders a day count the way a person writes one: 8.5, but 8 rather than 8.0.
func days(f float64) string {
	if f == math.Trunc(f) {
		return fmt.Sprintf("%.0f", f)
	}
	return fmt.Sprintf("%.1f", f)
}

// firstWord is the part of a leave type's name that identifies it: "Annual Time Off" is
// three words of which only the first says anything.
func firstWord(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return s
}

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
	if m.mode != ModeConfirm && m.mode != ModeAuth && m.mode != ModeForm {
		switch m.tab {
		case TabDash:
			label = "-- DASHBOARD --"
		case TabTime:
			label = m.timeLabel()
		}
	}

	// A modal is the exception: whatever accepts it has to be on screen, since it is
	// holding the keyboard and ? will not reach the toggle.
	help := []key.Binding{keys.Help}
	switch {
	case m.mode == ModeConfirm:
		// Which key accepts depends on what is being confirmed.
		help = []key.Binding{m.confirmKeys(), keys.No}
	case m.mode == ModeForm:
		// Its own keys, whichever tab is behind it, and only the ones the focused field
		// takes: j/k belong to a dropdown and are letters everywhere else.
		help = []key.Binding{keys.Next, keys.Accept, keys.ClearField, keys.Cancel}
		if m.leaveFieldIsDropdown() {
			help = []key.Binding{keys.Next, keys.Cycle, keys.Accept, keys.Cancel}
		}
	case !m.showHelp:
	case m.tab == TabDash:
		// It moves in screenfuls, not days, and there is no i: the query field filters
		// tasks and this is not that tab.
		help = []key.Binding{keys.Top, keys.HalfDown, keys.PrevMonth, m.clockHelp(),
			keys.TasksTab, keys.Refresh, keys.Quit, keys.Help}
	case m.tab == TabTime:
		// It moves in months, and the filters are the leave types' own initials, so they
		// are named by the answer rather than by the keymap.
		help = []key.Binding{keys.NewLeave, monthMoveHelp(), keys.Top, m.filterHelp()}
		if m.timeFilter != 0 {
			help = append(help, key.NewBinding(
				key.WithHelp(keys.Back.Help().Key, "clear filter")))
		}
		help = append(help, keys.TasksTab, keys.Refresh, keys.Quit, keys.Help)
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
