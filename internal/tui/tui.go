// Package tui provides the interactive arrow-key menu for aies.
package tui

import (
	"fmt"
	"strings"

	"charon/internal/models"
	"charon/internal/profile"
	"charon/internal/secret"
	"charon/internal/tools"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sahilm/fuzzy"
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
	viewSaveName      // quick-save current config ("s")
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

// Contextual key bindings surfaced in the list's help footer. Defining them as
// bindings (rather than baking them into the title) lets the themed footer show
// them and lets "?" expand the full list.
var (
	keySwitch = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "switch"))
	keyEdit   = key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit"))
	keySave   = key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "save current"))
	keyDelete = key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete"))
	keyBack   = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back"))
	keyOpen   = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open"))
	keyChoose = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "choose"))
	// keyFilter's key never matches a real press — it exists only to advertise
	// the type-to-search behaviour of the model picker in the help footer.
	keyFilter  = key.NewBinding(key.WithKeys("\x00filter"), key.WithHelp("type", "search"))
	keyRefresh = key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "refresh"))
)

// wizardStep maps an add-flow view to its 1-based step index, the total number
// of steps, and a short label. total is 0 for views with no progress indicator
// (edit and non-wizard screens).
func wizardStep(v view) (n, total int, label string) {
	switch v {
	case viewAddEndpoint:
		return 1, 4, "API base URL"
	case viewAddKey:
		return 2, 4, "API key"
	case viewFetching, viewPickModel:
		return 3, 4, "choose a model"
	case viewAddName:
		return 4, 4, "name the profile"
	}
	return 0, 0, ""
}

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
	delTarget string // profile name pending delete confirmation
	quitting  bool   // ctrl+d pressed once; a second press quits

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

// setHelpKeys registers the contextual key bindings shown in the list's help
// footer (short form) and its "?"-expanded full form.
func (m *model) setHelpKeys(bindings ...key.Binding) {
	m.list.AdditionalShortHelpKeys = func() []key.Binding { return bindings }
	m.list.AdditionalFullHelpKeys = func() []key.Binding { return bindings }
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
	m.list.SetDelegate(themedDelegate()) // two-line rows show each field's value
	m.list.SetItems([]list.Item{
		item{title: "Name", desc: m.wiz.name, value: fieldName},
		item{title: "URL", desc: endpoint, value: fieldURL},
		item{title: "Token", desc: token, value: fieldToken},
		item{title: "Model", desc: modelVal + "  (enter to fetch & pick)", value: fieldModel},
		item{title: "✓ Save changes", desc: "apply and switch to this profile", value: fieldSave},
	})
	m.list.Title = fmt.Sprintf("Edit %s / %s", m.tool.Title, m.wiz.name)
	// A fresh entry (no field targeted) starts on the first row; otherwise keep
	// the cursor on the field just visited so diving into a field (or the model
	// picker) and coming back lands on that row, not the first one.
	m.list.Select(0)
	m.selectByValue(m.editField)
	m.setHelpKeys(
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "edit field")),
		keyBack,
	)
}

// selectByValue moves the list cursor to the row whose value matches v. A miss
// (including v == "") leaves the default selection in place.
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
	for i, t := range tools.All() {
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
		// One line per profile: a leading ✓ marks the active one, and any saved
		// label is appended inline after the name.
		title := name
		if man, err := m.store.LoadManifest(m.tool.Name, name); err == nil && man.Label != "" {
			title += " — " + man.Label
		}
		if name == active {
			title = "✓ " + title
			selectedIndex = i // land the cursor on the active profile
		}
		items = append(items, item{title: title, value: name})
	}
	if m.tool.ApplyAuth != nil {
		items = append(items, item{title: "＋ Add new profile…", value: addSentinel})
	}
	m.list.SetDelegate(themedCompactDelegate())
	m.list.SetItems(items)
	m.list.Select(selectedIndex)
	m.list.Title = m.tool.Title + " profiles"
	m.setHelpKeys(keySwitch, keyEdit, keySave, keyDelete, keyBack)
	// Welcome a first-time user who has no profiles for this tool yet.
	if len(saved) == 0 && m.status == "" && m.tool.ApplyAuth != nil {
		m.setStatus(statusInfo, `No profiles yet — press enter on "Add new profile" to create one.`)
	}
}

