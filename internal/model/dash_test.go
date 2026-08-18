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
)

// month is a shortened August: a weekend, worked days, a holiday, an overtime day, a
// short day, and a working day with nothing on it yet.
func month() []api.DayLog {
	return []api.DayLog{
		{Date: "2026-08-01", Weekend: true},
		{Date: "2026-08-03", Actual: 8, Expected: 8},
		{Date: "2026-08-05", Holiday: true},
		{Date: "2026-08-06", Actual: 8, Expected: 8, LoggedTo: 16},
		{Date: "2026-08-10", Actual: 8.25, Expected: 8},
		{Date: "2026-08-17", Actual: 7.75, Expected: 8},
		{Date: "2026-08-18", Actual: 0, Expected: 8},
	}
}

// dashModel is the app on the dashboard tab with a month already answered for.
func dashModel(t *testing.T, width, height int) Model {
	t.Helper()
	return send(t, New(), tea.WindowSizeMsg{Width: width, Height: height},
		store.LoadedMsg{Tasks: []store.Task{{ID: 1, Key: "AI-1", Title: "task"}}},
		store.KeyMsg{Key: "k", DB: "db"},
		runes("d"),
		api.HourLogsMsg{Month: "2026-08-01", Days: month()})
}

// d opens the chart, t comes back, and the tab bar says which one is up.
func TestTabsSwitch(t *testing.T) {
	m := send(t, New(), tea.WindowSizeMsg{Width: 100, Height: 30},
		store.LoadedMsg{Tasks: []store.Task{{ID: 1, Key: "AI-1", Title: "task"}}})
	if m.tab != TabTasks {
		t.Fatalf("tab = %v at launch, want the task list", m.tab)
	}
	if !strings.Contains(m.View(), "tasks") || !strings.Contains(m.View(), "dashboard") {
		t.Errorf("the tab bar is missing:\n%s", m.View())
	}

	dash := send(t, m, runes("d"))
	if dash.tab != TabDash {
		t.Fatalf("tab = %v after d, want the dashboard", dash.tab)
	}
	if !strings.Contains(dash.View(), "-- DASHBOARD --") {
		t.Errorf("mode indicator does not say dashboard:\n%s", dash.View())
	}
	// The query field belongs to the task list, so it is not on this tab.
	if strings.Contains(dash.View(), "search title or tag") {
		t.Errorf("the search box followed us to the dashboard:\n%s", dash.View())
	}

	if back := send(t, dash, runes("t")); back.tab != TabTasks {
		t.Errorf("tab = %v after t, want the task list", back.tab)
	}
	// Digits are aliases in bar order, the way btop numbers its screens.
	if two := send(t, m, runes("2")); two.tab != TabDash {
		t.Errorf("tab = %v after 2, want the dashboard", two.tab)
	}
	if one := send(t, dash, runes("1")); one.tab != TabTasks {
		t.Errorf("tab = %v after 1, want the task list", one.tab)
	}
	// Coming back does not disturb where the cursor was.
	if back := send(t, dash, runes("t")); back.mode != ModeList {
		t.Errorf("mode = %v after coming back, want the list", back.mode)
	}
}

// The letters have to stay typeable where letters are typed.
func TestTabKeysDoNotStealFromFields(t *testing.T) {
	m := send(t, New(), tea.WindowSizeMsg{Width: 100, Height: 30},
		store.LoadedMsg{Tasks: []store.Task{{ID: 1, Key: "AI-1", Title: "draft"}}})

	// Digits too: a query of "2" must not land on the dashboard.
	if q := send(t, m, runes("i"), runes("2")); q.tab != TabTasks || q.search.Value() != "2" {
		t.Errorf("query = %q, tab = %v — a digit was swallowed as a tab switch",
			q.search.Value(), q.tab)
	}
	// The date prompt is all digits, so it cannot lose them either.
	if j := send(t, m, runes("l"), runes("/"), runes("12")); j.tab != TabTasks {
		t.Errorf("tab = %v — the jump prompt lost a digit to a tab switch", j.tab)
	}

	typed := send(t, m, runes("i"), runes("draft"))
	if typed.tab != TabTasks || typed.search.Value() != "draft" {
		t.Errorf("query = %q, tab = %v — d or t was swallowed as a tab switch",
			typed.search.Value(), typed.tab)
	}

	// Same inside an entry's description.
	ins := send(t, m, runes("l"), runes("a"), special(tea.KeyTab), runes("did the thing"))
	if ins.tab != TabTasks || ins.fields[fieldDesc].Value() != "did the thing" {
		t.Errorf("description = %q, tab = %v", ins.fields[fieldDesc].Value(), ins.tab)
	}
}

