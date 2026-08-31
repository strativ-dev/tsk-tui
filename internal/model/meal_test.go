package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/tasnimAlam/tsk/internal/api"
	"github.com/tasnimAlam/tsk/internal/store"
	"github.com/tasnimAlam/tsk/internal/theme"
)

// m opens the meal calendar and reads the month; t comes back, and the month is not read a
// second time on the way in.
func TestMealTabOpensAndReads(t *testing.T) {
	m := send(t, New(), tea.WindowSizeMsg{Width: 100, Height: 30},
		store.KeyMsg{Key: "k", DB: "db"})
	m.login = "user@example.com" // the sync that carries the email has already landed

	m, cmd := sendCmd(t, m, runes("m"))
	if m.tab != TabMeal {
		t.Fatalf("tab = %v, want TabMeal", m.tab)
	}
	if cmd == nil || !m.mealLoading {
		t.Fatal("m did not start reading the month")
	}
	if v := plain(m.View()); !strings.Contains(v, "reading this month's meals…") {
		t.Errorf("an empty calendar mid-read does not say so:\n%s", v)
	}

	m = send(t, m, mealMsg())
	if m.mealLoading || m.mealMonth == 0 {
		t.Errorf("loading = %v, month = %d", m.mealLoading, m.mealMonth)
	}
	v := plain(m.View())
	for _, want := range []string{
		strings.ToUpper(time.Now().Format("January 2006")),
		"mon", "sat", "breakfast", "lunch", "snacks", "days",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("the calendar is missing %q:\n%s", want, v)
		}
	}

	back := send(t, m, runes("t"))
	if back.tab != TabTasks {
		t.Error("t did not return to the task list")
	}
	again, cmd := sendCmd(t, back, runes("m"))
	if cmd != nil || again.mealLoading {
		t.Error("m read the month a second time — one month in hand at a time")
	}
}

// A booked meal is its type's colour, an open slot is the hueless bar, and a day the canteen
// is shut carries no bars at all.
func TestMealDayStates(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	m := mealModel(t, 100, 40)
	day := mealAt(1) // an early working day of this month, fully booked in the fixture
	// The legend's own swatches are full hue whatever the days hold, so the grid is what is
	// asserted on here — the rows the dates and bars are drawn in.
	grid := strings.Join(mealGridLines(m), "\n")

	if soon := mealSoon(); soon != "" {
		// A meal still to come is the full hue: that is a choice, not history.
		want := theme.MealBooked(theme.MealColor("Lunch")).Render(mealBarOn)
		if !strings.Contains(grid, want) {
			t.Errorf("no full-hue lunch bar for %s:\n%s", soon, plain(grid))
		}
	}
	if !strings.Contains(grid, theme.MealSlot.Render(mealBarOff)) {
		t.Error("no open slot on the calendar — every day cannot be fully booked")
	}
	// The closed day's own row holds its date and nothing else.
	closed := mealAt(0)
	if !m.mealClosed[closed] {
		t.Fatalf("fixture day %q is not closed", closed)
	}
	if got := len(m.mealsOn(closed)); got != 0 {
		t.Errorf("%d bookings on a closed day", got)
	}
	if got := len(m.mealsOn(day)); got != len(m.mealTypes) {
		t.Errorf("%d bookings on %s, want %d", got, day, len(m.mealTypes))
	}
}

// < and > step months, and > stops at this one: the canteen has nothing to report on a month
// that has not happened.
func TestMealStepsMonths(t *testing.T) {
	m := mealModel(t, 100, 40)

	back, cmd := sendCmd(t, m, runes("<"))
	if back.mealOffset != -1 || cmd == nil || !back.mealLoading {
		t.Fatalf("< left the offset at %d, loading = %v", back.mealOffset, back.mealLoading)
	}
	if v := plain(back.View()); !strings.Contains(v,
		strings.ToUpper(time.Now().AddDate(0, -1, 0).Format("January 2006"))) {
		t.Errorf("< did not title last month:\n%s", v)
	}
	if fwd, cmd := sendCmd(t, m, runes(">")); fwd.mealOffset != 0 || cmd != nil {
		t.Errorf("> stepped past this month to %d", fwd.mealOffset)
	}
	// And back again re-reads rather than keeping two months in hand.
	if again, cmd := sendCmd(t, back, runes(">")); again.mealOffset != 0 || cmd == nil {
		t.Errorf("> from last month left the offset at %d, cmd = %v", again.mealOffset, cmd)
	}
}

// The calendar cannot overflow the terminal: every line inside the width, the whole screen
// inside the height. The month grid is laid out in fixed columns, which is exactly the kind
// of layout that grows a line without being asked.
func TestMealCalendarFitsTheTerminal(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{
		{Width: 80, Height: 24}, {Width: 100, Height: 30},
		{Width: 121, Height: 40}, {Width: 158, Height: 44},
	} {
		m := mealMenuModel(t, size.Width, size.Height)
		v := m.View()
		lines := strings.Split(v, "\n")
		if len(lines) > size.Height {
			t.Errorf("%dx%d: %d lines", size.Width, size.Height, len(lines))
		}
		for i, l := range lines {
			if w := lipgloss.Width(l); w > size.Width {
				t.Errorf("%dx%d: line %d is %d cells: %q",
					size.Width, size.Height, i, w, plain(l))
			}
		}
	}
}

// Everything the ERP wrote goes through oneLine: a menu name with a newline in it would
// render the calendar a row taller than the screen it was given.
func TestMealTextCannotGrowTheView(t *testing.T) {
	m := mealModel(t, 100, 30)
	tall := send(t, m, api.MealMsg{Year: time.Now().Year(), Month: time.Now().Month(),
		Types: []api.MealType{{ID: 2, Name: "Break\nfast"}},
		Bookings: []api.MealBooking{{ID: 1, Date: mealAt(1), TypeID: 2,
			Type: "Break\nfast", Menu: "rice\nand\nfish"}},
		Closed: map[string]bool{}})
	if got, want := len(strings.Split(tall.View(), "\n")), len(strings.Split(m.View(), "\n")); got > want {
		t.Errorf("a newline in a menu name grew the view from %d to %d lines", want, got)
	}
}

