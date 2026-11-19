// Package dashboard arranges the sources' answers for Vista's pages. It
// judges nothing itself: states, conditions and counts are what Sonar and
// Blackbox report. What it adds is presentation (order, grouping) and the
// rule that an answer it doesn't have is shown as unknown.
package dashboard

import (
	"fmt"
	"slices"
	"sort"
	"strings"
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

// infrastructure are the projects Spark itself runs on, in the order they're
// listed. Sonar's group label decides for a project that carries it.
var infrastructure = []string{"sparkdb", "cove", "lighthouse", "cloudflared", "sonar", "blackbox"}

func isInfrastructure(name string, group *string) bool {
	if group != nil {
		return *group == "infrastructure"
	}
	for _, n := range infrastructure {
		if n == name {
			return true
		}
	}
	return false
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
	LastDeploy     *Deploy           `json:"last_deploy"`
	Lighthouse     *DeployState      `json:"lighthouse"` // nil when Lighthouse can't be read or doesn't know it

	// Sonar's and Blackbox's detail; nil or empty when they can't be read.
	StateSince      *time.Time             `json:"state_since"`
	StateSinceExact bool                   `json:"state_since_exact"`
	Checks          []source.MonitorCheck  `json:"checks"`
	Disk            *source.ProjectDiskUse `json:"disk"`
	Databases       []source.DatabaseNow   `json:"databases"`
	Traffic         []source.HostTraffic   `json:"traffic"`
	App             *source.AppProject     `json:"app"`
	Errors          *source.ErrorTotal     `json:"errors"`
}

// Container is one container, as Sonar reports it.
type Container struct {
	Project string `json:"project"`
	Service string `json:"service"`
	source.ContainerState
}

// DeployState is how Lighthouse reports a project's deploys.
type DeployState struct {
	State       string     `json:"state"` // ok, failing, broken, stopped, not_deployed
	Mode        string     `json:"mode"`  // branch or releases
	Version     *string    `json:"version"`
	DeployedAt  *time.Time `json:"deployed_at"`
	LastError   *string    `json:"last_error"`
	LastErrorAt *time.Time `json:"last_error_at"`
}

// Deploy is one deploy: Lighthouse's record when Lighthouse answers, or the
// container swap Sonar saw when it doesn't.
type Deploy struct {
	Source  string    `json:"source"`       // Lighthouse or Sonar
	ID      int64     `json:"id,omitempty"` // Lighthouse's id, for its steps
	Project string    `json:"project"`
	Started time.Time `json:"started"`
	Ended   time.Time `json:"ended"`
	Seconds int       `json:"seconds"`
	From    *string   `json:"from"` // Sonar only: the version it replaced
	To      *string   `json:"to"`
	Outcome string    `json:"outcome"` // ok, failed, timed out, rolled back
	Step    *string   `json:"step,omitempty"`
	Error   *string   `json:"error,omitempty"`
	Trigger string    `json:"trigger,omitempty"`
}

func deployFrom(u source.SonarUpdate) Deploy {
	return Deploy{Source: "Sonar", Project: u.Project, Started: u.Started, Ended: u.Ended, Seconds: u.Seconds, From: u.From, To: u.To, Outcome: u.Outcome}
}

// lighthouseOutcome names Lighthouse's statuses the way Vista shows outcomes.
var lighthouseOutcome = map[string]string{"succeeded": "ok", "failed": "failed", "rolled_back": "rolled back"}

func deployFromLighthouse(d source.LighthouseDeployment) Deploy {
	out := Deploy{Source: "Lighthouse", ID: d.ID, Project: d.Project, Started: d.StartedAt, Ended: d.FinishedAt,
		Seconds: int(d.FinishedAt.Sub(d.StartedAt).Seconds()), To: d.Version, Outcome: lighthouseOutcome[d.Status],
		Step: d.FailedStep, Error: d.Error, Trigger: d.Trigger}
	if out.Outcome == "" {
		out.Outcome = d.Status
	}
	if out.To == nil && d.SHA != nil && len(*d.SHA) >= 7 {
		sha := (*d.SHA)[:7]
		out.To = &sha
	}
	return out
}

// pickDeploys is Lighthouse's records when Lighthouse answered, otherwise
// the swaps Sonar saw; oldest first either way.
func pickDeploys(lh []source.LighthouseDeployment, lhKnown bool, sonar *source.SonarHost) []Deploy {
	out := []Deploy{}
	switch {
	case lhKnown:
		for _, d := range lh {
			out = append(out, deployFromLighthouse(d))
		}
		sort.SliceStable(out, func(i, j int) bool { return out[i].Started.Before(out[j].Started) })
	case sonar != nil:
		for _, u := range sonar.Updates {
			out = append(out, deployFrom(u))
		}
	}
	return out
}

// deployDetail describes a deploy in one line.
func deployDetail(d Deploy) string {
	var s string
	if d.Source == "Sonar" {
		s = versionChange(d.From, d.To)
	} else if d.To != nil {
		s = "Version " + *d.To + "."
	}
	errText := ""
	if d.Error != nil {
		errText = clip(*d.Error, 200)
	}
	if d.Step != nil && *d.Step != "" && !strings.HasPrefix(errText, *d.Step+":") {
		s += " Failed at " + *d.Step + "."
	}
	if errText != "" {
		s += " " + strings.TrimSuffix(errText, ".") + "."
	}
	return strings.TrimSpace(s)
}

// clip shortens s to n runes on one line.
func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
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

// Infra is one infrastructure project as Sonar reports it. Reported is false
// when Sonar's answer doesn't include it.
type Infra struct {
	Name     string  `json:"name"`
	State    Health  `json:"state"`
	Version  *string `json:"version"`
	Reported bool    `json:"reported"`
}

// Spark is the server's own health, apart from the apps, as Sonar's
// conditions report it. Reasons say why it's unknown.
type Spark struct {
	State      Health             `json:"state"`
	Reasons    []string           `json:"reasons"`
	Conditions []source.Condition `json:"conditions"`
	Collectors []source.Collector `json:"collectors"`
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
	Now           *source.HostNow    `json:"now"`
	Range         string             `json:"range"`
	BucketSeconds int                `json:"bucket_seconds"`
	From          *time.Time         `json:"from"`
	To            *time.Time         `json:"to"`
	Points        []source.HostPoint `json:"points"`
	Network       []source.NetSeries `json:"network"`
	Deploys       []Deploy           `json:"deploys"`
	DeploySource  string             `json:"deploy_source"` // Lighthouse, or Sonar when Lighthouse can't be read
}

// Errors is Blackbox's error counts for the range.
type Errors struct {
	Bucket      string               `json:"bucket"` // hour or day
	Buckets     []source.ErrorBucket `json:"buckets"`
	Totals      []source.ErrorTotal  `json:"totals"`
	Groups      []source.ErrorGroup  `json:"groups"` // nil when Blackbox couldn't group them
	TotalGroups int64                `json:"total_groups"`
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
	GeneratedAt    time.Time          `json:"generated_at"`
	Sources        []source.Status    `json:"sources"`
	Spark          Spark              `json:"spark"`
	Host           Host               `json:"host"`
	Totals         Totals             `json:"totals"`
	Projects       []Project          `json:"projects"`
	Attention      []Attention        `json:"attention"`
	Infrastructure []Infra            `json:"infrastructure"`
	Logs           *LogsSummary       `json:"logs"`
	RecentErrors   []source.LogRecord `json:"recent_errors"` // nil when Blackbox can't be read

	Containers   []Container            `json:"containers"` // nil when Sonar's detail can't be read
	Databases    *source.DatabaseServer `json:"databases"`
	Disk         *source.DiskUseNow     `json:"disk"`
	Traffic      []source.HostTraffic   `json:"traffic"` // nil when Sonar can't be read
	TrafficRange string                 `json:"traffic_range"`
	Errors       *Errors                `json:"errors"` // nil when Blackbox can't be read
}

// Inputs are the sources' answers for one Overview. A nil pointer is a
// source with no answer; its Status says why.
type Inputs struct {
	Now          time.Time
	Range        string
	Status       *source.SonarStatus
	StatusState  source.Status
	History      *source.SonarHost             // the chosen range, with the host now
	Day          *source.SonarHost             // the last 24 hours, for failed deploys
	Deploys      []source.LighthouseDeployment // the chosen range
	DeploysKnown bool
	DayDeploys   []source.LighthouseDeployment // the last 24 hours
	DayKnown     bool
	Lighthouse   []source.LighthouseProject
	LHKnown      bool
	RecentErrors []source.LogRecord
	ErrorsKnown  bool
	Gaps24h      []source.LogGap
	GapsKnown    bool
	Blackbox     *source.BlackboxStatus
	SourceStates []source.Status

	Projects    *source.SonarProjects
	Conditions  *source.SonarConditions
	CondCurrent bool // the conditions are Sonar's latest answer
	Databases   *source.SonarDatabases
	Disk        *source.SonarDisk
	Traffic     *source.SonarTraffic
	Apps        *source.SonarApps
	ErrorCounts *source.ErrorCounts
	ErrorGroups *source.ErrorGroups
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
	dayDeploys := pickDeploys(in.DayDeploys, in.DayKnown, in.Day)
	lastDeploy := map[string]Deploy{}
	for _, d := range dayDeploys {
		lastDeploy[d.Project] = d // oldest first, so the newest wins
	}
	lhByKey := map[string]source.LighthouseProject{}
	if in.LHKnown {
		for _, p := range in.Lighthouse {
			lhByKey[p.Key()] = p
		}
	}
	detail := map[string]source.MonitorProject{}
	if in.Projects != nil {
		for _, p := range in.Projects.Projects {
			detail[p.Project] = p
		}
	}
	if in.Status != nil {
		for _, p := range in.Status.Projects {
			d, hasDetail := detail[p.Project]
			row := Project{
				Name:           p.Project,
				Infrastructure: isInfrastructure(p.Project, d.Group),
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
			if d, ok := lastDeploy[p.Project]; ok {
				row.LastDeploy = &d
			}
			if lp, ok := lhByKey[p.Project]; ok {
				row.Lighthouse = &DeployState{State: lp.State, Mode: lp.DeployMode, Version: lp.DeployedVersion,
					DeployedAt: lp.DeployedAt, LastError: lp.LastError, LastErrorAt: lp.LastErrorAt}
			}
			if hasDetail {
				row.StateSince, row.StateSinceExact, row.Checks = d.StateSince, d.StateSinceExact, d.Checks
			}
			addDetail(&row, in)
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
	o.Host.DeploySource = "Sonar"
	if in.DeploysKnown {
		o.Host.DeploySource = "Lighthouse"
	}
	o.Infrastructure = buildInfra(o.Projects)
	o.Containers = buildContainers(in.Projects)
	if in.Databases != nil {
		o.Databases = in.Databases.Now
	}
	if in.Disk != nil {
		o.Disk = in.Disk.Now
	}
	o.TrafficRange = source.TrafficRange(in.Range)
	if in.Traffic != nil {
		o.Traffic = in.Traffic.Hosts
		if o.Traffic == nil {
			o.Traffic = []source.HostTraffic{}
		}
	}
	o.Errors = buildErrors(in.ErrorCounts, in.ErrorGroups)
	if in.ErrorsKnown {
		o.RecentErrors = in.RecentErrors
		if o.RecentErrors == nil {
			o.RecentErrors = []source.LogRecord{}
		}
	}
	if in.Blackbox != nil {
		b := in.Blackbox
		o.Logs = &LogsSummary{StoredBytes: b.StoredBytes, MaxBytes: b.MaxBytes, Oldest: b.Oldest, LagSeconds: b.LagSeconds,
			StreamsOpen: b.StreamsOpen, StreamsRefused: b.StreamsRefused, Gaps24h: b.Gaps24h, Version: b.Version}
	}
	o.Spark = buildSpark(in)
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

func buildHost(in Inputs) Host {
	h := Host{Range: in.Range, Points: []source.HostPoint{}, Network: []source.NetSeries{}}
	h.Deploys = pickDeploys(in.Deploys, in.DeploysKnown, in.History)
	if in.History == nil {
		return h
	}
	if n := in.History.Now; n != nil {
		h.Now = n
		if n.CPUPct != nil {
			h.CPU = &Reading{Pct: *n.CPUPct, At: n.ReadAt}
		}
		if n.MemoryTotal > 0 {
			h.Memory = &Reading{Pct: n.MemoryUsedPct, At: n.ReadAt}
		}
	}
	from, to := in.History.From, in.History.To
	h.From, h.To = &from, &to
	h.BucketSeconds = in.History.BucketSeconds
	if in.History.Points != nil {
		h.Points = in.History.Points
	}
	if in.History.Network != nil {
		h.Network = in.History.Network
	}
	return h
}

// addDetail attaches what Sonar and Blackbox report for one project.
func addDetail(row *Project, in Inputs) {
	name := row.Name
	if in.Disk != nil && in.Disk.Now != nil {
		for i := range in.Disk.Now.Projects {
			if in.Disk.Now.Projects[i].Project == name {
				row.Disk = &in.Disk.Now.Projects[i]
			}
		}
	}
	if in.Databases != nil && in.Databases.Now != nil {
		for _, d := range in.Databases.Now.Databases {
			if d.Project != nil && *d.Project == name {
				row.Databases = append(row.Databases, d)
			}
		}
	}
	if in.Traffic != nil {
		for _, h := range in.Traffic.Hosts {
			if h.Project != nil && *h.Project == name {
				h.Points = nil // the page shows totals only
				row.Traffic = append(row.Traffic, h)
			}
		}
	}
	if in.Apps != nil {
		for i := range in.Apps.Projects {
			if in.Apps.Projects[i].Project == name {
				row.App = &in.Apps.Projects[i]
			}
		}
	}
	if in.ErrorCounts != nil {
		row.Errors = &source.ErrorTotal{}
		for _, t := range in.ErrorCounts.Totals {
			if t.Project != nil && *t.Project == name {
				row.Errors = &t
			}
		}
	}
}

// buildContainers lists every container Sonar reports, by project and name.
func buildContainers(p *source.SonarProjects) []Container {
	if p == nil {
		return nil
	}
	out := []Container{}
	for _, proj := range p.Projects {
		for _, svc := range proj.Services {
			for _, c := range svc.Containers {
				out = append(out, Container{Project: proj.Project, Service: svc.Service, ContainerState: c})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Project != out[j].Project {
			return out[i].Project < out[j].Project
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// buildErrors is Blackbox's counts and repeated errors, nil without counts.
func buildErrors(counts *source.ErrorCounts, groups *source.ErrorGroups) *Errors {
	if counts == nil {
		return nil
	}
	e := &Errors{Bucket: counts.Bucket, Buckets: counts.Buckets, Totals: counts.Totals}
	if e.Buckets == nil {
		e.Buckets = []source.ErrorBucket{}
	}
	if e.Totals == nil {
		e.Totals = []source.ErrorTotal{}
	}
	if groups != nil {
		e.Groups, e.TotalGroups = groups.Groups, groups.TotalGroups
		if e.Groups == nil {
			e.Groups = []source.ErrorGroup{}
		}
	}
	return e
}

// buildInfra lists every infrastructure project with the state Sonar
// reports for it, or as not reported.
func buildInfra(projects []Project) []Infra {
	byName := map[string]Project{}
	for _, p := range projects {
		byName[p.Name] = p
	}
	names := append([]string{}, infrastructure...)
	for _, p := range projects {
		if p.Infrastructure && !slices.Contains(names, p.Name) {
			names = append(names, p.Name)
		}
	}
	out := make([]Infra, 0, len(names))
	for _, name := range names {
		i := Infra{Name: name, State: Unknown}
		if p, ok := byName[name]; ok {
			i.State, i.Version, i.Reported = p.State, p.Version, true
		}
		out = append(out, i)
	}
	return out
}

// sparkStates maps Sonar's word for Spark onto Vista's.
var sparkStates = map[string]Health{"ok": Healthy, "warning": Degraded, "critical": Down}

// buildSpark is Spark's state as Sonar's conditions report it, or unknown
// with the reason when they can't be trusted. Vista doesn't guess it.
func buildSpark(in Inputs) Spark {
	s := Spark{State: Unknown, Reasons: []string{}, Conditions: []source.Condition{}, Collectors: []source.Collector{}}
	c := in.Conditions
	if c != nil {
		if c.Conditions != nil {
			s.Conditions = c.Conditions
		}
		if c.Collectors != nil {
			s.Collectors = c.Collectors
		}
	}
	switch {
	case c == nil || !in.CondCurrent:
		s.Reasons = append(s.Reasons, "Sonar can't be read.")
	case !c.Fresh:
		s.Reasons = append(s.Reasons, "Sonar's measurements are out of date: its agent may be stopped.")
	default:
		if st, ok := sparkStates[c.State]; ok {
			s.State = st
		} else {
			s.Reasons = append(s.Reasons, "Sonar has no measurement of the server yet.")
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
	for _, d := range pickDeploys(in.DayDeploys, in.DayKnown, in.Day) {
		if d.Outcome == "ok" {
			continue
		}
		at := d.Started
		add(Attention{Level: "warn", Project: d.Project, Title: fmt.Sprintf("Deploy of %s %s", d.Project, d.Outcome),
			Detail: deployDetail(d), Source: d.Source, At: &at, Link: "#/activity?project=" + d.Project})
	}
	if in.LHKnown {
		for _, p := range in.Lighthouse {
			if p.Kind == "publish" {
				continue
			}
			a := Attention{Project: p.Key(), Source: "Lighthouse", At: p.LastErrorAt, Link: "#/projects/" + p.Key()}
			if p.LastError != nil {
				a.Detail = clip(*p.LastError, 200)
			}
			switch p.State {
			case "broken":
				a.Level, a.Title = "warn", "Lighthouse reports "+p.Key()+" as broken"
			case "failing":
				a.Level, a.Title = "warn", "Lighthouse reports "+p.Key()+"'s deploys failing"
			case "stopped":
				a.Level, a.Title, a.At = "info", p.Key()+" is stopped in Lighthouse", nil
			default:
				continue
			}
			add(a)
		}
	}
	if in.Conditions != nil && in.CondCurrent && in.Conditions.Fresh {
		for _, c := range in.Conditions.Conditions {
			if c.ID == "project" {
				continue // each project's own line says it
			}
			since := c.Since
			level := "warn"
			if c.Level == "critical" {
				level = "down"
			}
			add(Attention{Level: level, Title: c.Message, Source: "Sonar", At: &since, Link: "#/spark"})
		}
		for _, a := range in.Conditions.Attention {
			add(Attention{Level: "info", Title: "Sonar: " + a, Source: "Sonar", Link: "#/spark"})
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
