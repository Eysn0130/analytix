import type { ChartFilterToken } from "../api/stats-api";

export const SANKEY_CHART_WIDTH = 860;
export const SANKEY_CHART_HEIGHT = 372;
export const SANKEY_CHART_MIN_WIDTH = 640;
export const SANKEY_CHART_MIN_HEIGHT = 300;

export interface SankeyChartItem {
  label: string;
  amount: number;
  count: number;
  firstTime: string;
  lastTime: string;
}

export interface SankeyLayoutRow {
  item: SankeyChartItem;
  token: ChartFilterToken;
  centerY: number;
  nodeY: number;
  flowWidth: number;
  ribbonPath: string;
  rankAlpha: number;
  hoverX: number;
}

export interface SankeyChartLayout {
  width: number;
  height: number;
  stackTop: number;
  stackHeight: number;
  nodeWidth: number;
  nodeHeight: number;
  labelTextX: number;
  leftNodeX: number;
  amountTextX: number;
  targetX: number;
  labelCharLimit: number;
  rows: SankeyLayoutRow[];
}

export interface BuildSankeyChartLayoutInput {
  rawItems: Array<Record<string, unknown>>;
  direction: "in" | "out";
  width: number;
  height: number;
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : {};
}

function toNonNegativeNumber(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) && value >= 0 ? value : null;
}

function toNonNegativeInteger(value: unknown): number | null {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0 ? value : null;
}

function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value));
}

function normalizeSankeyChartItem(item: Record<string, unknown>): SankeyChartItem | null {
  const label = typeof item.label === "string" ? item.label.trim() : "";
  const amount = toNonNegativeNumber(item.amount);
  const count = toNonNegativeInteger(item.count);
  if (!label || amount == null || count == null) {
    return null;
  }
  return {
    label,
    amount,
    count,
    firstTime: typeof item.first_time === "string" ? item.first_time.trim() : "",
    lastTime: typeof item.last_time === "string" ? item.last_time.trim() : "",
  };
}

export function normalizeSankeyChartItems(rawItems: Array<Record<string, unknown>>, limit = 10): SankeyChartItem[] {
  return (Array.isArray(rawItems) ? rawItems : [])
    .map((item) => normalizeSankeyChartItem(asRecord(item)))
    .filter((item): item is SankeyChartItem => item !== null)
    .sort((left, right) => right.amount - left.amount || left.label.localeCompare(right.label, "zh-CN"))
    .slice(0, limit);
}

export function truncateSankeyLabel(value: string, limit: number): string {
  const text = String(value || "").trim();
  if (!text || text.length <= limit) {
    return text;
  }
  return `${text.slice(0, Math.max(1, limit - 1))}…`;
}

export function buildSankeyFlowBandPath({
  startX,
  startY,
  endX,
  endY,
  width,
  shoulderX,
}: {
  startX: number;
  startY: number;
  endX: number;
  endY: number;
  width: number;
  shoulderX: number;
}): string {
  const half = width / 2;
  const startTop = startY - half;
  const startBottom = startY + half;
  const endTop = endY - half;
  const endBottom = endY + half;
  const direction = endX >= startX ? 1 : -1;
  const remaining = Math.max(24, Math.abs(endX - shoulderX));
  const midX = shoulderX + direction * remaining * 0.54;
  const curveOneX = shoulderX + direction * remaining * 0.14;
  const curveTwoX = shoulderX + direction * remaining * 0.34;
  const curveThreeX = shoulderX + direction * remaining * 0.68;
  const curveFourX = shoulderX + direction * remaining * 0.9;
  const verticalShift = endY - startY;
  const bendCarry = verticalShift * 0.12;
  const c1TopY = startTop + verticalShift * 0.03;
  const c2TopY = startTop + verticalShift * 0.22 + bendCarry;
  const midTopY = startTop + verticalShift * 0.56 + bendCarry;
  const c3TopY = startTop + verticalShift * 0.86 + bendCarry * 0.38;
  const c4TopY = endTop - verticalShift * 0.04;
  const c1BottomY = startBottom + verticalShift * 0.03;
  const c2BottomY = startBottom + verticalShift * 0.22 + bendCarry;
  const midBottomY = startBottom + verticalShift * 0.56 + bendCarry;
  const c3BottomY = startBottom + verticalShift * 0.86 + bendCarry * 0.38;
  const c4BottomY = endBottom - verticalShift * 0.04;
  return [
    `M${startX},${startTop}`,
    `L${shoulderX},${startTop}`,
    `C ${curveOneX},${c1TopY} ${curveTwoX},${c2TopY} ${midX},${midTopY}`,
    `C ${curveThreeX},${c3TopY} ${curveFourX},${c4TopY} ${endX},${endTop}`,
    `L${endX},${endBottom}`,
    `C ${curveFourX},${c4BottomY} ${curveThreeX},${c3BottomY} ${midX},${midBottomY}`,
    `C ${curveTwoX},${c2BottomY} ${curveOneX},${c1BottomY} ${shoulderX},${startBottom}`,
    `L${startX},${startBottom}`,
    "Z",
  ].join(" ");
}

