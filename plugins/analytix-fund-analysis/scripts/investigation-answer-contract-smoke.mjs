#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import {
  INVESTIGATION_ANSWER_CONTRACT_VERSION,
  INVESTIGATION_ANSWER_REQUIRED_FIELDS,
  validateInvestigationAnswerContract
} from "../mcp/investigation-answer-contract.mjs";
import { withClaimReviewProtocol } from "../mcp/claim-verifier-protocol.mjs";

const __filename = fileURLToPath(import.meta.url);
const SCRIPT_DIR = path.dirname(__filename);
const PLUGIN_ROOT = path.resolve(SCRIPT_DIR, "..");

function text(value) {
  return String(value == null ? "" : value).trim();
}

function arrayOf(value) {
  return Array.isArray(value) ? value : [];
}

function objectOf(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function errorCodes(result) {
  return new Set(arrayOf(result.errors).map((error) => text(error.code)).filter(Boolean));
}

function sampleValidAnswer() {
  return {
    investigation_objective: {
      purpose: "核验甲账户向乙账户转账关系是否具有短期集中入账后分散转出的异常特征",
      scope: "甲账户 2024-01-01 至 2024-03-31 对乙账户及后续一跳出账",
      time_range: "2024-01-01 至 2024-03-31",
      source_boundary: "基于当前案件已清洗交易明细、资金流向图谱和专项核算 evidence_card"
    },
    conclusion: {
      summary: "现有流水支持甲账户向乙账户形成集中转入关系，乙账户后续存在分散转出特征；该情况可作为资金去向异常线索，资金性质和最终归属仍需补证。",
      evidence_maturity: "可疑特征",
      cannot_conclude: ["不能仅凭银行流水认定资金性质或最终归属"]
    },
    verified_facts: [
      {
        fact_id: "fact.direct-transfer",
        fact: "甲账户在统计期间向乙账户转入 12 笔，合计 123456.78 元。",
        fact_type: "direct_transfer",
        amount_yuan: 123456.78,
        txn_count: 12,
        period: "2024-01-01 至 2024-03-31",
        parties: ["甲账户", "乙账户"],
        support_status: "supported",
        evidence_refs: {
          query_ids: ["run_case_sql.direct-transfer"],
          evidence_ids: ["analysis_txn_detail_idx:txn-001"],
          source_hash: "sample-source-hash"
        },
        evidence_receipt_ids: ["receipt.fact.direct-transfer"]
      }
    ],
    fund_flow_paths: [
      {
        path_summary: "甲账户 -> 乙账户",
        path_nodes: ["甲账户", "乙账户"],
        edges: [
          {
            edge_id: "edge.supported.1",
            from: "甲账户",
            to: "乙账户",
            amount_yuan: 123456.78,
            txn_count: 12,
            support_status: "supported",
            evidence_refs: {
              evidence_ids: ["analysis_relation_edge:edge.supported.1"]
            },
            evidence_receipt_ids: ["receipt.edge.supported.1"]
          }
        ]
      }
    ],
    risk_patterns: [
      {
        pattern: "短期集中转入后存在分散转出特征",
        basis: "直接转账事实与后续一跳出账事实相互印证，但下游最终用途尚需补证。",
        related_fact_ids: ["fact.direct-transfer"]
      }
    ],
    evidence_boundaries: [
      {
        boundary: "现有数据主要证明账户间交易关系，尚不能直接证明资金性质、账户控制关系或最终归属。",
        reason: "缺少开户资料、交易用途凭证和下游账户完整流水。",
        next_proof_action: "调取开户资料、交易回单和乙账户后续流水。"
      }
    ],
    next_proof_actions: [
      {
        action: "调取乙账户后续出账明细及交易对手身份信息",
        target: "乙账户及其主要下游账户",
        material: "后续流水、开户资料、交易回单",
        time_scope: "2024-01-01 至 2024-03-31 后续 30 日",
        field_scope: "交易时间、金额、付款/收款账号户名、摘要、交易渠道、余额",
        linked_gap: "乙账户后续分散转出后的最终用途和承接关系尚未闭合",
        priority: "P1",
        proof_value: "用于核实资金承接关系、实际用途和最终去向。",
        evidence_boundary: "未取得该材料前，不能认定资金性质、账户控制关系或最终归属。",
        forbidden_upgrade: "不得写成已查明违法所得、实际控制或最终归属。"
      }
    ],
    visible_final_answer: "经核验现有交易明细，甲账户向乙账户形成集中转入关系，并存在后续分散转出特征。上述情况可作为资金去向异常线索；资金性质、账户控制关系和最终归属仍需进一步调取开户资料、交易回单及下游账户流水予以印证。"
  };
}

function verifySampleEvidenceReceipt({ receiptId, claimType, claim }) {
  if (claimType === "verified_fact") {
    return receiptId === "receipt.fact.direct-transfer"
      && Number(claim.amount_yuan) === 123456.78
      && Number(claim.txn_count) === 12
      && arrayOf(claim.parties).join("|") === "甲账户|乙账户";
  }
  if (claimType === "fund_flow_edge") {
    return receiptId === "receipt.edge.supported.1"
      && Number(claim.amount_yuan) === 123456.78
      && Number(claim.txn_count) === 12
      && text(claim.from) === "甲账户"
      && text(claim.to) === "乙账户";
  }
  return false;
}

const HOST_RECEIPT_OPTIONS = { verifyEvidenceReceipt: verifySampleEvidenceReceipt };

function runSmoke() {
  const schemaPath = path.join(PLUGIN_ROOT, "references", "investigation-answer.schema.json");
  const schema = JSON.parse(fs.readFileSync(schemaPath, "utf8"));
  assert(schema.title === "FinalInvestigationAnswer", "schema title must be FinalInvestigationAnswer");
  for (const field of INVESTIGATION_ANSWER_REQUIRED_FIELDS) {
    assert(arrayOf(schema.required).includes(field), `schema missing required field: ${field}`);
  }

  const valid = validateInvestigationAnswerContract(sampleValidAnswer(), HOST_RECEIPT_OPTIONS);
  assert(valid.ok, `valid sample failed: ${arrayOf(valid.errors).map((error) => error.code).join(", ")}`);
  assert(arrayOf(valid.audit_events).some((event) => event.event === "investigation_answer_validation:passed"), "valid sample must emit passed audit event");
  assert(!arrayOf(valid.warnings).length, `valid sample should not emit proof-action warnings: ${arrayOf(valid.warnings).map((warning) => warning.code).join(", ")}`);

  const missingEvidence = sampleValidAnswer();
  missingEvidence.verified_facts = [
    {
      fact: "甲账户向乙账户转账 12 笔，合计 123456.78 元。",
      support_status: "supported"
    }
  ];
  const missingEvidenceResult = validateInvestigationAnswerContract(missingEvidence, HOST_RECEIPT_OPTIONS);
  assert(!missingEvidenceResult.ok, "missing evidence refs must fail");
  assert(errorCodes(missingEvidenceResult).has("FACT_EVIDENCE_RECEIPT_REJECTED"), "missing evidence refs must return FACT_EVIDENCE_RECEIPT_REJECTED");
  assert(arrayOf(missingEvidenceResult.repair_actions).some((item) => item.fact_policy.includes("Evidence Registry")), "missing fact repair must route back to the host Evidence Registry");
  assert(arrayOf(missingEvidenceResult.audit_events).some((event) => event.event === "investigation_answer_validation_issue" && event.code === "FACT_EVIDENCE_RECEIPT_REJECTED" && event.category === "source_evidence"), "missing evidence must emit source_evidence audit event");

  const candidateEdge = sampleValidAnswer();
  candidateEdge.fund_flow_paths[0].edges[0].support_status = "candidate";
  const candidateEdgeResult = validateInvestigationAnswerContract(candidateEdge, HOST_RECEIPT_OPTIONS);
  assert(!candidateEdgeResult.ok, "candidate fund-flow edge must fail");
  assert(errorCodes(candidateEdgeResult).has("FLOW_EDGE_NOT_SUPPORTED"), "candidate edge must return FLOW_EDGE_NOT_SUPPORTED");

  const legalUpgrade = sampleValidAnswer();
  legalUpgrade.conclusion.summary = "现有流水已经坐实乙账户实际控制并锁定违法所得。";
  const legalUpgradeResult = validateInvestigationAnswerContract(legalUpgrade, HOST_RECEIPT_OPTIONS);
  assert(!legalUpgradeResult.ok, "unguarded legal upgrade must fail");
  assert(errorCodes(legalUpgradeResult).has("LEGAL_CONCLUSION_UNSUPPORTED"), "legal upgrade must return LEGAL_CONCLUSION_UNSUPPORTED");

  const visibleLeak = sampleValidAnswer();
  visibleLeak.visible_final_answer = "MCP 返回 query_id 后可直接输出。";
  const visibleLeakResult = validateInvestigationAnswerContract(visibleLeak, HOST_RECEIPT_OPTIONS);
  assert(!visibleLeakResult.ok, "visible engineering leak must fail");
  assert(errorCodes(visibleLeakResult).has("VISIBLE_ENGINEERING_LEAK"), "visible leak must return VISIBLE_ENGINEERING_LEAK");

  const fakeCitation = sampleValidAnswer();
  fakeCitation.verified_facts[0].evidence_receipt_ids = ["receipt.fake"];
  const fakeCitationResult = validateInvestigationAnswerContract(fakeCitation, HOST_RECEIPT_OPTIONS);
  assert(!fakeCitationResult.ok, "fake EvidenceReceipt must fail");
  assert(errorCodes(fakeCitationResult).has("FACT_EVIDENCE_RECEIPT_REJECTED"), "fake EvidenceReceipt must be rejected by the host verifier");

  const mismatchedCitation = sampleValidAnswer();
  mismatchedCitation.verified_facts[0].amount_yuan = 999999.99;
  const mismatchedCitationResult = validateInvestigationAnswerContract(mismatchedCitation, HOST_RECEIPT_OPTIONS);
  assert(!mismatchedCitationResult.ok, "mismatched EvidenceReceipt must fail");
  assert(errorCodes(mismatchedCitationResult).has("FACT_EVIDENCE_RECEIPT_REJECTED"), "mismatched EvidenceReceipt must be rejected by exact-field verification");

  const claimReviewCard = withClaimReviewProtocol({
    report_text: "报告拟写：甲账户向乙账户转账 12 笔，合计 123456.78 元。",
    facts: [],
    final_investigation_answer: missingEvidence
  });
  const claimReviewText = JSON.stringify({
    unsupported_claims: claimReviewCard.unsupported_claims,
    missing_source_boundaries: claimReviewCard.missing_source_boundaries,
    next_review_actions: claimReviewCard.next_review_actions,
    risk_markers: claimReviewCard.investigation_answer_contract_review
  });
  assert(claimReviewCard.write_blocked === true, "claim review must block invalid FinalInvestigationAnswer");
  assert(claimReviewText.includes("FACT_EVIDENCE_RECEIPT_REJECTED"), "claim review must surface FinalInvestigationAnswer missing evidence code");
  assert(/Evidence Registry|receipt|MCP\/DuckDB|evidence_refs|source_refs/iu.test(claimReviewText), "claim review repair must route missing facts back to host-verified evidence");
  assert(claimReviewText.includes("investigation_answer_contract_failed"), "claim review must expose internal contract risk marker");
  assert(arrayOf(objectOf(claimReviewCard._meta).audit_events).some((event) => objectOf(event).code === "FACT_EVIDENCE_RECEIPT_REJECTED"), "claim review must carry validation audit events in _meta");

  return {
    status: "ok",
    contract_version: INVESTIGATION_ANSWER_CONTRACT_VERSION,
    checked: [
      "schema required fields",
      "valid final investigation answer",
      "missing or rejected EvidenceReceipt fails as a host-registry fact gap",
      "fake and field-mismatched EvidenceReceipts are rejected",
      "candidate flow edge cannot be a supported path",
      "legal-sensitive upgrade is downgraded or blocked",
      "visible engineering leakage is blocked",
      "validation failures produce internal audit events",
      "claim review consumes FinalInvestigationAnswer validation failures"
    ]
  };
}

try {
  const result = runSmoke();
  console.log(JSON.stringify(result, null, 2));
} catch (error) {
  console.error(error?.stack || error?.message || String(error));
  process.exit(1);
}
