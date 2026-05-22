package pakasir

import (
	"errors"
	"testing"
)

func FuzzParseWebhook(f *testing.F) {
	seeds := []string{
		// Valid:
		`{"project":"s","order_id":"O","amount":1,"status":"completed","payment_method":"qris","completed_at":"2024-01-01T00:00:00Z","is_sandbox":false}`,
		`{"project":"s","order_id":"O","amount":1,"status":"pending","payment_method":"qris","completed_at":"","is_sandbox":true}`,
		// Edge:
		``,
		`   `,
		`{`,
		`{"order_id":null}`,
		`{"amount":"not-a-number"}`,
		`{"completed_at":"not-a-date"}`,
		`[]`,
		`null`,
		`{"order_id":" "}`,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		// Must not panic. Any error is acceptable.
		// If ParseWebhook returns nil error, event must be non-nil.
		evt, err := ParseWebhook(body)
		if err == nil && evt == nil {
			t.Errorf("nil event with nil error for body %q", body)
		}
		// ErrEmptyBody is allowed for empty/whitespace bodies.
		_ = errors.Is(err, ErrEmptyBody)
	})
}
