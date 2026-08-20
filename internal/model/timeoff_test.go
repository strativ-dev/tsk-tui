package model

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/tasnimAlam/tsk/internal/api"
	"github.com/tasnimAlam/tsk/internal/parse"
	"github.com/tasnimAlam/tsk/internal/store"
	"github.com/tasnimAlam/tsk/internal/theme"
)

// o opens the calendar and reads the year; t comes back, and the tab bar says which
// screen is up.
func TestTimeTabOpensAndReads(t *testing.T) {
	m := send(t, New(), tea.WindowSizeMsg{Width: 100, Height: 30},
		store.KeyMsg{Key: "k", DB: "db"})
	m.login = "user@example.com" // the sync that carries the email has already landed

	m, cmd := sendCmd(t, m, runes("o"))
	if m.tab != TabTime {
		t.Fatalf("tab = %v, want TabTime", m.tab)
	}
	if cmd == nil || !m.timeLoading {
		t.Fatal("o did not start reading the year")
	}
	if v := m.View(); !strings.Contains(plain(v), "reading this year's time off…") {
		t.Errorf("an empty calendar mid-read does not say so:\n%s", v)
	}

	m = send(t, m, api.TimeOffMsg{Year: 2026, Kinds: sampleKinds(),
		Leaves: sampleLeaves(), Holidays: sampleHolidays()})
	if m.timeLoading || m.timeYear != 2026 {
		t.Errorf("loading = %v, year = %d", m.timeLoading, m.timeYear)
	}
	v := plain(m.View())
	for _, want := range []string{"TIME OFF 2026", "sick Time Off", "paternity Time Off",
		"DAYS AVAILABLE", "wk   M    T    W    T    F    S    S", "days taken"} {
		if !strings.Contains(v, want) {
			t.Errorf("the calendar is missing %q:\n%s", want, v)
		}
	}
	// The balances are on the cards under their names, at double width: 9 sick, 8.5 annual.
	if figures := lineWith(t, v, wide("8.5")); !strings.Contains(figures, wide("9")) {
		t.Errorf("the balance row is missing a figure:\n%s", figures)
	}
	// Both ends of the year are reachable — the window follows whichever month is held.
	if top := plain(send(t, m, runes("g")).View()); !strings.Contains(top, "Jan") {
		t.Errorf("g does not reach January:\n%s", top)
	}
	if end := plain(send(t, m, runes("G")).View()); !strings.Contains(end, "Dec") {
		t.Errorf("G does not reach December:\n%s", end)
	}

	// Back to the tasks list, and the year is not read again on the way in.
	back := send(t, m, runes("t"))
	if back.tab != TabTasks {
		t.Errorf("t did not return to the task list")
	}
	again, cmd := sendCmd(t, back, runes("o"))
	if cmd != nil || again.timeLoading {
		t.Error("o read the year a second time — one year in hand at a time")
	}
}

// The year on screen stays up while a re-read is in flight, with the loader beside its
// title: a calendar blanked mid-read loses the answer it already had.
func TestTimeRefreshKeepsTheYearUp(t *testing.T) {
	m := timeModel(t, 100, 30)
	m.login = "user@example.com"

	m, cmd := sendCmd(t, m, runes("r"))
	if cmd == nil || !m.timeLoading {
		t.Fatal("r did not re-read the year")
	}
	v := plain(m.View())
	if !strings.Contains(v, "TIME OFF 2026") || !strings.Contains(v, "wk   M") {
		t.Errorf("the year was blanked while it re-read:\n%s", v)
	}
}

// enter lists the month's own time off in a modal: a line a day, with the leave type and what
// the request was for. esc closes it, and it destroys nothing so nothing else needs a key.
func TestMonthLeavesModal(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	m := send(t, timeModel(t, 120, 40), runes("g")) // January, where the fixture's leave is
	open := send(t, m, special(tea.KeyEnter))
	if open.mode != ModeLeaves {
		t.Fatalf("enter did not open the list: mode = %v", open.mode)
	}
	v := plain(open.View())
	// One line per day of a range, since that is what the calendar above reads as days.
	for _, want := range []string{"TIME OFF JAN 2026", "3 days",
		"21 Jan (Wed)", "22 Jan (Thu)", "23 Jan (Fri)", "casual", "Family errand"} {
		if !strings.Contains(v, want) {
			t.Errorf("the modal is missing %q:\n%s", want, v)
		}
	}
	// The type reads in its own colour, the one its days are drawn in behind the modal.
	if want := theme.LeaveInk(theme.LeaveColor("Casual Time Off")).Render("casual "); !strings.Contains(open.View(), want) {
		t.Error("the leave type is not in its own colour")
	}
	if !strings.Contains(plain(open.footer()), "-- TIME OFF --") {
		t.Errorf("the mode line does not name the modal:\n%s", plain(open.footer()))
	}

	// A half day says which half; a request still waiting on an approver says so.
	feb := send(t, m, runes("l"), special(tea.KeyEnter))
	if got := plain(feb.View()); !strings.Contains(got, "18 Feb (Wed, morning)") {
		t.Errorf("a half day does not say which half:\n%s", got)
	}
	apr := send(t, m, runes("l"), runes("l"), runes("l"), special(tea.KeyEnter))
	if got := plain(apr.View()); !strings.Contains(got, "pending") {
		t.Errorf("a request waiting on approval is not marked:\n%s", got)
	}

	// A month with nothing off says so rather than drawing an empty box.
	if got := plain(send(t, send(t, m, runes("G")), special(tea.KeyEnter)).View()); !strings.Contains(got,
		"nothing booked off this month") {
		t.Errorf("an empty month does not say so:\n%s", got)
	}

	// esc closes it, and the calendar is where it was.
	back := send(t, open, special(tea.KeyEsc))
	if back.mode == ModeLeaves {
		t.Error("esc did not close the list")
	}
	if back.timeMonth() != 0 {
		t.Errorf("closing it moved the calendar to month %d", back.timeMonth())
	}
	// The modal owns the keyboard while it is up: h and l do not walk the months behind it.
	if held := send(t, open, runes("l")); held.timeMonth() != 0 || held.mode != ModeLeaves {
		t.Errorf("l walked to month %d behind the modal", held.timeMonth())
	}
}

