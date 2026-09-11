#!/usr/bin/env node

import { fileURLToPath } from "node:url";

const SHA256 = /^[a-f0-9]{64}$/u;

export function validateInstalledMarkerIdentity({
  marker,
  workspaceContentSha256,
}) {
  const value = marker && typeof marker === "object" && !Array.isArray(marker)
    ? marker
    : {};
  const failures = [];
  const packageAuthorityFileSha256 = text(value.packageSha256);
  const sourceTreeSha256 = text(value.sourceTreeSha256);
  const sourceTreeFileCount = value.sourceTreeFileCount;
  const localRemount =
    value.transactionVersion === "RuntimeCacheRemountTransactionV1" &&
    value.commitOrder === "marketplace_pointer_last";
  const hostMaterialized = value.installType === "user";

  if (localRemount === hostMaterialized) {
    failures.push("installed marker identity kind is ambiguous");
  }
  if (!SHA256.test(packageAuthorityFileSha256)) {
    failures.push("packaged-build authority file SHA-256 is invalid");
  }
  if (localRemount) {
    if (!SHA256.test(workspaceContentSha256) || packageAuthorityFileSha256 !== workspaceContentSha256) {
      failures.push("local-remount content SHA-256 does not match the workspace content manifest");
    }
    if (sourceTreeSha256 || sourceTreeFileCount !== undefined) {
      failures.push("local-remount marker mixes host-materialized source-tree identity");
    }
  }
  if (hostMaterialized) {
    if (!SHA256.test(sourceTreeSha256)) {
      failures.push("host-materialized source-tree SHA-256 is invalid");
    }
    if (!Number.isSafeInteger(sourceTreeFileCount) || sourceTreeFileCount <= 0) {
      failures.push("host-materialized source-tree file count is invalid");
    }
    // packageSha256 intentionally remains distinct from sourceTreeSha256. It
    // is the independently anchored packaged-build authority file digest, not
    // a plugin-content digest and not a Hub catalog digest.
  }
  return {
    ok: failures.length === 0,
    kind: localRemount
      ? "local-remount"
      : hostMaterialized
        ? "host-materialized"
        : "unknown",
    packageAuthorityFileSha256,
    sourceTreeSha256,
    failures,
  };
}

export function validatePointerDigest(value, label) {
  return SHA256.test(text(value)) ? [] : [`${label} SHA-256 is invalid`];
}

export function classifyRuntimeInstallAuthority({
  expectedVersion,
  workspaceContentSha256,
  canonicalHostPresent,
  canonicalHostCanonical,
  canonicalVisibleVersions = [],
  canonicalMarker,
  legacyProjectionLabels = [],
}) {
  const version = text(expectedVersion);
  const visibleVersions = Array.isArray(canonicalVisibleVersions)
    ? canonicalVisibleVersions.map(text).filter(Boolean).sort()
    : [];
  const marker = canonicalHostPresent
    ? validateInstalledMarkerIdentity({
        marker: canonicalMarker,
        workspaceContentSha256,
      })
    : {
        ok: false,
        kind: "unknown",
        packageAuthorityFileSha256: "",
        sourceTreeSha256: "",
        failures: ["canonical host materialization is missing"],
      };
  const markerValue = canonicalMarker && typeof canonicalMarker === "object" && !Array.isArray(canonicalMarker)
    ? canonicalMarker
    : {};
  const failures = [
    !version ? "expected plugin version is missing" : "",
    !canonicalHostPresent ? "canonical host materialization is missing" : "",
    canonicalHostPresent && !canonicalHostCanonical
      ? "canonical host materialization path is unsafe"
      : "",
    canonicalHostPresent && marker.kind !== "host-materialized"
      ? "canonical host materialization marker is not host-issued"
      : "",
    ...marker.failures,
    canonicalHostPresent && text(markerValue.pluginName) !== "analytix-fund-analysis"
      ? "canonical host materialization plugin name is invalid"
      : "",
    canonicalHostPresent && text(markerValue.version) !== version
      ? "canonical host materialization version is invalid"
      : "",
    canonicalHostPresent && text(markerValue.marketplaceName) !== "analytix-hub"
      ? "canonical host materialization marketplace is invalid"
      : "",
    canonicalHostPresent && text(markerValue.managedBy) !== "analytix-hub"
      ? "canonical host materialization manager is invalid"
      : "",
    visibleVersions.length !== 1 || visibleVersions[0] !== version
      ? `canonical host materialization versions are invalid: ${visibleVersions.join(", ") || "none"}`
      : "",
  ].filter(Boolean);
  const warnings = Array.isArray(legacyProjectionLabels)
    ? legacyProjectionLabels.map(text).filter(Boolean)
    : [];
  return {
    ok: failures.length === 0,
    kind: marker.kind,
    packageAuthorityFileSha256: marker.packageAuthorityFileSha256,
    sourceTreeSha256: marker.sourceTreeSha256,
    failures,
    warnings,
  };
}

