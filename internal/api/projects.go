package api

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tasnimAlam/tsk/internal/store"
)

// projLimit is what the ERP's own project kanban asks for. There are 89 open projects on this
// database; the cap is here so a screen that lists them all cannot become a screen that reads
// a thousand.
const projLimit = 200

// ProjectsMsg is the office's open projects, as the ERP publishes them.
type ProjectsMsg struct {
	Projects []store.Project
	Err      error
}

// FetchProjects is a tea.Cmd: every project that is still open, with its teams named.
//
// Two reads behind one login, for the reason the employee detail needs its own: `team_ids` is
// a **many2many**, so Odoo answers it with bare ids and nothing on project.project carries
// their names. The web client does the same pair of calls — web_search_read for the cards,
// then a read of the team ids they mention.
//
// The teams come back in **one** call for the whole list rather than one per project, and by
// search_read rather than read: read raises on an id the caller may not see, where a domain
// simply leaves it out — and a project whose team is invisible should still be on screen.
//
// The domain and the order are the ERP's own list view: open projects only, and its own
// sequence before the name, so the screen reads in the order the web client shows.
func FetchProjects(key, login, db string) tea.Cmd {
	return func() tea.Msg {
		uid, err := connect(strings.TrimSpace(db), strings.TrimSpace(login), strings.TrimSpace(key))
		if err != nil {
			return ProjectsMsg{Err: err}
		}

		raw, err := rpc("object", "execute_kw", []any{
			db, uid, key, "project.project", "search_read",
			[]any{[]any{[]any{"is_closed", "=", false}}},
			map[string]any{
				// task_count, not project_task_count: it is Odoo's own field and what the
				// kanban card counts, where the serp one also counts what is closed — 237
				// against 181 on AI Transformation, and the card says 181.
				"fields": []string{"id", "display_name", "user_id", "team_ids", "task_count"},
				"order":  "sequence asc, name asc, id asc",
				"limit":  projLimit,
			},
		})
		if err != nil {
			return ProjectsMsg{Err: err}
		}

		var rows []struct {
			ID   int      `json:"id"`
			Name odooText `json:"display_name"`
			// The Project Manager, which arrives named: a many2one is an [id, name] pair, so
			// this costs no second call the way team_ids does.
			Manager odooRef `json:"user_id"`
			Teams   []int   `json:"team_ids"`
			Tasks   int     `json:"task_count"`
		}
		if err := json.Unmarshal(raw, &rows); err != nil {
			return ProjectsMsg{Err: fmt.Errorf("bad project result: %w", err)}
		}

		// Every distinct id the projects mention, so the names cost one read rather than one
		// per project.
		seen := map[int]bool{}
		var ids []int
		for _, r := range rows {
			for _, id := range r.Teams {
				if !seen[id] {
					seen[id] = true
					ids = append(ids, id)
				}
			}
		}
		teams, err := readTeams(db, uid, key, ids)
		if err != nil {
			return ProjectsMsg{Err: err}
		}

		out := make([]store.Project, 0, len(rows))
		for _, r := range rows {
			p := store.Project{
				ID: r.ID,
				// Flattened on the way in, the way a requisition's note is: a project name on
				// this database can hold a newline, and one of those grows the row it is
				// drawn on into two.
				Name:    oneLine(strings.TrimSpace(string(r.Name))),
				Tasks:   r.Tasks,
				Manager: r.Manager.Name,
			}
			// Mine is what the filter reads: the ERP has no "my projects" field, and the
			// question it answers — is this one of mine — is the manager being me or my being
			// on one of its teams. uid is what the login answered with, so this costs no call
			// and the answer keeps in the cache.
			p.Mine = r.Manager.ID == uid

			// In the order the ERP listed them, and an id whose team could not be read is
			// left out rather than drawn as a gap. The members are the union over the
			// project's teams — one list, since a person on two of them is one person.
			onIt := map[int]bool{}
			for _, id := range r.Teams {
				t, ok := teams[id]
				if !ok {
					continue
				}
				if t.name != "" {
					p.Teams = append(p.Teams, t.name)
				}
				for _, u := range t.users {
					if u == uid {
						p.Mine = true
					}
					if !onIt[u] {
						onIt[u] = true
						p.Members = append(p.Members, u)
					}
				}
			}
			out = append(out, p)
		}
		return ProjectsMsg{Projects: out}
	}
}

