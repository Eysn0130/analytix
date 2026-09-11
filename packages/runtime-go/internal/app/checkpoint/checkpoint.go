package checkpoint

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	contracts "analytix.local/runtime-go/internal/contracts"
	domaincheckpointref "analytix.local/runtime-go/internal/domain/checkpointref"
)

const DefaultSnapshotMaxBytes = 512 * 1024

type FileContent struct {
	Hash     string
	Content  string
	Encoding string
	RawBytes []byte
}

type AuditPlanInput struct {
	ThreadID     string
	CheckpointID string
	Workspace    string
	Scope        string
	CreatedAt    string
}

type PlanInput struct {
	ThreadID     string
	CheckpointID string
	Workspace    string
	Scope        string
	CreatedAt    string
	Checkpoint   map[string]any
	Files        []any
	Conversation map[string]any
}

func RuntimeCheckpointIDFromWorkspaceCheckpointID(value string) string {
	return domaincheckpointref.RuntimeID(value)
}

func PlanID(checkpointID string) string {
	return domaincheckpointref.PlanID(checkpointID)
}

func ApplyID(checkpointID string) string {
	return domaincheckpointref.ApplyID(checkpointID)
}

func RescueID(checkpointID string) string {
	return domaincheckpointref.RescueID(checkpointID)
}

func BuildAuditOnlyPlan(input AuditPlanInput) map[string]any {
	threadID := strings.TrimSpace(input.ThreadID)
	checkpointID := strings.TrimSpace(input.CheckpointID)
	createdAt := strings.TrimSpace(input.CreatedAt)
	workspace := strings.TrimSpace(input.Workspace)
	scope := strings.TrimSpace(input.Scope)
	plan := map[string]any{
		"schemaVersion": float64(1),
		"planId":        PlanID(checkpointID),
		"checkpointId":  checkpointID,
		"threadId":      threadID,
		"workspace":     workspace,
		"createdAt":     createdAt,
		"scope":         scope,
		"applyMode":     "plan_only",
		"destructive":   false,
		"checkpoint": map[string]any{
			"schemaVersion": float64(1),
			"checkpointId":  checkpointID,
			"threadId":      threadID,
			"workspace":     workspace,
			"createdAt":     createdAt,
			"status":        "captured",
			"changedFiles":  []any{},
		},
		"files": []any{},
		"conversation": map[string]any{
			"status":             "blocked",
			"reason":             "Go runtime default recorded checkpoint rewind as an audit-only boundary.",
			"retainedEventCount": float64(0),
			"removedEventCount":  float64(0),
			"removedTurnIds":     []any{},
			"projection": map[string]any{
				"latestSeq":       float64(0),
				"turnCount":       float64(0),
				"itemCount":       float64(0),
				"checkpointCount": float64(1),
			},
		},
		"summary": map[string]any{
			"fileCount":               float64(0),
			"readyFileCount":          float64(0),
			"manualReviewFileCount":   float64(0),
			"blockedFileCount":        float64(0),
			"retainedEventCount":      float64(0),
			"removedEventCount":       float64(0),
			"removedTurnCount":        float64(0),
			"containsRawPrompt":       false,
			"containsFullFileContent": false,
			"containsSecretValue":     false,
		},
	}
	plan["planDigest"] = PlanDigest(plan)
	return plan
}

func BuildPlan(input PlanInput) map[string]any {
	files := cloneAnyList(input.Files)
	ready, manual, blocked := 0, 0, 0
	for _, raw := range files {
		file, _ := raw.(map[string]any)
		switch stringField(file, "status") {
		case "ready":
			ready++
		case "manual_review":
			manual++
		case "blocked":
			blocked++
		}
	}
	plan := map[string]any{
		"schemaVersion": float64(1),
		"planId":        PlanID(input.CheckpointID),
		"checkpointId":  strings.TrimSpace(input.CheckpointID),
		"threadId":      strings.TrimSpace(input.ThreadID),
		"workspace":     strings.TrimSpace(input.Workspace),
		"createdAt":     strings.TrimSpace(input.CreatedAt),
		"scope":         strings.TrimSpace(input.Scope),
		"applyMode":     "plan_only",
		"destructive":   false,
		"checkpoint":    contracts.CloneMap(input.Checkpoint),
		"files":         files,
		"summary": map[string]any{
			"fileCount":               float64(len(files)),
			"readyFileCount":          float64(ready),
			"manualReviewFileCount":   float64(manual),
			"blockedFileCount":        float64(blocked),
			"retainedEventCount":      float64(0),
			"removedEventCount":       float64(0),
			"removedTurnCount":        float64(0),
			"containsRawPrompt":       false,
			"containsFullFileContent": false,
			"containsSecretValue":     false,
		},
	}
	if input.Conversation != nil {
		plan["conversation"] = contracts.CloneMap(input.Conversation)
	}
	plan["planDigest"] = PlanDigest(plan)
	return plan
}

