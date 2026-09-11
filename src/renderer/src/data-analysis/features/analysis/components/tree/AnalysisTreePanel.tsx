import { useCallback, useEffect, useRef, useState, type MutableRefObject } from "react";
import { WorkbenchButton, WorkbenchSegmentedControl } from "../../../../components/workbench-ui";
import type { StatsTreeTab } from "../../api/stats-api";
import type { StatsPreparedTreeGroup } from "../../resources/stats-tree-resource";
import { projectOrdinaryFieldValue } from "../../../shared/ordinary-pii-projection";

const TREE_TAB_OPTIONS: Array<{ value: StatsTreeTab; label: string }> = [
  { value: "byName", label: "户名" },
  { value: "byCard", label: "卡号" }
];

interface AnalysisTreePanelProps {
  panelRef: MutableRefObject<HTMLElement | null>;
  toolbarActionsRef: MutableRefObject<HTMLDivElement | null>;
  searchInputRef: MutableRefObject<HTMLInputElement | null>;
  leftCollapsed: boolean;
  treeTab: StatsTreeTab;
  leftSearch: string;
  leftSearchExpanded: boolean;
  visibleGroups: StatsPreparedTreeGroup[];
  selectedAccounts: string[];
  selectedAccountsSet: Set<string>;
  expandedGroupSet: Set<string>;
  showTreeBlockingState: boolean;
  treeUnavailable: boolean;
  collapsedDimensionTitle: string;
  collapsedDimensionValue: string;
  collapsedSelectedObjectCount: number;
  collapsedSelectedObjectCountText: string;
  collapsedSelectedCardCountText: string;
  onTreeTabChange: (value: StatsTreeTab) => void;
  onLeftSearchChange: (value: string) => void;
  onOpenLeftSearch: () => void;
  onCollapseLeftSearch: () => void;
  onSelectAllAccounts: () => void;
  onClearSelectedAccounts: () => void;
  onGroupExpandToggle: (groupId: string) => void;
  onGroupSelectedChange: (group: StatsPreparedTreeGroup, checked: boolean) => void;
  onAccountToggle: (groupId: string, accountKey: string) => void;
  onEmptySelectionContextMenu: () => void;
  onCopyTreeItemCardNumber: (text: string) => void | Promise<void>;
}

let textMeasureCanvas: HTMLCanvasElement | null = null;

function measureTextWidth(text: string, font: string): number {
  if (typeof document === "undefined") {
    return text.length * 8;
  }
  if (!textMeasureCanvas) {
    textMeasureCanvas = document.createElement("canvas");
  }
  const context = textMeasureCanvas.getContext("2d");
  if (!context) {
    return text.length * 8;
  }
  context.font = font;
  return context.measureText(text).width;
}

function fitMiddleEllipsisText(text: string, maxWidth: number, font: string, tailChars: number): string {
  const raw = String(text || "").trim();
  if (!raw || maxWidth <= 0 || raw.length <= tailChars + 1) {
    return raw;
  }
  if (measureTextWidth(raw, font) <= maxWidth) {
    return raw;
  }
  const ellipsis = "...";
  const tail = raw.slice(-tailChars);
  const tailWidth = measureTextWidth(tail, font);
  const ellipsisWidth = measureTextWidth(ellipsis, font);
  const availableWidth = maxWidth - tailWidth - ellipsisWidth;
  if (availableWidth <= 0) {
    return `${raw.slice(0, 1)}${ellipsis}${tail}`;
  }
  let low = 1;
  let high = raw.length - tailChars;
  let best = 1;
  while (low <= high) {
    const mid = Math.floor((low + high) / 2);
    const head = raw.slice(0, mid);
    if (measureTextWidth(head, font) <= availableWidth) {
      best = mid;
      low = mid + 1;
    } else {
      high = mid - 1;
    }
  }
  return `${raw.slice(0, best)}${ellipsis}${tail}`;
}

