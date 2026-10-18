package continuation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	continuationadapter "analytix.local/runtime-go/internal/adapters/outbound/continuationstore"
	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	controlapp "analytix.local/runtime-go/internal/app/control"
	appmodel "analytix.local/runtime-go/internal/app/model"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func privateUserInputPendingFixture(t *testing.T, now time.Time) appmodel.PendingToolCall {
	t.Helper()
	base := continuationServicePayloadFixture(t, now)
	arguments := json.RawMessage(`{"prompt":"  Original prompt  ","questions":[{"question":"  Pick  ","multi_select":true,"options":[{"label":" Same ","description":" First "},{"label":" Same ","description":" Second "}]},{"question":" Custom "},{"question":" Skip "}]}`)
	scope := []string{"request_user_input"}
	scopeBody, _ := json.Marshal(scope)
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{Context: base.SecurityContext, Provider: base.ProviderID, ServerIdentity: "host:builtin", ToolName: scope[0], ToolCallID: base.CallID, ArgsHash: domainsecurity.CanonicalJSONHash(arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("user-input-schema")), ScopeHash: domainsecurity.SHA256Hex(scopeBody), ReadOnly: true, ApprovalState: "not_required", IssuedAt: now, ExpiresAt: now.Add(15 * time.Minute)})
	config := domainmodel.TurnConfig{ProviderID: base.ProviderID, Model: base.Model, APIKey: "synthetic-test-only", BaseURL: "https://provider.invalid", EndpointFormat: "openai-chat-completions"}
	return appmodel.PendingToolCall{ThreadID: base.ThreadID, TurnID: base.TurnID, Workspace: base.SecurityContext.WorkspaceRealPath, ProviderConfig: config, ProviderID: config.ProviderID, Model: config.Model, Prompt: "continue exact provider step", ApprovalPolicy: base.ApprovalPolicy, SandboxMode: base.SandboxMode, Call: domainmodel.ToolCall{ID: base.CallID, Name: scope[0], Arguments: arguments}, ToolCallItemID: base.ToolCallItemID, ToolScope: scope, LogicalEffect: base.LogicalEffect, OrdinaryWork: true, ProviderStepExact: true, OrdinaryResultInputIsolated: true, ProviderNamespace: base.ProviderNamespace, SecurityContext: base.SecurityContext, ExecutionGrant: grant}
}

func privateUserInputRequestFixture(t *testing.T, pending appmodel.PendingToolCall, gateID string) (controlapp.UserInputResponse, domaincontinuation.UserInputSubmissionV1) {
	t.Helper()
	text := "  original@example.invalid\n  "
	request := controlapp.UserInputResponse{InputID: gateID, StructuredAnswers: []controlapp.UserInputAnswer{{ID: gateID + "_1", Kind: "answer", OptionIDs: []string{gateID + "_1_option_2", gateID + "_1_option_1"}}, {ID: gateID + "_2", Kind: "answer", OptionIDs: []string{}, CustomText: &text}, {ID: gateID + "_3", Kind: "skip"}}}
	var args map[string]any
	if json.Unmarshal(pending.Call.Arguments, &args) != nil {
		t.Fatal("args")
	}
	_, submission, err := controlapp.NormalizeAndCaptureUserInputAnswers(request, toolcatalogapp.UserInputQuestions(args, gateID, "Original prompt"), "")
	if err != nil || submission == nil {
		t.Fatal(err)
	}
	return request, *submission
}

func privateUserInputBinding(receipt domaincontinuation.Receipt) UserInputLocalDisplayBindingV1 {
	return UserInputLocalDisplayBindingV1{ThreadID: receipt.Payload.ThreadID, TurnID: receipt.Payload.TurnID, ItemID: receipt.Payload.ItemID, InputID: receipt.Payload.GateID, ReceiptID: receipt.ReceiptID, SecurityContext: receipt.Payload.SecurityContext}
}

