package model

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/tasnimAlam/tsk/internal/api"
	"github.com/tasnimAlam/tsk/internal/store"
)

func sampleReqs() []store.Requisition {
	return []store.Requisition{
		{ID: 476, Category: "New Accessories Requisition", Submitted: "30/03/26",
			Deadline: "30/04/26", For: "Md. Tasnim Alam", Designation: "Senior Software Engineer",
			Stage: "Done", Note: "Model: FB35CS, for the silent switch.",
			Props: []store.Prop{{Label: "Specification", Kind: "char", Value: "Mouse, silent"}}},
		{ID: 367, Category: "Accessories Replacement Requisition", Submitted: "25/02/26",
			Deadline: "24/02/26", For: "Md. Tasnim Alam", Designation: "Senior Software Engineer",
			Stage: "Rejected", Urgent: true, Urgency: "Broken on a client call",
			Props: []store.Prop{
				{Label: "Existing Device", Kind: "many2one", Value: "#26"},
				{Label: "Purpose of Replacement", Kind: "char",
					Value: "I need a headphone, I didn't take it from office previously"},
				{Label: "Is Data Backed Up", Kind: "boolean", Value: "yes"},
			}},
		{ID: 198, Category: "Device Maintenance Requisition", Submitted: "30/11/25",
			For: "Md. Tasnim Alam", Designation: "Senior Software Engineer", Stage: "Rejected"},
	}
}

func reqModel(t *testing.T, width, height int) Model {
	t.Helper()
	m := send(t, New(), tea.WindowSizeMsg{Width: width, Height: height},
		store.KeyMsg{Key: "k", DB: "db"})
	m.login = "user@example.com" // the sync that carries the email has already landed
	return send(t, m, runes("r"), api.RequisitionsMsg{Rows: sampleReqs()})
}

// r opens the tab and reads it once; the table draws a column per field the ERP's own list view
// shows, and the urgent one is a tick rather than a word.
func TestReqTabOpensAndReads(t *testing.T) {
	m := send(t, New(), tea.WindowSizeMsg{Width: 120, Height: 30},
		store.KeyMsg{Key: "k", DB: "db"})
	m.login = "user@example.com"

	m, cmd := sendCmd(t, m, runes("r"))
	if m.tab != TabReq {
		t.Fatalf("tab = %v, want TabReq", m.tab)
	}
	if cmd == nil || !m.reqLoading {
		t.Fatal("r did not start reading the requisitions")
	}
	if v := plain(m.View()); !strings.Contains(v, "reading your requisitions…") {
		t.Errorf("an empty table mid-read does not say so:\n%s", v)
	}

	m = send(t, m, api.RequisitionsMsg{Rows: sampleReqs()})
	if m.reqLoading || len(m.reqs) != 3 {
		t.Fatalf("loading = %v, %d rows", m.reqLoading, len(m.reqs))
	}
	v := plain(m.View())
	for _, want := range []string{"CATEGORY", "SUBMITTED", "DEADLINE", "STAGE", "FOR",
		"DESIGNATION", "New Accessories", "30/03/26", "30/04/26", "Done", "Rejected",
		"Md. Tasnim Alam", "3 requisitions",
		// Whole, not cut: the column is wide enough for the title most of this office has.
		"Senior Software Engineer"} {
		if !strings.Contains(v, want) {
			t.Errorf("the table is missing %q:\n%s", want, v)
		}
	}
	// The word every category ends in is not repeated down the column.
	if strings.Contains(v, "New Accessories Requis") {
		t.Errorf("the category column repeats the word Requisition:\n%s", v)
	}
	// A deadline the ERP left empty reads as a dash, not as an empty cell.
	if !strings.Contains(v, "—") {
		t.Errorf("a missing deadline draws nothing:\n%s", v)
	}
	// One tick, on the urgent one only.
	if n := strings.Count(v, "✓"); n != 1 {
		t.Errorf("%d ticks on screen, want one — only 367 is urgent:\n%s", n, v)
	}

	// Read once a session: the stage moves while HR works, so `R` is how it is re-read.
	back := send(t, m, runes("t"))
	again, cmd := sendCmd(t, back, runes("r"))
	if cmd != nil || again.reqLoading {
		t.Error("r read the requisitions a second time")
	}
	fresh, cmd := sendCmd(t, again, runes("R"))
	if cmd == nil || !fresh.reqLoading {
		t.Fatal("R did not re-read the requisitions")
	}
	if !strings.Contains(plain(fresh.View()), "New Accessories") {
		t.Error("the table was blanked while it re-read")
	}
	// A failed re-read keeps what was on screen.
	failed := send(t, m, api.RequisitionsMsg{Err: errors.New("odoo: gone")})
	if len(failed.reqs) != 3 || failed.err == nil {
		t.Errorf("%d rows left after a failed re-read", len(failed.reqs))
	}
}

