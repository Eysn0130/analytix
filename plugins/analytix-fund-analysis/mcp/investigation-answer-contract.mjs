export const INVESTIGATION_ANSWER_CONTRACT_VERSION = "0.15.128-investigation-answer-contract";

export const INVESTIGATION_ANSWER_REQUIRED_FIELDS = [
  "investigation_objective",
  "conclusion",
  "verified_facts",
  "risk_patterns",
  "evidence_boundaries",
  "next_proof_actions"
];

export const INVESTIGATION_ANSWER_SCHEMA = {
  $schema: "https://json-schema.org/draft/2020-12/schema",
  $id: "https://analytix.local/schemas/investigation-answer.schema.json",
  title: "FinalInvestigationAnswer",
  type: "object",
  additionalProperties: false,
  required: INVESTIGATION_ANSWER_REQUIRED_FIELDS,
  properties: {
    investigation_objective: {
      type: "object",
      additionalProperties: false,
      required: ["purpose", "scope", "source_boundary"],
      properties: {
        purpose: { type: "string", minLength: 1 },
        scope: { type: "string", minLength: 1 },
        time_range: { type: "string" },
        source_boundary: { type: "string", minLength: 1 }
      }
    },
    conclusion: {
      type: "object",
      additionalProperties: false,
      required: ["summary", "evidence_maturity"],
      properties: {
        summary: { type: "string", minLength: 1 },
        evidence_maturity: {
          type: "string",
          enum: ["已核验事实", "统计特征", "可疑特征", "线索", "需复核", "不可判断"]
        },
        cannot_conclude: {
          type: "array",
          items: { type: "string" }
        }
      }
    },
    verified_facts: {
      type: "array",
      minItems: 1,
      items: {
        type: "object",
        additionalProperties: false,
        required: ["fact", "evidence_receipt_ids"],
        properties: {
          fact_id: { type: "string" },
          fact: { type: "string", minLength: 1 },
          fact_type: { type: "string" },
          amount_yuan: { type: "number" },
          txn_count: { type: "integer" },
          period: { type: "string" },
          parties: { type: "array", items: { type: "string" } },
          support_status: { type: "string" },
          evidence_receipt_ids: { type: "array", items: { type: "string", minLength: 1 }, minItems: 1 },
          evidence_refs: {
            oneOf: [
              { type: "array", items: { type: "string" }, minItems: 1 },
              {
                type: "object",
                additionalProperties: false,
                minProperties: 1,
                properties: {
                  query_ids: { type: "array", items: { type: "string" } },
                  evidence_ids: { type: "array", items: { type: "string" } },
                  evidence_receipt_ids: { type: "array", items: { type: "string" } },
                  source_hash: { type: "string" }
                }
              }
            ]
          }
        }
      }
    },
    fund_flow_paths: {
      type: "array",
      items: {
        type: "object",
        additionalProperties: false,
        properties: {
          path_summary: { type: "string" },
          path_nodes: { type: "array", items: { type: "string" } },
          edges: {
            type: "array",
            items: {
              type: "object",
              additionalProperties: false,
              required: ["from", "to", "evidence_receipt_ids"],
              properties: {
                edge_id: { type: "string" },
                from: { type: "string", minLength: 1 },
                to: { type: "string", minLength: 1 },
                amount_yuan: { type: "number" },
                txn_count: { type: "integer" },
                support_status: { type: "string" },
                edge_status: { type: "string" },
                evidence_receipt_ids: { type: "array", items: { type: "string", minLength: 1 }, minItems: 1 },
                evidence_refs: { type: "object", additionalProperties: false, properties: { evidence_ids: { type: "array", items: { type: "string" } }, evidence_receipt_ids: { type: "array", items: { type: "string" } }, source_hash: { type: "string" } } }
              }
            }
          },
          evidence_receipt_ids: { type: "array", items: { type: "string", minLength: 1 } },
          evidence_refs: {
            oneOf: [
              { type: "array", items: { type: "string" } },
              { type: "object", additionalProperties: false, properties: { evidence_ids: { type: "array", items: { type: "string" } }, evidence_receipt_ids: { type: "array", items: { type: "string" } }, source_hash: { type: "string" } } }
            ]
          }
        }
      }
    },
    risk_patterns: {
      type: "array",
      minItems: 1,
      items: {
        type: "object",
        additionalProperties: false,
        required: ["pattern", "basis"],
        properties: {
          pattern: { type: "string", minLength: 1 },
          basis: { type: "string", minLength: 1 },
          related_fact_ids: { type: "array", items: { type: "string" } },
          evidence_receipt_ids: { type: "array", items: { type: "string", minLength: 1 } },
          evidence_refs: {
            oneOf: [
              { type: "array", items: { type: "string" } },
              { type: "object", additionalProperties: false, properties: { evidence_ids: { type: "array", items: { type: "string" } }, evidence_receipt_ids: { type: "array", items: { type: "string" } }, source_hash: { type: "string" } } }
            ]
          }
        }
      }
    },
    evidence_boundaries: {
      type: "array",
      minItems: 1,
      items: {
        oneOf: [
          { type: "string", minLength: 1 },
          {
            type: "object",
            additionalProperties: false,
            required: ["boundary"],
            properties: {
              boundary: { type: "string", minLength: 1 },
              reason: { type: "string" },
              next_proof_action: { type: "string" }
            }
          }
        ]
      }
    },
    next_proof_actions: {
      type: "array",
      minItems: 1,
      items: {
        type: "object",
        additionalProperties: false,
        required: ["action", "proof_value"],
        properties: {
          action: { type: "string", minLength: 1 },
          target: { type: "string" },
          material: { type: "string" },
          time_scope: { type: "string" },
          field_scope: { type: "string" },
          linked_gap: { type: "string" },
          priority: { type: "string" },
          proof_value: { type: "string", minLength: 1 },
          evidence_boundary: { type: "string" },
          forbidden_upgrade: { type: "string" },
          owner: { type: "string" }
        }
      }
    },
    visible_final_answer: { type: "string" },
    final_answer_text: { type: "string" }
  }
};

