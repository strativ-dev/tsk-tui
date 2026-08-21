package model

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/tasnimAlam/tsk/internal/api"
	"github.com/tasnimAlam/tsk/internal/parse"
)

// The refusal itself opens the line: the ERP names what it wants, and typing the request is
// the only thing left to do about it.
const wfhRefusal = "odoo: You have exceeded the number of days available for WFH. " +
	"Please submit a WFH request."

func refusedCheckIn(t *testing.T) Model {
	t.Helper()
	m := clockModel(t, false)
	return send(t, m, api.AttendanceMsg{Toggled: true, Want: true, Err: errors.New(wfhRefusal)})
}

func TestWFHRefusalOpensTheLine(t *testing.T) {
	m := refusedCheckIn(t)
	if m.mode != ModeWFH || !m.wfh.open {
		t.Fatalf("mode = %v, open = %v", m.mode, m.wfh.open)
	}
	// Today in both fields: the day whose check in was refused is the day it is for.
	if m.wfh.from.Value() != parse.Today() || m.wfh.to.Value() != parse.Today() {
		t.Errorf("dates = %q → %q", m.wfh.from.Value(), m.wfh.to.Value())
	}
	// The ERP's own words stay on screen — the line answers that sentence.
	if !strings.Contains(m.status, "WFH") {
		t.Errorf("status = %q", m.status)
	}

	v := m.View()
	for _, want := range []string{"wfh request", parse.Today(), "reason", "✓", "✕"} {
		if !strings.Contains(v, want) {
			t.Errorf("the line is missing %q:\n%s", want, v)
		}
	}

	// Another refusal of a different kind leaves the line out of it.
	other := send(t, clockModel(t, false),
		api.AttendanceMsg{Toggled: true, Want: true, Err: errors.New("odoo: access denied")})
	if other.mode == ModeWFH {
		t.Error("an unrelated refusal opened the WFH line")
	}
}

// It sits under the check in button, on the same right edge: the button is what refused the
// check in, and the line is about that refusal.
func TestWFHLineSitsUnderTheButton(t *testing.T) {
	m := send(t, refusedCheckIn(t), tea.WindowSizeMsg{Width: 100, Height: 30})
	lines := strings.Split(m.View(), "\n")
	button, row := -1, -1
	for i, l := range lines {
		if strings.Contains(l, "heck in") { // hinted() splits the c off into its own span
			button = i
		}
		if strings.Contains(l, "wfh request") {
			row = i
		}
	}
	if button < 0 || row < 0 {
		t.Fatalf("button at %d, row at %d:\n%s", button, row, m.View())
	}
	if row <= button {
		t.Errorf("the row is above the button: %d vs %d", row, button)
	}
	// Right-aligned, like the button's own box: two cells of margin and no more.
	for _, i := range []int{row - 1, row, row + 1} {
		if got := lipgloss.Width(strings.TrimRight(lines[i], " ")); got < 98 {
			t.Errorf("line %d stops at %d cells, not the right edge: %q", i, got, lines[i])
		}
	}
}

// The button gives up its accent while the line is open: it cannot fire, since the line has the
// keyboard, and an accent on a dead key is the accent lying.
func TestWFHLineDisablesTheClockButton(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	const accent = "255;192;0" // #FFC000, the c
	open := refusedCheckIn(t)
	line := lineWith(t, open.View(), "heck in")
	if strings.Contains(line, accent) {
		t.Errorf("the button still advertises its key:\n%q", line)
	}
	// Closed again, the key works and says so.
	shut := send(t, open, special(tea.KeyEsc))
	if !strings.Contains(lineWith(t, shut.View(), "heck in"), accent) {
		t.Error("the button stayed dim after the line closed")
	}
}

// The line owns the keyboard: the reason has to be able to hold the letters the chart's own
// keys are, and the tab bar's.
func TestWFHLineOwnsTheKeyboard(t *testing.T) {
	m := refusedCheckIn(t)
	m = send(t, m, special(tea.KeyTab), special(tea.KeyTab)) // dates → reason
	if m.wfh.field != wfhReasonField {
		t.Fatalf("field = %d, want the reason", m.wfh.field)
	}
	m = send(t, m, runes("at the dentist"))
	if got := m.wfh.reason.Value(); got != "at the dentist" {
		t.Errorf("reason = %q", got)
	}
	if m.tab != TabDash {
		t.Errorf("typing changed the tab to %v", m.tab)
	}
}

// esc and ✕ take the line away with nothing filed: the days and the reason are two keystrokes
// and a sentence, so a modal in front of them would cost more than it saves.
func TestWFHLineClosesWithoutAsking(t *testing.T) {
	for _, c := range []struct {
		what string
		keys []tea.Msg
	}{
		{"esc", []tea.Msg{special(tea.KeyEsc)}},
		{"✕", []tea.Msg{special(tea.KeyTab), special(tea.KeyTab), special(tea.KeyTab),
			special(tea.KeyTab), special(tea.KeyEnter)}},
	} {
		m := send(t, refusedCheckIn(t), c.keys...)
		if m.mode == ModeWFH || m.wfh.open {
			t.Errorf("%s left the line open: mode = %v", c.what, m.mode)
		}
		if m.mode == ModeConfirm {
			t.Errorf("%s asked first", c.what)
		}
	}
}