// The list follows the filter, so it and the calendar under it always say the same thing.
func TestMonthLeavesFollowsTheFilter(t *testing.T) {
	m := send(t, timeModel(t, 120, 40), runes("g"), runes("s")) // sick only
	// The modal itself, not the whole screen: the balance cards behind it name every type.
	got := plain(send(t, m, special(tea.KeyEnter)).leavesModal())
	if strings.Contains(got, "casual") {
		t.Errorf("the list ignored the sick filter:\n%s", got)
	}
	if !strings.Contains(got, "sick only") {
		t.Errorf("the head does not name the filter:\n%s", got)
	}
	// January holds only casual leave in the fixture, so with the sick filter on it is empty.
	if !strings.Contains(got, "nothing booked off this month") {
		t.Errorf("a filtered month with nothing in it does not say so:\n%s", got)
	}
}

// A letter filters by the leave type it starts, the same letter clears it, and so does
// esc. The types come from the ERP, so nothing here is hardcoded.
func TestTimeFilterFollowsTheTypes(t *testing.T) {
	m := timeModel(t, 100, 30)

	casual := send(t, m, runes("c"))
	if casual.timeFilter != 4 {
		t.Fatalf("filter = %d, want the casual type id 4", casual.timeFilter)
	}
	if v := plain(casual.View()); !strings.Contains(v, "-- CASUAL TIME OFF --") {
		t.Errorf("the mode line does not name the filter:\n%s", v)
	}
	// 3 casual days out of the sample's 7.5, so the count follows the filter too.
	if v := plain(casual.View()); !strings.Contains(v, "3 casual days taken") {
		t.Errorf("the count does not follow the filter:\n%s", v)
	}
	if again := send(t, casual, runes("c")); again.timeFilter != 0 {
		t.Errorf("the same letter twice did not clear the filter")
	}
	if esc := send(t, casual, special(tea.KeyEsc)); esc.timeFilter != 0 {
		t.Errorf("esc did not clear the filter")
	}

	// A letter no type starts is not a filter, and does not clear one either.
	if z := send(t, casual, runes("z")); z.timeFilter != 4 {
		t.Errorf("z cleared the filter; only a type's own letter should")
	}
}

// A key the calendar shares with the rest of the app keeps its own meaning: t is the tasks
// tab, not the filter for a type that happens to start with it.
func TestTabKeysBeatTheFilters(t *testing.T) {
	m := timeModel(t, 100, 30)
	m.timeKinds = append(m.timeKinds, api.LeaveKind{ID: 99, Name: "Toil Time Off"})

	if got := send(t, m, runes("t")); got.tab != TabTasks || got.timeFilter != 0 {
		t.Errorf("tab = %v, filter = %d — t is the tasks tab everywhere", got.tab, got.timeFilter)
	}
	if got := send(t, m, runes("d")); got.tab != TabDash {
		t.Errorf("tab = %v, want the dashboard", got.tab)
	}
}

// h j k l walk the grid the months are laid out in: one month either way, one row of them up
// or down, and none of them past January or December.
func TestTimeWalksMonthsWithHJKL(t *testing.T) {
	m := send(t, timeModel(t, 158, 46), runes("g")) // January
	cols := m.timeCols()

	if got := send(t, m, runes("l")); got.timeMonth() != 1 {
		t.Errorf("l landed on month %d, want February", got.timeMonth())
	}
	if got := send(t, m, runes("h")); got.timeMonth() != 0 {
		t.Errorf("h walked past January to month %d", got.timeMonth())
	}
	if got := send(t, m, runes("j")); got.timeMonth() != cols {
		t.Errorf("j landed on month %d, want a row of months on (%d)", got.timeMonth(), cols)
	}
	if got := send(t, m, runes("k")); got.timeMonth() != 0 {
		t.Errorf("k walked past January to month %d", got.timeMonth())
	}

	end := send(t, m, runes("G")) // December
	if got := send(t, end, runes("l")); got.timeMonth() != 11 {
		t.Errorf("l walked past December to month %d", got.timeMonth())
	}
	if got := send(t, end, runes("j")); got.timeMonth() != 11 {
		t.Errorf("j walked past December to month %d", got.timeMonth())
	}
	if got := send(t, end, runes("k")); got.timeMonth() != 11-cols {
		t.Errorf("k landed on month %d, want a row back", got.timeMonth())
	}
	// Walking there brings that month into view with the caret on it.
	sep := send(t, m, runes("j"), runes("j"), runes("l"))
	if v := plain(sep.View()); !strings.Contains(v, "▸ ") {
		t.Errorf("the month walked to has no caret:\n%s", v)
	}
	// And the footer names the four keys off their own bindings.
	if got := m.monthMoveHelp().Help(); got.Key != "h/j/k/l" || got.Desc != "month" {
		t.Errorf("the footer hint is %q %q", got.Key, got.Desc)
	}
}

// g and G pin the window to the ends of the year, ctrl+f and ctrl+b move a row of months,
// and none of them walks past January or December.
func TestTimeMovesInMonths(t *testing.T) {
	m := timeModel(t, 100, 30)
	if got := send(t, m, runes("g")); got.timeMonth() != 0 {
		t.Errorf("g landed on month %d, want January", got.timeMonth())
	}
	if got := send(t, m, runes("G")); got.timeMonth() != 11 {
		t.Errorf("G landed on month %d, want December", got.timeMonth())
	}

	top := send(t, m, runes("g"))
	if got := send(t, top, special(tea.KeyCtrlF)); got.timeMonth() != top.timeCols() {
		t.Errorf("ctrl+f landed on month %d, want a row of months on", got.timeMonth())
	}
	if got := send(t, top, special(tea.KeyCtrlB)); got.timeMonth() != 0 {
		t.Errorf("ctrl+b walked past January to month %d", got.timeMonth())
	}
	if got := send(t, send(t, m, runes("G")), special(tea.KeyCtrlF)); got.timeMonth() != 11 {
		t.Errorf("ctrl+f walked past December to month %d", got.timeMonth())
	}
}

// A day off is a filled badge in the leave type's colour with the date reversed out of it, a
// weekend and a holiday the same badge in the faint fill with the date dimmed, and a working
// day plain — so the colours are never the only signal.
func TestCalendarColorsTheDays(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	// g pins the window to January, which is where the fixture's casual leave is: the year
	// is taller than any terminal, so a colour off screen says nothing about the palette.
	m := send(t, timeModel(t, 158, 45), runes("g"))
	v := m.View()
	// Casual is #01B9AE, the link colour, as a background under 21-23 January.
	if !strings.Contains(v, "48;2;1;185;174") {
		t.Errorf("no casual band on the calendar:\n%s", v)
	}
	// Annual is its own violet, deliberately not the accent.
	if !strings.Contains(v, "48;2;124;107;232") {
		t.Errorf("no annual band on the calendar:\n%s", v)
	}
	// A weekend or a holiday is a filled badge, lighter than the surface under it.
	if !strings.Contains(v, "48;2;32;32;42") {
		t.Errorf("no weekend or holiday badge on the calendar:\n%s", v)
	}
	// Every month sits on the calendar's surface, and the one in view is tinted with the
	// accent — the only thing that colour does there.
	for _, want := range []string{"48;2;21;21;32", "48;2;32;29;31"} {
		if !strings.Contains(v, want) {
			t.Errorf("no month panel (%s) on the calendar:\n%s", want, v)
		}
	}
}

