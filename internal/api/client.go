// Package api talks to the ERP tasks API. The key lives in the Authorization
// header and nowhere else: no query string, no log line, no error message.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tasnimAlam/tsk/internal/parse"
	"github.com/tasnimAlam/tsk/internal/store"
)

// BaseEnv points the client at another ERP instance (staging, local Odoo).
const BaseEnv = "TSK_API_URL"

const defaultBase = "https://erp360.strativ.se"

var (
	// ErrNoKey means nothing to send: the model asks the user for a key.
	ErrNoKey = errors.New("no API key")
	// ErrUnauthorized is a 401: the key is missing, malformed, or revoked.
	ErrUnauthorized = errors.New("API key rejected — generate a new one in Odoo: Preferences → Account Security → New API Key")
)

// TasksMsg is the result of a fetch.
type TasksMsg struct {
	Tasks []store.Task
	Err   error
}

// BaseURL is the ERP root, overridable for staging.
func BaseURL() string {
	if v := strings.TrimSpace(os.Getenv(BaseEnv)); v != "" {
		return strings.TrimRight(v, "/")
	}
	return defaultBase
}

// FetchTasks is a tea.Cmd: GET /api/v1/tasks/my as the key's owner.
func FetchTasks(key string) tea.Cmd {
	return func() tea.Msg {
		key = strings.TrimSpace(key)
		if key == "" {
			return TasksMsg{Err: ErrNoKey}
		}

		req, err := http.NewRequest(http.MethodGet, BaseURL()+"/api/v1/tasks/my", nil)
		if err != nil {
			return TasksMsg{Err: err}
		}
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Accept", "application/json")

		resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
		if err != nil {
			return TasksMsg{Err: errors.New("cannot reach " + BaseURL())}
		}
		defer resp.Body.Close()

		body := io.LimitReader(resp.Body, 4<<20)
		switch {
		case resp.StatusCode == http.StatusUnauthorized:
			return TasksMsg{Err: ErrUnauthorized}
		case resp.StatusCode != http.StatusOK:
			return TasksMsg{Err: fmt.Errorf("API %s%s", resp.Status, detail(body))}
		}

		var payload struct {
			Data []struct {
				ID      int    `json:"id"`
				Key     string `json:"key"`
				Name    string `json:"name"`
				Project *struct {
					Name string `json:"name"`
				} `json:"project"`
			} `json:"data"`
		}
		if err := json.NewDecoder(body).Decode(&payload); err != nil {
			return TasksMsg{Err: fmt.Errorf("bad response from API: %w", err)}
		}

		tasks := make([]store.Task, 0, len(payload.Data))
		for _, t := range payload.Data {
			tag := ""
			if t.Project != nil {
				tag = t.Project.Name
			}
			title := t.Name
			if t.Key != "" {
				title = t.Key + " " + t.Name
			}
			tasks = append(tasks, store.Task{ID: t.ID, Title: title, Tag: tag})
		}
		return TasksMsg{Tasks: tasks}
	}
}

// DayHoursMsg is what the ERP says you logged on one date. The hour-log summary
// reports day totals only — it carries no per-entry lines, so the table rows stay
// local until a line-level endpoint exists.
type DayHoursMsg struct {
	Date    string // dd/mm/yy, as stored
	Minutes int
	// UserEmail is the key owner's Odoo login, which JSON-RPC needs to read
	// timesheet lines. This response is the only place the API hands it over.
	UserEmail string
	Err       error
}

// FetchDayHours is a tea.Cmd: GET /api/v1/timesheets/hour-log-summary for a
// single day. date is dd/mm/yy; the API wants YYYY-MM-DD.
func FetchDayHours(key, date string) tea.Cmd {
	return func() tea.Msg {
		key = strings.TrimSpace(key)
		if key == "" {
			return DayHoursMsg{Date: date, Err: ErrNoKey}
		}
		day, err := time.Parse(parse.DateLayout, date)
		if err != nil {
			return DayHoursMsg{Date: date, Err: err}
		}
		iso := day.Format("2006-01-02")

		req, err := http.NewRequest(http.MethodGet,
			BaseURL()+"/api/v1/timesheets/hour-log-summary?start_date="+iso+"&end_date="+iso, nil)
		if err != nil {
			return DayHoursMsg{Date: date, Err: err}
		}
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Accept", "application/json")

		resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
		if err != nil {
			return DayHoursMsg{Date: date, Err: errors.New("cannot reach " + BaseURL())}
		}
		defer resp.Body.Close()

		body := io.LimitReader(resp.Body, 1<<20)
		switch {
		case resp.StatusCode == http.StatusUnauthorized:
			return DayHoursMsg{Date: date, Err: ErrUnauthorized}
		case resp.StatusCode != http.StatusOK:
			return DayHoursMsg{Date: date, Err: fmt.Errorf("API %s%s", resp.Status, detail(body))}
		}

		var payload struct {
			TotalHours float64 `json:"total_hours"`
			User       struct {
				Email string `json:"email"`
			} `json:"user"`
		}
		if err := json.NewDecoder(body).Decode(&payload); err != nil {
			return DayHoursMsg{Date: date, Err: fmt.Errorf("bad response from API: %w", err)}
		}
		return DayHoursMsg{
			Date:      date,
			Minutes:   int(math.Round(payload.TotalHours * 60)),
			UserEmail: payload.User.Email,
		}
	}
}

// detail pulls the API's own error message, if it sent one.
func detail(r io.Reader) string {
	var e struct {
		Error string `json:"error"`
	}
	if json.NewDecoder(r).Decode(&e) == nil && e.Error != "" {
		return ": " + e.Error
	}
	return ""
}
