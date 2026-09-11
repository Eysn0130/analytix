import { WorkbenchSegmentedControl } from "../../../../components/workbench-ui";
import type { ChartFilterToken, ChartPanelViewState } from "../../api/stats-api";
import { projectOrdinaryFieldValue } from "../../../shared/ordinary-pii-projection";
import {
  formatCaseFactPercentage,
  readFiniteCaseFactNumber,
} from "../../model/stats-case-fact-number-model";
import { ChartPanel } from "../charts/ChartPanel";
import { HistogramChart } from "../charts/basic/HistogramChart";
import { HorizontalBarList } from "../charts/basic/HorizontalBarList";
import { normalizeHistogramChartItems, type NumericChartItem } from "../charts/basic/chart-render-utils";

interface EmptyCopy {
  title: string;
  description: string;
  variant: "selection" | "filtered";
}

interface AnomalyPanelBaseProps {
  loading: boolean;
  emptyState: boolean;
  emptyCopy: EmptyCopy;
  onPanelViewChange: (panelId: string, patch: Partial<ChartPanelViewState>) => void;
  onSelect: (token: ChartFilterToken) => void;
  formatMoney: (value: unknown) => string;
  formatCount: (value: unknown) => string;
}

interface AnomalyAmountPanelProps extends AnomalyPanelBaseProps {
  viewMode: string;
  amountBuckets: Array<Record<string, unknown>>;
  largeTxns: Array<Record<string, unknown>>;
  activeToken: ChartFilterToken | null;
  thresholdLabel: string;
}

interface AnomalyTypePanelProps extends AnomalyPanelBaseProps {
  metricBasis: string;
  txnTypeItems: Array<{
    label: string;
    rawValue?: string;
    value: number;
    amount?: number;
    count?: number;
    payload?: Record<string, unknown>;
  }>;
  activeToken: ChartFilterToken | null;
}

interface AnomalyEnvironmentPanelProps extends AnomalyPanelBaseProps {
  resolvedEnvView: string;
  ipCoverageRate: number | null;
  ipItems: Array<{
    label: string;
    rawValue?: string;
    value: number;
    amount?: number;
    count?: number;
    payload?: Record<string, unknown>;
  }>;
  summaryKeywordItems: Array<{
    label: string;
    rawValue?: string;
    value: number;
    amount?: number;
    count?: number;
    payload?: Record<string, unknown>;
  }>;
  activeToken: ChartFilterToken | null;
}

export function AnomalyAmountPanel({
  loading,
  emptyState,
  emptyCopy,
  viewMode,
  amountBuckets,
  largeTxns,
  activeToken,
  thresholdLabel,
  onPanelViewChange,
  onSelect,
  formatMoney,
}: AnomalyAmountPanelProps): JSX.Element {
  const histogramItems = normalizeHistogramChartItems(amountBuckets);
  const largeTxnItems = largeTxns.slice(0, 10).flatMap((item): NumericChartItem[] => {
    const amount = readFiniteCaseFactNumber(item.amount);
    const transactionId = typeof item.txn_id === "string" ? item.txn_id.trim() : "";
    if (amount == null || !transactionId) {
      return [];
    }
    return [{
      label: projectOrdinaryFieldValue("counterparty", item.counterparty || "未知对手"),
      rawValue: transactionId,
      value: amount,
      amount,
      detail: typeof item.txn_time === "string" && item.txn_time.trim() ? item.txn_time.trim() : "--",
      secondaryValue: projectOrdinaryFieldValue("summary", item.summary),
    }];
  });
  const selectedItemsEmpty = viewMode === "top" ? !largeTxnItems.length : !histogramItems.length;
  return (
    <ChartPanel
      panelId="anomalyAmount"
      title="大额交易分布图"
      subtitle={`阈值 ${thresholdLabel}，用于定位区间与重点单笔`}
      loading={loading}
      empty={emptyState || selectedItemsEmpty}
      className="chart-panel--span-4 chart-panel--narrow chart-panel--support"
      headerMode="compact"
      actionMode="menu"
      emptyTitle={emptyCopy.title}
      emptyDescription={emptyCopy.description}
      emptyVariant={emptyCopy.variant}
      headerControls={
        <WorkbenchSegmentedControl
          ariaLabel="大额图视图"
          className="chart-panel-segment"
          optionClassName="chart-panel-segment__option"
          indicatorClassName="chart-panel-segment__indicator"
          value={viewMode}
          options={[
            { value: "bucket", label: "区间" },
            { value: "top", label: "Top大额" },
          ]}
          onChange={(value) => onPanelViewChange("anomalyAmount", { view: value })}
        />
      }
    >
      {viewMode === "top" ? (
        <HorizontalBarList
          items={largeTxnItems}
          panelId="anomalyAmount"
          dimension="txn_id"
          activeToken={activeToken}
          onSelect={onSelect}
          variant="compact"
          showRank
          labelColumn="对手"
          subtitleFormatter={(item) =>
            [`金额 ${formatMoney(item.amount ?? item.value)}`, item.detail ? `时间 ${item.detail}` : "", item.secondaryValue].filter(
              (part): part is string => Boolean(part)
            )
          }
        />
      ) : (
        <HistogramChart items={histogramItems} activeToken={activeToken} onSelect={onSelect} />
      )}
    </ChartPanel>
  );
}

