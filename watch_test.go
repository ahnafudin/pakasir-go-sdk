package pakasir

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// detailJSON builds a {"transaction":{...}} response body with the given status.
func detailJSON(status Status) string {
	return `{"transaction":{"project":"slug","order_id":"O1","amount":1000,` +
		`"status":"` + string(status) + `","payment_method":"qris",` +
		`"completed_at":""}}`
}

func detailJSONCompleted() string {
	return `{"transaction":{"project":"slug","order_id":"O1","amount":1000,` +
		`"status":"completed","payment_method":"qris",` +
		`"completed_at":"2024-09-10T08:07:02+07:00"}}`
}

// watchTestClient builds a *Client pointing at the given test server.
func watchTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	return New("slug", "key", WithBaseURL(srv.URL), WithTimeout(2*time.Second))
}

// drainWatch reads all events from ch until it closes, returning them in order.
// Bounded by maxWait to prevent test hangs.
func drainWatch(t *testing.T, ch <-chan WatchEvent, maxWait time.Duration) []WatchEvent {
	t.Helper()
	var events []WatchEvent
	timer := time.NewTimer(maxWait)
	defer timer.Stop()
	for {
		select {
		case evt, ok := <-ch:
			if !ok {
				return events
			}
			events = append(events, evt)
		case <-timer.C:
			t.Fatalf("watch did not close within %v; got %d events so far", maxWait, len(events))
			return events
		}
	}
}

func TestWatch_TerminalImmediately(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, detailJSONCompleted())
	}))
	defer srv.Close()

	c := watchTestClient(t, srv)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	events := drainWatch(t, c.Watch(ctx, "O1", 1000, WithInterval(25*time.Millisecond)), 2*time.Second)
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(events), events)
	}
	if events[0].Err != nil {
		t.Errorf("unexpected err: %v", events[0].Err)
	}
	if events[0].Payment == nil || events[0].Payment.Status != StatusCompleted {
		t.Errorf("status = %v, want completed", events[0].Payment)
	}
}

func TestWatch_PendingThenCompleted(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := hits.Add(1)
		if n < 3 {
			_, _ = io.WriteString(w, detailJSON(StatusPending))
		} else {
			_, _ = io.WriteString(w, detailJSONCompleted())
		}
	}))
	defer srv.Close()

	c := watchTestClient(t, srv)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	events := drainWatch(t, c.Watch(ctx, "O1", 1000, WithInterval(25*time.Millisecond)), 2*time.Second)
	if len(events) < 2 {
		t.Fatalf("got %d events, want >=2: %+v", len(events), events)
	}
	// First event is pending.
	if events[0].Payment == nil || events[0].Payment.Status != StatusPending {
		t.Errorf("first event = %+v, want pending", events[0])
	}
	// Last event is completed (terminal).
	last := events[len(events)-1]
	if last.Payment == nil || last.Payment.Status != StatusCompleted {
		t.Errorf("last event = %+v, want completed", last)
	}
}

func TestWatch_StatusDedup(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := hits.Add(1)
		if n < 5 {
			_, _ = io.WriteString(w, detailJSON(StatusPending))
		} else {
			_, _ = io.WriteString(w, detailJSONCompleted())
		}
	}))
	defer srv.Close()

	c := watchTestClient(t, srv)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	events := drainWatch(t, c.Watch(ctx, "O1", 1000, WithInterval(25*time.Millisecond)), 2*time.Second)
	// Expect at most 2 events (pending, then completed). Server returned
	// pending 4 times — dedup should collapse to 1 emission for pending.
	if len(events) > 2 {
		t.Errorf("got %d events, want <=2 (dedup failed): %+v", len(events), events)
	}
	if len(events) < 2 {
		t.Fatalf("got %d events, want 2 (pending+completed)", len(events))
	}
	if events[0].Payment.Status != StatusPending {
		t.Errorf("event[0].status = %s, want pending", events[0].Payment.Status)
	}
	if events[1].Payment.Status != StatusCompleted {
		t.Errorf("event[1].status = %s, want completed", events[1].Payment.Status)
	}
}

