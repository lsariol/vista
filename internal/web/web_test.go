package web

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lsariol/vista/internal/config"
	"github.com/lsariol/vista/internal/source"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// fakeSources serves just enough of Sonar, Blackbox and Lighthouse for the handlers.
func fakeSources(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var seen []string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.URL.Path+"?"+r.URL.RawQuery)
		mu.Unlock()
		switch r.URL.Path {
		case "/v1/monitor/status": // Sonar
			io.WriteString(w, `{"generated_at":"2026-11-18T12:00:00Z","fresh":true,"overall":{"state":"operational","uptime_30d":{"pct":99.9,"coverage_pct":90}},
			"projects":[{"project":"lys","state":"operational","version":"v1.0.0","evidence":"checked","response_ms":50,"uptime_30d":{"pct":100,"coverage_pct":95},"uptime_90d":{"pct":null,"coverage_pct":30},"days":[]}]}`)
		case "/v1/status": // Blackbox
			io.WriteString(w, `{"generated_at":"2026-11-18T12:00:00Z","stored_bytes":100,"max_bytes":1000,"attention":[]}`)
		case "/v1/projects":
			io.WriteString(w, `{"projects":[{"name":"lys","compose_project":"lys","kind":"deploy","deploy_mode":"releases","state":"broken",
				"deployed_version":"v1.0.0","failure_count":3,"last_error":"test: go test failed","last_error_at":"2026-11-18T11:00:00Z"}]}`)
		case "/v1/deployments":
			io.WriteString(w, `{"deployments":[{"id":7,"project":"lys","version":"v1.1.0","sha":"abcdef0123","trigger":"check","status":"failed",
				"failure_kind":"permanent","failed_step":"test","error":"test: go test failed","started_at":"2026-11-18T11:00:00Z","finished_at":"2026-11-18T11:02:00Z"}],"next_cursor":null}`)
		case "/v1/deployments/7":
			io.WriteString(w, `{"id":7,"project":"lys","status":"failed","started_at":"2026-11-18T11:00:00Z","finished_at":"2026-11-18T11:02:00Z",
				"steps":[{"step":"test","status":"failed","started_at":"2026-11-18T11:01:00Z","finished_at":"2026-11-18T11:02:00Z","log":"FAIL"}]}`)
		case "/v1/releases":
			io.WriteString(w, `{"releases":[{"tag":"v1.1.0","sha":"abcdef0123","status":"waiting","created_at":"2026-11-18T10:00:00Z"}]}`)
		case "/v1/status/history":
			io.WriteString(w, `{"range":"1h","bucket_seconds":60,"points":[{"t":"2026-11-18T11:59:00Z","cpu_pct":{"avg":10,"min":5,"max":20},"mem_used_pct":null,"coverage_pct":100}],"updates":[]}`)
		case "/v1/logs":
			io.WriteString(w, `{"records":[{"id":5,"ts":"2026-11-18T11:58:00Z","project":"lys","service":"lys","container":"lys","stream":"stderr","level":"error","message":"<script>alert(1)</script>","attrs":null}],"next_cursor":null,"unknown_left_out":0}`)
		case "/v1/logs/5/context":
			io.WriteString(w, `{"target":5,"records":[]}`)
		case "/v1/gaps":
			io.WriteString(w, `{"gaps":[]}`)
		case "/v1/sources":
			io.WriteString(w, `{"sources":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func newTestHandler(t *testing.T, connected bool) (http.Handler, *[]string) {
	cfg := config.Config{Env: "dev", Version: "v9.9.9"}
	var seen *[]string
	if connected {
		srv, s := fakeSources(t)
		seen = s
		cfg.SonarURL, cfg.SonarToken = srv.URL, "tok"
		cfg.BlackboxURL, cfg.BlackboxToken = srv.URL, "tok"
		cfg.LighthouseURL, cfg.LighthouseToken = srv.URL, "tok"
	}
	return New(source.NewSet(cfg, quiet), quiet, cfg.Version), seen
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestHealthAndPage(t *testing.T) {
	h, _ := newTestHandler(t, false)
	rec := get(h, "/healthz")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"version":"v9.9.9"`) {
		t.Errorf("healthz: %d %s", rec.Code, rec.Body)
	}
	rec = get(h, "/")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "app.js") {
		t.Errorf("index: %d", rec.Code)
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") || strings.Contains(csp, "unsafe") {
		t.Errorf("CSP = %q", csp)
	}
	for _, f := range []string{"/app.js", "/app.css", "/favicon.svg"} {
		if rec := get(h, f); rec.Code != 200 {
			t.Errorf("%s: %d", f, rec.Code)
		}
	}
	if rec := get(h, "/api/v1/nope"); rec.Code != 404 || !strings.Contains(rec.Body.String(), "error") {
		t.Errorf("unknown API path: %d %s", rec.Code, rec.Body)
	}
}

