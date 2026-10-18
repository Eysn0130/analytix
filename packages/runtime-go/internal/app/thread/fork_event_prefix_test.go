package thread

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	controlapp "analytix.local/runtime-go/internal/app/control"
	steeringauthorityapp "analytix.local/runtime-go/internal/app/steeringauthority"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	"analytix.local/runtime-go/internal/contracts"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	testsecurity "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

// This pure replay fixture proves committed flat-event folding, not a timer
// or late-queue outcome. The server regression uses the production timer.
func TestForkQuestionEventPrefixUsesCommittedRequestAndResolutionV1(t *testing.T) {
	for _, scenario := range []struct {
		name, status, placement string
	}{
		{"timed-continued", "continued", "before"},
		{"legacy-submitted", "submitted", "before"},
		{"legacy-cancelled", "cancelled", "before"},
		{"still-pending", "pending", "before"},
		{"resolution-after-E", "continued", "late-resolution"},
		{"request-after-E", "continued", "late-request"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			input, itemID := forkQuestionPrefixFixtureV1(t, scenario.status, scenario.placement, "")
			prefix, seq, err := BuildForkEventPrefixV1(input)
			if err != nil || seq != input.Boundary.EndSeq {
				t.Fatalf("committed Question prefix failed: seq=%d err=%v", seq, err)
			}
			var question map[string]any
			for _, raw := range listAny(listAny(prefix["turns"])[0].(map[string]any)["items"]) {
				item := raw.(map[string]any)
				if item["id"] == itemID {
					question = item
				}
			}
			if scenario.placement == "late-request" {
				if question != nil {
					t.Fatal("later request bytes entered E")
				}
				return
			}
			status := scenario.status
			if scenario.placement == "late-resolution" {
				status = "pending"
			}
			if question == nil || question["status"] != status || question["prompt"] != "Committed synthetic question" || question["createdAt"] != "2026-07-14T23:59:59.1Z" {
				t.Fatal("Question copied current bytes or lost its committed request identity/time")
			}
			if status == "pending" && question["finishedAt"] != nil || status != "pending" && question["finishedAt"] != "2026-07-14T23:59:59.2Z" {
				t.Fatal("resolution time was copied from the primary or crossed E")
			}
			if question["continuationReceiptId"] != nil {
				t.Fatal("replay manufactured private request authority")
			}
			if scenario.status == "continued" || scenario.status == "pending" {
				wait, _ := question["timedWait"].(map[string]any)
				if wait["timeoutSeconds"] != float64(120) || wait["deadlineAt"] != "2026-07-15T00:01:59Z" {
					t.Fatal("committed timed metadata changed")
				}
			} else if question["timedWait"] != nil {
				t.Fatal("legacy Question gained timed semantics")
			}
		})
	}
}

func TestForkQuestionEventPrefixRejectsIncompleteOrConflictingProducersV1(t *testing.T) {
	for _, scenario := range []string{"missing-request", "missing-resolution", "resolution-first", "duplicate-request", "duplicate-resolution", "wrong-request-item", "wrong-resolution-input", "wrong-resolution-turn", "wrong-resolution-prompt", "invalid-resolution-status", "missing-request-time"} {
		t.Run(scenario, func(t *testing.T) {
			input, _ := forkQuestionPrefixFixtureV1(t, "continued", "before", scenario)
			if _, _, err := BuildForkEventPrefixV1(input); !errors.Is(err, ErrForkBoundaryUnavailable) {
				t.Fatalf("unproven Question history admitted: err=%v", err)
			}
		})
	}
}

func forkQuestionPrefixFixtureV1(t *testing.T, status, placement, invalid string) (ForkEventPrefixInputV1, string) {
	t.Helper()
	source, _, _ := canonicalGeneralTerminalPublicFixture(t, "completed", "success")
	turn := listAny(source["turns"])[0].(map[string]any)
	threadID, turnID := stringField(source, "id"), stringField(turn, "id")
	start := turnapp.BuildStartRecord(turnapp.StartRecordInput{ThreadID: threadID, TurnID: turnID, Prompt: "Synthetic request", Model: "synthetic-model", CreatedAt: "2026-07-14T23:59:58Z"})
	inputID := "input_" + strings.Repeat("a", 64)
	var wait *turnapp.TimedWaitViewV1
	if status == "continued" || status == "pending" {
		deadline := "2026-07-15T00:01:59Z"
		wait = &turnapp.TimedWaitViewV1{TimeoutSeconds: 120, DeadlineAt: &deadline}
	}
	question, request := turnapp.UserInputRequestRecords(turnapp.UserInputRequestInput{ThreadID: threadID, TurnID: turnID, ItemID: "item_" + inputID, InputID: inputID, CreatedAt: "2026-07-14T23:59:59Z", Prompt: "Committed synthetic question", Questions: []map[string]any{{"id": inputID + "_1", "question": "Choose later?", "options": []any{}, "multiSelect": false}}, ContinuationReceiptID: "synthetic-private-receipt", TimedWait: wait})
	resolution := turnapp.UserInputResolvedEvent(turnapp.PendingGateCancellation{ThreadID: threadID, TurnID: turnID, ItemID: "item_" + inputID, ID: inputID, Prompt: "Committed synthetic question"}, status)
	request["timestamp"], resolution["timestamp"] = "2026-07-14T23:59:59.1Z", "2026-07-14T23:59:59.2Z"
	question["status"], question["prompt"] = status, "Current-only synthetic prompt"
	if status != "pending" {
		question["finishedAt"] = "2026-07-14T23:59:59.3Z"
	}
	turn["prompt"], turn["createdAt"] = start.Turn["prompt"], start.Turn["createdAt"]
	turn["items"] = []any{start.UserItem, question, listAny(turn["items"])[0]}
	pre := []map[string]any{start.TurnStartedEvent, start.UserItemCreatedEvent}
	post := []map[string]any{}
	gateEvents := []map[string]any{request}
	if status != "pending" {
		gateEvents = append(gateEvents, resolution)
	}
	switch invalid {
	case "missing-request":
		gateEvents = []map[string]any{resolution}
	case "missing-resolution":
		gateEvents = []map[string]any{request}
	case "resolution-first":
		gateEvents = []map[string]any{resolution, request}
	case "duplicate-request":
		gateEvents = []map[string]any{request, request, resolution}
	case "duplicate-resolution":
		gateEvents = append(gateEvents, resolution)
	case "wrong-request-item":
		request["itemId"] = "item_unbound_question"
	case "wrong-resolution-input":
		resolution["inputId"] = "input_" + strings.Repeat("b", 64)
	case "wrong-resolution-turn":
		resolution["turnId"] = "turn_unbound_question"
	case "wrong-resolution-prompt":
		resolution["prompt"] = "Unbound synthetic prompt"
	case "invalid-resolution-status":
		resolution["status"] = "approved"
	case "missing-request-time":
		delete(request, "timestamp")
	}
	switch placement {
	case "late-resolution":
		resolution["timestamp"] = "2026-07-15T00:00:00.2Z"
		pre, post = append(pre, request), append(post, resolution)
	case "late-request":
		request["timestamp"], resolution["timestamp"] = "2026-07-15T00:00:00.1Z", "2026-07-15T00:00:00.2Z"
		post = append(post, gateEvents...)
	default:
		pre = append(pre, gateEvents...)
	}
	commit, err := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(turn["generalTerminalPublication"])
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := turnapp.PrepareGeneralTerminalEventBundleV1(source, turnID, commit, len(pre)+1)
	if err != nil {
		t.Fatal(err)
	}
	events := append(append(pre, terminal...), post...)
	for index, event := range events {
		// Detached records use the exact strict JSONL numeric representation;
		// marker groups were prepared at their real next sequence above.
		copy := map[string]any{}
		body, err := json.Marshal(event)
		if err != nil || json.Unmarshal(body, &copy) != nil {
			t.Fatal("synthetic event clone failed")
		}
		copy["seq"] = index + 1
		body, err = json.Marshal(copy)
		if err != nil {
			t.Fatal(err)
		}
		events[index], err = domainjsonstrict.DecodeObject(body, domainjsonstrict.Options{RequireObject: true, MaxBytes: len(body) + 1})
		if err != nil {
			t.Fatal(err)
		}
	}
	return ForkEventPrefixInputV1{Source: source, Events: events, Boundary: ForkBoundaryV1{TurnID: turnID, EndSeq: len(pre) + len(terminal)}, Projector: EnsurePublicProjector(nil)}, "item_" + inputID
}

