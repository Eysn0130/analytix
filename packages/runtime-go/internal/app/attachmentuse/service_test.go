package attachmentuse

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	domainattachment "analytix.local/runtime-go/internal/domain/attachment"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	storeport "analytix.local/runtime-go/internal/ports/attachmentauthority"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestAttachmentUseServiceIssuesVerifiesAndClosesExactBatch(t *testing.T) {
	fixture := newAttachmentUseServiceFixture(t, 2, func(context.Context, domainsecurity.TurnSecurityContext) error { return nil })
	receipts, err := fixture.service.IssueBatch(context.Background(), fixture.input)
	if err != nil || len(receipts) != 2 {
		t.Fatalf("issue batch failed: count=%d err=%v", len(receipts), err)
	}
	if err := fixture.service.VerifyOpenBatch(context.Background(), fixture.input, receipts); err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.RequireNoOpenForFrozenPublicationContext(context.Background(), fixture.input.SecurityContext); !errors.Is(err, ErrOpen) {
		t.Fatalf("open attachment use did not block terminal publication: %v", err)
	}
	dispositions, err := fixture.service.CloseBatch(
		context.Background(), receipts, domainattachment.AttachmentUseDispositionConsumedV1,
		"provider_consumed", fixture.input.IssuedAt.Add(time.Minute),
	)
	if err != nil || len(dispositions) != len(receipts) {
		t.Fatalf("close batch failed: count=%d err=%v", len(dispositions), err)
	}
	if err := fixture.service.VerifyOpenBatch(context.Background(), fixture.input, receipts); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed attachment use remained open: %v", err)
	}
	if err := fixture.service.RequireNoOpenForFrozenPublicationContext(context.Background(), fixture.input.SecurityContext); err != nil {
		t.Fatalf("closed attachment use still blocked terminal publication: %v", err)
	}
	plan, err := fixture.service.PlanRestartDispositions(context.Background())
	if err != nil || len(plan.OpenReceipts) != 0 {
		t.Fatalf("closed use entered restart plan: %#v err=%v", plan, err)
	}
}

func TestAttachmentUseFrozenPublicationGuardDoesNotRepeatCurrentContextValidation(t *testing.T) {
	validations := 0
	fixture := newAttachmentUseServiceFixture(t, 1, func(context.Context, domainsecurity.TurnSecurityContext) error {
		validations++
		return errors.New("case switched")
	})
	if err := fixture.service.RequireNoOpenForFrozenPublicationContext(context.Background(), fixture.input.SecurityContext); err != nil {
		t.Fatalf("closed frozen inventory should allow the stale turn to reach the final gate: %v", err)
	}
	if validations != 0 {
		t.Fatalf("terminal attachment guard repeated current-context validation %d times", validations)
	}
	if receipts, err := fixture.service.IssueBatch(context.Background(), fixture.input); !errors.Is(err, ErrMismatch) || len(receipts) != 0 {
		t.Fatalf("attachment issue must still reject stale context: count=%d err=%v", len(receipts), err)
	}
	if validations != 1 {
		t.Fatalf("effect issue did not validate current context exactly once: %d", validations)
	}
}