// x deletes a row; d no longer does, because d is the dashboard now.
func TestDeleteIsX(t *testing.T) {
	tasks := []store.Task{{ID: 1, Key: "AI-1", Title: "task", Rows: []store.Entry{
		{ID: 9, Date: "11/08/26", Desc: "row", Minutes: 60},
	}}}
	m := send(t, New(), tea.WindowSizeMsg{Width: 100, Height: 30},
		store.LoadedMsg{Tasks: tasks}, runes("l"))

	if d := send(t, m, runes("d")); d.mode == ModeConfirm {
		t.Error("d still opens the delete prompt")
	} else if d.tab != TabDash {
		t.Errorf("tab = %v after d inside a task, want the dashboard", d.tab)
	}
	if x := send(t, m, runes("x")); x.mode != ModeConfirm || x.cKind != confirmDeleteRow {
		t.Errorf("x did not open the delete prompt: mode %v kind %v", x.mode, x.cKind)
	}
}

// The chart itself: one row per day, bars for hours, and the days the ERP expected
// nothing of saying why instead of drawing an empty bar.
func TestDashChart(t *testing.T) {
	m := dashModel(t, 100, 30)
	view := m.View()

	for _, want := range []string{
		"AUGUST 2026",
		"logged", "32:00", "/ 40:00", "4 of 5 days", // 8+8+8.25+7.75 over 5 workdays
		"HOURS PER DAY",
		"sat  1", "weekend",
		"wed  5", "holiday",
		"mon  3", "8:00",
		"mon 10", "8:15", // decimal 8.25 reads as h:mm
		"mon 17", "7:45",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("the chart is missing %q:\n%s", want, view)
		}
	}
	// A bar, and a track for the hours not yet logged.
	if !strings.Contains(view, "█") || !strings.Contains(view, "┈") {
		t.Errorf("no bars drawn:\n%s", view)
	}
	// logged_this_day is 16 on the 6th; the bar reads actual, so no 16:00 anywhere.
	if strings.Contains(view, "16:00") {
		t.Errorf("the chart drew logged_this_day instead of actual:\n%s", view)
	}
}

// Each bar takes its colour from the hours on it: under 4h red, under 8h amber, 8h or
// more green. The number beside it agrees, so the colour is never the only signal.
func TestBarColorsByThreshold(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	const (
		red   = "224;87;79"  // #E0574F
		amber = "224;160;48" // #E0A030
		green = "95;191;127" // #5FBF7F
	)
	days := []api.DayLog{
		{Date: "2026-08-03", Actual: 3.5, Expected: 8}, // under 4h
		{Date: "2026-08-04", Actual: 4, Expected: 8},   // exactly 4h
		{Date: "2026-08-05", Actual: 7.9, Expected: 8}, // just under 8h
		{Date: "2026-08-06", Actual: 8, Expected: 8},   // on target
		{Date: "2026-08-07", Actual: 9.5, Expected: 8}, // over
	}
	m := send(t, New(), tea.WindowSizeMsg{Width: 100, Height: 30},
		store.KeyMsg{Key: "k", DB: "db"}, runes("d"),
		api.HourLogsMsg{Month: "2026-08-01", Days: days})

	view := m.View()
	for _, c := range []struct {
		row, want, name string
	}{
		{"mon  3", red, "3:30"},
		{"tue  4", amber, "4:00"},
		{"wed  5", amber, "7:54"},
		{"thu  6", green, "8:00"},
		{"fri  7", green, "9:30"},
	} {
		line := lineWith(t, view, c.row)
		if !strings.Contains(line, c.want) {
			t.Errorf("%s (%s) is not in the right band:\n%s", c.row, c.name, line)
		}
	}
	// A dotted track carrying the 4h and 8h ticks, past the fill.
	if !strings.Contains(view, "┈") || !strings.Contains(view, "┆") {
		t.Errorf("no dotted track with ticks:\n%s", view)
	}
	// The bar is a dark wash of its threshold colour, edged and lettered in the light one:
	// the edges are what separate it from the day stacked on top, so the band itself carries
	// no glyph pattern.
	l := lineWith(t, view, "thu  6")
	for _, want := range []string{
		"48;2;16;",        // the dark green band behind it, #10352A
		"38;2;95;191;127", // #5FBF7F, its light ink and edges
		"▏", "▕",          // its two ends
		";4m", // and the underline along the bottom, which is what separates it from the day below
	} {
		if !strings.Contains(l, want) {
			t.Errorf("the 8h bar is missing %q:\n%s", want, l)
		}
	}
	for _, gone := range []string{"48;2;95;191;127", "█", "▄", "━", "▔"} {
		if strings.Contains(l, gone) {
			t.Errorf("the bar still draws %q:\n%s", gone, l)
		}
	}
	// The hours print inside the band, so they cost the bar no width. Read off the plain text:
	// an underlined band is rendered a cell at a time, so the escapes sit between the digits.
	if !strings.Contains(plain(l), " 8:00 ") {
		t.Errorf("the label is not inside the band:\n%s", l)
	}
	// A working day's date is white; a weekend's stays dim.
	if day := lineWith(t, view, "thu  6"); !strings.Contains(day, "255;255;255") {
		t.Errorf("a working day's date is not white:\n%s", day)
	}
	// A bar that has reached 8h needs no 8h tick at its edge.
	if strings.Contains(l, " ┆") && !strings.Contains(l, "┈┆") {
		t.Errorf("tick drawn at the edge of a bar that reached it:\n%s", l)
	}
}

