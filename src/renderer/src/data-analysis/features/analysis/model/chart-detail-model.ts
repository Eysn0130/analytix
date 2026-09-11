import type { StatsTxnRowDTO } from "../api/stats-api";
import {
  projectOrdinaryCsvField,
  projectOrdinaryFieldValue,
} from "../../shared/ordinary-pii-projection";
import { formatCaseFactMoney } from "./stats-case-fact-number-model";

export const CHART_DETAIL_MIN_VISIBLE_COLUMNS = 1;

export interface ChartDetailColumnOption {
  id: string;
  label: string;
}

export interface ChartDetailColumnDefinition extends ChartDetailColumnOption {
  width: number;
  align?: "left" | "right" | "center";
  renderCell: (row: StatsTxnRowDTO) => string;
}

export interface ChartDetailColumnToggleResult {
  columnIds: string[];
  blocked: boolean;
}

export function formatChartDetailMoney(value: unknown): string {
  return formatCaseFactMoney(value);
}

export const CHART_DETAIL_COLUMN_DEFINITIONS: ChartDetailColumnDefinition[] = [
  { id: "txn_time", label: "交易时间", width: 164, renderCell: (row) => projectOrdinaryFieldValue("txn_time", row.txn_time) },
  { id: "amount", label: "交易金额", width: 120, align: "right", renderCell: (row) => formatChartDetailMoney(row.amount) },
  {
    id: "balance",
    label: "交易余额",
    width: 120,
    align: "right",
    renderCell: (row) => (row.balance === "" ? "" : formatChartDetailMoney(row.balance)),
  },
  { id: "dc_flag", label: "收付标志", width: 88, renderCell: (row) => projectOrdinaryFieldValue("dc_flag", row.dc_flag) },
  { id: "counterparty_acct", label: "对手账号", width: 170, renderCell: (row) => projectOrdinaryFieldValue("counterparty_acct", row.counterparty_acct) },
  { id: "counterparty_name", label: "对手户名", width: 140, renderCell: (row) => projectOrdinaryFieldValue("counterparty_name", row.counterparty_name) },
  { id: "counterparty_bank", label: "对手银行", width: 150, renderCell: (row) => projectOrdinaryFieldValue("counterparty_bank", row.counterparty_bank) },
  { id: "summary", label: "摘要说明", width: 160, renderCell: (row) => projectOrdinaryFieldValue("summary", row.summary) },
  { id: "currency", label: "币种", width: 76, renderCell: (row) => projectOrdinaryFieldValue("currency", row.currency) },
  { id: "location", label: "发生地", width: 120, renderCell: (row) => projectOrdinaryFieldValue("location", row.location) },
  { id: "is_success", label: "成功状态", width: 88, renderCell: (row) => projectOrdinaryFieldValue("is_success", row.is_success) },
  { id: "txn_type", label: "交易类型", width: 120, renderCell: (row) => projectOrdinaryFieldValue("txn_type", row.txn_type) },
  { id: "ip_addr", label: "IP地址", width: 140, renderCell: (row) => projectOrdinaryFieldValue("ip_addr", row.ip_addr) },
  { id: "mac_addr", label: "MAC地址", width: 140, renderCell: (row) => projectOrdinaryFieldValue("mac_addr", row.mac_addr) },
  { id: "remark", label: "备注", width: 160, renderCell: (row) => projectOrdinaryFieldValue("remark", row.remark) },
  {
    id: "query_feedback_reason",
    label: "反馈原因",
    width: 160,
    renderCell: (row) => projectOrdinaryFieldValue("query_feedback_reason", row.query_feedback_reason),
  },
];

export const CHART_DETAIL_COLUMN_OPTIONS: ChartDetailColumnOption[] = CHART_DETAIL_COLUMN_DEFINITIONS.map((column) => ({
  id: column.id,
  label: column.label,
}));

const CHART_DETAIL_COLUMN_REGISTRY = CHART_DETAIL_COLUMN_DEFINITIONS.reduce<Record<string, ChartDetailColumnDefinition>>((registry, column) => {
  registry[column.id] = column;
  return registry;
}, {});

export function getChartDetailColumnDefinitions(columnIds: string[]): ChartDetailColumnDefinition[] {
  return columnIds.map((id) => CHART_DETAIL_COLUMN_REGISTRY[id]).filter((column): column is ChartDetailColumnDefinition => Boolean(column));
}

export function getChartDetailExportColumns(columnIds: string[]): ChartDetailColumnOption[] {
  const visibleColumnIds = new Set(columnIds);
  return CHART_DETAIL_COLUMN_OPTIONS.filter((column) => visibleColumnIds.has(column.id));
}

export function toggleChartDetailColumnIds(
  previousColumnIds: string[],
  columnId: string,
  minVisibleColumns = CHART_DETAIL_MIN_VISIBLE_COLUMNS
): ChartDetailColumnToggleResult {
  const exists = previousColumnIds.includes(columnId);
  if (exists) {
    if (previousColumnIds.length <= minVisibleColumns) {
      return {
        columnIds: previousColumnIds,
        blocked: true,
      };
    }
    return {
      columnIds: previousColumnIds.filter((item) => item !== columnId),
      blocked: false,
    };
  }
  return {
    columnIds: [...previousColumnIds, columnId],
    blocked: false,
  };
}

export function encodeChartDetailCsvCell(value: unknown): string {
  const text = String(value ?? "");
  return `"${text.replace(/"/g, '""')}"`;
}

export function buildChartDetailCsv(rows: StatsTxnRowDTO[], columns: ChartDetailColumnOption[]): string {
  const header = columns.map((column) => column.label).join(",");
  const data = rows.map((row) =>
    columns
      .map((column) => {
        const raw = (row as unknown as Record<string, unknown>)[column.id];
        return encodeChartDetailCsvCell(projectOrdinaryCsvField(column.id, raw));
      })
      .join(",")
  );
  return [header, ...data].join("\n");
}

export function getChartDetailRowKey(row: StatsTxnRowDTO, index: number): string {
  return (
    [
      String(row.txn_id || row.id || ""),
      String(row.txn_time || ""),
      String(row.dc_flag || ""),
      String(row.amount || ""),
      String(row.counterparty_acct || row.counterparty_name || ""),
      String(row.log_id || ""),
      String(index),
    ]
      .filter(Boolean)
      .join(":") || String(index)
  );
}
