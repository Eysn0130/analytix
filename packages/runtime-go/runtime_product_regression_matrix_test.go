//go:build !analytix_prod

package runtimego

import (
	"testing"

	upstreamaudit "analytix.local/runtime-go/internal/upstreamaudit"
)

func TestProductRegressionMatrixCoversRequiredHighRiskSurfaces(t *testing.T) {
	matrix := upstreamaudit.BuildProductRegressionMatrix()
	if matrix.SchemaVersion != 1 ||
		matrix.ChangeID != upstreamaudit.ProductRegressionMatrixChangeID ||
		matrix.HighRiskCount != len(matrix.Rows) ||
		matrix.VerifiedCount != len(matrix.Rows) ||
		matrix.ExplicitUnfinishedCount != 0 ||
		matrix.DefaultGoMCPAvailable != false ||
		matrix.DefaultGoMCPLocalContract != false ||
		matrix.DefaultGoSubagentInternal != true ||
		matrix.FullBaselineClaimAllowed != false {
		t.Fatalf("baseline regression matrix identity/status mismatch: %#v", matrix)
	}
	rows := map[string]upstreamaudit.ProductRegressionRow{}
	for _, row := range matrix.Rows {
		if row.ID == "" || row.Surface == "" || row.RuntimeContract == "" || len(row.KunAnalytixSource) == 0 {
			t.Fatalf("baseline row is incomplete: %#v", row)
		}
		if row.HighRisk && len(row.Verifications) == 0 && row.UnfinishedScope == "" {
			t.Fatalf("high-risk baseline row lacks verification or explicit unfinished scope: %#v", row)
		}
		for _, verification := range row.Verifications {
			if verification.Kind == "" || verification.Status == "" || (verification.Path == "" && verification.Command == "") {
				t.Fatalf("verification must name a path or command: row=%s verification=%#v", row.ID, verification)
			}
		}
		rows[row.ID] = row
	}
	for _, id := range matrix.RequiredSurfaceIDs {
		if _, ok := rows[id]; !ok {
			t.Fatalf("required baseline surface missing: %s", id)
		}
	}
	mcp := rows["mcp-production-transport-contract"]
	if mcp.Status != "verified-production-contract" ||
		mcp.UnfinishedScope != "" ||
		mcp.ShortTermShim != "" ||
		mcp.ShimDeleteCondition != "" ||
		!verificationPathContains(mcp.Verifications, "packages/runtime-go/internal/mcp/manager_test.go") ||
		!verificationPathContains(mcp.Verifications, "packages/runtime-go/internal/server/capabilities_prod_test.go") ||
		!verificationPathContains(mcp.Verifications, "packages/runtime-go/mcp_lifecycle_contract_test.go") ||
		!verificationPathContains(mcp.Verifications, "packages/runtime-go/runtime_server_test.go") {
		t.Fatalf("MCP must be backed by production transport and runtime capability contracts: %#v", mcp)
	}
	usage := rows["usage-history-cost-cache"]
	if usage.Status != "verified-indexed" ||
		usage.UnfinishedScope != "" ||
		usage.ShortTermShim != "" ||
		usage.ShimDeleteCondition != "" ||
		!verificationPathContains(usage.Verifications, "packages/runtime-go/runtime_server_test.go") ||
		!verificationPathContains(usage.Verifications, "packages/runtime-go/internal/server/usage_test.go") ||
		!verificationPathContains(usage.Verifications, "packages/runtime/tests/usage-service.test.ts") {
		t.Fatalf("usage/history baseline must be functionally verified and indexed: %#v", usage)
	}
	subagent := rows["internal-subagent-job-lineage"]
	if subagent.Status != "verified-internal-durable-profile-parallel" ||
		subagent.UnfinishedScope != "" ||
		subagent.ShortTermShim == "" ||
		subagent.ShimDeleteCondition == "" ||
		!verificationPathContains(subagent.Verifications, "packages/runtime-go/runtime_server_test.go") ||
		!verificationPathContains(subagent.Verifications, "packages/runtime-go/internal/jobs/lineage_test.go") {
		t.Fatalf("subagent lineage must be internal-only durable/profile/parallel verified: %#v", subagent)
	}
	if !sameStringSet(matrix.ForbiddenEntrypointChecks, []string{
		"/v1/workflow",
		"/v1/workflows",
		"/v1/create-loop",
		"/v1/subagents",
		"/v1/autoresearch",
		"/v1/mcp-indexer",
	}) {
		t.Fatalf("forbidden route matrix mismatch: %#v", matrix.ForbiddenEntrypointChecks)
	}
}

func verificationPathContains(verifications []upstreamaudit.ProductRegressionVerification, path string) bool {
	for _, verification := range verifications {
		if verification.Path == path {
			return true
		}
	}
	return false
}
