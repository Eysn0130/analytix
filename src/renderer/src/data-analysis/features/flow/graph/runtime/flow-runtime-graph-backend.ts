import type { EmbeddedFlowBridgeService } from "../api/flow-bridge-service";
import type { FlowGraphData } from "../../../../services/analysis/flow-api";
import {
  readStrictAliasedFiniteNumber,
  readStrictAliasedNonNegativeSafeInteger,
} from "../../../../services/analysis/flow-direct-build-policy";
import {
  buildFlowRuntimeGraphResponse,
  sanitizeFlowRuntimeGraphPayload,
  sanitizeFlowSnapshotRef,
} from "../adapters/flow-runtime-bridge-mapper";
import { asFlowSnapshotRef, asRuntimeRevision, cloneFlowGraphData } from "../model/graph-data-model";
import {
  asArray,
  asNullableObject,
  asObject,
  clampInt,
  type EmbeddedFlowCallback,
  invoke,
  mapDirection,
  parseJson,
  text,
  toFiniteNumber,
  uniqueStrings,
} from "./flow-runtime-backend-utils";

export interface FlowRuntimeGraphBackendMethods {
  getFlowGraph: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  expandFlowResultSnapshot: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  syncFlowResultSnapshotProjectionLayout: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  computeLayoutNodePlan: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  projectNetworkSectorPlacement: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  projectGraphRenderPlan: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  projectGraphSearch: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  projectProjectionLayoutSeed: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  projectLayoutRoleGraph: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  projectAnalysisGraphData: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  mergeSameNameGraph: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  getFlowResultSnapshot: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  publishFlowGraphPatch: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
}

interface CreateFlowRuntimeGraphBackendMethodsOptions {
  service: EmbeddedFlowBridgeService;
  targetWindow: Window;
  getCurrentCaseId: () => string;
}

function hasOwn(source: Record<string, unknown>, key: string): boolean {
  return Object.prototype.hasOwnProperty.call(source, key);
}

function readSchemaBoolean(
  source: Record<string, unknown>,
  keys: readonly string[],
  schemaDefault = false
): boolean {
  const values = keys.filter((key) => hasOwn(source, key)).map((key) => source[key]);
  if (!values.length) {
    return schemaDefault;
  }
  if (values.some((value) => typeof value !== "boolean")) {
    throw new Error("flow_filter_boolean_invalid");
  }
  if (values.some((value) => value !== values[0])) {
    throw new Error("flow_filter_boolean_conflict");
  }
  return values[0] as boolean;
}

function shouldReturnFlowGraphObjectResponse(
  payload: Record<string, unknown>,
  response: Record<string, unknown>
): boolean {
  const layout = text(payload.layout || payload.layoutPreset).toLowerCase();
  return layout === "network" && response.runtime_graph_passthrough === true;
}

