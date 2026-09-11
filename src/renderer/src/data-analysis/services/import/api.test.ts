import { afterEach, describe, expect, it, vi } from "vitest";
import { httpClient } from "../http/client";
import {
  getImportJob,
  listHistoricalImportDatasets,
  listImportFiles,
  parseImportFileLogDTO,
  parseImportFileLogListData,
  parseImportHistoricalDatasetDTO,
  parseImportHistoricalDatasetListData,
  parseImportBatchActionResultDTO,
  parseImportJobDTO,
} from "./api";

const JOB_ID = "00000000-0000-4000-8000-000000000001";
const FILE_REF = `importfile_v1_${"a".repeat(64)}`;
const DATASET_REF = `importdataset_v1_${"b".repeat(64)}`;

afterEach(() => {
  vi.restoreAllMocks();
});

function payload(): Record<string, unknown> {
  return {
    job_id: JOB_ID,
    case_id: "case-alpha",
    status: "succeeded",
    progress: 100,
    imported_files: 0,
    summary: {
      total_files: 1,
      rows_total: 0,
      rows_imported_norm: 0,
    },
    files: [
      {
        file_id: FILE_REF,
        display_name: "Imported source",
        display_path: "",
        file_type: "csv",
        size: 0,
        md5: null,
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
      },
    ],
    current_file: "",
    error: null,
    created_at: "2026-07-20T00:00:00Z",
    updated_at: "2026-07-20T00:00:01Z",
  };
}

function fileLogPayload(): Record<string, unknown> {
  return {
    file_id: FILE_REF,
    kind: "fc_transaction",
    filename: "Imported source",
    display_path: "",
    stored_path: "",
    file_type: "csv",
    size: 0,
    md5: null,
    sha256: "a".repeat(64),
    rows_total: 0,
    rows_imported: 0,
    rows_imported_raw: 0,
    rows_imported_norm: 0,
    rows_dedup: 0,
    rows_error: 0,
    rows_skipped_non_data: 0,
    status: "succeeded",
    error: "",
    cleaned_status: "succeeded",
    cleaned_started_at: "2026-07-20T00:00:00Z",
    cleaned_finished_at: "2026-07-20T00:00:01Z",
    cleaned_error: "",
    cleaned_rows_affected: 0,
    created_at: "2026-07-20T00:00:00Z",
    finished_at: "2026-07-20T00:00:01Z",
    recycled_at: null,
  };
}

function historicalDatasetPayload(): Record<string, unknown> {
  return {
    dataset_id: DATASET_REF,
    filename: "Imported source",
    kind: "fc_transaction",
    rows: 7,
    cols: 3,
    imported_at: "2026-07-21T00:00:00Z",
    stored_path: "",
  };
}

describe("import job HTTP payload boundary", () => {
  it("preserves explicit zero and unresolved null as distinct states", () => {
    const explicitZero = parseImportJobDTO(payload(), "case-alpha");
    expect(explicitZero.imported_files).toBe(0);
    expect(explicitZero.files[0].rows_imported_norm).toBe(0);
    expect(explicitZero.files[0].size).toBe(0);

    const unresolved = payload();
    unresolved.imported_files = null;
    const files = unresolved.files as Array<Record<string, unknown>>;
    files[0].rows_total = null;
    files[0].rows_imported_norm = null;
    files[0].attempts = null;
    const parsed = parseImportJobDTO(unresolved, "case-alpha");
    expect(parsed.imported_files).toBeNull();
    expect(parsed.files[0].rows_total).toBeNull();
    expect(parsed.files[0].rows_imported_norm).toBeNull();
    expect(parsed.files[0].attempts).toBeNull();
  });

  it.each([true, -1, -0, 1.5, "7", Number.MAX_SAFE_INTEGER + 1])(
    "rejects a non-canonical job count %p",
    (invalid) => {
      const value = payload();
      value.imported_files = invalid;
      expect(() => parseImportJobDTO(value, "case-alpha")).toThrow("import_job_imported_files_invalid");
    },
  );

  it("rejects non-canonical nested counts", () => {
    const value = payload();
    const files = value.files as Array<Record<string, unknown>>;
    files[0].rows_total = true;
    expect(() => parseImportJobDTO(value, "case-alpha")).toThrow("import_job_file_rows_total_invalid");
  });

  it("rejects unknown fields at every external object boundary", () => {
    const jobValue = payload();
    jobValue.untrusted = 1;
    expect(() => parseImportJobDTO(jobValue, "case-alpha")).toThrow("import_job_payload_unknown_field");

    const fileValue = payload();
    (fileValue.files as Array<Record<string, unknown>>)[0].untrusted = 1;
    expect(() => parseImportJobDTO(fileValue, "case-alpha")).toThrow("import_job_file_unknown_field");

    const summaryValue = payload();
    (summaryValue.summary as Record<string, unknown>).untrusted = 1;
    expect(() => parseImportJobDTO(summaryValue, "case-alpha")).toThrow("import_job_summary_unknown_field");
  });

  it("rejects cross-case jobs and ordinary-path private source fields", () => {
    const wrongCase = payload();
    wrongCase.case_id = "case-bravo";
    expect(() => parseImportJobDTO(wrongCase, "case-alpha")).toThrow(
      "import_job_case_id_mismatch",
    );

    const privateSource = payload();
    const files = privateSource.files as Array<Record<string, unknown>>;
    files[0].display_name = "62220202020202020202.csv";
    expect(() => parseImportJobDTO(privateSource, "case-alpha")).toThrow(
      "import_job_file_display_name_invalid",
    );
  });

  it("binds job polling to the caller case", async () => {
    const get = vi.spyOn(httpClient, "get").mockResolvedValue({
      request_id: "request-1",
      timestamp: "2026-07-20T00:00:00Z",
      data: payload(),
    });

    await expect(getImportJob("case-alpha", JOB_ID)).resolves.toMatchObject({
      job_id: JOB_ID,
      case_id: "case-alpha",
    });
    expect(get).toHaveBeenCalledWith(
      `/api/v1/import/jobs/${JOB_ID}?case_id=case-alpha`,
    );
  });
});

