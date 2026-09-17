import { createHash } from "node:crypto";
import { describe, expect, it } from "vitest";
import {
  canvasFactsDigestInput,
  canvasImageOperationsSchema,
  canvasImageResultSchema,
  canvasOperationsSchema,
  canvasResultSchema,
  canvasSceneSchema,
  type CanvasScene,
} from "./canvas-editing";

function scene(): CanvasScene {
  const nodeStyle = {
    fill: "#ffffff",
    stroke: "#102030",
    strokeWidth: 2,
    dash: "solid",
    shape: "rounded",
  } as const;
  const edgeStyle = {
    fill: "#ffffff",
    stroke: "#102030",
    strokeWidth: 1,
    dash: "dashed",
    shape: "line",
  } as const;
  return {
    schemaVersion: 1,
    facts: {
      nodes: [
        {
          id: "n2",
          label: "Concept",
          attributes: {},
          sources: [],
          assumption: true,
        },
        {
          id: "n1",
          label: "事实 <>&\u2028",
          attributes: { z: null, amount: 0.25, active: true },
          sources: [
            { sourceId: "s1", locator: "😀", note: "astral" },
            { sourceId: "s1", locator: "\ue000", note: "BMP" },
            { sourceId: "s2", locator: "p:2", note: "" },
          ],
          assumption: false,
        },
      ],
      edges: [
        {
          id: "e2",
          from: "n1",
          to: "n2",
          label: "Assumed",
          relation: "hypothesis",
          attributes: {},
          sources: [],
          assumption: true,
        },
        {
          id: "e1",
          from: "n1",
          to: "n2",
          label: "Transfer",
          relation: "transfer",
          attributes: { currency: "CNY" },
          sources: [
            { sourceId: "s3", locator: "row:7", note: "source reference only" },
          ],
          assumption: false,
        },
      ],
    },
    presentation: {
      nodes: [
        {
          id: "n1",
          layout: { x: 0, y: 0, width: 100, height: 60 },
          displayLabel: "Original display",
          style: nodeStyle,
        },
        {
          id: "n2",
          layout: { x: 200, y: 50, width: 100, height: 60 },
          displayLabel: "Concept",
          style: nodeStyle,
        },
      ],
      edges: [
        { id: "e1", points: [], displayLabel: "Transfer", style: edgeStyle },
        {
          id: "e2",
          points: [
            { x: 0, y: 0 },
            { x: 200, y: 50 },
          ],
          displayLabel: "Assumed",
          style: edgeStyle,
        },
      ],
    },
  };
}
const hash = (value: CanvasScene) =>
  createHash("sha256").update(canvasFactsDigestInput(value)).digest("hex");

