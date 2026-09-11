package turnstart

import (
	"context"
	"errors"
	"testing"
	"time"

	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	testsecurity "analytix.local/runtime-go/internal/testsupport/securitycontext"
	toolidentitytest "analytix.local/runtime-go/internal/testsupport/toolidentity"
)

type childAuthorityStoreStub struct {
	loaded          domainjob.Record
	validated       domainjob.Record
	loadErr         error
	validateErr     error
	loadCalls       int
	validationCalls int
}

func (store *childAuthorityStoreStub) LoadChildRun(string) (domainjob.Record, error) {
	store.loadCalls++
	return cloneChildAuthorityRecord(store.loaded), store.loadErr
}

func (store *childAuthorityStoreStub) ValidateChildRunStart(domainjob.Record) (domainjob.Record, error) {
	store.validationCalls++
	return cloneChildAuthorityRecord(store.validated), store.validateErr
}

type childAuthorityFixture struct {
	parent domainsecurity.TurnSecurityContext
	frozen domainsecurity.TurnSecurityContext
	record domainjob.Record
}

func TestResolveChildTransitionAuthority(t *testing.T) {
	metadataStore := &childAuthorityStoreStub{}
	authority, err := ResolveChildTransitionAuthority(ChildTransitionResolveInput{
		ChildDepth: 0, ChildThreadID: "metadata-only", Store: metadataStore,
	})
	if err != nil || authority != nil || metadataStore.loadCalls != 0 {
		t.Fatalf("metadata-only top-level transition acquired child authority: authority=%#v calls=%d err=%v", authority, metadataStore.loadCalls, err)
	}
	if authority, err := ResolveChildTransitionAuthority(ChildTransitionResolveInput{ChildDepth: 1}); err == nil || authority != nil {
		t.Fatalf("child depth without a durable run was accepted: authority=%#v err=%v", authority, err)
	}

	for _, riskClass := range []string{domainsecurity.RiskClassGeneral, domainsecurity.RiskClassCase} {
		t.Run(riskClass+" parent", func(t *testing.T) {
			fixture := newChildAuthorityFixture(t, riskClass)
			store := &childAuthorityStoreStub{loaded: fixture.record, validated: fixture.record}
			authority, err := ResolveChildTransitionAuthority(ChildTransitionResolveInput{
				ChildRunID: fixture.record.ID, ChildDepth: 1, ChildThreadID: fixture.record.ChildThreadID,
				Store: store, Blocker: childAuthorityNoBlocker,
			})
			if err != nil {
				t.Fatalf("valid durable child transition authority was rejected: %v", err)
			}
			if authority == nil || authority.ChildRunID != fixture.record.ID || authority.ChildThreadID != fixture.record.ChildThreadID ||
				authority.Background != fixture.record.Background || authority.Binding == nil ||
				authority.Binding.BindingDigest != fixture.record.SecurityBinding.BindingDigest {
				t.Fatalf("resolved child authority mismatch: %#v", authority)
			}
			if authority.Binding == fixture.record.SecurityBinding {
				t.Fatal("resolved authority retained the caller-owned security binding pointer")
			}
		})
	}

	base := newChildAuthorityFixture(t, domainsecurity.RiskClassCase)
	loadFailure := errors.New("load failed")
	tests := []struct {
		name    string
		mutate  func(*domainjob.Record)
		loadErr error
		blocker ChildRunBlocker
	}{
		{name: "missing store", blocker: childAuthorityNoBlocker},
		{name: "missing blocker"},
		{name: "load failure", loadErr: loadFailure, blocker: childAuthorityNoBlocker},
		{name: "wrong run binding", mutate: func(record *domainjob.Record) { record.ID = "run-other" }, blocker: childAuthorityNoBlocker},
		{name: "wrong child thread binding", mutate: func(record *domainjob.Record) { record.ChildThreadID = "thread-other" }, blocker: childAuthorityNoBlocker},
		{name: "completed run cannot start", mutate: func(record *domainjob.Record) { record.Status = string(domainjob.StatusCompleted) }, blocker: childAuthorityNoBlocker},
		{name: "invalid security binding", mutate: func(record *domainjob.Record) {
			record.SecurityBinding.BindingDigest = domainsecurity.SHA256Hex([]byte("forged"))
		}, blocker: childAuthorityNoBlocker},
		{name: "record parent mismatch", mutate: func(record *domainjob.Record) { record.ParentTurnID = "turn-other" }, blocker: childAuthorityNoBlocker},
		{name: "missing completion receipt", blocker: func(domainjob.Record) string { return "child_completion_receipt_required" }},
		{name: "completion receipt wrong binding", blocker: func(domainjob.Record) string { return "child_completion_receipt_binding_mismatch" }},
		{name: "completion receipt cannot continue", blocker: func(domainjob.Record) string { return "child_completion_receipt_not_continuable" }},
		{name: "completion receipt epoch mismatch", blocker: func(domainjob.Record) string { return "child_completion_receipt_epoch_mismatch" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := cloneChildAuthorityRecord(base.record)
			if test.mutate != nil {
				test.mutate(&record)
			}
			var store ChildRunAuthorityStore
			if test.name != "missing store" {
				store = &childAuthorityStoreStub{loaded: record, validated: record, loadErr: test.loadErr}
			}
			authority, err := ResolveChildTransitionAuthority(ChildTransitionResolveInput{
				ChildRunID: base.record.ID, ChildDepth: 1, ChildThreadID: base.record.ChildThreadID,
				Store: store, Blocker: test.blocker,
			})
			if err == nil || authority != nil {
				t.Fatalf("invalid durable child authority was accepted: authority=%#v err=%v", authority, err)
			}
		})
	}
}

