package model

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
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
	// ModeLeaves is the month's own time off, listed in a modal over the calendar.
	ModeLeaves
	// ModeBook is the book-meal line under the meal calendar. It owns every key while it is
	// open, the way the new-timeoff line does: t means tomorrow there, not the tasks tab.
	ModeBook
	ModeConfirm
	ModeAuth
	ModeForm // the new-timeoff line on TabTime
	// ModeWFH is the work-from-home request line, opened by the ERP refusing a check in for
	// want of one. It owns every key while it is open, so its reason can hold a t or a d.
	ModeWFH
	// ModeReqForm is the new-requisition line. It owns every key while it is open, the way the
	// new-timeoff line does: its fields take letters.
	ModeReqForm
	// ModeEmpSearch is the employee tab's own query field. Its own mode, and its own input:
	// the task list's query filters tasks, and carrying one across would filter the other
	// screen by whatever was typed here.
	ModeEmpSearch
	// ModeProjSearch is the projects tab's own query field — a box in its header, focused with
	// i, exactly as the task list's is. Its own mode and its own input: the task query filters
	// tasks, and carrying one across would filter the other screen by whatever was typed here.
	ModeProjSearch
	// ModeProjJump is `/` on the projects tab: a prompt that looks for a person across every
	// project whose people are in hand, the way the date jump looks for a day across the tasks.
	ModeProjJump
	// ModeProjFound is the modal that answers it: the matches, grouped under the project they
	// are on. esc closes it and nothing else in it needs a key, since it destroys nothing.
	ModeProjFound
)

// Tab is the top-level screen, above modes: the task list and everything reached from
// it is one tab, the month's hour chart is another. Modes belong to a tab.
type Tab int

const (
	TabTasks Tab = iota // where the app opens
	TabDash
	TabTime
	TabMeal
	TabEmp
	TabReq
	TabProj
)

// k is the keymap this screen reads: the global one, or the tab's own where the config file
// gave it overrides. A method rather than a field, so there is no copy to keep in step with
// m.tab — every handler and the footer resolve it the same way, from the tab itself.
func (m Model) k() keyMap { return keysFor(m.tab) }

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
	confirmApplyLeave
	confirmDropLeave
	confirmDropMeals
	confirmBookMeals
	confirmDropForm
	confirmHourLogs
	confirmFileReq
	confirmDropReq
)

// The new-timeoff line's fields, in tab order. leaveTo is the range's end on a full day and
// the morning/afternoon dropdown on a half one — the same slot, since a half day is one day
// and has no end to give.
const (
	leaveKindField = iota
	leaveDurField
	leaveFromField
	leaveToField
	leaveDescField
	leaveOKField
	leaveXField
	leaveFieldCount
)

// leaveForm is the new-timeoff line. It is one struct so that discarding it is one
// assignment, and so nothing half-typed can outlive the line it was typed on.
type leaveForm struct {
	open  bool
	field int
	kind  int  // index into Model.timeKinds
	half  bool // full day / half day
	pm    bool // which half, when half
	// from and to are dd/mm/yy text fields; fresh marks one whose value is selected, so the
	// first keystroke replaces it rather than appending to it.
	from, to, desc textinput.Model
	fresh          [2]bool
}

// The book-meal line's scopes, in the order the dropdown cycles them. No letter of their own:
// `t` and `c` are the tasks tab and a leave filter everywhere else in the app, and a line that
// quietly took them back for one screen is a key meaning two things. j/k and space step this
// dropdown, the way they step every other one.
const (
	scopeToday = iota
	scopeTomorrow
	scopeWeek
	scopeCustom
	scopeCount
)

// reqForm is the new-requisition line. The fields are not fixed: the category says what it
// asks for, so the inputs are built when one is chosen and thrown away when it changes.
//
// vals is keyed by the property's own name rather than by index, since a late options answer or
// a re-chosen category must not land on the field that happens to sit in that slot. inputs is
// parallel to the chosen category's Fields, which is the one place the order lives.
type reqForm struct {
	open  bool
	field int
	cat   int // index into Model.reqCats, -1 until one is chosen
	// inputs is one per field the category asks for, in its order; a boolean or a many2one
	// field has no input and its slot is left zero.
	inputs []textinput.Model
	// on is the ticked booleans and picks, by property name: a boolean's value, or the index
	// into a many2one field's own options.
	on    map[string]bool
	picks map[string]int
	// urgent is the ERP's own is_urgent, and urgency its cause — one field on the line, the
	// other appearing only when it is ticked, since a cause for something not urgent is noise.
	urgent           bool
	urgency, noteBox textinput.Model
	// How much of the row's furniture the width can hold, decided by reqSizeInputs — the only
	// place that knows how wide the row came out. tight drops the space inside the boxes; the
	// three counts are how many cells the dropdown, a pick and a tick's own words get; label
	// keeps "new requisition" in front of it all.
	tight                      bool
	label                      bool
	catCells, pickCells, ticks int
}

// mealForm is the book-meal line: which days, which meals, and the two buttons. One struct,
// so closing it is one assignment and nothing half-typed outlives the line.
//
// on is keyed by serp.meal.type id rather than by index: the types come from the ERP, and a
// tick that survived a re-read into a different order would book the wrong meal.
type mealForm struct {
	open bool
	// drop is the cancel line rather than the book one: the same fields, the opposite verb.
	// One struct for both, since a row that can only hold one of them cannot hold two states.
	drop  bool
	field int
	scope int
	// from and to are dd/mm/yy text fields, used by the custom scope alone; fresh marks one
	// whose value is selected, so the first keystroke replaces it.
	from, to textinput.Model
	fresh    [2]bool
	on       map[int]bool
}

// The WFH request line's fields, in tab order: the two days, why, then the two buttons.
const (
	wfhFromField = iota
	wfhToField
	wfhReasonField
	wfhOKField
	wfhXField
	wfhFieldCount
)

