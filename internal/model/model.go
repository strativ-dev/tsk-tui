package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
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
	ModeDay
	ModeConfirm
	ModeAuth
)

// Tab is the top-level screen, above modes: the task list and everything reached from
// it is one tab, the month's hour chart is another. Modes belong to a tab.
type Tab int

const (
	TabTasks Tab = iota // where the app opens
	TabDash
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
	confirmQuit
	confirmCheckOut
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

	// spin animates while anything is in flight — a fetch, a pull, a write. spinning
	// says a tick is already scheduled, so work starting while other work runs does not
	// stack a second one and double the frame rate.
	spin     spinner.Model
	spinning bool

	// erpToday is what the ERP says was logged today, which beats the local
	// guess for the progress bar. -1 until a sync answers.
	erpToday int

	login string // the key owner's Odoo login, needed by JSON-RPC
	db    string // Odoo database, from the credential store — also a secret
	// pulled is the tasks Odoo has answered for; pulling is the ones in flight. A
	// failed pull stays out of both, so re-expanding tries again.
	pulled  map[int]bool
	pulling map[int]bool

	// showHelp opens the footer's key list. Closed by default: the keys are worth one
	// line of the screen when you want them, not on every screen forever.
	showHelp bool

	// tab is the screen; mode is the focus inside it.
	tab Tab
	// The dashboard's month, as the ERP reported it. dashMonth is the first of the
	// month it describes (YYYY-MM-01), so a late answer for another month is dropped.
	dashMonth   string
	dashDays    []api.DayLog
	dashLoading bool
	// dashWanted remembers that the chart is waiting on the login. JSON-RPC trades the
	// key for a uid using the owner's email, and that email only arrives with the REST
	// day-total answer — pressing d before the first sync lands would otherwise fail
	// with "sync first".
	dashWanted bool
	// The ERP's own clock. attEmp is the hr.employee behind the login, read once and kept
	// because it never changes; attKnown says an answer has landed, which is what makes
	// the c key safe — attendance_manual is a toggle, so firing it against a state we have
	// not read could check you out when you meant in. clocking is a toggle in flight.
	attEmp   int
	att      api.Attendance
	attKnown bool
	clocking bool
	// ticking says a 30s repaint is already scheduled for the elapsed figure, the same
	// guard spinning is for the spinner.
	ticking bool

	// dashHold is the day the chart's window is built around when the month is taller
	// than the terminal: -1 follows today, g and G pin it to the ends. There is no
	// cursor — the chart is a picture, not a list — so this only says what stays in view.
	dashHold int

	tasks    []store.Task
	cursor   int          // index into filtered()
	row      int          // row index inside the focused task
	expanded map[int]bool // task ID -> expanded

	// jumpDate is the date a jump from the list landed on, dd/mm/yy: one day, because
	// that is what the day modal reports on. jumpQuery is the other kind — the raw
	// query of a jump inside a task, matched part by part, so /12 finds the 12th of
	// any month among rows that are not all from this one. One or neither is set.
	//
	// Both outlive the prompt: the rows they match stay marked until the query is
	// cleared or another jump replaces it.
	jumpDate  string
	jumpQuery string
	// jumpInTask records where the prompt was opened. Inside a task's rows a jump is
	// a move — it walks the cursor to that date — while from the list there is no row
	// to move to, so it answers with the day modal instead.
	jumpInTask bool

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
	// Not focused at launch: the list is what you want to look at first. `i` or
	// ctrl+u puts the cursor in the field.

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

	spin := spinner.New(spinner.WithSpinner(spinner.Dot))
	spin.Style = theme.Bar // the accent, like the progress bar it sits under

	m := Model{
		mode:     ModeList, // the task list has focus at launch, not the query field
		spin:     spin,
		search:   search,
		jump:     jump,
		auth:     auth,
		expanded: map[int]bool{},
		erpToday: -1,
		pulled:   map[int]bool{},
		pulling:  map[int]bool{},
		dashHold: -1, // the chart opens on today, not on the 1st
	}
	// Sized for the fallback width until the first WindowSizeMsg lands, so the field
	// cannot wrap its own box on the very first frame either.
	m.search.Width = m.searchFieldWidth()
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

// Update runs the mode handlers and then keeps the two animations in step with the state:
// every command that puts work in flight starts the spinner from here, and being checked in
// starts the clock, so a new kind of fetch cannot forget either. One place rather than
// seven, because the model already knows what is outstanding.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	m, ok := next.(Model)
	if !ok {
		return next, cmd
	}
	if m.busy() && !m.spinning {
		m.spinning = true
		cmd = tea.Batch(cmd, m.spin.Tick)
	}
	if m.att.CheckedIn && !m.ticking {
		m.ticking = true
		cmd = tea.Batch(cmd, clockTick())
	}
	return m, cmd
}

