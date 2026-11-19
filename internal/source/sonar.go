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

// SonarHost is Sonar's GET /v1/monitor/host: the host now and over a range,
// network traffic per interface, and every project's deploys.
type SonarHost struct {
	Range         string        `json:"range"`
	From          time.Time     `json:"from"`
	To            time.Time     `json:"to"`
	BucketSeconds int           `json:"bucket_seconds"`
	GeneratedAt   time.Time     `json:"generated_at"`
	Now           *HostNow      `json:"now"` // nil until Sonar's agent has read the host
	Points        []HostPoint   `json:"points"`
	Network       []NetSeries   `json:"network"`
	Updates       []SonarUpdate `json:"updates"`
}

// HostNow is the host's latest reading.
type HostNow struct {
	ReadAt        time.Time  `json:"read_at"`
	CPUPct        *float64   `json:"cpu_pct"`
	CorePcts      []float64  `json:"core_pcts"`
	MemoryTotal   uint64     `json:"memory_total"`
	MemoryUsed    uint64     `json:"memory_used"`
	MemoryUsedPct float64    `json:"memory_used_pct"`
	SwapTotal     uint64     `json:"swap_total"`
	SwapUsed      uint64     `json:"swap_used"`
	Load1         float64    `json:"load1"`
	Load5         float64    `json:"load5"`
	Load15        float64    `json:"load15"`
	UptimeSeconds float64    `json:"uptime_seconds"`
	BootedAt      *time.Time `json:"booted_at"`
	Disk          *DiskNow   `json:"disk"`
	Network       []NetNow   `json:"network"`
}

// DiskNow is the host's filesystem.
type DiskNow struct {
	Total      uint64  `json:"total"`
	Used       uint64  `json:"used"`
	Available  uint64  `json:"available"`
	UsedPct    float64 `json:"used_pct"`
	Inodes     uint64  `json:"inodes"`
	InodesFree uint64  `json:"inodes_free"`
}

// NetNow is one interface's current rates; nil until Sonar has two readings.
type NetNow struct {
	Interface   string   `json:"interface"`
	RxBytesPerS *float64 `json:"rx_bytes_per_s"`
	TxBytesPerS *float64 `json:"tx_bytes_per_s"`
}

// NetSeries is one interface's traffic over the range, only buckets with data.
type NetSeries struct {
	Interface string     `json:"interface"`
	Points    []NetPoint `json:"points"`
}

// NetPoint is the bytes an interface moved during one bucket.
type NetPoint struct {
	T       time.Time `json:"t"`
	Minutes int       `json:"minutes"`
	RxBytes int64     `json:"rx_bytes"`
	TxBytes int64     `json:"tx_bytes"`
}

// HostPoint is one bucket of host measurements; nil values are gaps.
type HostPoint struct {
	T              time.Time `json:"t"`
	CoveragePct    float64   `json:"coverage_pct"`
	CPUPct         *Spread   `json:"cpu_pct"`
	MemUsedPct     *Spread   `json:"mem_used_pct"`
	MemUsedBytes   *int64    `json:"mem_used_bytes"`
	MemTotalBytes  *int64    `json:"mem_total_bytes"`
	SwapUsedBytes  *int64    `json:"swap_used_bytes"`
	Load1          *Peak     `json:"load1"`
	DiskUsedBytes  *int64    `json:"disk_used_bytes"`
	DiskTotalBytes *int64    `json:"disk_total_bytes"`
}

// Spread is a bucket's average, minimum and maximum, and its lowest and
// highest one-minute average (nil before Sonar v0.3.1).
type Spread struct {
	Avg       float64  `json:"avg"`
	Min       float64  `json:"min"`
	Max       float64  `json:"max"`
	MinuteMin *float64 `json:"minute_min"`
	MinuteMax *float64 `json:"minute_max"`
}

// Peak is a bucket's average and maximum.
type Peak struct {
	Avg float64 `json:"avg"`
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

func sonarHost(c *apiClient, r string) func(context.Context) (SonarHost, error) {
	return func(ctx context.Context) (SonarHost, error) {
		var h SonarHost
		err := c.getJSON(ctx, "/v1/monitor/host", url.Values{"range": {r}}, &h)
		return h, err
	}
}
