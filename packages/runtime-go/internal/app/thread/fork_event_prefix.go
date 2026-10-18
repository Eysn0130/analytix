// Selected Fork event-prefix semantics ported from DeepSeek Harness at
// 5badb15009ae1756c3afe0ae0cef1faafc290ccc (MIT, copyright 2026 DeepSeek).
// See THIRD_PARTY_NOTICES.md and docs/analytix/upstreams/code-reuse-provenance.md.
package thread

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	"analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsteering "analytix.local/runtime-go/internal/domain/steering"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
)

var (
	ErrForkBoundaryInvalid     = errors.New("fork event boundary is invalid")
	ErrForkBoundaryUnavailable = errors.New("fork history is unavailable at the requested event boundary")
)

type ForkBoundaryV1 struct {
	TurnID string
	EndSeq int
}

// Omission remains legal for whole-thread forks. A turn-only legacy request
// resolves its real terminal event below, never the latest thread cursor.
func ParseForkBoundaryV1(request map[string]any) (ForkBoundaryV1, error) {
	boundary := ForkBoundaryV1{}
	if raw, present := request["relation"]; present {
		relation, ok := raw.(string)
		if !ok || relation != "primary" && relation != "fork" && relation != "side" {
			return boundary, ErrForkBoundaryInvalid
		}
	}
	if raw, present := request["title"]; present {
		if _, ok := raw.(string); !ok {
			return boundary, ErrForkBoundaryInvalid
		}
	}
	if raw, present := request["turnId"]; present {
		text, ok := raw.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return boundary, ErrForkBoundaryInvalid
		}
		boundary.TurnID = strings.TrimSpace(text)
	}
	if raw, present := request["endSeq"]; present {
		seq, ok := contracts.NumericSeq(raw)
		if !ok || seq <= 0 || int64(seq) > 1<<53-1 || boundary.TurnID == "" {
			return boundary, ErrForkBoundaryInvalid
		}
		boundary.EndSeq = seq
	}
	return boundary, nil
}

type ForkEventPrefixInputV1 struct {
	Source    map[string]any
	Events    []map[string]any
	Boundary  ForkBoundaryV1
	Projector PublicProjector
	// Supplied only after the existing signed inherited inventory and strict
	// primary read have been verified by the durable owner. These inert CASE
	// bytes remain private and confer no public or model authority.
	InheritedCaseTurnIDs map[string]bool
	// The durable owner verifies native signed Question entries or reads these
	// inert bytes from the source's complete creation bundle before supplying
	// this map. Content syntax alone never grants the Question codec slot.
	QuestionHistoryEntries map[string]map[string]any
}

