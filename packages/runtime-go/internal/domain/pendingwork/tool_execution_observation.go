package pendingwork

import (
	"errors"
	"strings"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

const (
	SuccessfulToolExecutionObservationSchemaVersionV1 = 1
	ToolExecutionObservationDisclosureV1              = "metadata_only"
)

// SuccessfulToolExecutionObservationV1 is a closed public projection of one
// host-verified ordinary tool execution. It deliberately carries no command,
// workspace, arguments, output, provider content, case data, or PII.
type SuccessfulToolExecutionObservationV1 struct {
	SchemaVersion          int    `json:"schemaVersion"`
	Disclosure             string `json:"disclosure"`
	PrivatePayloadWithheld bool   `json:"privatePayloadWithheld"`
	ThreadID               string `json:"threadId"`
	TurnID                 string `json:"turnId"`
	ToolName               string `json:"toolName"`
	Status                 string `json:"status"`
	WorkID                 string `json:"workId"`
	ReceiptID              string `json:"receiptId"`
	DispositionID          string `json:"dispositionId"`
	ExecutionGrantID       string `json:"executionGrantId"`
	ResultItemID           string `json:"resultItemId"`
	ResultItemDigest       string `json:"resultItemDigest"`
}

func ValidateSuccessfulToolExecutionObservationV1(observation SuccessfulToolExecutionObservationV1) error {
	if observation.SchemaVersion != SuccessfulToolExecutionObservationSchemaVersionV1 ||
		observation.Disclosure != ToolExecutionObservationDisclosureV1 ||
		!observation.PrivatePayloadWithheld || observation.Status != StatusCompleted ||
		!toolExecutionObservationToolAllowedV1(observation.ToolName) || !canonicalOpaqueObservationID(observation.ThreadID) ||
		!canonicalOpaqueObservationID(observation.TurnID) || !domaintoolresult.IsToolResultItemIDV1(observation.ResultItemID) ||
		!domainsecurity.IsSHA256Hex(observation.WorkID) || !domainsecurity.IsSHA256Hex(observation.ReceiptID) ||
		!domainsecurity.IsSHA256Hex(observation.DispositionID) || !domainsecurity.IsSHA256Hex(observation.ExecutionGrantID) ||
		!domainsecurity.IsSHA256Hex(observation.ResultItemDigest) {
		return errors.New("successful tool execution observation is invalid")
	}
	return nil
}

func toolExecutionObservationToolAllowedV1(toolName string) bool {
	return toolName == "bash" || toolName == "read"
}

func canonicalOpaqueObservationID(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && len([]byte(value)) <= 4096
}
