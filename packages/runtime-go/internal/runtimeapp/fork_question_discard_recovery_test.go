//go:build darwin || linux

package runtimeapp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	eventlog "analytix.local/runtime-go/internal/adapters/outbound/eventlog"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	displayapp "analytix.local/runtime-go/internal/app/userinputdisplay"
	"analytix.local/runtime-go/internal/contracts"
	port "analytix.local/runtime-go/internal/ports/userinputdisplay"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

// Real production composition, local HTTP Provider and Host timer create the
// signed discard. Both reopenings use the same production factory and Registry
// without reseeding, directly covering the startup inventory rejection. This
// is a new isolated fixture, not migration of an existing bad product profile.
func TestForkDiscardHistoryProductionRecoveryAndReforkV1(t *testing.T) {
	forkQuestionHistoryProductionV1(t, false, false, false, false)
}
func TestForkAcceptedHistoryProductionRecoveryAndReforkV1(t *testing.T) {
	forkQuestionHistoryProductionV1(t, true, false, false, false)
}
func TestForkAcceptedAfterBoundaryHistoryRemainsUnavailableV1(t *testing.T) {
	forkQuestionHistoryProductionV1(t, false, true, false, false)
}
func TestForkAcceptedWholeThreadHistoryProductionRecoveryAndReforkV1(t *testing.T) {
	forkQuestionHistoryProductionV1(t, true, false, true, false)
}
func TestForkOrdinarySteeringWholeThreadProductionRecoveryAndReforkV1(t *testing.T) {
	forkQuestionHistoryProductionV1(t, true, false, true, true)
}
func forkQuestionHistoryProductionV1(t *testing.T, acceptedHistory, acceptedAfterE, wholeThreadFork, ordinarySteering bool) {
	var providerCalls atomic.Int32
	continued, release := make(chan struct{}, 1), make(chan struct{})
	var releaseOnce sync.Once
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		call := providerCalls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		if call == 1 {
			args := `{"prompt":"Synthetic optional question","questions":[{"question":"Choose a synthetic answer?"}],"timeout":1}`
			chunk := map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "synthetic-fork-discard-call", "type": "function", "function": map[string]any{"name": "request_user_input", "arguments": args}}}}, "finish_reason": "tool_calls"}}}
			encoded, _ := json.Marshal(chunk)
			_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", encoded)
			return
		}
		if call == 2 {
			continued <- struct{}{}
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		if acceptedHistory && call == 2 {
			args := `{"path":"accepted-synthetic.txt"}`
			chunk := map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "synthetic-accepted-read", "type": "function", "function": map[string]any{"name": "read", "arguments": args}}}}, "finish_reason": "tool_calls"}}}
			encoded, _ := json.Marshal(chunk)
			_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", encoded)
			return
		}
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Synthetic independent work completed.\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":8,\"completion_tokens\":3,\"total_tokens\":11}}\n\ndata: [DONE]\n\n")
	}))
	defer provider.Close()
	_, config := runtimeWitnessedRegistryConfigV2(t)
	config.ProductionDurableRoot = runtimeWitnessedRegistryHostTempV2(t, "fork-discard-history")
	config.BaseURL = provider.URL + "/v1"
	config.MCPConfigJSON = `{"runtime":{"userInput":{"policy":"timed","defaultTimeoutSeconds":1}}}`
	seedProviderRegistryExecutionAuthorityV1(t, config.DataDir, config.ProviderID, config.BaseURL, []string{config.Model}, config.Model, "test-only")
	var handler http.Handler
	open := func() {
		t.Helper()
		var err error
		handler, err = NewRuntimeServerHandlerE(config)
		if err != nil {
			// Emit only closed diagnostic labels, never runtime error bodies.
			reason := "unclassified"
			for _, known := range []string{
				"fork history creation seed is invalid",
				"fork history inherited turn inventory differs from seed",
				"fork history inherited turn metadata differs from seed",
				"fork history inherited item differs from seed",
				"fork history missing item is not a completed inert tool",
				"fork history inherited item order differs from seed",
				"closed primary projection is invalid",
				"durable thread history failed closed sanitation",
			} {
				if strings.Contains(err.Error(), known) {
					reason = known
					break
				}
			}
			t.Fatalf("production_recovery_failed errorType=%T code=%s reason=%s", err, RuntimeStartupPreparationFailureCodeV1(err), reason)
		}
	}
	open()
	defer func() {
		releaseOnce.Do(func() { close(release) })
		if handler != nil {
			shutdownOwnedRuntimeHandler(t, handler)
		}
	}()
	request := func(method, path string, body any, private bool) (int, []byte) {
		t.Helper()
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal("fixture_request_invalid")
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(encoded))
		req.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
		req.Header.Set("Content-Type", "application/json")
		if private {
			req.Header.Set(httpapi.LocalDisplayHeaderV1, httpapi.LocalDisplayHeaderValueV1)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code, rec.Body.Bytes()
	}
	jsonRequest := func(method, path string, body any, private bool, expected int) map[string]any {
		t.Helper()
		status, encoded := request(method, path, body, private)
		var result map[string]any
		if json.Unmarshal(encoded, &result) != nil {
			t.Fatal("fixture_response_invalid")
		}
		if status != expected {
			t.Fatalf("fixture_response_status=%d expected=%d code=%s", status, expected, contracts.StringField(result, "code"))
		}
		return result
	}
	wait := func(condition func() bool) {
		t.Helper()
		deadline, tick := time.NewTimer(20*time.Second), time.NewTicker(20*time.Millisecond)
		defer deadline.Stop()
		defer tick.Stop()
		for {
			if condition() {
				return
			}
			select {
			case <-deadline.C:
				t.Fatal("production_history_observation_timeout")
			case <-tick.C:
			}
		}
	}
	workspace := workspacetest.New(t)
	if acceptedHistory {
		if err := os.WriteFile(filepath.Join(workspace, "accepted-synthetic.txt"), []byte("Synthetic independent input"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	created := jsonRequest(http.MethodPost, "/v1/threads", map[string]any{"workspace": workspace, "providerId": config.ProviderID, "model": config.Model}, false, http.StatusCreated)
	threadID := contracts.StringField(created, "id")
	started := jsonRequest(http.MethodPost, "/v1/threads/"+threadID+"/turns", map[string]any{"prompt": "Ask one optional question, then finish independent ordinary work.", "async": true, "providerId": config.ProviderID, "model": config.Model, "approvalPolicy": "never", "sandboxMode": "read-only"}, false, http.StatusAccepted)
	turnID := contracts.StringField(started, "turnId")
	select {
	case <-continued:
	case <-time.After(20 * time.Second):
		t.Fatal("real_host_timer_did_not_continue")
	}
	var selector port.SelectorV1
	public := jsonRequest(http.MethodGet, "/v1/threads/"+threadID, nil, false, http.StatusOK)
	for _, rawTurn := range public["turns"].([]any) {
		turn := rawTurn.(map[string]any)
		for _, rawItem := range turn["items"].([]any) {
			item := rawItem.(map[string]any)
			if item["kind"] == "user_input" && item["turnId"] == turnID {
				selector = port.SelectorV1{ThreadID: threadID, TurnID: turnID, ItemID: contracts.StringField(item, "id"), RequestID: contracts.StringField(item, "inputId")}
			}
		}
	}
	if !port.ValidateSelectorV1(selector) {
		t.Fatal("actual_question_selector_unavailable")
	}
	readDisplay := func(selected port.SelectorV1) displayapp.DisplayV1 {
		t.Helper()
		status, body := request(http.MethodPost, httpapi.UserInputLocalDisplayPathV1, selected, true)
		var display displayapp.DisplayV1
		decoded := json.Unmarshal(body, &display) == nil
		valid := displayapp.ValidateDisplayV1(display, selected)
		if status != http.StatusOK || !decoded || !valid {
			var diagnostic map[string]any
			_ = json.Unmarshal(body, &diagnostic)
			sourceStatus := 0
			if selected.ThreadID != selector.ThreadID {
				sourceStatus, _ = request(http.MethodPost, httpapi.UserInputLocalDisplayPathV1, selector, true)
			}
			t.Fatalf("actual_private_question_display_unavailable status=%d code=%s decoded=%t valid=%t sourceStatus=%d", status, contracts.StringField(diagnostic, "code"), decoded, valid, sourceStatus)
		}
		return display
	}
	view := readDisplay(selector)
	if view.Status != "continued" || view.ReadOnly || len(view.Questions) != 1 || providerCalls.Load() != 2 {
		t.Fatal("actual_question_did_not_continue")
	}
	attempt := "00000000-0000-4000-8000-0000000000d1"
	lateBody := func(selected port.SelectorV1) map[string]any {
		return map[string]any{"selector": selected, "answers": []any{map[string]any{"id": view.Questions[0].ID, "kind": "answer", "optionIds": []string{}, "customText": "Synthetic discarded answer"}}, "lateReply": map[string]any{"clientUserMessageId": attempt, "expectedTurnId": turnID}}
	}
	queued := jsonRequest(http.MethodPost, "/v1/user-inputs/"+selector.RequestID, lateBody(selector), false, http.StatusOK)
	if queued["status"] != "continued" || queued["lateReply"].(map[string]any)["state"] != "queued" {
		t.Fatal("normal_late_answer_was_not_queued")
	}
	discardRequest := displayapp.DiscardLateReplyRequestV1{SelectorV1: selector, Action: "discardLateReply", ClientUserMessageID: attempt, ExpectedTurnID: turnID}
	for repeat := 0; repeat < 2; repeat++ {
		status, body := request(http.MethodPost, httpapi.UserInputLocalDisplayPathV1, discardRequest, true)
		var discarded displayapp.DiscardLateReplyResponseV1
		if status != http.StatusOK || json.Unmarshal(body, &discarded) != nil || !displayapp.ValidateDiscardLateReplyResponseV1(discarded, discardRequest) {
			t.Fatal("normal_discard_or_idempotence_failed")
		}
	}
	replyAttempt := "00000000-0000-4000-8000-0000000000d2"
	if acceptedHistory {
		body := lateBody(selector)
		body["lateReply"] = map[string]any{"clientUserMessageId": replyAttempt, "expectedTurnId": turnID}
		body["answers"] = []any{map[string]any{"id": view.Questions[0].ID, "kind": "answer", "optionIds": []string{}, "customText": "Synthetic accepted original answer"}}
		result := jsonRequest(http.MethodPost, "/v1/user-inputs/"+selector.RequestID, body, false, http.StatusOK)
		if result["lateReply"].(map[string]any)["state"] != "queued" {
			t.Fatal("second attempt was not queued before delivery")
		}
	}
	const ordinaryClientID, ordinaryText = "00000000-0000-4000-8000-0000000000d3", "Keep the final synthetic response brief."
	if ordinarySteering {
		admitted := jsonRequest(http.MethodPost, "/v1/threads/"+threadID+"/turns/"+turnID+"/steer", map[string]any{
			"text": ordinaryText, "clientUserMessageId": ordinaryClientID, "expectedTurnId": turnID,
		}, false, http.StatusOK)
		seq, ok := contracts.NumericSeq(admitted["admittedSeq"])
		if admitted["ok"] != true || admitted["clientUserMessageId"] != ordinaryClientID || admitted["threadId"] != threadID || admitted["turnId"] != turnID || !ok || seq <= 0 {
			t.Fatal("normal_ordinary_steering_identity_not_admitted")
		}
		current := jsonRequest(http.MethodGet, "/v1/threads/"+threadID, nil, false, http.StatusOK)
		pending := false
		for _, raw := range current["turns"].([]any)[0].(map[string]any)["steering"].([]any) {
			entry := raw.(map[string]any)
			if entry["clientUserMessageId"] == ordinaryClientID {
				pending = entry["status"] == "pending" && entry["promotedItemId"] == nil
			}
		}
		if !pending {
			t.Fatal("ordinary_admission_was_not_pending_before_provider_boundary")
		}
	}
	releaseOnce.Do(func() { close(release) })
	wait(func() bool {
		status, body := request(http.MethodGet, "/v1/threads/"+threadID, nil, false)
		var thread map[string]any
		if status != http.StatusOK || json.Unmarshal(body, &thread) != nil {
			return false
		}
		turns, _ := thread["turns"].([]any)
		return len(turns) == 1 && turns[0].(map[string]any)["status"] == "completed" && thread["status"] == "idle"
	})
	status, sse := request(http.MethodGet, "/v1/threads/"+threadID+"/events?since_seq=0", nil, false)
	if status != http.StatusOK {
		t.Fatal("finite_public_history_unavailable")
	}
	endSeq := 0
	frozenSourceEvents := []map[string]any{}
	var findTerminal func(map[string]any)
	findTerminal = func(event map[string]any) {
		frozenSourceEvents = append(frozenSourceEvents, contracts.CloneMap(event))
		if event["kind"] == "turn_completed" && event["turnId"] == turnID {
			endSeq, _ = contracts.NumericSeq(event["seq"])
		}
		if events, ok := event["events"].([]any); ok {
			for _, raw := range events {
				if child, ok := raw.(map[string]any); ok {
					findTerminal(child)
				}
			}
		}
	}
	for _, line := range bytes.Split(sse, []byte("\n")) {
		if !bytes.HasPrefix(line, []byte("data: ")) {
			continue
		}
		var event map[string]any
		if json.Unmarshal(bytes.TrimPrefix(line, []byte("data: ")), &event) == nil {
			findTerminal(event)
		}
	}
	if endSeq == 0 {
		t.Fatal("real_committed_terminal_boundary_unavailable")
	}
	primaryPath := func(id string) string {
		return filepath.Join(config.ProductionDurableRoot, "threads", id, "thread.json")
	}
	sourceBefore, err := os.ReadFile(primaryPath(threadID))
	if err != nil {
		t.Fatal("synthetic_source_primary_unavailable")
	}
	var sourceRecord map[string]any
	sourceDecoder := json.NewDecoder(bytes.NewReader(sourceBefore))
	sourceDecoder.UseNumber()
	if sourceDecoder.Decode(&sourceRecord) != nil {
		t.Fatal("synthetic_source_primary_invalid")
	}
	sourceDiscard := sourceRecord["turns"].([]any)[0].(map[string]any)["steering"].([]any)[0].(map[string]any)
	if ordinarySteering {
		found := false
		for _, raw := range sourceRecord["turns"].([]any)[0].(map[string]any)["steering"].([]any) {
			entry := raw.(map[string]any)
			if entry["clientUserMessageId"] == ordinaryClientID {
				found = entry["status"] == "promoted" && entry["text"] == ordinaryText && entry["userInputId"] == nil && entry["kind"] == nil &&
					entry["authoritySignature"] != nil && entry["promotionAuthoritySignature"] != nil && entry["logicalEffect"] != nil && entry["ordinaryWork"] != nil
			}
		}
		if !found {
			t.Fatal("normal_ordinary_steering_not_signed_and_promoted")
		}
	}
	if sourceDiscard["cancelReason"] != "user_input_discarded" || sourceDiscard["cancellationAuthoritySignature"] == nil {
		t.Fatal("source_lacks_real_native_discard_proof")
	}
	sourcePublic := jsonRequest(http.MethodGet, "/v1/threads/"+threadID, nil, false, http.StatusOK)
	sourcePublicItems := []any{}
	for _, raw := range sourcePublic["turns"].([]any)[0].(map[string]any)["items"].([]any) {
		// Current terminal publication and grant transitions have no inert
		// fork carrier. Compare every remaining item to the actual source.
		switch contracts.StringField(raw.(map[string]any), "kind") {
		case "assistant_text", "error", "execution_grant_transition":
			continue
		}
		sourcePublicItems = append(sourcePublicItems, raw)
	}
	// E preserves committed producer clocks, sampled separately from the
	// primary's ready/completion patches. Derive both calls independently
	// from the frozen public log, never from the seed under examination.
	eventCallTimes := map[string]map[string]any{}
	eventQuestionTimes := map[string]map[string]any{}
	if acceptedHistory && !wholeThreadFork {
		for _, raw := range sourcePublicItems {
			call := raw.(map[string]any)
			if call["kind"] != "tool_call" {
				continue
			}
			times := map[string]any{}
			for _, event := range frozenSourceEvents {
				seq, ok := contracts.NumericSeq(event["seq"])
				if !ok || seq > endSeq || event["threadId"] != threadID || event["turnId"] != turnID {
					continue
				}
				item, _ := event["item"].(map[string]any)
				switch event["kind"] {
				case "tool_call_ready":
					if event["itemId"] == call["id"] && event["callId"] == call["callId"] && event["toolName"] == call["toolName"] {
						times["createdAt"] = event["timestamp"]
					}
				case "tool_call_started":
					if item != nil && item["kind"] == "tool_call" && item["id"] == call["id"] && item["callId"] == call["callId"] && item["toolName"] == call["toolName"] && item["threadId"] == threadID && item["turnId"] == turnID {
						times["createdAt"] = item["createdAt"]
					}
				case "tool_call_finished":
					if item != nil && item["kind"] == "tool_result" && item["callId"] == call["callId"] && item["toolName"] == call["toolName"] && item["threadId"] == threadID && item["turnId"] == turnID && item["status"] == "completed" {
						times["finishedAt"] = item["finishedAt"]
					}
				}
			}
			for _, field := range []string{"createdAt", "finishedAt"} {
				if _, err := time.Parse(time.RFC3339Nano, contracts.StringField(times, field)); err != nil {
					t.Fatalf("committed_tool_clock_unavailable field=%s", field)
				}
			}
			eventCallTimes[contracts.StringField(call, "id")] = times
		}
		if len(eventCallTimes) != 2 {
			t.Fatal("committed_tool_clock_inventory_incomplete")
		}
		// Flat Question events likewise record a publication clock rather
		// than the receipt's issuedAt or the primary's later status patch.
		for _, raw := range sourcePublicItems {
			question := raw.(map[string]any)
			if question["kind"] != "user_input" {
				continue
			}
			times, requested, resolved := map[string]any{}, 0, 0
			for _, event := range frozenSourceEvents {
				seq, ok := contracts.NumericSeq(event["seq"])
				if !ok || seq > endSeq || event["threadId"] != threadID || event["turnId"] != turnID || event["itemId"] != question["id"] || event["inputId"] != question["inputId"] {
					continue
				}
				if event["kind"] == "user_input_requested" && event["status"] == "pending" {
					requested++
					times["createdAt"] = event["timestamp"]
				}
				if event["kind"] == "user_input_resolved" && event["status"] == question["status"] {
					resolved++
					times["finishedAt"] = event["timestamp"]
				}
			}
			if requested != 1 || resolved != 1 {
				t.Fatal("committed_question_clock_producers_incomplete")
			}
			for _, field := range []string{"createdAt", "finishedAt"} {
				if _, err := time.Parse(time.RFC3339Nano, contracts.StringField(times, field)); err != nil {
					t.Fatalf("committed_question_clock_unavailable field=%s", field)
				}
			}
			eventQuestionTimes[contracts.StringField(question, "id")] = times
		}
		if len(eventQuestionTimes) != 1 {
			t.Fatal("committed_question_clock_inventory_incomplete")
		}
	}
	forkBody := map[string]any{"turnId": turnID, "endSeq": endSeq}
	if wholeThreadFork {
		forkBody = map[string]any{}
	}
	fork := jsonRequest(http.MethodPost, "/v1/threads/"+threadID+"/fork", forkBody, false, http.StatusCreated)
	childID := contracts.StringField(fork, "id")
	assertPublicHistory := func(id string) {
		t.Helper()
		before, err := os.ReadFile(primaryPath(id))
		if err != nil {
			t.Fatal("synthetic_child_primary_unavailable")
		}
		thread := jsonRequest(http.MethodGet, "/v1/threads/"+id, nil, false, http.StatusOK)
		turns := thread["turns"].([]any)
		if len(turns) != 1 {
			t.Fatal("fork_changed_turn_history")
		}
		turn := turns[0].(map[string]any)
		entries := turn["steering"].([]any)
		wantEntries := 1
		if acceptedHistory {
			wantEntries = 2
		}
		if ordinarySteering {
			wantEntries++
		}
		if len(entries) != wantEntries {
			t.Fatal("fork_lost_fixed_attempts")
		}
		if ordinarySteering {
			entry := entries[len(entries)-1].(map[string]any)
			if entry["clientUserMessageId"] != ordinaryClientID || entry["text"] != ordinaryText || entry["status"] != "promoted" || entry["kind"] != nil || entry["delivery"] != nil {
				t.Fatal("fork_lost_ordinary_steering_history")
			}
		}
		entry := entries[0].(map[string]any)
		if entry["status"] != "cancelled" || entry["cancelReason"] != "user_input_discarded" || entry["cancelledAt"] == nil || entry["historyCancellationReason"] != nil {
			t.Fatalf("public_discard_fact_invalid cancelled=%t discarded=%t completedReason=%t originalTime=%t namespaceAbsent=%t", entry["status"] == "cancelled", entry["cancelReason"] == "user_input_discarded", entry["cancelReason"] == "completed", reflect.DeepEqual(entry["cancelledAt"], sourceDiscard["cancelledAt"]), entry["historyCancellationReason"] == nil)
		}
		for _, key := range []string{"id", "clientUserMessageId", "text", "displayText", "admittedAt", "cancelledAt", "status"} {
			if !reflect.DeepEqual(entry[key], sourceDiscard[key]) {
				t.Fatalf("fork_lost_original_discard_history_fact field=%s sourcePresent=%t forkPresent=%t", key, sourceDiscard[key] != nil, entry[key] != nil)
			}
		}
		found := false
		for _, raw := range turn["items"].([]any) {
			item := raw.(map[string]any)
			if item["id"] == selector.ItemID {
				found = item["status"] == "continued"
			}
			if acceptedAfterE && item["clientUserMessageId"] == replyAttempt {
				t.Fatal("after_E_reply_entered_old_prefix")
			}
			if item["kind"] == "user_message" && item["clientUserMessageId"] == attempt {
				t.Fatal("discard_manufactured_an_accepted_reply")
			}
		}
		if !found {
			t.Fatal("discard_changed_the_original_question_status")
		}
		selected := selector
		selected.ThreadID = id
		history := readDisplay(selected)
		if acceptedHistory {
			if history.Submission == nil || history.HistoryReply == nil || history.AnswerState != "available" || history.HistoryReply.ClientUserMessageID != replyAttempt || history.HistoryReply.TurnID != turnID {
				t.Fatal("accepted_prefix_history_not_restored")
			}
			foundReply := false
			for _, rawItem := range turn["items"].([]any) {
				item := rawItem.(map[string]any)
				if item["id"] == history.HistoryReply.ItemID && item["clientUserMessageId"] == replyAttempt && item["status"] == "completed" {
					foundReply = true
				}
			}
			encoded, _ := json.Marshal(history.Submission)
			if !foundReply || !bytes.Contains(encoded, []byte("Synthetic accepted original answer")) {
				t.Fatal("history_answer_not_bound_to_real_completed_item")
			}
		} else if history.Submission != nil || history.HistoryReply != nil {
			t.Fatal("discard_restored_an_answer")
		}
		if !history.ReadOnly || history.Status != "continued" || history.LateReply != nil || history.Draft != nil || history.TimedWait != nil {
			t.Fatal("fork_inherited_private_answer_state_or_execution")
		}
		status, _ := request(http.MethodPost, "/v1/user-inputs/"+selector.RequestID, lateBody(selected), false)
		if status == http.StatusOK || status == http.StatusAccepted {
			t.Fatal("fork_acquired_source_question_submission_authority")
		}
		after, err := os.ReadFile(primaryPath(id))
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("public_or_private_history_read_mutated_primary")
		}
	}
	assertPublicHistory(childID)
	sourceAfter, err := os.ReadFile(primaryPath(threadID))
	if err != nil || !bytes.Equal(sourceBefore, sourceAfter) {
		t.Fatal("fork_changed_original_signed_primary")
	}
	afterEChildID := ""
	if acceptedAfterE {
		// Durable idle can precede the Host releasing the completed turn's tail.
		// Use the production idle fence before making the single late-reply attempt.
		func() {
			lease := ""
			release := func() {
				heldLease := lease
				lease = ""
				released := jsonRequest(http.MethodPost, "/v1/runtime/quiescence", map[string]any{"operation": "release", "lease": heldLease}, false, http.StatusOK)
				if released["released"] != true {
					t.Fatal("fixture_idle_fence_release_invalid")
				}
			}
			defer func() {
				if lease != "" {
					release()
				}
			}()
			wait(func() bool {
				status, body := request(http.MethodPost, "/v1/runtime/quiescence", map[string]any{"operation": "prepare"}, false)
				var prepared map[string]any
				if json.Unmarshal(body, &prepared) != nil || prepared == nil {
					t.Fatal("fixture_idle_fence_response_invalid")
				}
				if status == http.StatusConflict && contracts.StringField(prepared, "code") == "conflict" {
					return false
				}
				if status != http.StatusOK {
					t.Fatalf("fixture_idle_fence_status=%d code=%s", status, contracts.StringField(prepared, "code"))
				}
				if preparedLease := contracts.StringField(prepared, "lease"); len(preparedLease) == 43 {
					lease = preparedLease
				}
				if lease == "" || prepared["state"] != "idle" {
					t.Fatal("fixture_idle_fence_not_ready")
				}
				return true
			})
			release()
		}()
		sourceAfter, err := os.ReadFile(primaryPath(threadID))
		if err != nil || !bytes.Equal(sourceBefore, sourceAfter) {
			t.Fatal("idle_fence_changed_original_signed_primary")
		}
		late := lateBody(selector)
		late["lateReply"] = map[string]any{"clientUserMessageId": replyAttempt, "expectedTurnId": turnID}
		late["answers"] = []any{map[string]any{"id": view.Questions[0].ID, "kind": "answer", "optionIds": []string{}, "customText": "Synthetic answer accepted after old E"}}
		jsonRequest(http.MethodPost, "/v1/user-inputs/"+selector.RequestID, late, false, http.StatusOK)
		wait(func() bool {
			status, body := request(http.MethodPost, httpapi.UserInputLocalDisplayPathV1, selector, true)
			var actual displayapp.DisplayV1
			if status != http.StatusOK || json.Unmarshal(body, &actual) != nil || actual.LateReply == nil {
				return false
			}
			return actual.LateReply.State == "accepted" && actual.LateReply.ClientUserMessageID == replyAttempt && actual.LateReply.TurnID != turnID
		})
		wait(func() bool {
			status, encoded := request(http.MethodGet, "/v1/threads/"+threadID, nil, false)
			var record map[string]any
			if json.Unmarshal(encoded, &record) != nil || record == nil {
				t.Fatal("fixture_response_invalid")
			}
			if status == http.StatusServiceUnavailable && contracts.StringField(record, "code") == "public_projection_pending" {
				return false
			}
			if status != http.StatusOK {
				t.Fatalf("fixture_response_status=%d expected=%d code=%s", status, http.StatusOK, contracts.StringField(record, "code"))
			}
			return record["status"] == "idle"
		})
		_, afterEvents := request(http.MethodGet, "/v1/threads/"+threadID+"/events?since_seq=0", nil, false)
		acceptedSeq, acceptedCount := 0, 0
		var findAccepted func(map[string]any)
		findAccepted = func(event map[string]any) {
			item, _ := event["item"].(map[string]any)
			if event["kind"] == "item_created" && item != nil && item["clientUserMessageId"] == replyAttempt && item["kind"] == "user_message" && item["delivery"] == "steer" && item["status"] == "completed" {
				acceptedCount++
				acceptedSeq, _ = contracts.NumericSeq(event["seq"])
			}
			for _, raw := range listRuntimeHistoryEventsV1(event["events"]) {
				if child, ok := raw.(map[string]any); ok {
					findAccepted(child)
				}
			}
		}
		for _, line := range bytes.Split(afterEvents, []byte("\n")) {
			if bytes.HasPrefix(line, []byte("data: ")) {
				var event map[string]any
				if json.Unmarshal(bytes.TrimPrefix(line, []byte("data: ")), &event) == nil {
					findAccepted(event)
				}
			}
		}
		if acceptedCount != 1 || acceptedSeq <= endSeq {
			t.Fatal("actual_later_promotion_not_after_fixed_E")
		}
		assertPublicHistory(childID)
		acceptedBefore, err := os.ReadFile(primaryPath(threadID))
		if err != nil {
			t.Fatal(err)
		}
		afterFork := jsonRequest(http.MethodPost, "/v1/threads/"+threadID+"/fork", map[string]any{"turnId": turnID, "endSeq": endSeq}, false, http.StatusCreated)
		afterEChildID = contracts.StringField(afterFork, "id")
		assertPublicHistory(afterEChildID)
		acceptedAfter, err := os.ReadFile(primaryPath(threadID))
		if err != nil || !bytes.Equal(acceptedBefore, acceptedAfter) {
			t.Fatal("old_boundary_fork_modified_new_accepted_source")
		}
	}
	shutdownOwnedRuntimeHandler(t, handler)
	handler = nil
	assertPersisted := func(id string) {
		t.Helper()
		body, err := os.ReadFile(primaryPath(id))
		if err != nil {
			t.Fatal("synthetic_persisted_primary_unavailable")
		}
		var primary map[string]any
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.UseNumber()
		if decoder.Decode(&primary) != nil {
			t.Fatal("synthetic_persisted_primary_invalid")
		}
		data, err := os.ReadFile(filepath.Join(config.ProductionDurableRoot, "threads", id, "events.jsonl"))
		if err != nil {
			t.Fatal("synthetic_creation_log_unavailable")
		}
		events := []map[string]any{}
		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Buffer(make([]byte, 4096), 1<<20)
		for scanner.Scan() {
			var event map[string]any
			decoder := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
			decoder.UseNumber()
			if decoder.Decode(&event) != nil {
				t.Fatal("synthetic_creation_event_invalid")
			}
			events = append(events, event)
		}
		if scanner.Err() != nil {
			t.Fatal("synthetic_creation_log_invalid")
		}
		observed, frontier, observationErr := eventlog.NewStore(config.ProductionDurableRoot).ObserveSinceWithFrontier(context.Background(), id, 0)
		observedSeed, _, observedSeedErr := threadapp.ReadForkCreatedSeedV1(primary, observed.Events)
		unsettled := false
		leaves, leafErr := os.ReadDir(filepath.Dir(primaryPath(id)))
		if leafErr != nil {
			t.Fatal("synthetic_thread_inventory_unavailable")
		}
		for _, leaf := range leaves {
			unsettled = unsettled || strings.HasPrefix(leaf.Name(), ".events-bundle-")
		}
		t.Logf("history_event_observe ok=%t errorType=%T diagnostics=%d rawEvents=%d observedEvents=%d frontierExists=%t observedSeed=%t unsettledAtomic=%t", observationErr == nil, observationErr, len(observed.Diagnostics), len(events), len(observed.Events), frontier.Exists, observedSeedErr == nil && observedSeed != nil, unsettled)
		seed, closed, err := threadapp.ReadForkCreatedSeedV1(primary, events)
		if err != nil || closed <= 0 {
			t.Fatal("complete_creation_seed_unavailable")
		}
		if acceptedAfterE && (id == childID || id == afterEChildID) {
			cutoff, ok := contracts.NumericSeq(seed["sourceEndSeq"])
			if !ok || cutoff != endSeq || seed["sourceTurnId"] != turnID {
				t.Fatal("old_seed_boundary_changed_after_later_acceptance")
			}
		}
		if wholeThreadFork {
			_, hasTurn := seed["sourceTurnId"]
			_, hasEnd := seed["sourceEndSeq"]
			if hasTurn || hasEnd {
				t.Fatal("whole_thread_seed_invented_explicit_boundary")
			}
		}
		if primary["turns"].([]any)[0].(map[string]any)["status"] != "completed" {
			t.Fatal("inherited_turn_not_terminal")
		}
		primaryItems := primary["turns"].([]any)[0].(map[string]any)["items"].([]any)
		seedItems := seed["turns"].([]any)[0].(map[string]any)["items"].([]any)
		if acceptedHistory {
			if len(seedItems) != len(sourcePublicItems) {
				t.Fatalf("real_producer_omitted_source_history seedItems=%d sourceItems=%d", len(seedItems), len(sourcePublicItems))
			}
			for index, raw := range sourcePublicItems {
				expected := contracts.CloneMap(raw.(map[string]any))
				expected["threadId"] = id
				if expected["kind"] == "tool_call" && !wholeThreadFork {
					times := eventCallTimes[contracts.StringField(expected, "id")]
					expected["createdAt"], expected["finishedAt"] = times["createdAt"], times["finishedAt"]
				}
				if expected["kind"] == "user_input" && !wholeThreadFork {
					times := eventQuestionTimes[contracts.StringField(expected, "id")]
					expected["createdAt"], expected["finishedAt"] = times["createdAt"], times["finishedAt"]
				}
				// Source continuation authority never belongs to inherited history.
				delete(expected, "continuationReceiptId")
				a, _ := json.Marshal(expected)
				b, _ := json.Marshal(seedItems[index])
				if !bytes.Equal(a, b) {
					actual := seedItems[index].(map[string]any)
					keys, differences := map[string]bool{}, []string{}
					for key := range expected {
						keys[key] = true
					}
					for key := range actual {
						keys[key] = true
					}
					for key := range keys {
						left, _ := json.Marshal(expected[key])
						right, _ := json.Marshal(actual[key])
						if !bytes.Equal(left, right) {
							differences = append(differences, key)
						}
					}
					sort.Strings(differences)
					t.Logf("source_history_difference index=%d kind=%s keys=%v sourceStatus=%s seedStatus=%s sourceFinished=%s seedFinished=%s", index, contracts.StringField(expected, "kind"), differences, contracts.StringField(expected, "status"), contracts.StringField(actual, "status"), contracts.StringField(expected, "finishedAt"), contracts.StringField(actual, "finishedAt"))
					t.Fatalf("real_producer_changed_source_history index=%d kind=%s", index, contracts.StringField(expected, "kind"))
				}
			}
		}
		primaryMetadata := contracts.CloneMap(primary["turns"].([]any)[0].(map[string]any))
		seedMetadata := contracts.CloneMap(seed["turns"].([]any)[0].(map[string]any))
		delete(primaryMetadata, "items")
		delete(seedMetadata, "items")
		primaryMetadataJSON, _ := json.Marshal(primaryMetadata)
		seedMetadataJSON, _ := json.Marshal(seedMetadata)
		if !bytes.Equal(primaryMetadataJSON, seedMetadataJSON) {
			t.Fatal("complete_fork_turn_metadata_changed")
		}
		primaryJSON, _ := json.Marshal(primaryItems)
		seedItemsJSON, _ := json.Marshal(seedItems)
		if !bytes.Equal(primaryJSON, seedItemsJSON) {
			for index := 0; index < len(primaryItems) && index < len(seedItems); index++ {
				a, b := primaryItems[index].(map[string]any), seedItems[index].(map[string]any)
				keys := map[string]bool{}
				for key := range a {
					keys[key] = true
				}
				for key := range b {
					keys[key] = true
				}
				differences := []string{}
				for key := range keys {
					aj, _ := json.Marshal(a[key])
					bj, _ := json.Marshal(b[key])
					if !bytes.Equal(aj, bj) {
						differences = append(differences, key)
					}
				}
				sort.Strings(differences)
				if len(differences) > 0 {
					t.Logf("history_item_difference index=%d kind=%s keys=%v", index, contracts.StringField(a, "kind"), differences)
				}
			}
			t.Fatalf("complete_fork_history_changed wholeThread=%t primaryItems=%d seedItems=%d", wholeThreadFork, len(primaryItems), len(seedItems))
		}
		if acceptedHistory {
			calls, results := 0, 0
			for _, rawItem := range primaryItems {
				item := rawItem.(map[string]any)
				if item["kind"] == "tool_call" || item["kind"] == "tool_result" {
					if item["status"] != "completed" || item["executionGrant"] != nil || item["executionGrantId"] != nil || item["toolKind"] == nil {
						t.Fatal("inherited_tool_history_not_complete_and_inert")
					}
				}
				if item["kind"] == "tool_call" {
					calls++
				}
				if item["kind"] == "tool_result" {
					results++
				}
			}
			wantItems := 7
			if ordinarySteering {
				wantItems++
			}
			if len(primaryItems) != wantItems || calls != 2 || results != 2 {
				t.Fatalf("real_accepted_fork_shape_changed items=%d calls=%d results=%d", len(primaryItems), calls, results)
			}
		}
		for _, record := range []map[string]any{primary, seed} {
			turn := record["turns"].([]any)[0].(map[string]any)
			if ordinarySteering {
				for _, raw := range turn["steering"].([]any) {
					entry := raw.(map[string]any)
					if entry["clientUserMessageId"] == ordinaryClientID {
						for _, field := range []string{"delivery", "projectionVersion", "contentDigest", "contextDigest", "authorityKeyId", "authorityPublicKey", "authoritySignature", "promotionAuthorityKeyId", "promotionAuthorityPublicKey", "promotionAuthoritySignature", "logicalEffect", "ordinaryWork"} {
							if _, present := entry[field]; present {
								t.Fatal("ordinary_inherited_entry_retained_native_material", field)
							}
						}
					}
				}
				for _, raw := range turn["items"].([]any) {
					item := raw.(map[string]any)
					if item["clientUserMessageId"] == ordinaryClientID {
						for _, field := range []string{"steeringProjectionVersion", "steeringContentDigest", "steeringOrigin", "contextDigest", "executionGrantId"} {
							if _, present := item[field]; present {
								t.Fatal("ordinary_inherited_item_retained_native_material", field)
							}
						}
					}
				}
			}
			entry := turn["steering"].([]any)[0].(map[string]any)
			if entry["historyCancellationReason"] != "user_input_discarded" || entry["cancelReason"] != nil || entry["status"] != "cancelled" {
				t.Fatal("primary_or_seed_has_native_discard_discriminator")
			}
			for key := range entry {
				if strings.HasPrefix(key, "authority") || strings.HasPrefix(key, "promotionAuthority") || strings.HasPrefix(key, "cancellationAuthority") || strings.HasPrefix(key, "userInput") {
					t.Fatal("inert_history_retained_execution_material")
				}
			}
		}
		public, err := threadapp.ProjectPublicThread(primary)
		if err != nil || !reflect.DeepEqual(primary["turns"].([]any)[0].(map[string]any)["steering"], seed["turns"].([]any)[0].(map[string]any)["steering"]) || public == nil {
			t.Fatal("persisted_primary_seed_or_public_history_mismatch")
		}
		findQuestion := func(record map[string]any) map[string]any {
			for _, rawTurn := range record["turns"].([]any) {
				for _, rawItem := range rawTurn.(map[string]any)["items"].([]any) {
					item := rawItem.(map[string]any)
					if item["id"] == selector.ItemID {
						return item
					}
				}
			}
			return nil
		}
		persistedQuestion, seedQuestion := findQuestion(primary), findQuestion(seed)
		currentPublic := public
		if handler != nil {
			currentPublic = jsonRequest(http.MethodGet, "/v1/threads/"+id, nil, false, http.StatusOK)
		}
		publicQuestion := findQuestion(currentPublic)
		publicJSON, _ := json.Marshal(publicQuestion)
		seedJSON, _ := json.Marshal(seedQuestion)
		differentKeys := []string{}
		keys := map[string]bool{}
		for key := range publicQuestion {
			keys[key] = true
		}
		for key := range seedQuestion {
			keys[key] = true
		}
		for key := range keys {
			a, _ := json.Marshal(publicQuestion[key])
			b, _ := json.Marshal(seedQuestion[key])
			if string(a) != string(b) {
				differentKeys = append(differentKeys, key)
			}
		}
		sort.Strings(differentKeys)
		identity := func(q map[string]any) bool {
			return q != nil && q["kind"] == "user_input" && q["threadId"] == id && q["turnId"] == selector.TurnID && q["id"] == selector.ItemID && q["inputId"] == selector.RequestID && q["status"] == "continued" && q["continuationReceiptId"] == nil
		}
		t.Logf("history_bind primaryIdentity=%t seedIdentity=%t publicIdentity=%t publicSeedEqual=%t differenceKeys=%v workspaceSame=%t", identity(persistedQuestion), identity(seedQuestion), identity(publicQuestion), string(publicJSON) == string(seedJSON), differentKeys, primary["workspace"] == sourceRecord["workspace"])
		if !identity(persistedQuestion) || !identity(seedQuestion) || !identity(publicQuestion) || string(publicJSON) != string(seedJSON) {
			t.Fatal("persisted_question_and_public_seed_binding_mismatch")
		}
		after, err := os.ReadFile(primaryPath(id))
		if err != nil || !bytes.Equal(body, after) {
			t.Fatal("public_copy_polluted_persisted_primary")
		}
	}
	assertPersisted(childID)
	open()
	assertPersisted(childID)
	assertPublicHistory(childID)
	if afterEChildID != "" {
		assertPersisted(afterEChildID)
		assertPublicHistory(afterEChildID)
	}
	jsonRequest(http.MethodGet, "/v1/threads/"+threadID, nil, false, http.StatusOK)
	reforkBody := map[string]any{"turnId": turnID}
	if wholeThreadFork {
		reforkBody = map[string]any{}
	}
	refork := jsonRequest(http.MethodPost, "/v1/threads/"+childID+"/fork", reforkBody, false, http.StatusCreated)
	grandchildID := contracts.StringField(refork, "id")
	assertPublicHistory(grandchildID)
	shutdownOwnedRuntimeHandler(t, handler)
	handler = nil
	assertPersisted(childID)
	assertPersisted(grandchildID)
	open()
	assertPersisted(childID)
	assertPersisted(grandchildID)
	assertPublicHistory(childID)
	assertPublicHistory(grandchildID)
	if !acceptedAfterE {
		finalSource, err := os.ReadFile(primaryPath(threadID))
		if err != nil || !bytes.Equal(sourceBefore, finalSource) {
			t.Fatal("cold_recovery_or_refork_changed_original_signed_primary")
		}
	}
	wantCalls := int32(2)
	if acceptedHistory || acceptedAfterE {
		wantCalls = 3
	}
	if providerCalls.Load() != wantCalls {
		t.Fatal("fork_recovery_or_discard_executed_extra_provider_work")
	}
}

func listRuntimeHistoryEventsV1(value any) []any { events, _ := value.([]any); return events }
