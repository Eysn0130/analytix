package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	continuationapp "analytix.local/runtime-go/internal/app/continuation"
	controlapp "analytix.local/runtime-go/internal/app/control"
	gatecontinuationapp "analytix.local/runtime-go/internal/app/gatecontinuation"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	displayapp "analytix.local/runtime-go/internal/app/userinputdisplay"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
	port "analytix.local/runtime-go/internal/ports/userinputdisplay"
)

// Uses the real signed request issuer, durable Fork/seed, strict disk reader
// and private HTTP display. The numeric Host identity is a synthetic fixture;
// this proves no timer, continued settlement or live Provider outcome.
func TestForkQuestionHostHistoryNumericIdentityReopenReforkV1(t *testing.T) {
	h, pending, _ := privateQuestionDraftPendingFixtureV1(t)
	pending.Call.ID = serverTestHostToolCallID("synthetic_fork_numeric_question")
	// Derive, rather than assign, the numeric identity from a valid grant.
	// Only the selected candidate reaches the signer or persistence owners.
	now, originalGrant := time.Now().UTC(), pending.ExecutionGrant
	numericRun := regexp.MustCompile(`[0-9]{13,}`)
	id := ""
	for candidate := 0; candidate < 1024; candidate++ {
		issued := now.Add(-time.Duration(candidate) * time.Nanosecond)
		grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{Context: pending.SecurityContext,
			Provider: originalGrant.Provider, ServerIdentity: originalGrant.ServerIdentity, ToolName: originalGrant.ToolName, ToolCallID: pending.Call.ID,
			ConnectionEpoch: originalGrant.ConnectionEpoch, ArgsHash: originalGrant.ArgsHash, SchemaHash: originalGrant.SchemaHash, ScopeHash: originalGrant.ScopeHash,
			ReadOnly: originalGrant.ReadOnly, ApprovalState: originalGrant.ApprovalState, IssuedAt: issued, ExpiresAt: issued.Add(15 * time.Minute)})
		derived := controlapp.SecureGateID(domaincontinuation.KindUserInput, pending.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest, grant.GrantID, pending.Call.ID)
		if numericRun.MatchString(derived) && domainsecurity.ContainsProtectedCaseData(derived+"_1") {
			id, pending.ExecutionGrant = derived, grant
			break
		}
	}
	if id == "" {
		t.Fatal("bounded grant selection did not derive the numeric Host identity")
	}
	var err error
	pending.ToolCallItemID, err = h.persistToolCallReady(context.Background(), pending.ThreadID, pending.TurnID, pending.Call, 1, pending.SecurityContext, pending.ExecutionGrant)
	if err != nil {
		t.Fatal("numeric source grant persistence", err)
	}
	if err := h.authorizeRuntimePending(context.Background(), pending, "", now.Add(time.Second)); err != nil {
		t.Fatal("numeric source lacks original execution authority", err)
	}
	receipt, err := h.continuations.IssuePendingHost(domaincontinuation.KindUserInput, id, "item_"+id, pending, now)
	if err != nil {
		t.Fatal("synthetic Host request issuance", err)
	}
	projection, err := gatecontinuationapp.ProjectVerifiedGateRequestV1(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.store.EnsureGateRequestItemExact(pending.ThreadID, pending.TurnID, projection.Item); err != nil {
		t.Fatal(err)
	}
	if err := h.recordPendingGateRequest(projection.Event); err != nil {
		t.Fatal(err)
	}
	origin := port.SelectorV1{ThreadID: pending.ThreadID, TurnID: pending.TurnID, ItemID: "item_" + id, RequestID: id}
	original := readTimedQuestionPrivateDisplay(t, h, origin)
	before, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), origin.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	fork, err := h.store.ForkThread(origin.ThreadID, map[string]any{"title": "Synthetic readonly history"})
	if err != nil {
		t.Fatal("actual durable fork rejected legitimate Host identity", err)
	}
	child := origin
	child.ThreadID = stringField(fork, "id")
	view := readTimedQuestionPrivateDisplay(t, h, child)
	if !view.ReadOnly || view.Status != "cancelled" || !reflect.DeepEqual(view.Questions, original.Questions) || !view.Questions[0].MultiSelect || view.Submission != nil || view.Draft != nil || view.TimedWait != nil {
		t.Fatal("inherited Question did not reopen as immutable readonly history")
	}
	if _, err := exactUserInputDisplayItemV1(fork, child); err == nil {
		t.Fatal("child retained an active receipt")
	}
	refork, err := h.store.ForkThread(child.ThreadID, map[string]any{"title": "Synthetic refork history"})
	if err != nil {
		t.Fatal("actual refork lost historical identity", err)
	}
	grandchild := child
	grandchild.ThreadID = stringField(refork, "id")
	reopened, err := newForkBoundaryStoreV1(t, h.store.root)
	if err != nil {
		t.Fatal(err)
	}
	h.store = reopened
	if restored := readTimedQuestionPrivateDisplay(t, h, grandchild); !restored.ReadOnly || !reflect.DeepEqual(restored.Questions, original.Questions) {
		t.Fatal("disk reopen/refork lost proven original questions")
	}
	after, err := h.store.ReadPrimaryThreadSnapshotV1(context.Background(), origin.ThreadID)
	if err != nil || before.ThreadFileSHA256 != after.ThreadFileSHA256 {
		t.Fatal("fork/read/reopen mutated original primary")
	}
	// An exact child selector cannot capture a draft, register a worker,
	// consume original pending or finish an original timer.
	if !h.gates.RegisterPendingUserInput(id, projection.Record, pending) {
		t.Fatal("register source gate")
	}
	h.gate.RequestUserInput(id, "Synthetic source Question")
	result, err := h.respondRuntimeUserInput(context.Background(), controlapp.UserInputResponse{InputID: id, Selector: &grandchild, Cancelled: true})
	if err != nil || result.StatusCode != http.StatusConflict {
		t.Fatal("child selector acted on source")
	}
	if _, _, _, hasPending := h.gates.PeekUserInput(id); !hasPending {
		t.Fatal("child consumed source pending")
	}
	draft := displayapp.SaveDraftRequestV1{SelectorV1: grandchild, Action: "saveDraft", Revision: "00000000-0000-4000-8000-000000000099", Draft: domaincontinuation.UserInputDraftSnapshotV1{CurrentPage: 0, Answers: []domaincontinuation.UserInputDraftAnswerV1{}}}
	privateQuestionDraftHTTPV1(t, h, draft, http.StatusNotFound)
	if sourceView := readTimedQuestionPrivateDisplay(t, h, origin); sourceView.ReadOnly || sourceView.Draft != nil || !reflect.DeepEqual(sourceView.Questions, original.Questions) {
		t.Fatal("history guard damaged original writable display")
	}
	request := controlapp.UserInputResponse{InputID: id, Selector: &origin, Cancelled: true}
	if result, err := h.respondRuntimeUserInput(context.Background(), request); err != nil || result.StatusCode != http.StatusOK {
		t.Fatal("exact original selector lost ordinary cancellation")
	}
	if result, err := h.respondRuntimeUserInput(context.Background(), request); err != nil || result.StatusCode != http.StatusConflict {
		t.Fatal("duplicate original action gained a second effect")
	}
	side, err := h.store.ForkThread(origin.ThreadID, map[string]any{"relation": "side", "title": "Synthetic closed side history"})
	if err != nil {
		t.Fatal("actual Side lost closed original Question", err)
	}
	sideSelector := origin
	sideSelector.ThreadID = stringField(side, "id")
	if sideView := readTimedQuestionPrivateDisplay(t, h, sideSelector); !sideView.ReadOnly || !reflect.DeepEqual(sideView.Questions, original.Questions) {
		t.Fatal("actual Side did not retain proven readonly Question history")
	}
}

type questionHistoryCeilingProbeV1 struct {
	threadapp.PublicProjector
	threadID string
}

func (p questionHistoryCeilingProbeV1) ProjectThread(thread map[string]any) (map[string]any, error) {
	if stringField(thread, "id") == p.threadID {
		return nil, errors.New("synthetic current ceiling rejected inherited history")
	}
	return p.PublicProjector.ProjectThread(thread)
}

