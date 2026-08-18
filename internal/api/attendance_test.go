package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// attendanceServer answers login, then dispatches execute_kw on the model and method it
// was called with. fakeOdoo cannot: it returns one canned result for every call, and this
// feature makes two or three different ones per command.
func attendanceServer(t *testing.T, results map[string]string, calls *[]map[string]any) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Params struct {
				Service string `json:"service"`
				Method  string `json:"method"`
				Args    []any  `json:"args"`
			} `json:"params"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("unparsable request: %v", err)
		}
		if calls != nil {
			*calls = append(*calls, map[string]any{
				"service": req.Params.Service,
				"method":  req.Params.Method,
				"args":    req.Params.Args,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		if req.Params.Method == "login" {
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":26}`))
			return
		}
		model, _ := req.Params.Args[3].(string)
		method, _ := req.Params.Args[4].(string)
		out, ok := results[model+"."+method]
		if !ok {
			t.Errorf("unexpected call %s.%s", model, method)
			out = "false"
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":` + out + `}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv(BaseEnv, srv.URL)
}

const (
	employeeIn  = `[{"id": 16, "attendance_state": "checked_in"}]`
	openSess    = `[{"id": 43372, "check_in": "2026-08-18 05:05:28"}]`
	closedSess  = `[]`
	manualOK    = `{"action": {"type": "ir.actions.client"}}`
	manualWarns = `{"warning": "You cannot check in twice from two devices"}`
)

// The clock, read without touching it: the employee behind the login, then the session
// that has no check-out yet. Odoo sends naive UTC, and that is what the API layer keeps —
// only the view turns it into the terminal's zone.
func TestFetchAttendanceCheckedIn(t *testing.T) {
	var calls []map[string]any
	attendanceServer(t, map[string]string{
		"hr.employee.search_read":   employeeIn,
		"hr.attendance.search_read": openSess,
	}, &calls)

	msg, ok := FetchAttendance("secret-key", "user@example.com", "erp-test", 0)().(AttendanceMsg)
	if !ok {
		t.Fatal("FetchAttendance did not return AttendanceMsg")
	}
	if msg.Err != nil {
		t.Fatalf("Err = %v", msg.Err)
	}
	if !msg.At.CheckedIn || msg.At.EmployeeID != 16 {
		t.Errorf("At = %+v, want employee 16 checked in", msg.At)
	}
	want := time.Date(2026, 8, 18, 5, 5, 28, 0, time.UTC)
	if !msg.At.Since.Equal(want) {
		t.Errorf("Since = %v, want %v (naive UTC, 11:05 in Dhaka)", msg.At.Since, want)
	}
	if msg.Toggled {
		t.Error("a plain read came back marked as a toggle")
	}

	// login, the employee, the session.
	if len(calls) != 3 {
		t.Fatalf("%d calls, want 3:\n%v", len(calls), calls)
	}
	for i, want := range []struct{ model, method string }{
		{"hr.employee", "search_read"},
		{"hr.attendance", "search_read"},
	} {
		args := calls[i+1]["args"].([]any)
		if args[3] != want.model || args[4] != want.method {
			t.Errorf("call %d is %v.%v, want %s.%s", i+1, args[3], args[4], want.model, want.method)
		}
	}
	// The open session is found by check_out = false, never by last_attendance_id, which
	// still points at the closed record after a check out.
	domain, _ := json.Marshal(calls[2]["args"].([]any)[5])
	if string(domain) != `[[["employee_id","=",16],["check_out","=",false]]]` {
		t.Errorf("session domain = %s", domain)
	}
}

// No open session means checked out, and nothing pretends to know since when.
func TestFetchAttendanceCheckedOut(t *testing.T) {
	attendanceServer(t, map[string]string{
		"hr.employee.search_read":   `[{"id": 16, "attendance_state": "checked_out"}]`,
		"hr.attendance.search_read": closedSess,
	}, nil)

	msg := FetchAttendance("k", "user@example.com", "db", 0)().(AttendanceMsg)
	if msg.Err != nil {
		t.Fatalf("Err = %v", msg.Err)
	}
	if msg.At.CheckedIn {
		t.Error("checked in with no open session")
	}
	if !msg.At.Since.IsZero() {
		t.Errorf("Since = %v, want the zero time", msg.At.Since)
	}
}

// A known employee id saves the lookup: two calls instead of three.
func TestFetchAttendanceReusesTheEmployee(t *testing.T) {
	var calls []map[string]any
	attendanceServer(t, map[string]string{"hr.attendance.search_read": openSess}, &calls)

	if msg := FetchAttendance("k", "user@example.com", "db", 16)().(AttendanceMsg); msg.Err != nil {
		t.Fatalf("Err = %v", msg.Err)
	}
	if len(calls) != 2 {
		t.Errorf("%d calls with the employee already known, want 2", len(calls))
	}
}