func PlanDigest(plan map[string]any) string {
	canonical := cloneMap(plan)
	delete(canonical, "planDigest")
	body, err := json.Marshal(canonical)
	if err != nil {
		return ""
	}
	return Hash(string(body))
}

func NormalizeChangeKind(value string) string {
	switch strings.TrimSpace(value) {
	case "created", "modified", "deleted":
		return strings.TrimSpace(value)
	default:
		return "unknown"
	}
}

func ConversationPlanLoadError(reason string) map[string]any {
	return blockedConversationPlan(strings.TrimSpace(reason), 0)
}

func ConversationPlanFromEvents(checkpointID string, events []map[string]any) map[string]any {
	checkpointTurnID := ""
	checkpointSeq := 0
	for _, event := range events {
		if stringField(event, "kind") == "checkpoint_captured" {
			checkpoint, _ := event["checkpoint"].(map[string]any)
			if checkpoint != nil && stringField(checkpoint, "checkpointId") == strings.TrimSpace(checkpointID) {
				checkpointTurnID = stringField(checkpoint, "turnId")
				checkpointSeq = intNumber(event["seq"])
			}
		}
	}
	boundarySeq := 0
	if checkpointTurnID != "" {
		for _, event := range events {
			if stringField(event, "kind") == "turn_started" && stringField(event, "turnId") == checkpointTurnID {
				boundarySeq = intNumber(event["seq"])
				break
			}
		}
	}
	if checkpointTurnID == "" || boundarySeq == 0 {
		return blockedConversationPlan("checkpoint conversation boundary is unavailable", 0)
	}
	retained := 0
	removed := 0
	removedTurns := []string{}
	seenTurn := map[string]bool{}
	latestSeq := 0
	for _, event := range events {
		seq := intNumber(event["seq"])
		if seq < boundarySeq {
			retained++
			if seq > latestSeq {
				latestSeq = seq
			}
			continue
		}
		removed++
		turnID := strings.TrimSpace(stringField(event, "turnId"))
		if turnID != "" && !seenTurn[turnID] {
			seenTurn[turnID] = true
			removedTurns = append(removedTurns, turnID)
		}
	}
	return map[string]any{
		"status":             "ready",
		"boundaryTurnId":     checkpointTurnID,
		"checkpointEventSeq": float64(checkpointSeq),
		"boundarySeq":        float64(boundarySeq),
		"retainedEventCount": float64(retained),
		"removedEventCount":  float64(removed),
		"removedTurnIds":     StringListAny(removedTurns),
		"projection": map[string]any{
			"latestSeq":       float64(latestSeq),
			"turnCount":       float64(0),
			"itemCount":       float64(0),
			"checkpointCount": float64(1),
		},
	}
}

func blockedConversationPlan(reason string, latestSeq int) map[string]any {
	return map[string]any{
		"status":             "blocked",
		"reason":             reason,
		"retainedEventCount": float64(0),
		"removedEventCount":  float64(0),
		"removedTurnIds":     []any{},
		"projection": map[string]any{
			"latestSeq":       float64(latestSeq),
			"turnCount":       float64(0),
			"itemCount":       float64(0),
			"checkpointCount": float64(0),
		},
	}
}

func ApplyConfirmationValid(value any) bool {
	confirmation, _ := value.(map[string]any)
	return confirmation != nil &&
		boolFromAny(confirmation["confirmed"]) &&
		boolFromAny(confirmation["destructive"]) &&
		stringField(confirmation, "phrase") == "APPLY_CHECKPOINT_REWIND"
}

func BoolValue(value any) bool {
	return boolFromAny(value)
}

func SnapshotEvidence(value any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, item := range listAny(value) {
		snapshot, _ := item.(map[string]any)
		relativePath := strings.TrimSpace(stringField(snapshot, "relativePath"))
		if relativePath == "" {
			continue
		}
		out[relativePath] = contracts.CloneMap(snapshot)
	}
	return out
}

