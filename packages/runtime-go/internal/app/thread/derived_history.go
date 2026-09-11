package thread

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"time"
)

// ValidateCaseDerivedHistoryTurnV1 checks original bytes without projecting
// away failures. It proves an inert case-derivation shape, not provenance of
// its text. Callers must separately bind signed lineage, absence of target
// execution records, and the complete primary digest before using this check.
func ValidateCaseDerivedHistoryTurnV1(threadID string, turn map[string]any) error {
	turnID := stringField(turn, "id")
	if turnID == "" || turnID != strings.TrimSpace(turnID) || stringField(turn, "threadId") != threadID || !publicTerminalStatus(stringField(turn, "status")) {
		return errors.New("derived history turn identity or terminal status is invalid")
	}
	if _, err := time.Parse(time.RFC3339Nano, stringField(turn, "finishedAt")); err != nil {
		return errors.New("derived history terminal time is invalid")
	}
	// Fork/resume retain normal start and terminal metadata at the root.
	// Unknown root fields cannot silently acquire an authority interpretation.
	for field := range turn {
		switch field {
		case "id", "threadId", "status", "createdAt", "startedAt", "finishedAt", "items", "caseHistoryProjection",
			"prompt", "model", "reasoningEffort", "steering", "attachmentIds", "attachments", "fileReferences", "workspaceCheckpointId",
			"activeSkillIds", "injectedMemoryIds", "skillInjectionBytes", "approvalPolicy", "sandboxMode", "mode", "guiPlan", "disableUserInput", "maxModelSteps",
			"usage", "cacheDiagnostics", "usageSource", "usageFinalStatus", "acceptedFinalView", "discard", "cancelled", "cancelledPendingGates":
		default:
			return errors.New("derived history contains an unexpected turn field")
		}
	}
	items, ok := turn["items"].([]any)
	if !ok {
		return errors.New("derived history item inventory is invalid")
	}
	if projection, present := turn["caseHistoryProjection"]; present {
		if err := validateCompactedDerivedHistoryShapeV1(turn, items, projection); err != nil {
			return err
		}
	}
	seen := map[string]bool{}
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok || stringField(item, "kind") != "user_message" || stringField(item, "role") != "user" || stringField(item, "status") != "completed" || stringField(item, "threadId") != threadID || stringField(item, "turnId") != turnID {
			return errors.New("derived history contains an execution item or wrong binding")
		}
		id := stringField(item, "id")
		if id == "" || id != strings.TrimSpace(id) || seen[id] {
			return errors.New("derived history item identity is invalid")
		}
		seen[id] = true
		for field := range item {
			if field != "threadId" && field != "turnId" && field != "role" && !slices.Contains(caseDerivedUserHistoryFieldsV1, field) {
				return errors.New("derived history retained source execution authority")
			}
		}
	}
	// Check the producer's structure without cloning numeric history values:
	// strict primary readers retain json.Number, while the producer's JSON clone
	// uses float64. Neither type conversion nor rounding is an authority check.
	if !reflect.DeepEqual(turn["attachmentIds"], AttachmentIDsFromItems(items)) {
		return errors.New("derived history attachment inventory differs from user items")
	}
	return nil
}

// These are the closed shapes emitted by BuildCompaction and retained by
// fork/resume after execution authority is stripped. A marker grants nothing:
// the caller still proves Original lineage and absence of execution identities.
func validateCompactedDerivedHistoryShapeV1(turn map[string]any, items []any, value any) error {
	projection, ok := value.(string)
	if !ok || (projection != "compaction_authority_v1" && projection != "authority_only_v1" && projection != "user_only_untrusted_v1") {
		return errors.New("derived history compaction projection is invalid")
	}
	for field, value := range turn {
		switch field {
		case "items", "attachmentIds":
			// The shared isolation check validates the exact item inventory and
			// attachment IDs derived from it, including empty authority skeletons.
			continue
		case "id", "threadId", "status", "createdAt", "startedAt", "finishedAt", "caseHistoryProjection":
		case "model":
			if projection == "user_only_untrusted_v1" {
				return errors.New("derived compaction tail contains unexpected metadata")
			}
		case "prompt":
			if projection != "compaction_authority_v1" || value != "/compact" {
				return errors.New("derived compaction prompt is invalid")
			}
		default:
			return errors.New("derived compaction contains an unexpected field")
		}
		if _, ok := value.(string); !ok {
			return errors.New("derived compaction metadata is malformed")
		}
	}
	if projection != "user_only_untrusted_v1" && len(items) != 0 {
		return errors.New("derived compaction authority skeleton contains items")
	}
	if projection == "compaction_authority_v1" && (turn["prompt"] != "/compact" || turn["status"] != "completed" || turn["createdAt"] != turn["finishedAt"] || turn["startedAt"] != turn["finishedAt"]) {
		return errors.New("derived compaction authority skeleton is malformed")
	}
	if projection == "user_only_untrusted_v1" {
		for _, raw := range items {
			item, ok := raw.(map[string]any)
			if !ok {
				return errors.New("derived compaction user item is malformed")
			}
			for field := range item {
				switch field {
				case "id", "threadId", "turnId", "kind", "role", "text", "delivery", "status", "createdAt", "finishedAt", "attachmentIds":
				default:
					return errors.New("derived compaction user item contains an unexpected field")
				}
			}
		}
	}
	return nil
}
