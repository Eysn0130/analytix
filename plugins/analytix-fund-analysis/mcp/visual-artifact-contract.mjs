import {
  arrayOf,
  objectOf,
  text
} from "./runtime-normalizers.mjs";

export const VISUAL_ARTIFACT_CONTRACT_VERSION = "0.15.128-visual-artifact-contract";

const FORBIDDEN_VISIBLE_PATTERN = /\b(?:supported|needs[_ -]?review|needs[_ -]?evidence|candidate|edge_status|delivery_state|workflow|case_id|support layer|artifact_id|query_id|audit_ref|debug|JSON|MCP|DuckDB|SQL)\b/iu;
const GRAPH_ARTIFACT_TYPES = new Set(["fund_flow_graph", "mermaid"]);
const TABLE_ARTIFACT_TYPES = new Set(["evidence_table", "chart_table", "appendix_table", "workbook", "csv", "xlsx"]);
const FILE_ARTIFACT_TYPES = new Set(["png", "jpg", "jpeg", "workbook", "csv", "xlsx", "report_figure"]);

function addError(errors, code, path, message, repairAction) {
  errors.push({
    code,
    path,
    message,
    repair_action: repairAction
  });
}

function hasSourceAnchor(value) {
  const source = objectOf(value);
  const refs = objectOf(source.source_refs || source.evidence_refs);
  return Boolean(
    text(source.source_hash)
    || text(source.query_id)
    || text(source.evidence_id)
    || text(source.txn_id)
    || text(source.edge_id)
    || arrayOf(source.evidence_refs).length
    || arrayOf(source.source_refs).length
    || arrayOf(refs.query_ids).length
    || arrayOf(refs.evidence_ids).length
    || arrayOf(refs.path_ids).length
    || text(refs.artifact_id)
  );
}

function cleanVisibleText(value) {
  return !FORBIDDEN_VISIBLE_PATTERN.test(text(value));
}

function artifactType(source) {
  return text(source.artifact_type || source.type || source.surface).toLowerCase();
}

function validateVisibleLabels(source, errors) {
  const labelFields = [
    ["title", source.title],
    ["subtitle", source.subtitle],
    ["file_name", source.file_name],
    ["alt_text", source.alt_text],
    ["legend", source.legend],
    ["caption", source.caption],
    ["table_after_analysis", source.table_after_analysis]
  ];
  for (const [field, value] of labelFields) {
    if (text(value) && !cleanVisibleText(value)) {
      addError(
        errors,
        "VISIBLE_LABEL_INTERNAL_TERM",
        field,
        "用户可见标题、图例、文件名或表后意见包含内部状态词。",
        "改为已有流水支持、需复核、需补证、线索、资金断点等经侦表述。"
      );
    }
  }
  for (const [index, label] of arrayOf(source.labels).entries()) {
    if (!cleanVisibleText(label)) {
      addError(
        errors,
        "VISIBLE_LABEL_INTERNAL_TERM",
        `labels[${index}]`,
        "用户可见标签包含内部状态词。",
        "将运行态标签转换为经侦业务标签后再交付。"
      );
    }
  }
}

function validateScope(source, type, errors) {
  const scope = objectOf(source.scope || source.metric_scope);
  const required = ["unit", "time_window", "metric", "direction", "evidence_status"];
  if (TABLE_ARTIFACT_TYPES.has(type) || type === "chart" || type === "dashboard_card") {
    for (const field of required) {
      if (!text(scope[field])) {
        addError(
          errors,
          "VISUAL_SCOPE_FIELD_MISSING",
          `scope.${field}`,
          "表格或图表缺少统计范围、单位、时间窗口、指标、方向或证据状态。",
          "补齐读者可见的统计口径后再作为交付材料。"
        );
      }
    }
  }
}

function validateRows(source, type, errors) {
  const rows = arrayOf(source.rows);
  if (!rows.length || (!TABLE_ARTIFACT_TYPES.has(type) && type !== "chart" && type !== "dashboard_card")) return;
  for (const [index, row] of rows.entries()) {
    const item = objectOf(row);
    if (!Object.keys(item).length) {
      addError(errors, "VISUAL_ROW_EMPTY", `rows[${index}]`, "表格存在空行。", "删除空行或补齐可核验字段。");
      continue;
    }
    if (!hasSourceAnchor(item) && !hasSourceAnchor(source)) {
      addError(
        errors,
        "VISUAL_ROW_SOURCE_ANCHOR_MISSING",
        `rows[${index}]`,
        "表格行缺少查询、证据、交易或交付物来源锚点。",
        "为每行补充 evidence_refs/source_refs，或降级为非报告级说明。"
      );
    }
  }
  if (!text(source.table_after_analysis) && !text(source.after_analysis)) {
    addError(
      errors,
      "TABLE_AFTER_ANALYSIS_MISSING",
      "table_after_analysis",
      "报告级表格缺少表后研判意见。",
      "补充异常特征、证明价值、证据边界和下一步取证动作。"
    );
  }
}

