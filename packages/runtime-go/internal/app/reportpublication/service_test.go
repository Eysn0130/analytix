package reportpublication

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	authorityadvanceapp "analytix.local/runtime-go/internal/app/authorityadvance"
	effectgateapp "analytix.local/runtime-go/internal/app/effectgate"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appmodel "analytix.local/runtime-go/internal/app/model"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	piiauthorizationapp "analytix.local/runtime-go/internal/app/piiauthorization"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	domainauthorityadvance "analytix.local/runtime-go/internal/domain/authorityadvance"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	authoritystoreport "analytix.local/runtime-go/internal/ports/authorityadvance"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	pendingworkstoreport "analytix.local/runtime-go/internal/ports/pendingworkstore"
	piiauthorizationport "analytix.local/runtime-go/internal/ports/piiauthorization"
	publicationport "analytix.local/runtime-go/internal/ports/reportpublication"
	testsecurity "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

const (
	reportCanonicalAccountV2   = "00123456789012345678"
	reportSourceExactAccountV2 = "0012-3456789012345678"
)

func TestWriteReportFalseProducesNoFile(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	input := fixture.input
	input.WriteReport = false
	result, err := fixture.service.Publish(context.Background(), input)
	if err != nil || !result.Skipped {
		t.Fatalf("write_report=false did not return the mechanical skip: result=%#v err=%v", result, err)
	}
	if fixture.totalCalls() != 0 {
		t.Fatalf("write_report=false crossed a side-effect boundary: calls=%d", fixture.totalCalls())
	}
	var nilService *Service
	if result, err := nilService.Publish(context.Background(), PublishInput{WriteReport: false}); err != nil || !result.Skipped {
		t.Fatalf("write_report=false depended on service construction: result=%#v err=%v", result, err)
	}
}

func TestControlledPublishRequiresTerminalLease(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionControlledFull)
	result, err := fixture.service.Service.Publish(context.Background(), fixture.input)
	if !errors.Is(err, ErrControlledLeaseRequired) || result.Decision.RecordDigest != "" || fixture.totalCalls() != 0 {
		t.Fatalf("raw ControlledFull input crossed the terminal lease gate: result=%#v calls=%d err=%v", result, fixture.totalCalls(), err)
	}
	input := fixture.input
	input.ReportBytes = nil
	result, err = fixture.service.Service.PublishControlled(context.Background(), input, nil)
	if !errors.Is(err, ErrControlledLeaseRequired) || result.Decision.RecordDigest != "" || fixture.totalCalls() != 0 {
		t.Fatalf("controlled publish accepted a missing concrete lease: result=%#v calls=%d err=%v", result, fixture.totalCalls(), err)
	}
}

