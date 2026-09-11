import type {
  FlowBuildJobDTO,
  FlowGraphData,
  FlowPublicOpaqueSnapshotRefDTO,
  FlowViewDTO,
  FlowViewListData,
  FlowViewStateDTO,
  PaginationMeta,
} from "./flow-api-contracts";

export const FLOW_PUBLIC_BOUNDARY_REJECTED = "flow_public_boundary_rejected";
export const FLOW_CONTENT_CONTROLLED_ARTIFACT_REQUIRED = "flow_content_controlled_artifact_required";

const PUBLIC_RESULT_KEYS = [
  "contract",
  "publication_status",
  "fact_answer_allowed",
  "content_access",
  "nodes",
  "edges",
  "stats",
  "runtime_graph",
  "result_snapshot_ref",
] as const;
const PUBLIC_SNAPSHOT_REF_KEYS = ["contract", "opaque_ref", "content_access"] as const;
const PUBLIC_VIEW_KEYS = [
  "contract",
  "view_id",
  "case_id",
  "view_name",
  "graph_query",
  "view_state",
  "created_at",
  "updated_at",
  "content_access",
] as const;
const PUBLIC_VIEW_STATE_KEYS = [
  "schema_version",
  "runtime_view",
  "view_state_v2",
  "graph",
  "filters",
  "counts",
] as const;
const PUBLIC_VIEW_STATE_V2_KEYS = [
  "schema_version",
  "graph",
  "filters",
  "counts",
] as const;
const PUBLIC_VIEW_GRAPH_KEYS = ["nodes", "edges"] as const;
const PUBLIC_JOB_KEYS = [
  "job_id",
  "case_id",
  "status",
  "progress",
  "summary",
  "error",
  "result_available",
  "created_at",
  "updated_at",
] as const;
const PAGINATION_KEYS = ["page", "page_size", "total", "total_pages", "has_next"] as const;
const VIEW_LIST_KEYS = ["items", "page"] as const;
const OPAQUE_FLOW_REF_PATTERN = /^flowref_v1_[a-f0-9]{64}$/u;
const CANONICAL_UUID_PATTERN = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/u;
const SAFE_IDENTIFIER_PATTERN = /^[A-Za-z0-9._:-]{1,256}$/u;
const SAFE_TIMESTAMP_PATTERN = /^\d{4}-\d{2}-\d{2}T[0-9:.+-]{8,40}(?:Z)?$/u;

function rejectBoundary(): never {
  throw new Error(FLOW_PUBLIC_BOUNDARY_REJECTED);
}

function asStrictObject(value: unknown, allowedKeys: readonly string[]): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return rejectBoundary();
  }
  const source = value as Record<string, unknown>;
  const keys = Object.keys(source);
  if (keys.length !== allowedKeys.length || keys.some((key) => !allowedKeys.includes(key))) {
    return rejectBoundary();
  }
  return source;
}

function requireEmptyArray(value: unknown): [] {
  if (!Array.isArray(value) || value.length !== 0) {
    return rejectBoundary();
  }
  return [];
}

function requireEmptyObject(value: unknown): Record<string, never> {
  if (!value || typeof value !== "object" || Array.isArray(value) || Object.keys(value).length !== 0) {
    return rejectBoundary();
  }
  return {};
}

function requireExactString(value: unknown, expected: string): string {
  if (value !== expected) {
    return rejectBoundary();
  }
  return expected;
}

function requireSafeTimestamp(value: unknown): string {
  if (typeof value !== "string" || !SAFE_TIMESTAMP_PATTERN.test(value) || !Number.isFinite(Date.parse(value))) {
    return rejectBoundary();
  }
  return value;
}

function requireNonNegativeInteger(value: unknown): number {
  if (typeof value !== "number" || !Number.isSafeInteger(value) || value < 0) {
    return rejectBoundary();
  }
  return value;
}

