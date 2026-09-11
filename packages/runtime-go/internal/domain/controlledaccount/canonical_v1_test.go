package controlledaccount

import (
	"strings"
	"testing"
)

func TestCanonicalFinancialAccountTextV2PreservesDigitsAcrossAllowedSpelling(t *testing.T) {
	tests := map[string]struct {
		source string
		want   string
	}{
		"digits":        {source: "6222021234567890", want: "6222021234567890"},
		"separators":    {source: "6222-0212 3456-7890", want: "6222021234567890"},
		"leading zeros": {source: "0012-3456 7890", want: "001234567890"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := CanonicalFinancialAccountTextV2(test.source)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatal("canonical financial account did not preserve the expected digits")
			}
		})
	}
}

func TestCanonicalFinancialAccountTextV2RejectsLossyOrAmbiguousSpellingWithoutEcho(t *testing.T) {
	tests := map[string]string{
		"empty":              "",
		"leading separator":  "-6222021234567890",
		"trailing separator": "6222021234567890 ",
		"double separator":   "6222--021234567890",
		"mixed separators":   "6222- 021234567890",
		"decimal":            "6222021234567890.0",
		"scientific":         "6.22202123456789e15",
		"full-width":         "６２２２０２１２３４５６７８９０",
		"non-breaking space": "6222\u00a0021234567890",
		"over byte limit":    strings.Repeat("1", maxCanonicalFinancialAccountBytesV2+1),
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := CanonicalFinancialAccountTextV2(source)
			if err == nil || got != "" {
				t.Fatal("invalid financial account spelling was accepted")
			}
			if source != "" && strings.Contains(err.Error(), source) {
				t.Fatal("canonicalization error exposed the source-exact value")
			}
		})
	}
}

func TestContainsCompleteFinancialAccountCandidateV2IsNarrowAndCanonicalizerBound(t *testing.T) {
	for _, value := range []string{
		"account 6222021234567890123",
		"card 6222-0212 3456-7890",
		"malformed prefix 1--6222021234567890",
	} {
		if !ContainsCompleteFinancialAccountCandidateV2(value) {
			t.Fatalf("complete supported account shape was not detected: %q", value)
		}
	}
	for _, value := range []string{
		"invoice 1234567",
		"arbitrary free text without a supported account shape",
		"unicode digits ６２２２０２１２３４５６７８９０",
	} {
		if ContainsCompleteFinancialAccountCandidateV2(value) {
			t.Fatalf("unsupported free text was classified as a complete account: %q", value)
		}
	}
}
