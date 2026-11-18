package source

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"strconv"
	"time"

	"github.com/lsariol/vista/internal/config"
)

// Set is every source Vista reads, each behind a short cache. Vista asks
// only when a page does; it collects nothing on its own.
type Set struct {
	SonarStatus    *Cached[SonarStatus]
	SonarHistory   map[string]*Cached[SonarHistory] // by range
	BlackboxStatus *Cached[BlackboxStatus]
	LogSources     *Cached[[]LogSource]
	RecentErrors   *Cached[[]LogRecord]         // the newest error lines, last 24 hours
	Gaps           map[string]*Cached[[]LogGap] // by range

	LighthouseProjects *Cached[[]LighthouseProject]
	Deployments        map[string]*Cached[[]LighthouseDeployment] // by range

	blackbox   *apiClient // nil when Blackbox isn't connected
	lighthouse *apiClient // nil when Lighthouse isn't connected
}

// Sonar's and Blackbox's Cove keys for Vista's tokens, named in error messages.
const (
	SonarTokenKey      = "SHARED_SONAR_VISTA_TOKEN"
	BlackboxTokenKey   = "SHARED_BLACKBOX_VISTA_TOKEN"
	LighthouseTokenKey = "SHARED_LIGHTHOUSE_VISTA_TOKEN"
)

// NewSet builds the sources the configuration connects.
func NewSet(cfg config.Config, log *slog.Logger) *Set {
	s := &Set{
		SonarHistory: map[string]*Cached[SonarHistory]{},
		Gaps:         map[string]*Cached[[]LogGap]{},
		Deployments:  map[string]*Cached[[]LighthouseDeployment]{},
	}

	var sonar *apiClient
	if cfg.SonarURL != "" && cfg.SonarToken != "" {
		sonar = newAPIClient("Sonar", cfg.SonarURL, cfg.SonarToken, SonarTokenKey)
	}
	sonarMissing := "Set VISTA_SONAR_URL and VISTA_SONAR_TOKEN (Cove key " + SonarTokenKey + ")."
	s.SonarStatus = NewCached("sonar", "Sonar", 10*time.Second, fetchOrNil(sonar, sonarStatus), sonarMissing, log)
	for _, r := range Ranges {
		var fetch func(context.Context) (SonarHistory, error)
		if sonar != nil {
			fetch = sonarHistory(sonar, r)
		}
		s.SonarHistory[r] = NewCached("sonar", "Sonar", 30*time.Second, fetch, sonarMissing, log)
	}

	if cfg.BlackboxURL != "" && cfg.BlackboxToken != "" {
		s.blackbox = newAPIClient("Blackbox", cfg.BlackboxURL, cfg.BlackboxToken, BlackboxTokenKey)
	}
	bbMissing := "Set VISTA_BLACKBOX_URL and VISTA_BLACKBOX_TOKEN (Cove key " + BlackboxTokenKey + ")."
	s.BlackboxStatus = NewCached("blackbox", "Blackbox", 15*time.Second, fetchOrNil(s.blackbox, blackboxStatus), bbMissing, log)
	s.LogSources = NewCached("blackbox", "Blackbox", time.Minute, fetchOrNil(s.blackbox, blackboxSources), bbMissing, log)
	s.RecentErrors = NewCached("blackbox", "Blackbox", 30*time.Second, fetchOrNil(s.blackbox, blackboxRecentErrors), bbMissing, log)
	for _, r := range Ranges {
		d, _ := RangeDuration(r)
		var gaps func(context.Context) ([]LogGap, error)
		if s.blackbox != nil {
			gaps = blackboxGaps(s.blackbox, d)
		}
		s.Gaps[r] = NewCached("blackbox", "Blackbox", time.Minute, gaps, bbMissing, log)
	}

	if cfg.LighthouseURL != "" && cfg.LighthouseToken != "" {
		s.lighthouse = newAPIClient("Lighthouse", cfg.LighthouseURL, cfg.LighthouseToken, LighthouseTokenKey)
	}
	lhMissing := "Set VISTA_LIGHTHOUSE_URL and VISTA_LIGHTHOUSE_TOKEN (Cove key " + LighthouseTokenKey + ")."
	s.LighthouseProjects = NewCached("lighthouse", "Lighthouse", 30*time.Second, fetchOrNil(s.lighthouse, lighthouseProjects), lhMissing, log)
	for _, r := range Ranges {
		d, _ := RangeDuration(r)
		var deploys func(context.Context) ([]LighthouseDeployment, error)
		if s.lighthouse != nil {
			deploys = lighthouseDeployments(s.lighthouse, d)
		}
		s.Deployments[r] = NewCached("lighthouse", "Lighthouse", 30*time.Second, deploys, lhMissing, log)
	}
	return s
}