// x clears the cursor day — every meal on it — and asks first with what will go named: three
// meals is information, a yes/no question is not. It takes y alone, since an unlink cannot be
// undone. The word is "clear", not "cancel": c is the key that cancels chosen meals.
func TestMealCancelAsksFirst(t *testing.T) {
	m := mealCursorOn(t, mealModel(t, 100, 40), mealAt(1))

	ask := send(t, m, runes("x"))
	if ask.mode != ModeConfirm || ask.cKind != confirmDropMeals {
		t.Fatalf("x did not ask: mode = %v, kind = %v", ask.mode, ask.cKind)
	}
	prompt := plain(ask.View())
	for _, want := range []string{"Clear ", "3 meals", "breakfast", "lunch", "snacks"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the prompt does not name %q:\n%s", want, prompt)
		}
	}
	// enter must not fire it: cancelling a day is not an enter-key reflex.
	if still, cmd := sendCmd(t, ask, special(tea.KeyEnter)); cmd != nil || still.mealCancelling {
		t.Error("enter cancelled the day, want y only")
	}
	if no := send(t, ask, runes("n")); no.mode == ModeConfirm || no.mealCancelling {
		t.Error("n did not come back to the calendar")
	}

	yes, cmd := sendCmd(t, ask, runes("y"))
	if cmd == nil || !yes.mealCancelling {
		t.Fatal("y did not send the cancellation")
	}
	// The answer re-reads the month rather than dropping the rows here: someone else can
	// book or cancel for you in the web client.
	done, cmd := sendCmd(t, yes, api.MealsDeletedMsg{Date: mealAt(1), N: 3})
	if cmd == nil || !done.mealLoading {
		t.Error("a finished cancellation did not re-read the month")
	}
	if !strings.Contains(done.status, "cancelled 3 meals") {
		t.Errorf("status = %q", done.status)
	}
	// A refusal keeps the day exactly as it was and says what the ERP said.
	kept := send(t, yes, api.MealsDeletedMsg{Date: mealAt(1),
		Err: errors.New("cannot change past bookings")})
	if len(kept.mealsOn(mealAt(1))) != 3 {
		t.Error("a refused cancellation dropped the meals from the screen")
	}
	if !strings.Contains(kept.status, "cannot change past bookings") {
		t.Errorf("the refusal is not in the status line: %q", kept.status)
	}
}

// The two refusals that happen before the round trip: a day with nothing on it, and a day
// the ERP has already locked.
func TestMealCancelRefusesWhatTheErpWould(t *testing.T) {
	m := mealModel(t, 100, 40)

	empty := mealCursorOn(t, m, mealAt(3)) // nothing booked in the fixture
	if got := send(t, empty, runes("x")); got.mode == ModeConfirm {
		t.Error("x asked about a day with nothing booked")
	} else if !strings.Contains(got.status, "nothing booked") {
		t.Errorf("status = %q", got.status)
	}

	locked := m
	for i := range locked.mealBookings {
		locked.mealBookings[i].Locked = true
	}
	locked = mealCursorOn(t, locked, mealAt(1))
	if got := send(t, locked, runes("x")); got.mode == ModeConfirm {
		t.Error("x asked about a day past its cutoff")
	} else if !strings.Contains(got.status, "cutoff") {
		t.Errorf("status = %q", got.status)
	}
}

// h j k l walk the days, and neither end of the month wraps into a month this screen has
// not read.
func TestMealWalksDays(t *testing.T) {
	m := send(t, mealModel(t, 120, 44), runes("g")) // the 1st
	if m.mealCursor() != 1 {
		t.Fatalf("g landed on day %d", m.mealCursor())
	}
	if got := send(t, m, runes("h")); got.mealCursor() != 1 {
		t.Errorf("h walked past the 1st to %d", got.mealCursor())
	}
	if got := send(t, m, runes("l")); got.mealCursor() != 2 {
		t.Errorf("l landed on %d, want the 2nd", got.mealCursor())
	}
	if got := send(t, m, runes("j")); got.mealCursor() != 8 {
		t.Errorf("j landed on %d, want a week on", got.mealCursor())
	}
	end := send(t, m, runes("G"))
	if got := send(t, end, runes("l")); got.mealCursor() != m.mealDays() {
		t.Errorf("l walked past the end of the month to %d", got.mealCursor())
	}
	// A month step puts the cursor back on today, or on the 1st of a month today is not in.
	if back := send(t, end, runes("<")); back.mealCursor() != 1 {
		t.Errorf("< left the cursor on day %d of last month", back.mealCursor())
	}
}

// A day already eaten goes grey: it cannot be booked or cancelled, which is what that grey
// says everywhere else on this screen, and it leaves the three hues to the days the keys can
// still act on. is_locked_for_user is not what decides it — locked means the booking can no
// longer be changed, which is true of tomorrow's lunch after this morning's cutoff.
func TestMealPastGoesGrey(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	m := mealModel(t, 120, 44)
	past := mealAt(1) // in the fixture the booked days are behind us
	if past >= time.Now().Format("2006-01-02") {
		t.Skip("the first working day of this month has not passed yet")
	}
	grid := strings.Join(mealGridLines(m), "\n")
	if want := theme.MealBooked(theme.MealQuiet).Render(mealBarOn); !strings.Contains(grid, want) {
		t.Errorf("a past booking is not drawn in the quiet grey:\n%s", plain(grid))
	}
	// Still a block, not the open slot's rule: the meal was eaten, and a day it was booked
	// on must not read as one nobody ate on.
	if slot := theme.MealSlot.Render(mealBarOff); strings.Count(grid, slot) == 0 {
		t.Error("no open slot on the grid at all — the fixture cannot say anything")
	}
	_ = past
}

// The weekday over the cursor's column takes the accent: the band behind the cell is two
// cells of a slightly lighter background, and on a narrow grid that is not much to find a
// column by. It outranks the weekend's quiet style, and it goes while a line owns the keys.
func TestMealHeadMarksTheCursorColumn(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	const accent = "255;192;0" // #FFC000

	// accented is the weekday whose head carries the accent, "" if none does.
	accented := func(m Model) string {
		for _, d := range []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"} {
			for _, span := range strings.Split(m.mealHeads(true), "\x1b[0m") {
				if strings.Contains(span, d) && strings.Contains(span, accent) {
					return d
				}
			}
		}
		return ""
	}

	m := mealModel(t, 120, 44)
	names := []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}
	want := names[m.mealCursorColumn()]
	if got := accented(m); got != want {
		t.Errorf("the accent is on %q, want the cursor's own column %q", got, want)
	}

	// One column along: whichever head had it hands it over, so exactly one ever has it. Back a
	// day rather than on when the cursor opens on the last of the month, where l cannot move.
	step := "l"
	if m.mealCursor() == m.mealDays() {
		step = "h"
	}
	next := send(t, m, runes(step))
	if got, w := accented(next), names[next.mealCursorColumn()]; got != w {
		t.Errorf("after l the accent is on %q, want %q", got, w)
	}
	if accented(next) == want {
		t.Error("the accent did not move with the cursor")
	}

	// A weekend column wins it too: the cursor can be parked there, and the column it is in
	// is the one thing on this row worth saying.
	// From the first of the month, so a Saturday is always within a week of the cursor whatever
	// day the test runs on.
	sat := m.holdMeal(1)
	for sat.mealCursorColumn() != 5 {
		sat = send(t, sat, runes("l"))
	}
	if got := accented(sat); got != "sat" {
		t.Errorf("a cursor on a Saturday leaves the accent on %q", got)
	}

	// The booking line owns the keyboard, so the head stops advertising a cursor the keys are
	// not on — the same rule the meal labels and the clock button follow.
	if got := accented(send(t, m, runes("b"))); got != "" {
		t.Errorf("the head kept its accent under the booking line: %q", got)
	}
}

