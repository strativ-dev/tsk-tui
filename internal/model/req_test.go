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