func TestOverviewWithoutSources(t *testing.T) {
	h, _ := newTestHandler(t, false)
	rec := get(h, "/api/v1/overview")
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	var o struct {
		Spark   struct{ State string }
		Sources []source.Status
	}
	json.Unmarshal(rec.Body.Bytes(), &o)
	if o.Spark.State != "unknown" {
		t.Errorf("spark = %q", o.Spark.State)
	}
	for _, s := range o.Sources {
		if s.State != source.NotConnected {
			t.Errorf("%s = %s", s.ID, s.State)
		}
	}
	if rec := get(h, "/api/v1/logs"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("logs without Blackbox: %d", rec.Code)
	}
	if rec := get(h, "/api/v1/deployments/7"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("a deploy without Lighthouse: %d", rec.Code)
	}
	if rec := get(h, "/api/v1/overview?range=2h"); rec.Code != http.StatusBadRequest {
		t.Errorf("bad range: %d", rec.Code)
	}
}

func TestOverviewWithSources(t *testing.T) {
	h, _ := newTestHandler(t, true)
	rec := get(h, "/api/v1/overview?range=1h")
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	var o struct {
		Spark    struct{ State string }
		Projects []struct {
			Name  string
			State string
		}
		Host struct {
			CPU *struct{ Pct float64 }
		}
		RecentErrors []struct{ ID int64 } `json:"recent_errors"`
		Attention    []struct{ Title string }
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &o); err != nil {
		t.Fatal(err)
	}
	if len(o.Projects) != 1 || o.Projects[0].State != "healthy" {
		t.Errorf("projects = %+v", o.Projects)
	}
	if len(o.RecentErrors) != 1 || o.RecentErrors[0].ID != 5 {
		t.Errorf("recent errors = %+v", o.RecentErrors)
	}
	if o.Host.CPU == nil || o.Host.CPU.Pct != 10 {
		t.Errorf("cpu = %+v", o.Host.CPU)
	}
	if o.Spark.State != "unknown" { // Sonar doesn't report the server's conditions yet
		t.Errorf("spark = %s", o.Spark.State)
	}
	var titles []string
	for _, a := range o.Attention {
		titles = append(titles, a.Title)
	}
	joined := strings.Join(titles, " | ")
	for _, want := range []string{"Deploy of lys failed", "Lighthouse reports lys as broken"} {
		if !strings.Contains(joined, want) {
			t.Errorf("attention %q lacks %q", joined, want)
		}
	}
}

func TestLighthouseDeployAndReleases(t *testing.T) {
	h, _ := newTestHandler(t, true)
	rec := get(h, "/api/v1/deployments/7")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"step":"test"`) {
		t.Errorf("deploy: %d %s", rec.Code, rec.Body)
	}
	if rec := get(h, "/api/v1/deployments/8"); rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "no deploy with that id") {
		t.Errorf("unknown deploy: %d %s", rec.Code, rec.Body)
	}
	if rec := get(h, "/api/v1/deployments/x"); rec.Code != http.StatusBadRequest {
		t.Errorf("bad id: %d", rec.Code)
	}
	rec = get(h, "/api/v1/projects/lys/releases")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"tag":"v1.1.0"`) {
		t.Errorf("releases: %d %s", rec.Code, rec.Body)
	}
	if rec := get(h, "/api/v1/projects/lys%20drop/releases"); rec.Code != http.StatusBadRequest {
		t.Errorf("bad project: %d", rec.Code)
	}
	rec = get(h, "/api/v1/activity?range=24h")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"deploy_id":7`) || !strings.Contains(rec.Body.String(), "Lighthouse's records") {
		t.Errorf("activity: %d %s", rec.Code, rec.Body)
	}
}

func TestLogSearchPassesCheckedParameters(t *testing.T) {
	h, seen := newTestHandler(t, true)
	rec := get(h, "/api/v1/logs?project=lys&level=error&q=time%20out&range=24h&limit=50")
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	// The message comes back as JSON text; the page puts it in as text, never HTML.
	if strings.Contains(rec.Body.String(), "<") || !strings.Contains(rec.Body.String(), "alert(1)") {
		t.Errorf("body = %s", rec.Body)
	}
	last := (*seen)[len(*seen)-1]
	q, _ := url.ParseQuery(last[strings.Index(last, "?")+1:])
	if q.Get("project") != "lys" || q.Get("level") != "error" || q.Get("q") != "time out" || q.Get("limit") != "50" || q.Get("from") == "" {
		t.Errorf("forwarded %s", last)
	}
	if rec := get(h, "/api/v1/logs/5/context?before=10"); rec.Code != 200 {
		t.Errorf("context: %d %s", rec.Code, rec.Body)
	}
}

func TestLogQueryRefusals(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	bad := []string{
		"range=5m", "from=yesterday", "from=2026-11-18T12:00:00Z&to=2026-11-18T11:00:00Z",
		"project=lys%20drop", "level=fatal", "stream=both", "order=random", "limit=0", "limit=1001",
		"cursor=" + url.QueryEscape("../../etc"), "field=novalue", "q=" + strings.Repeat("x", 201),
		"version=" + url.QueryEscape("v1 or 1=1"),
	}
	for _, qs := range bad {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/logs?"+qs, nil)
		if _, err := parseLogQuery(r, now); err == nil {
			t.Errorf("%s: accepted", qs)
		}
	}
	h, _ := newTestHandler(t, true)
	if rec := get(h, "/api/v1/logs/abc/context"); rec.Code != 400 {
		t.Errorf("non-numeric id: %d", rec.Code)
	}
	if rec := get(h, "/api/v1/logs/5/context?after=500"); rec.Code != 400 {
		t.Errorf("too many lines: %d", rec.Code)
	}
}
