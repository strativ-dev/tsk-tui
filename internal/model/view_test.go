package model

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/tasnimAlam/tsk/internal/api"
	"github.com/tasnimAlam/tsk/internal/parse"
	"github.com/tasnimAlam/tsk/internal/store"
)

// preview builds a model with content in it, at a known width.
func preview(t *testing.T, width int) Model {
	t.Helper()
	today := parse.Today()
	tasks := []store.Task{
		{ID: 1, Title: "Ship odds ingestion retry queue", Tag: "backend", Rows: []store.Entry{
			{ID: 3, Date: today, Desc: "Retry backoff + dead letter", Minutes: 150},
			{ID: 2, Date: today, Desc: "Queue consumer", Minutes: 135},
			{ID: 1, Date: "11/08/26", Desc: "Spike", Minutes: 120},
		}},
		{ID: 2, Title: "Parlay builder keyboard flow", Tag: "ui", Rows: []store.Entry{
			{ID: 1, Date: today, Desc: "Focus ring", Minutes: 405},
		}},
		{ID: 3, Title: "Sportsbook review CMS migration", Tag: "content"},
	}
	return send(t, New(),
		tea.WindowSizeMsg{Width: width, Height: 30},
		store.LoadedMsg{Tasks: tasks},
		api.DayHoursMsg{Date: today, Minutes: 105})
}

// Nothing may exceed the terminal width: one long line wraps and every row below
// it slides, which is how a TUI layout starts looking broken.
func TestViewFitsWidth(t *testing.T) {
	for _, width := range []int{80, 100, 120, 200} {
		m := preview(t, width)
		states := map[string]Model{"list": m} // launch state
		states["search"] = send(t, m, runes("i"))
		m = send(t, m, runes("l"))
		states["table"] = m
		states["insert"] = send(t, m, runes("a"))
		states["jump"] = send(t, m, runes("/"))
		states["confirm"] = send(t, m, runes("x"))

		for name, s := range states {
			for i, line := range strings.Split(s.View(), "\n") {
				if got := lipgloss.Width(line); got > width {
					t.Errorf("width %d, %s state, line %d is %d cells:\n%s", width, name, i, got, line)
				}
			}
		}
	}
}

// `a` types the new entry at the top of the table, where it will land; an edit
// stands in place of the row being edited, never as an extra row at the bottom.
func TestInsertRowPlacement(t *testing.T) {
	tasks := []store.Task{{ID: 1, Key: "AI-286", Title: "task", Rows: []store.Entry{
		{ID: 141604, Date: "11/08/26", Desc: "newest", Minutes: 240},
		{ID: 141446, Date: "10/08/26", Desc: "middle", Minutes: 450},
		{ID: 141348, Date: "07/08/26", Desc: "oldest", Minutes: 450},
	}}}
	base := send(t, New(), tea.WindowSizeMsg{Width: 120, Height: 30},
		store.LoadedMsg{Tasks: tasks}, runes("l"))

	// tableRows returns the description-bearing lines in render order.
	tableRows := func(m Model) []string {
		var out []string
		for _, l := range strings.Split(m.View(), "\n") {
			switch {
			case strings.Contains(l, "✓"): // the insert row
				out = append(out, "INPUTS")
			case strings.Contains(l, "newest"), strings.Contains(l, "middle"),
				strings.Contains(l, "oldest"):
				out = append(out, strings.Fields(l)[1]) // date, description, hours
			}
		}
		return out
	}

	got := tableRows(send(t, base, runes("a")))
	want := []string{"INPUTS", "newest", "middle", "oldest"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("a: rows = %v, want the inputs first", got)
	}

	// Edit the middle row: inputs take its place, count unchanged.
	got = tableRows(send(t, base, runes("j"), special(tea.KeyEnter)))
	want = []string{"newest", "INPUTS", "oldest"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("edit: rows = %v, want the inputs in place of the edited row", got)
	}
}