// BuildForkEventPrefixV1 folds the existing durable log's inclusive prefix.
// A canonical terminal may close an already logged, signed discarded admission
// into inert history. Current projection is never an item or Question baseline.
func BuildForkEventPrefixV1(input ForkEventPrefixInputV1) (map[string]any, int, error) {
	if input.Projector == nil || input.Boundary.TurnID == "" {
		return nil, 0, ErrForkBoundaryUnavailable
	}
	current, err := input.Projector.ProjectThread(input.Source)
	if err != nil {
		return nil, 0, errors.Join(ErrForkBoundaryUnavailable, err)
	}
	threadID := stringField(current, "id")
	selected, _, err := forkSourceTurns(listAny(current["turns"]), input.Boundary.TurnID)
	if err != nil {
		return nil, 0, err
	}
	allowed := map[string]map[string]any{}
	allowedItems := map[string]map[string]map[string]any{}
	for _, raw := range selected {
		turn, ok := raw.(map[string]any)
		if !ok || stringField(turn, "id") == "" {
			return nil, 0, ErrForkBoundaryUnavailable
		}
		turnID := stringField(turn, "id")
		if allowed[turnID] != nil {
			return nil, 0, ErrForkBoundaryUnavailable
		}
		allowed[turnID] = turn
		allowedItems[turnID] = map[string]map[string]any{}
		for _, value := range listAny(turn["items"]) {
			item, _ := value.(map[string]any)
			id := stringField(item, "id")
			if id == "" || allowedItems[turnID][id] != nil {
				return nil, 0, ErrForkBoundaryUnavailable
			}
			allowedItems[turnID][id] = item
		}
	}
	if _, err := domainevent.AtomicTerminalReplayEventsAfter(input.Events, 0); err != nil {
		return nil, 0, errors.Join(ErrForkBoundaryUnavailable, err)
	}
	projectedEvents := make([]map[string]any, len(input.Events))
	seed, seedEndSeq, err := ReadForkCreatedSeedV1(input.Source, input.Events)
	if err != nil {
		return nil, 0, err
	}
	endSeq := 0
	for index, event := range input.Events {
		seq, ok := contracts.NumericSeq(event["seq"])
		if !ok || seq != index+1 || stringField(event, "threadId") != threadID {
			return nil, 0, ErrForkBoundaryUnavailable
		}
		projected, visible, projectionErr := input.Projector.ProjectEvent(threadID, input.Source, event)
		if projectionErr != nil {
			return nil, 0, errors.Join(ErrForkBoundaryUnavailable, projectionErr)
		}
		if visible {
			projectedEvents[index] = projected
		}
		if visible && stringField(event, "turnId") == input.Boundary.TurnID && forkTerminalEventV1(projected) &&
			forkTerminalStatusV1(projected) == stringField(allowed[input.Boundary.TurnID], "status") {
			if input.Boundary.EndSeq == 0 || seq == input.Boundary.EndSeq {
				endSeq = seq
			}
		}
	}
	// An inherited turn is already closed at the complete creation seed bundle.
	if endSeq == 0 && seed != nil && (input.Boundary.EndSeq == 0 || input.Boundary.EndSeq == seedEndSeq) {
		for _, raw := range listAny(seed["turns"]) {
			turn, _ := raw.(map[string]any)
			if stringField(turn, "id") == input.Boundary.TurnID && publicTerminalStatus(stringField(turn, "status")) && stringField(turn, "status") == stringField(allowed[input.Boundary.TurnID], "status") {
				endSeq = seedEndSeq
			}
		}
	}
	if endSeq == 0 && input.InheritedCaseTurnIDs[input.Boundary.TurnID] && len(input.Events) > 0 &&
		stringField(input.Events[0], "kind") == "thread_created" && (input.Boundary.EndSeq == 0 || input.Boundary.EndSeq == 1) {
		endSeq = 1
	}
	if endSeq == 0 {
		return nil, 0, ErrForkBoundaryInvalid
	}
	turns := map[string]map[string]any{}
	promptLogged := map[string]bool{}
	loggedItems := map[string]map[string]bool{}
	questionItems := map[string]map[string]map[string]any{}
	prefixAdmissions := map[string]int{}
	terminalAuthorities, _ := domainturnterminal.GeneralTerminalProjectionAuthoritiesV1(input.Source)
	for id := range allowed {
		loggedItems[id] = map[string]bool{}
		questionItems[id] = map[string]map[string]any{}
	}
	if seed != nil {
		for _, raw := range listAny(seed["turns"]) {
			turn, _ := raw.(map[string]any)
			id := stringField(turn, "id")
			if allowed[id] == nil {
				continue
			}
			cloned := contracts.CloneMap(turn)
			cloned["items"] = []any{}
			cloned["steering"] = []any{}
			for _, rawEntry := range listAny(turn["steering"]) {
				entry, ok := rawEntry.(map[string]any)
				if legacy, isString := rawEntry.(string); isString {
					for _, currentEntry := range listAny(allowed[id]["steering"]) {
						if text, match := currentEntry.(string); match && text == legacy {
							cloned["steering"] = append(listAny(cloned["steering"]), legacy)
							break
						}
					}
					continue
				}
				if !ok {
					return nil, 0, ErrForkBoundaryUnavailable
				}
				if err := forkPrefixUpsertSteeringV1(cloned, allowed[id], entry, input.QuestionHistoryEntries); err != nil {
					return nil, 0, err
				}
			}
			for _, value := range listAny(turn["items"]) {
				item, _ := value.(map[string]any)
				itemID := stringField(item, "id")
				if allowedItems[id][itemID] == nil {
					continue
				}
				loggedItems[id][itemID] = true
				cloned["items"] = append(listAny(cloned["items"]), contracts.CloneMap(item))
			}
			turns[id] = cloned
			_, promptLogged[id] = cloned["prompt"].(string)
		}
	}
	for _, raw := range listAny(input.Source["turns"]) {
		turn, _ := raw.(map[string]any)
		id := stringField(turn, "id")
		if allowed[id] != nil && input.InheritedCaseTurnIDs[id] {
			if err := ValidateCaseDerivedHistoryTurnV1(threadID, turn); err != nil {
				return nil, 0, err
			}
			turns[id] = contracts.CloneMap(turn)
			promptLogged[id] = true
		}
	}
	for index, raw := range input.Events {
		seq := index + 1
		turnID := stringField(raw, "turnId")
		if allowed[turnID] == nil || input.InheritedCaseTurnIDs[turnID] {
			continue
		}
		projected := projectedEvents[index]
		var questionItem map[string]any
		if kind := stringField(raw, "kind"); kind == "user_input_requested" || kind == "user_input_resolved" {
			itemID := stringField(raw, "itemId")
			questionItem, err = forkQuestionEventItemV1(threadID, turnID, raw, projected, allowedItems[turnID][itemID], questionItems[turnID][itemID])
			if err != nil {
				return nil, 0, err
			}
			if questionItem != nil {
				questionItems[turnID][itemID] = questionItem
			}
		}
		if projected == nil {
			continue
		}
		if stringField(raw, "kind") == "compaction_completed" && seq <= endSeq {
			initial, ok := raw["turn"].(map[string]any)
			if !ok || stringField(initial, "id") != turnID || stringField(initial, "threadId") != threadID || stringField(initial, "status") != "completed" {
				return nil, 0, ErrForkBoundaryUnavailable
			}
			public, projectionErr := ProjectPublicThread(map[string]any{"id": threadID, "status": "idle", "turns": []any{initial}})
			if projectionErr != nil {
				return nil, 0, ErrForkBoundaryUnavailable
			}
			turn := public["turns"].([]any)[0].(map[string]any)
			turns[turnID] = contracts.CloneMap(turn)
			_, promptLogged[turnID] = turn["prompt"].(string)
			turns[turnID]["items"] = []any{}
			for _, rawItem := range listAny(turn["items"]) {
				item, _ := rawItem.(map[string]any)
				if allowedItems[turnID][stringField(item, "id")] == nil {
					continue
				}
				loggedItems[turnID][stringField(item, "id")] = true
				forkPrefixUpsertItemV1(turns[turnID], item)
			}
		}
		if stringField(raw, "kind") == "turn_started" && seq <= endSeq {
			turns[turnID], err = forkStartedTurnV1(threadID, allowed[turnID], raw, projected)
			if initial, ok := raw["turn"].(map[string]any); ok {
				_, promptLogged[turnID] = initial["prompt"].(string)
			}
			if err != nil {
				return nil, 0, err
			}
		}
		item, _ := projected["item"].(map[string]any)
		if questionItem != nil {
			item = questionItem
		}
		if item == nil && stringField(raw, "kind") == "pipeline_stage" {
			// Older completion bundles prove the Host item identity, but omit
			// its exact producer bytes and createdAt. A later event may be
			// excluded at E without inventing it; an included item needs bytes.
			if itemID := subagentapp.BackgroundJobCompletionItemID(turnID, raw); itemID != "" && allowedItems[turnID][itemID] != nil {
				loggedItems[turnID][itemID] = true
				if seq <= endSeq {
					item, _ = raw["item"].(map[string]any)
					if item == nil {
						return nil, 0, ErrForkBoundaryUnavailable
					}
					item, _ = projectOrdinaryPublicHistoryItemV1(item)
					if item == nil || stringField(item, "id") != itemID {
						return nil, 0, ErrForkBoundaryUnavailable
					}
				}
			}
		}
		if item == nil && stringField(raw, "kind") == "tool_call_ready" {
			item = forkReadyToolCallV1(threadID, turnID, raw)
		}
		if item != nil {
			itemID := stringField(item, "id")
			if stringField(item, "threadId") != threadID || stringField(item, "turnId") != turnID {
				return nil, 0, ErrForkBoundaryUnavailable
			}
			if currentItem := allowedItems[turnID][itemID]; currentItem != nil && stringField(currentItem, "kind") == stringField(item, "kind") {
				loggedItems[turnID][itemID] = true
				if seq <= endSeq {
					if turns[turnID] == nil {
						return nil, 0, ErrForkBoundaryUnavailable
					}
					if stringField(item, "kind") == "tool_call" {
						if err := forkBindToolCallKindV1(turns[turnID], item, currentItem); err != nil {
							return nil, 0, err
						}
					}
					if stringField(item, "kind") == "tool_result" &&
						(!forkSameToolCoordinatesV1(item, currentItem) ||
							stringField(item, "id") != domaintoolresult.ToolResultItemIDV1(turnID, stringField(item, "callId")) ||
							!forkPublicToolKindV1(stringField(item, "toolKind")) || item["toolKind"] != currentItem["toolKind"]) {
						return nil, 0, ErrForkBoundaryUnavailable
					}
					forkPrefixUpsertItemV1(turns[turnID], item)
					if stringField(item, "kind") == "user_message" && stringField(item, "delivery") != "steer" && !promptLogged[turnID] {
						turns[turnID]["prompt"] = item["text"]
						_, promptLogged[turnID] = item["text"].(string)
					}
					if stringField(raw, "kind") == "tool_call_finished" {
						for _, value := range listAny(turns[turnID]["items"]) {
							call, _ := value.(map[string]any)
							if stringField(call, "kind") == "tool_call" && stringField(call, "callId") == stringField(item, "callId") {
								currentCall := allowedItems[turnID][stringField(call, "id")]
								if !forkSameToolIdentityV1(call, item) || !forkSameToolIdentityV1(call, currentCall) ||
									!forkPublicToolKindV1(stringField(item, "toolKind")) || currentCall["toolKind"] != item["toolKind"] ||
									stringField(call, "toolKind") != "" && call["toolKind"] != item["toolKind"] {
									return nil, 0, ErrForkBoundaryUnavailable
								}
								// A real completion inside E proves this immutable kind;
								// the flat ready record does not carry it.
								call["toolKind"] = item["toolKind"]
								call["status"] = item["status"]
								call["finishedAt"] = item["finishedAt"]
							}
						}
					}
				}
			}
		}
		if seq <= endSeq && turns[turnID] != nil {
			if stringField(raw, "kind") == "turn_steered" {
				if metadata, ok := raw["steeringEntry"].(map[string]any); ok {
					entry := contracts.CloneMap(metadata)
					entry["text"], entry["displayText"] = raw["text"], raw["displayText"]
					if err := forkPrefixUpsertSteeringV1(turns[turnID], allowed[turnID], entry, input.QuestionHistoryEntries); err != nil {
						return nil, 0, err
					}
					if entry["status"] == "pending" {
						prefixAdmissions[stringField(entry, "id")]++
					}
				}
			}
			if item != nil && stringField(item, "kind") == "user_message" && stringField(item, "delivery") == "steer" {
				entry := map[string]any{"id": stringField(item, "id"), "clientUserMessageId": item["clientUserMessageId"], "text": item["text"], "admittedAt": item["createdAt"], "status": "promoted", "promotedAt": item["finishedAt"], "promotedItemId": item["id"], "attachmentIds": []any{}, "fileReferences": []any{}}
				for _, key := range []string{"displayText", "attachmentIds", "fileReferences"} {
					if value, present := item[key]; present {
						entry[key] = contracts.CloneValue(value)
					}
				}
				if err := forkPrefixUpsertSteeringV1(turns[turnID], allowed[turnID], entry, input.QuestionHistoryEntries); err != nil {
					return nil, 0, err
				}
			}
		}
		if seq <= endSeq && forkTerminalEventV1(projected) {
			if turns[turnID] == nil {
				return nil, 0, ErrForkBoundaryUnavailable
			}
			if stringField(projected, "kind") != "compaction_completed" {
				restoreForkDiscardHistoryAtTerminalV1(input.Source, turns[turnID], raw, endSeq, terminalAuthorities[turnID], input.QuestionHistoryEntries, prefixAdmissions)
				turns[turnID]["status"] = projected["status"]
				turns[turnID]["finishedAt"] = projected["timestamp"]
				turns[turnID]["steering"] = turnapp.CancelPendingSteeringEntries(turns[turnID]["steering"], stringField(projected, "timestamp"), stringField(projected, "status"))
			}
		}
	}
	ordered := make([]any, 0, len(selected))
	for _, raw := range selected {
		id := stringField(raw.(map[string]any), "id")
		turn := turns[id]
		if turn == nil || !promptLogged[id] {
			return nil, 0, ErrForkBoundaryUnavailable
		}
		// A current item without any real log producer or creation seed proves
		// incomplete legacy history. Late logged items are legitimately absent
		// from E; unlogged items must not be silently lost or copied from now.
		if !input.InheritedCaseTurnIDs[id] {
			for _, rawEntry := range listAny(allowed[id]["steering"]) {
				currentEntry, ok := rawEntry.(map[string]any)
				found := false
				for _, entry := range listAny(turn["steering"]) {
					if legacy, isString := rawEntry.(string); isString {
						text, _ := entry.(string)
						found = found || text == legacy
					} else if folded, isMap := entry.(map[string]any); ok && isMap {
						found = found || stringField(folded, "id") == stringField(currentEntry, "id")
					}
				}
				if !found {
					return nil, 0, ErrForkBoundaryUnavailable
				}
			}
			for itemID := range allowedItems[id] {
				if !loggedItems[id][itemID] {
					return nil, 0, ErrForkBoundaryUnavailable
				}
				// Resolve against the complete committed log, while only the
				// inclusive prefix supplies content. A later resolution may prove
				// the current status without changing the pending Question at E.
				if question := questionItems[id][itemID]; question != nil && question["status"] != allowedItems[id][itemID]["status"] {
					return nil, 0, ErrForkBoundaryUnavailable
				}
			}
		}
		for _, rawSourceTurn := range listAny(input.Source["turns"]) {
			original, _ := rawSourceTurn.(map[string]any)
			if stringField(original, "id") != id {
				continue
			}
			privateCalls := privateProtocolObservedCallIDsV1(original)
			kept := []any{}
			for _, rawItem := range listAny(turn["items"]) {
				item, _ := rawItem.(map[string]any)
				kind := stringField(item, "kind")
				if privateCalls[stringField(item, "callId")] && (kind == "tool_call" || kind == "tool_result") {
					continue
				}
				kept = append(kept, item)
			}
			turn["items"] = kept
		}
		for _, rawItem := range listAny(turn["items"]) {
			item, _ := rawItem.(map[string]any)
			if kind := stringField(item, "kind"); (kind == "tool_call" || kind == "tool_result") && !forkPublicToolKindV1(stringField(item, "toolKind")) {
				// Refuse before allocating a child instead of persisting an HTTP
				// response that the desktop's required public schema must reject.
				return nil, 0, ErrForkBoundaryUnavailable
			}
		}
		ordered = append(ordered, turn)
	}
	candidate := contracts.CloneMap(input.Source)
	stripDerivedExecutionAuthorityFields(candidate)
	candidate["turns"] = ordered
	delete(candidate, "todos") // A present-day Todo snapshot has no E authority.
	delete(candidate, "preview")
	caseSensitive, err := domainsecurity.ClassifyCaseSensitiveThread(input.Source)
	if err != nil {
		return nil, 0, err
	}
	if caseSensitive {
		candidate["historyAuthority"] = CaseBoundaryOnlyHistoryAuthority
	} else if err := deriveOrdinaryCompactionHistoryV1(input.Source, candidate, "fork-prefix"); err != nil {
		return nil, 0, errors.Join(ErrForkBoundaryUnavailable, err)
	}
	return candidate, endSeq, nil
}

