import { describe, expect, it } from "vitest";

import type { BackendHealth } from "../../../services/http/system";
import { getCleaningRuntimeUnavailableMessage } from "./health";

function health(overrides: Record<string, unknown>): BackendHealth {
  return {
    status: "ok",
    service: "analytix-data-analysis",
    version: "test",
    env: "test",
    phase: "ready",
    uptime_s: 1,
    checks: {},
    ...overrides,
  } as BackendHealth;
}

describe("cleaning runtime health boundary", () => {
  it("keeps an absent health response unavailable", () => {
    expect(getCleaningRuntimeUnavailableMessage(null)).toContain("尚未确认");
  });

  it.each([undefined, null, false, "true", 1, {}, []])(
    "treats non-exact native availability %p as unavailable",
    (cleaning_native_available) => {
      expect(
        getCleaningRuntimeUnavailableMessage(
          health({ cleaning_native_available, cleaning_native_reason: "native_binary_not_found" }),
        ),
      ).toContain("Rust 清洗运行时不可用");
    },
  );

  it("accepts only an exact native or explicitly authorized legacy availability", () => {
    expect(getCleaningRuntimeUnavailableMessage(health({ cleaning_native_available: true }))).toBe("");
    expect(
      getCleaningRuntimeUnavailableMessage(
        health({ cleaning_native_available: null, legacy_python_cleaning_allowed: true }),
      ),
    ).toBe("");
  });

  it.each([undefined, null, false, "true", 1, {}, []])(
    "does not treat non-exact legacy authorization %p as available",
    (legacy_python_cleaning_allowed) => {
      expect(
        getCleaningRuntimeUnavailableMessage(
          health({ cleaning_native_available: false, legacy_python_cleaning_allowed }),
        ),
      ).toContain("Rust 清洗运行时不可用");
    },
  );
});