// A normalized value has to fit the input it was written into. Typing a bare day
// and tabbing away fills in the month and year, but the date column was one cell
// too narrow for its own input — the field scrolled and 12/08/26 read as 2/08/26.
func TestInsertFieldsShowNormalizedValues(t *testing.T) {
	tasks := []store.Task{{ID: 1, Key: "AI-286", Title: "task"}}
	m := send(t, New(), tea.WindowSizeMsg{Width: 120, Height: 30},
		store.LoadedMsg{Tasks: tasks}, runes("l"), runes("a"))

	// Day only, normalized against today: month and year are kept.
	date, err := parse.Date("12", parse.Today())
	if err != nil {
		t.Fatal(err)
	}
	m = send(t, m, runes("12"), special(tea.KeyTab))
	if !strings.Contains(m.View(), date) {
		t.Errorf("date field does not show %s in full:\n%s", date, m.View())
	}

	// Same for two-digit hours, the widest thing the hours column holds.
	m = send(t, m, runes("desc"), special(tea.KeyTab), runes("10:30"), special(tea.KeyTab))
	if !strings.Contains(m.View(), "10:30") {
		t.Errorf("hours field does not show 10:30 in full:\n%s", m.View())
	}
}

// Every line the view emits has to be one line. Odoo handed back VD-427 with a
// newline inside its name, which rendered the task as two lines: the list grew past
// the terminal, the terminal scrolled, and the search box went off the top.
func TestErpTextCannotGrowTheView(t *testing.T) {
	tasks := []store.Task{
		{ID: 1, Key: "VD-427", Title: "\nProject Pulse: Meeting & Growth Framework FE",
			Tag: "VALUE-DRIVEN ENGAGEMENT,\nINTERNAL MEETINGS & TASKS", Rows: []store.Entry{
				{ID: 1, Date: "11/08/26", Desc: "stand-up\nnotes\twritten up", Minutes: 30},
			}},
		{ID: 2, Key: "VD-433", Title: "FE team all hands", Tag: "meetings"},
	}
	for _, width := range []int{80, 120, 200} {
		m := send(t, New(), tea.WindowSizeMsg{Width: width, Height: 24},
			store.LoadedMsg{Tasks: tasks})
		m.status = "synced 22 tasks"

		for name, s := range map[string]Model{
			"list":   m,
			"table":  send(t, m, runes("l")),
			"delete": send(t, m, runes("l"), runes("x")),
		} {
			lines := strings.Split(s.View(), "\n")
			if len(lines) > 24 {
				t.Errorf("width %d, %s: view is %d lines, terminal is 24", width, name, len(lines))
			}
			for i, l := range lines {
				if got := lipgloss.Width(l); got > width {
					t.Errorf("width %d, %s: line %d is %d cells:\n%s", width, name, i, got, l)
				}
			}
		}
		// The title still reads, just flattened onto its own line.
		if v := m.View(); !strings.Contains(v, "VD-427 Project Pulse") {
			t.Errorf("width %d: the flattened title is missing:\n%s", width, v)
		}
	}
}

// A query longer than the box must not wrap inside it: the box grows a line, the
// list is shoved down and the view no longer fits the terminal.
func TestLongQueryKeepsTheHeaderThreeLines(t *testing.T) {
	long := "alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu nu xi"
	for _, width := range []int{80, 100, 120, 200} {
		m := send(t, preview(t, width), runes("i"), runes(long))
		if h := len(strings.Split(m.header(), "\n")); h != 3 {
			t.Errorf("width %d: header is %d lines with a long query, want 3:\n%s",
				width, h, m.header())
		}
	}
}

// Focus is accent-colored in both halves of the screen: the search frame while the
// query field has the keys, the task title while the list does.
func TestFocusIsAccentColored(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	accent := "255;192;0" // theme.Accent as truecolor
	m := preview(t, 120)  // launches on the list, so the frame is quiet

	find := func(view, needle string) string {
		for _, l := range strings.Split(view, "\n") {
			if strings.Contains(l, needle) {
				return l
			}
		}
		t.Fatalf("%q missing from view:\n%s", needle, view)
		return ""
	}

	// The cursor starts on the first task; the second is not under it.
	if l := find(m.View(), "Ship odds"); !strings.Contains(l, accent) {
		t.Errorf("focused task title is not accent-colored:\n%s", l)
	}
	if l := find(m.View(), "Parlay builder"); strings.Contains(l, accent) {
		t.Errorf("unfocused task line is accent-colored:\n%s", l)
	}

	// Expanding keeps it: the rows have focus now, but the title still says which
	// task you are inside.
	if l := find(send(t, m, runes("l")).View(), "Ship odds"); !strings.Contains(l, accent) {
		t.Errorf("expanded task title lost the accent:\n%s", l)
	}

	// The frame is the box's top border, found by content: the tab bar sits above it.
	top := find(m.View(), "╭")
	if strings.Contains(top, accent) {
		t.Errorf("search frame is accent-colored while the list has focus:\n%s", top)
	}
	if top = find(send(t, m, runes("i")).View(), "╭"); !strings.Contains(top, accent) {
		t.Errorf("search frame is not accent-colored while the field has focus:\n%s", top)
	}
}

