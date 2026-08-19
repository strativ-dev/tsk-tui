package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tasnimAlam/tsk/internal/parse"
)

// LeaveKind is one hr.leave.type with the key owner's own allocation on it.
//
// Available is remaining_leaves, the figure the web client puts in its balance card, and
// Max is the allocation it is counted out of. Odoo sends both as strings ("8.5"), so they
// are parsed here rather than left for the view.
type LeaveKind struct {
	ID        int
	Name      string
	Available float64
	Max       float64
}

// Leave is one approved or pending request, as a range of whole days.
//
// The dates are request_date_from / request_date_to — Odoo's own Date fields, with no
// time and no zone. date_from / date_to are UTC datetimes for the same request, and
// reading a day out of those puts a 6am-Dhaka morning on the day before.
type Leave struct {
	From, To string // yyyy-mm-dd, inclusive
	KindID   int
	Kind     string
	Desc     string
	State    string // confirm | validate1 | validate | draft
	Half     bool
	Period   string // "am" | "pm", only when Half
}

// Holiday is a public holiday off the employee's own working calendar, inclusive of both
// ends. The ERP has one calendar per office — Dhaka and Sweden keep different holidays —
// so which one applies is a property of the employee, not of the company.
type Holiday struct {
	From, To string // yyyy-mm-dd
	Name     string
}

// TimeOffMsg is one year of time off: the balances, the requests, and the public holidays
// the calendar dims. All four reads travel in one message because they are one screen —
// half of them landing would draw a calendar whose days disagree with its own totals.
//
// Employee is the hr.employee the calendar belongs to, read on the way to its working
// calendar and kept because a new request has to name it.
type TimeOffMsg struct {
	Year     int
	Employee int
	Kinds    []LeaveKind
	Leaves   []Leave
	Holidays []Holiday
	Err      error
}

// LeaveRequestedMsg answers a request for time off with the hr.leave it created.
type LeaveRequestedMsg struct {
	ID  int
	Err error
}

// RequestLeave is a tea.Cmd: create one hr.leave, which is what the web client's own
// "New Time Off" does. Dates arrive in the app's dd/mm/yy and are converted here, so the
// UI's grammar stops at this boundary.
//
// It writes `request_date_*`, never `date_from`/`date_to`: those are computed from the
// request dates together with the employee's working calendar, which is the only thing that
// knows when their day starts. A half day is one day by definition, so `to` is ignored and
// the period says which half.
//
// Nothing retries. A timed-out create that in fact landed would otherwise book the leave
// twice, and a duplicate leave request is a conversation with HR.
func RequestLeave(key, login, db string, employee, kind int, from, to, desc string,
	half bool, period string) tea.Cmd {
	return func() tea.Msg {
		fail := func(err error) tea.Msg { return LeaveRequestedMsg{Err: err} }
		if kind == 0 {
			return fail(errors.New("pick a leave type first"))
		}
		start, err := time.Parse(parse.DateLayout, strings.TrimSpace(from))
		if err != nil {
			return fail(fmt.Errorf("unreadable date %q", from))
		}
		end := start
		if !half {
			if end, err = time.Parse(parse.DateLayout, strings.TrimSpace(to)); err != nil {
				return fail(fmt.Errorf("unreadable date %q", to))
			}
			if end.Before(start) {
				start, end = end, start // a range typed backwards is still a range
			}
		}

		uid, err := connect(db, login, key)
		if err != nil {
			return fail(err)
		}

		vals := map[string]any{
			"holiday_status_id": kind,
			"request_date_from": start.Format("2006-01-02"),
			"request_date_to":   end.Format("2006-01-02"),
			"name":              strings.TrimSpace(desc),
		}
		if employee != 0 {
			vals["employee_id"] = employee
		}
		if half {
			vals["request_unit_half"] = true
			vals["request_date_from_period"] = period
		}

		raw, err := rpc("object", "execute_kw", []any{
			db, uid, key, "hr.leave", "create", []any{vals},
			map[string]any{"context": map[string]any{"lang": "en_US", "tz": "Asia/Dhaka"}},
		})
		if err != nil {
			return fail(err)
		}
		var id int
		if err := json.Unmarshal(raw, &id); err != nil || id == 0 {
			return fail(errors.New("the ERP refused the request"))
		}
		return LeaveRequestedMsg{ID: id}
	}
}

