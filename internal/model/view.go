package model

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
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
	// A week row gets a line under it when the terminal can spend one: that air is what makes
	// the month read as a calendar rather than as a table, and it is the design's own
	// proportion — a day is nearly as tall as it is wide there. A roomier terminal also gets
	// the padding the design puts around the month cell. Both are given up to keep **two rows
	// of months** on screen, which is how the year is read here — six months at a time, a
	// half-year to a screenful (`timeTier`).
	//
	// timeChrome is what the calendar's own furniture costs in rows: the tab bar, the balance
	// cards, the two rules with the request line between them, then the status line and the
	// footer. An estimate, like dashChrome — being a row out only moves where a tier changes.
	timeChrome = 13
	// A month is its name, its weekday heads and six week rows — 8 rows bare, 13 with the air
	// between weeks, 16 with the design's padding round it as well.
	monthWeeks = 6
	monthBare  = monthWeeks + 2
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
	spanCells = 12 // "Sep 10 (Wed)" and "Mar 30-Apr 2", the longest a span gets
	// bookScopeCells is the widest scope name plus its arrow, so the dropdown does not
	// resize as it is cycled — a field that changes width moves everything to its right.
	bookScopeCells = 10

	// An employee card is the four lines the ERP's own directory shows — name, job title,
	// email, phone — inside a rounded box, so six rows with its border and the blank line
	// under it. One card a row: the lines are sentences, not columns, and two cards side by
	// side would cut the job titles this office writes ("Intern – Process Improvement &
	// Operational Efficiency - L0").
	empCardLines = 6
	// The label column in an open row: "project mgr" is the longest of them.
	empLabelCells = 13
	// The same idea on a requisition, where the labels are the ERP's own and longer:
	// "purpose of replacement".
	reqLabelCells = 24
	// The new-requisition line: the category dropdown's own width, a many2one pick's, and the
	// narrowest a text field is worth drawing.
	// The form under the table: its label column, and the widest a value is worth drawing.
	reqFormLabel = 26 // "purpose of replacement *" whole, which is the longest of them here
	reqValueMax  = 52
	// A row is two fixed columns: the name, then the job title in a chip. Fixed, so the chips
	// start on the same cell down the whole list — pushed to the right edge instead, every
	// title began somewhere else and the column read as ragged. 30 holds all but the longest
	// names here ("Rafee Mizan Khan Chowdhury"), and 40 holds the titles this office writes
	// ("Assistant Manager-Administration & Operation - L4" is cut, and is meant to be).
	empNameCells = 30
	empJobCells  = 40
	// Narrower than this a chip has no room to be one, so the name gives up cells instead.
	empMinJob = 14
	// The card is as wide as its widest line and no wider, capped: stretched to a 200-cell
	// terminal a card of four short lines reads as a banner.
	empCardCells = 52
	// A project row is two fixed columns and a count on the right edge, the same idea as an
	// employee row: the name gets 60 cells, so every chip starts on the same cell and the
	// teams read as a column rather than as a ragged edge. 60 holds all but the longest names
	// here ("Value-Driven Engagement, Internal Meetings & Tasks" is 50).
	projNameCells = 60
	// Narrower than this a chip has no room to be one, so the name gives up cells instead —
	// but the name loses them last, since it is what the row is for.
	projMinTeams = 12
	// An open row's own block: indented under the name, one label column for the manager, and
	// the widest a member's name column is worth drawing ("Rafee Mizan Khan Chowdhury Niloy").
	// What the found-people modal spends before a name: its own frame, the pinned head and the
	// blank under it, and the status line it sits above.
	projFoundChrome = 6
	projIndent      = "     "
	projLabelCells  = 10
	projMemberName  = 34
)

