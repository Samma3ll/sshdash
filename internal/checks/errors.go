package checks

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func httpStatusError(resp *http.Response, body []byte) error {
	message := strings.TrimSpace(string(body))
	if message != "" {
		message = " " + truncatePlain(singleLine(message), 160)
	}
	if location := resp.Header.Get("Location"); location != "" {
		message = " -> " + location + message
	}
	return fmt.Errorf("HTTP %d %s%s", resp.StatusCode, http.StatusText(resp.StatusCode), message)
}

func unexpectedContentTypeError(resp *http.Response, body []byte) error {
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "unknown content type"
	}
	preview := strings.TrimSpace(string(body))
	if preview != "" {
		preview = ": " + truncatePlain(singleLine(preview), 120)
	}
	return fmt.Errorf("HTTP %d returned %s instead of JSON%s", resp.StatusCode, contentType, preview)
}

func readLimitedErrorBody(resp *http.Response, limit int64) []byte {
	if resp.Body == nil {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, limit))
	return body
}

func singleLine(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	return strings.Join(strings.Fields(value), " ")
}

func truncatePlain(value string, maxLength int) string {
	if len(value) <= maxLength {
		return value
	}
	if maxLength <= 3 {
		return value[:maxLength]
	}
	return value[:maxLength-3] + "..."
}

func failureDetails(checkURL string, err error) []string {
	if err == nil {
		return []string{checkURL}
	}
	message := displayError(err)
	if message == checkURL || message == "GET "+checkURL {
		return []string{message}
	}
	return []string{message}
}

func displayError(err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		return urlErr.Err.Error()
	}
	return err.Error()
}
