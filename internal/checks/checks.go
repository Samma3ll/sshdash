package checks

import (
	"context"
	"io"
	"net"
	"net/http"
	"time"

	"sshdash/internal/apiparsers"
	"sshdash/internal/config"
)

type Status string

const (
	StatusOK      Status = "ok"
	StatusWarning Status = "warn"
	StatusError   Status = "error"
)

type Result struct {
	Name      string
	Kind      string
	Status    Status
	Target    string
	Summary   string
	Details   []string
	CheckedAt time.Time
	Latency   time.Duration
}

type Checker interface {
	Name() string
	Check(context.Context) Result
}

func FromConfig(cfg config.Config) []Checker {
	checkers := make([]Checker, 0, len(cfg.Services)+len(cfg.APIs)+7)
	for _, service := range cfg.Services {
		checkers = append(checkers, ServiceChecker{Config: service})
	}
	for _, api := range cfg.APIs {
		checkers = append(checkers, APIChecker{Config: api})
	}
	if cfg.Docker.Enabled {
		checkers = append(checkers, DockerChecker{Config: cfg.Docker})
	}
	if cfg.Proxmox.Enabled {
		checkers = append(checkers, ProxmoxHealthChecker{Config: cfg.Proxmox})
		checkers = append(checkers, ProxmoxVMChecker{Config: cfg.Proxmox})
	}
	if cfg.ProxmoxBackup.Enabled {
		checkers = append(checkers, PBSHealthChecker{Config: cfg.ProxmoxBackup})
		checkers = append(checkers, PBSDetailsChecker{Config: cfg.ProxmoxBackup})
	}
	if cfg.Weather.Enabled {
		checkers = append(checkers, WeatherChecker{Config: cfg.Weather})
	}
	return checkers
}

type ServiceChecker struct {
	Config config.ServiceConfig
}

func (c ServiceChecker) Name() string {
	return c.Config.Name
}

func (c ServiceChecker) Check(ctx context.Context) Result {
	start := time.Now()
	dialer := net.Dialer{Timeout: c.Config.Timeout}
	conn, err := dialer.DialContext(ctx, "tcp", c.Config.Address)
	if err != nil {
		return result(c.Config.Name, "service", StatusError, c.Config.Address, "unreachable", []string{err.Error()}, start)
	}
	defer conn.Close()

	return result(c.Config.Name, "service", StatusOK, c.Config.Address, "reachable", nil, start)
}

type APIChecker struct {
	Config config.APIConfig
}

func (c APIChecker) Name() string {
	return c.Config.Name
}

func (c APIChecker) Check(ctx context.Context) Result {
	start := time.Now()
	reqCtx, cancel := context.WithTimeout(ctx, c.Config.Timeout)
	defer cancel()

	parser := apiparsers.Get(c.Config.Parser)
	checkURL := parser.URL(c.Config.URL)

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, checkURL, nil)
	if err != nil {
		return result(c.Config.Name, "api", StatusError, checkURL, "invalid request", []string{err.Error()}, start)
	}
	for key, value := range c.Config.Headers {
		req.Header.Set(key, value)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return result(c.Config.Name, "api", StatusError, checkURL, displayError(err), failureDetails(checkURL, err), start)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := httpStatusError(resp, body)
		return result(c.Config.Name, "api", statusFromHTTP(resp.StatusCode), checkURL, err.Error(), failureDetails(checkURL, err), start)
	}

	parsed := parser.Parse(resp, body)
	status := statusFromHTTP(resp.StatusCode)
	if parsed.Status != apiparsers.StatusKeep {
		status = statusFromHint(parsed.Status)
	}

	return result(c.Config.Name, "api", status, checkURL, parsed.Summary, parsed.Details, start)
}

func result(name, kind string, status Status, target string, summary string, details []string, start time.Time) Result {
	return Result{
		Name:      name,
		Kind:      kind,
		Status:    status,
		Target:    target,
		Summary:   summary,
		Details:   details,
		CheckedAt: time.Now(),
		Latency:   time.Since(start),
	}
}

func statusFromHTTP(code int) Status {
	if code >= 200 && code < 300 {
		return StatusOK
	}
	if code >= 500 {
		return StatusError
	}
	return StatusWarning
}

func statusFromHint(hint apiparsers.StatusHint) Status {
	switch hint {
	case apiparsers.StatusOK:
		return StatusOK
	case apiparsers.StatusWarning:
		return StatusWarning
	case apiparsers.StatusError:
		return StatusError
	default:
		return StatusWarning
	}
}
