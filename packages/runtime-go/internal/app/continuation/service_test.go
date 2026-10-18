package continuation

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
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

func (store *continuationTestStore) VisitReceipts(ctx context.Context, visit func(domaincontinuation.Receipt) error) error {
	for _, receipt := range store.receipts {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := visit(receipt); err != nil {
			return err
		}
	}
	return nil
}
func (store *continuationTestStore) VisitDispositions(ctx context.Context, visit func(domaincontinuation.Disposition) error) error {
	for _, disposition := range store.dispositions {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := visit(disposition); err != nil {
			return err
		}
	}
	return nil
}

func TestPrivateQuestionDraftTrustedInventoryRejectsDetachedOrForgedChain(t *testing.T) {
	now := time.Now().UTC()
	store := newContinuationTestStore()
	authority := newContinuationTestAuthority(t)
	service := NewService(authority, store)
	pending := privateUserInputPendingFixture(t, now)
	inputID := domaincontinuation.GateID(domaincontinuation.KindUserInput, pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID)
	source, err := service.IssuePendingHost(domaincontinuation.KindUserInput, inputID, "item_"+inputID, pending, now)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := domaincontinuation.UserInputDraftSnapshotV1{CurrentPage: 0, Answers: []domaincontinuation.UserInputDraftAnswerV1{}}
	revision := "00000000-0000-4000-8000-000000000001"
	draft, err := service.SaveUserInputDraftHost(context.Background(), privateUserInputBinding(source), revision, nil, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.VerifyOpen(context.Background(), draft.Payload.GateID, time.Now().UTC()); err == nil {
		t.Fatal("draft restored execution authority")
	}
	if inventory, err := service.TrustedInventoryV1(context.Background()); err != nil || len(inventory.Receipts) != 1 {
		t.Fatal("draft was not filtered after validation", err)
	}
	delete(store.receipts, source.Payload.GateID)
	if _, err := service.TrustedInventoryV1(context.Background()); err == nil {
		t.Fatal("detached signed draft inventory accepted")
	}
	store.receipts[source.Payload.GateID] = source
	forged := draft
	link := *draft.Payload.UserInputDraft
	link.Snapshot = domaincontinuation.CloneUserInputDraftSnapshotV1(link.Snapshot)
	link.Snapshot.CurrentPage = 2
	forged.Payload.UserInputDraft = &link
	store.receipts[draft.Payload.GateID] = forged
	if _, err := service.TrustedInventoryV1(context.Background()); err == nil {
		t.Fatal("forged draft inventory accepted")
	}
	store.receipts[draft.Payload.GateID] = draft
	parent := "00000000-0000-4000-8000-000000000099"
	detached, err := domaincontinuation.NewUserInputDraftReceiptV1(source, "00000000-0000-4000-8000-000000000002", &parent, snapshot, draft.Payload.UserInputDraft.CaptureSetSHA256, time.Now().UTC(), authority.KeyID(), authority.PublicKey(), func(body []byte) ([]byte, error) { return authority.Sign(context.Background(), body) })
	if err != nil {
		t.Fatal(err)
	}
	store.receipts[detached.Payload.GateID] = detached
	if _, err := service.TrustedInventoryV1(context.Background()); err == nil {
		t.Fatal("signed orphan parent was accepted as a partial inventory")
	}
}

func TestUserInputHistoryReadsOnlySignedOriginalArguments(t *testing.T) {
	now := time.Now().UTC()
	store := newContinuationTestStore()
	service := NewService(newContinuationTestAuthority(t), store)
	pending := privateUserInputPendingFixture(t, now)
	inputID := domaincontinuation.GateID(domaincontinuation.KindUserInput, pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID)
	source, err := service.IssuePendingHost(domaincontinuation.KindUserInput, inputID, "item_"+inputID, pending, now)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := service.ResolveUserInputLocalDisplayOriginHost(context.Background(), inputID)
	if err != nil || !reflect.DeepEqual(binding, privateUserInputBinding(source)) {
		t.Fatal("original request identity was not verified")
	}
	draft, err := service.SaveUserInputDraftHost(context.Background(), binding, "00000000-0000-4000-8000-000000000001", nil,
		domaincontinuation.UserInputDraftSnapshotV1{CurrentPage: 1, Answers: []domaincontinuation.UserInputDraftAnswerV1{{QuestionID: inputID + "_2", OptionIDs: []string{}, CustomText: "synthetic later private draft"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, submission := privateUserInputRequestFixture(t, pending, inputID)
	if _, err := service.DisposePendingUserInputHost(inputID, now.Add(time.Second), pending, submission); err != nil {
		t.Fatal(err)
	}
	view, err := service.ReadUserInputHistoryHost(context.Background(), binding)
	if err != nil || string(view.Arguments) != string(source.Payload.Arguments) || view.CallID != source.Payload.CallID ||
		view.Status != "pending" || view.Draft != nil || view.DraftRevision != nil || view.Submission != nil || view.LateReply != nil || view.TimedWait != nil {
		t.Fatal("immutable reader exposed later state or lost signed arguments")
	}
	foreign := binding
	foreign.ThreadID = "child"
	if _, err := service.ReadUserInputHistoryHost(context.Background(), foreign); err == nil {
		t.Fatal("original receipt rebound to an inherited tuple")
	}
	if _, err := service.ResolveUserInputLocalDisplayOriginHost(context.Background(), draft.Payload.GateID); err == nil {
		t.Fatal("draft derivative became an original request")
	}
	forged := source
	forged.Payload.Arguments = json.RawMessage(`{"prompt":"synthetic altered original"}`)
	store.receipts[inputID] = forged
	if _, err := service.ReadUserInputHistoryHost(context.Background(), binding); err == nil {
		t.Fatal("history accepted a modified signed original")
	}
}

type draftReceiptFailureStoreV1 struct {
	*continuationTestStore
	after  bool
	writes int
}

func (store *draftReceiptFailureStoreV1) PutReceiptIfAbsent(ctx context.Context, receipt domaincontinuation.Receipt) error {
	if receipt.Payload.UserInputDraft == nil {
		return store.continuationTestStore.PutReceiptIfAbsent(ctx, receipt)
	}
	store.writes++
	if store.after {
		if err := store.continuationTestStore.PutReceiptIfAbsent(ctx, receipt); err != nil {
			return err
		}
	}
	return errors.New("synthetic draft write failure")
}
func TestPrivateQuestionDraftFailedWriteDoesNotAcknowledge(t *testing.T) {
	for _, after := range []bool{false, true} {
		store := &draftReceiptFailureStoreV1{continuationTestStore: newContinuationTestStore(), after: after}
		authority := newContinuationTestAuthority(t)
		service := NewService(authority, store)
		now := time.Now().UTC()
		pending := privateUserInputPendingFixture(t, now)
		inputID := domaincontinuation.GateID(domaincontinuation.KindUserInput, pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID)
		source, err := service.IssuePendingHost(domaincontinuation.KindUserInput, inputID, "item_"+inputID, pending, now)
		if err != nil {
			t.Fatal(err)
		}
		snapshot := domaincontinuation.UserInputDraftSnapshotV1{CurrentPage: 0, Answers: []domaincontinuation.UserInputDraftAnswerV1{}}
		revision := "00000000-0000-4000-8000-000000000001"
		if _, err := service.SaveUserInputDraftHost(context.Background(), privateUserInputBinding(source), revision, nil, snapshot); err == nil || store.writes != 1 {
			t.Fatal("failed CAS became ACK or was repeated")
		}
		if after {
			if _, err := service.SaveUserInputDraftHost(context.Background(), privateUserInputBinding(source), revision, nil, snapshot); err != nil || store.writes != 1 {
				t.Fatal("explicit exact retry did not reuse visible proof", err)
			}
		}
	}
}

func TestPrivateQuestionDraftRetainedByteCapacityRejectsBeforeAnotherCAS(t *testing.T) {
	store := newContinuationTestStore()
	authority := newContinuationTestAuthority(t)
	service := NewService(authority, store)
	now := time.Now().UTC()
	pending := privateUserInputPendingFixture(t, now)
	inputID := domaincontinuation.GateID(domaincontinuation.KindUserInput, pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID)
	source, err := service.IssuePendingHost(domaincontinuation.KindUserInput, inputID, "item_"+inputID, pending, now)
	if err != nil {
		t.Fatal(err)
	}
	binding := privateUserInputBinding(source)
	snapshot := domaincontinuation.UserInputDraftSnapshotV1{CurrentPage: 0, Answers: []domaincontinuation.UserInputDraftAnswerV1{{QuestionID: inputID + "_2", OptionIDs: []string{}, CustomText: ""}}}
	// Include JSON overhead: each snapshot occupies exactly 4 MiB, making
	// two real saves reach the cumulative 8 MiB limit rather than approach it.
	snapshot.Answers[0].CustomText = strings.Repeat("x", domaincontinuation.MaxUserInputDraftBytesV1-draftSnapshotBytesV1(snapshot))
	if draftSnapshotBytesV1(snapshot) != domaincontinuation.MaxUserInputDraftBytesV1 {
		t.Fatal("fixture did not reach the exact snapshot byte limit")
	}
	first := "00000000-0000-4000-8000-000000000001"
	second := "00000000-0000-4000-8000-000000000002"
	firstSaved, err := service.SaveUserInputDraftHost(context.Background(), binding, first, nil, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := service.SaveUserInputDraftHost(context.Background(), binding, second, &first, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	chain, retained, err := service.readUserInputDraftChainV1(context.Background(), source)
	if err != nil || len(chain) != 2 || retained != domaincontinuation.MaxUserInputDraftRetainedBytesV1 {
		t.Fatal("fixture did not reach the exact retained byte budget", err)
	}
	count := len(store.receipts)
	third := "00000000-0000-4000-8000-000000000003"
	empty := domaincontinuation.UserInputDraftSnapshotV1{CurrentPage: 0, Answers: []domaincontinuation.UserInputDraftAnswerV1{}}
	if _, err := service.SaveUserInputDraftHost(context.Background(), binding, third, &second, empty); !errors.Is(err, ErrUserInputDraftCapacityV1) || len(store.receipts) != count {
		t.Fatal("full byte budget wrote another immutable snapshot", err)
	}
	if _, err := store.ResolveReceipt(context.Background(), domaincontinuation.UserInputDraftSlotGateIDV1(source.ReceiptID, &second)); !errors.Is(err, continuationstoreport.ErrNotFound) {
		t.Fatal("capacity failure occupied the next parent slot", err)
	}
	view, err := service.ReadUserInputLocalDisplayHost(context.Background(), binding)
	if err != nil || view.Status != "pending" || view.DraftRevision == nil || *view.DraftRevision != second || view.Draft == nil || !reflect.DeepEqual(*view.Draft, snapshot) {
		t.Fatal("full byte budget made the existing draft unreadable", err)
	}
	retry, err := service.SaveUserInputDraftHost(context.Background(), binding, second, &first, snapshot)
	if err != nil || !reflect.DeepEqual(saved, retry) || len(store.receipts) != count {
		t.Fatal("full byte budget blocked or changed exact retry", err)
	}
	oldRetry, err := service.SaveUserInputDraftHost(context.Background(), binding, first, nil, snapshot)
	if err != nil || !reflect.DeepEqual(firstSaved, oldRetry) || len(store.receipts) != count {
		t.Fatal("full byte budget blocked the earliest valid exact retry", err)
	}
	if _, err := service.SaveUserInputDraftHost(context.Background(), binding, third, &first, empty); !errors.Is(err, ErrUserInputDraftConflictV1) {
		t.Fatal("full byte budget obscured stale parent conflict", err)
	}
	if inventory, err := service.TrustedInventoryV1(context.Background()); err != nil || len(inventory.Receipts) != 1 {
		t.Fatal("full byte budget invalidated existing signed inventory", err)
	}
	_, submission := privateUserInputRequestFixture(t, pending, inputID)
	if _, err := service.DisposePendingUserInputHost(inputID, time.Now().UTC(), pending, submission); err != nil {
		t.Fatal("draft byte budget blocked ordinary answer submission", err)
	}
	view, err = service.ReadUserInputLocalDisplayHost(context.Background(), binding)
	if err != nil || view.Status != "submitted" || view.Submission == nil || view.Draft != nil || view.DraftRevision == nil || *view.DraftRevision != second || len(store.receipts) != count {
		t.Fatal("ordinary submit failed to clear full-budget draft content", err)
	}
}

func TestPrivateQuestionDraftRevisionCapacityKeepsReadRetryAndOrdinarySubmit(t *testing.T) {
	store := newContinuationTestStore()
	authority := newContinuationTestAuthority(t)
	service := NewService(authority, store)
	now := time.Now().UTC().Add(-time.Minute)
	pending := privateUserInputPendingFixture(t, now)
	inputID := domaincontinuation.GateID(domaincontinuation.KindUserInput, pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID)
	source, err := service.IssuePendingHost(domaincontinuation.KindUserInput, inputID, "item_"+inputID, pending, now)
	if err != nil {
		t.Fatal(err)
	}
	binding := privateUserInputBinding(source)
	snapshot := domaincontinuation.UserInputDraftSnapshotV1{CurrentPage: 0, Answers: []domaincontinuation.UserInputDraftAnswerV1{}}
	captureSet, err := service.userInputDraftCaptureSetV1(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	var parent *string
	var firstReceipt domaincontinuation.Receipt
	// Seed valid signed audit history through the real receipt constructor
	// and existing CAS, then exercise the actual Host producer at the limit.
	// This avoids quadratic repeated saves and makes no timeout/queue claim.
	for i := 1; i < domaincontinuation.MaxUserInputDraftRevisionsV1; i++ {
		revision := fmt.Sprintf("00000000-0000-4000-8000-%012x", i)
		receipt, err := domaincontinuation.NewUserInputDraftReceiptV1(source, revision, parent, snapshot, captureSet, now.Add(time.Second), authority.KeyID(), authority.PublicKey(), func(body []byte) ([]byte, error) { return authority.Sign(context.Background(), body) })
		if err != nil {
			t.Fatal(err)
		}
		if err := store.PutReceiptIfAbsent(context.Background(), receipt); err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			firstReceipt = receipt
		}
		value := revision
		parent = &value
	}
	last := fmt.Sprintf("00000000-0000-4000-8000-%012x", domaincontinuation.MaxUserInputDraftRevisionsV1)
	saved, err := service.SaveUserInputDraftHost(context.Background(), binding, last, parent, snapshot)
	if err != nil {
		t.Fatal("last allowed revision failed", err)
	}
	chain, _, err := service.readUserInputDraftChainV1(context.Background(), source)
	if err != nil || len(chain) != domaincontinuation.MaxUserInputDraftRevisionsV1 {
		t.Fatal("full revision chain could not be read", err)
	}
	count := len(store.receipts)
	next := fmt.Sprintf("00000000-0000-4000-8000-%012x", domaincontinuation.MaxUserInputDraftRevisionsV1+1)
	if _, err := service.SaveUserInputDraftHost(context.Background(), binding, next, &last, snapshot); !errors.Is(err, ErrUserInputDraftCapacityV1) || len(store.receipts) != count {
		t.Fatal("revision budget admitted another child", err)
	}
	if _, err := store.ResolveReceipt(context.Background(), domaincontinuation.UserInputDraftSlotGateIDV1(source.ReceiptID, &last)); !errors.Is(err, continuationstoreport.ErrNotFound) {
		t.Fatal("revision failure occupied the next slot", err)
	}
	view, err := service.ReadUserInputLocalDisplayHost(context.Background(), binding)
	if err != nil || view.Draft == nil || view.DraftRevision == nil || *view.DraftRevision != last || !reflect.DeepEqual(*view.Draft, snapshot) {
		t.Fatal("full revision budget made the existing draft unreadable", err)
	}
	retry, err := service.SaveUserInputDraftHost(context.Background(), binding, last, parent, snapshot)
	if err != nil || !reflect.DeepEqual(saved, retry) || len(store.receipts) != count {
		t.Fatal("full revision budget changed exact retry", err)
	}
	if retry, err := service.SaveUserInputDraftHost(context.Background(), binding, firstReceipt.Payload.UserInputDraft.Revision, nil, snapshot); err != nil || !reflect.DeepEqual(firstReceipt, retry) || len(store.receipts) != count {
		t.Fatal("full revision budget blocked the earliest valid exact retry", err)
	}
	if _, err := service.SaveUserInputDraftHost(context.Background(), binding, next, nil, snapshot); !errors.Is(err, ErrUserInputDraftConflictV1) {
		t.Fatal("full revision budget obscured stale parent conflict", err)
	}
	if inventory, err := service.TrustedInventoryV1(context.Background()); err != nil || len(inventory.Receipts) != 1 || inventory.Receipts[0].ReceiptID != source.ReceiptID {
		t.Fatal("valid full-budget draft inventory was unavailable or became execution gates", err)
	}
	_, submission := privateUserInputRequestFixture(t, pending, inputID)
	if _, err := service.DisposePendingUserInputHost(inputID, time.Now().UTC(), pending, submission); err != nil {
		t.Fatal("draft revision budget blocked ordinary answer submission", err)
	}
	view, err = service.ReadUserInputLocalDisplayHost(context.Background(), binding)
	if err != nil || view.Status != "submitted" || view.Submission == nil || view.Draft != nil || view.DraftRevision == nil || *view.DraftRevision != last || len(store.receipts) != count {
		t.Fatal("ordinary submit failed to clear the full revision draft", err)
	}
}
