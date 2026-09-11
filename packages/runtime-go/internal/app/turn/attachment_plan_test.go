package turn

import (
	"context"
	"errors"
	"testing"
	"time"

	domainattachment "analytix.local/runtime-go/internal/domain/attachment"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	storeport "analytix.local/runtime-go/internal/ports/attachmentauthority"
)

func TestAttachmentPlanRequiresPrivateOwnerAndDefersAllContentReads(t *testing.T) {
	securityContext := newTurnCaseExecutionContextV2(t, "thread_a", "turn_a", "/workspace/a", 1, time.Now().UTC())
	id, record := caseAttachmentRecord(t, securityContext, "statement.pdf", "application/pdf", []byte("private statement"), map[string]any{
		"documentText":  "account 6222020000000000000",
		"localFilePath": "/private/statement.pdf",
	})
	owner, err := AttachmentOwnerAuthorityRecord(record.metadata)
	if err != nil {
		t.Fatal(err)
	}
	store := &attachmentPlanStoreStub{records: map[string]attachmentRecordStub{id: record}}
	owners := newAttachmentPlanOwnerStore(owner)
	planner := AttachmentPlanner{Store: store, Owners: owners}
	plan, err := planner.Plan(context.Background(), AttachmentPlanInput{
		IDs: []string{id}, SecurityContext: securityContext,
		ModelInputModalities: []string{"text"}, ModelMessageParts: []string{"text"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if store.metadataReads != 1 || store.contentReads != 0 {
		t.Fatalf("plan crossed the content boundary: metadata=%d content=%d", store.metadataReads, store.contentReads)
	}
	if plan.Empty() || !domainsecurity.IsSHA256Hex(plan.PlanDigest) || len(plan.Items) != 1 ||
		plan.Items[0].Owner.OwnerDigest != owner.OwnerDigest || len(plan.ProjectionSet()) != 1 {
		t.Fatalf("private-authority-backed plan mismatch: %#v", plan)
	}
	if _, leaked := plan.Metadata[0]["documentText"]; leaked {
		t.Fatalf("private document text leaked into retained plan: %#v", plan.Metadata[0])
	}
	if _, leaked := plan.Metadata[0]["localFilePath"]; leaked {
		t.Fatalf("private local path leaked into retained plan: %#v", plan.Metadata[0])
	}
	materialized, err := planner.Materialize(context.Background(), AttachmentMaterializeInput{Plan: plan, SecurityContext: securityContext})
	if err != nil {
		t.Fatal(err)
	}
	if store.contentReads != 1 || len(materialized.MessageParts) != 1 || materialized.MessageParts[0].Text == "" {
		t.Fatalf("authorized materialization mismatch: reads=%d result=%#v", store.contentReads, materialized)
	}
}

func TestAttachmentPlanRejectsMissingPrivateOwnerBeforeContentRead(t *testing.T) {
	securityContext := newTurnCaseExecutionContextV2(t, "thread_a", "turn_a", "/workspace/a", 1, time.Now().UTC())
	id, record := caseAttachmentRecord(t, securityContext, "statement.txt", "text/plain", []byte("private"), nil)
	store := &attachmentPlanStoreStub{records: map[string]attachmentRecordStub{id: record}}
	planner := AttachmentPlanner{Store: store, Owners: newAttachmentPlanOwnerStore()}
	_, err := planner.Plan(context.Background(), AttachmentPlanInput{
		IDs: []string{id}, SecurityContext: securityContext,
		ModelInputModalities: []string{"text"}, ModelMessageParts: []string{"text"},
	})
	if !errors.Is(err, ErrAttachmentNotAuthorized) || store.contentReads != 0 {
		t.Fatalf("missing private owner did not fail before content read: err=%v reads=%d", err, store.contentReads)
	}
}

func TestAttachmentMaterializationRejectsProjectionTamperBeforeContentRead(t *testing.T) {
	securityContext := newTurnCaseExecutionContextV2(t, "thread_a", "turn_a", "/workspace/a", 1, time.Now().UTC())
	id, record := caseAttachmentRecord(t, securityContext, "statement.txt", "text/plain", []byte("private"), nil)
	owner, err := AttachmentOwnerAuthorityRecord(record.metadata)
	if err != nil {
		t.Fatal(err)
	}
	store := &attachmentPlanStoreStub{records: map[string]attachmentRecordStub{id: record}}
	planner := AttachmentPlanner{Store: store, Owners: newAttachmentPlanOwnerStore(owner)}
	plan, err := planner.Plan(context.Background(), AttachmentPlanInput{
		IDs: []string{id}, SecurityContext: securityContext,
		ModelInputModalities: []string{"text"}, ModelMessageParts: []string{"text"},
	})
	if err != nil {
		t.Fatal(err)
	}
	plan.Items[0].ProjectionSHA256 = domainsecurity.SHA256Hex([]byte("forged projection"))
	if _, err := planner.Materialize(context.Background(), AttachmentMaterializeInput{Plan: plan, SecurityContext: securityContext}); !errors.Is(err, ErrAttachmentNotAuthorized) || store.contentReads != 0 {
		t.Fatalf("tampered projection reached content: err=%v reads=%d", err, store.contentReads)
	}
}

type attachmentPlanStoreStub struct {
	records       map[string]attachmentRecordStub
	metadataReads int
	contentReads  int
}

func (store *attachmentPlanStoreStub) Metadata(id string) (map[string]any, bool, error) {
	store.metadataReads++
	record, ok := store.records[id]
	if !ok || !record.found {
		return nil, false, nil
	}
	return record.metadata, true, nil
}

func (store *attachmentPlanStoreStub) Content(id string) (map[string]any, string, bool, error) {
	store.contentReads++
	record, ok := store.records[id]
	if !ok || !record.found {
		return nil, "", false, nil
	}
	return record.metadata, record.dataBase64, true, nil
}

type attachmentPlanOwnerStore struct {
	owners map[string]domainattachment.OwnerRecordV1
}

func newAttachmentPlanOwnerStore(owners ...domainattachment.OwnerRecordV1) *attachmentPlanOwnerStore {
	store := &attachmentPlanOwnerStore{owners: map[string]domainattachment.OwnerRecordV1{}}
	for _, owner := range owners {
		store.owners[owner.OwnerDigest] = owner
	}
	return store
}

func (store *attachmentPlanOwnerStore) PutOwnerIfAbsent(_ context.Context, owner domainattachment.OwnerRecordV1) error {
	store.owners[owner.OwnerDigest] = owner
	return nil
}

func (store *attachmentPlanOwnerStore) ResolveOwner(_ context.Context, digest string) (domainattachment.OwnerRecordV1, error) {
	owner, ok := store.owners[digest]
	if !ok {
		return domainattachment.OwnerRecordV1{}, storeport.ErrNotFound
	}
	return owner, nil
}

func (*attachmentPlanOwnerStore) PutUseReceiptIfAbsent(context.Context, domainattachment.AttachmentUseReceiptV1) error {
	return errors.New("unsupported")
}

func (*attachmentPlanOwnerStore) ResolveUseReceipt(context.Context, string) (domainattachment.AttachmentUseReceiptV1, error) {
	return domainattachment.AttachmentUseReceiptV1{}, storeport.ErrNotFound
}

func (*attachmentPlanOwnerStore) PutUseDispositionIfAbsent(context.Context, domainattachment.AttachmentUseDispositionV1) error {
	return errors.New("unsupported")
}

func (*attachmentPlanOwnerStore) ResolveUseDisposition(context.Context, string) (domainattachment.AttachmentUseDispositionV1, error) {
	return domainattachment.AttachmentUseDispositionV1{}, storeport.ErrNotFound
}
