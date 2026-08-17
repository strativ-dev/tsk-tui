package api

import (
	"strings"
	"testing"
	"time"
)

// A trimmed copy of what erp360 returns for get_employee_hour_logs: a weekend, two
// worked days, a holiday, a day with hours still to log, and the double-logged day that
// makes actual and logged_this_day disagree.
const hourLogsJSON = `[
 {"date":"2026-08-01","actual":0,"expected":0,"is_holiday":false,"is_on_leave":false,
  "leave_state":null,"is_half_day_leave":false,"is_weekend":true,"logged_this_day":0,
  "work_location":null,"office_hours":0,"home_hours":0,"is_absent":false},
 {"date":"2026-08-03","actual":8,"expected":8,"is_holiday":false,"is_on_leave":false,
  "leave_state":null,"is_half_day_leave":false,"is_weekend":false,"logged_this_day":8,
  "work_location":"office","office_hours":8.679444444444444,"home_hours":0,"is_absent":false},
 {"date":"2026-08-05","actual":0,"expected":0,"is_holiday":true,"is_on_leave":false,
  "leave_state":null,"is_half_day_leave":false,"is_weekend":false,"logged_this_day":0,
  "work_location":null,"office_hours":0,"home_hours":0,"is_absent":false},
 {"date":"2026-08-06","actual":8,"expected":8,"is_holiday":false,"is_on_leave":false,
  "leave_state":null,"is_half_day_leave":false,"is_weekend":false,"logged_this_day":16,
  "work_location":"office","office_hours":6.793611111111111,"home_hours":1.7038888888888888,
  "is_absent":false},
 {"date":"2026-08-17","actual":7.75,"expected":8,"is_holiday":false,"is_on_leave":false,
  "leave_state":null,"is_half_day_leave":false,"is_weekend":false,"logged_this_day":7.75,
  "work_location":"home","office_hours":0,"home_hours":9,"is_absent":false},
 {"date":"2026-08-18","actual":0,"expected":8,"is_holiday":false,"is_on_leave":false,
  "leave_state":null,"is_half_day_leave":false,"is_weekend":false,"logged_this_day":0,
  "work_location":null,"office_hours":0,"home_hours":0,"is_absent":false}
]`

func TestFetchHourLogs(t *testing.T) {
	var calls []map[string]any
	fakeOdoo(t, hourLogsJSON, &calls)

	msg := FetchHourLogs("k", "user@example.com", "db",
		time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC))().(HourLogsMsg)
	if msg.Err != nil {
		t.Fatalf("FetchHourLogs: %v", msg.Err)
	}
	if msg.Month != "2026-08-01" {
		t.Errorf("Month = %q, want the first of the month it describes", msg.Month)
	}
	if len(msg.Days) != 6 {
		t.Fatalf("days = %d, want 6", len(msg.Days))
	}

	// The fields the chart draws from, including the two that disagree on the 6th.
	for _, c := range []struct {
		date             string
		actual, expected float64
		weekend, holiday bool
		loggedThisDay    float64
	}{
		{date: "2026-08-01", weekend: true},
		{date: "2026-08-03", actual: 8, expected: 8, loggedThisDay: 8},
		{date: "2026-08-05", holiday: true},
		{date: "2026-08-06", actual: 8, expected: 8, loggedThisDay: 16},
		{date: "2026-08-17", actual: 7.75, expected: 8, loggedThisDay: 7.75},
		{date: "2026-08-18", expected: 8},
	} {
		var got DayLog
		for _, d := range msg.Days {
			if d.Date == c.date {
				got = d
			}
		}
		switch {
		case got.Date == "":
			t.Errorf("%s missing from the result", c.date)
		case got.Actual != c.actual || got.Expected != c.expected:
			t.Errorf("%s: actual %v expected %v, want %v and %v",
				c.date, got.Actual, got.Expected, c.actual, c.expected)
		case got.Weekend != c.weekend || got.Holiday != c.holiday:
			t.Errorf("%s: weekend %v holiday %v, want %v and %v",
				c.date, got.Weekend, got.Holiday, c.weekend, c.holiday)
		case got.LoggedTo != c.loggedThisDay:
			t.Errorf("%s: logged_this_day = %v, want %v", c.date, got.LoggedTo, c.loggedThisDay)
		}
	}

	// The call itself: the model method takes no ids and a timestamp inside the month.
	if len(calls) != 2 {
		t.Fatalf("calls = %d, want login then execute_kw", len(calls))
	}
	args := calls[1]["args"].([]any)
	if got := args[3]; got != "account.analytic.line" {
		t.Errorf("model = %v", got)
	}
	if got := args[4]; got != "get_employee_hour_logs" {
		t.Errorf("method = %v", got)
	}
	inner := args[5].([]any)
	if ids, ok := inner[0].([]any); !ok || len(ids) != 0 {
		t.Errorf("first argument = %#v, want an empty ids list", inner[0])
	}
	if got, ok := inner[1].(string); !ok || !strings.HasPrefix(got, "2026-08-17") {
		t.Errorf("date argument = %#v, want a timestamp in the month", inner[1])
	}
}

// A day with no key cannot be read, and the message has to say so rather than looking
// like a month with no hours in it.
func TestFetchHourLogsNeedsCredentials(t *testing.T) {
	if msg := FetchHourLogs("", "user@example.com", "db", time.Now())().(HourLogsMsg); msg.Err == nil {
		t.Error("no key produced no error")
	}
	if msg := FetchHourLogs("k", "user@example.com", "", time.Now())().(HourLogsMsg); msg.Err == nil {
		t.Error("no database produced no error")
	}
}
