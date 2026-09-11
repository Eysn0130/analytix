import {
  arrayOf,
  clampInt,
  text
} from "./runtime-normalizers.mjs";
import { throwIfAborted } from "./abort-runtime.mjs";

export const STATS_QUERY_RUNTIME_VERSION = "0.14.3";

export function createStatsQueryRuntime({ resolveCase, httpPostData }) {
  async function queryStatsRows(args = {}, { signal } = {}) {
    throwIfAborted(signal);
    const resolved = await resolveCase(args, { signal });
    const data = await httpPostData("/analysis/stats/v2/query/rows/direct", {
      case_id: resolved.case_id,
      mode: text(args.mode) || "inName",
      selected: arrayOf(args.selected).map(text).filter(Boolean),
      date_start: text(args.date_start),
      date_end: text(args.date_end),
      search_text: text(args.search_text),
      row_sort_col: text(args.row_sort_col),
      row_sort_dir: text(args.row_sort_dir) === "asc" ? "asc" : "desc",
      row_offset: clampInt(args.row_offset, 0, 0, 1000000),
      row_limit: clampInt(args.row_limit, 50, 1, 5000)
    }, { signal });
    return {
      case_id: resolved.case_id,
      source: resolved.source,
      response: data,
      boundary: "bounded row tools are evidence samples only; report-grade totals must come from aggregate/ranking tools."
    };
  }

  async function queryStatsTxnRows(args = {}, { signal } = {}) {
    throwIfAborted(signal);
    const resolved = await resolveCase(args, { signal });
    const data = await httpPostData("/analysis/stats/v2/query/txn-rows/direct", {
      case_id: resolved.case_id,
      selected: arrayOf(args.selected).map(text).filter(Boolean),
      date_start: text(args.date_start),
      date_end: text(args.date_end),
      key_type: text(args.key_type) === "name" ? "name" : "account",
      key_value: text(args.key_value),
      key_values: arrayOf(args.key_values).map(text).filter(Boolean),
      filter: ["in", "out"].includes(text(args.filter)) ? text(args.filter) : "all",
      sort_col: text(args.sort_col) === "amount" ? "amount" : "txn_time",
      sort_dir: text(args.sort_dir) === "desc" ? "desc" : "asc",
      limit: clampInt(args.limit, 200, 1, 5000),
      cursor: args.cursor && typeof args.cursor === "object" ? args.cursor : null,
      fields: arrayOf(args.fields).map(text).filter(Boolean)
    }, { signal });
    return {
      case_id: resolved.case_id,
      source: resolved.source,
      response: data,
      boundary: "bounded transaction rows are evidence samples only; unsupported downstream paths cannot be inferred from samples."
    };
  }

  async function queryAccountTxnRows(args = {}, { signal } = {}) {
    throwIfAborted(signal);
    const resolved = await resolveCase(args, { signal });
    const data = await httpPostData("/analysis/stats/v2/account-txn-rows", {
      case_id: resolved.case_id,
      account_key: text(args.account_key || args.accountKey),
      date_start: text(args.date_start),
      date_end: text(args.date_end),
      start_time: text(args.start_time || args.startTime),
      end_time: text(args.end_time || args.endTime),
      sort_dir: text(args.sort_dir) === "desc" ? "desc" : "asc",
      limit: clampInt(args.limit, 200, 1, 5000),
      cursor: args.cursor && typeof args.cursor === "object" ? args.cursor : null
    }, { signal });
    return {
      case_id: resolved.case_id,
      source: resolved.source,
      response: data,
      boundary: "account transaction rows are bounded evidence samples; aggregate claims need deterministic summaries."
    };
  }

  async function getAnalysisDashboard(args = {}, { signal } = {}) {
    throwIfAborted(signal);
    const resolved = await resolveCase(args, { signal });
    const data = await httpPostData("/analysis/stats/v2/chart-dashboard", {
      case_id: resolved.case_id,
      selected: arrayOf(args.selected).map(text).filter(Boolean),
      date_start: text(args.date_start),
      date_end: text(args.date_end),
      metric_mode: text(args.metric_mode) || "amount",
      direction_mode: text(args.direction_mode) || "all",
      granularity: text(args.granularity) || "day",
      success_filter: text(args.success_filter) || "all",
      cash_filter: text(args.cash_filter) || "all",
      chart_filters: arrayOf(args.chart_filters),
      panel_views: arrayOf(args.panel_views)
    }, { signal });
    return {
      case_id: resolved.case_id,
      source: resolved.source,
      dashboard: data,
      boundary: "dashboard panels are display summaries; report-grade claims still require evidence-ledger facts."
    };
  }

  return {
    getAnalysisDashboard,
    queryAccountTxnRows,
    queryStatsRows,
    queryStatsTxnRows
  };
}