export function buildSankeyChartLayout({
  rawItems,
  direction,
  width,
  height,
}: BuildSankeyChartLayoutInput): SankeyChartLayout {
  const items = normalizeSankeyChartItems(rawItems);
  const maxValue = Math.max(1, ...items.map((item) => item.amount));
  const sidePadding = clamp(width * 0.006, 6, 10);
  const rightGap = clamp(width * 0.03, 24, 36);
  const stackTop = clamp(height * 0.024, 8, 14);
  const stackBottom = height - clamp(height * 0.022, 8, 14);
  const stackHeight = Math.max(96, stackBottom - stackTop);
  const slotHeight = items.length ? stackHeight / items.length : stackHeight;
  const nodeWidth = clamp(width * 0.013, 11, 15);
  const nodeHeight = clamp(slotHeight * 0.66, 18, 24);
  const labelWidth = clamp(width * 0.165, 96, 132);
  const labelGap = clamp(width * 0.005, 5, 8);
  const amountOffset = clamp(width * 0.01, 8, 12);
  const labelTextX = sidePadding + labelWidth;
  const leftNodeX = labelTextX + labelGap;
  const lineStartX = leftNodeX + nodeWidth;
  const amountTextX = lineStartX + amountOffset;
  const targetX = width - rightGap - nodeWidth;
  const railX = targetX;
  const currentCenterY = stackTop + stackHeight / 2;
  const minFlowWidth = clamp(height * 0.0125, 4.4, 5.8);
  const flowSpread = clamp(height * 0.034, 11.5, 15.5);
  const flowWidths = items.map((item) => minFlowWidth + Math.pow(item.amount / maxValue, 0.58) * flowSpread);
  const targetGap = clamp(height * 0.008, 3, 5);
  const rawTargetDistances = flowWidths.map((widthValue, index) =>
    index === 0 ? 0 : (flowWidths[index - 1] + widthValue) / 2 + targetGap
  );
  const minTargetSpread = rawTargetDistances.reduce((sum, value) => sum + value, 0);
  const preferredTargetSpread = clamp(stackHeight * 0.255, stackHeight * 0.2, stackHeight * 0.32);
  const targetSpread = items.length > 1 ? Math.max(minTargetSpread, preferredTargetSpread) : 0;
  const targetScale = items.length > 1 && minTargetSpread > 0 ? targetSpread / minTargetSpread : 1;
  const targetPositions: number[] = [];
  let traveled = 0;
  for (let index = 0; index < items.length; index += 1) {
    if (items.length <= 1) {
      targetPositions.push(currentCenterY);
    } else if (index === 0) {
      targetPositions.push(currentCenterY - targetSpread / 2);
    } else {
      traveled += rawTargetDistances[index];
      targetPositions.push(currentCenterY - targetSpread / 2 + traveled * targetScale);
    }
  }
  const routeDistance = Math.max(120, railX - lineStartX);
  const shoulderX = lineStartX + clamp(routeDistance * 0.08, 14, 24);
  const labelCharLimit = width >= 980 ? 11 : width >= 860 ? 9 : 8;
  const rows = items.map((item, index): SankeyLayoutRow => {
    const centerY = stackTop + slotHeight * index + slotHeight / 2;
    const nodeY = centerY - nodeHeight / 2;
    const widthRatio = clamp(item.amount / maxValue, 0.02, 1);
    const flowWidth = flowWidths[index] || (minFlowWidth + Math.pow(widthRatio, 0.58) * flowSpread);
    const targetY = targetPositions[index] ?? currentCenterY;
    const ribbonPath = buildSankeyFlowBandPath({
      startX: lineStartX,
      startY: centerY,
      endX: railX,
      endY: targetY,
      width: flowWidth,
      shoulderX,
    });
    const token: ChartFilterToken = {
      source_panel_id: "flow",
      dimension: "flow_counterparty",
      value: item.label,
      label: item.label,
      payload: { side: direction },
    };
    const rankAlpha = index === 0 ? 0.72 : index < 3 ? 0.6 : index < 6 ? 0.48 : 0.38;
    const hoverX = lineStartX + (railX - lineStartX) * 0.74;
    return {
      item,
      token,
      centerY,
      nodeY,
      flowWidth,
      ribbonPath,
      rankAlpha,
      hoverX,
    };
  });

  return {
    width,
    height,
    stackTop,
    stackHeight,
    nodeWidth,
    nodeHeight,
    labelTextX,
    leftNodeX,
    amountTextX,
    targetX,
    labelCharLimit,
    rows,
  };
}