// FetchTimeOff is a tea.Cmd: everything the time off screen draws, for one year.
//
// Four execute_kw calls behind one login. The REST API exposes none of this, and the web
// client reaches the balances over /web/dataset/call_kw with a session cookie; we have an
// API key, so it goes the same execute_kw way as the rest of the app.
func FetchTimeOff(key, login, db string, year int) tea.Cmd {
	return func() tea.Msg {
		key, login, db = strings.TrimSpace(key), strings.TrimSpace(login), strings.TrimSpace(db)
		fail := func(err error) tea.Msg { return TimeOffMsg{Year: year, Err: err} }

		uid, err := connect(db, login, key)
		if err != nil {
			return fail(err)
		}
		kinds, err := leaveKinds(db, uid, key)
		if err != nil {
			return fail(err)
		}
		leaves, err := leaveRequests(db, uid, key, uid, year)
		if err != nil {
			return fail(err)
		}
		// A calendar of the year has to know which days were never workdays, and that
		// answer belongs to the employee's own working calendar.
		cal, err := calendarOf(db, uid, key)
		if err != nil {
			return fail(err)
		}
		holidays, err := publicHolidays(db, uid, key, cal.Calendar, year)
		if err != nil {
			return fail(err)
		}
		return TimeOffMsg{Year: year, Employee: cal.Employee, Kinds: kinds,
			Leaves: leaves, Holidays: holidays}
	}
}

// leaveKinds reads the balance cards. get_days_all_request is an @api.model method that
// takes nothing at all — not even an ids list — and answers with one array per type:
// [name, {figures as strings}, "yes", type id]. The id is the last element, which is what
// makes a filter by type possible without matching on the name.
func leaveKinds(db string, uid int, key string) ([]LeaveKind, error) {
	raw, err := rpc("object", "execute_kw", []any{
		db, uid, key,
		"hr.leave.type", "get_days_all_request",
		[]any{},
	})
	if err != nil {
		return nil, err
	}

	var rows [][]json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("bad leave-type result: %w", err)
	}

	out := make([]LeaveKind, 0, len(rows))
	for _, r := range rows {
		if len(r) < 4 {
			continue
		}
		var name odooText
		var id int
		var days struct {
			Remaining odooText `json:"remaining_leaves"`
			Max       odooText `json:"max_leaves"`
		}
		_ = json.Unmarshal(r[0], &name)
		_ = json.Unmarshal(r[1], &days)
		if err := json.Unmarshal(r[3], &id); err != nil {
			continue
		}
		out = append(out, LeaveKind{
			ID:        id,
			Name:      strings.TrimSpace(string(name)),
			Available: odooNum(string(days.Remaining)),
			Max:       odooNum(string(days.Max)),
		})
	}
	return out, nil
}

// leaveRequests reads the year's own requests, mine only.
//
// The domain overlaps the year rather than containing it, so a request that runs across
// New Year is on both years' calendars instead of neither. Refused and cancelled requests
// are dropped here: they are not time off, and a red day nobody is taking off would read
// as one that was.
func leaveRequests(db string, uid int, key string, user, year int) ([]Leave, error) {
	first, last := fmt.Sprintf("%04d-01-01", year), fmt.Sprintf("%04d-12-31", year)
	raw, err := rpc("object", "execute_kw", []any{
		db, uid, key,
		"hr.leave", "search_read",
		[]any{[]any{
			[]any{"user_id", "=", user},
			[]any{"request_date_from", "<=", last},
			[]any{"request_date_to", ">=", first},
			[]any{"state", "not in", []string{"refuse", "cancel"}},
		}},
		map[string]any{
			"fields": []string{"holiday_status_id", "request_date_from", "request_date_to",
				"name", "state", "request_unit_half", "request_date_from_period"},
			"order": "request_date_from asc",
		},
	})
	if err != nil {
		return nil, err
	}

	var rows []struct {
		Kind   odooRef  `json:"holiday_status_id"`
		From   odooText `json:"request_date_from"`
		To     odooText `json:"request_date_to"`
		Name   odooText `json:"name"`
		State  odooText `json:"state"`
		Half   bool     `json:"request_unit_half"`
		Period odooText `json:"request_date_from_period"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("bad leave result: %w", err)
	}

	out := make([]Leave, 0, len(rows))
	for _, r := range rows {
		to := string(r.To)
		if to == "" {
			to = string(r.From) // a one-day request Odoo left open-ended
		}
		l := Leave{
			From: string(r.From), To: to,
			KindID: r.Kind.ID, Kind: strings.TrimSpace(r.Kind.Name),
			Desc:  strings.TrimSpace(string(r.Name)),
			State: string(r.State),
			Half:  r.Half,
		}
		if l.Half {
			l.Period = string(r.Period)
		}
		out = append(out, l)
	}
	return out, nil
}

// calendarOf is the resource.calendar behind the key owner — which office's working week
// and holidays apply to them.
//
// A separate read from employeeOf on purpose: that one asks for exactly two fields
// because this user cannot read all of hr.employee, and one refused field fails the whole
// call. Widening it there would break the clock to save a round trip here.
func calendarOf(db string, uid int, key string) (employee, error) {
	raw, err := rpc("object", "execute_kw", []any{
		db, uid, key,
		"hr.employee", "search_read",
		[]any{[]any{[]any{"user_id", "=", uid}}},
		map[string]any{"fields": []string{"id", "resource_calendar_id"}, "limit": 1},
	})
	if err != nil {
		return employee{}, err
	}
	var rows []struct {
		ID       int     `json:"id"`
		Calendar odooRef `json:"resource_calendar_id"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return employee{}, fmt.Errorf("bad employee result: %w", err)
	}
	if len(rows) == 0 {
		// No employee record: no calendar to read, so no holidays, and a request for time
		// off will have to let Odoo work out who is asking.
		return employee{}, nil
	}
	return employee{Employee: rows[0].ID, Calendar: rows[0].Calendar.ID}, nil
}

