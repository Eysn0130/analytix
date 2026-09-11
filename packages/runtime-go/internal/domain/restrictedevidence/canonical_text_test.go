package restrictedevidence

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateCanonicalTextRejectsStructuredPrivateReferences(t *testing.T) {
	digest := strings.Repeat("a", 64)
	sha256 := strings.Repeat("b", 64)
	for _, text := range []string{
		"ordinary heading\npurpose: analytix.source-row-lineage/v1\n",
		"rawArtifactManifestDigest: " + digest + "\nrawArtifactManifestSha256: " + sha256 + "\nrawArtifactManifestByteLength: 42\n",
		"| sourceExactValue | 0012-3456789012345678 |\n| sourceExactValueSha256 | " + sha256 + " |\n| bindingDigest | " + digest + " |\n",
		"prefix {\"parsedPageDigest\":\"" + digest + "\",\"parsedPageSha256\":\"" + sha256 + "\",\"parsedPageByteLength\":42} suffix",
	} {
		if err := ValidateCanonicalText(text); !errors.Is(err, ErrRestrictedEvidence) {
			t.Fatalf("structured private reference reached canonical text: text=%q err=%v", text, err)
		}
	}
}

func TestValidateCanonicalTextRejectsEmbeddedTypedLocalResponsesWithNeutralCanaries(t *testing.T) {
	for _, test := range []struct {
		kind   string
		canary string
	}{
		{kind: "import_mapping_preview", canary: "NEUTRAL_TEXT_CANARY_IMPORT"},
		{kind: "cleaning_diff_preview", canary: "NEUTRAL_TEXT_CANARY_CLEANING"},
		{kind: "direct_source_preview", canary: "NEUTRAL_TEXT_CANARY_DIRECT"},
		{kind: "accepted_slot_display", canary: "NEUTRAL_TEXT_CANARY_ACCEPTED"},
	} {
		text := `prefix {"schemaVersion":1,"kind":"` + test.kind + `","nested":{"displayValue":"` + test.canary + `"}} suffix`
		if err := ValidateCanonicalText(text); !errors.Is(err, ErrRestrictedEvidence) {
			t.Fatalf("typed-local response reached canonical text: kind=%s err=%v", test.kind, err)
		}
	}
}

func TestValidateCanonicalTextAllowsDiscussionAndInvalidExamples(t *testing.T) {
	hash := strings.Repeat("a", 64)
	for _, text := range []string{
		"请讨论 analytix.source-row-lineage/v1 以及 lineageDigest 字段。",
		"rawArtifactManifestDigest: <digest>\nrawArtifactManifestSha256: <sha>\nrawArtifactManifestByteLength: 42\n",
		"reportSha256: " + hash + "\ndatasetSnapshotId: snapshot-v2\n",
		"purpose: analytix.source-row-producer-policy/v1\npolicyDigest: " + hash + "\n",
		"Please discuss typed-local-data-surface/v1 and direct_source_preview naming.",
	} {
		if err := ValidateCanonicalText(text); err != nil {
			t.Fatalf("ordinary canonical text was over-blocked: text=%q err=%v", text, err)
		}
	}
}

func TestValidateCanonicalTextRequiresPrivateFieldsInOneLocalWindow(t *testing.T) {
	digest := strings.Repeat("a", 64)
	sha256 := strings.Repeat("b", 64)
	text := "rawArtifactManifestDigest: " + digest + "\n" + strings.Repeat("x", canonicalTextWindow*2) +
		"\nrawArtifactManifestSha256: " + sha256 + "\nrawArtifactManifestByteLength: 42\n"
	if err := ValidateCanonicalText(text); err != nil {
		t.Fatalf("unrelated distant labels were treated as one private record: %v", err)
	}
}

func TestCanonicalTextValueParsersAreExact(t *testing.T) {
	hash := strings.Repeat("a", 64)
	if parsed, ok := canonicalTextValueAsSHA256("sha256:" + hash); !ok || parsed != hash {
		t.Fatalf("SHA parser mismatch: parsed=%q ok=%t", parsed, ok)
	}
	if parsed, ok := canonicalTextValueAsPositiveInteger("42"); !ok || parsed != 42 {
		t.Fatalf("integer parser mismatch: parsed=%d ok=%t", parsed, ok)
	}
}
