import {
  arrayOf,
  pruneEmpty,
  text
} from "./runtime-normalizers.mjs";
import { abortError, throwIfAborted } from "./abort-runtime.mjs";
import { fixedFundsScopeUnavailableBoundary } from "./source-unavailable-boundary.mjs";

export const CASE_SCOPE_MAP_RUNTIME_VERSION = "0.14.3";

function unverifiedBoundary() {
  return {
    coverage_complete: false,
    safe_for_case_conclusion: false,
    zero_result_verified: false,
    fact_answer_allowed: false,
    publication_readiness: "blocked",
    host_registry_verified: false,
    blocker: "dataset_snapshot_authority_unavailable"
  };
}

function unresolvedSummary() {
  return {
    status: "unresolved",
    complete: false,
    ...unverifiedBoundary()
  };
}

export function scopeMapNode(id, label, kind, metrics = {}, status = "unknown", notes = []) {
  return {
    id,
    label,
    kind,
    status: text(status) || "unknown",
    metrics,
    notes: arrayOf(notes).map(text).filter(Boolean)
  };
}

export function scopeMapEdge(from, to, relation, guard = "") {
  return {
    from,
    to,
    relation,
    guard: text(guard)
  };
}

export async function withTimeout(operation, timeoutMs, label, parentSignal) {
  throwIfAborted(parentSignal);
  const controller = new AbortController();
  let timer;
  let onParentAbort;
  const deadline = new Promise((_, reject) => {
    onParentAbort = () => {
      const error = abortError(parentSignal);
      controller.abort(error);
      reject(error);
    };
    parentSignal?.addEventListener("abort", onParentAbort, { once: true });
    timer = setTimeout(() => {
      const error = new Error(`${label} timed out after ${timeoutMs}ms`);
      controller.abort(error);
      reject(error);
    }, timeoutMs);
  });
  try {
    return await Promise.race([
      Promise.resolve().then(() => (typeof operation === "function" ? operation(controller.signal) : operation)),
      deadline
    ]);
  } finally {
    clearTimeout(timer);
    parentSignal?.removeEventListener("abort", onParentAbort);
  }
}

export function sleep(ms, signal) {
  throwIfAborted(signal);
  return new Promise((resolve, reject) => {
    let settled = false;
    const finish = (operation) => {
      if (settled) return;
      settled = true;
      signal?.removeEventListener("abort", onAbort);
      operation();
    };
    const timer = setTimeout(() => finish(resolve), ms);
    const onAbort = () => {
      clearTimeout(timer);
      finish(() => reject(abortError(signal)));
    };
    signal?.addEventListener("abort", onAbort, { once: true });
    if (signal?.aborted) onAbort();
  });
}

export function isTransientBackendError(error) {
  const message = text(error instanceof Error ? error.message : error);
  return /unexpected server error|temporar|timeout|timed out|ECONNRESET|ECONNREFUSED|EAI_AGAIN|socket hang up/iu.test(message);
}

export function stableChildErrorMessage(label, error) {
  void label;
  void error;
  return "案件事实来源不可用；仅允许说明能力缺口和补证动作，不得使用任何子结果继续研判。";
}

export function summarizeScopeMapSchema(schemaData) {
  void schemaData;
  return unresolvedSummary();
}

export function summarizeScopeMapQuality(dataQualityData) {
  void dataQualityData;
  return unresolvedSummary();
}

export function summarizeScopeMapCompare(scopeCompareData) {
  void scopeCompareData;
  return unresolvedSummary();
}

export function compactScopeMapPipelineDetails(pipeline) {
  void pipeline;
  return unresolvedSummary();
}

export function compactScopeMapChildDetails(details) {
  void details;
  return {
    compact: true,
    omitted_raw_child_payloads: true,
    ...unverifiedBoundary()
  };
}

export function scopeMapGateStatus(children) {
  void children;
  const reason = "Go 宿主 registry 尚无同案、同轮、同 epoch、同快照 EvidenceReceipt。";
  return [
    { gate: "source_audit", tool: "audit_unindexed_sources", status: "needs_review", reason },
    { gate: "data_quality", tool: "audit_case_data_quality", status: "needs_review", reason },
    { gate: "duplicate_family_policy", tool: "resolve_duplicate_families", status: "needs_review", reason },
    { gate: "reconciliation", tool: "get_case_reconciliation", status: "needs_review", reason },
    { gate: "coverage", tool: "get_scope_coverage", status: "needs_review", reason }
  ];
}

export function createCaseScopeMapRuntime({ resolveCase, executeSkill, getCaseDataPipelineOverview }) {
  if (
    typeof resolveCase !== "function"
    || typeof executeSkill !== "function"
    || typeof getCaseDataPipelineOverview !== "function"
  ) {
    throw new Error("createCaseScopeMapRuntime requires resolveCase/executeSkill/getCaseDataPipelineOverview");
  }

  async function getCaseScopeMap(args = {}, { signal } = {}) {
    throwIfAborted(signal);
    void args;
    // P0 quarantine is deliberately before case resolution and every child
    // call. Case binding and evidence authority belong to the Go host.
    return pruneEmpty(fixedFundsScopeUnavailableBoundary(scopeMapGateStatus({}))) || {};
  }

  return { getCaseScopeMap };
}
