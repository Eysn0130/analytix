package nativecomponent

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	ComponentImportAccelerator = "import-accelerator"
	ComponentCleaningOps       = "cleaning-ops"
	ComponentAnalysisCompute   = "analysis-compute"
	ComponentDataEngine        = "data-engine"

	NativeServerIdentity = "host:builtin"
	NativeProvider       = "host"
	MaxExecutionWindow   = 30 * time.Minute

	// A cold data-engine health probe includes exact executable revalidation,
	// staging, process-tree containment, readiness authentication, and—when a
	// case/epoch binding changes—confirmed termination of the prior session.
	// Keep one absolute fail-closed deadline, but budget the complete lifecycle
	// rather than only the in-process ping.
	dataEngineHealthMaxDuration = 10 * time.Second

	// The typed flow contract is intentionally bounded even though execution
	// remains unavailable until a DSV2 callback-scoped native handoff exists.
	dataEngineAccountFlowMaxDuration = 30 * time.Second
)

var (
	ErrRequestInvalid       = errors.New("native_component_request_invalid")
	ErrOperationUnavailable = errors.New("native_component_operation_unavailable")
	ErrContextMismatch      = errors.New("native_component_context_mismatch")
	ErrGrantInvalid         = errors.New("native_component_grant_invalid")
)

type OperationPolicy struct {
	ComponentID string
	Operation   string
	ReadOnly    bool
	MaxDuration time.Duration
	SchemaHash  string
	Required    []string
	Optional    []string
}

type Request struct {
	ComponentID          string
	Operation            string
	AccountFlowArguments *AnalyzeAccountFlowsArgumentsV1
	Context              domainsecurity.TurnSecurityContext
	Grant                domainsecurity.ExecutionGrant
	Deadline             time.Time
}

var operationPolicies = map[string]OperationPolicy{
	policyKey(ComponentDataEngine, "health"): operationPolicy(ComponentDataEngine, "health", true, dataEngineHealthMaxDuration, nil, nil),
	// This policy describes one fixed host-private typed contract. Registration
	// here does not advertise a provider tool and does not make the operation
	// executable: the native runner still rejects it before acquiring a helper
	// until an exact callback-scoped DSV2 handoff is composed.
	policyKey(ComponentDataEngine, OperationFundsAnalyzeAccountFlows): operationPolicy(
		ComponentDataEngine,
		OperationFundsAnalyzeAccountFlows,
		true,
		dataEngineAccountFlowMaxDuration,
		analyzeAccountFlowsRequiredFieldsV1,
		nil,
	),
}

func Policy(componentID, operation string) (OperationPolicy, bool) {
	policy, ok := operationPolicies[policyKey(componentID, operation)]
	if !ok {
		return OperationPolicy{}, false
	}
	policy.Required = append([]string(nil), policy.Required...)
	policy.Optional = append([]string(nil), policy.Optional...)
	return policy, true
}

func ToolName(componentID, operation string) string {
	if _, ok := Policy(componentID, operation); !ok {
		return ""
	}
	return "native__" + strings.ReplaceAll(componentID, "-", "_") + "__" + strings.ReplaceAll(operation, ".", "_")
}

func ParseToolName(toolName string) (OperationPolicy, bool) {
	toolName = strings.TrimSpace(toolName)
	for _, policy := range operationPolicies {
		if toolName == ToolName(policy.ComponentID, policy.Operation) {
			return Policy(policy.ComponentID, policy.Operation)
		}
	}
	return OperationPolicy{}, false
}

// CanonicalArgumentsV1 returns the only host-generated argument document for
// a registered zero-argument native operation. Any future operation with
// parameters must add an operation-specific typed value and encoder instead
// of widening this factory to arbitrary JSON.
func CanonicalArgumentsV1(componentID, operation string) (string, bool) {
	policy, ok := Policy(componentID, operation)
	if !ok || policy.ComponentID != ComponentDataEngine || policy.Operation != "health" ||
		len(policy.Required) != 0 || len(policy.Optional) != 0 {
		return "", false
	}
	return "{}", true
}

func ScopeHash(context domainsecurity.TurnSecurityContext, componentID, operation string) string {
	if domainsecurity.ValidateTurnSecurityContextForExecution(context) != nil {
		return ""
	}
	body, err := json.Marshal(struct {
		Version         int    `json:"version"`
		ThreadID        string `json:"threadId"`
		TurnID          string `json:"turnId"`
		CaseID          string `json:"caseId"`
		CaseBindingHash string `json:"caseBindingHash"`
		DatasetSnapshot string `json:"datasetSnapshotId"`
		ContextEpoch    uint64 `json:"contextEpoch"`
		ContextDigest   string `json:"contextDigest"`
		ComponentID     string `json:"componentId"`
		Operation       string `json:"operation"`
	}{
		Version: 1, ThreadID: context.ThreadID, TurnID: context.TurnID, CaseID: context.CaseID,
		CaseBindingHash: context.CaseBindingHash, DatasetSnapshot: context.DatasetSnapshotID,
		ContextEpoch: context.ContextEpoch, ContextDigest: context.ContextDigest,
		ComponentID: componentID, Operation: operation,
	})
	if err != nil {
		return ""
	}
	return domainsecurity.CanonicalJSONHash(body)
}

