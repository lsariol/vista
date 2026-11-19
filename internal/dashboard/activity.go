package dashboard

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lsariol/vista/internal/source"
)

// Event is one entry on the Activity timeline.
type Event struct {
	At       time.Time  `json:"at"`
	End      *time.Time `json:"end,omitempty"`
	Kind     string     `json:"kind"` // deploy, deploy_failed, state, log_gap
	Project  string     `json:"project,omitempty"`
	Title    string     `json:"title"`
	Detail   string     `json:"detail,omitempty"`
	Source   string     `json:"source"`
	Version  *string    `json:"version,omitempty"`
	DeployID int64      `json:"deploy_id,omitempty"` // Lighthouse's id, for its steps
	Link     string     `json:"link,omitempty"`
}

// Activity is the timeline page.
type Activity struct {
	GeneratedAt time.Time       `json:"generated_at"`
	Range       string          `json:"range"`
	Events      []Event         `json:"events"`
	Projects    []string        `json:"projects"` // every project with an event, for the filter
	Errors      *Errors         `json:"errors"`   // nil when Blackbox can't be read
	Notes       []string        `json:"notes"`
	Sources     []source.Status `json:"sources"`
}

// ActivityInputs are the answers the timeline is built from.
type ActivityInputs struct {
	Now          time.Time
	Range        string
	History      *source.SonarHost
	Deploys      []source.LighthouseDeployment
	DeploysKnown bool
	Gaps         []source.LogGap
	States       *source.SonarStates
	ErrorCounts  *source.ErrorCounts
	SourceStates []source.Status
}

// BuildActivity assembles the timeline, newest first.
func BuildActivity(in ActivityInputs) Activity {
	a := Activity{GeneratedAt: in.Now.UTC(), Range: in.Range, Events: []Event{}, Sources: mergeSources(in.SourceStates),
		Notes: []string{
			"Health changes are Sonar's: each stretch a project was down or degraded, with what wasn't up.",
			"Error counts are Blackbox's warn and error lines. Container restarts are counted on each container, on the Overview.",
			"Events close together are leads to investigate, not proof that one caused the other.",
		}}

	if in.DeploysKnown {
		a.Notes = append([]string{"Deploys are Lighthouse's records."}, a.Notes...)
	} else {
		a.Notes = append([]string{"Deploys are the container swaps Sonar saw: Lighthouse can't be read. Its deploy lines are in the Logs page."}, a.Notes...)
	}
	for _, d := range pickDeploys(in.Deploys, in.DeploysKnown, in.History) {
		e := Event{At: d.Started, Project: d.Project, Source: d.Source, Version: d.To, DeployID: d.ID,
			Link: "#/logs?project=" + d.Project + "&from=" + d.Started.Add(-5*time.Minute).Format(time.RFC3339) + "&to=" + d.Ended.Add(10*time.Minute).Format(time.RFC3339)}
		end := d.Ended
		e.End = &end
		if d.Outcome == "ok" {
			e.Kind = "deploy"
			e.Title = d.Project + " deployed"
			if d.To != nil {
				e.Title += " " + *d.To
			}
		} else {
			e.Kind = "deploy_failed"
			e.Title = d.Project + " deploy " + d.Outcome
		}
		e.Detail = strings.TrimSpace(deployDetail(d) + fmt.Sprintf(" Took %d s.", d.Seconds))
		a.Events = append(a.Events, e)
	}

	a.Errors = buildErrors(in.ErrorCounts, nil)
	if in.States != nil {
		for _, p := range in.States.Projects {
			for _, sp := range p.Spans {
				if sp.State != "down" && sp.State != "degraded" {
					continue
				}
				end := sp.End
				a.Events = append(a.Events, Event{At: sp.Start, End: &end, Kind: "state", Project: p.Project,
					Title: p.Project + " " + sp.State, Detail: spanDetail(sp), Source: "Sonar",
					Link: "#/logs?project=" + p.Project + "&from=" + sp.Start.Add(-5*time.Minute).Format(time.RFC3339) + "&to=" + sp.End.Add(5*time.Minute).Format(time.RFC3339)})
			}
		}
	}

	for _, g := range in.Gaps {
		e := Event{At: g.From, Kind: "log_gap", Title: "Possible log gap: " + gapKind(g.Kind), Detail: g.Detail, Source: "Blackbox"}
		if g.Container != nil {
			e.Project = *g.Container
			e.Title += " (" + *g.Container + ")"
		}
		to := g.To
		e.End = &to
		a.Events = append(a.Events, e)
	}

	sort.SliceStable(a.Events, func(i, j int) bool { return a.Events[i].At.After(a.Events[j].At) })
	seen := map[string]bool{}
	a.Projects = []string{}
	for _, e := range a.Events {
		if e.Project != "" && !seen[e.Project] {
			seen[e.Project] = true
			a.Projects = append(a.Projects, e.Project)
		}
	}
	sort.Strings(a.Projects)
	return a
}

