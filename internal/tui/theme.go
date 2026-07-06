package tui

import (
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
)

// Charon palette, built around the brand teal #377375.
var (
	colorPrimary = lipgloss.Color("#377375") // brand teal
	colorAccent  = lipgloss.Color("#5aa6a3") // lighter teal for highlights
	colorText    = lipgloss.Color("#c7d6d5") // primary foreground
	colorMuted   = lipgloss.Color("#6f8b89") // secondary / hints
	colorOnDark  = lipgloss.Color("#e8f2f1") // text on a teal background
)

var (
	// titleStyle is the teal header bar used across screens.
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorOnDark).
			Background(colorPrimary).
			Padding(0, 1)

	// statusStyle is the muted footer line for transient messages.
	statusStyle = lipgloss.NewStyle().Foreground(colorMuted).PaddingLeft(1)

	// promptStyle titles the input wizard steps.
	promptStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)

	// hintStyle is faint helper text under inputs.
	hintStyle = lipgloss.NewStyle().Foreground(colorMuted)
)

// themedDelegate is the list delegate styled to the Charon palette with a
// modest one-line gap between rows.
func themedDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()
	d.SetSpacing(1)

	d.Styles.NormalTitle = d.Styles.NormalTitle.Foreground(colorText).Padding(0, 0, 0, 1)
	d.Styles.NormalDesc = d.Styles.NormalDesc.Foreground(colorMuted).Padding(0, 0, 0, 1)

	d.Styles.SelectedTitle = d.Styles.SelectedTitle.
		Foreground(colorAccent).Bold(true).
		BorderForeground(colorPrimary).Padding(0, 0, 0, 1)
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.
		Foreground(colorPrimary).
		BorderForeground(colorPrimary).Padding(0, 0, 0, 1)

	return d
}
