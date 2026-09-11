import { Dispatch, SetStateAction, useEffect, useMemo, useRef, useState } from "react";
import { showToast, toToastErrorDetail } from "../../../../components/feedback/toast-copy";
import type { VirtualizedColumn } from "../../../../components/VirtualizedTable";
import { subscribeStatsCacheInvalidation } from "../../resources/cache-events";
import {
  WorkbenchButton,
  WorkbenchSegmentedControl,
} from "../../../../components/workbench-ui";
import {
  ChartFilterToken,
  ChartGranularity,
  ChartPanelViewState,
  StatsTxnRowDTO,
  StatsV2ChartDashboardDTO,
  queryStatsV2ChartDashboard,
  queryStatsV2ChartDetailRows,
} from "../../api/stats-api";
import { ChartAnalysisState, createDefaultChartAnalysisState } from "../../model/chart-analysis-types";
import { projectOrdinaryFieldValue } from "../../../shared/ordinary-pii-projection";
import {
  CHART_DETAIL_COLUMN_OPTIONS,
  getChartDetailColumnDefinitions,
  getChartDetailExportColumns,
  toggleChartDetailColumnIds,
} from "../../model/chart-detail-model";
import { controlledArtifactPublicationBlockReason } from "../../../../services/publication-quarantine";
import {
  DISTRIBUTION_OPTIONS,
  asList,
  asRecord,
  buildDashboardHeroCopy,
  buildDistributionItems,
  buildIpItems,
  buildRemarkKeywordItems,
  buildSummaryKeywordItems,
  buildSummaryTiles,
  buildTxnTypeItems,
  fmtCount,
  fmtMoney,
  formatRangeLabel,
  getDistributionSource,
  getEmptyStateCopy,
  getFlowPanelSubtitle,
  getDashboardEvidenceBoundaryCopy,
  resolveBalanceMode,
  resolveEnvironmentView,
  resolveSelectedObjectCount,
  type ChartEmptyStateMode,
} from "../../model/stats-dashboard-view-model";
import { readFiniteCaseFactNumber } from "../../model/stats-case-fact-number-model";
import { normalizeSankeyChartItems } from "../../model/stats-sankey-render-model";
import { normalizeTrendPoints } from "../../model/stats-trend-model";
import {
  buildActiveTokenByPanel,
  canResetChartTrendView,
  clearChartFilters,
  getPanelView,
  resetChartTrendView,
  setChartTrendGranularity,
  toggleChartFilter,
  updateChartPanelView,
} from "../../state/stats-chart-dashboard-state";
import { BalanceTrendChart } from "./basic/BalanceTrendChart";
import { HeatmapSvg } from "./basic/HeatmapSvg";
import { HorizontalBarList } from "./basic/HorizontalBarList";
import {
  normalizeBalanceTrendPoints,
  normalizeHeatmapChartCells,
} from "./basic/chart-render-utils";
import { ChartPanel } from "./ChartPanel";
import { ChartDetailPanel } from "./ChartDetailPanel";
import { SankeyChart } from "./flow/SankeyChart";
import { RemarkKeywordCloud } from "./keyword/RemarkKeywordCloud";
import { TrendWorkspace } from "./trend/TrendWorkspace";
import { AnomalyAmountPanel, AnomalyEnvironmentPanel, AnomalyTypePanel } from "../panels/AnomalyPanels";
import { ObjectHeroPanel } from "../panels/ObjectHeroPanel";
import { SummaryTilesPanel } from "../panels/SummaryTilesPanel";
import "../../styles/stats-chart-dashboard.css";

const DEFAULT_CHART_ANALYSIS_STATE = createDefaultChartAnalysisState();
const CHART_DASHBOARD_QUERY_DELAY_MS = 80;
const CHART_DETAIL_QUERY_DELAY_MS = 220;

interface ChartAnalysisDashboardProps {
  activeCaseId: string;
  selectedAccounts: string[];
  selectedObjectCount: number;
  dateStart: string;
  dateEnd: string;
  emptyStateMode: ChartEmptyStateMode;
  leftSearch: string;
  chartState: ChartAnalysisState;
  setChartState: Dispatch<SetStateAction<ChartAnalysisState>>;
}