// The hours column has to start at one offset on all three kinds of line. The head
// and the rows were right-aligned while the insert row's input left-aligns itself,
// which left h:mm two cells adrift of HOURS.
// Plain profile on purpose: the styled case is TestTableColumnsAlignWithStyling's
// job, and with colors on the cursor splits the placeholder mid-string.
func TestHoursColumnStartsAtOneOffset(t *testing.T) {
	tasks := []store.Task{{ID: 1, Title: "task", Tag: "ui", Rows: []store.Entry{
		{ID: 1, Date: "11/08/26", Desc: "short", Minutes: 450},
	}}}
	m := send(t, New(), tea.WindowSizeMsg{Width: 100, Height: 30},
		store.LoadedMsg{Tasks: tasks}, runes("l"))

	// column is where needle starts, in cells rather than bytes.
	column := func(view, needle string) int {
		for _, l := range strings.Split(view, "\n") {
			if i := strings.Index(l, needle); i >= 0 {
				return lipgloss.Width(l[:i])
			}
		}
		t.Fatalf("%q missing from view:\n%s", needle, view)
		return -1
	}

	head := column(m.View(), "HOURS")
	if row := column(m.View(), "7:30"); row != head {
		t.Errorf("row hours start at %d, HOURS at %d", row, head)
	}
	if ins := column(send(t, m, runes("a")).View(), "h:mm"); ins != head {
		t.Errorf("insert hours start at %d, HOURS at %d", ins, head)
	}
}

// A long title runs to the entry count, not to a flat half of the screen. It was
// truncated at cols()/2, which threw away most of a wide terminal.
func TestTitleFillsTheLine(t *testing.T) {
	// 110 cells: past the old cols()/2 cap at this width, inside what the fixed
	// parts of the line actually leave over.
	long := "Rework the sportsbook odds ingestion retry queue, its dead letter handling " +
		"and the replay tooling"
	tasks := []store.Task{{ID: 1, Title: long, Tag: "backend", Rows: []store.Entry{
		{ID: 1, Date: "11/08/26", Desc: "spike", Minutes: 120},
	}}}
	m := send(t, New(), tea.WindowSizeMsg{Width: 160, Height: 30}, store.LoadedMsg{Tasks: tasks})

	var line string
	for _, l := range strings.Split(m.View(), "\n") {
		if strings.Contains(l, "Rework") {
			line = l
		}
	}
	if line == "" {
		t.Fatalf("task line missing from view:\n%s", m.View())
	}
	if strings.Contains(line, "…") {
		t.Errorf("title truncated at 160 cells, where it fits whole:\n%s", line)
	}
	// Still one line, chip and total still on it.
	if !strings.Contains(line, "BACKEND") || !strings.Contains(line, "2h") {
		t.Errorf("title crowded out the chip or the total:\n%s", line)
	}
}

// The caret shows only while the field has focus, and never at the cost of the
// header shifting sideways.
func TestCaretFollowsFocus(t *testing.T) {
	// The app launches on the list, so there is no caret until you ask for the field.
	m := preview(t, 120)
	// The field's own line, located by content rather than index — the tab bar is above.
	fieldLine := func(v string) string { return lineWith(t, v, "search title or tag") }
	list := fieldLine(m.View())
	if strings.Contains(list, "❯") {
		t.Errorf("caret shown at launch, when focus is on the list:\n%s", list)
	}

	search := fieldLine(send(t, m, runes("i")).View())
	if !strings.Contains(search, "❯") {
		t.Errorf("no caret after i focused the field:\n%s", search)
	}
	if a, b := lipgloss.Width(search), lipgloss.Width(list); a != b {
		t.Errorf("header is %d cells focused, %d unfocused — it shifts", a, b)
	}
	// Compare cells, not bytes: ❯ is three bytes wide and one cell.
	boxColumn := func(line string) int { return lipgloss.Width(line[:strings.Index(line, "│")]) }
	if a, b := boxColumn(search), boxColumn(list); a != b {
		t.Errorf("box starts at column %d focused, %d unfocused", a, b)
	}
}

