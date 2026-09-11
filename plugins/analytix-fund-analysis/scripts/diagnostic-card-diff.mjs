import { renderDiagnosticCardForAudit } from "../mcp/card-renderer.mjs";

function text(value) {
  return String(value == null ? "" : value).trim();
}

function objectOf(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function arrayOf(value) {
  return Array.isArray(value) ? value : [];
}

function numberValue(value) {
  if (value && typeof value === "object") {
    const source = objectOf(value);
    return numberValue(source.yuan ?? source.amount_yuan ?? source.amount ?? source.value);
  }
  const normalized = String(value ?? "").replace(/[,，\s]/gu, "");
  if (!normalized) return null;
  const next = Number(normalized);
  return Number.isFinite(next) ? next : null;
}

function intValue(value) {
  const next = Number(value ?? 0);
  return Number.isFinite(next) ? Math.trunc(next) : 0;
}

function amountCents(value) {
  const next = numberValue(value);
  return next == null ? null : Math.round(next * 100);
}

function pruneEmpty(value) {
  if (Array.isArray(value)) {
    return value.map(pruneEmpty).filter((item) => item !== undefined);
  }
  if (!value || typeof value !== "object") {
    return value === "" || value === null ? undefined : value;
  }
  const output = {};
  for (const [key, item] of Object.entries(value)) {
    const next = pruneEmpty(item);
    if (next !== undefined) output[key] = next;
  }
  return Object.keys(output).length ? output : undefined;
}

function holderSubject(fact) {
  const holder = text(fact.holder);
  const idNo = text(fact.id_no);
  return holder && idNo ? `${holder}/${idNo}` : holder;
}

function normalizeCounterparty(fact) {
  const name = text(fact.counterparty);
  const account = text(fact.counterparty_account);
  if (!name && !account) return "空户名";
  if (!name && account) return `空户名/账号:${account}`;
  return account ? `${name}/${account}` : name;
}

function canonicalFact(fact) {
  const item = objectOf(fact);
  return pruneEmpty({
    fact_id: text(item.fact_id),
    label: text(item.label),
    value: typeof item.value === "number" ? item.value : text(item.value),
    amount_cents: item.amount == null ? null : amountCents(item.amount),
    count: item.count == null ? undefined : intValue(item.count),
    account: text(item.account),
    holder_subject: holderSubject(item),
    counterparty: normalizeCounterparty(item),
    direction: text(item.direction),
    time_range: text(item.time_range),
    source_level: text(item.source_level),
    support_status: text(item.support_status),
    support_query_name: text(item.support_query_name),
    golden_anchor_key: text(item.golden_anchor_key),
    source_hash: text(item.source_hash)
  }) || {};
}

export function validateDiagnosticCard(card, cardTypes) {
  const source = objectOf(card);
  const errors = [];
  if (!cardTypes.has(text(source.card_type))) errors.push("invalid card_type");
  if (!text(source.case_id)) errors.push("missing case_id");
  if (!text(source.intent)) errors.push("missing intent");
  if (!arrayOf(source.facts).length) errors.push("missing facts");
  if (source.answer_card_complete !== true) errors.push("missing answer_card_complete=true");
  if (!text(source.recommended_next_action)) errors.push("missing recommended_next_action");
  if (!Number.isFinite(Number(source.max_additional_tools))) errors.push("missing max_additional_tools");
  if (typeof source.required_facts_present !== "boolean") errors.push("missing required_facts_present");
  if (typeof source.unsupported_flows_present !== "boolean") errors.push("missing unsupported_flows_present");
  for (const [index, fact] of arrayOf(source.facts).entries()) {
    const item = objectOf(fact);
    if (!text(item.fact_id)) errors.push(`facts[${index}] missing fact_id`);
    if (!text(item.label)) errors.push(`facts[${index}] missing label`);
    if (!text(item.support_status)) errors.push(`facts[${index}] missing support_status`);
    if (!text(item.answer_text)) errors.push(`facts[${index}] missing answer_text`);
    if (!text(item.support_query_name)) errors.push(`facts[${index}] missing support_query_name`);
    if (!text(item.golden_anchor_key) && text(source.fact_source).includes("oracle")) errors.push(`facts[${index}] missing golden_anchor_key`);
    if (!text(item.source_hash)) errors.push(`facts[${index}] missing source_hash`);
  }
  return { ok: errors.length === 0, errors };
}

export function canonicalizeDiagnosticCard(card) {
  const source = objectOf(card);
  const facts = arrayOf(source.facts).map(canonicalFact);
  return {
    card_type: text(source.card_type),
    case_id: text(source.case_id),
    intent: text(source.intent),
    facts_by_id: Object.fromEntries(facts.map((fact) => [fact.fact_id || fact.label, fact])),
    warnings: arrayOf(source.warnings).map(text).filter(Boolean).sort(),
    unsupported_claims: arrayOf(source.unsupported_claims).map(text).filter(Boolean).sort(),
    answer_constraints: arrayOf(source.answer_constraints).map(text).filter(Boolean).sort(),
    claim_review: {
      supported: arrayOf(source.claim_review?.supported).map(text).filter(Boolean).sort(),
      corrected: arrayOf(source.claim_review?.corrected).map(text).filter(Boolean).sort(),
      unsupported: arrayOf(source.claim_review?.unsupported).map(text).filter(Boolean).sort(),
      downgraded: arrayOf(source.claim_review?.downgraded).map(text).filter(Boolean).sort()
    },
    protocol: {
      answer_card_complete: source.answer_card_complete === true,
      recommended_next_action: text(source.recommended_next_action),
      max_additional_tools: intValue(source.max_additional_tools),
      required_facts_present: source.required_facts_present === true,
      unsupported_flows_present: source.unsupported_flows_present === true
    }
  };
}

function compareField(factId, field, oracle, production, hardDiff) {
  if (oracle[field] === undefined || oracle[field] === null || oracle[field] === "") {
    return;
  }
  if ((oracle[field] ?? "") !== (production[field] ?? "")) {
    hardDiff.push({
      fact_id: factId,
      field,
      oracle: oracle[field] ?? null,
      production: production[field] ?? null
    });
  }
}

function missingSetDiff(label, oracleItems, productionItems, coverageDiff) {
  const productionSet = new Set(productionItems);
  for (const item of oracleItems) {
    if (!productionSet.has(item)) {
      coverageDiff.push({ field: label, missing: item });
    }
  }
}

export function diffDiagnosticCards({ oracleCard, productionCard }) {
  const oracle = canonicalizeDiagnosticCard(oracleCard);
  const production = canonicalizeDiagnosticCard(productionCard);
  const hardDiff = [];
  const coverageDiff = [];
  const renderDiff = [];
  const productionFactGaps = [];
  if (oracle.card_type !== production.card_type) {
    hardDiff.push({ field: "card_type", oracle: oracle.card_type, production: production.card_type });
  }
  if (oracle.case_id !== production.case_id) {
    hardDiff.push({ field: "case_id", oracle: oracle.case_id, production: production.case_id });
  }
  if (oracle.intent !== production.intent) {
    coverageDiff.push({ field: "intent", oracle: oracle.intent, production: production.intent });
  }
  for (const [factId, oracleFact] of Object.entries(oracle.facts_by_id)) {
    const productionFact = production.facts_by_id[factId];
    if (!productionFact) {
      const gap = { fact_id: factId, label: oracleFact.label, reason: "production card missing oracle fact" };
      hardDiff.push(gap);
      productionFactGaps.push(gap);
      continue;
    }
    for (const field of ["amount_cents", "count", "account", "holder_subject", "counterparty", "direction", "time_range"]) {
      compareField(factId, field, oracleFact, productionFact, hardDiff);
    }
    for (const field of ["support_status", "support_query_name", "source_hash"]) {
      if (!text(productionFact[field])) {
        coverageDiff.push({ fact_id: factId, field, reason: "production fact missing provenance/support metadata" });
      }
    }
  }
  missingSetDiff("warnings", oracle.warnings, production.warnings, coverageDiff);
  missingSetDiff("unsupported_claims", oracle.unsupported_claims, production.unsupported_claims, coverageDiff);
  missingSetDiff("answer_constraints", oracle.answer_constraints, production.answer_constraints, coverageDiff);
  for (const key of ["supported", "corrected", "unsupported", "downgraded"]) {
    missingSetDiff(`claim_review.${key}`, oracle.claim_review[key], production.claim_review[key], coverageDiff);
  }
  if (oracle.protocol.answer_card_complete !== production.protocol.answer_card_complete) {
    coverageDiff.push({ field: "protocol.answer_card_complete", oracle: oracle.protocol.answer_card_complete, production: production.protocol.answer_card_complete });
  }
  if (oracle.protocol.max_additional_tools !== production.protocol.max_additional_tools) {
    coverageDiff.push({ field: "protocol.max_additional_tools", oracle: oracle.protocol.max_additional_tools, production: production.protocol.max_additional_tools });
  }
  const oracleText = renderDiagnosticCardForAudit(oracleCard, { includeTitle: false });
  const productionText = renderDiagnosticCardForAudit(productionCard, { includeTitle: false });
  if (oracleText !== productionText) {
    renderDiff.push({
      field: "rendered_text",
      reason: "natural-language rendering differs; this is non-blocking unless hard/coverage diff exists"
    });
  }
  return {
    hard_diff: hardDiff,
    coverage_diff: coverageDiff,
    render_diff: renderDiff,
    production_fact_gaps: productionFactGaps,
    summary: {
      ok: hardDiff.length === 0,
      hard_diff_count: hardDiff.length,
      coverage_diff_count: coverageDiff.length,
      render_diff_count: renderDiff.length,
      production_fact_gap_count: productionFactGaps.length
    }
  };
}