func TestForkQuestionHostHistoryRejectsUnprovenPrefixAndCurrentAuthorityV1(t *testing.T) {
	for _, name := range []string{"missing_projector", "production_ceiling_rejects", "changed_current_item", "missing_creation_close", "source_scope_revoked"} {
		t.Run(name, func(t *testing.T) {
			h, _, receipt := privateQuestionDraftPendingFixtureV1(t)
			origin := port.SelectorV1{ThreadID: receipt.Payload.ThreadID, TurnID: receipt.Payload.TurnID, ItemID: receipt.Payload.ItemID, RequestID: receipt.Payload.GateID}
			fork, err := h.store.ForkThread(origin.ThreadID, map[string]any{"title": "Synthetic negative history"})
			if err != nil {
				t.Fatal(err)
			}
			child := origin
			child.ThreadID = stringField(fork, "id")
			if !readTimedQuestionPrivateDisplay(t, h, child).ReadOnly {
				t.Fatal("baseline did not use real readonly history")
			}
			switch name {
			case "missing_projector":
				h.publicProjector = nil
			case "production_ceiling_rejects":
				h.publicProjector = questionHistoryCeilingProbeV1{PublicProjector: h.publicProjector, threadID: child.ThreadID}
			case "changed_current_item":
				if err := h.store.PatchTurnItemStatus(child.ThreadID, child.TurnID, child.ItemID, "submitted"); err != nil {
					t.Fatal(err)
				}
			case "missing_creation_close":
				loaded, _, err := h.store.eventLog.ObserveSinceWithFrontier(context.Background(), child.ThreadID, 0)
				if err != nil {
					t.Fatal(err)
				}
				_, closed, err := threadapp.ReadForkCreatedSeedV1(fork, loaded.Events)
				if err != nil || closed <= 1 {
					t.Fatal("baseline creation did not close")
				}
				var body bytes.Buffer
				for _, event := range loaded.Events[:closed-1] {
					encoded, err := json.Marshal(event)
					if err != nil {
						t.Fatal(err)
					}
					body.Write(encoded)
					body.WriteByte('\n')
				}
				if err := os.WriteFile(h.store.eventLog.EventsPath(child.ThreadID), body.Bytes(), 0o600); err != nil {
					t.Fatal(err)
				}
			case "source_scope_revoked":
				h.toolSnapshotHistoryValidate = func(context.Context, map[string]any, domainsecurity.TurnSecurityContext) error {
					return errors.New("synthetic original scope revoked")
				}
			}
			body, _ := json.Marshal(child)
			request := httptest.NewRequest(http.MethodPost, httpapi.UserInputLocalDisplayPathV1, bytes.NewReader(body))
			request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
			request.Header.Set(httpapi.LocalDisplayHeaderV1, httpapi.LocalDisplayHeaderValueV1)
			response := httptest.NewRecorder()
			httpapi.LocalDisplayMuxV1{RuntimeToken: DefaultRuntimeToken, UserInputs: h.UserInputLocalDisplayV1()}.ServeHTTP(response, request)
			var failure map[string]any
			if response.Code != http.StatusNotFound || json.Unmarshal(response.Body.Bytes(), &failure) != nil ||
				!reflect.DeepEqual(failure, map[string]any{"code": "not_found", "message": "The requested resource was not found."}) {
				t.Fatal("unproven history escaped the private unavailable response")
			}
		})
	}
}

func displayPrimaryFixtureV1() (map[string]any, port.SelectorV1) {
	selector := port.SelectorV1{ThreadID: "thread-a", TurnID: "turn-a", ItemID: "item_input_" + strings.Repeat("a", 64), RequestID: "input_" + strings.Repeat("a", 64)}
	item := map[string]any{"id": selector.ItemID, "kind": "user_input", "threadId": selector.ThreadID, "turnId": selector.TurnID, "inputId": selector.RequestID, "continuationReceiptId": strings.Repeat("b", 64), "status": "pending"}
	return map[string]any{"id": selector.ThreadID, "turns": []any{map[string]any{"id": selector.TurnID, "items": []any{item}}}}, selector
}

func TestUserInputDisplayPrimaryRejectsRelatedIdentityConflicts(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(map[string]any, port.SelectorV1)
	}{
		{"wrong_kind", func(thread map[string]any, _ port.SelectorV1) {
			thread["turns"].([]any)[0].(map[string]any)["items"].([]any)[0].(map[string]any)["kind"] = "message"
		}},
		{"wrong_request", func(thread map[string]any, _ port.SelectorV1) {
			thread["turns"].([]any)[0].(map[string]any)["items"].([]any)[0].(map[string]any)["inputId"] = "input_" + strings.Repeat("c", 64)
		}},
		{"wrong_thread", func(thread map[string]any, _ port.SelectorV1) {
			thread["turns"].([]any)[0].(map[string]any)["items"].([]any)[0].(map[string]any)["threadId"] = "other"
		}},
		{"missing_receipt", func(thread map[string]any, _ port.SelectorV1) {
			delete(thread["turns"].([]any)[0].(map[string]any)["items"].([]any)[0].(map[string]any), "continuationReceiptId")
		}},
		{"duplicate_item", func(thread map[string]any, _ port.SelectorV1) {
			turn := thread["turns"].([]any)[0].(map[string]any)
			turn["items"] = append(turn["items"].([]any), turn["items"].([]any)[0])
		}},
		{"foreign_turn_request_alias", func(thread map[string]any, selector port.SelectorV1) {
			thread["turns"] = append(thread["turns"].([]any), map[string]any{"id": "turn-b", "items": []any{map[string]any{"id": "item-b", "inputId": selector.RequestID}}})
		}},
		{"foreign_receipt_alias", func(thread map[string]any, _ port.SelectorV1) {
			thread["turns"] = append(thread["turns"].([]any), map[string]any{"id": "turn-b", "items": []any{map[string]any{"id": "item-b", "continuationReceiptId": strings.Repeat("b", 64)}}})
		}},
		{"duplicate_turn", func(thread map[string]any, selector port.SelectorV1) {
			thread["turns"] = append(thread["turns"].([]any), map[string]any{"id": selector.TurnID, "items": []any{}})
		}},
		{"deleted", func(thread map[string]any, _ port.SelectorV1) { thread["status"] = "deleted" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			thread, selector := displayPrimaryFixtureV1()
			test.change(thread, selector)
			if _, err := exactUserInputDisplayItemV1(thread, selector); !errors.Is(err, port.ErrUnavailableV1) {
				t.Fatal("conflicting primary alias selected one matching item")
			}
		})
	}
}

func TestUserInputDisplayPrimaryPreservesExactHistoryStatuses(t *testing.T) {
	for _, status := range []string{"pending", "submitted", "cancelled"} {
		thread, selector := displayPrimaryFixtureV1()
		item := thread["turns"].([]any)[0].(map[string]any)["items"].([]any)[0].(map[string]any)
		item["status"] = status
		matched, err := exactUserInputDisplayItemV1(thread, selector)
		if err != nil || matched["status"] != status || matched["inputId"] != selector.RequestID {
			t.Fatal("historical status lost its exact primary identity")
		}
	}
}

func TestUserInputDisplayCurrentOwnAuthorityAndStatusRace(t *testing.T) {
	h, pending, _, _ := protectedSeamFixtureV1(t, "read", map[string]any{"path": "synthetic.txt"})
	selector := port.SelectorV1{ThreadID: pending.ThreadID, TurnID: pending.TurnID, ItemID: "item_input_" + strings.Repeat("a", 64), RequestID: "input_" + strings.Repeat("a", 64)}
	item := map[string]any{"id": selector.ItemID, "kind": "user_input", "threadId": selector.ThreadID, "turnId": selector.TurnID, "inputId": selector.RequestID, "continuationReceiptId": strings.Repeat("b", 64), "status": "pending"}
	if err := h.store.AppendItemToTurn(selector.ThreadID, selector.TurnID, item); err != nil {
		t.Fatal(err)
	}
	access := runtimeUserInputDisplayAccessV1{handler: h}
	authorized, release, err := access.AcquireCurrent(context.Background(), selector)
	if err != nil || release == nil {
		t.Fatal("existing signed current-own authority did not admit exact primary item")
	}
	defer release()
	if !reflect.DeepEqual(authorized.Original, pending.SecurityContext) || access.ValidateCurrent(context.Background(), authorized) != nil {
		t.Fatal("display authority changed the real frozen context")
	}
	if err := h.store.PatchTurnItemStatus(selector.ThreadID, selector.TurnID, selector.ItemID, "cancelled"); err != nil {
		t.Fatal(err)
	}
	if access.ValidateCurrent(context.Background(), authorized) == nil {
		t.Fatal("status race returned stale private display authority")
	}
	h.toolSnapshotHistoryValidate = nil
	if _, deniedRelease, err := access.AcquireCurrent(context.Background(), selector); err == nil || deniedRelease != nil {
		t.Fatal("missing historical authority inherited an ordinary fallback")
	}
}

