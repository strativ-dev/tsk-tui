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

func sampleProjects() []store.Project {
	return []store.Project{
		{ID: 850, Name: "AI Sales", Teams: []string{"AI Sales"}, Tasks: 9},
		{ID: 849, Name: "AI Transformation", Manager: "Reaz Abedin",
			Teams: []string{"UX/UI Designers", "AI implementation"}, Tasks: 181,
			Members: []int{557, 540, 26}, Mine: true},
		{ID: 858, Name: "Boo Företagsportalen - Underhåll", Tasks: 0},
		{ID: 787, Name: "Value-Driven Engagement, Internal Meetings & Tasks",
			Teams: []string{"Strativ dev team", "HR and administration"}, Tasks: 753,
			Mine: true},
	}
}

func projModel(t *testing.T, width, height int) Model {
	t.Helper()
	m := send(t, New(), tea.WindowSizeMsg{Width: width, Height: height},
		store.KeyMsg{Key: "k", DB: "db"})
	m.login = "user@example.com" // the sync that carries the email has already landed
	return send(t, m, runes("p"), api.ProjectsMsg{Projects: sampleProjects()})
}

// p opens the tab and reads it once: a row a project, the teams on it, and the task count on
// the right edge where a task's own entry count sits.
func TestProjTabOpensAndReads(t *testing.T) {
	m := send(t, New(), tea.WindowSizeMsg{Width: 120, Height: 34},
		store.KeyMsg{Key: "k", DB: "db"})
	m.login = "user@example.com"

	opened, cmd := sendCmd(t, m, runes("p"))
	if opened.tab != TabProj {
		t.Fatalf("p left the tab at %v", opened.tab)
	}
	if cmd == nil || !opened.projLoading {
		t.Fatal("p did not read the projects")
	}
	if v := plain(opened.View()); !strings.Contains(v, "reading the projects…") {
		t.Errorf("the body does not say it is reading:\n%s", v)
	}

	// It opens on mine, so the whole list needs the toggle pressed.
	landed := send(t, opened, api.ProjectsMsg{Projects: sampleProjects()}, runes("a"))
	v := plain(landed.View())
	for _, want := range []string{
		"PROJECTS", "4 open projects",
		"AI Sales", "9 tasks",
		"AI Transformation", "UX/UI Designers, AI implementation", "181 tasks",
		"Boo Företagsportalen - Underhåll", "0 tasks",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("the list is missing %q:\n%s", want, v)
		}
	}
	// One task reads as one task, not "1 tasks".
	one := send(t, landed, api.ProjectsMsg{Projects: []store.Project{
		{ID: 1, Name: "Solo", Tasks: 1, Mine: true}}})
	if got := plain(one.View()); !strings.Contains(got, "1 task ") &&
		!strings.HasSuffix(strings.TrimRight(lineWith(t, got, "Solo"), " "), "1 task") {
		t.Errorf("a single task is not singular:\n%s", got)
	}

	// Read once a session: coming back does not ask again.
	away := send(t, landed, runes("t"))
	if again, cmd := sendCmd(t, away, runes("p")); cmd != nil || again.projLoading {
		t.Error("the projects were read a second time")
	}
	// R does ask again, and the list stays up while it does.
	fresh, cmd := sendCmd(t, landed, runes("R"))
	if cmd == nil || !fresh.projLoading {
		t.Fatal("R did not re-read the projects")
	}
	if !strings.Contains(plain(fresh.View()), "AI Sales") {
		t.Error("the re-read emptied the list")
	}
	if !fresh.busy() {
		t.Error("the spinner will not animate while the read is out")
	}
}

