package model

import "github.com/charmbracelet/bubbles/key"

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
}

var keys = keyMap{
	Down:       key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j", "next")),
	Up:         key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k", "prev")),
	Expand:     key.NewBinding(key.WithKeys("l", "right", "enter"), key.WithHelp("l/enter", "expand")),
	Collapse:   key.NewBinding(key.WithKeys("h", "left"), key.WithHelp("h", "collapse")),
	Jump:       key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "date jump")),
	Search:     key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "search")),
	Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	Edit:       key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "edit row")),
	Add:        key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add entry")),
	Delete:     key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete row")),
	Back:       key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	Next:       key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next field")),
	Prev:       key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev field")),
	ClearField: key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "clear field")),
	Accept:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "commit")),
	Cancel:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	Yes:        key.NewBinding(key.WithKeys("y", "enter"), key.WithHelp("y", "yes")),
	// Quitting is not something enter should do by reflex, so it wants the letter.
	YesOnly:     key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "quit")),
	No:          key.NewBinding(key.WithKeys("n", "esc"), key.WithHelp("n", "no")),
	ClearQuery:  key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "clear + collapse")),
	Focus:       key.NewBinding(key.WithKeys("esc", "enter"), key.WithHelp("enter", "task list")),
	Refresh:     key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "fetch tasks")),
	SetKey:      key.NewBinding(key.WithKeys("K"), key.WithHelp("K", "api key")),
	ClearSearch: key.NewBinding(key.WithKeys("ctrl+l"), key.WithHelp("ctrl+l", "clear + search")),
}

// help is the footer set for a mode.
func (k keyMap) help(m Mode) []key.Binding {
	switch m {
	case ModeSearch:
		return []key.Binding{k.Focus, k.ClearQuery}
	case ModeList:
		return []key.Binding{k.Down, k.Up, k.Expand, k.Collapse, k.Jump, k.Refresh, k.SetKey,
			k.Search, k.ClearSearch, k.Quit}
	case ModeTable:
		return []key.Binding{k.Down, k.Up, k.Edit, k.Add, k.Delete, k.Jump, k.Collapse, k.Back}
	case ModeInsert:
		return []key.Binding{k.Next, k.Prev, k.ClearField, k.Accept, k.Cancel}
	case ModeJump:
		return []key.Binding{key.NewBinding(key.WithHelp("0-9 /", "day")), k.Accept, k.Cancel}
	case ModeConfirm:
		return []key.Binding{k.Yes, k.No} // the quit prompt swaps in YesOnly, see confirmKeys
	case ModeAuth:
		return []key.Binding{
			key.NewBinding(key.WithHelp("enter", "save key + fetch")),
			key.NewBinding(key.WithHelp("esc", "work offline")),
		}
	}
	return nil
}
