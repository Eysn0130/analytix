package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	controlapp "analytix.local/runtime-go/internal/app/control"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func TestDurableSteeringExactPrefixCASLeavesNewerTailPending(t *testing.T) {
	store, err := NewProductionDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	configureDurableSteeringAuthority(t, store)
	thread, err := store.CreateThread(
		map[string]any{"title": "Exact steering prefix", "workspace": "/workspace/steering-prefix"},
		"/workspace/steering-prefix",
	)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn-steering-prefix"
	securityContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: "/workspace/steering-prefix",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("steering-prefix-manifest")),
		ContextEpoch:       1, IssuedAt: time.Date(2026, 7, 27, 2, 0, 0, 0, time.UTC),
	})
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "prompt": "start",
		"steering": []any{}, "items": []any{}, "createdAt": "2026-07-27T02:00:00Z", "startedAt": "2026-07-27T02:00:00Z",
		"securityContext": securityRecord,
	}, "", map[string]any{"securityState": securityRecord}); err != nil {
		t.Fatal(err)
	}

	admitted := make([]map[string]any, 0, 3)
	for index, input := range []struct {
		effect domainsecurity.LogicalEffect
		text   string
	}{
		{domainsecurity.LogicalEffectOrdinary, "ordinary one"},
		{domainsecurity.LogicalEffectOrdinary, "ordinary two"},
		{domainsecurity.LogicalEffectCaseData, "case tail"},
	} {
		clientID := "client-steering-prefix-" + string(rune('1'+index))
		entry, admitErr := store.AdmitSteeringEntryForContext(
			threadID, turnID, turnID, securityContext.ContextDigest,
			map[string]any{
				"id": domainsteering.EntryIDV1(turnID, clientID), "clientUserMessageId": clientID,
				"text": input.text, "admittedAt": "2026-07-27T02:00:01Z", "delivery": "steer",
				"logicalEffect": string(input.effect), "ordinaryWork": input.effect == domainsecurity.LogicalEffectOrdinary,
			},
		)
		if admitErr != nil {
			t.Fatal(admitErr)
		}
		admitted = append(admitted, entry)
	}
	expected := make([]domainsteering.PendingEntryExpectationV1, 0, 2)
	for _, entry := range admitted[:2] {
		candidate, expectationErr := domainsteering.NewPendingEntryExpectationV1(entry)
		if expectationErr != nil {
			t.Fatal(expectationErr)
		}
		expected = append(expected, candidate)
	}
	promoted, items, err := store.PromotePendingSteeringEntryPrefixForContext(
		threadID, turnID, securityContext.ContextDigest, expected,
	)
	if err != nil || len(promoted) != 2 || len(items) != 2 {
		t.Fatalf("exact prefix promotion failed: promoted=%#v items=%#v err=%v", promoted, items, err)
	}
	pending, err := store.PendingSteeringEntriesForContext(threadID, turnID, securityContext.ContextDigest)
	if err != nil || len(pending) != 1 || stringField(pending[0], "id") != stringField(admitted[2], "id") {
		t.Fatalf("prefix promotion consumed newer tail: pending=%#v err=%v", pending, err)
	}

	staleEntries, staleItems, err := store.PromotePendingSteeringEntryPrefixForContext(
		threadID, turnID, securityContext.ContextDigest, expected,
	)
	if !errors.Is(err, domainsteering.ErrPendingPrefixMismatch) || len(staleEntries) != 0 || len(staleItems) != 0 {
		t.Fatalf("stale prefix CAS did not fail closed: entries=%#v items=%#v err=%v", staleEntries, staleItems, err)
	}
	pending, err = store.PendingSteeringEntriesForContext(threadID, turnID, securityContext.ContextDigest)
	if err != nil || len(pending) != 1 || stringField(pending[0], "id") != stringField(admitted[2], "id") {
		t.Fatalf("failed CAS changed durable pending tail: pending=%#v err=%v", pending, err)
	}

	lastEntries, lastItems, err := store.PromotePendingSteeringEntriesForContext(
		threadID, turnID, securityContext.ContextDigest,
	)
	if err != nil || len(lastEntries) != 1 || len(lastItems) != 1 ||
		stringField(lastEntries[0], "id") != stringField(admitted[2], "id") {
		t.Fatalf("legacy promote-all wrapper did not preserve the remaining tail: entries=%#v items=%#v err=%v", lastEntries, lastItems, err)
	}
}