func TestUserInputDisplayHistoricalScopeRevocationDeniesRead(t *testing.T) {
	h, pending, _, _ := protectedSeamFixtureV1(t, "read", map[string]any{"path": "synthetic.txt"})
	selector := port.SelectorV1{ThreadID: pending.ThreadID, TurnID: pending.TurnID, ItemID: "item_input_" + strings.Repeat("a", 64), RequestID: "input_" + strings.Repeat("a", 64)}
	if err := h.store.AppendItemToTurn(selector.ThreadID, selector.TurnID, map[string]any{"id": selector.ItemID, "kind": "user_input", "threadId": selector.ThreadID, "turnId": selector.TurnID, "inputId": selector.RequestID, "continuationReceiptId": strings.Repeat("b", 64), "status": "submitted"}); err != nil {
		t.Fatal(err)
	}
	h.toolSnapshotHistoryValidate = func(context.Context, map[string]any, domainsecurity.TurnSecurityContext) error {
		return errors.New("signed historical scope revoked")
	}
	if _, release, err := (runtimeUserInputDisplayAccessV1{handler: h}).AcquireCurrent(context.Background(), selector); err == nil || release != nil {
		t.Fatal("revoked historical scope admitted a display read")
	}
}

func TestUserInputDisplayNilDependenciesAndCancelledContextDeny(t *testing.T) {
	_, selector := displayPrimaryFixtureV1()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, test := range []struct {
		ctx     context.Context
		handler *runtimeServerHandler
	}{
		{context.Background(), nil},
		{nil, &runtimeServerHandler{}},
		{ctx, &runtimeServerHandler{}},
		{context.Background(), &runtimeServerHandler{}},
	} {
		if _, release, err := (runtimeUserInputDisplayAccessV1{handler: test.handler}).AcquireCurrent(test.ctx, selector); err == nil || release != nil {
			t.Fatal("nil/cancelled authority admitted a display read")
		}
	}
	if (*runtimeServerHandler)(nil).UserInputLocalDisplayV1() == nil {
		t.Fatal("nil composition getter did not return its unavailable leaf")
	}
}

