package ui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"strings"
)

func (m *model) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true
	if m.width <= 0 || m.height <= 0 {
		return v
	}
	w := min(72, m.width)
	if m.width < 48 || m.height < 18 {
		v.Content = bounded("Enlarge the terminal to at least 48 × 18.\nEsc exits; input is paused.", w, m.height, 0)
		return v
	}
	p := colors(m.dark)
	title := lipgloss.NewStyle().Foreground(lipgloss.Color(p.lavender)).Bold(true).Render("Golden Gate Setup")
	body := m.lines
	if m.form != nil {
		body = m.form.View()
	}
	if m.stage == "inspecting" {
		body = "Inspecting the current Mac…"
	}
	footer := "Esc cancels · selections remain local"
	if m.stage == "preview" {
		footer = "↑/↓ scroll · e edit · a apply reviewed plan · Esc exit"
	}
	if m.stage == "files" {
		footer = "↑/↓ scroll · y replace with backup · n/Enter preserve · Esc exit"
	}
	if m.stage == "done" {
		footer = "↑/↓ scroll · q or Enter closes"
	}
	body = bounded(body, max(1, w-4), max(1, m.height-7), m.scroll)
	content := title + "\n\n" + body + "\n\n" + bounded(footer, w-4, 1, 0)
	card := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(p.lavender)).Padding(0, 1).Width(w - 2).Render(content)
	v.Content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, card)
	v.ForegroundColor = lipgloss.Color(p.text)
	v.BackgroundColor = lipgloss.Color(p.surface)
	return v
}
func bounded(s string, w, h, offset int) string {
	if w <= 0 || h <= 0 {
		return ""
	}
	lines := strings.Split(ansi.Wrap(s, w, ""), "\n")
	offset = min(max(0, offset), max(0, len(lines)-h))
	end := min(len(lines), offset+h)
	out := make([]string, 0, end-offset)
	for _, line := range lines[offset:end] {
		out = append(out, ansi.Truncate(line, w, ""))
	}
	return strings.Join(out, "\n")
}