// Native discard and terminal CAS share the durable owner mutex. Once this
// exact canonical terminal closes the original context, its verified cancelled
// admission cannot acquire a later cancellation. This supplies only inert
// reason/time for a unique physical prefix admission, never answer authority.
func restoreForkDiscardHistoryAtTerminalV1(
	source, prefixTurn, terminal map[string]any,
	endSeq int,
	authority domainturnterminal.GeneralTerminalProjectionAuthorityV1,
	questions map[string]map[string]any,
	admissions map[string]int,
) {
	kind := stringField(terminal, "kind")
	seq, seqOK := contracts.NumericSeq(terminal["seq"])
	if kind != "turn_completed" && kind != "turn_failed" && kind != "turn_aborted" ||
		!seqOK || seq <= 0 || seq > endSeq || !authority.Governed || !authority.Terminal {
		return
	}
	governed, valid, err := domainturnterminal.ValidateGeneralTerminalPublicEventV1(source, terminal)
	commit := authority.Commit
	turnID := stringField(prefixTurn, "id")
	if err != nil || !governed || !valid || commit.ThreadID != stringField(source, "id") ||
		commit.TurnID != turnID || stringField(terminal, "turnId") != turnID ||
		stringField(terminal, "generalTerminalCommitId") != commit.CommitID ||
		stringField(terminal, "status") != commit.TerminalStatus || stringField(terminal, "timestamp") != commit.CommittedAt {
		return
	}
	var original map[string]any
	for _, raw := range listAny(source["turns"]) {
		turn, _ := raw.(map[string]any)
		if stringField(turn, "id") == turnID {
			if original != nil {
				return
			}
			original = turn
		}
	}
	if original == nil || stringField(original, "status") != commit.TerminalStatus || stringField(original, "finishedAt") != commit.CommittedAt {
		return
	}
	context, contextErr := domainsecurity.ParseTurnSecurityContext(original["securityContext"])
	terminalTime, timeErr := time.Parse(time.RFC3339Nano, commit.CommittedAt)
	if contextErr != nil || domainsecurity.ValidateTurnSecurityContextForExecution(context) != nil ||
		!domainsecurity.TurnSecurityContextIsGeneral(context) || context.ThreadID != commit.ThreadID || context.TurnID != turnID ||
		context.ContextDigest != commit.ContextDigest || context.ContextEpoch != commit.ContextEpoch || timeErr != nil {
		return
	}
	entries := listAny(prefixTurn["steering"])
	for index, raw := range entries {
		entry, _ := raw.(map[string]any)
		id := stringField(entry, "id")
		question := questions[id] // This map is cryptographically verified by the durable owner.
		if entry["status"] != "pending" || admissions[id] != 1 || question == nil ||
			domainsteering.ValidateCancelledUserInputEntryForContextV1(question, context.ContextDigest) != nil {
			continue
		}
		// Do not overwrite any pre-existing lifecycle metadata in the prefix.
		cleanPending := true
		for _, key := range []string{"cancelledAt", "cancelReason", forkHistoryCancellationReasonV1, "promotedAt", "promotedItemId"} {
			if _, present := entry[key]; present {
				cleanPending = false
			}
		}
		matches := 0
		for _, rawOriginal := range listAny(original["steering"]) {
			candidate, _ := rawOriginal.(map[string]any)
			if stringField(candidate, "id") == id {
				// JSON equality tolerates only the normal recovered numeric representation.
				a, aErr := json.Marshal(candidate)
				b, bErr := json.Marshal(question)
				if aErr != nil || bErr != nil || string(a) != string(b) {
					cleanPending = false
				}
				matches++
			}
		}
		cancelled, cancelErr := time.Parse(time.RFC3339Nano, stringField(question, "cancelledAt"))
		inert, inertErr := forkInertSteeringEntryV1(entry, question)
		if !cleanPending || matches != 1 || cancelErr != nil || cancelled.After(terminalTime) || inertErr != nil {
			continue
		}
		inert["status"], inert["cancelledAt"] = "cancelled", question["cancelledAt"]
		inert[forkHistoryCancellationReasonV1] = question["cancelReason"]
		if validateForkHistoryCancellationV1(inert) == nil {
			entries[index] = inert
		}
	}
	prefixTurn["steering"] = entries
}

