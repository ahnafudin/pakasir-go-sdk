package pakasir

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultMaxBodySize = 1 << 20 // 1 MiB

// Event is the decoded form of a Pakasir webhook payload.
//
// The SDK does not track idempotency — the merchant's database is the
// source of truth. After parsing, validate Event via Match (bool) or
// Verify (error-returning), then process side effects idempotently on
// (OrderID, Status).
type Event struct {
	// Project is the Pakasir project slug that triggered this webhook.
	Project string
	// OrderID is the merchant-supplied transaction identifier.
	OrderID string
	// Amount is the transaction amount in the smallest currency unit.
	Amount int64
	// Status is the current transaction status.
	Status Status
	// PaymentMethod is the payment method used for the transaction.
	PaymentMethod Method
	// CompletedAt is the time the transaction completed. Nil when Status is
	// not completed or the field was absent in the payload.
	CompletedAt *time.Time
	// IsSandbox distinguishes sandbox callbacks from production callbacks.
	IsSandbox bool
}

// eventDTO mirrors the Pakasir webhook wire format with JSON struct tags.
// It is decoded first and then converted to Event via toEvent.
type eventDTO struct {
	Project       string `json:"project"`
	OrderID       string `json:"order_id"`
	Amount        int64  `json:"amount"`
	Status        string `json:"status"`
	PaymentMethod string `json:"payment_method"`
	CompletedAt   string `json:"completed_at"`
	IsSandbox     bool   `json:"is_sandbox"`
}

// toEvent converts eventDTO to a fully typed Event. CompletedAt is parsed
// when non-empty; an empty string yields a nil *time.Time.
func (d *eventDTO) toEvent() (*Event, error) {
	evt := &Event{
		Project:       d.Project,
		OrderID:       d.OrderID,
		Amount:        d.Amount,
		Status:        Status(d.Status),
		PaymentMethod: Method(d.PaymentMethod),
		IsSandbox:     d.IsSandbox,
	}
	if d.CompletedAt != "" {
		t, err := parseTime(d.CompletedAt)
		if err != nil {
			return nil, fmt.Errorf("completed_at: %w", err)
		}
		evt.CompletedAt = &t
	}
	return evt, nil
}

// webhookConfig holds resolved options for ParseWebhookRequest.
type webhookConfig struct {
	maxBodySize int64
}

// WebhookOption configures webhook parsing.
type WebhookOption func(*webhookConfig)

// WithMaxBodySize sets the maximum webhook body size accepted by
// ParseWebhookRequest. Default 1 MiB. Bodies larger than this return
// ErrBodyTooLarge wrapped with the limit.
func WithMaxBodySize(bytes int64) WebhookOption {
	return func(c *webhookConfig) {
		if bytes > 0 {
			c.maxBodySize = bytes
		}
	}
}

// ParseWebhook decodes a webhook body. Returns ErrEmptyBody when body
// is empty or whitespace-only. Wrapped JSON / time errors otherwise.
//
// Caller is responsible for reading the body from the request
// (e.g., io.ReadAll(r.Body) or framework-specific accessor).
func ParseWebhook(body []byte) (*Event, error) {
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, ErrEmptyBody
	}
	var dto eventDTO
	if err := json.Unmarshal(body, &dto); err != nil {
		return nil, fmt.Errorf("pakasir: decode webhook body: %w", err)
	}
	return dto.toEvent()
}

// ParseWebhookRequest decodes from an *http.Request. Closes r.Body.
// Enforces a 1 MiB body size limit; override via WithMaxBodySize.
//
// Returns wrapped ErrBodyTooLarge if the body exceeds the limit.
func ParseWebhookRequest(r *http.Request, opts ...WebhookOption) (*Event, error) {
	cfg := &webhookConfig{maxBodySize: defaultMaxBodySize}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	limited := http.MaxBytesReader(nil, r.Body, cfg.maxBodySize)
	defer r.Body.Close()

	body, err := io.ReadAll(limited)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return nil, fmt.Errorf("pakasir: max %d: %w", cfg.maxBodySize, ErrBodyTooLarge)
		}
		return nil, fmt.Errorf("pakasir: read webhook body: %w", err)
	}

	return ParseWebhook(body)
}

// Match implements Pakasir's recommended validation: order_id and amount
// must match the merchant's record of the transaction. Returns true only
// when both match exactly.
//
// Use this on every received webhook. Reject mismatches with HTTP 403.
func (e *Event) Match(orderID string, expectedAmount int64) bool {
	return e.OrderID == orderID && e.Amount == expectedAmount
}

// Verify is the error-returning companion to Match. Returns ErrOrderIDMismatch
// or ErrAmountMismatch (wrappable, check with errors.Is) on failure, or nil
// when both match. Useful for structured logging or metrics where the
// specific cause matters.
//
// Order of checks: order_id first, then amount. A payload with BOTH
// fields mismatched returns ErrOrderIDMismatch only.
func (e *Event) Verify(orderID string, expectedAmount int64) error {
	if e.OrderID != orderID {
		return fmt.Errorf("got %q, want %q: %w", e.OrderID, orderID, ErrOrderIDMismatch)
	}
	if e.Amount != expectedAmount {
		return fmt.Errorf("got %d, want %d: %w", e.Amount, expectedAmount, ErrAmountMismatch)
	}
	return nil
}
