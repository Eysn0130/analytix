package reportpublication

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"os"
	"path/filepath"
	"testing"
	"time"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	testsecurity "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestReportStoresConstructorRollsBackEveryPriorCASAndCloseIsIdempotent(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "report-publication")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "render-inspections"), []byte("constructor obstacle"), 0o600); err != nil {
		t.Fatal(err)
	}
	if stores, err := NewStores(root, access); err == nil || stores != nil {
		t.Fatalf("publication stores ignored a late constructor failure: stores=%#v err=%v", stores, err)
	}
	// Replacing the failed root proves every earlier root-generation anchor was
	// released. A leaked CAS would retain the removed identity and reject reopen.
	if err := os.RemoveAll(root); err != nil {
		t.Fatalf("failed constructor retained an open root: %v", err)
	}
	stores, err := NewStores(root, access)
	if err != nil {
		t.Fatalf("constructor rollback retained stale private-CAS authority: %v", err)
	}
	if len(stores.owners) != 13 {
		t.Fatalf("publication lifecycle owner lost a store: %d", len(stores.owners))
	}
	if err := stores.Close(); err != nil {
		t.Fatal(err)
	}
	if err := stores.Close(); err != nil {
		t.Fatalf("publication stores close was not idempotent: %v", err)
	}
	if _, err := stores.HasRecords(context.Background()); err == nil {
		t.Fatal("closed publication stores retained live private-CAS authority")
	}
}