func TestPrivateUserInputSubmissionPersistsAndReloadsSameSignedAuthority(t *testing.T) {
	root := t.TempDir()
	storeRoot := filepath.Join(root, "continuations")
	authorityPath := filepath.Join(root, "authority", "continuation.json")
	access, err := privatecastest.NewAccessAuthority(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	store, err := continuationadapter.NewStore(storeRoot, access)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := finalauthorityadapter.OpenOrCreateFileAuthority(authorityPath, false)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(authority, store)
	now := time.Now().UTC()
	pending := privateUserInputPendingFixture(t, now)
	gateID := controlapp.SecureGateID(domaincontinuation.KindUserInput, pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID)
	receipt, err := service.IssuePendingHost(domaincontinuation.KindUserInput, gateID, "item_"+gateID, pending, now)
	if err != nil {
		t.Fatal(err)
	}
	_, submission := privateUserInputRequestFixture(t, pending, gateID)
	disposition, err := service.DisposePendingUserInputHost(gateID, now.Add(time.Minute), pending, submission)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopenedAccess, err := privatecastest.NewAccessAuthority(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	reopenedStore, err := continuationadapter.NewStore(storeRoot, reopenedAccess)
	if err != nil {
		t.Fatal(err)
	}
	defer reopenedStore.Close()
	reopenedAuthority, err := finalauthorityadapter.OpenOrCreateFileAuthority(authorityPath, true)
	if err != nil {
		t.Fatal(err)
	}
	restarted := NewService(reopenedAuthority, reopenedStore)
	view, err := restarted.ReadUserInputLocalDisplayHost(context.Background(), privateUserInputBinding(receipt))
	if err != nil || view.Status != "submitted" || view.AnswerState != "available" || view.CallID != receipt.Payload.CallID || view.Submission == nil || !userInputSubmissionsEqual(*view.Submission, submission) {
		t.Fatal("disk reload lost original authority", err)
	}
	var args map[string]any
	if json.Unmarshal(view.Arguments, &args) != nil || args["prompt"] != "  Original prompt  " {
		t.Fatal("readback projected original request")
	}
	_, resolved, err := restarted.ResolveTrustedDispositionForAudit(context.Background(), gateID)
	if err != nil || resolved.DispositionID != disposition.DispositionID {
		t.Fatal("reload changed signed disposition", err)
	}
	view.Submission.Answers[0] = 'x'
	view.Arguments[0] = 'x'
	second, err := restarted.ReadUserInputLocalDisplayHost(context.Background(), privateUserInputBinding(receipt))
	if err != nil || !userInputSubmissionsEqual(*second.Submission, submission) || second.Arguments[0] != '{' {
		t.Fatal("readback exposed mutable store aliases")
	}
	repeated, err := restarted.DisposePendingUserInputHost(gateID, now.Add(30*time.Minute), pending, submission)
	if err != nil || repeated.DispositionID != disposition.DispositionID {
		t.Fatal("exact retry changed authority", err)
	}
	different := domaincontinuation.CloneUserInputSubmissionV1(submission)
	different.Answers = bytes.Replace(different.Answers, []byte("original@example.invalid"), []byte("different@example.invalid"), 1)
	if _, err := restarted.DisposePendingUserInputHost(gateID, now.Add(2*time.Minute), pending, different); !errors.Is(err, ErrReceiptConsumed) {
		t.Fatal("different answers reused consumed authority", err)
	}
}

func TestPrivateUserInputReadbackRejectsForeignBindingAndForgedCapture(t *testing.T) {
	now := time.Now().UTC()
	store := newContinuationTestStore()
	authority := newContinuationTestAuthority(t)
	service := NewService(authority, store)
	pending := privateUserInputPendingFixture(t, now)
	gateID := controlapp.SecureGateID(domaincontinuation.KindUserInput, pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID)
	receipt, err := service.IssuePendingHost(domaincontinuation.KindUserInput, gateID, "item_"+gateID, pending, now)
	if err != nil {
		t.Fatal(err)
	}
	binding := privateUserInputBinding(receipt)
	pendingView, err := service.ReadUserInputLocalDisplayHost(context.Background(), binding)
	if err != nil || pendingView.Status != "pending" || pendingView.Submission != nil {
		t.Fatal("pending readback unavailable", err)
	}
	for name, change := range map[string]func(*UserInputLocalDisplayBindingV1){"thread": func(v *UserInputLocalDisplayBindingV1) { v.ThreadID = "foreign" }, "turn": func(v *UserInputLocalDisplayBindingV1) { v.TurnID = "foreign" }, "item": func(v *UserInputLocalDisplayBindingV1) { v.ItemID = "foreign" }, "input": func(v *UserInputLocalDisplayBindingV1) { v.InputID = "foreign" }, "receipt": func(v *UserInputLocalDisplayBindingV1) { v.ReceiptID = domainsecurity.SHA256Hex([]byte("foreign")) }, "frozen witness": func(v *UserInputLocalDisplayBindingV1) {
		v.SecurityContext.ContextDigest = domainsecurity.SHA256Hex([]byte("foreign"))
	}} {
		t.Run(name, func(t *testing.T) {
			wrong := binding
			change(&wrong)
			if _, err := service.ReadUserInputLocalDisplayHost(context.Background(), wrong); err == nil {
				t.Fatal("foreign readback admitted")
			}
		})
	}
	_, submission := privateUserInputRequestFixture(t, pending, gateID)
	original, err := service.DisposePendingUserInputHost(gateID, now.Add(time.Minute), pending, submission)
	if err != nil {
		t.Fatal(err)
	}
	forged := original
	copy := domaincontinuation.CloneUserInputSubmissionV1(*original.UserInputSubmission)
	copy.Answers = bytes.Replace(copy.Answers, []byte("original@example.invalid"), []byte("forged@example.invalid"), 1)
	forged.UserInputSubmission = &copy
	// A self-consistent digest is insufficient without the installation signature.
	forged.DispositionID = domainsecurity.SHA256Hex([]byte("forged"))
	store.dispositions[gateID] = forged
	if _, err := service.ReadUserInputLocalDisplayHost(context.Background(), binding); err == nil {
		t.Fatal("forged original answer admitted")
	}
	store.dispositions[gateID] = original
	mismatched := domaincontinuation.CloneUserInputSubmissionV1(submission)
	mismatched.Answers = bytes.Replace(mismatched.Answers, []byte(gateID+"_1_option_2"), []byte(gateID+"_1_option_3"), 1)
	if validateUserInputSubmissionForReceipt(receipt, mismatched) == nil {
		t.Fatal("nonexistent option acquired receipt authority")
	}
	legacy := domaincontinuation.UserInputSubmissionV1{SchemaVersion: 1, Format: "legacy", Answers: json.RawMessage(`[{"id":"q1","questionId":"foreign","value":"answer"}]`)}
	if validateUserInputSubmissionForReceipt(receipt, legacy) == nil {
		t.Fatal("legacy binding guessed a question")
	}
}

func TestPrivateUserInputHistoricalMissingOrCancelledAnswersStayUnavailable(t *testing.T) {
	for _, status := range []string{domaincontinuation.StatusSubmitted, domaincontinuation.StatusCancelled, domaincontinuation.StatusRestartInvalid} {
		t.Run(status, func(t *testing.T) {
			now := time.Now().UTC().Add(-time.Hour)
			store := newContinuationTestStore()
			authority := newContinuationTestAuthority(t)
			service := NewService(authority, store)
			pending := privateUserInputPendingFixture(t, now)
			gateID := controlapp.SecureGateID(domaincontinuation.KindUserInput, pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID)
			receipt, err := service.IssuePendingHost(domaincontinuation.KindUserInput, gateID, "item_"+gateID, pending, now)
			if err != nil {
				t.Fatal(err)
			}
			reason := "user_input_submitted"
			if status == domaincontinuation.StatusCancelled {
				reason = "user_input_cancelled"
			}
			if status == domaincontinuation.StatusRestartInvalid {
				reason = "restart_nonresumable"
			}
			if err := service.DisposeHost(gateID, status, reason, now.Add(time.Minute)); err != nil {
				t.Fatal(err)
			}
			view, err := service.ReadUserInputLocalDisplayHost(context.Background(), privateUserInputBinding(receipt))
			if err != nil || view.Submission != nil {
				t.Fatal("historical readback failed or invented original answers", err)
			}
			if status == domaincontinuation.StatusSubmitted {
				if view.Status != "submitted" || view.AnswerState != "unavailable" {
					t.Fatal("old answer was inferred")
				}
			} else if view.Status != "cancelled" || view.AnswerState != "" {
				t.Fatal("cancelled gate acquired answer state")
			}
		})
	}
}

type privateSubmissionFailureStore struct {
	*continuationTestStore
	failAfterCommit bool
	writes          int
}

func (store *privateSubmissionFailureStore) PutDispositionIfAbsent(ctx context.Context, value domaincontinuation.Disposition) error {
	store.writes++
	if store.failAfterCommit {
		if err := store.continuationTestStore.PutDispositionIfAbsent(ctx, value); err != nil {
			return err
		}
	}
	return errors.New("injected private disposition commit failure")
}

func TestPrivateUserInputCommitFailureCannotBecomeSuccessfulAcknowledgement(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "before commit", true: "after commit"}[after], func(t *testing.T) {
			now := time.Now().UTC()
			store := &privateSubmissionFailureStore{continuationTestStore: newContinuationTestStore(), failAfterCommit: after}
			service := NewService(newContinuationTestAuthority(t), store)
			pending := privateUserInputPendingFixture(t, now)
			gateID := controlapp.SecureGateID(domaincontinuation.KindUserInput, pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID)
			receipt, err := service.IssuePendingHost(domaincontinuation.KindUserInput, gateID, "item_"+gateID, pending, now)
			if err != nil {
				t.Fatal(err)
			}
			_, submission := privateUserInputRequestFixture(t, pending, gateID)
			if _, err := service.DisposePendingUserInputHost(gateID, now.Add(time.Minute), pending, submission); err == nil {
				t.Fatal("failed write became successful acknowledgement")
			}
			if store.writes != 1 {
				t.Fatal("failed attempt repeated CAS")
			}
			view, err := service.ReadUserInputLocalDisplayHost(context.Background(), privateUserInputBinding(receipt))
			if err != nil {
				t.Fatal(err)
			}
			if after {
				if view.Status != "submitted" || view.Submission == nil || !reflect.DeepEqual(view.Submission.Answers, submission.Answers) {
					t.Fatal("visible commit lost its matching original answer")
				}
			} else if view.Status != "pending" || view.Submission != nil {
				t.Fatal("failed precommit consumed authority")
			}
		})
	}
}

