package caseentity

import (
	"strings"
	"testing"
)

func TestReferenceV1EncodesKeyedDigestWithProviderSafeAlphabet(t *testing.T) {
	digest := strings.Repeat("0123456789abcdef", 4)
	want := "cer1_" + strings.Repeat("abcdefghijklmnop", 4)

	reference, err := NewReferenceV1FromKeyedDigest(digest)
	if err != nil {
		t.Fatal(err)
	}
	if string(reference) != want {
		t.Fatalf("reference encoding mismatch: got %q want %q", reference, want)
	}
	if err := ValidateReferenceV1(string(reference)); err != nil {
		t.Fatal(err)
	}
	for _, current := range string(reference)[len(ReferencePrefixV1):] {
		if current < 'a' || current > 'p' {
			t.Fatalf("reference suffix left the closed provider-safe alphabet: %q", reference)
		}
	}
}

func TestReferenceV1RejectsNonCanonicalDigestAndReferenceSyntax(t *testing.T) {
	for name, digest := range map[string]string{
		"short":     strings.Repeat("0", 63),
		"long":      strings.Repeat("0", 65),
		"uppercase": strings.Repeat("A", 64),
		"non-hex":   strings.Repeat("g", 64),
	} {
		t.Run("digest "+name, func(t *testing.T) {
			if reference, err := NewReferenceV1FromKeyedDigest(digest); err == nil || reference != "" {
				t.Fatalf("invalid keyed digest minted a reference: reference=%q err=%v", reference, err)
			}
		})
	}

	valid := "cer1_" + strings.Repeat("a", 64)
	for name, reference := range map[string]string{
		"short":        valid[:len(valid)-1],
		"wrong prefix": "cer2_" + strings.Repeat("a", 64),
		"decimal":      "cer1_" + strings.Repeat("a", 63) + "0",
		"uppercase":    "cer1_" + strings.Repeat("a", 63) + "A",
		"outside":      "cer1_" + strings.Repeat("a", 63) + "q",
	} {
		t.Run("reference "+name, func(t *testing.T) {
			if err := ValidateReferenceV1(reference); err == nil {
				t.Fatalf("invalid reference syntax was accepted: %q", reference)
			}
		})
	}
}

func TestReferenceCandidateV1SeparatesForgedTokensFromOrdinaryCodeIdentifiers(t *testing.T) {
	for _, value := range []string{
		"cer1_" + strings.Repeat("a", 64),
		"prefix cer1_" + strings.Repeat("deadbeef", 8) + " suffix",
		"prefix cer1_" + strings.Repeat("aaaa\u200b", 16) + " suffix",
		"prefix c.e.r.1_" + strings.Repeat("dead-beef-", 8) + " suffix",
		"prefix ＣＥＲ１＿" + strings.Repeat("Ａ", 64) + " suffix",
		"prefix 𝐜𝐞𝐫𝟏_" + strings.Repeat("𝐚", 64) + " suffix",
		"prefix ⓒⓔⓡ①＿" + strings.Repeat("ⓐ", 64) + " suffix",
		"prefix c\u034fe\u200br1_" + strings.Repeat("aaaa-aaaa-", 8) + " suffix",
		"cer1_" + strings.Repeat("deadbeef", 8) + " cer1_" + strings.Repeat("cafebabe", 8),
	} {
		if !ContainsReferenceCandidateV1(value) {
			t.Fatalf("reference-like token was not detected: %q", value)
		}
		masked, found := MaskReferenceCandidatesV1(value)
		if !found || ContainsReferenceCandidateV1(masked) || !strings.Contains(masked, "[ACCOUNT]") {
			t.Fatalf("reference-like token was not withheld by the shared scanner: raw=%q masked=%q found=%t", value, masked, found)
		}
	}
	for _, value := range []string{
		`ReferencePrefixV1 = "cer1_"`,
		"cer1_parser",
		"cer1_decoder",
		"cer1_callback",
		"cer1_deadbeef",
		"𝚌𝚎𝚛𝟷_𝚌𝚊𝚕𝚕𝚋𝚊𝚌𝚔",
	} {
		if ContainsReferenceCandidateV1(value) {
			t.Fatalf("ordinary code identifier was treated as a case reference: %q", value)
		}
		if masked, found := MaskReferenceCandidatesV1(value); found || masked != value {
			t.Fatalf("ordinary code identifier was changed: raw=%q masked=%q found=%t", value, masked, found)
		}
	}
	for prefixLength := 1; prefixLength < referenceCandidateFloorV1; prefixLength++ {
		value := "cer1_" + strings.Repeat("a", prefixLength) + " cer1_" + strings.Repeat("deadbeef", 8)
		masked, found := MaskReferenceCandidatesV1(value)
		if !found || ContainsReferenceCandidateV1(masked) || strings.Contains(masked, ReferencePrefixV1) {
			t.Fatalf("candidate masking was not closed over a short leading prefix: length=%d raw=%q masked=%q found=%t", prefixLength, value, masked, found)
		}
	}
}