// The button the keys are on fills with what it does — green for ✓, red for ✕ — with the
// mark reversed out in white, since these two are pressed rather than typed into.
func TestLeaveButtonsFillWhenFocused(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	const white = "38;2;255;255;255"
	for _, tc := range []struct {
		field int
		mark  string
		fill  string
	}{
		{leaveOKField, "✓", "48;2;18;204;99"}, // Complete
		{leaveXField, "✕", "48;2;225;52;0"},   // Destructive
	} {
		m := send(t, timeModel(t, 158, 46), runes("n"))
		for m.form.field != tc.field {
			m = send(t, m, special(tea.KeyTab))
		}
		row := lineWith(t, m.View(), tc.mark)
		if !strings.Contains(row, tc.fill) || !strings.Contains(row, white) {
			t.Errorf("%s focused is not filled with white on its own colour:\n%q", tc.mark, row)
		}
		// Unfocused it is a frame, not a fill.
		away := send(t, m, special(tea.KeyTab), special(tea.KeyTab))
		if got := lineWith(t, away.View(), tc.mark); strings.Contains(got, tc.fill) {
			t.Errorf("%s stayed filled with the keys elsewhere:\n%q", tc.mark, got)
		}
	}
}

// A half day bands one of the date's two cells and leaves the other faint, which says both
// that it is half a day and which half.
func TestHalfDayBandsOneCell(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	sick := dayMark{kind: "Sick Time Off", half: true, period: "am"}
	morning := dayCellOf(18, sick, false, false, theme.Surface, "")
	sick.period = "pm"
	afternoon := dayCellOf(18, sick, false, false, theme.Surface, "")

	if plain(morning) != " 18 " || plain(afternoon) != " 18 " {
		t.Fatalf("a half day changed the date: %q / %q", plain(morning), plain(afternoon))
	}
	if morning == afternoon {
		t.Error("a morning and an afternoon render the same")
	}
	// The whole day bands both cells, so it cannot be read as a half.
	sick.half = false
	if full := dayCellOf(18, sick, false, false, theme.Surface, ""); full == morning {
		t.Error("a full day renders as a half day")
	}
}

// The balance row is four boxes, ruled above and below and divided by verticals, and it
// ends exactly where its own rules do — the cells that do not divide evenly among the cards
// have to go somewhere.
func TestBalanceCardsFillTheirRow(t *testing.T) {
	for _, width := range []int{80, 100, 111, 130, 158} {
		v := plain(timeModel(t, width, 40).View())
		rule := lineWith(t, v, "────")
		names := lineWith(t, v, "sick Time Off")
		figures := lineWith(t, v, "DAYS AVAILABLE")
		for _, l := range []string{names, figures} {
			if lipgloss.Width(strings.TrimRight(l, " ")) > lipgloss.Width(rule) {
				t.Errorf("%d cells: a card row is wider than its rule:\n%s\n%s", width, rule, l)
			}
			if got, want := strings.Count(l, "│"), 3; got != want {
				t.Errorf("%d cells: %d dividers between four cards, want %d", width, got, want)
			}
		}
	}
}

// A type with nothing left is dim rather than shouting in its own colour, and the one being
// filtered by is reversed out, so the calendar and the card that explains it read together.
func TestBalanceCardsMarkZeroAndFilter(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	m := timeModel(t, 158, 44)
	// Paternity is 0 of 10 in the fixture: quiet ink, not the type's green.
	figures := lineWith(t, m.View(), wide("8.5"))
	if !strings.Contains(figures, "38;2;94;99;110") {
		t.Errorf("the exhausted balance is not dim:\n%s", figures)
	}
	// Filtering annual reverses its figure out on the violet.
	annual := lineWith(t, send(t, m, runes("a")).View(), wide("8.5"))
	if !strings.Contains(annual, "48;2;124;107;232") {
		t.Errorf("the filtering card is not reversed out:\n%s", annual)
	}
}

// Every line of a month is exactly the panel's width, its blank padding line included: the
// panel is a block of colour, and a line one cell short would notch its edge.
func TestMonthPanelIsRectangular(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	m := timeModel(t, 130, 45)
	marks := m.timeMarks()
	for mon := time.January; mon <= time.December; mon++ {
		block := m.monthBlock(2026, mon, marks)
		for i, l := range append(block.lines, block.filler) {
			if got := lipgloss.Width(l); got != monthCols {
				t.Errorf("%s line %d is %d cells, want %d", mon, i, got, monthCols)
			}
		}
		// The tinted panel is the month in view; the others are the plain one.
		want := "48;2;21;21;32"
		if int(mon)-1 == m.timeMonth() {
			want = "48;2;32;29;31"
		}
		if !strings.Contains(block.lines[1], want) {
			t.Errorf("%s is not on its own panel (%s)", mon, want)
		}
	}
}

// The panel is a pinned column: the months scroll under it and the holidays stay where
// they are read, head and all. Zipped into the body instead, the list scrolled away with
// January on the first keypress.
func TestHolidayPanelIsPinned(t *testing.T) {
	m := timeModel(t, 130, 30)
	// Which screen row the panel starts on, and what is in its column there.
	panelAt := func(v string) (int, string) {
		t.Helper()
		for i, l := range strings.Split(plain(v), "\n") {
			if strings.Contains(l, "PUBLIC HOLIDAYS") {
				// The last rule on the line, not the first: the months are divided by
				// hairlines of their own now.
				return i, l[strings.LastIndex(l, "│"):]
			}
		}
		t.Fatalf("no holiday panel on screen:\n%s", plain(v))
		return 0, ""
	}

	row, col := panelAt(m.View())
	for _, k := range []tea.Msg{runes("G"), special(tea.KeyCtrlF), runes("g")} {
		m = send(t, m, k)
		gotRow, gotCol := panelAt(m.View())
		if gotRow != row || gotCol != col {
			t.Errorf("the panel moved with the months: row %d→%d, %q→%q",
				row, gotRow, col, gotCol)
		}
	}
	// And it is a column, not a row of the body: the rule between them is on every line the
	// calendar occupies.
	rules := 0
	for _, l := range strings.Split(plain(m.View()), "\n") {
		if strings.Contains(l, "│ ") || strings.HasSuffix(l, "│") {
			rules++
		}
	}
	if rules < 15 {
		t.Errorf("the rule beside the panel covers %d lines:\n%s", rules, plain(m.View()))
	}
}

