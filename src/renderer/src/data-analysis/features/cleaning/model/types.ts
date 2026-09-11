import type { ImportFileLogDTO } from "../../../services/import/api";

export type ExportStateStatus = "idle" | "running" | "done" | "failed" | "canceled";
export type CleaningBoardStatus = "idle" | "running" | "done" | "failed";
export type ExportKind = "" | "cleaned" | "raw";
export type CleaningRunMode = "" | "run" | "reclean";
export type CleaningFileStage = "pending" | "running" | "done" | "failed";

export interface CleaningLiveEvent {
  sequence: number;
  event: string;
  job_id: string;
  level: string;
  message: string;
  step: number | null;
  progress: number | null;
  timestamp: string;
}

export interface ExportState {
  jobId: string;
  kind: ExportKind;
  status: ExportStateStatus;
  progress: number | null;
  outputPath: string;
  error: string;
  message: string;
}

export interface StepCatalogItem {
  step: number;
  title: string;
  kind: string;
  description: string;
}

export interface CleaningAvailability {
  hasTransactionData: boolean | null;
  scopeFiles: ImportFileLogDTO[];
  scopeRows: number | null;
}
