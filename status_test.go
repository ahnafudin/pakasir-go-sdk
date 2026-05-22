package pakasir

import "testing"

func TestStatus(t *testing.T) {
	tests := []struct {
		s        Status
		name     string
		terminal bool
		known    bool
	}{
		{StatusPending, "pending", false, true},
		{StatusCompleted, "completed", true, true},
		{StatusCancelled, "cancelled", true, true},
		{StatusExpired, "expired", true, true},
		{"", "empty", false, false},
		{"failed", "failed", false, false},
		{"refunded", "refunded", false, false},
		{"COMPLETED", "COMPLETED", false, false},
		{"completed ", "completed_trailing_space", false, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.s.IsTerminal(); got != tc.terminal {
				t.Errorf("Status(%q).IsTerminal() = %v, want %v", tc.s, got, tc.terminal)
			}
			if got := tc.s.IsKnown(); got != tc.known {
				t.Errorf("Status(%q).IsKnown() = %v, want %v", tc.s, got, tc.known)
			}
		})
	}
}
