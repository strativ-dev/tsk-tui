// Package api talks to the ERP tasks API. The key lives in the Authorization
// header and nowhere else: no query string, no log line, no error message.
package api

import (
	"bytes"
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
			// Key stays its own field: the view prefixes it to the title, and
			// logging hours needs it on its own.
			tasks = append(tasks, store.Task{ID: t.ID, Key: t.Key, Title: t.Name, Tag: tag})
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

// LoggedMsg is the result of writing one entry to the ERP. EntryID is the
// account.analytic.line the server created, which replaces the app's negative id.
type LoggedMsg struct {
	TaskID  int
	LocalID int // the app-side id of the row that was sent, so it can be found again
	EntryID int
	Minutes int
	Err     error
}

// LogHours is a tea.Cmd: POST /api/v1/timesheets/log, one entry for the key owner.
// The endpoint identifies the task by key ("SE360-1372"), takes decimal hours, and
// creates the line unconfirmed. There is no update endpoint, so this is create-only.
func LogHours(key, taskKey, date, desc string, minutes, taskID, localID int) tea.Cmd {
	return func() tea.Msg {
		fail := func(err error) tea.Msg {
			return LoggedMsg{TaskID: taskID, LocalID: localID, Err: err}
		}

		key, taskKey = strings.TrimSpace(key), strings.TrimSpace(taskKey)
		desc = strings.TrimSpace(desc)
		switch {
		case key == "":
			return fail(ErrNoKey)
		case taskKey == "":
			return fail(errors.New("task has no key in the ERP, cannot log against it"))
		case desc == "":
			return fail(errors.New("the ERP requires a description"))
		case minutes <= 0 || minutes > 24*60:
			return fail(fmt.Errorf("%s is out of range, the ERP takes 0 to 24h per entry",
				parse.FormatTotal(minutes)))
		}

		day, err := time.Parse(parse.DateLayout, strings.TrimSpace(date))
		if err != nil {
			return fail(fmt.Errorf("unreadable date %q", date))
		}

		body, err := json.Marshal(map[string]any{
			"task_key":    taskKey,
			"date":        day.Format("2006-01-02"),
			"hours":       float64(minutes) / 60,
			"description": desc,
		})
		if err != nil {
			return fail(err)
		}

		req, err := http.NewRequest(http.MethodPost, BaseURL()+"/api/v1/timesheets/log",
			bytes.NewReader(body))
		if err != nil {
			return fail(err)
		}
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
		if err != nil {
			return fail(errors.New("cannot reach " + BaseURL()))
		}
		defer resp.Body.Close()

		r := io.LimitReader(resp.Body, 1<<20)
		if resp.StatusCode == http.StatusUnauthorized {
			return fail(ErrUnauthorized)
		}
		if resp.StatusCode != http.StatusCreated {
			return fail(logError(resp.StatusCode, r))
		}

		var created struct {
			ID    int     `json:"id"`
			Hours float64 `json:"hours"`
		}
		if err := json.NewDecoder(r).Decode(&created); err != nil {
			return fail(fmt.Errorf("bad response from API: %w", err))
		}
		return LoggedMsg{
			TaskID:  taskID,
			LocalID: localID,
			EntryID: created.ID,
			Minutes: int(math.Round(created.Hours * 60)),
		}
	}
}

// logError turns the documented {error, code} body into something worth reading.
// The codes come from log-hours.md; the ones a person can act on get plainer text.
func logError(status int, r io.Reader) error {
	var e struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	_ = json.NewDecoder(r).Decode(&e)

	switch e.Code {
	case "daily_cap_exceeded":
		return errors.New("this would push the day past 24h")
	case "task_not_found":
		return errors.New("the ERP does not know that task key")
	case "task_ambiguous":
		return errors.New("that task key matches more than one project — log it in the ERP")
	case "no_employee", "no_hourly_cost":
		return fmt.Errorf("your ERP user is not set up for timesheets (%s) — ask HR", e.Code)
	}
	if e.Error != "" {
		return fmt.Errorf("API %d: %s", status, e.Error)
	}
	return fmt.Errorf("API %d", status)
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
