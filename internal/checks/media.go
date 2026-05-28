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

type JellyfinMediaChecker struct {
	Config config.MediaServiceConfig
}

type RadarrMediaChecker struct {
	Config config.MediaServiceConfig
}

type SonarrMediaChecker struct {
	Config config.MediaServiceConfig
}

type JellyseerrMediaChecker struct {
	Config config.MediaServiceConfig
}

type jellyfinCounts struct {
	MovieCount      int `json:"MovieCount"`
	SeriesCount     int `json:"SeriesCount"`
	EpisodeCount    int `json:"EpisodeCount"`
	ArtistCount     int `json:"ArtistCount"`
	ProgramCount    int `json:"ProgramCount"`
	TrailerCount    int `json:"TrailerCount"`
	SongCount       int `json:"SongCount"`
	AlbumCount      int `json:"AlbumCount"`
	MusicVideoCount int `json:"MusicVideoCount"`
	BoxSetCount     int `json:"BoxSetCount"`
	BookCount       int `json:"BookCount"`
	ItemCount       int `json:"ItemCount"`
}

type jellyfinItemsResponse struct {
	Items []jellyfinItem `json:"Items"`
}

type jellyfinItem struct {
	Name              string `json:"Name"`
	Type              string `json:"Type"`
	SeriesName        string `json:"SeriesName"`
	ParentIndexNumber int    `json:"ParentIndexNumber"`
	IndexNumber       int    `json:"IndexNumber"`
}

type mediaSystemStatus struct {
	Version string `json:"version"`
}

type radarrMovie struct {
	Title     string `json:"title"`
	Monitored bool   `json:"monitored"`
	HasFile   bool   `json:"hasFile"`
	Status    string `json:"status"`
}

type sonarrSeries struct {
	Title      string `json:"title"`
	Monitored  bool   `json:"monitored"`
	Statistics struct {
		EpisodeFileCount  int `json:"episodeFileCount"`
		EpisodeCount      int `json:"episodeCount"`
		TotalEpisodeCount int `json:"totalEpisodeCount"`
	} `json:"statistics"`
}

type jellyseerrStatus struct {
	Version string `json:"version"`
}

type jellyseerrRequestCount struct {
	Total      int `json:"total"`
	Pending    int `json:"pending"`
	Approved   int `json:"approved"`
	Declined   int `json:"declined"`
	Processing int `json:"processing"`
	Available  int `json:"available"`
}

func (c JellyfinMediaChecker) Name() string {
	return c.Config.Name
}

func (c JellyfinMediaChecker) Check(ctx context.Context) Result {
	start := time.Now()
	checkURL := appendURLPath(c.Config.URL, "/Items/Counts")
	body, err := fetchMediaJSON(ctx, c.Config, checkURL, "X-Emby-Token")
	if err != nil {
		return result(c.Config.Name, "media-jellyfin", StatusError, checkURL, displayError(err), failureDetails(checkURL, err), start)
	}

	counts, err := ParseJellyfinCounts(body)
	if err != nil {
		return result(c.Config.Name, "media-jellyfin", StatusError, checkURL, "parse failed", []string{err.Error()}, start)
	}
	var latestMovie, latestEpisode *jellyfinItem
	latestErrors := []string{}
	if item, err := fetchJellyfinLatest(ctx, c.Config, "Movie"); err == nil {
		latestMovie = item
	} else {
		latestErrors = append(latestErrors, "latest movie unavailable: "+displayError(err))
	}
	if item, err := fetchJellyfinLatest(ctx, c.Config, "Episode"); err == nil {
		latestEpisode = item
	} else {
		latestErrors = append(latestErrors, "latest episode unavailable: "+displayError(err))
	}
	version, versionErr := fetchMediaVersion(ctx, c.Config, appendURLPath(c.Config.URL, "/System/Info/Public"), "X-Emby-Token")

	status, summary, details := JellyfinCountsSummary(counts, latestMovie, latestEpisode, version)
	if len(latestErrors) > 0 {
		if status == StatusOK {
			status = StatusWarning
		}
		details = append(details, latestErrors...)
	}
	if versionErr != nil {
		if status == StatusOK {
			status = StatusWarning
		}
		details = append(details, "version unavailable: "+displayError(versionErr))
	}
	return result(c.Config.Name, "media-jellyfin", status, checkURL, summary, details, start)
}

