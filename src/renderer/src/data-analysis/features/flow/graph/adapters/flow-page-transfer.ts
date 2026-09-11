import {
  readStrictAliasedBoolean,
  readStrictAliasedFiniteNumber,
  readStrictAliasedNonNegativeSafeInteger,
} from "../../../../services/analysis/flow-direct-build-policy";

export interface FlowRequestPayload extends Record<string, unknown> {}

export const FLOW_TRANSFER_STORAGE_KEY = "analytix:flow:transfer";

let stagedFlowTransfer: FlowRequestPayload | null = null;

export function text(value: unknown): string {
  return String(value == null ? "" : value).trim();
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : {};
}

function asArray(value: unknown): unknown[] {
  if (Array.isArray(value)) {
    return value;
  }
  if (typeof value === "string" && value.trim()) {
    return [value];
  }
  return [];
}

function uniqueStrings(value: unknown): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  asArray(value).forEach((item) => {
    const next = text(item);
    if (!next || seen.has(next)) {
      return;
    }
    seen.add(next);
    out.push(next);
  });
  return out;
}

function assignAliasedValue(
  target: FlowRequestPayload,
  camelKey: string,
  snakeKey: string,
  value: number | boolean | undefined,
): void {
  delete target[camelKey];
  delete target[snakeKey];
  if (value !== undefined) {
    target[camelKey] = value;
    target[snakeKey] = value;
  }
}

export function normalizeFlowTransferPayload(payload: FlowRequestPayload): FlowRequestPayload {
  const source = asRecord(payload);
  const seeds = uniqueStrings(source.seeds ?? source.selectedSeeds ?? source.selected_seeds);
  const focusIds = uniqueStrings(source.focusIds ?? source.focus_ids);
  const focusNames = uniqueStrings(source.focusNames ?? source.focus_names);
  const leftSeeds = uniqueStrings(source.leftSeeds ?? source.left_seeds);
  const focusPlaceholderKinds = uniqueStrings(source.focusPlaceholderKinds ?? source.focus_placeholder_kinds);
  const expectedTotalAmount = readStrictAliasedFiniteNumber(
    source,
    "expectedTotalAmount",
    "expected_total_amount",
  );
  const expectedRowCount = readStrictAliasedNonNegativeSafeInteger(
    source,
    "expectedRowCount",
    "expected_row_count",
  );
  const normalized: FlowRequestPayload = {
    ...source,
    caseId: text(source.caseId ?? source.case_id),
    case_id: text(source.case_id ?? source.caseId),
    seeds,
    selectedSeeds: seeds,
    selected_seeds: seeds,
    leftSeeds,
    left_seeds: leftSeeds,
    focusId: text(source.focusId ?? source.focus_id),
    focus_id: text(source.focus_id ?? source.focusId),
    focusName: text(source.focusName ?? source.focus_name),
    focus_name: text(source.focus_name ?? source.focusName),
    focusLabel: text(source.focusLabel ?? source.focus_label),
    focus_label: text(source.focus_label ?? source.focusLabel),
    focusKeyType: text(source.focusKeyType ?? source.focus_key_type ?? source.keyType).toLowerCase(),
    focus_key_type: text(source.focus_key_type ?? source.focusKeyType ?? source.keyType).toLowerCase(),
    focusIds,
    focus_ids: focusIds,
    focusNames,
    focus_names: focusNames,
    focusPlaceholderKinds,
    focus_placeholder_kinds: focusPlaceholderKinds,
    dateStart: text(source.dateStart ?? source.date_start),
    date_start: text(source.date_start ?? source.dateStart),
    dateEnd: text(source.dateEnd ?? source.date_end),
    date_end: text(source.date_end ?? source.dateEnd),
    requestId: text(source.requestId ?? source.request_id),
    request_id: text(source.request_id ?? source.requestId),
  };
  assignAliasedValue(
    normalized,
    "focusOnly",
    "focus_only",
    readStrictAliasedBoolean(source, "focusOnly", "focus_only"),
  );
  assignAliasedValue(
    normalized,
    "focusUnknownName",
    "focus_unknown_name",
    readStrictAliasedBoolean(source, "focusUnknownName", "focus_unknown_name"),
  );
  assignAliasedValue(
    normalized,
    "focusCounterpartyStrict",
    "focus_counterparty_strict",
    readStrictAliasedBoolean(source, "focusCounterpartyStrict", "focus_counterparty_strict"),
  );
  assignAliasedValue(
    normalized,
    "includeMissingCounterparty",
    "include_missing_counterparty",
    readStrictAliasedBoolean(source, "includeMissingCounterparty", "include_missing_counterparty"),
  );
  assignAliasedValue(normalized, "expectedTotalAmount", "expected_total_amount", expectedTotalAmount);
  assignAliasedValue(normalized, "expectedRowCount", "expected_row_count", expectedRowCount);
  return normalized;
}

function clearLegacyPersistedFlowTransfer(): void {
  if (typeof localStorage === "undefined") {
    return;
  }
  try {
    localStorage.removeItem(FLOW_TRANSFER_STORAGE_KEY);
  } catch {
    // The in-memory handoff remains authoritative when storage is unavailable.
  }
}

export function stageFlowTransferPayload(payload: FlowRequestPayload): void {
  clearLegacyPersistedFlowTransfer();
  stagedFlowTransfer = normalizeFlowTransferPayload(payload);
}

export function readFlowTransferPayload(): FlowRequestPayload | null {
  clearLegacyPersistedFlowTransfer();
  return stagedFlowTransfer ? normalizeFlowTransferPayload(stagedFlowTransfer) : null;
}

export function clearStagedFlowTransferPayload(): void {
  stagedFlowTransfer = null;
  clearLegacyPersistedFlowTransfer();
}

export function hasStagedFlowTransferPayload(): boolean {
  clearLegacyPersistedFlowTransfer();
  return stagedFlowTransfer !== null;
}