// wfhForm is the work-from-home request line. One struct, so closing it is one assignment
// and nothing half-typed outlives the line it was typed on.
type wfhForm struct {
	open  bool
	field int
	// from and to are dd/mm/yy text fields; fresh marks one whose value is selected, so the
	// first keystroke replaces it rather than appending to it.
	from, to, reason textinput.Model
	fresh            [2]bool
}

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

	// wfh is the work-from-home request line. It is not opened by a key: the ERP refuses a
	// check in once the free WFH days are used up and names what it wants, so that refusal
	// opens it. wfhFiling is a create in flight.
	wfh       wfhForm
	wfhFiling bool
	// confirming is a confirm-hour-logs call in flight.
	confirming bool
	// wfhFiled says a request has already been filed this session, which is what stops the
	// line reappearing: the check in is retried after the request lands, and if the ERP still
	// wants an *approved* one it refuses with the same words — reopening the line there would
	// loop, and file the same days again.
	wfhFiled bool

	// dashHold is the day the chart's window is built around when the month is taller
	// than the terminal: -1 follows today, g and G pin it to the ends. There is no
	// cursor — the chart is a picture, not a list — so this only says what stays in view.
	dashHold int
	// dashOffset is the viewed month, in months from the current one: 0 is this month,
	// -1 last month, and so on. < and > move it; it never goes past 0 — the ERP has
	// nothing to report on a month that has not happened.
	dashOffset int

	// The time off year: the balances, the requests and the public holidays, all four
	// reads from one answer. timeYear is the year on screen — 0 until one lands — so it
	// doubles as the cache key, the way dashMonth does for the chart.
	timeYear     int
	timeKinds    []api.LeaveKind
	timeLeaves   []api.Leave
	timeHolidays []api.Holiday
	timeLoading  bool
	// timeWanted is the calendar waiting on the login, exactly as dashWanted is: RPC
	// needs the key owner's email, and that only arrives with the REST day total.
	timeWanted bool
	// timeHold is the month the window is built around, 0 for January, or -1 to follow
	// today. As on the chart there is no cursor — a calendar is a picture.
	timeHold int
	// timeEmp is the hr.employee a new request is filed for, from the same read that found
	// the working calendar.
	timeEmp int
	// One month of meals, the same one-in-hand rule the chart's month follows. mealMonth is
	// the cache key as well as what is on screen — year*12+month, 0 for nothing read yet —
	// and mealOffset is the viewed month in months from this one, so 0 is now and -1 is last
	// month.
	mealMonth    int
	mealOffset   int
	mealTypes    []api.MealType
	mealBookings []api.MealBooking
	mealMenus    []api.MealMenu
	// mealClosed is the ERP's own answer to which days the canteen is shut, weekends and
	// holidays together, keyed yyyy-mm-dd.
	mealClosed  map[string]bool
	mealLoading bool
	// mealHold is the day of the month the cursor is on, 0 to follow today. Unlike the year
	// calendar this screen has something to do to a day — x cancels its meals — so it needs
	// to say which one.
	mealHold int
	// book is the book-meal line. Closed, it is a label and nothing else.
	book mealForm
	// booking is a batch of creates in flight.
	booking bool
	// mealCancelling is an unlink in flight.
	mealCancelling bool
	// mealWanted is the meal calendar waiting on the login, exactly as dashWanted and
	// timeWanted are: RPC needs the key owner's email and only the REST day total carries it.
	mealWanted bool
	// The office directory. It is the same list every day — a name, a job title, an email —
	// so it is read once, cached on disk, and shown from the cache from then on; r re-reads
	// it. empQuery is this screen's own filter, matched against every field of a card.
	emps       []store.Employee
	empLoading bool
	// empWanted is the directory waiting on the login, exactly as dashWanted is: RPC needs
	// the key owner's email and only the REST day total carries it.
	empWanted bool
	empQuery  textinput.Model
	// empHold is the row the cursor is on: `l` opens that one, so the list needs to say which.
	empHold int
	// empOpen is the rows showing their detail, and empDetail what the ERP answered for them —
	// read once per employee, since a department and a team lead do not move while a terminal
	// is open. empPulling is the reads in flight, so a second `l` cannot ask twice.
	empOpen    map[int]bool
	empDetail  map[int]store.EmployeeDetail
	empPulling map[int]bool
	// The requisitions filed for the key's owner: read once a session, since a stage moves
	// while HR works and a cache on disk would be a promise this screen cannot keep. `R`
	// re-reads. reqOpen is the rows showing their own properties.
	reqs       []store.Requisition
	reqLoading bool
	reqWanted  bool
	reqHold    int
	reqOpen    map[int]bool
	// The office's open projects: read once a session, like the requisitions and for the same
	// reason — a task count moves while people work, so a cache on disk would answer this
	// screen's only question with yesterday's number. `R` re-reads.
	projs       []store.Project
	projLoading bool
	projWanted  bool
	projHold    int
	// projQuery is this tab's own filter — its own input, not the task list's, which filters
	// tasks. projOpen is the rows showing their manager and their people; projMembers is what
	// the ERP answered for them, read once per project, and projPulling the reads in flight so
	// a second `l` cannot ask twice.
	projQuery textinput.Model
	// projMine is the all/mine toggle, and it opens **on**: the list is 89 projects and nine
	// of them are yours, so the screen answers "what am I on" before it answers "what does
	// the office run".
	projMine bool
	// projFind is the last member search, kept after the prompt closes so the marks survive
	// scrolling — the same rule the date jump's own marks follow.
	projFind string
	// projFoundAt is the line the modal's own window is built around: ctrl+f and ctrl+b move
	// it, since a search with thirty hits does not fit a modal.
	projFoundAt int
	// find is the prompt that search is typed into — its own input again, since the query box
	// above it filters projects and this one looks inside them.
	find        textinput.Model
	projOpen    map[int]bool
	projMembers map[int][]store.Member
	projPulling map[int]bool
	// The categories a requisition can be filed under, and what each asks for: read once when
	// the line is first opened, since the office's own list of them does not move in a session.
	reqCats     []store.ReqCategory
	reqCatsRead bool
	// req is the new-requisition line. Closed, it is a label and nothing else.
	req reqForm
	// filing is a create in flight.
	filing bool
	// form is the new-timeoff line. Closed, it is a label and nothing else.
	form leaveForm
	// applying is a request for time off in flight.
	applying bool

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
		timeHold: -1, // and the calendar on this month, not on January
	}
	// Sized for the fallback width until the first WindowSizeMsg lands, so the field
	// cannot wrap its own box on the very first frame either.
	m.search.Width = m.searchFieldWidth()
	// The directory's own filter. Its own input, so a query typed here cannot filter tasks,
	// and a prompt rather than a box: it is opened with / and closed with esc, so it costs the
	// list no rows when it is not being typed into.
	m.empQuery = textinput.New()
	m.empQuery.Prompt = "search: "
	m.empQuery.PromptStyle = theme.Prompt
	m.empQuery.Width = 32
	m.empOpen = map[int]bool{}
	m.empDetail = map[int]store.EmployeeDetail{}
	m.empPulling = map[int]bool{}
	// The projects tab's own filter, on the same terms as the directory's: its own input, and
	// a prompt rather than a box.
	m.projQuery = textinput.New()
	m.projQuery.Prompt = "" // the ❯ caret sits outside the box, as on the task list
	m.projQuery.PromptStyle = theme.Prompt
	m.projQuery.Placeholder = "search a project"
	m.projQuery.Width = 32 // resized from the terminal, like the task query
	// The member prompt: `/` inside an open project, above the status line where the date
	// jump's own prompt renders.
	m.find = textinput.New()
	m.find.Prompt = "find "
	m.find.PromptStyle = theme.Prompt
	m.find.Width = 32
	m.projMine = true
	m.projOpen = map[int]bool{}
	m.projMembers = map[int][]store.Member{}
	m.projPulling = map[int]bool{}
	m.reqOpen = map[int]bool{}
	m.req.cat = -1
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
	return tea.Batch(textinput.Blink, store.Load, store.LoadEmployees, store.LoadProjects,
		store.LoadKey())
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
// the month's hour log (dashLoading), the year's time off (timeLoading), a request for
// time off (applying), a check in or out (clocking), or a task's lines.
func (m Model) busy() bool {
	return m.syncing || m.dashLoading || m.timeLoading || m.mealLoading || m.applying ||
		m.mealCancelling || m.booking || m.clocking || m.wfhFiling || m.confirming ||
		m.empLoading || m.reqLoading || m.projLoading || m.filing ||
		len(m.empPulling) > 0 || len(m.projPulling) > 0 || len(m.pulling) > 0
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
		m.projQuery.Width = m.projFieldWidth()
		m.fields[fieldDesc].Width = fieldWidth(m.descWidth())
		if m.form.open {
			m.form.desc.Width = m.leaveDescWidth()
		}
		if m.wfh.open {
			m.wfh.reason.Width = m.wfhReasonWidth()
		}
		if m.req.open {
			// Its fields are sized where they are made, so a resize has to size them again.
			m = m.reqSizeInputs()
		}
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
			// The chart or the calendar may have been waiting for exactly this. Both can
			// be, if d and o were pressed before the first sync answered.
			var cmds []tea.Cmd
			var cmd tea.Cmd
			// The clock is read **here**, on the login that makes it readable, rather than
			// when d opens the chart: it costs three round trips — a login, the employee
			// behind it, then the open session — and on this ERP that is about three seconds
			// with the check in button sitting dim for all of it. Started at launch it
			// overlaps the task list instead, and the button is live by the time the tab is.
			if !m.attKnown && !m.clocking && strings.TrimSpace(m.db) != "" {
				cmds = append(cmds, api.FetchAttendance(m.key, m.login, m.db, m.attEmp))
			}
			if m.dashWanted {
				m, cmd = m.loadDash()
				cmds = append(cmds, cmd)
			}
			if m.timeWanted {
				m, cmd = m.loadTime()
				cmds = append(cmds, cmd)
			}
			if m.mealWanted {
				m, cmd = m.loadMeal()
				cmds = append(cmds, cmd)
			}
			if m.empWanted {
				m, cmd = m.loadEmp()
				cmds = append(cmds, cmd)
			}
			if m.reqWanted {
				m, cmd = m.loadReq()
				cmds = append(cmds, cmd)
			}
			if m.projWanted {
				m, cmd = m.loadProj()
				cmds = append(cmds, cmd)
			}
			if len(cmds) > 0 {
				return m, tea.Batch(cmds...)
			}
		}
		if msg.Err != nil && (m.dashWanted || m.timeWanted || m.mealWanted || m.empWanted ||
			m.reqWanted || m.projWanted) {
			m.dashWanted, m.timeWanted, m.mealWanted = false, false, false
			m.empWanted, m.reqWanted, m.projWanted = false, false, false
			m.dashLoading, m.timeLoading, m.syncing, m.clocking = false, false, false, false
			m.mealLoading, m.empLoading, m.reqLoading = false, false, false
			m.projLoading = false
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

	case api.TimeOffMsg:
		m.timeLoading = false
		switch {
		case errors.Is(msg.Err, api.ErrUnauthorized):
			m.key = ""
			return m.askKey(msg.Err.Error()), textinput.Blink
		case msg.Err != nil:
			m.err = msg.Err
			m.status = "no time off for this year"
			return m, nil
		}
		// One answer, one screen: the balances, the requests and the holidays replace each
		// other together, so the calendar's days can never disagree with its own totals.
		m.timeYear, m.timeKinds = msg.Year, msg.Kinds
		m.timeLeaves, m.timeHolidays = msg.Leaves, msg.Holidays
		if msg.Employee != 0 {
			m.timeEmp = msg.Employee
		}
		// The window is left where it is when the line is open: a re-read after filing a
		// request must not scroll the month you were looking at off the screen.
		if !m.form.open {
			m.timeHold = -1
		}
		m.err = nil
		return m, nil

	case api.MealMsg:
		m.mealLoading = false
		switch {
		case errors.Is(msg.Err, api.ErrUnauthorized):
			m.key = ""
			return m.askKey(msg.Err.Error()), textinput.Blink
		case msg.Err != nil:
			m.err = msg.Err
			m.status = "no meals for " + msg.Month.String()
			return m, nil
		}
		// One answer, one calendar: the types, the bookings and the closed days replace each
		// other together, so a day can never disagree with the legend above it.
		m.mealMonth = msg.Year*12 + int(msg.Month)
		m.mealTypes, m.mealBookings, m.mealClosed = msg.Types, msg.Bookings, msg.Closed
		m.mealMenus = msg.Menus
		m.err = nil
		return m, nil

	case api.MealsDeletedMsg:
		m.mealCancelling = false
		switch {
		case errors.Is(msg.Err, api.ErrUnauthorized):
			m.key = ""
			return m.askKey(msg.Err.Error()), textinput.Blink
		case msg.Err != nil:
			// The day is left exactly as it was: a refused cancel must not hide a meal the
			// canteen is still counting on.
			m.err = msg.Err
			m.status = "the ERP kept " + mealDayLabel(msg.Date) + ": " + oneLine(msg.Err.Error())
			return m, nil
		}
		if m.book.open && m.book.drop {
			// What went is off the calendar behind it, which is a better answer than a form
			// still asking the same question.
			m.book = mealForm{}
			if m.mode == ModeBook {
				m.mode = ModeList
			}
		}
		m.status = fmt.Sprintf("cancelled %d %s", msg.N, plural(msg.N, "meal", "meals"))
		// The unlinked rows come off the day **now**. This is not a guess about what the ERP
		// will say — it has already said it, and the ids it confirmed are the ones dropped —
		// and the alternative was a day still drawing bars for meals nobody is serving until
		// the re-read landed, which is three round trips away.
		m.mealBookings = withoutBookings(m.mealBookings, msg.IDs)
		// Re-read all the same: someone else can book or cancel for you in the web client, so
		// the month is only ever what the ERP says it is. The drop above is what the screen
		// shows in the meantime, and the answer replaces it whole.
		return m.loadMeal()

	case api.MealBookedMsg:
		m.booking = false
		switch {
		case errors.Is(msg.Err, api.ErrUnauthorized):
			m.key = ""
			return m.askKey(msg.Err.Error()), textinput.Blink
		case msg.Err != nil:
			m.err = msg.Err
			m.status = "nothing was booked: " + oneLine(msg.Err.Error())
			return m, nil
		}
		// The line closes on anything the ERP took: what it booked is on the calendar behind
		// it, which is a better answer than a form still asking the same question.
		if msg.Booked > 0 {
			m.book = mealForm{}
			if m.mode == ModeBook {
				m.mode = ModeList
			}
		}
		m.status = fmt.Sprintf("booked %d %s", msg.Booked,
			plural(msg.Booked, "meal", "meals"))
		if msg.Skipped > 0 {
			// Whatever the ERP said about the ones it would not take, verbatim: it is usually
			// a rule this screen cannot see — a cutoff that passed while the line was open.
			m.status += fmt.Sprintf(", %d refused: %s", msg.Skipped, msg.Why)
		}
		// What it took goes on the day **now**, with the ids the ERP gave them: the re-read is
		// three round trips away, and until it landed the day drew open slots for meals that
		// are booked. The re-read still happens and replaces the month whole — someone else
		// can book for you in the web client, so the month is only ever what the ERP says.
		m.mealBookings = append(withoutBookings(m.mealBookings, rowIDs(msg.Rows)), msg.Rows...)
		return m.loadMeal()

	case api.LeaveRequestedMsg:
		m.applying = false
		switch {
		case errors.Is(msg.Err, api.ErrUnauthorized):
			m.key = ""
			return m.askKey(msg.Err.Error()), textinput.Blink
		case msg.Err != nil:
			// The line is still on screen with everything in it, so the request can be fixed
			// and filed again rather than typed again — and the year is re-read, because what
			// refused it is usually a leave this screen has not seen. Someone can file one
			// for you in the web client, so the calendar can never be fresh by itself.
			//
			// The reason goes in the **status**, not only in err: the re-read clears err when
			// it lands, which left a refusal on screen as the word "refused" and nothing else.
			// Whatever the ERP said is the one thing worth keeping.
			why := oneLine(msg.Err.Error())
			m.err = msg.Err
			m.status = "the ERP refused it: " + why
			if strings.Contains(strings.ToLower(why), "overlap") {
				m.status = "those days already have time off — " + why
			}
			return m.loadTime()
		}
		// Filed: the line closes, and the year is re-read so the days show up where the
		// calendar says they are rather than where this screen guessed.
		m.form = leaveForm{}
		m.mode = ModeList
		m.err = nil
		// state defaults to confirm on create, so the request is submitted, not a draft: it
		// is waiting on the leave type's approvers, two of them for most types here.
		m.status = "time off requested — waiting on approval"
		return m.loadTime()

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
			// A read that failed while the clock is not on screen says nothing worth saying:
			// the launch prefetch above is nobody's request, and its refusal on the task list
			// would read as something the task list did. A toggle always reports.
			if !msg.Toggled && m.tab != TabDash {
				return m, nil
			}
			// The clock is the ERP's, so a failed call changes nothing here.
			m.err = msg.Err
			m.status = "attendance unchanged: " + oneLine(msg.Err.Error())
			// One refusal names the thing that would fix it: "you have exceeded the number of
			// days available for WFH, please submit a WFH request". The line is the shortest
			// way from that sentence to the request, so the error opens it rather than only
			// reporting it. Once one has been filed it stays shut — see wfhFiled.
			if needsWFH(msg.Err) && !m.wfh.open && !m.wfhFiled {
				return m.openWFH()
			}
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
			// In: whatever the WFH days were short by is no longer in the way, so the next
			// refusal of this kind gets the line again.
			m.wfhFiled = false
			m.status = "checked in at " + clockTime(msg.At.Since)
		case msg.Toggled:
			m.status = "checked out at " + clockTime(time.Now())
		}
		return m, nil

	case store.EmployeesLoadedMsg:
		// The cache, off disk at launch. An error here is not worth a status line of its own:
		// the tab says it has nothing and r reads the ERP.
		if msg.Err == nil {
			m.emps = msg.Employees
		}
		return m, nil

	case api.EmployeesMsg:
		m.empLoading = false
		if msg.Err != nil {
			// The cache stays: a failed re-read must not empty a directory that was on screen.
			m.err = msg.Err
			m.status = "the directory is unchanged: " + oneLine(msg.Err.Error())
			return m, nil
		}
		m.emps, m.err = msg.Employees, nil
		m.empHold = min(m.empHold, max(len(m.empRows())-1, 0))
		m.status = fmt.Sprintf("%d %s from the ERP",
			len(msg.Employees), plural(len(msg.Employees), "employee", "employees"))
		return m, store.SaveEmployees(msg.Employees)

	case api.ReqCategoriesMsg:
		m.reqLoading = false
		if msg.Err != nil {
			m.err = msg.Err
			m.status = "could not read the categories: " + oneLine(msg.Err.Error())
			return m, nil
		}
		m.reqCats, m.reqCatsRead, m.err = msg.Categories, true, nil
		return m, nil

	case api.ReqOptionsMsg:
		if msg.Err != nil {
			m.err = msg.Err
			m.status = "could not read the choices: " + oneLine(msg.Err.Error())
			return m, nil
		}
		// Landed on the field it was read for, by name: a re-chosen category renumbers the
		// fields, so an index would put the office's devices in somebody else's slot.
		for i := range m.reqCats {
			for j := range m.reqCats[i].Fields {
				if m.reqCats[i].Fields[j].Name == msg.Field {
					m.reqCats[i].Fields[j].Opts = msg.Options
				}
			}
		}
		return m, nil

	case api.RequisitionFiledMsg:
		m.filing = false
		if msg.Err != nil {
			// The line stays exactly as typed: a refusal has to have something to come back to.
			m.err = msg.Err
			m.status = "the ERP refused it: " + oneLine(msg.Err.Error())
			return m, nil
		}
		// Filed: the line closes and the table is re-read, so the new row shows up where the
		// ERP says it is rather than where this screen guessed.
		m.req = reqForm{cat: -1}
		m.mode, m.err = ModeList, nil
		m.status = "requisition filed — waiting on approval"
		return m.loadReq()

	case api.RequisitionsMsg:
		m.reqLoading = false
		if msg.Err != nil {
			// Whatever was on screen stays: a failed re-read must not empty a list that was
			// answering the question a moment ago.
			m.err = msg.Err
			m.status = "the requisitions are unchanged: " + oneLine(msg.Err.Error())
			return m, nil
		}
		m.reqs, m.err = msg.Rows, nil
		m.reqHold = min(m.reqHold, max(len(m.reqs)-1, 0))
		m.status = fmt.Sprintf("%d %s from the ERP",
			len(msg.Rows), plural(len(msg.Rows), "requisition", "requisitions"))
		return m, nil

	case api.ProjectsMsg:
		m.projLoading = false
		if msg.Err != nil {
			// Whatever was on screen stays, the same as a failed requisition re-read: an
			// empty list is not the answer "no projects".
			m.err = msg.Err
			m.status = "the projects are unchanged: " + oneLine(msg.Err.Error())
			return m, nil
		}
		// The ERP does not answer with the people — that is a read per project — so a refresh
		// carries over what is already in hand, and only while the member ids are unchanged:
		// new ids mean the names beside them are somebody else's.
		m.projs, m.err = keepPeople(m.projs, msg.Projects), nil
		m.projMembers = map[int][]store.Member{}
		for _, p := range m.projs {
			if len(p.People) > 0 {
				m.projMembers = withVal(m.projMembers, p.ID, p.People)
			}
		}
		m.projHold = min(m.projHold, max(len(m.projRows())-1, 0))
		m.status = fmt.Sprintf("%d open %s from the ERP",
			len(msg.Projects), plural(len(msg.Projects), "project", "projects"))
		return m, store.SaveProjects(msg.Projects)

	case store.ProjectsLoadedMsg:
		if msg.Err != nil {
			// A cache that will not parse is not worth a message: the tab fetches instead.
			return m, nil
		}
		m.projs = msg.Projects
		// The people the cache holds are people already read: seeding the map here is what
		// stops `l` asking for them again on the first open after a restart.
		for _, p := range msg.Projects {
			if len(p.People) > 0 {
				m.projMembers = withVal(m.projMembers, p.ID, p.People)
			}
		}
		return m, nil

	case api.ProjectMembersMsg:
		m.projPulling = withoutKey(m.projPulling, msg.ID)
		if msg.Err != nil {
			m.err = msg.Err
			m.status = "could not read that project's people: " + oneLine(msg.Err.Error())
			return m, nil
		}
		// An empty answer is an answer: a project whose teams have nobody on them says so,
		// rather than asking again on the next keystroke.
		m.projMembers = withVal(m.projMembers, msg.ID, msg.Members)
		m.err = nil
		// And it goes on the cached record, so a restart does not ask again: this is the same
		// reason the list itself is on disk.
		next := make([]store.Project, len(m.projs))
		copy(next, m.projs)
		for i := range next {
			if next[i].ID == msg.ID {
				next[i].People = msg.Members
			}
		}
		m.projs = next
		return m, store.SaveProjects(m.projs)

	case api.EmployeeMsg:
		delete(m.empPulling, msg.ID)
		if msg.Err != nil {
			m.err = msg.Err
			m.status = "could not read that employee: " + oneLine(msg.Err.Error())
			return m, nil
		}
		// Copied on write for the same reason the open set is: a map inside a value model.
		next := make(map[int]store.EmployeeDetail, len(m.empDetail)+1)
		for k, v := range m.empDetail {
			next[k] = v
		}
		next[msg.ID] = msg.Detail
		m.empDetail, m.err = next, nil
		return m, nil

	case api.HoursConfirmedMsg:
		m.confirming = false
		month := msg.Month
		if t, err := time.Parse("2006-01-02", msg.Month); err == nil {
			month = t.Format("January 2006")
		}
		if msg.Err != nil {
			m.err = msg.Err
			m.status = "the ERP refused it: " + oneLine(msg.Err.Error())
			return m, nil
		}
		m.err = nil
		if msg.Count == 0 {
			m.status = month + " was already confirmed"
			return m, nil
		}
		// The month is re-read rather than trusted: confirm_hour_logs answers with a bare
		// false even when it wrote, so the chart is the only honest report of what happened.
		m.status = fmt.Sprintf("confirmed %d %s of %s",
			msg.Count, plural(msg.Count, "hour log", "hour logs"), month)
		return m.loadDash()

	case api.WFHRequestedMsg:
		m.wfhFiling = false
		if msg.Err != nil {
			m.err = msg.Err
			if msg.ID == 0 {
				// Nothing was filed, so the line stays exactly as typed: a refusal has to
				// have something to come back to.
				m.status = "the ERP refused the WFH request: " + oneLine(msg.Err.Error())
				return m, nil
			}
			// The record exists. Re-filing would ask HR for the same days twice, so the line
			// closes and the ERP's own words stand.
			m.wfh, m.mode, m.wfhFiled = wfhForm{}, ModeList, true
			m.status = oneLine(msg.Err.Error())
			return m, nil
		}
		// Filed and submitted, so the check in is worth another try — that is what the
		// request was for, and the ERP has the last word on whether it is enough.
		m.wfh, m.mode, m.err = wfhForm{}, ModeList, nil
		m.wfhFiled = true
		m.status = "WFH request submitted — checking in…"
		return m.toggleClock()

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
		// A screen that bound one of its own actions to a tab key keeps it there: the
		// per-tab table is an explicit "on this screen, this key does that", so it is
		// matched first and that tab loses the shortcut. Only that tab — everywhere else
		// the key is still the tab it always was.
		if m.mode != ModeSearch && m.mode != ModeInsert && m.mode != ModeJump &&
			m.mode != ModeAuth && m.mode != ModeForm && m.mode != ModeBook &&
			m.mode != ModeWFH && m.mode != ModeEmpSearch && m.mode != ModeReqForm &&
			m.mode != ModeProjSearch && m.mode != ModeProjJump &&
			m.mode != ModeProjFound && !claims(m.tab, msg) {
			switch {
			case key.Matches(msg, m.k().Help):
				m.showHelp = !m.showHelp
				return m, nil
			case key.Matches(msg, m.k().DashTab):
				return m.showDash()
			case key.Matches(msg, m.k().TimeTab):
				return m.showTime()
			case key.Matches(msg, m.k().MealTab):
				return m.showMeal()
			case key.Matches(msg, m.k().EmpTab):
				return m.showEmp()
			case key.Matches(msg, m.k().ReqTab):
				return m.showReq()
			case key.Matches(msg, m.k().ProjTab):
				return m.showProj()
			case key.Matches(msg, m.k().TasksTab):
				m.tab = TabTasks
				return m, nil
			}
		}
		// ModeAuth is excluded above so the key can be typed, which means the dash tab must
		// let it through too: a 401 on the hour log opens the prompt while this tab is up,
		// and routing here regardless left it unusable — every keystroke went to the chart.
		// The new-timeoff line owns the keyboard while it is open: its description has to be
		// able to hold a t, a d and an o, so it is excluded above and routed here first.
		if m.mode == ModeForm {
			return m.updateForm(msg)
		}
		// A modal over the calendar owns the keyboard the same way: routed before the tab
		// handlers, or h/l would walk the months behind a list that says which month it is.
		if m.mode == ModeLeaves {
			return m.updateLeaves(msg)
		}
		// The book-meal line owns the keyboard while it is open: its scope keys are t, w and
		// c, which are the tasks tab, nothing, and a leave filter everywhere else.
		if m.mode == ModeBook {
			return m.updateBook(msg)
		}
		// And the WFH request line: it sits over the chart, whose own keys are letters, so a
		// reason that says "at the dentist" must not step the month while it is typed.
		if m.mode == ModeWFH {
			return m.updateWFH(msg)
		}
		// And the new-requisition line, for the same reason: a purpose can hold a t and a d.
		if m.mode == ModeReqForm {
			return m.updateReqForm(msg)
		}
		if m.mode != ModeAuth {
			switch m.tab {
			case TabDash:
				return m.updateDash(msg)
			case TabTime:
				return m.updateTime(msg)
			case TabMeal:
				return m.updateMeal(msg)
			case TabEmp:
				return m.updateEmp(msg)
			case TabReq:
				return m.updateReq(msg)
			case TabProj:
				return m.updateProj(msg)
			}
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
	case key.Matches(msg, m.k().ClearQuery):
		m.search.SetValue("")
		m.expanded = map[int]bool{}
		m.jumpDate, m.jumpQuery, m.status = "", "", "" // a clean search drops the marks
		m.clampCursor()
		return m, nil
		// ClearSearch is ctrl+u too, so ClearQuery above already handles it here:
		// in the field, ctrl+u clears and also collapses.

	case key.Matches(msg, m.k().Focus):
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
	case key.Matches(msg, m.k().Cancel):
		m.auth.SetValue("")
		m.auth.Blur()
		m.mode, m.status = ModeSearch, "no API key — working offline on "+store.Path()
		m.search.Focus()
		return m, textinput.Blink

	case key.Matches(msg, m.k().Accept):
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
	case key.Matches(msg, m.k().Quit):
		// q is one key away from every other list key, so it asks first. ctrl+c
		// still leaves immediately.
		m.prev, m.mode = ModeList, ModeConfirm
		m.cKind, m.cPrompt = confirmQuit, "Quit tsk?"

	case key.Matches(msg, m.k().ClearSearch):
		m.search.SetValue("")
		m.jumpDate, m.jumpQuery, m.status = "", "", ""
		m.mode = ModeSearch
		m.search.Focus()
		m.clampCursor() // the unfiltered list is longer than the filtered one
		return m, textinput.Blink

	case key.Matches(msg, m.k().Down):
		m.cursor++
		m.clampCursor()

	case key.Matches(msg, m.k().Up):
		m.cursor--
		m.clampCursor()

	case key.Matches(msg, m.k().Top):
		m.cursor = 0

	case key.Matches(msg, m.k().Bottom):
		m.cursor = len(m.filtered()) - 1
		m.clampCursor()

	case key.Matches(msg, m.k().HalfDown):
		m.cursor += m.halfPage(taskLines)
		m.clampCursor()

	case key.Matches(msg, m.k().HalfUp):
		m.cursor -= m.halfPage(taskLines)
		m.clampCursor()

	case key.Matches(msg, m.k().Expand):
		// An empty task still opens: the table is where `a` adds the first entry.
		if ok {
			m.expanded[t.ID] = true
			m.row = 0
			m.mode = ModeTable
			return m.pullEntries(t.ID) // its lines live in Odoo, not on disk
		}

	case key.Matches(msg, m.k().Collapse):
		if ok {
			delete(m.expanded, t.ID)
		}

	case key.Matches(msg, m.k().Jump):
		// From the list a jump reaches every task, so it needs neither this task open
		// nor any rows in it to be worth opening.
		m.jumpInTask = false
		m.jump.SetValue("")
		m.jump.Focus()
		m.mode = ModeJump
		return m, textinput.Blink

	case key.Matches(msg, m.k().Refresh):
		return m.startSync()

	case key.Matches(msg, m.k().SetKey):
		return m.askKey("Replace the API key (current: " + store.MaskKey(m.key) + ")."), textinput.Blink

	case key.Matches(msg, m.k().Search), key.Matches(msg, m.k().Back):
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
	case key.Matches(msg, m.k().Down):
		m.row++
		m.clampRow()

	case key.Matches(msg, m.k().Up):
		m.row--
		m.clampRow()

	case key.Matches(msg, m.k().Top):
		m.row = 0

	case key.Matches(msg, m.k().Bottom):
		m.row = len(t.Rows) - 1
		m.clampRow()

	case key.Matches(msg, m.k().HalfDown):
		m.row += m.halfPage(entryLines)
		m.clampRow()

	case key.Matches(msg, m.k().HalfUp):
		m.row -= m.halfPage(entryLines)
		m.clampRow()

	case key.Matches(msg, m.k().Edit):
		if len(t.Rows) == 0 {
			return m, nil
		}
		r := t.Rows[m.row]
		return m.openInsert(insertEdit, r.Date, r.Desc, parse.FormatHM(r.Minutes))

	case key.Matches(msg, m.k().Add):
		return m.openInsert(insertNew, parse.Today(), "", "")

	case key.Matches(msg, m.k().Delete):
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

	case key.Matches(msg, m.k().Jump):
		// Inside the rows a jump is a move, not a report: these rows are on screen, so
		// walk the cursor to the date instead of covering them with a modal.
		m.jumpInTask = true
		m.jump.SetValue("")
		m.jump.Focus()
		m.mode = ModeJump
		return m, textinput.Blink

	case key.Matches(msg, m.k().Collapse), key.Matches(msg, m.k().Back):
		// esc collapses as h does: the rows are the thing esc undoes here, the same as it
		// undoes every other thing a key opened. Left open with the keys back on the task
		// line, the table sat there advertising `a` for a row the keys could not add.
		delete(m.expanded, t.ID)
		m.mode = ModeList

	case key.Matches(msg, m.k().Quit):
		// Same prompt as from the list, and "no" comes back to these rows rather than
		// collapsing the task you were reading.
		m.prev, m.mode = ModeTable, ModeConfirm
		m.cKind, m.cPrompt = confirmQuit, "Quit tsk?"

	case key.Matches(msg, m.k().Search):
		delete(m.expanded, t.ID)
		m.mode = ModeSearch
		m.search.Focus()
		return m, textinput.Blink

	case key.Matches(msg, m.k().ClearSearch):
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
	case key.Matches(msg, m.k().Next):
		m.normalize(m.focus)
		m.setFocus(m.focus + 1)
		return m, nil

	case key.Matches(msg, m.k().Prev):
		m.normalize(m.focus)
		m.setFocus(m.focus - 1)
		return m, nil

	case key.Matches(msg, m.k().ClearField):
		if m.focus < len(m.fields) {
			m.fields[m.focus].SetValue("")
			m.datePristine = false
		}
		return m, nil

	case key.Matches(msg, m.k().Accept):
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

	case key.Matches(msg, m.k().Cancel):
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
	case key.Matches(msg, m.k().Cancel):
		m.jump.Blur()
		m.err = nil
		m.mode = m.jumpReturn()
		return m, nil

	case key.Matches(msg, m.k().Accept):
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
	//
	// **Every task, not the filtered ones**: this is a report on a day across the whole
	// list, and a query typed to find one task would otherwise leave the other tasks'
	// hours out of it — silently, since the modal has no way to say what it did not read.
	var cmds []tea.Cmd
	unread := 0
	for _, t := range m.tasks {
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
	case key.Matches(msg, m.k().Cancel), key.Matches(msg, m.k().Accept),
		key.Matches(msg, m.k().Collapse):
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
	// Every task the list holds, filtered or not: the question is what the day went on, and
	// the search field answers a different one — which task you were looking for.
	for _, t := range m.tasks {
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
	// The same list dayRows counts, or the status line and the modal would disagree about
	// how many entries a day holds.
	for _, t := range m.tasks {
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
	if m.dashMonth == m.targetMonthKey() || m.dashLoading {
		return m, nil // already have it, or it is on its way
	}
	return m.loadDash()
}

// targetMonth is the month dashOffset points at, any timestamp inside it — what
// FetchHourLogs wants. targetMonthKey is the same month as the cache key HourLogsMsg
// carries, so showDash and loadDash can tell an already-loaded month from one still to
// fetch without repeating the time arithmetic.
func (m Model) targetMonth() time.Time { return time.Now().AddDate(0, m.dashOffset, 0) }
func (m Model) targetMonthKey() string { return m.targetMonth().Format("2006-01") + "-01" }

func (m Model) loadDash() (Model, tea.Cmd) {
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
	cmds := []tea.Cmd{api.FetchHourLogs(m.key, m.login, m.db, m.targetMonth())}
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

// --- employees ---------------------------------------------------------------

// showEmp opens the directory. The cache is what it shows: the list is read once and kept on
// disk, since a name and a job title do not change between two openings of a terminal. `r`
// is how it is re-read.
func (m Model) showEmp() (tea.Model, tea.Cmd) {
	m.tab = TabEmp
	if len(m.emps) > 0 || m.empLoading {
		return m, nil // the cache answers, or the read is already out
	}
	return m.loadEmp()
}

// loadEmp reads the whole directory in one call.
func (m Model) loadEmp() (Model, tea.Cmd) {
	if strings.TrimSpace(m.key) == "" {
		m.err = api.ErrNoKey
		return m, nil
	}
	m.empLoading = true
	if strings.TrimSpace(m.login) == "" {
		// The day total's answer carries the email the RPC login needs.
		m.empWanted = true
		return m, api.FetchDayHours(m.key, parse.Today())
	}
	m.empWanted = false
	return m, api.FetchEmployees(m.key, m.login, m.db)
}

// updateEmp is the directory: a filter and a window over it, and nothing that writes. There
// is no cursor to act with — a card is something to read — so the motions only say which
// card the window is built around.
func (m Model) updateEmp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.mode == ModeEmpSearch {
		return m.updateEmpSearch(msg)
	}
	switch {
	case key.Matches(msg, m.k().Down):
		return m.holdEmp(m.empHold + 1), nil
	case key.Matches(msg, m.k().Up):
		return m.holdEmp(m.empHold - 1), nil
	case key.Matches(msg, m.k().Top):
		return m.holdEmp(0), nil
	case key.Matches(msg, m.k().Bottom):
		return m.holdEmp(len(m.empRows()) - 1), nil
	case key.Matches(msg, m.k().HalfDown):
		return m.holdEmp(m.empHold + m.halfPage(empCardLines)), nil
	case key.Matches(msg, m.k().HalfUp):
		return m.holdEmp(m.empHold - m.halfPage(empCardLines)), nil

	case key.Matches(msg, m.k().Expand):
		// l opens the row under the cursor and reads its detail once: a department and a team
		// lead do not move while a terminal is open, so a second open costs nothing.
		return m.openEmp()
	case key.Matches(msg, m.k().Collapse):
		if e, ok := m.empAt(m.empHold); ok {
			delete(m.empOpen, e.ID)
		}
		return m, nil

	case key.Matches(msg, m.k().Jump):
		// / opens the filter, which is a prompt rather than a field on the screen: it belongs
		// to the moment you are typing it, and a box that is always there costs the list three
		// rows to say nothing.
		m.mode = ModeEmpSearch
		m.empQuery.Focus()
		return m, textinput.Blink

	case key.Matches(msg, m.k().Back):
		// esc from the list does what esc from the prompt does: back to what `e` opens on. It
		// is the same key whether or not the prompt is up, so a filtered list with three rows
		// open takes one keystroke to put back — and not two different ones depending on where
		// the keyboard happens to be.
		return m.clearEmpFilter(), nil

	case key.Matches(msg, m.k().Refresh):
		// The one thing that goes back to the ERP here. The cache stays up while it does, so
		// the screen keeps its cards with the loader beside the count.
		return m.loadEmp()

	case key.Matches(msg, m.k().Quit):
		m.prev, m.mode = m.mode, ModeConfirm
		m.cKind, m.cPrompt = confirmQuit, "Quit tsk?"
		return m, nil
	}
	return m, nil
}

// updateEmpSearch is the filter prompt: every key is a character, and the two ways out say what
// happens to the query. **esc drops it** and gives the whole list back, which is what esc means
// everywhere else here — it undoes the thing you opened. **enter keeps it** and hands the
// keyboard to the rows, so a filtered list can be walked and opened.
func (m Model) updateEmpSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.k().Cancel): // checked first: esc is in both sets
		m = m.clearEmpFilter()
		m.empQuery.Blur()
		m.mode = ModeList
		return m, nil
	case key.Matches(msg, m.k().Focus): // enter: the filter stands
		m.empQuery.Blur()
		m.mode = ModeList
		return m, nil
	}
	var cmd tea.Cmd
	m.empQuery, cmd = m.empQuery.Update(msg)
	// A narrower list means the held row may no longer exist.
	m.empHold = min(m.empHold, max(len(m.empRows())-1, 0))
	return m, cmd
}

