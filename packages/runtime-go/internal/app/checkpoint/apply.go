package checkpoint

import (
	"encoding/base64"
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
)

type RestoreFilePlanInput struct {
	Changed   map[string]any
	PathError string
}

type ApplyResponseInput struct {
	ThreadID     string
	CheckpointID string
	Workspace    string
	PlanID       string
	Scope        string
	CreatedAt    string
	Status       string
	Files        []map[string]any
	Plan         map[string]any
	Rescue       map[string]any
	AuditSeq     int
}

type RescueFile struct {
	RelativePath               string
	PathAuthoritySchemaVersion int
	AuthorityKind              string
	AuthorityRoot              string
	AuthorityRootIdentity      string
	AuthorityRootHash          string
	Existed                    bool
	Hash                       string
	Content                    string
	Encoding                   string
	RawBytes                   []byte
	Error                      string
}

type RescueRecordInput struct {
	ThreadID     string
	CheckpointID string
	PlanID       string
	Workspace    string
	CreatedAt    string
	Files        []RescueFile
}

type ApplyFileState struct {
	AbsolutePath      string
	PathError         string
	MutationPathError string
	HasStagedChanges  bool
	Exists            bool
	IsDir             bool
	StatError         string
	ReadError         string
	Current           FileContent
}

type ApplyFilePreflight struct {
	Plan                       map[string]any
	RelativePath               string
	Action                     string
	Status                     string
	Reason                     string
	AbsolutePath               string
	PathAuthoritySchemaVersion int
	AuthorityKind              string
	AuthorityRoot              string
	AuthorityRootIdentity      string
	AuthorityRootHash          string
	CurrentHash                string
	TargetContent              string
	TargetBytes                []byte
	TargetEncoding             string
	BeforeHash                 string
	AfterHash                  string
}

func BuildRestoreFilePlan(input RestoreFilePlanInput) map[string]any {
	changed := input.Changed
	relativePath := strings.TrimSpace(stringField(changed, "relativePath"))
	changeKind := NormalizeChangeKind(stringField(changed, "changeKind"))
	beforeHash := strings.TrimSpace(stringField(changed, "beforeHash"))
	afterHash := strings.TrimSpace(stringField(changed, "afterHash"))
	file := map[string]any{
		"relativePath":  relativePath,
		"changeKind":    changeKind,
		"action":        "manual_review",
		"status":        "manual_review",
		"risk":          "high",
		"reason":        "checkpoint change requires manual review",
		"contentSource": "checkpoint_metadata_only",
	}
	if beforeHash != "" {
		file["beforeHash"] = beforeHash
	}
	if afterHash != "" {
		file["afterHash"] = afterHash
	}
	if version, ok := nonNegativeInteger(changed["pathAuthoritySchemaVersion"]); ok && version == 1 {
		file["pathAuthoritySchemaVersion"] = float64(version)
		file["authorityKind"] = strings.TrimSpace(stringField(changed, "authorityKind"))
		file["authorityRootHash"] = strings.TrimSpace(stringField(changed, "authorityRootHash"))
	}
	if pathError := strings.TrimSpace(input.PathError); pathError != "" {
		file["action"] = "blocked"
		file["status"] = "blocked"
		file["reason"] = pathError
		return file
	}
	if changed["generatedOfficeCreation"] == true {
		file["reason"] = "generated Office file deletion requires manual review because binary checkpoint rescue is not available"
		return file
	}
	switch changeKind {
	case "created":
		file["action"] = "delete_created_file"
		file["status"] = "ready"
		file["risk"] = "medium"
		file["reason"] = "checkpoint captured a file created after the boundary"
	case "modified":
		file["action"] = "restore_previous_version"
		if beforeHash != "" && afterHash != "" {
			file["status"] = "ready"
			file["risk"] = "medium"
			file["reason"] = "runtime private snapshot can restore the previous file content"
		} else {
			file["status"] = "manual_review"
			file["risk"] = "high"
			file["reason"] = "modified file lacks complete before/after hash evidence"
		}
	case "deleted":
		file["action"] = "restore_deleted_file"
		if beforeHash != "" {
			file["status"] = "ready"
			file["risk"] = "medium"
			file["reason"] = "runtime private snapshot can restore the deleted file"
		} else {
			file["status"] = "manual_review"
			file["risk"] = "high"
			file["reason"] = "deleted file lacks before snapshot evidence"
		}
	default:
		file["action"] = "manual_review"
		file["status"] = "manual_review"
		file["risk"] = "high"
		file["reason"] = "checkpoint change kind is unknown"
	}
	return file
}

