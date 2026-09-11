//go:build darwin || linux

package runtimeapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	pendingapp "analytix.local/runtime-go/internal/app/pendingwork"
	publicationapp "analytix.local/runtime-go/internal/app/reportpublication"
	"analytix.local/runtime-go/internal/contracts"

	domainenrollment "analytix.local/runtime-go/internal/domain/authorityenrollment"
	domainpending "analytix.local/runtime-go/internal/domain/pendingwork"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	startupport "analytix.local/runtime-go/internal/ports/startup"
)

func TestRuntimeMaterialsDeferredAllowsOrdinaryHTTP(t *testing.T) {
	testRuntimeDeferredAllowsOrdinaryHTTPV1(t, runtimeOriginalReportHistoryFixtureOptionsV1{preWitnessCut: publicationapp.RestartAttemptMaterialsDurableV1}, false)
}

func TestRuntimeCommittedReportAllowsOrdinaryHTTPWitnessOutage(t *testing.T) {
	testRuntimeDeferredAllowsOrdinaryHTTPV1(t, runtimeOriginalReportHistoryFixtureOptionsV1{postWitnessCut: publicationapp.RestartAttemptCommitReceiptV1}, true)
}

func testRuntimeDeferredAllowsOrdinaryHTTPV1(t *testing.T, options runtimeOriginalReportHistoryFixtureOptionsV1, offline bool) {
	t.Helper()
	fixture := newRuntimeOriginalReportHistoryWithOptionsFixtureV1(t, options)
	workID := fixture.attempt.ReportStageWorkID
	pendingRoot := filepath.Join(fixture.core.roots.DataDir, "private", "pending-work")
	protected := []string{fixture.primary, filepath.Join(pendingRoot, "receipts", workID[:2], workID+".json")}
	for _, owner := range runtimePublicationOwnersV1 {
		protected = append(protected, filepath.Join(fixture.core.roots.DataDir, "private", owner))
	}
	if options.postWitnessCut != "" {
		history, err := prepareRuntimeOriginalReportHistoryV1(context.Background(), fixture.core, fixture.publication, fixture.advance, fixture.scope)
		if err != nil || history == nil || len(history.plan.Attempts) != 1 || history.plan.Attempts[0].State != options.postWitnessCut {
			t.Fatal("ordinary HTTP fixture lacks its exact post-witness prefix", err)
		}
		entry := history.plan.Attempts[0]
		if entry.Selection == nil || entry.Commit == nil || entry.Decision != nil || entry.GrantSettlement != nil || entry.Disposition != nil || entry.StageCompletion != nil || entry.DeliveryOutcome != nil {
			t.Fatal("ordinary HTTP fixture already has a post-commit suffix")
		}
		protected = append(protected, filepath.Join(fixture.core.roots.DataDir, "private", "authority-advance"))
		associated, _, _, observation, err := readRuntimeAssociatedSemanticInventoryV1(context.Background(), fixture.core, fixture.scope, nil)
		if err != nil {
			t.Fatal(err)
		}
		for name, entry := range associated["evidence-authority"] {
			if !entry.Directory {
				protected = append(protected, filepath.Join(fixture.core.roots.DataDir, "private", "evidence-authority", filepath.FromSlash(name)))
			}
		}
		if err := observation.Revalidate(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if offline && !fixture.setWitnessAvailable(domainenrollment.SharedEvidenceNamespaceV1, false) {
		t.Fatal("could not disable SharedWitness before initial startup and ordinary work")
	}
	before := startupWholeTreeRecordMapForTest(t, protected...)
	witnessAttempts := fixture.witnessAttempts()
	namespaceAttempts := func(namespace string) int {
		snapshot, ok := fixture.witnessSnapshot(namespace)
		if !ok {
			t.Fatal("expected enrolled witness namespace missing")
		}
		return snapshot.AttemptCalls
	}
	sharedAttempts := namespaceAttempts(domainenrollment.SharedEvidenceNamespaceV1)
	riskAttempts := namespaceAttempts(domainenrollment.ThreadRiskNamespaceV1)
	sharedBefore, _ := fixture.witnessSnapshot(domainenrollment.SharedEvidenceNamespaceV1)
	expectedSourceObservations := 0
	checkOriginal := func() {
		if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, protected...)) {
			t.Fatal("ordinary lifecycle changed deferred report original records")
		}
		if _, err := os.Lstat(filepath.Join(pendingRoot, "dispositions", workID[:2], workID+".json")); !os.IsNotExist(err) {
			t.Fatal("ordinary lifecycle fabricated deferred disposition")
		}
		// Each new boundary final attempts the independent current registry.
		// Local observation caches may grow; preservation and restart add no
		// witness calls and no publication advance or resolution is permitted.
		shared, _ := fixture.witnessSnapshot(domainenrollment.SharedEvidenceNamespaceV1)
		expectedSuccessfulObservations := expectedSourceObservations
		if offline {
			expectedSuccessfulObservations = 0
		}
		if shared.AttemptCalls-sharedAttempts != expectedSourceObservations ||
			shared.ObserveCalls-sharedBefore.ObserveCalls != expectedSuccessfulObservations ||
			shared.AdvanceCalls != sharedBefore.AdvanceCalls || shared.ResolveCalls != sharedBefore.ResolveCalls ||
			!reflect.DeepEqual(shared.Checkpoint, sharedBefore.Checkpoint) ||
			namespaceAttempts(domainenrollment.ThreadRiskNamespaceV1) != riskAttempts ||
			fixture.witnessAttempts()-witnessAttempts != expectedSourceObservations {
			t.Fatal("witness activity exceeded the independent source observation")
		}
	}
	config, threadID, _ := runRuntimePlanThenProtectedOrdinaryRestartWithCheckpointsV1(t, nil, func(t *testing.T, client *http.Client, url string) {
		checkOriginal()
		beforeDenial := fixture.witnessAttempts()
		status, result := packagedSourceUnavailableHydrationHTTPJSONV1(t, client, url, http.MethodPost, "/v1/threads/"+fixture.attempt.ThreadID+"/turns", map[string]any{"prompt": "Read the local ordinary file.", "mode": "agent"})
		if status < 400 || status == http.StatusNotFound || contracts.StringField(result, "code") == "" {
			t.Fatalf("deferred execution was not explicitly denied: status=%d", status)
		}
		for _, path := range []string{"/v1/threads/" + fixture.attempt.ThreadID, "/v1/usage?thread_id=" + fixture.attempt.ThreadID} {
			status, result := packagedSourceUnavailableHydrationHTTPJSONV1(t, client, url, http.MethodGet, path, nil)
			if status < 400 || status == http.StatusNotFound || contracts.StringField(result, "code") == "" {
				t.Fatalf("deferred public observation returned empty success: status=%d", status)
			}
		}
		checkOriginal()
		if fixture.witnessAttempts() != beforeDenial {
			t.Fatal("held report denial contacted a witness")
		}
	}, func(phase string) {
		if phase == "protected-source-unavailable" {
			expectedSourceObservations = 1
		}
		if phase == "ordinary-read-completed" {
			expectedSourceObservations = 2
		}
		shared, _ := fixture.witnessSnapshot(domainenrollment.SharedEvidenceNamespaceV1)
		t.Logf("%s witness delta: shared attempts=%d observe=%d advance=%d resolve=%d risk attempts=%d", phase, shared.AttemptCalls-sharedBefore.AttemptCalls, shared.ObserveCalls-sharedBefore.ObserveCalls, shared.AdvanceCalls-sharedBefore.AdvanceCalls, shared.ResolveCalls-sharedBefore.ResolveCalls, namespaceAttempts(domainenrollment.ThreadRiskNamespaceV1)-riskAttempts)
		checkOriginal()
		if phase == "shutdown" {
			if !fixture.setWitnessAvailable(domainenrollment.SharedEvidenceNamespaceV1, false) {
				t.Fatal("could not disable the enrolled SharedEvidence witness")
			}
			t.Log("SharedEvidence witness unavailable before fresh restart")
		}
	}, func(config *Config) {
		config.DataDir, config.ProductionDurableRoot = fixture.config.DataDir, fixture.config.ProductionDurableRoot
		config.AuthorityAnchorV1, config.AuthorityManifestRoot = fixture.config.AuthorityAnchorV1, fixture.config.AuthorityManifestRoot
		config.AuthorityCredentialProfileRoot, config.AuthorityCredentialBundleRoot = fixture.config.AuthorityCredentialProfileRoot, fixture.config.AuthorityCredentialBundleRoot
	})
	if threadID == fixture.attempt.ThreadID {
		t.Fatal("ordinary HTTP flow reused the held report thread")
	}
	checkOriginal()
	if expectedSourceObservations != 2 {
		t.Fatal("independent source observation checkpoint was not exercised")
	}
	t.Logf("ordinary witness delta: SharedEvidence=%d ThreadRisk=%d total=%d", namespaceAttempts(domainenrollment.SharedEvidenceNamespaceV1)-sharedAttempts, namespaceAttempts(domainenrollment.ThreadRiskNamespaceV1)-riskAttempts, fixture.witnessAttempts()-witnessAttempts)
	// A corrupt independent Core primary must still fail before startup writes.
	primary := filepath.Join(config.ProductionDurableRoot, "threads", threadID, "thread.json")
	if err := os.WriteFile(primary, []byte("synthetic malformed ordinary Core primary"), 0600); err != nil {
		t.Fatal(err)
	}
	corrupt := startupWholeTreeRecordMapForTest(t, config.DataDir, config.ProductionDurableRoot)
	beforeRejection := fixture.witnessAttempts()
	handler, err := NewRuntimeServerHandlerE(config)
	if err == nil {
		shutdownOwnedRuntimeHandler(t, handler)
		t.Fatal("deferred report hid a corrupt ordinary Core primary")
	}
	if !reflect.DeepEqual(corrupt, startupWholeTreeRecordMapForTest(t, config.DataDir, config.ProductionDurableRoot)) {
		t.Fatal("corrupt Core rejection changed managed state")
	}
	checkOriginal()
	if fixture.witnessAttempts() != beforeRejection {
		t.Fatal("corrupt Core rejection contacted a witness")
	}
}