function projectPublicSnapshotRef(value: unknown): FlowPublicOpaqueSnapshotRefDTO | null {
  if (value === null) {
    return null;
  }
  const source = asStrictObject(value, PUBLIC_SNAPSHOT_REF_KEYS);
  if (
    source.contract !== "FlowPublicOpaqueSnapshotRefV1" ||
    typeof source.opaque_ref !== "string" ||
    !OPAQUE_FLOW_REF_PATTERN.test(source.opaque_ref) ||
    source.content_access !== "controlled_artifact_required"
  ) {
    return rejectBoundary();
  }
  return {
    contract: "FlowPublicOpaqueSnapshotRefV1",
    opaque_ref: source.opaque_ref,
    content_access: "controlled_artifact_required",
  };
}

function projectPublicStats(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return rejectBoundary();
  }
  const source = value as Record<string, unknown>;
  const keys = Object.keys(source);
  if (keys.some((key) => key !== "build_ms" && key !== "cache_hit")) {
    return rejectBoundary();
  }
  const result: Record<string, unknown> = {};
  if ("build_ms" in source && source.build_ms !== null) {
    result.build_ms = requireNonNegativeInteger(source.build_ms);
  }
  if ("cache_hit" in source && source.cache_hit !== null) {
    if (typeof source.cache_hit !== "boolean") {
      return rejectBoundary();
    }
    result.cache_hit = source.cache_hit;
  }
  return result;
}

export function projectFlowPublicResultBoundary(value: unknown): FlowGraphData {
  const source = asStrictObject(value, PUBLIC_RESULT_KEYS);
  requireExactString(source.contract, "FlowPublicResultBoundaryV1");
  requireExactString(source.publication_status, "blocked");
  if (source.fact_answer_allowed !== false) {
    return rejectBoundary();
  }
  requireExactString(source.content_access, "controlled_artifact_required");
  requireEmptyArray(source.nodes);
  requireEmptyArray(source.edges);
  const runtimeGraph = asStrictObject(source.runtime_graph, ["nodes", "edges"]);
  requireEmptyArray(runtimeGraph.nodes);
  requireEmptyArray(runtimeGraph.edges);

  return {
    contract: "FlowPublicResultBoundaryV1",
    publication_status: "blocked",
    fact_answer_allowed: false,
    content_access: "controlled_artifact_required",
    nodes: [],
    edges: [],
    stats: projectPublicStats(source.stats),
    runtime_graph: { nodes: [], edges: [] },
    result_snapshot_ref: projectPublicSnapshotRef(source.result_snapshot_ref),
  };
}

function projectPublicViewGraph(value: unknown): FlowViewStateDTO["graph"] {
  const source = asStrictObject(value, PUBLIC_VIEW_GRAPH_KEYS);
  requireEmptyArray(source.nodes);
  requireEmptyArray(source.edges);
  return {
    nodes: [],
    edges: [],
  };
}

function projectPublicViewState(value: unknown): FlowViewStateDTO {
  const source = asStrictObject(value, PUBLIC_VIEW_STATE_KEYS);
  if (source.schema_version !== 1) {
    return rejectBoundary();
  }
  requireEmptyObject(source.runtime_view);
  requireEmptyObject(source.filters);
  requireEmptyObject(source.counts);
  const stateV2 = asStrictObject(source.view_state_v2, PUBLIC_VIEW_STATE_V2_KEYS);
  if (stateV2.schema_version !== 2) {
    return rejectBoundary();
  }
  requireEmptyObject(stateV2.filters);
  requireEmptyObject(stateV2.counts);

  const graph = projectPublicViewGraph(source.graph);
  const graphV2 = projectPublicViewGraph(stateV2.graph);

  return {
    schema_version: 1,
    runtime_view: {},
    view_state_v2: {
      schema_version: 2,
      graph: graphV2,
      filters: {},
      counts: {},
    },
    graph,
    filters: {},
    counts: {},
  };
}