// clockTickMsg repaints the elapsed figure beside the check-out button. The figure itself
// is derived from the check-in time on every render, so a late or dropped tick costs
// freshness and nothing else.
type clockTickMsg time.Time

func clockTick() tea.Cmd {
	return tea.Tick(30*time.Second, func(t time.Time) tea.Msg { return clockTickMsg(t) })
}

// busy is whether anything is waiting on the network: a task sync or write (syncing),
// the month's hour log (dashLoading), a check in or out (clocking), or a task's lines.
func (m Model) busy() bool {
	return m.syncing || m.dashLoading || m.clocking || len(m.pulling) > 0
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		// Stops of its own accord when the last request answers; Update starts it again
		// with the next one.
		if !m.busy() {
			m.spinning = false
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case clockTickMsg:
		// Stops itself once the clock is not running; Update starts it again on the next
		// check in. It keeps ticking on the task list: conditioning it on the tab would
		// mean a second place that schedules ticks, which is what the flag avoids, and a
		// repaint every 30 seconds is free.
		if !m.att.CheckedIn {
			m.ticking = false
			return m, nil
		}
		return m, clockTick()

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.search.Width = m.searchFieldWidth()
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
		m.db = msg.DB
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
			if m.dashWanted {
				// The chart was waiting for exactly this.
				return m.loadDash()
			}
		}
		if msg.Err != nil && m.dashWanted {
			m.dashWanted, m.dashLoading, m.syncing, m.clocking = false, false, false, false
			m.err = msg.Err
		}
		return m, nil

	case api.HourLogsMsg:
		m.syncing, m.dashLoading = false, false
		switch {
		case errors.Is(msg.Err, api.ErrUnauthorized):
			m.key = ""
			return m.askKey(msg.Err.Error()), textinput.Blink
		case msg.Err != nil:
			m.err = msg.Err
			m.status = "no hour log for this month"
			return m, nil
		}
		m.dashMonth, m.dashDays, m.dashHold = msg.Month, msg.Days, -1
		m.err = nil
		return m, nil

	case api.AttendanceMsg:
		// A read that was already in flight when a toggle started is thrown away: the
		// toggle's own read is newer, and the two answer the same question.
		if !msg.Toggled && m.clocking {
			return m, nil
		}
		if msg.Toggled {
			m.clocking = false
		}
		switch {
		case errors.Is(msg.Err, api.ErrUnauthorized):
			m.key = ""
			return m.askKey(msg.Err.Error()), textinput.Blink
		case msg.Err != nil:
			// The clock is the ERP's, so a failed call changes nothing here.
			m.err = msg.Err
			m.status = "attendance unchanged: " + oneLine(msg.Err.Error())
			return m, nil
		}

		m.att, m.attKnown = msg.At, true
		if msg.At.EmployeeID != 0 {
			m.attEmp = msg.At.EmployeeID
		}
		m.err = nil
		switch {
		case msg.Warning != "":
			// The ERP declined the toggle and said why; the state it reported alongside is
			// the truth, and it is already applied above.
			m.status = oneLine(msg.Warning)
		case msg.Toggled && msg.At.CheckedIn != msg.Want:
			// Never retry a toggle — a retry ping-pongs. Say what the server says.
			m.status = "the ERP says you are " + checkedLabel(msg.At.CheckedIn) + " — screen refreshed"
		case msg.Toggled && msg.At.CheckedIn:
			m.status = "checked in at " + clockTime(msg.At.Since)
		case msg.Toggled:
			m.status = "checked out at " + clockTime(time.Now())
		}
		return m, nil

	case api.LoggedMsg:
		m.syncing = false
		i, row := indexOfTask(m.tasks, msg.TaskID), -1
		if i >= 0 {
			row = indexOfEntry(m.tasks[i].Rows, msg.LocalID)
		}
		switch {
		case errors.Is(msg.Err, api.ErrUnauthorized):
			m.key = ""
			return m.askKey(msg.Err.Error()), textinput.Blink
		case msg.Err != nil:
			// The row stays Local, so the hours are still on screen and on disk.
			m.status = "not logged to the ERP: " + msg.Err.Error() + " — kept locally"
			return m, nil
		case i < 0 || row < 0:
			return m, nil
		}

		// Confirmed: the ERP owns it now, so it stops counting as unsynced.
		m.tasks[i].Rows[row].ID = msg.EntryID
		m.tasks[i].Rows[row].Local = false
		m.status = "logged " + parse.FormatTotal(msg.Minutes) + " to " + m.tasks[i].Key
		// The day total is now the ERP's business again.
		return m, tea.Batch(store.Save(m.tasks), api.FetchDayHours(m.key, parse.Today()))

	case api.UpdatedMsg:
		m.syncing = false
		i, row := indexOfTask(m.tasks, msg.TaskID), -1
		if i >= 0 {
			row = indexOfEntry(m.tasks[i].Rows, msg.EntryID)
		}
		switch {
		case errors.Is(msg.Err, api.ErrUnauthorized):
			m.key = ""
			return m.askKey(msg.Err.Error()), textinput.Blink
		case msg.Err != nil:
			m.status = "not updated in the ERP: " + msg.Err.Error() + " — kept locally"
			return m, nil
		case i < 0 || row < 0:
			return m, nil
		}
		m.tasks[i].Rows[row].Local = false // the ERP now matches what is on screen
		m.status = "updated " + parse.FormatTotal(msg.Minutes) + " in " + m.tasks[i].Key
		return m, tea.Batch(store.Save(m.tasks), api.FetchDayHours(m.key, parse.Today()))

	case api.DeletedMsg:
		m.syncing = false
		i, row := indexOfTask(m.tasks, msg.TaskID), -1
		if i >= 0 {
			row = indexOfEntry(m.tasks[i].Rows, msg.EntryID)
		}
		switch {
		case errors.Is(msg.Err, api.ErrUnauthorized):
			m.key = ""
			return m.askKey(msg.Err.Error()), textinput.Blink
		case msg.Err != nil:
			// Still on screen, because it is still in the ERP.
			m.status = "not deleted: " + msg.Err.Error()
			return m, nil
		case i < 0 || row < 0:
			return m, nil
		}
		m.tasks[i].Rows = m.dropRow(i, row)
		m.clampRow()
		m.status = "deleted the entry in " + m.tasks[i].Key
		return m, tea.Batch(store.Save(m.tasks), api.FetchDayHours(m.key, parse.Today()))

	case api.EntriesMsg:
		m.syncing = false
		delete(m.pulling, msg.TaskID)
		i := indexOfTask(m.tasks, msg.TaskID)
		switch {
		case errors.Is(msg.Err, api.ErrUnauthorized):
			m.key = ""
			return m.askKey(msg.Err.Error()), textinput.Blink
		case msg.Err != nil:
			// Not marked pulled: h then l tries again once the cause is fixed.
			m.err = msg.Err
			m.status = "no timesheet lines for this task"
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

		// A jump reads the tasks it has never seen, and this is one of the answers. The
		// modal is built from the tasks on every render, so these rows join it as they
		// land; the count in the status line has to keep up.
		if m.jumpDate != "" {
			m.status = jumpStatus(m.jumpDate, m.jumpHits(), len(m.pulling))
		}
		m.clampRow()
		return m, store.Save(m.tasks)

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		// A modal owns the keyboard while it is up, whichever tab is behind it.
		if m.mode == ModeConfirm {
			return m.updateConfirm(msg)
		}
		// Tabs and the key list come before modes, but never while a field is taking
		// letters — t, d and ? have to stay typeable in the search box and in an entry's
		// description.
		if m.mode != ModeSearch && m.mode != ModeInsert && m.mode != ModeJump && m.mode != ModeAuth {
			switch {
			case key.Matches(msg, keys.Help):
				m.showHelp = !m.showHelp
				return m, nil
			case key.Matches(msg, keys.DashTab):
				return m.showDash()
			case key.Matches(msg, keys.TasksTab):
				m.tab = TabTasks
				return m, nil
			}
		}
		// ModeAuth is excluded above so the key can be typed, which means the dash tab must
		// let it through too: a 401 on the hour log opens the prompt while this tab is up,
		// and routing here regardless left it unusable — every keystroke went to the chart.
		if m.tab == TabDash && m.mode != ModeAuth {
			return m.updateDash(msg)
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
		case ModeDay:
			return m.updateDay(msg)
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

// dropRow removes one entry from a task without disturbing the slice it shares
// with the copy of the model that is still on screen.
func (m Model) dropRow(task, row int) []store.Entry {
	rows := m.tasks[task].Rows
	if row < 0 || row >= len(rows) {
		return rows
	}
	return append(rows[:row:row], rows[row+1:]...)
}

func indexOfEntry(rows []store.Entry, id int) int {
	for i := range rows {
		if rows[i].ID == id {
			return i
		}
	}
	return -1
}

// pullEntries reads a task's timesheet lines from Odoo, once per task per sync.
// A failure is not recorded as a pull, so expanding the task again retries it —
// otherwise a missing db or a dropped connection would look like a task with no
// hours for the rest of the session.
func (m Model) pullEntries(taskID int) (Model, tea.Cmd) {
	if m.pulled[taskID] || m.pulling[taskID] || m.key == "" {
		return m, nil
	}
	m.pulling[taskID] = true
	m.syncing = true
	return m, api.FetchEntries(m.key, m.login, m.db, taskID)
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
		m.jumpDate, m.jumpQuery, m.status = "", "", "" // a clean search drops the marks
		m.clampCursor()
		return m, nil
		// ClearSearch is ctrl+u too, so ClearQuery above already handles it here:
		// in the field, ctrl+u clears and also collapses.

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
	m.pulled, m.pulling = map[int]bool{}, map[int]bool{}
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
		return m, tea.Batch(store.SaveKey(m.key, m.db), api.FetchTasks(m.key), textinput.Blink)
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
		// q is one key away from every other list key, so it asks first. ctrl+c
		// still leaves immediately.
		m.prev, m.mode = ModeList, ModeConfirm
		m.cKind, m.cPrompt = confirmQuit, "Quit tsk?"

	case key.Matches(msg, keys.ClearSearch):
		m.search.SetValue("")
		m.jumpDate, m.jumpQuery, m.status = "", "", ""
		m.mode = ModeSearch
		m.search.Focus()
		m.clampCursor() // the unfiltered list is longer than the filtered one
		return m, textinput.Blink

	case key.Matches(msg, keys.Down):
		m.cursor++
		m.clampCursor()

	case key.Matches(msg, keys.Up):
		m.cursor--
		m.clampCursor()

	case key.Matches(msg, keys.Top):
		m.cursor = 0

	case key.Matches(msg, keys.Bottom):
		m.cursor = len(m.filtered()) - 1
		m.clampCursor()

	case key.Matches(msg, keys.HalfDown):
		m.cursor += m.halfPage(taskLines)
		m.clampCursor()

	case key.Matches(msg, keys.HalfUp):
		m.cursor -= m.halfPage(taskLines)
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
		// From the list a jump reaches every task, so it needs neither this task open
		// nor any rows in it to be worth opening.
		m.jumpInTask = false
		m.jump.SetValue("")
		m.jump.Focus()
		m.mode = ModeJump
		return m, textinput.Blink

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

	case key.Matches(msg, keys.Top):
		m.row = 0

	case key.Matches(msg, keys.Bottom):
		m.row = len(t.Rows) - 1
		m.clampRow()

	case key.Matches(msg, keys.HalfDown):
		m.row += m.halfPage(entryLines)
		m.clampRow()

	case key.Matches(msg, keys.HalfUp):
		m.row -= m.halfPage(entryLines)
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
		if m.row >= len(t.Rows) {
			return m, nil
		}
		// Name the row. The unlink cannot be undone, so the modal has to say which
		// line is about to go, not just that one is.
		r := t.Rows[m.row]
		m.prev, m.mode = ModeTable, ModeConfirm
		m.cKind = confirmDeleteRow
		if desc := trunc(oneLine(r.Desc), 40); desc != "" {
			m.cPrompt = fmt.Sprintf("Delete this entry %q of %s?", desc, r.Date)
		} else {
			m.cPrompt = fmt.Sprintf("Delete the entry of %s?", r.Date)
		}

	case key.Matches(msg, keys.Jump):
		// Inside the rows a jump is a move, not a report: these rows are on screen, so
		// walk the cursor to the date instead of covering them with a modal.
		m.jumpInTask = true
		m.jump.SetValue("")
		m.jump.Focus()
		m.mode = ModeJump
		return m, textinput.Blink

	case key.Matches(msg, keys.Collapse):
		delete(m.expanded, t.ID)
		m.mode = ModeList

	case key.Matches(msg, keys.Back):
		m.mode = ModeList

	case key.Matches(msg, keys.Quit):
		// Same prompt as from the list, and "no" comes back to these rows rather than
		// collapsing the task you were reading.
		m.prev, m.mode = ModeTable, ModeConfirm
		m.cKind, m.cPrompt = confirmQuit, "Quit tsk?"

	case key.Matches(msg, keys.Search):
		delete(m.expanded, t.ID)
		m.mode = ModeSearch
		m.search.Focus()
		return m, textinput.Blink

	case key.Matches(msg, keys.ClearSearch):
		// Same as from the list, plus collapsing this task: ctrl+u means "back to a
		// clean search" wherever it is pressed.
		delete(m.expanded, t.ID)
		m.search.SetValue("")
		m.jumpDate, m.jumpQuery, m.status = "", "", ""
		m.mode = ModeSearch
		m.search.Focus()
		m.clampCursor()
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

		// A new entry belongs in the ERP, not only on disk. It stays Local until
		// the server confirms it, so a failed write cannot lose the hours.
		m.expanded[m.tasks[i].ID] = true
		m.err = nil
		m.mode = ModeTable
		m.syncing = true
		m.status = "logging " + parse.FormatTotal(e.Minutes) + " to " + m.tasks[i].Key + "…"
		return m, tea.Batch(
			store.Save(m.tasks),
			api.LogHours(m.key, m.tasks[i].Key, e.Date, e.Desc, e.Minutes, m.tasks[i].ID, e.ID),
		)
	}

	if m.editRow >= len(m.tasks[i].Rows) {
		m.mode = ModeTable
		return m, nil
	}
	e.ID = m.tasks[i].Rows[m.editRow].ID
	m.tasks[i].Rows[m.editRow] = e
	m.row = m.editRow

	m.expanded[m.tasks[i].ID] = true
	m.err = nil
	m.mode = ModeTable

	// A row the ERP owns is edited there too, over RPC `write`. It stays Local
	// until the server agrees, so a refused edit does not look saved.
	if e.ID > 0 {
		m.syncing = true
		m.status = "updating " + m.tasks[i].Key + " in the ERP…"
		return m, tea.Batch(
			store.Save(m.tasks),
			api.UpdateEntry(m.key, m.login, m.db, m.tasks[i].ID, e.ID, e.Date, e.Desc, e.Minutes),
		)
	}
	return m, store.Save(m.tasks)
}

// --- jump --------------------------------------------------------------------

func (m Model) updateJump(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Cancel):
		m.jump.Blur()
		m.err = nil
		m.mode = m.jumpReturn()
		return m, nil

	case key.Matches(msg, keys.Accept):
		m.jump.Blur()
		m.err = nil
		if strings.TrimSpace(m.jump.Value()) == "" {
			// Enter on an empty prompt is how a standing jump is called off.
			m.jumpDate, m.jumpQuery = "", ""
			m.status = ""
			m.mode = m.jumpReturn()
			return m, nil
		}
		return m.applyJump(m.jump.Value())
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

// jumpReturn is where the prompt hands focus back: into the rows if the task under
// the cursor is open, otherwise the task list.
func (m Model) jumpReturn() Mode {
	if t, ok := m.current(); ok && m.expanded[t.ID] {
		return ModeTable
	}
	return ModeList
}

// applyJump does one of two things with what was typed, depending on where the prompt
// was opened. Inside a task's rows it is a move: the cursor walks to the first row the
// query matches, part by part, and the rows stay on screen. From the list there is no
// row to move to, so it resolves one date and opens the day modal — what was logged on
// it across every task, without opening any of them.
func (m Model) applyJump(q string) (tea.Model, tea.Cmd) {
	m.jumpDate, m.jumpQuery = "", ""

	if m.jumpInTask {
		// Matched part by part rather than resolved against today: these rows span
		// months, so /12 has to mean the 12th of whichever month it turns up in.
		m.jumpQuery = strings.TrimSpace(q)
		m.mode = ModeTable
		t, ok := m.current()
		if !ok {
			return m, nil
		}
		hits, first := 0, -1
		for j, e := range t.Rows {
			if !m.onJumpDate(e) {
				continue
			}
			hits++
			if first < 0 {
				first = j
			}
		}
		if first >= 0 {
			m.row = first
		}
		m.status = fmt.Sprintf("%s — %d %s in this task", m.jumpQuery, hits,
			plural(hits, "entry", "entries"))
		return m, nil
	}

	// The day modal is about one day, so here the grammar is the date field's: 12 is
	// the 12th of this month, 12/07 the 12th of July, 12/07/26 exactly that.
	date, err := parse.Date(q, parse.Today())
	if err != nil {
		m.err = err
		m.jump.Focus() // stay on the prompt; the typed date is not a date
		return m, textinput.Blink
	}
	m.jumpDate = date
	m.mode = ModeDay

	// Read the tasks this jump cannot see yet. A pull returns a task's whole history,
	// so one with rows already on disk has nothing to add; one with none has never
	// been opened, and its hours would silently miss the day.
	var cmds []tea.Cmd
	unread := 0
	for _, t := range m.filtered() {
		if len(t.Rows) > 0 || m.pulled[t.ID] {
			continue
		}
		var cmd tea.Cmd
		if m, cmd = m.pullEntries(t.ID); cmd != nil {
			cmds = append(cmds, cmd)
			unread++
		}
	}

	m.status = jumpStatus(date, m.jumpHits(), unread)
	return m, tea.Batch(cmds...)
}

// updateDay is the day modal: it only closes. Nothing in it destroys anything, so esc
// needs no confirmation.
func (m Model) updateDay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Cancel), key.Matches(msg, keys.Accept),
		key.Matches(msg, keys.Collapse):
		m.mode = m.jumpReturn()
	}
	return m, nil
}

