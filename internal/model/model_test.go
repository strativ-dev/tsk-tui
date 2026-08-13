package model

import (
	"errors"
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

// sendCmd is send for the cases that care what command came back.
func sendCmd(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

// quits reports whether a command resolves to tea.Quit.
func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// One pass through the whole keyboard flow: search -> list -> table -> insert -> commit.
func TestAddEntryFlow(t *testing.T) {
	m := send(t, New(), store.LoadedMsg{Tasks: []store.Task{
		{ID: 1, Title: "Add hour-log summary API", Tag: "backend", Rows: []store.Entry{
			{ID: 1, Date: "01/08/26", Desc: "old", Minutes: 60},
		}},
		{ID: 2, Title: "Sprint report export", Tag: "reports"},
	}})

	// Filter to the second task, then focus the list.
	m = send(t, m, runes("i"), runes("report"), special(tea.KeyEsc))
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

// Committing a new entry must actually post it, and a confirmation must clear the
// unsynced state. Before this was wired, ✓ only wrote to tasks.json.
func TestNewEntryLogsToERP(t *testing.T) {
	tasks := []store.Task{{ID: 1372, Key: "SE360-1372", Title: "task", Tag: "ui"}}
	m := send(t, New(), tea.WindowSizeMsg{Width: 120, Height: 30},
		store.LoadedMsg{Tasks: tasks}, store.KeyMsg{Key: "k", DB: "db"},
		runes("l"))

	m = send(t, m, runes("a"))
	m = send(t, m, special(tea.KeyTab)) // keep today
	m = send(t, m, runes("wrote the client"), special(tea.KeyTab))
	m = send(t, m, runes("2:30"), special(tea.KeyTab))
	m, cmd := sendCmd(t, m, special(tea.KeyEnter)) // ✓

	if cmd == nil {
		t.Fatal("commit returned no command — nothing was sent to the ERP")
	}
	row := m.tasks[0].Rows[0]
	if !row.Local || row.ID >= 0 {
		t.Errorf("row = %+v, want it local with a negative id until the ERP confirms", row)
	}
	if pending := store.PendingMinutesOn(m.tasks, parse.Today()); pending != 150 {
		t.Errorf("pending = %d, want 150 while the write is in flight", pending)
	}

	// The server confirms with its own line id.
	m = send(t, m, api.LoggedMsg{TaskID: 1372, LocalID: row.ID, EntryID: 141605, Minutes: 150})
	row = m.tasks[0].Rows[0]
	if row.ID != 141605 || row.Local {
		t.Errorf("row = %+v, want the ERP's id and no longer local", row)
	}
	if pending := store.PendingMinutesOn(m.tasks, parse.Today()); pending != 0 {
		t.Errorf("pending = %d, want 0 once the ERP owns the entry", pending)
	}
	if !strings.Contains(m.status, "logged") {
		t.Errorf("status = %q, want confirmation", m.status)
	}
}

// A failed write keeps the hours on screen rather than throwing them away.
func TestFailedLogKeepsRow(t *testing.T) {
	tasks := []store.Task{{ID: 1372, Key: "SE360-1372", Title: "task", Rows: []store.Entry{
		{ID: -1, Date: parse.Today(), Desc: "typed", Minutes: 150, Local: true},
	}}}
	m := send(t, New(), store.LoadedMsg{Tasks: tasks},
		api.LoggedMsg{TaskID: 1372, LocalID: -1, Err: errors.New("this would push the day past 24h")})

	if len(m.tasks[0].Rows) != 1 || !m.tasks[0].Rows[0].Local {
		t.Errorf("rows = %+v, want the entry kept and still local", m.tasks[0].Rows)
	}
	if !strings.Contains(m.status, "kept locally") {
		t.Errorf("status = %q, want it to say the hours were kept", m.status)
	}
}

// g / G / ctrl+f / ctrl+b move like vim, in the list and the table. Half-up is
// ctrl+b rather than vim's ctrl+u, which clears the query here, so half-down is
// ctrl+f to keep the pair symmetric.
func TestVimMotions(t *testing.T) {
	var many []store.Task
	for i := 1; i <= 40; i++ {
		many = append(many, store.Task{ID: i, Title: fmt.Sprintf("Task %d", i), Tag: "ui"})
	}
	// 24 rows: 8 taken by chrome, 16 left, 8 collapsed tasks fit, so half is 4.
	m := send(t, New(), tea.WindowSizeMsg{Width: 120, Height: 24}, store.LoadedMsg{Tasks: many})
	if got := m.halfPage(taskLines); got != 4 {
		t.Fatalf("halfPage = %d, want 4 for a 24-row terminal", got)
	}

	m = send(t, m, runes("G"))
	if m.cursor != 39 {
		t.Errorf("G left cursor at %d, want the last task (39)", m.cursor)
	}
	m = send(t, m, runes("g"))
	if m.cursor != 0 {
		t.Errorf("g left cursor at %d, want 0", m.cursor)
	}

	m = send(t, m, special(tea.KeyCtrlF))
	if m.cursor != 4 {
		t.Errorf("ctrl+f left cursor at %d, want 4", m.cursor)
	}
	m = send(t, m, special(tea.KeyCtrlF), special(tea.KeyCtrlB))
	if m.cursor != 4 {
		t.Errorf("ctrl+f then ctrl+b left cursor at %d, want 4", m.cursor)
	}
	// Neither key runs off an end.
	m = send(t, m, special(tea.KeyCtrlB), special(tea.KeyCtrlB))
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want it clamped at 0", m.cursor)
	}
	for i := 0; i < 20; i++ {
		m = send(t, m, special(tea.KeyCtrlF))
	}
	if m.cursor != 39 {
		t.Errorf("cursor = %d, want it clamped at 39", m.cursor)
	}

	// Same motions inside a task's rows.
	rows := make([]store.Entry, 20)
	for i := range rows {
		rows[i] = store.Entry{ID: i + 1, Date: "11/08/26", Desc: fmt.Sprintf("row %d", i), Minutes: 60}
	}
	m = send(t, New(), tea.WindowSizeMsg{Width: 120, Height: 24},
		store.LoadedMsg{Tasks: []store.Task{{ID: 1, Title: "task", Rows: rows}}}, runes("l"))

	m = send(t, m, runes("G"))
	if m.row != 19 {
		t.Errorf("G in the table left row at %d, want 19", m.row)
	}
	m = send(t, m, runes("g"))
	if m.row != 0 {
		t.Errorf("g in the table left row at %d, want 0", m.row)
	}
	m = send(t, m, special(tea.KeyCtrlF))
	if want := m.halfPage(entryLines); m.row != want {
		t.Errorf("ctrl+f in the table left row at %d, want %d", m.row, want)
	}
}

// A failed pull must be retried when the task is expanded again. It used to be
// recorded as done regardless, so one failure (a missing db, a dropped connection)
// left the task looking empty for the rest of the session.
func TestFailedPullRetries(t *testing.T) {
	tasks := []store.Task{{ID: 30857, Key: "AI-286", Title: "Momentum implement"}}
	m := send(t, New(), tea.WindowSizeMsg{Width: 120, Height: 30},
		store.LoadedMsg{Tasks: tasks}, store.KeyMsg{Key: "k"}) // no DB: pulls will fail

	m, cmd := sendCmd(t, m, runes("l"))
	if cmd == nil {
		t.Fatal("expanding did not try to read the lines")
	}
	m = send(t, m, api.EntriesMsg{TaskID: 30857, Err: api.ErrNoDB})
	if m.pulled[30857] {
		t.Error("a failed pull was recorded as done")
	}
	if m.err == nil {
		t.Error("the failure was not surfaced — it looked like a task with no hours")
	}

	// Collapse, expand: it tries again.
	m = send(t, m, runes("h"))
	m, cmd = sendCmd(t, m, runes("l"))
	if cmd == nil {
		t.Fatal("re-expanding did not retry the read")
	}

	// This time it answers, and that is recorded so it is not re-read on every key.
	m = send(t, m, api.EntriesMsg{TaskID: 30857, Rows: []store.Entry{
		{ID: 141604, Date: "11/08/26", Desc: "Fine tune results", Minutes: 240},
	}})
	if !m.pulled[30857] || len(m.tasks[0].Rows) != 1 {
		t.Errorf("pulled = %v, rows = %+v", m.pulled[30857], m.tasks[0].Rows)
	}
	m = send(t, m, runes("h"))
	if _, cmd = sendCmd(t, m, runes("l")); cmd != nil {
		t.Error("re-expanding a task already read fired another request")
	}
}

// Editing a pulled row pushes the change over RPC, and a refusal keeps the row
// marked local instead of pretending the ERP agreed.
func TestEditPushesToERP(t *testing.T) {
	tasks := []store.Task{{ID: 1372, Key: "SE360-1372", Title: "task", Rows: []store.Entry{
		{ID: 141605, Date: parse.Today(), Desc: "pulled", Minutes: 60},
	}}}
	m := send(t, New(), tea.WindowSizeMsg{Width: 120, Height: 30},
		store.LoadedMsg{Tasks: tasks}, store.KeyMsg{Key: "k", DB: "db"},
		api.DayHoursMsg{Date: parse.Today(), Minutes: 60, UserEmail: "u@e.com"},
		runes("l"))

	m = send(t, m, special(tea.KeyEnter)) // edit the focused row
	m = send(t, m, special(tea.KeyTab), special(tea.KeyTab))
	m = send(t, m, special(tea.KeyCtrlU), runes("3:00"), special(tea.KeyTab))
	m, cmd := sendCmd(t, m, special(tea.KeyEnter)) // ✓

	if cmd == nil {
		t.Fatal("edit produced no command — the ERP was never told")
	}
	if row := m.tasks[0].Rows[0]; !row.Local || row.Minutes != 180 {
		t.Errorf("row = %+v, want 180 minutes and still local until confirmed", row)
	}

	// Refused first: it leaves the row alone, so the success case below still starts
	// from a local row. (Both branches share one Rows array.)
	bad := send(t, m, api.UpdatedMsg{TaskID: 1372, EntryID: 141605, Err: errors.New("no")})
	if row := bad.tasks[0].Rows[0]; !row.Local || row.Minutes != 180 {
		t.Errorf("row = %+v, want the edit kept locally", row)
	}
	if !strings.Contains(bad.status, "kept locally") {
		t.Errorf("status = %q, want it to admit the ERP is unchanged", bad.status)
	}

	// Confirmed: no longer diverged from the ERP.
	ok := send(t, m, api.UpdatedMsg{TaskID: 1372, EntryID: 141605, Minutes: 180})
	if row := ok.tasks[0].Rows[0]; row.Local {
		t.Errorf("row = %+v, want Local cleared once the ERP agreed", row)
	}
	if !strings.Contains(ok.status, "updated") {
		t.Errorf("status = %q", ok.status)
	}
}

// The modal names the row it is about to unlink — which description, which date.
func TestDeletePromptNamesTheRow(t *testing.T) {
	tasks := []store.Task{{ID: 1, Title: "task", Rows: []store.Entry{
		{ID: 9, Date: "11/08/26", Desc: "Retry backoff", Minutes: 90},
		{ID: 8, Date: "10/08/26", Desc: "", Minutes: 60}, // Odoo hands back empty names
	}}}
	base := send(t, New(), tea.WindowSizeMsg{Width: 120, Height: 30},
		store.LoadedMsg{Tasks: tasks}, runes("l"))

	m := send(t, base, runes("d"))
	if want := `Delete this entry "Retry backoff" of 11/08/26?`; m.cPrompt != want {
		t.Errorf("prompt = %q, want %q", m.cPrompt, want)
	}
	if !strings.Contains(m.View(), "Retry backoff") {
		t.Errorf("the modal does not show the row:\n%s", m.View())
	}

	// A row with no description still reads as a sentence.
	m = send(t, base, runes("j"), runes("d"))
	if want := "Delete the entry of 10/08/26?"; m.cPrompt != want {
		t.Errorf("prompt = %q, want %q", m.cPrompt, want)
	}
}

// Deleting a pulled row unlinks it in the ERP first: the row survives a refusal.
func TestDeletePushesToERP(t *testing.T) {
	tasks := []store.Task{{ID: 1372, Key: "SE360-1372", Title: "task", Rows: []store.Entry{
		{ID: 141605, Date: parse.Today(), Desc: "pulled", Minutes: 60},
		{ID: -1, Date: parse.Today(), Desc: "typed", Minutes: 30, Local: true},
	}}}
	m := send(t, New(), tea.WindowSizeMsg{Width: 120, Height: 30},
		store.LoadedMsg{Tasks: tasks}, store.KeyMsg{Key: "k", DB: "db"},
		runes("l"))

	m, cmd := sendCmd(t, m, runes("d"))
	m, cmd = sendCmd(t, m, runes("y"))
	if cmd == nil {
		t.Fatal("delete produced no command — the ERP was never told")
	}
	if len(m.tasks[0].Rows) != 2 {
		t.Errorf("rows = %d, want the row kept until the ERP confirms", len(m.tasks[0].Rows))
	}

	// A refusal leaves it on screen, because it still exists in the ERP.
	bad := send(t, m, api.DeletedMsg{TaskID: 1372, EntryID: 141605, Err: errors.New("access denied")})
	if len(bad.tasks[0].Rows) != 2 || !strings.Contains(bad.status, "not deleted") {
		t.Errorf("rows = %d, status = %q", len(bad.tasks[0].Rows), bad.status)
	}

	// Confirmed: gone here too.
	ok := send(t, m, api.DeletedMsg{TaskID: 1372, EntryID: 141605})
	if len(ok.tasks[0].Rows) != 1 || ok.tasks[0].Rows[0].Desc != "typed" {
		t.Errorf("rows = %+v, want only the local row left", ok.tasks[0].Rows)
	}

	// A local row needs no round trip: it goes at once.
	m2 := send(t, ok, runes("j"), runes("d"))
	m2, cmd = sendCmd(t, m2, runes("y"))
	if len(m2.tasks[0].Rows) != 0 {
		t.Errorf("rows = %+v, want the local row dropped immediately", m2.tasks[0].Rows)
	}
}

// ctrl+u from the task list wipes the query and puts the cursor back in the field.
func TestClearSearchFromList(t *testing.T) {
	tasks := []store.Task{
		{ID: 1, Title: "first ui task", Tag: "ui"},
		{ID: 2, Title: "backend task", Tag: "backend"},
		{ID: 3, Title: "second ui task", Tag: "ui"},
	}
	m := send(t, New(), store.LoadedMsg{Tasks: tasks}, runes("i"), runes("ui"), special(tea.KeyEsc))
	m = send(t, m, runes("j")) // sit on the second match
	if len(m.filtered()) != 2 || m.cursor != 1 {
		t.Fatalf("setup: %d filtered, cursor %d", len(m.filtered()), m.cursor)
	}

	m = send(t, m, special(tea.KeyCtrlU))
	if m.search.Value() != "" {
		t.Errorf("query = %q, want it cleared", m.search.Value())
	}
	if m.mode != ModeSearch || !m.search.Focused() {
		t.Errorf("mode = %v, focused = %v, want ModeSearch and focused", m.mode, m.search.Focused())
	}
	if len(m.filtered()) != 3 {
		t.Errorf("filtered = %d tasks, want all 3 back", len(m.filtered()))
	}
	if m.cursor < 0 || m.cursor >= 3 {
		t.Errorf("cursor = %d, out of range for the widened list", m.cursor)
	}
}

// ctrl+u works from wherever focus is, not just the list: inside a task's table it
// also collapses the task, and in the field it clears without losing focus.
func TestClearSearchFromEverywhere(t *testing.T) {
	tasks := []store.Task{
		{ID: 1, Title: "first ui task", Tag: "ui", Rows: []store.Entry{
			{ID: 1, Date: "11/08/26", Desc: "row", Minutes: 60},
		}},
		{ID: 2, Title: "backend task", Tag: "backend"},
	}

	// From the table.
	m := send(t, New(), store.LoadedMsg{Tasks: tasks}, runes("i"), runes("ui"),
		special(tea.KeyEsc), runes("l"))
	if m.mode != ModeTable {
		t.Fatalf("setup: mode = %v, want ModeTable", m.mode)
	}
	m = send(t, m, special(tea.KeyCtrlU))
	if m.mode != ModeSearch || !m.search.Focused() {
		t.Errorf("mode = %v, focused = %v, want ModeSearch and focused", m.mode, m.search.Focused())
	}
	if m.search.Value() != "" || len(m.filtered()) != 2 {
		t.Errorf("query = %q, filtered = %d, want cleared and all tasks back",
			m.search.Value(), len(m.filtered()))
	}
	if m.expanded[1] {
		t.Error("task 1 is still expanded")
	}

	// From the field itself: clears, keeps focus.
	m = send(t, m, runes("backend"), special(tea.KeyCtrlU))
	if m.search.Value() != "" {
		t.Errorf("query = %q, want cleared", m.search.Value())
	}
	if m.mode != ModeSearch || !m.search.Focused() {
		t.Errorf("mode = %v, focused = %v, want to stay in the field", m.mode, m.search.Focused())
	}
}

// q asks before quitting: enter confirms, n dismisses. ctrl+c still leaves at once.
func TestQuitConfirm(t *testing.T) {
	m := send(t, New(), store.LoadedMsg{Tasks: []store.Task{{ID: 1, Title: "task"}}})

	m, cmd := sendCmd(t, m, runes("q"))
	if quits(cmd) {
		t.Fatal("q quit immediately, want a confirmation first")
	}
	if m.mode != ModeConfirm {
		t.Fatalf("mode = %v, want ModeConfirm", m.mode)
	}
	if !strings.Contains(m.View(), "Quit") {
		t.Errorf("no quit prompt on screen:\n%s", m.View())
	}

	// n dismisses, back to the list, still running.
	back, cmd := sendCmd(t, m, runes("n"))
	if quits(cmd) {
		t.Error("n quit the app")
	}
	if back.mode != ModeList {
		t.Errorf("mode after n = %v, want ModeList", back.mode)
	}

	// y confirms.
	if _, cmd := sendCmd(t, m, runes("y")); !quits(cmd) {
		t.Error("y on the quit prompt did not quit")
	}
	// enter must not: quitting should not be an enter-key reflex.
	still, cmd := sendCmd(t, m, special(tea.KeyEnter))
	if quits(cmd) {
		t.Error("enter quit the app, want y only")
	}
	if still.mode != ModeConfirm {
		t.Errorf("mode after enter = %v, want the prompt still open", still.mode)
	}
	if !strings.Contains(m.View(), "y / n") {
		t.Errorf("quit prompt should advertise y, not enter:\n%s", m.View())
	}
	// ctrl+c bypasses the prompt entirely.
	if _, cmd := sendCmd(t, back, special(tea.KeyCtrlC)); !quits(cmd) {
		t.Error("ctrl+c did not quit immediately")
	}
}

// An Odoo pull must not delete the rows typed in the app — it used to assign
// msg.Rows straight over them.
func TestOdooPullKeepsLocalRows(t *testing.T) {
	tasks := []store.Task{{ID: 7, Title: "Task", Tag: "ui", Rows: []store.Entry{
		{ID: 90211, Date: "12/08/26", Desc: "pulled", Minutes: 60},
	}}}
	m := send(t, New(), tea.WindowSizeMsg{Width: 120, Height: 30}, store.LoadedMsg{Tasks: tasks},
		runes("l"))

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

// jumpModel is three tasks whose rows straddle two months, all of them collapsed.
func jumpModel(t *testing.T) Model {
	t.Helper()
	tasks := []store.Task{
		{ID: 1, Key: "AI-1", Title: "one", Rows: []store.Entry{
			{ID: 11, Date: "13/08/26", Desc: "later that month", Minutes: 60},
			{ID: 12, Date: "12/07/26", Desc: "july twelfth", Minutes: 90},
		}},
		{ID: 2, Key: "AI-2", Title: "two", Rows: []store.Entry{
			{ID: 21, Date: "12/08/26", Desc: "standup", Minutes: 30},
		}},
		{ID: 3, Key: "AI-3", Title: "three", Rows: []store.Entry{
			{ID: 31, Date: "12/08/26", Desc: "code review", Minutes: 45},
			{ID: 32, Date: "12/07/26", Desc: "july again", Minutes: 15},
		}},
	}
	return send(t, New(), tea.WindowSizeMsg{Width: 120, Height: 40},
		store.LoadedMsg{Tasks: tasks})
}

// lineWith is the rendered line carrying needle.
func lineWith(t *testing.T, view, needle string) string {
	t.Helper()
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(l, needle) {
			return l
		}
	}
	t.Fatalf("%q is not on screen:\n%s", needle, view)
	return ""
}

// From the list, /12 is the 12th of the current month and answers with the day modal:
// every entry logged on it, in whatever task, without opening any of them.
func TestJumpFromListListsTheDay(t *testing.T) {
	if parse.Today() != "13/08/26" {
		t.Skip("the day-only grammar resolves against today; fixture assumes 13/08/26")
	}
	m := send(t, jumpModel(t), runes("/"), runes("12"), special(tea.KeyEnter))

	if m.jumpDate != "12/08/26" {
		t.Fatalf("jumpDate = %q, want 12/08/26 — the month and year come from today", m.jumpDate)
	}
	if m.mode != ModeDay {
		t.Fatalf("mode = %v, want the day modal", m.mode)
	}
	// Nothing in the list moved or opened: the modal is the answer.
	if len(m.expanded) != 0 {
		t.Errorf("expanded = %v, want the tasks left shut", m.expanded)
	}

	rows, total := m.dayRows()
	if len(rows) != 2 || total != 75 {
		t.Fatalf("dayRows = %+v, total = %d, want 2 rows and 75 minutes", rows, total)
	}
	view := m.View()
	for _, want := range []string{"12/08/26", "1h15m in 2 entries", "AI-2", "standup",
		"0:30", "AI-3", "code review", "0:45", "esc close"} {
		if !strings.Contains(view, want) {
			t.Errorf("the day modal is missing %q:\n%s", want, view)
		}
	}
	// Only that date: the July rows and the 13th stay out of it.
	for _, unwanted := range []string{"july twelfth", "july again", "later that month"} {
		if strings.Contains(view, unwanted) {
			t.Errorf("the modal lists %q, which is not on 12/08/26:\n%s", unwanted, view)
		}
	}

	// esc closes it, with no confirmation in the way.
	closed := send(t, m, special(tea.KeyEsc))
	if closed.mode != ModeList {
		t.Errorf("mode = %v after esc, want the list", closed.mode)
	}
	if strings.Contains(closed.View(), "esc close") {
		t.Errorf("the modal is still up after esc:\n%s", closed.View())
	}
}

// Inside a task, /12 is the 12th of whatever month it turns up in. The rows in front
// of you span months, so resolving the day against today would skip most of them.
func TestJumpInsideATaskIgnoresMonthAndYear(t *testing.T) {
	if parse.Today() != "13/08/26" {
		t.Skip("fixture assumes today is 13/08/26, in a month with no 12th logged here")
	}
	// Task one holds 13/08/26 and 12/07/26 — nothing on the 12th of this month.
	m := send(t, jumpModel(t), runes("l"), runes("/"), runes("12"), special(tea.KeyEnter))

	if m.mode != ModeTable {
		t.Fatalf("mode = %v, want the rows", m.mode)
	}
	if m.jumpQuery != "12" || m.jumpDate != "" {
		t.Errorf("jumpQuery = %q, jumpDate = %q, want the raw query and no resolved date",
			m.jumpQuery, m.jumpDate)
	}
	if m.row != 1 {
		t.Errorf("row = %d, want 1 — the July 12th row, found by day alone", m.row)
	}
	if !strings.Contains(m.status, "12 — 1 entry in this task") {
		t.Errorf("status = %q", m.status)
	}
	// Every 12th is marked, in any month and any year; the 13th is not.
	for _, e := range []store.Entry{{Date: "12/07/26"}, {Date: "12/08/26"}, {Date: "12/01/25"}} {
		if !m.onJumpDate(e) {
			t.Errorf("%s is not marked by /12", e.Date)
		}
	}
	if m.onJumpDate(store.Entry{Date: "13/08/26"}) {
		t.Error("13/08/26 is marked by /12")
	}
}

// Inside a task's rows the key is a move, not a report: the rows are already on
// screen, so the cursor walks to the date and no modal covers them.
func TestJumpInsideATaskMovesTheCursor(t *testing.T) {
	m := send(t, jumpModel(t), runes("l")) // open task one, cursor on its first row
	m = send(t, m, runes("/"), runes("12/07/26"), special(tea.KeyEnter))

	if m.mode != ModeTable {
		t.Fatalf("mode = %v, want the rows, not a modal", m.mode)
	}
	if m.row != 1 {
		t.Errorf("row = %d, want 1 — the july row of this task", m.row)
	}
	if !strings.Contains(m.status, "12/07/26 — 1 entry in this task") {
		t.Errorf("status = %q", m.status)
	}
	if v := m.View(); strings.Contains(v, "esc close") {
		t.Errorf("a modal opened over the rows:\n%s", v)
	}

	// The marks still stand, so the row it walked to reads as the match.
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)
	const accent = "255;192;0"
	if l := lineWith(t, m.View(), "july twelfth"); !strings.Contains(l, accent) {
		t.Errorf("the row jumped to is not highlighted:\n%s", l)
	}
	if l := lineWith(t, m.View(), "later that month"); strings.Contains(l, accent) {
		t.Errorf("a row on another date is highlighted:\n%s", l)
	}
}

// A date nothing was logged on says so instead of looking like a broken key.
func TestJumpWithNoMatch(t *testing.T) {
	m := send(t, jumpModel(t), runes("/"), runes("1/1/20"), special(tea.KeyEnter))
	if !strings.Contains(m.status, "01/01/20 — no entries") {
		t.Errorf("status = %q", m.status)
	}
	if len(m.expanded) != 0 {
		t.Errorf("expanded = %v, want nothing opened", m.expanded)
	}
	if !strings.Contains(m.View(), "nothing logged on this date") {
		t.Errorf("the modal does not say the day is empty:\n%s", m.View())
	}
}

// An impossible date keeps the prompt open rather than silently doing nothing.
func TestJumpRejectsImpossibleDate(t *testing.T) {
	m := send(t, jumpModel(t), runes("/"), runes("31/02"), special(tea.KeyEnter))
	if m.mode != ModeJump || m.err == nil {
		t.Errorf("mode = %v, err = %v, want the prompt still open with an error", m.mode, m.err)
	}
	if m.jumpDate != "" {
		t.Errorf("jumpDate = %q, want it untouched", m.jumpDate)
	}
}

// A jump has to read the tasks it has never opened, or their hours quietly miss the
// search — Odoo holds the lines, and a task with none on disk was never pulled.
func TestJumpReadsUnopenedTasks(t *testing.T) {
	tasks := []store.Task{
		{ID: 1, Key: "AI-1", Title: "read already", Rows: []store.Entry{
			{ID: 11, Date: "12/07/26", Desc: "july", Minutes: 60},
		}},
		{ID: 2, Key: "AI-2", Title: "never opened"}, // no rows: Odoo may have some
	}
	m := send(t, New(), tea.WindowSizeMsg{Width: 120, Height: 40},
		store.LoadedMsg{Tasks: tasks}, store.KeyMsg{Key: "k", DB: "db"})

	m = send(t, m, runes("/"), runes("12/07/26"))
	m, cmd := sendCmd(t, m, special(tea.KeyEnter))
	if cmd == nil {
		t.Fatal("the jump asked Odoo for nothing — task 2 was never read")
	}
	if !m.pulling[2] {
		t.Errorf("pulling = %v, want the unopened task in flight", m.pulling)
	}
	if m.pulling[1] {
		t.Error("the task that already has rows was re-read; a pull returns its whole history")
	}
	if !strings.Contains(m.status, "reading 1 more") {
		t.Errorf("status = %q, want it to say a task is still being read", m.status)
	}

	// The answer joins the open modal, since the modal is built from the tasks on
	// every render rather than captured when it opened.
	m = send(t, m, api.EntriesMsg{TaskID: 2, Rows: []store.Entry{
		{ID: 21, Date: "12/07/26", Desc: "arrived late", Minutes: 30},
	}})
	rows, total := m.dayRows()
	if len(rows) != 2 || total != 90 {
		t.Errorf("dayRows = %+v, total = %d, want the pulled row to have joined", rows, total)
	}
	if !strings.Contains(m.View(), "arrived late") {
		t.Errorf("the pulled row is not in the modal:\n%s", m.View())
	}
	if !strings.Contains(m.status, "12/07/26 — 2 entries") {
		t.Errorf("status = %q, want the count to have caught up", m.status)
	}
}

// The marks are a standing filter, so there has to be a way to drop them: an empty
// prompt, or the ctrl+u that already means "back to a clean search".
func TestJumpCleared(t *testing.T) {
	// esc out of the modal first: it is up when the jump lands, and it only closes.
	m := send(t, jumpModel(t), runes("/"), runes("12/07/26"), special(tea.KeyEnter),
		special(tea.KeyEsc))

	empty := send(t, m, runes("/"), special(tea.KeyEnter))
	if empty.jumpDate != "" || empty.status != "" {
		t.Errorf("jumpDate = %q, status = %q after an empty prompt", empty.jumpDate, empty.status)
	}
	if empty.mode == ModeDay {
		t.Error("an empty prompt opened the modal instead of clearing the marks")
	}

	cleared := send(t, m, special(tea.KeyCtrlU))
	if cleared.jumpDate != "" {
		t.Errorf("jumpDate = %q after ctrl+u", cleared.jumpDate)
	}
}