// clearEmpFilter takes the screen back to what `e` opens on: the whole list, from the top, with
// every row shut. Clearing the query and leaving five rows open would put the list back and the
// screen still a screenful of detail.
//
// `esc` is the only way to it. There is no ctrl+u here: that key clears the **task** query
// everywhere else in the app, and a second meaning for it on one screen is a key that does two
// things — where esc already means "undo the thing you opened".
func (m Model) clearEmpFilter() Model {
	m.empQuery.SetValue("")
	m.empOpen = map[int]bool{}
	m.empHold = 0
	return m
}

// empAt is the employee on row i of the filtered list.
func (m Model) empAt(i int) (store.Employee, bool) {
	rows := m.empRows()
	if i < 0 || i >= len(rows) {
		return store.Employee{}, false
	}
	return rows[i], true
}

// openEmp shows the cursor's row in full, reading it from the ERP the first time. A row with
// nothing in it yet still opens — the loader goes where the detail will be, so the keypress is
// visibly doing something.
func (m Model) openEmp() (tea.Model, tea.Cmd) {
	e, ok := m.empAt(m.empHold)
	if !ok {
		return m, nil
	}
	m.empOpen = withKey(m.empOpen, e.ID, true)
	if _, have := m.empDetail[e.ID]; have || m.empPulling[e.ID] {
		return m, nil
	}
	if strings.TrimSpace(m.key) == "" {
		m.err = api.ErrNoKey
		return m, nil
	}
	if strings.TrimSpace(m.login) == "" {
		m.status = "sync first — the ERP login comes with today's total"
		return m, nil
	}
	m.empPulling = withKey(m.empPulling, e.ID, true)
	return m, api.FetchEmployee(m.key, m.login, m.db, e.ID)
}

// withKey is a map with one key set, as a new map: the model is a value everywhere else in
// this app, and a map inside it is a reference — writing in place reaches every copy that
// still holds the old one, which is the same trap ticksWith documents.
func withKey(in map[int]bool, id int, want bool) map[int]bool {
	out := make(map[int]bool, len(in)+1)
	for k, v := range in {
		out[k] = v
	}
	out[id] = want
	return out
}

// keepPeople carries the members already read onto a freshly read list, by id and only while
// that project's member ids are the same. A project the refresh dropped goes with them, and one
// whose teams changed is left to be read again.
func keepPeople(old, fresh []store.Project) []store.Project {
	had := make(map[int]store.Project, len(old))
	for _, p := range old {
		had[p.ID] = p
	}
	for i, p := range fresh {
		was, ok := had[p.ID]
		if ok && len(was.People) > 0 && slices.Equal(was.Members, p.Members) {
			fresh[i].People = was.People
		}
	}
	return fresh
}

// withVal and withoutKey are withKey for any value type, and its opposite. Same reason: the
// model is a value and a map inside it is a reference, so a write in place reaches every copy
// that still holds the old map.
func withVal[V any](in map[int]V, id int, v V) map[int]V {
	out := make(map[int]V, len(in)+1)
	for k, old := range in {
		out[k] = old
	}
	out[id] = v
	return out
}

func withoutKey[V any](in map[int]V, id int) map[int]V {
	out := make(map[int]V, len(in))
	for k, v := range in {
		if k != id {
			out[k] = v
		}
	}
	return out
}

// holdEmp pins the window to card i, clamped: the ends of the list stop rather than wrap.
func (m Model) holdEmp(i int) Model {
	m.empHold = min(max(i, 0), max(len(m.empRows())-1, 0))
	return m
}

// empRows is the cards the query leaves, derived on every render like every other filtered
// list here. The match is on the whole card — name, job title, email and phone joined — since
// "who is the security guard" and "who has a strativ.se address" are the same question asked
// of different fields.
func (m Model) empRows() []store.Employee {
	q := strings.ToLower(strings.TrimSpace(m.empQuery.Value()))
	if q == "" {
		return m.emps
	}
	out := make([]store.Employee, 0, len(m.emps))
	for _, e := range m.emps {
		hay := strings.ToLower(strings.Join([]string{e.Name, e.Job, e.Email, e.Phone}, " "))
		if strings.Contains(hay, q) {
			out = append(out, e)
		}
	}
	return out
}

// --- requisitions ------------------------------------------------------------

// showReq opens the requisitions and reads them once a session. Not cached to disk, unlike the
// directory: a stage moves while HR works on it, and a list of stale ones would answer the
// screen's only question wrongly.
func (m Model) showReq() (tea.Model, tea.Cmd) {
	m.tab = TabReq
	if len(m.reqs) > 0 || m.reqLoading {
		return m, nil
	}
	return m.loadReq()
}

func (m Model) loadReq() (Model, tea.Cmd) {
	if strings.TrimSpace(m.key) == "" {
		m.err = api.ErrNoKey
		return m, nil
	}
	m.reqLoading = true
	if strings.TrimSpace(m.login) == "" {
		// The day total's answer carries the email the RPC login needs.
		m.reqWanted = true
		return m, api.FetchDayHours(m.key, parse.Today())
	}
	m.reqWanted = false
	return m, api.FetchRequisitions(m.key, m.login, m.db)
}

// updateReq is the requisitions table: a cursor, a row that opens into its own properties, and
// nothing that writes — filing one is a form this screen does not have.
func (m Model) updateReq(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.k().Down):
		return m.holdReq(m.reqHold + 1), nil
	case key.Matches(msg, m.k().Up):
		return m.holdReq(m.reqHold - 1), nil
	case key.Matches(msg, m.k().Top):
		return m.holdReq(0), nil
	case key.Matches(msg, m.k().Bottom):
		return m.holdReq(len(m.reqs) - 1), nil
	case key.Matches(msg, m.k().HalfDown):
		return m.holdReq(m.reqHold + m.halfPage(2)), nil
	case key.Matches(msg, m.k().HalfUp):
		return m.holdReq(m.reqHold - m.halfPage(2)), nil

	case key.Matches(msg, m.k().Expand):
		// The detail came with the list — properties are a field on the same record — so this
		// opens a row rather than reading one.
		if r, ok := m.reqAt(m.reqHold); ok {
			m.reqOpen = withKey(m.reqOpen, r.ID, true)
		}
		return m, nil
	case key.Matches(msg, m.k().Collapse):
		if r, ok := m.reqAt(m.reqHold); ok {
			delete(m.reqOpen, r.ID)
		}
		return m, nil
	case key.Matches(msg, m.k().NewLeave):
		// The same key the time off request opens with: `n` is "file a new one" on whichever
		// screen files something.
		return m.openReqForm()

	case key.Matches(msg, m.k().Back):
		// esc shuts everything, the same as it does on the directory.
		m.reqOpen, m.reqHold = map[int]bool{}, 0
		return m, nil

	case key.Matches(msg, m.k().Refresh):
		return m.loadReq()

	case key.Matches(msg, m.k().Quit):
		m.prev, m.mode = m.mode, ModeConfirm
		m.cKind, m.cPrompt = confirmQuit, "Quit tsk?"
		return m, nil
	}
	return m, nil
}

func (m Model) holdReq(i int) Model {
	m.reqHold = min(max(i, 0), max(len(m.reqs)-1, 0))
	return m
}

// reqAt is the requisition on row i.
func (m Model) reqAt(i int) (store.Requisition, bool) {
	if i < 0 || i >= len(m.reqs) {
		return store.Requisition{}, false
	}
	return m.reqs[i], true
}

// --- projects ----------------------------------------------------------------

// showProj opens the projects. The cache is what it shows: a project's teams and who runs it do
// not change between two openings of a terminal, so the list is read once and kept on disk, the
// same as the directory. `R` is how it is re-read.
func (m Model) showProj() (tea.Model, tea.Cmd) {
	m.tab = TabProj
	if len(m.projs) > 0 || m.projLoading {
		return m, nil
	}
	return m.loadProj()
}

