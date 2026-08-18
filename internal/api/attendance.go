package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// attendanceAction is the xmlid attendance_manual wants for the action it would open in
// the web client. We throw the action away — the state read after it is what we trust —
// but the argument is positional and required.
const attendanceAction = "hr_attendance.hr_attendance_action_my_attendances"

// Attendance is where the key's owner stands with the ERP's own clock.
//
// Since is UTC because that is what Odoo stores; the view is the only place that turns it
// into local time, so the conversion has exactly one home.
type Attendance struct {
	EmployeeID int
	CheckedIn  bool
	Since      time.Time // check_in of the open session; zero when checked out
}

// AttendanceMsg answers both a plain read and a toggle, since both end with the same
// question: what does the server say now.
//
// Toggled and Want carry the intent back so the model can tell a stale read from a fresh
// answer, and can say so when the ERP disagrees with what was asked for. Warning is
// attendance_manual refusing — "already checked in", say — which is not an error: the
// state moved without us, and the snapshot beside it is what fixes the screen.
type AttendanceMsg struct {
	At      Attendance
	Toggled bool
	Want    bool
	Warning string
	Err     error
}

// FetchAttendance is a tea.Cmd: read the clock without touching it. Pass the employee id
// once it is known — it saves a round trip, and it never changes.
func FetchAttendance(key, login, db string, employee int) tea.Cmd {
	return func() tea.Msg {
		at, warn, err := attendance(key, login, db, employee, false)
		return AttendanceMsg{At: at, Warning: warn, Err: err}
	}
}

// ToggleAttendance is a tea.Cmd: check in if out, out if in — attendance_manual is one
// toggle, like the button in the web client — and then re-read, so the screen only ever
// shows state the server confirmed. want is what the caller was trying to reach, carried
// back for the case where the answer disagrees.
func ToggleAttendance(key, login, db string, employee int, want bool) tea.Cmd {
	return func() tea.Msg {
		at, warn, err := attendance(key, login, db, employee, true)
		return AttendanceMsg{At: at, Toggled: true, Want: want, Warning: warn, Err: err}
	}
}

func attendance(key, login, db string, employee int, toggle bool) (Attendance, string, error) {
	key, login, db = strings.TrimSpace(key), strings.TrimSpace(login), strings.TrimSpace(db)
	uid, err := connect(db, login, key)
	if err != nil {
		return Attendance{}, "", err
	}

	if employee == 0 {
		employee, err = employeeOf(db, uid, key)
		if err != nil {
			return Attendance{}, "", err
		}
	}

	var warning string
	if toggle {
		if warning, err = attendanceManual(db, uid, key, employee); err != nil {
			return Attendance{EmployeeID: employee}, "", err
		}
	}

	at, err := openSession(db, uid, key, employee)
	return at, warning, err
}

// employeeOf finds the hr.employee behind a uid.
//
// The field list is exactly these two on purpose: this user cannot read hours_last_month
// (it belongs to the Attendances officer group) and one refused field fails the whole
// read, so a widened list would break the feature rather than adding to it.
func employeeOf(db string, uid int, key string) (int, error) {
	raw, err := rpc("object", "execute_kw", []any{
		db, uid, key,
		"hr.employee", "search_read",
		[]any{[]any{[]any{"user_id", "=", uid}}},
		map[string]any{"fields": []string{"id", "attendance_state"}, "limit": 1},
	})
	if err != nil {
		return 0, err
	}

	var rows []struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return 0, fmt.Errorf("bad employee result: %w", err)
	}
	if len(rows) == 0 || rows[0].ID == 0 {
		return 0, errors.New("no employee record in the ERP for this login — ask HR")
	}
	return rows[0].ID, nil
}

// attendanceManual presses the ERP's own check in / check out button. Odoo answers with
// the action it would have opened, or {"warning": "…"} when it declined.
func attendanceManual(db string, uid int, key string, employee int) (string, error) {
	raw, err := rpc("object", "execute_kw", []any{
		db, uid, key,
		"hr.employee", "attendance_manual",
		[]any{[]int{employee}, attendanceAction},
		map[string]any{"context": map[string]any{"lang": "en_US"}},
	})
	if err != nil {
		return "", err
	}

	// A bare false is Odoo refusing outright; an action dict simply has no warning key.
	var refused bool
	if err := json.Unmarshal(raw, &refused); err == nil && !refused {
		return "", errors.New("the ERP refused the check in")
	}
	var out struct {
		Warning odooText `json:"warning"`
	}
	_ = json.Unmarshal(raw, &out)
	return string(out.Warning), nil
}

// openSession reads the attendance that has not been closed yet. That one row answers
// both questions at once — a row exists, so you are checked in, and it says since when.
//
// last_attendance_id would not: after a check out it still points at the closed record,
// whose check_in is populated, which would draw a running clock beside a "check in"
// button. check_out = false cannot be stale in that way.
func openSession(db string, uid int, key string, employee int) (Attendance, error) {
	at := Attendance{EmployeeID: employee}
	raw, err := rpc("object", "execute_kw", []any{
		db, uid, key,
		"hr.attendance", "search_read",
		[]any{[]any{
			[]any{"employee_id", "=", employee},
			[]any{"check_out", "=", false},
		}},
		map[string]any{
			"fields": []string{"check_in"},
			"limit":  1,
			"order":  "check_in desc",
		},
	})
	if err != nil {
		return at, err
	}

	var rows []struct {
		CheckIn odooText `json:"check_in"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return at, fmt.Errorf("bad attendance result: %w", err)
	}
	if len(rows) == 0 {
		return at, nil
	}

	at.CheckedIn = true
	// Odoo sends naive UTC: "2026-08-18 05:05:28" is 11:05 in Dhaka.
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", string(rows[0].CheckIn), time.UTC); err == nil {
		at.Since = t
	}
	return at, nil
}