// spanDetail says how long a span lasted and what wasn't up during it.
func spanDetail(sp source.StateSpan) string {
	parts := []string{"For " + minutes(int(sp.End.Sub(sp.Start).Minutes())) + "."}
	for _, c := range sp.Services {
		parts = append(parts, fmt.Sprintf("Service %s %s for %s.", c.Service, c.State, minutes(c.Minutes)))
	}
	for _, c := range sp.Checks {
		t := fmt.Sprintf("Check of %s: %d failed, %d slow", c.URL, c.Failures, c.Slow)
		if c.LastError != nil && *c.LastError != "" {
			t += ", last error: " + clip(*c.LastError, 160)
		}
		parts = append(parts, strings.TrimSuffix(t, ".")+".")
	}
	return strings.Join(parts, " ")
}

func minutes(m int) string {
	switch {
	case m < 1:
		return "under a minute"
	case m < 90:
		return plural(m, "minute", "minutes")
	case m < 48*60:
		return fmt.Sprintf("%.1f hours", float64(m)/60)
	}
	return fmt.Sprintf("%.1f days", float64(m)/1440)
}

func gapKind(k string) string {
	switch k {
	case "container_removed":
		return "container removed before it was read"
	case "collector_down":
		return "collector wasn't running"
	case "log_damaged":
		return "Docker's log file was damaged"
	}
	return strings.ReplaceAll(k, "_", " ")
}

// IncidentDay is one day a project was down or degraded.
type IncidentDay struct {
	Project         string   `json:"project"`
	Date            string   `json:"date"`
	State           Health   `json:"state"`
	DownMinutes     int      `json:"down_minutes"`
	DegradedMinutes int      `json:"degraded_minutes"`
	ObservedMinutes int      `json:"observed_minutes"`
	CoveragePct     float64  `json:"coverage_pct"`
	UptimePct       *float64 `json:"uptime_pct"`
}

// Incident is one stretch a project was down or degraded, from Sonar's spans.
type Incident struct {
	Project string    `json:"project"`
	State   Health    `json:"state"`
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
	Detail  string    `json:"detail"`
}

// Availability is one project's uptime.
type Availability struct {
	Project    string        `json:"project"`
	Monitoring string        `json:"monitoring"`
	Uptime30d  source.Uptime `json:"uptime_30d"`
	Uptime90d  source.Uptime `json:"uptime_90d"`
}

// Incidents is the Incidents page.
type Incidents struct {
	GeneratedAt  time.Time       `json:"generated_at"`
	Incidents    []Incident      `json:"incidents"` // nil when Sonar's spans can't be read
	Days         []IncidentDay   `json:"days"`
	Availability []Availability  `json:"availability"`
	Overall      *source.Uptime  `json:"overall_30d"`
	Notes        []string        `json:"notes"`
	Sources      []source.Status `json:"sources"`
}

// BuildIncidents lists the days with downtime or degradation, newest first.
func BuildIncidents(now time.Time, st *source.SonarStatus, spans *source.SonarStates, states []source.Status) Incidents {
	inc := Incidents{GeneratedAt: now.UTC(), Days: []IncidentDay{}, Availability: []Availability{}, Sources: mergeSources(states),
		Notes: []string{
			"Incidents are Sonar's state changes over 30 days, with when each began and ended; days with trouble go back 90 days.",
			"Degraded and deploy minutes count as available. Unobserved time is left out of uptime and shown as coverage instead.",
		}}
	if spans != nil {
		inc.Incidents = []Incident{}
		for _, p := range spans.Projects {
			for _, sp := range p.Spans {
				if sp.State == "down" || sp.State == "degraded" {
					inc.Incidents = append(inc.Incidents, Incident{Project: p.Project, State: fromSonar(sp.State), Start: sp.Start, End: sp.End, Detail: spanDetail(sp)})
				}
			}
		}
		sort.SliceStable(inc.Incidents, func(i, j int) bool { return inc.Incidents[i].Start.After(inc.Incidents[j].Start) })
	}
	if st == nil {
		return inc
	}
	overall := st.Overall.Uptime30d
	inc.Overall = &overall
	for _, p := range st.Projects {
		inc.Availability = append(inc.Availability, Availability{Project: p.Project, Monitoring: monitoring(p.Evidence), Uptime30d: p.Uptime30d, Uptime90d: p.Uptime90d})
		for _, d := range p.Days {
			if d.DownMinutes == 0 && d.DegradedMinutes == 0 {
				continue
			}
			state := Degraded
			if d.DownMinutes > 0 {
				state = Down
			}
			inc.Days = append(inc.Days, IncidentDay{Project: p.Project, Date: d.Date, State: state, DownMinutes: d.DownMinutes,
				DegradedMinutes: d.DegradedMinutes, ObservedMinutes: d.ObservedMinutes, CoveragePct: d.CoveragePct, UptimePct: d.UptimePct})
		}
	}
	sort.SliceStable(inc.Days, func(i, j int) bool {
		if inc.Days[i].Date != inc.Days[j].Date {
			return inc.Days[i].Date > inc.Days[j].Date
		}
		return inc.Days[i].Project < inc.Days[j].Project
	})
	return inc
}