func TestResolveDelegatedToolAuthorityUsesDurableScopeAndManifest(t *testing.T) {
	requestedScope := []string{"read", "mcp__docs__lookup"}
	toolSchemaHash := domainsecurity.SHA256Hex([]byte("delegated-tool-schema"))
	manifest, err := domainjob.NewDelegatedToolManifestV1(
		requestedScope, toolSchemaHash, domainsecurity.SHA256Hex([]byte("delegated-mcp-authority")),
	)
	if err != nil {
		t.Fatal(err)
	}
	authority := &ChildTransitionAuthority{ExpectedRecord: domainjob.Record{
		ToolScope: requestedScope, ToolSchemaHash: toolSchemaHash, DelegatedToolManifest: manifest,
	}}
	resolvedScope, resolvedManifest, err := ResolveDelegatedToolAuthority(authority, requestedScope)
	if err != nil {
		t.Fatalf("valid durable delegated authority was rejected: %v", err)
	}
	if len(resolvedScope) != 2 || resolvedScope[0] != "read" || resolvedManifest == nil || resolvedManifest.ManifestHash != manifest.ManifestHash {
		t.Fatalf("resolved delegated authority mismatch: scope=%#v manifest=%#v", resolvedScope, resolvedManifest)
	}
	resolvedScope[0] = "mutated"
	resolvedManifest.ManifestHash = domainsecurity.SHA256Hex([]byte("mutated"))
	if authority.ExpectedRecord.ToolScope[0] != "read" || authority.ExpectedRecord.DelegatedToolManifest.ManifestHash != manifest.ManifestHash {
		t.Fatal("resolved delegated authority retained caller-owned slices or pointers")
	}
	if _, _, err := ResolveDelegatedToolAuthority(authority, []string{"read"}); err == nil {
		t.Fatal("caller scope drift was accepted over the durable delegated scope")
	}
	plainScope, plainManifest, err := ResolveDelegatedToolAuthority(nil, requestedScope)
	if err != nil || plainManifest != nil || len(plainScope) != len(requestedScope) {
		t.Fatalf("top-level turn delegation resolution mismatch: scope=%#v manifest=%#v err=%v", plainScope, plainManifest, err)
	}
}