func TestPublicationAuthorityStoresPersistCanonicalRecordsAndOpaqueArtifact(t *testing.T) {
	root := t.TempDir()
	mutation, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)
	securityContext, err := testsecurity.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-publication-store", TurnID: "turn-publication-store", WorkspaceRealPath: "/workspace/publication-store",
		CaseID: "case-publication-store", CaseBindingHash: domainsecurity.SHA256Hex([]byte("publication-store-binding")), ContextEpoch: 3, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	evidenceID := "evr_" + domainsecurity.SHA256Hex([]byte("publication-store-no-hit"))
	ledger, err := domainpublication.NewClaimLedgerV1(domainpublication.ClaimLedgerInputV1{
		Context: securityContext, EvidenceReceiptIDs: []string{evidenceID}, Claims: nil, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	artifact := []byte("verified no-hit report")
	reportSHA := domainsecurity.SHA256Hex(artifact)
	projection, err := domainpublication.NewPIIProjectionV1(domainpublication.PIIProjectionInputV1{
		ProjectionClass: domainpublication.PIIProjectionOrdinaryMasked, RulesetHash: domainsecurity.SHA256Hex([]byte("publication-store-rules")),
		ProjectedContentSHA256: reportSHA,
	})
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := domainpublication.NewRenderInspectionV1(domainpublication.RenderInspectionInputV1{
		Renderer: "analytix-host", RendererVersion: "1.0.0", ReportSHA256: reportSHA,
		ReportByteLength: uint64(len(artifact)), MediaType: "application/pdf", Passed: true, IssueCodes: []string{}, InspectedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x52}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	keyID := domainsecurity.SHA256Hex(publicKey)
	installationID := domainsecurity.SHA256Hex([]byte("publication-store-installation"))
	enrollmentID := domainsecurity.SHA256Hex([]byte("publication-store-enrollment"))
	target := domainsecurity.SHA256Hex([]byte("publication-store-target"))
	receipt, err := domainpublication.NewPublicationReceiptV1(domainpublication.PublicationReceiptInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Context: securityContext,
		ReportVariant: domainpublication.VerifiedNoHitReport, EvidenceAuthorityBundleDigest: domainsecurity.SHA256Hex([]byte("publication-store-bundle")),
		EvidenceRegistryIndexDigest: domainsecurity.SHA256Hex([]byte("publication-store-registry-index")), EvidenceRegistryCount: 1,
		EvidenceRegistrySequence: 1, EvidenceRegistryStateDigest: domainsecurity.SHA256Hex([]byte("publication-store-registry-state")),
		ClaimLedger: ledger, ReportSHA256: reportSHA, ReportByteLength: uint64(len(artifact)), MediaType: "application/pdf",
		PIIProjection: projection, RenderInspection: inspection, Publisher: "analytix-host", PublisherVersion: "1.0.0",
		TargetIdentityDigest: target, IssuedAt: now, AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	stageReceipt, err := domainpendingwork.NewPendingWorkReceiptV1(domainpendingwork.ReceiptInputV1{
		Kind: domainpendingwork.KindReportStage, SecurityContext: securityContext,
		GrantRegistrySequence: 3, GrantRegistryDigest: domainsecurity.SHA256Hex([]byte("publication-store-grant-registry")),
		GrantMembers: []domainpendingwork.GrantMemberV1{{
			Ordinal: 1, GrantID: domainsecurity.SHA256Hex([]byte("publication-store-grant")), RegistrySequence: 3,
			RegistryEntryDigest: domainsecurity.SHA256Hex([]byte("publication-store-grant-entry")),
		}},
		PayloadHash: domainsecurity.SHA256Hex([]byte("publication-store-stage-payload")),
		RouteHash:   domainsecurity.SHA256Hex([]byte("publication-store-stage-route")),
		IssuedAt:    now, ExpiresAt: now.Add(time.Hour), AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	attemptID := domainpublication.PublicationAttemptIDV1(installationID, enrollmentID, stageReceipt.WorkID)
	index, err := domainpublication.NewPublicationIndexV1(domainpublication.PublicationIndexInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Generation: 1,
		PreviousIndexDigest: domainpublication.PublicationIndexGenesisDigestV1(),
		MutationID:          domainpublication.PublicationIndexMutationIDForAttemptV1(attemptID),
		ReceiptID:           receipt.ReceiptID, ReceiptRecordDigest: receipt.RecordDigest, TargetIdentityDigest: target,
		AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	toolCallID, err := domainsecurity.NewHostToolCallIDV1(bytes.Repeat([]byte{0x53}, domainsecurity.HostToolCallIDEntropyBytesV1))
	if err != nil {
		t.Fatal(err)
	}
	attemptInput := domainpublication.PublicationAttemptInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, ReportStageReceipt: stageReceipt,
		ToolCallID: toolCallID, StageInputHash: domainsecurity.SHA256Hex([]byte("publication-store-stage-input")),
		Candidate: receipt, Index: index, ExpectedEvidenceBundleDigest: receipt.EvidenceAuthorityBundleDigest,
		ExpectedPublicationIndexDigest: domainpublication.PublicationIndexGenesisDigestV1(), ExpectedPublicationCount: 0,
		NextEvidenceBundleDigest:     domainsecurity.SHA256Hex([]byte("publication-store-next-bundle")),
		AuthorityAdvanceIntentDigest: domainsecurity.SHA256Hex([]byte("publication-store-advance-intent")),
		AuthorityKeyID:               keyID, AuthorityPublicKey: publicKey,
	}
	attempt, err := domainpublication.NewPublicationAttemptV1(attemptInput, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}

	attempts, err := newAttemptStore(filepath.Join(root, "attempts"), mutation)
	if err != nil {
		t.Fatal(err)
	}
	receipts, err := newReceiptStore(filepath.Join(root, "receipts"), mutation)
	if err != nil {
		t.Fatal(err)
	}
	indexes, err := newIndexStore(filepath.Join(root, "indexes"), mutation)
	if err != nil {
		t.Fatal(err)
	}
	ledgers, err := newClaimLedgerStore(filepath.Join(root, "ledgers"), mutation)
	if err != nil {
		t.Fatal(err)
	}
	projections, err := newPIIProjectionStore(filepath.Join(root, "projections"), mutation)
	if err != nil {
		t.Fatal(err)
	}
	inspections, err := newRenderInspectionStore(filepath.Join(root, "inspections"), mutation)
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := newArtifactStore(filepath.Join(root, "artifacts"), mutation)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, cas := range []*finalauthorityadapter.SecurePrivateCAS{
			artifacts.cas, inspections.cas, projections.cas, ledgers.cas, indexes.cas, receipts.cas, attempts.cas,
		} {
			_ = cas.Close()
		}
	})
	for iteration := range 2 {
		created, err := attempts.CreateExclusive(context.Background(), attempt)
		if err != nil || created != (iteration == 0) {
			t.Fatalf("attempt exclusive reservation iteration %d: created=%v err=%v", iteration, created, err)
		}
		if err := receipts.PutIfAbsent(context.Background(), receipt); err != nil {
			t.Fatal(err)
		}
		if err := indexes.PutIfAbsent(context.Background(), index); err != nil {
			t.Fatal(err)
		}
		if err := ledgers.PutIfAbsent(context.Background(), ledger); err != nil {
			t.Fatal(err)
		}
		if err := projections.PutIfAbsent(context.Background(), projection); err != nil {
			t.Fatal(err)
		}
		if err := inspections.PutIfAbsent(context.Background(), inspection); err != nil {
			t.Fatal(err)
		}
		if err := artifacts.InstallNoReplace(context.Background(), target, artifact); err != nil {
			t.Fatal(err)
		}
	}
	changedInput := attemptInput
	changedInput.StageInputHash = domainsecurity.SHA256Hex([]byte("publication-store-changed-stage-input"))
	changedAttempt, err := domainpublication.NewPublicationAttemptV1(changedInput, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if changedAttempt.AttemptID != attempt.AttemptID || changedAttempt.RecordDigest == attempt.RecordDigest {
		t.Fatal("changed report stage did not retain the exclusive attempt identity")
	}
	if _, err := attempts.CreateExclusive(context.Background(), changedAttempt); err == nil {
		t.Fatal("attempt store accepted a changed plan under one report-stage identity")
	}
	if stored, err := attempts.Resolve(context.Background(), attempt.AttemptID); err != nil || stored.RecordDigest != attempt.RecordDigest {
		t.Fatalf("attempt readback mismatch: %#v err=%v", stored, err)
	}
	if stored, err := receipts.Resolve(context.Background(), receipt.RecordDigest); err != nil || stored.RecordDigest != receipt.RecordDigest {
		t.Fatalf("receipt readback mismatch: %#v err=%v", stored, err)
	}
	if stored, err := indexes.Resolve(context.Background(), index.IndexDigest); err != nil || domainpublication.ValidatePublicationIndexReceiptV1(stored, receipt) != nil {
		t.Fatalf("index readback mismatch: %#v err=%v", stored, err)
	}
	receiptVisits, indexVisits := 0, 0
	if err := receipts.VisitReceipts(context.Background(), func(candidate domainpublication.PublicationReceiptV1) error {
		receiptVisits++
		if candidate.RecordDigest != receipt.RecordDigest {
			t.Fatalf("receipt inventory returned another record: %#v", candidate)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := indexes.VisitIndexes(context.Background(), func(candidate domainpublication.PublicationIndexV1) error {
		indexVisits++
		if candidate.IndexDigest != index.IndexDigest {
			t.Fatalf("index inventory returned another record: %#v", candidate)
		}
		return nil
	}); err != nil || receiptVisits != 1 || indexVisits != 1 {
		t.Fatalf("publication inventory visit failed: receipts=%d indexes=%d err=%v", receiptVisits, indexVisits, err)
	}
	if stored, err := ledgers.Resolve(context.Background(), ledger.LedgerDigest); err != nil || stored.LedgerDigest != ledger.LedgerDigest {
		t.Fatalf("ledger readback mismatch: %#v err=%v", stored, err)
	}
	if stored, err := projections.Resolve(context.Background(), projection.ProjectionDigest); err != nil || stored.ProjectionDigest != projection.ProjectionDigest {
		t.Fatalf("projection readback mismatch: %#v err=%v", stored, err)
	}
	if stored, err := inspections.Resolve(context.Background(), inspection.InspectionDigest); err != nil || stored.InspectionDigest != inspection.InspectionDigest {
		t.Fatalf("inspection readback mismatch: %#v err=%v", stored, err)
	}
	if stored, err := artifacts.ResolveExact(context.Background(), target); err != nil || !bytes.Equal(stored, artifact) {
		t.Fatalf("artifact readback mismatch: %q err=%v", stored, err)
	}
	if _, err := artifacts.ResolveControlledMetadata(context.Background(), target); err == nil {
		t.Fatal("ordinary report bytes were accepted as a controlled PII artifact")
	}
	if err := artifacts.InstallNoReplace(context.Background(), target, []byte("different bytes")); err == nil {
		t.Fatal("artifact target was overwritten")
	}
}
