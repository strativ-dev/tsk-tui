package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
)

// Employee is one card on the employee tab: who they are, what they do, and how to reach
// them. Exactly what the ERP's own public directory shows, and nothing else — this list is
// read by everyone, so a field it does not publish has no business here.
type Employee struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Job   string `json:"job"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

// EmployeeDetail is what one card opens into: everything the ERP's own employee form shows
// that a colleague would want — how to reach them, where they sit in the org, and what they
// are on.
//
// The many2one fields arrive from Odoo as [id, name] pairs, so those are names already; the
// project list and the extra project managers arrive as bare ids and are resolved by their own
// reads (see api.FetchEmployee).
type EmployeeDetail struct {
	ID           int      `json:"id"`
	Email        string   `json:"email"`
	Phone        string   `json:"phone"`
	Mobile       string   `json:"mobile"`
	Department   string   `json:"department"`
	TeamLead     string   `json:"team_lead"`
	Coach        string   `json:"coach"`
	TimeOff      string   `json:"timeoff"`
	StackManager string   `json:"stack_manager"`
	Location     string   `json:"location"`
	Managers     []string `json:"managers"`
	Projects     []string `json:"projects"`
}

type EmployeesLoadedMsg struct {
	Employees []Employee
	Err       error
}

// EmployeesPath is the directory cache: $XDG_CONFIG_HOME/tsk/employees.json.
//
// A separate file from tasks.json, not a second key inside it: the two have nothing to do
// with each other, and a write of one must not be able to lose the other.
func EmployeesPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "tsk", "employees.json")
}

// LoadEmployees is a tea.Cmd. A missing file is not an error — the tab fetches on its first
// open and the cache exists from then on.
func LoadEmployees() tea.Msg {
	b, err := os.ReadFile(EmployeesPath())
	if errors.Is(err, fs.ErrNotExist) {
		return EmployeesLoadedMsg{}
	}
	if err != nil {
		return EmployeesLoadedMsg{Err: err}
	}
	var out []Employee
	if err := json.Unmarshal(b, &out); err != nil {
		return EmployeesLoadedMsg{Err: err}
	}
	return EmployeesLoadedMsg{Employees: out}
}

// SaveEmployees replaces the cache atomically, the same way the task list is written.
func SaveEmployees(list []Employee) tea.Cmd {
	return func() tea.Msg {
		b, err := json.MarshalIndent(list, "", "  ")
		if err != nil {
			return SavedMsg{Err: err}
		}
		path := EmployeesPath()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return SavedMsg{Err: err}
		}
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, b, 0o644); err != nil {
			return SavedMsg{Err: err}
		}
		return SavedMsg{Err: os.Rename(tmp, path)}
	}
}