// dayRow is one line of the day modal: which task, what was done, how long.
type dayRow struct {
	key   string
	desc  string
	mins  int
	local bool // typed here and not in the ERP yet
}

// dayRows is what was logged on jumpDate, newest task order, derived on every render
// so lines that arrive from a pull join the modal on their own.
func (m Model) dayRows() (rows []dayRow, total int) {
	for _, t := range m.filtered() {
		name := t.Key
		if name == "" {
			name = t.Title
		}
		for _, e := range t.Rows {
			if !m.onJumpDate(e) {
				continue
			}
			rows = append(rows, dayRow{key: name, desc: e.Desc, mins: e.Minutes, local: e.Local})
			total += e.Minutes
		}
	}
	return rows, total
}

func jumpStatus(date string, hits, unread int) string {
	s := fmt.Sprintf("%s — no entries", date)
	if hits == 1 {
		s = fmt.Sprintf("%s — 1 entry", date)
	} else if hits > 1 {
		s = fmt.Sprintf("%s — %d entries", date, hits)
	}
	if unread > 0 {
		s += fmt.Sprintf(", reading %d more tasks…", unread)
	}
	return s
}

// onJumpDate reports whether a row is one the standing jump marked: one exact day for
// a jump from the list, or every date the query matches for one inside a task.
func (m Model) onJumpDate(e store.Entry) bool {
	switch {
	case m.jumpDate != "":
		return e.Date == m.jumpDate
	case m.jumpQuery != "":
		return parse.DateMatches(e.Date, m.jumpQuery)
	}
	return false
}

