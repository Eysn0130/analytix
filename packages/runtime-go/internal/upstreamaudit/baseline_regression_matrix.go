//go:build !analytix_prod

package upstreamaudit

const ProductRegressionMatrixChangeID = "product-regression-matrix"

type ProductRegressionVerification struct {
	Kind    string `json:"kind"`
	Path    string `json:"path,omitempty"`
	Command string `json:"command,omitempty"`
	Status  string `json:"status"`
}

type ProductRegressionRow struct {
	ID                  string                          `json:"id"`
	Surface             string                          `json:"surface"`
	HighRisk            bool                            `json:"highRisk"`
	KunAnalytixSource   []string                        `json:"kunAnalytixSource"`
	RuntimeContract     string                          `json:"runtimeContract"`
	Verifications       []ProductRegressionVerification `json:"verifications"`
	Status              string                          `json:"status"`
	UnfinishedScope     string                          `json:"unfinishedScope,omitempty"`
	ShortTermShim       string                          `json:"shortTermShim,omitempty"`
	ShimDeleteCondition string                          `json:"shimDeleteCondition,omitempty"`
}

type ProductRegressionMatrix struct {
	SchemaVersion             int                    `json:"schemaVersion"`
	ChangeID                  string                 `json:"changeId"`
	Rows                      []ProductRegressionRow `json:"rows"`
	RequiredSurfaceIDs        []string               `json:"requiredSurfaceIds"`
	HighRiskCount             int                    `json:"highRiskCount"`
	VerifiedCount             int                    `json:"verifiedCount"`
	ExplicitUnfinishedCount   int                    `json:"explicitUnfinishedCount"`
	ForbiddenEntrypointChecks []string               `json:"forbiddenEntrypointChecks"`
	DefaultGoMCPAvailable     bool                   `json:"defaultGoMcpAvailable"`
	DefaultGoMCPLocalContract bool                   `json:"defaultGoMcpLocalContract"`
	DefaultGoSubagentInternal bool                   `json:"defaultGoSubagentInternal"`
	FullBaselineClaimAllowed  bool                   `json:"fullBaselineClaimAllowed"`
}

