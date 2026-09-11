import { HttpClient } from "../../../../services/http/client";
import {
  isDataAnalysisRuntimeEndpointAuthorityCurrent,
  resolveDataAnalysisRuntimeEndpointAuthority,
  type DataAnalysisRuntimeEndpointAuthority,
} from "../../../../services/runtime-base";
import {
  eventBelongsToCase,
  isFlowBuildEvent
} from "../../../../services/ws/domain-events";
import type { WsEventEnvelope } from "../../../../services/ws/types";
import { getSharedWsRuntime, SharedWsRuntime } from "../../../../services/ws/shared-runtime";
import {
  LEGACY_STATS_EVIDENCE_BLOCKER,
  type StatsTreeGroupDTO,
  type StatsTxnRowDTO,
  type StatsV2MetaDTO,
  type StatsV2TreeDTO,
} from "../../../../services/analysis/stats-shared";
import type { CaseDetailDTO } from "../../../cases/api";
import type {
  FlowAnalysisGraphDataReq,
  FlowAnalysisGraphDataResult,
  FlowBuildJobDTO,
  FlowGraphData,
  FlowGraphRenderPlanReq,
  FlowGraphRenderPlanResult,
  FlowGraphSearchReq,
  FlowGraphSearchResult,
  FlowLayoutNodePlanReq,
  FlowLayoutNodePlanResult,
  FlowLayoutRoleGraphProjectionReq,
  FlowLayoutRoleGraphProjectionResult,
  FlowNetworkSectorPlacementReq,
  FlowNetworkSectorPlacementResult,
  FlowProjectionLayoutSeedReq,
  FlowProjectionLayoutSeedResult,
  FlowProjectionLayoutSyncResult,
  FlowResultSnapshotPatchData,
  FlowSameNameMergeReq,
  FlowSameNameMergeResult,
  FlowSnapshotRefDTO,
  FlowViewDTO
} from "../../../../services/analysis/flow-api-contracts";
import {
  projectFlowBuildJobBoundary,
  projectFlowPublicResultBoundary,
  projectFlowPublicViewBoundary,
  projectFlowPublicViewListBoundary,
  requireControlledFlowArtifact,
} from "../../../../services/analysis/flow-public-boundary";
import {
  canonicalizeFlowCasePayload,
  shouldUseDirectBuildPath,
} from "./flow-direct-build-cache";
import {
  asFlowSnapshotRef,
  cloneFlowGraphData,
  cloneFlowView,
} from "../model/graph-data-model";
import {
  FLOW_BUILD_TIMEOUT_MS,
  getFlowBuildPollDelayMs,
} from "./flow-build-polling";

const FLOW_DIRECT_WARM_TRACE_PREFIX = "flow-warm";

function text(value: unknown): string {
  return String(value == null ? "" : value).trim();
}

function asNumber(value: unknown, fallback = 0): number {
  const next = Number(value);
  return Number.isFinite(next) ? next : fallback;
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => {
    window.setTimeout(resolve, Math.max(0, Number(ms) || 0));
  });
}

function toErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof Error && text(error.message)) {
    return text(error.message);
  }
  return fallback;
}

function authorityIdentity(authority: DataAnalysisRuntimeEndpointAuthority): string {
  return JSON.stringify({
    kind: authority.kind,
    authorityId: authority.authorityId,
    generation: authority.lease.generation,
    apiBaseUrl: authority.apiBaseUrl,
    wsBaseUrl: authority.wsBaseUrl,
  });
}

function waitForDirectBuild<T>(promise: Promise<T>, signal: AbortSignal): Promise<T> {
  if (signal.aborted) {
    return Promise.reject(new Error("flow_direct_build_context_revoked"));
  }
  return new Promise<T>((resolve, reject) => {
    const onAbort = (): void => {
      cleanup();
      reject(new Error("flow_direct_build_context_revoked"));
    };
    const cleanup = (): void => {
      signal.removeEventListener("abort", onAbort);
    };
    signal.addEventListener("abort", onAbort, { once: true });
    promise.then(
      (value) => {
        cleanup();
        resolve(value);
      },
      (error) => {
        cleanup();
        reject(error);
      },
    );
  });
}

interface EmbeddedFlowViewSavePayload {
  case_id: string;
  view_id?: string;
}

interface EmbeddedFlowReorderPayload {
  case_id: string;
  order: string[];
}

interface FlowAccountTxnRowsRequest {
  case_id: string;
  account_key: string;
  date_start?: string;
  date_end?: string;
  start_time?: string;
  end_time?: string;
  sort_dir?: "asc" | "desc";
  limit?: number;
  cursor?: Record<string, unknown> | null;
}

interface FlowAccountTxnRowsResult {
  contract: "StatsAccountTxnRowsPublicBoundaryV1";
  semantic_status: "source_unavailable" | "needs_input" | "blocked";
  fact_answer_allowed: false;
  raw_details_exposed: false;
  blocker: string;
  rows: StatsTxnRowDTO[];
  done?: boolean | null;
  next_cursor?: Record<string, unknown> | null;
  nextCursor?: Record<string, unknown> | null;
}

interface FlowStatsTxnRowsRequest {
  case_id: string;
  selected: string[];
  date_start?: string;
  date_end?: string;
  key_type?: "account" | "name";
  key_value?: string;
  key_values?: string[];
  filter?: "all" | "in" | "out";
  sort_col?: "txn_time" | "amount";
  sort_dir?: "asc" | "desc";
  limit?: number;
  cursor?: Record<string, unknown> | null;
}

interface FlowStatsTxnRowsResult {
  contract: "StatsTxnRowsPublicBoundaryV1";
  semantic_status: "blocked";
  fact_answer_allowed: false;
  raw_details_exposed: false;
  blocker?: string;
  rows: StatsTxnRowDTO[];
  done?: boolean | null;
  next_cursor?: Record<string, unknown> | null;
  nextCursor?: Record<string, unknown> | null;
}