func TestAttachmentUseServiceStalePostCommitClosesWithoutEffectAuthority(t *testing.T) {
	validations := 0
	fixture := newAttachmentUseServiceFixture(t, 2, func(context.Context, domainsecurity.TurnSecurityContext) error {
		validations++
		if validations > 1 {
			return errors.New("case switched")
		}
		return nil
	})
	if receipts, err := fixture.service.IssueBatch(context.Background(), fixture.input); err == nil || len(receipts) != 0 {
		t.Fatalf("stale post-commit issue returned effect authority: count=%d err=%v", len(receipts), err)
	}
	plan, err := fixture.service.PlanRestartDispositions(context.Background())
	if err != nil || len(plan.OpenReceipts) != 0 {
		t.Fatalf("stale uses were left open: %#v err=%v", plan, err)
	}
	for _, owner := range fixture.input.Owners {
		useID, err := domainattachment.ComputeAttachmentUseIDV1(domainattachment.AttachmentUseReceiptInputV1{
			SecurityContext: fixture.input.SecurityContext, Owner: owner,
			AttachmentSet: fixture.input.Owners, ProjectionSet: fixture.input.Projections,
			BatchIndex:          uint32(indexOfOwner(fixture.input.Owners, owner.OwnerDigest)),
			EffectBindingDigest: fixture.input.EffectBindingDigest, ConsumerKind: fixture.input.ConsumerKind,
			ConsumerBindingDigest: fixture.input.ConsumerBindingDigest,
		})
		if err != nil {
			t.Fatal(err)
		}
		disposition, err := fixture.store.ResolveUseDisposition(context.Background(), useID)
		if err != nil || disposition.Status != domainattachment.AttachmentUseDispositionStaleContextV1 {
			t.Fatalf("stale use disposition mismatch: %#v err=%v", disposition, err)
		}
	}
}