func BuildProductRegressionMatrix() ProductRegressionMatrix {
	rows := []ProductRegressionRow{
		baselineRow("settings-runtime-provider", "settings schema, runtime provider settings, and migration boundaries", true,
			[]string{"src/shared/app-settings-runtime.ts", "src/shared/app-settings-provider.ts", "src/main/settings-store.ts"},
			"top-level runtime settings only; no new agents.kun/agents.analytix writes",
			[]ProductRegressionVerification{
				verified("test", "src/shared/app-settings.test.ts", ""),
				verified("test", "src/shared/app-settings-provider.test.ts", ""),
				verified("test", "src/main/settings-store.test.ts", ""),
			}, "verified", "", "", ""),
		baselineRow("preload-bridge", "preload bridge and renderer global API", true,
			[]string{"src/preload/index.ts", "src/shared/analytix-api.ts"},
			"window.analytix only; no deprecated bridge alias",
			[]ProductRegressionVerification{
				verified("test", "src/preload/preload-sandbox.test.ts", ""),
				verified("test", "src/preload/preload-runtime-request.test.ts", ""),
				verified("scan", "scripts/scan-product-sovereignty.cjs", "npm run scan:product-sovereignty"),
			}, "verified", "", "", ""),
		baselineRow("renderer-agent-api", "renderer agent runtime API and owned route surface", true,
			[]string{"src/renderer/src/agent/analytix-runtime.ts", "src/renderer/src/agent/analytix-mapper.ts"},
			"renderer uses Analytix /v1 thread, session, approval, user-input, attachment, memory, and runtime routes",
			[]ProductRegressionVerification{
				verified("test", "src/renderer/src/agent/analytix-runtime.test.ts", ""),
				verified("test", "src/renderer/src/agent/analytix-mapper.test.ts", ""),
			}, "verified", "", "", ""),
		baselineRow("thread-list-create-resume-fork-archive-search", "thread list/create/resume/fork/archive/search", true,
			[]string{"packages/runtime/src/server/routes", "packages/runtime-go/internal/server/runtime_components.go"},
			"/v1/threads and /v1/sessions/:id/resume-thread keep the Analytix contract",
			[]ProductRegressionVerification{
				verified("test", "packages/runtime-go/runtime_server_test.go", ""),
				verified("test", "packages/runtime/tests/go-runtime-conformance.test.ts", ""),
				verified("test", "src/renderer/src/agent/analytix-runtime.test.ts", ""),
			}, "verified", "", "", ""),
		baselineRow("sse-replay", "thread SSE replay, durable highestSeq, and reconnect", true,
			[]string{"packages/runtime/src/services/runtime-event-recorder.ts", "packages/runtime-go/internal/server/durable_store.go"},
			"/v1/threads/:id/events replays by since_seq and Last-Event-ID",
			[]ProductRegressionVerification{
				verified("test", "packages/runtime-go/runtime_server_test.go", ""),
				verified("test", "src/main/runtime-sse-ipc.test.ts", ""),
				verified("test", "packages/runtime/tests/runtime-event-reducer.test.ts", ""),
			}, "verified", "", "", ""),
		baselineRow("usage-history-cost-cache", "usage history, cost/cache accounting, and thread detail usage", true,
			[]string{"packages/runtime/src/server/routes/usage.ts", "packages/runtime/src/services/usage-service.ts", "packages/runtime-go/internal/server/usage.go"},
			"/v1/usage supports runtime/thread/day/model grouping and thread detail includes cumulative usage; usage_events index/backfill avoids per-request thread event replay",
			[]ProductRegressionVerification{
				verified("test", "packages/runtime-go/runtime_server_test.go", ""),
				verified("test", "packages/runtime-go/internal/server/usage_test.go", ""),
				verified("test", "packages/runtime/tests/go-runtime-conformance.test.ts", ""),
				verified("test", "packages/runtime/tests/usage-service.test.ts", ""),
				verified("test", "packages/runtime/tests/file-session-store.test.ts", ""),
				verified("test", "packages/runtime/tests/runtime-event-recorder.test.ts", ""),
				verified("test", "src/renderer/src/agent/analytix-runtime.test.ts", ""),
			}, "verified-indexed", "", "", ""),
		baselineRow("approval", "approval routes and no-execute policy", true,
			[]string{"packages/runtime/src/adapters/tool/local-tool-host.ts", "packages/runtime-go/internal/agent/approval_user_input_contract.go"},
			"/v1/approvals/:id resolves pending gates and denied tools do not execute",
			[]ProductRegressionVerification{
				verified("test", "packages/runtime-go/runtime_server_test.go", ""),
				verified("test", "packages/runtime/tests/approval-user-input-route-contract.test.ts", ""),
			}, "verified", "", "", ""),
		baselineRow("user-input", "user-input request/submit/cancel privacy", true,
			[]string{"packages/runtime/src/contracts/user-input.ts", "packages/runtime-go/internal/agent/gates.go"},
			"/v1/user-inputs/:id returns submitted answers in HTTP response while SSE replay omits answers",
			[]ProductRegressionVerification{
				verified("test", "packages/runtime-go/runtime_server_test.go", ""),
				verified("test", "packages/runtime/tests/go-runtime-conformance.test.ts", ""),
			}, "verified", "", "", ""),
		baselineRow("attachments-multimodal-local-files", "attachments, multimodal image payloads, and local file references", true,
			[]string{"src/preload/index.ts", "src/renderer/src/agent/analytix-runtime.ts", "packages/runtime-go/internal/adapters/outbound/filestore/attachments.go"},
			"attachment IDs, localFilePath, content, and fileReferences remain contract fields",
			[]ProductRegressionVerification{
				verified("test", "packages/runtime-go/runtime_server_test.go", ""),
				verified("test", "src/renderer/src/agent/analytix-runtime.test.ts", ""),
				verified("test", "src/preload/preload-sandbox.test.ts", ""),
			}, "verified", "", "", ""),
		baselineRow("provider-multimodel-request-shape", "multi-provider URL/body/header/stream/usage request shape", true,
			[]string{"src/shared/openai-compat-url.ts", "packages/runtime-go/internal/provider/provider.go"},
			"DeepSeek cache/prefix fields stay provider-scoped; OpenAI chat, responses, Anthropic messages, and custom endpoint keep their own shape",
			[]ProductRegressionVerification{
				verified("test", "packages/runtime-go/runtime_server_test.go", ""),
				verified("test", "packages/runtime-go/kun_analytix_baseline_absorption_test.go", ""),
				verified("test", "packages/runtime/tests/provider-cache-contract.test.ts", ""),
			}, "verified", "", "", ""),
		baselineRow("mcp-production-transport-contract", "Go runtime MCP production manager transport, catalog refresh, reconnect, redaction, and SSE replay", true,
			[]string{"packages/runtime/src/adapters/tool/mcp-tool-provider.ts", "packages/runtime/src/adapters/tool/mcp-tool-search.ts", "packages/runtime-go/internal/mcp"},
			"unconfigured MCP remains unavailable/empty in Go default; configured MCP uses production stdio/http transports, canonical schema diagnostics, reconnect, redaction, and product-only runtime tools",
			[]ProductRegressionVerification{
				verified("test", "packages/runtime-go/internal/mcp/manager_test.go", ""),
				verified("test", "packages/runtime-go/internal/server/capabilities_prod_test.go", ""),
				verified("test", "packages/runtime-go/mcp_lifecycle_contract_test.go", ""),
				verified("test", "packages/runtime-go/runtime_server_test.go", ""),
				verified("test", "packages/runtime/tests/mcp-tool-lifecycle-contract.test.ts", ""),
			}, "verified-production-contract", "", "", ""),
		baselineRow("goal", "/goal create/read/update/clear and evidence audit", true,
			[]string{"packages/runtime/src/loop/agent-loop.ts", "packages/runtime-go/internal/goal/goal_evidence.go"},
			"/v1/threads/:id/goal remains the only goal route; goal_evidence_audit is replay evidence",
			[]ProductRegressionVerification{
				verified("test", "packages/runtime-go/runtime_server_test.go", ""),
				verified("test", "packages/runtime-go/goal_evidence_contract_test.go", ""),
				verified("test", "packages/runtime/tests/goal-tools.test.ts", ""),
			}, "verified", "", "", ""),
		baselineRow("internal-subagent-job-lineage", "internal job/sub-agent lineage under existing goal/thread/tool contract", true,
			[]string{"packages/runtime/src/delegation/delegation-runtime.ts", "packages/runtime-go/internal/jobs/lineage.go"},
			"internal child-run lineage is emitted as pipeline_stage under the parent goal/thread; /v1/subagents remains hidden",
			[]ProductRegressionVerification{
				verified("test", "packages/runtime-go/runtime_server_test.go", ""),
				verified("test", "packages/runtime-go/internal/jobs/lineage_test.go", ""),
				verified("test", "packages/runtime/tests/go-runtime-conformance.test.ts", ""),
			}, "verified-internal-durable-profile-parallel",
			"",
			"durable child-run records, default parent model/profile inheritance, artifacts, and parallel child execution stay scoped to existing /goal/thread/tool contracts",
			"delete the internal shim only if a future Analytix product decision exposes configurable subagent profiles through existing contracts without adding an external public protocol"),
		baselineRow("runtime-info-tools", "runtime info/tools diagnostics", true,
			[]string{"packages/runtime-go/internal/server/runtime_components.go", "src/renderer/src/components/RuntimeBanner.tsx"},
			"/health, /v1/runtime/info, and /v1/runtime/tools stay honest and renderer-safe",
			[]ProductRegressionVerification{
				verified("test", "packages/runtime-go/runtime_server_test.go", ""),
				verified("test", "packages/runtime/tests/go-runtime-conformance.test.ts", ""),
				verified("test", "src/renderer/src/agent/analytix-runtime.test.ts", ""),
			}, "verified", "", "", ""),
		baselineRow("packaged-dev-startup", "dev and packaged startup, health, runtime info, and exit port release", true,
			[]string{"src/main/runtime/analytix-adapter.ts", "packages/runtime-go/cmd/runtime-server/main.go"},
			"Go default startup reports health/runtime info and releases port 8901 on exit",
			[]ProductRegressionVerification{
				verified("test", "src/main/packaging-config.test.ts", ""),
				verified("script", "scripts/runtime-go-packaged-gui-smoke.mjs", ""),
				verified("test", "src/main/runtime/analytix-adapter.test.ts", ""),
				verified("smoke", "", "npm run dev smoke: health, runtime info, thread/turn/SSE/delete, exit port 8901"),
			}, "verified", "", "", ""),
		baselineRow("forbidden-reasonix-entries", "forbidden Reasonix top-level entries", true,
			[]string{"docs/analytix/specs/08-upstream-absorption-and-go-runtime.md", "packages/runtime-go/internal/server/runtime_components.go"},
			"/v1/workflow, /v1/workflows, /v1/create-loop, /v1/subagents, /v1/autoresearch, and /v1/mcp-indexer remain 404",
			[]ProductRegressionVerification{
				verified("test", "packages/runtime-go/runtime_server_test.go", ""),
				verified("test", "src/renderer/src/agent/analytix-runtime.test.ts", ""),
				verified("scan", "scripts/scan-product-sovereignty.cjs", "npm run scan:product-sovereignty"),
			}, "verified", "", "", ""),
	}
	matrix := ProductRegressionMatrix{
		SchemaVersion: 1,
		ChangeID:      ProductRegressionMatrixChangeID,
		Rows:          rows,
		RequiredSurfaceIDs: []string{
			"settings-runtime-provider",
			"preload-bridge",
			"renderer-agent-api",
			"thread-list-create-resume-fork-archive-search",
			"sse-replay",
			"usage-history-cost-cache",
			"approval",
			"user-input",
			"attachments-multimodal-local-files",
			"provider-multimodel-request-shape",
			"mcp-production-transport-contract",
			"goal",
			"internal-subagent-job-lineage",
			"runtime-info-tools",
			"packaged-dev-startup",
			"forbidden-reasonix-entries",
		},
		ForbiddenEntrypointChecks: []string{
			"/v1/workflow",
			"/v1/workflows",
			"/v1/create-loop",
			"/v1/subagents",
			"/v1/autoresearch",
			"/v1/mcp-indexer",
		},
		DefaultGoMCPAvailable:     false,
		DefaultGoMCPLocalContract: false,
		DefaultGoSubagentInternal: true,
		FullBaselineClaimAllowed:  false,
	}
	for _, row := range rows {
		if row.HighRisk {
			matrix.HighRiskCount++
		}
		if row.Status == "verified" ||
			row.Status == "verified-internal" ||
			row.Status == "verified-indexed" ||
			row.Status == "verified-production-contract" ||
			row.Status == "verified-internal-durable-profile-parallel" {
			matrix.VerifiedCount++
		}
		if row.UnfinishedScope != "" {
			matrix.ExplicitUnfinishedCount++
		}
	}
	return matrix
}

func baselineRow(
	id string,
	surface string,
	highRisk bool,
	sources []string,
	contract string,
	verifications []ProductRegressionVerification,
	status string,
	unfinished string,
	shim string,
	deleteCondition string,
) ProductRegressionRow {
	return ProductRegressionRow{
		ID:                  id,
		Surface:             surface,
		HighRisk:            highRisk,
		KunAnalytixSource:   sources,
		RuntimeContract:     contract,
		Verifications:       verifications,
		Status:              status,
		UnfinishedScope:     unfinished,
		ShortTermShim:       shim,
		ShimDeleteCondition: deleteCondition,
	}
}

func verified(kind string, path string, command string) ProductRegressionVerification {
	return ProductRegressionVerification{
		Kind:    kind,
		Path:    path,
		Command: command,
		Status:  "passed-or-required",
	}
}
