package checks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"sshdash/internal/config"
)

type WeatherChecker struct {
	Config config.WeatherConfig
}

type wttrResponse struct {
	CurrentCondition []struct {
		TempC        string `json:"temp_C"`
		FeelsLikeC   string `json:"FeelsLikeC"`
		Humidity     string `json:"humidity"`
		WindspeedKPH string `json:"windspeedKmph"`
		WeatherDesc  []struct {
			Value string `json:"value"`
		} `json:"weatherDesc"`
	} `json:"current_condition"`
	NearestArea []struct {
		AreaName []struct {
			Value string `json:"value"`
		} `json:"areaName"`
	} `json:"nearest_area"`
}

func (c WeatherChecker) Name() string {
	return c.Config.Name
}

func (c WeatherChecker) Check(ctx context.Context) Result {
	start := time.Now()
	checkURL := weatherURL(c.Config)
	reqCtx, cancel := context.WithTimeout(ctx, c.Config.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, checkURL, nil)
	if err != nil {
		return result(c.Config.Name, "weather", StatusError, checkURL, "invalid request", []string{err.Error()}, start)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return result(c.Config.Name, "weather", StatusError, checkURL, displayError(err), failureDetails(checkURL, err), start)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return result(c.Config.Name, "weather", StatusError, checkURL, "read failed", []string{err.Error()}, start)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := httpStatusError(resp, body)
		return result(c.Config.Name, "weather", statusFromHTTP(resp.StatusCode), checkURL, err.Error(), failureDetails(checkURL, err), start)
	}

	summary, details, err := ParseWeather(body)
	if err != nil {
		return result(c.Config.Name, "weather", StatusError, checkURL, "parse failed", []string{err.Error()}, start)
	}

	return result(c.Config.Name, "weather", StatusOK, checkURL, summary, details, start)
}

func weatherURL(cfg config.WeatherConfig) string {
	if cfg.URL != "" {
		return cfg.URL
	}
	return "https://wttr.in/" + url.PathEscape(cfg.Location) + "?format=j1"
}

func ParseWeather(body []byte) (string, []string, error) {
	var payload wttrResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", nil, err
	}
	if len(payload.CurrentCondition) == 0 {
		return "", nil, fmt.Errorf("missing current_condition")
	}

	current := payload.CurrentCondition[0]
	location := weatherLocation(payload)
	description := ""
	if len(current.WeatherDesc) > 0 {
		description = strings.TrimSpace(current.WeatherDesc[0].Value)
	}
	if description == "" {
		description = "weather"
	}

	summary := fmt.Sprintf("%s %s°C %s %s ", location, current.TempC, description, weatherIcon(description))
	details := []string{
		fmt.Sprintf("feels %s°C  humidity %s%%  wind %skm/h", current.FeelsLikeC, current.Humidity, current.WindspeedKPH),
	}
	return summary, details, nil
}

func weatherLocation(payload wttrResponse) string {
	if len(payload.NearestArea) == 0 || len(payload.NearestArea[0].AreaName) == 0 {
		return "Weather"
	}
	location := strings.TrimSpace(payload.NearestArea[0].AreaName[0].Value)
	if location == "" {
		return "Weather"
	}
	return location
}

func weatherIcon(description string) string {
	value := strings.ToLower(description)
	switch {
	case strings.Contains(value, "sunny") || strings.Contains(value, "clear"):
		return "☀"
	case strings.Contains(value, "thunder"):
		return "⚡"
	case strings.Contains(value, "snow") || strings.Contains(value, "sleet") || strings.Contains(value, "blizzard"):
		return "❄"
	case strings.Contains(value, "rain") || strings.Contains(value, "drizzle") || strings.Contains(value, "shower"):
		return "☔"
	case strings.Contains(value, "fog") || strings.Contains(value, "mist") || strings.Contains(value, "haze"):
		return "≋"
	case strings.Contains(value, "cloud") || strings.Contains(value, "overcast"):
		return "☁"
	case strings.Contains(value, "wind"):
		return "↝"
	default:
		return "•"
	}
}