// The holiday panel appears when two months still fit beside it, and gives its column up
// when they do not.
func TestHolidayPanelNeedsRoom(t *testing.T) {
	// The panel needs a full row of months beside it: three of them and their hairlines take
	// 95 cells, so it wants 130 and up.
	wide := plain(timeModel(t, 140, 45).View())
	if !strings.Contains(wide, "PUBLIC HOLIDAYS") || !strings.Contains(wide, "Victory day") {
		t.Errorf("no holiday panel on a wide terminal:\n%s", wide)
	}
	// Mar 30-Apr 2 crosses a month, so both names stay; a run inside one collapses, and one
	// day says which day of the week it takes.
	for _, want := range []string{"Mar 18-23", "Mar 30-Apr 2", "Aug 5 (Wed)"} {
		if !strings.Contains(wide, want) {
			t.Errorf("the panel is missing the span %q:\n%s", want, wide)
		}
	}
	if narrow := plain(timeModel(t, 90, 24).View()); strings.Contains(narrow, "PUBLIC HOLIDAYS") {
		t.Errorf("the panel took the calendar's room on a 90-cell terminal:\n%s", narrow)
	}
}

// The calendar's own furniture cannot overflow the terminal: every line inside the width,
// and the whole screen inside the height. The panel is zipped in beside the months, which
// is exactly the kind of layout that grows a line without being asked.
func TestCalendarFitsTheTerminal(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{
		{Width: 80, Height: 24}, {Width: 100, Height: 30},
		{Width: 121, Height: 40}, {Width: 130, Height: 45},
		{Width: 148, Height: 44}, {Width: 158, Height: 44},
		{Width: 60, Height: 20},
	} {
		m := timeModel(t, size.Width, size.Height)
		// Closed, filtered, and with the new-timeoff line open on each of its fields: the
		// line is where a cell too many would come from.
		open := send(t, m, runes("n"))
		half := send(t, open, special(tea.KeyTab), runes(" "))
		states := []Model{m, send(t, m, runes("a")), open, half}
		for i := range leaveFieldCount {
			states = append(states, m.leaveAt(i))
		}
		for _, filter := range states {
			lines := strings.Split(filter.View(), "\n")
			if len(lines) > size.Height {
				t.Errorf("%dx%d: the view is %d lines", size.Width, size.Height, len(lines))
			}
			for i, l := range lines {
				if w := lipgloss.Width(l); w > size.Width {
					t.Errorf("%dx%d: line %d is %d cells wide:\n%s",
						size.Width, size.Height, i, w, l)
				}
			}
		}
	}
}

// A leave the ERP wrote with a newline in its type name cannot grow the view, the same
// guard the task list has: one newline in a cell would push every month below it down.
func TestTimeOffTextCannotGrowTheView(t *testing.T) {
	m := send(t, New(), tea.WindowSizeMsg{Width: 100, Height: 30},
		store.KeyMsg{Key: "k", DB: "db"}, runes("o"),
		api.TimeOffMsg{Year: 2026,
			Kinds:    []api.LeaveKind{{ID: 1, Name: "Annual\nTime Off", Available: 1, Max: 2}},
			Leaves:   []api.Leave{{From: "2026-03-02", To: "2026-03-02", KindID: 1, Kind: "Annual\nTime Off", State: "validate"}},
			Holidays: []api.Holiday{{From: "2026-03-05", To: "2026-03-05", Name: "One\nHoliday"}}})

	lines := strings.Split(m.View(), "\n")
	if len(lines) > 30 {
		t.Errorf("the view is %d lines:\n%s", len(lines), m.View())
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w > 100 {
			t.Errorf("line %d is %d cells wide:\n%s", i, w, l)
		}
	}
}

// The days a leave covers are counted inclusively, and a half day counts as half.
func TestTimeTakenCountsDays(t *testing.T) {
	m := timeModel(t, 100, 30)
	// 3 casual + 0.5 sick + 1 annual pending + 3 annual = 7.5
	if got := m.timeTaken(); got != 7.5 {
		t.Errorf("timeTaken = %v, want 7.5", got)
	}
	if got := send(t, m, runes("s")).timeTaken(); got != 0.5 {
		t.Errorf("the sick half day counts %v, want 0.5", got)
	}
}

// A pending request is underlined and says so once, above the calendar; a year with
// nothing pending does not explain an underline nobody can see.
func TestPendingIsExplainedOnlyWhenPresent(t *testing.T) {
	m := timeModel(t, 120, 45)
	if v := plain(m.View()); !strings.Contains(v, "pending underlined") {
		t.Errorf("the pending note is missing:\n%s", v)
	}
	approved := make([]api.Leave, 0, len(sampleLeaves()))
	for _, l := range sampleLeaves() {
		l.State = "validate"
		approved = append(approved, l)
	}
	settled := send(t, m, api.TimeOffMsg{Year: 2026, Kinds: sampleKinds(), Leaves: approved})
	if v := plain(settled.View()); strings.Contains(v, "pending underlined") {
		t.Errorf("a settled year still explains the underline:\n%s", v)
	}
}

// The calendar opens on this month, wherever in the year it is, and marks it with a caret
// — there is no cursor, so that caret is the only thing that says where today is.
func TestCalendarOpensOnThisMonth(t *testing.T) {
	m := timeModel(t, 100, 30)
	if got, want := m.timeMonth(), int(time.Now().Month())-1; got != want {
		t.Errorf("timeMonth = %d, want %d", got, want)
	}
	caret := "▸ " + time.Now().Month().String()[:3]
	if v := plain(m.View()); !strings.Contains(v, caret) {
		t.Errorf("this month has no caret (%q):\n%s", caret, v)
	}
}

// A year the ERP has not answered for is not a blank screen, and r is named as the way to
// ask for it.
func TestEmptyCalendarSaysWhatToPress(t *testing.T) {
	m := send(t, New(), tea.WindowSizeMsg{Width: 100, Height: 30},
		store.KeyMsg{Key: "k", DB: "db"})
	m.tab = TabTime
	if v := plain(m.View()); !strings.Contains(v, "r to read this year") {
		t.Errorf("an empty calendar does not say what to press:\n%s", v)
	}
}