func TestRuntimeMaterialsDeferredRejectsForeignIdentity(t *testing.T) {
	for _, foreign := range []string{"installation", "enrollment"} {
		t.Run(foreign, func(t *testing.T) {
			fixture := newRuntimeOriginalReportHistoryWithOptionsFixtureV1(t, runtimeOriginalReportHistoryFixtureOptionsV1{preWitnessCut: publicationapp.RestartAttemptMaterialsDurableV1, foreignReportIdentity: foreign})
			before := startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)
			attempts := fixture.witnessAttempts()
			_, err := prepareRuntimeReportPreservationBeforeRecoveryV1(context.Background(), fixture.core, fixture.core.access, fixture.publication.installation, fixture.advance)
			if err == nil {
				t.Fatal("first runtime preservation gate admitted foreign report identity signed by the current key")
			}
			if !strings.Contains(err.Error(), "publication attempt installation anchor mismatch") {
				t.Fatalf("foreign report did not reach independent identity binding: %v", err)
			}
			handler, err := NewRuntimeServerHandlerE(fixture.config)
			if err == nil {
				shutdownOwnedRuntimeHandler(t, handler)
				t.Fatal("foreign report identity allowed runtime startup")
			}
			if !strings.Contains(err.Error(), "publication attempt installation anchor mismatch") {
				t.Fatalf("full runtime did not reach independent identity binding: %v", err)
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)) || fixture.witnessAttempts() != attempts {
				t.Fatal("foreign identity refusal changed managed state or contacted a witness")
			}
		})
	}
}

