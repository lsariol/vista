package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/lsariol/vista/internal/source"
)

var now = time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

func str(s string) *string { return &s }

func status(projects ...source.SonarProject) *source.SonarStatus {
	return &source.SonarStatus{Fresh: true, Projects: projects}
}

func ok(id string) source.Status { return source.Status{ID: id, Name: id, State: source.OK} }

func TestProjectsSortWorstFirstAndCount(t *testing.T) {
	o := BuildOverview(Inputs{Now: now, Range: "1h", StatusState: ok("sonar"),
		Status: status(
			source.SonarProject{Project: "lys", State: "operational", Evidence: "checked"},
			source.SonarProject{Project: "dibs", State: "down", Evidence: "container"},
			source.SonarProject{Project: "cove", State: "degraded"},
			source.SonarProject{Project: "plop", State: "updating"},
		)})
	var names []string
	for _, p := range o.Projects {
		names = append(names, p.Name)
	}
	if got := strings.Join(names, ","); got != "dibs,cove,plop,lys" {
		t.Errorf("order = %s", got)
	}
	if o.Totals != (Totals{Total: 4, Healthy: 1, Updating: 1, Degraded: 1, Down: 1}) {
		t.Errorf("totals = %+v", o.Totals)
	}
	if o.Projects[0].Monitoring != "containers only" || o.Projects[3].Monitoring != "outside check" {
		t.Errorf("monitoring: %q, %q", o.Projects[0].Monitoring, o.Projects[3].Monitoring)
	}
	if o.Spark.State != Degraded {
		t.Errorf("cove degraded should degrade Spark, got %s", o.Spark.State)
	}
	if len(o.Attention) < 2 || o.Attention[0].Level != "down" || o.Attention[0].Project != "dibs" {
		t.Errorf("attention = %+v", o.Attention)
	}
}

func TestStaleSonarMakesEveryStateUnknown(t *testing.T) {
	stale := source.Status{ID: "sonar", Name: "Sonar", State: source.Stale, Detail: "Sonar can't be reached"}
	o := BuildOverview(Inputs{Now: now, StatusState: stale, SourceStates: []source.Status{stale},
		Status: status(source.SonarProject{Project: "lys", State: "operational"})})
	p := o.Projects[0]
	if p.State != Unknown || p.LastKnown != Healthy {
		t.Errorf("project = %s, last known %s", p.State, p.LastKnown)
	}
	if o.Spark.State != Unknown {
		t.Errorf("spark = %s", o.Spark.State)
	}
	if o.Totals.Healthy != 0 || o.Totals.Unknown != 1 {
		t.Errorf("totals = %+v", o.Totals)
	}
	found := false
	for _, a := range o.Attention {
		if strings.Contains(a.Title, "Sonar isn't answering") {
			found = true
		}
	}
	if !found {
		t.Errorf("no attention item for stale Sonar: %+v", o.Attention)
	}
}

func TestNoSonarIsUnknownNotHealthy(t *testing.T) {
	o := BuildOverview(Inputs{Now: now, StatusState: source.Status{ID: "sonar", State: source.NotConnected}})
	if o.Spark.State != Unknown {
		t.Errorf("spark = %s", o.Spark.State)
	}
	if len(o.Projects) != 0 || o.Host.CPU != nil {
		t.Errorf("projects %v, cpu %v", o.Projects, o.Host.CPU)
	}
}

func TestFreshFalseIsUnknown(t *testing.T) {
	st := status()
	st.Fresh = false
	o := BuildOverview(Inputs{Now: now, StatusState: ok("sonar"), Status: st})
	if o.Spark.State != Unknown {
		t.Errorf("spark = %s", o.Spark.State)
	}
}

func TestErrorCountsAndBlackboxMissing(t *testing.T) {
	recs := []source.LogRecord{{Project: str("lys")}, {Project: str("lys")}, {Container: "loose"}}
	in := Inputs{Now: now, StatusState: ok("sonar"),
		Status:    status(source.SonarProject{Project: "lys", State: "operational"}, source.SonarProject{Project: "dibs", State: "operational"}),
		Errors24h: &source.ErrorWindow{Records: recs, Truncated: true}}
	o := BuildOverview(in)
	by := map[string]*Count{}
	for _, p := range o.Projects {
		by[p.Name] = p.Errors24h
	}
	if c := by["lys"]; c == nil || c.N != 2 || !c.AtLeast {
		t.Errorf("lys errors = %+v", c)
	}
	if c := by["dibs"]; c == nil || c.N != 0 {
		t.Errorf("dibs errors = %+v", c)
	}

	in.Errors24h = nil
	for _, p := range BuildOverview(in).Projects {
		if p.Errors24h != nil {
			t.Errorf("%s has an error count without Blackbox", p.Name)
		}
	}
}

