// Package theme holds every color and style in the app. Nothing else picks colors.
package theme

import "github.com/charmbracelet/lipgloss"

const (
	Accent      = lipgloss.Color("#FFC000")
	Chrome      = lipgloss.Color("#151520")
	Destructive = lipgloss.Color("#E13400")
	Complete    = lipgloss.Color("#12CC63")
	Link        = lipgloss.Color("#01B9AE")
	Muted       = lipgloss.Color("#8A8F99")
	// Rule draws the boxes and separators: visible on black, quieter than text.
	Rule = lipgloss.Color("#2B2B3A")
)

var focusBorder = lipgloss.Border{Left: "▏"}

var (
	Title = lipgloss.NewStyle().Bold(true)
	// TitleFocus is the task the cursor is on. Same weight, accent color, so the
	// border and background of Focus are not the only thing carrying the cursor.
	TitleFocus = Title.Foreground(Accent)
	Tag        = lipgloss.NewStyle().Foreground(Link)
	Dim        = lipgloss.NewStyle().Foreground(Muted)
	Mode       = lipgloss.NewStyle().Foreground(Accent).Bold(true)
	// Prompt is the "> " in front of the search field.
	Prompt = lipgloss.NewStyle().Foreground(Accent).Bold(true)
	Hint   = lipgloss.NewStyle().Foreground(Muted)
	Err    = lipgloss.NewStyle().Foreground(Destructive)
	Ok     = lipgloss.NewStyle().Foreground(Complete)

	// Focus and Blur pair up: border plus padding on one side, plain padding on
	// the other, both two cells wide, so a row never shifts when focus moves.
	Focus = lipgloss.NewStyle().
		Border(focusBorder, false, false, false, true).
		BorderForeground(Accent).
		Background(Chrome).
		PaddingLeft(1)
	Blur = lipgloss.NewStyle().PaddingLeft(2)

	Bar     = lipgloss.NewStyle().Foreground(Accent)
	BarDone = lipgloss.NewStyle().Foreground(Complete)
	BarRest = lipgloss.NewStyle().Foreground(Muted)

	Header = lipgloss.NewStyle().Foreground(Muted).Bold(true)

	// SearchBox frames the query field across the top; SearchBoxFocus is the same
	// frame in the accent color, so the box says where the keys are going.
	SearchBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Rule).
			Padding(0, 1)
	SearchBoxFocus = SearchBox.BorderForeground(Accent)
	// Chip is a tag: vertical bars only, so a task stays one line tall.
	Chip = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder(), false, true).
		BorderForeground(Link).
		Foreground(Link).
		Padding(0, 1)
	// Sep is the rule under the header.
	Sep = lipgloss.NewStyle().Foreground(Rule)
	// Total is a task's summed hours, the one number that should catch the eye.
	Total = lipgloss.NewStyle().Bold(true)
	Modal = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Accent).
		Padding(0, 2)
)
