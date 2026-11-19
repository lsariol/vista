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
	if o.Spark.State != Unknown {
		t.Errorf("Vista doesn't judge Spark's state itself, got %s", o.Spark.State)
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

func TestRecentErrorsAndBlackboxMissing(t *testing.T) {
	recs := []source.LogRecord{{ID: 2, Project: str("lys")}, {ID: 1, Container: "loose"}}
	in := Inputs{Now: now, StatusState: ok("sonar"), Status: status(), RecentErrors: recs, ErrorsKnown: true}
	if got := BuildOverview(in).RecentErrors; len(got) != 2 || got[0].ID != 2 {
		t.Errorf("recent errors = %+v", got)
	}
	in.RecentErrors = nil
	if got := BuildOverview(in).RecentErrors; got == nil || len(got) != 0 {
		t.Errorf("no errors should be an empty list, got %v", got)
	}
	in.ErrorsKnown = false
	if got := BuildOverview(in).RecentErrors; got != nil {
		t.Errorf("without Blackbox the list should be unknown, got %v", got)
	}
}

func TestInfrastructureNotReportedIsUnknown(t *testing.T) {
	o := BuildOverview(Inputs{Now: now, StatusState: ok("sonar"),
		Status: status(source.SonarProject{Project: "cove", State: "down", Version: str("v1.4.0")})})
	by := map[string]Infra{}
	for _, i := range o.Infrastructure {
		by[i.Name] = i
	}
	if len(o.Infrastructure) != len(infrastructure) {
		t.Fatalf("infrastructure = %+v", o.Infrastructure)
	}
	if c := by["cove"]; !c.Reported || c.State != Down || *c.Version != "v1.4.0" {
		t.Errorf("cove = %+v", c)
	}
	if s := by["sparkdb"]; s.Reported || s.State != Unknown {
		t.Errorf("sparkdb = %+v", s)
	}
}

func TestHostNowIsSonarsReading(t *testing.T) {
	cpu := 12.0
	read := now.Add(-30 * time.Second)
	host := &source.SonarHost{Now: &source.HostNow{ReadAt: read, CPUPct: &cpu, MemoryTotal: 100, MemoryUsedPct: 40, Load1: 0.7},
		Points: []source.HostPoint{{T: now.Add(-time.Minute)}}}
	o := BuildOverview(Inputs{Now: now, StatusState: ok("sonar"), Status: status(), History: host})
	if o.Host.CPU == nil || o.Host.CPU.Pct != 12 || !o.Host.CPU.At.Equal(read) || o.Host.Memory.Pct != 40 {
		t.Errorf("cpu = %+v, memory = %+v", o.Host.CPU, o.Host.Memory)
	}
	if o.Host.Now == nil || o.Host.Now.Load1 != 0.7 || o.Host.Network == nil {
		t.Errorf("now = %+v, network = %v", o.Host.Now, o.Host.Network)
	}
	o = BuildOverview(Inputs{Now: now, StatusState: ok("sonar"), Status: status()})
	if o.Host.CPU != nil || o.Host.Now != nil {
		t.Errorf("without Sonar's host: %+v", o.Host)
	}
}

func TestSparkStateIsSonarsConditions(t *testing.T) {
	cond := func(state string, fresh bool) *source.SonarConditions {
		return &source.SonarConditions{State: state, Fresh: fresh, Conditions: []source.Condition{
			{ID: "disk", Level: "critical", Message: "The disk is 97% full", Since: now.Add(-time.Hour)},
			{ID: "project", Subject: str("cove"), Level: "warning", Message: "cove is degraded", Since: now},
		}}
	}
	for _, c := range []struct {
		state   string
		fresh   bool
		current bool
		want    Health
	}{
		{"ok", true, true, Healthy}, {"warning", true, true, Degraded}, {"critical", true, true, Down},
		{"unknown", true, true, Unknown}, {"ok", false, true, Unknown}, {"ok", true, false, Unknown},
	} {
		o := BuildOverview(Inputs{Now: now, StatusState: ok("sonar"), Status: status(), Conditions: cond(c.state, c.fresh), CondCurrent: c.current})
		if o.Spark.State != c.want {
			t.Errorf("%s fresh=%v current=%v: spark = %s, want %s", c.state, c.fresh, c.current, o.Spark.State, c.want)
		}
		if c.want == Unknown && len(o.Spark.Reasons) == 0 {
			t.Errorf("%s: unknown without a reason", c.state)
		}
		if c.want == Down {
			if len(o.Attention) != 1 || o.Attention[0].Level != "down" || o.Attention[0].Title != "The disk is 97% full" {
				t.Errorf("attention = %+v", o.Attention)
			}
		}
	}
}

func TestDetailAttachesToEachProject(t *testing.T) {
	infra, app := "infrastructure", "app"
	data := int64(42)
	in := Inputs{Now: now, StatusState: ok("sonar"),
		Status: status(source.SonarProject{Project: "lys", State: "operational"}, source.SonarProject{Project: "vault", State: "operational"},
			source.SonarProject{Project: "sonar", State: "operational"}),
		Projects: &source.SonarProjects{Fresh: true, Projects: []source.MonitorProject{
			{Project: "vault", Group: &infra, Services: []source.MonitorService{{Service: "vault", Containers: []source.ContainerState{{Name: "vault", State: "running"}}}}},
			{Project: "sonar", Group: &app},
			{Project: "lys", StateSince: &now, Services: []source.MonitorService{{Service: "lys", Containers: []source.ContainerState{{Name: "lys", State: "running", RestartCount: 3}}}}},
		}},
		Disk:        &source.SonarDisk{Now: &source.DiskUseNow{Projects: []source.ProjectDiskUse{{Project: "lys", DataBytes: &data}}}},
		Databases:   &source.SonarDatabases{Now: &source.DatabaseServer{Databases: []source.DatabaseNow{{Name: "lys_db", Project: str("lys")}, {Name: "AdminDB"}}}},
		Traffic:     &source.SonarTraffic{Hosts: []source.HostTraffic{{Host: "lys.mobasity.com", Project: str("lys"), Requests: 9, Points: []source.TrafficPoint{{Requests: 9}}}}},
		ErrorCounts: &source.ErrorCounts{Totals: []source.ErrorTotal{{Project: str("lys"), Errors: 4}}},
	}
	o := BuildOverview(in)
	by := map[string]Project{}
	for _, p := range o.Projects {
		by[p.Name] = p
	}
	lys := by["lys"]
	if lys.Disk == nil || *lys.Disk.DataBytes != 42 || len(lys.Databases) != 1 || len(lys.Traffic) != 1 || lys.Traffic[0].Points != nil ||
		lys.Errors == nil || lys.Errors.Errors != 4 || lys.StateSince == nil {
		t.Errorf("lys = %+v", lys)
	}
	if e := by["vault"].Errors; e == nil || e.Errors != 0 {
		t.Errorf("a project without errors should count zero once Blackbox answers: %+v", e)
	}
	if !by["vault"].Infrastructure || by["sonar"].Infrastructure {
		t.Errorf("Sonar's group decides: vault %v, sonar %v", by["vault"].Infrastructure, by["sonar"].Infrastructure)
	}
	if len(o.Containers) != 2 || o.Containers[0].Project != "lys" || o.Containers[0].RestartCount != 3 {
		t.Errorf("containers = %+v", o.Containers)
	}
	var infraNames []string
	for _, i := range o.Infrastructure {
		infraNames = append(infraNames, i.Name)
	}
	if got := strings.Join(infraNames, ","); got != "sparkdb,cove,lighthouse,cloudflared,sonar,blackbox,vault" {
		t.Errorf("infrastructure = %s", got)
	}
	if o := BuildOverview(Inputs{Now: now, StatusState: ok("sonar"), Status: status()}); o.Containers != nil || o.Errors != nil {
		t.Errorf("without Sonar's detail or Blackbox: containers %v, errors %v", o.Containers, o.Errors)
	}
}

func TestStateChangesOnTheTimelineAndIncidents(t *testing.T) {
	spans := &source.SonarStates{Projects: []source.ProjectStates{{Project: "lys", Spans: []source.StateSpan{
		{State: "operational", Start: now.Add(-3 * time.Hour), End: now.Add(-2 * time.Hour)},
		{State: "down", Start: now.Add(-2 * time.Hour), End: now.Add(-90 * time.Minute),
			Services: []source.SpanCause{{Service: "lys", State: "down", Minutes: 30}},
			Checks: []source.CheckCause{{URL: "https://lys.mobasity.com", Failures: 6, Slow: 2, LastError: str("timeout"),
				Slowest: &source.SlowestCheck{Ms: 2600, DNSMs: 20, ConnectMs: 2400, TLSMs: 90, WaitMs: 80, ReadMs: 10}}}},
		{State: "operational", Start: now.Add(-90 * time.Minute), End: now},
	}}}}
	a := BuildActivity(ActivityInputs{Now: now, Range: "24h", States: spans,
		ErrorCounts: &source.ErrorCounts{Bucket: "hour", Buckets: []source.ErrorBucket{{Start: now, Project: str("lys"), Errors: 2}}}})
	if len(a.Events) != 1 || a.Events[0].Kind != "state" || a.Events[0].Title != "lys down" {
		t.Fatalf("events = %+v", a.Events)
	}
	for _, want := range []string{"30 minutes", "Service lys down", "6 failed", "slowest 2.6 s (2.4 s connecting)", "timeout"} {
		if !strings.Contains(a.Events[0].Detail, want) {
			t.Errorf("detail %q lacks %q", a.Events[0].Detail, want)
		}
	}
	if a.Errors == nil || len(a.Errors.Buckets) != 1 {
		t.Errorf("errors = %+v", a.Errors)
	}
	inc := BuildIncidents(now, nil, spans, nil)
	if len(inc.Incidents) != 1 || inc.Incidents[0].State != Down || !inc.Incidents[0].End.Equal(now.Add(-90*time.Minute)) {
		t.Errorf("incidents = %+v", inc.Incidents)
	}
}

func TestFailedDeployNeedsAttention(t *testing.T) {
	day := &source.SonarHost{Updates: []source.SonarUpdate{
		{Project: "lys", Started: now.Add(-time.Hour), From: str("v1.0.0"), To: str("v1.1.0"), Outcome: "failed"},
		{Project: "lys", Started: now.Add(-30 * time.Minute), From: str("v1.0.0"), To: str("v1.0.1"), Outcome: "ok"},
	}}
	o := BuildOverview(Inputs{Now: now, StatusState: ok("sonar"), Status: status(source.SonarProject{Project: "lys", State: "operational"}),
		Day: day})
	if o.Projects[0].LastDeploy == nil || *o.Projects[0].LastDeploy.To != "v1.0.1" {
		t.Errorf("last deploy = %+v", o.Projects[0].LastDeploy)
	}
	var titles []string
	for _, a := range o.Attention {
		titles = append(titles, a.Title)
	}
	if joined := strings.Join(titles, " | "); !strings.Contains(joined, "Deploy of lys failed") {
		t.Errorf("attention %q lacks the failed deploy", joined)
	}
}

func TestDeploysPreferLighthouse(t *testing.T) {
	sonarDay := &source.SonarHost{Updates: []source.SonarUpdate{{Project: "lys", Started: now.Add(-time.Hour), Outcome: "ok"}}}
	lh := []source.LighthouseDeployment{
		{ID: 2, Project: "lys", Version: str("v1.1.0"), Status: "rolled_back", FailedStep: str("health"), StartedAt: now.Add(-20 * time.Minute), FinishedAt: now.Add(-18 * time.Minute)},
		{ID: 1, Project: "lys", SHA: str("abcdef0123"), Status: "succeeded", StartedAt: now.Add(-2 * time.Hour), FinishedAt: now.Add(-2 * time.Hour).Add(time.Minute)},
	}
	got := pickDeploys(lh, true, sonarDay)
	if len(got) != 2 || got[0].ID != 1 || *got[0].To != "abcdef0" || got[1].Outcome != "rolled back" || got[1].Source != "Lighthouse" {
		t.Errorf("with Lighthouse: %+v", got)
	}
	if d := deployDetail(got[1]); d != "Version v1.1.0. Failed at health." {
		t.Errorf("detail = %q", d)
	}
	if got := pickDeploys(nil, true, sonarDay); len(got) != 0 {
		t.Errorf("Lighthouse answered with none, so none: %+v", got)
	}
	if got := pickDeploys(nil, false, sonarDay); len(got) != 1 || got[0].Source != "Sonar" {
		t.Errorf("without Lighthouse: %+v", got)
	}
}

func TestLighthouseStatesNeedAttention(t *testing.T) {
	lys := "lys"
	o := BuildOverview(Inputs{Now: now, StatusState: ok("sonar"), Status: status(source.SonarProject{Project: "lys", State: "operational"}),
		LHKnown: true, Lighthouse: []source.LighthouseProject{
			{Name: "Lys", ComposeProject: &lys, State: "failing", LastError: str("build: no space left"), DeployedVersion: str("v1.0.0")},
			{Name: "plop", State: "ok"},
			{Name: "blackboxlog", Kind: "publish", State: "broken"},
		}})
	if len(o.Attention) != 1 || o.Attention[0].Title != "Lighthouse reports lys's deploys failing" || o.Attention[0].Detail != "build: no space left" {
		t.Errorf("attention = %+v", o.Attention)
	}
	if l := o.Projects[0].Lighthouse; l == nil || l.State != "failing" || *l.Version != "v1.0.0" {
		t.Errorf("lys's Lighthouse state = %+v", l)
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

func TestActivityNewestFirst(t *testing.T) {
	base := now.Truncate(time.Hour)
	a := BuildActivity(ActivityInputs{Now: now, Range: "24h",
		History: &source.SonarHost{Updates: []source.SonarUpdate{{Project: "dibs", Started: base.Add(20 * time.Minute), Ended: base.Add(21 * time.Minute), To: str("v2.0.0"), Outcome: "ok"}}},
		Gaps:    []source.LogGap{{Kind: "collector_down", Container: str("lys"), From: base.Add(30 * time.Minute), To: base.Add(35 * time.Minute)}}})
	if len(a.Events) != 2 {
		t.Fatalf("events = %+v", a.Events)
	}
	if a.Events[0].Kind != "log_gap" || a.Events[1].Kind != "deploy" || a.Events[1].Title != "dibs deployed v2.0.0" {
		t.Errorf("events = %+v", a.Events)
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
	inc := BuildIncidents(now, st, nil, nil)
	if len(inc.Days) != 2 || inc.Days[0].Date != "2026-11-03" || inc.Days[0].State != Degraded || inc.Days[1].State != Down {
		t.Errorf("days = %+v", inc.Days)
	}
	if empty := BuildIncidents(now, nil, nil, nil); empty.Overall != nil || len(empty.Days) != 0 {
		t.Errorf("without Sonar: %+v", empty)
	}
}
