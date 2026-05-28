package checks

import (
	"net/url"
	"path"
	"strings"
)

func appendURLPath(rawURL, endpoint string) string {
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
