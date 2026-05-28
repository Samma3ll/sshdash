package apiparsers

import (
	"net/http"
	"strings"
	"testing"
)

func TestGitHubStatusParser(t *testing.T) {
	body := []byte(`{"status":{"indicator":"minor","description":"Minor Service Outage"}}`)

	got := GitHubStatusParser{}.Parse(&http.Response{StatusCode: http.StatusOK}, body)

	if got.Status != StatusWarning {
		t.Fatalf("Status = %q, want %q", got.Status, StatusWarning)
	}
	if got.Summary != "Minor Service Outage" {
		t.Fatalf("Summary = %q", got.Summary)
	}
	if len(got.Details) != 1 || got.Details[0] != "indicator: minor" {
		t.Fatalf("Details = %#v", got.Details)
	}
}

func TestDefaultParserCompactsJSON(t *testing.T) {
	body := []byte("{\n  \"hello\": \"world\"\n}")

	got := DefaultParser{}.Parse(&http.Response{StatusCode: http.StatusOK}, body)

	if got.Summary != "HTTP 200" {
		t.Fatalf("Summary = %q", got.Summary)
	}
	if len(got.Details) != 1 || strings.Contains(got.Details[0], "\n") {
		t.Fatalf("Details = %#v", got.Details)
	}
}

func TestPublicIPParser(t *testing.T) {
	body := []byte(`{"ip":"83.87.197.125"}`)

	got := PublicIPParser{}.Parse(&http.Response{StatusCode: http.StatusOK}, body)

	if got.Summary != "IP: 83.87.197.125" {
		t.Fatalf("Summary = %q", got.Summary)
	}
	if len(got.Details) != 0 {
		t.Fatalf("Details = %#v", got.Details)
	}
}

func TestHomeAssistantStatusParserURL(t *testing.T) {
	got := HomeAssistantStatusParser{}.URL("http://homeassistant.local:8123")

	if got != "http://homeassistant.local:8123/api/" {
		t.Fatalf("URL = %q", got)
	}
}

func TestHomeAssistantStatusParser(t *testing.T) {
	body := []byte(`{"message":"API running."}`)

	got := HomeAssistantStatusParser{}.Parse(&http.Response{StatusCode: http.StatusOK}, body)

	if got.Status != StatusOK {
		t.Fatalf("Status = %q, want %q", got.Status, StatusOK)
	}
	if got.Summary != "API running." {
		t.Fatalf("Summary = %q", got.Summary)
	}
}

func TestHomeAssistantStatusParserUnauthorized(t *testing.T) {
	got := HomeAssistantStatusParser{}.Parse(&http.Response{StatusCode: http.StatusUnauthorized}, nil)

	if got.Status != StatusWarning {
		t.Fatalf("Status = %q, want %q", got.Status, StatusWarning)
	}
	if got.Summary == "" {
		t.Fatal("Summary is empty")
	}
}

func TestJellyfinStatusParserURL(t *testing.T) {
	got := JellyfinStatusParser{}.URL("http://jellyfin.local:8096")

	if got != "http://jellyfin.local:8096/health" {
		t.Fatalf("URL = %q", got)
	}
}

func TestJellyfinStatusParser(t *testing.T) {
	got := JellyfinStatusParser{}.Parse(&http.Response{StatusCode: http.StatusNoContent}, nil)

	if got.Status != StatusOK {
		t.Fatalf("Status = %q, want %q", got.Status, StatusOK)
	}
	if got.Summary != "Healthy" {
		t.Fatalf("Summary = %q", got.Summary)
	}
}
