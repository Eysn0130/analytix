package goal

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainprivacy "analytix.local/runtime-go/internal/domain/privacyprojection"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type StatePatchInput struct {
	ThreadID string
	Existing map[string]any
	Patch    map[string]any
	Todos    map[string]any
	Now      string
}

type EvidenceAppendInput struct {
	ThreadID string
	Goal     map[string]any
	Entry    map[string]any
	Now      string
}

func ApplyStatePatch(input StatePatchInput) (map[string]any, error) {
	threadID := strings.TrimSpace(input.ThreadID)
	now := strings.TrimSpace(input.Now)
	patch := contracts.CloneMap(input.Patch)
	goal := projectGoalRecord(contracts.CloneMap(input.Existing))
	if goal == nil {
		objective := projectGoalText(strings.TrimSpace(stringField(patch, "objective")))
		if objective == "" {
			objective = "Continue current objective"
		}
		goal = map[string]any{
			"id":              "goal_" + contracts.SafeRecordID(threadID),
			"threadId":        threadID,
			"objective":       objective,
			"status":          "active",
			"tokenBudget":     nil,
			"tokensUsed":      float64(0),
			"timeUsedSeconds": float64(0),
			"evidenceLedger":  []any{},
			"createdAt":       now,
			"updatedAt":       now,
		}
	}
	if strings.TrimSpace(stringField(goal, "id")) == "" {
		goal["id"] = "goal_" + contracts.SafeRecordID(threadID)
	}
	if objective := projectGoalText(strings.TrimSpace(stringField(patch, "objective"))); objective != "" {
		goal["objective"] = objective
	}
	if status := strings.TrimSpace(stringField(patch, "status")); status != "" {
		goal["status"] = status
	}
	if tokenBudget, ok := patch["tokenBudget"]; ok {
		goal["tokenBudget"] = contracts.CloneValue(tokenBudget)
	}
	for _, key := range []string{"strictCompletion", "selfCheckRequired", "selfCheckCompleted"} {
		if value, ok := patch[key].(bool); ok {
			goal[key] = value
		}
	}
	for _, key := range []string{"selfCheckTurnId", "blockedReason", "blockedTurnId"} {
		if value := strings.TrimSpace(stringField(patch, key)); value != "" {
			if key == "blockedReason" {
				value = projectGoalText(value)
			}
			goal[key] = value
		}
	}
	if blockedCount, ok := positiveIntAny(patch["blockedCount"]); ok {
		goal["blockedCount"] = float64(blockedCount)
	}
	if research, ok := patch["research"].(map[string]any); ok && research["enabled"] == true {
		safeThreadID := contracts.SafeRecordID(threadID)
		goal["research"] = map[string]any{
			"enabled":          true,
			"stateRefDigest":   domainsecurity.SHA256Hex([]byte("analytix/autoresearch-state-ref/v1\x00" + safeThreadID)),
			"requirementCount": float64(len(listAny(research["requirements"]))),
		}
	}
	if stringField(goal, "status") == "complete" {
		if len(listAny(goal["evidenceLedger"])) == 0 {
			return nil, errors.New("cannot mark goal complete without evidence; call complete_step first")
		}
		if input.Todos != nil && incompleteTodos(input.Todos) > 0 {
			return nil, errors.New("cannot mark goal complete while todos are still incomplete")
		}
		if goal["strictCompletion"] == true && goal["selfCheckCompleted"] != true {
			return nil, errors.New("cannot mark strict goal complete until a completion self-check has been recorded")
		}
	}
	goal["updatedAt"] = now
	return goal, nil
}

func AppendEvidence(input EvidenceAppendInput) (map[string]any, map[string]any, error) {
	goal := projectGoalRecord(contracts.CloneMap(input.Goal))
	if goal == nil {
		return nil, nil, errors.New("cannot record goal evidence because this thread does not have a goal")
	}
	if stringField(goal, "status") != "active" {
		return nil, nil, fmt.Errorf("cannot record goal evidence because the current goal is %s, not active", stringField(goal, "status"))
	}
	entry := contracts.CloneMap(input.Entry)
	step := truncateText(projectGoalText(strings.TrimSpace(stringField(entry, "step"))), 1000)
	if step == "" {
		return nil, nil, errors.New("step is required")
	}
	evidence := EvidenceStrings(entry["evidence"])
	if len(evidence) == 0 {
		return nil, nil, errors.New("evidence must include at least one concrete non-empty item")
	}
	now := strings.TrimSpace(input.Now)
	ledger := append([]any(nil), listAny(goal["evidenceLedger"])...)
	record := map[string]any{
		"id":        fmt.Sprintf("goal_ev_%d", len(ledger)+1),
		"step":      step,
		"evidence":  evidence,
		"createdAt": now,
	}
	for _, key := range []string{"turnId", "toolCallId", "requirementId"} {
		if value := strings.TrimSpace(stringField(entry, key)); value != "" {
			record[key] = value
		}
	}
	if summary := truncateText(projectGoalText(strings.TrimSpace(stringField(entry, "summary"))), 2000); summary != "" {
		record["summary"] = summary
	}
	if details, ok := entry["evidenceDetails"].([]any); ok && len(details) > 0 {
		projected, _ := domainprivacy.ProjectPublicValue(details)
		record["evidenceDetails"] = projected
	}
	ledger = append(ledger, record)
	if len(ledger) > 500 {
		ledger = ledger[len(ledger)-500:]
	}
	goal["evidenceLedger"] = ledger
	goal["updatedAt"] = now
	return goal, record, nil
}

func EvidenceStrings(value any) []any {
	out := []any{}
	for _, raw := range listAny(value) {
		text := ""
		if item, ok := raw.(map[string]any); ok {
			text = strings.TrimSpace(stringField(item, "summary"))
			if text == "" {
				text = strings.TrimSpace(stringField(item, "command"))
			}
			if text == "" {
				text = strings.TrimSpace(stringField(item, "kind"))
			}
		} else {
			text = strings.TrimSpace(fmt.Sprint(raw))
		}
		text = truncateText(projectGoalText(text), 2000)
		if text == "" {
			continue
		}
		out = append(out, text)
		if len(out) >= 20 {
			break
		}
	}
	return out
}

func projectGoalRecord(goal map[string]any) map[string]any {
	if goal == nil {
		return nil
	}
	projected, _ := domainprivacy.ProjectPublicValue(goal)
	out, _ := projected.(map[string]any)
	return out
}

func projectGoalText(text string) string {
	return domainprivacy.ProjectText(text).Text
}

func positiveIntAny(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, typed > 0
	case int64:
		return int(typed), typed > 0
	case float64:
		if typed <= 0 || typed != float64(int(typed)) {
			return 0, false
		}
		return int(typed), true
	case json.Number:
		number, err := typed.Int64()
		if err == nil && number > 0 {
			return int(number), true
		}
	}
	return 0, false
}
