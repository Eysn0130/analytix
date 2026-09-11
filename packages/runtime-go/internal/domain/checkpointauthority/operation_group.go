package checkpointauthority

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"path"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	domaincheckpointref "analytix.local/runtime-go/internal/domain/checkpointref"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	OperationGroupIntentSchemaVersion   = 2
	OperationGroupIntentPurpose         = "analytix.checkpoint-operation-group-intent/v2"
	OperationGroupTerminalSchemaVersion = 2
	OperationGroupTerminalPurpose       = "analytix.checkpoint-operation-group-terminal/v2"
	MaxOperationArgumentsBytes          = 1024 * 1024
	maxOperationSnapshotStringBytes     = 4*((MaxSnapshotContentBytes+2)/3) + 64*1024
)

type OperationPathV2 struct {
	ArgumentKey                 string `json:"argumentKey"`
	RequestedPath               string `json:"requestedPath"`
	PathAuthoritySchemaVersion  int    `json:"pathAuthoritySchemaVersion,omitempty"`
	AuthorityKind               string `json:"authorityKind,omitempty"`
	AuthorityRoot               string `json:"authorityRoot,omitempty"`
	AuthorityRootIdentity       string `json:"authorityRootIdentity,omitempty"`
	AuthorityRootHash           string `json:"authorityRootHash,omitempty"`
	RelativePath                string `json:"relativePath"`
	Role                        string `json:"role"`
	BeforeExisted               bool   `json:"beforeExisted"`
	BeforeAvailable             bool   `json:"beforeAvailable"`
	BeforeHash                  string `json:"beforeHash,omitempty"`
	BeforeSizeBytes             int64  `json:"beforeSizeBytes"`
	BeforeSnapshotSchemaVersion int    `json:"beforeSnapshotSchemaVersion,omitempty"`
	BeforeEncoding              string `json:"beforeEncoding,omitempty"`
	BeforeBytesBase64           string `json:"beforeBytesBase64,omitempty"`
	BeforeContent               string `json:"beforeContent,omitempty"`
	ExpectedAfterExisted        bool   `json:"expectedAfterExisted"`
	ExpectedAfterHash           string `json:"expectedAfterHash,omitempty"`
}

type OperationPathInputV2 struct {
	ArgumentKey                 string
	RequestedPath               string
	PathAuthoritySchemaVersion  int
	AuthorityKind               string
	AuthorityRoot               string
	AuthorityRootIdentity       string
	AuthorityRootHash           string
	RelativePath                string
	Role                        string
	BeforeExisted               bool
	BeforeAvailable             bool
	BeforeHash                  string
	BeforeSizeBytes             int64
	BeforeSnapshotSchemaVersion int
	BeforeEncoding              string
	BeforeBytesBase64           string
	BeforeContent               string
	ExpectedAfterExisted        bool
	ExpectedAfterHash           string
}

type OperationGroupIntentV2 struct {
	SchemaVersion               int                                `json:"schemaVersion"`
	Purpose                     string                             `json:"purpose"`
	OperationGroupID            string                             `json:"operationGroupId"`
	IntentDigest                string                             `json:"intentDigest"`
	SecurityContext             domainsecurity.TurnSecurityContext `json:"securityContext"`
	ExecutionGrant              domainsecurity.ExecutionGrant      `json:"executionGrant"`
	CheckpointID                string                             `json:"checkpointId"`
	SourceWorkspaceCheckpointID string                             `json:"sourceWorkspaceCheckpointId"`
	OperationOrdinal            uint64                             `json:"operationOrdinal"`
	ToolName                    string                             `json:"toolName"`
	ArgumentsJSON               json.RawMessage                    `json:"argumentsJson"`
	Paths                       []OperationPathV2                  `json:"paths"`
	CreatedAt                   string                             `json:"createdAt"`
}

type OperationGroupIntentInputV2 struct {
	SecurityContext             domainsecurity.TurnSecurityContext
	ExecutionGrant              domainsecurity.ExecutionGrant
	CheckpointID                string
	SourceWorkspaceCheckpointID string
	OperationOrdinal            uint64
	ToolName                    string
	ArgumentsJSON               []byte
	Paths                       []OperationPathInputV2
	CreatedAt                   time.Time
}

