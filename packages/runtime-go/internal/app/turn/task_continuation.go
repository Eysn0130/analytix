package turn

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	threaddomain "analytix.local/runtime-go/internal/domain/thread"
)

const (
	maxContinuationGoalRunes       = 4000
	maxContinuationTodoRunes       = 1000
	maxContinuationConstraintRunes = 4000
)

func BuildTaskContinuationSnapshotV1(thread map[string]any) (threaddomain.TaskContinuationSnapshotV1, error) {
	snapshot := threaddomain.TaskContinuationSnapshotV1{
		SchemaVersion:         threaddomain.TaskContinuationSchemaVersionV1,
		Todos:                 []threaddomain.TaskContinuationTodoV1{},
		LatestUserConstraints: []string{},
		EvidenceReferences:    []threaddomain.TaskContinuationEvidenceReferenceV1{},
		EvidenceAuthority:     threaddomain.TaskContinuationEvidenceStateV1,
	}
	goal, _ := thread["goal"].(map[string]any)
	if goal != nil && strings.TrimSpace(stringField(goal, "status")) != "complete" {
		objective := continuationPublicTextV1(stringField(goal, "objective"), maxContinuationGoalRunes)
		goalID := strings.TrimSpace(stringField(goal, "id"))
		if goalID == "" {
			goalID = "goal_" + contracts.SafeRecordID(stringField(thread, "id"))
		}
		goalState := map[string]any{"goalId": goalID, "objective": objective, "status": strings.TrimSpace(stringField(goal, "status"))}
		snapshot.Goal = &threaddomain.TaskContinuationGoalV1{
			GoalID: goalID, Objective: objective, Status: strings.TrimSpace(stringField(goal, "status")), StateDigest: continuationStateDigestV1(goalState),
		}
		snapshot.EvidenceReferences = continuationEvidenceReferencesV1(goal)
	}
	if todos, _ := thread["todos"].(map[string]any); todos != nil {
		for _, raw := range listAny(todos["items"]) {
			item, _ := raw.(map[string]any)
			status := strings.TrimSpace(stringField(item, "status"))
			if item == nil || status == "completed" {
				continue
			}
			todoID := strings.TrimSpace(stringField(item, "id"))
			content := continuationPublicTextV1(stringField(item, "content"), maxContinuationTodoRunes)
			reason := strings.TrimSpace(stringField(item, "statusReasonCode"))
			state := map[string]any{"todoId": todoID, "content": content, "status": status}
			if reason != "" {
				state["statusReasonCode"] = reason
			}
			snapshot.Todos = append(snapshot.Todos, threaddomain.TaskContinuationTodoV1{
				TodoID: todoID, Content: content, Status: status, StatusReasonCode: reason, StateDigest: continuationStateDigestV1(state),
			})
		}
	}
	snapshot.LatestUserConstraints = latestContinuationUserConstraintsV1(thread)
	snapshot.PreviousContinuationDigest, snapshot.PreviousCompactionSourceDigest = previousContinuationDigestsV1(thread)
	return threaddomain.SealTaskContinuationSnapshotV1(snapshot)
}

func continuationEvidenceReferencesV1(goal map[string]any) []threaddomain.TaskContinuationEvidenceReferenceV1 {
	ledger := listAny(goal["evidenceLedger"])
	if len(ledger) > 32 {
		ledger = ledger[len(ledger)-32:]
	}
	out := make([]threaddomain.TaskContinuationEvidenceReferenceV1, 0, len(ledger))
	seen := map[string]bool{}
	for _, raw := range ledger {
		entry, _ := raw.(map[string]any)
		if entry == nil {
			continue
		}
		material := strings.TrimSpace(stringField(entry, "id"))
		if material == "" {
			body, _ := json.Marshal(entry)
			material = string(body)
		}
		digest := domainsecurity.SHA256Hex([]byte("analytix.task-continuation-evidence/v1\x00" + material))
		if seen[digest] {
			continue
		}
		seen[digest] = true
		out = append(out, threaddomain.TaskContinuationEvidenceReferenceV1{
			ReferenceDigest: digest, SupportStatus: threaddomain.TaskContinuationEvidenceStateV1,
		})
	}
	return out
}

func latestContinuationUserConstraintsV1(thread map[string]any) []string {
	constraints := []string{}
	for _, rawTurn := range listAny(thread["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		for _, rawItem := range listAny(turn["items"]) {
			item, _ := rawItem.(map[string]any)
			if stringField(item, "kind") != "user_message" {
				continue
			}
			if text := continuationPublicTextV1(stringField(item, "text"), maxContinuationConstraintRunes); text != "" {
				constraints = append(constraints, text)
			}
		}
	}
	if len(constraints) > 4 {
		constraints = constraints[len(constraints)-4:]
	}
	return constraints
}

func previousContinuationDigestsV1(thread map[string]any) (string, string) {
	turns := listAny(thread["turns"])
	for turnIndex := len(turns) - 1; turnIndex >= 0; turnIndex-- {
		turn, _ := turns[turnIndex].(map[string]any)
		items := listAny(turn["items"])
		for itemIndex := len(items) - 1; itemIndex >= 0; itemIndex-- {
			item, _ := items[itemIndex].(map[string]any)
			if stringField(item, "kind") != "compaction" {
				continue
			}
			sourceDigest := strings.TrimSpace(stringField(item, "sourceDigest"))
			if !domainsecurity.IsSHA256Hex(sourceDigest) {
				sourceDigest = ""
			}
			continuation, err := threaddomain.ParseTaskContinuationSnapshotV1(item["taskContinuation"])
			if err == nil {
				return continuation.StateDigest, sourceDigest
			}
			return "", sourceDigest
		}
	}
	return "", ""
}

func continuationPublicTextV1(value string, maxRunes int) string {
	value = strings.TrimSpace(privacyprojectionapp.ProjectOrdinaryText(value))
	if value == "" {
		return ""
	}
	filtered, err := domainevent.FilterPublicText(value)
	if err != nil || filtered != value {
		return "[user text omitted: reserved provider markup]"
	}
	if utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:maxRunes]))
}

func continuationStateDigestV1(value any) string {
	body, _ := json.Marshal(value)
	return domainsecurity.CanonicalJSONHash(body)
}
