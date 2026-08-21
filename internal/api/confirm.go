package api

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// HoursConfirmedMsg answers a confirm: how many of the month's lines were sent, and whether
// there was anything left to send at all.
type HoursConfirmedMsg struct {
	Month string // the first of the month it was for, YYYY-MM-01, so a late answer is placeable
	Count int
	Err   error
}

// ConfirmHours is a tea.Cmd: tell the ERP that a month's own hour logs are done.
//
// Two calls. account.analytic.line.confirm_hour_logs is a **recordset** method taking no
// arguments — the lines go in execute_kw's ids slot — and it sets the `confirmed` boolean on
// each, so the ids have to be read first. The domain is the key owner's own lines in the month,
// exactly as the table's read is: confirming somebody else's hours is not a thing this screen
// should be able to do by accident.
//
// **The return value is not a success signal.** The method answered `false` on a line it had in
// fact just written, so only an RPC error means failure here; the month is re-read afterwards
// rather than trusted. Nothing retries — a second call would be harmless, but a timed-out one
// that landed makes the count a lie.
func ConfirmHours(key, login, db string, month time.Time) tea.Cmd {
	return func() tea.Msg {
		first := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
		msg := HoursConfirmedMsg{Month: first.Format("2006-01-02")}
		fail := func(err error) tea.Msg { msg.Err = err; return msg }

		uid, err := connect(strings.TrimSpace(db), strings.TrimSpace(login), strings.TrimSpace(key))
		if err != nil {
			return fail(err)
		}

		raw, err := rpc("object", "execute_kw", []any{
			db, uid, key, "account.analytic.line", "search_read",
			[]any{[]any{
				[]any{"user_id", "=", uid},
				[]any{"date", ">=", first.Format("2006-01-02")},
				[]any{"date", "<=", first.AddDate(0, 1, -1).Format("2006-01-02")},
				[]any{"confirmed", "=", false},
			}},
			map[string]any{"fields": []string{"id"}},
		})
		if err != nil {
			return fail(err)
		}
		var rows []struct {
			ID int `json:"id"`
		}
		if err := json.Unmarshal(raw, &rows); err != nil {
			return fail(errors.New("bad hour log result"))
		}
		if len(rows) == 0 {
			return msg // nothing left to confirm, which is not a failure
		}

		ids := make([]int, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.ID)
		}
		if _, err := rpc("object", "execute_kw", []any{
			db, uid, key, "account.analytic.line", "confirm_hour_logs",
			[]any{ids},
			map[string]any{"context": map[string]any{"lang": "en_US", "tz": "Asia/Dhaka"}},
		}); err != nil {
			return fail(err)
		}
		msg.Count = len(ids)
		return msg
	}
}
