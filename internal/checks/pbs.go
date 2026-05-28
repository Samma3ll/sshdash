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

type PBSHealthChecker struct {
	Config config.ProxmoxBackupConfig
}

type PBSDetailsChecker struct {
	Config config.ProxmoxBackupConfig
}

type pbsDatastoreStatus struct {
	Total     float64 `json:"total"`
	Used      float64 `json:"used"`
	Avail     float64 `json:"avail"`
	GCStatus  string  `json:"gc-status"`
	Store     string  `json:"store"`
	Datastore string  `json:"datastore"`
}

type pbsDatastoreResponse struct {
	Data pbsDatastoreStatus `json:"data"`
}

type pbsTasksResponse struct {
	Data []pbsTask `json:"data"`
}

type pbsTask struct {
	UPID       string  `json:"upid"`
	WorkerType string  `json:"worker_type"`
	WorkerID   string  `json:"worker_id"`
	User       string  `json:"user"`
	StartTime  float64 `json:"starttime"`
	EndTime    float64 `json:"endtime"`
	Status     string  `json:"status"`
}

func (c PBSHealthChecker) Name() string {
	return c.Config.Name + "-health"
}

func (c PBSHealthChecker) Check(ctx context.Context) Result {
	start := time.Now()
	tasks, checkURL, err := fetchPBSTasks(ctx, c.Config, time.Now().Add(-24*time.Hour))
	if err != nil {
		return result("pbs", "pbs-health", StatusError, checkURL, displayError(err), failureDetails(checkURL, err), start)
	}

	status, summary, details := PBSTasksSummary(tasks)
	return result("pbs", "pbs-health", status, checkURL, summary, details, start)
}

func (c PBSDetailsChecker) Name() string {
	return c.Config.Name + "-details"
}

func (c PBSDetailsChecker) Check(ctx context.Context) Result {
	start := time.Now()
	statuses, checkURL, err := fetchPBSDatastores(ctx, c.Config)
	if err != nil {
		return result("datastores", "pbs-details", StatusError, checkURL, displayError(err), failureDetails(checkURL, err), start)
	}

	status, _, details := PBSSummary(statuses)
	return result("datastores", "pbs-details", status, checkURL, fmt.Sprintf("%d datastores", len(statuses)), details, start)
}

func fetchPBSDatastores(ctx context.Context, cfg config.ProxmoxBackupConfig) ([]pbsDatastoreStatus, string, error) {
	statuses := make([]pbsDatastoreStatus, 0, len(cfg.Datastores))
	lastURL := ""
	for _, datastore := range cfg.Datastores {
		checkURL := appendURLPath(cfg.URL, "/api2/json/admin/datastore/"+datastore+"/status")
		lastURL = checkURL
		reqCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, checkURL, nil)
		if err != nil {
			cancel()
			return nil, checkURL, err
		}
		req.Header.Set("Authorization", cfg.Token)

		resp, err := httpClient(cfg.SkipTLSVerify).Do(req)
		if err != nil {
			cancel()
			return nil, checkURL, err
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
		resp.Body.Close()
		cancel()
		if readErr != nil {
			return nil, checkURL, readErr
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			if resp.StatusCode == http.StatusForbidden {
				return nil, checkURL, fmt.Errorf("%s: HTTP 403 Forbidden - grant token permission on /datastore/%s", datastore, datastore)
			}
			return nil, checkURL, fmt.Errorf("%w for datastore %s", httpStatusError(resp, body), datastore)
		}
		if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "json") {
			return nil, checkURL, fmt.Errorf("%w for datastore %s", unexpectedContentTypeError(resp, body), datastore)
		}

		parsed, err := ParsePBSDatastore(body)
		if err != nil {
			return nil, checkURL, err
		}
		if parsed.Store == "" && parsed.Datastore == "" {
			parsed.Store = datastore
		}
		statuses = append(statuses, parsed)
	}
	return statuses, lastURL, nil
}

func fetchPBSTasks(ctx context.Context, cfg config.ProxmoxBackupConfig, since time.Time) ([]pbsTask, string, error) {
	checkURL := appendURLPath(cfg.URL, "/api2/json/nodes/localhost/tasks")
	checkURL += fmt.Sprintf("?since=%d&limit=50", since.Unix())
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

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return nil, checkURL, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, checkURL, httpStatusError(resp, body)
	}
	if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "json") {
		return nil, checkURL, unexpectedContentTypeError(resp, body)
	}

	tasks, err := ParsePBSTasks(body)
	return tasks, checkURL, err
}