// l opens a row into the properties its own category asked for — which differ per category, so
// they are drawn from the ERP's own labels rather than from fields this app knows the names of.
func TestReqRowOpensItsProperties(t *testing.T) {
	m := send(t, reqModel(t, 120, 34), runes("j"), runes("l"))
	if !m.reqOpen[367] {
		t.Fatal("l did not open the row")
	}
	v := plain(m.View())
	for _, want := range []string{"purpose of replacement", "I need a headphone",
		"existing device", "#26", "is data backed up", "yes", "urgency",
		"Broken on a client call", "▾"} {
		if !strings.Contains(v, want) {
			t.Errorf("the open row is missing %q:\n%s", want, v)
		}
	}
	// The note is on the one that has one, and only when it is open.
	first := plain(send(t, reqModel(t, 120, 34), runes("l")).View())
	if !strings.Contains(first, "note") || !strings.Contains(first, "silent switch") {
		t.Errorf("the note is not on the open row:\n%s", first)
	}
	// h closes it; esc closes everything.
	if shut := send(t, m, runes("h")); shut.reqOpen[367] {
		t.Error("h did not close the row")
	}
	all := send(t, m, runes("g"), runes("l"), special(tea.KeyEsc))
	if len(all.reqOpen) != 0 || all.reqHold != 0 {
		t.Errorf("esc left %d rows open at %d", len(all.reqOpen), all.reqHold)
	}
	// A requisition with no properties and no note says so rather than opening onto nothing.
	bare := plain(send(t, reqModel(t, 120, 34), runes("G"), runes("l")).View())
	if !strings.Contains(bare, "nothing else on this one") {
		t.Errorf("an empty row opens onto nothing:\n%s", bare)
	}
}

// The stage reads in the colour of what it means, on every row: green settled, red not
// happening, amber waiting on somebody. The urgent column is headed with the word, and ticked
// only where it is true.
func TestReqStageColoursAndUrgent(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	const (
		green = "18;204;99"  // #12CC63
		red   = "225;52;0"   // #E13400
		amber = "224;160;48" // #E0A030, waiting on somebody
	)
	rows := sampleReqs()
	rows[2].Stage = "PM Approval" // neither settled nor refused
	m := send(t, reqModel(t, 130, 30), api.RequisitionsMsg{Rows: rows})

	if line := lineWith(t, m.View(), "New Accessories"); !strings.Contains(line, green) {
		t.Errorf("Done is not green:\n%q", line)
	}
	if line := lineWith(t, m.View(), "Accessories Replacement"); !strings.Contains(line, red) {
		t.Errorf("Rejected is not red:\n%q", line)
	}
	if line := lineWith(t, m.View(), "Device Maintenance"); !strings.Contains(line, amber) {
		t.Errorf("a stage still waiting is not amber:\n%q", line)
	}
	// The colour survives the cursor: it is information, not focus.
	held := lineWith(t, m.View(), "New Accessories")
	if !strings.Contains(held, green) {
		t.Errorf("the held row lost its stage colour:\n%q", held)
	}

	// The urgent column says what it is, and only the urgent row carries the tick.
	v := plain(m.View())
	if !strings.Contains(v, "URGENT") {
		t.Errorf("the urgent column is not headed:\n%s", v)
	}
	if n := strings.Count(v, "✓"); n != 1 {
		t.Errorf("%d ticks, want one:\n%s", n, v)
	}
	if urgent := lineWith(t, plain(m.View()), "Accessories Replacement"); !strings.Contains(urgent, "✓") {
		t.Errorf("the urgent row has no tick:\n%q", urgent)
	}
}

