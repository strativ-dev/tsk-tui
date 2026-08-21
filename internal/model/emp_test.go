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

func sampleEmps() []store.Employee {
	return []store.Employee{
		{ID: 121, Name: "Abdul Alim Shohan", Job: "Software Engineer - L3",
			Email: "abdul.shohan@strativ.se"},
		{ID: 162, Name: "Abdullah Zayed", Job: "Software Engineer - L4",
			Email: "abdullah.zayed@strativ.se", Phone: "+46 72 130 50 43"},
		{ID: 141, Name: "Ariful Islam", Job: "Security Guard",
			Email: "arif75278u@gmail.com", Phone: "+46 72 130 50 43"},
		{ID: 57, Name: "Jonna Persson", Job: "", Email: "jonna@strativ.se"},
	}
}

func empModel(t *testing.T, width, height int) Model {
	t.Helper()
	m := send(t, New(), tea.WindowSizeMsg{Width: width, Height: height},
		store.KeyMsg{Key: "k", DB: "db"})
	m.login = "user@example.com" // the sync that carries the email has already landed
	return send(t, m, runes("e"), api.EmployeesMsg{Employees: sampleEmps()})
}

// e opens the directory, reads it once, and draws a card per employee in the ERP's own
// format: the name, what they do, and the two ways to reach them.
func TestEmpTabOpensAndReads(t *testing.T) {
	m := send(t, New(), tea.WindowSizeMsg{Width: 100, Height: 30},
		store.KeyMsg{Key: "k", DB: "db"})
	m.login = "user@example.com"

	m, cmd := sendCmd(t, m, runes("e"))
	if m.tab != TabEmp {
		t.Fatalf("tab = %v, want TabEmp", m.tab)
	}
	if cmd == nil || !m.empLoading {
		t.Fatal("e did not start reading the directory")
	}
	if v := plain(m.View()); !strings.Contains(v, "reading the directory…") {
		t.Errorf("an empty directory mid-read does not say so:\n%s", v)
	}

	m = send(t, m, api.EmployeesMsg{Employees: sampleEmps()})
	if m.empLoading || len(m.emps) != 4 {
		t.Fatalf("loading = %v, %d employees", m.empLoading, len(m.emps))
	}
	v := plain(m.View())
	for _, want := range []string{"Abdullah Zayed", "Software Engineer - L4",
		"Ariful Islam", "Security Guard", "4 employees"} {
		if !strings.Contains(v, want) {
			t.Errorf("the list is missing %q:\n%s", want, v)
		}
	}
	// A job title the ERP left empty still draws its line, or the card loses a row. Filtered
	// down to that one card, since four of them do not fit a 30-row terminal.
	jonna := plain(send(t, m, runes("/"), runes("Jonna")).View())
	if !strings.Contains(jonna, "Jonna Persson") || !strings.Contains(jonna, "—") {
		t.Errorf("an employee with no job title has no line for it:\n%s", jonna)
	}

	// The cache answers the second open: the list is the same every day, so it is read once.
	back := send(t, m, runes("t"))
	again, cmd := sendCmd(t, back, runes("e"))
	if cmd != nil || again.empLoading {
		t.Error("e read the directory a second time — the cache is what it shows")
	}
	// r is how it is re-read, and the cards stay up while it is out.
	fresh, cmd := sendCmd(t, again, runes("R"))
	if cmd == nil || !fresh.empLoading {
		t.Fatal("r did not re-read the directory")
	}
	if !strings.Contains(plain(fresh.View()), "Abdullah Zayed") {
		t.Error("the cards were blanked while it re-read")
	}
}

// The answer is written to disk, so the next run opens on the cache rather than on a fetch.
func TestEmpAnswerIsCached(t *testing.T) {
	m := empModel(t, 100, 30)
	_, cmd := sendCmd(t, m, api.EmployeesMsg{Employees: sampleEmps()})
	if cmd == nil {
		t.Fatal("the answer was not saved")
	}
	// And a failed re-read keeps what was on screen: a directory that was there must not be
	// emptied by a dropped connection.
	failed := send(t, m, api.EmployeesMsg{Err: errors.New("odoo: gone")})
	if len(failed.emps) != 4 || failed.err == nil {
		t.Errorf("%d employees left after a failed re-read", len(failed.emps))
	}
	if !strings.Contains(failed.status, "directory is unchanged") {
		t.Errorf("status = %q", failed.status)
	}
}

