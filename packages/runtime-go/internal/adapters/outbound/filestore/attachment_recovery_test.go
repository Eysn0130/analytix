package filestore

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	turnapp "analytix.local/runtime-go/internal/app/turn"
	domainattachment "analytix.local/runtime-go/internal/domain/attachment"
	storeport "analytix.local/runtime-go/internal/ports/attachmentauthority"
)

func TestAttachmentAuthorityReconciliationQuarantinesOwnerlessFilesWithoutMutation(t *testing.T) {
	store, created, _ := createRecoveryAttachment(t)
	id := created["id"].(string)

	plan, err := preflightRecoveryAttachment(store, &attachmentRecoveryAuthority{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Quarantined) != 1 || plan.Quarantined[0].AttachmentID != id ||
		plan.Quarantined[0].MetadataSHA256 == "" || plan.Quarantined[0].ContentSHA256 == "" {
		t.Fatalf("ownerless attachment was not deterministically quarantined: %#v", plan)
	}
	if _, _, found, err := store.Content(id); err != nil || !found {
		t.Fatalf("read-only reconciliation mutated quarantined user data: found=%v err=%v", found, err)
	}
}

func TestAttachmentAuthorityReconciliationRetainsExactOwnerBackedUpload(t *testing.T) {
	store, _, owner := createRecoveryAttachment(t)
	plan, err := preflightRecoveryAttachment(store, &attachmentRecoveryAuthority{owners: []domainattachment.OwnerRecordV1{owner}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Quarantined) != 0 {
		t.Fatalf("authorized attachment was quarantined: %#v", plan)
	}
	if _, _, found, err := store.Content(owner.AttachmentID); err != nil || !found {
		t.Fatalf("authorized attachment was not retained: found=%v err=%v", found, err)
	}
}

func TestAttachmentAuthorityReconciliationRejectsIncompleteOrCorruptAuthorizedUpload(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*PersistentAttachmentStore, string) error
	}{
		{name: "missing_metadata", mutate: func(store *PersistentAttachmentStore, id string) error {
			return os.Remove(store.metadataPath(id))
		}},
		{name: "missing_content", mutate: func(store *PersistentAttachmentStore, id string) error {
			return os.Remove(store.contentPath(id))
		}},
		{name: "corrupt_metadata", mutate: func(store *PersistentAttachmentStore, id string) error {
			return os.WriteFile(store.metadataPath(id), []byte(`{"id":"tampered"}`), 0o600)
		}},
		{name: "corrupt_content", mutate: func(store *PersistentAttachmentStore, id string) error {
			return os.WriteFile(store.contentPath(id), []byte("tampered"), 0o600)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, _, owner := createRecoveryAttachment(t)
			if err := test.mutate(store, owner.AttachmentID); err != nil {
				t.Fatal(err)
			}
			if _, err := preflightRecoveryAttachment(
				store, &attachmentRecoveryAuthority{owners: []domainattachment.OwnerRecordV1{owner}},
			); err == nil {
				t.Fatal("incomplete or corrupt owner-backed attachment passed startup reconciliation")
			}
		})
	}
}

func TestAttachmentAuthorityReconciliationMissingRootIsReadOnlyAndCancellable(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	store, err := NewPersistentAttachmentStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := preflightRecoveryAttachment(store, &attachmentRecoveryAuthority{})
	if err != nil || len(plan.Quarantined) != 0 {
		t.Fatalf("missing attachment root did not remain absent: plan=%#v err=%v", plan, err)
	}
	if _, err := os.Lstat(store.root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reconciliation created a missing attachment root: %v", err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.PreflightAuthorityReconciliation(
		cancelled, &attachmentRecoveryAuthority{}, func(domainattachment.OwnerRecordV1) error { return nil }, time.Now().UTC(),
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled reconciliation did not stop: %v", err)
	}
}