// Synthetic admission exercises the actual store mutex/signature/persistence;
// the private HTTP fixture separately supplies a real signed capture source.
func newDurableQuestionDiscardFixture(t *testing.T) (*DurableEventSessionStore, string, string, string, domainsteering.PendingEntryExpectationV1) {
	t.Helper()
	store, err := NewProductionDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	configureDurableSteeringAuthority(t, store)
	thread, err := store.CreateThread(map[string]any{"title": "Synthetic discard", "workspace": "/workspace/discard"}, "/workspace/discard")
	if err != nil {
		t.Fatal(err)
	}
	threadID, turnID := stringField(thread, "id"), "turn-discard"
	frozen := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: "/workspace/discard", ContextEpoch: 1, IssuedAt: time.Date(2026, 7, 18, 1, 2, 0, 0, time.UTC)})
	record := turnsecurityapp.PublicRecord(frozen)
	if err := store.AppendTurnToThread(threadID, map[string]any{"id": turnID, "threadId": threadID, "status": "running", "steering": []any{}, "items": []any{}, "createdAt": "2026-07-18T01:02:00Z", "securityContext": record}, "", map[string]any{"securityState": record}); err != nil {
		t.Fatal(err)
	}
	client, source := "00000000-0000-4000-8000-000000000091", "input_"+strings.Repeat("a", 64)
	identity := domainsteering.UserInputReplyIdentityV1{CallID: "call_host_" + strings.Repeat("b", 64), ToolName: "request_user_input", Questions: []domainsteering.UserInputReplyQuestionIdentityV1{{ID: source + "_1", OptionIDs: []string{}, SelectedOptionIDs: []string{}}}}
	entry, err := store.AdmitSteeringEntryForContext(threadID, turnID, turnID, frozen.ContextDigest, map[string]any{"id": domainsteering.EntryIDV1(turnID, client), "clientUserMessageId": client, "text": `{"kind":"question_reply_content","questions":[{"question":"Synthetic question?","header":"","options":[],"multiSelect":false}],"answers":[{"selected":[],"custom":"synthetic answer"}]}`, "admittedAt": "2026-07-18T01:02:03Z", "delivery": "steer", "userInputId": source, "userInputCaptureReceiptId": strings.Repeat("c", 64), "userInputReplyIdentity": domainsteering.UserInputReplyIdentityMapV1(identity)})
	if err != nil {
		t.Fatal(err)
	}
	expected, err := domainsteering.NewPendingEntryExpectationV1(entry)
	if err != nil {
		t.Fatal(err)
	}
	return store, threadID, turnID, frozen.ContextDigest, expected
}

func discardDurableQuestionFixture(ctx context.Context, store *DurableEventSessionStore, threadID, turnID, digest string, expected domainsteering.PendingEntryExpectationV1) (*userInputLateReplyRecordV1, error) {
	return store.discardUserInputLateReplyV1(ctx, threadID, "input_"+strings.Repeat("a", 64), "00000000-0000-4000-8000-000000000091", turnID, strings.Repeat("c", 64), digest, expected)
}

