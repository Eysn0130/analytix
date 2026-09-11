import crypto from "node:crypto";

export const EVIDENCE_LEDGER_VERSION = "evidence-ledger-v1";

const ENTRY_SCAN_LIMIT = 260;
const ENTRY_OUTPUT_LIMIT = 200;

const LEDGER_FIELD_KEYS = [
  "fact_id",
  "label",
  "support_query_name",
  "source_hash",
  "source_level",
  "amount",
  "amount_yuan",
  "amount_total",
  "value",
  "unit",
  "inflow",
  "inflow_total",
  "outflow",
  "outflow_total",
  "turnover",
  "turnover_total",
  "net_flow",
  "max_single",
  "max_single_amount",
  "count",
  "txn_count",
  "row_count",
  "account_count",
  "edge_count",
  "node_count",
  "supported_edge_count",
  "incomplete_edge_count",
  "source_seed_count",
  "downstream_seed_count",
  "blocking_gate_count",
  "claim_id",
  "claim_category",
  "risk_marker",
  "account",
  "account_key",
  "account_display",
  "account_open_name",
  "display_name",
  "holder",
  "holder_name",
  "id_no",
  "counterparty",
  "counterparty_name",
  "counterparty_account",
  "counterparty_key",
  "txn_id",
  "txn_time",
  "direction",
  "time_range",
  "edge_id",
  "edge_status",
  "from",
  "from_label",
  "to",
  "to_label",
  "missing_fields",
  "source_tool",
  "support_owner",
  "focused_owner"
];

const LEDGER_TRIGGER_KEYS = new Set([
  "fact_id",
  "support_query_name",
  "source_hash",
  "amount",
  "amount_yuan",
  "amount_total",
  "value",
  "inflow",
  "inflow_total",
  "outflow",
  "outflow_total",
  "turnover",
  "turnover_total",
  "net_flow",
  "max_single",
  "max_single_amount",
  "count",
  "txn_count",
  "row_count",
  "account_count",
  "edge_count",
  "node_count",
  "supported_edge_count",
  "incomplete_edge_count",
  "source_seed_count",
  "downstream_seed_count",
  "blocking_gate_count",
  "claim_id",
  "claim_category",
  "risk_marker",
  "account",
  "account_key",
  "account_display",
  "holder",
  "holder_name",
  "counterparty",
  "counterparty_name",
  "txn_id",
  "edge_id",
  "edge_status",
  "support_owner",
  "focused_owner"
]);

const LEDGER_CLAIM_LIST_KINDS = new Map([
  ["verified_claims", "verified_claim"],
  ["corrected_claims", "corrected_claim"],
  ["unsupported_claims", "unsupported_claim"],
  ["unsupported_flows", "unsupported_flow"],
  ["missing_source_boundaries", "missing_source_boundary"],
  ["forbidden_phrasings", "forbidden_phrasing"],
  ["next_review_actions", "next_review_action"],
  ["claim_support_index", "claim_support"],
  ["claim_support", "claim_support"],
  ["claim_support_refs", "claim_support"],
  ["must_write_facts", "context_must_write_fact"],
  ["must_state_boundaries", "context_must_state_boundary"],
  ["next_queries", "context_next_query"],
  ["stop_conditions", "context_stop_condition"],
  ["forbidden_as_facts", "forbidden_as_fact"]
]);

const LEDGER_CONTEXT_ONLY_KEYS = new Set(["scope"]);

export const EVIDENCE_LEDGER_REQUIRED_SURFACES = [
  "amount",
  "count",
  "account",
  "holder",
  "counterparty",
  "flow_or_transaction_edge",
  "report_claim",
  "source_refs"
];

export const EVIDENCE_LEDGER_TRACE_REQUIRED_SURFACES = EVIDENCE_LEDGER_REQUIRED_SURFACES.filter(
  (surface) => surface !== "source_refs"
);

function text(value) {
  return String(value == null ? "" : value).trim();
}

function arrayOf(value) {
  return Array.isArray(value) ? value : [];
}