// mealCursorOn walks the cursor onto one day, the way the keys would.
func mealCursorOn(t *testing.T, m Model, iso string) Model {
	t.Helper()
	for i := 0; m.mealCursorDate() != iso; i++ {
		if i > 40 {
			t.Fatalf("could not reach %s, stuck on %s", iso, m.mealCursorDate())
		}
		m = send(t, m, runes("l"))
		if m.mealCursor() == m.mealDays() && m.mealCursorDate() != iso {
			m = send(t, m, runes("g"))
		}
	}
	return m
}

// b opens the book-meal line, and the row is three lines tall either way — so revealing the
// fields moves neither the calendar above it nor the status line below.
func TestBookMealLineOpensWithoutShifting(t *testing.T) {
	m := mealMenuModel(t, 120, 34)
	closed := strings.Split(plain(m.View()), "\n")
	open := send(t, m, runes("b"))
	if open.mode != ModeBook || !open.book.open {
		t.Fatalf("b did not open the line: mode %v", open.mode)
	}
	if got := len(strings.Split(plain(open.View()), "\n")); got != len(closed) {
		t.Errorf("opening the line changed the view from %d to %d lines", len(closed), got)
	}
	if !strings.Contains(plain(m.View()), "book meal") {
		t.Error("the closed row does not carry its label")
	}
	// Every meal starts ticked: booking all of them is the common case.
	for _, ty := range open.mealTypes {
		if !open.book.on[ty.ID] {
			t.Errorf("%s did not open ticked", ty.Name)
		}
	}
	// esc closes it outright — nothing was filed, so nothing asks.
	if got := send(t, open, special(tea.KeyEsc)); got.book.open || got.mode == ModeBook {
		t.Error("esc left the line open")
	}
	// And so does ✕.
	x := open
	for x.book.field != x.bookXField() {
		x = send(t, x, special(tea.KeyTab))
	}
	if got := send(t, x, special(tea.KeyEnter)); got.book.open {
		t.Error("✕ left the line open")
	}
}

// The scope is a dropdown and nothing more: j, k and space step it, the way they step every
// other dropdown, and it has no letter of its own — t and c mean the tasks tab and a leave
// filter everywhere else in the app.
func TestBookMealScopeCycles(t *testing.T) {
	m := send(t, mealMenuModel(t, 120, 34), runes("b"))
	if m.book.scope != scopeToday {
		t.Fatalf("the line opens on scope %d, want today", m.book.scope)
	}
	for i, want := range []struct {
		scope int
		label string
	}{
		{scopeTomorrow, "tomorrow"}, {scopeWeek, "week"}, {scopeCustom, "custom"},
		{scopeToday, "today"},
	} {
		m = send(t, m, runes("j"))
		if m.book.scope != want.scope {
			t.Errorf("step %d landed on scope %d, want %d", i+1, m.book.scope, want.scope)
		}
		if row := bookRowLine(t, m); !strings.Contains(row, want.label) {
			t.Errorf("step %d does not read %q:\n%s", i+1, want.label, row)
		}
	}
	if back := send(t, m, runes("k")); back.book.scope != scopeCustom {
		t.Errorf("k stepped to scope %d, want custom", back.book.scope)
	}
	// The letters are gone: neither the row nor the footer advertises one, and pressing them
	// does not move the scope.
	for _, k := range []string{".", "t", "w", "c"} {
		if got := send(t, m, runes(k)); got.book.scope != m.book.scope {
			t.Errorf("%q still steps the scope", k)
		}
	}
	if row := bookRowLine(t, send(t, m, runes("j"))); strings.Contains(row, ".today") {
		t.Errorf("the row still hints a key:\n%s", row)
	}

	// Custom brings the two dates with it, and nothing else does.
	custom := m
	for custom.book.scope != scopeCustom {
		custom = send(t, custom, runes("j"))
	}
	if from, _ := custom.bookDateFields(); from < 0 {
		t.Error("custom has no date fields")
	}
	if from, _ := send(t, custom, runes("j")).bookDateFields(); from >= 0 {
		t.Error("the scope after custom is showing date fields")
	}
	// tab from the last date lands on ✓: the meals are their own letters, so a stop on each
	// tick would do nothing the letter does not.
	tabbed := send(t, custom, special(tea.KeyTab), special(tea.KeyTab), special(tea.KeyTab))
	if tabbed.book.field != tabbed.bookOKField() {
		t.Errorf("three tabs from the scope landed on field %d, want ✓ at %d",
			tabbed.book.field, tabbed.bookOKField())
	}
	if got := custom.bookFieldCount(); got != 5 {
		t.Errorf("the custom line has %d fields, want scope, two dates, ✓ and ✕", got)
	}

	// A cursor on the last field survives losing the two date fields.
	deep := custom
	for deep.book.field != deep.bookXField() {
		deep = send(t, deep, special(tea.KeyTab))
	}
	if got := send(t, deep, runes("j")); got.book.field >= got.bookFieldCount() {
		t.Errorf("the cursor is on field %d of %d", got.book.field, got.bookFieldCount())
	}
}

// A meal's own initial ticks it, and the days a scope covers skip what the canteen is shut on.
func TestBookMealTicksAndDays(t *testing.T) {
	m := send(t, mealMenuModel(t, 120, 34), runes("b"))

	off := send(t, m, runes("l"))
	if off.book.on[1] {
		t.Error("l did not untick lunch")
	}
	if again := send(t, off, runes("l")); !again.book.on[1] {
		t.Error("l did not tick lunch back on")
	}
	if got := send(t, m, runes("b")); got.book.on[2] {
		t.Error("b did not untick breakfast")
	}

	// today is one day, and the week is the seven ahead minus the closed ones.
	if got := len(m.bookDays()); got != 1 && !m.mealClosed[time.Now().Format("2006-01-02")] {
		t.Errorf("today covers %d days", got)
	}
	week := m
	for week.book.scope != scopeWeek {
		week = send(t, week, runes("j"))
	}
	days := week.bookDays()
	if len(days) == 0 || len(days) > 7 {
		t.Fatalf("the week covers %d days", len(days))
	}
	for _, d := range days {
		if week.mealClosed[d] {
			t.Errorf("%s is closed and still in the range", d)
		}
		if d < time.Now().Format("2006-01-02") {
			t.Errorf("%s is in the past", d)
		}
	}
}

