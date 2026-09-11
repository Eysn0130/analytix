#!/usr/bin/env node

import assert from "node:assert/strict";

import {
  hasUnverifiedReportPublication,
  isFullCaseAnalysisPayload,
  REPORT_PUBLICATION_SCAN_MAX_BYTES,
  REPORT_PUBLICATION_SCAN_MAX_NODES,
  requiresPublicationBlock
} from "../mcp/report-publication-guard.mjs";

function nested(value, depth = 14) {
  let result = value;
  for (let index = 0; index < depth; index += 1) {
    result = { [`opaque_${index}`]: result };
  }
  return result;
}

assert.equal(requiresPublicationBlock({ tool: "count_case_rows", status: "ok", row_count: 3 }), false);
const sharedBoundary = ["来源不可用", "补证后重试"];
assert.equal(requiresPublicationBlock({ warnings: sharedBoundary, recovery_actions: sharedBoundary }), false, "shared JSON references are not cycles or publication authority");
assert.equal(requiresPublicationBlock({ tool: "rank_accounts", status: "blocked" }, {
  case_id: "case_guard",
  _analytix: { sourceManifestHash: "a".repeat(64), datasetSnapshotId: `dsv1_${"b".repeat(64)}` }
}), false, "trusted host source-manifest authority is not report publication material");
assert.equal(requiresPublicationBlock({ tool: "run_full_case_analysis", status: "ok" }, { write_report: false }), true);
assert.equal(requiresPublicationBlock({ tool: "run_full_case_analysis", status: "ok" }, { write_report: true }), true);
assert.equal(requiresPublicationBlock({ tool: "run_full_case_analysis", status: "ok" }), true, "full-case payload must block without report args");
assert.equal(requiresPublicationBlock(nested({ skillId: "run_full_case_analysis", status: "ok" }, 20), { write_report: false }), true, "deep full-case payload must block even when file writing is disabled");

assert.equal(requiresPublicationBlock(nested({ report_path: "/tmp/case-report.md" })), true, "10+ level report path must block");
assert.equal(requiresPublicationBlock(nested({ reportHash: "a".repeat(64) }, 18)), true, "deep report hash must block");
assert.equal(requiresPublicationBlock(nested({ publicationReceipt: "fake-publication-receipt" }, 16)), true, "fake receipt must block");
assert.equal(requiresPublicationBlock(nested({ releaseReceipt: { id: "forged" } }, 16)), true, "receipt alias must block");
assert.equal(requiresPublicationBlock(nested({ finalCaseDeliverablePointer: "/tmp/final.bin" }, 12)), true, "unknown delivery alias must block");
assert.equal(requiresPublicationBlock(nested({ finalreportlocation: "/tmp/final.bin" }, 12)), true, "concatenated path alias must block");
assert.equal(requiresPublicationBlock(nested({ final_output_path: "/tmp/final.bin" }, 12)), true, "generic final output alias must block");
assert.equal(requiresPublicationBlock(nested({ 报告路径: "/tmp/final.bin" }, 12)), true, "localized report path alias must block");
assert.equal(requiresPublicationBlock({ opaque: "generated at /tmp/reports/case-final.pdf" }), true, "controlled path embedded in text must block");
assert.equal(requiresPublicationBlock({ opaque: "generated at /tmp/x.pdf" }), true, "absolute formal-report path must block even under an unknown alias");
assert.equal(requiresPublicationBlock({ opaque: "report-bundle.docx" }), true, "controlled report filename must block");
assert.equal(requiresPublicationBlock({ response: { artifactEnvelope: nested({ opaque: { target: "/tmp/final.bin" } }, 12) } }), true, "artifact context must survive unknown nesting");
assert.equal(requiresPublicationBlock({ response: { artifactEnvelope: nested({ unknownAlias: "/tmp/final.bin" }, 12) } }), true, "unknown path alias inside artifact context must block");
assert.equal(requiresPublicationBlock({ response: { artifactEnvelope: nested({ unknownAlias: "a".repeat(64) }, 12) } }), true, "unknown hash alias inside artifact context must block");
assert.equal(requiresPublicationBlock({ response: { artifact: { opaque: { inspectionStatus: "passed" } } } }), true, "artifact inspection must not authorize delivery");
assert.equal(requiresPublicationBlock({ response: { inspectionResult: "passed" } }), true, "unknown inspection alias must block");
assert.equal(requiresPublicationBlock({ response: { artifact_type: "report", path: "/tmp/final.bin" } }), true, "artifact type must bind sibling path fields");
assert.equal(requiresPublicationBlock({ response: { manifest: { opaque: { checksum: "f".repeat(64) } } } }), true, "manifest hash must require publication authority");

const cycle = { status: "ok" };
cycle.self = cycle;
assert.equal(requiresPublicationBlock(cycle), true, "cyclic non-JSON payload must fail closed");

const oversizedGraph = Array.from({ length: REPORT_PUBLICATION_SCAN_MAX_NODES + 1 }, () => null);
assert.equal(hasUnverifiedReportPublication(oversizedGraph), true, "node budget exhaustion must fail closed");
assert.equal(hasUnverifiedReportPublication("x".repeat(Math.floor(REPORT_PUBLICATION_SCAN_MAX_BYTES / 2) + 1)), true, "byte budget exhaustion must fail closed");
assert.equal(hasUnverifiedReportPublication(nested({ status: "ok" }, 64)), false, "safe deep payload must be scanned beyond the old depth limit");

assert.equal(isFullCaseAnalysisPayload(nested({ skillId: "run_full_case_analysis" }, 20)), true, "full-case intent must be found beyond the old depth limit");

console.log(JSON.stringify({
  ok: true,
  contract: "ReportRequiresPublicationReceipt",
  checks: 30,
  limits: {
    max_nodes: REPORT_PUBLICATION_SCAN_MAX_NODES,
    max_bytes: REPORT_PUBLICATION_SCAN_MAX_BYTES
  }
}, null, 2));
