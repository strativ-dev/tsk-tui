package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/tasnimAlam/tsk/internal/api"
	"github.com/tasnimAlam/tsk/internal/parse"
	"github.com/tasnimAlam/tsk/internal/store"
	"github.com/tasnimAlam/tsk/internal/theme"
)

// Mode decides which handler consumes a key. Only the active mode reads keys.
type Mode int

const (
	ModeSearch Mode = iota
	ModeList
	ModeTable
	ModeInsert
	ModeJump
	ModeConfirm
	ModeAuth
)

// DailyGoal is the 8h bar the header measures today against.
const DailyGoal = 8 * 60

type insertKind int

const (
	insertNew insertKind = iota
	insertEdit
)

type confirmKind int

const (
	confirmDeleteRow confirmKind = iota
	confirmDiscard
)

// Insert-mode focus positions.
const (
	fieldDate = iota
	fieldDesc
	fieldHours
	fieldAccept
	fieldReject
	fieldCount
)

type Model struct {
	mode Mode
	prev Mode // where ModeConfirm returns on "no"

	search textinput.Model
	jump   textinput.Model
	auth   textinput.Model
	fields [3]textinput.Model

	key     string // API key, held in memory only
	syncing bool
	status  string

	// erpToday is what the ERP says was logged today, which beats the local
	// guess for the progress bar. -1 until a sync answers.
	erpToday int

	login  string       // the key owner's Odoo login, needed by JSON-RPC
	pulled map[int]bool // task IDs whose lines have been read from Odoo

	tasks    []store.Task
	cursor   int          // index into filtered()
	row      int          // row index inside the focused task
	expanded map[int]bool // task ID -> expanded

	focus        int // insert-mode field
	kind         insertKind
	editRow      int
	datePristine bool // date is prefilled and selected; first keystroke replaces it

	cKind   confirmKind
	cPrompt string

	width, height int
	err           error
}

func New() Model {
	search := textinput.New()
	search.Prompt = "" // the ❯ caret sits outside the box, as in the design
	search.PromptStyle = theme.Prompt
	search.Placeholder = "search title or tag…"
	search.Width = 32
	search.Focus()

	jump := textinput.New()
	jump.Prompt = "/"
	jump.Width = 8
	jump.CharLimit = 8

	auth := textinput.New()
	auth.Prompt = "key "
	auth.Placeholder = "paste your Odoo API key"
	auth.Width = 40
	auth.EchoMode = textinput.EchoPassword // never render the key
	auth.EchoCharacter = '•'

	m := Model{
		mode:     ModeSearch,
		search:   search,
		jump:     jump,
		auth:     auth,
		expanded: map[int]bool{},
		erpToday: -1,
		pulled:   map[int]bool{},
	}
	// Field widths mirror the table columns, so the insert row sits in them.
	for i, ph := range []string{"dd/mm/yy", "what you did", "h:mm"} {
		f := textinput.New()
		f.Prompt = ""
		f.Placeholder = ph
		f.Width = fieldWidth([]int{dateWidth, m.descWidth(), hoursWidth}[i])
		m.fields[i] = f
	}
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, store.Load, store.LoadKey())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// The query field fills the box: total width minus caret, borders and the
		// progress cluster on the right.
		if w := msg.Width - 46; w > 24 {
			m.search.Width = w
		}
		m.fields[fieldDesc].Width = fieldWidth(m.descWidth())
		return m, nil

	case store.LoadedMsg:
		m.err = msg.Err
		if msg.Err == nil {
			m.tasks = msg.Tasks
		}
		return m, nil

	case store.SavedMsg:
		m.err = msg.Err
		return m, nil

	case store.KeyMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		if msg.Key == "" {
			return m.askKey("Paste your Odoo API key to fetch your tasks."), textinput.Blink
		}
		m.key = msg.Key
		return m.startSync()

	case store.KeySavedMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.status = "key encrypted into pass: " + store.PassName()
		return m, nil

	case api.TasksMsg:
		m.syncing = false
		switch {
		case errors.Is(msg.Err, api.ErrUnauthorized), errors.Is(msg.Err, api.ErrNoKey):
			m.key = ""
			return m.askKey(msg.Err.Error()), textinput.Blink
		case msg.Err != nil:
			m.err = msg.Err
			m.status = "offline — showing the tasks on disk"
			return m, nil
		}
		m.tasks = store.Merge(m.tasks, msg.Tasks)
		m.err = nil
		m.status = fmt.Sprintf("synced %d tasks from %s", len(msg.Tasks), api.BaseURL())
		m.clampCursor()
		m.clampRow()
		return m, store.Save(m.tasks)

	case api.DayHoursMsg:
		if msg.Err == nil && msg.Date == parse.Today() {
			m.erpToday = msg.Minutes
		}
		if msg.UserEmail != "" {
			m.login = msg.UserEmail
		}
		return m, nil

	case api.EntriesMsg:
		m.syncing = false
		i := indexOfTask(m.tasks, msg.TaskID)
		switch {
		case errors.Is(msg.Err, api.ErrUnauthorized):
			m.key = ""
			return m.askKey(msg.Err.Error()), textinput.Blink
		case msg.Err != nil:
			m.status = "timesheet lines unavailable: " + msg.Err.Error()
			return m, nil
		case i < 0:
			return m, nil
		}
		// Odoo owns its lines, but entries typed here are not in the ERP yet and
		// must survive the pull.
		before := len(m.tasks[i].Rows)
		m.tasks[i].Rows = store.MergeRows(m.tasks[i].Rows, msg.Rows)
		m.pulled[msg.TaskID] = true
		m.status = fmt.Sprintf("%d lines from Odoo, %d rows total (was %d)",
			len(msg.Rows), len(m.tasks[i].Rows), before)
		m.clampRow()
		return m, store.Save(m.tasks)

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		switch m.mode {
		case ModeSearch:
			return m.updateSearch(msg)
		case ModeList:
			return m.updateList(msg)
		case ModeTable:
			return m.updateTable(msg)
		case ModeInsert:
			return m.updateInsert(msg)
		case ModeJump:
			return m.updateJump(msg)
		case ModeConfirm:
			return m.updateConfirm(msg)
		case ModeAuth:
			return m.updateAuth(msg)
		}
	}
	return m, nil
}

