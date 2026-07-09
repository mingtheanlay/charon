package tui

import (
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
)

// Charon palette: brand teal #377375 plus semantic feedback colors. Most entries are
// AdaptiveColor so they stay readable on both light- and dark-background terminals
// (lipgloss picks a side by querying the terminal's background at startup); colorPrimary
// and colorOnDark are a fixed fill+text pair (the title bar's own teal background, not
// the terminal's) so they don't need to adapt.
var (
	colorPrimary = lipgloss.Color("#377375") // brand teal, title-bar fill
	colorOnDark  = lipgloss.Color("#e8f2f1") // text on the colorPrimary fill

	colorAccent = lipgloss.AdaptiveColor{Light: "#1f6b68", Dark: "#5aa6a3"} // highlights
	colorText   = lipgloss.AdaptiveColor{Light: "#22302f", Dark: "#c7d6d5"} // primary foreground
	colorMuted  = lipgloss.AdaptiveColor{Light: "#5b7371", Dark: "#6f8b89"} // secondary / hints

	colorSuccess = lipgloss.AdaptiveColor{Light: "#1f8f6f", Dark: "#5fbf9f"} // ✓ applied / saved / switched
	colorError   = lipgloss.AdaptiveColor{Light: "#c13f4a", Dark: "#e06c75"} // ✗ failures
	colorWarn    = lipgloss.AdaptiveColor{Light: "#946c00", Dark: "#d9a441"} // cautions (destructive actions)
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

// charonDelegate is the palette delegate that additionally draws the sepSentinel
// row as a single dim rule — a tight, visible divider between data and action rows.
type charonDelegate struct {
	list.DefaultDelegate
}

// dividerWidth caps the rule at a modest width so it reads as a separator, not a border.
const dividerWidth = 18

func (d charonDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	if it, ok := listItem.(item); ok && it.value == sepSentinel {
		rule := lipgloss.NewStyle().Foreground(colorMuted).Padding(0, 0, 0, 1).
			Render(strings.Repeat("─", dividerWidth))
		_, _ = io.WriteString(w, rule)
		return
	}
	d.DefaultDelegate.Render(w, m, index, listItem)
}

// baseDelegate is the shared list styling in the Charon palette, with a one-line row gap.
func baseDelegate() list.DefaultDelegate {
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

// themedDelegate is the two-line row delegate (title + description).
func themedDelegate() charonDelegate {
	return charonDelegate{DefaultDelegate: baseDelegate()}
}

// themedCompactDelegate is the same palette with single-line rows (no description),
// keeping the one-line row gap so spacing stays consistent with the other screens.
func themedCompactDelegate() charonDelegate {
	d := baseDelegate()
	d.ShowDescription = false
	return charonDelegate{DefaultDelegate: d}
}