func TestRuntimeMaterialsDurableReportPreservesPendingAndAllowsStartup(t *testing.T) {
	testRuntimePreWitnessReportPreservesPendingAndAllowsStartup(t, publicationapp.RestartAttemptMaterialsDurableV1)
}

func TestRuntimeEarlierPreWitnessReportPreservesPendingAndAllowsStartup(t *testing.T) {
	for _, cut := range []publicationapp.RestartAttemptStateV1{publicationapp.RestartAttemptReservedV1, publicationapp.RestartAttemptCandidateDurableV1} {
		t.Run(string(cut), func(t *testing.T) { testRuntimePreWitnessReportPreservesPendingAndAllowsStartup(t, cut) })
	}
}

func testRuntimePreWitnessReportPreservesPendingAndAllowsStartup(t *testing.T, cut publicationapp.RestartAttemptStateV1) {
	t.Helper()
	fixture := newRuntimeOriginalReportHistoryWithOptionsFixtureV1(t, runtimeOriginalReportHistoryFixtureOptionsV1{preWitnessCut: cut})
	history, err := prepareRuntimeOriginalReportHistoryV1(context.Background(), fixture.core, fixture.publication, fixture.advance, fixture.scope)
	if err != nil || history == nil || len(history.plan.Attempts) != 1 || history.plan.Attempts[0].State != cut {
		t.Fatalf("native materials did not establish the exact pre-witness cut: %v", err)
	}
	entry := history.plan.Attempts[0]
	if fixture.publication.unavailable || entry.Disposition != nil || len(fixture.scope.ThreadIDs()) != 1 {
		t.Fatal("healthy material cut lost its Original unresolved pending scope")
	}
	disposition := filepath.Join(fixture.core.roots.DataDir, "private", "pending-work", "dispositions", entry.Stage.WorkID[:2], entry.Stage.WorkID+".json")
	protected := []string{fixture.primary, filepath.Join(fixture.core.roots.DataDir, "private", "pending-work", "receipts", entry.Stage.WorkID[:2], entry.Stage.WorkID+".json")}
	for _, owner := range runtimePublicationOwnersV1 {
		protected = append(protected, filepath.Join(fixture.core.roots.DataDir, "private", owner))
	}
	before := startupWholeTreeRecordMapForTest(t, protected...)
	attempts := fixture.witnessAttempts()
	advanceRoot := filepath.Join(fixture.core.roots.DataDir, "private", "authority-advance", "v2")
	if _, err := os.Lstat(advanceRoot); !os.IsNotExist(err) {
		t.Fatal("pre-witness fixture already has an authority advance owner")
	}
	for restart := 0; restart < 2; restart++ {
		handler, err := NewRuntimeServerHandlerE(fixture.config)
		if err != nil {
			t.Fatalf("native %s cut blocked ordinary startup %d: %v", cut, restart, err)
		}
		shutdownOwnedRuntimeHandler(t, handler)
		if _, err := os.Lstat(disposition); !os.IsNotExist(err) {
			t.Fatal("deferred material cut acquired a fabricated pending disposition")
		}
		if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, protected...)) {
			t.Fatal("restart changed the exact original material or held primary/pending")
		}
		if fixture.witnessAttempts() != attempts {
			t.Fatal("deferred startup contacted a witness")
		}
		if _, err := os.Lstat(advanceRoot); !os.IsNotExist(err) {
			t.Fatal("deferred startup created an absent authority advance owner")
		}
	}
}