func forkTerminalEventV1(event map[string]any) bool {
	switch stringField(event, "kind") {
	case "turn_completed", "turn_failed", "turn_aborted":
		return true
	case "compaction_completed":
		return true
	}
	return false
}

func forkTerminalStatusV1(event map[string]any) string {
	if stringField(event, "kind") == "compaction_completed" {
		return "completed"
	}
	return stringField(event, "status")
}

func forkStartedTurnV1(threadID string, current, raw, projected map[string]any) (map[string]any, error) {
	turnID := stringField(raw, "turnId")
	turn := map[string]any{"id": turnID, "threadId": threadID, "status": "running", "createdAt": raw["timestamp"], "startedAt": raw["timestamp"], "items": []any{}, "steering": []any{}, "attachmentIds": []any{}, "activeSkillIds": []any{}, "injectedMemoryIds": []any{}}
	if initial, ok := raw["turn"].(map[string]any); ok {
		if stringField(initial, "id") != turnID || stringField(initial, "threadId") != threadID {
			return nil, ErrForkBoundaryUnavailable
		}
		public, safe := domainevent.SanitizePublicValue(initial, false)
		if !safe {
			return nil, ErrForkBoundaryUnavailable
		}
		for key, value := range public.(map[string]any) {
			if _, allowed := current[key]; allowed && key != "items" && key != "acceptedFinal" && key != "acceptedFinalView" && key != "finishedAt" {
				turn[key] = contracts.CloneValue(value)
			}
		}
	}
	for _, key := range []string{"model", "mode", "reasoningEffort", "attachmentIds", "workspaceCheckpointId", "disableUserInput", "maxModelSteps"} {
		if value, present := projected[key]; present {
			turn[key] = contracts.CloneValue(value)
		}
	}
	return turn, nil
}

func forkPrefixUpsertItemV1(turn, item map[string]any) {
	items := listAny(turn["items"])
	for index, raw := range items {
		if stringField(raw.(map[string]any), "id") == stringField(item, "id") {
			items[index] = contracts.CloneMap(item)
			turn["items"] = items
			return
		}
	}
	turn["items"] = append(items, contracts.CloneMap(item))
}

func forkPublicToolKindV1(kind string) bool {
	return kind == "tool_call" || kind == "command_execution" || kind == "file_change" || kind == "subagent"
}

func forkSameToolIdentityV1(left, right map[string]any) bool {
	return forkSameToolCoordinatesV1(left, right) &&
		stringField(left, "id") == domaintoolcall.ToolCallItemIDV1(stringField(left, "turnId"), stringField(left, "callId"))
}

func forkSameToolCoordinatesV1(left, right map[string]any) bool {
	return stringField(left, "threadId") == stringField(right, "threadId") &&
		stringField(left, "turnId") == stringField(right, "turnId") &&
		stringField(left, "callId") != "" && left["callId"] == right["callId"] &&
		stringField(left, "toolName") != "" && left["toolName"] == right["toolName"]
}

func forkBindToolCallKindV1(turn, item, current map[string]any) error {
	if !forkSameToolIdentityV1(item, current) {
		return ErrForkBoundaryUnavailable
	}
	for _, raw := range listAny(turn["items"]) {
		previous, _ := raw.(map[string]any)
		if stringField(previous, "id") != stringField(item, "id") {
			continue
		}
		if !forkSameToolIdentityV1(previous, item) || stringField(previous, "kind") != "tool_call" {
			return ErrForkBoundaryUnavailable
		}
		if kind := stringField(previous, "toolKind"); kind != "" {
			if !forkPublicToolKindV1(kind) || stringField(item, "toolKind") != "" && item["toolKind"] != kind {
				return ErrForkBoundaryUnavailable
			}
			// Preserve an earlier real start/creation producer when a later
			// flat ready event replaces the same item. No current bytes enter E.
			item["toolKind"] = kind
		}
	}
	if kind := stringField(item, "toolKind"); kind != "" && (!forkPublicToolKindV1(kind) || current["toolKind"] != kind) {
		return ErrForkBoundaryUnavailable
	}
	return nil
}

func forkReadyToolCallV1(threadID, turnID string, event map[string]any) map[string]any {
	callID := stringField(event, "callId")
	id := domaintoolcall.ToolCallItemIDV1(turnID, callID)
	if id == "" || stringField(event, "itemId") != id {
		return nil
	}
	return domaintoolcall.PublicToolCallItemRecordV1(map[string]any{"id": id, "threadId": threadID, "turnId": turnID, "role": "tool", "kind": "tool_call", "status": "pending", "createdAt": event["timestamp"], "callId": callID, "toolName": event["toolName"], "arguments": domaintoolcall.PublicToolCallArgumentsProjectionRecordV1()})
}

