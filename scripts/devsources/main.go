// Command devsources stands in for Sonar and Blackbox on a development
// machine, with made-up data, so Vista can be run and looked at locally.
//
//	go run ./scripts/devsources            # listens on :2599
//	APP_ENV=dev VISTA_SONAR_URL=http://localhost:2599 VISTA_SONAR_TOKEN=dev \
//	  VISTA_BLACKBOX_URL=http://localhost:2599 VISTA_BLACKBOX_TOKEN=dev \
//	  go run ./cmd/vista serve
package main

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type project struct {
	name, state, version, evidence string
	response                       int
	downDays                       map[int]int // days ago → minutes down
}

var projects = []project{
	{"blackbox", "operational", "v0.0.1", "container", 0, nil},
	{"cove", "operational", "v1.4.0", "container", 0, nil},
	{"dibs", "down", "v0.3.0", "checked", 0, map[int]int{0: 14, 6: 32}},
	{"lighthouse", "operational", "v2.3.1", "container", 0, nil},
	{"lucassariol-web", "operational", "v1.0.2", "checked", 38, nil},
	{"lys", "degraded", "v0.5.1", "checked", 182, map[int]int{3: 4}},
	{"marquee", "operational", "v1.1.0", "checked", 64, nil},
	{"mobasity-web", "updating", "v1.8.0", "checked", 41, nil},
	{"plop", "operational", "v1.3.0", "checked", 51, map[int]int{20: 90}},
	{"sonar", "operational", "v0.2.5", "container", 0, nil},
	{"vista", "operational", "v0.0.0", "checked", 12, nil},
}

