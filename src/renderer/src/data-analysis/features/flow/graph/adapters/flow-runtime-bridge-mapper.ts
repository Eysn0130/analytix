import type {
  FlowGraphData,
  FlowSnapshotRefDTO,
  FlowViewDTO,
} from "../../../../services/analysis/flow-api-contracts";

export interface FlowBridgeMapper {
  buildFlowRuntimeGraphResponse: (requestPayload: Record<string, unknown>, flowData: FlowGraphData) => Record<string, unknown>;
  mapBackendViewToRuntimeView: (item: FlowViewDTO) => Record<string, unknown>;
  sanitizeFlowRuntimeGraphPayload: (graph: unknown) => { nodes: Array<Record<string, unknown>>; edges: Array<Record<string, unknown>> };
  sanitizeSnapshotRef: (value: unknown) => FlowSnapshotRefDTO | null;
}

type FlowBridgeMapperHost = Window & {
  __ANALYTIX_FLOW_BRIDGE_MAPPER__?: Partial<FlowBridgeMapper>;
};

const REQUIRED_FLOW_BRIDGE_MAPPER_METHODS: Array<keyof FlowBridgeMapper> = [
  "buildFlowRuntimeGraphResponse",
  "mapBackendViewToRuntimeView",
  "sanitizeFlowRuntimeGraphPayload",
  "sanitizeSnapshotRef",
];

export function getRequiredFlowBridgeMapper(targetWindow: Window): FlowBridgeMapper {
  const mapper = (targetWindow as FlowBridgeMapperHost).__ANALYTIX_FLOW_BRIDGE_MAPPER__;
  if (!mapper || typeof mapper !== "object") {
    throw new Error("flow bridge mapper unavailable");
  }
  const missingMethod = REQUIRED_FLOW_BRIDGE_MAPPER_METHODS.find((method) => typeof mapper[method] !== "function");
  if (missingMethod) {
    throw new Error(`flow bridge mapper missing method: ${missingMethod}`);
  }
  return mapper as FlowBridgeMapper;
}

export function buildFlowRuntimeGraphResponse(
  targetWindow: Window,
  requestPayload: Record<string, unknown>,
  flowData: FlowGraphData
): Record<string, unknown> {
  return getRequiredFlowBridgeMapper(targetWindow).buildFlowRuntimeGraphResponse(requestPayload, flowData);
}

export function mapBackendViewToRuntimeView(targetWindow: Window, item: FlowViewDTO): Record<string, unknown> {
  return getRequiredFlowBridgeMapper(targetWindow).mapBackendViewToRuntimeView(item);
}

export function sanitizeFlowRuntimeGraphPayload(
  targetWindow: Window,
  graph: unknown
): { nodes: Array<Record<string, unknown>>; edges: Array<Record<string, unknown>> } {
  return getRequiredFlowBridgeMapper(targetWindow).sanitizeFlowRuntimeGraphPayload(graph);
}

export function sanitizeFlowSnapshotRef(targetWindow: Window, value: unknown): FlowSnapshotRefDTO | null {
  return getRequiredFlowBridgeMapper(targetWindow).sanitizeSnapshotRef(value);
}
