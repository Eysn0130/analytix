package reportpublication

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	testsecurity "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestReportPublicationPreparedRecoveryValidatesExactMaterialGraph(t *testing.T) {
	root := t.TempDir() + "/report-publication"
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	stores, err := NewStores(root, access)
	if err != nil {
		t.Fatal(err)
	}
	fixture := newPublicationRecoveryFixture(t)
	fixture.install(t, stores, fixture.artifact)
	if created, err := stores.Decisions.CreateExclusive(context.Background(), fixture.decision); err != nil || created {
		t.Fatalf("delivery decision exact replay was not idempotent: created=%v err=%v", created, err)
	}
	storedDecision, err := stores.Decisions.Resolve(context.Background(), fixture.decision.DecisionID)
	if err != nil || storedDecision.RecordDigest != fixture.decision.RecordDigest {
		t.Fatalf("delivery decision readback mismatch: %#v err=%v", storedDecision, err)
	}
	decisionBody, err := domainpublication.ReportDeliveryDecisionV1Bytes(storedDecision)
	if err != nil || bytes.Contains(decisionBody, []byte("6222020202020202020")) {
		t.Fatalf("delivery decision leaked raw bank account material: %s err=%v", decisionBody, err)
	}
	storedCompletion, err := stores.StageCompletions.Resolve(context.Background(), fixture.stageCompletion.CompletionID)
	if err != nil || storedCompletion.RecordDigest != fixture.stageCompletion.RecordDigest {
		t.Fatalf("report stage completion readback mismatch: %#v err=%v", storedCompletion, err)
	}
	completionBody, err := domainpublication.ReportStageCompletionV1Bytes(storedCompletion)
	if err != nil || bytes.Contains(completionBody, fixture.artifact) || bytes.Contains(completionBody, []byte("6222020202020202020")) {
		t.Fatalf("report stage completion leaked artifact or account material: %s err=%v", completionBody, err)
	}
	storedOutcome, err := stores.DeliveryOutcomes.ResolveOutcome(context.Background(), fixture.deliveryProjection.DeliveryID)
	if err != nil || storedOutcome.Kind != domainpublication.ReportDeliveryOutcomeProjectedV1 ||
		storedOutcome.Projection.RecordDigest != fixture.deliveryProjection.RecordDigest ||
		domainpublication.ValidateReportDeliveryOutcomeCompletionV1(storedOutcome, storedCompletion) != nil {
		t.Fatalf("report delivery outcome readback mismatch: %#v err=%v", storedOutcome, err)
	}
	deliveryBody, err := domainpublication.ReportDeliveryOutcomeV1Bytes(storedOutcome)
	if err != nil || bytes.Contains(deliveryBody, fixture.artifact) || bytes.Contains(deliveryBody, []byte("6222020202020202020")) ||
		bytes.Contains(deliveryBody, []byte(fixture.decision.Context.WorkspaceRealPath)) {
		t.Fatalf("report delivery projection leaked artifact, account, or workspace material: %s err=%v", deliveryBody, err)
	}
	if has, err := stores.HasRecords(context.Background()); err != nil || !has {
		t.Fatalf("report publication inventory was not detected: has=%v err=%v", has, err)
	}
	prepared, err := PrepareRecoveryV1(context.Background(), root, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.ValidateSemantics(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Revalidate(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestReportDeliveryDecisionStoreConcurrentExactReplayCreatesOnce(t *testing.T) {
	root := t.TempDir() + "/delivery-decisions"
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := newDeliveryDecisionStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.cas.Close() })
	decision := newPublicationRecoveryFixture(t).decision
	const contenders = 16
	var wait sync.WaitGroup
	results := make(chan bool, contenders)
	errorsSeen := make(chan error, contenders)
	for range contenders {
		wait.Add(1)
		go func() {
			defer wait.Done()
			created, createErr := store.CreateExclusive(context.Background(), decision)
			results <- created
			errorsSeen <- createErr
		}()
	}
	wait.Wait()
	close(results)
	close(errorsSeen)
	createdCount := 0
	for created := range results {
		if created {
			createdCount++
		}
	}
	for createErr := range errorsSeen {
		if createErr != nil {
			t.Fatal(createErr)
		}
	}
	if createdCount != 1 {
		t.Fatalf("delivery decision concurrent create count = %d, want 1", createdCount)
	}
}

func TestReportGrantSettlementStoreConcurrentExactReplayCreatesOnce(t *testing.T) {
	root := t.TempDir() + "/grant-settlements"
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := newReportGrantSettlementStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.cas.Close() })
	settlement := newPublicationRecoveryFixture(t).grantSettlement
	const contenders = 16
	var wait sync.WaitGroup
	results := make(chan bool, contenders)
	errorsSeen := make(chan error, contenders)
	for range contenders {
		wait.Add(1)
		go func() {
			defer wait.Done()
			created, createErr := store.CreateExclusive(context.Background(), settlement)
			results <- created
			errorsSeen <- createErr
		}()
	}
	wait.Wait()
	close(results)
	close(errorsSeen)
	createdCount := 0
	for created := range results {
		if created {
			createdCount++
		}
	}
	for createErr := range errorsSeen {
		if createErr != nil {
			t.Fatal(createErr)
		}
	}
	if createdCount != 1 {
		t.Fatalf("report grant settlement concurrent create count = %d, want 1", createdCount)
	}
	resolved, err := store.Resolve(context.Background(), settlement.SettlementID)
	if err != nil || resolved.RecordDigest != settlement.RecordDigest {
		t.Fatalf("report grant settlement exact readback failed: %#v err=%v", resolved, err)
	}
	visited := 0
	if err := store.VisitGrantSettlements(context.Background(), func(current domainpublication.ReportGrantSettlementV1) error {
		visited++
		if current.RecordDigest != settlement.RecordDigest {
			t.Fatalf("report grant settlement inventory changed exact record: %#v", current)
		}
		return nil
	}); err != nil || visited != 1 {
		t.Fatalf("report grant settlement inventory count = %d err=%v", visited, err)
	}
}

