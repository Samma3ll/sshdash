package checks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"sshdash/internal/config"
)

type DockerChecker struct {
	Config config.DockerConfig
}

type dockerContainer struct {
	Names  []string          `json:"Names"`
	Image  string            `json:"Image"`
	State  string            `json:"State"`
	Status string            `json:"Status"`
	Labels map[string]string `json:"Labels"`
}

func (c DockerChecker) Name() string {
	return c.Config.Name
}

func (c DockerChecker) Check(ctx context.Context) Result {
	start := time.Now()
	checkURL := dockerContainersURL(c.Config.URL, c.Config.ShowStopped)
	reqCtx, cancel := context.WithTimeout(ctx, c.Config.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, checkURL, nil)
	if err != nil {
		return result(c.Config.Name, "docker", StatusError, checkURL, "invalid request", []string{err.Error()}, start)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return result(c.Config.Name, "docker", StatusError, checkURL, displayError(err), failureDetails(checkURL, err), start)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return result(c.Config.Name, "docker", StatusError, checkURL, "read failed", []string{err.Error()}, start)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := httpStatusError(resp, body)
		return result(c.Config.Name, "docker", statusFromHTTP(resp.StatusCode), checkURL, err.Error(), failureDetails(checkURL, err), start)
	}

	containers, err := ParseDockerContainers(body)
	if err != nil {
		return result(c.Config.Name, "docker", StatusError, checkURL, "parse failed", []string{err.Error()}, start)
	}

	status, summary, details := DockerSummary(containers)
	return result(c.Config.Name, "docker", status, checkURL, summary, details, start)
}

func dockerContainersURL(baseURL string, showStopped bool) string {
	checkURL := appendURLPath(baseURL, "/containers/json")
	if showStopped {
		parsed, err := url.Parse(checkURL)
		if err == nil {
			query := parsed.Query()
			query.Set("all", "true")
			parsed.RawQuery = query.Encode()
			return parsed.String()
		}
		return checkURL + "?all=true"
	}
	return checkURL
}

func ParseDockerContainers(body []byte) ([]dockerContainer, error) {
	var containers []dockerContainer
	if err := json.Unmarshal(body, &containers); err != nil {
		return nil, err
	}
	return containers, nil
}

func DockerSummary(containers []dockerContainer) (Status, string, []string) {
	var running, stopped, unhealthy, restarting, dead int

	for _, container := range containers {
		state := strings.ToLower(container.State)
		statusText := strings.ToLower(container.Status)

		switch state {
		case "running":
			running++
		case "restarting":
			restarting++
		case "dead":
			dead++
		default:
			stopped++
		}
		if strings.Contains(statusText, "unhealthy") {
			unhealthy++
		}
	}

	status := StatusOK
	if stopped > 0 {
		status = StatusWarning
	}
	if unhealthy > 0 || restarting > 0 || dead > 0 {
		status = StatusError
	}

	summary := fmt.Sprintf("%d running, %d stopped, %d unhealthy, %d restarting", running, stopped, unhealthy, restarting)
	if len(containers) == 0 {
		summary = "no containers returned"
		status = StatusWarning
	}
	return status, summary, DockerDetailsByStack(containers)
}

func DockerDetailsByStack(containers []dockerContainer) []string {
	grouped := map[string][]dockerContainer{}
	singles := []dockerContainer{}
	for _, container := range containers {
		stack := dockerStackName(container)
		if stack == "" {
			singles = append(singles, container)
			continue
		}
		grouped[stack] = append(grouped[stack], container)
	}

	stackNames := make([]string, 0, len(grouped))
	for stack := range grouped {
		stackNames = append(stackNames, stack)
	}
	sort.Strings(stackNames)
	sortDockerContainers(singles)

	details := []string{}
	for _, stack := range stackNames {
		containers := grouped[stack]
		sortDockerContainers(containers)
		lines := []string{stack}
		for _, container := range containers {
			lines = append(lines, dockerContainerLine(container))
		}
		details = append(details, strings.Join(lines, "\n"))
	}
	for _, container := range singles {
		details = append(details, dockerContainerLine(container))
	}
	if len(details) > 12 {
		return append(details[:12], fmt.Sprintf("+%d more", len(details)-12))
	}
	return details
}

func dockerName(names []string) string {
	if len(names) == 0 {
		return "unknown"
	}
	return strings.TrimPrefix(names[0], "/")
}

func dockerStackName(container dockerContainer) string {
	if container.Labels == nil {
		return ""
	}
	if project := container.Labels["com.docker.compose.project"]; project != "" {
		return project
	}
	if stack := container.Labels["com.docker.stack.namespace"]; stack != "" {
		return stack
	}
	return ""
}

func dockerContainerLine(container dockerContainer) string {
	return fmt.Sprintf("%s: %s", dockerName(container.Names), container.Status)
}

func sortDockerContainers(containers []dockerContainer) {
	sort.Slice(containers, func(i, j int) bool {
		return dockerName(containers[i].Names) < dockerName(containers[j].Names)
	})
}