// A failed re-read keeps the list it had: an empty screen is not the answer "no projects".
func TestProjKeepsItsListOnAFailedRead(t *testing.T) {
	m := send(t, projModel(t, 120, 34), runes("a"))
	kept := send(t, m, api.ProjectsMsg{Err: errors.New("odoo: access denied")})
	if len(kept.projs) != len(sampleProjects()) {
		t.Errorf("the list holds %d projects after a refusal", len(kept.projs))
	}
	if !strings.Contains(kept.status, "unchanged") {
		t.Errorf("status = %q", kept.status)
	}
	if !strings.Contains(plain(kept.View()), "AI Sales") {
		t.Error("the rows went with the refusal")
	}
}

// The cursor walks the list and the accent follows it. There is nothing to open here — the row
// is the whole of what the ERP publishes — so the motions are the only keys.
func TestProjCursorWalks(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	const accent = "255;192;0" // #FFC000

	m := send(t, projModel(t, 120, 34), runes("a")) // all four, so there is a list to walk
	if m.projHold != 0 {
		t.Fatalf("the cursor opened on row %d", m.projHold)
	}
	if !strings.Contains(lineWith(t, m.View(), "AI Sales"), accent) {
		t.Error("the first row does not hold the accent")
	}

	down := send(t, m, runes("j"))
	if down.projHold != 1 {
		t.Errorf("j left the cursor on %d", down.projHold)
	}
	if strings.Contains(lineWith(t, down.View(), "AI Sales"), accent) {
		t.Error("the accent stayed on the row the cursor left")
	}

	// Clamped at both ends: wrapping would land on a project the list does not hold.
	if top := send(t, down, runes("k"), runes("k"), runes("k")); top.projHold != 0 {
		t.Errorf("k ran past the top: %d", top.projHold)
	}
	end := send(t, m, runes("G"))
	if end.projHold != len(sampleProjects())-1 {
		t.Errorf("G left the cursor on %d", end.projHold)
	}
	if last := send(t, end, runes("j")); last.projHold != end.projHold {
		t.Errorf("j ran past the end: %d", last.projHold)
	}
	// esc puts it back at the top, which is the only thing to undo on this screen.
	if back := send(t, end, special(tea.KeyEsc)); back.projHold != 0 {
		t.Errorf("esc left the cursor on %d", back.projHold)
	}
	if g := send(t, end, runes("g")); g.projHold != 0 {
		t.Errorf("g left the cursor on %d", g.projHold)
	}
}

// Nothing on the screen may exceed the terminal width, at any width the app supports: the tab
// bar is the first line, and a wrapped one pushes the whole UI down a row.
func TestProjFitsTheTerminal(t *testing.T) {
	for _, w := range []int{60, 80, 100, 120, 200} {
		m := send(t, projModel(t, w, 30), runes("a")) // all of them, the longest list
		for i, line := range strings.Split(m.View(), "\n") {
			if got := lipgloss.Width(line); got > w {
				t.Errorf("at %d cells line %d is %d wide: %q", w, i, got, plain(line))
			}
		}
		// The name is what the row is for, so it keeps its cells: a project whose teams are
		// cut still says which project it is.
		if !strings.Contains(plain(m.View()), "AI Sales") {
			t.Errorf("at %d cells the first name is gone:\n%s", w, plain(m.View()))
		}
	}
}