func TestWatch_TransientErrorContinues(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := hits.Add(1)
		switch {
		case n == 1:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"message":"boom"}`)
		case n == 2:
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			_, _ = io.WriteString(w, detailJSONCompleted())
		}
	}))
	defer srv.Close()

	c := watchTestClient(t, srv)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	events := drainWatch(t, c.Watch(ctx, "O1", 1000, WithInterval(25*time.Millisecond)), 2*time.Second)
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1 (terminal after transient): %+v", len(events), events)
	}
	if events[0].Err != nil {
		t.Errorf("unexpected err: %v", events[0].Err)
	}
	if events[0].Payment.Status != StatusCompleted {
		t.Errorf("status = %s", events[0].Payment.Status)
	}
}

func TestWatch_404IsTransient(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := hits.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, detailJSONCompleted())
	}))
	defer srv.Close()

	c := watchTestClient(t, srv)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	events := drainWatch(t, c.Watch(ctx, "O1", 1000, WithInterval(25*time.Millisecond)), 2*time.Second)
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1 (terminal after 404 transient): %+v", len(events), events)
	}
	if events[0].Err != nil {
		t.Errorf("unexpected err (404 should be transient): %v", events[0].Err)
	}
}

func TestWatch_PermanentErrorEmitsAndCloses(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"message":"bad order_id"}`)
	}))
	defer srv.Close()

	c := watchTestClient(t, srv)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	events := drainWatch(t, c.Watch(ctx, "O1", 1000, WithInterval(25*time.Millisecond)), 2*time.Second)
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1 (single permanent err)", len(events))
	}
	if events[0].Err == nil {
		t.Fatal("expected permanent err event, got payment")
	}
	var apiErr *APIError
	if !errors.As(events[0].Err, &apiErr) {
		t.Errorf("expected *APIError, got %T: %v", events[0].Err, events[0].Err)
	} else if apiErr.StatusCode != 400 {
		t.Errorf("status = %d, want 400", apiErr.StatusCode)
	}
}

func TestWatch_CtxCancelClosesSilently(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, detailJSON(StatusPending))
	}))
	defer srv.Close()

	c := watchTestClient(t, srv)
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()

	events := drainWatch(t, c.Watch(ctx, "O1", 1000, WithInterval(25*time.Millisecond)), 2*time.Second)
	// May contain 0..N pending events before ctx fires. None should be Err.
	for i, e := range events {
		if e.Err != nil {
			t.Errorf("event[%d] has err on ctx-cancel: %v", i, e.Err)
		}
	}
}

func TestWatch_ValidationStillWorks(t *testing.T) {
	// Even though Watch validates via DetailPayment, the loop should
	// emit a permanent err on validation failure and close.
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, detailJSONCompleted())
	}))
	defer srv.Close()

	c := watchTestClient(t, srv)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Empty order_id triggers ErrInvalidOrderID inside DetailPayment.
	events := drainWatch(t, c.Watch(ctx, "", 1000, WithInterval(25*time.Millisecond)), 2*time.Second)
	if len(events) != 1 || events[0].Err == nil {
		t.Fatalf("want 1 err event, got %d: %+v", len(events), events)
	}
	if !errors.Is(events[0].Err, ErrInvalidOrderID) {
		t.Errorf("err = %v, want ErrInvalidOrderID", events[0].Err)
	}
}

func TestWatch_DefaultInterval(t *testing.T) {
	t.Parallel()
	cfg := &watchConfig{interval: defaultWatchInterval}
	if cfg.interval != 3*time.Second {
		t.Errorf("default interval = %v, want 3s", cfg.interval)
	}
}

func TestWithInterval_ZeroIgnored(t *testing.T) {
	t.Parallel()
	cfg := &watchConfig{interval: defaultWatchInterval}
	WithInterval(0)(cfg)
	if cfg.interval != defaultWatchInterval {
		t.Errorf("interval = %v, want default %v", cfg.interval, defaultWatchInterval)
	}
}

func TestWithInterval_NegativeIgnored(t *testing.T) {
	t.Parallel()
	cfg := &watchConfig{interval: defaultWatchInterval}
	WithInterval(-1 * time.Second)(cfg)
	if cfg.interval != defaultWatchInterval {
		t.Errorf("interval = %v, want default", cfg.interval)
	}
}

func TestIsCtxError(t *testing.T) {
	t.Parallel()
	if !isCtxError(context.Canceled) {
		t.Error("Canceled should be ctx error")
	}
	if !isCtxError(context.DeadlineExceeded) {
		t.Error("DeadlineExceeded should be ctx error")
	}
	if isCtxError(errors.New("other")) {
		t.Error("plain error should not be ctx error")
	}
}

func TestIsTransientWatchError_5xx(t *testing.T) {
	t.Parallel()
	cases := []int{500, 502, 503, 504}
	for _, code := range cases {
		err := &APIError{StatusCode: code}
		if !isTransientWatchError(err) {
			t.Errorf("status %d should be transient", code)
		}
	}
}

func TestIsTransientWatchError_404(t *testing.T) {
	t.Parallel()
	if !isTransientWatchError(&APIError{StatusCode: 404}) {
		t.Error("404 should be transient (not-yet-indexed)")
	}
}

func TestIsTransientWatchError_4xxNon404(t *testing.T) {
	t.Parallel()
	for _, code := range []int{400, 401, 403, 422} {
		if isTransientWatchError(&APIError{StatusCode: code}) {
			t.Errorf("status %d should be PERMANENT", code)
		}
	}
}

func TestIsTransientWatchError_Unknown(t *testing.T) {
	t.Parallel()
	if isTransientWatchError(errors.New("decode failed")) {
		t.Error("unknown errors should be permanent")
	}
}
