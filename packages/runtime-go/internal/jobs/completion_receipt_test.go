package jobs

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	jobsecuritytest "analytix.local/runtime-go/internal/testsupport/jobsecurity"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type completionVerifierStub struct {
	digest string
	reject bool
}

func (stub completionVerifierStub) VerifyStoredChildCompletion(_ context.Context, record domainjob.Record) error {
	if stub.reject || record.ChildCompletionReceipt == nil || record.ChildCompletionReceipt.ReceiptDigest != stub.digest {
		return errors.New("untrusted completion receipt")
	}
	return nil
}

func TestTrustedChildCompletionReceiptPersistsAndRestartsStrictly(t *testing.T) {
	parent, child, binding, receipt := jobCompletionReceiptFixture(t)
	root := t.TempDir()
	verifier := completionVerifierStub{digest: receipt.ReceiptDigest}
	manager, err := NewManagerWithChildCompletionVerifier(root, verifier)
	if err != nil {
		t.Fatal(err)
	}
	startRequest := StartRequest{
		ParentGoalID: "goal-1", ParentThreadID: parent.ThreadID, ParentTurnID: parent.TurnID,
		ParentToolCallID: binding.ParentToolCallID, SecurityBinding: binding, ChildThreadID: child.ThreadID,
		Kind: "subagent", Status: string(domainjob.StatusRunning),
	}
	jobsecuritytest.BindDelegatedToolManifestRequest(t, &startRequest)
	record, err := manager.StartChildRun(startRequest)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.ChildRunID != record.ID {
		t.Fatalf("fixture run identity mismatch: receipt=%s record=%s", receipt.ChildRunID, record.ID)
	}
	completed, err := manager.UpdateChildRun(record.ID, UpdateRequest{
		Status: string(domainjob.StatusCompleted), ChildTurnID: child.TurnID, ChildCompletionReceipt: &receipt,
	})
	if err != nil || completed.ChildCompletionReceipt == nil || completed.Output != "" {
		t.Fatalf("trusted completion persistence failed: record=%#v err=%v", completed, err)
	}
	path := filepath.Join(root, record.ID+".json")
	beforeReplay, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	toolInvocations := completed.ToolInvocations
	replayed, err := manager.UpdateChildRun(record.ID, UpdateRequest{
		Status: string(domainjob.StatusCompleted), ChildTurnID: child.TurnID,
		ChildCompletionReceipt: &receipt, ToolInvocations: &toolInvocations,
	})
	if err != nil || replayed.UpdatedAt != completed.UpdatedAt || replayed.FinishedAt != completed.FinishedAt {
		t.Fatalf("exact completion replay was not idempotent: before=%#v after=%#v err=%v", completed, replayed, err)
	}
	afterReplay, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(beforeReplay, afterReplay) {
		t.Fatalf("exact completion replay rewrote durable bytes: err=%v", err)
	}
	restarted, err := NewManagerWithChildCompletionVerifier(root, verifier)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := restarted.LoadChildRun(record.ID)
	if err != nil || restored.ChildCompletionReceipt == nil || restored.ChildCompletionReceipt.ReceiptDigest != receipt.ReceiptDigest {
		t.Fatalf("strict restart lost trusted completion: record=%#v err=%v", restored, err)
	}
	if _, err := NewManager(root); err == nil {
		t.Fatal("restart accepted a completion receipt without a host trust verifier")
	}
	if _, err := NewManagerWithChildCompletionVerifier(root, completionVerifierStub{digest: receipt.ReceiptDigest, reject: true}); err == nil {
		t.Fatal("restart accepted a completion receipt rejected by the host trust verifier")
	}
}

