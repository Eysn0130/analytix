package continuation

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	continuationstoreport "analytix.local/runtime-go/internal/ports/continuationstore"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestPreBindingV3ContinuationIsTrustedForAuditButNeverExecution(t *testing.T) {
	now := time.Date(2026, 7, 28, 3, 4, 5, 0, time.UTC)
	authority := newContinuationTestAuthority(t)
	store := newContinuationTestStore()
	service := NewService(authority, store)
	payload := continuationServicePayloadFixture(t, now)
	payload.LogicalEffect = ""
	payload.OrdinaryWork = false
	payload.ProviderStepExact = false
	payload.CaseSourceUnavailable = false
	payload.OrdinaryResultInputIsolated = false
	payload.ProviderStepPromptSHA256 = ""
	receipt := signPreBindingV3Receipt(t, authority, payload)
	store.receipts[payload.GateID] = receipt

	if err := service.ValidateRestartRecordHost(
		payload.GateID, receipt.ReceiptID, payload.Kind, payload.ThreadID, payload.TurnID, payload.ItemID,
	); err != nil {
		t.Fatalf("pre-binding V3 receipt lost restart audit authority: %v", err)
	}
	if reason := service.RestartDispositionReason(
		payload.GateID, receipt.ReceiptID, payload.Kind, payload.ThreadID, payload.TurnID, payload.ItemID,
		nil, nil, nil,
	); reason != "restart_provider_step_binding_missing" {
		t.Fatalf("pre-binding V3 restart reason = %q", reason)
	}
	if err := service.DisposeHost(
		payload.GateID, domaincontinuation.StatusRestartInvalid, "restart_provider_step_binding_missing", now.Add(time.Minute),
	); err != nil {
		t.Fatalf("pre-binding V3 receipt could not be monotonically closed: %v", err)
	}
	auditReceipt, disposition, err := service.ResolveTrustedDispositionForAudit(context.Background(), payload.GateID)
	if err != nil || auditReceipt.ReceiptID != receipt.ReceiptID ||
		disposition.ReasonCode != "restart_provider_step_binding_missing" {
		t.Fatalf("pre-binding V3 audit pair mismatch: receipt=%#v disposition=%#v err=%v", auditReceipt, disposition, err)
	}
	if _, _, err := service.ResolveTrustedDisposition(context.Background(), payload.GateID); err == nil {
		t.Fatal("pre-binding V3 audit pair authorized current execution")
	}
}