func (c RadarrMediaChecker) Name() string {
	return c.Config.Name
}

func (c RadarrMediaChecker) Check(ctx context.Context) Result {
	start := time.Now()
	checkURL := appendURLPath(c.Config.URL, "/api/v3/movie")
	body, err := fetchMediaJSON(ctx, c.Config, checkURL, "X-Api-Key")
	if err != nil {
		return result(c.Config.Name, "media-radarr", StatusError, checkURL, displayError(err), failureDetails(checkURL, err), start)
	}

	movies, err := ParseRadarrMovies(body)
	if err != nil {
		return result(c.Config.Name, "media-radarr", StatusError, checkURL, "parse failed", []string{err.Error()}, start)
	}
	version, versionErr := fetchMediaVersion(ctx, c.Config, appendURLPath(c.Config.URL, "/api/v3/system/status"), "X-Api-Key")
	status, summary, details := RadarrSummary(movies, version)
	if versionErr != nil {
		if status == StatusOK {
			status = StatusWarning
		}
		details = append(details, "version unavailable: "+displayError(versionErr))
	}
	return result(c.Config.Name, "media-radarr", status, checkURL, summary, details, start)
}

func (c SonarrMediaChecker) Name() string {
	return c.Config.Name
}

func (c SonarrMediaChecker) Check(ctx context.Context) Result {
	start := time.Now()
	checkURL := appendURLPath(c.Config.URL, "/api/v3/series")
	body, err := fetchMediaJSON(ctx, c.Config, checkURL, "X-Api-Key")
	if err != nil {
		return result(c.Config.Name, "media-sonarr", StatusError, checkURL, displayError(err), failureDetails(checkURL, err), start)
	}

	series, err := ParseSonarrSeries(body)
	if err != nil {
		return result(c.Config.Name, "media-sonarr", StatusError, checkURL, "parse failed", []string{err.Error()}, start)
	}
	version, versionErr := fetchMediaVersion(ctx, c.Config, appendURLPath(c.Config.URL, "/api/v3/system/status"), "X-Api-Key")
	status, summary, details := SonarrSummary(series, version)
	if versionErr != nil {
		if status == StatusOK {
			status = StatusWarning
		}
		details = append(details, "version unavailable: "+displayError(versionErr))
	}
	return result(c.Config.Name, "media-sonarr", status, checkURL, summary, details, start)
}

func (c JellyseerrMediaChecker) Name() string {
	return c.Config.Name
}

func (c JellyseerrMediaChecker) Check(ctx context.Context) Result {
	start := time.Now()
	statusURL := appendURLPath(c.Config.URL, "/api/v1/status")
	statusBody, err := fetchMediaJSON(ctx, c.Config, statusURL, "X-Api-Key")
	if err != nil {
		return result(c.Config.Name, "media-jellyseerr", StatusError, statusURL, displayError(err), failureDetails(statusURL, err), start)
	}

	statusPayload, err := ParseJellyseerrStatus(statusBody)
	if err != nil {
		return result(c.Config.Name, "media-jellyseerr", StatusError, statusURL, "parse failed", []string{err.Error()}, start)
	}

	countURL := appendURLPath(c.Config.URL, "/api/v1/request/count")
	countBody, err := fetchMediaJSON(ctx, c.Config, countURL, "X-Api-Key")
	if err != nil {
		summary := "online"
		if statusPayload.Version != "" {
			summary = "online v" + statusPayload.Version
		}
		return result(c.Config.Name, "media-jellyseerr", StatusWarning, statusURL, summary, []string{displayError(err)}, start)
	}

	counts, err := ParseJellyseerrRequestCount(countBody)
	if err != nil {
		return result(c.Config.Name, "media-jellyseerr", StatusError, countURL, "parse failed", []string{err.Error()}, start)
	}
	status, summary, details := JellyseerrSummary(statusPayload, counts)
	return result(c.Config.Name, "media-jellyseerr", status, countURL, summary, details, start)
}

