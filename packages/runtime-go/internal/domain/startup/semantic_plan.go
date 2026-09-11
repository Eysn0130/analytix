package startup

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	SemanticStartupPlanSchemaVersion = 1
	SemanticStartupPlannerVersionV1  = "semantic-startup-planner-v1"
	MaxSemanticPlanOperationsV1      = 10_000
	MaxSemanticManagedFileBytesV1    = int64(64 << 20)
	MaxSemanticStagedTotalBytesV1    = int64(2 << 30)
	MaxSemanticManagedPathBytesV1    = 4 << 10
	MaxSemanticManagedPathDepthV1    = 64

	SemanticOperationCreateDirectory = "create_directory"
	SemanticOperationInstallFile     = "install_file"
	SemanticOperationSetMode         = "set_mode"
	SemanticOperationRemoveFile      = "remove_file"
	SemanticOperationRemoveDirectory = "remove_directory"
)

// SemanticEntryStateV1 is the bounded, non-secret identity of one managed
// persistence entry. File bytes live only in the private migration stage.
type SemanticEntryStateV1 struct {
	Type   string `json:"type"`
	Mode   uint32 `json:"mode"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256,omitempty"`
}

// SemanticStartupOperationV1 describes one exact pre/post filesystem state.
// The operation id binds both states, so a resumed journal can distinguish an
// unapplied operation from a completed operation without trusting its cursor.
type SemanticStartupOperationV1 struct {
	OperationID string               `json:"operationId"`
	Kind        string               `json:"kind"`
	Path        string               `json:"path"`
	Before      SemanticEntryStateV1 `json:"before"`
	After       SemanticEntryStateV1 `json:"after"`
}

// SemanticStartupPlanV1 binds the fixed-point snapshot, validated runtime
// configuration, deterministic target state, and ordered mutation program.
// It intentionally contains no credentials, case text, PII, or file bytes.
type SemanticStartupPlanV1 struct {
	SchemaVersion       int                          `json:"schemaVersion"`
	PlannerVersion      string                       `json:"plannerVersion"`
	BaselineDigest      string                       `json:"baselineDigest"`
	ConfigurationDigest string                       `json:"configurationDigest"`
	FinalStateDigest    string                       `json:"finalStateDigest"`
	Operations          []SemanticStartupOperationV1 `json:"operations"`
	PlanDigest          string                       `json:"planDigest"`
}

func NewSemanticStartupPlanV1(
	baselineDigest string,
	configurationDigest string,
	finalStateDigest string,
	operations []SemanticStartupOperationV1,
) (SemanticStartupPlanV1, error) {
	if len(operations) > MaxSemanticPlanOperationsV1 {
		return SemanticStartupPlanV1{}, errors.New("semantic startup operation budget is exceeded")
	}
	plan := SemanticStartupPlanV1{
		SchemaVersion:       SemanticStartupPlanSchemaVersion,
		PlannerVersion:      SemanticStartupPlannerVersionV1,
		BaselineDigest:      strings.TrimSpace(baselineDigest),
		ConfigurationDigest: strings.TrimSpace(configurationDigest),
		FinalStateDigest:    strings.TrimSpace(finalStateDigest),
		Operations:          append([]SemanticStartupOperationV1(nil), operations...),
	}
	sort.Slice(plan.Operations, func(left int, right int) bool {
		leftRank := semanticOperationRank(plan.Operations[left])
		rightRank := semanticOperationRank(plan.Operations[right])
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if leftRank == semanticOperationRankForRemovalDirectory {
			leftDepth := strings.Count(plan.Operations[left].Path, "/")
			rightDepth := strings.Count(plan.Operations[right].Path, "/")
			if leftDepth != rightDepth {
				return leftDepth > rightDepth
			}
		}
		return plan.Operations[left].Path < plan.Operations[right].Path
	})
	for index := range plan.Operations {
		operation := &plan.Operations[index]
		operation.OperationID = semanticOperationDigest(*operation)
	}
	if err := validateSemanticStartupPlanFields(plan); err != nil {
		return SemanticStartupPlanV1{}, err
	}
	plan.PlanDigest = semanticStartupPlanDigest(plan)
	return plan, nil
}