type ObservedOperationPathV2 struct {
	PathAuthoritySchemaVersion int    `json:"pathAuthoritySchemaVersion,omitempty"`
	AuthorityKind              string `json:"authorityKind,omitempty"`
	AuthorityRootHash          string `json:"authorityRootHash,omitempty"`
	RelativePath               string `json:"relativePath"`
	ObservationStatus          string `json:"observationStatus"`
	BlockerCode                string `json:"blockerCode,omitempty"`
	Existed                    bool   `json:"existed"`
	Hash                       string `json:"hash,omitempty"`
}

type OperationGroupTerminalV2 struct {
	SchemaVersion    int                       `json:"schemaVersion"`
	Purpose          string                    `json:"purpose"`
	OperationGroupID string                    `json:"operationGroupId"`
	TerminalDigest   string                    `json:"terminalDigest"`
	ThreadID         string                    `json:"threadId"`
	TurnID           string                    `json:"turnId"`
	ContextDigest    string                    `json:"contextDigest"`
	CheckpointID     string                    `json:"checkpointId"`
	OperationOrdinal uint64                    `json:"operationOrdinal"`
	Status           string                    `json:"status"`
	ReasonCode       string                    `json:"reasonCode"`
	ObservedPaths    []ObservedOperationPathV2 `json:"observedPaths"`
	SettledAt        string                    `json:"settledAt"`
}

type OperationGroupTerminalInputV2 struct {
	Intent        OperationGroupIntentV2
	Status        string
	ReasonCode    string
	ObservedPaths []ObservedOperationPathV2
	SettledAt     time.Time
}

type MaterializedOperationGroupV2 struct {
	Intent   OperationGroupIntentV2
	Terminal OperationGroupTerminalV2
}

type OperationGroupStateV2 struct {
	Intent   OperationGroupIntentV2
	Terminal *OperationGroupTerminalV2
}