// The cache is read off disk at launch, before anything asks the ERP.
func TestEmpCacheLoadsAtLaunch(t *testing.T) {
	m := send(t, New(), store.EmployeesLoadedMsg{Employees: sampleEmps()})
	if len(m.emps) != 4 {
		t.Fatalf("%d employees from the cache", len(m.emps))
	}
	shown, cmd := sendCmd(t, send(t, m, tea.WindowSizeMsg{Width: 100, Height: 30},
		store.KeyMsg{Key: "k", DB: "db"}), runes("e"))
	if cmd != nil || shown.empLoading {
		t.Error("the tab fetched even though the cache had answered")
	}
}

// The query filters on the whole card — a name, a title, an email or a phone — since those
// are the same question asked of different fields.
func TestEmpFilterMatchesAnyField(t *testing.T) {
	m := empModel(t, 100, 30)
	for _, c := range []struct {
		query string
		want  string
		n     int
	}{
		{"guard", "Ariful Islam", 1},
		{"zayed", "Abdullah Zayed", 1},
		{"strativ.se", "", 3},
		{"130 50", "", 2},
		{"L3", "Abdul Alim Shohan", 1},
	} {
		got := send(t, m, runes("/"), runes(c.query))
		if len(got.empRows()) != c.n {
			t.Errorf("%q matched %d, want %d", c.query, len(got.empRows()), c.n)
		}
		if c.want != "" && !strings.Contains(plain(got.View()), c.want) {
			t.Errorf("%q did not show %s", c.query, c.want)
		}
	}

	// The count says what is left of what there is, and the whole number when nothing is
	// filtered.
	if v := plain(m.View()); !strings.Contains(v, "4 employees") {
		t.Errorf("the head does not count the directory:\n%s", v)
	}
	one := send(t, m, runes("/"), runes("guard"))
	if v := plain(one.View()); !strings.Contains(v, "1 of 4") {
		t.Errorf("the head does not count the filter:\n%s", v)
	}
	// Nothing matching says so rather than drawing an empty screen.
	if v := plain(send(t, m, runes("/"), runes("zzz")).View()); !strings.Contains(v, "nobody matches") {
		t.Errorf("an empty filter result does not say so:\n%s", v)
	}
	// enter hands the keyboard back to the rows with the filter standing; esc drops it.
	kept := send(t, one, special(tea.KeyEnter))
	if kept.mode == ModeEmpSearch || len(kept.empRows()) != 1 {
		t.Errorf("enter left the prompt: mode = %v, %d rows", kept.mode, len(kept.empRows()))
	}
	dropped := send(t, one, special(tea.KeyEsc))
	if dropped.mode == ModeEmpSearch || len(dropped.empRows()) != 4 {
		t.Errorf("esc did not clear the filter: mode = %v, %d rows",
			dropped.mode, len(dropped.empRows()))
	}
}

// The query field owns the keyboard while it has the cursor: a name to search for can hold
// the letters the tabs are.
func TestEmpSearchOwnsTheKeyboard(t *testing.T) {
	m := send(t, empModel(t, 100, 30), runes("/"), runes("tasnim"))
	if m.tab != TabEmp || m.empQuery.Value() != "tasnim" {
		t.Errorf("tab = %v, query = %q", m.tab, m.empQuery.Value())
	}
	if !strings.Contains(plain(m.View()), "-- SEARCH --") {
		t.Errorf("the mode line does not name the field:\n%s", plain(m.View()))
	}
}

// The tab bar names it with the e picked out, and gives up the words rather than wrapping on a
// terminal too narrow for five of them.
func TestEmpTabInTheBar(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	m := empModel(t, 100, 30)
	bar := strings.Split(m.View(), "\n")[0]
	if !strings.Contains(plain(bar), "employee") || !strings.Contains(plain(bar), "⁵") {
		t.Errorf("the bar does not carry the tab:\n%s", plain(bar))
	}
	// Active, so it is the pill: dark ink on the accent.
	if !strings.Contains(bar, "48;2;255;192;0") {
		t.Errorf("the open tab is not the pill:\n%q", bar)
	}
	// 5 reaches it as well as e.
	if byDigit := send(t, send(t, m, runes("t")), runes("5")); byDigit.tab != TabEmp {
		t.Error("5 does not open the directory")
	}
	narrow := empModel(t, 60, 20)
	if got := lipgloss.Width(strings.Split(narrow.View(), "\n")[0]); got > 60 {
		t.Errorf("the bar is %d cells on a 60-cell terminal", got)
	}
}