function MiddleEllipsisText({
  text,
  className,
  tailChars = 4
}: {
  text: string;
  className: string;
  tailChars?: number;
}): JSX.Element {
  const ref = useRef<HTMLDivElement | null>(null);
  const [displayText, setDisplayText] = useState(String(text || "").trim());

  useEffect(() => {
    const element = ref.current;
    if (!element) {
      return;
    }
    let rafId = 0;
    const update = (): void => {
      const node = ref.current;
      if (!node) {
        return;
      }
      const raw = String(text || "").trim();
      if (!raw) {
        setDisplayText("");
        return;
      }
      const styles = window.getComputedStyle(node);
      const font = styles.font || `${styles.fontWeight} ${styles.fontSize} ${styles.fontFamily}`;
      const next = fitMiddleEllipsisText(raw, node.clientWidth, font, tailChars);
      setDisplayText((prev) => (prev === next ? prev : next));
    };
    const schedule = (): void => {
      if (rafId) {
        window.cancelAnimationFrame(rafId);
      }
      rafId = window.requestAnimationFrame(update);
    };

    schedule();
    const resizeObserver = typeof ResizeObserver !== "undefined" ? new ResizeObserver(schedule) : null;
    resizeObserver?.observe(element);
    window.addEventListener("resize", schedule);

    return () => {
      if (rafId) {
        window.cancelAnimationFrame(rafId);
      }
      resizeObserver?.disconnect();
      window.removeEventListener("resize", schedule);
    };
  }, [tailChars, text]);

  return (
    <div ref={ref} className={className} title={text}>
      {displayText}
    </div>
  );
}

