package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The project read: the ERP's own list domain and order, and team_ids resolved to names by a
// second call — a many2many comes back as bare ids, so nothing on project.project names them.
func TestFetchProjects(t *testing.T) {
	const projects = `[{
		"id": 850,
		"display_name": "AI Sales",
		"user_id": [508, "Sofia Wannerheim"],
		"team_ids": [125],
		"task_count": 9
	}, {
		"id": 849,
		"display_name": "AI Transformation",
		"user_id": [7, "Reaz Abedin"],
		"team_ids": [75, 126],
		"task_count": 181
	}, {
		"id": 858,
		"display_name": "Boo Företagsportalen - Underhåll",
		"user_id": false,
		"team_ids": [],
		"task_count": 0
	}, {
		"id": 787,
		"display_name": "Value-Driven\nEngagement",
		"user_id": [26, "Md. Tasnim Alam"],
		"team_ids": [9, 404],
		"task_count": 753
	}]`
	// 404 is an id the caller cannot read, which is why the teams are a search_read: it comes
	// back missing rather than raising, and the project is still on the list.
	const teams = `[
		{"id": 125, "name": "AI Sales", "user_ids": [516]},
		{"id": 75, "name": "UX/UI Designers", "user_ids": [570, 24, 104]},
		{"id": 126, "name": "AI implementation", "user_ids": [570, 7, 26]},
		{"id": 9, "name": "Strativ dev\tteam", "user_ids": [7]}
	]`

	var calls []string
	var projDomain []any
	var projKwargs, teamKwargs map[string]any
	var teamDomain []any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Params struct {
				Method string `json:"method"`
				Args   []any  `json:"args"`
			} `json:"params"`
		}
		_ = json.Unmarshal(body, &req)
		w.Header().Set("Content-Type", "application/json")

		if req.Params.Method == "login" {
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":26}`))
			return
		}
		model, _ := req.Params.Args[3].(string)
		calls = append(calls, model)
		domain, kwargs := []any(nil), map[string]any(nil)
		if len(req.Params.Args) >= 7 {
			if inner, ok := req.Params.Args[5].([]any); ok && len(inner) > 0 {
				domain, _ = inner[0].([]any)
			}
			kwargs, _ = req.Params.Args[6].(map[string]any)
		}
		result := teams
		if model == "project.project" {
			result, projDomain, projKwargs = projects, domain, kwargs
		} else {
			teamDomain, teamKwargs = domain, kwargs
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":` + result + `}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv(BaseEnv, srv.URL)

	msg, ok := FetchProjects("k", "user@example.com", "db")().(ProjectsMsg)
	if !ok {
		t.Fatal("FetchProjects did not return ProjectsMsg")
	}
	if msg.Err != nil {
		t.Fatalf("Err = %v", msg.Err)
	}

	// Two calls and no more: the teams are read once for the whole list, not once per project.
	if len(calls) != 2 || calls[0] != "project.project" || calls[1] != "serp_project.team" {
		t.Fatalf("calls = %v", calls)
	}

	// Open projects only, in the ERP's own order.
	open, _ := projDomain[0].([]any)
	if len(open) != 3 || open[0] != "is_closed" || open[2] != false {
		t.Errorf("domain = %v", projDomain)
	}
	if got := projKwargs["order"]; got != "sequence asc, name asc, id asc" {
		t.Errorf("order = %v", got)
	}
	// task_count, Odoo's own field, and not the serp one that also counts what is closed.
	fields, _ := projKwargs["fields"].([]any)
	var names []string
	for _, f := range fields {
		names = append(names, f.(string))
	}
	if joined := strings.Join(names, ","); !strings.Contains(joined, "task_count") ||
		strings.Contains(joined, "project_task_count") {
		t.Errorf("fields = %v", names)
	}
	// The manager comes with the list rather than costing a call: a many2one is an
	// [id, name] pair.
	if !strings.Contains(strings.Join(names, ","), "user_id") {
		t.Errorf("fields = %v, want the project manager among them", names)
	}

	// The teams are asked for by the ids the projects mentioned, each one once — 8 does not
	// appear at all here, and 9 appears in only one project.
	clause, _ := teamDomain[0].([]any)
	if len(clause) != 3 || clause[0] != "id" || clause[1] != "in" {
		t.Fatalf("team domain = %v", teamDomain)
	}
	ids, _ := clause[2].([]any)
	if len(ids) != 5 {
		t.Errorf("asked for %v, want the five distinct ids", ids)
	}
	if _, asked := teamKwargs["order"]; asked {
		t.Error("the teams were ordered, which the projects' own order already decides")
	}

	if len(msg.Projects) != 4 {
		t.Fatalf("%d projects", len(msg.Projects))
	}
	// Named, in the order the ERP listed them on the project.
	if p := msg.Projects[1]; p.Name != "AI Transformation" || p.Tasks != 181 ||
		p.Manager != "Reaz Abedin" ||
		strings.Join(p.Teams, ", ") != "UX/UI Designers, AI implementation" {
		t.Errorf("projects[1] = %+v", p)
	}
	// Mine is what the toggle reads: the login answered with uid 26, who is on this project's
	// second team. Nothing on project.project says "mine", so it is worked out here.
	if !msg.Projects[1].Mine {
		t.Error("a project whose team holds the key's owner is not marked mine")
	}
	if msg.Projects[0].Mine {
		t.Error("a project the key's owner is not on is marked mine")
	}
	// The manager counts too, not just the teams.
	if !msg.Projects[3].Mine {
		t.Error("a project the key's owner manages is not marked mine")
	}

	// The members are the union over the project's teams, in the order the ERP listed them and
	// each person once: 570 is on both of this project's teams and is one member.
	if got := msg.Projects[1].Members; len(got) != 5 ||
		got[0] != 570 || got[1] != 24 || got[2] != 104 || got[3] != 7 || got[4] != 26 {
		t.Errorf("projects[1] members = %v", got)
	}
	// No teams is no teams, not a gap — and no manager either, which Odoo sends as false.
	if p := msg.Projects[2]; len(p.Teams) != 0 || p.Tasks != 0 || p.Manager != "" ||
		len(p.Members) != 0 {
		t.Errorf("projects[2] = %+v", p)
	}
	// A team the caller cannot read is left out, and the project keeps the ones it can see.
	// Everything the ERP wrote is flattened to one line: a newline in a name would grow the
	// row it is drawn on.
	if p := msg.Projects[3]; strings.Join(p.Teams, ", ") != "Strativ dev team" {
		t.Errorf("projects[3] teams = %q", p.Teams)
	}
	for _, p := range msg.Projects {
		if strings.ContainsAny(p.Name, "\n\t") {
			t.Errorf("name %q carries a newline", p.Name)
		}
		for _, tm := range p.Teams {
			if strings.ContainsAny(tm, "\n\t") {
				t.Errorf("team %q carries a newline", tm)
			}
		}
	}
}

