import { useEffect, useState } from "react";
import type { CSSProperties } from "react";
import {
  FlowShellBridge,
  FlowShellContextMenuItem,
  FlowShellMounts,
  FlowShellState,
  FlowShellStylePopoverOption
} from "../runtime/flow-runtime";
import { FloatingPanel } from "./FlowOverlayPrimitives";

interface FlowToolbarOverlaysProps {
  bridge: FlowShellBridge;
  mounts: FlowShellMounts;
  state: FlowShellState;
}

function renderSymbolIcon(id: string): JSX.Element {
  return (
    <svg className="ico" aria-hidden="true">
      <use href={`#${id}`} />
    </svg>
  );
}

function buildSwatchStyle(color: string): CSSProperties {
  return { ["--swatch" as string]: color };
}

function normalizeColorInputValue(color: string): string {
  const raw = String(color || "").trim();
  const hex = raw.match(/^#([0-9a-f]{3}|[0-9a-f]{6})$/i);
  if (hex) {
    if (hex[1].length === 3) {
      return `#${hex[1]
        .split("")
        .map((part) => `${part}${part}`)
        .join("")}`.toLowerCase();
    }
    return raw.toLowerCase();
  }
  const rgba = raw.match(/^rgba?\(([^)]+)\)$/i);
  if (rgba) {
    const parts = rgba[1]
      .split(",")
      .slice(0, 3)
      .map((part) => Math.max(0, Math.min(255, Math.round(Number(part.trim()) || 0))));
    if (parts.length === 3) {
      return `#${parts.map((part) => part.toString(16).padStart(2, "0")).join("")}`;
    }
  }
  return "#1f2937";
}

function renderContextMenuItems(
  bridge: FlowShellBridge,
  items: FlowShellContextMenuItem[]
): JSX.Element[] {
  return items.map((item) => {
    if (item.separator) {
      return <div key={item.id} className="analytix-react-context-sep" aria-hidden="true" />;
    }
    return (
      <button
        key={item.id}
        type="button"
        className="analytix-react-context-item"
        disabled={item.disabled}
        onClick={() => {
          void bridge.commands.overlay.runContextMenuAction(item.id);
        }}
      >
        {item.label}
      </button>
    );
  });
}

function buildRegularPolygonPath(sides: number, radius: number, cx = 24, cy = 6, rotation = -Math.PI / 2): string {
  const points: Array<[number, number]> = [];
  for (let index = 0; index < sides; index += 1) {
    const angle = rotation + (Math.PI * 2 * index) / sides;
    points.push([cx + Math.cos(angle) * radius, cy + Math.sin(angle) * radius]);
  }
  return points
    .map(([x, y], index) => `${index === 0 ? "M" : "L"}${x.toFixed(2)} ${y.toFixed(2)}`)
    .join(" ")
    .concat(" Z");
}

function buildStarPath(cx = 24, cy = 6, outer = 5, inner = 2.4, rotation = -Math.PI / 2): string {
  const points: Array<[number, number]> = [];
  for (let index = 0; index < 10; index += 1) {
    const angle = rotation + (Math.PI * index) / 5;
    const radius = index % 2 === 0 ? outer : inner;
    points.push([cx + Math.cos(angle) * radius, cy + Math.sin(angle) * radius]);
  }
  return points
    .map(([x, y], index) => `${index === 0 ? "M" : "L"}${x.toFixed(2)} ${y.toFixed(2)}`)
    .join(" ")
    .concat(" Z");
}

