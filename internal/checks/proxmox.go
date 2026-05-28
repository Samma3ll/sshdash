package checks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"sshdash/internal/config"
)

type ProxmoxHealthChecker struct {
	Config config.ProxmoxConfig
}

type ProxmoxVMChecker struct {
	Config config.ProxmoxConfig
}

type proxmoxResourceResponse struct {
	Data []proxmoxResource `json:"data"`
}

type proxmoxResource struct {
	Type   string  `json:"type"`
	Node   string  `json:"node"`
	VMID   int     `json:"vmid"`
	Name   string  `json:"name"`
	Status string  `json:"status"`
	CPU    float64 `json:"cpu"`
	MaxCPU float64 `json:"maxcpu"`
	Mem    float64 `json:"mem"`
	MaxMem float64 `json:"maxmem"`
	Uptime float64 `json:"uptime"`
}

func (c ProxmoxHealthChecker) Name() string {
	return c.Config.Name + "-health"
}

func (c ProxmoxHealthChecker) Check(ctx context.Context) Result {
	start := time.Now()
	resources, checkURL, err := fetchProxmoxResources(ctx, c.Config)
	if err != nil {
		return result("proxmox", "proxmox-health", StatusError, checkURL, displayError(err), failureDetails(checkURL, err), start)
	}

	filtered := filterProxmoxResources(resources, c.Config.Nodes)
	status, summary, details := ProxmoxHealthSummary(filtered)
	return result("proxmox", "proxmox-health", status, checkURL, summary, details, start)
}

func (c ProxmoxVMChecker) Name() string {
	return c.Config.Name + "-vms"
}

func (c ProxmoxVMChecker) Check(ctx context.Context) Result {
	start := time.Now()
	resources, checkURL, err := fetchProxmoxResources(ctx, c.Config)
	if err != nil {
		return result("vms", "proxmox-vms", StatusError, checkURL, displayError(err), failureDetails(checkURL, err), start)
	}

	filtered := filterProxmoxResources(resources, c.Config.Nodes)
	status, summary, details := ProxmoxVMSummary(filtered)
	return result("vms", "proxmox-vms", status, checkURL, summary, details, start)
}

func fetchProxmoxResources(ctx context.Context, cfg config.ProxmoxConfig) ([]proxmoxResource, string, error) {
	checkURL := appendURLPath(cfg.URL, "/api2/json/cluster/resources")
	reqCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, checkURL, nil)
	if err != nil {
		return nil, checkURL, err
	}
	req.Header.Set("Authorization", cfg.Token)

	resp, err := httpClient(cfg.SkipTLSVerify).Do(req)
	if err != nil {
		return nil, checkURL, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, checkURL, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, checkURL, httpStatusError(resp, body)
	}

	resources, err := ParseProxmoxResources(body)
	return resources, checkURL, err
}

func ParseProxmoxResources(body []byte) ([]proxmoxResource, error) {
	var payload proxmoxResourceResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	return payload.Data, nil
}

func ProxmoxHealthSummary(resources []proxmoxResource) (Status, string, []string) {
	var online, offline int
	details := []string{}
	for _, resource := range resources {
		if resource.Type != "node" {
			continue
		}
		if strings.EqualFold(resource.Status, "online") {
			online++
		} else {
			offline++
		}
		details = append(details, fmt.Sprintf("%s: %s cpu %s mem %s", resource.Node, resource.Status, percent(resource.CPU), ratioPercent(resource.Mem, resource.MaxMem)))
	}

	status := StatusOK
	if offline > 0 {
		status = StatusError
	}
	if online == 0 {
		status = StatusError
	}
	return status, fmt.Sprintf("%d online, %d offline nodes", online, offline), details
}

func ProxmoxVMSummary(resources []proxmoxResource) (Status, string, []string) {
	var running, stopped int
	details := []string{}
	for _, resource := range resources {
		if resource.Type != "qemu" {
			continue
		}
		if strings.EqualFold(resource.Status, "running") {
			running++
		} else {
			stopped++
		}
		name := resource.Name
		if name == "" {
			name = fmt.Sprintf("%d", resource.VMID)
		}
		details = append(details, fmt.Sprintf(
			"%d %s @ %s: %s\ncpu %s  mem %s  uptime %s",
			resource.VMID,
			name,
			resource.Node,
			resource.Status,
			percent(resource.CPU),
			ratioPercent(resource.Mem, resource.MaxMem),
			durationHuman(resource.Uptime),
		))
	}
	sort.Strings(details)

	status := StatusOK
	if stopped > 0 {
		status = StatusWarning
	}
	if running == 0 && stopped == 0 {
		status = StatusWarning
	}
	return status, fmt.Sprintf("%d running, %d stopped VMs", running, stopped), limitDetails(details, 10)
}

func filterProxmoxResources(resources []proxmoxResource, nodes []string) []proxmoxResource {
	if len(nodes) == 0 {
		return resources
	}
	allowed := map[string]bool{}
	for _, node := range nodes {
		allowed[node] = true
	}
	filtered := make([]proxmoxResource, 0, len(resources))
	for _, resource := range resources {
		if allowed[resource.Node] {
			filtered = append(filtered, resource)
		}
	}
	return filtered
}

func percent(value float64) string {
	return fmt.Sprintf("%.0f%%", value*100)
}

func ratioPercent(value, maxValue float64) string {
	if maxValue <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", value/maxValue*100)
}

func durationHuman(seconds float64) string {
	duration := time.Duration(seconds) * time.Second
	if duration <= 0 {
		return "-"
	}
	days := int(duration.Hours()) / 24
	hours := int(duration.Hours()) % 24
	minutes := int(duration.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd%dh", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh%dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func limitDetails(details []string, limit int) []string {
	if len(details) <= limit {
		return details
	}
	return append(details[:limit], fmt.Sprintf("+%d more", len(details)-limit))
}