// The cursor walks the table and the held row takes the accent.
func TestReqCursorWalks(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	m := reqModel(t, 120, 30)
	if !strings.Contains(lineWith(t, m.View(), "New Accessories"), "255;192;0") {
		t.Error("the first row is not accented")
	}
	next := send(t, m, runes("j"))
	if next.reqHold != 1 {
		t.Fatalf("j held row %d", next.reqHold)
	}
	if strings.Contains(lineWith(t, next.View(), "New Accessories"), "255;192;0") {
		t.Error("the accent stayed on the first row")
	}
	if end := send(t, m, runes("G")); end.reqHold != 2 {
		t.Errorf("G held row %d, want the last", end.reqHold)
	}
	// Clamped at both ends.
	if up := send(t, m, runes("k")); up.reqHold != 0 {
		t.Errorf("k walked past the first row to %d", up.reqHold)
	}
	if down := send(t, send(t, m, runes("G")), runes("j")); down.reqHold != 2 {
		t.Errorf("j walked past the last row to %d", down.reqHold)
	}
}

// Narrow terminals drop columns from the right, and nothing ever exceeds the width.
func TestReqFitsTheTerminal(t *testing.T) {
	if wide, narrow := len(reqModel(t, 140, 30).reqCols()), len(reqModel(t, 70, 24).reqCols()); narrow >= wide {
		t.Errorf("a 70-cell terminal keeps %d columns and a 140-cell one %d", narrow, wide)
	}
	// The category, the dates and the stage are what a requisition is, so they stay.
	heads := map[string]bool{}
	for _, c := range reqModel(t, 70, 24).reqCols() {
		heads[c.head] = true
	}
	for _, want := range []string{"CATEGORY", "SUBMITTED", "STAGE"} {
		if !heads[want] {
			t.Errorf("a 70-cell terminal dropped %s", want)
		}
	}

	for _, size := range [][2]int{{60, 20}, {70, 24}, {80, 24}, {120, 30}, {200, 40}} {
		w, h := size[0], size[1]
		for _, m := range []Model{
			reqModel(t, w, h),
			send(t, reqModel(t, w, h), runes("j"), runes("l")),
			send(t, reqModel(t, w, h), runes("G"), runes("l")),
		} {
			lines := strings.Split(m.View(), "\n")
			if len(lines) > h {
				t.Errorf("%dx%d: %d lines", w, h, len(lines))
			}
			for i, l := range lines {
				if got := lipgloss.Width(l); got > w {
					t.Errorf("%dx%d: line %d is %d cells: %q", w, h, i, got, l)
				}
			}
		}
	}
}

// r is the tab from every screen, and 6 reaches it too; R is what re-reads, everywhere.
func TestReqTabKeyAndRefresh(t *testing.T) {
	m := reqModel(t, 120, 30)
	if byDigit := send(t, send(t, m, runes("t")), runes("6")); byDigit.tab != TabReq {
		t.Error("6 does not open the requisitions")
	}
	// The old refresh key is the tab now, so it must not re-read the task list either.
	tasks := send(t, m, runes("t"))
	if _, cmd := sendCmd(t, tasks, runes("r")); cmd != nil {
		t.Error("r still fetches on the task list")
	}
	if got := send(t, tasks, runes("r")); got.tab != TabReq {
		t.Errorf("r from the task list went to %v", got.tab)
	}
	if _, cmd := sendCmd(t, tasks, runes("R")); cmd == nil {
		t.Error("R does not fetch on the task list")
	}
}

