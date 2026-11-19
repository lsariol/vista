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
		case "/v1/monitor/host":
			io.WriteString(w, `{"range":"1h","bucket_seconds":60,"now":{"read_at":"2026-11-18T11:59:30Z","cpu_pct":10,"core_pcts":[9,11],"memory_total":1000,"memory_used":400,"memory_used_pct":40,
				"load1":0.5,"load5":0.4,"load15":0.3,"uptime_seconds":3600,"disk":{"total":100,"used":89,"available":11,"used_pct":89},"network":[{"interface":"eth0","rx_bytes_per_s":2000,"tx_bytes_per_s":500}]},
				"points":[{"t":"2026-11-18T11:59:00Z","cpu_pct":{"avg":10,"min":5,"max":20},"mem_used_pct":null,"coverage_pct":100,"disk_used_bytes":89}],
				"network":[{"interface":"eth0","points":[{"t":"2026-11-18T11:59:00Z","minutes":1,"rx_bytes":120000,"tx_bytes":30000}]}],"updates":[]}`)
		case "/v1/monitor/projects":
			io.WriteString(w, `{"fresh":true,"projects":[{"project":"lys","group":"app","state":"operational","state_since":"2026-11-18T10:00:00Z","evidence":"checked","services":[
				{"service":"lys","state":"operational","counted":true,"usage":{"cpu_cores":0.2},"containers":[{"name":"lys","image":"lys-lys","state":"running","health":"healthy","restart_count":2,
				"memory_limit":268435456,"usage":{"cpu_cores":0.2,"memory_bytes":104857600,"net_rx_bytes_per_s":300,"net_tx_bytes_per_s":100}}]}],"checks":[]}]}`)
		case "/v1/monitor/conditions":
			io.WriteString(w, `{"fresh":true,"state":"warning","conditions":[{"id":"disk","subject":null,"level":"warning","message":"The disk is 89% full","since":"2026-11-18T11:00:00Z"},
				{"id":"project","subject":"lys","level":"warning","message":"lys is degraded","since":"2026-11-18T11:00:00Z"}],"collectors":[],"attention":[]}`)
		case "/v1/monitor/databases":
			io.WriteString(w, `{"now":{"max_connections":100,"connections":7,"databases":[{"name":"lys_db","project":"lys","size_bytes":5000,"connections":3,"active":1}]}}`)
		case "/v1/monitor/disk":
			io.WriteString(w, `{"now":{"totals":{"image_bytes":10},"projects":[{"project":"lys","data_bytes":42,"image_bytes":7}]}}`)
		case "/v1/monitor/traffic":
			io.WriteString(w, `{"range":"24h","hosts":[{"host":"lys.mobasity.com","project":"lys","requests":120,"errors_5xx":2,"bytes":9000,"points":[]}]}`)
		case "/v1/monitor/apps":
			io.WriteString(w, `{"days":30,"projects":[]}`)
		case "/v1/monitor/states":
			io.WriteString(w, `{"range":"24h","projects":[{"project":"lys","spans":[{"state":"down","start":"2026-11-18T09:00:00Z","end":"2026-11-18T09:20:00Z","observed_minutes":20,
				"services":[{"service":"lys","state":"down","minutes":20}],"checks":[]},{"state":"operational","start":"2026-11-18T09:20:00Z","end":"2026-11-18T12:00:00Z","observed_minutes":160,"services":[],"checks":[]}]}]}`)
		case "/v1/monitor/usage":
			io.WriteString(w, `{"range":"24h","bucket_seconds":300,"services":[{"project":"lys","service":"lys","points":[{"t":"2026-11-18T11:55:00Z","minutes":5,"cpu_cores":{"avg":0.2,"max":0.4}}]}]}`)
		case "/v1/stats/errors":
			io.WriteString(w, `{"bucket":"hour","buckets":[{"start":"2026-11-18T11:00:00Z","project":"lys","error":4,"warn":1}],"totals":[{"project":"lys","error":4,"warn":1}]}`)
		case "/v1/stats/errors/groups":
			io.WriteString(w, `{"groups":[{"fingerprint":"db timeout after <n>ms","project":"lys","service":"lys","level":"error","count":4,"first_seen":"2026-11-18T11:00:00Z","last_seen":"2026-11-18T11:58:00Z","newest_id":5,"newest_message":"db timeout after 30ms"}],"total_groups":1}`)
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
			Now *struct {
				Load1 float64
			}
			Network []struct{ Interface string }
		}
		RecentErrors []struct{ ID int64 } `json:"recent_errors"`
		Attention    []struct{ Title string }
		Containers   []struct {
			Project, Service, Name string
			RestartCount           int `json:"restart_count"`
		}
		Errors *struct {
			Totals []struct{ Error int64 }
			Groups []struct{ Count int64 }
		}
		Disk      *struct{}
		Databases *struct{ Connections int }
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
	if o.Spark.State != "degraded" { // Sonar's conditions say warning
		t.Errorf("spark = %s", o.Spark.State)
	}
	if o.Host.Now == nil || o.Host.Now.Load1 != 0.5 || len(o.Host.Network) != 1 {
		t.Errorf("host now = %+v, network = %+v", o.Host.Now, o.Host.Network)
	}
	if len(o.Containers) != 1 || o.Containers[0].Project != "lys" || o.Containers[0].RestartCount != 2 {
		t.Errorf("containers = %+v", o.Containers)
	}
	if o.Errors == nil || len(o.Errors.Totals) != 1 || o.Errors.Totals[0].Error != 4 || len(o.Errors.Groups) != 1 {
		t.Errorf("errors = %+v", o.Errors)
	}
	if o.Disk == nil || o.Databases == nil || o.Databases.Connections != 7 {
		t.Errorf("disk = %v, databases = %+v", o.Disk, o.Databases)
	}
	var titles []string
	for _, a := range o.Attention {
		titles = append(titles, a.Title)
	}
	joined := strings.Join(titles, " | ")
	if strings.Contains(joined, "lys is degraded") {
		t.Errorf("a project condition is listed twice: %q", joined)
	}
	for _, want := range []string{"Deploy of lys failed", "Lighthouse reports lys as broken", "The disk is 89% full"} {
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
	if body := rec.Body.String(); !strings.Contains(body, `"kind":"state"`) || !strings.Contains(body, `"title":"lys down"`) || !strings.Contains(body, `"bucket":"hour"`) {
		t.Errorf("activity lacks the state change or error counts: %s", body)
	}
}

func TestIncidentsAndUsage(t *testing.T) {
	h, seen := newTestHandler(t, true)
	rec := get(h, "/api/v1/incidents")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"incidents":[{"project":"lys","state":"down"`) {
		t.Errorf("incidents: %d %s", rec.Code, rec.Body)
	}
	rec = get(h, "/api/v1/projects/lys/usage?range=24h")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"service":"lys"`) {
		t.Errorf("usage: %d %s", rec.Code, rec.Body)
	}
	last := (*seen)[len(*seen)-1]
	if !strings.Contains(last, "/v1/monitor/usage?") || !strings.Contains(last, "project=lys") || !strings.Contains(last, "range=24h") {
		t.Errorf("forwarded %s", last)
	}
	if rec := get(h, "/api/v1/projects/lys/usage?range=2h"); rec.Code != http.StatusBadRequest {
		t.Errorf("bad range: %d", rec.Code)
	}
	h, _ = newTestHandler(t, false)
	if rec := get(h, "/api/v1/projects/lys/usage"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("usage without Sonar: %d", rec.Code)
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
