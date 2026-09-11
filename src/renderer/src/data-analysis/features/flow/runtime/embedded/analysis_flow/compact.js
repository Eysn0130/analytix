/* Compact layout mode implementation */
/* eslint-disable no-restricted-globals */

(() => {
  const root =
    typeof globalThis !== "undefined"
      ? globalThis
      : typeof self !== "undefined"
      ? self
      : typeof window !== "undefined"
      ? window
      : null;
  const engine = root?.AnalytixLayoutEngine;
  if (!engine || typeof engine.registerLayoutModeRunner !== "function") return;

  const {
    clampLayout,
    stableHashUnit,
  } = engine;

  const DEFAULT_CFG = Object.freeze({
    R_ADJ_MIN: 74,
    R_ADJ_MAX: 192,
    R_LEAF_MIN: 220,
    COMPACT_CORE_SPACING: 108,
    COMPACT_ADJ_SPACING: 86,
    COMPACT_RING_GAP: 56,
    COMPACT_CLUSTER_MARGIN: 110,
  });
  const TWO_PI = Math.PI * 2;
  const NODE_LAYOUT_METRIC_CACHE = new WeakMap();
  const NODE_LABEL_MAIN_MAX_WIDTH = 236;
  const NODE_LABEL_SUB_MAX_WIDTH = 332;
  const NODE_LABEL_SINGLE_MAX_WIDTH = 312;

  function sortStableIds(ids = []) {
    return (ids || [])
      .map((id) => String(id || "").trim())
      .filter(Boolean)
      .sort((a, b) => String(a).localeCompare(String(b), "zh-CN"));
  }

  function pickFirstStableId(rows, predicate = null) {
    const list = Array.isArray(rows) ? rows : [];
    let best = "";
    for (let i = 0; i < list.length; i += 1) {
      const id = String(list[i] || "").trim();
      if (!id) continue;
      if (typeof predicate === "function" && !predicate(id)) continue;
      if (!best || String(id).localeCompare(best, "zh-CN") < 0) best = id;
    }
    return best;
  }

  function normalizeAngle(angle) {
    const valueRaw = Number(angle);
    if (!Number.isFinite(valueRaw)) return 0;
    let value = valueRaw % TWO_PI;
    if (!Number.isFinite(value)) return 0;
    if (value <= -Math.PI) value += TWO_PI;
    else if (value > Math.PI) value -= TWO_PI;
    return value;
  }

  function angleDelta(a, b) {
    return Math.abs(normalizeAngle((Number(a) || 0) - (Number(b) || 0)));
  }

  function normalizeAnglePositive(angle) {
    const valueRaw = Number(angle);
    if (!Number.isFinite(valueRaw)) return 0;
    let value = valueRaw % TWO_PI;
    if (!Number.isFinite(value)) return 0;
    if (value < 0) value += TWO_PI;
    return value;
  }

  function toWrappedInterval(row) {
    const base = Number(row?.angle);
    const half = Math.max(0, Number(row?.half) || 0);
    if (!Number.isFinite(base) || !(half > 0)) return [];
    if (half >= Math.PI) return [{ start: 0, end: TWO_PI }];
    const start = normalizeAnglePositive(base - half);
    const end = normalizeAnglePositive(base + half);
    if (start <= end) return [{ start, end }];
    return [
      { start: 0, end },
      { start, end: TWO_PI },
    ];
  }

  function mergeAngularIntervals(rows) {
    const list = [];
    (Array.isArray(rows) ? rows : []).forEach((row) => {
      toWrappedInterval(row).forEach((part) => {
        if (!part) return;
        const start = Math.max(0, Number(part.start) || 0);
        const end = Math.min(TWO_PI, Number(part.end) || 0);
        if (!(end > start)) return;
        list.push({ start, end });
      });
    });
    if (!list.length) return [];
    list.sort((a, b) => a.start - b.start || a.end - b.end);
    const merged = [list[0]];
    for (let i = 1; i < list.length; i += 1) {
      const cur = list[i];
      const last = merged[merged.length - 1];
      if (cur.start <= last.end + 1e-9) {
        last.end = Math.max(last.end, cur.end);
      } else {
        merged.push(cur);
      }
    }
    return merged;
  }

  function angleInMergedIntervals(angle, mergedRows) {
    const rows = Array.isArray(mergedRows) ? mergedRows : [];
    if (!rows.length) return false;
    const value = normalizeAnglePositive(angle);
    let lo = 0;
    let hi = rows.length - 1;
    while (lo <= hi) {
      const mid = (lo + hi) >> 1;
      const row = rows[mid];
      if (value < row.start) hi = mid - 1;
      else if (value > row.end) lo = mid + 1;
      else return true;
    }
    return false;
  }

  function isAngleInRanges(angle, ranges) {
    const rows = Array.isArray(ranges) ? ranges : [];
    if (!rows.length) return true;
    const value = normalizeAnglePositive(angle);
    for (const row of rows) {
      const start = normalizeAnglePositive(row?.start);
      const end = normalizeAnglePositive(row?.end);
      const span = Number(row?.span);
      if (Number.isFinite(span) && span >= Math.PI * 2 - 1e-3) return true;
      if (start <= end) {
        if (value >= start && value <= end) return true;
      } else if (value >= start || value <= end) {
        return true;
      }
    }
    return false;
  }

  function pickRepresentativeAngle(ranges, fallback = 0) {
    const rows = Array.isArray(ranges) ? ranges.filter(Boolean) : [];
    if (!rows.length) return Number(fallback) || 0;
    let best = rows[0];
    let bestSpan = Number(best?.span) || 0;
    rows.forEach((row) => {
      const span = Number(row?.span) || 0;
      if (span > bestSpan) {
        best = row;
        bestSpan = span;
      }
    });
    const start = Number(best?.start) || 0;
    const end = Number(best?.end) || 0;
    if (bestSpan >= Math.PI * 2 - 1e-3) return 0;
    const startPos = normalizeAnglePositive(start);
    const endPos = normalizeAnglePositive(end);
    const startNorm = startPos <= endPos ? startPos : startPos - Math.PI * 2;
    const endNorm = startPos <= endPos ? endPos : endPos;
    const mid = startNorm + (endNorm - startNorm) / 2;
    return normalizeAnglePositive(mid);
  }

  function isAngleBlocked(angle, sectors, guard = 0) {
    const rows = Array.isArray(sectors) ? sectors : [];
    for (const row of rows) {
      const base = Number(row?.angle);
      const half = Math.max(0, Number(row?.half) || 0) + Math.max(0, Number(guard) || 0);
      if (!Number.isFinite(base)) continue;
      if (angleDelta(angle, base) <= half) return true;
    }
    return false;
  }

  function estimateTextUnits(text) {
    const raw = String(text || "");
    if (!raw) return 0;
    let units = 0;
    for (const ch of raw) {
      const cp = ch.codePointAt(0) || 0;
      if (/\s/.test(ch)) units += 0.34;
      else if (cp > 255) units += 1.02;
      else if (/[0-9]/.test(ch)) units += 0.78;
      else if (/[A-Z]/.test(ch)) units += 0.7;
      else if (/[a-z]/.test(ch)) units += 0.62;
      else units += 0.68;
    }
    return units;
  }

  function resolveDisplayLabelText(node) {
    const row = node && typeof node === "object" ? node : {};
    const direct = String(row?.display_id || row?.displayId || row?.displayIdRaw || row?.id || "").trim();
    const ids =
      (Array.isArray(row?.display_ids) ? row.display_ids : Array.isArray(row?.displayIds) ? row.displayIds : [])
        .map((id) => String(id || "").trim())
        .filter(Boolean)
        .slice(0, 3);
    if (!ids.length) return direct;
    const grouped = ids.join("/");
    return grouped.length > direct.length ? grouped : direct;
  }

  function estimateNodeLabelProfile(node, radius) {
    const r = Math.max(9, Number(radius) || 18);
    const baseSize = clampLayout(Number(node?.fontSize) || 13, 8, 36);
    const subSize = Math.max(8, baseSize - 1);
    const name = String(node?.name || "").trim();
    const title = String(node?.title || node?.label || node?.id || "").trim();
    const displayId = resolveDisplayLabelText(node);
    const mainText = name ? name || title : displayId || title;
    const subText = name ? displayId || title : "";
    const singleText = name ? "" : displayId || title;
    const mainWidth = Math.min(NODE_LABEL_MAIN_MAX_WIDTH, estimateTextUnits(mainText) * Math.max(1, baseSize));
    const subWidth = Math.min(NODE_LABEL_SUB_MAX_WIDTH, estimateTextUnits(subText) * Math.max(1, subSize));
    const singleWidth = Math.min(NODE_LABEL_SINGLE_MAX_WIDTH, estimateTextUnits(singleText) * Math.max(1, baseSize));
    const halfWidth = Math.max(18, mainWidth / 2, subWidth / 2, singleWidth / 2);
    const mainHalf = Math.max(4, baseSize * 0.56);
    const subHalf = Math.max(4, subSize * 0.56);
    const bottom = name
      ? Math.max(r + 16 + mainHalf, r + 34 + subHalf)
      : Math.max(r + 20 + mainHalf, r);
    return {
      halfWidth,
      bottom,
      textDensity: Math.max(0, Math.min(1.2, (halfWidth - 18) / 120)),
    };
  }

  function getNodeLayoutMetrics(node) {
    if (!node || typeof node !== "object") {
      return { radius: 18, label: 18, collisionRadius: 36, labelHalfWidth: 24, labelBottom: 40 };
    }
    const cached = NODE_LAYOUT_METRIC_CACHE.get(node);
    if (cached) return cached;
    const radius = Math.max(9, Number(node?.r) || 18);
    const profile = estimateNodeLabelProfile(node, radius);
    const nameLen = String(node?.title || node?.name || "").trim().length;
    const label = clampLayout(
      12 + nameLen * 0.42 + profile.textDensity * 26 + Math.sqrt(Math.max(0, profile.bottom)) * 0.6,
      16,
      112
    );
    const collisionRadius = Math.max(
      radius + label,
      radius + profile.bottom * 0.62,
      radius * 0.18 + profile.halfWidth * 0.84
    );
    const metrics = {
      radius,
      label,
      collisionRadius,
      labelHalfWidth: profile.halfWidth,
      labelBottom: profile.bottom,
    };
    NODE_LAYOUT_METRIC_CACHE.set(node, metrics);
    return metrics;
  }

  function nodeRadius(node) {
    return getNodeLayoutMetrics(node).radius;
  }

  function labelPad(node) {
    return getNodeLayoutMetrics(node).label;
  }

  function nodeLabelHalfWidth(node) {
    return getNodeLayoutMetrics(node).labelHalfWidth;
  }

  function nodeCollisionRadius(node) {
    return getNodeLayoutMetrics(node).collisionRadius;
  }

  function computePairGap(leftNode, rightNode, options = {}) {
    const minBase = Math.max(212, Number(options?.minBase) || 248);
    const maxCap = Math.max(minBase + 36, Number(options?.maxCap) || 580);
    const radialNeed =
      nodeCollisionRadius(leftNode) + nodeCollisionRadius(rightNode) + Math.max(12, Number(options?.radialPad) || 22);
    const labelBase =
      nodeLabelHalfWidth(leftNode) + nodeLabelHalfWidth(rightNode) + Math.max(30, Number(options?.labelPad) || 54);
    const comfortBoost = clampLayout(labelBase * 0.25, 22, 116);
    const labelNeed = labelBase + comfortBoost;
    const edgeCorridorNeed =
      nodeRadius(leftNode) + nodeRadius(rightNode) + Math.max(110, Number(options?.edgeCorridor) || 146);
    return clampLayout(Math.max(minBase, radialNeed, labelNeed, edgeCorridorNeed), minBase, maxCap);
  }

  function makeZone(id, x, y, node, extra = 0) {
    return {
      id: String(id || ""),
      x: Number(x) || 0,
      y: Number(y) || 0,
      r: nodeCollisionRadius(node) + Math.max(0, Number(extra) || 0),
    };
  }

  function createZoneSpatialIndex(zones, options = {}) {
    const cellSize = Math.max(18, Number(options?.cellSize) || 148);
    const invCellSize = 1 / cellSize;
    const buckets = new Map();
    const entries = [];
    let maxRadius = 0;

    const addZone = (zone) => {
      if (!zone) return null;
      const x = Number(zone.x) || 0;
      const y = Number(zone.y) || 0;
      const r = Math.max(0, Number(zone.r) || 0);
      const cx = Math.floor(x * invCellSize);
      const cy = Math.floor(y * invCellSize);
      const entry = {
        zone,
        x,
        y,
        r,
        seq: entries.length,
      };
      entries.push(entry);
      if (r > maxRadius) maxRadius = r;
      let row = buckets.get(cx);
      if (!row) {
        row = new Map();
        buckets.set(cx, row);
      }
      const list = row.get(cy);
      if (list) list.push(entry);
      else row.set(cy, [entry]);
      return entry;
    };

    (Array.isArray(zones) ? zones : []).forEach((zone) => addZone(zone));

    const forEachNearby = (x, y, reach, visitor) => {
      const px = Number(x) || 0;
      const py = Number(y) || 0;
      const rr = Math.max(0, Number(reach) || 0);
      if (!(rr > 0) || typeof visitor !== "function") return false;
      const minCx = Math.floor((px - rr) * invCellSize);
      const maxCx = Math.floor((px + rr) * invCellSize);
      const minCy = Math.floor((py - rr) * invCellSize);
      const maxCy = Math.floor((py + rr) * invCellSize);
      for (let cx = minCx; cx <= maxCx; cx += 1) {
        const row = buckets.get(cx);
        if (!row) continue;
        for (let cy = minCy; cy <= maxCy; cy += 1) {
          const list = row.get(cy);
          if (!list || !list.length) continue;
          for (let i = 0; i < list.length; i += 1) {
            if (visitor(list[i])) return true;
          }
        }
      }
      return false;
    };

    return {
      addZone,
      forEachNearby,
      maxRadius: () => maxRadius,
    };
  }

  function collidesWithZones(x, y, node, zones, padding = 6, zoneIndex = null, nodeRadiusOverride = NaN) {
    const rows = Array.isArray(zones) ? zones : [];
    const px = Number(x) || 0;
    const py = Number(y) || 0;
    const radius = Number.isFinite(Number(nodeRadiusOverride))
      ? Math.max(0, Number(nodeRadiusOverride))
      : nodeCollisionRadius(node);
    const pad = Math.max(0, Number(padding) || 0);
    const maxZoneRadius = Number(zoneIndex?.maxRadius?.()) || 0;
    const reach = radius + pad + maxZoneRadius;
    if (zoneIndex && typeof zoneIndex.forEachNearby === "function") {
      const hit = zoneIndex.forEachNearby(
        px,
        py,
        reach,
        (entry) => {
          if (!entry) return false;
          const dx = entry.x - px;
          const dy = entry.y - py;
          const threshold = radius + entry.r + pad;
          return dx * dx + dy * dy < threshold * threshold;
        }
      );
      return !!hit;
    }
    for (const zone of rows) {
      if (!zone) continue;
      const dx = (Number(zone.x) || 0) - px;
      const dy = (Number(zone.y) || 0) - py;
      const threshold = radius + (Number(zone.r) || 0) + pad;
      if (dx * dx + dy * dy < threshold * threshold) return true;
    }
    return false;
  }

  function rolePriority(node, meta) {
    const role = String(meta?.role || node?.role || "").toLowerCase();
    const roleRank = role === "core" ? 0 : role === "adjacent" ? 1 : role === "leaf" ? 2 : 3;
    const seedRank = meta?.isSeedSelected || node?.isSeedSelected ? 0 : 1;
    return roleRank * 10 + seedRank;
  }

  function applyNodeSemanticMeta(node, meta = {}) {
    if (!node || typeof node !== "object") return;
    const prevX = Number.isFinite(Number(meta?.prevX))
      ? Number(meta.prevX)
      : Number.isFinite(Number(node?.x))
      ? Number(node.x)
      : null;
    const prevY = Number.isFinite(Number(meta?.prevY))
      ? Number(meta.prevY)
      : Number.isFinite(Number(node?.y))
      ? Number(node.y)
      : null;
    node.role = String(meta?.role || node?.role || "").toLowerCase() || "leaf";
    node.clusterId = String(meta?.clusterId || node?.clusterId || "");
    node.seedCoreCandidate = !!meta?.seedCoreCandidate;
    node.isSeedSelected = !!meta?.isSeedSelected;
    node.demoteReason = meta?.demoteReason || null;
    node.isPathPromotedAdjacent = !!meta?.isPathPromotedAdjacent;
    node.isBridgeAdjacent = !!meta?.isBridgeAdjacent;
    node.isHubAdjacent = !!meta?.isHubAdjacent;
    node.layoutAnchorForCluster = !!meta?.layoutAnchorForCluster;
    node.weight = Number(meta?.weight) || Number(node?.weight) || 0;
    node.prevX = prevX;
    node.prevY = prevY;
    node.layoutBand = node.role === "core" ? "core" : node.role === "adjacent" ? "adjacent" : "leaf";
    node.layoutRoleWhy = Array.isArray(meta?.why) ? meta.why.join("|") : String(node?.layoutRoleWhy || "");
  }

  function buildClusterAdjacency(cluster, clusterNodeSet) {
    const out = {};
    const source = cluster?.localAdjacency && typeof cluster.localAdjacency === "object" ? cluster.localAdjacency : {};
    clusterNodeSet.forEach((id) => {
      const raw = Array.isArray(source?.[id]) ? source[id] : [];
      if (!raw.length) {
        out[id] = [];
        return;
      }
      const rows = [];
      for (let i = 0; i < raw.length; i += 1) {
        const nid = String(raw[i] || "").trim();
        if (!nid || !clusterNodeSet.has(nid)) continue;
        rows.push(nid);
      }
      if (rows.length > 1) {
        rows.sort((a, b) => String(a).localeCompare(String(b), "zh-CN"));
      }
      out[id] = rows;
    });
    return out;
  }

  function pickFallbackCenter(clusterNodes) {
    const rows = (clusterNodes || []).slice().sort((a, b) =>
      String(a?.id || "").localeCompare(String(b?.id || ""), "zh-CN")
    );
    return rows[0] || null;
  }

  function buildIrregularCorePlacement(centerNodes, center, options = {}) {
    const rows = Array.isArray(centerNodes) ? centerNodes.filter(Boolean) : [];
    if (!rows.length) return [];
    const localAdjacency = options?.localAdjacency || {};
    const nodeMetaById = options?.nodeMetaById || {};
    const baseRadius = Math.max(72, Number(options?.baseRadius) || 180);
    const baseSpacing = Math.max(64, Number(options?.baseSpacing) || 108);
    const adjacentCount = Math.max(0, Number(options?.adjacentCount) || 0);
    const leafCount = Math.max(0, Number(options?.leafCount) || 0);
    const centerX = Number(center?.x) || 0;
    const centerY = Number(center?.y) || 0;
    const coreIdSet = new Set(rows.map((node) => String(node?.id || "").trim()).filter(Boolean));
    const demandRows = rows.map((node) => {
      const id = String(node?.id || "").trim();
      const neighborIds = sortStableIds(localAdjacency?.[id] || []);
      let peripheralLinks = 0;
      let coreLinks = 0;
      neighborIds.forEach((nid) => {
        if (coreIdSet.has(nid)) coreLinks += 1;
        else peripheralLinks += 1;
      });
      const seedBoost = nodeMetaById?.[id]?.isSeedSelected ? 0.45 : 0;
      const demand = Math.max(1, 1 + peripheralLinks * 0.72 + coreLinks * 0.35 + seedBoost);
      return { node, id, demand, peripheralLinks, coreLinks };
    });
    const demandTotal = demandRows.reduce((sum, row) => sum + row.demand, 0) || demandRows.length;
    let cursor =
      stableHashUnit(`compact-core-skeleton|${demandRows.map((row) => row.id).join("|")}`) * Math.PI * 2;
    const placement = demandRows.map((row) => {
      const span = (Math.PI * 2 * row.demand) / demandTotal;
      const angleJitter =
        (stableHashUnit(`compact-core-angle|${row.id}`) - 0.5) * Math.min(0.58, span * 0.42);
      const angle = cursor + span * 0.5 + angleJitter;
      cursor += span;
      const radialJitter = (stableHashUnit(`compact-core-radial|${row.id}`) - 0.5) * 0.52;
      const demandBoost = Math.min(0.18, row.peripheralLinks / Math.max(20, rows.length * 4));
      const radius = baseRadius * clampLayout(1 + radialJitter + demandBoost, 0.68, 1.62);
      return {
        ...row,
        angle,
        x: centerX + Math.cos(angle) * radius,
        y: centerY + Math.sin(angle) * radius,
      };
    });

    // Repel dense core points while keeping them inside a broad radial envelope.
    const pairBase = baseSpacing * 1.24 + Math.sqrt(Math.max(1, adjacentCount)) * 2.1;
    const pairLeafBoost = Math.sqrt(Math.max(1, leafCount)) * 0.8;
    const minRadius = Math.max(64, baseRadius * 0.62);
    const maxRadius = Math.max(
      minRadius + 120,
      baseRadius * clampLayout(1.9 + Math.sqrt(rows.length) / 5.5, 1.85, 3.1)
    );
    for (let iter = 0; iter < 120; iter += 1) {
      let moved = 0;
      for (let i = 0; i < placement.length; i += 1) {
        const a = placement[i];
        for (let j = i + 1; j < placement.length; j += 1) {
          const b = placement[j];
          let dx = (Number(b.x) || 0) - (Number(a.x) || 0);
          let dy = (Number(b.y) || 0) - (Number(a.y) || 0);
          let dist = Math.hypot(dx, dy);
          if (!Number.isFinite(dist) || dist < 1e-6) {
            const baseAngle = stableHashUnit(`compact-core-repel|${a.id}|${b.id}`) * Math.PI * 2;
            dx = Math.cos(baseAngle);
            dy = Math.sin(baseAngle);
            dist = 1;
          } else {
            dx /= dist;
            dy /= dist;
          }
          const desired = pairBase + pairLeafBoost + (a.demand + b.demand) * 2.3;
          if (dist >= desired) continue;
          const push = (desired - dist) * 0.5 * 0.84;
          a.x -= dx * push;
          a.y -= dy * push;
          b.x += dx * push;
          b.y += dy * push;
          moved += push;
        }
      }
      placement.forEach((row) => {
        let dx = (Number(row.x) || 0) - centerX;
        let dy = (Number(row.y) || 0) - centerY;
        let dist = Math.hypot(dx, dy);
        if (!Number.isFinite(dist) || dist < 1e-6) {
          const a = stableHashUnit(`compact-core-center|${row.id}`) * Math.PI * 2;
          dx = Math.cos(a);
          dy = Math.sin(a);
          dist = 1;
        } else {
          dx /= dist;
          dy /= dist;
        }
        if (dist > maxRadius) {
          const pull = (dist - maxRadius) * 0.26;
          row.x -= dx * pull;
          row.y -= dy * pull;
          moved += pull;
        } else if (dist < minRadius) {
          const push = (minRadius - dist) * 0.31;
          row.x += dx * push;
          row.y += dy * push;
          moved += push;
        }
      });
      if (moved < 0.28) break;
    }
    return placement;
  }

  function placeCompactBackbone(coreNodes, adjacentNodes, layoutAnchors, params) {
    const cfg = params?.config || DEFAULT_CFG;
    const nodeMetaById = params?.nodeMetaById || {};
    const localAdjacency = params?.localAdjacency || {};
    const leafCount = Math.max(0, Number(params?.leafCount) || 0);
    const center = params?.center || { x: 0, y: 0 };
    const out = {
      positions: new Map(),
      centers: [],
      occupiedZones: [],
    };
    const centerNodes = (coreNodes && coreNodes.length ? coreNodes : layoutAnchors || []).slice();
    const sortedCenters = centerNodes.sort((a, b) =>
      String(a?.id || "").localeCompare(String(b?.id || ""), "zh-CN")
    );
    const centerIdSet = new Set(
      sortedCenters
        .map((node) => String(node?.id || "").trim())
        .filter(Boolean)
    );
    if (sortedCenters.length === 1) {
      const node = sortedCenters[0];
      out.positions.set(node.id, { x: center.x, y: center.y });
      out.centers.push({ id: node.id, x: center.x, y: center.y, node });
      out.occupiedZones.push(makeZone(node.id, center.x, center.y, node, 10));
    } else if (sortedCenters.length > 1) {
      const centerCount = sortedCenters.length;
      const adjacentCount = Math.max(
        0,
        Array.isArray(adjacentNodes)
          ? adjacentNodes.filter((node) => {
              const id = String(node?.id || "").trim();
              return !!id && !centerIdSet.has(id);
            }).length
          : 0
      );
      const baseSpacing = Math.max(42, Number(cfg.COMPACT_CORE_SPACING) || 108);
      const baseFactor = Math.max(0.72, Math.min(1.42, centerCount / 2.8));
      const centerBoost = Math.max(1, Math.min(2.75, 1 + Math.max(0, centerCount - 4) * 0.22));
      const leafBoost = Math.max(1, Math.min(3.1, 1 + Math.sqrt(leafCount) / 8.2));
      const adjacentBoost = Math.max(1, Math.min(2.45, 1 + Math.sqrt(adjacentCount) / 6.1));
      const densityBoost = Math.max(1, Math.min(1.68, 1 + adjacentCount / 32 + leafCount / 610));
      const spreadBoost = Math.max(centerBoost, leafBoost, adjacentBoost) * densityBoost;
      const radius = baseSpacing * baseFactor * spreadBoost;
      const irregularCenters = buildIrregularCorePlacement(sortedCenters, center, {
        baseRadius: radius,
        baseSpacing,
        adjacentCount,
        leafCount,
        localAdjacency,
        nodeMetaById,
      });
      irregularCenters.forEach((row) => {
        const node = row.node;
        const x = Number(row.x) || center.x;
        const y = Number(row.y) || center.y;
        out.positions.set(node.id, { x, y });
        out.centers.push({ id: node.id, x, y, node });
        out.occupiedZones.push(makeZone(node.id, x, y, node, 10));
      });
    }
    const sortedAdjacent = (adjacentNodes || [])
      .filter((node) => {
        const id = String(node?.id || "").trim();
        return !!id && !centerIdSet.has(id);
      })
      .slice()
      .sort((a, b) => String(a?.id || "").localeCompare(String(b?.id || ""), "zh-CN"));
    if (sortedAdjacent.length) {
      const occupiedZoneIndex = createZoneSpatialIndex(out.occupiedZones, { cellSize: 152 });
      const minR = Math.max(22, Number(cfg.R_ADJ_MIN) || 74);
      const maxR = Math.max(minR + 12, Number(cfg.R_ADJ_MAX) || 192);
      const coreIdSet = centerIdSet;
      const pairLaneCount = new Map();
      const coreLaneCount = new Map();
      const fallbackStep = (Math.PI * 2) / Math.max(1, sortedAdjacent.length);
      const fallbackOffset = stableHashUnit(`adj|${sortedAdjacent.map((node) => node?.id).join("|")}`) * Math.PI * 2;
      const makePairKey = (a, b) => {
        const s = String(a || "").trim();
        const t = String(b || "").trim();
        if (!s || !t) return "";
        return s < t ? `${s}|${t}` : `${t}|${s}`;
      };
      const fallbackTargetR = clampLayout(minR + sortedAdjacent.length * 6, minR, maxR);
      sortedAdjacent.forEach((node, index) => {
        const meta = nodeMetaById?.[node.id] || {};
        const id = String(node?.id || "").trim();
        const linkedCenters = sortStableIds(
          (localAdjacency?.[id] || []).filter((nid) => coreIdSet.has(String(nid || "").trim()))
        );
        let x = Number.NaN;
        let y = Number.NaN;

        if (linkedCenters.length >= 2) {
          let bestPair = [linkedCenters[0], linkedCenters[1]];
          let bestDist = -1;
          for (let i = 0; i < linkedCenters.length; i += 1) {
            const aId = linkedCenters[i];
            const aPos = out.positions.get(aId);
            if (!aPos) continue;
            for (let j = i + 1; j < linkedCenters.length; j += 1) {
              const bId = linkedCenters[j];
              const bPos = out.positions.get(bId);
              if (!bPos) continue;
              const dist = Math.hypot((Number(bPos.x) || 0) - (Number(aPos.x) || 0), (Number(bPos.y) || 0) - (Number(aPos.y) || 0));
              if (dist > bestDist) {
                bestDist = dist;
                bestPair = [aId, bId];
              }
            }
          }
          const aPos = out.positions.get(bestPair[0]);
          const bPos = out.positions.get(bestPair[1]);
          if (aPos && bPos) {
            const dx = (Number(bPos.x) || 0) - (Number(aPos.x) || 0);
            const dy = (Number(bPos.y) || 0) - (Number(aPos.y) || 0);
            const dist = Math.hypot(dx, dy);
            if (dist > 1e-3) {
              const ux = dx / dist;
              const uy = dy / dist;
              const nx = -uy;
              const ny = ux;
              const pairKey = makePairKey(bestPair[0], bestPair[1]);
              const laneRaw = pairLaneCount.get(pairKey) || 0;
              pairLaneCount.set(pairKey, laneRaw + 1);
              const laneIndex = Math.floor(laneRaw / 2) + 1;
              const laneSign = laneRaw % 2 === 0 ? 1 : -1;
              const sideHash = stableHashUnit(`pair-side|${pairKey}|${id}`) < 0.5 ? -1 : 1;
              const tangentJitter = (stableHashUnit(`pair-t|${id}`) - 0.5) * Math.min(0.36, 1 / laneIndex);
              const t = clampLayout(0.5 + tangentJitter, 0.18, 0.82);
              const normalGap = clampLayout(
                44 + Math.sqrt(sortedAdjacent.length) * 3.8 + Math.sqrt(Math.max(1, leafCount)) * 1.2,
                38,
                154
              );
              const normalShift = sideHash * laneSign * laneIndex * normalGap;
              x = aPos.x + dx * t + nx * normalShift;
              y = aPos.y + dy * t + ny * normalShift;
            }
          }
        }

        if (!Number.isFinite(x) || !Number.isFinite(y)) {
          const coreId = linkedCenters[0] || "";
          const corePos = coreId ? out.positions.get(coreId) : null;
          if (corePos) {
            const laneRaw = coreLaneCount.get(coreId) || 0;
            coreLaneCount.set(coreId, laneRaw + 1);
            const laneIndex = Math.floor(laneRaw / 3);
            const outwardAngle = Math.atan2((Number(corePos.y) || 0) - center.y, (Number(corePos.x) || 0) - center.x);
            const angleJitter = (stableHashUnit(`core-jitter|${coreId}|${id}`) - 0.5) * 1.36;
            const angle = outwardAngle + angleJitter;
            let radius = minR + 74 + laneIndex * 18 + Math.sqrt(Math.max(1, leafCount)) * 1.6;
            if (meta?.isBridgeAdjacent) radius += 30;
            if (meta?.isHubAdjacent) radius += 16;
            radius = clampLayout(radius, minR, maxR + 156);
            x = corePos.x + Math.cos(angle) * radius;
            y = corePos.y + Math.sin(angle) * radius;
          }
        }

        if (!Number.isFinite(x) || !Number.isFinite(y)) {
          const baseAngle = fallbackOffset + index * fallbackStep;
          x = center.x + Math.cos(baseAngle) * fallbackTargetR;
          y = center.y + Math.sin(baseAngle) * fallbackTargetR;
        }

        const guardRadius = clampLayout(
          128 + Math.sqrt(sortedAdjacent.length) * 12 + Math.sqrt(Math.max(1, leafCount)) * 1.8,
          140,
          380
        );
        let gx = x - center.x;
        let gy = y - center.y;
        let gdist = Math.hypot(gx, gy);
        if (gdist < guardRadius) {
          if (!Number.isFinite(gdist) || gdist < 1e-3) {
            const a = stableHashUnit(`adj-guard|${id}`) * Math.PI * 2;
            gx = Math.cos(a);
            gy = Math.sin(a);
            gdist = 1;
          } else {
            gx /= gdist;
            gy /= gdist;
          }
          const push = guardRadius - gdist;
          x += gx * push;
          y += gy * push;
        }

        if (collidesWithZones(x, y, node, out.occupiedZones, 8, occupiedZoneIndex)) {
          const base = stableHashUnit(`adj-avoid|${id}`) * Math.PI * 2;
          for (let attempt = 1; attempt <= 12; attempt += 1) {
            const radius = 8 + attempt * 11;
            const angle = base + attempt * 0.62;
            const nx = x + Math.cos(angle) * radius;
            const ny = y + Math.sin(angle) * radius;
            if (!collidesWithZones(nx, ny, node, out.occupiedZones, 8, occupiedZoneIndex)) {
              x = nx;
              y = ny;
              break;
            }
          }
        }
        out.positions.set(node.id, { x, y });
        const zone = makeZone(node.id, x, y, node, 6);
        out.occupiedZones.push(zone);
        occupiedZoneIndex.addZone(zone);
      });
    }
    if (sortedCenters.length && sortedAdjacent.length) {
      const adjacentIdSet = new Set(
        sortedAdjacent
          .map((node) => String(node?.id || "").trim())
          .filter(Boolean)
      );
      const centerRefById = new Map(
        out.centers
          .map((row) => [String(row?.id || "").trim(), row])
          .filter(([id]) => !!id)
      );
      const zoneById = new Map(
        out.occupiedZones
          .map((zone) => [String(zone?.id || "").trim(), zone])
          .filter(([id]) => !!id)
      );
      sortedCenters.forEach((centerNode) => {
        const centerId = String(centerNode?.id || "").trim();
        if (!centerId) return;
        const centerPos = out.positions.get(centerId);
        if (!centerPos) return;
        const linkedAdjacents = sortStableIds(
          (localAdjacency?.[centerId] || []).filter((nid) => adjacentIdSet.has(nid))
        );
        if (!linkedAdjacents.length) return;
        const sum = linkedAdjacents.reduce(
          (acc, id) => {
            const pos = out.positions.get(id);
            if (!pos) return acc;
            acc.x += Number(pos.x) || 0;
            acc.y += Number(pos.y) || 0;
            acc.n += 1;
            return acc;
          },
          { x: 0, y: 0, n: 0 }
        );
        if (!sum.n) return;
        const target = { x: sum.x / sum.n, y: sum.y / sum.n };
        const dx = target.x - centerPos.x;
        const dy = target.y - centerPos.y;
        const dist = Math.hypot(dx, dy);
        if (!Number.isFinite(dist) || dist < 1) return;
        const maxShift = Math.max(8, Math.min(16, (Number(cfg.COMPACT_ADJ_SPACING) || 86) * 0.18));
        const shift = Math.min(maxShift, dist * 0.12);
        const scale = shift / dist;
        const nx = centerPos.x + dx * scale;
        const ny = centerPos.y + dy * scale;
        out.positions.set(centerId, { x: nx, y: ny });
        const centerRef = centerRefById.get(centerId);
        if (centerRef) {
          centerRef.x = nx;
          centerRef.y = ny;
        }
        const zone = zoneById.get(centerId);
        if (zone) {
          zone.x = nx;
          zone.y = ny;
        }
      });
    }
    return out;
  }

  function placeLeafRings(centerNode, leafNodes, occupiedSectors, occupiedZones, params) {
    const cfg = params?.config || DEFAULT_CFG;
    const centerX = Number(centerNode?.x) || 0;
    const centerY = Number(centerNode?.y) || 0;
    const rows = (leafNodes || [])
      .slice()
      .sort((a, b) => String(a?.id || "").localeCompare(String(b?.id || ""), "zh-CN"));
    const out = {
      positions: new Map(),
      report: {
        centerId: String(centerNode?.id || ""),
        leafMode: "circle",
        ringCount: 0,
        rings: [],
        avoidanceHits: 0,
        territoryRangeCount: 0,
      },
    };
    if (!rows.length) return out;
    const territoryRanges = Array.isArray(params?.territoryRanges) ? params.territoryRanges : [];
    out.report.territoryRangeCount = territoryRanges.length;
    const occupiedZoneIndex = createZoneSpatialIndex(occupiedZones, { cellSize: 148 });
    const blockedIntervals = mergeAngularIntervals(
      (Array.isArray(occupiedSectors) ? occupiedSectors : [])
        .map((row) => {
          const base = Number(row?.angle);
          const half = Math.max(0, Number(row?.half) || 0) + 0.06;
          if (!Number.isFinite(base) || !(half > 0)) return null;
          return { angle: base, half };
        })
        .filter(Boolean)
    );
    const normalizedTerritoryRanges = territoryRanges
      .map((row) => {
        const span = Number(row?.span);
        if (Number.isFinite(span) && span >= TWO_PI - 1e-3) {
          return { full: true, start: 0, end: 0 };
        }
        const start = normalizeAnglePositive(row?.start);
        const end = normalizeAnglePositive(row?.end);
        return { full: false, start, end };
      })
      .filter(Boolean);
    const territoryHasFullRange = normalizedTerritoryRanges.some((row) => !!row?.full);
    const isInTerritory = (angle) => {
      if (territoryHasFullRange) return true;
      if (!normalizedTerritoryRanges.length) return true;
      const value = normalizeAnglePositive(angle);
      for (let i = 0; i < normalizedTerritoryRanges.length; i += 1) {
        const row = normalizedTerritoryRanges[i];
        if (row.start <= row.end) {
          if (value >= row.start && value <= row.end) return true;
        } else if (value >= row.start || value <= row.end) {
          return true;
        }
      }
      return false;
    };
    const isBlocked = (angle) => angleInMergedIntervals(angle, blockedIntervals);
    const leafCount = rows.length;
    const ringGapBase = Math.max(30, Number(cfg.COMPACT_RING_GAP) || 56);
    const ringGapBoost = Math.max(1, Math.min(1.9, 1 + Math.sqrt(leafCount) / 20));
    const ringGap = ringGapBase * ringGapBoost;
    const cfgLeafRadius = Math.max(72, Number(cfg.R_LEAF_MIN) || 220);
    const smallLeafCap = leafCount <= 4 ? 148 + leafCount * 8 : cfgLeafRadius;
    const minLeafRadiusBase = Math.min(cfgLeafRadius, smallLeafCap);
    const minLeafRadiusBoost =
      leafCount <= 4
        ? clampLayout(0.9 + Math.sqrt(Math.max(1, leafCount)) / 22, 0.9, 1.02)
        : Math.max(1, Math.min(2.2, 1 + Math.sqrt(leafCount) / 15));
    const minLeafRadius = minLeafRadiusBase * minLeafRadiusBoost;
    const avgLeafCollision =
      rows.reduce((sum, node) => sum + nodeCollisionRadius(node), 0) / Math.max(1, rows.length);
    const baseRadius = Math.max(
      minLeafRadius,
      nodeRadius(centerNode?.node || centerNode) + ringGap,
      nodeCollisionRadius(centerNode?.node || centerNode) + avgLeafCollision + 24
    );
    const minGapFromLabels = avgLeafCollision * 1.15;
    const minGap = clampLayout(Math.max(38, 34 + Math.sqrt(leafCount) * 2.1, minGapFromLabels), 38, 160);
    let radius = baseRadius;
    let cursor = 0;
    let ringIndex = 0;
    while (cursor < rows.length) {
      const capacity = Math.max(4, Math.floor((2 * Math.PI * radius) / minGap));
      const ringNodes = rows.slice(cursor, cursor + capacity);
      const step = (Math.PI * 2) / Math.max(1, ringNodes.length);
      const offset = stableHashUnit(`${centerNode?.id || ""}|ring|${ringIndex}`) * Math.PI * 2;
      const angleStep = Math.max(0.09, step / 5);
      const maxAttempts = 14;
      const attemptShifts = new Array(maxAttempts + 1);
      const attemptCos = new Array(maxAttempts + 1);
      const attemptSin = new Array(maxAttempts + 1);
      for (let attempt = 0; attempt <= maxAttempts; attempt += 1) {
        const sign = attempt % 2 === 0 ? 1 : -1;
        const shift = attempt === 0 ? 0 : sign * angleStep * Math.ceil(attempt / 2);
        attemptShifts[attempt] = shift;
        attemptCos[attempt] = Math.cos(shift);
        attemptSin[attempt] = Math.sin(shift);
      }
      ringNodes.forEach((node, index) => {
        const collisionRadius = nodeCollisionRadius(node);
        const baseAngle = offset + index * step;
        const cosBase = Math.cos(baseAngle);
        const sinBase = Math.sin(baseAngle);
        let best = null;
        for (let attempt = 0; attempt <= maxAttempts; attempt += 1) {
          const shift = attemptShifts[attempt];
          const angle = baseAngle + shift;
          if (!isInTerritory(angle)) continue;
          if (isBlocked(angle)) continue;
          const cosA = cosBase * attemptCos[attempt] - sinBase * attemptSin[attempt];
          const sinA = sinBase * attemptCos[attempt] + cosBase * attemptSin[attempt];
          const x = centerX + cosA * radius;
          const y = centerY + sinA * radius;
          const collision = collidesWithZones(x, y, node, occupiedZones, 6, occupiedZoneIndex, collisionRadius);
          if (!collision) {
            best = { x, y, angle };
            break;
          }
          out.report.avoidanceHits += 1;
          if (!best) best = { x, y, angle };
        }
        if (!best) {
          best = {
            x: centerX + Math.cos(baseAngle) * radius,
            y: centerY + Math.sin(baseAngle) * radius,
            angle: baseAngle,
          };
          if (!isInTerritory(best.angle) && territoryRanges.length) {
            const fallbackAngle = pickRepresentativeAngle(territoryRanges, best.angle);
            best = {
              x: centerX + Math.cos(fallbackAngle) * radius,
              y: centerY + Math.sin(fallbackAngle) * radius,
              angle: fallbackAngle,
            };
          }
        }
        out.positions.set(node.id, { x: best.x, y: best.y });
        const zone = makeZone(node.id, best.x, best.y, node, 4);
        occupiedZones.push(zone);
        occupiedZoneIndex.addZone(zone);
      });
      out.report.rings.push({
        ring: ringIndex + 1,
        radius: Number(radius.toFixed(2)),
        count: ringNodes.length,
      });
      out.report.ringCount = out.report.rings.length;
      cursor += ringNodes.length;
      radius += ringGap;
      ringIndex += 1;
    }
    return out;
  }

  function computeClusterBBox(clusterNodeIds, nodesById, positions) {
    let minX = Infinity;
    let minY = Infinity;
    let maxX = -Infinity;
    let maxY = -Infinity;
    (clusterNodeIds || []).forEach((id) => {
      const node = nodesById.get(id);
      const pos = positions.get(id);
      if (!node || !pos) return;
      const pad = nodeRadius(node) + labelPad(node);
      minX = Math.min(minX, pos.x - pad);
      minY = Math.min(minY, pos.y - pad);
      maxX = Math.max(maxX, pos.x + pad);
      maxY = Math.max(maxY, pos.y + pad);
    });
    if (!Number.isFinite(minX)) {
      return { minX: 0, minY: 0, maxX: 0, maxY: 0, width: 0, height: 0, cx: 0, cy: 0 };
    }
    return {
      minX,
      minY,
      maxX,
      maxY,
      width: maxX - minX,
      height: maxY - minY,
      cx: (minX + maxX) / 2,
      cy: (minY + maxY) / 2,
    };
  }

  function placePairHorizontal(nodes, nodeMetaById, center) {
    const sorted = (nodes || [])
      .slice()
      .sort((a, b) => {
        const pa = rolePriority(a, nodeMetaById[a.id]);
        const pb = rolePriority(b, nodeMetaById[b.id]);
        if (pa !== pb) return pa - pb;
        return String(a?.id || "").localeCompare(String(b?.id || ""), "zh-CN");
      });
    const left = sorted[0];
    const right = sorted[1];
    const gap = computePairGap(left, right, {
      minBase: 248,
      maxCap: 580,
      labelPad: 54,
      radialPad: 22,
      edgeCorridor: 146,
    });
    const out = new Map();
    if (left) out.set(left.id, { x: center.x - gap / 2, y: center.y });
    if (right) out.set(right.id, { x: center.x + gap / 2, y: center.y });
    return out;
  }

  function placeTripleCase(nodes, adjacency, center, triangle = false) {
    const rows = (nodes || []).slice();
    const out = new Map();
    if (!rows.length) return out;
    if (triangle) {
      const sorted = rows.sort((a, b) => String(a?.id || "").localeCompare(String(b?.id || ""), "zh-CN"));
      const maxCollision = sorted.reduce((max, node) => Math.max(max, nodeCollisionRadius(node)), 0);
      const radius = clampLayout(Math.max(72, maxCollision * 1.28 + 24), 72, 320);
      sorted.forEach((node, index) => {
        const angle = -Math.PI / 2 + index * ((Math.PI * 2) / 3);
        out.set(node.id, {
          x: center.x + Math.cos(angle) * radius,
          y: center.y + Math.sin(angle) * radius,
        });
      });
      return out;
    }
    const degreeRows = rows.map((node) => ({
      node,
      degree: (adjacency?.[node.id] || []).length,
    }));
    degreeRows.sort((a, b) => b.degree - a.degree || String(a.node?.id || "").localeCompare(String(b.node?.id || ""), "zh-CN"));
    const middle = degreeRows[0]?.node || rows[0];
    const others = rows.filter((node) => node.id !== middle.id).sort((a, b) =>
      String(a?.id || "").localeCompare(String(b?.id || ""), "zh-CN")
    );
    out.set(middle.id, { x: center.x, y: center.y });
    const chainGap = clampLayout(
      computePairGap(middle, others[0] || others[1], {
        minBase: 360,
        maxCap: 760,
        labelPad: 112,
        radialPad: 28,
        edgeCorridor: 320,
      }),
      340,
      740
    );
    if (others[0]) out.set(others[0].id, { x: center.x - chainGap / 2, y: center.y });
    if (others[1]) out.set(others[1].id, { x: center.x + chainGap / 2, y: center.y });
    return out;
  }

  function placeQuadSquare(nodes, nodeMetaById, center) {
    const rows = (nodes || [])
      .slice()
      .sort((a, b) => {
        const metaA = nodeMetaById?.[a?.id] || {};
        const metaB = nodeMetaById?.[b?.id] || {};
        const seedRankA = metaA?.isSeedSelected ? 0 : 1;
        const seedRankB = metaB?.isSeedSelected ? 0 : 1;
        if (seedRankA !== seedRankB) return seedRankA - seedRankB;
        const coreRankA = String(metaA?.role || "").toLowerCase() === "core" ? 0 : 1;
        const coreRankB = String(metaB?.role || "").toLowerCase() === "core" ? 0 : 1;
        if (coreRankA !== coreRankB) return coreRankA - coreRankB;
        return String(a?.id || "").localeCompare(String(b?.id || ""), "zh-CN");
      });
    const out = new Map();
    if (!rows.length) return out;
    const maxCollision = rows.reduce((max, node) => Math.max(max, nodeCollisionRadius(node)), 0);
    const halfSide = clampLayout(Math.max(84, maxCollision * 1.26 + 20), 84, 240);
    const corners = [
      { x: -halfSide, y: -halfSide },
      { x: halfSide, y: -halfSide },
      { x: halfSide, y: halfSide },
      { x: -halfSide, y: halfSide },
    ];
    rows.slice(0, 4).forEach((node, index) => {
      const slot = corners[index] || corners[0];
      out.set(node.id, {
        x: center.x + slot.x,
        y: center.y + slot.y,
      });
    });
    return out;
  }

  function buildBlockedSectorsForCenter(centerRef, centerRefs) {
    const sectors = [];
    (centerRefs || []).forEach((row) => {
      if (!row || row.id === centerRef.id) return;
      const dx = row.x - centerRef.x;
      const dy = row.y - centerRef.y;
      const dist = Math.hypot(dx, dy);
      if (!Number.isFinite(dist) || dist <= 0) return;
      if (dist > 420) return;
      const angle = Math.atan2(dy, dx);
      const half = clampLayout(146 / Math.max(58, dist), 0.2, 1.08);
      sectors.push({ angle, half });
    });
    return sectors;
  }

  function createCenterTerritoryContext(centerRefs, options = {}) {
    const centers = Array.isArray(centerRefs)
      ? centerRefs
          .filter(Boolean)
          .map((row) => ({
            id: String(row?.id || ""),
            x: Number(row?.x) || 0,
            y: Number(row?.y) || 0,
          }))
      : [];
    const bucketCount = Math.max(36, Number(options?.bucketCount) || 144);
    const fallbackRadius = Math.max(180, Number(options?.sampleRadius) || 0);
    const guard = Math.max(0.03, Math.min(0.16, Number(options?.guard) || 0.09));
    const cosLut = new Array(bucketCount);
    const sinLut = new Array(bucketCount);
    for (let i = 0; i < bucketCount; i += 1) {
      const angle = -Math.PI + ((i + 0.5) / bucketCount) * TWO_PI;
      cosLut[i] = Math.cos(angle);
      sinLut[i] = Math.sin(angle);
    }
    const minDistById = new Map();
    for (let i = 0; i < centers.length; i += 1) {
      minDistById.set(centers[i].id, Infinity);
    }
    for (let i = 0; i < centers.length; i += 1) {
      const a = centers[i];
      for (let j = i + 1; j < centers.length; j += 1) {
        const b = centers[j];
        const dx = b.x - a.x;
        const dy = b.y - a.y;
        const dist = Math.hypot(dx, dy);
        if (!Number.isFinite(dist) || dist <= 1e-3) continue;
        if (dist < (minDistById.get(a.id) || Infinity)) minDistById.set(a.id, dist);
        if (dist < (minDistById.get(b.id) || Infinity)) minDistById.set(b.id, dist);
      }
    }
    return {
      centerRefs,
      centers,
      bucketCount,
      fallbackRadius,
      guard,
      cosLut,
      sinLut,
      minDistById,
    };
  }

  function buildCenterTerritoryRanges(centerRef, centerRefs, options = {}) {
    const context =
      options?._territoryContext &&
      options._territoryContext.centerRefs === centerRefs
        ? options._territoryContext
        : createCenterTerritoryContext(centerRefs, options);
    const centers = context.centers;
    if (!centerRef || !centers.length) {
      return [{ start: -Math.PI, end: Math.PI, span: Math.PI * 2 }];
    }
    if (centers.length <= 1) return [{ start: -Math.PI, end: Math.PI, span: Math.PI * 2 }];
    const centerId = String(centerRef?.id || "");
    const cx = Number(centerRef.x) || 0;
    const cy = Number(centerRef.y) || 0;
    let hasOther = false;
    for (let i = 0; i < centers.length; i += 1) {
      if (centers[i].id !== centerId) {
        hasOther = true;
        break;
      }
    }
    if (!hasOther) return [{ start: -Math.PI, end: Math.PI, span: Math.PI * 2 }];
    const minDist = Number(context.minDistById.get(centerId));
    const fallbackRadius = context.fallbackRadius;
    const sampleRadius = Number.isFinite(minDist)
      ? Math.max(160, Math.min(fallbackRadius || 520, minDist * 0.86))
      : fallbackRadius;
    const bucketCount = context.bucketCount;
    const marks = new Array(bucketCount).fill(false);
    for (let i = 0; i < bucketCount; i += 1) {
      const px = cx + (context.cosLut[i] || 0) * sampleRadius;
      const py = cy + (context.sinLut[i] || 0) * sampleRadius;
      let nearestId = "";
      let nearestDistSq = Infinity;
      for (let ci = 0; ci < centers.length; ci += 1) {
        const row = centers[ci];
        const dx = row.x - px;
        const dy = row.y - py;
        const distSq = dx * dx + dy * dy;
        if (distSq < nearestDistSq) {
          nearestDistSq = distSq;
          nearestId = row.id;
        }
      }
      if (nearestId === centerId) marks[i] = true;
    }
    const ranges = [];
    const step = TWO_PI / bucketCount;
    let cursor = 0;
    while (cursor < bucketCount) {
      if (!marks[cursor]) {
        cursor += 1;
        continue;
      }
      const startIndex = cursor;
      while (cursor < bucketCount && marks[cursor]) cursor += 1;
      const endIndex = cursor - 1;
      const start = -Math.PI + startIndex * step;
      const end = -Math.PI + (endIndex + 1) * step;
      ranges.push({ start, end, span: end - start });
    }
    if (!ranges.length) return [{ start: -Math.PI, end: Math.PI, span: Math.PI * 2 }];
    if (marks[0] && marks[bucketCount - 1] && ranges.length > 1) {
      const first = ranges[0];
      const last = ranges[ranges.length - 1];
      ranges[0] = {
        start: last.start,
        end: first.end,
        span: first.end - last.start + TWO_PI,
      };
      ranges.pop();
    }
    const guard = context.guard;
    const guarded = ranges
      .map((row) => {
        const span = Number(row?.span) || 0;
        if (span >= Math.PI * 2 - 1e-3) return { start: -Math.PI, end: Math.PI, span: Math.PI * 2 };
        if (span <= guard * 2) return null;
        return {
          start: Number(row.start) + guard,
          end: Number(row.end) - guard,
          span: span - guard * 2,
        };
      })
      .filter(Boolean);
    return guarded.length ? guarded : [{ start: -Math.PI, end: Math.PI, span: Math.PI * 2 }];
  }

  function layoutCompactCluster(clusterData, params = {}) {
    const cfg = params?.config || DEFAULT_CFG;
    const cluster = clusterData?.cluster || {};
    const nodeMetaById = clusterData?.nodeMetaById || {};
    const nodesById = clusterData?.nodesById || new Map();
    const leafEntryById = clusterData?.leafEntryById || {};
    const center = {
      x: Number(clusterData?.placement?.x) || 0,
      y: Number(clusterData?.placement?.y) || 0,
    };
    const clusterNodeIds = sortStableIds(cluster?.nodeIds || []);
    const clusterNodes = clusterNodeIds
      .map((id) => nodesById.get(id))
      .filter(Boolean);
    const clusterNodeSet = new Set(clusterNodeIds);
    const localAdjacency = buildClusterAdjacency(cluster, clusterNodeSet);
    const coreNodes = clusterNodes.filter((node) => String(nodeMetaById?.[node.id]?.role || node?.role || "").toLowerCase() === "core");
    const adjacentNodes = clusterNodes.filter((node) => String(nodeMetaById?.[node.id]?.role || node?.role || "").toLowerCase() === "adjacent");
    const leafNodes = clusterNodes.filter((node) => String(nodeMetaById?.[node.id]?.role || node?.role || "").toLowerCase() === "leaf");
    const anchorNodes = sortStableIds(cluster?.layoutAnchorIds || [])
      .map((id) => nodesById.get(id))
      .filter(Boolean);
    const allPositions = new Map();
    const clusterReport = {
      clusterId: cluster?.clusterId || "",
      layoutCase: String(cluster?.layoutCase || "general"),
      centerType: coreNodes.length ? "core" : "layoutAnchor",
      leafLayout: "circle",
      centers: [],
      ringCount: 0,
      rings: [],
      avoidanceHits: 0,
      bbox: null,
    };

    if (clusterNodes.length === 1) {
      const only = clusterNodes[0];
      allPositions.set(only.id, { x: center.x, y: center.y });
      clusterReport.layoutCase = "single";
    } else if (clusterNodes.length === 2 || cluster.layoutCase === "pair-horizontal") {
      const pairPos = placePairHorizontal(clusterNodes, nodeMetaById, center);
      pairPos.forEach((pos, id) => allPositions.set(id, pos));
      clusterReport.layoutCase = "pair-horizontal";
    } else if (clusterNodes.length === 3 && cluster.layoutCase === "triple-chain") {
      const triplePos = placeTripleCase(clusterNodes, localAdjacency, center, false);
      triplePos.forEach((pos, id) => allPositions.set(id, pos));
    } else if (clusterNodes.length === 3 && cluster.layoutCase === "triple-triangle") {
      const triplePos = placeTripleCase(clusterNodes, localAdjacency, center, true);
      triplePos.forEach((pos, id) => allPositions.set(id, pos));
    } else if (clusterNodes.length === 4 || cluster.layoutCase === "quad-square") {
      const quadPos = placeQuadSquare(clusterNodes, nodeMetaById, center);
      quadPos.forEach((pos, id) => allPositions.set(id, pos));
      clusterReport.layoutCase = "quad-square";
    } else if (
      String(cluster?.layoutCase || "") === "single-center-circle" &&
      !coreNodes.length &&
      anchorNodes.length === 1 &&
      clusterNodes.length <= 8
    ) {
      const anchor = anchorNodes[0];
      const anchorId = String(anchor?.id || "").trim();
      if (anchorId) {
        allPositions.set(anchorId, { x: center.x, y: center.y });
      }
      const ringNodes = clusterNodes.filter((node) => String(node?.id || "").trim() !== anchorId);
      if (ringNodes.length === 4) {
        const quadPos = placeQuadSquare(ringNodes, nodeMetaById, center);
        quadPos.forEach((pos, id) => allPositions.set(id, pos));
        const sample = quadPos.values().next().value || { x: center.x, y: center.y };
        const quadRadius = Math.hypot((Number(sample.x) || 0) - center.x, (Number(sample.y) || 0) - center.y);
        clusterReport.ringCount = 1;
        clusterReport.rings.push({
          ring: 1,
          radius: Number(quadRadius.toFixed(2)),
          count: 4,
        });
        clusterReport.centers.push({
          centerId: anchorId,
          centerType: "layoutAnchor",
          leafMode: "square",
          ringCount: 1,
          rings: [{ ring: 1, radius: Number(quadRadius.toFixed(2)), count: 4 }],
          avoidanceHits: 0,
          territoryRangeCount: 1,
        });
      } else {
        const occupiedZones = anchorId ? [makeZone(anchorId, center.x, center.y, anchor, 10)] : [];
        const ringPlaced = placeLeafRings(
          {
            id: anchorId,
            x: center.x,
            y: center.y,
            node: anchor,
          },
          ringNodes,
          [],
          occupiedZones,
          {
            config: cfg,
            territoryRanges: [{ start: -Math.PI, end: Math.PI, span: TWO_PI }],
          }
        );
        ringPlaced.positions.forEach((pos, id) => allPositions.set(id, pos));
        clusterReport.ringCount = Number(ringPlaced.report?.ringCount) || 0;
        clusterReport.rings.push(...(ringPlaced.report?.rings || []));
        clusterReport.avoidanceHits = Number(ringPlaced.report?.avoidanceHits) || 0;
        clusterReport.centers.push({
          centerId: anchorId,
          centerType: "layoutAnchor",
          leafMode: "circle",
          ringCount: Number(ringPlaced.report?.ringCount) || 0,
          rings: ringPlaced.report?.rings || [],
          avoidanceHits: Number(ringPlaced.report?.avoidanceHits) || 0,
          territoryRangeCount: Number(ringPlaced.report?.territoryRangeCount) || 0,
        });
      }
    } else {
      const fallback = pickFallbackCenter(clusterNodes);
      const layoutAnchors = anchorNodes.length ? anchorNodes : fallback ? [fallback] : [];
      const backbone = placeCompactBackbone(coreNodes, adjacentNodes, layoutAnchors, {
        center,
        config: cfg,
        nodeMetaById,
        localAdjacency,
        leafCount: leafNodes.length,
      });
      backbone.positions.forEach((pos, id) => allPositions.set(id, pos));
      const centerRefs =
        backbone.centers.length > 0
          ? backbone.centers
          : layoutAnchors.map((node) => ({
              id: node.id,
              x: center.x,
              y: center.y,
              node,
            }));
      if (!centerRefs.length && fallback) {
        centerRefs.push({ id: fallback.id, x: center.x, y: center.y, node: fallback });
      }
      if (!centerRefs.length && clusterNodes.length) {
        centerRefs.push({ id: clusterNodes[0].id, x: center.x, y: center.y, node: clusterNodes[0] });
      }
      const leafBuckets = new Map();
      centerRefs.forEach((row) => leafBuckets.set(row.id, []));
      const fallbackCenterId = centerRefs[0]?.id || "";
      leafNodes.forEach((leaf) => {
        const id = leaf.id;
        let centerId = String(leafEntryById?.[id] || "");
        if (!leafBuckets.has(centerId)) {
          centerId = pickFirstStableId(localAdjacency?.[id] || [], (nid) => leafBuckets.has(nid)) || fallbackCenterId;
        }
        if (!leafBuckets.has(centerId)) leafBuckets.set(centerId, []);
        leafBuckets.get(centerId).push(leaf);
      });
      const occupiedZones = backbone.occupiedZones.slice();
      const territoryContext = createCenterTerritoryContext(centerRefs, {
        bucketCount: 168,
        guard: 0.12,
      });
      centerRefs.forEach((centerRef) => {
        const groupedLeafs = leafBuckets.get(centerRef.id) || [];
        if (!groupedLeafs.length) {
          clusterReport.centers.push({
            centerId: centerRef.id,
            centerType: coreNodes.some((node) => node.id === centerRef.id) ? "core" : "layoutAnchor",
            leafMode: "circle",
            ringCount: 0,
            rings: [],
            avoidanceHits: 0,
            territoryRangeCount: 0,
          });
          return;
        }
        const blocked = buildBlockedSectorsForCenter(centerRef, centerRefs);
        const territoryRanges = buildCenterTerritoryRanges(centerRef, centerRefs, {
          _territoryContext: territoryContext,
        });
        const leafPlaced = placeLeafRings(
          {
            id: centerRef.id,
            x: centerRef.x,
            y: centerRef.y,
            node: centerRef.node || nodesById.get(centerRef.id),
          },
          groupedLeafs,
          blocked,
          occupiedZones,
          {
            config: cfg,
            territoryRanges,
          }
        );
        leafPlaced.positions.forEach((pos, id) => allPositions.set(id, pos));
        clusterReport.ringCount += Number(leafPlaced.report?.ringCount) || 0;
        clusterReport.rings.push(...(leafPlaced.report?.rings || []));
        clusterReport.avoidanceHits += Number(leafPlaced.report?.avoidanceHits) || 0;
        clusterReport.centers.push({
          centerId: centerRef.id,
          centerType: coreNodes.some((node) => node.id === centerRef.id) ? "core" : "layoutAnchor",
          leafMode: "circle",
          ringCount: Number(leafPlaced.report?.ringCount) || 0,
            rings: leafPlaced.report?.rings || [],
            avoidanceHits: Number(leafPlaced.report?.avoidanceHits) || 0,
            territoryRangeCount: Number(leafPlaced.report?.territoryRangeCount) || 0,
          });
      });
    }

    clusterNodes.forEach((node, index) => {
      if (allPositions.has(node.id)) return;
      const angle = stableHashUnit(`compact-fallback|${cluster.clusterId || ""}|${node.id}`) * Math.PI * 2;
      const radius = 42 + index * 14;
      allPositions.set(node.id, {
        x: center.x + Math.cos(angle) * radius,
        y: center.y + Math.sin(angle) * radius,
      });
    });

    const bbox = computeClusterBBox(clusterNodeIds, nodesById, allPositions);
    clusterReport.bbox = {
      minX: Number(bbox.minX.toFixed(2)),
      maxX: Number(bbox.maxX.toFixed(2)),
      minY: Number(bbox.minY.toFixed(2)),
      maxY: Number(bbox.maxY.toFixed(2)),
      width: Number(bbox.width.toFixed(2)),
      height: Number(bbox.height.toFixed(2)),
    };
    return {
      positions: allPositions,
      bbox,
      report: clusterReport,
    };
  }

  function buildFallbackSemantic(nodes) {
    const ids = sortStableIds((nodes || []).map((node) => node?.id));
    return {
      mode: "compact",
      config: { ...DEFAULT_CFG },
      nodeMetaById: ids.reduce((acc, id) => {
        acc[id] = {
          role: "leaf",
          clusterId: "cluster-0-0",
          seedCoreCandidate: false,
          isSeedSelected: false,
          demoteReason: null,
          isPathPromotedAdjacent: false,
          isBridgeAdjacent: false,
          isHubAdjacent: false,
          layoutAnchorForCluster: false,
          why: [],
          weight: 0,
        };
        return acc;
      }, {}),
      leafEntryById: {},
      clusters: [
        {
          clusterId: "cluster-0-0",
          nodeIds: ids,
          coreIds: [],
          adjacentIds: [],
          leafIds: ids.slice(),
          layoutAnchorIds: ids.length ? [ids[0]] : [],
          layoutCase: ids.length <= 1 ? "single" : ids.length === 2 ? "pair-horizontal" : "general",
          hasCore: false,
          edgeCount: 0,
          localAdjacency: {},
        },
      ],
      clusterPlacementById: {
        "cluster-0-0": { x: 0, y: 0, width: 420, height: 320 },
      },
      reports: {},
      roleGraph: { components: [{ componentId: 0, nodeIds: ids }] },
    };
  }

  function layoutCompact(nodes, edges, focusId, opts = {}) {
    void edges;
    void focusId;
    const hints = opts?.hints && typeof opts.hints === "object" ? opts.hints : {};
    const semanticSource =
      hints?.semantic && typeof hints.semantic === "object" ? hints.semantic : buildFallbackSemantic(nodes);
    const cfg = { ...DEFAULT_CFG, ...(semanticSource?.config || {}) };
    const nodeMetaById = semanticSource?.nodeMetaById || {};
    const clusterRows = Array.isArray(semanticSource?.clusters) ? semanticSource.clusters.slice() : [];
    const clusterPlacementById = semanticSource?.clusterPlacementById || {};
    const leafEntryById = semanticSource?.leafEntryById || {};
    const clusterCaseById = {};
    clusterRows.forEach((cluster) => {
      const cid = String(cluster?.clusterId || "");
      if (!cid) return;
      clusterCaseById[cid] = String(cluster?.layoutCase || "general");
    });
    const nodesById = new Map((nodes || []).map((node) => [String(node?.id || ""), node]));

    (nodes || []).forEach((node) => {
      const id = String(node?.id || "");
      applyNodeSemanticMeta(node, nodeMetaById?.[id] || {});
      const cid = String(node?.clusterId || "");
      node.layoutCase = clusterCaseById?.[cid] || "general";
      node.layoutClusterId = cid;
    });

    const reports = [];
    const assigned = new Set();
    clusterRows
      .slice()
      .sort((a, b) => String(a?.clusterId || "").localeCompare(String(b?.clusterId || ""), "zh-CN"))
      .forEach((cluster) => {
        const clusterId = String(cluster?.clusterId || "");
        const placement = clusterPlacementById?.[clusterId] || { x: 0, y: 0 };
        const laid = layoutCompactCluster(
          {
            cluster,
            placement,
            nodeMetaById,
            nodesById,
            leafEntryById,
          },
          { config: cfg }
        );
        laid.positions.forEach((pos, id) => {
          const node = nodesById.get(id);
          if (!node) return;
          node.x = pos.x;
          node.y = pos.y;
          assigned.add(id);
        });
        if (cluster && typeof cluster === "object" && laid?.bbox) {
          cluster.bbox = {
            minX: Number(laid.bbox.minX.toFixed(2)),
            minY: Number(laid.bbox.minY.toFixed(2)),
            maxX: Number(laid.bbox.maxX.toFixed(2)),
            maxY: Number(laid.bbox.maxY.toFixed(2)),
            width: Number(laid.bbox.width.toFixed(2)),
            height: Number(laid.bbox.height.toFixed(2)),
          };
        }
        reports.push(laid.report);
      });

    const fallbackRows = (nodes || [])
      .filter((node) => !assigned.has(String(node?.id || "")))
      .sort((a, b) => String(a?.id || "").localeCompare(String(b?.id || ""), "zh-CN"));
    if (fallbackRows.length) {
      const cols = Math.max(1, Math.ceil(Math.sqrt(fallbackRows.length)));
      const spacing = 120;
      const halfW = ((cols - 1) * spacing) / 2;
      const halfH = ((Math.ceil(fallbackRows.length / cols) - 1) * spacing) / 2;
      fallbackRows.forEach((node, index) => {
        const cx = index % cols;
        const cy = Math.floor(index / cols);
        node.x = cx * spacing - halfW;
        node.y = cy * spacing - halfH;
      });
    }

    if (!hints.__layoutReport || typeof hints.__layoutReport !== "object") {
      hints.__layoutReport = {};
    }
    hints.__layoutReport.compact = {
      clusterCount: reports.length,
      clusters: reports,
      mode: "compact",
    };
    return nodes;
  }

  if (typeof engine.registerLayoutExports === "function") {
    engine.registerLayoutExports({
      layoutCompact,
      layoutCompactCluster,
      placeCompactBackbone,
      placeLeafRings,
    });
  } else {
    engine.layoutCompact = layoutCompact;
    engine.layoutCompactCluster = layoutCompactCluster;
    engine.placeCompactBackbone = placeCompactBackbone;
    engine.placeLeafRings = placeLeafRings;
  }

  const runCompact = (ctx) => {
    try {
      layoutCompact(ctx.nodes, ctx.edges, ctx.focusId, { hints: ctx?.hints || {} });
      return true;
    } catch (e) {
      if (!ctx?.hints || typeof ctx.hints !== "object") return false;
      if (!ctx.hints.__layoutReport || typeof ctx.hints.__layoutReport !== "object") {
        ctx.hints.__layoutReport = {};
      }
      ctx.hints.__layoutReport.compactError = {
        message: String(e?.message || e || ""),
      };
      return false;
    }
  };
  engine.registerLayoutModeRunner("compact", runCompact);
  engine.registerLayoutModeRunner("relation", runCompact);
})();