interface ActiveFlowBuildState {
  token: number;
  jobId: string;
  caseId: string;
  traceId: string;
  startedAt: number;
}

interface EmbeddedFlowGraphPatchPayload extends Record<string, unknown> {
  case_id?: string;
  caseId?: string;
  trace_id?: string;
  traceId?: string;
}

interface FlowGraphPatchEventData {
  caseId: string;
  payload: Record<string, unknown>;
  data: FlowGraphData | null;
}

interface FlowExpandedPatchResult {
  data: FlowGraphData;
  patch: FlowResultSnapshotPatchData;
}

interface FlowGraphPatchPerfBucket {
  count: number;
  failedCount: number;
  deltaCount: number;
  fullCount: number;
  lastDurationMs: number;
  lastAt: number;
  lastKind: string;
  lastScope: string;
  lastSource: string;
  lastSnapshotId: string;
  lastBaseRuntimeRevision: number;
  lastRuntimeRevision: number;
  lastNodeCount: number;
  lastEdgeCount: number;
  lastTraceId: string;
  lastError: string;
}

interface FlowGraphPatchPerfSnapshot {
  publish: FlowGraphPatchPerfBucket;
  apply: FlowGraphPatchPerfBucket;
}

interface FlowBuildPerfBucket {
  count: number;
  successCount: number;
  failedCount: number;
  lastDurationMs: number;
  lastAt: number;
  lastStatus: string;
  lastSnapshotId: string;
  lastNodeCount: number;
  lastEdgeCount: number;
  lastTraceId: string;
  lastError: string;
}

function createFlowGraphPatchPerfBucket(): FlowGraphPatchPerfBucket {
  return {
    count: 0,
    failedCount: 0,
    deltaCount: 0,
    fullCount: 0,
    lastDurationMs: 0,
    lastAt: 0,
    lastKind: "",
    lastScope: "",
    lastSource: "",
    lastSnapshotId: "",
    lastBaseRuntimeRevision: 0,
    lastRuntimeRevision: 0,
    lastNodeCount: 0,
    lastEdgeCount: 0,
    lastTraceId: "",
    lastError: ""
  };
}

function createFlowGraphPatchPerfSnapshot(): FlowGraphPatchPerfSnapshot {
  return {
    publish: createFlowGraphPatchPerfBucket(),
    apply: createFlowGraphPatchPerfBucket()
  };
}

function createFlowBuildPerfBucket(): FlowBuildPerfBucket {
  return {
    count: 0,
    successCount: 0,
    failedCount: 0,
    lastDurationMs: 0,
    lastAt: 0,
    lastStatus: "",
    lastSnapshotId: "",
    lastNodeCount: 0,
    lastEdgeCount: 0,
    lastTraceId: "",
    lastError: ""
  };
}

function createTraceId(prefix: string, caseId = ""): string {
  const normalizedPrefix = text(prefix) || "flow";
  const normalizedCaseId = text(caseId).slice(-10) || "case";
  const nonce = Math.random().toString(36).slice(2, 8);
  return `${normalizedPrefix}:${normalizedCaseId}:${Date.now().toString(36)}:${nonce}`;
}

export interface EmbeddedFlowBridgeService {
  setCaseId: (caseId: string) => void;
  dispose: () => void;
  getCaseDetail: (caseId: string) => Promise<CaseDetailDTO>;
  getFundsMeta: (caseId: string) => Promise<StatsV2MetaDTO>;
  getStatsTree: (payload: { case_id: string; tab: "byName" | "byCard" }) => Promise<StatsV2TreeDTO>;
  buildFlowGraph: (payload: Record<string, unknown>) => Promise<FlowGraphData>;
  warmFlowGraph: (payload: Record<string, unknown>) => Promise<void>;
  getAccountTxnRows: (payload: FlowAccountTxnRowsRequest) => Promise<FlowAccountTxnRowsResult>;
  getStatsTxnRows: (payload: FlowStatsTxnRowsRequest) => Promise<FlowStatsTxnRowsResult>;
  listFlowViews: (caseId: string) => Promise<{ items: FlowViewDTO[] }>;
  getFlowResultSnapshot: (payload: { case_id: string; snapshot_id: string }) => Promise<FlowGraphData>;
  expandFlowResultSnapshot: (snapshotId: string, payload: Record<string, unknown>) => Promise<FlowGraphData>;
  expandFlowResultSnapshotDetailed: (snapshotId: string, payload: Record<string, unknown>) => Promise<FlowExpandedPatchResult>;
  syncFlowResultSnapshotProjectionLayout: (
    snapshotId: string,
    payload: Record<string, unknown>
  ) => Promise<FlowProjectionLayoutSyncResult>;
  computeLayoutNodePlan: (payload: FlowLayoutNodePlanReq) => Promise<FlowLayoutNodePlanResult>;
  projectGraphRenderPlan: (payload: FlowGraphRenderPlanReq) => Promise<FlowGraphRenderPlanResult>;
  projectNetworkSectorPlacement: (payload: FlowNetworkSectorPlacementReq) => Promise<FlowNetworkSectorPlacementResult>;
  projectGraphSearch: (payload: FlowGraphSearchReq) => Promise<FlowGraphSearchResult>;
  projectProjectionLayoutSeed: (payload: FlowProjectionLayoutSeedReq) => Promise<FlowProjectionLayoutSeedResult>;
  projectLayoutRoleGraph: (payload: FlowLayoutRoleGraphProjectionReq) => Promise<FlowLayoutRoleGraphProjectionResult>;
  projectAnalysisGraphData: (payload: FlowAnalysisGraphDataReq) => Promise<FlowAnalysisGraphDataResult>;
  mergeSameNameGraph: (payload: FlowSameNameMergeReq) => Promise<FlowSameNameMergeResult>;
  saveFlowView: (payload: EmbeddedFlowViewSavePayload) => Promise<FlowViewDTO>;
  deleteFlowView: (viewId: string, caseId: string) => Promise<void>;
  renameFlowView: (viewId: string, title: string, caseId: string) => Promise<FlowViewDTO>;
  reorderFlowViews: (payload: EmbeddedFlowReorderPayload) => Promise<{ count: number }>;
  publishFlowGraphPatch: (payload: EmbeddedFlowGraphPatchPayload) => Promise<FlowGraphData | null>;
  getFlowPerfSnapshot: () => Record<string, unknown>;
  subscribeFlowGraphPatch: (listener: (event: FlowGraphPatchEventData) => void) => () => void;
}

