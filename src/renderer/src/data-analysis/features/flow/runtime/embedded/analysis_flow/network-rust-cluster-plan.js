/* Rust network cluster plan adapter */
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
  if (!engine || typeof engine !== "object") return;

  const DENSE_RUST_PLAN_LEAF_LIMIT = 72;

  function shouldUseRustNetworkClusterPlan(centerReports) {
    const rows = Array.isArray(centerReports) ? centerReports : [];
    if (!rows.length) return true;
    if (rows.length > 1) return false;
    if (rows.some((row) => String(row?.centerType || "").toLowerCase() === "adjacent")) return false;
    return rows.every((row) => Math.max(0, Number(row?.leafCount) || 0) <= DENSE_RUST_PLAN_LEAF_LIMIT);
  }

  function collectRustNetworkClusterPlan(clusterData, clusterNodeSet, nodesById, options = {}) {
    const cluster = clusterData?.cluster || {};
    const clusterId = String(cluster?.clusterId || "");
    const byCluster = clusterData?.networkNodeUpdatesByClusterId || {};
    const sourceRows = Array.isArray(byCluster?.[clusterId]) ? byCluster[clusterId] : null;
    if (!sourceRows) return null;
    if (!(clusterNodeSet instanceof Set)) {
      throw new Error("network-rust-cluster-node-set-missing");
    }
    if (!nodesById || typeof nodesById.get !== "function") {
      throw new Error("network-rust-node-map-missing");
    }
    const makeZone = options?.makeZone;
    if (typeof makeZone !== "function") {
      throw new Error("network-rust-zone-factory-missing");
    }
    const reportRows = Array.isArray(clusterData?.networkCenterReportsByClusterId?.[clusterId])
      ? clusterData.networkCenterReportsByClusterId[clusterId]
      : null;
    if (!reportRows || !reportRows.length) {
      throw new Error("network-rust-report-missing");
    }
    const leafZoneRows = Array.isArray(clusterData?.networkLeafZonesByClusterId?.[clusterId])
      ? clusterData.networkLeafZonesByClusterId[clusterId]
      : null;
    if (!leafZoneRows) {
      throw new Error("network-rust-leaf-zones-missing");
    }
    const centerReports = reportRows.map((row) => {
      const reportCenterId = String(row?.centerId || "").trim();
      if (!reportCenterId || !clusterNodeSet.has(reportCenterId)) {
        throw new Error("network-rust-report-invalid");
      }
      return {
        centerId: reportCenterId,
        centerType: String(row?.centerType || "layoutAnchor"),
        x: Number(row?.x) || 0,
        y: Number(row?.y) || 0,
        leafCount: Number(row?.leafCount) || 0,
        layerCount: Number(row?.layerCount) || 0,
        sectorCount: Number(row?.sectorCount) || 0,
        sectorRanges: Array.isArray(row?.sectorRanges) ? row.sectorRanges : [],
        multiSectorUsed: !!row?.multiSectorUsed,
        leafMode: String(row?.leafMode || "rust-sector"),
        sectorFastPath: row?.sectorFastPath ? String(row.sectorFastPath) : null,
      };
    });
    const centerIds = new Set(centerReports.map((row) => row.centerId));
    const positions = new Map();
    sourceRows.forEach((row) => {
      const id = String(row?.id || "").trim();
      const x = Number(row?.x);
      const y = Number(row?.y);
      if (!id || !clusterNodeSet.has(id) || !Number.isFinite(x) || !Number.isFinite(y)) {
        throw new Error("network-rust-position-invalid");
      }
      if (positions.has(id)) {
        throw new Error("network-rust-position-duplicate");
      }
      positions.set(id, { x, y });
    });
    if (positions.size !== clusterNodeSet.size) {
      throw new Error("network-rust-position-incomplete");
    }
    const leafZones = leafZoneRows.map((row) => {
      const id = String(row?.id || "").trim();
      const x = Number(row?.x);
      const y = Number(row?.y);
      const ownerCenterId = String(row?.ownerCenterId || "").trim();
      const ownerSectorIndex = Number(row?.ownerSectorIndex);
      if (!id || !clusterNodeSet.has(id) || !Number.isFinite(x) || !Number.isFinite(y)) {
        throw new Error("network-rust-leaf-zone-invalid");
      }
      if (!ownerCenterId) {
        throw new Error("network-rust-owner-missing");
      }
      if (!centerIds.has(ownerCenterId)) {
        throw new Error("network-rust-owner-invalid");
      }
      if (
        !Number.isInteger(ownerSectorIndex) ||
        ownerSectorIndex < 0 ||
        ownerSectorIndex >= centerReports.length
      ) {
        throw new Error("network-rust-owner-sector-invalid");
      }
      const pos = positions.get(id);
      if (!pos) {
        throw new Error("network-rust-leaf-zone-position-missing");
      }
      if (pos.x !== x || pos.y !== y) {
        throw new Error("network-rust-leaf-zone-position-mismatch");
      }
      const node = nodesById.get(id);
      if (!node) {
        throw new Error("network-rust-leaf-zone-node-missing");
      }
      return {
        ...makeZone(id, x, y, node, 4),
        ownerCenterId,
        ownerSectorIndex,
      };
    });
    const reportedLeafCount = centerReports.reduce((sum, row) => sum + (Number(row?.leafCount) || 0), 0);
    if (reportedLeafCount !== leafZones.length) {
      throw new Error("network-rust-report-leaf-count-mismatch");
    }
    if (!shouldUseRustNetworkClusterPlan(centerReports)) return null;
    return { positions, centerReports, leafZones };
  }

  if (typeof engine.registerLayoutExports === "function") {
    engine.registerLayoutExports({ collectRustNetworkClusterPlan });
  } else {
    engine.collectRustNetworkClusterPlan = collectRustNetworkClusterPlan;
  }
})();
