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
		states["confirm"] = send(t, m, runes("d"))

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

// The caret shows only while the field has focus, and never at the cost of the
// header shifting sideways.
func TestCaretFollowsFocus(t *testing.T) {
	// The app launches on the list, so there is no caret until you ask for the field.
	m := preview(t, 120)
	list := strings.Split(m.View(), "\n")[1]
	if strings.Contains(list, "❯") {
		t.Errorf("caret shown at launch, when focus is on the list:\n%s", list)
	}

	search := strings.Split(send(t, m, runes("i")).View(), "\n")[1]
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
		if !strings.Contains(lines[1], "│") {
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