func ValidateSemanticStartupPlanV1(plan SemanticStartupPlanV1) error {
	if err := validateSemanticStartupPlanFields(plan); err != nil ||
		!domainsecurity.IsSHA256Hex(plan.PlanDigest) || semanticStartupPlanDigest(plan) != plan.PlanDigest {
		return errors.New("semantic startup plan integrity is invalid")
	}
	return nil
}

const (
	semanticOperationRankForCreateDirectory = iota
	semanticOperationRankForInstallFile
	semanticOperationRankForSetMode
	semanticOperationRankForRemovalFile
	semanticOperationRankForRemovalDirectory
)

func semanticOperationRank(operation SemanticStartupOperationV1) int {
	switch operation.Kind {
	case SemanticOperationCreateDirectory:
		return semanticOperationRankForCreateDirectory
	case SemanticOperationInstallFile:
		return semanticOperationRankForInstallFile
	case SemanticOperationSetMode:
		return semanticOperationRankForSetMode
	case SemanticOperationRemoveFile:
		return semanticOperationRankForRemovalFile
	case SemanticOperationRemoveDirectory:
		return semanticOperationRankForRemovalDirectory
	default:
		return 99
	}
}

func validateSemanticStartupPlanFields(plan SemanticStartupPlanV1) error {
	if plan.SchemaVersion != SemanticStartupPlanSchemaVersion || plan.PlannerVersion != SemanticStartupPlannerVersionV1 ||
		!domainsecurity.IsSHA256Hex(plan.BaselineDigest) || !domainsecurity.IsSHA256Hex(plan.ConfigurationDigest) ||
		!domainsecurity.IsSHA256Hex(plan.FinalStateDigest) || len(plan.Operations) > MaxSemanticPlanOperationsV1 {
		return errors.New("semantic startup plan fields are invalid")
	}
	seenIDs := map[string]bool{}
	seenPaths := map[string]bool{}
	previousRank := -1
	previousPath := ""
	for _, operation := range plan.Operations {
		if !validSemanticManagedPath(operation.Path) || seenPaths[operation.Path] ||
			!domainsecurity.IsSHA256Hex(operation.OperationID) || semanticOperationDigest(operation) != operation.OperationID ||
			validateSemanticOperation(operation) != nil {
			return errors.New("semantic startup operation is invalid")
		}
		rank := semanticOperationRank(operation)
		if rank < previousRank || (rank == previousRank && rank != semanticOperationRankForRemovalDirectory && operation.Path < previousPath) ||
			(rank == semanticOperationRankForRemovalDirectory && previousRank == rank &&
				(strings.Count(operation.Path, "/") > strings.Count(previousPath, "/") ||
					(strings.Count(operation.Path, "/") == strings.Count(previousPath, "/") && operation.Path < previousPath))) {
			return errors.New("semantic startup operations are not canonical")
		}
		if seenIDs[operation.OperationID] {
			return errors.New("semantic startup operation identity is duplicated")
		}
		seenIDs[operation.OperationID] = true
		seenPaths[operation.Path] = true
		previousRank = rank
		previousPath = operation.Path
	}
	return nil
}