function renderStylePopoverGraphic(
  kind: FlowShellState["overlay"]["stylePopover"]["kind"],
  option: FlowShellStylePopoverOption
): JSX.Element | null {
  if (kind === "lineStyle") {
    return (
      <svg viewBox="0 0 56 20" className="popoverIcon popoverIcon--line" aria-hidden="true">
        <line
          x1="4"
          y1="10"
          x2="52"
          y2="10"
          stroke="currentColor"
          strokeWidth="2.25"
          strokeLinecap="round"
          strokeDasharray={Array.isArray(option.dash) && option.dash.length ? option.dash.join(" ") : undefined}
        />
      </svg>
    );
  }

  if (kind === "arrow") {
    const value = option.value;
    return (
      <svg viewBox="0 0 56 20" className="popoverIcon popoverIcon--arrow" aria-hidden="true">
        <line x1="7" y1="10" x2="49" y2="10" stroke="currentColor" strokeWidth="2.25" strokeLinecap="round" />
        {value === "start" || value === "both" ? <path d="M 7 10 L 15 5 L 15 15 Z" fill="currentColor" /> : null}
        {value === "end" || value === "both" ? <path d="M 49 10 L 41 5 L 41 15 Z" fill="currentColor" /> : null}
      </svg>
    );
  }

  if (kind === "shape") {
    const value = option.value;
    return (
      <svg
        viewBox="0 0 24 24"
        className="popoverIcon popoverIcon--shape"
        aria-hidden="true"
        fill="none"
        stroke="currentColor"
        strokeWidth="2.25"
        strokeLinecap="round"
        strokeLinejoin="round"
      >
        {value === "circle" ? <circle cx="12" cy="12" r="7" /> : null}
        {value === "rect" ? <rect x="5" y="5.5" width="14" height="13" rx="1.75" /> : null}
        {value === "roundrect" ? <rect x="4.5" y="5" width="15" height="14" rx="4.75" /> : null}
        {value === "diamond" ? <path d="M12 4 L19.5 12 L12 20 L4.5 12 Z" /> : null}
        {value === "pill" ? <rect x="3.5" y="7" width="17" height="10" rx="5" /> : null}
        {value === "triangle" ? <path d={buildRegularPolygonPath(3, 8, 12, 12)} /> : null}
        {value === "pentagon" ? <path d={buildRegularPolygonPath(5, 7.6, 12, 12)} /> : null}
        {value === "hexagon" ? <path d={buildRegularPolygonPath(6, 7.6, 12, 12)} /> : null}
        {value === "octagon" ? <path d={buildRegularPolygonPath(8, 7.4, 12, 12)} /> : null}
        {value === "star" ? <path d={buildStarPath(12, 12, 7.8, 3.6)} /> : null}
      </svg>
    );
  }

  return null;
}

function FilterPopoverPortal({
  bridge,
  mounts,
  state
}: FlowToolbarOverlaysProps): JSX.Element | null {
  const [minText, setMinText] = useState("");
  const [maxText, setMaxText] = useState("");

  useEffect(() => {
    setMinText(state.analysis.filterMin == null ? "" : String(state.analysis.filterMin));
  }, [state.analysis.filterMin]);

  useEffect(() => {
    setMaxText(state.analysis.filterMax == null ? "" : String(state.analysis.filterMax));
  }, [state.analysis.filterMax]);

  if (!state.overlay.filter.open || !state.overlay.filter.anchorRect) {
    return null;
  }

  return (
    <FloatingPanel
      mount={mounts.overlay}
      open={state.overlay.filter.open}
      anchorRect={state.overlay.filter.anchorRect}
      anchorAlign="center"
      className="analytix-react-overlay-panel"
    >
      <div className="amountFilterPanel">
        <div className="amountFilterTitle">金额设定</div>
        <div className="amountFilterGrid">
          <div className="amountFilterField">
            <label htmlFor="analytix-react-filter-min">最小金额</label>
            <input
              id="analytix-react-filter-min"
              className="amountFilterInput"
              type="text"
              inputMode="decimal"
              placeholder="如 10000"
              value={minText}
              onChange={(event) => {
                const next = event.target.value;
                setMinText(next);
                bridge.commands.analysis.setFilterMin(next);
              }}
            />
          </div>
          <div className="amountFilterField">
            <label htmlFor="analytix-react-filter-max">最大金额</label>
            <input
              id="analytix-react-filter-max"
              className="amountFilterInput"
              type="text"
              inputMode="decimal"
              placeholder="如 100000"
              value={maxText}
              onChange={(event) => {
                const next = event.target.value;
                setMaxText(next);
                bridge.commands.analysis.setFilterMax(next);
              }}
            />
          </div>
        </div>
        <div className="amountFilterHint">只显示范围内金额。</div>
        <div className="amountFilterActions">
          <button
            className="amountFilterClear"
            type="button"
            title="清空金额范围"
            onClick={() => {
              setMinText("");
              setMaxText("");
              bridge.commands.analysis.clearFilter();
            }}
          >
            清空范围
          </button>
        </div>
      </div>
    </FloatingPanel>
  );
}