function objectOf(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function nonNegativeInteger(value) {
  if (value == null || (typeof value === "string" && !value.trim())) return undefined;
  if (typeof value !== "number" && typeof value !== "string") return undefined;
  if (typeof value === "string" && !/^(?:0|[1-9]\d*)$/u.test(value.trim())) return undefined;
  const number = Number(typeof value === "string" ? value.trim() : value);
  return Number.isSafeInteger(number) && number >= 0 ? number : undefined;
}

function stableHash(value) {
  return crypto
    .createHash("sha256")
    .update(JSON.stringify(value || {}, (_key, item) => (typeof item === "bigint" ? String(item) : item)))
    .digest("hex");
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

function humanizeLedgerText(value) {
  return text(value)
    .replace(/\bcasegraph:[\w:.-]+\b/giu, "")
    .replace(/\b(?:audit_ref|detail_ref|artifact_id|source_ref|source_refs|evidence_ref|evidence_refs|query_id|query_ids|evidence_ledger)\s*[:：]\s*[\w:.,;/-]+/giu, "")
    .replace(/\s+/gu, " ")
    .slice(0, 600);
}

function mergeRefList(...sources) {
  return uniqueTexts([
    ...sources.flatMap((source) => arrayOf(source).map(text))
  ], 16);
}

function mergeObjects(...sources) {
  const output = {};
  for (const source of sources.map(objectOf)) {
    for (const [key, value] of Object.entries(source)) {
      if (value !== undefined && value !== null && value !== "") output[key] = value;
    }
  }
  return output;
}

function rootLineageSeed(source = {}, options = {}) {
  const settings = objectOf(options);
  return {
    ...objectOf(source),
    evidence_refs: mergeObjects(objectOf(source.evidence_refs), objectOf(settings.evidence_refs)),
    detail_ref: mergeObjects(objectOf(source.detail_ref), objectOf(settings.detail_ref || settings.detailRef))
  };
}

function lineageFromObject(source = {}) {
  const sourceRefs = objectOf(source.source_refs);
  const refs = objectOf(source.evidence_refs);
  const citations = objectOf(source.citations);
  const detailRef = objectOf(source.detail_ref);
  return pruneEmpty({
    query_ids: mergeRefList(sourceRefs.query_ids, refs.query_ids, citations.query_ids, source.query_ids),
    evidence_ids: mergeRefList(sourceRefs.evidence_ids, refs.evidence_ids, citations.evidence_ids, source.evidence_ids),
    path_ids: mergeRefList(sourceRefs.path_ids, refs.path_ids, citations.path_ids, source.path_ids),
    report_ids: mergeRefList(sourceRefs.report_ids, refs.report_ids, citations.report_ids, source.report_ids),
    audit_ref: mergeObjects(sourceRefs.audit_ref, refs.audit_ref, citations.audit_ref, source.audit_ref),
    detail_ref: mergeObjects(sourceRefs.detail_ref, detailRef),
    artifact_id: text(sourceRefs.artifact_id || refs.artifact_id || detailRef.artifact_id || source.artifact_id)
  }) || {};
}

function ledgerValue(value) {
  if (value == null || value === "") return undefined;
  if (typeof value === "number" || typeof value === "boolean") return value;
  if (typeof value === "string") return humanizeLedgerText(value);
  if (Array.isArray(value)) return pruneEmpty(value.slice(0, 12));
  if (typeof value === "object") return pruneEmpty(value);
  return text(value);
}

function ledgerKindForPath(pathParts, source = {}) {
  const last = text(pathParts[pathParts.length - 1]);
  if (LEDGER_CLAIM_LIST_KINDS.has(last)) return LEDGER_CLAIM_LIST_KINDS.get(last);
  if (text(source.edge_id) || text(source.txn_id) || text(source.edge_status)) return "flow_or_transaction_edge";
  if (text(source.fact_id)) return "deterministic_fact";
  if (/claim/iu.test(pathParts.join("."))) return "report_claim";
  if (/flow_graph|edge|mermaid/iu.test(pathParts.join("."))) return "flow_or_transaction_edge";
  return "fact_field";
}

function ledgerFieldsFromObject(source = {}) {
  const fields = {};
  for (const key of LEDGER_FIELD_KEYS) {
    if (!Object.prototype.hasOwnProperty.call(source, key)) continue;
    const value = ledgerValue(source[key]);
    if (value !== undefined) fields[key] = value;
  }
  return pruneEmpty(fields) || {};
}

function hasLedgerSignal(source = {}) {
  return Object.keys(source).some((key) => LEDGER_TRIGGER_KEYS.has(key));
}

function supportStatusForEntry(kind, source = {}) {
  const explicit = text(source.support_status || source.edge_status);
  if (/^(?:candidate|unresolved|unsupported|refuted|rejected|missing|partial|boundary|forbidden|review_action|needs?_evidence|needs?_review)$/u.test(explicit)) {
    return explicit;
  }
  if (/unsupported|forbidden|missing_source_boundary/u.test(kind)) return "unsupported";
  if (/next_review_action/u.test(kind)) return "review_action";
  // This plugin cannot authenticate host Evidence Registry membership. Positive
  // support labels in tool/model payloads are therefore untrusted input.
  return "unresolved";
}

function hasAnyField(fields, keys) {
  return keys.some((key) => Object.prototype.hasOwnProperty.call(fields, key) && fields[key] !== undefined && fields[key] !== "");
}

function hasSourceRefs(sourceRefs = {}) {
  const refs = objectOf(sourceRefs);
  return Boolean(
    arrayOf(refs.query_ids).length
    || arrayOf(refs.evidence_ids).length
    || arrayOf(refs.path_ids).length
    || arrayOf(refs.report_ids).length
    || Object.keys(objectOf(refs.audit_ref)).length
    || Object.keys(objectOf(refs.detail_ref)).length
    || text(refs.artifact_id)
  );
}

function surfaceSignalsForEntry(entry = {}) {
  const source = objectOf(entry);
  const fields = objectOf(source.fields);
  const kind = text(source.kind);
  const sourcePath = text(source.source_path);
  const signals = [];
  if (hasAnyField(fields, ["amount", "amount_yuan", "amount_total", "value", "inflow", "inflow_total", "outflow", "outflow_total", "turnover", "turnover_total", "net_flow", "max_single", "max_single_amount"])) signals.push("amount");
  if (hasAnyField(fields, ["count", "txn_count", "row_count", "account_count", "edge_count", "node_count", "supported_edge_count", "incomplete_edge_count", "source_seed_count", "downstream_seed_count", "blocking_gate_count"])) signals.push("count");
  if (hasAnyField(fields, ["claim_id", "claim_category", "risk_marker"])) signals.push("report_claim");
  if (hasAnyField(fields, ["account", "account_key", "account_display", "counterparty_account", "counterparty_key", "from", "to"])) signals.push("account");
  if (hasAnyField(fields, ["holder", "holder_name", "account_open_name", "id_no", "from_label", "display_name"])) signals.push("holder");
  if (hasAnyField(fields, ["counterparty", "counterparty_name", "counterparty_account", "counterparty_key", "to", "to_label", "display_name"])) signals.push("counterparty");
  if (/flow_or_transaction_edge/u.test(kind) || hasAnyField(fields, ["edge_id", "edge_status", "txn_id", "txn_time", "source_tool"])) signals.push("flow_or_transaction_edge");
  if (/claim|forbidden_phrasing|next_review_action/u.test(kind) || text(source.claim_text)) signals.push("report_claim");
  if (/context_/u.test(kind) || /context_compiler/u.test(sourcePath)) signals.push("context_compiler_directive");
  if (/unsupported_flow/u.test(kind)) signals.push("unsupported_flow");
  if (hasSourceRefs(source.source_refs)) signals.push("source_refs");
  return uniqueTexts(signals, 12);
}

function buildCoverageSummary(entries = []) {
  const surfaceCounts = {};
  const supportStatusCounts = {};
  const untraceableRequiredSurfaceEntries = [];
  for (const entry of entries) {
    const signals = arrayOf(entry.surface_signals);
    for (const signal of signals) {
      surfaceCounts[signal] = Number(surfaceCounts[signal] || 0) + 1;
    }
    const supportStatus = text(entry.support_status) || "unresolved";
    supportStatusCounts[supportStatus] = Number(supportStatusCounts[supportStatus] || 0) + 1;
    const requiredSurfaces = signals.filter((signal) => EVIDENCE_LEDGER_TRACE_REQUIRED_SURFACES.includes(signal));
    if (requiredSurfaces.length && !signals.includes("source_refs")) {
      untraceableRequiredSurfaceEntries.push({
        entry_id: text(entry.entry_id),
        fact_id: text(entry.fact_id),
        kind: text(entry.kind),
        source_path: text(entry.source_path),
        support_status: text(entry.support_status),
        missing_source_refs_for: requiredSurfaces
      });
    }
  }
  return {
    coverage_scope: "all_scanned_entries_before_output_truncation",
    coverage_entry_count: entries.length,
    entry_output_limit: ENTRY_OUTPUT_LIMIT,
    coverage_includes_overflow_entries: true,
    required_surfaces: EVIDENCE_LEDGER_REQUIRED_SURFACES,
    trace_required_surfaces: EVIDENCE_LEDGER_TRACE_REQUIRED_SURFACES,
    traceability_policy: "Every amount, count, account, holder, counterparty, flow/transaction edge, and report claim ledger entry must carry source_refs to deterministic fact / SQL / MCP result / artifact.",
    receipt_policy: "Traceability is not EvidenceReceipt registry membership. This plugin cannot mark a ledger entry supported or publication-ready.",
    host_registry_verified: false,
    publication_ready: false,
    support_status_counts: supportStatusCounts,
    surface_counts: surfaceCounts,
    missing_required_surfaces: EVIDENCE_LEDGER_REQUIRED_SURFACES.filter((surface) => !Number(surfaceCounts[surface] || 0)),
    entries_with_source_refs: Number(surfaceCounts.source_refs || 0),
    entries_without_source_refs: entries.filter((entry) => !arrayOf(entry.surface_signals).includes("source_refs")).length,
    untraceable_required_surface_entry_count: untraceableRequiredSurfaceEntries.length,
    untraceable_required_surface_entries: untraceableRequiredSurfaceEntries.slice(0, 24)
  };
}

function deterministicFactId(kind, sourcePath, fields = {}, sourceRefs = {}, claimText = "") {
  const explicit = text(fields.fact_id);
  if (explicit) return explicit;
  const edgeId = text(fields.edge_id);
  if (edgeId) return `edge:${edgeId}`;
  const txnId = text(fields.txn_id);
  if (txnId) return `txn:${stableHash({ txn_id: txnId, amount: fields.amount ?? fields.amount_yuan, source_path: sourcePath })}`;
  return `fact:${stableHash({ kind, source_path: sourcePath, fields, source_refs: sourceRefs, claim_text: claimText })}`;
}

function pushLedgerEntry(entries, entry) {
  if (entries.length >= ENTRY_SCAN_LIMIT) return;
  const compact = pruneEmpty(entry);
  if (!compact) return;
  const sourcePath = text(compact.source_path || compact.path || "$");
  const kind = text(compact.kind) || "fact_field";
  const fields = objectOf(compact.fields);
  const sourceRefs = objectOf(compact.source_refs);
  const claimText = text(compact.claim_text);
  const factId = deterministicFactId(kind, sourcePath, fields, sourceRefs, claimText);
  const normalized = {
    ...compact,
    fact_id: factId,
    source_path: sourcePath,
    support_status: supportStatusForEntry(kind, {
      ...fields,
      support_status: compact.support_status
    }),
    host_registry_verified: false,
    citation_status: "unresolved"
  };
  const surfaceSignals = surfaceSignalsForEntry(normalized);
  entries.push({
    ...normalized,
    surface_signals: surfaceSignals.length ? surfaceSignals : undefined,
    entry_id: `el-${stableHash(normalized)}`
  });
}

function collectEvidenceLedgerEntries(value, pathParts = [], entries = [], depth = 0) {
  if (entries.length >= ENTRY_SCAN_LIMIT || depth > 9 || value == null) return entries;
  const lastPath = text(pathParts[pathParts.length - 1]);
  if (LEDGER_CONTEXT_ONLY_KEYS.has(lastPath)) return entries;
  const sourcePath = pathParts.join(".") || "$";
  if (LEDGER_CLAIM_LIST_KINDS.has(lastPath) && Array.isArray(value)) {
    const claimKind = LEDGER_CLAIM_LIST_KINDS.get(lastPath);
    value.slice(0, 80).forEach((item, index) => {
      const source = objectOf(item);
      const itemLineage = lineageFromObject(source);
      const claimText = ledgerValue(typeof item === "string" ? item : source.claim_text || source.text || source.claim || source.summary || source.reason || JSON.stringify(source));
      pushLedgerEntry(entries, {
        kind: claimKind,
        source_path: [...pathParts, String(index)].join("."),
        support_status: supportStatusForEntry(claimKind, source),
        source_refs: itemLineage,
        fields: ledgerFieldsFromObject({
          ...source,
          claim_id: text(source.claim_id || source.id || source.key),
          claim_category: text(source.category || source.claim_category || claimKind),
          risk_marker: text(source.risk_marker || source.marker || source.risk)
        }),
        claim_text: claimText
      });
    });
  }
  if (Array.isArray(value)) {
    value.slice(0, 120).forEach((item, index) => collectEvidenceLedgerEntries(item, [...pathParts, String(index)], entries, depth + 1));
    return entries;
  }
  if (typeof value !== "object") return entries;
  const source = objectOf(value);
  const nextLineage = lineageFromObject(source);
  if (hasLedgerSignal(source)) {
    const fields = ledgerFieldsFromObject(source);
    const kind = ledgerKindForPath(pathParts, source);
    pushLedgerEntry(entries, {
      kind,
      source_path: sourcePath,
      support_status: supportStatusForEntry(kind, source),
      source_refs: nextLineage,
      fields
    });
  }
  for (const [key, item] of Object.entries(source)) {
    if (/^(evidence_ledger|detail_ref|audit_ref|source_refs?|evidence_refs?|citations|query_ids?|evidence_ids?|path_ids?|report_ids?|artifact_id|artifact_status)$/iu.test(key)) {
      continue;
    }
    collectEvidenceLedgerEntries(item, [...pathParts, key], entries, depth + 1);
  }
  return entries;
}

export function buildEvidenceLedger(payload = {}, options = {}) {
  const source = objectOf(payload);
  const entries = [];
  collectEvidenceLedgerEntries(rootLineageSeed(source, options), [], entries, 0);
  const unique = [];
  const seen = new Set();
  for (const entry of entries) {
    if (seen.has(entry.entry_id)) continue;
    seen.add(entry.entry_id);
    unique.push(entry);
  }
  const limitedEntries = unique.slice(0, ENTRY_OUTPUT_LIMIT);
  const coverageSummary = buildCoverageSummary(unique);
  return {
    ledger_version: EVIDENCE_LEDGER_VERSION,
    ledger_id: `evidence-ledger:${stableHash({ tool: source.tool, case_id: source.case_id, entries: unique })}`,
    case_id: text(source.case_id),
    tool: text(source.tool || source.skill_id || "analytix_funds"),
    trace_policy: "Internal only. User-visible answers must cite concrete fact fields, not q_xxx/audit_ref/artifact_id.",
    raw_payload_policy: "Raw MCP/backend payloads are not model-visible; retention is authoritative only after host evidence-registry confirmation.",
    host_registry_verified: false,
    publication_ready: false,
    fact_answer_allowed: false,
    scanned_entry_count: unique.length,
    output_entry_limit: ENTRY_OUTPUT_LIMIT,
    entry_count: limitedEntries.length,
    overflow_count: Math.max(0, unique.length - limitedEntries.length),
    coverage_summary: coverageSummary,
    entries: limitedEntries
  };
}

export function validateEvidenceLedgerCoverage(ledger = {}) {
  const source = objectOf(ledger);
  const summary = objectOf(source.coverage_summary);
  const hasMissingSurfaceList = Array.isArray(summary.missing_required_surfaces);
  const missingRequiredSurfaces = arrayOf(summary.missing_required_surfaces).map(text).filter(Boolean);
  const untraceableCount = nonNegativeInteger(summary.untraceable_required_surface_entry_count);
  const entriesWithSourceRefs = nonNegativeInteger(summary.entries_with_source_refs);
  const scannedEntryCount = nonNegativeInteger(
    Object.prototype.hasOwnProperty.call(source, "scanned_entry_count")
      ? source.scanned_entry_count
      : summary.coverage_entry_count
  );
  const failures = [
    text(source.ledger_version) !== EVIDENCE_LEDGER_VERSION
      ? `ledger_version ${text(source.ledger_version) || "<missing>"} != ${EVIDENCE_LEDGER_VERSION}`
      : "",
    !text(source.ledger_id) ? "missing ledger_id" : "",
    scannedEntryCount === undefined ? "invalid or missing scanned_entry_count" : "",
    scannedEntryCount !== undefined && scannedEntryCount <= 0 ? "no scanned ledger entries" : "",
    !hasMissingSurfaceList ? "invalid or missing missing_required_surfaces" : "",
    missingRequiredSurfaces.length ? `missing required surfaces: ${missingRequiredSurfaces.join(", ")}` : "",
    untraceableCount === undefined ? "invalid or missing untraceable_required_surface_entry_count" : "",
    untraceableCount !== undefined && untraceableCount > 0 ? `untraceable required surface entries: ${untraceableCount}` : "",
    entriesWithSourceRefs === undefined ? "invalid or missing entries_with_source_refs" : "",
    entriesWithSourceRefs !== undefined && entriesWithSourceRefs <= 0 ? "no entries carry source_refs" : ""
  ].filter(Boolean);
  return {
    ok: failures.length === 0,
    coverage_ok: failures.length === 0,
    host_registry_verified: false,
    publication_ready: false,
    fact_answer_allowed: false,
    failures,
    summary: {
      ledger_id: text(source.ledger_id),
      scanned_entry_count: scannedEntryCount,
      required_surfaces: arrayOf(summary.required_surfaces),
      missing_required_surfaces: missingRequiredSurfaces,
      entries_with_source_refs: entriesWithSourceRefs,
      untraceable_required_surface_entry_count: untraceableCount,
      untraceable_required_surface_entries: arrayOf(summary.untraceable_required_surface_entries)
    }
  };
}

export function attachInternalEvidenceLedger(payload = {}, options = {}) {
  const source = objectOf(payload);
  if (!Object.keys(source).length || source.evidence_ledger) return source;
  return {
    ...source,
    evidence_ledger: buildEvidenceLedger(source, options)
  };
}