function validateGraphEdges(source, type, errors) {
  if (!GRAPH_ARTIFACT_TYPES.has(type)) return;
  const edges = arrayOf(source.edges || source.drawable_edges || source.mermaid_edges);
  if (!edges.length) {
    addError(
      errors,
      "GRAPH_DRAWABLE_EDGE_MISSING",
      "edges",
      "资金流向图没有可绘制交易链路。",
      "没有支持边时只能输出资金断点和补证事项，不画确定箭头。"
    );
    return;
  }
  for (const [index, edge] of edges.entries()) {
    const item = objectOf(edge);
    const status = text(item.edge_status || item.support_status || "supported");
    if (status !== "supported") {
      addError(
        errors,
        "GRAPH_EDGE_NOT_SUPPORTED",
        `edges[${index}].edge_status`,
        "资金流向图包含未达到 supported 的候选或待核边。",
        "将该边移出可见箭头，放入资金断点或待补证事项。"
      );
    }
    for (const field of ["from_label", "to_label", "amount", "txn_time"]) {
      if (!text(item[field]) && item[field] !== 0) {
        addError(
          errors,
          "GRAPH_EDGE_CORE_FIELD_MISSING",
          `edges[${index}].${field}`,
          "可绘制资金边缺少端点、金额或交易时间。",
          "补齐交易级字段后再画入资金流向图。"
        );
      }
    }
    if (!hasSourceAnchor(item) && !hasSourceAnchor(source)) {
      addError(
        errors,
        "GRAPH_EDGE_SOURCE_ANCHOR_MISSING",
        `edges[${index}]`,
        "可绘制资金边缺少交易、证据或查询来源锚点。",
        "补充交易号、证据引用或查询来源后再作为确定箭头。"
      );
    }
  }
}

function validateFileInspection(source, type, errors) {
  if (!FILE_ARTIFACT_TYPES.has(type)) return;
  const inspection = objectOf(source.file_inspection || source.inspection);
  const status = text(source.inspection_status || inspection.inspection_status);
  if (status !== "passed") {
    addError(
      errors,
      "ARTIFACT_FILE_INSPECTION_NOT_PASSED",
      "inspection_status",
      "文件型交付物未通过读取、签名或非空检查。",
      "重新读取/渲染检查，失败时只交付合规证据表并说明阻断。"
    );
  }
  if ((type === "png" || type === "jpg" || type === "jpeg" || type === "report_figure") && source.nonblank !== true && inspection.nonblank !== true) {
    addError(
      errors,
      "IMAGE_NONBLANK_CHECK_MISSING",
      "nonblank",
      "图片或报告插图缺少非空检查。",
      "完成像素/渲染非空检查后再声称图片已交付。"
    );
  }
}

export function validateVisualArtifactContract(artifact = {}) {
  const source = objectOf(artifact);
  const type = artifactType(source);
  const errors = [];
  if (!type) {
    addError(errors, "ARTIFACT_TYPE_MISSING", "artifact_type", "缺少交付物类型。", "声明 evidence_table、chart、fund_flow_graph、png、xlsx 等类型。");
  }
  if (!text(source.title)) {
    addError(errors, "ARTIFACT_TITLE_MISSING", "title", "缺少用户可见标题。", "使用公安经侦业务标题说明交付物用途。");
  }
  validateVisibleLabels(source, errors);
  validateScope(source, type, errors);
  validateRows(source, type, errors);
  validateGraphEdges(source, type, errors);
  validateFileInspection(source, type, errors);
  const status = errors.length ? "failed" : "passed";
  return {
    ok: status === "passed",
    status,
    contract_version: VISUAL_ARTIFACT_CONTRACT_VERSION,
    errors,
    repair_actions: errors.map((error) => error.repair_action).filter(Boolean),
    audit_events: [
      {
        event: `visual_artifact_validation:${status}`,
        artifact_type: type || "unknown",
        error_count: errors.length
      }
    ]
  };
}