func TestForkBoundaryClosedInputV1(t *testing.T) {
	for _, request := range []map[string]any{{"endSeq": 1}, {"turnId": " "}, {"turnId": nil}, {"turnId": "turn", "endSeq": nil}, {"turnId": "turn", "endSeq": "1"}, {"turnId": "turn", "endSeq": 1.5}, {"turnId": "turn", "endSeq": float64(1 << 53)}, {"turnId": "turn", "endSeq": -1}, {"relation": "other"}, {"title": false}} {
		if _, err := ParseForkBoundaryV1(request); !errors.Is(err, ErrForkBoundaryInvalid) {
			t.Fatalf("invalid boundary admitted: %#v err=%v", request, err)
		}
	}
	for _, request := range []map[string]any{nil, {"relation": "side"}, {"turnId": "turn"}, {"turnId": "turn", "endSeq": json.Number("13")}} {
		if _, err := ParseForkBoundaryV1(request); err != nil {
			t.Fatalf("compatible boundary rejected: %#v err=%v", request, err)
		}
	}
}

func TestForkCreationSeedRemainsPrivateAndInertV1(t *testing.T) {
	source := map[string]any{"id": "thr_parent", "status": "idle", "turns": []any{map[string]any{"id": "turn", "threadId": "thr_parent", "status": "completed", "createdAt": "2026-10-09T00:00:00Z", "finishedAt": "2026-10-09T00:00:00Z", "items": []any{map[string]any{"id": "user", "kind": "user_message", "role": "user", "threadId": "thr_parent", "turnId": "turn", "status": "completed", "text": "synthetic input"}}}}}
	fork, err := BuildFork(ForkInput{Source: source, ForkID: "thr_child", ParentThreadID: "thr_parent", Now: "2026-10-09T00:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	event, err := BuildForkCreatedEventV1(fork, "thr_parent", "turn", 13)
	if err != nil {
		t.Fatal(err)
	}
	if event["inheritedHistorySeed"] == nil {
		t.Fatal("missing existing-log seed")
	}
	projected, visible, err := NewTrustedPublicProjector(nil).ProjectEvent("thr_child", fork, event)
	if err != nil || !visible || projected["inheritedHistorySeed"] != nil {
		t.Fatal("creation seed escaped public event projection")
	}
	seed := event["inheritedHistorySeed"].(map[string]any)
	turn := seed["turns"].([]any)[0].(map[string]any)
	turn["executionGrantId"] = "forged"
	if _, err := validateForkHistorySeedV1(fork, event, seed); !errors.Is(err, ErrForkBoundaryUnavailable) {
		t.Fatal("seed retained execution authority")
	}
}

func TestForkCompactionUsesClosedCompletionProducerOnly(t *testing.T) {
	source, event := ordinaryCompactionCompletedFixture(t)
	turn := source["turns"].([]any)[0].(map[string]any)
	turn["prompt"], turn["status"], turn["createdAt"], turn["finishedAt"] = "compacted synthetic history", "completed", "2026-07-20T00:00:00Z", "2026-07-20T00:00:00Z"
	event["turn"], event["seq"], event["timestamp"] = turn, float64(2), "2026-07-20T00:00:01Z"
	events := []map[string]any{{"kind": "compaction_started", "threadId": "thread_compaction", "turnId": "turn_compaction", "itemId": "compaction_test", "seq": float64(1), "timestamp": "2026-07-20T00:00:00Z"}, event}
	input := ForkEventPrefixInputV1{Source: source, Events: events, Boundary: ForkBoundaryV1{TurnID: "turn_compaction", EndSeq: 2}, Projector: EnsurePublicProjector(nil)}
	prefix, seq, err := BuildForkEventPrefixV1(input)
	if err != nil || seq != 2 {
		t.Fatalf("complete compaction producer: seq=%d err=%v", seq, err)
	}
	item := prefix["turns"].([]any)[0].(map[string]any)["items"].([]any)[0].(map[string]any)
	if item["reasoningExclusionProof"] != event["reasoningExclusionProof"] || item["createdAt"] != "2026-07-20T00:00:00Z" {
		t.Fatal("compaction item proof or original timestamp changed")
	}
	delete(event, "turn")
	if _, _, err := BuildForkEventPrefixV1(input); !errors.Is(err, ErrForkBoundaryUnavailable) {
		t.Fatal("old incomplete compaction copied the current item")
	}
}

func TestForkCreationBundlePreservesLongAndLegacyHistoryV1(t *testing.T) {
	for _, scenario := range []struct {
		name         string
		turns, bytes int
	}{{"many-small", 700, 10}, {"over-single-frame", 8, 300000}} {
		t.Run(scenario.name, func(t *testing.T) {
			source := map[string]any{"id": "thr_parent", "status": "idle", "turns": []any{}}
			for index := 0; index < scenario.turns; index++ {
				id := fmt.Sprintf("turn_seed_%d", index)
				text := strings.Repeat("x", scenario.bytes)
				source["turns"] = append(source["turns"].([]any), map[string]any{"id": id, "threadId": "thr_parent", "status": "completed", "prompt": text, "model": "synthetic", "createdAt": "2026-10-09T00:00:00Z", "finishedAt": "2026-10-09T00:00:00Z", "steering": []any{"legacy inert note"}, "items": []any{map[string]any{"id": "user_" + id, "kind": "user_message", "role": "user", "threadId": "thr_parent", "turnId": id, "status": "completed", "text": text}}})
			}
			fork, err := BuildFork(ForkInput{Source: source, ForkID: "thr_child", ParentThreadID: "thr_parent", Now: "2026-10-09T00:00:00Z"})
			if err != nil {
				t.Fatal(err)
			}
			events, err := BuildForkCreatedEventsV1(fork, "thr_parent", "", 0)
			if err != nil {
				t.Fatal(err)
			}
			seed, end, err := ReadForkCreatedSeedV1(fork, events)
			if err != nil || end != len(events) || len(listAny(seed["turns"])) != scenario.turns {
				t.Fatalf("complete bounded bundle rejected: E=%d err=%v", end, err)
			}
			if len(events) < 3 {
				t.Fatal("missing unique closed bundle")
			}
			for _, event := range events {
				frame, _ := json.Marshal(event)
				if len(frame)+1 > 4<<20 {
					t.Fatal("actual JSONL frame exceeds existing ceiling")
				}
				public, visible, err := NewTrustedPublicProjector(nil).ProjectEvent("thr_child", fork, event)
				if err != nil || public["inheritedHistorySeed"] != nil || public["forkSeedFragment"] != nil || public["forkSeedClose"] != nil || (event["kind"] != "thread_created" && visible) {
					t.Fatal("private seed entered public events")
				}
			}
			prefix, _, err := BuildForkEventPrefixV1(ForkEventPrefixInputV1{Source: fork, Events: events, Boundary: ForkBoundaryV1{TurnID: fmt.Sprintf("turn_seed_%d", scenario.turns-1), EndSeq: end}, Projector: EnsurePublicProjector(nil)})
			if err != nil || !reflect.DeepEqual(prefix["turns"], seed["turns"]) {
				t.Fatalf("immutable legacy seed replay changed: %v", err)
			}
			for _, broken := range [][]map[string]any{events[:len(events)-1], append([]map[string]any{events[0], events[len(events)-1]}, events[1:len(events)-1]...)} {
				if _, _, err := ReadForkCreatedSeedV1(fork, broken); !errors.Is(err, ErrForkBoundaryUnavailable) {
					t.Fatal("partial or reordered bundle accepted")
				}
			}
		})
	}
}

func TestForkCreationSeedRejectsMalformedSteeringContainerV1(t *testing.T) {
	source := map[string]any{"id": "thr_parent", "status": "idle", "turns": []any{map[string]any{"id": "turn_seed", "threadId": "thr_parent", "status": "completed", "prompt": "synthetic input", "createdAt": "2026-10-09T00:00:00Z", "finishedAt": "2026-10-09T00:00:00Z", "items": []any{}}}}
	fork, err := BuildFork(ForkInput{Source: source, ForkID: "thr_child", ParentThreadID: "thr_parent", Now: "2026-10-09T00:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []any{17, map[string]any{}, nil} {
		event, err := BuildForkCreatedEventV1(fork, "thr_parent", "", 0)
		if err != nil {
			t.Fatal(err)
		}
		seed := event["inheritedHistorySeed"].(map[string]any)
		seed["turns"].([]any)[0].(map[string]any)["steering"] = bad
		if _, _, err := ReadForkCreatedSeedV1(fork, []map[string]any{event}); !errors.Is(err, ErrForkBoundaryUnavailable) {
			t.Fatalf("malformed seed steering silently normalized: %T err=%v", bad, err)
		}
	}
}

// Use the same strict JSON decoder as the production JSONL reader: its
// integers are json.Number, unlike in-memory producer maps.
func TestForkCreationSeedStrictJSONLSourceBoundaryV1(t *testing.T) {
	source := map[string]any{"id": "thr_parent", "status": "idle", "turns": []any{map[string]any{"id": "turn", "threadId": "thr_parent", "status": "completed", "createdAt": "2026-10-09T00:00:00Z", "finishedAt": "2026-10-09T00:00:00Z", "items": []any{map[string]any{"id": "user", "kind": "user_message", "role": "user", "threadId": "thr_parent", "turnId": "turn", "status": "completed", "text": "synthetic input"}}}}}
	fork, err := BuildFork(ForkInput{Source: source, ForkID: "thr_child", ParentThreadID: "thr_parent", Now: "2026-10-09T00:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	events, err := BuildForkCreatedEventsV1(fork, "thr_parent", "turn", 13)
	if err != nil {
		t.Fatal(err)
	}
	replay := make([]map[string]any, len(events))
	for index, event := range events {
		line, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		replay[index], err = domainjsonstrict.DecodeObject(append(line, '\n'), domainjsonstrict.Options{MaxBytes: len(line) + 1})
		if err != nil {
			t.Fatal(err)
		}
	}
	header := replay[0]["inheritedHistorySeed"].(map[string]any)
	if _, ok := header["sourceEndSeq"].(json.Number); !ok {
		t.Fatal("fixture bypassed the strict JSONL number representation")
	}
	seed, end, err := ReadForkCreatedSeedV1(fork, replay)
	if err != nil || seed == nil || end != len(replay) || seed["sourceEndSeq"] != float64(13) {
		t.Fatalf("real JSONL source E failed to round-trip: end=%d err=%v", end, err)
	}
	header["sourceEndSeq"] = json.Number("14")
	replay[len(replay)-1]["forkSeedClose"].(map[string]any)["sourceEndSeq"] = json.Number("14")
	if _, _, err := ReadForkCreatedSeedV1(fork, replay); !errors.Is(err, ErrForkBoundaryUnavailable) {
		t.Fatal("header/close E differing from the decoded seed was admitted")
	}
}

// This checks the inert projection only; the server fixture supplies real
// signed admission, committed history, HTTP Fork and restart source proof.
func TestForkQuestionHistoryCarrierRequiresProvenSourceV1(t *testing.T) {
	questions, answers := []any{}, []any{}
	for index := 0; index < 3; index++ {
		questions = append(questions, map[string]any{"question": "Synthetic detail?", "header": "", "options": []any{}, "multiSelect": false})
		answers = append(answers, map[string]any{"selected": []any{}, "custom": strings.Repeat(string(rune('a'+index)), 400000)})
	}
	body, err := json.Marshal(map[string]any{"kind": "question_reply_content", "questions": questions, "answers": answers})
	if err != nil {
		t.Fatal(err)
	}
	entry := map[string]any{"id": "steer_synthetic", "clientUserMessageId": "00000000-0000-4000-8000-0000000000a1", "text": string(body), "admittedAt": "2026-10-09T00:00:00Z", "status": "cancelled"}
	if _, err := forkInertSteeringEntryV1(entry, nil); err == nil {
		t.Fatal("Question-shaped ordinary JSON bypassed string limit")
	}
	plain := map[string]any{"id": entry["id"], "text": strings.Repeat("x", (1<<20)+1)}
	if _, err := forkInertSteeringEntryV1(plain, nil); err == nil {
		t.Fatal("ordinary string limit was widened")
	}
	inert, err := forkInertSteeringEntryV1(entry, entry)
	if err != nil || inert["text"] != string(body) || inert["kind"] != "user_message" || inert["delivery"] != "steer" {
		t.Fatal("proven complete Question projection failed", err)
	}
	if _, err := forkInertSteeringEntryV1(inert, nil); err == nil {
		t.Fatal("current marker acquired source authority without signed entry or seed")
	}
	changed := make(map[string]any)
	for key, value := range entry {
		changed[key] = value
	}
	changed["displayText"] = "changed"
	if _, err := forkInertSteeringEntryV1(changed, entry); err == nil {
		t.Fatal("same opaque ID substituted different immutable content")
	}
	answers[0].(map[string]any)["custom"] = strings.Repeat("x", (1<<20)+1)
	oversize, _ := json.Marshal(map[string]any{"kind": "question_reply_content", "questions": questions, "answers": answers})
	changed["text"], changed["displayText"] = string(oversize), ""
	if _, err := forkInertSteeringEntryV1(changed, changed); err == nil {
		t.Fatal("real Question leaf string limit was widened")
	}
}

// The complete production regression supplies the native signing and recovery
// proof. This pure fixture checks that the producer never conflates its two
// namespaces or changes the historical status, time or content.
func discardedForkHistoryFixtureV1(t *testing.T) (map[string]any, map[string]any) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"kind": "question_reply_content",
		"questions": []any{map[string]any{"question": "Synthetic choice?", "header": "Choice", "options": []any{}, "multiSelect": false}},
		"answers":   []any{map[string]any{"selected": []any{}, "custom": "Synthetic discarded answer"}}})
	if err != nil {
		t.Fatal(err)
	}
	entry := map[string]any{"id": domainsteering.EntryIDV1("turn_history", "00000000-0000-4000-8000-0000000000a1"),
		"clientUserMessageId": "00000000-0000-4000-8000-0000000000a1", "text": string(body), "displayText": string(body),
		"admittedAt": "2026-10-10T00:00:00Z", "status": "cancelled", "cancelledAt": "2026-10-10T00:00:01Z",
		"cancelReason": domainsteering.UserInputDiscardReasonV1, "authoritySignature": "synthetic-native-proof",
		"cancellationAuthoritySignature": "synthetic-cancellation-proof"}
	before := contracts.CloneMap(entry)
	inert, err := forkInertSteeringEntryV1(entry, entry)
	if err != nil || !reflect.DeepEqual(before, entry) {
		t.Fatal("inert producer changed its source or rejected proved content")
	}
	return before, inert
}

func TestForkDiscardHistoryPreservesReasonWithoutCancellationAuthorityV1(t *testing.T) {
	source, inert := discardedForkHistoryFixtureV1(t)
	if inert[forkHistoryCancellationReasonV1] != domainsteering.UserInputDiscardReasonV1 || inert["cancelReason"] != nil ||
		domainsteering.HasUserInputDiscardAuthorityV1(inert) {
		t.Fatal("history acquired native discard authority")
	}
	for _, key := range []string{"text", "displayText", "status", "cancelledAt", "id", "clientUserMessageId", "admittedAt"} {
		if !reflect.DeepEqual(inert[key], source[key]) {
			t.Fatal("discard history lost an original fact")
		}
	}
	for _, key := range []string{"authoritySignature", "cancellationAuthoritySignature", "userInputId", "userInputReplyIdentity", "userInputCaptureReceiptId"} {
		if _, present := inert[key]; present {
			t.Fatal("discard history retained execution material")
		}
	}
	refork, err := forkInertSteeringEntryV1(inert, inert)
	if err != nil || !reflect.DeepEqual(refork, inert) {
		t.Fatal("refork changed the inert fact")
	}
	for _, mutate := range []func(map[string]any){
		func(e map[string]any) { e["cancelReason"] = domainsteering.UserInputDiscardReasonV1 },
		func(e map[string]any) { e[forkHistoryCancellationReasonV1] = "invented" },
		func(e map[string]any) { e["status"] = "promoted" },
		func(e map[string]any) { e["cancelledAt"] = "invalid" },
		func(e map[string]any) { delete(e, "kind") },
	} {
		candidate := contracts.CloneMap(inert)
		mutate(candidate)
		if validateForkHistoryCancellationV1(candidate) == nil {
			t.Fatal("malformed or mixed history namespace was admitted")
		}
	}
	if _, err := forkInertSteeringEntryV1(source, nil); err == nil {
		t.Fatal("unproved discard acquired an inert history namespace")
	}
}

func TestForkDiscardHistoryCreationSeedAndPublicCopyStaySeparateV1(t *testing.T) {
	_, inert := discardedForkHistoryFixtureV1(t)
	fork := map[string]any{"id": "thr_history_fork", "forkedFromThreadId": "thr_history_source", "status": "idle", "createdAt": "2026-10-10T00:00:02Z",
		"turns": []any{map[string]any{"id": "turn_history", "threadId": "thr_history_fork", "status": "completed", "prompt": "Synthetic",
			"createdAt": "2026-10-10T00:00:00Z", "finishedAt": "2026-10-10T00:00:01Z", "items": []any{}, "steering": []any{inert}}}}
	before := contracts.CloneMap(fork)
	events, err := BuildForkCreatedEventsV1(fork, "thr_history_source", "turn_history", 9)
	if err != nil {
		t.Fatal(err)
	}
	seed, end, err := ReadForkCreatedSeedV1(fork, events)
	if err != nil || end != len(events) {
		t.Fatal("complete creation seed was not recoverable")
	}
	seedBefore := contracts.CloneMap(seed)
	seedEntry := listAny(listAny(seed["turns"])[0].(map[string]any)["steering"])[0].(map[string]any)
	if seedEntry[forkHistoryCancellationReasonV1] != domainsteering.UserInputDiscardReasonV1 || seedEntry["cancelReason"] != nil {
		t.Fatal("creation seed was polluted with the native cancellation discriminator")
	}
	public, err := ProjectPublicThread(fork)
	if err != nil {
		t.Fatal(err)
	}
	entry := listAny(listAny(public["turns"])[0].(map[string]any)["steering"])[0].(map[string]any)
	if entry["cancelReason"] != domainsteering.UserInputDiscardReasonV1 || entry[forkHistoryCancellationReasonV1] != nil || entry["status"] != "cancelled" {
		t.Fatal("existing public wire lost the historical discard fact")
	}
	if !reflect.DeepEqual(fork, before) || !reflect.DeepEqual(seed, seedBefore) {
		t.Fatal("public display mapping mutated durable primary or seed")
	}
	if err := validateForkSeedTurnV1(listAny(seed["turns"])[0].(map[string]any)); err != nil {
		t.Fatal(err)
	}
}

func signedForkDiscardEntryV1(t *testing.T, turnID, digest, cancelledAt string) (map[string]any, map[string]any) {
	t.Helper()
	clientID := "00000000-0000-4000-8000-0000000000a2"
	inputID := "input_" + strings.Repeat("a", 64)
	identity := domainsteering.UserInputReplyIdentityV1{CallID: "call_host_" + strings.Repeat("b", 64), ToolName: "request_user_input", Questions: []domainsteering.UserInputReplyQuestionIdentityV1{{ID: inputID + "_1", OptionIDs: []string{}, SelectedOptionIDs: []string{}}}}
	pending, err := domainsteering.BindPendingEntryV1(map[string]any{
		"id": domainsteering.EntryIDV1(turnID, clientID), "clientUserMessageId": clientID,
		"text":       `{"kind":"question_reply_content","questions":[{"question":"Synthetic choice?","header":"","options":[],"multiSelect":false}],"answers":[{"selected":[],"custom":"Synthetic discarded answer"}]}`,
		"admittedAt": "2026-07-14T23:59:59.4Z", "delivery": "steer", "userInputId": inputID,
		"userInputCaptureReceiptId": strings.Repeat("c", 64), "userInputReplyIdentity": domainsteering.UserInputReplyIdentityMapV1(identity),
	}, digest)
	if err != nil {
		t.Fatal(err)
	}
	authority := steeringauthorityapp.NewService(newProjectionTestAuthority(0x41))
	pending, err = authority.Seal(context.Background(), pending, digest)
	if err != nil {
		t.Fatal(err)
	}
	cancelled := contracts.CloneMap(pending)
	cancelled["status"], cancelled["cancelReason"], cancelled["cancelledAt"] = "cancelled", domainsteering.UserInputDiscardReasonV1, cancelledAt
	cancelled, err = authority.CancelUserInput(context.Background(), cancelled, digest)
	if err != nil || authority.Verify(context.Background(), cancelled, digest) != nil {
		t.Fatal("synthetic native cancellation was not signed and verified")
	}
	return pending, cancelled
}

func forkCanonicalDiscardClosureFixtureV1(t *testing.T) (ForkEventPrefixInputV1, map[string]any, map[string]any) {
	t.Helper()
	input, _ := forkQuestionPrefixFixtureV1(t, "continued", "before", "")
	turn := listAny(input.Source["turns"])[0].(map[string]any)
	frozen, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil {
		t.Fatal(err)
	}
	pending, cancelled := signedForkDiscardEntryV1(t, frozen.TurnID, frozen.ContextDigest, "2026-07-14T23:59:59.5Z")
	turn["steering"] = []any{cancelled}
	input.QuestionHistoryEntries = map[string]map[string]any{stringField(cancelled, "id"): cancelled}
	admission := controlapp.BuildTurnSteeredEvent(controlapp.TurnSteeredEventInput{ThreadID: frozen.ThreadID, TurnID: frozen.TurnID, ClientUserMessageID: stringField(pending, "clientUserMessageId"), Text: stringField(pending, "text"), DisplayText: stringField(pending, "displayText"), Entry: pending})
	admission["timestamp"] = pending["admittedAt"]
	pre := append([]map[string]any{}, input.Events[:4]...)
	pre = append(pre, admission)
	commit, err := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(turn["generalTerminalPublication"])
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := turnapp.PrepareGeneralTerminalEventBundleV1(input.Source, frozen.TurnID, commit, len(pre)+1)
	if err != nil {
		t.Fatal(err)
	}
	input.Events = append(pre, terminal...)
	for index, event := range input.Events {
		body, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		copy, err := domainjsonstrict.DecodeObject(body, domainjsonstrict.Options{RequireObject: true, MaxBytes: len(body) + 1})
		if err != nil {
			t.Fatal(err)
		}
		copy["seq"] = index + 1
		input.Events[index] = copy
	}
	input.Boundary.EndSeq = len(input.Events)
	return input, pending, cancelled
}

func TestForkDiscardCanonicalTerminalRestoresOnlyOriginalInertFactV1(t *testing.T) {
	input, _, cancelled := forkCanonicalDiscardClosureFixtureV1(t)
	beforeSource, _ := json.Marshal(input.Source)
	beforeEvents, _ := json.Marshal(input.Events)
	prefix, seq, err := BuildForkEventPrefixV1(input)
	if err != nil || seq != input.Boundary.EndSeq {
		t.Fatalf("canonical discard prefix failed: seq=%d err=%v", seq, err)
	}
	turn := listAny(prefix["turns"])[0].(map[string]any)
	entry := listAny(turn["steering"])[0].(map[string]any)
	if entry["status"] != "cancelled" || entry[forkHistoryCancellationReasonV1] != domainsteering.UserInputDiscardReasonV1 || entry["cancelReason"] != nil || entry["cancelledAt"] != cancelled["cancelledAt"] || entry["displayText"] != nil || domainsteering.HasUserInputDiscardAuthorityV1(entry) {
		t.Fatal("canonical terminal did not preserve only the original inert cancellation")
	}
	foundQuestion := false
	for _, raw := range listAny(turn["items"]) {
		if item := raw.(map[string]any); item["kind"] == "user_input" {
			foundQuestion = true
			if item["status"] != "continued" {
				t.Fatal("cancellation changed the original continued Question")
			}
		}
	}
	if !foundQuestion {
		t.Fatal("terminal closure lost the original Question")
	}
	for _, key := range []string{"authoritySignature", "cancellationAuthoritySignature", "contextDigest", "userInputId", "userInputReplyIdentity", "userInputCaptureReceiptId"} {
		if _, present := entry[key]; present {
			t.Fatal("terminal closure retained native authority")
		}
	}
	afterSource, _ := json.Marshal(input.Source)
	afterEvents, _ := json.Marshal(input.Events)
	if string(beforeSource) != string(afterSource) || string(beforeEvents) != string(afterEvents) {
		t.Fatal("terminal closure mutated source or manufactured a log event")
	}
}

func TestForkDiscardTerminalClosureRetainsPrefixOnFailedPrerequisiteV1(t *testing.T) {
	for _, scenario := range []string{"E-before-terminal", "compaction", "legacy", "future-cancellation", "context-mismatch", "identity-mismatch", "duplicate-admission"} {
		t.Run(scenario, func(t *testing.T) {
			input, pending, cancelled := forkCanonicalDiscardClosureFixtureV1(t)
			original := listAny(input.Source["turns"])[0].(map[string]any)
			frozen, err := domainsecurity.ParseTurnSecurityContext(original["securityContext"])
			if err != nil {
				t.Fatal(err)
			}
			inert, err := forkInertSteeringEntryV1(pending, cancelled)
			if err != nil {
				t.Fatal(err)
			}
			prefixTurn := map[string]any{"id": frozen.TurnID, "steering": []any{inert}}
			before := contracts.CloneMap(prefixTurn)
			terminal := contracts.CloneMap(input.Events[len(input.Events)-1])
			authorities, err := domainturnterminal.GeneralTerminalProjectionAuthoritiesV1(input.Source)
			if err != nil {
				t.Fatal(err)
			}
			authority, end := authorities[frozen.TurnID], input.Boundary.EndSeq
			admissions := map[string]int{stringField(pending, "id"): 1}
			switch scenario {
			case "E-before-terminal":
				end--
			case "compaction":
				terminal["kind"] = "compaction_completed"
			case "legacy":
				authority = domainturnterminal.GeneralTerminalProjectionAuthorityV1{}
				for key := range terminal {
					if strings.HasPrefix(key, "generalTerminal") {
						delete(terminal, key)
					}
				}
			case "future-cancellation":
				_, cancelled = signedForkDiscardEntryV1(t, frozen.TurnID, frozen.ContextDigest, "2026-07-15T00:00:00.1Z")
			case "context-mismatch":
				other, err := testsecurity.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{ThreadID: frozen.ThreadID, TurnID: frozen.TurnID, WorkspaceRealPath: "/workspace", ContextEpoch: 3, IssuedAt: time.Unix(3, 0)})
				if err != nil {
					t.Fatal(err)
				}
				_, cancelled = signedForkDiscardEntryV1(t, frozen.TurnID, other.ContextDigest, "2026-07-14T23:59:59.5Z")
			case "identity-mismatch":
				_, cancelled = signedForkDiscardEntryV1(t, "later-turn", frozen.ContextDigest, "2026-07-14T23:59:59.5Z")
			case "duplicate-admission":
				admissions[stringField(pending, "id")] = 2
			}
			original["steering"] = []any{cancelled}
			input.QuestionHistoryEntries[stringField(pending, "id")] = cancelled
			restoreForkDiscardHistoryAtTerminalV1(input.Source, prefixTurn, terminal, end, authority, input.QuestionHistoryEntries, admissions)
			if !reflect.DeepEqual(prefixTurn, before) {
				t.Fatal("failed closure condition rewrote prefix history")
			}
		})
	}
}

func TestForkDiscardLaterTurnAttemptCannotEnterEarlierTerminalPrefixV1(t *testing.T) {
	input, pending, _ := forkCanonicalDiscardClosureFixtureV1(t)
	original := listAny(input.Source["turns"])[0].(map[string]any)
	original["steering"] = []any{pending}
	input.QuestionHistoryEntries[stringField(pending, "id")] = pending
	other, err := testsecurity.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{ThreadID: stringField(input.Source, "id"), TurnID: "turn-later-attempt", WorkspaceRealPath: "/workspace", ContextEpoch: 3, IssuedAt: time.Unix(3, 0)})
	if err != nil {
		t.Fatal(err)
	}
	laterPending, laterCancelled := signedForkDiscardEntryV1(t, other.TurnID, other.ContextDigest, "2026-07-15T00:00:01Z")
	start := turnapp.BuildStartRecord(turnapp.StartRecordInput{ThreadID: other.ThreadID, TurnID: other.TurnID, Prompt: "Later attempt", Model: "synthetic", CreatedAt: "2026-07-15T00:00:00.1Z"})
	start.Turn["securityContext"] = contracts.CloneValue(other)
	start.Turn["steering"], start.Turn["items"] = []any{laterCancelled}, []any{start.UserItem}
	input.Source["turns"] = append(listAny(input.Source["turns"]), start.Turn)
	input.QuestionHistoryEntries[stringField(laterCancelled, "id")] = laterCancelled
	admission := controlapp.BuildTurnSteeredEvent(controlapp.TurnSteeredEventInput{ThreadID: other.ThreadID, TurnID: other.TurnID, ClientUserMessageID: stringField(laterPending, "clientUserMessageId"), Text: stringField(laterPending, "text"), Entry: laterPending})
	for _, event := range []map[string]any{start.TurnStartedEvent, start.UserItemCreatedEvent, admission} {
		event["seq"] = len(input.Events) + 1
		input.Events = append(input.Events, event)
	}
	prefix, _, err := BuildForkEventPrefixV1(input)
	if err != nil {
		t.Fatal(err)
	}
	turns := listAny(prefix["turns"])
	entry := listAny(turns[0].(map[string]any)["steering"])[0].(map[string]any)
	if len(turns) != 1 || entry["cancelReason"] != "completed" || entry[forkHistoryCancellationReasonV1] != nil {
		t.Fatal("later signed attempt crossed the earlier terminal prefix")
	}
}

