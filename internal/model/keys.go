package model

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// keyMap is the single source of truth for keys. Update matches against these
// bindings and the footer renders from the same set, so help cannot drift.
type keyMap struct {
	Down, Up                       key.Binding
	Expand, Collapse               key.Binding
	Jump, Search, Quit             key.Binding
	Edit, Add, Delete, Back        key.Binding
	Next, Prev, ClearField, Accept key.Binding
	Cancel, Yes, YesOnly, No       key.Binding
	ClearQuery, Focus              key.Binding
	Refresh, SetKey, ClearSearch   key.Binding
	Top, Bottom, HalfDown, HalfUp  key.Binding
	TasksTab, DashTab, TimeTab     key.Binding
	MealTab                        key.Binding
	Help, Clock, NewLeave          key.Binding
	PrevMonth, NextMonth           key.Binding
	Cycle                          key.Binding
}

// keys is what the handlers and the footer read. ApplyKeys is its only writer, called
// once from main before the program starts, so nothing races it.
var keys = defaultKeys()

func defaultKeys() keyMap {
	return keyMap{
		Down:     key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j", "next")),
		Up:       key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k", "prev")),
		Expand:   key.NewBinding(key.WithKeys("l", "right"), key.WithHelp("l", "expand")),
		Collapse: key.NewBinding(key.WithKeys("h", "left"), key.WithHelp("h", "collapse")),
		Jump:     key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "date jump")),
		Search:   key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "search")),
		Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Edit:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "edit row")),
		Add:      key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add entry")),
		// x, not d: d switches to the dashboard tab from every mode, and a key that
		// deletes an hour log must not be one keystroke away from a tab switch.
		Delete:     key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "delete row")),
		Back:       key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Next:       key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next field")),
		Prev:       key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev field")),
		ClearField: key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "clear field")),
		Accept:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "commit")),
		Cancel:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		Yes:        key.NewBinding(key.WithKeys("y", "enter"), key.WithHelp("y", "yes")),
		// Destructive prompts want the letter, never enter by reflex. The prompt says
		// what is about to happen, so this hint stays generic.
		YesOnly:    key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "yes")),
		No:         key.NewBinding(key.WithKeys("n", "esc"), key.WithHelp("n", "no")),
		ClearQuery: key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "clear + collapse")),
		Focus:      key.NewBinding(key.WithKeys("esc", "enter"), key.WithHelp("enter", "task list")),
		Refresh:    key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "fetch tasks")),
		SetKey:     key.NewBinding(key.WithKeys("K"), key.WithHelp("K", "api key")),
		// ctrl+l is unusable in practice: tmux and vim grab it. ctrl+u it is, which also
		// matches what ctrl+u already does inside the search field.
		ClearSearch: key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "clear search")),

		// vim motions. The paired keys carry no help of their own; the first of each
		// pair spells both out, so the footer stays one line per idea.
		Top:    key.NewBinding(key.WithKeys("g"), key.WithHelp("g/G", "top/bottom")),
		Bottom: key.NewBinding(key.WithKeys("G")),
		// Half up is ctrl+b, not vim's ctrl+u, because ctrl+u clears the query here, so
		// half down is ctrl+f to keep the pair symmetric.
		HalfDown: key.NewBinding(key.WithKeys("ctrl+f"), key.WithHelp("ctrl+f/b", "half page")),
		HalfUp:   key.NewBinding(key.WithKeys("ctrl+b")),

		// Tabs, from anywhere that is not typing into a field. Delete-row moved to x so
		// d can mean one thing everywhere. The digits are aliases in bar order, the way
		// btop numbers its screens; the help key stays the letter, which is the one
		// picked out inside the label.
		TasksTab: key.NewBinding(key.WithKeys("t", "1"), key.WithHelp("t", "tasks")),
		DashTab:  key.NewBinding(key.WithKeys("d", "2"), key.WithHelp("d", "dashboard")),
		// o, not t: the spec's own "t today" cannot have this key — t is the tasks tab from
		// every screen, and a tab key that means one thing everywhere is worth more than a
		// motion the calendar can do without, since it opens on today anyway.
		TimeTab: key.NewBinding(key.WithKeys("o", "3"), key.WithHelp("o", "timeoff")),
		// m, the initial of the word, free of every other tab key.
		MealTab: key.NewBinding(key.WithKeys("m", "4"), key.WithHelp("m", "meal")),

		// The footer's key list, off by default and toggled from anywhere that is not
		// typing. ? is the one key the closed footer still advertises, so the rest are
		// always one keystroke away without costing a line of every screen.
		Help: key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "keys")),

		// The ERP's own clock, on the dashboard. One key for both directions, because
		// attendance_manual is one toggle; checking out asks first, so a stray c cannot
		// close the day. The help label stays short — the open footer is already two
		// lines at 80 columns, and a third costs the chart a day.
		Clock: key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "clock")),

		// The time off form. n opens it and focuses the leave type; nothing after the label
		// is on screen until it does, so the line costs one row either way.
		NewLeave: key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new timeoff")),
		// What changes a dropdown inside that form. space as well as j/k, since a dropdown
		// is a button as much as it is a list.
		Cycle: key.NewBinding(key.WithKeys("j", "k", " ", "down", "up"),
			key.WithHelp("j/k", "change")),

		// Month navigation, dashboard only. Paired like g/G and ctrl+f/b: the first key
		// carries the help label, the second rides along unlabelled.
		PrevMonth: key.NewBinding(key.WithKeys("<"), key.WithHelp("</>", "month")),
		NextMonth: key.NewBinding(key.WithKeys(">")),
	}
}

