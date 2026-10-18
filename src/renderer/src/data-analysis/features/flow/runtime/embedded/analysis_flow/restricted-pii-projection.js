(() => {
  // The host installs the shared frozen projection before loading runtime assets.
  // Refuse initialization if the dependency is missing; never restore a raw-text fallback.
  const projection = window.__ANALYTIX_ORDINARY_PII_PROJECTION__;
  const methods = ["normalizeFieldName", "resolveKind", "projectRestricted", "projectField", "projectDetected"];
  if (!projection || methods.some((method) => typeof projection[method] !== "function")) {
    throw new Error("ordinary PII host projection missing");
  }
})();