describe("Canvas facts and presentation contract", () => {
  it("matches the Go facts vector, including UTF-8 reference order and escaped factual text", () => {
    const value = scene(),
      original = structuredClone(value);
    expect(hash(value)).toBe(
      "c5a9d7aea6eef9439c40c12d224510bcba06eac4db2298732f09b5c543116bc2",
    );
    expect(value).toEqual(original);
    value.facts.nodes.reverse();
    value.facts.edges.reverse();
    value.facts.nodes[0].sources.reverse();
    value.presentation.nodes[0].layout.x = 100;
    value.presentation.nodes[0].displayLabel = "Edited display";
    expect(hash(value)).toBe(hash(original));
    value.facts.nodes[0].label = "Edited fact";
    expect(hash(value)).not.toBe(hash(original));
  });
  it("retains all structured sources and accepts bounded parallel edges", () => {
    const value = scene();
    value.facts.nodes[0].sources = Array.from({ length: 12 }, (_, n) => ({
      sourceId: `evidence-${n}`,
      locator: `row:${n}`,
      note: "retained",
    }));
    expect(canvasSceneSchema.parse(value)).toEqual(value);
    expect(canvasSceneSchema.parse(value).facts.nodes[0].sources).toHaveLength(
      12,
    );
  });
  it.each([
    [
      "missing presentation",
      (value: CanvasScene) => {
        value.presentation.edges.pop();
      },
    ],
    [
      "duplicate node",
      (value: CanvasScene) => {
        value.facts.nodes[0].id = "n1";
      },
    ],
    [
      "missing endpoint",
      (value: CanvasScene) => {
        value.facts.edges[0].to = "unknown";
      },
    ],
    [
      "duplicate source",
      (value: CanvasScene) => {
        value.facts.nodes[1].sources.push(value.facts.nodes[1].sources[0]);
      },
    ],
    [
      "oversized factual label",
      (value: CanvasScene) => {
        value.facts.nodes[0].label = "x".repeat(1025);
      },
    ],
    [
      "invalid Unicode",
      (value: CanvasScene) => {
        value.facts.nodes[0].label = "\ud800";
      },
    ],
    [
      "out of bounds",
      (value: CanvasScene) => {
        value.presentation.nodes[0].layout.x = 1000000;
      },
    ],
    [
      "injected style",
      (value: CanvasScene) => {
        value.presentation.nodes[0].style.fill = "url(file:///private)";
      },
    ],
  ])("rejects %s", (_name, change) => {
    const value = scene();
    change(value);
    expect(canvasSceneSchema.safeParse(value).success).toBe(false);
  });
  it("requires false, empty facts and view arrays explicitly and excludes invented authority", () => {
    const value = scene(),
      node = { ...value.facts.nodes[1] } as Record<string, unknown>;
    delete node.assumption;
    expect(
      canvasSceneSchema.safeParse({
        ...value,
        facts: { ...value.facts, nodes: [node] },
      }).success,
    ).toBe(false);
    expect(
      canvasSceneSchema.safeParse({ ...value, ownerToken: "invented" }).success,
    ).toBe(false);
    expect(
      canvasSceneSchema.safeParse({
        ...value,
        facts: { ...value.facts, edges: null },
      }).success,
    ).toBe(false);
  });
  it("accepts only finite, complete, stable-ID presentation operations", () => {
    const ops = [
      {
        kind: "set-node-layout",
        id: "n1",
        layout: { x: 1, y: 2, width: 100, height: 50 },
      },
      {
        kind: "set-edge-route",
        id: "e1",
        points: [
          { x: 1, y: 2 },
          { x: 10, y: 20 },
        ],
      },
      { kind: "set-display-label", id: "n1", target: "node", displayLabel: "" },
      {
        kind: "set-style",
        id: "e1",
        target: "edge",
        style: scene().presentation.edges[0].style,
      },
    ];
    expect(canvasOperationsSchema.parse(ops)).toEqual(ops);
    expect(canvasOperationsSchema.safeParse([ops[0], ops[0]]).success).toBe(
      false,
    );
    expect(
      canvasOperationsSchema.safeParse([{ ...ops[2], label: "new fact" }])
        .success,
    ).toBe(false);
    expect(
      canvasOperationsSchema.safeParse([
        { ...ops[0], layout: { x: 0, y: 0, width: 10 } },
      ]).success,
    ).toBe(false);
    expect(
      canvasOperationsSchema.safeParse([{ ...ops[3], target: "node" }]).success,
    ).toBe(false);
    expect(
      canvasOperationsSchema.safeParse([{ kind: "delete-node", id: "n1" }])
        .success,
    ).toBe(false);
  });
  it("describes the factual label separately in a display-only Diff", () => {
    const value = scene();
    const diff = [
      {
        kind: "set-display-label",
        target: "edge",
        id: "e1",
        field: "displayLabel",
        factLabel: "Transfer",
        before: "Transfer",
        after: "Display only",
      },
    ];
    value.presentation.edges[0].displayLabel = "Display only";
    expect(
      canvasResultSchema.parse({ scene: value, factsDigest: hash(value), diff })
        .diff,
    ).toEqual(diff);
    expect(
      canvasResultSchema.safeParse({
        scene: value,
        factsDigest: hash(value),
        diff: [{ ...diff[0], field: "label" }],
      }).success,
    ).toBe(false);
  });
});

describe("Canvas image data contract", () => {
  it("accepts natural integer crop, quarter turns and stable-ID marks", () => {
    const operations = [
      { kind: "crop", region: { x: 1, y: 0, width: 2, height: 2 } },
      { kind: "rotate", degrees: 90 },
      {
        kind: "mark",
        mark: {
          id: "m1",
          shape: "line",
          x1: 0,
          y1: 0,
          x2: 1,
          y2: 1,
          stroke: "#ff0000",
          strokeWidth: 1,
        },
      },
    ];
    expect(canvasImageOperationsSchema.parse(operations)).toEqual(operations);
    expect(
      canvasImageOperationsSchema.safeParse([operations[2], operations[2]])
        .success,
    ).toBe(false);
    expect(
      canvasImageOperationsSchema.safeParse([{ kind: "rotate", degrees: 45 }])
        .success,
    ).toBe(false);
    expect(
      canvasImageOperationsSchema.safeParse([
        { kind: "crop", region: { x: 0.5, y: 0, width: 2, height: 2 } },
      ]).success,
    ).toBe(false);
    expect(
      canvasImageOperationsSchema.safeParse([
        { kind: "crop", region: { y: 0, width: 2, height: 2 } },
      ]).success,
    ).toBe(false);
    expect(
      canvasImageOperationsSchema.safeParse([
        { ...operations[0], sourcePath: "/tmp/private" },
      ]).success,
    ).toBe(false);
  });
  it("carries bounded PNG metadata and dimension-aware operation Diff", () => {
    const value = {
      mime: "image/png",
      sourceDigest: "a".repeat(64),
      digest: "b".repeat(64),
      width: 2,
      height: 3,
      byteLength: 100,
      diff: [
        {
          operation: { kind: "rotate", degrees: 90 },
          before: { width: 3, height: 2 },
          after: { width: 2, height: 3 },
        },
      ],
    };
    expect(canvasImageResultSchema.parse(value)).toEqual(value);
    expect(
      canvasImageResultSchema.safeParse({ ...value, width: 8192, height: 8192 })
        .success,
    ).toBe(false);
    expect(
      canvasImageResultSchema.safeParse({
        ...value,
        outputPath: "/tmp/private",
      }).success,
    ).toBe(false);
  });
});