func BuildApplyFilePreflight(plan map[string]any, snapshot map[string]any, state ApplyFileState) ApplyFilePreflight {
	relativePath := strings.TrimSpace(stringField(plan, "relativePath"))
	action := strings.TrimSpace(stringField(plan, "action"))
	beforeHash := strings.TrimSpace(stringField(plan, "beforeHash"))
	afterHash := strings.TrimSpace(stringField(plan, "afterHash"))
	base := ApplyFilePreflight{
		Plan:         cloneMap(plan),
		RelativePath: relativePath,
		Action:       action,
		Status:       "blocked",
		Reason:       "checkpoint file restore is blocked",
		AbsolutePath: strings.TrimSpace(state.AbsolutePath),
		BeforeHash:   beforeHash,
		AfterHash:    afterHash,
	}
	if pathError := strings.TrimSpace(state.PathError); pathError != "" {
		base.Reason = pathError
		return base
	}
	if status := strings.TrimSpace(stringField(plan, "status")); status == "blocked" || status == "manual_review" {
		base.Status = status
		base.Reason = firstNonEmptyAnyString(plan["reason"], "checkpoint file is "+status)
		return base
	}
	if action == "blocked" || action == "manual_review" {
		base.Status = action
		base.Reason = firstNonEmptyAnyString(plan["reason"], "checkpoint file action is "+action)
		return base
	}
	if mutationError := strings.TrimSpace(state.MutationPathError); mutationError != "" {
		base.Reason = mutationError
		return base
	}
	if state.HasStagedChanges {
		base.Reason = "current workspace path has staged git changes"
		return base
	}
	switch action {
	case "noop":
		base.Status = "noop"
		base.Reason = "checkpoint plan requires no file mutation"
		return base
	case "delete_created_file":
		return preflightDeleteCreatedFile(base, state)
	case "restore_previous_version":
		return preflightRestorePreviousVersion(base, snapshot, state)
	case "restore_deleted_file":
		return preflightRestoreDeletedFile(base, snapshot, state)
	default:
		base.Status = "manual_review"
		base.Reason = firstNonEmptyAnyString(plan["reason"], "checkpoint file action requires manual review")
		return base
	}
}

func preflightDeleteCreatedFile(base ApplyFilePreflight, state ApplyFileState) ApplyFilePreflight {
	if errText := strings.TrimSpace(state.StatError); errText != "" {
		base.Reason = errText
		return base
	}
	if !state.Exists {
		base.Status = "noop"
		base.Reason = "created file is already absent"
		return base
	}
	if state.IsDir {
		base.Reason = "current workspace path is a directory"
		return base
	}
	if base.AfterHash == "" {
		base.Reason = "delete_created_file requires afterHash evidence"
		return base
	}
	if errText := strings.TrimSpace(state.ReadError); errText != "" {
		base.Reason = errText
		return base
	}
	base.CurrentHash = state.Current.Hash
	if state.Current.Hash != base.AfterHash {
		base.Reason = "current file hash does not match checkpoint-created hash"
		return base
	}
	base.Status = "apply"
	base.Reason = "created file hash matches checkpoint-created hash"
	return base
}

func preflightRestorePreviousVersion(base ApplyFilePreflight, snapshot map[string]any, state ApplyFileState) ApplyFilePreflight {
	before, ok, reason := SnapshotContent(snapshot, "before", DefaultSnapshotMaxBytes)
	if !ok {
		base.Reason = firstNonEmptyAnyString(reason, "restore_previous_version requires before snapshot evidence")
		return base
	}
	if base.BeforeHash != "" && before.Hash != base.BeforeHash {
		base.Reason = "before snapshot hash does not match rewind plan beforeHash"
		return base
	}
	if errText := strings.TrimSpace(state.StatError); errText != "" {
		base.Reason = errText
		return base
	}
	if !state.Exists {
		base.Status = "manual_review"
		base.Reason = "current file is missing before modified restore"
		return base
	}
	if state.IsDir {
		base.Reason = "current workspace path is a directory"
		return base
	}
	if errText := strings.TrimSpace(state.ReadError); errText != "" {
		base.Reason = errText
		return base
	}
	base.CurrentHash = state.Current.Hash
	if state.Current.Hash == before.Hash {
		base.Status = "noop"
		base.Reason = "current file already matches before snapshot"
		return base
	}
	if base.AfterHash == "" {
		base.Reason = "restore_previous_version requires afterHash evidence"
		return base
	}
	if state.Current.Hash != base.AfterHash {
		base.Reason = "current file hash differs from checkpoint afterHash"
		return base
	}
	base.Status = "apply"
	base.Reason = "current file matches checkpoint afterHash and can restore before snapshot"
	base.TargetContent = before.Content
	base.TargetBytes = append([]byte(nil), before.RawBytes...)
	base.TargetEncoding = before.Encoding
	return base
}