export function createFlowRuntimeGraphBackendMethods(
  options: CreateFlowRuntimeGraphBackendMethodsOptions
): FlowRuntimeGraphBackendMethods {
  const { service, targetWindow, getCurrentCaseId } = options;

  return {
    async getFlowGraph(payloadJson, callback) {
      const payload = parseJson<Record<string, unknown>>(payloadJson, {});
      const caseId = text(payload.caseId || payload.case_id) || getCurrentCaseId();
      const seeds = uniqueStrings(
        payload.seeds ||
          payload.selectedSeeds ||
          payload.selected_seeds ||
          (text(payload.focusId || payload.focus_id) ? [payload.focusId || payload.focus_id] : [])
      );
      const focusId = text(payload.focusId || payload.focus_id);
      const focusName = text(payload.focusName || payload.focus_name);
      const focusLabel = text(payload.focusLabel || payload.focus_label);
      let focusSelfOnly: boolean;
      let focusOnly: boolean;
      let focusUnknownName: boolean;
      let includeMissingCounterparty: boolean;
      let focusCounterpartyStrict: boolean;
      try {
        focusSelfOnly = readSchemaBoolean(payload, ["focusSelfOnly", "focus_self_only", "onlySelf"]);
        focusOnly = readSchemaBoolean(payload, ["focusOnly", "focus_only"]);
        focusUnknownName = readSchemaBoolean(payload, ["focusUnknownName", "focus_unknown_name"]);
        includeMissingCounterparty = readSchemaBoolean(payload, [
          "includeMissingCounterparty",
          "include_missing_counterparty",
          "includeUnknownAccount",
          "include_unknown_account",
        ]);
        focusCounterpartyStrict = readSchemaBoolean(payload, [
          "focusCounterpartyStrict",
          "focus_counterparty_strict",
        ]);
      } catch (error) {
        invoke(callback, JSON.stringify({ ok: false, error: text((error as Error).message) || "图谱筛选参数无效" }));
        return;
      }
      if (!caseId) {
        invoke(callback, JSON.stringify({ ok: false, error: "未选择案件" }));
        return;
      }
      if (focusSelfOnly && (focusId || focusName || focusLabel)) {
        invoke(
          callback,
          JSON.stringify({
            ok: true,
            semantic_status: "blocked",
            publication_status: "blocked",
            fact_answer_allowed: false,
            blocker: "flow_self_only_placeholder_requires_evidence",
            nodes: [
              {
                id: focusId || focusLabel || focusName,
                title: focusLabel || focusName || focusId,
                name: focusName,
                display_id: focusId,
                ntype: "seed",
                placeholder: true,
                evidence_status: "unverified",
                fact_answer_allowed: false,
              },
            ],
            edges: [],
            stats: {},
          })
        );
        return;
      }
      if (!seeds.length) {
        invoke(callback, JSON.stringify({ ok: false, error: "请先选择中心账户" }));
        return;
      }
      try {
        const data = await service.buildFlowGraph({
          case_id: caseId,
          seeds,
          depth: clampInt(payload.hop || payload.depth, 1, 1, 8),
          direction: mapDirection(payload.dir || payload.direction),
          min_amount: Math.max(0, toFiniteNumber(payload.minAmount ?? payload.min_amount, 0)),
          source: text(payload.source),
          request_id: text(payload.requestId || payload.request_id),
          view: text(payload.view),
          layout: text(payload.layout || payload.layoutPreset),
          date_start: text(payload.dateStart || payload.date_start),
          date_end: text(payload.dateEnd || payload.date_end),
          focus_id: focusId,
          focus_name: focusName,
          focus_key_type: text(payload.focusKeyType || payload.focus_key_type || payload.keyType),
          focus_label: focusLabel,
          focus_only: focusOnly,
          focus_unknown_name: focusUnknownName,
          include_missing_counterparty: includeMissingCounterparty,
          focus_counterparty_strict: focusCounterpartyStrict,
          left_seeds: uniqueStrings(payload.leftSeeds || payload.left_seeds),
          expected_total_amount:
            readStrictAliasedFiniteNumber(payload, "expectedTotalAmount", "expected_total_amount") ?? null,
          expected_row_count:
            readStrictAliasedNonNegativeSafeInteger(payload, "expectedRowCount", "expected_row_count") ?? null,
          focus_ids: uniqueStrings([focusId, ...asArray(payload.focusIds || payload.focus_ids)]),
          focus_names: uniqueStrings([focusName, ...asArray(payload.focusNames || payload.focus_names)]),
          focus_placeholder_kinds: uniqueStrings(payload.focusPlaceholderKinds || payload.focus_placeholder_kinds || []),
          graph: {
            ...asObject(payload.graph),
            render_mode: text(asObject(payload.graph).render_mode || asObject(payload.graph).renderMode || "auto") || "auto",
          },
          drill: asObject(payload.drill),
        });
        const response = buildFlowRuntimeGraphResponse(targetWindow, payload, cloneFlowGraphData(data));
        invoke(callback, shouldReturnFlowGraphObjectResponse(payload, response) ? response : JSON.stringify(response));
      } catch (error) {
        invoke(callback, JSON.stringify({ ok: false, error: text((error as Error)?.message) || "图谱生成失败" }));
      }
    },
    async expandFlowResultSnapshot(payloadJson, callback) {
      const payload = parseJson<Record<string, unknown>>(payloadJson, {});
      const caseId = text(payload.caseId || payload.case_id) || getCurrentCaseId();
      const snapshotId = text(payload.snapshotId || payload.snapshot_id || payload.resultSnapshotId || payload.result_snapshot_id);
      if (!caseId || !snapshotId) {
        invoke(callback, JSON.stringify({ ok: false, error: "缺少图谱快照" }));
        return;
      }
      try {
        const expanded = await service.expandFlowResultSnapshotDetailed(snapshotId, {
          case_id: caseId,
          search_query: text(payload.searchQuery || payload.search_query || payload.query || payload.q),
          search_limit: clampInt(payload.searchLimit ?? payload.search_limit ?? payload.matchLimit ?? payload.match_limit, 24, 1, 256),
          cluster_ids: uniqueStrings(payload.clusterIds || payload.cluster_ids || []),
          tile_ids: uniqueStrings(payload.tileIds || payload.tile_ids || []),
          node_ids: uniqueStrings(payload.nodeIds || payload.node_ids || []),
          path_node_ids: uniqueStrings(payload.pathNodeIds || payload.path_node_ids || []),
          viewport: asObject(payload.viewport),
          include_neighbors: payload.includeNeighbors ?? payload.include_neighbors ?? true,
          neighbor_depth: clampInt(payload.neighborDepth ?? payload.neighbor_depth, 1, 0, 3),
          materialize_limit: clampInt(payload.materializeLimit ?? payload.materialize_limit, 4000, 1, 50000),
          reset: !!payload.reset,
          graph: {
            ...asObject(payload.graph),
            render_mode: text(asObject(payload.graph).render_mode || asObject(payload.graph).renderMode || "skeleton") || "skeleton",
          },
          drill: asObject(payload.drill),
        });
        const data = cloneFlowGraphData(expanded?.data || ({} as FlowGraphData));
        const patch = expanded?.patch || {};
        const patchObject = asObject(patch as unknown);
        invoke(
          callback,
          JSON.stringify({
            ...buildFlowRuntimeGraphResponse(targetWindow, payload, data),
            patch_kind: text(patchObject.patch_kind || "full") || "full",
            base_snapshot_ref: sanitizeFlowSnapshotRef(
              targetWindow,
              asFlowSnapshotRef(patchObject.base_snapshot_ref)
            ),
            base_runtime_revision: asRuntimeRevision(patchObject.base_runtime_revision),
            runtime_revision: asRuntimeRevision(patchObject.runtime_revision),
            runtime_graph_patch:
              patchObject.runtime_graph_patch && typeof patchObject.runtime_graph_patch === "object"
                ? patchObject.runtime_graph_patch
                : null,
            projection_action: text(patchObject.projection_action || "expand") || "expand",
          })
        );
      } catch (error) {
        invoke(callback, JSON.stringify({ ok: false, error: text((error as Error)?.message) || "图谱展开失败" }));
      }
    },
    async syncFlowResultSnapshotProjectionLayout(payloadJson, callback) {
      const payload = parseJson<Record<string, unknown>>(payloadJson, {});
      const caseId = text(payload.caseId || payload.case_id) || getCurrentCaseId();
      const snapshotId = text(payload.snapshotId || payload.snapshot_id || payload.resultSnapshotId || payload.result_snapshot_id);
      if (!caseId || !snapshotId) {
        invoke(callback, JSON.stringify({ ok: false, error: "缺少图谱快照" }));
        return;
      }
      const nodeRows = Array.isArray(payload.nodes)
        ? payload.nodes.filter((row) => row && typeof row === "object")
        : [];
      try {
        const result = await service.syncFlowResultSnapshotProjectionLayout(snapshotId, {
          case_id: caseId,
          snapshot_id: snapshotId,
          viewport: asObject(payload.viewport),
          graph_size: asObject(payload.graph_size || payload.graphSize),
          nodes: nodeRows,
        });
        invoke(callback, JSON.stringify({ ok: true, ...(result && typeof result === "object" ? result : {}) }));
      } catch (error) {
        invoke(callback, JSON.stringify({ ok: false, error: text((error as Error)?.message) || "图谱布局同步失败" }));
      }
    },
    async computeLayoutNodePlan(payloadJson, callback) {
      const payload = parseJson<Record<string, unknown>>(payloadJson, {});
      const caseId = text(payload.caseId || payload.case_id) || getCurrentCaseId();
      const operation = text(payload.operation) as
        | "project"
        | "cache_key"
        | "apply"
        | "apply_worker"
        | "clear"
        | "network_plan"
        | "network_community_quality";
      if (!caseId) {
        invoke(callback, JSON.stringify({ ok: false, error: "未选择案件" }));
        return;
      }
      if (
        operation !== "project" &&
        operation !== "cache_key" &&
        operation !== "apply" &&
        operation !== "apply_worker" &&
        operation !== "clear" &&
        operation !== "network_plan" &&
        operation !== "network_community_quality"
      ) {
        invoke(callback, JSON.stringify({ ok: false, error: "图谱布局节点计划操作无效" }));
        return;
      }
      try {
        const result = await service.computeLayoutNodePlan({
          case_id: caseId,
          operation,
          nodes: Array.isArray(payload.nodes) ? (payload.nodes as Array<Record<string, unknown>>) : [],
          edges: Array.isArray(payload.edges) ? (payload.edges as Array<Record<string, unknown>>) : [],
          nodesById: asObject(payload.nodesById || payload.nodes_by_id),
          workerNodes: Array.isArray(payload.workerNodes)
            ? (payload.workerNodes as Array<Record<string, unknown>>)
            : [],
          metaKeys: Array.isArray(payload.metaKeys || payload.meta_keys)
            ? ((payload.metaKeys || payload.meta_keys) as string[])
            : [],
          algoVersion: text(payload.algoVersion || payload.algo_version),
          mode: text(payload.mode || payload.preset),
          focusId: text(payload.focusId || payload.focus_id),
          graphMutationSeq: Number.isFinite(Number(payload.graphMutationSeq ?? payload.graph_mutation_seq ?? payload.mutationSeq))
            ? Number(payload.graphMutationSeq ?? payload.graph_mutation_seq ?? payload.mutationSeq)
            : 0,
          layoutDirection: asObject(payload.layoutDirection || payload.layout_direction),
          seedContext: asObject(payload.seedContext || payload.seed_context),
          semantic: asObject(payload.semantic),
          resultSnapshotRef: asObject(payload.resultSnapshotRef || payload.result_snapshot_ref),
        });
        invoke(callback, JSON.stringify({ ok: true, ...(result && typeof result === "object" ? result : {}) }));
      } catch (error) {
        invoke(callback, JSON.stringify({ ok: false, error: text((error as Error)?.message) || "图谱布局节点计划失败" }));
      }
    },
    async projectNetworkSectorPlacement(payloadJson, callback) {
      const payload = parseJson<Record<string, unknown>>(payloadJson, {});
      const caseId = text(payload.caseId || payload.case_id) || getCurrentCaseId();
      if (!caseId) {
        invoke(callback, JSON.stringify({ ok: false, error: "未选择案件" }));
        return;
      }
      try {
        const sectorPayload: Record<string, unknown> = { ...payload };
        delete sectorPayload.caseId;
        delete sectorPayload.case_id;
        const result = await service.projectNetworkSectorPlacement({
          ...sectorPayload,
          case_id: caseId,
        });
        invoke(callback, JSON.stringify({ ok: true, ...(result && typeof result === "object" ? result : {}) }));
      } catch (error) {
        invoke(callback, JSON.stringify({ ok: false, error: text((error as Error)?.message) || "图谱网络扇区布局失败" }));
      }
    },
    async projectGraphRenderPlan(payloadJson, callback) {
      const payload = parseJson<Record<string, unknown>>(payloadJson, {});
      const caseId = text(payload.caseId || payload.case_id) || getCurrentCaseId();
      if (!caseId) {
        invoke(callback, JSON.stringify({ ok: false, error: "未选择案件" }));
        return;
      }
      try {
        const rawPlans = payload.renderPlans || payload.render_plans;
        const request: {
          case_id: string;
          renderPlans?: Array<Record<string, unknown>>;
          renderPlan?: Record<string, unknown> | null;
          viewportExpandTargets?: Record<string, unknown> | null;
        } = {
          case_id: caseId,
        };
        if (Array.isArray(rawPlans)) {
          request.renderPlans = rawPlans as Array<Record<string, unknown>>;
        }
        const singlePlan = asNullableObject(payload.renderPlan || payload.render_plan);
        if (singlePlan) {
          request.renderPlan = singlePlan;
        }
        const viewportExpandTargets = asNullableObject(payload.viewportExpandTargets || payload.viewport_expand_targets);
        if (viewportExpandTargets) {
          request.viewportExpandTargets = viewportExpandTargets;
        }
        const result = await service.projectGraphRenderPlan(request);
        invoke(callback, JSON.stringify({ ok: true, ...(result && typeof result === "object" ? result : {}) }));
      } catch (error) {
        invoke(callback, JSON.stringify({ ok: false, error: text((error as Error)?.message) || "图谱展示计划失败" }));
      }
    },
    async projectLayoutRoleGraph(payloadJson, callback) {
      const payload = parseJson<Record<string, unknown>>(payloadJson, {});
      const caseId = text(payload.caseId || payload.case_id) || getCurrentCaseId();
      if (!caseId) {
        invoke(callback, JSON.stringify({ ok: false, error: "未选择案件" }));
        return;
      }
      try {
        const result = await service.projectLayoutRoleGraph({
          case_id: caseId,
          nodes: Array.isArray(payload.nodes) ? (payload.nodes as Array<Record<string, unknown>>) : [],
          edges: Array.isArray(payload.edges) ? (payload.edges as Array<Record<string, unknown>>) : [],
          traversal: asNullableObject(payload.traversal),
          demotion: asNullableObject(payload.demotion),
          promotion: asNullableObject(payload.promotion),
          roleResolution: asNullableObject(payload.roleResolution || payload.role_resolution),
          clusterResolution: asNullableObject(payload.clusterResolution || payload.cluster_resolution),
          semanticPipeline: asNullableObject(payload.semanticPipeline || payload.semantic_pipeline),
        });
        invoke(callback, JSON.stringify({ ok: true, ...(result && typeof result === "object" ? result : {}) }));
      } catch (error) {
        invoke(callback, JSON.stringify({ ok: false, error: text((error as Error)?.message) || "图谱角色投影失败" }));
      }
    },
    async projectAnalysisGraphData(payloadJson, callback) {
      const payload = parseJson<Record<string, unknown>>(payloadJson, {});
      const caseId = text(payload.caseId || payload.case_id) || getCurrentCaseId();
      if (!caseId) {
        invoke(callback, JSON.stringify({ ok: false, error: "未选择案件" }));
        return;
      }
      const filterMinValue = payload.filterMin ?? payload.filter_min;
      const filterMaxValue = payload.filterMax ?? payload.filter_max;
      const filterMin = Number.isFinite(Number(filterMinValue)) ? Number(filterMinValue) : null;
      const filterMax = Number.isFinite(Number(filterMaxValue)) ? Number(filterMaxValue) : null;
      try {
        const result = await service.projectAnalysisGraphData({
          case_id: caseId,
          source: asObject(payload.source),
          filterMin,
          filterMax,
          collapseChildren: Boolean(payload.collapseChildren ?? payload.collapse_children),
        });
        invoke(callback, JSON.stringify({ ok: true, ...(result && typeof result === "object" ? result : {}) }));
      } catch (error) {
        invoke(callback, JSON.stringify({ ok: false, error: text((error as Error)?.message) || "分析图谱计算失败" }));
      }
    },
    async projectGraphSearch(payloadJson, callback) {
      const payload = parseJson<Record<string, unknown>>(payloadJson, {});
      const caseId = text(payload.caseId || payload.case_id) || getCurrentCaseId();
      if (!caseId) {
        invoke(callback, JSON.stringify({ ok: false, error: "未选择案件" }));
        return;
      }
      try {
        const result = await service.projectGraphSearch({
          case_id: caseId,
          nodes: Array.isArray(payload.nodes) ? (payload.nodes as Array<Record<string, unknown>>) : [],
          query: text(payload.query || payload.searchQuery || payload.search_query),
          limit: clampInt(payload.limit ?? payload.searchLimit ?? payload.search_limit, 1, 1, 1000),
        });
        invoke(callback, JSON.stringify({ ok: true, ...(result && typeof result === "object" ? result : {}) }));
      } catch (error) {
        invoke(callback, JSON.stringify({ ok: false, error: text((error as Error)?.message) || "图谱搜索失败" }));
      }
    },
    async projectProjectionLayoutSeed(payloadJson, callback) {
      const payload = parseJson<Record<string, unknown>>(payloadJson, {});
      const caseId = text(payload.caseId || payload.case_id) || getCurrentCaseId();
      if (!caseId) {
        invoke(callback, JSON.stringify({ ok: false, error: "未选择案件" }));
        return;
      }
      try {
        const result = await service.projectProjectionLayoutSeed({
          case_id: caseId,
          nodes: Array.isArray(payload.nodes) ? (payload.nodes as Array<Record<string, unknown>>) : [],
          baseNodes: Array.isArray(payload.baseNodes || payload.base_nodes)
            ? ((payload.baseNodes || payload.base_nodes) as Array<Record<string, unknown>>)
            : [],
        });
        invoke(callback, JSON.stringify({ ok: true, ...(result && typeof result === "object" ? result : {}) }));
      } catch (error) {
        invoke(callback, JSON.stringify({ ok: false, error: text((error as Error)?.message) || "投影布局种子失败" }));
      }
    },
    async mergeSameNameGraph(payloadJson, callback) {
      const payload = parseJson<Record<string, unknown>>(payloadJson, {});
      const caseId = text(payload.caseId || payload.case_id) || getCurrentCaseId();
      if (!caseId) {
        invoke(callback, JSON.stringify({ ok: false, error: "未选择案件" }));
        return;
      }
      try {
        const result = await service.mergeSameNameGraph({
          case_id: caseId,
          nodes: Array.isArray(payload.nodes) ? (payload.nodes as Array<Record<string, unknown>>) : [],
          edges: Array.isArray(payload.edges) ? (payload.edges as Array<Record<string, unknown>>) : [],
          mode: payload.mode === "net" ? "net" : "gross",
        });
        invoke(callback, JSON.stringify({ ok: true, ...(result && typeof result === "object" ? result : {}) }));
      } catch (error) {
        invoke(callback, JSON.stringify({ ok: false, error: text((error as Error)?.message) || "同名节点合并失败" }));
      }
    },
    async getFlowResultSnapshot(payloadJson, callback) {
      const payload = parseJson<Record<string, unknown>>(payloadJson, {});
      const caseId = text(payload.caseId) || getCurrentCaseId();
      const snapshotId = text(payload.snapshotId || payload.snapshot_id);
      if (!caseId || !snapshotId) {
        invoke(
          callback,
          JSON.stringify({
            ok: false,
            error: "结果快照参数缺失",
            runtime_graph: { nodes: [], edges: [] },
            result_snapshot_ref: null,
          })
        );
        return;
      }
      try {
        const data: FlowGraphData = await service.getFlowResultSnapshot({ case_id: caseId, snapshot_id: snapshotId } as never);
        invoke(
          callback,
          JSON.stringify({
            ok: true,
            result_snapshot_ref: sanitizeFlowSnapshotRef(
              targetWindow,
              asFlowSnapshotRef(data.result_snapshot_ref)
            ),
            runtime_graph: sanitizeFlowRuntimeGraphPayload(targetWindow, data.runtime_graph),
            stats: asObject(data.stats),
            projection: asNullableObject((data as unknown as Record<string, unknown>).projection),
            render_hints: asNullableObject((data as unknown as Record<string, unknown>).render_hints),
          })
        );
      } catch (error) {
        invoke(
          callback,
          JSON.stringify({
            ok: false,
            error: text((error as Error)?.message) || "结果快照加载失败",
            runtime_graph: { nodes: [], edges: [] },
            result_snapshot_ref: null,
          })
        );
      }
    },
    async publishFlowGraphPatch(payloadJson, callback) {
      const payload = parseJson<Record<string, unknown>>(payloadJson, {});
      const caseId = text(payload.caseId || payload.case_id) || getCurrentCaseId();
      if (!caseId) {
        invoke(callback, JSON.stringify({ ok: false, error: "未选择案件" }));
        return;
      }
      try {
        const data = await service.publishFlowGraphPatch({ ...payload, case_id: caseId });
        invoke(
          callback,
          JSON.stringify({
            ok: true,
            patch_kind: text(payload.patch_kind || "full") || "full",
            patch_scope: text(payload.patch_scope || "local-update") || "local-update",
            patch_source: text(payload.patch_source || "local") || "local",
            node_count: Array.isArray(data?.runtime_graph?.nodes) ? data.runtime_graph.nodes.length : 0,
            edge_count: Array.isArray(data?.runtime_graph?.edges) ? data.runtime_graph.edges.length : 0,
          })
        );
      } catch (error) {
        invoke(callback, JSON.stringify({ ok: false, error: text((error as Error)?.message) || "图谱增量同步失败" }));
      }
    },
  };
}
