package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tasnimAlam/tsk/internal/parse"
	"github.com/tasnimAlam/tsk/internal/store"
)

// ErrNoDB means the Odoo database name is missing, so JSON-RPC cannot be called.
// The name is treated as a secret: it lives in the pass entry beside the key, or
// in $TSK_ODOO_DB, never in this repo. See store.LoadKey.
var ErrNoDB = errors.New("no Odoo database — add a `db:` line to the pass entry, or export " + store.DBEnv)

// EntriesMsg carries one task's timesheet lines, straight from Odoo.
type EntriesMsg struct {
	TaskID int
	Rows   []store.Entry
	Err    error
}

// FetchEntries is a tea.Cmd: log in for a uid, then search_read the task's
// account.analytic.line rows, newest first. db comes from the credential store,
// not from this package.
func FetchEntries(key, login, db string, taskID int) tea.Cmd {
	return func() tea.Msg {
		key, login, db = strings.TrimSpace(key), strings.TrimSpace(login), strings.TrimSpace(db)
		uid, err := connect(db, login, key)
		if err != nil {
			return EntriesMsg{TaskID: taskID, Err: err}
		}

		// Mine only. Odoo's own task form lists every employee's lines, but this is a
		// personal hour log: a colleague's hours in your table would inflate the task
		// total and the day's progress bar, and you could not edit them anyway.
		raw, err := rpc("object", "execute_kw", []any{
			db, uid, key,
			"account.analytic.line", "search_read",
			[]any{[]any{
				[]any{"task_id", "=", taskID},
				[]any{"user_id", "=", uid},
			}},
			map[string]any{
				"fields": []string{"date", "name", "unit_amount"},
				"order":  "date desc, id desc", // Rows are newest first
			},
		})
		if err != nil {
			return EntriesMsg{TaskID: taskID, Err: err}
		}

		var lines []struct {
			ID         int      `json:"id"`
			Date       odooText `json:"date"`
			Name       odooText `json:"name"`
			UnitAmount float64  `json:"unit_amount"`
		}
		if err := json.Unmarshal(raw, &lines); err != nil {
			return EntriesMsg{TaskID: taskID, Err: fmt.Errorf("bad search_read result: %w", err)}
		}

		rows := make([]store.Entry, 0, len(lines))
		for _, l := range lines {
			rows = append(rows, store.Entry{
				ID:      l.ID,
				Date:    isoToStored(string(l.Date)),
				Desc:    strings.TrimSpace(string(l.Name)),
				Minutes: int(math.Round(l.UnitAmount * 60)),
			})
		}
		return EntriesMsg{TaskID: taskID, Rows: rows}
	}
}

// UpdatedMsg and DeletedMsg report what the ERP did with a line it already owns.
type UpdatedMsg struct {
	TaskID  int
	EntryID int
	Minutes int
	Err     error
}

type DeletedMsg struct {
	TaskID  int
	EntryID int
	Err     error
}

// UpdateEntry writes an existing line: `write` on account.analytic.line. The REST
// API only creates, but the MCP and RPC both allow write, so an edit here is a real
// edit in the ERP.
func UpdateEntry(key, login, db string, taskID, entryID int, date, desc string, minutes int) tea.Cmd {
	return func() tea.Msg {
		fail := func(err error) tea.Msg {
			return UpdatedMsg{TaskID: taskID, EntryID: entryID, Err: err}
		}
		if entryID <= 0 {
			return fail(errors.New("that row is not in the ERP yet"))
		}
		vals, err := entryVals(date, desc, minutes)
		if err != nil {
			return fail(err)
		}
		uid, err := connect(db, login, key)
		if err != nil {
			return fail(err)
		}

		raw, err := rpc("object", "execute_kw", []any{
			db, uid, key,
			"account.analytic.line", "write",
			[]any{[]int{entryID}, vals},
		})
		if err != nil {
			return fail(err)
		}
		var ok bool
		if err := json.Unmarshal(raw, &ok); err != nil || !ok {
			return fail(errors.New("the ERP refused the edit"))
		}
		return UpdatedMsg{TaskID: taskID, EntryID: entryID, Minutes: minutes}
	}
}

