import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { SankeyChart } from "./flow/SankeyChart";
import { RemarkKeywordCloud } from "./keyword/RemarkKeywordCloud";

describe("chart fact rendering boundary", () => {
  it("keeps a verified zero keyword and discards invalid tooltip counts", () => {
    const html = renderToStaticMarkup(createElement(RemarkKeywordCloud, {
      items: [
        { label: "invalid", count: false as unknown as number },
        { label: "zero", count: 0 },
      ],
      panelId: "structure",
      activeToken: null,
      onSelect: vi.fn(),
    }));

    expect(html).toContain("zero · 命中 0 次");
    expect(html).not.toContain("invalid");
  });

  it("does not render an invalid Sankey amount as zero", () => {
    const html = renderToStaticMarkup(createElement(SankeyChart, {
      centerLabel: "当前账户",
      inboundItems: [
        { label: "invalid", amount: false, count: 1 },
        { label: "zero", amount: 0, count: 0 },
      ],
      outboundItems: [],
      direction: "in",
      activeToken: null,
      onSelect: vi.fn(),
    }));

    expect(html).toContain("¥0.00");
    expect(html).toContain("zero");
    expect(html).not.toContain("invalid");
  });
});