// The teams read as a column: the name takes a fixed 60 cells, so every chip starts on the same
// one — and the count is reserved at its widest over the list, or "1315 tasks" beside "9 tasks"
// would move the columns apart on a terminal narrow enough for the name to be giving up cells.
func TestProjTeamsShareOneColumn(t *testing.T) {
	wide := append(sampleProjects(), store.Project{ID: 2, Name: "ERP 360",
		Teams: []string{"STRATIV ERP 360 TEAM"}, Tasks: 1315})
	for _, w := range []int{72, 80, 100, 120, 160} {
		m := send(t, projModel(t, w, 30), runes("a"))
		m.projs = wide

		first := -1
		for _, p := range m.projs {
			if len(p.Teams) == 0 {
				continue // no chip on a project with no team, so nothing to line up
			}
			// A prefix, not the whole name: the row cuts long ones to its own column.
			r := []rune(p.Name)
			line := plain(lineWith(t, m.View(), string(r[:min(8, len(r))])))
			at := strings.Index(line, "│")
			if at < 0 {
				t.Fatalf("at %d cells %q drew no chip: %q", w, p.Name, line)
			}
			// In cells, not bytes: the focused row's own border is one cell and three bytes,
			// so a byte offset reports it two columns further along than it is drawn.
			got := lipgloss.Width(line[:at])
			if first < 0 {
				first = got
			}
			if got != first {
				t.Errorf("at %d cells the chip on %q starts at %d, the first at %d:\n%s",
					w, p.Name, got, first, plain(m.View()))
			}
		}
		// And it is where the fixed column puts it, wherever the terminal holds it.
		// Where the fixed column puts it: the row's own left margin, the indent, the name.
		want := gutter + 3 + projNameCells
		if name, _ := m.projColumns(); name == projNameCells && first != want {
			t.Errorf("at %d cells the chips start at %d, want %d", w, first, want)
		}
	}
}

func sampleMembers() []store.Member {
	return []store.Member{
		{Name: "Ashik Ahamed Aman Rafat", Email: "ashik.rafat@strativ.se"},
		{Name: "Benjamin Sonmez", Email: "benjamin.sonmez@strativ.se"},
		{Name: "Md. Tasnim Alam", Email: "tasnim@strativ.se"},
	}
}

// l opens a row into who runs the project and everyone on its teams, as a table of names and
// work emails. The manager came with the list — a many2one arrives named — so the read that
// opening costs is only ever about the people.
func TestProjRowOpensItsPeople(t *testing.T) {
	// All of them, so the rows are the fixture's own order; AI Transformation has both a
	// manager and members.
	m := send(t, projModel(t, 120, 40), runes("a"), runes("j"))

	open, cmd := sendCmd(t, m, runes("l"))
	if !open.projOpen[849] {
		t.Fatal("l did not open the row")
	}
	if cmd == nil || !open.projPulling[849] {
		t.Fatal("l did not read the project's people")
	}
	if !open.busy() {
		t.Error("the spinner will not animate while the read is out")
	}
	if v := plain(open.View()); !strings.Contains(v, "reading its people…") {
		t.Errorf("the open row does not say it is reading:\n%s", v)
	}
	// The manager is on screen before the read lands: it came with the list, and its label is
	// a column head like the table's own.
	if v := plain(open.View()); !strings.Contains(v, "MANAGER") ||
		!strings.Contains(v, "Reaz Abedin") {
		t.Errorf("the manager is missing:\n%s", v)
	}

	landed := send(t, open, api.ProjectMembersMsg{ID: 849, Members: sampleMembers()})
	if landed.projPulling[849] {
		t.Error("the read is still marked in flight")
	}
	v := plain(landed.View())
	for _, want := range []string{"TEAM MEMBERS", "NAME", "EMAIL",
		"Ashik Ahamed Aman Rafat", "ashik.rafat@strativ.se",
		"Md. Tasnim Alam", "tasnim@strativ.se"} {
		if !strings.Contains(v, want) {
			t.Errorf("the table is missing %q:\n%s", want, v)
		}
	}
	// The section sits between the manager and the table it names.
	if a, b, c := rowOf(t, v, "MANAGER"), rowOf(t, v, "TEAM MEMBERS"),
		rowOf(t, v, "NAME"); !(a < b && b < c) {
		t.Errorf("manager at %d, section at %d, table head at %d", a, b, c)
	}
	// The rows sit under the project they belong to, not under the next one.
	table := rowOf(t, v, "NAME")
	if row := rowOf(t, v, "AI Transformation"); table < row {
		t.Errorf("the table is at %d and its project at %d", table, row)
	}
	if next := rowOf(t, v, "Boo Företagsportalen"); table > next {
		t.Errorf("the table is at %d, past the next project at %d", table, next)
	}

	// Read once per project: a second l asks nothing.
	if again, cmd := sendCmd(t, landed, runes("l")); cmd != nil || again.projPulling[849] {
		t.Error("the people were read a second time")
	}
	// h closes the row and keeps what was read.
	shut := send(t, landed, runes("h"))
	if shut.projOpen[849] {
		t.Error("h did not close the row")
	}
	if len(shut.projMembers[849]) != len(sampleMembers()) {
		t.Error("closing the row threw away what was read")
	}
	if strings.Contains(plain(shut.View()), "ashik.rafat@strativ.se") {
		t.Error("the table is still on screen")
	}

	// A project whose teams have nobody on them says so rather than reading nothing forever.
	bare, cmd := sendCmd(t, send(t, projModel(t, 120, 40), runes("a"), runes("j"), runes("j")),
		runes("l"))
	if cmd != nil || bare.projPulling[858] {
		t.Error("a project with no member ids still asked the ERP")
	}
	if v := plain(bare.View()); !strings.Contains(v, "no members on its teams") {
		t.Errorf("the empty case is missing:\n%s", v)
	}
	// The section stands over it: an empty answer is still about the members.
	if v := plain(bare.View()); !strings.Contains(v, "TEAM MEMBERS") {
		t.Errorf("the section is missing over the empty case:\n%s", v)
	}
}

