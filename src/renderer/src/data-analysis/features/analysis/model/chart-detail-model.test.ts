import { describe, expect, it } from "vitest";
import type { StatsTxnRowDTO } from "../api/stats-api";
import { buildChartDetailCsv, getChartDetailColumnDefinitions } from "./chart-detail-model";

function txnRow(overrides: Partial<StatsTxnRowDTO>): StatsTxnRowDTO {
  return {
    id: "txn-1",
    card_no: "",
    acct_no: "",
    account_open_name: "",
    opener_id_no: "",
    txn_time: "",
    amount: "",
    balance: "",
    dc_flag: "",
    counterparty_acct: "",
    cash_flag: "",
    counterparty_name: "",
    counterparty_id_no: "",
    counterparty_bank: "",
    summary: "",
    currency: "",
    branch_name: "",
    location: "",
    is_success: "",
    voucher_no: "",
    ip_addr: "",
    mac_addr: "",
    counterparty_balance: "",
    txn_id: "",
    log_id: "",
    voucher_type: "",
    voucher_id: "",
    teller_no: "",
    remark: "",
    txn_type: "",
    query_feedback_reason: "",
    ...overrides,
  };
}

describe("ordinary chart detail projection", () => {
  it("renders restricted fields without mutating the source row", () => {
    const row = txnRow({
      counterparty_acct: "0000123456789012",
      ip_addr: "192.168.10.24",
      mac_addr: "AA:BB:CC:DD:EE:FF",
    });
    const columns = getChartDetailColumnDefinitions(["counterparty_acct", "ip_addr", "mac_addr"]);

    expect(columns.map((column) => column.renderCell(row))).toEqual([
      "0000********9012",
      "192.168.*.*",
      "AA:BB:CC:**:**:**",
    ]);
    expect(row.counterparty_acct).toBe("0000123456789012");
    expect(row.ip_addr).toBe("192.168.10.24");
    expect(row.mac_addr).toBe("AA:BB:CC:DD:EE:FF");
  });

  it("exports only masked CSV values and neutralizes formulas", () => {
    const row = txnRow({
      counterparty_acct: "0000123456789012",
      ip_addr: "192.168.10.24",
      summary: "复核账号 6214600780000579708",
      remark: "=HYPERLINK(\"https://example.invalid\")",
    });
    const csv = buildChartDetailCsv([row], [
      { id: "counterparty_acct", label: "对手账号" },
      { id: "ip_addr", label: "IP地址" },
      { id: "summary", label: "摘要" },
      { id: "remark", label: "备注" },
    ]);

    expect(csv).toContain('"0000********9012"');
    expect(csv).toContain('"192.168.*.*"');
    expect(csv).toContain('"复核账号 6214***********9708"');
    expect(csv).toContain('"\'=HYPERLINK(""https://example.invalid"")"');
    expect(csv).not.toContain("0000123456789012");
    expect(csv).not.toContain("192.168.10.24");
    expect(csv).not.toContain("6214600780000579708");
    expect(row.counterparty_acct).toBe("0000123456789012");
  });
});