func TestCurrentV3DispositionRemainsExecutableForExactSameProcessConsumer(t *testing.T) {
	now := time.Date(2026, 7, 28, 4, 5, 6, 0, time.UTC)
	authority := newContinuationTestAuthority(t)
	store := newContinuationTestStore()
	service := NewService(authority, store)
	payload := continuationServicePayloadFixture(t, now)
	receipt, err := domaincontinuation.NewReceipt(
		payload, authority.KeyID(), authority.PublicKey(),
		func(message []byte) ([]byte, error) { return authority.Sign(context.Background(), message) },
	)
	if err != nil {
		t.Fatal(err)
	}
	store.receipts[payload.GateID] = receipt
	if err := service.DisposeHost(payload.GateID, domaincontinuation.StatusAllowed, "approval_allowed", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	resolvedReceipt, disposition, err := service.ResolveTrustedDisposition(context.Background(), payload.GateID)
	if err != nil || resolvedReceipt.ReceiptID != receipt.ReceiptID || disposition.Status != domaincontinuation.StatusAllowed {
		t.Fatalf("current exact disposition did not remain executable: receipt=%#v disposition=%#v err=%v", resolvedReceipt, disposition, err)
	}
}

func continuationServicePayloadFixture(t *testing.T, now time.Time) domaincontinuation.Payload {
	t.Helper()
	securityContext, err := securitycontexttest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-continuation-service", TurnID: "turn-continuation-service", WorkspaceRealPath: "/workspace",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 1, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	callID, err := domainsecurity.NewHostToolCallIDV1(bytes.Repeat([]byte{0x52}, domainsecurity.HostToolCallIDEntropyBytesV1))
	if err != nil {
		t.Fatal(err)
	}
	arguments := json.RawMessage(`{"path":"main.go"}`)
	toolScope := []string{"write_file"}
	scopeBody, err := json.Marshal(toolScope)
	if err != nil {
		t.Fatal(err)
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider-a", ServerIdentity: "host:builtin", ToolName: toolScope[0], ToolCallID: callID,
		ArgsHash: domainsecurity.CanonicalJSONHash(arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex(scopeBody), ApprovalState: "pending", IssuedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	})
	gateID := domaincontinuation.GateID(
		domaincontinuation.KindApproval, securityContext.ThreadID, securityContext.TurnID,
		securityContext.ContextDigest, grant.GrantID, callID,
	)
	return domaincontinuation.Payload{
		Version: domaincontinuation.ContractVersionV3, Kind: domaincontinuation.KindApproval, GateID: gateID,
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, ItemID: "item_" + gateID,
		CallID: callID, ToolName: toolScope[0], ToolCallItemID: domaintoolcall.ToolCallItemIDV1(securityContext.TurnID, callID),
		Arguments: arguments, ProviderID: "provider-a", Model: "model-a",
		ProviderRouteHash: domainsecurity.SHA256Hex([]byte("route")), ApprovalPolicy: "on-request", SandboxMode: "workspace-write",
		ToolScope: toolScope, LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true, ProviderStepExact: true,
		OrdinaryResultInputIsolated: true,
		ProviderStepPromptSHA256:    domaincontinuation.CanonicalProviderStepPromptSHA256("continue exact provider step"),
		ProviderNamespace:           domaincontinuation.NewProviderContinuationNamespaceV1("turn", "", 1, nil),
		SecurityContext:             securityContext, ExecutionGrant: grant, IssuedAt: now.Add(time.Second).Format(time.RFC3339Nano),
	}
}

func signPreBindingV3Receipt(
	t *testing.T,
	authority *continuationTestAuthority,
	payload domaincontinuation.Payload,
) domaincontinuation.Receipt {
	t.Helper()
	payloadBody, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	receipt := domaincontinuation.Receipt{
		Version: domaincontinuation.ContractVersionV3, Payload: payload,
		ReceiptID:          domainsecurity.SHA256Hex(append([]byte("analytix/gate-continuation-receipt/v3\x00"), payloadBody...)),
		AuthorityAlgorithm: domaincontinuation.AuthorityAlgorithm, AuthorityKeyID: authority.KeyID(),
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(authority.PublicKey()),
	}
	signature, err := authority.Sign(context.Background(), domaincontinuation.ReceiptSigningBytes(receipt))
	if err != nil {
		t.Fatal(err)
	}
	receipt.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	if err := domaincontinuation.ValidateReceipt(receipt); err != nil {
		t.Fatal(err)
	}
	return receipt
}

type continuationTestAuthority struct {
	private ed25519.PrivateKey
	public  ed25519.PublicKey
	keyID   string
}

func newContinuationTestAuthority(t *testing.T) *continuationTestAuthority {
	t.Helper()
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &continuationTestAuthority{private: private, public: public, keyID: domainsecurity.SHA256Hex(public)}
}

func (authority *continuationTestAuthority) KeyID() string { return authority.keyID }

func (authority *continuationTestAuthority) PublicKey() []byte {
	return append([]byte(nil), authority.public...)
}

func (authority *continuationTestAuthority) Sign(_ context.Context, message []byte) ([]byte, error) {
	return ed25519.Sign(authority.private, message), nil
}

func (authority *continuationTestAuthority) VerifyTrusted(
	_ context.Context,
	keyID string,
	publicKey, message, signature []byte,
) error {
	if keyID != authority.keyID || !bytes.Equal(publicKey, authority.public) || !ed25519.Verify(authority.public, message, signature) {
		return errors.New("continuation test signature is untrusted")
	}
	return nil
}

type continuationTestStore struct {
	receipts     map[string]domaincontinuation.Receipt
	dispositions map[string]domaincontinuation.Disposition
}

func newContinuationTestStore() *continuationTestStore {
	return &continuationTestStore{
		receipts: map[string]domaincontinuation.Receipt{}, dispositions: map[string]domaincontinuation.Disposition{},
	}
}

func (store *continuationTestStore) PutReceiptIfAbsent(_ context.Context, receipt domaincontinuation.Receipt) error {
	if _, exists := store.receipts[receipt.Payload.GateID]; exists {
		return errors.New("continuation test receipt already exists")
	}
	store.receipts[receipt.Payload.GateID] = receipt
	return nil
}

func (store *continuationTestStore) ResolveReceipt(_ context.Context, gateID string) (domaincontinuation.Receipt, error) {
	receipt, exists := store.receipts[gateID]
	if !exists {
		return domaincontinuation.Receipt{}, continuationstoreport.ErrNotFound
	}
	return receipt, nil
}

func (store *continuationTestStore) PutDispositionIfAbsent(_ context.Context, disposition domaincontinuation.Disposition) error {
	if _, exists := store.dispositions[disposition.GateID]; exists {
		return errors.New("continuation test disposition already exists")
	}
	store.dispositions[disposition.GateID] = disposition
	return nil
}

func (store *continuationTestStore) ResolveDisposition(_ context.Context, gateID string) (domaincontinuation.Disposition, error) {
	disposition, exists := store.dispositions[gateID]
	if !exists {
		return domaincontinuation.Disposition{}, continuationstoreport.ErrNotFound
	}
	return disposition, nil
}

func (store *continuationTestStore) HasRecords(context.Context) (bool, error) {
	return len(store.receipts) > 0 || len(store.dispositions) > 0, nil
}
