package model

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tasnimAlam/tsk/internal/parse"
	"github.com/tasnimAlam/tsk/internal/store"
)

func send(t *testing.T, m Model, msgs ...tea.Msg) Model {
	t.Helper()
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

func runes(s string) tea.Msg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func special(k tea.KeyType) tea.Msg { return tea.KeyMsg{Type: k} }

// One pass through the whole keyboard flow: search -> list -> table -> insert -> commit.
func TestAddEntryFlow(t *testing.T) {
	m := send(t, New(), store.LoadedMsg{Tasks: []store.Task{
		{ID: 1, Title: "Add hour-log summary API", Tag: "backend", Rows: []store.Entry{
			{ID: 1, Date: "01/08/26", Desc: "old", Minutes: 60},
		}},
		{ID: 2, Title: "Sprint report export", Tag: "reports"},
	}})

	// Filter to the second task, then focus the list.
	m = send(t, m, runes("report"), special(tea.KeyEsc))
	if m.mode != ModeList {
		t.Fatalf("mode = %v, want ModeList", m.mode)
	}
	if got := m.filtered(); len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("filtered() = %v, want just task 2", got)
	}

	// ctrl+u in search clears the query and collapses everything.
	m = send(t, m, runes("i"), special(tea.KeyCtrlU), special(tea.KeyEsc))
	if len(m.filtered()) != 2 {
		t.Fatalf("filtered() after clear = %d tasks, want 2", len(m.filtered()))
	}

	// Expand task 1 and add an entry: date "9", desc, hours "2:30".
	m = send(t, m, runes("l"))
	if m.mode != ModeTable {
		t.Fatalf("mode = %v, want ModeTable", m.mode)
	}
	m = send(t, m, runes("a"))
	m = send(t, m, runes("9"), special(tea.KeyTab))
	m = send(t, m, runes("parser"), special(tea.KeyTab))
	m = send(t, m, runes("2:30"), special(tea.KeyTab))
	m = send(t, m, special(tea.KeyEnter)) // on ✓

	if m.mode != ModeTable {
		t.Fatalf("mode after commit = %v, want ModeTable", m.mode)
	}
	rows := m.tasks[0].Rows
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (new entry prepended)", len(rows))
	}
	want, _ := parse.Date("9", parse.Today())
	if rows[0].Date != want || rows[0].Desc != "parser" || rows[0].Minutes != 150 {
		t.Fatalf("rows[0] = %+v, want {%s parser 150}", rows[0], want)
	}
	if rows[0].ID == rows[1].ID {
		t.Fatalf("new entry reused id %d", rows[0].ID)
	}

	// Delete it again: d, then y.
	m = send(t, m, runes("d"))
	if m.mode != ModeConfirm {
		t.Fatalf("mode = %v, want ModeConfirm", m.mode)
	}
	m = send(t, m, runes("y"))
	if len(m.tasks[0].Rows) != 1 || m.tasks[0].Rows[0].Desc != "old" {
		t.Fatalf("rows after delete = %+v, want only the old entry", m.tasks[0].Rows)
	}
}

func TestJumpToDay(t *testing.T) {
	rows := []store.Entry{
		{Date: "12/08/26"},
		{Date: "09/08/26"},
		{Date: "09/07/26"},
	}
	cases := []struct {
		q    string
		want int
	}{
		{"12", 0},
		{"9", 1},
		{"9/7", 2},
		{"31", -1},
		{"", -1},
	}
	for _, c := range cases {
		if got := findDay(rows, c.q); got != c.want {
			t.Errorf("findDay(%q) = %d, want %d", c.q, got, c.want)
		}
	}
}