func validateSemanticOperation(operation SemanticStartupOperationV1) error {
	if validateSemanticEntryState(operation.Before) != nil || validateSemanticEntryState(operation.After) != nil {
		return errors.New("semantic startup operation state is invalid")
	}
	switch operation.Kind {
	case SemanticOperationCreateDirectory:
		if operation.Before.Type != ManagedEntryTypeAbsent || operation.After.Type != ManagedEntryTypeDirectory {
			return errors.New("semantic create-directory transition is invalid")
		}
	case SemanticOperationInstallFile:
		if operation.After.Type != ManagedEntryTypeFile ||
			(operation.Before.Type != ManagedEntryTypeAbsent && operation.Before.Type != ManagedEntryTypeFile) {
			return errors.New("semantic install-file transition is invalid")
		}
	case SemanticOperationSetMode:
		if operation.Before.Type == ManagedEntryTypeAbsent || operation.Before.Type != operation.After.Type ||
			operation.Before.SHA256 != operation.After.SHA256 || operation.Before.Size != operation.After.Size ||
			operation.Before.Mode == operation.After.Mode {
			return errors.New("semantic set-mode transition is invalid")
		}
	case SemanticOperationRemoveFile:
		if operation.Before.Type != ManagedEntryTypeFile || operation.After.Type != ManagedEntryTypeAbsent {
			return errors.New("semantic remove-file transition is invalid")
		}
	case SemanticOperationRemoveDirectory:
		if operation.Before.Type != ManagedEntryTypeDirectory || operation.After.Type != ManagedEntryTypeAbsent {
			return errors.New("semantic remove-directory transition is invalid")
		}
	default:
		return errors.New("semantic startup operation kind is invalid")
	}
	return nil
}

func validateSemanticEntryState(state SemanticEntryStateV1) error {
	if state.Size < 0 {
		return errors.New("semantic startup entry size is invalid")
	}
	switch state.Type {
	case ManagedEntryTypeAbsent:
		if state.Mode != 0 || state.Size != 0 || state.SHA256 != "" {
			return errors.New("semantic absent state is invalid")
		}
	case ManagedEntryTypeDirectory:
		if state.Size != 0 || state.SHA256 != "" || state.Mode == 0 {
			return errors.New("semantic directory state is invalid")
		}
	case ManagedEntryTypeFile:
		if state.Mode == 0 || state.Size > MaxSemanticManagedFileBytesV1 || !domainsecurity.IsSHA256Hex(state.SHA256) {
			return errors.New("semantic file state is invalid")
		}
	default:
		return errors.New("semantic startup entry type is invalid")
	}
	return nil
}

func validSemanticManagedPath(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > MaxSemanticManagedPathBytesV1 || strings.Count(value, "/")+1 > MaxSemanticManagedPathDepthV1 ||
		value != strings.TrimSuffix(value, "/") || strings.Contains(value, "\\") ||
		strings.Contains("/"+value+"/", "/../") || strings.Contains("/"+value+"/", "/./") {
		return false
	}
	return strings.HasPrefix(value, "data/") || strings.HasPrefix(value, "durable/")
}

func semanticOperationDigest(operation SemanticStartupOperationV1) string {
	body := struct {
		Kind   string               `json:"kind"`
		Path   string               `json:"path"`
		Before SemanticEntryStateV1 `json:"before"`
		After  SemanticEntryStateV1 `json:"after"`
	}{operation.Kind, operation.Path, operation.Before, operation.After}
	encoded, _ := json.Marshal(body)
	return domainsecurity.SHA256Hex(encoded)
}

func semanticStartupPlanDigest(plan SemanticStartupPlanV1) string {
	body := struct {
		SchemaVersion       int                          `json:"schemaVersion"`
		PlannerVersion      string                       `json:"plannerVersion"`
		BaselineDigest      string                       `json:"baselineDigest"`
		ConfigurationDigest string                       `json:"configurationDigest"`
		FinalStateDigest    string                       `json:"finalStateDigest"`
		Operations          []SemanticStartupOperationV1 `json:"operations"`
	}{plan.SchemaVersion, plan.PlannerVersion, plan.BaselineDigest, plan.ConfigurationDigest, plan.FinalStateDigest, plan.Operations}
	encoded, _ := json.Marshal(body)
	return domainsecurity.SHA256Hex(encoded)
}
