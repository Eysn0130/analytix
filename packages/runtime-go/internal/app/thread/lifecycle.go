package thread

import (
	"errors"
	"strings"

	executionpolicy "analytix.local/runtime-go/internal/app/executionpolicy"
	contracts "analytix.local/runtime-go/internal/contracts"
)

var ErrExecutionPolicyVersionRuntimeOwned = errors.New("executionPolicyVersion is runtime-owned")
var ErrInvalidThreadExecutionPolicy = errors.New("thread execution policy fields must be valid policy strings")
var ErrContextEpochStateRuntimeOwned = errors.New("contextEpochState is runtime-owned")
var ErrThreadStatusRuntimeOwned = errors.New("thread status is runtime-owned")
var ErrInvalidThreadArchiveIntent = errors.New("archived must be the sole boolean thread patch field")
var ErrInvalidSidePromotionIntent = errors.New("promoteSide must be the sole true thread patch field")
var ErrThreadRelationRuntimeOwned = errors.New("thread relation and parent are runtime-owned; use a supported thread intent")
var ErrSidePromotionUnavailable = errors.New("own Side promotion authority is unavailable")
var ErrCaseWorkspaceMutationRestricted = errors.New("case workspace changes require a host context epoch transition")

const (
	defaultApprovalPolicy = executionpolicy.DefaultApprovalPolicy
	defaultSandboxMode    = executionpolicy.DefaultSandboxMode
)

type CreateInput struct {
	Request           map[string]any
	ThreadID          string
	FallbackWorkspace string
	Now               string
}

func BuildThread(input CreateInput) map[string]any {
	request := contracts.CloneMap(input.Request)
	title := strings.TrimSpace(projectOrdinaryThreadText(stringField(request, "title")))
	if title == "" {
		title = "New thread"
	}
	workspace := strings.TrimSpace(stringField(request, "workspace"))
	if workspace == "" {
		workspace = strings.TrimSpace(input.FallbackWorkspace)
	}
	model := strings.TrimSpace(stringField(request, "model"))
	providerID := strings.TrimSpace(stringField(request, "providerId"))
	mode := strings.TrimSpace(stringField(request, "mode"))
	if mode != "plan" {
		mode = "agent"
	}
	approvalPolicy := strings.TrimSpace(stringField(request, "approvalPolicy"))
	if NormalizeApprovalPolicy(approvalPolicy) == "" {
		approvalPolicy = defaultApprovalPolicy
	}
	sandboxMode := strings.TrimSpace(stringField(request, "sandboxMode"))
	if NormalizeSandboxMode(sandboxMode) == "" {
		sandboxMode = defaultSandboxMode
	}
	relation := strings.TrimSpace(stringField(request, "relation"))
	if relation == "" {
		relation = "primary"
	}
	thread := map[string]any{
		"id":                     strings.TrimSpace(input.ThreadID),
		"title":                  title,
		"workspace":              workspace,
		"model":                  model,
		"providerId":             providerID,
		"mode":                   mode,
		"status":                 "idle",
		"executionPolicyVersion": float64(executionpolicy.CurrentVersion),
		"approvalPolicy":         approvalPolicy,
		"sandboxMode":            sandboxMode,
		"relation":               relation,
		"turns":                  []any{},
		"createdAt":              input.Now,
		"updatedAt":              input.Now,
	}
	if parentThreadID := strings.TrimSpace(stringField(request, "parentThreadId")); parentThreadID != "" {
		thread["parentThreadId"] = parentThreadID
	}
	if autoTitle, ok := request["autoTitle"].(bool); ok && autoTitle {
		thread["autoTitle"] = true
	}
	if providerID == "" {
		delete(thread, "providerId")
	}
	return thread
}

func ApplyPatch(thread map[string]any, patch map[string]any, now string) map[string]any {
	next := contracts.CloneMap(thread)
	for _, key := range []string{"workspace", "status", "relation", "parentThreadId", "providerId", "model", "mode", "approvalPolicy", "sandboxMode", "contextEpochState"} {
		if value, ok := patch[key]; ok {
			next[key] = contracts.CloneValue(value)
		}
	}
	approvalPolicy, approvalIsString := patch["approvalPolicy"].(string)
	sandboxMode, sandboxIsString := patch["sandboxMode"].(string)
	approvalPolicy = NormalizeApprovalPolicy(approvalPolicy)
	sandboxMode = NormalizeSandboxMode(sandboxMode)
	if approvalIsString && sandboxIsString && approvalPolicy != "" && sandboxMode != "" {
		// The complete explicit pair records a choice even when its values match
		// a legacy default. Partial metadata cannot establish that distinction.
		next["approvalPolicy"] = approvalPolicy
		next["sandboxMode"] = sandboxMode
		// CloneMap follows the durable JSON numeric representation. Preserve a
		// future version rather than interpreting this write as a downgrade.
		version, _ := next["executionPolicyVersion"].(float64)
		if version <= float64(executionpolicy.CurrentVersion) {
			next["executionPolicyVersion"] = float64(executionpolicy.CurrentVersion)
		}
	}
	if value, ok := patch["title"]; ok {
		if title, isString := value.(string); isString {
			next["title"] = projectOrdinaryThreadText(title)
		} else {
			next["title"] = contracts.CloneValue(value)
		}
	}
	next["updatedAt"] = now
	return next
}

func ValidatePublicPatch(patch map[string]any) error {
	if _, exists := patch["relation"]; exists {
		return ErrThreadRelationRuntimeOwned
	}
	if _, exists := patch["parentThreadId"]; exists {
		return ErrThreadRelationRuntimeOwned
	}
	if value, exists := patch["promoteSide"]; exists {
		if enabled, valid := value.(bool); !valid || !enabled || len(patch) != 1 {
			return ErrInvalidSidePromotionIntent
		}
	}
	if _, exists := patch["status"]; exists {
		return ErrThreadStatusRuntimeOwned
	}
	if _, exists := patch["executionPolicyVersion"]; exists {
		return ErrExecutionPolicyVersionRuntimeOwned
	}
	if _, exists := patch["contextEpochState"]; exists {
		return ErrContextEpochStateRuntimeOwned
	}
	if value, exists := patch["approvalPolicy"]; exists {
		policy, validString := value.(string)
		if !validString || NormalizeApprovalPolicy(policy) == "" {
			return ErrInvalidThreadExecutionPolicy
		}
	}
	if value, exists := patch["sandboxMode"]; exists {
		mode, validString := value.(string)
		if !validString || NormalizeSandboxMode(mode) == "" {
			return ErrInvalidThreadExecutionPolicy
		}
	}
	if value, exists := patch["archived"]; exists {
		if _, valid := value.(bool); !valid || len(patch) != 1 {
			return ErrInvalidThreadArchiveIntent
		}
	}
	return nil
}

func BuildUpdatedEvent(threadID string, _ string, status string) map[string]any {
	return map[string]any{
		"kind":     "thread_updated",
		"threadId": strings.TrimSpace(threadID),
		"status":   strings.TrimSpace(status),
	}
}

func MarkDeleted(thread map[string]any, now string) map[string]any {
	next := contracts.CloneMap(thread)
	next["status"] = "deleted"
	next["updatedAt"] = now
	return next
}