// employee is the two ids one read of hr.employee answers: who the key owner is, and which
// office's working week and holidays apply to them.
type employee struct {
	Employee int
	Calendar int
}

// publicHolidays reads the global closures on the employee's calendar — the rows of
// resource.calendar.leaves with no resource on them, which is how Odoo stores a holiday
// as opposed to one person's day off.
//
// Both the calendar's own and the ones on no calendar at all, since a company-wide
// closure is a holiday for everyone.
func publicHolidays(db string, uid int, key string, calendar, year int) ([]Holiday, error) {
	if calendar == 0 {
		return nil, nil
	}
	first := fmt.Sprintf("%04d-01-01 00:00:00", year)
	last := fmt.Sprintf("%04d-12-31 23:59:59", year)
	raw, err := rpc("object", "execute_kw", []any{
		db, uid, key,
		"resource.calendar.leaves", "search_read",
		[]any{[]any{
			[]any{"resource_id", "=", false},
			"|", []any{"calendar_id", "=", calendar}, []any{"calendar_id", "=", false},
			[]any{"date_from", "<=", last},
			[]any{"date_to", ">=", first},
		}},
		map[string]any{"fields": []string{"name", "date_from", "date_to"}, "order": "date_from asc"},
	})
	if err != nil {
		return nil, err
	}

	var rows []struct {
		Name string   `json:"name"`
		From odooText `json:"date_from"`
		To   odooText `json:"date_to"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("bad holiday result: %w", err)
	}

	out := make([]Holiday, 0, len(rows))
	for _, r := range rows {
		// These are stored as datetimes spanning the working day — 04:00 to 13:00 UTC for
		// Dhaka, 07:00 to 16:00 for Sweden — so both ends fall on their own local date and
		// the date part is the day itself.
		from, to := dayOf(string(r.From)), dayOf(string(r.To))
		if from == "" {
			continue
		}
		if to == "" {
			to = from
		}
		out = append(out, Holiday{From: from, To: to, Name: oneLine(r.Name)})
	}
	return out, nil
}

// odooRef is a many2one as Odoo sends it: [id, "display name"], or false when unset.
type odooRef struct {
	ID   int
	Name string
}

func (o *odooRef) UnmarshalJSON(b []byte) error {
	var pair []json.RawMessage
	if err := json.Unmarshal(b, &pair); err != nil {
		return nil // false, null, or anything else that is not a reference
	}
	if len(pair) > 0 {
		_ = json.Unmarshal(pair[0], &o.ID)
	}
	if len(pair) > 1 {
		var name odooText
		_ = json.Unmarshal(pair[1], &name)
		o.Name = string(name)
	}
	return nil
}

// odooNum reads a figure Odoo rendered as a string ("8.5"). An unreadable one is 0 rather
// than an error: a balance nobody can parse is worth a zero on the screen, not a blank
// screen.
func odooNum(s string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return f
}

// dayOf is the date half of an Odoo datetime, "" if it is not one.
func dayOf(s string) string {
	t, err := time.Parse("2006-01-02 15:04:05", strings.TrimSpace(s))
	if err != nil {
		return ""
	}
	return t.Format("2006-01-02")
}

// oneLine collapses whitespace in a name the ERP wrote, the way the model's own oneLine
// does for task titles: a newline in a holiday name would render the panel a line taller
// than it was given.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }
