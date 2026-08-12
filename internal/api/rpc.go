package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tasnimAlam/tsk/internal/parse"
	"github.com/tasnimAlam/tsk/internal/store"
)

// DBEnv names the Odoo database. The REST API does not need it, but JSON-RPC
// does, and the server refuses to list its databases.
const DBEnv = "TSK_ODOO_DB"

// ErrNoDB means timesheet lines cannot be read yet: JSON-RPC needs a database.
var ErrNoDB = errors.New("no Odoo database — export " + DBEnv + " to read timesheet lines")

// DB is the Odoo database name, or "" when unset.
func DB() string { return strings.TrimSpace(os.Getenv(DBEnv)) }

// EntriesMsg carries one task's timesheet lines, straight from Odoo.
type EntriesMsg struct {
	TaskID int
	Rows   []store.Entry
	Err    error
}

// FetchEntries is a tea.Cmd: log in for a uid, then search_read the task's
// account.analytic.line rows, newest first.
func FetchEntries(key, login string, taskID int) tea.Cmd {
	return func() tea.Msg {
		key, login = strings.TrimSpace(key), strings.TrimSpace(login)
		switch {
		case key == "":
			return EntriesMsg{TaskID: taskID, Err: ErrNoKey}
		case DB() == "":
			return EntriesMsg{TaskID: taskID, Err: ErrNoDB}
		case login == "":
			return EntriesMsg{TaskID: taskID, Err: errors.New("unknown Odoo login — sync first")}
		}

		uid, err := login_(login, key)
		if err != nil {
			return EntriesMsg{TaskID: taskID, Err: err}
		}

		raw, err := rpc("object", "execute_kw", []any{
			DB(), uid, key,
			"account.analytic.line", "search_read",
			[]any{[]any{[]any{"task_id", "=", taskID}}},
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

// login_ trades the API key for a uid. Named with a trailing underscore so it
// does not shadow the login argument elsewhere in the package.
func login_(user, key string) (int, error) {
	raw, err := rpc("common", "login", []any{DB(), user, key})
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