// Logging time must move the bar: it read only the ERP's day total, so a fresh
// entry changed nothing on screen.
func TestProgressCountsNewEntries(t *testing.T) {
	today := parse.Today()
	tasks := []store.Task{{ID: 7, Title: "Task", Tag: "ui", Rows: []store.Entry{
		{ID: 90211, Date: today, Desc: "pulled", Minutes: 60},
	}}}
	m := send(t, New(), tea.WindowSizeMsg{Width: 120, Height: 30},
		store.LoadedMsg{Tasks: tasks},
		api.DayHoursMsg{Date: today, Minutes: 60}, // the ERP knows about the pulled hour
		runes("l"))

	if got, pending := m.todayMinutes(); got != 60 || pending != 0 {
		t.Fatalf("before adding: %d minutes, %d pending, want 60 and 0", got, pending)
	}

	m = send(t, m, runes("a"))
	m = send(t, m, special(tea.KeyTab)) // keep today's date
	m = send(t, m, runes("typed here"), special(tea.KeyTab))
	m = send(t, m, runes("2:30"), special(tea.KeyTab), special(tea.KeyEnter))

	got, pending := m.todayMinutes()
	if got != 210 || pending != 150 {
		t.Errorf("after adding 2:30: %d minutes, %d pending, want 210 and 150", got, pending)
	}
	if view := m.View(); !strings.Contains(view, "3h30m") || !strings.Contains(view, "unsynced") {
		t.Errorf("header does not show the new total:\n%s", view)
	}

	// An edit of a pulled line is not added on top: the ERP total already counts it.
	m = send(t, m, runes("j"), special(tea.KeyEnter))
	m = send(t, m, special(tea.KeyTab), special(tea.KeyTab))
	m = send(t, m, special(tea.KeyCtrlU), runes("4:00"), special(tea.KeyTab), special(tea.KeyEnter))
	if got, _ := m.todayMinutes(); got != 210 {
		t.Errorf("after editing a pulled row: %d minutes, want 210 (no double count)", got)
	}
}

// Table columns must line up with styling switched on: fmt's %-10s counts the
// bytes of an ANSI escape as printable width, which silently skews the columns.
func TestTableColumnsAlignWithStyling(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	// The insert row is deliberately longer (it carries the ✓ / ✕ buttons), so it
	// is checked by TestViewFitsWidth instead.
	m := send(t, preview(t, 120), runes("l"))

	var table []string
	for _, line := range strings.Split(m.View(), "\n") {
		if strings.Contains(line, "DATE") || strings.Contains(line, "/08/26") {
			table = append(table, line)
		}
	}
	if len(table) < 3 {
		t.Fatalf("expected a head and rows, got %d lines:\n%s", len(table), m.View())
	}

	// Every row starts its hours column at the same offset as the head's HOURS.
	want := lipgloss.Width(table[0])
	for i, line := range table[1:] {
		if got := lipgloss.Width(line); got != want {
			t.Errorf("table line %d is %d cells, head is %d — columns do not align:\n%s\n%s",
				i+1, got, want, table[0], line)
		}
	}
}

// The header must survive a list longer than the screen — it used to scroll off
// the top, taking the search field with it.
func TestSearchFieldAlwaysVisible(t *testing.T) {
	var many []store.Task
	for i := 1; i <= 40; i++ {
		many = append(many, store.Task{ID: i, Title: fmt.Sprintf("Task %d", i), Tag: "backend"})
	}
	m := send(t, New(), tea.WindowSizeMsg{Width: 120, Height: 24}, store.LoadedMsg{Tasks: many})

	for _, state := range []Model{m, send(t, m, special(tea.KeyEsc), runes("G"), runes("l"))} {
		view := state.View()
		lines := strings.Split(view, "\n")
		if n := len(lines); n > 24 {
			t.Errorf("view is %d lines tall, terminal is 24", n)
		}
		// The box sits in the first few rows — under the tab bar, above the rule.
		if !strings.Contains(strings.Join(lines[:4], "\n"), "│") {
			t.Errorf("search box missing from the top of the view:\n%s", view)
		}
		if !strings.Contains(view, "SEARCH") && !strings.Contains(view, "LIST") && !strings.Contains(view, "TABLE") {
			t.Errorf("footer missing from the view:\n%s", view)
		}
	}
}

// Moving the cursor down a long list must scroll it into view.
func TestCursorStaysInWindow(t *testing.T) {
	var many []store.Task
	for i := 1; i <= 40; i++ {
		many = append(many, store.Task{ID: i, Title: fmt.Sprintf("Task %d", i), Tag: "ui"})
	}
	m := send(t, New(), tea.WindowSizeMsg{Width: 120, Height: 24}, store.LoadedMsg{Tasks: many})
	for i := 0; i < 30; i++ {
		m = send(t, m, runes("j"))
	}

	if want := "Task 31"; !strings.Contains(m.View(), want) {
		t.Errorf("cursor on %s but it is not in the window:\n%s", want, m.View())
	}
}