// enter on ✓ books what is ticked, with no modal: a booking is reversible — x cancels the day
// — so a prompt in front of every meal costs more than the mistake.
func TestBookMealSends(t *testing.T) {
	m := send(t, mealMenuModel(t, 120, 34), runes("b"), runes("w"))
	ok := m
	for ok.book.field != ok.bookOKField() {
		ok = send(t, ok, special(tea.KeyTab))
	}
	ask := send(t, ok, special(tea.KeyEnter))
	if ask.mode != ModeConfirm || ask.cKind != confirmBookMeals {
		t.Fatalf("enter on ✓ did not ask: mode %v, kind %v", ask.mode, ask.cKind)
	}
	// The prompt states what it is about to book — the days and the meals, not "are you sure".
	prompt := plain(ask.View())
	for _, want := range []string{"Book ", "breakfast", "lunch", "snacks"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the prompt does not name %q:\n%s", want, prompt)
		}
	}
	// n comes back to the line with everything still on it.
	if no := send(t, ask, runes("n")); no.mode != ModeBook || !no.book.open {
		t.Errorf("n left the line: mode %v, open %v", no.mode, no.book.open)
	}
	sent, cmd := sendCmd(t, ask, runes("y"))
	if cmd == nil || !sent.booking {
		t.Fatalf("y did not book: booking = %v", sent.booking)
	}
	if !strings.Contains(sent.status, "booking") {
		t.Errorf("status = %q", sent.status)
	}

	// The answer closes the line and re-reads the month: what was booked is on the calendar.
	done, cmd := sendCmd(t, sent, api.MealBookedMsg{Booked: 6})
	if done.book.open || done.mode == ModeBook {
		t.Error("a booked line stayed open")
	}
	if cmd == nil || !done.mealLoading {
		t.Error("a finished booking did not re-read the month")
	}
	if !strings.Contains(done.status, "booked 6 meals") {
		t.Errorf("status = %q", done.status)
	}
	// What the ERP refused is said in its own words, since it is usually a rule this screen
	// cannot see — a cutoff that passed while the line was open.
	part := send(t, sent, api.MealBookedMsg{Booked: 2, Skipped: 1, Why: "Booking is closed"})
	if !strings.Contains(part.status, "1 refused: Booking is closed") {
		t.Errorf("status = %q", part.status)
	}

	// Nothing ticked is refused before the round trip, and so is a range with no open day.
	bare := m
	for _, ty := range bare.mealTypes {
		bare.book.on[ty.ID] = false
	}
	for bare.book.field != bare.bookOKField() {
		bare = send(t, bare, special(tea.KeyTab))
	}
	if got, cmd := sendCmd(t, bare, special(tea.KeyEnter)); cmd != nil || got.mode == ModeConfirm {
		t.Error("a line with no meal ticked asked to be booked")
	} else if !strings.Contains(got.status, "tick a meal first") {
		t.Errorf("status = %q", got.status)
	}
}

// c opens the cancel line — the same fields, the opposite verb — and only one of the two is
// ever on the row: the other's label stays but goes dim, key and all.
func TestCancelMealLine(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	m := mealMenuModel(t, 120, 34)
	// Closed, both labels are there with their keys in the accent.
	closed := strings.Join(m.bookBand(), "\n")
	for _, want := range []string{theme.HintKey.Render("b"), theme.HintKey.Render("c")} {
		if !strings.Contains(closed, want) {
			t.Errorf("the closed row is missing an accented key")
		}
	}
	if got := plain(closed); !strings.Contains(got, "book meal") ||
		!strings.Contains(got, "cancel meal") {
		t.Errorf("the closed row does not carry both labels:\n%s", got)
	}

	drop := send(t, m, runes("c"))
	if drop.mode != ModeBook || !drop.book.open || !drop.book.drop {
		t.Fatalf("c did not open the cancel line: open %v, drop %v",
			drop.book.open, drop.book.drop)
	}
	if row := bookRowLine(t, drop); !strings.Contains(row, "cancel meal") {
		t.Errorf("the row does not say what it does:\n%s", row)
	}
	if !strings.Contains(plain(drop.footer()), "-- CANCEL MEAL --") {
		t.Errorf("the mode line does not name the verb:\n%s", plain(drop.footer()))
	}
	// The other verb's key is dim while this line is open: it does nothing until esc.
	if band := strings.Join(drop.bookBand(), "\n"); strings.Contains(band,
		theme.HintKey.Render("c")) {
		t.Error("the open line's own key is still advertised")
	}
	// b is breakfast here, not a way back — one key, one job. With nothing booked today it is
	// disabled instead, and says so rather than moving a tick that could not act.
	if got := send(t, drop, runes("b")); !got.book.drop {
		t.Error("b on the cancel line switched the verb")
	} else if !strings.Contains(got.status, "no breakfast booked") {
		t.Errorf("status = %q", got.status)
	}

	// ✓ refuses a scope with nothing of yours in it.
	ok := drop
	for ok.book.field != ok.bookOKField() {
		ok = send(t, ok, special(tea.KeyTab))
	}
	if empty := send(t, ok, special(tea.KeyEnter)); empty.mode == ModeConfirm {
		t.Error("✓ asked about a day with nothing of yours on it")
	}

	// With a booking on today, the line opens with that meal ticked and ✓ asks — y only.
	held := m
	held.mealBookings = append(held.mealBookings, api.MealBooking{ID: 900,
		Date: time.Now().Format("2006-01-02"), TypeID: 2, Type: "Breakfast"})
	held = send(t, held, runes("c"))
	if !held.book.on[2] {
		t.Error("the cancel line did not tick the meal it could cancel")
	}
	// A tick change does not reach the model it was copied from.
	if off := send(t, held, runes("b")); off.book.on[2] || !held.book.on[2] {
		t.Errorf("tick after b = %v, original = %v", off.book.on[2], held.book.on[2])
	}
	for held.book.field != held.bookOKField() {
		held = send(t, held, special(tea.KeyTab))
	}
	ask := send(t, held, special(tea.KeyEnter))
	if ask.mode != ModeConfirm || ask.cKind != confirmDropForm {
		t.Fatalf("✓ did not ask: mode %v, kind %v", ask.mode, ask.cKind)
	}
	if !strings.Contains(plain(ask.View()), "Cancel 1 meal?") {
		t.Errorf("the prompt does not say what goes:\n%s", ask.cPrompt)
	}
	if still, cmd := sendCmd(t, ask, special(tea.KeyEnter)); cmd != nil || still.mealCancelling {
		t.Error("enter fired the cancellation, want y only")
	}
	sent, cmd := sendCmd(t, ask, runes("y"))
	if cmd == nil || !sent.mealCancelling {
		t.Fatal("y did not send the cancellation")
	}
	// The answer closes the line and re-reads the month.
	done, cmd := sendCmd(t, sent, api.MealsDeletedMsg{Date: time.Now().Format("2006-01-02"), N: 1})
	if done.book.open || cmd == nil || !done.mealLoading {
		t.Errorf("open %v, loading %v", done.book.open, done.mealLoading)
	}
	if !strings.Contains(done.status, "cancelled 1 meal") {
		t.Errorf("status = %q", done.status)
	}
}