const ALLOWED_EVIDENCE_MATURITY = new Set([
  "已核验事实",
  "统计特征",
  "可疑特征",
  "线索",
  "需复核",
  "不可判断"
]);

const SUPPORTED_STATUSES = new Set([
  "supported",
  "verified",
  "已核验事实",
  "统计特征",
  "可疑特征"
]);

const LEGAL_UPGRADE_PATTERN = /(?:违法所得|赃款|非法所得|洗钱事实成立|虚开事实成立|实际控制|代持|最终归属|犯罪团伙|共同犯罪|坐实|锁定)/u;
const LEGAL_DOWNGRADE_PATTERN = /(?:涉嫌|可疑|线索|需复核|需进一步|待补证|暂不能认定|尚不能|不能直接证明|证据不足)/u;

function text(value) {
  return String(value == null ? "" : value).trim();
}

function arrayOf(value) {
  return Array.isArray(value) ? value : [];
}

function objectOf(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function uniqueTexts(values, limit = 24) {
  const seen = new Set();
  const output = [];
  for (const value of values.map(text).filter(Boolean)) {
    if (seen.has(value)) continue;
    seen.add(value);
    output.push(value);
    if (output.length >= limit) break;
  }
  return output;
}

function evidenceReceiptIds(source = {}) {
  const value = objectOf(source);
  const evidenceRefs = objectOf(value.evidence_refs);
  const sourceRefs = objectOf(value.source_refs);
  const citations = objectOf(value.citations);
  return uniqueTexts([
    value.evidence_receipt_id,
    value.receipt_id,
    ...arrayOf(value.evidence_receipt_ids),
    ...arrayOf(value.receipt_ids),
    evidenceRefs.evidence_receipt_id,
    evidenceRefs.receipt_id,
    ...arrayOf(evidenceRefs.evidence_receipt_ids),
    ...arrayOf(evidenceRefs.receipt_ids),
    sourceRefs.evidence_receipt_id,
    sourceRefs.receipt_id,
    ...arrayOf(sourceRefs.evidence_receipt_ids),
    ...arrayOf(sourceRefs.receipt_ids),
    citations.evidence_receipt_id,
    citations.receipt_id,
    ...arrayOf(citations.evidence_receipt_ids),
    ...arrayOf(citations.receipt_ids)
  ], 24);
}

function hasVerifiedEvidenceReceipt(source, settings, claimType, path) {
  const verifier = settings.verifyEvidenceReceipt;
  if (typeof verifier !== "function") return false;
  return evidenceReceiptIds(source).some((receiptId) => {
    try {
      return verifier({ receiptId, claimType, path, claim: objectOf(source) }) === true;
    } catch {
      return false;
    }
  });
}

function hasRelatedFactAnchor(source = {}) {
  return arrayOf(objectOf(source).related_fact_ids).map(text).some(Boolean);
}

function fail(errors, code, path, message, repair_action, severity = "error") {
  errors.push({
    severity,
    code,
    path,
    message,
    repair_action
  });
}

function warn(warnings, code, path, message, repair_action) {
  warnings.push({
    severity: "warning",
    code,
    path,
    message,
    repair_action
  });
}

function conclusionObject(value) {
  if (typeof value === "string") return { summary: text(value) };
  return objectOf(value);
}

function legalUpgradeUnguarded(value) {
  const body = text(value);
  return LEGAL_UPGRADE_PATTERN.test(body) && !LEGAL_DOWNGRADE_PATTERN.test(body);
}

function validateFact(fact, index, errors, settings) {
  const source = objectOf(fact);
  const path = `verified_facts[${index}]`;
  if (!text(source.fact || source.summary || source.statement)) {
    fail(errors, "FACT_TEXT_REQUIRED", `${path}.fact`, "已核验事实缺少可读事实表述。", "补写具体谁、何时、向谁、多少金额/笔数，不能只写标签。");
  }
  const supportStatus = text(source.support_status);
  if (!SUPPORTED_STATUSES.has(supportStatus)) {
    fail(errors, "FACT_NOT_VERIFIED", `${path}.support_status`, "verified_facts 只能承载已有数据支持的事实，候选/缺失/需复核事项应放入 evidence_boundaries。", "将该条降级为 evidence_boundaries，或回到 MCP/DuckDB 补足确定性证据。");
  }
  if (!hasVerifiedEvidenceReceipt(source, settings, "verified_fact", path)) {
    fail(errors, "FACT_EVIDENCE_RECEIPT_REJECTED", `${path}.evidence_receipt_ids`, "结论事实没有通过宿主权威 registry 的同案、同轮、同快照 EvidenceReceipt 核验。", "回到宿主 Evidence Registry 取得并逐字段核验 receipt；未知、伪造、错案或不支持本 claim 的 ID 必须拒绝，事实缺失时输出证据边界。");
  }
}

function validateFlowPath(pathItem, index, errors, settings) {
  const source = objectOf(pathItem);
  const edges = arrayOf(source.edges);
  const pathKey = `fund_flow_paths[${index}]`;
  const receiptOnPath = hasVerifiedEvidenceReceipt(source, settings, "fund_flow_path", pathKey);
  if (!text(source.path_summary) && !arrayOf(source.path_nodes).length && !edges.length) {
    fail(errors, "FLOW_PATH_EMPTY", `fund_flow_paths[${index}]`, "资金路径缺少路径说明、节点或交易边。", "补充可证实路径摘要；若只是线索，改入 evidence_boundaries。");
  }
  if (!edges.length && !receiptOnPath) {
    fail(errors, "FLOW_PATH_EVIDENCE_RECEIPT_REJECTED", `${pathKey}.evidence_receipt_ids`, "资金路径没有通过宿主 Evidence Registry 核验。", "无交易边时必须提供支持该路径的有效宿主回执；未知、伪造或错配 ID 必须拒绝。");
  }
  edges.forEach((edge, edgeIndex) => {
    const edgeSource = objectOf(edge);
    const status = text(edgeSource.support_status || edgeSource.edge_status || "unsupported");
    if (!SUPPORTED_STATUSES.has(status)) {
      fail(errors, "FLOW_EDGE_NOT_SUPPORTED", `fund_flow_paths[${index}].edges[${edgeIndex}]`, "资金流向路径中包含未支持或候选交易边。", "将该边移入 evidence_boundaries/next_proof_actions；先补交易明细后再画入资金流向图。");
    }
    const edgePath = `${pathKey}.edges[${edgeIndex}]`;
    if (!hasVerifiedEvidenceReceipt(edgeSource, settings, "fund_flow_edge", edgePath)) {
      fail(errors, "FLOW_EDGE_EVIDENCE_RECEIPT_REJECTED", `${edgePath}.evidence_receipt_ids`, "资金流向边没有通过宿主权威 registry 的逐边 EvidenceReceipt 核验。", "为该交易边提供同案、同轮、同快照且字段等值匹配的回执；不得用路径级或其他 claim 的引用批量支持。");
    }
  });
}

function validateRiskPattern(pattern, index, errors, settings, verifiedFactIds) {
  const source = objectOf(pattern);
  const path = `risk_patterns[${index}]`;
  if (!text(source.pattern)) {
    fail(errors, "RISK_PATTERN_REQUIRED", `${path}.pattern`, "异常特征缺少特征名称。", "写明短期集中入账、快速分散转出、频繁同额、资金断点等具体特征。");
  }
  if (!text(source.basis)) {
    fail(errors, "RISK_BASIS_REQUIRED", `${path}.basis`, "异常特征缺少事实依据。", "用已核验事实或证据引用说明该特征为何成立。");
  }
  const relatedFactsVerified = arrayOf(source.related_fact_ids)
    .map(text)
    .filter(Boolean)
    .every((factId) => verifiedFactIds.has(factId));
  if (!hasVerifiedEvidenceReceipt(source, settings, "risk_pattern", path) && !(hasRelatedFactAnchor(source) && relatedFactsVerified)) {
    fail(errors, "RISK_PATTERN_UNANCHORED", `${path}.evidence_receipt_ids`, "异常特征没有有效宿主 EvidenceReceipt，或引用了未通过核验的事实。", "关联已通过宿主 registry 核验的 verified_facts，或取得支持该特征的同案、同轮、同快照回执；不能凭经验补写。");
  }
}

function validateBoundary(boundary, index, errors) {
  const source = typeof boundary === "string" ? { boundary } : objectOf(boundary);
  if (!text(source.boundary || source.reason)) {
    fail(errors, "BOUNDARY_TEXT_REQUIRED", `evidence_boundaries[${index}]`, "证据边界缺少可读说明。", "说明当前数据只能证明什么、尚不能认定什么，以及缺口原因。");
  }
}

function validateProofAction(action, index, errors, warnings) {
  const source = objectOf(action);
  const path = `next_proof_actions[${index}]`;
  if (!text(source.action || source.material)) {
    fail(errors, "PROOF_ACTION_REQUIRED", `${path}.action`, "补证动作缺少调取事项。", "写明调取开户资料、交易对手身份、回单、合同、发票、平台/资产/税务等具体材料。");
  }
  if (!text(source.proof_value || source.purpose)) {
    fail(errors, "PROOF_VALUE_REQUIRED", `${path}.proof_value`, "补证动作缺少证明目的。", "说明该材料用于证明资金性质、账户控制、交易目的、承接关系或最终去向。");
  }
  if (!text(source.target)) {
    warn(warnings, "PROOF_ACTION_TARGET_RECOMMENDED", `${path}.target`, "补证动作缺少调取对象。", "补充银行、支付机构、平台、账户、主体、对手方或内部数据修复对象。");
  }
  if (!text(source.material)) {
    warn(warnings, "PROOF_ACTION_MATERIAL_RECOMMENDED", `${path}.material`, "补证动作缺少材料名称。", "写明开户资料、交易回单、后续流水、合同发票、平台订单、设备日志或资产登记证明等具体材料。");
  }
  if (!text(source.time_scope || source.field_scope)) {
    warn(warnings, "PROOF_ACTION_SCOPE_RECOMMENDED", `${path}.time_scope`, "补证动作缺少期间或字段范围。", "补充拟调取期间、账号范围、字段范围或交易区间，避免笼统补证。");
  }
  if (!text(source.linked_gap || source.evidence_boundary)) {
    warn(warnings, "PROOF_ACTION_LINKED_GAP_RECOMMENDED", `${path}.linked_gap`, "补证动作缺少对应证据缺口。", "关联 evidence_boundaries 中的缺口，说明该材料拟补哪一段证明链。");
  }
  if (!text(source.forbidden_upgrade)) {
    warn(warnings, "PROOF_ACTION_FORBIDDEN_UPGRADE_RECOMMENDED", `${path}.forbidden_upgrade`, "补证动作缺少禁止升级边界。", "说明该材料未取得前不得认定资金性质、控制关系、最终归属或法律事实。");
  }
}

function auditCategoryForCode(code) {
  const value = text(code);
  if (/^(OBJECTIVE|CONCLUSION)/u.test(value)) return "structure";
  if (/(?:FACT|SOURCE|EVIDENCE|REFS|FLOW_EDGE_MISSING|FLOW_PATH_MISSING)/u.test(value)) return "source_evidence";
  if (/^FLOW_/u.test(value)) return "fund_flow";
  if (/^RISK_/u.test(value)) return "risk_pattern";
  if (/^BOUNDARY_/u.test(value)) return "evidence_boundary";
  if (/^PROOF_ACTION_/u.test(value)) return "proof_action";
  if (/(?:LEGAL|VISIBLE|FORBIDDEN)/u.test(value)) return "delivery_qc";
  return "validation";
}

function factPolicyForValidationIssue(issue) {
  const body = text(objectOf(issue).repair_action || objectOf(issue).message);
  return /MCP|DuckDB|证据|流水|交易|source_refs|evidence_refs|回到/u.test(body)
    ? "missing_facts_return_to_mcp_duckdb_or_become_evidence_boundaries"
    : "structure_or_wording_may_be_repaired_without_adding_facts";
}

function auditEventsForValidationIssues({ errors, warnings, status }) {
  const issues = [
    ...arrayOf(errors),
    ...arrayOf(warnings)
  ];
  return issues.map((issue) => {
    const source = objectOf(issue);
    return {
      event: "investigation_answer_validation_issue",
      status,
      severity: text(source.severity) || "error",
      code: text(source.code),
      path: text(source.path),
      category: auditCategoryForCode(source.code),
      fact_policy: factPolicyForValidationIssue(source),
      repair_action: text(source.repair_action)
    };
  }).filter((event) => event.code);
}

export function validationErrorsAsReask(errors) {
  return arrayOf(errors).map((error) => ({
    code: text(error.code),
    path: text(error.path),
    instruction: text(error.repair_action || error.message),
    fact_policy: /MCP|DuckDB|证据|流水|交易|Evidence\s*Registry|receipt|source_refs|evidence_refs/iu.test(text(error.repair_action || error.message))
      ? "missing facts must return to the host Evidence Registry and verified tool execution or become evidence_boundaries; do not invent"
      : "repair the structure without changing supported facts"
  })).filter((item) => item.code && item.instruction);
}

export function validateInvestigationAnswerContract(answer, options = {}) {
  const source = objectOf(answer);
  const settings = objectOf(options);
  const errors = [];
  const warnings = [];
  const audit_events = [
    {
      event: "investigation_answer_validation:start",
      contract_version: INVESTIGATION_ANSWER_CONTRACT_VERSION
    }
  ];

  const objective = objectOf(source.investigation_objective);
  if (!text(objective.purpose)) {
    fail(errors, "OBJECTIVE_PURPOSE_REQUIRED", "investigation_objective.purpose", "缺少本次研判目的。", "补充用户要解决的办案问题；若目的会改变范围，先向用户追问。");
  }
  if (!text(objective.scope)) {
    fail(errors, "OBJECTIVE_SCOPE_REQUIRED", "investigation_objective.scope", "缺少研判对象或统计范围。", "列明主体、账户、对手方、期间或专题范围。");
  }
  if (!text(objective.source_boundary)) {
    fail(errors, "OBJECTIVE_SOURCE_BOUNDARY_REQUIRED", "investigation_objective.source_boundary", "缺少数据来源边界。", "说明结论基于当前案件 DuckDB/MCP 已核验事实、清洗表或具体附件。");
  }

  const conclusion = conclusionObject(source.conclusion);
  if (!text(conclusion.summary)) {
    fail(errors, "CONCLUSION_SUMMARY_REQUIRED", "conclusion.summary", "缺少结论摘要。", "用经侦口吻写明发现/未发现什么异常及证据成熟度。");
  }
  if (!ALLOWED_EVIDENCE_MATURITY.has(text(conclusion.evidence_maturity))) {
    fail(errors, "CONCLUSION_MATURITY_REQUIRED", "conclusion.evidence_maturity", "结论缺少有效证据成熟度。", "使用已核验事实、统计特征、可疑特征、线索、需复核或不可判断。");
  }
  if (legalUpgradeUnguarded(conclusion.summary)) {
    fail(errors, "LEGAL_CONCLUSION_UNSUPPORTED", "conclusion.summary", "结论从流水事实直接升级为法律敏感认定。", "降级为涉嫌、可疑、线索、需补证或暂不能认定，并列明缺少的外部证据。");
  }

  const facts = arrayOf(source.verified_facts);
  if (!facts.length) {
    fail(errors, "VERIFIED_FACTS_REQUIRED", "verified_facts", "最终研判缺少已核验事实。", "从 MCP/DuckDB 工具结果提取事实；若没有事实，结论只能写不可判断和证据边界。");
  }
  const verifiedFactIds = new Set(facts
    .filter((fact, index) => hasVerifiedEvidenceReceipt(fact, settings, "verified_fact", `verified_facts[${index}]`))
    .map((fact) => text(objectOf(fact).fact_id))
    .filter(Boolean));
  facts.forEach((fact, index) => validateFact(fact, index, errors, settings));

  arrayOf(source.fund_flow_paths).forEach((pathItem, index) => validateFlowPath(pathItem, index, errors, settings));

  const risks = arrayOf(source.risk_patterns);
  if (!risks.length && !settings.allowNoRiskPattern) {
    fail(errors, "RISK_PATTERNS_REQUIRED", "risk_patterns", "最终研判缺少异常特征或明确无异常说明。", "写出已核验异常特征；确无异常时写明核验范围和证据边界。");
  }
  risks.forEach((pattern, index) => validateRiskPattern(pattern, index, errors, settings, verifiedFactIds));

  const boundaries = arrayOf(source.evidence_boundaries);
  if (!boundaries.length) {
    fail(errors, "EVIDENCE_BOUNDARIES_REQUIRED", "evidence_boundaries", "最终研判缺少证据边界。", "说明当前数据不能证明的所有权、控制、资金性质、最终去向或法律结论。");
  }
  boundaries.forEach((boundary, index) => validateBoundary(boundary, index, errors));

  const actions = arrayOf(source.next_proof_actions);
  if (!actions.length) {
    fail(errors, "NEXT_PROOF_ACTIONS_REQUIRED", "next_proof_actions", "最终研判缺少下一步补证动作。", "列明调取对象、材料、期间和证明目的。");
  }
  actions.forEach((action, index) => validateProofAction(action, index, errors, warnings));

  const visible = text(source.visible_final_answer || source.final_answer_text);
  if (visible && /(?:DuckDB|MCP|schema|query_id|tool|workflow|JSON|doctor|eval|case_id)/iu.test(visible)) {
    fail(errors, "VISIBLE_ENGINEERING_LEAK", "visible_final_answer", "用户可见回答泄露工程词或内部协议。", "把内部来源翻译为公安经侦材料语言，保留事实、证据边界和补证动作。");
  }

  const status = errors.length ? "failed" : "passed";
  audit_events.push(...auditEventsForValidationIssues({ errors, warnings, status }));
  audit_events.push({
    event: `investigation_answer_validation:${status}`,
    error_count: errors.length,
    warning_count: warnings.length,
    categories: uniqueTexts([
      ...errors.map((error) => auditCategoryForCode(objectOf(error).code)),
      ...warnings.map((warning) => auditCategoryForCode(objectOf(warning).code))
    ], 12)
  });

  return {
    ok: errors.length === 0,
    contract_version: INVESTIGATION_ANSWER_CONTRACT_VERSION,
    required_fields: INVESTIGATION_ANSWER_REQUIRED_FIELDS,
    errors,
    warnings,
    repair_actions: validationErrorsAsReask(errors),
    audit_events
  };
}
