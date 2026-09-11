package reasoningmarkup

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestProviderIdentifierUnicodeDecodeMatchesSingleJSONCodeUnit(t *testing.T) {
	// Freeze the complete four-hex-digit contract, including lone surrogates.
	for value := 0; value <= 0xffff; value++ {
		escaped := fmt.Sprintf(`\u%04x`, value)
		quoted := fmt.Sprintf(`"\u%04x"`, value)
		var expected string
		if err := json.Unmarshal([]byte(quoted), &expected); err != nil {
			t.Fatal(err)
		}
		if actual := decodeProviderIdentifierUnicodeEscapes(escaped); actual != expected {
			t.Fatalf("code unit %04x decoded differently", value)
		}
	}
}

func TestProviderIdentifierUnicodeInvalidInputStaysLiteral(t *testing.T) {
	for _, value := range []string{`\u12`, `\uGGGG`, `\u+123`, `\u12"x`, `\u12\n`, `\u000g`, `plain`, `\U0001`} {
		if actual := decodeProviderIdentifierUnicodeEscapes(value); actual != value {
			t.Fatalf("invalid escape changed: %q", value)
		}
	}
	if actual := decodeProviderIdentifierUnicodeEscapes(`reason\u0069ng`); actual != "reasoning" {
		t.Fatal("escaped private field identifier must remain detectable")
	}
	if actual := decodeProviderIdentifierUnicodeEscapes(`\uD800\uDC00`); actual != strings.Repeat("\ufffd", 2) {
		t.Fatal("single-code-unit decoding must not silently change to surrogate-pair decoding")
	}
}
