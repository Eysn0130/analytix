import type { TxnDetailDialogColumn } from "./TxnDetailDialog";
import txnDetailColumnSpecs from "./txn-detail-column-specs.json";

export type TxnDetailSortColumnKey = "txn_time" | "amount";
export type TxnDetailColumnKind = "text" | "money" | "direction";

export interface TxnDetailDialogColumnSpec {
  key: string;
  title: string;
  sortable?: string;
  width: number;
  kind?: TxnDetailColumnKind;
  mono?: boolean;
}

export interface TxnDetailColumnSpec {
  key: string;
  title: string;
  sortable?: TxnDetailSortColumnKey;
  width: number;
  kind?: TxnDetailColumnKind;
  mono?: boolean;
}

export const TXN_DETAIL_COLUMN_SPECS = txnDetailColumnSpecs as readonly TxnDetailColumnSpec[];

export const TXN_DETAIL_EXPORT_ROW_KEYS = TXN_DETAIL_COLUMN_SPECS.map((column) => column.key);
export const TXN_DETAIL_ROW_FIELDS = ["id", ...TXN_DETAIL_EXPORT_ROW_KEYS];
export const TXN_DETAIL_MONEY_COLUMN_KEYS = new Set(
  TXN_DETAIL_COLUMN_SPECS.filter((column) => column.kind === "money").map((column) => column.key)
);
export const TXN_DETAIL_MONO_COLUMN_KEYS = new Set(
  TXN_DETAIL_COLUMN_SPECS.filter((column) => column.mono).map((column) => column.key)
);

function toFiniteNumber(value: unknown): number {
  const next = Number(value);
  return Number.isFinite(next) ? next : 0;
}

export function formatTxnDetailMoney(value: unknown): string {
  return toFiniteNumber(value).toLocaleString("zh-CN", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
}

export function buildTxnDetailDialogColumns({
  resizable = false,
  sortCol = "",
  sortDir = "asc",
  specs
}: {
  resizable?: boolean;
  sortCol?: string;
  sortDir?: "asc" | "desc";
  specs: readonly TxnDetailDialogColumnSpec[];
}): TxnDetailDialogColumn[] {
  return specs.map((column) => {
    const sortableKey = String(column.sortable || "");
    const sortable = sortableKey.length > 0;
    const sorted = sortable && sortCol === sortableKey;
    return {
      key: column.key,
      title: column.title,
      width: column.width,
      sortable,
      sorted,
      sortDir: sorted ? sortDir : "asc",
      ...(resizable ? { resizable: true } : {})
    };
  });
}

export function buildTxnDetailColumns({
  resizable = false,
  sortCol,
  sortDir
}: {
  resizable?: boolean;
  sortCol: TxnDetailSortColumnKey;
  sortDir: "asc" | "desc";
}): TxnDetailDialogColumn[] {
  return buildTxnDetailDialogColumns({
    resizable,
    sortCol,
    sortDir,
    specs: TXN_DETAIL_COLUMN_SPECS
  });
}