describe("import batch action HTTP payload boundary", () => {
  it("preserves a verified empty result as zero", () => {
    expect(
      parseImportBatchActionResultDTO(
        { ok: true, action: "recycle", affected_count: 0, file_ids: [] },
        "recycle",
        [FILE_REF],
      ),
    ).toEqual({ ok: true, action: "recycle", affected_count: 0, file_ids: [] });
  });

  it.each([
    { ok: true, action: "recycle", affected_count: 1, file_ids: [] },
    { ok: true, action: "recycle", affected_count: 2, file_ids: [FILE_REF, FILE_REF] },
    { ok: true, action: "recycle", affected_count: 1, file_ids: ["other-case-file"] },
    { ok: true, action: "restore", affected_count: 1, file_ids: [FILE_REF] },
    { ok: true, action: "recycle", affected_count: null, file_ids: [] },
    { ok: true, action: "recycle", affected_count: 1, file_ids: [FILE_REF], untrusted: true },
  ])("rejects unverified batch action coverage %#", (value) => {
    expect(() =>
      parseImportBatchActionResultDTO(value, "recycle", [FILE_REF]),
    ).toThrow();
  });
});

describe("import file log HTTP payload boundary", () => {
  it("preserves explicit zero and unresolved null as distinct states", () => {
    const explicitZero = parseImportFileLogDTO(fileLogPayload());
    expect(explicitZero.rows_total).toBe(0);
    expect(explicitZero.rows_skipped_non_data).toBe(0);
    expect(explicitZero.cleaned_rows_affected).toBe(0);

    const unresolved = fileLogPayload();
    for (const field of [
      "size",
      "rows_total",
      "rows_imported",
      "rows_imported_raw",
      "rows_imported_norm",
      "rows_dedup",
      "rows_error",
      "rows_skipped_non_data",
      "cleaned_rows_affected",
    ]) {
      unresolved[field] = null;
    }
    const parsed = parseImportFileLogListData({ items: [unresolved] });
    expect(parsed.items[0].size).toBeNull();
    expect(parsed.items[0].rows_total).toBeNull();
    expect(parsed.items[0].rows_skipped_non_data).toBeNull();
    expect(parsed.items[0].cleaned_rows_affected).toBeNull();
  });

  it.each([true, -1, -0, 1.5, "0", Number.MAX_SAFE_INTEGER + 1])(
    "rejects a non-canonical file-log count %p",
    (invalid) => {
      const value = fileLogPayload();
      value.rows_total = invalid;
      expect(() => parseImportFileLogDTO(value)).toThrow("import_file_log_rows_total_invalid");
    },
  );

  it("rejects missing and unknown external fields", () => {
    const missing = fileLogPayload();
    delete missing.rows_skipped_non_data;
    expect(() => parseImportFileLogDTO(missing)).toThrow(
      "import_file_log_rows_skipped_non_data_missing",
    );

    const unknown = fileLogPayload();
    unknown.untrusted = 1;
    expect(() => parseImportFileLogDTO(unknown)).toThrow("import_file_log_item_unknown_field");
    expect(() => parseImportFileLogListData({ items: [], untrusted: 1 })).toThrow(
      "import_file_log_list_unknown_field",
    );
  });

  it("rejects source names, paths, and raw identifiers from a compromised backend", () => {
    for (const [field, value] of [
      ["file_id", "62220202020202020202"],
      ["filename", "62220202020202020202.csv"],
      ["display_path", "/Users/private/account.csv"],
      ["stored_path", "/Users/private/account.csv"],
    ] as const) {
      const hostile = fileLogPayload();
      hostile[field] = value;
      expect(() => parseImportFileLogDTO(hostile)).toThrow();
    }
  });

  it("validates listImportFiles responses before returning them", async () => {
    const unresolved = fileLogPayload();
    unresolved.rows_total = null;
    unresolved.rows_skipped_non_data = null;
    const get = vi.spyOn(httpClient, "get").mockResolvedValue({
      request_id: "request-1",
      timestamp: "2026-07-20T00:00:00Z",
      data: { items: [unresolved] },
    });

    const result = await listImportFiles("case-alpha", "active");

    expect(get).toHaveBeenCalledWith("/api/v1/import/files?case_id=case-alpha&view=active");
    expect(result.items[0].rows_total).toBeNull();
    expect(result.items[0].rows_skipped_non_data).toBeNull();
  });
});

