import type { ImportFileLogDTO } from "../../../services/import/api";
import { getCleaningFileStage, getImportedRowCount } from "./status";
import { canonicalNonnegativeCount, sumCompleteCounts } from "./known-metrics";

export interface CleaningFileStats {
  all: ImportFileLogDTO[];
  byKind: Record<string, ImportFileLogDTO[]>;
  transaction: ImportFileLogDTO[];
  account: ImportFileLogDTO[];
  pending: ImportFileLogDTO[];
  running: ImportFileLogDTO[];
  done: ImportFileLogDTO[];
  failed: ImportFileLogDTO[];
  importedRowsByKind: Record<string, number | null>;
  rowsAffected: number | null;
  importedRows: number | null;
  transactionImportedRows: number | null;
  accountImportedRows: number | null;
}

export function buildCleaningFileStats(importFiles: ImportFileLogDTO[]): CleaningFileStats {
  const stats: CleaningFileStats = {
    all: [],
    byKind: {},
    transaction: [],
    account: [],
    pending: [],
    running: [],
    done: [],
    failed: [],
    importedRowsByKind: {},
    rowsAffected: null,
    importedRows: null,
    transactionImportedRows: null,
    accountImportedRows: null
  };
  const importedCounts: Array<number | null> = [];
  const transactionImportedCounts: Array<number | null> = [];
  const accountImportedCounts: Array<number | null> = [];
  const importedCountsByKind: Record<string, Array<number | null>> = {};
  const affectedCounts: Array<number | null> = [];

  importFiles.forEach((item) => {
    const kind = typeof item.kind === "string" ? item.kind : "";
    if (!kind.startsWith("fc_")) {
      return;
    }

    stats.all.push(item);
    if (!stats.byKind[kind]) {
      stats.byKind[kind] = [];
    }
    stats.byKind[kind].push(item);

    if (kind === "fc_transaction") {
      stats.transaction.push(item);
    } else if (kind === "fc_account") {
      stats.account.push(item);
    }

    const stage = getCleaningFileStage(item);
    stats[stage].push(item);

    const importedRowCount = getImportedRowCount(item);
    importedCounts.push(importedRowCount);
    if (!importedCountsByKind[kind]) {
      importedCountsByKind[kind] = [];
    }
    importedCountsByKind[kind].push(importedRowCount);
    if (kind === "fc_transaction") {
      transactionImportedCounts.push(importedRowCount);
    } else if (kind === "fc_account") {
      accountImportedCounts.push(importedRowCount);
    }
    affectedCounts.push(
      stage === "done" ? canonicalNonnegativeCount(item.cleaned_rows_affected) : null
    );
  });

  stats.importedRows = sumCompleteCounts(importedCounts);
  stats.transactionImportedRows = sumCompleteCounts(transactionImportedCounts);
  stats.accountImportedRows = sumCompleteCounts(accountImportedCounts);
  stats.rowsAffected = sumCompleteCounts(affectedCounts);
  Object.entries(importedCountsByKind).forEach(([kind, counts]) => {
    stats.importedRowsByKind[kind] = sumCompleteCounts(counts);
  });

  return stats;
}
