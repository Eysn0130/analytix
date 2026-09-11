import { describe, expect, it } from "vitest";
import ordinaryPIICorpusJSON from "../../../../../../packages/runtime/src/conformance/fixtures/ordinary-pii-projection-v1.json";
import { buildTxnDetailCell } from "../../components/txn-detail/txn-detail-cell-model";
import {
  normalizeOrdinaryPresentationFieldName,
  projectDetectedOrdinaryRestrictedPii,
  projectOrdinaryClipboardValue,
  projectOrdinaryCsvField,
  projectOrdinaryFieldValue,
  projectOrdinaryRestrictedPii,
  projectOrdinaryStructuredValue,
  protectOrdinaryCsvFormula,
  resolveRestrictedPiiKind,
  uncontrolledRestrictedPiiExportBlockReason,
} from "./ordinary-pii-projection";

type OrdinaryPIICorpus = {
  schemaVersion: number;
  syntheticOnly: boolean;
  untrustedTextCases: Array<{
    id: string;
    input: string;
    expected: "mask" | "preserve";
    sensitiveCanonical?: string;
  }>;
  typedFieldCases: Array<{
    id: string;
    fieldName: string;
    value: string;
    expected: "mask" | "preserve";
    sensitiveCanonical?: string;
  }>;
};

const ordinaryPIICorpus = ordinaryPIICorpusJSON as OrdinaryPIICorpus;

function canonicalDigits(value: string): string {
  return value.normalize("NFKC").replace(/[^0-9]/g, "");
}