func TestCurrentReadingSkipsGaps(t *testing.T) {
	hist := &source.SonarHistory{Points: []source.HostPoint{
		{T: now.Add(-2 * time.Minute), CPUPct: &source.Spread{Avg: 12}, MemUsedPct: &source.Spread{Avg: 40}},
		{T: now.Add(-time.Minute)},
	}}
	o := BuildOverview(Inputs{Now: now, StatusState: ok("sonar"), Status: status(), Current: hist})
	if o.Host.CPU == nil || o.Host.CPU.Pct != 12 || !o.Host.CPU.At.Equal(now.Add(-2*time.Minute)) {
		t.Errorf("cpu = %+v", o.Host.CPU)
	}
}

func TestFailedDeployAndProbeNeedAttention(t *testing.T) {
	day := &source.SonarHistory{Updates: []source.SonarUpdate{
		{Project: "lys", Started: now.Add(-time.Hour), From: str("v1.0.0"), To: str("v1.1.0"), Outcome: "failed"},
		{Project: "lys", Started: now.Add(-30 * time.Minute), From: str("v1.0.0"), To: str("v1.0.1"), Outcome: "ok"},
	}}
	o := BuildOverview(Inputs{Now: now, StatusState: ok("sonar"), Status: status(source.SonarProject{Project: "lys", State: "operational"}),
		Day: day, ProbesKnown: true,
		Probes: []source.Probe{{ID: "cove", Name: "Cove", Result: source.ProbeResult{Healthy: false, Detail: "answered 503 Service Unavailable"}}}})
	if o.Projects[0].LastDeploy == nil || *o.Projects[0].LastDeploy.To != "v1.0.1" {
		t.Errorf("last deploy = %+v", o.Projects[0].LastDeploy)
	}
	var titles []string
	for _, a := range o.Attention {
		titles = append(titles, a.Title)
	}
	joined := strings.Join(titles, " | ")
	for _, want := range []string{"Deploy of lys failed", "Cove's health check failed"} {
		if !strings.Contains(joined, want) {
			t.Errorf("attention %q lacks %q", joined, want)
		}
	}
	if o.Spark.State != Degraded {
		t.Errorf("spark = %s", o.Spark.State)
	}
}

func TestMergeSourcesKeepsTheWorst(t *testing.T) {
	got := mergeSources([]source.Status{
		{ID: "sonar", State: source.OK},
		{ID: "blackbox", State: source.OK},
		{ID: "sonar", State: source.Stale},
		{ID: "sonar", State: source.OK},
	})
	if len(got) != 2 || got[0].State != source.Stale || got[1].ID != "blackbox" {
		t.Errorf("%+v", got)
	}
}

func TestActivityGroupsErrors(t *testing.T) {
	base := now.Truncate(time.Hour)
	recs := []source.LogRecord{ // newest first, as Blackbox returns them
		{ID: 9, Time: base.Add(50 * time.Minute), Project: str("lys"), Message: "Twitch:   connection\nrefused", Version: str("v1.1.0")},
		{ID: 8, Time: base.Add(10 * time.Minute), Project: str("lys"), Message: "older"},
		{ID: 7, Time: base.Add(-10 * time.Minute), Project: str("lys"), Message: "previous hour"},
	}
	a := BuildActivity(ActivityInputs{Now: now, Range: "24h", Errors: &source.ErrorWindow{Records: recs},
		History: &source.SonarHistory{Updates: []source.SonarUpdate{{Project: "dibs", Started: base.Add(20 * time.Minute), Ended: base.Add(21 * time.Minute), To: str("v2.0.0"), Outcome: "ok"}}}})
	if len(a.Events) != 3 {
		t.Fatalf("events = %+v", a.Events)
	}
	first := a.Events[0]
	if first.Kind != "errors" || first.Title != "lys logged 2 error lines" || first.LogID != 9 || first.Detail != "Newest: Twitch: connection refused" {
		t.Errorf("first = %+v", first)
	}
	if a.Events[1].Kind != "deploy" || a.Events[1].Title != "dibs deployed v2.0.0" {
		t.Errorf("second = %+v", a.Events[1])
	}
	if strings.Join(a.Projects, ",") != "dibs,lys" {
		t.Errorf("projects = %v", a.Projects)
	}
}

func TestIncidentsListTroubledDaysNewestFirst(t *testing.T) {
	st := status(
		source.SonarProject{Project: "lys", Days: []source.SonarDay{
			{Date: "2026-11-01", DownMinutes: 5, ObservedMinutes: 1440},
			{Date: "2026-11-02", ObservedMinutes: 1440},
			{Date: "2026-11-03", DegradedMinutes: 30, ObservedMinutes: 1440},
		}},
	)
	inc := BuildIncidents(now, st, nil)
	if len(inc.Days) != 2 || inc.Days[0].Date != "2026-11-03" || inc.Days[0].State != Degraded || inc.Days[1].State != Down {
		t.Errorf("days = %+v", inc.Days)
	}
	if empty := BuildIncidents(now, nil, nil); empty.Overall != nil || len(empty.Days) != 0 {
		t.Errorf("without Sonar: %+v", empty)
	}
}