func TestLateQuestionCaptureIsAuditOnlyPrivateCASAndReloads(t *testing.T) {
	root := t.TempDir()
	storeRoot, authorityPath := filepath.Join(root, "continuations"), filepath.Join(root, "authority", "continuation.json")
	access, err := privatecastest.NewAccessAuthority(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	store, err := continuationadapter.NewStore(storeRoot, access)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := finalauthorityadapter.OpenOrCreateFileAuthority(authorityPath, false)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(authority, store)
	now := time.Now().Add(-2 * time.Second).UTC()
	pending := privateUserInputPendingFixture(t, now)
	pending.UserInputTimedWait = &domaincontinuation.UserInputTimedWaitV1{TimeoutSeconds: 1}
	inputID := controlapp.SecureGateID(domaincontinuation.KindUserInput, pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID)
	receipt, err := service.IssuePendingHost(domaincontinuation.KindUserInput, inputID, "item_"+inputID, pending, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.DisposePendingHost(inputID, domaincontinuation.StatusContinued, "user_input_continued", now.Add(time.Second), pending); err != nil {
		t.Fatal(err)
	}
	_, submission := privateUserInputRequestFixture(t, pending, inputID)
	binding := privateUserInputBinding(receipt)
	attemptID := "00000000-0000-4000-8000-000000000001"
	capture, err := service.CaptureLateUserInputSubmissionHost(context.Background(), binding, attemptID, submission)
	if err != nil {
		t.Fatal(err)
	}
	if domaincontinuation.ValidateReceiptForExecution(capture) == nil {
		t.Fatal("private late capture acquired execution")
	}
	if _, _, err := service.ResolveTrustedDisposition(context.Background(), capture.Payload.GateID); err == nil {
		t.Fatal("capture resumed original call")
	}
	repeated, err := service.CaptureLateUserInputSubmissionHost(context.Background(), binding, attemptID, submission)
	if err != nil || repeated.ReceiptID != capture.ReceiptID {
		t.Fatal("stable retry changed capture", err)
	}
	changed := domaincontinuation.CloneUserInputSubmissionV1(submission)
	changed.Answers = bytes.Replace(changed.Answers, []byte("original@example.invalid"), []byte("different@example.invalid"), 1)
	if _, err := service.CaptureLateUserInputSubmissionHost(context.Background(), binding, attemptID, changed); !errors.Is(err, ErrReceiptConsumed) {
		t.Fatal("attempt reused for different answers", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	access, err = privatecastest.NewAccessAuthority(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := continuationadapter.NewStore(storeRoot, access)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	authority, err = finalauthorityadapter.OpenOrCreateFileAuthority(authorityPath, true)
	if err != nil {
		t.Fatal(err)
	}
	restarted := NewService(authority, reopened)
	raw, err := restarted.ReadLateUserInputSubmissionHost(context.Background(), binding, attemptID, capture.ReceiptID)
	if err != nil || !userInputSubmissionsEqual(raw, submission) {
		t.Fatal("reload reconstructed or lost raw answers", err)
	}
	inventory, err := restarted.TrustedInventoryV1(context.Background())
	if err != nil || len(inventory.Receipts) != 1 || len(inventory.Dispositions) != 1 || inventory.Receipts[0].ReceiptID != receipt.ReceiptID {
		t.Fatal("audit capture became a recovered Gate", err)
	}
	wrong := binding
	wrong.ThreadID = "foreign-thread"
	if _, err := restarted.ReadLateUserInputSubmissionHost(context.Background(), wrong, attemptID, capture.ReceiptID); err == nil {
		t.Fatal("late raw crossed original view binding")
	}
}

func TestLateQuestionCaptureWriteErrorCannotAcknowledgeEvenAfterCommit(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "before", true: "after"}[after], func(t *testing.T) {
			base := newContinuationTestStore()
			authority := newContinuationTestAuthority(t)
			original := NewService(authority, base)
			now := time.Now().Add(-2 * time.Second).UTC()
			pending := privateUserInputPendingFixture(t, now)
			pending.UserInputTimedWait = &domaincontinuation.UserInputTimedWaitV1{TimeoutSeconds: 1}
			inputID := controlapp.SecureGateID(domaincontinuation.KindUserInput, pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID)
			receipt, err := original.IssuePendingHost(domaincontinuation.KindUserInput, inputID, "item_"+inputID, pending, now)
			if err != nil {
				t.Fatal(err)
			}
			if err := original.DisposePendingHost(inputID, domaincontinuation.StatusContinued, "user_input_continued", now.Add(time.Second), pending); err != nil {
				t.Fatal(err)
			}
			failing := &privateSubmissionFailureStore{continuationTestStore: base, failAfterCommit: after}
			service := NewService(authority, failing)
			_, submission := privateUserInputRequestFixture(t, pending, inputID)
			if _, err := service.CaptureLateUserInputSubmissionHost(context.Background(), privateUserInputBinding(receipt), "00000000-0000-4000-8000-000000000001", submission); err == nil {
				t.Fatal("write error acknowledged late capture")
			}
			if failing.writes != 1 {
				t.Fatal("same attempt repeated failed CAS")
			}
		})
	}
}

func TestIncompleteLateCaptureIsVerifiedButNeverRecoveredAsGate(t *testing.T) {
	root := t.TempDir()
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := continuationadapter.NewStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	authority := newContinuationTestAuthority(t)
	service := NewService(authority, store)
	now := time.Now().Add(-2 * time.Second).UTC()
	pending := privateUserInputPendingFixture(t, now)
	pending.UserInputTimedWait = &domaincontinuation.UserInputTimedWaitV1{TimeoutSeconds: 1}
	inputID := controlapp.SecureGateID(domaincontinuation.KindUserInput, pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID)
	source, err := service.IssuePendingHost(domaincontinuation.KindUserInput, inputID, "item_"+inputID, pending, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.DisposePendingHost(inputID, domaincontinuation.StatusContinued, "user_input_continued", now.Add(time.Second), pending); err != nil {
		t.Fatal(err)
	}
	capture, err := domaincontinuation.NewUserInputReplyCaptureReceiptV1(source, "00000000-0000-4000-8000-000000000001", time.Now().UTC(), authority.KeyID(), authority.PublicKey(), func(body []byte) ([]byte, error) { return authority.Sign(context.Background(), body) })
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutReceiptIfAbsent(context.Background(), capture); err != nil {
		t.Fatal(err)
	}
	inventory, err := service.TrustedInventoryV1(context.Background())
	if err != nil || len(inventory.Receipts) != 1 || len(inventory.Dispositions) != 1 {
		t.Fatal("incomplete capture created pending Gate", err)
	}
	latest, err := service.ReadLatestLateUserInputSubmissionHost(context.Background(), privateUserInputBinding(source))
	if err != nil || latest != nil {
		t.Fatal("missing disposition invented raw answers", err)
	}
	// Even an installation-signed audit capture cannot substitute a different
	// original receipt. Signature verification precedes, rather than replaces,
	// the exact source linkage validation used at process startup.
	payload := capture.Payload
	link := *payload.UserInputReplyCapture
	link.ClientUserMessageID = "00000000-0000-4000-8000-000000000002"
	link.SourceReceiptID = domainsecurity.SHA256Hex([]byte("wrong original"))
	payload.UserInputReplyCapture = &link
	payload.GateID = domaincontinuation.UserInputReplyCaptureGateIDV1(link.SourceInputID, link.SourceReceiptID, link.ClientUserMessageID)
	payload.ItemID = "item_" + payload.GateID
	forged := signPreBindingV3Receipt(t, authority, payload)
	if err := store.PutReceiptIfAbsent(context.Background(), forged); err != nil {
		t.Fatal(err)
	}
	if err := VerifyTrustedInventoryV1(context.Background(), store, authority); err == nil {
		t.Fatal("trusted signature bypassed capture parent binding")
	}
}

func TestPrivateQuestionDraftDiskReopenExactRetryAndLogicalClearing(t *testing.T) {
	root := t.TempDir()
	storeRoot := filepath.Join(root, "continuations")
	signerPath := filepath.Join(root, "authority", "continuation.json")
	access, err := privatecastest.NewAccessAuthority(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	store, err := continuationadapter.NewStore(storeRoot, access)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := finalauthorityadapter.OpenOrCreateFileAuthority(signerPath, false)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(authority, store)
	now := time.Now().UTC()
	pending := privateUserInputPendingFixture(t, now)
	inputID := controlapp.SecureGateID(domaincontinuation.KindUserInput, pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID)
	source, err := service.IssuePendingHost(domaincontinuation.KindUserInput, inputID, "item_"+inputID, pending, now)
	if err != nil {
		t.Fatal(err)
	}
	binding := privateUserInputBinding(source)
	snapshot := domaincontinuation.UserInputDraftSnapshotV1{CurrentPage: 1, Answers: []domaincontinuation.UserInputDraftAnswerV1{{QuestionID: inputID + "_1", OptionIDs: []string{inputID + "_1_option_2"}, CustomText: ""}, {QuestionID: inputID + "_2", OptionIDs: []string{}, CustomText: "  unfinished\n  "}}}
	first := "00000000-0000-4000-8000-000000000001"
	second := "00000000-0000-4000-8000-000000000002"
	saved, err := service.SaveUserInputDraftHost(context.Background(), binding, first, nil, snapshot)
	if err != nil {
		t.Fatal("first save failed", err)
	}
	retry, err := service.SaveUserInputDraftHost(context.Background(), binding, first, nil, snapshot)
	if err != nil || !reflect.DeepEqual(saved, retry) {
		t.Fatal("exact retry re-signed a new draft", err)
	}
	changed := domaincontinuation.CloneUserInputDraftSnapshotV1(snapshot)
	changed.Answers[1].CustomText += "changed"
	if _, err := service.SaveUserInputDraftHost(context.Background(), binding, first, nil, changed); !errors.Is(err, ErrUserInputDraftConflictV1) {
		t.Fatal("same revision changed content", err)
	}
	if _, err := service.SaveUserInputDraftHost(context.Background(), binding, second, nil, changed); !errors.Is(err, ErrUserInputDraftConflictV1) {
		t.Fatal("stale parent saved", err)
	}
	if _, err := service.SaveUserInputDraftHost(context.Background(), binding, second, &first, changed); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveUserInputDraftHost(context.Background(), binding, first, &second, snapshot); !errors.Is(err, ErrUserInputDraftConflictV1) {
		t.Fatal("old UUID created a cycle", err)
	}
	for _, mutate := range []func(*domaincontinuation.UserInputDraftSnapshotV1){
		func(v *domaincontinuation.UserInputDraftSnapshotV1) { v.CurrentPage = 3 },
		func(v *domaincontinuation.UserInputDraftSnapshotV1) {
			v.Answers[0].OptionIDs = []string{inputID + "_1_option_3"}
		},
		func(v *domaincontinuation.UserInputDraftSnapshotV1) {
			v.Answers[1].QuestionID = "input_" + strings.Repeat("a", 64) + "_2"
		},
	} {
		bad := domaincontinuation.CloneUserInputDraftSnapshotV1(snapshot)
		mutate(&bad)
		if _, err := service.SaveUserInputDraftHost(context.Background(), binding, "00000000-0000-4000-8000-000000000003", &second, bad); !errors.Is(err, ErrUserInputDraftInvalidV1) {
			t.Fatal("unbound partial snapshot accepted", err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopenedAccess, err := privatecastest.NewAccessAuthority(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := continuationadapter.NewStore(storeRoot, reopenedAccess)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedAuthority, err := finalauthorityadapter.OpenOrCreateFileAuthority(signerPath, true)
	if err != nil {
		t.Fatal(err)
	}
	restored := NewService(reopenedAuthority, reopened)
	view, err := restored.ReadUserInputLocalDisplayHost(context.Background(), binding)
	if err != nil || view.DraftRevision == nil || *view.DraftRevision != second || view.Draft == nil || !reflect.DeepEqual(*view.Draft, changed) || view.Submission != nil || view.TimedWait != nil {
		t.Fatal("actual disk reopen lost legacy unfinished draft", err)
	}
	inventory, err := restored.TrustedInventoryV1(context.Background())
	if err != nil || len(inventory.Receipts) != 1 || inventory.Receipts[0].ReceiptID != source.ReceiptID {
		t.Fatal("draft entered gate restoration inventory", err)
	}
	_, submission := privateUserInputRequestFixture(t, pending, inputID)
	if _, err := restored.DisposePendingUserInputHost(inputID, time.Now().UTC(), pending, submission); err != nil {
		t.Fatal(err)
	}
	view, err = restored.ReadUserInputLocalDisplayHost(context.Background(), binding)
	if err != nil || view.Status != "submitted" || view.Draft != nil || view.DraftRevision == nil || *view.DraftRevision != second {
		t.Fatal("submission did not clear draft content and retain revision cursor", err)
	}
	if _, err := restored.SaveUserInputDraftHost(context.Background(), binding, "00000000-0000-4000-8000-000000000004", &second, snapshot); err == nil {
		t.Fatal("submitted source remained editable")
	}
}

func TestPrivateQuestionDraftCaptureCompletionClearsByCausalSet(t *testing.T) {
	now := time.Now().UTC().Add(-time.Minute)
	store := newContinuationTestStore()
	authority := newContinuationTestAuthority(t)
	service := NewService(authority, store)
	pending := privateUserInputPendingFixture(t, now)
	pending.UserInputTimedWait = &domaincontinuation.UserInputTimedWaitV1{TimeoutSeconds: 120}
	inputID := controlapp.SecureGateID(domaincontinuation.KindUserInput, pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest, pending.ExecutionGrant.GrantID, pending.Call.ID)
	source, err := service.IssuePendingHost(domaincontinuation.KindUserInput, inputID, "item_"+inputID, pending, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.DisposePendingHost(inputID, domaincontinuation.StatusContinued, "user_input_continued", now.Add(time.Second), pending); err != nil {
		t.Fatal(err)
	}
	binding := privateUserInputBinding(source)
	_, submission := privateUserInputRequestFixture(t, pending, inputID)
	older, err := domaincontinuation.NewUserInputReplyCaptureReceiptV1(source, "00000000-0000-4000-8000-000000000011", now.Add(2*time.Second), authority.KeyID(), authority.PublicKey(), func(body []byte) ([]byte, error) { return authority.Sign(context.Background(), body) })
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutReceiptIfAbsent(context.Background(), older); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CaptureLateUserInputSubmissionHost(context.Background(), binding, "00000000-0000-4000-8000-000000000012", submission); err != nil {
		t.Fatal(err)
	}
	snapshot := domaincontinuation.UserInputDraftSnapshotV1{CurrentPage: 0, Answers: []domaincontinuation.UserInputDraftAnswerV1{}}
	first := "00000000-0000-4000-8000-000000000021"
	if _, err := service.SaveUserInputDraftHost(context.Background(), binding, first, nil, snapshot); err != nil {
		t.Fatal(err)
	}
	view, err := service.ReadUserInputLocalDisplayHost(context.Background(), binding)
	if err != nil || view.Draft == nil {
		t.Fatal("new edit did not restore draft", err)
	}
	// Complete the earlier-signed capture AFTER the later capture and save.
	disposition, err := domaincontinuation.NewUserInputSubmittedDisposition(older, submission, now.Add(2*time.Second), authority.KeyID(), authority.PublicKey(), func(body []byte) ([]byte, error) { return authority.Sign(context.Background(), body) })
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutDispositionIfAbsent(context.Background(), disposition); err != nil {
		t.Fatal(err)
	}
	view, err = service.ReadUserInputLocalDisplayHost(context.Background(), binding)
	if err != nil || view.Draft != nil || view.DraftRevision == nil || *view.DraftRevision != first {
		t.Fatal("later completion resurrected pre-capture draft", err)
	}
	second := "00000000-0000-4000-8000-000000000022"
	current, err := service.SaveUserInputDraftHost(context.Background(), binding, second, &first, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	beforeReceipts, _ := json.Marshal(store.receipts)
	beforeDispositions, _ := json.Marshal(store.dispositions)
	if _, err := service.SaveUserInputDraftHost(context.Background(), binding, first, nil, snapshot); !errors.Is(err, ErrAuthorityUnavailable) {
		t.Fatal("retired UUID did not remain unavailable under the changed capture set", err)
	}
	afterReceipts, _ := json.Marshal(store.receipts)
	afterDispositions, _ := json.Marshal(store.dispositions)
	if !bytes.Equal(beforeReceipts, afterReceipts) || !bytes.Equal(beforeDispositions, afterDispositions) {
		t.Fatal("retired retry changed signed source/draft/capture state")
	}
	view, err = service.ReadUserInputLocalDisplayHost(context.Background(), binding)
	if err != nil || view.Draft == nil || view.DraftRevision == nil || *view.DraftRevision != second || !reflect.DeepEqual(*view.Draft, snapshot) {
		t.Fatal("retired retry replaced the current draft", err)
	}
	if retry, err := service.SaveUserInputDraftHost(context.Background(), binding, second, &first, snapshot); err != nil || !reflect.DeepEqual(current, retry) {
		t.Fatal("same valid capture set changed the original signed result", err)
	}
	afterReceipts, _ = json.Marshal(store.receipts)
	afterDispositions, _ = json.Marshal(store.dispositions)
	if !bytes.Equal(beforeReceipts, afterReceipts) || !bytes.Equal(beforeDispositions, afterDispositions) {
		t.Fatal("valid exact retry produced a second effect")
	}
}
