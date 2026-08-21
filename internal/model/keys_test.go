package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/tasnimAlam/tsk/internal/api"
	"github.com/tasnimAlam/tsk/internal/store"
)

// rebind applies overrides and puts the defaults back afterwards: keys is a package
// var, so a test that leaves it changed poisons every test after it.
func rebind(t *testing.T, overrides map[string][]string) error {
	t.Helper()
	t.Cleanup(func() { keys = defaultKeys() })
	return ApplyKeys(overrides)
}

func TestApplyKeys(t *testing.T) {
	if err := rebind(t, map[string][]string{
		"half_down": {"ctrl+d"},
		"down":      {"n"},
		"quit":      {},
	}); err != nil {
		t.Fatal(err)
	}

	if got := strings.Join(keys.HalfDown.Keys(), ","); got != "ctrl+d" {
		t.Errorf("half_down keys = %q, want ctrl+d", got)
	}
	// The description is the part the config does not carry, so it has to survive.
	if got := keys.Down.Help(); got.Key != "n" || got.Desc != "next" {
		t.Errorf("down help = %+v, want {n next}", got)
	}
	// An empty list unbinds: nothing can match it any more.
	if keys.Quit.Enabled() && len(keys.Quit.Keys()) != 0 {
		t.Errorf("quit keys = %v, want none", keys.Quit.Keys())
	}
	// Untouched actions keep their defaults.
	if got := strings.Join(keys.HalfUp.Keys(), ","); got != "ctrl+b" {
		t.Errorf("half_up keys = %q, want the default ctrl+b", got)
	}
}

// --print-keys writes every action, so a config seeded from it overrides all of them.
// Rebinding an action to the key it already had must not rewrite its help label, or
// that file alone would degrade the paired hints to "g top/bottom" and
// "ctrl+f half page".
func TestUnchangedPrimaryKeyKeepsItsLabel(t *testing.T) {
	if err := rebind(t, map[string][]string{
		"top":       {"g"},
		"half_down": {"ctrl+f"},
		"down":      {"j", "down"},
	}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		got  key.Binding
		want string
	}{
		{"top", keys.Top, "g/G"},
		{"half_down", keys.HalfDown, "ctrl+f/b"},
		{"down", keys.Down, "j"},
	} {
		if c.got.Help().Key != c.want {
			t.Errorf("%s label = %q, want %q", c.name, c.got.Help().Key, c.want)
		}
	}
}

// A name that is not an action has to be reported, not ignored — a silently dropped
// line looks exactly like a keymap that does not work.
func TestApplyKeysRejectsUnknownAction(t *testing.T) {
	err := rebind(t, map[string][]string{"halfdown": {"ctrl+d"}})
	if err == nil {
		t.Fatal("ApplyKeys accepted an action that does not exist")
	}
	if !strings.Contains(err.Error(), "halfdown") || !strings.Contains(err.Error(), "half_down") {
		t.Errorf("error = %q, want the bad name and the valid ones", err)
	}
}

// The reflection has to write the same field the handlers read, so drive one.
func TestRebindDrivesTheHandler(t *testing.T) {
	if err := rebind(t, map[string][]string{"down": {"n"}}); err != nil {
		t.Fatal(err)
	}
	tasks := []store.Task{{ID: 1, Title: "one"}, {ID: 2, Title: "two"}}
	m := send(t, New(), tea.WindowSizeMsg{Width: 120, Height: 30}, store.LoadedMsg{Tasks: tasks})

	if m = send(t, m, runes("n")); m.cursor != 1 {
		t.Errorf("n left the cursor at %d, want 1 — the rebind did not reach the handler", m.cursor)
	}
	if m = send(t, m, runes("j")); m.cursor != 1 {
		t.Errorf("j still moves the cursor (now %d) after being unbound", m.cursor)
	}
	// The footer renders from the same binding, so it moves too.
	if v := send(t, m, runes("?")).View(); !strings.Contains(v, "n next") {
		t.Errorf("footer does not show the rebound key:\n%s", v)
	}
}

// Actions is what an error message offers and what --print-keys writes, so the two
// spellings have to round-trip.
func TestActionNamesRoundTrip(t *testing.T) {
	for _, a := range Actions() {
		if got := actionName(fieldName(a)); got != a {
			t.Errorf("%q became %q", a, got)
		}
	}
	if got := actionName("HalfDown"); got != "half_down" {
		t.Errorf("actionName(HalfDown) = %q", got)
	}
	if got := fieldName("clear_search"); got != "ClearSearch" {
		t.Errorf("fieldName(clear_search) = %q", got)
	}
}

// --print-keys has to produce a file the loader accepts, or seeding a config from the
// binary hands the user something broken.
func TestKeysTOMLIsValidConfig(t *testing.T) {
	out := KeysTOML()
	// The columns are padded for reading, so compare with the runs of spaces collapsed.
	var flat []string
	for _, l := range strings.Split(out, "\n") {
		flat = append(flat, strings.Join(strings.Fields(l), " "))
	}
	squashed := strings.Join(flat, "\n")
	for _, want := range []string{"[keys]", `half_down = ["ctrl+f"]`, `quit = ["q", "ctrl+c"]`} {
		if !strings.Contains(squashed, want) {
			t.Errorf("--print-keys output has no %s:\n%s", want, out)
		}
	}
	// Every line under [keys] names a real action.
	valid := map[string]bool{}
	for _, a := range Actions() {
		valid[a] = true
	}
	for _, line := range strings.Split(out, "\n") {
		name, _, ok := strings.Cut(line, " =")
		if !ok || strings.HasPrefix(line, "#") {
			continue
		}
		if !valid[strings.TrimSpace(name)] {
			t.Errorf("%q is not an action", strings.TrimSpace(name))
		}
	}
}