// i focuses the query box in the header — the task list's own key and its own box — and the
// list narrows as it is typed. esc and enter hand the rows the keyboard back; the query stands
// until esc is pressed on the list, which is what that key means on this tab.
func TestProjFilter(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	const accent = "255;192;0" // #FFC000

	m := send(t, projModel(t, 120, 40), runes("a"), runes("i"))
	if m.mode != ModeProjSearch {
		t.Fatalf("i left the mode at %v", m.mode)
	}
	// The box takes the accent frame while it holds the keys, with the caret in the gutter.
	box := lineWith(t, m.View(), "❯")
	if !strings.Contains(box, accent) {
		t.Errorf("the focused box is not in the accent: %q", box)
	}

	typed := send(t, m, runes("boo"))
	if got := typed.projQuery.Value(); got != "boo" {
		t.Fatalf("the query holds %q", got)
	}
	if len(typed.projRows()) != 1 {
		t.Errorf("the filter left %d rows", len(typed.projRows()))
	}
	// Its own input: the task query is untouched, or this would filter the other screen.
	if typed.search.Value() != "" {
		t.Errorf("the task query holds %q", typed.search.Value())
	}
	if v := plain(typed.View()); strings.Contains(v, "AI Sales") {
		t.Errorf("a row the filter excludes is on screen:\n%s", v)
	}

	// enter hands the rows the keyboard and the query stands, dim in its box, with the count
	// beside the title saying what it left.
	kept := send(t, typed, special(tea.KeyEnter))
	if kept.mode == ModeProjSearch || kept.projQuery.Value() != "boo" {
		t.Errorf("enter dropped the filter: mode %v, query %q", kept.mode, kept.projQuery.Value())
	}
	if !strings.Contains(plain(lineWith(t, kept.View(), "boo")), "boo") {
		t.Error("the query left the box when the keys did")
	}
	if head := plain(lineWith(t, kept.View(), "PROJECTS")); !strings.Contains(head, "1 of 4") {
		t.Errorf("the head does not count what the filter left: %q", head)
	}
	// esc from the field is the same door as enter: it hands the rows back and keeps the query,
	// since esc on the list is what clears it.
	if out := send(t, typed, special(tea.KeyEsc)); out.mode == ModeProjSearch ||
		out.projQuery.Value() != "boo" {
		t.Errorf("esc from the field: mode %v, query %q", out.mode, out.projQuery.Value())
	}

	// esc on the list drops the query and shuts the rows: one keystroke back to what `p`
	// opens on.
	opened := send(t, kept, runes("l"))
	if !opened.projOpen[858] {
		t.Fatalf("l did not open the only matching row: %+v", opened.projOpen)
	}
	back := send(t, opened, special(tea.KeyEsc))
	if back.projQuery.Value() != "" {
		t.Errorf("esc kept the query %q", back.projQuery.Value())
	}
	if len(back.projOpen) != 0 || back.projHold != 0 {
		t.Errorf("esc left %+v open with the cursor on %d", back.projOpen, back.projHold)
	}

	// Nothing matching says so rather than leaving a blank screen.
	none := send(t, m, runes("zzz"), special(tea.KeyEnter))
	if v := plain(none.View()); !strings.Contains(v, "no project matches that") {
		t.Errorf("an empty filter result is blank:\n%s", v)
	}

	// The two filters compose: "boo" matches a project that is not mine, so with the toggle
	// back on it leaves nothing.
	if both := send(t, kept, runes("a")); len(both.projRows()) != 0 {
		t.Errorf("the toggle did not narrow the query's own rows: %d", len(both.projRows()))
	}
}