class FlowBridgeServiceImpl implements EmbeddedFlowBridgeService {
  private readonly client: HttpClient;
  private readonly authority: DataAnalysisRuntimeEndpointAuthority;
  private readonly apiBaseUrl: string;
  private readonly wsBaseUrl: string;
  private wsRuntime: SharedWsRuntime | null = null;
  private releaseWsRuntime: (() => void) | null = null;
  private unsubscribeRuntimeEvents: (() => void) | null = null;

  private caseId = "";
  private caseBindingRevision = 0;
  private disposed = false;
  private buildToken = 0;
  private activeBuild: ActiveFlowBuildState | null = null;
  private readonly directBuildControllers = new Set<AbortController>();
  private readonly flowViewsCache = new Map<string, FlowViewDTO[]>();
  private readonly graphPatchListeners = new Set<(event: FlowGraphPatchEventData) => void>();
  private readonly perf = {
    graphPatch: createFlowGraphPatchPerfSnapshot(),
    build: createFlowBuildPerfBucket()
  };

  constructor(authority: DataAnalysisRuntimeEndpointAuthority) {
    this.authority = authority;
    const normalizedBase = authority.apiBaseUrl;
    this.apiBaseUrl = normalizedBase;
    this.wsBaseUrl = authority.wsBaseUrl;
    this.client = new HttpClient(normalizedBase);
  }

  setCaseId(caseId: string): void {
    const normalized = text(caseId);
    if (this.disposed) {
      throw new Error("flow_bridge_disposed");
    }
    this.requireCurrentAuthority();
    if (normalized === this.caseId && (!normalized || this.wsRuntime !== null)) {
      return;
    }
    this.caseBindingRevision += 1;
    this.revokeDirectBuilds();
    void this.cancelActiveFlowBuild();
    this.releaseWsBinding();
    this.flowViewsCache.clear();
    this.perf.graphPatch = createFlowGraphPatchPerfSnapshot();
    this.perf.build = createFlowBuildPerfBucket();
    this.caseId = normalized;
    if (!normalized) {
      return;
    }
    const runtime = getSharedWsRuntime({
      apiBaseUrl: this.apiBaseUrl,
      wsBaseUrl: this.wsBaseUrl,
      caseId: normalized,
    });
    const release = runtime.retain();
    this.wsRuntime = runtime;
    this.releaseWsRuntime = release;
    this.unsubscribeRuntimeEvents = runtime.onEvent((event) => {
      this.handleFlowViewsPatchEvent(event);
    });
  }

  dispose(): void {
    if (this.disposed) {
      return;
    }
    this.disposed = true;
    this.caseBindingRevision += 1;
    this.revokeDirectBuilds();
    void this.cancelActiveFlowBuild();
    this.releaseWsBinding();
    this.graphPatchListeners.clear();
  }

  isDisposed(): boolean {
    return this.disposed;
  }

  async getCaseDetail(caseId: string): Promise<CaseDetailDTO> {
    const normalizedCaseId = text(caseId);
    const revision = this.captureCaseContext(normalizedCaseId);
    const response = await this.client.get<CaseDetailDTO>(`/api/v1/cases/${encodeURIComponent(normalizedCaseId)}`);
    this.requireCaseContext(normalizedCaseId, revision);
    return response.data;
  }

  async getFundsMeta(caseId: string): Promise<StatsV2MetaDTO> {
    const normalizedCaseId = text(caseId);
    const revision = this.captureCaseContext(normalizedCaseId);
    this.requireCaseContext(normalizedCaseId, revision);
    return {
      contract: "StatsMetaPublicBoundaryV1",
      semantic_status: "source_unavailable",
      fact_answer_allowed: false,
      blocker: LEGACY_STATS_EVIDENCE_BLOCKER,
      case_id: "",
      funds_status: "source_unavailable",
      date_min: "",
      date_max: "",
    };
  }

  async getStatsTree(payload: { case_id: string; tab: "byName" | "byCard" }): Promise<StatsV2TreeDTO> {
    const caseId = text(payload.case_id);
    const revision = this.captureCaseContext(caseId);
    this.requireCaseContext(caseId, revision);
    return {
      contract: "StatsTreePublicBoundaryV1",
      semantic_status: "source_unavailable",
      fact_answer_allowed: false,
      blocker: LEGACY_STATS_EVIDENCE_BLOCKER,
      groups: [],
    };
  }

