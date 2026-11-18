// Package config reads Vista's settings from the environment once and checks them.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Config is every setting Vista reads.
type Config struct {
	Env     string // "prod" or "dev"
	Addr    string // listen address
	Version string // the deployed release

	// Cloudflare Access: the team name (<team>.cloudflareaccess.com) and the
	// application's audience tag. Both empty turns the check off, in dev only.
	AccessTeam string
	AccessAUD  string

	SonarURL      string
	SonarToken    string
	BlackboxURL   string
	BlackboxToken string
}

// AccessEnabled reports whether requests must carry a Cloudflare Access token.
func (c Config) AccessEnabled() bool { return c.AccessTeam != "" || c.AccessAUD != "" }

var teamName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// Load reads the settings through getenv and refuses ones it can't use.
func Load(getenv func(string) string) (Config, error) {
	get := func(key, def string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return def
	}
	c := Config{
		Env:           get("APP_ENV", "prod"),
		Addr:          get("VISTA_ADDR", ":3000"),
		Version:       get("VISTA_VERSION", "dev"),
		AccessTeam:    strings.ToLower(get("VISTA_ACCESS_TEAM", "")),
		AccessAUD:     get("VISTA_ACCESS_AUD", ""),
		SonarURL:      strings.TrimRight(get("VISTA_SONAR_URL", ""), "/"),
		SonarToken:    get("VISTA_SONAR_TOKEN", ""),
		BlackboxURL:   strings.TrimRight(get("VISTA_BLACKBOX_URL", ""), "/"),
		BlackboxToken: get("VISTA_BLACKBOX_TOKEN", ""),
	}

	var problems []error
	if c.Env != "prod" && c.Env != "dev" {
		problems = append(problems, fmt.Errorf("APP_ENV is %q: set it to prod or dev", c.Env))
	}
	switch {
	case c.AccessTeam == "" && c.AccessAUD == "":
		if c.Env == "prod" {
			problems = append(problems, errors.New("Cloudflare Access isn't set: VISTA_ACCESS_TEAM and VISTA_ACCESS_AUD are required when APP_ENV=prod"))
		}
	case c.AccessTeam == "" || c.AccessAUD == "":
		problems = append(problems, errors.New("Cloudflare Access is half set: give both VISTA_ACCESS_TEAM and VISTA_ACCESS_AUD, or neither (dev only)"))
	case !teamName.MatchString(c.AccessTeam):
		problems = append(problems, fmt.Errorf("VISTA_ACCESS_TEAM is %q: give only the team name, the part before .cloudflareaccess.com", c.AccessTeam))
	}
	for _, u := range []struct{ key, value string }{
		{"VISTA_SONAR_URL", c.SonarURL},
		{"VISTA_BLACKBOX_URL", c.BlackboxURL},
	} {
		if u.value == "" {
			continue
		}
		parsed, err := url.Parse(u.value)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			problems = append(problems, fmt.Errorf("%s is %q: give an address such as http://sonar-api:2500", u.key, u.value))
		}
	}
	if c.SonarToken != "" && c.SonarURL == "" {
		problems = append(problems, errors.New("VISTA_SONAR_TOKEN is set without VISTA_SONAR_URL"))
	}
	if c.BlackboxToken != "" && c.BlackboxURL == "" {
		problems = append(problems, errors.New("VISTA_BLACKBOX_TOKEN is set without VISTA_BLACKBOX_URL"))
	}
	if err := errors.Join(problems...); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	return c, nil
}