// o before the first sync lands waits for the login rather than failing: RPC needs the key
// owner's email, and only the day total carries it.
func TestTimeWaitsForTheLogin(t *testing.T) {
	m := send(t, New(), tea.WindowSizeMsg{Width: 100, Height: 30},
		store.KeyMsg{Key: "k", DB: "db"}, runes("o"))
	if !m.timeWanted || !m.timeLoading {
		t.Fatalf("wanted = %v, loading = %v — o should wait on the login",
			m.timeWanted, m.timeLoading)
	}
	m, cmd := sendCmd(t, m, api.DayHoursMsg{Date: "01/01/26", UserEmail: "user@example.com"})
	if cmd == nil || m.timeWanted {
		t.Error("the year was not read once the login arrived")
	}
}

// --- the new-timeoff line ----------------------------------------------------

// n opens the line and focuses the leave type. Nothing above or below it moves: the label
// was already on its own row and the row stays exactly one line tall.
func TestNewLeaveOpensWithoutShifting(t *testing.T) {
	m := timeModel(t, 120, 34)
	before := plain(m.View())
	if !strings.Contains(before, "new timeoff") {
		t.Fatalf("the label is not on screen closed:\n%s", before)
	}

	open := send(t, m, runes("n"))
	if open.mode != ModeForm || open.form.field != leaveKindField {
		t.Fatalf("mode = %v, field = %d", open.mode, open.form.field)
	}
	after := plain(open.View())
	if rowOf(t, before, "wk   M") != rowOf(t, after, "wk   M") {
		t.Errorf("opening the line moved the calendar:\n%s", after)
	}
	if len(strings.Split(before, "\n")) != len(strings.Split(after, "\n")) {
		t.Error("opening the line changed the height of the screen")
	}
	// The whole request is on that one row.
	line := lineWith(t, after, "new timeoff")
	for _, want := range []string{"Sick", "full day", "▾", "→", "✓", "✕"} {
		if !strings.Contains(line, want) {
			t.Errorf("the line is missing %q:\n%s", want, line)
		}
	}
}

// Tab walks the fields left to right and wraps; the dropdowns take j/k and space, and the
// duration one swaps the range's end for the half it is asking about.
func TestLeaveFormTabsAndDropdowns(t *testing.T) {
	m := send(t, timeModel(t, 120, 34), runes("n"))
	for i, want := range []int{leaveDurField, leaveFromField, leaveToField, leaveDescField,
		leaveOKField, leaveXField, leaveKindField} {
		m = send(t, m, special(tea.KeyTab))
		if m.form.field != want {
			t.Fatalf("tab %d landed on field %d, want %d", i+1, m.form.field, want)
		}
	}
	if back := send(t, m, special(tea.KeyShiftTab)); back.form.field != leaveXField {
		t.Errorf("shift+tab landed on %d, want the ✕", back.form.field)
	}

	// The type cycles through what the ERP sent, and wraps.
	kinds := send(t, m, runes("j"))
	if kinds.form.kind != 1 {
		t.Errorf("j left the type at %d", kinds.form.kind)
	}
	if back := send(t, m, runes("k")); back.form.kind != len(m.timeKinds)-1 {
		t.Errorf("k left the type at %d, want the last one", back.form.kind)
	}

	// And a type's own initial picks it outright — s/c/a/p, the filter chips' letters.
	for _, tc := range []struct {
		key  string
		want int
	}{{"a", 2}, {"p", 3}, {"s", 0}, {"c", 1}} {
		if got := send(t, m, runes(tc.key)); got.form.kind != tc.want {
			t.Errorf("%q chose type %d (%s), want %d", tc.key, got.form.kind,
				got.timeKinds[got.form.kind].Name, tc.want)
		}
	}

	// Duration is a dropdown, not a checkbox: full day / half day.
	half := send(t, m, special(tea.KeyTab), runes(" "))
	if !half.form.half {
		t.Fatal("space did not choose half day")
	}
	line := plain(half.View())
	if !strings.Contains(line, "half day") || !strings.Contains(line, "morning") {
		t.Errorf("a half day does not ask which half:\n%s", lineWith(t, line, "half day"))
	}
	// And that slot is the period, so tabbing into it and cycling changes the half.
	pm := send(t, half, special(tea.KeyTab), special(tea.KeyTab), runes(" "))
	if !pm.form.pm || !strings.Contains(plain(pm.View()), "afternoon") {
		t.Errorf("the period did not change: pm = %v", pm.form.pm)
	}
}

// Two marks, two meanings, as the design draws them: the accent frame says which field has
// the keys, and the accent fill says its value is selected — the next keystroke replaces it.
// The type reads in its own colour, the one its days are drawn in on the calendar.
func TestLeaveFormFocusMarks(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	const accent, fill = "38;2;255;192;0", "48;2;255;192;0"
	m := send(t, timeModel(t, 128, 36), runes("n"))

	// The type has the keys: an accent frame, and nothing is selected yet, so no fill.
	kind := m.leaveBand()[1]
	if !strings.Contains(kind, accent) {
		t.Errorf("the focused field has no accent frame:\n%s", kind)
	}
	if strings.Contains(kind, fill) {
		t.Errorf("something is reversed out before a date is focused:\n%s", kind)
	}

	// Tab onto a date and its value is selected: reversed out, and the cursor is gone
	// because the whole value is the selection.
	dated := send(t, m, special(tea.KeyTab), special(tea.KeyTab))
	if got := dated.form.field; got != leaveFromField {
		t.Fatalf("field = %d, want the start date", got)
	}
	if !strings.Contains(dated.leaveBand()[1], fill) {
		t.Errorf("a freshly focused date is not shown selected:\n%s", dated.leaveBand()[1])
	}
	// One keystroke and the selection is gone.
	typed := send(t, dated, runes("2"))
	if strings.Contains(typed.leaveBand()[1], fill) {
		t.Errorf("the selection survived a keystroke:\n%s", typed.leaveBand()[1])
	}
}