function ExportPopoverPortal({
  bridge,
  mounts,
  state
}: FlowToolbarOverlaysProps): JSX.Element | null {
  if (!state.overlay.exportPanel.open || !state.overlay.exportPanel.anchorRect) {
    return null;
  }

  const scale = state.overlay.exportPanel.scale || 1;

  return (
    <FloatingPanel
      mount={mounts.overlay}
      open={state.overlay.exportPanel.open}
      anchorRect={state.overlay.exportPanel.anchorRect}
      anchorAlign="center"
      className="analytix-react-overlay-panel"
    >
      <div className="popoverTitle">导出格式</div>
      <div className="exportGrid">
        {(["png", "jpg", "pdf"] as const).map((format) => (
          <button
            key={format}
            className="popoverItem"
            type="button"
            onClick={() => bridge.commands.ops.exportFormat(format)}
          >
            {format.toUpperCase()}
          </button>
        ))}
      </div>
      <div className="exportScale">
        {[1, 2, 4].map((value) => (
          <button
            key={value}
            type="button"
            className={scale === value ? "active" : ""}
            onClick={() => bridge.commands.ops.setExportScale(value)}
          >
            {`${value}x`}
          </button>
        ))}
      </div>
      <div className="analytix-react-export-hint">导出保持当前画布与四套布局核心不变，仅由 React 面板接管格式与倍数选择。</div>
    </FloatingPanel>
  );
}

function ContextMenuPortal({
  bridge,
  mounts,
  state
}: FlowToolbarOverlaysProps): JSX.Element | null {
  if (!state.overlay.contextMenu.open || !state.overlay.contextMenu.items.length) {
    return null;
  }

  return (
    <FloatingPanel
      mount={mounts.overlay}
      open={state.overlay.contextMenu.open}
      point={{ x: state.overlay.contextMenu.x, y: state.overlay.contextMenu.y }}
      className="analytix-react-overlay-panel analytix-react-context-menu"
    >
      {renderContextMenuItems(bridge, state.overlay.contextMenu.items)}
    </FloatingPanel>
  );
}

function ToolbarMenuPortal({
  bridge,
  mounts,
  state
}: FlowToolbarOverlaysProps): JSX.Element | null {
  if (!state.overlay.menu.open || !state.overlay.menu.anchorRect || !state.overlay.menu.items.length) {
    return null;
  }

  return (
    <FloatingPanel
      mount={mounts.overlay}
      open={state.overlay.menu.open}
      anchorRect={state.overlay.menu.anchorRect}
      anchorAlign="center"
      className="analytix-react-overlay-panel analytix-react-toolbar-menu"
    >
      {renderContextMenuItems(bridge, state.overlay.menu.items)}
    </FloatingPanel>
  );
}

