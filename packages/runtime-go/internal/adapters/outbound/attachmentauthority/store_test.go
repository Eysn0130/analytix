package attachmentauthority

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	domainattachment "analytix.local/runtime-go/internal/domain/attachment"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	storeport "analytix.local/runtime-go/internal/ports/attachmentauthority"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	privatecasrecoverytest "analytix.local/runtime-go/internal/testsupport/privatecasrecovery"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestAttachmentAuthorityStorePersistsExactOwnerReceiptAndDisposition(t *testing.T) {
	root := filepath.Join(t.TempDir(), "attachment-authority")
	store := newAttachmentAuthorityTestStore(t, root)
	fixture := newAttachmentAuthorityFixture(t)

	if err := store.PutOwnerIfAbsent(context.Background(), fixture.owner); err != nil {
		t.Fatal(err)
	}
	if err := store.PutOwnerIfAbsent(context.Background(), fixture.owner); err != nil {
		t.Fatalf("exact owner replay failed: %v", err)
	}
	if err := store.PutUseReceiptIfAbsent(context.Background(), fixture.receipt); err != nil {
		t.Fatal(err)
	}
	if err := store.PutUseDispositionIfAbsent(context.Background(), fixture.disposition); err != nil {
		t.Fatal(err)
	}
	if err := store.PutUseDispositionIfAbsent(context.Background(), fixture.disposition); err != nil {
		t.Fatalf("exact disposition replay failed: %v", err)
	}

	reopened := newAttachmentAuthorityTestStore(t, root)
	owner, ownerErr := reopened.ResolveOwner(context.Background(), fixture.owner.OwnerDigest)
	receipt, receiptErr := reopened.ResolveUseReceipt(context.Background(), fixture.receipt.UseID)
	disposition, dispositionErr := reopened.ResolveUseDisposition(context.Background(), fixture.receipt.UseID)
	if ownerErr != nil || owner.OwnerDigest != fixture.owner.OwnerDigest ||
		receiptErr != nil || receipt.ReceiptDigest != fixture.receipt.ReceiptDigest ||
		dispositionErr != nil || disposition.RecordDigest != fixture.disposition.RecordDigest {
		t.Fatalf("authority restart readback mismatch: owner=%v receipt=%v disposition=%v", ownerErr, receiptErr, dispositionErr)
	}
	hasRecords, err := reopened.HasRecords(context.Background())
	if err != nil || !hasRecords {
		t.Fatalf("authority inventory missing after restart: has=%v err=%v", hasRecords, err)
	}
}

