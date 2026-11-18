package source

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"time"
)

// LogRecord is one log line from Blackbox. Message and Attrs are untrusted
// text, as the container wrote them.
type LogRecord struct {
	ID        int64           `json:"id"`
	Time      time.Time       `json:"ts"`
	Project   *string         `json:"project"`
	Service   string          `json:"service"`
	Container string          `json:"container"`
	Version   *string         `json:"version"`
	Stream    string          `json:"stream"`
	Level     string          `json:"level"`
	Message   string          `json:"message"`
	Attrs     json.RawMessage `json:"attrs"`
	Truncated bool            `json:"truncated"`
	Redacted  bool            `json:"redacted"`
}

// LogPage is one page of a log search.
type LogPage struct {
	Records        []LogRecord `json:"records"`
	NextCursor     *string     `json:"next_cursor"`
	UnknownLeftOut int64       `json:"unknown_left_out"`
}

// LogContext is the lines around one record, in order.
type LogContext struct {
	Target  int64       `json:"target"`
	Records []LogRecord `json:"records"`
}

// LogSource is one project and service Blackbox holds lines for.
type LogSource struct {
	Project     *string    `json:"project"`
	Service     string     `json:"service"`
	Versions    []string   `json:"versions"`
	Newest      *time.Time `json:"newest"`
	Lines       int64      `json:"lines"`
	OpenStreams int        `json:"open_streams"`
}

// LogGap is a stretch of known or possible log loss.
type LogGap struct {
	Kind      string    `json:"kind"`
	Container *string   `json:"container"`
	From      time.Time `json:"from"`
	To        time.Time `json:"to"`
	Detail    string    `json:"detail"`
	NotedAt   time.Time `json:"noted_at"`
}

// BlackboxStatus is the collector's health and storage.
type BlackboxStatus struct {
	GeneratedAt    time.Time  `json:"generated_at"`
	Version        *string    `json:"version"`
	HeartbeatAt    *time.Time `json:"heartbeat_at"`
	LastSavedAt    *time.Time `json:"last_saved_at"`
	StreamsOpen    int        `json:"streams_open"`
	StreamsRefused int        `json:"streams_refused"`
	LagSeconds     float64    `json:"lag_seconds"`
	StoredBytes    int64      `json:"stored_bytes"`
	MaxBytes       int64      `json:"max_bytes"`
	Oldest         *time.Time `json:"oldest"`
	Gaps24h        int64      `json:"gaps_24h"`
	Attention      []string   `json:"attention"`
}

// RecentErrors is how many of the newest error lines Vista shows.
const RecentErrors = 10

func blackboxStatus(c *apiClient) func(context.Context) (BlackboxStatus, error) {
	return func(ctx context.Context) (BlackboxStatus, error) {
		var s BlackboxStatus
		err := c.getJSON(ctx, "/v1/status", nil, &s)
		return s, err
	}
}

func blackboxSources(c *apiClient) func(context.Context) ([]LogSource, error) {
	return func(ctx context.Context) ([]LogSource, error) {
		var s struct {
			Sources []LogSource `json:"sources"`
		}
		err := c.getJSON(ctx, "/v1/sources", nil, &s)
		return s.Sources, err
	}
}

func blackboxRecentErrors(c *apiClient) func(context.Context) ([]LogRecord, error) {
	return func(ctx context.Context) ([]LogRecord, error) {
		to := time.Now().UTC()
		var page LogPage
		err := c.getJSON(ctx, "/v1/logs", url.Values{
			"level": {"error"},
			"from":  {to.Add(-24 * time.Hour).Format(time.RFC3339)},
			"to":    {to.Format(time.RFC3339)},
			"limit": {strconv.Itoa(RecentErrors)},
		}, &page)
		return page.Records, err
	}
}

func blackboxGaps(c *apiClient, window time.Duration) func(context.Context) ([]LogGap, error) {
	return func(ctx context.Context) ([]LogGap, error) {
		to := time.Now().UTC()
		var g struct {
			Gaps []LogGap `json:"gaps"`
		}
		err := c.getJSON(ctx, "/v1/gaps", url.Values{
			"from": {to.Add(-window).Format(time.RFC3339)},
			"to":   {to.Format(time.RFC3339)},
		}, &g)
		return g.Gaps, err
	}
}
