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

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Run starts the interactive menu against the given store.
func Run(store *profile.Store, version string) error {
	_, err := tea.NewProgram(newModel(store, version), tea.WithAltScreen()).Run()
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
	viewDupName       // backup: name the duplicated proxy profile
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
	sepSentinel = "\x00sep"     // a blank divider row (inert; cursor skips it)
)

// isSentinel reports whether v is a synthetic action row rather than a profile.
func isSentinel(v string) bool {
	return v == addSentinel || v == skipModel || v == sepSentinel
}

type item struct {
	title, desc string
	value       string
	active      bool // the already-picked profile — stays primary-colored even off-cursor
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

// Contextual key bindings shown in the list's help footer (and "?"-expanded).
var (
	keySwitch = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "switch"))
	keyEdit   = key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit"))
	keyBackup = key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "backup"))
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
	dupSource string // profile being duplicated by the backup flow

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
	version     string
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

func newModel(store *profile.Store, version string) model {
	l := list.New(nil, themedDelegate(), 0, 0)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.InfiniteScrolling = true
	l.KeyMap.Quit.SetEnabled(false) // "q"/"esc" must not quit; only ctrl+c does
	l.Styles.Title = titleStyle
	l.Styles.TitleBar = l.Styles.TitleBar.Padding(0, 0, 1, 0)
	// Keep the paginator and help footer mostly in the terminal's palette (muted
	// gray labels; bubbles' defaults are fixed RGB grays, so every piece needs
	// the override), but call out the actionable keys themselves in the accent.
	l.Styles.HelpStyle = l.Styles.HelpStyle.Foreground(colorMuted)
	l.Styles.PaginationStyle = l.Styles.PaginationStyle.Foreground(colorMuted)
	l.Styles.ArabicPagination = lipgloss.NewStyle().Foreground(colorMuted)
	l.Styles.NoItems = lipgloss.NewStyle().Foreground(colorMuted)
	l.Help.Styles.ShortKey = l.Help.Styles.ShortKey.Foreground(colorAccent)
	l.Help.Styles.ShortDesc = l.Help.Styles.ShortDesc.Foreground(colorMuted)
	l.Help.Styles.ShortSeparator = l.Help.Styles.ShortSeparator.Foreground(colorMuted)
	l.Help.Styles.FullKey = l.Help.Styles.FullKey.Foreground(colorAccent)
	l.Help.Styles.FullDesc = l.Help.Styles.FullDesc.Foreground(colorMuted)
	l.Help.Styles.FullSeparator = l.Help.Styles.FullSeparator.Foreground(colorMuted)
	l.Help.Styles.Ellipsis = l.Help.Styles.Ellipsis.Foreground(colorMuted)

	ti := textinput.New()
	ti.CharLimit = 200
	// The focused field itself carries the accent color, so it's obvious which
	// input has keyboard focus.
	ti.PromptStyle = ti.PromptStyle.Foreground(colorAccent)
	ti.PlaceholderStyle = ti.PlaceholderStyle.Foreground(colorMuted)
	// A solid, steady block cursor rather than the default blink: blinking
	// alternates between a filled block and plain text, which reads as the
	// cursor flickering in and out rather than marking a stable position.
	ti.Cursor.Style = ti.Cursor.Style.Foreground(colorAccent)
	ti.Cursor.SetMode(cursor.CursorStatic)

	m := model{store: store, allTools: tools.All(), view: viewTools, list: l, input: ti, spinner: newSpinner(), version: version}
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
	case viewAddEndpoint, viewAddKey, viewAddName, viewDupName, viewEditField:
		return true
	}
	return false
}