function StylePopoverPortal({
  bridge,
  mounts,
  state
}: FlowToolbarOverlaysProps): JSX.Element | null {
  const stylePopover = state.overlay.stylePopover;
  if (!stylePopover.open || !stylePopover.anchorRect || !stylePopover.kind) {
    return null;
  }

  const isActiveOption = (optionValue: string): boolean => String(optionValue) === String(stylePopover.currentValue || "");
  const renderOptions = (): JSX.Element => {
    if (stylePopover.kind === "color") {
      const recentColors = stylePopover.recentColors.slice(0, 8);
      while (recentColors.length < 8) {
        recentColors.push("");
      }
      return (
        <>
          <div className="popoverTitle">常用色</div>
          <div className="colorGrid">
            {stylePopover.commonColors.map((color) => (
              <button
                key={color}
                className="colorCell"
                type="button"
                style={buildSwatchStyle(color)}
                onClick={() => bridge.commands.style.applyPopoverColor(color, true)}
              />
            ))}
          </div>
          <div className="popoverTitle">最近</div>
          <div className="colorGrid">
            {recentColors.map((color, index) =>
              color ? (
                <button
                  key={`${color}-${index}`}
                  className="colorCell"
                  type="button"
                  style={buildSwatchStyle(color)}
                  onClick={() => bridge.commands.style.applyPopoverColor(color, true)}
                />
              ) : (
                <div key={`empty-${index}`} className="colorCell empty" aria-hidden="true" />
              )
            )}
          </div>
          <div className="colorCustomRow">
            <input
              type="color"
              aria-label="自定义颜色"
              value={normalizeColorInputValue(stylePopover.currentValue)}
              onChange={(event) => bridge.commands.style.applyPopoverColor(event.target.value, false)}
            />
            <div style={{ fontSize: "12px", color: "rgba(2,6,23,.7)" }}>自定义</div>
          </div>
        </>
      );
    }

    if (stylePopover.kind === "iconLibrary") {
      return (
        <div className="iconPopover">
          <div className="iconTabs">
            {stylePopover.iconTabs.map((tab) => (
              <button
                key={tab.id}
                type="button"
                className={`iconTab ${stylePopover.activeTab === tab.id ? "active" : ""}`}
                onClick={() => bridge.commands.style.setIconLibraryTab(tab.id)}
              >
                {tab.label}
              </button>
            ))}
          </div>
          <div className="iconSearch">
            <input
              type="text"
              value={stylePopover.iconQuery}
              placeholder="搜索图标"
              onChange={(event) => bridge.commands.style.setIconLibraryQuery(event.target.value)}
            />
          </div>
          <div className="iconGrid">
            {stylePopover.icons.map((iconId) => (
              <button
                key={iconId}
                type="button"
                className={`iconBtn ${isActiveOption(iconId) ? "active" : ""}`}
                onClick={() => bridge.commands.style.applyIconSymbol(iconId)}
              >
                {renderSymbolIcon(iconId)}
              </button>
            ))}
          </div>
        </div>
      );
    }

    return (
      <div className="popoverList">
        {stylePopover.options.map((option) => (
          <button
            key={option.value}
            className={`popoverItem${stylePopover.kind === "list" ? "" : " withIcon"} ${isActiveOption(option.value) ? "active" : ""}`.trim()}
            type="button"
            onClick={() => bridge.commands.style.applyPopoverOption(option.value)}
          >
            {renderStylePopoverGraphic(stylePopover.kind, option)}
            <span className="popoverLabel">{option.label}</span>
          </button>
        ))}
      </div>
    );
  };

  return (
    <FloatingPanel
      mount={mounts.overlay}
      open={stylePopover.open}
      anchorRect={stylePopover.anchorRect}
      anchorAlign="center"
      className="analytix-react-overlay-panel"
    >
      {renderOptions()}
    </FloatingPanel>
  );
}

export function FlowToolbarOverlays({
  bridge,
  mounts,
  state
}: FlowToolbarOverlaysProps): JSX.Element {
  return (
    <>
      <FilterPopoverPortal bridge={bridge} mounts={mounts} state={state} />
      <StylePopoverPortal bridge={bridge} mounts={mounts} state={state} />
      <ToolbarMenuPortal bridge={bridge} mounts={mounts} state={state} />
      <ExportPopoverPortal bridge={bridge} mounts={mounts} state={state} />
      <ContextMenuPortal bridge={bridge} mounts={mounts} state={state} />
    </>
  );
}
