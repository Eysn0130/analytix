import { z } from "zod";

// Pure data contracts. sourceId/digests do not establish provenance, identity,
// permission, a session, or a persistence transaction.
export const MaxCanvasSceneBytes = 4 << 20;
export const MaxCanvasNodes = 256;
export const MaxCanvasEdges = 1024;
export const MaxCanvasOperations = 128;
export const MaxCanvasImagePixels = 16 << 20;
export const MaxCanvasImageDimension = 8192;
export const MaxCanvasImageBytes = 32 << 20;
const utf8 = new TextEncoder();
function validUnicode(value: string): boolean {
  for (let i = 0; i < value.length; i++) {
    const c = value.charCodeAt(i);
    if (c >= 0xd800 && c <= 0xdbff) {
      const next = value.charCodeAt(++i);
      if (!(next >= 0xdc00 && next <= 0xdfff)) return false;
    } else if (c >= 0xdc00 && c <= 0xdfff) return false;
  }
  return true;
}
const text = (maximum: number) =>
  z
    .string()
    .refine(
      (value) =>
        validUnicode(value) &&
        !value.includes("\0") &&
        utf8.encode(value).length <= maximum,
    );
const id = z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/);
const color = z.string().regex(/^#[0-9A-Fa-f]{6}$/);
const coordinate = z.number().finite().min(-1000000).max(1000000);
const scalar = z.union([
  text(2048),
  z
    .number()
    .finite()
    .min(-Number.MAX_SAFE_INTEGER)
    .max(Number.MAX_SAFE_INTEGER),
  z.boolean(),
  z.null(),
]);
const attributes = z
  .record(z.string().regex(/^[A-Za-z][A-Za-z0-9_-]{0,63}$/), scalar)
  .refine((value) => Object.keys(value).length <= 32);
export const canvasSourceRefSchema = z
  .object({
    sourceId: id,
    locator: text(1024).optional(),
    note: text(2048).optional(),
  })
  .strict();
const sources = z
  .array(canvasSourceRefSchema)
  .max(64)
  .refine(
    (value) =>
      new Set(value.map((ref) => `${ref.sourceId}\0${ref.locator ?? ""}`))
        .size === value.length,
  );
export const canvasNodeSchema = z
  .object({
    id,
    label: text(1024),
    attributes,
    sources,
    assumption: z.boolean(),
  })
  .strict();
export const canvasEdgeSchema = z
  .object({
    id,
    from: id,
    to: id,
    label: text(1024),
    relation: text(128).refine((value) => value.length > 0),
    attributes,
    sources,
    assumption: z.boolean(),
  })
  .strict();
export const canvasPointSchema = z
  .object({ x: coordinate, y: coordinate })
  .strict();
export const canvasLayoutSchema = z
  .object({
    x: coordinate,
    y: coordinate,
    width: coordinate.positive(),
    height: coordinate.positive(),
  })
  .strict()
  .refine(
    (value) =>
      Math.abs(value.x + value.width) <= 1000000 &&
      Math.abs(value.y + value.height) <= 1000000,
  );
const points = z
  .array(canvasPointSchema)
  .max(32)
  .refine((value) => value.length === 0 || value.length >= 2);
export const canvasStyleSchema = z
  .object({
    fill: color,
    stroke: color,
    strokeWidth: z.number().int().min(1).max(16),
    dash: z.enum(["solid", "dashed"]),
    shape: z.enum(["rectangle", "rounded", "ellipse", "line"]),
  })
  .strict();
const nodeStyle = canvasStyleSchema.refine((value) => value.shape !== "line");
const edgeStyle = canvasStyleSchema.refine((value) => value.shape === "line");
export const canvasFactsSchema = z
  .object({
    nodes: z.array(canvasNodeSchema).max(MaxCanvasNodes),
    edges: z.array(canvasEdgeSchema).max(MaxCanvasEdges),
  })
  .strict()
  .superRefine((facts, context) => {
    const nodes = new Set(facts.nodes.map((node) => node.id)),
      edges = new Set(facts.edges.map((edge) => edge.id));
    if (
      nodes.size !== facts.nodes.length ||
      edges.size !== facts.edges.length ||
      facts.edges.some(
        (edge) =>
          nodes.has(edge.id) || !nodes.has(edge.from) || !nodes.has(edge.to),
      )
    ) {
      context.addIssue({
        code: "custom",
        message: "Duplicate IDs or unknown edge endpoint",
      });
    }
  });
export const canvasSceneSchema = z
  .object({
    schemaVersion: z.literal(1),
    facts: canvasFactsSchema,
    presentation: z
      .object({
        nodes: z
          .array(
            z
              .object({
                id,
                layout: canvasLayoutSchema,
                displayLabel: text(1024),
                style: nodeStyle,
              })
              .strict(),
          )
          .max(MaxCanvasNodes),
        edges: z
          .array(
            z
              .object({
                id,
                points,
                displayLabel: text(1024),
                style: edgeStyle,
              })
              .strict(),
          )
          .max(MaxCanvasEdges),
      })
      .strict(),
  })
  .strict()
  .superRefine((scene, context) => {
    for (const target of ["nodes", "edges"] as const) {
      const factIds = new Set(scene.facts[target].map((item) => item.id)),
        viewIds = new Set(scene.presentation[target].map((item) => item.id));
      if (
        viewIds.size !== scene.presentation[target].length ||
        factIds.size !== viewIds.size ||
        [...viewIds].some((value) => !factIds.has(value))
      ) {
        context.addIssue({
          code: "custom",
          message: "Presentation must map each fact ID exactly once",
        });
      }
    }
    if (utf8.encode(JSON.stringify(scene)).length > MaxCanvasSceneBytes)
      context.addIssue({
        code: "custom",
        message: "Scene exceeds byte budget",
      });
  });
export const canvasOperationSchema = z
  .discriminatedUnion("kind", [
    z
      .object({
        kind: z.literal("set-node-layout"),
        id,
        layout: canvasLayoutSchema,
      })
      .strict(),
    z
      .object({
        kind: z.literal("set-edge-route"),
        id,
        points: z.array(canvasPointSchema).min(2).max(32),
      })
      .strict(),
    z
      .object({
        kind: z.literal("set-display-label"),
        id,
        target: z.enum(["node", "edge"]),
        displayLabel: text(1024),
      })
      .strict(),
    z
      .object({
        kind: z.literal("set-style"),
        id,
        target: z.enum(["node", "edge"]),
        style: canvasStyleSchema,
      })
      .strict(),
  ])
  .refine(
    (op) =>
      op.kind !== "set-style" ||
      (op.target === "edge"
        ? op.style.shape === "line"
        : op.style.shape !== "line"),
  );
export const canvasOperationsSchema = z
  .array(canvasOperationSchema)
  .min(1)
  .max(MaxCanvasOperations)
  .refine(
    (ops) =>
      new Set(ops.map((op) => `${op.kind}/${op.id}`)).size === ops.length,
  );
export const canvasChangeSchema = z.discriminatedUnion("field", [
  z
    .object({
      kind: z.literal("set-node-layout"),
      target: z.literal("node"),
      id,
      field: z.literal("layout"),
      factLabel: text(1024),
      before: canvasLayoutSchema,
      after: canvasLayoutSchema,
    })
    .strict(),
  z
    .object({
      kind: z.literal("set-edge-route"),
      target: z.literal("edge"),
      id,
      field: z.literal("points"),
      factLabel: text(1024),
      before: points,
      after: points,
    })
    .strict(),
  z
    .object({
      kind: z.literal("set-display-label"),
      target: z.enum(["node", "edge"]),
      id,
      field: z.literal("displayLabel"),
      factLabel: text(1024),
      before: text(1024),
      after: text(1024),
    })
    .strict(),
  z
    .object({
      kind: z.literal("set-style"),
      target: z.enum(["node", "edge"]),
      id,
      field: z.literal("style"),
      factLabel: text(1024),
      before: canvasStyleSchema,
      after: canvasStyleSchema,
    })
    .strict(),
]);
const digest = z.string().regex(/^[a-f0-9]{64}$/);
export const canvasResultSchema = z
  .object({
    scene: canvasSceneSchema,
    factsDigest: digest,
    diff: z.array(canvasChangeSchema).max(MaxCanvasOperations),
  })
  .strict();

export const canvasImageRegionSchema = z
  .object({
    x: z.number().int().min(0).max(8191),
    y: z.number().int().min(0).max(8191),
    width: z.number().int().min(1).max(8192),
    height: z.number().int().min(1).max(8192),
  })
  .strict();
export const canvasImageMarkSchema = z
  .object({
    id,
    shape: z.enum(["rectangle", "line"]),
    x1: z.number().int().min(0).max(8191),
    y1: z.number().int().min(0).max(8191),
    x2: z.number().int().min(0).max(8191),
    y2: z.number().int().min(0).max(8191),
    stroke: color,
    strokeWidth: z.number().int().min(1).max(16),
  })
  .strict()
  .refine(
    (mark) =>
      mark.shape !== "rectangle" || (mark.x1 < mark.x2 && mark.y1 < mark.y2),
  );
export const canvasImageOperationSchema = z.discriminatedUnion("kind", [
  z
    .object({ kind: z.literal("crop"), region: canvasImageRegionSchema })
    .strict(),
  z
    .object({
      kind: z.literal("rotate"),
      degrees: z.union([z.literal(90), z.literal(180), z.literal(270)]),
    })
    .strict(),
  z.object({ kind: z.literal("mark"), mark: canvasImageMarkSchema }).strict(),
]);
export const canvasImageOperationsSchema = z
  .array(canvasImageOperationSchema)
  .min(1)
  .max(MaxCanvasOperations)
  .refine((ops) => {
    const marks = ops.flatMap((op) => (op.kind === "mark" ? [op.mark.id] : []));
    return new Set(marks).size === marks.length;
  });
const dimensions = z
  .object({
    width: z.number().int().min(1).max(8192),
    height: z.number().int().min(1).max(8192),
  })
  .strict()
  .refine((value) => value.width * value.height <= MaxCanvasImagePixels);
export const canvasImageResultSchema = z
  .object({
    mime: z.literal("image/png"),
    sourceDigest: digest,
    digest,
    width: z.number().int().min(1).max(8192),
    height: z.number().int().min(1).max(8192),
    byteLength: z.number().int().min(1).max(MaxCanvasImageBytes),
    diff: z
      .array(
        z
          .object({
            operation: canvasImageOperationSchema,
            before: dimensions,
            after: dimensions,
          })
          .strict(),
      )
      .min(1)
      .max(MaxCanvasOperations),
  })
  .strict()
  .refine((result) => result.width * result.height <= MaxCanvasImagePixels);

export type CanvasScene = z.infer<typeof canvasSceneSchema>;
export type CanvasOperation = z.infer<typeof canvasOperationSchema>;
export type CanvasResult = z.infer<typeof canvasResultSchema>;
export type CanvasImageOperation = z.infer<typeof canvasImageOperationSchema>;
export type CanvasImageResult = z.infer<typeof canvasImageResultSchema>;

// Exact facts digest preimage shared with Go json.Marshal: stable ID/ref sorting,
// ASCII attribute-key sorting, JSON HTML/U+2028/U+2029 escaping, no trailing LF.
// Hash these UTF-8 bytes with SHA-256. This computes content, not authority.
export function canvasFactsDigestInput(input: CanvasScene): Uint8Array {
  const scene = canvasSceneSchema.parse(input);
  const compareUTF8 = (a: string, b: string): number => {
    const left = utf8.encode(a),
      right = utf8.encode(b);
    for (let i = 0; i < Math.min(left.length, right.length); i++)
      if (left[i] !== right[i]) return left[i] - right[i];
    return left.length - right.length;
  };
  const refs = (sources: z.infer<typeof canvasSourceRefSchema>[]) =>
    [...sources]
      .sort((a, b) =>
        a.sourceId < b.sourceId
          ? -1
          : a.sourceId > b.sourceId
            ? 1
            : compareUTF8(a.locator ?? "", b.locator ?? ""),
      )
      .map((ref) => ({
        sourceId: ref.sourceId,
        ...(ref.locator ? { locator: ref.locator } : {}),
        ...(ref.note ? { note: ref.note } : {}),
      }));
  const attrs = (value: Record<string, z.infer<typeof scalar>>) =>
    Object.fromEntries(
      Object.entries(value).sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0)),
    );
  const facts = {
    nodes: [...scene.facts.nodes]
      .sort((a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0))
      .map((node) => ({
        id: node.id,
        label: node.label,
        attributes: attrs(node.attributes),
        sources: refs(node.sources),
        assumption: node.assumption,
      })),
    edges: [...scene.facts.edges]
      .sort((a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0))
      .map((edge) => ({
        id: edge.id,
        from: edge.from,
        to: edge.to,
        label: edge.label,
        relation: edge.relation,
        attributes: attrs(edge.attributes),
        sources: refs(edge.sources),
        assumption: edge.assumption,
      })),
  };
  const escaped = JSON.stringify(facts).replace(
    /[<>&\u2028\u2029]/g,
    (character) =>
      `\\u${character.charCodeAt(0).toString(16).padStart(4, "0")}`,
  );
  return utf8.encode(`AnalytixCanvasFactsV1\0${escaped}`);
}
