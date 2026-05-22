package pakasir

// Method identifies a Pakasir payment method (QRIS, VA, etc.).
// Use one of the Method* constants — Method.IsValid() reports unknowns.
type Method string

const (
	MethodQRIS         Method = "qris"
	MethodBNIVA        Method = "bni_va"
	MethodBRIVA        Method = "bri_va"
	MethodCIMBVA       Method = "cimb_niaga_va"
	MethodMaybankVA    Method = "maybank_va"
	MethodPermataVA    Method = "permata_va"
	MethodBNCVA        Method = "bnc_va"
	MethodSampoernaVA  Method = "sampoerna_va"
	MethodATMBersamaVA Method = "atm_bersama_va"
	MethodArthaGrahaVA Method = "artha_graha_va"
)

// allMethods is the canonical ordered list of known payment methods.
// Order matches the const block above.
var allMethods = []Method{
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

// knownMethods provides O(1) lookup for IsValid.
var knownMethods = map[Method]struct{}{
	MethodQRIS:         {},
	MethodBNIVA:        {},
	MethodBRIVA:        {},
	MethodCIMBVA:       {},
	MethodMaybankVA:    {},
	MethodPermataVA:    {},
	MethodBNCVA:        {},
	MethodSampoernaVA:  {},
	MethodATMBersamaVA: {},
	MethodArthaGrahaVA: {},
}

// IsValid reports whether m is a known payment method. O(1) lookup.
func (m Method) IsValid() bool {
	_, ok := knownMethods[m]
	return ok
}

// AllMethods returns every known method in declaration order (helpful for
// validation or UI enumeration). Returns a new slice on each call; callers
// may safely modify the result.
func AllMethods() []Method {
	return append([]Method(nil), allMethods...)
}
