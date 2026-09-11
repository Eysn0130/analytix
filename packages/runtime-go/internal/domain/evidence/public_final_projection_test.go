package evidence

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestAcceptedFinalPublicViewProjectsOnlyBoundedMaskedMetadata(t *testing.T) {
	privateRecord := acceptedFinalPublicViewBoundaryFixture(t, "BLOCKER WITH UNSAFE DETAILS")
	view, err := NewAcceptedFinalPublicViewV2(privateRecord)
	if err != nil {
		t.Fatal(err)
	}
	if view.BlockerCode != AcceptedFinalBlockerCodeRedacted || view.CoverageStatus != AcceptedFinalCoverageUnavailable ||
		view.ReceiptMetadata.Projection != AcceptedFinalReceiptProjection || view.ReceiptMetadata.Count != 0 ||
		len(view.ReceiptMetadata.Citations) != 0 || view.AcceptedFinalDigest != privateRecord.AcceptedFinal.RecordDigest ||
		view.PublicViewDigest != privateRecord.AcceptedFinal.PublicViewDigest || view.EnvelopeDigest != privateRecord.Envelope.EnvelopeDigest {
		t.Fatalf("accepted-final public view mismatch: %#v", view)
	}
	body, _ := json.Marshal(view)
	for _, forbidden := range []string{"reconnect_source", "BLOCKER WITH UNSAFE DETAILS", "evidenceReceiptIds", "claims", "reasoning"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("public view leaked private envelope material %q: %s", forbidden, body)
		}
	}
	parsed := AcceptedFinalPublicViewRecordV2(view)
	if _, err := ParseAcceptedFinalPublicViewV2ForRecord(parsed, privateRecord.AcceptedFinal); err != nil {
		t.Fatalf("strict historical V2 expansion rejected its signed V5: %v", err)
	}
	parsed["providerSaysSafe"] = true
	if _, err := ParseAcceptedFinalPublicViewV2(parsed, privateRecord); err == nil {
		t.Fatal("accepted-final public view accepted an unknown property")
	}
	if _, err := ParseAcceptedFinalPublicViewV2ForRecord(parsed, privateRecord.AcceptedFinal); err == nil {
		t.Fatal("historical V2 expansion accepted an unknown property")
	}
	tampered := view
	tampered.ReceiptMetadata.Count = 1
	if err := ValidateAcceptedFinalPublicViewV2(tampered, privateRecord); err == nil {
		t.Fatal("accepted-final public view accepted metadata detached from private authority")
	}
	if _, err := NewAcceptedFinalPublicViewV1(privateRecord); err == nil {
		t.Fatal("unsigned accepted-final public view V1 remained live")
	}
}

func TestAcceptedFinalV5PublicViewCoreIsSigned(t *testing.T) {
	privateRecord := acceptedFinalPublicViewBoundaryFixture(t, "current_case_source_unavailable")
	original := privateRecord.AcceptedFinal

	detached := original
	detachedCore := *original.PublicView
	detachedCore.BlockerCode = "different_blocker"
	detached.PublicView = &detachedCore
	if ValidateAcceptedFinalRecord(detached) == nil {
		t.Fatal("accepted final allowed public core tampering with the original core digest")
	}

	rehashed := original
	rehashedCore := *original.PublicView
	rehashedCore.BlockerCode = "different_blocker"
	rehashed.PublicView = &rehashedCore
	rehashed.PublicViewDigest = acceptedFinalPublicViewCoreV2Digest(rehashedCore)
	if ValidateAcceptedFinalRecord(rehashed) == nil {
		t.Fatal("accepted final allowed a rehashed public core without a new installation-authority signature")
	}
}

