// Package store owns the task list on disk. Every read and write is a tea.Cmd,
// so Update and View stay free of I/O.
package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

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

// Load is a tea.Cmd. A missing file is not an error — it seeds the sample list.
func Load() tea.Msg {
	b, err := os.ReadFile(Path())
	if errors.Is(err, fs.ErrNotExist) {
		return LoadedMsg{Tasks: seed()}
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

// NextEntryID is one past the highest entry id in the task.
func NextEntryID(t Task) int {
	max := 0
	for _, r := range t.Rows {
		if r.ID > max {
			max = r.ID
		}
	}
	return max + 1
}

func seed() []Task {
	today := parse.Today()
	return []Task{
		{ID: 1372, Title: "Add hour-log summary API", Tag: "backend", Rows: []Entry{
			{ID: 2, Date: today, Desc: "Endpoint + serializer", Minutes: 150},
			{ID: 1, Date: today, Desc: "Model method and tests", Minutes: 195},
		}},
		{ID: 1401, Title: "Task TUI keyboard flow", Tag: "ui", Rows: []Entry{
			{ID: 1, Date: today, Desc: "Search and list modes", Minutes: 60},
		}},
		{ID: 1412, Title: "Sprint report export", Tag: "reports", Rows: nil},
	}
}