// A [keys.<tab>] table rebinds an action on that screen and leaves it alone everywhere else.
func TestPerTabKeysOnlyBindThatTab(t *testing.T) {
	t.Cleanup(func() {
		keys = defaultKeys()
		tabKeys, tabClaimed = map[Tab]keyMap{}, map[Tab]map[string]bool{}
	})
	if err := ApplyTabKeys(map[string]map[string][]string{
		"meal": {"delete": {"D"}},
	}); err != nil {
		t.Fatalf("ApplyTabKeys: %v", err)
	}

	if got := keysFor(TabMeal).Delete.Keys(); len(got) != 1 || got[0] != "D" {
		t.Errorf("the meal tab's delete = %v, want D", got)
	}
	if got := keysFor(TabTasks).Delete.Keys(); len(got) != 1 || got[0] != "x" {
		t.Errorf("the task list's delete = %v, want the global x", got)
	}
	// Everything the table did not name still comes from the global map.
	if got := keysFor(TabMeal).Refresh.Keys(); len(got) == 0 || got[0] != "R" {
		t.Errorf("the meal tab lost the global refresh: %v", got)
	}
	// And the footer follows it: the help key is read off the binding the screen uses.
	m := mealModel(t, 120, 34)
	if got := m.mealDropHelp().Help().Key; got != "D" {
		t.Errorf("the meal footer advertises %q, want D", got)
	}
	if got := New().k().Delete.Help().Key; got != "x" {
		t.Errorf("the task list footer advertises %q, want x", got)
	}
	// An unknown screen is refused rather than silently ignored.
	if err := ApplyTabKeys(map[string]map[string][]string{"lunch": {"delete": {"d"}}}); err == nil {
		t.Error("an unknown screen was accepted")
	}
}

// A screen that takes a tab key for its own action keeps it there, and only there.
func TestPerTabKeyBeatsTheTabKey(t *testing.T) {
	t.Cleanup(func() {
		keys = defaultKeys()
		tabKeys, tabClaimed = map[Tab]keyMap{}, map[Tab]map[string]bool{}
	})
	if err := ApplyTabKeys(map[string]map[string][]string{
		"meal": {"delete": {"d"}},
	}); err != nil {
		t.Fatalf("ApplyTabKeys: %v", err)
	}

	m := mealCursorOn(t, mealMenuModel(t, 120, 34), mealSoon())
	m.mealBookings = append(m.mealBookings, api.MealBooking{ID: 900, Date: mealSoon(),
		TypeID: m.mealTypes[0].ID, Type: m.mealTypes[0].Name})
	got := send(t, m, runes("d"))
	if got.tab != TabMeal {
		t.Fatalf("d switched to %v — the meal tab claimed that key", got.tab)
	}
	if got.mode != ModeConfirm || got.cKind != confirmDropMeals {
		t.Errorf("d did not cancel the day: mode %v, kind %v", got.mode, got.cKind)
	}
	// On every other screen d is still the dashboard.
	if away := send(t, New(), tea.WindowSizeMsg{Width: 120, Height: 34}, runes("d")); away.tab != TabDash {
		t.Errorf("d on the task list went to %v", away.tab)
	}
}

// The refusal names the way out, since the usual cause is a config file written by an older
// --print-keys, back when the key was not a tab key yet.
func TestKeyCollisionSaysHowToFixIt(t *testing.T) {
	t.Cleanup(func() { keys = defaultKeys() })
	if err := ApplyKeys(map[string][]string{"refresh": {"r"}}); err != nil {
		t.Fatalf("ApplyKeys: %v", err)
	}
	err := CheckKeys()
	if err == nil {
		t.Fatal("refresh on the requisitions tab key was accepted")
	}
	for _, want := range []string{"keys.refresh", "req_tab", "config.toml", "delete the line"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not mention %q:\n%s", want, err)
		}
	}
}

// A global binding that collides with a tab key is refused at startup: the tab keys are
// matched first, so the action could never fire.
func TestGlobalTabKeyCollisionIsRefused(t *testing.T) {
	t.Cleanup(func() { keys = defaultKeys() })
	if err := ApplyKeys(map[string][]string{"delete": {"d"}}); err != nil {
		t.Fatalf("ApplyKeys: %v", err)
	}
	err := CheckKeys()
	if err == nil {
		t.Fatal("delete = d was accepted, and d is the dashboard tab")
	}
	for _, want := range []string{"keys.delete", "dash_tab", "matched first"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	// The defaults themselves are clean, which is what makes the check safe to run always.
	keys = defaultKeys()
	if err := CheckKeys(); err != nil {
		t.Errorf("the compiled-in defaults collide: %v", err)
	}
}
