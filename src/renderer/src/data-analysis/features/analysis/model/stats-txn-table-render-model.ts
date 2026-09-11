import type {
  TxnDetailDialogColumn,
  TxnDetailDialogRow
} from "../../../components/txn-detail/TxnDetailDialog";
import {
  buildTxnDetailColumns,
  formatTxnDetailMoney,
  TXN_DETAIL_COLUMN_SPECS,
  TXN_DETAIL_EXPORT_ROW_KEYS,
  TXN_DETAIL_ROW_FIELDS
} from "../../../components/txn-detail/txn-detail-column-model";
import { buildTxnDetailCell, type TxnDetailRenderCell } from "../../../components/txn-detail/txn-detail-cell-model";
import type { StatsTxnRowDTO, StatsTxnSortCol } from "../api/stats-api";

export type StatsTxnTableColumn = {
  key: keyof StatsTxnRowDTO;
  title: string;
  sortable?: StatsTxnSortCol;
  width: number;
  kind?: "text" | "money" | "direction";
};

export type StatsTxnTableRenderColumn = TxnDetailDialogColumn;

export type StatsTxnTableRenderCell = TxnDetailRenderCell;

export interface StatsTxnTableRenderRow extends TxnDetailDialogRow {
  key: string;
  className: string;
  cells: Record<string, StatsTxnTableRenderCell>;
}

export const STATS_TXN_TABLE_COLUMNS: StatsTxnTableColumn[] = TXN_DETAIL_COLUMN_SPECS.map((column) => ({
  ...column,
  key: column.key as keyof StatsTxnRowDTO,
  sortable: column.sortable as StatsTxnSortCol | undefined
}));

export const STATS_TXN_EXPORT_ROW_KEYS = [...TXN_DETAIL_EXPORT_ROW_KEYS];
export const STATS_TXN_DETAIL_ROW_FIELDS = [...TXN_DETAIL_ROW_FIELDS];

export function formatStatsTxnTableMoney(value: unknown): string {
  return formatTxnDetailMoney(value);
}

export function buildStatsTxnModalColumns(
  txnSortCol: StatsTxnSortCol,
  txnSortDir: "asc" | "desc"
): StatsTxnTableRenderColumn[] {
  return buildTxnDetailColumns({ sortCol: txnSortCol, sortDir: txnSortDir });
}

export function buildStatsTxnModalRows(
  rows: StatsTxnRowDTO[],
  windowStart: number
): StatsTxnTableRenderRow[] {
  return rows.map((row, index) => {
    const absoluteIndex = windowStart + index;
    const stableRowIdentity = row.id === undefined || row.id === null || row.id === "" ? row.txn_id : row.id;
    const rowIdentity = stableRowIdentity || `${row.txn_time || ""}:${row.amount || ""}`;
    const key = `${rowIdentity}:${absoluteIndex}`;
    return {
      key,
      className: absoluteIndex % 2 === 0 ? "txnRow even" : "txnRow",
      cells: STATS_TXN_TABLE_COLUMNS.reduce<Record<string, StatsTxnTableRenderCell>>((accumulator, column) => {
        const raw = row[column.key];
        accumulator[String(column.key)] = buildTxnDetailCell({
          column,
          raw,
          formatMoney: formatStatsTxnTableMoney
        });
        return accumulator;
      }, {}),
    };
  });
}
