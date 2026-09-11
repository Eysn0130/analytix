package piiauthorization

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	testsecurity "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestControlledPIIArtifactPreservesAuthorizedAccountByteForByte(t *testing.T) {
	input := controlledPIIArtifactTestInput(t)
	artifact, err := NewControlledPIIArtifactV1(input)
	if err != nil {
		t.Fatal(err)
	}
	body, err := ControlledPIIArtifactV1Bytes(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(`"exactValue":"`+piiGrantTestAccount+`"`)) {
		t.Fatalf("controlled artifact did not preserve the exact account: %s", body)
	}
	for _, forbidden := range [][]byte{[]byte("****"), []byte("…"), []byte("...")} {
		if bytes.Contains(body, forbidden) {
			t.Fatalf("controlled artifact altered the exact account with %q: %s", forbidden, body)
		}
	}
	parsed, err := ParseControlledPIIArtifactV1(body)
	if err != nil || !reflect.DeepEqual(parsed, artifact) {
		t.Fatalf("controlled artifact strict roundtrip failed: parsed=%#v err=%v", parsed, err)
	}
	digest, err := ControlledPIIArtifactSHA256V1(parsed)
	if err != nil || digest != domainsecurity.SHA256Hex(body) {
		t.Fatalf("controlled artifact digest does not bind exact bytes: digest=%s err=%v", digest, err)
	}
	metadata, err := ControlledPIIArtifactMetadataFromBytesV1(body)
	if err != nil || metadata.SHA256 != digest || metadata.ByteLength != uint64(len(body)) ||
		metadata.Context != artifact.Context || metadata.ClaimLedgerDigest != artifact.ClaimLedgerDigest ||
		len(metadata.FieldBindings) != 1 || metadata.FieldBindings[0].ValueSHA256 != artifact.Fields[0].ValueSHA256 {
		t.Fatalf("controlled artifact metadata lost exact non-PII bindings: metadata=%#v err=%v", metadata, err)
	}
	metadataBody, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(metadataBody, []byte(piiGrantTestAccount)) {
		t.Fatalf("controlled artifact metadata leaked the exact account: %s", metadataBody)
	}

	tampered := artifact
	tampered.Fields = append([]ControlledPIIFieldV1(nil), artifact.Fields...)
	tampered.Fields[0].ExactValue = "6222020202020202021"
	if ValidateControlledPIIArtifactV1(tampered) == nil {
		t.Fatal("controlled artifact accepted an exact account mutation without its bound hash")
	}
}

func TestControlledPIIArtifactRejectsRenderBeforeContextIssue(t *testing.T) {
	input := controlledPIIArtifactTestInput(t)
	input.RenderedAt = time.Date(2026, 7, 16, 8, 59, 59, 0, time.UTC)
	if _, err := NewControlledPIIArtifactV1(input); err == nil {
		t.Fatal("controlled artifact accepted bytes rendered before the bound turn context")
	}
}

func TestControlledPIIArtifactStrictParserRejectsModelControlledAndNoncanonicalJSON(t *testing.T) {
	artifact, err := NewControlledPIIArtifactV1(controlledPIIArtifactTestInput(t))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := ControlledPIIArtifactV1Bytes(artifact)
	unknown := append([]byte(nil), body[:len(body)-1]...)
	unknown = append(unknown, []byte(`,"safeToAnswer":true}`)...)
	if _, err := ParseControlledPIIArtifactV1(unknown); err == nil {
		t.Fatal("controlled artifact accepted an unknown model-controlled property")
	}
	duplicate := append([]byte(`{"purpose":"`+ControlledPIIArtifactPurposeV1+`",`), body[1:]...)
	if _, err := ParseControlledPIIArtifactV1(duplicate); err == nil {
		t.Fatal("controlled artifact accepted a duplicate property")
	}
	if _, err := ParseControlledPIIArtifactV1(append(append([]byte(nil), body...), '\n')); err == nil {
		t.Fatal("controlled artifact accepted trailing noncanonical bytes")
	}
	var indented bytes.Buffer
	if err := json.Indent(&indented, body, "", "  "); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseControlledPIIArtifactV1(indented.Bytes()); err == nil {
		t.Fatal("controlled artifact accepted semantically equal noncanonical JSON")
	}
}