// The field list is two fields for a reason: this user cannot read hours_last_month, and
// one refused field fails the whole read.
func TestAttendanceFieldsStayMinimal(t *testing.T) {
	var calls []map[string]any
	attendanceServer(t, map[string]string{
		"hr.employee.search_read":   employeeIn,
		"hr.attendance.search_read": openSess,
	}, &calls)

	FetchAttendance("k", "user@example.com", "db", 0)()

	kwargs, _ := json.Marshal(calls[1]["args"].([]any)[6])
	if string(kwargs) != `{"fields":["id","attendance_state"],"limit":1}` {
		t.Errorf("employee kwargs = %s — extra fields break the whole call", kwargs)
	}
}

// The toggle presses the ERP's own button and then re-reads, so nothing on screen is a
// guess about what the server did.
func TestToggleAttendance(t *testing.T) {
	var calls []map[string]any
	attendanceServer(t, map[string]string{
		"hr.employee.attendance_manual": manualOK,
		"hr.attendance.search_read":     openSess,
	}, &calls)

	msg := ToggleAttendance("k", "user@example.com", "db", 16, true)().(AttendanceMsg)
	if msg.Err != nil {
		t.Fatalf("Err = %v", msg.Err)
	}
	if !msg.Toggled || !msg.Want {
		t.Errorf("Toggled = %v, Want = %v — the intent did not come back", msg.Toggled, msg.Want)
	}
	if !msg.At.CheckedIn {
		t.Error("the state read after the toggle did not run")
	}

	if len(calls) != 3 {
		t.Fatalf("%d calls, want login + toggle + read:\n%v", len(calls), calls)
	}
	args := calls[1]["args"].([]any)
	positional, _ := json.Marshal(args[5])
	if string(positional) != `[[16],"hr_attendance.hr_attendance_action_my_attendances"]` {
		t.Errorf("attendance_manual args = %s", positional)
	}
	if kwargs, _ := args[6].(map[string]any); kwargs["context"] == nil {
		t.Errorf("no context kwarg: %v", args[6])
	}
}

// A refusal is not an error: the state moved without us, and the snapshot beside the
// warning is what puts the screen right.
func TestToggleAttendanceWarning(t *testing.T) {
	attendanceServer(t, map[string]string{
		"hr.employee.attendance_manual": manualWarns,
		"hr.attendance.search_read":     openSess,
	}, nil)

	msg := ToggleAttendance("k", "user@example.com", "db", 16, false)().(AttendanceMsg)
	if msg.Err != nil {
		t.Fatalf("Err = %v, want the warning instead", msg.Err)
	}
	if msg.Warning == "" {
		t.Error("the warning was dropped")
	}
	if !msg.At.CheckedIn {
		t.Error("the state was not re-read after the refusal")
	}
}

// A bare false is Odoo refusing outright, so it fails rather than reading as success.
func TestToggleAttendanceRefused(t *testing.T) {
	attendanceServer(t, map[string]string{"hr.employee.attendance_manual": "false"}, nil)

	if msg := ToggleAttendance("k", "user@example.com", "db", 16, true)().(AttendanceMsg); msg.Err == nil {
		t.Error("a false answer read as a successful check in")
	}
}

// An ERP user with no employee record cannot be clocked in, and says so.
func TestAttendanceNeedsAnEmployee(t *testing.T) {
	attendanceServer(t, map[string]string{"hr.employee.search_read": `[]`}, nil)

	msg := FetchAttendance("k", "user@example.com", "db", 0)().(AttendanceMsg)
	if msg.Err == nil {
		t.Fatal("no employee record was not an error")
	}
	if got := msg.Err.Error(); !strings.Contains(got, "no employee record") {
		t.Errorf("Err = %q, want it to name the missing employee record", got)
	}
}

// The same credential guard as every other RPC command, and no request without them.
func TestAttendanceNeedsCredentials(t *testing.T) {
	t.Setenv(BaseEnv, "http://127.0.0.1:1")
	for _, tc := range []struct {
		name, key, login, db string
		want                 error
	}{
		{"no key", "", "user@example.com", "db", ErrNoKey},
		{"no db", "k", "user@example.com", "", ErrNoDB},
	} {
		msg := FetchAttendance(tc.key, tc.login, tc.db, 16)().(AttendanceMsg)
		if !errors.Is(msg.Err, tc.want) {
			t.Errorf("%s: Err = %v, want %v", tc.name, msg.Err, tc.want)
		}
	}
	if msg := FetchAttendance("k", "", "db", 16)().(AttendanceMsg); msg.Err == nil {
		t.Error("an empty login reached the network")
	}
}