func main() {
	addr := ":2599"
	if len(os.Args) > 1 {
		addr = os.Args[1]
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/status", status)
	mux.HandleFunc("GET /v1/status/history", history)
	mux.HandleFunc("GET /v1/logs", logs)
	mux.HandleFunc("GET /v1/logs/{id}/context", logContext)
	mux.HandleFunc("GET /v1/sources", sources)
	mux.HandleFunc("GET /v1/gaps", gaps)
	log.Printf("devsources: listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// status is Sonar's /v1/status, or Blackbox's when called without days.
func status(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	if r.URL.Query().Get("days") == "" {
		oldest := now.Add(-29 * 24 * time.Hour)
		writeJSON(w, map[string]any{
			"generated_at": now, "version": "v0.0.1", "heartbeat_at": now, "last_saved_at": now,
			"streams_open": 14, "streams_refused": 0, "lag_seconds": 0.4,
			"stored_bytes": 412 << 20, "max_bytes": 1 << 30, "oldest": oldest, "gaps_24h": 1, "attention": []string{},
		})
		return
	}
	var out []map[string]any
	for _, p := range projects {
		var days []map[string]any
		for ago := 89; ago >= 0; ago-- {
			date := now.AddDate(0, 0, -ago).Format("2006-01-02")
			if ago > 75 {
				days = append(days, map[string]any{"date": date, "state": "no_data", "uptime_pct": nil, "coverage_pct": 0, "updates": 0,
					"observed_minutes": 0, "degraded_minutes": 0, "down_minutes": 0})
				continue
			}
			observed := 1440
			if ago == 0 {
				observed = now.Hour()*60 + now.Minute()
			}
			down := p.downDays[ago]
			degraded := 0
			if p.name == "lys" && ago == 0 {
				degraded = 25
			}
			state := "operational"
			switch {
			case down > 5:
				state = "down"
			case degraded > 5 || down > 0:
				state = "degraded"
			}
			up := float64(observed-down) / float64(max(observed, 1)) * 100
			days = append(days, map[string]any{"date": date, "state": state, "uptime_pct": up, "coverage_pct": float64(observed) / 14.4,
				"updates": map[bool]int{true: 1, false: 0}[ago%9 == 2], "observed_minutes": observed, "degraded_minutes": degraded, "down_minutes": down})
		}
		var resp any
		if p.evidence == "checked" && p.state != "down" {
			resp = p.response
		}
		out = append(out, map[string]any{
			"project": p.name, "state": p.state, "version": p.version, "evidence": p.evidence, "response_ms": resp,
			"uptime_30d": map[string]any{"pct": 99.2 + float64(len(p.name)%7)/10, "coverage_pct": 98.5},
			"uptime_90d": map[string]any{"pct": 99.6, "coverage_pct": 84.0},
			"days":       days,
		})
	}
	writeJSON(w, map[string]any{"generated_at": now, "fresh": true,
		"overall": map[string]any{"state": "degraded", "uptime_30d": map[string]any{"pct": 99.71, "coverage_pct": 98.4}}, "projects": out})
}

func history(w http.ResponseWriter, r *http.Request) {
	rng := r.URL.Query().Get("range")
	span, bucket := map[string]time.Duration{"1h": time.Hour, "24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour}[rng],
		map[string]time.Duration{"1h": time.Minute, "24h": 5 * time.Minute, "7d": 30 * time.Minute, "30d": 2 * time.Hour}[rng]
	if span == 0 {
		http.Error(w, `{"error":"bad range"}`, http.StatusBadRequest)
		return
	}
	to := time.Now().UTC().Truncate(bucket)
	from := to.Add(-span)
	var points []map[string]any
	n := int(span / bucket)
	for i := 1; i <= n; i++ {
		t := from.Add(time.Duration(i) * bucket)
		x := float64(t.Unix()) / bucket.Seconds()
		if i > n/3 && i < n/3+n/30+1 { // a gap: the agent was down
			points = append(points, map[string]any{"t": t, "cpu_pct": nil, "mem_used_pct": nil, "coverage_pct": 0})
			continue
		}
		cpu := 16 + 6*math.Sin(x/7) + 3*math.Sin(x/2.3)
		if i == n*3/4 {
			cpu += 38
		}
		mem := 41 + 3*math.Sin(x/23)
		points = append(points, map[string]any{"t": t,
			"cpu_pct":      map[string]float64{"avg": cpu, "min": cpu * 0.55, "max": math.Min(100, cpu*1.6)},
			"mem_used_pct": map[string]float64{"avg": mem, "min": mem - 1.2, "max": mem + 1.5}, "coverage_pct": 100})
	}
	updates := []map[string]any{
		{"project": "lys", "started": to.Add(-span * 3 / 4), "ended": to.Add(-span*3/4 + 41*time.Second), "seconds": 41, "from": "v0.5.0", "to": "v0.5.1", "outcome": "ok"},
		{"project": "dibs", "started": to.Add(-span / 4), "ended": to.Add(-span/4 + 2*time.Minute), "seconds": 120, "from": "v0.2.9", "to": "v0.3.0", "outcome": "failed"},
		{"project": "mobasity-web", "started": to.Add(-2 * time.Minute), "ended": to, "seconds": 120, "from": "v1.7.2", "to": "v1.8.0", "outcome": "ok"},
	}
	writeJSON(w, map[string]any{"range": rng, "from": from, "to": to, "bucket_seconds": int(bucket.Seconds()), "generated_at": time.Now().UTC(),
		"points": points, "updates": updates})
}

type record struct {
	ID        int64           `json:"id"`
	TS        time.Time       `json:"ts"`
	Project   string          `json:"project"`
	Service   string          `json:"service"`
	Container string          `json:"container"`
	Version   string          `json:"version"`
	Stream    string          `json:"stream"`
	Level     string          `json:"level"`
	Message   string          `json:"message"`
	Attrs     json.RawMessage `json:"attrs"`
	Truncated bool            `json:"truncated"`
	Redacted  bool            `json:"redacted"`
}

var levelRank = map[string]int{"debug": 10, "info": 20, "warn": 30, "error": 40}

// fakeLogs makes the same lines on every call: one every 47 seconds over 30 days.
func fakeLogs() []record {
	msgs := []struct{ project, level, msg, attrs string }{
		{"lys", "info", "command completed", `{"command":"followage","duration_ms":83}`},
		{"dibs", "error", "health check failed", `{"err":"lobby store: connection refused"}`},
		{"lys", "warn", "Twitch: rate limited", `{"retry_after_s":2}`},
		{"marquee", "info", "request served", `{"path":"/","status":200,"duration_ms":12}`},
		{"lighthouse", "info", "check done", `{"projects":11,"changed":0}`},
		{"lys", "error", "Twitch: connection refused", `{"operation":"chat.send"}`},
		{"plop", "info", "upload stored", `{"bytes":48211}`},
		{"sonar", "debug", "snapshot written", `{"bytes":5123}`},
		{"dibs", "info", "<b>not bold</b> and <script>alert('escaped')</script>", `{"note":"Vista must show this as text"}`},
	}
	now := time.Now().UTC().Truncate(time.Second)
	var out []record
	for i := int64(0); i < 30*24*3600/47; i++ {
		m := msgs[i%int64(len(msgs))]
		out = append(out, record{ID: 100000 - i, TS: now.Add(-time.Duration(i*47) * time.Second), Project: m.project, Service: m.project,
			Container: m.project, Version: "v0.5.1", Stream: "stderr", Level: m.level, Message: m.msg, Attrs: json.RawMessage(m.attrs)})
	}
	return out
}

func logs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, _ := time.Parse(time.RFC3339, q.Get("from"))
	to, err := time.Parse(time.RFC3339, q.Get("to"))
	if err != nil {
		to = time.Now().UTC()
	}
	if from.IsZero() {
		from = to.Add(-time.Hour)
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 100
	}
	cursor, _ := strconv.ParseInt(q.Get("cursor"), 10, 64)
	text := strings.ToLower(q.Get("q"))
	var out []record
	more := false
	for _, rec := range fakeLogs() {
		if rec.TS.Before(from) || rec.TS.After(to) || (cursor > 0 && rec.ID >= cursor) {
			continue
		}
		if p := q["project"]; len(p) > 0 && p[0] != rec.Project {
			continue
		}
		if lvl := q.Get("level"); lvl != "" && levelRank[rec.Level] < levelRank[lvl] {
			continue
		}
		if v := q.Get("version"); v != "" && v != rec.Version {
			continue
		}
		if text != "" && !strings.Contains(strings.ToLower(rec.Message+string(rec.Attrs)), text) {
			continue
		}
		if len(out) == limit {
			more = true
			break
		}
		out = append(out, rec)
	}
	var next any
	if more {
		next = strconv.FormatInt(out[len(out)-1].ID, 10)
	}
	if out == nil {
		out = []record{}
	}
	writeJSON(w, map[string]any{"records": out, "next_cursor": next, "unknown_left_out": 0})
}

func logContext(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	all := fakeLogs()
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	var out []record
	for _, rec := range all {
		if rec.ID >= id-20 && rec.ID <= id+20 && rec.Project == projectOf(all, id) {
			out = append(out, rec)
		}
	}
	if out == nil {
		http.Error(w, `{"error":"no such record"}`, http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]any{"target": id, "records": out})
}

func projectOf(all []record, id int64) string {
	for _, r := range all {
		if r.ID == id {
			return r.Project
		}
	}
	return ""
}

func sources(w http.ResponseWriter, r *http.Request) {
	var out []map[string]any
	for _, p := range projects {
		out = append(out, map[string]any{"project": p.name, "service": p.name, "versions": []string{p.version}, "newest": time.Now().UTC(), "lines": 1000, "open_streams": 1})
	}
	writeJSON(w, map[string]any{"sources": out})
}

func gaps(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	writeJSON(w, map[string]any{"gaps": []map[string]any{
		{"kind": "collector_down", "container": nil, "from": now.Add(-5 * time.Hour), "to": now.Add(-5*time.Hour + 40*time.Second), "detail": "Blackbox's collector restarted", "noted_at": now.Add(-5 * time.Hour)},
	}})
}
