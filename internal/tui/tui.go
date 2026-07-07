// Package tui provides the interactive arrow-key menu for charon.
//
// The model is split across a few files in this package:
//   - tui.go     the model, lifecycle (Run/Init/Update) and top-level navigation
//   - views.go   rendering (View, wizard header, prompts, status line)
//   - wizard.go  the add/edit profile flow and confirm-delete prompt
//   - picker.go  the fetch-and-choose-a-model screen
package tui

import (
	"fmt"
	"time"

	"charon/internal/profile"
	"charon/internal/secret"
	"charon/internal/tools"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
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
	viewAddEndpoint   // wizard: enter endpoint
	viewAddKey        // wizard: enter API key
	viewFetching      // wizard: fetching models
	viewPickModel     // wizard: choose a model
	viewAddName       // wizard: name the profile
	viewEditForm      // edit: field picker (url/name/token/model)
	viewEditField     // edit: single-field text input
	viewConfirmDelete // confirm removing a profile (y/n)
)

// statusLevel colors the footer status line by severity.
type statusLevel int

const (
	statusInfo statusLevel = iota // neutral / muted
	statusOK                      // success (green)
	statusErr                     // failure (red)
)

const (
	addSentinel = "\x00add"     // the "add new" list row
	skipModel   = "\x00nomodel" // the "skip model" list row
)

type item struct {
	title, desc string
	value       string
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

// Contextual key bindings shown in the list's help footer (and "?"-expanded).
var (
	keySwitch = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "switch"))
	keyEdit   = key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit"))
	keyDelete = key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete"))
	keyBack   = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back"))
	keyOpen   = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open"))
	keyChoose = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "choose"))
	// keyFilter never matches a real press; it only advertises type-to-search.
	keyFilter  = key.NewBinding(key.WithKeys("\x00filter"), key.WithHelp("type", "search"))
	keyRefresh = key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "refresh"))
)

// exampleEndpoint is placeholder text; a real endpoint is never prefilled.
const exampleEndpoint = "https://api.example.com/v1"

type model struct {
	store     *profile.Store
	allTools  []*tools.Tool // registry built once; reused across renders
	view      view
	list      list.Model
	input     textinput.Model
	tool      *tools.Tool
	wiz       wizard
	editField string // which field the single-field editor is editing
	fromForm  bool   // model picker/fetch was launched from the edit form
	delTarget string // profile name pending delete confirmation

	spinner    spinner.Model
	loadingMsg string      // playful line shown on the loading screen, picked per fetch
	pending    *fetchedMsg // fetch result held back until the min-load window elapses
	fetchStart time.Time   // when the current fetch began, for the min-load throttle

	allModels   []string // full fetched model list, unfiltered
	modelFilter string   // current type-to-search query in the model picker
	status      string
	statusLvl   statusLevel
	width       int
	height      int
}

// setStatus records a footer message at the given severity.
func (m *model) setStatus(level statusLevel, msg string) {
	m.status = msg
	m.statusLvl = level
}

// clearStatus wipes the footer message.
func (m *model) clearStatus() {
	m.status = ""
	m.statusLvl = statusInfo
}

// findTool returns the registered tool with the given name, or nil.
func (m *model) findTool(name string) *tools.Tool {
	for _, t := range m.allTools {
		if t.Name == name {
			return t
		}
	}
	return nil
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
	l.InfiniteScrolling = true
	l.KeyMap.Quit.SetEnabled(false) // "q"/"esc" must not quit; only ctrl+c does
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

	m := model{store: store, allTools: tools.All(), view: viewTools, list: l, input: ti, spinner: newSpinner()}
	m.loadTools()
	return m
}

// setHelpKeys registers the contextual bindings shown in the list's help footer.
func (m *model) setHelpKeys(bindings ...key.Binding) {
	m.list.AdditionalShortHelpKeys = func() []key.Binding { return bindings }
	m.list.AdditionalFullHelpKeys = func() []key.Binding { return bindings }
}

// inputView reports whether the current view is a text-entry step.
func (m model) inputView() bool {
	switch m.view {
	case viewAddEndpoint, viewAddKey, viewAddName, viewEditField:
		return true
	}
	return false
}

// selectByValue moves the cursor to the row matching v; a miss keeps the default.
func (m *model) selectByValue(v string) {
	if v == "" {
		return
	}
	for i, it := range m.list.Items() {
		if li, ok := it.(item); ok && li.value == v {
			m.list.Select(i)
			return
		}
	}
}

func (m *model) loadTools() {
	var items []list.Item
	selectedIndex := 0
	for i, t := range m.allTools {
		desc := "not installed — see the README to set it up"
		if t.Detected != nil && t.Detected() {
			info, _ := t.Describe()
			active := m.store.Active(t.Name)
			if active == "" {
				active = "—"
			}
			desc = fmt.Sprintf("active: %s · %s · %s", active, info.AuthMode, info.Endpoint)
		}
		items = append(items, item{title: t.Title, desc: desc, value: t.Name})
		if m.tool != nil && t.Name == m.tool.Name {
			selectedIndex = i
		}
	}
	m.list.SetDelegate(themedDelegate()) // two-line rows show each tool's status
	m.list.SetItems(items)
	m.list.Select(selectedIndex)
	m.list.Title = "Charon — select a tool"
	m.setHelpKeys(keyOpen)
}