func (m Model) loadProj() (Model, tea.Cmd) {
	if strings.TrimSpace(m.key) == "" {
		m.err = api.ErrNoKey
		return m, nil
	}
	m.projLoading = true
	if strings.TrimSpace(m.login) == "" {
		// The day total's answer carries the email the RPC login needs, exactly as it does
		// for every other screen that talks RPC.
		m.projWanted = true
		return m, api.FetchDayHours(m.key, parse.Today())
	}
	m.projWanted = false
	return m, api.FetchProjects(m.key, m.login, m.db)
}

// updateProj is the project list: a filter, a cursor over it, and a row that opens into the
// project manager and the people on its teams. Nothing here writes.
func (m Model) updateProj(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.mode == ModeProjSearch {
		return m.updateProjSearch(msg)
	}
	if m.mode == ModeProjJump {
		return m.updateProjJump(msg)
	}
	if m.mode == ModeProjFound {
		return m.updateProjFound(msg)
	}
	switch {
	case key.Matches(msg, m.k().Down):
		return m.holdProj(m.projHold + 1), nil
	case key.Matches(msg, m.k().Up):
		return m.holdProj(m.projHold - 1), nil
	case key.Matches(msg, m.k().Top):
		return m.holdProj(0), nil
	case key.Matches(msg, m.k().Bottom):
		return m.holdProj(len(m.projRows()) - 1), nil
	case key.Matches(msg, m.k().HalfDown):
		return m.holdProj(m.projHold + m.halfPage(2)), nil
	case key.Matches(msg, m.k().HalfUp):
		return m.holdProj(m.projHold - m.halfPage(2)), nil

	case key.Matches(msg, m.k().Expand):
		// l opens the row and reads its people once: a team's membership does not move while
		// a terminal is open, so a second l on the same project asks nothing.
		return m.openProj()
	case key.Matches(msg, m.k().Collapse):
		if p, ok := m.projAt(m.projHold); ok {
			m.projOpen = withoutKey(m.projOpen, p.ID)
		}
		return m, nil

	case key.Matches(msg, m.k().Mine):
		// The one thing on this screen that is not a motion: whose projects are on it. It
		// costs no call — the ERP already said which are mine when it answered the list.
		m.projMine = !m.projMine
		m.projHold, m.projOpen = 0, map[int]bool{}
		return m, nil

	case key.Matches(msg, m.k().Search):
		// i focuses the query box in the header, the same key that reaches the task list's own.
		// Only i: Focus is esc **and** enter, and esc belongs to the list, where it clears.
		m.mode = ModeProjSearch
		m.projQuery.Focus()
		return m, textinput.Blink

	case key.Matches(msg, m.k().Jump):
		// / looks for a person across every project whose people are in hand — read once when
		// its row was opened, and kept, so this reaches more than what is on screen. Nothing
		// read yet is the one case it cannot answer.
		if !m.projAnyPeople() {
			m.status = "no people read yet — " + m.k().Expand.Help().Key + " opens a project"
			return m, nil
		}
		m.mode = ModeProjJump
		m.find.SetValue("")
		m.find.Focus()
		return m, textinput.Blink

	case key.Matches(msg, m.k().Back):
		// The same key whichever half of the screen has the keyboard, exactly as on the
		// directory: it takes the screen back to what `p` opens on.
		return m.clearProjFilter(), nil

	case key.Matches(msg, m.k().Refresh):
		return m.loadProj()

	case key.Matches(msg, m.k().Quit):
		m.prev, m.mode = m.mode, ModeConfirm
		m.cKind, m.cPrompt = confirmQuit, "Quit tsk?"
		return m, nil
	}
	return m, nil
}

// updateProjSearch is the query field: every key filters, live, and esc or enter hands the rows
// the keyboard back — the task list's own two ways out. The query stands either way; it is the
// list's own esc that clears it, which is what that key means on this tab.
func (m Model) updateProjSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.k().Cancel), key.Matches(msg, m.k().Focus):
		m.projQuery.Blur()
		m.mode = ModeList
		return m, nil
	}
	var cmd tea.Cmd
	m.projQuery, cmd = m.projQuery.Update(msg)
	// A narrower list means the held row may no longer exist.
	m.projHold = min(m.projHold, max(len(m.projRows())-1, 0))
	return m, cmd
}

// updateProjJump is `/` inside an open project: it finds a person by name or email and marks
// every row that matches, the way the date jump marks the lines it found. The marks stand after
// the prompt closes, so they survive scrolling; enter on an empty prompt clears them.
func (m Model) updateProjJump(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.k().Cancel):
		m.find.Blur()
		m.mode = ModeList
		return m, nil
	case key.Matches(msg, m.k().Focus):
		m.projFind = strings.TrimSpace(m.find.Value())
		m.find.Blur()
		if m.projFind == "" {
			m.mode, m.status = ModeList, ""
			return m, nil
		}
		if m.projFindHits() == 0 {
			// Nothing to open a modal on: the status line says so and the list stays put.
			m.mode = ModeList
			m.status = "nobody matches " + oneLine(m.projFind)
			return m, nil
		}
		// The modal is the answer, the same as the date jump's own: nothing in the list opens
		// or moves.
		m.mode, m.status, m.projFoundAt = ModeProjFound, "", 0
		return m, nil
	}
	var cmd tea.Cmd
	m.find, cmd = m.find.Update(msg)
	// Live, like every other filter here: the marks follow the query as it is typed.
	m.projFind = strings.TrimSpace(m.find.Value())
	return m, cmd
}

// updateProjFound is the modal: esc closes it and nothing else in it needs a key, since it
// destroys nothing and the list behind it never moved.
func (m Model) updateProjFound(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.k().Cancel), key.Matches(msg, m.k().Focus):
		m.mode = ModeList
	// A search across the office finds more people than a modal holds, so it scrolls by the
	// same half-screen keys every list here moves by.
	case key.Matches(msg, m.k().HalfDown):
		m.projFoundAt = min(m.projFoundAt+m.projFoundStep(), max(m.projFoundLen()-1, 0))
	case key.Matches(msg, m.k().HalfUp):
		m.projFoundAt = max(m.projFoundAt-m.projFoundStep(), 0)
	}
	return m, nil
}

// projFoundLen is how many lines the modal's body comes to — a heading and its names per
// group, with a blank between groups, which is what projFoundModal lays out. Clamped against
// so ctrl+f cannot scroll past the last name.
func (m Model) projFoundLen() int {
	n := 0
	for i, g := range m.projFoundRows() {
		if i > 0 {
			n++
		}
		n += 1 + len(g.people)
	}
	return n
}

// projAnyPeople says whether any project has had its people read, which is the whole of what `/`
// has to search: they arrive when a row is opened and are kept from then on.
func (m Model) projAnyPeople() bool {
	for _, p := range m.projs {
		if len(m.projMembers[p.ID]) > 0 {
			return true
		}
	}
	return false
}

// projFound is the matches, grouped under the project they are on — what the modal renders, and
// derived on every render like every other figure here.
type projFound struct {
	project string
	people  []string
}

func (m Model) projFoundRows() []projFound {
	var out []projFound
	for _, p := range m.projs {
		var hit []string
		for _, who := range m.projMembers[p.ID] {
			if m.onProjFind(who) {
				hit = append(hit, who.Name)
			}
		}
		if len(hit) > 0 {
			out = append(out, projFound{project: p.Name, people: hit})
		}
	}
	return out
}

// onProjFind marks a person: the query matched against the name and the email together, since
// "who is tasnim" and "who has a strativ.se address" are the same question of different fields.
func (m Model) onProjFind(who store.Member) bool {
	q := strings.ToLower(strings.TrimSpace(m.projFind))
	if q == "" {
		return false
	}
	return strings.Contains(strings.ToLower(who.Name+" "+who.Email), q)
}

// projFindHits counts the people the search found, derived rather than kept: a project whose
// row is opened after the search joins on its own.
func (m Model) projFindHits() int {
	n := 0
	for _, g := range m.projFoundRows() {
		n += len(g.people)
	}
	return n
}

// clearProjFilter takes the screen back to what `p` opens on: the whole list, from the top,
// with every row shut. Clearing the query and leaving five rows open would put the list back
// and the screen still a screenful of tables.
func (m Model) clearProjFilter() Model {
	m.projQuery.SetValue("")
	m.projQuery.Blur()
	m.projFind, m.projOpen, m.projHold = "", map[int]bool{}, 0
	if m.mode == ModeProjSearch {
		m.mode = ModeList
	}
	return m
}

// openProj opens the row under the cursor and reads its people the first time. The manager
// came with the list — a many2one arrives named — so this call is only ever about the members.
func (m Model) openProj() (tea.Model, tea.Cmd) {
	p, ok := m.projAt(m.projHold)
	if !ok {
		return m, nil
	}
	m.projOpen = withKey(m.projOpen, p.ID, true)
	if _, have := m.projMembers[p.ID]; have || m.projPulling[p.ID] || len(p.Members) == 0 {
		return m, nil
	}
	if strings.TrimSpace(m.key) == "" {
		m.err = api.ErrNoKey
		return m, nil
	}
	if strings.TrimSpace(m.login) == "" {
		m.status = "sync first — the ERP login comes with today's total"
		return m, nil
	}
	m.projPulling = withVal(m.projPulling, p.ID, true)
	return m, api.FetchProjectMembers(m.key, m.login, m.db, p.ID, p.Members)
}

// projAt is the project on row i of what the filter left.
func (m Model) projAt(i int) (store.Project, bool) {
	rows := m.projRows()
	if i < 0 || i >= len(rows) {
		return store.Project{}, false
	}
	return rows[i], true
}

// projRows is the rows the query leaves, derived on every render like every other filtered
// list here. The match is on the name, the teams and the manager joined: "who runs Coeo" and
// "what is the DevOps team on" are the same question asked of different fields.
func (m Model) projRows() []store.Project {
	q := strings.ToLower(strings.TrimSpace(m.projQuery.Value()))
	if q == "" && !m.projMine {
		return m.projs
	}
	out := make([]store.Project, 0, len(m.projs))
	for _, p := range m.projs {
		if m.projMine && !p.Mine {
			continue
		}
		if q != "" && !strings.Contains(m.projHay(p), q) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// projHay is everything a project can be found by, lowercased: its name, its teams, its manager
// and **the people on it** — "who runs Coeo", "what is the DevOps team on" and "which projects is
// Tasnim on" are the same question asked of different fields. The people only count once they
// have been read, which is what the cache is for.
func (m Model) projHay(p store.Project) string {
	var b strings.Builder
	b.WriteString(p.Name)
	b.WriteByte(' ')
	b.WriteString(strings.Join(p.Teams, " "))
	b.WriteByte(' ')
	b.WriteString(p.Manager)
	for _, who := range m.projMembers[p.ID] {
		b.WriteByte(' ')
		b.WriteString(who.Name)
		b.WriteByte(' ')
		b.WriteString(who.Email)
	}
	return strings.ToLower(b.String())
}

func (m Model) holdProj(i int) Model {
	m.projHold = min(max(i, 0), max(len(m.projRows())-1, 0))
	return m
}

// --- the new-requisition line ------------------------------------------------

// openReqForm reveals the line and reads the categories the first time. Nothing is on it until
// a category is chosen: the fields **are** the category's, so there is nothing to draw before
// one is picked.
func (m Model) openReqForm() (tea.Model, tea.Cmd) {
	m.req = reqForm{open: true, field: reqCatField, cat: -1,
		on: map[string]bool{}, picks: map[string]int{}}
	m.req.urgency = reqInput()
	m.req.noteBox = reqInput()
	m.mode = ModeReqForm

	if m.reqCatsRead || m.reqLoading {
		return m, textinput.Blink
	}
	if strings.TrimSpace(m.key) == "" {
		m.err = api.ErrNoKey
		return m, nil
	}
	if strings.TrimSpace(m.login) == "" {
		m.status = "sync first — the ERP login comes with today's total"
		return m, nil
	}
	m.reqLoading = true
	return m, tea.Batch(textinput.Blink, api.FetchReqCategories(m.key, m.login, m.db))
}

// reqInput is a text field on the form, sized by the view once the rows are laid out. No
// placeholder: the field's own name is in the column beside it, and repeating it inside the
// value is a value that reads as filled in.
func reqInput() textinput.Model {
	in := textinput.New()
	in.Prompt = ""
	in.Width = 12
	return in
}

// closeReqForm takes the line back to its label. Nothing has been filed, so nothing asks.
func (m Model) closeReqForm() (tea.Model, tea.Cmd) {
	m.req = reqForm{cat: -1}
	m.mode, m.err = ModeList, nil
	return m, nil
}

// reqCat is the chosen category, if one has been.
func (m Model) reqCat() (store.ReqCategory, bool) {
	if m.req.cat < 0 || m.req.cat >= len(m.reqCats) {
		return store.ReqCategory{}, false
	}
	return m.reqCats[m.req.cat], true
}

// The line's fixed fields: the category dropdown first, and the two buttons last. Everything
// between them is the category's own, so their positions are counted rather than named.
const reqCatField = 0

// reqFieldCount is the whole line: the category, the fields it asks for, urgent, the cause when
// it is ticked, the note, and the two buttons.
func (m Model) reqFieldCount() int {
	cat, ok := m.reqCat()
	if !ok {
		return 1 // the dropdown alone: there is nothing else to fill in yet
	}
	n := 1 + len(cat.Fields) + 1 + 1 + 2 // category, its fields, urgent, note, ✓ and ✕
	if m.req.urgent {
		n++ // the cause, which only exists while it is
	}
	return n
}

func (m Model) reqOKField() int { return m.reqFieldCount() - 2 }
func (m Model) reqXField() int  { return m.reqFieldCount() - 1 }

// reqPropField is which of the category's own fields a position is, or -1 for the rest of them.
func (m Model) reqPropField(field int) int {
	cat, ok := m.reqCat()
	if !ok || field < 1 || field > len(cat.Fields) {
		return -1
	}
	return field - 1
}

// reqUrgentField, reqUrgencyField and reqNoteField are where the ERP's own three sit, after the
// category's. The cause is between them and only while urgent is ticked.
func (m Model) reqUrgentField() int {
	cat, _ := m.reqCat()
	return 1 + len(cat.Fields)
}

func (m Model) reqUrgencyField() int {
	if !m.req.urgent {
		return -1
	}
	return m.reqUrgentField() + 1
}

func (m Model) reqNoteField() int {
	n := m.reqUrgentField() + 1
	if m.req.urgent {
		n++
	}
	return n
}

// updateReqForm is the new-requisition line: tab through the fields, j/k on a dropdown, space
// on a checkbox, enter on ✓ to file it and on ✕ to close.
func (m Model) updateReqForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.k().Next):
		return m.moveReqField(1), nil
	case key.Matches(msg, m.k().Prev):
		return m.moveReqField(-1), nil

	case key.Matches(msg, m.k().Accept):
		switch m.req.field {
		case m.reqOKField():
			return m.askFileReq()
		case m.reqXField():
			return m.closeReqForm()
		default:
			return m.moveReqField(1), nil
		}

	case key.Matches(msg, m.k().Cancel):
		// esc asks once there is anything to lose, the same as it does on the time off line:
		// it is the key pressed by accident, and a category's worth of typed fields goes with
		// it. Nothing is on the line until a category is chosen — the fields **are** the
		// category's — so that is the whole of "has this been filled in", and a line still as
		// `n` opened it closes outright rather than asking about nothing.
		if m.req.cat < 0 {
			return m.closeReqForm()
		}
		m.prev, m.mode = ModeReqForm, ModeConfirm
		m.cKind, m.cPrompt = confirmDropReq, "Discard this requisition?"
		return m, nil

	case key.Matches(msg, m.k().ClearField):
		if in := m.reqInput(); in != nil {
			in.SetValue("")
		}
		return m, nil
	}

	// The dropdowns and the checkboxes: j/k and space, which are letters everywhere else — so
	// they only mean this **on a chooser**. Matched before the input, they swallowed the space
	// bar in every text field on the line, and a purpose is a sentence.
	if key.Matches(msg, m.k().Cycle) && m.reqFieldIsChooser() {
		back := msg.String() == "k" || msg.String() == "up"
		switch {
		case m.req.field == reqCatField && len(m.reqCats) > 0:
			return m.pickReqCat(back)
		case m.req.field == m.reqUrgentField():
			m.req.urgent = !m.req.urgent
			return m.clampReqField(), nil
		}
		if i := m.reqPropField(m.req.field); i >= 0 {
			return m.stepReqProp(i, back), nil
		}
		return m, nil
	}

	in := m.reqInput()
	if in == nil {
		return m, nil
	}
	var cmd tea.Cmd
	*in, cmd = in.Update(msg)
	return m, cmd
}

// pickReqCat steps the category dropdown and rebuilds the line: the fields **are** the
// category's, so choosing another one is a different form. Whatever was typed goes with it,
// which is the honest thing — the values belonged to fields that no longer exist.
func (m Model) pickReqCat(back bool) (tea.Model, tea.Cmd) {
	n := len(m.reqCats)
	switch {
	case m.req.cat < 0 && back:
		m.req.cat = n - 1
	case m.req.cat < 0:
		m.req.cat = 0
	case back:
		m.req.cat = (m.req.cat - 1 + n) % n
	default:
		m.req.cat = (m.req.cat + 1) % n
	}

	cat, _ := m.reqCat()
	m.req.inputs = make([]textinput.Model, len(cat.Fields))
	m.req.on, m.req.picks = map[string]bool{}, map[string]int{}
	var cmds []tea.Cmd
	for i, f := range cat.Fields {
		switch f.Kind {
		case "boolean":
		case "many2one":
			// The options are read when the category is chosen rather than with the categories
			// themselves: a form nobody opened should not cost a call per field.
			if f.Comodel != "" && len(f.Opts) == 0 && m.key != "" && m.login != "" {
				cmds = append(cmds, api.FetchReqOptions(m.key, m.login, m.db, f.Name, f.Comodel))
			}
		case "date":
			// The one field with a shape: the placeholder is the shape, as it is on the insert
			// row, and what is typed into it is normalized on the way out.
			in := reqInput()
			in.Placeholder = "dd/mm/yy"
			in.CharLimit = 8
			m.req.inputs[i] = in
		default:
			m.req.inputs[i] = reqInput()
		}
	}
	return m.clampReqField(), tea.Batch(cmds...)
}