// DeleteEntry removes a line: `unlink` on account.analytic.line. The MCP blocks
// unlink, but the model-level ACL over RPC allows it; a record rule may still
// refuse someone else's line, which surfaces as an error rather than a silent no-op.
func DeleteEntry(key, login, db string, taskID, entryID int) tea.Cmd {
	return func() tea.Msg {
		fail := func(err error) tea.Msg {
			return DeletedMsg{TaskID: taskID, EntryID: entryID, Err: err}
		}
		if entryID <= 0 {
			return fail(errors.New("that row is not in the ERP"))
		}
		uid, err := connect(db, login, key)
		if err != nil {
			return fail(err)
		}

		raw, err := rpc("object", "execute_kw", []any{
			db, uid, key,
			"account.analytic.line", "unlink",
			[]any{[]int{entryID}},
		})
		if err != nil {
			return fail(err)
		}
		var ok bool
		if err := json.Unmarshal(raw, &ok); err != nil || !ok {
			return fail(errors.New("the ERP refused the delete"))
		}
		return DeletedMsg{TaskID: taskID, EntryID: entryID}
	}
}

// entryVals is the write payload: unit_amount is hours, name is the description.
func entryVals(date, desc string, minutes int) (map[string]any, error) {
	desc = strings.TrimSpace(desc)
	switch {
	case desc == "":
		return nil, errors.New("the ERP requires a description")
	case minutes <= 0 || minutes > 24*60:
		return nil, fmt.Errorf("%s is out of range, the ERP takes 0 to 24h per entry",
			parse.FormatTotal(minutes))
	}
	day, err := time.Parse(parse.DateLayout, strings.TrimSpace(date))
	if err != nil {
		return nil, fmt.Errorf("unreadable date %q", date)
	}
	return map[string]any{
		"name":        desc,
		"date":        day.Format("2006-01-02"),
		"unit_amount": float64(minutes) / 60,
	}, nil
}

// connect checks what every RPC call needs and returns the uid to call with.
func connect(db, login, key string) (int, error) {
	switch {
	case strings.TrimSpace(key) == "":
		return 0, ErrNoKey
	case strings.TrimSpace(db) == "":
		return 0, ErrNoDB
	case strings.TrimSpace(login) == "":
		return 0, errors.New("unknown Odoo login — sync first")
	}
	return login_(strings.TrimSpace(db), strings.TrimSpace(login), strings.TrimSpace(key))
}

// login_ trades the API key for a uid. Named with a trailing underscore so it
// does not shadow the login argument elsewhere in the package.
func login_(db, user, key string) (int, error) {
	raw, err := rpc("common", "login", []any{db, user, key})
	if err != nil {
		return 0, err
	}
	var uid int
	if err := json.Unmarshal(raw, &uid); err != nil || uid == 0 {
		return 0, ErrUnauthorized
	}
	return uid, nil
}

// rpc posts one JSON-RPC call. The key travels in the body because the protocol
// puts it there; it is never logged and never quoted back in an error.
func rpc(service, method string, args []any) (json.RawMessage, error) {
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "call",
		"id":      1,
		"params": map[string]any{
			"service": service,
			"method":  method,
			"args":    args,
		},
	})
	if err != nil {
		return nil, err
	}

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Post(
		BaseURL()+"/jsonrpc", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("cannot reach " + BaseURL())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jsonrpc %s", resp.Status)
	}

	var out struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
			Data    struct {
				Name    string `json:"name"`
				Message string `json:"message"`
			} `json:"data"`
		} `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("bad jsonrpc response: %w", err)
	}
	if out.Error != nil {
		if strings.Contains(out.Error.Data.Name, "AccessDenied") {
			return nil, ErrUnauthorized
		}
		msg := out.Error.Data.Message
		if msg == "" {
			msg = out.Error.Message
		}
		return nil, errors.New("odoo: " + firstLine(msg))
	}
	return out.Result, nil
}

// odooText reads a string field that Odoo renders as false when empty.
type odooText string

func (o *odooText) UnmarshalJSON(b []byte) error {
	switch {
	case bytes.Equal(b, []byte("false")), bytes.Equal(b, []byte("null")):
		*o = ""
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*o = odooText(s)
	return nil
}

// isoToStored turns Odoo's YYYY-MM-DD into the dd/mm/yy the model stores.
func isoToStored(iso string) string {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(iso))
	if err != nil {
		return iso
	}
	return t.Format(parse.DateLayout)
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	if len(line) > 120 {
		line = line[:120] + "…"
	}
	return line
}
