package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// fakeConfirm answers the login, the search for the month's lines and the confirm itself. The
// lines it reports are what the confirm has to be called with.
func fakeConfirm(t *testing.T, calls *[]map[string]any, lines string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Params struct {
				Method string `json:"method"`
				Args   []any  `json:"args"`
			} `json:"params"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("unparsable request: %v", err)
		}
		if calls != nil {
			*calls = append(*calls, map[string]any{
				"method": req.Params.Method, "args": req.Params.Args,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		result := `false` // what confirm_hour_logs itself answers, even when it wrote
		if req.Params.Method == "login" {
			result = `26`
		}
		if len(req.Params.Args) >= 5 {
			if method, _ := req.Params.Args[4].(string); method == "search_read" {
				result = lines
			}
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":` + result + `}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv(BaseEnv, srv.URL)
}

func TestConfirmHours(t *testing.T) {
	var calls []map[string]any
	fakeConfirm(t, &calls, `[{"id": 142740}, {"id": 142741}]`)

	msg, ok := ConfirmHours("k", "user@example.com", "db",
		time.Date(2026, time.August, 15, 0, 0, 0, 0, time.UTC))().(HoursConfirmedMsg)
	if !ok {
		t.Fatal("ConfirmHours did not return HoursConfirmedMsg")
	}
	// A bare false from confirm_hour_logs is not a failure: it answered that on a line it had
	// just written, so only an RPC error means anything here.
	if msg.Err != nil {
		t.Fatalf("Err = %v", msg.Err)
	}
	if msg.Count != 2 || msg.Month != "2026-08-01" {
		t.Errorf("msg = %+v", msg)
	}

	var domain, ids []any
	for _, c := range calls {
		args, _ := c["args"].([]any)
		if len(args) < 6 {
			continue
		}
		if model, _ := args[3].(string); model != "account.analytic.line" {
			continue
		}
		switch method, _ := args[4].(string); method {
		case "search_read":
			inner, _ := args[5].([]any)
			if len(inner) > 0 {
				domain, _ = inner[0].([]any)
			}
		case "confirm_hour_logs":
			ids, _ = args[5].([]any)
		}
	}

	// The month's own lines, the caller's own, and only the ones not already confirmed.
	want := map[string]bool{"user_id": false, "date": false, "confirmed": false}
	for _, clause := range domain {
		parts, _ := clause.([]any)
		if len(parts) == 3 {
			if field, _ := parts[0].(string); field != "" {
				want[field] = true
			}
		}
	}
	for field, found := range want {
		if !found {
			t.Errorf("the domain does not name %s: %v", field, domain)
		}
	}

	// It is a recordset method taking no arguments: the lines go in the ids slot.
	if len(ids) != 1 {
		t.Fatalf("confirm_hour_logs args = %v, want the ids alone", ids)
	}
	got, _ := ids[0].([]any)
	if len(got) != 2 || got[0] != 142740.0 || got[1] != 142741.0 {
		t.Errorf("confirmed ids = %v", got)
	}
}

// A month with nothing left to confirm costs no write, and is not an error.
func TestConfirmHoursSkipsAConfirmedMonth(t *testing.T) {
	var calls []map[string]any
	fakeConfirm(t, &calls, `[]`)

	msg := ConfirmHours("k", "u@e.com", "db", time.Now())().(HoursConfirmedMsg)
	if msg.Err != nil || msg.Count != 0 {
		t.Errorf("msg = %+v", msg)
	}
	for _, c := range calls {
		args, _ := c["args"].([]any)
		if len(args) >= 5 && args[4] == "confirm_hour_logs" {
			t.Error("a month with nothing to confirm was still written to")
		}
	}
}

func TestConfirmHoursRefusals(t *testing.T) {
	fakeConfirm(t, nil, `[{"id": 1}]`)
	for _, c := range []struct{ what, key, login, db string }{
		{"no key", "", "u@e.com", "db"},
		{"no db", "k", "u@e.com", ""},
	} {
		if msg := ConfirmHours(c.key, c.login, c.db, time.Now())().(HoursConfirmedMsg); msg.Err == nil {
			t.Errorf("%s was accepted", c.what)
		}
	}
}