describe("ordinary restricted PII projection", () => {
  it("conforms to the shared synthetic ordinary and typed-field PII corpus", () => {
    expect(ordinaryPIICorpus.schemaVersion).toBe(1);
    expect(ordinaryPIICorpus.syntheticOnly).toBe(true);
    for (const testCase of ordinaryPIICorpus.untrustedTextCases) {
      const projected = projectDetectedOrdinaryRestrictedPii(testCase.input);
      if (testCase.expected === "preserve") {
        expect(projected, testCase.id).toBe(testCase.input);
        continue;
      }
      expect(projected, testCase.id).not.toBe(testCase.input);
      if (testCase.sensitiveCanonical) {
        expect(canonicalDigits(projected), testCase.id).not.toContain(testCase.sensitiveCanonical);
      }
    }
    for (const testCase of ordinaryPIICorpus.typedFieldCases) {
      const projected = projectOrdinaryFieldValue(testCase.fieldName, testCase.value);
      if (testCase.expected === "preserve") {
        expect(projected, testCase.id).toBe(testCase.value);
        continue;
      }
      expect(projected, testCase.id).not.toBe(testCase.value);
      if (testCase.sensitiveCanonical) {
        expect(canonicalDigits(projected), testCase.id).not.toContain(testCase.sensitiveCanonical);
      }
    }
  });

  it("does not trust an arbitrary star as proof that a short account was safely masked", () => {
    const projected = projectOrdinaryRestrictedPii("1234*5678", "bank-account");
    expect(projected).not.toBe("1234*5678");
    expect(canonicalDigits(projected)).not.toContain("12345678");
  });

  it("masks long bank accounts without converting them to numbers", () => {
    const value = "621460078000057970812345678901234567890";
    const projected = projectOrdinaryRestrictedPii(value, "bank-account");

    expect(projected).toBe("6214*******************************7890");
    expect(projected).not.toContain(value);
    expect(value).toBe("621460078000057970812345678901234567890");
  });

  it("preserves leading zeroes only in the allowed prefix and suffix", () => {
    expect(projectOrdinaryFieldValue("acct_no", "0000123456789012")).toBe("0000********9012");
    expect(projectOrdinaryFieldValue("交易卡号", "0000123456789012")).toBe("0000********9012");
  });

  it("fails closed instead of masking already-coerced numeric identifiers", () => {
    const roundedAccount = Number("6222020202020202020");
    expect(projectOrdinaryFieldValue("acct_no", roundedAccount)).toBe("[ACCOUNT]");
    expect(projectOrdinaryClipboardValue("card_no", roundedAccount)).toBe("[ACCOUNT]");
    expect(projectOrdinaryCsvField("account_number", roundedAccount)).toBe("[ACCOUNT]");
    expect(projectOrdinaryStructuredValue({ account_no: roundedAccount })).toEqual({
      account_no: "[ACCOUNT]",
    });
  });

  it("projects identity, phone, IP, and MAC fields deterministically", () => {
    expect(projectOrdinaryFieldValue("opener_id_no", "32031177070600123X")).toBe("320***********123X");
    expect(projectOrdinaryFieldValue("phone_no", "13800138000")).toBe("138****8000");
    expect(projectOrdinaryFieldValue("contact_phone", "13800138000")).toBe("138****8000");
    expect(projectOrdinaryFieldValue("agent_id_no", "32031177070600123X")).toBe("320***********123X");
    expect(projectOrdinaryFieldValue("ip_addr", "192.168.10.24")).toBe("192.168.*.*");
    expect(projectOrdinaryFieldValue("mac_addr", "AA:BB:CC:DD:EE:FF")).toBe("AA:BB:CC:**:**:**");
  });

  it("feeds only projected restricted values into ordinary transaction cells", () => {
    const cell = buildTxnDetailCell({
      column: { key: "counterparty_id_no" },
      raw: "32031177070600123X",
    });

    expect(cell.text).toBe("320***********123X");
  });

  it("uses the same masked projection for ordinary copy-like detected values", () => {
    expect(projectOrdinaryClipboardValue("card_no", "0000123456789012")).toBe("0000********9012");
    expect(projectDetectedOrdinaryRestrictedPii("0000123456789012")).toBe("0000********9012");
    expect(projectDetectedOrdinaryRestrictedPii("账号 0000123456789012，IP 192.168.10.24")).toBe(
      "账号 0000********9012，IP 192.168.*.*"
    );
    expect(projectOrdinaryFieldValue("remark", "联系 13800138000，账号 0000123456789012")).toBe(
      "联系 138****8000，账号 0000********9012"
    );
    expect(projectDetectedOrdinaryRestrictedPii("plain node label")).toBe("plain node label");
    expect(projectOrdinaryFieldValue("txn_time", "2026-07-15")).toBe("2026-07-15");
  });

  it("uses exact aliases after Unicode and camelCase tokenization", () => {
    expect(normalizeOrdinaryPresentationFieldName("counterpartyAccount")).toBe("counterparty_account");
    expect(normalizeOrdinaryPresentationFieldName("card\u200bNo")).toBe("card_no");
    expect(normalizeOrdinaryPresentationFieldName("ｃａｒｄＮｏ")).toBe("card_no");
    expect(resolveRestrictedPiiKind("counterpartyAccount")).toBe("bank-account");
    expect(resolveRestrictedPiiKind("description")).toBeNull();
    expect(resolveRestrictedPiiKind("membership")).toBeNull();
    expect(projectOrdinaryFieldValue("counterpartyAccount", "6214600780000579708")).toBe("6214***********9708");
    expect(projectOrdinaryFieldValue("fcSubAccount", "6214600780000579708")).toBe("6214***********9708");
  });

  it("does not let untrusted field labels exempt account-like values", () => {
    expect(projectOrdinaryFieldValue("amount", "6214600780000579708")).toBe("6214***********9708");
    expect(projectOrdinaryFieldValue("totalAmount", "6.2146007800005797e+18")).toBe("6.2146007800005797e+18");
    expect(projectOrdinaryFieldValue("txnTime", "2026-07-15 12:34:56")).toBe("2026-07-15 12:34:56");
    expect(projectOrdinaryFieldValue("availableBalance", "6214600780000579708")).toBe("6214***********9708");
    expect(projectOrdinaryFieldValue("lastTxnTime", "20260715123456")).toBe("20260715123456");
    expect(projectOrdinaryFieldValue("description", "记录 6214600780000579708 元")).toBe(
      "记录 6214***********9708 元"
    );
    expect(projectOrdinaryFieldValue("description", "6214600780000579708")).toBe("6214***********9708");
    expect(projectOrdinaryFieldValue("lastTxnTime", "62146007800005")).not.toContain("62146007800005");
    expect(projectDetectedOrdinaryRestrictedPii("6214600780000579708")).toBe("6214***********9708");
    expect(projectDetectedOrdinaryRestrictedPii("图6214600780000579708")).toBe("图6214***********9708");
    expect(projectDetectedOrdinaryRestrictedPii("6.2146007800005797e+18")).toBe("6.2146007800005797e+18");
    expect(projectDetectedOrdinaryRestrictedPii("金额：6.2146007800005797e+18")).toBe(
      "金额：6.2146007800005797e+18"
    );
    expect(projectDetectedOrdinaryRestrictedPii("账号：6.2146007800005797e+18")).not.toContain(
      "6.2146007800005797e+18"
    );
  });

  it("uses the stricter identity projection for ambiguous evidence identifiers", () => {
    expect(projectOrdinaryFieldValue("idOrAcctNo", "32031177070600123X")).toBe("320***********123X");
  });

  it("closes fullwidth, separator, format-character, and formula-prefix bypasses", () => {
    expect(projectOrdinaryFieldValue("card\u200bNo", "６２１４\u200b６００７８００００５７９７０８")).toBe(
      "6214***********9708"
    );
    expect(projectDetectedOrdinaryRestrictedPii("账\u200b号：6214－6007－8000－0579－708")).not.toContain(
      "6214－6007－8000－0579－708"
    );
    expect(projectDetectedOrdinaryRestrictedPii("身份证 3203\u200b1177070600123X")).not.toContain(
      "32031177070600123X"
    );
    expect(projectDetectedOrdinaryRestrictedPii("٦٢١٤٦٠٠٧٨٠٠٠٠٥٧٩٧٠٨")).not.toContain(
      "٦٢١٤٦٠٠٧٨٠٠٠٠٥٧٩٧٠٨"
    );
    expect(protectOrdinaryCsvFormula("\u200b=HYPERLINK(\"https://example.invalid\")")).toBe(
      "'\u200b=HYPERLINK(\"https://example.invalid\")"
    );
  });

  it("projects nested ordinary diagnostic copies without mutating their raw objects", () => {
    const raw = {
      account_no: "0000123456789012",
      nested: {
        nodeId: "账号 6214600780000579708",
        metrics: 123456789,
      },
    };

    expect(projectOrdinaryStructuredValue(raw)).toEqual({
      account_no: "0000********9012",
      nested: {
        nodeId: "账号 6214***********9708",
        metrics: 123456789,
      },
    });
    expect(raw.account_no).toBe("0000123456789012");
    expect(raw.nested.nodeId).toBe("账号 6214600780000579708");
  });

  it("neutralizes spreadsheet formulas after applying field projection", () => {
    expect(protectOrdinaryCsvFormula("=HYPERLINK(\"https://example.invalid\")")).toBe("'=HYPERLINK(\"https://example.invalid\")");
    expect(protectOrdinaryCsvFormula("\t+cmd|' /C calc'!A0")).toBe("'\t+cmd|' /C calc'!A0");
    expect(projectOrdinaryCsvField("acct_no", "0000123456789012")).toBe("0000********9012");
  });

  it("fails closed when no controlled export authority can be proven", () => {
    expect(uncontrolledRestrictedPiiExportBlockReason()).toContain("已安全阻止");
  });
});