function text(value) {
  return typeof value === "string" ? value.trim() : "";
}

function selfTest() {
  const workspace = "1".repeat(64);
  const local = validateInstalledMarkerIdentity({
    workspaceContentSha256: workspace,
    marker: {
      packageSha256: workspace,
      transactionVersion: "RuntimeCacheRemountTransactionV1",
      commitOrder: "marketplace_pointer_last",
    },
  });
  const formal = validateInstalledMarkerIdentity({
    workspaceContentSha256: workspace,
    marker: {
      packageSha256: "2".repeat(64),
      sourceTreeSha256: "3".repeat(64),
      sourceTreeFileCount: 257,
      installType: "user",
    },
  });
  const wrongLocal = validateInstalledMarkerIdentity({
    workspaceContentSha256: workspace,
    marker: {
      packageSha256: "4".repeat(64),
      transactionVersion: "RuntimeCacheRemountTransactionV1",
      commitOrder: "marketplace_pointer_last",
    },
  });
  const collapsedFormal = validateInstalledMarkerIdentity({
    workspaceContentSha256: workspace,
    marker: {
      packageSha256: workspace,
      installType: "user",
    },
  });
  const canonicalMarker = {
    managedBy: "analytix-hub",
    marketplaceName: "analytix-hub",
    pluginName: "analytix-fund-analysis",
    version: "0.16.16",
    installType: "user",
    packageSha256: "2".repeat(64),
    sourceTreeSha256: "3".repeat(64),
    sourceTreeFileCount: 105,
  };
  const canonicalWithStaleLegacy = classifyRuntimeInstallAuthority({
    expectedVersion: "0.16.16",
    workspaceContentSha256: workspace,
    canonicalHostPresent: true,
    canonicalHostCanonical: true,
    canonicalVisibleVersions: ["0.16.16"],
    canonicalMarker,
    legacyProjectionLabels: ["marketplace:0.16.15"],
  });
  const forgedCanonical = classifyRuntimeInstallAuthority({
    expectedVersion: "0.16.16",
    workspaceContentSha256: workspace,
    canonicalHostPresent: true,
    canonicalHostCanonical: true,
    canonicalVisibleVersions: ["0.16.16"],
    canonicalMarker: { ...canonicalMarker, installType: "", packageSha256: workspace },
  });
  const legacyOnly = classifyRuntimeInstallAuthority({
    expectedVersion: "0.16.16",
    workspaceContentSha256: workspace,
    canonicalHostPresent: false,
    canonicalHostCanonical: false,
    canonicalVisibleVersions: [],
    canonicalMarker: null,
    legacyProjectionLabels: ["marketplace:0.16.16"],
  });
  const multipleCanonicalVersions = classifyRuntimeInstallAuthority({
    expectedVersion: "0.16.16",
    workspaceContentSha256: workspace,
    canonicalHostPresent: true,
    canonicalHostCanonical: true,
    canonicalVisibleVersions: ["0.16.15", "0.16.16"],
    canonicalMarker,
  });
  if (!local.ok || local.kind !== "local-remount" || !formal.ok || formal.kind !== "host-materialized" ||
      wrongLocal.ok || collapsedFormal.ok || validatePointerDigest("5".repeat(64), "pointer").length !== 0 ||
      validatePointerDigest("not-a-digest", "pointer").length === 0 ||
      !canonicalWithStaleLegacy.ok || canonicalWithStaleLegacy.warnings.length !== 1 ||
      forgedCanonical.ok || legacyOnly.ok || multipleCanonicalVersions.ok) {
    throw new Error("runtime install identity contract self-test failed");
  }
  console.log("Funds runtime install identity contract passed (10/10).");
}

if (process.argv[1] && fileURLToPath(import.meta.url) === fileURLToPath(new URL(`file://${process.argv[1]}`))) {
  selfTest();
}