// The cancel line previews the day as it will be: a ticked meal that is booked comes off the
// calendar, and one left unticked keeps its own colour, because it is staying.
func TestCancelMealPreviewsTheDay(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	day := mealSoon()
	if day == "" {
		t.Skip("no working day left in this month to book on")
	}
	m := mealMenuModel(t, 120, 34)
	for i, ty := range m.mealTypes {
		m.mealBookings = append(m.mealBookings, api.MealBooking{ID: 900 + i, Date: day,
			TypeID: ty.ID, Type: ty.Name})
	}

	c := send(t, m, runes("c"))
	for i := 0; !c.bookCovers(day) && i <= scopeCount; i++ {
		c = send(t, c, runes("j")) // walk the scope until it covers that day
	}
	if !c.bookCovers(day) {
		t.Fatalf("no scope covers %s", day)
	}

	slot := theme.MealSlot.Render(mealBarOff)
	lunch := theme.MealBooked(theme.MealColor("Lunch")).Render(mealBarOn)
	breakfast := theme.MealBooked(theme.MealColor("Breakfast")).Render(mealBarOn)

	// Everything ticked: the day empties.
	all := c.bookBar(day, m.mealCell(mealGap), mealGap)
	if strings.Contains(all, lunch) || strings.Contains(all, breakfast) {
		t.Errorf("a fully ticked day still draws its meals: %q", plain(all))
	}
	if got := strings.Count(all, slot); got != len(m.mealTypes) {
		t.Errorf("%d open slots, want %d", got, len(m.mealTypes))
	}

	// Only lunch ticked: lunch goes, the other two stay in their own colours.
	one := send(t, c, runes("b"), runes("s")) // untick breakfast and snacks
	bar := one.bookBar(day, m.mealCell(mealGap), mealGap)
	if strings.Contains(bar, lunch) {
		t.Errorf("lunch is ticked and still on the day: %q", plain(bar))
	}
	if !strings.Contains(bar, breakfast) {
		t.Errorf("breakfast is not ticked and came off the day: %q", plain(bar))
	}
	if got := strings.Count(bar, slot); got != 1 {
		t.Errorf("%d open slots, want just the lunch one", got)
	}
}

// A meal with nothing to cancel in the chosen scope is disabled: its tick is dim, its letter
// carries no accent, and pressing it says why rather than moving a tick that cannot act.
func TestCancelMealDisablesEmptyTicks(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	day := mealSoon()
	if day == "" {
		t.Skip("no working day left in this month")
	}
	m := mealMenuModel(t, 120, 34)
	// Lunch alone, on one day ahead.
	m.mealBookings = append(m.mealBookings, api.MealBooking{ID: 900, Date: day,
		TypeID: 1, Type: "Lunch"})

	c := send(t, m, runes("c"))
	for i := 0; !c.bookCovers(day) && i <= scopeCount; i++ {
		c = send(t, c, runes("j")) // a scope wide enough to reach that day
	}
	if !c.dropAvailable(1) {
		t.Fatalf("lunch is booked on %s and reads as unavailable", day)
	}
	for _, id := range []int{2, 6} {
		if c.dropAvailable(id) {
			t.Errorf("meal %d has nothing booked and reads as available", id)
		}
	}
	// Only what can be cancelled is ticked, and the scope walk keeps that true.
	if !c.book.on[1] || c.book.on[2] || c.book.on[6] {
		t.Errorf("ticks = %v, want lunch alone", c.book.on)
	}
	// The disabled ones say so and take no accent.
	band := strings.Join(c.bookBand(), "\n")
	if !strings.Contains(plain(band), "breakfast  none") {
		t.Errorf("a disabled tick does not say it has nothing:\n%s", plain(band))
	}
	if strings.Contains(band, theme.HintKey.Render("b")) {
		t.Error("a disabled tick still advertises its letter in the accent")
	}
	if !strings.Contains(band, theme.HintKey.Render("l")) {
		t.Error("lunch can be cancelled and its letter is not in the accent")
	}
	// And its key does nothing but explain itself.
	if got := send(t, c, runes("s")); got.book.on[6] {
		t.Error("a disabled tick toggled")
	} else if !strings.Contains(got.status, "no snacks booked") {
		t.Errorf("status = %q", got.status)
	}
}

// mealModel is the tab open with one month of meals on it.
func mealModel(t *testing.T, width, height int) Model {
	t.Helper()
	m := send(t, New(), tea.WindowSizeMsg{Width: width, Height: height},
		store.KeyMsg{Key: "k", DB: "db"})
	m.login = "user@example.com"
	return send(t, m, runes("m"), mealMsg())
}

// mealMsg is a month of this month's own days, so the fixture never rots: the first weekday
// is fully booked, the second has only breakfast, and the first Saturday is closed.
func mealMsg() api.MealMsg {
	now := time.Now()
	types := []api.MealType{
		{ID: 2, Name: "Breakfast", Serving: 10.5, Before: 1},
		{ID: 1, Name: "Lunch", Serving: 14, Before: 4.5},
		{ID: 6, Name: "Snacks", Serving: 17.5, Before: 8},
	}
	var booked []api.MealBooking
	for i, ty := range types {
		booked = append(booked, api.MealBooking{ID: 100 + i, Date: mealAt(1),
			TypeID: ty.ID, Type: ty.Name, Menu: "something"})
	}
	booked = append(booked, api.MealBooking{ID: 200, Date: mealAt(2), TypeID: 2,
		Type: "Breakfast", Menu: "paratha"})
	// And one still to come, when the month has a working day left: a meal in the future is
	// the state the full hue is for.
	if soon := mealSoon(); soon != "" {
		booked = append(booked, api.MealBooking{ID: 300, Date: soon, TypeID: 1,
			Type: "Lunch", Menu: "biriyani"})
	}

	// Two months of weekends, the way get_unusual_days answers: FetchMeals reads two months,
	// and the next one is on screen whenever this one is nearly over — a weekend the fixture
	// left open there draws bars in a column that has no room for them.
	closed := map[string]bool{}
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
	for day := first; day.Before(first.AddDate(0, 2, 0)); day = day.AddDate(0, 0, 1) {
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			closed[day.Format("2006-01-02")] = true
		}
	}
	closed[mealAt(0)] = true
	return api.MealMsg{Year: now.Year(), Month: now.Month(), Types: types,
		Bookings: booked, Closed: closed}
}

