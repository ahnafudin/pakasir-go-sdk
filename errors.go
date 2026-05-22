package pakasir

import (
	"errors"
	"fmt"
)

// APIError represents a non-2xx HTTP response from Pakasir.
// Callers can retrieve it via errors.As.
type APIError struct {
	// StatusCode is the HTTP status code (e.g., 400, 404, 500).
	StatusCode int
	// Status is the HTTP status text (e.g., "Not Found").
	Status string
	// Message is the best-effort decoded error message from the response body.
	// Empty when the body could not be decoded or was empty.
	Message string
	// Raw holds the raw response body for debugging purposes.
	Raw []byte
}

// Error returns a human-readable description of the API error.
// Format: "pakasir: HTTP <code> <status>" or
// "pakasir: HTTP <code> <status>: <message>" when Message is non-empty.
func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("pakasir: HTTP %d %s", e.StatusCode, e.Status)
	}
	return fmt.Sprintf("pakasir: HTTP %d %s: %s", e.StatusCode, e.Status, e.Message)
}

// Sentinel errors returned by SDK functions. All are wrappable; use errors.Is
// or errors.As to inspect wrapped errors.
var (
	// ErrInvalidOrderID is returned when an order_id argument is empty or
	// otherwise invalid before a request is made.
	ErrInvalidOrderID = errors.New("pakasir: invalid order_id")

	// ErrInvalidAmount is returned when an amount argument is not a positive
	// integer before a request is made.
	ErrInvalidAmount = errors.New("pakasir: invalid amount")

	// ErrInvalidMethod is returned when a payment method argument is not one
	// of the documented Method constants.
	ErrInvalidMethod = errors.New("pakasir: invalid payment method")

	// ErrEmptyBody is returned by ParseWebhook when the supplied byte slice is
	// nil or zero-length.
	ErrEmptyBody = errors.New("pakasir: empty webhook body")

	// ErrBodyTooLarge is returned by ParseWebhookRequest when the request body
	// exceeds the configured size limit (default 1 MiB).
	ErrBodyTooLarge = errors.New("pakasir: webhook body too large")

	// ErrAmountMismatch is returned by Event.Verify when the webhook amount
	// does not equal the merchant's expected amount.
	ErrAmountMismatch = errors.New("pakasir: amount mismatch")

	// ErrOrderIDMismatch is returned by Event.Verify when the webhook order_id
	// does not equal the merchant's expected order ID. When both fields
	// mismatch, ErrOrderIDMismatch is returned (order_id is checked first).
	ErrOrderIDMismatch = errors.New("pakasir: order_id mismatch")
)
