package source

import (
	"context"
	"net/url"
	"strconv"
	"time"
)

// SonarProjects is Sonar's GET /v1/monitor/projects: each project's group,
// services, containers and outside checks.
type SonarProjects struct {
	GeneratedAt *time.Time       `json:"generated_at"`
	Fresh       bool             `json:"fresh"`
	Projects    []MonitorProject `json:"projects"`
}

// MonitorProject is one project in SonarProjects.
type MonitorProject struct {
	Project         string           `json:"project"`
	Group           *string          `json:"group"` // infrastructure or app; nil without the label
	State           string           `json:"state"`
	StateSince      *time.Time       `json:"state_since"`
	StateSinceExact bool             `json:"state_since_exact"`
	Evidence        string           `json:"evidence"`
	Version         *string          `json:"version"`
	Services        []MonitorService `json:"services"`
	Checks          []MonitorCheck   `json:"checks"`
}

// MonitorService is one compose service.
type MonitorService struct {
	Service    string           `json:"service"`
	State      string           `json:"state"`
	Counted    bool             `json:"counted"` // false for one-off jobs
	Update     *ServiceUpdate   `json:"update"`  // a deploy in progress
	Usage      Usage            `json:"usage"`
	Containers []ContainerState `json:"containers"`
}

// ServiceUpdate is a deploy Sonar sees in progress.
type ServiceUpdate struct {
	Since time.Time `json:"since"`
	From  *string   `json:"from"`
	To    *string   `json:"to"`
}

// Usage is current resource use; nil values aren't measured yet.
type Usage struct {
	CPUCores     *float64 `json:"cpu_cores"`
	MemoryBytes  *uint64  `json:"memory_bytes"`
	NetRxRate    *float64 `json:"net_rx_bytes_per_s"`
	NetTxRate    *float64 `json:"net_tx_bytes_per_s"`
	BlkReadRate  *float64 `json:"blk_read_bytes_per_s"`
	BlkWriteRate *float64 `json:"blk_write_bytes_per_s"`
}

// ContainerState is one container as Docker reports it.
type ContainerState struct {
	Name         string     `json:"name"`
	Image        string     `json:"image"`
	Version      *string    `json:"version"`
	State        string     `json:"state"`  // running, exited, restarting, ...
	Health       string     `json:"health"` // healthy, unhealthy, starting, none
	ExitCode     int        `json:"exit_code"`
	RestartCount int        `json:"restart_count"`
	StartedAt    *time.Time `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at"`
	CPULimit     *float64   `json:"cpu_limit"`    // cores; nil when unlimited
	MemoryLimit  *int64     `json:"memory_limit"` // bytes; nil when unlimited
	Usage        Usage      `json:"usage"`
}