// The member read: one call, only the ids that project's own teams named, and search_read
// rather than read so a user the caller cannot see is left out instead of failing the table.
func TestFetchProjectMembers(t *testing.T) {
	const users = `[
		{"id": 557, "name": "Ashik Ahamed Aman Rafat", "email": "ashik.rafat@strativ.se"},
		{"id": 26, "name": "Md. Tasnim\nAlam", "email": "tasnim@strativ.se"},
		{"id": 31, "name": "Md. Toufiqur Rahman", "email": false}
	]`

	var model, method string
	var domain []any
	var kwargs map[string]any
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Params struct {
				Method string `json:"method"`
				Args   []any  `json:"args"`
			} `json:"params"`
		}
		_ = json.Unmarshal(body, &req)
		w.Header().Set("Content-Type", "application/json")
		if req.Params.Method == "login" {
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":26}`))
			return
		}
		calls++
		model, _ = req.Params.Args[3].(string)
		method, _ = req.Params.Args[4].(string)
		if inner, ok := req.Params.Args[5].([]any); ok && len(inner) > 0 {
			domain, _ = inner[0].([]any)
		}
		kwargs, _ = req.Params.Args[6].(map[string]any)
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":` + users + `}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv(BaseEnv, srv.URL)

	msg, ok := FetchProjectMembers("k", "user@example.com", "db", 849,
		[]int{557, 26, 31, 999})().(ProjectMembersMsg)
	if !ok {
		t.Fatal("FetchProjectMembers did not return ProjectMembersMsg")
	}
	if msg.Err != nil {
		t.Fatalf("Err = %v", msg.Err)
	}
	if msg.ID != 849 {
		t.Errorf("ID = %d, want the project it was asked about", msg.ID)
	}
	if calls != 1 || model != "res.users" || method != "search_read" {
		t.Fatalf("%d calls, %s.%s", calls, model, method)
	}
	clause, _ := domain[0].([]any)
	if len(clause) != 3 || clause[0] != "id" || clause[1] != "in" {
		t.Errorf("domain = %v", domain)
	}
	// A table somebody reads down is sorted by name, not by the order two teams' ids merged in.
	if got := kwargs["order"]; got != "name asc" {
		t.Errorf("order = %v", got)
	}

	if len(msg.Members) != 3 {
		t.Fatalf("members = %+v", msg.Members)
	}
	// Everything the ERP wrote is flattened to one line, and an empty email is empty rather
	// than the false Odoo sends for it — a false into a string fails the whole table.
	if m := msg.Members[1]; m.Name != "Md. Tasnim Alam" || m.Email != "tasnim@strativ.se" {
		t.Errorf("members[1] = %+v", m)
	}
	if m := msg.Members[2]; m.Email != "" {
		t.Errorf("members[2] email = %q", m.Email)
	}

	// No ids, no call: a project whose teams have nobody on them costs nothing.
	calls = 0
	if empty := FetchProjectMembers("k", "user@example.com", "db", 858, nil)().(ProjectMembersMsg); //
	empty.Err != nil || len(empty.Members) != 0 || calls != 0 {
		t.Errorf("an empty member list made %d calls: %+v", calls, empty)
	}
}
