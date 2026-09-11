package control

import (
	"encoding/json"
	"strings"

	contracts "analytix.local/runtime-go/internal/contracts"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func NormalizeStartTurnRequest(request StartTurnRequest) StartTurnRequest {
	request.ThreadID = strings.TrimSpace(request.ThreadID)
	request.RiskIntent = NormalizeRiskIntent(request.RiskIntent)
	request.Model = strings.TrimSpace(request.Model)
	request.ProviderID = strings.TrimSpace(request.ProviderID)
	request.EndpointFormat = strings.TrimSpace(request.EndpointFormat)
	request.ReasoningEffort = NormalizeReasoningEffort(request.ReasoningEffort)
	request.Mode = NormalizeTurnMode(request.Mode)
	request.ApprovalPolicy = NormalizeApprovalPolicy(request.ApprovalPolicy)
	request.SandboxMode = NormalizeSandboxMode(request.SandboxMode)
	request.AttachmentIDs = normalizeStringSlice(request.AttachmentIDs)
	request.FileReferences = NormalizeFileReferences(request.FileReferences)
	request.GUIPlan = contracts.CloneMap(request.GUIPlan)
	request.WorkspaceCheckpointID = strings.TrimSpace(request.WorkspaceCheckpointID)
	request.MaxModelSteps = cloneOptionalInt(request.MaxModelSteps)
	request.InternalToolScope = normalizeStringSlice(request.InternalToolScope)
	request.InternalUsageSource = strings.TrimSpace(request.InternalUsageSource)
	request.InternalChildRunID = strings.TrimSpace(request.InternalChildRunID)
	if request.InternalOutputTokenBudget < 0 {
		request.InternalOutputTokenBudget = 0
	}
	return request
}

// NormalizeSteerTurnRequest applies the same raise-only risk-hint rule as a
// foreground start request. The current frozen-context risk decision remains
// host-owned; this function only carries a validated request hint to that
// later boundary.
func NormalizeSteerTurnRequest(request SteerTurnRequest) SteerTurnRequest {
	request.ThreadID = strings.TrimSpace(request.ThreadID)
	request.TurnID = strings.TrimSpace(request.TurnID)
	request.Text = strings.TrimSpace(request.Text)
	request.DisplayText = strings.TrimSpace(request.DisplayText)
	request.RiskIntent = NormalizeRiskIntent(request.RiskIntent)
	request.ClientUserMessageID = strings.TrimSpace(request.ClientUserMessageID)
	request.ExpectedTurnID = strings.TrimSpace(request.ExpectedTurnID)
	request.AttachmentIDs = normalizeStringSlice(request.AttachmentIDs)
	request.FileReferences = NormalizeFileReferences(request.FileReferences)
	return request
}

// NormalizeRiskIntent keeps the external control-plane input raise-only. A
// caller may request case handling, but may never request a downgrade to a
// general publication policy or supply any host-owned publication state.
func NormalizeRiskIntent(value string) string {
	if strings.TrimSpace(value) == "case" {
		return "case"
	}
	return ""
}

func NormalizeTurnMode(value string) string {
	switch strings.TrimSpace(value) {
	case "agent", "plan":
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func NormalizeReasoningEffort(value string) string {
	projected, _ := domainmodel.ProjectReasoningEffortV1(value)
	return projected
}

func NormalizeApprovalPolicy(value string) string {
	switch strings.TrimSpace(value) {
	case "always", "auto", "on-request", "untrusted", "suggest", "never":
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func NormalizeSandboxMode(value string) string {
	switch strings.TrimSpace(value) {
	case "read-only", "workspace-write", "danger-full-access", "external-sandbox":
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func NormalizeFileReferences(value []any) []any {
	out := make([]any, 0, len(value))
	for _, item := range value {
		record, ok := item.(map[string]any)
		if !ok {
			continue
		}
		path := strings.TrimSpace(stringField(record, "path"))
		relativePath := strings.TrimSpace(stringField(record, "relativePath"))
		name := strings.TrimSpace(stringField(record, "name"))
		if path == "" || relativePath == "" || name == "" {
			continue
		}
		normalized := contracts.CloneMap(record)
		normalized["path"] = path
		normalized["relativePath"] = relativePath
		normalized["name"] = name
		if kind := strings.TrimSpace(stringField(record, "kind")); kind == "file" || kind == "directory" {
			normalized["kind"] = kind
		} else {
			delete(normalized, "kind")
		}
		out = append(out, normalized)
	}
	return out
}

func normalizeStringSlice(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		text := strings.TrimSpace(value)
		if text != "" {
			out = append(out, text)
		}
	}
	return out
}

func cloneOptionalInt(input *int) *int {
	if input == nil {
		return nil
	}
	value := *input
	return &value
}

func stringField(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return value
}

func NumericAny(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		return int(parsed), err == nil
	default:
		return 0, false
	}
}
