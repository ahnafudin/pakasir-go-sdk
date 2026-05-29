package pakasir

import (
	"context"
	"errors"
	"net"
	"net/url"
	"time"
)

const (
	defaultWatchInterval = 3 * time.Second
)

// WatchEvent is one observation from a Watch loop.
//
// When Err is non-nil, the poller treats the cause as a permanent failure
// and closes the channel immediately after delivering this event (the
// blocking-send guarantee). Transient failures — HTTP 5xx, HTTP 404
// (not yet indexed), and network errors — are NOT surfaced as
// WatchEvents; they are logged via the configured slog.Logger and the
// poller continues at the next interval.
//
// When Err is nil, Payment is the latest decoded response. Status dedup
// means consecutive polls returning the same status are collapsed into
// a single emission (the first one).
type WatchEvent struct {
	Payment *Payment
	Err     error
}

// WatchOption configures a Watch loop.
type WatchOption func(*watchConfig)

type watchConfig struct {
	interval time.Duration
}

// WithInterval sets the poll cadence (default 3s).
//
// Values below 1s are accepted but strongly discouraged in production —
// you will likely trigger upstream rate limits and contribute to platform
// load. The SDK does not clamp the value so tests can use short
// intervals (e.g., 25ms) against httptest servers.
func WithInterval(d time.Duration) WatchOption {
	return func(c *watchConfig) {
		if d > 0 {
			c.interval = d
		}
	}
}

// Watch polls DetailPayment until the status becomes terminal, a
// permanent error occurs, or ctx is cancelled. The returned channel
// closes exactly once when polling stops.
//
// Polling semantics (also in spec §5.3):
//   - Initial poll fires immediately at t=0; subsequent polls every
//     interval.
//   - Channel is buffered, size 1.
//   - Status dedup: a WatchEvent is emitted only when the status
//     differs from the prior emission. The first successful poll
//     always emits.
//   - Intermediate events use a best-effort send (drops if the consumer
//     hasn't read the prior event yet). Terminal events and permanent
//     errors use a blocking send with ctx.Done() escape — guaranteed
//     delivery unless the caller has already cancelled ctx.
//   - Channel is closed exactly once before the goroutine returns.
//
// To bound the watch duration, use context.WithTimeout on the parent
// ctx — there is no separate WithTimeout WatchOption.
func (c *Client) Watch(
	ctx context.Context,
	orderID string,
	amount int64,
	opts ...WatchOption,
) <-chan WatchEvent {
	cfg := &watchConfig{interval: defaultWatchInterval}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	ch := make(chan WatchEvent, 1)
	go c.watchLoop(ctx, orderID, amount, cfg.interval, ch)
	return ch
}

// watchLoop drives the polling. ch is always closed before this
// goroutine returns, exactly once.
func (c *Client) watchLoop(
	ctx context.Context,
	orderID string,
	amount int64,
	interval time.Duration,
	ch chan<- WatchEvent,
) {
	defer close(ch)

	var lastStatus Status
	firstEmit := true

	// pollOnce returns (terminal, abort, ctxDone).
	//   - terminal: a terminal status was observed and emitted.
	//   - abort:    a permanent error was emitted; the loop must exit.
	//   - ctxDone:  ctx was cancelled mid-poll; exit silently.
	pollOnce := func() (terminal, abort, ctxDone bool) {
		payment, err := c.DetailPayment(ctx, orderID, amount)
		if err != nil {
			if isCtxError(err) {
				return false, false, true
			}
			if isTransientWatchError(err) {
				c.logger.Debug("pakasir.Watch: transient error",
					"order_id", orderID,
					"amount", amount,
					"err", err.Error(),
				)
				return false, false, false
			}
			// Permanent: guaranteed-delivery send, then close.
			sendBlocking(ctx, ch, WatchEvent{Err: err})
			return false, true, false
		}

		// Success. Apply dedup.
		if firstEmit || payment.Status != lastStatus {
			firstEmit = false
			lastStatus = payment.Status
			evt := WatchEvent{Payment: payment}
			if payment.Status.IsTerminal() {
				sendBlocking(ctx, ch, evt)
				return true, false, false
			}
			sendNonBlocking(ch, evt)
		}
		return false, false, false
	}

	// Initial poll at t=0.
	if terminal, abort, ctxDone := pollOnce(); terminal || abort || ctxDone {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if terminal, abort, ctxDone := pollOnce(); terminal || abort || ctxDone {
				return
			}
		}
	}
}

// sendBlocking guarantees delivery of evt unless ctx is cancelled first.
func sendBlocking(ctx context.Context, ch chan<- WatchEvent, evt WatchEvent) {
	select {
	case ch <- evt:
	case <-ctx.Done():
	}
}

// sendNonBlocking attempts a send and drops the event if the buffer is full.
func sendNonBlocking(ch chan<- WatchEvent, evt WatchEvent) {
	select {
	case ch <- evt:
	default:
	}
}

// isCtxError reports whether err is a context cancellation or deadline.
// Such errors propagate through net/http and never indicate a Pakasir-side
// failure — the caller is exiting, so the watch loop exits silently.
func isCtxError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// isTransientWatchError reports whether err should cause the watch loop
// to log + continue (true) versus emit + close (false).
//
// Transient (true):
//   - HTTP 5xx (server-side fault, retry expected)
//   - HTTP 404 (transaction may not yet be indexed by Pakasir)
//   - Network errors (net.Error, url.Error from transport)
//
// Permanent (false): HTTP 4xx other than 404, malformed responses,
// unknown errors. Context errors are NOT classified here — see
// isCtxError, which short-circuits before this is called.
func isTransientWatchError(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.StatusCode >= 500:
			return true
		case apiErr.StatusCode == 404:
			return true
		default:
			return false
		}
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var urlErr *url.Error
	return errors.As(err, &urlErr)
}
