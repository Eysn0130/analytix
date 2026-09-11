/* Network layout mode implementation */
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
    NETWORK_CORE_SPACING: 152,
    NETWORK_ADJ_SPACING: 118,
    NETWORK_SECTOR_MIN_ANGLE: Math.PI / 5.5,
    NETWORK_SECTOR_MAX_ANGLE: Math.PI * 1.55,
    NETWORK_SECTOR_LAYER_GAP: 68,
    NETWORK_MAX_LEAF_RADIUS: 1900,
    NETWORK_CLUSTER_MARGIN: 136,
    NETWORK_PREFER_HORIZONTAL: true,
    NETWORK_HORIZONTAL_ASPECT_THRESHOLD: 1.08,
    ANGULAR_BUCKETS: 36,
  });
  const TWO_PI = Math.PI * 2;
  const NODE_LAYOUT_METRIC_CACHE = new WeakMap();
  const NODE_LABEL_MAIN_MAX_WIDTH = 244;
  const NODE_LABEL_SUB_MAX_WIDTH = 350;
  const NODE_LABEL_SINGLE_MAX_WIDTH = 328;

  function sortStableIds(ids = []) {
    return (ids || [])
      .map((id) => String(id || "").trim())
      .filter(Boolean)
      .sort((a, b) => String(a).localeCompare(String(b), "zh-CN"));
  }

  function normalizeAnglePositive(angle) {
    const valueRaw = Number(angle);
    if (!Number.isFinite(valueRaw)) return 0;
    let value = valueRaw % TWO_PI;
    if (!Number.isFinite(value)) return 0;
    if (value < 0) value += TWO_PI;
    return value;
  }

  function angleDiff(a, b) {
    const x = normalizeAnglePositive(a);
    const y = normalizeAnglePositive(b);
    const raw = Math.abs(x - y);
    return Math.min(raw, TWO_PI - raw);
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

  function clampAngleToArc(angle, start, span) {
    const full = TWO_PI;
    const arcSpan = clampLayout(Number(span) || 0, 0, full);
    if (arcSpan >= full - 1e-6) return Number(angle) || 0;
    const base = Number(start) || 0;
    const rel = normalizeAnglePositive((Number(angle) || 0) - base);
    const clampedRel = clampLayout(rel, 0, arcSpan);
    return base + clampedRel;
  }

  function estimateTextUnits(text) {
    const raw = String(text || "");
    if (!raw) return 0;
    let units = 0;
    for (const ch of raw) {
      const cp = ch.codePointAt(0) || 0;
      if (/\s/.test(ch)) units += 0.34;
      else if (cp > 255) units += 1.03;
      else if (/[0-9]/.test(ch)) units += 0.8;
      else if (/[A-Z]/.test(ch)) units += 0.72;
      else if (/[a-z]/.test(ch)) units += 0.64;
      else units += 0.7;
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
      textDensity: Math.max(0, Math.min(1.25, (halfWidth - 18) / 116)),
    };
  }

  function getNodeLayoutMetrics(node) {
    if (!node || typeof node !== "object") {
      return { radius: 18, label: 20, collisionRadius: 38, labelHalfWidth: 24, labelBottom: 42 };
    }
    if (node.__networkPlacementMetrics) {
      const radius = Math.max(9, Number(node?.r ?? node?.nodeRadius) || 18);
      const label = Math.max(0, Number(node?.labelPad) || 0);
      const collisionRadius = Math.max(0, Number(node?.collisionRadius) || radius + label);
      return {
        radius,
        label,
        collisionRadius,
        labelHalfWidth: Math.max(0, Number(node?.labelHalfWidth) || 24),
        labelBottom: Math.max(0, Number(node?.labelBottom) || 42),
      };
    }
    const cached = NODE_LAYOUT_METRIC_CACHE.get(node);
    if (cached) return cached;
    const radius = Math.max(9, Number(node?.r) || 18);
    const profile = estimateNodeLabelProfile(node, radius);
    const nameLen = String(node?.title || node?.name || "").trim().length;
    const weighted =
      12 +
      Math.min(26, nameLen * 0.52) +
      Math.max(0, nameLen - 18) * 0.34 +
      profile.textDensity * 28 +
      Math.sqrt(Math.max(0, profile.bottom)) * 0.62;
    const label = clampLayout(weighted, 18, 124);
    const collisionRadius = Math.max(
      radius + label,
      radius + profile.bottom * 0.64,
      radius * 0.16 + profile.halfWidth * 0.9
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

  function projectNetworkNodeMetrics(nodes = []) {
    const rows = Array.isArray(nodes) ? nodes : [];
    return {
      nodeCount: rows.length,
      metrics: rows.map((node) => {
        const metrics = getNodeLayoutMetrics(node);
        return {
          id: String(node?.id || ""),
          radius: Number(metrics.radius.toFixed(2)),
          label: Number(metrics.label.toFixed(2)),
          collisionRadius: Number(metrics.collisionRadius.toFixed(2)),
          labelHalfWidth: Number(metrics.labelHalfWidth.toFixed(2)),
          labelBottom: Number(metrics.labelBottom.toFixed(2)),
        };
      }),
    };
  }

  function roundNetworkMetric(value) {
    return Number((Number(value) || 0).toFixed(2));
  }

  function projectNetworkLeafPlacementRows(leafRows = []) {
    const rows = Array.isArray(leafRows) ? leafRows : [];
    return rows
      .map((row) => {
        const node = row?.node || row;
        const id = String(row?.id || node?.id || "");
        if (!id) return null;
        const metrics = getNodeLayoutMetrics(node);
        const explicitLabelPad = Number(row?.labelPad);
        const labelPadValue = Number.isFinite(explicitLabelPad) ? explicitLabelPad : metrics.label;
        return {
          id,
          labelPad: Number.isFinite(labelPadValue) ? labelPadValue : 0,
          nodeRadius: roundNetworkMetric(metrics.radius),
          collisionRadius: roundNetworkMetric(metrics.collisionRadius),
          labelHalfWidth: roundNetworkMetric(metrics.labelHalfWidth),
          labelBottom: roundNetworkMetric(metrics.labelBottom),
        };
      })
      .filter(Boolean);
  }

  function computePairGap(leftNode, rightNode, options = {}) {
    const minBase = Math.max(222, Number(options?.minBase) || 262);
    const maxCap = Math.max(minBase + 36, Number(options?.maxCap) || 620);
    const radialNeed =
      nodeCollisionRadius(leftNode) + nodeCollisionRadius(rightNode) + Math.max(14, Number(options?.radialPad) || 24);
    const labelBase =
      nodeLabelHalfWidth(leftNode) + nodeLabelHalfWidth(rightNode) + Math.max(32, Number(options?.labelPad) || 58);
    const comfortBoost = clampLayout(labelBase * 0.26, 24, 124);
    const labelNeed = labelBase + comfortBoost;
    const edgeCorridorNeed =
      nodeRadius(leftNode) + nodeRadius(rightNode) + Math.max(118, Number(options?.edgeCorridor) || 156);
    return clampLayout(Math.max(minBase, radialNeed, labelNeed, edgeCorridorNeed), minBase, maxCap);
  }

  function resolveNetworkSectorLayerLimit(leafCount) {
    const count = Math.max(0, Number(leafCount) || 0);
    if (count >= 260) return 24;
    if (count >= 180) return 21;
    if (count >= 120) return 18;
    if (count >= 80) return 16;
    return 14;
  }

  function resolveNetworkNominalArcGap(leafCount, avgLabelPad, avgNodeRadius, avgCollision) {
    const count = Math.max(0, Number(leafCount) || 0);
    const label = Math.max(0, Number(avgLabelPad) || 0);
    const radius = Math.max(0, Number(avgNodeRadius) || 0);
    const collision = Math.max(0, Number(avgCollision) || 0);
    const minGap = count >= 160 ? 82 : count >= 80 ? 68 : 36;
    const maxGap = count >= 240 ? 190 : count >= 120 ? 168 : count >= 80 ? 142 : 118;
    return clampLayout(
      28 + label * 0.62 + radius * 0.52 + collision * 0.42 + Math.sqrt(Math.max(1, count)) * 1.15,
      minGap,
      maxGap
    );
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
    const cellSize = Math.max(18, Number(options?.cellSize) || 160);
    const invCellSize = 1 / cellSize;
    const buckets = new Map();
    const entries = [];
    let maxRadius = 0;

    const getRow = (cx) => {
      let row = buckets.get(cx);
      if (!row) {
        row = new Map();
        buckets.set(cx, row);
      }
      return row;
    };

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
      const row = getRow(cx);
      const list = row.get(cy);
      if (list) list.push(entry);
      else row.set(cy, [entry]);
      return entry;
    };

    const rows = Array.isArray(zones) ? zones : [];
    rows.forEach((zone) => addZone(zone));

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

    const queryEntries = (x, y, reach) => {
      const out = [];
      forEachNearby(x, y, reach, (entry) => {
        out.push(entry);
        return false;
      });
      return out;
    };

    return {
      addZone,
      forEachNearby,
      queryEntries,
      maxRadius: () => maxRadius,
    };
  }

  function createCorridorSpatialIndex(corridors, options = {}) {
    const cellSize = Math.max(20, Number(options?.cellSize) || 220);
    const invCellSize = 1 / cellSize;
    const buckets = new Map();
    let maxHalfWidth = 0;

    const getRow = (cx) => {
      let row = buckets.get(cx);
      if (!row) {
        row = new Map();
        buckets.set(cx, row);
      }
      return row;
    };

    const insert = (corridor) => {
      if (!corridor) return;
      const x1 = Number(corridor.x1) || 0;
      const y1 = Number(corridor.y1) || 0;
      const x2 = Number(corridor.x2) || 0;
      const y2 = Number(corridor.y2) || 0;
      const halfWidth = Math.max(0, Number(corridor.halfWidth) || 0);
      if (halfWidth > maxHalfWidth) maxHalfWidth = halfWidth;
      const minCx = Math.floor((Math.min(x1, x2) - halfWidth) * invCellSize);
      const maxCx = Math.floor((Math.max(x1, x2) + halfWidth) * invCellSize);
      const minCy = Math.floor((Math.min(y1, y2) - halfWidth) * invCellSize);
      const maxCy = Math.floor((Math.max(y1, y2) + halfWidth) * invCellSize);
      for (let cx = minCx; cx <= maxCx; cx += 1) {
        const row = getRow(cx);
        for (let cy = minCy; cy <= maxCy; cy += 1) {
          const list = row.get(cy);
          if (list) list.push(corridor);
          else row.set(cy, [corridor]);
        }
      }
    };

    (Array.isArray(corridors) ? corridors : []).forEach((row) => insert(row));

    let queryToken = 1;
    const forEachNearby = (x, y, reach, visitor) => {
      const px = Number(x) || 0;
      const py = Number(y) || 0;
      const rr = Math.max(0, Number(reach) || 0);
      if (!(rr > 0) || typeof visitor !== "function") return false;
      queryToken += 1;
      if (queryToken > 1e9) queryToken = 2;
      const token = queryToken;
      const minCx = Math.floor((px - rr) * invCellSize);
      const maxCx = Math.floor((px + rr) * invCellSize);
      const minCy = Math.floor((py - rr) * invCellSize);
      const maxCy = Math.floor((py + rr) * invCellSize);
      for (let cx = minCx; cx <= maxCx; cx += 1) {
        const rowByX = buckets.get(cx);
        if (!rowByX) continue;
        for (let cy = minCy; cy <= maxCy; cy += 1) {
          const list = rowByX.get(cy);
          if (!list || !list.length) continue;
          for (let i = 0; i < list.length; i += 1) {
            const row = list[i];
            if (row.__lastQueryToken === token) continue;
            row.__lastQueryToken = token;
            if (visitor(row)) return true;
          }
        }
      }
      return false;
    };

    return {
      forEachNearby,
      maxHalfWidth: () => maxHalfWidth,
    };
  }

  function createRayPenaltyIndex(centerX, centerY, zones, options = {}) {
    const cx = Number(centerX) || 0;
    const cy = Number(centerY) || 0;
    const ownerCenterId = String(options?.ownerCenterId || "");
    const bucketCount = Math.max(96, Math.min(720, Number(options?.bucketCount) || 240));
    const bucketStep = TWO_PI / bucketCount;
    const buckets = Array.from({ length: bucketCount }, () => []);
    let seq = 0;

    const shouldIncludeZone = (zone) => {
      if (!zone) return false;
      if (!ownerCenterId) return true;
      if (!zone?.ownerCenterId) return true;
      return String(zone.ownerCenterId) === ownerCenterId;
    };

    const addZone = (zone) => {
      if (!shouldIncludeZone(zone)) return;
      const x = Number(zone.x) || 0;
      const y = Number(zone.y) || 0;
      const anglePos = normalizeAnglePositive(Math.atan2(y - cy, x - cx));
      const radius = Math.hypot(x - cx, y - cy);
      const idxRaw = Math.floor(anglePos / bucketStep);
      const idx = Math.max(0, Math.min(bucketCount - 1, idxRaw));
      buckets[idx].push({
        seq,
        x,
        y,
        anglePos,
        radius,
      });
      seq += 1;
    };

    const rows = Array.isArray(zones) ? zones : [];
    rows.forEach((zone) => addZone(zone));

    const penaltyAt = (x, y, angleThreshold, radialThreshold, maxPenalty = Number.POSITIVE_INFINITY) => {
      const at = clampLayout(Number(angleThreshold) || 0.08, 0.02, 0.24);
      const rt = Math.max(12, Number(radialThreshold) || 34);
      const cap =
        Number.isFinite(Number(maxPenalty)) ? Math.max(0, Number(maxPenalty)) : Number.POSITIVE_INFINITY;
      const targetX = Number(x) || 0;
      const targetY = Number(y) || 0;
      const targetAngle = normalizeAnglePositive(Math.atan2(targetY - cy, targetX - cx));
      const targetRadius = Math.hypot(targetX - cx, targetY - cy);
      const baseIdx = Math.max(0, Math.min(bucketCount - 1, Math.floor(targetAngle / bucketStep)));
      const spanBuckets = Math.min(bucketCount - 1, Math.ceil(at / bucketStep) + 1);
      let penalty = 0;
      for (let offset = -spanBuckets; offset <= spanBuckets; offset += 1) {
        const idx = ((baseIdx + offset) % bucketCount + bucketCount) % bucketCount;
        const list = buckets[idx];
        if (!list || !list.length) continue;
        for (let i = 0; i < list.length; i += 1) {
          const row = list[i];
          const ad = angleDiff(targetAngle, row.anglePos);
          if (ad >= at) continue;
          const rd = Math.abs(targetRadius - row.radius);
          const angleWeight = (at - ad) / Math.max(1e-6, at);
          const radialWeight = rd < rt ? 1.85 : rd < rt * 2.1 ? 1.12 : 0.55;
          penalty += angleWeight * radialWeight * 22;
          if (penalty > cap) return penalty;
        }
      }
      return penalty;
    };

    return {
      addZone,
      penaltyAt,
    };
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

  function collidesWithZones(x, y, node, zones, padding = 8, zoneIndex = null) {
    const rows = Array.isArray(zones) ? zones : [];
    const px = Number(x) || 0;
    const py = Number(y) || 0;
    const pad = Math.max(0, Number(padding) || 0);
    const radius = nodeCollisionRadius(node);
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
    const indexedRows =
      zoneIndex && typeof zoneIndex.queryEntries === "function"
        ? zoneIndex.queryEntries(px, py, reach)
        : null;
    if (indexedRows) {
      for (const entry of indexedRows) {
        if (!entry) continue;
        const dx = entry.x - px;
        const dy = entry.y - py;
        const threshold = radius + entry.r + pad;
        if (dx * dx + dy * dy < threshold * threshold) return true;
      }
      return false;
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

  function zoneOverlapPenalty(x, y, node, zones, padding = 8, zoneIndex = null, maxPenalty = Number.POSITIVE_INFINITY) {
    const rows = Array.isArray(zones) ? zones : [];
    const px = Number(x) || 0;
    const py = Number(y) || 0;
    const pad = Math.max(0, Number(padding) || 0);
    const radius = nodeCollisionRadius(node);
    const cap = Number.isFinite(Number(maxPenalty)) ? Math.max(0, Number(maxPenalty)) : Number.POSITIVE_INFINITY;
    const maxZoneRadius = Number(zoneIndex?.maxRadius?.()) || 0;
    const reach = radius + pad + maxZoneRadius;
    let penalty = 0;
    if (zoneIndex && typeof zoneIndex.forEachNearby === "function") {
      zoneIndex.forEachNearby(
        px,
        py,
        reach,
        (entry) => {
          if (!entry) return false;
          const dx = entry.x - px;
          const dy = entry.y - py;
          const threshold = radius + entry.r + pad;
          const distSq = dx * dx + dy * dy;
          const thresholdSq = threshold * threshold;
          if (distSq < thresholdSq) {
            penalty += threshold - Math.sqrt(Math.max(0, distSq));
            if (penalty > cap) return true;
          }
          return false;
        }
      );
      return penalty;
    }
    const indexedRows =
      zoneIndex && typeof zoneIndex.queryEntries === "function"
        ? zoneIndex.queryEntries(px, py, reach)
        : null;
    if (indexedRows) {
      for (const entry of indexedRows) {
        if (!entry) continue;
        const dx = entry.x - px;
        const dy = entry.y - py;
        const threshold = radius + entry.r + pad;
        const distSq = dx * dx + dy * dy;
        const thresholdSq = threshold * threshold;
        if (distSq < thresholdSq) {
          penalty += threshold - Math.sqrt(Math.max(0, distSq));
          if (penalty > cap) return penalty;
        }
      }
      return penalty;
    }
    for (const zone of rows) {
      if (!zone) continue;
      const dx = (Number(zone.x) || 0) - px;
      const dy = (Number(zone.y) || 0) - py;
      const threshold = radius + (Number(zone.r) || 0) + pad;
      const distSq = dx * dx + dy * dy;
      const thresholdSq = threshold * threshold;
      if (distSq < thresholdSq) {
        penalty += threshold - Math.sqrt(Math.max(0, distSq));
        if (penalty > cap) return penalty;
      }
    }
    return penalty;
  }

  function rayClusterPenalty(centerX, centerY, x, y, zones, options = {}) {
    const cap =
      Number.isFinite(Number(options?.maxPenalty))
        ? Math.max(0, Number(options.maxPenalty))
        : Number.POSITIVE_INFINITY;
    if (options?.rayIndex && typeof options.rayIndex.penaltyAt === "function") {
      return options.rayIndex.penaltyAt(x, y, options?.angleThreshold, options?.radialThreshold, cap);
    }
    const rows = Array.isArray(zones) ? zones : [];
    const ownerCenterId = String(options?.ownerCenterId || "");
    const angleThreshold = clampLayout(Number(options?.angleThreshold) || 0.08, 0.02, 0.24);
    const radialThreshold = Math.max(12, Number(options?.radialThreshold) || 34);
    const cx = Number(centerX) || 0;
    const cy = Number(centerY) || 0;
    const targetAngle = Math.atan2((Number(y) || 0) - cy, (Number(x) || 0) - cx);
    const targetRadius = Math.hypot((Number(x) || 0) - cx, (Number(y) || 0) - cy);
    let penalty = 0;
    for (const zone of rows) {
      if (!zone) continue;
      if (ownerCenterId && zone?.ownerCenterId && String(zone.ownerCenterId) !== ownerCenterId) continue;
      const zx = Number(zone.x) || 0;
      const zy = Number(zone.y) || 0;
      const angle = Math.atan2(zy - cy, zx - cx);
      const radius = Math.hypot(zx - cx, zy - cy);
      const ad = angleDiff(targetAngle, angle);
      if (ad >= angleThreshold) continue;
      const rd = Math.abs(targetRadius - radius);
      const angleWeight = (angleThreshold - ad) / Math.max(1e-6, angleThreshold);
      const radialWeight = rd < radialThreshold ? 1.85 : rd < radialThreshold * 2.1 ? 1.12 : 0.55;
      penalty += angleWeight * radialWeight * 22;
      if (penalty > cap) return penalty;
    }
    return penalty;
  }

  function projectNetworkRayPenalty(payload = {}) {
    const center = payload?.center || payload || {};
    const target = payload?.target || payload || {};
    const zones = Array.isArray(payload?.zones) ? payload.zones : [];
    const ownerCenterId = String(payload?.ownerCenterId || "");
    const bucketCount = Math.max(96, Math.min(720, Number(payload?.bucketCount) || 240));
    const angleThreshold = payload?.angleThreshold;
    const radialThreshold = payload?.radialThreshold;
    const maxPenalty = payload?.maxPenalty;
    const options = {
      ownerCenterId,
      angleThreshold,
      radialThreshold,
      maxPenalty,
    };
    const centerX = Number(center?.x) || 0;
    const centerY = Number(center?.y) || 0;
    const targetX = Number(target?.x) || 0;
    const targetY = Number(target?.y) || 0;
    const rayIndex = createRayPenaltyIndex(centerX, centerY, zones, {
      ownerCenterId,
      bucketCount,
    });
    const directPenalty = rayClusterPenalty(centerX, centerY, targetX, targetY, zones, options);
    const indexedPenalty = rayClusterPenalty(centerX, centerY, targetX, targetY, zones, {
      ...options,
      rayIndex,
    });
    const includedZoneCount = zones.filter((zone) => {
      if (!zone) return false;
      if (!ownerCenterId) return true;
      if (!zone?.ownerCenterId) return true;
      return String(zone.ownerCenterId) === ownerCenterId;
    }).length;
    return {
      penalty: indexedPenalty,
      directPenalty,
      indexedPenalty,
      includedZoneCount,
      bucketCount,
    };
  }

  function projectNetworkCandidateScore(payload = {}) {
    const center = payload?.center || payload || {};
    const target = payload?.target || payload || {};
    const node = payload?.node || {};
    const occupiedZones = Array.isArray(payload?.occupiedZones)
      ? payload.occupiedZones
      : Array.isArray(payload?.zones)
      ? payload.zones
      : [];
    const hardZones = Array.isArray(payload?.hardZones) ? payload.hardZones : [];
    const hardCorridors = Array.isArray(payload?.hardCorridors) ? payload.hardCorridors : [];
    const territoryRanges = Array.isArray(payload?.territoryRanges) ? payload.territoryRanges : [];
    const ownerCenters = Array.isArray(payload?.ownerCenters) ? payload.ownerCenters : [];
    const centerX = Number(center?.x) || 0;
    const centerY = Number(center?.y) || 0;
    const x = Number(target?.x) || 0;
    const y = Number(target?.y) || 0;
    const ownerCenterId = String(payload?.ownerCenterId || center?.id || "");
    const ownedByCenter = makeCenterOwnershipChecker(
      ownerCenterId,
      centerX,
      centerY,
      ownerCenters,
      payload?.strictCenterOwnership !== false
    )(x, y);
    const { hasTerritoryConstraint, isInTerritory } = createTerritoryAngleChecker(territoryRanges);
    const angle = Math.atan2(y - centerY, x - centerX);
    const inTerritory = !hasTerritoryConstraint || isInTerritory(angle);
    const allowTerritoryOverflow = !!payload?.allowTerritoryOverflow;
    const hardPadding = Math.max(0, Number(payload?.hardPadding) || 12);
    const hardCorridorPadding = Math.max(0, Number(payload?.hardCorridorPadding) || hardPadding);
    const overlapPadding = Math.max(0, Number(payload?.overlapPadding) || 8);
    const overlapWeight = Math.max(0, Number(payload?.overlapWeight) || 7.2);
    const hardCollision = collidesWithZones(x, y, node, hardZones, hardPadding);
    const hardCorridorCollision = collidesWithCorridors(
      x,
      y,
      node,
      hardCorridors,
      hardCorridorPadding
    );
    const overlapPenalty = zoneOverlapPenalty(
      x,
      y,
      node,
      occupiedZones,
      overlapPadding
    );
    const overlapWeighted = overlapPenalty * overlapWeight;
    const rayPenalty = rayClusterPenalty(centerX, centerY, x, y, occupiedZones, {
      ownerCenterId,
      angleThreshold: payload?.rayAngleThreshold,
      radialThreshold: payload?.rayRadialThreshold,
      maxPenalty: payload?.rayMaxPenalty,
    });
    const candidatePenalty = overlapWeighted + rayPenalty;
    const rejected =
      !ownedByCenter ||
      (!inTerritory && hasTerritoryConstraint && !allowTerritoryOverflow) ||
      hardCollision ||
      hardCorridorCollision;
    return {
      ownedByCenter,
      hasTerritoryConstraint,
      inTerritory,
      hardCollision,
      hardCorridorCollision,
      overlapPenalty,
      overlapWeighted,
      rayPenalty,
      candidatePenalty,
      rejected,
    };
  }

  function projectNetworkCandidateSelection(payload = {}, options = {}) {
    const center = payload?.center || payload || {};
    const node = payload?.node || {};
    const candidates = Array.isArray(payload?.candidates) ? payload.candidates : [];
    const occupiedZones = Array.isArray(payload?.occupiedZones)
      ? payload.occupiedZones
      : Array.isArray(payload?.zones)
      ? payload.zones
      : [];
    const hardZones = Array.isArray(payload?.hardZones) ? payload.hardZones : [];
    const hardCorridors = Array.isArray(payload?.hardCorridors) ? payload.hardCorridors : [];
    const territoryRanges = Array.isArray(payload?.territoryRanges) ? payload.territoryRanges : [];
    const ownerCenters = Array.isArray(payload?.ownerCenters) ? payload.ownerCenters : [];
    const centerX = Number(center?.x) || 0;
    const centerY = Number(center?.y) || 0;
    const ownerCenterId = String(payload?.ownerCenterId || center?.id || "");
    const ownedByCenter = makeCenterOwnershipChecker(
      ownerCenterId,
      centerX,
      centerY,
      ownerCenters,
      payload?.strictCenterOwnership !== false
    );
    const { hasTerritoryConstraint, isInTerritory } = createTerritoryAngleChecker(territoryRanges);
    const allowTerritoryOverflow = !!payload?.allowTerritoryOverflow;
    const hardPadding = Math.max(0, Number(payload?.hardPadding) || 12);
    const hardCorridorPadding = Math.max(0, Number(payload?.hardCorridorPadding) || hardPadding);
    const overlapPadding = Math.max(0, Number(payload?.overlapPadding) || 8);
    const overlapWeight = Math.max(0, Number(payload?.overlapWeight) || 7.2);
    const earlyPenalty = Math.max(0, Number(payload?.earlyPenalty) || 0.08);
    const cleanRayThreshold = Math.max(0, Number(payload?.cleanRayThreshold) || 0.35);
    const hardZoneIndex = options?.hardZoneIndex || null;
    const hardCorridorIndex = options?.hardCorridorIndex || null;
    const occupiedZoneIndex = options?.occupiedZoneIndex || null;
    const rayIndex = options?.rayIndex || null;
    let best = null;
    let bestPenalty = Number.POSITIVE_INFINITY;
    let bestIndex = -1;
    let avoidanceHits = 0;
    let hardRejectHits = 0;
    let corridorRejectHits = 0;
    let territoryOverflowHits = 0;
    let checkedCount = 0;
    let acceptedReason = "";
    for (let index = 0; index < candidates.length; index += 1) {
      const candidate = candidates[index] || {};
      const x = Number(candidate?.x) || 0;
      const y = Number(candidate?.y) || 0;
      const angle = Number.isFinite(Number(candidate?.angle))
        ? Number(candidate.angle)
        : Math.atan2(y - centerY, x - centerX);
      const inTerritory = isInTerritory(angle);
      if (!inTerritory && hasTerritoryConstraint && !allowTerritoryOverflow) continue;
      if (!inTerritory && hasTerritoryConstraint && allowTerritoryOverflow && candidate?.deferTerritoryOverflow) {
        continue;
      }
      if (!ownedByCenter(x, y)) continue;
      if (collidesWithZones(x, y, node, hardZones, hardPadding, hardZoneIndex)) {
        hardRejectHits += 1;
        continue;
      }
      if (collidesWithCorridors(x, y, node, hardCorridors, hardCorridorPadding, hardCorridorIndex)) {
        corridorRejectHits += 1;
        continue;
      }
      checkedCount += 1;
      const overlapStopAt = Number.isFinite(bestPenalty)
        ? Math.max(0, bestPenalty / overlapWeight)
        : Number.POSITIVE_INFINITY;
      const overlapPenalty = zoneOverlapPenalty(
        x,
        y,
        node,
        occupiedZones,
        overlapPadding,
        occupiedZoneIndex,
        overlapStopAt
      );
      const overlapWeighted = overlapPenalty * overlapWeight;
      if (Number.isFinite(bestPenalty) && overlapWeighted >= bestPenalty) {
        avoidanceHits += 1;
        continue;
      }
      const rayStopAt = Number.isFinite(bestPenalty)
        ? Math.max(0, bestPenalty - overlapWeighted)
        : Number.POSITIVE_INFINITY;
      const rayPenalty = rayClusterPenalty(centerX, centerY, x, y, occupiedZones, {
        rayIndex,
        ownerCenterId,
        angleThreshold: payload?.rayAngleThreshold,
        radialThreshold: payload?.rayRadialThreshold,
        maxPenalty: rayStopAt,
      });
      const candidatePenalty = overlapWeighted + rayPenalty;
      if (candidatePenalty <= earlyPenalty) {
        best = { x, y, angle };
        bestPenalty = candidatePenalty;
        bestIndex = index;
        acceptedReason = "early";
        if (!inTerritory && hasTerritoryConstraint) territoryOverflowHits += 1;
        break;
      }
      avoidanceHits += 1;
      if (candidatePenalty < bestPenalty) {
        best = { x, y, angle };
        bestPenalty = candidatePenalty;
        bestIndex = index;
      }
      if (overlapPenalty <= 1e-6 && rayPenalty <= cleanRayThreshold) {
        best = { x, y, angle };
        bestPenalty = candidatePenalty;
        bestIndex = index;
        acceptedReason = "clean-ray";
        if (!inTerritory && hasTerritoryConstraint) territoryOverflowHits += 1;
        break;
      }
    }
    return {
      found: !!best,
      best,
      bestPenalty: Number.isFinite(bestPenalty) ? bestPenalty : null,
      bestIndex,
      checkedCount,
      avoidanceHits,
      hardRejectHits,
      corridorRejectHits,
      territoryOverflowHits,
      acceptedReason,
    };
  }

  function projectNetworkCandidateAttemptSelection(payload = {}, options = {}) {
    const center = payload?.center || payload || {};
    const centerX = Number(center?.x) || 0;
    const centerY = Number(center?.y) || 0;
    const sectorStart = Number(payload?.sectorStart) || 0;
    const sectorSpan = Math.max(0, Number(payload?.sectorSpan) || 0);
    const keepInSector = !!payload?.keepInSector;
    const baseAngle = Number(payload?.baseAngle) || 0;
    const targetRadius = Number(payload?.targetRadius) || 0;
    const minRadius = Math.max(0, Number(payload?.minRadius) || 0);
    const maxLeafRadius = Number.isFinite(Number(payload?.maxLeafRadius))
      ? Number(payload.maxLeafRadius)
      : Number.POSITIVE_INFINITY;
    const layerGap = Math.max(0, Number(payload?.layerGap) || 0);
    const attemptMax = Math.max(0, Math.floor(Number(payload?.attemptMax) || 0));
    const shiftStep = Math.max(0, Number(payload?.shiftStep) || 0);
    const angleJitterBase = Math.max(0, Number(payload?.angleJitterBase) || 0);
    const attemptPhaseSeed = Number(payload?.attemptPhaseSeed) || 0;
    const denseSector = !!payload?.denseSector;
    const territoryRanges = Array.isArray(payload?.territoryRanges) ? payload.territoryRanges : [];
    const allowTerritoryOverflow = !!payload?.allowTerritoryOverflow;
    const { hasTerritoryConstraint, isInTerritory } = createTerritoryAngleChecker(territoryRanges);
    const ringStep = Math.max(8, layerGap * 0.2);
    const radiusLow = minRadius * 0.78;
    const candidates = [];
    for (let attempt = 0; attempt <= attemptMax; attempt += 1) {
      const sign = attempt % 2 === 0 ? 1 : -1;
      const shift = attempt === 0 ? 0 : sign * shiftStep * Math.ceil(attempt / 2);
      const attemptPhase = (attemptPhaseSeed + attempt * 0.6180339887498949) % 1;
      const microJitter = (attemptPhase - 0.5) * 2 * angleJitterBase * (denseSector ? 1.2 : 1);
      const rawAngle = baseAngle + shift + microJitter;
      const angle = keepInSector ? clampAngleToArc(rawAngle, sectorStart, sectorSpan) : rawAngle;
      const inTerritory = isInTerritory(angle);
      if (!inTerritory && hasTerritoryConstraint && !allowTerritoryOverflow) continue;
      if (!inTerritory && hasTerritoryConstraint && allowTerritoryOverflow && attempt < 4) continue;
      const rrRaw =
        targetRadius +
        Math.floor(attempt / 5) * ringStep +
        (attemptPhase - 0.5) * ringStep * 0.6;
      const rr = clampLayout(rrRaw, radiusLow, maxLeafRadius);
      candidates.push({
        x: centerX + Math.cos(angle) * rr,
        y: centerY + Math.sin(angle) * rr,
        angle,
      });
    }
    const selection = projectNetworkCandidateSelection(
      {
        ...payload,
        candidates,
      },
      options
    );
    return {
      ...selection,
      candidateCount: candidates.length,
    };
  }

  function nearestCenterIdAt(x, y, centers) {
    const rows = Array.isArray(centers) ? centers : [];
    let nearestId = "";
    let nearestDistSq = Infinity;
    const px = Number(x) || 0;
    const py = Number(y) || 0;
    for (let i = 0; i < rows.length; i += 1) {
      const row = rows[i];
      const id = row?.__id || String(row?.id || "").trim();
      if (!id) continue;
      const cx = Number.isFinite(row?.__x) ? row.__x : Number(row?.x) || 0;
      const cy = Number.isFinite(row?.__y) ? row.__y : Number(row?.y) || 0;
      const dx = cx - px;
      const dy = cy - py;
      const distSq = dx * dx + dy * dy;
      if (!Number.isFinite(distSq)) continue;
      if (distSq < nearestDistSq) {
        nearestDistSq = distSq;
        nearestId = id;
      }
    }
    return nearestId;
  }

  function makeCenterOwnershipChecker(centerId, centerX, centerY, ownerCenters, strictCenterOwnership = true) {
    if (!strictCenterOwnership) return () => true;
    const ownId = String(centerId || "");
    const ownX = Number(centerX) || 0;
    const ownY = Number(centerY) || 0;
    const rows = Array.isArray(ownerCenters)
      ? ownerCenters
          .map((row) => {
            const id = String(row?.id || "").trim();
            if (!id) return null;
            return {
              __id: id,
              __x: Number(row?.x) || 0,
              __y: Number(row?.y) || 0,
            };
          })
          .filter(Boolean)
      : [];
    if (!rows.length) return () => true;
    if (!ownId) return (x, y) => nearestCenterIdAt(x, y, rows) === "";
    let minOtherDist = Infinity;
    for (let i = 0; i < rows.length; i += 1) {
      const row = rows[i];
      if (row.__id !== ownId) continue;
      for (let j = 0; j < rows.length; j += 1) {
        const other = rows[j];
        if (other.__id === ownId) continue;
        const dx = other.__x - ownX;
        const dy = other.__y - ownY;
        const dist = Math.hypot(dx, dy);
        if (Number.isFinite(dist) && dist > 1e-6) minOtherDist = Math.min(minOtherDist, dist);
      }
      break;
    }
    const guaranteedOwnRadiusSq = Number.isFinite(minOtherDist)
      ? Math.max(0, (minOtherDist * 0.5 - 1e-6) ** 2)
      : Infinity;
    return (x, y) => {
      const px = Number(x) || 0;
      const py = Number(y) || 0;
      const dx = px - ownX;
      const dy = py - ownY;
      const ownDistSq = dx * dx + dy * dy;
      if (ownDistSq <= guaranteedOwnRadiusSq) return true;
      return nearestCenterIdAt(px, py, rows) === ownId;
    };
  }

  function rolePriority(node, meta) {
    const role = String(meta?.role || node?.role || "").toLowerCase();
    const roleRank = role === "core" ? 0 : role === "adjacent" ? 1 : role === "leaf" ? 2 : 3;
    const seedRank = meta?.isSeedSelected || node?.isSeedSelected ? 0 : 1;
    return roleRank * 10 + seedRank;
  }

  function buildClusterAdjacency(cluster, clusterNodeSet) {
    const out = {};
    const source = cluster?.localAdjacency && typeof cluster.localAdjacency === "object" ? cluster.localAdjacency : {};
    clusterNodeSet.forEach((id) => {
      out[id] = sortStableIds((source?.[id] || []).filter((nid) => clusterNodeSet.has(nid)));
    });
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

  function rotatePoint(point, pivot, angle) {
    const px = Number(point?.x) || 0;
    const py = Number(point?.y) || 0;
    const cx = Number(pivot?.x) || 0;
    const cy = Number(pivot?.y) || 0;
    const dx = px - cx;
    const dy = py - cy;
    const cs = Math.cos(Number(angle) || 0);
    const sn = Math.sin(Number(angle) || 0);
    return {
      x: cx + dx * cs - dy * sn,
      y: cy + dx * sn + dy * cs,
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
    const out = new Map();
    const gap = computePairGap(sorted[0], sorted[1], {
      minBase: 262,
      maxCap: 620,
      labelPad: 58,
      radialPad: 24,
      edgeCorridor: 156,
    });
    if (sorted[0]) out.set(sorted[0].id, { x: center.x - gap / 2, y: center.y });
    if (sorted[1]) out.set(sorted[1].id, { x: center.x + gap / 2, y: center.y });
    return out;
  }

  function placeTriple(nodes, adjacency, center, triangle = false) {
    const rows = (nodes || []).slice();
    const out = new Map();
    if (!rows.length) return out;
    if (triangle) {
      const sorted = rows.sort((a, b) => String(a?.id || "").localeCompare(String(b?.id || ""), "zh-CN"));
      const maxCollision = sorted.reduce((max, node) => Math.max(max, nodeCollisionRadius(node)), 0);
      const radius = clampLayout(Math.max(74, maxCollision * 1.32 + 26), 74, 340);
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
        minBase: 228,
        maxCap: 540,
        labelPad: 44,
        radialPad: 20,
        edgeCorridor: 132,
      }) * 0.95,
      202,
      500
    );
    if (others[0]) out.set(others[0].id, { x: center.x - chainGap / 2, y: center.y - 14 });
    if (others[1]) out.set(others[1].id, { x: center.x + chainGap / 2, y: center.y + 14 });
    return out;
  }

  function placeQuadSquare(nodes, nodeMetaById, center) {
    const rows = (nodes || [])
      .slice()
      .sort((a, b) => {
        const pa = rolePriority(a, nodeMetaById?.[a?.id] || {});
        const pb = rolePriority(b, nodeMetaById?.[b?.id] || {});
        if (pa !== pb) return pa - pb;
        return String(a?.id || "").localeCompare(String(b?.id || ""), "zh-CN");
      });
    const out = new Map();
    if (!rows.length) return out;
    const maxCollision = rows.reduce((max, node) => Math.max(max, nodeCollisionRadius(node)), 0);
    const halfSide = clampLayout(Math.max(88, maxCollision * 1.3 + 24), 88, 260);
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

    // Keep the network skeleton on the same organic backbone as the relation layout.
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
          const pull = (dist - maxRadius) * 0.28;
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

  function resolveNetworkSkeletonScale(cfg, leafCount, adjacentCount, centerCount) {
    const spacingScale = clampLayout(
      (Number(cfg?.NETWORK_ADJ_SPACING) || 118) / Math.max(1, Number(cfg?.COMPACT_ADJ_SPACING) || 86),
      1.12,
      1.68
    );
    const densityScale = clampLayout(
      1 +
        Math.sqrt(Math.max(0, Number(leafCount) || 0)) / 11 +
        Math.sqrt(Math.max(0, Number(adjacentCount) || 0)) / 18 +
        Math.max(0, Number(centerCount) || 0) / 32,
      1,
      3.22
    );
    return clampLayout(spacingScale * densityScale, 1.36, 4.85);
  }

  function placeNetworkBackbone(coreNodes, adjacentNodes, layoutAnchors, params = {}) {
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
      out.occupiedZones.push(makeZone(node.id, center.x, center.y, node, 14));
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
      const baseSpacing = Math.max(42, Number(cfg.NETWORK_CORE_SPACING) || 152);
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
        out.occupiedZones.push(makeZone(node.id, x, y, node, 14));
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
      const skeletonScale = resolveNetworkSkeletonScale(
        cfg,
        leafCount,
        sortedAdjacent.length,
        sortedCenters.length
      );
      const occupiedZoneIndex = createZoneSpatialIndex(out.occupiedZones, { cellSize: 152 * skeletonScale });
      const minR = Math.max(22, Number(cfg.R_ADJ_MIN) || 74);
      const maxR = Math.max(minR + 16, (Number(cfg.R_ADJ_MAX) || 192) * skeletonScale);
      const coreIdSet = centerIdSet;
      const pairLaneCount = new Map();
      const coreLaneCount = new Map();
      const fallbackStep = (Math.PI * 2) / Math.max(1, sortedAdjacent.length);
      const fallbackOffset = stableHashUnit(`adj|${sortedAdjacent.map((node) => node?.id).join("|")}`) * Math.PI * 2;
      const fallbackR = clampLayout(
        minR + sortedAdjacent.length * 7 * skeletonScale + Math.sqrt(Math.max(1, leafCount)) * 14 * skeletonScale,
        minR,
        maxR + 260 * skeletonScale
      );
      const makePairKey = (a, b) => {
        const s = String(a || "").trim();
        const t = String(b || "").trim();
        if (!s || !t) return "";
        return s < t ? `${s}|${t}` : `${t}|${s}`;
      };
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
              ) * skeletonScale;
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
            let radius =
              minR +
              74 * skeletonScale +
              laneIndex * 26 * skeletonScale +
              Math.sqrt(Math.max(1, leafCount)) * 7.5 * skeletonScale;
            if (meta?.isBridgeAdjacent) radius += 30 * skeletonScale;
            if (meta?.isHubAdjacent) radius += 16 * skeletonScale;
            radius = clampLayout(radius, minR, maxR + 320 * skeletonScale);
            x = corePos.x + Math.cos(angle) * radius;
            y = corePos.y + Math.sin(angle) * radius;
          }
        }

        if (!Number.isFinite(x) || !Number.isFinite(y)) {
          const angle = fallbackOffset + index * fallbackStep;
          x = center.x + Math.cos(angle) * fallbackR;
          y = center.y + Math.sin(angle) * fallbackR;
        }

        const guardRadius = clampLayout(
          (128 + Math.sqrt(sortedAdjacent.length) * 13 + Math.sqrt(Math.max(1, leafCount)) * 2.4) *
            skeletonScale,
          140 * skeletonScale,
          560 * skeletonScale
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

        if (collidesWithZones(x, y, node, out.occupiedZones, 8 * skeletonScale, occupiedZoneIndex)) {
          const base = stableHashUnit(`adj-avoid|${id}`) * Math.PI * 2;
          for (let attempt = 1; attempt <= 12; attempt += 1) {
            const radius = (8 + attempt * 11) * skeletonScale;
            const angle = base + attempt * 0.62;
            const nx = x + Math.cos(angle) * radius;
            const ny = y + Math.sin(angle) * radius;
            if (!collidesWithZones(nx, ny, node, out.occupiedZones, 8 * skeletonScale, occupiedZoneIndex)) {
              x = nx;
              y = ny;
              break;
            }
          }
        }
        out.positions.set(node.id, { x, y });
        const zone = makeZone(node.id, x, y, node, 6 * skeletonScale);
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
        const maxShift = Math.max(10, Math.min(18, (Number(cfg.NETWORK_ADJ_SPACING) || 118) * 0.17));
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

  function buildCenterTerritoryRanges(centerRef, centerRefs, options = {}) {
    const centers = Array.isArray(centerRefs) ? centerRefs.filter(Boolean) : [];
    if (!centerRef || !centers.length) {
      return [{ start: -Math.PI, end: Math.PI, span: Math.PI * 2 }];
    }
    if (centers.length <= 1) return [{ start: -Math.PI, end: Math.PI, span: Math.PI * 2 }];
    const centerId = String(centerRef?.id || "");
    const cx = Number(centerRef.x) || 0;
    const cy = Number(centerRef.y) || 0;
    const others = centers.filter((row) => String(row?.id || "") !== centerId);
    if (!others.length) return [{ start: -Math.PI, end: Math.PI, span: Math.PI * 2 }];

    let minDist = Infinity;
    others.forEach((row) => {
      const dx = (Number(row?.x) || 0) - cx;
      const dy = (Number(row?.y) || 0) - cy;
      const dist = Math.hypot(dx, dy);
      if (Number.isFinite(dist) && dist > 1e-3) minDist = Math.min(minDist, dist);
    });
    const fallbackRadius = Math.max(200, Number(options?.sampleRadius) || 0);
    const sampleRadius = Number.isFinite(minDist)
      ? Math.max(180, Math.min(fallbackRadius || 680, minDist * 0.92))
      : fallbackRadius;
    const bucketCount = Math.max(48, Number(options?.bucketCount) || 192);
    const marks = new Array(bucketCount).fill(false);
    for (let i = 0; i < bucketCount; i += 1) {
      const angle = -Math.PI + ((i + 0.5) / bucketCount) * Math.PI * 2;
      const px = cx + Math.cos(angle) * sampleRadius;
      const py = cy + Math.sin(angle) * sampleRadius;
      let nearestId = "";
      let nearestDist = Infinity;
      centers.forEach((row) => {
        const dx = (Number(row?.x) || 0) - px;
        const dy = (Number(row?.y) || 0) - py;
        const dist = Math.hypot(dx, dy);
        if (dist < nearestDist) {
          nearestDist = dist;
          nearestId = String(row?.id || "");
        }
      });
      if (nearestId === centerId) marks[i] = true;
    }
    const ranges = [];
    const step = (Math.PI * 2) / bucketCount;
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
        span: first.end - last.start + Math.PI * 2,
      };
      ranges.pop();
    }
    const guard = Math.max(0.03, Math.min(0.2, Number(options?.guard) || 0.11));
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

  function mergeAngleSegments(segments) {
    const rows = Array.isArray(segments) ? segments.filter(Boolean) : [];
    if (!rows.length) return [];
    const sorted = rows
      .map((row) => [Number(row[0]) || 0, Number(row[1]) || 0])
      .filter((row) => row[1] > row[0])
      .sort((a, b) => a[0] - b[0]);
    if (!sorted.length) return [];
    const out = [sorted[0].slice()];
    for (let i = 1; i < sorted.length; i += 1) {
      const cur = sorted[i];
      const prev = out[out.length - 1];
      if (cur[0] <= prev[1] + 1e-6) {
        prev[1] = Math.max(prev[1], cur[1]);
      } else {
        out.push(cur.slice());
      }
    }
    return out;
  }

  function rangesToPositiveSegments(ranges) {
    const rows = Array.isArray(ranges) ? ranges.filter(Boolean) : [];
    const full = Math.PI * 2;
    const out = [];
    for (const row of rows) {
      const spanRaw = Number(row?.span);
      if (Number.isFinite(spanRaw) && spanRaw >= full - 1e-3) return [[0, full]];
      const start = normalizeAnglePositive(Number(row?.start) || 0);
      const end = normalizeAnglePositive(Number(row?.end) || 0);
      if (start <= end) {
        out.push([start, end]);
      } else {
        out.push([start, full]);
        out.push([0, end]);
      }
    }
    return mergeAngleSegments(out);
  }

  function buildAngleWindowSegments(angle, span) {
    const full = Math.PI * 2;
    const w = clampLayout(Number(span) || 0, 0, full);
    if (w >= full - 1e-6) return [[0, full]];
    const mid = normalizeAnglePositive(Number(angle) || 0);
    const start = normalizeAnglePositive(mid - w * 0.5);
    const end = normalizeAnglePositive(mid + w * 0.5);
    if (start <= end) return [[start, end]];
    return [[start, full], [0, end]];
  }

  function intersectAngleRanges(ranges, angle, span) {
    const base = rangesToPositiveSegments(ranges);
    if (!base.length) return [];
    const win = mergeAngleSegments(buildAngleWindowSegments(angle, span));
    if (!win.length) return [];
    const out = [];
    base.forEach((a) => {
      win.forEach((b) => {
        const s = Math.max(a[0], b[0]);
        const e = Math.min(a[1], b[1]);
        if (e - s > 1e-5) out.push([s, e]);
      });
    });
    return mergeAngleSegments(out)
      .map((row) => {
        const start = row[0];
        const end = Math.min(Math.PI * 2 - 1e-6, row[1]);
        const spanVal = end - start;
        if (!Number.isFinite(spanVal) || spanVal <= 1e-4) return null;
        return { start, end, span: spanVal };
      })
      .filter(Boolean);
  }

  function computeLocalOccupiedMap(clusterData, params = {}) {
    const clusterNodeIds = sortStableIds(clusterData?.cluster?.nodeIds || []);
    const nodesById = clusterData?.nodesById || new Map();
    const positions = clusterData?.positions || new Map();
    const zones = [];
    const labelZones = [];
    clusterNodeIds.forEach((id) => {
      const node = nodesById.get(id);
      const pos = positions.get(id);
      if (!node || !pos) return;
      zones.push(makeZone(id, pos.x, pos.y, node, 8));
      labelZones.push(makeZone(id, pos.x, pos.y, node, 20));
    });
    const skeletonZones = zones.filter((zone) => {
      const role = String(clusterData?.nodeMetaById?.[zone.id]?.role || "").toLowerCase();
      return role === "core" || role === "adjacent";
    });
    void params;
    return { zones, skeletonZones, labelZones };
  }

  function computeGlobalOccupiedMap(allClusterBBoxes, laidOutLeafZones, canvasBounds, params = {}) {
    const rows = Array.isArray(allClusterBBoxes) ? allClusterBBoxes : [];
    const leafZones = Array.isArray(laidOutLeafZones) ? laidOutLeafZones : [];
    const safe = Math.max(0, Number(params?.boundaryPadding) || 58);
    const bounds = canvasBounds && typeof canvasBounds === "object"
      ? {
          minX: Number.isFinite(Number(canvasBounds.minX)) ? Number(canvasBounds.minX) : -1400,
          minY: Number.isFinite(Number(canvasBounds.minY)) ? Number(canvasBounds.minY) : -1100,
          maxX: Number.isFinite(Number(canvasBounds.maxX)) ? Number(canvasBounds.maxX) : 1400,
          maxY: Number.isFinite(Number(canvasBounds.maxY)) ? Number(canvasBounds.maxY) : 1100,
        }
      : { minX: -1400, minY: -1100, maxX: 1400, maxY: 1100 };
    return {
      clusterBBoxes: rows.map((row) => ({
        ...row,
        weight: Number.isFinite(Number(row?.weight)) ? Number(row.weight) : 1,
      })),
      leafZones,
      canvasBounds: bounds,
      safeBounds: {
        minX: bounds.minX + safe,
        minY: bounds.minY + safe,
        maxX: bounds.maxX - safe,
        maxY: bounds.maxY - safe,
      },
    };
  }

  function distanceToBox(point, box) {
    const px = Number(point?.x) || 0;
    const py = Number(point?.y) || 0;
    const minX = Number(box?.minX) || 0;
    const minY = Number(box?.minY) || 0;
    const maxX = Number(box?.maxX) || 0;
    const maxY = Number(box?.maxY) || 0;
    const dx = px < minX ? minX - px : px > maxX ? px - maxX : 0;
    const dy = py < minY ? minY - py : py > maxY ? py - maxY : 0;
    return Math.hypot(dx, dy);
  }

  function distancePointToSegment(px, py, x1, y1, x2, y2) {
    const ax = Number(x1) || 0;
    const ay = Number(y1) || 0;
    const bx = Number(x2) || 0;
    const by = Number(y2) || 0;
    const dx = bx - ax;
    const dy = by - ay;
    const denom = dx * dx + dy * dy;
    if (!Number.isFinite(denom) || denom <= 1e-9) {
      return Math.hypot((Number(px) || 0) - ax, (Number(py) || 0) - ay);
    }
    const tRaw = (((Number(px) || 0) - ax) * dx + ((Number(py) || 0) - ay) * dy) / denom;
    const t = clampLayout(tRaw, 0, 1);
    const cx = ax + dx * t;
    const cy = ay + dy * t;
    return Math.hypot((Number(px) || 0) - cx, (Number(py) || 0) - cy);
  }

  function collidesWithCorridors(x, y, node, corridors, padding = 0, corridorIndex = null) {
    const rows = Array.isArray(corridors) ? corridors : [];
    if (!rows.length) return false;
    const pointX = Number(x) || 0;
    const pointY = Number(y) || 0;
    const radius = nodeRadius(node) + Math.max(8, labelPad(node) * 0.34) + Math.max(0, Number(padding) || 0);
    if (corridorIndex && typeof corridorIndex.forEachNearby === "function") {
      const hit = corridorIndex.forEachNearby(
        pointX,
        pointY,
        radius + (Number(corridorIndex.maxHalfWidth?.()) || 0) + 8,
        (row) => {
          const dist = distancePointToSegment(pointX, pointY, row?.x1, row?.y1, row?.x2, row?.y2);
          const threshold = Math.max(6, Number(row?.halfWidth) || 0) + radius;
          return dist < threshold;
        }
      );
      return !!hit;
    }
    for (const row of rows) {
      if (!row) continue;
      const dist = distancePointToSegment(pointX, pointY, row?.x1, row?.y1, row?.x2, row?.y2);
      const threshold = Math.max(6, Number(row?.halfWidth) || 0) + radius;
      if (dist < threshold) return true;
    }
    return false;
  }

  function buildCoreAdjacentCorridors(centerRefs, localAdjacency, allPositions, nodeMetaById, cfg = {}) {
    const centers = Array.isArray(centerRefs) ? centerRefs.filter(Boolean) : [];
    const out = [];
    const seen = new Set();
    centers.forEach((centerRef) => {
      const centerId = String(centerRef?.id || "").trim();
      if (!centerId) return;
      const centerPos = allPositions?.get(centerId) || centerRef;
      if (!centerPos) return;
      const neighbors = sortStableIds(localAdjacency?.[centerId] || []);
      const density = neighbors.length;
      neighbors.forEach((nid) => {
        const role = String(nodeMetaById?.[nid]?.role || "").toLowerCase();
        if (role !== "adjacent") return;
        const adjPos = allPositions?.get(nid);
        if (!adjPos) return;
        const key = centerId < nid ? `${centerId}|${nid}` : `${nid}|${centerId}`;
        if (seen.has(key)) return;
        seen.add(key);
        const x1 = Number(centerPos.x) || 0;
        const y1 = Number(centerPos.y) || 0;
        const x2 = Number(adjPos.x) || 0;
        const y2 = Number(adjPos.y) || 0;
        const len = Math.hypot(x2 - x1, y2 - y1);
        if (!Number.isFinite(len) || len < 1e-3) return;
        const halfWidth = clampLayout(
          10 + Math.sqrt(Math.max(1, density)) * 1.4 + (Number(cfg.NETWORK_ADJ_SPACING) || 118) * 0.05,
          10,
          30
        );
        out.push({
          ownerCenterId: centerId,
          adjacentId: nid,
          x1,
          y1,
          x2,
          y2,
          halfWidth,
        });
      });
    });
    return out;
  }

  function scoreAngularBucketAngleConstraints(
    angle,
    territoryRanges,
    preferredAngles,
    allowTerritoryOverflow = false
  ) {
    let territoryPenalty = 0;
    let directionBonus = 0;
    if (!isAngleInRanges(angle, territoryRanges)) {
      territoryPenalty += allowTerritoryOverflow ? 260 : 1200;
    }
    (Array.isArray(preferredAngles) ? preferredAngles : []).forEach((row) => {
      const prefAngle = Number(row?.angle);
      if (!Number.isFinite(prefAngle)) return;
      const width = clampLayout(Number(row?.width) || 1.55, 0.35, Math.PI);
      const weight = clampLayout(Number(row?.weight) || 1, -3.2, 3.2);
      const diff = angleDiff(angle, prefAngle);
      const gain = Math.max(0, 1 - diff / width);
      directionBonus += gain * weight * 64;
    });
    return {
      territoryPenalty,
      directionBonus,
      angleConstraintScore: directionBonus - territoryPenalty,
    };
  }

  function scoreAngularBucketBoundaryPenalty(x, y, safeBounds = {}) {
    let boundaryPenalty = 0;
    const px = Number(x) || 0;
    const py = Number(y) || 0;
    const safe = safeBounds || {};
    if (px < (Number(safe.minX) || -Infinity)) boundaryPenalty += ((Number(safe.minX) || 0) - px) * 0.8;
    if (px > (Number(safe.maxX) || Infinity)) boundaryPenalty += (px - (Number(safe.maxX) || 0)) * 0.8;
    if (py < (Number(safe.minY) || -Infinity)) boundaryPenalty += ((Number(safe.minY) || 0) - py) * 0.8;
    if (py > (Number(safe.maxY) || Infinity)) boundaryPenalty += (py - (Number(safe.maxY) || 0)) * 0.8;
    return boundaryPenalty;
  }

  function scoreAngularBucketHardZonePenalty(x, y, hardZones = []) {
    let hardCollisionPenalty = 0;
    const px = Number(x) || 0;
    const py = Number(y) || 0;
    (Array.isArray(hardZones) ? hardZones : []).forEach((zone) => {
      const dist = Math.hypot((Number(zone.x) || 0) - px, (Number(zone.y) || 0) - py);
      const threshold = (Number(zone.r) || 0) + 22;
      if (dist < threshold) hardCollisionPenalty += (threshold - dist) * 2.6 + 38;
    });
    return hardCollisionPenalty;
  }

  function scoreAngularBucketHardCorridorPenalty(x, y, hardCorridors = []) {
    let hardCorridorPenalty = 0;
    const px = Number(x) || 0;
    const py = Number(y) || 0;
    (Array.isArray(hardCorridors) ? hardCorridors : []).forEach((corridor) => {
      const dist = distancePointToSegment(px, py, corridor?.x1, corridor?.y1, corridor?.x2, corridor?.y2);
      const threshold = Math.max(8, Number(corridor?.halfWidth) || 0) + 20;
      if (dist < threshold) hardCorridorPenalty += (threshold - dist) * 3.2 + 44;
    });
    return hardCorridorPenalty;
  }

  function scoreAngularBucketOccupiedScore(x, y, localMap = {}, globalMap = {}) {
    const px = Number(x) || 0;
    const py = Number(y) || 0;
    const probe = { x: px, y: py };
    let blankness = 120;
    let localCollisionPenalty = 0;
    let globalCollisionPenalty = 0;
    let labelCrowdingPenalty = 0;
    (localMap?.zones || []).forEach((zone) => {
      const dist = Math.hypot((Number(zone.x) || 0) - px, (Number(zone.y) || 0) - py);
      const threshold = (Number(zone.r) || 0) + 14;
      if (dist < threshold) localCollisionPenalty += (threshold - dist) * 0.9;
      blankness += Math.min(8, Math.max(0, (dist - threshold) * 0.02));
    });
    (globalMap?.leafZones || []).forEach((zone) => {
      const dist = Math.hypot((Number(zone.x) || 0) - px, (Number(zone.y) || 0) - py);
      const threshold = (Number(zone.r) || 0) + 16;
      if (dist < threshold) globalCollisionPenalty += (threshold - dist);
    });
    (globalMap?.clusterBBoxes || []).forEach((bbox) => {
      const dist = distanceToBox(probe, bbox);
      const weight = Math.max(0.25, Math.min(2.2, Number(bbox?.weight) || 1));
      if (dist < 18) globalCollisionPenalty += (18 - dist) * 1.8 * weight;
      blankness += Math.min(12, dist * 0.04);
    });
    (localMap?.labelZones || []).forEach((zone) => {
      const dist = Math.hypot((Number(zone.x) || 0) - px, (Number(zone.y) || 0) - py);
      const threshold = (Number(zone.r) || 0) + 20;
      if (dist < threshold) labelCrowdingPenalty += (threshold - dist) * 0.85;
    });
    return {
      blankness,
      localCollisionPenalty,
      globalCollisionPenalty,
      labelCrowdingPenalty,
    };
  }

  function scoreAngularBucketScore(angle, x, y, localMap, globalMap, params = {}) {
    const occupiedScore = scoreAngularBucketOccupiedScore(x, y, localMap, globalMap);
    const blankness = occupiedScore.blankness;
    const localCollisionPenalty = occupiedScore.localCollisionPenalty;
    const globalCollisionPenalty = occupiedScore.globalCollisionPenalty;
    const boundaryPenalty = scoreAngularBucketBoundaryPenalty(x, y, globalMap?.safeBounds || {});
    const labelCrowdingPenalty = occupiedScore.labelCrowdingPenalty;
    const territoryRanges = Array.isArray(params?.territoryRanges) ? params.territoryRanges : [];
    const hardZones = Array.isArray(params?.hardZones) ? params.hardZones : [];
    const hardCorridors = Array.isArray(params?.hardCorridors) ? params.hardCorridors : [];
    const preferredAngles = Array.isArray(params?.preferredAngles) ? params.preferredAngles : [];
    const allowTerritoryOverflow = !!params?.allowTerritoryOverflow;
    const hardCollisionPenalty = scoreAngularBucketHardZonePenalty(x, y, hardZones);
    const hardCorridorPenalty = scoreAngularBucketHardCorridorPenalty(x, y, hardCorridors);
    const angleConstraints = scoreAngularBucketAngleConstraints(
      angle,
      territoryRanges,
      preferredAngles,
      allowTerritoryOverflow
    );
    const territoryPenalty = angleConstraints.territoryPenalty;
    const directionBonus = angleConstraints.directionBonus;
    const score =
      blankness -
      localCollisionPenalty -
      globalCollisionPenalty -
      boundaryPenalty -
      labelCrowdingPenalty -
      hardCollisionPenalty -
      hardCorridorPenalty -
      territoryPenalty +
      directionBonus;
    return {
      score,
      blankness,
      localCollisionPenalty,
      globalCollisionPenalty,
      boundaryPenalty,
      labelCrowdingPenalty,
      hardCollisionPenalty,
      hardCorridorPenalty,
      territoryPenalty,
      directionBonus,
    };
  }

  function buildAngularBucketSectorPayload(centerNode, leafRows = [], localMap = {}, globalMap = {}, params = {}) {
    const rows = Array.isArray(leafRows) ? leafRows : [];
    const projectedLeafRows = projectNetworkLeafPlacementRows(rows);
    const leafLabelPads = projectedLeafRows.map((row) => row.labelPad);
    return {
      config: params?.config || DEFAULT_CFG,
      center: {
        x: Number(centerNode?.x) || 0,
        y: Number(centerNode?.y) || 0,
      },
      localMap: localMap || {},
      globalMap: globalMap || {},
      hardZones: Array.isArray(params?.hardZones) ? params.hardZones : [],
      hardCorridors: Array.isArray(params?.hardCorridors) ? params.hardCorridors : [],
      territoryRanges: Array.isArray(params?.territoryRanges) ? params.territoryRanges : [],
      preferredAngles: Array.isArray(params?.preferredAngles) ? params.preferredAngles : [],
      allowTerritoryOverflow: !!params?.allowTerritoryOverflow,
      preferRingMode: !!params?.preferRingMode,
      enforceSingleSector: params?.enforceSingleSector !== false,
      preferredMultiSectorCount: Math.max(1, Math.floor(Number(params?.preferredMultiSectorCount) || 1)),
      multiSectorGap: clampLayout(Number(params?.multiSectorGap) || 0.22, 0.08, Math.PI / 2),
      leafLabelPads,
    };
  }

  function buildNetworkSectorPlacementPayload(centerNode, bucketProjection, occupiedZones = [], params = {}) {
    const centerModel = centerNode?.node || centerNode || {};
    const centerMetrics = getNodeLayoutMetrics(centerModel);
    const sourceLeafRows = Array.isArray(bucketProjection?.leafRows) ? bucketProjection.leafRows : [];
    const rawMaxLeafRadius = params?.maxLeafRadius;
    const maxLeafRadius = rawMaxLeafRadius == null ? NaN : Number(rawMaxLeafRadius);
    const scoredBuckets = Array.isArray(bucketProjection?.scoredBuckets) ? bucketProjection.scoredBuckets : [];
    return {
      config: params?.config || DEFAULT_CFG,
      center: {
        id: String(centerNode?.id || centerModel?.id || ""),
        type: String(params?.centerType || centerNode?.centerType || ""),
        x: Number(centerNode?.x) || 0,
        y: Number(centerNode?.y) || 0,
        nodeRadius: roundNetworkMetric(centerMetrics.radius),
        collisionRadius: roundNetworkMetric(centerMetrics.collisionRadius),
      },
      leafRows: projectNetworkLeafPlacementRows(sourceLeafRows),
      bucketPayload: bucketProjection?.payload || null,
      bucketProjection: {
        bucketCount: scoredBuckets.length,
        buckets: scoredBuckets,
        sectorPlan: bucketProjection?.sectorPlan || null,
      },
      occupiedZones: Array.isArray(occupiedZones) ? occupiedZones : [],
      hardZones: Array.isArray(params?.hardZones) ? params.hardZones : [],
      hardCorridors: Array.isArray(params?.hardCorridors) ? params.hardCorridors : [],
      territoryRanges: Array.isArray(params?.territoryRanges) ? params.territoryRanges : [],
      preferredAngles: Array.isArray(params?.preferredAngles) ? params.preferredAngles : [],
      ownerCenters: Array.isArray(params?.ownerCenters) ? params.ownerCenters : [],
      strictCenterOwnership: params?.strictCenterOwnership !== false,
      allowTerritoryOverflow: !!params?.allowTerritoryOverflow,
      maxLeafRadius: Number.isFinite(maxLeafRadius) ? maxLeafRadius : null,
    };
  }

  function resolveNetworkSectorPlacementCache(params = {}) {
    const direct = params?.sectorPlacementCache;
    if (
      direct &&
      typeof direct === "object" &&
      typeof direct.makeKey === "function" &&
      typeof direct.read === "function"
    ) {
      return direct;
    }
    const cache = engine?.networkSectorPlacementCache;
    if (
      cache &&
      typeof cache === "object" &&
      typeof cache.makeKey === "function" &&
      typeof cache.read === "function"
    ) {
      return cache;
    }
    return null;
  }

  function readCachedNetworkSectorPlacement(cache, payload) {
    if (!cache || !payload || typeof payload !== "object") return { key: "", sectorPlacement: null };
    const key = String(cache.makeKey(payload) || "").trim();
    if (!key) return { key: "", sectorPlacement: null };
    const sectorPlacement = cache.read(key, { source: "layout" });
    return {
      key,
      sectorPlacement:
        sectorPlacement && typeof sectorPlacement === "object" ? sectorPlacement : null,
    };
  }

  function prefetchNetworkSectorPlacement(cache, payload, key = "") {
    if (!cache || typeof cache.prefetch !== "function") return false;
    return !!cache.prefetch(payload, key);
  }

  function shouldRequireNetworkSectorPlacementCacheHit(cache, params = {}) {
    return (
      params?.requireNetworkSectorPlacementCacheHit === true ||
      (cache && typeof cache === "object" && cache.requireHit === true)
    );
  }

  function cloneNetworkSectorPlacementPayloadForReport(payload = {}) {
    if (!payload || typeof payload !== "object") return null;
    try {
      return JSON.parse(JSON.stringify(payload));
    } catch (error) {
      return null;
    }
  }

  function collectNetworkSectorPlacementPayload(params = {}, payload = {}) {
    const collector = Array.isArray(params?.sectorPlacementPayloadCollector)
      ? params.sectorPlacementPayloadCollector
      : null;
    if (!collector) return false;
    const limit = Math.max(0, Number(params?.sectorPlacementPayloadLimit) || 0);
    if (limit > 0 && collector.length >= limit) return false;
    const cloned = cloneNetworkSectorPlacementPayloadForReport(payload);
    if (!cloned) return false;
    collector.push({
      centerId: String(cloned?.center?.id || ""),
      leafCount: Array.isArray(cloned?.leafRows) ? cloned.leafRows.length : 0,
      payload: cloned,
    });
    return true;
  }

  function makeNetworkPlacementMetricNode(row) {
    const item = row || {};
    const radius = Math.max(9, Number(item.nodeRadius) || 18);
    const labelPadValue = Math.max(0, Number(item.labelPad) || 0);
    return {
      id: String(item.id || ""),
      r: radius,
      __networkPlacementMetrics: true,
      labelPad: labelPadValue,
      collisionRadius: Math.max(0, Number(item.collisionRadius) || radius + labelPadValue),
      labelHalfWidth: Math.max(0, Number(item.labelHalfWidth) || 24),
      labelBottom: Math.max(0, Number(item.labelBottom) || 42),
    };
  }

  function readGeneralBatchIndex(value, fallback = 0) {
    const number = Number(value);
    if (!Number.isFinite(number)) return fallback;
    return Math.max(0, Math.floor(number));
  }

  function planNetworkGeneralBatch(payload = {}) {
    const cfg = payload?.config || DEFAULT_CFG;
    const center = payload?.center || {};
    const centerId = String(center?.id || "");
    const centerX = Number(center?.x) || 0;
    const centerY = Number(center?.y) || 0;
    const leafRows = Array.isArray(payload?.leafRows) ? payload.leafRows : [];
    const sectorPlan = payload?.bucketProjection?.sectorPlan || null;
    const sectors = Array.isArray(sectorPlan?.sectors) ? sectorPlan.sectors : [];
    if (!leafRows.length || !sectorPlan || !sectors.length) return null;
    const leafCount = Math.max(0, Number(sectorPlan.leafCount) || leafRows.length);
    if (leafCount >= 36 && sectors.length === 1) {
      const ownerCenters = Array.isArray(payload?.ownerCenters) ? payload.ownerCenters : [];
      const span = Math.max(0, Number(sectors[0]?.end) - Number(sectors[0]?.start));
      if (!(payload?.strictCenterOwnership !== false && ownerCenters.length > 1) && span > 0.12) {
        return null;
      }
    }
    const avgLabelPad =
      Number(sectorPlan.avgLabelPad) ||
      leafRows.reduce((sum, row) => sum + (Number(row?.labelPad) || 0), 0) /
        Math.max(1, leafRows.length);
    const denseSector = !!sectorPlan.denseSector;
    const minRadiusBase = Math.max(80, Number(sectorPlan.minRadiusBase) || Number(cfg.R_LEAF_MIN) || 220);
    const minRadiusBoost = Math.max(1, Math.min(2.2, 1 + Math.sqrt(leafCount) / 15));
    const denseRadiusScale = denseSector
      ? clampLayout(1 - Math.sqrt(Math.max(1, leafCount)) / 26, 0.58, 0.86)
      : 1;
    const multiSectorRadiusScale = sectors.length > 1 ? clampLayout(1 - (sectors.length - 1) * 0.08, 0.72, 1) : 1;
    const minRadius = minRadiusBase * minRadiusBoost * denseRadiusScale * multiSectorRadiusScale;
    const maxLeafRadiusInput = Number(payload?.maxLeafRadius);
    const maxLeafRadius =
      payload?.maxLeafRadius != null && Number.isFinite(maxLeafRadiusInput)
        ? Math.max(minRadius + 24, maxLeafRadiusInput)
        : Number.POSITIVE_INFINITY;
    const layerGapBase = Math.max(22, Number(cfg.NETWORK_SECTOR_LAYER_GAP) || 68);
    const layerGapDenseBoost = denseSector
      ? clampLayout(1 + Math.sqrt(Math.max(1, leafCount)) / 12.5, 1.26, 2.4)
      : 1;
    const layerGapSectorBoost = sectors.length > 1 ? clampLayout(1 + sectors.length * 0.08, 1, 1.45) : 1;
    const layerGap =
      layerGapBase *
      Math.max(1, Math.min(1.9, 1 + Math.sqrt(leafCount) / 22)) *
      layerGapDenseBoost *
      layerGapSectorBoost;
    const minNodeGapBase = Math.max(36, Math.min(96, 34 + Math.sqrt(leafCount) * 2.2));
    const labelGapBoost = clampLayout(0.92 + avgLabelPad / 36, 0.94, 1.86);
    const minNodeGap = minNodeGapBase * labelGapBoost;
    const targetLayerCount = denseSector
      ? Math.max(5, Math.ceil(Math.sqrt(Math.max(1, leafCount)) / (sectors.length > 1 ? 1.35 : 1.55)))
      : 0;
    const layerCapacityCap = denseSector
      ? Math.max(4, Math.ceil(leafCount / Math.max(1, targetLayerCount)))
      : Number.POSITIVE_INFINITY;
    const batch = payload?.generalBatch || {};
    const leafStartIndex = Math.min(
      leafRows.length,
      readGeneralBatchIndex(batch?.startIndex ?? payload?.generalBatchStartIndex, 0)
    );
    const layer = readGeneralBatchIndex(batch?.layer ?? payload?.generalBatchLayer, 0);
    const s = Math.min(
      sectors.length - 1,
      readGeneralBatchIndex(batch?.sectorIndex ?? payload?.generalBatchSectorIndex, 0)
    );
    const remaining = Math.max(0, leafRows.length - leafStartIndex);
    if (remaining <= 0) return null;
    const sector = sectors[s];
    const span = Math.max(0.24, Number(sector?.end) - Number(sector?.start));
    const keepInSector = denseSector || leafCount >= 24;
    const radiusRaw = minRadius + layer * layerGap;
    const radius = Math.min(radiusRaw, maxLeafRadius);
    const atRadiusCap = radiusRaw >= maxLeafRadius - 1e-6;
    const effectiveNodeGap = atRadiusCap ? Math.max(24, minNodeGap * 0.82) : minNodeGap;
    const rawCapacity = Math.max(2, Math.floor((span * radius) / effectiveNodeGap));
    const sectorsLeft = Math.max(1, sectors.length - s);
    const fairCapScale = denseSector && sectors.length > 1 ? 0.94 : 1.02;
    const fairCap = sectors.length > 1 ? Math.max(2, Math.ceil((remaining / sectorsLeft) * fairCapScale)) : remaining;
    const capacity = Math.max(2, Math.min(rawCapacity, layerCapacityCap, fairCap));
    const take = Math.min(capacity, remaining);
    if (take <= 0) return null;
    const items = leafRows
      .slice(leafStartIndex, leafStartIndex + take)
      .sort((a, b) => {
        const labelDiff = (Number(b?.labelPad) || 0) - (Number(a?.labelPad) || 0);
        if (labelDiff) return labelDiff;
        return String(a?.id || "").localeCompare(String(b?.id || ""), "zh-CN");
      });
    if (!items.length) return null;
    return {
      payload,
      centerId,
      centerX,
      centerY,
      leafCount,
      denseSector,
      minRadius,
      maxLeafRadius,
      layerGap,
      radius,
      layer,
      sectorIndex: s,
      leafStartIndex,
      sector,
      span,
      keepInSector,
      take,
      items,
    };
  }

  function planNetworkGeneralBatchAttempt(plan, item, index) {
    if (!plan || !item) return null;
    const {
      centerId,
      denseSector,
      minRadius,
      maxLeafRadius,
      layerGap,
      radius,
      layer,
      sectorIndex,
      sector,
      span,
      keepInSector,
      take,
    } = plan;
    const node = item?.node || makeNetworkPlacementMetricNode(item);
    const phaseSeed = stableHashUnit(`${centerId}|layer|${layer}|${sectorIndex}`);
    const layerPhase = denseSector
      ? (layer * 0.5 + sectorIndex * 0.19 + phaseSeed * 0.12) % 1
      : (phaseSeed * 0.35 + layer * 0.19 + sectorIndex * 0.11) % 1;
    const normalizedRatio = take <= 1 ? 0.5 : ((index + 0.5 + layerPhase) / take) % 1;
    const angleJitterBase = clampLayout(span / Math.max(18, take * 1.42), 0.018, denseSector ? 0.13 : 0.09);
    const baseAngleNoise =
      (stableHashUnit(`sector-angle|${centerId}|${node.id}|${layer}|${sectorIndex}`) - 0.5) *
      2 *
      angleJitterBase;
    const baseAngleRaw = Number(sector?.start) + span * normalizedRatio + baseAngleNoise;
    const baseAngle = keepInSector ? clampAngleToArc(baseAngleRaw, sector?.start, span) : baseAngleRaw;
    const radialSpread = Math.min(52, layerGap * 0.42);
    const radialJitterRange = denseSector ? radialSpread : Math.min(24, layerGap * 0.2);
    const radialJitter =
      (stableHashUnit(`sector-radial|${centerId}|${node.id}|${layer}`) - 0.5) * radialJitterRange;
    const targetRadius = clampLayout(radius + radialJitter, minRadius * 0.78, maxLeafRadius);
    const attemptMax = denseSector ? (take > 24 ? 30 : 24) : take > 14 ? 18 : 12;
    const shiftStep = clampLayout(span / Math.max(10, take * 0.9), 0.045, denseSector ? 0.2 : 0.16);
    const rayAngleThreshold = clampLayout(
      span / Math.max(18, take * (denseSector ? 1.22 : 1.08)),
      0.022,
      denseSector ? 0.1 : 0.13
    );
    return {
      node,
      baseAngle,
      targetRadius,
      attemptMax,
      shiftStep,
      angleJitterBase,
      attemptPhaseSeed: stableHashUnit(`sector-attempt|${centerId}|${node.id}|${layer}|${sectorIndex}`),
      rayAngleThreshold,
      rayRadialThreshold: Math.max(20, (Number(item.labelPad) || 0) * 1.45),
    };
  }

  function projectNetworkGeneralBatchCandidate(plan, item, index, occupiedZones, options = {}) {
    const attempt = planNetworkGeneralBatchAttempt(plan, item, index);
    if (!plan || !attempt) return null;
    const { payload, centerId, centerX, centerY, minRadius, maxLeafRadius, layerGap, sector, span, keepInSector } = plan;
    return projectNetworkCandidateAttemptSelection(
      {
        center: { id: centerId, x: centerX, y: centerY },
        node: attempt.node,
        occupiedZones: Array.isArray(occupiedZones) ? occupiedZones : [],
        hardZones: Array.isArray(payload?.hardZones) ? payload.hardZones : [],
        hardCorridors: Array.isArray(payload?.hardCorridors) ? payload.hardCorridors : [],
        territoryRanges: Array.isArray(payload?.territoryRanges) ? payload.territoryRanges : [],
        ownerCenters: Array.isArray(payload?.ownerCenters) ? payload.ownerCenters : [],
        ownerCenterId: centerId,
        strictCenterOwnership: payload?.strictCenterOwnership !== false,
        allowTerritoryOverflow: !!payload?.allowTerritoryOverflow,
        sectorStart: Number(sector?.start) || 0,
        sectorSpan: span,
        keepInSector,
        baseAngle: attempt.baseAngle,
        targetRadius: attempt.targetRadius,
        minRadius,
        maxLeafRadius,
        layerGap,
        attemptMax: attempt.attemptMax,
        shiftStep: attempt.shiftStep,
        angleJitterBase: attempt.angleJitterBase,
        attemptPhaseSeed: attempt.attemptPhaseSeed,
        denseSector: plan.denseSector,
        hardPadding: 12,
        hardCorridorPadding: 12,
        overlapPadding: 8,
        overlapWeight: 7.2,
        rayAngleThreshold: attempt.rayAngleThreshold,
        rayRadialThreshold: attempt.rayRadialThreshold,
        earlyPenalty: 0.08,
        cleanRayThreshold: 0.35,
      },
      options
    );
  }

  function projectNetworkGeneralNodePlacement(plan, item, index, occupiedZones, options = {}) {
    const attempt = planNetworkGeneralBatchAttempt(plan, item, index);
    const selection = projectNetworkGeneralBatchCandidate(plan, item, index, occupiedZones, options);
    if (!plan || !attempt || !selection) {
      return {
        selection: selection || null,
        best: null,
        fallbackPlacement: false,
        pushoutPlacement: false,
        territoryOverflowHits: 0,
      };
    }
    const { payload, centerId, centerX, centerY, minRadius, maxLeafRadius, layerGap, sector, span, keepInSector } = plan;
    const hardZones = Array.isArray(payload?.hardZones) ? payload.hardZones : [];
    const hardCorridors = Array.isArray(payload?.hardCorridors) ? payload.hardCorridors : [];
    const territoryRanges = Array.isArray(payload?.territoryRanges) ? payload.territoryRanges : [];
    const preferredAngles = Array.isArray(payload?.preferredAngles) ? payload.preferredAngles : [];
    const ownerCenters = Array.isArray(payload?.ownerCenters) ? payload.ownerCenters : [];
    const allowTerritoryOverflow = !!payload?.allowTerritoryOverflow;
    const strictCenterOwnership = payload?.strictCenterOwnership !== false;
    const { hasTerritoryConstraint, isInTerritory } = createTerritoryAngleChecker(territoryRanges);
    const ownedByCenter = makeCenterOwnershipChecker(
      centerId,
      centerX,
      centerY,
      ownerCenters,
      strictCenterOwnership
    );
    let best = selection.best || null;
    const bestPenalty = Number.isFinite(Number(selection.bestPenalty))
      ? Number(selection.bestPenalty)
      : Number.POSITIVE_INFINITY;
    let fallbackPlacement = false;
    let pushoutPlacement = false;
    let territoryOverflowHits = 0;

    if (!best) {
      const prefAngle = Number(preferredAngles?.[0]?.angle);
      const fallbackBase = Number.isFinite(prefAngle) && !keepInSector ? prefAngle : attempt.baseAngle;
      let fallbackAngle =
        hasTerritoryConstraint && !allowTerritoryOverflow && !isInTerritory(fallbackBase)
          ? pickRepresentativeAngle(territoryRanges, fallbackBase)
          : fallbackBase;
      if (keepInSector) fallbackAngle = clampAngleToArc(fallbackAngle, sector.start, span);
      let fallbackBest = null;
      const aaMax = plan.denseSector ? 96 : 48;
      const golden = 0.6180339887498949;
      const fallbackPhase = stableHashUnit(
        `sector-fallback|${centerId}|${attempt.node.id}|${plan.layer}|${plan.sectorIndex}`
      );
      const fallbackSweep = keepInSector
        ? span
        : hasTerritoryConstraint && !allowTerritoryOverflow
        ? Math.max(span * 1.04, Math.PI * 0.66)
        : Math.PI * 2;
      for (let ra = 0; ra <= 14; ra += 1) {
        const rrRaw = attempt.targetRadius + ra * Math.max(10, Number(payload?.config?.NETWORK_SECTOR_LAYER_GAP) * 0.18 || 12);
        const rr = clampLayout(rrRaw, minRadius * 0.78, maxLeafRadius);
        for (let aa = 0; aa <= aaMax; aa += 1) {
          const t = (fallbackPhase + aa * golden) % 1;
          const shift = (t - 0.5) * fallbackSweep;
          const angle = keepInSector ? sector.start + t * span : fallbackAngle + shift;
          const inTerritory = isInTerritory(angle);
          if (!inTerritory && hasTerritoryConstraint && !allowTerritoryOverflow) continue;
          if (!inTerritory && hasTerritoryConstraint && allowTerritoryOverflow && ra < 4) continue;
          const x = centerX + Math.cos(angle) * rr;
          const y = centerY + Math.sin(angle) * rr;
          if (!ownedByCenter(x, y)) continue;
          if (collidesWithZones(x, y, attempt.node, hardZones, 10, options?.hardZoneIndex || null)) continue;
          if (collidesWithCorridors(x, y, attempt.node, hardCorridors, 10, options?.hardCorridorIndex || null)) continue;
          const overlapPenalty = zoneOverlapPenalty(
            x,
            y,
            attempt.node,
            occupiedZones,
            8,
            options?.occupiedZoneIndex || null,
            1e-6
          );
          if (overlapPenalty > 1e-6) continue;
          const rayPenalty = rayClusterPenalty(centerX, centerY, x, y, occupiedZones, {
            rayIndex: options?.rayIndex || null,
            ownerCenterId: centerId,
            angleThreshold: attempt.rayAngleThreshold,
            radialThreshold: attempt.rayRadialThreshold,
            maxPenalty: 1.2,
          });
          if (rayPenalty > 1.2) continue;
          fallbackBest = { x, y, angle };
          if (!inTerritory && hasTerritoryConstraint) territoryOverflowHits += 1;
          break;
        }
        if (fallbackBest) break;
      }
      if (!fallbackBest) {
        let safest = null;
        let safestPenalty = Number.POSITIVE_INFINITY;
        const sampleCount = keepInSector ? 120 : 80;
        const baseRadius = clampLayout(attempt.targetRadius, minRadius * 0.78, maxLeafRadius);
        for (let aa = 0; aa < sampleCount; aa += 1) {
          const t = (aa + 0.5) / sampleCount;
          const angle = keepInSector ? sector.start + t * span : fallbackAngle + (t - 0.5) * Math.PI * 2;
          const x = centerX + Math.cos(angle) * baseRadius;
          const y = centerY + Math.sin(angle) * baseRadius;
          if (!ownedByCenter(x, y)) continue;
          if (collidesWithZones(x, y, attempt.node, hardZones, 10, options?.hardZoneIndex || null)) continue;
          if (collidesWithCorridors(x, y, attempt.node, hardCorridors, 10, options?.hardCorridorIndex || null)) continue;
          const overlapPenalty = zoneOverlapPenalty(
            x,
            y,
            attempt.node,
            occupiedZones,
            8,
            options?.occupiedZoneIndex || null,
            Number.isFinite(safestPenalty) ? Math.max(0, safestPenalty / 7.2) : Number.POSITIVE_INFINITY
          );
          const overlapWeighted = overlapPenalty * 7.2;
          if (Number.isFinite(safestPenalty) && overlapWeighted >= safestPenalty) continue;
          const rayPenalty = rayClusterPenalty(centerX, centerY, x, y, occupiedZones, {
            rayIndex: options?.rayIndex || null,
            ownerCenterId: centerId,
            angleThreshold: attempt.rayAngleThreshold,
            radialThreshold: attempt.rayRadialThreshold,
            maxPenalty: Number.isFinite(safestPenalty)
              ? Math.max(0, safestPenalty - overlapWeighted)
              : Number.POSITIVE_INFINITY,
          });
          const totalPenalty = overlapWeighted + rayPenalty;
          if (totalPenalty < safestPenalty) {
            safest = { x, y, angle };
            safestPenalty = totalPenalty;
            if (totalPenalty <= 0.08) break;
          }
        }
        fallbackBest = safest;
      }
      best = fallbackBest || best;
      if (!best) {
        let finalAngle = keepInSector ? clampAngleToArc(fallbackAngle, sector.start, span) : fallbackAngle;
        const finalRadius = clampLayout(attempt.targetRadius, minRadius * 0.78, maxLeafRadius);
        let finalX = centerX + Math.cos(finalAngle) * finalRadius;
        let finalY = centerY + Math.sin(finalAngle) * finalRadius;
        if (!ownedByCenter(finalX, finalY)) {
          for (let aa = 0; aa < 180; aa += 1) {
            const t = (aa + 0.5) / 180;
            const angle = keepInSector ? sector.start + t * span : finalAngle + (t - 0.5) * Math.PI * 2;
            const x = centerX + Math.cos(angle) * finalRadius;
            const y = centerY + Math.sin(angle) * finalRadius;
            if (!ownedByCenter(x, y)) continue;
            finalAngle = angle;
            finalX = x;
            finalY = y;
            break;
          }
        }
        best = { x: finalX, y: finalY, angle: finalAngle };
      }
      fallbackPlacement = !!best;
    } else if (bestPenalty > 0.08) {
      let pushed = null;
      const radialStep = Math.max(10, layerGap * 0.22);
      for (let step = 1; step <= 16; step += 1) {
        const rrRaw = attempt.targetRadius + step * radialStep;
        const rr = clampLayout(rrRaw, minRadius * 0.78, maxLeafRadius);
        const swing = clampLayout(attempt.shiftStep * (1.9 + step * 0.35), 0.05, span * 0.46);
        const candidateAngles = [best.angle, best.angle + swing, best.angle - swing];
        for (const aRaw of candidateAngles) {
          const angle = keepInSector ? clampAngleToArc(aRaw, sector.start, span) : aRaw;
          const x = centerX + Math.cos(angle) * rr;
          const y = centerY + Math.sin(angle) * rr;
          if (!ownedByCenter(x, y)) continue;
          if (collidesWithZones(x, y, attempt.node, hardZones, 10, options?.hardZoneIndex || null)) continue;
          if (collidesWithCorridors(x, y, attempt.node, hardCorridors, 10, options?.hardCorridorIndex || null)) continue;
          const overlapPenalty = zoneOverlapPenalty(
            x,
            y,
            attempt.node,
            occupiedZones,
            8,
            options?.occupiedZoneIndex || null,
            1e-6
          );
          if (overlapPenalty > 1e-6) continue;
          const rayPenalty = rayClusterPenalty(centerX, centerY, x, y, occupiedZones, {
            rayIndex: options?.rayIndex || null,
            ownerCenterId: centerId,
            angleThreshold: attempt.rayAngleThreshold,
            radialThreshold: attempt.rayRadialThreshold,
            maxPenalty: 1.2,
          });
          if (rayPenalty > 1.2) continue;
          pushed = { x, y, angle };
          break;
        }
        if (pushed) break;
      }
      if (pushed) {
        best = pushed;
        pushoutPlacement = true;
      }
    }

    return { selection, best, fallbackPlacement, pushoutPlacement, territoryOverflowHits };
  }

  function projectNetworkGeneralAttemptSelection(payload = {}) {
    const plan = planNetworkGeneralBatch(payload);
    if (!plan) return null;
    return projectNetworkGeneralBatchCandidate(
      plan,
      plan.items[0],
      0,
      Array.isArray(payload?.occupiedZones) ? payload.occupiedZones : []
    );
  }

  function projectNetworkGeneralBatchPlacementFromPlan(plan, occupiedZonesInput = [], options = {}) {
    if (!plan) return null;
    const sourceZones = Array.isArray(occupiedZonesInput) ? occupiedZonesInput : [];
    const occupiedZones =
      options?.cloneOccupiedZones === false
        ? sourceZones
        : options?.cloneOccupiedZones === "refs"
        ? sourceZones.slice()
        : sourceZones.map((zone) => ({ ...zone }));
    const includeUpdates = options?.includeUpdates !== false;
    const updates = [];
    const leafZones = [];
    const report = {
      avoidanceHits: 0,
      hardRejectHits: 0,
      corridorRejectHits: 0,
      territoryOverflowHits: 0,
      fallbackPlacements: 0,
      pushoutPlacements: 0,
    };
    let completed = true;
    let stoppedAt = null;

    plan.items.forEach((item, index) => {
      if (!completed) return;
      const placement = projectNetworkGeneralNodePlacement(plan, item, index, occupiedZones, options);
      const selection = placement.selection;
      report.avoidanceHits += Number(selection?.avoidanceHits) || 0;
      report.hardRejectHits += Number(selection?.hardRejectHits) || 0;
      report.corridorRejectHits += Number(selection?.corridorRejectHits) || 0;
      report.territoryOverflowHits += Number(selection?.territoryOverflowHits) || 0;
      report.territoryOverflowHits += Number(placement.territoryOverflowHits) || 0;
      report.fallbackPlacements = (Number(report.fallbackPlacements) || 0) + (placement.fallbackPlacement ? 1 : 0);
      report.pushoutPlacements = (Number(report.pushoutPlacements) || 0) + (placement.pushoutPlacement ? 1 : 0);
      const best = placement.best || null;
      if (!best) {
        completed = false;
        stoppedAt = index;
        return;
      }
      const node = item?.node || makeNetworkPlacementMetricNode(item);
      const zone = {
        ...makeZone(node.id, best.x, best.y, node, 4),
        ownerCenterId: plan.centerId,
        ownerSectorIndex: plan.sectorIndex,
      };
      occupiedZones.push(zone);
      if (typeof options?.onZone === "function") options.onZone(zone);
      leafZones.push(zone);
      if (includeUpdates) {
        updates.push({
          id: node.id,
          x: best.x,
          y: best.y,
          ownerCenterId: plan.centerId,
        });
      }
    });

    return {
      layer: plan.layer,
      sectorIndex: plan.sectorIndex,
      leafStartIndex: plan.leafStartIndex,
      take: plan.take,
      completed,
      stoppedAt,
      updates,
      leafZones,
      report,
    };
  }

  function projectNetworkGeneralBatchPlacement(payload = {}) {
    const plan = planNetworkGeneralBatch(payload);
    return projectNetworkGeneralBatchPlacementFromPlan(plan, payload?.occupiedZones, { cloneOccupiedZones: true });
  }

  function projectNetworkSectorPlacementFromPlan(payload = {}, options = {}) {
    const leafRows = Array.isArray(payload?.leafRows) ? payload.leafRows : [];
    const sectorPlan = payload?.bucketProjection?.sectorPlan || null;
    const sectors = Array.isArray(sectorPlan?.sectors) ? sectorPlan.sectors : [];
    const firstPlan = planNetworkGeneralBatch(payload);
    if (!firstPlan || !leafRows.length || !sectors.length) return null;
    const includeBatches = options?.includeBatches !== false && payload?.includeGeneralBatches !== false;
    const includeUpdates = options?.includeUpdates !== false && payload?.includeGeneralUpdates !== false;
    const sourceZones = Array.isArray(payload?.occupiedZones) ? payload.occupiedZones : [];
    const occupiedZones =
      options?.cloneOccupiedZones === false
        ? sourceZones
        : options?.cloneOccupiedZones === "refs"
        ? sourceZones.slice()
        : sourceZones.map((zone) => ({ ...zone }));
    const batches = [];
    const updates = [];
    const leafZones = [];
    const report = {
      avoidanceHits: 0,
      hardRejectHits: 0,
      corridorRejectHits: 0,
      territoryOverflowHits: 0,
      fallbackPlacements: 0,
      pushoutPlacements: 0,
    };
    let completed = true;
    let stoppedAt = null;
    let leafStartIndex = firstPlan.leafStartIndex;
    let layer = firstPlan.layer;
    let sectorIndex = firstPlan.sectorIndex;
    let batchIndex = 0;

    while (leafStartIndex < leafRows.length) {
      const plan =
        batchIndex === 0
          ? firstPlan
          : planNetworkGeneralBatch({
              ...payload,
              generalBatch: {
                startIndex: leafStartIndex,
                layer,
                sectorIndex,
              },
            });
      if (!plan) {
        completed = false;
        stoppedAt = batchIndex;
        break;
      }
      const batch = projectNetworkGeneralBatchPlacementFromPlan(plan, occupiedZones, {
        ...options,
        cloneOccupiedZones: false,
        includeUpdates,
      });
      if (!batch) {
        completed = false;
        stoppedAt = batchIndex;
        break;
      }
      if (includeBatches) batches.push(batch);
      if (includeUpdates) updates.push(...(batch.updates || []));
      leafZones.push(...(batch.leafZones || []));
      const batchReport = batch.report || {};
      report.avoidanceHits += Number(batchReport.avoidanceHits) || 0;
      report.hardRejectHits += Number(batchReport.hardRejectHits) || 0;
      report.corridorRejectHits += Number(batchReport.corridorRejectHits) || 0;
      report.territoryOverflowHits += Number(batchReport.territoryOverflowHits) || 0;
      report.fallbackPlacements += Number(batchReport.fallbackPlacements) || 0;
      report.pushoutPlacements += Number(batchReport.pushoutPlacements) || 0;
      if (!batch.completed) {
        completed = false;
        stoppedAt = batchIndex;
        break;
      }
      leafStartIndex += plan.take;
      sectorIndex += 1;
      if (sectorIndex >= sectors.length) {
        sectorIndex = 0;
        layer += 1;
      }
      batchIndex += 1;
    }

    return {
      generalBatchPlacements: {
        completed,
        stoppedAt,
        startIndex: firstPlan.leafStartIndex,
        startLayer: firstPlan.layer,
        startSectorIndex: firstPlan.sectorIndex,
        minRadius: firstPlan.minRadius,
        maxLeafRadius: Number.isFinite(firstPlan.maxLeafRadius) ? firstPlan.maxLeafRadius : null,
        layerGap: firstPlan.layerGap,
        batches: includeBatches ? batches : [],
        updates,
        leafZones,
        report,
      },
    };
  }

  function applyNetworkGeneralPlacementResult(out, generalPlacement, options = {}) {
    if (!generalPlacement || !out || typeof out !== "object") {
      throw new Error("network-general-sector-placement-missing");
    }
    if (!out.positions || typeof out.positions.set !== "function" || !Array.isArray(out.leafZones)) {
      throw new Error("network-general-sector-placement-target-invalid");
    }
    const placementReport = generalPlacement.report || {};
    const report = out.report || {};
    report.avoidanceHits = (Number(report.avoidanceHits) || 0) + (Number(placementReport.avoidanceHits) || 0);
    report.hardRejectHits = (Number(report.hardRejectHits) || 0) + (Number(placementReport.hardRejectHits) || 0);
    report.corridorRejectHits = (Number(report.corridorRejectHits) || 0) + (Number(placementReport.corridorRejectHits) || 0);
    report.territoryOverflowHits =
      (Number(report.territoryOverflowHits) || 0) + (Number(placementReport.territoryOverflowHits) || 0);
    report.fallbackPlacements = (Number(report.fallbackPlacements) || 0) + (Number(placementReport.fallbackPlacements) || 0);
    report.pushoutPlacements = (Number(report.pushoutPlacements) || 0) + (Number(placementReport.pushoutPlacements) || 0);
    out.report = report;
    const placementLeafZones = Array.isArray(generalPlacement.leafZones)
      ? generalPlacement.leafZones
      : [];
    const positionRows =
      Array.isArray(generalPlacement.updates) && generalPlacement.updates.length
        ? generalPlacement.updates
        : placementLeafZones;
    positionRows.forEach((row) => {
      const id = String(row?.id || "");
      if (!id) return;
      out.positions.set(id, {
        x: Number(row?.x) || 0,
        y: Number(row?.y) || 0,
      });
    });
    out.leafZones.push(...placementLeafZones);
    if (Array.isArray(options?.occupiedZones)) {
      options.occupiedZones.push(...placementLeafZones);
    }
    return out;
  }

  function applyNetworkSectorPlacementResult(out, sectorPlacement, options = {}) {
    const placement =
      sectorPlacement && typeof sectorPlacement === "object" ? sectorPlacement : null;
    if (!placement) {
      throw new Error("network-sector-placement-missing");
    }
    const generalPlacement =
      placement.generalBatchPlacements && typeof placement.generalBatchPlacements === "object"
        ? placement.generalBatchPlacements
        : null;
    if (!generalPlacement) {
      throw new Error("network-sector-placement-general-missing");
    }
    return applyNetworkGeneralPlacementResult(out, generalPlacement, options);
  }

  function applyNetworkSectorEnvelopePlacementResult(out, envelopePlacement) {
    const placement =
      envelopePlacement && typeof envelopePlacement === "object" ? envelopePlacement : null;
    if (!placement) {
      throw new Error("network-sector-envelope-placement-missing");
    }
    const zoneRows = Array.isArray(placement.zones) ? placement.zones : [];
    const zonesById = new Map();
    zoneRows.forEach((row) => {
      const id = String(row?.id || "");
      if (id) zonesById.set(id, row);
    });
    if (Array.isArray(out?.leafZones) && zonesById.size) {
      out.leafZones.forEach((zone) => {
        const id = String(zone?.id || "");
        const row = zonesById.get(id);
        if (!row) return;
        zone.x = Number(row?.x) || 0;
        zone.y = Number(row?.y) || 0;
        zone.r = Math.max(0, Number(row?.r) || 0);
        zone.ownerCenterId = String(row?.ownerCenterId || "");
        zone.ownerSectorIndex = Math.max(0, Number(row?.ownerSectorIndex) || 0);
      });
    }
    const updateRows = Array.isArray(placement.updates) ? placement.updates : zoneRows;
    updateRows.forEach((row) => {
      const id = String(row?.id || "");
      if (!id) return;
      out.positions.set(id, {
        x: Number(row?.x) || 0,
        y: Number(row?.y) || 0,
      });
    });
    return out;
  }

  function applyNetworkSectorPostProcessPlacementResult(out, generalPlacement) {
    const placement =
      generalPlacement && typeof generalPlacement === "object" ? generalPlacement : null;
    if (!placement) {
      throw new Error("network-sector-post-process-placement-missing");
    }
    const envelopePlacement =
      placement.postEnvelopePlacement && typeof placement.postEnvelopePlacement === "object"
        ? placement.postEnvelopePlacement
        : null;
    if (!envelopePlacement) {
      throw new Error("network-sector-envelope-placement-missing");
    }
    const uniformPlacement =
      placement.postUniformPlacement && typeof placement.postUniformPlacement === "object"
        ? placement.postUniformPlacement
        : null;
    if (!uniformPlacement) {
      throw new Error("network-sector-uniform-placement-missing");
    }
    const postUniformEnvelopePlacement =
      placement.postUniformEnvelopePlacement && typeof placement.postUniformEnvelopePlacement === "object"
        ? placement.postUniformEnvelopePlacement
        : null;
    if (!postUniformEnvelopePlacement) {
      throw new Error("network-sector-post-uniform-envelope-placement-missing");
    }
    applyNetworkSectorEnvelopePlacementResult(out, postUniformEnvelopePlacement);
    return {
      envelopeResult: envelopePlacement,
      uniformResult: uniformPlacement,
      postUniformEnvelopeResult: postUniformEnvelopePlacement,
    };
  }

  function scoreAngularBucketsFromPayload(payload = {}) {
    const cfg = payload?.config || DEFAULT_CFG;
    const bucketCount = Math.max(12, Number(cfg.ANGULAR_BUCKETS) || 36);
    const center = payload?.center || {};
    const centerX = Number(center?.x) || 0;
    const centerY = Number(center?.y) || 0;
    const localMap = payload?.localMap || {};
    const globalMap = payload?.globalMap || {};
    const sampleRadius = Math.max(80, Number(cfg.R_LEAF_MIN) || 220);
    const buckets = [];
    const step = (Math.PI * 2) / bucketCount;
    for (let i = 0; i < bucketCount; i += 1) {
      const angle = -Math.PI + step * i;
      const px = centerX + Math.cos(angle) * sampleRadius;
      const py = centerY + Math.sin(angle) * sampleRadius;
      const bucketScore = scoreAngularBucketScore(angle, px, py, localMap, globalMap, payload);
      buckets.push({
        index: i,
        angle,
        x: px,
        y: py,
        ...bucketScore,
      });
    }
    return buckets;
  }

  function scoreAngularBuckets(centerNode, localMap, globalMap, params = {}) {
    return scoreAngularBucketsFromPayload(
      buildAngularBucketSectorPayload(centerNode, [], localMap, globalMap, params)
    );
  }

  function projectAngularBucketSectorProjection(centerNode, leafRows = [], localMap = {}, globalMap = {}, params = {}) {
    const rows = Array.isArray(leafRows) ? leafRows : [];
    const payload = buildAngularBucketSectorPayload(centerNode, rows, localMap, globalMap, params);
    const scoredBuckets = scoreAngularBucketsFromPayload(payload);
    const sectorPlan = planAngularBucketSectors(scoredBuckets, rows, payload);
    return {
      payload,
      scoredBuckets,
      sectorPlan,
      leafRows: rows,
    };
  }

  function selectBestWindow(scoredBuckets, needed) {
    const rows = Array.isArray(scoredBuckets) ? scoredBuckets : [];
    const count = rows.length;
    if (!count) return null;
    const windowSize = Math.max(1, Math.min(count, needed));
    const scores = rows.map((row) => Number(row?.score) || 0);
    let sum = 0;
    for (let i = 0; i < windowSize; i += 1) sum += scores[i];
    let best = { start: 0, windowSize, avg: sum / windowSize };
    for (let start = 1; start < count; start += 1) {
      const removeIdx = start - 1;
      const addIdx = (start + windowSize - 1) % count;
      sum += scores[addIdx] - scores[removeIdx];
      const avg = sum / windowSize;
      if (avg > best.avg) best = { start, windowSize, avg };
    }
    if (!best) return null;
    const list = [];
    for (let i = 0; i < best.windowSize; i += 1) {
      list.push(rows[(best.start + i) % count]);
    }
    return { ...best, buckets: list };
  }

  function selectAngularBucketWindow(scoredBuckets, needed) {
    return selectBestWindow(scoredBuckets, needed);
  }

  function normalizeSectorsWithGap(sectors, minGap = 0.2, minSpan = 0.18) {
    const list = Array.isArray(sectors)
      ? sectors
          .map((row) => {
            const start = Number(row?.start);
            const end = Number(row?.end);
            const span = Math.max(0, end - start);
            if (!Number.isFinite(start) || !Number.isFinite(end) || span <= 0) return null;
            return {
              ...row,
              start,
              end,
              span,
              mid: normalizeAnglePositive(start + span * 0.5),
            };
          })
          .filter(Boolean)
      : [];
    if (list.length <= 1) return list;
    const gap = clampLayout(Number(minGap) || 0.2, 0, Math.PI / 2);
    const floorSpan = clampLayout(Number(minSpan) || 0.18, 0.08, Math.PI);
    list.sort((a, b) => a.mid - b.mid);
    const mids = list.map((row) => row.mid);
    const spans = list.map((row) => Math.max(floorSpan, row.span));
    const n = list.length;
    for (let iter = 0; iter < 8; iter += 1) {
      for (let i = 0; i < n; i += 1) {
        const j = (i + 1) % n;
        const leftMid = mids[i];
        const rightMid = j === 0 ? mids[0] + Math.PI * 2 : mids[j];
        const dist = Math.max(1e-6, rightMid - leftMid);
        const limit = Math.max(floorSpan, dist - gap);
        const pairHalf = spans[i] * 0.5 + spans[j] * 0.5;
        if (pairHalf <= limit + 1e-6) continue;
        const scale = clampLayout(limit / pairHalf, 0.1, 1);
        spans[i] = Math.max(floorSpan, spans[i] * scale);
        spans[j] = Math.max(floorSpan, spans[j] * scale);
      }
    }
    return list.map((row, idx) => {
      const span = spans[idx];
      const start = row.mid - span * 0.5;
      return {
        ...row,
        start,
        end: start + span,
        span,
      };
    });
  }

  function createTerritoryAngleChecker(territoryRanges = []) {
    const normalizedTerritoryRanges = (Array.isArray(territoryRanges) ? territoryRanges : [])
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
    const hasTerritoryConstraint = normalizedTerritoryRanges.length > 0;
    const isInTerritory = (angle) => {
      if (!hasTerritoryConstraint || territoryHasFullRange) return true;
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
    return { hasTerritoryConstraint, isInTerritory };
  }

  function planAngularBucketSectors(scoredBuckets, leafRows = [], params = {}) {
    const cfg = params?.config || DEFAULT_CFG;
    const territoryRanges = Array.isArray(params?.territoryRanges) ? params.territoryRanges : [];
    const allowTerritoryOverflow = !!params?.allowTerritoryOverflow;
    const preferRingMode = !!params?.preferRingMode;
    const enforceSingleSector = params?.enforceSingleSector !== false;
    const preferredMultiSectorCount = Math.max(1, Math.floor(Number(params?.preferredMultiSectorCount) || 1));
    const multiSectorGap = clampLayout(Number(params?.multiSectorGap) || 0.22, 0.08, Math.PI / 2);
    const rows = Array.isArray(leafRows) ? leafRows : [];
    const leafCount = rows.length;
    const { hasTerritoryConstraint, isInTerritory } = createTerritoryAngleChecker(territoryRanges);
    const avgLabelPad = rows.reduce((sum, row) => sum + (Number(row?.labelPad) || 0), 0) / Math.max(1, rows.length);
    const denseSector = leafCount >= 36;
    const scoredRows = (scoredBuckets || []).filter(Boolean);
    const effectiveScoredRows = hasTerritoryConstraint
      ? scoredRows.filter((row) => isInTerritory(Number(row?.angle) || 0))
      : scoredRows;
    const layoutRows = effectiveScoredRows.length ? effectiveScoredRows : scoredRows;
    const bucketCount = Math.max(12, scoredRows.length || Number(cfg.ANGULAR_BUCKETS) || 36);
    const bucketStep = (Math.PI * 2) / bucketCount;
    const arcLabelScale = clampLayout(0.94 + avgLabelPad / 130, 0.92, 1.28);
    const minArc = Math.max(0.4, (Number(cfg.NETWORK_SECTOR_MIN_ANGLE) || Math.PI / 5.5) * arcLabelScale);
    const maxArcBase = Math.max(
      minArc,
      (Number(cfg.NETWORK_SECTOR_MAX_ANGLE) || Math.PI * 1.55) * clampLayout(0.96 + avgLabelPad / 100, 0.94, 1.34)
    );
    const maxArc = denseSector
      ? Math.max(maxArcBase, Math.min(Math.PI * 1.68, maxArcBase * 1.08))
      : maxArcBase;
    const minRadiusBase = Math.max(80, Number(cfg.R_LEAF_MIN) || 220);
    const layerGapForArc = Math.max(18, Number(cfg.NETWORK_SECTOR_LAYER_GAP) || 68);
    const plannedLayersForArc = denseSector
      ? Math.max(4, Math.ceil(Math.sqrt(Math.max(1, leafCount)) / 1.22))
      : Math.max(2, Math.ceil(Math.sqrt(Math.max(1, leafCount)) / 1.68));
    const perLayerForArc = Math.max(1, Math.ceil(leafCount / Math.max(1, plannedLayersForArc)));
    const arcGapForArc = clampLayout(18 + avgLabelPad * 0.34, 22, 64);
    const arcRadiusRef = Math.max(
      minRadiusBase * 1.06,
      minRadiusBase + Math.max(0, plannedLayersForArc - 1) * layerGapForArc * 0.52
    );
    const arcNeeded = (perLayerForArc * arcGapForArc) / Math.max(72, arcRadiusRef);
    const arcDensityBoost = denseSector ? 1.18 : 1.1;
    const targetArc = clampLayout(arcNeeded * arcDensityBoost, minArc, maxArc);
    const territorySpanTotal = (territoryRanges || []).reduce(
      (sum, row) => sum + Math.max(0, Number(row?.span) || 0),
      0
    );
    const maxTerritoryBuckets =
      territoryRanges.length && !allowTerritoryOverflow
        ? Math.max(1, Math.floor(territorySpanTotal / bucketStep))
        : bucketCount;
    const neededBuckets = Math.max(1, Math.min(maxTerritoryBuckets, Math.ceil(targetArc / bucketStep)));
    const territorySlackBuckets = Math.max(0, maxTerritoryBuckets - neededBuckets);
    const denseExtraDemand = denseSector ? Math.ceil(Math.sqrt(Math.max(1, leafCount)) * 1.05) : 0;
    const extraBuckets = Math.max(0, Math.min(territorySlackBuckets, denseExtraDemand));
    const expandedBuckets = Math.max(neededBuckets, neededBuckets + extraBuckets);
    const bestWindow = selectAngularBucketWindow(layoutRows, expandedBuckets);
    const bestBuckets = bestWindow?.buckets?.length ? bestWindow.buckets : layoutRows.slice(0, neededBuckets);
    const bucketTopScore = bestBuckets.length
      ? Number(Math.max(...bestBuckets.map((row) => Number(row?.score) || 0)).toFixed(3))
      : null;
    const sectors = [];
    if (preferRingMode) {
      sectors.push({ start: -Math.PI, end: Math.PI, score: bestWindow?.avg ?? 0 });
    } else {
      const desiredSectorCount = enforceSingleSector
        ? 1
        : Math.max(1, Math.min(5, preferredMultiSectorCount));
      const perSectorArcCap =
        desiredSectorCount > 1
          ? Math.max(
              bucketStep,
              Math.min(
                maxArc,
                ((Math.PI * 2 - multiSectorGap * desiredSectorCount) / desiredSectorCount) *
                  (denseSector ? 1.28 : 1.18)
              )
            )
          : maxArc;
      const windowRows = layoutRows.map((row) => ({ ...row }));
      const pushWindowAsSector = (window, minFactor = 0.6) => {
        if (!window?.buckets?.length) return;
        const firstAngle = Number(window.buckets[0]?.angle);
        const startAngle = Number.isFinite(firstAngle) ? firstAngle - bucketStep * 0.5 : -targetArc / 2;
        const windowArc = Math.max(bucketStep, window.buckets.length * bucketStep);
        const arcSpan = clampLayout(Math.max(targetArc * minFactor, windowArc), bucketStep, perSectorArcCap);
        sectors.push({ start: startAngle, end: startAngle + arcSpan, score: window.avg ?? 0 });
        const windowMid = startAngle + arcSpan * 0.5;
        const exclusion = Math.max(multiSectorGap, arcSpan * 0.54);
        windowRows.forEach((row) => {
          const a = Number(row?.angle) || 0;
          const d = angleDiff(a, windowMid);
          if (d < exclusion) row.score -= (exclusion - d) * 1300 + 2000;
        });
      };
      if (bestWindow?.buckets?.length) {
        pushWindowAsSector(bestWindow, 0.6);
        for (let si = 2; si <= desiredSectorCount; si += 1) {
          const windowFactor = denseSector
            ? si === 2
              ? 0.74
              : si === 3
              ? 0.62
              : si === 4
              ? 0.52
              : 0.44
            : si === 2
            ? 0.74
            : 0.58;
          const minFactor = denseSector
            ? si === 2
              ? 0.48
              : si === 3
              ? 0.4
              : si === 4
              ? 0.34
              : 0.3
            : si === 2
            ? 0.44
            : 0.36;
          const windowSize = Math.max(1, Math.floor(expandedBuckets * windowFactor));
          const next = selectAngularBucketWindow(windowRows, windowSize);
          if (!next?.buckets?.length) break;
          pushWindowAsSector(next, minFactor);
        }
      }
      if (!sectors.length) {
        sectors.push({
          start: -targetArc / 2,
          end: targetArc / 2,
          score: bestWindow?.avg ?? 0,
        });
      }
    }
    const normalizedSectors = normalizeSectorsWithGap(
      sectors,
      multiSectorGap,
      Math.max(bucketStep * 0.92, 0.18)
    );
    return {
      sectors: normalizedSectors,
      bucketTopScore,
      leafCount,
      avgLabelPad,
      denseSector,
      bucketStep,
      targetArc,
      minRadiusBase,
    };
  }

  function quantileSortedValues(sortedValues, q = 0.5) {
    const rows = Array.isArray(sortedValues) ? sortedValues : [];
    if (!rows.length) return 0;
    const qq = clampLayout(Number(q) || 0, 0, 1);
    const idx = (rows.length - 1) * qq;
    const lo = Math.floor(idx);
    const hi = Math.ceil(idx);
    const lv = Number(rows[lo]) || 0;
    const hv = Number(rows[hi]) || lv;
    if (hi <= lo) return lv;
    return lv + (hv - lv) * (idx - lo);
  }

  function isAngleInsideSector(angle, start, span, tolerance = 1e-4) {
    const arcSpan = clampLayout(Number(span) || 0, 0, Math.PI * 2);
    if (arcSpan >= Math.PI * 2 - 1e-6) return true;
    const clamped = clampAngleToArc(angle, start, arcSpan);
    return angleDiff(angle, clamped) <= Math.max(1e-6, Number(tolerance) || 1e-4);
  }

  function applySectorEnvelopePostProcess(
    centerNode,
    sectors,
    leafZones,
    occupiedZones,
    options = {}
  ) {
    const centerX = Number(centerNode?.x) || 0;
    const centerY = Number(centerNode?.y) || 0;
    const zoneRows = Array.isArray(leafZones) ? leafZones.filter(Boolean) : [];
    const sectorRows = Array.isArray(sectors) ? sectors.filter(Boolean) : [];
    if (!zoneRows.length || !sectorRows.length) {
      return { adjusted: 0, pulledByRadius: 0, pulledByAngle: 0 };
    }
    const rowsById = options?.rowsById instanceof Map ? options.rowsById : new Map();
    const hardZones = Array.isArray(options?.hardZones) ? options.hardZones : [];
    const hardCorridors = Array.isArray(options?.hardCorridors) ? options.hardCorridors : [];
    const territoryRanges = Array.isArray(options?.territoryRanges) ? options.territoryRanges : [];
    const allowTerritoryOverflow = !!options?.allowTerritoryOverflow;
    const strictCenterOwnership = options?.strictCenterOwnership !== false;
    const ownerCenters = Array.isArray(options?.ownerCenters) ? options.ownerCenters : [];
    const minRadiusFloor = Math.max(0, Number(options?.minRadiusFloor) || 0);
    const maxRadiusInput = Number(options?.maxRadiusCap);
    const maxRadiusCap = Number.isFinite(maxRadiusInput)
      ? Math.max(minRadiusFloor + 16, maxRadiusInput)
      : Number.POSITIVE_INFINITY;
    const centerId = String(centerNode?.id || "");
    const visibleZones = Array.isArray(occupiedZones) ? occupiedZones : [];
    const hardZoneIndex = hardZones.length ? createZoneSpatialIndex(hardZones, { cellSize: 168 }) : null;
    const hardCorridorIndex = hardCorridors.length
      ? createCorridorSpatialIndex(hardCorridors, { cellSize: 220 })
      : null;
    const rayVisibleZones = centerId
      ? visibleZones.filter((zone) => !(zone?.ownerCenterId && String(zone.ownerCenterId) !== centerId))
      : visibleZones;
    const visibleZoneIndex = createZoneSpatialIndex(rayVisibleZones, { cellSize: 172 });

    const ownedByCenter = makeCenterOwnershipChecker(
      centerId,
      centerX,
      centerY,
      ownerCenters,
      strictCenterOwnership
    );

    const overlapPenaltyExcludingSelf = (x, y, node, ignoreId) => {
      const radius = nodeCollisionRadius(node);
      let penalty = 0;
      const px = Number(x) || 0;
      const py = Number(y) || 0;
      visibleZoneIndex.forEachNearby(
        px,
        py,
        radius + 8 + (Number(visibleZoneIndex.maxRadius?.()) || 0),
        (entry) => {
          if (!entry) return false;
          if (String(entry?.zone?.id || "") === String(ignoreId || "")) return false;
          const dx = (Number(entry.x) || 0) - px;
          const dy = (Number(entry.y) || 0) - py;
          const dist = Math.hypot(dx, dy);
          const threshold = radius + (Number(entry.r) || 0) + 8;
          if (dist < threshold) penalty += threshold - dist;
          return false;
        }
      );
      return penalty;
    };

    const scoreCandidate = (x, y, node, zone, sector) => {
      if (!ownedByCenter(x, y)) return Number.POSITIVE_INFINITY;
      const angle = Math.atan2((Number(y) || 0) - centerY, (Number(x) || 0) - centerX);
      const span = Math.max(0, Number(sector?.end) - Number(sector?.start));
      if (!isAngleInsideSector(angle, sector?.start, span, 0.012)) {
        return Number.POSITIVE_INFINITY;
      }
      if (!allowTerritoryOverflow && territoryRanges.length && !isAngleInRanges(angle, territoryRanges)) {
        return Number.POSITIVE_INFINITY;
      }
      if (collidesWithZones(x, y, node, hardZones, 10, hardZoneIndex)) return Number.POSITIVE_INFINITY;
      if (collidesWithCorridors(x, y, node, hardCorridors, 10, hardCorridorIndex)) return Number.POSITIVE_INFINITY;
      const overlapPenalty = overlapPenaltyExcludingSelf(x, y, node, zone?.id);
      const nodeLabel = labelPad(node);
      const rayPenalty = rayClusterPenalty(centerX, centerY, x, y, rayVisibleZones, {
        angleThreshold: 0.09,
        radialThreshold: Math.max(22, nodeLabel * 1.5),
      });
      return overlapPenalty * 7.2 + rayPenalty;
    };

    let adjusted = 0;
    let pulledByRadius = 0;
    let pulledByAngle = 0;

    sectorRows.forEach((sector, sectorIndex) => {
      const span = Math.max(0, Number(sector?.end) - Number(sector?.start));
      if (!(span > 1e-5)) return;
      const entries = zoneRows
        .filter((zone) => Number(zone?.ownerSectorIndex) === sectorIndex)
        .map((zone) => {
          const node = rowsById.get(String(zone?.id || ""));
          if (!node) return null;
          const dx = (Number(zone.x) || 0) - centerX;
          const dy = (Number(zone.y) || 0) - centerY;
          return {
            zone,
            node,
            radius: Math.hypot(dx, dy),
            angle: Math.atan2(dy, dx),
          };
        })
        .filter(Boolean);
      if (entries.length < 4) return;
      const radii = entries
        .map((row) => Number(row?.radius) || 0)
        .sort((a, b) => a - b);
      const q1 = quantileSortedValues(radii, 0.25);
      const q3 = quantileSortedValues(radii, 0.75);
      const q90 = quantileSortedValues(radii, 0.9);
      const iqr = Math.max(0, q3 - q1);
      const radiusLower = Math.max(minRadiusFloor, q1 - Math.max(20, iqr * 1.1));
      const radiusUpper = Math.min(
        maxRadiusCap,
        Math.max(q90 + 6, q3 + Math.max(24, iqr * 1.3))
      );
      const safeUpper = Math.max(radiusLower + 16, radiusUpper);
      const angleStep = clampLayout(span / Math.max(28, entries.length * 0.72), 0.012, 0.08);
      const radialStep = clampLayout((safeUpper - radiusLower) / 9, 8, 26);
      const angleOffsets = [0];
      for (let i = 1; i <= 7; i += 1) {
        angleOffsets.push(i * angleStep, -i * angleStep);
      }
      const radialOffsets = [0];
      for (let i = 1; i <= 6; i += 1) {
        radialOffsets.push(-i * radialStep, i * radialStep);
      }

      entries.forEach((entry) => {
        const currentAngle = Number(entry?.angle) || 0;
        const currentRadius = Number(entry?.radius) || 0;
        const outByAngle = !isAngleInsideSector(currentAngle, sector?.start, span, 0.012);
        const outByRadius = currentRadius > safeUpper + 4 || currentRadius < radiusLower - 4;
        if (!outByAngle && !outByRadius) return;

        const baseAngle = clampAngleToArc(currentAngle, sector?.start, span);
        const baseRadius = clampLayout(currentRadius, radiusLower, safeUpper);
        const currentPenalty = scoreCandidate(
          Number(entry?.zone?.x) || 0,
          Number(entry?.zone?.y) || 0,
          entry.node,
          entry.zone,
          sector
        );
        let best = null;
        let bestPenalty = Number.POSITIVE_INFINITY;

        for (const rrShift of radialOffsets) {
          const rr = clampLayout(baseRadius + rrShift, radiusLower, safeUpper);
          for (const aaShift of angleOffsets) {
            const angle = clampAngleToArc(baseAngle + aaShift, sector?.start, span);
            const x = centerX + Math.cos(angle) * rr;
            const y = centerY + Math.sin(angle) * rr;
            const penalty = scoreCandidate(x, y, entry.node, entry.zone, sector);
            if (!(penalty < bestPenalty)) continue;
            bestPenalty = penalty;
            best = { x, y, angle };
            if (penalty <= 0.08) break;
          }
          if (bestPenalty <= 0.08) break;
        }

        if (!best || !Number.isFinite(bestPenalty)) return;
        const shouldApply =
          !Number.isFinite(currentPenalty) ||
          bestPenalty + 0.05 < currentPenalty ||
          outByAngle ||
          outByRadius;
        if (!shouldApply) return;
        entry.zone.x = Number(best.x) || 0;
        entry.zone.y = Number(best.y) || 0;
        adjusted += 1;
        if (outByRadius) pulledByRadius += 1;
        if (outByAngle) pulledByAngle += 1;
      });
    });

    return { adjusted, pulledByRadius, pulledByAngle };
  }

  function projectNetworkSectorEnvelope(payload = {}) {
    const center = payload?.center || {};
    const sectors = Array.isArray(payload?.sectors) ? payload.sectors.map((row) => ({ ...row })) : [];
    const leafZones = Array.isArray(payload?.leafZones) ? payload.leafZones.map((row) => ({ ...row })) : [];
    const occupiedZones = Array.isArray(payload?.occupiedZones)
      ? payload.occupiedZones.map((row) => ({ ...row }))
      : leafZones;
    const nodeRows = Array.isArray(payload?.nodes) ? payload.nodes : [];
    const rowsById = new Map(nodeRows.map((node) => [String(node?.id || ""), node]));
    const result = applySectorEnvelopePostProcess(center, sectors, leafZones, occupiedZones, {
      rowsById,
      hardZones: Array.isArray(payload?.hardZones) ? payload.hardZones : [],
      hardCorridors: Array.isArray(payload?.hardCorridors) ? payload.hardCorridors : [],
      territoryRanges: Array.isArray(payload?.territoryRanges) ? payload.territoryRanges : [],
      allowTerritoryOverflow: !!payload?.allowTerritoryOverflow,
      strictCenterOwnership: payload?.strictCenterOwnership !== false,
      ownerCenters: Array.isArray(payload?.ownerCenters) ? payload.ownerCenters : [],
      minRadiusFloor: Number(payload?.minRadiusFloor) || 0,
      maxRadiusCap: payload?.maxRadiusCap,
    });
    return {
      ...result,
      zones: leafZones.map((zone) => ({
        id: String(zone?.id || ""),
        x: Number(zone?.x) || 0,
        y: Number(zone?.y) || 0,
        r: Number(zone?.r) || 0,
        ownerCenterId: String(zone?.ownerCenterId || ""),
        ownerSectorIndex: Number(zone?.ownerSectorIndex) || 0,
      })),
    };
  }

  function projectNetworkSectorUniformity(payload = {}) {
    const center = payload?.center || {};
    const sectors = Array.isArray(payload?.sectors) ? payload.sectors.map((row) => ({ ...row })) : [];
    const leafZones = Array.isArray(payload?.leafZones) ? payload.leafZones.map((row) => ({ ...row })) : [];
    const occupiedZones = Array.isArray(payload?.occupiedZones)
      ? payload.occupiedZones.map((row) => ({ ...row }))
      : leafZones;
    const nodeRows = Array.isArray(payload?.nodes) ? payload.nodes : [];
    const rowsById = new Map(nodeRows.map((node) => [String(node?.id || ""), node]));
    const result = applySectorUniformityPostProcess(center, sectors, leafZones, occupiedZones, {
      rowsById,
      hardZones: Array.isArray(payload?.hardZones) ? payload.hardZones : [],
      hardCorridors: Array.isArray(payload?.hardCorridors) ? payload.hardCorridors : [],
      territoryRanges: Array.isArray(payload?.territoryRanges) ? payload.territoryRanges : [],
      allowTerritoryOverflow: !!payload?.allowTerritoryOverflow,
      strictCenterOwnership: payload?.strictCenterOwnership !== false,
      ownerCenters: Array.isArray(payload?.ownerCenters) ? payload.ownerCenters : [],
      minRadiusFloor: Number(payload?.minRadiusFloor) || 0,
      maxRadiusCap: payload?.maxRadiusCap,
    });
    return {
      adjusted: Number(result?.adjusted) || 0,
      planned: Number(result?.planned) || 0,
      rejected: Number(result?.rejected) || 0,
      sparseLayerRebalances: Number(result?.sparseLayerRebalances) || 0,
      fallbackRescues: Number(result?.fallbackRescues) || 0,
      zones: leafZones.map((zone) => ({
        id: String(zone?.id || ""),
        x: Number(zone?.x) || 0,
        y: Number(zone?.y) || 0,
        r: Number(zone?.r) || 0,
        ownerCenterId: String(zone?.ownerCenterId || ""),
        ownerSectorIndex: Number(zone?.ownerSectorIndex) || 0,
      })),
    };
  }

  function applySectorUniformityPostProcess(
    centerNode,
    sectors,
    leafZones,
    occupiedZones,
    options = {}
  ) {
    const centerX = Number(centerNode?.x) || 0;
    const centerY = Number(centerNode?.y) || 0;
    const zoneRows = Array.isArray(leafZones) ? leafZones.filter(Boolean) : [];
    const sectorRows = Array.isArray(sectors) ? sectors.filter(Boolean) : [];
    if (!zoneRows.length || !sectorRows.length) return { adjusted: 0 };

    const rowsById = options?.rowsById instanceof Map ? options.rowsById : new Map();
    const hardZones = Array.isArray(options?.hardZones) ? options.hardZones : [];
    const hardCorridors = Array.isArray(options?.hardCorridors) ? options.hardCorridors : [];
    const territoryRanges = Array.isArray(options?.territoryRanges) ? options.territoryRanges : [];
    const allowTerritoryOverflow = !!options?.allowTerritoryOverflow;
    const strictCenterOwnership = options?.strictCenterOwnership !== false;
    const ownerCenters = Array.isArray(options?.ownerCenters) ? options.ownerCenters : [];
    const minRadiusFloor = Math.max(0, Number(options?.minRadiusFloor) || 0);
    const maxRadiusInput = Number(options?.maxRadiusCap);
    const maxRadiusCap = Number.isFinite(maxRadiusInput)
      ? Math.max(minRadiusFloor + 16, maxRadiusInput)
      : Number.POSITIVE_INFINITY;
    const centerId = String(centerNode?.id || "");
    const visibleZones = Array.isArray(occupiedZones) ? occupiedZones : [];
    const hardZoneIndex = hardZones.length ? createZoneSpatialIndex(hardZones, { cellSize: 168 }) : null;
    const hardCorridorIndex = hardCorridors.length
      ? createCorridorSpatialIndex(hardCorridors, { cellSize: 220 })
      : null;
    const rayBaseVisibleZones = centerId
      ? visibleZones.filter((zone) => !(zone?.ownerCenterId && String(zone.ownerCenterId) !== centerId))
      : visibleZones;

    const ownedByCenter = makeCenterOwnershipChecker(
      centerId,
      centerX,
      centerY,
      ownerCenters,
      strictCenterOwnership
    );

    const overlapPenaltyExcluding = (
      x,
      y,
      node,
      ignoreIdsSet,
      draftZones = [],
      externalZoneIndex = null,
      draftZoneIndex = null
    ) => {
      const px = Number(x) || 0;
      const py = Number(y) || 0;
      const radius = nodeCollisionRadius(node);
      let penalty = 0;
      if (externalZoneIndex && typeof externalZoneIndex.forEachNearby === "function") {
        externalZoneIndex.forEachNearby(
          px,
          py,
          radius + 8 + (Number(externalZoneIndex.maxRadius?.()) || 0),
          (zone) => {
            if (!zone) return false;
            const dx = (Number(zone.x) || 0) - px;
            const dy = (Number(zone.y) || 0) - py;
            const dist = Math.hypot(dx, dy);
            const threshold = radius + (Number(zone.r) || 0) + 8;
            if (dist < threshold) penalty += threshold - dist;
            return false;
          }
        );
      } else {
        for (const zone of visibleZones) {
          if (!zone) continue;
          const zid = String(zone?.id || "");
          if (ignoreIdsSet?.has(zid)) continue;
          const dx = (Number(zone.x) || 0) - px;
          const dy = (Number(zone.y) || 0) - py;
          const dist = Math.hypot(dx, dy);
          const threshold = radius + (Number(zone.r) || 0) + 8;
          if (dist < threshold) penalty += threshold - dist;
        }
      }
      if (draftZoneIndex && typeof draftZoneIndex.forEachNearby === "function") {
        draftZoneIndex.forEachNearby(
          px,
          py,
          radius + 8 + (Number(draftZoneIndex.maxRadius?.()) || 0),
          (zone) => {
            if (!zone) return false;
            const dx = (Number(zone.x) || 0) - px;
            const dy = (Number(zone.y) || 0) - py;
            const dist = Math.hypot(dx, dy);
            const threshold = radius + (Number(zone.r) || 0) + 8;
            if (dist < threshold) penalty += threshold - dist;
            return false;
          }
        );
      } else {
        const drafts = Array.isArray(draftZones) ? draftZones : [];
        drafts.forEach((zone) => {
          if (!zone) return;
          const dx = (Number(zone.x) || 0) - px;
          const dy = (Number(zone.y) || 0) - py;
          const dist = Math.hypot(dx, dy);
          const threshold = radius + (Number(zone.r) || 0) + 8;
          if (dist < threshold) penalty += threshold - dist;
        });
      }
      return penalty;
    };

    const rayPenaltyWithDraft = (x, y, ignoreIdsSet, draftZones = [], angleThreshold = 0.085, radialThreshold = 34) => {
      const targetX = Number(x) || 0;
      const targetY = Number(y) || 0;
      const at = clampLayout(Number(angleThreshold) || 0.085, 0.02, 0.24);
      const rt = Math.max(12, Number(radialThreshold) || 34);
      const targetAngle = Math.atan2(targetY - centerY, targetX - centerX);
      const targetRadius = Math.hypot(targetX - centerX, targetY - centerY);
      let penalty = 0;
      for (const zone of rayBaseVisibleZones) {
        if (!zone) continue;
        const zid = String(zone?.id || "");
        if (ignoreIdsSet?.has(zid)) continue;
        const zx = Number(zone.x) || 0;
        const zy = Number(zone.y) || 0;
        const angle = Math.atan2(zy - centerY, zx - centerX);
        const radius = Math.hypot(zx - centerX, zy - centerY);
        const ad = angleDiff(targetAngle, angle);
        if (ad >= at) continue;
        const rd = Math.abs(targetRadius - radius);
        const angleWeight = (at - ad) / Math.max(1e-6, at);
        const radialWeight = rd < rt ? 1.85 : rd < rt * 2.1 ? 1.12 : 0.55;
        penalty += angleWeight * radialWeight * 22;
      }
      const drafts = Array.isArray(draftZones) ? draftZones : [];
      drafts.forEach((zone) => {
        if (!zone) return;
        const zx = Number(zone.x) || 0;
        const zy = Number(zone.y) || 0;
        const angle = Math.atan2(zy - centerY, zx - centerX);
        const radius = Math.hypot(zx - centerX, zy - centerY);
        const ad = angleDiff(targetAngle, angle);
        if (ad >= at) return;
        const rd = Math.abs(targetRadius - radius);
        const angleWeight = (at - ad) / Math.max(1e-6, at);
        const radialWeight = rd < rt ? 1.85 : rd < rt * 2.1 ? 1.12 : 0.55;
        penalty += angleWeight * radialWeight * 22;
      });
      return penalty;
    };

    const scoreCandidate = (
      x,
      y,
      node,
      sector,
      targetAngle,
      targetRadius,
      ignoreIdsSet,
      draftZones,
      scoringCtx = null
    ) => {
      if (!ownedByCenter(x, y)) return Number.POSITIVE_INFINITY;
      const angle = Math.atan2((Number(y) || 0) - centerY, (Number(x) || 0) - centerX);
      const radius = Math.hypot((Number(x) || 0) - centerX, (Number(y) || 0) - centerY);
      const span = Math.max(0, Number(sector?.end) - Number(sector?.start));
      if (!isAngleInsideSector(angle, sector?.start, span, 0.01)) return Number.POSITIVE_INFINITY;
      if (!allowTerritoryOverflow && territoryRanges.length && !isAngleInRanges(angle, territoryRanges)) {
        return Number.POSITIVE_INFINITY;
      }
      if (collidesWithZones(x, y, node, hardZones, 10, hardZoneIndex)) return Number.POSITIVE_INFINITY;
      if (collidesWithCorridors(x, y, node, hardCorridors, 5, hardCorridorIndex)) return Number.POSITIVE_INFINITY;
      const overlapPenalty = overlapPenaltyExcluding(
        x,
        y,
        node,
        ignoreIdsSet,
        draftZones,
        scoringCtx?.externalZoneIndex || null,
        scoringCtx?.draftZoneIndex || null
      );
      const rayThreshold = Math.max(20, labelPad(node) * 1.42);
      const rayPenalty =
        scoringCtx?.externalRayIndex && scoringCtx?.draftRayIndex
          ? scoringCtx.externalRayIndex.penaltyAt(x, y, 0.085, rayThreshold) +
            scoringCtx.draftRayIndex.penaltyAt(x, y, 0.085, rayThreshold)
          : rayPenaltyWithDraft(x, y, ignoreIdsSet, draftZones, 0.085, rayThreshold);
      const anchorPenalty =
        angleDiff(angle, targetAngle) * Math.max(16, targetRadius * 0.08) * 0.85 +
        Math.abs(radius - targetRadius) * 0.11;
      return overlapPenalty * 2.2 + rayPenalty * 0.9 + anchorPenalty;
    };

    let adjusted = 0;
    let planned = 0;
    let rejected = 0;
    let sparseLayerRebalances = 0;
    let fallbackRescues = 0;
    sectorRows.forEach((sector, sectorIndex) => {
      const span = Math.max(0, Number(sector?.end) - Number(sector?.start));
      if (!(span > 1e-5)) return;
      const entries = zoneRows
        .filter((zone) => Number(zone?.ownerSectorIndex) === sectorIndex)
        .map((zone) => {
          const node = rowsById.get(String(zone?.id || ""));
          if (!node) return null;
          const dx = (Number(zone.x) || 0) - centerX;
          const dy = (Number(zone.y) || 0) - centerY;
          const angle = Math.atan2(dy, dx);
          return {
            zone,
            node,
            angle,
            rel: normalizeAnglePositive(angle - Number(sector?.start || 0)),
            radius: Math.hypot(dx, dy),
            weight: 1,
          };
        })
        .filter(Boolean)
        .sort((a, b) => a.rel - b.rel);
      if (entries.length < 6) return;

      const gapThreshold = clampLayout(
        Math.max(0.14, span / Math.max(3.2, Math.sqrt(entries.length) * 1.35)),
        0.14,
        0.78
      );
      const groups = [];
      let currentGroup = [];
      entries.forEach((entry, idx) => {
        if (idx === 0) {
          currentGroup.push(entry);
          return;
        }
        const prev = entries[idx - 1];
        const gap = Math.max(0, Number(entry?.rel) - Number(prev?.rel));
        if (gap > gapThreshold && currentGroup.length) {
          groups.push(currentGroup);
          currentGroup = [entry];
        } else {
          currentGroup.push(entry);
        }
      });
      if (currentGroup.length) groups.push(currentGroup);
      if (!groups.length) return;
      const largestGroupSize = groups.reduce((max, g) => Math.max(max, Array.isArray(g) ? g.length : 0), 0);
      const useWholeSectorUniform = entries.length >= 18 || largestGroupSize >= entries.length * 0.58;
      const uniformGroups = useWholeSectorUniform ? [entries] : groups;

      uniformGroups.forEach((group, groupIndex) => {
        const groupRows = Array.isArray(group) ? group.filter(Boolean) : [];
        if (groupRows.length < 2) return;
        const groupCount = groupRows.length;
        const radii = groupRows.map((row) => Number(row?.radius) || 0).sort((a, b) => a - b);
        const q10 = quantileSortedValues(radii, 0.1);
        const q25 = quantileSortedValues(radii, 0.25);
        const q50 = quantileSortedValues(radii, 0.5);
        const q75 = quantileSortedValues(radii, 0.75);
        const q90 = quantileSortedValues(radii, 0.9);
        const iqr = Math.max(0, q75 - q25);
        const avgLabel = groupRows.reduce((sum, row) => sum + labelPad(row?.node), 0) / Math.max(1, groupCount);
        const avgNodeRadius = groupRows.reduce((sum, row) => sum + nodeRadius(row?.node), 0) / Math.max(1, groupCount);
        const avgCollision =
          groupRows.reduce((sum, row) => sum + nodeCollisionRadius(row?.node), 0) /
          Math.max(1, groupCount);
        const nominalArcGap = resolveNetworkNominalArcGap(
          groupCount,
          avgLabel,
          avgNodeRadius,
          avgCollision
        );
        const groupStartRel = Number(groupRows[0]?.rel) || 0;
        const groupEndRel = Number(groupRows[groupCount - 1]?.rel) || groupStartRel;
        const rawGroupSpan = Math.max(0.05, groupEndRel - groupStartRel);
        const denseGroup = groupCount >= 24;
        const edgePad = clampLayout(span * 0.018, 0.012, 0.12);
        let usableStartRel;
        let usableEndRel;
        if (denseGroup || rawGroupSpan >= span * 0.62) {
          usableStartRel = edgePad;
          usableEndRel = span - edgePad;
        } else {
          const extra = clampLayout(rawGroupSpan * 0.38 + (denseGroup ? 0.28 : 0.18), 0.16, Math.max(0.16, span * 0.36));
          usableStartRel = clampLayout(groupStartRel - extra, edgePad, Math.max(edgePad, span - edgePad - 0.06));
          usableEndRel = clampLayout(groupEndRel + extra, usableStartRel + 0.06, span - edgePad);
        }
        let usableSpan = Math.max(0.05, usableEndRel - usableStartRel);
        const midRadiusForCap = clampLayout(
          Number(q50) || (minRadiusFloor + 40),
          minRadiusFloor + 8,
          Number.isFinite(maxRadiusCap) ? Math.max(minRadiusFloor + 20, maxRadiusCap) : Number.MAX_SAFE_INTEGER
        );
        const bandCapacity = Math.max(
          2,
          Math.floor((usableSpan * Math.max(midRadiusForCap, minRadiusFloor + 12)) / nominalArcGap)
        );
        const layerCountRaw = Math.ceil(groupCount / bandCapacity);
        const layerCount = Math.max(
          1,
          Math.min(groupCount, Math.floor(layerCountRaw || 1), resolveNetworkSectorLayerLimit(groupCount))
        );
        const bandGap = clampLayout(
          Math.max(18, avgLabel * 0.28 + avgNodeRadius * 0.42 + Math.sqrt(groupCount) * 0.7),
          18,
          68
        );
        const jitterAmp = clampLayout(Math.max(8, iqr * 0.2 + Math.sqrt(groupCount) * 0.58), 8, 34);
        const idealSpan = clampLayout(
          (groupCount / Math.max(1, bandCapacity)) * 0.14 +
            (groupCount - 1) * (nominalArcGap / Math.max(140, midRadiusForCap)),
          0.08,
          span * 0.98
        );
        if (usableSpan < idealSpan) {
          const centerRel = (groupStartRel + groupEndRel) * 0.5;
          const half = idealSpan * 0.5;
          usableStartRel = clampLayout(centerRel - half, edgePad, Math.max(edgePad, span - edgePad - idealSpan));
          usableEndRel = clampLayout(usableStartRel + idealSpan, usableStartRel + 0.05, span - edgePad);
          usableSpan = Math.max(0.05, usableEndRel - usableStartRel);
        }

        const localCap = Number.isFinite(maxRadiusCap)
          ? maxRadiusCap
          : Number(q50) + Math.max(120, bandGap * 9);
        const spread = clampLayout(
          Math.max(bandGap * Math.max(1, layerCount - 1) + 16, iqr + bandGap * 1.15),
          22,
          Math.max(24, localCap - minRadiusFloor)
        );
        const radiusCenterBase = clampLayout(
          Number(q50) || (minRadiusFloor + spread * 0.5),
          minRadiusFloor + spread * 0.5,
          localCap - spread * 0.5
        );
        let radiusLow = clampLayout(Math.max(minRadiusFloor, q10 - Math.max(16, iqr * 0.45)), minRadiusFloor, localCap - 8);
        let radiusHigh = clampLayout(
          Math.max(radiusLow + 8, q90 + Math.max(20, iqr * 0.66)),
          radiusLow + 8,
          localCap
        );
        const spreadNeeded = (layerCount - 1) * Math.max(10, bandGap * 0.9);
        const currentSpread = Math.max(0, radiusHigh - radiusLow);
        if (currentSpread < spreadNeeded) {
          const missing = spreadNeeded - currentSpread;
          const canGrowUp = Math.max(0, localCap - radiusHigh);
          const growUp = Math.min(canGrowUp, missing * 0.75);
          const canGrowDown = Math.max(0, radiusLow - minRadiusFloor);
          const growDown = Math.min(canGrowDown, Math.max(0, missing - growUp));
          radiusHigh += growUp;
          radiusLow -= growDown;
        }
        const radiusCenter = clampLayout(
          radiusCenterBase,
          radiusLow + Math.min(8, (radiusHigh - radiusLow) * 0.28),
          radiusHigh - Math.min(8, (radiusHigh - radiusLow) * 0.28)
        );
        const spanRadius = Math.max(8, radiusHigh - radiusLow);
        const layerRadii = [];
        for (let li = 0; li < layerCount; li += 1) {
          if (layerCount <= 1) {
            layerRadii.push(radiusCenter);
          } else {
            const t = li / Math.max(1, layerCount - 1);
            layerRadii.push(clampLayout(radiusLow + spanRadius * t, radiusLow, radiusHigh));
          }
        }
        const layerCaps = layerRadii.map((rr) =>
          Math.max(2, Math.floor((usableSpan * Math.max(rr, minRadiusFloor + 12)) / nominalArcGap))
        );
        const allocated = allocateLayerCounts(groupCount, layerCaps);
        const minLayerNodes =
          groupCount >= 120 ? 7 : groupCount >= 90 ? 6 : groupCount >= 56 ? 5 : groupCount >= 28 ? 4 : 3;
        const sparseBefore = allocated.filter((n) => n > 0 && n < minLayerNodes).length;
        const donorOrderByCount = () =>
          allocated
            .map((value, idx) => ({ idx, value }))
            .sort((a, b) => b.value - a.value || a.idx - b.idx);
        for (let iter = 0; iter < 2048; iter += 1) {
          const sparse = allocated
            .map((value, idx) => ({ idx, value }))
            .filter((row) => row.value > 0 && row.value < minLayerNodes)
            .sort((a, b) => a.value - b.value || a.idx - b.idx)[0];
          if (!sparse) break;
          const donor = donorOrderByCount().find((row) => row.value > minLayerNodes + 1);
          if (!donor) break;
          allocated[donor.idx] -= 1;
          allocated[sparse.idx] += 1;
          sparseLayerRebalances += 1;
        }
        const sparseAfter = allocated.filter((n) => n > 0 && n < minLayerNodes).length;
        if (sparseAfter > sparseBefore) sparseLayerRebalances += 1;
        const orderedEntries = groupRows
          .slice()
          .sort((a, b) => {
            const ar = Number(a?.rel) || 0;
            const br = Number(b?.rel) || 0;
            if (Math.abs(ar - br) > 1e-9) return ar - br;
            return String(a?.zone?.id || "").localeCompare(String(b?.zone?.id || ""), "zh-CN");
          });
        const assignments = [];
        let entryCursor = 0;
        for (let li = 0; li < allocated.length && entryCursor < orderedEntries.length; li += 1) {
          const count = Math.max(0, Number(allocated[li]) || 0);
          if (!count) continue;
          const phase = stableHashUnit(`sector-uniform-phase|${centerId}|${sectorIndex}|${groupIndex}|${li}`);
          const layerSpan = clampLayout(usableSpan * layerSpanScaleForCount(count), 0.04, usableSpan);
          const layerStartRel = clampLayout(
            usableStartRel + (usableSpan - layerSpan) * 0.5,
            usableStartRel,
            usableEndRel - layerSpan
          );
          for (let i = 0; i < count && entryCursor < orderedEntries.length; i += 1) {
            const entry = orderedEntries[entryCursor];
            const ratio = count <= 1 ? 0.5 : ((i + 0.5 + phase) % count) / count;
            const targetRel = layerStartRel + layerSpan * ratio;
            const targetAngle = clampAngleToArc(Number(sector?.start) + targetRel, sector?.start, span);
            const radialNoise =
              (stableHashUnit(`sector-uniform-r|${centerId}|${entry?.zone?.id || ""}`) - 0.5) * jitterAmp * 0.65;
            const targetRadius = clampLayout(layerRadii[li] + radialNoise, radiusLow, radiusHigh);
            assignments.push({
              entry,
              layerIndex: li,
              countInLayer: count,
              targetAngle,
              targetRadius,
            });
            entryCursor += 1;
          }
        }
        while (entryCursor < orderedEntries.length) {
          const entry = orderedEntries[entryCursor];
          const li = Math.max(0, Math.min(layerRadii.length - 1, entryCursor % Math.max(1, layerRadii.length)));
          const ratio = orderedEntries.length <= 1 ? 0.5 : (entryCursor + 0.5) / orderedEntries.length;
          const targetRel = usableStartRel + usableSpan * ratio;
          const targetAngle = clampAngleToArc(Number(sector?.start) + targetRel, sector?.start, span);
          const targetRadius = clampLayout(layerRadii[li], radiusLow, radiusHigh);
          assignments.push({
            entry,
            layerIndex: li,
            countInLayer: Math.max(1, allocated[li] || 1),
            targetAngle,
            targetRadius,
          });
          entryCursor += 1;
        }
        const peerIds = new Set(groupRows.map((row) => String(row?.zone?.id || "")).filter(Boolean));
        const ignoreIds = peerIds;
        const externalRows = visibleZones.filter(
          (zone) => zone && !ignoreIds.has(String(zone?.id || ""))
        );
        const scoringCtx = {
          externalZoneIndex: createZoneSpatialIndex(externalRows, { cellSize: 172 }),
          draftZoneIndex: createZoneSpatialIndex([], { cellSize: 172 }),
          externalRayIndex: createRayPenaltyIndex(centerX, centerY, externalRows, {
            bucketCount: 240,
          }),
          draftRayIndex: createRayPenaltyIndex(centerX, centerY, [], {
            bucketCount: 240,
          }),
        };
        const draftZones = [];
        assignments.forEach((item) => {
          planned += 1;
          const entry = item?.entry;
          const node = entry?.node;
          const zone = entry?.zone;
          if (!entry || !node || !zone) return;
          const targetAngle = Number(item?.targetAngle) || 0;
          const targetRadius = Number(item?.targetRadius) || 0;
          const countInLayer = Math.max(1, Number(item?.countInLayer) || 1);
          const localAngleStep = clampLayout(usableSpan / Math.max(14, countInLayer * 1.25), 0.006, 0.075);
          const localRadialStep = clampLayout(Math.max(6, spanRadius / Math.max(4, layerCount * 1.5)), 6, 24);
          const localAngleOffsets = [
            0,
            localAngleStep,
            -localAngleStep,
            localAngleStep * 2,
            -localAngleStep * 2,
            localAngleStep * 3,
            -localAngleStep * 3,
            localAngleStep * 4,
            -localAngleStep * 4,
            localAngleStep * 5,
            -localAngleStep * 5,
          ];
          const localRadialOffsets = [
            0,
            -localRadialStep,
            localRadialStep,
            -localRadialStep * 2,
            localRadialStep * 2,
            -localRadialStep * 3,
            localRadialStep * 3,
            -localRadialStep * 4,
            localRadialStep * 4,
          ];
          const currentX = Number(zone?.x) || 0;
          const currentY = Number(zone?.y) || 0;
          const currentAngle = Math.atan2(currentY - centerY, currentX - centerX);
          const currentRadius = Math.hypot(currentX - centerX, currentY - centerY);
          const currentPenalty = scoreCandidate(
            currentX,
            currentY,
            node,
            sector,
            targetAngle,
            targetRadius,
            ignoreIds,
            draftZones,
            scoringCtx
          );
          let best = null;
          let bestPenalty = Number.POSITIVE_INFINITY;
          let usedFallback = false;
          for (const rrShift of localRadialOffsets) {
            const rr = clampLayout(targetRadius + rrShift, radiusLow, radiusHigh);
            for (const aaShift of localAngleOffsets) {
              const angle = clampAngleToArc(targetAngle + aaShift, sector?.start, span);
              const x = centerX + Math.cos(angle) * rr;
              const y = centerY + Math.sin(angle) * rr;
              const penalty = scoreCandidate(
                x,
                y,
                node,
                sector,
                targetAngle,
                targetRadius,
                ignoreIds,
                draftZones,
                scoringCtx
              );
              if (!(penalty < bestPenalty)) continue;
              bestPenalty = penalty;
              best = { x, y, angle, radius: rr };
              if (penalty <= 0.08) break;
            }
            if (bestPenalty <= 0.08) break;
          }
          if (!best || !Number.isFinite(bestPenalty)) {
            let fallbackBest = null;
            let fallbackPenalty = Number.POSITIVE_INFINITY;
            const fallbackPhase = stableHashUnit(
              `sector-uniform-fallback|${centerId}|${sectorIndex}|${groupIndex}|${zone?.id || ""}`
            );
            const sweepCount = Math.max(
              64,
              Math.min(360, Math.round(Math.max(72, usableSpan / Math.max(0.01, localAngleStep) * 1.6)))
            );
            const radialStride = clampLayout(Math.max(8, localRadialStep * 0.9), 8, 36);
            const radialCandidates = [];
            for (let ri = 0; ri <= 14; ri += 1) {
              const inward = clampLayout(targetRadius - ri * radialStride, radiusLow, radiusHigh);
              radialCandidates.push(inward);
              if (ri > 0) {
                const outward = clampLayout(targetRadius + ri * radialStride, radiusLow, radiusHigh);
                radialCandidates.push(outward);
              }
            }
            const seenRadius = new Set();
            const uniqueRadials = radialCandidates.filter((rr) => {
              const key = String(Number(rr).toFixed(2));
              if (seenRadius.has(key)) return false;
              seenRadius.add(key);
              return true;
            });
            for (const rr of uniqueRadials) {
              for (let si = 0; si < sweepCount; si += 1) {
                const ratio = ((si + fallbackPhase) % sweepCount) / sweepCount;
                const targetRel = usableStartRel + usableSpan * ratio;
                const angle = clampAngleToArc(Number(sector?.start) + targetRel, sector?.start, span);
                const x = centerX + Math.cos(angle) * rr;
                const y = centerY + Math.sin(angle) * rr;
                const penalty = scoreCandidate(
                  x,
                  y,
                  node,
                  sector,
                  targetAngle,
                  targetRadius,
                  ignoreIds,
                  draftZones,
                  scoringCtx
                );
                if (!(penalty < fallbackPenalty)) continue;
                fallbackPenalty = penalty;
                fallbackBest = { x, y, angle, radius: rr };
                if (penalty <= 0.08) break;
              }
              if (fallbackPenalty <= 0.08) break;
            }
            if (fallbackBest && Number.isFinite(fallbackPenalty)) {
              best = fallbackBest;
              bestPenalty = fallbackPenalty;
              usedFallback = true;
              fallbackRescues += 1;
            }
          }
          if (!best || !Number.isFinite(bestPenalty)) {
            rejected += 1;
            const draftZone = {
              id: String(zone?.id || ""),
              x: currentX,
              y: currentY,
              r: Number(zone?.r) || nodeCollisionRadius(node),
            };
            draftZones.push(draftZone);
            scoringCtx.draftZoneIndex.addZone(draftZone);
            scoringCtx.draftRayIndex.addZone(draftZone);
            return;
          }
          const angularDrift = angleDiff(currentAngle, targetAngle);
          const radialDrift = Math.abs(currentRadius - targetRadius);
          const shouldApply =
            usedFallback ||
            !Number.isFinite(currentPenalty) ||
            bestPenalty + 0.06 < currentPenalty ||
            angularDrift > localAngleStep * 0.65 ||
            radialDrift > localRadialStep * 0.65;
          if (shouldApply) {
            zone.x = Number(best.x) || 0;
            zone.y = Number(best.y) || 0;
            adjusted += 1;
          }
          const draftZone = {
            id: String(zone?.id || ""),
            x: Number(zone?.x) || 0,
            y: Number(zone?.y) || 0,
            r: Number(zone?.r) || nodeCollisionRadius(node),
          };
          draftZones.push(draftZone);
          scoringCtx.draftZoneIndex.addZone(draftZone);
          scoringCtx.draftRayIndex.addZone(draftZone);
        });
      });
    });

    return { adjusted, planned, rejected, sparseLayerRebalances, fallbackRescues };
  }

  function allocateLayerCounts(totalCount, layerCaps) {
    const count = Math.max(0, Number(totalCount) || 0);
    const caps = (Array.isArray(layerCaps) ? layerCaps : [])
      .map((value) => Math.max(0, Number(value) || 0))
      .filter((value) => value > 0);
    if (!count || !caps.length) return [];
    const totalCap = Math.max(1, caps.reduce((sum, value) => sum + value, 0));
    const target = caps.map((value) => (value / totalCap) * count);
    const allocated = target.map((value) => Math.max(0, Math.floor(value)));
    let allocatedTotal = allocated.reduce((sum, value) => sum + value, 0);
    if (allocatedTotal < count) {
      const residues = target
        .map((value, idx) => ({ idx, residue: value - Math.floor(value) }))
        .sort((a, b) => b.residue - a.residue || a.idx - b.idx);
      let cursor = 0;
      while (allocatedTotal < count && residues.length) {
        allocated[residues[cursor % residues.length].idx] += 1;
        allocatedTotal += 1;
        cursor += 1;
      }
    } else if (allocatedTotal > count) {
      const order = allocated
        .map((value, idx) => ({ idx, value }))
        .sort((a, b) => b.value - a.value || a.idx - b.idx);
      let cursor = 0;
      while (allocatedTotal > count && order.length) {
        const pick = order[cursor % order.length];
        if (allocated[pick.idx] > 0) {
          allocated[pick.idx] -= 1;
          allocatedTotal -= 1;
        }
        cursor += 1;
        if (cursor > count + order.length + 8) break;
      }
    }
    return allocated;
  }

  function layerSpanScaleForCount(count) {
    if (count >= 16) return 0.94;
    if (count >= 12) return 0.9;
    if (count >= 10) return 0.86;
    if (count === 9) return 0.82;
    if (count === 8) return 0.78;
    if (count === 7) return 0.74;
    if (count === 6) return 0.68;
    if (count === 5) return 0.62;
    if (count === 4) return 0.54;
    if (count === 3) return 0.44;
    if (count === 2) return 0.34;
    return 0.24;
  }

  function planNetworkSectorFastRingLayers(leafCount, sectorSpan, radiusLow, radiusHigh, nominalArcGap) {
    const count = Math.max(0, Number(leafCount) || 0);
    if (!count) return { leafCount: 0, layerCount: 0, totalCount: 0, layers: [] };
    const span = Math.max(0.05, Number(sectorSpan) || 0);
    const low = Math.max(1, Number(radiusLow) || 1);
    const high = Math.max(low + 16, Number(radiusHigh) || low);
    const arcGap = Math.max(1, Number(nominalArcGap) || 1);
    const midRadius = (low + high) * 0.5;
    const midCapacity = Math.max(2, Math.floor((span * Math.max(midRadius, low + 12)) / arcGap));
    const layerCount = Math.max(
      1,
      Math.min(resolveNetworkSectorLayerLimit(count), Math.ceil(count / Math.max(1, midCapacity)))
    );
    const radii = [];
    for (let li = 0; li < layerCount; li += 1) {
      const t = layerCount <= 1 ? 0.5 : li / Math.max(1, layerCount - 1);
      radii.push(clampLayout(low + (high - low) * t, low, high));
    }
    const caps = radii.map((rr) =>
      Math.max(2, Math.floor((span * Math.max(rr, low + 12)) / arcGap))
    );
    const allocated = allocateLayerCounts(count, caps);
    const layers = radii
      .map((radius, idx) => ({
        radius,
        count: Math.max(0, Number(allocated[idx]) || 0),
      }))
      .filter((row) => row.count > 0)
      .map((row) => ({
        ...row,
        spanScale: layerSpanScaleForCount(row.count),
      }));
    return {
      leafCount: count,
      layerCount: layers.length,
      totalCount: layers.reduce((sum, row) => sum + row.count, 0),
      layers,
    };
  }

  function placeLeafSectorsFastRing(centerNode, leafRows, sectorPlan, scoredBuckets, occupiedZones, params = {}) {
    const rows = Array.isArray(leafRows) ? leafRows.filter((row) => row?.node) : [];
    if (rows.length < 36) return null;
    const sectors = Array.isArray(sectorPlan?.sectors) ? sectorPlan.sectors : [];
    if (sectors.length !== 1) return null;
    const hardZones = Array.isArray(params?.hardZones) ? params.hardZones : [];
    const hardCorridors = Array.isArray(params?.hardCorridors) ? params.hardCorridors : [];
    const ownerCenters = Array.isArray(params?.ownerCenters) ? params.ownerCenters : [];
    if (params?.strictCenterOwnership !== false && ownerCenters.length > 1) return null;

    const cfg = params?.config || DEFAULT_CFG;
    const sector = sectors[0];
    const span = Math.max(0, Number(sector?.end) - Number(sector?.start));
    if (!(span > 0.12)) return null;
    const centerX = Number(centerNode?.x) || 0;
    const centerY = Number(centerNode?.y) || 0;
    const centerId = String(centerNode?.id || "");
    const sourceOccupiedZones = Array.isArray(occupiedZones) ? occupiedZones : [];
    const canReturnNullAfterPartialPlacement = hardZones.length > 0 || hardCorridors.length > 0;
    const visibleZones = canReturnNullAfterPartialPlacement ? sourceOccupiedZones.slice() : sourceOccupiedZones;
    const zoneIndex = createZoneSpatialIndex(visibleZones, { cellSize: 148 });
    const hardZoneIndex = hardZones.length ? createZoneSpatialIndex(hardZones, { cellSize: 172 }) : null;
    const hardCorridorIndex = hardCorridors.length
      ? createCorridorSpatialIndex(hardCorridors, { cellSize: 220 })
      : null;
    const { hasTerritoryConstraint, isInTerritory } = createTerritoryAngleChecker(params?.territoryRanges || []);
    const leafCount = rows.length;
    const avgLabelPad =
      Number(sectorPlan?.avgLabelPad) ||
      rows.reduce((sum, row) => sum + (Number(row?.labelPad) || labelPad(row.node)), 0) / Math.max(1, leafCount);
    const avgNodeRadius = rows.reduce((sum, row) => sum + nodeRadius(row.node), 0) / Math.max(1, leafCount);
    const avgCollision = rows.reduce((sum, row) => sum + nodeCollisionRadius(row.node), 0) / Math.max(1, leafCount);
    const minRadiusBase = Math.max(80, Number(sectorPlan?.minRadiusBase) || Number(cfg.R_LEAF_MIN) || 220);
    const minRadiusBoost = Math.max(1, Math.min(2.2, 1 + Math.sqrt(leafCount) / 15));
    const denseRadiusScale = clampLayout(1 - Math.sqrt(Math.max(1, leafCount)) / 26, 0.58, 0.86);
    const centerCollision = nodeCollisionRadius(centerNode?.node || centerNode);
    const radiusLow = Math.max(
      minRadiusBase * minRadiusBoost * denseRadiusScale * 0.78,
      centerCollision + avgCollision + 24
    );
    const nominalArcGap = resolveNetworkNominalArcGap(
      leafCount,
      avgLabelPad,
      avgNodeRadius,
      avgCollision
    );
    const layerLimit = resolveNetworkSectorLayerLimit(leafCount);
    const layerGap = Math.max(22, Number(cfg.NETWORK_SECTOR_LAYER_GAP) || 68);
    const edgePad = clampLayout(span * 0.018, 0.012, 0.12);
    const usableStartRel = edgePad;
    const usableEndRel = Math.max(edgePad + 0.05, span - edgePad);
    const usableSpan = Math.max(0.05, usableEndRel - usableStartRel);
    const perLayerTarget = Math.max(2, Math.ceil(leafCount / Math.max(1, layerLimit)));
    const radiusForArcCapacity = (perLayerTarget * nominalArcGap) / Math.max(0.12, usableSpan);
    const denseRadiusHighFloor =
      radiusLow +
      Math.max(
        120,
        Math.sqrt(Math.max(1, leafCount)) * 34,
        Math.max(0, layerLimit - 1) * layerGap * 0.52,
        radiusForArcCapacity * 0.42
      );
    const maxLeafRadiusInput = Number(params?.maxLeafRadius);
    const radiusHigh = Number.isFinite(maxLeafRadiusInput)
      ? Math.max(radiusLow + 16, maxLeafRadiusInput, denseRadiusHighFloor)
      : denseRadiusHighFloor;
    const ringPlan = planNetworkSectorFastRingLayers(
      leafCount,
      usableSpan,
      radiusLow,
      radiusHigh,
      nominalArcGap
    );
    if (!ringPlan.layers.length || ringPlan.totalCount !== leafCount) return null;

    const out = {
      positions: new Map(),
      leafZones: [],
      report: {
        centerId,
        leafMode: "sector",
        sectors: sectors.map((row) => ({
          start: Number(Number(row?.start || 0).toFixed(3)),
          end: Number(Number(row?.end || 0).toFixed(3)),
          score: Number((Number(row?.score) || 0).toFixed(3)),
        })),
        bucketTopScore: sectorPlan?.bucketTopScore ?? null,
        avoidanceHits: 0,
        boundaryPenaltyTriggered: (Array.isArray(scoredBuckets) ? scoredBuckets : []).some(
          (row) => Number(row?.boundaryPenalty) > 0
        ),
        territoryRangeCount: Array.isArray(params?.territoryRanges) ? params.territoryRanges.length : 0,
        hardRejectHits: 0,
        corridorRejectHits: 0,
        territoryOverflowHits: 0,
        maxLeafRadius: Number.isFinite(radiusHigh) ? Number(radiusHigh.toFixed(2)) : null,
        envelopeAdjustments: 0,
        envelopeRadiusPulls: 0,
        envelopeAnglePulls: 0,
        uniformAdjustments: 0,
        uniformPlanned: 0,
        uniformRejected: 0,
        uniformSparseLayerRebalances: 0,
        uniformFallbackRescues: 0,
        sectorFastPath: "single-sector-ring",
      },
    };

    const angleOffsetSteps = [0, 1, -1, 2, -2, 3, -3, 4, -4, 5, -5];
    const radialOffsetSteps = [0, 1, -1];
    let cursor = 0;
    for (let li = 0; li < ringPlan.layers.length && cursor < rows.length; li += 1) {
      const layer = ringPlan.layers[li];
      const count = Math.max(0, Number(layer?.count) || 0);
      if (!count) continue;
      const radius = Number(layer?.radius) || radiusLow;
      const layerSpan = clampLayout(
        usableSpan * (Number(layer?.spanScale) || layerSpanScaleForCount(count)),
        0.04,
        usableSpan
      );
      const layerStartRel = clampLayout(
        usableStartRel + (usableSpan - layerSpan) * 0.5,
        usableStartRel,
        usableEndRel - layerSpan
      );
      const phase = stableHashUnit(`network-fast-sector|${centerId}|${li}`);
      const angleStep = clampLayout(layerSpan / Math.max(10, count * 1.25), 0.008, 0.075);
      const radialStep = clampLayout((radiusHigh - radiusLow) / Math.max(6, ringPlan.layers.length * 1.8), 6, 22);
      for (let i = 0; i < count && cursor < rows.length; i += 1) {
        const item = rows[cursor];
        cursor += 1;
        const node = item.node;
        const ratio = count <= 1 ? 0.5 : ((i + 0.5 + phase) % count) / count;
        const targetAngle = clampAngleToArc(
          Number(sector?.start) + layerStartRel + layerSpan * ratio,
          sector?.start,
          span
        );
        const jitter =
          (stableHashUnit(`network-fast-radius|${centerId}|${item.id || node?.id || ""}`) - 0.5) *
          radialStep *
          0.72;
        let best = null;
        for (const rrMul of radialOffsetSteps) {
          const rr = clampLayout(radius + jitter + rrMul * radialStep, radiusLow, radiusHigh);
          for (const aaMul of angleOffsetSteps) {
            const angle = clampAngleToArc(targetAngle + aaMul * angleStep, sector?.start, span);
            if (hasTerritoryConstraint && !isInTerritory(angle)) continue;
            const x = centerX + Math.cos(angle) * rr;
            const y = centerY + Math.sin(angle) * rr;
            if (hardZoneIndex && collidesWithZones(x, y, node, hardZones, 12, hardZoneIndex)) {
              out.report.hardRejectHits += 1;
              continue;
            }
            if (hardCorridorIndex && collidesWithCorridors(x, y, node, hardCorridors, 12, hardCorridorIndex)) {
              out.report.corridorRejectHits += 1;
              continue;
            }
            if (!collidesWithZones(x, y, node, visibleZones, 8, zoneIndex)) {
              best = { x, y };
              break;
            }
            out.report.avoidanceHits += 1;
          }
          if (best) break;
        }
        if (!best) {
          let angle = targetAngle;
          if (hasTerritoryConstraint && !isInTerritory(angle)) {
            angle = pickRepresentativeAngle(params?.territoryRanges || [], targetAngle);
          }
          best = {
            x: centerX + Math.cos(angle) * radius,
            y: centerY + Math.sin(angle) * radius,
          };
          if (
            (hardZoneIndex && collidesWithZones(best.x, best.y, item.node, hardZones, 12, hardZoneIndex)) ||
            (hardCorridorIndex &&
              collidesWithCorridors(best.x, best.y, item.node, hardCorridors, 12, hardCorridorIndex))
          ) {
            return null;
          }
        }
        out.positions.set(node.id, best);
        const zone = {
          ...makeZone(node.id, best.x, best.y, node, 4),
          ownerCenterId: centerId,
          ownerSectorIndex: 0,
        };
        visibleZones.push(zone);
        zoneIndex.addZone(zone);
        out.leafZones.push(zone);
      }
    }
    if (canReturnNullAfterPartialPlacement) {
      sourceOccupiedZones.push(...out.leafZones);
    }
    return out;
  }

  function placeNetworkOutwardLeafSectors(centerNode, leafRows, occupiedZones, params = {}) {
    const cfg = params?.config || DEFAULT_CFG;
    const rows = (Array.isArray(leafRows) ? leafRows : [])
      .filter((row) => row?.node)
      .slice()
      .sort((a, b) => String(a?.id || a?.node?.id || "").localeCompare(String(b?.id || b?.node?.id || ""), "zh-CN"));
    const centerId = String(centerNode?.id || "");
    const centerX = Number(centerNode?.x) || 0;
    const centerY = Number(centerNode?.y) || 0;
    const leafCount = rows.length;
    const outwardAngleInput = Number(params?.outwardAngle);
    const outwardAngle = Number.isFinite(outwardAngleInput)
      ? outwardAngleInput
      : stableHashUnit(`network-outward-angle|${centerId}`) * Math.PI * 2;
    const outwardSpanInput = Number(params?.outwardSpan);
    const outwardSpan = clampLayout(
      Number.isFinite(outwardSpanInput)
        ? outwardSpanInput
        : 1.08 + Math.sqrt(Math.max(1, leafCount)) * 0.17,
      0.82,
      Math.PI * 0.92
    );
    const sectorStart = outwardAngle - outwardSpan * 0.5;
    const sectorEnd = sectorStart + outwardSpan;
    const sourceOccupiedZones = Array.isArray(occupiedZones) ? occupiedZones : [];
    const hardZones = Array.isArray(params?.hardZones) ? params.hardZones : [];
    const hardCorridors = Array.isArray(params?.hardCorridors) ? params.hardCorridors : [];
    const ownerCenters = Array.isArray(params?.ownerCenters) ? params.ownerCenters : [];
    const strictCenterOwnership = params?.strictCenterOwnership !== false;
    const ownsPoint = makeCenterOwnershipChecker(
      centerId,
      centerX,
      centerY,
      ownerCenters,
      strictCenterOwnership
    );
    const blockerZones = sourceOccupiedZones.concat(hardZones);
    const zoneIndex = createZoneSpatialIndex(blockerZones, { cellSize: 176 });
    const hardCorridorIndex = hardCorridors.length
      ? createCorridorSpatialIndex(hardCorridors, { cellSize: 240 })
      : null;
    const avgLabelPad =
      rows.reduce((sum, row) => sum + (Number(row?.labelPad) || labelPad(row.node)), 0) / Math.max(1, leafCount);
    const avgNodeRadius = rows.reduce((sum, row) => sum + nodeRadius(row.node), 0) / Math.max(1, leafCount);
    const avgCollision =
      rows.reduce((sum, row) => sum + nodeCollisionRadius(row.node), 0) / Math.max(1, leafCount);
    const centerCollision = nodeCollisionRadius(centerNode?.node || centerNode);
    const minRadiusBase = Math.max(
      Number(cfg.R_LEAF_MIN) || 220,
      centerCollision + avgCollision + Math.max(44, avgLabelPad * 0.42)
    );
    const nominalArcGap = resolveNetworkNominalArcGap(
      leafCount,
      avgLabelPad,
      avgNodeRadius,
      avgCollision
    );
    const layerGap = Math.max(36, Number(cfg.NETWORK_SECTOR_LAYER_GAP) || 68, avgCollision * 0.78);
    const layerLimit = resolveNetworkSectorLayerLimit(leafCount);
    const maxLeafRadiusInput = Number(params?.maxLeafRadius);
    const radiusHigh = Number.isFinite(maxLeafRadiusInput)
      ? Math.max(minRadiusBase + layerGap * 2, maxLeafRadiusInput)
      : minRadiusBase + Math.max(2, layerLimit - 1) * layerGap;
    const out = {
      positions: new Map(),
      leafZones: [],
      report: {
        centerId,
        leafMode: "sector",
        sectors: [
          {
            start: Number(sectorStart.toFixed(3)),
            end: Number(sectorEnd.toFixed(3)),
            score: null,
          },
        ],
        bucketTopScore: null,
        avoidanceHits: 0,
        boundaryPenaltyTriggered: false,
        territoryRangeCount: 1,
        hardRejectHits: 0,
        corridorRejectHits: 0,
        territoryOverflowHits: 0,
        maxLeafRadius: Number(radiusHigh.toFixed(2)),
        envelopeAdjustments: 0,
        envelopeRadiusPulls: 0,
        envelopeAnglePulls: 0,
        uniformAdjustments: 0,
        uniformPlanned: 0,
        uniformRejected: 0,
        uniformSparseLayerRebalances: 0,
        uniformFallbackRescues: 0,
        sectorFastPath: "network-outward-sector",
      },
    };
    if (!rows.length) return out;

    let cursor = 0;
    let layer = 0;
    const maxPerLayer = Math.max(2, Math.ceil(Math.sqrt(Math.max(1, leafCount)) * 2.35));
    while (cursor < rows.length) {
      const radius = Math.min(
        radiusHigh + Math.max(0, layer - layerLimit + 1) * layerGap * 0.72,
        minRadiusBase + layer * layerGap
      );
      const capacityByArc = Math.max(1, Math.floor((outwardSpan * radius) / Math.max(32, nominalArcGap)));
      const count = Math.min(rows.length - cursor, Math.max(1, Math.min(maxPerLayer, capacityByArc)));
      const layerSpan = clampLayout(
        outwardSpan * clampLayout(0.76 + count / Math.max(8, maxPerLayer * 1.9), 0.64, 1),
        0.42,
        outwardSpan
      );
      const layerStart = sectorStart + (outwardSpan - layerSpan) * 0.5;
      const phase = (stableHashUnit(`network-outward-layer|${centerId}|${layer}`) - 0.5) *
        Math.min(0.18, layerSpan / Math.max(4, count * 2));
      for (let i = 0; i < count && cursor < rows.length; i += 1) {
        const item = rows[cursor];
        cursor += 1;
        const node = item.node;
        const id = String(item?.id || node?.id || "");
        const slotRatio = count <= 1 ? 0.5 : (i + 0.5) / count;
        const baseAngle =
          layerStart +
          layerSpan * slotRatio +
          phase +
          (stableHashUnit(`network-outward-angle-jitter|${centerId}|${id}`) - 0.5) *
            Math.min(0.16, layerSpan / Math.max(3, count));
        const baseRadius =
          radius +
          (stableHashUnit(`network-outward-radius-jitter|${centerId}|${id}`) - 0.5) *
            Math.min(layerGap * 0.36, 28);
        let best = null;
        for (let attempt = 0; attempt < 16; attempt += 1) {
          const side = attempt % 2 === 0 ? 1 : -1;
          const rank = Math.floor((attempt + 1) / 2);
          const angle = clampAngleToArc(
            baseAngle + side * rank * Math.min(0.13, outwardSpan * 0.055),
            sectorStart,
            outwardSpan
          );
          const rr = Math.max(
            minRadiusBase,
            baseRadius + Math.floor(attempt / 3) * Math.max(16, layerGap * 0.38)
          );
          const x = centerX + Math.cos(angle) * rr;
          const y = centerY + Math.sin(angle) * rr;
          const ownershipOk = ownsPoint(x, y);
          if (!ownershipOk && attempt < 12) {
            out.report.territoryOverflowHits += 1;
            continue;
          }
          if (hardCorridorIndex && collidesWithCorridors(x, y, node, hardCorridors, 12, hardCorridorIndex)) {
            out.report.corridorRejectHits += 1;
            continue;
          }
          if (collidesWithZones(x, y, node, blockerZones, 10, zoneIndex)) {
            out.report.avoidanceHits += 1;
            continue;
          }
          best = { x, y };
          break;
        }
        if (!best) {
          const angle = clampAngleToArc(baseAngle, sectorStart, outwardSpan);
          const rr = Math.max(minRadiusBase, baseRadius + layerGap * 1.4);
          best = {
            x: centerX + Math.cos(angle) * rr,
            y: centerY + Math.sin(angle) * rr,
          };
        }
        out.positions.set(node.id, best);
        const zone = {
          ...makeZone(node.id, best.x, best.y, node, 5),
          ownerCenterId: centerId,
          ownerSectorIndex: 0,
        };
        sourceOccupiedZones.push(zone);
        blockerZones.push(zone);
        zoneIndex.addZone(zone);
        out.leafZones.push(zone);
      }
      layer += 1;
    }
    out.report.maxLeafRadius = Number(
      Math.max(
        ...out.leafZones.map((zone) => Math.hypot((Number(zone.x) || 0) - centerX, (Number(zone.y) || 0) - centerY)),
        radiusHigh
      ).toFixed(2)
    );
    return out;
  }

  function placeLeafSectors(centerNode, bucketProjection, occupiedZones, params = {}) {
    const cfg = params?.config || DEFAULT_CFG;
    const territoryRanges = Array.isArray(params?.territoryRanges) ? params.territoryRanges : [];
    const hardZones = Array.isArray(params?.hardZones) ? params.hardZones : [];
    const hardCorridors = Array.isArray(params?.hardCorridors) ? params.hardCorridors : [];
    const preferredAngles = Array.isArray(params?.preferredAngles) ? params.preferredAngles : [];
    const allowTerritoryOverflow = !!params?.allowTerritoryOverflow;
    const strictCenterOwnership = params?.strictCenterOwnership !== false;
    const ownerCenters = Array.isArray(params?.ownerCenters) ? params.ownerCenters : [];
    const maxLeafRadiusInput = Number(params?.maxLeafRadius);
    const centerX = Number(centerNode?.x) || 0;
    const centerY = Number(centerNode?.y) || 0;
    const centerId = String(centerNode?.id || "");
    const sourceLeafRows = Array.isArray(bucketProjection?.leafRows) ? bucketProjection.leafRows : null;
    if (!sourceLeafRows) {
      throw new Error("network-bucket-sector-projection-missing");
    }
    const leafRows = sourceLeafRows
      .map((row) => {
        const node = row?.node || null;
        if (!node) return null;
        return {
          node,
          id: String(row?.id || node?.id || ""),
          labelPad: Number.isFinite(Number(row?.labelPad)) ? Number(row.labelPad) : labelPad(node),
        };
      })
      .filter(Boolean);
    const rows = leafRows.map((row) => row.node);
    const out = {
      positions: new Map(),
      leafZones: [],
      report: {
        centerId,
        leafMode: "sector",
        sectors: [],
        bucketTopScore: null,
        avoidanceHits: 0,
        boundaryPenaltyTriggered: false,
        territoryRangeCount: territoryRanges.length,
        hardRejectHits: 0,
        corridorRejectHits: 0,
        territoryOverflowHits: 0,
        fallbackPlacements: 0,
        pushoutPlacements: 0,
        maxLeafRadius: null,
        envelopeAdjustments: 0,
        envelopeRadiusPulls: 0,
        envelopeAnglePulls: 0,
        uniformAdjustments: 0,
        uniformPlanned: 0,
        uniformRejected: 0,
        uniformSparseLayerRebalances: 0,
        uniformFallbackRescues: 0,
      },
    };
    const scoredBuckets = Array.isArray(bucketProjection?.scoredBuckets) ? bucketProjection.scoredBuckets : null;
    const sectorPlan = bucketProjection?.sectorPlan || null;
    if (!scoredBuckets || !sectorPlan || !Array.isArray(sectorPlan.sectors)) {
      throw new Error("network-bucket-sector-projection-missing");
    }
    const projectedLeafCount = Math.max(0, Number(sectorPlan?.leafCount) || 0);
    if (!rows.length) {
      if (projectedLeafCount > 0 || sourceLeafRows.length > 0) {
        throw new Error("network-bucket-sector-projection-missing");
      }
      return out;
    }
    if (projectedLeafCount > 0 && projectedLeafCount !== rows.length) {
      throw new Error("network-bucket-sector-projection-missing");
    }
    const fastRing = placeLeafSectorsFastRing(
      centerNode,
      leafRows,
      sectorPlan,
      scoredBuckets,
      occupiedZones,
      params
    );
    if (fastRing) return fastRing;

    const hardZoneIndex = hardZones.length ? createZoneSpatialIndex(hardZones, { cellSize: 172 }) : null;
    const hardCorridorIndex = hardCorridors.length
      ? createCorridorSpatialIndex(hardCorridors, { cellSize: 220 })
      : null;
    const placementOccupiedZoneIndex = createZoneSpatialIndex(occupiedZones, { cellSize: 172 });
    const placementRayIndex = createRayPenaltyIndex(centerX, centerY, occupiedZones, {
      ownerCenterId: centerId,
      bucketCount: 240,
    });
    const sectors = sectorPlan.sectors;
    out.report.bucketTopScore = sectorPlan.bucketTopScore;
    const placementPayload = buildNetworkSectorPlacementPayload(centerNode, bucketProjection, occupiedZones, {
      ...params,
      centerType: params?.centerType || centerNode?.centerType || "",
      ownerCenters,
      strictCenterOwnership,
      allowTerritoryOverflow,
      maxLeafRadius: Number.isFinite(maxLeafRadiusInput) ? maxLeafRadiusInput : null,
      includeGeneralBatches: false,
      includeGeneralUpdates: false,
    });
    placementPayload.includeGeneralBatches = false;
    placementPayload.includeGeneralUpdates = false;
    const sectorPlacementCacheDisabled = params?.disableSectorPlacementCache === true;
    if (!sectorPlacementCacheDisabled) {
      collectNetworkSectorPlacementPayload(params, placementPayload);
    }
    const sectorPlacementCache = sectorPlacementCacheDisabled ? null : resolveNetworkSectorPlacementCache(params);
    const requireSectorPlacementCacheHit =
      !sectorPlacementCacheDisabled && shouldRequireNetworkSectorPlacementCacheHit(sectorPlacementCache, params);
    if (requireSectorPlacementCacheHit && !sectorPlacementCache) {
      throw new Error("network-sector-placement-cache-required");
    }
    const cachedPlacement = readCachedNetworkSectorPlacement(sectorPlacementCache, placementPayload);
    let sectorPlacement = cachedPlacement.sectorPlacement;
    const cachedSectorPlacementHit = !!sectorPlacement;
    if (!sectorPlacement) {
      prefetchNetworkSectorPlacement(sectorPlacementCache, placementPayload, cachedPlacement.key);
    }
    if (!sectorPlacement && requireSectorPlacementCacheHit) {
      throw new Error("network-sector-placement-cache-miss");
    }
    if (!sectorPlacement) {
      sectorPlacement = projectNetworkSectorPlacementFromPlan(placementPayload, {
        includeBatches: false,
        includeUpdates: false,
        cloneOccupiedZones: "refs",
        hardZoneIndex,
        hardCorridorIndex,
        occupiedZoneIndex: placementOccupiedZoneIndex,
        rayIndex: placementRayIndex,
        onZone(zone) {
          placementOccupiedZoneIndex.addZone(zone);
          placementRayIndex.addZone(zone);
        },
      });
    }
    const generalPlacement =
      sectorPlacement?.generalBatchPlacements && typeof sectorPlacement.generalBatchPlacements === "object"
        ? sectorPlacement.generalBatchPlacements
        : null;
    if (!generalPlacement) {
      const firstSector = sectors[0] || {};
      const sectorStart = Number(firstSector?.start);
      const sectorEnd = Number(firstSector?.end);
      const fallbackSpan =
        Number.isFinite(sectorStart) && Number.isFinite(sectorEnd) && sectorEnd > sectorStart
          ? sectorEnd - sectorStart
          : null;
      const fallback = placeNetworkOutwardLeafSectors(centerNode, leafRows, occupiedZones, {
        ...params,
        outwardAngle:
          fallbackSpan != null ? sectorStart + fallbackSpan / 2 : Number(params?.preferredAngles?.[0]) || undefined,
        outwardSpan: fallbackSpan != null ? fallbackSpan : undefined,
        disableSectorPlacementCache: true,
      });
      if (fallback?.report && typeof fallback.report === "object") {
        fallback.report.sectorFastPath = "network-general-sector-placement-js-fallback";
        fallback.report.generalPlacementFallback = true;
      }
      return fallback;
    }
    const minRadius = Number(generalPlacement.minRadius) || 0;
    const maxLeafRadius =
      generalPlacement.maxLeafRadius == null ? Number.POSITIVE_INFINITY : Number(generalPlacement.maxLeafRadius);
    out.report.maxLeafRadius = Number.isFinite(maxLeafRadius) ? Number(maxLeafRadius.toFixed(2)) : null;
    applyNetworkSectorPlacementResult(out, sectorPlacement, { occupiedZones });
    const rowsById = new Map(rows.map((node) => [String(node?.id || ""), node]));
    const rustEnvelopePlacement =
      generalPlacement?.postEnvelopePlacement && typeof generalPlacement.postEnvelopePlacement === "object"
        ? generalPlacement.postEnvelopePlacement
        : null;
    if (cachedSectorPlacementHit) {
      const rustPostProcessResult = applyNetworkSectorPostProcessPlacementResult(out, generalPlacement);
      const postUniformEnvelope = rustPostProcessResult.postUniformEnvelopeResult;
      const uniformResult = rustPostProcessResult.uniformResult;
      out.report.envelopeAdjustments =
        (Number(rustPostProcessResult.envelopeResult?.adjusted) || 0) +
        (Number(postUniformEnvelope?.adjusted) || 0);
      out.report.envelopeRadiusPulls =
        (Number(rustPostProcessResult.envelopeResult?.pulledByRadius) || 0) +
        (Number(postUniformEnvelope?.pulledByRadius) || 0);
      out.report.envelopeAnglePulls =
        (Number(rustPostProcessResult.envelopeResult?.pulledByAngle) || 0) +
        (Number(postUniformEnvelope?.pulledByAngle) || 0);
      out.report.uniformAdjustments = Number(uniformResult?.adjusted) || 0;
      out.report.uniformPlanned = Number(uniformResult?.planned) || 0;
      out.report.uniformRejected = Number(uniformResult?.rejected) || 0;
      out.report.uniformSparseLayerRebalances = Number(uniformResult?.sparseLayerRebalances) || 0;
      out.report.uniformFallbackRescues = Number(uniformResult?.fallbackRescues) || 0;
      out.report.sectors = sectors.map((sector) => ({
        start: Number(sector.start.toFixed(3)),
        end: Number(sector.end.toFixed(3)),
        score: Number((sector.score || 0).toFixed(3)),
      }));
      out.report.boundaryPenaltyTriggered = (Array.isArray(scoredBuckets) ? scoredBuckets : []).some(
        (row) => Number(row?.boundaryPenalty) > 0
      );
      return out;
    }
    let envelopeResult = rustEnvelopePlacement;
    if (rustEnvelopePlacement) {
      applyNetworkSectorEnvelopePlacementResult(out, rustEnvelopePlacement);
    } else {
      if (requireSectorPlacementCacheHit) {
        throw new Error("network-sector-envelope-placement-missing");
      }
      envelopeResult = applySectorEnvelopePostProcess(
        centerNode,
        sectors,
        out.leafZones,
        occupiedZones,
        {
          rowsById,
          hardZones,
          hardCorridors,
          territoryRanges,
          allowTerritoryOverflow,
          strictCenterOwnership,
          ownerCenters,
          minRadiusFloor: minRadius * 0.78,
          maxRadiusCap: maxLeafRadius,
        }
      );
      if (Number(envelopeResult?.adjusted) > 0) {
        out.leafZones.forEach((zone) => {
          out.positions.set(String(zone?.id || ""), {
            x: Number(zone?.x) || 0,
            y: Number(zone?.y) || 0,
          });
        });
      }
    }
    out.report.envelopeAdjustments = Number(envelopeResult?.adjusted) || 0;
    out.report.envelopeRadiusPulls = Number(envelopeResult?.pulledByRadius) || 0;
    out.report.envelopeAnglePulls = Number(envelopeResult?.pulledByAngle) || 0;
    const uniformResult = applySectorUniformityPostProcess(
      centerNode,
      sectors,
      out.leafZones,
      occupiedZones,
      {
        rowsById,
        hardZones,
        hardCorridors,
        territoryRanges,
        allowTerritoryOverflow,
        strictCenterOwnership,
        ownerCenters,
        minRadiusFloor: minRadius * 0.78,
        maxRadiusCap: maxLeafRadius,
      }
    );
    if (Number(uniformResult?.adjusted) > 0) {
      out.leafZones.forEach((zone) => {
        out.positions.set(String(zone?.id || ""), {
          x: Number(zone?.x) || 0,
          y: Number(zone?.y) || 0,
        });
      });
    }
    out.report.uniformAdjustments = Number(uniformResult?.adjusted) || 0;
    out.report.uniformPlanned = Number(uniformResult?.planned) || 0;
    out.report.uniformRejected = Number(uniformResult?.rejected) || 0;
    out.report.uniformSparseLayerRebalances = Number(uniformResult?.sparseLayerRebalances) || 0;
    out.report.uniformFallbackRescues = Number(uniformResult?.fallbackRescues) || 0;
    // Run envelope once more after uniform redistribution so sector boundary constraints stay enforced.
    const postUniformEnvelope = applySectorEnvelopePostProcess(
      centerNode,
      sectors,
      out.leafZones,
      occupiedZones,
      {
        rowsById,
        hardZones,
        hardCorridors,
        territoryRanges,
        allowTerritoryOverflow,
        strictCenterOwnership,
        ownerCenters,
        minRadiusFloor: minRadius * 0.78,
        maxRadiusCap: maxLeafRadius,
      }
    );
    if (Number(postUniformEnvelope?.adjusted) > 0) {
      out.leafZones.forEach((zone) => {
        out.positions.set(String(zone?.id || ""), {
          x: Number(zone?.x) || 0,
          y: Number(zone?.y) || 0,
        });
      });
    }
    out.report.envelopeAdjustments += Number(postUniformEnvelope?.adjusted) || 0;
    out.report.envelopeRadiusPulls += Number(postUniformEnvelope?.pulledByRadius) || 0;
    out.report.envelopeAnglePulls += Number(postUniformEnvelope?.pulledByAngle) || 0;
    out.report.sectors = sectors.map((sector) => ({
      start: Number(sector.start.toFixed(3)),
      end: Number(sector.end.toFixed(3)),
      score: Number((sector.score || 0).toFixed(3)),
    }));
    out.report.boundaryPenaltyTriggered = (Array.isArray(scoredBuckets) ? scoredBuckets : []).some(
      (row) => Number(row?.boundaryPenalty) > 0
    );
    return out;
  }

  function layoutNetworkCluster(clusterData, params = {}) {
    const cfg = params?.config || DEFAULT_CFG;
    const sectorPlacementPayloads = Array.isArray(params?.sectorPlacementPayloadCollector)
      ? params.sectorPlacementPayloadCollector
      : null;
    const cluster = clusterData?.cluster || {};
    const nodeMetaById = clusterData?.nodeMetaById || {};
    const nodesById = clusterData?.nodesById || new Map();
    const leafEntryById = clusterData?.leafEntryById || {};
    const center = {
      x: Number(clusterData?.placement?.x) || 0,
      y: Number(clusterData?.placement?.y) || 0,
    };
    const clusterNodeIds = sortStableIds(cluster?.nodeIds || []);
    const clusterNodeSet = new Set(clusterNodeIds);
    const allPositions = new Map();
    const clusterId = String(cluster?.clusterId || "");
    const clusterReport = {
      clusterId,
      layoutCase: String(cluster?.layoutCase || "general"),
      centers: [],
      bbox: null,
    };
    const rustPlanCollector =
      typeof engine.collectRustNetworkClusterPlan === "function" ? engine.collectRustNetworkClusterPlan : null;
    const hasRustSourceRows = Array.isArray(clusterData?.networkNodeUpdatesByClusterId?.[clusterId]);
    if (hasRustSourceRows && !rustPlanCollector) {
      throw new Error("network-rust-plan-adapter-missing");
    }
    const rustClusterPlan = hasRustSourceRows && rustPlanCollector
      ? rustPlanCollector(clusterData, clusterNodeSet, nodesById, { makeZone })
      : null;

    if (rustClusterPlan) {
      rustClusterPlan.positions.forEach((pos, id) => allPositions.set(id, pos));
      clusterData.__leafZones = rustClusterPlan.leafZones;
      clusterReport.centers.push(...rustClusterPlan.centerReports);
    } else {
      const localAdjacency = buildClusterAdjacency(cluster, clusterNodeSet);
      const clusterNodes = clusterNodeIds.map((id) => nodesById.get(id)).filter(Boolean);
      const coreNodes = clusterNodes.filter((node) => String(nodeMetaById?.[node.id]?.role || node?.role || "").toLowerCase() === "core");
      const adjacentNodes = clusterNodes.filter((node) => String(nodeMetaById?.[node.id]?.role || node?.role || "").toLowerCase() === "adjacent");
      const leafNodes = clusterNodes.filter((node) => String(nodeMetaById?.[node.id]?.role || node?.role || "").toLowerCase() === "leaf");
      const anchorNodes = sortStableIds(cluster?.layoutAnchorIds || [])
        .map((id) => nodesById.get(id))
        .filter(Boolean);
      if (clusterNodes.length === 1) {
        const only = clusterNodes[0];
        allPositions.set(only.id, { x: center.x, y: center.y });
        clusterReport.layoutCase = "single";
      } else if (clusterNodes.length === 2 || cluster.layoutCase === "pair-horizontal") {
        const pair = placePairHorizontal(clusterNodes, nodeMetaById, center);
        pair.forEach((pos, id) => allPositions.set(id, pos));
        clusterReport.layoutCase = "pair-horizontal";
      } else if (clusterNodes.length === 3 && cluster.layoutCase === "triple-chain") {
        const triple = placeTriple(clusterNodes, localAdjacency, center, false);
        triple.forEach((pos, id) => allPositions.set(id, pos));
      } else if (clusterNodes.length === 3 && cluster.layoutCase === "triple-triangle") {
        const triple = placeTriple(clusterNodes, localAdjacency, center, true);
        triple.forEach((pos, id) => allPositions.set(id, pos));
      } else if (clusterNodes.length === 4 || cluster.layoutCase === "quad-square") {
        const quad = placeQuadSquare(clusterNodes, nodeMetaById, center);
        quad.forEach((pos, id) => allPositions.set(id, pos));
        clusterReport.layoutCase = "quad-square";
      } else {
        const fallbackNode = clusterNodes
          .slice()
          .sort((a, b) => String(a?.id || "").localeCompare(String(b?.id || ""), "zh-CN"))[0];
        const layoutAnchors = anchorNodes.length ? anchorNodes : fallbackNode ? [fallbackNode] : [];
        const backbone = placeNetworkBackbone(coreNodes, adjacentNodes, layoutAnchors, {
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
            : layoutAnchors.map((node) => ({ id: node.id, x: center.x, y: center.y, node }));
        if (!centerRefs.length && fallbackNode) {
          centerRefs.push({ id: fallbackNode.id, x: center.x, y: center.y, node: fallbackNode });
        }
        const centerRefIdSet = new Set(centerRefs.map((row) => String(row?.id || "")).filter(Boolean));
        const adjacentOwnerIds = new Set(
          leafNodes
            .map((leaf) => String(leafEntryById?.[leaf?.id] || ""))
            .filter(Boolean)
        );
        adjacentNodes
          .slice()
          .sort((a, b) => String(a?.id || "").localeCompare(String(b?.id || ""), "zh-CN"))
          .forEach((node) => {
            const id = String(node?.id || "");
            if (!id || centerRefIdSet.has(id) || !adjacentOwnerIds.has(id)) return;
            const pos = backbone.positions.get(id);
            if (!pos) return;
            centerRefs.push({ id, x: Number(pos.x) || 0, y: Number(pos.y) || 0, node });
            centerRefIdSet.add(id);
          });
        const leafBuckets = new Map();
        centerRefs.forEach((row) => leafBuckets.set(row.id, []));
        const fallbackCenterId = centerRefs[0]?.id || "";
        leafNodes.forEach((leaf) => {
          const id = leaf.id;
          let centerId = String(leafEntryById?.[id] || "");
          if (!leafBuckets.has(centerId)) {
            const linkedCenters = sortStableIds((localAdjacency?.[id] || []).filter((nid) => leafBuckets.has(nid)));
            centerId = linkedCenters[0] || fallbackCenterId;
          }
          if (!leafBuckets.has(centerId)) leafBuckets.set(centerId, []);
          leafBuckets.get(centerId).push(leaf);
        });
        const centerWorkRows = centerRefs
          .slice()
          .sort((a, b) => {
            const ac = (leafBuckets.get(String(a?.id || "")) || []).length;
            const bc = (leafBuckets.get(String(b?.id || "")) || []).length;
            if (ac !== bc) return bc - ac;
            return String(a?.id || "").localeCompare(String(b?.id || ""), "zh-CN");
          });
        const localMap = computeLocalOccupiedMap(
          {
            cluster,
            nodeMetaById,
            nodesById,
            positions: allPositions,
          },
          { config: cfg }
        );
        const occupiedZones = localMap.zones.slice();
        const globalMap = params?.globalMap || computeGlobalOccupiedMap([], [], null, {});
        const reservedLeafZones = centerRefs.map((row) => {
          const centerNode = row?.node || nodesById.get(row?.id);
          return {
            centerId: String(row?.id || ""),
            zone: makeZone(
              `reserved:${String(row?.id || "")}`,
              Number(row?.x) || 0,
              Number(row?.y) || 0,
              centerNode,
              Math.max(24, (Number(cfg.R_LEAF_MIN) || 220) * 0.34)
            ),
          };
        });
        const skeletonCorridors = buildCoreAdjacentCorridors(
          centerRefs,
          localAdjacency,
          allPositions,
          nodeMetaById,
          cfg
        );
        const skeletonCentroid = centerRefs.reduce(
          (acc, row) => {
            acc.x += Number(row?.x) || 0;
            acc.y += Number(row?.y) || 0;
            acc.n += 1;
            return acc;
          },
          { x: 0, y: 0, n: 0 }
        );
        const skeletonCenter = skeletonCentroid.n
          ? { x: skeletonCentroid.x / skeletonCentroid.n, y: skeletonCentroid.y / skeletonCentroid.n }
          : { x: center.x, y: center.y };
        const clusterLeafZones = [];
        const useOutwardLeafPlacement = centerRefs.length > 1;
        centerWorkRows.forEach((centerRef) => {
          const groupedLeafs = leafBuckets.get(centerRef.id) || [];
          if (!groupedLeafs.length) return;
          const isBackboneCore = coreNodes.some((row) => String(row?.id || "") === String(centerRef?.id || ""));
          const linkedCorePositions = isBackboneCore
            ? []
            : sortStableIds(
                (localAdjacency?.[String(centerRef?.id || "")] || []).filter((nid) =>
                  coreNodes.some((node) => String(node?.id || "") === String(nid || ""))
                )
              )
                .map((id) => allPositions.get(id))
                .filter(Boolean);
          const outwardAnchor = linkedCorePositions.length
            ? linkedCorePositions.reduce(
                (acc, pos) => {
                  acc.x += Number(pos?.x) || 0;
                  acc.y += Number(pos?.y) || 0;
                  acc.n += 1;
                  return acc;
                },
                { x: 0, y: 0, n: 0 }
              )
            : { x: Number(skeletonCenter?.x) || 0, y: Number(skeletonCenter?.y) || 0, n: 1 };
          const outwardAnchorPoint = {
            x: outwardAnchor.n ? outwardAnchor.x / outwardAnchor.n : Number(skeletonCenter?.x) || 0,
            y: outwardAnchor.n ? outwardAnchor.y / outwardAnchor.n : Number(skeletonCenter?.y) || 0,
          };
          let outwardAngle = Math.atan2(
            (Number(centerRef?.y) || 0) - (Number(outwardAnchorPoint?.y) || 0),
            (Number(centerRef?.x) || 0) - (Number(outwardAnchorPoint?.x) || 0)
          );
          if (!Number.isFinite(outwardAngle)) {
            outwardAngle = stableHashUnit(`network-outward-center|${String(centerRef?.id || "")}`) * Math.PI * 2;
          }
          const inwardAngle = outwardAngle + Math.PI;
          const outwardSpan = clampLayout(
            (isBackboneCore ? 1.18 : 0.96) + Math.sqrt(Math.max(1, groupedLeafs.length || 1)) * 0.17,
            isBackboneCore ? 1.12 : 0.82,
            isBackboneCore ? Math.PI * 0.98 : Math.PI * 0.92
          );
          let territoryRanges = buildCenterTerritoryRanges(centerRef, centerRefs, {
            bucketCount: 216,
            guard: 0.14,
            sampleRadius: Math.max(
              (Number(cfg.R_LEAF_MIN) || 220) * 1.8,
              (Number(cfg.R_LEAF_MIN) || 220) + Math.sqrt(groupedLeafs.length || 1) * 48
            ),
          });
          if (!isBackboneCore) {
            const outwardSpan = clampLayout(
              1.72 + Math.sqrt(Math.max(1, groupedLeafs.length || 1)) * 0.13,
              1.48,
              Math.PI * 1.34
            );
            const outwardOnly = intersectAngleRanges(territoryRanges, outwardAngle, outwardSpan);
            if (outwardOnly.length) territoryRanges = outwardOnly;
          }
          const dynamicLocalMap = {
            zones: []
              .concat(localMap.zones || [])
              .concat(clusterLeafZones)
              .concat(
                reservedLeafZones
                  .filter((row) => row.centerId !== centerRef.id)
                  .map((row) => row.zone)
              ),
            skeletonZones: localMap.skeletonZones || [],
            labelZones: []
              .concat(localMap.labelZones || [])
              .concat(clusterLeafZones),
          };
          const hardZones = []
            .concat(
              (localMap.skeletonZones || [])
                .filter((zone) => String(zone?.id || "") !== String(centerRef?.id || ""))
                .map((zone) => ({
                  ...zone,
                  r: (Number(zone?.r) || 0) + Math.max(20, (Number(cfg.R_LEAF_MIN) || 220) * 0.18),
                }))
            )
            .concat(
              (clusterLeafZones || []).map((zone) => ({
                ...zone,
                r: (Number(zone?.r) || 0) + 14,
              }))
            )
            .concat(
              reservedLeafZones
                .filter((row) => row.centerId !== centerRef.id)
                .map((row) => ({
                  ...row.zone,
                  r: (Number(row?.zone?.r) || 0) + Math.max(34, (Number(cfg.R_LEAF_MIN) || 220) * 0.42),
                }))
            )
            .concat(
              (globalMap?.leafZones || []).map((zone) => ({
                ...zone,
                r: (Number(zone?.r) || 0) + 12,
              }))
            );
          const hardCorridors = (skeletonCorridors || []).map((row) => ({
            ...row,
            halfWidth:
              (Number(row?.halfWidth) || 0) +
              (String(row?.ownerCenterId || "") === String(centerRef?.id || "") ? 18 : 30),
          }));
          const ownCorridors = (skeletonCorridors || []).filter(
            (row) => String(row?.ownerCenterId || "") === String(centerRef?.id || "")
          );
          const ownVec = ownCorridors.reduce(
            (acc, row) => {
              const dx = (Number(row?.x2) || 0) - (Number(row?.x1) || 0);
              const dy = (Number(row?.y2) || 0) - (Number(row?.y1) || 0);
              const len = Math.hypot(dx, dy) || 1;
              acc.x += dx / len;
              acc.y += dy / len;
              return acc;
            },
            { x: 0, y: 0 }
          );
          const ownCorridorAngle =
            Math.hypot(ownVec.x, ownVec.y) > 1e-4 ? Math.atan2(ownVec.y, ownVec.x) : Number.NaN;
          const antiCorridorAngle =
            Math.hypot(ownVec.x, ownVec.y) > 1e-4 ? Math.atan2(-ownVec.y, -ownVec.x) : Number.NaN;
          const preferredAngles = [
            { angle: outwardAngle, weight: 2.8, width: 1.3 },
          ];
          if (Number.isFinite(antiCorridorAngle)) {
            preferredAngles.push({ angle: antiCorridorAngle, weight: 1.45, width: 1.14 });
          }
          if (Number.isFinite(ownCorridorAngle)) {
            preferredAngles.push({ angle: ownCorridorAngle, weight: -1.65, width: 0.98 });
          }
          preferredAngles.push({ angle: inwardAngle, weight: -2.2, width: 1.34 });
          const nearestCenterDist = centerRefs.reduce((min, row) => {
            const id = String(row?.id || "");
            if (!id || id === String(centerRef?.id || "")) return min;
            const dx = (Number(row?.x) || 0) - (Number(centerRef?.x) || 0);
            const dy = (Number(row?.y) || 0) - (Number(centerRef?.y) || 0);
            const dist = Math.hypot(dx, dy);
            if (!Number.isFinite(dist) || dist <= 1e-3) return min;
            return Math.min(min, dist);
          }, Infinity);
          const leafCountForCenter = Math.max(1, groupedLeafs.length);
          const denseLevel =
            leafCountForCenter >= 220 ? 4 : leafCountForCenter >= 140 ? 3 : leafCountForCenter >= 84 ? 2 : leafCountForCenter >= 46 ? 1 : 0;
          const centerSpacingFactor =
            denseLevel >= 4 ? 0.82 : denseLevel === 3 ? 0.76 : denseLevel === 2 ? 0.7 : denseLevel === 1 ? 0.62 : 0.5;
          const densityRadiusNeed =
            (Number(cfg.R_LEAF_MIN) || 220) * (denseLevel >= 3 ? 1.14 : denseLevel >= 2 ? 1.08 : 1.0) +
            Math.sqrt(leafCountForCenter) * (denseLevel >= 4 ? 14 : denseLevel === 3 ? 12 : denseLevel === 2 ? 10 : denseLevel === 1 ? 7 : 4.5);
          const territorySpanTotal = (territoryRanges || []).reduce(
            (sum, row) => sum + Math.max(0, Number(row?.span) || 0),
            0
          );
          const effectiveTerritorySpan = clampLayout(
            territorySpanTotal > 0 ? territorySpanTotal : Math.PI * 2,
            Math.PI / 3,
            Math.PI * 2
          );
          const desiredLayers =
            leafCountForCenter >= 260
              ? 18
              : leafCountForCenter >= 180
              ? 16
              : leafCountForCenter >= 120
              ? 14
              : leafCountForCenter >= 80
              ? 11
              : leafCountForCenter >= 46
              ? 8
              : 4;
          const desiredPerLayer = Math.max(1, Math.ceil(leafCountForCenter / desiredLayers));
          const groupedAvgLabelPad =
            groupedLeafs.reduce((sum, node) => sum + labelPad(node), 0) / Math.max(1, leafCountForCenter);
          const groupedAvgNodeRadius =
            groupedLeafs.reduce((sum, node) => sum + nodeRadius(node), 0) / Math.max(1, leafCountForCenter);
          const groupedAvgCollision =
            groupedLeafs.reduce((sum, node) => sum + nodeCollisionRadius(node), 0) /
            Math.max(1, leafCountForCenter);
          const nominalArcGap = resolveNetworkNominalArcGap(
            leafCountForCenter,
            groupedAvgLabelPad,
            groupedAvgNodeRadius,
            groupedAvgCollision
          );
          const radiusForArcCapacity = (desiredPerLayer * nominalArcGap) / Math.max(0.42, effectiveTerritorySpan);
          const layerGapNeed = Math.max(22, (Number(cfg.NETWORK_SECTOR_LAYER_GAP) || 68) * 0.64);
          const spreadNeed = Math.max(28, (desiredLayers - 1) * layerGapNeed);
          const densityDrivenRadiusNeed = Math.max(
            densityRadiusNeed,
            radiusForArcCapacity + Math.max(26, (Number(cfg.R_LEAF_MIN) || 220) * 0.28),
            Math.max(0, (Number(cfg.R_LEAF_MIN) || 220) * 0.82) + spreadNeed * 0.62
          );
          const spacingSoftFloor = Number.isFinite(nearestCenterDist)
            ? Math.max(0, nearestCenterDist * centerSpacingFactor)
            : 0;
          const cfgRadiusCap = Math.max(260, Number(cfg.NETWORK_MAX_LEAF_RADIUS) || 1900);
          let maxLeafRadius = Math.max(110, spacingSoftFloor, densityDrivenRadiusNeed);
          maxLeafRadius = clampLayout(maxLeafRadius, 110, cfgRadiusCap);
          const allowTerritoryOverflow = false;
          const centerNode = {
            id: centerRef.id,
            x: centerRef.x,
            y: centerRef.y,
            node: centerRef.node || nodesById.get(centerRef.id),
          };
          const bucketLeafRows = groupedLeafs
            .slice()
            .sort((a, b) => String(a?.id || "").localeCompare(String(b?.id || ""), "zh-CN"))
            .map((node) => ({
              node,
              id: String(node?.id || ""),
              labelPad: labelPad(node),
            }));
          const sectorProjectionParams = {
            config: cfg,
            territoryRanges,
            hardZones,
            hardCorridors,
            preferredAngles,
            allowTerritoryOverflow,
            maxLeafRadius,
            ownerCenters: centerRefs.map((row) => ({
              id: String(row?.id || ""),
              x: Number(row?.x) || 0,
              y: Number(row?.y) || 0,
            })),
            strictCenterOwnership: true,
            preferRingMode: isBackboneCore,
            enforceSingleSector: true,
            preferredMultiSectorCount: 1,
            multiSectorGap:
              groupedLeafs.length >= 220 ? 0.36 : groupedLeafs.length >= 120 ? 0.3 : groupedLeafs.length >= 60 ? 0.24 : 0.2,
            sectorPlacementPayloadCollector: sectorPlacementPayloads,
            sectorPlacementPayloadLimit: 96,
            disableSectorPlacementCache: useOutwardLeafPlacement || !isBackboneCore,
          };
          const placed = useOutwardLeafPlacement
            ? placeNetworkOutwardLeafSectors(centerNode, bucketLeafRows, occupiedZones, {
                ...sectorProjectionParams,
                outwardAngle,
                outwardSpan,
              })
            : placeLeafSectors(
                centerNode,
                projectAngularBucketSectorProjection(
                  centerNode,
                  bucketLeafRows,
                  dynamicLocalMap,
                  globalMap,
                  sectorProjectionParams
                ),
                occupiedZones,
                sectorProjectionParams
              );
          placed.positions.forEach((pos, id) => allPositions.set(id, pos));
          clusterLeafZones.push(...placed.leafZones);
          clusterReport.centers.push({
            centerId: centerRef.id,
            centerType: coreNodes.some((node) => node.id === centerRef.id) ? "core" : "layoutAnchor",
            leafCount: groupedLeafs.length,
            sectorCount: Array.isArray(placed.report?.sectors) ? placed.report.sectors.length : 0,
            multiSectorUsed:
              (Array.isArray(placed.report?.sectors) ? placed.report.sectors.length : 0) > 1,
            leafMode: "sector",
            sectorRanges: placed.report?.sectors || [],
            topScore: placed.report?.bucketTopScore ?? null,
            avoidanceHits: placed.report?.avoidanceHits || 0,
            hardRejectHits: placed.report?.hardRejectHits || 0,
            corridorRejectHits: placed.report?.corridorRejectHits || 0,
            territoryOverflowHits: placed.report?.territoryOverflowHits || 0,
            envelopeAdjustments: placed.report?.envelopeAdjustments || 0,
            envelopeRadiusPulls: placed.report?.envelopeRadiusPulls || 0,
            envelopeAnglePulls: placed.report?.envelopeAnglePulls || 0,
            uniformAdjustments: placed.report?.uniformAdjustments || 0,
            uniformPlanned: placed.report?.uniformPlanned || 0,
            uniformRejected: placed.report?.uniformRejected || 0,
            uniformSparseLayerRebalances: placed.report?.uniformSparseLayerRebalances || 0,
            uniformFallbackRescues: placed.report?.uniformFallbackRescues || 0,
            sectorFastPath: placed.report?.sectorFastPath || null,
            maxLeafRadius: Number.isFinite(maxLeafRadius) ? Number(maxLeafRadius.toFixed(2)) : null,
            boundaryPenaltyTriggered: !!placed.report?.boundaryPenaltyTriggered,
            territoryRangeCount: Number(placed.report?.territoryRangeCount) || 0,
          });
        });
        clusterData.__leafZones = clusterLeafZones;
      }

      clusterNodes.forEach((node, index) => {
        if (allPositions.has(node.id)) return;
        const angle = stableHashUnit(`network-fallback|${cluster.clusterId || ""}|${node.id}`) * Math.PI * 2;
        const radius = 56 + index * 18;
        allPositions.set(node.id, {
          x: center.x + Math.cos(angle) * radius,
          y: center.y + Math.sin(angle) * radius,
        });
      });
    }
    let bbox = computeClusterBBox(clusterNodeIds, nodesById, allPositions);
    const preferHorizontal = params?.preferHorizontal ?? !!cfg.NETWORK_PREFER_HORIZONTAL;
    const horizontalAspectThreshold = Math.max(
      1,
      Number(params?.horizontalAspectThreshold) || Number(cfg.NETWORK_HORIZONTAL_ASPECT_THRESHOLD) || 1.08
    );
    const aspectBefore = bbox.height / Math.max(1e-6, bbox.width);
    if (preferHorizontal && bbox.height > bbox.width * horizontalAspectThreshold) {
      const pivot = { x: bbox.cx, y: bbox.cy };
      const rotateAngle = -Math.PI / 2;
      clusterNodeIds.forEach((id) => {
        const pos = allPositions.get(id);
        if (!pos) return;
        allPositions.set(id, rotatePoint(pos, pivot, rotateAngle));
      });
      const clusterLeafZones = Array.isArray(clusterData.__leafZones) ? clusterData.__leafZones : [];
      clusterLeafZones.forEach((zone, idx) => {
        if (!zone) return;
        const rotated = rotatePoint(zone, pivot, rotateAngle);
        clusterLeafZones[idx] = { ...zone, x: rotated.x, y: rotated.y };
      });
      (clusterReport.centers || []).forEach((row, idx) => {
        if (!row) return;
        const rotated = rotatePoint({ x: row.x, y: row.y }, pivot, rotateAngle);
        clusterReport.centers[idx] = { ...row, x: rotated.x, y: rotated.y };
      });
      bbox = computeClusterBBox(clusterNodeIds, nodesById, allPositions);
      clusterReport.horizontalNormalized = true;
      clusterReport.horizontalAspectBefore = Number(aspectBefore.toFixed(3));
      clusterReport.horizontalAspectAfter = Number((bbox.height / Math.max(1e-6, bbox.width)).toFixed(3));
    } else {
      clusterReport.horizontalNormalized = false;
      clusterReport.horizontalAspectBefore = Number(aspectBefore.toFixed(3));
    }
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
      leafZones: clusterData.__leafZones || [],
      report: clusterReport,
    };
  }

  function buildFallbackSemantic(nodes) {
    const ids = sortStableIds((nodes || []).map((node) => node?.id));
    return {
      mode: "network",
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
        "cluster-0-0": { x: 0, y: 0, width: 520, height: 360 },
      },
      reports: {},
      roleGraph: { components: [{ componentId: 0, nodeIds: ids }] },
      canvasBounds: { minX: -1400, minY: -1100, maxX: 1400, maxY: 1100 },
    };
  }

  function nowNetworkMs() {
    const perf =
      typeof performance !== "undefined" && performance && typeof performance.now === "function"
        ? performance
        : null;
    return perf ? perf.now() : Date.now();
  }

  function resolveNetworkPlanMode(nodeCount, edgeCount) {
    const nodes = Math.max(0, Number(nodeCount) || 0);
    const edges = Math.max(0, Number(edgeCount) || 0);
    if (nodes > 5000 || edges > 15000) return "xlarge";
    if (nodes > 1200 || edges > 3500) return "large";
    if (nodes > 150 || edges > 400) return "medium";
    return "small";
  }

  function networkEdgeEndpoint(value) {
    if (value == null) return "";
    if (typeof value === "object") return String(value.id || value.key || "").trim();
    return String(value || "").trim();
  }

  function networkEdgeWeight(edge) {
    const row = edge && typeof edge === "object" ? edge : {};
    const values = [
      row.amount,
      row.weight,
      row.total_amount,
      row.totalAmount,
      row.forward_amount,
      row.reverse_amount,
      row.out_amount,
      row.in_amount,
      row.count,
    ];
    for (const value of values) {
      const next = Math.abs(Number(value));
      if (Number.isFinite(next) && next > 0) return Math.max(1, next);
    }
    return 1;
  }

  function buildNetworkTopology(nodes = [], edges = [], mode = "small") {
    const rows = Array.isArray(nodes) ? nodes : [];
    const nodesById = new Map(rows.map((node, index) => [String(node?.id || ""), { node, index }]));
    const edgeRows = [];
    let maxRawWeight = 1;
    const sourceEdges = Array.isArray(edges) ? edges : [];
    const edgeSampleTarget = mode === "xlarge" ? 2800 : mode === "large" ? 6500 : Number.POSITIVE_INFINITY;
    const edgeSampleStep =
      Number.isFinite(edgeSampleTarget) && sourceEdges.length > edgeSampleTarget
        ? Math.max(1, Math.ceil(sourceEdges.length / edgeSampleTarget))
        : 1;
    sourceEdges.forEach((edge, edgeIndex) => {
      if (edgeSampleStep > 1 && edgeIndex % edgeSampleStep !== 0) return;
      const sourceId = networkEdgeEndpoint(edge?.source);
      const targetId = networkEdgeEndpoint(edge?.target);
      if (!sourceId || !targetId || sourceId === targetId) return;
      const source = nodesById.get(sourceId);
      const target = nodesById.get(targetId);
      if (!source || !target) return;
      const rawWeight = networkEdgeWeight(edge);
      maxRawWeight = Math.max(maxRawWeight, rawWeight);
      edgeRows.push({ source: source.index, target: target.index, rawWeight, weight: rawWeight });
    });
    const denom = Math.max(1, Math.log1p(maxRawWeight));
    edgeRows.forEach((edge) => {
      edge.weight = clampLayout(0.35 + (Math.log1p(edge.rawWeight) / denom) * 2.65, 0.35, 3);
    });
    const adjacency = rows.map(() => []);
    edgeRows.forEach((edge) => {
      adjacency[edge.source].push({ index: edge.target, weight: edge.weight });
      adjacency[edge.target].push({ index: edge.source, weight: edge.weight });
    });
    return { edgeRows, adjacency };
  }

  function assignNetworkTopologyCommunities(nodes = [], topology = {}, mode = "small") {
    const rows = Array.isArray(nodes) ? nodes : [];
    const adjacency = Array.isArray(topology?.adjacency) ? topology.adjacency : [];
    const communities = rows.map((node) => {
      const semantic = String(node?.layoutCommunity || node?.layoutClusterId || node?.clusterId || "").trim();
      return semantic || `seed:${String(node?.id || "")}`;
    });
    const maxIterations = mode === "xlarge" ? 1 : mode === "large" ? 2 : mode === "medium" ? 4 : 6;
    for (let iter = 0; iter < maxIterations; iter += 1) {
      let changed = false;
      rows.forEach((node, index) => {
        void node;
        const scores = new Map([[communities[index], 0.28]]);
        (adjacency[index] || []).forEach((edge) => {
          const key = communities[edge.index] || "";
          if (!key) return;
          scores.set(key, (scores.get(key) || 0) + (Number(edge.weight) || 1));
        });
        let bestKey = communities[index];
        let bestScore = scores.get(bestKey) || 0;
        scores.forEach((score, key) => {
          if (
            score > bestScore + 1e-9 ||
            (Math.abs(score - bestScore) <= 1e-9 && String(key).localeCompare(String(bestKey), "zh-CN") < 0)
          ) {
            bestKey = key;
            bestScore = score;
          }
        });
        if (bestKey && bestKey !== communities[index]) {
          communities[index] = bestKey;
          changed = true;
        }
      });
      if (!changed) break;
    }
    const counts = new Map();
    communities.forEach((key) => counts.set(key, (counts.get(key) || 0) + 1));
    const remap = new Map(
      Array.from(counts.entries())
        .sort((a, b) => b[1] - a[1] || String(a[0]).localeCompare(String(b[0]), "zh-CN"))
        .map(([key], index) => [key, `c${index + 1}`])
    );
    return communities.map((key) => remap.get(key) || "c1");
  }

  function assignNetworkCoarseSkeletonCommunities(nodes = []) {
    const rows = Array.isArray(nodes) ? nodes : [];
    const raw = rows.map((node, index) => {
      const semantic = String(node?.layoutCommunity || node?.layoutClusterId || node?.clusterId || "").trim();
      return semantic || `x${Math.floor(index / 80)}`;
    });
    const counts = new Map();
    raw.forEach((key) => counts.set(key, (counts.get(key) || 0) + 1));
    const remap = new Map(
      Array.from(counts.entries())
        .sort((a, b) => b[1] - a[1] || String(a[0]).localeCompare(String(b[0]), "zh-CN"))
        .map(([key], index) => [key, `c${index + 1}`])
    );
    return raw.map((key) => remap.get(key) || "c1");
  }

  function scoreNetworkTopologyRoles(nodes = [], topology = {}, communities = []) {
    const rows = Array.isArray(nodes) ? nodes : [];
    const adjacency = Array.isArray(topology?.adjacency) ? topology.adjacency : [];
    let maxDegree = 1;
    const weighted = rows.map((node, index) => {
      void node;
      const value = (adjacency[index] || []).reduce((sum, edge) => sum + (Number(edge.weight) || 1), 0);
      maxDegree = Math.max(maxDegree, value);
      return value;
    });
    rows.forEach((node, index) => {
      const own = communities[index] || "";
      const neighborCommunities = new Set();
      let external = 0;
      let total = 0;
      (adjacency[index] || []).forEach((edge) => {
        const community = communities[edge.index] || "";
        neighborCommunities.add(community);
        total += Number(edge.weight) || 1;
        if (community && community !== own) external += Number(edge.weight) || 1;
      });
      const diversity = Math.max(0, neighborCommunities.size - 1);
      const externalRatio = total > 0 ? external / total : 0;
      node.layoutBridgeScore = Number(
        clampLayout(externalRatio * 0.72 + Math.min(1, diversity / 4) * 0.28, 0, 1).toFixed(4)
      );
      node.__networkTopologyHubScore = clampLayout(weighted[index] / Math.max(1, maxDegree), 0, 1);
      node.layoutCommunity = own || node.layoutCommunity || "";
    });
  }

  function computeNetworkCommunityCentroids(nodes = [], communities = []) {
    const sums = new Map();
    (Array.isArray(nodes) ? nodes : []).forEach((node, index) => {
      const key = communities[index] || String(node?.layoutCommunity || "c1");
      const row = sums.get(key) || { x: 0, y: 0, count: 0 };
      row.x += Number(node?.x) || 0;
      row.y += Number(node?.y) || 0;
      row.count += 1;
      sums.set(key, row);
    });
    sums.forEach((row) => {
      row.x /= Math.max(1, row.count);
      row.y /= Math.max(1, row.count);
    });
    return sums;
  }

  function applyNetworkVisibilityTiers(nodes = [], mode = "small") {
    return;
  }

  function networkTopologyLabelPriority(node = {}) {
    const role = String(node?.role || node?.layoutBand || "").toLowerCase();
    const roleRank = role === "core" ? 4 : role === "bridge" ? 3 : role === "adjacent" ? 2 : 1;
    return (
      roleRank * 1_000_000 +
      (Number(node?.layoutBridgeScore) || 0) * 90_000 +
      (Number(node?.__networkTopologyHubScore) || 0) * 70_000 +
      (Number(node?.weight) || Number(node?.amount) || 0) * 0.001
    );
  }

  function suppressNetworkTopologyVisibleLabels(nodes = [], mode = "small") {
    return 0;
  }

  function networkXlargeSkeletonRoleRank(node) {
    const role = String(node?.role || node?.layoutBand || "").toLowerCase();
    return role === "core" ? 3 : role === "adjacent" ? 2 : 1;
  }

  function compareNetworkXlargeSeedIndexes(leftIndex, rightIndex, rows = [], weightedDegree = []) {
    const roleDiff =
      networkXlargeSkeletonRoleRank(rows[rightIndex]) - networkXlargeSkeletonRoleRank(rows[leftIndex]);
    if (roleDiff) return roleDiff;
    const degreeDiff = (Number(weightedDegree[rightIndex]) || 0) - (Number(weightedDegree[leftIndex]) || 0);
    if (Math.abs(degreeDiff) > 1e-9) return degreeDiff;
    const leftId = String(rows[leftIndex]?.id || leftIndex);
    const rightId = String(rows[rightIndex]?.id || rightIndex);
    return leftId < rightId ? -1 : leftId > rightId ? 1 : leftIndex - rightIndex;
  }

  function pushNetworkXlargeSeedIndex(seeds, nodeIndex, limit, rows = [], weightedDegree = []) {
    const maxRows = Math.max(0, Number(limit) || 0);
    if (!Array.isArray(seeds) || maxRows <= 0) return false;
    if (
      seeds.length >= maxRows &&
      compareNetworkXlargeSeedIndexes(nodeIndex, seeds[seeds.length - 1], rows, weightedDegree) >= 0
    ) {
      return false;
    }
    let insertAt = seeds.length;
    while (
      insertAt > 0 &&
      compareNetworkXlargeSeedIndexes(nodeIndex, seeds[insertAt - 1], rows, weightedDegree) < 0
    ) {
      insertAt -= 1;
    }
    seeds.splice(insertAt, 0, nodeIndex);
    if (seeds.length > maxRows) seeds.pop();
    return true;
  }

  function seedLargeNetworkCommunities(nodes = [], communities = [], mode = "large") {
    if (mode !== "large" && mode !== "xlarge") return;
    const groups = new Map();
    nodes.forEach((node, index) => {
      const key = communities[index] || String(node?.layoutCommunity || "c1");
      if (!groups.has(key)) groups.set(key, []);
      groups.get(key).push({ node, index });
    });
    if (groups.size <= 1) return;
    const radius = Math.sqrt(Math.max(1, nodes.length)) * 82 + 220;
    const manyGroups = groups.size > 2000;
    const keys = Array.from(groups.keys()).sort((a, b) => {
      const ac = groups.get(a)?.length || 0;
      const bc = groups.get(b)?.length || 0;
      if (bc !== ac) return bc - ac;
      return manyGroups ? 0 : String(a).localeCompare(String(b), "zh-CN");
    });
    keys.forEach((key, keyIndex) => {
      const angle = -Math.PI + (Math.PI * 2 * (keyIndex + stableHashUnit(`network-community|${key}`) * 0.18)) / keys.length;
      const cx = Math.cos(angle) * radius;
      const cy = Math.sin(angle) * radius;
      (groups.get(key) || [])
        .slice()
        .sort(
          (a, b) =>
            (Number(b.node?.__networkTopologyHubScore) || 0) - (Number(a.node?.__networkTopologyHubScore) || 0) ||
            String(a.node?.id || "").localeCompare(String(b.node?.id || ""), "zh-CN")
        )
        .forEach(({ node }, localIndex) => {
          const localAngle = Math.PI * 2 * stableHashUnit(`network-local|${key}|${node.id}`);
          const localRadius = 26 + Math.sqrt(localIndex) * 32;
          const tx = cx + Math.cos(localAngle) * localRadius;
          const ty = cy + Math.sin(localAngle) * localRadius;
          node.x = (Number(node.x) || 0) * 0.34 + tx * 0.66;
          node.y = (Number(node.y) || 0) * 0.34 + ty * 0.66;
        });
    });
  }

  function buildNetworkXlargeSkeletonGroups(nodes = [], edges = []) {
    const rows = Array.isArray(nodes) ? nodes : [];
    const groups = new Map();
    let groupOrder = 0;
    rows.forEach((node, index) => {
      const key =
        String(node?.layoutCommunity || node?.layoutClusterId || node?.clusterId || "").trim() ||
        `x${Math.floor(index / 80)}`;
      if (node && typeof node === "object") node.layoutCommunity = key;
      if (!groups.has(key)) {
        groups.set(key, []);
        groupOrder += 1;
      }
      if (node && typeof node === "object" && (!Number.isFinite(Number(node.x)) || !Number.isFinite(Number(node.y)))) {
        const order = groupOrder - 1;
        node.x = ((order % 240) - 120) * 72;
        node.y = Math.floor(order / 240) * 72;
      }
      groups.get(key).push(index);
    });
    const oversized = Array.from(groups.values()).some((indexes) => indexes.length > 420);
    if (groups.size >= 120 || !oversized || !Array.isArray(edges) || !edges.length) {
      return { groups, splitLargeGroups: false };
    }

    const idToIndex = new Map();
    rows.forEach((node, index) => {
      const id = String(node?.id || "");
      if (id && !idToIndex.has(id)) idToIndex.set(id, index);
    });
    const weightedDegree = rows.map(() => 0);
    const edgeSampleTarget = 3200;
    const edgeSampleStep =
      Array.isArray(edges) && edges.length > edgeSampleTarget
        ? Math.max(1, Math.ceil(edges.length / edgeSampleTarget))
        : 1;
    (edges || []).forEach((edge, edgeIndex) => {
      if (edgeSampleStep > 1 && edgeIndex % edgeSampleStep !== 0) return;
      const source = networkEdgeEndpoint(edge?.source);
      const target = networkEdgeEndpoint(edge?.target);
      if (!source || !target || source === target) return;
      const sourceIndex = idToIndex.get(source);
      const targetIndex = idToIndex.get(target);
      if (sourceIndex == null || targetIndex == null) return;
      const weight = clampLayout(networkEdgeWeight(edge), 0.1, 8);
      weightedDegree[sourceIndex] += weight;
      weightedDegree[targetIndex] += weight;
    });

    const refined = new Map();
    groups.forEach((indexes, key) => {
      if (!Array.isArray(indexes) || indexes.length <= 420) {
        refined.set(key, indexes || []);
        return;
      }
      const targetBuckets = Math.max(2, Math.min(260, Math.ceil(indexes.length / 120)));
      const seeds = [];
      indexes.forEach((nodeIndex) => {
        pushNetworkXlargeSeedIndex(seeds, nodeIndex, targetBuckets, rows, weightedDegree);
      });
      const bucketByIndex = new Map();
      seeds.forEach((nodeIndex, bucketIndex) => {
        bucketByIndex.set(nodeIndex, bucketIndex);
      });
      const buckets = Array.from({ length: targetBuckets }, () => []);
      indexes.forEach((nodeIndex, localIndex) => {
        let bucket = bucketByIndex.get(nodeIndex);
        if (!Number.isInteger(bucket)) {
          bucket = localIndex % targetBuckets;
        }
        buckets[bucket].push(nodeIndex);
      });
      buckets.forEach((bucketRows, bucketIndex) => {
        if (!bucketRows.length) return;
        refined.set(`${key}#${bucketIndex}`, bucketRows);
      });
    });
    return { groups: refined, splitLargeGroups: true };
  }

  function compareNetworkXlargeSkeletonGroupRows(left, right, manyGroups = false) {
    const countDiff = (Number(right?.count) || 0) - (Number(left?.count) || 0);
    if (countDiff) return countDiff;
    if (manyGroups) return (Number(left?.order) || 0) - (Number(right?.order) || 0);
    return String(left?.key || "").localeCompare(String(right?.key || ""), "zh-CN");
  }

  function selectNetworkXlargeSkeletonGroupKeys(groups, limit = 520) {
    const maxRows = Math.max(1, Number(limit) || 520);
    if (!groups || typeof groups.forEach !== "function") return [];
    const manyGroups = groups.size > 2000;
    if (manyGroups) {
      const keysByCount = new Map();
      groups.forEach((indexes, key) => {
        const count = Array.isArray(indexes) ? indexes.length : 0;
        if (!keysByCount.has(count)) keysByCount.set(count, []);
        keysByCount.get(count).push(key);
      });
      const selected = [];
      Array.from(keysByCount.keys())
        .sort((left, right) => right - left)
        .some((count) => {
          const keys = keysByCount.get(count) || [];
          for (let i = 0; i < keys.length; i += 1) {
            selected.push(keys[i]);
            if (selected.length >= maxRows) return true;
          }
          return false;
        });
      return selected;
    }
    if (groups.size <= maxRows * 2) {
      return Array.from(groups.entries())
        .map(([key, indexes], order) => ({ key, count: Array.isArray(indexes) ? indexes.length : 0, order }))
        .sort((left, right) => compareNetworkXlargeSkeletonGroupRows(left, right, manyGroups))
        .slice(0, maxRows)
        .map((row) => row.key);
    }
    const rows = [];
    let order = 0;
    groups.forEach((indexes, key) => {
      rows.push({ key, count: Array.isArray(indexes) ? indexes.length : 0, order });
      order += 1;
      if (rows.length > maxRows * 2) {
        rows.sort((left, right) => compareNetworkXlargeSkeletonGroupRows(left, right, manyGroups));
        rows.length = maxRows;
      }
    });
    return rows
      .sort((left, right) => compareNetworkXlargeSkeletonGroupRows(left, right, manyGroups))
      .slice(0, maxRows)
      .map((row) => row.key);
  }

  function applyNetworkXlargeSkeletonSeed(nodes = [], edges = []) {
    return applyNetworkTopologyRelaxationDetailed(Array.isArray(nodes) ? nodes : [], edges, "xlarge");
  }

  function applyNetworkTopologyRelaxation(nodes = [], edges = []) {
    const rows = Array.isArray(nodes) ? nodes : [];
    const mode = resolveNetworkPlanMode(rows.length, Array.isArray(edges) ? edges.length : 0);
    return applyNetworkTopologyRelaxationDetailed(rows, edges, mode);
  }

  function applyNetworkTopologyRelaxationDetailed(rows = [], edges = [], mode = "small") {
    const topology = buildNetworkTopology(rows, edges, mode);
    const edgeRows = topology.edgeRows;
    const startedAt = nowNetworkMs();
    if (rows.length <= 1) {
      return buildNetworkQualityReport(rows, edgeRows, mode, startedAt, []);
    }
    const seedPositions = rows.map((node, index) => ({
      x: Number.isFinite(Number(node?.x))
        ? Number(node.x)
        : Math.cos(stableHashUnit(`network-seed-x|${node?.id || index}`) * Math.PI * 2) * 120,
      y: Number.isFinite(Number(node?.y))
        ? Number(node.y)
        : Math.sin(stableHashUnit(`network-seed-y|${node?.id || index}`) * Math.PI * 2) * 120,
    }));
    const duplicateSeedGroups = new Map();
    seedPositions.forEach((pos, index) => {
      const key = `${pos.x.toFixed(1)}|${pos.y.toFixed(1)}`;
      if (!duplicateSeedGroups.has(key)) duplicateSeedGroups.set(key, []);
      duplicateSeedGroups.get(key).push(index);
    });
    duplicateSeedGroups.forEach((indexes) => {
      if (!Array.isArray(indexes) || indexes.length <= 1) return;
      const radius = Math.sqrt(indexes.length) * 24;
      indexes.forEach((nodeIndex, localIndex) => {
        const angle =
          (Math.PI * 2 * (localIndex + stableHashUnit(String(rows[nodeIndex]?.id || nodeIndex)) * 0.31)) /
          indexes.length;
        const dx = Math.cos(angle) * radius;
        const dy = Math.sin(angle) * radius;
        seedPositions[nodeIndex].x += dx;
        seedPositions[nodeIndex].y += dy;
        rows[nodeIndex].x = (Number(rows[nodeIndex].x) || 0) + dx;
        rows[nodeIndex].y = (Number(rows[nodeIndex].y) || 0) + dy;
      });
    });
    const communities = assignNetworkTopologyCommunities(rows, topology, mode);
    scoreNetworkTopologyRoles(rows, topology, communities);
    seedLargeNetworkCommunities(rows, communities, mode);
    const iterations =
      mode === "small" ? (rows.length <= 40 && edgeRows.length <= 120 ? 34 : 44) : mode === "medium" ? 6 : mode === "large" ? 1 : 0;
    const repelTarget =
      mode === "small" ? 240 : mode === "medium" ? 80 : mode === "large" ? 48 : 28;
    const repelSampleStep = rows.length <= repelTarget ? 1 : Math.max(1, Math.ceil(rows.length / repelTarget));
    const attractionScale = mode === "small" ? 0.014 : mode === "medium" ? 0.011 : mode === "large" ? 0.007 : 0.005;
    const repelScale = mode === "small" ? 5000 : mode === "medium" ? 4100 : mode === "large" ? 3000 : 2200;
    const anchorScale = mode === "small" ? 0.012 : mode === "medium" ? 0.006 : mode === "large" ? 0.0025 : 0.0016;
    const gravityScale = mode === "small" ? 0.0008 : mode === "medium" ? 0.00062 : mode === "large" ? 0.00042 : 0.00032;
    const velocity = rows.map(() => ({ x: 0, y: 0 }));
    const collisionRadii = rows.map((node) => Math.max(nodeRadius(node) + 8, nodeCollisionRadius(node) * 0.74));
    for (let iter = 0; iter < iterations; iter += 1) {
      const cooling = Math.max(0.22, 1 - (iter / Math.max(1, iterations)) * 0.74);
      const maxStep = (mode === "small" ? 26 : mode === "medium" ? 23 : mode === "large" ? 17 : 13) * cooling;
      const forces = rows.map(() => ({ x: 0, y: 0 }));
      edgeRows.forEach((edge) => {
        const a = rows[edge.source];
        const b = rows[edge.target];
        const dx = (Number(b.x) || 0) - (Number(a.x) || 0);
        const dy = (Number(b.y) || 0) - (Number(a.y) || 0);
        const dist = Math.max(0.01, Math.hypot(dx, dy));
        const ideal = clampLayout(178 / Math.sqrt(Math.max(0.35, edge.weight)), 76, 240) + (nodeRadius(a) + nodeRadius(b)) * 0.55;
        const force = (dist - ideal) * attractionScale * edge.weight * cooling;
        const ux = dx / dist;
        const uy = dy / dist;
        forces[edge.source].x += ux * force;
        forces[edge.source].y += uy * force;
        forces[edge.target].x -= ux * force;
        forces[edge.target].y -= uy * force;
      });
      for (let i = 0; i < rows.length; i += 1) {
        for (let j = i + 1; j < rows.length; j += 1) {
          if (repelSampleStep > 1 && (i * 31 + j * 17) % repelSampleStep !== 0) continue;
          let dx = (Number(rows[j].x) || 0) - (Number(rows[i].x) || 0);
          let dy = (Number(rows[j].y) || 0) - (Number(rows[i].y) || 0);
          if (Math.abs(dx) + Math.abs(dy) < 1e-6) {
            const angle = Math.PI * 2 * stableHashUnit(`network-overlap|${rows[i]?.id || i}|${rows[j]?.id || j}`);
            dx = Math.cos(angle) * 0.1;
            dy = Math.sin(angle) * 0.1;
          }
          const distSq = Math.max(9, dx * dx + dy * dy);
          const dist = Math.sqrt(distSq);
          const minDist = collisionRadii[i] + collisionRadii[j] + 10;
          const sameCommunity = communities[i] === communities[j];
          let force = (repelScale * (sameCommunity ? 0.72 : 1.12) * repelSampleStep) / distSq;
          if (dist < minDist) force += (minDist - dist) * 0.12 * repelSampleStep;
          const ux = dx / dist;
          const uy = dy / dist;
          forces[i].x -= ux * force;
          forces[i].y -= uy * force;
          forces[j].x += ux * force;
          forces[j].y += uy * force;
        }
      }
      const centroids = computeNetworkCommunityCentroids(rows, communities);
      rows.forEach((node, index) => {
        const centroid = centroids.get(communities[index] || "");
        if (centroid && centroid.count > 1) {
          forces[index].x += (centroid.x - (Number(node.x) || 0)) * 0.0028 * cooling;
          forces[index].y += (centroid.y - (Number(node.y) || 0)) * 0.0028 * cooling;
        }
        const role = String(node?.role || node?.layoutBand || "").toLowerCase();
        const roleAnchor = role === "core" ? 2.4 : role === "adjacent" ? 1.5 : 0.82;
        forces[index].x += (seedPositions[index].x - (Number(node.x) || 0)) * anchorScale * roleAnchor * cooling;
        forces[index].y += (seedPositions[index].y - (Number(node.y) || 0)) * anchorScale * roleAnchor * cooling;
        forces[index].x -= (Number(node.x) || 0) * gravityScale;
        forces[index].y -= (Number(node.y) || 0) * gravityScale;
      });
      rows.forEach((node, index) => {
        velocity[index].x = clampLayout(velocity[index].x * 0.62 + forces[index].x, -maxStep, maxStep);
        velocity[index].y = clampLayout(velocity[index].y * 0.62 + forces[index].y, -maxStep, maxStep);
        node.x = (Number(node.x) || 0) + velocity[index].x;
        node.y = (Number(node.y) || 0) + velocity[index].y;
      });
    }
    applyNetworkCollisionPolish(rows, mode, collisionRadii);
    applyNetworkVisibilityTiers(rows, mode);
    applyNetworkVisibleNodeCollisionPolish(rows, mode);
    const labelCollisionSuppressedCount = suppressNetworkTopologyVisibleLabels(rows, mode);
    rows.forEach((node) => {
      delete node.__networkTopologyHubScore;
    });
    return buildNetworkQualityReport(rows, edgeRows, mode, startedAt, communities, {
      labelCollisionSuppressedCount,
    });
  }

  function applyNetworkCollisionPolish(nodes = [], mode = "small", collisionRadii = []) {
    const rows = Array.isArray(nodes) ? nodes : [];
    const passes = mode === "small" ? 3 : mode === "medium" ? 1 : mode === "large" ? 1 : 0;
    const sampleTarget = mode === "small" ? 240 : mode === "medium" ? 80 : mode === "large" ? 48 : 28;
    const sampleStep = rows.length <= sampleTarget ? 1 : Math.max(1, Math.ceil(rows.length / sampleTarget));
    for (let pass = 0; pass < passes; pass += 1) {
      for (let i = 0; i < rows.length; i += 1) {
        for (let j = i + 1; j < rows.length; j += 1) {
          if (sampleStep > 1 && (i * 13 + j * 23) % sampleStep !== 0) continue;
          let dx = (Number(rows[j].x) || 0) - (Number(rows[i].x) || 0);
          let dy = (Number(rows[j].y) || 0) - (Number(rows[i].y) || 0);
          if (Math.abs(dx) + Math.abs(dy) < 1e-6) {
            const angle = Math.PI * 2 * stableHashUnit(`network-collision|${rows[i]?.id || i}|${rows[j]?.id || j}`);
            dx = Math.cos(angle) * 0.1;
            dy = Math.sin(angle) * 0.1;
          }
          const dist = Math.max(0.01, Math.hypot(dx, dy));
          const minDist = (collisionRadii[i] || nodeCollisionRadius(rows[i])) + (collisionRadii[j] || nodeCollisionRadius(rows[j])) + 8;
          if (dist >= minDist) continue;
          const push = (minDist - dist) * 0.52;
          const ux = dx / dist;
          const uy = dy / dist;
          rows[i].x = (Number(rows[i].x) || 0) - ux * push;
          rows[i].y = (Number(rows[i].y) || 0) - uy * push;
          rows[j].x = (Number(rows[j].x) || 0) + ux * push;
          rows[j].y = (Number(rows[j].y) || 0) + uy * push;
        }
      }
    }
  }

  function applyNetworkVisibleNodeCollisionPolish(nodes = [], mode = "small") {
    const rows = (Array.isArray(nodes) ? nodes : [])
      .map((node, index) => ({ node, index }))
      .filter(({ node }) => isNetworkQualityVisibleNode(node, mode));
    if (rows.length <= 1) return 0;
    const passes = mode === "small" ? 1 : mode === "medium" ? 2 : mode === "large" ? 5 : 0;
    if (passes <= 0) return 0;
    rows.forEach((row, order) => {
      row.order = order;
      row.radius = nodeRadius(row.node);
    });
    const maxRadius = rows.reduce((max, row) => Math.max(max, Number(row.radius) || 18), 18);
    const cellSize = Math.max(64, maxRadius * 2 + 16);
    let adjustments = 0;
    for (let pass = 0; pass < passes; pass += 1) {
      const buckets = new Map();
      rows.forEach((row) => {
        const node = row.node;
        row.cx = Math.floor((Number(node?.x) || 0) / cellSize);
        row.cy = Math.floor((Number(node?.y) || 0) / cellSize);
        const key = `${row.cx}:${row.cy}`;
        const bucket = buckets.get(key);
        if (bucket) bucket.push(row);
        else buckets.set(key, [row]);
      });
      rows.forEach((leftRow) => {
        const left = leftRow.node;
        for (let ox = -1; ox <= 1; ox += 1) {
          for (let oy = -1; oy <= 1; oy += 1) {
            const bucket = buckets.get(`${leftRow.cx + ox}:${leftRow.cy + oy}`);
            if (!bucket) continue;
            for (let k = 0; k < bucket.length; k += 1) {
              const rightRow = bucket[k];
              if (rightRow.order <= leftRow.order) continue;
              const right = rightRow.node;
              let dx = (Number(right.x) || 0) - (Number(left.x) || 0);
              let dy = (Number(right.y) || 0) - (Number(left.y) || 0);
              if (Math.abs(dx) + Math.abs(dy) < 1e-6) {
                const angle = Math.PI * 2 * stableHashUnit(`network-visible-collision|${left?.id || leftRow.index}|${right?.id || rightRow.index}`);
                dx = Math.cos(angle) * 0.1;
                dy = Math.sin(angle) * 0.1;
              }
              const dist = Math.max(0.01, Math.hypot(dx, dy));
              const minDist = (Number(leftRow.radius) || nodeRadius(left)) + (Number(rightRow.radius) || nodeRadius(right)) + 12;
              if (dist >= minDist) continue;
              const push = (minDist - dist) * (mode === "large" ? 0.68 : 0.58);
              const ux = dx / dist;
              const uy = dy / dist;
              const leftLock = 1;
              const rightLock = 1;
              left.x = (Number(left.x) || 0) - ux * push * leftLock;
              left.y = (Number(left.y) || 0) - uy * push * leftLock;
              right.x = (Number(right.x) || 0) + ux * push * rightLock;
              right.y = (Number(right.y) || 0) + uy * push * rightLock;
              adjustments += 1;
            }
          }
        }
      });
    }
    return adjustments;
  }

    function buildNetworkQualityReport(nodes = [], edgeRows = [], mode = "small", startedAt = nowNetworkMs(), communities = [], extras = {}) {
      const rows = Array.isArray(nodes) ? nodes : [];
      const durationMs = Math.max(0, nowNetworkMs() - startedAt);
      const communityBoxes = computeNetworkCommunityBoxes(rows, communities, mode);
      const qualityEdgeCount = Array.isArray(edgeRows) ? edgeRows.length : 0;
      const visibleTierStats = {
        "node:visible": rows.length,
        "label:visible": rows.length,
        "edge:visible": qualityEdgeCount,
      };
      return {
        durationMs: Number(durationMs.toFixed(2)),
        nodeOverlapCount: estimateNetworkNodeOverlap(rows, mode),
        labelOverlapEstimate: estimateNetworkLabelOverlap(rows, mode),
        edgeCrossingSample: estimateNetworkEdgeCrossings(rows, edgeRows, mode),
        edgeLengthStdDev: Number(estimateNetworkEdgeLengthStdDev(rows, edgeRows, mode).toFixed(2)),
        clusterBBoxOverlapCount: estimateBoxOverlapCount(communityBoxes),
        mode,
        communityCount: communityBoxes.length,
        labelCollisionSuppressedCount: Math.max(0, Number(extras?.labelCollisionSuppressedCount) || 0),
        qualityScope: "all",
        qualityNodeCount: rows.length,
        qualityLabelCount: rows.length,
        qualityEdgeCount,
        visibleTierStats,
      };
    }

    function isNetworkSkeletonQualityMode(mode = "small") {
      return false;
    }

    function isNetworkQualityVisibleNode(node, mode = "small") {
      return true;
    }

    function isNetworkQualityVisibleLabel(node, mode = "small") {
      return true;
    }

    function isNetworkQualityVisibleEdge(nodes = [], edge = {}, mode = "small") {
      return true;
    }

    function estimateNetworkNodeOverlap(nodes = [], mode = "small") {
      const sourceRows = Array.isArray(nodes) ? nodes : [];
      const rows = isNetworkSkeletonQualityMode(mode)
        ? sourceRows.filter((node) => isNetworkQualityVisibleNode(node, mode))
        : sourceRows;
      const target = mode === "small" ? 420 : mode === "medium" ? 160 : mode === "large" ? 72 : 24;
      const step = rows.length <= target ? 1 : Math.max(1, Math.ceil(rows.length / target));
      let count = 0;
      for (let i = 0; i < rows.length; i += 1) {
        for (let j = i + 1; j < rows.length; j += 1) {
          if (step > 1 && (i * 19 + j * 29) % step !== 0) continue;
          const dx = (Number(rows[j].x) || 0) - (Number(rows[i].x) || 0);
          const dy = (Number(rows[j].y) || 0) - (Number(rows[i].y) || 0);
        const minDist = nodeRadius(rows[i]) + nodeRadius(rows[j]) + 8;
        if (dx * dx + dy * dy < minDist * minDist) count += step;
      }
    }
    return count;
  }

  function networkLabelBox(node) {
    const half = nodeLabelHalfWidth(node);
    const r = nodeRadius(node);
    const bottom = getNodeLayoutMetrics(node).labelBottom || r + 36;
    return {
      minX: (Number(node?.x) || 0) - half,
      maxX: (Number(node?.x) || 0) + half,
      minY: (Number(node?.y) || 0) + r + 6,
      maxY: (Number(node?.y) || 0) + r + 6 + bottom,
    };
  }

  function boxesOverlap(a, b) {
    return a.minX <= b.maxX && a.maxX >= b.minX && a.minY <= b.maxY && a.maxY >= b.minY;
  }

  function expandNetworkLabelBox(box, padding = 0) {
    const pad = Math.max(0, Number(padding) || 0);
    return {
      minX: Number(box?.minX || 0) - pad,
      maxX: Number(box?.maxX || 0) + pad,
      minY: Number(box?.minY || 0) - pad,
      maxY: Number(box?.maxY || 0) + pad,
    };
  }

    function estimateNetworkLabelOverlap(nodes = [], mode = "small") {
      const sourceRows = Array.isArray(nodes) ? nodes : [];
      const rows = sourceRows.filter((node) => isNetworkQualityVisibleLabel(node, mode));
      const target = mode === "small" ? 320 : mode === "medium" ? 120 : mode === "large" ? 56 : 24;
      const step = rows.length <= target ? 1 : Math.max(1, Math.ceil(rows.length / target));
      let count = 0;
      for (let i = 0; i < rows.length; i += 1) {
        const a = networkLabelBox(rows[i]);
        for (let j = i + 1; j < rows.length; j += 1) {
          if (step > 1 && (i * 11 + j * 37) % step !== 0) continue;
          if (boxesOverlap(a, networkLabelBox(rows[j]))) count += step;
        }
    }
    return count;
  }

    function estimateNetworkEdgeLengthStdDev(nodes = [], edgeRows = [], mode = "small") {
      if (!Array.isArray(edgeRows) || !edgeRows.length) return 0;
      const visibleEdges = edgeRows.filter((edge) => isNetworkQualityVisibleEdge(nodes, edge, mode));
      if (!visibleEdges.length) return 0;
      const target = mode === "small" ? 800 : mode === "medium" ? 520 : mode === "large" ? 300 : 120;
      const step = visibleEdges.length <= target ? 1 : Math.max(1, Math.ceil(visibleEdges.length / target));
      const lengths = visibleEdges.filter((_, index) => index % step === 0).map((edge) => {
        const a = nodes[edge.source] || {};
        const b = nodes[edge.target] || {};
        return Math.hypot((Number(b.x) || 0) - (Number(a.x) || 0), (Number(b.y) || 0) - (Number(a.y) || 0));
    });
    const mean = lengths.reduce((sum, value) => sum + value, 0) / Math.max(1, lengths.length);
    const variance = lengths.reduce((sum, value) => sum + (value - mean) * (value - mean), 0) / Math.max(1, lengths.length);
    return Math.sqrt(variance);
  }

  function segmentsCross(a, b, c, d) {
    const orient = (p, q, r) => (q.x - p.x) * (r.y - p.y) - (q.y - p.y) * (r.x - p.x);
    const o1 = orient(a, b, c);
    const o2 = orient(a, b, d);
    const o3 = orient(c, d, a);
    const o4 = orient(c, d, b);
    return ((o1 > 0 && o2 < 0) || (o1 < 0 && o2 > 0)) && ((o3 > 0 && o4 < 0) || (o3 < 0 && o4 > 0));
  }

    function estimateNetworkEdgeCrossings(nodes = [], edgeRows = [], mode = "small") {
      const rows = (Array.isArray(edgeRows) ? edgeRows : []).filter((edge) =>
        isNetworkQualityVisibleEdge(nodes, edge, mode)
      );
      if (rows.length < 2) return 0;
      const target = mode === "small" ? 260 : mode === "medium" ? 100 : mode === "large" ? 52 : 24;
      const step = rows.length <= target ? 1 : Math.max(1, Math.ceil(rows.length / target));
    const sampled = rows.filter((_, index) => index % step === 0);
    let count = 0;
    for (let i = 0; i < sampled.length; i += 1) {
      for (let j = i + 1; j < sampled.length; j += 1) {
        const a = sampled[i];
        const b = sampled[j];
        if (a.source === b.source || a.source === b.target || a.target === b.source || a.target === b.target) continue;
        const p1 = { x: Number(nodes[a.source]?.x) || 0, y: Number(nodes[a.source]?.y) || 0 };
        const p2 = { x: Number(nodes[a.target]?.x) || 0, y: Number(nodes[a.target]?.y) || 0 };
        const p3 = { x: Number(nodes[b.source]?.x) || 0, y: Number(nodes[b.source]?.y) || 0 };
        const p4 = { x: Number(nodes[b.target]?.x) || 0, y: Number(nodes[b.target]?.y) || 0 };
        if (segmentsCross(p1, p2, p3, p4)) count += step;
      }
    }
    return count;
  }

    function computeNetworkCommunityBoxes(nodes = [], communities = [], mode = "small") {
      const boxes = new Map();
      (Array.isArray(nodes) ? nodes : []).forEach((node, index) => {
        if (!isNetworkQualityVisibleNode(node, mode)) return;
        const key = communities[index] || String(node?.layoutCommunity || node?.layoutClusterId || node?.clusterId || "");
        if (!key) return;
      const r = nodeCollisionRadius(node);
      const box = boxes.get(key) || { minX: Infinity, minY: Infinity, maxX: -Infinity, maxY: -Infinity };
      box.minX = Math.min(box.minX, (Number(node?.x) || 0) - r);
      box.minY = Math.min(box.minY, (Number(node?.y) || 0) - r);
      box.maxX = Math.max(box.maxX, (Number(node?.x) || 0) + r);
      box.maxY = Math.max(box.maxY, (Number(node?.y) || 0) + r);
      boxes.set(key, box);
    });
    return Array.from(boxes.values()).filter((box) => Number.isFinite(box.minX) && Number.isFinite(box.maxX));
  }

  function estimateBoxOverlapCount(boxes = []) {
    let count = 0;
    for (let i = 0; i < boxes.length; i += 1) {
      for (let j = i + 1; j < boxes.length; j += 1) {
        if (boxesOverlap(boxes[i], boxes[j])) count += 1;
      }
    }
    return count;
  }

  function seedNetworkTopologyFromSemantic(nodes = [], clusterRows = [], clusterPlacementById = {}, nodeMetaById = {}, mode = "medium") {
    const rows = Array.isArray(nodes) ? nodes : [];
    const nodesById = new Map(rows.map((node) => [String(node?.id || ""), node]));
    const assigned = new Set();
    const clusterList = (Array.isArray(clusterRows) ? clusterRows : []).slice();
    clusterList
      .sort((a, b) => String(a?.clusterId || "").localeCompare(String(b?.clusterId || ""), "zh-CN"))
      .forEach((cluster, clusterIndex) => {
        const clusterId = String(cluster?.clusterId || `cluster-${clusterIndex}`);
        const placement = clusterPlacementById?.[clusterId] || {};
        const ids = sortStableIds(cluster?.nodeIds || []).filter((id) => nodesById.has(id));
        if (!ids.length) return;
        const fallbackAngle = -Math.PI + (Math.PI * 2 * (clusterIndex + stableHashUnit(clusterId) * 0.18)) / Math.max(1, clusterList.length);
        const fallbackRadius = Math.sqrt(Math.max(1, rows.length)) * 74 + 180;
        const cx = Number.isFinite(Number(placement?.x)) ? Number(placement.x) : Math.cos(fallbackAngle) * fallbackRadius;
        const cy = Number.isFinite(Number(placement?.y)) ? Number(placement.y) : Math.sin(fallbackAngle) * fallbackRadius;
        const roleBuckets = { core: [], adjacent: [], leaf: [] };
        ids.forEach((id) => {
          const node = nodesById.get(id);
          const role = String(nodeMetaById?.[id]?.role || node?.role || node?.layoutBand || "leaf").toLowerCase();
          if (role === "core") roleBuckets.core.push(node);
          else if (role === "adjacent") roleBuckets.adjacent.push(node);
          else roleBuckets.leaf.push(node);
        });
        const placeBucket = (bucket, baseRadius, gap, jitterPrefix) => {
          const count = bucket.length;
          bucket.forEach((node, index) => {
            const angle = Math.PI * 2 * ((index + stableHashUnit(`${jitterPrefix}|${clusterId}|${node.id}`) * 0.31) / Math.max(1, count));
            const radius = count <= 1 ? baseRadius : baseRadius + Math.sqrt(index) * gap;
            node.x = cx + Math.cos(angle) * radius;
            node.y = cy + Math.sin(angle) * radius;
            assigned.add(String(node.id || ""));
          });
        };
        if (roleBuckets.core.length === 1) {
          const node = roleBuckets.core[0];
          node.x = cx;
          node.y = cy;
          assigned.add(String(node.id || ""));
        } else {
          placeBucket(roleBuckets.core, 26, 18, "network-topology-core-seed");
        }
        placeBucket(roleBuckets.adjacent, mode === "medium" ? 96 : mode === "large" ? 124 : 152, 20, "network-topology-adj-seed");
        placeBucket(roleBuckets.leaf, mode === "medium" ? 168 : mode === "large" ? 220 : 280, 28, "network-topology-leaf-seed");
      });
    const fallbackRows = rows
      .filter((node) => !assigned.has(String(node?.id || "")))
      .sort((a, b) => String(a?.id || "").localeCompare(String(b?.id || ""), "zh-CN"));
    if (fallbackRows.length) {
      const cols = Math.max(1, Math.ceil(Math.sqrt(fallbackRows.length)));
      const spacing = mode === "medium" ? 112 : mode === "large" ? 148 : 188;
      const halfW = ((cols - 1) * spacing) / 2;
      const halfH = ((Math.ceil(fallbackRows.length / cols) - 1) * spacing) / 2;
      fallbackRows.forEach((node, index) => {
        const cx = index % cols;
        const cy = Math.floor(index / cols);
        node.x = cx * spacing - halfW;
        node.y = cy * spacing - halfH;
      });
    }
    return assigned.size + fallbackRows.length;
  }

  function layoutNetwork(nodes, edges, focusId, opts = {}) {
    void focusId;
    const hints = opts?.hints && typeof opts.hints === "object" ? opts.hints : {};
    const hasExplicitSemantic = !!(hints?.semantic && typeof hints.semantic === "object");
    const semanticSource = hasExplicitSemantic ? hints.semantic : buildFallbackSemantic(nodes);
    const cfg = { ...DEFAULT_CFG, ...(semanticSource?.config || {}) };
    const nodeMetaById = semanticSource?.nodeMetaById || {};
    const clusterRows = Array.isArray(semanticSource?.clusters) ? semanticSource.clusters.slice() : [];
    const clusterPlacementById = semanticSource?.clusterPlacementById || {};
    const leafEntryById = semanticSource?.leafEntryById || {};
    const nodesById = new Map((nodes || []).map((node) => [String(node?.id || ""), node]));
    const canvasBounds = semanticSource?.canvasBounds || { minX: -1400, minY: -1100, maxX: 1400, maxY: 1100 };
    const topologyMode = resolveNetworkPlanMode((nodes || []).length, (edges || []).length);
    const sectorCache = resolveNetworkSectorPlacementCache({});
    const requiresSectorCacheHit = sectorCache && typeof sectorCache === "object" && sectorCache.requireHit === true;
    const topologySeedOnly =
      topologyMode !== "small" ||
      (hasExplicitSemantic && hints.collectNetworkSectorPlacementPayloads !== true && !requiresSectorCacheHit);
    const skipFullSemanticMeta = topologySeedOnly && topologyMode === "xlarge";
    const clusterCaseById = {};
    if (!skipFullSemanticMeta) {
      clusterRows.forEach((cluster) => {
        const cid = String(cluster?.clusterId || "");
        if (!cid) return;
        clusterCaseById[cid] = String(cluster?.layoutCase || "general");
      });
    }

    (nodes || []).forEach((node) => {
      const id = String(node?.id || "");
      if (skipFullSemanticMeta) {
        const meta = nodeMetaById?.[id] || {};
        if (!node.role && meta?.role) node.role = meta.role;
        if (!node.clusterId && meta?.clusterId) node.clusterId = meta.clusterId;
      } else {
        applyNodeSemanticMeta(node, nodeMetaById?.[id] || {});
      }
      const cid = String(node?.clusterId || "");
      node.layoutCase = clusterCaseById?.[cid] || "general";
      node.layoutClusterId = cid;
    });

    const reports = [];
    if (topologySeedOnly) {
      if (topologyMode !== "xlarge") {
        seedNetworkTopologyFromSemantic(nodes || [], clusterRows, clusterPlacementById, nodeMetaById, topologyMode);
      }
      const topologyQuality = applyNetworkTopologyRelaxation(nodes || [], edges || []);
      if (!hints.__layoutReport || typeof hints.__layoutReport !== "object") {
        hints.__layoutReport = {};
      }
      hints.__layoutReport.network = {
        clusterCount: Math.max(0, clusterRows.length),
        clusters: [],
        mode: "network",
        seedMode: "topology-first",
        semanticSeedSkipped: topologyMode === "xlarge",
        topology: topologyQuality,
        quality: topologyQuality,
      };
      return nodes;
    }
    const placedBBoxes = [];
    const placedLeafZones = [];
    const sectorPlacementPayloads = hints.collectNetworkSectorPlacementPayloads === true ? [] : null;
    const assigned = new Set();
    const placedClusterIds = new Set();
    const plannedClusterBBoxById = {};
    clusterRows.forEach((cluster) => {
      const clusterId = String(cluster?.clusterId || "");
      if (!clusterId) return;
      const placement = clusterPlacementById?.[clusterId] || {};
      const width = Math.max(
        180,
        Number(placement?.width) ||
          Number(cluster?.bbox?.width) ||
          Math.max(220, Math.sqrt(Math.max(1, (cluster?.nodeIds || []).length)) * 120)
      );
      const height = Math.max(
        150,
        Number(placement?.height) ||
          Number(cluster?.bbox?.height) ||
          Math.max(180, Math.sqrt(Math.max(1, (cluster?.nodeIds || []).length)) * 102)
      );
      const cx = Number(placement?.x) || 0;
      const cy = Number(placement?.y) || 0;
      plannedClusterBBoxById[clusterId] = {
        clusterId,
        minX: cx - width / 2,
        maxX: cx + width / 2,
        minY: cy - height / 2,
        maxY: cy + height / 2,
      };
    });
    clusterRows
      .slice()
      .sort((a, b) => String(a?.clusterId || "").localeCompare(String(b?.clusterId || ""), "zh-CN"))
      .forEach((cluster) => {
        const clusterId = String(cluster?.clusterId || "");
        const placement = clusterPlacementById?.[clusterId] || { x: 0, y: 0 };
        const futureBBoxes = clusterRows
          .map((row) => String(row?.clusterId || ""))
          .filter((id) => id && id !== clusterId && !placedClusterIds.has(id))
          .map((id) => ({ ...(plannedClusterBBoxById?.[id] || {}), weight: 0.72 }));
        const occupiedClusterBBoxes = placedBBoxes.map((row) => ({ ...row, weight: 1.12 }));
        const globalMap = computeGlobalOccupiedMap(
          occupiedClusterBBoxes.concat(futureBBoxes),
          placedLeafZones,
          canvasBounds,
          { boundaryPadding: 58 }
        );
        const laid = layoutNetworkCluster(
          {
            cluster,
            placement,
            nodeMetaById,
            nodesById,
            leafEntryById,
            networkNodeUpdatesByClusterId: semanticSource?.networkNodeUpdatesByClusterId || {},
            networkCenterReportsByClusterId: semanticSource?.networkCenterReportsByClusterId || {},
            networkLeafZonesByClusterId: semanticSource?.networkLeafZonesByClusterId || {},
          },
          {
            config: cfg,
            globalMap,
            sectorPlacementPayloadCollector: sectorPlacementPayloads,
          }
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
        placedBBoxes.push({
          clusterId,
          minX: laid.bbox.minX,
          minY: laid.bbox.minY,
          maxX: laid.bbox.maxX,
          maxY: laid.bbox.maxY,
        });
        placedLeafZones.push(...(laid.leafZones || []));
        placedClusterIds.add(clusterId);
      });

    const fallbackRows = (nodes || [])
      .filter((node) => !assigned.has(String(node?.id || "")))
      .sort((a, b) => String(a?.id || "").localeCompare(String(b?.id || ""), "zh-CN"));
    if (fallbackRows.length) {
      const cols = Math.max(1, Math.ceil(Math.sqrt(fallbackRows.length)));
      const spacing = 130;
      const halfW = ((cols - 1) * spacing) / 2;
      const halfH = ((Math.ceil(fallbackRows.length / cols) - 1) * spacing) / 2;
      fallbackRows.forEach((node, index) => {
        const cx = index % cols;
        const cy = Math.floor(index / cols);
        node.x = cx * spacing - halfW;
        node.y = cy * spacing - halfH;
      });
    }

    const topologyQuality = applyNetworkTopologyRelaxation(nodes || [], edges || []);

    if (!hints.__layoutReport || typeof hints.__layoutReport !== "object") {
      hints.__layoutReport = {};
    }
    hints.__layoutReport.network = {
      clusterCount: reports.length,
      clusters: reports,
      mode: "network",
      topology: topologyQuality,
      quality: topologyQuality,
    };
    if (Array.isArray(sectorPlacementPayloads) && sectorPlacementPayloads.length) {
      hints.__layoutReport.networkSectorPlacementPayloads = sectorPlacementPayloads;
    }
    return nodes;
  }

  if (typeof engine.registerLayoutExports === "function") {
    engine.registerLayoutExports({
      layoutNetwork,
      layoutNetworkCluster,
      placeNetworkBackbone,
      resolveNetworkSkeletonScale,
      computeLocalOccupiedMap,
      computeGlobalOccupiedMap,
      buildCenterTerritoryRanges,
      intersectAngleRanges,
      scoreAngularBucketAngleConstraints,
      scoreAngularBucketBoundaryPenalty,
      scoreAngularBucketHardZonePenalty,
      scoreAngularBucketHardCorridorPenalty,
      scoreAngularBucketOccupiedScore,
      scoreAngularBucketScore,
      projectNetworkRayPenalty,
      projectNetworkCandidateScore,
      projectNetworkCandidateSelection,
      projectNetworkCandidateAttemptSelection,
      projectNetworkSectorEnvelope,
      projectNetworkSectorUniformity,
      projectNetworkNodeMetrics,
      projectNetworkLeafPlacementRows,
      buildAngularBucketSectorPayload,
      buildNetworkSectorPlacementPayload,
      collectNetworkSectorPlacementPayload,
      projectNetworkGeneralAttemptSelection,
      projectNetworkGeneralBatchPlacement,
      projectNetworkSectorPlacementFromPlan,
      applyNetworkGeneralPlacementResult,
      applyNetworkSectorPlacementResult,
      applyNetworkSectorEnvelopePlacementResult,
      applyNetworkSectorPostProcessPlacementResult,
      scoreAngularBucketsFromPayload,
      scoreAngularBuckets,
      projectAngularBucketSectorProjection,
      selectAngularBucketWindow,
      planAngularBucketSectors,
      planNetworkSectorFastRingLayers,
      placeNetworkOutwardLeafSectors,
      applyNetworkTopologyRelaxation,
      placeLeafSectors,
    });
  } else {
    engine.layoutNetwork = layoutNetwork;
    engine.layoutNetworkCluster = layoutNetworkCluster;
    engine.placeNetworkBackbone = placeNetworkBackbone;
    engine.resolveNetworkSkeletonScale = resolveNetworkSkeletonScale;
    engine.computeLocalOccupiedMap = computeLocalOccupiedMap;
    engine.computeGlobalOccupiedMap = computeGlobalOccupiedMap;
    engine.buildCenterTerritoryRanges = buildCenterTerritoryRanges;
    engine.intersectAngleRanges = intersectAngleRanges;
    engine.scoreAngularBucketAngleConstraints = scoreAngularBucketAngleConstraints;
    engine.scoreAngularBucketBoundaryPenalty = scoreAngularBucketBoundaryPenalty;
    engine.scoreAngularBucketHardZonePenalty = scoreAngularBucketHardZonePenalty;
    engine.scoreAngularBucketHardCorridorPenalty = scoreAngularBucketHardCorridorPenalty;
    engine.scoreAngularBucketOccupiedScore = scoreAngularBucketOccupiedScore;
    engine.scoreAngularBucketScore = scoreAngularBucketScore;
    engine.projectNetworkRayPenalty = projectNetworkRayPenalty;
    engine.projectNetworkCandidateScore = projectNetworkCandidateScore;
    engine.projectNetworkCandidateSelection = projectNetworkCandidateSelection;
    engine.projectNetworkCandidateAttemptSelection = projectNetworkCandidateAttemptSelection;
    engine.projectNetworkSectorEnvelope = projectNetworkSectorEnvelope;
    engine.projectNetworkSectorUniformity = projectNetworkSectorUniformity;
    engine.projectNetworkNodeMetrics = projectNetworkNodeMetrics;
    engine.projectNetworkLeafPlacementRows = projectNetworkLeafPlacementRows;
    engine.buildAngularBucketSectorPayload = buildAngularBucketSectorPayload;
    engine.buildNetworkSectorPlacementPayload = buildNetworkSectorPlacementPayload;
    engine.collectNetworkSectorPlacementPayload = collectNetworkSectorPlacementPayload;
    engine.projectNetworkGeneralAttemptSelection = projectNetworkGeneralAttemptSelection;
    engine.projectNetworkGeneralBatchPlacement = projectNetworkGeneralBatchPlacement;
    engine.projectNetworkSectorPlacementFromPlan = projectNetworkSectorPlacementFromPlan;
    engine.applyNetworkGeneralPlacementResult = applyNetworkGeneralPlacementResult;
    engine.applyNetworkSectorPlacementResult = applyNetworkSectorPlacementResult;
    engine.applyNetworkSectorEnvelopePlacementResult = applyNetworkSectorEnvelopePlacementResult;
    engine.applyNetworkSectorPostProcessPlacementResult = applyNetworkSectorPostProcessPlacementResult;
    engine.scoreAngularBucketsFromPayload = scoreAngularBucketsFromPayload;
    engine.scoreAngularBuckets = scoreAngularBuckets;
    engine.projectAngularBucketSectorProjection = projectAngularBucketSectorProjection;
    engine.selectAngularBucketWindow = selectAngularBucketWindow;
    engine.planAngularBucketSectors = planAngularBucketSectors;
    engine.planNetworkSectorFastRingLayers = planNetworkSectorFastRingLayers;
    engine.placeNetworkOutwardLeafSectors = placeNetworkOutwardLeafSectors;
    engine.applyNetworkTopologyRelaxation = applyNetworkTopologyRelaxation;
    engine.placeLeafSectors = placeLeafSectors;
  }

  engine.registerLayoutModeRunner("network", (ctx) => {
    try {
      layoutNetwork(ctx.nodes, ctx.edges, ctx.focusId, { hints: ctx?.hints || {} });
      return true;
    } catch (e) {
      if (!ctx?.hints || typeof ctx.hints !== "object") return false;
      if (!ctx.hints.__layoutReport || typeof ctx.hints.__layoutReport !== "object") {
        ctx.hints.__layoutReport = {};
      }
      ctx.hints.__layoutReport.networkError = {
        message: String(e?.message || e || ""),
      };
      return false;
    }
  });
})();
