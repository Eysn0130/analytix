package checkpoint

import (
	"strings"

	domaincheckpointref "analytix.local/runtime-go/internal/domain/checkpointref"
)

type SnapshotDraft struct {
	Enabled                     bool
	AuthorityIntentID           string
	CheckpointID                string
	SourceWorkspaceCheckpointID string
	RelativePath                string
	BeforeHash                  string
	BeforeContent               string
	BeforeAvailable             bool
	BeforeExisted               bool
	CreatedAt                   string
}

type CapturedCheckpointInput struct {
	ThreadID                    string
	TurnID                      string
	CheckpointID                string
	SourceWorkspaceCheckpointID string
	WorkspaceFallback           string
	CreatedAtFallback           string
	Records                     []map[string]any
	CaptureEventID              string
}

func BuildCapturedCheckpointMetadata(input CapturedCheckpointInput) (map[string]any, bool) {
	if len(input.Records) == 0 {
		return nil, false
	}
	workspace := firstNonEmptyAnyString(input.Records[0]["workspace"], input.WorkspaceFallback)
	createdAt := firstNonEmptyAnyString(input.Records[0]["createdAt"], input.CreatedAtFallback)
	changedFiles := []any{}
	for _, record := range input.Records {
		relativePath := strings.TrimSpace(stringField(record, "relativePath"))
		if relativePath == "" {
			continue
		}
		file := map[string]any{
			"relativePath": relativePath,
			"changeKind":   NormalizeChangeKind(stringField(record, "changeKind")),
		}
		if version, ok := nonNegativeInteger(record["pathAuthoritySchemaVersion"]); ok && version == 1 {
			file["pathAuthoritySchemaVersion"] = float64(version)
			file["authorityKind"] = strings.TrimSpace(stringField(record, "authorityKind"))
			file["authorityRootHash"] = strings.TrimSpace(stringField(record, "authorityRootHash"))
		}
		if beforeHash := strings.TrimSpace(stringField(record, "beforeHash")); beforeHash != "" {
			file["beforeHash"] = beforeHash
		}
		if afterHash := strings.TrimSpace(stringField(record, "afterHash")); afterHash != "" {
			file["afterHash"] = afterHash
		}
		if record["generatedOfficeCreation"] == true {
			file["generatedOfficeCreation"] = true
		}
		changedFiles = append(changedFiles, file)
	}
	if len(changedFiles) == 0 {
		return nil, false
	}
	return map[string]any{
		"schemaVersion":               float64(1),
		"checkpointId":                strings.TrimSpace(input.CheckpointID),
		"sourceWorkspaceCheckpointId": strings.TrimSpace(input.SourceWorkspaceCheckpointID),
		"threadId":                    strings.TrimSpace(input.ThreadID),
		"turnId":                      strings.TrimSpace(input.TurnID),
		"workspace":                   workspace,
		"createdAt":                   createdAt,
		"status":                      "captured",
		"changedFiles":                changedFiles,
		"snapshotStorage":             "runtime_private_cas",
	}, true
}

func BuildCapturedCheckpointEvent(input CapturedCheckpointInput) (map[string]any, bool) {
	checkpoint, ok := BuildCapturedCheckpointMetadata(input)
	if !ok && input.CaptureEventID == "" {
		return nil, false
	}
	changedFileCount := 0
	createdAt := strings.TrimSpace(input.CreatedAtFallback)
	if ok {
		changedFiles, _ := checkpoint["changedFiles"].([]any)
		changedFileCount = len(changedFiles)
		createdAt = stringField(checkpoint, "createdAt")
	}
	closedCheckpoint := map[string]any{
		"schemaVersion":    float64(1),
		"checkpointId":     strings.TrimSpace(input.CheckpointID),
		"threadId":         strings.TrimSpace(input.ThreadID),
		"turnId":           strings.TrimSpace(input.TurnID),
		"createdAt":        createdAt,
		"status":           "captured",
		"changedFileCount": float64(changedFileCount),
		"snapshotStorage":  "runtime_private_cas",
	}
	if input.CaptureEventID != "" {
		if !domaincheckpointref.IsCaptureEventIDV2(input.CaptureEventID) {
			return nil, false
		}
		closedCheckpoint["captureEventId"] = input.CaptureEventID
		closedCheckpoint["capturePayloadDigest"] = domaincheckpointref.CapturedPayloadDigest(closedCheckpoint)
	}
	return map[string]any{
		"kind":       "checkpoint_captured",
		"threadId":   strings.TrimSpace(input.ThreadID),
		"turnId":     strings.TrimSpace(input.TurnID),
		"checkpoint": closedCheckpoint,
	}, true
}

func SnapshotEvidenceFromRecords(records []map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, record := range records {
		relativePath := strings.TrimSpace(stringField(record, "relativePath"))
		if relativePath == "" {
			continue
		}
		snapshot := map[string]any{"relativePath": relativePath}
		if before, ok := record["before"].(map[string]any); ok && before != nil {
			snapshot["before"] = cloneMap(before)
		}
		if after, ok := record["after"].(map[string]any); ok && after != nil {
			snapshot["after"] = cloneMap(after)
		}
		if version, ok := nonNegativeInteger(record["pathAuthoritySchemaVersion"]); ok && version == 1 {
			snapshot["pathAuthoritySchemaVersion"] = float64(version)
			snapshot["authorityKind"] = strings.TrimSpace(stringField(record, "authorityKind"))
			snapshot["authorityRoot"] = stringField(record, "authorityRoot")
			snapshot["authorityRootIdentity"] = stringField(record, "authorityRootIdentity")
			snapshot["authorityRootHash"] = strings.TrimSpace(stringField(record, "authorityRootHash"))
		}
		out[SnapshotEvidenceKey(snapshot)] = snapshot
	}
	return out
}

func SnapshotEvidenceKey(record map[string]any) string {
	relativePath := strings.TrimSpace(stringField(record, "relativePath"))
	version, ok := nonNegativeInteger(record["pathAuthoritySchemaVersion"])
	if !ok || version == 0 {
		return relativePath
	}
	return strings.TrimSpace(stringField(record, "authorityKind")) + "\x00" +
		strings.TrimSpace(stringField(record, "authorityRootHash")) + "\x00" + relativePath
}

func LatestCapturedCheckpointFromEvents(checkpointID string, events []map[string]any) (map[string]any, bool) {
	checkpointID = strings.TrimSpace(checkpointID)
	if checkpointID == "" {
		return nil, false
	}
	var checkpoint map[string]any
	for _, event := range events {
		if stringField(event, "kind") != "checkpoint_captured" {
			continue
		}
		candidate, _ := event["checkpoint"].(map[string]any)
		if candidate == nil || stringField(candidate, "checkpointId") != checkpointID {
			continue
		}
		checkpoint = cloneMap(candidate)
	}
	if checkpoint == nil {
		return nil, false
	}
	return checkpoint, true
}
