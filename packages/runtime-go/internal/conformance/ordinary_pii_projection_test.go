package conformance_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"unicode"

	domainprivacy "analytix.local/runtime-go/internal/domain/privacyprojection"
)

type ordinaryPIIConformanceFixture struct {
	SchemaVersion      int  `json:"schemaVersion"`
	SyntheticOnly      bool `json:"syntheticOnly"`
	UntrustedTextCases []struct {
		ID                 string `json:"id"`
		Input              string `json:"input"`
		Expected           string `json:"expected"`
		SensitiveCanonical string `json:"sensitiveCanonical"`
	} `json:"untrustedTextCases"`
}

func TestOrdinaryPIIProjectionV1SharedConformanceCorpus(t *testing.T) {
	raw, err := os.ReadFile("../../../runtime/src/conformance/fixtures/ordinary-pii-projection-v1.json")
	if err != nil {
		t.Fatalf("read shared ordinary PII corpus: %v", err)
	}
	var fixture ordinaryPIIConformanceFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("decode shared ordinary PII corpus: %v", err)
	}
	if fixture.SchemaVersion != 1 || !fixture.SyntheticOnly || len(fixture.UntrustedTextCases) == 0 {
		t.Fatalf("invalid shared ordinary PII corpus metadata: %#v", fixture)
	}
	for _, testCase := range fixture.UntrustedTextCases {
		t.Run(testCase.ID, func(t *testing.T) {
			projection := domainprivacy.ProjectText(testCase.Input)
			switch testCase.Expected {
			case "mask":
				if projection.Text == testCase.Input || len(projection.Findings) == 0 {
					t.Fatalf("restricted synthetic value was not projected: %#v", projection)
				}
				if testCase.SensitiveCanonical != "" && strings.Contains(canonicalDecimalDigits(projection.Text), testCase.SensitiveCanonical) {
					t.Fatalf("projection retained the complete identifier digits: %q", projection.Text)
				}
			case "preserve":
				if projection.Text != testCase.Input || len(projection.Findings) != 0 {
					t.Fatalf("safe synthetic measure was modified: %#v", projection)
				}
			default:
				t.Fatalf("unknown corpus expectation %q", testCase.Expected)
			}
		})
	}
}

func canonicalDecimalDigits(value string) string {
	var digits strings.Builder
	for _, character := range value {
		if unicode.IsDigit(character) {
			if character >= '0' && character <= '9' {
				digits.WriteRune(character)
				continue
			}
			// The corpus uses ASCII and fullwidth decimal digits. Restricting this
			// helper to that closed set keeps the conformance assertion deterministic.
			if character >= '\uff10' && character <= '\uff19' {
				digits.WriteByte(byte('0' + character - '\uff10'))
			}
		}
	}
	return digits.String()
}
