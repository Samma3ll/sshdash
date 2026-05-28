package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadRejectsUnknownAPIParser(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte(`
apis:
  - name: bad-api
    url: https://example.com
    parser: missing_parser
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load returned nil error")
	}
	if !strings.Contains(err.Error(), "unknown api parser missing_parser") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadDefaultsAPIParser(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte(`
apis:
  - name: default-api
    url: https://example.com
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIs[0].Parser != "default" {
		t.Fatalf("Parser = %q, want default", cfg.APIs[0].Parser)
	}
}

func TestLoadRejectsEnabledDockerWithoutURL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte(`
docker:
  enabled: true
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "docker.url") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRejectsEnabledProxmoxWithoutToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte(`
proxmox:
  enabled: true
  url: https://pve.example:8006
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "proxmox.url and proxmox.token") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRejectsEnabledPBSWithoutDatastores(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte(`
proxmox_backup:
  enabled: true
  url: https://pbs.example:8007
  token: PBSAPIToken=user@pbs!tokenid=secret
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "proxmox_backup.datastores") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadDefaultsMediaServices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte(`
media:
  enabled: true
  jellyfin:
    enabled: true
    url: http://jellyfin.example
    api_key: secret
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Media.Name != "media" {
		t.Fatalf("Media.Name = %q, want media", cfg.Media.Name)
	}
	if cfg.Media.Jellyfin.Name != "jellyfin" {
		t.Fatalf("Media.Jellyfin.Name = %q, want jellyfin", cfg.Media.Jellyfin.Name)
	}
	if cfg.Media.Jellyfin.Timeout == 0 {
		t.Fatal("Media.Jellyfin.Timeout was not defaulted")
	}
}

func TestLoadRejectsEnabledMediaServiceWithoutAPIKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte(`
media:
  enabled: true
  sonarr:
    enabled: true
    url: http://sonarr.example
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "media.sonarr.api_key") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadNormalizesCommonTokenIDPlaceholder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte(`
proxmox:
  enabled: true
  url: https://pve.example:8006
  token: "PVEAPIToken=user@pve!dash!tokenid=secret"
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Proxmox.Token != "PVEAPIToken=user@pve!dash=secret" {
		t.Fatalf("Token = %q", cfg.Proxmox.Token)
	}
}

func TestLoadNormalizesPBSAPITokenSeparator(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte(`
proxmox_backup:
  enabled: true
  url: https://pbs.example:8007
  token: "PBSAPIToken=user@pbs!dash=secret"
  datastores:
    - main
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ProxmoxBackup.Token != "PBSAPIToken=user@pbs!dash:secret" {
		t.Fatalf("Token = %q", cfg.ProxmoxBackup.Token)
	}
}

func TestSaveRoundTripsReadableDurations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := Config{
		Refresh: 45 * time.Second,
		Services: []ServiceConfig{
			{Name: "router", Address: "192.168.1.1:80", Timeout: 3 * time.Second},
		},
		APIs: []APIConfig{
			{
				Name:    "status",
				URL:     "https://example.com/status",
				Parser:  "default",
				Headers: map[string]string{"Authorization": "Bearer secret"},
				Timeout: 7 * time.Second,
			},
		},
		Docker: DockerConfig{
			Enabled:     true,
			Name:        "docker",
			URL:         "http://docker.example:2375",
			Timeout:     6 * time.Second,
			ShowStopped: true,
		},
		Weather: WeatherConfig{
			Enabled:  true,
			Name:     "weather",
			Location: "Amsterdam",
			Timeout:  4 * time.Second,
		},
	}

	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"refresh: 45s", "timeout: 3s", "timeout: 7s", "timeout: 6s", "timeout: 4s"} {
		if !strings.Contains(text, want) {
			t.Fatalf("saved config missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "45000000000") {
		t.Fatalf("saved config used numeric duration:\n%s", text)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Refresh != 45*time.Second {
		t.Fatalf("Refresh = %s", loaded.Refresh)
	}
	if loaded.APIs[0].Headers["Authorization"] != "Bearer secret" {
		t.Fatalf("header was not round-tripped: %#v", loaded.APIs[0].Headers)
	}
}

func TestSaveRoundTripsAllCardConfigs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := Config{
		Refresh: 45 * time.Second,
		Services: []ServiceConfig{
			{Name: "router", Address: "192.168.1.1:80", Timeout: 3 * time.Second},
		},
		APIs: []APIConfig{
			{
				Name:    "status",
				URL:     "https://example.com/status",
				Parser:  "default",
				Headers: map[string]string{"Authorization": "Bearer secret"},
				Timeout: 7 * time.Second,
			},
		},
		Docker: DockerConfig{
			Enabled:     true,
			Name:        "docker-card",
			URL:         "http://docker.example:2375",
			Timeout:     6 * time.Second,
			ShowStopped: true,
		},
		Proxmox: ProxmoxConfig{
			Enabled:       true,
			Name:          "pve-card",
			URL:           "https://pve.example:8006",
			Token:         "PVEAPIToken=user@pve!dash=secret",
			Timeout:       8 * time.Second,
			Mode:          "cluster",
			Nodes:         []string{"pve-1", "pve-2"},
			SkipTLSVerify: true,
		},
		ProxmoxBackup: ProxmoxBackupConfig{
			Enabled:       true,
			Name:          "pbs-card",
			URL:           "https://pbs.example:8007",
			Token:         "PBSAPIToken=user@pbs!dash:secret",
			Datastores:    []string{"main", "archive"},
			Timeout:       9 * time.Second,
			SkipTLSVerify: true,
		},
		Weather: WeatherConfig{
			Enabled:  true,
			Name:     "outside",
			Location: "Amsterdam",
			URL:      "https://weather.example",
			Timeout:  4 * time.Second,
		},
		Media: MediaConfig{
			Enabled: true,
			Name:    "media-card",
			Jellyfin: MediaServiceConfig{
				Enabled: true,
				Name:    "jellyfin",
				URL:     "http://jellyfin.example",
				APIKey:  "jellyfin-secret",
				Timeout: 5 * time.Second,
			},
			Radarr: MediaServiceConfig{
				Enabled: true,
				Name:    "radarr",
				URL:     "http://radarr.example",
				APIKey:  "radarr-secret",
				Timeout: 6 * time.Second,
			},
			Sonarr: MediaServiceConfig{
				Enabled: true,
				Name:    "sonarr",
				URL:     "http://sonarr.example",
				APIKey:  "sonarr-secret",
				Timeout: 7 * time.Second,
			},
			Jellyseerr: MediaServiceConfig{
				Enabled: true,
				Name:    "jellyseerr",
				URL:     "http://jellyseerr.example",
				APIKey:  "jellyseerr-secret",
				Timeout: 8 * time.Second,
			},
		},
	}

	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Docker.Name != cfg.Docker.Name || loaded.Docker.URL != cfg.Docker.URL || loaded.Docker.Timeout != cfg.Docker.Timeout || !loaded.Docker.ShowStopped {
		t.Fatalf("docker did not round-trip: %#v", loaded.Docker)
	}
	if loaded.Proxmox.Token != cfg.Proxmox.Token || strings.Join(loaded.Proxmox.Nodes, ",") != "pve-1,pve-2" || !loaded.Proxmox.SkipTLSVerify {
		t.Fatalf("proxmox did not round-trip: %#v", loaded.Proxmox)
	}
	if loaded.ProxmoxBackup.Token != cfg.ProxmoxBackup.Token || strings.Join(loaded.ProxmoxBackup.Datastores, ",") != "main,archive" || !loaded.ProxmoxBackup.SkipTLSVerify {
		t.Fatalf("proxmox backup did not round-trip: %#v", loaded.ProxmoxBackup)
	}
	if loaded.Weather.Name != cfg.Weather.Name || loaded.Weather.URL != cfg.Weather.URL || loaded.Weather.Location != cfg.Weather.Location {
		t.Fatalf("weather did not round-trip: %#v", loaded.Weather)
	}
	if loaded.Media.Name != cfg.Media.Name ||
		loaded.Media.Jellyfin.APIKey != cfg.Media.Jellyfin.APIKey ||
		loaded.Media.Radarr.Timeout != cfg.Media.Radarr.Timeout ||
		loaded.Media.Sonarr.URL != cfg.Media.Sonarr.URL ||
		loaded.Media.Jellyseerr.APIKey != cfg.Media.Jellyseerr.APIKey {
		t.Fatalf("media did not round-trip: %#v", loaded.Media)
	}
}

func TestSaveRejectsInvalidConfigWithoutModifyingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	original := []byte("refresh: 15s\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	err := Save(path, Config{
		Services: []ServiceConfig{{Name: "missing-address"}},
	})
	if err == nil || !strings.Contains(err.Error(), "services require name and address") {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(original) {
		t.Fatalf("file changed after failed save:\n%s", data)
	}
}

func TestSaveDefaultsReloadConsistently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := Config{
		Services: []ServiceConfig{{Name: "dns", Address: "1.1.1.1:53"}},
		APIs:     []APIConfig{{Name: "api", URL: "https://example.com"}},
	}

	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Refresh != 30*time.Second {
		t.Fatalf("Refresh = %s, want 30s", loaded.Refresh)
	}
	if loaded.Services[0].Timeout != 2*time.Second {
		t.Fatalf("Service timeout = %s, want 2s", loaded.Services[0].Timeout)
	}
	if loaded.APIs[0].Parser != "default" {
		t.Fatalf("API parser = %q, want default", loaded.APIs[0].Parser)
	}
	if loaded.APIs[0].Timeout != 5*time.Second {
		t.Fatalf("API timeout = %s, want 5s", loaded.APIs[0].Timeout)
	}
}
