import { MAX_REMARK_KEYWORD_CLOUD_ITEMS } from "./stats-keyword-cloud-model";
import { projectOrdinaryFieldValue } from "../../shared/ordinary-pii-projection";

export type ChartEmptyStateMode = "no-case" | "tree-loading" | "tree-unavailable" | "no-objects" | "search-empty" | "await-selection";
export type DashboardEmptyCopyKind = "primary" | "secondary" | "detail";
export type SummaryGlyphKind = "in" | "out" | "net" | "counterparty" | "txn";

export interface DashboardRankItem {
  label: string;
  value: number;
  rawValue?: string;
  amount?: number;
  count?: number;
}

export interface DashboardSummaryTile {
  label: string;
  value: string;
  tone: "in" | "out" | "net" | "neutral";
  glyph: SummaryGlyphKind;
  metric: "money" | "count";
}

export interface DashboardHeroStat {
  tone: "in" | "out" | "net";
  label: string;
  value: string;
  note: string;
}

export interface DashboardHeroCopy {
  muted: boolean;
  title: string;
  subtitle: string;
  stats: DashboardHeroStat[];
}

export interface DashboardEmptyStateCopy {
  title: string;
  description: string;
  variant: "selection" | "filtered";
}

export const DISTRIBUTION_OPTIONS = [
  { value: "bank", label: "银行" },
  { value: "location", label: "地域" },
];

export function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : {};
}

export function asList<T = Record<string, unknown>>(value: unknown): T[] {
  return Array.isArray(value) ? (value as T[]) : [];
}

function toFiniteNumber(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) ? value : null;
}

function toNonNegativeInteger(value: unknown): number | null {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0 ? value : null;
}

function toLabel(value: unknown): string | null {
  if (typeof value !== "string") {
    return null;
  }
  const label = value.trim();
  return label ? label : null;
}

export function toNum(value: unknown): number {
  return toFiniteNumber(value) ?? Number.NaN;
}

export function fmtMoney(value: unknown): string {
  const amount = toFiniteNumber(value);
  if (amount == null) return "--";
  return `¥${amount.toLocaleString("zh-CN", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`;
}

export function fmtSignedMoney(value: unknown): string {
  const amount = toFiniteNumber(value);
  if (amount == null) return "--";
  if (!amount) {
    return fmtMoney(0);
  }
  return `${amount > 0 ? "+" : "-"}${fmtMoney(Math.abs(amount))}`;
}

export function fmtCount(value: unknown): string {
  const count = toNonNegativeInteger(value);
  return count == null ? "--" : count.toLocaleString("zh-CN");
}

export function formatRangeLabel(start: string, end: string): string {
  const startText = String(start || "").trim();
  const endText = String(end || "").trim();
  if (!startText && !endText) {
    return "未限定";
  }
  if (startText && endText) {
    return `${startText} 至 ${endText}`;
  }
  return startText || endText;
}

interface NumericAliasResult {
  present: boolean;
  value: number | null;
}

function readNumericAlias(item: Record<string, unknown>, keys: string[], count = false): NumericAliasResult {
  for (const key of keys) {
    if (Object.prototype.hasOwnProperty.call(item, key)) {
      return {
        present: true,
        value: count ? toNonNegativeInteger(item[key]) : toFiniteNumber(item[key]),
      };
    }
  }
  return { present: false, value: null };
}

function readNumericPair(
  item: Record<string, unknown>,
  leftKey: string,
  rightKey: string,
  operation: "add" | "subtract",
  count = false
): number | null {
  const left = readNumericAlias(item, [leftKey], count);
  const right = readNumericAlias(item, [rightKey], count);
  if (!left.present || !right.present || left.value == null || right.value == null) {
    return null;
  }
  return operation === "add" ? left.value + right.value : left.value - right.value;
}

export function getMetricItemValue(item: Record<string, unknown>, metricBasis: string): number | null {
  if (metricBasis === "count") {
    const direct = readNumericAlias(item, ["total_count", "count"], true);
    return direct.present ? direct.value : readNumericPair(item, "in_count", "out_count", "add", true);
  }
  if (metricBasis === "net") {
    const direct = readNumericAlias(item, ["net_amount", "net", "value"]);
    return direct.present ? direct.value : readNumericPair(item, "in_amount", "out_amount", "subtract");
  }
  const direct = readNumericAlias(item, ["total_amount", "amount", "value"]);
  return direct.present ? direct.value : readNumericPair(item, "in_amount", "out_amount", "add");
}

