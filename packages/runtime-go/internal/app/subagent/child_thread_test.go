package subagent

import (
	"errors"
	"strings"
	"testing"

	domainjob "analytix.local/runtime-go/internal/domain/job"
)

type childThreadStoreStub struct {
	forkThreadID string
	forkRequest  map[string]any
	createPatch  map[string]any
	createRoot   string
	patchThread  string
	patchPatch   map[string]any
	createResult map[string]any
	forkResult   map[string]any
	err          error
}

func (stub *childThreadStoreStub) ForkThread(threadID string, request map[string]any) (map[string]any, error) {
	stub.forkThreadID = threadID
	stub.forkRequest = request
	if stub.err != nil {
		return nil, stub.err
	}
	if stub.forkResult != nil {
		return stub.forkResult, nil
	}
	return map[string]any{"id": "thread-fork"}, nil
}

func (stub *childThreadStoreStub) CreateThread(request map[string]any, fallbackWorkspace string) (map[string]any, error) {
	stub.createPatch = request
	stub.createRoot = fallbackWorkspace
	if stub.err != nil {
		return nil, stub.err
	}
	if stub.createResult != nil {
		return stub.createResult, nil
	}
	return map[string]any{"id": "thread-child"}, nil
}

func (stub *childThreadStoreStub) PatchThread(threadID string, patch map[string]any) (map[string]any, error) {
	stub.patchThread = threadID
	stub.patchPatch = patch
	if stub.err != nil {
		return nil, stub.err
	}
	return map[string]any{"id": threadID}, nil
}

func TestPrepareChildThreadContinuesSameParentSource(t *testing.T) {
	store := &childThreadStoreStub{}
	threadID, err := PrepareChildThread(PrepareChildThreadInput{
		Store:          store,
		ParentThreadID: "thread-parent",
		Request:        TaskRequest{ContinueFrom: "child-run-1"},
		HasSource:      true,
		Source:         domainjob.Record{ParentThreadID: "thread-parent", ChildThreadID: "thread-child"},
	})
	if err != nil {
		t.Fatalf("prepare child thread: %v", err)
	}
	if threadID != "thread-child" {
		t.Fatalf("continued thread id mismatch: %q", threadID)
	}
	if store.createPatch != nil || store.forkRequest != nil {
		t.Fatalf("continue same-parent source should not create/fork: %#v %#v", store.createPatch, store.forkRequest)
	}
}

func TestPrepareChildThreadForksReferencedSource(t *testing.T) {
	store := &childThreadStoreStub{forkResult: map[string]any{"id": "thread-forked"}}
	threadID, err := PrepareChildThread(PrepareChildThreadInput{
		Store:                 store,
		ParentThreadID:        "thread-parent",
		PendingCallID:         "call-1",
		Request:               TaskRequest{ForkFrom: "child-run-1", Name: "Reviewer"},
		HasSource:             true,
		Source:                domainjob.Record{ParentThreadID: "thread-other", ChildThreadID: "thread-source"},
		DistinctNameCandidate: "Reviewer",
	})
	if err != nil {
		t.Fatalf("prepare forked child thread: %v", err)
	}
	if threadID != "thread-forked" || store.forkThreadID != "thread-source" {
		t.Fatalf("fork mismatch thread=%q source=%q", threadID, store.forkThreadID)
	}
	if store.forkRequest["relation"] != "side" || store.forkRequest["parentThreadId"] != "thread-parent" || store.forkRequest["title"] != "Child agent: Reviewer fork" {
		t.Fatalf("fork request mismatch: %#v", store.forkRequest)
	}
}

func TestPrepareChildThreadRejectsSecurityBoundContinueAndForkBeforeTranscriptRead(t *testing.T) {
	for _, request := range []TaskRequest{{ContinueFrom: "child-run-1"}, {ForkFrom: "child-run-1"}} {
		store := &childThreadStoreStub{}
		_, err := PrepareChildThread(PrepareChildThreadInput{
			Store: store, ParentThreadID: "thread-parent", Request: request, HasSource: true,
			Source: domainjob.Record{ParentThreadID: "thread-parent", ChildThreadID: "thread-secret", SecurityBinding: &domainjob.SecurityBinding{Version: 999}},
		})
		if err == nil || err.Error() != "security_bound_source_output_unavailable" || store.forkRequest != nil || store.createPatch != nil {
			t.Fatalf("security-bound source was reused: request=%#v err=%v store=%#v", request, err, store)
		}
	}
}

func TestPrepareChildThreadCreatesSideThread(t *testing.T) {
	store := &childThreadStoreStub{createResult: map[string]any{"id": "thread-new"}}
	threadID, err := PrepareChildThread(PrepareChildThreadInput{
		Store:                 store,
		ParentThreadID:        "thread-parent",
		PendingCallID:         "call-1",
		Request:               TaskRequest{Name: "Reviewer", ToolPolicy: "readOnly"},
		DistinctNameCandidate: "Reviewer",
		ProviderID:            "deepseek",
		Model:                 "deepseek-chat",
		EndpointFormat:        "chat_completions",
		Effort:                "high",
		Workspace:             "/workspace",
		ApprovalPolicy:        "on-request",
		SandboxMode:           "workspace-write",
	})
	if err != nil {
		t.Fatalf("prepare new child thread: %v", err)
	}
	if threadID != "thread-new" || store.patchThread != "thread-new" || store.patchPatch["relation"] != "side" || store.patchPatch["parentThreadId"] != "thread-parent" {
		t.Fatalf("create/patch mismatch thread=%q patch=%#v", threadID, store.patchPatch)
	}
	if store.createRoot != "/workspace" ||
		store.createPatch["title"] != "Child agent: Reviewer" ||
		store.createPatch["approvalPolicy"] != "never" ||
		store.createPatch["relation"] != "side" ||
		store.createPatch["parentThreadId"] != "thread-parent" {
		t.Fatalf("create patch mismatch root=%q patch=%#v", store.createRoot, store.createPatch)
	}
}

func TestPrepareChildThreadRejectsInvalidStoreResults(t *testing.T) {
	if _, err := PrepareChildThread(PrepareChildThreadInput{}); err == nil {
		t.Fatal("missing store should fail")
	}
	store := &childThreadStoreStub{createResult: map[string]any{"id": " "}}
	if _, err := PrepareChildThread(PrepareChildThreadInput{Store: store}); err == nil {
		t.Fatal("missing created id should fail")
	}
	expected := errors.New("store failed")
	store = &childThreadStoreStub{err: expected}
	if _, err := PrepareChildThread(PrepareChildThreadInput{Store: store}); !errors.Is(err, expected) {
		t.Fatalf("expected store error, got %v", err)
	}
}

func TestPrepareChildThreadRejectsInvalidReasoningEffortBeforeStoreEffects(t *testing.T) {
	const sentinel = "SOL_PRIVATE_REASONING_SENTINEL_7F3C"
	store := &childThreadStoreStub{}
	_, err := PrepareChildThread(PrepareChildThreadInput{
		Store: store, ParentThreadID: "thread-parent", Effort: sentinel,
	})
	if err == nil || strings.Contains(err.Error(), sentinel) || store.createPatch != nil ||
		store.forkRequest != nil || store.patchPatch != nil {
		t.Fatalf("invalid effort reached or was reflected by child thread store: err=%v store=%#v", err, store)
	}
}