// stepReqProp steps whichever kind of chooser this field is: a boolean's tick, or a many2one's
// own options. A text field is typed into instead and this does nothing to it.
func (m Model) stepReqProp(i int, back bool) Model {
	cat, ok := m.reqCat()
	if !ok || i >= len(cat.Fields) {
		return m
	}
	f := cat.Fields[i]
	switch f.Kind {
	case "boolean":
		// Copied on write, the same rule ticksWith follows for the meal line's ids: the model
		// is a value and a map inside it is a reference. Keyed by the property's own name,
		// since a re-chosen category renumbers the fields.
		next := make(map[string]bool, len(m.req.on)+1)
		for k, v := range m.req.on {
			next[k] = v
		}
		next[f.Name] = !next[f.Name]
		m.req.on = next
	case "many2one":
		if len(f.Opts) == 0 {
			return m
		}
		next := map[string]int{}
		for k, v := range m.req.picks {
			next[k] = v
		}
		at := next[f.Name]
		if back {
			at = (at - 1 + len(f.Opts)) % len(f.Opts)
		} else {
			at = (at + 1) % len(f.Opts)
		}
		next[f.Name] = at
		m.req.picks = next
	}
	return m
}

// moveReqField normalizes the date being left — dates are rewritten on exit, never per
// keystroke, the same as the insert row's — then focuses the next field, wrapping.
func (m Model) moveReqField(by int) Model {
	m.normalizeReqDate()
	n := m.reqFieldCount()
	m.req.field = (m.req.field + by + n) % n
	for i := range m.req.inputs {
		m.req.inputs[i].Blur()
	}
	m.req.urgency.Blur()
	m.req.noteBox.Blur()
	if in := m.reqInput(); in != nil {
		in.Focus()
	}
	return m
}

// normalizeReqDate rewrites the date field being left: `30` is the 30th of this month, `30/9`
// the 30th of September, exactly as the insert row and every other date on every other screen
// read what is typed into them.
func (m *Model) normalizeReqDate() {
	cat, ok := m.reqCat()
	if !ok {
		return
	}
	i := m.reqPropField(m.req.field)
	if i < 0 || i >= len(m.req.inputs) || cat.Fields[i].Kind != "date" {
		return
	}
	raw := strings.TrimSpace(m.req.inputs[i].Value())
	if raw == "" {
		return
	}
	if d, err := parse.Date(raw, parse.Today()); err == nil {
		m.req.inputs[i].SetValue(d)
		m.err = nil
	} else {
		m.err = err
	}
}

// clampReqField keeps the cursor on a field that still exists: choosing a category with fewer
// fields, or unticking urgent, takes one away.
func (m Model) clampReqField() Model {
	if n := m.reqFieldCount(); m.req.field >= n {
		m.req.field = n - 1
	}
	return m
}

// reqInput is the text field the cursor is in, or nil on a dropdown, a checkbox or a button.
func (m *Model) reqInput() *textinput.Model {
	if i := m.reqPropField(m.req.field); i >= 0 && i < len(m.req.inputs) {
		cat, _ := m.reqCat()
		switch cat.Fields[i].Kind {
		case "boolean", "many2one":
			return nil
		}
		return &m.req.inputs[i]
	}
	switch m.req.field {
	case m.reqUrgencyField():
		return &m.req.urgency
	case m.reqNoteField():
		return &m.req.noteBox
	}
	return nil
}

// reqValues is what the line would file, and the first field it cannot: every field the
// category calls required has to have something in it, which the ERP would otherwise refuse one
// round trip later.
func (m Model) reqValues() ([]store.PropValue, string) {
	cat, ok := m.reqCat()
	if !ok {
		return nil, "pick a category first"
	}
	var out []store.PropValue
	for i, f := range cat.Fields {
		p := store.PropValue{Name: f.Name, Kind: f.Kind, Label: f.Label}
		switch f.Kind {
		case "boolean":
			p.Value = m.req.on[f.Name]
		case "many2one":
			if len(f.Opts) == 0 {
				if f.Required {
					return nil, "no " + strings.ToLower(f.Label) + " to choose from"
				}
				continue
			}
			p.Value = f.Opts[min(m.req.picks[f.Name], len(f.Opts)-1)].ID
		case "date":
			raw := strings.TrimSpace(m.req.inputs[i].Value())
			if raw == "" {
				if f.Required {
					return nil, strings.ToLower(f.Label) + " is required"
				}
				continue
			}
			d, err := parse.Date(raw, parse.Today())
			if err != nil {
				return nil, "unreadable " + strings.ToLower(f.Label)
			}
			t, err := time.Parse(parse.DateLayout, d)
			if err != nil {
				return nil, "unreadable " + strings.ToLower(f.Label)
			}
			p.Value = t.Format("2006-01-02")
		case "integer", "float":
			raw := strings.TrimSpace(m.req.inputs[i].Value())
			if raw == "" {
				if f.Required {
					return nil, strings.ToLower(f.Label) + " is required"
				}
				continue
			}
			n, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				return nil, strings.ToLower(f.Label) + " has to be a number"
			}
			p.Value = n
		default:
			raw := strings.TrimSpace(oneLine(m.req.inputs[i].Value()))
			if raw == "" {
				if f.Required {
					return nil, strings.ToLower(f.Label) + " is required"
				}
				continue
			}
			p.Value = raw
		}
		out = append(out, p)
	}
	return out, ""
}

// askFileReq states what is about to be filed and waits for a y or an n: it asks the office for
// something, and the category and its fields are worth reading back before that goes.
func (m Model) askFileReq() (tea.Model, tea.Cmd) {
	if m.filing {
		m.status = "still waiting on the ERP…"
		return m, nil
	}
	props, refuse := m.reqValues()
	if refuse != "" {
		m.status = refuse
		return m, nil
	}
	cat, _ := m.reqCat()

	lines := make([]string, 0, len(props)+1)
	for _, p := range props {
		lines = append(lines, fmt.Sprintf("%s: %v", strings.ToLower(p.Label), p.Value))
	}
	if m.req.urgent {
		lines = append(lines, "urgent")
	}
	m.prev, m.mode = m.mode, ModeConfirm
	m.cKind = confirmFileReq
	m.cPrompt = fmt.Sprintf("File a %s?\n\n%s", cat.Name, strings.Join(lines, "\n"))
	return m, nil
}

// fileReq sends the create, once the modal has been answered.
func (m Model) fileReq() (Model, tea.Cmd) {
	props, refuse := m.reqValues()
	if refuse != "" {
		m.status = refuse
		return m, nil
	}
	cat, _ := m.reqCat()
	m.filing, m.err = true, nil
	m.status = "filing the " + strings.ToLower(cat.Name) + "…"
	return m, api.FileRequisition(m.key, m.login, m.db, m.reqEmployee(), cat.ID,
		props, m.req.urgent, m.req.urgency.Value(), m.req.noteBox.Value())
}

// reqFieldIsChooser says whether the focused field is stepped rather than typed into: the
// category dropdown, a boolean, or a many2one's own options. The footer reads it, so it only
// offers j/k where they do something.
func (m Model) reqFieldIsChooser() bool {
	if !m.req.open {
		return false
	}
	if m.req.field == reqCatField || m.req.field == m.reqUrgentField() {
		return true
	}
	if i := m.reqPropField(m.req.field); i >= 0 {
		cat, _ := m.reqCat()
		switch cat.Fields[i].Kind {
		case "boolean", "many2one":
			return true
		}
	}
	return false
}

// reqEmployee is the hr.employee this key files as, when the app already knows it: the clock
// reads it, and the field defaults to the same record anyway.
func (m Model) reqEmployee() int { return m.attEmp }

// needsWFH says whether a refusal is the one a work-from-home request answers. The ERP's own
// sentence is "You have exceeded the number of days available for WFH. Please submit a WFH
// request.", and matching its name for the thing is the only reading that cannot mistake some
// other refusal for this one.
func needsWFH(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "wfh")
}

// openWFH reveals the request line, focused on the first day, with today in both date fields
// — the day whose check in was just refused is the day the request is for.
func (m Model) openWFH() (tea.Model, tea.Cmd) {
	f := wfhForm{open: true, field: wfhFromField}
	for _, in := range []*textinput.Model{&f.from, &f.to} {
		*in = textinput.New()
		in.Prompt = ""
		in.Placeholder = "dd/mm/yy"
		in.Width = fieldWidth(dateWidth)
		in.CharLimit = 8
	}
	f.from.SetValue(parse.Today())
	f.to.SetValue(parse.Today())
	f.fresh = [2]bool{true, true}

	f.reason = textinput.New()
	f.reason.Prompt = ""
	f.reason.Placeholder = "reason"

	m.wfh, m.mode = f, ModeWFH
	// Sized after the form is in the model, never before: the width is measured on the row as
	// it will be drawn, and an empty wfhForm draws no date fields to measure against.
	m.wfh.reason.Width = m.wfhReasonWidth()
	m.wfh.from.Focus()
	return m, textinput.Blink
}

// closeWFH takes the line away. Nothing has been filed, so nothing asks — the days and the
// reason are two keystrokes and a sentence, and the refusal that opened it is still in the
// status line to open it again.
func (m Model) closeWFH() (tea.Model, tea.Cmd) {
	m.wfh = wfhForm{}
	m.mode, m.err = ModeList, nil
	return m, nil
}

// updateWFH is the request line: tab through the fields, enter on ✓ to file it and check in,
// enter on ✕ or esc to drop it.
func (m Model) updateWFH(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.k().Next):
		return m.moveWFHField(1), nil
	case key.Matches(msg, m.k().Prev):
		return m.moveWFHField(-1), nil

	case key.Matches(msg, m.k().Accept):
		switch m.wfh.field {
		case wfhOKField:
			return m.fileWFH()
		case wfhXField:
			return m.closeWFH()
		default:
			return m.moveWFHField(1), nil
		}

	case key.Matches(msg, m.k().Cancel):
		return m.closeWFH()

	case key.Matches(msg, m.k().ClearField):
		if in := m.wfhInput(); in != nil {
			in.SetValue("")
			if i := m.wfhDateIndex(); i >= 0 {
				m.wfh.fresh[i] = false
			}
		}
		return m, nil
	}

	in := m.wfhInput()
	if in == nil {
		return m, nil
	}
	// A date field opens with its value selected: the first thing typed replaces it whole.
	if i := m.wfhDateIndex(); i >= 0 && m.wfh.fresh[i] && msg.Type == tea.KeyRunes {
		in.SetValue("")
		m.wfh.fresh[i] = false
	}
	var cmd tea.Cmd
	*in, cmd = in.Update(msg)
	return m, cmd
}

// moveWFHField normalizes the date being left — dates are rewritten on exit, never per
// keystroke — and focuses the next field, wrapping.
func (m Model) moveWFHField(by int) Model {
	m.normalizeWFHDates()
	m.wfh.field = (m.wfh.field + by + wfhFieldCount) % wfhFieldCount
	for _, in := range []*textinput.Model{&m.wfh.from, &m.wfh.to, &m.wfh.reason} {
		in.Blur()
	}
	if in := m.wfhInput(); in != nil {
		in.Focus()
	}
	if i := m.wfhDateIndex(); i >= 0 {
		m.wfh.fresh[i] = true // freshly focused: the value is selected
	}
	return m
}

// wfhInput is the text field the cursor is in, or nil on a button.
func (m *Model) wfhInput() *textinput.Model {
	switch m.wfh.field {
	case wfhFromField:
		return &m.wfh.from
	case wfhToField:
		return &m.wfh.to
	case wfhReasonField:
		return &m.wfh.reason
	}
	return nil
}

// wfhDateIndex is which of the two date fields has the cursor, or -1.
func (m Model) wfhDateIndex() int {
	switch m.wfh.field {
	case wfhFromField:
		return 0
	case wfhToField:
		return 1
	}
	return -1
}

// normalizeWFHDates rewrites a date field as it is left, and drags the end along when the
// start passes it — a range that reads backwards would cover the days between.
func (m *Model) normalizeWFHDates() {
	switch m.wfh.field {
	case wfhFromField:
		if d, err := parse.Date(m.wfh.from.Value(), parse.Today()); err == nil {
			m.wfh.from.SetValue(d)
			if before(d, m.wfh.to.Value()) {
				m.wfh.to.SetValue(d)
			}
			m.err = nil
		} else {
			m.err = err
		}
	case wfhToField:
		if d, err := parse.Date(m.wfh.to.Value(), m.wfh.from.Value()); err == nil {
			m.wfh.to.SetValue(d)
			m.err = nil
		} else {
			m.err = err
		}
	}
}

// fileWFH sends the request. No modal: it asks a manager for days, which is not destructive,
// and the line itself already states everything the prompt would repeat.
func (m Model) fileWFH() (Model, tea.Cmd) {
	m.normalizeWFHDates()
	if strings.TrimSpace(m.wfh.reason.Value()) == "" {
		// The ERP requires it, so this is a refusal it would make one round trip later.
		m.status = "say why you are working from home"
		return m, nil
	}
	if m.wfhFiling {
		m.status = "still waiting on the ERP…"
		return m, nil
	}

	m.wfhFiling, m.err = true, nil
	m.status = "requesting work from home…"
	return m, api.RequestWFH(m.key, m.login, m.db, m.attEmp,
		m.wfh.from.Value(), m.wfh.to.Value(), m.wfh.reason.Value())
}

// confirmHours tells the ERP the month's own hour logs are done, once the modal has been
// answered. The chart behind it is what the claim is about, so the answer re-reads the month
// rather than assuming anything moved.
func (m Model) confirmHours() (tea.Model, tea.Cmd) {
	if strings.TrimSpace(m.key) == "" {
		m.err = api.ErrNoKey
		return m, nil
	}
	m.confirming, m.err = true, nil
	m.status = "confirming " + m.targetMonth().Format("January 2006") + "'s hour logs…"
	return m, api.ConfirmHours(m.key, m.login, m.db, m.targetMonth())
}

// updateDash is the chart tab: the month's ends, half a screen either way, a refresh, and
// the keys that leave for the task list. There is no cursor to walk a day at a time — the
// chart is one picture, not a list — so it moves in screenfuls: g and G to the ends,
// ctrl+f / ctrl+b by half of what is showing, and today in between, where it opens.
func (m Model) updateDash(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.k().Top):
		return m.holdDash(0), nil
	case key.Matches(msg, m.k().Bottom):
		return m.holdDash(len(m.dashDays) - 1), nil
	case key.Matches(msg, m.k().HalfDown):
		return m.holdDash(m.dashDayIndex() + m.halfPage(dashRowLines)), nil
	case key.Matches(msg, m.k().HalfUp):
		return m.holdDash(m.dashDayIndex() - m.halfPage(dashRowLines)), nil

	case key.Matches(msg, m.k().PrevMonth):
		// dashMonth is left alone: the month on screen stays coherent — its own header
		// and rows, both from the same answer — with a loader beside the title, until
		// the new one lands and replaces both at once.
		m.dashOffset--
		return m.loadDash()

	case key.Matches(msg, m.k().NextMonth):
		if m.dashOffset >= 0 {
			return m, nil // nothing to report on a month that has not happened
		}
		m.dashOffset++
		return m.loadDash()

	case key.Matches(msg, m.k().Clock):
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

	case key.Matches(msg, m.k().ConfirmHours):
		// It asks first and takes y or n: it tells the ERP a month is done, which is a claim
		// about every day on the chart behind the modal, so the month is named in the prompt.
		if m.confirming {
			m.status = "still waiting on the ERP…"
			return m, nil
		}
		m.prev, m.mode = m.mode, ModeConfirm
		m.cKind = confirmHourLogs
		m.cPrompt = "Have you logged all hours of " +
			m.targetMonth().Format("January 2006") + " ?"
		return m, nil

	case key.Matches(msg, m.k().Refresh):
		m.dashMonth = "" // force a re-read of the month
		return m.loadDash()

	case key.Matches(msg, m.k().Quit):
		m.prev, m.mode = m.mode, ModeConfirm
		m.cKind, m.cPrompt = confirmQuit, "Quit tsk?"
		return m, nil

	case key.Matches(msg, m.k().Search), key.Matches(msg, m.k().ClearSearch):
		// Searching is a task-list act, so it takes you back there.
		m.tab, m.mode = TabTasks, ModeSearch
		if key.Matches(msg, m.k().ClearSearch) {
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

// --- time off ----------------------------------------------------------------

// showTime opens the year calendar and reads the year if it is not already on screen.
// One year at a time, like the chart's one month: r re-reads it.
func (m Model) showTime() (tea.Model, tea.Cmd) {
	m.tab = TabTime
	if m.timeYear == time.Now().Year() || m.timeLoading {
		return m, nil // already have it, or it is on its way
	}
	return m.loadTime()
}

// loadTime asks for the whole year in one call. The year on screen is left alone while it
// is in flight, so a re-read keeps a coherent calendar — its own days, its own balances —
// with a loader beside the title, exactly as the chart does with its month.
func (m Model) loadTime() (Model, tea.Cmd) {
	if strings.TrimSpace(m.key) == "" {
		m.err = api.ErrNoKey
		return m, nil
	}
	m.timeLoading = true
	if strings.TrimSpace(m.login) == "" {
		// The day total's answer carries the email the RPC login needs.
		m.timeWanted = true
		return m, api.FetchDayHours(m.key, parse.Today())
	}
	m.timeWanted = false
	return m, api.FetchTimeOff(m.key, m.login, m.db, time.Now().Year())
}

// updateTime is the calendar tab. There is no cursor — a year is a picture, not a list —
// so it moves in months: g and G to the ends, ctrl+f / ctrl+b by a row of them.
//
// The filters come last, after every bound key, because they are letters the ERP chose:
// they are the leave types' own initials, so a type named after a key that already means
// something must not shadow it.
func (m Model) updateTime(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	// All four walk the year a month at a time — Jan, Feb, Mar — which is the sequence a year is
	// read in and the unit the caret and the holiday panel both follow. h/l used to step a
	// month while j/k stepped a row of them, and a row is 2, 3 or 4 months depending on the
	// width, so the same key covered a different distance on every terminal; they are aliases
	// now, and whichever hand you reach with lands on the next month. They are the bindings the
	// task list moves by, so a rebind moves both screens.
	case key.Matches(msg, m.k().Down), key.Matches(msg, m.k().Expand):
		return m.holdTime(m.timeMonth() + 1), nil
	case key.Matches(msg, m.k().Up), key.Matches(msg, m.k().Collapse):
		return m.holdTime(m.timeMonth() - 1), nil

	case key.Matches(msg, m.k().Top):
		return m.holdTime(0), nil
	case key.Matches(msg, m.k().Bottom):
		return m.holdTime(11), nil
	// A row of months is what a screenful means here, which is the one place a width-dependent
	// jump still says something.
	case key.Matches(msg, m.k().HalfDown):
		return m.holdTime(m.timeMonth() + m.timeCols()), nil
	case key.Matches(msg, m.k().HalfUp):
		return m.holdTime(m.timeMonth() - m.timeCols()), nil

	case key.Matches(msg, m.k().Accept):
		// The month in view is the one it lists: the caret says which that is, and there is
		// no cursor here to mean anything else.
		m.prev, m.mode = m.mode, ModeLeaves
		return m, nil

	case key.Matches(msg, m.k().NewLeave):
		return m.openLeaveForm()

	case key.Matches(msg, m.k().Refresh):
		// timeYear is left alone: loadTime is called outright here, so nothing needs the
		// cache key cleared, and clearing it would blank the calendar and its totals for
		// as long as the read takes. The loader beside the title says it is re-reading.
		return m.loadTime()

	case key.Matches(msg, m.k().Quit):
		m.prev, m.mode = m.mode, ModeConfirm
		m.cKind, m.cPrompt = confirmQuit, "Quit tsk?"
		return m, nil

	case key.Matches(msg, m.k().Search), key.Matches(msg, m.k().ClearSearch):
		// Searching is a task-list act, so it takes you back there.
		m.tab, m.mode = TabTasks, ModeSearch
		if key.Matches(msg, m.k().ClearSearch) {
			m.search.SetValue("")
			m.clampCursor()
		}
		m.search.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

// updateLeaves is the month's time off modal: it destroys nothing, so esc and enter both
// close it and nothing else in it needs a key.
func (m Model) updateLeaves(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.k().Back), key.Matches(msg, m.k().Accept):
		m.mode = m.prev
		if m.mode == ModeLeaves {
			m.mode = ModeList
		}
	}
	return m, nil
}