// Cycling the type recolours its text to that type's own colour — the one its days are drawn
// in on the calendar — and it keeps that colour while the dropdown has the keys, which is
// exactly when the colour is being changed.
func TestLeaveTypeCarriesItsColor(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	// The fixture's types in order: sick, casual, annual, paternity.
	want := []string{"38;2;225;52;0", "38;2;1;185;174", "38;2;124;107;232", "38;2;18;204;99"}
	m := send(t, timeModel(t, 128, 36), runes("n"))
	for i, code := range want {
		// The colour has to be on the type's own span, not merely somewhere on the row: red
		// is also the ✕ button's and green the ✓'s.
		word := firstWord(m.timeKinds[i].Name)
		line := m.leaveBand()[1]
		at := strings.Index(line, word)
		if at < 0 {
			t.Fatalf("%s is not on the line:\n%s", word, line)
		}
		if span := line[max(at-len(code)-10, 0):at]; !strings.Contains(span, code) {
			t.Errorf("%s is not in its own colour (%s), it is styled %q", word, code, span)
		}
		m = send(t, m, runes("j"))
	}
	// A full turn comes back to where it started.
	if m.form.kind != 0 {
		t.Errorf("four steps through four types landed on %d", m.form.kind)
	}
}

// The row is three lines tall whether or not the line is open, because a box is three lines
// and the calendar under it must not move when the fields appear.
func TestLeaveBandKeepsItsHeight(t *testing.T) {
	m := timeModel(t, 128, 36)
	closed := m.leaveBand()
	open := send(t, m, runes("n")).leaveBand()
	if len(closed) != 3 || len(open) != 3 {
		t.Fatalf("the band is %d lines closed and %d open, want 3 either way",
			len(closed), len(open))
	}
	// Boxed, as the design draws it, and the label sits on the middle line beside them.
	if !strings.Contains(plain(open[0]), "╭") || !strings.Contains(plain(open[2]), "╰") {
		t.Errorf("the fields are not boxed:\n%s", plain(strings.Join(open, "\n")))
	}
	if !strings.Contains(plain(open[1]), "new timeoff") {
		t.Errorf("the label is not on the middle line:\n%s", plain(open[1]))
	}
}

// A date field opens with its value selected: typing 21 replaces it whole, and tab fills in
// this month and this year. 21 tab 23 tab lands on the description with both dates written.
func TestLeaveDatesAutofill(t *testing.T) {
	m := send(t, timeModel(t, 120, 34), runes("n"),
		special(tea.KeyTab), special(tea.KeyTab)) // on the start date
	if m.form.field != leaveFromField {
		t.Fatalf("field = %d, want the start date", m.form.field)
	}

	m = send(t, m, runes("21"), special(tea.KeyTab), runes("23"), special(tea.KeyTab))
	if m.form.field != leaveDescField {
		t.Fatalf("two dates and two tabs landed on field %d, want the description", m.form.field)
	}
	now := time.Now()
	wantFrom := fmt.Sprintf("21/%02d/%02d", int(now.Month()), now.Year()%100)
	wantTo := fmt.Sprintf("23/%02d/%02d", int(now.Month()), now.Year()%100)
	if got := m.form.from.Value(); got != wantFrom {
		t.Errorf("start = %q, want %q — a day on its own is this month", got, wantFrom)
	}
	if got := m.form.to.Value(); got != wantTo {
		t.Errorf("end = %q, want %q", got, wantTo)
	}
}

// The first keystroke on a freshly focused date replaces the whole value rather than being
// appended to it — the value is selected, as it is in the entry row.
func TestLeaveDateIsSelectedOnFocus(t *testing.T) {
	m := send(t, timeModel(t, 120, 34), runes("n"),
		special(tea.KeyTab), special(tea.KeyTab), runes("7"))
	if got := m.form.from.Value(); got != "7" {
		t.Errorf("start = %q, want just the 7 that was typed", got)
	}
	// And moving away and back selects it again.
	again := send(t, m, special(tea.KeyTab), special(tea.KeyShiftTab), runes("9"))
	if got := again.form.from.Value(); got != "9" {
		t.Errorf("start = %q after coming back, want just the 9", got)
	}
}

// The days the line covers are marked on the calendar as they are typed, both ends and
// everything between, and the month they are in comes into view.
func TestLeaveRangeMarksTheCalendar(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	m := send(t, timeModel(t, 120, 40), runes("n"),
		special(tea.KeyTab), special(tea.KeyTab), runes("21/3"), special(tea.KeyTab),
		runes("23/3"), special(tea.KeyTab))

	// March, wherever the calendar was before.
	if got := m.timeMonth(); got != 2 {
		t.Errorf("the window is on month %d, want March — the dates typed are in it", got)
	}
	marks := m.timeMarks()
	for _, d := range []string{"2026-03-21", "2026-03-22", "2026-03-23"} {
		if !marks[d].selected {
			t.Errorf("%s is not marked, and it is inside the range", d)
		}
	}
	for _, d := range []string{"2026-03-20", "2026-03-24"} {
		if marks[d].selected {
			t.Errorf("%s is marked, and it is outside the range", d)
		}
	}
	// Marked days are reversed out in the accent, the same way a date jump marks a row.
	if v := m.View(); !strings.Contains(v, "48;2;255;192;0") {
		t.Errorf("nothing on the calendar is reversed out in the accent:\n%s", v)
	}
}

// A half day covers one day however the range field was left.
func TestLeaveHalfDayIsOneDay(t *testing.T) {
	m := send(t, timeModel(t, 120, 34), runes("n"),
		special(tea.KeyTab), special(tea.KeyTab), runes("21/3"), special(tea.KeyTab),
		runes("25/3"), special(tea.KeyShiftTab), special(tea.KeyShiftTab), runes(" "))
	if !m.form.half {
		t.Fatal("the duration is not half day")
	}
	from, to, ok := m.leaveRange()
	if !ok || from != to {
		t.Errorf("range = %s → %s, want one day", from, to)
	}
}

