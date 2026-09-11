package evidence

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	domainmcpname "analytix.local/runtime-go/internal/domain/mcpname"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const ToolOutcomeVersion = 1

type TransportStatus string

const (
	TransportSuccess   TransportStatus = "success"
	TransportFailure   TransportStatus = "failure"
	TransportCancelled TransportStatus = "cancelled"
	TransportTimeout   TransportStatus = "timeout"
)

type SemanticStatus string

const (
	SemanticSuccess     SemanticStatus = "success"
	SemanticPartial     SemanticStatus = "partial"
	SemanticBlocked     SemanticStatus = "blocked"
	SemanticUnavailable SemanticStatus = "unavailable"
	SemanticFailure     SemanticStatus = "failure"
	SemanticCancelled   SemanticStatus = "cancelled"
	SemanticTimeout     SemanticStatus = "timeout"
)

// ToolOutcome is host-normalized. Case, epoch, snapshot, server, context, and
// grant fields come only from frozen host authority. Every Reported* field,
// candidate receipt, coverage object, data object, and meta value remains
// untrusted source material until a later host verifier issues a receipt.
type ToolOutcome struct {
	Version                       int              `json:"version"`
	ToolName                      string           `json:"toolName"`
	ToolCallID                    string           `json:"toolCallId"`
	ContextDigest                 string           `json:"contextDigest"`
	ExecutionGrantID              string           `json:"executionGrantId"`
	CaseID                        string           `json:"caseId"`
	ContextEpoch                  uint64           `json:"contextEpoch"`
	DatasetSnapshotID             string           `json:"datasetSnapshotId"`
	ServerIdentity                string           `json:"serverIdentity"`
	TransportStatus               TransportStatus  `json:"transportStatus"`
	SemanticStatus                SemanticStatus   `json:"semanticStatus"`
	SafeToAnswer                  bool             `json:"safeToAnswer"`
	IsError                       bool             `json:"isError"`
	Blocker                       string           `json:"blocker"`
	PartialCoverage               map[string]any   `json:"partialCoverage"`
	Data                          map[string]any   `json:"data"`
	EvidenceReceiptIDs            []string         `json:"evidenceReceiptIds"`
	CandidateEvidenceReceipts     []map[string]any `json:"candidateEvidenceReceipts"`
	ReportedSemanticStatus        string           `json:"reportedSemanticStatus"`
	ReportedSafeToAnswer          *bool            `json:"reportedSafeToAnswer"`
	ReportedCaseID                string           `json:"reportedCaseId"`
	ReportedContextEpoch          uint64           `json:"reportedContextEpoch"`
	ReportedDatasetSnapshotID     string           `json:"reportedDatasetSnapshotId"`
	ReportedServerIdentity        string           `json:"reportedServerIdentity"`
	UntrustedMeta                 map[string]any   `json:"untrustedMeta"`
	SourceAssertionsAuthoritative bool             `json:"sourceAssertionsAuthoritative"`
	IssuedAt                      string           `json:"issuedAt"`
	OutcomeDigest                 string           `json:"outcomeDigest"`
}

type ToolOutcomeInput struct {
	ToolName                  string
	ToolCallID                string
	ContextDigest             string
	ExecutionGrantID          string
	CaseID                    string
	ContextEpoch              uint64
	DatasetSnapshotID         string
	ServerIdentity            string
	TransportStatus           TransportStatus
	SemanticStatus            SemanticStatus
	IsError                   bool
	Blocker                   string
	PartialCoverage           map[string]any
	Data                      map[string]any
	CandidateEvidenceReceipts []map[string]any
	ReportedSemanticStatus    string
	ReportedSafeToAnswer      *bool
	ReportedCaseID            string
	ReportedContextEpoch      uint64
	ReportedDatasetSnapshotID string
	ReportedServerIdentity    string
	UntrustedMeta             map[string]any
	IssuedAt                  time.Time
}

