package tui

import (
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
)

// Charon palette, built around the brand teal #377375, plus a small set of
// semantic feedback colors so successes, errors, and cautions read distinctly.
var (
	colorPrimary = lipgloss.Color("#377375") // brand teal
	colorAccent  = lipgloss.Color("#5aa6a3") // lighter teal for highlights
	colorText    = lipgloss.Color("#c7d6d5") // primary foreground
	colorMuted   = lipgloss.Color("#6f8b89") // secondary / hints
	colorOnDark  = lipgloss.Color("#e8f2f1") // text on a teal background

	colorSuccess = lipgloss.Color("#5fbf9f") // ✓ applied / saved / switched
	colorError   = lipgloss.Color("#e06c75") // ✗ failures
	colorWarn    = lipgloss.Color("#d9a441") // cautions (destructive actions)
)

var (
	// titleStyle is the teal header bar used across screens.
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorOnDark).
			Background(colorPrimary).
			Padding(0, 1)

	// statusStyle is the muted footer line for neutral/info messages.
	statusStyle = lipgloss.NewStyle().Foreground(colorMuted).PaddingLeft(1)

	// successStyle / errorStyle / warnStyle color the status line by severity.
	successStyle = lipgloss.NewStyle().Foreground(colorSuccess).PaddingLeft(1)
	errorStyle   = lipgloss.NewStyle().Foreground(colorError).PaddingLeft(1)
	warnStyle    = lipgloss.NewStyle().Foreground(colorWarn).PaddingLeft(1)

	// promptStyle titles the input wizard steps.
	promptStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)

	// hintStyle is faint helper text under inputs.
	hintStyle = lipgloss.NewStyle().Foreground(colorMuted)

	// stepStyle labels wizard progress (e.g. "Step 2 of 4").
	stepStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).PaddingLeft(1)
)

// themedDelegate is the list delegate styled to the Charon palette with a
// modest one-line gap between rows.
func themedDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()
	d.SetSpacing(1)

	d.Styles.NormalTitle = d.Styles.NormalTitle.Foreground(colorText).Padding(0, 0, 0, 1)
	d.Styles.NormalDesc = d.Styles.NormalDesc.Foreground(colorMuted).Padding(0, 0, 0, 1)

	// Highlighted row reads as one unit: accent title + accent description.
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.
		Foreground(colorAccent).Bold(true).
		BorderForeground(colorPrimary).Padding(0, 0, 0, 1)
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.
		Foreground(colorAccent).
		BorderForeground(colorPrimary).Padding(0, 0, 0, 1)

	return d
}

// themedCompactDelegate is the same palette rendered one line per row (no
// description, no inter-row gap) for dense lists like profiles and the model
// picker, where a two-line row makes the selection cursor feel oversized.
func themedCompactDelegate() list.DefaultDelegate {
	d := themedDelegate()
	d.ShowDescription = false
	d.SetSpacing(0)
	return d
}