func TestRuntimeMaterialsDeferredRejectsResignedInputAndErasedFinal(t *testing.T) {
	for _, fault := range []string{"resigned-stage-input", "signed-final-erases-materials"} {
		t.Run(fault, func(t *testing.T) {
			ctx := context.Background()
			fixture := newRuntimeOriginalReportHistoryWithOptionsFixtureV1(t, runtimeOriginalReportHistoryFixtureOptionsV1{preWitnessCut: publicationapp.RestartAttemptMaterialsDurableV1})
			history, err := prepareRuntimeOriginalReportHistoryV1(ctx, fixture.core, fixture.publication, fixture.advance, fixture.scope)
			if err != nil {
				t.Fatal(err)
			}
			if fault == "resigned-stage-input" {
				entry, authority := history.plan.Attempts[0], fixture.publication.installation
				original := entry.Attempt
				changed, err := domainpublication.NewPublicationAttemptV1(domainpublication.PublicationAttemptInputV1{InstallationID: original.InstallationID, EnrollmentID: original.EnrollmentID, ReportStageReceipt: entry.Stage, ToolCallID: original.ToolCallID, StageInputHash: domainsecurity.SHA256Hex([]byte("different stage input with unchanged original pending")), Candidate: *entry.Candidate, Index: *entry.Index, ExpectedEvidenceBundleDigest: original.ExpectedEvidenceBundleDigest, ExpectedPublicationIndexDigest: original.ExpectedPublicationIndexDigest, ExpectedPublicationCount: original.ExpectedPublicationCount, NextEvidenceBundleDigest: original.NextEvidenceBundleDigest, AuthorityAdvanceIntentDigest: original.AuthorityAdvanceIntentDigest, AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey()}, func(body []byte) ([]byte, error) { return authority.Sign(ctx, body) })
				if err != nil {
					t.Fatal(err)
				}
				body, err := domainpublication.PublicationAttemptV1Bytes(changed)
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(fixture.core.roots.DataDir, "private", "report-publication", "attempts", changed.AttemptID[:2], changed.AttemptID+".json")
				if err := os.WriteFile(path, body, 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				interruptRuntimeErasedMaterialsFinalForTestV1(t, fixture)
			}
			before := startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)
			attempts := fixture.witnessAttempts()
			handler, err := NewRuntimeServerHandlerE(fixture.config)
			if err == nil {
				shutdownOwnedRuntimeHandler(t, handler)
				t.Fatal("changed material input/Final was accepted")
			}
			if fault == "resigned-stage-input" && !errors.Is(err, pendingapp.ErrOperationMismatch) {
				t.Fatalf("resigned input did not reach keyed pending binding: %v", err)
			}
			if fault == "signed-final-erases-materials" && !strings.Contains(err.Error(), "deferred report Original/Final prefix changed") {
				t.Fatalf("erased signed Final did not reach full prefix comparison: %v", err)
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)) || fixture.witnessAttempts() != attempts {
				t.Fatal("rejected material prefix changed managed state or contacted witness")
			}
		})
	}
}