// jumpHits counts the marked rows across the list, derived rather than kept, since
// pulls and edits both change it.
func (m Model) jumpHits() int {
	n := 0
	for _, t := range m.filtered() {
		for _, e := range t.Rows {
			if m.onJumpDate(e) {
				n++
			}
		}
	}
	return n
}

// --- dashboard ---------------------------------------------------------------

// showDash opens the chart tab and reads the month if it has not been read yet. The
// ERP owns these numbers — nothing here is derived from the local task list, which only
// knows the tasks that have been opened.
func (m Model) showDash() (tea.Model, tea.Cmd) {
	m.tab = TabDash
	if m.dashMonth == thisMonth() || m.dashLoading {
		return m, nil // already have it, or it is on its way
	}
	return m.loadDash()
}

func (m Model) loadDash() (tea.Model, tea.Cmd) {
	if strings.TrimSpace(m.key) == "" {
		m.err = api.ErrNoKey
		return m, nil
	}
	m.dashLoading = true
	m.syncing = true
	if strings.TrimSpace(m.login) == "" {
		// Ask for the day total first: its answer carries the email the RPC login needs.
		m.dashWanted = true
		return m, api.FetchDayHours(m.key, parse.Today())
	}
	m.dashWanted = false
	cmds := []tea.Cmd{api.FetchHourLogs(m.key, m.login, m.db, time.Now())}
	// Not while a toggle is out: its own re-read is newer, and two answers to the same
	// question could land in the wrong order.
	if !m.clocking {
		cmds = append(cmds, api.FetchAttendance(m.key, m.login, m.db, m.attEmp))
	}
	return m, tea.Batch(cmds...)
}

