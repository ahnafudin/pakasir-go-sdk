package pakasir

// Status represents a Pakasir transaction status.
//
// StatusCompleted is the status formally guaranteed by Pakasir; the other
// three (StatusPending, StatusCancelled, StatusExpired) are observed in
// practice and treated as stable, but may evolve over time — use IsKnown to
// detect any future additions returned by the API.
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
