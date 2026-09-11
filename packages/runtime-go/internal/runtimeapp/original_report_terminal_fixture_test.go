//go:build darwin || linux

package runtimeapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	authorityadvancefs "analytix.local/runtime-go/internal/adapters/outbound/authorityadvancefs"
	casestore "analytix.local/runtime-go/internal/adapters/outbound/casethreadauthority"
	evidenceregistrystore "analytix.local/runtime-go/internal/adapters/outbound/evidenceregistry"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	pendingstore "analytix.local/runtime-go/internal/adapters/outbound/pendingworkstore"
	publicationstore "analytix.local/runtime-go/internal/adapters/outbound/reportpublication"
	authorityadvanceapp "analytix.local/runtime-go/internal/app/authorityadvance"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appmodel "analytix.local/runtime-go/internal/app/model"
	pendingapp "analytix.local/runtime-go/internal/app/pendingwork"
	publicationapp "analytix.local/runtime-go/internal/app/reportpublication"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainauthority "analytix.local/runtime-go/internal/domain/authorityadvance"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	authorityfixture "analytix.local/runtime-go/internal/formalauthority"
	authorityport "analytix.local/runtime-go/internal/ports/authorityadvance"
	evidenceport "analytix.local/runtime-go/internal/ports/evidenceauthority"
)

// Synthetic persisted history: the report primary/result, grant replay,
// current-key pending/records, enrolled local witness transition and stored
// observation are real. This is not dataset admission or producer/UI evidence.
func newRuntimeOriginalCompletedReportHistoryFixtureV1(t *testing.T, rejected bool) *runtimeOriginalReservedReportHistoryFixtureV1 {
	t.Helper()
	return newRuntimeOriginalReportHistoryVariantFixtureV1(t, rejected, false)
}

func newRuntimeOriginalReportHistoryVariantFixtureV1(t *testing.T, rejected, controlled bool) *runtimeOriginalReservedReportHistoryFixtureV1 {
	t.Helper()
	return newRuntimeOriginalReportHistoryWithOptionsFixtureV1(t, runtimeOriginalReportHistoryFixtureOptionsV1{rejected: rejected, controlled: controlled})
}

type runtimeOriginalReportHistoryFixtureOptionsV1 struct {
	deferRefresh            bool
	rejected                bool
	controlled              bool
	openAccess              bool
	additionalOpenAccess    bool
	preWitnessCut           publicationapp.RestartAttemptStateV1
	postWitnessCut          publicationapp.RestartAttemptStateV1
	existingHead            bool
	unknownAfterCommit      bool
	restartStatus           string
	foreignIntentWitness    bool
	foreignCommittedWitness bool
	postWitnessCoreResult   string
	foreignReportIdentity   string
	preWitnessDisposition   string
	omitFailedResult        bool
}

func newRuntimeOriginalReportHistoryWithOptionsFixtureV1(t *testing.T, options runtimeOriginalReportHistoryFixtureOptionsV1) *runtimeOriginalReservedReportHistoryFixtureV1 {
	t.Helper()
	witness, config := runtimeWitnessedRegistryConfigV2(t)
	return newRuntimeOriginalReportAtInstallationFixtureV1(t, options, witness, config)
}

