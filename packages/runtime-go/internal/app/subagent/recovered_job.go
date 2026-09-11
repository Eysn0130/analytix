package subagent

import (
	"errors"
	"strings"

	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

type RecoveredJobToolResultInput struct {
	ThreadID string
	TurnID   string
	Record   domainjob.Record
	Message  string
	CallID   string
	ToolName string
}

func RecoveredJobToolResult(input RecoveredJobToolResultInput) (toolcatalogapp.ToolResultRecords, error) {
	timestamp := RecoveredJobSettlementTimestamp(input.Record)
	if timestamp == "" {
		return toolcatalogapp.ToolResultRecords{}, toolcatalogapp.ErrToolResultSettlementInvalid
	}
	output := RunOutput(input.Record, "", input.Record.Usage, errors.New("runtime recovered job interrupted"))
	if !SecurityBoundChildOutput(input.Record) {
		output["code"] = "runtime_recovered_job_interrupted"
		output["status"] = firstNonEmptyAnyString(input.Record.Status, string(domainjob.StatusInterrupted))
	}
	toolName := strings.TrimSpace(input.ToolName)
	if toolName == "" {
		toolName = JobDefaultToolName(input.Record)
	}
	return toolcatalogapp.SettleToolResult(toolcatalogapp.ToolResultInput{
		ThreadID:   input.ThreadID,
		TurnID:     input.TurnID,
		CreatedAt:  timestamp,
		FinishedAt: timestamp,
		Call: domainmodel.ToolCall{
			ID:   input.CallID,
			Name: toolName,
		},
		Projection: toolcatalogapp.BuildPublicToolResultProjectionV1(toolName, output, true),
		IsError:    true,
	})
}

func RecoveredJobSettlementTimestamp(record domainjob.Record) string {
	for _, value := range []string{record.RecoveryUpdatedAt, record.UpdatedAt, record.FinishedAt} {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
