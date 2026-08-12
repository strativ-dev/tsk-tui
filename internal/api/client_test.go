package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func fetch(t *testing.T, handler http.HandlerFunc, key string) TasksMsg {
	t.Helper()
	srv := httptest.NewServer(handler)
	defer srv.Close()
	t.Setenv(BaseEnv, srv.URL)

	msg := FetchTasks(key)()
	got, ok := msg.(TasksMsg)
	if !ok {
		t.Fatalf("FetchTasks returned %T, want TasksMsg", msg)
	}
	return got
}

func TestFetchTasks(t *testing.T) {
	const body = `{"count":2,"data":[
		{"id":1372,"key":"SE360-1372","name":"Add hour-log summary API","project":{"id":42,"name":"ERP 360"}},
		{"id":1401,"key":null,"name":"No project","project":null}
	]}`

	var gotAuth string
	got := fetch(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/api/v1/tasks/my" {
			t.Errorf("path = %q, want /api/v1/tasks/my", r.URL.Path)
		}
		if r.URL.RawQuery != "" {
			t.Errorf("query = %q, want the key nowhere but the header", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}, "secret-key")

	if got.Err != nil {
		t.Fatalf("Err = %v", got.Err)
	}
	if gotAuth != "Bearer secret-key" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if len(got.Tasks) != 2 {
		t.Fatalf("tasks = %d, want 2", len(got.Tasks))
	}
	// Key is its own field: logging hours needs it apart from the title.
	if g := got.Tasks[0]; g.Key != "SE360-1372" || g.Title != "Add hour-log summary API" || g.Tag != "ERP 360" {
		t.Errorf("tasks[0] = %+v", g)
	}
	if g := got.Tasks[1]; g.Key != "" || g.Title != "No project" || g.Tag != "" {
		t.Errorf("tasks[1] = %+v, want no key and no tag", g)
	}
}

func TestLogHours(t *testing.T) {
	var gotBody map[string]any
	var gotMethod, gotPath, gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotAuth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":90211,"task_key":"SE360-1372","date":"2026-08-12","hours":2.5,"description":"work"}`))
	}))
	defer srv.Close()
	t.Setenv(BaseEnv, srv.URL)

	msg, ok := LogHours("secret-key", "SE360-1372", "12/08/26", "work", 150, 1372, -1)().(LoggedMsg)
	if !ok {
		t.Fatal("LogHours did not return LoggedMsg")
	}
	if msg.Err != nil {
		t.Fatalf("Err = %v", msg.Err)
	}
	if msg.EntryID != 90211 || msg.Minutes != 150 || msg.TaskID != 1372 || msg.LocalID != -1 {
		t.Errorf("msg = %+v, want the created id and the row it belongs to", msg)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/timesheets/log" {
		t.Errorf("%s %s, want POST /api/v1/timesheets/log", gotMethod, gotPath)
	}
	if gotAuth != "Bearer secret-key" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	// The endpoint wants the task key, an ISO date, and decimal hours.
	if gotBody["task_key"] != "SE360-1372" || gotBody["date"] != "2026-08-12" ||
		gotBody["hours"] != 2.5 || gotBody["description"] != "work" {
		t.Errorf("body = %v", gotBody)
	}
}

func TestLogHoursRefusals(t *testing.T) {
	// These never reach the network: no server is running.
	t.Setenv(BaseEnv, "http://127.0.0.1:1")
	cases := map[string]LoggedMsg{
		"no key":      LogHours("", "K-1", "12/08/26", "d", 60, 1, -1)().(LoggedMsg),
		"no task key": LogHours("k", "", "12/08/26", "d", 60, 1, -1)().(LoggedMsg),
		"no desc":     LogHours("k", "K-1", "12/08/26", "  ", 60, 1, -1)().(LoggedMsg),
		"zero hours":  LogHours("k", "K-1", "12/08/26", "d", 0, 1, -1)().(LoggedMsg),
		"over a day":  LogHours("k", "K-1", "12/08/26", "d", 25*60, 1, -1)().(LoggedMsg),
		"bad date":    LogHours("k", "K-1", "not-a-date", "d", 60, 1, -1)().(LoggedMsg),
	}
	for name, got := range cases {
		if got.Err == nil {
			t.Errorf("%s: nil error, want a refusal before the request", name)
		}
		if got.LocalID != -1 || got.TaskID != 1 {
			t.Errorf("%s: msg = %+v, must still name the row so it can stay local", name, got)
		}
	}
}

// The documented error codes become sentences a person can act on.
func TestLogHoursServerErrors(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   string
	}{
		{400, `{"error":"x","code":"daily_cap_exceeded"}`, "past 24h"},
		{404, `{"error":"x","code":"task_not_found"}`, "does not know that task key"},
		{409, `{"error":"x","code":"task_ambiguous"}`, "more than one project"},
		{400, `{"error":"x","code":"no_hourly_cost"}`, "ask HR"},
		{500, `{"error":"boom"}`, "boom"},
	}
	for _, c := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(c.status)
			_, _ = w.Write([]byte(c.body))
		}))
		t.Setenv(BaseEnv, srv.URL)
		got := LogHours("k", "K-1", "12/08/26", "d", 60, 1, -1)().(LoggedMsg)
		srv.Close()

		if got.Err == nil || !strings.Contains(got.Err.Error(), c.want) {
			t.Errorf("status %d: err = %v, want it to mention %q", c.status, got.Err, c.want)
		}
	}

	// A 401 must be the shared sentinel, so the app reprompts for the key.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	t.Setenv(BaseEnv, srv.URL)
	if got := LogHours("k", "K-1", "12/08/26", "d", 60, 1, -1)().(LoggedMsg); !errors.Is(got.Err, ErrUnauthorized) {
		t.Errorf("401 gave %v, want ErrUnauthorized", got.Err)
	}
}

func TestFetchTasksErrors(t *testing.T) {
	got := fetch(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}, "stale-key")
	if !errors.Is(got.Err, ErrUnauthorized) {
		t.Errorf("401 gave %v, want ErrUnauthorized", got.Err)
	}

	got = fetch(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom","code":"oops"}`))
	}, "k")
	if got.Err == nil || errors.Is(got.Err, ErrUnauthorized) {
		t.Errorf("500 gave %v, want a plain error", got.Err)
	}

	var cmd tea.Cmd = FetchTasks("   ")
	if msg := cmd().(TasksMsg); !errors.Is(msg.Err, ErrNoKey) {
		t.Errorf("empty key gave %v, want ErrNoKey", msg.Err)
	}
}
