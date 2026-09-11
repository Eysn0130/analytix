export type StatsTreeTab = "byName" | "byCard";
export type StatsMode = "inAccount" | "outAccount" | "inName" | "outName";
export type StatsKeyType = "account" | "name";
export type StatsTxnFilter = "all" | "in" | "out";
export type StatsTxnSortCol = "txn_time" | "amount";
export type StatsWorkspaceTab = "stats" | "charts";
export type ChartMetricMode = "amount" | "count" | "counterparty";
export type ChartDirectionMode = "all" | "in" | "out" | "net";
export type ChartGranularity = "day" | "week" | "month" | "hour";
export type ChartSuccessFilter = "all" | "success" | "failed";
export type ChartCashFilter = "all" | "cash" | "non-cash";

export const LEGACY_STATS_EVIDENCE_BLOCKER = "host_evidence_receipt_required";

export interface StatsV2MetaDTO {
  contract: "StatsMetaPublicBoundaryV1";
  semantic_status: "source_unavailable" | "blocked";
  fact_answer_allowed: false;
  blocker: string;
  case_id: string;
  funds_status: string;
  date_min: string;
  date_max: string;
}

export interface StatsTreeItemDTO {
  id: string;
  title: string;
  sub: string;
}

export interface StatsTreeGroupDTO {
  id: string;
  title: string;
  meta: string;
  extra: string;
  items: StatsTreeItemDTO[];
}

export interface StatsV2TreeDTO {
  contract: "StatsTreePublicBoundaryV1";
  semantic_status: "source_unavailable" | "blocked";
  fact_answer_allowed: false;
  blocker: string;
  groups: StatsTreeGroupDTO[];
}

export interface StatsRowDTO {
  id: string;
  counterparty_account: string;
  counterparty_name: string;
  relation: string;
  location: string;
  bank: string;
  doc: string;
  total_amount: number;
  total_count: number;
  net_in: number;
  net_out: number;
  in_amount: number;
  in_count: number;
  out_amount: number;
  out_count: number;
  first_time: string;
  last_time: string;
  keyType: StatsKeyType;
  keyValue: string;
  keyLabel?: string;
  drillKeyType?: StatsKeyType;
  drillKeyValue?: string;
  placeholderKind?: string;
}

export interface StatsTxnRowDTO {
  id: number | string;
  card_no: string;
  acct_no: string;
  account_open_name: string;
  opener_id_no: string;
  txn_time: string;
  amount: number | "";
  balance: number | "";
  dc_flag: string;
  counterparty_acct: string;
  cash_flag: string;
  counterparty_name: string;
  counterparty_id_no: string;
  counterparty_bank: string;
  summary: string;
  currency: string;
  branch_name: string;
  location: string;
  is_success: string;
  voucher_no: string;
  ip_addr: string;
  mac_addr: string;
  counterparty_balance: number | "";
  txn_id: string;
  log_id: string;
  voucher_type: string;
  voucher_id: string;
  teller_no: string;
  remark: string;
  txn_type: string;
  query_feedback_reason: string;
}

export interface StatsV2RowsDTO {
  contract: "StatsRowsPublicBoundaryV1";
  semantic_status: "blocked";
  blocker: string;
  fact_answer_allowed: false;
  raw_details_exposed: false;
  rows: unknown[];
  row_fields?: string[];
  status?: string;
  total?: number | null;
  row_summary?: {
    total_amount?: number;
    total_count?: number;
  };
}

export interface StatsV2TxnRowsDTO {
  contract: "StatsTxnRowsPublicBoundaryV1";
  semantic_status: "blocked";
  blocker: string;
  fact_answer_allowed: false;
  raw_details_exposed: false;
  rows: unknown[];
  row_fields?: string[];
  done?: boolean;
  next_cursor?: Record<string, unknown> | null;
}

export interface ChartFilterToken {
  source_panel_id: string;
  dimension: string;
  value: string;
  label: string;
  payload?: Record<string, unknown>;
}

export interface ChartPanelViewState {
  panel_id: string;
  view?: string;
  metric_basis?: string;
  dimension?: string;
  extra?: Record<string, unknown>;
}

export interface StatsV2ChartDashboardDTO {
  evidence_status: "verified" | "verified_no_hit" | "partial" | "source_unavailable" | "needs_selection" | "blocked";
  answer_card_complete: boolean;
  fact_answer_allowed: boolean;
  blocker: string;
  coverage: {
    total_rows: number | null;
    amount_present_rows: number | null;
    amount_missing_rows: number | null;
    amount_parse_failed_rows: number | null;
    direction_covered_rows: number | null;
  };
  selection_mode: "single-card" | "multi-card";
  object_summary: Record<string, unknown>;
  summary: Record<string, unknown>;
  trend: Record<string, unknown>;
  counterparties: Record<string, unknown>;
  structure: Record<string, unknown>;
  heatmap: Record<string, unknown>;
  distribution: Record<string, unknown>;
  flow: Record<string, unknown>;
  anomaly: Record<string, unknown>;
}

export interface StatsV2ChartDetailRowsDTO {
  evidence_status: "verified" | "verified_no_hit" | "partial" | "source_unavailable" | "needs_selection" | "blocked";
  answer_card_complete: boolean;
  fact_answer_allowed: boolean;
  blocker: string;
  selection_mode: "single-card" | "multi-card";
  rows: StatsTxnRowDTO[];
  total: number | null;
}

export async function queryStatsV2Tree(payload?: {
  case_id: string;
  tab: StatsTreeTab;
}): Promise<StatsV2TreeDTO> {
  void payload;
  return {
    contract: "StatsTreePublicBoundaryV1",
    semantic_status: "source_unavailable",
    fact_answer_allowed: false,
    blocker: LEGACY_STATS_EVIDENCE_BLOCKER,
    groups: [],
  };
}
