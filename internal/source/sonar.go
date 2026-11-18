package source

import (
	"context"
	"net/url"
	"strconv"
	"time"
)

// SonarStatus is Sonar's GET /v1/monitor/status: every project's state,
// version and uptime, the ones its public answer leaves out too.
type SonarStatus struct {
	GeneratedAt *time.Time `json:"generated_at"`
	Fresh       bool       `json:"fresh"`
	Overall     struct {
		State     string `json:"state"`
		Uptime30d Uptime `json:"uptime_30d"`
	} `json:"overall"`
	Projects []SonarProject `json:"projects"`
}

// SonarProject is one project in SonarStatus.
type SonarProject struct {
	Project    string     `json:"project"`
	State      string     `json:"state"` // operational, updating, degraded, down, unknown
	Version    *string    `json:"version"`
	Evidence   string     `json:"evidence"` // checked or container
	ResponseMs *int       `json:"response_ms"`
	Uptime30d  Uptime     `json:"uptime_30d"`
	Uptime90d  Uptime     `json:"uptime_90d"`
	Days       []SonarDay `json:"days"`
}

// Uptime is a percentage with how much of the window was observed. Pct is
// nil under half coverage.
type Uptime struct {
	Pct         *float64 `json:"pct"`
	CoveragePct float64  `json:"coverage_pct"`
}

// SonarDay is one UTC day of a project's history.
type SonarDay struct {
	Date            string   `json:"date"`
	State           string   `json:"state"` // operational, degraded, down, no_data
	UptimePct       *float64 `json:"uptime_pct"`
	CoveragePct     float64  `json:"coverage_pct"`
	Updates         int      `json:"updates"`
	ObservedMinutes int      `json:"observed_minutes"`
	DegradedMinutes int      `json:"degraded_minutes"`
	DownMinutes     int      `json:"down_minutes"`
}

// SonarHistory is Sonar's GET /v1/status/history: host CPU and memory, and deploys.
type SonarHistory struct {
	Range         string        `json:"range"`
	From          time.Time     `json:"from"`
	To            time.Time     `json:"to"`
	BucketSeconds int           `json:"bucket_seconds"`
	GeneratedAt   time.Time     `json:"generated_at"`
	Points        []HostPoint   `json:"points"`
	Updates       []SonarUpdate `json:"updates"`
}

// HostPoint is one bucket of host measurements; nil values are gaps.
type HostPoint struct {
	T           time.Time `json:"t"`
	CPUPct      *Spread   `json:"cpu_pct"`
	MemUsedPct  *Spread   `json:"mem_used_pct"`
	CoveragePct float64   `json:"coverage_pct"`
}

// Spread is a bucket's average, minimum and maximum.
type Spread struct {
	Avg float64 `json:"avg"`
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// SonarUpdate is one deploy Sonar saw.
type SonarUpdate struct {
	Project string    `json:"project"`
	Started time.Time `json:"started"`
	Ended   time.Time `json:"ended"`
	Seconds int       `json:"seconds"`
	From    *string   `json:"from"`
	To      *string   `json:"to"`
	Outcome string    `json:"outcome"` // ok, failed, timed out
}

// Ranges are the history windows Sonar serves, shortest first.
var Ranges = []string{"1h", "24h", "7d", "30d"}

// RangeDuration is how long a range is; ok is false for an unknown one.
func RangeDuration(r string) (time.Duration, bool) {
	switch r {
	case "1h":
		return time.Hour, true
	case "24h":
		return 24 * time.Hour, true
	case "7d":
		return 7 * 24 * time.Hour, true
	case "30d":
		return 30 * 24 * time.Hour, true
	}
	return 0, false
}

// statusDays is how many days of project history Vista asks for.
const statusDays = 90

func sonarStatus(c *apiClient) func(context.Context) (SonarStatus, error) {
	return func(ctx context.Context) (SonarStatus, error) {
		var s SonarStatus
		err := c.getJSON(ctx, "/v1/monitor/status", url.Values{"days": {strconv.Itoa(statusDays)}}, &s)
		return s, err
	}
}

func sonarHistory(c *apiClient, r string) func(context.Context) (SonarHistory, error) {
	return func(ctx context.Context) (SonarHistory, error) {
		var h SonarHistory
		err := c.getJSON(ctx, "/v1/status/history", url.Values{"range": {r}}, &h)
		return h, err
	}
}
