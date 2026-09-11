import { afterEach, beforeEach, describe, expect, it } from "vitest";
import edgeQueryModelScript from "../../runtime/embedded/analysis_flow/graph-edge-txn-query-model.js?raw";
import transactionModelScript from "../../runtime/embedded/analysis_flow/graph-transaction-model.js?raw";

type TransactionModel = {
  normalizeAccountTxnRows: (rows: Array<Record<string, unknown>>) => Array<Record<string, unknown>>;
  matchInboundTxn: (rows: Array<Record<string, unknown>>, base: Record<string, unknown>) => Record<string, unknown> | null;
  computeDrillOutflows: (
    rows: Array<Record<string, unknown>>,
    base: Record<string, unknown>,
    config?: Record<string, unknown>
  ) => { outflows: Array<Record<string, unknown>> };
};

type EdgeQueryModel = {
  normalizeEdgeTxnCursorPageResult: (
    value: Record<string, unknown>,
    limit?: number
  ) => { ok: boolean; factAnswerAllowed: boolean; rows: unknown[]; done: boolean; nextCursor: unknown };
};

beforeEach(() => {
  Function(transactionModelScript)();
  Function(edgeQueryModelScript)();
});

afterEach(() => {
  delete (globalThis as Record<string, unknown>).__ANALYTIX_FLOW_GRAPH_TRANSACTION_MODEL__;
  delete (globalThis as Record<string, unknown>).__ANALYTIX_FLOW_EDGE_TXN_QUERY_MODEL__;
});

describe("flow transaction evidence model", () => {
  it("rejects missing amount or direction instead of inferring a zero-value inflow", () => {
    const model = (globalThis as Record<string, unknown>).__ANALYTIX_FLOW_GRAPH_TRANSACTION_MODEL__ as TransactionModel;
    const rows = model.normalizeAccountTxnRows([
      { txn_id: "missing-amount", amount: "", dc_flag: "进" },
      { txn_id: "invalid-amount", amount: "bad", dc_flag: "出" },
      { txn_id: "missing-direction", amount: 100, dc_flag: "" },
      { txn_id: "valid-zero", amount: 0, dc_flag: "进" },
      { txn_id: "valid-out", amount: 100, dc_flag: "出" },
    ]);

    expect(rows.map((row) => row.txn_id)).toEqual(["valid-zero", "valid-out"]);
    expect(rows[0]).toMatchObject({ amount: 0, __dir: "in" });
    expect(rows[1]).toMatchObject({ amount: 100, __dir: "out" });
  });

  it("cannot match or drill from a base transaction with missing amount", () => {
    const model = (globalThis as Record<string, unknown>).__ANALYTIX_FLOW_GRAPH_TRANSACTION_MODEL__ as TransactionModel;
    const rows = model.normalizeAccountTxnRows([{ txn_id: "in-1", amount: 100, dc_flag: "进" }]);
    expect(model.matchInboundTxn(rows, { txn_id: "in-1", amount: "" })).toBeNull();
    expect(model.computeDrillOutflows(rows, { txn_id: "in-1", amount: "" }).outflows).toEqual([]);
  });

  it("requires both semantic success and fact authority before exposing cursor rows", () => {
    const model = (globalThis as Record<string, unknown>).__ANALYTIX_FLOW_EDGE_TXN_QUERY_MODEL__ as EdgeQueryModel;
    const blocked = model.normalizeEdgeTxnCursorPageResult({
      ok: true,
      factAnswerAllowed: false,
      rows: [{ amount: 0 }],
      done: true,
      nextCursor: { offset: 1 },
    }, 200);
    expect(blocked).toMatchObject({ ok: false, factAnswerAllowed: false, rows: [], done: false, nextCursor: null });

    const verified = model.normalizeEdgeTxnCursorPageResult({
      ok: true,
      factAnswerAllowed: true,
      semanticStatus: "verified",
      rows: [{ amount: 100 }],
      done: true,
      nextCursor: null,
    }, 200);
    expect(verified).toMatchObject({ ok: true, factAnswerAllowed: true, rows: [{ amount: 100 }], done: true });
  });
});
