package ui

import (
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

type palette struct{ surface, text, muted, lavender, peach, mint, rose, cyan string }

func colors(dark bool) palette {
	if dark {
		return palette{"#1E1E2E", "#CDD6F4", "#9AA1BA", "#D2A8FF", "#FF9E64", "#50FA7B", "#FF79C6", "#7DCFFF"}
	}
	return palette{"#EFF1F5", "#4C4F69", "#62677F", "#7000CC", "#A33D00", "#157347", "#B51949", "#006B9A"}
}
func formTheme(dark bool) *huh.Styles {
	p := colors(dark)
	s := huh.ThemeBase(dark)
	s.Focused.Title = lipgloss.NewStyle().Foreground(lipgloss.Color(p.lavender)).Bold(true)
	s.Focused.Description = lipgloss.NewStyle().Foreground(lipgloss.Color(p.muted))
	s.Focused.SelectedOption = lipgloss.NewStyle().Foreground(lipgloss.Color(p.mint))
	s.Focused.UnselectedOption = lipgloss.NewStyle().Foreground(lipgloss.Color(p.text))
	s.Focused.ErrorMessage = lipgloss.NewStyle().Foreground(lipgloss.Color(p.rose))
	s.Blurred.Title = s.Focused.Title
	s.Blurred.Description = s.Focused.Description
	for _, f := range []*huh.FieldStyles{&s.Focused, &s.Blurred} {
		f.Base = f.Base.Foreground(lipgloss.Color(p.text)).BorderForeground(lipgloss.Color(p.cyan))
		f.Title = f.Title.Foreground(lipgloss.Color(p.lavender)).Bold(true)
		f.NoteTitle = f.Title
		f.Description = f.Description.Foreground(lipgloss.Color(p.muted))
		f.ErrorIndicator = f.ErrorIndicator.Foreground(lipgloss.Color(p.rose))
		f.ErrorMessage = f.ErrorMessage.Foreground(lipgloss.Color(p.rose))
		f.Option = f.Option.Foreground(lipgloss.Color(p.text))
		f.SelectedOption = f.SelectedOption.Foreground(lipgloss.Color(p.mint))
		f.SelectedPrefix = f.SelectedPrefix.Foreground(lipgloss.Color(p.mint))
		f.UnselectedOption = f.UnselectedOption.Foreground(lipgloss.Color(p.text))
		f.UnselectedPrefix = f.UnselectedPrefix.Foreground(lipgloss.Color(p.muted))
		f.MultiSelectSelector = f.MultiSelectSelector.Foreground(lipgloss.Color(p.peach))
		f.TextInput.Text = f.TextInput.Text.Foreground(lipgloss.Color(p.text))
		f.TextInput.Prompt = f.TextInput.Prompt.Foreground(lipgloss.Color(p.cyan))
		f.TextInput.Placeholder = f.TextInput.Placeholder.Foreground(lipgloss.Color(p.muted))
		f.FocusedButton = f.FocusedButton.Background(lipgloss.Color(p.lavender)).Foreground(lipgloss.Color(p.surface))
		f.BlurredButton = f.BlurredButton.Background(lipgloss.Color(p.surface)).Foreground(lipgloss.Color(p.text))
	}
	s.Help.ShortKey = s.Help.ShortKey.Foreground(lipgloss.Color(p.cyan))
	s.Help.ShortDesc = s.Help.ShortDesc.Foreground(lipgloss.Color(p.muted))
	return s
}