// ApplyKeys rebinds actions from the [keys] table of the config file. It writes the
// package var, so main calls it once before the program runs — after that the
// handlers and the footer only read.
//
// Each action keeps its help description and takes its help key label from the first
// key given, so the footer follows a rebind without a second place to edit. An empty
// list unbinds the action.
func ApplyKeys(overrides map[string][]string) error {
	return applyTo(&keys, overrides)
}

// tabKeys is the per-tab keymaps: the global one with that tab's own overrides on top.
// tabClaimed remembers which actions a tab set explicitly, which is what lets one of them
// beat a tab key on that screen and nowhere else.
var (
	tabKeys    = map[Tab]keyMap{}
	tabClaimed = map[Tab]map[string]bool{}
)

// TabNames is the name each tab answers to in the config file, in bar order.
func TabNames() []string { return []string{"tasks", "dash", "time", "meal"} }

func tabByName(name string) (Tab, bool) {
	switch name {
	case "tasks":
		return TabTasks, true
	case "dash":
		return TabDash, true
	case "time":
		return TabTime, true
	case "meal":
		return TabMeal, true
	}
	return 0, false
}

// ApplyTabKeys rebinds actions for one screen only, from a `[keys.<tab>]` table.
//
// A tab's own binding is what that screen reads, and everything it does not name falls
// through to the global keymap — so `[keys.meal] delete = ["d"]` puts cancel on d there while
// x still deletes a timesheet row. It is called once from main, after ApplyKeys, since the
// per-tab maps are built on top of the global one.
func ApplyTabKeys(per map[string]map[string][]string) error {
	tabKeys, tabClaimed = map[Tab]keyMap{}, map[Tab]map[string]bool{}
	for name, overrides := range per {
		tab, ok := tabByName(name)
		if !ok {
			return fmt.Errorf("keys.%s is not a screen — try one of: %s",
				name, strings.Join(TabNames(), ", "))
		}
		k := keys // a copy: keyMap is all values, so this cannot write the global one
		if err := applyTo(&k, overrides); err != nil {
			return err
		}
		tabKeys[tab] = k
		claimed := map[string]bool{}
		for action := range overrides {
			claimed[action] = true
		}
		tabClaimed[tab] = claimed
	}
	return nil
}

// keysFor is the keymap a screen reads: its own, or the global one when it has no overrides.
func keysFor(t Tab) keyMap {
	if k, ok := tabKeys[t]; ok {
		return k
	}
	return keys
}

// claims says whether this tab bound one of its own keys to msg. Those are matched before the
// tab keys, so a screen that deliberately takes `d` for its own action keeps it — and loses
// the dashboard shortcut there, which is what asking for it means.
func claims(t Tab, msg tea.KeyMsg) bool {
	v := reflect.ValueOf(keysFor(t))
	for action := range tabClaimed[t] {
		f := v.FieldByName(fieldName(action))
		if !f.IsValid() {
			continue
		}
		if key.Matches(msg, f.Interface().(key.Binding)) {
			return true
		}
	}
	return false
}

