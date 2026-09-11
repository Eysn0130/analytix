package checkpointauthority

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	SnapshotIntentSchemaVersion      = 1
	SnapshotIntentPurpose            = "analytix.checkpoint-snapshot-intent/v1"
	SnapshotCompletionSchemaVersion  = 1
	SnapshotCompletionPurpose        = "analytix.checkpoint-snapshot-completion/v1"
	SnapshotDispositionSchemaVersion = 1
	SnapshotDispositionPurpose       = "analytix.checkpoint-snapshot-disposition/v1"
	MaxSnapshotAuthorityRecordBytes  = 2 * 1024 * 1024
	MaxSnapshotContentBytes          = 512 * 1024
)

type SnapshotIntentV1 struct {
	SchemaVersion               int                                `json:"schemaVersion"`
	Purpose                     string                             `json:"purpose"`
	SnapshotIntentID            string                             `json:"snapshotIntentId"`
	SecurityContext             domainsecurity.TurnSecurityContext `json:"securityContext"`
	ExecutionGrant              domainsecurity.ExecutionGrant      `json:"executionGrant"`
	CheckpointID                string                             `json:"checkpointId"`
	SourceWorkspaceCheckpointID string                             `json:"sourceWorkspaceCheckpointId"`
	RelativePath                string                             `json:"relativePath"`
	MutationOrdinal             uint64                             `json:"mutationOrdinal"`
	BeforeExisted               bool                               `json:"beforeExisted"`
	BeforeAvailable             bool                               `json:"beforeAvailable"`
	BeforeHash                  string                             `json:"beforeHash,omitempty"`
	BeforeContent               string                             `json:"beforeContent,omitempty"`
	CreatedAt                   string                             `json:"createdAt"`
}

type SnapshotIntentInputV1 struct {
	SecurityContext             domainsecurity.TurnSecurityContext
	ExecutionGrant              domainsecurity.ExecutionGrant
	CheckpointID                string
	SourceWorkspaceCheckpointID string
	RelativePath                string
	MutationOrdinal             uint64
	BeforeExisted               bool
	BeforeAvailable             bool
	BeforeHash                  string
	BeforeContent               string
	CreatedAt                   time.Time
}

type SnapshotCompletionV1 struct {
	SchemaVersion    int    `json:"schemaVersion"`
	Purpose          string `json:"purpose"`
	SnapshotIntentID string `json:"snapshotIntentId"`
	CompletionDigest string `json:"completionDigest"`
	ThreadID         string `json:"threadId"`
	TurnID           string `json:"turnId"`
	ContextDigest    string `json:"contextDigest"`
	CheckpointID     string `json:"checkpointId"`
	RelativePath     string `json:"relativePath"`
	MutationOrdinal  uint64 `json:"mutationOrdinal"`
	AfterExisted     bool   `json:"afterExisted"`
	AfterHash        string `json:"afterHash,omitempty"`
	ChangeKind       string `json:"changeKind"`
	CompletedAt      string `json:"completedAt"`
}

type SnapshotCompletionInputV1 struct {
	Intent       SnapshotIntentV1
	AfterExisted bool
	AfterHash    string
	CompletedAt  time.Time
}

type SnapshotDispositionV1 struct {
	SchemaVersion     int    `json:"schemaVersion"`
	Purpose           string `json:"purpose"`
	SnapshotIntentID  string `json:"snapshotIntentId"`
	DispositionDigest string `json:"dispositionDigest"`
	ThreadID          string `json:"threadId"`
	TurnID            string `json:"turnId"`
	ContextDigest     string `json:"contextDigest"`
	CheckpointID      string `json:"checkpointId"`
	RelativePath      string `json:"relativePath"`
	MutationOrdinal   uint64 `json:"mutationOrdinal"`
	Status            string `json:"status"`
	ReasonCode        string `json:"reasonCode"`
	ClosedAt          string `json:"closedAt"`
}

type SnapshotDispositionInputV1 struct {
	Intent     SnapshotIntentV1
	ReasonCode string
	ClosedAt   time.Time
}

