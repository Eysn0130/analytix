#!/usr/bin/env node

import assert from "node:assert/strict";
import { createToolCallRuntime } from "../mcp/tool-call-runtime.mjs";

const baseArgs = {
  purpose: "Verify an explicit bounded aggregate",
  sql: "SELECT count(*) AS txn_count FROM analysis_txn_detail_idx",
  row_limit: 10
};

async function runCaseSql(payload) {
  const runtime = createToolCallRuntime({
    executeSkill: async () => payload,
    env: { ANALYTIX_FUNDS_DISABLE_LOCAL_DUCKDB_FALLBACK: "1" }
  });
  return runtime.callTool("run_case_sql", baseArgs);
}

const missing = await runCaseSql({ status: "ok", data: { records: [] } });
assert.equal(missing.status, "ok");
assert.equal(missing.evidence_card?.support_status, "unsupported");
assert.equal(missing.evidence_card?.fact_answer_allowed, false);
assert.equal(missing.evidence_card?.metric_scope?.row_count, undefined);
assert.equal(JSON.stringify(missing.evidence_card).includes('"row_count":0'), false);
for (const field of ["query_id", "row_count", "pagination_completeness", "source_sha256", "host_evidence_receipt"]) {
  assert(missing.evidence_card?.missing_fields?.includes(field), `missing ${field} must remain explicit`);
}

const explicitZero = await runCaseSql({
  status: "ok",
  data: {
    query_id: "query.explicit-zero",
    source_scope: ["analysis_txn_detail_idx"],
    source_hash: "a".repeat(64),
    row_count: 0,
    truncated: false,
    records: []
  }
});
assert.equal(explicitZero.evidence_card?.metric_scope?.row_count, 0);
assert.equal(explicitZero.evidence_card?.metric_scope?.truncated, false);
assert.equal(explicitZero.evidence_card?.support_status, "unsupported");
assert.equal(explicitZero.evidence_card?.missing_fields?.includes("host_evidence_receipt"), true);
assert.equal(explicitZero.evidence_card?.missing_fields?.includes("row_count"), false);

console.log(JSON.stringify({
  status: "ok",
  checks: [
    "missing workbench row_count never becomes zero or supported",
    "explicit zero remains distinct but cannot bypass the host EvidenceReceipt gate",
    "query scope, completeness, SHA-256, and host receipt gaps remain explicit"
  ]
}, null, 2));