// LighthouseConnected reports whether Vista has Lighthouse's address and token.
func (s *Set) LighthouseConnected() bool { return s.lighthouse != nil }

// fetchOrNil returns make(c), or nil when the client isn't connected.
func fetchOrNil[T any](c *apiClient, make func(*apiClient) func(context.Context) (T, error)) func(context.Context) (T, error) {
	if c == nil {
		return nil
	}
	return make(c)
}

// ErrLogsNotConnected means Vista has no Blackbox token.
var ErrLogsNotConnected = errors.New("Blackbox isn't connected: set VISTA_BLACKBOX_URL and VISTA_BLACKBOX_TOKEN (Cove key " + BlackboxTokenKey + ")")

// ErrLighthouseNotConnected means Vista has no Lighthouse token.
var ErrLighthouseNotConnected = errors.New("Lighthouse isn't connected: set VISTA_LIGHTHOUSE_URL and VISTA_LIGHTHOUSE_TOKEN (Cove key " + LighthouseTokenKey + ")")

// Deployment returns one deploy with its steps, uncached.
func (s *Set) Deployment(ctx context.Context, id int64) (LighthouseDeploymentDetail, error) {
	if s.lighthouse == nil {
		return LighthouseDeploymentDetail{}, ErrLighthouseNotConnected
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	var d LighthouseDeploymentDetail
	err := s.lighthouse.getJSON(ctx, "/v1/deployments/"+strconv.FormatInt(id, 10), nil, &d)
	return d, err
}

// Releases returns a project's version tags, newest first, uncached.
func (s *Set) Releases(ctx context.Context, project string) ([]LighthouseRelease, error) {
	if s.lighthouse == nil {
		return nil, ErrLighthouseNotConnected
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	var a struct {
		Releases []LighthouseRelease `json:"releases"`
	}
	err := s.lighthouse.getJSON(ctx, "/v1/releases", url.Values{"project": {project}, "limit": {"20"}}, &a)
	return a.Releases, err
}

// LogQuery is a validated log search.
type LogQuery struct {
	From, To                   time.Time
	Projects, Services, Fields []string
	Version, Level, Stream, Q  string
	Order, Cursor              string
	Limit                      int
}

// SearchLogs runs a log search on Blackbox, uncached.
func (s *Set) SearchLogs(ctx context.Context, q LogQuery) (LogPage, error) {
	if s.blackbox == nil {
		return LogPage{}, ErrLogsNotConnected
	}
	v := url.Values{}
	if !q.From.IsZero() {
		v.Set("from", q.From.UTC().Format(time.RFC3339))
	}
	if !q.To.IsZero() {
		v.Set("to", q.To.UTC().Format(time.RFC3339))
	}
	for _, p := range q.Projects {
		v.Add("project", p)
	}
	for _, sv := range q.Services {
		v.Add("service", sv)
	}
	for _, f := range q.Fields {
		v.Add("field", f)
	}
	for key, val := range map[string]string{"version": q.Version, "level": q.Level, "stream": q.Stream, "q": q.Q, "order": q.Order, "cursor": q.Cursor} {
		if val != "" {
			v.Set(key, val)
		}
	}
	if q.Limit > 0 {
		v.Set("limit", strconv.Itoa(q.Limit))
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	var page LogPage
	err := s.blackbox.getJSON(ctx, "/v1/logs", v, &page)
	return page, err
}

// LogContext returns the lines around record id.
func (s *Set) LogContext(ctx context.Context, id int64, before, after int) (LogContext, error) {
	if s.blackbox == nil {
		return LogContext{}, ErrLogsNotConnected
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	var c LogContext
	err := s.blackbox.getJSON(ctx, "/v1/logs/"+strconv.FormatInt(id, 10)+"/context",
		url.Values{"before": {strconv.Itoa(before)}, "after": {strconv.Itoa(after)}}, &c)
	return c, err
}
