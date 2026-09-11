package attachmentauthority

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	domainattachment "analytix.local/runtime-go/internal/domain/attachment"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	storeport "analytix.local/runtime-go/internal/ports/attachmentauthority"
	attachmentuploadport "analytix.local/runtime-go/internal/ports/attachmentupload"
)

func TestAttachmentOwnerServiceRequiresExactThreadWorkspaceAndPrivateMembership(t *testing.T) {
	workspace := "/cases/a"
	observation := attachmentServiceObservation(t, workspace, "case-a", "binding-a")
	owners := &attachmentOwnerMemoryStore{owners: map[string]domainattachment.OwnerRecordV1{}}
	service := &Service{
		Threads:  attachmentThreadReaderStub{thread: map[string]any{"id": "thread-a", "workspace": workspace}},
		Bindings: &attachmentBindingStub{workspace: workspace, observation: observation},
		Owners:   owners,
	}
	prepared, err := service.PrepareUpload(map[string]any{
		"name": "statement.pdf", "threadId": "thread-a", "workspace": workspace,
	})
	if err != nil || prepared["threadId"] != "thread-a" || prepared["workspace"] != workspace {
		t.Fatalf("exact upload scope was not prepared: %#v err=%v", prepared, err)
	}
	for name, body := range map[string]map[string]any{
		"missing workspace": {"threadId": "thread-a"},
		"wrong workspace":   {"threadId": "thread-a", "workspace": "/cases/b"},
		"wrong thread":      {"threadId": "thread-b", "workspace": workspace},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := service.PrepareUpload(body); err == nil {
				t.Fatal("invalid upload scope was accepted")
			}
		})
	}

	owner := attachmentServiceOwner(t, observation)
	metadata := attachmentServiceMetadata(t, owner)
	if service.AuthorizeContent(context.Background(), metadata, "thread-a", workspace) {
		t.Fatal("self-declared embedded owner gained authority without private membership")
	}
	if err := service.CommitUpload(context.Background(), metadata); err != nil {
		t.Fatal(err)
	}
	if !service.AuthorizeContent(context.Background(), metadata, "thread-a", workspace) {
		t.Fatal("exact privately committed owner was rejected")
	}
	service.Bindings.(*attachmentBindingStub).observation = attachmentServiceObservation(t, workspace, "case-b", "binding-b")
	if service.AuthorizeContent(context.Background(), metadata, "thread-a", workspace) {
		t.Fatal("old-case attachment remained readable after a same-thread case switch")
	}
}

func TestAttachmentOwnerServiceRejectsAuthorityReadbackMismatch(t *testing.T) {
	workspace := "/cases/a"
	observation := attachmentServiceObservation(t, workspace, "case-a", "binding-a")
	owner := attachmentServiceOwner(t, observation)
	owners := &attachmentOwnerMemoryStore{owners: map[string]domainattachment.OwnerRecordV1{}, replaceOnRead: true}
	service := &Service{
		Threads:  attachmentThreadReaderStub{thread: map[string]any{"id": "thread-a", "workspace": workspace}},
		Bindings: &attachmentBindingStub{workspace: workspace, observation: observation}, Owners: owners,
	}
	if err := service.CommitUpload(context.Background(), attachmentServiceMetadata(t, owner)); err == nil {
		t.Fatal("mismatched private owner readback was accepted")
	}
}

func TestAttachmentOwnerServiceRejectsCaseSwitchBeforeOwnerCommit(t *testing.T) {
	workspace := "/cases/a"
	before := attachmentServiceObservation(t, workspace, "case-a", "binding-a")
	owners := &attachmentOwnerMemoryStore{owners: map[string]domainattachment.OwnerRecordV1{}}
	bindings := &attachmentBindingStub{workspace: workspace, observation: before}
	service := &Service{
		Threads:  attachmentThreadReaderStub{thread: map[string]any{"id": "thread-a", "workspace": workspace}},
		Bindings: bindings,
		Owners:   owners,
	}
	owner := attachmentServiceOwner(t, before)
	bindings.observation = attachmentServiceObservation(t, workspace, "case-b", "binding-b")
	if err := service.CommitUpload(context.Background(), attachmentServiceMetadata(t, owner)); err == nil {
		t.Fatal("stale case attachment owner was committed after a case switch")
	}
	if len(owners.owners) != 0 {
		t.Fatalf("rejected stale owner mutated private authority: %#v", owners.owners)
	}
}

