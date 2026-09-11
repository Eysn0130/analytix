import { describe, expect, it } from "vitest";

import {
  normalizeCleaningJobBoundary,
  normalizeCleaningStepDetailBoundary,
  normalizeCleaningStepSummaryBoundary
} from "../../../services/cleaning/api";
import type { ImportFileLogDTO } from "../../../services/import/api";
import { buildUnverifiedCleanableDataActionBlock } from "./action-blocks";
import {
  applyExportJobSnapshotState,
  buildIdleExportState,
  buildExportJobSnapshotProjection
} from "./export";
import { buildCleaningFileStats } from "./file-stats";
import { formatCount, formatDurationMs, formatNodeCount } from "./formatters";
import { buildCleaningHeroActionsViewModel } from "./hero-actions";
import {
  buildCleaningAvailability,
  getCleaningFileStage,
  getImportedRowCount
} from "./status";
import type { ExportState } from "./types";
import {
  buildCleanedExportReadinessViewModel,
  buildCleaningStepViewModels,
  buildCleaningSummaryCards
} from "./view-model";
import {
  buildCleaningRealtimeProjection,
  buildExportRealtimeProjection
} from "./realtime-events";

const CASE_ID = "case-cleaning-missing-values";

function importFile(overrides: Record<string, unknown> = {}): ImportFileLogDTO {
  return {
    file_id: "file-a",
    kind: "fc_transaction",
    filename: "transactions.csv",
    display_path: "transactions.csv",
    stored_path: "/controlled/transactions.csv",
    file_type: "csv",
    size: 1,
    md5: "",
    sha256: "a".repeat(64),
    rows_total: 9,
    rows_imported: 8,
    rows_imported_raw: 8,
    rows_imported_norm: 7,
    rows_dedup: 1,
    rows_error: 0,
    status: "done",
    error: null,
    cleaned_status: "done",
    cleaned_started_at: "2026-07-20T00:00:00Z",
    cleaned_finished_at: "2026-07-20T00:00:01Z",
    cleaned_error: null,
    cleaned_rows_affected: 2,
    created_at: "2026-07-20T00:00:00Z",
    finished_at: "2026-07-20T00:00:01Z",
    recycled_at: null,
    ...overrides
  } as unknown as ImportFileLogDTO;
}

