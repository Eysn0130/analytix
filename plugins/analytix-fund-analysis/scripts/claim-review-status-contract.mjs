#!/usr/bin/env node

import assert from "node:assert/strict";

import { classifyClaimReviewRows } from "../mcp/claim-review-diagnostic-runtime.mjs";

const fixtures = [
  { row: { status: "supported" }, expected: "supported" },
  { row: { status: "VERIFIED" }, expected: "supported" },
  { row: { support_status: "matched" }, expected: "supported" },
  { row: { status: "corrected" }, expected: "corrected" },
  { row: { status: "mismatch" }, expected: "corrected" },
  { row: { support_status: "adjusted" }, expected: "corrected" },
  { row: { support_status: "revised" }, expected: "corrected" },
  { row: { status: "需纠正" }, expected: "corrected" },
  { row: { status: "unsupported" }, expected: "unsupported" },
  { row: { status: "not_supported" }, expected: "unsupported" },
  { row: { status: "not-supported" }, expected: "unsupported" },
  { row: { status: "unmatched" }, expected: "unsupported" },
  { row: { status: "missing" }, expected: "unsupported" },
  { row: { status: "insufficient" }, expected: "unsupported" },
  { row: { status: "unverified" }, expected: "unsupported" },
  { row: { status: "partial" }, expected: "unsupported" },
  { row: { status: "unsupported_ok" }, expected: "unsupported" },
  { row: { status: "not_supported_pass" }, expected: "unsupported" },
  { row: { status: "unmatched_verified" }, expected: "unsupported" },
  { row: { status: "supportive" }, expected: "unsupported" },
  { row: { status: "support" }, expected: "unsupported" },
  { row: { status: "ok" }, expected: "unsupported" },
  { row: { status: "pass" }, expected: "unsupported" },
  { row: { status: "passed" }, expected: "unsupported" },
  { row: { status: "status_ok" }, expected: "unsupported" },
  { row: { status: "verified-ish" }, expected: "unsupported" },
  { row: { status: "matched_ok" }, expected: "unsupported" },
  { row: { status: "totally_new_status" }, expected: "unsupported" },
  { row: { status: "__proto__" }, expected: "unsupported" },
  { row: { status: "unknown", support_status: "supported" }, expected: "unsupported" },
  { row: { status: "unknown", finding: "corrected" }, expected: "unsupported" },
  { row: {}, expected: "unsupported" }
];

for (const [index, fixture] of fixtures.entries()) {
  const marker = `claim_status_${String(index + 1).padStart(2, "0")}`;
  const classified = classifyClaimReviewRows([{ ...fixture.row, text: marker }]);
  const actual = Object.fromEntries(
    Object.entries(classified).map(([category, rows]) => [
      category,
      rows.some((row) => row.text === marker)
    ])
  );
  assert.equal(actual[fixture.expected], true, `${marker} must be ${fixture.expected}`);
  assert.equal(
    Object.values(actual).filter(Boolean).length,
    1,
    `${marker} must belong to exactly one claim-review category: ${JSON.stringify(actual)}`
  );
}

console.log(`claim-review status contract passed (${fixtures.length} closed-enum cases)`);