func preflightRestoreDeletedFile(base ApplyFilePreflight, snapshot map[string]any, state ApplyFileState) ApplyFilePreflight {
	before, ok, reason := SnapshotContent(snapshot, "before", DefaultSnapshotMaxBytes)
	if !ok {
		base.Reason = firstNonEmptyAnyString(reason, "restore_deleted_file requires before snapshot evidence")
		return base
	}
	if base.BeforeHash != "" && before.Hash != base.BeforeHash {
		base.Reason = "before snapshot hash does not match rewind plan beforeHash"
		return base
	}
	if errText := strings.TrimSpace(state.StatError); errText != "" {
		base.Reason = errText
		return base
	}
	if !state.Exists {
		base.Status = "apply"
		base.Reason = "deleted file can be restored from before snapshot"
		base.TargetContent = before.Content
		base.TargetBytes = append([]byte(nil), before.RawBytes...)
		base.TargetEncoding = before.Encoding
		return base
	}
	if state.IsDir {
		base.Reason = "current workspace path is a directory"
		return base
	}
	if errText := strings.TrimSpace(state.ReadError); errText != "" {
		base.Reason = errText
		return base
	}
	base.CurrentHash = state.Current.Hash
	if state.Current.Hash == before.Hash {
		base.Status = "noop"
		base.Reason = "deleted file is already restored"
		return base
	}
	base.Reason = "current file exists and differs from before snapshot"
	return base
}

func ApplyFilePreflightResult(file ApplyFilePreflight) map[string]any {
	result := map[string]any{
		"relativePath": file.RelativePath,
		"action":       file.Action,
		"status":       file.Status,
		"reason":       file.Reason,
	}
	if file.BeforeHash != "" {
		result["beforeHash"] = file.BeforeHash
	}
	if file.AfterHash != "" {
		result["afterHash"] = file.AfterHash
	}
	if file.CurrentHash != "" {
		result["currentHash"] = file.CurrentHash
	}
	return result
}

func BuildApplyResponse(input ApplyResponseInput) map[string]any {
	planID := strings.TrimSpace(input.PlanID)
	if planID == "" {
		planID = PlanID(input.CheckpointID)
	}
	scope := strings.TrimSpace(input.Scope)
	if scope == "" {
		scope = "combined"
	}
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = "blocked"
	}
	out := map[string]any{
		"schemaVersion": float64(1),
		"applyId":       ApplyID(input.CheckpointID),
		"planId":        planID,
		"checkpointId":  strings.TrimSpace(input.CheckpointID),
		"threadId":      strings.TrimSpace(input.ThreadID),
		"workspace":     strings.TrimSpace(input.Workspace),
		"createdAt":     strings.TrimSpace(input.CreatedAt),
		"scope":         scope,
		"status":        status,
		"destructive":   true,
		"files":         mapsListAny(input.Files),
		"conversation":  ConversationApply(input.Plan, status),
		"summary":       ApplySummary(input.Files),
	}
	if input.Rescue != nil {
		if summary, ok := BuildRescueSummary(input.Rescue, 0); ok {
			out["rescue"] = BuildRescueResponseSummary(summary)
		}
	}
	if input.AuditSeq > 0 {
		out["auditEventSeq"] = float64(input.AuditSeq)
		if conversation, ok := out["conversation"].(map[string]any); ok && conversation["status"] == "audit_recorded" {
			conversation["auditEventSeq"] = float64(input.AuditSeq)
		}
	}
	return out
}

func BuildRescueCreatedEvent(threadID string, rescue map[string]any) map[string]any {
	summary, ok := BuildRescueSummary(rescue, 0)
	if !ok || exactString(summary, "threadId") != threadID {
		return nil
	}
	return map[string]any{
		"kind":     "checkpoint_rewind_rescue_created",
		"threadId": threadID,
		"rescue":   summary,
	}
}

