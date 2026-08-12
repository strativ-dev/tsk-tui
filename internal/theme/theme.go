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
)

var focusBorder = lipgloss.Border{Left: "▏"}

var (
	Title = lipgloss.NewStyle().Bold(true)
	Tag   = lipgloss.NewStyle().Foreground(Link)
	Dim   = lipgloss.NewStyle().Foreground(Muted)
	Mode  = lipgloss.NewStyle().Foreground(Accent).Bold(true)
	Hint  = lipgloss.NewStyle().Foreground(Muted)
	Err   = lipgloss.NewStyle().Foreground(Destructive)
	Ok    = lipgloss.NewStyle().Foreground(Complete)

	// Focus and Blur pair up: both cost one leading cell, so rows never shift.
	Focus = lipgloss.NewStyle().
		Border(focusBorder, false, false, false, true).
		BorderForeground(Accent).
		Background(Chrome)
	Blur = lipgloss.NewStyle().PaddingLeft(1)

	Bar     = lipgloss.NewStyle().Foreground(Accent)
	BarDone = lipgloss.NewStyle().Foreground(Complete)
	BarRest = lipgloss.NewStyle().Foreground(Muted)

	Header = lipgloss.NewStyle().Foreground(Muted).Bold(true)
	Modal  = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Accent).
		Padding(0, 2)
)
