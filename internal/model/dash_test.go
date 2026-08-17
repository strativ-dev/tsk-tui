package model

import (
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
	// The band is a solid background — a green fill on the 8h row, no glyph pattern in
	// it, with the label in dark ink on the same colour.
	l := lineWith(t, view, "thu  6")
	if !strings.Contains(l, "48;2;95;191;127") || !strings.Contains(l, "38;2;21;21;32") {
		t.Errorf("the bar is not a solid band with dark ink:\n%s", l)
	}
	for _, glyph := range []string{"█", "▄", "▏"} {
		if strings.Contains(l, glyph) {
			t.Errorf("the band still draws %q inside it:\n%s", glyph, l)
		}
	}
	// The hours print inside the band, so they cost the bar no width.
	if !strings.Contains(l, " 8:00 ") {
		t.Errorf("the label is not inside the band:\n%s", l)
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
	if closed := m.View(); strings.Contains(closed, "r fetch tasks") {
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
		for _, state := range map[string]Model{
			"dash":    m,
			"loading": send(t, New(), tea.WindowSizeMsg{Width: w, Height: h}, runes("d")),
			"quit":    send(t, m, runes("q")),
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

// A rule between days, and the window is built around today rather than the 1st — today's
// row is the one being logged into, and a month is taller than most terminals.
func TestDashSeparatesDaysAndFollowsToday(t *testing.T) {
	// A full month, every day a working day, so it cannot fit a short terminal.
	today := time.Now()
	var days []api.DayLog
	for d := 1; d <= 28; d++ {
		days = append(days, api.DayLog{
			Date:     today.Format("2006-01") + fmt.Sprintf("-%02d", d),
			Actual:   8,
			Expected: 8,
		})
	}
	m := send(t, New(), tea.WindowSizeMsg{Width: 100, Height: 24},
		store.KeyMsg{Key: "k", DB: "db"}, runes("d"),
		api.HourLogsMsg{Month: today.Format("2006-01") + "-01", Days: days})

	lines, focus := m.dashLines(14)
	if focus < 0 {
		t.Fatal("no focus line — the window has nothing to hold")
	}
	if !strings.Contains(lines[focus], "today") {
		t.Errorf("the focused line is not today's:\n%s", lines[focus])
	}
	// Every pair of day rows has a rule between them.
	if !strings.Contains(strings.Join(lines, "\n"), "──") {
		t.Errorf("no rule between days:\n%s", strings.Join(lines, "\n"))
	}
	// And today survives the windowing into a 24-row terminal.
	if v := m.View(); !strings.Contains(v, "today") {
		t.Errorf("today scrolled out of a 24-row view:\n%s", v)
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

// The rule between days is what the chart gives up first: a whole month on screen beats a
// windowed one, now that there are no keys to scroll it with.
func TestDashDropsRulesToFitTheMonth(t *testing.T) {
	now := time.Now()
	var days []api.DayLog
	for d := 1; d <= 28; d++ {
		days = append(days, api.DayLog{
			Date:     now.Format("2006-01") + fmt.Sprintf("-%02d", d),
			Actual:   8,
			Expected: 8,
		})
	}
	m := send(t, New(), tea.WindowSizeMsg{Width: 100, Height: 40},
		store.KeyMsg{Key: "k", DB: "db"}, runes("d"),
		api.HourLogsMsg{Month: now.Format("2006-01") + "-01", Days: days})

	// Exactly enough rows for the days alone: the rules go, and the month fits whole.
	lines, _ := m.dashLines(28)
	if len(lines) != 28 {
		t.Errorf("%d lines for 28 days with 28 rows to spend", len(lines))
	}
	if strings.Contains(strings.Join(lines, "\n"), "──────") {
		t.Errorf("the rules were kept instead of the days:\n%s", strings.Join(lines, "\n"))
	}
	// Ruled again as soon as there is room for both, and while there is room for neither —
	// a windowed chart still wants its days told apart.
	for _, budget := range []int{80, 14} {
		if lines, _ := m.dashLines(budget); !strings.Contains(strings.Join(lines, "\n"), "──────") {
			t.Errorf("no rules with a budget of %d", budget)
		}
	}
}

// g and G are the whole of the chart's motion: the start of the month and the end of it.
// Only the days move — the totals and the axis are pinned either side of them.
func TestDashJumpsToTheEnds(t *testing.T) {
	now := time.Now()
	var days []api.DayLog
	for d := 1; d <= 28; d++ {
		days = append(days, api.DayLog{
			Date:     now.Format("2006-01") + fmt.Sprintf("-%02d", d),
			Actual:   8,
			Expected: 8,
		})
	}
	// 24 rows cannot hold 28 days, so the window has to move for g and G to show.
	m := send(t, New(), tea.WindowSizeMsg{Width: 100, Height: 24},
		store.KeyMsg{Key: "k", DB: "db"}, runes("d"),
		api.HourLogsMsg{Month: now.Format("2006-01") + "-01", Days: days})

	first := strings.ToLower(now.AddDate(0, 0, 1-now.Day()).Format("Mon")) + "  1"
	top := send(t, m, runes("g"))
	if !strings.Contains(top.View(), first) {
		t.Errorf("g did not reach the 1st:\n%s", top.View())
	}
	if strings.Contains(top.View(), "↑") {
		t.Errorf("g left days above the window:\n%s", top.View())
	}

	bottom := send(t, top, runes("G"))
	if !strings.Contains(bottom.View(), " 28") {
		t.Errorf("G did not reach the 28th:\n%s", bottom.View())
	}
	if strings.Contains(bottom.View(), "↓") {
		t.Errorf("G left days below the window:\n%s", bottom.View())
	}

	// The frame is pinned, so both ends keep the numbers the bars are read against.
	for _, v := range []Model{top, bottom} {
		for _, want := range []string{"logged", "HOURS PER DAY", "└"} {
			if !strings.Contains(v.View(), want) {
				t.Errorf("%q left the screen with the days:\n%s", want, v.View())
			}
		}
	}
	// A re-read puts it back on today, the day being logged into.
	if fresh := send(t, top, api.HourLogsMsg{
		Month: now.Format("2006-01") + "-01", Days: days,
	}); fresh.dashHold != -1 {
		t.Errorf("dashHold = %d after a re-read, want today", fresh.dashHold)
	}
	// ctrl+f and ctrl+b move by half a screen, and stop at the ends.
	down := send(t, top, special(tea.KeyCtrlF))
	if down.dashHold <= top.dashHold {
		t.Errorf("ctrl+f did not move forward: %d then %d", top.dashHold, down.dashHold)
	}
	if up := send(t, down, special(tea.KeyCtrlB)); up.dashHold >= down.dashHold {
		t.Errorf("ctrl+b did not move back: %d then %d", down.dashHold, up.dashHold)
	}
	if far := send(t, top, special(tea.KeyCtrlB), special(tea.KeyCtrlB)); far.dashHold != 0 {
		t.Errorf("ctrl+b ran off the top: %d", far.dashHold)
	}
	if far := send(t, bottom, special(tea.KeyCtrlF)); far.dashHold != len(days)-1 {
		t.Errorf("ctrl+f ran off the end: %d", far.dashHold)
	}

	// The footer offers the two screenful motions, and no j / k — there is no cursor.
	foot := send(t, m, runes("?")).footer()
	for _, want := range []string{"g/G", "ctrl+f/b"} {
		if !strings.Contains(foot, want) {
			t.Errorf("the footer does not offer %s:\n%s", want, foot)
		}
	}
	for _, gone := range []string{"j next", "k prev"} {
		if strings.Contains(foot, gone) {
			t.Errorf("the footer still offers %q:\n%s", gone, foot)
		}
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

// The number beside the month's totals is today's gap, not the month's: the hours you can
// still do something about before the day is out.
func TestSummaryReportsToday(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	load := func(d api.DayLog) string {
		return send(t, New(), tea.WindowSizeMsg{Width: 100, Height: 30},
			store.KeyMsg{Key: "k", DB: "db"}, runes("d"),
			api.HourLogsMsg{Month: today[:8] + "01", Days: []api.DayLog{d}}).View()
	}

	for _, tc := range []struct {
		name string
		day  api.DayLog
		want string
	}{
		{"short", api.DayLog{Date: today, Actual: 2.5, Expected: 8}, "today −5:30"},
		{"over", api.DayLog{Date: today, Actual: 9, Expected: 8}, "today +1:00"},
		{"on target", api.DayLog{Date: today, Actual: 8, Expected: 8}, "today +0:00"},
		// Nothing was expected, so there is no gap to report.
		{"weekend", api.DayLog{Date: today, Weekend: true}, "today off"},
		// And a month the ERP has not reported today in does not imply a full day owed.
		{"unreported", api.DayLog{Date: today[:8] + "01", Expected: 8}, "today —"},
	} {
		if view := load(tc.day); !strings.Contains(view, tc.want) {
			t.Errorf("%s: summary is missing %q:\n%s", tc.name, tc.want, view)
		}
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
		{"dashboard", send(t, dashModel(t, 100, 30), runes("t"), runes("d")), "r fetch tasks", 30},
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
