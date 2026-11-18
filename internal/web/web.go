// Package web serves Vista's pages and the JSON API they read.
package web

import (
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lsariol/vista/internal/dashboard"
	"github.com/lsariol/vista/internal/source"
)

//go:embed static
var static embed.FS

// Server answers Vista's HTTP requests.
type Server struct {
	src     *source.Set
	log     *slog.Logger
	version string
	now     func() time.Time
}

// New returns Vista's handler. Sign-in is checked in front of it (access).
func New(src *source.Set, log *slog.Logger, version string) http.Handler {
	s := &Server{src: src, log: log, version: version, now: time.Now}
	files, err := fs.Sub(static, "static")
	if err != nil {
		panic(err) // the embedded folder is part of the build
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /api/v1/overview", s.overview)
	mux.HandleFunc("GET /api/v1/activity", s.activity)
	mux.HandleFunc("GET /api/v1/incidents", s.incidents)
	mux.HandleFunc("GET /api/v1/logs", s.logs)
	mux.HandleFunc("GET /api/v1/logs/{id}/context", s.logContext)
	mux.HandleFunc("GET /api/v1/log-sources", s.logSources)
	mux.HandleFunc("GET /api/v1/deployments/{id}", s.deployment)
	mux.HandleFunc("GET /api/v1/projects/{name}/releases", s.releases)
	mux.HandleFunc("GET /api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "No such API path.")
	})
	fileServer := http.FileServerFS(files)
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		fileServer.ServeHTTP(w, r)
	})
	return securityHeaders(mux)
}

// csp allows only Vista's own files: every value from a source is put on the
// page as text, never as HTML.
const csp = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; " +
	"base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": s.version})
}

// rangeParam reads ?range=, defaulting to def.
func rangeParam(r *http.Request, def string) (string, bool) {
	v := r.URL.Query().Get("range")
	if v == "" {
		return def, true
	}
	_, ok := source.RangeDuration(v)
	return v, ok
}

const badRange = "range must be 1h, 24h, 7d or 30d."

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	rng, ok := rangeParam(r, "1h")
	if !ok {
		writeError(w, http.StatusBadRequest, badRange)
		return
	}
	ctx := r.Context()
	in := dashboard.Inputs{Now: s.now(), Range: rng}
	var mu sync.Mutex
	var states []source.Status
	note := func(st source.Status) {
		mu.Lock()
		states = append(states, st)
		mu.Unlock()
	}
	parallel(
		func() {
			v, have, st := s.src.SonarStatus.Get(ctx)
			if have {
				in.Status = &v
			}
			in.StatusState = st
			note(st)
		},
		func() {
			v, have, st := s.src.SonarHistory["1h"].Get(ctx)
			if have {
				in.Current = &v
			}
			note(st)
		},
		func() {
			v, have, st := s.src.SonarHistory[rng].Get(ctx)
			if have {
				in.History = &v
			}
			note(st)
		},
		func() {
			v, have, st := s.src.SonarHistory["24h"].Get(ctx)
			if have {
				in.Day = &v
			}
			note(st)
		},
		func() {
			v, have, st := s.src.RecentErrors.Get(ctx)
			in.RecentErrors, in.ErrorsKnown = v, have
			note(st)
		},
		func() {
			v, have, st := s.src.Gaps["24h"].Get(ctx)
			in.Gaps24h, in.GapsKnown = v, have && st.State == source.OK
			note(st)
		},
		func() {
			v, have, st := s.src.BlackboxStatus.Get(ctx)
			if have {
				in.Blackbox = &v
			}
			note(st)
		},
		func() {
			v, have, st := s.src.LighthouseProjects.Get(ctx)
			in.Lighthouse, in.LHKnown = v, have
			note(st)
		},
		func() {
			v, have, st := s.src.Deployments[rng].Get(ctx)
			in.Deploys, in.DeploysKnown = v, have
			note(st)
		},
		func() {
			v, have, st := s.src.Deployments["24h"].Get(ctx)
			in.DayDeploys, in.DayKnown = v, have
			note(st)
		},
	)
	in.SourceStates = orderSources(append(states, appsStatus))
	writeJSON(w, http.StatusOK, dashboard.BuildOverview(in))
}

