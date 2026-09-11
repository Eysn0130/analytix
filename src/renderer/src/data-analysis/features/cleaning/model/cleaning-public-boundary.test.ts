import { describe, expect, it } from "vitest";
import cleaningPageSource from "../CleaningPage.tsx?raw";

import {
  normalizeCleaningJobBoundary,
  normalizeCleaningStepDetailBoundary,
  normalizeCleaningStepSummaryBoundary
} from "../../../services/cleaning/api";
import { formatCount } from "./formatters";
import {
  buildCleaningStepDetailFooterViewModel,
  buildCleaningStepImpactViewModel,
  buildCleaningStepViewModels
} from "./view-model";

const CASE_ID = "case-cleaning-a";
const FULL_ACCOUNT = "6222020202020202020";

describe("cleaning ordinary evidence boundary", () => {
  it("mounts the typed deterministic producer without restoring legacy cleaning mutation", () => {
    expect(cleaningPageSource).toContain("window.analytix.runtime.runDeterministicFundsCleaning()");
    expect(cleaningPageSource).toContain("<CleaningDiffPreview selector={deterministicCleaning.selector}");
    expect(cleaningPageSource).toContain("onRunCleaning={onRunDeterministicCleaning}");
    expect(cleaningPageSource).not.toContain("onRunCleaning={onRunCleaning}");
    expect(cleaningPageSource).not.toMatch(/setJobs\([^)]*deterministicCleaning/);
  });

  it("strips hostile job result counts and summaries", () => {
    const job = normalizeCleaningJobBoundary({
      job_id: "job-a",
      case_id: CASE_ID,
      status: "succeeded",
      progress: 100,
      cleaned_rows: 2645472,
      summary: { account_no: FULL_ACCOUNT },
      fact_answer_allowed: true,
      created_at: "2026-07-18T00:00:00Z",
      updated_at: "2026-07-18T00:01:00Z"
    }, CASE_ID);

    expect(job.cleaned_rows).toBeNull();
    expect(job.summary).toEqual({});
    expect(job.fact_answer_allowed).toBe(false);
    expect(JSON.stringify(job)).not.toContain(FULL_ACCOUNT);
  });

  it("strips hostile counts, raw rows, totals and self-reported permission", () => {
    const summaries = normalizeCleaningStepSummaryBoundary({
      contract: "hostile",
      case_id: CASE_ID,
      fact_answer_allowed: true,
      items: [{
        step: 1,
        key: "step-1",
        title: "步骤一",
        kind: "修正",
        description: "desc",
        affected_rows: 2645472,
        account_no: FULL_ACCOUNT
      }]
    }, CASE_ID);
    const detail = normalizeCleaningStepDetailBoundary({
      case_id: CASE_ID,
      step: 1,
      fact_answer_allowed: true,
      raw_details_exposed: true,
      key: "step-1",
      title: "步骤一",
      kind: "修正",
      description: "desc",
      headers: ["交易卡号"],
      items: [{ values: [FULL_ACCOUNT] }],
      page: { total: 2645472 }
    }, CASE_ID, 1);

    expect(summaries.fact_answer_allowed).toBe(false);
    expect(summaries.items[0]?.affected_rows).toBeNull();
    expect(JSON.stringify(summaries)).not.toContain(FULL_ACCOUNT);
    expect(detail.fact_answer_allowed).toBe(false);
    expect(detail.raw_details_exposed).toBe(false);
    expect(detail.items).toEqual([]);
    expect(detail.page).toBeNull();
    expect(JSON.stringify(detail)).not.toContain(FULL_ACCOUNT);
  });

  it("never renders missing evidence as zero or no-hit", () => {
    const summaries = normalizeCleaningStepSummaryBoundary({
      case_id: CASE_ID,
      items: [{
        step: 1,
        key: "step-1",
        title: "步骤一",
        kind: "修正",
        description: "desc",
        affected_rows: 0
      }]
    }, CASE_ID);
    const steps = buildCleaningStepViewModels(summaries.items);
    const impact = buildCleaningStepImpactViewModel(steps);

    expect(formatCount(null)).toBe("--");
    expect(steps[0]?.affectedRows).toBeNull();
    expect(steps[0]?.affectedRowsLabel).toBe("--");
    expect(steps[0]?.stateLabel).toBe("待证据");
    expect(steps.map((item) => item.stateLabel)).not.toContain("无命中");
    expect(impact.txnStepAffected).toBeNull();
    expect(impact.accountStepAffected).toBeNull();
  });

  it("rejects cross-case and cross-step responses", () => {
    expect(() => normalizeCleaningStepSummaryBoundary({ case_id: "case-b", items: [] }, CASE_ID))
      .toThrow("cleaning_case_binding_mismatch");
    expect(() => normalizeCleaningStepDetailBoundary({ case_id: CASE_ID, step: 2 }, CASE_ID, 1))
      .toThrow("cleaning_case_binding_mismatch");
  });

  it("uses a fixed evidence boundary instead of an empty-result claim", () => {
    const detail = normalizeCleaningStepDetailBoundary({
      case_id: CASE_ID,
      step: 1,
      title: "步骤一"
    }, CASE_ID, 1);
    const footer = buildCleaningStepDetailFooterViewModel(detail);

    expect(footer?.totalCount).toBeNull();
    expect(footer?.label).toContain("证据回执");
    expect(footer?.label).not.toContain("共 0 条");
  });
});