func sampleCats() []store.ReqCategory {
	return []store.ReqCategory{
		{ID: 14, Name: "Team Outing", Fields: []store.ReqField{
			{Name: "num_people", Kind: "integer", Label: "No. of People", Required: true},
			{Name: "maximum_limit", Kind: "float", Label: "Maximum Limit", Required: true},
		}},
		{ID: 17, Name: "Software Requisition", Fields: []store.ReqField{
			{Name: "software_name", Kind: "char", Label: "Software Name", Required: true},
			{Name: "reason", Kind: "char", Label: "Purpose/Reason", Required: true},
			{Name: "deadline", Kind: "date", Label: "Deadline", Required: true},
		}},
		{ID: 5, Name: "Accessories Replacement Requisition", Fields: []store.ReqField{
			{Name: "existing_device_id", Kind: "many2one", Label: "Existing Device",
				Required: true, Comodel: "maintenance.equipment",
				Opts: []store.Opt{{ID: 12, Name: "Mackbook Pro"}, {ID: 22, Name: "Headphone"}}},
			{Name: "purpose_of_replacement", Kind: "char", Label: "Purpose of Replacement",
				Required: true},
			{Name: "is_data_backed_up", Kind: "boolean", Label: "Is Data Backed Up",
				Required: true},
		}},
	}
}

// formModel is the tab with the categories in hand and the line open on the given one.
func formModel(t *testing.T, cat string) Model {
	t.Helper()
	m := send(t, reqModel(t, 200, 34), api.ReqCategoriesMsg{Categories: sampleCats()}, runes("n"))
	for range len(sampleCats()) + 1 {
		if got, ok := m.reqCat(); ok && got.Name == cat {
			return m
		}
		m = send(t, m, runes("j"))
	}
	t.Fatalf("never reached %q", cat)
	return m
}

// n opens the line, and nothing is on it until a category is chosen: the fields **are** the
// category's, so there is nothing to draw before one is picked.
func TestNewReqLineOpensOnTheCategory(t *testing.T) {
	m := reqModel(t, 200, 34)
	closed := m.View()
	if !strings.Contains(plain(closed), "new requisition") {
		t.Fatalf("the label is not on screen closed:\n%s", plain(closed))
	}

	open, cmd := sendCmd(t, m, runes("n"))
	if open.mode != ModeReqForm || !open.req.open {
		t.Fatalf("mode = %v, open = %v", open.mode, open.req.open)
	}
	if cmd == nil || !open.reqLoading {
		t.Fatal("n did not read the categories")
	}
	if v := plain(open.View()); !strings.Contains(v, "reading the categories…") {
		t.Errorf("the line does not say it is reading:\n%s", v)
	}
	// Opening it moves nothing: the row is there either way.
	landed := send(t, open, api.ReqCategoriesMsg{Categories: sampleCats()})
	if a, b := len(strings.Split(closed, "\n")), len(strings.Split(landed.View(), "\n")); a != b {
		t.Errorf("opening the line changed the height: %d → %d", a, b)
	}
	if rowOf(t, plain(closed), "CATEGORY  ") != rowOf(t, plain(landed.View()), "CATEGORY  ") {
		t.Error("opening the line moved the table")
	}
	// Nothing but the dropdown until one is chosen.
	v := plain(landed.View())
	if !strings.Contains(v, "pick a category ▾") || !strings.Contains(v, "pick one") {
		t.Errorf("the closed dropdown does not ask for one:\n%s", v)
	}
	if strings.Contains(v, "✓") && landed.reqFieldCount() != 1 {
		t.Errorf("the line drew its buttons before a category was chosen:\n%s", v)
	}

	// The categories are read once: closing and reopening asks nothing.
	shut := send(t, landed, special(tea.KeyEsc))
	if shut.req.open || shut.mode == ModeReqForm {
		t.Error("esc did not close the line")
	}
	if _, cmd := sendCmd(t, shut, runes("n")); cmd != nil && shut.reqCatsRead {
		if again := send(t, shut, runes("n")); again.reqLoading {
			t.Error("the categories were read a second time")
		}
	}
}

