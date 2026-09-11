import type { TxnDetailDialogCell, TxnDetailDialogColumn, TxnDetailDialogRow } from "./TxnDetailDialog";
import { TXN_DETAIL_MONEY_COLUMN_KEYS } from "./txn-detail-column-model";

export interface TxnDetailDialogSourceRow {
  key: string;
  className?: string;
  cells: Record<string, TxnDetailDialogCell | undefined>;
}

interface BuildTxnDetailDialogRowsOptions<Row extends TxnDetailDialogSourceRow> {
  rows: Row[];
  columns: Array<Pick<TxnDetailDialogColumn, "key">>;
  windowStart: number;
  windowEnd: number;
  moneyColumnKeys?: ReadonlySet<string>;
}

export const DEFAULT_TXN_DETAIL_MONEY_COLUMN_KEYS = TXN_DETAIL_MONEY_COLUMN_KEYS;

export function resolveTxnDetailCellKind(
  columnKey: string,
  cell: TxnDetailDialogCell | undefined,
  moneyColumnKeys: ReadonlySet<string> = DEFAULT_TXN_DETAIL_MONEY_COLUMN_KEYS
): TxnDetailDialogCell["kind"] {
  if (cell?.kind) {
    return cell.kind;
  }
  if (columnKey === "dc_flag") {
    return "direction";
  }
  if (moneyColumnKeys.has(columnKey)) {
    return "money";
  }
  return "text";
}

export function buildTxnDetailDialogRows<Row extends TxnDetailDialogSourceRow>({
  rows,
  columns,
  windowStart,
  windowEnd,
  moneyColumnKeys = DEFAULT_TXN_DETAIL_MONEY_COLUMN_KEYS
}: BuildTxnDetailDialogRowsOptions<Row>): TxnDetailDialogRow[] {
  const safeWindowStart = Math.max(0, Math.floor(Number(windowStart) || 0));
  const safeWindowEnd = Math.max(safeWindowStart, Math.floor(Number(windowEnd) || safeWindowStart));
  return rows.slice(safeWindowStart, safeWindowEnd).map((row, index) => {
    const absoluteIndex = safeWindowStart + index;
    return {
      key: row.key,
      className: row.className || (absoluteIndex % 2 === 0 ? "txnRow even" : "txnRow"),
      cells: columns.reduce<TxnDetailDialogRow["cells"]>((accumulator, column) => {
        const cell = row.cells[column.key];
        accumulator[column.key] = {
          ...(cell || {}),
          text: cell?.text ?? "-",
          kind: resolveTxnDetailCellKind(column.key, cell, moneyColumnKeys)
        };
        return accumulator;
      }, {})
    };
  });
}