describe("cleaning missing-value boundary", () => {
  it("preserves an explicit normalized zero without falling through to another metric", () => {
    const file = importFile({ rows_imported_norm: 0, rows_imported: 8, rows_total: 9 });

    expect(getImportedRowCount(file)).toBe(0);
    expect(buildCleaningFileStats([file]).importedRows).toBe(0);
    expect(buildCleaningAvailability([file], null)).toEqual({
      hasTransactionData: false,
      scopeFiles: [],
      scopeRows: 0
    });
  });

  it.each([
    null,
    undefined,
    Number.NaN,
    Number.POSITIVE_INFINITY,
    -1,
    -0,
    1.5,
    Number.MAX_SAFE_INTEGER + 1,
    "0",
    false
  ])(
    "keeps a non-canonical normalized count %p unresolved",
    (invalid) => {
      const file = importFile({ rows_imported_norm: invalid, rows_imported: 8, rows_total: 9 });

      expect(getImportedRowCount(file)).toBeNull();
      const stats = buildCleaningFileStats([file]);
      expect(stats.importedRows).toBeNull();
      expect(stats.transactionImportedRows).toBeNull();
      expect(stats.importedRowsByKind.fc_transaction).toBeNull();

      const availability = buildCleaningAvailability([file], null);
      expect(availability.scopeRows).toBeNull();
      expect(availability.hasTransactionData).toBeNull();
    }
  );

  it("does not publish a partial aggregate when one file count is unresolved", () => {
    const stats = buildCleaningFileStats([
      importFile({ file_id: "known", rows_imported_norm: 7 }),
      importFile({ file_id: "unknown", rows_imported_norm: null })
    ]);

    expect(stats.importedRows).toBeNull();
    expect(stats.transactionImportedRows).toBeNull();
    expect(stats.importedRowsByKind.fc_transaction).toBeNull();
  });

  it("does not publish a count whose aggregate exceeds safe integer precision", () => {
    const stats = buildCleaningFileStats([
      importFile({ file_id: "large", rows_imported_norm: Number.MAX_SAFE_INTEGER }),
      importFile({ file_id: "one", rows_imported_norm: 1 })
    ]);

    expect(stats.importedRows).toBeNull();
    expect(stats.transactionImportedRows).toBeNull();
  });

  it("keeps absent aggregates and unrun cleaning effects unresolved", () => {
    const empty = buildCleaningFileStats([]);
    expect(empty.importedRows).toBeNull();
    expect(empty.rowsAffected).toBeNull();

    const pending = buildCleaningFileStats([
      importFile({
        cleaned_status: null,
        cleaned_finished_at: null,
        cleaned_rows_affected: 0
      })
    ]);
    expect(pending.rowsAffected).toBeNull();
  });

  it("does not upgrade an unbound finished marker into a completed cleaning stage", () => {
    expect(getCleaningFileStage(importFile({ cleaned_status: null, cleaned_finished_at: false }))).toBe("pending");
    expect(getCleaningFileStage(importFile({ cleaned_status: null, cleaned_finished_at: { hostile: true } }))).toBe("pending");
    expect(getCleaningFileStage(importFile({ cleaned_status: null, cleaned_finished_at: "2026-07-20T00:00:01Z" }))).toBe("pending");
    expect(getCleaningFileStage(importFile({ cleaned_status: "done", cleaned_finished_at: null }))).toBe("done");
  });

  it("does not turn conflicting source counts into a case row fact", () => {
    const availability = buildCleaningAvailability(
      [importFile({ rows_imported_norm: 7 })],
      { stats: { tx: 0 } } as never
    );

    expect(availability.scopeRows).toBeNull();
    expect(availability.hasTransactionData).toBeNull();
  });

  it("renders untrusted numeric lookalikes as unknown while preserving real zero", () => {
    expect(formatCount(0)).toBe("0");
    expect(formatNodeCount(0)).toBe("0");
    expect(formatDurationMs(0)).toBe("0ms");

    for (const invalid of [
      null,
      undefined,
      Number.NaN,
      Number.POSITIVE_INFINITY,
      -1,
      -0,
      1.5,
      Number.MAX_SAFE_INTEGER + 1,
      "0",
      false
    ]) {
      expect(formatCount(invalid as never)).toBe("--");
      expect(formatNodeCount(invalid as never)).toBe("--");
      expect(formatDurationMs(invalid as never)).toBe("--");
    }
  });

  it("keeps every unrun catalog step unresolved instead of creating zero hits", () => {
    const steps = buildCleaningStepViewModels([]);

    expect(steps.length).toBeGreaterThan(0);
    expect(steps.every((step) => step.affectedRows === null)).toBe(true);
    expect(steps.every((step) => step.affectedRowsLabel === "--")).toBe(true);
    expect(steps.every((step) => step.hit === false && step.stateLabel === "待证据")).toBe(true);
  });

  it("renders unresolved file and case row metrics as unknown", () => {
    const fileStats = buildCleaningFileStats([
      importFile({ rows_imported_norm: null })
    ]);
    const cleaningAvailability = buildCleaningAvailability(fileStats.all, null);
    const cards = buildCleaningSummaryCards({
      accountStepAffected: null,
      caseDetail: null,
      cleanedOutputReady: false,
      cleaningAvailability,
      currentJob: null,
      fileStats,
      latestSucceededJob: null,
      txnStepAffected: null
    });

    expect(cards.find((card) => card.key === "scope")?.value).toBe("--");
    expect(cards.find((card) => card.key === "transaction")?.value).toBe("--");
  });

  it("blocks cleaning and cleaned export when transaction coverage is unknown", () => {
    const cleaningAvailability = {
      hasTransactionData: null,
      scopeFiles: [],
      scopeRows: null
    };
    const readiness = buildCleanedExportReadinessViewModel(cleaningAvailability, null);
    const actions = buildCleaningHeroActionsViewModel({
      cleanedExportButtonTitle: readiness.buttonTitle,
      cleanedExportReady: readiness.ready,
      cleaningAvailability,
      cleaningRuntimeUnavailable: false,
      cleaningRuntimeUnavailableMessage: "",
      currentJobActive: false,
      exportBusy: false,
      exportCleanedWorking: false,
      exportRawWorking: false,
      recleanActionWorking: false,
      runActionWorking: false
    });

    expect(readiness.ready).toBe(false);
    expect(readiness.buttonTitle).toContain("尚未验证");
    expect(actions.reclean.disabled).toBe(true);
    expect(buildUnverifiedCleanableDataActionBlock().message).toContain("已阻止");
  });

  it.each([null, undefined, "0", false, Number.NaN, Number.POSITIVE_INFINITY, -1, -0, 1.5])(
    "rejects a non-canonical cleaning progress %p",
    (invalid) => {
      expect(() => normalizeCleaningJobBoundary({
        job_id: "job-a",
        case_id: CASE_ID,
        status: "running",
        progress: invalid,
        created_at: "2026-07-20T00:00:00Z",
        updated_at: "2026-07-20T00:00:01Z"
      }, CASE_ID)).toThrow("cleaning_job_progress_invalid");
    }
  );

  it("accepts only canonical numeric step identities", () => {
    const summary = normalizeCleaningStepSummaryBoundary({
      case_id: CASE_ID,
      items: [{ step: "1", key: "hostile-string-step" }]
    }, CASE_ID);
    expect(summary.items).toEqual([]);
    expect(() => normalizeCleaningStepDetailBoundary({ case_id: CASE_ID, step: "1" }, CASE_ID, 1))
      .toThrow("cleaning_case_binding_mismatch");
  });

  it("does not replace an unresolved export snapshot progress with zero", () => {
    const previous: ExportState = {
      jobId: "export-a",
      kind: "cleaned",
      status: "running",
      progress: 37,
      outputPath: "",
      error: "",
      message: "导出进行中"
    };
    const projection = buildExportJobSnapshotProjection({ status: "running", progress: null });

    expect(projection.progress).toBeNull();
    expect(applyExportJobSnapshotState(previous, "export-a", projection).progress).toBe(37);
  });

  it("keeps not-started export progress unknown", () => {
    expect(buildIdleExportState().progress).toBeNull();
  });

  it("rejects malformed realtime progress and step lookalikes", () => {
    const cleaning = buildCleaningRealtimeProjection({
      sequence: 1,
      event: "cleaning.job.progress",
      job_id: "cleaning-a",
      type: "progress",
      timestamp: "2026-07-20T00:00:00Z",
      payload: { progress: Number.NaN, step: "1" }
    } as never);
    const exportProjection = buildExportRealtimeProjection({
      sequence: 2,
      event: "export.job.progress",
      job_id: "export-a",
      type: "progress",
      timestamp: "2026-07-20T00:00:00Z",
      payload: { progress: Number.POSITIVE_INFINITY }
    } as never);

    expect(cleaning.liveEvent.progress).toBeNull();
    expect(cleaning.liveEvent.step).toBeNull();
    expect(exportProjection.progress).toBeNull();
  });
});
