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
	At      time.Time  `json:"at"`
	End     *time.Time `json:"end,omitempty"`
	Kind    string     `json:"kind"` // deploy, deploy_failed, log_gap
	Project string     `json:"project,omitempty"`
	Title   string     `json:"title"`
	Detail  string     `json:"detail,omitempty"`
	Source  string     `json:"source"`
	Version *string    `json:"version,omitempty"`
	Link    string     `json:"link,omitempty"`
}

// Activity is the timeline page.
type Activity struct {
	GeneratedAt time.Time       `json:"generated_at"`
	Range       string          `json:"range"`
	Events      []Event         `json:"events"`
	Projects    []string        `json:"projects"` // every project with an event, for the filter
	Notes       []string        `json:"notes"`
	Sources     []source.Status `json:"sources"`
}

// ActivityInputs are the answers the timeline is built from.
type ActivityInputs struct {
	Now          time.Time
	Range        string
	History      *source.SonarHistory
	Gaps         []source.LogGap
	SourceStates []source.Status
}

// BuildActivity assembles the timeline, newest first.
func BuildActivity(in ActivityInputs) Activity {
	a := Activity{GeneratedAt: in.Now.UTC(), Range: in.Range, Events: []Event{}, Sources: mergeSources(in.SourceStates),
		Notes: []string{
			"Deploys are the container swaps Sonar saw. Lighthouse's own deploy records aren't connected yet.",
			"Health changes and container restarts aren't listed: Sonar doesn't serve them yet.",
			"Errors per project and hour aren't listed: Blackbox doesn't serve error counts yet. The Logs page shows every error line.",
			"Events close together are leads to investigate, not proof that one caused the other.",
		}}

	if in.History != nil {
		for _, u := range in.History.Updates {
			e := Event{At: u.Started, Project: u.Project, Source: "Sonar", Version: u.To,
				Link: "#/logs?project=" + u.Project + "&from=" + u.Started.Add(-5*time.Minute).Format(time.RFC3339) + "&to=" + u.Ended.Add(10*time.Minute).Format(time.RFC3339)}
			end := u.Ended
			e.End = &end
			if u.Outcome == "ok" {
				e.Kind = "deploy"
				e.Title = u.Project + " deployed"
				if u.To != nil {
					e.Title += " " + *u.To
				}
			} else {
				e.Kind = "deploy_failed"
				e.Title = u.Project + " deploy " + u.Outcome
			}
			e.Detail = versionChange(u.From, u.To) + fmt.Sprintf(" Took %d s.", u.Seconds)
			a.Events = append(a.Events, e)
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
	Days         []IncidentDay   `json:"days"`
	Availability []Availability  `json:"availability"`
	Overall      *source.Uptime  `json:"overall_30d"`
	Notes        []string        `json:"notes"`
	Sources      []source.Status `json:"sources"`
}

// BuildIncidents lists the days with downtime or degradation, newest first.
func BuildIncidents(now time.Time, st *source.SonarStatus, states []source.Status) Incidents {
	inc := Incidents{GeneratedAt: now.UTC(), Days: []IncidentDay{}, Availability: []Availability{}, Sources: mergeSources(states),
		Notes: []string{
			"Incidents are whole UTC days from Sonar's history: it doesn't serve when each one began and ended yet.",
			"Degraded and deploy minutes count as available. Unobserved time is left out of uptime and shown as coverage instead.",
		}}
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