func TestAttachmentAuthorityReconciliationRejectsUnknownResidue(t *testing.T) {
	store, err := NewPersistentAttachmentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(store.metadataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.metadataDir, "README"), []byte("unknown"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := preflightRecoveryAttachment(store, &attachmentRecoveryAuthority{}); err == nil {
		t.Fatal("unknown attachment residue passed startup reconciliation")
	}
}

func TestCompleteIntentStageRecoversOwnerExactlyOnce(t *testing.T) {
	store, _, owner := createRecoveryAttachment(t)
	intent := recoveryUploadIntent(t, store, owner)
	authority := &attachmentRecoveryAuthority{intents: []domainattachment.UploadIntentV1{intent}}
	plan, err := preflightRecoveryAttachment(store, authority)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 || !plan.Actions[0].CommitOwner ||
		plan.Actions[0].Disposition.Status != domainattachment.UploadDispositionCommittedV1 {
		t.Fatalf("exact staged upload did not plan owner recovery: %#v", plan)
	}
	if err := store.ApplyAuthorityReconciliation(
		context.Background(), authority, func(domainattachment.OwnerRecordV1) error { return nil }, plan,
	); err != nil {
		t.Fatal(err)
	}
	if len(authority.owners) != 1 || len(authority.dispositions) != 1 ||
		authority.dispositions[0].Status != domainattachment.UploadDispositionCommittedV1 {
		t.Fatalf("exact staged upload did not converge: %#v", authority)
	}
	replayed, err := preflightRecoveryAttachment(store, authority)
	if err != nil || len(replayed.Actions) != 0 {
		t.Fatalf("committed upload restart was not idempotent: plan=%#v err=%v", replayed, err)
	}
}

type heldAttachmentRecoveryAuthorityV1 struct {
	*attachmentRecoveryAuthority
	held   string
	checks int
	err    error
}

func (authority *heldAttachmentRecoveryAuthorityV1) RevalidateAttachmentRestartV1(ctx context.Context) error {
	authority.checks++
	return errors.Join(authority.err, ctx.Err())
}

func TestAttachmentRestartMixedUploadRecoveryAndLateFailure(t *testing.T) {
	ctx := context.Background()
	store, _, heldOwner := createRecoveryAttachment(t)
	heldIntent := recoveryUploadIntent(t, store, heldOwner)
	body := recoveryAttachmentBody(t, "independent.txt", "independent upload bytes")
	body["threadId"] = "thread-independent"
	created, err := store.Create(body)
	if err != nil {
		t.Fatal(err)
	}
	independentOwner, err := validateAttachmentMetadataIntegrity(created["id"].(string), created)
	if err != nil {
		t.Fatal(err)
	}
	independentIntent := recoveryUploadIntent(t, store, independentOwner)
	authority := &heldAttachmentRecoveryAuthorityV1{attachmentRecoveryAuthority: &attachmentRecoveryAuthority{intents: []domainattachment.UploadIntentV1{heldIntent, independentIntent}}, held: heldOwner.ThreadID}
	validate := func(owner domainattachment.OwnerRecordV1) error {
		if owner.ThreadID != independentOwner.ThreadID {
			t.Fatal("held upload reached current validator")
		}
		return nil
	}
	plan, err := store.PreflightAuthorityReconciliation(ctx, authority, validate, time.Now().UTC().Add(time.Minute))
	if err != nil || len(plan.Actions) != 1 || len(plan.PreservedAttachmentIDs) != 1 || plan.Actions[0].Intent.UploadID != independentIntent.UploadID {
		t.Fatalf("mixed upload recovery plan: %v", err)
	}
	cause := errors.New("synthetic original attachment changed before apply")
	authority.err = cause
	if err := store.ApplyAuthorityReconciliation(ctx, authority, validate, plan); !errors.Is(err, cause) {
		t.Fatalf("late preservation cause: %v", err)
	}
	if len(authority.owners) != 0 || len(authority.dispositions) != 0 {
		t.Fatal("late original observation failure wrote recovery state")
	}
	authority.err = nil
	if err := store.ApplyAuthorityReconciliation(ctx, authority, validate, plan); err != nil {
		t.Fatal(err)
	}
	if len(authority.owners) != 1 || authority.owners[0].OwnerDigest != independentOwner.OwnerDigest || len(authority.dispositions) != 1 || authority.dispositions[0].UploadID != independentIntent.UploadID {
		t.Fatal("mixed upload recovery changed held state or failed independent recovery")
	}
}

func TestAttachmentRecoveryApplyKeepsCurrentObservationFailureCause(t *testing.T) {
	store, _, owner := createRecoveryAttachment(t)
	intent := recoveryUploadIntent(t, store, owner)
	authority := &attachmentRecoveryAuthority{intents: []domainattachment.UploadIntentV1{intent}}
	plan, err := preflightRecoveryAttachment(store, authority)
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("synthetic current upload primary read failure")
	if err := store.ApplyAuthorityReconciliation(context.Background(), authority, func(domainattachment.OwnerRecordV1) error { return cause }, plan); !errors.Is(err, cause) {
		t.Fatalf("current upload observation failure cause was flattened: %v", err)
	}
	if len(authority.owners) != 0 || len(authority.dispositions) != 0 {
		t.Fatal("current observation failure wrote upload recovery state")
	}
}

func (authority *heldAttachmentRecoveryAuthorityV1) RestartPreservesThreadV1(id string) bool {
	return id == authority.held
}

func TestAttachmentRestartDoesNotCompleteHeldUpload(t *testing.T) {
	store, _, owner := createRecoveryAttachment(t)
	intent := recoveryUploadIntent(t, store, owner)
	authority := &heldAttachmentRecoveryAuthorityV1{attachmentRecoveryAuthority: &attachmentRecoveryAuthority{intents: []domainattachment.UploadIntentV1{intent}}, held: owner.ThreadID}
	currentCalls := 0
	validate := func(domainattachment.OwnerRecordV1) error { currentCalls++; return nil }
	plan, err := store.PreflightAuthorityReconciliation(context.Background(), authority, validate, time.Now().UTC().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 0 {
		t.Errorf("held upload entered recovery actions: %d", len(plan.Actions))
	}
	if err := store.ApplyAuthorityReconciliation(context.Background(), authority, validate, plan); err != nil {
		t.Fatal(err)
	}
	if len(authority.owners) != 0 || len(authority.dispositions) != 0 || currentCalls != 0 || authority.checks == 0 {
		t.Errorf("held upload was recovered or borrowed live authority: owners=%d dispositions=%d current=%d checks=%d", len(authority.owners), len(authority.dispositions), currentCalls, authority.checks)
	}
}

func TestPartialOrStaleIntentStageNeverGainsOwner(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*PersistentAttachmentStore, string) error
		stale  bool
		reason string
	}{
		{name: "partial", mutate: func(store *PersistentAttachmentStore, id string) error {
			return os.Remove(store.metadataPath(id))
		}, reason: "restart_partial_upload_files"},
		{name: "corrupt", mutate: func(store *PersistentAttachmentStore, id string) error {
			return os.WriteFile(store.contentPath(id), []byte("corrupt"), 0o600)
		}, reason: "restart_corrupt_upload_files"},
		{name: "stale_binding", stale: true, reason: "restart_stale_upload_binding"},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, _, owner := createRecoveryAttachment(t)
			intent := recoveryUploadIntent(t, store, owner)
			if test.mutate != nil {
				if err := test.mutate(store, owner.AttachmentID); err != nil {
					t.Fatal(err)
				}
			}
			authority := &attachmentRecoveryAuthority{intents: []domainattachment.UploadIntentV1{intent}}
			validator := func(domainattachment.OwnerRecordV1) error { return nil }
			if test.stale {
				validator = func(domainattachment.OwnerRecordV1) error { return turnapp.ErrAttachmentNotAuthorized }
			}
			plan, err := store.PreflightAuthorityReconciliation(
				context.Background(), authority, validator, time.Now().UTC().Add(time.Minute),
			)
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Actions) != 1 || plan.Actions[0].CommitOwner ||
				plan.Actions[0].Disposition.Status != domainattachment.UploadDispositionQuarantinedV1 ||
				plan.Actions[0].Disposition.ReasonCode != test.reason {
				t.Fatalf("unsafe staged upload was not quarantined: %#v", plan)
			}
			if err := store.ApplyAuthorityReconciliation(context.Background(), authority, validator, plan); err != nil {
				t.Fatal(err)
			}
			if len(authority.owners) != 0 || len(authority.dispositions) != 1 {
				t.Fatalf("unsafe staged upload gained owner authority: %#v", authority)
			}
		})
	}
}

