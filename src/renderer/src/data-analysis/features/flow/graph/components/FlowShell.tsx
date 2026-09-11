import {
  DragEvent,
  Fragment,
  KeyboardEvent,
  MouseEvent as ReactMouseEvent,
  Suspense,
  lazy,
  useEffect,
  useRef,
  useState
} from "react";
import type { ReactPortal } from "react";
import { createPortal } from "react-dom";
import { WorkbenchSegmentedControl } from "../../../../components/workbench-ui";
import { projectOrdinaryFieldValue } from "../../../shared/ordinary-pii-projection";
import type { FlowAnchorRect, FlowShellBridge, FlowShellMounts, FlowShellState } from "../runtime/flow-runtime";

interface FlowShellProps {
  bridge: FlowShellBridge;
  mounts: FlowShellMounts;
  state: FlowShellState;
}

const FlowModalOverlays = lazy(async () => {
  const mod = await import("../interactions/FlowModalOverlays");
  return { default: mod.FlowModalOverlays };
});

const FlowInfoPopovers = lazy(async () => {
  const mod = await import("../interactions/FlowInfoPopovers");
  return { default: mod.FlowInfoPopovers };
});

const FlowToolbarOverlays = lazy(async () => {
  const mod = await import("../interactions/FlowToolbarOverlays");
  return { default: mod.FlowToolbarOverlays };
});

