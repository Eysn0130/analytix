import type { StatsRowDTO } from "../api/stats-api";

type FlowTransferView = "relation" | "net";

function toAmount(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) ? value : null;
}

export function resolveStatsFlowExpectedRowAmount(row: StatsRowDTO, view: FlowTransferView): number | null {
  if (view === "net") {
    return null;
  }
  return toAmount(row.total_amount);
}

export function resolveStatsFlowExpectedTotalAmount(rows: StatsRowDTO[], view: FlowTransferView): number | null {
  if (view === "net") {
    return null;
  }
  if (!Array.isArray(rows) || rows.length === 0) {
    return null;
  }
  let total = 0;
  for (const row of rows) {
    const amount = toAmount(row.total_amount);
    if (amount == null) {
      return null;
    }
    total += amount;
  }
  return total;
}
