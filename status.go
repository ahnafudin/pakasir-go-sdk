package pakasir

// Status represents a Pakasir transaction status.
//
// Only StatusCompleted is officially documented by Pakasir; the other three
// (StatusPending, StatusCancelled, StatusExpired) are inferred from community
// SDKs (zeative/pakasir-sdk, H0llyW00dzZ/pakasir-go-sdk) and field
// observation. They are expected to be stable but may evolve — use IsKnown
// to detect future additions.
type Status string

const (
	// StatusPending indicates the transaction is awaiting payment.
	StatusPending Status = "pending"
	// StatusCompleted indicates the transaction was paid successfully.
	StatusCompleted Status = "completed"
	// StatusCancelled indicates the transaction was cancelled before payment.
	StatusCancelled Status = "cancelled"
	// StatusExpired indicates the transaction window elapsed without payment.
	StatusExpired Status = "expired"
)

// IsTerminal reports whether s indicates a final outcome (no further
// status changes are expected). Returns false for unknown statuses —
// when Status.IsKnown() is false, the caller must treat the value
// defensively (e.g., log + continue polling).
func (s Status) IsTerminal() bool {
	switch s {
	case StatusCompleted, StatusCancelled, StatusExpired:
		return true
	default:
		return false
	}
}

// IsKnown reports whether s is one of the documented status values.
// Returns false for statuses Pakasir may add in the future.
func (s Status) IsKnown() bool {
	switch s {
	case StatusPending, StatusCompleted, StatusCancelled, StatusExpired:
		return true
	default:
		return false
	}
}
