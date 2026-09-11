import type { ChartFilterToken } from "../api/stats-api";

function stableStringify(value: unknown): string {
  return JSON.stringify(value, (_key, rawValue) => {
    if (Array.isArray(rawValue)) {
      return rawValue;
    }
    if (rawValue && typeof rawValue === "object") {
      return Object.keys(rawValue as Record<string, unknown>)
        .sort()
        .reduce<Record<string, unknown>>((acc, key) => {
          acc[key] = (rawValue as Record<string, unknown>)[key];
          return acc;
        }, {});
    }
    return rawValue;
  });
}

export function normalizeChartSelectedAccounts(selectedAccounts: string[]): string[] {
  return Array.from(new Set((selectedAccounts || []).map((item) => String(item || "").trim()).filter(Boolean))).sort();
}

export function buildChartSelectedAccountsKey(selectedAccounts: string[]): string {
  return normalizeChartSelectedAccounts(selectedAccounts).join("\u001f");
}

export function normalizeChartFilterTokens(chartFilters: ChartFilterToken[]): ChartFilterToken[] {
  return (Array.isArray(chartFilters) ? chartFilters : [])
    .map((token) => ({
      ...token,
      source_panel_id: String(token.source_panel_id || "").trim(),
      dimension: String(token.dimension || "").trim(),
      value: String(token.value || "").trim(),
      label: String(token.label || "").trim(),
      payload: token.payload && typeof token.payload === "object" && !Array.isArray(token.payload) ? token.payload : {},
    }))
    .filter((token) => token.source_panel_id && token.dimension && token.value)
    .sort((left, right) =>
      `${left.source_panel_id}:${left.dimension}:${left.value}:${stableStringify(left.payload)}`.localeCompare(
        `${right.source_panel_id}:${right.dimension}:${right.value}:${stableStringify(right.payload)}`,
        "en"
      )
    );
}

export function buildChartFilterTokensKey(chartFilters: ChartFilterToken[]): string {
  return stableStringify(
    normalizeChartFilterTokens(chartFilters).map((token) => ({
      source_panel_id: token.source_panel_id,
      dimension: token.dimension,
      value: token.value,
      payload: token.payload || {},
    }))
  );
}
