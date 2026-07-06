package tui

import "fmt"

// statusRender styles a status line for the given level, prefixing a glyph.
// It returns "" for an empty message so callers can append unconditionally.
func statusRender(level statusLevel, msg string) string {
	if msg == "" {
		return ""
	}
	switch level {
	case statusOK:
		return successStyle.Render("✓ " + msg)
	case statusErr:
		return errorStyle.Render("✗ " + msg)
	default:
		return statusStyle.Render(msg)
	}
}

func (m model) View() string {
	switch m.view {
	case viewConfirmDelete:
		body := "\n" + titleStyle.Render(m.tool.Title+" · delete profile") +
			"\n\n" + warnStyle.Render(fmt.Sprintf("Delete profile %q? This can't be undone.", m.delTarget)) +
			"\n\n" + hintStyle.Render("y: delete · n / esc: cancel")
		return body
	case viewFetching:
		return m.wizardHeader() +
			promptStyle.Render(m.spinner.View()+m.loadingMsg) +
			"\n\n" + hintStyle.Render("fetching models from "+m.wiz.endpoint)
	case viewAddEndpoint, viewAddKey, viewAddName, viewSaveName, viewEditField:
		body := m.wizardHeader() +
			promptStyle.Render(m.prompt()) +
			"\n\n  " + m.input.View() +
			"\n\n" + hintStyle.Render("enter: continue · esc: cancel")
		if line := statusRender(m.statusLvl, m.status); line != "" {
			body += "\n" + line
		}
		return body
	}
	out := m.list.View()
	if m.view == viewTools {
		out = banner() + "\n" + out
	}
	if line := statusRender(m.statusLvl, m.status); line != "" {
		out += "\n" + line
	}
	return out
}

// wizardHeader renders the titled bar and "Step n of N" progress line shown
// atop add-flow screens. It returns just a leading blank line for non-wizard
// input steps (edit-single-field, quick-save) that have no progress.
func (m model) wizardHeader() string {
	n, total, label := wizardStep(m.view)
	if total == 0 {
		return "\n"
	}
	title := titleStyle.Render(m.tool.Title + " · new profile")
	step := stepStyle.Render(fmt.Sprintf("Step %d of %d · %s", n, total, label))
	return "\n" + title + "\n" + step + "\n\n"
}

func (m model) prompt() string {
	if m.view == viewEditField {
		switch m.editField {
		case fieldName:
			return "Edit name:"
		case fieldURL:
			return "Edit API base URL:"
		case fieldToken:
			return "Edit API key (hidden):"
		}
	}
	switch m.view {
	case viewAddEndpoint:
		if m.tool.DefaultEndpoint != "" {
			return "API base URL — leave blank for the default (" + m.tool.DefaultEndpoint + "):"
		}
		return "API base URL:"
	case viewAddKey:
		return "API key — input is hidden as you type:"
	case viewAddName:
		return "Name this profile (e.g. work, openrouter-fast):"
	default:
		return "Save current " + m.tool.Title + " config as:"
	}
}