func applyTo(dst *keyMap, overrides map[string][]string) error {
	v := reflect.ValueOf(dst).Elem()
	for action, binds := range overrides {
		field := v.FieldByName(fieldName(action))
		if !field.IsValid() {
			return fmt.Errorf("keys.%s is not an action — try one of: %s",
				action, strings.Join(Actions(), ", "))
		}
		old := field.Interface().(key.Binding)
		opts := []key.BindingOpt{key.WithKeys(binds...)}
		switch {
		case len(binds) == 0:
			// Unbound: no keys, and no hint for a key that does nothing.
		case len(old.Keys()) > 0 && binds[0] == old.Keys()[0]:
			// The primary key did not move, so keep the label as written. This is what
			// preserves the paired hints — "g/G", "ctrl+f/b" — for a config file that
			// lists every action, which is exactly what --print-keys writes.
			opts = append(opts, key.WithHelp(old.Help().Key, old.Help().Desc))
		default:
			opts = append(opts, key.WithHelp(binds[0], old.Help().Desc))
		}
		field.Set(reflect.ValueOf(key.NewBinding(opts...)))
	}
	return nil
}

// CheckKeys refuses a keymap whose actions are shadowed by the tab keys.
//
// The tab keys and `?` are matched **before** the per-tab handlers, so an action bound to one
// of them can never fire: `delete = ["d"]` puts the dashboard on the key and leaves x-deletes
// -a-row — and cancel-a-meal — unreachable, while the footer honestly advertises `d` for
// them. That is worse than a message on stderr, which is why a misspelled key is refused here
// too: a keymap you cannot drive should not reach the alt screen.
//
// Rebinding the tab keys themselves is fine, and so is anything the typing modes protect —
// only the actions those handlers reach are checked.
func CheckKeys() error {
	tabs := map[string]string{}
	for _, t := range []struct {
		name string
		b    key.Binding
	}{
		{"tasks_tab", keys.TasksTab}, {"dash_tab", keys.DashTab},
		{"time_tab", keys.TimeTab}, {"meal_tab", keys.MealTab}, {"help", keys.Help},
	} {
		for _, k := range t.b.Keys() {
			tabs[k] = t.name
		}
	}

	// Only the global map is checked: a per-tab override that takes a tab key does so on
	// purpose, and only on that one screen.
	v := reflect.ValueOf(keys)
	tt := v.Type()
	var clashes []string
	for i := range tt.NumField() {
		name := actionName(tt.Field(i).Name)
		if _, isTab := map[string]bool{"tasks_tab": true, "dash_tab": true,
			"time_tab": true, "meal_tab": true, "help": true}[name]; isTab {
			continue
		}
		for _, k := range v.Field(i).Interface().(key.Binding).Keys() {
			if owner, taken := tabs[k]; taken {
				clashes = append(clashes, fmt.Sprintf("keys.%s = %q is already %s",
					name, k, owner))
			}
		}
	}
	if len(clashes) == 0 {
		return nil
	}
	sort.Strings(clashes)
	return fmt.Errorf("%s\nthe tab keys are matched first, so those actions could never fire",
		strings.Join(clashes, "\n"))
}

// Actions is every name the [keys] table accepts, sorted.
func Actions() []string {
	t := reflect.TypeOf(keyMap{})
	out := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		out = append(out, actionName(t.Field(i).Name))
	}
	sort.Strings(out)
	return out
}

// keysTOMLHeader explains the file to whoever opens it. It lives with the defaults it
// documents, so there is no second copy of the keymap anywhere to fall out of date —
// `tsk --print-keys` is the only source.
const keysTOMLHeader = `# tsk keymap — every default, so this file changes nothing until you edit it.
#
# Write it where tsk reads it, then edit what you want:
#
#     mkdir -p ~/.config/tsk && tsk --print-keys > ~/.config/tsk/config.toml
#
# tsk reads ~/.config/tsk/config.toml ($XDG_CONFIG_HOME/tsk/config.toml) and nothing
# else. Keep just the lines you want to change: anything absent keeps its default, and
# no file at all is fine.
#
# An empty list unbinds an action (quit = []); ctrl+c always quits. Keys are spelled
# bubbletea's way: one character, ctrl+/alt+/shift+ and one, or enter esc tab
# shift+tab space backspace delete up down left right home end pgup pgdown. A
# misspelling is refused at startup rather than silently never matching.
#
# Per screen: a [keys.<tab>] table rebinds an action on that screen only, and leaves it
# alone everywhere else. The screens are tasks, dash, time and meal.
#
#     [keys.meal]
#     delete = ["d"]        # d cancels the day's meals here; x still deletes a row
#
# A screen's own binding is matched before the tab keys, so a table like that trades the
# dashboard shortcut for it on that screen — and nowhere else. Globally the tab keys win,
# and a global binding that collides with one is refused at startup.
[keys]
`