  async buildFlowGraph(payload: Record<string, unknown>): Promise<FlowGraphData> {
    const canonical = canonicalizeFlowCasePayload({
      ...(payload as Record<string, unknown>)
    });
    const caseId = canonical.caseId;
    const requestPayload = canonical.payload;
    this.requireCaseBinding(caseId);
    const buildTraceId = text(requestPayload.trace_id || requestPayload.traceId) || createTraceId("flow-build", caseId);
    requestPayload.trace_id = buildTraceId;

    this.revokeDirectBuilds();
    const token = this.buildToken;
    await this.cancelActiveFlowBuild();
    this.requireBuildContext(caseId, token);
    const startedAt = typeof performance !== "undefined" && typeof performance.now === "function" ? performance.now() : Date.now();
    const directBuild = shouldUseDirectBuildPath(requestPayload);

    if (directBuild) {
      try {
        const data = await this.executeDirectBuild(requestPayload, token);
        this.requireBuildContext(caseId, token);
        this.recordBuildPerf("succeeded", {
          traceId: buildTraceId,
          snapshotRef: asFlowSnapshotRef(data.result_snapshot_ref),
          data,
          startedAt
        });
        return data;
      } catch (error) {
        const message = toErrorMessage(error, "图谱生成失败");
        if (this.isBuildContextCurrent(caseId, token)) {
          this.recordBuildPerf(/cancel/i.test(message) ? "canceled" : "failed", {
            traceId: buildTraceId,
            startedAt,
            errorMessage: message
          });
        }
        throw error;
      }
    }

    const createResponse = await this.client.post<unknown>("/api/v1/analysis/flow/jobs", requestPayload);
    this.requireBuildContext(caseId, token);
    const job = projectFlowBuildJobBoundary(createResponse.data, caseId);
    const jobId = text(job.job_id);
    if (!jobId) {
      throw new Error("图谱任务创建失败");
    }

    this.activeBuild = {
      token,
      jobId,
      caseId,
      traceId: buildTraceId,
      startedAt
    };

    try {
      const data = await this.waitForFlowBuildResult(jobId, caseId, token);
      this.requireBuildContext(caseId, token);
      if (this.isActiveFlowBuildToken(token, jobId)) {
        this.activeBuild = null;
      }
      return data;
    } catch (error) {
      if (this.isActiveFlowBuildToken(token, jobId)) {
        this.activeBuild = null;
      }
      throw error;
    }
  }

  async warmFlowGraph(payload: Record<string, unknown>): Promise<void> {
    const canonical = canonicalizeFlowCasePayload({
      ...(payload as Record<string, unknown>)
    });
    this.requireCaseBinding(canonical.caseId);
    const requestPayload = canonical.payload;
    const caseId = canonical.caseId;
    if (!shouldUseDirectBuildPath(requestPayload)) {
      return;
    }
    const traceId =
      text(requestPayload.trace_id || requestPayload.traceId) || createTraceId(FLOW_DIRECT_WARM_TRACE_PREFIX, caseId);
    requestPayload.trace_id = traceId;
    try {
      await this.executeDirectBuild(requestPayload, this.buildToken);
    } catch {
      // Warmup is best-effort; user-triggered build will retry on demand.
    }
  }

  async getAccountTxnRows(payload: FlowAccountTxnRowsRequest): Promise<FlowAccountTxnRowsResult> {
    const caseId = text(payload.case_id);
    const revision = this.captureCaseContext(caseId);
    this.requireCaseContext(caseId, revision);
    return {
      contract: "StatsAccountTxnRowsPublicBoundaryV1",
      semantic_status: "source_unavailable",
      fact_answer_allowed: false,
      raw_details_exposed: false,
      blocker: LEGACY_STATS_EVIDENCE_BLOCKER,
      rows: [],
      done: false,
      next_cursor: null,
    };
  }

  async getStatsTxnRows(payload: FlowStatsTxnRowsRequest): Promise<FlowStatsTxnRowsResult> {
    const caseId = text(payload.case_id);
    const revision = this.captureCaseContext(caseId);
    this.requireCaseContext(caseId, revision);
    return {
      contract: "StatsTxnRowsPublicBoundaryV1",
      semantic_status: "blocked",
      fact_answer_allowed: false,
      raw_details_exposed: false,
      blocker: LEGACY_STATS_EVIDENCE_BLOCKER,
      rows: [],
      done: false,
      next_cursor: null,
    };
  }

  async listFlowViews(caseId: string): Promise<{ items: FlowViewDTO[] }> {
    const normalizedCaseId = text(caseId);
    const revision = this.captureCaseContext(normalizedCaseId);
    const cached = this.flowViewsCache.get(normalizedCaseId);
    if (cached) {
      this.requireCaseContext(normalizedCaseId, revision);
      return { items: cached.map((item) => cloneFlowView(item)) };
    }
    const query = new URLSearchParams();
    query.set("case_id", normalizedCaseId);
    query.set("page", "1");
    query.set("page_size", "500");
    const response = await this.client.get<unknown>(`/api/v1/analysis/flow/views?${query.toString()}`);
    this.requireCaseContext(normalizedCaseId, revision);
    const projected = projectFlowPublicViewListBoundary(response.data, normalizedCaseId);
    const items = projected.items.map((item) => cloneFlowView(item));
    this.flowViewsCache.set(normalizedCaseId, items);
    return { items: items.map((item) => cloneFlowView(item)) };
  }

  async getFlowResultSnapshot(payload: { case_id: string; snapshot_id: string }): Promise<FlowGraphData> {
    void payload;
    return requireControlledFlowArtifact();
  }

  async expandFlowResultSnapshot(snapshotId: string, payload: Record<string, unknown>): Promise<FlowGraphData> {
    const expanded = await this.expandFlowResultSnapshotDetailed(snapshotId, payload);
    return expanded.data;
  }

  async expandFlowResultSnapshotDetailed(
    snapshotId: string,
    payload: Record<string, unknown>
  ): Promise<FlowExpandedPatchResult> {
    void snapshotId;
    void payload;
    return requireControlledFlowArtifact();
  }

  async syncFlowResultSnapshotProjectionLayout(
    snapshotId: string,
    payload: Record<string, unknown>
  ): Promise<FlowProjectionLayoutSyncResult> {
    void snapshotId;
    void payload;
    return requireControlledFlowArtifact();
  }

