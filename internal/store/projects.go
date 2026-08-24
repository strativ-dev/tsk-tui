package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
)

// Project is one row on the projects tab: what it is called, whose teams are on it, and how
// many tasks it holds. Exactly what the ERP's own project kanban card shows, and nothing
// else — this screen answers "what is running and who is on it", not "how is it going".
//
// Teams are names rather than ids: team_ids is a many2many, so Odoo answers it with bare ids
// and they are resolved by their own read (see api.FetchProjects).
type Project struct {
	ID    int      `json:"id"`
	Name  string   `json:"name"`
	Teams []string `json:"teams"`
	Tasks int      `json:"tasks"`
	// Manager is the ERP's own Project Manager (project.project.user_id), which arrives named
	// because a many2one comes back as an [id, name] pair.
	Manager string `json:"manager"`
	// Members is every user on the project's teams, distinct and in the order the ERP listed
	// them. Ids, not names: they come off serp_project.team.user_ids, which is a many2many, and
	// resolving a few hundred of them for a list nobody has opened would be a call to answer a
	// question nobody asked. api.FetchProjectMembers turns them into people when a row opens.
	Members []int `json:"members"`
	// Mine is whether the key's owner is on this project — its manager, or on one of its
	// teams. Worked out where the ids are (api.FetchProjects knows the uid the login answered
	// with) rather than on render, so the filter costs nothing and works off the cache.
	Mine bool `json:"mine"`
	// People is Members with names on, once a row has been opened and the ERP has answered.
	// It lives on the cached record so a restart does not ask again: a team's membership does
	// not change between two openings of a terminal, which is the whole reason this list is on
	// disk. It is carried across a refresh only while Members is unchanged — new ids mean the
	// names beside them are somebody else's.
	People []Member `json:"people,omitempty"`
}

// Member is one person on a project's teams, as the row's own table shows them.
type Member struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// ProjectsLoadedMsg is the cache coming off disk.
type ProjectsLoadedMsg struct {
	Projects []Project
	Err      error
}

// ProjectsPath is the project cache: $XDG_CONFIG_HOME/tsk/projects.json.
//
// Its own file, like the directory's: the two have nothing to do with each other, and a write
// of one must not be able to lose the other.
func ProjectsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "tsk", "projects.json")
}

// LoadProjects is a tea.Cmd. A missing file is not an error — the tab fetches on its first open
// and the cache exists from then on.
func LoadProjects() tea.Msg {
	b, err := os.ReadFile(ProjectsPath())
	if errors.Is(err, fs.ErrNotExist) {
		return ProjectsLoadedMsg{}
	}
	if err != nil {
		return ProjectsLoadedMsg{Err: err}
	}
	var out []Project
	if err := json.Unmarshal(b, &out); err != nil {
		return ProjectsLoadedMsg{Err: err}
	}
	return ProjectsLoadedMsg{Projects: out}
}

// SaveProjects replaces the cache atomically, the same way the directory is written.
func SaveProjects(list []Project) tea.Cmd {
	return func() tea.Msg {
		b, err := json.MarshalIndent(list, "", "  ")
		if err != nil {
			return SavedMsg{Err: err}
		}
		path := ProjectsPath()
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
