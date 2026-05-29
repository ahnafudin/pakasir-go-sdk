package pakasir

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestNew_Defaults(t *testing.T) {
	t.Parallel()
	c := New("slug", "key")
	if c == nil {
		t.Fatal("New returned nil")
	}
	if c.slug != "slug" || c.apiKey != "key" {
		t.Errorf("credentials: slug=%q apiKey=%q", c.slug, c.apiKey)
	}
	if c.baseURL != defaultBaseURL {
		t.Errorf("baseURL = %q, want %q", c.baseURL, defaultBaseURL)
	}
	if c.userAgent != defaultUserAgent {
		t.Errorf("userAgent = %q, want %q", c.userAgent, defaultUserAgent)
	}
	if c.httpClient == nil {
		t.Fatal("httpClient nil")
	}
	if c.httpClient.Timeout != defaultTimeout {
		t.Errorf("timeout = %v, want %v", c.httpClient.Timeout, defaultTimeout)
	}
	if c.logger == nil {
		t.Fatal("logger nil")
	}
}

func TestNew_NilOptionIgnored(t *testing.T) {
	t.Parallel()
	// Should not panic.
	c := New("s", "k", nil, WithUserAgent("custom"), nil)
	if c.userAgent != "custom" {
		t.Errorf("userAgent = %q, want %q", c.userAgent, "custom")
	}
}

func TestNew_WithBaseURL_TrimsTrailingSlash(t *testing.T) {
	t.Parallel()
	c := New("s", "k", WithBaseURL("https://example.com/"))
	if c.baseURL != "https://example.com" {
		t.Errorf("baseURL = %q, want stripped", c.baseURL)
	}
}

func TestNew_WithBaseURL_EmptyIgnored(t *testing.T) {
	t.Parallel()
	c := New("s", "k", WithBaseURL(""))
	if c.baseURL != defaultBaseURL {
		t.Errorf("baseURL = %q, want default", c.baseURL)
	}
}

func TestNew_WithTimeout(t *testing.T) {
	t.Parallel()
	c := New("s", "k", WithTimeout(5*time.Second))
	if c.httpClient.Timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", c.httpClient.Timeout)
	}
}

func TestNew_WithTimeout_ZeroIgnored(t *testing.T) {
	t.Parallel()
	c := New("s", "k", WithTimeout(0))
	if c.httpClient.Timeout != defaultTimeout {
		t.Errorf("zero timeout overrode default; got %v", c.httpClient.Timeout)
	}
}

func TestNew_WithHTTPClient_OverridesTimeout(t *testing.T) {
	t.Parallel()
	custom := &http.Client{Timeout: 7 * time.Second}
	c := New("s", "k", WithTimeout(99*time.Second), WithHTTPClient(custom))
	if c.httpClient != custom {
		t.Error("WithHTTPClient did not install the custom client")
	}
	if c.httpClient.Timeout != 7*time.Second {
		t.Errorf("custom client timeout mutated: %v", c.httpClient.Timeout)
	}
}

func TestNew_WithHTTPClient_NilIgnored(t *testing.T) {
	t.Parallel()
	c := New("s", "k", WithHTTPClient(nil))
	if c.httpClient == nil {
		t.Fatal("nil custom client overrode default")
	}
}

func TestNew_WithUserAgent(t *testing.T) {
	t.Parallel()
	c := New("s", "k", WithUserAgent("MyApp/1.0"))
	if c.userAgent != "MyApp/1.0" {
		t.Errorf("userAgent = %q", c.userAgent)
	}
}

func TestNew_WithLogger(t *testing.T) {
	t.Parallel()
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(buf, nil))
	c := New("s", "k", WithLogger(logger))
	if c.logger != logger {
		t.Error("logger not installed")
	}
	c.logger.Info("hello")
	if !strings.Contains(buf.String(), "hello") {
		t.Errorf("logger silent: %q", buf.String())
	}
}

func TestDiscardHandler(t *testing.T) {
	t.Parallel()
	h := discardHandler{}
	if h.Enabled(context.Background(), slog.LevelError) {
		t.Error("discardHandler.Enabled = true; want false")
	}
	if err := h.Handle(context.Background(), slog.Record{}); err != nil {
		t.Errorf("Handle returned error: %v", err)
	}
	if _, ok := h.WithAttrs(nil).(discardHandler); !ok {
		t.Error("WithAttrs did not return discardHandler")
	}
	if _, ok := h.WithGroup("g").(discardHandler); !ok {
		t.Error("WithGroup did not return discardHandler")
	}
}

