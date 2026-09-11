package turnsecurity

import (
	"errors"
	"testing"
)

func TestCurrentFailureCodePreservesOnlyClosedSecurityCodes(t *testing.T) {
	for _, code := range []string{
		"turn_security_workspace_mismatch",
		"turn_security_case_binding_mismatch",
		"turn_security_dataset_snapshot_mismatch",
		"turn_security_risk_policy_mismatch",
	} {
		if got := CurrentFailureCode(errors.New(code)); got != code {
			t.Fatalf("failure code %q projected as %q", code, got)
		}
	}
	for _, err := range []error{nil, errors.New("provider details must not escape"), errors.New("turn_security_workspace_mismatch_extra")} {
		if got := CurrentFailureCode(err); got != "turn_security_context_invalid" {
			t.Fatalf("untrusted failure leaked as %q for %v", got, err)
		}
	}
}