// toggleClock presses the ERP's check in / check out button, or explains why it cannot.
// Both callers — the c key and the check-out modal — go through here, so neither can skip
// the guard: attendance_manual is a toggle, and firing it against a state we have not read
// could check you out when you meant in.
func (m Model) toggleClock() (tea.Model, tea.Cmd) {
	switch {
	case m.clocking:
		m.status = "still waiting on the ERP…"
		return m, nil
	case !m.attKnown || m.attEmp == 0:
		// Never a silent no-op: a key that looks dead gets pressed harder.
		m.status = "reading attendance…"
		return m, nil
	}

	want := !m.att.CheckedIn
	m.clocking = true
	m.err = nil
	if want {
		m.status = "checking in…"
	} else {
		m.status = "checking out…"
	}
	return m, api.ToggleAttendance(m.key, m.login, m.db, m.attEmp, want)
}

// updateDash is the chart tab: the month's ends, half a screen either way, a refresh, and
// the keys that leave for the task list. There is no cursor to walk a day at a time — the
// chart is one picture, not a list — so it moves in screenfuls: g and G to the ends,
// ctrl+f / ctrl+b by half of what is showing, and today in between, where it opens.
func (m Model) updateDash(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Top):
		return m.holdDash(0), nil
	case key.Matches(msg, keys.Bottom):
		return m.holdDash(len(m.dashDays) - 1), nil
	case key.Matches(msg, keys.HalfDown):
		return m.holdDash(m.dashDayIndex() + m.halfPage(dashRowLines)), nil
	case key.Matches(msg, keys.HalfUp):
		return m.holdDash(m.dashDayIndex() - m.halfPage(dashRowLines)), nil

	case key.Matches(msg, keys.Clock):
		// Checking in is one keystroke; checking out asks first, since it closes a session
		// the ERP then bills, and y alone answers it.
		if m.attKnown && m.att.CheckedIn && m.attEmp != 0 && !m.clocking {
			m.prev, m.mode = m.mode, ModeConfirm
			m.cKind = confirmCheckOut
			// The prompt states facts, not a prediction: it is frozen the moment it is
			// built, and the ERP stamps the check-out with its own clock.
			m.cPrompt = fmt.Sprintf("Check out now? (in since %s, %s)",
				clockTime(m.att.Since), parse.FormatHM(int(time.Since(m.att.Since).Minutes())))
			return m, nil
		}
		return m.toggleClock()

	case key.Matches(msg, keys.Refresh):
		m.dashMonth = "" // force a re-read of the month
		return m.loadDash()

	case key.Matches(msg, keys.Quit):
		m.prev, m.mode = m.mode, ModeConfirm
		m.cKind, m.cPrompt = confirmQuit, "Quit tsk?"
		return m, nil

	case key.Matches(msg, keys.Search), key.Matches(msg, keys.ClearSearch):
		// Searching is a task-list act, so it takes you back there.
		m.tab, m.mode = TabTasks, ModeSearch
		if key.Matches(msg, keys.ClearSearch) {
			m.search.SetValue("")
			m.clampCursor()
		}
		m.search.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

