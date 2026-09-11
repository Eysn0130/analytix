from __future__ import annotations

from typing import Any, Dict, Literal, Optional

from pydantic import BaseModel, ConfigDict, Field


class FlowGraphReq(BaseModel):
    model_config = ConfigDict(extra="forbid", strict=True, allow_inf_nan=False)

    case_id: str = Field(min_length=1)
    seeds: list[str] = Field(min_length=1)
    depth: int = Field(default=3, ge=1, le=8)
    direction: Literal["in", "out", "both"] = "both"
    min_amount: float = Field(default=0.0, ge=0.0)
    source: str = ""
    request_id: str = ""
    view: str = ""
    layout: str = ""
    date_start: str = ""
    date_end: str = ""
    focus_id: str = ""
    focus_name: str = ""
    focus_key_type: str = ""
    focus_label: str = ""
    focus_only: bool = False
    focus_unknown_name: bool = False
    include_missing_counterparty: bool = False
    focus_counterparty_strict: bool = False
    left_seeds: list[str] = Field(default_factory=list)
    expected_total_amount: Optional[float] = None
    expected_row_count: Optional[int] = None
    focus_ids: list[str] = Field(default_factory=list)
    focus_names: list[str] = Field(default_factory=list)
    focus_placeholder_kinds: list[str] = Field(default_factory=list)
    graph: Dict[str, Any] = Field(default_factory=dict)
    drill: Dict[str, Any] = Field(default_factory=dict)


class FlowBuildJobReq(FlowGraphReq):
    pass


class FlowGraphExpandReq(BaseModel):
    model_config = ConfigDict(extra="allow")

    case_id: str = Field(min_length=1)
    search_query: str = ""
    search_limit: int = Field(default=24, ge=1, le=256)
    cluster_ids: list[str] = Field(default_factory=list)
    tile_ids: list[str] = Field(default_factory=list)
    node_ids: list[str] = Field(default_factory=list)
    path_node_ids: list[str] = Field(default_factory=list)
    viewport: Dict[str, Any] = Field(default_factory=dict)
    include_neighbors: bool = True
    neighbor_depth: int = Field(default=1, ge=0, le=3)
    materialize_limit: int = Field(default=4000, ge=1, le=50000)
    reset: bool = False
    graph: Dict[str, Any] = Field(default_factory=dict)
    drill: Dict[str, Any] = Field(default_factory=dict)


class FlowProjectionLayoutSyncReq(BaseModel):
    model_config = ConfigDict(extra="allow")

    case_id: str = Field(min_length=1)
    viewport: Dict[str, Any] = Field(default_factory=dict)
    graph_size: Dict[str, Any] = Field(default_factory=dict)
    nodes: list[Dict[str, Any]] = Field(default_factory=list)


class FlowLayoutNodePlanReq(BaseModel):
    model_config = ConfigDict(extra="forbid", populate_by_name=True)

    case_id: str = Field(min_length=1)
    operation: Literal[
        "project",
        "cache_key",
        "apply",
        "apply_worker",
        "clear",
        "network_plan",
        "network_community_quality",
    ]
    nodes: list[Dict[str, Any]] = Field(default_factory=list)
    edges: list[Dict[str, Any]] = Field(default_factory=list)
    nodes_by_id: Dict[str, Any] = Field(default_factory=dict, alias="nodesById")
    worker_nodes: list[Dict[str, Any]] = Field(default_factory=list, alias="workerNodes")
    meta_keys: Optional[list[str]] = Field(default=None, alias="metaKeys")
    algo_version: str = Field(default="", alias="algoVersion")
    mode: str = ""
    focus_id: str = Field(default="", alias="focusId")
    graph_mutation_seq: int = Field(default=0, alias="graphMutationSeq")
    layout_direction: Dict[str, Any] = Field(default_factory=dict, alias="layoutDirection")
    seed_context: Dict[str, Any] = Field(default_factory=dict, alias="seedContext")
    semantic: Dict[str, Any] = Field(default_factory=dict)
    result_snapshot_ref: Dict[str, Any] = Field(default_factory=dict, alias="resultSnapshotRef")