func BuildRewindAppliedEvent(threadID string, apply map[string]any) map[string]any {
	summary, ok := BuildApplyAuditSummary(apply)
	if !ok || exactString(summary, "threadId") != threadID {
		return nil
	}
	return map[string]any{
		"kind":     "checkpoint_rewind_applied",
		"threadId": threadID,
		"apply":    summary,
	}
}

// BuildRescueSummary projects a private rescue record into the only shape
// allowed in HTTP responses and the ordinary durable event stream. File
// names, hashes, content, workspace paths, and read errors stay in the private
// sidecar.
func BuildRescueSummary(rescue map[string]any, eventSeq int) (map[string]any, bool) {
	if !schemaVersionOne(rescue) {
		return nil, false
	}
	checkpointID := exactString(rescue, "checkpointId")
	threadID := exactString(rescue, "threadId")
	planID := exactString(rescue, "planId")
	rescueID := exactString(rescue, "rescueId")
	createdAt := exactString(rescue, "createdAt")
	if !prefixedIdentifier(checkpointID, "axcp_") || threadID == "" ||
		planID != PlanID(checkpointID) || rescueID != RescueID(checkpointID) || createdAt == "" {
		return nil, false
	}
	fileCount := -1
	if files, ok := rescue["files"].([]any); ok {
		for _, raw := range files {
			if _, ok := raw.(map[string]any); !ok {
				return nil, false
			}
		}
		fileCount = len(files)
	} else if count, ok := nonNegativeInteger(rescue["fileCount"]); ok {
		fileCount = count
	}
	if fileCount < 0 {
		return nil, false
	}
	out := map[string]any{
		"schemaVersion": float64(1),
		"rescueId":      rescueID,
		"planId":        planID,
		"checkpointId":  checkpointID,
		"threadId":      threadID,
		"createdAt":     createdAt,
		"fileCount":     float64(fileCount),
		"storage":       "runtime_private_sidecar",
	}
	if eventSeq <= 0 {
		if stored, ok := nonNegativeInteger(rescue["eventSeq"]); ok && stored > 0 {
			eventSeq = stored
		}
	}
	if eventSeq > 0 {
		out["eventSeq"] = float64(eventSeq)
	}
	return out, true
}

func BuildRescueResponseSummary(rescue map[string]any) map[string]any {
	out := map[string]any{
		"rescueId":  exactString(rescue, "rescueId"),
		"fileCount": rescue["fileCount"],
	}
	if eventSeq, ok := nonNegativeInteger(rescue["eventSeq"]); ok && eventSeq > 0 {
		out["eventSeq"] = float64(eventSeq)
	}
	return out
}

// BuildApplyAuditSummary keeps rewind audit records useful without retaining
// the operational response's workspace, file paths, hashes, reasons, or
// conversation removal details in events.jsonl.
func BuildApplyAuditSummary(apply map[string]any) (map[string]any, bool) {
	if !schemaVersionOne(apply) {
		return nil, false
	}
	checkpointID := exactString(apply, "checkpointId")
	threadID := exactString(apply, "threadId")
	planID := exactString(apply, "planId")
	applyID := exactString(apply, "applyId")
	createdAt := exactString(apply, "createdAt")
	status := exactString(apply, "status")
	scope := exactString(apply, "scope")
	destructive, destructiveOK := apply["destructive"].(bool)
	if !prefixedIdentifier(checkpointID, "axcp_") || threadID == "" ||
		planID != PlanID(checkpointID) || applyID != ApplyID(checkpointID) || createdAt == "" ||
		!oneOfExact(status, "applied", "blocked", "failed", "already_applied") ||
		!oneOfExact(scope, "code", "conversation", "combined") || !destructiveOK || !destructive {
		return nil, false
	}
	files, ok := apply["files"].([]any)
	if !ok {
		return nil, false
	}
	for _, raw := range files {
		file, ok := raw.(map[string]any)
		if !ok || exactString(file, "relativePath") == "" ||
			!oneOfExact(exactString(file, "status"), "applied", "noop", "manual_review", "blocked", "failed") {
			return nil, false
		}
	}
	conversation, ok := apply["conversation"].(map[string]any)
	if !ok {
		return nil, false
	}
	conversationStatus := exactString(conversation, "status")
	if !oneOfExact(conversationStatus, "not_requested", "audit_recorded", "blocked", "already_applied") {
		return nil, false
	}
	out := map[string]any{
		"schemaVersion":      float64(1),
		"applyId":            applyID,
		"planId":             planID,
		"checkpointId":       checkpointID,
		"threadId":           threadID,
		"createdAt":          createdAt,
		"scope":              scope,
		"status":             status,
		"destructive":        true,
		"fileCount":          float64(len(files)),
		"conversationStatus": conversationStatus,
		"summary":            ApplySummary(mapsFromAny(files)),
	}
	return out, true
}