// The category says what the line asks for: choosing one builds its fields, and choosing
// another throws them away — they belonged to fields that no longer exist.
func TestNewReqFieldsFollowTheCategory(t *testing.T) {
	soft := formModel(t, "Software Requisition")
	v := plain(soft.View())
	for _, want := range []string{"Software Requisition ▾", "software name *",
		"purpose/reason *", "deadline *", "urgent", "note", "✓", "✕"} {
		if !strings.Contains(v, want) {
			t.Errorf("the line is missing %q:\n%s", want, v)
		}
	}
	// Its own count: the dropdown, three fields, urgent, the note and the two buttons.
	if got := soft.reqFieldCount(); got != 8 {
		t.Errorf("the line has %d fields, want 8", got)
	}

	// Typed, then the category changes: the values go with the fields they belonged to.
	typed := send(t, soft, special(tea.KeyTab), runes("Figma"))
	if typed.req.inputs[0].Value() != "Figma" {
		t.Fatalf("the field did not take the text: %q", typed.req.inputs[0].Value())
	}
	moved := send(t, typed, special(tea.KeyShiftTab), runes("j"))
	if got, _ := moved.reqCat(); got.Name == "Software Requisition" {
		t.Fatal("j did not step the category")
	}
	for _, in := range moved.req.inputs {
		if in.Value() != "" {
			t.Errorf("a value survived the category change: %q", in.Value())
		}
	}

	// A replacement asks different things, including a device to pick and a box to tick.
	repl := formModel(t, "Accessories Replacement Requisition")
	rv := plain(repl.View())
	for _, want := range []string{"Mackbook Pro ▾", "purpose of replacement *",
		"is data backed up *", "☐ no"} {
		if !strings.Contains(rv, want) {
			t.Errorf("the replacement line is missing %q:\n%s", want, rv)
		}
	}
	// The device dropdown steps its own options.
	picked := send(t, repl, special(tea.KeyTab), runes("j"))
	if !strings.Contains(plain(picked.View()), "Headphone ▾") {
		t.Errorf("j did not step the device:\n%s", plain(picked.View()))
	}
	// The tick is toggled with space, and urgent reveals its cause.
	ticked := send(t, picked, special(tea.KeyTab), special(tea.KeyTab), runes(" "))
	if !ticked.req.on["is_data_backed_up"] {
		t.Error("space did not tick the box")
	}
	urgent := send(t, ticked, special(tea.KeyTab), runes(" "))
	if !urgent.req.urgent || urgent.reqUrgencyField() < 0 {
		t.Error("urgent did not reveal its cause")
	}
	if !strings.Contains(plain(urgent.View()), "why it cannot wait") {
		t.Errorf("the cause has no line:\n%s", plain(urgent.View()))
	}
}

// The keyboard is on the form, so the table gives up its accent while it is open — a highlighted
// row says the keys are somewhere they are not. Every field reads as a field, and the one holding
// the keys is the accent one.
func TestNewReqFormMarksItsOwnFields(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	const (
		accent = "255;192;0" // the frame and the label of the field with the keys
		frame  = "43;43;58"  // theme.Rule, the frame every other field has
	)
	shut := reqModel(t, 120, 34)
	if !strings.Contains(lineWith(t, shut.View(), "New Accessories"), accent) {
		t.Fatal("the table's own row is not accented with the form closed")
	}

	m := formModel(t, "Software Requisition")
	if line := lineWith(t, m.View(), "New Accessories"); strings.Contains(line, accent) {
		t.Errorf("the table keeps its accent while the form has the keys:\n%q", line)
	}
	// Every value is framed, the same rounded field the time off line has, so an empty one is
	// still visibly somewhere to type.
	for _, needle := range []string{"software name", "purpose/reason", "deadline", "note"} {
		line := lineWith(t, m.View(), needle)
		if !strings.Contains(line, frame) && !strings.Contains(line, accent) {
			t.Errorf("%q has no field to type in:\n%q", needle, line)
		}
		if !strings.Contains(plain(line), "│") {
			t.Errorf("%q is not a field:\n%q", needle, plain(line))
		}
	}
	// The frame with the keys is the accent one, and it moves with tab.
	if line := lineWith(t, m.View(), "new requisition"); !strings.Contains(line, accent) {
		t.Errorf("the focused field is not framed in the accent:\n%q", line)
	}
	next := send(t, m, special(tea.KeyTab))
	if line := lineWith(t, next.View(), "new requisition"); strings.Contains(line, accent) {
		t.Errorf("the accent frame stayed on the category:\n%q", line)
	}
	if line := lineWith(t, next.View(), "software name"); !strings.Contains(line, accent) {
		t.Errorf("the accent frame did not move with tab:\n%q", line)
	}
	// A chooser reads in the accent while it holds the keys, so stepping one says so where it
	// happened rather than only in the label beside it.
	if line := lineWith(t, m.View(), "new requisition"); !strings.Contains(line, accent) {
		t.Errorf("the dropdown's own value is not accented:\n%q", line)
	}
}