  async computeLayoutNodePlan(payload: FlowLayoutNodePlanReq): Promise<FlowLayoutNodePlanResult> {
    void payload;
    return requireControlledFlowArtifact();
  }

  async projectGraphRenderPlan(payload: FlowGraphRenderPlanReq): Promise<FlowGraphRenderPlanResult> {
    void payload;
    return requireControlledFlowArtifact();
  }

  async projectNetworkSectorPlacement(payload: FlowNetworkSectorPlacementReq): Promise<FlowNetworkSectorPlacementResult> {
    void payload;
    return requireControlledFlowArtifact();
  }

  async projectLayoutRoleGraph(payload: FlowLayoutRoleGraphProjectionReq): Promise<FlowLayoutRoleGraphProjectionResult> {
    void payload;
    return requireControlledFlowArtifact();
  }

  async projectAnalysisGraphData(payload: FlowAnalysisGraphDataReq): Promise<FlowAnalysisGraphDataResult> {
    void payload;
    return requireControlledFlowArtifact();
  }

  async projectGraphSearch(payload: FlowGraphSearchReq): Promise<FlowGraphSearchResult> {
    void payload;
    return requireControlledFlowArtifact();
  }

  async projectProjectionLayoutSeed(payload: FlowProjectionLayoutSeedReq): Promise<FlowProjectionLayoutSeedResult> {
    void payload;
    return requireControlledFlowArtifact();
  }

  async mergeSameNameGraph(payload: FlowSameNameMergeReq): Promise<FlowSameNameMergeResult> {
    void payload;
    return requireControlledFlowArtifact();
  }

  async saveFlowView(payload: EmbeddedFlowViewSavePayload): Promise<FlowViewDTO> {
    const caseId = text(payload.case_id);
    const revision = this.captureCaseContext(caseId);
    if (text(payload.view_id)) {
      const query = new URLSearchParams({ case_id: payload.case_id });
      const response = await this.client.patch<unknown>(
        `/api/v1/analysis/flow/views/${encodeURIComponent(text(payload.view_id))}?${query.toString()}`,
        {}
      );
      this.requireCaseContext(caseId, revision);
      const item = projectFlowPublicViewBoundary(response.data, caseId);
      this.upsertCachedFlowView(caseId, item);
      return item;
    }
    const response = await this.client.post<unknown>("/api/v1/analysis/flow/views", {
      case_id: payload.case_id
    });
    this.requireCaseContext(caseId, revision);
    const item = projectFlowPublicViewBoundary(response.data, caseId);
    this.upsertCachedFlowView(caseId, item);
    return item;
  }

  async deleteFlowView(viewId: string, caseId: string): Promise<void> {
    const targetViewId = text(viewId);
    const normalizedCaseId = text(caseId);
    if (!targetViewId || !normalizedCaseId) {
      throw new Error("未选择案件");
    }
    const revision = this.captureCaseContext(normalizedCaseId);
    const query = new URLSearchParams({ case_id: normalizedCaseId });
    await this.client.delete<{ ok: boolean }>(
      `/api/v1/analysis/flow/views/${encodeURIComponent(targetViewId)}?${query.toString()}`
    );
    this.requireCaseContext(normalizedCaseId, revision);
    this.removeCachedFlowView(normalizedCaseId, targetViewId);
  }

  async renameFlowView(viewId: string, title: string, caseId: string): Promise<FlowViewDTO> {
    void title;
    const normalizedCaseId = text(caseId);
    if (!normalizedCaseId) {
      throw new Error("未选择案件");
    }
    const revision = this.captureCaseContext(normalizedCaseId);
    const query = new URLSearchParams({ case_id: normalizedCaseId });
    const response = await this.client.patch<unknown>(
      `/api/v1/analysis/flow/views/${encodeURIComponent(text(viewId))}?${query.toString()}`,
      {}
    );
    this.requireCaseContext(normalizedCaseId, revision);
    const item = projectFlowPublicViewBoundary(response.data, normalizedCaseId);
    this.upsertCachedFlowView(item.case_id, item);
    return item;
  }

  async reorderFlowViews(payload: EmbeddedFlowReorderPayload): Promise<{ count: number }> {
    const caseId = text(payload.case_id);
    const revision = this.captureCaseContext(caseId);
    const response = await this.client.post<{ count: number }>("/api/v1/analysis/flow/views/reorder", payload);
    this.requireCaseContext(caseId, revision);
    this.reorderCachedFlowViews(caseId, payload.order);
    return response.data;
  }

  async publishFlowGraphPatch(payload: EmbeddedFlowGraphPatchPayload): Promise<FlowGraphData | null> {
    void payload;
    return null;
  }

  getFlowPerfSnapshot(): Record<string, unknown> {
    this.requireCaseBinding(this.caseId);
    return {
      graphPatch: {
        publish: { ...this.perf.graphPatch.publish },
        apply: { ...this.perf.graphPatch.apply }
      },
      build: { ...this.perf.build },
      transport: {
        ws: this.wsRuntime?.getSnapshot() ?? null
      },
      runtimeBridge: {
        activeBuild: this.activeBuild
          ? {
              token: this.activeBuild.token,
              jobId: this.activeBuild.jobId,
              caseId: this.activeBuild.caseId,
              traceId: this.activeBuild.traceId,
              startedAt: this.activeBuild.startedAt
            }
          : null,
        latestCaseId: this.caseId
      }
    };
  }

  subscribeFlowGraphPatch(listener: (event: FlowGraphPatchEventData) => void): () => void {
    if (typeof listener !== "function") {
      return () => undefined;
    }
    this.graphPatchListeners.add(listener);
    return () => {
      this.graphPatchListeners.delete(listener);
    };
  }

