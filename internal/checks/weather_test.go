package checks

import (
	"strings"
	"testing"
)

func TestParseWeather(t *testing.T) {
	body := []byte(`{
		"current_condition": [{
			"temp_C": "12",
			"FeelsLikeC": "10",
			"humidity": "80",
			"windspeedKmph": "14",
			"weatherDesc": [{"value": "Partly cloudy"}]
		}],
		"nearest_area": [{
			"areaName": [{"value": "Amsterdam"}]
		}]
	}`)

	summary, details, err := ParseWeather(body)
	if err != nil {
		t.Fatal(err)
	}
	if summary != "Amsterdam 12°C Partly cloudy ☁ " {
		t.Fatalf("summary = %q", summary)
	}
	if len(details) != 1 || !strings.Contains(details[0], "feels 10°C") {
		t.Fatalf("details = %#v", details)
	}
}

func TestWeatherIcon(t *testing.T) {
	tests := map[string]string{
		"Sunny":         "☀",
		"Light rain":    "☔",
		"Heavy snow":    "❄",
		"Thunderstorm":  "⚡",
		"Partly cloudy": "☁",
	}

	for input, want := range tests {
		if got := weatherIcon(input); got != want {
			t.Fatalf("weatherIcon(%q) = %q, want %q", input, got, want)
		}
	}
}