func fetchMediaJSON(ctx context.Context, cfg config.MediaServiceConfig, checkURL string, apiKeyHeader string) ([]byte, error) {
	reqCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, checkURL, nil)
	if err != nil {
		return nil, err
	}
	if cfg.APIKey != "" {
		req.Header.Set(apiKeyHeader, cfg.APIKey)
	}

	resp, err := httpClient(false).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, httpStatusError(resp, body)
	}
	if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "json") {
		return nil, unexpectedContentTypeError(resp, body)
	}
	return body, nil
}

func fetchJellyfinLatest(ctx context.Context, cfg config.MediaServiceConfig, itemType string) (*jellyfinItem, error) {
	checkURL := jellyfinLatestURL(cfg.URL, itemType)
	body, err := fetchMediaJSON(ctx, cfg, checkURL, "X-Emby-Token")
	if err != nil {
		return nil, err
	}
	items, err := ParseJellyfinItems(body)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("no %s items returned", strings.ToLower(itemType))
	}
	return &items[0], nil
}

func fetchMediaVersion(ctx context.Context, cfg config.MediaServiceConfig, checkURL string, apiKeyHeader string) (string, error) {
	body, err := fetchMediaJSON(ctx, cfg, checkURL, apiKeyHeader)
	if err != nil {
		return "", err
	}
	var status mediaSystemStatus
	if err := json.Unmarshal(body, &status); err != nil {
		return "", err
	}
	return status.Version, nil
}