// selectedProfile returns the highlighted profile row, or false (with a status hint)
// when the cursor is on a sentinel row like "Add new profile" or the divider — the
// shared guard for the e/b/d shortcuts, which only act on real profiles.
func (m *model) selectedProfile() (item, bool) {
	it, ok := m.list.SelectedItem().(item)
	if !ok || isSentinel(it.value) {
		m.setStatus(statusInfo, "select a profile first")
		return item{}, false
	}
	return it, true
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
			if drift, _ := m.store.Drift(t); drift {
				active += " ⚠"
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

// loadProfiles rebuilds the profile list for the current tool. selectName, if
// non-empty, is the row the cursor should land on (e.g. the profile just
// edited or backed up); otherwise the cursor defaults to the active profile.
// This keeps an edit or backup from silently relocating the cursor onto
// whatever happens to be active — only an explicit switch should do that.
func (m *model) loadProfiles(selectName string) {
	var items []list.Item
	active := m.store.Active(m.tool.Name)
	saved := m.store.List(m.tool.Name)
	drift, _ := m.store.Drift(m.tool) // live config changed outside charon?
	target := selectName
	if target == "" {
		target = active
	}
	selectedIndex := 0
	for i, name := range saved {
		// ✓ marks the active profile; url and model show on the line below.
		title := name
		isActive := name == active
		if isActive {
			title = "✓ " + title
			if drift {
				title += "  ⚠ modified" // snapshot no longer matches live config
			}
		}
		if name == target {
			selectedIndex = i
		}
		items = append(items, item{title: title, desc: m.profileDetail(name), value: name, active: isActive})
	}
	// The add row sits below the profiles, set off by a thin divider.
	if m.tool.ApplyAuth != nil {
		if len(saved) > 0 {
			items = append(items, item{value: sepSentinel}) // gap between profiles and actions
		}
		items = append(items, item{title: "＋ Add new profile…", value: addSentinel})
	}
	m.list.SetDelegate(themedDelegate()) // two-line rows show each profile's url and model
	m.list.SetItems(items)
	m.list.Select(selectedIndex)
	m.list.Title = m.tool.Title + " profiles"
	m.setHelpKeys(keySwitch, keyEdit, keyBackup, keyDelete, keyBack)
	// Welcome a first-time user who has no profiles for this tool yet.
	if len(saved) == 0 && m.status == "" && m.tool.ApplyAuth != nil {
		m.setStatus(statusInfo, `No profiles yet — press enter on "Add new profile" to create one.`)
	}
}

// profileDetail is the second-line summary of a profile: its endpoint, model, and
// reasoning-effort level when known, falling back to the manifest label for profiles
// charon captured rather than created itself.
func (m *model) profileDetail(name string) string {
	model, effort := m.store.ProfileModelEffort(m.tool, name)
	// The active profile's on-disk snapshot only resyncs when you switch away (see
	// refreshMergerArtifacts) — an external /model change while it's active leaves the
	// snapshot stale. Read live config instead so the list matches what's actually set.
	if name == m.store.Active(m.tool.Name) && m.tool.Describe != nil {
		if info, err := m.tool.Describe(); err == nil {
			if info.Model != "" {
				model = info.Model
			}
			if info.Effort != "" {
				effort = info.Effort
			}
		}
	}
	spec, hasSpec := m.store.GetSpec(m.tool.Name, name)
	if hasSpec && model == "" {
		model = spec.Model
	}
	if !hasSpec && model == "" && effort == "" {
		if man, err := m.store.LoadManifest(m.tool.Name, name); err == nil && man.Label != "" {
			return man.Label
		}
		return "captured config"
	}
	url := "default endpoint"
	if hasSpec {
		if u := m.tool.ResolveEndpoint(spec.Endpoint); u != "" {
			url = u
		}
	}
	if model == "" {
		model = "no model override"
	}
	detail := url + " · " + model
	if effort != "" {
		detail += " · effort: " + effort
	}
	return detail
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
				return m.onEditKey()
			}
		case "b":
			if m.view == viewProfiles {
				if it, ok := m.selectedProfile(); ok {
					return m.startBackup(it.value)
				}
				return m, nil
			}
		case "d":
			if m.view == viewProfiles {
				return m.onDeleteKey()
			}
		}
	}

	before := m.list.Index()
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	if m.view == viewProfiles {
		m.skipSeparators(before)
	}
	return m, cmd
}