func TestDurableQuestionDiscardAndPromotionShareOneMutex(t *testing.T) {
	store, threadID, turnID, digest, expected := newDurableQuestionDiscardFixture(t)
	start := make(chan struct{})
	discardErr, promoteErr := make(chan error, 1), make(chan error, 1)
	go func() {
		<-start
		_, err := discardDurableQuestionFixture(context.Background(), store, threadID, turnID, digest, expected)
		discardErr <- err
	}()
	go func() {
		<-start
		_, _, err := store.PromotePendingSteeringEntryPrefixForContext(threadID, turnID, digest, []domainsteering.PendingEntryExpectationV1{expected})
		promoteErr <- err
	}()
	close(start)
	dErr, pErr := <-discardErr, <-promoteErr
	delivery, err := store.readUserInputLateReplyV1(threadID, "input_"+strings.Repeat("a", 64), "00000000-0000-4000-8000-000000000091")
	if err != nil || delivery == nil {
		t.Fatal("race lost delivery", err)
	}
	switch delivery.State {
	case "discarded":
		if dErr != nil || !errors.Is(pErr, domainsteering.ErrPendingPrefixMismatch) {
			t.Fatal("discard winner also promoted", dErr, pErr)
		}
	case "accepted":
		if pErr != nil || !errors.Is(dErr, domainsteering.ErrEntryPromoted) {
			t.Fatal("promotion winner was withdrawn", dErr, pErr)
		}
	default:
		t.Fatal("race remained queued")
	}
	pending, err := store.PendingSteeringEntriesForContext(threadID, turnID, digest)
	if err != nil || len(pending) != 0 {
		t.Fatal("winner did not settle pending entry", err)
	}
}

func TestDurableQuestionDiscardReopensAndKeepsOriginalProof(t *testing.T) {
	store, threadID, turnID, digest, expected := newDurableQuestionDiscardFixture(t)
	first, err := discardDurableQuestionFixture(context.Background(), store, threadID, turnID, digest, expected)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(thread)
	reopened, err := NewProductionDurableEventSessionStore(store.root)
	if err != nil {
		t.Fatal(err)
	}
	reopened.SetSteeringAuthority(store.steeringAuthority)
	replayed, err := discardDurableQuestionFixture(context.Background(), reopened, threadID, turnID, digest, expected)
	if err != nil || !reflect.DeepEqual(first, replayed) {
		t.Fatal("reopen replay changed signed cancellation", err)
	}
	afterThread, err := reopened.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(afterThread)
	if string(before) != string(after) {
		t.Fatal("idempotent discard rewrote primary state")
	}
	if entries, items, err := reopened.PromotePendingSteeringEntriesForContext(threadID, turnID, digest); err != nil || len(entries) != 0 || len(items) != 0 {
		t.Fatal("reopened discard promoted", err)
	}
}