// Question producers are flat gate events, not item events. Rebuild only the
// admitted public identity and the committed request bytes. As with ready
// tool calls, replay times come from the log; the primary's later patch clock
// is neither an E baseline nor a second producer.
func forkQuestionEventItemV1(threadID, turnID string, raw, projected, current, previous map[string]any) (map[string]any, error) {
	if current == nil {
		return nil, nil // The current public ceiling has removed this item.
	}
	inputID, itemID := stringField(raw, "inputId"), stringField(raw, "itemId")
	if projected == nil || inputID == "" || itemID != "item_"+inputID ||
		stringField(current, "kind") != "user_input" || stringField(current, "inputId") != inputID ||
		stringField(projected, "inputId") != inputID || stringField(projected, "threadId") != threadID || stringField(projected, "turnId") != turnID {
		return nil, ErrForkBoundaryUnavailable
	}
	timestamp := stringField(projected, "timestamp")
	if _, err := time.Parse(time.RFC3339Nano, timestamp); err != nil {
		return nil, ErrForkBoundaryUnavailable
	}
	if stringField(raw, "kind") == "user_input_requested" {
		if previous != nil || stringField(projected, "status") != "pending" || stringField(projected, "prompt") == "" {
			return nil, ErrForkBoundaryUnavailable
		}
		questions, ok := projected["questions"].([]any)
		if !ok || len(questions) == 0 {
			return nil, ErrForkBoundaryUnavailable
		}
		item := map[string]any{"id": itemID, "threadId": threadID, "turnId": turnID, "role": "system", "kind": "user_input", "inputId": inputID, "status": "pending", "createdAt": timestamp, "prompt": projected["prompt"], "questions": contracts.CloneValue(questions)}
		if wait, present := projected["timedWait"]; present {
			item["timedWait"] = contracts.CloneValue(wait)
		}
		return item, nil
	}
	status := stringField(projected, "status")
	if previous == nil || stringField(previous, "status") != "pending" ||
		(status != "submitted" && status != "cancelled" && status != "continued") {
		return nil, ErrForkBoundaryUnavailable
	}
	for _, field := range []string{"prompt", "questions", "timedWait"} {
		if value, present := projected[field]; present && !reflect.DeepEqual(value, previous[field]) {
			return nil, ErrForkBoundaryUnavailable
		}
	}
	item := contracts.CloneMap(previous)
	item["status"], item["finishedAt"] = status, timestamp
	return item, nil
}

func forkPrefixUpsertSteeringV1(turn, current, entry map[string]any, questions map[string]map[string]any) error {
	id, clientID := stringField(entry, "id"), stringField(entry, "clientUserMessageId")
	if id == "" || clientID == "" || id != domainsteering.EntryIDV1(stringField(turn, "id"), clientID) {
		return ErrForkBoundaryUnavailable
	}
	readable := false
	for _, raw := range listAny(current["steering"]) {
		candidate, _ := raw.(map[string]any)
		if stringField(candidate, "id") == id && stringField(candidate, "clientUserMessageId") == clientID {
			readable = true
		}
	}
	if !readable {
		return nil
	}
	for key := range entry {
		switch key {
		case "id", "clientUserMessageId", "text", "displayText", "attachmentIds", "fileReferences", "admittedAt", "status", "promotedAt", "promotedItemId", "cancelledAt", "cancelReason", forkHistoryCancellationReasonV1, "kind", "delivery":
		default:
			return ErrForkBoundaryUnavailable
		}
	}
	if stringField(entry, "text") == "" || stringField(entry, "admittedAt") == "" {
		return ErrForkBoundaryUnavailable
	}
	var err error
	entry, err = forkInertSteeringEntryV1(entry, questions[id])
	if err != nil {
		return err
	}
	entries := listAny(turn["steering"])
	for index, raw := range entries {
		if stringField(raw.(map[string]any), "id") == id {
			entries[index] = contracts.CloneMap(entry)
			turn["steering"] = entries
			return nil
		}
	}
	turn["steering"] = append(entries, contracts.CloneMap(entry))
	return nil
}

// NormalizeForkQuestionHistoryV1 strips native steering authority from the
// newly built fork. The primary and creation seed share the same inert history;
// Question bytes retain only their existing user-message codec markers.
func NormalizeForkQuestionHistoryV1(fork map[string]any, questions map[string]map[string]any) error {
	for _, raw := range listAny(fork["turns"]) {
		turn, ok := raw.(map[string]any)
		if !ok {
			return ErrForkBoundaryUnavailable
		}
		if value, present := turn["steering"]; present && value != nil {
			if _, ok := value.([]any); !ok {
				return ErrForkBoundaryUnavailable
			}
		}
		if _, err := NormalizeInertForkTurnPolicyV1(turn); err != nil {
			return err
		}
		delete(turn, "approvalPolicy")
		delete(turn, "sandboxMode")
		sourceEntries := map[string]map[string]any{}
		entries := []any{}
		for _, rawEntry := range listAny(turn["steering"]) {
			if legacy, ok := rawEntry.(string); ok {
				entries = append(entries, legacy)
				continue
			}
			entry, ok := rawEntry.(map[string]any)
			if !ok || sourceEntries[stringField(entry, "id")] != nil {
				return ErrForkBoundaryUnavailable
			}
			sourceEntries[stringField(entry, "id")] = entry
			question := questions[stringField(entry, "id")]
			inert, err := forkInertSteeringEntryV1(entry, question)
			if err != nil {
				return err
			}
			entries = append(entries, inert)
		}
		items := []any{}
		for _, rawItem := range listAny(turn["items"]) {
			item, ok := rawItem.(map[string]any)
			if !ok {
				return ErrForkBoundaryUnavailable
			}
			inert, err := NormalizeInertForkSteeringItemV1(item)
			if err != nil {
				return err
			}
			if _, native := item["steeringProjectionVersion"]; native {
				entry := sourceEntries[stringField(item, "id")]
				if stringField(item, "threadId") != stringField(fork, "id") || stringField(item, "turnId") != stringField(turn, "id") ||
					!forkPromotedItemMatchesEntryV1(item, entry) {
					return ErrForkBoundaryUnavailable
				}
			}
			items = append(items, inert)
		}
		turn["steering"], turn["items"] = entries, items
	}
	return nil
}

// NormalizeInertForkTurnPolicyV1 removes only validated source execution
// policy metadata excluded by the closed creation seed. This is inert history,
// not a turn configuration: migration callers must still prove the complete
// remaining metadata and ordered items against the immutable seed.
func NormalizeInertForkTurnPolicyV1(turn map[string]any) (map[string]any, error) {
	out := contracts.CloneMap(turn)
	for field, normalize := range map[string]func(string) string{
		"approvalPolicy": NormalizeApprovalPolicy,
		"sandboxMode":    NormalizeSandboxMode,
	} {
		if raw, present := out[field]; present {
			value, ok := raw.(string)
			if !ok || value == "" || normalize(value) != value {
				return nil, ErrForkBoundaryUnavailable
			}
			delete(out, field)
		}
	}
	return out, nil
}

// NormalizeInertForkSteeringItemV1 recognizes only the three native ordinary
// promotion markers left by older fork producers. It removes no other bytes
// and grants no authority: callers must bind the remaining complete item to
// the admitted entry (producer) or immutable closed creation seed (migration).
func NormalizeInertForkSteeringItemV1(item map[string]any) (map[string]any, error) {
	out := contracts.CloneMap(item)
	_, version := item["steeringProjectionVersion"]
	_, digest := item["steeringContentDigest"]
	_, origin := item["steeringOrigin"]
	if !version && !digest && !origin {
		return out, nil
	}
	v, validVersion := contracts.NumericSeq(item["steeringProjectionVersion"])
	turnID, clientID := stringField(item, "turnId"), stringField(item, "clientUserMessageId")
	if !version || !digest || !origin || !validVersion || v != domainsteering.ProjectionVersionV1 ||
		item["steeringOrigin"] != "ordinary" || !domainsecurity.IsSHA256Hex(stringField(item, "steeringContentDigest")) ||
		stringField(item, "steeringContentDigest") != strings.TrimSpace(stringField(item, "steeringContentDigest")) ||
		item["kind"] != "user_message" || item["role"] != "user" || item["status"] != "completed" || item["delivery"] != "steer" ||
		turnID == "" || clientID == "" || stringField(item, "id") == "" {
		return nil, ErrForkBoundaryUnavailable
	}
	delete(out, "steeringProjectionVersion")
	delete(out, "steeringContentDigest")
	delete(out, "steeringOrigin")
	return out, nil
}