// View stacks a fixed header, a windowed list, and a fixed footer. The header and
// footer are laid out first and the list gets whatever rows are left, so the
// search field can never be pushed off the top of the screen.
func (m Model) View() string {
	top := m.tabBar()
	if m.tab == TabTime {
		// The year and what has been taken out of it ride here: the tab bar's row is half
		// empty, and a title line of its own would cost the calendar a row. Only when the bar
		// leaves room for it — spread clamps its gap to two cells rather than truncating, so
		// on a bar that already fills the width it would push the row three cells over and
		// wrap the whole screen.
		if room := m.cols() - lipgloss.Width(top) - 2; room > 0 {
			top = spread(top, m.timeSummary(room)+" ", m.cols())
		}
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
		// The keys themselves take the accent, which is what that colour means everywhere
		// else: this is what to press. The slash between them stays dim — it is punctuation,
		// not a key.
		hint := theme.HintKey.Render(m.confirmKeys().Help().Key) +
			theme.Dim.Render(" / ") + theme.HintKey.Render(m.k().No.Help().Key)
		// A prompt of several lines puts its keys on a line of their own: appended to the
		// last one they read as part of it, and "Coast trip  y / n" is not a description.
		prompt := m.cPrompt + "  " + hint
		if strings.Contains(m.cPrompt, "\n") {
			prompt = m.cPrompt + "\n\n" + hint
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
	case ModeLeaves:
		tail = append(tail, strings.Split(m.leavesModal(), "\n")...)
	case ModeEmpSearch:
		// Above the status line, like the date jump's own prompt: it belongs to the moment it
		// is being typed, and the header says the filter is on once the prompt has closed.
		tail = append(tail, theme.Blur.Render(m.empQuery.View()))
	case ModeProjJump:
		// Above the status line, exactly where the date jump's own prompt goes: it belongs to
		// the moment it is being typed.
		tail = append(tail, theme.Blur.Render(m.find.View()))
	case ModeProjFound:
		tail = append(tail, strings.Split(m.projFoundModal(), "\n")...)
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
		frames := []*[]string{&dFoot, &dHead}
		if m.wfh.open {
			// The request line lives in the head and holds the keyboard, so the head cannot be
			// the thing that is given up: a field you cannot see is a field you cannot fill.
			frames = frames[:1]
		}
		for _, frame := range frames {
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
	if m.tab == TabMeal {
		// The month, the keys that step it and the legend are laid out with the header, so
		// the weeks scroll under the figures they are read against rather than taking them
		// along.
		head = append(head, m.mealHead()...)
	}
	if m.tab == TabEmp {
		head = append(head, m.empHead()...)
	}
	if m.tab == TabReq {
		head = append(head, m.reqHead()...)
	}
	if m.tab == TabProj {
		head = append(head, m.projHead()...)
	}

	// The body takes the rows left between header and footer, and is padded out to
	// them, which pins the status line and the key hints to the bottom of the screen.
	budget := m.rows() - len(head) - len(tail)
	body, focus := m.listLines()
	if m.tab == TabTime {
		body, focus = m.timeLines(budget)
	}
	if m.tab == TabMeal {
		body, focus = m.mealLines()
	}
	if m.tab == TabEmp {
		body, focus = m.empLines()
	}
	if m.tab == TabReq {
		body, focus = m.reqLines()
	}
	if m.tab == TabProj {
		body, focus = m.projLines()
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
	// The book-meal line sits directly under the calendar with one blank line above it, not
	// pinned to the bottom of the screen: it is about the days on the grid, and a row of
	// fields a screen away from them reads as belonging to nothing. Its rows come out of the
	// weeks' budget, so the month is windowed into what is left rather than being pushed off.
	var band []string
	if m.tab == TabMeal {
		band = m.bookBand()
		budget = max(budget-len(band)-1, 1)
	}
	if m.tab == TabReq {
		// Under the table, the way the book-meal line sits under the calendar: it is about what
		// goes into the table, and a form a screen away from it reads as belonging to nothing.
		// Its rows come out of the table's budget, so the rows window into what is left rather
		// than the form being pushed off the bottom.
		band = m.reqBand()
		budget = max(budget-len(band), 1)
	}
	body = window(body, focus, budget)
	if m.tab == TabMeal {
		// Directly under the last week, before the padding rather than after it: padded first,
		// the line drifted to the bottom of the screen, which is what it is not for.
		body = append(body, "")
		body = append(body, band...)
		budget += len(band) + 1
	}
	if m.tab == TabReq {
		body = append(body, band...)
		budget += len(band)
	}
	for len(body) < budget {
		body = append(body, "")
	}
	if m.tab == TabTime {
		// After the window, not before it: the holiday list is a pinned column, so the
		// months scroll under it rather than taking it with them.
		body = m.withHolidayPanel(body)
	}
	if m.tab == TabMeal {
		// Same reason: the week's menu is a column of the screen, so the weeks scroll under it.
		// Composed over the whole body, the line included, so the column keeps its own length
		// instead of being cut to however tall this month happens to be.
		body = m.withMealPanel(body)
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
		{TabTasks, "tasks", m.k().TasksTab},
		{TabDash, "dashboard", m.k().DashTab},
		{TabTime, "timeoff", m.k().TimeTab},
		{TabMeal, "meal", m.k().MealTab},
		{TabEmp, "employee", m.k().EmpTab},
		{TabReq, "requisitions", m.k().ReqTab},
		{TabProj, "projects", m.k().ProjTab},
	}

	bar := func(short bool) string {
		var b strings.Builder
		b.WriteString("  ")
		for i, t := range tabs {
			active := t.tab == m.tab
			body, hint := theme.Dim, theme.HintKey
			if active {
				body, hint = theme.Pill, theme.PillKey
			}
			label := t.label
			if short {
				// A terminal too narrow for five words keeps the digits and the letters and
				// gives up the words: every tab is still named by the key that reaches it,
				// which is the part of the label that does the work.
				label = t.key.Help().Key
			}
			// A superscript index sits at the top-left of the label, btop style, and is a key
			// in its own right: 1 and 2 in bar order, alongside the letter.
			b.WriteString(body.Render(" ") + hint.Render(superscript(i+1)) +
				hinted(label, t.key, body, hint) + body.Render(" "))
			b.WriteString("  ")
		}
		return b.String()
	}
	// The bar is the first line of every screen, so it can never be the thing that wraps: a
	// wrapped bar pushes the whole UI down a row and scrolls the terminal.
	full := bar(false)
	if lipgloss.Width(full) > m.cols() {
		return bar(true)
	}
	return full
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

	// Two boxes, three lines each, so the things on this screen you can press look like it.
	// They share the button band between the month's title and its totals: the clock on the
	// right edge where its own status line sits, and the month's confirm on the left, which
	// costs the chart no rows of its own. Each line goes in separately: View budgets rows by
	// counting elements.
	out := []string{m.clockLine(title, m.clockStatus())}
	confirm := strings.Split(m.confirmButton(), "\n")
	for i, l := range strings.Split(m.clockButton(), "\n") {
		left := ""
		if i < len(confirm) {
			left = confirm[i]
		}
		out = append(out, m.clockLine(left, l))
	}
	// The WFH request line goes under the button, on the same right edge: it is about the check
	// in that was just refused, and the button is what refused it. Nothing opens it but that
	// refusal, so the rows cost the chart nothing the rest of the time.
	if m.wfh.open {
		for _, l := range m.wfhBand() {
			out = append(out, m.clockLine("", l))
		}
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
	return key.NewBinding(key.WithKeys(m.k().Clock.Keys()...),
		key.WithHelp(m.k().Clock.Help().Key, what))
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
	box, label := theme.ClockIn, hinted("check in", m.k().Clock, theme.ClockText, theme.HintKey)
	switch {
	case m.wfh.open:
		// The request line under it holds the keyboard, so c does nothing: the button goes dim
		// and gives up the accent, since an accent on a key that cannot fire is the accent lying.
		box, label = theme.ClockOff, theme.Dim.Render("check in")
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
		label = hinted("check out", m.k().Clock, theme.ClockText, theme.HintKey)
	}
	return box.Render(label)
}

// confirmButton is the month's own hour logs, told to the ERP that they are done: a box like
// the clock's, since it is the other thing on this screen you press.
//
// The border is **green** — the colour that invites an action not yet taken, the same as the
// check in button's — and it stays green while the month is unconfirmed rather than turning
// amber: nothing here is running, there is only something to do. The words are white and only
// the `C` is the accent, exactly as on the clock, so the key reads as the key.
func (m Model) confirmButton() string {
	box := theme.ClockIn
	if m.confirming {
		// The loader takes the label's place so the box does not move while the ERP thinks
		// about it, the same as the clock's does.
		return box.Render(m.spin.View() + " " + theme.ClockText.Render("confirming…"))
	}
	if m.wfh.open {
		// The WFH line has the keyboard, so C cannot fire: a lit key that does nothing is the
		// accent lying, the same rule the clock button follows.
		return theme.ClockOff.Render(theme.Dim.Render("Confirm hour logs"))
	}
	if m.mode == ModeConfirm && m.cKind == confirmHourLogs {
		// Pressed: the box fills with the green its border carries and the words go white,
		// which is what a held-down button looks like everywhere else in this app. The key is
		// not picked out here — it has already been pressed, and the modal in front of it is
		// what the keyboard is answering.
		return theme.ClockOn.Render(theme.ClockOnText.Render("Confirm hour logs"))
	}
	return box.Render(hinted("Confirm hour logs", m.k().ConfirmHours,
		theme.ClockText, theme.HintKey))
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

// wfhBand is the work-from-home request line: the two days, why, and the two buttons. Three
// rows, because every field is a box, the same as the time off line's are.
//
// It is not opened by a key. The ERP refuses a check in once the free WFH days are gone and
// says what it wants instead, so the line is the answer to that sentence and appears with it.
func (m Model) wfhBand() []string {
	// Bare lines: clockLine is what puts them on the right edge and in the blurred gutter, the
	// same as the button's own three rows, and a second gutter here pushed the row off the edge.
	return strings.Split(m.wfhRow(m.wfh.reason.View(), m.wfhCompact()), "\n")
}

// wfhRow draws the line with the reason given, so the same code both renders it and measures
// what is left for that reason — the two cannot disagree about a cell.
//
// compact is the narrow terminal's version: the boxes give up the space inside them but stay
// boxes, the same trade the book-meal line makes.
func (m Model) wfhRow(reason string, compact bool) string {
	parts := []string{
		theme.DayLabel.Render("wfh request"),
		m.wfhField(m.wfhDate(0), wfhFromField, compact),
		theme.Dim.Render(" → "),
		m.wfhField(m.wfhDate(1), wfhToField, compact),
		m.wfhField(reason, wfhReasonField, compact),
	}
	// The two buttons carry what they do in the frame and fill when the keys are on them,
	// exactly as the other lines' do: these are pressed, not typed into.
	ok, drop := theme.FieldOk, theme.FieldDrop
	tick, cross := theme.Ok, theme.Err
	if m.wfh.field == wfhOKField {
		ok, tick = theme.FieldOkOn, theme.OnOk
	}
	if m.wfh.field == wfhXField {
		drop, cross = theme.FieldDropOn, theme.OnDrop
	}
	if compact {
		ok, drop = ok.Padding(0), drop.Padding(0)
	}
	parts = append(parts, ok.Render(tick.Render("✓")), drop.Render(cross.Render("✕")))

	spaced := make([]string, 0, 2*len(parts))
	for i, p := range parts {
		if i > 0 {
			spaced = append(spaced, " ")
		}
		spaced = append(spaced, p)
	}
	// Center, which is what puts the one-line label beside the three-line boxes rather than
	// on their top rule.
	return lipgloss.JoinHorizontal(lipgloss.Center, spaced...)
}

// wfhField is one box on the line, framed in the accent while it holds the keys.
func (m Model) wfhField(s string, field int, compact bool) string {
	box := theme.Field
	if m.wfh.field == field {
		box = theme.FieldFocus
	}
	if compact {
		box = box.Padding(0)
	}
	return box.Render(s)
}

// wfhDate draws one of the two date fields. A field just tabbed onto shows its value
// selected — the next keystroke replaces the whole thing — which is what the accent fill
// means here, as it does on every other line.
func (m Model) wfhDate(i int) string {
	in := m.wfh.from
	if i == 1 {
		in = m.wfh.to
	}
	if m.wfhDateIndex() == i && m.wfh.fresh[i] {
		// Padded to the full column, so a selected value measures exactly what the input it
		// stands in for does: sized a cell short, the box shrank as the keys moved onto it and
		// the two buttons slid along the row.
		return theme.Match.Render(pad(in.Value(), dateWidth))
	}
	return in.View()
}

// wfhSkeleton is the row with an empty reason: everything on it that is fixed, measured on the
// row as it is drawn rather than added up, since the padding and the spaces between the boxes
// are each a cell that arithmetic forgets.
func (m Model) wfhSkeleton(compact bool) int {
	return lipgloss.Width(strings.Split(m.wfhRow("", compact), "\n")[1])
}

// wfhRoom is what the line has to fit in: the width, less the gutter, less the two cells
// clockLine keeps between the left of the row and the right-aligned cluster it puts the line in.
func (m Model) wfhRoom() int { return m.cols() - gutter - 2 }

// wfhCompact says whether the row has to give up the space inside its boxes: the reason needs a
// cell of its own and a cell for the cursor after it.
func (m Model) wfhCompact() bool {
	return m.wfhRoom()-m.wfhSkeleton(false) < 2
}

// wfhMaxReason keeps the line a cluster under the button rather than a band across the screen:
// stretched to the edge on a wide terminal the row ran the whole width, which read as a second
// header rather than as something belonging to the button above it. The input scrolls what it
// holds, so the cap costs visible characters and nothing else.
const wfhMaxReason = 24

// wfhReasonWidth is what the reason gets: the row less everything that is fixed, less the
// cursor cell an input always draws after its text, capped.
func (m Model) wfhReasonWidth() int {
	return min(max(m.wfhRoom()-m.wfhSkeleton(m.wfhCompact())-1, 1), wfhMaxReason)
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
// --- projects ----------------------------------------------------------------

// projHead is the list's own header: what it is, and how many projects are open.
func (m Model) projHead() []string {
	left := theme.Header.Render("PROJECTS")
	// The toggle sits before the count, on the right: it says what pressing `a` gives you
	// rather than what is on screen — the clock button's own rule — and the count beside it is
	// what says which of the two you are looking at.
	right := m.projFilterLabel() + "   " + m.projCount()
	if m.projLoading {
		// The list stays on screen while it is re-read, so the loader sits beside the count
		// rather than replacing the rows — the same as the directory and the chart's month.
		right = m.spin.View() + " " + right
	}
	// The query field first, as on the task list: a box across the width with the caret in the
	// gutter while it has the keys. The count line under it says what the query left.
	out := strings.Split(m.projSearchBox(), "\n")
	return append(out, theme.Blur.Render(spread(left, right, m.cols()-gutter)), "")
}

// projSearchBox is the query field, the task list's own box without the progress cluster beside
// it: the caret marks focus and keeps its cells when it goes, so nothing shifts, and a query
// that is on but not being typed into renders dim rather than with a cursor.
func (m Model) projSearchBox() string {
	caret := "   "
	if m.mode == ModeProjSearch {
		caret = theme.Prompt.Render(" ❯ ")
	}
	field := m.projQuery.View()
	if m.mode != ModeProjSearch && m.projQuery.Value() != "" {
		field = theme.Dim.Render(m.projQuery.Value())
	}
	frame := theme.SearchBox
	if m.mode == ModeProjSearch {
		frame = theme.SearchBoxFocus
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, caret,
		frame.Width(m.projBoxWidth()).Render(field))
}

// projBoxWidth is the box, and projFieldWidth the input inside it: the box's own two padding
// cells and the cursor cell the input draws after its Width, so no query can wrap the box onto
// a second line and shove the list down.
func (m Model) projBoxWidth() int {
	return max(m.cols()-caretCol-4, 24)
}

func (m Model) projFieldWidth() int {
	return max(m.projBoxWidth()-3, 16)
}

// projFilterLabel is the all/mine toggle: one label, `all projects`, with its key picked out —
// and the **whole** label takes the accent while it is on, which is the "frame says where the
// keys are, fill says the value is chosen" idiom the request lines use.
//
// One label rather than a name per state, because the key has to be inside the word it is on:
// "my projects" holds no `a`, so hinted() spelled the key out after it and the row read as
// "my projects a".
func (m Model) projFilterLabel() string {
	if !m.projMine {
		return hinted("all projects", m.k().Mine, theme.MatchText, theme.HintKey)
	}
	return hinted("all projects", m.k().Mine, theme.Dim, theme.HintKey)
}

// projCount is how many rows the filter leaves out of how many there are — the whole number
// alone when nothing is filtered, since "89 of 89" answers a question nobody asked.
func (m Model) projCount() string {
	shown, all := len(m.projRows()), len(m.projs)
	switch {
	case all == 0:
		return ""
	case shown == all:
		return theme.Dim.Render(fmt.Sprintf("%d open %s", all, plural(all, "project", "projects")))
	}
	return theme.Dim.Render(fmt.Sprintf("%d of %d", shown, all))
}

// projLines is the body: one row per project, shaped like the task list — the name, the teams
// on it in a chip, and the task count on the right edge, which is where a task's own entry
// count sits.
func (m Model) projLines() ([]string, int) {
	rows := m.projRows()
	if len(rows) == 0 {
		switch {
		case m.projLoading:
			return []string{theme.Blur.Render(
				m.spin.View() + theme.Dim.Render(" reading the projects…"))}, -1
		case len(m.projs) == 0:
			return []string{theme.Blur.Render(
				theme.Dim.Render("no projects yet — R to read them"))}, -1
		}
		if strings.TrimSpace(m.projQuery.Value()) == "" && m.projMine {
			// The toggle is what emptied it, not the query, so it names the key that fills it.
			return []string{theme.Blur.Render(theme.Dim.Render("none of these are yours — ") +
				theme.HintKey.Render(m.k().Mine.Help().Key) +
				theme.Dim.Render(" for all of them"))}, -1
		}
		return []string{theme.Blur.Render(theme.Dim.Render("no project matches that"))}, -1
	}

	var out []string
	focus := -1
	held := min(m.projHold, len(rows)-1)
	for i, p := range rows {
		if i == held {
			focus = len(out)
		}
		out = append(out, m.projRow(p, i == held))
		if m.projOpen[p.ID] {
			out = append(out, m.projDetailLines(p)...)
		}
		out = append(out, "") // one blank line between projects, as between tasks
	}
	return out, focus
}

// projDetailLines is the open row's own block: who runs the project, and everyone on its teams
// as a table of names and work emails. Indented under the name, the way an employee's own
// detail is.
func (m Model) projDetailLines(p store.Project) []string {
	var out []string
	if p.Manager != "" {
		out = append(out, theme.Blur.Render(projIndent+
			theme.Header.Render(pad("MANAGER", projLabelCells))+theme.DayLabel.Render(p.Manager)))
	}

	// The section says what the table under it is, and it stands whatever the table turns out
	// to hold — a wait and an empty answer are both about the members, so both read under it.
	out = append(out, theme.Blur.Render(projIndent+theme.Header.Render("TEAM MEMBERS")))

	people, have := m.projMembers[p.ID]
	switch {
	case have && len(people) > 0:
		// People in hand answer first: a refresh that moved the ids should not turn a table
		// already on screen into "no members".
	case len(p.Members) == 0:
		// Not a wait and not a failure: the ERP says the teams have nobody on them.
		return append(out, theme.Blur.Render(projIndent+
			theme.Dim.Render("no members on its teams")))
	case !have && m.projPulling[p.ID]:
		return append(out, theme.Blur.Render(projIndent+m.spin.View()+
			theme.Dim.Render(" reading its people…")))
	case !have:
		return append(out, theme.Blur.Render(projIndent+
			theme.Dim.Render("nothing read yet — l again")))
	case len(people) == 0:
		return append(out, theme.Blur.Render(projIndent+
			theme.Dim.Render("its people are not readable from here")))
	}

	// A table, because two facts a person read down two columns is a table — one sized to what
	// it actually holds, so a list of short names does not pay for a column it never fills.
	nameW, mailW := projMemberColumns(people, m.cols()-gutter-len(projIndent))
	out = append(out, theme.Blur.Render(projIndent+theme.Header.Render(
		pad("NAME", nameW)+trunc("EMAIL", mailW))))
	for _, who := range people {
		out = append(out, theme.Blur.Render(projIndent+
			theme.DayLabel.Render(pad(trunc(who.Name, nameW-1), nameW))+
			theme.Tag.Render(trunc(orDash(who.Email), mailW))))
	}
	return out
}

// projMemberColumns sizes the two columns from the rows themselves, capped, and gives the name
// its cells first: an email is recoverable from a name here, and a truncated name is not.
func projMemberColumns(people []store.Member, room int) (name, mail int) {
	for _, who := range people {
		name = max(name, lipgloss.Width(who.Name)+2)
		mail = max(mail, lipgloss.Width(who.Email))
	}
	name = min(name, projMemberName)
	if name+mail > room {
		mail = room - name
	}
	return name, max(mail, 0)
}

// projRow is one line: caret, name, the teams in a chip, and the task count against the right
// edge. The same shape as a task line, since it answers the same kind of question — what this
// is, whose it is, how much is in it.
func (m Model) projRow(p store.Project, focused bool) string {
	// No caret: the task list and the directory carry one because their rows open, and there
	// is nothing to open here. The indent is theirs, so the names line up across the tabs.
	caret := "   "
	count := theme.Dim.Render(fmt.Sprintf("%d %s", p.Tasks, plural(p.Tasks, "task", "tasks")))

	ink, chip := theme.DayLabel, theme.Chip
	if focused {
		// The accent marks whatever holds the keys, and the whole row takes it — a name in
		// the accent beside a chip still in the tag's teal reads as two rows overlapping,
		// which is the rule the directory's own rows follow.
		ink, chip = theme.TitleFocus, theme.ChipFocus
	}

	nameW, teamW := m.projColumns()
	name := ink.Render(pad(trunc(oneLine(p.Name), nameW-1), nameW))

	teams := ""
	// The chip's own frame and padding are four of its cells, so the names get the rest. A
	// project with no team draws no chip: an empty one reads as a team called nothing.
	if names := oneLine(strings.Join(p.Teams, ", ")); names != "" && teamW > 4 {
		teams = chip.Render(trunc(names, teamW-4))
	}
	return row(spread(caret+name+pad(teams, teamW), count, m.cols()-gutter), focused)
}

// projColumns is what the two columns get: the fixed widths where the terminal holds them, and
// the team column giving up cells first where it does not — a name is what the row is for, and
// a chip below projMinTeams has no room to be a chip, so the name pays after that.
//
// The count is reserved at its **widest** over the whole list, not measured per row, which is
// what keeps the chips in one column: sized row by row, "1315 tasks" and "9 tasks" moved the
// columns two cells apart on a terminal narrow enough for the name to be giving up cells.
func (m Model) projColumns() (name, teams int) {
	room := m.cols() - gutter - 3 - m.projCountCells() - 2 // the indent, the count, spread's gap
	name = projNameCells
	if teams = room - name; teams >= projMinTeams {
		return name, teams
	}
	name = max(room-projMinTeams, 12)
	return name, max(room-name, 0)
}

// projCountCells is the widest task count in the list, so every row lays its columns out
// against the same right-hand cluster.
func (m Model) projCountCells() int {
	n := 0
	for _, p := range m.projs {
		n = max(n, lipgloss.Width(fmt.Sprintf("%d %s", p.Tasks, plural(p.Tasks, "task", "tasks"))))
	}
	return n
}

// --- requisitions ------------------------------------------------------------

// The requisitions table's columns, in the order the ERP's own list view puts them. Fixed
// widths, so the columns line up down the screen and the head means what it says; a narrow
// terminal drops them from the **right** (`reqCols`), which is the order they stop being worth
// a cell in — the designation is the same on every row of your own list, and who it is for is
// you.
const (
	reqCatCells    = 24 // "Accessories Replacement Requisition" is cut, and is meant to be
	reqDateCells   = 10 // dd/mm/yy, and wide enough for SUBMITTED to head it uncut
	reqForCells    = 18
	reqDesigCells  = 26 // "Senior Software Engineer" whole, which is most of this office
	reqStageCells  = 10
	reqUrgentCells = 8 // headed URGENT, the tick under it
)

// reqHead is the table's own header: the count, then the column names under it.
func (m Model) reqHead() []string {
	left := theme.Header.Render("REQUISITIONS")
	right := ""
	if n := len(m.reqs); n > 0 {
		right = theme.Dim.Render(fmt.Sprintf("%d %s", n, plural(n, "requisition", "requisitions")))
	}
	if m.reqLoading {
		right = m.spin.View() + " " + right
	}
	return []string{
		theme.Blur.Render(spread(left, right, m.cols()-gutter)),
		"",
		theme.Blur.Render(m.reqColumns()),
	}
}

// reqColumns is the head row, drawn by the same widths the rows are, so the two cannot drift.
func (m Model) reqColumns() string {
	cells := []string{pad("", 2)}
	for _, c := range m.reqCols() {
		// Cut a cell short of the column, exactly as the rows are: SUBMITTED is nine
		// characters in a nine-cell column, and the two heads ran together into one word.
		cells = append(cells, pad(trunc(c.head, c.cells-1), c.cells))
	}
	return theme.Header.Render(strings.Join(cells, ""))
}

// reqCol is one column: its heading, its width, and what a row puts in it.
type reqCol struct {
	head  string
	cells int
	value func(store.Requisition) string
}

// reqCols is the columns the width holds, dropped from the right as it narrows: the category,
// the two dates and the stage are what a requisition **is**, so they go last.
func (m Model) reqCols() []reqCol {
	all := []reqCol{
		{"CATEGORY", reqCatCells, func(r store.Requisition) string { return reqCategory(r.Category) }},
		{"SUBMITTED", reqDateCells, func(r store.Requisition) string { return orDash(r.Submitted) }},
		{"DEADLINE", reqDateCells, func(r store.Requisition) string { return orDash(r.Deadline) }},
		{"STAGE", reqStageCells, func(r store.Requisition) string { return orDash(r.Stage) }},
		// Headed with the word rather than a `!`: three cells and a bare exclamation mark read
		// as punctuation left behind between two columns, and the column is only ever ticked on
		// a row or two, so there is nothing else to work out what it means from. The row draws
		// it, so the tick can carry its own colour.
		{"URGENT", reqUrgentCells, nil},
		{"FOR", reqForCells, func(r store.Requisition) string { return orDash(r.For) }},
		{"DESIGNATION", reqDesigCells, func(r store.Requisition) string { return orDash(r.Designation) }},
	}
	room, out := m.cols()-gutter-2, []reqCol{}
	for _, c := range all {
		if room-c.cells < 0 {
			break
		}
		room -= c.cells
		out = append(out, c)
	}
	return out
}

// reqCategory is the category without the word every one of them ends in. "New Accessories
// Requisition" in a 24-cell column is "New Accessories Requis…", where the half that was cut is
// the half every row shares — the column is headed CATEGORY, so the word is already said.
func reqCategory(s string) string {
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "Requisition"))
}

// reqBand is the new-requisition form, under the table: the label alone until `n` opens it, then
// **one field a line** — the category, everything that category asks for, urgent, its cause while
// it is ticked, and a note — with the two buttons boxed on the right edge under them.
//
// A line each rather than a row of boxes: a replacement asks six things, and eleven boxes across
// one row left every field four cells wide and every label cut to a syllable. Down the page each
// one keeps its own words and its value has room to be read.
func (m Model) reqBand() []string {
	if !m.req.open {
		return []string{"",
			theme.Blur.Render(hinted("new requisition", m.k().NewLeave, theme.Dim, theme.HintKey))}
	}

	out := []string{"", m.reqLine("new requisition", m.reqCatBox(), reqCatField)}
	cat, chosen := m.reqCat()
	if !chosen {
		// Nothing until a category is chosen: the fields **are** the category's, so there is
		// nothing to draw and nothing to fill in.
		hint := theme.Dim.Render("  pick one")
		if m.reqLoading {
			hint = "  " + m.spin.View() + theme.Dim.Render(" reading the categories…")
		}
		return append(out, theme.Blur.Render(hint))
	}

	for i, f := range cat.Fields {
		// A field the category calls required says so, since ✓ refuses without it — and it says
		// it the way every form does, with a star.
		label := strings.ToLower(oneLine(f.Label))
		if f.Required {
			label += " *"
		}
		out = append(out, m.reqLine(label, m.reqFieldBox(i, f), 1+i))
	}
	out = append(out, m.reqLine("urgent", m.reqTick(m.req.urgent, m.reqUrgentField()),
		m.reqUrgentField()))
	if m.req.urgent {
		out = append(out, m.reqLine("why it cannot wait", m.req.urgency.View(),
			m.reqUrgencyField()))
	}
	out = append(out, m.reqLine("note", m.req.noteBox.View(), m.reqNoteField()))

	// The two buttons keep their boxes and sit **under the values**, indented past the label
	// column so they line up with what they commit: they are pressed rather than typed into,
	// which is what a box says everywhere else here, and the fields above them are lines. Pushed
	// to the right edge instead they belonged to nothing on the form.
	ok, drop := theme.FieldOk, theme.FieldDrop
	tick, cross := theme.Ok, theme.Err
	if m.req.field == m.reqOKField() {
		ok, tick = theme.FieldOkOn, theme.OnOk
	}
	if m.req.field == m.reqXField() {
		drop, cross = theme.FieldDropOn, theme.OnDrop
	}
	buttons := lipgloss.JoinHorizontal(lipgloss.Center,
		ok.Render(tick.Render("✓")), " ", drop.Render(cross.Render("✕")))
	for _, l := range strings.Split(buttons, "\n") {
		out = append(out, theme.Blur.Render(strings.Repeat(" ", reqFormLabel)+l))
	}
	return out
}

// reqLine is one field of the form: its own name in a column, then the value **in its own frame**,
// the same rounded field the time off line has — and the accent says which one has the keys, as it
// does there. The frame is left and right rules only: stacked fields with a four-sided box each
// would cost two rows apiece, which is a screen of borders for a category that asks six things.
func (m Model) reqLine(label, value string, field int) string {
	focused := m.req.field == field
	ink, box := theme.Dim, theme.ReqBox
	if focused {
		ink, box = theme.TitleFocus, theme.ReqBoxFocus
	}
	return theme.Blur.Render(ink.Render(pad(trunc(label, reqFormLabel-1), reqFormLabel)) +
		box.Render(pad(value, m.reqValueCells()-4)))
}

// reqValueCells is what a field's value gets: the width less its label column, capped — a note
// stretched across a 200-cell terminal reads as a banner, and the inputs scroll anyway.
func (m Model) reqValueCells() int {
	return min(max(m.cols()-gutter-reqFormLabel-4, 8), reqValueMax)
}

// reqSizeInputs sizes every text field to the same width, so the values line up down the page.
// Its frame and padding are four of the value's cells, and one more is the cursor an input always
// draws after its text.
//
// Called where the fields are **made** and when the terminal resizes, not from View: a textinput
// works out which slice of its value to show when it is updated, so one sized after the fact went
// on showing the twelve characters it was built for.
func (m Model) reqSizeInputs() Model {
	if _, ok := m.reqCat(); !ok {
		return m
	}
	return m.reqTextWidths(m.reqValueCells() - 5)
}

// reqCatBox is the category dropdown. Nothing in it is picked out: it is stepped with j/k or
// space like every other dropdown here.
func (m Model) reqCatBox() string {
	name := "pick a category"
	if cat, ok := m.reqCat(); ok {
		name = cat.Name
	}
	return m.reqChooserInk(reqCatField).Render(trunc(oneLine(name), m.reqValueCells()-8)) +
		theme.Dim.Render(" ▾")
}

// reqChooserInk is the ink a dropdown's or a checkbox's own value reads in: the accent while it
// holds the keys, so stepping one says so where it happened rather than only in the frame around
// it. A text field needs none of this — its cursor is already there.
func (m Model) reqChooserInk(field int) lipgloss.Style {
	if m.req.field == field {
		return theme.TitleFocus
	}
	return theme.DayLabel
}

// reqFieldBox is one of the category's own fields, drawn by its kind: a tick for a boolean, a
// dropdown for a many2one, and its own input for everything else.
func (m Model) reqFieldBox(i int, f store.ReqField) string {
	switch f.Kind {
	case "boolean":
		return m.reqTick(m.req.on[f.Name], 1+i)
	case "many2one":
		name := "…"
		switch {
		case len(f.Opts) > 0:
			name = f.Opts[min(m.req.picks[f.Name], len(f.Opts)-1)].Name
		case m.busy():
			name = "reading…"
		case f.Comodel != "":
			name = "none"
		}
		return m.reqChooserInk(1+i).Render(trunc(oneLine(name), m.reqValueCells()-8)) +
			theme.Dim.Render(" ▾")
	}
	if i < len(m.req.inputs) {
		return m.req.inputs[i].View()
	}
	return ""
}

// reqTick is a checkbox: the field's own name is already in the column beside it, so this is the
// box and its answer and nothing else. Ticked it is green — the colour "yes" is everywhere else
// here — and the word takes the accent while the keys are on it.
func (m Model) reqTick(on bool, field int) string {
	box, word := theme.Dim.Render("☐"), "no"
	if on {
		box, word = theme.Ok.Render("☑"), "yes"
	}
	return box + m.reqChooserInk(field).Render(" "+word)
}

// reqTextCount is how many text fields the line has: the category's own, plus the cause while
// urgent is ticked, plus the note.
func (m Model) reqTextCount() int {
	cat, ok := m.reqCat()
	if !ok {
		return 0
	}
	n := 1 // the note
	if m.req.urgent {
		n++
	}
	for i, f := range cat.Fields {
		switch f.Kind {
		case "boolean", "many2one":
		default:
			if i < len(m.req.inputs) {
				n++
			}
		}
	}
	return n
}

// reqTextWidths sets every text field on the line to w.
func (m Model) reqTextWidths(w int) Model {
	cat, ok := m.reqCat()
	if !ok {
		return m
	}
	// SetValue after the width, or the input keeps the window it worked out for the old one and
	// shows the tail of what is in it.
	resize := func(in textinput.Model) textinput.Model {
		in.Width = w
		in.SetValue(in.Value())
		return in
	}
	ins := make([]textinput.Model, len(m.req.inputs))
	copy(ins, m.req.inputs)
	for i, f := range cat.Fields {
		switch f.Kind {
		case "boolean", "many2one":
		default:
			if i < len(ins) {
				ins[i] = resize(ins[i])
			}
		}
	}
	m.req.inputs = ins
	m.req.urgency = resize(m.req.urgency)
	m.req.noteBox = resize(m.req.noteBox)
	return m
}

// reqLines is the body: one row per requisition, and the row under the cursor opening into the
// properties its own category asked for.
func (m Model) reqLines() ([]string, int) {
	if len(m.reqs) == 0 {
		if m.reqLoading {
			return []string{theme.Blur.Render(
				m.spin.View() + theme.Dim.Render(" reading your requisitions…"))}, -1
		}
		return []string{theme.Blur.Render(
			theme.Dim.Render("nothing filed — " + m.k().Refresh.Help().Key + " to read again"))}, -1
	}

	var out []string
	focus := -1
	held := min(m.reqHold, len(m.reqs)-1)
	for i, r := range m.reqs {
		if i == held {
			focus = len(out)
		}
		// The row keeps the window on it, but not the accent: while the form below has the
		// keyboard, a highlighted row says the keys are somewhere they are not.
		out = append(out, m.reqRow(r, i == held && !m.req.open))
		if m.reqOpen[r.ID] {
			out = append(out, m.reqDetail(r)...)
			out = append(out, "")
		}
	}
	return out, focus
}

// reqRow is one line of the table: the caret and a cell per column, the held row in the accent.
func (m Model) reqRow(r store.Requisition, focused bool) string {
	caret := theme.Dim.Render("▸ ")
	if m.reqOpen[r.ID] {
		caret = theme.Dim.Render("▾ ")
	}
	ink := theme.DayLabel
	if focused {
		ink = theme.TitleFocus
	}

	cells := []string{caret}
	for _, c := range m.reqCols() {
		if c.value == nil {
			// Urgent is a tick and nothing when it is not: a column of crosses says "no" over
			// and over, where an empty cell says the same thing and reads as empty.
			// Centred under its own head, and nothing at all when it is not urgent: a column of
			// crosses says "no" over and over, where an empty cell says the same thing.
			mark := ""
			if r.Urgent {
				mark = theme.Err.Render("✓")
			}
			cells = append(cells, pad(center(mark, c.cells-1), c.cells))
			continue
		}
		cell := ink
		if c.head == "STAGE" {
			// The stage keeps its own colour on every row, held or not: green is settled, red is
			// not happening, amber is waiting on somebody, and that is information rather than
			// focus. The border and the rest of the row's accent are what say where the cursor
			// is.
			cell = theme.StageInk(r.Stage)
		}
		cells = append(cells, cell.Render(pad(trunc(oneLine(c.value(r)), c.cells-1), c.cells)))
	}
	return row(strings.Join(cells, ""), focused)
}

// reqDetail is the open row's own block: what its category asked for, then the note. The
// properties come from the ERP with their own labels, so a category nobody has taught this app
// about still reads correctly.
func (m Model) reqDetail(r store.Requisition) []string {
	var out []string
	add := func(label, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		room := m.cols() - gutter - reqLabelCells - 6
		for i, line := range wrapCells(oneLine(value), room) {
			head := label
			if i > 0 {
				head = "" // the label heads the run; the rest is indented under it
			}
			out = append(out, theme.Blur.Render("    "+
				theme.Dim.Render(pad(head, reqLabelCells))+theme.DayLabel.Render(line)))
		}
	}
	if r.Urgent {
		add("urgency", orDash(r.Urgency))
	}
	for _, p := range r.Props {
		add(strings.ToLower(p.Label), p.Value)
	}
	add("note", r.Note)
	if len(out) == 0 {
		return []string{theme.Blur.Render(theme.Dim.Render("    nothing else on this one"))}
	}
	return out
}

// wrapCells breaks text into lines of at most n cells, on spaces where it can: a purpose or a
// note is a sentence, and cutting one to the width loses the half that says why.
func wrapCells(s string, n int) []string {
	if n < 8 {
		n = 8
	}
	var out []string
	line := ""
	for _, w := range strings.Fields(s) {
		switch {
		case line == "":
			line = w
		case lipgloss.Width(line)+1+lipgloss.Width(w) <= n:
			line += " " + w
		default:
			out = append(out, line)
			line = w
		}
		for lipgloss.Width(line) > n { // a single word longer than the column
			out = append(out, trunc(line, n))
			line = string([]rune(line)[len([]rune(trunc(line, n))):])
		}
	}
	if line != "" {
		out = append(out, line)
	}
	return out
}

// --- employees ---------------------------------------------------------------

// empHead is the directory's own header: how many rows the filter leaves out of how many
// there are, and the filter itself when one is on.
//
// No query box: the filter is a prompt opened with `/` and closed with esc, so the header is
// one line and the list gets the rest — a field that is always on screen costs three rows to
// say nothing most of the time.
func (m Model) empHead() []string {
	left := theme.Header.Render("EMPLOYEES")
	if q := strings.TrimSpace(m.empQuery.Value()); q != "" {
		// A filter that is on but not being typed still has to be visible, or the list is
		// short for a reason nothing on screen explains.
		left += theme.Dim.Render("  /") + theme.MatchText.Render(trunc(oneLine(q), 24))
	}
	right := m.empCount()
	if m.empLoading {
		// The cache is still on screen, so the loader sits beside the count rather than
		// replacing the rows, the same as the chart's month does.
		right = m.spin.View() + " " + right
	}
	return []string{
		theme.Blur.Render(spread(left, right, m.cols()-gutter)),
		"",
	}
}

// empCount is how many rows the filter leaves out of how many there are — the whole number
// alone when nothing is filtered, since "82 of 82" answers a question nobody asked.
func (m Model) empCount() string {
	shown, all := len(m.empRows()), len(m.emps)
	switch {
	case all == 0:
		return ""
	case shown == all:
		return theme.Dim.Render(fmt.Sprintf("%d %s", all, plural(all, "employee", "employees")))
	}
	return theme.Dim.Render(fmt.Sprintf("%d of %d", shown, all))
}

// empLines is the body: one row per employee, the way the task list is one row per task —
// a caret, the name, and the job title on the right edge — and the row under the cursor opens
// into everything the ERP knows about them.
func (m Model) empLines() ([]string, int) {
	rows := m.empRows()
	if len(rows) == 0 {
		switch {
		case m.empLoading:
			return []string{theme.Blur.Render(
				m.spin.View() + theme.Dim.Render(" reading the directory…"))}, -1
		case len(m.emps) == 0:
			return []string{theme.Blur.Render(
				theme.Dim.Render("no directory yet — r to read it"))}, -1
		}
		return []string{theme.Blur.Render(theme.Dim.Render("nobody matches that"))}, -1
	}

	var out []string
	focus := -1
	held := min(m.empHold, len(rows)-1)
	for i, e := range rows {
		if i == held {
			focus = len(out)
		}
		out = append(out, m.empRow(e, i == held))
		if m.empOpen[e.ID] {
			out = append(out, m.empDetailLines(e)...)
		}
		out = append(out, "") // one blank line between people, as between tasks
	}
	return out, focus
}

// empRow is one line of the list: the caret, the name in its own column, then the job title in
// a chip beside it. Two fixed columns rather than a name and a right-aligned title, so every
// chip starts on the same cell and the list reads as two columns instead of two ragged edges.
func (m Model) empRow(e store.Employee, focused bool) string {
	caret := theme.Dim.Render("▸ ")
	if m.empOpen[e.ID] {
		caret = theme.Dim.Render("▾ ")
	}

	nameW, jobW := m.empColumns()
	ink, chip := theme.DayLabel, theme.Chip
	if focused {
		// The accent marks whatever holds the keys, exactly as the task under the cursor does —
		// and the **whole** row takes it, title included: a name in the accent beside a chip
		// still in the tag's teal read as two rows overlapping.
		ink, chip = theme.TitleFocus, theme.ChipFocus
	}
	name := ink.Render(pad(trunc(oneLine(e.Name), nameW-1), nameW))
	// The chip's own frame and padding are four of its cells, so the title gets the rest.
	job := chip.Render(trunc(oneLine(orDash(e.Job)), jobW-4))
	return row(caret+name+pad(job, jobW), focused)
}

// empColumns is what the two columns get: the fixed widths where the terminal holds them, and
// the job column giving up cells first where it does not — a name is what the row is for.
func (m Model) empColumns() (name, job int) {
	room := m.cols() - gutter - 2 // the caret
	name, job = empNameCells, empJobCells
	if name+job <= room {
		return name, job
	}
	if job = room - name; job < empMinJob {
		name = max(room-empMinJob, 12)
		job = max(room-name, 0)
	}
	return name, job
}

// empDetailLines is the open row's own block: how to reach them, where they sit, and what they
// are on. Indented under the name, one fact a line, and the projects as chips.
func (m Model) empDetailLines(e store.Employee) []string {
	d, have := m.empDetail[e.ID]
	if !have {
		if m.empPulling[e.ID] {
			return []string{theme.Blur.Render("    " + m.spin.View() +
				theme.Dim.Render(" reading their details…"))}
		}
		return []string{theme.Blur.Render(theme.Dim.Render("    nothing read yet — l again"))}
	}

	var out []string
	add := func(label, value string) {
		if strings.TrimSpace(value) == "" {
			return // a field the ERP left empty is left out rather than drawn as a dash
		}
		out = append(out, theme.Blur.Render("    "+
			theme.Dim.Render(pad(label, empLabelCells))+
			theme.DayLabel.Render(trunc(oneLine(value), m.cols()-gutter-empLabelCells-6))))
	}
	add("email", d.Email)
	add("phone", d.Phone)
	add("mobile", d.Mobile)
	add("department", d.Department)
	add("team lead", d.TeamLead)
	add("project mgr", strings.Join(d.Managers, ", "))
	add("time off", d.TimeOff)
	add("stack mgr", d.StackManager)
	add("coach", d.Coach)
	add("location", d.Location)

	// The projects are chips rather than a sentence: they are a list of names, several of them
	// long enough to be mistaken for a phrase, and the frames are what say where one ends.
	for i, line := range m.empChips(d.Projects) {
		label := "projects"
		if i > 0 {
			label = "" // the label heads the run, and the rest of it is indented under it
		}
		out = append(out, theme.Blur.Render("    "+theme.Dim.Render(pad(label, empLabelCells))+line))
	}
	return out
}

// empChips wraps the project names into rows of pills that fit the width. A pill is the name
// reversed out of a filled lozenge, the shape the tab bar's active tab has.
func (m Model) empChips(names []string) []string {
	room := m.cols() - gutter - empLabelCells - 6
	if len(names) == 0 || room < 12 {
		return nil
	}
	var out []string
	line := ""
	for _, n := range names {
		chip := theme.ProjectPill.Render(trunc(oneLine(n), room-4))
		if line != "" && lipgloss.Width(line)+lipgloss.Width(chip)+1 > room {
			out = append(out, line)
			line = ""
		}
		if line != "" {
			line += " "
		}
		line += chip
	}
	if line != "" {
		out = append(out, line)
	}
	return out
}

// orDash is what a field the ERP left empty reads as on the row itself: the job title column
// still has to say something, or the line reads as a name with a missing half.
func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

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
// The months are what this screen is, and a full row of three is what makes a screenful half a
// year — two rows of three — so the panel never costs a month its column. It had a column of
// its own from about 130 cells, at the price of the third month; the holidays are still on the
// calendar as dimmed days, where a month is not recoverable from a list. panel is 0 when there
// is no column left for it.
func (m Model) timeLayout() (cols, panel int) {
	room := m.cols() - gutter
	// One hairline between each pair of months, and no gap: they are cells of one grid.
	fit := min(max((room+1)/(monthCols+1), 1), maxTimeCols)
	if len(m.timeHolidays) == 0 {
		return fit, 0
	}
	if left := room - (fit*monthCols + fit - 1) - len(colGap); left >= panelMin {
		return fit, min(left, panelMax)
	}
	return fit, 0
}

// timeTier is how much air a month cell can afford: the padding the design puts around it,
// and the blank line under every week row.
//
// Both are spent only when **two rows of months** still fit — six months at a time is what a
// year is read in here, and a screen showing one row of three is a quarter, not a half-year.
// The padding goes first and the air second, since the air is what makes a month read as a
// calendar rather than as a table.
func (m Model) timeTier() (roomy, airy bool) {
	budget := max(m.rows()-timeChrome, 1)
	// Two rows of months, the rule between them, and the two lines the window spends on its
	// own "↑ N more" / "↓ N more" — a year is four rows, so something is always hidden.
	rows := func(n int) int { return 2*n + 1 + 2 }
	switch {
	case budget >= rows(monthBare+monthWeeks-1+3):
		return true, true
	case budget >= rows(monthBare+monthWeeks-1):
		return false, true
	}
	return false, false
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
			theme.Blur.Render(hinted("new timeoff", m.k().NewLeave, theme.Dim, theme.HintKey)),
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
		// The label keeps its `n` picked out only while the line is shut. Open, the line owns
		// the keyboard and `n` types an n into the description, so the accent on it would be
		// advertising a key that cannot fire — the same rule the meal labels and the clock
		// button follow.
		parts = append(parts, hinted("new timeoff", m.k().NewLeave, theme.DayLabel, theme.DayLabel))
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

	// The two buttons frame themselves in what they do — green commits, red throws away —
	// and the one the keys are on **fills** with that colour, its mark reversed out in
	// white: these two are pressed, not typed into, so a fill says more than a frame.
	ok, drop := theme.FieldOk, theme.FieldDrop
	tick, cross := theme.Ok, theme.Err
	if m.form.field == leaveOKField {
		ok, tick = theme.FieldOkOn, theme.OnOk
	}
	if m.form.field == leaveXField {
		drop, cross = theme.FieldDropOn, theme.OnDrop
	}
	if compact {
		ok, drop = ok.Padding(0), drop.Padding(0)
	}
	parts = append(parts,
		m.leaveField(desc, leaveDescField, compact),
		ok.Render(tick.Render("✓")),
		drop.Render(cross.Render("✕")))

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

// timePending is whether anything on the calendar is still waiting on approval.
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
//
// It takes the budget and cuts itself to **whole rows of months**, which is why it is handed
// one at all (`dashLines` takes it for the same reason): the generic line window cut wherever
// the rows ran out, which left a row of months sliced through its third week above a
// "↓ 11 more" that counted lines nobody thinks in. What it hides, it hides by the row and says
// in months.
func (m Model) timeLines(budget int) ([]string, int) {
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

	// One entry per row of months, so the body can be cut by the row rather than by the line.
	rows := make([][]string, 0, (12+cols-1)/cols)
	for r := 0; r < (12+cols-1)/cols; r++ {
		blocks, tall := make([]monthPanel, 0, cols), 0
		for c := range cols {
			mon := r*cols + c
			if mon > 11 {
				blocks = append(blocks, blank) // a short last row is padded, not shifted
				continue
			}
			b := m.monthBlock(year, time.Month(mon+1), marks)
			tall = max(tall, len(b.lines))
			blocks = append(blocks, b)
		}
		// Every month in the row is padded to the tallest of them, on its own panel, and no
		// further: a month that spans five weeks beside one that spans six costs one line,
		// where padding all twelve to six would cost the year four.
		out := make([]string, 0, tall)
		for i := range tall {
			row := make([]string, 0, cols)
			for _, b := range blocks {
				if i >= len(b.lines) {
					row = append(row, b.filler)
					continue
				}
				row = append(row, b.lines[i])
			}
			out = append(out, strings.Join(row, vRule))
		}
		rows = append(rows, out)
	}

	first, last := m.timeWindow(rows, hold/cols, budget)
	var lines []string
	if first > 0 {
		lines = append(lines, monthsHidden("↑", first*cols))
	}
	for r := first; r < last; r++ {
		if r > first {
			lines = append(lines, hRule)
		}
		if r == hold/cols {
			focus = len(lines)
		}
		lines = append(lines, rows[r]...)
	}
	if last < len(rows) {
		lines = append(lines, monthsHidden("↓", 12-last*cols))
	}

	for i, l := range lines {
		lines[i] = theme.Blur.Render(l)
	}
	return lines, focus
}

// timeWindow is the rows of months that fit the budget, as a half-open range around the row
// the caret is in. It grows forward first — a year is read Jan to Dec — and the caret's own row
// goes in whether it fits or not, since a body of nothing but markers answers nothing.
func (m Model) timeWindow(rows [][]string, hold, budget int) (first, last int) {
	height := func(a, b int) int {
		n := b - a - 1 // the hairline between each pair of rows
		for _, r := range rows[a:b] {
			n += len(r)
		}
		if a > 0 {
			n++ // the "↑ N more months" line
		}
		if b < len(rows) {
			n++
		}
		return n
	}

	first, last = hold, hold+1
	for {
		grew := false
		if last < len(rows) && height(first, last+1) <= budget {
			last, grew = last+1, true
		}
		if first > 0 && height(first-1, last) <= budget {
			first, grew = first-1, true
		}
		if !grew {
			return first, last
		}
	}
}

// monthsHidden is the marker for the rows the budget could not hold, counted in **months**:
// this body is cut by the row, so a line count would be an answer to a question nobody asked.
func monthsHidden(arrow string, n int) string {
	return theme.Dim.Render(fmt.Sprintf("%s %d more %s", arrow, n, plural(n, "month", "months")))
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
	// White, not the muted head style: a month's own name is how the year is read, and twelve
	// dim names beside one accented month read as eleven things switched off.
	name, style := "  "+label, theme.Title.Foreground(theme.White)
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
	// heads — and puts a line under every week row. A terminal that cannot hold two rows of
	// months spends those on months instead; see timeTier.
	roomy, airy := m.timeTier()
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
		if r > 0 && airy {
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

// holidaySpan is a holiday's dates, as short as they can be said: one day is "Aug 5 (Wed)",
// a run inside one month collapses to "Mar 18-23", and one that crosses a month keeps both
// names.
//
// The weekday rides along on a single day only, since that is the question a holiday raises
// — which day of the week it takes — and a run of days already answers it by being a run.
func holidaySpan(h api.Holiday) string {
	from, err := time.Parse("2006-01-02", h.From)
	if err != nil {
		return h.From
	}
	to, err := time.Parse("2006-01-02", h.To)
	if err != nil || !to.After(from) {
		return from.Format("Jan 2 (Mon)")
	}
	if to.Month() == from.Month() {
		return from.Format("Jan 2") + "-" + to.Format("2")
	}
	return from.Format("Jan 2") + "-" + to.Format("Jan 2")
}

// monthMoveHelp is the footer's entry for the four motions, named after what they move —
// months, not rows or days — with the keys read off the bindings themselves so a rebind
// follows. All four step one month: h/l are aliases of j/k here, so the hint lists them in the
// order a hand finds them rather than implying two different distances.
func (m Model) monthMoveHelp() key.Binding {
	keysOf := []string{m.k().Collapse.Help().Key, m.k().Down.Help().Key,
		m.k().Up.Help().Key, m.k().Expand.Help().Key}
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

		editing := m.mode == ModeInsert && onTask
		add(theme.Blur.Render(m.tableHead()))

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
			add(theme.Blur.Render(theme.Dim.Render(tableIndent + "no entries yet")))
		}
		// The label under the table names the key that fills it, in the date column the
		// row it adds will start in. It belongs to the task the keys are actually in:
		// walking the list past a task left open advertises an `a` that would add a row
		// to whichever task the cursor is on instead, and a screen of open tasks said it
		// several times over. It steps aside for the inputs a opens too — they are the
		// answer to it, and both at once would advertise a key already pressed.
		if inTable {
			add(theme.Blur.Render(tableIndent +
				hinted("add a line", m.k().Add, theme.Dim, theme.HintKey)))
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

// projFoundModal is what `/` on the projects tab answers with: the people it found, grouped
// under the project they are on. The modal **is** the answer, exactly as the day modal is for a
// date jump — nothing in the list behind it opens or moves, so there is nothing to go back to
// and esc simply closes it.
func (m Model) projFoundModal() string {
	groups := m.projFoundRows()
	// Distinct people, not hits: somebody on three of these projects is one person, and the
	// count is there to say how many the search found.
	who := map[string]bool{}
	for _, g := range groups {
		for _, name := range g.people {
			who[name] = true
		}
	}

	head := theme.Title.Render(oneLine(m.projFind)) + theme.Dim.Render(fmt.Sprintf(
		"   %d %s found in %d %s", len(who), plural(len(who), "person", "people"),
		len(groups), plural(len(groups), "project", "projects")))

	wide := min(m.cols()-gutter-6, projMemberName+projLabelCells)
	var body []string
	for i, g := range groups {
		if i > 0 {
			body = append(body, "")
		}
		body = append(body, theme.DayLabel.Render(trunc(g.project, wide)))
		for _, name := range g.people {
			// The name in the accent: it is what was searched for, and the project above it is
			// the answer to "where".
			body = append(body, "  "+theme.MatchText.Render(trunc(name, wide-2)))
		}
	}

	// A search across the office finds more people than a modal holds, so the names scroll
	// under a pinned head, ctrl+f and ctrl+b moving them. Its own slice rather than window():
	// that one keeps a **cursor** centred, so the first press moved nothing on screen, where
	// here projFoundAt is the top line and every press scrolls by what it says it does.
	room := m.projFoundRoom()
	top := min(m.projFoundAt, max(len(body)-room, 0))
	end := min(top+room, len(body))
	view := append([]string{}, body[top:end]...)
	if top > 0 {
		view[0] = theme.Dim.Render(fmt.Sprintf("  ↑ %d more", top))
	}
	if hidden := len(body) - end; hidden > 0 {
		view[len(view)-1] = theme.Dim.Render(fmt.Sprintf("  ↓ %d more", hidden))
	}
	return theme.Modal.Render(strings.Join(append([]string{head, ""}, view...), "\n"))
}

// projFoundRoom is how many lines of names the modal shows. It is capped at **four fifths of
// the terminal**, less its own frame and pinned head: a modal as tall as the screen runs off
// the top, and the list it is about is worth still seeing behind it.
func (m Model) projFoundRoom() int {
	// Four fifths of the terminal, but never more than the rows the screen actually has left:
	// the modal is rendered in the tail, so one taller than that pushes the header off the top.
	return max(min(m.rows()*4/5, m.rows()-chromeRows)-projFoundChrome, 3)
}

// projFoundStep is what ctrl+f and ctrl+b move by: half of what the modal itself shows, not
// half the screen — a screenful is nearly the whole modal, so two presses hit the bottom and
// the scroll reads as a jump rather than a scroll.
func (m Model) projFoundStep() int {
	return max(m.projFoundRoom()/2, 1)
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
	if m.mode == ModeBook && m.book.drop {
		// One mode, two verbs: the line says which it is, and so should the mode line.
		label = "-- CANCEL MEAL --"
	}
	if m.mode != ModeConfirm && m.mode != ModeAuth && m.mode != ModeForm &&
		m.mode != ModeLeaves && m.mode != ModeBook && m.mode != ModeWFH &&
		m.mode != ModeEmpSearch && m.mode != ModeProjSearch &&
		m.mode != ModeProjJump && m.mode != ModeProjFound && m.mode != ModeReqForm {
		switch m.tab {
		case TabDash:
			label = "-- DASHBOARD --"
		case TabTime:
			label = m.timeLabel()
		case TabMeal:
			label = "-- MEAL --"
		case TabEmp:
			label = "-- EMPLOYEE --"
		case TabReq:
			label = "-- REQUISITIONS --"
		case TabProj:
			label = "-- PROJECTS --"
		}
	}

	// A modal is the exception: whatever accepts it has to be on screen, since it is
	// holding the keyboard and ? will not reach the toggle.
	help := []key.Binding{m.k().Help}
	switch {
	case m.mode == ModeConfirm:
		// Which key accepts depends on what is being confirmed.
		help = []key.Binding{m.confirmKeys(), m.k().No}
	case m.mode == ModeBook:
		// Its own keys, whichever tab is behind it: the line holds the keyboard, so the tab's
		// own hints would be advertising keys that cannot fire.
		help = m.k().help(ModeBook)
	case m.mode == ModeWFH:
		// Same reason: the request line has the keyboard, and the chart's month keys behind
		// it are letters a reason has to be able to hold.
		help = m.k().help(ModeWFH)
	case m.mode == ModeForm:
		// Its own keys, whichever tab is behind it, and only the ones the focused field
		// takes: j/k belong to a dropdown and are letters everywhere else.
		help = []key.Binding{m.k().Next, m.k().Accept, m.k().ClearField, m.k().Cancel}
		if m.leaveFieldIsDropdown() {
			help = []key.Binding{m.k().Next, m.k().Cycle, m.k().Accept, m.k().Cancel}
		}
	case !m.showHelp:
	case m.tab == TabDash:
		// It moves in screenfuls, not days, and there is no i: the query field filters
		// tasks and this is not that tab. No tab key either — the bar across the top picks the
		// letter out of every tab's own label, so naming one here spends a slot saying it twice.
		help = []key.Binding{m.k().Top, m.k().HalfDown, m.k().PrevMonth, m.clockHelp(),
			m.k().ConfirmHours, m.k().Refresh, m.k().Quit, m.k().Help}
	case m.tab == TabTime:
		// It moves in months, and the filters are the leave types' own initials, so they
		// are named by the answer rather than by the keymap. No tab key, for the same reason
		// the chart and the meal calendar have none.
		help = []key.Binding{m.k().NewLeave, m.monthMoveHelp(), m.k().Top, m.filterHelp()}
		if m.timeFilter != 0 {
			help = append(help, key.NewBinding(
				key.WithHelp(m.k().Back.Help().Key, "clear filter")))
		}
		help = append(help, m.k().Refresh, m.k().Quit, m.k().Help)
	case m.mode == ModeReqForm:
		// Its own keys, whichever tab is behind it: the line holds the keyboard, and only the
		// ones the focused field takes — j/k belong to a dropdown or a checkbox and are letters
		// everywhere else.
		help = []key.Binding{m.k().Next, m.k().Accept, m.k().ClearField, m.k().Cancel}
		if m.reqFieldIsChooser() {
			help = []key.Binding{m.k().Next, m.k().Cycle, m.k().Accept, m.k().Cancel}
		}
	case m.tab == TabReq:
		// A cursor and a row that opens, and `n` to file a new one.
		help = []key.Binding{m.k().Down, m.k().Up, m.k().Top, m.k().HalfDown,
			key.NewBinding(key.WithHelp(m.k().Expand.Help().Key, "details")),
			key.NewBinding(key.WithHelp(m.k().Collapse.Help().Key, "close")),
			key.NewBinding(key.WithHelp(m.k().NewLeave.Help().Key, "new requisition")),
			m.k().Refresh, m.k().Quit, m.k().Help}
	case m.tab == TabProj:
		// A filter, a cursor, and a row that opens into its people. Nothing here writes.
		// The footer names the action, so its hint flips with the state — the clock's own
		// rule, where the label in the head is the thing being switched.
		mine := "all projects"
		if !m.projMine {
			mine = "only mine"
		}
		help = []key.Binding{m.k().Down, m.k().Up, m.k().Top, m.k().HalfDown,
			key.NewBinding(key.WithHelp(m.k().Expand.Help().Key, "manager + members")),
			key.NewBinding(key.WithHelp(m.k().Collapse.Help().Key, "close")),
			key.NewBinding(key.WithHelp(m.k().Mine.Help().Key, mine)),
			key.NewBinding(key.WithHelp(m.k().Search.Help().Key, "search")),
			key.NewBinding(key.WithHelp(m.k().Jump.Help().Key, "find a person")),

			key.NewBinding(key.WithHelp(m.k().Back.Help().Key, "clear + collapse")),
			m.k().Refresh, m.k().Quit, m.k().Help}
	case m.tab == TabEmp:
		// A filter and a window over it, and nothing that writes: r is the only key here that
		// reaches the ERP.
		help = []key.Binding{m.k().Down, m.k().Up, m.k().Top, m.k().HalfDown,
			key.NewBinding(key.WithHelp(m.k().Expand.Help().Key, "details")),
			key.NewBinding(key.WithHelp(m.k().Collapse.Help().Key, "close")),
			key.NewBinding(key.WithHelp(m.k().Jump.Help().Key, "filter")),
			key.NewBinding(key.WithHelp(m.k().Back.Help().Key, "clear + collapse")),
			m.k().Refresh, m.k().Quit, m.k().Help}
	case m.tab == TabMeal:
		// It moves in months and nothing else: the calendar is read only for now, so there
		// is no key here that changes a booking.
		// No tab key here: the bar across the top already picks the letter out of every tab's
		// own label, so repeating `t tasks` in the footer spends a slot saying it twice.
		help = []key.Binding{m.mealMoveHelp(), m.k().PrevMonth, m.k().BookMeal, m.k().DropMeal,
			m.mealDropHelp(), m.mealRefreshHelp(), m.k().Quit, m.k().Help}
	default:
		help = append(keys.help(m.mode), m.k().Help)
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

// --- the meal calendar -------------------------------------------------------

const (
	// A bar is two cells, one per meal, with a space between them — so a day's content is
	// `▬▬ ▬▬ ▬▬` at three meals — and the gap after it is what stops a week of booked days
	// from reading as one long stripe. The design's own proportion: a weekday column is
	// eleven cells at three meals.
	mealBar    = 2
	mealGap    = 3
	mealMinGap = 1
	// A closed day holds no bars, so the weekend columns need only their date. Two of them
	// at seven cells is what buys the five weekday columns their width on an 80-cell
	// terminal.
	mealQuietCol = 7
)

// A booked meal is a thick bar and an open slot a light rule: the two are told apart by
// weight before any colour is read, which is what a colourblind reader has left. The bar is
// about double the design's own heavy rule (`━━`, an eighth of a cell) — at the two cells a
// day gives it that read as a hairline on a dark terminal, and the hue it carries is the only
// thing on the day. Two glyphs that are not this were tried: a full block (`██`) filled the
// whole cell, so a week of booked days read as a wall of colour rather than as one bar a
// meal; and the quarter block (`▂▂`) sits on the bottom of its cell, which put the bar under
// its own row instead of on the line the open slots are drawn on. This one is **centred**,
// like the rule it replaces, so booked and open days line up across a week.
const (
	mealBarOn  = "▬▬"
	mealBarOff = "──"
)

// mealCell is how wide one weekday column is: the bars, the spaces between them, and the
// gap to the next day. It shrinks by the gap alone — the bars are the data and never lose a
// cell, or a booked meal and an open one would measure the same.
func (m Model) mealCell(gap int) int {
	n := max(len(m.mealTypes), 1)
	return n*mealBar + (n - 1) + gap
}

// mealGapFor is the widest gap the terminal can hold, down to one cell. Below that the
// month is as narrow as it goes and the days run to the edge.
func (m Model) mealGapFor() int {
	// The menu column takes its cells before the gap does: the panel is decided at the
	// narrowest grid there is (mealMinGap), so this can shrink to fit around it without the
	// two of them chasing each other.
	room := m.cols()
	if p := m.mealPanelCells(); p > 0 {
		room -= p + 3
	}
	for gap := mealGap; gap > mealMinGap; gap-- {
		if gutter+5*m.mealCell(gap)+2*mealQuietCol <= room {
			return gap
		}
	}
	return mealMinGap
}

// mealHead is the month, the keys that step it, and the legend: one swatch per meal type in
// the type's own colour, straight off the answer, so an office that starts serving a fourth
// meal gets a fourth swatch with nothing to edit here.
func (m Model) mealHead() []string {
	if m.mealMonth == 0 && !m.mealLoading {
		return nil
	}
	at := m.mealViewed()
	title := theme.HintKey.Render("< ") +
		theme.Title.Render(strings.ToUpper(at.Format("January 2006")))
	if m.mealOffset < 0 {
		// > only past this month: the canteen has nothing to say about one that has not
		// happened, so a key that does nothing does not appear.
		title += theme.HintKey.Render(" >")
	}
	if m.mealLoading {
		// The month on screen is the last one answered, so the loader sits beside its title
		// rather than replacing it. The empty case has its own spinner in mealLines.
		title += " " + m.spin.View()
	}

	swatches := make([]string, 0, len(m.mealTypes))
	named := make([]string, 0, len(m.mealTypes))
	for _, t := range m.mealTypes {
		sw := theme.MealBooked(theme.MealColor(t.Name)).Render(mealBarOn)
		swatches = append(swatches, sw)
		named = append(named, sw+theme.Dim.Render(" "+strings.ToLower(firstWord(t.Name))))
	}
	count := ""
	if booked, open := m.mealDaysBooked(); open > 0 {
		count = theme.Dim.Render(fmt.Sprintf("%d of %d days", booked, open))
	}
	// The legend gives up its parts from the tail in — first the count, then the meals' names,
	// leaving the swatches, which the bars below are read by. A line wider than the terminal
	// wraps and takes a row off the month.
	line := ""
	for _, try := range []string{
		strings.Join(named, "  ") + "  " + count,
		strings.Join(named, "  "),
		strings.Join(swatches, " ") + "  " + count,
		strings.Join(swatches, " "),
		"",
	} {
		// Measured on the line as it will be drawn, not by adding up its parts: the padding,
		// the gutter and the trailing space are each a cell that arithmetic forgot.
		line = spread(theme.Blur.Render(title), try+" ", m.cols())
		if lipgloss.Width(line) <= m.cols() {
			break
		}
	}
	out := []string{line, ""}
	// The weekday row is as wide as the grid, so it goes when the grid does — the body says
	// what is wrong there rather than both of them overflowing.
	if heads := m.mealHeads(); heads != "" {
		out = append(out, heads, "")
	}
	return out
}

// mealHeads is the weekday row over the columns. The two days the canteen never serves on
// are dimmer than the five it does, which is the same thing the dates below them say.
//
// The cursor's own column takes the accent at the top of the screen: the band behind its cell
// is two cells of a slightly lighter background, which on a narrow grid is not much to find a
// column by, and the menu panel already marks the same day the same way. It outranks the quiet
// style, since the cursor can be parked on a weekend and the column it is in is the one thing
// on this row worth saying — and it goes while a line owns the keyboard (`ModeBook`), the way
// the meal labels and the clock button give up theirs.
func (m Model) mealHeads() string {
	if m.cols() < gutter+5*m.mealCell(mealMinGap)+2*mealQuietCol {
		return ""
	}
	gap, held := m.mealGapFor(), -1
	if m.mode != ModeBook {
		held = m.mealCursorColumn()
	}
	var b strings.Builder
	for i, d := range []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"} {
		style, w := theme.Dim, m.mealCell(gap)
		if i >= 5 {
			style, w = theme.MealQuietInk, mealQuietCol
		}
		if i == held {
			style = theme.TitleFocus
		}
		b.WriteString(style.Render(pad(d, w)))
	}
	return theme.Blur.Render(strings.TrimRight(b.String(), " "))
}

// mealCursorColumn is which of the seven columns the cursor is in — its weekday, Monday
// first, the same offset the grid lays its own weeks out by.
func (m Model) mealCursorColumn() int {
	at := m.mealViewed()
	day := time.Date(at.Year(), at.Month(), m.mealCursor(), 0, 0, 0, 0, time.Local)
	return (int(day.Weekday()) + 6) % 7
}

// mealLines is the month itself: one block per week — the dates, then their bars, then a
// blank line — Monday first, the way the canteen's own week runs.
//
// Every figure is derived here and none of it is stored: the day-to-booking map, the counts,
// which days are past. A month is a picture of an answer, so it is drawn from that answer.
func (m Model) mealLines() ([]string, int) {
	if m.mealMonth == 0 {
		if m.mealLoading {
			return []string{theme.Blur.Render(
				m.spin.View() + theme.Dim.Render(" reading this month's meals…"))}, -1
		}
		return []string{theme.Blur.Render(
			theme.Dim.Render("no meals yet — r to read this month"))}, -1
	}

	// Seven columns is what a week is, so below the narrowest grid there is the month cannot
	// be drawn at all — and a grid wider than the terminal wraps, which costs the screen a row
	// per week and reads as the dates printing twice.
	if m.cols() < gutter+5*m.mealCell(mealMinGap)+2*mealQuietCol {
		return []string{theme.Blur.Render(theme.Dim.Render(
			trunc("a week needs 61 cells — widen the terminal", m.cols()-gutter)))}, -1
	}

	at := m.mealViewed()
	lines, focus := m.monthGrid(at, true)

	// A booking range that runs past the end of the month brings the next month with it: the
	// days it covers are marked, and marks on a month that is not on screen say nothing.
	// Side by side where the width holds two grids, stacked underneath where it does not.
	if next, ok := m.bookSpill(); ok {
		spill, _ := m.monthGrid(next, false)
		if m.mealTwoUp() {
			lines = m.sideBySide(lines, spill)
		} else {
			lines = append(append(lines, ""), spill...)
		}
	}
	return lines, focus
}

// mealTwoUp says whether two month grids fit beside each other, with the menu column and the
// hairline between them accounted for. Below that they stack, since a grid cut in half is not
// a calendar.
func (m Model) mealTwoUp() bool { return m.twoUpWithout(m.mealPanelCells()) }

// monthCells is one month grid's own width, gutter aside: five weekday columns and the two
// narrow weekend ones. The two-up test and the zip that lays them out both measure with it, so
// they cannot disagree by the cell that wraps a row.
func (m Model) monthCells() int {
	return 5*m.mealCell(mealGap) + 2*mealQuietCol
}

// twoUpWithout says whether two grids fit beside each other once panel cells are taken out of
// the width. Split from mealTwoUp so the panel can ask the question without asking itself.
func (m Model) twoUpWithout(panel int) bool {
	return m.twoUpCells()+3+panel <= m.cols()
}

// twoUpCells is what two months side by side occupy, laid out the way sideBySide lays them.
func (m Model) twoUpCells() int { return gutter + m.monthCells() + 2 + m.monthCells() }

// sideBySide zips two grids into one column of lines, the second beginning where the first
// month's own width ends, so the weekday heads above still line up over the left one.
func (m Model) sideBySide(left, right []string) []string {
	w := gutter + m.monthCells()
	out := make([]string, max(len(left), len(right)))
	for i := range out {
		l := ""
		if i < len(left) {
			l = left[i]
		}
		out[i] = pad(l, w+2)
		if i < len(right) {
			out[i] += right[i]
		}
		out[i] = strings.TrimRight(out[i], " ")
	}
	return out
}

// bookSpill is the month a booking range runs into, when it runs into one. Only the next
// month: the ERP takes bookings 30 days out, so a range can cross one month boundary and no
// more, and the read already covers that month.
func (m Model) bookSpill() (time.Time, bool) {
	if !m.book.open {
		return time.Time{}, false
	}
	at := m.mealViewed()
	next := time.Date(at.Year(), at.Month(), 1, 0, 0, 0, 0, time.Local).AddDate(0, 1, 0)
	for _, iso := range m.bookDays() {
		d, err := time.Parse("2006-01-02", iso)
		if err != nil {
			continue
		}
		if d.Year() == next.Year() && d.Month() == next.Month() {
			return next, true
		}
	}
	return time.Time{}, false
}

// monthGrid draws one month: a week to a row of dates, its bars under it, and a blank line
// between. head puts the month's name over it, which the month in the header does not need.
func (m Model) monthGrid(at time.Time, main bool) ([]string, int) {
	gap := m.mealGapFor()
	first := time.Date(at.Year(), at.Month(), 1, 0, 0, 0, 0, time.Local)
	days := first.AddDate(0, 1, -1).Day()
	// Monday is column 0, which is what the ERP's own working week starts on.
	lead := (int(first.Weekday()) + 6) % 7
	today, cursor := time.Now().Format("2006-01-02"), m.mealCursor()
	if !main {
		// The cursor belongs to the month the keys move in; the month a range spilled into
		// carries no cursor and says its own name instead, since the header names the other.
		cursor = 0
	}

	var lines []string
	focus := -1
	if !main {
		lines = append(lines, theme.Blur.Render(theme.Title.Render(
			strings.ToUpper(at.Format("January 2006")))), "")
	}
	for start := 1 - lead; start <= days; start += 7 {
		dates, bars, served := make([]string, 0, 7), make([]string, 0, 7), false
		for i := range 7 {
			d := start + i
			w := m.mealCell(gap)
			if i >= 5 {
				w = mealQuietCol
			}
			if d < 1 || d > days {
				dates = append(dates, strings.Repeat(" ", w))
				bars = append(bars, strings.Repeat(" ", w))
				continue
			}
			day := time.Date(at.Year(), at.Month(), d, 0, 0, 0, 0, time.Local)
			iso := day.Format("2006-01-02")
			date, bar := m.mealDay(d, iso, w, gap, iso == today, iso < today,
				d == cursor && main)
			if d == cursor {
				focus = len(lines)
			}
			if !m.mealClosed[iso] {
				served = true
			}
			dates = append(dates, date)
			bars = append(bars, bar)
		}
		lines = append(lines, theme.Blur.Render(strings.TrimRight(strings.Join(dates, ""), " ")))
		// A week the canteen served nothing in — the weekend tail of a month — costs no row
		// for bars that would all be blank.
		if served {
			lines = append(lines,
				theme.Blur.Render(strings.TrimRight(strings.Join(bars, ""), " ")))
		}
		lines = append(lines, "")
	}
	return lines, focus
}

// mealDay draws one day: its date, and the bars under it.
//
// The states are the design's own vocabulary. A booked meal is a solid bar in its type's
// colour; an open slot is a thin hueless one, so the colour on a day is only ever what is
// booked. A day that cannot be acted on — past, or past its cutoff — keeps the solid bar and
// loses the hue, since it is a fact rather than a choice. A day the canteen is shut carries
// no bars at all: the empty row is what says "nothing was on offer", the same way the hour
// chart's band does.
func (m Model) mealDay(d int, iso string, w, gap int, today, past, cursor bool) (string, string) {
	// The band the design puts behind a day marks the **cursor**, since that is what x acts
	// on; today says itself with a bright underlined date. A background has to be on every
	// span or it dies at the first one that sets its own colour — the same rule the month
	// panels on the time off calendar follow.
	band := func(st lipgloss.Style) lipgloss.Style {
		if cursor {
			return st.Background(theme.MealBand)
		}
		return st
	}
	// The gap after the cell is not part of the day, so the band stops with the content.
	fill := func(n int) string {
		if n <= 0 {
			return ""
		}
		return band(lipgloss.NewStyle()).Render(strings.Repeat(" ", min(n, w-gap))) +
			strings.Repeat(" ", max(n-(w-gap), 0))
	}

	label, ink := strconv.Itoa(d), theme.MealDate
	switch {
	// A day the open booking line covers is reversed out in the accent — the same mark a date
	// jump leaves on the rows it found, and what the time off form does with its own range.
	// It outranks the rest, because it is what the keys are about.
	case m.bookCovers(iso):
		return theme.Match.Render(pad(label, 2)) + fill(w-2), m.bookBar(iso, w, gap)
	case today:
		ink = theme.MealToday
	case cursor:
		ink = theme.MealDate.Bold(true)
	case past, m.mealClosed[iso]:
		ink = theme.MealQuietInk
	}
	date := band(ink).Render(label) + fill(w-lipgloss.Width(label))

	if m.mealClosed[iso] {
		// No bars at all: the empty row is what says nothing was on offer, the same way the
		// hour chart's band does for a day nothing was expected of.
		return date, fill(w)
	}
	booked := m.mealsOn(iso)
	cells := make([]string, 0, len(m.mealTypes))
	for _, t := range m.mealTypes {
		_, ok := booked[t.ID]
		switch {
		case ok && past:
			// A day already eaten keeps its meal's hue, dimmed. is_locked_for_user is not
			// what decides this: locked means the booking can no longer be changed, which is
			// true of tomorrow's lunch after this morning's cutoff — and greying that read
			// as a meal that had already happened.
			cells = append(cells, band(theme.MealBooked(theme.MealPastColor(t.Name))).Render(mealBarOn))
		case ok:
			cells = append(cells, band(theme.MealBooked(theme.MealColor(t.Name))).Render(mealBarOn))
		default:
			cells = append(cells, band(theme.MealSlot).Render(mealBarOff))
		}
	}
	bar := strings.Join(cells, band(lipgloss.NewStyle()).Render(" "))
	if lipgloss.Width(bar) > w {
		return date, fitCell(bar, w)
	}
	return date, bar + fill(w-lipgloss.Width(bar))
}

// mealMoveHelp is the footer's entry for the four motions, named after what they move here —
// days, not months — with the keys read off the bindings so a rebind follows.
func (m Model) mealMoveHelp() key.Binding {
	keysOf := []string{m.k().Collapse.Help().Key, m.k().Down.Help().Key,
		m.k().Up.Help().Key, m.k().Expand.Help().Key}
	return key.NewBinding(key.WithHelp(strings.Join(keysOf, "/"), "day"))
}

// mealDropHelp is the footer's entry for x on this screen. The key comes off the binding, as
// everywhere else, but the description is this tab's: it cancels the day's meals, not a
// timesheet row — and it says "meal", since that is the thing being cancelled.
func (m Model) mealDropHelp() key.Binding {
	return key.NewBinding(key.WithHelp(m.k().Delete.Help().Key, "clear day"))
}

// mealRefreshHelp is r on this screen: it re-reads the month, not the task list, so the
// footer says so rather than repeating the list's own label.
func (m Model) mealRefreshHelp() key.Binding {
	return key.NewBinding(key.WithHelp(m.k().Refresh.Help().Key, "reload month"))
}

const (
	// The menu panel gets whatever is left beside the grid, between these: below
	// mealPanelMin a dish name is not a dish name any more, and above mealPanelMax the
	// column is just wide.
	mealPanelMin = 28
	mealPanelMax = 44
)

// mealPanelCells is how wide the menu column can be: what is left once the grid has its
// own cells and the rule between them, and 0 when that is not enough to read a dish on.
// The grid never gives up a cell for it — the bars are what the screen is about.
func (m Model) mealPanelCells() int {
	if m.mealMonth == 0 {
		return 0
	}

	// Measured against a grid at gap 2, not at the narrowest one: at a single cell between
	// days a week of bars runs together into one stripe, which is the thing the gap exists to
	// stop. The panel gives its cells back before that happens.
	room := m.cols() - gutter - (5*m.mealCell(mealMinGap+1) + 2*mealQuietCol) - 3
	if room < mealPanelMin {
		return 0
	}
	panel := min(room, mealPanelMax)
	// A booking range that crosses into the next month wants both grids side by side, and the
	// days it covers are worth more than what is on the menu — so the column goes when the
	// two cannot both fit, and stays when they can.
	if _, spill := m.bookSpill(); spill && m.twoUpWithout(0) && !m.twoUpWithout(panel) {
		return 0
	}
	return panel
}

// withMealPanel pins the week's menu flush to the right edge, exactly as the holiday panel
// does on the year calendar: it is a column of the screen, not of the month, so it stays put
// while the weeks scroll under it. Composed after the window for the same reason — zipped in
// first, its header would scroll away with the first week.
func (m Model) withMealPanel(body []string) []string {
	panel := m.mealMenuPanel()
	if len(panel) == 0 {
		return body
	}
	rule := theme.Sep.Render("│")
	grid := m.cols() - m.mealPanelCells() - 3

	out := make([]string, len(body))
	for i, l := range body {
		// Trimmed as well as padded: a row wider than the grid — a booking line with every
		// box open — would push the column past the terminal and wrap it.
		out[i] = trunc(pad(l, grid), grid) + " " + rule + " "
		if i < len(panel) {
			out[i] += panel[i]
		}
		out[i] = strings.TrimRight(out[i], " ")
	}
	// A menu taller than the body says so rather than stopping mid-week.
	if hidden := len(panel) - len(body); hidden > 0 && len(out) > 0 {
		out[len(out)-1] = strings.Repeat(" ", grid) + " " + rule + " " +
			theme.Dim.Render(fmt.Sprintf("… %d more", hidden+1))
	}
	return out
}

// mealMenuPanel is what the canteen is serving this week: one block per day, in the order the
// week runs, with the meals in the order the bars are drawn in.
//
// The **cursor's** block takes the accent and nothing else on the panel does — the same thing
// the band marks on the grid, so the day you are pointing at and what it is serving read
// together. Today still says `· today`, without the accent: it says itself on the grid with a
// bright underlined date, exactly as it does there. A day with no menu rows is left out: that
// is the weekend, and an empty heading says nothing the grid has not said.
func (m Model) mealMenuPanel() []string {
	w := m.mealPanelCells()
	if w == 0 {
		return nil
	}
	mon := m.mealWeekStart()
	today := time.Now().Format("2006-01-02")
	cursor := m.mealCursorDate()

	out := []string{
		theme.Header.Render(truncShaped("MENU · week of "+mon.Format("2 Jan"), w)),
		"",
	}
	for i := range 7 {
		day := mon.AddDate(0, 0, i)
		iso := day.Format("2006-01-02")
		on := m.menusOn(iso)
		if len(on) == 0 {
			continue
		}
		head, label := theme.DayLabel, day.Format("Mon 2")
		if iso == today {
			label += " · today"
		}
		switch {
		case iso == cursor:
			// The one accented thing here: the day the cursor is on, which is the day the grid
			// bands and the day x and the two lines act on.
			head = theme.HintKey
		case m.mealClosed[iso]:
			// The ERP has a menu on a day it says it is shut. Dimmed rather than dropped:
			// hiding the odd one out hides a fact.
			head = theme.Dim
		}
		out = append(out, head.Render(truncShaped(label, w)))
		for _, t := range m.mealTypes {
			mn, ok := on[t.ID]
			if !ok {
				continue
			}
			out = append(out, m.menuLine(t, mn, w, iso == cursor))
		}
	}
	if len(out) == 2 {
		return append(out, theme.Dim.Render(trunc("no menu for this week", w)))
	}
	return out
}

// menuLine is one meal on the panel: its swatch, in the type's own colour so it reads against
// that meal's bar on the grid, then the dish.
//
// One line each, cut rather than wrapped: three meals a day for five days has to fit beside a
// month, and a wrapped panel ran twice the height of the body. The swatch carries which meal
// it is — the legend above the grid already says which colour is which, so repeating the word
// here would spend a third of the column saying it twice.
func (m Model) menuLine(t api.MealType, mn api.MealMenu, w int, cursor bool) string {
	dish := strings.TrimSpace(mn.Options)
	if c := strings.TrimSpace(mn.Common); dish == "" {
		dish = c
	} else if c != "" {
		// The choice first: it is the part of a menu worth reading, and what everyone gets
		// follows it when the column has room.
		dish += " · " + c
	}
	// The cursor's whole block takes the accent, dishes included, not just its heading: what is
	// being served on the day you are pointing at is what the panel is there to answer, and a
	// day marked only by its heading still reads as four lines of the same weight as every
	// other day.
	ink := theme.MealDate
	if cursor {
		ink = theme.HintKey
	}
	return theme.MealBooked(theme.MealColor(t.Name)).Render(mealBarOn) + " " +
		ink.Render(truncShaped(dish, w-3))
}

// truncShaped cuts text the terminal may not measure the way we do, to n cells **and** n
// runes.
//
// The menus are written in Bangla, and lipgloss counts its combining marks — the matras and
// the hasanta — as zero cells: "পরোটা, অমলেট, মুগ ডাল" measures 18 while being 21 runes. A
// terminal without Bengali shaping draws one cell per rune, so a line cut to the measured
// width came out wider than the column, wrapped, and pushed the grid's own last columns onto
// the next screen row — which read as the calendar printing its dates twice.
//
// Cutting to whichever is smaller is safe under either rendering: no shaping and it is n
// cells, shaping and it is fewer.
func truncShaped(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n && lipgloss.Width(s) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	if len(r) > n-1 {
		r = r[:n-1]
	}
	for len(r) > 0 && lipgloss.Width(string(r)) > n-1 {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

// mealWeekStart is the Monday of the week the cursor is in — the panel follows the cursor,
// so walking to next week's Thursday brings next week's menu with it.
func (m Model) mealWeekStart() time.Time {
	at := m.mealViewed()
	day := time.Date(at.Year(), at.Month(), m.mealCursor(), 0, 0, 0, 0, time.Local)
	return day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
}

// menusOn is what is on offer on one day, keyed by meal type id. Derived on every render from
// the answer, like every other figure on this screen.
func (m Model) menusOn(day string) map[int]api.MealMenu {
	out := make(map[int]api.MealMenu, len(m.mealTypes))
	for _, mn := range m.mealMenus {
		if mn.Date == day {
			out[mn.TypeID] = mn
		}
	}
	return out
}

// leavesModal lists the month in view's own time off, one line a day: the date as a person
// says it, the leave type in its own colour — the same colour that day is drawn in on the
// calendar behind — and what the request said it was for.
//
// A day rather than a request: the calendar above already reads a range as the days it
// covers, so collapsing 19-21 Aug into one row would answer a different question. It destroys
// nothing, so esc closes it and there is nothing else to press.
func (m Model) leavesModal() string {
	rows := m.monthLeaves()
	month := time.Date(m.timeYearOf(), time.Month(m.timeMonth()+1), 1, 0, 0, 0, 0, time.Local)

	head := theme.Title.Render("TIME OFF "+strings.ToUpper(month.Format("Jan 2006"))) +
		theme.Dim.Render(fmt.Sprintf("   %d %s", len(rows), plural(len(rows), "day", "days")))
	if k, ok := m.timeKind(m.timeFilter); ok {
		head += theme.LeaveInk(theme.LeaveColor(k.Name)).
			Render("   " + strings.ToLower(firstWord(k.Name)) + " only")
	}
	lines := []string{head}
	if len(rows) == 0 {
		return theme.Modal.Render(head + "\n" +
			theme.Dim.Render("nothing booked off this month"))
	}

	// Both columns are sized from the rows themselves, so the reasons line up under each
	// other and a month with no half day in it does not pay for the words "afternoon".
	dateW, kindW := 0, 0
	when := make([]string, len(rows))
	for i, r := range rows {
		when[i] = leaveWhen(r)
		if w := lipgloss.Width(when[i]); w > dateW {
			dateW = w
		}
		if w := lipgloss.Width(firstWord(r.kind)); w > kindW {
			kindW = w
		}
	}
	dateW += 2 // the gap to the type, which the pad carries
	kindW += 1
	descW := m.cols() - gutter - 8 - dateW - kindW
	if descW > descCap {
		descW = descCap
	}

	// A month with more days off than this is a holiday, not a list; the head still counts
	// them all.
	const most = 14
	for i, r := range rows {
		if i == most {
			lines = append(lines, theme.Dim.Render(fmt.Sprintf("… %d more", len(rows)-most)))
			break
		}
		desc := trunc(oneLine(r.desc), descW)
		if desc == "" {
			desc = "—" // the ERP lets a request go out with no reason on it
		}
		line := theme.DayLabel.Render(pad(when[i], dateW)) +
			theme.LeaveInk(theme.LeaveColor(r.kind)).Render(pad(strings.ToLower(firstWord(r.kind)), kindW)) +
			theme.Dim.Render(" : ") + desc
		// Waiting on an approver is the other thing a day off can be, and the calendar says
		// it with an underline that a list has no room for.
		if r.state != "validate" {
			line += theme.Dim.Render("  pending")
		}
		lines = append(lines, line)
	}
	return theme.Modal.Render(strings.Join(lines, "\n"))
}

// leaveWhen is the date as a person says it — "19 Aug (Wed)" — and a half day says which half
// rather than leaving the reader to wonder why a day is worth half of one.
func leaveWhen(r leaveRow) string {
	if r.half {
		return r.date.Format("2 Jan (Mon") + ", " + halfName(r.period) + ")"
	}
	return r.date.Format("2 Jan (Mon)")
}

// bookCovers says whether the open booking line would book this day. Derived from the same
// bookDays the request is built from, so the calendar cannot mark a day the ✓ would skip.
func (m Model) bookCovers(iso string) bool {
	if !m.book.open {
		return false
	}
	for _, d := range m.bookDays() {
		if d == iso {
			return true
		}
	}
	return false
}

// bookBar is a covered day's bars: the ticked meals in their own colours, the rest as open
// slots, so the line's own ticks read on the calendar as well as on the row.
//
// On the cancel line the ticked meals are drawn in the destructive colour instead, and only
// where one is actually booked: what is about to be taken away should look like it.
func (m Model) bookBar(iso string, w, gap int) string {
	booked := m.mealsOn(iso)
	cells := make([]string, 0, len(m.mealTypes))
	past := iso < time.Now().Format("2006-01-02")
	for _, t := range m.mealTypes {
		if m.book.drop {
			// The day as it will be, not as it is: a ticked meal that is booked is **gone** —
			// drawn as the open slot it is about to become — and one left unticked keeps its
			// own colour, because it is staying. Ticking lunch takes the lunch bar off the day,
			// which is the whole question the tick is answering.
			_, held := booked[t.ID]
			switch {
			case held && m.book.on[t.ID]:
				cells = append(cells, theme.MealSlot.Render(mealBarOff))
			case held && past:
				cells = append(cells, theme.MealBooked(theme.MealPastColor(t.Name)).Render(mealBarOn))
			case held:
				cells = append(cells, theme.MealBooked(theme.MealColor(t.Name)).Render(mealBarOn))
			default:
				cells = append(cells, theme.MealSlot.Render(mealBarOff))
			}
			continue
		}
		switch {
		case m.book.on[t.ID]:
			cells = append(cells, theme.MealBooked(theme.MealColor(t.Name)).Render(mealBarOn))
		case len(booked) > 0:
			if _, held := booked[t.ID]; held {
				cells = append(cells, theme.MealQuietInk.Render(mealBarOn))
				continue
			}
			cells = append(cells, theme.MealSlot.Render(mealBarOff))
		default:
			cells = append(cells, theme.MealSlot.Render(mealBarOff))
		}
	}
	return fitCell(strings.Join(cells, " "), w)
}

// fitCell holds a day's own drawing inside its column: the two weekend columns are narrower
// than the rest, since a day the canteen is shut carries no bars — and a Saturday the ERP does
// serve on would otherwise run a cell past its neighbour and wrap the row.
func fitCell(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return trunc(s, w)
}

// halfName is which half of a day a half-day request takes.
func halfName(period string) string {
	if strings.EqualFold(period, "pm") {
		return "afternoon"
	}
	return "morning"
}

// --- the book-meal line ------------------------------------------------------

// bookBand is the row under the calendar. Closed it is the label alone with its key picked
// out; open it is the same row with the fields revealed, three lines either way, so pressing
// b moves nothing above or below it — the same shape the new-timeoff line keeps.
func (m Model) bookBand() []string {
	// A label a line, in the order they read: book, then cancel. Closed, both carry their key
	// in the accent. Open, the one that is not on the row goes **fully dim, key and all** — its
	// key does nothing while the other line holds the keyboard, and an accent on a key that
	// does nothing is the accent lying.
	label := func(name string, k key.Binding, live bool) string {
		hint := theme.HintKey
		if !live {
			hint = theme.Dim
		}
		return theme.Blur.Render(hinted(name, k, theme.Dim, hint))
	}
	book := label("book meal", m.k().BookMeal, !m.book.open)
	drop := label("cancel meal", m.k().DropMeal, !m.book.open)
	if !m.book.open {
		return []string{"", book, drop, ""}
	}

	row := make([]string, 0, 8)
	for _, l := range strings.Split(m.bookRow(m.bookCompact()), "\n") {
		row = append(row, theme.Blur.Render(l))
	}
	// The open line keeps its place in the pair: booking first, cancelling under it.
	if m.book.drop {
		return append([]string{"", book}, row...)
	}
	return append(append([]string{""}, row...), drop)
}

// bookCompact says whether the row has to give up the spaces around its boxes. Measured on
// the row as it is drawn, since that is the only thing that cannot disagree with itself.
func (m Model) bookCompact() bool {
	room := m.cols() - gutter
	if p := m.mealPanelCells(); p > 0 {
		// The menu column is beside this row too, so the boxes have to fit what it leaves.
		room -= p + 3
	}
	return lipgloss.Width(strings.Split(m.bookRow(false), "\n")[1]) > room
}

// bookRow draws the line: the scope, the two dates when it is custom, a tick per meal, then
// ✓ and ✕ — left to right, which is the only order it reads in.
func (m Model) bookRow(compact bool) string {
	sep := " "
	if compact {
		sep = ""
	}

	// The line says which of the two verbs it is. Only the label: the other's key does not
	// work while this one is open — b, l and s are the meals' own ticks here — so advertising
	// it would advertise a key that does nothing.
	verb := "book meal"
	if m.book.drop {
		verb = "cancel meal"
	}
	parts := []string{
		theme.DayLabel.Render(verb),
		m.bookField(m.bookScope(), bookScopeField, compact),
	}
	if from, to := m.bookDateFields(); from >= 0 {
		parts = append(parts,
			m.bookField(m.bookDate(0), from, compact),
			theme.Dim.Render(" → "),
			m.bookField(m.bookDate(1), to, compact))
	}
	// The two buttons carry what they do in the frame and fill when the keys are on them,
	// exactly as the time off line's do: these are pressed, not typed into.
	// ✓ is green on both lines: it means "commit this row", and the row already says which verb
	// it is. Red there made the cancel line's commit look like its discard, which are the two
	// things a reader most needs to tell apart. ✕ carries the red.
	ok, drop := theme.FieldOk, theme.FieldDrop
	tick, cross := theme.Ok, theme.Err
	if m.book.field == m.bookOKField() {
		ok, tick = theme.FieldOkOn, theme.OnOk
	}
	if m.book.field == m.bookXField() {
		drop, cross = theme.FieldDropOn, theme.OnDrop
	}
	if compact {
		ok, drop = ok.Padding(0), drop.Padding(0)
	}
	parts = append(parts, ok.Render(tick.Render("✓")), drop.Render(cross.Render("✕")))

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
	row := lipgloss.JoinHorizontal(lipgloss.Center, parts...)

	// One meal a line under the fields, indented to the boxes: three ticks in a row read as
	// three more fields to tab through, and they are not — each is its own letter.
	lines := strings.Split(row, "\n")
	indent := strings.Repeat(" ", lipgloss.Width(verb)+2)
	for _, t := range m.mealTypes {
		lines = append(lines, indent+m.bookTick(t))
	}
	return strings.Join(lines, "\n")
}

// bookField is one box on the line, framed in the accent while it holds the keys.
func (m Model) bookField(s string, field int, compact bool) string {
	box := theme.Field
	if m.book.field == field {
		box = theme.FieldFocus
	}
	if compact {
		box = box.Padding(0)
	}
	return box.Render(s)
}

// bookScope is the days dropdown. Nothing inside it is picked out: it is stepped with j/k or
// space like every other dropdown in the app, and a hint here would be advertising a letter
// that means the tasks tab everywhere else.
func (m Model) bookScope() string {
	name := []string{"today", "tomorrow", "week", "custom"}[m.book.scope]
	return theme.DayLabel.Render(name) + theme.Dim.Render(" ▾") +
		strings.Repeat(" ", max(bookScopeCells-len(name)-2, 0))
}

// bookTick is one meal's checkbox: its own initial picked out of the name, and the box filled
// when it is on. A meal that is already booked on every day the scope covers says so instead,
// since ticking it again would only be refused.
func (m Model) bookTick(t api.MealType) string {
	name := strings.ToLower(firstWord(t.Name))
	letter := key.NewBinding(key.WithKeys(name[:1]), key.WithHelp(name[:1], ""))

	// On the cancel line a meal with nothing to cancel in this scope is **disabled**: dim box,
	// dim name, and no accent on its letter, because the letter does nothing. A tick that
	// cannot act on anything would be a tick that lies.
	if m.book.drop && !m.dropAvailable(t.ID) {
		return theme.Dim.Render("☐ ") + hinted(name, letter, theme.Dim, theme.Dim) +
			theme.Dim.Render("  none")
	}

	box, ink := "☐", theme.Dim
	if m.book.on[t.ID] {
		box, ink = "☑", theme.MealBooked(theme.MealColor(t.Name))
	}
	return ink.Render(box) + " " + hinted(name, letter, theme.DayLabel, theme.HintKey)
}

// bookDate draws one of the two date fields. A field just tabbed onto shows its value
// selected — the next keystroke replaces the whole thing — which is what the accent fill
// means here, as it does on the time off line.
func (m Model) bookDate(i int) string {
	in := m.book.from
	if i == 1 {
		in = m.book.to
	}
	if m.bookDateIndex() == i && m.book.fresh[i] {
		return theme.Match.Render(pad(in.Value(), dateWidth-1))
	}
	return in.View()
}
