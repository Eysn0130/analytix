const KEYWORD_CLOUD_COLORS = ["#9edbdc", "#8fd6de", "#81d0e0", "#72c7e4", "#63b9e8", "#4f74e6"];
export const KEYWORD_CLOUD_WIDTH = 420;
export const KEYWORD_CLOUD_HEIGHT = 264;
export const KEYWORD_CLOUD_MIN_WIDTH = 280;
export const KEYWORD_CLOUD_MIN_HEIGHT = 220;


function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value));
}

function hashText(value: string): number {
  let hash = 0;
  for (let index = 0; index < value.length; index += 1) {
    hash = (hash * 33 + value.charCodeAt(index)) >>> 0;
  }
  return hash;
}

export function estimateKeywordWidth(label: string, fontSize: number): number {
  const asciiLength = (label.match(/[A-Za-z0-9]/g) || []).length;
  const wideLength = Math.max(0, label.length - asciiLength);
  return Math.max(44, wideLength * fontSize * 0.9 + asciiLength * fontSize * 0.58);
}

let keywordCloudMeasureCanvas: HTMLCanvasElement | null = null;

export function measureKeywordSize(label: string, fontSize: number): { width: number; height: number } {
  const fallbackWidth = estimateKeywordWidth(label, fontSize);
  if (typeof document === "undefined") {
    return {
      width: fallbackWidth,
      height: fontSize * 0.96,
    };
  }
  keywordCloudMeasureCanvas = keywordCloudMeasureCanvas || document.createElement("canvas");
  const context = keywordCloudMeasureCanvas.getContext("2d");
  if (!context) {
    return {
      width: fallbackWidth,
      height: fontSize * 0.96,
    };
  }
  context.font = `700 ${fontSize}px "Avenir Next", "PingFang SC", "Microsoft YaHei", sans-serif`;
  const metrics = context.measureText(label);
  const width = Math.max(fallbackWidth * 0.86, metrics.width + 6);
  const height = Math.max(fontSize * 0.92, (metrics.actualBoundingBoxAscent || fontSize * 0.76) + (metrics.actualBoundingBoxDescent || fontSize * 0.18));
  return { width, height };
}

function mixHexColor(start: string, end: string, amount: number): string {
  const from = start.replace("#", "");
  const to = end.replace("#", "");
  const ratio = clamp(amount, 0, 1);
  const read = (hex: string, offset: number): number => parseInt(hex.slice(offset, offset + 2), 16);
  const channel = (left: number, right: number): string =>
    Math.round(left + (right - left) * ratio)
      .toString(16)
      .padStart(2, "0");
  return `#${channel(read(from, 0), read(to, 0))}${channel(read(from, 2), read(to, 2))}${channel(read(from, 4), read(to, 4))}`;
}

function resolveKeywordCloudColor(normalized: number, rank: number, tier: KeywordCloudTier): string {
  const capped = clamp(normalized, 0, 1);
  if (rank === 0) {
    return "#4564dc";
  }
  if (rank === 1) {
    return "#5f84e8";
  }
  if (tier === "center-ring") {
    return mixHexColor("#72c8e4", "#5f86e7", 0.24 + capped * 0.58);
  }
  if (tier === "middle-ring") {
    return mixHexColor("#85d2df", "#69b5e8", 0.18 + capped * 0.42);
  }
  if (tier === "outer-ring") {
    return mixHexColor("#9edcdc", "#84d1df", 0.16 + capped * 0.2);
  }
  const paletteIndex = Math.round(capped * (KEYWORD_CLOUD_COLORS.length - 1));
  return KEYWORD_CLOUD_COLORS[clamp(paletteIndex, 0, KEYWORD_CLOUD_COLORS.length - 1)];
}

function resolveKeywordCloudHoverColor(normalized: number, rank: number, tier: KeywordCloudTier): string {
  const base = resolveKeywordCloudColor(normalized, rank, tier);
  if (tier === "outer-ring") {
    return mixHexColor(base, "#58ace6", 0.42);
  }
  if (tier === "middle-ring") {
    return mixHexColor(base, "#5d8ce6", 0.34);
  }
  return mixHexColor(base, "#4362dd", 0.22);
}

export type KeywordCloudTier = "hero" | "hero-secondary" | "center-ring" | "middle-ring" | "outer-ring";