func ParsePBSDatastore(body []byte) (pbsDatastoreStatus, error) {
	var payload pbsDatastoreResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return pbsDatastoreStatus{}, err
	}
	return payload.Data, nil
}

func ParsePBSTasks(body []byte) ([]pbsTask, error) {
	var payload pbsTasksResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	return payload.Data, nil
}

func PBSTasksSummary(tasks []pbsTask) (Status, string, []string) {
	var okCount, runningCount, failedCount int
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].StartTime > tasks[j].StartTime
	})

	details := make([]string, 0, min(len(tasks), 8))
	for _, task := range tasks {
		taskStatus := strings.TrimSpace(task.Status)
		switch {
		case taskStatus == "":
			runningCount++
		case strings.EqualFold(taskStatus, "OK"):
			okCount++
		default:
			failedCount++
		}
		if len(details) < 8 {
			details = append(details, pbsTaskLine(task))
		}
	}

	status := StatusOK
	if runningCount > 0 {
		status = StatusWarning
	}
	if failedCount > 0 {
		status = StatusError
	}
	if len(tasks) == 0 {
		return StatusWarning, "no visible tasks in last 24h", []string{"grant Sys.Audit on /system/status or use a token that can read node tasks"}
	}
	return status, fmt.Sprintf("24h tasks: %d ok, %d running, %d failed", okCount, runningCount, failedCount), details
}

func pbsTaskLine(task pbsTask) string {
	status := strings.TrimSpace(task.Status)
	if status == "" {
		status = "running"
	}
	started := ""
	if task.StartTime > 0 {
		started = time.Unix(int64(task.StartTime), 0).Format("15:04")
	}

	return fmt.Sprintf(
		"%-10s %-22s %-10s %s",
		shortTaskType(task.WorkerType),
		truncatePlain(shortTaskTarget(task.WorkerID), 22),
		shortTaskStatus(status),
		started,
	)
}

func shortTaskType(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case "garbage_collection":
		return "gc"
	case "prunejob":
		return "prune"
	case "syncjob":
		return "sync"
	case "verifyjob":
		return "verify"
	case "aptupdate":
		return "apt"
	default:
		return truncatePlain(value, 10)
	}
}

func shortTaskTarget(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	parts := strings.Split(value, "/")
	if len(parts) >= 2 {
		last := parts[len(parts)-1]
		if len(last) > 8 {
			last = last[:8]
		}
		return parts[len(parts)-2] + "/" + last
	}
	return value
}

func shortTaskStatus(value string) string {
	value = strings.TrimSpace(value)
	switch {
	case value == "":
		return "running"
	case strings.EqualFold(value, "OK"):
		return "OK"
	case strings.Contains(strings.ToLower(value), "unknown"):
		return "unknown"
	case strings.Contains(strings.ToLower(value), "error"):
		return "failed"
	default:
		return truncatePlain(value, 10)
	}
}

func PBSSummary(statuses []pbsDatastoreStatus) (Status, string, []string) {
	status := StatusOK
	details := make([]string, 0, len(statuses))
	var highest float64
	for _, datastore := range statuses {
		usedPercent := datastoreUsedPercent(datastore)
		if usedPercent > highest {
			highest = usedPercent
		}
		if usedPercent >= 90 {
			status = StatusError
		} else if usedPercent >= 80 && status != StatusError {
			status = StatusWarning
		}

		name := datastore.Store
		if name == "" {
			name = datastore.Datastore
		}
		details = append(details, fmt.Sprintf("%s: %.0f%% used (%s / %s)", name, usedPercent, bytesHuman(datastore.Used), bytesHuman(datastore.Total)))
		if strings.TrimSpace(datastore.GCStatus) != "" {
			details = append(details, name+" GC: "+datastore.GCStatus)
		}
	}
	if len(statuses) == 0 {
		return StatusWarning, "no datastores returned", nil
	}
	return status, fmt.Sprintf("%d datastores, highest %.0f%% used", len(statuses), highest), details
}

func datastoreUsedPercent(datastore pbsDatastoreStatus) float64 {
	if datastore.Total <= 0 {
		return 0
	}
	return datastore.Used / datastore.Total * 100
}

func bytesHuman(value float64) string {
	units := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%.0f%s", value, units[unit])
	}
	return fmt.Sprintf("%.1f%s", value, units[unit])
}