// ✓ files it. No modal — it asks a manager for days, which destroys nothing — and a reason is
// required before the round trip, since the ERP requires it too.
func TestWFHTickFilesTheRequest(t *testing.T) {
	m := refusedCheckIn(t)
	for range 3 {
		m = send(t, m, special(tea.KeyTab)) // → ✓
	}
	if m.wfh.field != wfhOKField {
		t.Fatalf("field = %d, want ✓", m.wfh.field)
	}

	blank, cmd := sendCmd(t, m, special(tea.KeyEnter))
	if cmd != nil || blank.wfhFiling {
		t.Error("a request with no reason was sent")
	}
	if !strings.Contains(blank.status, "why") {
		t.Errorf("status = %q", blank.status)
	}

	typed := send(t, m, special(tea.KeyShiftTab), runes("at the dentist"), special(tea.KeyTab))
	filed, cmd := sendCmd(t, typed, special(tea.KeyEnter))
	if cmd == nil || !filed.wfhFiling {
		t.Fatal("the request was not sent")
	}
	if filed.mode != ModeWFH {
		t.Errorf("mode = %v, want the line still up while the ERP answers", filed.mode)
	}
	if !filed.busy() {
		t.Error("the spinner will not animate while the request is out")
	}
}

// What the request was for is a check in, so the answer tries it again — and a refusal keeps
// the line exactly as typed, since there is nothing else to fix it with.
func TestWFHAnswerRetriesTheCheckIn(t *testing.T) {
	m := send(t, refusedCheckIn(t), special(tea.KeyTab), special(tea.KeyTab),
		runes("at the dentist"))
	m.wfhFiling = true

	ok, cmd := sendCmd(t, m, api.WFHRequestedMsg{ID: 1384})
	if ok.wfhFiling || ok.wfh.open {
		t.Errorf("the line survived the answer: %+v", ok.wfh)
	}
	if cmd == nil || !ok.clocking {
		t.Error("the check in was not tried again")
	}
	if !strings.Contains(ok.status, "checking in") {
		t.Errorf("status = %q", ok.status)
	}

	// Refused, with nothing filed: the line stays as typed.
	kept, cmd := sendCmd(t, m, api.WFHRequestedMsg{Err: errors.New("no manager set")})
	if !kept.wfh.open || kept.mode != ModeWFH || cmd != nil {
		t.Errorf("a refusal closed the line: mode = %v", kept.mode)
	}
	if kept.wfh.reason.Value() != "at the dentist" {
		t.Errorf("the reason was lost: %q", kept.wfh.reason.Value())
	}

	// A create that landed with a submit that did not closes it: re-filing would ask HR for
	// the same days twice.
	draft, _ := sendCmd(t, m, api.WFHRequestedMsg{ID: 1384,
		Err: errors.New("filed as a draft, not submitted: no manager set")})
	if draft.wfh.open || !draft.wfhFiled {
		t.Errorf("a filed draft left the line open: %+v", draft.wfh)
	}
	if !strings.Contains(draft.status, "draft") {
		t.Errorf("status = %q", draft.status)
	}
}

// The retry can be refused with the same words — approval may still be pending — and a line
// that reopened there would loop, filing the same days again on every attempt.
func TestWFHLineDoesNotReopenAfterFiling(t *testing.T) {
	m := send(t, refusedCheckIn(t), runes("at the dentist"))
	m = send(t, m, api.WFHRequestedMsg{ID: 1384})
	m.clocking = true

	again := send(t, m, api.AttendanceMsg{Toggled: true, Want: true, Err: errors.New(wfhRefusal)})
	if again.mode == ModeWFH {
		t.Error("the line reopened on the same refusal")
	}
	if !strings.Contains(again.status, "WFH") {
		t.Errorf("the ERP's own words were dropped: %q", again.status)
	}

	// A check in that lands clears the guard: the next time the days run out, the line is
	// worth having again.
	in := send(t, again, api.AttendanceMsg{Toggled: true, Want: true,
		At: api.Attendance{EmployeeID: 16, CheckedIn: true}})
	if in.wfhFiled {
		t.Error("the guard survived a successful check in")
	}
}

// A range typed backwards is normalized as the field is left, the same rule the other lines
// follow, and nothing on the row may run past the terminal.
func TestWFHLineFitsAndOrdersItsDates(t *testing.T) {
	m := refusedCheckIn(t)
	// 20th in the start field with the 24th already in the end: the end follows the start
	// only when the start passes it, so this range stands.
	m = send(t, m, runes("20"), special(tea.KeyTab), runes("24"), special(tea.KeyTab))
	if !strings.HasPrefix(m.wfh.from.Value(), "20/") || !strings.HasPrefix(m.wfh.to.Value(), "24/") {
		t.Errorf("dates = %q → %q", m.wfh.from.Value(), m.wfh.to.Value())
	}
	// The start dragged past the end takes the end with it, or the request would cover the
	// days between while reading backwards.
	m = send(t, m, special(tea.KeyShiftTab), special(tea.KeyShiftTab), runes("26"),
		special(tea.KeyTab))
	if m.wfh.to.Value() != m.wfh.from.Value() {
		t.Errorf("the end was left behind the start: %q → %q",
			m.wfh.from.Value(), m.wfh.to.Value())
	}

	// The row itself fits well below what the chart behind it needs: it gives up the space
	// inside its boxes rather than running past the edge.
	for _, w := range []int{60, 70, 80, 100, 140} {
		at := send(t, m, tea.WindowSizeMsg{Width: w, Height: 30})
		for i, line := range at.wfhBand() {
			if got := lipgloss.Width(line); got > w {
				t.Errorf("at %d cells row %d is %d wide: %q", w, i, got, line)
			}
		}
		if w < 80 {
			continue // the chart behind it needs its own width; see the dashboard tests
		}
		for i, line := range strings.Split(at.View(), "\n") {
			if got := lipgloss.Width(line); got > w {
				t.Errorf("at %d cells line %d is %d wide: %q", w, i, got, line)
			}
		}
	}
}
