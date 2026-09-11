import { describe, expect, it } from "vitest";

import flowBridgeSource from "./flow-bridge-service.ts?raw";
import { shouldUseDirectBuildPath } from "./flow-direct-build-cache";

function directPayload(): Record<string, unknown> {
  return {
    case_id: "case-a",
    seeds: ["seed-a"],
    depth: 1,
    direction: "both",
    min_amount: 0,
    source: "stats",
    focus_only: true,
    focus_unknown_name: false,
    include_missing_counterparty: false,
    focus_counterparty_strict: false,
    expected_total_amount: 0,
    expected_row_count: 0,
  };
}

describe("flow direct-build missing-value boundary", () => {
  it("accepts explicit zero facts and false strategies without rewriting them", () => {
    const payload = directPayload();
    expect(shouldUseDirectBuildPath(payload)).toBe(true);
  });

  it("rejects missing, null, non-finite, string, fractional, negative, and unsafe expected facts", () => {
    for (const value of [undefined, null, Number.NaN, Number.POSITIVE_INFINITY, "0"]) {
      expect(shouldUseDirectBuildPath({ ...directPayload(), expected_total_amount: value })).toBe(false);
    }
    for (const value of [
      undefined,
      null,
      Number.NaN,
      Number.POSITIVE_INFINITY,
      "0",
      1.5,
      -1,
      Number.MAX_SAFE_INTEGER + 1,
      10_001,
    ]) {
      expect(shouldUseDirectBuildPath({ ...directPayload(), expected_row_count: value })).toBe(false);
    }
    expect(shouldUseDirectBuildPath({ ...directPayload(), expected_total_amount: 1.25 })).toBe(true);
  });

  it("requires an explicit finite nonnegative min amount", () => {
    for (const value of [undefined, null, Number.NaN, Number.POSITIVE_INFINITY, "0", -1]) {
      expect(shouldUseDirectBuildPath({ ...directPayload(), min_amount: value })).toBe(false);
    }
    expect(shouldUseDirectBuildPath({ ...directPayload(), min_amount: 0 })).toBe(true);
    expect(shouldUseDirectBuildPath({ ...directPayload(), min_amount: 0.25 })).toBe(true);
  });

  it("rejects missing or partially invalid required seed facts", () => {
    for (const seeds of [undefined, null, [], [""], ["seed-a", null], "seed-a"]) {
      expect(shouldUseDirectBuildPath({ ...directPayload(), seeds })).toBe(false);
    }
    expect(shouldUseDirectBuildPath({ ...directPayload(), seeds: ["seed-a", "seed-a"] })).toBe(true);
  });

  it("requires every direct-build strategy boolean to have an explicit boolean type", () => {
    const fields = [
      "focus_only",
      "focus_unknown_name",
      "include_missing_counterparty",
      "focus_counterparty_strict",
    ];
    for (const field of fields) {
      for (const value of [undefined, null, "false", 0, 1]) {
        expect(shouldUseDirectBuildPath({ ...directPayload(), [field]: value })).toBe(false);
      }
    }

    for (const field of fields.slice(1)) {
      expect(shouldUseDirectBuildPath({ ...directPayload(), [field]: true })).toBe(true);
      expect(shouldUseDirectBuildPath({ ...directPayload(), [field]: false })).toBe(true);
    }
    expect(shouldUseDirectBuildPath({ ...directPayload(), focus_only: false })).toBe(false);
  });

  it("keeps direct factual results request-local with no renderer cache or inflight dedup", () => {
    expect(flowBridgeSource).not.toContain("getSharedFlowDirectBuildStore");
    expect(flowBridgeSource).not.toContain("ownedDirectBuilds");
    expect(flowBridgeSource).not.toContain("buildDirectBuildCacheKey");
    expect(flowBridgeSource).not.toContain("sharedStore.inflight");
    expect(flowBridgeSource).not.toContain("sharedStore.cache");
  });
});
