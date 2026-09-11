import { afterEach, describe, expect, it, vi } from "vitest";
import { httpClient } from "../../../services/http/client";
import { getActiveCase, getCaseDetail, normalizeCaseImportLogDTO } from "./index";

function casePayload(caseId: string) {
  return {
    request_id: "request-1",
    timestamp: "2026-07-15T00:00:00.000Z",
    data: {
      case_id: caseId,
      case_name: "显式案件",
      case_number: "CASE-001",
      owner: "",
      note: "",
      case_type: "other",
      tags: [],
      status: "active" as const,
      is_deleted: false,
      size_label: "0 B",
      size_bytes: 0,
      size_status: "available" as const,
      import_health: "unknown" as const,
      import_source_status: "available" as const,
      created_at: "2026-07-15T00:00:00.000Z",
      updated_at: "2026-07-15T00:00:00.000Z",
    },
  };
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("explicit case selection lookup", () => {
  it("fails closed before HTTP when case_id is empty", async () => {
    const get = vi.spyOn(httpClient, "get");

    await expect(getActiveCase("   ")).rejects.toThrow("case_id_required");

    expect(get).not.toHaveBeenCalled();
  });

  it("queries the active compatibility route with an encoded explicit case_id", async () => {
    const caseId = "case /? 中文";
    const get = vi.spyOn(httpClient, "get").mockResolvedValue(casePayload(caseId));

    const result = await getActiveCase(`  ${caseId}  `);

    expect(get).toHaveBeenCalledOnce();
    expect(get).toHaveBeenCalledWith(
      `/api/v1/cases/active?${new URLSearchParams({ case_id: caseId }).toString()}`,
    );
    expect(result.case_id).toBe(caseId);
  });
});

describe("case import count boundary", () => {
  it("keeps unresolved counts unknown and renders the message deterministically", async () => {
    const response = casePayload("case-alpha");
    const get = vi.spyOn(httpClient, "get").mockResolvedValue({
      ...response,
      data: {
        ...response.data,
        stats: { tasks: null, accounts: null, persons: null, tx: null, sub: null },
        stats_source_status: "unavailable" as const,
        imports_source_status: "available" as const,
        imports: [
          {
            title: "Imported source",
            time: "2026-07-20T00:00:00Z",
            status: "ok",
            msg: "导入 0 条",
            rows_total: null,
            rows_imported: null,
            rows_dedup: null,
            rows_error: null,
            rows_skipped_non_data: null,
            error: "",
          },
        ],
      },
    });

    const detail = await getCaseDetail("case-alpha");

    expect(get).toHaveBeenCalledWith("/api/v1/cases/case-alpha");
    expect(detail.imports[0].rows_total).toBeNull();
    expect(detail.imports[0].rows_imported).toBeNull();
    expect(detail.imports[0].rows_skipped_non_data).toBeNull();
    expect(detail.imports[0].status).toBe("unknown");
    expect(detail.imports[0].msg).toBe("导入数量未知");
  });

  it("preserves verified zero and downgrades invalid counts to unknown", () => {
    const explicitZero = normalizeCaseImportLogDTO({
      title: "empty.csv",
      time: "",
      status: "ok",
      msg: "untrusted",
      rows_total: 0,
      rows_imported: 0,
      rows_dedup: 0,
      rows_error: 0,
      rows_skipped_non_data: 0,
      error: "",
    });
    expect(explicitZero.rows_imported).toBe(0);
    expect(explicitZero.rows_skipped_non_data).toBe(0);
    expect(explicitZero.status).toBe("ok");
    expect(explicitZero.msg).toBe("导入 0 条");

    for (const invalid of [true, -1, -0, 1.5, "0", Number.MAX_SAFE_INTEGER + 1]) {
      const projected = normalizeCaseImportLogDTO({
        ...explicitZero,
        rows_imported: invalid,
      });
      expect(projected.rows_imported).toBeNull();
      expect(projected.msg).toBe("导入数量未知");
    }
  });
});