// The tab bar names the tabs, with the key picked out inside the word rather than
// spelled beside it, and no i on a tab that has no query field.
func TestTabBarAndFooter(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	m := dashModel(t, 100, 30)
	bar := strings.Split(m.View(), "\n")[0]
	if strings.Contains(bar, "t tasks") || strings.Contains(bar, "d dashboard") {
		t.Errorf("the key is spelled beside the label instead of inside it:\n%s", bar)
	}
	// The accent marks the hint letter on the inactive tab.
	if !strings.Contains(bar, "255;192;0") {
		t.Errorf("no accent-coloured hint letter in the tab bar:\n%s", bar)
	}
	// Each tab carries its position as a raised digit, top-left of the label.
	if !strings.Contains(bar, "¹") || !strings.Contains(bar, "²") {
		t.Errorf("no superscript tab numbers:\n%s", bar)
	}

	// The key list is closed until ? opens it, and then it is this tab's keys.
	if closed := m.View(); strings.Contains(closed, "g/G top/bottom") {
		t.Errorf("the key list is open before ? was pressed:\n%s", closed)
	}
	footer := send(t, m, runes("?")).View()
	if strings.Contains(footer, "i search") {
		t.Errorf("the dashboard footer offers the query field:\n%s", footer)
	}
	if !strings.Contains(footer, "tasks") || !strings.Contains(footer, "quit") {
		t.Errorf("the dashboard footer is missing its keys:\n%s", footer)
	}
}

// Same invariants as every other screen: nothing wider than the terminal, nothing
// taller. A month is 31 rows, so this one has to window itself.
func TestDashFitsTheTerminal(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 14}, {200, 40}} {
		w, h := size[0], size[1]
		m := dashModel(t, w, h)
		in := send(t, m, api.AttendanceMsg{At: api.Attendance{
			EmployeeID: 16, CheckedIn: true,
			Since: time.Now().UTC().Add(-5*time.Hour - 12*time.Minute),
		}})
		for _, state := range map[string]Model{
			"dash":        m,
			"loading":     send(t, New(), tea.WindowSizeMsg{Width: w, Height: h}, runes("d")),
			"quit":        send(t, m, runes("q")),
			"checked in":  in,
			"checking in": send(t, in, runes("c"), runes("y")),
		} {
			lines := strings.Split(state.View(), "\n")
			if len(lines) > h {
				t.Errorf("%dx%d: view is %d lines\n%s", w, h, len(lines), state.View())
			}
			for i, l := range lines {
				if got := lipgloss.Width(l); got > w {
					t.Errorf("%dx%d: line %d is %d cells:\n%s", w, h, i, got, l)
				}
			}
		}
	}
}

// The whole month fits one screen: the days run down as many columns as the rows on offer
// need, every one of them keeping its own label and its own printed hours.
func TestDashFitsTheMonthInColumns(t *testing.T) {
	now := time.Now()
	var days []api.DayLog
	for d := 1; d <= 31; d++ {
		days = append(days, api.DayLog{
			Date:     now.Format("2006-01") + fmt.Sprintf("-%02d", d),
			Actual:   8,
			Expected: 8,
		})
	}
	m := send(t, New(), tea.WindowSizeMsg{Width: 100, Height: 24},
		store.KeyMsg{Key: "k", DB: "db"}, runes("d"),
		api.HourLogsMsg{Month: now.Format("2006-01") + "-01", Days: days})

	view := m.View()
	// Nothing is off screen, and nothing had to be windowed away.
	if strings.Contains(view, "more") {
		t.Errorf("the month did not fit a 24-row terminal:\n%s", view)
	}
	// Every day is labelled, and every one carries its hours. The label is matched whole —
	// a bare "26" would also find AUGUST 2026, and "2" the axis.
	for d := 1; d <= 31; d++ {
		date := now.AddDate(0, 0, d-now.Day())
		label := strings.ToLower(date.Format("Mon")) + " " + date.Format("_2")
		if day := lineWith(t, view, label); !strings.Contains(day, "8:00") {
			t.Errorf("%s has no hours on it:\n%s", label, day)
		}
	}
	if cols, rows := m.dashGrid(); cols < 2 || cols*rows < 31 {
		t.Errorf("grid is %dx%d, too small for 31 days at 24 rows", cols, rows)
	}
}

