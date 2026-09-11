import { httpClient } from "../http/client";
import type {
  FlowBuildJobDTO,
  FlowBuildJobReq,
  FlowGraphData,
  FlowGraphExpandReq,
  FlowGraphReq,
  FlowResultSnapshotPatchData,
  FlowViewCreateReq,
  FlowViewDTO,
  FlowViewListData,
  FlowViewUpdateReq,
} from "./flow-api-contracts";
import {
  projectFlowBuildJobBoundary,
  projectFlowPublicResultBoundary,
  projectFlowPublicViewBoundary,
  projectFlowPublicViewListBoundary,
  requireControlledFlowArtifact,
} from "./flow-public-boundary";

export type * from "./flow-api-contracts";

export async function buildFlowGraph(payload: FlowGraphReq): Promise<FlowGraphData> {
  const response = await httpClient.post<unknown>("/api/v1/analysis/flow/graph", payload);
  return projectFlowPublicResultBoundary(response.data);
}

export async function createFlowBuildJob(payload: FlowBuildJobReq): Promise<FlowBuildJobDTO> {
  const response = await httpClient.post<unknown>("/api/v1/analysis/flow/jobs", payload);
  return projectFlowBuildJobBoundary(response.data, payload.case_id);
}

export async function getFlowBuildJob(jobId: string, caseId: string): Promise<FlowBuildJobDTO> {
  const query = new URLSearchParams({ case_id: caseId });
  const response = await httpClient.get<unknown>(
    `/api/v1/analysis/flow/jobs/${encodeURIComponent(jobId)}?${query.toString()}`,
  );
  return projectFlowBuildJobBoundary(response.data, caseId);
}

export async function getFlowBuildResult(jobId: string, caseId: string): Promise<FlowGraphData> {
  const query = new URLSearchParams({ case_id: caseId });
  const response = await httpClient.get<unknown>(
    `/api/v1/analysis/flow/jobs/${encodeURIComponent(jobId)}/result?${query.toString()}`,
  );
  return projectFlowPublicResultBoundary(response.data);
}

export async function cancelFlowBuildJob(jobId: string, caseId: string): Promise<void> {
  const query = new URLSearchParams({ case_id: caseId });
  await httpClient.post<{ ok: boolean }>(
    `/api/v1/analysis/flow/jobs/${encodeURIComponent(jobId)}/cancel?${query.toString()}`,
  );
}

export async function createFlowView(payload: FlowViewCreateReq): Promise<FlowViewDTO> {
  const response = await httpClient.post<unknown>("/api/v1/analysis/flow/views", payload);
  return projectFlowPublicViewBoundary(response.data, payload.case_id);
}

export async function listFlowViews(params: {
  caseId: string;
  page?: number;
  pageSize?: number;
}): Promise<FlowViewListData> {
  const query = new URLSearchParams();
  query.set("case_id", params.caseId);
  query.set("page", String(params.page ?? 1));
  query.set("page_size", String(params.pageSize ?? 50));
  const response = await httpClient.get<unknown>(`/api/v1/analysis/flow/views?${query.toString()}`);
  return projectFlowPublicViewListBoundary(response.data, params.caseId);
}

export async function getFlowView(viewId: string, caseId: string): Promise<FlowViewDTO> {
  const query = new URLSearchParams({ case_id: caseId });
  const response = await httpClient.get<unknown>(
    `/api/v1/analysis/flow/views/${encodeURIComponent(viewId)}?${query.toString()}`,
  );
  return projectFlowPublicViewBoundary(response.data, caseId);
}

export async function updateFlowView(viewId: string, caseId: string, payload: FlowViewUpdateReq): Promise<FlowViewDTO> {
  const query = new URLSearchParams({ case_id: caseId });
  const response = await httpClient.patch<unknown>(
    `/api/v1/analysis/flow/views/${encodeURIComponent(viewId)}?${query.toString()}`,
    payload,
  );
  return projectFlowPublicViewBoundary(response.data, caseId);
}

export async function deleteFlowView(viewId: string, caseId: string): Promise<void> {
  const query = new URLSearchParams({ case_id: caseId });
  await httpClient.delete<{ ok: boolean }>(
    `/api/v1/analysis/flow/views/${encodeURIComponent(viewId)}?${query.toString()}`,
  );
}

export async function getFlowResultSnapshotPatch(params: {
  caseId: string;
  snapshotId: string;
  baseSnapshotId: string;
}): Promise<FlowResultSnapshotPatchData> {
  void params;
  return requireControlledFlowArtifact();
}

export async function getFlowResultSnapshot(params: { caseId: string; snapshotId: string }): Promise<FlowGraphData> {
  void params;
  return requireControlledFlowArtifact();
}

export async function expandFlowResultSnapshot(
  snapshotId: string,
  payload: FlowGraphExpandReq
): Promise<FlowResultSnapshotPatchData> {
  void snapshotId;
  void payload;
  return requireControlledFlowArtifact();
}