func interruptRuntimeErasedMaterialsFinalForTestV1(t *testing.T, fixture *runtimeOriginalReservedReportHistoryFixtureV1) {
	t.Helper()
	ctx := context.Background()
	core := fixture.core
	snapshot, err := persistencefs.NewStartupSnapshotReader(core.roots).CaptureManagedSnapshotV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("erased material signed Final"))
	baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, digest, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	root, err := persistencefs.FreezeRootAuthority(core.roots)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := persistencefs.FreezeJournalNamespaceAuthorityForRoots(core.roots)
	if err != nil {
		t.Fatal(err)
	}
	first, last := filepath.Join("private", "a-material-cut.bin"), filepath.Join("private", "z-material-cut.bin")
	builder := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(core.roots, root, journal, runtimeReportRestartPreservationV1{})
	prepared, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
		for name, entry := range fixture.publication.original["report-publication"] {
			if entry.Directory {
				continue
			}
			if err := os.Remove(filepath.Join(stage.DataDir, "private", "report-publication", filepath.FromSlash(name))); err != nil {
				return err
			}
		}
		if err := os.WriteFile(filepath.Join(stage.DataDir, first), []byte("applied signed prefix"), 0600); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(stage.DataDir, last), []byte("unapplied signed Final"), 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	cancelled, cancel := context.WithCancel(ctx)
	defer cancel()
	cut := cancelAfterTerminalDispositionContextV1{Context: cancelled, cancel: cancel, dispositionPath: filepath.Join(core.roots.DataDir, first), intentPath: filepath.Join(core.roots.DataDir, last)}
	if err := prepared.Apply(cut); !errors.Is(err, context.Canceled) {
		t.Fatalf("material Final did not reach signed interruption: %v", err)
	}
	if _, err := os.Lstat(cut.dispositionPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(cut.intentPath); !os.IsNotExist(err) {
		t.Fatal("signed Final did not retain an unapplied suffix")
	}
	for name, entry := range fixture.publication.original["report-publication"] {
		if entry.Directory {
			continue
		}
		body, err := os.ReadFile(filepath.Join(core.roots.DataDir, "private", "report-publication", filepath.FromSlash(name)))
		if err != nil || !reflect.DeepEqual(body, entry.Body) {
			t.Fatal("cut lost physical Original material")
		}
	}
}