  private isActiveFlowBuildToken(token: number, jobId?: string): boolean {
    if (
      !this.activeBuild ||
      this.activeBuild.token !== token ||
      !this.isBuildContextCurrent(this.activeBuild.caseId, token)
    ) {
      return false;
    }
    if (!jobId) {
      return true;
    }
    return this.activeBuild.jobId === text(jobId);
  }

  private requireCurrentAuthority(): void {
    if (this.disposed) {
      throw new Error("flow_bridge_disposed");
    }
    if (!isDataAnalysisRuntimeEndpointAuthorityCurrent(this.authority)) {
      throw new Error("flow_bridge_authority_stale");
    }
  }

  private requireCaseBinding(caseId: string): void {
    this.requireCurrentAuthority();
    const normalizedCaseId = text(caseId);
    if (!normalizedCaseId || !this.caseId || this.wsRuntime === null) {
      throw new Error("flow_bridge_case_binding_missing");
    }
    if (this.caseId !== normalizedCaseId || this.wsRuntime.getCaseId() !== normalizedCaseId) {
      throw new Error("flow_bridge_case_binding_mismatch");
    }
  }

  private captureCaseContext(caseId: string): number {
    this.requireCaseBinding(caseId);
    return this.caseBindingRevision;
  }

  private requireCaseContext(caseId: string, revision: number): void {
    this.requireCaseBinding(caseId);
    if (revision !== this.caseBindingRevision) {
      throw new Error("flow_case_context_revoked");
    }
  }

  private requireBuildContext(caseId: string, token: number, signal?: AbortSignal): void {
    this.requireCaseBinding(caseId);
    if (token !== this.buildToken || signal?.aborted) {
      throw new Error("flow_direct_build_context_revoked");
    }
  }

  private isBuildContextCurrent(caseId: string, token: number): boolean {
    return Boolean(
      !this.disposed &&
      token === this.buildToken &&
      this.caseId === text(caseId) &&
      this.wsRuntime?.getCaseId() === text(caseId) &&
      isDataAnalysisRuntimeEndpointAuthorityCurrent(this.authority)
    );
  }

  private revokeDirectBuilds(): void {
    this.buildToken += 1;
    for (const controller of this.directBuildControllers) {
      controller.abort();
    }
    this.directBuildControllers.clear();
  }

  private async cancelActiveFlowBuild(): Promise<void> {
    const active = this.activeBuild;
    this.activeBuild = null;
    if (!active || !text(active.jobId)) {
      return;
    }
    try {
      const query = new URLSearchParams({ case_id: active.caseId });
      await this.client.post<{ ok: boolean }>(
        `/api/v1/analysis/flow/jobs/${encodeURIComponent(active.jobId)}/cancel?${query.toString()}`
      );
    } catch {
      // Ignore cancellation race; the next request will own the UI state.
    }
  }

  private releaseWsBinding(): void {
    const unsubscribe = this.unsubscribeRuntimeEvents;
    const release = this.releaseWsRuntime;
    this.unsubscribeRuntimeEvents = null;
    this.releaseWsRuntime = null;
    this.wsRuntime = null;
    unsubscribe?.();
    release?.();
  }

  private async getFlowBuildJob(jobId: string, caseId: string): Promise<FlowBuildJobDTO> {
    const query = new URLSearchParams({ case_id: caseId });
    const response = await this.client.get<unknown>(
      `/api/v1/analysis/flow/jobs/${encodeURIComponent(text(jobId))}?${query.toString()}`
    );
    return projectFlowBuildJobBoundary(response.data, caseId);
  }

  private rememberResultSnapshot(caseId: string, data: FlowGraphData): FlowGraphData {
    void caseId;
    return projectFlowPublicResultBoundary(data);
  }

  private async executeDirectBuild(
    rawRequestPayload: Record<string, unknown>,
    token: number,
  ): Promise<FlowGraphData> {
    const canonical = canonicalizeFlowCasePayload(rawRequestPayload);
    const caseId = canonical.caseId;
    const requestPayload = canonical.payload;
    this.requireBuildContext(caseId, token);
    const controller = new AbortController();
    this.directBuildControllers.add(controller);
    try {
      const requestPromise = (async (): Promise<FlowGraphData> => {
        const response = await this.client.post<unknown>(
          "/api/v1/analysis/flow/graph",
          requestPayload,
          { signal: controller.signal },
        );
        this.requireBuildContext(caseId, token, controller.signal);
        const data = projectFlowPublicResultBoundary(response.data);
        const remembered = this.rememberResultSnapshot(caseId, data);
        this.requireBuildContext(caseId, token, controller.signal);
        return cloneFlowGraphData(remembered);
      })();
      try {
        const data = await waitForDirectBuild(requestPromise, controller.signal);
        this.requireBuildContext(caseId, token, controller.signal);
        return this.rememberResultSnapshot(caseId, cloneFlowGraphData(data));
      } catch (error) {
        if (controller.signal.aborted) {
          throw new Error("flow_direct_build_context_revoked");
        }
        throw error;
      }
    } finally {
      this.directBuildControllers.delete(controller);
    }
  }

  private async getFlowBuildResult(jobId: string, caseId: string): Promise<FlowGraphData> {
    const query = new URLSearchParams({ case_id: caseId });
    const response = await this.client.get<unknown>(
      `/api/v1/analysis/flow/jobs/${encodeURIComponent(text(jobId))}/result?${query.toString()}`
    );
    const data = projectFlowPublicResultBoundary(response.data);
    return this.rememberResultSnapshot(caseId, data);
  }

