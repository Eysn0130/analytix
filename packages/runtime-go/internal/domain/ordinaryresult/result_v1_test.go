package ordinaryresult

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestResultSlotV1RetainsHistoricalIdentityAndProjectsNewSourceProse(t *testing.T) {
	const source = "Review /Users/private-owner/SYNTHETIC-PII.csv"
	current, err := NewResultSlotV1(source)
	if err != nil || current.Text != "Review [PRIVATE_PATH]" {
		t.Fatalf("new output retained private prose: %v", err)
	}
	legacy := current
	legacy.Text = source
	legacy.TextSHA256 = domainsecurity.SHA256Hex([]byte(legacy.Text))
	legacy.ResultDigest = resultSlotDigestV1(legacy)
	before, _ := json.Marshal(legacy)
	if err := ValidateResultSlotV1(legacy); err != nil {
		t.Fatalf("current output rule invalidated original v1 identity: %v", err)
	}
	after, _ := json.Marshal(legacy)
	if !bytes.Equal(before, after) {
		t.Fatal("historical result identity was rewritten")
	}
	for _, unsafe := range []string{"phone 13800138000", "Authorization: Bearer sk-private-token-1234"} {
		legacy.Text = unsafe
		legacy.TextSHA256 = domainsecurity.SHA256Hex([]byte(unsafe))
		legacy.ResultDigest = resultSlotDigestV1(legacy)
		if ValidateResultSlotV1(legacy) == nil {
			t.Fatal("historical identity check lost its existing privacy gate")
		}
	}
}

func TestResultSlotV1ProjectsReasoningCredentialsAndPIIAtFixedPoint(t *testing.T) {
	slot, err := NewResultSlotV1("public<think>PRIVATE_REASONING</think> phone 13800138000 Authorization: Bearer sk-private-token-1234")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"PRIVATE_REASONING", "13800138000", "sk-private-token-1234", "<think>"} {
		if strings.Contains(slot.Text, forbidden) {
			t.Fatalf("ordinary result retained %q: %#v", forbidden, slot)
		}
	}
	if err := ValidateResultSlotV1(slot); err != nil {
		t.Fatal(err)
	}
}

func TestResultSlotV1RejectsCaseFactsAndInternalReferences(t *testing.T) {
	if _, err := NewResultSlotV1("张某实际控制甲公司，涉案金额为 2645472 元。"); !errors.Is(err, ErrResultSlotProtectedFactV1) {
		t.Fatalf("protected fact error = %v", err)
	}
	reference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewResultSlotV1("ordinary result for " + string(reference)); !errors.Is(err, ErrResultSlotInternalReferenceV1) {
		t.Fatalf("internal reference error = %v", err)
	}
}

func TestHostFixedResultSlotRejectsArbitraryText(t *testing.T) {
	if _, err := NewHostFixedResultSlotV1("arbitrary provider prose"); err == nil {
		t.Fatal("arbitrary prose was relabeled as host fixed")
	}
	slot, err := NewHostFixedResultSlotV1(HostFixedProviderResultWithheldTextV1)
	if err != nil || slot.CandidateOrigin != ResultSlotOriginHostFixedV1 {
		t.Fatalf("host-fixed slot = %#v err=%v", slot, err)
	}
}

func TestResultSlotV1RejectsCanonicalPrivateEvidenceText(t *testing.T) {
	digest := strings.Repeat("a", 64)
	sha256 := strings.Repeat("b", 64)
	_, err := NewResultSlotV1("rawArtifactManifestDigest: " + digest +
		"\nrawArtifactManifestSha256: " + sha256 + "\nrawArtifactManifestByteLength: 42\n")
	if !errors.Is(err, ErrResultSlotProjectionV1) {
		t.Fatalf("canonical private evidence error = %v", err)
	}
}

func TestResultSlotV1RejectsSourceRowEvidenceReferencesRawAndEscaped(t *testing.T) {
	for name, reference := range map[string]string{
		"raw":     "srow1_" + strings.Repeat("a", 64),
		"escaped": "srow1_" + strings.Repeat(`\u0061`, 64),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewResultSlotV1("ordinary effect copied funds evidence " + reference); !errors.Is(err, ErrResultSlotInternalReferenceV1) {
				t.Fatalf("source-row evidence reference reached an ordinary result: %v", err)
			}
		})
	}
}

func TestResultSlotV1StrictParseRejectsTampering(t *testing.T) {
	slot, err := NewResultSlotV1("Updated the Go test and all focused checks pass.")
	if err != nil {
		t.Fatal(err)
	}
	value := ResultSlotV1Map(slot)
	value["text"] = "tampered"
	if _, err := ParseResultSlotV1(value); err == nil {
		t.Fatal("tampered ordinary result slot parsed")
	}
	value = ResultSlotV1Map(slot)
	value["unknown"] = true
	if _, err := ParseResultSlotV1(value); err == nil {
		t.Fatal("unknown ordinary result slot field parsed")
	}
	for _, field := range []string{"evidenceAuthority", "citationAuthority", "factAnswerAllowed"} {
		value = ResultSlotV1Map(slot)
		delete(value, field)
		if _, err := ParseResultSlotV1(value); err == nil {
			t.Fatalf("missing closed flag %q parsed", field)
		}
	}
	body, err := json.Marshal(slot)
	if err != nil {
		t.Fatal(err)
	}
	duplicate := bytes.Replace(body, []byte(`"purpose":`), []byte(`"purpose":"analytix.ordinary-result/v1","purpose":`), 1)
	var decoded ResultSlotV1
	if err := json.Unmarshal(duplicate, &decoded); err == nil {
		t.Fatal("duplicate ordinary result slot field parsed")
	}
}