func forkPromotedItemMatchesEntryV1(item, entry map[string]any) bool {
	if entry == nil || entry["status"] != "promoted" {
		return false
	}
	for _, pair := range [][2]string{{"id", "id"}, {"id", "promotedItemId"}, {"clientUserMessageId", "clientUserMessageId"},
		{"text", "text"}, {"createdAt", "admittedAt"}, {"finishedAt", "promotedAt"}} {
		left, leftOK := item[pair[0]].(string)
		right, rightOK := entry[pair[1]].(string)
		if !leftOK || !rightOK || left == "" || left != right {
			return false
		}
	}
	display, itemDisplay := "", ""
	if value, present := entry["displayText"]; present {
		var ok bool
		if display, ok = value.(string); !ok {
			return false
		}
	}
	if value, present := item["displayText"]; present {
		var ok bool
		if itemDisplay, ok = value.(string); !ok {
			return false
		}
	}
	if display == stringField(entry, "text") {
		display = ""
	}
	if itemDisplay != display {
		return false
	}
	if digest, present := entry["contentDigest"]; present && digest != item["steeringContentDigest"] {
		return false
	}
	return true
}

// This field preserves a proved, discarded attempt as history. It is never a
// native cancellation authority discriminator, nor an inherited answer handle.
const forkHistoryCancellationReasonV1 = "historyCancellationReason"

func validateForkHistoryCancellationV1(entry map[string]any) error {
	reason, present := entry[forkHistoryCancellationReasonV1]
	if !present {
		return nil
	}
	_, nativeReason := entry["cancelReason"]
	_, timeErr := time.Parse(time.RFC3339Nano, stringField(entry, "cancelledAt"))
	if nativeReason || reason != domainsteering.UserInputDiscardReasonV1 || entry["status"] != "cancelled" ||
		entry["kind"] != "user_message" || entry["delivery"] != "steer" || timeErr != nil {
		return ErrForkBoundaryUnavailable
	}
	return nil
}

func forkInertSteeringEntryV1(entry, question map[string]any) (map[string]any, error) {
	out := selectPublicFields(entry, []string{"id", "clientUserMessageId", "text", "displayText", "attachmentIds", "fileReferences", "admittedAt", "status", "promotedAt", "promotedItemId", "cancelledAt", "cancelReason", forkHistoryCancellationReasonV1})
	if _, marker := entry["kind"]; marker && (question == nil || entry["kind"] != "user_message" || entry["delivery"] != "steer") {
		return nil, ErrForkBoundaryUnavailable
	}
	if question != nil {
		// Match immutable signed/seed bytes to the physical prefix. Never
		// substitute current content, even when the opaque identities match.
		for _, key := range []string{"id", "clientUserMessageId", "text", "displayText", "attachmentIds", "fileReferences", "admittedAt"} {
			a, b := contracts.CloneValue(entry[key]), contracts.CloneValue(question[key])
			if key == "displayText" {
				if a == nil {
					a = ""
				}
				if b == nil {
					b = ""
				}
			}
			if key == "attachmentIds" || key == "fileReferences" {
				if a == nil {
					a = []any{}
				}
				if b == nil {
					b = []any{}
				}
			}
			if !reflect.DeepEqual(a, b) {
				return nil, ErrForkBoundaryUnavailable
			}
		}
		// The log builder writes an empty displayText even when the canonical
		// signed admission omits it. Preserve that proved absence after matching
		// the equivalent empty bytes; never copy current display content.
		if _, present := question["displayText"]; !present && out["displayText"] == "" {
			delete(out, "displayText")
		}
		if err := domainevent.ValidateQuestionReplyContentV1(stringField(entry, "text")); err != nil {
			return nil, errors.Join(ErrForkBoundaryUnavailable, err)
		}
		out["kind"], out["delivery"] = "user_message", "steer"
	}
	_, historyReason := out[forkHistoryCancellationReasonV1]
	if out["cancelReason"] == domainsteering.UserInputDiscardReasonV1 {
		if question == nil || historyReason {
			return nil, ErrForkBoundaryUnavailable
		}
		out[forkHistoryCancellationReasonV1] = out["cancelReason"]
		delete(out, "cancelReason")
	}
	if err := validateForkHistoryCancellationV1(out); err != nil {
		return nil, err
	}
	if err := domainevent.ValidatePublicRecord(out); err != nil {
		return nil, errors.Join(ErrForkBoundaryUnavailable, err)
	}
	return out, nil
}

// BuildForkCreatedEventV1 is the sole creation producer for durable forks.
// Ordinary inherited bytes live in the existing log. CASE keeps its existing
// signed, private inherited-history chain and metadata-only public event.
func BuildForkCreatedEventV1(fork map[string]any, sourceThreadID, sourceTurnID string, sourceEndSeq int) (map[string]any, error) {
	id := stringField(fork, "id")
	event := map[string]any{"kind": "thread_created", "threadId": id, "status": stringField(fork, "status"), "timestamp": stringField(fork, "createdAt"), "seq": float64(1)}
	caseSensitive, err := domainsecurity.ClassifyCaseSensitiveThread(fork)
	if err != nil {
		return nil, err
	}
	if caseSensitive {
		return event, domainevent.ValidatePublicRecord(event)
	}
	// Preserve only already validated carriers for the existing inert seed
	// whitelist and complete seed checks; the public DTO closes them later.
	public, err := NewTrustedPublicProjector(nil).projectValidatedThreadV1(fork)
	if err != nil {
		return nil, err
	}
	seedTurns := contracts.CloneValue(public["turns"]).([]any)
	for _, raw := range seedTurns {
		turn := raw.(map[string]any)
		entries := []any{}
		for _, rawEntry := range listAny(turn["steering"]) {
			entry, ok := rawEntry.(map[string]any)
			if legacy, isString := rawEntry.(string); isString {
				entries = append(entries, legacy)
				continue
			}
			if !ok {
				return nil, ErrForkBoundaryUnavailable
			}
			entries = append(entries, selectPublicFields(entry, []string{"id", "clientUserMessageId", "text", "displayText", "attachmentIds", "fileReferences", "admittedAt", "status", "promotedAt", "promotedItemId", "cancelledAt", "cancelReason", forkHistoryCancellationReasonV1, "kind", "delivery"}))
		}
		turn["steering"] = entries
	}
	seed := map[string]any{"schemaVersion": float64(1), "threadId": id, "parentThreadId": stringField(fork, "forkedFromThreadId"), "sourceThreadId": sourceThreadID, "turns": seedTurns}
	if sourceTurnID != "" {
		seed["sourceTurnId"] = sourceTurnID
		seed["sourceEndSeq"] = float64(sourceEndSeq)
	}
	event["inheritedHistorySeed"] = seed
	if _, err := validateForkHistorySeedV1(fork, event, seed); err != nil {
		return nil, err
	}
	return event, nil
}

