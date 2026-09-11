package security

import "testing"

func TestLogicalEffectIsClosedToHostOwnedEffectClasses(t *testing.T) {
	for _, effect := range []LogicalEffect{
		LogicalEffectOrdinary,
		LogicalEffectCaseData,
		LogicalEffectFundsData,
	} {
		if err := ValidateLogicalEffect(effect); err != nil {
			t.Fatalf("valid logical effect %q was rejected: %v", effect, err)
		}
	}
	for _, effect := range []LogicalEffect{"", "case", "funds", " ordinary", "ordinary "} {
		if err := ValidateLogicalEffect(effect); err == nil {
			t.Fatalf("unrecognized logical effect %q was accepted", effect)
		}
	}
}
