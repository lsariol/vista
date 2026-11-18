// Package dashboard turns the sources' answers into what Vista's pages show.
// Everything here is a pure function of those answers, so the rules (what is
// unknown, what needs attention) live in one tested place.
package dashboard

import (
	"fmt"
	"sort"
	"time"

	"github.com/lsariol/vista/internal/source"
)

// Health is a state as Vista shows it.
type Health string

const (
	Healthy  Health = "healthy"
	Updating Health = "updating"
	Degraded Health = "degraded"
	Down     Health = "down"
	Unknown  Health = "unknown"
)

// fromSonar maps Sonar's state names to Vista's.
func fromSonar(s string) Health {
	switch s {
	case "operational":
		return Healthy
	case "updating":
		return Updating
	case "degraded":
		return Degraded
	case "down":
		return Down
	}
	return Unknown
}

// infrastructure are the projects Spark itself runs on. Their trouble is
// Spark's trouble, shown apart from the apps.
var infrastructure = map[string]bool{
	"sparkdb": true, "cove": true, "lighthouse": true, "cloudflared": true, "sonar": true, "blackbox": true,
}

// Project is one project's row.
type Project struct {
	Name           string            `json:"name"`
	Infrastructure bool              `json:"infrastructure"`
	State          Health            `json:"state"`
	LastKnown      Health            `json:"last_known,omitempty"` // when State is unknown because Sonar's answer is old
	Version        *string           `json:"version"`
	Monitoring     string            `json:"monitoring"` // "outside check" or "containers only"
	ResponseMs     *int              `json:"response_ms"`
	Uptime30d      source.Uptime     `json:"uptime_30d"`
	Uptime90d      source.Uptime     `json:"uptime_90d"`
	Days           []source.SonarDay `json:"days"`
	Errors24h      *Count            `json:"errors_24h"` // nil when Blackbox can't be read
	LastDeploy     *Deploy           `json:"last_deploy"`
}

// Count is a number that may be a lower bound.
type Count struct {
	N       int  `json:"n"`
	AtLeast bool `json:"at_least"`
}

// Deploy is one deploy as Sonar saw it.
type Deploy struct {
	Project string    `json:"project"`
	Started time.Time `json:"started"`
	Ended   time.Time `json:"ended"`
	Seconds int       `json:"seconds"`
	From    *string   `json:"from"`
	To      *string   `json:"to"`
	Outcome string    `json:"outcome"`
}

func deployFrom(u source.SonarUpdate) Deploy {
	return Deploy{Project: u.Project, Started: u.Started, Ended: u.Ended, Seconds: u.Seconds, From: u.From, To: u.To, Outcome: u.Outcome}
}

// Attention is one thing that needs a look, most serious first.
type Attention struct {
	Level   string     `json:"level"` // down, degraded, unknown, warn, info
	Project string     `json:"project,omitempty"`
	Title   string     `json:"title"`
	Detail  string     `json:"detail,omitempty"`
	Source  string     `json:"source"`
	At      *time.Time `json:"at,omitempty"`
	Link    string     `json:"link,omitempty"` // a page in Vista
}

var levelRank = map[string]int{"down": 0, "degraded": 1, "unknown": 2, "warn": 3, "info": 4}

