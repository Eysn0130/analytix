package turn

import (
	"fmt"
	"strings"
	"time"

	executionpolicy "analytix.local/runtime-go/internal/app/executionpolicy"
	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
)

type StartPlanInput struct {
	ThreadID                     string
	TurnNumber                   int
	Prompt                       string
	DisplayText                  string
	Model                        string
	ProviderID                   string
	ReasoningEffort              string
	EndpointFormat               string
	RequestApprovalPolicy        string
	ThreadApprovalPolicy         string
	RuntimeApprovalPolicy        string
	RequestSandboxMode           string
	ThreadSandboxMode            string
	RuntimeSandboxMode           string
	ThreadExecutionPolicyVersion int
	ThreadPolicyMigrationAllowed bool
	CreatedAt                    string
	Mode                         string
	AttachmentIDs                []string
	Attachments                  AttachmentPlan
	FileReferences               []any
	WorkspaceCheckpointID        string
	GUIPlan                      map[string]any
	DisableUserInput             bool
	DisableUserInputSet          bool
	MaxModelSteps                *int
}

type StartPlan struct {
	ThreadID       string
	TurnID         string
	UserItemID     string
	StartedAt      string
	ApprovalPolicy string
	SandboxMode    string
	Record         StartRecord
	ThreadPatch    map[string]any
}

func (p StartPlan) Response() map[string]any {
	return map[string]any{
		"threadId":          p.ThreadID,
		"turnId":            p.TurnID,
		"userMessageItemId": p.UserItemID,
	}
}

func BuildStartPlan(input StartPlanInput) StartPlan {
	threadID := strings.TrimSpace(input.ThreadID)
	turnID := fmt.Sprintf("turn_%d", input.TurnNumber)
	userItemID := "item_" + turnID + "_user"
	startedAt := strings.TrimSpace(input.CreatedAt)
	if startedAt == "" {
		startedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	threadApprovalPolicy := input.ThreadApprovalPolicy
	threadSandboxMode := input.ThreadSandboxMode
	persistExecutionPolicy := input.ThreadPolicyMigrationAllowed &&
		input.ThreadExecutionPolicyVersion == 0
	if persistExecutionPolicy &&
		normalizeStartApprovalPolicy(threadApprovalPolicy) == executionpolicy.LegacyApprovalPolicy &&
		normalizeStartSandboxMode(threadSandboxMode) == executionpolicy.LegacySandboxMode {
		threadApprovalPolicy = executionpolicy.DefaultApprovalPolicy
		threadSandboxMode = executionpolicy.DefaultSandboxMode
	}
	approvalPolicy := ResolveStartApprovalPolicy(input.RequestApprovalPolicy, threadApprovalPolicy, input.RuntimeApprovalPolicy)
	sandboxMode := ResolveStartSandboxMode(input.RequestSandboxMode, threadSandboxMode, input.RuntimeSandboxMode)
	if approvalPolicy == "" {
		approvalPolicy = executionpolicy.DefaultApprovalPolicy
	}
	if sandboxMode == "" {
		sandboxMode = executionpolicy.DefaultSandboxMode
	}
	record := BuildStartRecord(StartRecordInput{
		ThreadID:              threadID,
		TurnID:                turnID,
		UserItemID:            userItemID,
		Prompt:                input.Prompt,
		DisplayText:           input.DisplayText,
		Model:                 input.Model,
		ProviderID:            input.ProviderID,
		ReasoningEffort:       input.ReasoningEffort,
		ApprovalPolicy:        approvalPolicy,
		SandboxMode:           sandboxMode,
		CreatedAt:             startedAt,
		Mode:                  input.Mode,
		AttachmentIDs:         input.AttachmentIDs,
		Attachments:           privacyprojectionapp.ProjectAttachmentMetadata(input.Attachments.Metadata),
		AttachmentPipeline:    input.Attachments.PipelineDetails(),
		FileReferences:        input.FileReferences,
		WorkspaceCheckpointID: input.WorkspaceCheckpointID,
		GUIPlan:               input.GUIPlan,
		DisableUserInput:      input.DisableUserInput,
		DisableUserInputSet:   input.DisableUserInputSet,
		MaxModelSteps:         input.MaxModelSteps,
	})
	return StartPlan{
		ThreadID:       threadID,
		TurnID:         turnID,
		UserItemID:     userItemID,
		StartedAt:      startedAt,
		ApprovalPolicy: approvalPolicy,
		SandboxMode:    sandboxMode,
		Record:         record,
		ThreadPatch: BuildStartThreadPatch(StartThreadPatchInput{
			RequestApprovalPolicy:   input.RequestApprovalPolicy,
			RequestSandboxMode:      input.RequestSandboxMode,
			PersistExecutionPolicy:  persistExecutionPolicy,
			EffectiveApprovalPolicy: approvalPolicy,
			EffectiveSandboxMode:    sandboxMode,
			ExecutionPolicyVersion:  executionpolicy.CurrentVersion,
			Model:                   input.Model,
			ReasoningEffort:         input.ReasoningEffort,
			EndpointFormat:          input.EndpointFormat,
		}),
	}
}

func ResolveStartApprovalPolicy(requestValue string, threadValue string, runtimeDefault string) string {
	return firstStartPolicy(normalizeStartApprovalPolicy, requestValue, threadValue, runtimeDefault)
}

func ResolveStartSandboxMode(requestValue string, threadValue string, runtimeDefault string) string {
	return firstStartPolicy(normalizeStartSandboxMode, requestValue, threadValue, runtimeDefault)
}

func firstStartPolicy(normalize func(string) string, values ...string) string {
	for _, value := range values {
		if normalized := normalize(value); normalized != "" {
			return normalized
		}
	}
	return ""
}

func normalizeStartApprovalPolicy(value string) string {
	switch strings.TrimSpace(value) {
	case "always", "auto", "on-request", "untrusted", "suggest", "never":
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func normalizeStartSandboxMode(value string) string {
	switch strings.TrimSpace(value) {
	case "read-only", "workspace-write", "danger-full-access", "external-sandbox":
		return strings.TrimSpace(value)
	default:
		return ""
	}
}
