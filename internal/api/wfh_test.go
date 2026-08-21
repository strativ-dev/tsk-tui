package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeWFH answers the login, the create and the submit, and records every call: the two-step
// shape is the thing worth holding — a created request sits in draft until action_confirm.
func fakeWFH(t *testing.T, calls *[]map[string]any, confirmFails bool) {
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
		result := `true`
		if req.Params.Method == "login" {
			result = `26`
		}
		if len(req.Params.Args) >= 5 {
			switch method, _ := req.Params.Args[4].(string); method {
			case "create":
				result = `1384`
			case "action_confirm":
				if confirmFails {
					_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":200,` +
						`"message":"Odoo Server Error","data":{"message":"no manager set"}}}`))
					return
				}
			}
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":` + result + `}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv(BaseEnv, srv.URL)
}

func TestRequestWFH(t *testing.T) {
	var calls []map[string]any
	fakeWFH(t, &calls, false)

	msg, ok := RequestWFH("k", "user@example.com", "db", 16,
		"24/08/26", "26/08/26", "  Working from\nhome  ")().(WFHRequestedMsg)
	if !ok {
		t.Fatal("RequestWFH did not return WFHRequestedMsg")
	}
	if msg.Err != nil || msg.ID != 1384 {
		t.Fatalf("msg = %+v", msg)
	}

	vals := wfhCreateVals(t, calls)
	for k, want := range map[string]any{
		"employee_id": 16.0,
		"start_date":  "2026-08-24",
		"end_date":    "2026-08-26",
		"description": "Working from home", // one line: the ERP's own text field
	} {
		if vals[k] != want {
			t.Errorf("%s = %v (%T), want %v", k, vals[k], vals[k], want)
		}
	}
	// state is computed by the module and stepped with action_confirm; sending it would set a
	// status the workflow never agreed to.
	if _, sent := vals["state"]; sent {
		t.Errorf("create sends state: %v", vals)
	}

	// Create alone leaves it in draft — "To Submit" — and the refusal that opened the line
	// asked for a submitted request, so the confirm is half of filing one.
	if !wfhConfirmed(calls, 1384) {
		t.Errorf("the request was created but never submitted: %v", calls)
	}
}

// A range typed backwards is still a range, and a request with no employee in hand lets Odoo
// work out who is asking rather than being sent a zero.
func TestRequestWFHOrdersTheRange(t *testing.T) {
	var calls []map[string]any
	fakeWFH(t, &calls, false)
	RequestWFH("k", "u@e.com", "db", 0, "26/08/26", "24/08/26", "why")()

	vals := wfhCreateVals(t, calls)
	if vals["start_date"] != "2026-08-24" || vals["end_date"] != "2026-08-26" {
		t.Errorf("dates = %v → %v", vals["start_date"], vals["end_date"])
	}
	if _, sent := vals["employee_id"]; sent {
		t.Errorf("employee_id 0 was sent: %v", vals)
	}
}

// A create that landed and a submit that did not is still a record: the id goes back with the
// error, or the next attempt asks HR for the same days twice.
func TestRequestWFHKeepsTheIDWhenTheSubmitFails(t *testing.T) {
	fakeWFH(t, nil, true)
	msg := RequestWFH("k", "u@e.com", "db", 16, "24/08/26", "24/08/26", "why")().(WFHRequestedMsg)
	if msg.Err == nil {
		t.Fatal("a failed submit was reported as success")
	}
	if msg.ID != 1384 {
		t.Errorf("ID = %d, want the record that exists", msg.ID)
	}
}

func TestRequestWFHRefusals(t *testing.T) {
	var calls []map[string]any
	fakeWFH(t, &calls, false)
	for _, c := range []struct {
		what           string
		from, to       string
		reason         string
		key, login, db string
	}{
		{"no reason", "24/08/26", "24/08/26", "   ", "k", "u@e.com", "db"},
		{"unreadable start", "the 24th", "24/08/26", "why", "k", "u@e.com", "db"},
		{"unreadable end", "24/08/26", "next week", "why", "k", "u@e.com", "db"},
		{"no key", "24/08/26", "24/08/26", "why", "", "u@e.com", "db"},
		{"no db", "24/08/26", "24/08/26", "why", "k", "u@e.com", ""},
	} {
		msg := RequestWFH(c.key, c.login, c.db, 16, c.from, c.to, c.reason)().(WFHRequestedMsg)
		if msg.Err == nil {
			t.Errorf("%s was accepted", c.what)
		}
		if msg.ID != 0 {
			t.Errorf("%s came back with an id: %+v", c.what, msg)
		}
	}
	// A refusal made here costs no round trip, the same rule the hour log follows.
	for _, c := range calls {
		if args, _ := c["args"].([]any); len(args) >= 5 && args[4] == "create" {
			t.Errorf("a refused request was still sent: %v", c)
		}
	}
}

func wfhCreateVals(t *testing.T, calls []map[string]any) map[string]any {
	t.Helper()
	for _, c := range calls {
		args, _ := c["args"].([]any)
		if len(args) < 6 || args[3] != wfhModel || args[4] != "create" {
			continue
		}
		vals, _ := args[5].([]any)
		if len(vals) == 0 {
			t.Fatalf("create was called with no values: %v", args[5])
		}
		out, ok := vals[0].(map[string]any)
		if !ok {
			t.Fatalf("create values are %T", vals[0])
		}
		return out
	}
	t.Fatalf("no %s create was made", wfhModel)
	return nil
}

// wfhConfirmed says whether action_confirm was called on the id the create returned.
func wfhConfirmed(calls []map[string]any, id int) bool {
	for _, c := range calls {
		args, _ := c["args"].([]any)
		if len(args) < 6 || args[3] != wfhModel || args[4] != "action_confirm" {
			continue
		}
		ids, _ := args[5].([]any)
		if len(ids) == 0 {
			continue
		}
		inner, _ := ids[0].([]any)
		if len(inner) == 1 && inner[0] == float64(id) {
			return true
		}
	}
	return false
}