// team is what one serp_project.team row is worth here: what it is called, and who is on it.
type team struct {
	name  string
	users []int
}

// readTeams resolves those ids in one call. No ids, no call.
//
// serp_project.team is not exposed on the MCP, so this contract was read off RPC: `name` and
// `display_name` hold the same string, `user_ids` is the ERP's own **Members**, and the model
// is readable by an ordinary API key. search_read rather than read, since read raises on an id
// the caller may not see where a domain simply leaves it out.
//
// The members come along here rather than when a row opens: they are one more field on a call
// this screen already makes, where fetching them per project would be a call per open. They
// stay ids until somebody asks — see FetchProjectMembers.
func readTeams(db string, uid int, key string, ids []int) (map[int]team, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	raw, err := rpc("object", "execute_kw", []any{
		db, uid, key, "serp_project.team", "search_read",
		[]any{[]any{[]any{"id", "in", ids}}},
		map[string]any{"fields": []string{"id", "name", "user_ids"}},
	})
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID    int      `json:"id"`
		Name  odooText `json:"name"`
		Users []int    `json:"user_ids"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("bad team result: %w", err)
	}
	out := make(map[int]team, len(rows))
	for _, r := range rows {
		out[r.ID] = team{name: oneLine(strings.TrimSpace(string(r.Name))), users: r.Users}
	}
	return out, nil
}

// ProjectMembersMsg is one project's people, the answer to opening its row.
type ProjectMembersMsg struct {
	ID      int
	Members []store.Member
	Err     error
}

// FetchProjectMembers is a tea.Cmd: the name and work email of everyone on a project's teams.
//
// One call, and only for the ids that project's own teams named — the same shape the employee
// detail uses, and the same call the web client makes when it needs to put names to a
// many2many. search_read rather than read for the reason readTeams gives: a user the caller
// cannot see is left out rather than failing the whole table.
//
// The order is the ERP's own name collation, not the order the ids arrived in: this is a table
// somebody reads down, where a project's team_ids order says something and a union of two
// teams' members says nothing.
func FetchProjectMembers(key, login, db string, id int, users []int) tea.Cmd {
	return func() tea.Msg {
		fail := func(err error) tea.Msg { return ProjectMembersMsg{ID: id, Err: err} }
		if len(users) == 0 {
			return ProjectMembersMsg{ID: id}
		}
		uid, err := connect(strings.TrimSpace(db), strings.TrimSpace(login), strings.TrimSpace(key))
		if err != nil {
			return fail(err)
		}
		raw, err := rpc("object", "execute_kw", []any{
			db, uid, key, "res.users", "search_read",
			[]any{[]any{[]any{"id", "in", users}}},
			map[string]any{
				"fields": []string{"id", "name", "email"},
				"order":  "name asc",
			},
		})
		if err != nil {
			return fail(err)
		}
		var rows []struct {
			Name  odooText `json:"name"`
			Email odooText `json:"email"`
		}
		if err := json.Unmarshal(raw, &rows); err != nil {
			return fail(fmt.Errorf("bad member result: %w", err))
		}
		out := make([]store.Member, 0, len(rows))
		for _, r := range rows {
			// Both odooText: an empty char field arrives as false, and a false into a string
			// fails the whole table rather than the one row.
			out = append(out, store.Member{
				Name:  oneLine(strings.TrimSpace(string(r.Name))),
				Email: oneLine(strings.TrimSpace(string(r.Email))),
			})
		}
		return ProjectMembersMsg{ID: id, Members: out}
	}
}
