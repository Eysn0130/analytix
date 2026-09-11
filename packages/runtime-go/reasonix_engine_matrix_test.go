//go:build !analytix_prod

package runtimego

import (
	"net/http"
	"net/http/httptest"
	"testing"

	upstreamaudit "analytix.local/runtime-go/internal/upstreamaudit"
)

func TestReasonixCapabilityAuditMatrixIsMachineReadable(t *testing.T) {
	matrix := upstreamaudit.BuildReasonixCapabilityAuditMatrix()
	if matrix.SchemaVersion != 1 || matrix.ChangeID != "reasonix-capability-audit" {
		t.Fatalf("matrix identity mismatch: %#v", matrix)
	}
	if matrix.SourcePath != upstreamaudit.ReasonixSourcePath || matrix.SourceCommit != upstreamaudit.ReasonixSourceCommit {
		t.Fatalf("matrix must pin the local Reasonix source checkout: %#v", matrix)
	}
	if !matrix.ReadyForG6 || !matrix.CapabilityMatrixGreen {
		t.Fatalf("Reasonix capability audit should be G6-clear after AutoResearch project state is code-level absorbed: %#v", matrix)
	}
	if matrix.DefaultBackendReady || len(matrix.ReadinessSemantics) == 0 {
		t.Fatalf("Reasonix capability audit green must not be default-backend readiness: %#v", matrix)
	}
	if matrix.GreenCount != 12 || matrix.RedCount != 0 || matrix.DeferredCount != 0 || matrix.RejectedCount != 1 {
		t.Fatalf("matrix status counts drifted: %#v", matrix)
	}
	if matrix.ForbiddenPublicProtocolRowCount != 0 ||
		matrix.RendererContractChangedRowCount != 0 ||
		matrix.ProductIdentityChangedRowCount != 0 ||
		matrix.KunProductEntryDriftRowCount != 0 {
		t.Fatalf("Reasonix capability audit must not import forbidden upstream/product surfaces: %#v", matrix)
	}
	if len(matrix.PostG6DeleteCandidates) != 4 || len(matrix.G6Blockers) != 0 {
		t.Fatalf("delete/blocker inventory mismatch: %#v", matrix)
	}

	rows := map[string]upstreamaudit.ReasonixCapabilityAuditRow{}
	for _, row := range matrix.Rows {
		if row.ID == "" || row.Capability == "" || row.AnalytixLanding == "" {
			t.Fatalf("row must be machine readable: %#v", row)
		}
		if len(row.ReasonixSources) == 0 || len(row.MachineChecks) == 0 {
			t.Fatalf("row must point to Reasonix sources and machine checks: %#v", row)
		}
		for _, check := range row.MachineChecks {
			if check.ID == "" || check.Kind == "" || check.Status == "" {
				t.Fatalf("machine check must be complete: row=%s check=%#v", row.ID, check)
			}
		}
		switch row.Status {
		case upstreamaudit.AbsorptionStatusGreen:
			if len(row.AnalytixEvidence) == 0 {
				t.Fatalf("green row needs code-level analytix evidence: %#v", row)
			}
			for _, check := range row.MachineChecks {
				if check.Status != "passed" || check.Evidence == "" {
					t.Fatalf("green row needs passing evidence checks: row=%s check=%#v", row.ID, check)
				}
			}
		case upstreamaudit.AbsorptionStatusRed, upstreamaudit.AbsorptionStatusDeferred:
			if len(row.Blockers) == 0 {
				t.Fatalf("red/deferred row needs explicit blocker: %#v", row)
			}
			if !rowHasRedMachineCheck(row) {
				t.Fatalf("red/deferred row needs a failing machine check id: %#v", row)
			}
		case upstreamaudit.AbsorptionStatusRejected:
			if row.AbsorptionClass != upstreamaudit.AbsorptionReject {
				t.Fatalf("rejected row must use reject classification: %#v", row)
			}
		default:
			t.Fatalf("unknown row status: %#v", row)
		}
		rows[row.ID] = row
	}

	expectedClasses := map[string]upstreamaudit.RuntimeAbsorptionClass{
		"provider-cache-streaming":     upstreamaudit.AbsorptionCodePortAndAdapt,
		"multi-model-non-regression":   upstreamaudit.AbsorptionContractReimplement,
		"deepseek-prefix-cache":        upstreamaudit.AbsorptionCodePortAndAdapt,
		"context-contract-maintenance": upstreamaudit.AbsorptionContractReimplement,
		"approval-user-input-gates":    upstreamaudit.AbsorptionContractReimplement,
		"mcp-lifecycle-search-call":    upstreamaudit.AbsorptionContractReimplement,
		"job-subagent-lineage":         upstreamaudit.AbsorptionContractReimplement,
		"durable-replay":               upstreamaudit.AbsorptionContractReimplement,
		"crash-restart-recovery":       upstreamaudit.AbsorptionContractReimplement,
		"candidate-rollback":           upstreamaudit.AbsorptionReplace,
		"goal-evidence-kernel":         upstreamaudit.AbsorptionContractReimplement,
		"autoresearch-project-state":   upstreamaudit.AbsorptionContractReimplement,
		"reasonix-public-protocol":     upstreamaudit.AbsorptionReject,
	}
	for id, class := range expectedClasses {
		row, ok := rows[id]
		if !ok {
			t.Fatalf("missing Reasonix capability audit row %s", id)
		}
		if row.AbsorptionClass != class {
			t.Fatalf("row %s class mismatch: got %s want %s", id, row.AbsorptionClass, class)
		}
	}
}

func TestRuntimeInfoDoesNotExposeReasonixCapabilityMatrixOrProtocol(t *testing.T) {
	g1 := loadG1Contract(t)
	g2 := loadG2Contract(t)
	server := httptest.NewServer(newRuntimeServerContractTestHandler(t, RuntimeServerContractConfig{
		RuntimeToken:   g1.RuntimeToken,
		StartedAt:      g1.StartedAt,
		Routes:         g2.Routes,
		DurableTempDir: t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		DataDir:        t.TempDir(),
	}))
	defer server.Close()

	info := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/runtime/info", g1.RuntimeToken, nil, http.StatusOK)
	capabilities := mapField(t, info, "capabilities")
	if _, exposed := capabilities["upstreamAbsorption"]; exposed {
		t.Fatalf("public runtime info must not expose internal upstream audit matrices: %#v", capabilities)
	}
	forbidden := assertLiveJSON(t, server.URL, http.MethodGet, "/v1/reasonix", g1.RuntimeToken, nil, http.StatusNotFound)
	if forbidden["code"] != "not_found" {
		t.Fatalf("matrix must not add Reasonix public routes: %#v", forbidden)
	}
}

func rowHasRedMachineCheck(row upstreamaudit.ReasonixCapabilityAuditRow) bool {
	for _, check := range row.MachineChecks {
		if check.Status == "red" || check.Status == "failed" || check.Status == "missing" {
			return true
		}
	}
	return false
}
