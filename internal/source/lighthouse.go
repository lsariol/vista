package source

import (
	"context"
	"net/url"
	"strconv"
	"time"
)

// LighthouseProject is one project in Lighthouse's GET /v1/projects. State
// is Lighthouse's own word for it: ok, failing, broken, stopped or
// not_deployed.
type LighthouseProject struct {
	Name            string     `json:"name"`
	ComposeProject  *string    `json:"compose_project"`
	Kind            string     `json:"kind"`        // deploy or publish
	DeployMode      string     `json:"deploy_mode"` // branch or releases
	State           string     `json:"state"`
	DeployedVersion *string    `json:"deployed_version"`
	DeployedSHA     *string    `json:"deployed_sha"`
	DeployedAt      *time.Time `json:"deployed_at"`
	FailureCount    int        `json:"failure_count"`
	LastError       *string    `json:"last_error"`
	LastErrorAt     *time.Time `json:"last_error_at"`
	LastCheckedAt   *time.Time `json:"last_checked_at"`
}

// Key is the name the project goes by in Sonar and Blackbox: its compose
// project, or Lighthouse's name before its first deploy.
func (p LighthouseProject) Key() string {
	if p.ComposeProject != nil && *p.ComposeProject != "" {
		return *p.ComposeProject
	}
	return p.Name
}

// LighthouseDeployment is one deploy attempt, as Lighthouse recorded it.
type LighthouseDeployment struct {
	ID          int64     `json:"id"`
	Project     string    `json:"project"` // the compose project, as in Sonar and Blackbox
	Version     *string   `json:"version"`
	SHA         *string   `json:"sha"`
	Trigger     string    `json:"trigger"` // check, manual, retry, reconcile
	Status      string    `json:"status"`  // succeeded, failed, rolled_back
	FailureKind *string   `json:"failure_kind"`
	FailedStep  *string   `json:"failed_step"`
	Error       *string   `json:"error"`
	StartedAt   time.Time `json:"started_at"`
	FinishedAt  time.Time `json:"finished_at"`
}

// LighthouseStep is one step of a deploy, with the end of its output.
type LighthouseStep struct {
	Step       string    `json:"step"`
	Status     string    `json:"status"` // succeeded, failed, skipped
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	Log        *string   `json:"log"`
}

// LighthouseDeploymentDetail is one deploy with its steps.
type LighthouseDeploymentDetail struct {
	LighthouseDeployment
	Steps []LighthouseStep `json:"steps"`
}

// LighthouseRelease is one version tag Lighthouse tracks.
type LighthouseRelease struct {
	Tag         string     `json:"tag"`
	SHA         string     `json:"sha"`
	Status      string     `json:"status"`
	Note        *string    `json:"note"`
	PublishedAt *time.Time `json:"published_at"`
	URL         *string    `json:"url"`
	CreatedAt   time.Time  `json:"created_at"`
}

// deploymentLimit is the most deploys Vista reads for one range.
const deploymentLimit = 500

func lighthouseProjects(c *apiClient) func(context.Context) ([]LighthouseProject, error) {
	return func(ctx context.Context) ([]LighthouseProject, error) {
		var a struct {
			Projects []LighthouseProject `json:"projects"`
		}
		err := c.getJSON(ctx, "/v1/projects", nil, &a)
		return a.Projects, err
	}
}

func lighthouseDeployments(c *apiClient, window time.Duration) func(context.Context) ([]LighthouseDeployment, error) {
	return func(ctx context.Context) ([]LighthouseDeployment, error) {
		to := time.Now().UTC()
		var a struct {
			Deployments []LighthouseDeployment `json:"deployments"`
		}
		err := c.getJSON(ctx, "/v1/deployments", url.Values{
			"from":  {to.Add(-window).Format(time.RFC3339)},
			"to":    {to.Format(time.RFC3339)},
			"limit": {strconv.Itoa(deploymentLimit)},
		}, &a)
		return a.Deployments, err
	}
}