// This focused store/producer check is not timed-wait or HTTP acceptance.
func TestDurableLargeQuestionPreviewUsesProductionProducersV1(t *testing.T) {
	store, threadID, turnID, digest, _ := newDurableQuestionDiscardFixture(t)
	client, source := "00000000-0000-4000-8000-0000000000a1", "input_"+strings.Repeat("d", 64)
	identity := domainsteering.UserInputReplyIdentityV1{CallID: "call_host_" + strings.Repeat("e", 64), ToolName: "request_user_input", Questions: []domainsteering.UserInputReplyQuestionIdentityV1{}}
	content := domainsteering.UserInputReplyContentV1{Kind: domainsteering.UserInputReplyContentKindV1, Questions: []domainsteering.UserInputReplyQuestionContentV1{}, Answers: []domainsteering.UserInputReplyAnswerContentV1{}}
	for index := 0; index < 3; index++ {
		identity.Questions = append(identity.Questions, domainsteering.UserInputReplyQuestionIdentityV1{ID: fmt.Sprintf("%s_%d", source, index+1), OptionIDs: []string{}, SelectedOptionIDs: []string{}})
		content.Questions = append(content.Questions, domainsteering.UserInputReplyQuestionContentV1{Question: "Synthetic detail?", Options: []domainsteering.UserInputReplyOptionContentV1{}})
		content.Answers = append(content.Answers, domainsteering.UserInputReplyAnswerContentV1{Selected: []string{}, Custom: strings.Repeat(string(rune('a'+index)), 400000)})
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.AdmitSteeringEntryForContext(threadID, turnID, turnID, digest, map[string]any{"id": domainsteering.EntryIDV1(turnID, client), "clientUserMessageId": client, "text": string(encoded), "admittedAt": "2026-07-18T01:02:04Z", "delivery": "steer", "userInputId": source, "userInputCaptureReceiptId": strings.Repeat("f", 64), "userInputReplyIdentity": domainsteering.UserInputReplyIdentityMapV1(identity)})
	if err != nil {
		t.Fatalf("complete legal production producer preview: %v", err)
	}
}

func TestForkEventBoundaryLargeQuestionForkRestartAndRefork(t *testing.T) {
	store, err := newForkBoundaryStoreV1(t, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	configureDurableSteeringAuthority(t, store)
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{"title": "Synthetic large Question", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID, turnID := stringField(thread, "id"), "turn_large_question"
	parent := appendForkLoggedParentV1(t, store, threadID, turnID, workspace)
	digest := parent.Context.ContextDigest
	client, source := "00000000-0000-4000-8000-0000000000a1", "input_"+strings.Repeat("d", 64)
	identity := domainsteering.UserInputReplyIdentityV1{CallID: "call_host_" + strings.Repeat("e", 64), ToolName: "request_user_input", Questions: []domainsteering.UserInputReplyQuestionIdentityV1{}}
	content := domainsteering.UserInputReplyContentV1{Kind: domainsteering.UserInputReplyContentKindV1, Questions: []domainsteering.UserInputReplyQuestionContentV1{}, Answers: []domainsteering.UserInputReplyAnswerContentV1{}}
	for index := 0; index < 3; index++ {
		identity.Questions = append(identity.Questions, domainsteering.UserInputReplyQuestionIdentityV1{ID: fmt.Sprintf("%s_%d", source, index+1), OptionIDs: []string{}, SelectedOptionIDs: []string{}})
		content.Questions = append(content.Questions, domainsteering.UserInputReplyQuestionContentV1{Question: "Synthetic detail?", Options: []domainsteering.UserInputReplyOptionContentV1{}})
		content.Answers = append(content.Answers, domainsteering.UserInputReplyAnswerContentV1{Selected: []string{}, Custom: strings.Repeat(string(rune('a'+index)), 400000)})
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	admitted, err := store.AdmitSteeringEntryForContext(threadID, turnID, turnID, digest, map[string]any{"id": domainsteering.EntryIDV1(turnID, client), "clientUserMessageId": client, "text": string(encoded), "admittedAt": "2026-07-18T01:02:04Z", "delivery": "steer", "userInputId": source, "userInputCaptureReceiptId": strings.Repeat("f", 64), "userInputReplyIdentity": domainsteering.UserInputReplyIdentityMapV1(identity)})
	if err != nil {
		t.Fatal(err)
	}
	event, _, err := store.RecordEvent(controlapp.BuildTurnSteeredEvent(controlapp.TurnSteeredEventInput{ThreadID: threadID, TurnID: turnID, Text: stringField(admitted, "text"), ClientUserMessageID: client, Entry: admitted}))
	if err != nil {
		t.Fatalf("legal large Question admitted but publication failed: %v", err)
	}
	if event["steeringEntry"].(map[string]any)["text"] != nil {
		t.Fatal("reply duplicated into unrecognized resource slot")
	}
	finishForkFixtureV1(t, store, parent.Context)
	endSeq, err := store.HighestSeq(threadID)
	if err != nil {
		t.Fatal(err)
	}
	h := &runtimeServerHandler{store: store, runtimeToken: DefaultRuntimeToken, publicProjector: threadapp.EnsurePublicProjector(nil)}
	first := forkBoundaryHTTPV1(t, h, http.MethodPost, threadID, map[string]any{"turnId": turnID, "endSeq": endSeq}, http.StatusCreated)
	assertQuestion := func(thread map[string]any) {
		t.Helper()
		turns := listAny(thread["turns"])
		if len(turns) != 1 {
			t.Fatal("large Question inherited turn lost")
		}
		entries := listAny(turns[0].(map[string]any)["steering"])
		if len(entries) != 1 {
			t.Fatal("large Question steering history lost")
		}
		entry := entries[0].(map[string]any)
		if entry["text"] != string(encoded) {
			t.Fatal("complete three-answer history changed")
		}
		for _, key := range []string{"userInputId", "userInputCaptureReceiptId", "userInputReplyIdentity", "contextDigest", "authoritySignature", "authorityPublicKey"} {
			if _, present := entry[key]; present {
				t.Fatalf("derived Question retained authority field %s", key)
			}
		}
	}
	assertQuestion(first)
	reopened, err := newForkBoundaryStoreV1(t, store.root)
	if err != nil {
		t.Fatal(err)
	}
	reopened.SetSteeringAuthority(store.steeringAuthority)
	firstID := stringField(first, "id")
	readback, err := reopened.GetThread(firstID)
	if err != nil {
		t.Fatal(err)
	}
	assertQuestion(readback)
	creation, err := reopened.LoadEventsSince(firstID, 0)
	if err != nil {
		t.Fatal(err)
	}
	h.store = reopened
	second := forkBoundaryHTTPV1(t, h, http.MethodPost, firstID, map[string]any{"turnId": turnID, "endSeq": len(creation.Events)}, http.StatusCreated)
	assertQuestion(second)
}

// These focused event-replay cases use verified signed steering records. They
// do not claim production timer, capture-source or final Provider acceptance.
func TestForkLateQuestionAdmissionEventReplayCompatibility(t *testing.T) {
	for _, scenario := range []string{"legacy_without_metadata", "cancelled_exact", "content_conflict", "duplicate"} {
		t.Run(scenario, func(t *testing.T) {
			store, threadID, turnID, digest, expected := newDurableQuestionDiscardFixture(t)
			delivery, err := store.readUserInputLateReplyV1(threadID, "input_"+strings.Repeat("a", 64), "00000000-0000-4000-8000-000000000091")
			if err != nil || delivery == nil {
				t.Fatal("verified queued fixture missing", err)
			}
			input := controlapp.TurnSteeredEventInput{ThreadID: threadID, TurnID: turnID, Text: stringField(delivery.Entry, "text"), ClientUserMessageID: stringField(delivery.Entry, "clientUserMessageId"), Entry: delivery.Entry}
			if scenario == "legacy_without_metadata" {
				input.Entry = nil
			}
			if scenario == "content_conflict" {
				input.Text = strings.Replace(input.Text, "synthetic answer", "conflicting answer", 1)
			}
			if _, _, err := store.RecordEvent(controlapp.BuildTurnSteeredEvent(input)); err != nil {
				t.Fatal(err)
			}
			if scenario == "duplicate" {
				if _, _, err := store.RecordEvent(controlapp.BuildTurnSteeredEvent(input)); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "cancelled_exact" {
				if _, err := discardDurableQuestionFixture(context.Background(), store, threadID, turnID, digest, expected); err != nil {
					t.Fatal(err)
				}
				reopened, err := NewProductionDurableEventSessionStore(store.root)
				if err != nil {
					t.Fatal(err)
				}
				reopened.SetSteeringAuthority(store.steeringAuthority)
				store = reopened
				delivery, err = store.readUserInputLateReplyV1(threadID, "input_"+strings.Repeat("a", 64), "00000000-0000-4000-8000-000000000091")
				if err != nil || delivery == nil || delivery.State != "discarded" || delivery.Entry["status"] != "cancelled" {
					t.Fatal("cancelled signed replay fixture unavailable", err)
				}
			}
			before, err := store.HighestSeq(threadID)
			if err != nil {
				t.Fatal(err)
			}
			h := &runtimeServerHandler{store: store}
			err = h.ensureRuntimeLateUserInputEvent(threadID, delivery)
			wantError := scenario == "content_conflict" || scenario == "duplicate"
			if (err != nil) != wantError {
				t.Fatalf("admission replay %s: err=%v", scenario, err)
			}
			after, seqErr := store.HighestSeq(threadID)
			if seqErr != nil || before != after {
				t.Fatal("replay appended another admission event", seqErr)
			}
		})
	}
}

// This collector regression uses a real signed pending record. The old-turn
// fixture is read-only history; it is not a production timeout acceptance.
func TestForkQuestionHistoryVerificationPreservesLegacyAndTurnAuthority(t *testing.T) {
	store, threadID, turnID, _, _ := newDurableQuestionDiscardFixture(t)
	source, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	current := listAny(source["turns"])[0].(map[string]any)
	entry := listAny(current["steering"])[0].(map[string]any)
	earlierContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{ThreadID: threadID, TurnID: "legacy_earlier", WorkspaceRealPath: "/workspace/discard", ContextEpoch: 1, IssuedAt: time.Date(2026, 7, 18, 1, 1, 0, 0, time.UTC)})
	bound, err := domainsteering.BindPendingEntryV1(map[string]any{"id": domainsteering.EntryIDV1("legacy_earlier", stringField(entry, "clientUserMessageId")), "clientUserMessageId": entry["clientUserMessageId"], "text": "ordinary history", "admittedAt": "2026-07-18T01:01:01Z", "delivery": "steer"}, earlierContext.ContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	ordinary, err := store.steeringAuthority.Seal(context.Background(), bound, earlierContext.ContextDigest)
	if err != nil {
		t.Fatal(err)
	}
	ordinary = cancelPendingDurableSteeringEntries([]any{ordinary}, "2026-07-18T01:01:02Z", "completed")[0].(map[string]any)
	for _, scenario := range []string{"legacy_string", "invalid_entry", "pending_not_last", "identity_conflict"} {
		t.Run(scenario, func(t *testing.T) {
			candidate := cloneMap(source)
			earlier := map[string]any{"id": "legacy_earlier", "status": "completed", "steering": []any{"legacy history"}, "items": []any{}}
			candidate["turns"] = append([]any{earlier}, listAny(candidate["turns"])...)
			switch scenario {
			case "invalid_entry":
				earlier["steering"] = []any{float64(1)}
			case "pending_not_last":
				candidate["turns"] = append(listAny(candidate["turns"]), map[string]any{"id": "later", "status": "completed", "steering": []any{}, "items": []any{}})
			case "identity_conflict":
				earlier["threadId"] = threadID
				earlier["securityContext"] = turnsecurityapp.PublicRecord(earlierContext)
				earlier["finishedAt"] = "2026-07-18T01:01:02Z"
				earlier["steering"] = append(listAny(earlier["steering"]), ordinary)
			}
			before := cloneMap(candidate)
			store.mu.Lock()
			verified, verifyErr := store.forkQuestionHistoryEntriesNoLockV1(candidate)
			store.mu.Unlock()
			if !reflect.DeepEqual(candidate, before) {
				t.Fatal("read-only verification changed original history")
			}
			if scenario == "legacy_string" {
				if verifyErr != nil || !reflect.DeepEqual(verified[stringField(entry, "id")], entry) {
					t.Fatal("legacy history blocked genuine signed Question", verifyErr)
				}
				if stringField(listAny(candidate["turns"])[1].(map[string]any), "id") != turnID {
					t.Fatal("turn order changed")
				}
			} else if !errors.Is(verifyErr, threadapp.ErrForkBoundaryUnavailable) {
				t.Fatalf("%s lost original authority constraint: %v", scenario, verifyErr)
			}
		})
	}
}