// One column is the roomy case, and it keeps the rule between days — with the window built
// around today, since that is the row being logged into.
func TestDashSeparatesDaysAndFollowsToday(t *testing.T) {
	now := time.Now()
	var days []api.DayLog
	for d := 1; d <= 28; d++ {
		days = append(days, api.DayLog{
			Date:     now.Format("2006-01") + fmt.Sprintf("-%02d", d),
			Actual:   8,
			Expected: 8,
		})
	}
	// Tall enough for one day per row and a rule between them.
	m := send(t, New(), tea.WindowSizeMsg{Width: 100, Height: 70},
		store.KeyMsg{Key: "k", DB: "db"}, runes("d"),
		api.HourLogsMsg{Month: now.Format("2006-01") + "-01", Days: days})

	if cols, _ := m.dashGrid(); cols != 1 {
		t.Fatalf("%d columns at 70 rows, want one day per row", cols)
	}
	lines, focus := m.dashLines(58)
	if focus < 0 {
		t.Fatal("no focus line — the window has nothing to hold")
	}
	if !strings.Contains(lines[focus], "today") {
		t.Errorf("the focused line is not today's:\n%s", lines[focus])
	}
	if !strings.Contains(strings.Join(lines, "\n"), "──") {
		t.Errorf("no rule between days:\n%s", strings.Join(lines, "\n"))
	}

	// Narrow the terminal and the columns take the rules' place.
	tight := send(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})
	if l, _ := tight.dashLines(8); strings.Contains(strings.Join(l, "\n"), "──") {
		t.Errorf("rules survived into a multi-column month:\n%s", strings.Join(l, "\n"))
	}
}

// The whole month is on the chart, the days still to come included — that is where this
// month's holidays are. They draw a bare track, never an empty red bar: nobody could have
// logged hours on a day that has not happened.
func TestDashShowsTheWholeMonth(t *testing.T) {
	now := time.Now()
	day := func(offset int, d api.DayLog) api.DayLog {
		d.Date = now.AddDate(0, 0, offset).Format("2006-01-02")
		return d
	}
	m := send(t, New(), tea.WindowSizeMsg{Width: 100, Height: 40},
		store.KeyMsg{Key: "k", DB: "db"}, runes("d"),
		api.HourLogsMsg{Month: now.Format("2006-01") + "-01", Days: []api.DayLog{
			day(0, api.DayLog{Actual: 8, Expected: 8}),
			day(1, api.DayLog{Expected: 8}),   // still to come
			day(2, api.DayLog{Holiday: true}), // the reason to show them
			day(3, api.DayLog{Weekend: true}),
		}})

	view := m.View()
	tomorrow := strings.ToLower(now.AddDate(0, 0, 1).Format("Mon")) + " " +
		now.AddDate(0, 0, 1).Format("_2")
	if !strings.Contains(view, "holiday") {
		t.Errorf("a holiday later this month is not on the chart:\n%s", view)
	}
	line := lineWith(t, view, tomorrow)
	if strings.Contains(line, "0:00") {
		t.Errorf("a day that has not happened drew a 0:00 bar:\n%s", line)
	}
	if !strings.Contains(line, "┈") {
		t.Errorf("a day still to come drew no track:\n%s", line)
	}
}

// The month's totals and the axis are the frame the bars are read against, so they stay on
// screen whatever the days do.
func TestDashFrameStaysPut(t *testing.T) {
	now := time.Now()
	var days []api.DayLog
	for d := 1; d <= 28; d++ {
		days = append(days, api.DayLog{
			Date: now.Format("2006-01") + fmt.Sprintf("-%02d", d), Actual: 8, Expected: 8,
		})
	}
	for _, h := range []int{24, 40} {
		m := send(t, New(), tea.WindowSizeMsg{Width: 100, Height: h},
			store.KeyMsg{Key: "k", DB: "db"}, runes("d"),
			api.HourLogsMsg{Month: now.Format("2006-01") + "-01", Days: days})
		view := m.View()
		for _, want := range []string{"logged", "HOURS PER DAY", "└"} {
			if !strings.Contains(view, want) {
				t.Errorf("%d rows: %q is not on screen:\n%s", h, want, view)
			}
		}
	}
}

