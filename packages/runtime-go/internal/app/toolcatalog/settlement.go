package toolcatalog

import (
	"errors"
	"strings"
	"time"

	"analytix.local/runtime-go/internal/contracts"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

type ToolResultInput struct {
	ThreadID           string
	TurnID             string
	CreatedAt          string
	FinishedAt         string
	Call               domainmodel.ToolCall
	Projection         domaintoolresult.PublicToolResultProjectionV1
	IsError            bool
	ContextDigest      string
	ContextEpoch       uint64
	ExecutionGrantID   string
	CaseID             string
	DatasetSnapshotID  string
	ServerIdentity     string
	PrivateCaseOutcome *domainevidence.ToolOutcome
	EvidenceSettlement *domainevidence.HostEvidenceSettlementMarker
}

type ToolResultRecords struct {
	Status       string
	ResultItem   map[string]any
	Event        map[string]any
	ModelReply   domainmodel.Message
	ResultItemID string
}

var ErrToolResultSettlementInvalid = errors.New("tool result settlement input is invalid")

func SettleToolResult(input ToolResultInput) (ToolResultRecords, error) {
	if strings.TrimSpace(input.ThreadID) == "" || strings.TrimSpace(input.TurnID) == "" ||
		strings.TrimSpace(input.Call.Name) == "" || !domainmodel.IsHostToolCallIDV1(input.Call.ID) {
		return ToolResultRecords{}, ErrToolResultSettlementInvalid
	}
	resultItemID := ToolResultItemID(input.TurnID, input.Call.ID)
	if resultItemID == "" || !domaintoolresult.IsToolResultItemIDV1(resultItemID) {
		return ToolResultRecords{}, ErrToolResultSettlementInvalid
	}
	status, err := domaintoolresult.SettlementLifecycleStatusV1(input.Projection, input.IsError)
	if err != nil {
		return ToolResultRecords{}, errors.Join(ErrToolResultSettlementInvalid, err)
	}
	if err := validatePrivateCaseSettlement(input); err != nil {
		return ToolResultRecords{}, ErrToolResultSettlementInvalid
	}
	var caseSourceBindingProof map[string]any
	if input.Projection.ProjectionKind == domaintoolresult.ProjectionCaseSourceStatus {
		proof, proofErr := domaintoolresult.NewCaseSourceBindingProofV1(domaintoolresult.CaseSourceBindingInputV1{
			ToolName: input.Call.Name, ToolCallID: input.Call.ID,
			ContextDigest: input.ContextDigest, ContextEpoch: input.ContextEpoch, ExecutionGrantID: input.ExecutionGrantID,
			Projection: input.Projection, IsError: input.IsError,
		})
		if proofErr != nil {
			return ToolResultRecords{}, errors.Join(ErrToolResultSettlementInvalid, proofErr)
		}
		caseSourceBindingProof = domaintoolresult.CaseSourceBindingProofRecordV1(proof)
	}
	if input.EvidenceSettlement != nil && domainevidence.ValidateHostEvidenceSettlementMarker(*input.EvidenceSettlement) != nil {
		return ToolResultRecords{}, ErrToolResultSettlementInvalid
	}
	createdAt := input.CreatedAt
	if createdAt == "" {
		createdAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	finishedAt := input.FinishedAt
	if finishedAt == "" {
		finishedAt = createdAt
	}
	publicOutput := domaintoolresult.PublicToolResultProjectionRecordV1(input.Projection)
	resultItem := map[string]any{
		"id":         resultItemID,
		"turnId":     input.TurnID,
		"threadId":   input.ThreadID,
		"role":       "tool",
		"status":     status,
		"createdAt":  createdAt,
		"finishedAt": finishedAt,
		"kind":       "tool_result",
		"toolName":   input.Call.Name,
		"callId":     input.Call.ID,
		"toolKind":   ToolKind(input.Call.Name),
		"output":     publicOutput,
		"isError":    input.IsError,
	}
	authorityPresent := strings.TrimSpace(input.ContextDigest) != "" || input.ContextEpoch != 0 ||
		strings.TrimSpace(input.ExecutionGrantID) != "" || input.EvidenceSettlement != nil
	if authorityPresent {
		// Preserve partial authority inputs so the atomic durable append can
		// reject them. Never silently downgrade a malformed bound settlement
		// into an authority-free historical result.
		resultItem["contextDigest"] = input.ContextDigest
		resultItem["contextEpoch"] = input.ContextEpoch
		resultItem["executionGrantId"] = input.ExecutionGrantID
	}
	if input.EvidenceSettlement != nil {
		markerRecord, markerErr := domainevidence.HostEvidenceSettlementMarkerRecordV1(*input.EvidenceSettlement)
		if markerErr != nil {
			return ToolResultRecords{}, ErrToolResultSettlementInvalid
		}
		resultItem["hostEvidenceSettlement"] = markerRecord
	}
	if caseSourceBindingProof != nil {
		resultItem[domaintoolresult.CaseSourceBindingProofFieldV1] = caseSourceBindingProof
	}
	eventItem := contracts.CloneMap(resultItem)
	delete(eventItem, "hostEvidenceSettlement")
	event := map[string]any{
		"kind":     "tool_call_finished",
		"threadId": input.ThreadID,
		"turnId":   input.TurnID,
		"itemId":   resultItemID,
		"item":     eventItem,
	}
	if authorityPresent {
		event["contextDigest"] = input.ContextDigest
		event["contextEpoch"] = input.ContextEpoch
		event["executionGrantId"] = input.ExecutionGrantID
	}
	return ToolResultRecords{
		Status:       status,
		ResultItem:   resultItem,
		ResultItemID: resultItemID,
		Event:        event,
		ModelReply: domainmodel.Message{
			Role:       "tool",
			Name:       input.Call.Name,
			ToolCallID: input.Call.ID,
		},
	}, nil
}

// validatePrivateCaseSettlement keeps host settlement authority out of the
// serialized public projection. The original host-normalized ToolOutcome is
// process-local input and must still bind every frozen execution coordinate.
func validatePrivateCaseSettlement(input ToolResultInput) error {
	isCaseProjection := input.Projection.ProjectionKind == domaintoolresult.ProjectionCaseSourceStatus
	if isCaseProjection != (input.PrivateCaseOutcome != nil) {
		return ErrToolResultSettlementInvalid
	}
	if !isCaseProjection {
		return nil
	}
	outcome := *input.PrivateCaseOutcome
	if domainevidence.ValidateToolOutcome(outcome) != nil || outcome.ToolName != input.Call.Name || outcome.ToolCallID != input.Call.ID ||
		outcome.ContextDigest != input.ContextDigest || outcome.ContextEpoch != input.ContextEpoch ||
		outcome.ExecutionGrantID != input.ExecutionGrantID || outcome.CaseID != input.CaseID ||
		outcome.DatasetSnapshotID != input.DatasetSnapshotID || outcome.ServerIdentity != input.ServerIdentity ||
		outcome.IsError != input.IsError || outcome.SafeToAnswer || len(outcome.EvidenceReceiptIDs) != 0 {
		return ErrToolResultSettlementInvalid
	}
	if input.EvidenceSettlement != nil && (outcome.TransportStatus != domainevidence.TransportSuccess || outcome.IsError ||
		strings.TrimSpace(outcome.Blocker) != "" ||
		(outcome.SemanticStatus != domainevidence.SemanticSuccess && outcome.SemanticStatus != domainevidence.SemanticPartial)) {
		return ErrToolResultSettlementInvalid
	}
	return nil
}

func ToolResultMessage(call domainmodel.ToolCall, content string) domainmodel.Message {
	return domainmodel.Message{
		Role:       "tool",
		Name:       call.Name,
		ToolCallID: call.ID,
		Content:    content,
	}
}

func ToolResultItemID(turnID string, callID string) string {
	return domaintoolresult.ToolResultItemIDV1(turnID, callID)
}