func (m *model) loadProfiles() {
	var items []list.Item
	active := m.store.Active(m.tool.Name)
	saved := m.store.List(m.tool.Name)
	selectedIndex := 0
	for i, name := range saved {
		// ✓ marks the active profile; url and model show on the line below.
		title := name
		if name == active {
			title = "✓ " + title
			selectedIndex = i // land the cursor on the active profile
		}
		items = append(items, item{title: title, desc: m.profileDetail(name), value: name})
	}
	if m.tool.ApplyAuth != nil {
		items = append(items, item{title: "＋ Add new profile…", value: addSentinel})
	}
	m.list.SetDelegate(themedDelegate()) // two-line rows show each profile's url and model
	m.list.SetItems(items)
	m.list.Select(selectedIndex)
	m.list.Title = m.tool.Title + " profiles"
	m.setHelpKeys(keySwitch, keyEdit, keyDelete, keyBack)
	// Welcome a first-time user who has no profiles for this tool yet.
	if len(saved) == 0 && m.status == "" && m.tool.ApplyAuth != nil {
		m.setStatus(statusInfo, `No profiles yet — press enter on "Add new profile" to create one.`)
	}
}

// profileDetail is the second-line summary of a profile: its endpoint and model
// when recorded, falling back to the manifest label for captured profiles.
func (m *model) profileDetail(name string) string {
	if spec, ok := m.store.GetSpec(m.tool.Name, name); ok {
		url := m.tool.ResolveEndpoint(spec.Endpoint)
		if url == "" {
			url = "default endpoint"
		}
		model := spec.Model
		if model == "" {
			model = "no model override"
		}
		return url + " · " + model
	}
	if man, err := m.store.LoadManifest(m.tool.Name, name); err == nil && man.Label != "" {
		return man.Label
	}
	return "captured config"
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		return m, nil

	case fetchedMsg:
		// Hold a too-fast result until minLoadDuration so the loading screen never flickers.
		if elapsed := time.Since(m.fetchStart); elapsed < minLoadDuration {
			m.pending = &msg
			return m, tea.Tick(minLoadDuration-elapsed, func(time.Time) tea.Msg { return minLoadElapsedMsg{} })
		}
		return m.applyFetched(msg)

	case minLoadElapsedMsg:
		if m.pending == nil {
			return m, nil
		}
		result := *m.pending
		m.pending = nil
		return m.applyFetched(result)

	case spinner.TickMsg:
		if m.view != viewFetching {
			return m, nil // ignore stray ticks once we've left the loading screen
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		// ctrl+c is the only way to quit, from any screen.
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		if m.inputView() {
			return m.updateInput(msg)
		}
		if m.view == viewConfirmDelete {
			return m.updateConfirmDelete(msg)
		}
		if m.view == viewPickModel {
			return m.updatePickModel(msg)
		}
		switch msg.String() {
		case "esc":
			return m.onEsc()
		case "enter":
			return m.onEnter()
		case "e":
			if m.view == viewEditForm {
				// Inside the edit form, "e" opens the highlighted field for editing.
				if it, ok := m.list.SelectedItem().(item); ok {
					return m.onEditFormSelect(it.value)
				}
				return m, nil
			}
			if m.view == viewProfiles && m.tool.ApplyAuth != nil {
				if it, ok := m.list.SelectedItem().(item); ok && it.value != addSentinel {
					if it.value == profile.DefaultName {
						m.setStatus(statusInfo, "the default profile can't be edited")
						return m, nil
					}
					sp, _ := m.store.GetSpec(m.tool.Name, it.value)
					m.wiz = wizard{name: it.value, origName: it.value, edit: true,
						endpoint: sp.Endpoint, key: sp.Key, model: sp.Model}
					m.editField = "" // fresh edit starts on the first field
					m.view = viewEditForm
					m.clearStatus()
					m.loadEditForm()
					return m, nil
				}
			}
		case "d":
			if m.view == viewProfiles {
				if it, ok := m.list.SelectedItem().(item); ok && it.value != addSentinel {
					if it.value == profile.DefaultName {
						m.setStatus(statusInfo, "the default profile can't be deleted")
						return m, nil
					}
					m.delTarget = it.value
					m.view = viewConfirmDelete
					m.clearStatus()
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
		m.clearStatus()
		m.loadTools()
		m.resize() // banner returns → shrink the list
	case viewEditForm:
		m.editField = ""               // leaving edit expires the field focus → next entry starts on Name
		return m.finishAdd(m.wiz.name) // back applies edits automatically — no explicit save step
	case viewPickModel:
		if m.fromForm {
			m.fromForm = false
			m.editField = fieldModel // land back on the Model row
			m.view = viewEditForm
			m.loadEditForm()
		} else {
			m.view = viewProfiles
			m.setStatus(statusInfo, "cancelled")
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
		t := m.findTool(it.value)
		if t == nil || t.Detected == nil || !t.Detected() {
			m.setStatus(statusInfo, it.title+" isn't installed yet — see the README to set it up")
			return m, nil
		}
		m.tool = t
		m.view = viewProfiles
		m.clearStatus()
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
			m.setStatus(statusErr, err.Error())
		} else {
			info, _ := m.tool.Describe()
			m.setStatus(statusOK, fmt.Sprintf("Switched to %s (%s · %s). Backup: %s",
				it.value, info.Endpoint, secret.Mask(info.Secret), backup))
			m.loadProfiles()
		}

	case viewEditForm:
		// "e" edits the highlighted field; enter is inert (esc saves & backs out).
		return m, nil

	case viewPickModel:
		if it.value == skipModel {
			m.wiz.model = ""
		} else {
			m.wiz.model = it.value
		}
		if m.fromForm {
			m.fromForm = false
			m.editField = fieldModel // land back on the Model row
			m.view = viewEditForm
			m.setStatus(statusInfo, "model set to "+m.wiz.model)
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
