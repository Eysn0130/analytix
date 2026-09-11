import type { EmbeddedFlowBridgeService } from "../api/flow-bridge-service";
import { createFlowRuntimeDomainBackendMethods } from "./flow-runtime-domain-backend";
import { createFlowRuntimeGraphBackendMethods } from "./flow-runtime-graph-backend";
import { createFlowRuntimeOpsBackendMethods } from "./flow-runtime-ops-backend";
import { createFlowRuntimeViewBackendMethods } from "./flow-runtime-view-backend";
import {
  type EmbeddedFlowCallback,
  invoke,
  text,
} from "./flow-runtime-backend-utils";

export interface FlowSignal<T> {
  connect: (listener: (value: T) => void) => void;
  disconnect: (listener: (value: T) => void) => void;
  emit: (value: T) => void;
}

export interface EmbeddedFlowRuntimeBackend {
  caseChanged: FlowSignal<string>;
  getCaseId: (callback?: EmbeddedFlowCallback) => void;
  getCaseName: (callback?: EmbeddedFlowCallback) => Promise<void>;
  getFlowLogConfig: (callback?: EmbeddedFlowCallback) => void;
  logFlowDebug: (payloadJson?: string, callback?: EmbeddedFlowCallback) => void;
  getFundsStatus: (callback?: EmbeddedFlowCallback) => Promise<void>;
  getStatsTree: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  getFlowGraph: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  expandFlowResultSnapshot: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  syncFlowResultSnapshotProjectionLayout: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  computeLayoutNodePlan: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  projectNetworkSectorPlacement: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  projectLayoutRoleGraph: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  projectAnalysisGraphData: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  mergeSameNameGraph: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  getAccountTxnRows: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  getStatsTxnRows: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  listFlowViews: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  getFlowResultSnapshot: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  saveFlowView: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  deleteFlowView: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  renameFlowView: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  reorderFlowViews: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  publishFlowGraphPatch: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  getFlowPerfSnapshot: () => Record<string, unknown>;
  exportFlowGraph: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
}

interface CreateEmbeddedFlowRuntimeBackendOptions {
  service: EmbeddedFlowBridgeService;
  targetWindow: Window;
  caseChanged: FlowSignal<string>;
  getCurrentCaseId: () => string;
}

export function createEmbeddedFlowRuntimeBackend(options: CreateEmbeddedFlowRuntimeBackendOptions): EmbeddedFlowRuntimeBackend {
  const { service, targetWindow, caseChanged, getCurrentCaseId } = options;
  const domainBackend = createFlowRuntimeDomainBackendMethods({ service, getCurrentCaseId });
  const graphBackend = createFlowRuntimeGraphBackendMethods({ service, targetWindow, getCurrentCaseId });
  const opsBackend = createFlowRuntimeOpsBackendMethods({ service, targetWindow });
  const viewBackend = createFlowRuntimeViewBackendMethods({ service, targetWindow, getCurrentCaseId });

  return {
    caseChanged,
    ...domainBackend,
    ...graphBackend,
    ...opsBackend,
    ...viewBackend,
    getCaseId(callback) {
      invoke(callback, getCurrentCaseId());
    },
    async getCaseName(callback) {
      const currentCaseId = getCurrentCaseId();
      if (!currentCaseId) {
        invoke(callback, "");
        return;
      }
      try {
        const data = await service.getCaseDetail(currentCaseId);
        invoke(callback, text(data.case_name));
      } catch {
        invoke(callback, "");
      }
    },
  };
}