func NewToolOutcome(input ToolOutcomeInput) ToolOutcome {
	issuedAt := input.IssuedAt.UTC()
	if issuedAt.IsZero() {
		issuedAt = time.Now().UTC()
	}
	outcome := ToolOutcome{
		Version: ToolOutcomeVersion, ToolName: strings.TrimSpace(input.ToolName), ToolCallID: strings.TrimSpace(input.ToolCallID),
		ContextDigest: strings.TrimSpace(input.ContextDigest), ExecutionGrantID: strings.TrimSpace(input.ExecutionGrantID),
		CaseID: strings.TrimSpace(input.CaseID), ContextEpoch: input.ContextEpoch, DatasetSnapshotID: strings.TrimSpace(input.DatasetSnapshotID),
		ServerIdentity: strings.TrimSpace(input.ServerIdentity), TransportStatus: input.TransportStatus, SemanticStatus: input.SemanticStatus,
		SafeToAnswer: false, IsError: input.IsError, Blocker: strings.TrimSpace(input.Blocker),
		PartialCoverage: cloneMap(input.PartialCoverage), Data: cloneMap(input.Data), EvidenceReceiptIDs: []string{},
		CandidateEvidenceReceipts: cloneMaps(input.CandidateEvidenceReceipts), ReportedSemanticStatus: strings.TrimSpace(input.ReportedSemanticStatus),
		ReportedSafeToAnswer: cloneBool(input.ReportedSafeToAnswer), ReportedCaseID: strings.TrimSpace(input.ReportedCaseID),
		ReportedContextEpoch: input.ReportedContextEpoch, ReportedDatasetSnapshotID: strings.TrimSpace(input.ReportedDatasetSnapshotID),
		ReportedServerIdentity: strings.TrimSpace(input.ReportedServerIdentity), UntrustedMeta: cloneMap(input.UntrustedMeta),
		SourceAssertionsAuthoritative: false, IssuedAt: issuedAt.Format(time.RFC3339Nano),
	}
	if outcome.PartialCoverage == nil {
		outcome.PartialCoverage = map[string]any{}
	}
	if outcome.Data == nil {
		outcome.Data = map[string]any{}
	}
	if outcome.CandidateEvidenceReceipts == nil {
		outcome.CandidateEvidenceReceipts = []map[string]any{}
	}
	if outcome.UntrustedMeta == nil {
		outcome.UntrustedMeta = map[string]any{}
	}
	outcome.OutcomeDigest = toolOutcomeDigest(outcome)
	return outcome
}

