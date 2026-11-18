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

// fakeSources serves just enough of Sonar and Blackbox for the handlers.
func fakeSources(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var seen []string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.URL.Path+"?"+r.URL.RawQuery)
		mu.Unlock()
		switch r.URL.Path {
		case "/v1/status":
			if r.URL.Query().Get("days") != "" { // Sonar
				io.WriteString(w, `{"generated_at":"2026-11-18T12:00:00Z","fresh":true,"overall":{"state":"operational","uptime_30d":{"pct":99.9,"coverage_pct":90}},
				"projects":[{"project":"lys","state":"operational","version":"v1.0.0","evidence":"checked","response_ms":50,"uptime_30d":{"pct":100,"coverage_pct":95},"uptime_90d":{"pct":null,"coverage_pct":30},"days":[]}]}`)
				return
			}
			io.WriteString(w, `{"generated_at":"2026-11-18T12:00:00Z","stored_bytes":100,"max_bytes":1000,"attention":[]}`)
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
		case "/healthz":
			io.WriteString(w, `{"status":"ok","version":"v0.1.0"}`)
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
			Name      string
			State     string
			Errors24h *struct{ N int } `json:"errors_24h"`
		}
		Host struct {
			CPU *struct{ Pct float64 }
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &o); err != nil {
		t.Fatal(err)
	}
	if len(o.Projects) != 1 || o.Projects[0].State != "healthy" || o.Projects[0].Errors24h == nil || o.Projects[0].Errors24h.N != 1 {
		t.Errorf("projects = %+v", o.Projects)
	}
	if o.Host.CPU == nil || o.Host.CPU.Pct != 10 {
		t.Errorf("cpu = %+v", o.Host.CPU)
	}
	if o.Spark.State != "healthy" {
		t.Errorf("spark = %s", o.Spark.State)
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
