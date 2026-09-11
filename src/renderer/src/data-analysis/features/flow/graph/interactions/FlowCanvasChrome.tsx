import {
  DragEvent,
  KeyboardEvent,
  MouseEvent as ReactMouseEvent,
  useEffect,
  useRef,
  useState
} from "react";
import { createPortal } from "react-dom";
import { projectOrdinaryFieldValue } from "../../../shared/ordinary-pii-projection";
import type { FlowShellBridge, FlowShellMounts, FlowShellState } from "../runtime/flow-runtime";
import { stopEventPropagation } from "./FlowOverlayPrimitives";

interface FlowCanvasChromeProps {
  bridge: FlowShellBridge;
  mounts: FlowShellMounts;
  state: FlowShellState;
}

function CloseIcon(): JSX.Element {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path d="M6 6L18 18" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
      <path d="M18 6L6 18" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
    </svg>
  );
}

function PlusIcon(): JSX.Element {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path d="M12 5V19" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
      <path d="M5 12H19" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
    </svg>
  );
}

function ViewportExpandIcon(): JSX.Element {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path d="M5 9V5h4" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M15 5h4v4" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M19 15v4h-4" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M9 19H5v-4" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

function PathExpandIcon(): JSX.Element {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <circle cx="6" cy="6" r="2" fill="currentColor" />
      <circle cx="18" cy="18" r="2" fill="currentColor" />
      <circle cx="18" cy="6" r="2" fill="currentColor" />
      <path d="M8 6H16" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
      <path d="M18 8V14" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
    </svg>
  );
}

function displayViewTitle(rawTitle: string): string {
  const title = String(rawTitle || "").trim();
  const match =
    title.match(/^图(\d+)$/) ||
    title.match(/^视图(\d+)$/) ||
    title.match(/^起始页(?:\s*(\d+))?$/);
  if (match) {
    const matchedNumber = match[1] || (title === "起始页" ? "1" : "");
    const num = /^\d{1,6}$/.test(matchedNumber) ? String(Number(matchedNumber) || "") : matchedNumber;
    return projectOrdinaryFieldValue("node_id", `图${num}`) || "图";
  }
  return projectOrdinaryFieldValue("node_id", title || "图") || "图";
}

function formatMoneyValue(value: number): string {
  return value.toLocaleString("zh-CN", { maximumFractionDigits: 2, minimumFractionDigits: 0 });
}