class FlowLayoutRoleGraphProjectionReq(BaseModel):
    model_config = ConfigDict(extra="allow", populate_by_name=True)

    case_id: str = Field(min_length=1)
    nodes: list[Dict[str, Any]] = Field(default_factory=list)
    edges: list[Dict[str, Any]] = Field(default_factory=list)
    traversal: Optional[Dict[str, Any]] = None
    demotion: Optional[Dict[str, Any]] = None
    promotion: Optional[Dict[str, Any]] = None
    role_resolution: Optional[Dict[str, Any]] = Field(default=None, alias="roleResolution")
    cluster_resolution: Optional[Dict[str, Any]] = Field(default=None, alias="clusterResolution")
    semantic_pipeline: Optional[Dict[str, Any]] = Field(default=None, alias="semanticPipeline")


class FlowGraphRenderPlanReq(BaseModel):
    model_config = ConfigDict(extra="allow", populate_by_name=True)

    case_id: str = Field(min_length=1)
    render_plans: list[Dict[str, Any]] = Field(default_factory=list, alias="renderPlans")
    render_plan: Optional[Dict[str, Any]] = Field(default=None, alias="renderPlan")
    viewport_expand_targets: Optional[Dict[str, Any]] = Field(default=None, alias="viewportExpandTargets")


class FlowNetworkSectorPlacementReq(BaseModel):
    model_config = ConfigDict(extra="allow", populate_by_name=True)

    case_id: str = Field(min_length=1)


class FlowAnalysisGraphDataReq(BaseModel):
    model_config = ConfigDict(extra="allow", populate_by_name=True)

    case_id: str = Field(min_length=1)
    source: Dict[str, Any] = Field(default_factory=dict)
    filter_min: Optional[float] = Field(default=None, alias="filterMin")
    filter_max: Optional[float] = Field(default=None, alias="filterMax")
    collapse_children: bool = Field(default=False, alias="collapseChildren")


class FlowSameNameMergeReq(BaseModel):
    model_config = ConfigDict(extra="allow", populate_by_name=True)

    case_id: str = Field(min_length=1)
    nodes: list[Dict[str, Any]] = Field(default_factory=list)
    edges: list[Dict[str, Any]] = Field(default_factory=list)
    mode: str = Field(default="gross")


class FlowGraphDataMergeReq(BaseModel):
    model_config = ConfigDict(extra="allow", populate_by_name=True)

    case_id: str = Field(min_length=1)
    base_graph_data: Optional[Dict[str, Any]] = Field(default=None, alias="baseGraphData")
    patch_payload: Dict[str, Any] = Field(default_factory=dict, alias="patchPayload")
    use_base_graph: bool = Field(default=True, alias="useBaseGraph")


class FlowGraphSearchReq(BaseModel):
    model_config = ConfigDict(extra="allow", populate_by_name=True)

    case_id: str = Field(min_length=1)
    nodes: list[Dict[str, Any]] = Field(default_factory=list)
    query: str = ""
    limit: int = Field(default=1, ge=1, le=1000)


class FlowProjectionLayoutSeedReq(BaseModel):
    model_config = ConfigDict(extra="allow", populate_by_name=True)

    case_id: str = Field(min_length=1)
    nodes: list[Dict[str, Any]] = Field(default_factory=list)
    base_nodes: list[Dict[str, Any]] = Field(default_factory=list, alias="baseNodes")


class FlowLayoutNodePlanResultDTO(BaseModel):
    model_config = ConfigDict(extra="allow")

    operation: Literal[
        "project",
        "cache_key",
        "apply",
        "apply_worker",
        "clear",
        "network_plan",
        "network_community_quality",
    ]
    node_plan: Optional[Dict[str, Any]] = None
    cacheKey: Optional[Dict[str, Any]] = None
    applied: Optional[bool] = None
    source: Optional[str] = None
    matched: Optional[int] = None
    gridFallbackLikely: Optional[bool] = None
    updates: Optional[list[Dict[str, Any]]] = None
    nodes: Optional[list[Dict[str, Any]]] = None
    networkPlan: Optional[Dict[str, Any]] = None
    communityQuality: Optional[list[Dict[str, Any]]] = None
    quality: Optional[Dict[str, Any]] = None
    report: Optional[Dict[str, Any]] = None
    mode: Optional[str] = None