  private async getFlowBuildResultWithRetry(jobId: string, caseId: string, isCurrent: () => boolean): Promise<FlowGraphData> {
    let lastError: unknown = null;
    for (let attempt = 0; attempt < 12; attempt += 1) {
      if (!isCurrent()) {
        throw new Error("图谱生成已取消");
      }
      try {
        const result = await this.getFlowBuildResult(jobId, caseId);
        if (!isCurrent()) {
          throw new Error("图谱生成已取消");
        }
        return result;
      } catch (error) {
        if (!isCurrent()) {
          throw new Error("图谱生成已取消");
        }
        lastError = error;
        if (attempt >= 11) {
          break;
        }
        await sleep(140 + attempt * 90);
      }
    }
    throw lastError instanceof Error ? lastError : new Error("图谱结果加载失败");
  }

  private upsertCachedFlowView(caseId: string, item: FlowViewDTO): void {
    const normalizedCaseId = text(caseId || item.case_id);
    if (!normalizedCaseId) {
      return;
    }
    const cached = this.flowViewsCache.get(normalizedCaseId) || [];
    const next = cached.slice();
    const index = next.findIndex((view) => text(view.view_id) === text(item.view_id));
    if (index >= 0) {
      next[index] = cloneFlowView(item);
    } else {
      next.push(cloneFlowView(item));
    }
    this.flowViewsCache.set(normalizedCaseId, next);
  }

  private removeCachedFlowView(caseId: string, viewId: string): void {
    const normalizedCaseId = text(caseId);
    const targetViewId = text(viewId);
    const cached = this.flowViewsCache.get(normalizedCaseId);
    if (!cached) {
      return;
    }
    this.flowViewsCache.set(
      normalizedCaseId,
      cached.filter((item) => text(item.view_id) !== targetViewId).map((item) => cloneFlowView(item))
    );
  }

  private reorderCachedFlowViews(caseId: string, order: string[]): void {
    const normalizedCaseId = text(caseId);
    const cached = this.flowViewsCache.get(normalizedCaseId);
    if (!cached?.length) {
      return;
    }
    const next = cached.slice();
    const rank = new Map(order.map((viewId, index) => [text(viewId), index]));
    next.sort((left, right) => {
      const leftRank = rank.get(text(left.view_id));
      const rightRank = rank.get(text(right.view_id));
      if (leftRank == null && rightRank == null) return 0;
      if (leftRank == null) return 1;
      if (rightRank == null) return -1;
      return leftRank - rightRank;
    });
    this.flowViewsCache.set(normalizedCaseId, next.map((item) => cloneFlowView(item)));
  }

  private handleFlowViewsPatchEvent(event: WsEventEnvelope): void {
    if (event.event !== "analysis.flow.views.patch") {
      return;
    }
    const caseId = text(event.case_id);
    if (
      !caseId ||
      caseId !== this.caseId ||
      !eventBelongsToCase(event, this.caseId) ||
      !isDataAnalysisRuntimeEndpointAuthorityCurrent(this.authority)
    ) {
      return;
    }
    this.flowViewsCache.delete(caseId);
  }

  private async resolveFlowBuildSnapshot(
    jobId: string,
    caseId: string,
    _snapshotRef: FlowSnapshotRefDTO | null,
    isCurrent: () => boolean
  ): Promise<FlowGraphData> {
    return this.getFlowBuildResultWithRetry(jobId, caseId, isCurrent);
  }

  private recordBuildPerf(
    status: "succeeded" | "failed" | "canceled",
    {
      traceId = "",
      snapshotRef = null,
      data = null,
      startedAt = 0,
      errorMessage = "",
    }: {
      traceId?: string;
      snapshotRef?: FlowSnapshotRefDTO | null;
      data?: FlowGraphData | null;
      startedAt?: number;
      errorMessage?: string;
    } = {}
  ): void {
    const bucket = this.perf.build;
    const endedAt = typeof performance !== "undefined" && typeof performance.now === "function" ? performance.now() : Date.now();
    bucket.count += 1;
    if (status === "succeeded") {
      bucket.successCount += 1;
    } else {
      bucket.failedCount += 1;
    }
    bucket.lastDurationMs = Math.max(0, Math.round(endedAt - Math.max(0, asNumber(startedAt, endedAt))));
    bucket.lastAt = Date.now();
    bucket.lastStatus = status;
    const dataSnapshotRef = asFlowSnapshotRef(data?.result_snapshot_ref);
    bucket.lastSnapshotId = text(snapshotRef?.snapshot_id || snapshotRef?.graph_hash || dataSnapshotRef?.snapshot_id || "");
    bucket.lastNodeCount = Array.isArray(data?.runtime_graph?.nodes) ? data.runtime_graph.nodes.length : 0;
    bucket.lastEdgeCount = Array.isArray(data?.runtime_graph?.edges) ? data.runtime_graph.edges.length : 0;
    bucket.lastTraceId = text(traceId || bucket.lastTraceId);
    bucket.lastError = text(errorMessage);
  }

