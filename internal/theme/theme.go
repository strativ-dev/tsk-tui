// Package theme holds every color and style in the app. Nothing else picks colors.
package theme

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	Accent      = lipgloss.Color("#FFC000")
	Chrome      = lipgloss.Color("#151520")
	Destructive = lipgloss.Color("#E13400")
	Complete    = lipgloss.Color("#12CC63")
	Link        = lipgloss.Color("#01B9AE")
	Muted       = lipgloss.Color("#8A8F99")
	// Rule draws the boxes and separators: visible on black, quieter than text.
	Rule = lipgloss.Color("#2B2B3A")
	// White is the strongest ink there is: a button's label, and a mark reversed out of a
	// filled one.
	White = lipgloss.Color("#FFFFFF")

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

	// Time off palette, one colour per leave type, from the Time Off spec. Annual is a
	// violet of its own rather than the accent: focus is accent everywhere else, and a
	// calendar full of accent days beside an accent cursor says nothing.
	LeaveSick      = Destructive
	LeaveCasual    = Link
	LeaveAnnual    = lipgloss.Color("#7C6BE8")
	LeavePaternity = Complete
	// LeaveOther is a type the palette has no colour for. White, not muted: a leave day
	// in muted ink on a dim band is exactly what a weekend looks like.
	LeaveOther = lipgloss.Color("#FFFFFF")

	// The meal calendar's palette, from docs/meal-calendar-palette.html. One hue per meal,
	// warmest first: morning amber, the hot main meal, then something green.
	MealBreakfast = lipgloss.Color("#E8A33D")
	MealLunch     = lipgloss.Color("#DD5F45")
	MealSnacks    = lipgloss.Color("#93C572")
	// The same three hues at ~45% toward the background, from the palette. A day already
	// eaten keeps its meal's colour and loses the brightness: history reads as history, but
	// a month of past days still says which meals were on. The palette earmarks these for
	// staged edits, which are told apart by their dashed glyphs rather than by hue.
	MealBreakfastPast = lipgloss.Color("#8A6428")
	MealLunchPast     = lipgloss.Color("#85392A")
	MealSnacksPast    = lipgloss.Color("#587643")

	// MealOpen is a slot nobody has taken: faint and deliberately hueless, so the booked
	// bars beside it are the only coloured thing on the day.
	MealOpen = lipgloss.Color("#3E3E47")
	// MealQuiet is a day that cannot be acted on — a weekend, a holiday, anything past.
	// Present, but with the hue taken out.
	MealQuiet = lipgloss.Color("#55555F")
	// MealBand is the band behind today's cell: background only, no hue of its own.
	MealBand = lipgloss.Color("#262633")

	// The time off screen's surfaces, from docs/timeoff-styles.md. The calendar body is one
	// surface with a hairline between the month cells; the month in view is tinted with the
	// accent at 4.5%, which is the only thing that colour is doing there.
	Surface   = lipgloss.Color("#151520")
	PanelHold = lipgloss.Color("#201D1F")
	// Raised is the holiday panel and the request line; Strip is the balance boxes.
	Raised = lipgloss.Color("#12121C")
	Strip  = lipgloss.Color("#14141F")
	// WeekendBand is the fill under a weekend or a holiday date — white at 4.5% over the
	// surface — so a day nobody works reads as a filled day rather than as a gap.
	WeekendBand = lipgloss.Color("#20202A")

	// The text ramp, brightest to dimmest. Ink is what sits on the accent or on a leave
	// colour: never white, or the number stops reading against its own badge.
	Ink      = lipgloss.Color("#0B0B10")
	DayInk   = lipgloss.Color("#C9CCD3") // working day numbers, field values, holiday names
	QuietInk = lipgloss.Color("#5E636E") // weekend numbers, placeholders, dropdown arrows
	UnitInk  = lipgloss.Color("#6C717C") // DAYS AVAILABLE, the mode indicator
	WeekInk  = lipgloss.Color("#3E424B") // week numbers
)

// MealColor is the colour of a meal type, matched on its name for the same reason
// LeaveColor is: the ERP's ids are per database and the palette is per meaning. An unknown
// type gets white — it is a meal, whatever the office calls it.
func MealColor(name string) lipgloss.Color {
	name = strings.ToLower(name)
	switch {
	case strings.Contains(name, "break"):
		return MealBreakfast
	case strings.Contains(name, "lunch"):
		return MealLunch
	case strings.Contains(name, "snack"):
		return MealSnacks
	case strings.Contains(name, "iftar"), strings.Contains(name, "dinner"):
		return LeaveAnnual // the violet, the one hue no other meal claims
	}
	return White
}

