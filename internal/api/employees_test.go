package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The directory read: the public model, the four fields the card shows, and Odoo's false for
// an empty one — half this office has no work_phone, and a false into a string would fail the
// whole list rather than the one row.
func TestFetchEmployees(t *testing.T) {
	const rows = `[
		{"id": 121, "name": "Abdul Alim Shohan", "job_designation": "Software Engineer - L3",
			"work_email": "abdul.shohan@strativ.se", "work_phone": false},
		{"id": 57, "name": "Jonna Persson", "job_designation": "",
			"work_email": "jonna@strativ.se", "work_phone": false}
	]`

	var calls []map[string]any
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
		calls = append(calls, map[string]any{"method": req.Params.Method, "args": req.Params.Args})

		w.Header().Set("Content-Type", "application/json")
		result := rows
		if req.Params.Method == "login" {
			result = `26`
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":` + result + `}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv(BaseEnv, srv.URL)

	msg, ok := FetchEmployees("k", "user@example.com", "db")().(EmployeesMsg)
	if !ok {
		t.Fatal("FetchEmployees did not return EmployeesMsg")
	}
	if msg.Err != nil {
		t.Fatalf("Err = %v", msg.Err)
	}
	if len(msg.Employees) != 2 {
		t.Fatalf("%d employees", len(msg.Employees))
	}
	if got := msg.Employees[0]; got.Name != "Abdul Alim Shohan" ||
		got.Job != "Software Engineer - L3" || got.Email != "abdul.shohan@strativ.se" ||
		got.Phone != "" {
		t.Errorf("employees[0] = %+v", got)
	}

	// hr.employee.public, not hr.employee: the private model refuses most of its fields to a
	// user who is not an HR officer, and one refused field fails the whole read.
	var model, method string
	var kwargs map[string]any
	for _, c := range calls {
		args, _ := c["args"].([]any)
		if len(args) < 7 {
			continue
		}
		model, _ = args[3].(string)
		method, _ = args[4].(string)
		kwargs, _ = args[6].(map[string]any)
	}
	if model != "hr.employee.public" || method != "search_read" {
		t.Errorf("read %s.%s", model, method)
	}
	if kwargs["order"] != "name asc" {
		t.Errorf("order = %v, want the ERP's own name collation", kwargs["order"])
	}
	fields, _ := kwargs["fields"].([]any)
	want := map[string]bool{"id": false, "name": false, "job_designation": false,
		"work_email": false, "work_phone": false}
	for _, f := range fields {
		name, _ := f.(string)
		if _, known := want[name]; !known {
			t.Errorf("the read asks for %q, which no card shows", name)
		}
		want[name] = true
	}
	for name, asked := range want {
		if !asked {
			t.Errorf("the read does not ask for %s", name)
		}
	}
}

// One employee's own detail: the many2one fields arrive named, and the two many2many ones
// arrive as bare ids that need their own reads to become words.
func TestFetchEmployee(t *testing.T) {
	const detail = `[{"id": 162, "work_email": "abdullah.zayed@strativ.se",
		"work_phone": "+46 72 130 50 43", "mobile_phone": false,
		"department_id": [3, "Technical"], "parent_id": [17, "Saqibur Rahman"],
		"coach_id": [78, "Milon Mahato"], "leave_manager_id": [10, "Saqibur Rahman"],
		"stack_manager_id": [97, "K.M. Jiaul Islam Jibon"],
		"work_location_id": [3, "Bangladesh"],
		"additional_project_manager_ids": [2],
		"assigned_project_ids": [760, 15]}]`
	const projects = `[{"id": 15, "name": "LumberScan"}, {"id": 760, "name": "Learn and Grow"}]`
	const managers = `[{"id": 2, "name": "Reaz Abedin"}]`

	var models []string
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
		result := `[]`
		if req.Params.Method == "login" {
			result = `26`
		}
		if len(req.Params.Args) >= 6 {
			model, _ := req.Params.Args[3].(string)
			models = append(models, model)
			inner, _ := req.Params.Args[5].([]any)
			ids, _ := inner[0].([]any)
			switch {
			case model == "project.project":
				result = projects
			case model == "hr.employee.public" && len(ids) == 1 && ids[0] == 2.0:
				result = managers
			case model == "hr.employee.public":
				result = detail
			}
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":` + result + `}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv(BaseEnv, srv.URL)

	msg, ok := FetchEmployee("k", "user@example.com", "db", 162)().(EmployeeMsg)
	if !ok {
		t.Fatal("FetchEmployee did not return EmployeeMsg")
	}
	if msg.Err != nil {
		t.Fatalf("Err = %v", msg.Err)
	}
	d := msg.Detail
	for what, got := range map[string]string{
		"email": d.Email, "phone": d.Phone, "department": d.Department,
		"team lead": d.TeamLead, "time off": d.TimeOff, "stack manager": d.StackManager,
		"coach": d.Coach, "location": d.Location,
	} {
		if got == "" {
			t.Errorf("%s came back empty: %+v", what, d)
		}
	}
	// mobile_phone is Odoo's false, which must be an empty string rather than a failed read.
	if d.Mobile != "" {
		t.Errorf("mobile = %q", d.Mobile)
	}
	// The projects read back as names, in the order the employee's own field lists them.
	if len(d.Projects) != 2 || d.Projects[0] != "Learn and Grow" || d.Projects[1] != "LumberScan" {
		t.Errorf("projects = %v, want the ERP's own order", d.Projects)
	}
	if len(d.Managers) != 1 || d.Managers[0] != "Reaz Abedin" {
		t.Errorf("managers = %v", d.Managers)
	}
	// project.project is read because a many2many answers with ids and nothing else.
	var sawProjects bool
	for _, m := range models {
		if m == "project.project" {
			sawProjects = true
		}
	}
	if !sawProjects {
		t.Errorf("the projects were never named: %v", models)
	}
}

// Nothing to resolve, nothing to read: an employee on no projects costs one call.
func TestFetchEmployeeSkipsEmptyLists(t *testing.T) {
	const detail = `[{"id": 57, "work_email": "jonna@strativ.se", "work_phone": false,
		"mobile_phone": false, "department_id": false, "parent_id": false, "coach_id": false,
		"leave_manager_id": false, "stack_manager_id": false, "work_location_id": false,
		"additional_project_manager_ids": [], "assigned_project_ids": []}]`

	var models []string
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
		result := detail
		if req.Params.Method == "login" {
			result = `26`
		} else if len(req.Params.Args) >= 4 {
			model, _ := req.Params.Args[3].(string)
			models = append(models, model)
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":` + result + `}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv(BaseEnv, srv.URL)

	msg := FetchEmployee("k", "u@e.com", "db", 57)().(EmployeeMsg)
	if msg.Err != nil {
		t.Fatalf("Err = %v", msg.Err)
	}
	if len(msg.Detail.Projects) != 0 || len(msg.Detail.Managers) != 0 {
		t.Errorf("detail = %+v", msg.Detail)
	}
	for _, m := range models {
		if m == "project.project" {
			t.Error("an employee on no projects still read project.project")
		}
	}
	// A many2one the ERP left empty is an empty string, not a crash.
	if msg.Detail.Department != "" || msg.Detail.TeamLead != "" {
		t.Errorf("detail = %+v", msg.Detail)
	}
}