// mealAt picks days out of this month by their kind: 0 is the first closed day, and 1, 2 …
// are the working days in order. Dates are never hardcoded, or the fixture stops being this
// month's fixture next month.
func mealAt(nth int) string {
	now := time.Now()
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
	weekend, work := "", []string{}
	for d := 1; d <= first.AddDate(0, 1, -1).Day(); d++ {
		day := time.Date(now.Year(), now.Month(), d, 0, 0, 0, 0, time.Local)
		iso := day.Format("2006-01-02")
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			if weekend == "" {
				weekend = iso
			}
			continue
		}
		work = append(work, iso)
	}
	if nth == 0 {
		return weekend
	}
	if nth <= len(work) {
		return work[nth-1]
	}
	panic(fmt.Sprintf("no %dth working day this month", nth))
}

// The week's menu is a column down the right when the grid can spare the cells, and today's
// own weekday is the one accented thing on it.
func TestMealMenuPanel(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	m := mealMenuModel(t, 120, 34)
	v := plain(m.View())
	if !strings.Contains(v, "MENU · week of "+m.mealWeekStart().Format("2 Jan")) {
		t.Fatalf("no menu panel on a 120-cell terminal:\n%s", v)
	}
	// The panel's own lines: the body cuts the tail of a long week with "… N more", so what is
	// on screen depends on the terminal's height rather than on the panel being right.
	column := plain(strings.Join(m.mealMenuPanel(), "\n"))
	for _, want := range []string{"paratha, omlet, mug dal", "chatpati", "faluda"} {
		if !strings.Contains(column, want) {
			t.Errorf("the panel is missing %q:\n%s", want, column)
		}
	}
	// Today's heading is the accent, and it says so; no other day's is.
	today := time.Now().Format("Mon 2") + " · today"
	if !strings.Contains(v, today) {
		t.Errorf("the panel does not mark today (%q):\n%s", today, v)
	}
	styled := strings.Join(m.mealMenuPanel(), "\n")
	if !strings.Contains(styled, theme.HintKey.Render(today)) {
		t.Errorf("today's weekday is not in the accent")
	}
	// The whole block, not just the heading: today's dishes take the accent too, and another
	// day's do not.
	on := m.menusOn(time.Now().Format("2006-01-02"))
	if len(on) == 0 {
		t.Fatal("the fixture has no menu for today")
	}
	for _, ty := range m.mealTypes {
		mn, ok := on[ty.ID]
		if !ok {
			continue
		}
		// The dish as the panel composes it: the choice, then what everyone gets after a `·`.
		dish := mn.Options
		if mn.Common != "" {
			dish += " · " + mn.Common
		}
		if !strings.Contains(styled, theme.HintKey.Render(truncShaped(dish, m.mealPanelCells()-3))) {
			t.Errorf("today's %s dish is not in the accent", ty.Name)
		}
	}
	other := m.menusOn(m.mealWeekStart().Format("2006-01-02"))
	if mn, ok := other[m.mealTypes[0].ID]; ok && m.mealWeekStart().Format("2006-01-02") !=
		time.Now().Format("2006-01-02") {
		if strings.Contains(styled, theme.HintKey.Render(truncShaped(mn.Options, m.mealPanelCells()-3))) {
			t.Error("another day's dish is accented too")
		}
	}
	for _, other := range []string{"Mon", "Tue", "Wed", "Thu", "Fri"} {
		day := other + " " // the bare heading, not today's
		if strings.Contains(styled, theme.HintKey.Render(day)) &&
			!strings.HasPrefix(today, other) {
			t.Errorf("%s is accented and it is not today", other)
		}
	}

	// A narrow terminal keeps the whole grid and drops the column instead.
	if narrow := plain(mealMenuModel(t, 80, 24).View()); strings.Contains(narrow, "MENU ·") {
		t.Errorf("the panel took the grid's room on an 80-cell terminal:\n%s", narrow)
	}
}

// The panel follows the cursor: walking into another week brings that week's menu.
func TestMealMenuFollowsTheCursor(t *testing.T) {
	m := send(t, mealMenuModel(t, 120, 34), runes("g")) // the 1st
	first := m.mealWeekStart()
	next := send(t, m, runes("j")) // a week on
	if !next.mealWeekStart().After(first) {
		t.Fatalf("j left the panel on the week of %s", first.Format("2 Jan"))
	}
	if v := plain(next.View()); !strings.Contains(v,
		"week of "+next.mealWeekStart().Format("2 Jan")) {
		t.Errorf("the panel did not retitle to the cursor's week:\n%s", v)
	}
}

// The accent on the panel is the cursor's day, not today's: it marks what the grid bands and
// what the keys act on, and today already says itself there with a bright underlined date.
func TestMealMenuAccentsTheCursorsDay(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	// Walk to a weekday that is not today, so the two marks cannot be confused — and one this
	// month holds, since the cursor is clamped to the month on screen and the last week of it
	// runs into the next.
	m := mealMenuModel(t, 120, 34)
	mon := m.mealWeekStart()
	var want time.Time
	for _, week := range []time.Time{mon, mon.AddDate(0, 0, -7)} {
		for i := range 5 {
			day := week.AddDate(0, 0, i)
			if day.Month() != m.mealViewed().Month() || day.Day() > m.mealDays() {
				continue
			}
			if day.Format("2006-01-02") != time.Now().Format("2006-01-02") {
				want = day
				break
			}
		}
		if !want.IsZero() {
			break
		}
	}
	m = m.holdMeal(want.Day())
	if m.mealCursorDate() != want.Format("2006-01-02") {
		t.Fatalf("the cursor is on %s, not %s", m.mealCursorDate(), want.Format("2006-01-02"))
	}

	styled := strings.Join(m.mealMenuPanel(), "\n")
	if !strings.Contains(styled, theme.HintKey.Render(truncShaped(want.Format("Mon 2"),
		m.mealPanelCells()))) {
		t.Errorf("the cursor's day (%s) is not accented:\n%s", want.Format("Mon 2"), styled)
	}
	// Its dishes come with it, and today's — a different day now — do not.
	for _, mn := range m.menusOn(want.Format("2006-01-02")) {
		if !strings.Contains(styled, theme.HintKey.Render(truncShaped(mn.Options,
			m.mealPanelCells()-3))) {
			t.Errorf("the cursor's %s dish is not accented", mn.Type)
		}
	}
	for _, mn := range m.menusOn(time.Now().Format("2006-01-02")) {
		if strings.Contains(styled, theme.HintKey.Render(truncShaped(mn.Options,
			m.mealPanelCells()-3))) {
			t.Errorf("today's %s dish is accented and the cursor is elsewhere", mn.Type)
		}
	}
	// Today keeps its own label, without the accent.
	if today := time.Now().Format("Mon 2") + " · today"; strings.Contains(
		plain(styled), today) && strings.Contains(styled, theme.HintKey.Render(today)) {
		t.Error("today is still accented with the cursor on another day")
	}
}