// The number beside the month's totals is today's own hours — what is logged, not the gap
// against the eight due, since a day that has barely started owes nothing yet.
func TestSummaryReportsToday(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	load := func(d api.DayLog) string {
		return send(t, New(), tea.WindowSizeMsg{Width: 100, Height: 30},
			store.KeyMsg{Key: "k", DB: "db"}, runes("d"),
			api.HourLogsMsg{Month: today[:8] + "01", Days: []api.DayLog{d}}).View()
	}

	// The colour carries the shortfall the number no longer states, so the profile is on and
	// "today" and the figure are asserted apart: styled separately, escape codes sit between
	// them.
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	for _, tc := range []struct {
		name string
		day  api.DayLog
		want []string
	}{
		// Nothing logged yet is 0:00, not the eight hours it will owe by tonight, and the
		// red says the day is short without the number implying hours already missed.
		{"nothing logged yet", api.DayLog{Date: today, Expected: 8}, []string{"0:00", "224;87;79"}},
		{"part of a day", api.DayLog{Date: today, Actual: 2.5, Expected: 8}, []string{"2:30", "224;87;79"}},
		{"most of a day", api.DayLog{Date: today, Actual: 7, Expected: 8}, []string{"7:00", "224;160;48"}},
		{"on target", api.DayLog{Date: today, Actual: 8, Expected: 8}, []string{"8:00", "95;191;127"}},
		{"over", api.DayLog{Date: today, Actual: 9, Expected: 8}, []string{"9:00", "95;191;127"}},
		// Nothing was expected, so nothing is owed.
		{"weekend", api.DayLog{Date: today, Weekend: true}, []string{"today off"}},
		// And a month the ERP has not reported today in does not imply a full day owed.
		{"unreported", api.DayLog{Date: today[:8] + "01", Expected: 8}, []string{"today —"}},
	} {
		view := load(tc.day)
		if !strings.Contains(view, "today") {
			t.Errorf("%s: the summary does not name today:\n%s", tc.name, view)
		}
		for _, want := range tc.want {
			if !strings.Contains(view, want) {
				t.Errorf("%s: summary is missing %q:\n%s", tc.name, want, view)
			}
		}
	}
	// The gap it used to print is gone: a red −8:00 on an untouched day read as hours missed.
	if view := load(api.DayLog{Date: today, Expected: 8}); strings.Contains(view, "−8:00") {
		t.Errorf("the summary still states today's gap:\n%s", view)
	}
}

// `logged` runs against the whole month's target — that is the figure the month is billed
// against — while what has been logged, and the days it landed on, stop at today.
func TestSummaryTargetsTheWholeMonth(t *testing.T) {
	now := time.Now()
	day := func(offset int, actual float64) api.DayLog {
		return api.DayLog{
			Date:     now.AddDate(0, 0, offset).Format("2006-01-02"),
			Actual:   actual,
			Expected: 8,
		}
	}
	m := send(t, New(), tea.WindowSizeMsg{Width: 100, Height: 30},
		store.KeyMsg{Key: "k", DB: "db"}, runes("d"),
		api.HourLogsMsg{Month: now.Format("2006-01") + "-01", Days: []api.DayLog{
			day(-1, 8), day(0, 4), day(1, 0), day(2, 0), // two days still to come
		}})

	logged, target, worked, workdays := m.dashTotals()
	if logged != 12 || target != 32 {
		t.Errorf("logged %v of %v, want 12 of 32 — the target is the whole month", logged, target)
	}
	if worked != 2 || workdays != 4 {
		t.Errorf("%d of %d days, want 2 of 4", worked, workdays)
	}
	if view := m.View(); !strings.Contains(view, "12:00") || !strings.Contains(view, "/ 32:00") {
		t.Errorf("the summary does not read 12:00 / 32:00:\n%s", view)
	}
}

// The key list is off until ? asks for it, on both tabs, and ? closes it again. The
// footer keeps its line either way, so opening the list never shoves the body around.
func TestHelpTogglesTheKeyList(t *testing.T) {
	tasks := []store.Task{{ID: 1, Key: "AI-1", Title: "task"}}
	list := send(t, New(), tea.WindowSizeMsg{Width: 100, Height: 30},
		store.LoadedMsg{Tasks: tasks})

	for _, tc := range []struct {
		name  string
		m     Model
		key   string // a hint only that tab's open list carries
		lines int
	}{
		{"tasks", list, "l expand", 30},
		{"dashboard", send(t, dashModel(t, 100, 30), runes("t"), runes("d")), "g/G top/bottom", 30},
	} {
		closed := tc.m.View()
		if strings.Contains(closed, tc.key) {
			t.Errorf("%s: the key list is open before ? was pressed:\n%s", tc.name, closed)
		}
		// Closed, the footer still says which key buys it back.
		if !strings.Contains(closed, "? keys") {
			t.Errorf("%s: nothing advertises ?:\n%s", tc.name, closed)
		}

		open := send(t, tc.m, runes("?"))
		if !strings.Contains(open.View(), tc.key) {
			t.Errorf("%s: ? did not open the key list:\n%s", tc.name, open.View())
		}
		if again := send(t, open, runes("?")); again.showHelp {
			t.Errorf("%s: ? did not close it again", tc.name)
		}
		// The line count does not move, so neither does the body.
		if got := len(strings.Split(open.View(), "\n")); got > tc.lines {
			t.Errorf("%s: the open list made the view %d lines:\n%s", tc.name, got, open.View())
		}
	}

	// ? has to stay typeable where letters are typed.
	q := send(t, list, runes("i"), runes("?"))
	if q.showHelp || q.search.Value() != "?" {
		t.Errorf("query = %q, help = %v — ? was swallowed by the toggle",
			q.search.Value(), q.showHelp)
	}
	desc := send(t, list, runes("l"), runes("a"), special(tea.KeyTab), runes("why?"))
	if desc.showHelp || desc.fields[fieldDesc].Value() != "why?" {
		t.Errorf("description = %q, help = %v", desc.fields[fieldDesc].Value(), desc.showHelp)
	}
}

