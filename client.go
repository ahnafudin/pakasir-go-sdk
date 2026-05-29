package pakasir

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Version is the SDK version. Bumped on each release.
const Version = "1.0.1"

const (
	defaultBaseURL   = "https://app.pakasir.com"
	defaultTimeout   = 30 * time.Second
	defaultUserAgent = "pakasir-go-sdk/" + Version
)

// Client is a Pakasir API client. Safe for concurrent use by multiple
// goroutines after construction.
type Client struct {
	slug       string
	apiKey     string
	httpClient *http.Client
	baseURL    string
	userAgent  string
	logger     *slog.Logger
}

// Option configures a Client at construction time.
type Option func(*config)

// config is the resolved set of options passed to Client. Unexported by
// design — callers compose Options instead of constructing config directly.
type config struct {
	httpClient *http.Client
	timeout    time.Duration
	baseURL    string
	userAgent  string
	logger     *slog.Logger
}

// New creates a Client. slug and apiKey are required; nil/empty values are
// accepted (no panic) but every API call will then fail with the API
// returning an authentication error.
func New(slug, apiKey string, opts ...Option) *Client {
	cfg := &config{
		timeout:   defaultTimeout,
		baseURL:   defaultBaseURL,
		userAgent: defaultUserAgent,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	// Resolve HTTP client: a custom one overrides the timeout option per spec.
	hc := cfg.httpClient
	if hc == nil {
		hc = &http.Client{Timeout: cfg.timeout}
	}

	// Resolve logger: nil → discard.
	logger := cfg.logger
	if logger == nil {
		logger = slog.New(discardHandler{})
	}

	return &Client{
		slug:       slug,
		apiKey:     apiKey,
		httpClient: hc,
		baseURL:    strings.TrimRight(cfg.baseURL, "/"),
		userAgent:  cfg.userAgent,
		logger:     logger,
	}
}

// WithHTTPClient overrides the default *http.Client (which has a 30s timeout).
// Use this to inject a custom Transport — for example, to layer retries,
// tracing, metrics, or a corporate proxy in front of the SDK's requests.
//
// When WithHTTPClient is provided, WithTimeout is ignored: timeout management
// becomes the caller's responsibility on the provided client.
func WithHTTPClient(h *http.Client) Option {
	return func(c *config) {
		if h != nil {
			c.httpClient = h
		}
	}
}

// WithBaseURL overrides the API base. Default: "https://app.pakasir.com".
//
// NOT FOR SANDBOX SWITCHING. Pakasir does not publish a separate sandbox
// host; sandbox is enabled per-project in the Pakasir dashboard, and the
// webhook payload carries an is_sandbox flag (see Event.IsSandbox). Use
// this option only for proxying, mock servers (httptest.NewServer in
// tests), or self-hosted gateways.
//
// Any trailing slash is stripped.
func WithBaseURL(u string) Option {
	return func(c *config) {
		if u != "" {
			c.baseURL = u
		}
	}
}

// WithTimeout sets the HTTP client timeout (default 30s). Ignored when
// WithHTTPClient is also provided — see that option's doc.
//
// Zero or negative values are ignored.
func WithTimeout(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.timeout = d
		}
	}
}

// WithUserAgent overrides the User-Agent header.
// Default: "pakasir-go-sdk/<version>".
func WithUserAgent(ua string) Option {
	return func(c *config) {
		if ua != "" {
			c.userAgent = ua
		}
	}
}

// WithLogger sets a structured logger used by the SDK for non-fatal events
// (Watch poll failures, transient HTTP errors, body-size truncation).
// Defaults to a discard logger so the SDK is silent unless explicitly
// opted in.
//
// Zero-dep: log/slog is part of the standard library.
//
// The SDK does not log request or response bodies — never the api_key or PII.
func WithLogger(l *slog.Logger) Option {
	return func(c *config) {
		if l != nil {
			c.logger = l
		}
	}
}

// --- Internal: HTTP plumbing ---

// do performs an HTTP request and decodes a JSON response into out.
// For non-2xx responses, returns a *APIError with the raw body captured.
//
// Arguments:
//   - method: http.MethodGet, MethodPost, etc.
//   - path:   path beginning with "/", joined to c.baseURL.
//   - query:  optional url.Values appended as a query string; pass nil to
//     omit the query string entirely.
//   - body:   optional Go value JSON-encoded into the request body; pass
//     nil to omit. When non-nil, the Content-Type header is set.
//   - out:    optional pointer to a Go value into which a successful
//     response body is JSON-decoded; pass nil to discard.
//
// The Accept and User-Agent headers are always set.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	fullURL := c.baseURL + path
	if len(query) > 0 {
		fullURL += "?" + query.Encode()
	}

	var bodyReader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("pakasir: encode request body: %w", err)
		}
		bodyReader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return fmt.Errorf("pakasir: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("pakasir: do request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("pakasir: read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return newAPIError(resp.StatusCode, resp.Status, raw)
	}

	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("pakasir: decode response body: %w", err)
		}
	}
	return nil
}

// newAPIError constructs an *APIError, extracting a best-effort message
// from a JSON body. The raw body is defensively copied so callers may
// retain it after the underlying response Body is closed.
func newAPIError(statusCode int, status string, raw []byte) *APIError {
	rawCopy := append([]byte(nil), raw...)
	return &APIError{
		StatusCode: statusCode,
		Status:     status,
		Message:    extractMessage(raw),
		Raw:        rawCopy,
	}
}

// extractMessage tries to parse raw as a JSON object and returns the value
// of "message" or "error" if either is a string. Returns "" on any
// failure (non-JSON body, missing fields, wrong type) so the caller can
// fall back to the HTTP status text.
func extractMessage(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	if s, ok := m["message"].(string); ok && s != "" {
		return s
	}
	if s, ok := m["error"].(string); ok && s != "" {
		return s
	}
	return ""
}

// --- Internal: time parsing ---

// parseTime decodes a Pakasir timestamp string. Pakasir emits RFC3339 with
// timezone offset, sometimes with fractional seconds (e.g.
// "2024-09-10T08:07:02.819+07:00"). RFC3339Nano covers both shapes;
// RFC3339 is tried as a fallback for strict no-fractional inputs.
//
// Empty input returns the zero time and an error; callers should guard
// against empty strings before invoking.
func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, fmt.Errorf("pakasir: empty time string")
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("pakasir: parse time %q: %w", s, err)
	}
	return t, nil
}

// --- Internal: discardHandler (slog.Handler that drops everything) ---

// discardHandler is a slog.Handler that discards all records and reports
// itself as disabled. Used as the default logger when WithLogger is not
// provided. Zero-allocation in the hot path (no records are constructed
// because Enabled returns false).
type discardHandler struct{}

func (discardHandler) Enabled(context.Context, slog.Level) bool  { return false }
func (discardHandler) Handle(context.Context, slog.Record) error { return nil }
func (discardHandler) WithAttrs([]slog.Attr) slog.Handler        { return discardHandler{} }
func (discardHandler) WithGroup(string) slog.Handler             { return discardHandler{} }
