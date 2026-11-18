package source

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestCachedKeepsTheLastGoodAnswer(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	calls, fail := 0, false
	c := NewCached("x", "X", 10*time.Second, func(context.Context) (int, error) {
		calls++
		if fail {
			return 0, errors.New("X can't be reached")
		}
		return calls, nil
	}, "", quiet)
	c.now = func() time.Time { return now }

	v, have, st := c.Get(context.Background())
	if v != 1 || !have || st.State != OK || st.LastOK == nil {
		t.Fatalf("first: %d %v %+v", v, have, st)
	}
	now = now.Add(5 * time.Second)
	if v, _, _ := c.Get(context.Background()); v != 1 || calls != 1 {
		t.Fatalf("within the TTL it fetched again: v=%d calls=%d", v, calls)
	}
	now = now.Add(10 * time.Second)
	fail = true
	v, have, st = c.Get(context.Background())
	if v != 1 || !have || st.State != Stale || !strings.Contains(st.Detail, "can't be reached") {
		t.Fatalf("after a failure: %d %v %+v", v, have, st)
	}
	now = now.Add(10 * time.Second)
	fail = false
	if _, _, st := c.Get(context.Background()); st.State != OK || st.Detail != "" {
		t.Fatalf("after recovering: %+v", st)
	}
}

func TestCachedWithoutAnAnswerIsUnavailable(t *testing.T) {
	c := NewCached("x", "X", time.Second, func(context.Context) (int, error) { return 0, errors.New("down") }, "", quiet)
	_, have, st := c.Get(context.Background())
	if have || st.State != Unavailable || st.LastOK != nil {
		t.Fatalf("%v %+v", have, st)
	}
}

func TestCachedNotConnected(t *testing.T) {
	c := NewCached[int]("x", "X", time.Second, nil, "Set X_URL.", quiet)
	_, have, st := c.Get(context.Background())
	if have || st.State != NotConnected || st.Detail != "Set X_URL." {
		t.Fatalf("%v %+v", have, st)
	}
}

func TestClientErrorsSayHowToFix(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   string
	}{
		{http.StatusUnauthorized, "", "add the vista consumer to Sonar, with its token in Cove as SHARED_SONAR_VISTA_TOKEN"},
		{http.StatusForbidden, "", "doesn't have the scope"},
		{http.StatusServiceUnavailable, `{"error":"database behind on migrations"}`, "database behind on migrations"},
		{http.StatusTeapot, `{"error":{"message":"short and stout"}}`, "short and stout"},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer tok" {
				t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
			}
			w.WriteHeader(tc.status)
			io.WriteString(w, tc.body)
		}))
		c := newAPIClient("Sonar", srv.URL, "tok", SonarTokenKey)
		var out any
		err := c.getJSON(context.Background(), "/v1/status", nil, &out)
		srv.Close()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%d: got %v, want it to mention %q", tc.status, err, tc.want)
		}
		if strings.Contains(err.Error(), "tok") && !strings.Contains(err.Error(), "token") {
			t.Errorf("%d: the error shows the token: %v", tc.status, err)
		}
	}
}

func TestUnreachableSource(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	c := newAPIClient("Blackbox", url, "tok", BlackboxTokenKey)
	var out any
	err := c.getJSON(context.Background(), "/v1/status", nil, &out)
	if err == nil || !strings.Contains(err.Error(), "can't be reached") {
		t.Fatalf("got %v", err)
	}
}
