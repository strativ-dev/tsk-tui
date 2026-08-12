// Package store owns the task list on disk. Every read and write is a tea.Cmd,
// so Update and View stay free of I/O.
package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tasnimAlam/tsk/internal/parse"
)

type Task struct {
	ID    int     `json:"id"`
	Title string  `json:"title"`
	Tag   string  `json:"tag"`
	Rows  []Entry `json:"rows"` // newest first
}

type Entry struct {
	ID      int    `json:"id"`
	Date    string `json:"date"` // dd/mm/yy
	Desc    string `json:"desc"`
	Minutes int    `json:"minutes"`
	// Local marks an entry the ERP does not have, or has differently: typed with
	// `a`, or a pulled line edited here. An Odoo pull must not discard these.
	// App-created entries also carry a negative ID, so they cannot collide with an
	// account.analytic.line id.
	Local bool `json:"local,omitempty"`
}

type LoadedMsg struct {
	Tasks []Task
	Err   error
}

type SavedMsg struct{ Err error }

// Path is the tasks file: $XDG_CONFIG_HOME/tsk/tasks.json.
func Path() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "tsk", "tasks.json")
}

// Load is a tea.Cmd. A missing file is not an error — a first run starts empty
// and fills up from the API.
func Load() tea.Msg {
	b, err := os.ReadFile(Path())
	if errors.Is(err, fs.ErrNotExist) {
		return LoadedMsg{}
	}
	if err != nil {
		return LoadedMsg{Err: err}
	}
	var tasks []Task
	if err := json.Unmarshal(b, &tasks); err != nil {
		return LoadedMsg{Err: err}
	}
	return LoadedMsg{Tasks: tasks}
}

// Save writes the whole list, replacing the file atomically.
func Save(tasks []Task) tea.Cmd {
	return func() tea.Msg {
		b, err := json.MarshalIndent(tasks, "", "  ")
		if err != nil {
			return SavedMsg{Err: err}
		}
		path := Path()
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

// Total sums an entry list. Totals are always derived, never stored.
func Total(rows []Entry) int {
	sum := 0
	for _, r := range rows {
		sum += r.Minutes
	}
	return sum
}

// MinutesOn sums every entry on one date across every task.
func MinutesOn(tasks []Task, date string) int {
	sum := 0
	for _, t := range tasks {
		for _, r := range t.Rows {
			if r.Date == date {
				sum += r.Minutes
			}
		}
	}
	return sum
}

// PendingMinutesOn sums the entries on a date that the ERP has no copy of at all
// — app-created rows, which carry a negative id. Local *edits* of pulled lines are
// left out on purpose: the ERP's own day total already counts their original hours,
// so adding them again would double-count.
func PendingMinutesOn(tasks []Task, date string) int {
	sum := 0
	for _, t := range tasks {
		for _, r := range t.Rows {
			if r.Local && r.ID < 0 && r.Date == date {
				sum += r.Minutes
			}
		}
	}
	return sum
}

// NextEntryID numbers an entry created in the app. App ids run negative so a
// pull can tell Odoo's own lines from ours.
func NextEntryID(t Task) int {
	low := 0
	for _, r := range t.Rows {
		if r.ID < low {
			low = r.ID
		}
	}
	return low - 1
}

// MergeRows refreshes the lines Odoo owns without dropping what was typed here:
// a local edit of a pulled line wins over the pulled copy, and app-created
// entries are kept alongside. Newest first, as everywhere else.
func MergeRows(local, remote []Entry) []Entry {
	edits := make(map[int]Entry, len(local))
	var added []Entry
	for _, e := range local {
		switch {
		case !e.Local:
			continue // Odoo's copy is the truth for these
		case e.ID < 0:
			added = append(added, e)
		default:
			edits[e.ID] = e
		}
	}

	out := make([]Entry, 0, len(remote)+len(added))
	for _, r := range remote {
		if e, ok := edits[r.ID]; ok {
			out = append(out, e)
			continue
		}
		out = append(out, r)
	}
	out = append(out, added...)

	sort.SliceStable(out, func(i, j int) bool { return after(out[i].Date, out[j].Date) })
	return out
}

// after reports whether date a falls later than b; unreadable dates sort last.
func after(a, b string) bool {
	ta, errA := time.Parse(parse.DateLayout, a)
	tb, errB := time.Parse(parse.DateLayout, b)
	switch {
	case errA != nil:
		return false
	case errB != nil:
		return true
	}
	return ta.After(tb)
}
