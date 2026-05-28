package dashboard

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"sshdash/internal/apiparsers"
	"sshdash/internal/config"
)

type settingsAction int

const (
	settingsActionNone settingsAction = iota
	settingsActionClose
	settingsActionSave
)

type settingsFieldKind int

const (
	settingsFieldText settingsFieldKind = iota
	settingsFieldBool
	settingsFieldInfo
)

type settingsListKind int

const (
	settingsListNone settingsListKind = iota
	settingsListService
	settingsListAPI
	settingsListHeader
)

type settingsModel struct {
	pending      config.Config
	section      int
	field        int
	editing      bool
	editingField settingsField
	input        textinput.Model
	notice       string
}

type settingsField struct {
	Label     string
	Value     string
	Secret    bool
	Kind      settingsFieldKind
	ListKind  settingsListKind
	ListIndex int
	HeaderKey string
	Apply     func(*config.Config, string) error
	Toggle    func(*config.Config)
}

var settingsSections = []string{"Refresh", "Services", "APIs", "Docker", "Proxmox", "PBS", "Weather", "Media"}

func newSettingsModel(cfg config.Config) settingsModel {
	input := textinput.New()
	input.Prompt = ""
	input.CharLimit = 4096
	input.Width = 48

	return settingsModel{
		pending: cloneConfig(cfg),
		input:   input,
	}
}

func (m settingsModel) Update(msg tea.KeyMsg) (settingsModel, settingsAction) {
	if m.editing {
		switch msg.String() {
		case "enter":
			if err := m.commitEdit(); err != nil {
				m.notice = err.Error()
				return m, settingsActionNone
			}
			m.notice = ""
			return m, settingsActionNone
		case "ctrl+s":
			if err := m.commitEdit(); err != nil {
				m.notice = err.Error()
				return m, settingsActionNone
			}
			m.notice = "saving..."
			return m, settingsActionSave
		case "esc":
			m.editing = false
			m.input.Blur()
			return m, settingsActionNone
		}

		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		_ = cmd
		return m, settingsActionNone
	}

	switch msg.String() {
	case "esc":
		return m, settingsActionClose
	case "ctrl+s":
		m.notice = "saving..."
		return m, settingsActionSave
	case "tab", "right", "l":
		m.section = (m.section + 1) % len(settingsSections)
		m.field = 0
	case "shift+tab", "left":
		m.section = (m.section + len(settingsSections) - 1) % len(settingsSections)
		m.field = 0
	case "up", "k":
		m.field = max(0, m.field-1)
	case "down", "j":
		m.field++
	case "enter":
		field, ok := m.currentField()
		if !ok || field.Kind == settingsFieldInfo {
			break
		}
		if field.Kind == settingsFieldBool {
			field.Toggle(&m.pending)
			m.notice = ""
			break
		}
		m.editing = true
		m.editingField = field
		m.input.SetValue(field.Value)
		m.input.CursorEnd()
		m.input.Focus()
	case "a":
		m.addListItem()
	case "d":
		m.deleteListItem()
	case "h":
		m.addHeader()
	}

	m.clampField()
	return m, settingsActionNone
}

func (m settingsModel) View(width, height int) string {
	m.clampField()

	leftWidth := 18
	rightWidth := max(42, min(width-leftWidth-8, 88))
	left := m.renderSections(leftWidth)
	right := m.renderFields(rightWidth)

	return lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
}