// The field owns the keyboard while it has the keys: a project name can hold the letters the
// tabs are, and a `?`.
func TestProjFilterOwnsTheKeyboard(t *testing.T) {
	m := send(t, projModel(t, 120, 40), runes("a"), runes("i"), runes("top dep"))
	if m.tab != TabProj {
		t.Errorf("typing changed the tab to %v", m.tab)
	}
	if got := m.projQuery.Value(); got != "top dep" {
		t.Errorf("the query holds %q", got)
	}
	if q := send(t, m, runes("?")); q.showHelp {
		t.Error("? toggled the key list from inside the field")
	}
}

// The box is there whether or not it has the keys, and nothing it holds may wrap it onto a
// second line — a wrapped box would shove the whole list down a row.
func TestProjSearchBoxFits(t *testing.T) {
	for _, w := range []int{60, 80, 100, 120, 200} {
		m := send(t, projModel(t, w, 30), runes("i"),
			runes(strings.Repeat("long project name ", 20)))
		if got := len(strings.Split(m.projSearchBox(), "\n")); got != 3 {
			t.Errorf("at %d cells the box is %d lines", w, got)
		}
		for i, line := range strings.Split(m.View(), "\n") {
			if got := lipgloss.Width(line); got > w {
				t.Errorf("at %d cells line %d is %d wide: %q", w, i, got, plain(line))
			}
		}
	}
}

