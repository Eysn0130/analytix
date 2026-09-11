import type {
  StatsKeyType,
  StatsRowDTO,
  StatsTxnFilter,
  StatsTxnRowDTO,
  StatsTxnSortCol,
} from "../api/stats-api";

export const STATS_TXN_ROWS_PAGE_LIMIT = 200;
export const STATS_TXN_ROWS_FIRST_PAGE_LIMIT = 120;
export const STATS_TXN_ROWS_NEXT_PAGE_LIMIT = 240;

export interface StatsTxnActiveRowKey {
  keyType: StatsKeyType;
  keyValue: string;
}

export interface StatsTxnRowsQueryRequest {
  case_id: string;
  selected: string[];
  date_start: string;
  date_end: string;
  key_type: StatsKeyType;
  key_value: string;
  filter: StatsTxnFilter;
  sort_col: StatsTxnSortCol;
  sort_dir: "asc" | "desc";
  limit: number;
  cursor: Record<string, unknown> | null;
  row_format?: "object" | "array";
  fields?: string[];
}

export interface StatsTxnRowsResultProjection {
  factAnswerAllowed: boolean;
  semanticStatus: string;
  blocker: string;
  rows: StatsTxnRowDTO[];
  done: boolean;
  nextCursor: Record<string, unknown> | null;
}

export function resolveStatsTxnActiveRowKey(activeRow: StatsRowDTO | null | undefined): StatsTxnActiveRowKey | null {
  if (!activeRow) {
    return null;
  }
  return {
    keyType: activeRow.drillKeyType || activeRow.keyType || "account",
    keyValue: activeRow.drillKeyValue || activeRow.keyValue || "",
  };
}

export function buildStatsTxnRowsQueryRequest(params: {
  caseId: string;
  selectedAccounts: string[];
  dateStart: string;
  dateEnd: string;
  activeRow: StatsRowDTO | null;
  txnFilter: StatsTxnFilter;
  txnSortCol: StatsTxnSortCol;
  txnSortDir: "asc" | "desc";
  cursor: Record<string, unknown> | null;
  limit?: number;
  rowFormat?: "object" | "array";
  fields?: string[];
}): StatsTxnRowsQueryRequest | null {
  const caseId = String(params.caseId || "").trim();
  const activeRowKey = resolveStatsTxnActiveRowKey(params.activeRow);
  if (!caseId || !activeRowKey) {
    return null;
  }
  return {
    case_id: caseId,
    selected: params.selectedAccounts,
    date_start: params.dateStart,
    date_end: params.dateEnd,
    key_type: activeRowKey.keyType,
    key_value: activeRowKey.keyValue,
    filter: params.txnFilter,
    sort_col: params.txnSortCol,
    sort_dir: params.txnSortDir,
    limit: params.limit ?? STATS_TXN_ROWS_PAGE_LIMIT,
    cursor: params.cursor,
    row_format: params.rowFormat,
    fields: Array.isArray(params.fields) ? params.fields : [],
  };
}

export function buildStatsTxnRowsRequestKey(request: StatsTxnRowsQueryRequest): string {
  return JSON.stringify({
    case_id: request.case_id,
    selected: Array.isArray(request.selected) ? request.selected : [],
    date_start: request.date_start,
    date_end: request.date_end,
    key_type: request.key_type,
    key_value: request.key_value,
    filter: request.filter,
    sort_col: request.sort_col,
    sort_dir: request.sort_dir,
    limit: request.limit,
    cursor: request.cursor || null,
    row_format: request.row_format || "object",
    fields: Array.isArray(request.fields) ? request.fields : [],
  });
}

export function resolveStatsTxnRowsPageLimit(params: {
  firstPageLimit?: number;
  nextPageLimit?: number;
  reset: boolean;
}): number {
  return toPositiveInteger(
    params.reset ? params.firstPageLimit : params.nextPageLimit,
    params.reset ? STATS_TXN_ROWS_FIRST_PAGE_LIMIT : STATS_TXN_ROWS_NEXT_PAGE_LIMIT
  );
}

export function normalizeStatsTxnRowsResult(raw: unknown): StatsTxnRowsResultProjection {
  void raw;
  return {
    factAnswerAllowed: false,
    semanticStatus: "blocked",
    blocker: "host_evidence_receipt_required",
    rows: [],
    done: false,
    nextCursor: null,
  };
}

function toPositiveInteger(value: number | undefined, fallback: number): number {
  const numberValue = Math.floor(Number(value || 0));
  const fallbackValue = Math.floor(Number(fallback || 0));
  if (Number.isFinite(numberValue) && numberValue > 0) {
    return numberValue;
  }
  return Number.isFinite(fallbackValue) && fallbackValue > 0 ? fallbackValue : 1;
}
