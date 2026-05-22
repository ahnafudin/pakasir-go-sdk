package pakasir

import (
	"errors"
	"fmt"
	"testing"
)

func TestAPIError_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  *APIError
		want string
	}{
		{
			name: "no message",
			err:  &APIError{StatusCode: 404, Status: "Not Found", Message: ""},
			want: "pakasir: HTTP 404 Not Found",
		},
		{
			name: "with message",
			err:  &APIError{StatusCode: 400, Status: "Bad Request", Message: "bad input"},
			want: "pakasir: HTTP 400 Bad Request: bad input",
		},
		{
			name: "server error with message",
			err:  &APIError{StatusCode: 500, Status: "Internal Server Error", Message: "server died"},
			want: "pakasir: HTTP 500 Internal Server Error: server died",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.err.Error()
			if got != tc.want {
				t.Errorf("APIError.Error() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAPIError_ErrorsAs(t *testing.T) {
	t.Parallel()

	wrapped := fmt.Errorf("wrapped: %w", &APIError{StatusCode: 500, Status: "Internal Server Error"})

	var apiErr *APIError
	if !errors.As(wrapped, &apiErr) {
		t.Fatal("errors.As should unwrap APIError")
	}
	if apiErr.StatusCode != 500 {
		t.Errorf("StatusCode = %d, want 500", apiErr.StatusCode)
	}
}

func TestSentinelErrors_NonNilAndMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{"ErrInvalidOrderID", ErrInvalidOrderID, "pakasir: invalid order_id"},
		{"ErrInvalidAmount", ErrInvalidAmount, "pakasir: invalid amount"},
		{"ErrInvalidMethod", ErrInvalidMethod, "pakasir: invalid payment method"},
		{"ErrEmptyBody", ErrEmptyBody, "pakasir: empty webhook body"},
		{"ErrBodyTooLarge", ErrBodyTooLarge, "pakasir: webhook body too large"},
		{"ErrAmountMismatch", ErrAmountMismatch, "pakasir: amount mismatch"},
		{"ErrOrderIDMismatch", ErrOrderIDMismatch, "pakasir: order_id mismatch"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.err == nil {
				t.Fatalf("%s is nil, want non-nil sentinel", tc.name)
			}
			if got := tc.err.Error(); got != tc.wantMsg {
				t.Errorf("%s.Error() = %q, want %q", tc.name, got, tc.wantMsg)
			}
		})
	}
}

func TestSentinelErrors_WrappingIsMatchable(t *testing.T) {
	t.Parallel()

	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrInvalidOrderID", ErrInvalidOrderID},
		{"ErrInvalidAmount", ErrInvalidAmount},
		{"ErrInvalidMethod", ErrInvalidMethod},
		{"ErrEmptyBody", ErrEmptyBody},
		{"ErrBodyTooLarge", ErrBodyTooLarge},
		{"ErrAmountMismatch", ErrAmountMismatch},
		{"ErrOrderIDMismatch", ErrOrderIDMismatch},
	}

	for _, tc := range sentinels {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			wrapped := fmt.Errorf("ctx: %w", tc.err)
			if !errors.Is(wrapped, tc.err) {
				t.Errorf("errors.Is(wrapped %s) = false, want true", tc.name)
			}
		})
	}
}

func TestSentinelErrors_Distinct(t *testing.T) {
	t.Parallel()

	// Each pair (a, b): errors.Is(a, b) must be false.
	pairs := []struct {
		nameA, nameB string
		a, b         error
	}{
		{"ErrInvalidOrderID", "ErrInvalidAmount", ErrInvalidOrderID, ErrInvalidAmount},
		{"ErrInvalidOrderID", "ErrInvalidMethod", ErrInvalidOrderID, ErrInvalidMethod},
		{"ErrInvalidOrderID", "ErrEmptyBody", ErrInvalidOrderID, ErrEmptyBody},
		{"ErrInvalidOrderID", "ErrBodyTooLarge", ErrInvalidOrderID, ErrBodyTooLarge},
		{"ErrInvalidOrderID", "ErrAmountMismatch", ErrInvalidOrderID, ErrAmountMismatch},
		{"ErrInvalidOrderID", "ErrOrderIDMismatch", ErrInvalidOrderID, ErrOrderIDMismatch},
		{"ErrAmountMismatch", "ErrOrderIDMismatch", ErrAmountMismatch, ErrOrderIDMismatch},
		{"ErrEmptyBody", "ErrBodyTooLarge", ErrEmptyBody, ErrBodyTooLarge},
	}

	for _, tc := range pairs {
		tc := tc
		t.Run(tc.nameA+"_vs_"+tc.nameB, func(t *testing.T) {
			t.Parallel()
			if errors.Is(tc.a, tc.b) {
				t.Errorf("errors.Is(%s, %s) = true, want false (sentinels must be distinct)", tc.nameA, tc.nameB)
			}
		})
	}
}