export interface KeywordCloudLayoutWord {
  label: string;
  count: number;
  x: number;
  y: number;
  fontSize: number;
  opacity: number;
  color: string;
  hoverColor: string;
  hoverScale: number;
  tier: KeywordCloudTier;
}

export interface KeywordCloudRect {
  left: number;
  top: number;
  right: number;
  bottom: number;
}

interface KeywordCloudPlacedRect extends KeywordCloudRect {
  x: number;
  y: number;
  tier: KeywordCloudTier;
}

interface KeywordCloudAnchor {
  x: number;
  y: number;
  angle: number;
  targetSpread: number;
}

export function getKeywordCloudLayoutBounds(layout: KeywordCloudLayoutWord[]): KeywordCloudRect {
  if (!layout.length) {
    return {
      left: 0,
      top: 0,
      right: KEYWORD_CLOUD_WIDTH,
      bottom: KEYWORD_CLOUD_HEIGHT,
    };
  }
  return layout.reduce<KeywordCloudRect>(
    (bounds, item) => {
      const size = measureKeywordSize(item.label, item.fontSize);
      return {
        left: Math.min(bounds.left, item.x - size.width / 2),
        top: Math.min(bounds.top, item.y - size.height / 2),
        right: Math.max(bounds.right, item.x + size.width / 2),
        bottom: Math.max(bounds.bottom, item.y + size.height / 2),
      };
    },
    {
      left: Number.POSITIVE_INFINITY,
      top: Number.POSITIVE_INFINITY,
      right: Number.NEGATIVE_INFINITY,
      bottom: Number.NEGATIVE_INFINITY,
    }
  );
}

function pointOnRectBand(centerX: number, centerY: number, radiusX: number, radiusY: number, progress: number): { x: number; y: number; angle: number; spread: number } {
  const clampedRadiusX = Math.max(18, radiusX);
  const clampedRadiusY = Math.max(14, radiusY);
  const bandWidth = clampedRadiusX * 2;
  const bandHeight = clampedRadiusY * 2;
  const perimeter = bandWidth * 2 + bandHeight * 2;
  let cursor = ((((progress % 1) + 1) % 1) * perimeter) || 0;
  const left = centerX - clampedRadiusX;
  const right = centerX + clampedRadiusX;
  const top = centerY - clampedRadiusY;
  const bottom = centerY + clampedRadiusY;
  const spread = Math.max(clampedRadiusX / Math.max(radiusX, 1), clampedRadiusY / Math.max(radiusY, 1));
  if (cursor <= bandWidth) {
    return { x: left + cursor, y: top, angle: -Math.PI / 2, spread };
  }
  cursor -= bandWidth;
  if (cursor <= bandHeight) {
    return { x: right, y: top + cursor, angle: 0, spread };
  }
  cursor -= bandHeight;
  if (cursor <= bandWidth) {
    return { x: right - cursor, y: bottom, angle: Math.PI / 2, spread };
  }
  cursor -= bandWidth;
  return { x: left, y: bottom - cursor, angle: Math.PI, spread };
}