type MaterializedSnapshotV1 struct {
	Intent     SnapshotIntentV1
	Completion SnapshotCompletionV1
}

func NewSnapshotIntentV1(input SnapshotIntentInputV1) (SnapshotIntentV1, error) {
	record := SnapshotIntentV1{
		SchemaVersion: SnapshotIntentSchemaVersion, Purpose: SnapshotIntentPurpose,
		SecurityContext: input.SecurityContext, ExecutionGrant: input.ExecutionGrant,
		CheckpointID:                strings.TrimSpace(input.CheckpointID),
		SourceWorkspaceCheckpointID: strings.TrimSpace(input.SourceWorkspaceCheckpointID),
		RelativePath:                strings.TrimSpace(input.RelativePath), MutationOrdinal: input.MutationOrdinal,
		BeforeExisted: input.BeforeExisted, BeforeAvailable: input.BeforeAvailable,
		BeforeHash: strings.TrimSpace(input.BeforeHash), BeforeContent: input.BeforeContent,
		CreatedAt: input.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	record.SnapshotIntentID = snapshotIntentDigest(record)
	if err := ValidateSnapshotIntentV1(record); err != nil {
		return SnapshotIntentV1{}, err
	}
	return record, nil
}

func ValidateSnapshotIntentV1(record SnapshotIntentV1) error {
	createdAt, timeErr := time.Parse(time.RFC3339Nano, record.CreatedAt)
	if record.SchemaVersion != SnapshotIntentSchemaVersion || record.Purpose != SnapshotIntentPurpose ||
		domainsecurity.ValidateTurnSecurityContextForExecution(record.SecurityContext) != nil ||
		domainsecurity.ValidateExecutionGrantForContext(record.ExecutionGrant, record.SecurityContext) != nil ||
		record.ExecutionGrant.ReadOnly || record.ExecutionGrant.ToolCallID == "" ||
		!canonicalCheckpointID(record.CheckpointID) || !canonicalSourceCheckpointID(record.SourceWorkspaceCheckpointID) ||
		!canonicalRelativePath(record.RelativePath) || record.MutationOrdinal == 0 || timeErr != nil ||
		createdAt.Location() != time.UTC || createdAt.Format(time.RFC3339Nano) != record.CreatedAt ||
		!domainsecurity.IsSHA256Hex(record.SnapshotIntentID) || record.SnapshotIntentID != snapshotIntentDigest(record) {
		return errors.New("checkpoint snapshot intent integrity is invalid")
	}
	if record.BeforeAvailable {
		if !record.BeforeExisted || len(record.BeforeContent) > MaxSnapshotContentBytes || !utf8.ValidString(record.BeforeContent) ||
			!domainsecurity.IsSHA256Hex(record.BeforeHash) || domainsecurity.SHA256Hex([]byte(record.BeforeContent)) != record.BeforeHash {
			return errors.New("checkpoint snapshot before content integrity is invalid")
		}
	} else if record.BeforeHash != "" || record.BeforeContent != "" {
		return errors.New("checkpoint unavailable before content is not canonical")
	}
	return nil
}

func NewSnapshotCompletionV1(input SnapshotCompletionInputV1) (SnapshotCompletionV1, error) {
	intent := input.Intent
	changeKind := "created"
	if intent.BeforeExisted && input.AfterExisted {
		changeKind = "modified"
	} else if intent.BeforeExisted && !input.AfterExisted {
		changeKind = "deleted"
	}
	record := SnapshotCompletionV1{
		SchemaVersion: SnapshotCompletionSchemaVersion, Purpose: SnapshotCompletionPurpose,
		SnapshotIntentID: intent.SnapshotIntentID, ThreadID: intent.SecurityContext.ThreadID,
		TurnID: intent.SecurityContext.TurnID, ContextDigest: intent.SecurityContext.ContextDigest,
		CheckpointID: intent.CheckpointID, RelativePath: intent.RelativePath, MutationOrdinal: intent.MutationOrdinal,
		AfterExisted: input.AfterExisted, AfterHash: strings.TrimSpace(input.AfterHash), ChangeKind: changeKind,
		CompletedAt: input.CompletedAt.UTC().Format(time.RFC3339Nano),
	}
	record.CompletionDigest = snapshotCompletionDigest(record)
	if err := ValidateSnapshotCompletionForIntentV1(record, intent); err != nil {
		return SnapshotCompletionV1{}, err
	}
	return record, nil
}

func ValidateSnapshotCompletionForIntentV1(record SnapshotCompletionV1, intent SnapshotIntentV1) error {
	completedAt, timeErr := time.Parse(time.RFC3339Nano, record.CompletedAt)
	expectedKind := "created"
	if intent.BeforeExisted && record.AfterExisted {
		expectedKind = "modified"
	} else if intent.BeforeExisted && !record.AfterExisted {
		expectedKind = "deleted"
	}
	if ValidateSnapshotIntentV1(intent) != nil || record.SchemaVersion != SnapshotCompletionSchemaVersion ||
		record.Purpose != SnapshotCompletionPurpose || record.SnapshotIntentID != intent.SnapshotIntentID ||
		record.ThreadID != intent.SecurityContext.ThreadID || record.TurnID != intent.SecurityContext.TurnID ||
		record.ContextDigest != intent.SecurityContext.ContextDigest || record.CheckpointID != intent.CheckpointID ||
		record.RelativePath != intent.RelativePath || record.MutationOrdinal != intent.MutationOrdinal ||
		record.ChangeKind != expectedKind || timeErr != nil || completedAt.Location() != time.UTC ||
		completedAt.Format(time.RFC3339Nano) != record.CompletedAt || !domainsecurity.IsSHA256Hex(record.CompletionDigest) ||
		record.CompletionDigest != snapshotCompletionDigest(record) {
		return errors.New("checkpoint snapshot completion integrity is invalid")
	}
	if record.AfterExisted {
		if !domainsecurity.IsSHA256Hex(record.AfterHash) {
			return errors.New("checkpoint snapshot after hash is invalid")
		}
	} else if record.AfterHash != "" {
		return errors.New("checkpoint missing after state is not canonical")
	}
	return nil
}

func NewSnapshotDispositionV1(input SnapshotDispositionInputV1) (SnapshotDispositionV1, error) {
	intent := input.Intent
	record := SnapshotDispositionV1{
		SchemaVersion: SnapshotDispositionSchemaVersion, Purpose: SnapshotDispositionPurpose,
		SnapshotIntentID: intent.SnapshotIntentID, ThreadID: intent.SecurityContext.ThreadID,
		TurnID: intent.SecurityContext.TurnID, ContextDigest: intent.SecurityContext.ContextDigest,
		CheckpointID: intent.CheckpointID, RelativePath: intent.RelativePath, MutationOrdinal: intent.MutationOrdinal,
		Status: "aborted", ReasonCode: strings.TrimSpace(input.ReasonCode),
		ClosedAt: input.ClosedAt.UTC().Format(time.RFC3339Nano),
	}
	record.DispositionDigest = snapshotDispositionDigest(record)
	if err := ValidateSnapshotDispositionForIntentV1(record, intent); err != nil {
		return SnapshotDispositionV1{}, err
	}
	return record, nil
}

func ValidateSnapshotDispositionForIntentV1(record SnapshotDispositionV1, intent SnapshotIntentV1) error {
	closedAt, timeErr := time.Parse(time.RFC3339Nano, record.ClosedAt)
	if ValidateSnapshotIntentV1(intent) != nil || record.SchemaVersion != SnapshotDispositionSchemaVersion ||
		record.Purpose != SnapshotDispositionPurpose || record.SnapshotIntentID != intent.SnapshotIntentID ||
		record.ThreadID != intent.SecurityContext.ThreadID || record.TurnID != intent.SecurityContext.TurnID ||
		record.ContextDigest != intent.SecurityContext.ContextDigest || record.CheckpointID != intent.CheckpointID ||
		record.RelativePath != intent.RelativePath || record.MutationOrdinal != intent.MutationOrdinal ||
		record.Status != "aborted" || record.ReasonCode != "mutation_failed" || timeErr != nil ||
		closedAt.Location() != time.UTC || closedAt.Format(time.RFC3339Nano) != record.ClosedAt ||
		!domainsecurity.IsSHA256Hex(record.DispositionDigest) || record.DispositionDigest != snapshotDispositionDigest(record) {
		return errors.New("checkpoint snapshot disposition integrity is invalid")
	}
	return nil
}

func SnapshotIntentV1Bytes(record SnapshotIntentV1) ([]byte, error) {
	if err := ValidateSnapshotIntentV1(record); err != nil {
		return nil, err
	}
	return json.Marshal(record)
}

func SnapshotCompletionV1Bytes(record SnapshotCompletionV1, intent SnapshotIntentV1) ([]byte, error) {
	if err := ValidateSnapshotCompletionForIntentV1(record, intent); err != nil {
		return nil, err
	}
	return json.Marshal(record)
}

func SnapshotDispositionV1Bytes(record SnapshotDispositionV1, intent SnapshotIntentV1) ([]byte, error) {
	if err := ValidateSnapshotDispositionForIntentV1(record, intent); err != nil {
		return nil, err
	}
	return json.Marshal(record)
}

func ParseSnapshotIntentV1(body []byte) (SnapshotIntentV1, error) {
	var record SnapshotIntentV1
	if err := decodeStrictSnapshotRecord(body, &record); err != nil {
		return SnapshotIntentV1{}, err
	}
	return record, ValidateSnapshotIntentV1(record)
}

func ParseSnapshotCompletionV1(body []byte, intent SnapshotIntentV1) (SnapshotCompletionV1, error) {
	var record SnapshotCompletionV1
	if err := decodeStrictSnapshotRecord(body, &record); err != nil {
		return SnapshotCompletionV1{}, err
	}
	return record, ValidateSnapshotCompletionForIntentV1(record, intent)
}

func ParseSnapshotDispositionV1(body []byte, intent SnapshotIntentV1) (SnapshotDispositionV1, error) {
	var record SnapshotDispositionV1
	if err := decodeStrictSnapshotRecord(body, &record); err != nil {
		return SnapshotDispositionV1{}, err
	}
	return record, ValidateSnapshotDispositionForIntentV1(record, intent)
}

func snapshotIntentDigest(record SnapshotIntentV1) string {
	record.SnapshotIntentID = ""
	body, _ := json.Marshal(record)
	return domainsecurity.SHA256Hex(body)
}

func snapshotCompletionDigest(record SnapshotCompletionV1) string {
	record.CompletionDigest = ""
	body, _ := json.Marshal(record)
	return domainsecurity.SHA256Hex(body)
}

func snapshotDispositionDigest(record SnapshotDispositionV1) string {
	record.DispositionDigest = ""
	body, _ := json.Marshal(record)
	return domainsecurity.SHA256Hex(body)
}

func decodeStrictSnapshotRecord(body []byte, target any) error {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: MaxSnapshotAuthorityRecordBytes, MaxDepth: 32,
		MaxTokens: 16_000, MaxStringBytes: MaxSnapshotContentBytes + 64*1024,
	}); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("checkpoint snapshot authority contains trailing JSON")
	}
	return nil
}

func canonicalCheckpointID(value string) bool {
	return canonicalASCIIID(value, "axcp_")
}

func canonicalSourceCheckpointID(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= 512
}

func canonicalASCIIID(value, prefix string) bool {
	if value == "" || value != strings.TrimSpace(value) || !strings.HasPrefix(value, prefix) || len(value) <= len(prefix) {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func canonicalRelativePath(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && !strings.Contains(value, "\\") &&
		!strings.HasPrefix(value, "/") && path.Clean(value) == value && value != "." && value != ".." &&
		!strings.HasPrefix(value, "../")
}