func SnapshotContent(snapshot map[string]any, key string, maxBytes int) (FileContent, bool, string) {
	if maxBytes <= 0 {
		maxBytes = DefaultSnapshotMaxBytes
	}
	raw, _ := snapshot[key].(map[string]any)
	if raw == nil {
		return FileContent{}, false, key + " snapshot evidence is missing"
	}
	encoding := strings.TrimSpace(stringField(raw, "encoding"))
	version, versionOK := contracts.NumericSeq(raw["schemaVersion"])
	if _, present := raw["schemaVersion"]; present && (!versionOK || version != 1) {
		return FileContent{}, false, key + " snapshot schema version is not supported for automatic checkpoint apply"
	}
	if versionOK && version == 1 {
		if !checkpointTextEncoding(encoding) {
			return FileContent{}, false, key + " snapshot encoding is not supported for automatic checkpoint apply"
		}
		encoded := stringField(raw, "bytesBase64")
		content, err := base64.StdEncoding.Strict().DecodeString(encoded)
		if err != nil {
			return FileContent{}, false, key + " snapshot bytes are not canonical base64"
		}
		if len(content) > maxBytes {
			return FileContent{}, false, fmt.Sprintf("%s snapshot exceeds %d byte automatic checkpoint apply limit", key, maxBytes)
		}
		hash := strings.TrimSpace(stringField(raw, "hash"))
		if hash == "" || HashBytes(content) != hash {
			return FileContent{}, false, key + " snapshot hash does not match bytes"
		}
		return FileContent{Hash: hash, Encoding: encoding, RawBytes: append([]byte(nil), content...)}, true, ""
	}
	if encoding != "utf8" {
		return FileContent{}, false, key + " snapshot encoding is not supported for automatic checkpoint apply"
	}
	content := stringField(raw, "content")
	if len([]byte(content)) > maxBytes {
		return FileContent{}, false, fmt.Sprintf("%s snapshot exceeds %d byte automatic checkpoint apply limit", key, maxBytes)
	}
	if !utf8.ValidString(content) {
		return FileContent{}, false, key + " snapshot is not valid UTF-8"
	}
	hash := strings.TrimSpace(stringField(raw, "hash"))
	if hash == "" || Hash(content) != hash {
		return FileContent{}, false, key + " snapshot hash does not match content"
	}
	return FileContent{Hash: hash, Content: content, Encoding: "utf8", RawBytes: []byte(content)}, true, ""
}

func checkpointTextEncoding(value string) bool {
	switch value {
	case "utf8", "utf8-bom", "utf16le", "utf16be", "utf16le-nobom", "utf16be-nobom":
		return true
	default:
		return false
	}
}

func AppliedReason(action string) string {
	switch action {
	case "delete_created_file":
		return "created file deleted"
	case "restore_previous_version":
		return "previous file version restored"
	case "restore_deleted_file":
		return "deleted file restored"
	default:
		return "checkpoint file mutation applied"
	}
}

func ApplySummary(files []map[string]any) map[string]any {
	summary := map[string]any{
		"fileAppliedCount":      float64(0),
		"fileNoopCount":         float64(0),
		"fileManualReviewCount": float64(0),
		"fileBlockedCount":      float64(0),
		"fileFailedCount":       float64(0),
	}
	for _, file := range files {
		switch stringField(file, "status") {
		case "applied":
			summary["fileAppliedCount"] = summary["fileAppliedCount"].(float64) + 1
		case "noop":
			summary["fileNoopCount"] = summary["fileNoopCount"].(float64) + 1
		case "manual_review":
			summary["fileManualReviewCount"] = summary["fileManualReviewCount"].(float64) + 1
		case "blocked":
			summary["fileBlockedCount"] = summary["fileBlockedCount"].(float64) + 1
		case "failed":
			summary["fileFailedCount"] = summary["fileFailedCount"].(float64) + 1
		}
	}
	return summary
}

func ConversationApply(plan map[string]any, applyStatus string) map[string]any {
	conversation, _ := plan["conversation"].(map[string]any)
	if conversation == nil {
		return map[string]any{"status": "not_requested"}
	}
	if applyStatus == "applied" {
		return map[string]any{
			"status":             "audit_recorded",
			"boundaryTurnId":     stringField(conversation, "boundaryTurnId"),
			"retainedEventCount": float64(intNumber(conversation["retainedEventCount"])),
			"removedEventCount":  float64(intNumber(conversation["removedEventCount"])),
			"removedTurnIds":     StringListAny(stringList(conversation["removedTurnIds"])),
		}
	}
	return map[string]any{
		"status": "blocked",
		"reason": firstNonEmptyAnyString(conversation["reason"], "checkpoint conversation rewind is blocked"),
	}
}

func Hash(content string) string {
	return HashBytes([]byte(content))
}

func HashBytes(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func StringListAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func mapsListAny(values []map[string]any) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, cloneMap(value))
	}
	return out
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	return contracts.CloneMap(value)
}

func cloneAnyList(items []any) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		if mapped, ok := item.(map[string]any); ok {
			out = append(out, contracts.CloneMap(mapped))
			continue
		}
		out = append(out, contracts.CloneValue(item))
	}
	return out
}

func stringField(record map[string]any, key string) string {
	return contracts.StringField(record, key)
}

func listAny(value any) []any {
	if items, ok := value.([]any); ok {
		return items
	}
	return []any{}
}

func boolFromAny(value any) bool {
	boolValue, _ := value.(bool)
	return boolValue
}

func intNumber(value any) int {
	number, _ := contracts.NumericSeq(value)
	return number
}

func stringList(value any) []string {
	out := []string{}
	for _, raw := range listAny(value) {
		text := strings.TrimSpace(fmt.Sprint(raw))
		if text != "" {
			out = append(out, text)
		}
	}
	return out
}

func firstNonEmptyAnyString(values ...any) string {
	for _, value := range values {
		text := strings.TrimSpace(fmt.Sprint(value))
		if text != "" && text != "<nil>" {
			return text
		}
	}
	return ""
}