// Nothing on this screen may exceed the terminal, cards and query box included.
func TestEmpFitsTheTerminal(t *testing.T) {
	for _, size := range [][2]int{{60, 20}, {80, 24}, {100, 30}, {200, 40}} {
		w, h := size[0], size[1]
		for _, m := range []Model{
			empModel(t, w, h),
			send(t, empModel(t, w, h), runes("/"), runes("engineer")),
			send(t, empModel(t, w, h), runes("l"), api.EmployeeMsg{ID: 121,
				Detail: sampleDetail()}),
			send(t, empModel(t, w, h), runes("G")),
		} {
			lines := strings.Split(m.View(), "\n")
			if len(lines) > h {
				t.Errorf("%dx%d: %d lines", w, h, len(lines))
			}
			for i, l := range lines {
				if got := lipgloss.Width(l); got > w {
					t.Errorf("%dx%d: line %d is %d cells wide: %q", w, h, i, got, l)
				}
			}
		}
	}
}

// It scrolls: g and G reach the ends of the list, and the window follows the held card.
func TestEmpScrolls(t *testing.T) {
	m := empModel(t, 100, 16) // short enough that four cards cannot all fit
	if got := plain(m.View()); !strings.Contains(got, "Abdul Alim Shohan") {
		t.Errorf("the list does not open at the top:\n%s", got)
	}
	end := send(t, m, runes("G"))
	if end.empHold != 3 {
		t.Errorf("G held card %d, want the last", end.empHold)
	}
	if got := plain(end.View()); !strings.Contains(got, "Jonna Persson") {
		t.Errorf("G did not bring the last card into view:\n%s", got)
	}
	if top := send(t, end, runes("g")); top.empHold != 0 {
		t.Errorf("g held card %d", top.empHold)
	}
	// Clamped at both ends rather than wrapping.
	if up := send(t, m, runes("k")); up.empHold != 0 {
		t.Errorf("k walked past the first card to %d", up.empHold)
	}
	if down := send(t, end, runes("j")); down.empHold != 3 {
		t.Errorf("j walked past the last card to %d", down.empHold)
	}
}

func sampleDetail() store.EmployeeDetail {
	return store.EmployeeDetail{
		ID: 121, Email: "abdul.shohan@strativ.se", Phone: "+46 72 130 50 43",
		Department: "Technical", TeamLead: "Saqibur Rahman", Coach: "Milon Mahato",
		TimeOff: "Saqibur Rahman", StackManager: "K.M. Jiaul Islam Jibon",
		Location: "Bangladesh", Managers: []string{"Reaz Abedin"},
		Projects: []string{"Learn and Grow", "LumberScan",
			"Value-Driven Engagement, Internal Meetings & Tasks"},
	}
}

// A row is two fixed columns: the name, then the job title in a chip. Fixed, so every chip
// starts on the same cell — right-aligned instead, each title began somewhere else.
func TestEmpRowColumnsLineUp(t *testing.T) {
	m := empModel(t, 100, 30)
	at := -1
	for _, l := range strings.Split(plain(m.View()), "\n") {
		before, _, found := strings.Cut(l, "│")
		if !found {
			continue
		}
		// Cells, not bytes: the caret and the rules are multi-byte, so an index would count
		// them two or three times over.
		i := lipgloss.Width(before)
		if at >= 0 && i != at {
			t.Errorf("a chip starts at cell %d and another at %d:\n%s", at, i, plain(m.View()))
		}
		at = i
	}
	if at < 0 {
		t.Fatalf("no job chips on screen:\n%s", plain(m.View()))
	}
	// The name column is what puts them there: the caret, then 30 cells.
	if want := gutter + 2 + empNameCells; at != want {
		t.Errorf("the chips start at cell %d, want %d", at, want)
	}
	// The two columns are what the row is, so nothing else may push them off a narrow screen.
	for _, w := range []int{60, 70, 80, 120} {
		narrow := empModel(t, w, 20)
		name, job := narrow.empColumns()
		if name+job > w-gutter-2 {
			t.Errorf("at %d cells the columns want %d+%d", w, name, job)
		}
		for i, l := range strings.Split(narrow.View(), "\n") {
			if got := lipgloss.Width(l); got > w {
				t.Errorf("at %d cells line %d is %d wide: %q", w, i, got, l)
			}
		}
	}
}

