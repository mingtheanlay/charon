package tui

import (
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
)

// Charon palette: brand teal #377375 plus semantic feedback colors.
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

// newSpinner returns the accent-colored dot spinner shown on the loading screen.
func newSpinner() spinner.Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(colorAccent)
	return s
}

// themedDelegate is the list delegate in the Charon palette, with a one-line row gap.
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

// themedCompactDelegate is the same palette with single-line rows (no description),
// keeping the one-line row gap so spacing stays consistent with the other screens.
func themedCompactDelegate() list.DefaultDelegate {
	d := themedDelegate()
	d.ShowDescription = false
	return d
}