func TestChildCompletionReceiptCannotMutateChildIdentityInSameUpdate(t *testing.T) {
	parent, child, binding, receipt := jobCompletionReceiptFixture(t)
	manager, err := NewManagerWithChildCompletionVerifier(t.TempDir(), completionVerifierStub{digest: receipt.ReceiptDigest})
	if err != nil {
		t.Fatal(err)
	}
	startRequest := StartRequest{
		ParentGoalID: "goal-1", ParentThreadID: parent.ThreadID, ParentTurnID: parent.TurnID,
		ParentToolCallID: binding.ParentToolCallID, SecurityBinding: binding, ChildThreadID: child.ThreadID,
		Kind: "subagent", Status: string(domainjob.StatusRunning),
	}
	jobsecuritytest.BindDelegatedToolManifestRequest(t, &startRequest)
	record, err := manager.StartChildRun(startRequest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateChildRun(record.ID, UpdateRequest{
		Status: string(domainjob.StatusCompleted), ChildThreadID: "thread-other", ChildTurnID: child.TurnID,
		ChildCompletionReceipt: &receipt,
	}); err == nil {
		t.Fatal("completion update changed the immutable child thread")
	}
	if _, err := manager.UpdateChildRun(record.ID, UpdateRequest{
		Status: string(domainjob.StatusCompleted), ChildTurnID: "turn-other", ChildCompletionReceipt: &receipt,
	}); err == nil {
		t.Fatal("completion update attached a receipt to another child turn")
	}
}

func TestForegroundHandoffReceiptPersistsAuditOnlyWithoutChildBodyOrRestartAuthority(t *testing.T) {
	now := time.Date(2026, 7, 23, 9, 0, 0, 0, time.UTC)
	workspace := t.TempDir()
	parent, err := securitycontexttest.HostGeneralOnlyExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "foreground-parent", TurnID: "foreground-parent-turn", WorkspaceRealPath: workspace,
		ContextEpoch: 2, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := securitycontexttest.HostGeneralOnlyExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "foreground-child", TurnID: "foreground-child-turn", WorkspaceRealPath: workspace,
		ContextEpoch: 1, IssuedAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	callID := jobsTestHostToolCallID("foreground-handoff")
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: parent, Provider: "provider", ServerIdentity: "host:builtin", ToolName: "task", ToolCallID: callID,
		ArgsHash: domainsecurity.SHA256Hex([]byte("args")), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required", IssuedAt: now,
	})
	binding, err := domainjob.NewSecurityBinding(parent, grant, callID)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	manager, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	start := StartRequest{
		ParentGoalID: "goal-1", ParentThreadID: parent.ThreadID, ParentTurnID: parent.TurnID,
		ParentToolCallID: callID, SecurityBinding: binding, ChildThreadID: child.ThreadID,
		Kind: "subagent", Status: string(domainjob.StatusRunning), Workspace: workspace,
		ToolScope: []string{"submit_child_result"},
	}
	jobsecuritytest.BindDelegatedToolManifestRequest(t, &start)
	record, err := manager.StartChildRun(start)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := domainjob.NewForegroundChildHandoffReceiptV1(domainjob.ForegroundChildHandoffReceiptInputV1{
		ParentThreadID: parent.ThreadID, ParentTurnID: parent.TurnID, ParentRunID: parent.TurnID,
		ChildThreadID: child.ThreadID, ChildTurnID: child.TurnID, ChildRunID: record.ID,
		WorkspaceRealPath: workspace, ParentContextEpoch: parent.ContextEpoch, ChildContextEpoch: child.ContextEpoch,
		ParentExecutionGrantID: grant.GrantID, ParentToolCallID: callID,
		ChildAcceptedTerminalDigest: domainsecurity.SHA256Hex([]byte("terminal")),
		SubmissionDigest:            domainsecurity.SHA256Hex([]byte("private body")),
		PrivacyProjectionDigest:     domainsecurity.SHA256Hex([]byte("projected body")),
		ConsumptionNonce:            domainsecurity.SHA256Hex([]byte("nonce")), IssuedAt: now.Add(2 * time.Second), ExpiresAt: now.Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateChildRun(record.ID, UpdateRequest{
		Status: string(domainjob.StatusCompleted), ChildTurnID: child.TurnID,
	}); err == nil {
		t.Fatal("foreground child completed without a handoff receipt")
	}
	if _, err := manager.UpdateChildRun(record.ID, UpdateRequest{
		Status: string(domainjob.StatusCompleted), ChildTurnID: child.TurnID,
		ForegroundChildHandoffReceipt: &receipt, Output: "private body",
	}); err == nil {
		t.Fatal("foreground child body entered durable completion")
	}
	completed, err := manager.UpdateChildRun(record.ID, UpdateRequest{
		Status: string(domainjob.StatusCompleted), ChildTurnID: child.TurnID,
		ForegroundChildHandoffReceipt: &receipt,
	})
	if err != nil || completed.Output != "" || completed.ForegroundChildHandoffReceipt == nil {
		t.Fatalf("foreground audit receipt did not persist safely: record=%#v err=%v", completed, err)
	}
	replayed, err := manager.UpdateChildRun(record.ID, UpdateRequest{
		Status: string(domainjob.StatusCompleted), ChildTurnID: child.TurnID,
		ForegroundChildHandoffReceipt: &receipt,
	})
	if err != nil || replayed.UpdatedAt != completed.UpdatedAt || replayed.FinishedAt != completed.FinishedAt {
		t.Fatalf("exact foreground receipt replay mutated durable state: before=%#v after=%#v err=%v", completed, replayed, err)
	}
	conflict, err := domainjob.NewForegroundChildHandoffReceiptV1(domainjob.ForegroundChildHandoffReceiptInputV1{
		ParentThreadID: parent.ThreadID, ParentTurnID: parent.TurnID, ParentRunID: parent.TurnID,
		ChildThreadID: child.ThreadID, ChildTurnID: child.TurnID, ChildRunID: record.ID,
		WorkspaceRealPath: workspace, ParentContextEpoch: parent.ContextEpoch, ChildContextEpoch: child.ContextEpoch,
		ParentExecutionGrantID: grant.GrantID, ParentToolCallID: callID,
		ChildAcceptedTerminalDigest: domainsecurity.SHA256Hex([]byte("terminal")),
		SubmissionDigest:            domainsecurity.SHA256Hex([]byte("different body")),
		PrivacyProjectionDigest:     domainsecurity.SHA256Hex([]byte("different projection")),
		ConsumptionNonce:            domainsecurity.SHA256Hex([]byte("different nonce")), IssuedAt: now.Add(2 * time.Second), ExpiresAt: now.Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateChildRun(record.ID, UpdateRequest{
		Status: string(domainjob.StatusCompleted), ChildTurnID: child.TurnID,
		ForegroundChildHandoffReceipt: &conflict,
	}); err == nil {
		t.Fatal("completed foreground receipt was replaced")
	}
	restarted, err := NewManager(root)
	if err != nil {
		t.Fatalf("audit-only receipt should restart without acquiring live authority: %v", err)
	}
	restored, err := restarted.LoadChildRun(record.ID)
	if err != nil || restored.Output != "" || restored.ForegroundChildHandoffReceipt == nil ||
		restored.ForegroundChildHandoffReceipt.ReceiptDigest != receipt.ReceiptDigest {
		t.Fatalf("foreground receipt restart projection drifted: record=%#v err=%v", restored, err)
	}
}

func jobCompletionReceiptFixture(t *testing.T) (domainsecurity.TurnSecurityContext, domainsecurity.TurnSecurityContext, *domainjob.SecurityBinding, domainjob.ChildCompletionReceiptV1) {
	t.Helper()
	now := time.Date(2026, 7, 13, 8, 0, 0, 0, time.UTC)
	parent, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-parent", TurnID: "turn-parent", WorkspaceRealPath: "/cases/case-a", CaseID: "case-a",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-a"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 1, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-child", TurnID: "turn-child", WorkspaceRealPath: parent.WorkspaceRealPath, CaseID: parent.CaseID,
		CaseBindingHash: parent.CaseBindingHash, DatasetSnapshotID: parent.DatasetSnapshotID,
		SourceManifestHash: parent.SourceManifestHash, ContextEpoch: 1, IssuedAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	toolCallID := jobsTestHostToolCallID("completion-receipt")
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: parent, Provider: "provider-a", ServerIdentity: "host:builtin", ToolName: "task", ToolCallID: toolCallID,
		ArgsHash: domainsecurity.SHA256Hex([]byte("args")), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required", IssuedAt: now,
	})
	binding, err := domainjob.NewSecurityBinding(parent, grant, grant.ToolCallID)
	if err != nil {
		t.Fatal(err)
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{43}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	envelope, err := domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
		Variant: domainevidence.SourceUnavailableAnswer, Context: child, TerminalReason: "source_unavailable",
		Blocker: "current_case_source_unavailable", AcquisitionSteps: []string{"reconnect_source"}, IssuedAt: now.Add(2 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := domainevidence.NewEvidenceReceiptRegistry(child)
	if err != nil {
		t.Fatal(err)
	}
	head, err := domainevidence.NewEvidenceRegistryHead(registry)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := domainevidence.RenderFinalAnswer(envelope)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := domainevidence.NewTerminalPublicationIntent(domainevidence.TerminalPublicationIntentInput{
		CreatedAt: now.Add(2 * time.Second).Format(time.RFC3339Nano), TerminalStatus: "completed",
	}, envelope.TerminalReason)
	if err != nil {
		t.Fatal(err)
	}
	privateDigest, err := domainevidence.PrivateAcceptedFinalDigest(child, envelope, rendered, intent)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := domainevidence.NewAcceptedFinalRecord(domainevidence.AcceptedFinalRecordInput{
		Context: child, Envelope: envelope, RenderedText: rendered, RegistryHead: head, PrivateRecordDigest: privateDigest,
		AcceptedAt: now.Add(2 * time.Second), AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := domainjob.NewChildCompletionReceiptV1(domainjob.ChildCompletionReceiptInputV1{
		ChildRunID: "job-1", SecurityBinding: binding, ParentContext: parent, ChildContext: child, AcceptedFinal: accepted,
		CanContinueParent: true, IssuedAt: now.Add(3 * time.Second), AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil {
		t.Fatal(err)
	}
	return parent, child, binding, receipt
}