export function projectFlowPublicViewBoundary(value: unknown, expectedCaseId: string): FlowViewDTO {
  const source = asStrictObject(value, PUBLIC_VIEW_KEYS);
  requireExactString(source.contract, "FlowPublicViewV1");
  if (
    typeof expectedCaseId !== "string" ||
    expectedCaseId.length === 0 ||
    source.case_id !== expectedCaseId ||
    typeof source.view_id !== "string" ||
    !CANONICAL_UUID_PATTERN.test(source.view_id)
  ) {
    return rejectBoundary();
  }
  requireExactString(source.view_name, "Saved flow view");
  requireEmptyObject(source.graph_query);
  requireExactString(source.created_at, "");
  requireExactString(source.updated_at, "");
  requireExactString(source.content_access, "controlled_artifact_required");

  return {
    contract: "FlowPublicViewV1",
    view_id: source.view_id,
    case_id: expectedCaseId,
    view_name: "Saved flow view",
    graph_query: {},
    view_state: projectPublicViewState(source.view_state),
    created_at: "",
    updated_at: "",
    content_access: "controlled_artifact_required",
  };
}

export function projectFlowBuildJobBoundary(value: unknown, expectedCaseId: string): FlowBuildJobDTO {
  const source = asStrictObject(value, PUBLIC_JOB_KEYS);
  if (
    typeof expectedCaseId !== "string" ||
    expectedCaseId.length === 0 ||
    source.case_id !== expectedCaseId ||
    typeof source.job_id !== "string" ||
    !SAFE_IDENTIFIER_PATTERN.test(source.job_id)
  ) {
    return rejectBoundary();
  }
  const statuses = new Set(["queued", "running", "succeeded", "failed", "canceled"]);
  if (typeof source.status !== "string" || !statuses.has(source.status)) {
    return rejectBoundary();
  }
  const progress = requireNonNegativeInteger(source.progress);
  if (progress > 100 || typeof source.result_available !== "boolean") {
    return rejectBoundary();
  }
  const summary = projectPublicStats(source.summary);
  if (source.error !== null && source.error !== "flow_build_failed" && source.error !== "flow_build_canceled") {
    return rejectBoundary();
  }
  const error = source.error as FlowBuildJobDTO["error"];
  const status = source.status as FlowBuildJobDTO["status"];
  if (
    (status === "failed" && error !== "flow_build_failed") ||
    (status === "canceled" && error !== "flow_build_canceled") ||
    ((status === "queued" || status === "running" || status === "succeeded") && error !== null) ||
    (source.result_available !== (status === "succeeded"))
  ) {
    return rejectBoundary();
  }

  return {
    job_id: source.job_id,
    case_id: expectedCaseId,
    status,
    progress,
    summary,
    error,
    result_available: source.result_available,
    created_at: requireSafeTimestamp(source.created_at),
    updated_at: requireSafeTimestamp(source.updated_at),
  };
}

function projectPagination(value: unknown): PaginationMeta {
  const source = asStrictObject(value, PAGINATION_KEYS);
  const page = requireNonNegativeInteger(source.page);
  const pageSize = requireNonNegativeInteger(source.page_size);
  const total = requireNonNegativeInteger(source.total);
  const totalPages = requireNonNegativeInteger(source.total_pages);
  if (page < 1 || pageSize < 1 || typeof source.has_next !== "boolean") {
    return rejectBoundary();
  }
  return {
    page,
    page_size: pageSize,
    total,
    total_pages: totalPages,
    has_next: source.has_next,
  };
}

export function projectFlowPublicViewListBoundary(value: unknown, expectedCaseId: string): FlowViewListData {
  const source = asStrictObject(value, VIEW_LIST_KEYS);
  if (!Array.isArray(source.items)) {
    return rejectBoundary();
  }
  const items = source.items.map((item) => projectFlowPublicViewBoundary(item, expectedCaseId));
  const page = projectPagination(source.page);
  if (page.total < items.length) {
    return rejectBoundary();
  }
  return { items, page };
}

export function requireControlledFlowArtifact(): never {
  throw new Error(FLOW_CONTENT_CONTROLLED_ARTIFACT_REQUIRED);
}
