export interface PaginationMeta {
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
  has_next: boolean;
}

export type FlowDirection = "in" | "out" | "both";
export type FlowGraphTier = "small" | "medium" | "large" | "xlarge";

export interface FlowGraphReq {
  case_id: string;
  seeds: string[];
  depth: number;
  direction: FlowDirection;
  min_amount: number;
  source?: string;
  request_id?: string;
  view?: string;
  layout?: string;
  date_start?: string;
  date_end?: string;
  focus_id?: string;
  focus_name?: string;
  focus_key_type?: string;
  focus_label?: string;
  focus_only?: boolean;
  focus_unknown_name?: boolean;
  include_missing_counterparty?: boolean;
  focus_counterparty_strict?: boolean;
  left_seeds?: string[];
  expected_total_amount?: number;
  expected_row_count?: number;
  focus_ids?: string[];
  focus_names?: string[];
  focus_placeholder_kinds?: string[];
  graph?: Record<string, unknown>;
  drill?: Record<string, unknown>;
  [key: string]: unknown;
}

export interface FlowNodeDTO {
  node_id: string;
  label: string;
  node_type: "account" | "holder" | "unknown";
  risk_score?: number | null;
}

export interface FlowEdgeDTO {
  edge_id: string;
  from_node_id: string;
  to_node_id: string;
  tx_count: number;
  amount_total: number;
}

export interface FlowRuntimeNodeDTO {
  id: string;
  title?: string;
  name?: string;
  display_id?: string;
  display_ids?: string[] | null;
  ntype?: string;
  total_amount?: number;
  total_count?: number;
  [key: string]: unknown;
}

export interface FlowRuntimeEdgeDTO {
  id: string;
  source: string;
  target: string;
  amount?: number;
  count?: number;
  label?: string;
  first_time?: string;
  last_time?: string;
  mode?: string;
  edgeArrow?: string;
  [key: string]: unknown;
}

export interface FlowRuntimeGraphDTO {
  nodes: FlowRuntimeNodeDTO[];
  edges: FlowRuntimeEdgeDTO[];
  runtime_revision?: number | null;
}

export interface FlowRenderHints {
  tier?: FlowGraphTier | string;
  view_mode?: string;
  all_nodes_visible?: boolean;
  default_node_mode?: "entity" | "mixed" | "dot" | string;
  entity_node_limit?: number;
  focus_entity_limit?: number;
  t0_label_limit?: number;
  t1_reveal_batch_size?: number;
  t1_reveal_delay_ms?: number;
  t1_batch_gap_ms?: number;
  show_edge_labels?: boolean;
  show_detail_edge_labels?: boolean;
  edge_mode?: "full" | "batched" | "skeleton" | string;
  edge_batch_threshold?: number;
  edge_batch_size?: number;
  edge_refresh_batch_size?: number;
  animation_mode?: "full" | "light" | "minimal" | string;
  layout_switch_animate_max_nodes?: number;
  layout_switch_animate_max_edges?: number;
  prefer_fast_first_paint?: boolean;
  projection_auto_expand?: boolean;
  projection_auto_expand_delay_ms?: number;
  projection_auto_expand_cooldown_ms?: number;
  projection_auto_expand_min_zoom?: number;
  projection_auto_expand_max_clusters?: number;
  projection_auto_expand_max_nodes?: number;
  projection_viewport_materialize_limit?: number;
  point_layer_mode?: "none" | "clustered" | string;
  [key: string]: unknown;
}

export interface FlowGraphProjection {
  mode?: "full" | "skeleton" | string;
  source_result_snapshot_ref?: FlowSnapshotRefDTO | null;
  source_query?: Record<string, unknown>;
  anchor_node_ids?: string[];
  expanded_cluster_ids?: string[];
  expanded_tile_ids?: string[];
  expanded_node_ids?: string[];
  path_node_ids?: string[];
  search_query?: string;
  search_match_node_ids?: string[];
  clusters?: Array<Record<string, unknown>>;
  [key: string]: unknown;
}

export interface FlowSnapshotRefDTO {
  snapshot_id: string;
  graph_hash: string;
  node_count: number;
  edge_count: number;
  stored_at: string;
}

export interface FlowPublicOpaqueSnapshotRefDTO {
  contract: "FlowPublicOpaqueSnapshotRefV1";
  opaque_ref: string;
  content_access: "controlled_artifact_required";
}

export interface FlowViewStateDTO {
  schema_version: 1;
  runtime_view: Record<string, never>;
  view_state_v2: {
    schema_version: 2;
    graph: {
      nodes: [];
      edges: [];
    };
    filters: Record<string, never>;
    counts: Record<string, never>;
  };
  graph: {
    nodes: [];
    edges: [];
  };
  filters: Record<string, never>;
  counts: Record<string, never>;
}

