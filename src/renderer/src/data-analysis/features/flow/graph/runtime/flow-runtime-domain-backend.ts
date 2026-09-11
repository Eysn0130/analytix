import type { EmbeddedFlowBridgeService } from "../api/flow-bridge-service";
import {
  asNullableObject,
  clampInt,
  type EmbeddedFlowCallback,
  invoke,
  parseJson,
  text,
  uniqueStrings,
} from "./flow-runtime-backend-utils";

export interface FlowRuntimeDomainBackendMethods {
  getFundsStatus: (callback?: EmbeddedFlowCallback) => Promise<void>;
  getStatsTree: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  getAccountTxnRows: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
  getStatsTxnRows: (payloadJson?: string, callback?: EmbeddedFlowCallback) => Promise<void>;
}

interface CreateFlowRuntimeDomainBackendMethodsOptions {
  service: EmbeddedFlowBridgeService;
  getCurrentCaseId: () => string;
}

export function createFlowRuntimeDomainBackendMethods(
  options: CreateFlowRuntimeDomainBackendMethodsOptions
): FlowRuntimeDomainBackendMethods {
  const { service, getCurrentCaseId } = options;

  return {
    async getFundsStatus(callback) {
      const currentCaseId = getCurrentCaseId();
      if (!currentCaseId) {
        invoke(callback, "");
        return;
      }
      try {
        const data = await service.getFundsMeta(currentCaseId);
        invoke(callback, text(data.funds_status));
      } catch {
        invoke(callback, "");
      }
    },
    async getStatsTree(payloadJson, callback) {
      const payload = parseJson<Record<string, unknown>>(payloadJson, {});
      const caseId = text(payload.caseId) || getCurrentCaseId();
      if (!caseId) {
        invoke(callback, JSON.stringify({ ok: false, error: "no-case", groups: [] }));
        return;
      }
      try {
        const data = await service.getStatsTree({ case_id: caseId, tab: text(payload.tab) === "byCard" ? "byCard" : "byName" });
        const sourceUnavailable = data.semantic_status === "source_unavailable";
        invoke(callback, JSON.stringify({
          ok: false,
          semanticStatus: sourceUnavailable ? "source_unavailable" : "blocked",
          factAnswerAllowed: false,
          blocker: sourceUnavailable ? "materialization_unavailable" : "host_evidence_receipt_required",
          error: sourceUnavailable ? "materialization_unavailable" : "host_evidence_receipt_required",
          groups: []
        }));
      } catch (error) {
        invoke(callback, JSON.stringify({ ok: false, error: text((error as Error)?.message) || "tree-load-failed", groups: [] }));
      }
    },
    async getAccountTxnRows(payloadJson, callback) {
      const payload = parseJson<Record<string, unknown>>(payloadJson, {});
      const caseId = text(payload.caseId) || getCurrentCaseId();
      const accountKey = text(payload.accountKey || payload.account_key || payload.acctKey || payload.acct_key);
      if (!caseId || !accountKey) {
        invoke(callback, JSON.stringify({
          ok: false,
          semanticStatus: "needs_input",
          factAnswerAllowed: false,
          error: !caseId ? "no-case" : "account-key-required",
          rows: []
        }));
        return;
      }
      try {
        const data = await service.getAccountTxnRows({
          case_id: caseId,
          account_key: accountKey,
          date_start: text(payload.dateStart || payload.date_start),
          date_end: text(payload.dateEnd || payload.date_end),
          start_time: text(payload.startTime || payload.start_time),
          end_time: text(payload.endTime || payload.end_time),
          sort_dir: text(payload.sortDir || payload.sort_dir || "asc").toLowerCase() === "desc" ? "desc" : "asc",
          limit: clampInt(payload.limit, 0, 0, 5000),
          cursor: asNullableObject(payload.cursor || payload.nextCursor || payload.next_cursor),
        });
        const sourceUnavailable = data.semantic_status === "source_unavailable";
        const result: Record<string, unknown> = {
          ok: false,
          semanticStatus: sourceUnavailable ? "source_unavailable" : "blocked",
          factAnswerAllowed: false,
          blocker: sourceUnavailable ? "materialization_unavailable" : "host_evidence_receipt_required",
          rows: []
        };
        result.error = sourceUnavailable ? "materialization_unavailable" : "host_evidence_receipt_required";
        invoke(callback, JSON.stringify(result));
      } catch (error) {
        invoke(callback, JSON.stringify({ ok: false, error: text((error as Error)?.message) || "txn-rows-load-failed", rows: [] }));
      }
    },
    async getStatsTxnRows(payloadJson, callback) {
      const payload = parseJson<Record<string, unknown>>(payloadJson, {});
      const caseId = text(payload.caseId || payload.case_id) || getCurrentCaseId();
      if (!caseId) {
        invoke(callback, JSON.stringify({ ok: false, semanticStatus: "needs_input", factAnswerAllowed: false, error: "no-case", rows: [] }));
        return;
      }
      const keyType = text(payload.keyType || payload.key_type).toLowerCase() === "name" ? "name" : "account";
      const filterRaw = text(payload.filter || payload.direction).toLowerCase();
      const filter = filterRaw === "in" || filterRaw === "out" ? filterRaw : "all";
      const sortCol = text(payload.sortCol || payload.sort_col).toLowerCase() === "amount" ? "amount" : "txn_time";
      try {
        await service.getStatsTxnRows({
          case_id: caseId,
          selected: uniqueStrings(payload.selected || payload.selectedKeys || payload.selected_keys),
          date_start: text(payload.dateStart || payload.date_start),
          date_end: text(payload.dateEnd || payload.date_end),
          key_type: keyType,
          key_value: text(payload.keyValue || payload.key_value),
          key_values: uniqueStrings(payload.keyValues || payload.key_values),
          filter,
          sort_col: sortCol,
          sort_dir: text(payload.sortDir || payload.sort_dir || "asc").toLowerCase() === "desc" ? "desc" : "asc",
          limit: clampInt(payload.limit, 0, 0, 5000),
          cursor: asNullableObject(payload.cursor || payload.nextCursor || payload.next_cursor),
        });
        const result: Record<string, unknown> = {
          ok: false,
          semanticStatus: "blocked",
          factAnswerAllowed: false,
          blocker: "host_evidence_receipt_required",
          rows: []
        };
        result.error = "host_evidence_receipt_required";
        invoke(callback, JSON.stringify(result));
      } catch (error) {
        invoke(callback, JSON.stringify({ ok: false, error: text((error as Error)?.message) || "txn-rows-load-failed", rows: [] }));
      }
    },
  };
}