// q asks on this tab too, and answering no leaves you on the chart.
func TestDashQuitAsksFirst(t *testing.T) {
	m := send(t, dashModel(t, 100, 30), runes("q"))
	if m.mode != ModeConfirm || m.cKind != confirmQuit {
		t.Fatalf("mode = %v kind = %v, want the quit prompt", m.mode, m.cKind)
	}
	if back := send(t, m, runes("n")); back.tab != TabDash {
		t.Errorf("tab = %v after n, want to stay on the chart", back.tab)
	}
	if _, cmd := sendCmd(t, m, runes("y")); !quits(cmd) {
		t.Error("y did not quit")
	}
}

// The month is read once, and r reads it again.
func TestDashReadsTheMonthOnce(t *testing.T) {
	base := send(t, New(), tea.WindowSizeMsg{Width: 100, Height: 30},
		store.KeyMsg{Key: "k", DB: "db"})

	m, cmd := sendCmd(t, base, runes("d"))
	if cmd == nil {
		t.Fatal("opening the dashboard asked the ERP for nothing")
	}
	if !m.dashLoading {
		t.Error("dashLoading is false while the read is in flight")
	}
	if !strings.Contains(m.View(), "reading this month") {
		t.Errorf("the tab does not say it is loading:\n%s", m.View())
	}

	m = send(t, m, api.HourLogsMsg{Month: thisMonth(), Days: month()})
	if m.dashLoading || m.dashMonth != thisMonth() {
		t.Fatalf("loading %v month %q after the answer", m.dashLoading, m.dashMonth)
	}
	// Leaving and coming back does not re-read a month already in hand.
	again, cmd := sendCmd(t, send(t, m, runes("t")), runes("d"))
	if cmd != nil {
		t.Error("the month was read twice")
	}
	if _, cmd = sendCmd(t, again, runes("r")); cmd == nil {
		t.Error("r did not re-read the month")
	}
}

// An answer for another month is not the month on screen.
func TestDashIgnoresNothingUseful(t *testing.T) {
	m := send(t, dashModel(t, 100, 30),
		api.HourLogsMsg{Month: "2026-07-01", Days: []api.DayLog{
			{Date: "2026-07-01", Actual: 3, Expected: 8},
		}})
	// The model keeps whatever answered last; what matters is that it stays coherent —
	// the month heading and the rows come from the same answer.
	if strings.Contains(m.View(), "AUGUST 2026") && strings.Contains(m.View(), "wed  1") {
		t.Errorf("a July answer left August's heading over July's rows:\n%s", m.View())
	}
}

// clockModel is the chart with the ERP's clock already answered for.
func clockModel(t *testing.T, in bool) Model {
	t.Helper()
	at := api.Attendance{EmployeeID: 16}
	if in {
		at.CheckedIn = true
		at.Since = time.Now().UTC().Add(-5*time.Hour - 12*time.Minute)
	}
	return send(t, dashModel(t, 100, 30), api.AttendanceMsg{At: at})
}

// The button is on screen before the month is, because the clock is not about the month.
func TestClockButtonShowsBeforeTheMonth(t *testing.T) {
	m := send(t, New(), tea.WindowSizeMsg{Width: 100, Height: 30},
		store.KeyMsg{Key: "k", DB: "db"}, runes("d"))

	if view := m.View(); !strings.Contains(view, "check in") {
		t.Errorf("no clock button while the month is still loading:\n%s", view)
	}
}

// attendance_manual is a toggle, so c must not fire against a state nobody has read: it
// could check you out when you meant in. It says so rather than doing nothing.
func TestClockKeyNeedsState(t *testing.T) {
	m, cmd := sendCmd(t, dashModel(t, 100, 30), runes("c"))
	if cmd != nil {
		t.Error("c fired a toggle before any attendance answer had landed")
	}
	if m.status == "" {
		t.Error("c was a silent no-op")
	}
	if m.mode == ModeConfirm {
		t.Error("c opened a prompt with no state to act on")
	}
}

// Checking in is one keystroke, and the loader takes the button's place until the ERP
// answers.
func TestClockChecksIn(t *testing.T) {
	m, cmd := sendCmd(t, clockModel(t, false), runes("c"))
	if cmd == nil {
		t.Fatal("c did not fire a check in")
	}
	if !m.clocking || !m.busy() {
		t.Errorf("clocking = %v, busy = %v — the spinner will not animate", m.clocking, m.busy())
	}
	if view := m.View(); !strings.Contains(view, "checking in…") {
		t.Errorf("no loader while the check in is out:\n%s", view)
	}

	// The button only flips once the server has confirmed it.
	done := send(t, m, api.AttendanceMsg{
		At:      api.Attendance{EmployeeID: 16, CheckedIn: true, Since: time.Now().UTC()},
		Toggled: true, Want: true,
	})
	if done.clocking || !done.att.CheckedIn {
		t.Errorf("clocking = %v, checked in = %v after the answer", done.clocking, done.att.CheckedIn)
	}
	if !strings.Contains(done.status, "checked in at") {
		t.Errorf("status = %q", done.status)
	}
}