func TestAcceptedFinalV5PublicViewCoreRejectsResignedMalformedSemantics(t *testing.T) {
	privateRecord := acceptedFinalPublicViewBoundaryFixture(t, "current_case_source_unavailable")
	privateKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))

	tests := map[string]func(*AcceptedFinalPublicViewCoreV2){
		"projection": func(core *AcceptedFinalPublicViewCoreV2) {
			core.ReceiptMetadata.Projection = "count_only"
		},
		"count": func(core *AcceptedFinalPublicViewCoreV2) {
			core.ReceiptMetadata.Count = 1
		},
		"citation handle": func(core *AcceptedFinalPublicViewCoreV2) {
			core.ReceiptMetadata.Count = 1
			core.ReceiptMetadata.Citations = []AcceptedFinalPublicCitationV1{{Handle: "raw-receipt", Label: "evidence-1"}}
		},
		"citation label": func(core *AcceptedFinalPublicViewCoreV2) {
			core.ReceiptMetadata.Count = 1
			core.ReceiptMetadata.Citations = []AcceptedFinalPublicCitationV1{{
				Handle: "cite_" + strings.Repeat("a", 64), Label: "receipt-1",
			}}
		},
		"claim types": func(core *AcceptedFinalPublicViewCoreV2) {
			core.ClaimTypes = []string{"AccountFlowAmount", "AccountFlowAmount"}
		},
		"coverage": func(core *AcceptedFinalPublicViewCoreV2) {
			core.CoverageStatus = AcceptedFinalCoverageComplete
		},
		"timestamp": func(core *AcceptedFinalPublicViewCoreV2) {
			core.AcceptedAt = "2026-07-11T08:00:00+00:00"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			record := privateRecord.AcceptedFinal
			core := *record.PublicView
			core.ClaimTypes = append([]string{}, core.ClaimTypes...)
			core.ReceiptMetadata = cloneAcceptedFinalReceiptMetadataV1(core.ReceiptMetadata)
			mutate(&core)
			record.PublicView = &core
			record.PublicViewDigest = acceptedFinalPublicViewCoreV2Digest(core)
			record.AuthoritySignature = ""
			record.RecordDigest = ""
			record.AuthoritySignature = base64.RawURLEncoding.EncodeToString(
				ed25519.Sign(privateKey, AcceptedFinalSigningBytes(record)),
			)
			record.RecordDigest = acceptedFinalRecordDigest(record)
			if ValidateAcceptedFinalRecord(record) == nil {
				t.Fatal("resigned malformed historical V5 public core was accepted")
			}
		})
	}
}

