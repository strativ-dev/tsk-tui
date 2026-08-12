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
	t.Setenv(DBEnv, "erp-test")
}

func TestFetchEntries(t *testing.T) {
	const lines = `[
		{"id": 90211, "date": "2026-08-12", "name": "Retry backoff", "unit_amount": 2.5},
		{"id": 90210, "date": "2026-08-11", "name": false, "unit_amount": 1.25}
	]`
	var calls []map[string]any
	fakeOdoo(t, lines, &calls)

	msg, ok := FetchEntries("secret-key", "tasnim@strativ.se", 1372)().(EntriesMsg)
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
	domain, _ := json.Marshal(args[5])
	if want := `[[["task_id","=",1372]]]`; string(domain) != want {
		t.Errorf("domain = %s, want %s", domain, want)
	}
}

func TestFetchEntriesRefusals(t *testing.T) {
	fakeOdoo(t, `[]`, nil)

	if got := FetchEntries("", "a@b.c", 1)().(EntriesMsg); !errors.Is(got.Err, ErrNoKey) {
		t.Errorf("no key: %v, want ErrNoKey", got.Err)
	}
	if got := FetchEntries("k", "", 1)().(EntriesMsg); got.Err == nil {
		t.Error("no login: nil error, want a complaint")
	}
	t.Setenv(DBEnv, "")
	if got := FetchEntries("k", "a@b.c", 1)().(EntriesMsg); !errors.Is(got.Err, ErrNoDB) {
		t.Errorf("no db: %v, want ErrNoDB", got.Err)
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
	t.Setenv(DBEnv, "erp-test")

	if got := FetchEntries("stale", "a@b.c", 1)().(EntriesMsg); !errors.Is(got.Err, ErrUnauthorized) {
		t.Errorf("Err = %v, want ErrUnauthorized", got.Err)
	}
}