// The focus marker must not shift a row sideways, or the list jitters as the
// cursor moves.
func TestFocusKeepsColumns(t *testing.T) {
	m := preview(t, 120)
	lines := strings.Split(m.View(), "\n")

	var focused, unfocused string
	for _, l := range lines {
		switch {
		case strings.Contains(l, "Ship odds"):
			focused = l // the cursor starts on the first task
		case strings.Contains(l, "Parlay builder"):
			unfocused = l
		}
	}
	if focused == "" || unfocused == "" {
		t.Fatalf("task lines missing from view:\n%s", m.View())
	}
	if a, b := indexOfCaret(focused), indexOfCaret(unfocused); a != b {
		t.Errorf("caret at column %d when focused, %d when not", a, b)
	}
}

func indexOfCaret(line string) int {
	if i := strings.IndexAny(line, "▸▾"); i >= 0 {
		return lipgloss.Width(line[:i])
	}
	return -1
}

// An expanded table says which key fills it: the label sits over the column heads with its
// own key in the accent, and it steps aside for the inputs that key opens — advertising `a`
// above the row `a` just drew would name a key already pressed.
func TestAddALineLabel(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	const accent = "255;192;0" // #FFC000, the a

	shut := preview(t, 120)
	if strings.Contains(shut.View(), "add a line") {
		t.Error("a collapsed task advertises a key that would not add to it")
	}

	open := send(t, shut, runes("l"))
	lines := strings.Split(open.View(), "\n")
	label, head := -1, -1
	for i, l := range lines {
		if strings.Contains(l, "dd a line") { // hinted() splits the a into its own span
			label = i
		}
		if head < 0 && strings.Contains(l, "DESCRIPTION") {
			head = i
		}
	}
	if label < 0 || head < 0 {
		t.Fatalf("label at %d, head at %d:\n%s", label, head, open.View())
	}
	last := -1
	for i, l := range lines {
		if strings.Contains(l, "Spike") { // the oldest row of the task preview() expands
			last = i
		}
	}
	if last < 0 || label < last {
		t.Errorf("the label is at %d and the last row at %d, want it under the table", label, last)
	}
	// Flush with the DATE column: a label hanging a column to the left of the table it is
	// about reads as belonging to the task line above it.
	if a, b := strings.Index(plain(lines[label]), "add a line"),
		strings.Index(plain(lines[head]), "DATE"); a != b {
		t.Errorf("the label starts at cell %d and DATE at %d:\n%s\n%s", a, b,
			plain(lines[label]), plain(lines[head]))
	}
	if !strings.Contains(lines[label], accent) {
		t.Errorf("the label does not pick out its key:\n%q", lines[label])
	}

	// Pressed, the label goes and the inputs stand in the place it pointed at.
	adding := send(t, open, runes("a"))
	if strings.Contains(adding.View(), "add a line") {
		t.Errorf("the label survived the key it names:\n%s", adding.View())
	}
	// An edit is the same: `a` cannot fire while the fields hold the keyboard.
	if v := send(t, open, special(tea.KeyEnter)).View(); strings.Contains(v, "add a line") {
		t.Errorf("the label stayed up over an edit:\n%s", v)
	}

	// A task left open with the keys back on the list belongs to nobody: `a` there would add
	// a row to whichever task the cursor walked onto, so no open table advertises it.
	walking := open
	walking.mode = ModeList
	if v := walking.View(); strings.Contains(v, "add a line") {
		t.Errorf("an open table advertises `a` while the list has the keys:\n%s", v)
	}
}

// esc collapses the task, as h does: the rows are the thing esc undoes here, and a table left
// open behind the list's own cursor advertised an `a` that would not have added to it.
func TestEscCollapsesTheTask(t *testing.T) {
	m := send(t, preview(t, 120), runes("l"))
	if m.mode != ModeTable {
		t.Fatalf("mode = %v, want the rows", m.mode)
	}
	id := m.filtered()[m.cursor].ID
	if !m.expanded[id] {
		t.Fatal("l did not expand the task")
	}

	shut := send(t, m, special(tea.KeyEsc))
	if shut.mode != ModeList {
		t.Errorf("mode = %v, want the list", shut.mode)
	}
	if shut.expanded[id] {
		t.Error("esc left the task open")
	}
	if strings.Contains(shut.View(), "DESCRIPTION") {
		t.Error("the table is still on screen")
	}
}
