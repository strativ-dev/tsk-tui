package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tasnimAlam/tsk/internal/store"
)

// EmployeesMsg is the office directory, as the ERP publishes it.
type EmployeesMsg struct {
	Employees []store.Employee
	Err       error
}

// EmployeeMsg is one employee's own detail, the answer to opening their row.
type EmployeeMsg struct {
	ID     int
	Detail store.EmployeeDetail
	Err    error
}

// FetchEmployee is a tea.Cmd: everything the ERP's own employee form shows about one person.
//
// Up to three reads behind one login, and the second two only when there is something to
// resolve: Odoo answers a many2one with an [id, name] pair, so a department or a team lead
// arrives named, but a **many2many comes back as bare ids** — the assigned projects and the
// extra project managers each need their own read to become words. The web client does exactly
// the same thing; there is no field on hr.employee.public that carries those names.
func FetchEmployee(key, login, db string, id int) tea.Cmd {
	return func() tea.Msg {
		fail := func(err error) tea.Msg { return EmployeeMsg{ID: id, Err: err} }
		uid, err := connect(strings.TrimSpace(db), strings.TrimSpace(login), strings.TrimSpace(key))
		if err != nil {
			return fail(err)
		}

		raw, err := rpc("object", "execute_kw", []any{
			db, uid, key, "hr.employee.public", "read",
			[]any{[]int{id}, []string{
				"work_email", "work_phone", "mobile_phone", "department_id", "parent_id",
				"coach_id", "leave_manager_id", "stack_manager_id", "work_location_id",
				"additional_project_manager_ids", "assigned_project_ids",
			}},
			map[string]any{"context": map[string]any{"lang": "en_US", "tz": "Asia/Dhaka"}},
		})
		if err != nil {
			return fail(err)
		}
		var rows []struct {
			Email    odooText `json:"work_email"`
			Phone    odooText `json:"work_phone"`
			Mobile   odooText `json:"mobile_phone"`
			Dept     odooRef  `json:"department_id"`
			Lead     odooRef  `json:"parent_id"`
			Coach    odooRef  `json:"coach_id"`
			TimeOff  odooRef  `json:"leave_manager_id"`
			Stack    odooRef  `json:"stack_manager_id"`
			Location odooRef  `json:"work_location_id"`
			Managers []int    `json:"additional_project_manager_ids"`
			Projects []int    `json:"assigned_project_ids"`
		}
		if err := json.Unmarshal(raw, &rows); err != nil {
			return fail(fmt.Errorf("bad employee result: %w", err))
		}
		if len(rows) == 0 {
			return fail(errors.New("the ERP has no such employee"))
		}
		r := rows[0]

		out := store.EmployeeDetail{
			ID:           id,
			Email:        strings.TrimSpace(string(r.Email)),
			Phone:        strings.TrimSpace(string(r.Phone)),
			Mobile:       strings.TrimSpace(string(r.Mobile)),
			Department:   r.Dept.Name,
			TeamLead:     r.Lead.Name,
			Coach:        r.Coach.Name,
			TimeOff:      r.TimeOff.Name,
			StackManager: r.Stack.Name,
			Location:     r.Location.Name,
		}
		if out.Projects, err = names(db, uid, key, "project.project", r.Projects); err != nil {
			return fail(err)
		}
		if out.Managers, err = names(db, uid, key, "hr.employee.public", r.Managers); err != nil {
			return fail(err)
		}
		return EmployeeMsg{ID: id, Detail: out}
	}
}

// names reads the name of each id, in the order the ids came in — which is the order the ERP
// itself lists them in. No ids, no call.
func names(db string, uid int, key, model string, ids []int) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	raw, err := rpc("object", "execute_kw", []any{
		db, uid, key, model, "read",
		[]any{ids, []string{"id", "name"}},
		map[string]any{"context": map[string]any{"lang": "en_US"}},
	})
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID   int      `json:"id"`
		Name odooText `json:"name"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("bad %s result: %w", model, err)
	}
	byID := make(map[int]string, len(rows))
	for _, r := range rows {
		byID[r.ID] = oneLine(strings.TrimSpace(string(r.Name)))
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if n := byID[id]; n != "" {
			out = append(out, n)
		}
	}
	return out, nil
}

// FetchEmployees is a tea.Cmd: the whole directory, one call.
//
// hr.employee.public, not hr.employee: the public model is what the web client's own
// directory reads and what everyone is allowed to see — the private one refuses most of its
// fields to a user who is not an HR officer, and one refused field fails the whole read (the
// same trap the attendance code documents). The browser reaches it over
// /web/dataset/call_kw with a session cookie; we have an API key, so it goes through
// execute_kw like the rest of the app, and search_read is the same query web_search_read
// wraps.
//
// Ordered by name here rather than sorted on render: the ERP has a collation for names and
// the cache should hold them the way the directory reads them.
func FetchEmployees(key, login, db string) tea.Cmd {
	return func() tea.Msg {
		uid, err := connect(strings.TrimSpace(db), strings.TrimSpace(login), strings.TrimSpace(key))
		if err != nil {
			return EmployeesMsg{Err: err}
		}

		raw, err := rpc("object", "execute_kw", []any{
			db, uid, key, "hr.employee.public", "search_read",
			[]any{[]any{}},
			map[string]any{
				"fields": []string{"id", "name", "job_designation", "work_email", "work_phone"},
				"order":  "name asc",
			},
		})
		if err != nil {
			return EmployeesMsg{Err: err}
		}

		var rows []struct {
			ID    int      `json:"id"`
			Name  odooText `json:"name"`
			Job   odooText `json:"job_designation"`
			Email odooText `json:"work_email"`
			Phone odooText `json:"work_phone"`
		}
		if err := json.Unmarshal(raw, &rows); err != nil {
			return EmployeesMsg{Err: fmt.Errorf("bad employee result: %w", err)}
		}

		out := make([]store.Employee, 0, len(rows))
		for _, r := range rows {
			// Every field is odooText: an empty char field comes back as false, and a false
			// into a string fails the **whole** directory rather than the one row. Half this
			// list has no work_phone.
			out = append(out, store.Employee{
				ID:    r.ID,
				Name:  strings.TrimSpace(string(r.Name)),
				Job:   strings.TrimSpace(string(r.Job)),
				Email: strings.TrimSpace(string(r.Email)),
				Phone: strings.TrimSpace(string(r.Phone)),
			})
		}
		return EmployeesMsg{Employees: out}
	}
}
