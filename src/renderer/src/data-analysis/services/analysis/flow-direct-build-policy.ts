const FLOW_DIRECT_BUILD_MAX_ROW_COUNT = 10_000;

export interface FlowDirectBuildFacts {
  seeds: string[];
  depth: 1;
  direction: "in" | "out" | "both";
  minAmount: number;
  focusOnly: true;
  focusUnknownName: boolean;
  includeMissingCounterparty: boolean;
  focusCounterpartyStrict: boolean;
  expectedTotalAmount: number;
  expectedRowCount: number;
}

function hasOwn(source: Record<string, unknown>, key: string): boolean {
  return Object.prototype.hasOwnProperty.call(source, key);
}

function readAliasedValue<T>(
  source: Record<string, unknown>,
  camelKey: string,
  snakeKey: string,
  reader: (value: unknown) => T | undefined,
): T | undefined {
  const values: T[] = [];
  for (const key of [camelKey, snakeKey]) {
    if (!hasOwn(source, key)) {
      continue;
    }
    const value = reader(source[key]);
    if (value === undefined) {
      return undefined;
    }
    values.push(value);
  }
  if (values.length === 0) {
    return undefined;
  }
  if (values.length === 2 && values[0] !== values[1]) {
    return undefined;
  }
  return values[0];
}

export function readStrictFiniteNumber(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

export function readStrictNonNegativeFiniteNumber(value: unknown): number | undefined {
  const number = readStrictFiniteNumber(value);
  return number !== undefined && number >= 0 ? number : undefined;
}

export function readStrictNonNegativeSafeInteger(value: unknown): number | undefined {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0 ? value : undefined;
}

export function readStrictBoolean(value: unknown): boolean | undefined {
  return typeof value === "boolean" ? value : undefined;
}

export function readStrictAliasedFiniteNumber(
  source: Record<string, unknown>,
  camelKey: string,
  snakeKey: string,
): number | undefined {
  return readAliasedValue(source, camelKey, snakeKey, readStrictFiniteNumber);
}

export function readStrictAliasedNonNegativeSafeInteger(
  source: Record<string, unknown>,
  camelKey: string,
  snakeKey: string,
): number | undefined {
  return readAliasedValue(source, camelKey, snakeKey, readStrictNonNegativeSafeInteger);
}

export function readStrictAliasedBoolean(
  source: Record<string, unknown>,
  camelKey: string,
  snakeKey: string,
): boolean | undefined {
  return readAliasedValue(source, camelKey, snakeKey, readStrictBoolean);
}

function text(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function uniqueSortedStrings(values: unknown): string[] {
  if (!Array.isArray(values)) {
    return [];
  }
  return Array.from(
    new Set(values.filter((item): item is string => typeof item === "string").map((item) => item.trim()).filter(Boolean)),
  ).sort();
}

function readRequiredStringArray(value: unknown): string[] | null {
  if (
    !Array.isArray(value) ||
    value.length === 0 ||
    value.some((item) => typeof item !== "string" || !item.trim())
  ) {
    return null;
  }
  return uniqueSortedStrings(value);
}

function asObject(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : {};
}

export function readFlowDirectBuildFacts(payload: Record<string, unknown>): FlowDirectBuildFacts | null {
  if (text(payload.source).toLowerCase() !== "stats") {
    return null;
  }
  const seeds = readRequiredStringArray(payload.seeds);
  if (!seeds) {
    return null;
  }
  const depth = readStrictNonNegativeSafeInteger(payload.depth);
  const direction = payload.direction;
  const minAmount = readStrictNonNegativeFiniteNumber(payload.min_amount);
  const focusOnly = readStrictBoolean(payload.focus_only);
  const focusUnknownName = readStrictBoolean(payload.focus_unknown_name);
  const includeMissingCounterparty = readStrictBoolean(payload.include_missing_counterparty);
  const focusCounterpartyStrict = readStrictBoolean(payload.focus_counterparty_strict);
  const expectedTotalAmount = readStrictFiniteNumber(payload.expected_total_amount);
  const expectedRowCount = readStrictNonNegativeSafeInteger(payload.expected_row_count);

  if (
    depth !== 1 ||
    (direction !== "in" && direction !== "out" && direction !== "both") ||
    minAmount === undefined ||
    focusOnly !== true ||
    focusUnknownName === undefined ||
    includeMissingCounterparty === undefined ||
    focusCounterpartyStrict === undefined ||
    expectedTotalAmount === undefined ||
    expectedRowCount === undefined ||
    expectedRowCount > FLOW_DIRECT_BUILD_MAX_ROW_COUNT
  ) {
    return null;
  }

  return {
    seeds,
    depth,
    direction,
    minAmount,
    focusOnly,
    focusUnknownName,
    includeMissingCounterparty,
    focusCounterpartyStrict,
    expectedTotalAmount,
    expectedRowCount,
  };
}

export function buildFlowDirectBuildCacheFields(payload: Record<string, unknown>): Record<string, unknown> | null {
  const facts = readFlowDirectBuildFacts(payload);
  if (!facts) {
    return null;
  }
  const graph = asObject(payload.graph);
  return {
    seeds: facts.seeds,
    left_seeds: uniqueSortedStrings(payload.left_seeds),
    depth: facts.depth,
    direction: facts.direction,
    min_amount: facts.minAmount,
    source: "stats",
    view: text(payload.view),
    layout: text(payload.layout),
    date_start: text(payload.date_start),
    date_end: text(payload.date_end),
    focus_id: text(payload.focus_id),
    focus_name: text(payload.focus_name),
    focus_key_type: text(payload.focus_key_type),
    focus_label: text(payload.focus_label),
    focus_only: facts.focusOnly,
    focus_unknown_name: facts.focusUnknownName,
    include_missing_counterparty: facts.includeMissingCounterparty,
    focus_counterparty_strict: facts.focusCounterpartyStrict,
    expected_total_amount: facts.expectedTotalAmount,
    expected_row_count: facts.expectedRowCount,
    focus_ids: uniqueSortedStrings(payload.focus_ids),
    focus_names: uniqueSortedStrings(payload.focus_names),
    focus_placeholder_kinds: uniqueSortedStrings(payload.focus_placeholder_kinds),
    graph_render_mode: text(graph.render_mode ?? graph.renderMode) || "auto",
  };
}
