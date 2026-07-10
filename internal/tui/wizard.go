package tui

import (
	"fmt"
	"strings"

	"charon/internal/profile"
	"charon/internal/secret"
	"charon/internal/tools"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	fieldName  = "\x00name"
	fieldURL   = "\x00url"
	fieldToken = "\x00token"
	fieldModel = "\x00model"
)

type wizard struct {
	endpoint, key, model string
	name                 string // target profile name when editing
	origName             string // pre-edit name, to clean up on rename
	edit                 bool   // true = overwrite an existing profile
}

// wizardStep maps an add-flow view to its step index, total, and label (total 0 = no progress).
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
		item{title: "Model", desc: modelVal + "  (e to fetch & pick)", value: fieldModel},
	})
	m.list.Title = fmt.Sprintf("Edit %s / %s", m.tool.Title, m.wiz.name)
	// Land on the field last visited; a fresh edit falls back to the first row.
	m.list.Select(0)
	m.selectByValue(m.editField)
	m.setHelpKeys(
		key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit field")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "save & back")),
	)
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
		cmd := m.beginFetch()
		return m, cmd
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
		src := m.dupSource
		m.dupSource = ""
		m.view = viewProfiles
		m.setStatus(statusInfo, "cancelled")
		m.loadProfiles(src) // land back on the profile that was being duplicated, if any
		return m, nil
	case "enter":
		val := m.input.Value()
		switch m.view {
		case viewEditField:
			refetch := false
			switch m.editField {
			case fieldName:
				val = strings.TrimSpace(val)
				if val == "" {
					m.setStatus(statusErr, "name is required")
					return m, nil
				}
				m.wiz.name = val
			case fieldURL:
				val = strings.TrimSpace(val)
				if err := tools.ValidateEndpoint(val); err != nil {
					m.setStatus(statusErr, err.Error())
					return m, nil
				}
				if m.wiz.endpoint != val {
					m.wiz.endpoint = val
					refetch = true
				}
			case fieldToken:
				val = strings.TrimSpace(val)
				if err := tools.ValidateKey(val); err != nil {
					m.setStatus(statusErr, err.Error())
					return m, nil
				}
				if m.wiz.key != val {
					m.wiz.key = val
					refetch = true
				}
			}
			if refetch && m.wiz.endpoint != "" && m.wiz.key != "" {
				m.fromForm = true
				cmd := m.beginFetch()
				return m, cmd
			}
			m.clearStatus()
			m.view = viewEditForm
			m.loadEditForm()
			return m, nil

		case viewAddEndpoint:
			val = strings.TrimSpace(val)
			if err := tools.ValidateEndpoint(val); err != nil {
				m.setStatus(statusErr, err.Error())
				return m, nil
			}
			m.wiz.endpoint = m.tool.ResolveEndpoint(val) // blank accepts the provider default
			m.view = viewAddKey
			m.clearStatus()
			m.startInput("API key", true)
			return m, textinput.Blink

		case viewAddKey:
			val = strings.TrimSpace(val)
			if err := tools.ValidateKey(val); err != nil {
				m.setStatus(statusErr, err.Error())
				return m, nil
			}
			m.wiz.key = val
			m.clearStatus()
			cmd := m.beginFetch()
			return m, cmd

		case viewAddName:
			val = strings.TrimSpace(val)
			if val == "" {
				m.setStatus(statusErr, "name is required")
				return m, nil
			}
			return m.finishAdd(val)

		case viewDupName:
			val = strings.TrimSpace(val)
			if val == "" {
				m.setStatus(statusErr, "name is required")
				return m, nil
			}
			src := m.dupSource
			m.dupSource = ""
			m.view = viewProfiles
			if err := m.store.Duplicate(m.tool.Name, src, val); err != nil {
				m.setStatus(statusErr, err.Error())
			} else {
				m.setStatus(statusOK, "Duplicated "+src+" → "+val)
			}
			// Stay on the source row rather than jumping to the new duplicate.
			m.loadProfiles(src)
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// updateConfirmDelete handles the y/n prompt guarding profile deletion.
func (m model) updateConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		name := m.delTarget
		m.delTarget = ""
		m.view = viewProfiles
		if m.store.Active(m.tool.Name) == name {
			if _, err := m.store.Apply(m.tool, profile.DefaultName); err != nil {
				m.setStatus(statusErr, err.Error())
				m.loadProfiles(name)
				return m, nil
			}
		}
		if err := m.store.Remove(m.tool.Name, name); err != nil {
			m.setStatus(statusErr, err.Error())
			m.loadProfiles(name)
		} else {
			m.setStatus(statusOK, "Deleted "+name)
			m.loadProfiles("") // the row is gone; fall back to the active profile
		}
		return m, nil
	case "n", "N", "esc":
		name := m.delTarget
		m.delTarget = ""
		m.view = viewProfiles
		m.setStatus(statusInfo, "cancelled")
		m.loadProfiles(name)
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

// finishAdd applies the wizard's endpoint/key/model and snapshots it as the named
// profile — via EditProfile when editing, so a rename also cleans up the old name.
func (m model) finishAdd(name string) (tea.Model, tea.Cmd) {
	spec := profile.Spec{Endpoint: m.wiz.endpoint, Key: m.wiz.key, Model: m.wiz.model}
	verb := "Added"
	var err error
	if m.wiz.edit {
		verb = "Updated"
		err = m.store.EditProfile(m.tool, m.wiz.origName, name, spec, m.allModels...)
	} else {
		err = m.store.AddProfile(m.tool, name, spec, m.allModels...)
	}
	if err != nil {
		m.setStatus(statusErr, err.Error())
	} else {
		model := m.wiz.model
		if model == "" {
			model = "no model override"
		}
		m.setStatus(statusOK, fmt.Sprintf("%s %s (%s · %s)", verb, name, m.wiz.endpoint, model))
	}
	m.view = viewProfiles
	m.loadProfiles(name) // land on the profile just added/edited, not wherever is active
	return m, nil
}