// A date field reads what is typed into it the way every other date in this app does, and
// normalizes on the way out rather than per keystroke.
func TestNewReqDateNormalizesOnTab(t *testing.T) {
	m := formModel(t, "Software Requisition")
	at := func(m Model, field int) Model {
		for m.req.field != field {
			m = send(t, m, special(tea.KeyTab))
		}
		return m
	}
	// The deadline is the category's fourth field; dd/mm/yy is what it says it takes.
	deadline := at(m, 3)
	if !strings.Contains(plain(deadline.View()), "dd/mm/yy") {
		t.Errorf("the date field does not say its shape:\n%s", plain(deadline.View()))
	}
	typed := send(t, deadline, runes("30"))
	if got := typed.req.inputs[2].Value(); got != "30" {
		t.Fatalf("the field holds %q", got)
	}
	left := send(t, typed, special(tea.KeyTab))
	// dd/mm/yy, with the month and the year filled in from today — the insert row's own grammar.
	if got := left.req.inputs[2].Value(); len(got) != 8 || !strings.HasPrefix(got, "30/") {
		t.Errorf("the date was not normalized on the way out: %q", got)
	}
}

// Space types where a field takes letters: j/k/space step a chooser, and matched before the
// input they swallowed the space bar in every text field on the form.
func TestNewReqFieldsTakeSpace(t *testing.T) {
	m := formModel(t, "Software Requisition")
	typed := send(t, m, special(tea.KeyTab), runes("Adobe Creative Cloud"))
	if got := typed.req.inputs[0].Value(); got != "Adobe Creative Cloud" {
		t.Errorf("the field holds %q — the space bar went somewhere else", got)
	}
	// And on the chooser it still steps: the category, not a space.
	stepped := send(t, m, runes(" "))
	if got, _ := stepped.reqCat(); got.Name == "Software Requisition" {
		t.Error("space did not step the category")
	}
}

// ✓ asks first, and refuses what the ERP would: every field the category calls required.
func TestNewReqTickAsksAndRefuses(t *testing.T) {
	m := formModel(t, "Software Requisition")
	at := func(m Model, field int) Model {
		for m.req.field != field {
			m = send(t, m, special(tea.KeyTab))
		}
		return m
	}

	blank, cmd := sendCmd(t, at(m, m.reqOKField()), special(tea.KeyEnter))
	if cmd != nil || blank.mode == ModeConfirm {
		t.Error("a requisition with nothing in it was sent")
	}
	if !strings.Contains(blank.status, "required") {
		t.Errorf("status = %q", blank.status)
	}

	// Filled in, and the prompt states what is about to be filed.
	filled := m
	for i, text := range []string{"Figma", "design handoff", "30/09/26"} {
		filled = at(filled, 1+i)
		filled = send(t, filled, runes(text))
	}
	asked := send(t, at(filled, filled.reqOKField()), special(tea.KeyEnter))
	if asked.mode != ModeConfirm || asked.cKind != confirmFileReq {
		t.Fatalf("mode = %v, kind = %v", asked.mode, asked.cKind)
	}
	for _, want := range []string{"Software Requisition", "software name: Figma",
		"deadline: 2026-09-30"} {
		if !strings.Contains(asked.cPrompt, want) {
			t.Errorf("the prompt is missing %q:\n%s", want, asked.cPrompt)
		}
	}
	// n comes back to the line with everything on it.
	back := send(t, asked, runes("n"))
	if back.mode != ModeReqForm || back.req.inputs[0].Value() != "Figma" {
		t.Errorf("n lost the line: mode = %v", back.mode)
	}
	// y files it, and the line stays up until the ERP answers.
	filed, cmd := sendCmd(t, asked, runes("y"))
	if cmd == nil || !filed.filing || filed.mode != ModeReqForm {
		t.Fatalf("y did not file it: filing = %v, mode = %v", filed.filing, filed.mode)
	}
	if !filed.busy() {
		t.Error("the spinner will not animate while the create is out")
	}

	// The answer closes the line and re-reads the table; a refusal keeps it as typed.
	done, cmd := sendCmd(t, filed, api.RequisitionFiledMsg{ID: 902})
	if done.req.open || cmd == nil {
		t.Errorf("open = %v after it was filed", done.req.open)
	}
	if !strings.Contains(done.status, "waiting on approval") {
		t.Errorf("status = %q", done.status)
	}
	refused := send(t, filed, api.RequisitionFiledMsg{Err: errors.New("odoo: no")})
	if !refused.req.open || refused.req.inputs[0].Value() != "Figma" {
		t.Error("a refusal closed the line")
	}
}