// onEditKey opens the edit form for the highlighted profile ("e" on the profiles
// view). The default profile and captured login backups have nothing to edit.
func (m model) onEditKey() (tea.Model, tea.Cmd) {
	it, ok := m.selectedProfile()
	if !ok {
		return m, nil
	}
	if it.value == profile.DefaultName {
		m.setStatus(statusInfo, "the default profile can't be edited")
		return m, nil
	}
	sp, ok := m.store.GetSpec(m.tool.Name, it.value)
	if !ok {
		// OAuth / captured backups have no endpoint/key to change.
		m.setStatus(statusInfo, "this login backup has no editable settings")
		return m, nil
	}
	// spec.Model is frozen at Add-time; prefer whatever the list just showed (the
	// captured snapshot, or live config for the active profile) so edit matches it.
	model := sp.Model
	if snapModel, _ := m.store.ProfileModelEffort(m.tool, it.value); snapModel != "" {
		model = snapModel
	}
	if it.value == m.store.Active(m.tool.Name) && m.tool.Describe != nil {
		if info, err := m.tool.Describe(); err == nil && info.Model != "" {
			model = info.Model
		}
	}
	m.wiz = wizard{name: it.value, origName: it.value, edit: true,
		endpoint: sp.Endpoint, key: sp.Key, model: model}
	m.editField = "" // fresh edit starts on the first field
	m.view = viewEditForm
	m.clearStatus()
	m.loadEditForm()
	return m, nil
}

// onDeleteKey arms the confirm-delete prompt for the highlighted profile ("d" on
// the profiles view). The default profile is not deletable.
func (m model) onDeleteKey() (tea.Model, tea.Cmd) {
	it, ok := m.selectedProfile()
	if !ok {
		return m, nil
	}
	if it.value == profile.DefaultName {
		m.setStatus(statusInfo, "the default profile can't be deleted")
		return m, nil
	}
	m.delTarget = it.value
	m.view = viewConfirmDelete
	m.clearStatus()
	return m, nil
}

// startBackup routes the "b" shortcut by profile type: an OAuth/original login is
// snapshotted straight away, named after its account (non-editable); an API-proxy
// profile opens a name prompt to make an editable, deletable duplicate.
func (m model) startBackup(name string) (tea.Model, tea.Cmd) {
	if _, ok := m.store.GetSpec(m.tool.Name, name); ok || name == profile.DefaultName {
		// API proxy or default → duplicate with a numbered name the user can adjust.
		m.dupSource = name
		m.view = viewDupName
		m.startInput("new profile name", false)
		m.input.SetValue(m.nextCopyName(name))
		m.clearStatus()
		return m, textinput.Blink
	}
	// OAuth / original login → capture the current account, named by its email.
	saved, err := m.store.SaveCurrentAccount(m.tool)
	if err != nil {
		m.setStatus(statusErr, err.Error())
	} else {
		m.setStatus(statusOK, "Backed up login as "+saved)
	}
	// Stay on the original row rather than jumping to the new backup.
	m.loadProfiles(name)
	return m, nil
}

// nextCopyName returns the first free "<base>-N" name (N starting at 2).
func (m *model) nextCopyName(base string) string {
	for i := 2; ; i++ {
		cand := fmt.Sprintf("%s-%d", base, i)
		if !m.store.Exists(m.tool.Name, cand) {
			return cand
		}
	}
}

// skipSeparators nudges the cursor off an inert divider row after a move, continuing
// in the direction of travel so the blank gap never traps the selection.
func (m *model) skipSeparators(before int) {
	if it, ok := m.list.SelectedItem().(item); !ok || it.value != sepSentinel {
		return
	}
	if m.list.Index() >= before {
		m.list.CursorDown()
	} else {
		m.list.CursorUp()
	}
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
			m.loadProfiles("")
		}
	}
	return m, nil
}

func (m model) onEnter() (tea.Model, tea.Cmd) {
	it, ok := m.list.SelectedItem().(item)
	if !ok {
		return m, nil
	}
	if it.value == sepSentinel {
		return m, nil // the blank divider is inert
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
		m.loadProfiles("") // land on the active profile
		m.resize()         // banner hidden → grow the list

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
			m.loadProfiles(it.value)
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