func TestValidateChildTransitionFrozen(t *testing.T) {
	if err := ValidateChildTransitionFrozen(nil, nil, nil, nil, domainsecurity.TurnSecurityContext{}); err != nil {
		t.Fatalf("metadata-only transition required child authority: %v", err)
	}

	for _, riskClass := range []string{domainsecurity.RiskClassGeneral, domainsecurity.RiskClassCase} {
		t.Run(riskClass+" parent", func(t *testing.T) {
			fixture := newChildAuthorityFixture(t, riskClass)
			authority := resolveChildAuthorityForTest(t, fixture.record, childAuthorityNoBlocker)
			store := &childAuthorityStoreStub{loaded: fixture.record, validated: fixture.record}
			if err := ValidateChildTransitionFrozen(context.Background(), authority, store, childAuthorityNoBlocker, fixture.frozen); err != nil {
				t.Fatalf("unchanged frozen child transition was rejected: %v", err)
			}
			if store.validationCalls != 1 {
				t.Fatalf("frozen child record was not revalidated exactly once: %d", store.validationCalls)
			}
		})
	}

	base := newChildAuthorityFixture(t, domainsecurity.RiskClassCase)
	tests := []struct {
		name         string
		context      func() context.Context
		mutateFrozen func(*domainsecurity.TurnSecurityContext)
		mutateRecord func(*domainjob.Record)
		validateErr  error
		blocker      ChildRunBlocker
	}{
		{name: "nil context", context: func() context.Context { return nil }, blocker: childAuthorityNoBlocker},
		{name: "cancelled context", context: cancelledChildAuthorityContext, blocker: childAuthorityNoBlocker},
		{name: "missing store", blocker: childAuthorityNoBlocker},
		{name: "missing blocker"},
		{name: "wrong frozen child thread", mutateFrozen: func(frozen *domainsecurity.TurnSecurityContext) { frozen.ThreadID = "thread-other" }, blocker: childAuthorityNoBlocker},
		{name: "frozen epoch tampering", mutateFrozen: func(frozen *domainsecurity.TurnSecurityContext) { frozen.ContextEpoch++ }, blocker: childAuthorityNoBlocker},
		{name: "case binding changed", mutateFrozen: func(frozen *domainsecurity.TurnSecurityContext) {
			frozen.CaseBindingHash = domainsecurity.SHA256Hex([]byte("other-case-binding"))
		}, blocker: childAuthorityNoBlocker},
		{name: "record changed during writer wait", validateErr: errors.New("record changed"), blocker: childAuthorityNoBlocker},
		{name: "record id mutation", mutateRecord: func(record *domainjob.Record) { record.ID = "run-other" }, blocker: childAuthorityNoBlocker},
		{name: "record child mutation", mutateRecord: func(record *domainjob.Record) { record.ChildThreadID = "thread-other" }, blocker: childAuthorityNoBlocker},
		{name: "record background mutation", mutateRecord: func(record *domainjob.Record) { record.Background = !record.Background }, blocker: childAuthorityNoBlocker},
		{name: "record binding mutation", mutateRecord: func(record *domainjob.Record) {
			record.SecurityBinding.BindingDigest = domainsecurity.SHA256Hex([]byte("changed-binding"))
		}, blocker: childAuthorityNoBlocker},
		{name: "completion receipt removed during writer wait", blocker: func(domainjob.Record) string { return "child_completion_receipt_required" }},
		{name: "completion receipt no longer continuable", blocker: func(domainjob.Record) string { return "child_completion_receipt_not_continuable" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			authority := resolveChildAuthorityForTest(t, base.record, childAuthorityNoBlocker)
			frozen := base.frozen
			if test.mutateFrozen != nil {
				test.mutateFrozen(&frozen)
			}
			validated := cloneChildAuthorityRecord(base.record)
			if test.mutateRecord != nil {
				test.mutateRecord(&validated)
			}
			var store ChildRunAuthorityStore
			if test.name != "missing store" {
				store = &childAuthorityStoreStub{loaded: base.record, validated: validated, validateErr: test.validateErr}
			}
			ctx := context.Background()
			if test.context != nil {
				ctx = test.context()
			}
			if err := ValidateChildTransitionFrozen(ctx, authority, store, test.blocker, frozen); err == nil {
				t.Fatal("mutated frozen child transition was accepted")
			}
		})
	}
}