// --- derived state -----------------------------------------------------------

// filtered derives the visible task list from the query on every call; there is
// no second slice to keep in sync.
func (m Model) filtered() []store.Task {
	q := strings.ToLower(strings.TrimSpace(m.search.Value()))
	if q == "" {
		return m.tasks
	}
	var out []store.Task
	for _, t := range m.tasks {
		if strings.Contains(strings.ToLower(t.Title), q) || strings.Contains(strings.ToLower(t.Tag), q) {
			out = append(out, t)
		}
	}
	return out
}

// current is the focused task in the filtered list.
func (m Model) current() (store.Task, bool) {
	f := m.filtered()
	if m.cursor < 0 || m.cursor >= len(f) {
		return store.Task{}, false
	}
	return f[m.cursor], true
}

// currentIndex locates the focused task in m.tasks, where edits land.
func (m Model) currentIndex() int {
	t, ok := m.current()
	if !ok {
		return -1
	}
	return indexOfTask(m.tasks, t.ID)
}

func indexOfTask(tasks []store.Task, id int) int {
	for i := range tasks {
		if tasks[i].ID == id {
			return i
		}
	}
	return -1
}

// pullEntries reads a task's timesheet lines from Odoo, once per task per sync.
func (m Model) pullEntries(taskID int) (Model, tea.Cmd) {
	if m.pulled[taskID] || m.key == "" || api.DB() == "" {
		return m, nil
	}
	m.pulled[taskID] = true // one attempt per task, success or not
	m.syncing = true
	return m, api.FetchEntries(m.key, m.login, taskID)
}

func (m *Model) clampCursor() {
	n := len(m.filtered())
	if m.cursor >= n {
		m.cursor = n - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *Model) clampRow() {
	t, ok := m.current()
	if !ok {
		m.row = 0
		return
	}
	if m.row >= len(t.Rows) {
		m.row = len(t.Rows) - 1
	}
	if m.row < 0 {
		m.row = 0
	}
}

// --- search ------------------------------------------------------------------

func (m Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.ClearQuery):
		m.search.SetValue("")
		m.expanded = map[int]bool{}
		m.clampCursor()
		return m, nil

	case key.Matches(msg, keys.Focus):
		m.search.Blur()
		m.mode = ModeList
		m.clampCursor()
		return m, nil
	}

	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)
	m.clampCursor()
	return m, cmd
}

