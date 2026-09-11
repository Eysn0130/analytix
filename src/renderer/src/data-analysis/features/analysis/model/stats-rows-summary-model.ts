import {
  readFiniteCaseFactNumber,
  readNonNegativeCaseFactInteger,
} from "./stats-case-fact-number-model";

export interface StatsRowsSummary {
  totalAmount: number | null;
  totalCount: number | null;
}

export function normalizeStatsRowsSummary(raw: unknown): StatsRowsSummary {
  const source = raw && typeof raw === "object" && !Array.isArray(raw) ? (raw as Record<string, unknown>) : {};
  return {
    totalAmount: readFiniteCaseFactNumber(source.total_amount),
    totalCount: readNonNegativeCaseFactInteger(source.total_count),
  };
}