func newRuntimeOriginalReportAtInstallationFixtureV1(t *testing.T, options runtimeOriginalReportHistoryFixtureOptionsV1, witness *authorityfixture.Service, config Config, supplied ...domainsecurity.TurnSecurityContext) *runtimeOriginalReservedReportHistoryFixtureV1 {
	t.Helper()
	rejected, controlled := options.rejected, options.controlled
	if (options.openAccess || options.additionalOpenAccess) && (!controlled || rejected) {
		t.Fatal("open V2 fixture requires a controlled projected report")
	}
	if options.preWitnessCut != "" && (controlled || rejected) {
		t.Fatal("materials fixture requires an ordinary unfinished report")
	}
	if options.foreignReportIdentity != "" && options.preWitnessCut != publicationapp.RestartAttemptMaterialsDurableV1 {
		t.Fatal("foreign report identity fixture requires the materials cut")
	}
	if options.preWitnessDisposition != "" && options.preWitnessCut == "" {
		t.Fatal("pre-witness disposition requires a native prefix cut")
	}
	if options.postWitnessCut != "" && (options.preWitnessCut != "" || options.preWitnessDisposition != "" || controlled || rejected) {
		t.Fatal("post-witness fixture requires an exact supported unfinished cut")
	}
	switch options.postWitnessCut {
	case "", publicationapp.RestartAttemptIntentDurableV1, publicationapp.RestartAttemptCommittedSettlementV1, publicationapp.RestartAttemptCommitSelectionV1, publicationapp.RestartAttemptCommitReceiptV1, publicationapp.RestartAttemptDeliveryDecisionV1, publicationapp.RestartAttemptGrantSettlementV1, publicationapp.RestartAttemptStageDispositionV1, publicationapp.RestartAttemptStageCompletionV1:
	default:
		t.Fatal("unknown post-witness fixture cut")
	}
	if options.unknownAfterCommit && options.postWitnessCut != publicationapp.RestartAttemptCommittedSettlementV1 {
		t.Fatal("native UNKNOWN requires the committed settlement cut")
	}
	if options.restartStatus != "" && (options.unknownAfterCommit || options.preWitnessDisposition != "" || (options.restartStatus != domainpendingwork.StatusOutcomeUnknown && options.restartStatus != domainpendingwork.StatusFailed)) {
		t.Fatal("invalid native restart disposition fixture")
	}
	closedPostWitness := options.postWitnessCut == publicationapp.RestartAttemptStageDispositionV1 || options.postWitnessCut == publicationapp.RestartAttemptStageCompletionV1
	if options.foreignIntentWitness && options.postWitnessCut != publicationapp.RestartAttemptIntentDurableV1 {
		t.Fatal("foreign intent witness requires the unsettled cut")
	}
	if options.foreignCommittedWitness && options.postWitnessCut != publicationapp.RestartAttemptCommittedSettlementV1 {
		t.Fatal("foreign witness fixture requires the committed settlement cut")
	}
	if options.postWitnessCoreResult != "" && ((options.postWitnessCut != publicationapp.RestartAttemptDeliveryDecisionV1 && options.postWitnessCut != publicationapp.RestartAttemptGrantSettlementV1) || (options.postWitnessCoreResult != "matching" && options.postWitnessCoreResult != "foreign-admission" && options.postWitnessCoreResult != "missing-admission")) {
		t.Fatal("post-witness Core result requires the exact decision cut")
	}
	switch options.preWitnessCut {
	case "", publicationapp.RestartAttemptReservedV1, publicationapp.RestartAttemptCandidateDurableV1, publicationapp.RestartAttemptMaterialsDurableV1:
	default:
		t.Fatal("unknown pre-witness fixture cut")
	}
	ctx := context.Background()
	roots, err := resolveRuntimePersistenceRoots(config)
	if err != nil {
		t.Fatal(err)
	}
	core, primaryPath := runtimeReportPreservationAtRootsFixtureV1(t, roots, false, false, false, supplied...)
	for _, owner := range runtimePublicationOwnersV1 {
		for _, root := range runtimePrivateCASExpectedRoots(roots.DataDir, owner) {
			if err := os.MkdirAll(root, 0700); err != nil {
				t.Fatal(err)
			}
		}
	}
	var stored struct {
		SecurityState domainsecurity.TurnSecurityContext `json:"securityState"`
		Turns         []struct {
			ID    string `json:"id"`
			Items []struct {
				ExecutionGrant domainsecurity.ExecutionGrant `json:"executionGrant"`
			} `json:"items"`
		} `json:"turns"`
	}
	primaryBody, err := os.ReadFile(primaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(primaryBody, &stored); err != nil {
		t.Fatal(err)
	}
	securityContext := stored.SecurityState
	fixtureIdentity := ""
	if len(supplied) > 0 {
		fixtureIdentity = "/" + securityContext.ContextDigest
	}
	var grant domainsecurity.ExecutionGrant
	for _, turn := range stored.Turns {
		if turn.ID == securityContext.TurnID {
			grant = turn.Items[0].ExecutionGrant
		}
	}
	if grant.GrantID == "" {
		t.Fatal("report fixture current turn grant is missing")
	}
	toolCallID := grant.ToolCallID
	var stageReceipt domainpendingwork.PendingWorkReceiptV1
	for _, pending := range core.pendingInventory.Receipts {
		if pending.Context.ThreadID == securityContext.ThreadID && pending.Context.TurnID == securityContext.TurnID && len(pending.GrantMembers) == 1 && pending.GrantMembers[0].GrantID == grant.GrantID {
			if stageReceipt.WorkID != "" {
				t.Fatal("ambiguous report stage fixture")
			}
			stageReceipt = pending
		}
	}
	if stageReceipt.WorkID == "" {
		t.Fatal("current report stage fixture is missing")
	}
	now, err := time.Parse(time.RFC3339Nano, securityContext.IssuedAt)
	if err != nil {
		t.Fatal(err)
	}
	authority := witness.Authority
	publicKey, keyID := authority.PublicKey(), authority.KeyID()
	sign := func(body []byte) ([]byte, error) { return authority.Sign(ctx, body) }
	lineage, err := domainsecurity.NewCaseThreadAuthorityRecord(securityContext, keyID, publicKey, sign)
	if err != nil {
		t.Fatal(err)
	}
	caseStore, err := casestore.NewStore(filepath.Join(roots.DataDir, "private", "case-thread-authority"), core.access)
	if err != nil {
		t.Fatal(err)
	}
	if err := caseStore.PutIfAbsent(ctx, lineage); err != nil {
		t.Fatal(err)
	}
	epochState, err := contextepochapp.BootstrapState(securityContext.ThreadID, securityContext.ContextEpoch, []domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(securityContext)}, now)
	if err != nil {
		t.Fatal(err)
	}
	committedContext, err := domainsecurity.NewCommittedTurnContextAuthorityRecord(securityContext, epochState, now, keyID, publicKey, sign)
	if err != nil {
		t.Fatal(err)
	}
	if err := caseStore.PutIfAbsent(ctx, committedContext); err != nil {
		t.Fatal(err)
	}
	enrolled, configured, err := loadRuntimeSharedEvidenceEnrollmentV2(ctx, config, authority)
	if err != nil || !configured {
		t.Fatalf("enrollment: %v", err)
	}
	installationID, enrollmentID := enrolled.projection.InstallationID, enrolled.projection.Enrollment.EnrollmentID
	witnessKeyID, witnessPublic := enrolled.projection.Enrollment.WitnessKeyID, enrolled.witnessKey
	composition := runtimeWitnessedRegistryDirectCompositionV2(t, witness, config)
	var initializedBundle domainevidence.EvidenceAuthorityBundleV1
	if options.existingHead {
		head, err := composition.evidence.ObserveFresh(ctx)
		if err != nil || !head.HasBundle {
			t.Fatal("existing fixture head unavailable", err)
		}
		initializedBundle = head.Bundle
	} else {
		head, err := composition.evidence.Initialize(ctx)
		if err != nil {
			t.Fatal(err)
		}
		initializedBundle = head.Bundle
	}
	// Build the evidence settlement from the actual existing report-call
	// prefix, then issue the report stage from the resulting complete registry.
	var evidencePrimary map[string]any
	if err := json.Unmarshal(primaryBody, &evidencePrimary); err != nil {
		t.Fatal(err)
	}
	grantPrefix, err := executiongrantapp.RegistryFromThread(securityContext.ThreadID, evidencePrimary, securityContext.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	var prepareDraft func(*domainevidence.EvidenceReceiptInput) ([]byte, []byte)
	if controlled {
		prepareDraft = runtimeControlledEvidenceDraftForTestV1(t)
	}
	preparedEvidence := runtimePreparedSettlementWithDraftForSemanticTestV1(t, securityContext, authority, prepareDraft, grantPrefix)
	preparedBody, err := domainevidence.PreparedEvidenceSettlementBytes(preparedEvidence)
	if err != nil {
		t.Fatal(err)
	}
	preparedPath := filepath.Join(roots.DataDir, "private", "evidence-settlements", "prepared", preparedEvidence.SettlementID[:2], preparedEvidence.SettlementID+".json")
	if err := os.MkdirAll(filepath.Dir(preparedPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(preparedPath, preparedBody, 0600); err != nil {
		t.Fatal(err)
	}
	marker, err := domainevidence.NewHostEvidenceSettlementMarker(preparedEvidence)
	if err != nil {
		t.Fatal(err)
	}
	markerRecord, err := domainevidence.HostEvidenceSettlementMarkerRecordV1(marker)
	if err != nil {
		t.Fatal(err)
	}
	evidenceGrant := preparedEvidence.ExecutionGrant
	evidenceCall := map[string]any{
		"id": domaintoolcall.ToolCallItemIDV1(securityContext.TurnID, evidenceGrant.ToolCallID), "kind": "tool_call", "role": "assistant", "status": "completed",
		"threadId": securityContext.ThreadID, "turnId": securityContext.TurnID, "toolName": evidenceGrant.ToolName, "callId": evidenceGrant.ToolCallID,
		"contextDigest": securityContext.ContextDigest, "contextEpoch": float64(securityContext.ContextEpoch), "executionGrantId": evidenceGrant.GrantID,
		"executionGrant": evidenceGrant, "arguments": domaintoolcall.WithheldArgumentsProjectionV1(), "createdAt": evidenceGrant.IssuedAt,
	}
	evidenceSettledAt := now.Add(4 * time.Minute).Format(time.RFC3339Nano)
	evidenceResult, valid := domaintoolresult.PrivateDurableToolResultItemRecordV1(map[string]any{
		"id": preparedEvidence.ResultItemID, "kind": "tool_result", "role": "tool", "status": "completed", "toolKind": "tool_call",
		"threadId": securityContext.ThreadID, "turnId": securityContext.TurnID, "toolName": evidenceGrant.ToolName, "callId": evidenceGrant.ToolCallID,
		"contextDigest": securityContext.ContextDigest, "contextEpoch": float64(securityContext.ContextEpoch), "executionGrantId": evidenceGrant.GrantID,
		"createdAt": evidenceSettledAt, "finishedAt": evidenceSettledAt, "isError": false, "hostEvidenceSettlement": markerRecord,
		"output": domaintoolresult.PublicToolResultProjectionRecordV1(domaintoolresult.WithheldProjectionV1("completed", "tool_output_private")),
	})
	if !valid {
		t.Fatal("actual evidence result is invalid")
	}
	evidenceTurn, found := appmodel.TurnByID(evidencePrimary, securityContext.TurnID)
	if !found {
		t.Fatal("report fixture evidence turn is missing")
	}
	evidenceTurn["items"] = append(evidenceTurn["items"].([]any), evidenceCall, evidenceResult)
	primaryBody, err = json.Marshal(evidencePrimary)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(primaryPath, primaryBody, 0600); err != nil {
		t.Fatal(err)
	}
	primaryBody, err = os.ReadFile(primaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(primaryBody, &evidencePrimary); err != nil {
		t.Fatal(err)
	}
	evidenceDurable, err := executiongrantapp.DurableSettlementFromThread(securityContext.ThreadID, evidencePrimary, securityContext.TurnID, preparedEvidence.ResultItemID, evidenceGrant)
	if err != nil {
		t.Fatal(err)
	}
	if evidenceDurable.ActiveRegistry.Sequence != preparedEvidence.ActiveGrantRegistrySequence || evidenceDurable.ActiveRegistry.StateDigest != preparedEvidence.ActiveGrantRegistryDigest {
		t.Fatal("persisted evidence result changed its signed active grant prefix")
	}
	reportRegistry, err := executiongrantapp.RegistryFromThread(securityContext.ThreadID, evidencePrimary, securityContext.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	reportMember, found := domainsecurity.ExecutionGrantRegistryEntryByID(reportRegistry, grant.GrantID)
	if !found || reportMember.Status != domainsecurity.GrantRegistryActive {
		t.Fatal("evidence settlement lost the original active report grant")
	}
	reissuedStage, err := domainpendingwork.NewPendingWorkReceiptV1(domainpendingwork.ReceiptInputV1{
		Kind: domainpendingwork.KindReportStage, SecurityContext: securityContext, GrantRegistrySequence: reportRegistry.Sequence, GrantRegistryDigest: reportRegistry.StateDigest,
		GrantMembers: []domainpendingwork.GrantMemberV1{{Ordinal: 1, GrantID: grant.GrantID, RegistrySequence: reportMember.Sequence, RegistryEntryDigest: reportMember.EntryDigest}},
		PayloadHash:  stageReceipt.PayloadHash, RouteHash: stageReceipt.RouteHash, IssuedAt: now.Add(4*time.Minute + time.Second), ExpiresAt: now.Add(30 * time.Minute),
		AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
	}, sign)
	if err != nil {
		t.Fatal(err)
	}
	if reissuedStage.WorkID != stageReceipt.WorkID || reissuedStage.ReceiptID == stageReceipt.ReceiptID {
		t.Fatal("actual report stage did not retain its work identity with a new signed prefix")
	}
	stageReceipt = reissuedStage
	stageBody, err := domainpendingwork.PendingWorkReceiptV1Bytes(stageReceipt)
	if err != nil {
		t.Fatal(err)
	}
	// This replaces only the seed fixture receipt before any publication
	// attempt exists. It is synthetic history construction, not live recovery.
	if err := os.WriteFile(filepath.Join(roots.DataDir, "private", "pending-work", "receipts", stageReceipt.WorkID[:2], stageReceipt.WorkID+".json"), stageBody, 0600); err != nil {
		t.Fatal(err)
	}
	registry, err := domainevidence.NewEvidenceReceiptRegistry(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	registry, evidenceReceipt, err := domainevidence.RegisterEvidenceReceipt(registry, preparedEvidence.ReceiptDraft, preparedEvidence.CanonicalEvidence, domainevidence.EvidenceSettlementProof{SettlementID: preparedEvidence.SettlementID, PreparedRecordDigest: preparedEvidence.RecordDigest}, now.Add(4*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	capsule, err := domainevidence.NewEvidenceRegistryAuthorityCapsule(securityContext, registry, keyID, publicKey, sign)
	if err != nil {
		t.Fatal(err)
	}
	registryIndex, err := domainevidence.NewEvidenceRegistryAuthorityIndexV2(domainevidence.EvidenceRegistryAuthorityIndexInputV2{InstallationID: installationID, EnrollmentID: enrollmentID, Generation: initializedBundle.EvidenceRegistryCount + 1, PreviousIndexDigest: initializedBundle.EvidenceRegistryIndexDigest, MutationID: domainsecurity.SHA256Hex([]byte("terminal fixture registry" + fixtureIdentity))}, capsule, keyID, publicKey, sign)
	if err != nil {
		t.Fatal(err)
	}
	capsules, err := evidenceregistrystore.NewAuthorityCapsuleStoreV2(filepath.Join(roots.DataDir, "private", "evidence-registry", "capsules"), core.access)
	if err != nil {
		t.Fatal(err)
	}
	indexes, err := evidenceregistrystore.NewAuthorityIndexStoreV2(filepath.Join(roots.DataDir, "private", "evidence-registry", "indexes"), core.access)
	if err != nil {
		t.Fatal(err)
	}
	if err := capsules.PutIfAbsent(ctx, capsule); err != nil {
		t.Fatal(err)
	}
	if options.existingHead {
		previous, err := indexes.Resolve(ctx, initializedBundle.EvidenceRegistryIndexDigest)
		if err != nil || domainevidence.ValidateEvidenceRegistryAuthorityIndexTransitionV2(previous, registryIndex) != nil {
			t.Fatal("second registry index lost original predecessor", err)
		}
	}
	if err := indexes.PutIfAbsent(ctx, registryIndex); err != nil {
		t.Fatal(err)
	}
	previousHead, err := composition.evidence.AdvanceEvidenceRegistry(ctx, evidenceport.RegistryAdvanceInput{ExpectedBundleDigest: initializedBundle.RecordDigest, NextIndexDigest: registryIndex.IndexDigest})
	if err != nil {
		t.Fatal(err)
	}
	previousBundle := previousHead.Bundle
	registryIndexDigest := registryIndex.IndexDigest
	evidenceID := evidenceReceipt.ReceiptID
	material, err := domainevidence.ParseCanonicalEvidenceMaterial(preparedEvidence.CanonicalEvidence)
	if err != nil || len(material.Facts) != 1 {
		t.Fatalf("terminal fixture requires one actual evidence fact: %v", err)
	}
	payload, claimType := material.Facts[0].NormalizedPayload, material.Facts[0].ClaimType
	claimID := "claim-terminal-" + string(claimType)
	claim, err := domainevidence.NewClaimRecord(domainevidence.ClaimRecordInput{
		ClaimID: claimID, Proposal: domainevidence.ClaimProposal{SchemaVersion: domainevidence.ClaimProposalVersion, ProposalID: "proposal-terminal-" + string(claimType), ClaimType: claimType, NormalizedPayload: payload, EvidenceIDs: []string{evidenceID}, CounterEvidenceIDs: []string{}},
		SupportState: domainevidence.ClaimVerified, EvidenceIDs: []string{evidenceID}, CounterEvidenceIDs: []string{}, SupportedScope: &evidenceReceipt.QueryRange,
		AllowedWording: []string{"the synthetic source contains the exact recorded fact"}, ProhibitedUpgrades: []string{"no ownership inference"},
		VerifierReceiptID: domainevidence.VerifierReceiptDigest(claimID, claimType, payload, []string{evidenceID}, []string{}, domainevidence.ClaimVerified), VerificationReason: "synthetic exact source fact", VerifiedAt: now.Add(4 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := domainpublication.NewClaimLedgerV1(domainpublication.ClaimLedgerInputV1{Context: securityContext, EvidenceReceiptIDs: []string{evidenceID}, Claims: []domainevidence.ClaimRecord{claim}, CreatedAt: now.Add(4 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	artifact := []byte("The synthetic dataset contains three rows.")
	target := domainsecurity.SHA256Hex([]byte("publication-recovery-target" + fixtureIdentity))
	mediaType := "application/pdf"
	reportSHA := domainsecurity.SHA256Hex(artifact)
	projection, err := domainpublication.NewPIIProjectionV1(domainpublication.PIIProjectionInputV1{ProjectionClass: domainpublication.PIIProjectionOrdinaryMasked, RulesetHash: domainsecurity.SHA256Hex([]byte("terminal report rules")), ProjectedContentSHA256: reportSHA})
	if err != nil {
		t.Fatal(err)
	}
	var piiGrant *domainpii.PIIProjectionGrantV1
	if controlled {
		artifact, projection, piiGrant = runtimeControlledReportArtifactForTestV1(t, core, authority, securityContext, ledger, material, target, now)
		reportSHA = domainsecurity.SHA256Hex(artifact)
		mediaType = domainpii.ControlledPIIArtifactMediaTypeV1
	}
	inspection, err := domainpublication.NewRenderInspectionV1(domainpublication.RenderInspectionInputV1{Renderer: "analytix-host", RendererVersion: "1.0.0", ReportSHA256: reportSHA, ReportByteLength: uint64(len(artifact)), MediaType: mediaType, Passed: true, IssueCodes: []string{}, InspectedAt: now.Add(4 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	reportInstallationID, reportEnrollmentID := installationID, enrollmentID
	switch options.foreignReportIdentity {
	case "installation":
		reportInstallationID = domainsecurity.SHA256Hex([]byte("foreign report installation"))
	case "enrollment":
		reportEnrollmentID = domainsecurity.SHA256Hex([]byte("foreign report enrollment"))
	case "":
	default:
		t.Fatal("unknown foreign report identity fixture")
	}
	receipt, err := domainpublication.NewPublicationReceiptV1(domainpublication.PublicationReceiptInputV1{
		InstallationID: reportInstallationID, EnrollmentID: reportEnrollmentID, Context: securityContext,
		ReportVariant:                 domainpublication.EvidenceBackedReport,
		EvidenceAuthorityBundleDigest: previousBundle.RecordDigest,
		EvidenceRegistryIndexDigest:   registryIndexDigest, EvidenceRegistryCount: previousBundle.EvidenceRegistryCount,
		EvidenceRegistrySequence: registry.Sequence, EvidenceRegistryStateDigest: registry.StateDigest,
		ClaimLedger: ledger, ReportSHA256: reportSHA, ReportByteLength: uint64(len(artifact)), MediaType: mediaType,
		PIIProjection: projection, RenderInspection: inspection, Publisher: "analytix-host", PublisherVersion: "1.0.0",
		TargetIdentityDigest: target, IssuedAt: now.Add(5 * time.Minute), AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
	}, sign)
	if err != nil {
		t.Fatal(err)
	}
	stageInputHash := domainsecurity.SHA256Hex([]byte("publication-recovery-stage-input"))
	if options.preWitnessCut != "" || options.postWitnessCut != "" {
		seedShard := stageReceipt.WorkID[:2]
		stageReceipt, stageInputHash = runtimeMaterialReportStageForTestV1(t, core, authority, securityContext, grant, stageReceipt, receipt)
		// The synthetic stage replacement may leave its obsolete seed shard.
		// Another report can share the same prefix; preserve its records and
		// remove only a confirmed empty fixture directory before native restart.
		if (options.unknownAfterCommit || options.restartStatus != "" || len(supplied) > 0) && seedShard != stageReceipt.WorkID[:2] {
			seedPath := filepath.Join(roots.DataDir, "private", "pending-work", "receipts", seedShard)
			entries, err := os.ReadDir(seedPath)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) == 0 {
				if err := os.Remove(seedPath); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	attemptID := domainpublication.PublicationAttemptIDV1(reportInstallationID, reportEnrollmentID, stageReceipt.WorkID)
	index, err := domainpublication.NewPublicationIndexV1(domainpublication.PublicationIndexInputV1{
		InstallationID: receipt.InstallationID, EnrollmentID: receipt.EnrollmentID, Generation: previousBundle.PublicationCount + 1,
		PreviousIndexDigest: previousBundle.PublicationIndexDigest,
		MutationID:          domainpublication.PublicationIndexMutationIDForAttemptV1(attemptID),
		ReceiptID:           receipt.ReceiptID, ReceiptRecordDigest: receipt.RecordDigest, TargetIdentityDigest: target,
		AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
	}, sign)
	if err != nil {
		t.Fatal(err)
	}
	committedBundle, err := domainevidence.NewEvidenceAuthorityBundleV1(domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Generation: previousBundle.Generation + 1,
		PreviousBundleDigest:        previousBundle.RecordDigest,
		MutationID:                  domainpublication.PublicationAuthorityMutationIDForAttemptV1(attemptID),
		DatasetSnapshotIndexDigest:  previousBundle.DatasetSnapshotIndexDigest,
		DatasetSnapshotCount:        previousBundle.DatasetSnapshotCount,
		EvidenceRegistryIndexDigest: previousBundle.EvidenceRegistryIndexDigest,
		EvidenceRegistryCount:       previousBundle.EvidenceRegistryCount,
		PublicationIndexDigest:      index.IndexDigest,
		PublicationCount:            previousBundle.PublicationCount + 1,
		AuthorityKeyID:              keyID,
		AuthorityPublicKey:          publicKey,
	}, sign)
	if err != nil {
		t.Fatal(err)
	}

	checkpoint := previousHead.Observation.Checkpoint
	var foreignWitnessSign domainsecurity.MonotonicHeadSignFunc
	if options.foreignCommittedWitness || options.foreignIntentWitness {
		checkpoint, foreignWitnessSign = runtimeForeignReportCheckpointForTestV1(t, checkpoint)
	}
	advanceRequest, err := domainsecurity.NewMonotonicHeadAdvanceRequestV1(domainsecurity.MonotonicHeadAdvanceRequestInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Namespace: checkpoint.Namespace, ExpectedGeneration: checkpoint.Generation,
		ExpectedCheckpointDigest: checkpoint.CheckpointDigest, ExpectedStateDigest: checkpoint.CurrentStateDigest, NextGeneration: checkpoint.Generation + 1,
		NextStateDigest: committedBundle.RecordDigest, ExpectedFenceNonce: checkpoint.FenceNonce, MutationID: committedBundle.MutationID, AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
	}, sign)
	if err != nil {
		t.Fatal(err)
	}
	advanceRoot, transition, err := domainauthority.NewEvidenceTransitionBindingV2(previousBundle, committedBundle)
	if err != nil || advanceRoot != domainauthority.AdvanceRootPublicationV2 {
		t.Fatalf("publication transition: %v", err)
	}
	intent, err := domainauthority.NewMonotonicAdvanceIntentV2(domainauthority.MonotonicAdvanceIntentInputV2{Root: advanceRoot, PreviousCheckpoint: checkpoint, AdvanceRequest: advanceRequest, Transition: transition, AuthorityKeyID: keyID, AuthorityPublicKey: publicKey}, sign)
	if err != nil {
		t.Fatal(err)
	}
	intentDigest := intent.RecordDigest
	attempt, err := domainpublication.NewPublicationAttemptV1(domainpublication.PublicationAttemptInputV1{
		InstallationID: reportInstallationID, EnrollmentID: reportEnrollmentID, ReportStageReceipt: stageReceipt,
		ToolCallID: toolCallID, StageInputHash: stageInputHash,
		Candidate: receipt, Index: index, ExpectedEvidenceBundleDigest: previousBundle.RecordDigest,
		ExpectedPublicationIndexDigest: previousBundle.PublicationIndexDigest,
		ExpectedPublicationCount:       previousBundle.PublicationCount,
		NextEvidenceBundleDigest:       committedBundle.RecordDigest,
		AuthorityAdvanceIntentDigest:   intentDigest,
		AuthorityKeyID:                 keyID, AuthorityPublicKey: publicKey,
	}, sign)
	if err != nil {
		t.Fatal(err)
	}

	stores, err := publicationstore.NewStores(filepath.Join(roots.DataDir, "private", "report-publication"), core.access)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := stores.Close(); err != nil {
			t.Error(err)
		}
	})
	finishPrefix := func() *runtimeOriginalReservedReportHistoryFixtureV1 {
		if options.preWitnessDisposition != "" || options.restartStatus == domainpendingwork.StatusFailed {
			if !options.omitFailedResult && options.preWitnessDisposition != domainpendingwork.StatusOutcomeUnknown {
				at := now.Add(6 * time.Minute).Format(time.RFC3339Nano)
				resultID := domaintoolresult.ToolResultItemIDV1(securityContext.TurnID, toolCallID)
				result, valid := domaintoolresult.PrivateDurableToolResultItemRecordV1(map[string]any{
					"id": resultID, "kind": "tool_result", "role": "tool", "status": "failed", "toolKind": "tool_call",
					"threadId": securityContext.ThreadID, "turnId": securityContext.TurnID, "toolName": grant.ToolName, "callId": toolCallID,
					"contextDigest": securityContext.ContextDigest, "contextEpoch": float64(securityContext.ContextEpoch), "executionGrantId": grant.GrantID,
					"createdAt": at, "finishedAt": at, "isError": true,
					"output": domaintoolresult.PublicToolResultProjectionRecordV1(domaintoolresult.WithheldProjectionV1("failed", "tool_output_private")),
				})
				if !valid {
					t.Fatal("native failed report result is invalid")
				}
				if options.preWitnessDisposition == domainpendingwork.StatusCancelled {
					projection := toolcatalogapp.BuildPublicToolResultProjectionV1("stage_case_report", map[string]any{"code": "tool_cancelled"}, true)
					records, err := toolcatalogapp.SettleToolResult(toolcatalogapp.ToolResultInput{
						ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, CreatedAt: at, FinishedAt: at,
						Call:       domainmodel.ToolCall{ID: toolCallID, Name: "stage_case_report", Arguments: json.RawMessage(`{}`)},
						Projection: projection, IsError: true, ContextDigest: securityContext.ContextDigest, ContextEpoch: securityContext.ContextEpoch, ExecutionGrantID: grant.GrantID,
					})
					if err != nil {
						t.Fatal(err)
					}
					result = records.ResultItem
				}
				var primary map[string]any
				if err := json.Unmarshal(primaryBody, &primary); err != nil {
					t.Fatal(err)
				}
				turn, found := appmodel.TurnByID(primary, securityContext.TurnID)
				if !found {
					t.Fatal("report fixture failed turn is missing")
				}
				turn["items"] = append(turn["items"].([]any), result)
				body, err := json.Marshal(primary)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(primaryPath, body, 0600); err != nil {
					t.Fatal(err)
				}
				body, err = os.ReadFile(primaryPath)
				if err != nil || json.Unmarshal(body, &primary) != nil {
					t.Fatal("failed report result could not be re-read")
				}
				if _, err := executiongrantapp.DurableSettlementFromThread(securityContext.ThreadID, primary, securityContext.TurnID, resultID, grant); err != nil {
					t.Fatal(err)
				}
			}
			if options.preWitnessDisposition != "" {
				reason := "report_stage_failed"
				if options.preWitnessDisposition == domainpendingwork.StatusCancelled {
					reason = "report_stage_cancelled"
				}
				if options.preWitnessDisposition == domainpendingwork.StatusOutcomeUnknown {
					reason = "report_stage_outcome_unknown_after_restart"
				}
				disposition, err := domainpendingwork.NewPendingWorkDispositionV1(stageReceipt, options.preWitnessDisposition, reason, now.Add(6*time.Minute), keyID, publicKey, sign)
				if err != nil {
					t.Fatal(err)
				}
				body, err := domainpendingwork.PendingWorkDispositionV1Bytes(disposition)
				if err != nil {
					t.Fatal(err)
				}
				store, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(roots.DataDir, "private", "pending-work", "dispositions"), 1<<20, core.access)
				if err != nil {
					t.Fatal(err)
				}
				if err := errors.Join(store.PutIfAbsent(ctx, stageReceipt.WorkID, body), store.Close()); err != nil {
					t.Fatal(err)
				}
			}
		}

		if options.unknownAfterCommit || options.restartStatus != "" {
			expectedStatus, expectedReason := options.restartStatus, "report_stage_outcome_unknown_after_restart"
			if options.unknownAfterCommit {
				expectedStatus = domainpendingwork.StatusOutcomeUnknown
			}
			if expectedStatus == domainpendingwork.StatusFailed {
				expectedReason = "report_stage_failed"
			}
			pendingStore, err := pendingstore.NewStoreContext(ctx, filepath.Join(roots.DataDir, "private", "pending-work"), core.access)
			if err != nil {
				t.Fatal(err)
			}
			// Observe the actual current primary after stage materialization.
			body, err := os.ReadFile(primaryPath)
			if err != nil {
				t.Fatal(err)
			}
			primary, err := finalauthority.ParsePrimaryThreadSnapshotV1(ctx, securityContext.ThreadID, body)
			if err != nil {
				t.Fatal(err)
			}
			service := pendingapp.NewService(authority, pendingStore, runtimeOriginalReportThreadsV1{ctx, runtimeReportSemanticPrimariesV1{securityContext.ThreadID: primary}})
			closed, err := service.CloseAllOpenOnRestart(ctx, now.Add(6*time.Minute))
			if err != nil || len(closed) != 1 || closed[0].WorkID != stageReceipt.WorkID || closed[0].Status != expectedStatus || closed[0].ReasonCode != expectedReason {
				t.Fatalf("native restart did not persist expected disposition %s: %v", expectedStatus, err)
			}
			inventory, err := service.TrustedInventoryV1(ctx)
			actual, found := inventory.Dispositions[stageReceipt.WorkID]
			if err != nil || !found || !reflect.DeepEqual(actual, closed[0]) {
				t.Fatal("native restart disposition readback failed", err)
			}
		}
		fixture := &runtimeOriginalReservedReportHistoryFixtureV1{config: config, core: core, primary: primaryPath, attempt: attempt, witnessAttempts: witness.TotalAttempts, witnessSnapshot: witness.Snapshot, setWitnessAvailable: witness.SetWitnessAvailable}
		if !options.deferRefresh {
			fixture.refresh(t)
		}
		return fixture
	}
	if previousBundle.PublicationCount != 0 {
		previous, err := stores.Indexes.Resolve(ctx, previousBundle.PublicationIndexDigest)
		if err != nil || domainpublication.ValidatePublicationIndexTransitionV1(previous, index) != nil {
			t.Fatal("second publication index lost original predecessor", err)
		}
	}
	// Stop at the actual producer ordering: write-ahead attempt, artifact and
	// materials, candidate receipt, then index; no missing suffix is installed.
	if created, err := stores.Attempts.CreateExclusive(ctx, attempt); err != nil || !created {
		t.Fatalf("native attempt reservation: created=%t err=%v", created, err)
	}
	if options.preWitnessCut == publicationapp.RestartAttemptReservedV1 {
		return finishPrefix()
	}
	for _, write := range []func() error{
		func() error { return stores.Artifacts.InstallNoReplace(ctx, target, artifact) },
		func() error { return stores.Ledgers.PutIfAbsent(ctx, ledger) }, func() error { return stores.PIIProjections.PutIfAbsent(ctx, projection) }, func() error { return stores.Inspections.PutIfAbsent(ctx, inspection) },
		func() error { return stores.Receipts.PutIfAbsent(ctx, receipt) },
	} {
		if err := write(); err != nil {
			t.Fatal(err)
		}
	}
	if options.preWitnessCut == publicationapp.RestartAttemptCandidateDurableV1 {
		return finishPrefix()
	}
	if err := stores.Indexes.PutIfAbsent(ctx, index); err != nil {
		t.Fatal(err)
	}
	if options.preWitnessCut == publicationapp.RestartAttemptMaterialsDurableV1 {
		return finishPrefix()
	}
	evidenceStores, err := openRuntimeSharedEvidenceStoresV2(roots.DataDir, core.access)
	if err != nil {
		t.Fatal(err)
	}
	if err := evidenceStores.bundles.PutIfAbsent(ctx, committedBundle); err != nil {
		t.Fatal(err)
	}
	journal, err := authorityadvancefs.NewStore(filepath.Join(roots.DataDir, "private", "authority-advance"), core.access)
	if err != nil {
		t.Fatal(err)
	}
	if options.foreignIntentWitness {
		if err := journal.PutIntentIfAbsent(ctx, intent); err != nil {
			t.Fatal(err)
		}
		if err := authorityadvanceapp.VerifyTrustedInventoryV2(ctx, journal, journal, authority); err != nil {
			t.Fatal("foreign intent lacks complete current-key inventory", err)
		}
		return finishPrefix()
	}
	if options.foreignCommittedWitness {
		// A self-consistent, current-installation-signed adversarial history.
		// The real enrollment anchor and stored witness observations stay intact.
		receipt := runtimeForeignReportAdvanceReceiptForTestV1(t, checkpoint, advanceRequest, foreignWitnessSign)
		settlement, err := domainauthority.NewCommittedMonotonicAdvanceSettlementV2(intent, receipt, sign)
		if err != nil {
			t.Fatal(err)
		}
		if err := journal.PutIntentIfAbsent(ctx, intent); err != nil {
			t.Fatal(err)
		}
		if err := journal.PutSettlementIfAbsent(ctx, settlement); err != nil {
			t.Fatal(err)
		}
		if err := authorityadvanceapp.VerifyTrustedInventoryV2(ctx, journal, journal, authority); err != nil {
			t.Fatal("foreign witness fixture lacks a valid complete current-key inventory", err)
		}
		return finishPrefix()
	}
	var intentStore authorityport.IntentStore = journal
	commitContext := ctx
	if options.postWitnessCut == publicationapp.RestartAttemptIntentDurableV1 {
		var cancel context.CancelFunc
		commitContext, cancel = context.WithCancel(ctx)
		defer cancel()
		intentStore = runtimeCancelAfterStoredReportIntentV1{IntentStore: journal, cancel: cancel}
	}
	coordinator, err := authorityadvanceapp.New(authorityadvanceapp.Config{InstallationID: installationID, EnrollmentID: enrollmentID, Namespace: enrolled.projection.Enrollment.Namespace, Authority: authority, WitnessKeyID: witnessKeyID, WitnessPublicKey: witnessPublic, Witness: enrolled.credentials.SharedEvidence, Intents: intentStore, Settlements: journal, EvidenceBundles: evidenceStores.bundles})
	if err != nil {
		t.Fatal(err)
	}
	if options.postWitnessCut == publicationapp.RestartAttemptIntentDurableV1 {
		id := intent.MutationID
		calls := witness.TotalAttempts()
		if _, err := coordinator.CommitExact(commitContext, intent); !errors.Is(err, context.Canceled) {
			t.Fatal("native intent did not stop after persistence", err)
		}
		stored, err := journal.ResolveIntent(ctx, id)
		if err != nil || stored.RecordDigest != intent.RecordDigest || domainauthority.ValidateMonotonicAdvanceIntentExactReplayV2(stored, intent.AdvanceRequest) != nil {
			t.Fatal("native unsettled intent readback is invalid", err)
		}
		if _, err := journal.ResolveSettlement(ctx, id); !errors.Is(err, authorityport.ErrNotFound) {
			t.Fatal("native intent acquired a settlement", err)
		}
		if witness.TotalAttempts() != calls {
			t.Fatal("native cancellation cut crossed the witness boundary")
		}
		return finishPrefix()
	}
	advanceResult, err := coordinator.CommitExact(ctx, intent)
	if err != nil {
		t.Fatal(err)
	}
	if options.postWitnessCut == publicationapp.RestartAttemptCommittedSettlementV1 {
		return finishPrefix()
	}
	advanced, err := composition.evidence.ObserveFresh(ctx)
	if err != nil || !advanced.HasBundle || advanced.Bundle.RecordDigest != committedBundle.RecordDigest {
		t.Fatalf("committed witnessed bundle: %v", err)
	}
	observed, err := evidenceStores.observations.Resolve(ctx, advanced.Observation.ObservationDigest)
	if err != nil {
		t.Fatal(err)
	}
	observeRequest, observation := observed.Request, observed.Observation
	commitInput := domainpublication.PublicationCommitReceiptInputV1{
		PreviousBundle: previousBundle, CommittedBundle: committedBundle,
		ObserveRequest: observeRequest, Observation: observation, Candidate: receipt, Index: index,
		InstallationID: installationID, EnrollmentID: enrollmentID,
		AuthorityKeyID: keyID, AuthorityPublicKey: publicKey, WitnessKeyID: witnessKeyID, WitnessPublicKey: witnessPublic,
	}
	settlementDigest := advanceResult.Settlement.RecordDigest
	selection, err := domainpublication.NewPublicationCommitSelectionV1(domainpublication.PublicationCommitSelectionInputV1{
		Attempt: attempt, SettlementDigest: settlementDigest, CommitInput: commitInput,
	}, sign)
	if err != nil {
		t.Fatal(err)
	}
	if options.postWitnessCut == publicationapp.RestartAttemptCommitSelectionV1 || options.postWitnessCut == publicationapp.RestartAttemptCommitReceiptV1 || options.postWitnessCut == publicationapp.RestartAttemptDeliveryDecisionV1 || options.postWitnessCut == publicationapp.RestartAttemptGrantSettlementV1 || closedPostWitness {
		created, err := stores.Selections.CreateExclusive(ctx, selection)
		if err != nil || !created {
			t.Fatal("native selection was not created", err)
		}
		stored, err := stores.Selections.Resolve(ctx, selection.SelectionID)
		if err != nil || domainpublication.ValidatePublicationCommitSelectionExactV1(stored, domainpublication.PublicationCommitSelectionInputV1{
			Attempt: attempt, SettlementDigest: settlementDigest, CommitInput: commitInput,
		}) != nil {
			t.Fatal("native selection readback is invalid", err)
		}
		if options.postWitnessCut == publicationapp.RestartAttemptCommitSelectionV1 {
			return finishPrefix()
		}
	}
	commit, err := domainpublication.NewPublicationCommitReceiptV1(commitInput, sign)
	if err != nil || domainpublication.ValidatePublicationCommitSelectionCommitV1(selection, commit) != nil {
		t.Fatalf("publication recovery commit graph is invalid: %v", err)
	}
	if options.postWitnessCut == publicationapp.RestartAttemptCommitReceiptV1 || options.postWitnessCut == publicationapp.RestartAttemptDeliveryDecisionV1 || options.postWitnessCut == publicationapp.RestartAttemptGrantSettlementV1 || closedPostWitness {
		if err := stores.Commits.PutIfAbsent(ctx, commit); err != nil {
			t.Fatal(err)
		}
		stored, err := stores.Commits.Resolve(ctx, commit.RecordDigest)
		if err != nil || domainpublication.ValidatePublicationCommitReceiptExactV1(stored, commitInput) != nil ||
			domainpublication.ValidatePublicationCommitSelectionCommitV1(selection, stored) != nil || stored.RecordDigest != commit.RecordDigest {
			t.Fatal("native commit readback is invalid", err)
		}
		if options.postWitnessCut == publicationapp.RestartAttemptCommitReceiptV1 {
			return finishPrefix()
		}
	}
	decisionInput := domainpublication.ReportDeliveryDecisionInputV1{
		Context: securityContext, StageReceipt: stageReceipt, Attempt: attempt, Candidate: receipt, Index: index,
		Selection: selection, Commit: commit, Ledger: ledger, Projection: projection, Inspection: inspection,
		CommitInput:                            commitInput,
		AuthorityAdvanceSettlementDigest:       settlementDigest,
		WitnessedEvidenceAuthorityBundleDigest: committedBundle.RecordDigest,
		WitnessedEvidenceRegistryIndexDigest:   committedBundle.EvidenceRegistryIndexDigest,
		EvidenceRegistryCount:                  committedBundle.EvidenceRegistryCount,
		EvidenceRegistrySequence:               receipt.EvidenceRegistrySequence,
		EvidenceRegistryStateDigest:            receipt.EvidenceRegistryStateDigest,
	}
	decision, err := domainpublication.NewReportDeliveryDecisionV1(decisionInput, sign)
	if err != nil {
		t.Fatalf("publication recovery delivery decision graph is invalid: %v", err)
	}
	if options.postWitnessCut == publicationapp.RestartAttemptDeliveryDecisionV1 || options.postWitnessCut == publicationapp.RestartAttemptGrantSettlementV1 || closedPostWitness {
		created, err := stores.Decisions.CreateExclusive(ctx, decision)
		if err != nil || !created {
			t.Fatal("native decision was not created", err)
		}
		stored, err := stores.Decisions.Resolve(ctx, decision.DecisionID)
		if err != nil || stored != decision || domainpublication.ValidateReportDeliveryDecisionGraphV1(stored, decisionInput) != nil {
			t.Fatal("native decision readback is invalid", err)
		}
		if options.postWitnessCut == publicationapp.RestartAttemptDeliveryDecisionV1 && options.postWitnessCoreResult == "" {
			return finishPrefix()
		}
	}

	settledAt := now.Add(8 * time.Minute)
	resultID := domaintoolresult.ToolResultItemIDV1(securityContext.TurnID, toolCallID)
	admissionID, admissionDigest := decision.DecisionID, decision.RecordDigest
	if options.postWitnessCoreResult == "foreign-admission" {
		admissionID = domainsecurity.SHA256Hex([]byte("foreign original decision id"))
		admissionDigest = domainsecurity.SHA256Hex([]byte("foreign original decision record"))
	}
	resultItem, ok := domaintoolresult.PrivateDurableToolResultItemRecordV1(map[string]any{
		"id": resultID, "kind": "tool_result", "role": "tool", "status": "completed", "threadId": securityContext.ThreadID, "turnId": securityContext.TurnID, "toolName": grant.ToolName, "callId": toolCallID, "toolKind": "tool_call", "isError": false,
		"createdAt": settledAt.Format(time.RFC3339Nano), "finishedAt": settledAt.Format(time.RFC3339Nano), "contextDigest": securityContext.ContextDigest, "contextEpoch": float64(securityContext.ContextEpoch), "executionGrantId": grant.GrantID,
		"hostReportAdmission": domaintoolresult.HostReportAdmissionRecordV1(domaintoolresult.NewHostReportAdmissionV1(admissionID, admissionDigest)),
		"output":              domaintoolresult.PublicToolResultProjectionRecordV1(domaintoolresult.WithheldProjectionV1("completed", "tool_output_private")),
	})
	if !ok {
		t.Fatal("actual report result is invalid")
	}
	if options.postWitnessCoreResult == "missing-admission" {
		delete(resultItem, "hostReportAdmission")
	}
	var primary map[string]any
	if err := json.Unmarshal(primaryBody, &primary); err != nil {
		t.Fatal(err)
	}
	turn, found := appmodel.TurnByID(primary, securityContext.TurnID)
	if !found {
		t.Fatal("report fixture result turn is missing")
	}
	turn["items"] = append(turn["items"].([]any), resultItem)
	primaryBody, err = json.Marshal(primary)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(primaryPath, primaryBody, 0600); err != nil {
		t.Fatal(err)
	}
	// Settlement inputs come from re-reading the actual persisted primary.
	primaryBody, err = os.ReadFile(primaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(primaryBody, &primary); err != nil {
		t.Fatal(err)
	}
	durable, err := executiongrantapp.DurableSettlementFromThread(securityContext.ThreadID, primary, securityContext.TurnID, resultID, grant)
	if err != nil {
		t.Fatal(err)
	}
	if options.postWitnessCoreResult != "" && options.postWitnessCut == publicationapp.RestartAttemptDeliveryDecisionV1 {
		return finishPrefix()
	}
	resultBody, err := json.Marshal(durable.ResultItem)
	if err != nil {
		t.Fatal(err)
	}
	grantSettlementInput := domainpublication.ReportGrantSettlementInputV1{
		Decision: decision, Grant: grant, ActiveRegistry: durable.ActiveRegistry, SettledRegistry: durable.SettledRegistry,
		ResultItemID:     domaintoolresult.ToolResultItemIDV1(securityContext.TurnID, toolCallID),
		ResultItemDigest: domainsecurity.CanonicalJSONHash(resultBody),
		SettledAt:        settledAt, AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
	}
	grantSettlement, err := domainpublication.NewReportGrantSettlementV1(grantSettlementInput, sign)
	if err != nil {
		t.Fatalf("publication recovery report grant settlement is invalid: %v", err)
	}
	if options.postWitnessCut == publicationapp.RestartAttemptGrantSettlementV1 || closedPostWitness {
		created, err := stores.GrantSettlements.CreateExclusive(ctx, grantSettlement)
		if err != nil || !created {
			t.Fatal("native grant settlement was not created", err)
		}
		stored, err := stores.GrantSettlements.Resolve(ctx, grantSettlement.SettlementID)
		if err != nil || stored != grantSettlement || domainpublication.ValidateReportGrantSettlementGraphV1(stored, grantSettlementInput) != nil {
			t.Fatal("native grant settlement readback is invalid", err)
		}
		if options.postWitnessCut == publicationapp.RestartAttemptGrantSettlementV1 {
			return finishPrefix()
		}
	}
	stageDisposition, err := domainpendingwork.NewPendingWorkDispositionV1(
		stageReceipt, domainpendingwork.StatusCompleted, "report_stage_completed", settledAt, keyID, publicKey,
		sign,
	)
	if err != nil {
		t.Fatal(err)
	}
	if closedPostWitness {
		body, err := domainpendingwork.PendingWorkDispositionV1Bytes(stageDisposition)
		if err != nil {
			t.Fatal(err)
		}
		root := filepath.Join(roots.DataDir, "private", "pending-work", "dispositions")
		store, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(root, 1<<20, core.access)
		if err != nil {
			t.Fatal(err)
		}
		if err := errors.Join(store.PutIfAbsent(ctx, stageReceipt.WorkID, body), store.Close()); err != nil {
			t.Fatal(err)
		}
		stored, err := os.ReadFile(filepath.Join(root, stageReceipt.WorkID[:2], stageReceipt.WorkID+".json"))
		if err != nil || !bytes.Equal(stored, body) {
			t.Fatal("native completed disposition readback changed", err)
		}
		if options.postWitnessCut == publicationapp.RestartAttemptStageDispositionV1 {
			return finishPrefix()
		}
	}
	stageCompletionInput := domainpublication.ReportStageCompletionInputV1{
		Decision: decision, GrantSettlement: grantSettlement, StageReceipt: stageReceipt, StageDisposition: stageDisposition,
		AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
	}
	stageCompletion, err := domainpublication.NewReportStageCompletionV1(stageCompletionInput, sign)
	if err != nil {
		t.Fatalf("publication recovery report stage completion is invalid: %v", err)
	}
	if options.postWitnessCut == publicationapp.RestartAttemptStageCompletionV1 {
		created, err := stores.StageCompletions.CreateExclusive(ctx, stageCompletion)
		if err != nil || !created {
			t.Fatal("native stage completion was not created", err)
		}
		stored, err := stores.StageCompletions.Resolve(ctx, stageCompletion.CompletionID)
		if err != nil || stored != stageCompletion || domainpublication.ValidateReportStageCompletionGraphV1(stored, stageCompletionInput) != nil {
			t.Fatal("native stage completion readback is invalid", err)
		}
		return finishPrefix()
	}
	deliveryProjectionInput := domainpublication.ReportDeliveryProjectionInputV1{
		Decision: decision, GrantSettlement: grantSettlement, StageReceipt: stageReceipt,
		StageDisposition: stageDisposition, StageCompletion: stageCompletion,
		AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
	}
	deliveryProjection, err := domainpublication.NewReportDeliveryProjectionV1(deliveryProjectionInput, sign)
	if err != nil {
		t.Fatalf("publication recovery report delivery projection is invalid: %v", err)
	}
	deliveryOutcome, err := domainpublication.ProjectedReportDeliveryOutcomeV1(deliveryProjection)
	if err != nil {
		t.Fatalf("publication recovery report delivery outcome is invalid: %v", err)
	}

	if rejected {
		revoked, err := domainevidence.RevokeEvidenceReceipt(registry, evidenceID, "synthetic_report_evidence_revoked", now.Add(9*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		nextCapsule, err := domainevidence.NewEvidenceRegistryAuthorityCapsule(securityContext, revoked, keyID, publicKey, sign)
		if err != nil {
			t.Fatal(err)
		}
		nextIndex, err := domainevidence.NewEvidenceRegistryAuthorityIndexV2(domainevidence.EvidenceRegistryAuthorityIndexInputV2{InstallationID: installationID, EnrollmentID: enrollmentID, Generation: registryIndex.Generation + 1, PreviousIndexDigest: registryIndex.IndexDigest, MutationID: domainsecurity.SHA256Hex([]byte("terminal fixture registry revocation"))}, nextCapsule, keyID, publicKey, sign)
		if err != nil {
			t.Fatal(err)
		}
		if err := capsules.PutIfAbsent(ctx, nextCapsule); err != nil {
			t.Fatal(err)
		}
		if err := indexes.PutIfAbsent(ctx, nextIndex); err != nil {
			t.Fatal(err)
		}
		changed, err := composition.evidence.AdvanceEvidenceRegistry(ctx, evidenceport.RegistryAdvanceInput{ExpectedBundleDigest: committedBundle.RecordDigest, NextIndexDigest: nextIndex.IndexDigest})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := evidenceStores.observations.Resolve(ctx, changed.Observation.ObservationDigest); err != nil {
			t.Fatal(err)
		}
		rejection, err := domainpublication.NewReportDeliveryRejectionV1(domainpublication.ReportDeliveryRejectionInputV1{
			Decision: decision, GrantSettlement: grantSettlement, StageReceipt: stageReceipt, StageDisposition: stageDisposition, StageCompletion: stageCompletion,
			ReasonCode: domainpublication.ReportDeliveryRejectionEvidenceChangedV1, WitnessObservationDigest: changed.Observation.ObservationDigest, EvidenceAuthorityBundleDigest: changed.Bundle.RecordDigest,
			EvidenceRegistryIndexDigest: nextIndex.IndexDigest, EvidenceRegistrySequence: revoked.Sequence, EvidenceRegistryStateDigest: revoked.StateDigest, AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
		}, sign)
		if err != nil {
			t.Fatal(err)
		}
		deliveryOutcome, err = domainpublication.RejectedReportDeliveryOutcomeV1(rejection)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, write := range []func() error{
		func() error { _, err := stores.Selections.CreateExclusive(ctx, selection); return err }, func() error { return stores.Commits.PutIfAbsent(ctx, commit) },
		func() error { _, err := stores.Decisions.CreateExclusive(ctx, decision); return err }, func() error { _, err := stores.GrantSettlements.CreateExclusive(ctx, grantSettlement); return err },
		func() error { _, err := stores.StageCompletions.CreateExclusive(ctx, stageCompletion); return err }, func() error {
			_, _, err := stores.DeliveryOutcomes.CreateOutcomeExclusive(ctx, stageCompletion, deliveryOutcome)
			return err
		},
	} {
		if err := write(); err != nil {
			t.Fatal(err)
		}
	}
	dispositionBody, err := domainpendingwork.PendingWorkDispositionV1Bytes(stageDisposition)
	if err != nil {
		t.Fatal(err)
	}
	dispositions, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(roots.DataDir, "private", "pending-work", "dispositions"), 1<<20, core.access)
	if err != nil {
		t.Fatal(err)
	}
	if err := errors.Join(dispositions.PutIfAbsent(ctx, stageReceipt.WorkID, dispositionBody), dispositions.Close()); err != nil {
		t.Fatal(err)
	}
	if controlled {
		runtimeStoreControlledAccessForTestV2(t, core, authority, securityContext, *piiGrant, ledger, receipt, projection, artifact, commit, deliveryOutcome, now, options.openAccess, "synthetic-history-use-slot")
		if options.additionalOpenAccess {
			runtimeStoreControlledAccessForTestV2(t, core, authority, securityContext, *piiGrant, ledger, receipt, projection, artifact, commit, deliveryOutcome, now, true, "synthetic-history-second-open-slot")
		}
	}
	fixture := &runtimeOriginalReservedReportHistoryFixtureV1{config: config, core: core, primary: primaryPath, attempt: attempt}
	if !options.deferRefresh {
		fixture.refresh(t)
	}
	return fixture
}

// Cancel only after the native store completes its full durable CAS authority
// update, before the coordinator can invoke the external witness.
type runtimeCancelAfterStoredReportIntentV1 struct {
	authorityport.IntentStore
	cancel context.CancelFunc
}

func (store runtimeCancelAfterStoredReportIntentV1) PutIntentIfAbsent(ctx context.Context, intent domainauthority.MonotonicAdvanceIntentV2) error {
	if err := store.IntentStore.PutIntentIfAbsent(ctx, intent); err != nil {
		return err
	}
	store.cancel()
	return nil
}
