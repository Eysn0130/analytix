import { afterEach, describe, expect, it } from "vitest";

import {
  clearStagedFlowTransferPayload,
  normalizeFlowTransferPayload,
  readFlowTransferPayload,
  stageFlowTransferPayload,
} from "./flow-page-transfer";

function expectAliasesOmitted(payload: Record<string, unknown>, camelKey: string, snakeKey: string): void {
  expect(Object.prototype.hasOwnProperty.call(payload, camelKey)).toBe(false);
  expect(Object.prototype.hasOwnProperty.call(payload, snakeKey)).toBe(false);
}

afterEach(() => {
  clearStagedFlowTransferPayload();
});

describe("flow page transfer missing-value boundary", () => {
  it("omits missing, null, coercible, and non-finite expected amounts while retaining finite fractions and zero", () => {
    for (const value of [undefined, null, Number.NaN, Number.POSITIVE_INFINITY, "0"]) {
      const normalized = normalizeFlowTransferPayload({
        expectedTotalAmount: value,
        expected_total_amount: value,
      });
      expectAliasesOmitted(normalized, "expectedTotalAmount", "expected_total_amount");
    }
    expectAliasesOmitted(normalizeFlowTransferPayload({}), "expectedTotalAmount", "expected_total_amount");

    expect(normalizeFlowTransferPayload({ expectedTotalAmount: 1.25 })).toMatchObject({
      expectedTotalAmount: 1.25,
      expected_total_amount: 1.25,
    });
    expect(normalizeFlowTransferPayload({ expected_total_amount: 0 })).toMatchObject({
      expectedTotalAmount: 0,
      expected_total_amount: 0,
    });
  });

  it("requires expected row counts to be explicit nonnegative safe integers without rounding", () => {
    for (const value of [undefined, null, Number.NaN, Number.POSITIVE_INFINITY, "0", 1.5, -1]) {
      const normalized = normalizeFlowTransferPayload({
        expectedRowCount: value,
        expected_row_count: value,
      });
      expectAliasesOmitted(normalized, "expectedRowCount", "expected_row_count");
    }
    expectAliasesOmitted(normalizeFlowTransferPayload({}), "expectedRowCount", "expected_row_count");

    expect(normalizeFlowTransferPayload({ expectedRowCount: 0 })).toMatchObject({
      expectedRowCount: 0,
      expected_row_count: 0,
    });
    expect(normalizeFlowTransferPayload({ expected_row_count: 12 })).toMatchObject({
      expectedRowCount: 12,
      expected_row_count: 12,
    });
  });

  it("omits invalid or conflicting aliases instead of coercing them to false or another fact", () => {
    for (const value of [undefined, null, "false", 0, 1]) {
      const normalized = normalizeFlowTransferPayload({
        focusUnknownName: value,
        focus_unknown_name: value,
      });
      expectAliasesOmitted(normalized, "focusUnknownName", "focus_unknown_name");
    }

    const conflict = normalizeFlowTransferPayload({
      expectedTotalAmount: 0,
      expected_total_amount: 1,
      focusOnly: true,
      focus_only: false,
    });
    expectAliasesOmitted(conflict, "expectedTotalAmount", "expected_total_amount");
    expectAliasesOmitted(conflict, "focusOnly", "focus_only");

    const explicit = normalizeFlowTransferPayload({
      focusOnly: true,
      focusUnknownName: false,
      includeMissingCounterparty: false,
      focusCounterpartyStrict: true,
    });
    expect(explicit).toMatchObject({
      focusOnly: true,
      focus_only: true,
      focusUnknownName: false,
      focus_unknown_name: false,
      includeMissingCounterparty: false,
      include_missing_counterparty: false,
      focusCounterpartyStrict: true,
      focus_counterparty_strict: true,
    });
  });

  it("keeps explicit zero and false through the one-shot in-memory handoff", () => {
    stageFlowTransferPayload({
      expectedTotalAmount: 0,
      expectedRowCount: 0,
      focusOnly: true,
      focusUnknownName: false,
      includeMissingCounterparty: false,
      focusCounterpartyStrict: false,
    });

    expect(readFlowTransferPayload()).toMatchObject({
      expectedTotalAmount: 0,
      expected_total_amount: 0,
      expectedRowCount: 0,
      expected_row_count: 0,
      focusUnknownName: false,
      includeMissingCounterparty: false,
      focusCounterpartyStrict: false,
    });
  });
});