// --- api key -----------------------------------------------------------------

// askKey opens the key prompt and says why it opened.
func (m Model) askKey(reason string) Model {
	m.mode = ModeAuth
	m.status = reason
	m.syncing = false
	m.auth.SetValue("")
	m.auth.Focus()
	m.search.Blur()
	m.jump.Blur()
	return m
}

func (m Model) startSync() (tea.Model, tea.Cmd) {
	if strings.TrimSpace(m.key) == "" {
		return m.askKey("An API key is needed to fetch your tasks."), textinput.Blink
	}
	m.syncing = true
	m.status = "syncing with " + api.BaseURL() + "…"
	m.err = nil

	cmds := []tea.Cmd{api.FetchTasks(m.key), api.FetchDayHours(m.key, parse.Today())}
	// A refresh re-reads the lines of whatever is open.
	m.pulled = map[int]bool{}
	for id := range m.expanded {
		var cmd tea.Cmd
		if m, cmd = m.pullEntries(id); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return m, tea.Batch(cmds...)
}

func (m Model) updateAuth(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Cancel):
		m.auth.SetValue("")
		m.auth.Blur()
		m.mode, m.status = ModeSearch, "no API key — working offline on "+store.Path()
		m.search.Focus()
		return m, textinput.Blink

	case key.Matches(msg, keys.Accept):
		v := strings.TrimSpace(m.auth.Value())
		if v == "" {
			return m, nil
		}
		m.key = v
		m.auth.SetValue("") // keep one copy of the key, in m.key
		m.auth.Blur()
		m.mode = ModeSearch
		m.search.Focus()
		m.syncing = true
		m.status = "syncing with " + api.BaseURL() + "…"
		return m, tea.Batch(store.SaveKey(m.key), api.FetchTasks(m.key), textinput.Blink)
	}

	var cmd tea.Cmd
	m.auth, cmd = m.auth.Update(msg)
	return m, cmd
}

// --- list --------------------------------------------------------------------