export function sortItemsByMetric(items: Array<Record<string, unknown>>, metricBasis: string): Array<Record<string, unknown>> {
  return items.filter((item) => getMetricItemValue(item, metricBasis) != null).sort((left, right) => {
    const leftValue = getMetricItemValue(left, metricBasis);
    const rightValue = getMetricItemValue(right, metricBasis);
    if (leftValue == null || rightValue == null) {
      return 0;
    }
    const diff = Math.abs(rightValue) - Math.abs(leftValue);
    if (diff !== 0) {
      return diff;
    }
    return String(left.label || "").localeCompare(String(right.label || ""), "zh-CN");
  });
}

export function buildSummaryTiles(summary: Record<string, unknown>, hasSelection: boolean): DashboardSummaryTile[] {
  return [
    { label: "总流入金额", value: hasSelection ? fmtMoney(summary.total_in_amount) : "--", tone: "in", glyph: "in", metric: "money" },
    { label: "总流出金额", value: hasSelection ? fmtMoney(summary.total_out_amount) : "--", tone: "out", glyph: "out", metric: "money" },
    { label: "净流入金额", value: hasSelection ? fmtSignedMoney(summary.net_in_amount) : "--", tone: "net", glyph: "net", metric: "money" },
    { label: "交易对手", value: hasSelection ? fmtCount(summary.active_counterparty_count) : "--", tone: "neutral", glyph: "counterparty", metric: "count" },
    { label: "交易笔数", value: hasSelection ? fmtCount(summary.txn_total_count) : "--", tone: "neutral", glyph: "txn", metric: "count" },
  ];
}

export function getDashboardEvidenceBoundaryCopy(
  evidenceStatus: unknown,
  blocker: unknown
): DashboardEmptyStateCopy | null {
  const status = String(evidenceStatus || "").trim();
  if (status === "verified") return null;
  if (status === "partial") {
    return {
      title: "数据覆盖不完整",
      description: "当前范围存在缺失、解析失败或方向覆盖不足；为避免把部分数据升级为全案结论，图表事实已被宿主阻断。",
      variant: "filtered",
    };
  }
  if (status === "source_unavailable") {
    return {
      title: "案件数据源不可用",
      description: "当前数据快照或物化结果不可验证，未生成金额、笔数或关系结论。请恢复数据源后重新检查。",
      variant: "filtered",
    };
  }
  if (status === "verified_no_hit") {
    return {
      title: "已查范围未发现匹配记录",
      description: "该结论仅适用于已验证的查询范围，不表示全案为零、不存在或无异常。",
      variant: "filtered",
    };
  }
  if (status === "needs_selection") return null;
  return {
    title: "案件事实发布已阻断",
    description: String(blocker || "").trim() === "host_evidence_receipt_required"
      ? "尚无同轮、同案件、同快照的宿主 EvidenceReceipt，不能展示金额、笔数或关系事实。"
      : "当前结果未通过宿主证据校验，未发布任何案件事实。",
    variant: "filtered",
  };
}

export function buildDashboardHeroCopy({
  activeFilterCount,
  emptyStateMode,
  hasActiveFilters,
  hasSelection,
  leftSearch,
  resolvedSelectedObjectCount,
  singleSelectionTitle,
  selectedAccounts,
}: {
  activeFilterCount: number;
  emptyStateMode: ChartEmptyStateMode;
  hasActiveFilters: boolean;
  hasSelection: boolean;
  leftSearch: string;
  resolvedSelectedObjectCount: number;
  singleSelectionTitle?: string;
  selectedAccounts: string[];
}): DashboardHeroCopy {
  if (hasSelection) {
    const singleSelection = selectedAccounts.length === 1;
    return {
      muted: false,
      title: singleSelection
        ? singleSelectionTitle || `账户 ${projectOrdinaryFieldValue("account_no", selectedAccounts[0])}`
        : "账户集合图表分析",
      subtitle: "",
      stats: [
        { tone: "in", label: "分析对象", value: fmtCount(resolvedSelectedObjectCount), note: "当前纳入分析的对象数" },
        { tone: "out", label: "账户数量", value: fmtCount(selectedAccounts.length), note: singleSelection ? "当前为单账户模式" : "当前为账户集合模式" },
        { tone: "net", label: "联动筛选", value: fmtCount(activeFilterCount), note: hasActiveFilters ? "图表联动筛选已生效" : "当前未叠加联动筛选" },
      ],
    };
  }

  if (emptyStateMode === "no-case") {
    return {
      muted: true,
      title: "请先选择案件",
      subtitle: "当前还没有活动案件。图表分析需要先在案件页打开一个案件，随后才能在左侧选择对象进入分析。",
      stats: [],
    };
  }
  if (emptyStateMode === "tree-loading") {
    return {
      muted: true,
      title: "正在准备分析对象",
      subtitle: "左侧对象列表仍在加载中。加载完成后，你可以直接选择户名、卡号或证件号进入图表分析。",
      stats: [],
    };
  }
  if (emptyStateMode === "tree-unavailable") {
    return {
      muted: true,
      title: "分析对象来源不可用",
      subtitle: "宿主尚未取得当前案件、当前轮次和当前数据快照的有效证据回执，无法确认对象范围。请恢复数据源或补充证据后重试。",
      stats: [],
    };
  }
  if (emptyStateMode === "no-objects") {
    return {
      muted: true,
      title: "当前案件暂无可分析对象",
      subtitle: "这通常意味着还没有完成导入或清洗。等左侧出现对象列表后，再进入图表分析会更顺畅。",
      stats: [],
    };
  }
  if (emptyStateMode === "search-empty") {
    const normalizedSearchText = String(leftSearch || "").trim();
    return {
      muted: true,
      title: "左侧没有匹配对象",
      subtitle: normalizedSearchText ? `当前搜索词为“${normalizedSearchText}”。请调整搜索条件，或清空搜索后重新选择对象。` : "请调整左侧搜索条件，或清空搜索后重新选择对象。",
      stats: [],
    };
  }
  return {
    muted: true,
    title: "请选择左侧账户或卡号",
    subtitle: "选中对象后，右侧会立即更新趋势、关键词、流向、异常和联动明细，形成完整的图表分析工作台。",
    stats: [],
  };
}