// The row under the cursor takes the accent whole — the name and the job chip — since a name in
// the accent beside a chip in the tag's teal reads as two rows overlapping.
func TestEmpFocusedRowIsAccentWhole(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	const (
		accent = "255;192;0" // #FFC000
		teal   = "1;185;174" // #01B9AE, what a chip carries when it is not the one selected
	)
	m := empModel(t, 100, 30)
	rows := func(v string) (held, other string) {
		for _, l := range strings.Split(v, "\n") {
			switch {
			case strings.Contains(plain(l), "Abdul Alim Shohan"):
				held = l
			case strings.Contains(plain(l), "Abdullah Zayed"):
				other = l
			}
		}
		return held, other
	}

	held, other := rows(m.View())
	if !strings.Contains(held, accent) {
		t.Errorf("the held row is not accented:\n%q", held)
	}
	// Its chip too, not only its name: the accent has to be on the half of the line the title
	// is in.
	_, chip, _ := strings.Cut(held, "Shohan")
	if !strings.Contains(chip, accent) {
		t.Errorf("the held row's job chip is not accented:\n%q", chip)
	}
	if strings.Contains(held, teal) {
		t.Errorf("the held row keeps the tag's teal:\n%q", held)
	}
	// And the row below it keeps the quiet pair.
	if strings.Contains(other, accent) || !strings.Contains(other, teal) {
		t.Errorf("an unheld row is accented:\n%q", other)
	}
	// Walking moves both marks together.
	next := send(t, m, runes("j"))
	heldNext, prev := rows(next.View())
	if strings.Contains(heldNext, accent) {
		t.Errorf("the cursor did not leave the first row:\n%q", heldNext)
	}
	if !strings.Contains(prev, accent) || strings.Contains(prev, teal) {
		t.Errorf("j did not take the accent with it:\n%q", prev)
	}
}

// l opens the row under the cursor and reads it once; h closes it again.
func TestEmpRowOpensItsDetail(t *testing.T) {
	m := empModel(t, 100, 32)

	open, cmd := sendCmd(t, m, runes("l"))
	if cmd == nil || !open.empPulling[121] {
		t.Fatal("l did not read the employee")
	}
	if !open.empOpen[121] || !open.busy() {
		t.Errorf("open = %v, busy = %v", open.empOpen[121], open.busy())
	}
	if v := plain(open.View()); !strings.Contains(v, "reading their details…") {
		t.Errorf("an open row with nothing in it yet does not say so:\n%s", v)
	}

	done := send(t, open, api.EmployeeMsg{ID: 121, Detail: sampleDetail()})
	if done.empPulling[121] {
		t.Error("the in-flight flag survived the answer")
	}
	v := plain(done.View())
	for _, want := range []string{"abdul.shohan@strativ.se", "+46 72 130 50 43",
		"Technical", "Saqibur Rahman", "Reaz Abedin", "K.M. Jiaul Islam Jibon",
		"Milon Mahato", "Bangladesh", "Learn and Grow", "LumberScan",
		"email", "department", "team lead", "project mgr", "time off", "stack mgr",
		"projects"} {
		if !strings.Contains(v, want) {
			t.Errorf("the open row is missing %q:\n%s", want, v)
		}
	}
	// The caret says which way the row goes.
	if !strings.Contains(v, "▾") {
		t.Errorf("an open row keeps the closed caret:\n%s", v)
	}

	// Read once: opening it again asks nothing.
	closed := send(t, done, runes("h"))
	if closed.empOpen[121] {
		t.Error("h did not close the row")
	}
	if again, cmd := sendCmd(t, closed, runes("l")); cmd != nil || again.empPulling[121] {
		t.Error("l read the same employee twice")
	}

	// A refusal says so and leaves the row open to try again.
	failed := send(t, open, api.EmployeeMsg{ID: 121, Err: errors.New("odoo: denied")})
	if failed.empPulling[121] || failed.err == nil {
		t.Errorf("a refused read was swallowed: %v", failed.err)
	}
	if !strings.Contains(failed.status, "could not read that employee") {
		t.Errorf("status = %q", failed.status)
	}
}

