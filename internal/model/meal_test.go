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
		want := theme.MealBooked(theme.MealColor("Lunch")).Render("━━")
		if !strings.Contains(grid, want) {
			t.Errorf("no full-hue lunch bar for %s:\n%s", soon, plain(grid))
		}
	}
	if !strings.Contains(grid, theme.MealSlot.Render("──")) {
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

// x cancels a day's meals, and it asks first with what will go named — three meals is
// information, a yes/no question is not. It takes y alone, since an unlink cannot be undone.
func TestMealCancelAsksFirst(t *testing.T) {
	m := mealCursorOn(t, mealModel(t, 100, 40), mealAt(1))

	ask := send(t, m, runes("x"))
	if ask.mode != ModeConfirm || ask.cKind != confirmDropMeals {
		t.Fatalf("x did not ask: mode = %v, kind = %v", ask.mode, ask.cKind)
	}
	prompt := plain(ask.View())
	for _, want := range []string{"Cancel 3 meals", "breakfast", "lunch", "snacks"} {
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

// A day already eaten keeps its meal's hue, dimmed: a month of past days still says which
// meals were on. is_locked_for_user is not what decides that — locked means the booking can
// no longer be changed, which is true of tomorrow's lunch after this morning's cutoff.
func TestMealPastKeepsItsHue(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	m := mealModel(t, 120, 44)
	past := mealAt(1) // in the fixture the booked days are behind us
	if past >= time.Now().Format("2006-01-02") {
		t.Skip("the first working day of this month has not passed yet")
	}
	grid := strings.Join(mealGridLines(m), "\n")
	if want := theme.MealBooked(theme.MealPastColor("Breakfast")).Render("━━"); !strings.Contains(grid, want) {
		t.Errorf("a past breakfast lost its hue entirely:\n%s", plain(grid))
	}
	if grey := theme.MealQuietInk.Render("━━"); strings.Contains(grid, grey) {
		t.Error("a booked meal is drawn in the weekend grey")
	}
	_ = past
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

	closed := map[string]bool{}
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
	for d := 1; d <= first.AddDate(0, 1, -1).Day(); d++ {
		day := time.Date(now.Year(), now.Month(), d, 0, 0, 0, 0, time.Local)
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
	for _, want := range []string{"paratha, omlet, mug dal", "chatpati", "faluda"} {
		if !strings.Contains(v, want) {
			t.Errorf("the panel is missing %q:\n%s", want, v)
		}
	}
	// Today's heading is the accent, and it says so; no other day's is.
	today := time.Now().Format("Mon 2") + " · today"
	if !strings.Contains(v, today) {
		t.Errorf("the panel does not mark today (%q):\n%s", today, v)
	}
	styled := m.View()
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

// sampleMenus is this week's own days, so the fixture never rots.
func sampleMenus() []api.MealMenu {
	mon := time.Now().AddDate(0, 0, -((int(time.Now().Weekday()) + 6) % 7))
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