// holdDash pins the window to day i, clamped, so the ends of the month stop rather than
// wrap.
func (m Model) holdDash(i int) Model {
	m.dashHold = min(max(i, 0), max(len(m.dashDays)-1, 0))
	return m
}

// dashDayIndex is the row the window is built around: today, or the last day the month
// has if today is not among them. It is the day being logged into, so it is the one worth
// landing on — and the chart takes no motions, so it is the only row that ever holds the
// window.
func (m Model) dashDayIndex() int {
	if m.dashHold >= 0 {
		return min(m.dashHold, max(len(m.dashDays)-1, 0))
	}
	today := time.Now().Format("2006-01-02")
	for i, d := range m.dashDays {
		if d.Date == today {
			return i
		}
	}
	return max(len(m.dashDays)-1, 0)
}

// thisMonth is the first of the current month, the key the dashboard caches under.
func thisMonth() string { return time.Now().Format("2006-01") + "-01" }

// dashTotals sums what the month's rows say. The target and the working-day count are the
// **whole month** — 152:00 over 22 days, not the 88:00 owed so far — because that is the
// figure the month is billed against and the one worth watching a bar fill up towards.
// What has been logged, and the days it landed on, can only be counted up to today.
// Derived on render, never stored.
func (m Model) dashTotals() (logged, target float64, worked, workdays int) {
	today := time.Now().Format("2006-01-02")
	for _, d := range m.dashDays {
		if d.Expected > 0 {
			target += d.Expected
			workdays++
		}
		if d.Date > today { // ISO dates sort as strings
			continue
		}
		logged += d.Actual
		if d.Expected > 0 && d.Actual > 0 {
			worked++
		}
	}
	return logged, target, worked, workdays
}

