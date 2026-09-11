import { describe, expect, it } from "vitest";
import type { ImportPreviewArchiveChildDTO, ImportPreviewFileDTO } from "../api";
import { buildWizardFile } from "./preview-to-wizard";

function child(overrides: Partial<ImportPreviewArchiveChildDTO> = {}): ImportPreviewArchiveChildDTO {
  return {
    file_name: "流水.csv",
    archive_path: "outer.zip::流水.csv",
    file_type: "CSV",
    size: 12,
    sha256: "b".repeat(64),
    rows_total: 1,
    columns_total: 2,
    header_preview: ["交易账号", "金额"],
    sample_rows: [["6222020202020202020", "1"]],
    domain_category: "structured",
    suggested_kind: "fc_transaction",
    suggested_kind_label: "交易明细",
    status: "ready",
    issue: "",
    detected_by: "archive-entry",
    field_mapping: { 交易账号: "account" },
    field_mapping_origins: { 交易账号: "template" },
    mapping_status: "ready",
    mapping_method: "template",
    mapping_message: "",
    mapping_required_missing: [],
    ...overrides
  };
}

function preview(overrides: Partial<ImportPreviewFileDTO> = {}): ImportPreviewFileDTO {
  return {
    file_name: "source.zip",
    source_path: "/controlled/source.zip",
    file_type: "ZIP",
    size: 100,
    sha256: "a".repeat(64),
    rows_total: 1,
    columns_total: 2,
    header_preview: [],
    sample_rows: [],
    domain_category: "structured",
    suggested_kind: "",
    suggested_kind_label: "待映射",
    status: "ready",
    issue: "",
    accepts_password: true,
    requires_password: true,
    detected_by: "archive",
    archive_children: [child()],
    field_mapping: {},
    field_mapping_origins: {},
    mapping_status: "ready",
    mapping_method: "template",
    mapping_message: "",
    mapping_required_missing: [],
    ...overrides
  };
}

describe("preview receipt state inheritance", () => {
  it("drops parent and child state when the parent digest changes", () => {
    const previous = buildWizardFile(preview());
    previous.password = "secret";
    previous.passwordVerified = true;
    previous.passwordState = "verified";
    previous.selectedKind = "fc_account";
    previous.archiveChildren[0].selectedKind = "fc_account";
    previous.archiveChildren[0].fieldMapping = { 交易账号: "bank_account" };

    const next = buildWizardFile(
      preview({ sha256: "c".repeat(64) }),
      previous
    );

    expect(next.password).toBe("");
    expect(next.passwordVerified).toBe(false);
    expect(next.selectedKind).toBe("");
    expect(next.archiveChildren[0].selectedKind).toBe("fc_transaction");
    expect(next.archiveChildren[0].fieldMapping).toEqual({});
  });

  it("never fills a missing new receipt from stale state", () => {
    const previous = buildWizardFile(preview());
    const next = buildWizardFile(
      preview({ sha256: "", archive_children: [child({ sha256: "" })] }),
      previous
    );

    expect(next.sha256).toBe("");
    expect(next.archiveChildren[0].sha256).toBe("");
  });
});
