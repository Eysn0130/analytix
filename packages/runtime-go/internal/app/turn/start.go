package turn

import (
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

type StartRecordInput struct {
	ThreadID              string
	TurnID                string
	UserItemID            string
	Prompt                string
	DisplayText           string
	Model                 string
	ProviderID            string
	ReasoningEffort       string
	ApprovalPolicy        string
	SandboxMode           string
	CreatedAt             string
	Mode                  string
	AttachmentIDs         []string
	Attachments           []map[string]any
	AttachmentPipeline    map[string]any
	FileReferences        []any
	WorkspaceCheckpointID string
	GUIPlan               map[string]any
	DisableUserInput      bool
	DisableUserInputSet   bool
	MaxModelSteps         *int
}

type StartRecord struct {
	UserItemID           string
	UserItem             map[string]any
	Turn                 map[string]any
	TurnStartedEvent     map[string]any
	UserItemCreatedEvent map[string]any
}

type StartThreadPatchInput struct {
	RequestApprovalPolicy   string
	RequestSandboxMode      string
	PersistExecutionPolicy  bool
	EffectiveApprovalPolicy string
	EffectiveSandboxMode    string
	ExecutionPolicyVersion  int
	Model                   string
	ReasoningEffort         string
	EndpointFormat          string
}

func BuildStartThreadPatch(input StartThreadPatchInput) map[string]any {
	patch := map[string]any{}
	if strings.TrimSpace(input.RequestApprovalPolicy) != "" {
		patch["approvalPolicy"] = strings.TrimSpace(input.RequestApprovalPolicy)
	}
	if strings.TrimSpace(input.RequestSandboxMode) != "" {
		patch["sandboxMode"] = strings.TrimSpace(input.RequestSandboxMode)
	}
	if input.PersistExecutionPolicy {
		patch["approvalPolicy"] = strings.TrimSpace(input.EffectiveApprovalPolicy)
		patch["sandboxMode"] = strings.TrimSpace(input.EffectiveSandboxMode)
		patch["executionPolicyVersion"] = float64(input.ExecutionPolicyVersion)
	}
	if strings.TrimSpace(input.Model) != "" {
		patch["model"] = strings.TrimSpace(input.Model)
	}
	if reasoningEffort, valid := domainmodel.ProjectReasoningEffortV1(input.ReasoningEffort); valid && reasoningEffort != "" {
		patch["reasoningEffort"] = reasoningEffort
	}
	if strings.TrimSpace(input.EndpointFormat) != "" {
		patch["endpointFormat"] = strings.TrimSpace(input.EndpointFormat)
	}
	return patch
}

func BuildStartRecord(input StartRecordInput) StartRecord {
	threadID := strings.TrimSpace(input.ThreadID)
	turnID := strings.TrimSpace(input.TurnID)
	userItemID := strings.TrimSpace(input.UserItemID)
	if userItemID == "" {
		userItemID = "item_" + turnID + "_user"
	}
	now := strings.TrimSpace(input.CreatedAt)
	userItem := map[string]any{
		"id":         userItemID,
		"turnId":     turnID,
		"threadId":   threadID,
		"role":       "user",
		"status":     "completed",
		"createdAt":  now,
		"finishedAt": now,
		"kind":       "user_message",
		"text":       input.Prompt,
	}
	if strings.TrimSpace(input.DisplayText) != "" && input.DisplayText != input.Prompt {
		userItem["displayText"] = input.DisplayText
	}
	if len(input.AttachmentIDs) > 0 {
		userItem["attachmentIds"] = stringListAny(input.AttachmentIDs)
	}
	if len(input.Attachments) > 0 {
		userItem["attachments"] = mapsListAny(input.Attachments)
	}
	if len(input.FileReferences) > 0 {
		userItem["fileReferences"] = contracts.CloneValue(input.FileReferences)
	}
	if strings.TrimSpace(input.WorkspaceCheckpointID) != "" {
		userItem["workspaceCheckpointId"] = strings.TrimSpace(input.WorkspaceCheckpointID)
	}

	turn := map[string]any{
		"id":                turnID,
		"threadId":          threadID,
		"status":            "running",
		"prompt":            input.Prompt,
		"model":             strings.TrimSpace(input.Model),
		"steering":          []any{},
		"createdAt":         now,
		"startedAt":         now,
		"items":             []any{contracts.CloneMap(userItem)},
		"attachmentIds":     stringListAny(input.AttachmentIDs),
		"activeSkillIds":    []any{},
		"injectedMemoryIds": []any{},
		"approvalPolicy":    strings.TrimSpace(input.ApprovalPolicy),
		"sandboxMode":       strings.TrimSpace(input.SandboxMode),
	}
	if reasoningEffort, valid := domainmodel.ProjectReasoningEffortV1(input.ReasoningEffort); valid && reasoningEffort != "" {
		turn["reasoningEffort"] = reasoningEffort
	}
	if len(input.Attachments) > 0 {
		turn["attachments"] = mapsListAny(input.Attachments)
	}
	if len(input.FileReferences) > 0 {
		turn["fileReferences"] = contracts.CloneValue(input.FileReferences)
	}
	if strings.TrimSpace(input.WorkspaceCheckpointID) != "" {
		turn["workspaceCheckpointId"] = strings.TrimSpace(input.WorkspaceCheckpointID)
	}
	if strings.TrimSpace(input.Mode) != "" {
		turn["mode"] = strings.TrimSpace(input.Mode)
	}
	if input.GUIPlan != nil {
		turn["guiPlan"] = contracts.CloneMap(input.GUIPlan)
	}
	if input.DisableUserInputSet {
		turn["disableUserInput"] = input.DisableUserInput
	}
	if input.MaxModelSteps != nil {
		turn["maxModelSteps"] = float64(*input.MaxModelSteps)
	}

	turnStarted := map[string]any{
		"kind":           "turn_started",
		"threadId":       threadID,
		"turnId":         turnID,
		"status":         "running",
		"model":          strings.TrimSpace(input.Model),
		"providerId":     strings.TrimSpace(input.ProviderID),
		"approvalPolicy": strings.TrimSpace(input.ApprovalPolicy),
		"sandboxMode":    strings.TrimSpace(input.SandboxMode),
		"attachmentIds":  stringListAny(input.AttachmentIDs),
		"turn":           contracts.CloneMap(turn),
	}
	if len(input.Attachments) > 0 {
		turnStarted["attachments"] = mapsListAny(input.Attachments)
		if input.AttachmentPipeline != nil {
			turnStarted["attachmentPipeline"] = contracts.CloneMap(input.AttachmentPipeline)
		}
	}
	if len(input.FileReferences) > 0 {
		turnStarted["fileReferences"] = contracts.CloneValue(input.FileReferences)
	}
	if strings.TrimSpace(input.WorkspaceCheckpointID) != "" {
		turnStarted["workspaceCheckpointId"] = strings.TrimSpace(input.WorkspaceCheckpointID)
	}
	if strings.TrimSpace(input.Mode) != "" {
		turnStarted["mode"] = strings.TrimSpace(input.Mode)
	}
	if input.GUIPlan != nil {
		turnStarted["guiPlan"] = contracts.CloneMap(input.GUIPlan)
	}
	if input.DisableUserInputSet {
		turnStarted["disableUserInput"] = input.DisableUserInput
	}
	if input.MaxModelSteps != nil {
		turnStarted["maxModelSteps"] = float64(*input.MaxModelSteps)
	}
	return StartRecord{
		UserItemID:       userItemID,
		UserItem:         userItem,
		Turn:             turn,
		TurnStartedEvent: turnStarted,
		UserItemCreatedEvent: map[string]any{
			"kind":     "item_created",
			"threadId": threadID,
			"turnId":   turnID,
			"itemId":   userItemID,
			"item":     contracts.CloneMap(userItem),
		},
	}
}

func stringListAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func mapsListAny(items []map[string]any) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		out = append(out, contracts.CloneMap(item))
	}
	return out
}
