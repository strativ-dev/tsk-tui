package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tasnimAlam/tsk/internal/parse"
)

// wfhModel is the serp attendance module's own request, which is **not** an hr.leave: there
// is no work-from-home leave type on this database, and hr.attendance points at this model
// through wfh_request_id. Its fields are start_date / end_date / description (labelled
// "Reason"), and the employee defaults to the caller.
const wfhModel = "serp_attendance.wfh_request"

// WFHRequestedMsg answers the create. ID is set whenever a record exists — including the
// case where the create landed and the submit did not, since a second attempt would file
// the same days twice.
type WFHRequestedMsg struct {
	ID  int
	Err error
}

// RequestWFH is a tea.Cmd: file a work-from-home request over the days given and submit it.
//
// Two calls, because create alone is not a request: state defaults to draft — the ERP calls
// it "To Submit" — and what refused the check in asked for a *submitted* one, so
// action_confirm is the second half of filing rather than a nicety. Nothing retries: a
// timed-out create that in fact landed would file the same days twice, and a duplicate
// request is a conversation with HR.
func RequestWFH(key, login, db string, employee int, from, to, reason string) tea.Cmd {
	return func() tea.Msg {
		fail := func(err error) tea.Msg { return WFHRequestedMsg{Err: err} }

		reason = strings.TrimSpace(oneLine(reason))
		if reason == "" {
			// description is required in the ERP, so this would be one round trip to be told
			// the same thing.
			return fail(errors.New("say why you are working from home"))
		}
		start, err := time.Parse(parse.DateLayout, strings.TrimSpace(from))
		if err != nil {
			return fail(fmt.Errorf("unreadable date %q", from))
		}
		end, err := time.Parse(parse.DateLayout, strings.TrimSpace(to))
		if err != nil {
			return fail(fmt.Errorf("unreadable date %q", to))
		}
		if end.Before(start) {
			start, end = end, start // typed backwards is still a range
		}

		uid, err := connect(db, login, key)
		if err != nil {
			return fail(err)
		}

		vals := map[string]any{
			"start_date":  start.Format("2006-01-02"),
			"end_date":    end.Format("2006-01-02"),
			"description": reason,
		}
		if employee != 0 {
			// The same hr.employee the clock was read for. The field defaults to the caller's
			// own record anyway, so this only saves the default a lookup — it can never name
			// somebody else.
			vals["employee_id"] = employee
		}

		raw, err := rpc("object", "execute_kw", []any{
			db, uid, key, wfhModel, "create", []any{vals},
			map[string]any{"context": map[string]any{"lang": "en_US", "tz": "Asia/Dhaka"}},
		})
		if err != nil {
			return fail(err)
		}
		var id int
		if err := json.Unmarshal(raw, &id); err != nil || id == 0 {
			return fail(errors.New("the ERP refused the request"))
		}

		if _, err := rpc("object", "execute_kw", []any{
			db, uid, key, wfhModel, "action_confirm", []any{[]int{id}},
			map[string]any{"context": map[string]any{"lang": "en_US"}},
		}); err != nil {
			// The record exists, so the id goes back with the error: re-filing would book the
			// same days twice, and a draft is something HR can still see and approve.
			return WFHRequestedMsg{ID: id,
				Err: fmt.Errorf("filed as a draft, not submitted: %w", err)}
		}
		return WFHRequestedMsg{ID: id}
	}
}
