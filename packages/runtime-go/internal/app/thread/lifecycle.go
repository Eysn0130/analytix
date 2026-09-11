package thread

import (
	"errors"
	"strings"

	executionpolicy "analytix.local/runtime-go/internal/app/executionpolicy"
	contracts "analytix.local/runtime-go/internal/contracts"
)

var ErrExecutionPolicyVersionRuntimeOwned = errors.New("executionPolicyVersion is runtime-owned")
var ErrContextEpochStateRuntimeOwned = errors.New("contextEpochState is runtime-owned")
var ErrThreadStatusRuntimeOwned = errors.New("thread status is runtime-owned")
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
	if _, exists := patch["status"]; exists {
		return ErrThreadStatusRuntimeOwned
	}
	if _, exists := patch["executionPolicyVersion"]; exists {
		return ErrExecutionPolicyVersionRuntimeOwned
	}
	if _, exists := patch["contextEpochState"]; exists {
		return ErrContextEpochStateRuntimeOwned
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
