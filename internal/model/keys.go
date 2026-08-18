package model

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/key"
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
	TasksTab, DashTab              key.Binding
	Help, Clock                    key.Binding
	PrevMonth, NextMonth           key.Binding
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

		// The footer's key list, off by default and toggled from anywhere that is not
		// typing. ? is the one key the closed footer still advertises, so the rest are
		// always one keystroke away without costing a line of every screen.
		Help: key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "keys")),

		// The ERP's own clock, on the dashboard. One key for both directions, because
		// attendance_manual is one toggle; checking out asks first, so a stray c cannot
		// close the day. The help label stays short — the open footer is already two
		// lines at 80 columns, and a third costs the chart a day.
		Clock: key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "clock")),

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
	v := reflect.ValueOf(&keys).Elem()
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
[keys]
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
	case ModeJump:
		return []key.Binding{
			key.NewBinding(key.WithHelp("12 · 12/7 · 12/7/26", "date")),
			key.NewBinding(key.WithHelp("enter", "go to it, or list the day")),
			k.Cancel,
		}
	case ModeDay:
		return []key.Binding{key.NewBinding(key.WithHelp("esc", "close"))}
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