func TestAttachmentUploadCommitsIntentFilesOwnerDispositionInOrder(t *testing.T) {
	workspace := "/cases/a"
	observation := attachmentServiceObservation(t, workspace, "case-a", "binding-a")
	owner := attachmentServiceOwner(t, observation)
	metadata := attachmentServiceMetadata(t, owner)
	events := []string{}
	owners := &attachmentOwnerMemoryStore{
		owners: map[string]domainattachment.OwnerRecordV1{}, intents: map[string]domainattachment.UploadIntentV1{},
		uploadDispositions: map[string]domainattachment.UploadDispositionV1{}, events: &events,
	}
	prepared := &attachmentPreparedUploadStub{
		owner: owner, metadata: metadata, metadataSHA256: domainsecurity.SHA256Hex([]byte("metadata bytes")), events: &events,
	}
	service := &Service{
		Threads:  attachmentThreadReaderStub{thread: map[string]any{"id": "thread-a", "workspace": workspace}},
		Bindings: &attachmentBindingStub{workspace: workspace, observation: observation}, Owners: owners, Uploads: owners,
	}
	result, err := service.Upload(context.Background(), map[string]any{
		"name": "statement.pdf", "threadId": "thread-a", "workspace": workspace,
	}, attachmentUploadStoreStub{prepared: prepared})
	if err != nil || result["id"] != owner.AttachmentID {
		t.Fatalf("attachment upload failed: result=%#v err=%v", result, err)
	}
	expected := []string{"intent", "files", "owner", "disposition:committed"}
	if !equalAttachmentEvents(events, expected) {
		t.Fatalf("upload transaction order = %#v, want %#v", events, expected)
	}
	if len(owners.uploadDispositions) != 1 {
		t.Fatalf("committed upload disposition missing: %#v", owners.uploadDispositions)
	}
}

func TestCaseSwitchBeforeOwnerCommitRejectsAndAbortsUpload(t *testing.T) {
	workspace := "/cases/a"
	before := attachmentServiceObservation(t, workspace, "case-a", "binding-a")
	bindings := &attachmentBindingStub{workspace: workspace, observation: before}
	owner := attachmentServiceOwner(t, before)
	events := []string{}
	owners := &attachmentOwnerMemoryStore{
		owners: map[string]domainattachment.OwnerRecordV1{}, intents: map[string]domainattachment.UploadIntentV1{},
		uploadDispositions: map[string]domainattachment.UploadDispositionV1{}, events: &events,
	}
	prepared := &attachmentPreparedUploadStub{
		owner: owner, metadata: attachmentServiceMetadata(t, owner), metadataSHA256: domainsecurity.SHA256Hex([]byte("metadata bytes")),
		events: &events, afterCommit: func() {
			bindings.observation = attachmentServiceObservation(t, workspace, "case-b", "binding-b")
		},
	}
	service := &Service{
		Threads:  attachmentThreadReaderStub{thread: map[string]any{"id": "thread-a", "workspace": workspace}},
		Bindings: bindings, Owners: owners, Uploads: owners,
	}
	if _, err := service.Upload(context.Background(), map[string]any{
		"name": "statement.pdf", "threadId": "thread-a", "workspace": workspace,
	}, attachmentUploadStoreStub{prepared: prepared}); err == nil {
		t.Fatal("case switch before owner commit was accepted")
	}
	if len(owners.owners) != 0 || len(owners.uploadDispositions) != 1 || !prepared.aborted {
		t.Fatalf("rejected upload did not converge safely: owners=%#v dispositions=%#v aborted=%v", owners.owners, owners.uploadDispositions, prepared.aborted)
	}
	for _, disposition := range owners.uploadDispositions {
		if disposition.Status != domainattachment.UploadDispositionRejectedV1 {
			t.Fatalf("case-switched upload status = %q", disposition.Status)
		}
	}
}

func TestAttachmentUploadCancellationCannotLeaveExecutableIndeterminateState(t *testing.T) {
	workspace := "/cases/a"
	observation := attachmentServiceObservation(t, workspace, "case-a", "binding-a")
	owner := attachmentServiceOwner(t, observation)
	events := []string{}
	ctx, cancel := context.WithCancel(context.Background())
	owners := &attachmentOwnerMemoryStore{
		owners: map[string]domainattachment.OwnerRecordV1{}, intents: map[string]domainattachment.UploadIntentV1{},
		uploadDispositions: map[string]domainattachment.UploadDispositionV1{}, events: &events, afterIntent: cancel,
	}
	prepared := &attachmentPreparedUploadStub{
		owner: owner, metadata: attachmentServiceMetadata(t, owner), metadataSHA256: domainsecurity.SHA256Hex([]byte("metadata bytes")), events: &events,
	}
	service := &Service{
		Threads:  attachmentThreadReaderStub{thread: map[string]any{"id": "thread-a", "workspace": workspace}},
		Bindings: &attachmentBindingStub{workspace: workspace, observation: observation}, Owners: owners, Uploads: owners,
	}
	if _, err := service.Upload(ctx, map[string]any{
		"name": "statement.pdf", "threadId": "thread-a", "workspace": workspace,
	}, attachmentUploadStoreStub{prepared: prepared}); err == nil {
		t.Fatal("cancelled upload unexpectedly succeeded")
	}
	if len(owners.owners) != 0 || len(owners.uploadDispositions) != 1 || !prepared.aborted {
		t.Fatalf("cancelled upload remained executable or indeterminate: %#v %#v", owners.owners, owners.uploadDispositions)
	}
}

