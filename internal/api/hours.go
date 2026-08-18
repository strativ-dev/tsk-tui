package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// DayLog is one day of the ERP's own hour-log summary for the key's owner, as
// account.analytic.line.get_employee_hour_logs returns it. Hours are decimal, the way
// Odoo keeps them; the model turns them into minutes only when it needs to.
type DayLog struct {
	Date     string  `json:"date"` // YYYY-MM-DD
	Actual   float64 `json:"actual"`
	Expected float64 `json:"expected"`

	Holiday  bool    `json:"is_holiday"`
	Weekend  bool    `json:"is_weekend"`
	OnLeave  bool    `json:"is_on_leave"`
	HalfDay  bool    `json:"is_half_day_leave"`
	Absent   bool    `json:"is_absent"`
	LoggedTo float64 `json:"logged_this_day"`

	// WorkLocation is "office", "home" or "" — the ERP's own verdict, which is one value
	// even on a day whose office_hours and home_hours are both set. odooText, not string:
	// Odoo sends false for an empty char field, and false into a string fails the whole
	// month rather than the one day.
	WorkLocation odooText `json:"work_location"`
}

// HourLogsMsg carries a month of day summaries. Month is the first of the month it
// describes, so a late answer for a month nobody is looking at can be ignored.
type HourLogsMsg struct {
	Month string // YYYY-MM-01
	Days  []DayLog
	Err   error
}

// FetchHourLogs is a tea.Cmd: one month of the employee's daily hour log.
//
// The browser reaches this over /web/dataset/call_kw with a session cookie; we have an
// API key, so it goes through the same execute_kw path as everything else. The method
// is @api.model, hence the empty ids list as its first argument, and it takes any
// timestamp inside the month it should report on.
func FetchHourLogs(key, login, db string, month time.Time) tea.Cmd {
	first := month.Format("2006-01") + "-01"
	return func() tea.Msg {
		key, login, db = strings.TrimSpace(key), strings.TrimSpace(login), strings.TrimSpace(db)
		uid, err := connect(db, login, key)
		if err != nil {
			return HourLogsMsg{Month: first, Err: err}
		}

		raw, err := rpc("object", "execute_kw", []any{
			db, uid, key,
			"account.analytic.line", "get_employee_hour_logs",
			[]any{[]int{}, month.Format("2006-01-02") + " 00:00:00"},
		})
		if err != nil {
			return HourLogsMsg{Month: first, Err: err}
		}

		var days []DayLog
		if err := json.Unmarshal(raw, &days); err != nil {
			return HourLogsMsg{Month: first, Err: fmt.Errorf("bad hour-log result: %w", err)}
		}
		return HourLogsMsg{Month: first, Days: days}
	}
}