export function AnalysisTreePanel({
  panelRef,
  toolbarActionsRef,
  searchInputRef,
  leftCollapsed,
  treeTab,
  leftSearch,
  leftSearchExpanded,
  visibleGroups,
  selectedAccounts,
  selectedAccountsSet,
  expandedGroupSet,
  showTreeBlockingState,
  treeUnavailable,
  collapsedDimensionTitle,
  collapsedDimensionValue,
  collapsedSelectedObjectCount,
  collapsedSelectedObjectCountText,
  collapsedSelectedCardCountText,
  onTreeTabChange,
  onLeftSearchChange,
  onOpenLeftSearch,
  onCollapseLeftSearch,
  onSelectAllAccounts,
  onClearSelectedAccounts,
  onGroupExpandToggle,
  onGroupSelectedChange,
  onAccountToggle,
  onEmptySelectionContextMenu,
  onCopyTreeItemCardNumber
}: AnalysisTreePanelProps): JSX.Element {
  const [suppressedTreeCopyItemId, setSuppressedTreeCopyItemId] = useState<string | null>(null);
  const setPanelNode = useCallback(
    (node: HTMLElement | null): void => {
      panelRef.current = node;
    },
    [panelRef]
  );
  const setToolbarActionsNode = useCallback(
    (node: HTMLDivElement | null): void => {
      toolbarActionsRef.current = node;
    },
    [toolbarActionsRef]
  );
  const setSearchInputNode = useCallback(
    (node: HTMLInputElement | null): void => {
      searchInputRef.current = node;
    },
    [searchInputRef]
  );

  return (
    <section className="card left" id="leftCard" ref={setPanelNode}>
      <div className="leftControlsBar">
        <div className="leftToolbarRow">
          <WorkbenchSegmentedControl
            className={`treeSwitch ${treeTab === "byCard" ? "is-by-card" : "is-by-name"}`}
            optionClassName="treeSwitchChip"
            indicatorClassName="treeSwitchIndicator"
            ariaLabel="对象维度"
            value={treeTab}
            options={TREE_TAB_OPTIONS}
            onChange={onTreeTabChange}
          />
        </div>

        <div className="leftToolbarRow">
          <div className={`leftTools ${leftSearchExpanded ? "is-search-open" : ""}`} ref={setToolbarActionsNode}>
            <div className="toolbarActionGrid" role="toolbar" aria-label="对象操作" aria-hidden={leftSearchExpanded}>
              <WorkbenchButton
                className="chipBtn navStyle withBadge toolbarActionBtn"
                type="button"
                onClick={onSelectAllAccounts}
                disabled={!visibleGroups.length || leftSearchExpanded}
                badge={selectedAccounts.length ? (selectedAccounts.length > 99 ? "99+" : selectedAccounts.length) : null}
              >
                全选
              </WorkbenchButton>
              <WorkbenchButton
                className="chipBtn navStyle ghost toolbarActionBtn"
                tone="ghost"
                type="button"
                onClick={onClearSelectedAccounts}
                disabled={!selectedAccounts.length || leftSearchExpanded}
              >
                清空
              </WorkbenchButton>
              <WorkbenchButton
                className="chipBtn navStyle toolbarActionBtn"
                type="button"
                aria-expanded={leftSearchExpanded}
                aria-label="搜索对象"
                disabled={leftSearchExpanded}
                onClick={onOpenLeftSearch}
              >
                搜索
              </WorkbenchButton>
            </div>

            <div className="toolbarSearchField" role="search" aria-hidden={!leftSearchExpanded}>
              <input
                ref={setSearchInputNode}
                className="toolbarSearchInput"
                disabled={!leftSearchExpanded}
                tabIndex={leftSearchExpanded ? 0 : -1}
                value={leftSearch}
                onChange={(event) => onLeftSearchChange(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Escape") {
                    event.preventDefault();
                    onCollapseLeftSearch();
                  }
                }}
                placeholder="搜索：户名/卡号/证件号/开户行"
                autoComplete="off"
              />
              <button
                className="toolbarSearchClear"
                title="取消搜索"
                type="button"
                aria-label="删除搜索并收起"
                disabled={!leftSearchExpanded}
                tabIndex={leftSearchExpanded ? 0 : -1}
                onClick={onCollapseLeftSearch}
              >
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" aria-hidden="true">
                  <path d="M7 7L17 17" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
                  <path d="M17 7L7 17" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
                </svg>
              </button>
            </div>
          </div>
        </div>
      </div>

      {leftCollapsed ? (
        <div className="leftCollapsedPanel" aria-label="折叠态信息面板">
          <div className="leftCollapsedCard" title={collapsedDimensionTitle}>
            <span className="leftCollapsedCardLabel">维度</span>
            <span className="leftCollapsedCardValue">{collapsedDimensionValue}</span>
          </div>

          <div
            className={`leftCollapsedSelectionCard ${selectedAccounts.length ? "has-selection" : ""}`}
            title={`当前选择：对象 ${collapsedSelectedObjectCount}，卡号 ${selectedAccounts.length}`}
          >
            <div className="leftCollapsedSelectionTitle">选择</div>
            <div className="leftCollapsedSelectionRows">
              <div className="leftCollapsedSelectionRow">
                <span className="leftCollapsedSelectionKey">对象</span>
                <span className="leftCollapsedSelectionValue">{collapsedSelectedObjectCountText}</span>
              </div>
              <div className="leftCollapsedSelectionRow">
                <span className="leftCollapsedSelectionKey">卡号</span>
                <span className="leftCollapsedSelectionValue">{collapsedSelectedCardCountText}</span>
              </div>
            </div>
          </div>
        </div>
      ) : null}

      <div className="tree" aria-label="对象列表">
        {showTreeBlockingState ? <div className="treeEmpty">加载中...</div> : null}
        {!showTreeBlockingState && treeUnavailable ? <div className="treeEmpty">数据源不可用，无法确认对象范围</div> : null}
        {!showTreeBlockingState && !treeUnavailable && !visibleGroups.length ? <div className="treeEmpty">暂无数据</div> : null}
        {visibleGroups.map((group) => {
          const itemKeys = group.itemKeys;
          const checkedCount = itemKeys.filter((key) => selectedAccountsSet.has(key)).length;
          const allChecked = itemKeys.length > 0 && checkedCount === itemKeys.length;
          const indeterminate = checkedCount > 0 && checkedCount < itemKeys.length;
          const expanded = expandedGroupSet.has(group.id);
          const groupTitle = projectOrdinaryFieldValue(group.isAccountLikeGroup ? "account_no" : "", group.title);
          const groupSecondaryText = treeTab === "byName"
            ? projectOrdinaryFieldValue("identity_no", group.secondaryText)
            : projectOrdinaryFieldValue("", group.secondaryText);
          return (
            <div className={`group ${expanded ? "open" : ""}`} key={group.id}>
              <div
                className="gHead"
                role="button"
                tabIndex={0}
                aria-expanded={expanded}
                onClick={() => onGroupExpandToggle(group.id)}
                onKeyDown={(event) => {
                  if (event.key === "Enter" || event.key === " ") {
                    event.preventDefault();
                    onGroupExpandToggle(group.id);
                  }
                }}
                onContextMenu={(event) => {
                  event.preventDefault();
                  if (!selectedAccounts.length) {
                    onEmptySelectionContextMenu();
                  }
                }}
              >
                <button
                  type="button"
                  className={`cb ${allChecked ? "checked" : ""} ${indeterminate ? "indeterminate" : ""}`}
                  onClick={(event) => {
                    event.stopPropagation();
                    onGroupSelectedChange(group, !allChecked);
                  }}
                  aria-label="切换分组选择"
                >
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none">
                    <path d="M20 6L9 17l-5-5" stroke="rgba(31,111,235,.95)" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round" />
                  </svg>
                </button>
                <div className={`gAvatar ${group.isAccountLikeGroup ? "card" : "user"}`} aria-hidden="true">
                  {group.isAccountLikeGroup ? (
                    <svg viewBox="0 0 24 24" fill="none">
                      <rect x="4.25" y="6.75" width="15.5" height="10.5" rx="2.2" />
                      <path d="M4.25 10.35h15.5" strokeLinecap="round" />
                      <path d="M7.4 14.1h3.2" strokeLinecap="round" />
                    </svg>
                  ) : (
                    <svg viewBox="0 0 24 24" fill="none">
                      <circle cx="12" cy="8" r="3.2" />
                      <path d="M6.8 18.2c1.4-3 3.3-4.4 5.2-4.4s3.8 1.4 5.2 4.4" strokeLinecap="round" />
                    </svg>
                  )}
                </div>
                <div className="gMain">
                  <div className="gTitle">{groupTitle || "未命名分组"}</div>
                  <div className="gMeta" title={groupSecondaryText}>
                    {groupSecondaryText}
                  </div>
                  <div className="gAside">
                    <button
                      type="button"
                      className={`gToggleChip ${expanded ? "open" : ""}`}
                      onClick={(event) => {
                        event.stopPropagation();
                        onGroupExpandToggle(group.id);
                      }}
                      aria-label={expanded ? "收起用户银行卡列表" : "展开用户银行卡列表"}
                      aria-expanded={expanded}
                    >
                      <span className="gToggleChipCount">{group.items.length}</span>
                      <svg viewBox="0 0 24 24" fill="none">
                        <path d="m8 10 4 4 4-4" strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" />
                      </svg>
                    </button>
                  </div>
                </div>
              </div>
              <div className={`gBody ${expanded ? "open" : ""}`}>
                {expanded
                  ? group.items.map((item) => {
                      const checked = selectedAccountsSet.has(item.id);
                      const copySuppressed = suppressedTreeCopyItemId === item.id;
                      const projectedCardNumber = projectOrdinaryFieldValue("card_no", item.primaryText);
                      const projectedSecondaryText = projectOrdinaryFieldValue("", item.secondaryText);
                      return (
                        <div
                          className={`item ${checked ? "selected" : ""} ${copySuppressed ? "copy-suppressed" : ""}`}
                          key={item.id}
                          onClick={() => onAccountToggle(group.id, item.id)}
                          onMouseLeave={() => {
                            setSuppressedTreeCopyItemId((prev) => (prev === item.id ? null : prev));
                          }}
                        >
                          <div className={`cb ${checked ? "checked" : ""}`}>
                            <svg width="14" height="14" viewBox="0 0 24 24" fill="none">
                              <path d="M20 6L9 17l-5-5" stroke="rgba(31,111,235,.95)" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round" />
                            </svg>
                          </div>
                          <div className="iMain">
                            <MiddleEllipsisText className="iTitle" text={projectedCardNumber} tailChars={4} />
                            {item.secondaryText ? (
                              <div className="iSub">
                                <span className="iSubText" title={projectedSecondaryText}>
                                  {projectedSecondaryText}
                                </span>
                                <button
                                  type="button"
                                  className="iCopyBtn"
                                  title="复制卡号脱敏副本"
                                  aria-label={`复制脱敏卡号 ${projectedCardNumber}`}
                                  onClick={(event) => {
                                    event.stopPropagation();
                                    event.currentTarget.blur();
                                    setSuppressedTreeCopyItemId(item.id);
                                    onCopyTreeItemCardNumber(item.primaryText);
                                  }}
                                >
                                  <svg viewBox="0 0 24 24" fill="none" aria-hidden="true" focusable="false">
                                    <rect x="8" y="8" width="10" height="10" rx="2" strokeWidth="2" />
                                    <path d="M6 14H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h7a2 2 0 0 1 2 2v1" strokeWidth="2" strokeLinecap="round" />
                                  </svg>
                                </button>
                              </div>
                            ) : null}
                          </div>
                        </div>
                      );
                    })
                  : null}
              </div>
            </div>
          );
        })}
      </div>
    </section>
  );
}