func TestChildTransitionFrozenCannotReplaceReservedTurnIdentity(t *testing.T) {
	fixture := newChildAuthorityFixture(t, domainsecurity.RiskClassGeneral)
	fixture.record.ChildTurnID = "turn_999"
	authority := resolveChildAuthorityForTest(t, fixture.record, childAuthorityNoBlocker)
	store := &childAuthorityStoreStub{loaded: fixture.record, validated: fixture.record}
	if err := ValidateChildTransitionFrozen(context.Background(), authority, store, childAuthorityNoBlocker, fixture.frozen); err == nil {
		t.Fatal("child freeze replaced the durable reserved turn identity")
	}
}

func newChildAuthorityFixture(t *testing.T, riskClass string) childAuthorityFixture {
	t.Helper()
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	parentInput := domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-parent", TurnID: "turn-parent", WorkspaceRealPath: "/workspace/canonical",
		ContextEpoch: 7, IssuedAt: now,
	}
	var parent domainsecurity.TurnSecurityContext
	var err error
	if riskClass == domainsecurity.RiskClassCase {
		parentInput.CaseID = "case-a"
		parentInput.CaseBindingHash = domainsecurity.SHA256Hex([]byte("case-binding-a"))
		parentInput.DatasetSnapshotID = testsecurity.DatasetSnapshotID("snapshot-a")
		parentInput.SourceManifestHash = domainsecurity.SHA256Hex([]byte("sources-a"))
		parent, err = testsecurity.CaseExecutionContextV2(parentInput)
	} else {
		parent, err = testsecurity.GeneralExecutionContextV2(parentInput)
	}
	if err != nil {
		t.Fatal(err)
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: parent, Provider: "provider-a", ServerIdentity: "host:builtin", ToolName: "task", ToolCallID: toolidentitytest.MustHostToolCallIDV1("turnstart-child-authority-" + riskClass),
		ArgsHash: domainsecurity.SHA256Hex([]byte("args")), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required", IssuedAt: now,
	})
	binding, err := domainjob.NewSecurityBinding(parent, grant, grant.ToolCallID)
	if err != nil {
		t.Fatal(err)
	}
	record := domainjob.Record{
		ID: "run-child", ParentThreadID: parent.ThreadID, ParentTurnID: parent.TurnID,
		ParentToolCallID: grant.ToolCallID, ChildThreadID: "thread-child", ChildTurnID: "turn-child", Kind: "subagent",
		Status: string(domainjob.StatusRunning), SecurityBinding: binding, Output: "UNTRUSTED_CHILD_OUTPUT",
	}
	childInput := domainsecurity.TurnSecurityContextInput{
		ThreadID: record.ChildThreadID, TurnID: "turn-child", WorkspaceRealPath: parent.WorkspaceRealPath,
		ContextEpoch: 1, IssuedAt: now.Add(time.Second),
	}
	var frozen domainsecurity.TurnSecurityContext
	if riskClass == domainsecurity.RiskClassCase {
		childInput.CaseID = parent.CaseID
		childInput.ContextEpoch = parent.ContextEpoch
		childInput.CaseBindingHash = parent.CaseBindingHash
		childInput.DatasetSnapshotID = parent.DatasetSnapshotID
		childInput.SourceManifestHash = parent.SourceManifestHash
		frozen, err = testsecurity.CaseExecutionContextV2(childInput)
	} else {
		frozen, err = testsecurity.GeneralExecutionContextV2(childInput)
	}
	if err != nil {
		t.Fatal(err)
	}
	return childAuthorityFixture{parent: parent, frozen: frozen, record: record}
}

func resolveChildAuthorityForTest(t *testing.T, record domainjob.Record, blocker ChildRunBlocker) *ChildTransitionAuthority {
	t.Helper()
	store := &childAuthorityStoreStub{loaded: record, validated: record}
	authority, err := ResolveChildTransitionAuthority(ChildTransitionResolveInput{
		ChildRunID: record.ID, ChildDepth: 1, ChildThreadID: record.ChildThreadID, Store: store, Blocker: blocker,
	})
	if err != nil {
		t.Fatal(err)
	}
	return authority
}

func childAuthorityNoBlocker(domainjob.Record) string { return "" }

func cloneChildAuthorityRecord(record domainjob.Record) domainjob.Record {
	record.SecurityBinding = domainjob.CloneSecurityBinding(record.SecurityBinding)
	return record
}

func cancelledChildAuthorityContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}