func TestReportGrantSettlementStoreRejectsStableIdentityCollision(t *testing.T) {
	root := t.TempDir() + "/grant-settlements"
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := newReportGrantSettlementStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.cas.Close() })
	fixture := newPublicationRecoveryFixture(t)
	if created, err := store.CreateExclusive(context.Background(), fixture.grantSettlement); err != nil || !created {
		t.Fatalf("create baseline report grant settlement: created=%v err=%v", created, err)
	}
	conflictInput := fixture.grantSettlementInput
	conflictInput.ResultItemDigest = domainsecurity.SHA256Hex([]byte("different-private-tool-result"))
	conflict, err := domainpublication.NewReportGrantSettlementV1(conflictInput, func(message []byte) ([]byte, error) {
		return ed25519.Sign(fixture.authorityKey, message), nil
	})
	if err != nil || conflict.SettlementID != fixture.grantSettlement.SettlementID ||
		conflict.RecordDigest == fixture.grantSettlement.RecordDigest {
		t.Fatalf("construct stable-identity collision: conflict=%#v err=%v", conflict, err)
	}
	if created, err := store.CreateExclusive(context.Background(), conflict); err == nil || created {
		t.Fatalf("conflicting report grant settlement replaced stable identity: created=%v err=%v", created, err)
	}
	resolved, err := store.Resolve(context.Background(), fixture.grantSettlement.SettlementID)
	if err != nil || resolved.RecordDigest != fixture.grantSettlement.RecordDigest {
		t.Fatalf("collision changed the admitted report grant settlement: %#v err=%v", resolved, err)
	}
}

func TestReportStageCompletionStoreConcurrentExactReplayCreatesOnce(t *testing.T) {
	root := t.TempDir() + "/stage-completions"
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := newReportStageCompletionStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.cas.Close() })
	completion := newPublicationRecoveryFixture(t).stageCompletion
	const contenders = 16
	var wait sync.WaitGroup
	results := make(chan bool, contenders)
	errorsSeen := make(chan error, contenders)
	for range contenders {
		wait.Add(1)
		go func() {
			defer wait.Done()
			created, createErr := store.CreateExclusive(context.Background(), completion)
			results <- created
			errorsSeen <- createErr
		}()
	}
	wait.Wait()
	close(results)
	close(errorsSeen)
	createdCount := 0
	for created := range results {
		if created {
			createdCount++
		}
	}
	for createErr := range errorsSeen {
		if createErr != nil {
			t.Fatal(createErr)
		}
	}
	if createdCount != 1 {
		t.Fatalf("report stage completion concurrent create count = %d, want 1", createdCount)
	}
	resolved, err := store.Resolve(context.Background(), completion.CompletionID)
	if err != nil || resolved.RecordDigest != completion.RecordDigest {
		t.Fatalf("report stage completion exact readback failed: %#v err=%v", resolved, err)
	}
	visited := 0
	if err := store.VisitStageCompletions(context.Background(), func(current domainpublication.ReportStageCompletionV1) error {
		visited++
		if current.RecordDigest != completion.RecordDigest {
			t.Fatalf("report stage completion inventory changed exact record: %#v", current)
		}
		return nil
	}); err != nil || visited != 1 {
		t.Fatalf("report stage completion inventory count = %d err=%v", visited, err)
	}
}