// filterModels fuzzy-matches query against all model ids, returning them ranked
// best-first. An empty query returns the full list unchanged.
func filterModels(all []string, query string) []string {
	q := strings.TrimSpace(query)
	if q == "" {
		return all
	}
	matches := fuzzy.Find(q, all)
	out := make([]string, len(matches))
	for i, mt := range matches {
		out[i] = mt.Str
	}
	return out
}

// showModels installs a freshly fetched model list and resets the search query.
func (m *model) showModels(ids []string) {
	m.allModels = ids
	m.modelFilter = ""
	m.renderModels()
}

// renderModels rebuilds the picker rows from allModels filtered by the current
// query. There is no visible filter input: typing narrows the list in place and
// the active query is echoed in the title. A trailing "skip" row is always kept.
func (m *model) renderModels() {
	ids := filterModels(m.allModels, m.modelFilter)
	var items []list.Item
	for _, id := range ids {
		title := id
		if id == m.wiz.model {
			title = "✓ " + id // mark the profile's currently selected model
		}
		items = append(items, item{title: title, desc: "", value: id})
	}
	skipTitle := "(skip — no model override)"
	if m.wiz.model == "" {
		skipTitle = "✓ " + skipTitle
	}
	items = append(items, item{title: skipTitle, desc: "", value: skipModel})
	m.list.SetDelegate(themedCompactDelegate())
	m.list.SetItems(items)
	// With no active search, land the cursor on the checked row — the current
	// model, or the skip row when there's no override. Once a query is typed,
	// ranked-best-first means the top row is the target.
	selectedIndex := 0
	if m.modelFilter == "" {
		if m.wiz.model == "" {
			selectedIndex = len(ids) // the trailing skip row
		} else {
			for i, id := range ids {
				if id == m.wiz.model {
					selectedIndex = i
					break
				}
			}
		}
	}
	m.list.Select(selectedIndex)
	title := m.tool.Title + " — choose a model"
	if m.modelFilter != "" {
		title += fmt.Sprintf(" · search: %s (%d)", m.modelFilter, len(ids))
	}
	m.list.Title = title
	m.setHelpKeys(keyChoose, keyFilter, keyRefresh, keyBack)
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
				m.setStatus(statusErr, msg.err.Error())
				m.view = viewEditForm
				m.loadEditForm()
				return m, nil
			}
			// Model list unavailable; proceed without a model override.
			m.wiz.model = ""
			if m.wiz.edit {
				m.setStatus(statusErr, msg.err.Error())
				return m.finishAdd(m.wiz.name)
			}
			m.setStatus(statusErr, msg.err.Error()+" — you can name it without a model")
			m.view = viewAddName
			m.startInput("profile name", false)
			return m, textinput.Blink
		}
		m.view = viewPickModel
		m.setStatus(statusInfo, fmt.Sprintf("%d models found", len(msg.list)))
		m.showModels(msg.list)
		return m, nil

	case tea.KeyMsg:
		// ctrl+d quits, but only on a second consecutive press ("press again to
		// quit"). Any other key disarms the pending quit. Handled before the
		// per-view dispatch so it works from every screen, inputs included.
		if msg.Type == tea.KeyCtrlD {
			return m.onQuit()
		}
		if m.quitting {
			m.quitting = false
			m.clearStatus()
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
		case "ctrl+c":
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
					m.editField = "" // fresh edit starts on the first field
					m.view = viewEditForm
					m.clearStatus()
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

// onQuit arms the two-step quit: the first ctrl+d prompts for confirmation, the
// second (with no intervening key) actually exits.
func (m model) onQuit() (tea.Model, tea.Cmd) {
	if m.quitting {
		return m, tea.Quit
	}
	m.quitting = true
	m.setStatus(statusInfo, "Press ctrl+d again to quit")
	return m, nil
}

// updateConfirmDelete handles the y/n prompt guarding profile deletion.
func (m model) updateConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		name := m.delTarget
		m.delTarget = ""
		m.view = viewProfiles
		if err := m.store.Remove(m.tool.Name, name); err != nil {
			m.setStatus(statusErr, err.Error())
		} else {
			m.setStatus(statusOK, "Deleted "+name)
		}
		m.loadProfiles()
		return m, nil
	case "n", "N", "esc", "q":
		m.delTarget = ""
		m.view = viewProfiles
		m.setStatus(statusInfo, "cancelled")
		m.loadProfiles()
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

// updatePickModel drives the model picker. Printable keys build a fuzzy-search
// query in place (no visible input); ctrl+r refetches the list; navigation keys
// fall through to the list; enter/esc choose or cancel.
func (m model) updatePickModel(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		return m.onEsc()
	case tea.KeyEnter:
		return m.onEnter()
	case tea.KeyCtrlR:
		if m.wiz.endpoint == "" || m.wiz.key == "" {
			m.setStatus(statusInfo, "set URL and token first, then refresh")
			return m, nil
		}
		m.view = viewFetching
		m.setStatus(statusInfo, "Refreshing models…")
		return m, fetchModelsCmd(m.tool.Provider, m.wiz.endpoint, m.wiz.key)
	case tea.KeyBackspace:
		if r := []rune(m.modelFilter); len(r) > 0 {
			m.modelFilter = string(r[:len(r)-1])
			m.renderModels()
		}
		return m, nil
	case tea.KeyRunes:
		m.modelFilter += string(msg.Runes)
		m.renderModels()
		return m, nil
	case tea.KeySpace:
		m.modelFilter += " "
		m.renderModels()
		return m, nil
	}
	// Arrows, page keys, home/end, ctrl+n/ctrl+p: let the list move the cursor.
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) onEsc() (tea.Model, tea.Cmd) {
	switch m.view {
	case viewProfiles:
		m.view = viewTools
		m.clearStatus()
		m.loadTools()
		m.resize() // banner returns → shrink the list
	case viewEditForm:
		m.editField = "" // leaving edit expires the field focus → next entry starts on Name
		m.view = viewProfiles
		m.setStatus(statusInfo, "cancelled")
		m.loadProfiles()
		m.selectByValue(m.wiz.origName) // stay on the profile we were editing
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
		t := tools.Find(it.value)
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
		return m.onEditFormSelect(it.value)

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
		m.editField = fieldModel
		if m.wiz.endpoint == "" || m.wiz.key == "" {
			m.setStatus(statusInfo, "set URL and token first, then fetch models")
			return m, nil
		}
		m.fromForm = true
		m.view = viewFetching
		m.setStatus(statusInfo, "Fetching models…")
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
		m.setStatus(statusInfo, "cancelled")
		m.loadProfiles()
		return m, nil
	case "enter":
		val := m.input.Value()
		switch m.view {
		case viewEditField:
			refetch := false
			switch m.editField {
			case fieldName:
				if val != "" {
					m.wiz.name = val
				}
			case fieldURL:
				if m.wiz.endpoint != val {
					m.wiz.endpoint = val
					refetch = true
				}
			case fieldToken:
				if m.wiz.key != val {
					m.wiz.key = val
					refetch = true
				}
			}
			if refetch && m.wiz.endpoint != "" && m.wiz.key != "" {
				m.fromForm = true
				m.view = viewFetching
				m.setStatus(statusInfo, "Fetching models…")
				return m, fetchModelsCmd(m.tool.Provider, m.wiz.endpoint, m.wiz.key)
			}
			m.view = viewEditForm
			m.loadEditForm()
			return m, nil

		case viewSaveName:
			if val == "" {
				m.setStatus(statusInfo, "name required")
				return m, nil
			}
			if err := m.store.Save(m.tool, val, val, ""); err != nil {
				m.setStatus(statusErr, err.Error())
			} else {
				m.setStatus(statusOK, "Saved current config as "+val)
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
				m.setStatus(statusInfo, "key required")
				return m, nil
			}
			m.wiz.key = val
			m.view = viewFetching
			m.setStatus(statusInfo, "Fetching models…")
			return m, fetchModelsCmd(m.tool.Provider, m.wiz.endpoint, m.wiz.key)

		case viewAddName:
			if val == "" {
				m.setStatus(statusInfo, "name required")
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
		m.setStatus(statusErr, err.Error())
	} else if err := m.store.SaveWithSpec(m.tool, name,
		profile.Spec{Endpoint: m.wiz.endpoint, Key: m.wiz.key, Model: m.wiz.model}); err != nil {
		m.setStatus(statusErr, "saved config but failed to record profile: "+err.Error())
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
		model := m.wiz.model
		if model == "" {
			model = "no model override"
		}
		m.setStatus(statusOK, fmt.Sprintf("%s %s (%s · %s)", verb, name, m.wiz.endpoint, model))
	}
	m.view = viewProfiles
	m.loadProfiles()
	return m, nil
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
			promptStyle.Render("Fetching models from "+m.wiz.endpoint+" …") +
			"\n\n" + hintStyle.Render("please wait")
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