// appsStatus is the source Vista will read once applications serve usage counts.
var appsStatus = source.Status{ID: "apps", Name: "Applications", State: source.NotConnected,
	Detail: "No application serves usage counts yet."}

// sourceOrder is the order sources are listed in.
var sourceOrder = map[string]int{"sonar": 0, "blackbox": 1, "lighthouse": 2, "apps": 3}

func orderSources(states []source.Status) []source.Status {
	out := make([]source.Status, len(states))
	copy(out, states)
	for i := 1; i < len(out); i++ { // tiny list: insertion sort, stable
		for j := i; j > 0 && sourceOrder[out[j].ID] < sourceOrder[out[j-1].ID]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func (s *Server) activity(w http.ResponseWriter, r *http.Request) {
	rng, ok := rangeParam(r, "24h")
	if !ok {
		writeError(w, http.StatusBadRequest, badRange)
		return
	}
	ctx := r.Context()
	in := dashboard.ActivityInputs{Now: s.now(), Range: rng}
	var states [3]source.Status
	parallel(
		func() {
			v, have, st := s.src.SonarHistory[rng].Get(ctx)
			if have {
				in.History = &v
			}
			states[0] = st
		},
		func() {
			v, _, st := s.src.Gaps[rng].Get(ctx)
			in.Gaps = v
			states[1] = st
		},
		func() {
			v, have, st := s.src.Deployments[rng].Get(ctx)
			in.Deploys, in.DeploysKnown = v, have
			states[2] = st
		},
	)
	in.SourceStates = states[:]
	writeJSON(w, http.StatusOK, dashboard.BuildActivity(in))
}

func (s *Server) incidents(w http.ResponseWriter, r *http.Request) {
	v, have, st := s.src.SonarStatus.Get(r.Context())
	var status *source.SonarStatus
	if have {
		status = &v
	}
	writeJSON(w, http.StatusOK, dashboard.BuildIncidents(s.now(), status, []source.Status{st}))
}

func (s *Server) logSources(w http.ResponseWriter, r *http.Request) {
	v, _, st := s.src.LogSources.Get(r.Context())
	if v == nil {
		v = []source.LogSource{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": v, "source": st})
}

var (
	cursorPattern = regexp.MustCompile(`^[A-Za-z0-9_=-]{1,200}$`)
	namePattern   = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
)

// parseLogQuery checks a log search's parameters before they reach Blackbox.
func parseLogQuery(r *http.Request, now time.Time) (source.LogQuery, error) {
	v := r.URL.Query()
	q := source.LogQuery{Order: "newest", Limit: 100}

	if rng := v.Get("range"); rng != "" {
		d, ok := source.RangeDuration(rng)
		if !ok {
			return q, errors.New(badRange)
		}
		q.To = now.UTC()
		q.From = q.To.Add(-d)
	}
	for key, dst := range map[string]*time.Time{"from": &q.From, "to": &q.To} {
		if raw := v.Get(key); raw != "" {
			t, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				return q, errors.New(key + " must be an RFC 3339 time, such as 2026-11-18T12:00:00Z.")
			}
			*dst = t
		}
	}
	if !q.From.IsZero() && !q.To.IsZero() && !q.From.Before(q.To) {
		return q, errors.New("from must be before to.")
	}
	for key, dst := range map[string]*[]string{"project": &q.Projects, "service": &q.Services} {
		vals := v[key]
		if len(vals) > 10 {
			return q, errors.New("at most 10 " + key + " values.")
		}
		for _, val := range vals {
			if val == "" {
				continue
			}
			if !namePattern.MatchString(val) {
				return q, errors.New(key + " must be a plain name.")
			}
			*dst = append(*dst, val)
		}
	}
	if fields := v["field"]; len(fields) > 5 {
		return q, errors.New("at most 5 field filters.")
	} else {
		for _, f := range fields {
			if f == "" {
				continue
			}
			if !strings.Contains(f, "=") || len(f) > 200 {
				return q, errors.New("field must be key=value, such as request_id=abc.")
			}
			q.Fields = append(q.Fields, f)
		}
	}
	q.Version = v.Get("version")
	if q.Version != "" && !namePattern.MatchString(q.Version) {
		return q, errors.New("version must be a plain version, such as v1.3.0.")
	}
	switch q.Level = v.Get("level"); q.Level {
	case "", "debug", "info", "warn", "error":
	default:
		return q, errors.New("level must be debug, info, warn or error.")
	}
	switch q.Stream = v.Get("stream"); q.Stream {
	case "", "stdout", "stderr":
	default:
		return q, errors.New("stream must be stdout or stderr.")
	}
	if q.Q = strings.TrimSpace(v.Get("q")); len(q.Q) > 200 {
		return q, errors.New("the search text is limited to 200 characters.")
	}
	if o := v.Get("order"); o != "" {
		if o != "newest" && o != "oldest" {
			return q, errors.New("order must be newest or oldest.")
		}
		q.Order = o
	}
	if l := v.Get("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 1 || n > 500 {
			return q, errors.New("limit must be between 1 and 500.")
		}
		q.Limit = n
	}
	if c := v.Get("cursor"); c != "" {
		if !cursorPattern.MatchString(c) {
			return q, errors.New("cursor isn't one Blackbox gave.")
		}
		q.Cursor = c
	}
	return q, nil
}

func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	q, err := parseLogQuery(r, s.now())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	page, err := s.src.SearchLogs(r.Context(), q)
	if err != nil {
		s.sourceError(w, "log search", err)
		return
	}
	if page.Records == nil {
		page.Records = []source.LogRecord{}
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) logContext(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, "The log line's id must be a positive number.")
		return
	}
	side := func(key string) (int, bool) {
		raw := r.URL.Query().Get(key)
		if raw == "" {
			return 20, true
		}
		n, err := strconv.Atoi(raw)
		return n, err == nil && n >= 0 && n <= 200
	}
	before, ok1 := side("before")
	after, ok2 := side("after")
	if !ok1 || !ok2 {
		writeError(w, http.StatusBadRequest, "before and after must be between 0 and 200.")
		return
	}
	c, err := s.src.LogContext(r.Context(), id, before, after)
	if err != nil {
		s.sourceError(w, "log context", err)
		return
	}
	if c.Records == nil {
		c.Records = []source.LogRecord{}
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) deployment(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, "The deploy's id must be a positive number.")
		return
	}
	d, err := s.src.Deployment(r.Context(), id)
	if err != nil {
		s.sourceError(w, "deployment", err)
		return
	}
	if d.Steps == nil {
		d.Steps = []source.LighthouseStep{}
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) releases(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !namePattern.MatchString(name) {
		writeError(w, http.StatusBadRequest, "The project must be a plain name.")
		return
	}
	rel, err := s.src.Releases(r.Context(), name)
	if err != nil {
		s.sourceError(w, "releases", err)
		return
	}
	if rel == nil {
		rel = []source.LighthouseRelease{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"releases": rel})
}

// notFound says what a 404 from each call means.
var notFound = map[string]string{
	"log context": "Blackbox no longer has that log line: it may be past retention.",
	"deployment":  "Lighthouse has no deploy with that id: its project may have been removed.",
	"releases":    "Lighthouse doesn't know that project.",
}

// sourceError answers a failed call to a source.
func (s *Server) sourceError(w http.ResponseWriter, what string, err error) {
	switch {
	case errors.Is(err, source.ErrLogsNotConnected), errors.Is(err, source.ErrLighthouseNotConnected):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, source.ErrNotFound):
		msg := notFound[what]
		if msg == "" {
			msg = "Not found."
		}
		writeError(w, http.StatusNotFound, msg)
	default:
		s.log.Warn("source call failed", "component", "http", "call", what, "err", err)
		writeError(w, http.StatusBadGateway, err.Error())
	}
}

// parallel runs fns at once and waits for all of them.
func parallel(fns ...func()) {
	var wg sync.WaitGroup
	for _, f := range fns {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f()
		}()
	}
	wg.Wait()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
