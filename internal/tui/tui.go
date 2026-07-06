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
	"github.com/charmbracelet/lipgloss"
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
)

const addSentinel = "\x00add"   // magic value for the "add new" list row
const skipModel = "\x00nomodel" // magic value for "skip model" list row

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

var (
	titleStyle  = lipgloss.NewStyle().Bold(true)
	statusStyle = lipgloss.NewStyle().Faint(true)
)

// compactDelegate is the default list delegate with the inter-item blank line
// and extra left padding removed, so rows sit tight together.
func compactDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()
	d.SetSpacing(0)
	d.Styles.NormalTitle = d.Styles.NormalTitle.Padding(0, 0, 0, 1)
	d.Styles.NormalDesc = d.Styles.NormalDesc.Padding(0, 0, 0, 1)
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.Padding(0, 0, 0, 1)
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.Padding(0, 0, 0, 1)
	return d
}

type wizard struct {
	endpoint, key, model string
}

type model struct {
	store  *profile.Store
	view   view
	list   list.Model
	input  textinput.Model
	tool   *tools.Tool
	wiz    wizard
	status string
	width  int
	height int
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
	l := list.New(nil, compactDelegate(), 0, 0)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = titleStyle
	// Trim the list's own vertical padding around the title.
	l.Styles.TitleBar = l.Styles.TitleBar.Padding(0, 0, 1, 0)

	ti := textinput.New()
	ti.CharLimit = 200

	m := model{store: store, view: viewTools, list: l, input: ti}
	m.loadTools()
	return m
}

// inputView reports whether the current view is a text-entry step.
func (m model) inputView() bool {
	switch m.view {
	case viewSaveName, viewAddEndpoint, viewAddKey, viewAddName:
		return true
	}
	return false
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
	m.list.Title = fmt.Sprintf("%s — enter: switch · s: save current · d: delete · esc: back", m.tool.Title)
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
			// Let the user still add the profile, just without a model override.
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
	case viewPickModel:
		m.view = viewProfiles
		m.status = "cancelled"
		m.loadProfiles()
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
			m.startInput("API base URL", false)
			m.input.SetValue(m.tool.DefaultEndpoint)
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

	case viewPickModel:
		if it.value == skipModel {
			m.wiz.model = ""
		} else {
			m.wiz.model = it.value
		}
		m.view = viewAddName
		m.startInput("profile name (e.g. openrouter-fast)", false)
		return m, textinput.Blink
	}
	return m, nil
}

func (m model) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.view = viewProfiles
		m.status = "cancelled"
		m.loadProfiles()
		return m, nil
	case "enter":
		val := m.input.Value()
		switch m.view {
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
	} else if err := m.store.Save(m.tool, name, name, ""); err != nil {
		m.status = "✗ saved config but failed to record profile: " + err.Error()
	} else {
		_ = m.store.SetActiveName(m.tool.Name, name)
		m.status = fmt.Sprintf("✓ Added %s (%s · %s)", name, m.wiz.endpoint, m.wiz.model)
	}
	m.view = viewProfiles
	m.loadProfiles()
	return m, nil
}

func (m model) View() string {
	switch m.view {
	case viewFetching:
		return titleStyle.Render("Fetching models from "+m.wiz.endpoint+" …") +
			"\n\n" + statusStyle.Render("please wait")
	case viewAddEndpoint, viewAddKey, viewAddName, viewSaveName:
		return titleStyle.Render(m.prompt()) +
			"\n\n  " + m.input.View() +
			"\n\n" + statusStyle.Render("enter: continue · esc: cancel")
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
	switch m.view {
	case viewAddEndpoint:
		return "New " + m.tool.Title + " profile — API base URL:"
	case viewAddKey:
		return "API key (hidden):"
	case viewAddName:
		return "Name this profile:"
	default:
		return "Save current " + m.tool.Title + " config as:"
	}
}
