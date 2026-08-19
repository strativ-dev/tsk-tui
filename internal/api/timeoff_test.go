package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeTimeOff answers the four reads FetchTimeOff makes, each with its own body, and
// records the calls so the domains can be checked.
func fakeTimeOff(t *testing.T, calls *[]map[string]any) {
	t.Helper()
	const balances = `[
		["Sick Time Off", {"remaining_leaves": "9", "max_leaves": "14",
			"closest_allocation_expire": "31/12/2026"}, "yes", 2],
		["Casual Time Off", {"remaining_leaves": "6", "max_leaves": "12",
			"closest_allocation_expire": false}, "yes", 4],
		["Annual Time Off", {"remaining_leaves": "8.5", "max_leaves": "11.5",
			"closest_allocation_expire": false}, "yes", 1]
	]`
	const leaves = `[
		{"id": 2445, "holiday_status_id": [4, "Casual Time Off"],
			"request_date_from": "2026-01-21", "request_date_to": "2026-01-23",
			"name": "Family errand", "state": "validate",
			"request_unit_half": false, "request_date_from_period": "am"},
		{"id": 2601, "holiday_status_id": [1, "Annual Time Off"],
			"request_date_from": "2026-02-18", "request_date_to": "2026-02-18",
			"name": false, "state": "confirm",
			"request_unit_half": true, "request_date_from_period": "pm"}
	]`
	const employee = `[{"id": 16, "resource_calendar_id": [1, "Standard 40 hours/week (Dhaka)"]}]`
	const holidays = `[
		{"id": 2305, "name": "Shab e-barat*", "date_from": "2026-02-04 04:00:00",
			"date_to": "2026-02-04 13:00:00"},
		{"id": 2314, "name": "Public\nHoliday", "date_from": "2026-02-11 04:00:00",
			"date_to": "2026-02-12 13:00:00"}
	]`

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
		result := `[]`
		if req.Params.Method == "login" {
			result = `26`
		}
		if len(req.Params.Args) >= 5 {
			switch model, _ := req.Params.Args[3].(string); model {
			case "hr.leave.type":
				result = balances
			case "hr.employee":
				result = employee
			case "resource.calendar.leaves":
				result = holidays
			case "hr.leave":
				result = leaves
				if req.Params.Args[4] == "create" {
					result = `3120` // Odoo answers create with the new id
				}
			}
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":` + result + `}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv(BaseEnv, srv.URL)
}

func TestFetchTimeOff(t *testing.T) {
	var calls []map[string]any
	fakeTimeOff(t, &calls)

	msg, ok := FetchTimeOff("secret-key", "user@example.com", "erp-test", 2026)().(TimeOffMsg)
	if !ok {
		t.Fatal("FetchTimeOff did not return TimeOffMsg")
	}
	if msg.Err != nil {
		t.Fatalf("Err = %v", msg.Err)
	}
	if msg.Year != 2026 {
		t.Errorf("Year = %d, want 2026", msg.Year)
	}

	// Balances: Odoo sends the figures as strings, and the type id is the last element of
	// the row — the only thing that lets a filter name a type without matching its name.
	if len(msg.Kinds) != 3 {
		t.Fatalf("kinds = %d, want 3", len(msg.Kinds))
	}
	if got := msg.Kinds[2]; got.ID != 1 || got.Name != "Annual Time Off" ||
		got.Available != 8.5 || got.Max != 11.5 {
		t.Errorf("kinds[2] = %+v, want the annual balance parsed out of its strings", got)
	}

	if len(msg.Leaves) != 2 {
		t.Fatalf("leaves = %d, want 2", len(msg.Leaves))
	}
	if got := msg.Leaves[0]; got.From != "2026-01-21" || got.To != "2026-01-23" ||
		got.KindID != 4 || got.Kind != "Casual Time Off" || got.Desc != "Family errand" ||
		got.State != "validate" || got.Half {
		t.Errorf("leaves[0] = %+v", got)
	}
	// A half day says which half, and Odoo's false for an empty description is "".
	if got := msg.Leaves[1]; !got.Half || got.Period != "pm" || got.Desc != "" ||
		got.State != "confirm" {
		t.Errorf("leaves[1] = %+v", got)
	}

	// Holidays keep both ends, and a name the ERP wrote with a newline in it is flattened:
	// one newline in a panel line would render the panel a row taller than it was given.
	if len(msg.Holidays) != 2 {
		t.Fatalf("holidays = %d, want 2", len(msg.Holidays))
	}
	if got := msg.Holidays[0]; got.From != "2026-02-04" || got.To != "2026-02-04" {
		t.Errorf("holidays[0] = %+v", got)
	}
	if got := msg.Holidays[1]; got.From != "2026-02-11" || got.To != "2026-02-12" ||
		got.Name != "Public Holiday" {
		t.Errorf("holidays[1] = %+v", got)
	}

	if len(calls) != 5 {
		t.Fatalf("made %d calls, want login + four reads", len(calls))
	}
}

// get_days_all_request takes no arguments at all — not even an empty ids list. Passing one
// fails the whole read with "takes 1 positional argument but 2 were given", which is a
// blank set of balances on the screen.
func TestBalancesArePassedNoArguments(t *testing.T) {
	var calls []map[string]any
	fakeTimeOff(t, &calls)
	FetchTimeOff("k", "user@example.com", "db", 2026)()

	for _, c := range calls {
		args, _ := c["args"].([]any)
		if len(args) < 6 || args[3] != "hr.leave.type" {
			continue
		}
		if args[4] != "get_days_all_request" {
			t.Errorf("method = %v", args[4])
		}
		if got, _ := args[5].([]any); len(got) != 0 {
			t.Errorf("args = %v, want none: the method takes only self", got)
		}
		return
	}
	t.Fatal("no hr.leave.type call was made")
}

// The leave domain overlaps the year rather than sitting inside it, so a request that runs
// across New Year is on both years' calendars; refused and cancelled ones are not time off
// and never reach the calendar.
func TestLeaveDomainOverlapsTheYear(t *testing.T) {
	var calls []map[string]any
	fakeTimeOff(t, &calls)
	FetchTimeOff("k", "user@example.com", "db", 2026)()

	for _, c := range calls {
		args, _ := c["args"].([]any)
		if len(args) < 6 || args[3] != "hr.leave" {
			continue
		}
		domain, _ := json.Marshal(args[5])
		for _, want := range []string{
			`["user_id","=",26]`,
			// json.Marshal escapes the operators, hence < / >.
			`["request_date_from","\u003c=","2026-12-31"]`,
			`["request_date_to","\u003e=","2026-01-01"]`,
			`["state","not in",["refuse","cancel"]]`,
		} {
			if !strings.Contains(string(domain), want) {
				t.Errorf("domain %s is missing %s", domain, want)
			}
		}
		return
	}
	t.Fatal("no hr.leave call was made")
}

// Holidays are per office: Dhaka and Sweden keep different ones, and which applies is the
// employee's own working calendar. A company-wide closure has no calendar at all.
func TestHolidaysFollowTheEmployeeCalendar(t *testing.T) {
	var calls []map[string]any
	fakeTimeOff(t, &calls)
	FetchTimeOff("k", "user@example.com", "db", 2026)()

	for _, c := range calls {
		args, _ := c["args"].([]any)
		if len(args) < 6 || args[3] != "resource.calendar.leaves" {
			continue
		}
		domain, _ := json.Marshal(args[5])
		for _, want := range []string{
			`["resource_id","=",false]`,
			`"|"`,
			`["calendar_id","=",1]`,
			`["calendar_id","=",false]`,
		} {
			if !strings.Contains(string(domain), want) {
				t.Errorf("domain %s is missing %s", domain, want)
			}
		}
		return
	}
	t.Fatal("no resource.calendar.leaves call was made")
}

func TestFetchTimeOffNeedsCredentials(t *testing.T) {
	for _, c := range []struct{ key, login, db string }{
		{"", "u@example.com", "db"},
		{"k", "", "db"},
		{"k", "u@example.com", ""},
	} {
		msg := FetchTimeOff(c.key, c.login, c.db, 2026)().(TimeOffMsg)
		if msg.Err == nil {
			t.Errorf("key=%q login=%q db=%q was accepted", c.key, c.login, c.db)
		}
	}
}

// A request for time off writes request_date_from / request_date_to, never date_from: those
// are computed from these together with the employee's own working calendar.
func TestRequestLeave(t *testing.T) {
	var calls []map[string]any
	fakeTimeOff(t, &calls)
	msg := RequestLeave("k", "user@example.com", "db", 16, 1,
		"21/01/26", "23/01/26", "  Coast trip  ", false, "am")().(LeaveRequestedMsg)
	if msg.Err != nil {
		t.Fatalf("Err = %v", msg.Err)
	}

	vals := createVals(t, calls)
	for k, want := range map[string]any{
		"holiday_status_id": 1.0,
		"employee_id":       16.0,
		"request_date_from": "2026-01-21",
		"request_date_to":   "2026-01-23",
		"name":              "Coast trip",
	} {
		if vals[k] != want {
			t.Errorf("%s = %v (%T), want %v", k, vals[k], vals[k], want)
		}
	}
	// Not a half day, so neither half-day field is sent at all.
	if _, ok := vals["request_unit_half"]; ok {
		t.Errorf("request_unit_half was sent for a full day: %v", vals)
	}
}

// A half day is one day by definition: the range's end is ignored and the period says which
// half of it.
func TestRequestLeaveHalfDay(t *testing.T) {
	var calls []map[string]any
	fakeTimeOff(t, &calls)
	RequestLeave("k", "user@example.com", "db", 16, 2,
		"18/02/26", "28/02/26", "Headache", true, "pm")()

	vals := createVals(t, calls)
	if vals["request_date_to"] != "2026-02-18" {
		t.Errorf("request_date_to = %v, want the one day it covers", vals["request_date_to"])
	}
	if vals["request_unit_half"] != true || vals["request_date_from_period"] != "pm" {
		t.Errorf("half day = %v / %v", vals["request_unit_half"], vals["request_date_from_period"])
	}
}

// A range typed backwards is still a range.
func TestRequestLeaveOrdersTheRange(t *testing.T) {
	var calls []map[string]any
	fakeTimeOff(t, &calls)
	RequestLeave("k", "user@example.com", "db", 0, 4, "23/01/26", "21/01/26", "", false, "am")()

	vals := createVals(t, calls)
	if vals["request_date_from"] != "2026-01-21" || vals["request_date_to"] != "2026-01-23" {
		t.Errorf("dates = %v → %v", vals["request_date_from"], vals["request_date_to"])
	}
	// No employee id known: Odoo works out who is asking rather than being sent a zero.
	if _, ok := vals["employee_id"]; ok {
		t.Errorf("employee_id 0 was sent: %v", vals)
	}
}

func TestRequestLeaveRefusals(t *testing.T) {
	fakeTimeOff(t, nil)
	for _, c := range []struct {
		what           string
		kind           int
		from, to       string
		key, login, db string
	}{
		{"no type", 0, "21/01/26", "21/01/26", "k", "u@e.com", "db"},
		{"unreadable start", 1, "the 21st", "21/01/26", "k", "u@e.com", "db"},
		{"unreadable end", 1, "21/01/26", "next week", "k", "u@e.com", "db"},
		{"no key", 1, "21/01/26", "21/01/26", "", "u@e.com", "db"},
		{"no db", 1, "21/01/26", "21/01/26", "k", "u@e.com", ""},
	} {
		msg := RequestLeave(c.key, c.login, c.db, 16, c.kind, c.from, c.to, "x", false, "am")().(LeaveRequestedMsg)
		if msg.Err == nil {
			t.Errorf("%s was accepted", c.what)
		}
	}
}

// createVals is the values dict of the hr.leave create among the recorded calls.
func createVals(t *testing.T, calls []map[string]any) map[string]any {
	t.Helper()
	for _, c := range calls {
		args, _ := c["args"].([]any)
		if len(args) < 6 || args[3] != "hr.leave" || args[4] != "create" {
			continue
		}
		vals, _ := args[5].([]any)
		if len(vals) == 0 {
			t.Fatalf("create was called with no values: %v", args[5])
		}
		out, ok := vals[0].(map[string]any)
		if !ok {
			t.Fatalf("create values are %T", vals[0])
		}
		return out
	}
	t.Fatal("no hr.leave create was made")
	return nil
}
