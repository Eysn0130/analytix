import {
  readFlowDirectBuildFacts,
} from "../../../../services/analysis/flow-direct-build-policy";

function text(value: unknown): string {
  return String(value == null ? "" : value).trim();
}

export function readCanonicalFlowCaseId(payload: Record<string, unknown>): string {
  const snakeCaseId = text(payload.case_id);
  const camelCaseId = text(payload.caseId);
  if (snakeCaseId && camelCaseId && snakeCaseId !== camelCaseId) {
    throw new Error("flow_case_binding_conflict");
  }
  return snakeCaseId || camelCaseId;
}

export function canonicalizeFlowCasePayload(payload: Record<string, unknown>): {
  caseId: string;
  payload: Record<string, unknown>;
} {
  const caseId = readCanonicalFlowCaseId(payload);
  if (!caseId) {
    throw new Error("flow_case_binding_missing");
  }
  const canonicalPayload: Record<string, unknown> = {
    ...payload,
    case_id: caseId,
  };
  delete canonicalPayload.caseId;
  return { caseId, payload: canonicalPayload };
}

export function shouldUseDirectBuildPath(payload: Record<string, unknown>): boolean {
  return readFlowDirectBuildFacts(payload) !== null;
}
