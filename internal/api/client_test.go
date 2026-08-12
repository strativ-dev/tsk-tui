package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
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
	if got.Tasks[0].Title != "SE360-1372 Add hour-log summary API" || got.Tasks[0].Tag != "ERP 360" {
		t.Errorf("tasks[0] = %+v", got.Tasks[0])
	}
	if got.Tasks[1].Title != "No project" || got.Tasks[1].Tag != "" {
		t.Errorf("tasks[1] = %+v, want no key prefix and no tag", got.Tasks[1])
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