// Checking out closes a session the ERP then bills, so it asks first — and takes y only,
// never a reflexive enter.
func TestClockChecksOutAfterAsking(t *testing.T) {
	m := clockModel(t, true)
	ask, cmd := sendCmd(t, m, runes("c"))
	if cmd != nil {
		t.Error("c checked out without asking")
	}
	if ask.mode != ModeConfirm || ask.cKind != confirmCheckOut {
		t.Fatalf("mode = %v kind = %v, want the check-out prompt", ask.mode, ask.cKind)
	}
	// The prompt states facts, not a predicted time: it is frozen when it is built.
	if !strings.Contains(ask.cPrompt, "in since") || !strings.Contains(ask.cPrompt, "5:12") {
		t.Errorf("cPrompt = %q", ask.cPrompt)
	}

	if enter, cmd := sendCmd(t, ask, special(tea.KeyEnter)); cmd != nil || enter.clocking {
		t.Error("enter checked out — the prompt should take y only")
	}
	if no := send(t, ask, runes("n")); no.clocking || no.tab != TabDash || no.mode == ModeTable {
		t.Errorf("n left mode = %v tab = %v clocking = %v", no.mode, no.tab, no.clocking)
	}

	yes, cmd := sendCmd(t, ask, runes("y"))
	if cmd == nil || !yes.clocking {
		t.Error("y did not fire the check out")
	}
	if yes.mode == ModeTable {
		t.Errorf("mode = %v after y — the chart is what had the keyboard", yes.mode)
	}
}

// One request at a time: a second press while the first is out does nothing.
func TestClockIgnoresASecondPress(t *testing.T) {
	out := send(t, clockModel(t, false), runes("c"))
	again, cmd := sendCmd(t, out, runes("c"))
	if cmd != nil {
		t.Error("a second c fired while the first was still out")
	}
	if !again.clocking {
		t.Error("the second press cleared the in-flight flag")
	}
}

// A plain read already in flight when a toggle starts is thrown away: the toggle's own
// read is newer, and both answer the same question.
func TestClockDropsAStaleRead(t *testing.T) {
	m := send(t, clockModel(t, false), runes("c")) // clocking, waiting on a check in
	stale := send(t, m, api.AttendanceMsg{At: api.Attendance{EmployeeID: 16}})
	if stale.att.CheckedIn || !stale.clocking {
		t.Errorf("a stale read landed: checked in = %v clocking = %v",
			stale.att.CheckedIn, stale.clocking)
	}
}

// When the ERP disagrees with what was asked for, the server wins and says so. Never a
// retry: attendance_manual is a toggle, so a retry ping-pongs.
func TestClockReportsADisagreement(t *testing.T) {
	m := send(t, clockModel(t, false), runes("c"))
	answer, cmd := sendCmd(t, m, api.AttendanceMsg{
		At: api.Attendance{EmployeeID: 16}, Toggled: true, Want: true,
	})
	if cmd != nil {
		t.Error("a disagreement was retried")
	}
	if answer.att.CheckedIn {
		t.Error("the screen kept our guess instead of the server's answer")
	}
	if !strings.Contains(answer.status, "checked out") {
		t.Errorf("status = %q, want it to say what the ERP says", answer.status)
	}
}

// A refusal is not an error: it says why, and the state it came with is applied.
func TestClockAppliesAWarning(t *testing.T) {
	m := send(t, clockModel(t, false), runes("c"))
	warned := send(t, m, api.AttendanceMsg{
		At:      api.Attendance{EmployeeID: 16, CheckedIn: true, Since: time.Now().UTC()},
		Toggled: true, Want: true,
		Warning: "You are already checked in",
	})
	if warned.err != nil {
		t.Errorf("err = %v, want the warning in the status line", warned.err)
	}
	if !strings.Contains(warned.status, "already checked in") || !warned.att.CheckedIn {
		t.Errorf("status = %q, checked in = %v", warned.status, warned.att.CheckedIn)
	}
}

// A failed call leaves the clock alone — it is the ERP's, not ours.
func TestClockKeepsStateOnFailure(t *testing.T) {
	m := send(t, clockModel(t, true), runes("c"), runes("y")) // check out, in flight
	failed := send(t, m, api.AttendanceMsg{Toggled: true, Want: false,
		Err: errors.New("cannot reach the ERP")})
	if !failed.att.CheckedIn {
		t.Error("a failed check out unchecked the clock on screen")
	}
	if failed.clocking {
		t.Error("the in-flight flag survived the answer")
	}
	if !strings.Contains(failed.status, "attendance unchanged") {
		t.Errorf("status = %q", failed.status)
	}
}

