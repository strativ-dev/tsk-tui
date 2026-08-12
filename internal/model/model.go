package model

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/tasnimAlam/tsk/internal/parse"
	"github.com/tasnimAlam/tsk/internal/store"
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
	fields [3]textinput.Model

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
	search.Prompt = "  "
	search.Placeholder = "search title or tag…"
	search.Width = 32
	search.Focus()

	jump := textinput.New()
	jump.Prompt = "/"
	jump.Width = 8
	jump.CharLimit = 8

	m := Model{
		mode:     ModeSearch,
		search:   search,
		jump:     jump,
		expanded: map[int]bool{},
	}
	for i, ph := range []string{"dd/mm/yy", "what you did", "7h30m"} {
		f := textinput.New()
		f.Prompt = ""
		f.Placeholder = ph
		f.Width = []int{10, 40, 8}[i]
		m.fields[i] = f
	}
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, store.Load)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
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
	for i := range m.tasks {
		if m.tasks[i].ID == t.ID {
			return i
		}
	}
	return -1
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
		if ok && len(t.Rows) > 0 {
			m.expanded[t.ID] = true
			m.row = 0
			m.mode = ModeTable
		}

	case key.Matches(msg, keys.Collapse):
		if ok {
			delete(m.expanded, t.ID)
		}

	case key.Matches(msg, keys.Jump):
		if ok && len(t.Rows) > 0 {
			m.expanded[t.ID] = true
			m.jump.SetValue("")
			m.jump.Focus()
			m.mode = ModeJump
		}

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
	e := store.Entry{Date: date, Desc: strings.TrimSpace(m.fields[fieldDesc].Value()), Minutes: min}

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
	}
	return ""
}
