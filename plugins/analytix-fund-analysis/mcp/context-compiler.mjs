export const CONTEXT_COMPILER_VERSION = "context-compiler-v1";

function text(value) {
  return String(value == null ? "" : value).trim();
}

function arrayOf(value) {
  return Array.isArray(value) ? value : [];
}

function objectOf(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function pruneEmpty(value) {
  if (Array.isArray(value)) {
    const next = value.map(pruneEmpty).filter((item) => {
      if (item == null || item === "") return false;
      if (Array.isArray(item)) return item.length > 0;
      if (typeof item === "object") return Object.keys(item).length > 0;
      return true;
    });
    return next.length ? next : undefined;
  }
  if (value && typeof value === "object") {
    const next = {};
    for (const [key, item] of Object.entries(value)) {
      const compact = pruneEmpty(item);
      if (compact !== undefined && compact !== "") next[key] = compact;
    }
    return Object.keys(next).length ? next : undefined;
  }
  if (value == null || value === "") return undefined;
  return value;
}

function uniqueTexts(values, limit = 12) {
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

function humanizeText(value) {
  return text(value)
    .replace(/\bcasegraph:[\w:.-]+\b/giu, "")
    .replace(/\b(?:audit_ref|detail_ref|artifact_id|source_ref|source_refs|evidence_ref|evidence_refs|query_id|query_ids|evidence_ledger)\s*[:：]\s*[\w:.,;/-]+/giu, "")
    .replace(/\s+/gu, " ")
    .slice(0, 800);
}

function compilerTextList(...sources) {
  return uniqueTexts(
    sources.flatMap((source) => arrayOf(source).map((item) => {
      if (typeof item === "string") return item;
      const record = objectOf(item);
      return record.answer_text || record.summary || record.reason || record.text || record.claim || record.label || "";
    })),
    12
  ).map(humanizeText).filter(Boolean);
}

function compilerQueryList(source = {}) {
  const queries = [
    ...arrayOf(source.next_queries),
    ...arrayOf(source.next_review_actions).map((item) => ({ why_this_query: item })),
    ...arrayOf(source.next_actions).map((item) => ({
      why_this_query: objectOf(item).reason
    }))
  ];
  return queries
    .map((item) => {
      const query = objectOf(item);
      return pruneEmpty({
        why_this_query: humanizeText(query.why_this_query || query.reason || query.text || query.label)
      }) || {};
    })
    .filter((item) => Object.keys(item).length)
    .slice(0, 8);
}

export function answerCardContextCompiler(source = {}, stopConditions = []) {
  const card = objectOf(source);
  return pruneEmpty({
    version: CONTEXT_COMPILER_VERSION,
    model_context: "answer_card_only",
    raw_result_policy: "Raw MCP/backend payloads are not model-visible; retention may be claimed only when the host evidence registry confirms it.",
    must_write_facts: compilerTextList(
      card.must_write_facts,
      arrayOf(card.facts).map((fact) => objectOf(fact).answer_text || objectOf(fact).label),
      card.verified_claims,
      card.confirmed_facts,
      card.verified_facts
    ),
    must_state_boundaries: compilerTextList(
      card.must_state_boundaries,
      card.missing_source_boundaries,
      card.answer_constraints,
      card.warnings,
      card.quality_guards,
      card.stop_conditions
    ),
    forbidden_as_facts: compilerTextList(
      card.forbidden_as_facts,
      card.unsupported_claims,
      card.unsupported_flows,
      card.forbidden_phrasings,
      objectOf(card.claim_review).unsupported,
      objectOf(card.claim_review).downgraded
    ),
    next_queries: compilerQueryList(card),
    stop_conditions: compilerTextList(stopConditions)
  }) || {};
}