class FlowGraphRenderPlanResultDTO(BaseModel):
    model_config = ConfigDict(extra="allow")

    renderResults: list[Dict[str, Any]] = Field(default_factory=list)
    viewportExpandTargets: Optional[Dict[str, Any]] = None


class FlowNetworkSectorPlacementResultDTO(BaseModel):
    model_config = ConfigDict(extra="allow")

    sectorPlacement: Dict[str, Any] = Field(default_factory=dict)


class FlowAnalysisGraphDataResultDTO(BaseModel):
    model_config = ConfigDict(extra="allow")

    nodes: list[Dict[str, Any]] = Field(default_factory=list)
    edges: list[Dict[str, Any]] = Field(default_factory=list)
    coreIds: list[str] = Field(default_factory=list)
    childMap: Dict[str, str] = Field(default_factory=dict)


class FlowSameNameMergeResultDTO(BaseModel):
    model_config = ConfigDict(extra="allow")

    mode: str = "gross"
    hasMergeTarget: bool = False
    nodes: list[Dict[str, Any]] = Field(default_factory=list)
    edges: list[Dict[str, Any]] = Field(default_factory=list)


class FlowGraphDataMergeResultDTO(BaseModel):
    model_config = ConfigDict(extra="allow")

    data: Optional[Dict[str, Any]] = None
    targetSnapshotRef: Optional[Dict[str, Any]] = None
    patchKind: str = "full"
    errorMessage: str = ""


class FlowGraphSearchResultDTO(BaseModel):
    model_config = ConfigDict(extra="allow")

    query: str = ""
    nodeIds: list[str] = Field(default_factory=list)
    firstNodeId: Optional[str] = None
    indexSummary: Dict[str, Any] = Field(default_factory=dict)


class FlowProjectionLayoutSeedResultDTO(BaseModel):
    model_config = ConfigDict(extra="allow")

    summary: Dict[str, Any] = Field(default_factory=dict)
    updates: list[Dict[str, Any]] = Field(default_factory=list)


class FlowSnapshotRefDTO(BaseModel):
    snapshot_id: str = ""
    graph_hash: str = ""
    node_count: int = Field(default=0, ge=0)
    edge_count: int = Field(default=0, ge=0)
    stored_at: str = ""


class FlowNodeDTO(BaseModel):
    node_id: str
    label: str
    node_type: Literal["account", "holder", "unknown"]
    risk_score: Optional[float] = None


class FlowEdgeDTO(BaseModel):
    edge_id: str
    from_node_id: str
    to_node_id: str
    tx_count: int = Field(ge=0)
    amount_total: float


class FlowPublicSnapshotRefDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    contract: Literal["FlowPublicOpaqueSnapshotRefV1"] = "FlowPublicOpaqueSnapshotRefV1"
    opaque_ref: str = Field(pattern=r"^flowref_v1_[a-f0-9]{64}$")
    content_access: Literal["controlled_artifact_required"] = "controlled_artifact_required"


class FlowPublicStatsDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    build_ms: Optional[int] = Field(default=None, ge=0)


class FlowPublicRuntimeGraphDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    nodes: list[None] = Field(default_factory=list, max_length=0)
    edges: list[None] = Field(default_factory=list, max_length=0)


class FlowPublicResultDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    contract: Literal["FlowPublicResultBoundaryV1"] = "FlowPublicResultBoundaryV1"
    publication_status: Literal["blocked"] = "blocked"
    fact_answer_allowed: Literal[False] = False
    content_access: Literal["controlled_artifact_required"] = "controlled_artifact_required"
    nodes: list[None] = Field(default_factory=list, max_length=0)
    edges: list[None] = Field(default_factory=list, max_length=0)
    stats: FlowPublicStatsDTO = Field(default_factory=FlowPublicStatsDTO)
    runtime_graph: FlowPublicRuntimeGraphDTO = Field(default_factory=FlowPublicRuntimeGraphDTO)
    result_snapshot_ref: Optional[FlowPublicSnapshotRefDTO] = None