export function getEmptyStateCopy(
  hasSelection: boolean,
  hasFilters: boolean,
  kind: DashboardEmptyCopyKind,
  emptyStateMode: ChartEmptyStateMode,
  searchText = ""
): DashboardEmptyStateCopy {
  if (!hasSelection) {
    if (emptyStateMode === "no-case") {
      return {
        title: "请先选择案件",
        description: kind === "detail" ? "请先在案件页打开一个案件，再查看联动明细" : "请先在案件页打开一个案件，再开始图表分析",
        variant: "selection",
      };
    }
    if (emptyStateMode === "tree-loading") {
      return {
        title: "正在加载可分析对象",
        description: kind === "detail" ? "对象列表加载完成后，可在左侧选择账户或卡号查看联动明细" : "对象列表加载完成后，可在左侧选择账户或卡号查看图表结果",
        variant: "selection",
      };
    }
    if (emptyStateMode === "tree-unavailable") {
      return {
        title: "分析对象来源不可用",
        description: "当前没有可验证的同案证据回执，不能把来源不可用解释为没有对象",
        variant: "selection",
      };
    }
    if (emptyStateMode === "no-objects") {
      return {
        title: "当前案件暂无可分析对象",
        description: "请先导入并清洗资金数据，生成左侧可选对象",
        variant: "selection",
      };
    }
    if (emptyStateMode === "search-empty") {
      const normalizedSearchText = String(searchText || "").trim();
      return {
        title: "左侧没有匹配对象",
        description: normalizedSearchText ? `未找到与“${normalizedSearchText}”匹配的户名、卡号或证件号，请调整搜索条件` : "未找到匹配对象，请调整搜索条件",
        variant: "selection",
      };
    }
    if (kind === "detail") {
      return {
        title: "当前未选择分析对象",
        description: "请选择左侧户名或卡号后查看联动明细",
        variant: "selection",
      };
    }
    if (kind === "primary") {
      return {
        title: "请选择左侧账户或卡号",
        description: "选中后显示该对象在不同时间段的进账与出账资金分布",
        variant: "selection",
      };
    }
    return {
      title: "请选择左侧账户或卡号",
      description: "选中后显示相关分析结果",
      variant: "selection",
    };
  }
  if (kind === "detail") {
    return {
      title: "当前筛选条件下暂无明细",
      description: hasFilters ? "可尝试清空联动或调整筛选条件" : "可尝试调整时间范围或重新选择对象",
      variant: "filtered",
    };
  }
  if (kind === "primary") {
    return {
      title: "当前筛选条件下暂无趋势数据",
      description: hasFilters ? "可尝试清空联动或调整筛选条件" : "可尝试调整时间范围后重新分析",
      variant: "filtered",
    };
  }
  return {
    title: "当前筛选条件下暂无数据",
    description: hasFilters ? "可尝试清空联动或调整筛选条件" : "可尝试调整时间范围或切换分析对象",
    variant: "filtered",
  };
}

export function resolveSelectedObjectCount(objectSummary: Record<string, unknown>, selectedObjectCount: number, hasSelection: boolean): number {
  const reportedCount = toNonNegativeInteger(objectSummary.selected_object_count);
  const localCount = toNonNegativeInteger(selectedObjectCount) ?? 0;
  return Math.max(reportedCount ?? 0, localCount, hasSelection ? 1 : 0);
}

export function getDistributionSource(distribution: Record<string, unknown>, view: string): Array<Record<string, unknown>> {
  return asList<Record<string, unknown>>(asRecord(distribution.views)[view]);
}