func TestReportStageCompletionStoreRejectsStableIdentityCollision(t *testing.T) {
	root := t.TempDir() + "/stage-completions"
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := newReportStageCompletionStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.cas.Close() })
	fixture := newPublicationRecoveryFixture(t)
	if created, err := store.CreateExclusive(context.Background(), fixture.stageCompletion); err != nil || !created {
		t.Fatalf("create baseline report stage completion: created=%v err=%v", created, err)
	}
	otherSettlementInput := fixture.grantSettlementInput
	otherSettlementInput.ResultItemDigest = domainsecurity.SHA256Hex([]byte("different-completed-private-tool-result"))
	otherSettlement, err := domainpublication.NewReportGrantSettlementV1(otherSettlementInput, func(message []byte) ([]byte, error) {
		return ed25519.Sign(fixture.authorityKey, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	conflictInput := fixture.stageCompletionInput
	conflictInput.GrantSettlement = otherSettlement
	conflict, err := domainpublication.NewReportStageCompletionV1(conflictInput, func(message []byte) ([]byte, error) {
		return ed25519.Sign(fixture.authorityKey, message), nil
	})
	if err != nil || conflict.CompletionID != fixture.stageCompletion.CompletionID ||
		conflict.RecordDigest == fixture.stageCompletion.RecordDigest {
		t.Fatalf("construct report stage completion stable-identity collision: conflict=%#v err=%v", conflict, err)
	}
	if created, err := store.CreateExclusive(context.Background(), conflict); err == nil || created {
		t.Fatalf("conflicting report stage completion replaced stable identity: created=%v err=%v", created, err)
	}
	resolved, err := store.Resolve(context.Background(), fixture.stageCompletion.CompletionID)
	if err != nil || resolved.RecordDigest != fixture.stageCompletion.RecordDigest {
		t.Fatalf("completion collision changed admitted record: %#v err=%v", resolved, err)
	}
}

func TestReportDeliveryOutcomeStoreConcurrentProjectionAndRejectionHaveOneWinner(t *testing.T) {
	root := t.TempDir() + "/delivery-projections"
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := newReportDeliveryOutcomeStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.cas.Close() })
	fixture := newPublicationRecoveryFixture(t)
	rejection, rejectedOutcome := fixture.evidenceChangedRejection(t)
	if rejection.DeliveryID != fixture.deliveryProjection.DeliveryID {
		t.Fatalf("opposite delivery outcomes did not share one stable identity: projected=%s rejected=%s",
			fixture.deliveryProjection.DeliveryID, rejection.DeliveryID)
	}
	const contenders = 16
	var wait sync.WaitGroup
	errorsSeen := make(chan error, contenders)
	winners := make(chan domainpublication.ReportDeliveryOutcomeV1, contenders)
	for index := range contenders {
		wait.Add(1)
		candidate := rejectedOutcome
		if index%2 == 0 {
			candidate = fixture.deliveryOutcome
		}
		go func(candidate domainpublication.ReportDeliveryOutcomeV1) {
			defer wait.Done()
			winner, _, createErr := store.CreateOutcomeExclusive(context.Background(), fixture.stageCompletion, candidate)
			winners <- winner
			errorsSeen <- createErr
		}(candidate)
	}
	wait.Wait()
	close(errorsSeen)
	close(winners)
	for createErr := range errorsSeen {
		if createErr != nil {
			t.Fatal(createErr)
		}
	}
	winnerDigest := ""
	for winner := range winners {
		if err := domainpublication.ValidateReportDeliveryOutcomeCompletionV1(winner, fixture.stageCompletion); err != nil {
			t.Fatal(err)
		}
		if winnerDigest == "" {
			winnerDigest = domainpublication.ReportDeliveryOutcomeRecordDigest(winner)
		} else if domainpublication.ReportDeliveryOutcomeRecordDigest(winner) != winnerDigest {
			t.Fatalf("shared delivery CAS returned multiple terminal winners: first=%s next=%s", winnerDigest,
				domainpublication.ReportDeliveryOutcomeRecordDigest(winner))
		}
	}
	visited := 0
	if err := store.VisitDeliveryOutcomes(context.Background(), func(current domainpublication.ReportDeliveryOutcomeV1) error {
		visited++
		if domainpublication.ReportDeliveryOutcomeRecordDigest(current) != winnerDigest {
			t.Fatalf("delivery outcome inventory changed the first-writer winner: %#v", current)
		}
		return nil
	}); err != nil || visited != 1 {
		t.Fatalf("delivery outcome inventory count = %d err=%v", visited, err)
	}

	otherSettlementInput := fixture.grantSettlementInput
	otherSettlementInput.ResultItemDigest = domainsecurity.SHA256Hex([]byte("different-delivery-private-tool-result"))
	otherSettlement, err := domainpublication.NewReportGrantSettlementV1(otherSettlementInput, func(message []byte) ([]byte, error) {
		return ed25519.Sign(fixture.authorityKey, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	otherCompletionInput := fixture.stageCompletionInput
	otherCompletionInput.GrantSettlement = otherSettlement
	otherCompletion, err := domainpublication.NewReportStageCompletionV1(otherCompletionInput, func(message []byte) ([]byte, error) {
		return ed25519.Sign(fixture.authorityKey, message), nil
	})
	if err != nil || otherCompletion.CompletionID != fixture.stageCompletion.CompletionID {
		t.Fatalf("construct completion collision: %#v err=%v", otherCompletion, err)
	}
	conflictInput := fixture.deliveryProjectionInput
	conflictInput.GrantSettlement = otherSettlement
	conflictInput.StageCompletion = otherCompletion
	conflict, err := domainpublication.NewReportDeliveryProjectionV1(conflictInput, func(message []byte) ([]byte, error) {
		return ed25519.Sign(fixture.authorityKey, message), nil
	})
	if err != nil || conflict.DeliveryID != fixture.deliveryProjection.DeliveryID ||
		conflict.RecordDigest == fixture.deliveryProjection.RecordDigest {
		t.Fatalf("construct delivery stable-identity collision: %#v err=%v", conflict, err)
	}
	conflictOutcome, err := domainpublication.ProjectedReportDeliveryOutcomeV1(conflict)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.CreateOutcomeExclusive(context.Background(), otherCompletion, conflictOutcome); err == nil {
		t.Fatal("conflicting completion reused the delivery outcome stable identity")
	}
	resolved, err := store.ResolveOutcome(context.Background(), fixture.deliveryProjection.DeliveryID)
	if err != nil || domainpublication.ReportDeliveryOutcomeRecordDigest(resolved) != winnerDigest {
		t.Fatalf("delivery collision changed the original projection: %#v err=%v", resolved, err)
	}
}

func TestReportDeliveryRejectionIsCanonicalPrivateAndPreparedRecoveryAcceptsIt(t *testing.T) {
	root := t.TempDir() + "/report-publication"
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	stores, err := NewStores(root, access)
	if err != nil {
		t.Fatal(err)
	}
	fixture := newPublicationRecoveryFixture(t)
	rejection, outcome := fixture.evidenceChangedRejection(t)
	fixture.deliveryOutcome = outcome
	body, err := domainpublication.ReportDeliveryOutcomeV1Bytes(outcome)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"6222020202020202020", "/workspace/publication-recovery", "case-publication-recovery",
		string(fixture.artifact), "database timeout with raw customer row",
	} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("private delivery rejection leaked forbidden content %q: %s", forbidden, body)
		}
	}
	parsed, err := domainpublication.ParseReportDeliveryOutcomeV1(body)
	if err != nil || parsed.Kind != domainpublication.ReportDeliveryOutcomeRejectedV1 ||
		parsed.Rejection == nil || parsed.Rejection.RecordDigest != rejection.RecordDigest {
		t.Fatalf("delivery rejection canonical round trip failed: parsed=%#v err=%v", parsed, err)
	}
	unknown := append(append([]byte(nil), body[:len(body)-1]...), []byte(`,"unknown":true}`)...)
	if _, err := domainpublication.ParseReportDeliveryOutcomeV1(unknown); err == nil {
		t.Fatal("delivery outcome accepted an unknown wire property")
	}
	invalidReason := fixture.evidenceChangedRejectionInput()
	invalidReason.ReasonCode = domainpublication.ReportDeliveryRejectionReasonV1("stale_context")
	if _, err := domainpublication.NewReportDeliveryRejectionV1(invalidReason, func(message []byte) ([]byte, error) {
		return ed25519.Sign(fixture.authorityKey, message), nil
	}); err == nil {
		t.Fatal("delivery rejection accepted a reason without a typed host proof authority")
	}
	fixture.install(t, stores, fixture.artifact)
	prepared, err := PrepareRecoveryV1(context.Background(), root, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.ValidateSemantics(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Revalidate(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestReportDeliveryDecisionRejectsChangedWitnessAndContext(t *testing.T) {
	fixture := newPublicationRecoveryFixture(t)
	otherContext, err := testsecurity.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: fixture.decision.Context.ThreadID, TurnID: fixture.decision.Context.TurnID,
		WorkspaceRealPath: fixture.decision.Context.WorkspaceRealPath,
		CaseID:            "case-publication-recovery-other",
		CaseBindingHash:   domainsecurity.SHA256Hex([]byte("publication-recovery-other-binding")),
		ContextEpoch:      fixture.decision.Context.ContextEpoch + 1,
		IssuedAt:          time.Date(2026, 7, 16, 13, 1, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*domainpublication.ReportDeliveryDecisionInputV1)
	}{
		{name: "settlement", mutate: func(input *domainpublication.ReportDeliveryDecisionInputV1) {
			input.AuthorityAdvanceSettlementDigest = domainsecurity.SHA256Hex([]byte("different-settlement"))
		}},
		{name: "witnessed bundle", mutate: func(input *domainpublication.ReportDeliveryDecisionInputV1) {
			input.WitnessedEvidenceAuthorityBundleDigest = domainsecurity.SHA256Hex([]byte("different-bundle"))
		}},
		{name: "registry index", mutate: func(input *domainpublication.ReportDeliveryDecisionInputV1) {
			input.WitnessedEvidenceRegistryIndexDigest = domainsecurity.SHA256Hex([]byte("different-registry-index"))
		}},
		{name: "registry sequence", mutate: func(input *domainpublication.ReportDeliveryDecisionInputV1) {
			input.EvidenceRegistrySequence++
		}},
		{name: "registry state", mutate: func(input *domainpublication.ReportDeliveryDecisionInputV1) {
			input.EvidenceRegistryStateDigest = domainsecurity.SHA256Hex([]byte("different-registry-state"))
		}},
		{name: "current context", mutate: func(input *domainpublication.ReportDeliveryDecisionInputV1) {
			input.Context = otherContext
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := fixture.decisionInput
			test.mutate(&input)
			if _, err := domainpublication.NewReportDeliveryDecisionV1(input, func(message []byte) ([]byte, error) {
				return ed25519.Sign(fixture.authorityKey, message), nil
			}); err == nil {
				t.Fatal("changed host witness material created a delivery decision")
			}
		})
	}
}

func TestReportDeliveryDecisionDurableGraphRejectsMixedValidMembers(t *testing.T) {
	fixture := newPublicationRecoveryFixture(t)
	other := newPublicationRecoveryFixtureWithArtifact(t, []byte("different deterministic verified no-hit report"))
	otherLedger, err := domainpublication.NewClaimLedgerV1(domainpublication.ClaimLedgerInputV1{
		Context:            fixture.decision.Context,
		EvidenceReceiptIDs: []string{"evr_" + domainsecurity.SHA256Hex([]byte("other-ledger-evidence"))},
		CreatedAt:          time.Date(2026, 7, 16, 13, 2, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	validate := func(
		attempt domainpublication.PublicationAttemptV1,
		candidate domainpublication.PublicationReceiptV1,
		index domainpublication.PublicationIndexV1,
		selection domainpublication.PublicationCommitSelectionV1,
		commit domainpublication.PublicationCommitReceiptV1,
		ledger domainpublication.ClaimLedgerV1,
		projection domainpublication.PIIProjectionV1,
		inspection domainpublication.RenderInspectionV1,
	) error {
		return domainpublication.ValidateReportDeliveryDecisionDurableGraphV1(
			fixture.decision, attempt, candidate, index, selection, commit, ledger, projection, inspection,
		)
	}
	if err := validate(fixture.attempt, fixture.receipt, fixture.index, fixture.selection, fixture.commit, fixture.ledger, fixture.projection, fixture.inspection); err != nil {
		t.Fatalf("baseline delivery decision durable graph failed: %v", err)
	}
	tests := []struct {
		name string
		err  error
	}{
		{name: "attempt", err: validate(other.attempt, fixture.receipt, fixture.index, fixture.selection, fixture.commit, fixture.ledger, fixture.projection, fixture.inspection)},
		{name: "candidate", err: validate(fixture.attempt, other.receipt, fixture.index, fixture.selection, fixture.commit, fixture.ledger, fixture.projection, fixture.inspection)},
		{name: "index", err: validate(fixture.attempt, fixture.receipt, other.index, fixture.selection, fixture.commit, fixture.ledger, fixture.projection, fixture.inspection)},
		{name: "selection", err: validate(fixture.attempt, fixture.receipt, fixture.index, other.selection, fixture.commit, fixture.ledger, fixture.projection, fixture.inspection)},
		{name: "commit", err: validate(fixture.attempt, fixture.receipt, fixture.index, fixture.selection, other.commit, fixture.ledger, fixture.projection, fixture.inspection)},
		{name: "ledger", err: validate(fixture.attempt, fixture.receipt, fixture.index, fixture.selection, fixture.commit, otherLedger, fixture.projection, fixture.inspection)},
		{name: "projection", err: validate(fixture.attempt, fixture.receipt, fixture.index, fixture.selection, fixture.commit, fixture.ledger, other.projection, fixture.inspection)},
		{name: "inspection", err: validate(fixture.attempt, fixture.receipt, fixture.index, fixture.selection, fixture.commit, fixture.ledger, fixture.projection, other.inspection)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.err == nil {
				t.Fatal("mixed valid durable member was accepted")
			}
		})
	}
}

func TestReportPublicationPreparedRecoveryRejectsArtifactAndIndexMismatches(t *testing.T) {
	t.Run("artifact", func(t *testing.T) {
		root := t.TempDir() + "/report-publication"
		access, err := privatecastest.NewAccessAuthority(root)
		if err != nil {
			t.Fatal(err)
		}
		stores, err := NewStores(root, access)
		if err != nil {
			t.Fatal(err)
		}
		fixture := newPublicationRecoveryFixture(t)
		fixture.install(t, stores, []byte("different protected artifact"))
		prepared, err := PrepareRecoveryV1(context.Background(), root, access)
		if err != nil {
			t.Fatal(err)
		}
		if err := prepared.ValidateSemantics(context.Background()); err == nil {
			t.Fatal("report recovery accepted receipt/artifact hash mismatch")
		}
	})
	t.Run("orphan index", func(t *testing.T) {
		root := t.TempDir() + "/report-publication"
		access, err := privatecastest.NewAccessAuthority(root)
		if err != nil {
			t.Fatal(err)
		}
		stores, err := NewStores(root, access)
		if err != nil {
			t.Fatal(err)
		}
		fixture := newPublicationRecoveryFixture(t)
		if err := stores.Indexes.PutIfAbsent(context.Background(), fixture.index); err != nil {
			t.Fatal(err)
		}
		prepared, err := PrepareRecoveryV1(context.Background(), root, access)
		if err != nil {
			t.Fatal(err)
		}
		if err := prepared.ValidateSemantics(context.Background()); err == nil {
			t.Fatal("report recovery accepted an index without its exact receipt")
		}
	})
}

func TestReportPublicationBinaryArtifactSurvivesStrictSnapshotAndPreparedRecovery(t *testing.T) {
	base := t.TempDir()
	roots, err := persistencefs.ResolveRootSet(
		filepath.Join(base, "data"),
		filepath.Join(base, "durable"),
	)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(roots.DataDir, "private", "report-publication")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	stores, err := NewStores(root, access)
	if err != nil {
		t.Fatal(err)
	}
	artifact := []byte("%PDF-1.7\n%\xe2\xe3\xcf\xd3\ncontrolled report bytes\n%%EOF")
	fixture := newPublicationRecoveryFixtureWithArtifact(t, artifact)
	fixture.install(t, stores, fixture.artifact)

	snapshot, err := persistencefs.CaptureStrict(roots)
	if err != nil || snapshot.FileCount == 0 {
		t.Fatalf("strict snapshot rejected a valid binary publication artifact: files=%d err=%v", snapshot.FileCount, err)
	}
	prepared, err := PrepareRecoveryV1(context.Background(), root, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.ValidateSemantics(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Revalidate(context.Background()); err != nil {
		t.Fatal(err)
	}
}

type publicationRecoveryFixture struct {
	decisionInput           domainpublication.ReportDeliveryDecisionInputV1
	grantSettlementInput    domainpublication.ReportGrantSettlementInputV1
	stageCompletionInput    domainpublication.ReportStageCompletionInputV1
	deliveryProjectionInput domainpublication.ReportDeliveryProjectionInputV1
	authorityKey            ed25519.PrivateKey
	receipt                 domainpublication.PublicationReceiptV1
	index                   domainpublication.PublicationIndexV1
	stage                   domainpendingwork.PendingWorkReceiptV1
	attempt                 domainpublication.PublicationAttemptV1
	selection               domainpublication.PublicationCommitSelectionV1
	commit                  domainpublication.PublicationCommitReceiptV1
	decision                domainpublication.ReportDeliveryDecisionV1
	grantSettlement         domainpublication.ReportGrantSettlementV1
	stageDisposition        domainpendingwork.PendingWorkDispositionV1
	stageCompletion         domainpublication.ReportStageCompletionV1
	deliveryProjection      domainpublication.ReportDeliveryProjectionV1
	deliveryOutcome         domainpublication.ReportDeliveryOutcomeV1
	ledger                  domainpublication.ClaimLedgerV1
	projection              domainpublication.PIIProjectionV1
	inspection              domainpublication.RenderInspectionV1
	target                  string
	artifact                []byte
}

func (fixture publicationRecoveryFixture) evidenceChangedRejectionInput() domainpublication.ReportDeliveryRejectionInputV1 {
	return domainpublication.ReportDeliveryRejectionInputV1{
		Decision: fixture.decision, GrantSettlement: fixture.grantSettlement,
		StageReceipt: fixture.stage, StageDisposition: fixture.stageDisposition, StageCompletion: fixture.stageCompletion,
		ReasonCode:                    domainpublication.ReportDeliveryRejectionEvidenceChangedV1,
		WitnessObservationDigest:      domainsecurity.SHA256Hex([]byte("publication-recovery-rejection-observation")),
		EvidenceAuthorityBundleDigest: domainsecurity.SHA256Hex([]byte("publication-recovery-rejection-bundle")),
		EvidenceRegistryIndexDigest:   domainsecurity.SHA256Hex([]byte("publication-recovery-rejection-registry-index")),
		EvidenceRegistrySequence:      fixture.decision.EvidenceRegistrySequence + 1,
		EvidenceRegistryStateDigest:   domainsecurity.SHA256Hex([]byte("publication-recovery-rejection-registry-state")),
		AuthorityKeyID:                fixture.decision.AuthorityKeyID,
		AuthorityPublicKey:            fixture.authorityKey.Public().(ed25519.PublicKey),
	}
}

func (fixture publicationRecoveryFixture) evidenceChangedRejection(
	t *testing.T,
) (domainpublication.ReportDeliveryRejectionV1, domainpublication.ReportDeliveryOutcomeV1) {
	t.Helper()
	rejection, err := domainpublication.NewReportDeliveryRejectionV1(
		fixture.evidenceChangedRejectionInput(),
		func(message []byte) ([]byte, error) { return ed25519.Sign(fixture.authorityKey, message), nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := domainpublication.RejectedReportDeliveryOutcomeV1(rejection)
	if err != nil {
		t.Fatal(err)
	}
	return rejection, outcome
}

func newPublicationRecoveryFixture(t *testing.T) publicationRecoveryFixture {
	return newPublicationRecoveryFixtureWithArtifact(
		t,
		[]byte("deterministic verified no-hit report"),
	)
}

func newPublicationRecoveryFixtureWithArtifact(t *testing.T, artifact []byte) publicationRecoveryFixture {
	t.Helper()
	artifact = append([]byte(nil), artifact...)
	if len(artifact) == 0 {
		t.Fatal("publication recovery artifact is empty")
	}
	now := time.Date(2026, 7, 16, 13, 0, 0, 0, time.UTC)
	securityContext, err := testsecurity.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-publication-recovery", TurnID: "turn-publication-recovery", WorkspaceRealPath: "/workspace/publication-recovery",
		CaseID: "case-publication-recovery", CaseBindingHash: domainsecurity.SHA256Hex([]byte("publication-recovery-binding")),
		ContextEpoch: 6, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	evidenceID := "evr_" + domainsecurity.SHA256Hex([]byte("publication-recovery-no-hit"))
	ledger, err := domainpublication.NewClaimLedgerV1(domainpublication.ClaimLedgerInputV1{
		Context: securityContext, EvidenceReceiptIDs: []string{evidenceID}, Claims: nil, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	reportSHA := domainsecurity.SHA256Hex(artifact)
	projection, err := domainpublication.NewPIIProjectionV1(domainpublication.PIIProjectionInputV1{
		ProjectionClass: domainpublication.PIIProjectionOrdinaryMasked,
		RulesetHash:     domainsecurity.SHA256Hex([]byte("publication-recovery-rules")), ProjectedContentSHA256: reportSHA,
	})
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := domainpublication.NewRenderInspectionV1(domainpublication.RenderInspectionInputV1{
		Renderer: "analytix-host", RendererVersion: "1.0.0", ReportSHA256: reportSHA, ReportByteLength: uint64(len(artifact)),
		MediaType: "application/pdf", Passed: true, IssueCodes: []string{}, InspectedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x36}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	keyID := domainsecurity.SHA256Hex(publicKey)
	witnessPrivate := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x46}, ed25519.SeedSize))
	witnessPublic := witnessPrivate.Public().(ed25519.PublicKey)
	witnessKeyID := domainsecurity.SHA256Hex(witnessPublic)
	installationID := domainsecurity.SHA256Hex([]byte("publication-recovery-installation"))
	enrollmentID := domainsecurity.SHA256Hex([]byte("publication-recovery-enrollment"))
	registryIndexDigest := domainsecurity.SHA256Hex([]byte("publication-recovery-registry-index"))
	previousBundle, err := domainevidence.NewEvidenceAuthorityBundleV1(domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Generation: 1,
		MutationID:                  domainsecurity.SHA256Hex([]byte("publication-recovery-previous-bundle")),
		DatasetSnapshotIndexDigest:  domainsecurity.SHA256Hex([]byte("publication-recovery-dataset-index")),
		DatasetSnapshotCount:        1,
		EvidenceRegistryIndexDigest: registryIndexDigest,
		EvidenceRegistryCount:       1,
		PublicationIndexDigest:      domainpublication.PublicationIndexGenesisDigestV1(),
		PublicationCount:            0,
		AuthorityKeyID:              keyID,
		AuthorityPublicKey:          publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	target := domainsecurity.SHA256Hex([]byte("publication-recovery-target"))
	receipt, err := domainpublication.NewPublicationReceiptV1(domainpublication.PublicationReceiptInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Context: securityContext,
		ReportVariant:                 domainpublication.VerifiedNoHitReport,
		EvidenceAuthorityBundleDigest: previousBundle.RecordDigest,
		EvidenceRegistryIndexDigest:   registryIndexDigest, EvidenceRegistryCount: 1,
		EvidenceRegistrySequence: 1, EvidenceRegistryStateDigest: domainsecurity.SHA256Hex([]byte("publication-recovery-registry-state")),
		ClaimLedger: ledger, ReportSHA256: reportSHA, ReportByteLength: uint64(len(artifact)), MediaType: "application/pdf",
		PIIProjection: projection, RenderInspection: inspection, Publisher: "analytix-host", PublisherVersion: "1.0.0",
		TargetIdentityDigest: target, IssuedAt: now, AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	toolCallID, err := domainmodel.NewHostToolCallIDV1(bytes.Repeat([]byte{0x72}, domainmodel.HostToolCallIDEntropyBytesV1))
	if err != nil {
		t.Fatal(err)
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider", ServerIdentity: "host:builtin",
		ToolName: "stage_case_report", ToolCallID: toolCallID,
		ArgsHash:   domainsecurity.CanonicalJSONHash([]byte(`{"report":"case"}`)),
		SchemaHash: domainsecurity.SHA256Hex([]byte("publication-recovery-report-schema")),
		ScopeHash:  domainsecurity.SHA256Hex([]byte("publication-recovery-report-scope")),
		ReadOnly:   false, ApprovalState: "approved", IssuedAt: now, ExpiresAt: now.Add(time.Hour),
	})
	activeRegistry, err := domainsecurity.RegisterExecutionGrant(
		domainsecurity.NewExecutionGrantRegistry(securityContext.ThreadID), securityContext.ThreadID, grant, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	activeEntry, found := domainsecurity.ExecutionGrantRegistryEntryByID(activeRegistry, grant.GrantID)
	if !found {
		t.Fatal("publication recovery report grant was not registered")
	}
	stageReceipt, err := domainpendingwork.NewPendingWorkReceiptV1(domainpendingwork.ReceiptInputV1{
		Kind: domainpendingwork.KindReportStage, SecurityContext: securityContext,
		GrantRegistrySequence: activeRegistry.Sequence,
		GrantRegistryDigest:   activeRegistry.StateDigest,
		GrantMembers: []domainpendingwork.GrantMemberV1{{
			Ordinal: 1, GrantID: grant.GrantID, RegistrySequence: activeEntry.Sequence,
			RegistryEntryDigest: activeEntry.EntryDigest,
		}},
		PayloadHash: domainsecurity.SHA256Hex([]byte("publication-recovery-stage-payload")),
		RouteHash:   domainsecurity.SHA256Hex([]byte("publication-recovery-stage-route")),
		IssuedAt:    now, ExpiresAt: now.Add(time.Hour), AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	attemptID := domainpublication.PublicationAttemptIDV1(installationID, enrollmentID, stageReceipt.WorkID)
	index, err := domainpublication.NewPublicationIndexV1(domainpublication.PublicationIndexInputV1{
		InstallationID: receipt.InstallationID, EnrollmentID: receipt.EnrollmentID, Generation: 1,
		PreviousIndexDigest: domainpublication.PublicationIndexGenesisDigestV1(),
		MutationID:          domainpublication.PublicationIndexMutationIDForAttemptV1(attemptID),
		ReceiptID:           receipt.ReceiptID, ReceiptRecordDigest: receipt.RecordDigest, TargetIdentityDigest: target,
		AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
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
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	intentDigest := domainsecurity.SHA256Hex([]byte("publication-recovery-advance-intent"))
	attempt, err := domainpublication.NewPublicationAttemptV1(domainpublication.PublicationAttemptInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, ReportStageReceipt: stageReceipt,
		ToolCallID: toolCallID, StageInputHash: domainsecurity.SHA256Hex([]byte("publication-recovery-stage-input")),
		Candidate: receipt, Index: index, ExpectedEvidenceBundleDigest: previousBundle.RecordDigest,
		ExpectedPublicationIndexDigest: previousBundle.PublicationIndexDigest,
		ExpectedPublicationCount:       previousBundle.PublicationCount,
		NextEvidenceBundleDigest:       committedBundle.RecordDigest,
		AuthorityAdvanceIntentDigest:   intentDigest,
		AuthorityKeyID:                 keyID, AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Namespace: committedBundle.Namespace,
		Generation: committedBundle.Generation, CurrentStateDigest: committedBundle.RecordDigest,
		PreviousStateDigest:      previousBundle.RecordDigest,
		PreviousCheckpointDigest: domainsecurity.SHA256Hex([]byte("publication-recovery-previous-checkpoint")),
		FenceNonce:               domainsecurity.SHA256Hex([]byte("publication-recovery-fence")),
		MutationID:               committedBundle.MutationID,
		WitnessKeyID:             witnessKeyID,
		WitnessPublicKey:         witnessPublic,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(witnessPrivate, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	observeRequest, err := domainsecurity.NewMonotonicHeadObserveRequestV1(domainsecurity.MonotonicHeadObserveRequestInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Namespace: committedBundle.Namespace,
		ChallengeNonce: domainsecurity.SHA256Hex([]byte("publication-recovery-observe-challenge")),
		AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	observation, err := domainsecurity.NewMonotonicHeadObservationV1(observeRequest, checkpoint, func(message []byte) ([]byte, error) {
		return ed25519.Sign(witnessPrivate, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	commitInput := domainpublication.PublicationCommitReceiptInputV1{
		PreviousBundle: previousBundle, CommittedBundle: committedBundle,
		ObserveRequest: observeRequest, Observation: observation, Candidate: receipt, Index: index,
		InstallationID: installationID, EnrollmentID: enrollmentID,
		AuthorityKeyID: keyID, AuthorityPublicKey: publicKey, WitnessKeyID: witnessKeyID, WitnessPublicKey: witnessPublic,
	}
	settlementDigest := domainsecurity.SHA256Hex([]byte("publication-recovery-committed-settlement"))
	selection, err := domainpublication.NewPublicationCommitSelectionV1(domainpublication.PublicationCommitSelectionInputV1{
		Attempt: attempt, SettlementDigest: settlementDigest, CommitInput: commitInput,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	commit, err := domainpublication.NewPublicationCommitReceiptV1(commitInput, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil || domainpublication.ValidatePublicationCommitSelectionCommitV1(selection, commit) != nil {
		t.Fatalf("publication recovery commit graph is invalid: %v", err)
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
	decision, err := domainpublication.NewReportDeliveryDecisionV1(decisionInput, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil {
		t.Fatalf("publication recovery delivery decision graph is invalid: %v", err)
	}
	settledAt := now.Add(2 * time.Minute)
	settledRegistry, err := domainsecurity.SettleRegisteredExecutionGrant(activeRegistry, grant.GrantID, settledAt)
	if err != nil {
		t.Fatal(err)
	}
	grantSettlementInput := domainpublication.ReportGrantSettlementInputV1{
		Decision: decision, Grant: grant, ActiveRegistry: activeRegistry, SettledRegistry: settledRegistry,
		ResultItemID:     domaintoolresult.ToolResultItemIDV1(securityContext.TurnID, toolCallID),
		ResultItemDigest: domainsecurity.SHA256Hex([]byte("publication-recovery-private-tool-result")),
		SettledAt:        settledAt, AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
	}
	grantSettlement, err := domainpublication.NewReportGrantSettlementV1(grantSettlementInput, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil {
		t.Fatalf("publication recovery report grant settlement is invalid: %v", err)
	}
	stageDisposition, err := domainpendingwork.NewPendingWorkDispositionV1(
		stageReceipt, domainpendingwork.StatusCompleted, "report_stage_completed", settledAt, keyID, publicKey,
		func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	stageCompletionInput := domainpublication.ReportStageCompletionInputV1{
		Decision: decision, GrantSettlement: grantSettlement, StageReceipt: stageReceipt, StageDisposition: stageDisposition,
		AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
	}
	stageCompletion, err := domainpublication.NewReportStageCompletionV1(stageCompletionInput, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil {
		t.Fatalf("publication recovery report stage completion is invalid: %v", err)
	}
	deliveryProjectionInput := domainpublication.ReportDeliveryProjectionInputV1{
		Decision: decision, GrantSettlement: grantSettlement, StageReceipt: stageReceipt,
		StageDisposition: stageDisposition, StageCompletion: stageCompletion,
		AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
	}
	deliveryProjection, err := domainpublication.NewReportDeliveryProjectionV1(deliveryProjectionInput, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil {
		t.Fatalf("publication recovery report delivery projection is invalid: %v", err)
	}
	deliveryOutcome, err := domainpublication.ProjectedReportDeliveryOutcomeV1(deliveryProjection)
	if err != nil {
		t.Fatalf("publication recovery report delivery outcome is invalid: %v", err)
	}
	return publicationRecoveryFixture{
		decisionInput: decisionInput, grantSettlementInput: grantSettlementInput, stageCompletionInput: stageCompletionInput,
		deliveryProjectionInput: deliveryProjectionInput,
		authorityKey:            append(ed25519.PrivateKey(nil), privateKey...),
		receipt:                 receipt, index: index, stage: stageReceipt, attempt: attempt, selection: selection, commit: commit, decision: decision,
		grantSettlement: grantSettlement, stageDisposition: stageDisposition, stageCompletion: stageCompletion,
		deliveryProjection: deliveryProjection, deliveryOutcome: deliveryOutcome,
		ledger: ledger, projection: projection, inspection: inspection, target: target, artifact: artifact,
	}
}

func (fixture publicationRecoveryFixture) install(t *testing.T, stores *Stores, artifact []byte) {
	t.Helper()
	for _, action := range []func() error{
		func() error { return stores.Ledgers.PutIfAbsent(context.Background(), fixture.ledger) },
		func() error { return stores.PIIProjections.PutIfAbsent(context.Background(), fixture.projection) },
		func() error { return stores.Inspections.PutIfAbsent(context.Background(), fixture.inspection) },
		func() error { return stores.Artifacts.InstallNoReplace(context.Background(), fixture.target, artifact) },
		func() error { return stores.Receipts.PutIfAbsent(context.Background(), fixture.receipt) },
		func() error { return stores.Indexes.PutIfAbsent(context.Background(), fixture.index) },
		func() error {
			_, err := stores.Attempts.CreateExclusive(context.Background(), fixture.attempt)
			return err
		},
		func() error {
			_, err := stores.Selections.CreateExclusive(context.Background(), fixture.selection)
			return err
		},
		func() error { return stores.Commits.PutIfAbsent(context.Background(), fixture.commit) },
		func() error {
			_, err := stores.Decisions.CreateExclusive(context.Background(), fixture.decision)
			return err
		},
		func() error {
			_, err := stores.GrantSettlements.CreateExclusive(context.Background(), fixture.grantSettlement)
			return err
		},
		func() error {
			_, err := stores.StageCompletions.CreateExclusive(context.Background(), fixture.stageCompletion)
			return err
		},
		func() error {
			_, _, err := stores.DeliveryOutcomes.CreateOutcomeExclusive(
				context.Background(), fixture.stageCompletion, fixture.deliveryOutcome,
			)
			return err
		},
	} {
		if err := action(); err != nil {
			t.Fatal(err)
		}
	}
}