func TestControlledPIIArtifactRejectsCrossCaseLedgerRulesAndTargetBindings(t *testing.T) {
	input := controlledPIIArtifactTestInput(t)
	artifact, err := NewControlledPIIArtifactV1(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateControlledPIIArtifactForBindingsV1(
		artifact, input.SecurityContext, input.ClaimLedgerDigest, input.ProjectionRulesetHash, input.TargetIdentityDigest,
	); err != nil {
		t.Fatal(err)
	}
	otherContext, err := testsecurity.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-other-artifact", TurnID: "turn-other-artifact", WorkspaceRealPath: "/workspace/other-artifact",
		CaseID: "case-other-artifact", CaseBindingHash: domainsecurity.SHA256Hex([]byte("other-artifact-binding")),
		ContextEpoch: 4, IssuedAt: time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		context domainsecurity.TurnSecurityContext
		ledger  string
		rules   string
		target  string
	}{
		{"cross case context", otherContext, input.ClaimLedgerDigest, input.ProjectionRulesetHash, input.TargetIdentityDigest},
		{"other ledger", input.SecurityContext, domainsecurity.SHA256Hex([]byte("other-ledger")), input.ProjectionRulesetHash, input.TargetIdentityDigest},
		{"other rules", input.SecurityContext, input.ClaimLedgerDigest, domainsecurity.SHA256Hex([]byte("other-rules")), input.TargetIdentityDigest},
		{"other target", input.SecurityContext, input.ClaimLedgerDigest, input.ProjectionRulesetHash, domainsecurity.SHA256Hex([]byte("other-target"))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if ValidateControlledPIIArtifactForBindingsV1(artifact, test.context, test.ledger, test.rules, test.target) == nil {
				t.Fatal("controlled artifact crossed an exact publication binding")
			}
		})
	}
}

func TestControlledPIIArtifactConstructorCanonicalizesFieldsAndRejectsDuplicates(t *testing.T) {
	input := controlledPIIArtifactTestInput(t)
	deviceValue := "02:00:5e:10:00:00"
	input.Fields = append(input.Fields, ControlledPIIFieldV1{
		PIIClass: PIIClassDeviceIdentifierV1, ClaimID: "claim-device",
		ClaimRecordDigest: domainsecurity.SHA256Hex([]byte("claim-device-record")), ClaimType: domainevidence.ClaimDeviceIdentifier,
		FieldName: "deviceIdentifier", ExactValue: deviceValue, ValueSHA256: domainsecurity.SHA256Hex([]byte(deviceValue)),
		EvidenceReceiptIDs: []string{"evr_" + domainsecurity.SHA256Hex([]byte("device-evidence"))},
	})
	artifact, err := NewControlledPIIArtifactV1(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Fields) != 2 || artifact.Fields[0].PIIClass != PIIClassDeviceIdentifierV1 ||
		artifact.Fields[1].PIIClass != PIIClassFinancialAccountV1 || artifact.PreservedControlledFieldCount != 2 {
		t.Fatalf("controlled artifact fields are not canonical: %#v", artifact.Fields)
	}

	duplicate := controlledPIIArtifactTestInput(t)
	duplicate.Fields = append(duplicate.Fields, duplicate.Fields[0])
	if _, err := NewControlledPIIArtifactV1(duplicate); err == nil {
		t.Fatal("controlled artifact accepted a duplicate exact field")
	}

	unsorted := artifact
	unsorted.Fields = append([]ControlledPIIFieldV1(nil), artifact.Fields...)
	unsorted.Fields[0], unsorted.Fields[1] = unsorted.Fields[1], unsorted.Fields[0]
	if ValidateControlledPIIArtifactV1(unsorted) == nil {
		t.Fatal("controlled artifact accepted a noncanonical field order")
	}
}

func controlledPIIArtifactTestInput(t *testing.T) ControlledPIIArtifactInputV1 {
	t.Helper()
	fixture := newPIIGrantFixture(t)
	binding := fixture.input.FieldBindings[0]
	return ControlledPIIArtifactInputV1{
		SecurityContext: fixture.input.SecurityContext, ClaimLedgerDigest: fixture.input.ClaimLedgerDigest,
		ProjectionRulesetHash: fixture.input.ProjectionRulesetHash, TargetIdentityDigest: fixture.input.TargetIdentityDigest,
		Fields: []ControlledPIIFieldV1{{
			PIIClass: binding.PIIClass, ClaimID: binding.ClaimID, ClaimRecordDigest: binding.ClaimRecordDigest,
			ClaimType: binding.ClaimType, FieldName: binding.FieldName, ExactValue: piiGrantTestAccount,
			ValueSHA256: binding.ValueSHA256, EvidenceReceiptIDs: append([]string(nil), binding.EvidenceReceiptIDs...),
		}},
		RenderedAt: time.Date(2026, 7, 16, 9, 2, 0, 0, time.UTC),
	}
}