// `/` looks for a person across every project whose people have been read and answers with a
// modal: the names it found, grouped under the project they are on. esc closes it, and nothing in
// the list behind it moved.
func TestProjFindsAPerson(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	const accent = "255;192;0" // #FFC000

	// Two projects with their people read, so the modal has something to group.
	open := send(t, projModel(t, 120, 40), runes("l"))
	open = send(t, open, api.ProjectMembersMsg{ID: 849, Members: sampleMembers()})
	open = send(t, open, runes("j"), runes("l"),
		api.ProjectMembersMsg{ID: 787, Members: []store.Member{
			{Name: "Md. Tasnim Alam", Email: "tasnim@strativ.se"},
			{Name: "Tasnim Mahmood", Email: "tasnim.chowdhury@strativ.se"},
			{Name: "Reaz Abedin", Email: "reaz@strativ.se"},
		}})

	m := send(t, open, runes("/"))
	if m.mode != ModeProjJump {
		t.Fatalf("/ left the mode at %v", m.mode)
	}
	typed := send(t, m, runes("tasnim"))
	if got := typed.projFindHits(); got != 3 {
		t.Fatalf("%d people matched, want 3", got)
	}

	// enter answers with the modal: the query in its head, then a project and the names on it.
	found := send(t, typed, special(tea.KeyEnter))
	if found.mode != ModeProjFound {
		t.Fatalf("enter left the mode at %v", found.mode)
	}
	// The modal itself, not the whole screen: the list behind it still has its own rows, and
	// one of them names the manager this search did not match.
	modal := found.projFoundModal()
	v := plain(modal)
	for _, want := range []string{"tasnim", "3 people on 2 projects",
		"AI Transformation", "Md. Tasnim Alam", "Tasnim Mahmood"} {
		if !strings.Contains(v, want) {
			t.Errorf("the modal is missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "Reaz Abedin") {
		t.Errorf("the modal lists somebody nothing matched:\n%s", v)
	}
	// The name is the accent — it is what was searched for — and it sits under its project.
	if !strings.Contains(lineWith(t, modal, "Tasnim Mahmood"), accent) {
		t.Error("the found name is not in the accent")
	}
	if a, b := rowOf(t, v, "AI Transformation"), rowOf(t, v, "Tasnim Mahmood"); a > b {
		t.Errorf("a project at %d is below its own people at %d", a, b)
	}
	// And the modal is on screen, whole.
	if !strings.Contains(plain(found.View()), "3 people on 2 projects") {
		t.Errorf("the modal is not on screen:\n%s", plain(found.View()))
	}

	// An email is the same question of a different field.
	if byMail := send(t, m, runes("chowdhury@")); byMail.projFindHits() != 1 {
		t.Errorf("an email found %d people", byMail.projFindHits())
	}

	// esc closes it and the list is exactly as it was — nothing opened or moved.
	shut := send(t, found, special(tea.KeyEsc))
	if shut.mode == ModeProjFound {
		t.Error("esc did not close the modal")
	}
	if shut.projHold != open.projHold || len(shut.projOpen) != len(open.projOpen) {
		t.Errorf("the list moved: cursor %d, open %+v", shut.projHold, shut.projOpen)
	}
	if strings.Contains(plain(shut.View()), "3 people on") {
		t.Error("the modal is still on screen")
	}

	// Nothing matching opens no modal: the status line says so and the list stays put.
	none := send(t, m, runes("zzz"), special(tea.KeyEnter))
	if none.mode == ModeProjFound {
		t.Error("a search with no hits opened a modal")
	}
	if !strings.Contains(none.status, "nobody matches") {
		t.Errorf("status = %q", none.status)
	}

	// Nothing read yet, nothing to search: the people arrive when a row is opened.
	fresh := send(t, projModel(t, 120, 40), runes("/"))
	if fresh.mode == ModeProjJump {
		t.Error("/ opened the prompt with nobody read")
	}
	if !strings.Contains(fresh.status, "no people read yet") {
		t.Errorf("status = %q", fresh.status)
	}
}

// The query field matches the people too, once they have been read: "which projects is Tasnim
// on" is the same question as "who runs Coeo", asked of a different field.
func TestProjFilterMatchesPeople(t *testing.T) {
	m := send(t, projModel(t, 120, 40), runes("l"))
	m = send(t, m, api.ProjectMembersMsg{ID: 849, Members: sampleMembers()})

	typed := send(t, m, runes("i"), runes("benjamin"))
	rows := typed.projRows()
	if len(rows) != 1 || rows[0].ID != 849 {
		t.Fatalf("the filter left %+v", rows)
	}
	// An email works the same way.
	if byMail := send(t, m, runes("i"), runes("ashik.rafat@")); len(byMail.projRows()) != 1 {
		t.Errorf("an email left %d rows", len(byMail.projRows()))
	}
	// A project whose people have not been read is matched on what it does have.
	if unread := send(t, m, runes("i"), runes("benjamin")); len(unread.projRows()) != 1 {
		t.Errorf("a name reached a project whose people are not in hand")
	}
}

// It opens on your own projects, and `a` toggles: 89 open projects with nine of them yours is a
// screen that answers "what does the office run" before "what am I on".
func TestProjOpensOnMineAndTogglesWithA(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	const accent = "255;192;0" // #FFC000

	m := projModel(t, 120, 34)
	if !m.projMine {
		t.Fatal("the tab did not open on your own projects")
	}
	mine := plain(m.View())
	for _, p := range sampleProjects() {
		on := strings.Contains(mine, string([]rune(p.Name)[:min(8, len([]rune(p.Name)))]))
		if on != p.Mine {
			t.Errorf("%q on screen = %v, want %v", p.Name, on, p.Mine)
		}
	}
	// The count says which of the two is on screen, since the label names the action.
	if !strings.Contains(mine, "2 of 4") {
		t.Errorf("the head does not count what the toggle left:\n%s", mine)
	}

	// The label carries its key in the accent either way, and takes the accent whole while
	// all of them are on screen: the frame-and-fill idiom the request lines use.
	head := lineWith(t, m.View(), "ll projects") // hinted() splits the a into its own span
	if !strings.Contains(head, accent) {
		t.Errorf("the toggle does not advertise its key: %q", head)
	}

	all := send(t, m, runes("a"))
	if all.projMine {
		t.Fatal("a did not toggle")
	}
	v := plain(all.View())
	for _, p := range sampleProjects() {
		if !strings.Contains(v, string([]rune(p.Name)[:min(8, len([]rune(p.Name)))])) {
			t.Errorf("%q is missing from the whole list:\n%s", p.Name, v)
		}
	}
	if !strings.Contains(v, "4 open projects") {
		t.Errorf("the head still counts a filter:\n%s", v)
	}
	// On, the whole label is accent — and it is still one label, never "my projects a": the
	// key has to sit inside the word it is picked out of.
	if strings.Contains(v, "my projects") {
		t.Errorf("the toggle renamed itself:\n%s", v)
	}

	// It resets the cursor and shuts the rows: the row under the cursor is a different project
	// on the other side of the toggle.
	opened := send(t, all, runes("j"), runes("l"))
	if len(opened.projOpen) == 0 || opened.projHold == 0 {
		t.Fatalf("nothing to reset: open %+v, cursor %d", opened.projOpen, opened.projHold)
	}
	back := send(t, opened, runes("a"))
	if back.projHold != 0 || len(back.projOpen) != 0 {
		t.Errorf("the toggle kept the cursor on %d with %+v open", back.projHold, back.projOpen)
	}

	// A key owner on none of them is told which key fills the screen.
	none := send(t, m, api.ProjectsMsg{Projects: []store.Project{
		{ID: 1, Name: "Somebody else's", Tasks: 3}}})
	if v := plain(none.View()); !strings.Contains(v, "none of these are yours") {
		t.Errorf("an empty mine list is blank:\n%s", v)
	}
}

// The list is cached on disk: a project's teams and who runs it do not change between two
// openings of a terminal, so the tab shows the cache and only an empty one fetches. R re-reads.
func TestProjReadsFromTheCache(t *testing.T) {
	m := send(t, New(), tea.WindowSizeMsg{Width: 120, Height: 34},
		store.KeyMsg{Key: "k", DB: "db"}, store.ProjectsLoadedMsg{Projects: sampleProjects()})
	m.login = "user@example.com"

	shown, cmd := sendCmd(t, m, runes("p"))
	if cmd != nil || shown.projLoading {
		t.Error("the cache was on hand and the tab still fetched")
	}
	if !strings.Contains(plain(shown.View()), "AI Transformation") {
		t.Errorf("the cache is not on screen:\n%s", plain(shown.View()))
	}
	// R goes back to the ERP, and what comes back is written to the cache.
	fresh, cmd := sendCmd(t, shown, runes("R"))
	if cmd == nil || !fresh.projLoading {
		t.Fatal("R did not re-read")
	}
	_, saved := sendCmd(t, fresh, api.ProjectsMsg{Projects: sampleProjects()})
	if saved == nil {
		t.Error("the answer was not written to the cache")
	}

	// The people are cached with the project, so a restart does not ask for them again: a
	// team's membership does not change between two openings of a terminal, which is the whole
	// reason this list is on disk.
	withPeople, wrote := sendCmd(t, send(t, shown, runes("l")),
		api.ProjectMembersMsg{ID: 849, Members: sampleMembers()})
	if wrote == nil {
		t.Fatal("the people were not written to the cache")
	}
	var kept []store.Project
	for _, p := range withPeople.projs {
		if p.ID == 849 {
			kept = append(kept, p)
		}
	}
	if len(kept) != 1 || len(kept[0].People) != len(sampleMembers()) {
		t.Fatalf("the people are not on the cached record: %+v", kept)
	}

	// A fresh start off that cache opens the row with nothing in flight.
	back := send(t, New(), tea.WindowSizeMsg{Width: 120, Height: 34},
		store.KeyMsg{Key: "k", DB: "db"},
		store.ProjectsLoadedMsg{Projects: withPeople.projs})
	back.login = "user@example.com"
	again, cmd := sendCmd(t, send(t, back, runes("p")), runes("l"))
	if cmd != nil || again.projPulling[849] {
		t.Error("the people were read again after a restart")
	}
	if v := plain(again.View()); !strings.Contains(v, "tasnim@strativ.se") {
		t.Errorf("the cached people are not on screen:\n%s", v)
	}

	// A refresh keeps them while the member ids are unchanged, and drops them when they move:
	// new ids mean the names beside them are somebody else's.
	moved := sampleProjects()
	for i := range moved {
		if moved[i].ID == 849 {
			moved[i].Members = []int{999}
		}
	}
	after := send(t, again, api.ProjectsMsg{Projects: sampleProjects()})
	if len(after.projMembers[849]) != len(sampleMembers()) {
		t.Error("a refresh threw away people whose ids had not changed")
	}
	if changed := send(t, again, api.ProjectsMsg{Projects: moved}); len(changed.projMembers[849]) != 0 {
		t.Error("a refresh kept people whose team had changed")
	}

	// A cache that will not parse is not an error on screen: the tab fetches instead.
	broken := send(t, m, store.ProjectsLoadedMsg{Err: errors.New("bad json")})
	if broken.err != nil {
		t.Errorf("a broken cache put %v on screen", broken.err)
	}
}

// The tab is reachable by its letter and by its position, and both are matched where every
// other tab key is — so a typing mode still protects them.
func TestProjTabKeyAndDigit(t *testing.T) {
	m := projModel(t, 120, 34)
	if away := send(t, m, runes("t")); away.tab != TabTasks {
		t.Errorf("t did not leave the projects: %v", away.tab)
	}
	back := send(t, send(t, m, runes("t")), runes("7"))
	if back.tab != TabProj {
		t.Errorf("7 did not reach the projects: %v", back.tab)
	}
	// The query field takes the letter instead: p is typeable there.
	typed := send(t, send(t, m, runes("t")), runes("i"), runes("p"))
	if typed.tab != TabTasks || typed.search.Value() != "p" {
		t.Errorf("p in the query box switched tabs: tab %v, query %q",
			typed.tab, typed.search.Value())
	}
}

// The empty state says how to fill it rather than leaving a blank screen, and names the key
// that does — off the binding, so a rebind follows.
func TestProjEmptyStateNamesTheKey(t *testing.T) {
	m := send(t, New(), tea.WindowSizeMsg{Width: 100, Height: 30},
		store.KeyMsg{Key: "k", DB: "db"})
	m.login = "user@example.com"
	empty := send(t, m, runes("p"), api.ProjectsMsg{})
	v := plain(empty.View())
	if !strings.Contains(v, "no projects yet") {
		t.Errorf("the empty state is missing:\n%s", v)
	}
	// The header counts nothing: "0 open projects" beside an empty body says it twice.
	if head := plain(strings.Join(empty.projHead(), "\n")); strings.Contains(head, "0 open") {
		t.Errorf("an empty list still counts itself in the head: %q", head)
	}
}
