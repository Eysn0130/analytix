import type { TxnDetailDialogCell } from "./TxnDetailDialog";
import {
  formatTxnDetailMoney,
  type TxnDetailColumnKind,
  type TxnDetailColumnSpec,
  TXN_DETAIL_MONO_COLUMN_KEYS,
  TXN_DETAIL_MONEY_COLUMN_KEYS
} from "./txn-detail-column-model";
import { projectOrdinaryFieldValue } from "../../features/shared/ordinary-pii-projection";

export type TxnDetailDirectionTone = "in" | "out";

export interface BuildTxnDetailCellOptions {
  column: Pick<TxnDetailColumnSpec, "key" | "kind">;
  raw: unknown;
  emptyText?: string;
  formatMoney?: (value: unknown) => string;
  includeMono?: boolean;
}

export interface TxnDetailRenderCell extends TxnDetailDialogCell {
  text: string;
  kind: TxnDetailColumnKind;
  tone: string;
  align: "left" | "right";
}

export function getTxnDetailDirectionTone(value: unknown): TxnDetailDirectionTone {
  const text = String(value || "").trim();
  if (text.includes("出") || text.includes("付") || text.includes("借")) {
    return "out";
  }
  return "in";
}

export function isTxnDetailMoneyColumn(columnKey: string): boolean {
  return TXN_DETAIL_MONEY_COLUMN_KEYS.has(columnKey);
}

export function isTxnDetailMonoColumn(columnKey: string): boolean {
  return TXN_DETAIL_MONO_COLUMN_KEYS.has(columnKey);
}

export function resolveTxnDetailColumnKind(column: Pick<TxnDetailColumnSpec, "key" | "kind">): TxnDetailColumnKind {
  if (column.kind === "direction" || column.key === "dc_flag") {
    return "direction";
  }
  if (column.kind === "money" || isTxnDetailMoneyColumn(column.key)) {
    return "money";
  }
  return "text";
}

export function formatTxnDetailCellText({
  emptyText = "",
  formatMoney = formatTxnDetailMoney,
  kind,
  raw
}: {
  emptyText?: string;
  formatMoney?: (value: unknown) => string;
  kind: TxnDetailColumnKind;
  raw: unknown;
}): string {
  if (raw === "" || raw == null) {
    return emptyText;
  }
  return kind === "money" ? formatMoney(raw) : String(raw);
}

export function buildTxnDetailCell({
  column,
  emptyText = "",
  formatMoney = formatTxnDetailMoney,
  includeMono = false,
  raw
}: BuildTxnDetailCellOptions): TxnDetailRenderCell {
  const kind = resolveTxnDetailColumnKind(column);
  const projectedRaw = kind === "text" ? projectOrdinaryFieldValue(column.key, raw) : raw;
  const cell: TxnDetailRenderCell = {
    text: formatTxnDetailCellText({ emptyText, formatMoney, kind, raw: projectedRaw }),
    kind,
    tone: kind === "direction" ? getTxnDetailDirectionTone(raw) : "",
    align: kind === "money" ? "right" : "left"
  };
  if (includeMono && isTxnDetailMonoColumn(column.key)) {
    cell.mono = true;
  }
  return cell;
}