func TestControlledPublishConsumesAuthorizationIssuedTerminalLease(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionControlledFull)
	claim := fixture.input.ClaimLedger.Claims[0]
	account := reportSourceExactAccountV2
	binding := domainpii.FieldBindingV1{
		PIIClass: domainpii.PIIClassFinancialAccountV1, ClaimID: claim.ClaimID, ClaimRecordDigest: claim.RecordDigest,
		ClaimType: claim.ClaimType, FieldName: "accountId", ValueSHA256: domainsecurity.SHA256Hex([]byte(account)),
		EvidenceReceiptIDs: append([]string(nil), claim.EvidenceIDs...),
	}
	grantStore := &reportPIIGrantStore{records: map[string]domainpii.PIIProjectionGrantV1{}}
	evidenceAuthority, err := piiauthorizationapp.NewWitnessedEvidenceAuthority(fixture.evidence)
	if err != nil {
		t.Fatal(err)
	}
	piiService, err := piiauthorizationapp.New(piiauthorizationapp.Config{
		Authority: fixture.authority, Store: grantStore, Ledgers: fixture.ledgers,
		Approval: reportPIIApprovalAuthorityStub{}, Evidence: evidenceAuthority,
		ValidateCurrent: func(context.Context, domainsecurity.TurnSecurityContext) error { return nil },
		Now:             func() time.Time { return fixture.now.Add(time.Minute) },
	})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := piiService.PrepareControlledArtifact(context.Background(), piiauthorizationapp.PrepareControlledArtifactInputV1{
		SecurityContext: fixture.input.PendingToolCall.SecurityContext, ClaimLedger: fixture.input.ClaimLedger,
		FieldBindings: []domainpii.FieldBindingV1{binding}, ProjectionRulesetHash: fixture.input.PIIProjection.RulesetHash,
		TargetIdentityDigest:  fixture.input.TargetIdentityDigest,
		AllowedAccessActions:  []string{domainpii.ControlledArtifactAccessActionDisplayV1, domainpii.ControlledArtifactAccessActionExportV1},
		AccessPolicyDigest:    domainsecurity.SHA256Hex([]byte("report-controlled-access-policy")),
		RetentionPolicyDigest: domainsecurity.SHA256Hex([]byte("report-controlled-retention-policy")),
		RetentionUntil:        fixture.now.Add(24 * time.Hour), ExpiresAt: fixture.now.Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	authorized, err := piiService.AuthorizePreparedControlledArtifact(context.Background(), piiauthorizationapp.AuthorizePreparedControlledArtifactInputV1{
		SecurityContext: fixture.input.PendingToolCall.SecurityContext, ClaimLedger: fixture.input.ClaimLedger,
		Prepared: prepared, ApprovalID: "appr_report_controlled_12345678",
		ApprovalRecordDigest: domainsecurity.SHA256Hex([]byte("report-controlled-approval-record")),
	})
	if err != nil {
		t.Fatal(err)
	}
	input := fixture.input
	input.ReportBytes = nil
	input.PIIProjection = authorized.PIIProjection
	input.RenderInspection, err = domainpublication.NewRenderInspectionV1(domainpublication.RenderInspectionInputV1{
		Renderer: "analytix-host", RendererVersion: "1.0.0", ReportSHA256: authorized.ProtectedSHA256,
		ReportByteLength: authorized.ProtectedByteLength, MediaType: domainpii.ControlledPIIArtifactMediaTypeV1,
		Passed: true, IssueCodes: []string{}, InspectedAt: fixture.now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := fixture.service.Service.PublishControlled(context.Background(), input, authorized.Lease)
	if err != nil || result.Decision.RecordDigest == "" {
		t.Fatalf("authorization-issued terminal lease was not published: result=%#v err=%v", result, err)
	}
	disposition, terminal := authorized.Lease.Disposition()
	if !terminal || disposition.Status != piiauthorizationapp.ControlledPIITerminalHandoffConsumedV1 ||
		disposition.ObservedByteLength != authorized.ProtectedByteLength {
		t.Fatalf("controlled publication did not consume the exact lease: disposition=%#v terminal=%v", disposition, terminal)
	}
	stored := fixture.artifacts.records[input.TargetIdentityDigest]
	if !bytes.Contains(stored, []byte(`"exactValue":"`+account+`"`)) || len(input.ReportBytes) != 0 {
		t.Fatalf("controlled artifact was not exact or escaped into public input: stored=%s inputBytes=%d", stored, len(input.ReportBytes))
	}
	t.Run("stored controlled outcome bindings", func(t *testing.T) {
		verifyStoredControlledOutcomeBindingsFixture(t, fixture, result, grantStore.records[result.Candidate.AuthorizationAuditDigest])
	})
}

func TestOrdinaryReportIgnoresCallerPassedAndScansFinalBytes(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	input := fixture.input
	rebuildReportBytesForTest(t, &input, []byte(
		"账号: 0000123456789012345\n手机号: 13800138000\nMAC地址: aa:bb:cc:dd:ee:ff",
	))
	result, err := fixture.service.Publish(context.Background(), input)
	if err == nil || result.Commit.RecordDigest != "" {
		t.Fatalf("caller-reported passed inspection released restricted ordinary report: result=%#v err=%v", result, err)
	}
	if fixture.extractor.calls != 1 || fixture.stages.beginCalls != 0 || fixture.artifacts.installCalls != 0 ||
		fixture.receipts.putCalls != 0 || fixture.coordinator.advanceCalls != 0 || fixture.delivery.calls != 0 {
		t.Fatalf("ordinary privacy rejection crossed a side-effect boundary: extractor=%d stage=%d artifact=%d receipt=%d advance=%d delivery=%d",
			fixture.extractor.calls, fixture.stages.beginCalls, fixture.artifacts.installCalls,
			fixture.receipts.putCalls, fixture.coordinator.advanceCalls, fixture.delivery.calls)
	}
}

func TestOrdinaryReportIncompleteSurfaceFailsBeforeStage(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	fixture.extractor.err = errors.New("unsupported or incomplete report surface")
	result, err := fixture.service.Publish(context.Background(), fixture.input)
	if err == nil || result.Commit.RecordDigest != "" || fixture.extractor.calls != 1 || fixture.stages.beginCalls != 0 ||
		fixture.artifacts.installCalls != 0 || fixture.coordinator.advanceCalls != 0 {
		t.Fatalf("incomplete ordinary surface did not fail closed: result=%#v extractor=%d stage=%d artifact=%d advance=%d err=%v",
			result, fixture.extractor.calls, fixture.stages.beginCalls, fixture.artifacts.installCalls, fixture.coordinator.advanceCalls, err)
	}
}

func TestOrdinaryReportReasoningSurfaceFailsBeforeStage(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "complete reasoning block", body: "public<think>PRIVATE_REASONING</think>tail"},
		{name: "incomplete reasoning block", body: "public<think>PRIVATE_REASONING"},
		{name: "serialized reasoning field", body: `{"reasoning_content":"PRIVATE_REASONING"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
			input := fixture.input
			rebuildReportBytesForTest(t, &input, []byte(test.body))

			result, err := fixture.service.Publish(context.Background(), input)
			if err == nil || result.Commit.RecordDigest != "" || fixture.extractor.calls != 1 ||
				fixture.stages.beginCalls != 0 || fixture.artifacts.installCalls != 0 ||
				fixture.receipts.putCalls != 0 || fixture.coordinator.advanceCalls != 0 || fixture.delivery.calls != 0 {
				t.Fatalf("reasoning surface crossed report stage: result=%#v extractor=%d stage=%d artifact=%d receipt=%d advance=%d delivery=%d err=%v",
					result, fixture.extractor.calls, fixture.stages.beginCalls, fixture.artifacts.installCalls,
					fixture.receipts.putCalls, fixture.coordinator.advanceCalls, fixture.delivery.calls, err)
			}
		})
	}
}

func TestOrdinaryReportRestrictedEvidenceSurfaceFailsBeforeStage(t *testing.T) {
	hash := strings.Repeat("a", 64)
	tests := []string{
		`{"purpose":"analytix.raw-artifact-manifest/v1","manifestDigest":"` + hash + `"}`,
		`{"neutral":{"parsedGenerationReceiptDigest":"` + hash + `","parsedGenerationReceiptSha256":"` + hash + `","parsedGenerationReceiptByteLength":42}}`,
		`{"review":{"output":{"lineageDigest":"` + hash + `","lineageSha256":"` + hash + `","lineageByteLength":42}}}`,
	}
	for _, body := range tests {
		fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
		input := fixture.input
		rebuildReportBytesForTest(t, &input, []byte(body))

		result, err := fixture.service.Publish(context.Background(), input)
		if err == nil || result.Commit.RecordDigest != "" || fixture.extractor.calls != 1 ||
			fixture.stages.beginCalls != 0 || fixture.artifacts.installCalls != 0 ||
			fixture.receipts.putCalls != 0 || fixture.coordinator.advanceCalls != 0 || fixture.delivery.calls != 0 {
			t.Fatalf("restricted evidence surface crossed report stage: result=%#v extractor=%d stage=%d artifact=%d receipt=%d advance=%d delivery=%d err=%v",
				result, fixture.extractor.calls, fixture.stages.beginCalls, fixture.artifacts.installCalls,
				fixture.receipts.putCalls, fixture.coordinator.advanceCalls, fixture.delivery.calls, err)
		}
	}
}

func TestOrdinaryReportFinalSurfaceRevalidationCreatesNoDecision(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	fixture.extractor.canonicalByCall = [][]byte{
		[]byte("safe staged surface"),
		[]byte("safe durable artifact surface"),
		[]byte("账号: 0000123456789012345"),
	}
	result, err := fixture.service.Publish(context.Background(), fixture.input)
	if err == nil || result.Decision.RecordDigest != "" || fixture.extractor.calls != 3 ||
		fixture.commits.putCalls != 1 || fixture.decisions.createCalls != 0 || fixture.delivery.calls != 0 || fixture.stages.closeCalls != 0 {
		t.Fatalf("final ordinary privacy failure created delivery authority: result=%#v scans=%d commits=%d decisions=%d delivery=%d close=%d err=%v",
			result, fixture.extractor.calls, fixture.commits.putCalls, fixture.decisions.createCalls,
			fixture.delivery.calls, fixture.stages.closeCalls, err)
	}
}

func rebuildReportBytesForTest(t *testing.T, input *PublishInput, reportBytes []byte) {
	t.Helper()
	reportBytes = bytes.Clone(reportBytes)
	reportSHA := domainsecurity.SHA256Hex(reportBytes)
	projection, err := domainpublication.NewPIIProjectionV1(domainpublication.PIIProjectionInputV1{
		ProjectionClass:               input.PIIProjection.ProjectionClass,
		RulesetHash:                   input.PIIProjection.RulesetHash,
		ProjectedContentSHA256:        reportSHA,
		RestrictedFieldCount:          input.PIIProjection.RestrictedFieldCount,
		PreservedControlledFieldCount: input.PIIProjection.PreservedControlledFieldCount,
		AuthorizationAuditDigest:      input.PIIProjection.AuthorizationAuditDigest,
	})
	if err != nil {
		t.Fatal(err)
	}
	inspectedAt, err := time.Parse(time.RFC3339Nano, input.RenderInspection.InspectedAt)
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := domainpublication.NewRenderInspectionV1(domainpublication.RenderInspectionInputV1{
		Renderer: input.RenderInspection.Renderer, RendererVersion: input.RenderInspection.RendererVersion,
		ReportSHA256: reportSHA, ReportByteLength: uint64(len(reportBytes)), MediaType: input.RenderInspection.MediaType,
		Passed: true, IssueCodes: []string{}, InspectedAt: inspectedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	input.ReportBytes = reportBytes
	input.PIIProjection = projection
	input.RenderInspection = inspection
}

func TestPublicationAdvanceWithoutFreshObservationNeverDelivers(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	fixture.coordinator.omitWitness = true
	result, err := fixture.service.Publish(context.Background(), fixture.input)
	if err == nil || result.Commit.RecordDigest != "" || fixture.delivery.calls != 0 || fixture.stages.closeCalls != 0 {
		t.Fatalf("unwitnessed publication reached delivery: result=%#v delivery=%d err=%v", result, fixture.delivery.calls, err)
	}
}

func TestReportPublicationCreatesDecisionButCannotDeliverBeforeSettlement(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	result, err := fixture.service.Publish(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Skipped || domainpublication.ValidatePublicationReceiptV1(result.Candidate) != nil ||
		domainpublication.ValidatePublicationIndexReceiptV1(result.Index, result.Candidate) != nil ||
		domainpublication.ValidatePublicationCommitReceiptV1(result.Commit) != nil ||
		domainpublication.ValidateReportDeliveryDecisionV1(result.Decision) != nil {
		t.Fatalf("formal report lacks valid publication authority: %#v", result)
	}
	if fixture.stages.beginCalls != 1 || fixture.stages.resolveCalls != 1 || fixture.stages.verifyCalls != 4 || fixture.stages.closeCalls != 0 ||
		fixture.evidence.calls != 3 || fixture.coordinator.advanceCalls != 1 || fixture.artifacts.installCalls != 1 ||
		fixture.decisions.createCalls != 1 || fixture.decisions.resolveCalls != 1 || fixture.delivery.calls != 0 {
		t.Fatalf("publication sequence call counts are invalid: stage=%#v evidence=%d advance=%d artifact=%d delivery=%d",
			fixture.stages, fixture.evidence.calls, fixture.coordinator.advanceCalls, fixture.artifacts.installCalls, fixture.delivery.calls)
	}
	if result.Decision.CommitRecordDigest != result.Commit.RecordDigest ||
		fixture.decisions.records[result.Decision.DecisionID].RecordDigest != result.Decision.RecordDigest {
		t.Fatalf("final admission did not bind the exact post-witness commit: %#v", result.Decision)
	}
	if result.Candidate.EvidenceAuthorityBundleDigest != fixture.initialSnapshot.Head.Bundle.RecordDigest ||
		result.Candidate.EvidenceRegistryIndexDigest != fixture.initialSnapshot.Head.Bundle.EvidenceRegistryIndexDigest ||
		result.Candidate.EvidenceRegistrySequence != fixture.initialSnapshot.Registry.Sequence ||
		result.Candidate.ReportSHA256 != domainsecurity.SHA256Hex(fixture.input.ReportBytes) ||
		result.Commit.CandidateRecordDigest != result.Candidate.RecordDigest {
		t.Fatalf("publication commit did not bind the inspected evidence/report: %#v", result)
	}
	tampered := result.Commit
	tampered.ReportSHA256 = domainsecurity.SHA256Hex([]byte("tampered-report"))
	if domainpublication.ValidatePublicationCommitReceiptV1(tampered) == nil {
		t.Fatal("tampered post-witness publication commit was accepted")
	}
}

func TestReportGrantSettlementBindsDecisionResultAndRegistryTransition(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	result, err := fixture.service.Publish(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	grant := fixture.input.PendingToolCall.ExecutionGrant
	active, err := domainsecurity.RegisterExecutionGrant(
		domainsecurity.NewExecutionGrantRegistry(fixture.input.PendingToolCall.ThreadID),
		fixture.input.PendingToolCall.ThreadID, grant, fixture.now,
	)
	if err != nil {
		t.Fatal(err)
	}
	active, err = domainsecurity.RegisterExecutionGrant(
		active, fixture.input.PendingToolCall.ThreadID, fixture.nonStageGrant, fixture.now,
	)
	if err != nil {
		t.Fatal(err)
	}
	settledAt := fixture.now.Add(6 * time.Minute)
	settled, err := domainsecurity.SettleRegisteredExecutionGrant(active, grant.GrantID, settledAt)
	if err != nil {
		t.Fatal(err)
	}
	input := domainpublication.ReportGrantSettlementInputV1{
		Decision: result.Decision, Grant: grant, ActiveRegistry: active, SettledRegistry: settled,
		ResultItemID:     domaintoolresult.ToolResultItemIDV1(grant.TurnID, grant.ToolCallID),
		ResultItemDigest: domainsecurity.SHA256Hex([]byte("canonical-private-report-tool-result")),
		SettledAt:        settledAt, AuthorityKeyID: fixture.authority.keyID, AuthorityPublicKey: fixture.authority.publicKey,
	}
	grantSettlement, err := domainpublication.NewReportGrantSettlementV1(input, func(message []byte) ([]byte, error) {
		return fixture.authority.Sign(context.Background(), message)
	})
	if err != nil || domainpublication.ValidateReportGrantSettlementGraphV1(grantSettlement, input) != nil {
		t.Fatalf("exact report grant settlement was rejected: settlement=%#v err=%v", grantSettlement, err)
	}
	body, err := domainpublication.ReportGrantSettlementV1Bytes(grantSettlement)
	if err != nil || bytes.Contains(body, fixture.input.ReportBytes) || bytes.Contains(body, []byte("00123456789012345678")) {
		t.Fatalf("report grant settlement leaked report or controlled content: err=%v", err)
	}
	tampered := input
	tampered.ResultItemDigest = domainsecurity.SHA256Hex([]byte("different-result"))
	if domainpublication.ValidateReportGrantSettlementGraphV1(grantSettlement, tampered) == nil {
		t.Fatal("report grant settlement authorized a different durable tool result")
	}
	otherToolCallID, err := domainmodel.NewHostToolCallIDV1(bytes.Repeat([]byte{0x72}, domainmodel.HostToolCallIDEntropyBytesV1))
	if err != nil {
		t.Fatal(err)
	}
	mismatchedResult := input
	mismatchedResult.ResultItemID = domaintoolresult.ToolResultItemIDV1(grant.TurnID, otherToolCallID)
	if _, err := domainpublication.NewReportGrantSettlementV1(mismatchedResult, func(message []byte) ([]byte, error) {
		return fixture.authority.Sign(context.Background(), message)
	}); err == nil {
		t.Fatal("syntactically valid result identity from another tool call was accepted")
	}
	wrongTransition := input
	wrongTransition.SettledRegistry = active
	if _, err := domainpublication.NewReportGrantSettlementV1(wrongTransition, func(message []byte) ([]byte, error) {
		return fixture.authority.Sign(context.Background(), message)
	}); err == nil {
		t.Fatal("active registry was accepted as a settled report grant")
	}
	unrelatedToolCallID, err := domainmodel.NewHostToolCallIDV1(bytes.Repeat([]byte{0x73}, domainmodel.HostToolCallIDEntropyBytesV1))
	if err != nil {
		t.Fatal(err)
	}
	unrelated := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: fixture.input.PendingToolCall.SecurityContext, Provider: grant.Provider, ServerIdentity: grant.ServerIdentity,
		ToolName: grant.ToolName, ToolCallID: unrelatedToolCallID, ArgsHash: domainsecurity.CanonicalJSONHash([]byte(`{"report":"other"}`)),
		SchemaHash: grant.SchemaHash, ScopeHash: grant.ScopeHash, ReadOnly: false, ApprovalState: "approved",
		IssuedAt: fixture.now.Add(time.Second), ExpiresAt: fixture.now.Add(time.Hour),
	})
	widerActive, err := domainsecurity.RegisterExecutionGrant(
		active, fixture.input.PendingToolCall.ThreadID, unrelated, fixture.now.Add(time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	widerSettled, err := domainsecurity.SettleRegisteredExecutionGrant(widerActive, grant.GrantID, settledAt)
	if err != nil {
		t.Fatal(err)
	}
	wrongStageRegistry := input
	wrongStageRegistry.ActiveRegistry = widerActive
	wrongStageRegistry.SettledRegistry = widerSettled
	if _, err := domainpublication.NewReportGrantSettlementV1(wrongStageRegistry, func(message []byte) ([]byte, error) {
		return fixture.authority.Sign(context.Background(), message)
	}); err == nil {
		t.Fatal("a later registry head replaced the exact signed report-stage registry")
	}
}

func TestReportStageCompletionRequiresExactCompletedDisposition(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionControlledFull)
	result, err := fixture.service.Publish(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	settlement := fixture.installDurableReportGrantSettlement(t, result.Decision)
	settledAt, err := time.Parse(time.RFC3339Nano, settlement.SettledAt)
	if err != nil {
		t.Fatal(err)
	}
	disposition, err := domainpendingwork.NewPendingWorkDispositionV1(
		fixture.stages.receipt, domainpendingwork.StatusCompleted, "report_stage_completed", settledAt,
		fixture.authority.keyID, fixture.authority.publicKey,
		func(message []byte) ([]byte, error) { return fixture.authority.Sign(context.Background(), message) },
	)
	if err != nil {
		t.Fatal(err)
	}
	input := domainpublication.ReportStageCompletionInputV1{
		Decision: result.Decision, GrantSettlement: settlement, StageReceipt: fixture.stages.receipt,
		StageDisposition: disposition, AuthorityKeyID: fixture.authority.keyID, AuthorityPublicKey: fixture.authority.publicKey,
	}
	completion, err := domainpublication.NewReportStageCompletionV1(input, func(message []byte) ([]byte, error) {
		return fixture.authority.Sign(context.Background(), message)
	})
	if err != nil || domainpublication.ValidateReportStageCompletionGraphV1(completion, input) != nil ||
		domainpublication.ValidateReportStageCompletionDecisionSettlementV1(completion, result.Decision, settlement) != nil {
		t.Fatalf("exact report stage completion was rejected: completion=%#v err=%v", completion, err)
	}
	body, err := domainpublication.ReportStageCompletionV1Bytes(completion)
	if err != nil || bytes.Contains(body, fixture.input.ReportBytes) || bytes.Contains(body, []byte("00123456789012345678")) {
		t.Fatalf("report stage completion leaked report or controlled account bytes: %s err=%v", body, err)
	}
	lateDisposition, err := domainpendingwork.NewPendingWorkDispositionV1(
		fixture.stages.receipt, domainpendingwork.StatusCompleted, "report_stage_completed", settledAt.Add(time.Second),
		fixture.authority.keyID, fixture.authority.publicKey,
		func(message []byte) ([]byte, error) { return fixture.authority.Sign(context.Background(), message) },
	)
	if err != nil {
		t.Fatal(err)
	}
	wrongTime := input
	wrongTime.StageDisposition = lateDisposition
	if _, err := domainpublication.NewReportStageCompletionV1(wrongTime, func(message []byte) ([]byte, error) {
		return fixture.authority.Sign(context.Background(), message)
	}); err == nil {
		t.Fatal("a disposition not deterministically bound to the grant settlement time created completion authority")
	}
	failedDisposition, err := domainpendingwork.NewPendingWorkDispositionV1(
		fixture.stages.receipt, domainpendingwork.StatusFailed, "report_stage_failed", settledAt,
		fixture.authority.keyID, fixture.authority.publicKey,
		func(message []byte) ([]byte, error) { return fixture.authority.Sign(context.Background(), message) },
	)
	if err != nil {
		t.Fatal(err)
	}
	failed := input
	failed.StageDisposition = failedDisposition
	if _, err := domainpublication.NewReportStageCompletionV1(failed, func(message []byte) ([]byte, error) {
		return fixture.authority.Sign(context.Background(), message)
	}); err == nil {
		t.Fatal("failed report stage disposition created completion authority")
	}
}

func TestReportDeliveryProjectionRequiresCompleteSignedStageGraph(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionControlledFull)
	result, err := fixture.service.Publish(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	settlement := fixture.installDurableReportGrantSettlement(t, result.Decision)
	completion := fixture.installDurableReportStageCompletion(t, result.Decision, settlement)
	if fixture.stages.disposition == nil {
		t.Fatal("completed report stage disposition is unavailable")
	}
	input := domainpublication.ReportDeliveryProjectionInputV1{
		Decision: result.Decision, GrantSettlement: settlement, StageReceipt: fixture.stages.receipt,
		StageDisposition: *fixture.stages.disposition, StageCompletion: completion,
		AuthorityKeyID: fixture.authority.keyID, AuthorityPublicKey: fixture.authority.publicKey,
	}
	projection, err := domainpublication.NewReportDeliveryProjectionV1(input, func(message []byte) ([]byte, error) {
		return fixture.authority.Sign(context.Background(), message)
	})
	if err != nil || domainpublication.ValidateReportDeliveryProjectionGraphV1(projection, input) != nil ||
		domainpublication.ValidateReportDeliveryProjectionCompletionV1(projection, completion) != nil {
		t.Fatalf("exact completion-bound delivery projection was rejected: projection=%#v err=%v", projection, err)
	}
	body, err := domainpublication.ReportDeliveryProjectionV1Bytes(projection)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := domainpublication.ParseReportDeliveryProjectionV1(body)
	if err != nil || !reflect.DeepEqual(parsed, projection) {
		t.Fatalf("delivery projection strict round trip failed: parsed=%#v err=%v", parsed, err)
	}
	for _, forbidden := range [][]byte{
		fixture.input.ReportBytes,
		[]byte("00123456789012345678"),
		[]byte(fixture.input.PendingToolCall.SecurityContext.WorkspaceRealPath),
		[]byte(`"context"`),
	} {
		if len(forbidden) > 0 && bytes.Contains(body, forbidden) {
			t.Fatalf("delivery projection leaked report, PII, path, or full context: %s", body)
		}
	}
	completionBody, err := domainpublication.ReportStageCompletionV1Bytes(completion)
	if err != nil || bytes.Equal(completionBody, body) || projection.DeliveryID == completion.CompletionID {
		t.Fatalf("stage completion was reused as the delivery transition: err=%v", err)
	}
	missingCompletion := input
	missingCompletion.StageCompletion = domainpublication.ReportStageCompletionV1{}
	if _, err := domainpublication.NewReportDeliveryProjectionV1(missingCompletion, func(message []byte) ([]byte, error) {
		return fixture.authority.Sign(context.Background(), message)
	}); err == nil {
		t.Fatal("delivery authority was created without a signed stage completion")
	}
	wrongDisposition := input
	wrongDisposition.StageDisposition = domainpendingwork.PendingWorkDispositionV1{}
	if _, err := domainpublication.NewReportDeliveryProjectionV1(wrongDisposition, func(message []byte) ([]byte, error) {
		return fixture.authority.Sign(context.Background(), message)
	}); err == nil {
		t.Fatal("delivery authority was created without the exact completed disposition")
	}
}

func TestReportPublicationAttemptIsDurableBeforeAnyReportMaterial(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	checked := false
	fixture.artifacts.beforeInstall = func() {
		checked = true
		if fixture.attempts.createCalls != 1 || fixture.attempts.resolveCalls != 2 || len(fixture.attempts.records) != 1 ||
			fixture.receipts.putCalls != 0 || fixture.indexes.putCalls != 0 || fixture.bundles.putCalls != 0 ||
			fixture.ledgers.putCalls != 0 || fixture.projections.putCalls != 0 || fixture.inspections.putCalls != 0 ||
			fixture.coordinator.advanceCalls != 0 {
			t.Fatalf("report material preceded its durable attempt: attempts=%#v receipt=%d index=%d bundle=%d ledger=%d projection=%d inspection=%d advance=%d",
				fixture.attempts, fixture.receipts.putCalls, fixture.indexes.putCalls, fixture.bundles.putCalls,
				fixture.ledgers.putCalls, fixture.projections.putCalls, fixture.inspections.putCalls, fixture.coordinator.advanceCalls)
		}
	}
	if _, err := fixture.service.Publish(context.Background(), fixture.input); err != nil {
		t.Fatal(err)
	}
	if !checked {
		t.Fatal("artifact store was not reached")
	}
}

func TestExistingPublicationAttemptNeverDowngradesToDefinitiveFailure(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	stageRequest, err := fixture.service.validateInput(fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := fixture.service.preparePublicationPlan(
		context.Background(), fixture.input, stageRequest, fixture.stages.receipt, fixture.initialSnapshot,
	)
	if err != nil {
		t.Fatal(err)
	}
	created, err := fixture.attempts.CreateExclusive(context.Background(), plan.attempt)
	if err != nil || !created {
		t.Fatalf("pre-reserve report publication attempt: created=%v err=%v", created, err)
	}
	fixture.attempts.createCalls = 0
	fixture.attempts.resolveCalls = 0
	result, err := fixture.service.Publish(context.Background(), fixture.input)
	if err == nil || result.Commit.RecordDigest != "" || fixture.stages.closeCalls != 0 || fixture.artifacts.installCalls != 0 ||
		fixture.coordinator.advanceCalls != 0 {
		t.Fatalf("existing attempt was retried or downgraded: result=%#v close=%d artifact=%d advance=%d err=%v",
			result, fixture.stages.closeCalls, fixture.artifacts.installCalls, fixture.coordinator.advanceCalls, err)
	}
}

func TestReportPublicationRestartPreflightValidatesCommittedAttemptGraph(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	result, err := fixture.service.Publish(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	journal := &reportRestartJournalStub{intent: &fixture.coordinator.intent, settlement: &fixture.coordinator.settlement}
	plan, err := PreflightRestartV1(context.Background(), RestartPreflightConfigV1{
		Pending:  pendingworkapp.TrustedInventoryV1{Receipts: []domainpendingwork.PendingWorkReceiptV1{fixture.stages.receipt}},
		Attempts: fixture.attempts, Receipts: fixture.receipts, Indexes: fixture.indexes,
		Commits: fixture.commits, Selections: fixture.selections, Decisions: fixture.decisions, GrantSettlements: fixture.grantSettlements, StageCompletions: fixture.stageCompletions, DeliveryOutcomes: fixture.delivery, Threads: fixture.threads,
		Ledgers: fixture.ledgers, Projections: fixture.projections, Inspections: fixture.inspections,
		Intents: journal, Settlements: journal, Authority: fixture.authority,
	})
	if err != nil || len(plan.Attempts) != 1 || plan.Attempts[0].State != RestartAttemptDeliveryDecisionV1 ||
		plan.Attempts[0].Commit == nil || plan.Attempts[0].Commit.RecordDigest != result.Commit.RecordDigest ||
		plan.Attempts[0].Decision == nil || plan.Attempts[0].Decision.RecordDigest != result.Decision.RecordDigest {
		t.Fatalf("committed restart plan = %#v err=%v", plan, err)
	}
}

func TestReportPublicationRestartPreflightAllowsCandidateCrashCutButRejectsOrphanSettlement(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	stageRequest, err := fixture.service.validateInput(fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	publicationPlan, err := fixture.service.preparePublicationPlan(
		context.Background(), fixture.input, stageRequest, fixture.stages.receipt, fixture.initialSnapshot,
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture.attempts.records[publicationPlan.attempt.AttemptID] = publicationPlan.attempt
	fixture.receipts.records[publicationPlan.receipt.RecordDigest] = publicationPlan.receipt
	journal := &reportRestartJournalStub{}
	config := RestartPreflightConfigV1{
		Pending:  pendingworkapp.TrustedInventoryV1{Receipts: []domainpendingwork.PendingWorkReceiptV1{fixture.stages.receipt}},
		Attempts: fixture.attempts, Receipts: fixture.receipts, Indexes: fixture.indexes,
		Commits: fixture.commits, Selections: fixture.selections, Decisions: fixture.decisions, GrantSettlements: fixture.grantSettlements, StageCompletions: fixture.stageCompletions, DeliveryOutcomes: fixture.delivery, Threads: fixture.threads,
		Ledgers: fixture.ledgers, Projections: fixture.projections, Inspections: fixture.inspections,
		Intents: journal, Settlements: journal, Authority: fixture.authority,
	}
	plan, err := PreflightRestartV1(context.Background(), config)
	if err != nil || len(plan.Attempts) != 1 || plan.Attempts[0].State != RestartAttemptCandidateDurableV1 {
		t.Fatalf("candidate-only crash cut = %#v err=%v", plan, err)
	}
	orphanSettlement := domainauthorityadvance.MonotonicAdvanceSettlementV2{
		MutationID: publicationPlan.attempt.AuthorityAdvanceMutationID,
	}
	journal.settlement = &orphanSettlement
	if _, err := PreflightRestartV1(context.Background(), config); err == nil {
		t.Fatal("orphan publication settlement passed restart preflight")
	}
}

func TestRestartPreflightRejectsIntentBeforePublicationIndex(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	stageRequest, err := fixture.service.validateInput(fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	publicationPlan, err := fixture.service.preparePublicationPlan(
		context.Background(), fixture.input, stageRequest, fixture.stages.receipt, fixture.initialSnapshot,
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture.attempts.records[publicationPlan.attempt.AttemptID] = publicationPlan.attempt
	fixture.receipts.records[publicationPlan.receipt.RecordDigest] = publicationPlan.receipt
	journal := &reportRestartJournalStub{intent: &publicationPlan.intent}
	_, err = PreflightRestartV1(context.Background(), RestartPreflightConfigV1{
		Pending:  pendingworkapp.TrustedInventoryV1{Receipts: []domainpendingwork.PendingWorkReceiptV1{fixture.stages.receipt}},
		Attempts: fixture.attempts, Receipts: fixture.receipts, Indexes: fixture.indexes,
		Commits: fixture.commits, Selections: fixture.selections, Decisions: fixture.decisions, GrantSettlements: fixture.grantSettlements, StageCompletions: fixture.stageCompletions, DeliveryOutcomes: fixture.delivery, Threads: fixture.threads,
		Ledgers: fixture.ledgers, Projections: fixture.projections, Inspections: fixture.inspections,
		Intents: journal, Settlements: journal, Authority: fixture.authority,
	})
	if err == nil || !strings.Contains(err.Error(), "precedes materials") {
		t.Fatalf("publication intent without its durable index passed restart preflight: %v", err)
	}
}

func TestRestartCommittedAttemptWithFailedStageDispositionBecomesDeliveryBlocked(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	if _, err := fixture.service.Publish(context.Background(), fixture.input); err != nil {
		t.Fatal(err)
	}
	disposition, err := domainpendingwork.NewPendingWorkDispositionV1(
		fixture.stages.receipt, domainpendingwork.StatusFailed, "report_stage_failed", fixture.now.Add(6*time.Minute),
		fixture.authority.keyID, fixture.authority.publicKey,
		func(message []byte) ([]byte, error) { return fixture.authority.Sign(context.Background(), message) },
	)
	if err != nil {
		t.Fatal(err)
	}
	journal := &reportRestartJournalStub{intent: &fixture.coordinator.intent, settlement: &fixture.coordinator.settlement}
	plan, err := PreflightRestartV1(context.Background(), RestartPreflightConfigV1{
		Pending: pendingworkapp.TrustedInventoryV1{
			Receipts:     []domainpendingwork.PendingWorkReceiptV1{fixture.stages.receipt},
			Dispositions: map[string]domainpendingwork.PendingWorkDispositionV1{fixture.stages.receipt.WorkID: disposition},
		},
		Attempts: fixture.attempts, Receipts: fixture.receipts, Indexes: fixture.indexes,
		Commits: fixture.commits, Selections: fixture.selections, Decisions: fixture.decisions, GrantSettlements: fixture.grantSettlements, StageCompletions: fixture.stageCompletions, DeliveryOutcomes: fixture.delivery, Threads: fixture.threads,
		Ledgers: fixture.ledgers, Projections: fixture.projections, Inspections: fixture.inspections,
		Intents: journal, Settlements: journal, Authority: fixture.authority,
	})
	if err != nil || len(plan.Attempts) != 1 || plan.Attempts[0].State != RestartAttemptDeliveryBlockedV1 {
		t.Fatalf("committed failed report stage did not become delivery-blocked: plan=%#v err=%v", plan, err)
	}
	fixture.stages.disposition = &disposition
	result, err := ApplyRestartV1(context.Background(), plan, fixture.restartApplyConfig())
	if err != nil || result.DeliveryBlocked != 1 || result.Delivered != 0 || fixture.delivery.calls != 0 {
		t.Fatalf("delivery-blocked report crossed projection: result=%#v project=%d err=%v", result, fixture.delivery.calls, err)
	}
}

func TestRestartAbortedBeforeWitnessCannotPublish(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	stageRequest, err := fixture.service.validateInput(fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	publicationPlan, err := fixture.service.preparePublicationPlan(
		context.Background(), fixture.input, stageRequest, fixture.stages.receipt, fixture.initialSnapshot,
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture.attempts.records[publicationPlan.attempt.AttemptID] = publicationPlan.attempt
	fixture.receipts.records[publicationPlan.receipt.RecordDigest] = publicationPlan.receipt
	disposition, err := domainpendingwork.NewPendingWorkDispositionV1(
		fixture.stages.receipt, domainpendingwork.StatusFailed, "report_stage_failed", fixture.now.Add(6*time.Minute),
		fixture.authority.keyID, fixture.authority.publicKey,
		func(message []byte) ([]byte, error) { return fixture.authority.Sign(context.Background(), message) },
	)
	if err != nil {
		t.Fatal(err)
	}
	journal := &reportRestartJournalStub{}
	plan, err := PreflightRestartV1(context.Background(), RestartPreflightConfigV1{
		Pending: pendingworkapp.TrustedInventoryV1{
			Receipts:     []domainpendingwork.PendingWorkReceiptV1{fixture.stages.receipt},
			Dispositions: map[string]domainpendingwork.PendingWorkDispositionV1{fixture.stages.receipt.WorkID: disposition},
		},
		Attempts: fixture.attempts, Receipts: fixture.receipts, Indexes: fixture.indexes,
		Commits: fixture.commits, Selections: fixture.selections, Decisions: fixture.decisions, GrantSettlements: fixture.grantSettlements, StageCompletions: fixture.stageCompletions, DeliveryOutcomes: fixture.delivery, Threads: fixture.threads,
		Ledgers: fixture.ledgers, Projections: fixture.projections, Inspections: fixture.inspections,
		Intents: journal, Settlements: journal, Authority: fixture.authority,
	})
	if err != nil || len(plan.Attempts) != 1 || plan.Attempts[0].State != RestartAttemptAbortedV1 {
		t.Fatalf("pre-witness failure did not become a fixed aborted restart state: plan=%#v err=%v", plan, err)
	}
	fixture.stages.disposition = &disposition
	result, err := ApplyRestartV1(context.Background(), plan, fixture.restartApplyConfig())
	if err != nil || result.AbortedBeforeWitness != 1 || result.Delivered != 0 || fixture.delivery.calls != 0 {
		t.Fatalf("aborted report restart crossed a publication boundary: result=%#v delivery=%d err=%v", result, fixture.delivery.calls, err)
	}
}

func TestRestartPlanTamperingFailsClosedBeforeEffects(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	if _, err := fixture.service.Publish(context.Background(), fixture.input); err != nil {
		t.Fatal(err)
	}
	journal := &reportRestartJournalStub{intent: &fixture.coordinator.intent, settlement: &fixture.coordinator.settlement}
	plan, err := PreflightRestartV1(context.Background(), RestartPreflightConfigV1{
		Pending:  pendingworkapp.TrustedInventoryV1{Receipts: []domainpendingwork.PendingWorkReceiptV1{fixture.stages.receipt}},
		Attempts: fixture.attempts, Receipts: fixture.receipts, Indexes: fixture.indexes,
		Commits: fixture.commits, Selections: fixture.selections, Decisions: fixture.decisions, GrantSettlements: fixture.grantSettlements, StageCompletions: fixture.stageCompletions, DeliveryOutcomes: fixture.delivery, Threads: fixture.threads,
		Ledgers: fixture.ledgers, Projections: fixture.projections, Inspections: fixture.inspections,
		Intents: journal, Settlements: journal, Authority: fixture.authority,
	})
	if err != nil {
		t.Fatal(err)
	}
	plan.Attempts[0].State = RestartAttemptReservedV1
	fixture.delivery = &deliveryProjectionStub{}
	config := fixture.restartApplyConfig()
	config.DeliveryOutcomes = fixture.delivery
	if _, err := ApplyRestartV1(context.Background(), plan, config); !errors.Is(err, ErrPublicationIntegrity) || fixture.delivery.calls != 0 {
		t.Fatalf("tampered restart plan was applied: delivery=%d err=%v", fixture.delivery.calls, err)
	}
}

func TestRestartPlanNestedTamperingFailsClosed(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	if _, err := fixture.service.Publish(context.Background(), fixture.input); err != nil {
		t.Fatal(err)
	}
	journal := &reportRestartJournalStub{intent: &fixture.coordinator.intent, settlement: &fixture.coordinator.settlement}
	plan, err := PreflightRestartV1(context.Background(), RestartPreflightConfigV1{
		Pending:  pendingworkapp.TrustedInventoryV1{Receipts: []domainpendingwork.PendingWorkReceiptV1{fixture.stages.receipt}},
		Attempts: fixture.attempts, Receipts: fixture.receipts, Indexes: fixture.indexes,
		Commits: fixture.commits, Selections: fixture.selections, Decisions: fixture.decisions, GrantSettlements: fixture.grantSettlements, StageCompletions: fixture.stageCompletions, DeliveryOutcomes: fixture.delivery, Threads: fixture.threads,
		Ledgers: fixture.ledgers, Projections: fixture.projections, Inspections: fixture.inspections,
		Intents: journal, Settlements: journal, Authority: fixture.authority,
	})
	if err != nil {
		t.Fatal(err)
	}
	plan.Attempts[0].Stage.GrantMembers[0].GrantID = domainsecurity.SHA256Hex([]byte("tampered-nested-grant"))
	fixture.delivery = &deliveryProjectionStub{}
	if _, err := ApplyRestartV1(context.Background(), plan, fixture.restartApplyConfig()); !errors.Is(err, ErrPublicationIntegrity) || fixture.delivery.calls != 0 {
		t.Fatalf("nested restart plan tampering crossed effects: project=%d err=%v", fixture.delivery.calls, err)
	}
}

func TestRestartPlanSnapshotIsolatedFromInventoryAliases(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	if _, err := fixture.service.Publish(context.Background(), fixture.input); err != nil {
		t.Fatal(err)
	}
	pending := pendingworkapp.TrustedInventoryV1{
		Receipts: []domainpendingwork.PendingWorkReceiptV1{fixture.stages.receipt},
	}
	journal := &reportRestartJournalStub{intent: &fixture.coordinator.intent, settlement: &fixture.coordinator.settlement}
	plan, err := PreflightRestartV1(context.Background(), RestartPreflightConfigV1{
		Pending: pending, Attempts: fixture.attempts, Receipts: fixture.receipts, Indexes: fixture.indexes,
		Commits: fixture.commits, Selections: fixture.selections, Decisions: fixture.decisions, GrantSettlements: fixture.grantSettlements, StageCompletions: fixture.stageCompletions, DeliveryOutcomes: fixture.delivery, Threads: fixture.threads,
		Ledgers: fixture.ledgers, Projections: fixture.projections, Inspections: fixture.inspections,
		Intents: journal, Settlements: journal, Authority: fixture.authority,
	})
	if err != nil {
		t.Fatal(err)
	}
	original := plan.Attempts[0].Stage.GrantMembers[0].GrantID
	pending.Receipts[0].GrantMembers[0].GrantID = domainsecurity.SHA256Hex([]byte("mutated-inventory-alias"))
	if plan.Attempts[0].Stage.GrantMembers[0].GrantID != original {
		t.Fatal("preflight output retained a caller-owned nested slice alias")
	}
	if _, err := snapshotRestartPlanV1(plan); err != nil {
		t.Fatalf("isolated restart plan lost its seal after source mutation: %v", err)
	}
}

func TestReportPublicationRestartPreflightRejectsOrphanMaterialGraph(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	if _, err := fixture.service.Publish(context.Background(), fixture.input); err != nil {
		t.Fatal(err)
	}
	journal := &reportRestartJournalStub{intent: &fixture.coordinator.intent, settlement: &fixture.coordinator.settlement}
	fixture.attempts.records = map[string]domainpublication.PublicationAttemptV1{}
	_, err := PreflightRestartV1(context.Background(), RestartPreflightConfigV1{
		Pending:  pendingworkapp.TrustedInventoryV1{Receipts: []domainpendingwork.PendingWorkReceiptV1{fixture.stages.receipt}},
		Attempts: fixture.attempts, Receipts: fixture.receipts, Indexes: fixture.indexes,
		Commits: fixture.commits, Selections: fixture.selections, Decisions: fixture.decisions, GrantSettlements: fixture.grantSettlements, StageCompletions: fixture.stageCompletions, DeliveryOutcomes: fixture.delivery, Threads: fixture.threads,
		Ledgers: fixture.ledgers, Projections: fixture.projections, Inspections: fixture.inspections,
		Intents: journal, Settlements: journal, Authority: fixture.authority,
	})
	if err == nil || !strings.Contains(err.Error(), "orphan delivery decision") {
		t.Fatalf("orphan publication material graph was accepted: %v", err)
	}
}

func TestApplyReportPublicationRestartBlocksDecisionWithoutSettledGrant(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	if _, err := fixture.service.Publish(context.Background(), fixture.input); err != nil {
		t.Fatal(err)
	}
	journal := &reportRestartJournalStub{intent: &fixture.coordinator.intent, settlement: &fixture.coordinator.settlement}
	plan, err := PreflightRestartV1(context.Background(), RestartPreflightConfigV1{
		Pending:  pendingworkapp.TrustedInventoryV1{Receipts: []domainpendingwork.PendingWorkReceiptV1{fixture.stages.receipt}},
		Attempts: fixture.attempts, Receipts: fixture.receipts, Indexes: fixture.indexes,
		Commits: fixture.commits, Selections: fixture.selections, Decisions: fixture.decisions, GrantSettlements: fixture.grantSettlements, StageCompletions: fixture.stageCompletions, DeliveryOutcomes: fixture.delivery, Threads: fixture.threads,
		Ledgers: fixture.ledgers, Projections: fixture.projections, Inspections: fixture.inspections,
		Intents: journal, Settlements: journal, Authority: fixture.authority,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Attempts) != 1 || plan.Attempts[0].State != RestartAttemptDeliveryDecisionV1 || plan.Attempts[0].Decision == nil {
		t.Fatalf("restart plan omitted the durable delivery decision: %#v", plan.Attempts)
	}
	fixture.delivery.calls = 0
	fixture.delivery.resolveCalls = 0
	config := fixture.restartApplyConfig()
	result, err := ApplyRestartV1(context.Background(), plan, config)
	if !errors.Is(err, ErrPublicationRestartUnresolved) || result.Delivered != 0 || fixture.delivery.calls != 0 || fixture.delivery.resolveCalls != 0 {
		t.Fatalf("commit-only restart was not blocked: result=%#v project=%d resolve=%d err=%v",
			result, fixture.delivery.calls, fixture.delivery.resolveCalls, err)
	}
	result, err = ApplyRestartV1(context.Background(), plan, config)
	if !errors.Is(err, ErrPublicationRestartUnresolved) || result.Delivered != 0 || fixture.delivery.calls != 0 || fixture.delivery.resolveCalls != 0 {
		t.Fatalf("replayed commit-only restart was not blocked: result=%#v project=%d resolve=%d err=%v",
			result, fixture.delivery.calls, fixture.delivery.resolveCalls, err)
	}
}

func TestReportPublicationRestartSettledGrantConvergesThroughCompletedStageAndDelivery(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	result, err := fixture.service.Publish(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	grantSettlement := fixture.installDurableReportGrantSettlement(t, result.Decision)
	plan := fixture.committedRestartPlan(t)
	if len(plan.Attempts) != 1 || plan.Attempts[0].State != RestartAttemptGrantSettlementV1 ||
		plan.Attempts[0].GrantSettlement == nil ||
		plan.Attempts[0].GrantSettlement.RecordDigest != grantSettlement.RecordDigest {
		t.Fatalf("exact grant settlement was not sealed into restart plan: %#v", plan.Attempts)
	}
	fixture.delivery.calls = 0
	fixture.delivery.resolveCalls = 0
	config := fixture.restartApplyConfig()
	for attempt := 1; attempt <= 2; attempt++ {
		replayed, applyErr := ApplyRestartV1(context.Background(), plan, config)
		if applyErr != nil || replayed.Delivered != 1 || fixture.delivery.calls != 1 ||
			fixture.stages.putDispositionCalls != 1 || fixture.stageCompletions.createCalls != 1 ||
			fixture.delivery.projection.DeliveryID == "" {
			t.Fatalf("grant-settled restart did not converge exactly once on replay %d: result=%#v disposition=%d completion=%d project=%d resolve=%d err=%v",
				attempt, replayed, fixture.stages.putDispositionCalls, fixture.stageCompletions.createCalls,
				fixture.delivery.calls, fixture.delivery.resolveCalls, applyErr)
		}
	}
}

func TestReportPublicationRestartRejectsGrantSettlementWhenDurableResultChanges(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	result, err := fixture.service.Publish(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	settlement := fixture.installDurableReportGrantSettlement(t, result.Decision)
	thread := fixture.threads.records[settlement.ThreadID]
	turn := thread["turns"].([]any)[0].(map[string]any)
	items := turn["items"].([]any)
	resultItem := items[len(items)-1].(map[string]any)
	resultItem["output"] = map[string]any{"status": "admitted", "decisionId": domainsecurity.SHA256Hex([]byte("different-decision"))}
	if _, err := fixture.restartPlan(); err == nil || !strings.Contains(err.Error(), "durable result transition") {
		t.Fatalf("changed durable report result retained settlement authority: %v", err)
	}
}

func TestReportPublicationRestartSealedCompletionProjectsOnlyThroughCompletionBoundAdapter(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	result, err := fixture.service.Publish(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	settlement := fixture.installDurableReportGrantSettlement(t, result.Decision)
	completion := fixture.installDurableReportStageCompletion(t, result.Decision, settlement)
	plan := fixture.committedRestartPlan(t)
	if len(plan.Attempts) != 1 || plan.Attempts[0].State != RestartAttemptStageCompletionV1 ||
		plan.Attempts[0].Disposition == nil || plan.Attempts[0].StageCompletion == nil ||
		plan.Attempts[0].StageCompletion.RecordDigest != completion.RecordDigest {
		t.Fatalf("exact report stage completion was not sealed into restart plan: %#v", plan.Attempts)
	}
	fixture.delivery.calls = 0
	fixture.delivery.resolveCalls = 0
	for replay := 1; replay <= 2; replay++ {
		applied, applyErr := ApplyRestartV1(context.Background(), plan, fixture.restartApplyConfig())
		if applyErr != nil || applied.Delivered != 1 || fixture.delivery.calls != 1 ||
			fixture.delivery.projection.CompletionID != completion.CompletionID {
			t.Fatalf("stage completion did not converge through the completion-bound projection on replay %d: result=%#v project=%d resolve=%d err=%v",
				replay, applied, fixture.delivery.calls, fixture.delivery.resolveCalls, applyErr)
		}
	}
}

func TestReportPublicationRestartSuffixACKLossUsesExactReadback(t *testing.T) {
	testCases := []struct {
		name   string
		inject func(*reportPublicationFixture, error)
	}{
		{
			name: "stage disposition",
			inject: func(fixture *reportPublicationFixture, ackLost error) {
				fixture.stages.putDispositionErr = ackLost
				fixture.stages.putDispositionCommitOnErr = true
			},
		},
		{
			name: "stage completion",
			inject: func(fixture *reportPublicationFixture, ackLost error) {
				fixture.stageCompletions.createErr = ackLost
				fixture.stageCompletions.commitOnErr = true
			},
		},
		{
			name: "delivery projection",
			inject: func(fixture *reportPublicationFixture, ackLost error) {
				fixture.delivery.err = ackLost
				fixture.delivery.commitOnErr = true
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
			published, err := fixture.service.Publish(context.Background(), fixture.input)
			if err != nil {
				t.Fatal(err)
			}
			fixture.installDurableReportGrantSettlement(t, published.Decision)
			plan := fixture.committedRestartPlan(t)
			testCase.inject(fixture, errors.New("durable write committed but acknowledgement was lost"))
			result, err := ApplyRestartV1(context.Background(), plan, fixture.restartApplyConfig())
			if err != nil || result.Delivered != 1 || fixture.delivery.projection.DeliveryID == "" ||
				fixture.stages.disposition == nil || len(fixture.stageCompletions.records) != 1 {
				t.Fatalf("exact ACK-loss readback did not converge: result=%#v disposition=%#v completions=%d projection=%#v err=%v",
					result, fixture.stages.disposition, len(fixture.stageCompletions.records), fixture.delivery.projection, err)
			}
		})
	}
}

func TestReportPublicationRestartUncommittedSuffixFailureResumesSameSealedPlan(t *testing.T) {
	testCases := []struct {
		name        string
		inject      func(*reportPublicationFixture, error)
		clear       func(*reportPublicationFixture)
		assertFirst func(*testing.T, *reportPublicationFixture)
	}{
		{
			name: "stage disposition",
			inject: func(fixture *reportPublicationFixture, failure error) {
				fixture.stages.putDispositionErr = failure
			},
			clear: func(fixture *reportPublicationFixture) { fixture.stages.putDispositionErr = nil },
			assertFirst: func(t *testing.T, fixture *reportPublicationFixture) {
				if fixture.stages.disposition != nil || len(fixture.stageCompletions.records) != 0 || fixture.delivery.calls != 0 {
					t.Fatalf("uncommitted disposition failure crossed a later effect")
				}
			},
		},
		{
			name: "stage completion",
			inject: func(fixture *reportPublicationFixture, failure error) {
				fixture.stageCompletions.createErr = failure
			},
			clear: func(fixture *reportPublicationFixture) { fixture.stageCompletions.createErr = nil },
			assertFirst: func(t *testing.T, fixture *reportPublicationFixture) {
				if fixture.stages.disposition == nil || len(fixture.stageCompletions.records) != 0 || fixture.delivery.calls != 0 {
					t.Fatalf("uncommitted completion failure did not stop at its exact predecessor")
				}
			},
		},
		{
			name: "delivery projection",
			inject: func(fixture *reportPublicationFixture, failure error) {
				fixture.delivery.err = failure
			},
			clear: func(fixture *reportPublicationFixture) { fixture.delivery.err = nil },
			assertFirst: func(t *testing.T, fixture *reportPublicationFixture) {
				if fixture.stages.disposition == nil || len(fixture.stageCompletions.records) != 1 || fixture.delivery.projection.DeliveryID != "" {
					t.Fatalf("uncommitted delivery failure did not preserve only its exact predecessor")
				}
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
			published, err := fixture.service.Publish(context.Background(), fixture.input)
			if err != nil {
				t.Fatal(err)
			}
			fixture.installDurableReportGrantSettlement(t, published.Decision)
			plan := fixture.committedRestartPlan(t)
			testCase.inject(fixture, errors.New("write did not commit"))
			first, firstErr := ApplyRestartV1(context.Background(), plan, fixture.restartApplyConfig())
			if !errors.Is(firstErr, ErrPublicationRestartUnresolved) || first.Delivered != 0 {
				t.Fatalf("uncommitted suffix failure was not unresolved: result=%#v err=%v", first, firstErr)
			}
			testCase.assertFirst(t, fixture)
			testCase.clear(fixture)
			second, secondErr := ApplyRestartV1(context.Background(), plan, fixture.restartApplyConfig())
			if secondErr != nil || second.Delivered != 1 || fixture.delivery.projection.DeliveryID == "" {
				t.Fatalf("same sealed plan did not resume after exact predecessor readback: result=%#v projection=%#v err=%v",
					second, fixture.delivery.projection, secondErr)
			}
		})
	}
}

func TestReportPublicationRestartOutcomeUnknownDispositionNeverProjects(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	published, err := fixture.service.Publish(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	settlement := fixture.installDurableReportGrantSettlement(t, published.Decision)
	plan := fixture.committedRestartPlan(t)
	settledAt, err := time.Parse(time.RFC3339Nano, settlement.SettledAt)
	if err != nil {
		t.Fatal(err)
	}
	disposition, err := domainpendingwork.NewPendingWorkDispositionV1(
		fixture.stages.receipt, domainpendingwork.StatusOutcomeUnknown,
		"report_stage_outcome_unknown_after_restart", settledAt,
		fixture.authority.keyID, fixture.authority.publicKey,
		func(message []byte) ([]byte, error) { return fixture.authority.Sign(context.Background(), message) },
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture.stages.disposition = &disposition
	result, err := ApplyRestartV1(context.Background(), plan, fixture.restartApplyConfig())
	if err != nil || result.DeliveryBlocked != 1 || result.Delivered != 0 ||
		len(fixture.stageCompletions.records) != 0 || fixture.delivery.calls != 0 {
		t.Fatalf("outcome-unknown report stage crossed delivery: result=%#v completions=%d project=%d err=%v",
			result, len(fixture.stageCompletions.records), fixture.delivery.calls, err)
	}
	preflight, err := fixture.restartPlan()
	if err != nil || len(preflight.Attempts) != 1 || preflight.Attempts[0].State != RestartAttemptDeliveryBlockedV1 {
		t.Fatalf("outcome-unknown disposition was not durably classified as blocked: plan=%#v err=%v", preflight.Attempts, err)
	}
}

func TestReportPublicationRestartConcurrentExactReplayConvergesSingleSuffixGraph(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	published, err := fixture.service.Publish(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	fixture.installDurableReportGrantSettlement(t, published.Decision)
	plan := fixture.committedRestartPlan(t)
	config := fixture.restartApplyConfig()

	const workers = 8
	start := make(chan struct{})
	results := make(chan RestartApplyResultV1, workers)
	errorsSeen := make(chan error, workers)
	var group sync.WaitGroup
	group.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer group.Done()
			<-start
			result, applyErr := ApplyRestartV1(context.Background(), plan, config)
			results <- result
			errorsSeen <- applyErr
		}()
	}
	close(start)
	group.Wait()
	close(results)
	close(errorsSeen)
	for applyErr := range errorsSeen {
		if applyErr != nil {
			t.Fatalf("concurrent exact restart failed: %v", applyErr)
		}
	}
	for result := range results {
		if result.Delivered != 1 {
			t.Fatalf("concurrent exact restart result = %#v", result)
		}
	}
	if fixture.stages.putDispositionCalls != 1 || fixture.stageCompletions.createCalls != 1 ||
		fixture.delivery.calls != 1 || fixture.delivery.projection.DeliveryID == "" {
		t.Fatalf("concurrent restart did not converge to one suffix graph: dispositions=%d completions=%d projections=%d projection=%#v",
			fixture.stages.putDispositionCalls, fixture.stageCompletions.createCalls,
			fixture.delivery.calls, fixture.delivery.projection)
	}
}

func TestReportPublicationRestartFinalDeliverySerializesWithCaseEpochTransition(t *testing.T) {
	mutations := []struct {
		name       string
		switchCase bool
	}{
		{name: "epoch bump", switchCase: false},
		{name: "case switch", switchCase: true},
	}
	for _, mutation := range mutations {
		for _, winner := range []string{"delivery", "transition"} {
			t.Run(mutation.name+"/"+winner+" wins", func(t *testing.T) {
				fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
				published, err := fixture.service.Publish(context.Background(), fixture.input)
				if err != nil {
					t.Fatal(err)
				}
				settlement := fixture.installDurableReportGrantSettlement(t, published.Decision)
				fixture.installDurableReportStageCompletion(t, published.Decision, settlement)
				plan := fixture.committedRestartPlan(t)
				if len(plan.Attempts) != 1 || plan.Attempts[0].State != RestartAttemptStageCompletionV1 {
					t.Fatalf("restart plan is not at the final delivery cut: %#v", plan.Attempts)
				}

				state := subagentapp.NewRuntimeState()
				next := nextReportSecurityContext(t, fixture.input.PendingToolCall.SecurityContext, fixture.now, mutation.switchCase)
				delegate := &deliveryProjectionStub{}
				probe := newDeliveryProjectionRaceProbe(delegate)
				config := fixture.restartApplyConfig()
				config.DeliveryOutcomes = probe
				effectAttempted := make(chan struct{})
				effectAcquired := make(chan struct{})
				var attemptOnce sync.Once
				var acquireOnce sync.Once
				config.AcquireContextEffect = func(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
					attemptOnce.Do(func() { close(effectAttempted) })
					effectCtx, release, acquireErr := state.AcquireContextEffect(ctx, securityContext)
					if acquireErr == nil {
						acquireOnce.Do(func() { close(effectAcquired) })
					}
					return effectCtx, release, acquireErr
				}

				if winner == "delivery" {
					runDeliveryWinsReportTransitionRace(t, fixture, state, next, plan, config, probe, delegate, effectAcquired)
					return
				}
				runTransitionWinsReportDeliveryRace(t, fixture, state, next, plan, config, delegate, effectAttempted, effectAcquired)
			})
		}
	}
}

func TestRestartPlanDecisionTamperingFailsClosedBeforeEffects(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	if _, err := fixture.service.Publish(context.Background(), fixture.input); err != nil {
		t.Fatal(err)
	}
	plan := fixture.committedRestartPlan(t)
	if len(plan.Attempts) != 1 || plan.Attempts[0].Decision == nil {
		t.Fatalf("restart plan lacks decision: %#v", plan.Attempts)
	}
	plan.Attempts[0].Decision.CommitRecordDigest = domainsecurity.SHA256Hex([]byte("tampered-decision-commit"))
	fixture.delivery = &deliveryProjectionStub{}
	if _, err := ApplyRestartV1(context.Background(), plan, fixture.restartApplyConfig()); !errors.Is(err, ErrPublicationIntegrity) || fixture.delivery.calls != 0 || fixture.delivery.resolveCalls != 0 {
		t.Fatalf("tampered decision crossed restart effects: project=%d resolve=%d err=%v",
			fixture.delivery.calls, fixture.delivery.resolveCalls, err)
	}
}

func TestControlledPIIRevokedAfterWitnessCommitCannotRestartDeliver(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionControlledFull)
	if _, err := fixture.service.Publish(context.Background(), fixture.input); err != nil {
		t.Fatal(err)
	}
	journal := &reportRestartJournalStub{intent: &fixture.coordinator.intent, settlement: &fixture.coordinator.settlement}
	plan, err := PreflightRestartV1(context.Background(), RestartPreflightConfigV1{
		Pending:  pendingworkapp.TrustedInventoryV1{Receipts: []domainpendingwork.PendingWorkReceiptV1{fixture.stages.receipt}},
		Attempts: fixture.attempts, Receipts: fixture.receipts, Indexes: fixture.indexes,
		Commits: fixture.commits, Selections: fixture.selections, Decisions: fixture.decisions, GrantSettlements: fixture.grantSettlements, StageCompletions: fixture.stageCompletions, DeliveryOutcomes: fixture.delivery, Threads: fixture.threads,
		Ledgers: fixture.ledgers, Projections: fixture.projections, Inspections: fixture.inspections,
		Intents: journal, Settlements: journal, Authority: fixture.authority,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.delivery = &deliveryProjectionStub{}
	// Restart must revalidate the controlled artifact authorization before it
	// can even remain eligible for later settlement and delivery.
	fixture.pii.failAt = fixture.pii.calls + 1
	if _, err := ApplyRestartV1(context.Background(), plan, fixture.restartApplyConfig()); err == nil ||
		fixture.delivery.calls != 0 || fixture.contexts.calls != 1 || fixture.pii.calls != fixture.pii.failAt {
		t.Fatalf("revoked controlled PII restarted into delivery: project=%d context=%d pii=%d failAt=%d err=%v",
			fixture.delivery.calls, fixture.contexts.calls, fixture.pii.calls, fixture.pii.failAt, err)
	}
}

func TestControlledPIIAuthorityRejectedInsideFinalRestartSnapshotRemainsUnresolved(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionControlledFull)
	published, err := fixture.service.Publish(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	settlement := fixture.installDurableReportGrantSettlement(t, published.Decision)
	fixture.installDurableReportStageCompletion(t, published.Decision, settlement)
	plan := fixture.committedRestartPlan(t)
	if len(plan.Attempts) != 1 || plan.Attempts[0].State != RestartAttemptStageCompletionV1 {
		t.Fatalf("controlled restart did not reach the final delivery cut: %#v", plan.Attempts)
	}
	fixture.pii.mu.Lock()
	fixture.pii.failWitnessedAt = fixture.pii.witnessedCalls + 1
	wantWitnessed := fixture.pii.failWitnessedAt
	fixture.pii.mu.Unlock()

	result, err := ApplyRestartV1(context.Background(), plan, fixture.restartApplyConfig())
	fixture.pii.mu.Lock()
	witnessedCalls := fixture.pii.witnessedCalls
	fixture.pii.mu.Unlock()
	if !errors.Is(err, ErrPublicationRestartUnresolved) || result.Delivered != 0 || result.DeliveryRejected != 0 || witnessedCalls != wantWitnessed ||
		fixture.delivery.calls != 0 || fixture.delivery.projection.DeliveryID != "" {
		t.Fatalf("untyped final witnessed PII rejection crossed or terminalized delivery: result=%#v witnessed=%d want=%d project=%d projection=%#v err=%v",
			result, witnessedCalls, wantWitnessed, fixture.delivery.calls, fixture.delivery.projection, err)
	}
}

func TestEvidenceChangedDurablyRejectsCompletedDelivery(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	published, err := fixture.service.Publish(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	settlement := fixture.installDurableReportGrantSettlement(t, published.Decision)
	fixture.installDurableReportStageCompletion(t, published.Decision, settlement)
	plan := fixture.committedRestartPlan(t)
	if len(plan.Attempts) != 1 || plan.Attempts[0].State != RestartAttemptStageCompletionV1 {
		t.Fatalf("restart did not reach completed-stage cut: %#v", plan.Attempts)
	}
	fixture.evidence.mu.Lock()
	fixture.evidence.third = fixture.revokedSnapshotAfterPublication(t)
	fixture.evidence.headReader = nil
	fixture.evidence.mu.Unlock()

	result, err := ApplyRestartV1(context.Background(), plan, fixture.restartApplyConfig())
	fixture.delivery.mu.Lock()
	winner := fixture.delivery.outcome
	createCalls := fixture.delivery.calls
	fixture.delivery.mu.Unlock()
	if err != nil || result.Delivered != 0 || result.DeliveryRejected != 1 || createCalls != 1 ||
		winner.Kind != domainpublication.ReportDeliveryOutcomeRejectedV1 || winner.Rejection == nil ||
		winner.Rejection.ReasonCode != domainpublication.ReportDeliveryRejectionEvidenceChangedV1 ||
		winner.Rejection.EvidenceRegistryStateDigest == published.Decision.EvidenceRegistryStateDigest {
		t.Fatalf("changed evidence did not converge to one private rejection: result=%#v winner=%#v calls=%d err=%v",
			result, winner, createCalls, err)
	}

	rejectedPlan := fixture.committedRestartPlan(t)
	if len(rejectedPlan.Attempts) != 1 || rejectedPlan.Attempts[0].State != RestartAttemptDeliveryRejectionV1 {
		t.Fatalf("durable rejection was not recovered as terminal history: %#v", rejectedPlan.Attempts)
	}
	current := nextReportSecurityContext(t, fixture.input.PendingToolCall.SecurityContext, fixture.now, true)
	setRestartCurrentContext(fixture.contexts, current)
	config := fixture.restartApplyConfig()
	config.HeadReader = unavailableFreshHeadReaderV1{}
	config.AcquireContextEffect = func(context.Context, domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
		return nil, nil, errors.New("historical rejection requested a current effect lease")
	}
	replayed, err := ApplyRestartV1(context.Background(), rejectedPlan, config)
	fixture.delivery.mu.Lock()
	replayCreateCalls := fixture.delivery.calls
	fixture.delivery.mu.Unlock()
	if err != nil || replayed.DeliveryRejected != 1 || replayed.Delivered != 0 || replayCreateCalls != createCalls {
		t.Fatalf("historical rejection was reauthorized or rewritten after case switch: result=%#v calls=%d err=%v",
			replayed, replayCreateCalls, err)
	}
}

func TestEvidenceChangedRejectionACKLossUsesExactSharedReadback(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	published, err := fixture.service.Publish(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	settlement := fixture.installDurableReportGrantSettlement(t, published.Decision)
	fixture.installDurableReportStageCompletion(t, published.Decision, settlement)
	plan := fixture.committedRestartPlan(t)
	fixture.evidence.mu.Lock()
	fixture.evidence.third = fixture.revokedSnapshotAfterPublication(t)
	fixture.evidence.headReader = nil
	fixture.evidence.mu.Unlock()
	fixture.delivery.err = errors.New("delivery rejection acknowledgement lost")
	fixture.delivery.commitOnErr = true

	result, err := ApplyRestartV1(context.Background(), plan, fixture.restartApplyConfig())
	fixture.delivery.mu.Lock()
	winner := fixture.delivery.outcome
	fixture.delivery.mu.Unlock()
	if err != nil || result.DeliveryRejected != 1 || result.Delivered != 0 ||
		winner.Kind != domainpublication.ReportDeliveryOutcomeRejectedV1 || winner.Rejection == nil ||
		winner.Rejection.ReasonCode != domainpublication.ReportDeliveryRejectionEvidenceChangedV1 {
		t.Fatalf("rejection ACK-loss did not converge through exact shared readback: result=%#v winner=%#v err=%v",
			result, winner, err)
	}
}

func TestHistoricalProjectionDoesNotRegainCurrentAuthorityAfterCaseSwitch(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	published, err := fixture.service.Publish(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	settlement := fixture.installDurableReportGrantSettlement(t, published.Decision)
	fixture.installDurableReportStageCompletion(t, published.Decision, settlement)
	completedPlan := fixture.committedRestartPlan(t)
	first, err := ApplyRestartV1(context.Background(), completedPlan, fixture.restartApplyConfig())
	if err != nil || first.Delivered != 1 {
		t.Fatalf("initial completed delivery did not project: result=%#v err=%v", first, err)
	}
	projectedPlan := fixture.committedRestartPlan(t)
	if len(projectedPlan.Attempts) != 1 || projectedPlan.Attempts[0].State != RestartAttemptDeliveryProjectionV1 {
		t.Fatalf("durable projection was not recovered as terminal history: %#v", projectedPlan.Attempts)
	}
	current := nextReportSecurityContext(t, fixture.input.PendingToolCall.SecurityContext, fixture.now, true)
	setRestartCurrentContext(fixture.contexts, current)
	config := fixture.restartApplyConfig()
	config.HeadReader = unavailableFreshHeadReaderV1{}
	config.AcquireContextEffect = func(context.Context, domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
		return nil, nil, errors.New("historical projection requested a current effect lease")
	}
	replayed, err := ApplyRestartV1(context.Background(), projectedPlan, config)
	if err != nil || replayed.Delivered != 1 || replayed.DeliveryRejected != 0 || fixture.delivery.calls != 1 {
		t.Fatalf("historical projection was reauthorized or rewritten: result=%#v calls=%d err=%v",
			replayed, fixture.delivery.calls, err)
	}
}

type unavailableFreshHeadReaderV1 struct{}

func (unavailableFreshHeadReaderV1) ObserveFresh(context.Context) (evidenceauthorityport.FreshHead, error) {
	return evidenceauthorityport.FreshHead{}, errors.New("historical terminal requested a current live head")
}

func TestRestartCaseRegistrySequenceIsIndependentFromGlobalRegistryGeneration(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	published, err := fixture.service.Publish(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	settlement := fixture.installDurableReportGrantSettlement(t, published.Decision)
	fixture.installDurableReportStageCompletion(t, published.Decision, settlement)
	plan := fixture.committedRestartPlan(t)
	current := fixture.unrelatedRegistryAdvanceSnapshot(t)
	if current.Registry.Sequence == current.Head.Bundle.EvidenceRegistryCount {
		t.Fatalf("test fixture did not separate case sequence from global generation: sequence=%d global=%d",
			current.Registry.Sequence, current.Head.Bundle.EvidenceRegistryCount)
	}
	fixture.evidence.mu.Lock()
	fixture.evidence.third = current
	fixture.evidence.headReader = nil
	fixture.evidence.mu.Unlock()

	result, err := ApplyRestartV1(context.Background(), plan, fixture.restartApplyConfig())
	if err != nil || result.Delivered != 1 || fixture.delivery.projection.DeliveryID == "" {
		t.Fatalf("unrelated case registry advance blocked a still-supported report: result=%#v projection=%#v err=%v",
			result, fixture.delivery.projection, err)
	}
}

func TestRestartRejectsCommitWithSyntacticButNonExactWitnessBinding(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	result, err := fixture.service.Publish(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	journal := &reportRestartJournalStub{intent: &fixture.coordinator.intent, settlement: &fixture.coordinator.settlement}
	plan, err := PreflightRestartV1(context.Background(), RestartPreflightConfigV1{
		Pending:  pendingworkapp.TrustedInventoryV1{Receipts: []domainpendingwork.PendingWorkReceiptV1{fixture.stages.receipt}},
		Attempts: fixture.attempts, Receipts: fixture.receipts, Indexes: fixture.indexes,
		Commits: fixture.commits, Selections: fixture.selections, Decisions: fixture.decisions, GrantSettlements: fixture.grantSettlements, StageCompletions: fixture.stageCompletions, DeliveryOutcomes: fixture.delivery, Threads: fixture.threads,
		Ledgers: fixture.ledgers, Projections: fixture.projections, Inspections: fixture.inspections,
		Intents: journal, Settlements: journal, Authority: fixture.authority,
	})
	if err != nil {
		t.Fatal(err)
	}
	other, err := fixture.coordinator.ObserveFresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	fixture.observations.records[result.Commit.WitnessBinding.ObservationDigest] = evidenceauthorityport.ObservationBundle{
		Bundle: other.Bundle, Request: other.Request, Observation: other.Observation,
	}
	fixture.delivery = &deliveryProjectionStub{}
	if _, err := ApplyRestartV1(context.Background(), plan, fixture.restartApplyConfig()); !errors.Is(err, ErrPublicationIntegrity) || fixture.delivery.calls != 0 {
		t.Fatalf("non-exact stored witness binding reached delivery: project=%d err=%v", fixture.delivery.calls, err)
	}
}

func TestRestartRejectsCommitWhoseResolvedBundleDoesNotMatchCommittedFields(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	result, err := fixture.service.Publish(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	journal := &reportRestartJournalStub{intent: &fixture.coordinator.intent, settlement: &fixture.coordinator.settlement}
	plan, err := PreflightRestartV1(context.Background(), RestartPreflightConfigV1{
		Pending:  pendingworkapp.TrustedInventoryV1{Receipts: []domainpendingwork.PendingWorkReceiptV1{fixture.stages.receipt}},
		Attempts: fixture.attempts, Receipts: fixture.receipts, Indexes: fixture.indexes,
		Commits: fixture.commits, Selections: fixture.selections, Decisions: fixture.decisions, GrantSettlements: fixture.grantSettlements, StageCompletions: fixture.stageCompletions, DeliveryOutcomes: fixture.delivery, Threads: fixture.threads,
		Ledgers: fixture.ledgers, Projections: fixture.projections, Inspections: fixture.inspections,
		Intents: journal, Settlements: journal, Authority: fixture.authority,
	})
	if err != nil {
		t.Fatal(err)
	}
	tampered := fixture.bundles.records[result.Commit.CommittedEvidenceBundleDigest]
	tampered.PublicationCount++
	fixture.bundles.records[result.Commit.CommittedEvidenceBundleDigest] = tampered
	fixture.delivery = &deliveryProjectionStub{}
	if _, err := ApplyRestartV1(context.Background(), plan, fixture.restartApplyConfig()); !errors.Is(err, ErrPublicationIntegrity) || fixture.delivery.calls != 0 {
		t.Fatalf("commit with mismatched stored bundle reached delivery: project=%d err=%v", fixture.delivery.calls, err)
	}
}

func TestRestartRechecksCurrentContextImmediatelyBeforeDelivery(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	if _, err := fixture.service.Publish(context.Background(), fixture.input); err != nil {
		t.Fatal(err)
	}
	journal := &reportRestartJournalStub{intent: &fixture.coordinator.intent, settlement: &fixture.coordinator.settlement}
	plan, err := PreflightRestartV1(context.Background(), RestartPreflightConfigV1{
		Pending:  pendingworkapp.TrustedInventoryV1{Receipts: []domainpendingwork.PendingWorkReceiptV1{fixture.stages.receipt}},
		Attempts: fixture.attempts, Receipts: fixture.receipts, Indexes: fixture.indexes,
		Commits: fixture.commits, Selections: fixture.selections, Decisions: fixture.decisions, GrantSettlements: fixture.grantSettlements, StageCompletions: fixture.stageCompletions, DeliveryOutcomes: fixture.delivery, Threads: fixture.threads,
		Ledgers: fixture.ledgers, Projections: fixture.projections, Inspections: fixture.inspections,
		Intents: journal, Settlements: journal, Authority: fixture.authority,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.contexts.failAt = 1
	fixture.delivery = &deliveryProjectionStub{}
	if _, err := ApplyRestartV1(context.Background(), plan, fixture.restartApplyConfig()); err == nil ||
		fixture.contexts.calls != 1 || fixture.delivery.calls != 0 {
		t.Fatalf("stale restart context crossed delivery: context=%d project=%d err=%v",
			fixture.contexts.calls, fixture.delivery.calls, err)
	}
}

func TestApplyReportPublicationRestartReissuesCommitAfterCommittedSettlement(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	if _, err := fixture.service.Publish(context.Background(), fixture.input); err != nil {
		t.Fatal(err)
	}
	fixture.commits.records = map[string]domainpublication.PublicationCommitReceiptV1{}
	fixture.decisions.records = map[string]domainpublication.ReportDeliveryDecisionV1{}
	fixture.delivery.commit = domainpublication.PublicationCommitReceiptV1{}
	journal := &reportRestartJournalStub{intent: &fixture.coordinator.intent, settlement: &fixture.coordinator.settlement}
	plan, err := PreflightRestartV1(context.Background(), RestartPreflightConfigV1{
		Pending:  pendingworkapp.TrustedInventoryV1{Receipts: []domainpendingwork.PendingWorkReceiptV1{fixture.stages.receipt}},
		Attempts: fixture.attempts, Receipts: fixture.receipts, Indexes: fixture.indexes,
		Commits: fixture.commits, Selections: fixture.selections, Decisions: fixture.decisions, GrantSettlements: fixture.grantSettlements, StageCompletions: fixture.stageCompletions, DeliveryOutcomes: fixture.delivery, Threads: fixture.threads,
		Ledgers: fixture.ledgers, Projections: fixture.projections, Inspections: fixture.inspections,
		Intents: journal, Settlements: journal, Authority: fixture.authority,
	})
	if err != nil || len(plan.Attempts) != 1 || plan.Attempts[0].State != RestartAttemptCommitSelectionV1 {
		t.Fatalf("committed-settlement restart plan = %#v err=%v", plan, err)
	}
	result, err := ApplyRestartV1(context.Background(), plan, fixture.restartApplyConfig())
	if !errors.Is(err, ErrPublicationRestartUnresolved) || result.IssuedCommits != 1 || result.Delivered != 0 || len(fixture.commits.records) != 1 {
		t.Fatalf("commit restart reconciliation = %#v commits=%d err=%v", result, len(fixture.commits.records), err)
	}
}

func TestRestartCommittedSettlementPlanCannotIssueSecondCommit(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	if _, err := fixture.service.Publish(context.Background(), fixture.input); err != nil {
		t.Fatal(err)
	}
	fixture.commits.records = map[string]domainpublication.PublicationCommitReceiptV1{}
	fixture.selections.records = map[string]domainpublication.PublicationCommitSelectionV1{}
	fixture.decisions.records = map[string]domainpublication.ReportDeliveryDecisionV1{}
	fixture.delivery = &deliveryProjectionStub{}
	journal := &reportRestartJournalStub{intent: &fixture.coordinator.intent, settlement: &fixture.coordinator.settlement}
	plan, err := PreflightRestartV1(context.Background(), RestartPreflightConfigV1{
		Pending:  pendingworkapp.TrustedInventoryV1{Receipts: []domainpendingwork.PendingWorkReceiptV1{fixture.stages.receipt}},
		Attempts: fixture.attempts, Receipts: fixture.receipts, Indexes: fixture.indexes,
		Commits: fixture.commits, Selections: fixture.selections, Decisions: fixture.decisions, GrantSettlements: fixture.grantSettlements, StageCompletions: fixture.stageCompletions, DeliveryOutcomes: fixture.delivery, Threads: fixture.threads,
		Ledgers: fixture.ledgers, Projections: fixture.projections, Inspections: fixture.inspections,
		Intents: journal, Settlements: journal, Authority: fixture.authority,
	})
	if err != nil || len(plan.Attempts) != 1 || plan.Attempts[0].State != RestartAttemptCommittedSettlementV1 {
		t.Fatalf("unselected committed restart plan = %#v err=%v", plan, err)
	}
	first, err := ApplyRestartV1(context.Background(), plan, fixture.restartApplyConfig())
	if !errors.Is(err, ErrPublicationRestartUnresolved) || first.IssuedCommits != 1 || first.Delivered != 0 {
		t.Fatalf("first selected restart = %#v err=%v", first, err)
	}
	second, err := ApplyRestartV1(context.Background(), plan, fixture.restartApplyConfig())
	if !errors.Is(err, ErrPublicationRestartUnresolved) || second.IssuedCommits != 0 || second.Delivered != 0 ||
		len(fixture.selections.records) != 1 || len(fixture.commits.records) != 1 || fixture.delivery.calls != 0 {
		t.Fatalf("replayed plan selected another commit or projected without decision: second=%#v selections=%d commits=%d delivery=%d err=%v",
			second, len(fixture.selections.records), len(fixture.commits.records), fixture.delivery.calls, err)
	}
}

func TestConcurrentPublicationCommitSelectionSignsOneCommitIdentity(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	result, err := fixture.service.Publish(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	attempt := fixture.onlyPublicationAttempt(t)
	fixture.resetCommitSelectionStores()
	fixture.authority.resetCommitSigningDigests()

	firstHead, err := reportFreshHeadValue(
		fixture.coordinator.bundle, fixture.authority, fixture.coordinator.witnessPrivate,
		fixture.coordinator.witnessPublic, "concurrent-selection-a", 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	secondHead, err := reportFreshHeadValue(
		fixture.coordinator.bundle, fixture.authority, fixture.coordinator.witnessPrivate,
		fixture.coordinator.witnessPublic, "concurrent-selection-b", 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	fixture.selections.beforeCreate = func() {
		ready <- struct{}{}
		<-release
	}
	type selectionResult struct {
		commit domainpublication.PublicationCommitReceiptV1
		err    error
	}
	results := make(chan selectionResult, 2)
	for _, head := range []evidenceauthorityport.FreshHead{firstHead, secondHead} {
		head := head
		go func() {
			commit, _, selectErr := selectAndSignPublicationCommitV1(
				context.Background(), attempt, result.Candidate, result.Index,
				fixture.coordinator.settlement.RecordDigest, &head, fixture.commitSelectorConfig(),
			)
			results <- selectionResult{commit: commit, err: selectErr}
		}()
	}
	for range 2 {
		select {
		case <-ready:
		case <-time.After(5 * time.Second):
			close(release)
			t.Fatal("concurrent selectors did not both reach the exclusive create boundary")
		}
	}
	close(release)
	first := <-results
	second := <-results
	if first.err != nil || second.err != nil || first.commit.RecordDigest == "" ||
		first.commit.RecordDigest != second.commit.RecordDigest {
		t.Fatalf("concurrent selectors diverged: first=%s firstErr=%v second=%s secondErr=%v",
			first.commit.RecordDigest, first.err, second.commit.RecordDigest, second.err)
	}
	fixture.selections.mu.Lock()
	selectionCount := len(fixture.selections.records)
	fixture.selections.mu.Unlock()
	fixture.commits.mu.Lock()
	commitCount := len(fixture.commits.records)
	fixture.commits.mu.Unlock()
	if selectionCount != 1 || commitCount != 1 || fixture.authority.distinctCommitSigningDigests() != 1 {
		t.Fatalf("concurrent selection issued multiple commit identities: selections=%d commits=%d signingDigests=%d",
			selectionCount, commitCount, fixture.authority.distinctCommitSigningDigests())
	}
}

func TestPublicationCommitSelectionAcknowledgementLossRequiresExactReadback(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		commitOnErr bool
		wantSuccess bool
	}{
		{name: "durable write with lost acknowledgement", commitOnErr: true, wantSuccess: true},
		{name: "lost write with lost acknowledgement", commitOnErr: false, wantSuccess: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
			result, err := fixture.service.Publish(context.Background(), fixture.input)
			if err != nil {
				t.Fatal(err)
			}
			attempt := fixture.onlyPublicationAttempt(t)
			fixture.resetCommitSelectionStores()
			fixture.authority.resetCommitSigningDigests()
			fixture.selections.createErr = errors.New("selection acknowledgement lost")
			fixture.selections.commitOnErr = testCase.commitOnErr
			head, headErr := reportFreshHeadValue(
				fixture.coordinator.bundle, fixture.authority, fixture.coordinator.witnessPrivate,
				fixture.coordinator.witnessPublic, "selection-ack-loss", 1,
			)
			if headErr != nil {
				t.Fatal(headErr)
			}
			commit, _, selectErr := selectAndSignPublicationCommitV1(
				context.Background(), attempt, result.Candidate, result.Index,
				fixture.coordinator.settlement.RecordDigest, &head, fixture.commitSelectorConfig(),
			)
			fixture.selections.mu.Lock()
			selectionCount := len(fixture.selections.records)
			fixture.selections.mu.Unlock()
			fixture.commits.mu.Lock()
			commitCount := len(fixture.commits.records)
			fixture.commits.mu.Unlock()
			if testCase.wantSuccess {
				if selectErr != nil || domainpublication.ValidatePublicationCommitReceiptV1(commit) != nil ||
					selectionCount != 1 || commitCount != 1 || fixture.authority.distinctCommitSigningDigests() != 1 {
					t.Fatalf("durable selector ACK loss did not reconcile exactly: commit=%s selections=%d commits=%d signs=%d err=%v",
						commit.RecordDigest, selectionCount, commitCount, fixture.authority.distinctCommitSigningDigests(), selectErr)
				}
				return
			}
			if !errors.Is(selectErr, ErrPublicationRestartUnresolved) || commit.RecordDigest != "" ||
				selectionCount != 0 || commitCount != 0 || fixture.authority.distinctCommitSigningDigests() != 0 {
				t.Fatalf("unresolved selector ACK loss crossed commit signing: commit=%s selections=%d commits=%d signs=%d err=%v",
					commit.RecordDigest, selectionCount, commitCount, fixture.authority.distinctCommitSigningDigests(), selectErr)
			}
		})
	}
}

func TestPublicationCommitSelectionClassifiesCancellationAndRegistryUnavailability(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	result, err := fixture.service.Publish(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	attempt := fixture.onlyPublicationAttempt(t)
	fixture.resetCommitSelectionStores()
	fixture.authority.resetCommitSigningDigests()
	head, err := reportFreshHeadValue(
		fixture.coordinator.bundle, fixture.authority, fixture.coordinator.witnessPrivate,
		fixture.coordinator.witnessPublic, "selector-error-classification", 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := selectAndSignPublicationCommitV1(
		cancelled, attempt, result.Candidate, result.Index, fixture.coordinator.settlement.RecordDigest,
		&head, fixture.commitSelectorConfig(),
	); !errors.Is(err, context.Canceled) || fixture.authority.distinctCommitSigningDigests() != 0 {
		t.Fatalf("cancelled selector was misclassified or signed: signs=%d err=%v",
			fixture.authority.distinctCommitSigningDigests(), err)
	}
	unavailable := errors.New("selection registry temporarily unavailable")
	fixture.selections.mu.Lock()
	fixture.selections.resolveErr = unavailable
	fixture.selections.mu.Unlock()
	if _, _, err := selectAndSignPublicationCommitV1(
		context.Background(), attempt, result.Candidate, result.Index, fixture.coordinator.settlement.RecordDigest,
		&head, fixture.commitSelectorConfig(),
	); !errors.Is(err, ErrPublicationRestartUnresolved) || !errors.Is(err, unavailable) ||
		fixture.authority.distinctCommitSigningDigests() != 0 {
		t.Fatalf("unavailable selector registry was not fail-closed: signs=%d err=%v",
			fixture.authority.distinctCommitSigningDigests(), err)
	}
}

func TestRestartCommitPlanRequiresCurrentDurableRegistryMembership(t *testing.T) {
	testCases := []struct {
		name   string
		remove func(*reportPublicationFixture)
	}{
		{name: "attempt", remove: func(fixture *reportPublicationFixture) {
			fixture.attempts.records = map[string]domainpublication.PublicationAttemptV1{}
		}},
		{name: "report stage", remove: func(fixture *reportPublicationFixture) {
			fixture.stages.receipt = domainpendingwork.PendingWorkReceiptV1{}
		}},
		{name: "candidate receipt", remove: func(fixture *reportPublicationFixture) {
			fixture.receipts.records = map[string]domainpublication.PublicationReceiptV1{}
		}},
		{name: "publication index", remove: func(fixture *reportPublicationFixture) {
			fixture.indexes.records = map[string]domainpublication.PublicationIndexV1{}
		}},
		{name: "advance intent", remove: func(fixture *reportPublicationFixture) {
			fixture.coordinator.intent = domainauthorityadvance.MonotonicAdvanceIntentV2{}
		}},
		{name: "advance settlement", remove: func(fixture *reportPublicationFixture) {
			fixture.coordinator.settlement = domainauthorityadvance.MonotonicAdvanceSettlementV2{}
		}},
		{name: "commit selection", remove: func(fixture *reportPublicationFixture) {
			fixture.selections.mu.Lock()
			fixture.selections.records = map[string]domainpublication.PublicationCommitSelectionV1{}
			fixture.selections.mu.Unlock()
		}},
		{name: "commit receipt", remove: func(fixture *reportPublicationFixture) {
			fixture.commits.mu.Lock()
			fixture.commits.records = map[string]domainpublication.PublicationCommitReceiptV1{}
			fixture.commits.mu.Unlock()
		}},
		{name: "delivery decision", remove: func(fixture *reportPublicationFixture) {
			fixture.decisions.mu.Lock()
			fixture.decisions.records = map[string]domainpublication.ReportDeliveryDecisionV1{}
			fixture.decisions.mu.Unlock()
		}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
			if _, err := fixture.service.Publish(context.Background(), fixture.input); err != nil {
				t.Fatal(err)
			}
			plan := fixture.committedRestartPlan(t)
			fixture.delivery = &deliveryProjectionStub{}
			fixture.authority.resetCommitSigningDigests()
			testCase.remove(fixture)
			if _, err := ApplyRestartV1(context.Background(), plan, fixture.restartApplyConfig()); err == nil ||
				fixture.delivery.calls != 0 || fixture.authority.distinctCommitSigningDigests() != 0 {
				t.Fatalf("missing %s registry member crossed restart: delivery=%d signs=%d err=%v",
					testCase.name, fixture.delivery.calls, fixture.authority.distinctCommitSigningDigests(), err)
			}
		})
	}
}

func TestRestartCommittedSettlementRequiresRegistryMembershipBeforeCommitSigning(t *testing.T) {
	testCases := []struct {
		name   string
		remove func(*reportPublicationFixture)
	}{
		{name: "attempt", remove: func(fixture *reportPublicationFixture) {
			fixture.attempts.records = map[string]domainpublication.PublicationAttemptV1{}
		}},
		{name: "candidate receipt", remove: func(fixture *reportPublicationFixture) {
			fixture.receipts.records = map[string]domainpublication.PublicationReceiptV1{}
		}},
		{name: "publication index", remove: func(fixture *reportPublicationFixture) {
			fixture.indexes.records = map[string]domainpublication.PublicationIndexV1{}
		}},
		{name: "advance intent", remove: func(fixture *reportPublicationFixture) {
			fixture.coordinator.intent = domainauthorityadvance.MonotonicAdvanceIntentV2{}
		}},
		{name: "advance settlement", remove: func(fixture *reportPublicationFixture) {
			fixture.coordinator.settlement = domainauthorityadvance.MonotonicAdvanceSettlementV2{}
		}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
			if _, err := fixture.service.Publish(context.Background(), fixture.input); err != nil {
				t.Fatal(err)
			}
			fixture.resetCommitSelectionStores()
			plan := fixture.committedRestartPlan(t)
			if len(plan.Attempts) != 1 || plan.Attempts[0].State != RestartAttemptCommittedSettlementV1 {
				t.Fatalf("restart plan state = %#v, want committed settlement", plan.Attempts)
			}
			fixture.delivery = &deliveryProjectionStub{}
			fixture.authority.resetCommitSigningDigests()
			testCase.remove(fixture)
			if _, err := ApplyRestartV1(context.Background(), plan, fixture.restartApplyConfig()); err == nil ||
				fixture.delivery.calls != 0 || fixture.authority.distinctCommitSigningDigests() != 0 {
				t.Fatalf("missing %s member crossed commit signing: delivery=%d signs=%d err=%v",
					testCase.name, fixture.delivery.calls, fixture.authority.distinctCommitSigningDigests(), err)
			}
		})
	}
}

func TestEvidenceRevocationBetweenInspectionAndPublishLeavesArtifactUndeliverable(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	fixture.evidence.second = fixture.revokedSnapshot(t)
	result, err := fixture.service.Publish(context.Background(), fixture.input)
	if !errors.Is(err, ErrPublicationChanged) || result.Commit.RecordDigest != "" {
		t.Fatalf("evidence change did not block publication: result=%#v err=%v", result, err)
	}
	if fixture.artifacts.installCalls != 0 || fixture.delivery.calls != 0 || fixture.receipts.putCalls != 0 ||
		fixture.indexes.putCalls != 0 || fixture.coordinator.advanceCalls != 0 || fixture.stages.closeCalls != 1 {
		t.Fatalf("revocation activated an orphan artifact: artifact=%d delivery=%d receipt=%d index=%d advance=%d close=%d",
			fixture.artifacts.installCalls, fixture.delivery.calls, fixture.receipts.putCalls, fixture.indexes.putCalls,
			fixture.coordinator.advanceCalls, fixture.stages.closeCalls)
	}
}

func TestEvidenceRevocationInsideFinalAdmissionCreatesNoDecision(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	fixture.evidence.third = fixture.revokedSnapshot(t)
	result, err := fixture.service.Publish(context.Background(), fixture.input)
	if !errors.Is(err, ErrPublicationChanged) || result.Decision.RecordDigest != "" ||
		fixture.commits.putCalls != 1 || fixture.decisions.createCalls != 0 || fixture.delivery.calls != 0 || fixture.stages.closeCalls != 0 {
		t.Fatalf("final evidence revocation created delivery authority: result=%#v commits=%d decisions=%d delivery=%d close=%d err=%v",
			result, fixture.commits.putCalls, fixture.decisions.createCalls, fixture.delivery.calls, fixture.stages.closeCalls, err)
	}
}

func TestReportRequiresPublicationReceipt(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	fixture.coordinator.advanceErr = errors.New("publication witness unavailable")
	result, err := fixture.service.Publish(context.Background(), fixture.input)
	if !errors.Is(err, fixture.coordinator.advanceErr) || result.Commit.RecordDigest != "" {
		t.Fatalf("witness failure was hidden: result=%#v err=%v", result, err)
	}
	if fixture.artifacts.installCalls != 1 || fixture.receipts.putCalls != 1 || fixture.indexes.putCalls != 1 ||
		fixture.delivery.calls != 0 || fixture.stages.closeCalls != 0 {
		t.Fatalf("unwitnessed publication became visible: artifact=%d receipt=%d index=%d delivery=%d close=%d",
			fixture.artifacts.installCalls, fixture.receipts.putCalls, fixture.indexes.putCalls, fixture.delivery.calls, fixture.stages.closeCalls)
	}
}

func TestPublicationCASAckLossNeverMarksReportStageFailed(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	fixture.coordinator.advanceAckErr = errors.New("publication witness acknowledgement lost")
	result, err := fixture.service.Publish(context.Background(), fixture.input)
	if !errors.Is(err, fixture.coordinator.advanceAckErr) || result.Commit.RecordDigest != "" {
		t.Fatalf("publication CAS acknowledgement loss was hidden: result=%#v err=%v", result, err)
	}
	if fixture.coordinator.advanceCalls != 1 || fixture.coordinator.bundle.PublicationCount != fixture.initialSnapshot.Head.Bundle.PublicationCount+1 ||
		fixture.stages.closeCalls != 0 || fixture.commits.putCalls != 0 || fixture.delivery.calls != 0 {
		t.Fatalf("CAS acknowledgement loss was given a definitive stage outcome: advance=%d publicationCount=%d close=%d commits=%d delivery=%d",
			fixture.coordinator.advanceCalls, fixture.coordinator.bundle.PublicationCount, fixture.stages.closeCalls,
			fixture.commits.putCalls, fixture.delivery.calls)
	}
}

func TestControlledPIIAuthorizationMustRemainCurrentAtPublication(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionControlledFull)
	fixture.pii.failAt = 2
	result, err := fixture.service.Publish(context.Background(), fixture.input)
	if err == nil || result.Commit.RecordDigest != "" {
		t.Fatalf("expired controlled PII authorization published a report: result=%#v err=%v", result, err)
	}
	if fixture.pii.calls != 2 || fixture.artifacts.installCalls != 0 || fixture.receipts.putCalls != 0 || fixture.delivery.calls != 0 {
		t.Fatalf("controlled PII revalidation ordering is invalid: pii=%d artifact=%d receipt=%d delivery=%d",
			fixture.pii.calls, fixture.artifacts.installCalls, fixture.receipts.putCalls, fixture.delivery.calls)
	}
}

func TestControlledFullRejectsNonCanonicalArtifactBeforeStage(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionControlledFull)
	input := fixture.input
	rebuildReportBytesForTest(t, &input, []byte("caller-declared controlled report"))
	result, err := fixture.service.Publish(context.Background(), input)
	if err == nil || result.Decision.RecordDigest != "" {
		t.Fatalf("non-canonical controlled bytes created delivery authority: result=%#v err=%v", result, err)
	}
	if fixture.stages.beginCalls != 0 || fixture.artifacts.installCalls != 0 || fixture.pii.calls != 0 ||
		fixture.attempts.createCalls != 0 || fixture.coordinator.advanceCalls != 0 || fixture.decisions.createCalls != 0 {
		t.Fatalf("non-canonical controlled bytes crossed a side-effect boundary: stage=%d artifact=%d pii=%d attempt=%d advance=%d decision=%d",
			fixture.stages.beginCalls, fixture.artifacts.installCalls, fixture.pii.calls,
			fixture.attempts.createCalls, fixture.coordinator.advanceCalls, fixture.decisions.createCalls)
	}
}

func TestControlledReportPreservesAuthorizedLeadingZeroAccountByteExact(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionControlledFull)
	const account = reportSourceExactAccountV2
	result, err := fixture.service.Publish(context.Background(), fixture.input)
	if err != nil || domainpublication.ValidateReportDeliveryDecisionV1(result.Decision) != nil {
		t.Fatalf("canonical controlled report did not reach private decision authority: result=%#v err=%v", result, err)
	}
	stored, err := fixture.artifacts.ResolveExact(context.Background(), fixture.input.TargetIdentityDigest)
	if err != nil || !bytes.Equal(stored, fixture.input.ReportBytes) {
		t.Fatalf("controlled artifact bytes changed during publication: err=%v", err)
	}
	artifact, err := domainpii.ParseControlledPIIArtifactV1(stored)
	if err != nil || len(artifact.Fields) != 1 || artifact.Fields[0].ExactValue != account ||
		artifact.Fields[0].ValueSHA256 != domainsecurity.SHA256Hex([]byte(account)) {
		t.Fatalf("authorized leading-zero account was truncated, masked, or rebound: err=%v", err)
	}
	decisionBody, err := domainpublication.ReportDeliveryDecisionV1Bytes(result.Decision)
	if err != nil || bytes.Contains(decisionBody, []byte(account)) {
		t.Fatalf("private delivery decision leaked the controlled value: err=%v", err)
	}
	if fixture.delivery.calls != 0 || fixture.stages.closeCalls != 0 {
		t.Fatalf("controlled artifact was publicly projected before settlement: delivery=%d close=%d",
			fixture.delivery.calls, fixture.stages.closeCalls)
	}
}

func TestControlledPIIRevokedAfterWitnessCommitRemainsIndeterminateAndNeverDelivers(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionControlledFull)
	fixture.pii.failAt = 4
	result, err := fixture.service.Publish(context.Background(), fixture.input)
	if err == nil || result.Commit.RecordDigest != "" || fixture.pii.calls != 4 || fixture.coordinator.advanceCalls != 1 ||
		fixture.commits.putCalls != 0 || fixture.delivery.calls != 0 || fixture.stages.closeCalls != 0 ||
		fixture.coordinator.bundle.PublicationCount != fixture.initialSnapshot.Head.Bundle.PublicationCount+1 {
		t.Fatalf("post-witness PII revocation reached publication: result=%#v pii=%d advance=%d commits=%d delivery=%d err=%v",
			result, fixture.pii.calls, fixture.coordinator.advanceCalls, fixture.commits.putCalls, fixture.delivery.calls, err)
	}
}

func TestControlledPIIRevokedInsideFinalSnapshotCreatesNoDecision(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionControlledFull)
	fixture.pii.failAt = 5
	result, err := fixture.service.Publish(context.Background(), fixture.input)
	if err == nil || result.Decision.RecordDigest != "" || fixture.pii.calls != 5 || fixture.pii.witnessedCalls != 1 ||
		fixture.commits.putCalls != 1 || fixture.decisions.createCalls != 0 || fixture.delivery.calls != 0 || fixture.stages.closeCalls != 0 {
		t.Fatalf("final-snapshot PII revocation created delivery authority: result=%#v pii=%d witnessed=%d commits=%d decisions=%d delivery=%d err=%v",
			result, fixture.pii.calls, fixture.pii.witnessedCalls, fixture.commits.putCalls,
			fixture.decisions.createCalls, fixture.delivery.calls, err)
	}
}

func TestDeliveryDecisionFailureAfterWitnessCommitNeverMarksReportStageFailed(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	fixture.decisions.createErr = errors.New("decision acknowledgement lost")
	result, err := fixture.service.Publish(context.Background(), fixture.input)
	if !errors.Is(err, fixture.decisions.createErr) || result.Commit.RecordDigest != "" || result.Decision.RecordDigest != "" {
		t.Fatalf("decision failure was hidden: result=%#v err=%v", result, err)
	}
	if fixture.coordinator.advanceCalls != 1 || fixture.commits.putCalls != 1 || fixture.decisions.createCalls != 1 ||
		fixture.delivery.calls != 0 || fixture.stages.closeCalls != 0 {
		t.Fatalf("indeterminate decision was given a definitive stage outcome: advance=%d commits=%d decisions=%d delivery=%d close=%d",
			fixture.coordinator.advanceCalls, fixture.commits.putCalls, fixture.decisions.createCalls, fixture.delivery.calls, fixture.stages.closeCalls)
	}
}

func TestDeliveryDecisionAcknowledgementLossReconcilesExactDecisionWithoutProjection(t *testing.T) {
	fixture := newReportPublicationFixture(t, domainpublication.PIIProjectionOrdinaryMasked)
	fixture.decisions.createErr = errors.New("decision acknowledgement lost after commit")
	fixture.decisions.commitOnErr = true
	result, err := fixture.service.Publish(context.Background(), fixture.input)
	if err != nil || domainpublication.ValidateReportDeliveryDecisionV1(result.Decision) != nil {
		t.Fatalf("exact decision readback did not reconcile acknowledgement loss: result=%#v err=%v", result, err)
	}
	if fixture.decisions.createCalls != 1 || fixture.decisions.resolveCalls != 1 ||
		fixture.delivery.calls != 0 || fixture.stages.closeCalls != 0 {
		t.Fatalf("decision acknowledgement recovery crossed an invalid boundary: create=%d resolve=%d project=%d close=%d",
			fixture.decisions.createCalls, fixture.decisions.resolveCalls, fixture.delivery.calls, fixture.stages.closeCalls)
	}
}

// reportPublicationTestService keeps legacy controlled-core fault-injection
// tests focused on the private state machine. Public entry tests separately
// prove that raw ControlledFull input cannot cross Service.Publish.
type reportPublicationTestService struct {
	*Service
}

func (service *reportPublicationTestService) Publish(ctx context.Context, input PublishInput) (PublishResult, error) {
	if !input.WriteReport {
		return service.Service.Publish(ctx, input)
	}
	return service.Service.publish(ctx, input, input.PIIProjection.ProjectionClass == domainpublication.PIIProjectionControlledFull)
}

type reportPIIGrantStore struct {
	records map[string]domainpii.PIIProjectionGrantV1
}

func (store *reportPIIGrantStore) PutGrantIfAbsent(_ context.Context, grant domainpii.PIIProjectionGrantV1) error {
	if current, exists := store.records[grant.RecordDigest]; exists && !reflect.DeepEqual(current, grant) {
		return piiauthorizationport.ErrConflict
	}
	store.records[grant.RecordDigest] = grant
	return nil
}

func (store *reportPIIGrantStore) ResolveGrant(_ context.Context, digest string) (domainpii.PIIProjectionGrantV1, error) {
	grant, exists := store.records[digest]
	if !exists {
		return domainpii.PIIProjectionGrantV1{}, piiauthorizationport.ErrNotFound
	}
	return grant, nil
}

type reportPIIApprovalAuthorityStub struct{}

func (reportPIIApprovalAuthorityStub) ValidateCurrent(context.Context, piiauthorizationport.ApprovalValidationV1) error {
	return nil
}

type reportPublicationFixture struct {
	service          *reportPublicationTestService
	input            PublishInput
	initialSnapshot  registryport.WitnessedSnapshot
	authority        *reportPublicationAuthority
	stages           *reportStageStub
	evidence         *reportEvidenceStub
	coordinator      *reportPublicationCoordinator
	bundles          *memoryEvidenceBundleStore
	observations     *memoryObservationStore
	contexts         *restartContextAuthorityStub
	effectGate       *effectgateapp.Gate
	attempts         *memoryPublicationAttemptStore
	receipts         *memoryPublicationReceiptStore
	commits          *memoryPublicationCommitStore
	selections       *memoryPublicationSelectionStore
	decisions        *memoryReportDeliveryDecisionStore
	grantSettlements *memoryReportGrantSettlementStore
	stageCompletions *memoryReportStageCompletionStore
	threads          *restartThreadReaderStub
	indexes          *memoryPublicationIndexStore
	ledgers          *memoryClaimLedgerStore
	projections      *memoryPIIProjectionStore
	inspections      *memoryRenderInspectionStore
	artifacts        *memoryArtifactStore
	extractor        *completeReportSurfaceExtractorStub
	pii              *piiAuthorizationStub
	delivery         *deliveryProjectionStub
	nonStageGrant    domainsecurity.ExecutionGrant
	nonStageArgs     json.RawMessage
	now              time.Time
}

func newReportPublicationFixture(t *testing.T, projectionClass string) *reportPublicationFixture {
	t.Helper()
	now := time.Date(2026, 7, 13, 7, 0, 0, 0, time.UTC)
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x41}, ed25519.SeedSize))
	authority := &reportPublicationAuthority{privateKey: privateKey, publicKey: privateKey.Public().(ed25519.PublicKey)}
	authority.keyID = domainsecurity.SHA256Hex(authority.publicKey)
	witnessPrivate := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x51}, ed25519.SeedSize))
	witnessPublic := witnessPrivate.Public().(ed25519.PublicKey)
	installationID := domainsecurity.SHA256Hex([]byte("report-publication-installation"))
	enrollmentID := domainsecurity.SHA256Hex([]byte("report-publication-enrollment"))
	securityContext, err := testsecurity.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-report-publication", TurnID: "turn-report-publication", WorkspaceRealPath: "/workspace/report-publication",
		CaseID: "case-report-publication", CaseBindingHash: domainsecurity.SHA256Hex([]byte("report-publication-binding")), ContextEpoch: 7, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, claim, evidenceID := reportPublicationRegistry(
		t, authority, securityContext, now, projectionClass == domainpublication.PIIProjectionControlledFull,
	)
	capsule, err := domainevidence.NewEvidenceRegistryAuthorityCapsule(
		securityContext, registry, authority.keyID, authority.publicKey,
		func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) },
	)
	if err != nil {
		t.Fatal(err)
	}
	rootIndex, err := domainevidence.NewEvidenceRegistryAuthorityIndexV2(domainevidence.EvidenceRegistryAuthorityIndexInputV2{
		InstallationID: installationID, EnrollmentID: enrollmentID, Generation: 1,
		PreviousIndexDigest: domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(),
		MutationID:          domainsecurity.SHA256Hex([]byte("report-registry-index-mutation")),
	}, capsule, authority.keyID, authority.publicKey, func(message []byte) ([]byte, error) {
		return authority.Sign(context.Background(), message)
	})
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := newReportEvidenceBundle(t, authority, installationID, enrollmentID, rootIndex.IndexDigest, 1,
		domainpublication.PublicationIndexGenesisDigestV1(), 0, "initial")
	if err != nil {
		t.Fatal(err)
	}
	initialHead := reportFreshHead(t, bundle, authority, witnessPrivate, witnessPublic, "initial")
	snapshot := registryport.WitnessedSnapshot{
		Head: initialHead, RootIndex: rootIndex,
		Context: securityContext, Registry: registry,
	}
	ledger, err := domainpublication.NewClaimLedgerV1(domainpublication.ClaimLedgerInputV1{
		Context: securityContext, EvidenceReceiptIDs: []string{evidenceID}, Claims: []domainevidence.ClaimRecord{claim}, CreatedAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	targetIdentityDigest := domainsecurity.SHA256Hex([]byte("report-target"))
	rulesetHash := domainsecurity.SHA256Hex([]byte("report-pii-rules"))
	reportBytes := []byte("deterministic report artifact")
	mediaType := "application/pdf"
	if projectionClass == domainpublication.PIIProjectionControlledFull {
		account := reportSourceExactAccountV2
		artifact, artifactErr := domainpii.NewControlledPIIArtifactV1(domainpii.ControlledPIIArtifactInputV1{
			SecurityContext: securityContext, ClaimLedgerDigest: ledger.LedgerDigest,
			ProjectionRulesetHash: rulesetHash, TargetIdentityDigest: targetIdentityDigest,
			Fields: []domainpii.ControlledPIIFieldV1{{
				PIIClass: domainpii.PIIClassFinancialAccountV1, ClaimID: claim.ClaimID,
				ClaimRecordDigest: claim.RecordDigest, ClaimType: claim.ClaimType, FieldName: "accountId",
				ExactValue: account, ValueSHA256: domainsecurity.SHA256Hex([]byte(account)),
				EvidenceReceiptIDs: append([]string(nil), claim.EvidenceIDs...),
			}},
			RenderedAt: now.Add(time.Minute),
		})
		if artifactErr != nil {
			t.Fatal(artifactErr)
		}
		reportBytes, artifactErr = domainpii.ControlledPIIArtifactV1Bytes(artifact)
		if artifactErr != nil {
			t.Fatal(artifactErr)
		}
		mediaType = domainpii.ControlledPIIArtifactMediaTypeV1
	}
	reportSHA := domainsecurity.SHA256Hex(reportBytes)
	projectionInput := domainpublication.PIIProjectionInputV1{
		ProjectionClass: projectionClass, RulesetHash: rulesetHash,
		ProjectedContentSHA256: reportSHA, RestrictedFieldCount: 1,
	}
	if projectionClass == domainpublication.PIIProjectionControlledFull {
		projectionInput.PreservedControlledFieldCount = 1
		projectionInput.AuthorizationAuditDigest = domainsecurity.SHA256Hex([]byte("report-controlled-authorization"))
	}
	projection, err := domainpublication.NewPIIProjectionV1(projectionInput)
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := domainpublication.NewRenderInspectionV1(domainpublication.RenderInspectionInputV1{
		Renderer: "analytix-host", RendererVersion: "1.0.0", ReportSHA256: reportSHA,
		ReportByteLength: uint64(len(reportBytes)), MediaType: mediaType, Passed: true, IssueCodes: []string{},
		InspectedAt: now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	arguments := json.RawMessage(`{"report":"case"}`)
	toolCallID, err := domainmodel.NewHostToolCallIDV1(bytes.Repeat([]byte{0x71}, domainmodel.HostToolCallIDEntropyBytesV1))
	if err != nil {
		t.Fatal(err)
	}
	call := domainmodel.ToolCall{ID: toolCallID, Name: pendingworkapp.ReportStageToolName, Arguments: arguments}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider", ServerIdentity: "host:builtin", ToolName: call.Name, ToolCallID: call.ID,
		ArgsHash: domainsecurity.CanonicalJSONHash(arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("report-schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("report-scope")), ReadOnly: false, ApprovalState: "approved",
		IssuedAt: now, ExpiresAt: now.Add(time.Hour),
	})
	activeGrantRegistry, err := domainsecurity.RegisterExecutionGrant(
		domainsecurity.NewExecutionGrantRegistry(securityContext.ThreadID), securityContext.ThreadID, grant, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	activeGrantEntry, found := domainsecurity.ExecutionGrantRegistryEntryByID(activeGrantRegistry, grant.GrantID)
	if !found {
		t.Fatal("report-stage execution grant was not registered")
	}
	nonStageArgs := json.RawMessage(`{"path":"prerequisite.txt"}`)
	nonStageCallID, err := domainmodel.NewHostToolCallIDV1(bytes.Repeat([]byte{0x72}, domainmodel.HostToolCallIDEntropyBytesV1))
	if err != nil {
		t.Fatal(err)
	}
	nonStageGrant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "host", ServerIdentity: "host:builtin", ToolName: "read_file", ToolCallID: nonStageCallID,
		ArgsHash: domainsecurity.CanonicalJSONHash(nonStageArgs), SchemaHash: domainsecurity.SHA256Hex([]byte("report-prerequisite-schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("report-prerequisite-scope")), ReadOnly: true, ApprovalState: "approved",
		IssuedAt: now, ExpiresAt: now.Add(time.Hour),
	})
	activeGrantRegistry, err = domainsecurity.RegisterExecutionGrant(
		activeGrantRegistry, securityContext.ThreadID, nonStageGrant, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	pending := appmodel.PendingToolCall{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: securityContext.WorkspaceRealPath,
		Call: call, SecurityContext: securityContext, ExecutionGrant: grant,
	}
	stageReceipt, err := domainpendingwork.NewPendingWorkReceiptV1(domainpendingwork.ReceiptInputV1{
		Kind: domainpendingwork.KindReportStage, SecurityContext: securityContext,
		GrantRegistrySequence: activeGrantRegistry.Sequence, GrantRegistryDigest: activeGrantRegistry.StateDigest,
		GrantMembers: []domainpendingwork.GrantMemberV1{{
			Ordinal: 1, GrantID: grant.GrantID, RegistrySequence: activeGrantEntry.Sequence,
			RegistryEntryDigest: activeGrantEntry.EntryDigest,
		}},
		PayloadHash: domainsecurity.SHA256Hex([]byte("report-publication-stage-payload")),
		RouteHash:   domainsecurity.SHA256Hex([]byte("report-publication-stage-route")),
		IssuedAt:    now, ExpiresAt: now.Add(time.Hour), AuthorityKeyID: authority.keyID, AuthorityPublicKey: authority.publicKey,
	}, func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	stages := &reportStageStub{receipt: stageReceipt}
	evidence := &reportEvidenceStub{first: snapshot}
	bundles := &memoryEvidenceBundleStore{records: map[string]domainevidence.EvidenceAuthorityBundleV1{bundle.RecordDigest: bundle}}
	observations := &memoryObservationStore{records: map[string]evidenceauthorityport.ObservationBundle{}}
	contexts := &restartContextAuthorityStub{current: securityContext}
	effectGate := effectgateapp.New()
	coordinator := &reportPublicationCoordinator{
		bundle: bundle, authority: authority, witnessPrivate: witnessPrivate, witnessPublic: witnessPublic,
	}
	evidence.headReader = coordinator
	attempts := &memoryPublicationAttemptStore{records: map[string]domainpublication.PublicationAttemptV1{}}
	receipts := &memoryPublicationReceiptStore{records: map[string]domainpublication.PublicationReceiptV1{}}
	commits := &memoryPublicationCommitStore{records: map[string]domainpublication.PublicationCommitReceiptV1{}}
	selections := &memoryPublicationSelectionStore{records: map[string]domainpublication.PublicationCommitSelectionV1{}}
	decisions := &memoryReportDeliveryDecisionStore{records: map[string]domainpublication.ReportDeliveryDecisionV1{}}
	grantSettlements := &memoryReportGrantSettlementStore{records: map[string]domainpublication.ReportGrantSettlementV1{}}
	stageCompletions := &memoryReportStageCompletionStore{records: map[string]domainpublication.ReportStageCompletionV1{}}
	threads := &restartThreadReaderStub{records: map[string]map[string]any{}}
	indexes := &memoryPublicationIndexStore{records: map[string]domainpublication.PublicationIndexV1{}}
	ledgers := &memoryClaimLedgerStore{records: map[string]domainpublication.ClaimLedgerV1{}}
	projections := &memoryPIIProjectionStore{records: map[string]domainpublication.PIIProjectionV1{}}
	inspections := &memoryRenderInspectionStore{records: map[string]domainpublication.RenderInspectionV1{}}
	artifacts := &memoryArtifactStore{records: map[string][]byte{}}
	extractor := &completeReportSurfaceExtractorStub{}
	pii := &piiAuthorizationStub{}
	delivery := &deliveryProjectionStub{}
	service, err := New(Config{
		InstallationID: installationID, EnrollmentID: enrollmentID, Authority: authority, Stages: stages,
		WitnessKeyID: domainsecurity.SHA256Hex(witnessPublic), WitnessKey: witnessPublic,
		Evidence: evidence, HeadReader: coordinator, Advances: coordinator, Bundles: bundles, Observations: observations,
		Attempts: attempts,
		Receipts: receipts, Commits: commits, Selections: selections, Decisions: decisions, Indexes: indexes, Ledgers: ledgers,
		PIIProjections: projections, Inspections: inspections, Artifacts: artifacts,
		SurfaceExtractor: extractor, PIIAuthority: pii, DeliveryOutcomes: delivery,
		Now: func() time.Time { return now.Add(5 * time.Minute) },
	})
	if err != nil {
		t.Fatal(err)
	}
	input := PublishInput{
		WriteReport: true, PendingToolCall: pending, ReportVariant: domainpublication.EvidenceBackedReport,
		ClaimLedger: ledger, PIIProjection: projection, RenderInspection: inspection, ReportBytes: reportBytes,
		Publisher: "analytix-host", PublisherVersion: "1.0.0", TargetIdentityDigest: targetIdentityDigest,
		IssuedAt: now.Add(3 * time.Minute),
	}
	return &reportPublicationFixture{
		service: &reportPublicationTestService{Service: service}, input: input, initialSnapshot: snapshot, authority: authority, stages: stages, evidence: evidence,
		coordinator: coordinator, bundles: bundles, observations: observations, contexts: contexts, effectGate: effectGate,
		attempts: attempts, receipts: receipts, commits: commits, selections: selections, decisions: decisions,
		grantSettlements: grantSettlements, stageCompletions: stageCompletions, threads: threads,
		indexes: indexes, ledgers: ledgers, projections: projections,
		inspections: inspections, artifacts: artifacts, extractor: extractor, pii: pii, delivery: delivery, now: now,
		nonStageGrant: nonStageGrant, nonStageArgs: nonStageArgs,
	}
}

func (fixture *reportPublicationFixture) revokedSnapshot(t *testing.T) registryport.WitnessedSnapshot {
	return fixture.revokedSnapshotFrom(t, fixture.initialSnapshot.Head.Bundle)
}

func (fixture *reportPublicationFixture) revokedSnapshotAfterPublication(t *testing.T) registryport.WitnessedSnapshot {
	fixture.coordinator.mu.Lock()
	previous := fixture.coordinator.bundle
	fixture.coordinator.mu.Unlock()
	return fixture.revokedSnapshotFrom(t, previous)
}

func (fixture *reportPublicationFixture) revokedSnapshotFrom(
	t *testing.T,
	previous domainevidence.EvidenceAuthorityBundleV1,
) registryport.WitnessedSnapshot {
	t.Helper()
	revoked, err := domainevidence.RevokeEvidenceReceipt(
		fixture.initialSnapshot.Registry, fixture.input.ClaimLedger.EvidenceReceiptIDs[0], "source_retracted", fixture.now.Add(4*time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	capsule, err := domainevidence.NewEvidenceRegistryAuthorityCapsule(
		fixture.initialSnapshot.Context, revoked, fixture.authority.keyID, fixture.authority.publicKey,
		func(message []byte) ([]byte, error) { return fixture.authority.Sign(context.Background(), message) },
	)
	if err != nil {
		t.Fatal(err)
	}
	rootIndex, err := domainevidence.NewEvidenceRegistryAuthorityIndexV2(
		domainevidence.EvidenceRegistryAuthorityIndexInputV2{
			InstallationID:      fixture.initialSnapshot.Head.Bundle.InstallationID,
			EnrollmentID:        fixture.initialSnapshot.Head.Bundle.EnrollmentID,
			Generation:          fixture.initialSnapshot.RootIndex.Generation + 1,
			PreviousIndexDigest: fixture.initialSnapshot.RootIndex.IndexDigest,
			MutationID:          domainsecurity.SHA256Hex([]byte("revoked-registry-index-mutation")),
		},
		capsule, fixture.authority.keyID, fixture.authority.publicKey,
		func(message []byte) ([]byte, error) { return fixture.authority.Sign(context.Background(), message) },
	)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := domainevidence.NewEvidenceAuthorityBundleV1(domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID: previous.InstallationID, EnrollmentID: previous.EnrollmentID, Generation: previous.Generation + 1,
		PreviousBundleDigest: previous.RecordDigest, MutationID: domainsecurity.SHA256Hex([]byte("report-bundle-revoked")),
		DatasetSnapshotIndexDigest: previous.DatasetSnapshotIndexDigest, DatasetSnapshotCount: previous.DatasetSnapshotCount,
		EvidenceRegistryIndexDigest: rootIndex.IndexDigest, EvidenceRegistryCount: previous.EvidenceRegistryCount + 1,
		PublicationIndexDigest: previous.PublicationIndexDigest, PublicationCount: previous.PublicationCount,
		AuthorityKeyID: fixture.authority.keyID, AuthorityPublicKey: fixture.authority.publicKey,
	}, func(message []byte) ([]byte, error) { return fixture.authority.Sign(context.Background(), message) })
	if err != nil {
		t.Fatal(err)
	}
	head := reportFreshHead(t, bundle, fixture.authority, fixture.coordinator.witnessPrivate, fixture.coordinator.witnessPublic, "revoked")
	return registryport.WitnessedSnapshot{Head: head, RootIndex: rootIndex, Context: fixture.initialSnapshot.Context, Registry: revoked}
}

func (fixture *reportPublicationFixture) unrelatedRegistryAdvanceSnapshot(t *testing.T) registryport.WitnessedSnapshot {
	t.Helper()
	caseB, err := testsecurity.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-report-publication-case-b", TurnID: "turn-report-publication-case-b",
		WorkspaceRealPath: fixture.input.PendingToolCall.SecurityContext.WorkspaceRealPath,
		CaseID:            "case-report-publication-b", CaseBindingHash: domainsecurity.SHA256Hex([]byte("report-publication-binding-b")),
		ContextEpoch: 1, IssuedAt: fixture.now.Add(7 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	registryB, _, _ := reportPublicationRegistry(
		t, fixture.authority, caseB, fixture.now.Add(7*time.Minute),
		fixture.input.PIIProjection.ProjectionClass == domainpublication.PIIProjectionControlledFull,
	)
	capsuleB, err := domainevidence.NewEvidenceRegistryAuthorityCapsule(
		caseB, registryB, fixture.authority.keyID, fixture.authority.publicKey,
		func(message []byte) ([]byte, error) { return fixture.authority.Sign(context.Background(), message) },
	)
	if err != nil {
		t.Fatal(err)
	}
	previousIndex := fixture.initialSnapshot.RootIndex
	nextIndex, err := domainevidence.NewEvidenceRegistryAuthorityIndexV2(
		domainevidence.EvidenceRegistryAuthorityIndexInputV2{
			InstallationID: previousIndex.InstallationID, EnrollmentID: previousIndex.EnrollmentID,
			Generation: previousIndex.Generation + 1, PreviousIndexDigest: previousIndex.IndexDigest,
			MutationID: domainsecurity.SHA256Hex([]byte("report-registry-index-unrelated-case")),
		},
		capsuleB, fixture.authority.keyID, fixture.authority.publicKey,
		func(message []byte) ([]byte, error) { return fixture.authority.Sign(context.Background(), message) },
	)
	if err != nil || domainevidence.ValidateEvidenceRegistryAuthorityIndexTransitionV2(previousIndex, nextIndex) != nil {
		t.Fatalf("construct unrelated registry root advance: %v", err)
	}
	fixture.coordinator.mu.Lock()
	previousBundle := fixture.coordinator.bundle
	fixture.coordinator.mu.Unlock()
	nextBundle, err := domainevidence.NewEvidenceAuthorityBundleV1(domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID: previousBundle.InstallationID, EnrollmentID: previousBundle.EnrollmentID,
		Generation: previousBundle.Generation + 1, PreviousBundleDigest: previousBundle.RecordDigest,
		MutationID:                 domainsecurity.SHA256Hex([]byte("report-bundle-unrelated-registry-advance")),
		DatasetSnapshotIndexDigest: previousBundle.DatasetSnapshotIndexDigest, DatasetSnapshotCount: previousBundle.DatasetSnapshotCount,
		EvidenceRegistryIndexDigest: nextIndex.IndexDigest, EvidenceRegistryCount: previousBundle.EvidenceRegistryCount + 1,
		PublicationIndexDigest: previousBundle.PublicationIndexDigest, PublicationCount: previousBundle.PublicationCount,
		AuthorityKeyID: fixture.authority.keyID, AuthorityPublicKey: fixture.authority.publicKey,
	}, func(message []byte) ([]byte, error) { return fixture.authority.Sign(context.Background(), message) })
	if err != nil || domainevidence.ValidateEvidenceAuthorityBundleTransitionV1(previousBundle, nextBundle) != nil {
		t.Fatalf("construct unrelated registry bundle advance: %v", err)
	}
	head := reportFreshHead(
		t, nextBundle, fixture.authority, fixture.coordinator.witnessPrivate, fixture.coordinator.witnessPublic,
		"unrelated-registry-advance",
	)
	return registryport.WitnessedSnapshot{
		Head: head, RootIndex: nextIndex, Context: fixture.initialSnapshot.Context, Registry: fixture.initialSnapshot.Registry,
	}
}

func (fixture *reportPublicationFixture) totalCalls() int {
	return fixture.stages.beginCalls + fixture.stages.resolveCalls + fixture.stages.verifyCalls + fixture.stages.closeCalls + fixture.stages.putDispositionCalls + fixture.evidence.calls +
		fixture.coordinator.advanceCalls + fixture.bundles.putCalls + fixture.bundles.resolveCalls +
		fixture.attempts.createCalls + fixture.attempts.resolveCalls + fixture.receipts.putCalls + fixture.receipts.resolveCalls +
		fixture.commits.putCalls + fixture.commits.resolveCalls + fixture.decisions.createCalls + fixture.decisions.resolveCalls + fixture.indexes.putCalls +
		fixture.indexes.resolveCalls + fixture.ledgers.putCalls + fixture.ledgers.resolveCalls + fixture.projections.putCalls +
		fixture.projections.resolveCalls + fixture.inspections.putCalls + fixture.inspections.resolveCalls +
		fixture.artifacts.installCalls + fixture.artifacts.resolveCalls + fixture.extractor.calls + fixture.pii.calls +
		fixture.delivery.calls + fixture.delivery.resolveCalls
}

func (fixture *reportPublicationFixture) restartApplyConfig() RestartApplyConfigV1 {
	bundle := fixture.initialSnapshot.Head.Bundle
	return RestartApplyConfigV1{
		InstallationID: bundle.InstallationID, EnrollmentID: bundle.EnrollmentID, Authority: fixture.authority,
		WitnessKeyID: domainsecurity.SHA256Hex(fixture.coordinator.witnessPublic), WitnessKey: fixture.coordinator.witnessPublic,
		Evidence: fixture.evidence, HeadReader: fixture.coordinator, Advances: fixture.coordinator, Pending: fixture.stages,
		Attempts: fixture.attempts, Receipts: fixture.receipts, Indexes: fixture.indexes,
		Intents: fixture.coordinator, Settlements: fixture.coordinator, Bundles: fixture.bundles,
		Observations: fixture.observations, Contexts: fixture.contexts, Ledgers: fixture.ledgers,
		PIIProjections: fixture.projections, Inspections: fixture.inspections, Artifacts: fixture.artifacts,
		PIIAuthority: fixture.pii, Selections: fixture.selections, Commits: fixture.commits, DeliveryOutcomes: fixture.delivery,
		Decisions: fixture.decisions, GrantSettlements: fixture.grantSettlements, StageCompletions: fixture.stageCompletions, Threads: fixture.threads,
		AcquireContextEffect: fixture.effectGate.AcquireEffect,
	}
}

func (fixture *reportPublicationFixture) committedRestartPlan(t *testing.T) RestartPlanV1 {
	t.Helper()
	plan, err := fixture.restartPlan()
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func (fixture *reportPublicationFixture) restartPlan() (RestartPlanV1, error) {
	journal := &reportRestartJournalStub{intent: &fixture.coordinator.intent, settlement: &fixture.coordinator.settlement}
	pending := pendingworkapp.TrustedInventoryV1{
		Receipts:     []domainpendingwork.PendingWorkReceiptV1{fixture.stages.receipt},
		Dispositions: map[string]domainpendingwork.PendingWorkDispositionV1{},
	}
	if fixture.stages.disposition != nil {
		pending.Dispositions[fixture.stages.receipt.WorkID] = *fixture.stages.disposition
	}
	return PreflightRestartV1(context.Background(), RestartPreflightConfigV1{
		Pending:  pending,
		Attempts: fixture.attempts, Receipts: fixture.receipts, Indexes: fixture.indexes,
		Commits: fixture.commits, Selections: fixture.selections, Decisions: fixture.decisions, GrantSettlements: fixture.grantSettlements, StageCompletions: fixture.stageCompletions, DeliveryOutcomes: fixture.delivery, Threads: fixture.threads,
		Ledgers: fixture.ledgers, Projections: fixture.projections, Inspections: fixture.inspections,
		Intents: journal, Settlements: journal, Authority: fixture.authority,
	})
}

func (fixture *reportPublicationFixture) installDurableReportGrantSettlement(
	t *testing.T,
	decision domainpublication.ReportDeliveryDecisionV1,
) domainpublication.ReportGrantSettlementV1 {
	t.Helper()
	contextRecord := map[string]any{}
	contextBody, err := json.Marshal(fixture.input.PendingToolCall.SecurityContext)
	if err != nil || json.Unmarshal(contextBody, &contextRecord) != nil {
		t.Fatalf("encode report security context: %v", err)
	}
	grant := fixture.input.PendingToolCall.ExecutionGrant
	threadID := fixture.input.PendingToolCall.SecurityContext.ThreadID
	grantRecord := map[string]any{}
	grantBody, err := json.Marshal(grant)
	if err != nil || json.Unmarshal(grantBody, &grantRecord) != nil {
		t.Fatalf("encode report execution grant: %v", err)
	}
	arguments := map[string]any{}
	if err := json.Unmarshal(fixture.input.PendingToolCall.Call.Arguments, &arguments); err != nil {
		t.Fatal(err)
	}
	nonStageGrantRecord := map[string]any{}
	nonStageGrantBody, err := json.Marshal(fixture.nonStageGrant)
	if err != nil || json.Unmarshal(nonStageGrantBody, &nonStageGrantRecord) != nil {
		t.Fatalf("encode non-stage execution grant: %v", err)
	}
	nonStageArguments := map[string]any{}
	if err := json.Unmarshal(fixture.nonStageArgs, &nonStageArguments); err != nil {
		t.Fatal(err)
	}
	resultItemID := domaintoolresult.ToolResultItemIDV1(
		fixture.input.PendingToolCall.TurnID, fixture.input.PendingToolCall.Call.ID,
	)
	settledAt := fixture.now.Add(6 * time.Minute)
	toolCallItem := map[string]any{
		"id": "item_tool_" + fixture.input.PendingToolCall.Call.ID, "kind": "tool_call", "role": "assistant", "status": "completed",
		"threadId": threadID, "turnId": grant.TurnID, "toolName": grant.ToolName, "callId": grant.ToolCallID,
		"arguments": arguments, "createdAt": grant.IssuedAt, "contextDigest": grant.ContextDigest,
		"contextEpoch":     float64(fixture.input.PendingToolCall.SecurityContext.ContextEpoch),
		"executionGrantId": grant.GrantID, "executionGrant": grantRecord,
	}
	nonStageToolCallItem := map[string]any{
		"id": "item_tool_" + fixture.nonStageGrant.ToolCallID, "kind": "tool_call", "role": "assistant", "status": "completed",
		"threadId": threadID, "turnId": fixture.nonStageGrant.TurnID, "toolName": fixture.nonStageGrant.ToolName,
		"callId": fixture.nonStageGrant.ToolCallID, "arguments": nonStageArguments, "createdAt": fixture.nonStageGrant.IssuedAt,
		"contextDigest":    fixture.nonStageGrant.ContextDigest,
		"contextEpoch":     float64(fixture.input.PendingToolCall.SecurityContext.ContextEpoch),
		"executionGrantId": fixture.nonStageGrant.GrantID, "executionGrant": nonStageGrantRecord,
	}
	resultItem, resultItemOK := domaintoolresult.PrivateDurableToolResultItemRecordV1(map[string]any{
		"id": resultItemID, "kind": "tool_result", "role": "tool", "status": "completed",
		"threadId": threadID, "turnId": grant.TurnID, "toolName": grant.ToolName, "callId": grant.ToolCallID,
		"toolKind": "tool_call", "isError": false,
		"createdAt": settledAt.Format(time.RFC3339Nano), "finishedAt": settledAt.Format(time.RFC3339Nano),
		"contextDigest": grant.ContextDigest, "contextEpoch": float64(fixture.input.PendingToolCall.SecurityContext.ContextEpoch),
		"executionGrantId": grant.GrantID,
		"hostReportAdmission": domaintoolresult.HostReportAdmissionRecordV1(
			domaintoolresult.NewHostReportAdmissionV1(decision.DecisionID, decision.RecordDigest),
		),
		"output": domaintoolresult.PublicToolResultProjectionRecordV1(
			domaintoolresult.WithheldProjectionV1("completed", "tool_output_private"),
		),
	})
	if !resultItemOK {
		t.Fatal("report result did not fit the private durable result contract")
	}
	thread := map[string]any{
		"id": threadID,
		"turns": []any{map[string]any{
			"id": grant.TurnID, "status": "running", "securityContext": contextRecord,
			"items": []any{toolCallItem, nonStageToolCallItem, resultItem},
		}},
	}
	durable, err := executiongrantapp.DurableSettlementFromThread(
		threadID, thread, grant.TurnID, resultItemID, grant,
	)
	if err != nil {
		t.Fatalf("construct durable report grant settlement authority: %v", err)
	}
	resultItemBody, err := json.Marshal(durable.ResultItem)
	if err != nil {
		t.Fatal(err)
	}
	fixture.stages.mu.Lock()
	fixture.stages.trustedGrant = grant
	fixture.stages.trustedActiveRegistry = durable.ActiveRegistry
	fixture.stages.trustedSettledRegistry = durable.SettledRegistry
	fixture.stages.trustedResultItemID = resultItemID
	fixture.stages.trustedResultItemDigest = domainsecurity.CanonicalJSONHash(resultItemBody)
	fixture.stages.trustedResultItem = durable.ResultItem
	fixture.stages.trustedSettledAt = durable.SettledAt
	fixture.stages.mu.Unlock()
	settlement, err := domainpublication.NewReportGrantSettlementV1(
		domainpublication.ReportGrantSettlementInputV1{
			Decision: decision, Grant: grant, ActiveRegistry: durable.ActiveRegistry, SettledRegistry: durable.SettledRegistry,
			ResultItemID: resultItemID, ResultItemDigest: domainsecurity.CanonicalJSONHash(resultItemBody),
			SettledAt: durable.SettledAt, AuthorityKeyID: fixture.authority.keyID, AuthorityPublicKey: fixture.authority.publicKey,
		},
		func(message []byte) ([]byte, error) { return fixture.authority.Sign(context.Background(), message) },
	)
	if err != nil {
		t.Fatalf("sign report grant settlement: %v", err)
	}
	fixture.threads.records[threadID] = thread
	fixture.grantSettlements.records[settlement.SettlementID] = settlement
	return settlement
}

func (fixture *reportPublicationFixture) installDurableReportStageCompletion(
	t *testing.T,
	decision domainpublication.ReportDeliveryDecisionV1,
	settlement domainpublication.ReportGrantSettlementV1,
) domainpublication.ReportStageCompletionV1 {
	t.Helper()
	settledAt, err := time.Parse(time.RFC3339Nano, settlement.SettledAt)
	if err != nil {
		t.Fatal(err)
	}
	disposition, err := domainpendingwork.NewPendingWorkDispositionV1(
		fixture.stages.receipt, domainpendingwork.StatusCompleted, "report_stage_completed", settledAt,
		fixture.authority.keyID, fixture.authority.publicKey,
		func(message []byte) ([]byte, error) { return fixture.authority.Sign(context.Background(), message) },
	)
	if err != nil {
		t.Fatal(err)
	}
	completion, err := domainpublication.NewReportStageCompletionV1(
		domainpublication.ReportStageCompletionInputV1{
			Decision: decision, GrantSettlement: settlement, StageReceipt: fixture.stages.receipt,
			StageDisposition: disposition, AuthorityKeyID: fixture.authority.keyID, AuthorityPublicKey: fixture.authority.publicKey,
		},
		func(message []byte) ([]byte, error) { return fixture.authority.Sign(context.Background(), message) },
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture.stages.disposition = &disposition
	fixture.stageCompletions.records[completion.CompletionID] = completion
	return completion
}

func (fixture *reportPublicationFixture) commitSelectorConfig() CommitSelectorConfigV1 {
	bundle := fixture.initialSnapshot.Head.Bundle
	return CommitSelectorConfigV1{
		InstallationID: bundle.InstallationID, EnrollmentID: bundle.EnrollmentID, Authority: fixture.authority,
		WitnessKeyID: domainsecurity.SHA256Hex(fixture.coordinator.witnessPublic), WitnessKey: fixture.coordinator.witnessPublic,
		HeadReader: fixture.coordinator, Bundles: fixture.bundles, Observations: fixture.observations,
		Selections: fixture.selections, Commits: fixture.commits,
	}
}

func (fixture *reportPublicationFixture) onlyPublicationAttempt(t *testing.T) domainpublication.PublicationAttemptV1 {
	t.Helper()
	if len(fixture.attempts.records) != 1 {
		t.Fatalf("publication attempt count = %d, want 1", len(fixture.attempts.records))
	}
	for _, attempt := range fixture.attempts.records {
		return attempt
	}
	return domainpublication.PublicationAttemptV1{}
}

func (fixture *reportPublicationFixture) resetCommitSelectionStores() {
	fixture.selections.mu.Lock()
	fixture.selections.records = map[string]domainpublication.PublicationCommitSelectionV1{}
	fixture.selections.createCalls = 0
	fixture.selections.resolveCalls = 0
	fixture.selections.createErr = nil
	fixture.selections.commitOnErr = false
	fixture.selections.beforeCreate = nil
	fixture.selections.resolveErr = nil
	fixture.selections.mu.Unlock()
	fixture.commits.mu.Lock()
	fixture.commits.records = map[string]domainpublication.PublicationCommitReceiptV1{}
	fixture.commits.putCalls = 0
	fixture.commits.resolveCalls = 0
	fixture.commits.mu.Unlock()
	fixture.decisions.mu.Lock()
	fixture.decisions.records = map[string]domainpublication.ReportDeliveryDecisionV1{}
	fixture.decisions.createCalls = 0
	fixture.decisions.resolveCalls = 0
	fixture.decisions.createErr = nil
	fixture.decisions.commitOnErr = false
	fixture.decisions.mu.Unlock()
}

func reportPublicationRegistry(
	t *testing.T,
	authority *reportPublicationAuthority,
	securityContext domainsecurity.TurnSecurityContext,
	now time.Time,
	controlledSource bool,
) (domainevidence.EvidenceReceiptRegistry, domainevidence.ClaimRecord, string) {
	t.Helper()
	payload := domainevidence.NormalizedClaimPayload{SubjectID: "entity-a", AccountID: reportCanonicalAccountV2}
	rawHash := domainsecurity.SHA256Hex([]byte("report-evidence-raw"))
	materialValue := domainevidence.CanonicalEvidenceMaterial{
		SchemaVersion: domainevidence.CanonicalEvidenceVersion,
		Facts: []domainevidence.CanonicalEvidenceFact{{
			FactID: "fact-account", ClaimType: domainevidence.ClaimAccount, NormalizedPayload: payload,
		}},
	}
	piiClassification := domainevidence.PIIMasked
	if controlledSource {
		sourceField, err := domainevidence.NewSourceFieldBindingV2(domainevidence.SourceFieldBindingInputV2{
			FactID: "fact-account", ClaimType: domainevidence.ClaimAccount, CanonicalEntityID: "entity-a", CanonicalAccountID: reportCanonicalAccountV2,
			SourceRecordID: "record-account", RawArtifactSHA256: rawHash,
			SourceRecordSHA256: domainsecurity.SHA256Hex([]byte("report-source-account-row")),
			SourceRecordPath:   "/record", SourceRecordIDPath: "/record/sourceRecordId", SourceEntityIDPath: "/record/entityId",
			SourceFieldPath:  "/record/accountId",
			SourceScalarKind: domainevidence.SourceFieldBindingScalarTextV2, SourceExactValue: reportSourceExactAccountV2,
		})
		if err != nil {
			t.Fatal(err)
		}
		sourceFields, err := domainevidence.CanonicalSourceFieldBindingsV2([]domainevidence.SourceFieldBindingV2{sourceField})
		if err != nil {
			t.Fatal(err)
		}
		sourceFieldSetDigest, err := domainevidence.SourceFieldBindingSetDigestV2(sourceFields)
		if err != nil {
			t.Fatal(err)
		}
		materialValue.SchemaVersion = domainevidence.CanonicalEvidenceVersionV2
		materialValue.Purpose = domainevidence.CanonicalEvidencePurposeV2
		materialValue.SourceFieldBindings = sourceFields
		materialValue.SourceFieldBindingSetDigest = sourceFieldSetDigest
		piiClassification = domainevidence.PIIControlled
	}
	materialBody, _ := json.Marshal(materialValue)
	canonical, err := domainevidence.CanonicalEvidenceBytes(materialBody)
	if err != nil {
		t.Fatal(err)
	}
	settlementID := domainsecurity.SHA256Hex([]byte("report-evidence-settlement"))
	evidenceID := domainevidence.EvidenceSettlementReceiptID(settlementID)
	identity, err := domainsecurity.NewVerifiedMCPServerIdentity(
		"analytix_funds", "analytix_funds", "1.0.0", domainsecurity.SHA256Hex([]byte("report-mcp-instance")), 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	start := now.Add(-24 * time.Hour).Format(time.RFC3339Nano)
	end := now.Format(time.RFC3339Nano)
	scope := domainevidence.EvidenceQueryRange{
		EntityIDs: []string{"entity-a"}, AccountIDs: []string{reportCanonicalAccountV2}, Directions: []string{"in"},
		StartAt: start, EndAt: end, SourceIDs: []string{"bank-flow"}, FiltersHash: domainsecurity.SHA256Hex([]byte("report-evidence-filters")),
	}
	toolCallID, err := domainmodel.NewHostToolCallIDV1(bytes.Repeat([]byte{0x65}, domainmodel.HostToolCallIDEntropyBytesV1))
	if err != nil {
		t.Fatal(err)
	}
	draft, err := domainevidence.NewEvidenceReceiptDraft(domainevidence.EvidenceReceiptInput{
		ReceiptID: evidenceID, Context: securityContext, ExecutionGrantID: domainsecurity.SHA256Hex([]byte("report-evidence-grant")),
		ToolCallID: toolCallID, ServerIdentity: identity, ServerVersion: "1.0.0", ConnectionEpoch: 1,
		ToolName: "mcp__analytix_funds__query_transactions", ArgsHash: domainsecurity.SHA256Hex([]byte("report-evidence-args")),
		ResultHash: domainsecurity.CanonicalJSONHash(canonical), SourceType: "transactions", DatasetSnapshotID: securityContext.DatasetSnapshotID,
		QueryHash: domainsecurity.SHA256Hex([]byte("report-evidence-query")), QueryRange: scope, Granularity: "transaction",
		Currency: "CNY", Timezone: "Asia/Shanghai", PaginationCompleteness: domainevidence.PaginationComplete,
		SourceRecordIDs: []string{"record-account"}, RawSHA256: rawHash,
		TransformationLineage: []domainevidence.TransformationLineageStep{}, PIIClassification: piiClassification, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := domainevidence.NewEvidenceReceiptRegistry(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	registry, _, err = domainevidence.RegisterEvidenceReceipt(registry, draft, canonical, domainevidence.EvidenceSettlementProof{
		SettlementID: settlementID, PreparedRecordDigest: domainsecurity.SHA256Hex([]byte("report-prepared-record")),
	}, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	proposal := domainevidence.ClaimProposal{
		SchemaVersion: domainevidence.ClaimProposalVersion, ProposalID: "proposal-account", ClaimType: domainevidence.ClaimAccount,
		NormalizedPayload: payload, EvidenceIDs: []string{evidenceID}, CounterEvidenceIDs: []string{},
	}
	claim, err := domainevidence.NewClaimRecord(domainevidence.ClaimRecordInput{
		ClaimID: "claim-account", Proposal: proposal, SupportState: domainevidence.ClaimVerified, EvidenceIDs: []string{evidenceID},
		CounterEvidenceIDs: []string{}, SupportedScope: &scope, AllowedWording: []string{"exact verified account"},
		ProhibitedUpgrades: []string{"do not infer ownership"},
		VerifierReceiptID:  domainevidence.VerifierReceiptDigest("claim-account", domainevidence.ClaimAccount, payload, []string{evidenceID}, []string{}, domainevidence.ClaimVerified),
		VerificationReason: "exact current evidence support", VerifiedAt: now.Add(2 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = authority
	return registry, claim, evidenceID
}

func newReportEvidenceBundle(t *testing.T, authority *reportPublicationAuthority, installationID, enrollmentID, registryRoot string, registryCount uint64, publicationRoot string, publicationCount uint64, label string) (domainevidence.EvidenceAuthorityBundleV1, error) {
	t.Helper()
	return domainevidence.NewEvidenceAuthorityBundleV1(domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Generation: 1,
		MutationID:                 domainsecurity.SHA256Hex([]byte("report-bundle-" + label)),
		DatasetSnapshotIndexDigest: domainsecurity.SHA256Hex([]byte("report-dataset-index")), DatasetSnapshotCount: 1,
		EvidenceRegistryIndexDigest: registryRoot, EvidenceRegistryCount: registryCount,
		PublicationIndexDigest: publicationRoot, PublicationCount: publicationCount,
		AuthorityKeyID: authority.keyID, AuthorityPublicKey: authority.publicKey,
	}, func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) })
}

type reportPublicationAuthority struct {
	privateKey           ed25519.PrivateKey
	publicKey            ed25519.PublicKey
	keyID                string
	mu                   sync.Mutex
	commitSigningDigests map[string]int
}

func (authority *reportPublicationAuthority) KeyID() string { return authority.keyID }
func (authority *reportPublicationAuthority) PublicKey() []byte {
	return append([]byte(nil), authority.publicKey...)
}
func (authority *reportPublicationAuthority) Sign(_ context.Context, message []byte) ([]byte, error) {
	if bytes.HasPrefix(message, []byte("analytix.publication-commit-receipt/signature/v1\x00")) {
		authority.mu.Lock()
		if authority.commitSigningDigests == nil {
			authority.commitSigningDigests = map[string]int{}
		}
		authority.commitSigningDigests[domainsecurity.SHA256Hex(message)]++
		authority.mu.Unlock()
	}
	return ed25519.Sign(authority.privateKey, message), nil
}
func (authority *reportPublicationAuthority) VerifyTrusted(_ context.Context, keyID string, publicKey, message, signature []byte) error {
	if keyID != authority.keyID || !bytes.Equal(publicKey, authority.publicKey) || !ed25519.Verify(authority.publicKey, message, signature) {
		return errors.New("test publication authority mismatch")
	}
	return nil
}

func (authority *reportPublicationAuthority) resetCommitSigningDigests() {
	authority.mu.Lock()
	authority.commitSigningDigests = map[string]int{}
	authority.mu.Unlock()
}

func (authority *reportPublicationAuthority) distinctCommitSigningDigests() int {
	authority.mu.Lock()
	defer authority.mu.Unlock()
	return len(authority.commitSigningDigests)
}

type reportStageStub struct {
	mu                        sync.Mutex
	beginCalls                int
	resolveCalls              int
	verifyCalls               int
	closeCalls                int
	putDispositionCalls       int
	beginErr                  error
	verifyErr                 error
	putDispositionErr         error
	putDispositionCommitOnErr bool
	receipt                   domainpendingwork.PendingWorkReceiptV1
	disposition               *domainpendingwork.PendingWorkDispositionV1
	trustedGrant              domainsecurity.ExecutionGrant
	trustedActiveRegistry     domainsecurity.ExecutionGrantRegistry
	trustedSettledRegistry    domainsecurity.ExecutionGrantRegistry
	trustedResultItemID       string
	trustedResultItemDigest   string
	trustedResultItem         map[string]any
	trustedSettledAt          time.Time
}

func (stage *reportStageStub) BeginReportStage(context.Context, pendingworkapp.ReportStageRequest) (pendingworkapp.ReportStageLease, error) {
	stage.mu.Lock()
	defer stage.mu.Unlock()
	stage.beginCalls++
	return pendingworkapp.ReportStageLease{}, stage.beginErr
}
func (stage *reportStageStub) ResolveReportStageLease(context.Context, pendingworkapp.ReportStageLease, pendingworkapp.ReportStageRequest) (domainpendingwork.PendingWorkReceiptV1, error) {
	stage.mu.Lock()
	defer stage.mu.Unlock()
	stage.resolveCalls++
	if domainpendingwork.ValidatePendingWorkReceiptV1(stage.receipt) != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, errors.New("test report-stage receipt is unavailable")
	}
	return stage.receipt, nil
}
func (stage *reportStageStub) VerifyReportStageRequest(context.Context, pendingworkapp.ReportStageLease, pendingworkapp.ReportStageRequest, time.Time) error {
	stage.mu.Lock()
	defer stage.mu.Unlock()
	stage.verifyCalls++
	return stage.verifyErr
}
func (stage *reportStageStub) CloseReportStageLease(context.Context, pendingworkapp.ReportStageLease, pendingworkapp.ReportStageRequest, string, time.Time) (domainpendingwork.PendingWorkDispositionV1, error) {
	stage.mu.Lock()
	defer stage.mu.Unlock()
	stage.closeCalls++
	return domainpendingwork.PendingWorkDispositionV1{}, nil
}

func (stage *reportStageStub) ReadReceipt(_ context.Context, workID string) (domainpendingwork.PendingWorkReceiptV1, error) {
	stage.mu.Lock()
	defer stage.mu.Unlock()
	if stage.receipt.WorkID != workID {
		return domainpendingwork.PendingWorkReceiptV1{}, pendingworkstoreport.ErrNotFound
	}
	return stage.receipt, nil
}

func (stage *reportStageStub) PutDispositionIfAbsent(
	_ context.Context,
	disposition domainpendingwork.PendingWorkDispositionV1,
) error {
	stage.mu.Lock()
	defer stage.mu.Unlock()
	stage.putDispositionCalls++
	if domainpendingwork.ValidatePendingWorkDispositionForReceiptV1(disposition, stage.receipt) != nil {
		return errors.New("test report-stage disposition is invalid")
	}
	if stage.disposition != nil {
		if !reflect.DeepEqual(*stage.disposition, disposition) {
			return errors.New("test report-stage disposition conflicts")
		}
		return nil
	}
	if stage.putDispositionErr == nil || stage.putDispositionCommitOnErr {
		value := disposition
		stage.disposition = &value
	}
	return stage.putDispositionErr
}

func (stage *reportStageStub) ReadDisposition(_ context.Context, workID string) (domainpendingwork.PendingWorkDispositionV1, error) {
	stage.mu.Lock()
	defer stage.mu.Unlock()
	if stage.disposition == nil || stage.disposition.WorkID != workID {
		return domainpendingwork.PendingWorkDispositionV1{}, pendingworkstoreport.ErrNotFound
	}
	return *stage.disposition, nil
}

func (stage *reportStageStub) ResolveTrustedCompletedReportStageV1(
	_ context.Context,
	workID string,
) (pendingworkapp.TrustedCompletedReportStageV1, error) {
	stage.mu.Lock()
	defer stage.mu.Unlock()
	if stage.receipt.WorkID != workID || stage.disposition == nil ||
		stage.disposition.Status != domainpendingwork.StatusCompleted ||
		domainpendingwork.ValidatePendingWorkDispositionForReceiptV1(*stage.disposition, stage.receipt) != nil ||
		stage.trustedResultItemID == "" || stage.trustedResultItem == nil {
		return pendingworkapp.TrustedCompletedReportStageV1{}, errors.New("test report-stage terminal is not trusted completed authority")
	}
	receipt := stage.receipt
	receipt.GrantMembers = append([]domainpendingwork.GrantMemberV1(nil), receipt.GrantMembers...)
	active := stage.trustedActiveRegistry
	active.Entries = append([]domainsecurity.ExecutionGrantRegistryEntry(nil), active.Entries...)
	settled := stage.trustedSettledRegistry
	settled.Entries = append([]domainsecurity.ExecutionGrantRegistryEntry(nil), settled.Entries...)
	resultItem := make(map[string]any, len(stage.trustedResultItem))
	for key, value := range stage.trustedResultItem {
		resultItem[key] = value
	}
	return pendingworkapp.TrustedCompletedReportStageV1{
		Receipt: receipt, Disposition: *stage.disposition, Grant: stage.trustedGrant,
		ActiveRegistry: active, SettledRegistry: settled,
		ResultItemID: stage.trustedResultItemID, ResultItemDigest: stage.trustedResultItemDigest,
		ResultItem: resultItem, SettledAt: stage.trustedSettledAt,
	}, nil
}

func (stage *reportStageStub) ResolveTrustedSettledReportStageForDecisionV1(
	_ context.Context,
	workID string,
	decisionID string,
	decisionRecordDigest string,
) (pendingworkapp.TrustedSettledReportStageV1, error) {
	stage.mu.Lock()
	defer stage.mu.Unlock()
	if stage.receipt.WorkID != workID || stage.trustedResultItemID == "" || stage.trustedResultItem == nil {
		return pendingworkapp.TrustedSettledReportStageV1{}, errors.Join(
			pendingworkapp.ErrGrantAuthority, errors.New("test report-stage settlement authority is unavailable"),
		)
	}
	if stage.disposition != nil {
		return pendingworkapp.TrustedSettledReportStageV1{}, errors.Join(
			pendingworkapp.ErrWorkClosed, errors.New("test report-stage work already has a disposition"),
		)
	}
	if domaintoolresult.ValidatePrivateAdmittedReportResultItemV1(
		stage.trustedResultItem, decisionID, decisionRecordDigest,
	) != nil {
		return pendingworkapp.TrustedSettledReportStageV1{}, errors.Join(
			pendingworkapp.ErrGrantAuthority, errors.New("test report-stage result is not trusted decision authority"),
		)
	}
	receipt := stage.receipt
	receipt.GrantMembers = append([]domainpendingwork.GrantMemberV1(nil), receipt.GrantMembers...)
	active := stage.trustedActiveRegistry
	active.Entries = append([]domainsecurity.ExecutionGrantRegistryEntry(nil), active.Entries...)
	settled := stage.trustedSettledRegistry
	settled.Entries = append([]domainsecurity.ExecutionGrantRegistryEntry(nil), settled.Entries...)
	resultItem := make(map[string]any, len(stage.trustedResultItem))
	for key, value := range stage.trustedResultItem {
		resultItem[key] = value
	}
	return pendingworkapp.TrustedSettledReportStageV1{
		Receipt: receipt, Grant: stage.trustedGrant, ActiveRegistry: active, SettledRegistry: settled,
		ResultItemID: stage.trustedResultItemID, ResultItemDigest: stage.trustedResultItemDigest,
		ResultItem: resultItem, SettledAt: stage.trustedSettledAt,
	}, nil
}

func (stage *reportStageStub) WithTrustedSettledReportStageForDecisionV1(
	ctx context.Context,
	workID string,
	decisionID string,
	decisionRecordDigest string,
	callback func(pendingworkapp.TrustedSettledReportStageV1) error,
) error {
	if callback == nil {
		return pendingworkapp.ErrAuthorityUnavailable
	}
	settled, err := stage.ResolveTrustedSettledReportStageForDecisionV1(ctx, workID, decisionID, decisionRecordDigest)
	if err != nil {
		return err
	}
	return callback(settled)
}

type reportEvidenceStub struct {
	mu         sync.Mutex
	first      registryport.WitnessedSnapshot
	second     registryport.WitnessedSnapshot
	third      registryport.WitnessedSnapshot
	headReader evidenceauthorityport.FreshHeadReader
	calls      int
}

type reportWitnessCapabilityStub struct {
	active   bool
	snapshot registryport.WitnessedSnapshot
}

func (stub *reportEvidenceStub) WithWitnessedSnapshot(ctx context.Context, _ domainsecurity.TurnSecurityContext, callback func(registryport.WitnessedSnapshot) error) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.calls++
	snapshot := stub.first
	if stub.calls == 2 && stub.second.Head.HasBundle {
		snapshot = stub.second
	} else if stub.calls > 2 && stub.third.Head.HasBundle {
		snapshot = stub.third
	}
	if stub.calls > 2 && stub.headReader != nil {
		head, err := stub.headReader.ObserveFresh(ctx)
		if err != nil {
			return err
		}
		snapshot.Head = head
	}
	return callback(snapshot)
}

func (stub *reportEvidenceStub) WithWitnessedSnapshotAuthority(
	ctx context.Context,
	_ domainsecurity.TurnSecurityContext,
	callback func(registryport.WitnessedSnapshot, registryport.WitnessedSnapshotCapability) error,
) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.calls++
	snapshot := stub.first
	if stub.calls == 2 && stub.second.Head.HasBundle {
		snapshot = stub.second
	} else if stub.calls > 2 && stub.third.Head.HasBundle {
		snapshot = stub.third
	}
	if stub.calls > 2 && stub.headReader != nil {
		head, err := stub.headReader.ObserveFresh(ctx)
		if err != nil {
			return err
		}
		snapshot.Head = head
	}
	capability := &reportWitnessCapabilityStub{active: true, snapshot: snapshot}
	defer func() { capability.active = false }()
	return callback(snapshot, capability)
}

func (capability *reportWitnessCapabilityStub) UseExact(
	snapshot registryport.WitnessedSnapshot,
	mutation func() error,
) error {
	if capability == nil || !capability.active || mutation == nil || !reflect.DeepEqual(snapshot, capability.snapshot) {
		return errors.New("report witness capability is inactive")
	}
	return mutation()
}

type reportPublicationCoordinator struct {
	mu             sync.Mutex
	bundle         domainevidence.EvidenceAuthorityBundleV1
	authority      *reportPublicationAuthority
	witnessPrivate ed25519.PrivateKey
	witnessPublic  ed25519.PublicKey
	observeCalls   int
	advanceCalls   int
	advanceErr     error
	advanceAckErr  error
	omitWitness    bool
	intent         domainauthorityadvance.MonotonicAdvanceIntentV2
	settlement     domainauthorityadvance.MonotonicAdvanceSettlementV2
}

func (coordinator *reportPublicationCoordinator) ObserveFresh(context.Context) (evidenceauthorityport.FreshHead, error) {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	coordinator.observeCalls++
	if coordinator.omitWitness {
		return evidenceauthorityport.FreshHead{HasBundle: true, Bundle: coordinator.bundle}, nil
	}
	return reportFreshHeadValue(coordinator.bundle, coordinator.authority, coordinator.witnessPrivate, coordinator.witnessPublic, "observe", coordinator.observeCalls)
}
func (coordinator *reportPublicationCoordinator) AdvancePublication(_ context.Context, input evidenceauthorityport.PublicationAdvanceInput) (evidenceauthorityport.FreshHead, error) {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	coordinator.advanceCalls++
	if coordinator.advanceErr != nil {
		return evidenceauthorityport.FreshHead{}, coordinator.advanceErr
	}
	previous := coordinator.bundle
	if input.ExpectedBundleDigest != previous.RecordDigest {
		return evidenceauthorityport.FreshHead{}, errors.New("test publication CAS conflict")
	}
	next, err := domainevidence.NewEvidenceAuthorityBundleV1(domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID: previous.InstallationID, EnrollmentID: previous.EnrollmentID, Generation: previous.Generation + 1,
		PreviousBundleDigest: previous.RecordDigest, MutationID: domainsecurity.SHA256Hex([]byte("publication-bundle-" + input.NextIndexDigest)),
		DatasetSnapshotIndexDigest: previous.DatasetSnapshotIndexDigest, DatasetSnapshotCount: previous.DatasetSnapshotCount,
		EvidenceRegistryIndexDigest: previous.EvidenceRegistryIndexDigest, EvidenceRegistryCount: previous.EvidenceRegistryCount,
		PublicationIndexDigest: input.NextIndexDigest, PublicationCount: previous.PublicationCount + 1,
		AuthorityKeyID: coordinator.authority.keyID, AuthorityPublicKey: coordinator.authority.publicKey,
	}, func(message []byte) ([]byte, error) { return coordinator.authority.Sign(context.Background(), message) })
	if err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	coordinator.bundle = next
	if coordinator.advanceAckErr != nil {
		return evidenceauthorityport.FreshHead{}, coordinator.advanceAckErr
	}
	if coordinator.omitWitness {
		return evidenceauthorityport.FreshHead{HasBundle: true, Bundle: next}, nil
	}
	coordinator.observeCalls++
	return reportFreshHeadValue(next, coordinator.authority, coordinator.witnessPrivate, coordinator.witnessPublic, "advance", coordinator.observeCalls)
}

func (coordinator *reportPublicationCoordinator) CommitExact(_ context.Context, intent domainauthorityadvance.MonotonicAdvanceIntentV2) (authorityadvanceapp.CommitResultV2, error) {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	coordinator.advanceCalls++
	if coordinator.advanceErr != nil {
		return authorityadvanceapp.CommitResultV2{}, coordinator.advanceErr
	}
	if domainauthorityadvance.ValidateMonotonicAdvanceIntentV2(intent) != nil || intent.Transition.EvidenceBundle == nil ||
		intent.Transition.EvidenceBundle.PreviousBundle.RecordDigest != coordinator.bundle.RecordDigest {
		return authorityadvanceapp.CommitResultV2{}, errors.New("test exact publication intent conflict")
	}
	next := intent.Transition.EvidenceBundle.NextBundle
	previousCheckpoint := intent.PreviousCheckpoint
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: intent.InstallationID, EnrollmentID: intent.EnrollmentID, Namespace: intent.Namespace,
		Generation: intent.AdvanceRequest.NextGeneration, CurrentStateDigest: intent.AdvanceRequest.NextStateDigest,
		PreviousStateDigest: previousCheckpoint.CurrentStateDigest, PreviousCheckpointDigest: previousCheckpoint.CheckpointDigest,
		FenceNonce: domainsecurity.SHA256Hex([]byte("report-next-fence-" + intent.MutationID)), MutationID: intent.MutationID,
		WitnessKeyID: domainsecurity.SHA256Hex(coordinator.witnessPublic), WitnessPublicKey: coordinator.witnessPublic,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(coordinator.witnessPrivate, message), nil })
	if err != nil {
		return authorityadvanceapp.CommitResultV2{}, err
	}
	receipt, err := domainsecurity.NewMonotonicHeadAdvanceReceiptV1(intent.AdvanceRequest, checkpoint, func(message []byte) ([]byte, error) {
		return ed25519.Sign(coordinator.witnessPrivate, message), nil
	})
	if err != nil {
		return authorityadvanceapp.CommitResultV2{}, err
	}
	settlement, err := domainauthorityadvance.NewCommittedMonotonicAdvanceSettlementV2(intent, receipt, func(message []byte) ([]byte, error) {
		return coordinator.authority.Sign(context.Background(), message)
	})
	if err != nil {
		return authorityadvanceapp.CommitResultV2{}, err
	}
	coordinator.bundle = next
	coordinator.intent = intent
	coordinator.settlement = settlement
	if coordinator.advanceAckErr != nil {
		return authorityadvanceapp.CommitResultV2{}, coordinator.advanceAckErr
	}
	return authorityadvanceapp.CommitResultV2{Intent: intent, Settlement: settlement}, nil
}

func (coordinator *reportPublicationCoordinator) RecoverExact(_ context.Context, mutationID string) (authorityadvanceapp.CommitResultV2, error) {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	if coordinator.intent.MutationID != mutationID || coordinator.settlement.MutationID != mutationID {
		return authorityadvanceapp.CommitResultV2{}, errors.New("test publication mutation is unresolved")
	}
	return authorityadvanceapp.CommitResultV2{Intent: coordinator.intent, Settlement: coordinator.settlement, Replayed: true}, nil
}

func (coordinator *reportPublicationCoordinator) ResolveIntent(
	_ context.Context,
	mutationID string,
) (domainauthorityadvance.MonotonicAdvanceIntentV2, error) {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	if coordinator.intent.MutationID != mutationID {
		return domainauthorityadvance.MonotonicAdvanceIntentV2{}, authoritystoreport.ErrNotFound
	}
	return coordinator.intent, nil
}

func (coordinator *reportPublicationCoordinator) PutIntentIfAbsent(
	context.Context,
	domainauthorityadvance.MonotonicAdvanceIntentV2,
) error {
	return errors.New("test publication coordinator intent journal is read-only")
}

func (coordinator *reportPublicationCoordinator) ResolveSettlement(
	_ context.Context,
	mutationID string,
) (domainauthorityadvance.MonotonicAdvanceSettlementV2, error) {
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	if coordinator.settlement.MutationID != mutationID {
		return domainauthorityadvance.MonotonicAdvanceSettlementV2{}, authoritystoreport.ErrNotFound
	}
	return coordinator.settlement, nil
}

func (coordinator *reportPublicationCoordinator) PutSettlementIfAbsent(
	context.Context,
	domainauthorityadvance.MonotonicAdvanceSettlementV2,
) error {
	return errors.New("test publication coordinator settlement journal is read-only")
}

func reportFreshHead(t *testing.T, bundle domainevidence.EvidenceAuthorityBundleV1, authority *reportPublicationAuthority, witnessPrivate ed25519.PrivateKey, witnessPublic ed25519.PublicKey, label string) evidenceauthorityport.FreshHead {
	t.Helper()
	head, err := reportFreshHeadValue(bundle, authority, witnessPrivate, witnessPublic, label, 1)
	if err != nil {
		t.Fatal(err)
	}
	return head
}

func reportFreshHeadValue(bundle domainevidence.EvidenceAuthorityBundleV1, authority *reportPublicationAuthority, witnessPrivate ed25519.PrivateKey, witnessPublic ed25519.PublicKey, label string, sequence int) (evidenceauthorityport.FreshHead, error) {
	witnessKeyID := domainsecurity.SHA256Hex(witnessPublic)
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: bundle.InstallationID, EnrollmentID: bundle.EnrollmentID, Namespace: bundle.Namespace,
		Generation: bundle.Generation, CurrentStateDigest: bundle.RecordDigest,
		PreviousStateDigest:      domainsecurity.SHA256Hex([]byte("report-previous-state")),
		PreviousCheckpointDigest: domainsecurity.SHA256Hex([]byte("report-previous-checkpoint")),
		FenceNonce:               domainsecurity.SHA256Hex([]byte("report-fence")), MutationID: bundle.MutationID,
		WitnessKeyID: witnessKeyID, WitnessPublicKey: witnessPublic,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(witnessPrivate, message), nil })
	if err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	request, err := domainsecurity.NewMonotonicHeadObserveRequestV1(domainsecurity.MonotonicHeadObserveRequestInputV1{
		InstallationID: bundle.InstallationID, EnrollmentID: bundle.EnrollmentID, Namespace: bundle.Namespace,
		ChallengeNonce: domainsecurity.SHA256Hex([]byte(fmt.Sprintf("%s-%d-%s", label, sequence, bundle.RecordDigest))),
		AuthorityKeyID: authority.keyID, AuthorityPublicKey: authority.publicKey,
	}, func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) })
	if err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	observation, err := domainsecurity.NewMonotonicHeadObservationV1(request, checkpoint, func(message []byte) ([]byte, error) {
		return ed25519.Sign(witnessPrivate, message), nil
	})
	if err != nil {
		return evidenceauthorityport.FreshHead{}, err
	}
	return evidenceauthorityport.FreshHead{HasBundle: true, Bundle: bundle, Request: request, Observation: observation}, nil
}

type memoryPublicationAttemptStore struct {
	mu                        sync.Mutex
	records                   map[string]domainpublication.PublicationAttemptV1
	createCalls, resolveCalls int
}

func (store *memoryPublicationAttemptStore) CreateExclusive(_ context.Context, attempt domainpublication.PublicationAttemptV1) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.createCalls++
	if current, found := store.records[attempt.AttemptID]; found {
		if !reflect.DeepEqual(current, attempt) {
			return false, errors.New("publication attempt conflict")
		}
		return false, nil
	}
	store.records[attempt.AttemptID] = attempt
	return true, nil
}

func (store *memoryPublicationAttemptStore) Resolve(_ context.Context, attemptID string) (domainpublication.PublicationAttemptV1, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.resolveCalls++
	attempt, found := store.records[attemptID]
	if !found {
		return domainpublication.PublicationAttemptV1{}, publicationport.ErrNotFound
	}
	return attempt, nil
}

func (store *memoryPublicationAttemptStore) VisitAttempts(_ context.Context, visit func(domainpublication.PublicationAttemptV1) error) error {
	store.mu.Lock()
	values := make([]domainpublication.PublicationAttemptV1, 0, len(store.records))
	for _, attempt := range store.records {
		values = append(values, attempt)
	}
	store.mu.Unlock()
	for _, attempt := range values {
		if err := visit(attempt); err != nil {
			return err
		}
	}
	return nil
}

type memoryEvidenceBundleStore struct {
	mu                     sync.Mutex
	records                map[string]domainevidence.EvidenceAuthorityBundleV1
	putCalls, resolveCalls int
}

func (store *memoryEvidenceBundleStore) PutIfAbsent(_ context.Context, bundle domainevidence.EvidenceAuthorityBundleV1) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.putCalls++
	if current, found := store.records[bundle.RecordDigest]; found && !reflect.DeepEqual(current, bundle) {
		return errors.New("evidence bundle conflict")
	}
	store.records[bundle.RecordDigest] = bundle
	return nil
}

func (store *memoryEvidenceBundleStore) Resolve(_ context.Context, digest string) (domainevidence.EvidenceAuthorityBundleV1, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.resolveCalls++
	bundle, found := store.records[digest]
	if !found {
		return domainevidence.EvidenceAuthorityBundleV1{}, errors.New("evidence bundle missing")
	}
	return bundle, nil
}

type memoryObservationStore struct {
	mu                     sync.Mutex
	records                map[string]evidenceauthorityport.ObservationBundle
	putCalls, resolveCalls int
}

func (store *memoryObservationStore) PutIfAbsent(_ context.Context, bundle evidenceauthorityport.ObservationBundle) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.putCalls++
	digest := bundle.Observation.ObservationDigest
	if current, found := store.records[digest]; found && !reflect.DeepEqual(current, bundle) {
		return errors.New("evidence observation conflict")
	}
	store.records[digest] = bundle
	return nil
}

func (store *memoryObservationStore) Resolve(_ context.Context, digest string) (evidenceauthorityport.ObservationBundle, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.resolveCalls++
	bundle, found := store.records[digest]
	if !found {
		return evidenceauthorityport.ObservationBundle{}, errors.New("evidence observation missing")
	}
	return bundle, nil
}

type restartContextAuthorityStub struct {
	mu      sync.Mutex
	current domainsecurity.TurnSecurityContext
	err     error
	calls   int
	failAt  int
}

func (stub *restartContextAuthorityStub) ResolveCurrent(
	_ context.Context,
	threadID string,
	turnID string,
	contextDigest string,
) (domainsecurity.TurnSecurityContext, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.calls++
	if stub.err != nil || (stub.failAt > 0 && stub.calls >= stub.failAt) {
		if stub.err == nil {
			return domainsecurity.TurnSecurityContext{}, errors.New("restart context became stale")
		}
		return domainsecurity.TurnSecurityContext{}, stub.err
	}
	if stub.current.ThreadID != threadID || stub.current.TurnID != turnID || stub.current.ContextDigest != contextDigest {
		return domainsecurity.TurnSecurityContext{}, errors.New("restart context is not current")
	}
	return stub.current, nil
}

type memoryPublicationReceiptStore struct {
	mu                     sync.Mutex
	records                map[string]domainpublication.PublicationReceiptV1
	putCalls, resolveCalls int
}

func (store *memoryPublicationReceiptStore) PutIfAbsent(_ context.Context, record domainpublication.PublicationReceiptV1) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.putCalls++
	store.records[record.RecordDigest] = record
	return nil
}
func (store *memoryPublicationReceiptStore) Resolve(_ context.Context, digest string) (domainpublication.PublicationReceiptV1, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.resolveCalls++
	record, ok := store.records[digest]
	if !ok {
		return domainpublication.PublicationReceiptV1{}, publicationport.ErrNotFound
	}
	return record, nil
}

type memoryPublicationCommitStore struct {
	mu                     sync.Mutex
	records                map[string]domainpublication.PublicationCommitReceiptV1
	putCalls, resolveCalls int
}

type memoryReportDeliveryDecisionStore struct {
	mu                        sync.Mutex
	records                   map[string]domainpublication.ReportDeliveryDecisionV1
	createCalls, resolveCalls int
	createErr                 error
	commitOnErr               bool
}

type memoryReportGrantSettlementStore struct {
	mu                        sync.Mutex
	records                   map[string]domainpublication.ReportGrantSettlementV1
	createCalls, resolveCalls int
	createErr                 error
	commitOnErr               bool
}

type memoryReportStageCompletionStore struct {
	mu                        sync.Mutex
	records                   map[string]domainpublication.ReportStageCompletionV1
	createCalls, resolveCalls int
	createErr                 error
	commitOnErr               bool
}

func (store *memoryReportGrantSettlementStore) CreateExclusive(
	_ context.Context,
	settlement domainpublication.ReportGrantSettlementV1,
) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.createCalls++
	if current, found := store.records[settlement.SettlementID]; found {
		if !reflect.DeepEqual(current, settlement) {
			return false, errors.New("grant settlement conflict")
		}
		return false, nil
	}
	if store.createErr == nil || store.commitOnErr {
		store.records[settlement.SettlementID] = settlement
	}
	return store.createErr == nil, store.createErr
}

func (store *memoryReportGrantSettlementStore) Resolve(
	_ context.Context,
	settlementID string,
) (domainpublication.ReportGrantSettlementV1, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.resolveCalls++
	settlement, found := store.records[settlementID]
	if !found {
		return domainpublication.ReportGrantSettlementV1{}, publicationport.ErrNotFound
	}
	return settlement, nil
}

func (store *memoryReportGrantSettlementStore) VisitGrantSettlements(
	_ context.Context,
	visit func(domainpublication.ReportGrantSettlementV1) error,
) error {
	store.mu.Lock()
	values := make([]domainpublication.ReportGrantSettlementV1, 0, len(store.records))
	for _, settlement := range store.records {
		values = append(values, settlement)
	}
	store.mu.Unlock()
	for _, settlement := range values {
		if err := visit(settlement); err != nil {
			return err
		}
	}
	return nil
}

func (store *memoryReportStageCompletionStore) CreateExclusive(
	_ context.Context,
	completion domainpublication.ReportStageCompletionV1,
) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.createCalls++
	if current, found := store.records[completion.CompletionID]; found {
		if !reflect.DeepEqual(current, completion) {
			return false, errors.New("stage completion conflict")
		}
		return false, nil
	}
	if store.createErr == nil || store.commitOnErr {
		store.records[completion.CompletionID] = completion
	}
	return store.createErr == nil, store.createErr
}

func (store *memoryReportStageCompletionStore) Resolve(
	_ context.Context,
	completionID string,
) (domainpublication.ReportStageCompletionV1, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.resolveCalls++
	completion, found := store.records[completionID]
	if !found {
		return domainpublication.ReportStageCompletionV1{}, publicationport.ErrNotFound
	}
	return completion, nil
}

func (store *memoryReportStageCompletionStore) VisitStageCompletions(
	_ context.Context,
	visit func(domainpublication.ReportStageCompletionV1) error,
) error {
	store.mu.Lock()
	values := make([]domainpublication.ReportStageCompletionV1, 0, len(store.records))
	for _, completion := range store.records {
		values = append(values, completion)
	}
	store.mu.Unlock()
	for _, completion := range values {
		if err := visit(completion); err != nil {
			return err
		}
	}
	return nil
}

type restartThreadReaderStub struct {
	records map[string]map[string]any
	err     error
}

func (stub *restartThreadReaderStub) GetThread(threadID string) (map[string]any, error) {
	if stub.err != nil {
		return nil, stub.err
	}
	thread, found := stub.records[threadID]
	if !found {
		return nil, errors.New("thread missing")
	}
	return thread, nil
}

type memoryPublicationSelectionStore struct {
	mu                        sync.Mutex
	records                   map[string]domainpublication.PublicationCommitSelectionV1
	createCalls, resolveCalls int
	createErr                 error
	commitOnErr               bool
	beforeCreate              func()
	resolveErr                error
}

func (store *memoryPublicationSelectionStore) CreateExclusive(
	_ context.Context,
	selection domainpublication.PublicationCommitSelectionV1,
) (bool, error) {
	if store.beforeCreate != nil {
		store.beforeCreate()
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.createCalls++
	if _, found := store.records[selection.SelectionID]; found {
		return false, nil
	}
	if store.createErr == nil || store.commitOnErr {
		store.records[selection.SelectionID] = selection
	}
	return store.createErr == nil, store.createErr
}

func (store *memoryPublicationSelectionStore) Resolve(
	_ context.Context,
	selectionID string,
) (domainpublication.PublicationCommitSelectionV1, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.resolveCalls++
	if store.resolveErr != nil {
		return domainpublication.PublicationCommitSelectionV1{}, store.resolveErr
	}
	selection, found := store.records[selectionID]
	if !found {
		return domainpublication.PublicationCommitSelectionV1{}, publicationport.ErrNotFound
	}
	return selection, nil
}

func (store *memoryPublicationSelectionStore) VisitCommitSelections(
	_ context.Context,
	visit func(domainpublication.PublicationCommitSelectionV1) error,
) error {
	store.mu.Lock()
	values := make([]domainpublication.PublicationCommitSelectionV1, 0, len(store.records))
	for _, selection := range store.records {
		values = append(values, selection)
	}
	store.mu.Unlock()
	for _, selection := range values {
		if err := visit(selection); err != nil {
			return err
		}
	}
	return nil
}

func (store *memoryPublicationCommitStore) PutIfAbsent(_ context.Context, record domainpublication.PublicationCommitReceiptV1) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.putCalls++
	store.records[record.RecordDigest] = record
	return nil
}

func (store *memoryPublicationCommitStore) Resolve(_ context.Context, digest string) (domainpublication.PublicationCommitReceiptV1, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.resolveCalls++
	record, ok := store.records[digest]
	if !ok {
		return domainpublication.PublicationCommitReceiptV1{}, publicationport.ErrNotFound
	}
	return record, nil
}

func (store *memoryReportDeliveryDecisionStore) CreateExclusive(
	_ context.Context,
	decision domainpublication.ReportDeliveryDecisionV1,
) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.createCalls++
	if existing, found := store.records[decision.DecisionID]; found {
		if !reflect.DeepEqual(existing, decision) {
			return false, errors.New("delivery decision conflict")
		}
		return false, store.createErr
	}
	if store.createErr == nil || store.commitOnErr {
		store.records[decision.DecisionID] = decision
	}
	return store.createErr == nil, store.createErr
}

func (store *memoryReportDeliveryDecisionStore) Resolve(
	_ context.Context,
	decisionID string,
) (domainpublication.ReportDeliveryDecisionV1, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.resolveCalls++
	decision, found := store.records[decisionID]
	if !found {
		return domainpublication.ReportDeliveryDecisionV1{}, publicationport.ErrNotFound
	}
	return decision, nil
}

func (store *memoryReportDeliveryDecisionStore) VisitDeliveryDecisions(
	_ context.Context,
	visit func(domainpublication.ReportDeliveryDecisionV1) error,
) error {
	store.mu.Lock()
	values := make([]domainpublication.ReportDeliveryDecisionV1, 0, len(store.records))
	for _, decision := range store.records {
		values = append(values, decision)
	}
	store.mu.Unlock()
	for _, decision := range values {
		if err := visit(decision); err != nil {
			return err
		}
	}
	return nil
}

type memoryPublicationIndexStore struct {
	mu                     sync.Mutex
	records                map[string]domainpublication.PublicationIndexV1
	putCalls, resolveCalls int
}

func (store *memoryPublicationIndexStore) PutIfAbsent(_ context.Context, record domainpublication.PublicationIndexV1) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.putCalls++
	store.records[record.IndexDigest] = record
	return nil
}
func (store *memoryPublicationIndexStore) Resolve(_ context.Context, digest string) (domainpublication.PublicationIndexV1, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.resolveCalls++
	record, ok := store.records[digest]
	if !ok {
		return domainpublication.PublicationIndexV1{}, publicationport.ErrNotFound
	}
	return record, nil
}

type reportRestartJournalStub struct {
	intent     *domainauthorityadvance.MonotonicAdvanceIntentV2
	settlement *domainauthorityadvance.MonotonicAdvanceSettlementV2
}

func (*reportRestartJournalStub) PutIntentIfAbsent(context.Context, domainauthorityadvance.MonotonicAdvanceIntentV2) error {
	return errors.New("restart journal is read-only")
}

func (journal *reportRestartJournalStub) ResolveIntent(_ context.Context, mutationID string) (domainauthorityadvance.MonotonicAdvanceIntentV2, error) {
	if journal.intent == nil || journal.intent.MutationID != mutationID {
		return domainauthorityadvance.MonotonicAdvanceIntentV2{}, authoritystoreport.ErrNotFound
	}
	return *journal.intent, nil
}

func (journal *reportRestartJournalStub) VisitIntents(_ context.Context, visit func(domainauthorityadvance.MonotonicAdvanceIntentV2) error) error {
	if journal.intent == nil {
		return nil
	}
	return visit(*journal.intent)
}

func (*reportRestartJournalStub) PutSettlementIfAbsent(context.Context, domainauthorityadvance.MonotonicAdvanceSettlementV2) error {
	return errors.New("restart journal is read-only")
}

func (journal *reportRestartJournalStub) ResolveSettlement(_ context.Context, mutationID string) (domainauthorityadvance.MonotonicAdvanceSettlementV2, error) {
	if journal.settlement == nil || journal.settlement.MutationID != mutationID {
		return domainauthorityadvance.MonotonicAdvanceSettlementV2{}, authoritystoreport.ErrNotFound
	}
	return *journal.settlement, nil
}

func (journal *reportRestartJournalStub) VisitSettlements(_ context.Context, visit func(domainauthorityadvance.MonotonicAdvanceSettlementV2) error) error {
	if journal.settlement == nil {
		return nil
	}
	return visit(*journal.settlement)
}

type memoryClaimLedgerStore struct {
	mu                     sync.Mutex
	records                map[string]domainpublication.ClaimLedgerV1
	putCalls, resolveCalls int
}

func (store *memoryClaimLedgerStore) PutIfAbsent(_ context.Context, record domainpublication.ClaimLedgerV1) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.putCalls++
	store.records[record.LedgerDigest] = record
	return nil
}
func (store *memoryClaimLedgerStore) Resolve(_ context.Context, digest string) (domainpublication.ClaimLedgerV1, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.resolveCalls++
	record, ok := store.records[digest]
	if !ok {
		return domainpublication.ClaimLedgerV1{}, errors.New("ledger missing")
	}
	return record, nil
}

type memoryPIIProjectionStore struct {
	mu                     sync.Mutex
	records                map[string]domainpublication.PIIProjectionV1
	putCalls, resolveCalls int
}

func (store *memoryPIIProjectionStore) PutIfAbsent(_ context.Context, record domainpublication.PIIProjectionV1) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.putCalls++
	store.records[record.ProjectionDigest] = record
	return nil
}
func (store *memoryPIIProjectionStore) Resolve(_ context.Context, digest string) (domainpublication.PIIProjectionV1, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.resolveCalls++
	record, ok := store.records[digest]
	if !ok {
		return domainpublication.PIIProjectionV1{}, errors.New("projection missing")
	}
	return record, nil
}

type memoryRenderInspectionStore struct {
	mu                     sync.Mutex
	records                map[string]domainpublication.RenderInspectionV1
	putCalls, resolveCalls int
}

func (store *memoryRenderInspectionStore) PutIfAbsent(_ context.Context, record domainpublication.RenderInspectionV1) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.putCalls++
	store.records[record.InspectionDigest] = record
	return nil
}
func (store *memoryRenderInspectionStore) Resolve(_ context.Context, digest string) (domainpublication.RenderInspectionV1, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.resolveCalls++
	record, ok := store.records[digest]
	if !ok {
		return domainpublication.RenderInspectionV1{}, errors.New("inspection missing")
	}
	return record, nil
}

type memoryArtifactStore struct {
	mu                         sync.Mutex
	records                    map[string][]byte
	installCalls, resolveCalls int
	beforeInstall              func()
}

func (store *memoryArtifactStore) InstallNoReplace(_ context.Context, digest string, body []byte) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.installCalls++
	if store.beforeInstall != nil {
		store.beforeInstall()
	}
	if current, ok := store.records[digest]; ok && !bytes.Equal(current, body) {
		return errors.New("artifact conflict")
	}
	store.records[digest] = append([]byte(nil), body...)
	return nil
}
func (store *memoryArtifactStore) ResolveExact(_ context.Context, digest string) ([]byte, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.resolveCalls++
	body, ok := store.records[digest]
	if !ok {
		return nil, errors.New("artifact missing")
	}
	return append([]byte(nil), body...), nil
}

func (store *memoryArtifactStore) ResolveControlledMetadata(
	_ context.Context,
	digest string,
) (domainpii.ControlledPIIArtifactMetadataV1, error) {
	body, ok := store.records[digest]
	if !ok {
		return domainpii.ControlledPIIArtifactMetadataV1{}, errors.New("artifact missing")
	}
	return domainpii.ControlledPIIArtifactMetadataFromBytesV1(body)
}

type completeReportSurfaceExtractorStub struct {
	calls           int
	err             error
	canonical       []byte
	canonicalByCall [][]byte
}

func (stub *completeReportSurfaceExtractorStub) ExtractCompleteCanonicalSurface(
	_ context.Context,
	_ string,
	artifact []byte,
) (publicationport.ExtractedReportSurfaceV1, error) {
	stub.calls++
	if stub.err != nil {
		return publicationport.ExtractedReportSurfaceV1{}, stub.err
	}
	canonical := artifact
	if stub.calls <= len(stub.canonicalByCall) {
		canonical = stub.canonicalByCall[stub.calls-1]
	} else if stub.canonical != nil {
		canonical = stub.canonical
	}
	return publicationport.ExtractedReportSurfaceV1{
		ExtractorID: "analytix-test-complete-surface", ExtractorVersion: "1.0.0",
		CanonicalText: bytes.Clone(canonical),
	}, nil
}

type piiAuthorizationStub struct {
	mu                            sync.Mutex
	calls, witnessedCalls, failAt int
	failWitnessedAt               int
}

func (stub *piiAuthorizationStub) ValidateCurrent(context.Context, domainsecurity.TurnSecurityContext, domainpublication.PIIProjectionV1, string) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.calls++
	if stub.failAt > 0 && stub.calls >= stub.failAt {
		return errors.New("controlled authorization expired")
	}
	return nil
}

func (stub *piiAuthorizationStub) ValidateControlledArtifactCurrent(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	projection domainpublication.PIIProjectionV1,
	metadata domainpii.ControlledPIIArtifactMetadataV1,
) error {
	return stub.validateControlledArtifact(ctx, securityContext, projection, metadata, false)
}

func (stub *piiAuthorizationStub) ValidateControlledArtifactCurrentWithinSnapshot(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	projection domainpublication.PIIProjectionV1,
	metadata domainpii.ControlledPIIArtifactMetadataV1,
	_ registryport.WitnessedSnapshot,
	_ registryport.WitnessedSnapshotCapability,
) error {
	return stub.validateControlledArtifact(ctx, securityContext, projection, metadata, true)
}

func (stub *piiAuthorizationStub) validateControlledArtifact(
	_ context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	projection domainpublication.PIIProjectionV1,
	metadata domainpii.ControlledPIIArtifactMetadataV1,
	witnessed bool,
) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.calls++
	if witnessed {
		stub.witnessedCalls++
	}
	if domainpii.ValidateControlledPIIArtifactMetadataForBindingsV1(
		metadata, securityContext, metadata.ClaimLedgerDigest, projection.RulesetHash,
		metadata.TargetIdentityDigest, projection.ProjectedContentSHA256, metadata.ByteLength,
		projection.PreservedControlledFieldCount,
	) != nil || stub.failAt > 0 && stub.calls >= stub.failAt ||
		witnessed && stub.failWitnessedAt > 0 && stub.witnessedCalls >= stub.failWitnessedAt {
		return errors.New("controlled authorization expired")
	}
	return nil
}

type deliveryProjectionStub struct {
	mu           sync.Mutex
	calls        int
	resolveCalls int
	commit       domainpublication.PublicationCommitReceiptV1
	projection   domainpublication.ReportDeliveryProjectionV1
	outcome      domainpublication.ReportDeliveryOutcomeV1
	err          error
	commitOnErr  bool
}

type deliveryProjectionRaceProbe struct {
	delegate             *deliveryProjectionStub
	projectCommitted     chan struct{}
	allowProjectReturn   chan struct{}
	readbackObserved     chan struct{}
	allowReadbackReturn  chan struct{}
	readbackReturned     chan struct{}
	projectSignalOnce    sync.Once
	readbackSignalOnce   sync.Once
	readbackReturnedOnce sync.Once
}

func newDeliveryProjectionRaceProbe(delegate *deliveryProjectionStub) *deliveryProjectionRaceProbe {
	return &deliveryProjectionRaceProbe{
		delegate: delegate, projectCommitted: make(chan struct{}), allowProjectReturn: make(chan struct{}),
		readbackObserved: make(chan struct{}), allowReadbackReturn: make(chan struct{}), readbackReturned: make(chan struct{}),
	}
}

func (probe *deliveryProjectionRaceProbe) CreateOutcomeExclusive(
	ctx context.Context,
	completion domainpublication.ReportStageCompletionV1,
	outcome domainpublication.ReportDeliveryOutcomeV1,
) (domainpublication.ReportDeliveryOutcomeV1, bool, error) {
	winner, created, err := probe.delegate.CreateOutcomeExclusive(ctx, completion, outcome)
	if err != nil {
		return domainpublication.ReportDeliveryOutcomeV1{}, false, err
	}
	probe.projectSignalOnce.Do(func() { close(probe.projectCommitted) })
	select {
	case <-probe.allowProjectReturn:
		return winner, created, nil
	case <-ctx.Done():
		return domainpublication.ReportDeliveryOutcomeV1{}, false, ctx.Err()
	}
}

func (probe *deliveryProjectionRaceProbe) ResolveOutcome(
	ctx context.Context,
	deliveryID string,
) (domainpublication.ReportDeliveryOutcomeV1, error) {
	outcome, err := probe.delegate.ResolveOutcome(ctx, deliveryID)
	if err != nil {
		return domainpublication.ReportDeliveryOutcomeV1{}, err
	}
	blocked := false
	probe.readbackSignalOnce.Do(func() {
		blocked = true
		close(probe.readbackObserved)
	})
	if blocked {
		select {
		case <-probe.allowReadbackReturn:
			probe.readbackReturnedOnce.Do(func() { close(probe.readbackReturned) })
		case <-ctx.Done():
			return domainpublication.ReportDeliveryOutcomeV1{}, ctx.Err()
		}
	}
	return outcome, nil
}

func (probe *deliveryProjectionRaceProbe) VisitDeliveryOutcomes(
	ctx context.Context,
	visit func(domainpublication.ReportDeliveryOutcomeV1) error,
) error {
	return probe.delegate.VisitDeliveryOutcomes(ctx, visit)
}

func (stub *deliveryProjectionStub) CreateOutcomeExclusive(
	_ context.Context,
	completion domainpublication.ReportStageCompletionV1,
	outcome domainpublication.ReportDeliveryOutcomeV1,
) (domainpublication.ReportDeliveryOutcomeV1, bool, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.calls++
	if domainpublication.ValidateReportDeliveryOutcomeCompletionV1(outcome, completion) != nil {
		return domainpublication.ReportDeliveryOutcomeV1{}, false, errors.New("test delivery outcome does not bind completion")
	}
	if domainpublication.ValidateReportDeliveryOutcomeV1(stub.outcome) == nil {
		return stub.outcome, false, nil
	}
	if stub.err == nil || stub.commitOnErr {
		stub.outcome = outcome
		if outcome.Kind == domainpublication.ReportDeliveryOutcomeProjectedV1 {
			stub.projection = *outcome.Projection
		}
	}
	return outcome, stub.err == nil, stub.err
}

func (stub *deliveryProjectionStub) ResolveOutcome(_ context.Context, deliveryID string) (domainpublication.ReportDeliveryOutcomeV1, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.resolveCalls++
	if domainpublication.ReportDeliveryOutcomeID(stub.outcome) != deliveryID {
		return domainpublication.ReportDeliveryOutcomeV1{}, publicationport.ErrNotFound
	}
	return stub.outcome, nil
}

func (stub *deliveryProjectionStub) VisitDeliveryOutcomes(
	_ context.Context,
	visit func(domainpublication.ReportDeliveryOutcomeV1) error,
) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if domainpublication.ValidateReportDeliveryOutcomeV1(stub.outcome) != nil {
		return nil
	}
	return visit(stub.outcome)
}

type reportRestartApplyOutcome struct {
	result RestartApplyResultV1
	err    error
}

type reportTransitionOutcome struct {
	readbackReturnedBeforeAcquire bool
	err                           error
}

func runDeliveryWinsReportTransitionRace(
	t *testing.T,
	fixture *reportPublicationFixture,
	state *subagentapp.RuntimeState,
	next domainsecurity.TurnSecurityContext,
	plan RestartPlanV1,
	config RestartApplyConfigV1,
	probe *deliveryProjectionRaceProbe,
	delegate *deliveryProjectionStub,
	effectAcquired <-chan struct{},
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	applyDone := make(chan reportRestartApplyOutcome, 1)
	go func() {
		result, err := ApplyRestartV1(ctx, plan, config)
		applyDone <- reportRestartApplyOutcome{result: result, err: err}
	}()
	waitReportSignal(t, effectAcquired, "restart effect lease")
	waitReportSignal(t, probe.projectCommitted, "durable delivery CAS")

	writerReserved := make(chan struct{})
	writerAcquired := make(chan struct{})
	transitionDone := make(chan reportTransitionOutcome, 1)
	go func() {
		transition, err := state.BeginSecurityContextTransitionWithBarrier(ctx, next, func() error {
			close(writerReserved)
			return nil
		})
		if err != nil {
			transitionDone <- reportTransitionOutcome{err: err}
			return
		}
		close(writerAcquired)
		readbackReturned := reportSignalClosed(probe.readbackReturned)
		if err := transition.Prepare(ctx, next, 5*time.Second); err != nil {
			transition.Abort()
			transitionDone <- reportTransitionOutcome{readbackReturnedBeforeAcquire: readbackReturned, err: err}
			return
		}
		setRestartCurrentContext(fixture.contexts, next)
		transitionDone <- reportTransitionOutcome{
			readbackReturnedBeforeAcquire: readbackReturned,
			err:                           transition.Commit(),
		}
	}()
	waitReportSignal(t, writerReserved, "reserved context-transition writer")
	assertReportSignalPending(t, writerAcquired, "context writer crossed an in-flight delivery CAS")

	close(probe.allowProjectReturn)
	waitReportSignal(t, probe.readbackObserved, "exact delivery readback")
	assertReportSignalPending(t, writerAcquired, "context writer crossed delivery before exact readback")
	close(probe.allowReadbackReturn)
	waitReportSignal(t, probe.readbackReturned, "completed exact delivery readback")

	apply := waitReportApplyOutcome(t, applyDone)
	transition := waitReportTransitionOutcome(t, transitionDone)
	delegate.mu.Lock()
	projectCalls := delegate.calls
	projection := delegate.projection
	delegate.mu.Unlock()
	if apply.err != nil || apply.result.Delivered != 1 || transition.err != nil ||
		!transition.readbackReturnedBeforeAcquire || projectCalls != 1 || projection.DeliveryID == "" {
		t.Fatalf("delivery winner did not linearize before context transition: result=%#v project=%d projection=%#v transition=%#v err=%v",
			apply.result, projectCalls, projection, transition, apply.err)
	}
}

func runTransitionWinsReportDeliveryRace(
	t *testing.T,
	fixture *reportPublicationFixture,
	state *subagentapp.RuntimeState,
	next domainsecurity.TurnSecurityContext,
	plan RestartPlanV1,
	config RestartApplyConfigV1,
	delegate *deliveryProjectionStub,
	effectAttempted <-chan struct{},
	effectAcquired <-chan struct{},
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	transition, err := state.BeginSecurityContextTransition(ctx, next)
	if err != nil {
		t.Fatal(err)
	}
	defer transition.Abort()
	if err := transition.Prepare(ctx, next, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	applyDone := make(chan reportRestartApplyOutcome, 1)
	go func() {
		result, applyErr := ApplyRestartV1(ctx, plan, config)
		applyDone <- reportRestartApplyOutcome{result: result, err: applyErr}
	}()
	waitReportSignal(t, effectAttempted, "blocked restart effect attempt")
	assertReportSignalPending(t, effectAcquired, "restart acquired a second gate while transition writer was held")
	select {
	case outcome := <-applyDone:
		t.Fatalf("restart completed while transition writer was held: %#v", outcome)
	default:
	}
	delegate.mu.Lock()
	projectCallsBeforeCommit := delegate.calls
	delegate.mu.Unlock()
	if projectCallsBeforeCommit != 0 {
		t.Fatalf("delivery projected before context transition committed: calls=%d", projectCallsBeforeCommit)
	}

	setRestartCurrentContext(fixture.contexts, next)
	if err := transition.Commit(); err != nil {
		t.Fatal(err)
	}
	waitReportSignal(t, effectAcquired, "post-transition restart effect lease")
	apply := waitReportApplyOutcome(t, applyDone)
	delegate.mu.Lock()
	projectCalls := delegate.calls
	projection := delegate.projection
	delegate.mu.Unlock()
	if !errors.Is(apply.err, ErrPublicationRestartUnresolved) || apply.result.Delivered != 0 ||
		projectCalls != 0 || projection.DeliveryID != "" {
		t.Fatalf("transition winner allowed stale report delivery: result=%#v project=%d projection=%#v err=%v",
			apply.result, projectCalls, projection, apply.err)
	}
}

func nextReportSecurityContext(
	t *testing.T,
	current domainsecurity.TurnSecurityContext,
	now time.Time,
	switchCase bool,
) domainsecurity.TurnSecurityContext {
	t.Helper()
	caseID := current.CaseID
	caseBindingHash := current.CaseBindingHash
	label := "epoch-bump"
	if switchCase {
		label = "case-switch"
		caseID = current.CaseID + "-next"
		caseBindingHash = domainsecurity.SHA256Hex([]byte("report-transition-binding:" + caseID))
	}
	next, err := testsecurity.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: current.ThreadID, TurnID: current.TurnID + "-" + label,
		WorkspaceRealPath: current.WorkspaceRealPath, TenantID: current.TenantID, UserID: current.UserID,
		CaseID: caseID, CaseBindingHash: caseBindingHash,
		DatasetSnapshotID:  testsecurity.DatasetSnapshotID(current.ThreadID + "\x00" + label),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("report-transition-manifest:" + label)),
		ContextEpoch:       current.ContextEpoch + 1, IssuedAt: now.Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func setRestartCurrentContext(stub *restartContextAuthorityStub, current domainsecurity.TurnSecurityContext) {
	stub.mu.Lock()
	stub.current = current
	stub.mu.Unlock()
}

func waitReportSignal(t *testing.T, signal <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", label)
	}
}

func assertReportSignalPending(t *testing.T, signal <-chan struct{}, failure string) {
	t.Helper()
	select {
	case <-signal:
		t.Fatal(failure)
	default:
	}
}

func reportSignalClosed(signal <-chan struct{}) bool {
	select {
	case <-signal:
		return true
	default:
		return false
	}
}

func waitReportApplyOutcome(t *testing.T, outcomes <-chan reportRestartApplyOutcome) reportRestartApplyOutcome {
	t.Helper()
	select {
	case outcome := <-outcomes:
		return outcome
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for report restart result")
		return reportRestartApplyOutcome{}
	}
}

func waitReportTransitionOutcome(t *testing.T, outcomes <-chan reportTransitionOutcome) reportTransitionOutcome {
	t.Helper()
	select {
	case outcome := <-outcomes:
		return outcome
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for report context transition")
		return reportTransitionOutcome{}
	}
}

func verifyStoredControlledOutcomeBindingsFixture(t *testing.T, fixture *reportPublicationFixture, result PublishResult, grant domainpii.PIIProjectionGrantV1) {
	t.Helper()
	settlement := fixture.installDurableReportGrantSettlement(t, result.Decision)
	completion := fixture.installDurableReportStageCompletion(t, result.Decision, settlement)
	if fixture.stages.disposition == nil {
		t.Fatal("actual completed report stage is missing")
	}
	sign := func(body []byte) ([]byte, error) { return fixture.authority.Sign(context.Background(), body) }
	projection, err := domainpublication.NewReportDeliveryProjectionV1(domainpublication.ReportDeliveryProjectionInputV1{
		Decision: result.Decision, GrantSettlement: settlement, StageReceipt: fixture.stages.receipt,
		StageDisposition: *fixture.stages.disposition, StageCompletion: completion,
		AuthorityKeyID: fixture.authority.keyID, AuthorityPublicKey: fixture.authority.publicKey,
	}, sign)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := domainpublication.ProjectedReportDeliveryOutcomeV1(projection)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := fixture.artifacts.ResolveControlledMetadata(context.Background(), result.Candidate.TargetIdentityDigest)
	if err != nil {
		t.Fatal(err)
	}
	materials := piiauthorizationapp.StoredControlledPublicationMaterialsV1{
		Grant: grant, Receipt: result.Candidate, Commit: result.Commit, Index: result.Index,
		Ledger: fixture.ledgers.records[result.Candidate.ClaimLedgerDigest], Projection: fixture.projections.records[result.Candidate.PIIProjectionDigest],
		Inspection: fixture.inspections.records[result.Candidate.RenderInspectionDigest], Artifact: artifact,
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, grant.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	hash := func(value string) string { return domainsecurity.SHA256Hex([]byte("R129 synthetic outcome " + value)) }
	input := domainpii.ControlledArtifactAccessReceiptInputV2{
		SecurityContext: fixture.input.PendingToolCall.SecurityContext, AccessAction: domainpii.ControlledArtifactAccessActionDisplayV1,
		ControlledHandleDigest: hash("handle"), UseSlotDigest: hash("slot"), RendererPrincipalDigest: hash("principal"), RendererGeneration: 1, BackendGeneration: 1,
		AccessPolicyDigest: grant.AccessPolicyDigest, RetentionPolicyDigest: grant.RetentionPolicyDigest,
		DeliveryID: domainpublication.ReportDeliveryOutcomeID(outcome), DeliveryOutcomeRecordDigest: projection.RecordDigest,
		PublicationCommitDigest: result.Commit.RecordDigest, PublicationReceiptDigest: result.Candidate.RecordDigest,
		PIIProjectionDigest: result.Candidate.PIIProjectionDigest, PIIAuthorizationDigest: grant.RecordDigest, ClaimLedgerDigest: grant.ClaimLedgerDigest,
		TargetIdentityDigest: grant.TargetIdentityDigest, ReleaseTargetIdentityDigest: hash("release target"),
		ArtifactSHA256: artifact.SHA256, ArtifactByteLength: artifact.ByteLength, MediaType: artifact.MediaType,
		RequestedAt: fixture.now.Add(7 * time.Minute), AuthorizedUntil: expiresAt, AuthorityKeyID: fixture.authority.keyID, AuthorityPublicKey: fixture.authority.publicKey,
	}
	for _, scenario := range []string{"exact projected outcome", "different outcome digest", "rejected outcome"} {
		t.Run(scenario, func(t *testing.T) {
			candidateInput, candidateOutcome := input, outcome
			if scenario == "different outcome digest" {
				candidateInput.DeliveryOutcomeRecordDigest = hash("other outcome")
			}
			if scenario == "rejected outcome" {
				rejection, err := domainpublication.NewReportDeliveryRejectionV1(domainpublication.ReportDeliveryRejectionInputV1{
					Decision: result.Decision, GrantSettlement: settlement, StageReceipt: fixture.stages.receipt,
					StageDisposition: *fixture.stages.disposition, StageCompletion: completion,
					ReasonCode: domainpublication.ReportDeliveryRejectionEvidenceChangedV1, WitnessObservationDigest: hash("changed witness"),
					EvidenceAuthorityBundleDigest: hash("changed bundle"), EvidenceRegistryIndexDigest: hash("changed index"),
					EvidenceRegistrySequence: result.Decision.EvidenceRegistrySequence + 1, EvidenceRegistryStateDigest: hash("changed state"),
					AuthorityKeyID: fixture.authority.keyID, AuthorityPublicKey: fixture.authority.publicKey,
				}, sign)
				if err != nil {
					t.Fatal(err)
				}
				candidateOutcome, err = domainpublication.RejectedReportDeliveryOutcomeV1(rejection)
				if err != nil {
					t.Fatal(err)
				}
				candidateInput.DeliveryID = domainpublication.ReportDeliveryOutcomeID(candidateOutcome)
				candidateInput.DeliveryOutcomeRecordDigest = domainpublication.ReportDeliveryOutcomeRecordDigest(candidateOutcome)
			}
			access, err := domainpii.NewControlledArtifactAccessReceiptV2(candidateInput, sign)
			if err != nil {
				t.Fatal(err)
			}
			err = ValidateStoredControlledAccessOutcomeV2(access, candidateOutcome, materials)
			if (err == nil) != (scenario == "exact projected outcome") {
				t.Fatalf("actual stored outcome binding classification: %v", err)
			}
			body, marshalErr := domainpublication.ReportDeliveryOutcomeV1Bytes(candidateOutcome)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			digest := domainpublication.ReportDeliveryOutcomeID(candidateOutcome)
			var inventory StoredControlledOutcomesV1
			if inventory.ValidateAccess(access, materials) == nil {
				t.Fatal("missing stored outcome admitted access")
			}
			if inventory.Add("different-id", body) == nil || inventory.Add(digest, []byte(`{}`)) == nil {
				t.Fatal("invalid stored outcome entered inventory")
			}
			if err := inventory.Add(digest, body); err != nil {
				t.Fatal(err)
			}
			if inventory.Add(digest, body) == nil {
				t.Fatal("duplicate stored outcome entered inventory")
			}
			if err := inventory.ValidateAccess(access, materials); (err == nil) != (scenario == "exact projected outcome") {
				t.Fatalf("stored owner inventory changed access binding: %v", err)
			}
		})
	}
}