func ValidateRequest(request Request, now time.Time) error {
	policy, ok := Policy(request.ComponentID, request.Operation)
	if !ok {
		return ErrOperationUnavailable
	}
	if request.ComponentID != policy.ComponentID || request.Operation != policy.Operation ||
		request.ComponentID != strings.TrimSpace(request.ComponentID) || request.Operation != strings.TrimSpace(request.Operation) {
		return ErrRequestInvalid
	}
	if domainsecurity.ValidateTurnSecurityContextForExecution(request.Context) != nil ||
		!domainsecurity.TurnSecurityContextAllowsCaseEvidence(request.Context) ||
		request.Context.CaseID == domainsecurity.UnboundCaseID {
		return ErrContextMismatch
	}
	if domainsecurity.ValidateExecutionGrantForContext(request.Grant, request.Context) != nil ||
		request.Grant.Provider != NativeProvider || request.Grant.ServerIdentity != NativeServerIdentity ||
		request.Grant.ConnectionEpoch != 0 || request.Grant.ToolName != ToolName(policy.ComponentID, policy.Operation) ||
		request.Grant.SchemaHash != policy.SchemaHash || request.Grant.ScopeHash != ScopeHash(request.Context, policy.ComponentID, policy.Operation) ||
		request.Grant.ReadOnly != policy.ReadOnly {
		return ErrGrantInvalid
	}
	argumentsHash, argumentsErr := requestArgumentsHashV1(request)
	if argumentsErr != nil {
		return argumentsErr
	}
	if request.Grant.ArgsHash != argumentsHash {
		return ErrGrantInvalid
	}
	if policy.ReadOnly {
		if request.Grant.ApprovalState != "not_required" && request.Grant.ApprovalState != "approved" {
			return ErrGrantInvalid
		}
	} else if request.Grant.ApprovalState != "approved" {
		return ErrGrantInvalid
	}
	issuedAt, issuedErr := time.Parse(time.RFC3339Nano, request.Grant.IssuedAt)
	expiresAt, expiresErr := time.Parse(time.RFC3339Nano, request.Grant.ExpiresAt)
	now = now.UTC()
	if now.IsZero() || issuedErr != nil || expiresErr != nil || now.Before(issuedAt) || !now.Before(expiresAt) ||
		request.Deadline.IsZero() || !request.Deadline.After(now) || request.Deadline.After(expiresAt) ||
		policy.MaxDuration <= 0 || policy.MaxDuration > MaxExecutionWindow || request.Deadline.Sub(now) > policy.MaxDuration {
		return ErrGrantInvalid
	}
	return nil
}

func requestArgumentsHashV1(request Request) (string, error) {
	switch request.Operation {
	case "health":
		if request.AccountFlowArguments != nil {
			return "", ErrRequestInvalid
		}
		canonicalArguments, ok := CanonicalArgumentsV1(request.ComponentID, request.Operation)
		if !ok {
			return "", ErrRequestInvalid
		}
		return domainsecurity.CanonicalJSONHash([]byte(canonicalArguments)), nil
	case OperationFundsAnalyzeAccountFlows:
		if request.ComponentID != ComponentDataEngine || request.AccountFlowArguments == nil {
			return "", ErrRequestInvalid
		}
		if !analyzeAccountFlowsArgumentsMatchContextV1(*request.AccountFlowArguments, request.Context) {
			return "", ErrContextMismatch
		}
		hash := AnalyzeAccountFlowsArgumentsHashV1(*request.AccountFlowArguments)
		if hash == "" {
			return "", ErrRequestInvalid
		}
		return hash, nil
	default:
		return "", ErrOperationUnavailable
	}
}

func operationPolicy(componentID, operation string, readOnly bool, maxDuration time.Duration, required, optional []string) OperationPolicy {
	shape, _ := json.Marshal(struct {
		Version           int      `json:"version"`
		ComponentID       string   `json:"componentId"`
		Operation         string   `json:"operation"`
		ReadOnly          bool     `json:"readOnly"`
		MaxDurationMillis int64    `json:"maxDurationMillis"`
		Required          []string `json:"required"`
		Optional          []string `json:"optional"`
	}{
		Version: 1, ComponentID: componentID, Operation: operation, ReadOnly: readOnly,
		MaxDurationMillis: maxDuration.Milliseconds(),
		Required:          append([]string(nil), required...), Optional: append([]string(nil), optional...),
	})
	return OperationPolicy{
		ComponentID: componentID, Operation: operation, ReadOnly: readOnly,
		MaxDuration: maxDuration,
		SchemaHash:  domainsecurity.CanonicalJSONHash(shape),
		Required:    append([]string(nil), required...), Optional: append([]string(nil), optional...),
	}
}

func policyKey(componentID, operation string) string {
	return strings.TrimSpace(componentID) + "\x00" + strings.TrimSpace(operation)
}