// The elapsed figure repaints itself while checked in, and the tick stops on its own once
// the clock is not running.
func TestClockTickStopsWhenCheckedOut(t *testing.T) {
	in := clockModel(t, true)
	if !in.ticking {
		t.Error("no repaint was scheduled while checked in")
	}
	if _, cmd := sendCmd(t, in, clockTickMsg(time.Now())); cmd == nil {
		t.Error("the tick did not schedule the next one")
	}

	out := clockModel(t, false)
	stopped, cmd := sendCmd(t, out, clockTickMsg(time.Now()))
	if cmd != nil || stopped.ticking {
		t.Error("the clock kept ticking with no session open")
	}
}

// The line above the button: where you are working, when the session started in local
// time, and how long it has run.
func TestClockLineReadsLocal(t *testing.T) {
	days := month()
	days[len(days)-1].WorkLocation = "home" // today
	m := send(t, New(), tea.WindowSizeMsg{Width: 100, Height: 30},
		store.KeyMsg{Key: "k", DB: "db"}, runes("d"),
		api.HourLogsMsg{Month: "2026-08-01", Days: days})

	since := time.Now().UTC().Add(-5*time.Hour - 12*time.Minute)
	in := send(t, m, api.AttendanceMsg{
		At: api.Attendance{EmployeeID: 16, CheckedIn: true, Since: since},
	})
	line := lineWith(t, in.View(), "WFH")
	for _, want := range []string{since.Local().Format("3:04 PM"), "(5:12)"} {
		if !strings.Contains(line, want) {
			t.Errorf("the clock line is missing %q:\n%s", want, line)
		}
	}

	// Checked out says so rather than leaving a clock running.
	out := send(t, m, api.AttendanceMsg{At: api.Attendance{EmployeeID: 16}})
	if v := out.View(); !strings.Contains(v, "checked out") || strings.Contains(v, "(5:12)") {
		t.Errorf("the checked-out line still shows a session:\n%s", v)
	}
}

// Green invites a check in, amber says a clock is running and wants closing.
func TestClockButtonColors(t *testing.T) {
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(restore)

	const (
		amber = "224;160;48" // #E0A030
		green = "95;191;127" // #5FBF7F
	)
	const (
		accent = "255;192;0"   // #FFC000, the c
		white  = "255;255;255" // the words themselves
	)

	// "k in" rather than "check in": the label has its key picked out inside the word, so
	// escape codes sit between the "che" and the "c", as on the tab bar.
	out := lineWith(t, clockModel(t, false).View(), "k in")
	if !strings.Contains(out, green) {
		t.Errorf("the check-in button is not green:\n%s", out)
	}
	in := lineWith(t, clockModel(t, true).View(), "k out")
	if !strings.Contains(in, amber) {
		t.Errorf("the check-out button is not amber:\n%s", in)
	}
	// The words are white and only the c is the accent: the border already carries the
	// state, so the accent is left meaning one thing.
	for _, line := range []string{out, in} {
		if !strings.Contains(line, accent) {
			t.Errorf("the c is not picked out in the accent:\n%s", line)
		}
		if !strings.Contains(line, white) {
			t.Errorf("the label is not white:\n%s", line)
		}
	}
	// And the box is a box: borders above and below the label.
	for _, v := range []string{clockModel(t, false).View(), clockModel(t, true).View()} {
		if !strings.Contains(v, "╭") || !strings.Contains(v, "╰") {
			t.Errorf("the button is not boxed:\n%s", v)
		}
	}
}

// A 401 on the hour log opens the key prompt while this tab is up, so the prompt has to
// reach its own field: routing every key to the chart left it unusable, and c would have
// fired a real check in while someone was pasting a key.
func TestAuthPromptWorksOnTheDashboard(t *testing.T) {
	m := send(t, dashModel(t, 100, 30),
		api.HourLogsMsg{Month: "2026-08-01", Err: api.ErrUnauthorized})
	if m.mode != ModeAuth {
		t.Fatalf("mode = %v after a 401, want the key prompt", m.mode)
	}

	typed := send(t, m, runes("cabd2"))
	if typed.auth.Value() != "cabd2" {
		t.Errorf("key = %q — the chart swallowed the keystrokes", typed.auth.Value())
	}
	if typed.clocking || typed.tab != TabDash {
		t.Errorf("clocking = %v tab = %v", typed.clocking, typed.tab)
	}
}

// The footer names what c will do next, not the thing it belongs to: "clock" left you to
// guess which way the key would go.
func TestClockHelpNamesTheAction(t *testing.T) {
	for _, tc := range []struct {
		name string
		m    Model
		want string
	}{
		{"checked out", clockModel(t, false), "c check in"},
		{"checked in", clockModel(t, true), "c check out"},
		{"in flight", send(t, clockModel(t, false), runes("c")), "c checking in…"},
	} {
		if foot := send(t, tc.m, runes("?")).footer(); !strings.Contains(foot, tc.want) {
			t.Errorf("%s: footer is missing %q:\n%s", tc.name, tc.want, foot)
		}
	}
}
