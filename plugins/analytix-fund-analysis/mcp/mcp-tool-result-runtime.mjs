import { objectOf, text } from "./runtime-normalizers.mjs";
import {
  isFullCaseAnalysisPayload,
  PUBLICATION_RECEIPT_REQUIRED_TEXT,
  reportPublicationBlockedPayload,
  requiresPublicationBlock,
} from "./report-publication-guard.mjs";
import { assertDuckdbDiagnosticV1 } from "./duckdb-diagnostic.mjs";

export const MCP_TOOL_RESULT_RUNTIME_VERSION = "0.16.16";

const HOST_EVIDENCE_RECEIPT_REQUIRED_TEXT =
  "案件事实未发布：当前工具结果尚未绑定宿主权威 registry 中同案、同轮、同数据快照的有效 EvidenceReceipt。仅可说明能力边界、已检查范围、数据缺口和补证建议。";

function duckdbBoundaryText(diagnostic) {
  if (!diagnostic) return "";
  const code = text(diagnostic.code);
  let detail =
    "当前查询未产生可接受的案件事实；本轮未接受任何金额、笔数、账户、主体、时间范围或关系结论。";
  if (
    [
      "runner_unavailable",
      "runner_spawn_failed",
      "database_unavailable",
    ].includes(code)
  ) {
    detail =
      "当前案件数据源不可用；本轮未取得可验证的交易、账户、金额或覆盖范围。";
  } else if (code === "runner_timeout") {
    detail =
      "当前案件数据读取未在宿主时限内完成；超时不等于零、不存在、无关联或无异常。";
  } else if (code === "runner_cancelled") {
    detail = "当前案件数据读取已取消；取消前的任何部分结果均未被接受。";
  } else if (code === "database_locked") {
    detail = "当前案件数据源正处于受控更新状态；本轮没有接受旧结果或部分结果。";
  } else if (code === "dataset_snapshot_mismatch") {
    detail = "当前案件数据快照在核验期间发生变化；旧快照结果已被拒绝。";
  }
  return `${HOST_EVIDENCE_RECEIPT_REQUIRED_TEXT} ${detail} 已检查范围：当前案件绑定与数据源就绪状态。未检查范围：交易明细及其完整性。补证建议：恢复并冻结当前案件数据快照后，重新执行同案、同轮、同快照核验。`;
}

export function createMcpToolResultRuntime({
  serverName,
  serverVersion,
  compactToolText,
  compactStructuredContent,
  env = {},
}) {
  function toolResult(payload, args = {}) {
    const fullCaseAnalysis = isFullCaseAnalysisPayload(payload);
    const publicationBlocked = requiresPublicationBlock(payload, args);
    const publicPayload = publicationBlocked
      ? reportPublicationBlockedPayload(payload)
      : payload;
    void fullCaseAnalysis;
    void serverName;
    void serverVersion;
    void compactToolText;
    void compactStructuredContent;
    const source = objectOf(publicPayload);
    let duckdbDiagnostic = null;
    try {
      duckdbDiagnostic = assertDuckdbDiagnosticV1(source.duckdb_diagnostic);
    } catch {
      duckdbDiagnostic = null;
    }
    const diagnosticSemanticStatus = duckdbDiagnostic
      ? duckdbDiagnostic.code === "runner_timeout"
        ? "timeout"
        : duckdbDiagnostic.code === "runner_cancelled"
          ? "cancelled"
          : [
                "runner_unavailable",
                "runner_spawn_failed",
                "database_unavailable",
              ].includes(duckdbDiagnostic.code)
            ? "unavailable"
            : "blocked"
      : "";
    const reportedSemanticStatus =
      diagnosticSemanticStatus ||
      text(source.semantic_status || source.semanticStatus || source.status) ||
      "unknown";
    const blocker = publicationBlocked
      ? PUBLICATION_RECEIPT_REQUIRED_TEXT
      : duckdbBoundaryText(duckdbDiagnostic) ||
        HOST_EVIDENCE_RECEIPT_REQUIRED_TEXT;
    const outcome = {
      transportStatus: "success",
      semanticStatus: "blocked",
      reportedSemanticStatus,
      safeToAnswer: false,
      isError: true,
      blocker,
      partialCoverage: {},
      data: {},
      candidateEvidenceReceipts: [],
    };
    const result = {
      content: [
        {
          type: "text",
          text: blocker,
        },
      ],
      structuredContent: outcome,
      isError: true,
      _meta: {
        analytix_tool_outcome: outcome,
        analytix_evidence_ledger: {
          status: "unsupported",
          host_registry_verified: false,
          receipt_count: 0,
        },
      },
    };
    return result;
  }

  return { toolResult };
}
