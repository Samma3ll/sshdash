package dashboard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"sshdash/internal/config"
)

func TestSettingsShortcutAndEscape(t *testing.T) {
	m := newModel(testConfig(), "")

	m = updateModel(t, m, keyRunes("s"))
	if m.mode != modeSettings {
		t.Fatalf("mode = %v, want settings", m.mode)
	}

	m = updateModel(t, m, keyType(tea.KeyEsc))
	if m.mode != modeDashboard {
		t.Fatalf("mode = %v, want dashboard", m.mode)
	}
}

func TestSettingsAddsEditsAndDeletesServicesAndAPIs(t *testing.T) {
	m := newModel(testConfig(), "")
	m = updateModel(t, m, keyRunes("s"))

	m.settings.section = 1
	m = updateModel(t, m, keyRunes("a"))
	if got := len(m.settings.pending.Services); got != 2 {
		t.Fatalf("services len = %d, want 2", got)
	}
	m.settings.field = 3
	m = setActiveSettingsField(t, m, "nas")
	if got := m.settings.pending.Services[1].Name; got != "nas" {
		t.Fatalf("service name = %q, want nas", got)
	}
	m = updateModel(t, m, keyRunes("d"))
	if got := len(m.settings.pending.Services); got != 1 {
		t.Fatalf("services len after delete = %d, want 1", got)
	}

	m.settings.section = 2
	m.settings.field = 0
	m = updateModel(t, m, keyRunes("a"))
	if got := len(m.settings.pending.APIs); got != 2 {
		t.Fatalf("apis len = %d, want 2", got)
	}
	m.settings.field = m.settings.apiFieldStart(1)
	m = setActiveSettingsField(t, m, "status")
	if got := m.settings.pending.APIs[1].Name; got != "status" {
		t.Fatalf("api name = %q, want status", got)
	}
	m = updateModel(t, m, keyRunes("h"))
	if got := len(m.settings.pending.APIs[1].Headers); got != 1 {
		t.Fatalf("headers len = %d, want 1", got)
	}
	m = updateModel(t, m, keyRunes("d"))
	if got := len(m.settings.pending.APIs); got != 1 {
		t.Fatalf("apis len after delete = %d, want 1", got)
	}
}

func TestSettingsExposesAllCardConfigFields(t *testing.T) {
	m := newSettingsModel(fullCardConfig())

	expected := map[string][]string{
		"Refresh":  {"refresh"},
		"Services": {"service 1 name", "service 1 address", "service 1 timeout"},
		"APIs":     {"api 1 name", "api 1 url", "api 1 parser", "api 1 timeout", "api 1 header key", "api 1 header value"},
		"Docker":   {"enabled", "name", "url", "timeout", "show stopped"},
		"Proxmox":  {"enabled", "name", "url", "token", "timeout", "mode", "nodes", "skip tls verify"},
		"PBS":      {"enabled", "name", "url", "token", "datastores", "timeout", "skip tls verify"},
		"Weather":  {"enabled", "name", "location", "url", "timeout"},
		"Media": {
			"enabled", "name",
			"jellyfin enabled", "jellyfin name", "jellyfin url", "jellyfin api key", "jellyfin timeout",
			"radarr enabled", "radarr name", "radarr url", "radarr api key", "radarr timeout",
			"sonarr enabled", "sonarr name", "sonarr url", "sonarr api key", "sonarr timeout",
			"jellyseerr enabled", "jellyseerr name", "jellyseerr url", "jellyseerr api key", "jellyseerr timeout",
		},
	}

	for sectionIndex, section := range settingsSections {
		m.section = sectionIndex
		labels := settingsFieldLabels(m.fields())
		for _, want := range expected[section] {
			if !containsLabel(labels, want) {
				t.Fatalf("section %s missing field %q; got %#v", section, want, labels)
			}
		}
	}
}

func TestSettingsSaveWritesReloadsAndRebuilds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := testConfig()
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	m := newModel(loaded, path)
	m = updateModel(t, m, keyRunes("s"))
	m.settings.pending.Refresh = 3 * time.Second
	m.settings.pending.Services = append(m.settings.pending.Services, config.ServiceConfig{
		Name:    "nas",
		Address: "192.168.1.2:443",
		Timeout: time.Second,
	})

	updated, cmd := m.Update(keyType(tea.KeyCtrlS))
	m = updated.(model)
	if cmd == nil {
		t.Fatal("save did not return a command")
	}
	updated, cmd = m.Update(cmd())
	m = updated.(model)
	if cmd == nil {
		t.Fatal("save success did not trigger a refresh command")
	}

	if m.mode != modeDashboard {
		t.Fatalf("mode = %v, want dashboard", m.mode)
	}
	if m.refresh != 3*time.Second {
		t.Fatalf("refresh = %s, want 3s", m.refresh)
	}
	if got := len(m.checkers); got != 3 {
		t.Fatalf("checkers len = %d, want 3", got)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "refresh: 3s") || !strings.Contains(string(data), "name: nas") {
		t.Fatalf("saved config missing updates:\n%s", data)
	}
}

func TestSettingsSaveRejectsInvalidPendingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := testConfig()
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	m := newModel(loaded, path)
	originalCheckers := len(m.checkers)
	m = updateModel(t, m, keyRunes("s"))
	m.settings.pending.Services[0].Address = ""

	updated, cmd := m.Update(keyType(tea.KeyCtrlS))
	m = updated.(model)
	if cmd == nil {
		t.Fatal("save did not return a command")
	}
	updated, _ = m.Update(cmd())
	m = updated.(model)

	if m.mode != modeSettings {
		t.Fatalf("mode = %v, want settings", m.mode)
	}
	if len(m.checkers) != originalCheckers {
		t.Fatalf("checkers len = %d, want %d", len(m.checkers), originalCheckers)
	}
	if !strings.Contains(m.settings.notice, "services require name and address") {
		t.Fatalf("notice = %q", m.settings.notice)
	}
}

func testConfig() config.Config {
	return config.Config{
		Refresh: 2 * time.Second,
		Services: []config.ServiceConfig{
			{Name: "router", Address: "192.168.1.1:80", Timeout: time.Second},
		},
		APIs: []config.APIConfig{
			{Name: "api", URL: "https://example.com", Parser: "default", Timeout: time.Second},
		},
	}
}

func fullCardConfig() config.Config {
	cfg := testConfig()
	cfg.Docker = config.DockerConfig{
		Enabled:     true,
		Name:        "docker",
		URL:         "http://docker.example:2375",
		Timeout:     5 * time.Second,
		ShowStopped: true,
	}
	cfg.Proxmox = config.ProxmoxConfig{
		Enabled:       true,
		Name:          "pve",
		URL:           "https://pve.example:8006",
		Token:         "PVEAPIToken=user@pve!dash=secret",
		Timeout:       8 * time.Second,
		Mode:          "cluster",
		Nodes:         []string{"pve-1"},
		SkipTLSVerify: true,
	}
	cfg.ProxmoxBackup = config.ProxmoxBackupConfig{
		Enabled:       true,
		Name:          "pbs",
		URL:           "https://pbs.example:8007",
		Token:         "PBSAPIToken=user@pbs!dash:secret",
		Datastores:    []string{"main"},
		Timeout:       8 * time.Second,
		SkipTLSVerify: true,
	}
	cfg.Weather = config.WeatherConfig{
		Enabled:  true,
		Name:     "weather",
		Location: "Amsterdam",
		URL:      "https://weather.example",
		Timeout:  5 * time.Second,
	}
	cfg.Media = config.MediaConfig{
		Enabled: true,
		Name:    "media",
		Jellyfin: config.MediaServiceConfig{
			Enabled: true,
			Name:    "jellyfin",
			URL:     "http://jellyfin.example",
			APIKey:  "jellyfin-secret",
			Timeout: 5 * time.Second,
		},
		Radarr: config.MediaServiceConfig{
			Enabled: true,
			Name:    "radarr",
			URL:     "http://radarr.example",
			APIKey:  "radarr-secret",
			Timeout: 5 * time.Second,
		},
		Sonarr: config.MediaServiceConfig{
			Enabled: true,
			Name:    "sonarr",
			URL:     "http://sonarr.example",
			APIKey:  "sonarr-secret",
			Timeout: 5 * time.Second,
		},
		Jellyseerr: config.MediaServiceConfig{
			Enabled: true,
			Name:    "jellyseerr",
			URL:     "http://jellyseerr.example",
			APIKey:  "jellyseerr-secret",
			Timeout: 5 * time.Second,
		},
	}
	cfg.APIs[0].Headers = map[string]string{"Authorization": "Bearer secret"}
	return cfg
}

func updateModel(t *testing.T, m model, msg tea.Msg) model {
	t.Helper()
	updated, _ := m.Update(msg)
	next, ok := updated.(model)
	if !ok {
		t.Fatalf("updated model type = %T", updated)
	}
	return next
}

func setActiveSettingsField(t *testing.T, m model, value string) model {
	t.Helper()
	m = updateModel(t, m, keyType(tea.KeyEnter))
	if !m.settings.editing {
		t.Fatal("settings field did not enter edit mode")
	}
	m.settings.input.SetValue(value)
	return updateModel(t, m, keyType(tea.KeyEnter))
}

func keyRunes(value string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(value)}
}

func keyType(key tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: key}
}

func settingsFieldLabels(fields []settingsField) []string {
	labels := make([]string, 0, len(fields))
	for _, field := range fields {
		labels = append(labels, field.Label)
	}
	return labels
}

func containsLabel(labels []string, want string) bool {
	for _, label := range labels {
		if label == want {
			return true
		}
	}
	return false
}
