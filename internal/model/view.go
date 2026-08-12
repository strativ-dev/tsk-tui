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
	// gutter is the indent every line shares, one cell of which the focus border
	// or the blur padding occupies.
	gutter = 2
	// Used until the first WindowSizeMsg lands.
	fallbackWidth  = 100
	fallbackHeight = 24

	// Table columns, shared by the head, the rows and the insert line.
	tableIndent = "    "
	colGap      = "   "
	dateWidth   = 8 // dd/mm/yy
	hoursWidth  = 5 // h:mm, or hh:mm
	descCap     = 48
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
		hint := "enter / n"
		if m.cKind == confirmQuit {
			hint = "y / n"
		}
		tail = append(tail, strings.Split(
			theme.Modal.Render(m.cPrompt+"  "+theme.Dim.Render(hint)), "\n")...)
	case ModeAuth:
		tail = append(tail, strings.Split(m.authModal(), "\n")...)
	}
	if m.err != nil {
		tail = append(tail, theme.Blur.Render(theme.Err.Render("! "+m.err.Error())))
	}
	if m.status != "" {
		tail = append(tail, theme.Blur.Render(theme.Dim.Render(m.status)))
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
	box := theme.SearchBox.Width(boxWidth).Render(field)

	return lipgloss.JoinHorizontal(lipgloss.Center, caret, box, "  ", prog)
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
		(m.mode == ModeConfirm && m.prev == ModeList)

	for i, t := range tasks {
		onTask := i == m.cursor
		focused := onTask && listFocus
		add(row(m.taskLine(t), focused))
		if focused {
			mark()
		}

		if !m.expanded[t.ID] {
			add("") // a collapsed list breathes, as in the design
			continue
		}

		add(theme.Blur.Render(m.tableHead()))
		inTable := onTask && !listFocus &&
			(m.mode == ModeTable || m.mode == ModeJump || m.mode == ModeConfirm)
		for j, e := range t.Rows {
			add(row(m.entryLine(e), inTable && j == m.row))
			if inTable && j == m.row {
				mark()
			}
		}
		if len(t.Rows) == 0 && m.mode != ModeInsert {
			add(theme.Blur.Render(theme.Dim.Render("    no entries yet — a to add one")))
		}
		if m.mode == ModeInsert && onTask {
			add(row(m.insertLine(), true))
			mark()
		}
		if m.mode == ModeJump && onTask {
			add(theme.Blur.Render("    jump to day " + m.jump.View()))
			mark()
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
func (m Model) taskLine(t store.Task) string {
	caret := "▸"
	if m.expanded[t.ID] {
		caret = "▾"
	}

	left := theme.Dim.Render(caret) + "  " + theme.Title.Render(trunc(t.Title, m.cols()/2))
	if t.Tag != "" {
		left += "  " + theme.Chip.Render(strings.ToUpper(t.Tag))
	}

	count := fmt.Sprintf("%d entries", len(t.Rows))
	if len(t.Rows) == 1 {
		count = "1 entry"
	}
	right := theme.Dim.Render(count) + "   " + theme.Total.Render(fmt.Sprintf("%7s",
		parse.FormatTotal(store.Total(t.Rows))))

	return spread(left, right, m.cols()-gutter)
}

func (m Model) tableHead() string {
	return theme.Header.Render(cells(
		pad("DATE", dateWidth),
		pad("DESCRIPTION", m.descWidth()),
		padLeft("HOURS", hoursWidth)))
}

func (m Model) entryLine(e store.Entry) string {
	desc := m.descWidth()
	// Pad first, style second: fmt counts the bytes of an ANSI escape as width.
	return cells(
		theme.Dim.Render(pad(e.Date, dateWidth)),
		pad(trunc(e.Desc, desc), desc),
		theme.Total.Render(padLeft(parse.FormatHM(e.Minutes), hoursWidth)))
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

// pad and padLeft measure with lipgloss, so styled text lands in the right column.
func pad(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

func padLeft(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return strings.Repeat(" ", d) + s
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

func trunc(s string, n int) string {
	if n <= 1 || lipgloss.Width(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}
