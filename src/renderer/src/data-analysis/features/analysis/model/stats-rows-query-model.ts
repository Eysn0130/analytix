import type { StatsRowDTO } from "../api/stats-api";

export const STATS_ROWS_DIRECT_ROW_FIELDS = [
  "id",
  "counterparty_account",
  "counterparty_name",
  "relation",
  "location",
  "bank",
  "doc",
  "total_amount",
  "total_count",
  "net_in",
  "net_out",
  "in_amount",
  "in_count",
  "out_amount",
  "out_count",
  "first_time",
  "last_time",
  "keyType",
  "keyValue",
  "keyLabel",
  "drillKeyType",
  "drillKeyValue",
  "placeholderKind",
];

export interface StatsRowsResultProjection {
  factAnswerAllowed: boolean;
  semanticStatus: string;
  blocker: string;
  rows: StatsRowDTO[];
  rowSummary: Record<string, unknown>;
  status: string;
  total: number | null;
}

export function normalizeStatsRowsResult(raw: unknown): StatsRowsResultProjection {
  void raw;
  return {
    factAnswerAllowed: false,
    semanticStatus: "blocked",
    blocker: "host_evidence_receipt_required",
    rows: [],
    rowSummary: {},
    status: "controlled_projection_required",
    total: null,
  };
}