export function AnomalyTypePanel({
  loading,
  emptyState,
  emptyCopy,
  metricBasis,
  txnTypeItems,
  activeToken,
  onPanelViewChange,
  onSelect,
  formatMoney,
  formatCount,
}: AnomalyTypePanelProps): JSX.Element {
  return (
    <ChartPanel
      panelId="anomalyType"
      title="交易类型分布图"
      subtitle="查看交易类型金额和笔数结构"
      loading={loading}
      empty={emptyState || !txnTypeItems.length}
      className="chart-panel--span-4 chart-panel--narrow chart-panel--support"
      headerMode="compact"
      actionMode="menu"
      emptyTitle={emptyCopy.title}
      emptyDescription={emptyCopy.description}
      emptyVariant={emptyCopy.variant}
      headerControls={
        <WorkbenchSegmentedControl
          ariaLabel="交易类型口径"
          className="chart-panel-segment"
          optionClassName="chart-panel-segment__option"
          indicatorClassName="chart-panel-segment__indicator"
          value={metricBasis}
          options={[
            { value: "amount", label: "金额" },
            { value: "count", label: "笔数" },
          ]}
          onChange={(value) => onPanelViewChange("anomalyType", { metric_basis: value })}
        />
      }
    >
      <HorizontalBarList
        items={txnTypeItems}
        panelId="anomalyType"
        dimension="txn_type"
        activeToken={activeToken}
        onSelect={onSelect}
        variant="compact"
        showRank
        labelColumn="类型"
        subtitleFormatter={(item) => [`金额 ${formatMoney(item.amount)}`, `次数 ${formatCount(item.count)}`]}
      />
    </ChartPanel>
  );
}

export function AnomalyEnvironmentPanel({
  loading,
  emptyState,
  emptyCopy,
  resolvedEnvView,
  ipCoverageRate,
  ipItems,
  summaryKeywordItems,
  activeToken,
  onPanelViewChange,
  onSelect,
  formatMoney,
  formatCount,
}: AnomalyEnvironmentPanelProps): JSX.Element {
  const isIpView = resolvedEnvView === "ip";
  return (
    <ChartPanel
      panelId="anomalyEnv"
      title={isIpView ? "IP 地址画像图" : "摘要说明分类图"}
      subtitle={isIpView ? `IP 覆盖率 ${formatCaseFactPercentage(ipCoverageRate)}，缺失高时自动弱化` : "辅助识别高频摘要类别"}
      loading={loading}
      empty={emptyState || (isIpView ? !ipItems.length : !summaryKeywordItems.length)}
      className="chart-panel--span-4 chart-panel--narrow chart-panel--support"
      headerMode="compact"
      actionMode="menu"
      emptyTitle={emptyCopy.title}
      emptyDescription={emptyCopy.description}
      emptyVariant={emptyCopy.variant}
      headerControls={
        <WorkbenchSegmentedControl
          ariaLabel="环境画像视图"
          className="chart-panel-segment"
          optionClassName="chart-panel-segment__option"
          indicatorClassName="chart-panel-segment__indicator"
          value={resolvedEnvView}
          options={[
            { value: "ip", label: "IP" },
            { value: "summary", label: "摘要" },
          ]}
          onChange={(value) => onPanelViewChange("anomalyEnv", { view: value })}
        />
      }
    >
      <HorizontalBarList
        items={isIpView ? ipItems : summaryKeywordItems}
        panelId="anomalyEnv"
        dimension={isIpView ? "ip_addr" : "summary_keyword"}
        activeToken={activeToken}
        onSelect={onSelect}
        variant="compact"
        showRank
        labelColumn={isIpView ? "IP" : "摘要"}
        subtitleFormatter={(item) =>
          isIpView
            ? [`金额 ${formatMoney(item.amount)}`, `次数 ${formatCount(item.count)}`]
            : [`次数 ${formatCount(item.count)}`]
        }
      />
    </ChartPanel>
  );
}