func NewOperationGroupIntentV2(input OperationGroupIntentInputV2) (OperationGroupIntentV2, error) {
	canonicalArguments, err := canonicalOperationArguments(input.ArgumentsJSON)
	if err != nil {
		return OperationGroupIntentV2{}, err
	}
	paths := make([]OperationPathV2, 0, len(input.Paths))
	for _, source := range input.Paths {
		pathAuthorityVersion := source.PathAuthoritySchemaVersion
		authorityKind := strings.TrimSpace(source.AuthorityKind)
		authorityRoot := source.AuthorityRoot
		authorityRootIdentity := strings.TrimSpace(source.AuthorityRootIdentity)
		authorityRootHash := strings.TrimSpace(source.AuthorityRootHash)
		beforeSnapshotVersion := source.BeforeSnapshotSchemaVersion
		beforeEncoding := strings.TrimSpace(source.BeforeEncoding)
		beforeBytesBase64 := source.BeforeBytesBase64
		beforeContent := source.BeforeContent
		beforeSizeBytes := source.BeforeSizeBytes
		if source.BeforeExisted && source.BeforeAvailable && beforeSnapshotVersion == 0 && beforeEncoding == "" && beforeBytesBase64 == "" {
			beforeSnapshotVersion = 1
			beforeEncoding = "utf8"
			beforeBytesBase64 = base64.StdEncoding.EncodeToString([]byte(beforeContent))
			beforeSizeBytes = int64(len([]byte(beforeContent)))
			beforeContent = ""
		}
		paths = append(paths, OperationPathV2{
			ArgumentKey: strings.TrimSpace(source.ArgumentKey), RequestedPath: source.RequestedPath,
			PathAuthoritySchemaVersion: pathAuthorityVersion,
			AuthorityKind:              authorityKind, AuthorityRoot: authorityRoot,
			AuthorityRootIdentity: authorityRootIdentity,
			AuthorityRootHash:     authorityRootHash,
			RelativePath:          strings.TrimSpace(source.RelativePath), Role: strings.TrimSpace(source.Role),
			BeforeExisted: source.BeforeExisted, BeforeAvailable: source.BeforeAvailable,
			BeforeHash: strings.TrimSpace(source.BeforeHash), BeforeSizeBytes: beforeSizeBytes,
			BeforeSnapshotSchemaVersion: beforeSnapshotVersion,
			BeforeEncoding:              beforeEncoding, BeforeBytesBase64: beforeBytesBase64,
			BeforeContent:        beforeContent,
			ExpectedAfterExisted: source.ExpectedAfterExisted, ExpectedAfterHash: strings.TrimSpace(source.ExpectedAfterHash),
		})
	}
	record := OperationGroupIntentV2{
		SchemaVersion: OperationGroupIntentSchemaVersion, Purpose: OperationGroupIntentPurpose,
		SecurityContext: input.SecurityContext, ExecutionGrant: input.ExecutionGrant,
		CheckpointID: strings.TrimSpace(input.CheckpointID), SourceWorkspaceCheckpointID: strings.TrimSpace(input.SourceWorkspaceCheckpointID),
		OperationOrdinal: input.OperationOrdinal, ToolName: strings.TrimSpace(input.ToolName),
		ArgumentsJSON: canonicalArguments, Paths: paths, CreatedAt: input.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	record.OperationGroupID = operationGroupIdentity(record)
	record.IntentDigest = operationGroupIntentDigest(record)
	if err := ValidateOperationGroupIntentV2(record); err != nil {
		return OperationGroupIntentV2{}, err
	}
	return record, nil
}

func ValidateOperationGroupIntentV2(record OperationGroupIntentV2) error {
	createdAt, createdErr := time.Parse(time.RFC3339Nano, record.CreatedAt)
	grantIssuedAt, issuedErr := time.Parse(time.RFC3339Nano, record.ExecutionGrant.IssuedAt)
	grantExpiresAt, expiresErr := time.Parse(time.RFC3339Nano, record.ExecutionGrant.ExpiresAt)
	canonicalArguments, argumentsErr := canonicalOperationArguments(record.ArgumentsJSON)
	if record.SchemaVersion != OperationGroupIntentSchemaVersion || record.Purpose != OperationGroupIntentPurpose ||
		domainsecurity.ValidateTurnSecurityContextForOrdinaryEffect(record.SecurityContext) != nil ||
		domainsecurity.ValidateExecutionGrantForOrdinaryContext(record.ExecutionGrant, record.SecurityContext) != nil ||
		record.ExecutionGrant.ServerIdentity != "host:builtin" || record.ExecutionGrant.ReadOnly ||
		(record.ExecutionGrant.ApprovalState != "approved" && record.ExecutionGrant.ApprovalState != "not_required") ||
		record.ExecutionGrant.ToolName != record.ToolName || !allowedMutationTool(record.ToolName) ||
		domainsecurity.CanonicalJSONHash(record.ArgumentsJSON) != record.ExecutionGrant.ArgsHash || argumentsErr != nil ||
		!bytes.Equal(canonicalArguments, record.ArgumentsJSON) || !domaincheckpointref.IsCanonicalRuntimeIDV2(record.CheckpointID) ||
		!canonicalSourceCheckpointID(record.SourceWorkspaceCheckpointID) || record.OperationOrdinal == 0 || len(record.Paths) == 0 ||
		createdErr != nil || issuedErr != nil || expiresErr != nil || createdAt.Location() != time.UTC ||
		createdAt.Format(time.RFC3339Nano) != record.CreatedAt || createdAt.Before(grantIssuedAt) || createdAt.After(grantExpiresAt) ||
		!domainsecurity.IsSHA256Hex(record.OperationGroupID) || record.OperationGroupID != operationGroupIdentity(record) ||
		!domainsecurity.IsSHA256Hex(record.IntentDigest) || record.IntentDigest != operationGroupIntentDigest(record) {
		return errors.New("checkpoint operation group intent integrity is invalid")
	}
	arguments, ok := operationArgumentsObject(record.ArgumentsJSON)
	if !ok || !validOperationPaths(record.ToolName, record.Paths, arguments, record.SecurityContext) {
		return errors.New("checkpoint operation group path authority is invalid")
	}
	return nil
}

func NewOperationGroupTerminalV2(input OperationGroupTerminalInputV2) (OperationGroupTerminalV2, error) {
	intent := input.Intent
	observed := append([]ObservedOperationPathV2(nil), input.ObservedPaths...)
	sort.Slice(observed, func(i, j int) bool {
		return observedOperationPathKey(observed[i]) < observedOperationPathKey(observed[j])
	})
	record := OperationGroupTerminalV2{
		SchemaVersion: OperationGroupTerminalSchemaVersion, Purpose: OperationGroupTerminalPurpose,
		OperationGroupID: intent.OperationGroupID, ThreadID: intent.SecurityContext.ThreadID,
		TurnID: intent.SecurityContext.TurnID, ContextDigest: intent.SecurityContext.ContextDigest,
		CheckpointID: intent.CheckpointID, OperationOrdinal: intent.OperationOrdinal,
		Status: strings.TrimSpace(input.Status), ReasonCode: strings.TrimSpace(input.ReasonCode), ObservedPaths: observed,
		SettledAt: input.SettledAt.UTC().Format(time.RFC3339Nano),
	}
	record.TerminalDigest = operationGroupTerminalDigest(record)
	if err := ValidateOperationGroupTerminalForIntentV2(record, intent); err != nil {
		return OperationGroupTerminalV2{}, err
	}
	return record, nil
}

func ValidateOperationGroupTerminalForIntentV2(record OperationGroupTerminalV2, intent OperationGroupIntentV2) error {
	settledAt, settledErr := time.Parse(time.RFC3339Nano, record.SettledAt)
	createdAt, createdErr := time.Parse(time.RFC3339Nano, intent.CreatedAt)
	if ValidateOperationGroupIntentV2(intent) != nil || record.SchemaVersion != OperationGroupTerminalSchemaVersion ||
		record.Purpose != OperationGroupTerminalPurpose || record.OperationGroupID != intent.OperationGroupID ||
		record.ThreadID != intent.SecurityContext.ThreadID || record.TurnID != intent.SecurityContext.TurnID ||
		record.ContextDigest != intent.SecurityContext.ContextDigest || record.CheckpointID != intent.CheckpointID ||
		record.OperationOrdinal != intent.OperationOrdinal || settledErr != nil || createdErr != nil || settledAt.Before(createdAt) ||
		settledAt.Location() != time.UTC || settledAt.Format(time.RFC3339Nano) != record.SettledAt ||
		!domainsecurity.IsSHA256Hex(record.TerminalDigest) || record.TerminalDigest != operationGroupTerminalDigest(record) ||
		!allowedTerminalReason(record.Status, record.ReasonCode) {
		return errors.New("checkpoint operation group terminal integrity is invalid")
	}
	classification, ok := ClassifyObservedOperationGroup(intent, record.ObservedPaths)
	if !ok || classification != record.Status {
		return errors.New("checkpoint operation group terminal does not match observed filesystem state")
	}
	return nil
}

func ClassifyObservedOperationGroup(intent OperationGroupIntentV2, observed []ObservedOperationPathV2) (string, bool) {
	if len(intent.Paths) == 0 || len(observed) != len(intent.Paths) {
		return "", false
	}
	byPath := make(map[string]ObservedOperationPathV2, len(observed))
	for _, item := range observed {
		if !canonicalOperationRelativePath(item.RelativePath) || !validObservedPathAuthority(item) ||
			!oneOfOperationValue(item.ObservationStatus, "exact", "unavailable") {
			return "", false
		}
		if item.ObservationStatus == "exact" {
			if item.BlockerCode != "" || item.Existed && !domainsecurity.IsSHA256Hex(item.Hash) || !item.Existed && item.Hash != "" {
				return "", false
			}
		} else if !canonicalObservationBlocker(item.BlockerCode) || item.Existed || item.Hash != "" {
			return "", false
		}
		key := observedOperationPathKey(item)
		if _, duplicate := byPath[key]; duplicate {
			return "", false
		}
		byPath[key] = item
	}
	allBefore := true
	allAfter := true
	for _, expected := range intent.Paths {
		actual, found := byPath[operationPathKey(expected)]
		if !found {
			return "", false
		}
		if actual.ObservationStatus != "exact" {
			allBefore = false
			allAfter = false
			continue
		}
		if !sameObservedState(actual, expected.BeforeExisted, expected.BeforeHash) {
			allBefore = false
		}
		if !sameObservedState(actual, expected.ExpectedAfterExisted, expected.ExpectedAfterHash) {
			allAfter = false
		}
	}
	switch {
	case allAfter:
		return "completed", true
	case allBefore:
		return "no_effect", true
	default:
		return "quarantined", true
	}
}

func OperationGroupIntentV2Bytes(record OperationGroupIntentV2) ([]byte, error) {
	if err := ValidateOperationGroupIntentV2(record); err != nil {
		return nil, err
	}
	return json.Marshal(record)
}

func OperationGroupTerminalV2Bytes(record OperationGroupTerminalV2, intent OperationGroupIntentV2) ([]byte, error) {
	if err := ValidateOperationGroupTerminalForIntentV2(record, intent); err != nil {
		return nil, err
	}
	return json.Marshal(record)
}

func ParseOperationGroupIntentV2(body []byte) (OperationGroupIntentV2, error) {
	var record OperationGroupIntentV2
	if err := decodeStrictOperationGroupRecord(body, &record); err != nil {
		return OperationGroupIntentV2{}, err
	}
	return record, ValidateOperationGroupIntentV2(record)
}

func ParseOperationGroupTerminalV2(body []byte, intent OperationGroupIntentV2) (OperationGroupTerminalV2, error) {
	var record OperationGroupTerminalV2
	if err := decodeStrictOperationGroupRecord(body, &record); err != nil {
		return OperationGroupTerminalV2{}, err
	}
	return record, ValidateOperationGroupTerminalForIntentV2(record, intent)
}

func TerminalSemanticEqual(left, right OperationGroupTerminalV2) bool {
	left.TerminalDigest, right.TerminalDigest = "", ""
	left.SettledAt, right.SettledAt = "", ""
	return operationGroupTerminalDigest(left) == operationGroupTerminalDigest(right)
}

func operationGroupIdentity(record OperationGroupIntentV2) string {
	identity := struct {
		Purpose                     string `json:"purpose"`
		ContextDigest               string `json:"contextDigest"`
		GrantID                     string `json:"grantId"`
		CheckpointID                string `json:"checkpointId"`
		SourceWorkspaceCheckpointID string `json:"sourceWorkspaceCheckpointId"`
		ToolName                    string `json:"toolName"`
		ArgsHash                    string `json:"argsHash"`
	}{
		Purpose: OperationGroupIntentPurpose, ContextDigest: record.SecurityContext.ContextDigest,
		GrantID: record.ExecutionGrant.GrantID, CheckpointID: record.CheckpointID,
		SourceWorkspaceCheckpointID: record.SourceWorkspaceCheckpointID, ToolName: record.ToolName,
		ArgsHash: record.ExecutionGrant.ArgsHash,
	}
	body, _ := json.Marshal(identity)
	return domainsecurity.SHA256Hex(body)
}

func operationGroupIntentDigest(record OperationGroupIntentV2) string {
	record.IntentDigest = ""
	body, _ := json.Marshal(record)
	return domainsecurity.SHA256Hex(body)
}

func operationGroupTerminalDigest(record OperationGroupTerminalV2) string {
	record.TerminalDigest = ""
	body, _ := json.Marshal(record)
	return domainsecurity.SHA256Hex(body)
}

func canonicalOperationArguments(raw []byte) ([]byte, error) {
	value, err := domainjsonstrict.DecodeValue(raw, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: MaxOperationArgumentsBytes, MaxDepth: 32, MaxTokens: 100_000,
		MaxStringBytes: MaxSnapshotContentBytes + 64*1024,
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func operationArgumentsObject(raw []byte) (map[string]any, bool) {
	value, err := domainjsonstrict.DecodeValue(raw, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: MaxOperationArgumentsBytes, MaxDepth: 32, MaxTokens: 100_000,
		MaxStringBytes: MaxSnapshotContentBytes + 64*1024,
	})
	object, ok := value.(map[string]any)
	return object, err == nil && ok
}

func validOperationPaths(
	toolName string,
	paths []OperationPathV2,
	arguments map[string]any,
	securityContext domainsecurity.TurnSecurityContext,
) bool {
	seen := map[string]bool{}
	roles := map[string]OperationPathV2{}
	for _, item := range paths {
		argumentValue, argumentOK := arguments[item.ArgumentKey].(string)
		if !argumentOK || argumentValue != item.RequestedPath || item.RequestedPath == "" || len(item.RequestedPath) > 4096 ||
			!allowedPathArgumentKey(toolName, item.ArgumentKey, item.Role) || !canonicalOperationRelativePath(item.RelativePath) ||
			!validOperationPathAuthority(item, securityContext) || seen[operationPathKey(item)] ||
			!oneOfOperationValue(item.Role, "target", "source", "destination") {
			return false
		}
		seen[operationPathKey(item)] = true
		roles[item.Role] = item
		if item.BeforeExisted {
			if !item.BeforeAvailable || !validOperationBeforeSnapshot(item) {
				return false
			}
		} else if item.BeforeAvailable || item.BeforeHash != "" || item.BeforeSizeBytes != 0 || item.BeforeSnapshotSchemaVersion != 0 ||
			item.BeforeEncoding != "" || item.BeforeBytesBase64 != "" || item.BeforeContent != "" {
			return false
		}
		if item.ExpectedAfterExisted {
			if !domainsecurity.IsSHA256Hex(item.ExpectedAfterHash) {
				return false
			}
		} else if item.ExpectedAfterHash != "" {
			return false
		}
	}
	if toolName == "move_file" {
		source, sourceOK := roles["source"]
		destination, destinationOK := roles["destination"]
		return len(paths) == 2 && sourceOK && destinationOK && source.BeforeExisted && !source.ExpectedAfterExisted &&
			!destination.BeforeExisted && destination.ExpectedAfterExisted && source.BeforeHash == destination.ExpectedAfterHash
	}
	target, targetOK := roles["target"]
	if len(paths) != 1 || !targetOK || !target.ExpectedAfterExisted {
		return false
	}
	if toolName != "write" && toolName != "write_file" && !target.BeforeExisted {
		return false
	}
	return true
}

func validOperationPathAuthority(item OperationPathV2, securityContext domainsecurity.TurnSecurityContext) bool {
	if item.PathAuthoritySchemaVersion == 0 {
		return false
	}
	if item.PathAuthoritySchemaVersion != 1 || !oneOfOperationValue(item.AuthorityKind, "workspace", "allow_write") ||
		item.AuthorityRoot == "" || strings.TrimSpace(item.AuthorityRoot) != item.AuthorityRoot || strings.ContainsRune(item.AuthorityRoot, '\x00') ||
		item.AuthorityRootIdentity == "" || strings.TrimSpace(item.AuthorityRootIdentity) != item.AuthorityRootIdentity || strings.ContainsRune(item.AuthorityRootIdentity, '\x00') ||
		!domainsecurity.IsSHA256Hex(item.AuthorityRootHash) || operationAuthorityRootHash(item.AuthorityRoot, item.AuthorityRootIdentity) != item.AuthorityRootHash {
		return false
	}
	if item.AuthorityKind == "workspace" {
		return item.AuthorityRoot == securityContext.WorkspaceRealPath
	}
	return item.AuthorityRoot != securityContext.WorkspaceRealPath
}

func operationAuthorityRootHash(root, identity string) string {
	return domainsecurity.SHA256Hex([]byte(root + "\x00" + identity))
}

func validOperationBeforeSnapshot(item OperationPathV2) bool {
	if !domainsecurity.IsSHA256Hex(item.BeforeHash) {
		return false
	}
	if item.BeforeSnapshotSchemaVersion == 0 {
		return item.BeforeEncoding == "" && item.BeforeBytesBase64 == "" && len(item.BeforeContent) <= MaxSnapshotContentBytes &&
			int64(len(item.BeforeContent)) == item.BeforeSizeBytes &&
			utf8.ValidString(item.BeforeContent) && domainsecurity.SHA256Hex([]byte(item.BeforeContent)) == item.BeforeHash
	}
	if item.BeforeSnapshotSchemaVersion != 1 || item.BeforeContent != "" || !canonicalCheckpointTextEncoding(item.BeforeEncoding) {
		return false
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(item.BeforeBytesBase64)
	return err == nil && len(raw) <= MaxSnapshotContentBytes && int64(len(raw)) == item.BeforeSizeBytes &&
		domainsecurity.SHA256Hex(raw) == item.BeforeHash
}

func canonicalCheckpointTextEncoding(value string) bool {
	return oneOfOperationValue(value, "utf8", "utf8-bom", "utf16le", "utf16be", "utf16le-nobom", "utf16be-nobom")
}

func operationPathKey(item OperationPathV2) string {
	if item.PathAuthoritySchemaVersion == 0 {
		return "legacy-workspace\x00" + item.RelativePath
	}
	return item.AuthorityKind + "\x00" + item.AuthorityRootHash + "\x00" + item.RelativePath
}

func observedOperationPathKey(item ObservedOperationPathV2) string {
	if item.PathAuthoritySchemaVersion == 0 {
		return "legacy-workspace\x00" + item.RelativePath
	}
	return item.AuthorityKind + "\x00" + item.AuthorityRootHash + "\x00" + item.RelativePath
}

func validObservedPathAuthority(item ObservedOperationPathV2) bool {
	if item.PathAuthoritySchemaVersion == 0 {
		return item.AuthorityKind == "" && item.AuthorityRootHash == ""
	}
	return item.PathAuthoritySchemaVersion == 1 && oneOfOperationValue(item.AuthorityKind, "workspace", "allow_write") &&
		domainsecurity.IsSHA256Hex(item.AuthorityRootHash)
}

func allowedMutationTool(toolName string) bool {
	return oneOfOperationValue(toolName,
		"write", "write_file", "edit", "edit_file", "multi_edit", "move_file", "notebook_edit", "delete_range", "delete_symbol",
	)
}

func allowedPathArgumentKey(toolName, key, role string) bool {
	switch toolName {
	case "write", "write_file", "edit", "edit_file", "multi_edit", "notebook_edit", "delete_range", "delete_symbol":
		return role == "target" && oneOfOperationValue(key, "path", "filePath", "FilePath")
	case "move_file":
		if role == "source" {
			return oneOfOperationValue(key, "source_path", "sourcePath", "source", "from", "path")
		}
		return role == "destination" && oneOfOperationValue(key, "destination_path", "destinationPath", "destination", "to")
	default:
		return false
	}
}

func allowedTerminalReason(status, reason string) bool {
	switch status {
	case "completed":
		return oneOfOperationValue(reason, "mutation_completed", "recovery_observed_after")
	case "no_effect":
		return oneOfOperationValue(reason, "mutation_failed_before_effect", "recovery_observed_before")
	case "quarantined":
		return oneOfOperationValue(reason, "mixed_filesystem_state", "filesystem_diverged", "filesystem_observation_failed", "terminal_write_uncertain")
	default:
		return false
	}
}

func sameObservedState(actual ObservedOperationPathV2, existed bool, hash string) bool {
	return actual.ObservationStatus == "exact" && actual.Existed == existed && (!existed || actual.Hash == hash) && (existed || actual.Hash == "")
}

func canonicalObservationBlocker(value string) bool {
	return oneOfOperationValue(value, "path_unsafe", "read_failed", "too_large", "unsupported_encoding")
}

func oneOfOperationValue(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func decodeStrictOperationGroupRecord(body []byte, target any) error {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: MaxSnapshotAuthorityRecordBytes, MaxDepth: 32,
		MaxTokens: 200_000, MaxStringBytes: maxOperationSnapshotStringBytes,
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
		return errors.New("checkpoint operation group authority contains trailing JSON")
	}
	return nil
}

func canonicalOperationRelativePath(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && !strings.Contains(value, "\\") &&
		!strings.HasPrefix(value, "/") && path.Clean(value) == value && value != "." && value != ".." &&
		!strings.HasPrefix(value, "../")
}