export interface FlowGraphData {
  contract?: "FlowPublicResultBoundaryV1";
  publication_status?: "blocked";
  fact_answer_allowed?: false;
  content_access?: "controlled_artifact_required";
  nodes: FlowNodeDTO[];
  edges: FlowEdgeDTO[];
  stats: Record<string, unknown>;
  graph_tier?: FlowGraphTier | string;
  render_hints?: FlowRenderHints;
  projection?: FlowGraphProjection;
  runtime_graph?: FlowRuntimeGraphDTO;
  result_snapshot_ref?: FlowSnapshotRefDTO | FlowPublicOpaqueSnapshotRefDTO | null;
}

export interface FlowRuntimeGraphPatchDTO {
  remove_node_ids: string[];
  upsert_nodes: FlowRuntimeNodeDTO[];
  remove_edge_ids: string[];
  upsert_edges: FlowRuntimeEdgeDTO[];
  summary: Record<string, unknown>;
}

export interface FlowResultSnapshotPatchData extends FlowGraphData {
  base_snapshot_ref: FlowSnapshotRefDTO | null;
  patch_kind: "delta" | "full";
  runtime_graph_patch?: FlowRuntimeGraphPatchDTO | null;
  base_runtime_revision?: number | null;
  runtime_revision?: number | null;
  trace_id?: string;
}

export interface FlowGraphExpandReq {
  case_id: string;
  search_query?: string;
  search_limit?: number;
  cluster_ids?: string[];
  tile_ids?: string[];
  node_ids?: string[];
  path_node_ids?: string[];
  viewport?: Record<string, unknown>;
  include_neighbors?: boolean;
  neighbor_depth?: number;
  materialize_limit?: number;
  reset?: boolean;
  graph?: Record<string, unknown>;
  drill?: Record<string, unknown>;
}

export interface FlowProjectionLayoutNodeSyncDTO {
  id: string;
  x?: number | null;
  y?: number | null;
  cluster_node?: boolean;
  projection_cluster_id?: string;
  node_render_mode?: string;
  projection_visible?: boolean | null;
  projection_collapsed?: boolean | null;
}

export interface FlowProjectionLayoutSyncReq {
  case_id: string;
  viewport?: Record<string, unknown>;
  graph_size?: Record<string, unknown>;
  nodes: FlowProjectionLayoutNodeSyncDTO[];
}

export interface FlowProjectionLayoutSyncResult {
  snapshot_ref?: FlowSnapshotRefDTO | null;
  updated_node_count?: number;
  layout_index_summary?: Record<string, unknown>;
  layout_sync_signature?: string;
  layout_sync_skipped?: boolean;
}

export type FlowLayoutNodePlanOperation =
  | "project"
  | "cache_key"
  | "apply"
  | "apply_worker"
  | "clear"
  | "network_plan"
  | "network_community_quality";

export interface FlowLayoutNodePlanReq {
  case_id: string;
  operation: FlowLayoutNodePlanOperation;
  nodes: Array<Record<string, unknown>>;
  edges?: Array<Record<string, unknown>>;
  nodesById?: Record<string, unknown>;
  nodes_by_id?: Record<string, unknown>;
  workerNodes?: Array<Record<string, unknown>>;
  worker_nodes?: Array<Record<string, unknown>>;
  metaKeys?: string[];
  meta_keys?: string[];
  algoVersion?: string;
  algo_version?: string;
  mode?: string;
  preset?: string;
  focusId?: string;
  focus_id?: string;
  graphMutationSeq?: number;
  graph_mutation_seq?: number;
  mutationSeq?: number;
  layoutDirection?: Record<string, unknown>;
  layout_direction?: Record<string, unknown>;
  seedContext?: Record<string, unknown>;
  seed_context?: Record<string, unknown>;
  semantic?: Record<string, unknown>;
  resultSnapshotRef?: Record<string, unknown>;
  result_snapshot_ref?: Record<string, unknown>;
}

export interface FlowLayoutNodePlanResult {
  operation: FlowLayoutNodePlanOperation;
  node_plan?: Record<string, Record<string, unknown>> | null;
  cacheKey?: Record<string, unknown> | null;
  applied?: boolean;
  source?: string;
  matched?: number;
  gridFallbackLikely?: boolean;
  updates?: Array<Record<string, unknown>>;
  nodes?: Array<Record<string, unknown>>;
  networkPlan?: Record<string, unknown> | null;
  communityQuality?: Array<Record<string, unknown>>;
  quality?: Record<string, unknown> | null;
  report?: Record<string, unknown> | null;
  mode?: string;
}