func (m Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	t, ok := m.current()

	switch {
	case key.Matches(msg, keys.Quit):
		return m, tea.Quit

	case key.Matches(msg, keys.Down):
		m.cursor++
		m.clampCursor()

	case key.Matches(msg, keys.Up):
		m.cursor--
		m.clampCursor()

	case key.Matches(msg, keys.Expand):
		// An empty task still opens: the table is where `a` adds the first entry.
		if ok {
			m.expanded[t.ID] = true
			m.row = 0
			m.mode = ModeTable
			return m.pullEntries(t.ID) // its lines live in Odoo, not on disk
		}

	case key.Matches(msg, keys.Collapse):
		if ok {
			delete(m.expanded, t.ID)
		}

	case key.Matches(msg, keys.Jump):
		if ok && len(t.Rows) > 0 {
			m.expanded[t.ID] = true
			m.row = 0
			m.jump.SetValue("")
			m.jump.Focus()
			m.mode = ModeJump
		}

	case key.Matches(msg, keys.Refresh):
		return m.startSync()

	case key.Matches(msg, keys.SetKey):
		return m.askKey("Replace the API key (current: " + store.MaskKey(m.key) + ")."), textinput.Blink

	case key.Matches(msg, keys.Search), key.Matches(msg, keys.Back):
		m.mode = ModeSearch
		m.search.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

// --- table -------------------------------------------------------------------

func (m Model) updateTable(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	t, ok := m.current()
	if !ok {
		m.mode = ModeList
		return m, nil
	}

	switch {
	case key.Matches(msg, keys.Down):
		m.row++
		m.clampRow()

	case key.Matches(msg, keys.Up):
		m.row--
		m.clampRow()

	case key.Matches(msg, keys.Edit):
		if len(t.Rows) == 0 {
			return m, nil
		}
		r := t.Rows[m.row]
		return m.openInsert(insertEdit, r.Date, r.Desc, parse.FormatHM(r.Minutes))

	case key.Matches(msg, keys.Add):
		return m.openInsert(insertNew, parse.Today(), "", "")

	case key.Matches(msg, keys.Delete):
		if len(t.Rows) == 0 {
			return m, nil
		}
		m.prev, m.mode = ModeTable, ModeConfirm
		m.cKind, m.cPrompt = confirmDeleteRow, "Delete this entry?"

	case key.Matches(msg, keys.Jump):
		m.jump.SetValue("")
		m.jump.Focus()
		m.mode = ModeJump
		return m, textinput.Blink

	case key.Matches(msg, keys.Collapse):
		delete(m.expanded, t.ID)
		m.mode = ModeList

	case key.Matches(msg, keys.Back):
		m.mode = ModeList

	case key.Matches(msg, keys.Search):
		delete(m.expanded, t.ID)
		m.mode = ModeSearch
		m.search.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

// --- insert ------------------------------------------------------------------

func (m Model) openInsert(kind insertKind, date, desc, hours string) (tea.Model, tea.Cmd) {
	m.kind = kind
	m.editRow = m.row
	m.fields[fieldDate].SetValue(date)
	m.fields[fieldDesc].SetValue(desc)
	m.fields[fieldHours].SetValue(hours)
	m.datePristine = true
	m.setFocus(fieldDate)
	m.mode = ModeInsert
	return m, textinput.Blink
}

func (m *Model) setFocus(i int) {
	m.focus = (i + fieldCount) % fieldCount
	for j := range m.fields {
		if j == m.focus {
			m.fields[j].Focus()
		} else {
			m.fields[j].Blur()
		}
	}
	if m.focus != fieldDate {
		m.datePristine = false
	}
}

// normalize rewrites a field on exit — never per keystroke.
func (m *Model) normalize(i int) {
	switch i {
	case fieldDate:
		base := parse.Today()
		if m.kind == insertEdit {
			if t, ok := m.current(); ok && m.editRow < len(t.Rows) {
				base = t.Rows[m.editRow].Date
			}
		}
		if d, err := parse.Date(m.fields[fieldDate].Value(), base); err == nil {
			m.fields[fieldDate].SetValue(d)
			m.err = nil
		} else {
			m.err = err
		}
	case fieldHours:
		v := strings.TrimSpace(m.fields[fieldHours].Value())
		if v == "" {
			return
		}
		if min, err := parse.Minutes(v); err == nil {
			m.fields[fieldHours].SetValue(parse.FormatHM(min))
			m.err = nil
		} else {
			m.err = err
		}
	}
}

func (m Model) updateInsert(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Next):
		m.normalize(m.focus)
		m.setFocus(m.focus + 1)
		return m, nil

	case key.Matches(msg, keys.Prev):
		m.normalize(m.focus)
		m.setFocus(m.focus - 1)
		return m, nil

	case key.Matches(msg, keys.ClearField):
		if m.focus < len(m.fields) {
			m.fields[m.focus].SetValue("")
			m.datePristine = false
		}
		return m, nil

	case key.Matches(msg, keys.Accept):
		switch m.focus {
		case fieldAccept:
			return m.commit()
		case fieldReject:
			return m.askDiscard(), nil
		default:
			m.normalize(m.focus)
			m.setFocus(m.focus + 1)
			return m, nil
		}

	case key.Matches(msg, keys.Cancel):
		if m.kind == insertEdit {
			m.mode = ModeTable
			m.err = nil
			return m, nil
		}
		return m.askDiscard(), nil
	}

	if m.focus >= len(m.fields) {
		return m, nil
	}
	if m.focus == fieldDate && m.datePristine && msg.Type == tea.KeyRunes {
		m.fields[fieldDate].SetValue("") // selected text is replaced by the first keystroke
	}
	m.datePristine = false

	var cmd tea.Cmd
	m.fields[m.focus], cmd = m.fields[m.focus].Update(msg)
	return m, cmd
}

func (m Model) askDiscard() Model {
	m.prev, m.mode = ModeInsert, ModeConfirm
	m.cKind, m.cPrompt = confirmDiscard, "Discard this entry?"
	return m
}

func (m Model) commit() (tea.Model, tea.Cmd) {
	m.normalize(fieldDate)
	m.normalize(fieldHours)

	min, err := parse.Minutes(m.fields[fieldHours].Value())
	if err != nil {
		m.err = err
		m.setFocus(fieldHours)
		return m, nil
	}
	date, err := parse.Date(m.fields[fieldDate].Value(), parse.Today())
	if err != nil {
		m.err = err
		m.setFocus(fieldDate)
		return m, nil
	}

	i := m.currentIndex()
	if i < 0 {
		m.mode = ModeTable
		return m, nil
	}
	// Local: the ERP has no copy of this, or no copy that matches.
	e := store.Entry{
		Date:    date,
		Desc:    strings.TrimSpace(m.fields[fieldDesc].Value()),
		Minutes: min,
		Local:   true,
	}

	if m.kind == insertNew {
		e.ID = store.NextEntryID(m.tasks[i])
		m.tasks[i].Rows = append([]store.Entry{e}, m.tasks[i].Rows...)
		m.row = 0
	} else {
		if m.editRow >= len(m.tasks[i].Rows) {
			m.mode = ModeTable
			return m, nil
		}
		e.ID = m.tasks[i].Rows[m.editRow].ID
		m.tasks[i].Rows[m.editRow] = e
		m.row = m.editRow
	}

	m.expanded[m.tasks[i].ID] = true
	m.err = nil
	m.mode = ModeTable
	return m, store.Save(m.tasks)
}

// --- jump --------------------------------------------------------------------

func (m Model) updateJump(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Cancel):
		m.jump.Blur()
		m.mode = ModeTable
		return m, nil

	case key.Matches(msg, keys.Accept):
		if t, ok := m.current(); ok {
			if i := findDay(t.Rows, m.jump.Value()); i >= 0 {
				m.row = i
			}
		}
		m.jump.Blur()
		m.mode = ModeTable
		return m, nil
	}

	if msg.Type == tea.KeyRunes { // digits and / only
		for _, r := range msg.Runes {
			if (r < '0' || r > '9') && r != '/' {
				return m, nil
			}
		}
	}
	var cmd tea.Cmd
	m.jump, cmd = m.jump.Update(msg)
	return m, cmd
}