func TestDo_Success_DecodesBody(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != defaultUserAgent {
			t.Errorf("User-Agent = %q", got)
		}
		// No body expected on GET
		if r.Header.Get("Content-Type") != "" {
			t.Errorf("Content-Type set on GET-with-nil-body: %q", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"ok": true, "n": 42}`)
	}))
	defer srv.Close()

	c := New("s", "k", WithBaseURL(srv.URL))
	var out struct {
		OK bool `json:"ok"`
		N  int  `json:"n"`
	}
	if err := c.do(context.Background(), http.MethodGet, "/foo", nil, nil, &out); err != nil {
		t.Fatalf("do err: %v", err)
	}
	if !out.OK || out.N != 42 {
		t.Errorf("decoded incorrectly: %+v", out)
	}
}

func TestDo_PostSendsJSONBody(t *testing.T) {
	t.Parallel()
	type req struct {
		A int    `json:"a"`
		B string `json:"b"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var got req
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if got.A != 1 || got.B != "two" {
			t.Errorf("body = %+v", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New("s", "k", WithBaseURL(srv.URL))
	if err := c.do(context.Background(), http.MethodPost, "/x", nil, req{A: 1, B: "two"}, nil); err != nil {
		t.Fatalf("do err: %v", err)
	}
}

func TestDo_AppendsQueryString(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("k"); got != "v with space" {
			t.Errorf("query k = %q, want %q", got, "v with space")
		}
		if got := r.URL.Query().Get("n"); got != "42" {
			t.Errorf("query n = %q", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New("s", "k", WithBaseURL(srv.URL))
	q := url.Values{"k": {"v with space"}, "n": {"42"}}
	if err := c.do(context.Background(), http.MethodGet, "/q", q, nil, nil); err != nil {
		t.Fatalf("do err: %v", err)
	}
}

func TestDo_APIError_ExtractsMessageField(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"message": "bad input"}`)
	}))
	defer srv.Close()

	c := New("s", "k", WithBaseURL(srv.URL))
	err := c.do(context.Background(), http.MethodGet, "/x", nil, nil, nil)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As did not unwrap *APIError: %v", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("StatusCode = %d, want 400", apiErr.StatusCode)
	}
	if apiErr.Message != "bad input" {
		t.Errorf("Message = %q, want %q", apiErr.Message, "bad input")
	}
	if string(apiErr.Raw) != `{"message": "bad input"}` {
		t.Errorf("Raw = %q", string(apiErr.Raw))
	}
}

func TestDo_APIError_ExtractsErrorField(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error": "not found"}`)
	}))
	defer srv.Close()

	c := New("s", "k", WithBaseURL(srv.URL))
	err := c.do(context.Background(), http.MethodGet, "/x", nil, nil, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("not APIError: %v", err)
	}
	if apiErr.Message != "not found" {
		t.Errorf("Message = %q", apiErr.Message)
	}
}

func TestDo_APIError_NonJSONBody(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "<html>500</html>")
	}))
	defer srv.Close()

	c := New("s", "k", WithBaseURL(srv.URL))
	err := c.do(context.Background(), http.MethodGet, "/x", nil, nil, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("not APIError: %v", err)
	}
	if apiErr.Message != "" {
		t.Errorf("Message should be empty for non-JSON body, got %q", apiErr.Message)
	}
	if string(apiErr.Raw) != "<html>500</html>" {
		t.Errorf("Raw = %q", string(apiErr.Raw))
	}
}

func TestDo_NetworkError_NotAPIError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	srv.Close() // close before request

	c := New("s", "k", WithBaseURL(srv.URL), WithTimeout(200*time.Millisecond))
	err := c.do(context.Background(), http.MethodGet, "/x", nil, nil, nil)
	if err == nil {
		t.Fatal("want error from closed server")
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		t.Errorf("network error wrongly classified as APIError: %v", err)
	}
}

func TestDo_MalformedJSONResponse(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "{not json")
	}))
	defer srv.Close()

	c := New("s", "k", WithBaseURL(srv.URL))
	var out struct {
		Foo string `json:"foo"`
	}
	err := c.do(context.Background(), http.MethodGet, "/x", nil, nil, &out)
	if err == nil {
		t.Fatal("want decode error")
	}
	if !strings.Contains(err.Error(), "decode response body") {
		t.Errorf("err = %v; want decode error", err)
	}
}

func TestDo_ContextCancel(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New("s", "k", WithBaseURL(srv.URL))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := c.do(ctx, http.MethodGet, "/x", nil, nil, nil)
	if err == nil {
		t.Fatal("want ctx error")
	}
}

func TestParseTime(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		input   string
		wantErr bool
		want    string // RFC3339Nano of expected, "" if don't care or err
	}{
		{"with-fractional", "2024-09-10T08:07:02.819+07:00", false, "2024-09-10T08:07:02.819+07:00"},
		{"no-fractional", "2024-09-10T08:07:02+07:00", false, "2024-09-10T08:07:02+07:00"},
		{"utc-z", "2024-09-10T01:07:02Z", false, "2024-09-10T01:07:02Z"},
		{"empty", "", true, ""},
		{"malformed", "not-a-date", true, ""},
		{"no-timezone", "2024-09-10T08:07:02", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseTime(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("parseTime(%q) want err, got %v", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseTime(%q) err: %v", tc.input, err)
			}
			if tc.want != "" && got.Format(time.RFC3339Nano) != tc.want {
				t.Errorf("parseTime(%q) = %v, want %v", tc.input, got.Format(time.RFC3339Nano), tc.want)
			}
		})
	}
}

func TestExtractMessage(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"message-field", `{"message":"x"}`, "x"},
		{"error-field", `{"error":"y"}`, "y"},
		{"both-prefers-message", `{"message":"m","error":"e"}`, "m"},
		{"empty-message-falls-through", `{"message":"","error":"e"}`, "e"},
		{"non-string-message", `{"message":123}`, ""},
		{"empty-body", ``, ""},
		{"non-json", `<html>500</html>`, ""},
		{"array-body", `["a","b"]`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := extractMessage([]byte(tc.raw))
			if got != tc.want {
				t.Errorf("extractMessage(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// Compile-time check: discardHandler satisfies slog.Handler.
var _ slog.Handler = discardHandler{}
