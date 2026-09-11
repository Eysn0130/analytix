import type { EmbeddedFlowBridgeService } from "../api/flow-bridge-service";
import { mapBackendViewToRuntimeView } from "../adapters/flow-runtime-bridge-mapper";
import {
  asObject,
  clampInt,
  type EmbeddedFlowCallback,
  invoke,
  parseJson,
  text,
  uniqueStrings,
} from "./flow-runtime-backend-utils";

export interface FlowRuntimeViewBackendMethods {
  listFlowViews: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  saveFlowView: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  deleteFlowView: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  renameFlowView: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  reorderFlowViews: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
}

interface CreateFlowRuntimeViewBackendMethodsOptions {
  service: EmbeddedFlowBridgeService;
  targetWindow: Window;
  getCurrentCaseId: () => string;
}

export function createFlowRuntimeViewBackendMethods(
  options: CreateFlowRuntimeViewBackendMethodsOptions
): FlowRuntimeViewBackendMethods {
  const { service, targetWindow, getCurrentCaseId } = options;

  return {
    async listFlowViews(payloadJson, callback) {
      const payload = parseJson<Record<string, unknown>>(payloadJson, {});
      const caseId = text(payload.caseId) || getCurrentCaseId();
      if (!caseId) {
        invoke(callback, JSON.stringify({ ok: false, error: "未选择案件", views: [] }));
        return;
      }
      try {
        const data = await service.listFlowViews(caseId);
        const views = (Array.isArray(data.items) ? data.items : []).map((item) => mapBackendViewToRuntimeView(targetWindow, item));
        invoke(callback, JSON.stringify({ ok: true, views }));
      } catch (error) {
        invoke(callback, JSON.stringify({ ok: false, error: text((error as Error)?.message) || "视图列表加载失败", views: [] }));
      }
    },
    async saveFlowView(payloadJson, callback) {
      const payload = parseJson<Record<string, unknown>>(payloadJson, {});
      const caseId = text(payload.caseId) || getCurrentCaseId();
      if (!caseId) {
        invoke(callback, JSON.stringify({ ok: false, error: "未选择案件" }));
        return;
      }
      const rawView = payload.view && typeof payload.view === "string" ? parseJson<Record<string, unknown>>(payload.view, {}) : asObject(payload.view);
      try {
        const savedItem = await service.saveFlowView({
          case_id: caseId,
          view_id: rawView.saved ? text(rawView.id) : "",
        });
        invoke(callback, JSON.stringify({ ok: true, id: text(savedItem.view_id) }));
      } catch (error) {
        invoke(callback, JSON.stringify({ ok: false, error: text((error as Error)?.message) || "视图存储失败" }));
      }
    },
    async deleteFlowView(payloadJson, callback) {
      const payload = parseJson<Record<string, unknown>>(payloadJson, {});
      const viewId = text(payload.viewId);
      const caseId = text(payload.caseId) || getCurrentCaseId();
      if (!viewId || !caseId) {
        invoke(callback, JSON.stringify({ ok: false, error: "参数错误" }));
        return;
      }
      try {
        await service.deleteFlowView(viewId, caseId);
        invoke(callback, JSON.stringify({ ok: true }));
      } catch (error) {
        invoke(callback, JSON.stringify({ ok: false, error: text((error as Error)?.message) || "视图删除失败" }));
      }
    },
    async renameFlowView(payloadJson, callback) {
      const payload = parseJson<Record<string, unknown>>(payloadJson, {});
      const viewId = text(payload.viewId);
      const title = text(payload.title);
      const caseId = text(payload.caseId) || getCurrentCaseId();
      if (!viewId || !caseId) {
        invoke(callback, JSON.stringify({ ok: false, error: "参数错误" }));
        return;
      }
      try {
        await service.renameFlowView(viewId, title || "视图", caseId);
        invoke(callback, JSON.stringify({ ok: true, updated: true }));
      } catch (error) {
        invoke(callback, JSON.stringify({ ok: false, error: text((error as Error)?.message) || "视图重命名失败", updated: false }));
      }
    },
    async reorderFlowViews(payloadJson, callback) {
      const payload = parseJson<Record<string, unknown>>(payloadJson, {});
      const caseId = text(payload.caseId) || getCurrentCaseId();
      if (!caseId) {
        invoke(callback, JSON.stringify({ ok: false, error: "参数错误" }));
        return;
      }
      try {
        const data = await service.reorderFlowViews({ case_id: caseId, order: uniqueStrings(payload.order) });
        invoke(callback, JSON.stringify({ ok: true, count: clampInt(data.count, 0, 0, Number.MAX_SAFE_INTEGER) }));
      } catch (error) {
        invoke(callback, JSON.stringify({ ok: false, error: text((error as Error)?.message) || "视图排序失败" }));
      }
    },
  };
}