func validateForkHistorySeedV1(source, event map[string]any, raw any) (map[string]any, error) {
	seed, ok := raw.(map[string]any)
	version, valid := contracts.NumericSeq(seed["schemaVersion"])
	seq, seqValid := contracts.NumericSeq(event["seq"])
	if !ok || !valid || version != 1 || stringField(seed, "sourceThreadId") == "" || !seqValid || seq != 1 || stringField(event, "kind") != "thread_created" ||
		stringField(seed, "threadId") != stringField(source, "id") || stringField(seed, "parentThreadId") != stringField(source, "forkedFromThreadId") ||
		stringField(event, "timestamp") != stringField(source, "createdAt") {
		return nil, ErrForkBoundaryUnavailable
	}
	for key := range seed {
		if key != "schemaVersion" && key != "threadId" && key != "parentThreadId" && key != "sourceThreadId" && key != "turns" && key != "sourceTurnId" && key != "sourceEndSeq" {
			return nil, ErrForkBoundaryUnavailable
		}
	}
	if _, present := seed["sourceTurnId"]; present {
		seq, ok := contracts.NumericSeq(seed["sourceEndSeq"])
		if !ok || seq <= 0 || int64(seq) > 1<<53-1 || strings.TrimSpace(stringField(seed, "sourceTurnId")) == "" {
			return nil, ErrForkBoundaryUnavailable
		}
	} else if _, present := seed["sourceEndSeq"]; present {
		return nil, ErrForkBoundaryUnavailable
	}
	turns, ok := seed["turns"].([]any)
	if !ok {
		return nil, ErrForkBoundaryUnavailable
	}
	seen := map[string]bool{}
	for _, rawTurn := range turns {
		turn, ok := rawTurn.(map[string]any)
		id := stringField(turn, "id")
		if !ok || id == "" || seen[id] || stringField(turn, "threadId") != stringField(source, "id") || !publicTerminalStatus(stringField(turn, "status")) ||
			!reflect.DeepEqual(contracts.CloneMap(turn), CloneTurnForThread(turn, stringField(source, "id"), stringField(source, "createdAt"))) {
			return nil, fmt.Errorf("%w: inherited seed is not inert", ErrForkBoundaryUnavailable)
		}
		seen[id] = true
	}
	if sourceTurnID := stringField(seed, "sourceTurnId"); sourceTurnID != "" {
		if len(turns) == 0 || stringField(turns[len(turns)-1].(map[string]any), "id") != sourceTurnID {
			return nil, ErrForkBoundaryUnavailable
		}
	}
	metadata := contracts.CloneMap(seed)
	delete(metadata, "turns")
	if err := domainevent.ValidatePublicRecord(metadata); err != nil {
		return nil, errors.Join(ErrForkBoundaryUnavailable, err)
	}
	for _, rawTurn := range turns {
		if err := validateForkSeedTurnV1(rawTurn.(map[string]any)); err != nil {
			return nil, err
		}
	}
	return contracts.CloneMap(seed), nil
}

// BuildForkCreatedEventsV1 uses the existing atomic log writer. The first
// record binds the exact deauthorized seed; bounded fragments close at E=N+2.
// Encoded bytes are private log material, never a public event projection.
func BuildForkCreatedEventsV1(fork map[string]any, sourceThreadID, sourceTurnID string, sourceEndSeq int) ([]map[string]any, error) {
	event, err := BuildForkCreatedEventV1(fork, sourceThreadID, sourceTurnID, sourceEndSeq)
	if err != nil {
		return nil, err
	}
	seed, ordinary := event["inheritedHistorySeed"].(map[string]any)
	if !ordinary {
		return []map[string]any{event}, domainevent.ValidatePublicRecord(event)
	}
	body, err := json.Marshal(seed)
	if err != nil {
		return nil, err
	}
	const fragmentBytes = 128 << 10
	count := (len(body) + fragmentBytes - 1) / fragmentBytes
	header := contracts.CloneMap(seed)
	delete(header, "turns")
	header["schemaVersion"], header["chunkCount"], header["historyBytes"] = float64(2), float64(count), float64(len(body))
	header["historySHA256"] = domainsecurity.SHA256Hex(body)
	header["turnCount"], header["itemCount"] = float64(len(listAny(seed["turns"]))), float64(forkSeedItemCountV1(seed))
	event["inheritedHistorySeed"] = header
	events := []map[string]any{event}
	for index, offset := 0, 0; offset < len(body); index, offset = index+1, offset+fragmentBytes {
		end := offset + fragmentBytes
		if end > len(body) {
			end = len(body)
		}
		part := contracts.CloneMap(event)
		part["seq"] = float64(index + 2)
		part["kind"] = "thread_fork_seed"
		delete(part, "status")
		delete(part, "inheritedHistorySeed")
		part["forkSeedFragment"] = map[string]any{"schemaVersion": float64(2), "index": float64(index + 1), "data": base64.StdEncoding.EncodeToString(body[offset:end])}
		events = append(events, part)
	}
	closeEvent := contracts.CloneMap(event)
	closeEvent["kind"], closeEvent["seq"] = "thread_fork_seed_completed", float64(count+2)
	delete(closeEvent, "status")
	delete(closeEvent, "inheritedHistorySeed")
	closeEvent["forkSeedClose"] = contracts.CloneMap(header)
	events = append(events, closeEvent)
	for _, part := range events {
		if err := domainevent.ValidatePublicRecord(part); err != nil {
			return nil, err
		}
	}
	return events, nil
}

func forkSeedItemCountV1(seed map[string]any) int {
	count := 0
	for _, raw := range listAny(seed["turns"]) {
		turn, _ := raw.(map[string]any)
		count += len(listAny(turn["items"]))
	}
	return count
}

