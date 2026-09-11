import type {
  StatsTxnRowDTO,
  StatsV2ChartDetailRowsDTO,
  StatsV2ChartDetailRowsRequest,
} from "../api/stats-api";

export * from "../../../services/analysis/stats-chart-detail-resource";

export type StatsChartDetailRowsQueryPayload = StatsV2ChartDetailRowsRequest;
export type StatsChartDetailRowsLoader = (payload: StatsChartDetailRowsQueryPayload) => Promise<StatsV2ChartDetailRowsDTO>;

function requireNonNegativeInteger(value: unknown): number {
  if (typeof value !== "number" || !Number.isSafeInteger(value) || value < 0) {
    throw new Error("联动明细总数缺少可验证的完整性信息，请稍后重试。");
  }
  return value;
}

export async function loadAllStatsChartDetailRows(
  payload: StatsChartDetailRowsQueryPayload,
  loader: StatsChartDetailRowsLoader
): Promise<{ rows: StatsTxnRowDTO[]; total: number }> {
  const rows: StatsTxnRowDTO[] = [];
  let page = 1;
  let total: number | null = null;
  if (!Number.isSafeInteger(payload.limit) || payload.limit <= 0) {
    throw new Error("联动明细分页参数无效，请稍后重试。");
  }

  while (true) {
    const detail = await loader({ ...payload, page });
    if (
      detail.fact_answer_allowed !== true ||
      (detail.evidence_status !== "verified" && detail.evidence_status !== "verified_no_hit") ||
      !Array.isArray(detail.rows)
    ) {
      throw new Error("联动明细尚未通过事实发布校验，请稍后重试。");
    }
    const pageRows = detail.rows;
    const nextTotal = requireNonNegativeInteger(detail.total);
    if (total !== null && total !== nextTotal) {
      throw new Error("联动明细总数在分页期间发生变化，请重新查询。");
    }
    total = nextTotal;
    rows.push(...pageRows);

    if (rows.length > total) {
      throw new Error(`联动明细返回 ${rows.length} 行，超过已验证总数 ${total} 行。`);
    }
    if (rows.length === total) {
      break;
    }
    if (!pageRows.length) {
      throw new Error(`导出时仅拉取到 ${rows.length}/${total} 行联动明细，请稍后重试。`);
    }
    if (pageRows.length < payload.limit) {
      throw new Error(`导出时仅拉取到 ${rows.length}/${total} 行联动明细，请稍后重试。`);
    }
    page += 1;
  }

  if (total === null || rows.length !== total) {
    throw new Error(`导出时仅拉取到 ${rows.length}/${total} 行联动明细，请稍后重试。`);
  }

  return {
    rows,
    total,
  };
}
