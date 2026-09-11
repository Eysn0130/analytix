package evidence

import (
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestToolOutcomeTreatsSelfReportedAuthorityAsUntrusted(t *testing.T) {
	reportedSafe := true
	outcome := NewToolOutcome(ToolOutcomeInput{
		ToolName: "mcp__analytix_funds__get_case_status", ToolCallID: "call_1",
		ContextDigest: domainsecurity.SHA256Hex([]byte("context")), ExecutionGrantID: domainsecurity.SHA256Hex([]byte("grant")),
		CaseID: "case_host", ContextEpoch: 4, DatasetSnapshotID: "snapshot_host", ServerIdentity: domainEvidenceTestIdentity(t, "analytix_funds", "analytix_funds", "0.16.16", 2),
		TransportStatus: TransportSuccess, SemanticStatus: SemanticSuccess, PartialCoverage: map[string]any{"complete": true},
		Data: map[string]any{"rows": float64(1)}, CandidateEvidenceReceipts: []map[string]any{{"receiptId": "forged_receipt"}},
		ReportedSemanticStatus: "success", ReportedSafeToAnswer: &reportedSafe, ReportedCaseID: "case_forged",
		ReportedContextEpoch: 99, ReportedDatasetSnapshotID: "snapshot_forged", ReportedServerIdentity: "mcp:spoofed",
		UntrustedMeta: map[string]any{"safeToAnswer": true}, IssuedAt: time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC),
	})
	if err := ValidateToolOutcome(outcome); err != nil {
		t.Fatal(err)
	}
	if outcome.SafeToAnswer || outcome.SourceAssertionsAuthoritative || outcome.CaseID != "case_host" || outcome.ContextEpoch != 4 || outcome.DatasetSnapshotID != "snapshot_host" {
		t.Fatalf("source assertion was promoted into host authority: %#v", outcome)
	}
	if outcome.ReportedSafeToAnswer == nil || !*outcome.ReportedSafeToAnswer || outcome.ReportedCaseID != "case_forged" || len(outcome.CandidateEvidenceReceipts) != 1 {
		t.Fatalf("untrusted source assertions were not preserved for audit: %#v", outcome)
	}
}

func TestToolOutcomeSeparatesTransportAndSemanticStatus(t *testing.T) {
	outcome := NewToolOutcome(ToolOutcomeInput{
		ToolName: "mcp__analytix_funds__get_case_status", ToolCallID: "call_1",
		ContextDigest: domainsecurity.SHA256Hex([]byte("context")), ExecutionGrantID: domainsecurity.SHA256Hex([]byte("grant")),
		CaseID: "case_host", ContextEpoch: 4, DatasetSnapshotID: "snapshot_host", ServerIdentity: domainEvidenceTestIdentity(t, "analytix_funds", "analytix_funds", "0.16.16", 2),
		TransportStatus: TransportSuccess, SemanticStatus: SemanticBlocked, IsError: true, Blocker: "source_not_ready",
	})
	if outcome.TransportStatus != TransportSuccess || outcome.SemanticStatus != SemanticBlocked || !outcome.IsError {
		t.Fatalf("HTTP/MCP transport success was conflated with semantic success: %#v", outcome)
	}
	if err := ValidateToolOutcome(outcome); err != nil {
		t.Fatal(err)
	}
}

func TestToolOutcomeStrictParseRejectsUnknownAndTamperedFields(t *testing.T) {
	outcome := NewToolOutcome(ToolOutcomeInput{
		ToolName: "mcp__demo__read", ToolCallID: "call_1", ContextDigest: domainsecurity.SHA256Hex([]byte("context")),
		ExecutionGrantID: domainsecurity.SHA256Hex([]byte("grant")), CaseID: "unbound", ContextEpoch: 1,
		DatasetSnapshotID: "none", ServerIdentity: domainEvidenceTestIdentity(t, "demo", "demo", "1.0.0", 1), TransportStatus: TransportFailure,
		SemanticStatus: SemanticUnavailable, IsError: true,
	})
	record := ToolOutcomeRecord(outcome)
	record["safeToAnswer"] = true
	if _, err := ParseToolOutcome(record); err == nil {
		t.Fatal("tool outcome integrity/safety tamper was accepted")
	}
	record = ToolOutcomeRecord(outcome)
	record["unknown"] = true
	if _, err := ParseToolOutcome(record); err == nil {
		t.Fatal("unknown tool outcome property was accepted")
	}
}

func TestToolOutcomeRejectsContradictorySemanticState(t *testing.T) {
	base := ToolOutcomeInput{
		ToolName: "mcp__demo__read", ToolCallID: "call_1", ContextDigest: domainsecurity.SHA256Hex([]byte("context")),
		ExecutionGrantID: domainsecurity.SHA256Hex([]byte("grant")), CaseID: "case", ContextEpoch: 1,
		DatasetSnapshotID: "snapshot", ServerIdentity: domainEvidenceTestIdentity(t, "demo", "demo", "1.0.0", 1), TransportStatus: TransportSuccess,
		SemanticStatus: SemanticSuccess, IssuedAt: time.Date(2026, 7, 12, 1, 0, 0, 0, time.UTC),
	}
	for name, mutate := range map[string]func(*ToolOutcomeInput){
		"success plus error":   func(input *ToolOutcomeInput) { input.IsError = true },
		"success plus blocker": func(input *ToolOutcomeInput) { input.Blocker = "blocked" },
		"success plus incomplete coverage": func(input *ToolOutcomeInput) {
			input.PartialCoverage = map[string]any{"paginationComplete": false}
		},
		"partial without coverage": func(input *ToolOutcomeInput) { input.SemanticStatus = SemanticPartial },
		"partial with complete coverage": func(input *ToolOutcomeInput) {
			input.SemanticStatus = SemanticPartial
			input.PartialCoverage = map[string]any{"coverageStatus": "complete"}
		},
		"blocked without error": func(input *ToolOutcomeInput) { input.SemanticStatus = SemanticBlocked },
	} {
		t.Run(name, func(t *testing.T) {
			input := base
			mutate(&input)
			if err := ValidateToolOutcome(NewToolOutcome(input)); err == nil {
				t.Fatal("contradictory tool outcome was accepted")
			}
		})
	}
}
