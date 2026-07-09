package tui

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"charon/internal/models"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sahilm/fuzzy"
)

// minLoadDuration is the floor the loading screen stays up so a fast fetch doesn't flicker.
const minLoadDuration = 1 * time.Second

// fetchedMsg carries the async result of a models.Fetch call.
type fetchedMsg struct {
	list []string
	err  error
}

// minLoadElapsedMsg fires when the min-load window closes on a result parked in model.pending.
type minLoadElapsedMsg struct{}

func fetchModelsCmd(provider, endpoint, key string) tea.Cmd {
	return func() tea.Msg {
		l, err := models.Fetch(models.Provider(provider), endpoint, key)
		return fetchedMsg{list: l, err: err}
	}
}

// loadingMessages are playful lines shown while fetching, one picked at random per fetch.
var loadingMessages = []string{
	"fetching models, almost there…",
	"ferrying your request across the Styx…",
	"summoning the model list…",
	"asking the endpoint nicely…",
	"warming up the engines…",
	"charting the crossing…",
	"counting the models…",
	"reticulating splines…",
	"the ferry is departing, hold tight…",
	"hang on, nearly across…",
}

func randomLoadingMsg() string {
	return loadingMessages[rand.Intn(len(loadingMessages))]
}

// beginFetch shows the loading screen and starts the throttle, spinner, and model fetch.
func (m *model) beginFetch() tea.Cmd {
	m.view = viewFetching
	m.fetchStart = time.Now()
	m.pending = nil
	m.loadingMsg = randomLoadingMsg()
	m.spinner = newSpinner()
	return tea.Batch(m.spinner.Tick, fetchModelsCmd(m.tool.Provider, m.wiz.endpoint, m.wiz.key))
}

// applyFetched moves to the model picker on success, or recovers gracefully on error.
func (m model) applyFetched(msg fetchedMsg) (tea.Model, tea.Cmd) {
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
}

// filterModels fuzzy-matches query against model ids, ranked best-first (empty = all).
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

// renderModels rebuilds the picker rows for the current query (echoed in the title),
// always keeping a trailing "skip" row.
func (m *model) renderModels() {
	ids := filterModels(m.allModels, m.modelFilter)
	var items []list.Item
	for _, id := range ids {
		title := id
		isChosen := id == m.wiz.model
		if isChosen {
			title = "✓ " + id // mark the profile's currently selected model
		}
		items = append(items, item{title: title, desc: "", value: id, active: isChosen})
	}
	// A blank divider sets the skip row apart from the models (only when not searching).
	if len(ids) > 0 && m.modelFilter == "" {
		items = append(items, item{value: sepSentinel})
	}
	skipTitle := "(skip — no model override)"
	skipChosen := m.wiz.model == ""
	if skipChosen {
		skipTitle = "✓ " + skipTitle
	}
	items = append(items, item{title: skipTitle, desc: "", value: skipModel, active: skipChosen})
	skipIndex := len(items) - 1
	m.list.SetDelegate(themedCompactDelegate())
	m.list.SetItems(items)
	// No search: land on the checked row; while searching, the best match is on top.
	selectedIndex := 0
	if m.modelFilter == "" {
		if m.wiz.model == "" {
			selectedIndex = skipIndex // the trailing skip row
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

// updatePickModel drives the picker: printable keys search, ctrl+r refetches,
// nav keys fall through to the list, enter/esc choose or cancel.
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
		cmd := m.beginFetch()
		return m, cmd
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
	before := m.list.Index()
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	m.skipSeparators(before)
	return m, cmd
}