// ReadForkCreatedSeedV1 rejects incomplete or reordered creation history. It
// validates both before encoding and after decoding; Base64 is no admission
// exception and cannot confer current source authority.
func ReadForkCreatedSeedV1(source map[string]any, events []map[string]any) (map[string]any, int, error) {
	if len(events) == 0 {
		return nil, 0, nil
	}
	raw, present := events[0]["inheritedHistorySeed"]
	if !present {
		for _, event := range events[1:] {
			if forkSeedRecordPresentV1(event) {
				return nil, 0, ErrForkBoundaryUnavailable
			}
		}
		return nil, 0, nil
	}
	header, ok := raw.(map[string]any)
	version, valid := contracts.NumericSeq(header["schemaVersion"])
	if !ok || !valid {
		return nil, 0, ErrForkBoundaryUnavailable
	}
	if version == 1 {
		for _, event := range events[1:] {
			if forkSeedRecordPresentV1(event) {
				return nil, 0, ErrForkBoundaryUnavailable
			}
		}
		seed, err := validateForkHistorySeedV1(source, events[0], header)
		return seed, 1, err
	}
	if version != 2 {
		return nil, 0, ErrForkBoundaryUnavailable
	}
	for key := range header {
		switch key {
		case "schemaVersion", "threadId", "parentThreadId", "sourceThreadId", "sourceTurnId", "sourceEndSeq", "chunkCount", "historyBytes", "historySHA256", "turnCount", "itemCount":
		default:
			return nil, 0, ErrForkBoundaryUnavailable
		}
	}
	count, cok := contracts.NumericSeq(header["chunkCount"])
	bytes, bok := contracts.NumericSeq(header["historyBytes"])
	turns, tok := contracts.NumericSeq(header["turnCount"])
	items, iok := contracts.NumericSeq(header["itemCount"])
	seq, sok := contracts.NumericSeq(events[0]["seq"])
	if !cok || !bok || !tok || !iok || !sok || seq != 1 || count <= 0 || count > len(events)-2 || bytes <= 0 || turns < 0 || items < 0 || stringField(header, "historySHA256") == "" {
		return nil, 0, ErrForkBoundaryUnavailable
	}
	body := []byte{}
	for index := 1; index <= count; index++ {
		event := events[index]
		part, ok := event["forkSeedFragment"].(map[string]any)
		partVersion, vok := contracts.NumericSeq(part["schemaVersion"])
		partIndex, pok := contracts.NumericSeq(part["index"])
		seq, sok := contracts.NumericSeq(event["seq"])
		data, dok := part["data"].(string)
		if !ok || len(part) != 3 || !vok || partVersion != 2 || !pok || partIndex != index || !sok || seq != index+1 || !dok || len(data) > base64.StdEncoding.EncodedLen(128<<10) || stringField(event, "kind") != "thread_fork_seed" || stringField(event, "threadId") != stringField(source, "id") || stringField(event, "timestamp") != stringField(source, "createdAt") {
			return nil, 0, ErrForkBoundaryUnavailable
		}
		if err := domainevent.ValidatePublicRecord(event); err != nil {
			return nil, 0, ErrForkBoundaryUnavailable
		}
		decoded, err := base64.StdEncoding.Strict().DecodeString(data)
		if err != nil || len(decoded) == 0 || len(decoded) > 128<<10 || (index < count && len(decoded) != 128<<10) {
			return nil, 0, ErrForkBoundaryUnavailable
		}
		body = append(body, decoded...)
		if len(body) > bytes {
			return nil, 0, ErrForkBoundaryUnavailable
		}
	}
	closing := events[count+1]
	closedSeq, closedOK := contracts.NumericSeq(closing["seq"])
	if !closedOK || closedSeq != count+2 || stringField(closing, "kind") != "thread_fork_seed_completed" || stringField(closing, "threadId") != stringField(source, "id") || stringField(closing, "timestamp") != stringField(source, "createdAt") || !reflect.DeepEqual(closing["forkSeedClose"], header) || domainevent.ValidatePublicRecord(closing) != nil {
		return nil, 0, ErrForkBoundaryUnavailable
	}
	for _, event := range events[count+2:] {
		if forkSeedRecordPresentV1(event) {
			return nil, 0, ErrForkBoundaryUnavailable
		}
	}
	if len(body) != bytes || domainsecurity.SHA256Hex(body) != stringField(header, "historySHA256") || domainevent.ValidatePublicRecord(events[0]) != nil {
		return nil, 0, ErrForkBoundaryUnavailable
	}
	decoded, err := domainjsonstrict.DecodeObject(body, domainjsonstrict.Options{MaxBytes: len(body)})
	if err != nil {
		return nil, 0, ErrForkBoundaryUnavailable
	}
	// JSON-number conversion here is data normalization only. All integers
	// have already been checked exactly against physical records above.
	normalized := contracts.CloneMap(decoded)
	if len(listAny(normalized["turns"])) != turns || forkSeedItemCountV1(normalized) != items {
		return nil, 0, ErrForkBoundaryUnavailable
	}
	for _, key := range []string{"threadId", "parentThreadId", "sourceThreadId", "sourceTurnId"} {
		if !reflect.DeepEqual(normalized[key], header[key]) {
			return nil, 0, ErrForkBoundaryUnavailable
		}
	}
	// The real JSONL decoder retains json.Number while the validated seed
	// normalizes data to float64. Bind E by its exact integer value before
	// that conversion; Go representation equality is not event identity.
	decodedEnd, decodedHasEnd := decoded["sourceEndSeq"]
	headerEnd, headerHasEnd := header["sourceEndSeq"]
	if decodedHasEnd != headerHasEnd {
		return nil, 0, ErrForkBoundaryUnavailable
	}
	if decodedHasEnd {
		decodedSeq, decodedOK := contracts.NumericSeq(decodedEnd)
		headerSeq, headerOK := contracts.NumericSeq(headerEnd)
		if !decodedOK || !headerOK || decodedSeq <= 0 || int64(decodedSeq) > 1<<53-1 || decodedSeq != headerSeq {
			return nil, 0, ErrForkBoundaryUnavailable
		}
	}
	seed, err := validateForkHistorySeedV1(source, events[0], normalized)
	return seed, count + 2, err
}

func forkSeedRecordPresentV1(event map[string]any) bool {
	return event["inheritedHistorySeed"] != nil || event["forkSeedFragment"] != nil || event["forkSeedClose"] != nil || stringField(event, "kind") == "thread_fork_seed" || stringField(event, "kind") == "thread_fork_seed_completed"
}

func validateForkSeedTurnV1(turn map[string]any) error {
	for key := range turn {
		switch key {
		case "id", "threadId", "status", "prompt", "model", "reasoningEffort", "steering", "createdAt", "startedAt", "finishedAt", "items", "attachmentIds", "activeSkillIds", "injectedMemoryIds", "skillInjectionBytes", "workspaceCheckpointId", "toolCatalogFingerprint", "toolCatalogToolCount", "toolCatalogDrift", "maxModelSteps", "guiPlan", "mode", "disableUserInput", "error":
		default:
			return ErrForkBoundaryUnavailable
		}
	}
	steering, ok := turn["steering"].([]any)
	if !ok {
		return ErrForkBoundaryUnavailable
	}
	view := contracts.CloneMap(turn)
	entries := []any{}
	for _, raw := range steering {
		if legacy, ok := raw.(string); ok {
			entries = append(entries, legacy)
			continue
		}
		entry, ok := raw.(map[string]any)
		if !ok {
			return ErrForkBoundaryUnavailable
		}
		for key := range entry {
			switch key {
			case "id", "clientUserMessageId", "text", "displayText", "attachmentIds", "fileReferences", "admittedAt", "status", "promotedAt", "promotedItemId", "cancelledAt", "cancelReason", forkHistoryCancellationReasonV1, "kind", "delivery":
			default:
				return ErrForkBoundaryUnavailable
			}
		}
		if err := validateForkHistoryCancellationV1(entry); err != nil {
			return err
		}
		// The creator persists an inert carrier only after source proof. A
		// Question-shaped string without that carrier stays ordinary text.
		if _, present := entry["kind"]; present {
			if entry["kind"] != "user_message" || entry["delivery"] != "steer" || domainevent.ValidateQuestionReplyContentV1(stringField(entry, "text")) != nil {
				return ErrForkBoundaryUnavailable
			}
		} else if _, present := entry["delivery"]; present {
			return ErrForkBoundaryUnavailable
		}
		entries = append(entries, contracts.CloneMap(entry))
	}
	view["steering"] = entries
	if err := domainevent.ValidatePublicRecord(view); err != nil {
		return errors.Join(ErrForkBoundaryUnavailable, err)
	}
	for _, raw := range listAny(turn["items"]) {
		item, ok := raw.(map[string]any)
		if !ok {
			return ErrForkBoundaryUnavailable
		}
		projected, ok := projectOrdinaryPublicHistoryItemV1(item)
		if !ok || !reflect.DeepEqual(projected, item) {
			return ErrForkBoundaryUnavailable
		}
	}
	return nil
}