function CheckIcon(): JSX.Element {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path
        d="M20 6L9 17l-5-5"
        stroke="rgba(31,111,235,.95)"
        strokeWidth="3"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

function ChevronIcon(): JSX.Element {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path
        d="M9 6l6 6-6 6"
        stroke="rgba(2,6,23,.55)"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

function CloseIcon(): JSX.Element {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path d="M6 6L18 18" stroke="rgba(2,6,23,.65)" strokeWidth="2" strokeLinecap="round" />
      <path d="M18 6L6 18" stroke="rgba(2,6,23,.65)" strokeWidth="2" strokeLinecap="round" />
    </svg>
  );
}

function GapIcon(): JSX.Element {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path
        d="M15 6l-6 6 6 6"
        stroke="rgba(2,6,23,.6)"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

function groupCheckboxState(group: FlowShellState["treeData"][number], selected: Set<string>): {
  checked: boolean;
  indeterminate: boolean;
} {
  const ids = (group.items || []).map((item) => item.id);
  let selectedCount = 0;
  ids.forEach((id) => {
    if (selected.has(id)) {
      selectedCount += 1;
    }
  });
  if (selectedCount === 0) {
    return { checked: false, indeterminate: false };
  }
  if (selectedCount === ids.length) {
    return { checked: true, indeterminate: false };
  }
  return { checked: false, indeterminate: true };
}

function renderSymbolIcon(id: string, props: JSX.IntrinsicElements["svg"] = {}): JSX.Element {
  const { className, ...restProps } = props;
  return (
    <svg className={className ? `ico ${className}` : "ico"} aria-hidden="true" {...restProps}>
      <use href={`#${id}`} />
    </svg>
  );
}

type RectLike = {
  left: number;
  top: number;
  right: number;
  bottom: number;
  width: number;
  height: number;
};

type DomTargetLike = {
  ownerDocument?: Document | null;
  getBoundingClientRect?: () => RectLike;
  closest?: (selector: string) => Element | null;
};

function toAnchorRect(target: EventTarget | null): FlowAnchorRect {
  const fallback = { left: 0, top: 0, right: 0, bottom: 0, width: 0, height: 0 };
  const element = target as DomTargetLike | null;
  if (!element || typeof element.getBoundingClientRect !== "function") {
    return fallback;
  }
  const rect = element.getBoundingClientRect();
  return {
    left: rect.left,
    top: rect.top,
    right: rect.right,
    bottom: rect.bottom,
    width: rect.width,
    height: rect.height
  };
}

function resolveMenuAnchorTarget(target: EventTarget | null): EventTarget | null {
  const element = target as DomTargetLike | null;
  if (!element || typeof element.closest !== "function") {
    return target;
  }
  return element.closest("[data-menu-anchor='true']") ?? target;
}

function toMenuAnchorRect(target: EventTarget | null): FlowAnchorRect {
  return toAnchorRect(resolveMenuAnchorTarget(target));
}

function isToolbarMenuOpen(state: FlowShellState, sourceKey: string): boolean {
  return state.overlay.menu.open && state.overlay.menu.sourceKey === sourceKey;
}

const FLOW_TREE_TAB_OPTIONS: Array<{ value: "byName" | "byCard"; label: string }> = [
  { value: "byName", label: "按户名" },
  { value: "byCard", label: "按卡号" }
];

function isFlowTreePresentationToken(value: string, kind: "group" | "account"): boolean {
  return new RegExp(`^flow-tree-${kind}-\\d+-\\d+$`).test(value);
}

export function FlowTreePresentationList({
  bridge,
  state
}: {
  bridge: FlowShellBridge;
  state: FlowShellState;
}): JSX.Element {
  const selectedSet = new Set(state.selectedIds.filter((id) => isFlowTreePresentationToken(id, "account")));
  const expandedSet = new Set(state.expandedIds.filter((id) => isFlowTreePresentationToken(id, "group")));
  const query = projectOrdinaryFieldValue("node_id", state.search).trim().toLowerCase();
  const filteredGroups = state.treeData
    .filter((group) => isFlowTreePresentationToken(group.id, "group"))
    .map((group) => {
      const missingIdentity = group.meta === "无证件号" || group.meta === "未登记户名";
      return {
        ...group,
        title: projectOrdinaryFieldValue(group.meta === "未登记户名" ? "account_no" : "", group.title),
        meta: projectOrdinaryFieldValue(
          state.tab === "byName" && !missingIdentity ? "id_no" : "",
          group.meta
        ),
        extra: projectOrdinaryFieldValue("", group.extra || ""),
        items: (group.items || [])
          .filter((item) => isFlowTreePresentationToken(item.id, "account"))
          .map((item) => ({
            ...item,
            title: projectOrdinaryFieldValue("account_no", item.title),
            sub: projectOrdinaryFieldValue("", item.sub)
          }))
      };
    })
    .filter((group) => {
      const groupText = `${group.title} ${group.meta} ${group.extra || ""}`.toLowerCase();
      if (!query || groupText.includes(query)) {
        return true;
      }
      return group.items.some((item) => `${item.title} ${item.sub}`.toLowerCase().includes(query));
    });

  return (
    <div className="tree" aria-label="对象列表">
      {!filteredGroups.length ? (
        <div className="treeEmpty">
          {state.caseId
            ? state.treeSemanticStatus === "source_unavailable" || state.treeSemanticStatus === "blocked"
              ? "数据源不可用，无法确认对象范围"
              : "对象范围尚未取得宿主证据"
            : "未选择案件"}
        </div>
      ) : (
        filteredGroups.map((group) => {
          const groupText = `${group.title} ${group.meta} ${group.extra || ""}`.toLowerCase();
          const checkbox = groupCheckboxState(group, selectedSet);
          return (
            <div key={group.id} className={`group ${expandedSet.has(group.id) ? "open" : ""}`}>
              <div className="gHead" data-gid={group.id} onClick={() => bridge.commands.toggleGroup(group.id)}>
                <div
                  className={`gArrow ${expandedSet.has(group.id) ? "open" : ""}`}
                  onClick={(event) => {
                    event.stopPropagation();
                    bridge.commands.toggleGroup(group.id);
                  }}
                >
                  <ChevronIcon />
                </div>
                <div
                  className={`cb ${checkbox.checked ? "checked" : ""}`}
                  onClick={(event) => {
                    event.stopPropagation();
                    bridge.commands.toggleGroupSelect(group.id);
                  }}
                  style={
                    checkbox.indeterminate
                      ? {
                          background: "rgba(245,158,11,.12)",
                          borderColor: "rgba(245,158,11,.40)"
                        }
                      : undefined
                  }
                >
                  <CheckIcon />
                </div>
                <div className="gMain">
                  <div className="gTitle">{group.title}</div>
                  <div className="gMeta">{group.meta + (group.extra ? ` · ${group.extra}` : "")}</div>
                </div>
                <div className="gCount">{group.items.length}</div>
              </div>

              <div className={`gBody ${expandedSet.has(group.id) ? "open" : ""}`}>
                {group.items
                  .filter((item) => {
                    if (!query) {
                      return true;
                    }
                    const text = `${item.title} ${item.sub}`.toLowerCase();
                    return text.includes(query) || groupText.includes(query);
                  })
                  .map((item) => (
                    <div
                      key={item.id}
                      className="item"
                      data-iid={item.id}
                      onClick={() => bridge.commands.toggleItemSelect(item.id)}
                    >
                      <div className={`cb ${selectedSet.has(item.id) ? "checked" : ""}`}>
                        <CheckIcon />
                      </div>
                      <div className="iMain">
                        <div className="iTitle">{item.title}</div>
                        <div className="iSub">{item.sub}</div>
                      </div>
                    </div>
                  ))}
              </div>
            </div>
          );
        })
      )}
    </div>
  );
}


function renderLeftPanel(
  bridge: FlowShellBridge,
  mounts: FlowShellMounts,
  state: FlowShellState
): ReactPortal | null {
  if (!mounts.left) {
    return null;
  }
  const presentationCaseName = projectOrdinaryFieldValue("", state.caseName || state.caseId);

  const leftContent = (
    <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
      <div className="leftHeader">
        <div className="titleRow">
          {state.leftCollapsed ? null : <div className="title">可视化分析</div>}
          <div className="titleActions">
            {state.leftCollapsed ? null : (
              <div className="badge" title={presentationCaseName ? `案件：${presentationCaseName}` : "未选择案件"}>
                <span className="badgeDot" />
                <span className="badgeText">{presentationCaseName ? `案件：${presentationCaseName}` : "未选择案件"}</span>
              </div>
            )}
          </div>
        </div>
      </div>

      {!state.leftCollapsed ? (
        <>
          <div className="segmented">
            <WorkbenchSegmentedControl
              className="flowSegmentedControl"
              optionClassName="flowSegmentedOption"
              indicatorClassName="flowSegmentedIndicator"
              ariaLabel="对象维度"
              value={state.tab}
              options={FLOW_TREE_TAB_OPTIONS}
              onChange={(nextValue) => {
                void bridge.commands.setTab(nextValue);
              }}
            />
          </div>

          <div className="leftSearch">
            <input
              className="input"
              value={projectOrdinaryFieldValue("node_id", state.search)}
              placeholder="搜索：户名 / 卡号 / 证件号 / 开户行…"
              autoComplete="off"
              onChange={(event) => bridge.commands.setSearch(event.target.value)}
            />
            <button
              className="iconBtn"
              title="清空"
              type="button"
              aria-label="清空搜索"
              onClick={() => bridge.commands.setSearch("")}
            >
              <CloseIcon />
            </button>
          </div>

          <div className="leftTools">
            <div className="toolBtns">
              <button className="chipBtn" type="button" onClick={() => bridge.commands.selectAll()}>
                全选
              </button>
              <button className="chipBtn ghost" type="button" onClick={() => bridge.commands.clearSelection()}>
                清空
              </button>
            </div>
            <div className="selCount">{`已选 ${state.selectedCount}`}</div>
          </div>

          <LeftFilters bridge={bridge} state={state} />

          <FlowTreePresentationList bridge={bridge} state={state} />

          <div className="leftBottom">
            <button className="btn primary full" type="button" onClick={() => void bridge.commands.buildGraph()}>
              生成图谱
            </button>
            <button className="btn ghost full" type="button" onClick={() => bridge.commands.clearGraph()}>
              清空图谱
            </button>
          </div>
        </>
      ) : null}
    </div>
  );

  return createPortal(leftContent, mounts.left);
}

function LeftFilters({ bridge, state }: { bridge: FlowShellBridge; state: FlowShellState }): JSX.Element {
  const [minAmountText, setMinAmountText] = useState(String(state.filters.minAmount));
  const [maxEdgesText, setMaxEdgesText] = useState(String(state.filters.maxEdges));

  useEffect(() => {
    setMinAmountText(String(state.filters.minAmount));
  }, [state.filters.minAmount]);

  useEffect(() => {
    setMaxEdgesText(String(state.filters.maxEdges));
  }, [state.filters.maxEdges]);

  const commitMinAmount = (): void => {
    bridge.commands.setMinAmount(minAmountText.trim());
  };

  const commitMaxEdges = (): void => {
    bridge.commands.setMaxEdges(maxEdgesText.trim());
  };

  const onInputEnter = (event: KeyboardEvent<HTMLInputElement>, commit: () => void): void => {
    if (event.key === "Enter") {
      event.preventDefault();
      commit();
      event.currentTarget.blur();
    }
  };

  return (
    <div className="leftOpts">
      <div className="optRow">
        <div className="optLabel">方向</div>
        <div className="segSmall" role="tablist" aria-label="资金方向">
          {[
            { label: "全部", value: "all" },
            { label: "流入", value: "in" },
            { label: "流出", value: "out" }
          ].map((row) => (
            <button
              key={row.value}
              className={`segMini ${state.filters.dir === row.value ? "active" : ""}`}
              type="button"
              onClick={() => bridge.commands.setDir(row.value as "all" | "in" | "out")}
            >
              {row.label}
            </button>
          ))}
        </div>
      </div>
      <div className="optRow">
        <div className="optLabel">层级</div>
        <input
          className="range"
          type="range"
          min="1"
          max="3"
          value={state.filters.hop}
          onChange={(event) => bridge.commands.setHop(event.target.value)}
        />
        <div className="optValue">{state.filters.hop}</div>
      </div>
      <div className="optRow">
        <div className="optLabel">最小金额</div>
        <input
          className="input sm"
          value={minAmountText}
          inputMode="decimal"
          onChange={(event) => setMinAmountText(event.target.value)}
          onBlur={commitMinAmount}
          onKeyDown={(event) => onInputEnter(event, commitMinAmount)}
        />
      </div>
      <div className="optRow">
        <div className="optLabel">最大边数</div>
        <input
          className="input sm"
          value={maxEdgesText}
          inputMode="numeric"
          onChange={(event) => setMaxEdgesText(event.target.value)}
          onBlur={commitMaxEdges}
          onKeyDown={(event) => onInputEnter(event, commitMaxEdges)}
        />
      </div>
    </div>
  );
}

function renderGapToggle(
  bridge: FlowShellBridge,
  mounts: FlowShellMounts,
  state: FlowShellState
): ReactPortal | null {
  if (!mounts.gap) {
    return null;
  }
  return createPortal(
    <button
      className="gapToggle"
      title={state.leftCollapsed ? "展开" : "折叠"}
      type="button"
      aria-label={state.leftCollapsed ? "展开左侧" : "折叠左侧"}
      aria-expanded={!state.leftCollapsed}
      onClick={() => bridge.commands.toggleLeftCollapsed()}
    >
      <GapIcon />
    </button>,
    mounts.gap
  );
}

function renderStyleGroup(
  bridge: FlowShellBridge,
  mounts: FlowShellMounts,
  state: FlowShellState
): ReactPortal | null {
  if (!mounts.style) {
    return null;
  }

  const isStylePopoverOpen = (sourceKey: string): boolean =>
    state.overlay.stylePopover.open && state.overlay.stylePopover.sourceKey === sourceKey;
  const toggleCreateNodeMode = (): void => {
    bridge.commands.style.toggleCreateNodeMode();
  };
  const toggleCreateNodeModeFromChild = (
    event: ReactMouseEvent<SVGSVGElement | HTMLSpanElement>
  ): void => {
    event.stopPropagation();
    toggleCreateNodeMode();
  };

  const content = (
    <div className="ribbonGroupShell ribbonGroupShell--style">
      <div className="ribbonGroupBody styleBlocks">
        <div className="ribbonSub typography textBlock ribbonSubLined">
          <div className="ribbonSubBody typoGrid">
            <div className="typoRow top">
              <div className="comboField">
                <button
                  className={`comboBtn ${isStylePopoverOpen("style-font-family") ? "active" : ""}`}
                  type="button"
                  aria-label="字体"
                  title="选择字体"
                  aria-expanded={isStylePopoverOpen("style-font-family")}
                  onClick={(event: ReactMouseEvent<HTMLButtonElement>) =>
                    bridge.commands.style.openFontFamily(toAnchorRect(event.currentTarget))
                  }
                >
                  {renderSymbolIcon("ico-font")}
                  <span className="comboLabel">{state.style.fontFamilyLabel}</span>
                  <svg className="ico caret" aria-hidden="true">
                    <use href="#ico-caret" />
                  </svg>
                </button>
                <button
                  className={`comboBtn narrow ${isStylePopoverOpen("style-font-size") ? "active" : ""}`}
                  type="button"
                  aria-label="字号"
                  title="选择字号"
                  aria-expanded={isStylePopoverOpen("style-font-size")}
                  onClick={(event: ReactMouseEvent<HTMLButtonElement>) =>
                    bridge.commands.style.openFontSize(toAnchorRect(event.currentTarget))
                  }
                >
                  <span className="comboLabel">{state.style.fontSizeLabel}</span>
                  <svg className="ico caret" aria-hidden="true">
                    <use href="#ico-caret" />
                  </svg>
                </button>
              </div>
              <div className="splitBtns">
                <button
                  className="ribbonMini sizeBtn"
                  type="button"
                  aria-label="字号加一"
                  title="增大字号"
                  onClick={() => bridge.commands.style.increaseFontSize()}
                >
                  {renderSymbolIcon("ico-text-plus")}
                </button>
                <button
                  className="ribbonMini sizeBtn"
                  type="button"
                  aria-label="字号减一"
                  title="减小字号"
                  onClick={() => bridge.commands.style.decreaseFontSize()}
                >
                  {renderSymbolIcon("ico-text-minus")}
                </button>
              </div>
            </div>
            <div className="typoRow bottom">
              <div className="toggleRow">
                <button
                  className={`ribbonMini toggle ${state.style.fontBold ? "active" : ""}`}
                  type="button"
                  title="切换加粗"
                  onClick={() => bridge.commands.style.toggleBold()}
                >
                  B
                </button>
                <button
                  className={`ribbonMini toggle ${state.style.fontItalic ? "active" : ""}`}
                  type="button"
                  title="切换斜体"
                  onClick={() => bridge.commands.style.toggleItalic()}
                >
                  I
                </button>
                <button
                  className={`ribbonMini toggle ${state.style.fontUnderline ? "active" : ""}`}
                  type="button"
                  title="切换下划线"
                  onClick={() => bridge.commands.style.toggleUnderline()}
                >
                  U
                </button>
                <button
                  className={`ribbonMini toggle ${state.style.fontShadow ? "active" : ""}`}
                  type="button"
                  title="切换文字阴影"
                  onClick={() => bridge.commands.style.toggleShadow()}
                >
                  S
                </button>
              </div>
              <div className="colorRow">
                <button
                  className={`ribbonColorBtn compact ${isStylePopoverOpen("style-text-color") ? "active" : ""}`}
                  type="button"
                  title="设置文字颜色"
                  aria-expanded={isStylePopoverOpen("style-text-color")}
                  onClick={(event: ReactMouseEvent<HTMLButtonElement>) =>
                    bridge.commands.style.openTextColor(toAnchorRect(event.currentTarget))
                  }
                >
                  <span className="colorSwatch" style={{ background: state.style.textColor }} />
                  <span className="colorLabel">字体</span>
                  <svg className="ico caret" aria-hidden="true">
                    <use href="#ico-caret" />
                  </svg>
                </button>
                <button
                  className={`ribbonColorBtn compact ${isStylePopoverOpen("style-outline-color") ? "active" : ""}`}
                  type="button"
                  title="设置轮廓颜色"
                  aria-expanded={isStylePopoverOpen("style-outline-color")}
                  onClick={(event: ReactMouseEvent<HTMLButtonElement>) =>
                    bridge.commands.style.openOutlineColor(toAnchorRect(event.currentTarget))
                  }
                >
                  <span className="colorSwatch" style={{ background: state.style.outlineColor }} />
                  <span className="colorLabel">轮廓</span>
                  <svg className="ico caret" aria-hidden="true">
                    <use href="#ico-caret" />
                  </svg>
                </button>
              </div>
            </div>
          </div>
          <div className="ribbonSubCaption ribbonSubBaseline" aria-hidden="true" />
        </div>

        <div className="ribbonSubDivider">｜</div>

        <div className="ribbonSub edgeBlock ribbonSubLined">
          <div className="ribbonSubBody edgeGrid">
            <button
              className={`ribbonCellBtn inline ${isStylePopoverOpen("style-line-style") ? "active" : ""}`}
              type="button"
              title="设置线型"
              aria-expanded={isStylePopoverOpen("style-line-style")}
              onClick={(event: ReactMouseEvent<HTMLButtonElement>) =>
                bridge.commands.style.openLineStyle(toAnchorRect(event.currentTarget))
              }
            >
              {renderSymbolIcon("ico-line-style")}
              <span className="btnText">线型</span>
              <svg className="ico caret" aria-hidden="true">
                <use href="#ico-caret" />
              </svg>
            </button>
            <button
              className={`ribbonCellBtn inline ${isStylePopoverOpen("style-line-width") ? "active" : ""}`}
              type="button"
              title="设置线宽"
              aria-expanded={isStylePopoverOpen("style-line-width")}
              onClick={(event: ReactMouseEvent<HTMLButtonElement>) =>
                bridge.commands.style.openLineWidth(toAnchorRect(event.currentTarget))
              }
            >
              {renderSymbolIcon("ico-line-width")}
              <span className="btnText">线宽</span>
              <svg className="ico caret" aria-hidden="true">
                <use href="#ico-caret" />
              </svg>
            </button>
            <button
              className={`ribbonCellBtn inline ${isStylePopoverOpen("style-line-arrow") ? "active" : ""}`}
              type="button"
              title={state.style.canEditEdgeDirection ? "设置线条方向" : "方向已锁定"}
              disabled={!state.style.canEditEdgeDirection}
              aria-expanded={isStylePopoverOpen("style-line-arrow")}
              onClick={(event: ReactMouseEvent<HTMLButtonElement>) =>
                bridge.commands.style.openLineArrow(toAnchorRect(event.currentTarget))
              }
            >
              {renderSymbolIcon("ico-arrow")}
              <span className="btnText">方向</span>
              <svg className="ico caret" aria-hidden="true">
                <use href="#ico-caret" />
              </svg>
            </button>
          </div>
          <div className="ribbonSubCaption ribbonSubBaseline" aria-hidden="true" />
        </div>

        <div className="ribbonSubDivider">｜</div>

        <div className="ribbonSub nodeBlock">
          <div className="ribbonSubBody nodeGrid">
            <button
              className={`ribbonCellBtn inline nodeCell nodeCell-shape ${isStylePopoverOpen("style-node-shape") ? "active" : ""}`}
              type="button"
              title="设置节点形状"
              aria-expanded={isStylePopoverOpen("style-node-shape")}
              onClick={(event: ReactMouseEvent<HTMLButtonElement>) =>
                bridge.commands.style.openNodeShape(toAnchorRect(event.currentTarget))
              }
            >
              {renderSymbolIcon("ico-node-shape")}
              <span className="btnText">形状</span>
              <svg className="ico caret" aria-hidden="true">
                <use href="#ico-caret" />
              </svg>
            </button>
            <button
              className={`ribbonColorBtn compact nodeCell nodeCell-fill nodeFillBtn ${isStylePopoverOpen("style-node-fill") ? "active" : ""}`}
              type="button"
              title="设置节点填充"
              aria-expanded={isStylePopoverOpen("style-node-fill")}
              onClick={(event: ReactMouseEvent<HTMLButtonElement>) =>
                bridge.commands.style.openNodeFill(toAnchorRect(event.currentTarget))
              }
            >
              <span className="colorSwatch" style={{ background: state.style.nodeFill }} />
              <span className="colorLabel">填充</span>
              <svg className="ico caret" aria-hidden="true">
                <use href="#ico-caret" />
              </svg>
            </button>
            <button
              className={`ribbonCellBtn inline nodeCell nodeCell-size ${isStylePopoverOpen("style-icon-size") ? "active" : ""}`}
              type="button"
              title="设置节点大小"
              aria-expanded={isStylePopoverOpen("style-icon-size")}
              onClick={(event: ReactMouseEvent<HTMLButtonElement>) =>
                bridge.commands.style.openIconSize(toAnchorRect(event.currentTarget))
              }
            >
              {renderSymbolIcon("ico-icon-size")}
              <span className="btnText">大小</span>
              <svg className="ico caret" aria-hidden="true">
                <use href="#ico-caret" />
              </svg>
            </button>
            <button
              className={`ribbonCellBtn inline nodeCell nodeCell-new ${state.style.createNodeMode ? "active" : ""}`}
              type="button"
              title="新建节点"
              onClick={toggleCreateNodeMode}
            >
              {renderSymbolIcon("ico-plus", { onClick: toggleCreateNodeModeFromChild })}
              <span className="btnText" onClick={toggleCreateNodeModeFromChild}>
                新建
              </span>
            </button>
            <button
              className={`ribbonCellBtn inline nodeCell nodeCell-style ${isStylePopoverOpen("style-icon-library") ? "active" : ""}`}
              type="button"
              title="选择节点样式"
              aria-expanded={isStylePopoverOpen("style-icon-library")}
              onClick={(event: ReactMouseEvent<HTMLButtonElement>) =>
                bridge.commands.style.openIconLibrary(toAnchorRect(event.currentTarget))
              }
            >
              {renderSymbolIcon("ico-icon-library")}
              <span className="btnText">样式</span>
              <svg className="ico caret" aria-hidden="true">
                <use href="#ico-caret" />
              </svg>
            </button>
            <button
              className="ribbonCellBtn inline nodeCell nodeCell-link"
              type="button"
              title="连接选中节点"
              onClick={() => bridge.commands.style.createLinkFromSelection()}
            >
              {renderSymbolIcon("ico-link")}
              <span className="btnText">链接</span>
            </button>
          </div>
          <div className="ribbonSubCaption ribbonSubBaseline" aria-hidden="true" />
        </div>
      </div>
      <div className="ribbonCaption ribbonCaptionGhost">样式</div>
    </div>
  );

  return createPortal(content, mounts.style);
}

function renderLayoutGroup(
  bridge: FlowShellBridge,
  mounts: FlowShellMounts,
  state: FlowShellState
): ReactPortal | null {
  if (!mounts.layout) {
    return null;
  }

  const rows = [
    { id: "btnLayoutCompact", preset: "compact", label: "关联图", icon: "ico-layout-compact" },
    { id: "btnLayoutNetwork", preset: "network", label: "网络图", icon: "ico-layout-network" },
    { id: "btnLayoutHierarchy", preset: "hierarchy", label: "层级图", icon: "ico-layout-hierarchy" },
    { id: "btnLayoutFlow", preset: "flow", label: "流向图", icon: "ico-layout-flow" }
  ];

  const content = (
    <div className="ribbonGroupShell ribbonGroupShell--lined">
      <div className="ribbonGroupBody layoutGrid">
        {rows.map((row) => {
          const isActive = state.layout.selected && state.layout.preset === row.preset;
          const hasMenu = row.preset === "hierarchy" || row.preset === "flow";
          if (!hasMenu) {
            return (
              <button
                key={row.id}
                className={`ribbonTile layoutBtn ${isActive ? "active" : ""}`}
                type="button"
                title={`切换${row.label}`}
                onClick={() => bridge.commands.layout.setPreset(row.preset as "compact" | "network" | "hierarchy" | "flow")}
              >
                {renderSymbolIcon(row.icon)}
                <span className="btnText">{row.label}</span>
              </button>
            );
          }

          const mode = row.preset as "hierarchy" | "flow";
          const sourceKey = `layout-edge-routing-${mode}`;
          const menuOpen = isToolbarMenuOpen(state, sourceKey);

          return (
            <div
              key={row.id}
              className={`ribbonSplitTile layoutSplit ${isActive ? "active" : ""} ${menuOpen ? "menu-open" : ""}`}
              data-menu-anchor="true"
            >
              <button
                className="ribbonSplitMain"
                type="button"
                title={`切换${row.label}`}
                onClick={() => bridge.commands.layout.setPreset(mode)}
                onContextMenu={(event) => {
                  event.preventDefault();
                  bridge.commands.layout.openEdgeRoutingMenu(mode, toMenuAnchorRect(event.currentTarget));
                }}
              >
                {renderSymbolIcon(row.icon)}
                <span className="btnText">{row.label}</span>
              </button>
              <button
                className="ribbonSplitTrigger"
                type="button"
                aria-label={`${row.label} 更多选项`}
                title={`${row.label}选项`}
                aria-haspopup="menu"
                aria-expanded={menuOpen}
                onClick={(event: ReactMouseEvent<HTMLButtonElement>) => {
                  event.preventDefault();
                  event.stopPropagation();
                  bridge.commands.layout.openEdgeRoutingMenu(mode, toMenuAnchorRect(event.currentTarget));
                }}
              >
                <svg className="ico caret" aria-hidden="true">
                  <use href="#ico-caret" />
                </svg>
              </button>
            </div>
          );
        })}
      </div>
      <div className="ribbonCaption ribbonCaptionBaseline">布局</div>
    </div>
  );

  return createPortal(content, mounts.layout);
}

function renderAnalyzeGroup(
  bridge: FlowShellBridge,
  mounts: FlowShellMounts,
  state: FlowShellState
): ReactPortal | null {
  if (!mounts.analyze) {
    return null;
  }

  const content = (
    <div className="ribbonGroupShell ribbonGroupShell--lined">
      <div className="ribbonGroupBody analyzeGrid">
        <div className="ribbonRow analyzeTopRow">
          <button
            className={`ribbonTile analysisCmdBtn drop ${state.analysis.filterActive ? "active" : ""}`}
            data-analysis-slot="filter"
            type="button"
            title="按金额筛选"
            aria-expanded={state.overlay.filter.open}
            onClick={(event: ReactMouseEvent<HTMLButtonElement>) =>
              bridge.commands.analysis.openFilter(toAnchorRect(event.currentTarget))
            }
          >
            {renderSymbolIcon("ico-filter")}
            <span className="btnText">筛选</span>
            <svg className="ico caret" aria-hidden="true">
              <use href="#ico-caret" />
            </svg>
          </button>
          <button
            className={`ribbonTile analysisCmdBtn ${state.analysis.collapseChildren ? "active" : ""}`}
            data-analysis-slot="collapse"
            type="button"
            title="收缩弱关联"
            onClick={() => bridge.commands.analysis.toggleCollapseChildren()}
          >
            {renderSymbolIcon("ico-collapse")}
            <span className="btnText">收缩</span>
          </button>
          <button
            className="ribbonTile comboBtn"
            data-analysis-slot="group1"
            type="button"
            title="加入组合 1"
            onClick={() => bridge.commands.analysis.assignGroupTag(1)}
          >
            <span className="btnText">组合 1</span>
          </button>
          <button
            className="ribbonTile comboBtn"
            data-analysis-slot="group2"
            type="button"
            title="加入组合 2"
            onClick={() => bridge.commands.analysis.assignGroupTag(2)}
          >
            <span className="btnText">组合 2</span>
          </button>
          <button
            className="ribbonTile comboBtn"
            data-analysis-slot="group3"
            type="button"
            title="加入组合 3"
            onClick={() => bridge.commands.analysis.assignGroupTag(3)}
          >
            <span className="btnText">组合 3</span>
          </button>
          <button
            className={`ribbonTile comboBtn drop ${state.overlay.menu.open && state.overlay.menu.sourceKey === "analysis-group-ops" ? "active" : ""}`}
            data-analysis-slot="groupOps"
            type="button"
            title="组合运算"
            aria-expanded={state.overlay.menu.open && state.overlay.menu.sourceKey === "analysis-group-ops"}
            onClick={(event: ReactMouseEvent<HTMLButtonElement>) =>
              bridge.commands.analysis.openGroupOps(toAnchorRect(event.currentTarget))
            }
          >
            <span className="btnText">更多</span>
            <svg className="ico caret" aria-hidden="true">
              <use href="#ico-caret" />
            </svg>
          </button>
        </div>
        <div className="ribbonRow row3 analyzeBottomRow">
          <button
            className={`ribbonTile analysisBtn ${state.analysis.encodeWidth ? "active" : ""}`}
            type="button"
            title="金额映射粗细"
            onClick={() => bridge.commands.analysis.toggleEncodeWidth()}
          >
            {renderSymbolIcon("ico-encode-width")}
            <span className="btnText">金额→粗细</span>
          </button>
          <button
            className={`ribbonTile analysisBtn ${state.analysis.encodeColor ? "active" : ""}`}
            type="button"
            title="金额映射颜色"
            onClick={() => bridge.commands.analysis.toggleEncodeColor()}
          >
            {renderSymbolIcon("ico-encode-color")}
            <span className="btnText">金额→颜色</span>
          </button>
          <button
            className={`ribbonTile analysisBtn ${state.analysis.nodeScale ? "active" : ""}`}
            type="button"
            title="金额映射节点"
            onClick={() => bridge.commands.analysis.toggleNodeScale()}
          >
            {renderSymbolIcon("ico-node-scale")}
            <span className="btnText">节点缩放</span>
          </button>
        </div>
      </div>
      <div className="ribbonCaption ribbonCaptionBaseline">分析</div>
    </div>
  );

  return createPortal(content, mounts.analyze);
}

function renderOpsGroup(
  bridge: FlowShellBridge,
  mounts: FlowShellMounts,
  state: FlowShellState
): ReactPortal | null {
  if (!mounts.ops) {
    return null;
  }

  const content = (
    <div className="ribbonGroupShell ribbonGroupShell--lined">
      <div className="ribbonGroupBody opsGrid">
        <div className="ribbonRow row2">
          <div
            className={`ribbonSplitTile mergeSplit ${isToolbarMenuOpen(state, "ops-merge") ? "menu-open active" : ""}`}
            data-menu-anchor="true"
          >
            <button
              className="ribbonSplitMain"
              type="button"
              title="合并同名节点"
              onClick={() => bridge.commands.ops.mergeNodes()}
              onContextMenu={(event) => {
                event.preventDefault();
                bridge.commands.ops.openMergeMenu(toMenuAnchorRect(event.currentTarget));
              }}
            >
              {renderSymbolIcon("ico-group-2")}
              <span className="btnText">合并</span>
            </button>
            <button
              className="ribbonSplitTrigger"
              type="button"
              aria-label="合并 更多选项"
              title="合并选项"
              aria-haspopup="menu"
              aria-expanded={isToolbarMenuOpen(state, "ops-merge")}
              onClick={(event: ReactMouseEvent<HTMLButtonElement>) => {
                event.preventDefault();
                event.stopPropagation();
                bridge.commands.ops.openMergeMenu(toMenuAnchorRect(event.currentTarget));
              }}
            >
              <svg className="ico caret" aria-hidden="true">
                <use href="#ico-caret" />
              </svg>
            </button>
          </div>
          <button className="ribbonTile" type="button" title="适配当前视图" onClick={() => bridge.commands.ops.fitGraph()}>
            {renderSymbolIcon("ico-fit")}
            <span className="btnText">适配视图</span>
          </button>
          <button
            className="ribbonTile drop"
            type="button"
            title="导出图谱"
            aria-expanded={state.overlay.exportPanel.open}
            onClick={(event: ReactMouseEvent<HTMLButtonElement>) =>
              bridge.commands.ops.openExport(toAnchorRect(event.currentTarget))
            }
          >
            {renderSymbolIcon("ico-export")}
            <span className="btnText">导出</span>
            <svg className="ico caret" aria-hidden="true">
              <use href="#ico-caret" />
            </svg>
          </button>
        </div>
        <div className="ribbonRow row2">
          <button
            className={`ribbonTile detailBtn ${state.ops.edgeDetail ? "active" : ""}`}
            type="button"
            title="显示连线详情"
            onClick={() => bridge.commands.ops.toggleEdgeDetail()}
          >
            {renderSymbolIcon("ico-detail")}
            <span className="btnText">详细</span>
          </button>
          <button
            className="ribbonTile"
            type="button"
            title="恢复初始样式"
            disabled={!state.ops.canRedo}
            onClick={() => bridge.commands.ops.redo()}
          >
            {renderSymbolIcon("ico-redo")}
            <span className="btnText">重做</span>
          </button>
          <button
            className="ribbonTile"
            type="button"
            title="撤销上一步"
            disabled={!state.ops.canUndo}
            onClick={() => bridge.commands.ops.undo()}
          >
            {renderSymbolIcon("ico-undo")}
            <span className="btnText">撤销</span>
          </button>
        </div>
      </div>
      <div className="ribbonCaption ribbonCaptionBaseline">操作</div>
    </div>
  );

  return createPortal(content, mounts.ops);
}

export function FlowShell({ bridge, mounts, state }: FlowShellProps): JSX.Element {
  return (
    <>
      {renderLeftPanel(bridge, mounts, state)}
      {renderGapToggle(bridge, mounts, state)}
      {renderStyleGroup(bridge, mounts, state)}
      {renderLayoutGroup(bridge, mounts, state)}
      {renderAnalyzeGroup(bridge, mounts, state)}
      {renderOpsGroup(bridge, mounts, state)}
      <Suspense fallback={null}>
        <FlowToolbarOverlays bridge={bridge} mounts={mounts} state={state} />
      </Suspense>
      <Suspense fallback={null}>
        <FlowInfoPopovers bridge={bridge} mounts={mounts} state={state} />
      </Suspense>
      <Suspense fallback={null}>
        <FlowModalOverlays bridge={bridge} mounts={mounts} state={state} />
      </Suspense>
    </>
  );
}
