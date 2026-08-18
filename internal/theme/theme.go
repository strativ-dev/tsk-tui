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

	// Dashboard palette, from the design spec. The three hour colors are thresholds:
	// under 4h, under 8h, on target. The spec draws one-colour bars and tints only the
	// number, warning that red-amber-green is the colourblind trap; the bars carry the
	// colour here by choice, so the redundant signals it relies on stay — the 4h and 8h
	// ticks, the printed number, and the labelled axis.
	HourLow  = lipgloss.Color("#E0574F") // < 4h
	HourMid  = lipgloss.Color("#E0A030") // 4h to < 8h
	HourHigh = lipgloss.Color("#5FBF7F") // >= 8h
	// Bar bodies: a dark wash of each threshold colour, so a bar reads as a filled band with
	// its number and its edges in the light colour on top of it — the design's own look, and
	// the reason the bar needs no glyph pattern inside it.
	BandLow  = lipgloss.Color("#3A1512")
	BandMid  = lipgloss.Color("#3A2C0C")
	BandHigh = lipgloss.Color("#10352A")
	// TrackColor is the unfilled remainder and its threshold ticks.
	TrackColor = lipgloss.Color("#2A2A33")
	// OffBand is a day nothing was expected of.
	OffBand = lipgloss.Color("#1E1E26")
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
	// Sep is the rule under the header; Track draws the empty part of a bar and the
	// threshold ticks inside it.
	Sep   = lipgloss.NewStyle().Foreground(Rule)
	Track = lipgloss.NewStyle().Foreground(TrackColor)

	// Tab bar. Pill is the active tab, reversed out in the accent so the screen you are
	// on is the one thing the primary colour marks up here. The key hint is highlighted
	// inside the word itself: accent on an inactive tab, and dark ink underlined on the
	// pill, where accent on accent would say nothing.
	Pill    = lipgloss.NewStyle().Foreground(Chrome).Background(Accent).Bold(true)
	HintKey = lipgloss.NewStyle().Foreground(Accent).Bold(true)
	PillKey = lipgloss.NewStyle().Foreground(Chrome).Background(Accent).Bold(true).Underline(true)

	// The clock's button, boxed so it reads as the one thing on this screen you can press.
	// Border and label share one colour, the chart's own thresholds: green invites an action
	// not yet taken, amber says a clock is running and wants closing. A plain rule is the
	// state we have not read yet.
	ClockIn  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(HourHigh).Padding(0, 1)
	ClockOut = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(HourMid).Padding(0, 1)
	ClockOff = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(Rule).Padding(0, 1)
	// The label inside the box is white: the border already carries the state, so the words
	// are just the words, and the accent is left to mean one thing — the key. White on black
	// is the strongest ink there is, which is what makes the button read as pressable.
	ClockText = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true)

	// Off is a day the ERP expected nothing of — weekend, holiday, leave. The band
	// spans the whole track on purpose: width itself says "nothing was expected".
	Off = lipgloss.NewStyle().Foreground(Muted).Background(OffBand)

	// The three hour bands. Low/Mid/High colour a number on its own; the Fill trio is
	// the bar itself — a solid band with the hours in dark ink on it, so the label reads
	// inside the bar instead of costing width beside it.
	Low  = lipgloss.NewStyle().Foreground(HourLow)
	Mid  = lipgloss.NewStyle().Foreground(HourMid)
	High = lipgloss.NewStyle().Foreground(HourHigh)

	// Underlined on purpose: the underline runs along the bottom of the band in the light
	// threshold colour, which is what keeps a bar separate from the day stacked directly under
	// it. With the two edges it draws the bar as an outlined box, and it costs no row — a rule
	// between days would cost one each, and the month is laid out in columns to save rows.
	LowFill  = lipgloss.NewStyle().Foreground(HourLow).Background(BandLow).Bold(true).Underline(true)
	MidFill  = lipgloss.NewStyle().Foreground(HourMid).Background(BandMid).Bold(true).Underline(true)
	HighFill = lipgloss.NewStyle().Foreground(HourHigh).Background(BandHigh).Bold(true).Underline(true)
	// DayLabel is a working day's date: white, so the weekends and holidays beside it read as
	// the quiet ones without the working days having to be coloured.
	DayLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	// The key picked out inside a button's own label, as on the tab bar's pill: underlined
	// dark ink, since accent on a light band would fail contrast.
	MidFillKey  = MidFill.Underline(true)
	HighFillKey = HighFill.Underline(true)
	// Total is a task's summed hours, the one number that should catch the eye.
	Total = lipgloss.NewStyle().Bold(true)
	// Match and MatchText are a row a date jump found. The date is reversed out so it
	// stands out among the dim ones; its description takes the accent, since that is
	// what you are scanning for.
	Match     = lipgloss.NewStyle().Foreground(Chrome).Background(Accent).Bold(true)
	MatchText = lipgloss.NewStyle().Foreground(Accent)
	Modal     = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Accent).
			Padding(0, 2)
)