function ViewsBarPortal({
  bridge,
  mounts,
  state
}: FlowCanvasChromeProps): JSX.Element | null {
  const [dragViewId, setDragViewId] = useState("");
  const [dropHint, setDropHint] = useState<{ targetId: string; place: "before" | "after" } | null>(null);
  const showTabs = state.views.length > 0;

  useEffect(() => {
    const parent = mounts.views?.parentElement;
    if (!(parent instanceof HTMLElement)) {
      return;
    }
    const documentRef = parent.ownerDocument;
    const mirroredTargets = [
      documentRef.getElementById("graphToolbar"),
      documentRef.getElementById("graphCard"),
      documentRef.getElementById("graphWrap"),
      parent.closest(".app")
    ].filter((target): target is HTMLElement => target instanceof HTMLElement);
    parent.dataset.viewsMode = showTabs ? "expanded" : "collapsed";
    parent.dataset.viewCount = String(state.views.length);
    mirroredTargets.forEach((target) => {
      target.dataset.viewsMode = showTabs ? "expanded" : "collapsed";
      target.dataset.viewCount = String(state.views.length);
    });
    return () => {
      delete parent.dataset.viewsMode;
      delete parent.dataset.viewCount;
      mirroredTargets.forEach((target) => {
        delete target.dataset.viewsMode;
        delete target.dataset.viewCount;
      });
    };
  }, [mounts.views, showTabs, state.views.length]);

  if (!mounts.views) {
    return null;
  }

  const collapsedMount = mounts.emptyView ?? mounts.views;

  const onDropOnTab = (event: DragEvent<HTMLDivElement>, targetId: string): void => {
    event.preventDefault();
    const fromId = dragViewId || event.dataTransfer.getData("text/plain") || "";
    if (!fromId || fromId === targetId) {
      setDropHint(null);
      return;
    }
    const rect = event.currentTarget.getBoundingClientRect();
    const place = event.clientX < rect.left + rect.width / 2 ? "before" : "after";
    void bridge.commands.reorderViews(fromId, targetId, place);
    setDropHint(null);
    setDragViewId("");
  };

  const content = showTabs ? (
    <div className="analytix-react-views-bar is-expanded">
      <div className="viewsScroll" style={{ flex: "1 1 auto" }} role="tablist" aria-label="图谱页面">
        {state.views.map((view) => (
          <div
            key={view.id}
            className={[
              "viewTab",
              view.active ? "active" : "",
              dragViewId === view.id ? "dragging" : "",
              dropHint?.targetId === view.id && dropHint.place === "before" ? "dragOverLeft" : "",
              dropHint?.targetId === view.id && dropHint.place === "after" ? "dragOverRight" : ""
            ]
              .filter(Boolean)
              .join(" ")}
            role="tab"
            aria-selected={view.active}
            tabIndex={0}
            draggable
            onClick={() => bridge.commands.activateView(view.id)}
            onContextMenu={(event) => {
              event.preventDefault();
              bridge.commands.openViewContextMenu(view.id, event.clientX, event.clientY);
            }}
            onDoubleClick={() => void bridge.commands.renameView(view.id)}
            onKeyDown={(event) => {
              if (event.key === "Enter" || event.key === " ") {
                event.preventDefault();
                bridge.commands.activateView(view.id);
              }
            }}
            onDragStart={(event) => {
              setDragViewId(view.id);
              event.dataTransfer.effectAllowed = "move";
              event.dataTransfer.setData("text/plain", view.id);
            }}
            onDragEnd={() => {
              setDragViewId("");
              setDropHint(null);
            }}
            onDragOver={(event) => {
              event.preventDefault();
              if (!dragViewId || dragViewId === view.id) {
                return;
              }
              const rect = event.currentTarget.getBoundingClientRect();
              const place = event.clientX < rect.left + rect.width / 2 ? "before" : "after";
              setDropHint({ targetId: view.id, place });
            }}
            onDragLeave={() => {
              if (dropHint?.targetId === view.id) {
                setDropHint(null);
              }
            }}
            onDrop={(event) => onDropOnTab(event, view.id)}
          >
            <div className="viewTitle">{displayViewTitle(view.title)}</div>
            <button
              type="button"
              className="viewClose"
              aria-label="关闭标签页"
              onMouseDown={(event) => {
                event.stopPropagation();
                event.preventDefault();
              }}
              onClick={(event) => {
                event.stopPropagation();
                void bridge.commands.closeView(view.id);
              }}
            >
              <CloseIcon />
            </button>
          </div>
        ))}
      </div>
      <button className="viewAdd" type="button" aria-label="新建视图" onClick={() => bridge.commands.addView()}>
        <PlusIcon />
      </button>
    </div>
  ) : (
    <div className="analytix-react-views-bar is-collapsed">
      <button className="viewAdd viewAddSolo" type="button" aria-label="新建视图" onClick={() => bridge.commands.addView()}>
        <span className="viewAddSoloIcon" aria-hidden="true">
          <PlusIcon />
        </span>
        <span className="viewAddSoloText">新建页面</span>
      </button>
    </div>
  );

  return createPortal(content, showTabs ? mounts.views : collapsedMount);
}