// mealSoon is the next working day after today in this month, or "" if the month is out of
// them — which is what makes the future-coloured assertions skippable rather than flaky.
func mealSoon() string {
	now := time.Now()
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
	for d := now.Day() + 1; d <= first.AddDate(0, 1, -1).Day(); d++ {
		day := time.Date(now.Year(), now.Month(), d, 0, 0, 0, 0, time.Local)
		if day.Weekday() != time.Saturday && day.Weekday() != time.Sunday {
			return day.Format("2006-01-02")
		}
	}
	return ""
}

// mealBookable is a day of this month the cancel keys can act on: the next working day, or
// today when the month has none left after it — the last day of a month still has to be a day
// the tests can put a booking on.
func mealBookable() string {
	if soon := mealSoon(); soon != "" {
		return soon
	}
	return time.Now().Format("2006-01-02")
}

// mealGridLines is the month's own rows — the dates and their bars — without the legend
// above them, whose swatches are full hue whatever the days hold.
func mealGridLines(m Model) []string {
	lines, _ := m.mealLines()
	return lines
}

// mealMenuModel is the tab open with a month of meals and this week's menus on it.
func mealMenuModel(t *testing.T, width, height int) Model {
	t.Helper()
	m := mealModel(t, width, height)
	msg := mealMsg()
	msg.Menus = sampleMenus()
	return send(t, m, msg)
}

// sampleMenus is this week's own days and last week's, so the fixture never rots — and so a
// test needing a weekday of this month that is not today has one to reach for in the week
// nobody is on when today is the last day of the month.
func sampleMenus() []api.MealMenu {
	mon := time.Now().AddDate(0, 0, -((int(time.Now().Weekday()) + 6) % 7))
	out := weekMenus(mon.AddDate(0, 0, -7))
	// Its own dishes, or a test asking whether the cursor's day is marked and today's is not
	// would be comparing one Monday's menu against the same string on the other.
	for i := range out {
		out[i].Options = "last week's " + out[i].Options
	}
	return append(out, weekMenus(mon)...)
}

// weekMenus is one Monday-to-Friday run of the canteen's own dishes.
func weekMenus(mon time.Time) []api.MealMenu {
	dishes := [][3]string{
		{"paratha, omlet, mug dal", "fried rice, fried chicken, chinese vegetables", "ice-cream"},
		{"bread, jam, boiled egg", "rui fish curry / fried egg", "chicken haleem, half nan"},
		{"vegetable khichuri, omlet", "chicken jhal fry", "chatpati"},
		{"paratha, dal and vegetables", "fried egg, two bhortas, rice", "chicken fry, cold drink"},
		{"ruti, mixed vegetables, boiled egg", "hash bhuna / chicken bhuna", "faluda"},
	}
	types := []api.MealType{{ID: 2, Name: "Breakfast"}, {ID: 1, Name: "Lunch"}, {ID: 6, Name: "Snacks"}}
	var out []api.MealMenu
	for d := range dishes {
		day := mon.AddDate(0, 0, d).Format("2006-01-02")
		for i, ty := range types {
			mn := api.MealMenu{Date: day, TypeID: ty.ID, Type: ty.Name, Options: dishes[d][i]}
			if i == 1 && d == 4 {
				mn.Common = "khichuri, achar, cold drink, salad"
			}
			out = append(out, mn)
		}
	}
	return out
}

// The menus are written in Bangla, whose combining marks lipgloss counts as zero cells while
// a terminal without Bengali shaping draws one cell each. Every line has to fit the width
// **by rune count too**, or the panel wraps and the grid's last columns land on the next
// screen row — which reads as the calendar printing its dates twice.
func TestMealPanelFitsUnshapedText(t *testing.T) {
	bangla := []string{
		"পরোটা, অমলেট, মুগ ডাল",
		"ফ্রাইড রাইস,ফ্রাইড চিকেন+চাইনিজ সবজি, মিষ্টি,কোল্ড ড্রিংক,সালাদ",
		"চিকেন হালিম,হাফ নান ও কোল্ড ড্রিংক",
	}
	for _, size := range []tea.WindowSizeMsg{
		{Width: 92, Height: 24}, {Width: 100, Height: 30},
		{Width: 120, Height: 34}, {Width: 158, Height: 44},
	} {
		m := mealModel(t, size.Width, size.Height)
		msg := mealMsg()
		mon := time.Now().AddDate(0, 0, -((int(time.Now().Weekday()) + 6) % 7))
		for d := range 5 {
			day := mon.AddDate(0, 0, d).Format("2006-01-02")
			for i, ty := range msg.Types {
				msg.Menus = append(msg.Menus, api.MealMenu{Date: day, TypeID: ty.ID,
					Type: ty.Name, Options: bangla[i%len(bangla)]})
			}
		}
		m = send(t, m, msg)
		for i, l := range strings.Split(plain(m.View()), "\n") {
			if got := len([]rune(l)); got > size.Width {
				t.Errorf("%dx%d: line %d is %d runes (%d cells) for a %d-cell terminal: %q",
					size.Width, size.Height, i, got, lipgloss.Width(l), size.Width, l)
			}
		}
	}
}

