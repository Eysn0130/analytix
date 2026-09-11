package caseentity

import (
	"math"
	"testing"

	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
)

func TestModelEntityAliasV1ClosedGrammar(t *testing.T) {
	tests := []struct {
		entityType string
		ordinal    uint32
		want       string
	}{
		{domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1, 1, "acct:1"},
		{domaincontrolledaccount.ControlledAccountFinancialFieldBankCardNumberV1, math.MaxUint32, "card:4294967295"},
	}
	for _, test := range tests {
		alias, err := NewModelEntityAliasV1(test.entityType, test.ordinal)
		if err != nil || string(alias) != test.want {
			t.Fatalf("model alias mismatch: alias=%q err=%v", alias, err)
		}
		entityType, ordinal, err := ParseModelEntityAliasV1(string(alias))
		if err != nil || entityType != test.entityType || ordinal != test.ordinal {
			t.Fatalf("model alias roundtrip mismatch: type=%q ordinal=%d err=%v", entityType, ordinal, err)
		}
	}
}

func TestModelEntityAliasV1RejectsUnsupportedAndNonCanonicalValues(t *testing.T) {
	for _, value := range []string{
		"", "acct:0", "acct:01", "acct:+1", "acct:-1", "acct:4294967296",
		"card:0", "card:01", "account:1", "person:1", "org:1", "phone:1",
		"device:1", "merchant:1", "acct:1 ", " acct:1", "ACCT:1", "acct:１",
		"acct:1#suffix", "acct:1****", "acct:sha256:deadbeef", "card:ciphertext", "card:****1234",
	} {
		if ValidateModelEntityAliasV1(value) == nil {
			t.Fatalf("invalid model alias accepted: %q", value)
		}
	}
	if _, err := NewModelEntityAliasV1("person", 1); err == nil {
		t.Fatal("unsupported entity type produced a model alias")
	}
}