class FlowRuntimeNodeDTO(BaseModel):
    model_config = ConfigDict(extra="allow")

    id: str = ""
    title: str = ""
    name: str = ""
    display_id: str = ""
    display_ids: Optional[list[str]] = None
    ntype: str = "node"
    total_amount: float = 0.0
    total_count: int = 0


class FlowRuntimeEdgeDTO(BaseModel):
    model_config = ConfigDict(extra="allow")

    id: str = ""
    source: str = ""
    target: str = ""
    amount: float = 0.0
    count: int = 0
    label: str = ""
    first_time: str = ""
    last_time: str = ""
    mode: str = ""
    edgeArrow: str = ""


class FlowRuntimeGraphDTO(BaseModel):
    model_config = ConfigDict(extra="allow")

    nodes: list[FlowRuntimeNodeDTO] = Field(default_factory=list)
    edges: list[FlowRuntimeEdgeDTO] = Field(default_factory=list)
    runtime_revision: Optional[int] = Field(default=None, ge=0)


def normalize_flow_runtime_graph(value: Any) -> Dict[str, Any]:
    if isinstance(value, BaseModel):
        payload = value.model_dump(exclude_none=True)
    else:
        payload = value if isinstance(value, dict) else {}
    return FlowRuntimeGraphDTO.model_validate(payload).model_dump(exclude_none=True)

class FlowBuildJobSummaryDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    build_ms: Optional[int] = Field(default=None, ge=0)


class FlowBuildJobDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    job_id: str
    case_id: str
    status: Literal["queued", "running", "succeeded", "failed", "canceled"]
    progress: int = Field(ge=0, le=100)
    summary: FlowBuildJobSummaryDTO = Field(default_factory=FlowBuildJobSummaryDTO)
    error: Optional[Literal["flow_build_failed", "flow_build_canceled"]] = None
    result_available: bool = False
    created_at: str
    updated_at: str


class FlowViewCreateReq(BaseModel):
    model_config = ConfigDict(extra="forbid")

    case_id: str = Field(min_length=1)


class FlowViewUpdateReq(BaseModel):
    model_config = ConfigDict(extra="forbid")


class FlowPublicEmptyMapDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")


class FlowPublicViewGraphDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    nodes: list[None] = Field(default_factory=list, max_length=0)
    edges: list[None] = Field(default_factory=list, max_length=0)


class FlowPublicViewStateV2DTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    schema_version: Literal[2] = 2
    graph: FlowPublicViewGraphDTO = Field(default_factory=FlowPublicViewGraphDTO)
    filters: FlowPublicEmptyMapDTO = Field(default_factory=FlowPublicEmptyMapDTO)
    counts: FlowPublicEmptyMapDTO = Field(default_factory=FlowPublicEmptyMapDTO)


class FlowPublicViewStateDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    schema_version: Literal[1] = 1
    runtime_view: FlowPublicEmptyMapDTO = Field(default_factory=FlowPublicEmptyMapDTO)
    view_state_v2: FlowPublicViewStateV2DTO = Field(default_factory=FlowPublicViewStateV2DTO)
    graph: FlowPublicViewGraphDTO = Field(default_factory=FlowPublicViewGraphDTO)
    filters: FlowPublicEmptyMapDTO = Field(default_factory=FlowPublicEmptyMapDTO)
    counts: FlowPublicEmptyMapDTO = Field(default_factory=FlowPublicEmptyMapDTO)


class FlowPublicViewDTO(BaseModel):
    model_config = ConfigDict(extra="forbid")

    contract: Literal["FlowPublicViewV1"] = "FlowPublicViewV1"
    view_id: str
    case_id: str
    view_name: Literal["Saved flow view"] = "Saved flow view"
    graph_query: FlowPublicEmptyMapDTO = Field(default_factory=FlowPublicEmptyMapDTO)
    view_state: FlowPublicViewStateDTO = Field(default_factory=FlowPublicViewStateDTO)
    created_at: Literal[""] = ""
    updated_at: Literal[""] = ""
    content_access: Literal["controlled_artifact_required"] = "controlled_artifact_required"


class FlowViewReorderReq(BaseModel):
    model_config = ConfigDict(extra="forbid", strict=True)

    case_id: str = Field(min_length=1, max_length=256)
    order: list[str] = Field(default_factory=list, max_length=10_000)