// ✕ and esc close the line outright — nothing has been filed — and the label comes back.
func TestNewReqLineClosesWithoutAsking(t *testing.T) {
	m := formModel(t, "Software Requisition")
	x := m
	for x.req.field != x.reqXField() {
		x = send(t, x, special(tea.KeyTab))
	}
	for _, shut := range []Model{
		send(t, x, special(tea.KeyEnter)),
		send(t, m, special(tea.KeyEsc)),
	} {
		if shut.req.open || shut.mode == ModeConfirm {
			t.Errorf("the line is still open: mode = %v", shut.mode)
		}
		if !strings.Contains(plain(shut.View()), "new requisition") {
			t.Error("the label did not come back")
		}
	}
}

// The line owns the keyboard while it is open: a purpose can hold the letters the tabs are.
func TestNewReqLineOwnsTheKeyboard(t *testing.T) {
	m := send(t, formModel(t, "Software Requisition"), special(tea.KeyTab), runes("meeting notes"))
	if m.tab != TabReq {
		t.Errorf("typing changed the tab to %v", m.tab)
	}
	if got := m.req.inputs[0].Value(); got != "meeting notes" {
		t.Errorf("the field holds %q", got)
	}
}

// The form is one field a line under the table, with the buttons against the right edge: nothing
// on it may exceed the width, and its rows come out of the table's own budget rather than off the
// bottom of the screen.
func TestNewReqLineFits(t *testing.T) {
	// The buttons line up under the values, past the label column: pushed to the right edge they
	// belonged to nothing on the form.
	wide := formModel(t, "Software Requisition")
	lines := strings.Split(plain(wide.View()), "\n")
	// The button row, not the table's own urgent tick: the boxed mark is what tells them apart.
	btn := rowOf(t, plain(wide.View()), "│ ✓ │")
	before, _, _ := strings.Cut(lines[btn], "│")
	// The label column plus the two cells every blurred line starts with: the same cell a value
	// starts on.
	if at, want := lipgloss.Width(before), gutter+reqFormLabel; at != want {
		t.Errorf("the buttons start at cell %d, want %d:\n%q", at, want, lines[btn])
	}
	// And under the table, not above it.
	if rowOf(t, plain(wide.View()), "CATEGORY  ") > btn {
		t.Error("the form is above the table")
	}
	if rowOf(t, plain(wide.View()), "new requisition ") > btn {
		t.Error("the buttons are above the fields they commit")
	}

	for _, size := range [][2]int{{200, 34}, {166, 30}, {120, 24}, {100, 20}, {80, 24}} {
		w, h := size[0], size[1]
		for _, cat := range []string{"Software Requisition", "Accessories Replacement Requisition"} {
			m := send(t, reqModel(t, w, h), api.ReqCategoriesMsg{Categories: sampleCats()},
				runes("n"))
			for i := 0; i < len(sampleCats()); i++ {
				if got, ok := m.reqCat(); ok && got.Name == cat {
					break
				}
				m = send(t, m, runes("j"))
			}
			urgent := m
			for urgent.req.field != urgent.reqUrgentField() {
				urgent = send(t, urgent, special(tea.KeyTab))
			}
			urgent = send(t, urgent, runes(" "))
			for _, v := range []Model{m, urgent} {
				lines := strings.Split(v.View(), "\n")
				if len(lines) > h {
					t.Errorf("%s at %dx%d: %d lines", cat, w, h, len(lines))
				}
				for i, l := range lines {
					if got := lipgloss.Width(l); got > w {
						t.Errorf("%s at %d cells: line %d is %d wide: %q", cat, w, i, got, l)
					}
				}
			}
		}
	}
}
