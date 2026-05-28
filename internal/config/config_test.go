package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