export function buildDistributionItems(distributionSource: Array<Record<string, unknown>>): DashboardRankItem[] {
  const metricBasis = "amount";
  return sortItemsByMetric(distributionSource, metricBasis)
    .flatMap((item): DashboardRankItem[] => {
      const rawValue = toLabel(item.label);
      const value = getMetricItemValue(item, metricBasis);
      if (!rawValue || value == null) {
        return [];
      }
      const count = readNumericAlias(item, ["total_count", "count"], true).value;
      return [{
        label: projectOrdinaryFieldValue("", rawValue),
        rawValue,
        value,
        amount: value,
        ...(count == null ? {} : { count }),
      }];
    })
    .slice(0, 10);
}

export function buildTxnTypeItems(items: Array<Record<string, unknown>>, metricBasis: string): DashboardRankItem[] {
  return sortItemsByMetric(items, metricBasis)
    .flatMap((item): DashboardRankItem[] => {
      const rawValue = toLabel(item.label);
      const value = getMetricItemValue(item, metricBasis);
      if (!rawValue || value == null) {
        return [];
      }
      const amount = readNumericAlias(item, ["amount"]).value;
      const count = readNumericAlias(item, ["count"], true).value;
      return [{
        label: projectOrdinaryFieldValue("", rawValue),
        rawValue,
        value,
        ...(amount == null ? {} : { amount }),
        ...(count == null ? {} : { count }),
      }];
    })
    .slice(0, 10);
}

export function buildIpItems(items: Array<Record<string, unknown>>): DashboardRankItem[] {
  return sortItemsByMetric(items, "amount")
    .flatMap((item): DashboardRankItem[] => {
      const rawValue = toLabel(item.label);
      const amount = readNumericAlias(item, ["amount"]).value;
      if (!rawValue || amount == null) {
        return [];
      }
      const count = readNumericAlias(item, ["count"], true).value;
      return [{
        label: projectOrdinaryFieldValue("ip_addr", rawValue),
        rawValue,
        value: amount,
        amount,
        ...(count == null ? {} : { count }),
      }];
    })
    .slice(0, 10);
}

export function buildSummaryKeywordItems(items: Array<Record<string, unknown>>): DashboardRankItem[] {
  return items
    .flatMap((item): DashboardRankItem[] => {
      const rawValue = toLabel(item.label);
      const count = toNonNegativeInteger(item.count);
      if (!rawValue || count == null) {
        return [];
      }
      return [{
        label: projectOrdinaryFieldValue("summary_keyword", rawValue),
        rawValue,
        value: count,
        count,
      }];
    })
    .sort((left, right) => right.value - left.value || left.label.localeCompare(right.label, "zh-CN"))
    .slice(0, 10);
}

export function buildRemarkKeywordItems(items: Array<Record<string, unknown>>): Array<{ label: string; count: number }> {
  return items
    .flatMap((item): Array<{ label: string; count: number }> => {
      const rawLabel = toLabel(item.label);
      const count = toNonNegativeInteger(item.count);
      if (!rawLabel || count == null) {
        return [];
      }
      const label = projectOrdinaryFieldValue("remark_keyword", rawLabel);
      return label ? [{ label, count }] : [];
    })
    .sort((left, right) => right.count - left.count || left.label.localeCompare(right.label, "zh-CN"))
    .slice(0, MAX_REMARK_KEYWORD_CLOUD_ITEMS);
}

export function resolveEnvironmentView({
  currentView,
  ipCoverageRate,
  ipItemsLength,
  summaryKeywordItemsLength,
}: {
  currentView: string;
  ipCoverageRate: number;
  ipItemsLength: number;
  summaryKeywordItemsLength: number;
}): string {
  const preferred = ipCoverageRate >= 20 && ipItemsLength ? "ip" : "summary";
  const current = String(currentView || preferred);
  if (current === "ip" && !ipItemsLength && summaryKeywordItemsLength) {
    return "summary";
  }
  if (current === "summary" && !summaryKeywordItemsLength && ipItemsLength) {
    return "ip";
  }
  return current;
}

export function resolveBalanceMode(selectionMode: string, trend: Record<string, unknown>): "balance" | "cumulative-net" {
  if (selectionMode !== "single-card") {
    return "cumulative-net";
  }
  const backendMode = String(trend.balance_mode || "");
  if (backendMode === "balance" || backendMode === "cumulative-net") {
    return backendMode;
  }
  return toNum(trend.balance_available) ? "balance" : "cumulative-net";
}

export function getFlowPanelSubtitle(direction: "in" | "out"): string {
  return direction === "in"
    ? "按金额取前十位进账来源，悬停查看金额、次数与时间范围"
    : "按金额取前十位出账去向，悬停查看金额、次数与时间范围";
}