func (m settingsModel) renderSections(width int) string {
	lines := []string{settingsLabelStyle.Render("Settings")}
	for i, section := range settingsSections {
		label := fmt.Sprintf(" %s ", section)
		if i == m.section {
			lines = append(lines, settingsActiveStyle.Width(width).Render(label))
			continue
		}
		lines = append(lines, mutedStyle.Width(width).Render(label))
	}
	return settingsPaneStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func (m settingsModel) renderFields(width int) string {
	fields := m.fields()
	lines := []string{settingsLabelStyle.Render(settingsSections[m.section])}
	if m.notice != "" {
		lines = append(lines, m.notice)
	}
	if len(fields) == 0 {
		lines = append(lines, mutedStyle.Render("No fields in this section."))
	}

	for i, field := range fields {
		prefix := "  "
		style := settingsValueStyle
		if i == m.field {
			prefix = "> "
			style = settingsActiveStyle
		}

		value := field.Value
		if field.Secret {
			value = maskSecret(value)
		}
		if m.editing && i == m.field {
			value = m.input.View()
		}
		if field.Kind == settingsFieldInfo {
			lines = append(lines, mutedStyle.Render(prefix+field.Label))
			continue
		}
		lines = append(lines, style.Render(fmt.Sprintf("%s%s: %s", prefix, field.Label, value)))
	}

	if settingsSections[m.section] == "APIs" {
		lines = append(lines, "", mutedStyle.Render("Parsers: "+strings.Join(apiparsers.Names(), ", ")))
	}
	return settingsPaneStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func (m *settingsModel) commitEdit() error {
	if m.editingField.Apply == nil {
		m.editing = false
		m.input.Blur()
		return nil
	}
	if err := m.editingField.Apply(&m.pending, strings.TrimSpace(m.input.Value())); err != nil {
		return err
	}
	m.editing = false
	m.input.Blur()
	return nil
}

func (m *settingsModel) clampField() {
	fields := m.fields()
	if len(fields) == 0 {
		m.field = 0
		return
	}
	if m.field >= len(fields) {
		m.field = len(fields) - 1
	}
	if m.field < 0 {
		m.field = 0
	}
}

func (m settingsModel) currentField() (settingsField, bool) {
	fields := m.fields()
	if m.field < 0 || m.field >= len(fields) {
		return settingsField{}, false
	}
	return fields[m.field], true
}

func (m *settingsModel) addListItem() {
	switch settingsSections[m.section] {
	case "Services":
		m.pending.Services = append(m.pending.Services, config.ServiceConfig{
			Name:    uniqueName("new-service", len(m.pending.Services)+1),
			Address: "127.0.0.1:80",
			Timeout: 2 * time.Second,
		})
		m.field = max(0, len(m.pending.Services)-1) * 3
		m.notice = "service added"
	case "APIs":
		m.pending.APIs = append(m.pending.APIs, config.APIConfig{
			Name:    uniqueName("new-api", len(m.pending.APIs)+1),
			URL:     "https://example.com",
			Parser:  "default",
			Timeout: 5 * time.Second,
		})
		m.field = m.apiFieldStart(len(m.pending.APIs) - 1)
		m.notice = "api added"
	}
}

func (m *settingsModel) deleteListItem() {
	field, ok := m.currentField()
	if !ok {
		return
	}
	switch field.ListKind {
	case settingsListService:
		i := field.ListIndex
		if i >= 0 && i < len(m.pending.Services) {
			m.pending.Services = append(m.pending.Services[:i], m.pending.Services[i+1:]...)
			m.notice = "service deleted"
		}
	case settingsListAPI:
		i := field.ListIndex
		if i >= 0 && i < len(m.pending.APIs) {
			m.pending.APIs = append(m.pending.APIs[:i], m.pending.APIs[i+1:]...)
			m.notice = "api deleted"
		}
	case settingsListHeader:
		apiIndex := field.ListIndex
		if apiIndex >= 0 && apiIndex < len(m.pending.APIs) {
			delete(m.pending.APIs[apiIndex].Headers, field.HeaderKey)
			m.notice = "header deleted"
		}
	}
}

func (m *settingsModel) addHeader() {
	field, ok := m.currentField()
	if !ok || field.ListIndex < 0 || field.ListIndex >= len(m.pending.APIs) {
		return
	}
	if m.pending.APIs[field.ListIndex].Headers == nil {
		m.pending.APIs[field.ListIndex].Headers = map[string]string{}
	}
	key := uniqueHeaderKey(m.pending.APIs[field.ListIndex].Headers)
	m.pending.APIs[field.ListIndex].Headers[key] = "value"
	m.notice = "header added"
}

func (m settingsModel) apiFieldStart(apiIndex int) int {
	field := 0
	for i := 0; i < apiIndex && i < len(m.pending.APIs); i++ {
		field += 4 + len(m.pending.APIs[i].Headers)*2
	}
	return field
}

func (m settingsModel) fields() []settingsField {
	switch settingsSections[m.section] {
	case "Refresh":
		return []settingsField{
			textField("refresh", durationValue(m.pending.Refresh), false, func(cfg *config.Config, value string) error {
				duration, err := parseOptionalDuration(value)
				if err != nil {
					return err
				}
				cfg.Refresh = duration
				return nil
			}),
		}
	case "Services":
		return m.serviceFields()
	case "APIs":
		return m.apiFields()
	case "Docker":
		return dockerFields(m.pending)
	case "Proxmox":
		return proxmoxFields(m.pending)
	case "PBS":
		return pbsFields(m.pending)
	case "Weather":
		return weatherFields(m.pending)
	case "Media":
		return mediaFields(m.pending)
	default:
		return nil
	}
}

func (m settingsModel) serviceFields() []settingsField {
	if len(m.pending.Services) == 0 {
		return []settingsField{infoField("No services yet. Press a to add one.")}
	}

	fields := make([]settingsField, 0, len(m.pending.Services)*3)
	for i, service := range m.pending.Services {
		index := i
		prefix := fmt.Sprintf("service %d ", i+1)
		fields = append(fields,
			listTextField(prefix+"name", service.Name, false, settingsListService, index, "", func(cfg *config.Config, value string) error {
				cfg.Services[index].Name = value
				return nil
			}),
			listTextField(prefix+"address", service.Address, false, settingsListService, index, "", func(cfg *config.Config, value string) error {
				cfg.Services[index].Address = value
				return nil
			}),
			listTextField(prefix+"timeout", durationValue(service.Timeout), false, settingsListService, index, "", func(cfg *config.Config, value string) error {
				duration, err := parseOptionalDuration(value)
				if err != nil {
					return err
				}
				cfg.Services[index].Timeout = duration
				return nil
			}),
		)
	}
	return fields
}

func (m settingsModel) apiFields() []settingsField {
	if len(m.pending.APIs) == 0 {
		return []settingsField{infoField("No APIs yet. Press a to add one.")}
	}

	fields := []settingsField{}
	for i, api := range m.pending.APIs {
		index := i
		prefix := fmt.Sprintf("api %d ", i+1)
		fields = append(fields,
			listTextField(prefix+"name", api.Name, false, settingsListAPI, index, "", func(cfg *config.Config, value string) error {
				cfg.APIs[index].Name = value
				return nil
			}),
			listTextField(prefix+"url", api.URL, false, settingsListAPI, index, "", func(cfg *config.Config, value string) error {
				cfg.APIs[index].URL = value
				return nil
			}),
			listTextField(prefix+"parser", api.Parser, false, settingsListAPI, index, "", func(cfg *config.Config, value string) error {
				cfg.APIs[index].Parser = value
				return nil
			}),
			listTextField(prefix+"timeout", durationValue(api.Timeout), false, settingsListAPI, index, "", func(cfg *config.Config, value string) error {
				duration, err := parseOptionalDuration(value)
				if err != nil {
					return err
				}
				cfg.APIs[index].Timeout = duration
				return nil
			}),
		)

		keys := sortedHeaderKeys(api.Headers)
		for _, key := range keys {
			headerKey := key
			fields = append(fields,
				listTextField(prefix+"header key", headerKey, false, settingsListHeader, index, headerKey, func(cfg *config.Config, value string) error {
					value = strings.TrimSpace(value)
					if value == "" {
						return fmt.Errorf("header key cannot be empty")
					}
					oldValue := cfg.APIs[index].Headers[headerKey]
					delete(cfg.APIs[index].Headers, headerKey)
					cfg.APIs[index].Headers[value] = oldValue
					return nil
				}),
				listTextField(prefix+"header value", api.Headers[headerKey], true, settingsListHeader, index, headerKey, func(cfg *config.Config, value string) error {
					cfg.APIs[index].Headers[headerKey] = value
					return nil
				}),
			)
		}
	}
	return fields
}

func dockerFields(cfg config.Config) []settingsField {
	return []settingsField{
		boolField("enabled", cfg.Docker.Enabled, func(cfg *config.Config) { cfg.Docker.Enabled = !cfg.Docker.Enabled }),
		textField("name", cfg.Docker.Name, false, func(cfg *config.Config, value string) error { cfg.Docker.Name = value; return nil }),
		textField("url", cfg.Docker.URL, false, func(cfg *config.Config, value string) error { cfg.Docker.URL = value; return nil }),
		textField("timeout", durationValue(cfg.Docker.Timeout), false, func(cfg *config.Config, value string) error {
			duration, err := parseOptionalDuration(value)
			if err != nil {
				return err
			}
			cfg.Docker.Timeout = duration
			return nil
		}),
		boolField("show stopped", cfg.Docker.ShowStopped, func(cfg *config.Config) { cfg.Docker.ShowStopped = !cfg.Docker.ShowStopped }),
	}
}

func proxmoxFields(cfg config.Config) []settingsField {
	return []settingsField{
		boolField("enabled", cfg.Proxmox.Enabled, func(cfg *config.Config) { cfg.Proxmox.Enabled = !cfg.Proxmox.Enabled }),
		textField("name", cfg.Proxmox.Name, false, func(cfg *config.Config, value string) error { cfg.Proxmox.Name = value; return nil }),
		textField("url", cfg.Proxmox.URL, false, func(cfg *config.Config, value string) error { cfg.Proxmox.URL = value; return nil }),
		textField("token", cfg.Proxmox.Token, true, func(cfg *config.Config, value string) error { cfg.Proxmox.Token = value; return nil }),
		textField("timeout", durationValue(cfg.Proxmox.Timeout), false, func(cfg *config.Config, value string) error {
			duration, err := parseOptionalDuration(value)
			if err != nil {
				return err
			}
			cfg.Proxmox.Timeout = duration
			return nil
		}),
		textField("mode", cfg.Proxmox.Mode, false, func(cfg *config.Config, value string) error { cfg.Proxmox.Mode = value; return nil }),
		textField("nodes", strings.Join(cfg.Proxmox.Nodes, ", "), false, func(cfg *config.Config, value string) error {
			cfg.Proxmox.Nodes = splitList(value)
			return nil
		}),
		boolField("skip tls verify", cfg.Proxmox.SkipTLSVerify, func(cfg *config.Config) { cfg.Proxmox.SkipTLSVerify = !cfg.Proxmox.SkipTLSVerify }),
	}
}

func pbsFields(cfg config.Config) []settingsField {
	return []settingsField{
		boolField("enabled", cfg.ProxmoxBackup.Enabled, func(cfg *config.Config) { cfg.ProxmoxBackup.Enabled = !cfg.ProxmoxBackup.Enabled }),
		textField("name", cfg.ProxmoxBackup.Name, false, func(cfg *config.Config, value string) error { cfg.ProxmoxBackup.Name = value; return nil }),
		textField("url", cfg.ProxmoxBackup.URL, false, func(cfg *config.Config, value string) error { cfg.ProxmoxBackup.URL = value; return nil }),
		textField("token", cfg.ProxmoxBackup.Token, true, func(cfg *config.Config, value string) error { cfg.ProxmoxBackup.Token = value; return nil }),
		textField("datastores", strings.Join(cfg.ProxmoxBackup.Datastores, ", "), false, func(cfg *config.Config, value string) error {
			cfg.ProxmoxBackup.Datastores = splitList(value)
			return nil
		}),
		textField("timeout", durationValue(cfg.ProxmoxBackup.Timeout), false, func(cfg *config.Config, value string) error {
			duration, err := parseOptionalDuration(value)
			if err != nil {
				return err
			}
			cfg.ProxmoxBackup.Timeout = duration
			return nil
		}),
		boolField("skip tls verify", cfg.ProxmoxBackup.SkipTLSVerify, func(cfg *config.Config) {
			cfg.ProxmoxBackup.SkipTLSVerify = !cfg.ProxmoxBackup.SkipTLSVerify
		}),
	}
}

func weatherFields(cfg config.Config) []settingsField {
	return []settingsField{
		boolField("enabled", cfg.Weather.Enabled, func(cfg *config.Config) { cfg.Weather.Enabled = !cfg.Weather.Enabled }),
		textField("name", cfg.Weather.Name, false, func(cfg *config.Config, value string) error { cfg.Weather.Name = value; return nil }),
		textField("location", cfg.Weather.Location, false, func(cfg *config.Config, value string) error { cfg.Weather.Location = value; return nil }),
		textField("url", cfg.Weather.URL, false, func(cfg *config.Config, value string) error { cfg.Weather.URL = value; return nil }),
		textField("timeout", durationValue(cfg.Weather.Timeout), false, func(cfg *config.Config, value string) error {
			duration, err := parseOptionalDuration(value)
			if err != nil {
				return err
			}
			cfg.Weather.Timeout = duration
			return nil
		}),
	}
}

func mediaFields(cfg config.Config) []settingsField {
	fields := []settingsField{
		boolField("enabled", cfg.Media.Enabled, func(cfg *config.Config) { cfg.Media.Enabled = !cfg.Media.Enabled }),
		textField("name", cfg.Media.Name, false, func(cfg *config.Config, value string) error { cfg.Media.Name = value; return nil }),
	}
	fields = append(fields, mediaServiceFields("jellyfin", cfg.Media.Jellyfin, true)...)
	fields = append(fields, mediaServiceFields("radarr", cfg.Media.Radarr, true)...)
	fields = append(fields, mediaServiceFields("sonarr", cfg.Media.Sonarr, true)...)
	fields = append(fields, mediaServiceFields("jellyseerr", cfg.Media.Jellyseerr, true)...)
	return fields
}

func mediaServiceFields(name string, service config.MediaServiceConfig, showAPIKey bool) []settingsField {
	return []settingsField{
		boolField(name+" enabled", service.Enabled, func(cfg *config.Config) { mediaService(cfg, name).Enabled = !mediaService(cfg, name).Enabled }),
		textField(name+" name", service.Name, false, func(cfg *config.Config, value string) error { mediaService(cfg, name).Name = value; return nil }),
		textField(name+" url", service.URL, false, func(cfg *config.Config, value string) error { mediaService(cfg, name).URL = value; return nil }),
		textField(name+" api key", service.APIKey, showAPIKey, func(cfg *config.Config, value string) error { mediaService(cfg, name).APIKey = value; return nil }),
		textField(name+" timeout", durationValue(service.Timeout), false, func(cfg *config.Config, value string) error {
			duration, err := parseOptionalDuration(value)
			if err != nil {
				return err
			}
			mediaService(cfg, name).Timeout = duration
			return nil
		}),
	}
}

func mediaService(cfg *config.Config, name string) *config.MediaServiceConfig {
	switch name {
	case "jellyfin":
		return &cfg.Media.Jellyfin
	case "radarr":
		return &cfg.Media.Radarr
	case "sonarr":
		return &cfg.Media.Sonarr
	default:
		return &cfg.Media.Jellyseerr
	}
}

func textField(label, value string, secret bool, apply func(*config.Config, string) error) settingsField {
	return settingsField{Label: label, Value: value, Secret: secret, Kind: settingsFieldText, Apply: apply, ListIndex: -1}
}

func listTextField(label, value string, secret bool, list settingsListKind, index int, headerKey string, apply func(*config.Config, string) error) settingsField {
	field := textField(label, value, secret, apply)
	field.ListKind = list
	field.ListIndex = index
	field.HeaderKey = headerKey
	return field
}

func boolField(label string, value bool, toggle func(*config.Config)) settingsField {
	return settingsField{
		Label:     label,
		Value:     boolValue(value),
		Kind:      settingsFieldBool,
		Toggle:    toggle,
		ListIndex: -1,
	}
}

func infoField(label string) settingsField {
	return settingsField{Label: label, Kind: settingsFieldInfo, ListIndex: -1}
}

func durationValue(duration time.Duration) string {
	if duration == 0 {
		return ""
	}
	return duration.String()
}

func parseOptionalDuration(value string) (time.Duration, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, err
	}
	if duration < 0 {
		return 0, fmt.Errorf("duration cannot be negative")
	}
	return duration, nil
}

func boolValue(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func maskSecret(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return "****"
	}
	return strings.Repeat("*", 8) + value[len(value)-4:]
}

func splitList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}

func sortedHeaderKeys(headers map[string]string) []string {
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func uniqueHeaderKey(headers map[string]string) string {
	for i := 1; ; i++ {
		key := uniqueName("Header-Name", i)
		if _, ok := headers[key]; !ok {
			return key
		}
	}
}

func uniqueName(prefix string, n int) string {
	if n <= 1 {
		return prefix
	}
	return fmt.Sprintf("%s-%d", prefix, n)
}

func cloneConfig(cfg config.Config) config.Config {
	next := cfg
	next.Services = append([]config.ServiceConfig(nil), cfg.Services...)
	next.APIs = make([]config.APIConfig, 0, len(cfg.APIs))
	for _, api := range cfg.APIs {
		nextAPI := api
		if api.Headers != nil {
			nextAPI.Headers = make(map[string]string, len(api.Headers))
			for key, value := range api.Headers {
				nextAPI.Headers[key] = value
			}
		}
		next.APIs = append(next.APIs, nextAPI)
	}
	next.Proxmox.Nodes = append([]string(nil), cfg.Proxmox.Nodes...)
	next.ProxmoxBackup.Datastores = append([]string(nil), cfg.ProxmoxBackup.Datastores...)
	return next
}