// leaveRow is one day off in the modal: the date as a person says it, and what it is for.
type leaveRow struct {
	date   time.Time
	kind   string // the leave type's name, which is what its colour comes from
	desc   string
	half   bool
	period string // "am" | "pm", only when half
	state  string
}

// monthLeaves is every day off in the month in view, one line each, in date order.
//
// A day rather than a request: a range reads as the days it covers on the calendar above, so
// a list that collapsed 19-21 Aug into one row would answer a different question from the one
// the month is asking. Derived on render like everything else here, and it follows the type
// filter, so the list and the calendar under it always say the same thing.
func (m Model) monthLeaves() []leaveRow {
	month, year := time.Month(m.timeMonth()+1), m.timeYearOf()
	var out []leaveRow
	for _, l := range m.timeLeaves {
		from, err := time.Parse("2006-01-02", l.From)
		if err != nil {
			continue
		}
		to, err := time.Parse("2006-01-02", l.To)
		if err != nil || to.Before(from) {
			to = from
		}
		for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
			if d.Month() != month || d.Year() != year {
				continue
			}
			out = append(out, leaveRow{date: d, kind: l.Kind, desc: l.Desc,
				half: l.Half, period: l.Period, state: l.State})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].date.Before(out[j].date) })
	return out
}

// --- the new-timeoff line ----------------------------------------------------

// openLeaveForm reveals the line's fields and focuses the leave type, which is the first
// thing a request has to say. The label was on screen all along and the row it sits on does
// not move, so nothing below it shifts when this happens.
func (m Model) openLeaveForm() (tea.Model, tea.Cmd) {
	if len(m.timeKinds) == 0 {
		// Without the types there is no request to make, and inventing one locally would be
		// a form that cannot be filed.
		m.status = "no leave types yet — r to read this year"
		return m, nil
	}
	f := leaveForm{open: true, field: leaveKindField}
	for _, in := range []struct {
		p  *textinput.Model
		ph string
		w  int
	}{{&f.from, "dd/mm/yy", fieldWidth(dateWidth)}, {&f.to, "dd/mm/yy", fieldWidth(dateWidth)},
		{&f.desc, "description…", 20}} {
		*in.p = textinput.New()
		in.p.Prompt = ""
		in.p.Placeholder = in.ph
		in.p.Width = in.w
		in.p.CharLimit = 120
	}
	// Today in both date fields: the day you are most likely asking about, and it makes the
	// calendar's highlight say something the moment the line opens.
	f.from.SetValue(parse.Today())
	f.to.SetValue(parse.Today())
	f.fresh = [2]bool{true, true}
	m.form = f // the width is measured against the line as it will be rendered
	f.desc.Width = m.leaveDescWidth()

	m.form, m.mode = f, ModeForm
	m.err = nil
	return m, textinput.Blink
}

// updateForm is the new-timeoff line: tab through the fields, j/k inside a dropdown, enter
// on ✓ to confirm and on ✕ to start over. It owns every key while it is open — the
// description has to be able to hold a t, a d and an o.
func (m Model) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.k().Next):
		return m.moveLeaveField(1), nil
	case key.Matches(msg, m.k().Prev):
		return m.moveLeaveField(-1), nil

	case key.Matches(msg, m.k().Accept):
		switch m.form.field {
		case leaveOKField:
			return m.askApplyLeave()
		case leaveXField:
			// ✕ closes the line and does not ask: nothing has been filed, so there is
			// nothing to lose, and the row goes back to the `new timeoff` label the tab
			// opened on. esc is the one that asks, since it is pressed by accident.
			m.form = leaveForm{}
			m.mode, m.err = ModeList, nil
			return m, nil
		default:
			return m.moveLeaveField(1), nil
		}

	case key.Matches(msg, m.k().Cancel):
		// esc asks, because everything typed goes with it.
		m.prev, m.mode = ModeForm, ModeConfirm
		m.cKind, m.cPrompt = confirmDropLeave, "Discard this time off request?"
		return m, nil

	case key.Matches(msg, m.k().ClearField):
		switch m.form.field {
		case leaveFromField:
			m.form.from.SetValue("")
			m.form.fresh[0] = false
		case leaveToField:
			if !m.form.half {
				m.form.to.SetValue("")
				m.form.fresh[1] = false
			}
		case leaveDescField:
			m.form.desc.SetValue("")
		}
		return m, nil
	}

	// The dropdowns take j/k and space; the text fields must keep those letters.
	if m.leaveFieldIsDropdown() && key.Matches(msg, m.k().Cycle) {
		return m.cycleLeaveField(msg.String()), nil
	}
	// On the leave type, a type's own initial picks it — the same letters the filter chips
	// use, so s/c/a/p mean one thing on this screen. Matched after Cycle, so j/k still step.
	if m.form.field == leaveKindField && msg.Type == tea.KeyRunes {
		if i, ok := m.kindByLetter(msg.String()); ok {
			m.form.kind = i
			return m.followLeaveDates(), nil
		}
	}

	in := m.leaveInput()
	if in == nil {
		return m, nil
	}
	// A date field opens with its value selected: the first thing typed replaces it whole,
	// so 21 on a focused field is the 21st and not 21 appended to today.
	if i := m.leaveDateIndex(); i >= 0 && m.form.fresh[i] && msg.Type == tea.KeyRunes {
		in.SetValue("")
		m.form.fresh[i] = false
	}
	if m.form.field == leaveDescField {
		m.form.fresh = [2]bool{false, false}
	}

	var cmd tea.Cmd
	*in, cmd = in.Update(msg)
	// The calendar follows what is being typed, so the month holding the date comes into
	// view as the digits land.
	return m.followLeaveDates(), cmd
}

// moveLeaveField normalizes the field being left — dates are rewritten on exit, never per
// keystroke — and focuses the next one, wrapping.
func (m Model) moveLeaveField(by int) Model {
	m.normalizeLeaveDates(m.form.field)
	m.form.field = (m.form.field + by + leaveFieldCount) % leaveFieldCount
	// A half day has no range end, so that slot is the period dropdown and the fields
	// either side of it still tab in one line.
	for _, in := range []*textinput.Model{&m.form.from, &m.form.to, &m.form.desc} {
		in.Blur()
	}
	if in := m.leaveInput(); in != nil {
		in.Focus()
	}
	if i := m.leaveDateIndex(); i >= 0 {
		m.form.fresh[i] = true // freshly focused: the value is selected
	}
	return m.followLeaveDates()
}

// leaveInput is the text field the cursor is in, or nil on a dropdown or a button.
func (m *Model) leaveInput() *textinput.Model {
	switch {
	case m.form.field == leaveFromField:
		return &m.form.from
	case m.form.field == leaveToField && !m.form.half:
		return &m.form.to
	case m.form.field == leaveDescField:
		return &m.form.desc
	}
	return nil
}

// leaveDateIndex is which of the two date fields has the cursor, or -1.
func (m Model) leaveDateIndex() int {
	switch {
	case m.form.field == leaveFromField:
		return 0
	case m.form.field == leaveToField && !m.form.half:
		return 1
	}
	return -1
}

func (m Model) leaveFieldIsDropdown() bool {
	return m.form.field == leaveKindField || m.form.field == leaveDurField ||
		(m.form.field == leaveToField && m.form.half)
}

// cycleLeaveField changes the focused dropdown. k and up go back, everything else forward,
// so one binding drives both directions.
func (m Model) cycleLeaveField(k string) Model {
	back := k == "k" || k == "up"
	switch m.form.field {
	case leaveKindField:
		n := len(m.timeKinds)
		if n == 0 {
			return m
		}
		if back {
			m.form.kind = (m.form.kind - 1 + n) % n
		} else {
			m.form.kind = (m.form.kind + 1) % n
		}
	case leaveDurField:
		m.form.half = !m.form.half
	case leaveToField:
		m.form.pm = !m.form.pm
	}
	return m.followLeaveDates()
}

// normalizeLeaveDates rewrites a date field as it is left: 21 becomes the 21st of this
// month, and the range's end is read against its start so a range across a month works.
func (m *Model) normalizeLeaveDates(field int) {
	switch field {
	case leaveFromField:
		if d, err := parse.Date(m.form.from.Value(), parse.Today()); err == nil {
			m.form.from.SetValue(d)
			// The end comes along when the start passes it, the way a date range behaves
			// everywhere. Left behind, the range quietly reads backwards and covers the days
			// between — asking for the 20th with the 19th still in the end field booked both,
			// and the ERP refused the pair over a leave already on the 19th.
			if before(d, m.form.to.Value()) {
				m.form.to.SetValue(d)
			}
			m.err = nil
		} else {
			m.err = err
		}
	case leaveToField:
		if m.form.half {
			return
		}
		if d, err := parse.Date(m.form.to.Value(), m.form.from.Value()); err == nil {
			m.form.to.SetValue(d)
			m.err = nil
		} else {
			m.err = err
		}
	}
}

// before reports whether the second date is earlier than the first, both dd/mm/yy. An
// unreadable date is not earlier than anything — there is nothing to drag it to.
func before(a, b string) bool {
	x, errA := time.Parse(parse.DateLayout, strings.TrimSpace(a))
	y, errB := time.Parse(parse.DateLayout, strings.TrimSpace(b))
	return errA == nil && errB == nil && y.Before(x)
}

// followLeaveDates brings the month the request starts in into view, so the days the
// highlight is on are the days on screen. Partial input counts — 21 is already a date.
func (m Model) followLeaveDates() Model {
	from, _, ok := m.leaveRange()
	if !ok {
		return m
	}
	if t, err := time.Parse(parse.DateLayout, from); err == nil && t.Year() == m.timeYearOf() {
		m.timeHold = int(t.Month()) - 1
	}
	return m
}

// leaveRange is the days the open line covers, dd/mm/yy and in order, resolved the way the
// fields themselves resolve so that half-typed input highlights too. A half day is one day.
func (m Model) leaveRange() (from, to string, ok bool) {
	if !m.form.open {
		return "", "", false
	}
	from, err := parse.Date(m.form.from.Value(), parse.Today())
	if err != nil {
		return "", "", false
	}
	if m.form.half {
		return from, from, true
	}
	to, err = parse.Date(m.form.to.Value(), from)
	if err != nil {
		return from, from, true // the end is not readable yet; the start still is
	}
	a, errA := time.Parse(parse.DateLayout, from)
	b, errB := time.Parse(parse.DateLayout, to)
	if errA == nil && errB == nil && b.Before(a) {
		from, to = to, from // typed backwards is still a range
	}
	return from, to, true
}

// leaveClash is the first day of the request that already has time off on it, "" if none.
//
// Odoo refuses two leaves that overlap on one day — a hard constraint, not a warning — so
// this is checked before the round trip rather than after it, the way the hour log's own
// refusals are.
func (m Model) leaveClash() string {
	from, to, ok := m.leaveRange()
	if !ok {
		return ""
	}
	taken := map[string]bool{}
	for _, l := range m.timeLeaves {
		for _, d := range daysOf(l.From, l.To) {
			taken[d] = true
		}
	}
	for _, d := range daysOf(storedToISO(from), storedToISO(to)) {
		if taken[d] {
			return d
		}
	}
	return ""
}

// askApplyLeave states what is about to be filed and asks. The prompt is built from the
// fields as they stand, so it cannot promise something the request will not say.
func (m Model) askApplyLeave() (tea.Model, tea.Cmd) {
	m.normalizeLeaveDates(leaveFromField)
	m.normalizeLeaveDates(leaveToField)
	from, to, ok := m.leaveRange()
	if !ok {
		m.err = parse.ErrDate
		return m, nil
	}
	kind, hasKind := m.leaveKind()
	if !hasKind {
		m.status = "pick a leave type first"
		return m, nil
	}
	if clash := m.leaveClash(); clash != "" {
		// Never a modal that would only be refused: the ERP allows one leave per day, and
		// this one already has one.
		m.status = "you already have time off on " + isoDate(clash) +
			" — the ERP takes one leave per day"
		return m, nil
	}

	span := from + "  →  " + to
	how := plural(m.leaveDays(), "1 day", fmt.Sprintf("%d days", m.leaveDays()))
	if m.form.half {
		span, how = from, "half day · "+m.leavePeriodName()
	}
	desc := strings.TrimSpace(m.form.desc.Value())
	if desc == "" {
		desc = "—"
	}
	// What it costs against the allocation, said rather than enforced: some types are allowed
	// to run negative, and refusing a request the ERP would have taken is worse than warning
	// about one it will not.
	takes := float64(m.leaveDays())
	if m.form.half {
		takes = 0.5
	}
	balance := fmt.Sprintf("%s left, this takes %s", days(kind.Available), days(takes))
	if takes > kind.Available {
		balance += "  — more than you have"
	}

	m.prev, m.mode = ModeForm, ModeConfirm
	m.cKind = confirmApplyLeave
	m.cPrompt = strings.Join([]string{
		"CONFIRM TIME OFF",
		"TYPE         " + strings.ToUpper(oneLine(kind.Name)),
		"DATES        " + span,
		"DURATION     " + how,
		"BALANCE      " + balance,
		"DESCRIPTION  " + trunc(oneLine(desc), 48),
	}, "\n")
	return m, nil
}

// applyLeave files the request. The line stays exactly as typed until the ERP answers: a
// refusal must leave the request on screen to fix, not lose it.
func (m Model) applyLeave() (tea.Model, tea.Cmd) {
	kind, ok := m.leaveKind()
	from, to, dates := m.leaveRange()
	switch {
	case !ok || !dates:
		m.status = "the request is not complete"
		return m, nil
	case strings.TrimSpace(m.key) == "":
		m.err = api.ErrNoKey
		return m, nil
	}
	m.applying = true
	// The days are in the status because they are what a refusal is usually about: an overlap
	// the ERP names without naming the day is unreadable unless the screen says which days
	// were asked for.
	span := from
	if to != from {
		span = from + " → " + to
	}
	m.err = nil // whatever the last attempt was refused for is not this attempt's news
	m.status = "asking the ERP for " + strings.ToLower(firstWord(kind.Name)) +
		" time off, " + span + "…"
	return m, api.RequestLeave(m.key, m.login, m.db, m.timeEmp, kind.ID,
		from, to, m.form.desc.Value(), m.form.half, m.leavePeriod())
}

// leaveKind is the type the line has selected.
func (m Model) leaveKind() (api.LeaveKind, bool) {
	if m.form.kind < 0 || m.form.kind >= len(m.timeKinds) {
		return api.LeaveKind{}, false
	}
	return m.timeKinds[m.form.kind], true
}

// leaveDays is how many days the range covers, both ends counted.
func (m Model) leaveDays() int {
	from, to, ok := m.leaveRange()
	if !ok {
		return 0
	}
	return len(daysOf(storedToISO(from), storedToISO(to)))
}

