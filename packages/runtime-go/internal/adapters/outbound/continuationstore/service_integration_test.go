package continuationstore_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	continuationstore "analytix.local/runtime-go/internal/adapters/outbound/continuationstore"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	continuationapp "analytix.local/runtime-go/internal/app/continuation"
	appmodel "analytix.local/runtime-go/internal/app/model"
	"analytix.local/runtime-go/internal/contracts"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func continuationStoreTestToolCallID(seed string) string {
	entropy := sha256.Sum256([]byte("analytix.continuation-store-test-tool-call/v1\x00" + seed))
	identity, err := domainmodel.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		panic(err)
	}
	return identity
}

func TestContinuationServicePersistsAndConsumesExactlyOnceAcrossRestart(t *testing.T) {
	root := t.TempDir()
	store, err := newContinuationTestStore(t, root+"/continuations")
	if err != nil {
		t.Fatal(err)
	}
	authorityPath := root + "/authority/continuation-ed25519-v1.json"
	authority, err := finalauthority.OpenOrCreateFileAuthority(authorityPath, false)
	if err != nil {
		t.Fatal(err)
	}
	service := continuationapp.NewService(authority, store)
	now := time.Now().UTC()
	payload := servicePayloadFixture(t, now)
	receipt, err := service.Issue(context.Background(), payload)
	if err != nil {
		t.Fatal(err)
	}
	if verified, err := service.VerifyOpen(context.Background(), receipt.Payload.GateID, now.Add(time.Minute)); err != nil || verified.ReceiptID != receipt.ReceiptID {
		t.Fatalf("open receipt verification failed verified=%#v err=%v", verified, err)
	}
	if _, err := service.Consume(context.Background(), receipt.Payload.GateID, domaincontinuation.StatusAllowed, "approval_allowed", now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Consume(context.Background(), receipt.Payload.GateID, domaincontinuation.StatusAllowed, "approval_allowed", now.Add(3*time.Minute)); !errors.Is(err, continuationapp.ErrReceiptConsumed) {
		t.Fatalf("second consume must be rejected, got %v", err)
	}
	restartedAuthority, err := finalauthority.OpenOrCreateFileAuthority(authorityPath, true)
	if err != nil {
		t.Fatal(err)
	}
	restartedStore, err := newContinuationTestStore(t, root+"/continuations")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := continuationapp.NewService(restartedAuthority, restartedStore).VerifyOpen(context.Background(), receipt.Payload.GateID, now.Add(4*time.Minute)); !errors.Is(err, continuationapp.ErrReceiptConsumed) {
		t.Fatalf("restart must retain exact-once disposition, got %v", err)
	}
}

func TestApprovalAndUserInputReceiptsPersistExactContextAndGrantAcrossRestart(t *testing.T) {
	root := t.TempDir()
	store, err := newContinuationTestStore(t, filepath.Join(root, "continuations"))
	if err != nil {
		t.Fatal(err)
	}
	authorityPath := filepath.Join(root, "authority", "continuation-ed25519-v1.json")
	authority, err := finalauthority.OpenOrCreateFileAuthority(authorityPath, false)
	if err != nil {
		t.Fatal(err)
	}
	service := continuationapp.NewService(authority, store)
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		payload    domaincontinuation.Payload
		status     string
		reasonCode string
	}{
		{name: "approval", payload: servicePayloadFixture(t, now), status: domaincontinuation.StatusAllowed, reasonCode: "approval_allowed"},
		{name: "user_input", payload: serviceUserInputPayloadFixture(t, now), status: domaincontinuation.StatusSubmitted, reasonCode: "user_input_submitted"},
	}
	receipts := make(map[string]domaincontinuation.Receipt, len(tests))
	for _, test := range tests {
		receipt, err := service.Issue(context.Background(), test.payload)
		if err != nil {
			t.Fatalf("issue %s receipt: %v", test.name, err)
		}
		receipts[test.name] = receipt
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	restartedAuthority, err := finalauthority.OpenOrCreateFileAuthority(authorityPath, true)
	if err != nil {
		t.Fatal(err)
	}
	restartedStore, err := newContinuationTestStore(t, filepath.Join(root, "continuations"))
	if err != nil {
		t.Fatal(err)
	}
	defer restartedStore.Close()
	restarted := continuationapp.NewService(restartedAuthority, restartedStore)
	for _, test := range tests {
		receipt := receipts[test.name]
		verified, err := restarted.VerifyOpen(context.Background(), receipt.Payload.GateID, now.Add(2*time.Minute))
		if err != nil {
			t.Fatalf("verify restarted %s receipt: %v", test.name, err)
		}
		if verified.Payload.Kind != test.payload.Kind ||
			verified.Payload.SecurityContext.ContextDigest != test.payload.SecurityContext.ContextDigest ||
			verified.Payload.ExecutionGrant.GrantID != test.payload.ExecutionGrant.GrantID {
			t.Fatalf("restarted %s authority changed context/grant binding: %#v", test.name, verified.Payload)
		}
		if _, err := restarted.Consume(context.Background(), verified.Payload.GateID, test.status, test.reasonCode, now.Add(3*time.Minute)); err != nil {
			t.Fatalf("consume restarted %s receipt: %v", test.name, err)
		}
	}
}

func TestTrustedContinuationInventoryPinsInstallationAndExactDispositionGraph(t *testing.T) {
	root := t.TempDir()
	store, err := newContinuationTestStore(t, filepath.Join(root, "continuations"))
	if err != nil {
		t.Fatal(err)
	}
	authority, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(root, "authority", "final.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	service := continuationapp.NewService(authority, store)
	now := time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC)
	payload := servicePayloadWithPrivateArguments(t, now, json.RawMessage(`{"path":"case-a.txt","account":"6222020202020202020"}`))
	receipt, err := service.Issue(context.Background(), payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Consume(context.Background(), receipt.Payload.GateID, domaincontinuation.StatusAllowed, "approval_allowed", now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := continuationapp.VerifyTrustedInventoryV1(context.Background(), store, authority); err != nil {
		t.Fatalf("current installation inventory was rejected: %v", err)
	}

	foreign, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(root, "foreign-authority", "final.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := continuationapp.VerifyTrustedInventoryV1(context.Background(), store, foreign); err == nil {
		t.Fatal("foreign installation authority accepted private continuation inventory")
	}

	orphanRoot := filepath.Join(root, "orphan-continuations")
	orphanAccess, err := privatecastest.NewAccessAuthority(orphanRoot)
	if err != nil {
		t.Fatal(err)
	}
	disposition, err := domaincontinuation.NewDisposition(
		receipt, domaincontinuation.StatusDenied, "approval_denied", now.Add(3*time.Minute),
		authority.KeyID(), authority.PublicKey(), func(message []byte) ([]byte, error) {
			return authority.Sign(context.Background(), message)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	dispositionBody, err := domaincontinuation.DispositionBytes(disposition)
	if err != nil {
		t.Fatal(err)
	}
	digest := strings.TrimPrefix(strings.TrimPrefix(disposition.GateID, "appr_"), "input_")
	dispositionCAS, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(
		filepath.Join(orphanRoot, "dispositions-v2"), 6*1024*1024, orphanAccess,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := dispositionCAS.PutIfAbsent(context.Background(), digest, dispositionBody); err != nil {
		t.Fatal(err)
	}
	if err := dispositionCAS.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := continuationstore.NewStore(orphanRoot, orphanAccess); err == nil {
		t.Fatal("orphan continuation disposition acquired authority without its receipt")
	}
}

func TestContinuationInventoryRejectsBroadenedPrivateFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows effective access is ACL-based rather than POSIX-mode based")
	}
	base := t.TempDir()
	root := filepath.Join(base, "continuations")
	store, err := newContinuationTestStore(t, root)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(base, "authority", "final.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	payload := servicePayloadWithPrivateArguments(t, now, json.RawMessage(`{"account":"6222020202020202020"}`))
	receipt, err := continuationapp.NewService(authority, store).Issue(context.Background(), payload)
	if err != nil {
		t.Fatal(err)
	}
	digest := strings.TrimPrefix(strings.TrimPrefix(receipt.Payload.GateID, "appr_"), "input_")
	receiptPath := filepath.Join(root, "receipts-v2", digest[:2], digest+".json")
	body, err := os.ReadFile(receiptPath)
	if err != nil || !bytes.Contains(body, []byte("6222020202020202020")) {
		t.Fatalf("private receipt fixture did not preserve exact authorized arguments: err=%v", err)
	}
	if err := os.Chmod(receiptPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.VisitReceipts(context.Background(), func(domaincontinuation.Receipt) error { return nil }); err == nil {
		t.Fatal("world-readable private continuation record passed inventory validation")
	}
}

func TestContinuationServiceCanCloseExpiredReceiptButCannotExecuteIt(t *testing.T) {
	root := t.TempDir()
	store, err := newContinuationTestStore(t, root+"/continuations")
	if err != nil {
		t.Fatal(err)
	}
	authority, err := finalauthority.OpenOrCreateFileAuthority(root+"/authority/continuation.json", false)
	if err != nil {
		t.Fatal(err)
	}
	service := continuationapp.NewService(authority, store)
	now := time.Now().UTC()
	payload := servicePayloadFixture(t, now)
	receipt, err := service.Issue(context.Background(), payload)
	if err != nil {
		t.Fatal(err)
	}
	afterExpiry := now.Add(16 * time.Minute)
	if _, err := service.Consume(context.Background(), receipt.Payload.GateID, domaincontinuation.StatusAllowed, "approval_allowed", afterExpiry); !errors.Is(err, continuationapp.ErrReceiptExpired) {
		t.Fatalf("expired receipt must not authorize execution, got %v", err)
	}
	if _, err := service.Close(context.Background(), receipt.Payload.GateID, domaincontinuation.StatusRestartInvalid, "restart_execution_grant_expired", afterExpiry); err != nil {
		t.Fatalf("expired receipt must remain closable: %v", err)
	}
}

func TestContinuationServiceRejectsReceiptPendingNamespaceMismatchBeforeConsume(t *testing.T) {
	root := t.TempDir()
	store, err := newContinuationTestStore(t, root+"/continuations")
	if err != nil {
		t.Fatal(err)
	}
	authority, err := finalauthority.OpenOrCreateFileAuthority(root+"/authority/continuation.json", false)
	if err != nil {
		t.Fatal(err)
	}
	service := continuationapp.NewService(authority, store)
	now := time.Now().UTC()
	pending := servicePendingFixture(t, now)
	gateID := domaincontinuation.GateID(domaincontinuation.KindApproval, pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID)
	receipt, err := service.IssuePendingHost(domaincontinuation.KindApproval, gateID, "item_"+gateID, pending, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	rebuilt, err := appmodel.RebuildPendingToolCallFromReceipt(receipt, servicePendingThread(pending, receipt), serviceProviderResolver{config: pending.ProviderConfig})
	if err != nil || rebuilt.ProviderNamespace != pending.ProviderNamespace || rebuilt.TerminalRecoveryKind != pending.TerminalRecoveryKind {
		t.Fatalf("signed provider/recovery namespace did not survive restart rebuild: rebuilt=%#v err=%v", rebuilt, err)
	}
	tampered := pending
	tampered.ProviderNamespace.ProviderCallSequence++
	if err := service.DisposePendingHost(gateID, domaincontinuation.StatusAllowed, "approval_allowed", now.Add(2*time.Minute), tampered); err == nil {
		t.Fatal("pending namespace mismatch consumed the signed continuation receipt")
	}
	if verified, err := service.VerifyOpen(context.Background(), gateID, now.Add(2*time.Minute)); err != nil || verified.ReceiptID != receipt.ReceiptID {
		t.Fatalf("mismatch should leave the receipt open: verified=%#v err=%v", verified, err)
	}
	if err := service.DisposePendingHost(gateID, domaincontinuation.StatusAllowed, "approval_allowed", now.Add(2*time.Minute), pending); err != nil {
		t.Fatalf("matching pending authority could not consume receipt: %v", err)
	}
}

func TestContinuationRestartRestoresLatestSignedPromotedProviderStep(t *testing.T) {
	root := t.TempDir()
	store, err := newContinuationTestStore(t, root+"/continuations")
	if err != nil {
		t.Fatal(err)
	}
	authority, err := finalauthority.OpenOrCreateFileAuthority(root+"/authority/continuation.json", false)
	if err != nil {
		t.Fatal(err)
	}
	service := continuationapp.NewService(authority, store)
	now := time.Now().UTC()
	pending := servicePendingFixture(t, now)
	promotedPrompt := "analyze the snapshot and continue the code edit"
	pending.Prompt = promotedPrompt
	pending.LogicalEffect = domainsecurity.LogicalEffectFundsData
	pending.OrdinaryWork = true
	pending.ProviderStepExact = true
	pending.CaseSourceUnavailable = false
	pending.OrdinaryResultInputIsolated = false
	gateID := domaincontinuation.GateID(
		domaincontinuation.KindApproval, pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest,
		pending.ExecutionGrant.GrantID, pending.Call.ID,
	)
	receipt, err := service.IssuePendingHost(
		domaincontinuation.KindApproval, gateID, "item_"+gateID, pending, now.Add(time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	entry, item := serviceSignedPromotedSteering(
		t, authority, pending.SecurityContext, "restart-steer", promotedPrompt,
		now.Add(-time.Second), now, domainsecurity.LogicalEffectFundsData, true,
	)
	thread := servicePendingThread(pending, receipt)
	turn := thread["turns"].([]any)[0].(map[string]any)
	turn["steering"] = []any{entry}
	items := turn["items"].([]any)
	items[0].(map[string]any)["text"] = "continue"
	turn["items"] = append(append([]any(nil), items[:1]...), append([]any{item}, items[1:]...)...)

	var rebuilt appmodel.PendingToolCall
	reason := service.RestartDispositionReason(
		gateID, receipt.ReceiptID, domaincontinuation.KindApproval,
		pending.ThreadID, pending.TurnID, receipt.Payload.ItemID,
		thread, serviceProviderResolver{config: pending.ProviderConfig},
		func(candidate appmodel.PendingToolCall) error {
			rebuilt = candidate
			return nil
		},
	)
	if reason != "restart_revalidation_passed_nonresumable" {
		t.Fatalf("restart did not revalidate the signed provider step: reason=%q rebuilt=%#v", reason, rebuilt)
	}
	if rebuilt.Prompt != promotedPrompt ||
		rebuilt.LogicalEffect != domainsecurity.LogicalEffectFundsData || !rebuilt.OrdinaryWork || !rebuilt.ProviderStepExact {
		t.Fatalf("restart lost exact promoted provider step: %#v", rebuilt)
	}
	foundSteer := false
	for _, message := range rebuilt.Messages {
		if strings.Contains(message.Content, promotedPrompt) {
			foundSteer = true
		}
	}
	if !foundSteer {
		t.Fatalf("restart provider history omitted signed steering: %#v", rebuilt.Messages)
	}
}

type serviceSteeringSigningAuthority interface {
	KeyID() string
	PublicKey() []byte
	Sign(context.Context, []byte) ([]byte, error)
}

func serviceSignedPromotedSteering(
	t *testing.T,
	authority serviceSteeringSigningAuthority,
	securityContext domainsecurity.TurnSecurityContext,
	clientID, text string,
	admittedAt, promotedAt time.Time,
	logicalEffect domainsecurity.LogicalEffect,
	ordinaryWork bool,
) (map[string]any, map[string]any) {
	t.Helper()
	entryID := domainsteering.EntryIDV1(securityContext.TurnID, clientID)
	admitted := admittedAt.UTC().Format(time.RFC3339Nano)
	promotedTime := promotedAt.UTC().Format(time.RFC3339Nano)
	pending, err := domainsteering.BindPendingEntryV1(map[string]any{
		"id": entryID, "clientUserMessageId": clientID, "text": text,
		"admittedAt": admitted, "delivery": "steer",
		"logicalEffect": string(logicalEffect), "ordinaryWork": ordinaryWork,
	}, securityContext.ContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	signingBytes, err := domainsteering.PendingEntrySigningBytesV1(pending, securityContext.ContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := authority.Sign(context.Background(), signingBytes)
	if err != nil {
		t.Fatal(err)
	}
	pending, err = domainsteering.SealPendingEntryAuthorityV1(
		pending, securityContext.ContextDigest, authority.KeyID(), authority.PublicKey(), signature,
	)
	if err != nil {
		t.Fatal(err)
	}
	promoted := contracts.CloneMap(pending)
	promoted["status"] = "promoted"
	promoted["promotedAt"] = promotedTime
	promoted["promotedItemId"] = entryID
	promotionBytes, err := domainsteering.PromotedEntrySigningBytesV1(promoted, securityContext.ContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	promotionSignature, err := authority.Sign(context.Background(), promotionBytes)
	if err != nil {
		t.Fatal(err)
	}
	promoted, err = domainsteering.SealPromotedEntryAuthorityV1(
		promoted, securityContext.ContextDigest, authority.KeyID(), authority.PublicKey(), promotionSignature,
	)
	if err != nil {
		t.Fatal(err)
	}
	item := map[string]any{
		"id": entryID, "turnId": securityContext.TurnID, "threadId": securityContext.ThreadID,
		"role": "user", "status": "completed", "kind": "user_message", "delivery": "steer",
		"text": text, "createdAt": admitted, "finishedAt": promotedTime,
		"clientUserMessageId": clientID, "contextDigest": securityContext.ContextDigest, "steeringOrigin": "ordinary",
		"steeringProjectionVersion": float64(domainsteering.ProjectionVersionV1),
		"steeringContentDigest":     promoted["contentDigest"],
	}
	return promoted, item
}

func servicePayloadFixture(t *testing.T, now time.Time) domaincontinuation.Payload {
	t.Helper()
	securityContext, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-a", TurnID: "turn-a", WorkspaceRealPath: "/workspace", SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 1, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	scope := []string{"write_file"}
	scopeBody, _ := json.Marshal(scope)
	toolCallID := continuationStoreTestToolCallID("service-write-file")
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider-a", ServerIdentity: "host:builtin", ToolName: "write_file", ToolCallID: toolCallID,
		ArgsHash: domainsecurity.CanonicalJSONHash([]byte(`{"path":"a.txt"}`)), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")), ScopeHash: domainsecurity.SHA256Hex(scopeBody),
		ApprovalState: "pending", IssuedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	})
	gateID := domaincontinuation.GateID(domaincontinuation.KindApproval, securityContext.ThreadID, securityContext.TurnID, securityContext.ContextDigest, grant.GrantID, grant.ToolCallID)
	return domaincontinuation.Payload{
		Version: domaincontinuation.ContractVersion, Kind: domaincontinuation.KindApproval, GateID: gateID, ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		ItemID: "item_" + gateID, CallID: grant.ToolCallID, ToolName: grant.ToolName,
		ToolCallItemID: domaintoolcall.ToolCallItemIDV1(securityContext.TurnID, grant.ToolCallID), Arguments: json.RawMessage(`{"path":"a.txt"}`),
		ProviderID: grant.Provider, Model: "model-a",
		ProviderRouteHash: domainsecurity.SHA256Hex([]byte("route")), ApprovalPolicy: "on-request", SandboxMode: "workspace-write", ToolScope: scope,
		LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true, ProviderStepExact: true,
		CaseSourceUnavailable: false, OrdinaryResultInputIsolated: true,
		ProviderStepPromptSHA256: domaincontinuation.CanonicalProviderStepPromptSHA256("continue"),
		ProviderNamespace:        domaincontinuation.NewProviderContinuationNamespaceV1("turn", "", 1, nil),
		SecurityContext:          securityContext, ExecutionGrant: grant, IssuedAt: now.Add(time.Second).Format(time.RFC3339Nano),
	}
}

func serviceUserInputPayloadFixture(t *testing.T, now time.Time) domaincontinuation.Payload {
	t.Helper()
	payload := servicePayloadFixture(t, now)
	arguments := json.RawMessage(`{"questions":[{"id":"confirm","question":"Continue?"}]}`)
	toolName := "request_user_input"
	toolCallID := continuationStoreTestToolCallID("service-user-input")
	scope := []string{toolName}
	scopeBody, _ := json.Marshal(scope)
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: payload.SecurityContext, Provider: payload.ProviderID, ServerIdentity: "host:builtin",
		ToolName: toolName, ToolCallID: toolCallID, ArgsHash: domainsecurity.CanonicalJSONHash(arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("user-input-schema")), ScopeHash: domainsecurity.SHA256Hex(scopeBody),
		ReadOnly: true, ApprovalState: "not_required", IssuedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	})
	gateID := domaincontinuation.GateID(
		domaincontinuation.KindUserInput, payload.ThreadID, payload.TurnID,
		payload.SecurityContext.ContextDigest, grant.GrantID, toolCallID,
	)
	payload.Kind = domaincontinuation.KindUserInput
	payload.GateID = gateID
	payload.ItemID = "item_" + gateID
	payload.CallID = toolCallID
	payload.ToolName = toolName
	payload.ToolCallItemID = domaintoolcall.ToolCallItemIDV1(payload.TurnID, toolCallID)
	payload.Arguments = arguments
	payload.ToolScope = scope
	payload.ExecutionGrant = grant
	return payload
}

func servicePayloadWithPrivateArguments(t *testing.T, now time.Time, arguments json.RawMessage) domaincontinuation.Payload {
	t.Helper()
	payload := servicePayloadFixture(t, now)
	canonicalArguments, err := domaincontinuation.CanonicalPrivateToolArgumentsV2(arguments)
	if err != nil {
		t.Fatal(err)
	}
	issuedAt, err := time.Parse(time.RFC3339Nano, payload.ExecutionGrant.IssuedAt)
	if err != nil {
		t.Fatal(err)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, payload.ExecutionGrant.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	payload.Arguments = canonicalArguments
	payload.ExecutionGrant = domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: payload.SecurityContext, Provider: payload.ExecutionGrant.Provider, ServerIdentity: payload.ExecutionGrant.ServerIdentity,
		ToolName: payload.ExecutionGrant.ToolName, ToolCallID: payload.ExecutionGrant.ToolCallID,
		ConnectionEpoch: payload.ExecutionGrant.ConnectionEpoch, ArgsHash: domainsecurity.CanonicalJSONHash(payload.Arguments),
		SchemaHash: payload.ExecutionGrant.SchemaHash, ScopeHash: payload.ExecutionGrant.ScopeHash,
		ReadOnly: payload.ExecutionGrant.ReadOnly, ApprovalState: payload.ExecutionGrant.ApprovalState,
		IssuedAt: issuedAt, ExpiresAt: expiresAt,
	})
	payload.GateID = domaincontinuation.GateID(
		payload.Kind, payload.ThreadID, payload.TurnID, payload.SecurityContext.ContextDigest,
		payload.ExecutionGrant.GrantID, payload.CallID,
	)
	payload.ItemID = "item_" + payload.GateID
	return payload
}

func servicePendingFixture(t *testing.T, now time.Time) appmodel.PendingToolCall {
	t.Helper()
	securityContext, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-pending", TurnID: "turn-pending", WorkspaceRealPath: "/workspace",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 1, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	call := domainmodel.ToolCall{ID: continuationStoreTestToolCallID("service-pending"), Name: "write_file", Arguments: json.RawMessage(`{"path":"a.txt"}`)}
	scope := []string{call.Name}
	scopeBody, _ := json.Marshal(scope)
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider-a", ServerIdentity: "host:builtin", ToolName: call.Name, ToolCallID: call.ID,
		ArgsHash: domainsecurity.CanonicalJSONHash(call.Arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")), ScopeHash: domainsecurity.SHA256Hex(scopeBody),
		ApprovalState: "pending", IssuedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	})
	providerConfig := domainmodel.TurnConfig{ProviderID: grant.Provider, Model: "model-a", APIKey: "test-only", BaseURL: "https://provider.invalid", EndpointFormat: "openai-chat-completions"}
	return appmodel.PendingToolCall{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, ProviderConfig: providerConfig, ProviderID: grant.Provider, Model: providerConfig.Model,
		Workspace: securityContext.WorkspaceRealPath, Prompt: "continue", ApprovalPolicy: "on-request", SandboxMode: "workspace-write",
		LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true, ProviderStepExact: true,
		CaseSourceUnavailable: false, OrdinaryResultInputIsolated: true,
		ProviderNamespace:    domaincontinuation.NewProviderContinuationNamespaceV1("turn", "", 1, nil),
		TerminalRecoveryKind: domaincontinuation.TerminalRecoveryAppliedV1,
		Call:                 call, ToolCallItemID: domaintoolcall.ToolCallItemIDV1(securityContext.TurnID, call.ID), ToolScope: scope,
		SecurityContext: securityContext, ExecutionGrant: grant,
	}
}

type serviceProviderResolver struct{ config domainmodel.TurnConfig }

func (resolver serviceProviderResolver) TurnConfig(_, _ string) domainmodel.TurnConfig {
	return resolver.config
}
func (resolver serviceProviderResolver) HasProvider(providerID string) bool {
	return providerID == resolver.config.ProviderID
}
func (resolver serviceProviderResolver) ValidateExecutionModel(providerID, model string) error {
	if providerID == resolver.config.ProviderID && model == resolver.config.Model {
		return nil
	}
	return errors.New("provider model mismatch")
}

func servicePendingThread(pending appmodel.PendingToolCall, receipt domaincontinuation.Receipt) map[string]any {
	contextRecord := serviceRecord(pending.SecurityContext)
	turn := map[string]any{
		"id": pending.TurnID, "status": "waiting", "securityContext": contextRecord,
		"items": []any{
			map[string]any{"id": "item-user", "kind": "user_message", "role": "user", "text": pending.Prompt},
			map[string]any{"id": pending.ToolCallItemID, "kind": "tool_call", "callId": pending.Call.ID, "toolName": pending.Call.Name, "arguments": domaintoolcall.PublicToolCallArgumentsProjectionRecordV1()},
			map[string]any{"id": receipt.Payload.ItemID, "kind": "approval", "approvalId": receipt.Payload.GateID, "toolName": pending.Call.Name, "status": "pending", "continuationReceiptId": receipt.ReceiptID},
		},
	}
	return map[string]any{"id": pending.ThreadID, "workspace": pending.Workspace, "securityState": contextRecord, "turns": []any{turn}}
}

func serviceRecord(value any) map[string]any {
	body, _ := json.Marshal(value)
	record := map[string]any{}
	_ = json.Unmarshal(body, &record)
	return record
}

func newContinuationTestStore(t *testing.T, root string) (*continuationstore.Store, error) {
	t.Helper()
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		return nil, err
	}
	return continuationstore.NewStore(root, access)
}