// Uses signed Host disposition and actual capture/control/queue/private HTTP.
// It deliberately does not claim timed wait settlement or Provider delivery.
func TestPrivateQuestionDiscardActualQueueAndPromotionConflict(t *testing.T) {
	h, pending, durable, _ := protectedSeamFixtureV1(t, "read", map[string]any{"path": "synthetic.txt"})
	h.userInputConfig = toolcatalogapp.UserInputConfig{Mode: "timed"}
	configureHistory := configureTimedQuestionHistoryAuthority(t, h)
	source := timedQuestionSignedAdmissionSource(t, h, pending.SecurityContext, serverTestHostToolCallID("synthetic_discard_question"), false, 9, "")
	persistTimedQuestionSignedSourceProjection(t, h, source)
	selector := port.SelectorV1{ThreadID: source.Payload.ThreadID, TurnID: source.Payload.TurnID, ItemID: source.Payload.ItemID, RequestID: source.Payload.GateID}
	binding := continuationapp.UserInputLocalDisplayBindingV1{ThreadID: selector.ThreadID, TurnID: selector.TurnID, ItemID: selector.ItemID, InputID: selector.RequestID, ReceiptID: source.ReceiptID, SecurityContext: source.Payload.SecurityContext}
	queue := func(attempt string) {
		t.Helper()
		var args map[string]any
		if json.Unmarshal(source.Payload.Arguments, &args) != nil {
			t.Fatal("source arguments unavailable")
		}
		questions := toolcatalogapp.UserInputQuestionsForLocalDisplay(args, selector.RequestID, "User input required")
		custom := "ordinary synthetic answer"
		_, submission, err := controlapp.NormalizeAndCaptureUserInputAnswers(controlapp.UserInputResponse{StructuredAnswers: []controlapp.UserInputAnswer{{ID: selector.RequestID + "_1", Kind: "answer", OptionIDs: []string{}, CustomText: &custom}}}, questions, "")
		if err != nil || submission == nil {
			t.Fatal("signed answer normalization failed", err)
		}
		capture, err := h.continuations.CaptureLateUserInputSubmissionHost(context.Background(), binding, attempt, *submission)
		if err != nil {
			t.Fatal("capture failed", err)
		}
		carrier, err := h.verifiedLateUserInputSteeringV1(context.Background(), binding, attempt, capture.ReceiptID, pending.TurnID)
		if err != nil {
			t.Fatal("capture carrier unavailable", err)
		}
		result, err := h.runtimeControl().SteerTurn(context.WithValue(context.Background(), lateUserInputSteeringContextKeyV1{}, carrier), carrier.requestV1())
		if err != nil || result.StatusCode != http.StatusOK {
			t.Fatalf("actual queue admission failed: status=%d errorType=%T", result.StatusCode, err)
		}
		delivery, err := h.store.readUserInputLateReplyV1(selector.ThreadID, selector.RequestID, attempt)
		if err != nil || delivery == nil || delivery.State != "queued" {
			t.Fatal("admission was not queued", err)
		}
	}
	discard := func(handler *runtimeServerHandler, attempt string, expectedStatus int) displayapp.DiscardLateReplyResponseV1 {
		t.Helper()
		request := displayapp.DiscardLateReplyRequestV1{SelectorV1: selector, Action: "discardLateReply", ClientUserMessageID: attempt, ExpectedTurnID: pending.TurnID}
		body, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, httpapi.UserInputLocalDisplayPathV1, bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
		r.Header.Set(httpapi.LocalDisplayHeaderV1, httpapi.LocalDisplayHeaderValueV1)
		w := httptest.NewRecorder()
		httpapi.LocalDisplayMuxV1{RuntimeToken: DefaultRuntimeToken, UserInputs: handler.UserInputLocalDisplayV1()}.ServeHTTP(w, r)
		if w.Code != expectedStatus {
			t.Fatalf("actual private discard status=%d want=%d", w.Code, expectedStatus)
		}
		var response displayapp.DiscardLateReplyResponseV1
		if expectedStatus == http.StatusOK && (json.Unmarshal(w.Body.Bytes(), &response) != nil || !displayapp.ValidateDiscardLateReplyResponseV1(response, request)) {
			t.Fatal("invalid actual discard ACK")
		}
		return response
	}
	first := "00000000-0000-4000-8000-000000000091"
	queue(first)
	ack := discard(h, first, http.StatusOK)
	proof, err := h.store.readUserInputLateReplyV1(selector.ThreadID, selector.RequestID, first)
	if err != nil || proof == nil || proof.State != "discarded" {
		t.Fatal("ACK had no durable discard", err)
	}
	if _, err := domainsteering.EntryAuthorityMaterialV1(proof.Entry, pending.SecurityContext.ContextDigest); err != nil {
		t.Fatal("discard has no signed authority", err)
	}
	if repeated := discard(h, first, http.StatusOK); !reflect.DeepEqual(ack, repeated) {
		t.Fatal("same UUID changed ACK")
	}
	if entries, items, err := h.store.PromotePendingSteeringEntriesForContext(selector.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest); err != nil || len(entries) != 0 || len(items) != 0 {
		t.Fatal("discarded reply promoted", err)
	}
	second := "00000000-0000-4000-8000-000000000092"
	queue(second)
	queuedFork, err := h.store.ForkThread(selector.ThreadID, map[string]any{"title": "Synthetic queued immutable history"})
	if err != nil {
		t.Fatal(err)
	}
	queuedChild := selector
	queuedChild.ThreadID = stringField(queuedFork, "id")
	assertNoAnswer := func(handler *runtimeServerHandler, selected port.SelectorV1) {
		t.Helper()
		view := readTimedQuestionPrivateDisplay(t, handler, selected)
		if !view.ReadOnly || view.Status != "continued" || view.AnswerState != "unavailable" || view.Submission != nil || view.HistoryReply != nil || view.LateReply != nil {
			t.Fatal("queued prefix gained later accepted answer")
		}
	}
	assertNoAnswer(h, queuedChild)
	entries, items, err := h.store.PromotePendingSteeringEntriesForContext(selector.ThreadID, pending.TurnID, pending.SecurityContext.ContextDigest)
	if err != nil || len(entries) != 1 || len(items) != 1 {
		t.Fatal("new UUID was not independently promotable", err)
	}
	discard(h, second, http.StatusConflict)
	assertNoAnswer(h, queuedChild)
	accepted, err := h.store.readUserInputLateReplyV1(selector.ThreadID, selector.RequestID, second)
	if err != nil || accepted == nil || accepted.State != "accepted" {
		t.Fatal("conflict withdrew accepted reply", err)
	}
	fork, err := h.store.ForkThread(selector.ThreadID, map[string]any{"title": "Synthetic accepted immutable history"})
	if err != nil {
		t.Fatal(err)
	}
	child := selector
	child.ThreadID = stringField(fork, "id")

	historical := readTimedQuestionPrivateDisplay(t, h, child)
	if !historical.ReadOnly || historical.AnswerState != "available" || historical.Submission == nil || historical.HistoryReply == nil ||
		historical.HistoryReply.ClientUserMessageID != second || historical.HistoryReply.ItemID != stringField(items[0], "id") || historical.HistoryReply.TurnID != pending.TurnID || historical.LateReply != nil || historical.Draft != nil || historical.TimedWait != nil {
		t.Fatal("signed accepted history was not independently restored")
	}
	// The same UUID remains forbidden as a child write, including without a view.
	beforeWrites := map[string][]byte{}
	for _, threadID := range []string{selector.ThreadID, child.ThreadID} {
		beforeWrites[threadID], err = os.ReadFile(h.store.threadPath(threadID))
		if err != nil {
			t.Fatal(err)
		}
	}
	privateQuestionDraftHTTPV1(t, h, displayapp.SaveDraftRequestV1{SelectorV1: child, Action: "saveDraft", Revision: "00000000-0000-4000-8000-000000000093", Draft: domaincontinuation.UserInputDraftSnapshotV1{Answers: []domaincontinuation.UserInputDraftAnswerV1{}}}, http.StatusNotFound)
	// This signed fixture has no registered active Question gate. Its normal
	// submission follows the existing exact missing-gate response.
	rejected, err := h.respondRuntimeUserInput(context.Background(), controlapp.UserInputResponse{InputID: child.RequestID, Selector: &child, Cancelled: true})
	if err != nil || rejected.StatusCode != http.StatusNotFound || stringField(rejected.Body, "code") != "not_found" {
		t.Fatalf("historical normal submission was not rejected: status=%d code=%s errorType=%T", rejected.StatusCode, stringField(rejected.Body, "code"), err)
	}
	lateRejected, err := h.respondRuntimeUserInput(context.Background(), controlapp.UserInputResponse{InputID: child.RequestID, Selector: &child, LateReply: &controlapp.UserInputLateReplyV1{ClientUserMessageID: second, ExpectedTurnID: pending.TurnID}})
	if err != nil || lateRejected.StatusCode != http.StatusConflict || stringField(lateRejected.Body, "code") != "late_reply_unavailable" {
		t.Fatalf("historical late submission gained authority: status=%d code=%s errorType=%T", lateRejected.StatusCode, stringField(lateRejected.Body, "code"), err)
	}
	badDiscard := displayapp.DiscardLateReplyRequestV1{SelectorV1: child, Action: "discardLateReply", ClientUserMessageID: second, ExpectedTurnID: pending.TurnID}
	if _, err := (runtimeUserInputDisplayServiceV1{h}).DiscardLateReply(context.Background(), badDiscard); !errors.Is(err, port.ErrUnavailableV1) {
		t.Fatal("historical child gained discard authority")
	}
	for threadID, before := range beforeWrites {
		after, err := os.ReadFile(h.store.threadPath(threadID))
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("rejected historical writes changed primary state", err)
		}
	}
	// Keep the actual private HTTP body for the strict Main Copy seam; this
	// environment variable is test-only and is absent from product composition.
	if witness := os.Getenv("ANALYTIX_HISTORY_MAIN_WITNESS_FILE"); witness != "" {
		body, err := json.Marshal(child)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, httpapi.UserInputLocalDisplayPathV1, bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
		r.Header.Set(httpapi.LocalDisplayHeaderV1, httpapi.LocalDisplayHeaderValueV1)
		w := httptest.NewRecorder()
		httpapi.LocalDisplayMuxV1{RuntimeToken: DefaultRuntimeToken, UserInputs: h.UserInputLocalDisplayV1()}.ServeHTTP(w, r)
		var actual displayapp.DisplayV1
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &actual) != nil || !reflect.DeepEqual(actual, historical) {
			t.Fatal("actual private HTTP history body changed")
		}
		if os.WriteFile(witness, w.Body.Bytes(), 0o600) != nil {
			t.Fatal("history Main witness unavailable")
		}
	}
	// Changes to the accepted source proof or this child pair must fail closed.
	for _, mutation := range []string{"source_signature", "fixed_capture", "child_pair", "child_private_field"} {
		target := selector.ThreadID
		if mutation == "child_pair" || mutation == "child_private_field" {
			target = child.ThreadID
		}
		path := h.store.threadPath(target)
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var changed map[string]any
		if json.Unmarshal(before, &changed) != nil {
			t.Fatal("negative primary decode")
		}
		for _, rawTurn := range listAny(changed["turns"]) {
			turn := rawTurn.(map[string]any)
			if mutation == "child_pair" || mutation == "child_private_field" {
				for _, rawItem := range listAny(turn["items"]) {
					item := rawItem.(map[string]any)
					if item["id"] == historical.HistoryReply.ItemID {
						if mutation == "child_private_field" {
							item["steeringContentDigest"] = strings.Repeat("e", 64)
						} else {
							item["text"] = "Changed synthetic bytes"
						}
					}
				}
			} else {
				for _, rawEntry := range listAny(turn["steering"]) {
					entry, ok := rawEntry.(map[string]any)
					if ok && entry["clientUserMessageId"] == second {
						if mutation == "source_signature" {
							entry["promotionAuthoritySignature"] = "invalid"
						} else {
							entry["userInputCaptureReceiptId"] = strings.Repeat("f", 64)
						}
					}
				}
			}
		}
		encoded, err := json.Marshal(changed)
		if err != nil || os.WriteFile(path, encoded, 0o600) != nil {
			t.Fatal("negative primary write")
		}
		_, readErr := (runtimeUserInputDisplayServiceV1{h}).Read(context.Background(), child)
		if restoreErr := os.WriteFile(path, before, 0o600); restoreErr != nil {
			t.Fatal(restoreErr)
		}
		if !errors.Is(readErr, port.ErrUnavailableV1) {
			t.Fatalf("changed historical proof accepted: %s", mutation)
		}
	}
	thread, err := h.store.GetThread(selector.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	original, err := exactUserInputDisplayItemV1(thread, selector)
	if err != nil || original["status"] != "continued" {
		t.Fatal("discard changed original Question", err)
	}
	// The protected seam fixture created a durable running turn without a
	// worker. Close it through the actual interrupt/terminal owner before
	// opening a new handler; Shutdown cannot join an unregistered worker.
	interrupted, err := h.runtimeControl().InterruptTurn(context.Background(), controlapp.InterruptTurnRequest{ThreadID: selector.ThreadID, TurnID: pending.TurnID})
	if err != nil || interrupted.StatusCode != http.StatusOK {
		t.Fatalf("fixture terminal close failed: status=%d errorType=%T", interrupted.StatusCode, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	err = h.Shutdown(ctx)
	cancel()
	if err != nil {
		t.Fatal("fixture shutdown failed", err)
	}
	restarted := NewRuntimeServerHandler(RuntimeServerConfig{RuntimeToken: DefaultRuntimeToken, DurableTempDir: durable, DataDir: h.dataDir, ProviderID: "synthetic", BaseURL: "http://127.0.0.1:1/v1", APIKey: "synthetic-only", EndpointFormat: "chat_completions", Model: "synthetic"}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, restarted)
	// This seam fixture's current risk authority is in memory. Reuse that
	// exact authority; continuation/store/signing records reopen from disk.
	// This proves handler/proof recovery, not risk-authority process recovery.
	restarted.turnSecurity.RiskAuthority = h.turnSecurity.RiskAuthority
	configureHistory(restarted)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := restarted.Shutdown(ctx); err != nil {
			t.Errorf("restarted fixture shutdown errorType=%T", err)
		}
	})
	discard(restarted, first, http.StatusOK)
	restored, err := restarted.store.readUserInputLateReplyV1(selector.ThreadID, selector.RequestID, first)
	if err != nil || !reflect.DeepEqual(proof, restored) {
		t.Fatal("reopen changed original discard proof", err)
	}
	discard(restarted, second, http.StatusConflict)
	assertNoAnswer(restarted, queuedChild)
	queuedRefork, err := restarted.store.ForkThread(queuedChild.ThreadID, map[string]any{"title": "Synthetic queued refork"})
	if err != nil {
		t.Fatal(err)
	}
	queuedGrandchild := queuedChild
	queuedGrandchild.ThreadID = stringField(queuedRefork, "id")
	assertNoAnswer(restarted, queuedGrandchild)
	restoredHistory := readTimedQuestionPrivateDisplay(t, restarted, child)
	if !reflect.DeepEqual(restoredHistory.HistoryReply, historical.HistoryReply) || !reflect.DeepEqual(restoredHistory.Submission, historical.Submission) {
		t.Fatal("cold accepted history lost its fixed proof")
	}
	refork, err := restarted.store.ForkThread(child.ThreadID, map[string]any{"title": "Synthetic accepted refork"})
	if err != nil {
		t.Fatal(err)
	}
	grandchild := child
	grandchild.ThreadID = stringField(refork, "id")
	reforked := readTimedQuestionPrivateDisplay(t, restarted, grandchild)
	if !reforked.ReadOnly || !reflect.DeepEqual(reforked.HistoryReply, historical.HistoryReply) || !reflect.DeepEqual(reforked.Submission, historical.Submission) {
		t.Fatal("refork lost original accepted history")
	}
}