func TestAcceptedFinalPublicViewV3IsClosedWithoutChangingV2(t *testing.T) {
	privateRecord := acceptedFinalPublicViewBoundaryFixture(t, "current_case_source_unavailable")
	legacy, err := NewAcceptedFinalPublicViewV2(privateRecord)
	if err != nil {
		t.Fatal(err)
	}
	view, err := NewAcceptedFinalPublicViewV3(privateRecord)
	if err != nil {
		t.Fatal(err)
	}
	if legacy.SchemaVersion != AcceptedFinalPublicViewV2Version ||
		legacy.EnvelopeDigest != privateRecord.AcceptedFinal.EnvelopeDigest ||
		legacy.ContextDigest != privateRecord.SecurityContext.ContextDigest ||
		legacy.DatasetSnapshotID != privateRecord.SecurityContext.DatasetSnapshotID {
		t.Fatalf("V2 compatibility projection changed: %#v", legacy)
	}
	if view.SchemaVersion != AcceptedFinalPublicViewV3Version ||
		view.AcceptedFinalDigest != privateRecord.AcceptedFinal.RecordDigest ||
		view.PublicationState != AcceptedFinalPublicationAccepted ||
		view.CheckedScopeDigest != legacy.CheckedScopeDigest ||
		view.ReceiptMetadata.SetDigest != legacy.ReceiptMetadata.SetDigest {
		t.Fatalf("V3 generic projection mismatch: %#v", view)
	}
	viewRecord := AcceptedFinalPublicViewRecordV3(view)
	allowed := []string{
		"schemaVersion", "acceptedFinalDigest", "publicationState", "variant", "terminalReason",
		"blockerCode", "coverageStatus", "checkedScopeDigest", "missingScopeCount", "claimCount",
		"claimTypes", "receiptMetadata", "noHitWording", "acceptedAt",
	}
	if len(viewRecord) != len(allowed) {
		t.Fatalf("V3 generic projection keys are not the closed allowlist: %#v", viewRecord)
	}
	for _, field := range allowed {
		if _, present := viewRecord[field]; !present {
			t.Fatalf("V3 generic projection omitted allowlisted field %q: %#v", field, viewRecord)
		}
	}
	receiptMetadata, ok := viewRecord["receiptMetadata"].(map[string]any)
	if !ok || len(receiptMetadata) != 4 {
		t.Fatalf("V3 generic receipt metadata is not closed: %#v", viewRecord["receiptMetadata"])
	}
	for _, field := range []string{"projection", "count", "setDigest", "citations"} {
		if _, present := receiptMetadata[field]; !present {
			t.Fatalf("V3 generic receipt metadata omitted allowlisted field %q: %#v", field, receiptMetadata)
		}
	}
	for _, forbidden := range []string{
		"publicViewDigest", "threadId", "turnId", "renderedTextSha256", "envelopeIssuedAt",
		"envelopeDigest", "contextDigest", "contextEpoch", "datasetSnapshotId",
		"caseBindingHash", "factFinalWitnessAdmission", "publicationSnapshotProof",
		"publicationSnapshotProofDigest", "registryHead", "publicationIntent", "storeDigest",
	} {
		if _, present := viewRecord[forbidden]; present {
			t.Fatalf("V3 generic projection exposed private field %q: %#v", forbidden, viewRecord)
		}
	}
	if state, ok := viewRecord["publicationState"].(string); !ok || state != AcceptedFinalPublicationAccepted {
		t.Fatalf("V3 generic projection omitted its fixed publication state: %#v", viewRecord)
	}
	missingPublicationState := AcceptedFinalPublicViewRecordV3(view)
	delete(missingPublicationState, "publicationState")
	if _, err := ParseAcceptedFinalPublicViewV3Value(missingPublicationState); err == nil {
		t.Fatal("V3 generic projection accepted a missing publication state")
	}
	wrongPublicationState := AcceptedFinalPublicViewRecordV3(view)
	wrongPublicationState["publicationState"] = "pending"
	if _, err := ParseAcceptedFinalPublicViewV3Value(wrongPublicationState); err == nil {
		t.Fatal("V3 generic projection accepted a non-accepted publication state")
	}
	body, _ := json.Marshal(viewRecord)
	for _, forbiddenValue := range []string{
		privateRecord.AcceptedFinal.ThreadID,
		privateRecord.AcceptedFinal.TurnID,
		privateRecord.AcceptedFinal.EnvelopeDigest,
		privateRecord.AcceptedFinal.RenderedTextSHA256,
		privateRecord.AcceptedFinal.PublicViewDigest,
		privateRecord.SecurityContext.ContextDigest,
		privateRecord.SecurityContext.CaseBindingHash,
		privateRecord.SecurityContext.DatasetSnapshotID,
		privateRecord.PrivateRecordDigest,
		privateRecord.StoreDigest,
	} {
		if forbiddenValue != "" && strings.Contains(string(body), forbiddenValue) {
			t.Fatalf("V3 generic projection exposed private exact value %q: %s", forbiddenValue, body)
		}
	}
	viewRecord["providerSaysSafe"] = true
	if _, err := ParseAcceptedFinalPublicViewV3Value(viewRecord); err == nil {
		t.Fatal("V3 generic projection accepted an unknown property")
	}
	mismatchedCoverage := AcceptedFinalPublicViewRecordV3(view)
	mismatchedCoverage["coverageStatus"] = AcceptedFinalCoverageComplete
	if _, err := ParseAcceptedFinalPublicViewV3Value(mismatchedCoverage); err == nil {
		t.Fatal("V3 generic projection accepted coverage detached from its variant")
	}
}

func acceptedFinalPublicViewBoundaryFixture(t *testing.T, blocker string) PrivateAcceptedFinalRecord {
	t.Helper()
	now := evidenceReceiptTestTime()
	context := evidenceReceiptTestContext(t, "thread-public", "turn-public", "case-public", "snapshot-public", 4)
	envelope, err := NewFinalAnswerEnvelope(FinalAnswerEnvelopeInput{
		Variant: SourceUnavailableAnswer, Context: context, TerminalReason: "source_unavailable",
		Blocker: blocker, AcquisitionSteps: []string{"reconnect_source"}, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewEvidenceReceiptRegistry(context)
	if err != nil {
		t.Fatal(err)
	}
	head, err := NewEvidenceRegistryHead(registry)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := RenderFinalAnswer(envelope)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := NewTerminalPublicationIntent(TerminalPublicationIntentInput{
		CreatedAt: now.Format(time.RFC3339Nano), TerminalStatus: "completed",
	}, envelope.TerminalReason)
	if err != nil {
		t.Fatal(err)
	}
	privateDigest, err := PrivateAcceptedFinalDigest(context, envelope, rendered, intent)
	if err != nil {
		t.Fatal(err)
	}
	privateKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	accepted, err := NewAcceptedFinalRecord(AcceptedFinalRecordInput{
		Context: context, Envelope: envelope, RenderedText: rendered, RegistryHead: head,
		PrivateRecordDigest: privateDigest, AcceptedAt: now, AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	privateRecord, err := NewPrivateAcceptedFinalRecord(context, envelope, rendered, head, intent, accepted)
	if err != nil {
		t.Fatal(err)
	}
	return privateRecord
}