// findDay returns the first row whose day (and month, if typed) matches.
func findDay(rows []store.Entry, q string) int {
	want := strings.Split(strings.TrimSpace(q), "/")
	if want[0] == "" {
		return -1
	}
	for i, r := range rows {
		got := strings.Split(r.Date, "/")
		match := true
		for j := range want {
			if want[j] == "" {
				continue
			}
			if j >= len(got) || !sameNumber(want[j], got[j]) {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func sameNumber(a, b string) bool {
	return strings.TrimLeft(a, "0") == strings.TrimLeft(b, "0")
}

// --- confirm -----------------------------------------------------------------

func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.No): // checked first: esc is in both sets
		m.mode = m.prev
		return m, nil

	case key.Matches(msg, keys.Yes):
		switch m.cKind {
		case confirmDeleteRow:
			i := m.currentIndex()
			if i < 0 || m.row >= len(m.tasks[i].Rows) {
				m.mode = ModeTable
				return m, nil
			}
			rows := m.tasks[i].Rows
			m.tasks[i].Rows = append(rows[:m.row:m.row], rows[m.row+1:]...)
			m.mode = ModeTable
			m.clampRow()
			return m, store.Save(m.tasks)

		case confirmDiscard:
			m.err = nil
			m.mode = ModeTable
			return m, nil
		}
	}
	return m, nil
}

// modeLabel is the top-right indicator.
func modeLabel(m Mode) string {
	switch m {
	case ModeSearch:
		return "-- SEARCH --"
	case ModeList:
		return "-- LIST --"
	case ModeTable:
		return "-- TABLE --"
	case ModeInsert:
		return "-- INSERT --"
	case ModeJump:
		return "-- JUMP --"
	case ModeConfirm:
		return "-- CONFIRM --"
	case ModeAuth:
		return "-- API KEY --"
	}
	return ""
}