// The days a booking would cover are marked on the calendar as the range is typed, and a range
// that runs into the next month brings that month onto the screen with it.
func TestBookMealRangeShowsOnTheCalendar(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	// A range from the 28th into the next month, typed the way the keys do it.
	m := bookCustom(t, mealMenuModel(t, 160, 40))
	m = send(t, m, special(tea.KeyTab), runes("28"), special(tea.KeyTab), runes("3/9"),
		special(tea.KeyTab))

	days := m.bookDays()
	if len(days) == 0 {
		t.Fatal("the range covers no day")
	}
	if !m.bookCovers(days[0]) {
		t.Errorf("%s is in the range and not marked", days[0])
	}
	// Marked in the accent, the same mark a date jump leaves.
	grid, _ := m.mealLines()
	first, _ := time.Parse("2006-01-02", days[0])
	if want := theme.Match.Render(pad(strconv.Itoa(first.Day()), 2)); !strings.Contains(
		strings.Join(grid, "\n"), want) {
		t.Error("no accent mark on a day the range covers")
	}

	// Both months are on screen, side by side on a terminal this wide.
	// From the first, not from today: AddDate on a 31st rolls a 30-day month over into the
	// one after it, and the test would then look for a month nobody drew.
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).AddDate(0, 1, 0)
	if v := plain(m.View()); !strings.Contains(v, strings.ToUpper(next.Format("January 2006"))) {
		t.Errorf("the month the range runs into is not on screen:\n%s", v)
	}
	if !m.mealTwoUp() {
		t.Error("160 cells does not fit two months")
	}
	// Narrow, they stack instead, and the menu column gives up its cells first.
	tight := bookCustom(t, mealMenuModel(t, 100, 40))
	tight = send(t, tight, special(tea.KeyTab), runes("28"), special(tea.KeyTab), runes("3/9"),
		special(tea.KeyTab))
	if tight.mealTwoUp() {
		t.Error("100 cells claims to fit two months")
	}
	if v := plain(tight.View()); !strings.Contains(v, strings.ToUpper(next.Format("January 2006"))) {
		t.Errorf("the stacked month is missing:\n%s", v)
	}
	// Closing the line takes the second month with it — unless the month is nearly over, which
	// is a reason of its own to keep it there (TestMealTailBringsNextMonth).
	shut := send(t, m, special(tea.KeyEsc))
	if v := plain(shut.View()); shut.mealDays()-now.Day() >= mealTailDays && strings.Contains(v,
		strings.ToUpper(next.Format("January 2006"))) {
		t.Error("the second month outlived the line that brought it")
	}
}

// Two months on screen are laid out the same way, week for week: the header names both and
// pins a weekday row over each, the grids hold nothing but weeks, and a week costs the same
// rows in both — a barless week costing one row in one grid and two in the other slides the
// months a line out of step from there on.
func TestMealTwoMonthsLineUp(t *testing.T) {
	m := bookCustom(t, mealMenuModel(t, 160, 40))
	m = send(t, m, special(tea.KeyTab), runes("28"), special(tea.KeyTab), runes("3/9"),
		special(tea.KeyTab))
	next, twoUp := m.mealTwoMonths()
	if !twoUp {
		t.Fatal("a range into the next month did not put two months side by side")
	}

	// Both named once, on the header's own line, and this month keeps the keys that step it.
	head := plain(strings.Join(m.mealHead(), "\n"))
	for _, want := range []string{
		"< " + strings.ToUpper(m.mealViewed().Format("January 2006")),
		strings.ToUpper(next.Format("January 2006")),
	} {
		if strings.Count(head, want) != 1 {
			t.Errorf("the header says %q %d times:\n%s", want, strings.Count(head, want), head)
		}
	}
	// A weekday row over each grid, beginning where that grid begins.
	heads := ""
	for _, l := range strings.Split(head, "\n") {
		if strings.Contains(l, "mon") && strings.Contains(l, "sun") {
			heads = l
		}
	}
	if strings.Count(heads, "mon") != 2 {
		t.Fatalf("the weekday row does not cover both months: %q", heads)
	}
	if at := lipgloss.Width(heads[:strings.LastIndex(heads, "mon")]); at != gutter+m.monthCells()+2 {
		t.Errorf("the right month's weekday row starts at %d, want %d",
			at, gutter+m.monthCells()+2)
	}

	// Weeks and nothing else in the grids, three lines apiece so the two keep step.
	left, _ := m.monthGrid(m.mealViewed(), true, true)
	right, _ := m.monthGrid(next, false, true)
	for what, grid := range map[string][]string{"this month": left, "the next": right} {
		if joined := plain(strings.Join(grid, "\n")); strings.Contains(joined, "mon") ||
			strings.Contains(joined, strings.ToUpper(next.Format("January"))) {
			t.Errorf("%s carries a head of its own, which the screen would scroll away:\n%s",
				what, joined)
		}
		if len(grid)%3 != 0 {
			t.Errorf("%s spends %d lines, which is not three a week", what, len(grid))
		}
	}
}

// A month with less than a week left in it brings the next one on screen with nothing typed:
// a week booked from here lands mostly in that month, and a grid stopping at the 31st cannot
// show where it went.
func TestMealTailBringsNextMonth(t *testing.T) {
	m := mealMenuModel(t, 160, 40)
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).AddDate(0, 1, 0)

	at, tail := m.mealSpill()
	if want := m.mealDays()-now.Day() < mealTailDays; tail != want {
		t.Fatalf("%d days left after today spills = %v, want %v",
			m.mealDays()-now.Day(), tail, want)
	}
	if tail {
		if !at.Equal(next) {
			t.Errorf("spilled into %s, want %s", at.Format("January 2006"),
				next.Format("January 2006"))
		}
		if v := plain(m.View()); !strings.Contains(v,
			strings.ToUpper(next.Format("January 2006"))) {
			t.Errorf("the tail of the month did not bring the next one on screen:\n%s", v)
		}
	}

	// A past month has no days left in it to run out, and the month after it is one this
	// screen has not read.
	past := send(t, m, runes("<"))
	if _, spill := past.mealSpill(); spill {
		t.Error("a past month spilled into the next one")
	}
}

// bookCustom opens the booking line and steps its dropdown to the custom scope, the way the
// keys do it now that the scopes have no letters of their own.
func bookCustom(t *testing.T, m Model) Model {
	t.Helper()
	m = send(t, m, runes("b"))
	for i := 0; m.book.scope != scopeCustom; i++ {
		if i > scopeCount {
			t.Fatalf("could not reach the custom scope, stuck on %d", m.book.scope)
		}
		m = send(t, m, runes("j"))
	}
	return m
}

// bookRowLine is the booking line's own row — the one with the dropdown on it — found by what
// is on it rather than by its index, since the band grows a label line above or below it.
func bookRowLine(t *testing.T, m Model) string {
	t.Helper()
	for _, l := range m.bookBand() {
		if strings.Contains(plain(l), "▾") {
			return plain(l)
		}
	}
	return ""
}
