package config

import (
	"errors"
	"os"
	"strings"
	"time"

	"sshdash/internal/apiparsers"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server        ServerConfig        `yaml:"server"`
	Refresh       time.Duration       `yaml:"refresh"`
	Services      []ServiceConfig     `yaml:"services"`
	APIs          []APIConfig         `yaml:"apis"`
	Docker        DockerConfig        `yaml:"docker"`
	Proxmox       ProxmoxConfig       `yaml:"proxmox"`
	ProxmoxBackup ProxmoxBackupConfig `yaml:"proxmox_backup"`
	Weather       WeatherConfig       `yaml:"weather"`
}

type ServerConfig struct {
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	HostKeyPath string `yaml:"host_key_path"`
}

type ServiceConfig struct {
	Name    string        `yaml:"name"`
	Address string        `yaml:"address"`
	Timeout time.Duration `yaml:"timeout"`
}

type APIConfig struct {
	Name    string            `yaml:"name"`
	URL     string            `yaml:"url"`
	Parser  string            `yaml:"parser"`
	Headers map[string]string `yaml:"headers"`
	Timeout time.Duration     `yaml:"timeout"`
}

type DockerConfig struct {
	Enabled     bool          `yaml:"enabled"`
	Name        string        `yaml:"name"`
	URL         string        `yaml:"url"`
	Timeout     time.Duration `yaml:"timeout"`
	ShowStopped bool          `yaml:"show_stopped"`
}

type ProxmoxConfig struct {
	Enabled       bool          `yaml:"enabled"`
	Name          string        `yaml:"name"`
	URL           string        `yaml:"url"`
	Token         string        `yaml:"token"`
	Timeout       time.Duration `yaml:"timeout"`
	Mode          string        `yaml:"mode"`
	Nodes         []string      `yaml:"nodes"`
	SkipTLSVerify bool          `yaml:"skip_tls_verify"`
}

type ProxmoxBackupConfig struct {
	Enabled       bool          `yaml:"enabled"`
	Name          string        `yaml:"name"`
	URL           string        `yaml:"url"`
	Token         string        `yaml:"token"`
	Datastores    []string      `yaml:"datastores"`
	Timeout       time.Duration `yaml:"timeout"`
	SkipTLSVerify bool          `yaml:"skip_tls_verify"`
}

type WeatherConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Name     string        `yaml:"name"`
	Location string        `yaml:"location"`
	URL      string        `yaml:"url"`
	Timeout  time.Duration `yaml:"timeout"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}

	applyDefaults(&cfg)
	return cfg, validate(cfg)
}

func applyDefaults(cfg *Config) {
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 23234
	}
	if cfg.Server.HostKeyPath == "" {
		cfg.Server.HostKeyPath = ".ssh/sshdash_ed25519"
	}
	if cfg.Refresh == 0 {
		cfg.Refresh = 30 * time.Second
	}
	for i := range cfg.Services {
		if cfg.Services[i].Timeout == 0 {
			cfg.Services[i].Timeout = 2 * time.Second
		}
	}
	for i := range cfg.APIs {
		if cfg.APIs[i].Parser == "" {
			cfg.APIs[i].Parser = "default"
		}
		if cfg.APIs[i].Timeout == 0 {
			cfg.APIs[i].Timeout = 5 * time.Second
		}
	}
	if cfg.Docker.Enabled {
		if cfg.Docker.Name == "" {
			cfg.Docker.Name = "docker"
		}
		if cfg.Docker.Timeout == 0 {
			cfg.Docker.Timeout = 5 * time.Second
		}
	}
	if cfg.Proxmox.Enabled {
		if cfg.Proxmox.Name == "" {
			cfg.Proxmox.Name = "proxmox"
		}
		cfg.Proxmox.Token = normalizeAPIToken(cfg.Proxmox.Token)
		if cfg.Proxmox.Timeout == 0 {
			cfg.Proxmox.Timeout = 8 * time.Second
		}
		if cfg.Proxmox.Mode == "" {
			cfg.Proxmox.Mode = "cluster"
		}
	}
	if cfg.ProxmoxBackup.Enabled {
		if cfg.ProxmoxBackup.Name == "" {
			cfg.ProxmoxBackup.Name = "pbs"
		}
		cfg.ProxmoxBackup.Token = normalizePBSAPIToken(cfg.ProxmoxBackup.Token)
		if cfg.ProxmoxBackup.Timeout == 0 {
			cfg.ProxmoxBackup.Timeout = 8 * time.Second
		}
	}
	if cfg.Weather.Enabled {
		if cfg.Weather.Name == "" {
			cfg.Weather.Name = "weather"
		}
		if cfg.Weather.Timeout == 0 {
			cfg.Weather.Timeout = 5 * time.Second
		}
	}
}

func validate(cfg Config) error {
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		return errors.New("server.port must be between 1 and 65535")
	}
	for _, service := range cfg.Services {
		if service.Name == "" || service.Address == "" {
			return errors.New("services require name and address")
		}
	}
	for _, api := range cfg.APIs {
		if api.Name == "" || api.URL == "" {
			return errors.New("apis require name and url")
		}
		if !apiparsers.Exists(api.Parser) {
			return errors.New("unknown api parser " + api.Parser + "; available parsers: " + strings.Join(apiparsers.Names(), ", "))
		}
	}
	if cfg.Docker.Enabled && cfg.Docker.URL == "" {
		return errors.New("docker.url is required when docker.enabled is true")
	}
	if cfg.Proxmox.Enabled {
		if cfg.Proxmox.URL == "" || cfg.Proxmox.Token == "" {
			return errors.New("proxmox.url and proxmox.token are required when proxmox.enabled is true")
		}
		if cfg.Proxmox.Mode != "cluster" {
			return errors.New("proxmox.mode must be cluster")
		}
	}
	if cfg.ProxmoxBackup.Enabled {
		if cfg.ProxmoxBackup.URL == "" || cfg.ProxmoxBackup.Token == "" {
			return errors.New("proxmox_backup.url and proxmox_backup.token are required when proxmox_backup.enabled is true")
		}
		if len(cfg.ProxmoxBackup.Datastores) == 0 {
			return errors.New("proxmox_backup.datastores requires at least one datastore")
		}
	}
	if cfg.Weather.Enabled && cfg.Weather.Location == "" && cfg.Weather.URL == "" {
		return errors.New("weather.location or weather.url is required when weather.enabled is true")
	}
	return nil
}

func normalizeAPIToken(token string) string {
	return strings.Replace(token, "!tokenid=", "=", 1)
}

func normalizePBSAPIToken(token string) string {
	token = strings.Replace(token, "!tokenid=", ":", 1)
	if !strings.HasPrefix(token, "PBSAPIToken=") {
		return token
	}
	prefix := "PBSAPIToken="
	value := strings.TrimPrefix(token, prefix)
	if strings.Contains(value, ":") {
		return token
	}
	lastEquals := strings.LastIndex(value, "=")
	if lastEquals < 0 {
		return token
	}
	return prefix + value[:lastEquals] + ":" + value[lastEquals+1:]
}
