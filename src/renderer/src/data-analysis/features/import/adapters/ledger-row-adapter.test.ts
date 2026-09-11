import { describe, expect, it } from "vitest";
import type { ImportFileLogDTO, ImportJobDTO, ImportJobFileDTO } from "../api";
import {
  resolveStatusText,
  toLedgerRow,
  toLedgerRowFromJobFile,
} from "./ledger-row-adapter";
import { knownNonnegativeInt } from "../model/public-count-projection";

function jobFile(overrides: Partial<ImportJobFileDTO> = {}): ImportJobFileDTO {
  return {
    file_id: "file-alpha",
    display_name: "alpha.csv",
    display_path: "",
    file_type: "CSV",
    size: 0,
    md5: "",
    sha256: "a".repeat(64),
    source_sha256: "a".repeat(64),
    source_size: 0,
    kind: "fc_transaction",
    status: "succeeded",
    rows_total: 0,
    rows_seen: 0,
    rows_imported_raw: 0,
    rows_imported_norm: 0,
    rows_dedup: 0,
    rows_error: 0,
    rows_skipped_non_data: 0,
    note: "",
    error: "",
    attempts: 0,
    ...overrides,
  };
}

function job(overrides: Partial<ImportJobDTO> = {}): ImportJobDTO {
  return {
    job_id: "job-alpha",
    case_id: "case-alpha",
    status: "succeeded",
    progress: 100,
    imported_files: 0,
    summary: {},
    files: [],
    current_file: "",
    error: null,
    created_at: "2026-07-20T00:00:00Z",
    updated_at: "2026-07-20T00:00:01Z",
    ...overrides,
  };
}

function fileLog(overrides: Partial<ImportFileLogDTO> = {}): ImportFileLogDTO {
  return {
    file_id: "file-alpha",
    kind: "fc_transaction",
    filename: "alpha.csv",
    display_path: "",
    stored_path: "controlled-source",
    file_type: "CSV",
    size: null,
    md5: "",
    sha256: "a".repeat(64),
    rows_total: null,
    rows_imported: null,
    rows_imported_raw: null,
    rows_imported_norm: null,
    rows_dedup: null,
    rows_error: null,
    rows_skipped_non_data: null,
    status: "已完成",
    error: "",
    cleaned_status: null,
    cleaned_started_at: null,
    cleaned_finished_at: null,
    cleaned_error: null,
    cleaned_rows_affected: null,
    created_at: "2026-07-20T00:00:00Z",
    finished_at: "2026-07-20T00:00:01Z",
    recycled_at: null,
    ...overrides,
  };
}

describe("import job verified-count boundary", () => {
  it("keeps running placeholder counts unresolved", () => {
    const row = toLedgerRowFromJobFile(
      jobFile({
        status: "running",
        rows_total: null,
        rows_seen: null,
        rows_imported_raw: null,
        rows_imported_norm: null,
        rows_dedup: null,
        rows_error: null,
        rows_skipped_non_data: null,
        attempts: null,
      }),
      job({ status: "running", progress: 25, imported_files: null }),
    );

    expect(row.rows_total).toBeNull();
    expect(row.rows_seen).toBeNull();
    expect(row.rows_imported_norm).toBeNull();
    expect(row.valid_rows).toBeNull();
    expect(row.duplicate_rows).toBeNull();
    expect(row.rows_skipped_non_data).toBeNull();
    expect(row.status_text).toBe("导入中");
  });

  it("preserves explicit verified zero without falling back to raw", () => {
    const row = toLedgerRowFromJobFile(
      jobFile({ rows_total: 7, rows_seen: 7, rows_imported_raw: 7, rows_imported_norm: 0 }),
      job(),
    );

    expect(row.rows_imported_raw).toBe(7);
    expect(row.rows_imported_norm).toBe(0);
    expect(row.rows_skipped_non_data).toBe(0);
    expect(row.valid_rows).toBe(0);
    expect(row.size).toBe(0);
    expect(row.attempts).toBe(0);
    expect(row.status_text).toBe("已完成");
  });

  it("does not call a zero-normalized result duplicate without exact dedup support", () => {
    const status = resolveStatusText({
      rawStatus: "succeeded",
      rowsTotal: 7,
      rowsSeen: 7,
      rowsRaw: 0,
      rowsNorm: 0,
      rowsDedup: null,
      rowsError: 0,
      errorText: "",
    });

    expect(status).toBe("已完成（统计未验证）");
  });

  it("requires an exact verified duplicate count before declaring duplicate data", () => {
    const status = resolveStatusText({
      rawStatus: "succeeded",
      rowsTotal: 7,
      rowsSeen: 7,
      rowsRaw: 0,
      rowsNorm: 0,
      rowsDedup: 7,
      rowsError: 0,
      errorText: "",
    });

    expect(status).toBe("重复数据");
  });

  it("does not infer a terminal state from numeric fields", () => {
    const status = resolveStatusText({
      rawStatus: "",
      rowsTotal: 7,
      rowsSeen: 7,
      rowsRaw: 7,
      rowsNorm: 7,
      rowsDedup: 0,
      rowsError: 0,
      errorText: "",
    });

    expect(status).toBe("状态未验证");
  });

  it.each([null, true, false, -1, -0, 1.5, "7", Number.NaN, Number.POSITIVE_INFINITY, Number.MAX_SAFE_INTEGER + 1])(
    "rejects non-canonical unsigned count %p",
    (value) => {
      expect(knownNonnegativeInt(value)).toBeNull();
    },
  );
});

describe("persisted import-log verified-count boundary", () => {
  it("keeps unresolved values null through the ledger adapter", () => {
    const row = toLedgerRow(fileLog());

    expect(row.rows_total).toBeNull();
    expect(row.rows_imported_norm).toBeNull();
    expect(row.rows_skipped_non_data).toBeNull();
    expect(row.valid_rows).toBeNull();
    expect(row.status_text).toBe("已完成（统计未验证）");
  });

  it("preserves an explicit verified zero", () => {
    const row = toLedgerRow(fileLog({
      size: 0,
      rows_total: 0,
      rows_imported: 0,
      rows_imported_raw: 0,
      rows_imported_norm: 0,
      rows_dedup: 0,
      rows_error: 0,
      rows_skipped_non_data: 0,
    }));

    expect(row.rows_total).toBe(0);
    expect(row.rows_imported_norm).toBe(0);
    expect(row.rows_skipped_non_data).toBe(0);
    expect(row.size).toBe(0);
  });
});
