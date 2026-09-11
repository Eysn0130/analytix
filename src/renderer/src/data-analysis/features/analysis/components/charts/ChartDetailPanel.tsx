import type { VirtualizedColumn } from "../../../../components/VirtualizedTable";
import { VirtualizedTable } from "../../../../components/VirtualizedTable";
import {
  WorkbenchButton,
  WorkbenchSegmentedControl,
  WorkbenchTableActions,
  WorkbenchTableFooter,
  WorkbenchTableShell,
  WorkbenchTableToolbar,
} from "../../../../components/workbench-ui";
import type { StatsTxnRowDTO } from "../../api/stats-api";
import {
  getChartDetailRowKey,
  type ChartDetailColumnOption,
} from "../../model/chart-detail-model";
import { formatCaseFactCount } from "../../model/stats-case-fact-number-model";
import { ChartPanel } from "./ChartPanel";

interface ChartDetailEmptyCopy {
  title: string;
  description: string;
  variant: "selection" | "filtered";
}

interface ChartDetailPanelProps {
  rows: StatsTxnRowDTO[];
  total: number | null;
  columns: Array<VirtualizedColumn<StatsTxnRowDTO>>;
  columnOptions: ChartDetailColumnOption[];
  visibleColumnIds: string[];
  sortCol: string;
  sortDir: "asc" | "desc";
  selectionMode: string;
  loading: boolean;
  empty: boolean;
  emptyCopy: ChartDetailEmptyCopy;
  canExport: boolean;
  exporting: boolean;
  onClearFilters: () => void;
  onExport: () => void;
  onToggleColumn: (columnId: string) => void;
  onSortColChange: (columnId: string) => void;
  onSortDirChange: (direction: "asc" | "desc") => void;
}

export function ChartDetailPanel({
  rows,
  total,
  columns,
  columnOptions,
  visibleColumnIds,
  sortCol,
  sortDir,
  selectionMode,
  loading,
  empty,
  emptyCopy,
  canExport,
  exporting,
  onClearFilters,
  onExport,
  onToggleColumn,
  onSortColChange,
  onSortDirChange,
}: ChartDetailPanelProps): JSX.Element {
  return (
    <ChartPanel
      panelId="detail"
      title="资金交易明细"
      subtitle="跟随上方所有图表联动过滤"
      loading={loading}
      empty={empty}
      className="chart-panel--detail"
      headerMode="default"
      actionMode="full"
      emptyTitle={emptyCopy.title}
      emptyDescription={emptyCopy.description}
      emptyVariant={emptyCopy.variant}
    >
      <WorkbenchTableShell className="chart-detail-shell">
        <WorkbenchTableToolbar className="chart-detail-toolbar">
          <div className="chart-detail-toolbar__meta">
            <div className="chart-detail-toolbar__summary">联动明细 {formatCaseFactCount(total)} 行</div>
            <div className="chart-detail-toolbar__sorts">
              <WorkbenchSegmentedControl
                ariaLabel="明细排序字段"
                className="chart-panel-segment"
                optionClassName="chart-panel-segment__option"
                indicatorClassName="chart-panel-segment__indicator"
                value={sortCol}
                options={[
                  { value: "txn_time", label: "时间" },
                  { value: "amount", label: "金额" },
                  { value: "balance", label: "余额" },
                  { value: "counterparty_name", label: "对手" },
                ]}
                onChange={onSortColChange}
              />
              <WorkbenchSegmentedControl
                ariaLabel="明细排序方向"
                className="chart-panel-segment"
                optionClassName="chart-panel-segment__option"
                indicatorClassName="chart-panel-segment__indicator"
                value={sortDir}
                options={[
                  { value: "desc", label: "降序" },
                  { value: "asc", label: "升序" },
                ]}
                onChange={onSortDirChange}
              />
            </div>
          </div>
          <WorkbenchTableActions className="chart-detail-toolbar__actions">
            <details className="chart-detail-columns">
              <summary>列显隐</summary>
              <div className="chart-detail-columns__menu">
                {columnOptions.map((column) => (
                  <button
                    key={column.id}
                    type="button"
                    className={`chart-detail-columns__item ${visibleColumnIds.includes(column.id) ? "is-active" : ""}`}
                    onClick={(event) => {
                      event.preventDefault();
                      onToggleColumn(column.id);
                    }}
                  >
                    {column.label}
                  </button>
                ))}
              </div>
            </details>
            <WorkbenchButton className="chart-toolbar-btn" tone="ghost" size="sm" type="button" onClick={onClearFilters}>
              重置筛选
            </WorkbenchButton>
            <WorkbenchButton className="chart-toolbar-btn" tone="ghost" size="sm" type="button" disabled={!canExport} onClick={onExport}>
              {exporting ? "正在导出..." : "导出明细"}
            </WorkbenchButton>
          </WorkbenchTableActions>
        </WorkbenchTableToolbar>
        <div className="chart-detail-table-wrap">
          <VirtualizedTable
            rows={rows}
            columns={columns}
            rowKey={getChartDetailRowKey}
            height={340}
            rowHeight={34}
            className="chart-detail-table"
            emptyText="暂无联动明细"
          />
        </div>
        <WorkbenchTableFooter className="chart-detail-footer">
          <div>显示列 {formatCaseFactCount(columns.length)}</div>
          <div>
            当前排序 {sortCol} / {sortDir === "desc" ? "降序" : "升序"}
          </div>
          <div>分析模式 {selectionMode === "single-card" ? "单账户" : "账户集合"}</div>
        </WorkbenchTableFooter>
      </WorkbenchTableShell>
    </ChartPanel>
  );
}