type attachmentThreadReaderStub struct{ thread map[string]any }

func (stub attachmentThreadReaderStub) GetThread(id string) (map[string]any, error) {
	if stub.thread["id"] != id {
		return nil, errors.New("not found")
	}
	return stub.thread, nil
}

type attachmentBindingStub struct {
	workspace   string
	observation domainsecurity.CaseBindingObservationV1
}

func (stub *attachmentBindingStub) WorkspaceRealPath(workspace string) (string, error) {
	if workspace != stub.workspace {
		return "", errors.New("workspace mismatch")
	}
	return stub.workspace, nil
}

func (stub *attachmentBindingStub) Observe(workspace string) (domainsecurity.CaseBindingObservationV1, error) {
	if workspace != stub.workspace {
		return domainsecurity.CaseBindingObservationV1{}, errors.New("workspace mismatch")
	}
	return stub.observation, nil
}

type attachmentOwnerMemoryStore struct {
	owners             map[string]domainattachment.OwnerRecordV1
	intents            map[string]domainattachment.UploadIntentV1
	uploadDispositions map[string]domainattachment.UploadDispositionV1
	replaceOnRead      bool
	events             *[]string
	afterIntent        func()
}

func (store *attachmentOwnerMemoryStore) PutOwnerIfAbsent(_ context.Context, owner domainattachment.OwnerRecordV1) error {
	if store.events != nil {
		*store.events = append(*store.events, "owner")
	}
	store.owners[owner.OwnerDigest] = owner
	return nil
}

func (store *attachmentOwnerMemoryStore) PutUploadIntentIfAbsent(_ context.Context, intent domainattachment.UploadIntentV1) error {
	if store.intents == nil {
		store.intents = map[string]domainattachment.UploadIntentV1{}
	}
	if store.events != nil {
		*store.events = append(*store.events, "intent")
	}
	store.intents[intent.UploadID] = intent
	if store.afterIntent != nil {
		store.afterIntent()
	}
	return nil
}

func (store *attachmentOwnerMemoryStore) ResolveUploadIntent(_ context.Context, uploadID string) (domainattachment.UploadIntentV1, error) {
	intent, ok := store.intents[uploadID]
	if !ok {
		return domainattachment.UploadIntentV1{}, storeport.ErrNotFound
	}
	return intent, nil
}

func (store *attachmentOwnerMemoryStore) CommitOwnerForOpenUpload(_ context.Context, intent domainattachment.UploadIntentV1) error {
	if _, ok := store.intents[intent.UploadID]; !ok {
		return storeport.ErrNotFound
	}
	if disposition, ok := store.uploadDispositions[intent.UploadID]; ok && disposition.Status != domainattachment.UploadDispositionCommittedV1 {
		return storeport.ErrConflict
	}
	return store.PutOwnerIfAbsent(context.Background(), intent.Owner)
}

func (store *attachmentOwnerMemoryStore) PutUploadDispositionIfAbsent(_ context.Context, disposition domainattachment.UploadDispositionV1) error {
	if store.uploadDispositions == nil {
		store.uploadDispositions = map[string]domainattachment.UploadDispositionV1{}
	}
	if existing, ok := store.uploadDispositions[disposition.UploadID]; ok && existing.RecordDigest != disposition.RecordDigest {
		return storeport.ErrConflict
	}
	if store.events != nil {
		*store.events = append(*store.events, "disposition:"+disposition.Status)
	}
	store.uploadDispositions[disposition.UploadID] = disposition
	return nil
}

func (store *attachmentOwnerMemoryStore) ResolveUploadDisposition(_ context.Context, uploadID string) (domainattachment.UploadDispositionV1, error) {
	disposition, ok := store.uploadDispositions[uploadID]
	if !ok {
		return domainattachment.UploadDispositionV1{}, storeport.ErrNotFound
	}
	return disposition, nil
}