function GraphSearchPortal({
  bridge,
  mounts,
  state
}: FlowCanvasChromeProps): JSX.Element | null {
  const inputRef = useRef<HTMLInputElement | null>(null);
  const lastFocusTokenRef = useRef<number>(state.graphSearch.focusToken);
  const canExpandProjection = !!state.projection?.canExpand;
  const canExpandSelectionPath = canExpandProjection && Number(state.projection?.selectedNodeCount || 0) >= 2;

  useEffect(() => {
    const nextToken = Number(state.graphSearch.focusToken) || 0;
    if (nextToken === lastFocusTokenRef.current) {
      return;
    }
    lastFocusTokenRef.current = nextToken;
    window.setTimeout(() => {
      inputRef.current?.focus();
      inputRef.current?.select();
    }, 0);
  }, [state.graphSearch.focusToken]);

  if (!mounts.graphSearch) {
    return null;
  }

  return createPortal(
    <div
      className={`analytix-react-graph-search${state.graphSearch.query.trim() ? " has-value" : ""}`}
      onMouseDown={stopEventPropagation}
      onClick={stopEventPropagation}
    >
      <input
        ref={inputRef}
        className="input analytix-react-graph-search-input"
        placeholder="定位：卡号/账号/户名"
        autoComplete="off"
        value={state.graphSearch.query}
        onChange={(event) => bridge.commands.graph.setSearch(event.target.value)}
        onKeyDown={(event: KeyboardEvent<HTMLInputElement>) => {
          if (event.key === "Enter") {
            event.preventDefault();
            bridge.commands.graph.runSearch();
          }
        }}
      />
      <button
        className="iconBtn sm analytix-react-graph-search-action"
        title="展开当前视口"
        type="button"
        aria-label="展开当前视口"
        onClick={() => {
          bridge.commands.graph.expandViewport();
        }}
        style={{ display: canExpandProjection ? undefined : "none" }}
      >
        <ViewportExpandIcon />
      </button>
      <button
        className="iconBtn sm analytix-react-graph-search-action"
        title={canExpandSelectionPath ? "展开选中路径" : "至少选择两个节点后可展开路径"}
        type="button"
        aria-label="展开选中路径"
        disabled={!canExpandSelectionPath}
        onClick={() => {
          bridge.commands.graph.expandSelectionPath();
        }}
        style={{ display: canExpandProjection ? undefined : "none" }}
      >
        <PathExpandIcon />
      </button>
      <button
        className="iconBtn sm analytix-react-graph-search-clear"
        title="清空"
        type="button"
        aria-label="清空搜索"
        onClick={() => {
          bridge.commands.graph.clearSearch();
          inputRef.current?.focus();
        }}
      >
        <CloseIcon />
      </button>
    </div>,
    mounts.graphSearch
  );
}

function GraphStatsPortal({ mounts, state }: FlowCanvasChromeProps): JSX.Element | null {
  if (!mounts.graphStats) {
    return null;
  }

  const summary = state.graphStats;
  if (summary.status !== "verified") {
    return createPortal(
      <div
        className={`analytix-react-graph-stats is-${summary.status}`}
        aria-live="polite"
        data-flow-summary-status={summary.status}
      >
        <div className="graphStatsItem graphStatsItem-boundary">
          <span className="graphStatsLabel">数据状态</span>
          <span className="graphStatsValue">{summary.boundaryText}</span>
        </div>
      </div>,
      mounts.graphStats
    );
  }

  return createPortal(
    <div className="analytix-react-graph-stats is-verified" aria-live="polite" data-flow-summary-status="verified">
      <div className="graphStatsItem">
        <span className="graphStatsLabel">节点</span>
        <span className="graphStatsValue">{summary.nodes}</span>
      </div>
      <div className="graphStatsItem">
        <span className="graphStatsLabel">线条</span>
        <span className="graphStatsValue">{summary.edges}</span>
      </div>
      <div className="graphStatsItem graphStatsItem-amount">
        <span className="graphStatsLabel">交易金额</span>
        <span className="graphStatsValue">{`￥${formatMoneyValue(summary.amount)}`}</span>
      </div>
    </div>,
    mounts.graphStats
  );
}

export function FlowCanvasChrome({
  bridge,
  mounts,
  state
}: FlowCanvasChromeProps): JSX.Element {
  return (
    <>
      <ViewsBarPortal bridge={bridge} mounts={mounts} state={state} />
      <GraphSearchPortal bridge={bridge} mounts={mounts} state={state} />
      <GraphStatsPortal bridge={bridge} mounts={mounts} state={state} />
    </>
  );
}