func jellyfinLatestURL(baseURL, itemType string) string {
	checkURL := appendURLPath(baseURL, "/Items")
	parsed, err := url.Parse(checkURL)
	if err != nil {
		return checkURL
	}
	query := parsed.Query()
	query.Set("Recursive", "true")
	query.Set("IncludeItemTypes", itemType)
	query.Set("SortBy", "DateCreated")
	query.Set("SortOrder", "Descending")
	query.Set("Limit", "1")
	query.Set("Fields", "SeriesName,ParentIndexNumber,IndexNumber,DateCreated")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func ParseJellyfinCounts(body []byte) (jellyfinCounts, error) {
	var counts jellyfinCounts
	if err := json.Unmarshal(body, &counts); err != nil {
		return jellyfinCounts{}, err
	}
	return counts, nil
}

func ParseJellyfinItems(body []byte) ([]jellyfinItem, error) {
	var response jellyfinItemsResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	return response.Items, nil
}

func ParseRadarrMovies(body []byte) ([]radarrMovie, error) {
	var movies []radarrMovie
	if err := json.Unmarshal(body, &movies); err != nil {
		return nil, err
	}
	return movies, nil
}

func ParseSonarrSeries(body []byte) ([]sonarrSeries, error) {
	var series []sonarrSeries
	if err := json.Unmarshal(body, &series); err != nil {
		return nil, err
	}
	return series, nil
}

func ParseJellyseerrStatus(body []byte) (jellyseerrStatus, error) {
	var status jellyseerrStatus
	if err := json.Unmarshal(body, &status); err != nil {
		return jellyseerrStatus{}, err
	}
	return status, nil
}

func ParseJellyseerrRequestCount(body []byte) (jellyseerrRequestCount, error) {
	var counts jellyseerrRequestCount
	if err := json.Unmarshal(body, &counts); err != nil {
		return jellyseerrRequestCount{}, err
	}
	return counts, nil
}

func JellyfinCountsSummary(counts jellyfinCounts, latestMovie, latestEpisode *jellyfinItem, version string) (Status, string, []string) {
	total := counts.MovieCount + counts.SeriesCount + counts.EpisodeCount
	details := []string{
		fmt.Sprintf("movies %d  series %d  episodes %d", counts.MovieCount, counts.SeriesCount, counts.EpisodeCount),
	}
	if latestMovie != nil {
		details = append(details, "last movie: "+formatJellyfinMovie(*latestMovie))
	}
	if latestEpisode != nil {
		details = append(details, "last episode: "+formatJellyfinEpisode(*latestEpisode))
	}
	if version != "" {
		details = append(details, "version "+version)
	}
	if total == 0 {
		return StatusWarning, "no media items returned", details
	}
	return StatusOK, fmt.Sprintf("%d video items", total), details
}

func formatJellyfinMovie(item jellyfinItem) string {
	if item.Name == "" {
		return "unknown movie"
	}
	return item.Name
}

func formatJellyfinEpisode(item jellyfinItem) string {
	title := item.Name
	if title == "" {
		title = "unknown episode"
	}

	episodeCode := ""
	if item.ParentIndexNumber > 0 && item.IndexNumber > 0 {
		episodeCode = fmt.Sprintf(" S%02dE%02d", item.ParentIndexNumber, item.IndexNumber)
	} else if item.IndexNumber > 0 {
		episodeCode = fmt.Sprintf(" E%02d", item.IndexNumber)
	}

	if item.SeriesName != "" {
		return strings.TrimSpace(item.SeriesName + episodeCode + " - " + title)
	}
	return strings.TrimSpace(episodeCode + " " + title)
}

func RadarrSummary(movies []radarrMovie, version string) (Status, string, []string) {
	var downloaded, monitored, missing int
	missingTitles := []string{}
	for _, movie := range movies {
		if movie.HasFile {
			downloaded++
		}
		if movie.Monitored {
			monitored++
			if !movie.HasFile {
				missing++
				if len(missingTitles) < 5 {
					missingTitles = append(missingTitles, movie.Title)
				}
			}
		}
	}
	details := []string{
		fmt.Sprintf("downloaded %d/%d", downloaded, len(movies)),
		fmt.Sprintf("monitored %d  missing monitored %d", monitored, missing),
	}
	if len(missingTitles) > 0 {
		details = append(details, "missing: "+strings.Join(missingTitles, ", "))
	}
	if version != "" {
		details = append(details, "version "+version)
	}
	status := StatusOK
	if missing > 0 {
		status = StatusWarning
	}
	return status, fmt.Sprintf("%d movies, %d missing", len(movies), missing), details
}

func SonarrSummary(series []sonarrSeries, version string) (Status, string, []string) {
	var monitored, episodeFiles, episodes int
	for _, item := range series {
		if item.Monitored {
			monitored++
		}
		episodeFiles += item.Statistics.EpisodeFileCount
		if item.Statistics.TotalEpisodeCount > 0 {
			episodes += item.Statistics.TotalEpisodeCount
		} else {
			episodes += item.Statistics.EpisodeCount
		}
	}

	missing := max(0, episodes-episodeFiles)
	details := []string{
		fmt.Sprintf("episodes %d/%d", episodeFiles, episodes),
		fmt.Sprintf("monitored series %d/%d", monitored, len(series)),
	}
	if version != "" {
		details = append(details, "version "+version)
	}
	status := StatusOK
	if missing > 0 {
		status = StatusWarning
	}
	return status, fmt.Sprintf("%d series, %d missing episodes", len(series), missing), details
}

func JellyseerrSummary(status jellyseerrStatus, counts jellyseerrRequestCount) (Status, string, []string) {
	details := []string{
		fmt.Sprintf("approved %d  available %d", counts.Approved, counts.Available),
		fmt.Sprintf("processing %d  declined %d", counts.Processing, counts.Declined),
	}
	if status.Version != "" {
		details = append(details, "version "+status.Version)
	}

	resultStatus := StatusOK
	if counts.Pending > 0 || counts.Processing > 0 {
		resultStatus = StatusWarning
	}
	return resultStatus, fmt.Sprintf("%d requests, %d pending", counts.Total, counts.Pending), details
}
