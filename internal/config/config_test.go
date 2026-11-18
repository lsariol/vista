package config

import (
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestProdNeedsAccess(t *testing.T) {
	_, err := Load(env(map[string]string{"APP_ENV": "prod"}))
	if err == nil || !strings.Contains(err.Error(), "VISTA_ACCESS_TEAM and VISTA_ACCESS_AUD are required") {
		t.Fatalf("got %v", err)
	}
	// An unset APP_ENV is prod.
	if _, err := Load(env(nil)); err == nil {
		t.Fatal("an empty environment was accepted")
	}
}

func TestDevMayRunWithoutAccess(t *testing.T) {
	c, err := Load(env(map[string]string{"APP_ENV": "dev"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.AccessEnabled() || c.Addr != ":3000" || c.Version != "dev" {
		t.Errorf("%+v", c)
	}
}

func TestFullConfig(t *testing.T) {
	c, err := Load(env(map[string]string{
		"APP_ENV": "prod", "VISTA_ACCESS_TEAM": "Mobasity", "VISTA_ACCESS_AUD": "abc",
		"VISTA_SONAR_URL": "http://sonar-api:2500/", "VISTA_SONAR_TOKEN": "t",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !c.AccessEnabled() || c.AccessTeam != "mobasity" || c.SonarURL != "http://sonar-api:2500" {
		t.Errorf("%+v", c)
	}
}

func TestRefusals(t *testing.T) {
	cases := map[string]map[string]string{
		"half access":    {"APP_ENV": "dev", "VISTA_ACCESS_TEAM": "team"},
		"team as domain": {"APP_ENV": "dev", "VISTA_ACCESS_TEAM": "team.cloudflareaccess.com", "VISTA_ACCESS_AUD": "a"},
		"bad env":        {"APP_ENV": "staging"},
		"bad url":        {"APP_ENV": "dev", "VISTA_SONAR_URL": "sonar-api:2500"},
		"token, no url":  {"APP_ENV": "dev", "VISTA_BLACKBOX_TOKEN": "t"},
	}
	for name, m := range cases {
		if _, err := Load(env(m)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