func (m Model) leavePeriod() string {
	if m.form.pm {
		return "pm"
	}
	return "am"
}

func (m Model) leavePeriodName() string {
	if m.form.pm {
		return "afternoon"
	}
	return "morning"
}

// isoDate is the reverse: the ERP's yyyy-mm-dd said the way this app writes a date.
func isoDate(iso string) string {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(iso))
	if err != nil {
		return iso
	}
	return t.Format(parse.DateLayout)
}

// storedToISO turns the app's dd/mm/yy into the yyyy-mm-dd the calendar's own maps are
// keyed by, so a typed date can be looked up beside the ERP's own.
func storedToISO(date string) string {
	t, err := time.Parse(parse.DateLayout, strings.TrimSpace(date))
	if err != nil {
		return ""
	}
	return t.Format("2006-01-02")
}

// kindByLetter is the index of the first leave type whose name starts with s. Nothing is
// hardcoded — the types come from the ERP, and so do the letters that pick them on the
// request line, which is the one place on this screen a letter still means a leave type.
func (m Model) kindByLetter(s string) (int, bool) {
	if len([]rune(s)) != 1 {
		return 0, false
	}
	for i, k := range m.timeKinds {
		if strings.HasPrefix(strings.ToLower(k.Name), strings.ToLower(s)) {
			return i, true
		}
	}
	return 0, false
}

// holdTime pins the window to a month, clamped, so the ends of the year stop rather than
// wrap.
func (m Model) holdTime(i int) Model {
	m.timeHold = min(max(i, 0), 11)
	return m
}

// timeMonth is the month the window is built around, 0 for January: whichever g, G or a
// half page pinned, else today's — or January, in a year that is not this one.
func (m Model) timeMonth() int {
	if m.timeHold >= 0 {
		return min(m.timeHold, 11)
	}
	now := time.Now()
	if m.timeYear == now.Year() {
		return int(now.Month()) - 1
	}
	return 0
}

// timeKind is a leave type by id.
func (m Model) timeKind(id int) (api.LeaveKind, bool) {
	for _, k := range m.timeKinds {
		if k.ID == id {
			return k, true
		}
	}
	return api.LeaveKind{}, false
}

// dayMark is what one square of the calendar says: the leave on it, whether that leave is
// half a day and which half, whether it is still waiting on approval, and whether the day
// was a public holiday anyway.
type dayMark struct {
	kind    string // leave type name, "" when nothing was taken
	half    bool
	period  string // "am" | "pm", only when half
	pending bool   // not approved yet
	holiday string // holiday name, "" on a working day
	// selected is a day the open new-timeoff line covers. Derived from what is typed, so a
	// half-finished date already marks the day it names.
	selected bool
}

// timeMarks is the calendar's lookup, built once per render rather than per square: every
// day of the year the filter lets through, and every public holiday. Derived, never
// stored, so a late answer joins the picture on its own.
func (m Model) timeMarks() map[string]dayMark {
	marks := map[string]dayMark{}
	for _, h := range m.timeHolidays {
		for _, d := range daysOf(h.From, h.To) {
			mark := marks[d]
			mark.holiday = h.Name
			marks[d] = mark
		}
	}
	for _, l := range m.timeLeaves {
		for _, d := range daysOf(l.From, l.To) {
			mark := marks[d]
			mark.kind, mark.half, mark.period = l.Kind, l.Half, l.Period
			mark.pending = l.State != "validate"
			marks[d] = mark
		}
	}
	// Last, so the request being typed is on top of whatever the year already holds — it is
	// the thing the keys are about.
	if from, to, ok := m.leaveRange(); ok {
		for _, d := range daysOf(storedToISO(from), storedToISO(to)) {
			mark := marks[d]
			mark.selected = true
			marks[d] = mark
		}
	}
	return marks
}

// daysOf expands an inclusive yyyy-mm-dd range. A range that will not parse is no days
// rather than an error: one unreadable row must not cost the whole year.
func daysOf(from, to string) []string {
	start, err := time.Parse("2006-01-02", from)
	if err != nil {
		return nil
	}
	end, err := time.Parse("2006-01-02", to)
	if err != nil || end.Before(start) {
		end = start
	}
	var out []string
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		out = append(out, d.Format("2006-01-02"))
	}
	return out
}

// timeTaken is the days off the year holds, counting a half day as half. It follows the
// filter, so the figure beside the calendar always answers for what is drawn on it.
func (m Model) timeTaken() float64 {
	days := 0.0
	for _, l := range m.timeLeaves {
		n := float64(len(daysOf(l.From, l.To)))
		if l.Half {
			n *= 0.5
		}
		days += n
	}
	return days
}

// --- meals -------------------------------------------------------------------

// showMeal opens the meal calendar and reads the month if it is not already on screen.
// One month in hand at a time, the way the chart holds its own: r re-reads it.
func (m Model) showMeal() (tea.Model, tea.Cmd) {
	m.tab = TabMeal
	if m.mealMonth == mealKey(m.mealViewed()) || m.mealLoading {
		return m, nil // already have it, or it is on its way
	}
	return m.loadMeal()
}

// loadMeal asks for the month in one call. The month on screen is left alone while it is in
// flight, so a re-read keeps a coherent calendar — its own days, its own count — with the
// loader beside the title, exactly as the chart and the year calendar do.
func (m Model) loadMeal() (Model, tea.Cmd) {
	if strings.TrimSpace(m.key) == "" {
		m.err = api.ErrNoKey
		return m, nil
	}
	m.mealLoading = true
	if strings.TrimSpace(m.login) == "" {
		// The day total's answer carries the email the RPC login needs.
		m.mealWanted = true
		return m, api.FetchDayHours(m.key, parse.Today())
	}
	m.mealWanted = false
	at := m.mealViewed()
	return m, api.FetchMeals(m.key, m.login, m.db, at.Year(), at.Month())
}

// updateMeal is the meal calendar. There is no cursor — a month of meals is a picture, not
// a list — so the only motion is between months, the same keys the chart steps by.
func (m Model) updateMeal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.k().PrevMonth), key.Matches(msg, m.k().NextMonth):
		by := -1
		if key.Matches(msg, m.k().NextMonth) {
			by = 1
		}
		// Forward stops at this month: the canteen has nothing to report on one that has
		// not happened, and the bookings for it do not exist yet.
		if m.mealOffset+by > 0 {
			return m, nil
		}
		m.mealOffset += by
		m.mealMonth = 0 // a new month, so it is read rather than kept
		m.mealHold = 0  // and the cursor follows today again, or the first of a past month
		return m.loadMeal()

	// The cursor walks the grid the days are laid out in: one day either way, one week up or
	// down. They are the four bindings the task list moves by, so a rebind moves both.
	case key.Matches(msg, m.k().Collapse):
		return m.holdMeal(m.mealCursor() - 1), nil
	case key.Matches(msg, m.k().Expand):
		return m.holdMeal(m.mealCursor() + 1), nil
	case key.Matches(msg, m.k().Up):
		return m.holdMeal(m.mealCursor() - 7), nil
	case key.Matches(msg, m.k().Down):
		return m.holdMeal(m.mealCursor() + 7), nil
	case key.Matches(msg, m.k().Top):
		return m.holdMeal(1), nil
	case key.Matches(msg, m.k().Bottom):
		return m.holdMeal(m.mealDays()), nil
	case key.Matches(msg, m.k().HalfDown):
		return m.holdMeal(m.mealCursor() + 7), nil
	case key.Matches(msg, m.k().HalfUp):
		return m.holdMeal(m.mealCursor() - 7), nil

	case key.Matches(msg, m.k().BookMeal):
		return m.openBookMeal()

	case key.Matches(msg, m.k().DropMeal):
		return m.openDropMeal()

	case key.Matches(msg, m.k().Delete):
		return m.askCancelMeals()

	case key.Matches(msg, m.k().Refresh):
		// mealMonth is left alone: loadMeal is called outright, so nothing needs the cache
		// key cleared, and clearing it would blank the month for as long as the read takes.
		return m.loadMeal()

	case key.Matches(msg, m.k().Quit):
		m.prev, m.mode = m.mode, ModeConfirm
		m.cKind, m.cPrompt = confirmQuit, "Quit tsk?"
		return m, nil

	case key.Matches(msg, m.k().Search), key.Matches(msg, m.k().ClearSearch):
		// Searching is a task-list act, so it takes you back there.
		m.tab, m.mode = TabTasks, ModeSearch
		if key.Matches(msg, m.k().ClearSearch) {
			m.search.SetValue("")
			m.clampCursor()
		}
		m.search.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

// mealViewed is the month on screen: this one, or however many months back mealOffset says.
// The day is the first of it, so adding months can never land on a 31st that does not exist.
func (m Model) mealViewed() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).
		AddDate(0, m.mealOffset, 0)
}

// mealKey is the cache key for a month: one number, so "have I read this" is a comparison
// rather than two.
func mealKey(t time.Time) int { return t.Year()*12 + int(t.Month()) }

// mealsOn is what is booked on one day, keyed by meal type id, derived on every render
// from the answer the ERP gave — never stored, so a re-read cannot leave a stale day behind.
func (m Model) mealsOn(day string) map[int]api.MealBooking {
	out := make(map[int]api.MealBooking, len(m.mealTypes))
	for _, b := range m.mealBookings {
		if b.Date == day {
			out[b.TypeID] = b
		}
	}
	return out
}

// mealDaysBooked counts the days with anything booked on them, and the days the canteen was
// open at all, which is what those days are worth reading against.
func (m Model) mealDaysBooked() (booked, open int) {
	at := m.mealViewed()
	days := time.Date(at.Year(), at.Month()+1, 0, 0, 0, 0, 0, time.Local).Day()
	on := make(map[string]bool, len(m.mealBookings))
	for _, b := range m.mealBookings {
		on[b.Date] = true
	}
	for d := 1; d <= days; d++ {
		day := time.Date(at.Year(), at.Month(), d, 0, 0, 0, 0, time.Local).
			Format("2006-01-02")
		if m.mealClosed[day] {
			continue
		}
		open++
		if on[day] {
			booked++
		}
	}
	return booked, open
}

// mealDays is how many days the viewed month has.
func (m Model) mealDays() int {
	at := m.mealViewed()
	return time.Date(at.Year(), at.Month()+1, 0, 0, 0, 0, 0, time.Local).Day()
}

// mealCursor is the day the cursor is on: whichever a motion pinned, else today — or the
// first of the month, in a month that is not this one, since today is not among its days.
func (m Model) mealCursor() int {
	if m.mealHold > 0 {
		return min(m.mealHold, m.mealDays())
	}
	if m.mealOffset == 0 {
		return time.Now().Day()
	}
	return 1
}

// holdMeal pins the cursor to a day, clamped, so the ends of the month stop rather than
// wrap into the next one — which would be a month this screen has not read.
func (m Model) holdMeal(d int) Model {
	m.mealHold = min(max(d, 1), m.mealDays())
	return m
}

// mealCursorDate is the cursor's day as the ERP writes it.
func (m Model) mealCursorDate() string {
	at := m.mealViewed()
	return time.Date(at.Year(), at.Month(), m.mealCursor(), 0, 0, 0, 0, time.Local).
		Format("2006-01-02")
}

// askCancelMeals opens the modal for x: cancelling a day's meals unlinks rows the canteen
// has already counted, so it names what will go rather than asking whether you are sure —
// three named meals is information, a yes/no question is not.
//
// The refusals happen here, before the round trip, the same way the hour log refuses what
// the endpoint would: a day with nothing on it, and a day the ERP has already locked, which
// it reports per booking rather than leaving us to work out from the cutoff.
func (m Model) askCancelMeals() (tea.Model, tea.Cmd) {
	day := m.mealCursorDate()
	booked := m.mealsOn(day)
	if len(booked) == 0 {
		m.status = "nothing booked on " + mealDayLabel(day)
		return m, nil
	}
	names := make([]string, 0, len(booked))
	locked := 0
	for _, t := range m.mealTypes {
		b, ok := booked[t.ID]
		if !ok {
			continue
		}
		names = append(names, strings.ToLower(firstWord(t.Name)))
		if b.Locked {
			locked++
		}
	}
	if locked == len(names) {
		m.status = mealDayLabel(day) + " is past its cutoff — the ERP will not change it"
		return m, nil
	}

	m.prev, m.mode = m.mode, ModeConfirm
	m.cKind = confirmDropMeals
	// "Clear", not "cancel": c cancels the meals it is told to over the days it is given, and
	// this takes the cursor's day whole — the prompt should say which of the two is happening.
	m.cPrompt = fmt.Sprintf("Clear %s?\n\n%d %s: %s", mealDayLabel(day),
		len(names), plural(len(names), "meal", "meals"), strings.Join(names, " · "))
	if len(names) == 1 {
		m.cPrompt = fmt.Sprintf("Clear %s on %s?", names[0], mealDayLabel(day))
	}
	return m, nil
}

// cancelMeals unlinks the cursor day's bookings and re-reads the month, so what is on screen
// is what the ERP has rather than what this screen guessed.
func (m Model) cancelMeals() (Model, tea.Cmd) {
	day := m.mealCursorDate()
	ids := make([]int, 0, len(m.mealTypes))
	for _, t := range m.mealTypes {
		if b, ok := m.mealsOn(day)[t.ID]; ok && !b.Locked {
			ids = append(ids, b.ID)
		}
	}
	if len(ids) == 0 {
		m.status = "nothing left to cancel on " + mealDayLabel(day)
		return m, nil
	}
	m.mealCancelling = true
	m.err = nil
	m.status = fmt.Sprintf("cancelling %d meals on %s…", len(ids), mealDayLabel(day))
	return m, api.CancelMeals(m.key, m.login, m.db, day, ids)
}

// mealDayLabel is a day as a person says it — "Thu 20 Aug" — for the modal and the status
// line. The grid says the number; a prompt about deleting something should say the weekday.
func mealDayLabel(iso string) string {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}
	return t.Format("Mon 2 Jan")
}

// --- the book-meal line ------------------------------------------------------

// bookFieldCount is how many fields the line has: the scope, the two dates when the scope is
// custom, then ✓ and ✕.
//
// The ticks are **not** among them. Each meal is toggled by its own initial — b, l, s — so
// tabbing onto one would be a stop that does nothing the letter does not already do, and tab
// from the last date lands where the booking is actually pressed.
func (m Model) bookFieldCount() int {
	n := 1 + 2
	if m.book.scope == scopeCustom {
		n += 2
	}
	return n
}

// The scope is always the first field; the rest is laid out around what it chose.
const bookScopeField = 0

// bookDateFields is where the two date fields sit, or -1 when the scope has no dates.
func (m Model) bookDateFields() (from, to int) {
	if m.book.scope != scopeCustom {
		return -1, -1
	}
	return 1, 2
}

func (m Model) bookOKField() int { return m.bookFieldCount() - 2 }
func (m Model) bookXField() int  { return m.bookFieldCount() - 1 }

// openBookMeal reveals the line's fields and focuses the scope, which is the first thing a
// booking has to say. The label was on screen all along and the row does not move, so nothing
// below it shifts.
func (m Model) openBookMeal() (tea.Model, tea.Cmd) { return m.openMealForm(false) }

// openDropMeal is the same line with the opposite verb: `c` cancels what `b` books, over a
// scope and a set of meals chosen the same way.
func (m Model) openDropMeal() (tea.Model, tea.Cmd) { return m.openMealForm(true) }

// openMealForm reveals the line's fields and focuses the scope. Opening either one **replaces**
// whatever was there: the row holds one line, so `b` while cancelling turns it into a booking
// rather than leaving two verbs on screen with one ✓ between them.
func (m Model) openMealForm(drop bool) (tea.Model, tea.Cmd) {
	if len(m.mealTypes) == 0 {
		// Without the types there is nothing to book, and inventing them locally would be a
		// tick that books nothing.
		m.status = "no meal types yet — r to read this month"
		return m, nil
	}
	f := mealForm{open: true, drop: drop, field: bookScopeField, on: map[int]bool{}}
	for _, in := range []*textinput.Model{&f.from, &f.to} {
		*in = textinput.New()
		in.Prompt = ""
		in.Placeholder = "dd/mm/yy"
		in.Width = fieldWidth(dateWidth)
		in.CharLimit = 8
	}
	// Today in both, which is the day you are most likely booking, and it makes the dates
	// mean something the moment the scope turns to custom.
	f.from.SetValue(parse.Today())
	f.to.SetValue(parse.Today())
	f.fresh = [2]bool{true, true}
	if drop {
		// The cancel line opens on the first scope with something in it. today is the right
		// default for booking — you book in the morning — and the wrong one here: the ERP
		// locks a day's meals at its own cutoff, so by mid-morning today is the one day that
		// cannot be cancelled, and the line opened on three empty boxes with nothing to press.
		f.scope = m.dropScope()
	}
	// Every meal ticked: booking all of them is the common case, and unticking one is a
	// keystroke where ticking three is three. On the cancel line only what is **there** to
	// cancel is ticked, since the rest cannot be.
	m.book, m.mode = f, ModeBook
	for _, t := range m.mealTypes {
		m.book.on[t.ID] = !drop || m.dropAvailable(t.ID)
	}
	m.err = nil
	// Nothing ticked on a cancel line means nothing in reach can be cancelled, and three boxes
	// reading `none` say that without saying why. The cutoff is the why.
	if drop && len(m.bookTypes()) == 0 {
		m.status = "nothing you can still cancel in the week ahead — " +
			"the ERP locks a day's meals at its cutoff"
	}
	return m, textinput.Blink
}

