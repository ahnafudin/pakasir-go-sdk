package pakasir

import "testing"

// TestMethodIsValid_KnownMethods verifies that IsValid returns true for every
// value returned by AllMethods.
func TestMethodIsValid_KnownMethods(t *testing.T) {
	for _, m := range AllMethods() {
		t.Run(string(m), func(t *testing.T) {
			if !m.IsValid() {
				t.Errorf("IsValid() = false for known method %q; want true", m)
			}
		})
	}
}

// TestMethodIsValid_UnknownValues verifies that IsValid returns false for
// values that are not declared constants (including case-sensitive mismatches
// and values with trailing whitespace).
func TestMethodIsValid_UnknownValues(t *testing.T) {
	tests := []struct {
		name   string
		method Method
	}{
		{name: "empty string", method: ""},
		{name: "bca_va", method: "bca_va"},
		{name: "unknown", method: "unknown"},
		{name: "QRIS uppercase", method: "QRIS"},
		{name: "qris trailing space", method: "qris "},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.method.IsValid() {
				t.Errorf("IsValid() = true for unknown method %q; want false", tc.method)
			}
		})
	}
}

// TestAllMethods_Count verifies that AllMethods returns exactly 10 entries.
func TestAllMethods_Count(t *testing.T) {
	const want = 10
	got := len(AllMethods())
	if got != want {
		t.Errorf("len(AllMethods()) = %d; want %d", got, want)
	}
}

// TestAllMethods_Order verifies that AllMethods returns entries in the same
// order as the const declarations.
func TestAllMethods_Order(t *testing.T) {
	want := []Method{
		MethodQRIS,
		MethodBNIVA,
		MethodBRIVA,
		MethodCIMBVA,
		MethodMaybankVA,
		MethodPermataVA,
		MethodBNCVA,
		MethodSampoernaVA,
		MethodATMBersamaVA,
		MethodArthaGrahaVA,
	}
	got := AllMethods()
	if len(got) != len(want) {
		t.Fatalf("AllMethods() len = %d; want %d", len(got), len(want))
	}
	for i, m := range want {
		t.Run(string(m), func(t *testing.T) {
			if got[i] != m {
				t.Errorf("AllMethods()[%d] = %q; want %q", i, got[i], m)
			}
		})
	}
}

// TestAllMethods_DefensiveCopy verifies that mutating the returned slice does
// not affect subsequent calls to AllMethods.
func TestAllMethods_DefensiveCopy(t *testing.T) {
	first := AllMethods()
	first[0] = "MUTATED"
	second := AllMethods()
	if second[0] != MethodQRIS {
		t.Errorf("AllMethods returned shared slice — got %q, want %q", second[0], MethodQRIS)
	}
}

// TestAllMethods_AgreeWithIsValid verifies that every entry from AllMethods
// satisfies IsValid.
func TestAllMethods_AgreeWithIsValid(t *testing.T) {
	for _, m := range AllMethods() {
		t.Run(string(m), func(t *testing.T) {
			if !m.IsValid() {
				t.Errorf("AllMethods() contains %q but IsValid() = false", m)
			}
		})
	}
}
