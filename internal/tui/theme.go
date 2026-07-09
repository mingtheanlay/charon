package tui

import (
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
)

// The TUI respects the terminal's own theme: text renders in the default
// foreground, secondary/semantic colors come from the standard ANSI palette
// (which the user's terminal scheme defines), and the only fixed brand color
// is the teal on the ASCII banner wordmark (see banner.go).
var (
	colorBrand = lipgloss.Color("#377375") // brand teal — banner art only

	colorMuted   = lipgloss.Color("8") // ANSI bright black: hints, secondary text
	colorSuccess = lipgloss.Color("2") // ANSI green: ✓ applied / saved / switched
	colorError   = lipgloss.Color("1") // ANSI red: ✗ failures
	colorWarn    = lipgloss.Color("3") // ANSI yellow: cautions (destructive actions)
)

var (
	// titleStyle is the screen header; bold default-foreground text, no fill.
	titleStyle = lipgloss.NewStyle().Bold(true).Padding(0, 1)

	// statusStyle is the muted footer line for neutral/info messages.
	statusStyle = lipgloss.NewStyle().Foreground(colorMuted).PaddingLeft(1)

	// successStyle / errorStyle / warnStyle color the status line by severity.
	successStyle = lipgloss.NewStyle().Foreground(colorSuccess).PaddingLeft(1)
	errorStyle   = lipgloss.NewStyle().Foreground(colorError).PaddingLeft(1)
	warnStyle    = lipgloss.NewStyle().Foreground(colorWarn).PaddingLeft(1)

	// promptStyle titles the input wizard steps.
	promptStyle = lipgloss.NewStyle().Bold(true)

	// hintStyle is faint helper text under inputs.
	hintStyle = lipgloss.NewStyle().Foreground(colorMuted)

	// stepStyle labels wizard progress (e.g. "Step 2 of 4").
	stepStyle = lipgloss.NewStyle().Foreground(colorMuted).Bold(true).PaddingLeft(1)
)

// newSpinner returns the dot spinner shown on the loading screen.
func newSpinner() spinner.Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return s
}

// charonDelegate is the list delegate that additionally draws the sepSentinel
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

// baseDelegate is the shared theme-respecting list styling, with a one-line row gap:
// default foreground for titles, ANSI muted for descriptions, bold for the selection.
func baseDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()
	d.SetSpacing(1)

	d.Styles.NormalTitle = d.Styles.NormalTitle.UnsetForeground().Padding(0, 0, 0, 1)
	d.Styles.NormalDesc = d.Styles.NormalDesc.Foreground(colorMuted).Padding(0, 0, 0, 1)

	// Highlighted row reads as one unit: bold title + border bar in the default
	// foreground, so the selection stands out without leaving the terminal palette.
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.
		UnsetForeground().Bold(true).
		UnsetBorderForeground().Padding(0, 0, 0, 1)
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.
		Foreground(colorMuted).
		UnsetBorderForeground().Padding(0, 0, 0, 1)

	return d
}

// themedDelegate is the two-line row delegate (title + description).
func themedDelegate() charonDelegate {
	return charonDelegate{DefaultDelegate: baseDelegate()}
}

// themedCompactDelegate is the same styling with single-line rows (no description),
// keeping the one-line row gap so spacing stays consistent with the other screens.
func themedCompactDelegate() charonDelegate {
	d := baseDelegate()
	d.ShowDescription = false
	return charonDelegate{DefaultDelegate: d}
}
