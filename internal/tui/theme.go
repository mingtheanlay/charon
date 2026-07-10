package tui

import (
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
)

// The TUI mostly respects the terminal's own theme — plain text renders in the
// default foreground, and hints/dividers come from the standard ANSI palette.
// The brand teal is reserved for the few spots that should always read as
// "Charon", regardless of terminal scheme: the header bar, the selected row,
// focused input, and the banner wordmark.
var (
	colorBrand  = lipgloss.Color("#377375")                                 // brand teal — header fill, selection
	colorOnDark = lipgloss.Color("#e8f2f1")                                 // text on the colorBrand fill
	colorAccent = lipgloss.AdaptiveColor{Light: "#1f6b68", Dark: "#5aa6a3"} // focus: cursor, prompts, step labels

	colorMuted   = lipgloss.Color("8") // ANSI bright black: hints, secondary text
	colorSuccess = lipgloss.Color("2") // ANSI green: ✓ applied / saved / switched
	colorError   = lipgloss.Color("1") // ANSI red: ✗ failures
	colorWarn    = lipgloss.Color("3") // ANSI yellow: cautions (destructive actions)
)

var (
	// titleStyle is the screen header: bold text on a brand-teal fill.
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorOnDark).Background(colorBrand).Padding(0, 1)

	// statusStyle is the muted footer line for neutral/info messages.
	statusStyle = lipgloss.NewStyle().Foreground(colorMuted).PaddingLeft(1)

	// successStyle / errorStyle / warnStyle color the status line by severity.
	successStyle = lipgloss.NewStyle().Foreground(colorSuccess).PaddingLeft(1)
	errorStyle   = lipgloss.NewStyle().Foreground(colorError).PaddingLeft(1)
	warnStyle    = lipgloss.NewStyle().Foreground(colorWarn).PaddingLeft(1)

	// promptStyle titles the input wizard steps, in the focus accent.
	promptStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)

	// hintStyle is faint helper text under inputs.
	hintStyle = lipgloss.NewStyle().Foreground(colorMuted)

	// stepStyle labels wizard progress (e.g. "Step 2 of 4").
	stepStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).PaddingLeft(1)
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
	it, ok := listItem.(item)
	if !ok {
		d.DefaultDelegate.Render(w, m, index, listItem)
		return
	}
	if it.value == sepSentinel {
		rule := lipgloss.NewStyle().Foreground(colorMuted).Padding(0, 0, 0, 1).
			Render(strings.Repeat("─", dividerWidth))
		_, _ = io.WriteString(w, rule)
		return
	}
	// A row with no description (e.g. "Add new profile…") must render as a single
	// line even under a two-line delegate — otherwise the selected style's teal
	// background still paints an empty second line, bleeding color under the text.
	// d is a value receiver, so this only affects this one row's render call.
	if it.desc == "" {
		d.DefaultDelegate.ShowDescription = false
	}
	// The already-picked profile keeps a primary-colored title even once the
	// cursor moves elsewhere, so "what's active" and "what's under the cursor"
	// stay two distinct, simultaneously visible signals. d is a value receiver,
	// so this style swap only affects this one row's render call.
	if it.active && index != m.Index() {
		d.Styles.NormalTitle = d.Styles.NormalTitle.Foreground(colorBrand).Bold(true)
	}
	d.DefaultDelegate.Render(w, m, index, listItem)
}

// baseDelegate is the shared list styling, with a one-line row gap: default
// foreground for normal titles, ANSI muted for descriptions, and a flat
// brand-color fill marking the selected row — no separate bar glyph and no
// extra indent, so row text lines up identically whether selected or not.
func baseDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()
	d.SetSpacing(1)

	d.Styles.NormalTitle = d.Styles.NormalTitle.UnsetForeground().Padding(0, 0, 0, 1)
	d.Styles.NormalDesc = d.Styles.NormalDesc.Foreground(colorMuted).Padding(0, 0, 0, 1)

	d.Styles.SelectedTitle = d.Styles.SelectedTitle.
		UnsetBorderStyle().UnsetBorderLeft().
		Bold(true).Foreground(colorOnDark).Background(colorBrand).
		Padding(0, 0, 0, 1)
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.
		UnsetBorderStyle().UnsetBorderLeft().
		Foreground(colorOnDark).Background(colorBrand).
		Padding(0, 0, 0, 1)

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