describe("historical import dataset HTTP payload boundary", () => {
  it("MissingCountsRemainUnknown", () => {
    const value = historicalDatasetPayload();
    value.rows = null;
    value.cols = null;

    const parsed = parseImportHistoricalDatasetListData({ items: [value] });

    expect(parsed.items[0].rows).toBeNull();
    expect(parsed.items[0].cols).toBeNull();
  });

  it.each([
    ["rows", true],
    ["rows", -1],
    ["rows", -0],
    ["rows", 1.5],
    ["rows", "7"],
    ["rows", Number.MAX_SAFE_INTEGER + 1],
    ["cols", false],
    ["cols", -1],
    ["cols", -0],
    ["cols", 1.5],
    ["cols", "7"],
    ["cols", Number.MAX_SAFE_INTEGER + 1],
  ])(
    "MalformedCountsFailClosed: rejects %s=%p",
    (field, invalid) => {
      const value = historicalDatasetPayload();
      value[field as string] = invalid;
      expect(() => parseImportHistoricalDatasetDTO(value)).toThrow(
        `historical_dataset_${field}_invalid`,
      );
    },
  );

  it("MalformedCountsFailClosed: rejects missing and extra fields", () => {
    const missing = historicalDatasetPayload();
    delete missing.cols;
    expect(() => parseImportHistoricalDatasetDTO(missing)).toThrow(
      "historical_dataset_cols_missing",
    );

    const unknown = historicalDatasetPayload();
    unknown.untrusted = 1;
    expect(() => parseImportHistoricalDatasetDTO(unknown)).toThrow(
      "historical_dataset_item_unknown_field",
    );
    expect(() => parseImportHistoricalDatasetListData({ items: [], untrusted: 1 })).toThrow(
      "historical_dataset_list_unknown_field",
    );
    expect(() => parseImportHistoricalDatasetListData({})).toThrow(
      "historical_dataset_list_items_missing",
    );
  });

  it("rejects raw dataset identity and source paths", () => {
    const hostile = historicalDatasetPayload();
    hostile.dataset_id = "62220202020202020202";
    hostile.filename = "62220202020202020202.csv";
    hostile.stored_path = "/Users/private/account.csv";
    expect(() => parseImportHistoricalDatasetDTO(hostile)).toThrow();
  });

  it("validates listHistoricalImportDatasets responses before returning them", async () => {
    const unresolved = historicalDatasetPayload();
    unresolved.rows = null;
    unresolved.cols = null;
    const get = vi.spyOn(httpClient, "get").mockResolvedValue({
      request_id: "request-1",
      timestamp: "2026-07-21T00:00:00Z",
      data: { items: [unresolved] },
    });

    const result = await listHistoricalImportDatasets("case-alpha");

    expect(get).toHaveBeenCalledWith(
      "/api/v1/import/files/historical-datasets?case_id=case-alpha",
    );
    expect(result.items[0].rows).toBeNull();
    expect(result.items[0].cols).toBeNull();
  });
});