// enter on ✓ states what is about to be filed and asks. y files it; n comes back to the line
// with everything still in it.
func TestLeaveConfirmThenApply(t *testing.T) {
	m := send(t, timeModel(t, 120, 34), runes("n"), runes("j"), runes("j"), // annual
		special(tea.KeyTab), special(tea.KeyTab), runes("21/3"), special(tea.KeyTab),
		runes("23/3"), special(tea.KeyTab), runes("Coast trip"),
		special(tea.KeyTab), special(tea.KeyEnter))

	if m.mode != ModeConfirm || m.cKind != confirmApplyLeave {
		t.Fatalf("mode = %v, kind = %v", m.mode, m.cKind)
	}
	v := plain(m.View())
	for _, want := range []string{"CONFIRM TIME OFF", "ANNUAL TIME OFF",
		"21/03/26  →  23/03/26", "3 days", "8.5 left, this takes 3", "Coast trip"} {
		if !strings.Contains(v, want) {
			t.Errorf("the modal does not say %q:\n%s", want, v)
		}
	}
	// A request the ERP then bills for takes y alone, never a reflexive enter.
	if got := m.confirmKeys().Help().Key; got != "y" {
		t.Errorf("the modal accepts %q", got)
	}
	if enter := send(t, m, special(tea.KeyEnter)); enter.applying {
		t.Error("enter filed the request; only y should")
	}

	// n returns to the line with the request intact.
	no := send(t, m, runes("n"))
	if no.mode != ModeForm || no.form.desc.Value() != "Coast trip" || no.form.from.Value() != "21/03/26" {
		t.Errorf("n did not come back to the line as it was: %v %q %q",
			no.mode, no.form.desc.Value(), no.form.from.Value())
	}

	yes, cmd := sendCmd(t, m, runes("y"))
	if cmd == nil || !yes.applying {
		t.Fatalf("y did not file the request: applying = %v", yes.applying)
	}
	// The days go out in the status: a refusal that names an overlap without naming a day is
	// unreadable unless the screen says which days were asked for.
	if !strings.Contains(yes.status, "21/03/26 → 23/03/26") {
		t.Errorf("status = %q, want the range it is asking for", yes.status)
	}
	// The line stays exactly as typed until the ERP answers, so a refusal has something to
	// come back to.
	if yes.form.desc.Value() != "Coast trip" {
		t.Error("the request was cleared before the ERP answered")
	}
	filed := send(t, yes, api.LeaveRequestedMsg{ID: 3120})
	if filed.form.open || filed.applying {
		t.Errorf("the line is still open after the ERP took the request")
	}
	refused := send(t, yes, api.LeaveRequestedMsg{Err: errors.New("not enough days")})
	if !refused.form.open || refused.form.desc.Value() != "Coast trip" {
		t.Error("a refused request lost what was typed")
	}
}

// The end date comes along when the start passes it. Left behind, the range reads backwards
// and quietly covers the days between — asking for the 20th with the 19th still in the end
// field booked both, and the ERP refused the pair over a leave already on the 19th.
func TestLeaveEndFollowsTheStart(t *testing.T) {
	m := send(t, timeModel(t, 158, 44), runes("n"),
		special(tea.KeyTab), special(tea.KeyTab), runes("19/8"), special(tea.KeyTab),
		runes("19/8"), special(tea.KeyShiftTab))
	// Now move the start past the end.
	m = send(t, m, runes("20/8"), special(tea.KeyTab))
	if got := m.form.to.Value(); got != "20/08/26" {
		t.Errorf("end = %q, want it dragged to the start", got)
	}
	from, to, ok := m.leaveRange()
	if !ok || from != "20/08/26" || to != "20/08/26" {
		t.Errorf("range = %s → %s, want one day", from, to)
	}

	// Moving the start back does not drag the end with it — that is a range, not a move.
	back := send(t, m, special(tea.KeyShiftTab), runes("17/8"), special(tea.KeyTab))
	if got := back.form.to.Value(); got != "20/08/26" {
		t.Errorf("end = %q, want it left where it was", got)
	}
}

// A refused request keeps everything typed and re-reads the year: what refused it is usually
// a leave this screen has not seen, and someone else can file one for you in the web client.
func TestRefusedRequestRereadsTheYear(t *testing.T) {
	m := send(t, timeModel(t, 158, 44), runes("n"),
		special(tea.KeyTab), special(tea.KeyTab), runes("26/1"), special(tea.KeyTab),
		runes("26/1"), special(tea.KeyTab), runes("Overlaps"))
	m.login = "user@example.com"

	after, cmd := sendCmd(t, m, api.LeaveRequestedMsg{
		Err: errors.New("odoo: You can not set two time off that overlap on the same day.")})
	if cmd == nil || !after.timeLoading {
		t.Error("a refusal did not re-read the year")
	}
	if !after.form.open || after.form.desc.Value() != "Overlaps" {
		t.Error("a refusal lost the request")
	}
	if !strings.Contains(after.status, "already have time off") {
		t.Errorf("status = %q, want it to name the overlap", after.status)
	}

	// The reason survives the re-read. It lives in the status because the answer to that
	// re-read clears err, which left a refusal reading "refused" and nothing else.
	landed := send(t, after, api.TimeOffMsg{Year: 2026, Kinds: sampleKinds(),
		Leaves: sampleLeaves(), Holidays: sampleHolidays()})
	if !strings.Contains(landed.status, "overlap") {
		t.Errorf("the reason was lost when the year came back: %q", landed.status)
	}

	// Any other refusal keeps its own sentence, whatever the ERP called it.
	other := send(t, m, api.LeaveRequestedMsg{
		Err: errors.New("odoo: You must be an Officer to request this")})
	if !strings.Contains(other.status, "You must be an Officer") {
		t.Errorf("status = %q, want the ERP's own words", other.status)
	}
}

// The ERP takes one leave per day, so a request over a day that already has some is refused
// here rather than sent and refused there.
func TestLeaveRefusesADayAlreadyTaken(t *testing.T) {
	// The fixture's casual leave runs 21-23 January.
	m := send(t, timeModel(t, 158, 44), runes("n"),
		special(tea.KeyTab), special(tea.KeyTab), runes("22/1"), special(tea.KeyTab),
		runes("25/1"), special(tea.KeyTab), runes("Overlaps"),
		special(tea.KeyTab), special(tea.KeyEnter))

	if m.mode == ModeConfirm {
		t.Fatalf("the modal opened for a request the ERP would refuse:\n%s", plain(m.View()))
	}
	if !strings.Contains(m.status, "22/01/26") || !strings.Contains(m.status, "one leave per day") {
		t.Errorf("status = %q, want it to name the day and why", m.status)
	}
	// Moved off that day, it asks as usual.
	clear := send(t, m, special(tea.KeyShiftTab), special(tea.KeyShiftTab),
		special(tea.KeyShiftTab), runes("26/1"), special(tea.KeyTab), runes("27/1"),
		special(tea.KeyTab), special(tea.KeyTab), special(tea.KeyEnter))
	if clear.mode != ModeConfirm {
		t.Errorf("a request on free days did not ask: %v %q", clear.mode, clear.status)
	}
}

