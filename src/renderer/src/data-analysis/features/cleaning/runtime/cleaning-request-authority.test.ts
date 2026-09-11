import { describe, expect, it } from "vitest";

import { CleaningRequestAuthority } from "./cleaning-request-authority";

describe("CleaningRequestAuthority", () => {
  it("rejects a late case A response after case B becomes current", () => {
    const authority = new CleaningRequestAuthority();
    authority.bindCase("case-a");
    const caseA = authority.issue("context");

    authority.bindCase("case-b");
    const caseB = authority.issue("context");

    expect(authority.accepts(caseB)).toBe(true);
    expect(authority.accepts(caseA)).toBe(false);
  });

  it("rejects an older request from the same case and channel", () => {
    const authority = new CleaningRequestAuthority();
    authority.bindCase("case-a");
    const older = authority.issue("jobs");
    const newer = authority.issue("jobs");

    expect(authority.accepts(newer)).toBe(true);
    expect(authority.accepts(older)).toBe(false);
  });

  it("keeps context and job request sequences independent", () => {
    const authority = new CleaningRequestAuthority();
    authority.bindCase("case-a");
    const context = authority.issue("context");
    const jobs = authority.issue("jobs");

    expect(authority.accepts(context)).toBe(true);
    expect(authority.accepts(jobs)).toBe(true);
  });
});