// The projects are pills — filled, so a name that reads as a phrase is one object at a glance.
func TestEmpProjectsArePills(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	m := send(t, empModel(t, 100, 32), runes("l"),
		api.EmployeeMsg{ID: 121, Detail: sampleDetail()})
	line := lineWith(t, m.View(), "Learn and Grow")
	// The fill, and a cell of it either side of the name: that padding is the lozenge's shape.
	if !strings.Contains(line, "48;2;32;32;42") {
		t.Errorf("the projects are not filled:\n%q", line)
	}
	// Each pill's own padding plus the cell between them: touching, a run of fills reads as one
	// long pill.
	if !strings.Contains(plain(line), " Learn and Grow   LumberScan ") {
		t.Errorf("the pills do not sit side by side:\n%q", plain(line))
	}
	// And no rules: a pill is a fill, not a name between two bars.
	if strings.Contains(plain(line), "│") {
		t.Errorf("the pills still carry chip rules:\n%q", plain(line))
	}
}

// The filter prompt is only on screen while it is being typed; the header carries the query
// once it has closed, or a short list has no visible reason.
func TestEmpFilterPromptOpensAndCloses(t *testing.T) {
	m := empModel(t, 100, 30)
	if v := plain(m.View()); strings.Contains(v, "search:") {
		t.Errorf("the prompt is on screen before / was pressed:\n%s", v)
	}
	open := send(t, m, runes("/"), runes("guard"))
	if open.mode != ModeEmpSearch {
		t.Fatalf("mode = %v", open.mode)
	}
	if v := plain(open.View()); !strings.Contains(v, "search: guard") {
		t.Errorf("the prompt does not show what is typed:\n%s", v)
	}
	// enter closes the prompt and keeps the filter, which the head then carries — a short list
	// with nothing on screen to explain it reads as a directory missing people.
	kept := plain(send(t, open, special(tea.KeyEnter)).View())
	if strings.Contains(kept, "search: guard") {
		t.Errorf("the prompt stayed open:\n%s", kept)
	}
	if !strings.Contains(kept, "/guard") || !strings.Contains(kept, "1 of 4") {
		t.Errorf("the filter was lost with the prompt:\n%s", kept)
	}
	// esc also shuts whatever was open: the list comes back, and a screenful of detail behind a
	// cleared filter is not the list coming back.
	opened := send(t, m, runes("l"), api.EmployeeMsg{ID: 121, Detail: sampleDetail()})
	if !opened.empOpen[121] {
		t.Fatal("the row did not open")
	}
	collapsed := send(t, opened, runes("/"), runes("guard"), special(tea.KeyEsc))
	if collapsed.empOpen[121] || len(collapsed.empRows()) != 4 {
		t.Errorf("esc left %d rows with 121 open = %v",
			len(collapsed.empRows()), collapsed.empOpen[121])
	}
	// And esc does it from the list too, not only from the prompt: a filtered list with rows
	// open takes one keystroke to put back, whichever half of the screen has the keyboard.
	fromList := send(t, opened, runes("/"), runes("guard"), special(tea.KeyEnter),
		special(tea.KeyEsc))
	if fromList.empOpen[121] || len(fromList.empRows()) != 4 ||
		fromList.empQuery.Value() != "" {
		t.Errorf("esc from the list left %d rows, 121 open = %v, query %q",
			len(fromList.empRows()), fromList.empOpen[121], fromList.empQuery.Value())
	}
	// ctrl+u is left alone: it clears the task query everywhere else, so it is not given a
	// second meaning here.
	if got := send(t, opened, special(tea.KeyCtrlU)); !got.empOpen[121] {
		t.Error("ctrl+u took a second meaning on this screen")
	}

	// esc drops it: the whole list comes back and the head says nothing about a filter.
	shut := send(t, open, special(tea.KeyEsc))
	v := plain(shut.View())
	if strings.Contains(v, "search:") || strings.Contains(v, "/guard") {
		t.Errorf("esc left the filter behind:\n%s", v)
	}
	if len(shut.empRows()) != 4 || shut.empQuery.Value() != "" {
		t.Errorf("esc kept %d rows and %q", len(shut.empRows()), shut.empQuery.Value())
	}
}