// MealPastColor is a meal already eaten: the same hue, dimmed. An unknown type falls back
// to the quiet grey, since there is no dim version of a colour we do not have.
func MealPastColor(name string) lipgloss.Color {
	switch MealColor(name) {
	case MealBreakfast:
		return MealBreakfastPast
	case MealLunch:
		return MealLunchPast
	case MealSnacks:
		return MealSnacksPast
	}
	return MealQuiet
}

// MealBooked is a meal that is on: a solid bar in its own colour, the one coloured thing on
// a day.
func MealBooked(c lipgloss.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(c).Bold(true)
}

// OnPanel is anything drawn on a month's panel: the panel's own colour behind it, since a
// background wrapped around a whole line does not survive the first span that sets its own.
func OnPanel(bg lipgloss.Color) lipgloss.Style {
	return lipgloss.NewStyle().Background(bg)
}

// LeaveColor is the colour of a leave type, matched on its name, since the ERP's own type
// ids are per database and the palette is per meaning. An unknown type still gets a
// colour — it is real time off, whatever it is called.
func LeaveColor(name string) lipgloss.Color {
	name = strings.ToLower(name)
	switch {
	case strings.Contains(name, "sick"):
		return LeaveSick
	case strings.Contains(name, "casual"):
		return LeaveCasual
	case strings.Contains(name, "annual"):
		return LeaveAnnual
	case strings.Contains(name, "paternity"), strings.Contains(name, "maternity"):
		return LeavePaternity
	}
	return LeaveOther
}

// LeaveDay is a day taken off: the type's colour as a band with the date in dark ink on
// it, the terminal's version of the spec's filled circle.
func LeaveDay(c lipgloss.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(Ink).Background(c).Bold(true)
}

// LeaveInk is the same colour as text — the balance chips and the holiday panel, where
// there is no band to reverse out of.
func LeaveInk(c lipgloss.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(c)
}

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
	// A pressed button fills **inside its frame**: the content and the padding take the colour
	// the border is drawn in, and the border cells themselves stay unpainted, so the box keeps
	// the shape it has when it is not pressed. Painted onto the border too, the block of colour
	// read as bigger than the button it was standing in for — the same reason the ✓ and ✕ on
	// the request lines stop short of their own frames. The label carries the fill on its own
	// span, since a foreground set inside resets the background the box put behind it.
	ClockOn     = ClockIn.Background(HourHigh)
	ClockOnText = lipgloss.NewStyle().Foreground(White).Background(HourHigh).Bold(true)

	// The new-timeoff line's fields: a rounded box each, as the design draws them, which
	// makes that row three lines tall — it is three lines whether or not the line is open,
	// so revealing the fields moves nothing. Accent when the keys are going into it, which
	// is what the search box's own frame does.
	Field      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(Rule).Padding(0, 1)
	FieldFocus = Field.BorderForeground(Accent)
	// The two buttons at the end of that line carry their own meaning in the frame, as the
	// clock's button does: green commits, red throws away.
	FieldOk   = Field.BorderForeground(Complete)
	FieldDrop = Field.BorderForeground(Destructive)
	// And they fill with that colour while the keys are on them, which is what a button
	// under the pointer does everywhere else. The fill stops **inside** the frame: painted
	// onto the border cells as well it spread a cell past the box on every side and read as
	// a green blob rather than as a pressed button.
	FieldOkOn   = FieldOk.Background(Complete)
	FieldDropOn = FieldDrop.Background(Destructive)
	// The mark inside a filled button: white, and carrying the fill itself — a foreground
	// set on an inner span resets the background the box put behind it.
	OnOk   = lipgloss.NewStyle().Foreground(White).Background(Complete).Bold(true)
	OnDrop = lipgloss.NewStyle().Foreground(White).Background(Destructive).Bold(true)

	// The meal calendar's text and its empty slots. A date is the default ink, today the
	// brightest thing on the screen, and a day nobody can act on — a weekend, a holiday, a
	// day already gone — keeps its number in the quiet grey.
	MealDate     = lipgloss.NewStyle().Foreground(lipgloss.Color("#C8C8D0"))
	MealToday    = lipgloss.NewStyle().Foreground(lipgloss.Color("#E4E4E8")).Bold(true).Underline(true)
	MealQuietInk = lipgloss.NewStyle().Foreground(MealQuiet)
	MealSlot     = lipgloss.NewStyle().Foreground(MealOpen)

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