// tabKeysTOMLFooter shows the per-screen tables the same way, commented, so the file that
// --print-keys writes documents them without changing any binding.
const tabKeysTOMLFooter = `
# Per-screen overrides. Uncomment a table and the actions you want it to change.
#
# [keys.tasks]
# [keys.dash]
# [keys.time]
# [keys.meal]
# delete = ["d"]
`

// KeysTOML is the live keymap as a [keys] table, so `tsk --print-keys` can seed the
// config file and the documented defaults cannot drift from defaultKeys.
func KeysTOML() string {
	v := reflect.ValueOf(keys)
	t := v.Type()

	width := 0
	names := make([]string, t.NumField())
	for i := range t.NumField() {
		names[i] = actionName(t.Field(i).Name)
		if len(names[i]) > width {
			width = len(names[i])
		}
	}

	var b strings.Builder
	b.WriteString(keysTOMLHeader)
	for i := range t.NumField() {
		quoted := make([]string, 0, 4)
		for _, k := range v.Field(i).Interface().(key.Binding).Keys() {
			quoted = append(quoted, fmt.Sprintf("%q", k))
		}
		fmt.Fprintf(&b, "%-*s = [%s]\n", width, names[i], strings.Join(quoted, ", "))
	}
	b.WriteString(tabKeysTOMLFooter)
	return b.String()
}

// actionName is the config spelling of a field: HalfDown -> half_down.
func actionName(field string) string {
	var b strings.Builder
	for i, r := range field {
		if i > 0 && unicode.IsUpper(r) {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// fieldName is the reverse: half_down -> HalfDown.
func fieldName(action string) string {
	parts := strings.Split(action, "_")
	for i, p := range parts {
		if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}

// help is the footer set for a mode.
func (k keyMap) help(m Mode) []key.Binding {
	switch m {
	case ModeSearch:
		return []key.Binding{k.Focus, k.ClearQuery}
	case ModeList:
		return []key.Binding{k.Down, k.Up, k.Top, k.HalfDown, k.Expand, k.Collapse, k.Jump,
			k.DashTab, k.Refresh, k.SetKey, k.Search, k.ClearSearch, k.Quit}
	case ModeTable:
		return []key.Binding{k.Down, k.Up, k.Top, k.HalfDown, k.Edit, k.Add, k.Delete,
			k.Jump, k.Collapse, k.ClearSearch, k.Back, k.Quit}
	case ModeInsert:
		return []key.Binding{k.Next, k.Prev, k.ClearField, k.Accept, k.Cancel}
	case ModeForm:
		return []key.Binding{k.Next, k.Prev, k.Cycle, k.ClearField, k.Accept, k.Cancel}
	case ModeJump:
		return []key.Binding{
			key.NewBinding(key.WithHelp("12 · 12/7 · 12/7/26", "date")),
			key.NewBinding(key.WithHelp("enter", "go to it, or list the day")),
			k.Cancel,
		}
	case ModeDay:
		return []key.Binding{key.NewBinding(key.WithHelp("esc", "close"))}
	case ModeLeaves:
		return []key.Binding{key.NewBinding(key.WithHelp(k.Back.Help().Key, "close"))}
	case ModeConfirm:
		return []key.Binding{k.Yes, k.No} // destructive prompts swap in YesOnly, see confirmKeys
	case ModeAuth:
		return []key.Binding{
			key.NewBinding(key.WithHelp("enter", "save key + fetch")),
			key.NewBinding(key.WithHelp("esc", "work offline")),
		}
	}
	return nil
}