// A request for more days than are left says so in the modal rather than being refused: some
// leave types are allowed to run negative, and the ERP is the one that decides.
func TestLeaveWarnsPastTheBalance(t *testing.T) {
	m := timeModel(t, 158, 44)
	// Paternity is 0 of 10 in the fixture — the fourth type, so three steps down the list.
	m = send(t, m, runes("n"), runes("j"), runes("j"), runes("j"),
		special(tea.KeyTab), special(tea.KeyTab), runes("14/9"), special(tea.KeyTab),
		runes("15/9"), special(tea.KeyTab), runes("Baby"),
		special(tea.KeyTab), special(tea.KeyEnter))
	if m.mode != ModeConfirm {
		t.Fatalf("mode = %v, status = %q", m.mode, m.status)
	}
	if v := plain(m.View()); !strings.Contains(v, "more than you have") {
		t.Errorf("the modal does not warn about the balance:\n%s", v)
	}
}

// esc asks before throwing the request away, and ✕ starts over without asking — nothing has
// been filed, so there is nothing to lose but the typing.
func TestLeaveDiscardAndReset(t *testing.T) {
	m := send(t, timeModel(t, 120, 34), runes("n"),
		special(tea.KeyTab), special(tea.KeyTab), runes("21/3"), special(tea.KeyTab))

	esc := send(t, m, special(tea.KeyEsc))
	if esc.mode != ModeConfirm || esc.cKind != confirmDropLeave {
		t.Fatalf("esc did not ask: mode = %v, kind = %v", esc.mode, esc.cKind)
	}
	if !strings.Contains(plain(esc.View()), "Discard this time off request?") {
		t.Errorf("the prompt does not say what it is asking:\n%s", plain(esc.View()))
	}
	if no := send(t, esc, runes("n")); no.mode != ModeForm || no.form.from.Value() != "21/03/26" {
		t.Errorf("n did not come back to the line: %v %q", no.mode, no.form.from.Value())
	}
	if yes := send(t, esc, runes("y")); yes.form.open || yes.mode == ModeForm {
		if yes.form.open {
			t.Error("y left the line open")
		}
	}

	// ✕ closes the line without asking, back to the label the tab opened on.
	closed := m
	for closed.form.field != leaveXField {
		closed = send(t, closed, special(tea.KeyTab))
	}
	closed = send(t, closed, special(tea.KeyEnter))
	if closed.form.open || closed.mode != ModeList {
		t.Fatalf("✕ left the line open = %v in mode %v", closed.form.open, closed.mode)
	}
	if line := lineWith(t, plain(closed.View()), "new timeoff"); !strings.Contains(line, "new timeoff") {
		t.Errorf("the row does not read as the closed label:\n%s", line)
	}
	// And n opens it again, on its first field with the dates back to today.
	again := send(t, closed, runes("n"))
	if !again.form.open || again.form.field != leaveKindField {
		t.Fatalf("n after ✕ left the line open = %v on field %d", again.form.open, again.form.field)
	}
	if again.form.from.Value() != parse.Today() {
		t.Errorf("reopening left the start date at %q", again.form.from.Value())
	}
}

// The description takes the letters that are keys everywhere else: the line owns the
// keyboard while it is open.
func TestLeaveDescriptionTakesEveryLetter(t *testing.T) {
	m := send(t, timeModel(t, 120, 34), runes("n"),
		special(tea.KeyTab), special(tea.KeyTab), special(tea.KeyTab), special(tea.KeyTab))
	if m.form.field != leaveDescField {
		t.Fatalf("field = %d, want the description", m.form.field)
	}
	typed := send(t, m, runes("to do? not done"))
	if got := typed.form.desc.Value(); got != "to do? not done" {
		t.Errorf("description = %q — t, d, o, n and ? have to be typeable", got)
	}
	if typed.tab != TabTime || typed.mode != ModeForm {
		t.Errorf("typing left the screen: tab = %v, mode = %v", typed.tab, typed.mode)
	}
}

// rowOf is which line of a view holds a string.
func rowOf(t *testing.T, view, needle string) int {
	t.Helper()
	for i, l := range strings.Split(view, "\n") {
		if strings.Contains(l, needle) {
			return i
		}
	}
	t.Fatalf("%q is not on screen:\n%s", needle, view)
	return -1
}

// --- fixtures ----------------------------------------------------------------

// timeModel is the app on the calendar tab with a year already answered for.
func timeModel(t *testing.T, width, height int) Model {
	t.Helper()
	return send(t, New(), tea.WindowSizeMsg{Width: width, Height: height},
		store.KeyMsg{Key: "k", DB: "db"}, runes("o"),
		api.TimeOffMsg{Year: 2026, Kinds: sampleKinds(), Leaves: sampleLeaves(),
			Holidays: sampleHolidays()})
}

func sampleKinds() []api.LeaveKind {
	return []api.LeaveKind{
		{ID: 2, Name: "Sick Time Off", Available: 9, Max: 14},
		{ID: 4, Name: "Casual Time Off", Available: 6, Max: 12},
		{ID: 1, Name: "Annual Time Off", Available: 8.5, Max: 11.5},
		{ID: 7, Name: "Paternity Time Off", Available: 0, Max: 10},
	}
}

func sampleLeaves() []api.Leave {
	return []api.Leave{
		{From: "2026-01-21", To: "2026-01-23", KindID: 4, Kind: "Casual Time Off",
			Desc: "Family errand", State: "validate"},
		{From: "2026-02-18", To: "2026-02-18", KindID: 2, Kind: "Sick Time Off",
			Desc: "Headache", State: "validate", Half: true, Period: "am"},
		{From: "2026-04-16", To: "2026-04-16", KindID: 1, Kind: "Annual Time Off",
			Desc: "Travelling", State: "confirm"},
		{From: "2026-08-19", To: "2026-08-21", KindID: 1, Kind: "Annual Time Off",
			Desc: "Coast trip", State: "validate"},
	}
}

func sampleHolidays() []api.Holiday {
	return []api.Holiday{
		{From: "2026-02-04", To: "2026-02-04", Name: "Shab e-barat*"},
		{From: "2026-02-11", To: "2026-02-12", Name: "Public Holiday (Election)"},
		{From: "2026-03-18", To: "2026-03-23", Name: "Eid-ul-Fitar* & Jumatul Bidah"},
		{From: "2026-03-30", To: "2026-04-02", Name: "Crossing the month"},
		{From: "2026-08-05", To: "2026-08-05", Name: "July Uprising day"},
		{From: "2026-12-16", To: "2026-12-16", Name: "Victory day"},
	}
}

// leaveAt opens the line and tabs to one field, for the tests that walk all of them.
func (m Model) leaveAt(field int) Model {
	next, _ := m.openLeaveForm()
	out := next.(Model)
	for range field {
		out = out.moveLeaveField(1)
	}
	return out
}
