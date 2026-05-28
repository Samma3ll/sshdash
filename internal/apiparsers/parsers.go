package apiparsers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
)

type StatusHint string

const (
	StatusKeep    StatusHint = ""
	StatusOK      StatusHint = "ok"
	StatusWarning StatusHint = "warn"
	StatusError   StatusHint = "error"
)

type Result struct {
	Status  StatusHint
	Summary string
	Details []string
}

type Parser interface {
	Name() string
	URL(string) string
	Parse(*http.Response, []byte) Result
}

var registry = map[string]Parser{
	"default":              DefaultParser{},
	"github_status":        GitHubStatusParser{},
	"homeassistant-status": HomeAssistantStatusParser{},
	"homeassistant_status": HomeAssistantStatusParser{},
	"jellyfin-status":      JellyfinStatusParser{},
	"jellyfin_status":      JellyfinStatusParser{},
	"public_ip":            PublicIPParser{},
}

func Names() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func Exists(name string) bool {
	_, ok := registry[name]
	return ok
}

func Get(name string) Parser {
	if parser, ok := registry[name]; ok {
		return parser
	}
	return registry["default"]
}

type DefaultParser struct{}

func (DefaultParser) Name() string {
	return "default"
}

func (DefaultParser) URL(rawURL string) string {
	return rawURL
}

func (DefaultParser) Parse(resp *http.Response, body []byte) Result {
	summary := fmt.Sprintf("HTTP %d", resp.StatusCode)
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return Result{Summary: summary}
	}

	var value any
	if err := json.Unmarshal(body, &value); err == nil {
		compact, err := json.Marshal(value)
		if err == nil {
			return Result{
				Summary: summary,
				Details: []string{
					truncate(string(compact), 180),
				},
			}
		}
	}

	return Result{
		Summary: summary,
		Details: []string{
			truncate(singleLine(trimmed), 180),
		},
	}
}

type GitHubStatusParser struct{}

func (GitHubStatusParser) Name() string {
	return "github_status"
}

func (GitHubStatusParser) URL(rawURL string) string {
	return rawURL
}

func (GitHubStatusParser) Parse(resp *http.Response, body []byte) Result {
	var payload struct {
		Status struct {
			Indicator   string `json:"indicator"`
			Description string `json:"description"`
		} `json:"status"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return DefaultParser{}.Parse(resp, body)
	}

	description := strings.TrimSpace(payload.Status.Description)
	if description == "" {
		description = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}

	indicator := strings.ToLower(strings.TrimSpace(payload.Status.Indicator))
	parsed := Result{
		Summary: description,
	}
	if indicator != "" {
		parsed.Details = []string{"indicator: " + indicator}
	}

	switch indicator {
	case "none":
		parsed.Status = StatusOK
	case "minor", "major":
		parsed.Status = StatusWarning
	case "critical":
		parsed.Status = StatusError
	}

	return parsed
}

type PublicIPParser struct{}

func (PublicIPParser) Name() string {
	return "public_ip"
}

func (PublicIPParser) URL(rawURL string) string {
	return rawURL
}

func (PublicIPParser) Parse(resp *http.Response, body []byte) Result {
	var payload struct {
		IP string `json:"ip"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return DefaultParser{}.Parse(resp, body)
	}

	ip := strings.TrimSpace(payload.IP)
	if ip == "" {
		return DefaultParser{}.Parse(resp, body)
	}

	return Result{
		Summary: "IP: " + ip,
	}
}

type HomeAssistantStatusParser struct{}

func (HomeAssistantStatusParser) Name() string {
	return "homeassistant-status"
}

func (HomeAssistantStatusParser) URL(rawURL string) string {
	return appendPath(rawURL, "/api/")
}

func (HomeAssistantStatusParser) Parse(resp *http.Response, body []byte) Result {
	if resp.StatusCode == http.StatusUnauthorized {
		return Result{
			Status:  StatusWarning,
			Summary: "Unauthorized - add a Home Assistant bearer token",
		}
	}

	var payload struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return DefaultParser{}.Parse(resp, body)
	}

	message := strings.TrimSpace(payload.Message)
	if message == "" {
		return DefaultParser{}.Parse(resp, body)
	}

	parsed := Result{Summary: message}
	if strings.EqualFold(message, "API running.") {
		parsed.Status = StatusOK
	}
	return parsed
}

type JellyfinStatusParser struct{}

func (JellyfinStatusParser) Name() string {
	return "jellyfin_status"
}

func (JellyfinStatusParser) URL(rawURL string) string {
	return appendPath(rawURL, "/health")
}

func (JellyfinStatusParser) Parse(resp *http.Response, body []byte) Result {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return Result{
			Status:  StatusOK,
			Summary: "Healthy",
		}
	}
	return DefaultParser{}.Parse(resp, body)
}

func appendPath(rawURL, endpoint string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return strings.TrimRight(rawURL, "/") + endpoint
	}

	joined := path.Join(parsed.Path, endpoint)
	if strings.HasSuffix(endpoint, "/") {
		joined += "/"
	}
	if !strings.HasPrefix(joined, "/") {
		joined = "/" + joined
	}
	parsed.Path = joined
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func singleLine(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	return strings.Join(strings.Fields(value), " ")
}

func truncate(value string, maxLength int) string {
	if len(value) <= maxLength {
		return value
	}
	if maxLength <= 3 {
		return value[:maxLength]
	}
	return value[:maxLength-3] + "..."
}
