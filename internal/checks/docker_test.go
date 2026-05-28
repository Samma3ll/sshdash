package checks

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

func TestDockerSummaryFlagsUnhealthy(t *testing.T) {
	containers := []dockerContainer{
		{Names: []string{"/app"}, State: "running", Status: "Up 1 hour (healthy)"},
		{Names: []string{"/db"}, State: "running", Status: "Up 1 hour (unhealthy)"},
	}

	status, summary, details := DockerSummary(containers)

	if status != StatusError {
		t.Fatalf("status = %q, want %q", status, StatusError)
	}
	if summary != "2 running, 0 stopped, 1 unhealthy, 0 restarting" {
		t.Fatalf("summary = %q", summary)
	}
	if len(details) != 2 {
		t.Fatalf("details = %#v", details)
	}
}

func TestDockerSummaryWarnsForStopped(t *testing.T) {
	containers := []dockerContainer{
		{Names: []string{"/app"}, State: "running", Status: "Up 1 hour"},
		{Names: []string{"/old"}, State: "exited", Status: "Exited (0) 2 days ago"},
	}

	status, _, _ := DockerSummary(containers)

	if status != StatusWarning {
		t.Fatalf("status = %q, want %q", status, StatusWarning)
	}
}

func TestDockerSummaryErrorsForDead(t *testing.T) {
	containers := []dockerContainer{
		{Names: []string{"/bad"}, State: "dead", Status: "Dead"},
	}

	status, _, _ := DockerSummary(containers)

	if status != StatusError {
		t.Fatalf("status = %q, want %q", status, StatusError)
	}
}

func TestDisplayErrorUnwrapsURLError(t *testing.T) {
	err := &url.Error{
		Op:  "Get",
		URL: "https://192.168.10.11:8006/api2/json/cluster/resources",
		Err: errors.New("tls: failed to verify certificate"),
	}

	got := displayError(err)

	if got != "tls: failed to verify certificate" {
		t.Fatalf("displayError = %q", got)
	}
}

func TestDockerDetailsByStack(t *testing.T) {
	containers := []dockerContainer{
		{Names: []string{"/standalone"}, Status: "Up 1 hour"},
		{Names: []string{"/app-db"}, Status: "Up 2 hours", Labels: map[string]string{"com.docker.compose.project": "app"}},
		{Names: []string{"/app-web"}, Status: "Up 2 hours", Labels: map[string]string{"com.docker.compose.project": "app"}},
		{Names: []string{"/media-jellyfin"}, Status: "Up 3 hours", Labels: map[string]string{"com.docker.compose.project": "media"}},
	}

	details := DockerDetailsByStack(containers)

	if len(details) != 3 {
		t.Fatalf("details = %#v", details)
	}
	if !strings.HasPrefix(details[0], "app\n") {
		t.Fatalf("first group = %q", details[0])
	}
	if !strings.Contains(details[0], "app-db: Up 2 hours") || !strings.Contains(details[0], "app-web: Up 2 hours") {
		t.Fatalf("app group = %q", details[0])
	}
	if details[2] != "standalone: Up 1 hour" {
		t.Fatalf("standalone = %q", details[2])
	}
}