func privateQuestionDraftHTTPV1(t *testing.T, h *runtimeServerHandler, request displayapp.SaveDraftRequestV1, want int) displayapp.SaveDraftResponseV1 {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, httpapi.UserInputLocalDisplayPathV1, bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	r.Header.Set(httpapi.LocalDisplayHeaderV1, httpapi.LocalDisplayHeaderValueV1)
	w := httptest.NewRecorder()
	httpapi.LocalDisplayMuxV1{RuntimeToken: DefaultRuntimeToken, UserInputs: h.UserInputLocalDisplayV1()}.ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("actual private draft status=%d want=%d", w.Code, want)
	}
	var ack displayapp.SaveDraftResponseV1
	if want == http.StatusOK && (json.Unmarshal(w.Body.Bytes(), &ack) != nil || !displayapp.ValidateSaveDraftResponseV1(ack, request)) {
		t.Fatal("invalid real draft ACK")
	}
	if want != http.StatusOK && len(request.Draft.Answers) > 0 && request.Draft.Answers[0].CustomText != "" && strings.Contains(w.Body.String(), request.Draft.Answers[0].CustomText) {
		t.Fatal("private error returned raw draft")
	}
	if want == http.StatusNotFound {
		var failure map[string]any
		if json.Unmarshal(w.Body.Bytes(), &failure) != nil || !reflect.DeepEqual(failure, map[string]any{"code": "not_found", "message": "The requested resource was not found."}) {
			t.Fatal("unavailable draft returned a variable or open error body")
		}
	}
	return ack
}