// The new material fixture uses the actual producer hash shape and keyed
// pending payload. This replaces only its seed receipt before any attempt.
// It does not normalize or regenerate an existing product history.
func runtimeMaterialReportStageForTestV1(t *testing.T, core *runtimeChildIdentityStartupV1, authority authorityport.Authority, frozen domainsecurity.TurnSecurityContext, grant domainsecurity.ExecutionGrant, original domainpending.PendingWorkReceiptV1, candidate domainpublication.PublicationReceiptV1) (domainpending.PendingWorkReceiptV1, string) {
	t.Helper()
	body, err := json.Marshal(struct {
		ContextDigest       string `json:"contextDigest"`
		GrantID             string `json:"grantId"`
		ReportVariant       string `json:"reportVariant"`
		ClaimLedgerDigest   string `json:"claimLedgerDigest"`
		PIIProjectionDigest string `json:"piiProjectionDigest"`
		InspectionDigest    string `json:"inspectionDigest"`
		ReportSHA256        string `json:"reportSha256"`
		TargetDigest        string `json:"targetDigest"`
	}{frozen.ContextDigest, grant.GrantID, candidate.ReportVariant, candidate.ClaimLedgerDigest, candidate.PIIProjectionDigest, candidate.RenderInspectionDigest, candidate.ReportSHA256, candidate.TargetIdentityDigest})
	if err != nil {
		t.Fatal(err)
	}
	inputHash := domainsecurity.SHA256Hex(body)
	payload, err := json.Marshal(map[string]any{"schemaVersion": 1, "kind": domainpending.KindReportStage, "contextDigest": frozen.ContextDigest, "datasetSnapshotId": frozen.DatasetSnapshotID, "grantId": grant.GrantID, "toolCallId": grant.ToolCallID, "toolName": grant.ToolName, "argsHash": grant.ArgsHash, "stageInputHash": inputHash})
	if err != nil {
		t.Fatal(err)
	}
	payloadHash, err := pendingapp.NewService(authority, nil, nil).KeyedPayloadHash(context.Background(), "report_stage.semantic_request", payload)
	if err != nil {
		t.Fatal(err)
	}
	issued, err := time.Parse(time.RFC3339Nano, original.IssuedAt)
	if err != nil {
		t.Fatal(err)
	}
	expires, err := time.Parse(time.RFC3339Nano, original.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := domainpending.NewPendingWorkReceiptV1(domainpending.ReceiptInputV1{Kind: domainpending.KindReportStage, SecurityContext: frozen, GrantRegistrySequence: original.GrantRegistrySequence, GrantRegistryDigest: original.GrantRegistryDigest, GrantMembers: original.GrantMembers, PayloadHash: payloadHash, RouteHash: domainsecurity.SHA256Hex([]byte("analytix/pending-work-route/report-stage/v1")), IssuedAt: issued, ExpiresAt: expires, AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey()}, func(body []byte) ([]byte, error) { return authority.Sign(context.Background(), body) })
	if err != nil {
		t.Fatal(err)
	}
	body, err = domainpending.PendingWorkReceiptV1Bytes(receipt)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(core.roots.DataDir, "private", "pending-work", "receipts")
	target := filepath.Join(root, receipt.WorkID[:2], receipt.WorkID+".json")
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, body, 0600); err != nil {
		t.Fatal(err)
	}
	if receipt.WorkID == original.WorkID {
		t.Fatal("synthetic seed unexpectedly had the actual keyed stage identity")
	}
	if err := os.Remove(filepath.Join(root, original.WorkID[:2], original.WorkID+".json")); err != nil {
		t.Fatal(err)
	}
	return receipt, inputHash
}
