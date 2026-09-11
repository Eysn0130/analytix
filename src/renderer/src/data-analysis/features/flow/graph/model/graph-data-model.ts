import type {
  FlowGraphData,
  FlowGraphProjection,
  FlowPublicOpaqueSnapshotRefDTO,
  FlowRenderHints,
  FlowRuntimeGraphDTO,
  FlowSnapshotRefDTO,
  FlowViewDTO,
} from "../../../../services/analysis/flow-api-contracts";

function text(value: unknown): string {
  return String(value == null ? "" : value).trim();
}

function asObject(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : {};
}

function hasOwn(source: Record<string, unknown>, key: string): boolean {
  return Object.prototype.hasOwnProperty.call(source, key);
}

function strictAliasedNonNegativeSafeInteger(
  source: Record<string, unknown>,
  snakeKey: string,
  camelKey: string
): number | null {
  const values = [snakeKey, camelKey].filter((key) => hasOwn(source, key)).map((key) => source[key]);
  if (
    values.length === 0 ||
    values.some((value) => typeof value !== "number" || !Number.isSafeInteger(value) || value < 0) ||
    values.some((value) => value !== values[0])
  ) {
    return null;
  }
  return values[0] as number;
}

export function cloneFlowValue<T>(value: T): T {
  try {
    return structuredClone(value) as T;
  } catch {
    if (Array.isArray(value)) {
      return value.map((item) => cloneFlowValue(item)) as T;
    }
    if (value && typeof value === "object") {
      return Object.fromEntries(
        Object.entries(value as Record<string, unknown>).map(([key, item]) => [key, cloneFlowValue(item)])
      ) as T;
    }
    return value;
  }
}

export function asFlowSnapshotRef(value: unknown): FlowSnapshotRefDTO | null {
  const payload = asObject(value);
  const snapshotId = text(payload.snapshot_id || payload.snapshotId);
  const graphHash = text(payload.graph_hash || payload.graphHash);
  const nodeCount = strictAliasedNonNegativeSafeInteger(payload, "node_count", "nodeCount");
  const edgeCount = strictAliasedNonNegativeSafeInteger(payload, "edge_count", "edgeCount");
  const storedAt = text(payload.stored_at || payload.storedAt);
  if (!snapshotId || !graphHash || nodeCount == null || edgeCount == null || !storedAt) {
    return null;
  }
  return {
    snapshot_id: snapshotId,
    graph_hash: graphHash,
    node_count: nodeCount,
    edge_count: edgeCount,
    stored_at: storedAt,
  };
}

function asFlowPublicOpaqueSnapshotRef(value: unknown): FlowPublicOpaqueSnapshotRefDTO | null {
  const payload = asObject(value);
  const opaqueRef = text(payload.opaque_ref);
  if (
    payload.contract !== "FlowPublicOpaqueSnapshotRefV1" ||
    !/^flowref_v1_[a-f0-9]{64}$/u.test(opaqueRef) ||
    payload.content_access !== "controlled_artifact_required" ||
    Object.keys(payload).length !== 3
  ) {
    return null;
  }
  return {
    contract: "FlowPublicOpaqueSnapshotRefV1",
    opaque_ref: opaqueRef,
    content_access: "controlled_artifact_required",
  };
}

export function asRuntimeRevision(value: unknown): number | null {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0 ? value : null;
}

export function cloneRuntimeGraph(graph: FlowRuntimeGraphDTO | null | undefined): FlowRuntimeGraphDTO {
  const source = graph && typeof graph === "object" ? graph : { nodes: [], edges: [] };
  const runtimeRevision = asRuntimeRevision(source.runtime_revision);
  const payload: FlowRuntimeGraphDTO = {
    nodes: Array.isArray(source.nodes) ? source.nodes.map((item) => cloneFlowValue(item)) : [],
    edges: Array.isArray(source.edges) ? source.edges.map((item) => cloneFlowValue(item)) : [],
  };
  if (runtimeRevision != null) {
    payload.runtime_revision = runtimeRevision;
  }
  return payload;
}

export function cloneRenderHints(value: FlowRenderHints | Record<string, unknown> | null | undefined): FlowRenderHints | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return undefined;
  }
  return { ...(value as FlowRenderHints) };
}

export function cloneProjection(
  value: FlowGraphProjection | Record<string, unknown> | null | undefined
): FlowGraphProjection | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return undefined;
  }
  const payload = value as FlowGraphProjection;
  return {
    ...payload,
    source_result_snapshot_ref: asFlowSnapshotRef(payload.source_result_snapshot_ref),
    anchor_node_ids: Array.isArray(payload.anchor_node_ids) ? [...payload.anchor_node_ids] : undefined,
    expanded_cluster_ids: Array.isArray(payload.expanded_cluster_ids) ? [...payload.expanded_cluster_ids] : undefined,
    expanded_tile_ids: Array.isArray(payload.expanded_tile_ids) ? [...payload.expanded_tile_ids] : undefined,
    expanded_node_ids: Array.isArray(payload.expanded_node_ids) ? [...payload.expanded_node_ids] : undefined,
    path_node_ids: Array.isArray(payload.path_node_ids) ? [...payload.path_node_ids] : undefined,
    clusters: Array.isArray(payload.clusters) ? payload.clusters.map((item) => cloneFlowValue(item || {})) : undefined,
  };
}

export function cloneFlowGraphData(data: FlowGraphData): FlowGraphData {
  if (data.contract === "FlowPublicResultBoundaryV1") {
    return {
      contract: "FlowPublicResultBoundaryV1",
      publication_status: "blocked",
      fact_answer_allowed: false,
      content_access: "controlled_artifact_required",
      nodes: [],
      edges: [],
      stats: { ...(data.stats || {}) },
      runtime_graph: { nodes: [], edges: [] },
      result_snapshot_ref: asFlowPublicOpaqueSnapshotRef(data.result_snapshot_ref),
    };
  }
  return {
    nodes: Array.isArray(data.nodes) ? data.nodes.map((item) => cloneFlowValue(item)) : [],
    edges: Array.isArray(data.edges) ? data.edges.map((item) => cloneFlowValue(item)) : [],
    stats: { ...(data.stats || {}) },
    graph_tier: text(data.graph_tier),
    render_hints: cloneRenderHints(data.render_hints),
    projection: cloneProjection(data.projection),
    runtime_graph: cloneRuntimeGraph(data.runtime_graph),
    result_snapshot_ref: asFlowSnapshotRef(data.result_snapshot_ref),
  };
}

export function cloneFlowView(view: FlowViewDTO): FlowViewDTO {
  return {
    ...view,
    graph_query: {},
    view_state: {
      schema_version: 1,
      runtime_view: {},
      view_state_v2: {
        schema_version: 2,
        graph: { nodes: [], edges: [] },
        filters: {},
        counts: {},
      },
      graph: { nodes: [], edges: [] },
      filters: {},
      counts: {},
    },
  };
}
