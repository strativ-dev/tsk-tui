package model

import (
	"fmt"
	"strings"

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
	descCap    = 48
)

// View stacks a fixed header, a windowed list, and a fixed footer. The header and
// footer are laid out first and the list gets whatever rows are left, so the
// search field can never be pushed off the top of the screen.
func (m Model) View() string {
	head := append(strings.Split(m.header(), "\n"),
		theme.Sep.Render(strings.Repeat("─", m.cols())), "")

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
		tail = append(tail, theme.Blur.Render(theme.Dim.Render(
			trunc(oneLine(m.status), m.cols()-gutter))))
	}
	tail = append(tail, strings.Split(m.footer(), "\n")...)

	// The list takes the rows left between header and footer, and is padded out to
	// them, which pins the status line and the key hints to the bottom of the screen.
	budget := m.rows() - len(head) - len(tail)
	body, focus := m.listLines()
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

func (m Model) footer() string {
	help := keys.help(m.mode)
	if m.mode == ModeConfirm {
		// Which key accepts depends on what is being confirmed.
		help = []key.Binding{m.confirmKeys(), keys.No}
	}

	parts := []string{theme.Mode.Render(modeLabel(m.mode))}
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