func ParseToolOutcome(value any) (ToolOutcome, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return ToolOutcome{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var outcome ToolOutcome
	if err := decoder.Decode(&outcome); err != nil {
		return ToolOutcome{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ToolOutcome{}, errors.New("tool outcome contains trailing JSON")
	}
	if err := ValidateToolOutcome(outcome); err != nil {
		return ToolOutcome{}, err
	}
	return outcome, nil
}

func ValidateToolOutcome(outcome ToolOutcome) error {
	if outcome.Version != ToolOutcomeVersion || strings.TrimSpace(outcome.ToolName) == "" || strings.TrimSpace(outcome.ToolCallID) == "" ||
		!validSHA256(outcome.ContextDigest) || !validSHA256(outcome.ExecutionGrantID) || strings.TrimSpace(outcome.CaseID) == "" ||
		outcome.ContextEpoch == 0 || strings.TrimSpace(outcome.DatasetSnapshotID) == "" || strings.TrimSpace(outcome.ServerIdentity) == "" ||
		!validTransportStatus(outcome.TransportStatus) || !validSemanticStatus(outcome.SemanticStatus) || outcome.PartialCoverage == nil ||
		outcome.Data == nil || outcome.EvidenceReceiptIDs == nil || outcome.CandidateEvidenceReceipts == nil || outcome.UntrustedMeta == nil ||
		outcome.SourceAssertionsAuthoritative || strings.TrimSpace(outcome.IssuedAt) == "" || !validSHA256(outcome.OutcomeDigest) {
		return errors.New("tool outcome is incomplete")
	}
	if _, err := time.Parse(time.RFC3339Nano, outcome.IssuedAt); err != nil {
		return errors.New("tool outcome issuedAt is invalid")
	}
	serverID, _, isMCP := domainmcpname.Parse(outcome.ToolName)
	if outcome.ServerIdentity == "host:builtin" {
		if isMCP {
			return errors.New("MCP tool outcome has builtin server identity")
		}
	} else {
		identity, err := domainsecurity.ParseVerifiedMCPServerIdentity(outcome.ServerIdentity)
		if err != nil || !isMCP || !domainsecurity.VerifiedMCPServerIdentityCanAuthorizeFacts(identity) || identity.ServerID != serverID {
			return errors.New("tool outcome server identity is invalid")
		}
	}
	if outcome.SafeToAnswer && len(outcome.EvidenceReceiptIDs) == 0 {
		return errors.New("tool outcome cannot be safe without host evidence receipts")
	}
	if outcome.SafeToAnswer && (outcome.IsError || strings.TrimSpace(outcome.Blocker) != "" ||
		(outcome.SemanticStatus != SemanticSuccess && outcome.SemanticStatus != SemanticPartial)) {
		return errors.New("tool outcome cannot be safe while blocked or failed")
	}
	if outcome.TransportStatus != TransportSuccess && !outcome.IsError {
		return errors.New("failed tool transport must be an error")
	}
	if outcome.SemanticStatus == SemanticSuccess && outcome.TransportStatus != TransportSuccess {
		return errors.New("semantic success requires successful transport")
	}
	if outcome.SemanticStatus == SemanticSuccess && (outcome.IsError || strings.TrimSpace(outcome.Blocker) != "" || toolOutcomeCoverageIncomplete(outcome.PartialCoverage, 0)) {
		return errors.New("semantic success conflicts with negative outcome evidence")
	}
	if err := ValidateCoverageDescriptor(outcome.PartialCoverage); err != nil {
		return errors.New("tool outcome coverage descriptor is invalid")
	}
	if outcome.SemanticStatus == SemanticPartial && (len(outcome.PartialCoverage) == 0 || !toolOutcomeCoverageIncomplete(outcome.PartialCoverage, 0)) {
		return errors.New("partial semantic outcome requires explicit incomplete coverage")
	}
	if outcome.SemanticStatus != SemanticSuccess && outcome.SemanticStatus != SemanticPartial && !outcome.IsError {
		return errors.New("unusable semantic outcome must be an error")
	}
	if expected := toolOutcomeDigest(outcome); expected != outcome.OutcomeDigest {
		return errors.New("tool outcome integrity is invalid")
	}
	return nil
}

func toolOutcomeCoverageIncomplete(value any, depth int) bool {
	if depth > 12 {
		return true
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			switch key {
			case "partial", "partialCoverage", "partial_coverage", "truncated", "hasMore", "has_more", "support_payload_truncated", "coverageConflict":
				if flag, ok := child.(bool); ok && flag {
					return true
				}
			case "complete", "coverageComplete", "coverage_complete", "paginationComplete", "pagination_complete", "source_coverage_complete":
				if flag, ok := child.(bool); ok && !flag {
					return true
				}
			case "coverageStatus", "coverage_status", "paginationCompleteness", "pagination_completeness", "status":
				if text, ok := child.(string); ok {
					switch strings.ToLower(strings.TrimSpace(text)) {
					case "partial", "incomplete", "unknown", "truncated":
						return true
					}
				}
			}
			if toolOutcomeCoverageIncomplete(child, depth+1) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if toolOutcomeCoverageIncomplete(child, depth+1) {
				return true
			}
		}
	}
	return false
}

func ToolOutcomeRecord(outcome ToolOutcome) map[string]any {
	body, _ := json.Marshal(outcome)
	record := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	_ = decoder.Decode(&record)
	return record
}

func ToolOutcomeWithRegisteredEvidence(outcome ToolOutcome, evidence []RegisteredEvidence) (ToolOutcome, error) {
	if err := ValidateToolOutcome(outcome); err != nil || len(evidence) == 0 || outcome.TransportStatus != TransportSuccess || outcome.IsError ||
		(outcome.SemanticStatus != SemanticSuccess && outcome.SemanticStatus != SemanticPartial) {
		return ToolOutcome{}, errors.New("tool outcome cannot accept evidence")
	}
	receiptIDs := make([]string, 0, len(evidence))
	seen := map[string]bool{}
	for _, registered := range evidence {
		receipt := registered.Receipt
		if registered.Revoked || ValidateEvidenceReceipt(receipt) != nil || receipt.ContextDigest != outcome.ContextDigest ||
			receipt.ExecutionGrantID != outcome.ExecutionGrantID || receipt.ToolCallID != outcome.ToolCallID || receipt.ToolName != outcome.ToolName ||
			receipt.CaseID != outcome.CaseID || receipt.ContextEpoch != outcome.ContextEpoch || receipt.DatasetSnapshotID != outcome.DatasetSnapshotID ||
			receipt.ServerIdentity != outcome.ServerIdentity ||
			(outcome.SemanticStatus == SemanticSuccess && receipt.PaginationCompleteness != PaginationComplete) ||
			seen[receipt.ReceiptID] {
			return ToolOutcome{}, errors.New("registered evidence does not match tool outcome")
		}
		seen[receipt.ReceiptID] = true
		receiptIDs = append(receiptIDs, receipt.ReceiptID)
	}
	outcome.EvidenceReceiptIDs = receiptIDs
	outcome.SafeToAnswer = true
	outcome.OutcomeDigest = toolOutcomeDigest(outcome)
	if err := ValidateToolOutcome(outcome); err != nil {
		return ToolOutcome{}, err
	}
	return outcome, nil
}

func validTransportStatus(status TransportStatus) bool {
	switch status {
	case TransportSuccess, TransportFailure, TransportCancelled, TransportTimeout:
		return true
	default:
		return false
	}
}

func validSemanticStatus(status SemanticStatus) bool {
	switch status {
	case SemanticSuccess, SemanticPartial, SemanticBlocked, SemanticUnavailable, SemanticFailure, SemanticCancelled, SemanticTimeout:
		return true
	default:
		return false
	}
}

func toolOutcomeDigest(outcome ToolOutcome) string {
	outcome.OutcomeDigest = ""
	body, _ := json.Marshal(outcome)
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func validSHA256(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	body, _ := json.Marshal(value)
	out := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	_ = decoder.Decode(&out)
	return out
}

func cloneMaps(values []map[string]any) []map[string]any {
	if values == nil {
		return nil
	}
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		out = append(out, cloneMap(value))
	}
	return out
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