  private async waitForFlowBuildResult(jobId: string, caseId: string, token: number): Promise<FlowGraphData> {
    const wsRuntime = this.wsRuntime;
    if (!wsRuntime || wsRuntime.getCaseId() !== caseId) {
      throw new Error("flow_ws_case_binding_missing");
    }
    return new Promise((resolve, reject) => {
      let settled = false;
      let loadingResult = false;
      let pollTimer = 0;
      let timeoutTimer = 0;
      let unsubscribe: (() => void) | null = null;
      let pollCount = 0;

      const cleanup = (): void => {
        if (pollTimer) {
          window.clearTimeout(pollTimer);
          pollTimer = 0;
        }
        if (timeoutTimer) {
          window.clearTimeout(timeoutTimer);
          timeoutTimer = 0;
        }
        if (unsubscribe) {
          unsubscribe();
          unsubscribe = null;
        }
      };

      const isCurrent = (): boolean => this.isActiveFlowBuildToken(token, jobId);
      const getActiveBuild = (): ActiveFlowBuildState | null =>
        this.isActiveFlowBuildToken(token, jobId) ? this.activeBuild : null;
      const schedulePoll = (): void => {
        if (settled || loadingResult) {
          return;
        }
        const delayMs = getFlowBuildPollDelayMs({
          wsReady: wsRuntime.getStatus() === "open",
          pollCount,
        });
        pollTimer = window.setTimeout(() => {
          pollCount += 1;
          void poll();
        }, delayMs);
      };

      const rejectFlowBuild = (message: string): void => {
        if (settled) {
          return;
        }
        const activeBuild = getActiveBuild();
        if (activeBuild) {
          this.recordBuildPerf(/cancel/i.test(message) ? "canceled" : "failed", {
            traceId: activeBuild.traceId,
            startedAt: activeBuild.startedAt,
            errorMessage: message,
          });
        }
        settled = true;
        cleanup();
        reject(new Error(text(message) || "图谱生成失败"));
      };

      const resolveFlowBuild = async (snapshotRef?: FlowSnapshotRefDTO | null, traceId = ""): Promise<void> => {
        if (settled || loadingResult) {
          return;
        }
        if (!isCurrent()) {
          rejectFlowBuild("图谱生成已取消");
          return;
        }
        loadingResult = true;
        try {
          const data = await this.resolveFlowBuildSnapshot(jobId, caseId, snapshotRef ?? null, isCurrent);
          if (settled) {
            return;
          }
          if (!isCurrent()) {
            rejectFlowBuild("图谱生成已取消");
            return;
          }
          const activeBuild = getActiveBuild();
          this.recordBuildPerf("succeeded", {
            traceId: text(traceId) || text(activeBuild?.traceId),
            snapshotRef: asFlowSnapshotRef(data?.result_snapshot_ref) || snapshotRef || null,
            data,
            startedAt: activeBuild?.startedAt || 0,
          });
          settled = true;
          cleanup();
          resolve(data);
        } catch (error) {
          loadingResult = false;
          rejectFlowBuild(toErrorMessage(error, "图谱结果加载失败"));
        }
      };

      const handleTerminalState = (jobState: Record<string, unknown>): void => {
        if (settled) {
          return;
        }
        if (!isCurrent()) {
          rejectFlowBuild("图谱生成已取消");
          return;
        }
        const status = text(jobState.status).toLowerCase();
        if (status === "succeeded") {
          void resolveFlowBuild(null, "");
          return;
        }
        if (status === "failed" || status === "canceled") {
          rejectFlowBuild(status === "canceled" ? "图谱生成已取消" : "图谱生成失败");
        }
      };

      const handleFlowEvent = (event: WsEventEnvelope): void => {
        if (!isFlowBuildEvent(event) || !eventBelongsToCase(event, caseId)) {
          return;
        }
        if (text(event.job_id) !== jobId) {
          return;
        }
        if (event.event === "analysis.flow.build.completed") {
          handleTerminalState({ status: "succeeded" });
          return;
        }
        if (event.event === "analysis.flow.graph.patch") {
          return;
        }
        if (event.event === "analysis.flow.build.failed") {
          handleTerminalState({ status: "failed" });
        }
      };

      const poll = async (): Promise<void> => {
        if (settled || loadingResult) {
          return;
        }
        if (!isCurrent()) {
          rejectFlowBuild("图谱生成已取消");
          return;
        }
        try {
          const job = await this.getFlowBuildJob(jobId, caseId);
          handleTerminalState(job as unknown as Record<string, unknown>);
        } catch {
          // Keep waiting for poll or WS terminal state until timeout.
        } finally {
          if (!settled && !loadingResult) {
            schedulePoll();
          }
        }
      };

      unsubscribe = wsRuntime.onEvent((event) => {
        handleFlowEvent(event);
      });

      timeoutTimer = window.setTimeout(() => {
        rejectFlowBuild("图谱生成超时");
      }, FLOW_BUILD_TIMEOUT_MS);

      schedulePoll();
    });
  }
}

let sharedEmbeddedFlowBridgeService: EmbeddedFlowBridgeService | null = null;
let sharedEmbeddedFlowBridgeServiceAuthorityIdentity = "";
let sharedEmbeddedFlowBridgeServiceAuthoritySignal: AbortSignal | null = null;

export function getSharedEmbeddedFlowBridgeService(options: { apiBaseUrl?: string } = {}): EmbeddedFlowBridgeService {
  const authority = resolveDataAnalysisRuntimeEndpointAuthority({
    requestedApiBaseUrl: text(options.apiBaseUrl),
  });
  const identity = authorityIdentity(authority);
  if (
    !sharedEmbeddedFlowBridgeService ||
    (sharedEmbeddedFlowBridgeService instanceof FlowBridgeServiceImpl && sharedEmbeddedFlowBridgeService.isDisposed()) ||
    sharedEmbeddedFlowBridgeServiceAuthorityIdentity !== identity ||
    sharedEmbeddedFlowBridgeServiceAuthoritySignal !== authority.lease.signal
  ) {
    if (sharedEmbeddedFlowBridgeService instanceof FlowBridgeServiceImpl) {
      sharedEmbeddedFlowBridgeService.dispose();
    }
    sharedEmbeddedFlowBridgeService = new FlowBridgeServiceImpl(authority);
    sharedEmbeddedFlowBridgeServiceAuthorityIdentity = identity;
    sharedEmbeddedFlowBridgeServiceAuthoritySignal = authority.lease.signal;
  }
  return sharedEmbeddedFlowBridgeService;
}

export function createEmbeddedFlowBridgeService(options: { apiBaseUrl: string }): EmbeddedFlowBridgeService {
  const authority = resolveDataAnalysisRuntimeEndpointAuthority({
    requestedApiBaseUrl: text(options.apiBaseUrl),
  });
  return new FlowBridgeServiceImpl(authority);
}