// rowIDs is the ids of a batch of bookings — what the calendar already holds of them, so a
// row the ERP has just answered for cannot land on the day twice.
func rowIDs(rows []api.MealBooking) []int {
	out := make([]int, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out
}

// withoutBookings is the month's bookings with the given ids gone. A new slice, since the
// model is a value everywhere else in this app and the one it was copied from still holds the
// month it was drawn from — the trap ticksWith documents for the maps.
func withoutBookings(rows []api.MealBooking, ids []int) []api.MealBooking {
	if len(ids) == 0 || len(rows) == 0 {
		return rows
	}
	gone := make(map[int]bool, len(ids))
	for _, id := range ids {
		gone[id] = true
	}
	out := make([]api.MealBooking, 0, len(rows))
	for _, r := range rows {
		if !gone[r.ID] {
			out = append(out, r)
		}
	}
	return out
}

// dropScope is the scope the cancel line opens on: the first of today, tomorrow and the week
// ahead that holds a meal this key can still cancel, so the line opens ticked rather than
// asking for three keystrokes. The week is the fallback, since its boxes reading `none` is the
// widest way of saying there is nothing.
func (m Model) dropScope() int {
	for _, scope := range []int{scopeToday, scopeTomorrow, scopeWeek} {
		probe := m
		probe.book = mealForm{open: true, drop: true, scope: scope}
		for _, t := range m.mealTypes {
			if probe.dropAvailable(t.ID) {
				return scope
			}
		}
	}
	return scopeWeek
}

// closeBookMeal takes the line back to its label. Nothing has been filed, so nothing asks:
// the meals it would have booked are one `b` away again.
func (m Model) closeBookMeal() (tea.Model, tea.Cmd) {
	m.book = mealForm{}
	m.mode, m.err = ModeList, nil
	return m, nil
}

// updateBook is the book-meal line: tab through the fields, a letter for the scope, a meal's
// own initial for its tick, enter on ✓ to book and on ✕ to close.
func (m Model) updateBook(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.k().Next):
		return m.moveBookField(1), nil
	case key.Matches(msg, m.k().Prev):
		return m.moveBookField(-1), nil

	case key.Matches(msg, m.k().Accept):
		switch m.book.field {
		case m.bookOKField():
			if m.book.drop {
				return m.askDropMeals()
			}
			return m.askBookMeals()
		case m.bookXField():
			return m.closeBookMeal()
		default:
			return m.moveBookField(1), nil
		}

	case key.Matches(msg, m.k().Cancel):
		// esc closes it outright, as ✕ does: a booking that was never filed has nothing to
		// lose, and asking about it would be a modal in front of two keystrokes of typing.
		return m.closeBookMeal()

	case key.Matches(msg, m.k().ClearField):
		if in := m.bookInput(); in != nil {
			in.SetValue("")
			if i := m.bookDateIndex(); i >= 0 {
				m.book.fresh[i] = false
			}
		}
		return m, nil
	}

	// A meal's own initial ticks it — b, l, s here, off the ERP's own names, so an office
	// that starts serving dinner gets d with nothing to edit.
	if id, ok := m.mealByLetter(msg.String()); ok {
		if m.book.drop && !m.dropAvailable(id) {
			// Nothing of that meal to cancel in this scope, so the key does nothing and says
			// so rather than moving a tick that could not act on anything.
			m.status = "no " + strings.ToLower(firstWord(m.mealName(id))) +
				" booked on those days"
			return m, nil
		}
		// Copied before it is written: the model is a value everywhere else in this app, but a
		// map inside it is a reference, so toggling in place reaches every copy that still
		// holds the old form — including the one a test or an earlier frame kept.
		m.book.on = ticksWith(m.book.on, id, !m.book.on[id])
		return m, nil
	}
	// No switching between the two verbs while a line is open: b and l and s are the meals'
	// own ticks here, and b meaning breakfast on one line and "start booking instead" on the
	// other is a key with two jobs. esc closes the row, and then b or c opens the one you
	// want.
	// space and j/k cycle the scope: a dropdown is a button as much as it is a list, and this
	// is the only dropdown on the line, so they mean it wherever the cursor is.
	if key.Matches(msg, m.k().Cycle) {
		back := msg.String() == "k" || msg.String() == "up"
		if back {
			m.book.scope = (m.book.scope - 1 + scopeCount) % scopeCount
		} else {
			m.book.scope = (m.book.scope + 1) % scopeCount
		}
		return m.retickForScope().clampBookField(), nil
	}

	in := m.bookInput()
	if in == nil {
		return m, nil
	}
	// A date field opens with its value selected: the first thing typed replaces it whole.
	if i := m.bookDateIndex(); i >= 0 && m.book.fresh[i] && msg.Type == tea.KeyRunes {
		in.SetValue("")
		m.book.fresh[i] = false
	}
	var cmd tea.Cmd
	*in, cmd = in.Update(msg)
	// A typed date changes which days the scope covers, so on the cancel line it changes what
	// is there to cancel.
	return m.retickForScope(), cmd
}

// mealName is a meal type's name by id, for the sentences that have to name one.
func (m Model) mealName(id int) string {
	for _, t := range m.mealTypes {
		if t.ID == id {
			return t.Name
		}
	}
	return "meal"
}

// ticksWith is the tick map with one meal changed, as a new map: see the note where it is
// called — the form is copied by value and the map inside it is not.
func ticksWith(on map[int]bool, id int, want bool) map[int]bool {
	out := make(map[int]bool, len(on)+1)
	for k, v := range on {
		out[k] = v
	}
	out[id] = want
	return out
}

// moveBookField normalizes the date being left — dates are rewritten on exit, never per
// keystroke — and focuses the next field, wrapping.
func (m Model) moveBookField(by int) Model {
	m.normalizeBookDates()
	n := m.bookFieldCount()
	m.book.field = (m.book.field + by + n) % n
	for _, in := range []*textinput.Model{&m.book.from, &m.book.to} {
		in.Blur()
	}
	if in := m.bookInput(); in != nil {
		in.Focus()
	}
	if i := m.bookDateIndex(); i >= 0 {
		m.book.fresh[i] = true // freshly focused: the value is selected
	}
	return m
}

// retickForScope re-derives the cancel line's ticks after the days change: a new scope is a
// new set of bookings, so what is there to cancel is a different question — and a tick left
// behind on a meal the scope no longer holds would be a tick that cannot act. The booking line
// keeps whatever was ticked, since every meal is bookable on every open day.
func (m Model) retickForScope() Model {
	if !m.book.drop {
		return m
	}
	on := make(map[int]bool, len(m.mealTypes))
	for _, t := range m.mealTypes {
		on[t.ID] = m.dropAvailable(t.ID)
	}
	m.book.on = on
	return m
}

// clampBookField keeps the cursor on a field that still exists: turning custom off takes two
// fields away, and a cursor left past the end would land on nothing.
func (m Model) clampBookField() Model {
	if n := m.bookFieldCount(); m.book.field >= n {
		m.book.field = n - 1
	}
	return m
}

// bookInput is the text field the cursor is in, or nil on the scope, a tick or a button.
func (m *Model) bookInput() *textinput.Model {
	from, to := m.bookDateFields()
	switch m.book.field {
	case from:
		return &m.book.from
	case to:
		return &m.book.to
	}
	return nil
}

// bookDateIndex is which of the two date fields has the cursor, or -1.
func (m Model) bookDateIndex() int {
	from, to := m.bookDateFields()
	switch m.book.field {
	case from:
		return 0
	case to:
		return 1
	}
	return -1
}

// mealByLetter is the id of the first meal type whose name starts with s, the same match the
// leave filters make on their own types.
func (m Model) mealByLetter(s string) (int, bool) {
	if len([]rune(s)) != 1 {
		return 0, false
	}
	for _, t := range m.mealTypes {
		if strings.HasPrefix(strings.ToLower(t.Name), strings.ToLower(s)) {
			return t.ID, true
		}
	}
	return 0, false
}

// normalizeBookDates rewrites a date field as it is left, and drags the end along when the
// start passes it — a range that reads backwards would book the days between.
func (m *Model) normalizeBookDates() {
	from, to := m.bookDateFields()
	switch m.book.field {
	case from:
		if d, err := parse.Date(m.book.from.Value(), parse.Today()); err == nil {
			m.book.from.SetValue(d)
			if before(d, m.book.to.Value()) {
				m.book.to.SetValue(d)
			}
			m.err = nil
		} else {
			m.err = err
		}
	case to:
		if d, err := parse.Date(m.book.to.Value(), m.book.from.Value()); err == nil {
			m.book.to.SetValue(d)
			m.err = nil
		} else {
			m.err = err
		}
	}
}

// bookDays is the days the line would book, in order: what the scope says, minus the days the
// canteen is shut and the days already gone.
//
// The ERP refuses a past date and a day it serves nothing on, so both are dropped here rather
// than sent to be refused one round trip later — the same rule the hour log follows.
func (m Model) bookDays() []string {
	today := time.Now()
	first, last := today, today
	switch m.book.scope {
	case scopeTomorrow:
		first = today.AddDate(0, 0, 1)
		last = first
	case scopeWeek:
		// The week ahead, today included: a canteen books forward, and "this week" on a
		// Friday would be one day.
		last = today.AddDate(0, 0, 6)
	case scopeCustom:
		a, errA := time.Parse(parse.DateLayout, strings.TrimSpace(m.book.from.Value()))
		b, errB := time.Parse(parse.DateLayout, strings.TrimSpace(m.book.to.Value()))
		if errA != nil || errB != nil {
			return nil
		}
		if b.Before(a) {
			a, b = b, a // typed backwards is still a range
		}
		first, last = a, b
	}

	var out []string
	for d := first; !d.After(last); d = d.AddDate(0, 0, 1) {
		iso := d.Format("2006-01-02")
		switch {
		case iso < today.Format("2006-01-02"):
			continue // the ERP takes no past date
		case m.mealClosed[iso]:
			continue // nothing is served, so there is nothing to book
		case d.After(today.AddDate(0, 0, 30)):
			continue // the ERP's own ceiling
		}
		out = append(out, iso)
	}
	return out
}

// bookTypes is the ticked meals, in the ERP's own order so the request reads the way the line
// does.
func (m Model) bookTypes() []int {
	var out []int
	for _, t := range m.mealTypes {
		if m.book.on[t.ID] {
			out = append(out, t.ID)
		}
	}
	return out
}

// askBookMeals states what is about to be booked and waits for a y or an n.
//
// The refusals happen here, before the modal as well as before the round trip: a day with
// nothing left to book is not worth a prompt. Days the ERP already holds are dropped the same
// way, since it keeps one booking per meal per day.
func (m Model) askBookMeals() (tea.Model, tea.Cmd) {
	days, types := m.bookWanted()
	switch {
	case len(m.bookTypes()) == 0:
		m.status = "tick a meal first"
		return m, nil
	case len(m.bookDays()) == 0:
		m.status = "no day to book — the canteen is shut on those"
		return m, nil
	case len(days) == 0:
		m.status = "already booked"
		return m, nil
	}

	names := make([]string, 0, len(types))
	for _, t := range m.mealTypes {
		if m.book.on[t.ID] {
			names = append(names, strings.ToLower(firstWord(t.Name)))
		}
	}
	when := mealDayLabel(days[0])
	if len(days) > 1 {
		when = fmt.Sprintf("%s → %s  (%d days)", mealDayLabel(days[0]),
			mealDayLabel(days[len(days)-1]), len(days))
	}
	m.prev, m.mode = m.mode, ModeConfirm
	m.cKind = confirmBookMeals
	m.cPrompt = fmt.Sprintf("Book %d %s?\n\n%s\n%s", len(names)*len(days),
		plural(len(names)*len(days), "meal", "meals"), when, strings.Join(names, " · "))
	return m, nil
}

// bookWanted is the days the request would actually create rows on, and the meals it would
// create them for: the scope's days minus the ones already booked for every ticked meal.
func (m Model) bookWanted() ([]string, []int) {
	types := m.bookTypes()
	var want []string
	for _, day := range m.bookDays() {
		on := m.mealsOn(day)
		for _, t := range types {
			if _, taken := on[t]; !taken {
				want = append(want, day)
				break
			}
		}
	}
	return want, types
}

// bookMeals sends the batch, once the modal has been answered.
func (m Model) bookMeals() (Model, tea.Cmd) {
	want, types := m.bookWanted()
	if len(want) == 0 || len(types) == 0 {
		m.status = "nothing left to book"
		return m, nil
	}

	m.booking, m.err = true, nil
	m.status = fmt.Sprintf("booking %d %s on %d %s…", len(types),
		plural(len(types), "meal", "meals"), len(want), plural(len(want), "day", "days"))
	return m, api.BookMeals(m.key, m.login, m.db, want, types)
}

// dropWanted is what the cancel line would unlink: the bookings on the scope's days for the
// ticked meals, minus the ones the ERP has already locked — it refuses to change those, so
// sending them would only collect refusals.
func (m Model) dropWanted() (ids []int, days []string, names []string) {
	types := m.bookTypes()
	seen := map[string]bool{}
	for _, day := range m.bookDays() {
		on := m.mealsOn(day)
		for _, t := range types {
			b, held := on[t]
			if !held || b.Locked {
				continue
			}
			ids = append(ids, b.ID)
			if !seen[day] {
				seen[day], days = true, append(days, day)
			}
		}
	}
	for _, t := range m.mealTypes {
		if m.book.on[t.ID] {
			names = append(names, strings.ToLower(firstWord(t.Name)))
		}
	}
	return ids, days, names
}

// dropAvailable says whether a meal has anything to cancel in the chosen scope: a booking of
// that type, on one of those days, that the ERP has not locked. It is what greys the tick —
// there is nothing for it to do, and a tick that cannot act on anything is a tick that lies.
func (m Model) dropAvailable(id int) bool {
	for _, day := range m.bookDays() {
		if b, held := m.mealsOn(day)[id]; held && !b.Locked {
			return true
		}
	}
	return false
}

// askDropMeals states what is about to go and takes **y only**: an unlink cannot be undone,
// which is the same rule x follows on a single day.
func (m Model) askDropMeals() (tea.Model, tea.Cmd) {
	ids, days, names := m.dropWanted()
	switch {
	case len(m.bookTypes()) == 0:
		m.status = "tick a meal first"
		return m, nil
	case len(m.bookDays()) == 0:
		m.status = "no day to cancel — the canteen is shut on those"
		return m, nil
	case len(ids) == 0:
		m.status = "nothing of yours to cancel on those days"
		return m, nil
	}

	when := mealDayLabel(days[0])
	if len(days) > 1 {
		when = fmt.Sprintf("%s → %s  (%d days)", mealDayLabel(days[0]),
			mealDayLabel(days[len(days)-1]), len(days))
	}
	m.prev, m.mode = m.mode, ModeConfirm
	m.cKind = confirmDropForm
	m.cPrompt = fmt.Sprintf("Cancel %d %s?\n\n%s\n%s", len(ids),
		plural(len(ids), "meal", "meals"), when, strings.Join(names, " · "))
	return m, nil
}

// dropMeals sends the unlink, once the modal has been answered.
func (m Model) dropMeals() (Model, tea.Cmd) {
	ids, days, _ := m.dropWanted()
	if len(ids) == 0 {
		m.status = "nothing left to cancel"
		return m, nil
	}
	m.mealCancelling, m.err = true, nil
	m.status = fmt.Sprintf("cancelling %d %s…", len(ids), plural(len(ids), "meal", "meals"))
	return m, api.CancelMeals(m.key, m.login, m.db, days[0], ids)
}

// --- confirm -----------------------------------------------------------------

// confirmKeys is what accepts the open modal. Anything that destroys something —
// quitting, deleting an entry — takes y alone, so a stray enter cannot do it.
// Discarding an entry you are still typing keeps y or enter.
func (m Model) confirmKeys() key.Binding {
	switch m.cKind {
	case confirmQuit, confirmDeleteRow, confirmCheckOut, confirmApplyLeave, confirmDropMeals,
		confirmDropForm:
		return m.k().YesOnly
	}
	return m.k().Yes
}

func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.k().No): // checked first: esc is in both sets
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

		case confirmFileReq:
			// Back to the line, which stays as typed until the ERP answers.
			m.mode = ModeReqForm
			return m.fileReq()

		case confirmHourLogs:
			m.mode = m.prev
			return m.confirmHours()

		case confirmCheckOut:
			// Back to whatever had the keyboard — the chart, not a task's rows, which is
			// what the other arms hard-code.
			m.mode = m.prev
			return m.toggleClock()

		case confirmApplyLeave:
			// Back to the line, and it stays as typed until the ERP answers: a refusal has
			// to have something to come back to.
			m.mode = ModeForm
			return m.applyLeave()

		case confirmDropMeals:
			m.mode = m.prev
			return m.cancelMeals()

		case confirmDropForm:
			// Back to the line: a refused cancel is worth having the fields still on screen
			// for, the same as a refused booking.
			m.mode = ModeBook
			return m.dropMeals()

		case confirmBookMeals:
			// Back to the line, which the answer closes if the ERP takes anything: a booking
			// that was refused is worth having the fields still on screen for.
			m.mode = ModeBook
			return m.bookMeals()

		case confirmDropLeave:
			// Everything typed goes with the line, which is what the prompt asked about.
			m.form = leaveForm{}
			m.mode, m.err = ModeList, nil
			return m, nil

		case confirmDropReq:
			// The same: the category and every field it asked for go with the line.
			return m.closeReqForm()

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
	case ModeLeaves:
		return "-- TIME OFF --"
	case ModeBook:
		return "-- BOOK MEAL --"
	case ModeConfirm:
		return "-- CONFIRM --"
	case ModeAuth:
		return "-- API KEY --"
	case ModeForm:
		return "-- NEW TIMEOFF --"
	case ModeWFH:
		return "-- WFH REQUEST --"
	case ModeEmpSearch, ModeProjSearch:
		return "-- SEARCH --"
	case ModeProjJump, ModeProjFound:
		return "-- FIND --"
	case ModeReqForm:
		return "-- NEW REQUISITION --"
	}
	return ""
}
