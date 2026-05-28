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
	Media         MediaConfig         `yaml:"media"`
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

type MediaConfig struct {
	Enabled    bool               `yaml:"enabled"`
	Name       string             `yaml:"name"`
	Jellyfin   MediaServiceConfig `yaml:"jellyfin"`
	Radarr     MediaServiceConfig `yaml:"radarr"`
	Sonarr     MediaServiceConfig `yaml:"sonarr"`
	Jellyseerr MediaServiceConfig `yaml:"jellyseerr"`
}

type MediaServiceConfig struct {
	Enabled bool          `yaml:"enabled"`
	Name    string        `yaml:"name"`
	URL     string        `yaml:"url"`
	APIKey  string        `yaml:"api_key"`
	Timeout time.Duration `yaml:"timeout"`
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

func Save(path string, cfg Config) error {
	next := cloneConfig(cfg)
	applyDefaults(&next)
	if err := validate(next); err != nil {
		return err
	}

	data, err := yaml.Marshal(toYAMLConfig(next))
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func cloneConfig(cfg Config) Config {
	next := cfg
	next.Services = append([]ServiceConfig(nil), cfg.Services...)
	next.APIs = make([]APIConfig, 0, len(cfg.APIs))
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
	if cfg.Media.Enabled {
		if cfg.Media.Name == "" {
			cfg.Media.Name = "media"
		}
		applyMediaServiceDefaults(&cfg.Media.Jellyfin, "jellyfin")
		applyMediaServiceDefaults(&cfg.Media.Radarr, "radarr")
		applyMediaServiceDefaults(&cfg.Media.Sonarr, "sonarr")
		applyMediaServiceDefaults(&cfg.Media.Jellyseerr, "jellyseerr")
	}
}

type yamlDuration time.Duration

func (d yamlDuration) MarshalYAML() (any, error) {
	return time.Duration(d).String(), nil
}

type yamlConfig struct {
	Server        ServerConfig            `yaml:"server"`
	Refresh       yamlDuration            `yaml:"refresh"`
	Services      []yamlServiceConfig     `yaml:"services"`
	APIs          []yamlAPIConfig         `yaml:"apis"`
	Docker        yamlDockerConfig        `yaml:"docker"`
	Proxmox       yamlProxmoxConfig       `yaml:"proxmox"`
	ProxmoxBackup yamlProxmoxBackupConfig `yaml:"proxmox_backup"`
	Weather       yamlWeatherConfig       `yaml:"weather"`
	Media         yamlMediaConfig         `yaml:"media"`
}

type yamlServiceConfig struct {
	Name    string       `yaml:"name"`
	Address string       `yaml:"address"`
	Timeout yamlDuration `yaml:"timeout"`
}

type yamlAPIConfig struct {
	Name    string            `yaml:"name"`
	URL     string            `yaml:"url"`
	Parser  string            `yaml:"parser"`
	Headers map[string]string `yaml:"headers,omitempty"`
	Timeout yamlDuration      `yaml:"timeout"`
}

type yamlDockerConfig struct {
	Enabled     bool         `yaml:"enabled"`
	Name        string       `yaml:"name"`
	URL         string       `yaml:"url"`
	Timeout     yamlDuration `yaml:"timeout"`
	ShowStopped bool         `yaml:"show_stopped"`
}

type yamlProxmoxConfig struct {
	Enabled       bool         `yaml:"enabled"`
	Name          string       `yaml:"name"`
	URL           string       `yaml:"url"`
	Token         string       `yaml:"token"`
	Timeout       yamlDuration `yaml:"timeout"`
	Mode          string       `yaml:"mode"`
	Nodes         []string     `yaml:"nodes"`
	SkipTLSVerify bool         `yaml:"skip_tls_verify"`
}

type yamlProxmoxBackupConfig struct {
	Enabled       bool         `yaml:"enabled"`
	Name          string       `yaml:"name"`
	URL           string       `yaml:"url"`
	Token         string       `yaml:"token"`
	Datastores    []string     `yaml:"datastores"`
	Timeout       yamlDuration `yaml:"timeout"`
	SkipTLSVerify bool         `yaml:"skip_tls_verify"`
}

type yamlWeatherConfig struct {
	Enabled  bool         `yaml:"enabled"`
	Name     string       `yaml:"name"`
	Location string       `yaml:"location"`
	URL      string       `yaml:"url"`
	Timeout  yamlDuration `yaml:"timeout"`
}

type yamlMediaConfig struct {
	Enabled    bool                   `yaml:"enabled"`
	Name       string                 `yaml:"name"`
	Jellyfin   yamlMediaServiceConfig `yaml:"jellyfin"`
	Radarr     yamlMediaServiceConfig `yaml:"radarr"`
	Sonarr     yamlMediaServiceConfig `yaml:"sonarr"`
	Jellyseerr yamlMediaServiceConfig `yaml:"jellyseerr"`
}

type yamlMediaServiceConfig struct {
	Enabled bool         `yaml:"enabled"`
	Name    string       `yaml:"name"`
	URL     string       `yaml:"url"`
	APIKey  string       `yaml:"api_key"`
	Timeout yamlDuration `yaml:"timeout"`
}

func toYAMLConfig(cfg Config) yamlConfig {
	services := make([]yamlServiceConfig, 0, len(cfg.Services))
	for _, service := range cfg.Services {
		services = append(services, yamlServiceConfig{
			Name:    service.Name,
			Address: service.Address,
			Timeout: yamlDuration(service.Timeout),
		})
	}

	apis := make([]yamlAPIConfig, 0, len(cfg.APIs))
	for _, api := range cfg.APIs {
		apis = append(apis, yamlAPIConfig{
			Name:    api.Name,
			URL:     api.URL,
			Parser:  api.Parser,
			Headers: api.Headers,
			Timeout: yamlDuration(api.Timeout),
		})
	}

	return yamlConfig{
		Server:   cfg.Server,
		Refresh:  yamlDuration(cfg.Refresh),
		Services: services,
		APIs:     apis,
		Docker: yamlDockerConfig{
			Enabled:     cfg.Docker.Enabled,
			Name:        cfg.Docker.Name,
			URL:         cfg.Docker.URL,
			Timeout:     yamlDuration(cfg.Docker.Timeout),
			ShowStopped: cfg.Docker.ShowStopped,
		},
		Proxmox: yamlProxmoxConfig{
			Enabled:       cfg.Proxmox.Enabled,
			Name:          cfg.Proxmox.Name,
			URL:           cfg.Proxmox.URL,
			Token:         cfg.Proxmox.Token,
			Timeout:       yamlDuration(cfg.Proxmox.Timeout),
			Mode:          cfg.Proxmox.Mode,
			Nodes:         cfg.Proxmox.Nodes,
			SkipTLSVerify: cfg.Proxmox.SkipTLSVerify,
		},
		ProxmoxBackup: yamlProxmoxBackupConfig{
			Enabled:       cfg.ProxmoxBackup.Enabled,
			Name:          cfg.ProxmoxBackup.Name,
			URL:           cfg.ProxmoxBackup.URL,
			Token:         cfg.ProxmoxBackup.Token,
			Datastores:    cfg.ProxmoxBackup.Datastores,
			Timeout:       yamlDuration(cfg.ProxmoxBackup.Timeout),
			SkipTLSVerify: cfg.ProxmoxBackup.SkipTLSVerify,
		},
		Weather: yamlWeatherConfig{
			Enabled:  cfg.Weather.Enabled,
			Name:     cfg.Weather.Name,
			Location: cfg.Weather.Location,
			URL:      cfg.Weather.URL,
			Timeout:  yamlDuration(cfg.Weather.Timeout),
		},
		Media: yamlMediaConfig{
			Enabled:    cfg.Media.Enabled,
			Name:       cfg.Media.Name,
			Jellyfin:   toYAMLMediaService(cfg.Media.Jellyfin),
			Radarr:     toYAMLMediaService(cfg.Media.Radarr),
			Sonarr:     toYAMLMediaService(cfg.Media.Sonarr),
			Jellyseerr: toYAMLMediaService(cfg.Media.Jellyseerr),
		},
	}
}

func toYAMLMediaService(service MediaServiceConfig) yamlMediaServiceConfig {
	return yamlMediaServiceConfig{
		Enabled: service.Enabled,
		Name:    service.Name,
		URL:     service.URL,
		APIKey:  service.APIKey,
		Timeout: yamlDuration(service.Timeout),
	}
}

func applyMediaServiceDefaults(service *MediaServiceConfig, name string) {
	if !service.Enabled {
		return
	}
	if service.Name == "" {
		service.Name = name
	}
	if service.Timeout == 0 {
		service.Timeout = 5 * time.Second
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
	if cfg.Media.Enabled {
		if !cfg.Media.Jellyfin.Enabled && !cfg.Media.Radarr.Enabled && !cfg.Media.Sonarr.Enabled && !cfg.Media.Jellyseerr.Enabled {
			return errors.New("media requires at least one enabled service")
		}
		if err := validateMediaService("media.jellyfin", cfg.Media.Jellyfin, true); err != nil {
			return err
		}
		if err := validateMediaService("media.radarr", cfg.Media.Radarr, true); err != nil {
			return err
		}
		if err := validateMediaService("media.sonarr", cfg.Media.Sonarr, true); err != nil {
			return err
		}
		if err := validateMediaService("media.jellyseerr", cfg.Media.Jellyseerr, false); err != nil {
			return err
		}
	}
	return nil
}

func validateMediaService(prefix string, service MediaServiceConfig, requireAPIKey bool) error {
	if !service.Enabled {
		return nil
	}
	if service.URL == "" {
		return errors.New(prefix + ".url is required when enabled")
	}
	if requireAPIKey && service.APIKey == "" {
		return errors.New(prefix + ".api_key is required when enabled")
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
