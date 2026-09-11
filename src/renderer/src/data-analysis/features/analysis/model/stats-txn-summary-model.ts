import type { StatsRowDTO, StatsTxnRowDTO } from "../api/stats-api";

export type StatsTxnDirection = "in" | "out" | "unknown";

export interface StatsTxnFooterSummary {
  totalCount: number | null;
  inCount: number | null;
  outCount: number | null;
  inAmount: number | null;
  outAmount: number | null;
  firstTime: string;
  lastTime: string;
}

function toNum(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) ? value : null;
}

export function getStatsTxnDirectionTone(value: unknown): StatsTxnDirection {
  if (typeof value !== "string") {
    return "unknown";
  }
  const text = value.trim();
  if (!text) {
    return "unknown";
  }
  const normalized = text.toUpperCase();
  if (["IN", "INFLOW", "C", "CR", "CREDIT", "进", "进账", "入", "入账", "收入", "收", "贷"].includes(normalized)) {
    return "in";
  }
  if (["OUT", "OUTFLOW", "D", "DR", "DEBIT", "出", "出账", "支出", "支", "付", "借"].includes(normalized)) {
    return "out";
  }
  return "unknown";
}

export function summarizeStatsTxnRows(rows: StatsTxnRowDTO[]): StatsTxnFooterSummary {
  let inCount = 0;
  let outCount = 0;
  let inAmount = 0;
  let outAmount = 0;
  let hasIn = false;
  let hasOut = false;
  let inAmountComplete = true;
  let outAmountComplete = true;
  let directionComplete = true;
  let firstTime = "";
  let lastTime = "";

  for (const row of rows) {
    const amount = toNum(row.amount);
    const direction = getStatsTxnDirectionTone(row.dc_flag);
    if (direction === "out") {
      hasOut = true;
      outCount += 1;
      if (amount == null) {
        outAmountComplete = false;
      } else {
        outAmount += amount;
      }
    } else if (direction === "in") {
      hasIn = true;
      inCount += 1;
      if (amount == null) {
        inAmountComplete = false;
      } else {
        inAmount += amount;
      }
    } else {
      directionComplete = false;
    }
    const txnTime = String(row.txn_time || "").trim();
    if (!txnTime) {
      continue;
    }
    if (!firstTime || txnTime < firstTime) {
      firstTime = txnTime;
    }
    if (!lastTime || txnTime > lastTime) {
      lastTime = txnTime;
    }
  }

  return {
    totalCount: rows.length > 0 ? rows.length : null,
    inCount: directionComplete && hasIn ? inCount : null,
    outCount: directionComplete && hasOut ? outCount : null,
    inAmount: directionComplete && hasIn && inAmountComplete ? inAmount : null,
    outAmount: directionComplete && hasOut && outAmountComplete ? outAmount : null,
    firstTime,
    lastTime,
  };
}

export function summarizeStatsRowForTxnFooter(row: StatsRowDTO): StatsTxnFooterSummary {
  return {
    totalCount: toNum(row.total_count),
    inCount: toNum(row.in_count),
    outCount: toNum(row.out_count),
    inAmount: toNum(row.in_amount),
    outAmount: toNum(row.out_amount),
    firstTime: String(row.first_time || "").trim(),
    lastTime: String(row.last_time || "").trim(),
  };
}

export function buildStatsTxnFooterSummary(
  activeRow: StatsRowDTO | null,
  txnRows: StatsTxnRowDTO[]
): StatsTxnFooterSummary {
  return activeRow ? summarizeStatsRowForTxnFooter(activeRow) : summarizeStatsTxnRows(txnRows);
}
