// Package tui provides the interactive arrow-key menu for aies.
package tui

import (
	"fmt"

	"charon/internal/models"
	"charon/internal/profile"
	"charon/internal/secret"
	"charon/internal/tools"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Run starts the interactive menu against the given store.
func Run(store *profile.Store) error {
	_, err := tea.NewProgram(newModel(store), tea.WithAltScreen()).Run()
	return err
}

type view int

const (
	viewTools view = iota
	viewProfiles
	viewSaveName    // quick-save current config ("s")
	viewAddEndpoint // wizard: enter endpoint
	viewAddKey      // wizard: enter API key
	viewFetching    // wizard: fetching models
	viewPickModel   // wizard: choose a model
	viewAddName     // wizard: name the profile
	viewEditForm    // edit: field picker (url/name/token/model)
	viewEditField   // edit: single-field text input
)

const (
	addSentinel = "\x00add"     // the "add new" list row
	skipModel   = "\x00nomodel" // the "skip model" list row
	fieldName   = "\x00name"
	fieldURL    = "\x00url"
	fieldToken  = "\x00token"
	fieldModel  = "\x00model"
	fieldSave   = "\x00save"
)

type item struct {
	title, desc string
	value       string
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

// fetchedMsg carries the async result of a models.Fetch call.
type fetchedMsg struct {
	list []string
	err  error
}

func fetchModelsCmd(provider, endpoint, key string) tea.Cmd {
	return func() tea.Msg {
		l, err := models.Fetch(models.Provider(provider), endpoint, key)
		return fetchedMsg{list: l, err: err}
	}
}

type wizard struct {
	endpoint, key, model string
	name                 string // target profile name when editing
	origName             string // pre-edit name, to clean up on rename
	edit                 bool   // true = overwrite an existing profile
}

// exampleEndpoint is shown as placeholder text so we never prefill (or reveal)
// a real endpoint value in the input.
const exampleEndpoint = "https://api.example.com/v1"

type model struct {
	store     *profile.Store
	view      view
	list      list.Model
	input     textinput.Model
	tool      *tools.Tool
	wiz       wizard
	editField string // which field the single-field editor is editing
	fromForm  bool   // model picker/fetch was launched from the edit form
	status    string
	width     int
	height    int
}

// resize sizes the list, reserving space for the banner on the tools screen.
func (m *model) resize() {
	header := 1
	if m.view == viewTools {
		header = bannerHeight + 1
	}
	h := m.height - header
	if h < 3 {
		h = 3
	}
	m.list.SetSize(m.width, h)
}

func newModel(store *profile.Store) model {
	l := list.New(nil, themedDelegate(), 0, 0)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = titleStyle
	l.Styles.TitleBar = l.Styles.TitleBar.Padding(0, 0, 1, 0)
	// Theme the paginator and help footer to match.
	l.Styles.HelpStyle = l.Styles.HelpStyle.Foreground(colorMuted)
	l.Styles.PaginationStyle = l.Styles.PaginationStyle.Foreground(colorMuted)
	l.Help.Styles.ShortKey = l.Help.Styles.ShortKey.Foreground(colorAccent)
	l.Help.Styles.ShortDesc = l.Help.Styles.ShortDesc.Foreground(colorMuted)

	ti := textinput.New()
	ti.CharLimit = 200
	ti.PromptStyle = ti.PromptStyle.Foreground(colorAccent)
	ti.Cursor.Style = ti.Cursor.Style.Foreground(colorAccent)
	ti.PlaceholderStyle = ti.PlaceholderStyle.Foreground(colorMuted)

	m := model{store: store, view: viewTools, list: l, input: ti}
	m.loadTools()
	return m
}

// inputView reports whether the current view is a text-entry step.
func (m model) inputView() bool {
	switch m.view {
	case viewSaveName, viewAddEndpoint, viewAddKey, viewAddName, viewEditField:
		return true
	}
	return false
}

// loadEditForm populates the field picker from the working wizard values.
func (m *model) loadEditForm() {
	token := "(none)"
	if m.wiz.key != "" {
		token = secret.Mask(m.wiz.key)
	}
	modelVal := m.wiz.model
	if modelVal == "" {
		modelVal = "(none)"
	}
	endpoint := m.wiz.endpoint
	if endpoint == "" {
		endpoint = "(none)"
	}
	m.list.SetItems([]list.Item{
		item{title: "Name", desc: m.wiz.name, value: fieldName},
		item{title: "URL", desc: endpoint, value: fieldURL},
		item{title: "Token", desc: token, value: fieldToken},
		item{title: "Model", desc: modelVal + "  (enter to fetch & pick)", value: fieldModel},
		item{title: "✓ Save changes", desc: "apply and switch to this profile", value: fieldSave},
	})
	m.list.Title = fmt.Sprintf("Edit %s / %s — enter: edit field · esc: cancel", m.tool.Title, m.wiz.name)
}

func (m *model) loadTools() {
	var items []list.Item
	for _, t := range tools.All() {
		desc := "not detected"
		if t.Detected != nil && t.Detected() {
			info, _ := t.Describe()
			active := m.store.Active(t.Name)
			if active == "" {
				active = "—"
			}
			desc = fmt.Sprintf("active: %s · %s · %s", active, info.AuthMode, info.Endpoint)
		}
		items = append(items, item{title: t.Title, desc: desc, value: t.Name})
	}
	m.list.SetItems(items)
	m.list.Title = "Charon — select a tool"
}

func (m *model) loadProfiles() {
	var items []list.Item
	active := m.store.Active(m.tool.Name)
	for _, name := range m.store.List(m.tool.Name) {
		man, _ := m.store.LoadManifest(m.tool.Name, name)
		marker := ""
		if name == active {
			marker = "✓ "
		}
		items = append(items, item{title: marker + name, desc: man.Label, value: name})
	}
	if m.tool.ApplyAuth != nil {
		items = append(items, item{title: "＋ Add new profile…", desc: "enter endpoint + key, pick a model", value: addSentinel})
	}
	m.list.SetItems(items)
	m.list.Title = fmt.Sprintf("%s — enter: switch · e: edit · s: save · d: delete · esc: back", m.tool.Title)
}

func (m *model) showModels(ids []string) {
	var items []list.Item
	for _, id := range ids {
		items = append(items, item{title: id, desc: "", value: id})
	}
	items = append(items, item{title: "(skip — no model override)", desc: "", value: skipModel})
	m.list.SetItems(items)
	m.list.Title = m.tool.Title + " — choose a model · esc: cancel"
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		return m, nil

	case fetchedMsg:
		if msg.err != nil {
			if m.fromForm {
				// Stay on the edit form; keep the existing model value.
				m.fromForm = false
				m.status = "✗ " + msg.err.Error()
				m.view = viewEditForm
				m.loadEditForm()
				return m, nil
			}
			// Model list unavailable; proceed without a model override.
			m.wiz.model = ""
			if m.wiz.edit {
				m.status = "✗ " + msg.err.Error()
				return m.finishAdd(m.wiz.name)
			}
			m.status = "✗ " + msg.err.Error() + " — you can name it without a model"
			m.view = viewAddName
			m.startInput("profile name", false)
			return m, textinput.Blink
		}
		m.view = viewPickModel
		m.status = fmt.Sprintf("%d models found", len(msg.list))
		m.showModels(msg.list)
		return m, nil

	case tea.KeyMsg:
		if m.inputView() {
			return m.updateInput(msg)
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc":
			return m.onEsc()
		case "enter":
			return m.onEnter()
		case "e":
			if m.view == viewProfiles && m.tool.ApplyAuth != nil {
				if it, ok := m.list.SelectedItem().(item); ok && it.value != addSentinel {
					sp, _ := m.store.GetSpec(m.tool.Name, it.value)
					m.wiz = wizard{name: it.value, origName: it.value, edit: true,
						endpoint: sp.Endpoint, key: sp.Key, model: sp.Model}
					m.view = viewEditForm
					m.status = ""
					m.loadEditForm()
					return m, nil
				}
			}
		case "s":
			if m.view == viewProfiles {
				m.view = viewSaveName
				m.startInput("profile name (e.g. work-key)", false)
				return m, textinput.Blink
			}
		case "d":
			if m.view == viewProfiles {
				if it, ok := m.list.SelectedItem().(item); ok && it.value != addSentinel {
					if err := m.store.Remove(m.tool.Name, it.value); err != nil {
						m.status = "✗ " + err.Error()
					} else {
						m.status = "Deleted " + it.value
						m.loadProfiles()
					}
				}
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// startInput configures the text field for a step (password masks the echo).
func (m *model) startInput(placeholder string, password bool) {
	m.input.SetValue("")
	m.input.Placeholder = placeholder
	if password {
		m.input.EchoMode = textinput.EchoPassword
	} else {
		m.input.EchoMode = textinput.EchoNormal
	}
	m.input.Focus()
}

func (m model) onEsc() (tea.Model, tea.Cmd) {
	switch m.view {
	case viewProfiles:
		m.view = viewTools
		m.status = ""
		m.loadTools()
		m.resize() // banner returns → shrink the list
	case viewEditForm:
		m.view = viewProfiles
		m.status = "cancelled"
		m.loadProfiles()
	case viewPickModel:
		if m.fromForm {
			m.fromForm = false
			m.view = viewEditForm
			m.loadEditForm()
		} else {
			m.view = viewProfiles
			m.status = "cancelled"
			m.loadProfiles()
		}
	}
	return m, nil
}

func (m model) onEnter() (tea.Model, tea.Cmd) {
	it, ok := m.list.SelectedItem().(item)
	if !ok {
		return m, nil
	}
	switch m.view {
	case viewTools:
		t := tools.Find(it.value)
		if t == nil || t.Detected == nil || !t.Detected() {
			m.status = it.title + " is not detected"
			return m, nil
		}
		m.tool = t
		m.view = viewProfiles
		m.status = ""
		m.loadProfiles()
		m.resize() // banner hidden → grow the list

	case viewProfiles:
		if it.value == addSentinel {
			m.wiz = wizard{}
			m.view = viewAddEndpoint
			// Placeholder only — never prefill (or reveal) a real endpoint value.
			m.startInput(exampleEndpoint, false)
			return m, textinput.Blink
		}
		backup, err := m.store.Apply(m.tool, it.value)
		if err != nil {
			m.status = "✗ " + err.Error()
		} else {
			info, _ := m.tool.Describe()
			m.status = fmt.Sprintf("✓ Switched to %s (%s · %s). Backup: %s",
				it.value, info.Endpoint, secret.Mask(info.Secret), backup)
			m.loadProfiles()
		}

	case viewEditForm:
		return m.onEditFormSelect(it.value)

	case viewPickModel:
		if it.value == skipModel {
			m.wiz.model = ""
		} else {
			m.wiz.model = it.value
		}
		if m.fromForm {
			m.fromForm = false
			m.view = viewEditForm
			m.status = "model set to " + m.wiz.model
			m.loadEditForm()
			return m, nil
		}
		if m.wiz.edit {
			return m.finishAdd(m.wiz.name) // editing keeps the existing name
		}
		m.view = viewAddName
		m.startInput("profile name (e.g. openrouter-fast)", false)
		return m, textinput.Blink
	}
	return m, nil
}

// onEditFormSelect handles a chosen row in the edit field-picker.
func (m model) onEditFormSelect(field string) (tea.Model, tea.Cmd) {
	switch field {
	case fieldName:
		m.editField = field
		m.view = viewEditField
		m.startInput("profile name", false)
		m.input.SetValue(m.wiz.name)
		return m, textinput.Blink
	case fieldURL:
		m.editField = field
		m.view = viewEditField
		m.startInput(exampleEndpoint, false)
		m.input.SetValue(m.wiz.endpoint)
		return m, textinput.Blink
	case fieldToken:
		m.editField = field
		m.view = viewEditField
		m.startInput("API key", true)
		m.input.SetValue(m.wiz.key)
		return m, textinput.Blink
	case fieldModel:
		if m.wiz.endpoint == "" || m.wiz.key == "" {
			m.status = "set URL and token first, then fetch models"
			return m, nil
		}
		m.fromForm = true
		m.view = viewFetching
		m.status = "Fetching models…"
		return m, fetchModelsCmd(m.tool.Provider, m.wiz.endpoint, m.wiz.key)
	case fieldSave:
		return m.finishAdd(m.wiz.name)
	}
	return m, nil
}

func (m model) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.view == viewEditField {
			m.view = viewEditForm // cancel a single field → back to the form
			m.loadEditForm()
			return m, nil
		}
		m.view = viewProfiles
		m.status = "cancelled"
		m.loadProfiles()
		return m, nil
	case "enter":
		val := m.input.Value()
		switch m.view {
		case viewEditField:
			switch m.editField {
			case fieldName:
				if val != "" {
					m.wiz.name = val
				}
			case fieldURL:
				m.wiz.endpoint = val
			case fieldToken:
				m.wiz.key = val
			}
			m.view = viewEditForm
			m.loadEditForm()
			return m, nil

		case viewSaveName:
			if val == "" {
				m.status = "name required"
				return m, nil
			}
			if err := m.store.Save(m.tool, val, val, ""); err != nil {
				m.status = "✗ " + err.Error()
			} else {
				m.status = "Saved current config as " + val
			}
			m.view = viewProfiles
			m.loadProfiles()
			return m, nil

		case viewAddEndpoint:
			if val == "" {
				val = m.tool.DefaultEndpoint // blank accepts the provider default
			}
			m.wiz.endpoint = val
			m.view = viewAddKey
			m.startInput("API key", true)
			return m, textinput.Blink

		case viewAddKey:
			if val == "" {
				m.status = "key required"
				return m, nil
			}
			m.wiz.key = val
			m.view = viewFetching
			m.status = "Fetching models…"
			return m, fetchModelsCmd(m.tool.Provider, m.wiz.endpoint, m.wiz.key)

		case viewAddName:
			if val == "" {
				m.status = "name required"
				return m, nil
			}
			return m.finishAdd(val)
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// finishAdd writes the wizard's endpoint/key/model into the tool's live config
// and snapshots it as the named profile.
func (m model) finishAdd(name string) (tea.Model, tea.Cmd) {
	spec := tools.AuthSpec{Endpoint: m.wiz.endpoint, Key: m.wiz.key, Model: m.wiz.model}
	if err := m.tool.ApplyAuth(spec); err != nil {
		m.status = "✗ " + err.Error()
	} else if err := m.store.SaveWithSpec(m.tool, name,
		profile.Spec{Endpoint: m.wiz.endpoint, Key: m.wiz.key, Model: m.wiz.model}); err != nil {
		m.status = "✗ saved config but failed to record profile: " + err.Error()
	} else {
		_ = m.store.SetActiveName(m.tool.Name, name)
		verb := "Added"
		if m.wiz.edit {
			verb = "Updated"
			// If the profile was renamed, remove the old one.
			if m.wiz.origName != "" && m.wiz.origName != name {
				_ = m.store.Remove(m.tool.Name, m.wiz.origName)
			}
		}
		m.status = fmt.Sprintf("✓ %s %s (%s · %s)", verb, name, m.wiz.endpoint, m.wiz.model)
	}
	m.view = viewProfiles
	m.loadProfiles()
	return m, nil
}

func (m model) View() string {
	switch m.view {
	case viewFetching:
		return "\n" + promptStyle.Render("Fetching models from "+m.wiz.endpoint+" …") +
			"\n\n" + hintStyle.Render("please wait")
	case viewAddEndpoint, viewAddKey, viewAddName, viewSaveName, viewEditField:
		body := "\n" + promptStyle.Render(m.prompt()) +
			"\n\n  " + m.input.View() +
			"\n\n" + hintStyle.Render("enter: continue · esc: cancel")
		if m.status != "" {
			body += "\n" + statusStyle.Render(m.status)
		}
		return body
	}
	out := m.list.View()
	if m.view == viewTools {
		out = banner() + "\n" + out
	}
	if m.status != "" {
		out += "\n" + statusStyle.Render(m.status)
	}
	return out
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
		if m.wiz.edit {
			return "Edit " + m.wiz.name + " — API base URL:"
		}
		return "New " + m.tool.Title + " profile — API base URL:"
	case viewAddKey:
		return "API key (hidden):"
	case viewAddName:
		return "Name this profile:"
	default:
		return "Save current " + m.tool.Title + " config as:"
	}
}