func TestAttachmentUseServiceRestartInvalidatesOpenReceiptsDeterministically(t *testing.T) {
	fixture := newAttachmentUseServiceFixture(t, 2, func(context.Context, domainsecurity.TurnSecurityContext) error { return nil })
	receipts, err := fixture.service.IssueBatch(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := fixture.service.PlanRestartDispositions(context.Background())
	if err != nil || len(plan.OpenReceipts) != len(receipts) {
		t.Fatalf("restart plan mismatch: %#v err=%v", plan, err)
	}
	for index := 1; index < len(plan.OpenReceipts); index++ {
		if plan.OpenReceipts[index-1].UseID >= plan.OpenReceipts[index].UseID {
			t.Fatal("restart plan is not sorted by stable use identity")
		}
	}
	if err := fixture.service.ApplyRestartDispositions(context.Background(), plan, fixture.input.IssuedAt.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.VerifyOpenBatch(context.Background(), fixture.input, receipts); !errors.Is(err, ErrClosed) {
		t.Fatalf("restart-invalid receipt remained open: %v", err)
	}
}

func TestAttachmentUseServiceRejectsActualProjectionMismatch(t *testing.T) {
	fixture := newAttachmentUseServiceFixture(t, 1, func(context.Context, domainsecurity.TurnSecurityContext) error { return nil })
	receipts, err := fixture.service.IssueBatch(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	mismatched := fixture.input
	mismatched.Projections = append([]domainattachment.AttachmentUseProjectionV1(nil), fixture.input.Projections...)
	mismatched.Projections[0].ProjectionSHA256 = domainsecurity.SHA256Hex([]byte("different-provider-projection"))
	if err := fixture.service.VerifyOpenBatch(context.Background(), mismatched, receipts); !errors.Is(err, ErrMismatch) {
		t.Fatalf("actual provider projection mismatch was accepted: %v", err)
	}
}

type attachmentUseServiceFixture struct {
	service *Service
	store   *attachmentUseMemoryStore
	input   IssueBatchInput
}

type attachmentRestartStoreV1 struct {
	*attachmentUseMemoryStore
	held   string
	checks int
	err    error
}

func (store *attachmentRestartStoreV1) RevalidateAttachmentRestartV1(ctx context.Context) error {
	store.checks++
	return errors.Join(store.err, ctx.Err())
}

func (store *attachmentRestartStoreV1) RestartPreservesThreadV1(id string) bool {
	return id == store.held
}

func TestAttachmentRestartPreservesOpenHeldUsesAndPublicationDenial(t *testing.T) {
	fixture := newAttachmentUseServiceFixture(t, 1, func(context.Context, domainsecurity.TurnSecurityContext) error { return nil })
	held, err := fixture.service.IssueBatch(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	original, err := domainattachment.AttachmentUseReceiptV1Bytes(held[0])
	if err != nil {
		t.Fatal(err)
	}
	preserved := &attachmentRestartStoreV1{attachmentUseMemoryStore: fixture.store, held: fixture.input.SecurityContext.ThreadID}
	service := NewService(fixture.service.authority, preserved, fixture.service.validateCurrent)
	plan, err := service.PlanRestartDispositions(context.Background())
	if err != nil || len(plan.OpenReceipts) != 1 {
		t.Fatalf("held original missing from full open inventory: count=%d err=%v", len(plan.OpenReceipts), err)
	}
	if err := service.ApplyRestartDispositions(context.Background(), plan, fixture.input.IssuedAt.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(fixture.store.dispositions) != 0 {
		t.Error("held original use received a restart disposition")
	}
	if err := service.RequireNoOpenForFrozenPublicationContext(context.Background(), fixture.input.SecurityContext); !errors.Is(err, ErrOpen) {
		t.Errorf("held open use became no-open publication authority: %v", err)
	}
	retained, err := domainattachment.AttachmentUseReceiptV1Bytes(fixture.store.receipts[held[0].UseID])
	if err != nil || !bytes.Equal(original, retained) || preserved.checks == 0 {
		t.Errorf("held original was changed or not revalidated: checks=%d err=%v", preserved.checks, err)
	}
}

func newAttachmentUseServiceFixture(
	t *testing.T,
	count int,
	validate CurrentContextValidator,
	threadIDs ...string,
) attachmentUseServiceFixture {
	t.Helper()
	store := newAttachmentUseMemoryStore()
	authority := newAttachmentUseTestAuthority(17)
	issuedAt := time.Date(2026, 7, 14, 3, 0, 0, 0, time.UTC)
	threadID := "thread-a"
	if len(threadIDs) != 0 {
		threadID = threadIDs[0]
	}
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: "turn-a", WorkspaceRealPath: "/cases/a",
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
	owners := make([]domainattachment.OwnerRecordV1, 0, count)
	projections := make([]domainattachment.AttachmentUseProjectionV1, 0, count)
	for index := 0; index < count; index++ {
		owner, err := domainattachment.NewOwnerRecordV1(domainattachment.OwnerRecordInputV1{
			OwnerNonce: fmt.Sprintf("%032x", index+1), BlobSHA256: domainsecurity.SHA256Hex([]byte(fmt.Sprintf("blob-%d", index))),
			ByteSize: int64(index + 1), MIMEType: "application/pdf", ThreadID: securityContext.ThreadID,
			WorkspaceRealPath: securityContext.WorkspaceRealPath, CaseBindingObservation: &observation,
			ProjectionSHA256: domainsecurity.SHA256Hex([]byte(fmt.Sprintf("upload-metadata-%d", index))), CreatedAt: issuedAt.Add(-time.Hour),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.PutOwnerIfAbsent(context.Background(), owner); err != nil {
			t.Fatal(err)
		}
		projection := []byte(fmt.Sprintf("actual-provider-projection-%d", index))
		owners = append(owners, owner)
		projections = append(projections, domainattachment.AttachmentUseProjectionV1{
			AttachmentID: owner.AttachmentID, ProjectionSHA256: domainsecurity.SHA256Hex(projection), ProjectionByteSize: int64(len(projection)),
		})
	}
	return attachmentUseServiceFixture{
		service: NewService(authority, store, validate), store: store,
		input: IssueBatchInput{
			SecurityContext: securityContext, Owners: owners, Projections: projections,
			EffectBindingDigest:   domainsecurity.SHA256Hex([]byte("provider-attempt")),
			ConsumerKind:          domainattachment.AttachmentUseConsumerPrimaryProviderV1,
			ConsumerBindingDigest: domainsecurity.SHA256Hex([]byte("provider-route")), IssuedAt: issuedAt,
		},
	}
}

func TestAttachmentRestartMixedUsesKeepHeldOpenAndCloseIndependent(t *testing.T) {
	ctx := context.Background()
	held := newAttachmentUseServiceFixture(t, 1, func(context.Context, domainsecurity.TurnSecurityContext) error { return nil })
	independent := newAttachmentUseServiceFixture(t, 1, func(context.Context, domainsecurity.TurnSecurityContext) error { return nil }, "thread-independent")
	heldReceipts, err := held.service.IssueBatch(ctx, held.input)
	if err != nil {
		t.Fatal(err)
	}
	independentReceipts, err := independent.service.IssueBatch(ctx, independent.input)
	if err != nil {
		t.Fatal(err)
	}
	for id, owner := range independent.store.owners {
		held.store.owners[id] = owner
	}
	for id, receipt := range independent.store.receipts {
		held.store.receipts[id] = receipt
	}
	preserved := &attachmentRestartStoreV1{attachmentUseMemoryStore: held.store, held: held.input.SecurityContext.ThreadID}
	service := NewService(held.service.authority, preserved, held.service.validateCurrent)
	plan, err := service.PlanRestartDispositions(ctx)
	if err != nil || len(plan.OpenReceipts) != 2 || len(plan.PreservedUseIDs) != 1 {
		t.Fatalf("mixed original denominator: %v", err)
	}
	if err := service.ApplyRestartDispositions(ctx, plan, held.input.IssuedAt.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(held.store.dispositions) != 1 || held.store.dispositions[independentReceipts[0].UseID].Status != domainattachment.AttachmentUseDispositionRestartInvalidV1 {
		t.Fatal("independent use did not receive the exact restart disposition")
	}
	if _, exists := held.store.dispositions[heldReceipts[0].UseID]; exists {
		t.Fatal("held use received a disposition")
	}
	if err := service.RequireNoOpenForFrozenPublicationContext(ctx, held.input.SecurityContext); !errors.Is(err, ErrOpen) {
		t.Fatalf("held publication no-open check: %v", err)
	}
	if err := service.RequireNoOpenForFrozenPublicationContext(ctx, independent.input.SecurityContext); err != nil {
		t.Fatalf("independent publication retained false open use: %v", err)
	}
}

func indexOfOwner(owners []domainattachment.OwnerRecordV1, digest string) int {
	for index, owner := range owners {
		if owner.OwnerDigest == digest {
			return index
		}
	}
	return -1
}

type attachmentUseTestAuthority struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	keyID      string
}

func newAttachmentUseTestAuthority(seed byte) *attachmentUseTestAuthority {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{seed}, ed25519.SeedSize))
	publicKey := append(ed25519.PublicKey(nil), privateKey.Public().(ed25519.PublicKey)...)
	return &attachmentUseTestAuthority{
		privateKey: privateKey,
		publicKey:  publicKey,
		keyID:      domainsecurity.SHA256Hex(publicKey),
	}
}

func (authority *attachmentUseTestAuthority) KeyID() string { return authority.keyID }

func (authority *attachmentUseTestAuthority) PublicKey() []byte {
	return append([]byte(nil), authority.publicKey...)
}

func (authority *attachmentUseTestAuthority) Sign(_ context.Context, body []byte) ([]byte, error) {
	return ed25519.Sign(authority.privateKey, body), nil
}

func (authority *attachmentUseTestAuthority) VerifyTrusted(
	_ context.Context,
	keyID string,
	publicKey, body, signature []byte,
) error {
	if keyID != authority.keyID || !bytes.Equal(publicKey, authority.publicKey) || !ed25519.Verify(publicKey, body, signature) {
		return errors.New("untrusted attachment use authority")
	}
	return nil
}

type attachmentUseMemoryStore struct {
	owners       map[string]domainattachment.OwnerRecordV1
	receipts     map[string]domainattachment.AttachmentUseReceiptV1
	dispositions map[string]domainattachment.AttachmentUseDispositionV1
}

func newAttachmentUseMemoryStore() *attachmentUseMemoryStore {
	return &attachmentUseMemoryStore{
		owners:       map[string]domainattachment.OwnerRecordV1{},
		receipts:     map[string]domainattachment.AttachmentUseReceiptV1{},
		dispositions: map[string]domainattachment.AttachmentUseDispositionV1{},
	}
}

func (store *attachmentUseMemoryStore) PutOwnerIfAbsent(_ context.Context, owner domainattachment.OwnerRecordV1) error {
	if existing, ok := store.owners[owner.OwnerDigest]; ok {
		if existing.OwnerDigest != owner.OwnerDigest || existing.BlobSHA256 != owner.BlobSHA256 {
			return storeport.ErrConflict
		}
		return nil
	}
	store.owners[owner.OwnerDigest] = owner
	return nil
}

func (store *attachmentUseMemoryStore) ResolveOwner(_ context.Context, digest string) (domainattachment.OwnerRecordV1, error) {
	owner, ok := store.owners[digest]
	if !ok {
		return domainattachment.OwnerRecordV1{}, storeport.ErrNotFound
	}
	return owner, nil
}

func (store *attachmentUseMemoryStore) PutUseReceiptIfAbsent(_ context.Context, receipt domainattachment.AttachmentUseReceiptV1) error {
	if existing, ok := store.receipts[receipt.UseID]; ok {
		if existing.ReceiptDigest != receipt.ReceiptDigest {
			return storeport.ErrConflict
		}
		return nil
	}
	store.receipts[receipt.UseID] = receipt
	return nil
}

func (store *attachmentUseMemoryStore) ResolveUseReceipt(_ context.Context, useID string) (domainattachment.AttachmentUseReceiptV1, error) {
	receipt, ok := store.receipts[useID]
	if !ok {
		return domainattachment.AttachmentUseReceiptV1{}, storeport.ErrNotFound
	}
	return receipt, nil
}

func (store *attachmentUseMemoryStore) PutUseDispositionIfAbsent(_ context.Context, disposition domainattachment.AttachmentUseDispositionV1) error {
	if existing, ok := store.dispositions[disposition.UseID]; ok {
		if existing.RecordDigest != disposition.RecordDigest {
			return storeport.ErrConflict
		}
		return nil
	}
	store.dispositions[disposition.UseID] = disposition
	return nil
}

func (store *attachmentUseMemoryStore) ResolveUseDisposition(_ context.Context, useID string) (domainattachment.AttachmentUseDispositionV1, error) {
	disposition, ok := store.dispositions[useID]
	if !ok {
		return domainattachment.AttachmentUseDispositionV1{}, storeport.ErrNotFound
	}
	return disposition, nil
}

func (store *attachmentUseMemoryStore) VisitOwners(_ context.Context, visit func(domainattachment.OwnerRecordV1) error) error {
	keys := sortedAttachmentUseKeys(store.owners)
	for _, key := range keys {
		if err := visit(store.owners[key]); err != nil {
			return err
		}
	}
	return nil
}

func (store *attachmentUseMemoryStore) VisitUseReceipts(_ context.Context, visit func(domainattachment.AttachmentUseReceiptV1) error) error {
	keys := sortedAttachmentUseKeys(store.receipts)
	for _, key := range keys {
		if err := visit(store.receipts[key]); err != nil {
			return err
		}
	}
	return nil
}

func (store *attachmentUseMemoryStore) VisitUseDispositions(_ context.Context, visit func(domainattachment.AttachmentUseDispositionV1) error) error {
	keys := sortedAttachmentUseKeys(store.dispositions)
	for _, key := range keys {
		if err := visit(store.dispositions[key]); err != nil {
			return err
		}
	}
	return nil
}

func (store *attachmentUseMemoryStore) HasRecords(context.Context) (bool, error) {
	return len(store.owners)+len(store.receipts)+len(store.dispositions) > 0, nil
}

func sortedAttachmentUseKeys[T any](records map[string]T) []string {
	keys := make([]string, 0, len(records))
	for key := range records {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
