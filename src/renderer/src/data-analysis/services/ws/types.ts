export interface WsEventEnvelope {
  version: string;
  event: string;
  type: "info" | "progress" | "success" | "error" | "log" | string;
  channel: string;
  job_id?: string | null;
  case_id?: string | null;
  sequence: number;
  payload: Record<string, unknown>;
  timestamp: string;
}

export type SafeWsEventName =
  | "import.job.queued"
  | "import.job.progress"
  | "import.job.completed"
  | "import.job.failed"
  | "cleaning.job.queued"
  | "cleaning.job.progress"
  | "cleaning.job.log"
  | "cleaning.job.completed"
  | "cleaning.job.failed"
  | "analysis.flow.build.progress"
  | "analysis.flow.build.completed"
  | "analysis.flow.graph.patch"
  | "analysis.flow.views.patch"
  | "analysis.flow.build.failed"
  | "export.job.queued"
  | "export.job.progress"
  | "export.job.completed"
  | "export.job.failed"
  | "analysis.stats.export.progress"
  | "analysis.stats.export.completed"
  | "analysis.stats.export.failed"
  | "analysis.stats.query.progress"
  | "analysis.stats.query.completed"
  | "analysis.stats.query.failed";

export type SafeWsEventType = "info" | "progress" | "success" | "warning" | "error" | "log";
export type SafeWsChannel = "analysis" | "cleaning" | "export" | "import" | "system";
export type SafeWsEventStatus = "queued" | "running" | "completed" | "failed" | "canceled";
export type SafeWsEventLevel = "info" | "progress" | "success" | "warning" | "error";
export type SafeWsExportTable =
  | "fc_coercive_measure"
  | "fc_person_contact"
  | "fc_person_address"
  | "fc_person"
  | "fc_sub_account"
  | "fc_account"
  | "fc_transaction"
  | "fc_task_fail"
  | "fc_task_success";

export interface SafeWsEventCounters {
  readonly table?: SafeWsExportTable;
  readonly rows_total?: number;
  readonly rows_exported?: number;
  readonly requested_steps?: number;
  readonly cleaned_rows?: number;
  readonly invalid?: number;
  readonly failed?: number;
  readonly reversal?: number;
  readonly step?: number;
  readonly job_count?: number;
  readonly processed_count?: number;
  readonly record_count?: number;
  readonly row_count?: number;
  readonly skipped_count?: number;
}

export interface SafeWsEventPayload extends Readonly<Record<string, unknown>> {
  readonly event_id: string;
  readonly status: SafeWsEventStatus;
  readonly stage: SafeWsEventStatus;
  readonly level: SafeWsEventLevel;
  readonly progress?: number;
  readonly step?: number;
  readonly code?: "JOB_CANCELED";
  readonly counters?: SafeWsEventCounters;
}

export interface SafeWsEventEnvelope {
  readonly version: "v1";
  readonly event: SafeWsEventName;
  readonly type: SafeWsEventType;
  readonly channel: SafeWsChannel;
  readonly job_id: string | null;
  readonly case_id: string;
  readonly sequence: number;
  readonly payload: SafeWsEventPayload;
  readonly timestamp: string;
}

export type WsConnectionStatus = "idle" | "connecting" | "open" | "closed" | "error";
