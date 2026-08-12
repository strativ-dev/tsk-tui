package model

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/tasnimAlam/tsk/internal/parse"
	"github.com/tasnimAlam/tsk/internal/store"
	"github.com/tasnimAlam/tsk/internal/theme"
)

const barWidth = 16

func (m Model) View() string {
	var b strings.Builder

	b.WriteString(m.header())
	b.WriteString("\n\n")
	b.WriteString(m.list())
	b.WriteString("\n")

	if m.mode == ModeConfirm {
		b.WriteString(theme.Modal.Render(m.cPrompt+"  "+theme.Dim.Render("y / n")) + "\n")
	}
	if m.err != nil {
		b.WriteString(theme.Err.Render("! "+m.err.Error()) + "\n")
	}
	b.WriteString(m.footer())
	return b.String()
}

func (m Model) header() string {
	label := theme.Mode.Render(modeLabel(m.mode))
	left := m.search.View()
	if m.mode != ModeSearch && m.search.Value() != "" {
		left = theme.Dim.Render("  " + m.search.Value())
	}

	head := left + "   " + m.progress()
	gap := m.width - lipgloss.Width(head) - lipgloss.Width(label)
	if gap < 2 {
		gap = 2
	}
	return head + strings.Repeat(" ", gap) + label
}

func (m Model) progress() string {
	done := store.MinutesOn(m.tasks, parse.Today())
	pct := float64(done) / DailyGoal
	if pct > 1 {
		pct = 1
	}
	full := int(pct * barWidth)

	style := theme.Bar
	if done >= DailyGoal {
		style = theme.BarDone
	}
	bar := style.Render(strings.Repeat("█", full)) + theme.BarRest.Render(strings.Repeat("░", barWidth-full))

	return fmt.Sprintf("%s %s %s", bar,
		parse.FormatTotal(done)+theme.Dim.Render(" / 8h"),
		theme.Dim.Render(fmt.Sprintf("%.0f%%", pct*100)))
}

// ponytail: renders every task line, no scrolling — wrap in a bubbles/viewport
// once a real list runs past one screen.
func (m Model) list() string {
	tasks := m.filtered()
	if len(tasks) == 0 {
		return theme.Dim.Render("  no tasks match")
	}

	var b strings.Builder
	for i, t := range tasks {
		focused := i == m.cursor && (m.mode == ModeList || m.mode == ModeSearch)
		b.WriteString(row(m.taskLine(t), focused) + "\n")

		if !m.expanded[t.ID] {
			continue
		}
		b.WriteString(theme.Blur.Render("  "+m.tableHead()) + "\n")
		for j, e := range t.Rows {
			inTable := i == m.cursor && (m.mode == ModeTable || m.mode == ModeJump || m.mode == ModeConfirm)
			b.WriteString(row("  "+entryLine(e), inTable && j == m.row) + "\n")
		}
		if m.mode == ModeInsert && i == m.cursor {
			b.WriteString(row("  "+m.insertLine(), true) + "\n")
		}
		if m.mode == ModeJump && i == m.cursor {
			b.WriteString(theme.Blur.Render("  jump to day "+m.jump.View()) + "\n")
		}
	}
	return b.String()
}

// row shows focus with a left border plus a dim background — never color alone.
func row(s string, focused bool) string {
	if focused {
		return theme.Focus.Render(s)
	}
	return theme.Blur.Render(s)
}

func (m Model) taskLine(t store.Task) string {
	caret := "▸"
	if m.expanded[t.ID] {
		caret = "▾"
	}
	count := fmt.Sprintf("%d entries", len(t.Rows))
	if len(t.Rows) == 1 {
		count = "1 entry"
	}
	return fmt.Sprintf("%s %-40s %s  %s  %s",
		caret,
		trunc(t.Title, 40),
		theme.Tag.Render("["+t.Tag+"]"),
		theme.Dim.Render(count),
		parse.FormatTotal(store.Total(t.Rows)))
}

func (m Model) tableHead() string {
	return theme.Header.Render(fmt.Sprintf("%-10s %-40s %6s", "DATE", "DESCRIPTION", "HOURS"))
}

func entryLine(e store.Entry) string {
	return fmt.Sprintf("%-10s %-40s %6s", e.Date, trunc(e.Desc, 40), parse.FormatHM(e.Minutes))
}

func (m Model) insertLine() string {
	date := m.fields[fieldDate].View()
	if m.datePristine && m.focus == fieldDate {
		// "selected": the whole value is highlighted until the first keystroke.
		date = lipgloss.NewStyle().Foreground(theme.Chrome).Background(theme.Accent).
			Render(m.fields[fieldDate].Value())
	}

	ok, cancel := " ✓ ", " ✕ "
	okStyle := lipgloss.NewStyle().Foreground(theme.Complete)
	cancelStyle := lipgloss.NewStyle().Foreground(theme.Destructive)
	if m.focus == fieldAccept {
		okStyle = okStyle.Reverse(true)
	}
	if m.focus == fieldReject {
		cancelStyle = cancelStyle.Reverse(true)
	}

	return fmt.Sprintf("%-10s %-40s %6s  %s %s",
		date, m.fields[fieldDesc].View(), m.fields[fieldHours].View(),
		okStyle.Render(ok), cancelStyle.Render(cancel))
}

func (m Model) footer() string {
	var parts []string
	for _, b := range keys.help(m.mode) {
		h := b.Help()
		parts = append(parts, theme.Mode.Render(h.Key)+theme.Hint.Render(" "+h.Desc))
	}
	return theme.Blur.Render(strings.Join(parts, theme.Hint.Render("  ·  ")))
}

func trunc(s string, n int) string {
	if lipgloss.Width(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