export interface FlowGraphRenderPlanReq {
  case_id: string;
  renderPlans?: Array<Record<string, unknown>>;
  render_plans?: Array<Record<string, unknown>>;
  renderPlan?: Record<string, unknown> | null;
  render_plan?: Record<string, unknown> | null;
  viewportExpandTargets?: Record<string, unknown> | null;
  viewport_expand_targets?: Record<string, unknown> | null;
}

export interface FlowGraphRenderPlanResult {
  renderResults: Array<Record<string, unknown>>;
  viewportExpandTargets?: Record<string, unknown> | null;
}

export interface FlowNetworkSectorPlacementReq {
  case_id: string;
  [key: string]: unknown;
}

export interface FlowNetworkSectorPlacementResult {
  sectorPlacement?: Record<string, unknown> | null;
}

export interface FlowGraphDataMergeReq {
  case_id: string;
  baseGraphData?: FlowGraphData | Record<string, unknown> | null;
  base_graph_data?: FlowGraphData | Record<string, unknown> | null;
  patchPayload: Record<string, unknown>;
  patch_payload?: Record<string, unknown>;
  useBaseGraph?: boolean;
  use_base_graph?: boolean;
}

export interface FlowGraphDataMergeResult {
  data?: FlowGraphData | null;
  targetSnapshotRef?: FlowSnapshotRefDTO | null;
  patchKind: string;
  errorMessage: string;
}

export interface FlowGraphSearchReq {
  case_id: string;
  nodes: Array<Record<string, unknown>>;
  query: string;
  limit?: number;
}

export interface FlowGraphSearchResult {
  query: string;
  nodeIds: string[];
  firstNodeId?: string | null;
  indexSummary?: Record<string, unknown>;
}

export interface FlowProjectionLayoutSeedReq {
  case_id: string;
  nodes: Array<Record<string, unknown>>;
  baseNodes?: Array<Record<string, unknown>>;
  base_nodes?: Array<Record<string, unknown>>;
}

export interface FlowProjectionLayoutSeedResult {
  summary: Record<string, unknown>;
  updates: Array<Record<string, unknown>>;
}

export interface FlowLayoutRoleGraphProjectionReq {
  case_id: string;
  nodes: Array<Record<string, unknown>>;
  edges: Array<Record<string, unknown>>;
  traversal?: Record<string, unknown> | null;
  demotion?: Record<string, unknown> | null;
  promotion?: Record<string, unknown> | null;
  roleResolution?: Record<string, unknown> | null;
  clusterResolution?: Record<string, unknown> | null;
  semanticPipeline?: Record<string, unknown> | null;
}

export interface FlowLayoutRoleGraphProjectionResult {
  projection?: Record<string, unknown> | null;
}

export interface FlowAnalysisGraphDataReq {
  case_id: string;
  source: {
    nodes?: Array<Record<string, unknown>>;
    edges?: Array<Record<string, unknown>>;
    [key: string]: unknown;
  };
  filterMin?: number | null;
  filterMax?: number | null;
  filter_min?: number | null;
  filter_max?: number | null;
  collapseChildren?: boolean;
  collapse_children?: boolean;
}

export interface FlowAnalysisGraphDataResult {
  nodes: Array<Record<string, unknown>>;
  edges: Array<Record<string, unknown>>;
  coreIds: string[];
  childMap: Record<string, string>;
}

export type FlowSameNameMergeMode = "gross" | "net";

export interface FlowSameNameMergeReq {
  case_id: string;
  nodes: Array<Record<string, unknown>>;
  edges: Array<Record<string, unknown>>;
  mode?: FlowSameNameMergeMode;
}

export interface FlowSameNameMergeResult {
  mode: FlowSameNameMergeMode;
  hasMergeTarget: boolean;
  nodes: Array<Record<string, unknown>>;
  edges: Array<Record<string, unknown>>;
}

export interface FlowBuildJobReq extends FlowGraphReq {}

export interface FlowBuildJobDTO {
  job_id: string;
  case_id: string;
  status: "queued" | "running" | "succeeded" | "failed" | "canceled";
  progress: number;
  summary: Record<string, unknown>;
  error: string | null;
  result_available: boolean;
  created_at: string;
  updated_at: string;
}

export interface FlowViewDTO {
  contract?: "FlowPublicViewV1";
  view_id: string;
  case_id: string;
  view_name: string;
  graph_query: Record<string, unknown>;
  view_state: FlowViewStateDTO;
  created_at: string;
  updated_at: string;
  content_access?: "controlled_artifact_required";
}

export interface FlowViewListData {
  items: FlowViewDTO[];
  page: PaginationMeta;
}

export interface FlowViewCreateReq {
  case_id: string;
}

export type FlowViewUpdateReq = Record<string, never>;