// --- confirm -----------------------------------------------------------------

// confirmKeys is what accepts the open modal. Anything that destroys something —
// quitting, deleting an entry — takes y alone, so a stray enter cannot do it.
// Discarding an entry you are still typing keeps y or enter.
func (m Model) confirmKeys() key.Binding {
	switch m.cKind {
	case confirmQuit, confirmDeleteRow, confirmCheckOut:
		return keys.YesOnly
	}
	return keys.Yes
}

func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.No): // checked first: esc is in both sets
		m.mode = m.prev
		return m, nil

	case key.Matches(msg, m.confirmKeys()):
		switch m.cKind {
		case confirmDeleteRow:
			i := m.currentIndex()
			if i < 0 || m.row >= len(m.tasks[i].Rows) {
				m.mode = ModeTable
				return m, nil
			}
			m.mode = ModeTable

			// A row the ERP owns is unlinked there first; it stays on screen until
			// the server confirms, so a refusal cannot hide hours that still exist.
			if e := m.tasks[i].Rows[m.row]; e.ID > 0 {
				m.syncing = true
				m.status = "deleting the entry in the ERP…"
				return m, api.DeleteEntry(m.key, m.login, m.db, m.tasks[i].ID, e.ID)
			}
			m.tasks[i].Rows = m.dropRow(i, m.row)
			m.clampRow()
			return m, store.Save(m.tasks)

		case confirmDiscard:
			m.err = nil
			m.mode = ModeTable
			return m, nil

		case confirmCheckOut:
			// Back to whatever had the keyboard — the chart, not a task's rows, which is
			// what the other arms hard-code.
			m.mode = m.prev
			return m.toggleClock()

		case confirmQuit:
			return m, tea.Quit
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
	case ModeDay:
		return "-- DAY --"
	case ModeConfirm:
		return "-- CONFIRM --"
	case ModeAuth:
		return "-- API KEY --"
	}
	return ""
}