func ApplyResponseWithAuditEventSeq(apply map[string]any, event map[string]any) map[string]any {
	if apply == nil {
		return nil
	}
	if seq, ok := contracts.NumericSeq(event["seq"]); ok && seq > 0 {
		apply["auditEventSeq"] = float64(seq)
		if conversation, ok := apply["conversation"].(map[string]any); ok && conversation["status"] == "audit_recorded" {
			conversation["auditEventSeq"] = float64(seq)
		}
	}
	return apply
}

func BuildBlockedApply(threadID, checkpointID, workspace, planID, scope, createdAt, reason string) map[string]any {
	return BuildApplyResponse(ApplyResponseInput{
		ThreadID:     threadID,
		CheckpointID: checkpointID,
		Workspace:    workspace,
		PlanID:       planID,
		Scope:        scope,
		CreatedAt:    createdAt,
		Status:       "blocked",
		Plan: map[string]any{
			"conversation": map[string]any{
				"status": "blocked",
				"reason": strings.TrimSpace(reason),
			},
		},
	})
}

func BuildRescueRecord(input RescueRecordInput) map[string]any {
	files := []map[string]any{}
	for _, file := range input.Files {
		entry := map[string]any{
			"relativePath": strings.TrimSpace(file.RelativePath),
			"existed":      file.Existed,
		}
		if file.PathAuthoritySchemaVersion == 1 {
			entry["pathAuthoritySchemaVersion"] = float64(1)
			entry["authorityKind"] = strings.TrimSpace(file.AuthorityKind)
			entry["authorityRoot"] = file.AuthorityRoot
			entry["authorityRootIdentity"] = strings.TrimSpace(file.AuthorityRootIdentity)
			entry["authorityRootHash"] = strings.TrimSpace(file.AuthorityRootHash)
		}
		if file.Existed {
			if hash := strings.TrimSpace(file.Hash); hash != "" {
				entry["hash"] = hash
			}
			if file.RawBytes != nil {
				entry["snapshotSchemaVersion"] = float64(1)
				entry["bytesBase64"] = base64.StdEncoding.EncodeToString(file.RawBytes)
				entry["encoding"] = strings.TrimSpace(file.Encoding)
			} else {
				entry["content"] = file.Content
				entry["encoding"] = "utf8"
			}
		}
		if errText := strings.TrimSpace(file.Error); errText != "" {
			entry["error"] = errText
		}
		files = append(files, entry)
	}
	return map[string]any{
		"schemaVersion": float64(1),
		"rescueId":      RescueID(input.CheckpointID),
		"planId":        strings.TrimSpace(input.PlanID),
		"checkpointId":  strings.TrimSpace(input.CheckpointID),
		"threadId":      strings.TrimSpace(input.ThreadID),
		"workspace":     strings.TrimSpace(input.Workspace),
		"createdAt":     strings.TrimSpace(input.CreatedAt),
		"files":         mapsListAny(files),
	}
}

func exactString(record map[string]any, key string) string {
	if record == nil {
		return ""
	}
	value, ok := record[key].(string)
	if !ok || value == "" || strings.TrimSpace(value) != value {
		return ""
	}
	return value
}

func schemaVersionOne(record map[string]any) bool {
	value, ok := nonNegativeInteger(record["schemaVersion"])
	return ok && value == 1
}

func prefixedIdentifier(value, prefix string) bool {
	if !strings.HasPrefix(value, prefix) || len(value) <= len(prefix) {
		return false
	}
	for _, character := range value[len(prefix):] {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func nonNegativeInteger(value any) (int, bool) {
	parsed, ok := contracts.NumericSeq(value)
	return parsed, ok && parsed >= 0
}

func oneOfExact(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func mapsFromAny(values []any) []map[string]any {
	out := make([]map[string]any, 0, len(values))
	for _, raw := range values {
		mapped, _ := raw.(map[string]any)
		out = append(out, mapped)
	}
	return out
}