func TestPrivateQuestionDraftActualLegacyHTTPKeepsPublicStateAndCAS(t *testing.T) {
	h, pending, source := privateQuestionDraftPendingFixtureV1(t)
	selector := port.SelectorV1{ThreadID: source.Payload.ThreadID, TurnID: source.Payload.TurnID, ItemID: source.Payload.ItemID, RequestID: source.Payload.GateID}
	before, err := h.store.LoadEventsSince(selector.ThreadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	request := displayapp.SaveDraftRequestV1{SelectorV1: selector, Action: "saveDraft", Revision: "00000000-0000-4000-8000-000000000051", Draft: domaincontinuation.UserInputDraftSnapshotV1{CurrentPage: 1, Answers: []domaincontinuation.UserInputDraftAnswerV1{{QuestionID: selector.RequestID + "_2", OptionIDs: []string{}, CustomText: "  SYNTHETIC_PRIVATE_DRAFT_SENTINEL\n  "}}}}
	initial := readTimedQuestionPrivateDisplay(t, h, selector)
	if initial.Status != "pending" || initial.TimedWait != nil || initial.Draft != nil || initial.Submission != nil {
		t.Fatal("legacy source was not actual pending Question")
	}
	ack := privateQuestionDraftHTTPV1(t, h, request, http.StatusOK)
	if repeated := privateQuestionDraftHTTPV1(t, h, request, http.StatusOK); !reflect.DeepEqual(ack, repeated) {
		t.Fatal("same revision changed actual ACK")
	}
	view := readTimedQuestionPrivateDisplay(t, h, selector)
	if view.Status != "pending" || view.AnswerState != "pending" || view.TimedWait != nil || view.Submission != nil || view.LateReply != nil || view.Draft == nil || view.DraftRevision == nil || *view.DraftRevision != request.Revision || !reflect.DeepEqual(*view.Draft, request.Draft) {
		t.Fatal("real Host draft changed legacy answer/gate state")
	}
	first := request.Revision
	request.Revision = "00000000-0000-4000-8000-000000000052"
	privateQuestionDraftHTTPV1(t, h, request, http.StatusConflict)
	request.ExpectedRevision = &first
	request.Draft.Answers[0].CustomText += "second"
	privateQuestionDraftHTTPV1(t, h, request, http.StatusOK)
	bad := request
	bad.Revision = "00000000-0000-4000-8000-000000000053"
	bad.ExpectedRevision = &request.Revision
	bad.Draft = domaincontinuation.CloneUserInputDraftSnapshotV1(request.Draft)
	bad.Draft.Answers[0].OptionIDs = []string{selector.RequestID + "_2_option_1"}
	privateQuestionDraftHTTPV1(t, h, bad, http.StatusBadRequest)
	binding := continuationapp.UserInputLocalDisplayBindingV1{ThreadID: selector.ThreadID, TurnID: selector.TurnID, ItemID: selector.ItemID, InputID: selector.RequestID, ReceiptID: source.ReceiptID, SecurityContext: source.Payload.SecurityContext}
	receipt, err := h.continuations.SaveUserInputDraftHost(context.Background(), binding, request.Revision, request.ExpectedRevision, request.Draft)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gatecontinuationapp.ProjectVerifiedGateRequestV1(receipt); err == nil {
		t.Fatal("private draft became public Gate")
	}
	after, err := h.store.LoadEventsSince(selector.ThreadID, 0)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("save emitted public/outbox events", err)
	}
	thread, err := h.store.GetThread(selector.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(thread)
	if strings.Contains(string(body), "SYNTHETIC_PRIVATE_DRAFT_SENTINEL") {
		t.Fatal("raw draft entered persisted public/model history")
	}
	_, err = h.continuations.DisposePendingUserInputHost(selector.RequestID, time.Now().UTC(), pending, domaincontinuation.UserInputSubmissionV1{SchemaVersion: 1, Format: "structured", Answers: json.RawMessage(`[{"id":"` + selector.RequestID + `_1","kind":"skip"},{"id":"` + selector.RequestID + `_2","kind":"skip"}]`)})
	if err != nil {
		t.Fatal(err)
	}
	// Before the public item patch, trusted source disposition must already
	// prevent a save ACK under otherwise unchanged private-view authority.
	bad = request
	bad.Revision = "00000000-0000-4000-8000-000000000054"
	bad.ExpectedRevision = &request.Revision
	privateQuestionDraftHTTPV1(t, h, bad, http.StatusNotFound)
}

func TestPrivateQuestionDraftActualContinuedHandlerReopen(t *testing.T) {
	h, pending, durable, _ := protectedSeamFixtureV1(t, "read", map[string]any{"path": "synthetic.txt"})
	h.userInputConfig = toolcatalogapp.UserInputConfig{Mode: "timed"}
	configureHistory := configureTimedQuestionHistoryAuthority(t, h)
	source := timedQuestionSignedAdmissionSource(t, h, pending.SecurityContext, serverTestHostToolCallID("synthetic_draft_reopen"), false, 13, "")
	persistTimedQuestionSignedSourceProjection(t, h, source)
	selector := port.SelectorV1{ThreadID: source.Payload.ThreadID, TurnID: source.Payload.TurnID, ItemID: source.Payload.ItemID, RequestID: source.Payload.GateID}
	request := displayapp.SaveDraftRequestV1{SelectorV1: selector, Action: "saveDraft", Revision: "00000000-0000-4000-8000-000000000061", Draft: domaincontinuation.UserInputDraftSnapshotV1{CurrentPage: 0, Answers: []domaincontinuation.UserInputDraftAnswerV1{{QuestionID: selector.RequestID + "_1", OptionIDs: []string{}, CustomText: " unfinished synthetic reopen "}}}}
	ack := privateQuestionDraftHTTPV1(t, h, request, http.StatusOK)
	if view := readTimedQuestionPrivateDisplay(t, h, selector); view.Draft == nil || view.Submission != nil || view.LateReply != nil || view.Status != "continued" {
		t.Fatal("unfinished continued source became a submission")
	}
	if result, err := h.runtimeControl().InterruptTurn(context.Background(), controlapp.InterruptTurnRequest{ThreadID: pending.ThreadID, TurnID: pending.TurnID}); err != nil || result.StatusCode != http.StatusOK {
		t.Fatal("fixture actual terminal close failed", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	err := h.Shutdown(ctx)
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	restarted := NewRuntimeServerHandler(RuntimeServerConfig{RuntimeToken: DefaultRuntimeToken, DurableTempDir: durable, DataDir: h.dataDir, ProviderID: "synthetic", BaseURL: "http://127.0.0.1:1/v1", APIKey: "synthetic-only", EndpointFormat: "chat_completions", Model: "synthetic"}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, restarted)
	restarted.turnSecurity.RiskAuthority = h.turnSecurity.RiskAuthority
	configureHistory(restarted)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := restarted.Shutdown(ctx); err != nil {
			t.Errorf("reopened draft fixture shutdown errorType=%T", err)
		}
	})
	view := readTimedQuestionPrivateDisplay(t, restarted, selector)
	if view.Draft == nil || view.DraftRevision == nil || *view.DraftRevision != request.Revision || !reflect.DeepEqual(*view.Draft, request.Draft) {
		t.Fatal("actual continuation/signer disk reopen lost draft")
	}
	if retry := privateQuestionDraftHTTPV1(t, restarted, request, http.StatusOK); !reflect.DeepEqual(retry, ack) {
		t.Fatal("reopened exact retry changed signed save")
	}
	// This test retains explicit current test RiskAuthority. It proves the
	// private handler/draft proof reopen path, not risk-process/timer recovery.
}

// This uses real signed capture, queue admission, private discard, and save
// producers. Its initial continued receipt is a fixture, not timeout evidence.
func TestPrivateQuestionDraftRetiredUUIDActualPrivateHTTPHasNoSecondEffect(t *testing.T) {
	h, pending, _, _ := protectedSeamFixtureV1(t, "read", map[string]any{"path": "synthetic.txt"})
	h.userInputConfig = toolcatalogapp.UserInputConfig{Mode: "timed"}
	configureTimedQuestionHistoryAuthority(t, h)
	source := timedQuestionSignedAdmissionSource(t, h, pending.SecurityContext, serverTestHostToolCallID("synthetic_draft_retirement"), false, 14, "")
	persistTimedQuestionSignedSourceProjection(t, h, source)
	t.Cleanup(func() {
		if h.runtimeControl().ShuttingDown() {
			return
		}
		_, _ = h.runtimeControl().InterruptTurn(context.Background(), controlapp.InterruptTurnRequest{ThreadID: pending.ThreadID, TurnID: pending.TurnID})
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := h.Shutdown(ctx); err != nil {
			t.Errorf("retirement fixture shutdown errorType=%T", err)
		}
	})
	selector := port.SelectorV1{ThreadID: source.Payload.ThreadID, TurnID: source.Payload.TurnID, ItemID: source.Payload.ItemID, RequestID: source.Payload.GateID}
	binding := continuationapp.UserInputLocalDisplayBindingV1{ThreadID: selector.ThreadID, TurnID: selector.TurnID, ItemID: selector.ItemID, InputID: selector.RequestID, ReceiptID: source.ReceiptID, SecurityContext: source.Payload.SecurityContext}
	first := displayapp.SaveDraftRequestV1{SelectorV1: selector, Action: "saveDraft", Revision: "00000000-0000-4000-8000-000000000071", Draft: domaincontinuation.UserInputDraftSnapshotV1{CurrentPage: 0, Answers: []domaincontinuation.UserInputDraftAnswerV1{{QuestionID: selector.RequestID + "_1", OptionIDs: []string{}, CustomText: "SYNTHETIC_RETIRED_DRAFT_SENTINEL"}}}}
	firstACK := privateQuestionDraftHTTPV1(t, h, first, http.StatusOK)
	// Supersession alone does not retire a UUID in the same capture set.
	middle := first
	middle.Revision = "00000000-0000-4000-8000-000000000072"
	middle.ExpectedRevision = &first.Revision
	middle.Draft = domaincontinuation.CloneUserInputDraftSnapshotV1(first.Draft)
	middle.Draft.Answers[0].CustomText = "SYNTHETIC_CURRENT_DRAFT_SENTINEL"
	privateQuestionDraftHTTPV1(t, h, middle, http.StatusOK)
	if retry := privateQuestionDraftHTTPV1(t, h, first, http.StatusOK); !reflect.DeepEqual(firstACK, retry) {
		t.Fatal("valid superseded UUID changed its original ACK")
	}
	if view := readTimedQuestionPrivateDisplay(t, h, selector); view.DraftRevision == nil || *view.DraftRevision != middle.Revision || view.Draft == nil || !reflect.DeepEqual(*view.Draft, middle.Draft) {
		t.Fatal("valid historical retry replaced the current draft")
	}
	var arguments map[string]any
	if json.Unmarshal(source.Payload.Arguments, &arguments) != nil {
		t.Fatal("synthetic source unavailable")
	}
	custom := "ordinary synthetic submitted answer"
	questions := toolcatalogapp.UserInputQuestionsForLocalDisplay(arguments, selector.RequestID, "User input required")
	_, submission, err := controlapp.NormalizeAndCaptureUserInputAnswers(controlapp.UserInputResponse{StructuredAnswers: []controlapp.UserInputAnswer{{ID: selector.RequestID + "_1", Kind: "answer", OptionIDs: []string{}, CustomText: &custom}}}, questions, "")
	if err != nil || submission == nil {
		t.Fatal("real answer normalization failed", err)
	}
	attempt := "00000000-0000-4000-8000-000000000073"
	capture, err := h.continuations.CaptureLateUserInputSubmissionHost(context.Background(), binding, attempt, *submission)
	if err != nil {
		t.Fatal(err)
	}
	carrier, err := h.verifiedLateUserInputSteeringV1(context.Background(), binding, attempt, capture.ReceiptID, pending.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := h.runtimeControl().SteerTurn(context.WithValue(context.Background(), lateUserInputSteeringContextKeyV1{}, carrier), carrier.requestV1())
	if err != nil || result.StatusCode != http.StatusOK {
		t.Fatalf("real queue admission status=%d errorType=%T", result.StatusCode, err)
	}
	queued, err := h.store.readUserInputLateReplyV1(selector.ThreadID, selector.RequestID, attempt)
	if err != nil || queued == nil || queued.State != "queued" {
		t.Fatal("capture was not actually queued", err)
	}
	privateQuestionDraftHTTPV1(t, h, first, http.StatusNotFound)
	discardRequest := displayapp.DiscardLateReplyRequestV1{SelectorV1: selector, Action: "discardLateReply", ClientUserMessageID: attempt, ExpectedTurnID: pending.TurnID}
	body, err := json.Marshal(discardRequest)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, httpapi.UserInputLocalDisplayPathV1, bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	r.Header.Set(httpapi.LocalDisplayHeaderV1, httpapi.LocalDisplayHeaderValueV1)
	w := httptest.NewRecorder()
	httpapi.LocalDisplayMuxV1{RuntimeToken: DefaultRuntimeToken, UserInputs: h.UserInputLocalDisplayV1()}.ServeHTTP(w, r)
	var discardACK displayapp.DiscardLateReplyResponseV1
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &discardACK) != nil || !displayapp.ValidateDiscardLateReplyResponseV1(discardACK, discardRequest) {
		t.Fatal("real private discard did not acknowledge its proof")
	}
	proof, err := h.store.readUserInputLateReplyV1(selector.ThreadID, selector.RequestID, attempt)
	if err != nil || proof == nil || proof.State != "discarded" {
		t.Fatal("discard ACK had no signed stored proof", err)
	}
	if _, err := domainsteering.EntryAuthorityMaterialV1(proof.Entry, pending.SecurityContext.ContextDigest); err != nil {
		t.Fatal(err)
	}
	beforeEvents, err := h.store.LoadEventsSince(selector.ThreadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	beforeThread, err := h.store.GetThread(selector.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	beforeInventory, err := h.continuations.TrustedInventoryV1(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	privateQuestionDraftHTTPV1(t, h, first, http.StatusNotFound)
	privateQuestionDraftHTTPV1(t, h, middle, http.StatusNotFound)
	cleared := readTimedQuestionPrivateDisplay(t, h, selector)
	if cleared.Draft != nil || cleared.DraftRevision == nil || *cleared.DraftRevision != middle.Revision {
		t.Fatal("retired UUID restored the cleared content or lost the cursor")
	}
	afterProof, err := h.store.readUserInputLateReplyV1(selector.ThreadID, selector.RequestID, attempt)
	if err != nil || !reflect.DeepEqual(proof, afterProof) {
		t.Fatal("retired retry changed signed queue/discard proof", err)
	}
	afterEvents, err := h.store.LoadEventsSince(selector.ThreadID, 0)
	if err != nil || !reflect.DeepEqual(beforeEvents, afterEvents) {
		t.Fatal("retired retry emitted events", err)
	}
	afterThread, err := h.store.GetThread(selector.ThreadID)
	if err != nil || !reflect.DeepEqual(beforeThread, afterThread) {
		t.Fatal("retired retry changed public/model history", err)
	}
	afterInventory, err := h.continuations.TrustedInventoryV1(context.Background())
	inventoryBody := func(inventory continuationapp.TrustedInventoryV1) []byte {
		receipts := map[string]domaincontinuation.Receipt{}
		for _, receipt := range inventory.Receipts {
			receipts[receipt.Payload.GateID] = receipt
		}
		body, err := json.Marshal(struct {
			Receipts     map[string]domaincontinuation.Receipt
			Dispositions map[string]domaincontinuation.Disposition
		}{receipts, inventory.Dispositions})
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	beforeJSON := inventoryBody(beforeInventory)
	afterJSON := inventoryBody(afterInventory)
	if err != nil || !bytes.Equal(beforeJSON, afterJSON) {
		t.Fatal("retired retry changed signed original/capture/disposition inventory", err)
	}
	if strings.Contains(string(afterJSON), "SYNTHETIC_RETIRED_DRAFT_SENTINEL") {
		t.Fatal("raw draft entered restoration inventory")
	}
	next := middle
	next.Revision = "00000000-0000-4000-8000-000000000074"
	next.ExpectedRevision = &middle.Revision
	next.Draft = domaincontinuation.CloneUserInputDraftSnapshotV1(middle.Draft)
	next.Draft.Answers[0].CustomText = "SYNTHETIC_AFTER_DISCARD_DRAFT_SENTINEL"
	nextACK := privateQuestionDraftHTTPV1(t, h, next, http.StatusOK)
	currentProof, err := h.continuations.SaveUserInputDraftHost(context.Background(), binding, next.Revision, next.ExpectedRevision, next.Draft)
	if err != nil {
		t.Fatal(err)
	}
	privateQuestionDraftHTTPV1(t, h, first, http.StatusNotFound)
	if retry := privateQuestionDraftHTTPV1(t, h, next, http.StatusOK); !reflect.DeepEqual(nextACK, retry) {
		t.Fatal("current capture set retry changed its ACK")
	}
	if retry, err := h.continuations.SaveUserInputDraftHost(context.Background(), binding, next.Revision, next.ExpectedRevision, next.Draft); err != nil || !reflect.DeepEqual(currentProof, retry) {
		t.Fatal("current capture set retry changed signed content", err)
	}
	if view := readTimedQuestionPrivateDisplay(t, h, selector); view.Status != "continued" || view.DraftRevision == nil || *view.DraftRevision != next.Revision || view.Draft == nil || !reflect.DeepEqual(*view.Draft, next.Draft) || view.LateReply == nil || view.LateReply.State != "discarded" {
		t.Fatal("retired retry replaced new editable content or queue state")
	}
}

// The same fixture compiles against the exact baseline and fixed producer.
// It uses real signed source/save/display/capture and disk-reopen owners;
// explicit signed continuation here does not claim real timer settlement.
func TestPrivateQuestionDraftProductionChain(t *testing.T) {
	h, workerPending, durable, _ := protectedSeamFixtureV1(t, "read", map[string]any{"path": "synthetic.txt"})
	h.userInputConfig = toolcatalogapp.UserInputConfig{Mode: "timed"}
	configureHistory := configureTimedQuestionHistoryAuthority(t, h)
	source, pending := timedQuestionSignedPendingAdmissionSource(t, h, workerPending.SecurityContext, serverTestHostToolCallID("synthetic_draft_production_chain"), false, 15, "")
	selector := port.SelectorV1{ThreadID: source.Payload.ThreadID, TurnID: source.Payload.TurnID, ItemID: source.Payload.ItemID, RequestID: source.Payload.GateID}
	projection, err := gatecontinuationapp.ProjectVerifiedGateRequestV1(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.store.EnsureGateRequestItemExact(selector.ThreadID, selector.TurnID, projection.Item); err != nil {
		t.Fatal(err)
	}
	if err := h.recordPendingGateRequest(projection.Event); err != nil {
		t.Fatal(err)
	}
	current := h
	t.Cleanup(func() {
		if !current.runtimeControl().ShuttingDown() {
			_, _ = current.runtimeControl().InterruptTurn(context.Background(), controlapp.InterruptTurnRequest{ThreadID: workerPending.ThreadID, TurnID: workerPending.TurnID})
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := current.Shutdown(ctx); err != nil {
				t.Errorf("production-chain shutdown errorType=%T", err)
			}
		}
	})
	call := func(command any, want int) map[string]any {
		t.Helper()
		body, err := json.Marshal(command)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, httpapi.UserInputLocalDisplayPathV1, bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
		r.Header.Set(httpapi.LocalDisplayHeaderV1, httpapi.LocalDisplayHeaderValueV1)
		w := httptest.NewRecorder()
		httpapi.LocalDisplayMuxV1{RuntimeToken: DefaultRuntimeToken, UserInputs: current.UserInputLocalDisplayV1()}.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("actual signed source/read precondition succeeded; production draft route status=%d want=%d", w.Code, want)
		}
		var result map[string]any
		if json.Unmarshal(w.Body.Bytes(), &result) != nil {
			t.Fatal("private route returned invalid JSON")
		}
		if want == http.StatusNotFound && !reflect.DeepEqual(result, map[string]any{"code": "not_found", "message": "The requested resource was not found."}) {
			t.Fatal("retired draft did not return fixed closed404")
		}
		return result
	}
	// Fingerprint only real synthetic continuation CAS record JSON, not keys,
	// leases or metadata. No raw protected body or digest is printed.
	auditRecords := func() map[string]string {
		t.Helper()
		result := map[string]string{}
		root := filepath.Join(current.dataDir, "private", "gate-continuations")
		for _, partition := range []string{"receipts-v2", "dispositions-v2"} {
			err := filepath.WalkDir(filepath.Join(root, partition), func(path string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() {
					return nil
				}
				name := strings.TrimSuffix(entry.Name(), ".json")
				if len(name) != 64 {
					return nil
				}
				if _, err := hex.DecodeString(name); err != nil {
					return nil
				}
				body, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				digest := sha256.Sum256(body)
				rel, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}
				result[rel] = hex.EncodeToString(digest[:])
				return nil
			})
			if err != nil {
				t.Fatal("synthetic CAS record inventory unavailable", err)
			}
		}
		return result
	}
	initial := call(selector, http.StatusOK)
	if initial["status"] != "pending" || initial["submission"] != nil {
		t.Fatal("real signed pending source/private read precondition failed")
	}
	t.Log("real signed pending source/private read precondition PASS")
	revision := "00000000-0000-4000-8000-000000000081"
	draft := map[string]any{"currentPage": 0, "answers": []any{map[string]any{"questionId": selector.RequestID + "_1", "optionIds": []string{}, "customText": "  \n ", "skipped": false}}}
	command := map[string]any{"threadId": selector.ThreadID, "turnId": selector.TurnID, "itemId": selector.ItemID, "requestId": selector.RequestID, "action": "saveDraft", "revision": revision, "expectedRevision": nil, "draft": draft}
	expectedJSON, _ := json.Marshal(draft)
	var expectedDraft any
	_ = json.Unmarshal(expectedJSON, &expectedDraft)
	baseRecords := auditRecords()
	ack := call(command, http.StatusOK)
	if len(ack) != 8 || ack["status"] != "pending" || ack["draftRevision"] != revision || !reflect.DeepEqual(ack["draft"], expectedDraft) {
		t.Fatal("real save changed partial blank payload or invented terminal ACK")
	}
	savedRecords := auditRecords()
	if len(savedRecords) != len(baseRecords)+1 {
		t.Fatal("first real save did not add exactly one CAS receipt")
	}
	if retry := call(command, http.StatusOK); !reflect.DeepEqual(ack, retry) || !reflect.DeepEqual(savedRecords, auditRecords()) {
		t.Fatal("exact retry rewrote signed CAS records or changed same-status ACK")
	}
	savedView := call(selector, http.StatusOK)
	if savedView["status"] != "pending" || savedView["submission"] != nil || savedView["draftRevision"] != revision || !reflect.DeepEqual(savedView["draft"], expectedDraft) {
		t.Fatal("real private read lost unsubmitted blank draft")
	}
	// Real signed transition/projection; no registered wait/timer is fabricated.
	if err := h.continuations.DisposePendingHost(selector.RequestID, domaincontinuation.StatusContinued, "user_input_continued", time.Now().UTC(), pending); err != nil {
		t.Fatal(err)
	}
	persistTimedQuestionSignedSourceProjection(t, h, source)
	continuedRecords := auditRecords()
	continuedACK := call(command, http.StatusOK)
	if continuedACK["status"] != "continued" {
		t.Fatal("ACK did not observe current Host continued state")
	}
	for _, field := range []string{"schemaVersion", "threadId", "turnId", "itemId", "requestId", "draftRevision", "draft"} {
		if !reflect.DeepEqual(ack[field], continuedACK[field]) {
			t.Fatal("state observation changed original save identity/payload")
		}
	}
	if !reflect.DeepEqual(continuedRecords, auditRecords()) {
		t.Fatal("current-state retry rewrote the original signed save")
	}
	continuedView := call(selector, http.StatusOK)
	if continuedView["status"] != "continued" || continuedView["submission"] != nil || continuedView["lateReply"] != nil {
		t.Fatal("draft ACK made the original source submitted or accepted")
	}
	if result, err := h.runtimeControl().InterruptTurn(context.Background(), controlapp.InterruptTurnRequest{ThreadID: workerPending.ThreadID, TurnID: workerPending.TurnID}); err != nil || result.StatusCode != http.StatusOK {
		t.Fatal("actual fixture terminal close failed", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	err = h.Shutdown(ctx)
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	current = NewRuntimeServerHandler(RuntimeServerConfig{RuntimeToken: DefaultRuntimeToken, DurableTempDir: durable, DataDir: h.dataDir, ProviderID: "synthetic", BaseURL: "http://127.0.0.1:1/v1", APIKey: "synthetic-only", EndpointFormat: "chat_completions", Model: "synthetic"}).(*runtimeServerHandler)
	configureServerGeneralExecution(t, current)
	current.turnSecurity.RiskAuthority = h.turnSecurity.RiskAuthority
	configureHistory(current)
	reopened := call(selector, http.StatusOK)
	if reopened["status"] != "continued" || reopened["draftRevision"] != revision || !reflect.DeepEqual(reopened["draft"], expectedDraft) {
		t.Fatal("actual protected-store/file-signer reopen lost original draft")
	}
	beforeRetry := auditRecords()
	if retry := call(command, http.StatusOK); !reflect.DeepEqual(continuedACK, retry) || !reflect.DeepEqual(beforeRetry, auditRecords()) {
		t.Fatal("reopen exact retry rewrote signed CAS or changed saved result")
	}
	binding := continuationapp.UserInputLocalDisplayBindingV1{ThreadID: selector.ThreadID, TurnID: selector.TurnID, ItemID: selector.ItemID, InputID: selector.RequestID, ReceiptID: source.ReceiptID, SecurityContext: source.Payload.SecurityContext}
	var arguments map[string]any
	if json.Unmarshal(source.Payload.Arguments, &arguments) != nil {
		t.Fatal("synthetic source args unavailable")
	}
	custom := "ordinary synthetic captured answer"
	questions := toolcatalogapp.UserInputQuestionsForLocalDisplay(arguments, selector.RequestID, "User input required")
	_, submission, err := controlapp.NormalizeAndCaptureUserInputAnswers(controlapp.UserInputResponse{StructuredAnswers: []controlapp.UserInputAnswer{{ID: selector.RequestID + "_1", Kind: "answer", OptionIDs: []string{}, CustomText: &custom}}}, questions, "")
	if err != nil || submission == nil {
		t.Fatal("real normalization failed", err)
	}
	capture, err := current.continuations.CaptureLateUserInputSubmissionHost(context.Background(), binding, "00000000-0000-4000-8000-000000000082", *submission)
	if err != nil {
		t.Fatal(err)
	}
	_, captureProof, err := current.continuations.ResolveTrustedDispositionForAudit(context.Background(), capture.Payload.GateID)
	if err != nil {
		t.Fatal(err)
	}
	retiredRecords := auditRecords()
	beforeEvents, err := current.store.LoadEventsSince(selector.ThreadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	beforeThread, err := current.store.GetThread(selector.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	call(command, http.StatusNotFound)
	retiredView := call(selector, http.StatusOK)
	if retiredView["draft"] != nil || retiredView["draftRevision"] != revision || retiredView["status"] != "continued" {
		t.Fatal("real capture retirement restored content or changed original request")
	}
	afterCapture, afterProof, err := current.continuations.ResolveTrustedDispositionForAudit(context.Background(), capture.Payload.GateID)
	if err != nil || !reflect.DeepEqual(capture, afterCapture) || !reflect.DeepEqual(captureProof, afterProof) || !reflect.DeepEqual(retiredRecords, auditRecords()) {
		t.Fatal("retired retry changed signed source/draft/capture/disposition records", err)
	}
	afterEvents, err := current.store.LoadEventsSince(selector.ThreadID, 0)
	if err != nil || !reflect.DeepEqual(beforeEvents, afterEvents) {
		t.Fatal("retired retry emitted events", err)
	}
	afterThread, err := current.store.GetThread(selector.ThreadID)
	if err != nil || !reflect.DeepEqual(beforeThread, afterThread) {
		t.Fatal("retired retry changed public history", err)
	}
	// This proves actual signer/store reopen under the same explicit current
	// test RiskAuthority. It does not establish process-risk or timer recovery.
}