export function ChartAnalysisDashboard({
  activeCaseId,
  selectedAccounts,
  selectedObjectCount,
  dateStart,
  dateEnd,
  emptyStateMode,
  leftSearch,
  chartState,
  setChartState,
}: ChartAnalysisDashboardProps): JSX.Element {
  const [dashboardData, setDashboardData] = useState<StatsV2ChartDashboardDTO | null>(null);
  const [detailRows, setDetailRows] = useState<StatsTxnRowDTO[]>([]);
  const [detailTotal, setDetailTotal] = useState<number | null>(null);
  const [detailBoundaryCopy, setDetailBoundaryCopy] = useState<ReturnType<typeof getDashboardEvidenceBoundaryCopy>>(null);
  const [loadingDashboard, setLoadingDashboard] = useState(false);
  const [loadingDetail, setLoadingDetail] = useState(false);
  const [dataRevision, setDataRevision] = useState(0);
  const dashboardQuerySeqRef = useRef(0);
  const detailQuerySeqRef = useRef(0);
  const dashboardDataRef = useRef<StatsV2ChartDashboardDTO | null>(null);
  const hasSelection = selectedAccounts.length > 0;
  const hasActiveFilters = chartState.chartFilters.length > 0;
  const dashboardFactsAllowed = Boolean(
    dashboardData?.evidence_status === "verified" &&
    dashboardData.fact_answer_allowed === true
  );

  useEffect(() => {
    dashboardDataRef.current = dashboardData;
  }, [dashboardData]);

  useEffect(() => {
    if (!activeCaseId) {
      return;
    }
    return subscribeStatsCacheInvalidation((event) => {
      if (event.caseId !== activeCaseId) {
        return;
      }
      setDataRevision((prev) => prev + 1);
    });
  }, [activeCaseId]);

  useEffect(() => {
    if (!activeCaseId || !hasSelection) {
      setDashboardData(null);
      setLoadingDashboard(false);
      return;
    }
    const token = dashboardQuerySeqRef.current + 1;
    dashboardQuerySeqRef.current = token;
    setLoadingDashboard(!dashboardDataRef.current);
    const timer = window.setTimeout(() => {
      void queryStatsV2ChartDashboard()
        .then((dashboard) => {
          if (dashboardQuerySeqRef.current !== token) {
            return;
          }
          setDashboardData(dashboard);
        })
        .catch((error) => {
          if (dashboardQuerySeqRef.current !== token) {
            return;
          }
          showToast({
            tone: "error",
            title: "图表分析加载失败",
            detail: toToastErrorDetail(error instanceof Error ? error.message : "请稍后重试。")
          });
          setDashboardData(null);
        })
        .finally(() => {
          if (dashboardQuerySeqRef.current === token) {
            setLoadingDashboard(false);
          }
        });
    }, CHART_DASHBOARD_QUERY_DELAY_MS);
    return () => {
      window.clearTimeout(timer);
    };
  }, [activeCaseId, dataRevision, hasSelection]);

  useEffect(() => {
    if (!dashboardFactsAllowed) {
      setDetailRows([]);
      setDetailTotal(null);
      setDetailBoundaryCopy(null);
      setLoadingDetail(false);
      return;
    }
    const token = detailQuerySeqRef.current + 1;
    detailQuerySeqRef.current = token;
    setLoadingDetail(true);
    setDetailBoundaryCopy(null);
    const timer = window.setTimeout(() => {
      void queryStatsV2ChartDetailRows()
        .then((detail) => {
          if (detailQuerySeqRef.current !== token) {
            return;
          }
          if (detail.evidence_status !== "verified" || detail.fact_answer_allowed !== true) {
            setDetailRows([]);
            setDetailTotal(null);
            setDetailBoundaryCopy(getDashboardEvidenceBoundaryCopy(detail.evidence_status, detail.blocker));
            return;
          }
          const rows = Array.isArray(detail.rows) ? detail.rows : [];
          const total = detail.total;
          if (typeof total !== "number" || !Number.isSafeInteger(total) || total < rows.length) {
            setDetailRows([]);
            setDetailTotal(null);
            setDetailBoundaryCopy(getDashboardEvidenceBoundaryCopy("blocked", "detail_result_invalid"));
            return;
          }
          setDetailRows(rows);
          setDetailTotal(total);
          setDetailBoundaryCopy(null);
        })
        .catch((error) => {
          if (detailQuerySeqRef.current !== token) {
            return;
          }
          showToast({
            tone: "error",
            title: "联动明细加载失败",
            detail: toToastErrorDetail(error instanceof Error ? error.message : "请稍后重试。")
          });
          setDetailRows([]);
          setDetailTotal(null);
          setDetailBoundaryCopy(getDashboardEvidenceBoundaryCopy("blocked", "detail_query_failed"));
        })
        .finally(() => {
          if (detailQuerySeqRef.current === token) {
            setLoadingDetail(false);
          }
        });
    }, CHART_DETAIL_QUERY_DELAY_MS);
    return () => {
      window.clearTimeout(timer);
    };
  }, [dashboardFactsAllowed, dataRevision]);

  const selectionMode = String(dashboardData?.selection_mode || (selectedAccounts.length === 1 ? "single-card" : "multi-card"));
  const objectSummary = asRecord(dashboardData?.object_summary);
  const summary = asRecord(dashboardData?.summary);
  const trend = asRecord(dashboardData?.trend);
  const heatmap = asRecord(dashboardData?.heatmap);
  const distribution = asRecord(dashboardData?.distribution);
  const flow = asRecord(dashboardData?.flow);
  const anomaly = asRecord(dashboardData?.anomaly);

  const evidenceBoundaryCopy = dashboardData && hasSelection && !dashboardFactsAllowed
    ? getDashboardEvidenceBoundaryCopy(dashboardData.evidence_status, dashboardData.blocker)
    : null;

  const heatmapView = getPanelView(chartState, "heatmap", "weekday-hour", "count");
  const distributionView = getPanelView(chartState, "distribution", String(distribution.default_view || "bank"), "amount");
  const flowView = getPanelView(chartState, "flow", "in", "amount");
  const anomalyAmountView = getPanelView(chartState, "anomalyAmount", "bucket");
  const anomalyTypeView = getPanelView(chartState, "anomalyType", "txn-type", "amount");
  const anomalyEnvView = getPanelView(chartState, "anomalyEnv", "ip");

  const trendEmpty = evidenceBoundaryCopy || getEmptyStateCopy(hasSelection, hasActiveFilters, "primary", emptyStateMode, leftSearch);
  const secondaryEmpty = evidenceBoundaryCopy || getEmptyStateCopy(hasSelection, hasActiveFilters, "secondary", emptyStateMode, leftSearch);
  const detailEmpty = evidenceBoundaryCopy || detailBoundaryCopy || getEmptyStateCopy(hasSelection, hasActiveFilters, "detail", emptyStateMode, leftSearch);

  const activeTokenByPanel = useMemo(() => buildActiveTokenByPanel(chartState.chartFilters), [chartState.chartFilters]);

  const updatePanelView = (panelId: string, patch: Partial<ChartPanelViewState>): void => {
    setChartState((prev) => updateChartPanelView(prev, panelId, patch));
  };

  const toggleFilter = (token: ChartFilterToken): void => {
    setChartState((prev) => toggleChartFilter(prev, token));
  };

  const clearFilters = (): void => {
    setChartState(clearChartFilters);
  };

  const setTrendGranularity = (nextGranularity: ChartGranularity): void => {
    setChartState((prev) => setChartTrendGranularity(prev, nextGranularity));
  };

  const resetTrendView = (): void => {
    setChartState((prev) => resetChartTrendView(prev, DEFAULT_CHART_ANALYSIS_STATE));
  };

  const summaryTiles = useMemo(() => buildSummaryTiles(summary, hasSelection && dashboardFactsAllowed), [dashboardFactsAllowed, hasSelection, summary]);

  const distributionOptions = DISTRIBUTION_OPTIONS;
  const distributionSource = getDistributionSource(distribution, String(distributionView.view || distribution.default_view || "bank"));
  const distributionItems = useMemo(() => buildDistributionItems(distributionSource), [distributionSource]);

  const heatmapMetricBasis = String(heatmapView.metric_basis || "count") === "amount" ? "amount" : "count";
  const rawHeatmapCells = asList<Record<string, unknown>>(heatmap.cells);
  const heatmapCells = useMemo(
    () => normalizeHeatmapChartCells(rawHeatmapCells, heatmapMetricBasis),
    [heatmapMetricBasis, rawHeatmapCells]
  );
  const trendPoints = asList<Record<string, unknown>>(trend.points);
  const validTrendPointCount = useMemo(
    () => normalizeTrendPoints(trendPoints, chartState.granularity).length,
    [chartState.granularity, trendPoints]
  );
  const canResetTrendView = canResetChartTrendView({
    hasSelection,
    hasActiveFilters,
    state: chartState,
    defaults: DEFAULT_CHART_ANALYSIS_STATE,
  });

  const resolvedBalanceMode = resolveBalanceMode(selectionMode, trend);

  const rawBalancePoints = resolvedBalanceMode === "balance" ? asList<Record<string, unknown>>(trend.balance_points) : asList<Record<string, unknown>>(trend.cumulative_net_points);
  const balancePoints = useMemo(
    () => normalizeBalanceTrendPoints(rawBalancePoints, resolvedBalanceMode),
    [rawBalancePoints, resolvedBalanceMode]
  );
  const balanceMarkers = asList<Record<string, unknown>>(trend.balance_markers);

  const resolvedFlowDirection = String(flowView.view || "in") === "out" ? "out" : "in";
  const flowInboundItems = asList<Record<string, unknown>>(flow.inbound_items);
  const flowOutboundItems = asList<Record<string, unknown>>(flow.outbound_items);
  const validFlowItemCount = useMemo(
    () => normalizeSankeyChartItems(flowInboundItems).length + normalizeSankeyChartItems(flowOutboundItems).length,
    [flowInboundItems, flowOutboundItems]
  );
  const flowPanelSubtitle = getFlowPanelSubtitle(resolvedFlowDirection);

  const anomalyAmountViewMode = String(anomalyAmountView.view || "bucket");
  const amountBuckets = asList<Record<string, unknown>>(anomaly.amount_buckets);
  const largeTxns = asList<Record<string, unknown>>(anomaly.large_txns);
  const txnTypeItems = useMemo(() => {
    const metricBasis = String(anomalyTypeView.metric_basis || "amount");
    return buildTxnTypeItems(asList<Record<string, unknown>>(anomaly.txn_type_top), metricBasis);
  }, [anomaly.txn_type_top, anomalyTypeView.metric_basis]);

  const ipItems = useMemo(
    () => buildIpItems(asList<Record<string, unknown>>(anomaly.ip_top)),
    [anomaly.ip_top]
  );
  const summaryKeywordItems = useMemo(
    () => buildSummaryKeywordItems(asList<Record<string, unknown>>(anomaly.summary_keyword_top)),
    [anomaly.summary_keyword_top]
  );
  const remarkKeywordItems = useMemo(
    () => buildRemarkKeywordItems(asList<Record<string, unknown>>(anomaly.remark_keyword_top)),
    [anomaly.remark_keyword_top]
  );
  const ipCoverageRate = readFiniteCaseFactNumber(asRecord(anomaly.quality).ip_non_empty_rate);
  const resolvedEnvView = useMemo(
    () => {
      const currentView = String(anomalyEnvView.view || "");
      if (ipCoverageRate == null) {
        return currentView === "summary" ? "summary" : "ip";
      }
      return resolveEnvironmentView({
        currentView,
        ipCoverageRate,
        ipItemsLength: ipItems.length,
        summaryKeywordItemsLength: summaryKeywordItems.length,
      });
    },
    [anomalyEnvView.view, ipCoverageRate, ipItems.length, summaryKeywordItems.length]
  );

  const detailColumns = useMemo<Array<VirtualizedColumn<StatsTxnRowDTO>>>(() => {
    return getChartDetailColumnDefinitions(chartState.detailColumns).map((column) => ({
      id: column.id,
      header: column.label,
      width: column.width,
      align: column.align,
      renderCell: (row) => column.renderCell(row),
    }));
  }, [chartState.detailColumns]);

  const detailExportColumns = useMemo(
    () => getChartDetailExportColumns(chartState.detailColumns),
    [chartState.detailColumns]
  );

  const onToggleDetailColumn = (columnId: string): void => {
    setChartState((prev) => {
      const next = toggleChartDetailColumnIds(prev.detailColumns, columnId);
      if (next.blocked) {
        showToast({
          tone: "info",
          title: "至少保留 1 列明细",
          detail: "请至少保留一列后，再调整当前明细视图。"
        });
        return prev;
      }
      if (next.columnIds === prev.detailColumns) {
        return prev;
      }
      return { ...prev, detailColumns: next.columnIds };
    });
  };

  const emptyState = !hasSelection || !dashboardFactsAllowed;
  const activeFilterCount = chartState.chartFilters.length;
  const resolvedSelectedObjectCount = resolveSelectedObjectCount(objectSummary, selectedObjectCount, hasSelection);
  const summaryCardNo = String(summary.card_no || "").trim();
  const summaryAccountKey = String(summary.account_key || selectedAccounts[0] || "").trim();
  const singleSelectionTitle = selectedAccounts.length === 1
    ? summaryCardNo
      ? `卡号 ${projectOrdinaryFieldValue("card_no", summaryCardNo)}`
      : summaryAccountKey
        ? `账户 ${projectOrdinaryFieldValue("account_no", summaryAccountKey)}`
        : ""
    : "";
  const heroCopy = useMemo(
    () =>
      buildDashboardHeroCopy({
        activeFilterCount,
        emptyStateMode,
        hasActiveFilters,
        hasSelection,
        leftSearch,
        resolvedSelectedObjectCount,
        singleSelectionTitle,
        selectedAccounts,
      }),
    [activeFilterCount, emptyStateMode, hasActiveFilters, hasSelection, leftSearch, resolvedSelectedObjectCount, singleSelectionTitle, selectedAccounts]
  );

  const distributionTitle = distributionView.view === "location" ? "地域分布图" : "对手银行分布图";
  const distributionSubtitle = distributionView.view === "location" ? "按交易地域查看分布，可切回银行维度" : "默认按银行查看，可切换地域";
  const canExportDetail = dashboardFactsAllowed && detailTotal !== null && !loadingDetail && detailExportColumns.length > 0 && (detailRows.length > 0 || detailTotal > 0);

  function handleExportDetail(): void {
    showToast({
      tone: "info",
      title: "受控发布尚未就绪",
      detail: controlledArtifactPublicationBlockReason(),
    });
  }

  return (
    <section className="chart-analysis-dashboard">
      <ObjectHeroPanel heroCopy={heroCopy} dateRangeLabel={formatRangeLabel(dateStart, dateEnd)} />

      <section className="chart-section-shell chart-section-shell--overview">
        <SummaryTilesPanel tiles={summaryTiles} hasSelection={hasSelection && dashboardFactsAllowed} />

        <div className={`chart-section-shell__divider chart-section-shell__divider--soft ${hasActiveFilters ? "" : "is-hidden"}`} />
        <section className={`chart-analysis-toolbar ${hasActiveFilters ? "" : "is-hidden"}`} aria-label="图表联动筛选">
          {hasActiveFilters ? (
            <>
              <div className="chart-analysis-toolbar__chips">
                {chartState.chartFilters.map((token) => (
                  <button key={`${token.source_panel_id}-${token.dimension}-${token.value}`} type="button" className="chart-filter-chip" onClick={() => toggleFilter(token)}>
                    {projectOrdinaryFieldValue(token.dimension, token.label || token.value)}
                  </button>
                ))}
              </div>
              <WorkbenchButton className="chart-toolbar-btn" tone="ghost" size="sm" type="button" onClick={clearFilters}>
                清空联动
              </WorkbenchButton>
            </>
          ) : null}
        </section>

        <div className="chart-section-shell__divider" />
        <section className="chart-grid chart-grid--primary">
          <TrendWorkspace
            points={trendPoints}
            granularity={chartState.granularity}
            metricMode={chartState.metricMode}
            activeToken={activeTokenByPanel.trend || null}
            loading={loadingDashboard}
            empty={emptyState || validTrendPointCount === 0}
            emptyTitle={trendEmpty.title}
            emptyDescription={trendEmpty.description}
            emptyVariant={trendEmpty.variant}
            canResetTrendView={canResetTrendView}
            onSelect={toggleFilter}
            onResetView={resetTrendView}
            onGranularityChange={setTrendGranularity}
            dateStart={dateStart}
            dateEnd={dateEnd}
          />
        </section>
      </section>

      <section className="chart-section-shell chart-section-shell--support-shell">
        <section className="chart-grid chart-grid--support">
          <ChartPanel
            panelId="balance"
            title={resolvedBalanceMode === "balance" ? "余额变化趋势图" : "净额累计趋势图"}
            subtitle={resolvedBalanceMode === "balance" ? "单账户显示余额变化，多账户显示净额累计" : "单账户显示余额变化，多账户显示净额累计"}
            loading={loadingDashboard}
            empty={emptyState || !balancePoints.length}
            className="chart-panel--span-4 chart-panel--narrow chart-panel--support"
            headerMode="compact"
            actionMode="menu"
            emptyTitle={secondaryEmpty.title}
            emptyDescription={secondaryEmpty.description}
            emptyVariant={secondaryEmpty.variant}
          >
            <BalanceTrendChart
              points={balancePoints}
              markers={balanceMarkers}
              granularity={chartState.granularity}
              mode={resolvedBalanceMode}
              activeToken={activeTokenByPanel.balance || null}
              onSelect={toggleFilter}
            />
          </ChartPanel>

          <ChartPanel
            panelId="heatmap"
            title="时间热力图"
            subtitle="识别小时与星期维度的行为规律"
            loading={loadingDashboard}
            empty={emptyState || !heatmapCells.length}
            className="chart-panel--span-4 chart-panel--support"
            headerMode="compact"
            actionMode="menu"
            emptyTitle={secondaryEmpty.title}
            emptyDescription={secondaryEmpty.description}
            emptyVariant={secondaryEmpty.variant}
            headerControls={
              <WorkbenchSegmentedControl
                ariaLabel="热力口径"
                className="chart-panel-segment"
                optionClassName="chart-panel-segment__option"
                indicatorClassName="chart-panel-segment__indicator"
                value={heatmapMetricBasis}
                options={[
                  { value: "count", label: "笔数" },
                  { value: "amount", label: "金额" },
                ]}
                onChange={(value) => updatePanelView("heatmap", { metric_basis: value })}
              />
            }
          >
            <HeatmapSvg cells={heatmapCells} activeToken={activeTokenByPanel.heatmap || null} onSelect={toggleFilter} />
          </ChartPanel>

          <AnomalyAmountPanel
            loading={loadingDashboard}
            emptyState={emptyState}
            emptyCopy={secondaryEmpty}
            viewMode={anomalyAmountViewMode}
            amountBuckets={amountBuckets}
            largeTxns={largeTxns}
            activeToken={activeTokenByPanel.anomalyAmount || null}
            thresholdLabel={fmtMoney(anomaly.large_txn_threshold)}
            onPanelViewChange={updatePanelView}
            onSelect={toggleFilter}
            formatMoney={fmtMoney}
            formatCount={fmtCount}
          />
        </section>
      </section>

      <section className="chart-section-shell chart-section-shell--support-shell">
        <section className="chart-grid chart-grid--support chart-grid--support-band">
          <ChartPanel
            panelId="flow"
            title="资金流向图"
            subtitle={flowPanelSubtitle}
            loading={loadingDashboard}
            empty={emptyState || validFlowItemCount === 0}
            className="chart-panel--span-7 chart-panel--support chart-panel--flow-focus"
            headerMode="compact"
            actionMode="full"
            emptyTitle={secondaryEmpty.title}
            emptyDescription={secondaryEmpty.description}
            emptyVariant={secondaryEmpty.variant}
            headerControls={
              <WorkbenchSegmentedControl
                ariaLabel="流向方向"
                className="chart-panel-segment"
                optionClassName="chart-panel-segment__option"
                indicatorClassName="chart-panel-segment__indicator"
                value={resolvedFlowDirection}
                options={[
                  { value: "in", label: "进账" },
                  { value: "out", label: "出账" },
                ]}
                onChange={(value) => updatePanelView("flow", { view: value })}
              />
            }
          >
            <SankeyChart
              centerLabel={String(flow.center_label || (selectionMode === "single-card" ? "当前账户" : "账户集合"))}
              inboundItems={flowInboundItems}
              outboundItems={flowOutboundItems}
              direction={resolvedFlowDirection}
              activeToken={activeTokenByPanel.flow || null}
              onSelect={toggleFilter}
            />
          </ChartPanel>

          <ChartPanel
            panelId="structure"
            title="关键词"
            loading={loadingDashboard}
            empty={emptyState || !remarkKeywordItems.length}
            className="chart-panel--span-5 chart-panel--support chart-panel--keyword-cloud"
            headerMode="compact"
            actionMode="menu"
            emptyTitle={secondaryEmpty.title}
            emptyDescription={secondaryEmpty.description}
            emptyVariant={secondaryEmpty.variant}
          >
            <RemarkKeywordCloud items={remarkKeywordItems} panelId="structure" activeToken={activeTokenByPanel.structure || null} onSelect={toggleFilter} />
          </ChartPanel>
        </section>
      </section>

      <section className="chart-section-shell chart-section-shell--support-shell">
        <section className="chart-grid chart-grid--support">
          <ChartPanel
            panelId="distribution"
            title={distributionTitle}
            subtitle={distributionSubtitle}
            loading={loadingDashboard}
            empty={emptyState || !distributionItems.length}
            className="chart-panel--span-4 chart-panel--narrow chart-panel--support"
            headerMode="compact"
            actionMode="menu"
            emptyTitle={secondaryEmpty.title}
            emptyDescription={secondaryEmpty.description}
            emptyVariant={secondaryEmpty.variant}
            headerControls={
              <WorkbenchSegmentedControl
                ariaLabel="分布维度"
                className="chart-panel-segment"
                optionClassName="chart-panel-segment__option"
                indicatorClassName="chart-panel-segment__indicator"
                value={String(distributionView.view || distribution.default_view || "bank")}
                options={distributionOptions}
                onChange={(value) => updatePanelView("distribution", { view: value })}
              />
            }
          >
            <HorizontalBarList
              items={distributionItems}
              panelId="distribution"
              dimension={distributionView.view === "location" ? "location" : "counterparty_bank"}
              activeToken={activeTokenByPanel.distribution || null}
              onSelect={toggleFilter}
              variant="compact"
              showRank
              labelColumn={distributionView.view === "location" ? "地域" : "银行"}
              subtitleFormatter={(item) => [`金额 ${fmtMoney(item.amount)}`, `次数 ${fmtCount(item.count)}`]}
            />
          </ChartPanel>

          <AnomalyTypePanel
            loading={loadingDashboard}
            emptyState={emptyState}
            emptyCopy={secondaryEmpty}
            metricBasis={String(anomalyTypeView.metric_basis || "amount")}
            txnTypeItems={txnTypeItems}
            activeToken={activeTokenByPanel.anomalyType || null}
            onPanelViewChange={updatePanelView}
            onSelect={toggleFilter}
            formatMoney={fmtMoney}
            formatCount={fmtCount}
          />

          <AnomalyEnvironmentPanel
            loading={loadingDashboard}
            emptyState={emptyState}
            emptyCopy={secondaryEmpty}
            resolvedEnvView={resolvedEnvView}
            ipCoverageRate={ipCoverageRate}
            ipItems={ipItems}
            summaryKeywordItems={summaryKeywordItems}
            activeToken={activeTokenByPanel.anomalyEnv || null}
            onPanelViewChange={updatePanelView}
            onSelect={toggleFilter}
            formatMoney={fmtMoney}
            formatCount={fmtCount}
          />
        </section>
      </section>

      <ChartDetailPanel
        rows={detailRows}
        total={detailTotal}
        columns={detailColumns}
        columnOptions={CHART_DETAIL_COLUMN_OPTIONS}
        visibleColumnIds={chartState.detailColumns}
        sortCol={chartState.detailSort.col}
        sortDir={chartState.detailSort.dir}
        selectionMode={selectionMode}
        loading={loadingDetail}
        empty={emptyState || !detailRows.length}
        emptyCopy={detailEmpty}
        canExport={canExportDetail}
        exporting={false}
        onClearFilters={clearFilters}
        onExport={() => void handleExportDetail()}
        onToggleColumn={onToggleDetailColumn}
        onSortColChange={(value) => setChartState((prev) => ({ ...prev, detailSort: { ...prev.detailSort, col: value } }))}
        onSortDirChange={(value) => setChartState((prev) => ({ ...prev, detailSort: { ...prev.detailSort, dir: value } }))}
      />
    </section>
  );
}