// Infra is one infrastructure service's own health check.
type Infra struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	State   Health `json:"state"`
	Version string `json:"version,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

// Spark is the server's own health, apart from the apps.
type Spark struct {
	State   Health   `json:"state"`
	Reasons []string `json:"reasons"`
}

// Reading is one current measurement.
type Reading struct {
	Pct float64   `json:"pct"`
	At  time.Time `json:"at"`
}

// Host is the server's measurements.
type Host struct {
	CPU           *Reading           `json:"cpu"`
	Memory        *Reading           `json:"memory"`
	Range         string             `json:"range"`
	BucketSeconds int                `json:"bucket_seconds"`
	From          *time.Time         `json:"from"`
	To            *time.Time         `json:"to"`
	Points        []source.HostPoint `json:"points"`
	Deploys       []Deploy           `json:"deploys"`
	NotCollected  []string           `json:"not_collected"`
}

// notCollected are measurements the idea calls for that no source serves yet.
var notCollected = []string{
	"Disk space",
	"Network traffic",
	"Load average",
	"Per-project CPU, memory and network",
	"Container restarts",
}

// Totals counts projects by state.
type Totals struct {
	Total    int `json:"total"`
	Healthy  int `json:"healthy"`
	Updating int `json:"updating"`
	Degraded int `json:"degraded"`
	Down     int `json:"down"`
	Unknown  int `json:"unknown"`
}

// LogsSummary is Blackbox's collection state.
type LogsSummary struct {
	StoredBytes    int64      `json:"stored_bytes"`
	MaxBytes       int64      `json:"max_bytes"`
	Oldest         *time.Time `json:"oldest"`
	LagSeconds     float64    `json:"lag_seconds"`
	StreamsOpen    int        `json:"streams_open"`
	StreamsRefused int        `json:"streams_refused"`
	Gaps24h        int64      `json:"gaps_24h"`
	Version        *string    `json:"version"`
}

// Overview is the Overview page, and the base of Projects, Spark and Sources.
type Overview struct {
	GeneratedAt    time.Time       `json:"generated_at"`
	Sources        []source.Status `json:"sources"`
	Spark          Spark           `json:"spark"`
	Host           Host            `json:"host"`
	Totals         Totals          `json:"totals"`
	Projects       []Project       `json:"projects"`
	Attention      []Attention     `json:"attention"`
	Infrastructure []Infra         `json:"infrastructure"`
	Logs           *LogsSummary    `json:"logs"`
}

// Inputs are the sources' answers for one Overview. A nil pointer is a
// source with no answer; its Status says why.
type Inputs struct {
	Now          time.Time
	Range        string
	Status       *source.SonarStatus
	StatusState  source.Status
	Current      *source.SonarHistory // the last hour, for current readings
	History      *source.SonarHistory // the chosen range
	Day          *source.SonarHistory // the last 24 hours, for failed deploys
	Errors24h    *source.ErrorWindow
	Gaps24h      []source.LogGap
	GapsKnown    bool
	Blackbox     *source.BlackboxStatus
	Probes       []source.Probe
	ProbesKnown  bool
	SourceStates []source.Status
}

// BuildOverview assembles the Overview.
func BuildOverview(in Inputs) Overview {
	o := Overview{
		GeneratedAt: in.Now.UTC(),
		Sources:     mergeSources(in.SourceStates),
		Projects:    []Project{},
		Attention:   []Attention{},
	}

	sonarCurrent := in.Status != nil && in.StatusState.State == source.OK
	errorsBy := countErrors(in.Errors24h)
	lastDeploy := map[string]Deploy{}
	if in.Day != nil {
		for _, u := range in.Day.Updates {
			lastDeploy[u.Project] = deployFrom(u) // oldest first, so the newest wins
		}
	}
	if in.Status != nil {
		for _, p := range in.Status.Projects {
			row := Project{
				Name:           p.Project,
				Infrastructure: infrastructure[p.Project],
				State:          fromSonar(p.State),
				Version:        p.Version,
				Monitoring:     monitoring(p.Evidence),
				ResponseMs:     p.ResponseMs,
				Uptime30d:      p.Uptime30d,
				Uptime90d:      p.Uptime90d,
				Days:           p.Days,
			}
			if !sonarCurrent {
				row.LastKnown, row.State = row.State, Unknown
			}
			if in.Errors24h != nil {
				c := errorsBy[p.Project]
				row.Errors24h = &Count{N: c, AtLeast: in.Errors24h.Truncated}
			}
			if d, ok := lastDeploy[p.Project]; ok {
				row.LastDeploy = &d
			}
			o.Projects = append(o.Projects, row)
		}
	}
	sort.Slice(o.Projects, func(i, j int) bool {
		a, b := o.Projects[i], o.Projects[j]
		if ra, rb := stateRank(a.State), stateRank(b.State); ra != rb {
			return ra < rb
		}
		return a.Name < b.Name
	})
	for _, p := range o.Projects {
		o.Totals.Total++
		switch p.State {
		case Healthy:
			o.Totals.Healthy++
		case Updating:
			o.Totals.Updating++
		case Degraded:
			o.Totals.Degraded++
		case Down:
			o.Totals.Down++
		default:
			o.Totals.Unknown++
		}
	}

	o.Host = buildHost(in)
	o.Infrastructure = buildInfra(in)
	if in.Blackbox != nil {
		b := in.Blackbox
		o.Logs = &LogsSummary{StoredBytes: b.StoredBytes, MaxBytes: b.MaxBytes, Oldest: b.Oldest, LagSeconds: b.LagSeconds,
			StreamsOpen: b.StreamsOpen, StreamsRefused: b.StreamsRefused, Gaps24h: b.Gaps24h, Version: b.Version}
	}
	o.Spark = buildSpark(in, o.Projects, o.Infrastructure)
	o.Attention = buildAttention(in, o)
	return o
}

func monitoring(evidence string) string {
	if evidence == "checked" {
		return "outside check"
	}
	return "containers only"
}

// stateRank orders states worst first, for sorting.
func stateRank(h Health) int {
	switch h {
	case Down:
		return 0
	case Degraded:
		return 1
	case Unknown:
		return 2
	case Updating:
		return 3
	}
	return 4
}

// countErrors counts error lines by project. Lines without a project count
// under their container's name.
func countErrors(w *source.ErrorWindow) map[string]int {
	out := map[string]int{}
	if w == nil {
		return out
	}
	for _, r := range w.Records {
		out[recordProject(r)]++
	}
	return out
}

func recordProject(r source.LogRecord) string {
	if r.Project != nil && *r.Project != "" {
		return *r.Project
	}
	return r.Container
}

func buildHost(in Inputs) Host {
	h := Host{Range: in.Range, Points: []source.HostPoint{}, Deploys: []Deploy{}, NotCollected: notCollected}
	if in.Current != nil {
		for i := len(in.Current.Points) - 1; i >= 0; i-- {
			p := in.Current.Points[i]
			if h.CPU == nil && p.CPUPct != nil {
				h.CPU = &Reading{Pct: p.CPUPct.Avg, At: p.T}
			}
			if h.Memory == nil && p.MemUsedPct != nil {
				h.Memory = &Reading{Pct: p.MemUsedPct.Avg, At: p.T}
			}
		}
	}
	if in.History != nil {
		from, to := in.History.From, in.History.To
		h.From, h.To = &from, &to
		h.BucketSeconds = in.History.BucketSeconds
		h.Points = in.History.Points
		for _, u := range in.History.Updates {
			h.Deploys = append(h.Deploys, deployFrom(u))
		}
	}
	return h
}

func buildInfra(in Inputs) []Infra {
	out := []Infra{}
	for _, p := range in.Probes {
		i := Infra{ID: p.ID, Name: p.Name, Version: p.Result.Version, Detail: p.Result.Detail, State: Healthy}
		if !p.Result.Healthy {
			i.State = Down
		}
		if !in.ProbesKnown {
			i.State = Unknown
		}
		out = append(out, i)
	}
	return out
}

func buildSpark(in Inputs, projects []Project, infra []Infra) Spark {
	s := Spark{State: Healthy, Reasons: []string{}}
	switch {
	case in.Status == nil || in.StatusState.State != source.OK:
		s.State = Unknown
		s.Reasons = append(s.Reasons, "Sonar can't be read, so Spark's state isn't known.")
	case !in.Status.Fresh:
		s.State = Unknown
		s.Reasons = append(s.Reasons, "Sonar's measurements are out of date: its agent may be stopped.")
	}
	for _, i := range infra {
		if i.State == Down {
			s.Reasons = append(s.Reasons, fmt.Sprintf("%s's health check failed (%s).", i.Name, i.Detail))
			if s.State == Healthy {
				s.State = Degraded
			}
		}
	}
	for _, p := range projects {
		if p.Infrastructure && (p.State == Down || p.State == Degraded) {
			s.Reasons = append(s.Reasons, fmt.Sprintf("%s is %s.", p.Name, p.State))
			if s.State == Healthy {
				s.State = Degraded
			}
		}
	}
	return s
}

func buildAttention(in Inputs, o Overview) []Attention {
	var out []Attention
	add := func(a Attention) { out = append(out, a) }

	for _, src := range o.Sources {
		switch src.State {
		case source.Unavailable:
			add(Attention{Level: "warn", Title: src.Name + " can't be read", Detail: src.Detail, Source: "Vista", Link: "#/sources"})
		case source.Stale:
			add(Attention{Level: "warn", Title: src.Name + " isn't answering; older data is shown", Detail: src.Detail, Source: "Vista", At: src.LastOK, Link: "#/sources"})
		}
	}
	if in.Status != nil && in.StatusState.State == source.OK && !in.Status.Fresh {
		add(Attention{Level: "unknown", Title: "Sonar's measurements are out of date", Detail: "Every live state is unknown until its agent reports again.", Source: "Sonar", Link: "#/spark"})
	}
	for _, p := range o.Projects {
		link := "#/projects/" + p.Name
		switch {
		case p.State == Down || p.State == Degraded:
			add(Attention{Level: string(p.State), Project: p.Name, Title: p.Name + " is " + string(p.State),
				Detail: "Sonar reports it " + string(p.State) + " (" + p.Monitoring + ").", Source: "Sonar", Link: link})
		case p.State == Unknown && p.LastKnown == "":
			add(Attention{Level: "unknown", Project: p.Name, Title: p.Name + "'s state is unknown",
				Detail: "Sonar has no current measurement for it.", Source: "Sonar", Link: link})
		}
	}
	for _, i := range o.Infrastructure {
		if i.State == Down {
			add(Attention{Level: "degraded", Title: i.Name + "'s health check failed", Detail: i.Detail, Source: "Vista", Link: "#/spark"})
		}
	}
	if in.Day != nil {
		for _, u := range in.Day.Updates {
			if u.Outcome == "ok" {
				continue
			}
			at := u.Started
			add(Attention{Level: "warn", Project: u.Project, Title: fmt.Sprintf("Deploy of %s %s", u.Project, u.Outcome),
				Detail: versionChange(u.From, u.To), Source: "Sonar", At: &at, Link: "#/activity?project=" + u.Project})
		}
	}
	if in.Blackbox != nil {
		for _, a := range in.Blackbox.Attention {
			add(Attention{Level: "warn", Title: "Log collection: " + a, Source: "Blackbox", Link: "#/sources"})
		}
	}
	if in.GapsKnown && len(in.Gaps24h) > 0 {
		add(Attention{Level: "info", Title: plural(len(in.Gaps24h), "possible log gap", "possible log gaps") + " in the last 24 hours",
			Detail: "Lines may be missing while these lasted.", Source: "Blackbox", Link: "#/activity?kind=log_gap"})
	}
	for _, p := range o.Projects {
		if p.Errors24h != nil && p.Errors24h.N > 0 {
			n := plural(p.Errors24h.N, "error line", "error lines")
			if p.Errors24h.AtLeast {
				n = "At least " + n
			}
			add(Attention{Level: "info", Project: p.Name, Title: n + " from " + p.Name + " in 24 hours", Source: "Blackbox",
				Link: "#/logs?project=" + p.Name + "&level=error&range=24h"})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return levelRank[out[i].Level] < levelRank[out[j].Level] })
	if out == nil {
		out = []Attention{}
	}
	return out
}

// mergeSources keeps one status per source: the worst of its caches.
func mergeSources(all []source.Status) []source.Status {
	rank := map[source.State]int{source.Unavailable: 0, source.Stale: 1, source.NotConnected: 2, source.OK: 3}
	byID := map[string]source.Status{}
	var order []string
	for _, s := range all {
		cur, seen := byID[s.ID]
		if !seen {
			order = append(order, s.ID)
			byID[s.ID] = s
			continue
		}
		if rank[s.State] < rank[cur.State] {
			byID[s.ID] = s
		}
	}
	out := make([]source.Status, 0, len(order))
	for _, id := range order {
		out = append(out, byID[id])
	}
	return out
}

func versionChange(from, to *string) string {
	f, t := "an unknown version", "an unknown version"
	if from != nil {
		f = *from
	}
	if to != nil {
		t = *to
	}
	if from == nil && to != nil {
		return "First deploy, " + t + "."
	}
	return "From " + f + " to " + t + "."
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