// MonitorCheck is one outside check of a public address.
type MonitorCheck struct {
	Service             string     `json:"service"`
	URL                 string     `json:"url"`
	State               string     `json:"state"`
	LastAt              *time.Time `json:"last_at"`
	LastOutcome         *string    `json:"last_outcome"` // ok, slow or failed
	LastStatus          *int       `json:"last_status"`
	LastLatencyMs       *int64     `json:"last_latency_ms"`
	LastError           *string    `json:"last_error"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
}

// SonarConditions is Sonar's GET /v1/monitor/conditions: Spark's own state.
type SonarConditions struct {
	GeneratedAt *time.Time  `json:"generated_at"`
	Fresh       bool        `json:"fresh"`
	State       string      `json:"state"` // ok, warning, critical, unknown
	Conditions  []Condition `json:"conditions"`
	Collectors  []Collector `json:"collectors"`
	Attention   []string    `json:"attention"` // configuration problems, such as a refused label
}

// Condition is one reason Spark isn't ok.
type Condition struct {
	ID      string    `json:"id"`
	Subject *string   `json:"subject"`
	Level   string    `json:"level"` // warning or critical
	Message string    `json:"message"`
	Since   time.Time `json:"since"`
}

// Collector is one of Sonar's measurements and whether it works.
type Collector struct {
	Name   string     `json:"name"`
	State  string     `json:"state"` // available, unavailable, stale, unsupported
	LastOK *time.Time `json:"last_ok"`
	Error  *string    `json:"error"`
}

// SonarStates is Sonar's GET /v1/monitor/states: when each project's state changed.
type SonarStates struct {
	Range    string          `json:"range"`
	From     time.Time       `json:"from"`
	To       time.Time       `json:"to"`
	Projects []ProjectStates `json:"projects"`
}

// ProjectStates is one project's spans, oldest first.
type ProjectStates struct {
	Project string      `json:"project"`
	Spans   []StateSpan `json:"spans"`
}

// StateSpan is a stretch of one state, with what wasn't up during it.
type StateSpan struct {
	State           string       `json:"state"`
	Start           time.Time    `json:"start"`
	End             time.Time    `json:"end"`
	ObservedMinutes int          `json:"observed_minutes"`
	Services        []SpanCause  `json:"services"`
	Checks          []CheckCause `json:"checks"`
}

// SpanCause is a service that wasn't up during a span.
type SpanCause struct {
	Service string `json:"service"`
	State   string `json:"state"`
	Minutes int    `json:"minutes"`
}

// CheckCause is an outside check that failed or was slow during a span.
type CheckCause struct {
	Service   string  `json:"service"`
	URL       string  `json:"url"`
	State     string  `json:"state"`
	Failures  int     `json:"failures"`
	Slow      int     `json:"slow"`
	LastError *string `json:"last_error"`
}

// SonarUsage is Sonar's GET /v1/monitor/usage: each service's use over a range.
type SonarUsage struct {
	Range         string         `json:"range"`
	From          time.Time      `json:"from"`
	To            time.Time      `json:"to"`
	BucketSeconds int            `json:"bucket_seconds"`
	Services      []ServiceUsage `json:"services"`
}

// ServiceUsage is one service's buckets, only those with data.
type ServiceUsage struct {
	Project string       `json:"project"`
	Service string       `json:"service"`
	Points  []UsagePoint `json:"points"`
}

// UsagePoint is one bucket of a service's use.
type UsagePoint struct {
	T             time.Time `json:"t"`
	Minutes       int       `json:"minutes"`
	CPUCores      *Peak     `json:"cpu_cores"`
	MemoryBytes   *PeakInt  `json:"memory_bytes"`
	NetRxBytes    int64     `json:"net_rx_bytes"`
	NetTxBytes    int64     `json:"net_tx_bytes"`
	BlkReadBytes  int64     `json:"blk_read_bytes"`
	BlkWriteBytes int64     `json:"blk_write_bytes"`
}

// PeakInt is a bucket's average and maximum, in whole units.
type PeakInt struct {
	Avg int64 `json:"avg"`
	Max int64 `json:"max"`
}

// SonarDatabases is Sonar's GET /v1/monitor/databases.
type SonarDatabases struct {
	Now *DatabaseServer `json:"now"` // nil until Sonar has read sparkdb
}

// DatabaseServer is sparkdb now.
type DatabaseServer struct {
	ReadAt         time.Time     `json:"read_at"`
	MaxConnections int           `json:"max_connections"`
	Connections    int           `json:"connections"`
	Databases      []DatabaseNow `json:"databases"`
}

// DatabaseNow is one database now.
type DatabaseNow struct {
	Name           string  `json:"name"`
	Project        *string `json:"project"`
	SizeBytes      int64   `json:"size_bytes"`
	Connections    int     `json:"connections"`
	Active         int     `json:"active"`
	OldestXactSecs *int64  `json:"oldest_xact_secs"`
}

// SonarDisk is Sonar's GET /v1/monitor/disk: what each project keeps on disk.
type SonarDisk struct {
	Now *DiskUseNow `json:"now"` // nil until Sonar has measured
}

// DiskUseNow is the latest measurement.
type DiskUseNow struct {
	MeasuredAt time.Time        `json:"measured_at"`
	Totals     DiskTotals       `json:"totals"`
	Projects   []ProjectDiskUse `json:"projects"`
}

// DiskTotals is Docker's and the data folders' use in all.
type DiskTotals struct {
	DataBytes       *int64 `json:"data_bytes"`
	ImageBytes      int64  `json:"image_bytes"`
	ContainerBytes  int64  `json:"container_bytes"`
	VolumeBytes     int64  `json:"volume_bytes"`
	BuildCacheBytes int64  `json:"build_cache_bytes"`
}

// ProjectDiskUse is one project's share.
type ProjectDiskUse struct {
	Project        string `json:"project"`
	DataBytes      *int64 `json:"data_bytes"` // nil without a data folder
	DataFiles      int64  `json:"data_files"`
	DataUnreadable int64  `json:"data_unreadable"`
	ImageBytes     int64  `json:"image_bytes"`
	ContainerBytes int64  `json:"container_bytes"`
	VolumeBytes    int64  `json:"volume_bytes"`
}

// SonarTraffic is Sonar's GET /v1/monitor/traffic: requests per public site.
type SonarTraffic struct {
	Range         string        `json:"range"`
	BucketSeconds int           `json:"bucket_seconds"`
	Hosts         []HostTraffic `json:"hosts"`
}

// HostTraffic is one hostname's traffic over the range.
type HostTraffic struct {
	Host      string         `json:"host"`
	Project   *string        `json:"project"`
	Requests  int64          `json:"requests"`
	Errors5xx int64          `json:"errors_5xx"`
	Bytes     int64          `json:"bytes"`
	Points    []TrafficPoint `json:"points"`
}

// TrafficPoint is one bucket with traffic.
type TrafficPoint struct {
	T         time.Time `json:"t"`
	Requests  int64     `json:"requests"`
	Errors5xx int64     `json:"errors_5xx"`
	Bytes     int64     `json:"bytes"`
}

// SonarApps is Sonar's GET /v1/monitor/apps: the reports apps serve.
type SonarApps struct {
	Days     int          `json:"days"`
	Projects []AppProject `json:"projects"`
}

// AppProject is one project's report.
type AppProject struct {
	Project   string        `json:"project"`
	Reporters []AppReporter `json:"reporters"`
	Counters  []AppCounter  `json:"counters"`
	Checks    []AppCheck    `json:"checks"`
	Changes   []AppChange   `json:"changes"`
}

// AppReporter is a container that serves a report.
type AppReporter struct {
	Service   string     `json:"service"`
	Container string     `json:"container"`
	ReadAt    *time.Time `json:"read_at"`
	Error     *string    `json:"error"`
}

// AppCounter is one counter over the window, with its days.
type AppCounter struct {
	Name  string     `json:"name"`
	Total int64      `json:"total"`
	Days  []CountDay `json:"days"`
}

// CountDay is a counter's total on one UTC day.
type CountDay struct {
	Date  string `json:"date"`
	Total int64  `json:"total"`
}

// AppCheck is one of an app's own checks now. Message is the app's text.
type AppCheck struct {
	Service string    `json:"service"`
	Name    string    `json:"name"`
	OK      bool      `json:"ok"`
	Message string    `json:"message"`
	Since   time.Time `json:"since"`
}

// AppChange is a check changing in the window.
type AppChange struct {
	Service string    `json:"service"`
	Name    string    `json:"name"`
	At      time.Time `json:"at"`
	OK      bool      `json:"ok"`
	Message string    `json:"message"`
}

// TrafficRange maps Vista's range onto the shortest traffic range that
// covers it: Cloudflare's data starts at 24 hours.
func TrafficRange(r string) string {
	if r == "1h" {
		return "24h"
	}
	return r
}

// appDays is how many days of app counters Vista asks for.
const appDays = 30

func sonarGet[T any](path string, q url.Values) func(*apiClient) func(context.Context) (T, error) {
	return func(c *apiClient) func(context.Context) (T, error) {
		return func(ctx context.Context) (T, error) {
			var v T
			err := c.getJSON(ctx, path, q, &v)
			return v, err
		}
	}
}

var (
	sonarProjects   = sonarGet[SonarProjects]("/v1/monitor/projects", nil)
	sonarConditions = sonarGet[SonarConditions]("/v1/monitor/conditions", nil)
	sonarDatabases  = sonarGet[SonarDatabases]("/v1/monitor/databases", url.Values{"range": {"1h"}})
	sonarDisk       = sonarGet[SonarDisk]("/v1/monitor/disk", url.Values{"range": {"7d"}})
	sonarApps       = sonarGet[SonarApps]("/v1/monitor/apps", url.Values{"days": {strconv.Itoa(appDays)}})
)

func sonarStates(r string) func(*apiClient) func(context.Context) (SonarStates, error) {
	return sonarGet[SonarStates]("/v1/monitor/states", url.Values{"range": {r}})
}

func sonarTraffic(r string) func(*apiClient) func(context.Context) (SonarTraffic, error) {
	return sonarGet[SonarTraffic]("/v1/monitor/traffic", url.Values{"range": {r}})
}

// ErrorCounts is Blackbox's GET /v1/stats/errors: warn and error lines per
// project per bucket, only non-empty buckets.
type ErrorCounts struct {
	From    time.Time     `json:"from"`
	To      time.Time     `json:"to"`
	Bucket  string        `json:"bucket"` // hour or day
	Buckets []ErrorBucket `json:"buckets"`
	Totals  []ErrorTotal  `json:"totals"`
}

// ErrorBucket is one project's lines in one bucket.
type ErrorBucket struct {
	Start   time.Time `json:"start"`
	Project *string   `json:"project"`
	Errors  int64     `json:"error"`
	Warns   int64     `json:"warn"`
}

// ErrorTotal is one project's lines over the range.
type ErrorTotal struct {
	Project *string `json:"project"`
	Errors  int64   `json:"error"`
	Warns   int64   `json:"warn"`
}

// ErrorGroups is Blackbox's GET /v1/stats/errors/groups: repeated lines.
type ErrorGroups struct {
	Groups      []ErrorGroup `json:"groups"`
	TotalGroups int64        `json:"total_groups"`
}

// ErrorGroup is the lines of one service and level that share a fingerprint.
// Fingerprint and NewestMessage are untrusted text.
type ErrorGroup struct {
	Fingerprint   string    `json:"fingerprint"`
	Project       *string   `json:"project"`
	Service       string    `json:"service"`
	Level         string    `json:"level"`
	Count         int64     `json:"count"`
	FirstSeen     time.Time `json:"first_seen"`
	LastSeen      time.Time `json:"last_seen"`
	NewestID      int64     `json:"newest_id"`
	NewestMessage string    `json:"newest_message"`
}

// errorGroupLimit is how many repeated errors Vista lists.
const errorGroupLimit = 15

// statsWindow is the last window up to now; Blackbox answers at most 31 days.
func statsWindow(window time.Duration) url.Values {
	to := time.Now().UTC()
	return url.Values{"from": {to.Add(-window).Format(time.RFC3339)}, "to": {to.Format(time.RFC3339)}}
}

func blackboxErrorCounts(window time.Duration) func(*apiClient) func(context.Context) (ErrorCounts, error) {
	return func(c *apiClient) func(context.Context) (ErrorCounts, error) {
		return func(ctx context.Context) (ErrorCounts, error) {
			q := statsWindow(window)
			q.Set("bucket", "hour")
			if window > 7*24*time.Hour {
				q.Set("bucket", "day")
			}
			var v ErrorCounts
			err := c.getJSON(ctx, "/v1/stats/errors", q, &v)
			return v, err
		}
	}
}

func blackboxErrorGroups(window time.Duration) func(*apiClient) func(context.Context) (ErrorGroups, error) {
	return func(c *apiClient) func(context.Context) (ErrorGroups, error) {
		return func(ctx context.Context) (ErrorGroups, error) {
			q := statsWindow(window)
			q.Set("level", "error")
			q.Set("limit", strconv.Itoa(errorGroupLimit))
			var v ErrorGroups
			err := c.getJSON(ctx, "/v1/stats/errors/groups", q, &v)
			return v, err
		}
	}
}