func (store *attachmentOwnerMemoryStore) ResolveOwner(_ context.Context, digest string) (domainattachment.OwnerRecordV1, error) {
	owner, ok := store.owners[digest]
	if !ok {
		return domainattachment.OwnerRecordV1{}, storeport.ErrNotFound
	}
	if store.replaceOnRead {
		owner.BlobSHA256 = domainsecurity.SHA256Hex([]byte("different"))
	}
	return owner, nil
}

func (*attachmentOwnerMemoryStore) PutUseReceiptIfAbsent(context.Context, domainattachment.AttachmentUseReceiptV1) error {
	return errors.New("unsupported")
}

func (*attachmentOwnerMemoryStore) ResolveUseReceipt(context.Context, string) (domainattachment.AttachmentUseReceiptV1, error) {
	return domainattachment.AttachmentUseReceiptV1{}, storeport.ErrNotFound
}

func (*attachmentOwnerMemoryStore) PutUseDispositionIfAbsent(context.Context, domainattachment.AttachmentUseDispositionV1) error {
	return errors.New("unsupported")
}

func (*attachmentOwnerMemoryStore) ResolveUseDisposition(context.Context, string) (domainattachment.AttachmentUseDispositionV1, error) {
	return domainattachment.AttachmentUseDispositionV1{}, storeport.ErrNotFound
}

type attachmentUploadStoreStub struct{ prepared *attachmentPreparedUploadStub }

func (store attachmentUploadStoreStub) Prepare(context.Context, map[string]any) (attachmentuploadport.PreparedUpload, error) {
	return store.prepared, nil
}

type attachmentPreparedUploadStub struct {
	owner          domainattachment.OwnerRecordV1
	metadata       map[string]any
	metadataSHA256 string
	events         *[]string
	afterCommit    func()
	aborted        bool
}

func (prepared *attachmentPreparedUploadStub) Owner() domainattachment.OwnerRecordV1 {
	return prepared.owner
}
func (prepared *attachmentPreparedUploadStub) Metadata() map[string]any { return prepared.metadata }
func (prepared *attachmentPreparedUploadStub) MetadataSHA256() string   { return prepared.metadataSHA256 }
func (prepared *attachmentPreparedUploadStub) Commit(ctx context.Context) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if prepared.events != nil {
		*prepared.events = append(*prepared.events, "files")
	}
	if prepared.afterCommit != nil {
		prepared.afterCommit()
	}
	return prepared.metadata, nil
}
func (prepared *attachmentPreparedUploadStub) Abort(context.Context) error {
	prepared.aborted = true
	if prepared.events != nil {
		*prepared.events = append(*prepared.events, "abort")
	}
	return nil
}

func equalAttachmentEvents(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func attachmentServiceObservation(t *testing.T, workspace, caseID, seed string) domainsecurity.CaseBindingObservationV1 {
	t.Helper()
	observation, err := domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: workspace, State: domainsecurity.CaseBindingStateValid, CaseID: caseID,
		BindingSHA256:   domainsecurity.SHA256Hex([]byte("document:" + seed)),
		CaseBindingHash: domainsecurity.SHA256Hex([]byte(seed)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

func attachmentServiceOwner(t *testing.T, observation domainsecurity.CaseBindingObservationV1) domainattachment.OwnerRecordV1 {
	t.Helper()
	owner, err := domainattachment.NewOwnerRecordV1(domainattachment.OwnerRecordInputV1{
		OwnerNonce: "00000000000000000000000000000001", BlobSHA256: domainsecurity.SHA256Hex([]byte("blob")),
		ByteSize: 4, MIMEType: "application/pdf", ThreadID: "thread-a", WorkspaceRealPath: observation.WorkspaceRealPath,
		CaseBindingObservation: &observation, ProjectionSHA256: domainsecurity.SHA256Hex([]byte("metadata")),
		CreatedAt: time.Date(2026, 7, 14, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return owner
}

func attachmentServiceMetadata(t *testing.T, owner domainattachment.OwnerRecordV1) map[string]any {
	t.Helper()
	body, err := domainattachment.OwnerRecordV1Bytes(owner)
	if err != nil {
		t.Fatal(err)
	}
	var ownerMap map[string]any
	if err := json.Unmarshal(body, &ownerMap); err != nil {
		t.Fatal(err)
	}
	return map[string]any{
		"id": owner.AttachmentID, "name": "statement.pdf", "kind": "document", "mimeType": owner.MIMEType,
		"byteSize": float64(owner.ByteSize), "hash": owner.BlobSHA256, "scope": "thread",
		"threadIds": []any{owner.ThreadID}, "workspaces": []any{owner.WorkspaceRealPath}, "ownerRecord": ownerMap,
	}
}
