export interface TxnDetailCursorPageResult<Row> {
  rows: Row[];
  done: boolean;
  nextCursor: Record<string, unknown> | null;
}

export interface TxnDetailCursorPageResultOptions<Row> {
  inferDoneFromCursor?: boolean;
  limit?: number;
  mapRow?: (row: unknown) => Row;
  raw: unknown;
}

export interface TxnDetailOffsetPageRequest {
  pageSize: number;
  offset: number;
  knownTotal?: number;
}

export interface TxnDetailOffsetPageRequestOptions {
  pageSize: number | undefined;
  offset?: number;
  knownTotal?: number;
  fallbackPageSize: number;
}

function asRecord(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return {};
  }
  return value as Record<string, unknown>;
}

function asOptionalRecord(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return null;
  }
  return value as Record<string, unknown>;
}

function toPositiveInteger(value: number | undefined, fallback: number): number {
  const numberValue = Math.floor(Number(value || 0));
  const fallbackValue = Math.floor(Number(fallback || 0));
  return Number.isFinite(numberValue) && numberValue > 0
    ? numberValue
    : Number.isFinite(fallbackValue) && fallbackValue > 0
      ? fallbackValue
      : 1;
}

function toNonNegativeInteger(value: number | undefined, fallback = 0): number {
  const numberValue = Math.floor(Number(value ?? fallback));
  return Number.isFinite(numberValue) && numberValue >= 0 ? numberValue : Math.max(0, Math.floor(fallback));
}

export function normalizeTxnDetailCursorPageResult<Row = unknown>({
  inferDoneFromCursor = false,
  limit = 0,
  mapRow = (row) => row as Row,
  raw
}: TxnDetailCursorPageResultOptions<Row>): TxnDetailCursorPageResult<Row> {
  const payload = asRecord(raw);
  const rows = Array.isArray(payload.rows) ? payload.rows.map((row) => mapRow(row)) : [];
  const nextCursor = asOptionalRecord(payload.nextCursor) || asOptionalRecord(payload.next_cursor);
  const done = typeof payload.done === "boolean"
    ? payload.done
    : inferDoneFromCursor
      ? !nextCursor || (limit > 0 && rows.length < limit)
      : false;
  return {
    rows,
    done,
    nextCursor
  };
}

export function buildTxnDetailOffsetPageRequest({
  pageSize,
  offset = 0,
  knownTotal,
  fallbackPageSize
}: TxnDetailOffsetPageRequestOptions): TxnDetailOffsetPageRequest {
  const request: TxnDetailOffsetPageRequest = {
    pageSize: toPositiveInteger(pageSize, fallbackPageSize),
    offset: toNonNegativeInteger(offset)
  };
  if (typeof knownTotal === "number" && Number.isFinite(knownTotal) && knownTotal >= 0) {
    request.knownTotal = Math.floor(knownTotal);
  }
  return request;
}
