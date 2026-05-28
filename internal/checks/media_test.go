package checks

import (
	"strings"
	"testing"
)

func TestJellyfinCountsSummary(t *testing.T) {
	latestMovie := jellyfinItem{Name: "Dune: Part Two"}
	latestEpisode := jellyfinItem{
		Name:              "The Beginning",
		SeriesName:        "Foundation",
		ParentIndexNumber: 2,
		IndexNumber:       1,
	}
	status, summary, details := JellyfinCountsSummary(jellyfinCounts{
		MovieCount:   12,
		SeriesCount:  3,
		EpisodeCount: 90,
		SongCount:    25,
		AlbumCount:   2,
		ArtistCount:  4,
		BookCount:    7,
		BoxSetCount:  5,
	}, &latestMovie, &latestEpisode, "10.10.7")

	if status != StatusOK {
		t.Fatalf("status = %q, want %q", status, StatusOK)
	}
	if summary != "105 video items" {
		t.Fatalf("summary = %q", summary)
	}
	detailText := strings.Join(details, "\n")
	if !strings.Contains(detailText, "movies 12") || strings.Contains(detailText, "songs") || strings.Contains(detailText, "books") {
		t.Fatalf("details = %#v", details)
	}
	if !strings.Contains(detailText, "last movie: Dune: Part Two") {
		t.Fatalf("details = %#v", details)
	}
	if !strings.Contains(detailText, "last episode: Foundation S02E01 - The Beginning") {
		t.Fatalf("details = %#v", details)
	}
	if !strings.Contains(detailText, "version 10.10.7") {
		t.Fatalf("details = %#v", details)
	}
}

func TestRadarrSummaryWarnsForMissingMonitoredMovies(t *testing.T) {
	status, summary, details := RadarrSummary([]radarrMovie{
		{Title: "Ready", Monitored: true, HasFile: true},
		{Title: "Wanted", Monitored: true, HasFile: false},
		{Title: "Ignored", Monitored: false, HasFile: false},
	}, "5.4.3")

	if status != StatusWarning {
		t.Fatalf("status = %q, want %q", status, StatusWarning)
	}
	if summary != "3 movies, 1 missing" {
		t.Fatalf("summary = %q", summary)
	}
	if !strings.Contains(strings.Join(details, "\n"), "Wanted") {
		t.Fatalf("details = %#v", details)
	}
	if !strings.Contains(strings.Join(details, "\n"), "version 5.4.3") {
		t.Fatalf("details = %#v", details)
	}
}

func TestSonarrSummaryUsesEpisodeTotals(t *testing.T) {
	series := []sonarrSeries{{Title: "Show", Monitored: true}}
	series[0].Statistics.EpisodeFileCount = 8
	series[0].Statistics.TotalEpisodeCount = 10

	status, summary, details := SonarrSummary(series, "4.0.15")

	if status != StatusWarning {
		t.Fatalf("status = %q, want %q", status, StatusWarning)
	}
	if summary != "1 series, 2 missing episodes" {
		t.Fatalf("summary = %q", summary)
	}
	if !strings.Contains(strings.Join(details, "\n"), "episodes 8/10") {
		t.Fatalf("details = %#v", details)
	}
	if !strings.Contains(strings.Join(details, "\n"), "version 4.0.15") {
		t.Fatalf("details = %#v", details)
	}
}

func TestJellyseerrSummaryWarnsForPendingRequests(t *testing.T) {
	status, summary, _ := JellyseerrSummary(jellyseerrStatus{Version: "1.2.3"}, jellyseerrRequestCount{
		Total:   6,
		Pending: 2,
	})

	if status != StatusWarning {
		t.Fatalf("status = %q, want %q", status, StatusWarning)
	}
	if summary != "6 requests, 2 pending" {
		t.Fatalf("summary = %q", summary)
	}
}
