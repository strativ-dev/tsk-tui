package model

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tasnimAlam/tsk/internal/api"
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

// A missing key opens the prompt; a 401 reopens it and forgets the key.
func TestKeyPrompt(t *testing.T) {
	m := send(t, New(), store.KeyMsg{})
	if m.mode != ModeAuth {
		t.Fatalf("mode = %v, want ModeAuth when no key is stored", m.mode)
	}

	// esc works offline instead.
	m = send(t, m, special(tea.KeyEsc))
	if m.mode != ModeSearch || m.key != "" {
		t.Fatalf("esc left mode %v key %q, want ModeSearch and no key", m.mode, m.key)
	}

	// Typing a key stores it in the model and clears the input.
	m = send(t, m, runes("K"))
	if m.mode != ModeAuth {
		// K only fires from the list, so get there first.
		m = send(t, m, special(tea.KeyEsc), runes("K"))
	}
	m = send(t, m, runes("odookey1234"), special(tea.KeyEnter))
	if m.key != "odookey1234" {
		t.Fatalf("key = %q, want the typed key", m.key)
	}
	if m.auth.Value() != "" {
		t.Errorf("auth input still holds the key: %q", m.auth.Value())
	}
	if v := m.View(); strings.Contains(v, "odookey1234") {
		t.Error("View() rendered the API key")
	}

	// A rejected key drops it from memory and asks again.
	m = send(t, m, api.TasksMsg{Err: api.ErrUnauthorized})
	if m.mode != ModeAuth || m.key != "" {
		t.Fatalf("after 401: mode %v key %q, want ModeAuth and no key", m.mode, m.key)
	}
}

// A fetch failure that is not a 401 keeps whatever is on disk.
func TestOfflineKeepsLocalTasks(t *testing.T) {
	local := []store.Task{{ID: 1, Title: "local", Rows: []store.Entry{{ID: 1, Minutes: 60}}}}
	m := send(t, New(), store.LoadedMsg{Tasks: local}, api.TasksMsg{Err: errors.New("cannot reach host")})

	if m.mode == ModeAuth {
		t.Error("a network error must not ask for a new key")
	}
	if len(m.tasks) != 1 || m.tasks[0].Title != "local" {
		t.Errorf("tasks = %+v, want the disk copy kept", m.tasks)
	}
}

// An Odoo pull must not delete the rows typed in the app — it used to assign
// msg.Rows straight over them.
func TestOdooPullKeepsLocalRows(t *testing.T) {
	tasks := []store.Task{{ID: 7, Title: "Task", Tag: "ui", Rows: []store.Entry{
		{ID: 90211, Date: "12/08/26", Desc: "pulled", Minutes: 60},
	}}}
	m := send(t, New(), tea.WindowSizeMsg{Width: 120, Height: 30}, store.LoadedMsg{Tasks: tasks},
		special(tea.KeyEsc), runes("l"))

	// Add an entry, then let a pull land afterwards.
	m = send(t, m, runes("a"))
	m = send(t, m, runes("9"), special(tea.KeyTab))
	m = send(t, m, runes("typed here"), special(tea.KeyTab))
	m = send(t, m, runes("2:00"), special(tea.KeyTab), special(tea.KeyEnter))
	if len(m.tasks[0].Rows) != 2 {
		t.Fatalf("after add: %d rows, want 2", len(m.tasks[0].Rows))
	}

	m = send(t, m, api.EntriesMsg{TaskID: 7, Rows: []store.Entry{
		{ID: 90211, Date: "12/08/26", Desc: "pulled", Minutes: 60},
	}})

	if len(m.tasks[0].Rows) != 2 {
		t.Fatalf("after pull: %d rows, want the pulled one and the typed one:\n%+v",
			len(m.tasks[0].Rows), m.tasks[0].Rows)
	}
	var typed bool
	for _, r := range m.tasks[0].Rows {
		if r.Desc == "typed here" {
			typed = true
		}
	}
	if !typed {
		t.Errorf("the typed entry was dropped by the pull: %+v", m.tasks[0].Rows)
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
