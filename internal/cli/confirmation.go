package cli

import (
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"strings"
)

func confirmationDialog(background string, width, height int, title, name, id, note, action string, focused, danger bool) string {
	return choiceDialog(background, width, height, title, name, id, note, "Cancel", action, focused, danger)
}

func choiceDialog(background string, width, height int, title, name, id, note, first, second string, focused, danger bool) string {
	if width == 0 {
		width = 80
	}
	if height == 0 {
		height = 24
	}
	if width < 32 || height < 14 {
		return fullScreenView(width, height, ansi.Truncate("Enlarge window; Esc cancels", max(1, width), "…")).Content
	}
	inner := min(52, width-6)
	base := lipgloss.NewStyle().Foreground(currentTheme.Title).Background(currentTheme.Surface)
	lines := []string{base.Bold(true).Render(title), "", ansi.Truncate(bookCell(name), inner, "…")}
	if id != "" {
		lines = append(lines, ansi.Truncate("ID: "+id, inner, "…"))
	}
	if note != "" {
		lines = append(lines, "", ansi.Truncate(bookCell(note), inner, "…"))
	}
	firstButton, secondButton := renderActionButton(first, !focused, false), renderActionButton(second, focused, danger)
	buttons := lipgloss.JoinHorizontal(lipgloss.Top, firstButton, base.Render("  "), secondButton)
	if lipgloss.Width(buttons) > inner {
		buttons = lipgloss.JoinVertical(lipgloss.Left, firstButton, secondButton)
	}
	lines = append(lines, "", buttons,
		"", "←/→ select · Enter · Esc")
	content := strings.Join(lines, "\n")
	dialog := base.Width(inner).Padding(1, 1).Border(lipgloss.RoundedBorder()).BorderForeground(currentTheme.Accent).Render(content)
	x, y := max(0, (width-lipgloss.Width(dialog))/2), max(0, (height-lipgloss.Height(dialog))/2)
	dim := lipgloss.NewStyle().Foreground(currentTheme.Muted).Render(ansi.Strip(background))
	return fullScreenView(width, height, lipgloss.NewCompositor(lipgloss.NewLayer(dim), lipgloss.NewLayer(dialog).X(x).Y(y)).Render()).Content
}