func TestForkToolKindUsesOnlyBoundProducerWithinEV1(t *testing.T) {
	for _, scenario := range []string{"started", "completed-without-start", "ready-after-start", "ready-only", "completion-after-E", "start-after-E", "wrong-name", "wrong-call", "wrong-thread", "wrong-turn", "invalid-kind", "conflicting-kind"} {
		t.Run(scenario, func(t *testing.T) {
			input, callID := forkToolKindPrefixFixtureV1(t, scenario)
			prefix, _, err := BuildForkEventPrefixV1(input)
			valid := scenario == "started" || scenario == "completed-without-start" || scenario == "ready-after-start"
			if !valid {
				if !errors.Is(err, ErrForkBoundaryUnavailable) {
					t.Fatalf("unproven tool kind admitted: err=%v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, raw := range listAny(listAny(prefix["turns"])[0].(map[string]any)["items"]) {
				item := raw.(map[string]any)
				if stringField(item, "kind") == "tool_call" && item["callId"] == callID {
					found = true
					if item["toolKind"] != "tool_call" {
						t.Fatal("committed kind was lost")
					}
					if scenario == "started" && (item["status"] != "running" || item["finishedAt"] != nil) {
						t.Fatal("unfinished call gained completion")
					}
					if scenario == "completed-without-start" && item["status"] != "completed" {
						t.Fatal("real completion was lost")
					}
				}
			}
			if !found {
				t.Fatal("committed call missing")
			}
		})
	}
}

func forkToolKindPrefixFixtureV1(t *testing.T, scenario string) (ForkEventPrefixInputV1, string) {
	t.Helper()
	source, _, _ := canonicalGeneralTerminalPublicFixture(t, "completed", "success")
	turn := listAny(source["turns"])[0].(map[string]any)
	threadID, turnID := stringField(source, "id"), stringField(turn, "id")
	start := turnapp.BuildStartRecord(turnapp.StartRecordInput{ThreadID: threadID, TurnID: turnID, Prompt: "Synthetic tool kind boundary", Model: "synthetic-model", CreatedAt: "2026-07-14T23:59:58Z"})
	callID, err := domainmodel.NewHostToolCallIDV1([]byte(strings.Repeat("a", domainmodel.HostToolCallIDEntropyBytesV1)))
	if err != nil {
		t.Fatal(err)
	}
	call := domainmodel.ToolCall{ID: callID, Name: "read"}
	itemID := domaintoolcall.ToolCallItemIDV1(turnID, callID)
	started, ok := toolcatalogapp.BuildToolStartedEvent(toolcatalogapp.ToolStartedEventInput{ThreadID: threadID, TurnID: turnID, ItemID: itemID, CallID: callID, ToolName: call.Name, CreatedAt: "2026-07-14T23:59:59Z"})
	if !ok {
		t.Fatal("real started producer unavailable")
	}
	started["timestamp"] = "2026-07-14T23:59:59Z"
	settled, err := toolcatalogapp.SettleToolResult(toolcatalogapp.ToolResultInput{ThreadID: threadID, TurnID: turnID, Call: call, CreatedAt: "2026-07-14T23:59:59Z", FinishedAt: "2026-07-14T23:59:59.1Z", Projection: domaintoolresult.WithheldProjectionV1("completed", "tool_output_private")})
	if err != nil {
		t.Fatal(err)
	}
	settled.Event["timestamp"] = "2026-07-14T23:59:59.1Z"
	ready := map[string]any{"kind": "tool_call_ready", "threadId": threadID, "turnId": turnID, "itemId": itemID, "callId": callID, "toolName": call.Name, "readyCount": 1, "timestamp": "2026-07-14T23:59:58.9Z"}
	currentCall := contracts.CloneMap(started["item"].(map[string]any))
	turn["prompt"], turn["createdAt"] = start.Turn["prompt"], start.Turn["createdAt"]
	turn["items"] = []any{start.UserItem, currentCall, listAny(turn["items"])[0]}
	pre := []map[string]any{start.TurnStartedEvent, start.UserItemCreatedEvent, ready}
	post := []map[string]any{}
	switch scenario {
	case "started":
		pre = append(pre, started)
	case "ready-after-start":
		pre = append(pre, started, contracts.CloneMap(ready))
	case "ready-only":
	case "start-after-E":
		post = append(post, started)
	default:
		turn["items"] = append(listAny(turn["items"]), settled.ResultItem)
		currentCall["status"], currentCall["finishedAt"] = "completed", settled.ResultItem["finishedAt"]
		if scenario == "completion-after-E" {
			post = append(post, settled.Event)
		} else {
			if scenario == "conflicting-kind" {
				started["item"].(map[string]any)["toolKind"] = "subagent"
				pre = append(pre, started)
			}
			switch scenario {
			case "wrong-name":
				settled.ResultItem["toolName"] = "unbound-tool"
			case "wrong-call":
				settled.ResultItem["callId"] = "unbound-call"
			case "wrong-thread":
				settled.Event["item"].(map[string]any)["threadId"] = "unbound-thread"
			case "wrong-turn":
				settled.Event["item"].(map[string]any)["turnId"] = "unbound-turn"
			case "invalid-kind":
				settled.ResultItem["toolKind"] = "unbound-kind"
			}
			pre = append(pre, settled.Event)
		}
	}
	commit, err := domainturnterminal.ParseGeneralTerminalPublicationCommitV1(turn["generalTerminalPublication"])
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := turnapp.PrepareGeneralTerminalEventBundleV1(source, turnID, commit, len(pre)+1)
	if err != nil {
		t.Fatal(err)
	}
	events := append(append(pre, terminal...), post...)
	for index, event := range events {
		event["seq"] = index + 1
		body, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		events[index], err = domainjsonstrict.DecodeObject(body, domainjsonstrict.Options{RequireObject: true, MaxBytes: len(body) + 1})
		if err != nil {
			t.Fatal(err)
		}
	}
	return ForkEventPrefixInputV1{Source: source, Events: events, Boundary: ForkBoundaryV1{TurnID: turnID, EndSeq: len(pre) + len(terminal)}, Projector: EnsurePublicProjector(nil)}, callID
}

func forkNativePromotedSteeringFixtureV1(t *testing.T, question bool) (map[string]any, map[string]map[string]any) {
	t.Helper()
	const threadID, turnID, admittedAt, promotedAt = "thread-native-history", "turn-native-history", "2026-07-14T23:59:59.4Z", "2026-07-14T23:59:59.6Z"
	digest := strings.Repeat("a", 64)
	authority := steeringauthorityapp.NewService(newProjectionTestAuthority(0x41))
	var pending map[string]any
	if question {
		pending, _ = signedForkDiscardEntryV1(t, turnID, digest, promotedAt)
	} else {
		clientID := "ordinary-client-1"
		var err error
		pending, err = domainsteering.BindPendingEntryV1(map[string]any{
			"id": domainsteering.EntryIDV1(turnID, clientID), "clientUserMessageId": clientID,
			"text": "Synthetic ordinary steering", "displayText": "Synthetic visible steering", "admittedAt": admittedAt,
		}, digest)
		if err != nil {
			t.Fatal(err)
		}
		pending, err = authority.Seal(context.Background(), pending, digest)
		if err != nil {
			t.Fatal(err)
		}
	}
	turn := map[string]any{"id": turnID, "threadId": threadID, "status": "completed", "createdAt": admittedAt, "finishedAt": promotedAt,
		"approvalPolicy": "on-request", "sandboxMode": "workspace-write",
		"steering": []any{pending}, "items": []any{}, "activeSkillIds": []any{}, "injectedMemoryIds": []any{}}
	expected, err := domainsteering.NewPendingEntryExpectationV1(pending)
	if err != nil {
		t.Fatal(err)
	}
	updated, entries, items, err := domainsteering.PromoteTurnEntryPrefixV1(threadID, turnID, turn, []domainsteering.PendingEntryExpectationV1{expected}, digest, promotedAt,
		func(entry map[string]any, contextDigest string) error {
			return authority.Verify(context.Background(), entry, contextDigest)
		},
		func(entry map[string]any, contextDigest string) (map[string]any, error) {
			return authority.Promote(context.Background(), entry, contextDigest)
		})
	if err != nil || len(entries) != 1 || len(items) != 1 {
		t.Fatal("real signed promotion fixture unavailable", err)
	}
	questions := map[string]map[string]any{}
	if question {
		questions[stringField(entries[0], "id")] = entries[0]
	}
	return map[string]any{"id": threadID, "status": "idle", "turns": []any{updated}}, questions
}

func TestForkPromotedSteeringPrimaryMatchesSeedV1(t *testing.T) {
	for _, question := range []bool{false, true} {
		t.Run(fmt.Sprintf("question=%t", question), func(t *testing.T) {
			source, questions := forkNativePromotedSteeringFixtureV1(t, question)
			before := contracts.CloneMap(source)
			fork, err := BuildFork(ForkInput{Source: source, ForkID: "thread-inert-child", ParentThreadID: stringField(source, "id"), Now: "2026-07-15T00:00:00Z"})
			if err != nil || NormalizeForkQuestionHistoryV1(fork, questions) != nil {
				t.Fatal("native promotion did not derive inert history", err)
			}
			events, err := BuildForkCreatedEventsV1(fork, stringField(source, "id"), "", 0)
			if err != nil {
				t.Fatal(err)
			}
			seed, _, err := ReadForkCreatedSeedV1(fork, events)
			if err != nil || !reflect.DeepEqual(fork["turns"], seed["turns"]) {
				t.Fatal("inert primary and closed seed differ", err)
			}
			if !reflect.DeepEqual(contracts.CloneMap(source), before) {
				t.Fatal("normalizing child changed signed source")
			}
			turn := listAny(fork["turns"])[0].(map[string]any)
			item := listAny(turn["items"])[0].(map[string]any)
			for _, key := range []string{"steeringProjectionVersion", "steeringContentDigest", "steeringOrigin", "contextDigest"} {
				if _, present := item[key]; present {
					t.Fatal("child retained native promotion material", key)
				}
			}
			refork, err := BuildFork(ForkInput{Source: fork, ForkID: "thread-inert-grandchild", ParentThreadID: stringField(fork, "id"), Now: "2026-07-15T00:00:01Z"})
			inheritedQuestions := map[string]map[string]any{}
			if question {
				entry := listAny(turn["steering"])[0].(map[string]any)
				inheritedQuestions[stringField(entry, "id")] = entry
			}
			if err != nil || NormalizeForkQuestionHistoryV1(refork, inheritedQuestions) != nil {
				t.Fatal("inert refork unavailable", err)
			}
		})
	}
}

func TestForkPromotedSteeringNormalizationRejectsUnboundOrMalformedMarkersV1(t *testing.T) {
	for _, scenario := range []string{"no-entry", "duplicate-entry", "wrong-client", "wrong-text", "wrong-display", "wrong-created", "wrong-finished", "wrong-digest", "partial", "version", "task-job", "invalid-digest", "padded-digest", "tool-marker"} {
		t.Run(scenario, func(t *testing.T) {
			source, questions := forkNativePromotedSteeringFixtureV1(t, true)
			fork, err := BuildFork(ForkInput{Source: source, ForkID: "thread-inert-negative", ParentThreadID: stringField(source, "id"), Now: "2026-07-15T00:00:00Z"})
			if err != nil {
				t.Fatal(err)
			}
			turn := listAny(fork["turns"])[0].(map[string]any)
			item := listAny(turn["items"])[0].(map[string]any)
			switch scenario {
			case "no-entry":
				turn["steering"] = []any{}
			case "duplicate-entry":
				turn["steering"] = append(listAny(turn["steering"]), contracts.CloneMap(listAny(turn["steering"])[0].(map[string]any)))
			case "wrong-client":
				item["clientUserMessageId"] = "other-client"
			case "wrong-text":
				item["text"] = "other text"
			case "wrong-display":
				item["displayText"] = "other display"
			case "wrong-created":
				item["createdAt"] = "2026-07-14T23:59:59.3Z"
			case "wrong-finished":
				item["finishedAt"] = "2026-07-14T23:59:59.7Z"
			case "wrong-digest":
				item["steeringContentDigest"] = strings.Repeat("b", 64)
			case "partial":
				delete(item, "steeringOrigin")
			case "version":
				item["steeringProjectionVersion"] = 2
			case "task-job":
				item["steeringOrigin"] = "task_job"
			case "invalid-digest":
				item["steeringContentDigest"] = "invalid"
			case "padded-digest":
				item["steeringContentDigest"] = " " + strings.Repeat("a", 64)
			case "tool-marker":
				item["kind"] = "tool_call"
			}
			if !errors.Is(NormalizeForkQuestionHistoryV1(fork, questions), ErrForkBoundaryUnavailable) {
				t.Fatal("unbound native material became inert history")
			}
		})
	}
}

func TestForkOrdinaryPromotionMalformedFieldsReturnUnavailableV1(t *testing.T) {
	for _, field := range []string{"text", "createdAt", "finishedAt", "displayText"} {
		t.Run(field, func(t *testing.T) {
			source, questions := forkNativePromotedSteeringFixtureV1(t, false)
			fork, err := BuildFork(ForkInput{Source: source, ForkID: "thread-malformed-history", ParentThreadID: stringField(source, "id"), Now: "2026-07-15T00:00:00Z"})
			if err != nil {
				t.Fatal(err)
			}
			turn := listAny(fork["turns"])[0].(map[string]any)
			item := listAny(turn["items"])[0].(map[string]any)
			entry := listAny(turn["steering"])[0].(map[string]any)
			entryField := field
			if field == "createdAt" {
				entryField = "admittedAt"
			}
			if field == "finishedAt" {
				entryField = "promotedAt"
			}
			item[field], entry[entryField] = []any{"malformed"}, []any{"malformed"}
			if !errors.Is(NormalizeForkQuestionHistoryV1(fork, questions), ErrForkBoundaryUnavailable) {
				t.Fatal("malformed producer fields did not return unavailable")
			}
		})
	}
}
