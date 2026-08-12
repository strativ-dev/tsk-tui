package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeOdoo answers common.login with uid 26 and execute_kw with the given lines.
func fakeOdoo(t *testing.T, lines string, calls *[]map[string]any) {
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
		switch req.Params.Method {
		case "login":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":26}`))
		case "execute_kw":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":` + lines + `}`))
		default:
			t.Errorf("unexpected method %q", req.Params.Method)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv(BaseEnv, srv.URL)
}

func TestFetchEntries(t *testing.T) {
	const lines = `[
		{"id": 90211, "date": "2026-08-12", "name": "Retry backoff", "unit_amount": 2.5},
		{"id": 90210, "date": "2026-08-11", "name": false, "unit_amount": 1.25}
	]`
	var calls []map[string]any
	fakeOdoo(t, lines, &calls)

	msg, ok := FetchEntries("secret-key", "user@example.com", "erp-test", 1372)().(EntriesMsg)
	if !ok {
		t.Fatal("FetchEntries did not return EntriesMsg")
	}
	if msg.Err != nil {
		t.Fatalf("Err = %v", msg.Err)
	}
	if msg.TaskID != 1372 {
		t.Errorf("TaskID = %d, want 1372", msg.TaskID)
	}
	if len(msg.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(msg.Rows))
	}
	if got := msg.Rows[0]; got.ID != 90211 || got.Date != "12/08/26" || got.Desc != "Retry backoff" || got.Minutes != 150 {
		t.Errorf("rows[0] = %+v", got)
	}
	if got := msg.Rows[1]; got.Date != "11/08/26" || got.Desc != "" || got.Minutes != 75 {
		t.Errorf("rows[1] = %+v — Odoo sends false for an empty description", got)
	}

	// Login first, then a search_read filtered on the numeric task id.
	if len(calls) != 2 {
		t.Fatalf("made %d calls, want login + execute_kw", len(calls))
	}
	if calls[0]["service"] != "common" || calls[1]["service"] != "object" {
		t.Errorf("services = %v, %v", calls[0]["service"], calls[1]["service"])
	}
	args := calls[1]["args"].([]any)
	if uid, okNum := args[1].(float64); !okNum || uid != 26 {
		t.Errorf("uid arg = %v (%T), want the number 26 from login", args[1], args[1])
	}
	if args[3] != "account.analytic.line" || args[4] != "search_read" {
		t.Errorf("model/method = %v %v", args[3], args[4])
	}
	// Scoped to the caller: the task form in Odoo shows every employee's lines,
	// this table shows only the ones you logged.
	domain, _ := json.Marshal(args[5])
	if want := `[[["task_id","=",1372],["user_id","=",26]]]`; string(domain) != want {
		t.Errorf("domain = %s, want %s", domain, want)
	}
}

func TestFetchEntriesRefusals(t *testing.T) {
	fakeOdoo(t, `[]`, nil)

	if got := FetchEntries("", "a@b.c", "erp-test", 1)().(EntriesMsg); !errors.Is(got.Err, ErrNoKey) {
		t.Errorf("no key: %v, want ErrNoKey", got.Err)
	}
	if got := FetchEntries("k", "", "erp-test", 1)().(EntriesMsg); got.Err == nil {
		t.Error("no login: nil error, want a complaint")
	}
}

// writeServer answers login, then one execute_kw returning result, recording args.
func writeServer(t *testing.T, result string, args *[]any) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Params struct {
				Method string `json:"method"`
				Args   []any  `json:"args"`
			} `json:"params"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)

		if req.Params.Method == "login" {
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":26}`))
			return
		}
		if args != nil {
			*args = req.Params.Args
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":` + result + `}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv(BaseEnv, srv.URL)
}