export function buildKeywordCloudLayout(items: Array<{ label: string; count: number }>, width: number, height: number): KeywordCloudLayoutWord[] {
  if (!items.length) {
    return [];
  }
  const safeWidth = Math.max(width, KEYWORD_CLOUD_MIN_WIDTH);
  const safeHeight = Math.max(height, KEYWORD_CLOUD_MIN_HEIGHT);
  const boundaryWidth = Math.max(1, safeWidth - 24);
  const boundaryHeight = Math.max(1, safeHeight - 24);
  const densityRatio = Math.max(1, items.length / 42);
  const maxCount = Math.max(1, ...items.map((item) => item.count));
  const minCount = Math.min(...items.map((item) => item.count), maxCount);
  const centerX = safeWidth / 2;
  const centerY = safeHeight / 2;
  const logCountSpan = Math.max(0.0001, Math.log(maxCount + 1) - Math.log(minCount + 1));
  const paddingX = clamp(Math.round(safeWidth * (items.length > 160 ? 0.024 : 0.038)), 4, 16);
  const paddingY = clamp(Math.round(safeHeight * (items.length > 160 ? 0.03 : 0.046)), 5, 14);
  const boundary = {
    left: paddingX,
    top: paddingY,
    right: safeWidth - paddingX,
    bottom: safeHeight - paddingY,
  };
  const maxRadiusX = Math.max(48, safeWidth / 2 - paddingX - 6);
  const maxRadiusY = Math.max(38, safeHeight / 2 - paddingY - 6);
  const radiusBase = Math.min(maxRadiusX, maxRadiusY);
  const heroFontSize = clamp(Math.min(safeWidth * 0.156, safeHeight * 0.132), 46, 76);
  const secondaryHeroFontSize = clamp(heroFontSize * 0.78, 38, 60);
  const centerRingFontSize = clamp(heroFontSize * 0.48, 25, 40);
  const middleRingMaxFont = clamp(heroFontSize * 0.34, 19, 30);
  const outerRingMaxFont = clamp(heroFontSize * 0.23, 11, 19);
  const minFontSize = clamp(Math.min(safeWidth * 0.018, safeHeight * 0.025) / Math.pow(densityRatio, 0.24), 5, 9);
  const minimumReadableFont = items.length > 220 ? 6 : items.length > 140 ? 7 : 8;
  const baseWords = items.map((item, index) => {
    const normalized =
      maxCount === minCount ? 1 : (Math.log(item.count + 1) - Math.log(minCount + 1)) / logCountSpan;
    const tier: KeywordCloudTier =
      index === 0
        ? "hero"
        : index === 1
          ? "hero-secondary"
          : index < 8
            ? "center-ring"
            : index < 28
              ? "middle-ring"
              : "outer-ring";
    const emphasis = clamp(
      (index === 0 ? 1 : index === 1 ? 0.94 : tier === "center-ring" ? 0.24 : tier === "middle-ring" ? 0.14 : 0.08) +
        Math.pow(normalized, tier === "outer-ring" ? 1.08 : 0.98) * 0.8 -
        index * 0.0044,
      0.06,
      1
    );
    let baseFontSize = minFontSize;
    if (tier === "hero") {
      baseFontSize = heroFontSize;
    } else if (tier === "hero-secondary") {
      baseFontSize = secondaryHeroFontSize;
    } else if (tier === "center-ring") {
      const tierIndex = index - 2;
      const decay = 1 - tierIndex * 0.058;
      baseFontSize = clamp(centerRingFontSize * decay * (0.84 + normalized * 0.24), 24, centerRingFontSize * 1.02);
    } else if (tier === "middle-ring") {
      const tierIndex = index - 8;
      const middleFloor = clamp(middleRingMaxFont * 0.56, 12, 16);
      baseFontSize = clamp(
        middleFloor + Math.pow(normalized, 0.92) * (middleRingMaxFont - middleFloor) * (tierIndex < 8 ? 1.04 : 0.96),
        12,
        middleRingMaxFont
      );
    } else {
      const decay = index > 120 ? 0.84 : index > 72 ? 0.9 : 0.96;
      baseFontSize = clamp(
        minFontSize + Math.pow(normalized, 0.94) * (outerRingMaxFont - minFontSize) * decay,
        minFontSize,
        outerRingMaxFont
      );
    }
    const size = measureKeywordSize(item.label, baseFontSize);
    return {
      ...item,
      index,
      normalized,
      emphasis,
      baseFontSize,
      baseWidth: size.width,
      baseHeight: size.height,
      seed: hashText(item.label),
      tier,
    };
  });
  const heroArea = baseWords
    .filter((item) => item.tier === "hero" || item.tier === "hero-secondary")
    .reduce((sum, item) => sum + item.baseWidth * item.baseHeight, 0);
  const centerArea = baseWords
    .filter((item) => item.tier === "center-ring" || item.tier === "middle-ring")
    .reduce((sum, item) => sum + item.baseWidth * item.baseHeight, 0);
  const outerArea = baseWords.filter((item) => item.tier === "outer-ring").reduce((sum, item) => sum + item.baseWidth * item.baseHeight, 0);
  const availableArea = boundaryWidth * boundaryHeight;
  const heroBudget = availableArea * (items.length > 120 ? 0.2 : 0.22);
  const centerBudget = availableArea * (items.length > 120 ? 0.29 : 0.31);
  const outerBudget = availableArea * (items.length > 160 ? 0.24 : items.length > 100 ? 0.26 : 0.28);
  const heroScale = clamp(Math.sqrt(heroBudget / Math.max(heroArea, 1)), 0.94, 1.02);
  const centerScale = clamp(Math.sqrt(centerBudget / Math.max(centerArea, 1)), 0.75, 0.97);
  const outerScale = clamp(Math.sqrt(outerBudget / Math.max(outerArea, 1)), 0.22, 0.74);
  const placed: KeywordCloudPlacedRect[] = [];
  const gridColumns = clamp(Math.round(safeWidth / 76), 5, 8);
  const gridRows = clamp(Math.round(safeHeight / 54), 4, 6);
  const densityGrid = Array.from({ length: gridRows }, () => Array.from({ length: gridColumns }, () => 0));
  const centerRingAngles = [-164, -126, -34, 28, 122, 162].map((degree) => (degree * Math.PI) / 180);
  const middleSlotsPerBand = [10, 14];
  const outerSlotsPerBand = items.length > 180 ? [26, 32, 38, 44] : items.length > 120 ? [24, 30, 36, 42] : [20, 26, 32, 38];
  const bandJitter = (seed: number, amount: number): number => (((seed % 1000) / 999) * 2 - 1) * amount;

  const resolveAnchor = (index: number, seed: number): KeywordCloudAnchor => {
    if (index === 0) {
      return {
        x: centerX,
        y: centerY - heroFontSize * 0.08,
        angle: -Math.PI / 2,
        targetSpread: 0.1,
      };
    }
    if (index === 1) {
      return {
        x: centerX,
        y: centerY + secondaryHeroFontSize * 0.82,
        angle: Math.PI / 2,
        targetSpread: 0.26,
      };
    }
    if (index < 8) {
      const angle = centerRingAngles[index - 2];
      return {
        x: centerX + Math.cos(angle) * maxRadiusX * 0.3,
        y: centerY + Math.sin(angle) * maxRadiusY * 0.24,
        angle,
        targetSpread: 0.34,
      };
    }
    if (index < 28) {
      const middleIndex = index - 8;
      const band = middleIndex < middleSlotsPerBand[0] ? 0 : 1;
      const slot = band === 0 ? middleIndex : middleIndex - middleSlotsPerBand[0];
      const slots = middleSlotsPerBand[band];
      const progress = (slot + 0.5) / slots + bandJitter(seed, 0.012);
      const point = pointOnRectBand(centerX, centerY, maxRadiusX * (band === 0 ? 0.42 : 0.58), maxRadiusY * (band === 0 ? 0.3 : 0.42), progress);
      return {
        x: point.x + bandJitter(seed >> 2, band === 0 ? 5 : 7),
        y: point.y + bandJitter(seed >> 4, band === 0 ? 4 : 5),
        angle: point.angle,
        targetSpread: band === 0 ? 0.44 : 0.58,
      };
    }
    let remaining = index - 28;
    for (let band = 0; band < outerSlotsPerBand.length; band += 1) {
      const slots = outerSlotsPerBand[band];
      const isLastBand = band === outerSlotsPerBand.length - 1;
      const radiusX = maxRadiusX * Math.min(0.7 + band * 0.09, 0.98);
      const radiusY = maxRadiusY * Math.min(0.52 + band * 0.12, 0.96);
      if (remaining < slots || isLastBand) {
        const slot = remaining % slots;
        const progress = (slot + 0.5) / slots + bandJitter(seed + band * 17, 0.016);
        const point = pointOnRectBand(centerX, centerY, radiusX, radiusY, progress);
        return {
          x: point.x + bandJitter(seed >> 1, Math.max(2, 8 - band)),
          y: point.y + bandJitter(seed >> 3, Math.max(2, 6 - Math.min(band, 3))),
          angle: point.angle,
          targetSpread: Math.max(0.66, point.spread),
        };
      }
      remaining -= slots;
    }
    return {
      x: centerX,
      y: centerY,
      angle: -Math.PI / 2,
      targetSpread: 0.72,
    };
  };

  const getDensityCell = (x: number, y: number): { col: number; row: number } => ({
    col: clamp(Math.floor(((x - boundary.left) / Math.max(boundary.right - boundary.left, 1)) * gridColumns), 0, gridColumns - 1),
    row: clamp(Math.floor(((y - boundary.top) / Math.max(boundary.bottom - boundary.top, 1)) * gridRows), 0, gridRows - 1),
  });

  const measureLocalDensity = (x: number, y: number): number => {
    const { col, row } = getDensityCell(x, y);
    let density = 0;
    for (let rowOffset = -1; rowOffset <= 1; rowOffset += 1) {
      for (let colOffset = -1; colOffset <= 1; colOffset += 1) {
        const nextRow = row + rowOffset;
        const nextCol = col + colOffset;
        if (nextRow < 0 || nextRow >= gridRows || nextCol < 0 || nextCol >= gridColumns) {
          continue;
        }
        density += densityGrid[nextRow][nextCol] * (rowOffset === 0 && colOffset === 0 ? 1.2 : 0.6);
      }
    }
    return density;
  };

  const markDensity = (x: number, y: number): void => {
    const { col, row } = getDensityCell(x, y);
    densityGrid[row][col] += 1;
  };

  return baseWords
    .map((item) => {
      const anchor = resolveAnchor(item.index, item.seed);
      const scale =
        item.tier === "hero" || item.tier === "hero-secondary"
          ? heroScale
          : item.tier === "center-ring" || item.tier === "middle-ring"
            ? centerScale
            : outerScale;
      const collisionX = clamp(
        Math.round(item.baseFontSize * scale * (item.tier === "hero" || item.tier === "hero-secondary" ? 0.11 : item.tier === "center-ring" ? 0.09 : item.tier === "middle-ring" ? 0.08 : 0.074)),
        2,
        14
      );
      const collisionY = clamp(
        Math.round(item.baseFontSize * scale * (item.tier === "hero" || item.tier === "hero-secondary" ? 0.08 : item.tier === "center-ring" ? 0.058 : item.tier === "middle-ring" ? 0.054 : 0.05)),
        2,
        11
      );
      let resolved: KeywordCloudLayoutWord | null = null;

      for (let shrinkStep = 0; shrinkStep < 12 && !resolved; shrinkStep += 1) {
        const fontSize = Math.max(minimumReadableFont, Math.round(item.baseFontSize * scale) - shrinkStep);
        const wordSize = measureKeywordSize(item.label, fontSize);
        let bestCandidate: { rect: KeywordCloudRect; x: number; y: number; score: number } | null = null;
        const stepLimit =
          item.tier === "hero" || item.tier === "hero-secondary"
            ? 240
            : item.tier === "center-ring"
              ? 360
              : item.tier === "middle-ring"
                ? 520
                : 720;
        const radialStep =
          item.tier === "hero" || item.tier === "hero-secondary"
            ? 0.18
            : item.tier === "center-ring"
              ? 0.42
              : item.tier === "middle-ring"
                ? 0.68
                : 0.82;
        const angleStep =
          item.tier === "hero" || item.tier === "hero-secondary"
            ? 0.28
            : item.tier === "center-ring"
              ? 0.34
              : item.tier === "middle-ring"
                ? 0.28
                : 0.24;

        for (let step = 0; step < stepLimit; step += 1) {
          const spiralRadius = step * radialStep;
          const angle = anchor.angle + step * angleStep;
          const candidateX = anchor.x + Math.cos(angle) * spiralRadius;
          const candidateY = anchor.y + Math.sin(angle) * spiralRadius * (item.tier === "outer-ring" ? 0.88 : 0.9);
          const candidate: KeywordCloudRect = {
            left: candidateX - wordSize.width / 2,
            top: candidateY - wordSize.height / 2,
            right: candidateX + wordSize.width / 2,
            bottom: candidateY + wordSize.height / 2,
          };
          const insideBounds =
            candidate.left >= boundary.left &&
            candidate.top >= boundary.top &&
            candidate.right <= boundary.right &&
            candidate.bottom <= boundary.bottom;
          if (!insideBounds) {
            continue;
          }
          const collides = placed.some(
            (current) =>
              !(candidate.right <= current.left + collisionX || candidate.left >= current.right - collisionX || candidate.bottom <= current.top + collisionY || candidate.top >= current.bottom - collisionY)
          );
          if (collides) {
            continue;
          }
          const normalizedX = Math.abs(candidateX - centerX) / Math.max(maxRadiusX, 1);
          const normalizedY = Math.abs(candidateY - centerY) / Math.max(maxRadiusY, 1);
          const spread = Math.max(normalizedX, normalizedY);
          const neighborPenalty = placed.reduce((sum, current) => {
            const distance = Math.hypot(candidateX - current.x, candidateY - current.y);
            const threshold = item.tier === "outer-ring" ? 78 : item.tier === "middle-ring" ? 70 : 54;
            if (distance >= threshold) {
              return sum;
            }
            return sum + ((threshold - distance) / threshold) * (current.tier === item.tier ? 1.2 : 0.74);
          }, 0);
          const densityPenalty = measureLocalDensity(candidateX, candidateY);
          const spreadPenalty =
            Math.abs(spread - anchor.targetSpread) * (item.tier === "outer-ring" ? 2.4 : item.tier === "middle-ring" ? 1.6 : 1.1);
          const axisPenalty =
            item.tier === "outer-ring"
              ? Math.abs((candidateY - centerY) / Math.max(maxRadiusY, 1)) * 0.05
              : item.tier === "middle-ring"
                ? Math.abs((candidateX - centerX) / Math.max(maxRadiusX, 1)) * 0.02
                : 0;
          const anchorPenalty = Math.hypot(candidateX - anchor.x, candidateY - anchor.y) / Math.max(radiusBase, 1);
          const score = densityPenalty * (item.tier === "outer-ring" ? 1.34 : 1.12) + neighborPenalty * 1.16 + spreadPenalty + anchorPenalty * 0.22 + axisPenalty + step * 0.0009;
          if (!bestCandidate || score < bestCandidate.score) {
            bestCandidate = { rect: candidate, x: candidateX, y: candidateY, score };
            if (score < 0.9 && step > 16) {
              break;
            }
          }
        }

        if (bestCandidate) {
          placed.push({
            ...bestCandidate.rect,
            x: bestCandidate.x,
            y: bestCandidate.y,
            tier: item.tier,
          });
          markDensity(bestCandidate.x, bestCandidate.y);
          resolved = {
            label: item.label,
            count: item.count,
            x: bestCandidate.x,
            y: bestCandidate.y,
            fontSize,
            opacity:
              item.tier === "outer-ring"
                ? 0.92 + item.emphasis * 0.08
                : item.tier === "middle-ring"
                  ? 0.88 + item.emphasis * 0.1
                  : 0.82 + item.emphasis * 0.16,
            color: resolveKeywordCloudColor(item.emphasis, item.index, item.tier),
            hoverColor: resolveKeywordCloudHoverColor(item.emphasis, item.index, item.tier),
            hoverScale: item.tier === "outer-ring" ? 1.12 : item.tier === "middle-ring" ? 1.09 : 1.06,
            tier: item.tier,
          };
        }
      }

      if (resolved) {
        return resolved;
      }

      const fallbackFontSize = Math.max(minimumReadableFont, Math.round(item.baseFontSize * scale) - (item.tier === "outer-ring" ? 10 : 7));
      const fallbackSize = measureKeywordSize(item.label, fallbackFontSize);
      const fallbackX = clamp(anchor.x, boundary.left + fallbackSize.width / 2, boundary.right - fallbackSize.width / 2);
      const fallbackY = clamp(anchor.y, boundary.top + fallbackSize.height / 2, boundary.bottom - fallbackSize.height / 2);
      placed.push({
        left: fallbackX - fallbackSize.width / 2,
        top: fallbackY - fallbackSize.height / 2,
        right: fallbackX + fallbackSize.width / 2,
        bottom: fallbackY + fallbackSize.height / 2,
        x: fallbackX,
        y: fallbackY,
        tier: item.tier,
      });
      markDensity(fallbackX, fallbackY);
      return {
        label: item.label,
        count: item.count,
        x: fallbackX,
        y: fallbackY,
        fontSize: fallbackFontSize,
        opacity:
          item.tier === "outer-ring"
            ? 0.89 + item.emphasis * 0.08
            : item.tier === "middle-ring"
              ? 0.85 + item.emphasis * 0.1
              : 0.78 + item.emphasis * 0.18,
        color: resolveKeywordCloudColor(item.emphasis, item.index, item.tier),
        hoverColor: resolveKeywordCloudHoverColor(item.emphasis, item.index, item.tier),
        hoverScale: item.tier === "outer-ring" ? 1.12 : item.tier === "middle-ring" ? 1.09 : 1.06,
        tier: item.tier,
      };
    })
    .filter((item): item is KeywordCloudLayoutWord => Boolean(item));
}
