// Package source reads the services Vista shows: Sonar, Blackbox and the
// health endpoints of the infrastructure. Each answer comes with the state of
// its source, so the dashboard never shows missing data as zero or healthy.
package source

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// State is how fresh a source's answer is.
type State string

const (
	OK           State = "ok"            // answered on the last try
	Stale        State = "stale"         // the last try failed; an older answer is shown
	Unavailable  State = "unavailable"   // no answer yet, and the last try failed
	NotConnected State = "not_connected" // Vista isn't set up to read it
)

// Status describes one source for the dashboard. Detail is safe to show.
type Status struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	State     State      `json:"state"`
	Detail    string     `json:"detail,omitempty"`
	CheckedAt *time.Time `json:"checked_at"`
	LastOK    *time.Time `json:"last_ok"`
}

// fetchTimeout bounds one call to a source.
const fetchTimeout = 5 * time.Second

// Cached fetches a value at most once per ttl and keeps the last good one
// when a fetch fails. Concurrent callers share one fetch.
type Cached[T any] struct {
	id, name string
	ttl      time.Duration
	fetch    func(context.Context) (T, error) // nil: not connected
	missing  string                           // why it's not connected
	log      *slog.Logger
	now      func() time.Time

	mu        sync.Mutex
	value     T
	have      bool
	lastOK    time.Time
	checkedAt time.Time
	lastErr   error
}

// NewCached returns a cache over fetch. A nil fetch is a source Vista can't
// read; missing says why.
func NewCached[T any](id, name string, ttl time.Duration, fetch func(context.Context) (T, error), missing string, log *slog.Logger) *Cached[T] {
	return &Cached[T]{id: id, name: name, ttl: ttl, fetch: fetch, missing: missing, log: log, now: time.Now}
}

// Get returns the value, whether there is one, and the source's status.
func (c *Cached[T]) Get(ctx context.Context) (T, bool, Status) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fetch == nil {
		var zero T
		return zero, false, Status{ID: c.id, Name: c.name, State: NotConnected, Detail: c.missing}
	}
	now := c.now()
	if c.checkedAt.IsZero() || now.Sub(c.checkedAt) >= c.ttl {
		// The fetch outlives a cancelled request, so one closed tab doesn't
		// count as the source failing.
		fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), fetchTimeout)
		v, err := c.fetch(fctx)
		cancel()
		c.checkedAt = c.now()
		switch {
		case err == nil:
			if c.lastErr != nil {
				c.log.Info("source answering again", "component", "source", "source", c.id)
			}
			c.value, c.have, c.lastOK, c.lastErr = v, true, c.checkedAt, nil
		case c.lastErr == nil:
			c.log.Warn("source failed", "component", "source", "source", c.id, "err", err)
			c.lastErr = err
		default:
			c.lastErr = err
		}
	}
	return c.value, c.have, c.status()
}

func (c *Cached[T]) status() Status {
	s := Status{ID: c.id, Name: c.name, State: OK}
	if !c.checkedAt.IsZero() {
		t := c.checkedAt.UTC()
		s.CheckedAt = &t
	}
	if c.have {
		t := c.lastOK.UTC()
		s.LastOK = &t
	}
	if c.lastErr != nil {
		s.Detail = c.lastErr.Error()
		s.State = Unavailable
		if c.have {
			s.State = Stale
		}
	}
	return s
}