func TestUpdateEntry(t *testing.T) {
	var args []any
	writeServer(t, "true", &args)

	msg := UpdateEntry("k", "u@e.com", "db", 1372, 141605, "12/08/26", "fixed desc", 150)().(UpdatedMsg)
	if msg.Err != nil {
		t.Fatalf("Err = %v", msg.Err)
	}
	if msg.EntryID != 141605 || msg.Minutes != 150 || msg.TaskID != 1372 {
		t.Errorf("msg = %+v", msg)
	}
	if args[3] != "account.analytic.line" || args[4] != "write" {
		t.Fatalf("called %v %v, want write on account.analytic.line", args[3], args[4])
	}
	// [[id], {name, date, unit_amount}] — unit_amount is hours, name the description.
	payload, _ := json.Marshal(args[5])
	if want := `[[141605],{"date":"2026-08-12","name":"fixed desc","unit_amount":2.5}]`; string(payload) != want {
		t.Errorf("write args = %s, want %s", payload, want)
	}

	// A row the ERP does not have cannot be written, and never reaches the server.
	if got := UpdateEntry("k", "u@e.com", "db", 1, -1, "12/08/26", "d", 60)().(UpdatedMsg); got.Err == nil {
		t.Error("local row: nil error, want a refusal")
	}
	// Odoo returning false rather than an error still means it did not happen.
	writeServer(t, "false", nil)
	if got := UpdateEntry("k", "u@e.com", "db", 1, 5, "12/08/26", "d", 60)().(UpdatedMsg); got.Err == nil {
		t.Error("write returned false but the app called it a success")
	}
}

func TestDeleteEntry(t *testing.T) {
	var args []any
	writeServer(t, "true", &args)

	msg := DeleteEntry("k", "u@e.com", "db", 1372, 141605)().(DeletedMsg)
	if msg.Err != nil {
		t.Fatalf("Err = %v", msg.Err)
	}
	if args[4] != "unlink" {
		t.Errorf("method = %v, want unlink", args[4])
	}
	ids, _ := json.Marshal(args[5])
	if want := `[[141605]]`; string(ids) != want {
		t.Errorf("unlink args = %s, want %s", ids, want)
	}

	// A record rule refusing someone else's line must surface, not pass silently.
	writeServer(t, "false", nil)
	if got := DeleteEntry("k", "u@e.com", "db", 1, 5)().(DeletedMsg); got.Err == nil {
		t.Error("unlink returned false but the app called it a success")
	}
}

func TestWritesNeedCredentials(t *testing.T) {
	t.Setenv(BaseEnv, "http://127.0.0.1:1")
	if got := UpdateEntry("", "u@e.com", "db", 1, 5, "12/08/26", "d", 60)().(UpdatedMsg); !errors.Is(got.Err, ErrNoKey) {
		t.Errorf("no key: %v, want ErrNoKey", got.Err)
	}
	if got := DeleteEntry("k", "u@e.com", "", 1, 5)().(DeletedMsg); !errors.Is(got.Err, ErrNoDB) {
		t.Errorf("no db: %v, want ErrNoDB", got.Err)
	}
	if got := UpdateEntry("k", "u@e.com", "db", 1, 5, "12/08/26", "  ", 60)().(UpdatedMsg); got.Err == nil {
		t.Error("blank description: nil error, want a refusal")
	}
	if got := UpdateEntry("k", "u@e.com", "db", 1, 5, "12/08/26", "d", 25*60)().(UpdatedMsg); got.Err == nil {
		t.Error("25h: nil error, want a refusal")
	}
}

// A rejected key must surface as ErrUnauthorized so the app reprompts.
func TestRPCAccessDenied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":200,"message":"Odoo Server Error",
			"data":{"name":"odoo.exceptions.AccessDenied","message":"Access Denied"}}}`))
	}))
	defer srv.Close()
	t.Setenv(BaseEnv, srv.URL)

	if got := FetchEntries("stale", "a@b.c", "erp-test", 1)().(EntriesMsg); !errors.Is(got.Err, ErrUnauthorized) {
		t.Errorf("Err = %v, want ErrUnauthorized", got.Err)
	}
}