func TestOwnerCommittedBeforeResponseRecoversCommittedDisposition(t *testing.T) {
	store, _, owner := createRecoveryAttachment(t)
	intent := recoveryUploadIntent(t, store, owner)
	authority := &attachmentRecoveryAuthority{
		owners: []domainattachment.OwnerRecordV1{owner}, intents: []domainattachment.UploadIntentV1{intent},
	}
	plan, err := preflightRecoveryAttachment(store, authority)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].CommitOwner ||
		plan.Actions[0].Disposition.Status != domainattachment.UploadDispositionCommittedV1 {
		t.Fatalf("owner-before-response crash did not plan committed settlement: %#v", plan)
	}
	if err := store.ApplyAuthorityReconciliation(
		context.Background(), authority, func(domainattachment.OwnerRecordV1) error { return nil }, plan,
	); err != nil {
		t.Fatal(err)
	}
	if len(authority.dispositions) != 1 || authority.dispositions[0].Status != domainattachment.UploadDispositionCommittedV1 {
		t.Fatalf("owner-before-response crash did not settle: %#v", authority)
	}
}

func TestAttachmentRecoveryPreflightFailsBeforeFirstMutation(t *testing.T) {
	dataDir := t.TempDir()
	store, err := NewPersistentAttachmentStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Create(recoveryAttachmentBody(t, "first.txt", "first"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Create(recoveryAttachmentBody(t, "second.txt", "second"))
	if err != nil {
		t.Fatal(err)
	}
	firstOwner, _ := validateAttachmentMetadataIntegrity(first["id"].(string), first)
	secondOwner, _ := validateAttachmentMetadataIntegrity(second["id"].(string), second)
	authority := &attachmentRecoveryAuthority{intents: []domainattachment.UploadIntentV1{
		recoveryUploadIntent(t, store, firstOwner), recoveryUploadIntent(t, store, secondOwner),
	}}
	plan, err := preflightRecoveryAttachment(store, authority)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.contentPath(secondOwner.AttachmentID), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.ApplyAuthorityReconciliation(
		context.Background(), authority, func(domainattachment.OwnerRecordV1) error { return nil }, plan,
	); err == nil {
		t.Fatal("changed later recovery action was accepted")
	}
	if len(authority.owners) != 0 || len(authority.dispositions) != 0 {
		t.Fatalf("recovery mutated an earlier action before later validation failed: %#v", authority)
	}
}

func createRecoveryAttachment(t *testing.T) (*PersistentAttachmentStore, map[string]any, domainattachment.OwnerRecordV1) {
	t.Helper()
	store, err := NewPersistentAttachmentStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Create(map[string]any{
		"name":       "evidence.txt",
		"mimeType":   "text/plain",
		"dataBase64": base64.StdEncoding.EncodeToString([]byte("authorized evidence")),
		"threadId":   "thread_recovery",
		"workspace":  t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := validateAttachmentMetadataIntegrity(created["id"].(string), created)
	if err != nil {
		t.Fatal(err)
	}
	return store, created, owner
}

func recoveryAttachmentBody(t *testing.T, name string, content string) map[string]any {
	t.Helper()
	return map[string]any{
		"name": name, "mimeType": "text/plain", "dataBase64": base64.StdEncoding.EncodeToString([]byte(content)),
		"threadId": "thread_recovery", "workspace": t.TempDir(),
	}
}

func recoveryUploadIntent(t *testing.T, store *PersistentAttachmentStore, owner domainattachment.OwnerRecordV1) domainattachment.UploadIntentV1 {
	t.Helper()
	metadataDigest, err := attachmentRecoveryFileDigest(context.Background(), store.metadataPath(owner.AttachmentID))
	if err != nil {
		t.Fatal(err)
	}
	intent, err := domainattachment.NewUploadIntentV1(owner, metadataDigest)
	if err != nil {
		t.Fatal(err)
	}
	return intent
}

type attachmentRecoveryAuthority struct {
	owners       []domainattachment.OwnerRecordV1
	intents      []domainattachment.UploadIntentV1
	dispositions []domainattachment.UploadDispositionV1
}

func preflightRecoveryAttachment(
	store *PersistentAttachmentStore,
	authority *attachmentRecoveryAuthority,
) (AttachmentAuthorityReconciliationPlanV1, error) {
	return store.PreflightAuthorityReconciliation(
		context.Background(), authority, func(domainattachment.OwnerRecordV1) error { return nil },
		time.Now().UTC().Add(time.Minute),
	)
}

func (authority *attachmentRecoveryAuthority) PutOwnerIfAbsent(context.Context, domainattachment.OwnerRecordV1) error {
	return errors.New("read-only test authority")
}

func (authority *attachmentRecoveryAuthority) ResolveOwner(_ context.Context, digest string) (domainattachment.OwnerRecordV1, error) {
	for _, owner := range authority.owners {
		if owner.OwnerDigest == digest {
			return owner, nil
		}
	}
	return domainattachment.OwnerRecordV1{}, storeport.ErrNotFound
}

func (authority *attachmentRecoveryAuthority) PutUploadIntentIfAbsent(_ context.Context, intent domainattachment.UploadIntentV1) error {
	for _, existing := range authority.intents {
		if existing.UploadID == intent.UploadID {
			return nil
		}
	}
	authority.intents = append(authority.intents, intent)
	return nil
}

func (authority *attachmentRecoveryAuthority) ResolveUploadIntent(_ context.Context, uploadID string) (domainattachment.UploadIntentV1, error) {
	for _, intent := range authority.intents {
		if intent.UploadID == uploadID {
			return intent, nil
		}
	}
	return domainattachment.UploadIntentV1{}, storeport.ErrNotFound
}

func (authority *attachmentRecoveryAuthority) CommitOwnerForOpenUpload(_ context.Context, intent domainattachment.UploadIntentV1) error {
	for _, disposition := range authority.dispositions {
		if disposition.UploadID == intent.UploadID && disposition.Status != domainattachment.UploadDispositionCommittedV1 {
			return storeport.ErrConflict
		}
	}
	for _, owner := range authority.owners {
		if owner.OwnerDigest == intent.Owner.OwnerDigest {
			return nil
		}
	}
	authority.owners = append(authority.owners, intent.Owner)
	return nil
}

func (authority *attachmentRecoveryAuthority) PutUploadDispositionIfAbsent(_ context.Context, disposition domainattachment.UploadDispositionV1) error {
	for _, existing := range authority.dispositions {
		if existing.UploadID == disposition.UploadID {
			if existing.RecordDigest == disposition.RecordDigest {
				return nil
			}
			return storeport.ErrConflict
		}
	}
	authority.dispositions = append(authority.dispositions, disposition)
	return nil
}

func (authority *attachmentRecoveryAuthority) ResolveUploadDisposition(_ context.Context, uploadID string) (domainattachment.UploadDispositionV1, error) {
	for _, disposition := range authority.dispositions {
		if disposition.UploadID == uploadID {
			return disposition, nil
		}
	}
	return domainattachment.UploadDispositionV1{}, storeport.ErrNotFound
}

func (authority *attachmentRecoveryAuthority) PutUseReceiptIfAbsent(context.Context, domainattachment.AttachmentUseReceiptV1) error {
	return errors.New("read-only test authority")
}

func (authority *attachmentRecoveryAuthority) ResolveUseReceipt(context.Context, string) (domainattachment.AttachmentUseReceiptV1, error) {
	return domainattachment.AttachmentUseReceiptV1{}, errors.New("receipt not found")
}

func (authority *attachmentRecoveryAuthority) PutUseDispositionIfAbsent(context.Context, domainattachment.AttachmentUseDispositionV1) error {
	return errors.New("read-only test authority")
}

func (authority *attachmentRecoveryAuthority) ResolveUseDisposition(context.Context, string) (domainattachment.AttachmentUseDispositionV1, error) {
	return domainattachment.AttachmentUseDispositionV1{}, errors.New("disposition not found")
}

func (authority *attachmentRecoveryAuthority) VisitOwners(_ context.Context, visit func(domainattachment.OwnerRecordV1) error) error {
	for _, owner := range authority.owners {
		if err := visit(owner); err != nil {
			return err
		}
	}
	return nil
}

func (authority *attachmentRecoveryAuthority) VisitUseReceipts(context.Context, func(domainattachment.AttachmentUseReceiptV1) error) error {
	return nil
}

func (authority *attachmentRecoveryAuthority) VisitUseDispositions(context.Context, func(domainattachment.AttachmentUseDispositionV1) error) error {
	return nil
}

func (authority *attachmentRecoveryAuthority) VisitUploadIntents(_ context.Context, visit func(domainattachment.UploadIntentV1) error) error {
	for _, intent := range authority.intents {
		if err := visit(intent); err != nil {
			return err
		}
	}
	return nil
}

func (authority *attachmentRecoveryAuthority) VisitUploadDispositions(_ context.Context, visit func(domainattachment.UploadDispositionV1) error) error {
	for _, disposition := range authority.dispositions {
		if err := visit(disposition); err != nil {
			return err
		}
	}
	return nil
}

func (authority *attachmentRecoveryAuthority) HasRecords(context.Context) (bool, error) {
	return len(authority.owners)+len(authority.intents)+len(authority.dispositions) > 0, nil
}