func TestAttachmentAuthorityStoreRejectsMissingOwnerAndConflictingDisposition(t *testing.T) {
	root := filepath.Join(t.TempDir(), "attachment-authority")
	store := newAttachmentAuthorityTestStore(t, root)
	fixture := newAttachmentAuthorityFixture(t)
	if err := store.PutUseReceiptIfAbsent(context.Background(), fixture.receipt); !errors.Is(err, storeport.ErrCorrupt) {
		t.Fatalf("receipt without owner classification = %v", err)
	}
	if err := store.PutOwnerIfAbsent(context.Background(), fixture.owner); err != nil {
		t.Fatal(err)
	}
	if err := store.PutUseReceiptIfAbsent(context.Background(), fixture.receipt); err != nil {
		t.Fatal(err)
	}
	if err := store.PutUseDispositionIfAbsent(context.Background(), fixture.disposition); err != nil {
		t.Fatal(err)
	}
	conflict, err := domainattachment.NewAttachmentUseDispositionV1(
		fixture.receipt, domainattachment.AttachmentUseDispositionFailedV1, "provider_failed",
		fixture.disposedAt.Add(time.Second), fixture.keyID, fixture.publicKey, fixture.sign,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutUseDispositionIfAbsent(context.Background(), conflict); !errors.Is(err, storeport.ErrConflict) {
		t.Fatalf("second disposition classification = %v", err)
	}
}

func TestAttachmentAuthorityStoreFailsClosedOnSemanticFilenameCorruption(t *testing.T) {
	root := filepath.Join(t.TempDir(), "attachment-authority")
	store := newAttachmentAuthorityTestStore(t, root)
	fixture := newAttachmentAuthorityFixture(t)
	if err := store.PutOwnerIfAbsent(context.Background(), fixture.owner); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "owners", fixture.owner.OwnerDigest[:2], fixture.owner.OwnerDigest+".json")
	if err := os.WriteFile(path, []byte(`{"attacker":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveOwner(context.Background(), fixture.owner.OwnerDigest); !errors.Is(err, storeport.ErrCorrupt) {
		t.Fatalf("corrupt owner classification = %v", err)
	}
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(root, access); !errors.Is(err, storeport.ErrCorrupt) {
		t.Fatalf("corrupt inventory reopened: %v", err)
	}
}

func TestAttachmentAuthorityRejectsDispositionWithoutExactIntent(t *testing.T) {
	store := newAttachmentAuthorityTestStore(t, filepath.Join(t.TempDir(), "attachment-authority"))
	fixture := newAttachmentAuthorityFixture(t)
	intent, err := domainattachment.NewUploadIntentV1(
		fixture.owner, domainsecurity.SHA256Hex([]byte("metadata")),
	)
	if err != nil {
		t.Fatal(err)
	}
	disposition, err := domainattachment.NewUploadDispositionV1(
		intent, domainattachment.UploadDispositionQuarantinedV1, "restart_missing_upload_files", "", "", fixture.disposedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutUploadDispositionIfAbsent(context.Background(), disposition); !errors.Is(err, storeport.ErrCorrupt) {
		t.Fatalf("disposition without intent classification = %v", err)
	}
	if err := store.PutUploadIntentIfAbsent(context.Background(), intent); err != nil {
		t.Fatal(err)
	}
	if err := store.PutUploadDispositionIfAbsent(context.Background(), disposition); err != nil {
		t.Fatal(err)
	}
	if err := store.PutUploadDispositionIfAbsent(context.Background(), disposition); err != nil {
		t.Fatalf("exact quarantined disposition replay failed: %v", err)
	}
}

func TestAttachmentAuthorityRejectsOwnerCommitAfterQuarantine(t *testing.T) {
	root := filepath.Join(t.TempDir(), "attachment-authority")
	store := newAttachmentAuthorityTestStore(t, root)
	fixture := newAttachmentAuthorityFixture(t)
	intent, err := domainattachment.NewUploadIntentV1(
		fixture.owner, domainsecurity.SHA256Hex([]byte("metadata")),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutUploadIntentIfAbsent(context.Background(), intent); err != nil {
		t.Fatal(err)
	}
	quarantined, err := domainattachment.NewUploadDispositionV1(
		intent, domainattachment.UploadDispositionQuarantinedV1, "restart_stale_upload_binding", "", "", fixture.disposedAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutUploadDispositionIfAbsent(context.Background(), quarantined); err != nil {
		t.Fatal(err)
	}
	if err := store.CommitOwnerForOpenUpload(context.Background(), intent); !errors.Is(err, storeport.ErrConflict) {
		t.Fatalf("owner commit after quarantine classification = %v", err)
	}
	if err := store.PutOwnerIfAbsent(context.Background(), fixture.owner); !errors.Is(err, storeport.ErrConflict) {
		t.Fatalf("legacy owner path bypassed quarantined upload: %v", err)
	}
	if _, err := store.ResolveOwner(context.Background(), fixture.owner.OwnerDigest); !errors.Is(err, storeport.ErrNotFound) {
		t.Fatalf("quarantined upload gained owner authority: %v", err)
	}
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(root, access); err != nil {
		t.Fatalf("valid quarantined inventory did not reopen: %v", err)
	}
}

func TestAttachmentAuthorityRecoveryPreflightDoesNotCreateMissingRoots(t *testing.T) {
	root := filepath.Join(t.TempDir(), "attachment-authority")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := PreflightRecovery(context.Background(), root, access); err != nil {
		t.Fatal(err)
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
	if err := privatecasrecoverytest.ApplyV4(context.Background(), "attachment-authority-test", prepared); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recovery created an absent authority root: %v", err)
	}
}

type attachmentAuthorityFixture struct {
	owner       domainattachment.OwnerRecordV1
	receipt     domainattachment.AttachmentUseReceiptV1
	disposition domainattachment.AttachmentUseDispositionV1
	publicKey   ed25519.PublicKey
	privateKey  ed25519.PrivateKey
	keyID       string
	disposedAt  time.Time
}

func newAttachmentAuthorityFixture(t *testing.T) attachmentAuthorityFixture {
	t.Helper()
	issuedAt := time.Date(2026, 7, 14, 3, 0, 0, 0, time.UTC)
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-a", TurnID: "turn-a", WorkspaceRealPath: "/cases/a",
		CaseID: "case-a", ContextEpoch: 7, IssuedAt: issuedAt.Add(-2 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	observation, err := domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: securityContext.WorkspaceRealPath, State: domainsecurity.CaseBindingStateValid,
		CaseID: securityContext.CaseID, BindingSHA256: domainsecurity.SHA256Hex([]byte("binding-document")),
		CaseBindingHash: securityContext.CaseBindingHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := domainattachment.NewOwnerRecordV1(domainattachment.OwnerRecordInputV1{
		OwnerNonce: fmt.Sprintf("%032x", 1), BlobSHA256: domainsecurity.SHA256Hex([]byte("blob")),
		ByteSize: 4, MIMEType: "application/pdf", ThreadID: securityContext.ThreadID,
		WorkspaceRealPath: securityContext.WorkspaceRealPath, CaseBindingObservation: &observation,
		ProjectionSHA256: domainsecurity.SHA256Hex([]byte("upload-metadata-projection")), CreatedAt: issuedAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	fixture := attachmentAuthorityFixture{
		owner: owner, publicKey: publicKey, privateKey: privateKey,
		keyID: domainsecurity.SHA256Hex(publicKey), disposedAt: issuedAt.Add(time.Minute),
	}
	projection := []byte("actual-provider-projection")
	receipt, err := domainattachment.NewAttachmentUseReceiptV1(domainattachment.AttachmentUseReceiptInputV1{
		SecurityContext: securityContext, Owner: owner, AttachmentSet: []domainattachment.OwnerRecordV1{owner},
		ProjectionSet: []domainattachment.AttachmentUseProjectionV1{{
			AttachmentID: owner.AttachmentID, ProjectionSHA256: domainsecurity.SHA256Hex(projection),
			ProjectionByteSize: int64(len(projection)),
		}},
		BatchIndex: 0, EffectBindingDigest: domainsecurity.SHA256Hex([]byte("provider-attempt")),
		ConsumerKind:          domainattachment.AttachmentUseConsumerPrimaryProviderV1,
		ConsumerBindingDigest: domainsecurity.SHA256Hex([]byte("provider-route")), IssuedAt: issuedAt,
		AuthorityKeyID: fixture.keyID, AuthorityPublicKey: fixture.publicKey,
	}, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	fixture.receipt = receipt
	fixture.disposition, err = domainattachment.NewAttachmentUseDispositionV1(
		receipt, domainattachment.AttachmentUseDispositionConsumedV1, "provider_consumed",
		fixture.disposedAt, fixture.keyID, fixture.publicKey, fixture.sign,
	)
	if err != nil {
		t.Fatal(err)
	}
	return fixture
}

func (fixture attachmentAuthorityFixture) sign(message []byte) ([]byte, error) {
	return ed25519.Sign(fixture.privateKey, message), nil
}

func newAttachmentAuthorityTestStore(t *testing.T, root string) *Store {
	t.Helper()
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	return store
}
