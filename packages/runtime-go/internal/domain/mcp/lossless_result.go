package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// HostRawToolResultKey is an in-process-only carrier used between the MCP
// transport, manager, and evidence settlement. The value must be removed
// before any model, event, thread, log, or renderer projection is built.
const HostRawToolResultKey = "_analytixHostRawToolResult"

// ToolResultObservation is a host-private, exact-raw-derived projection of
// remote MCP assertions. None of its fields are authority: they are retained
// only so the host can monotonically downgrade an outcome and audit what the
// source reported without trusting the provider-facing result projection.
// RawSHA256 binds the observation to the exact tools/call result bytes.
type ToolResultObservation struct {
	RawSHA256                 string
	SemanticStatus            string
	IsError                   bool
	Blocker                   string
	PartialCoverage           map[string]any
	CandidateEvidenceReceipts []map[string]any
	ReportedSemanticStatus    string
	ReportedSafeToAnswer      *bool
	ReportedCaseID            string
	ReportedContextEpoch      uint64
	ReportedDatasetSnapshotID string
	ReportedServerIdentity    string
	UntrustedMeta             map[string]any
}

// LosslessToolResult preserves the exact MCP tools/call result bytes alongside
// a UseNumber-decoded value. Its fields are deliberately excluded from JSON so
// an accidental generic marshal cannot persist raw evidence bytes.
type LosslessToolResult struct {
	Value       any                    `json:"-"`
	RawResult   json.RawMessage        `json:"-"`
	RawSHA256   string                 `json:"-"`
	Observation *ToolResultObservation `json:"-"`
	// HostPrivate is a process-local carrier slot. Only a host-owned adapter may
	// populate it after recomputing Value and RawSHA256 from RawResult. Generic
	// MCP results and every serialized projection must ignore it.
	HostPrivate any `json:"-"`
}

func ValidLosslessToolResult(result LosslessToolResult) bool {
	if len(result.RawResult) == 0 || len(strings.TrimSpace(result.RawSHA256)) != 64 {
		return false
	}
	digest := sha256.Sum256(result.RawResult)
	return strings.EqualFold(strings.TrimSpace(result.RawSHA256), hex.EncodeToString(digest[:]))
}

func ValidToolResultObservation(result LosslessToolResult) bool {
	if !ValidLosslessToolResult(result) || result.Observation == nil ||
		!strings.EqualFold(strings.TrimSpace(result.Observation.RawSHA256), strings.TrimSpace(result.RawSHA256)) ||
		result.Observation.PartialCoverage == nil || result.Observation.CandidateEvidenceReceipts == nil ||
		result.Observation.UntrustedMeta == nil {
		return false
	}
	return true
}
