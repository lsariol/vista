package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// maxBody bounds what Vista reads from a source.
const maxBody = 8 << 20

// apiClient calls one service's JSON API with a bearer token.
type apiClient struct {
	service  string // "Sonar", "Blackbox"
	base     string
	token    string
	tokenKey string // the Cove key the service checks Vista's token against
	http     *http.Client
}

func newAPIClient(service, base, token, tokenKey string) *apiClient {
	return &apiClient{service: service, base: base, token: token, tokenKey: tokenKey,
		http: &http.Client{Timeout: 10 * time.Second}} // callers set tighter deadlines
}

// ErrNotFound is a 404 from a source.
var ErrNotFound = errors.New("not found")

// getJSON decodes GET base+path?query into out.
func (c *apiClient) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	u := c.base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("%s: %w", c.service, err)
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		var netErr net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
			return fmt.Errorf("%s didn't answer in time; try a narrower search, or check its logs", c.service)
		}
		return fmt.Errorf("%s can't be reached at %s: is it running and on the spark network?", c.service, c.base)
	}
	defer resp.Body.Close()
	body := io.LimitReader(resp.Body, maxBody)
	switch resp.StatusCode {
	case http.StatusOK:
		if err := json.NewDecoder(body).Decode(out); err != nil {
			return fmt.Errorf("%s: unreadable answer from %s: %v", c.service, path, err)
		}
		return nil
	case http.StatusUnauthorized:
		return fmt.Errorf("%s refused Vista's token: add the vista consumer to %s, with its token in Cove as %s, then redeploy both", c.service, c.service, c.tokenKey)
	case http.StatusForbidden:
		return fmt.Errorf("%s: Vista's token doesn't have the scope %s needs; add it to the vista consumer in %s", c.service, path, c.service)
	case http.StatusNotFound:
		return fmt.Errorf("%s %s: %w", c.service, path, ErrNotFound)
	case http.StatusServiceUnavailable:
		return fmt.Errorf("%s answered 503: %s", c.service, errorMessage(body, "its database is unavailable or behind on migrations"))
	default:
		return fmt.Errorf("%s answered %s for %s: %s", c.service, resp.Status, path, errorMessage(body, "no detail"))
	}
}

// errorMessage pulls a short message out of a JSON error body.
func errorMessage(body io.Reader, fallback string) string {
	var e struct {
		Error   any    `json:"error"`
		Message string `json:"message"`
	}
	raw, _ := io.ReadAll(io.LimitReader(body, 4096))
	if json.Unmarshal(raw, &e) == nil {
		msg := e.Message
		if m, ok := e.Error.(map[string]any); ok && msg == "" {
			msg, _ = m["message"].(string)
		}
		if s, ok := e.Error.(string); ok && msg == "" {
			msg = s
		}
		if msg = strings.TrimSpace(msg); msg != "" {
			if len(msg) > 200 {
				msg = msg[:200] + "…"
			}
			return msg
		}
	}
	return fallback
}
