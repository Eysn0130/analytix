import path from "node:path";

export const ARTIFACT_PATH_POLICY_VERSION = "1.0.0";

export const DEFAULT_ANALYTIX_ARTIFACT_PARTS = [
  ".analytix",
  "artifacts",
  "analytix-funds"
];

function text(value) {
  return String(value == null ? "" : value).trim();
}

function objectOf(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function isSameOrInside(childPath, parentPath) {
  if (!parentPath) return false;
  const child = path.resolve(childPath);
  const parent = path.resolve(parentPath);
  const relative = path.relative(parent, child);
  return relative === "" || (!relative.startsWith("..") && !path.isAbsolute(relative));
}

export function resolveAnalytixArtifactRoot({ env = process.env, pluginRoot = "" } = {}) {
  const source = objectOf(env);
  const configured = text(source.ANALYTIX_FUNDS_ARTIFACT_DIR);
  const home = text(source.HOME) || text(pluginRoot) || ".";
  return path.resolve(configured || path.join(home, ...DEFAULT_ANALYTIX_ARTIFACT_PARTS));
}

export function assertAnalytixArtifactRoot(artifactRoot, { env = process.env } = {}) {
  const root = path.resolve(text(artifactRoot) || ".");
  const home = text(objectOf(env).HOME);
  const systemCodexHome = home ? path.resolve(home, ".codex") : "";
  if (isSameOrInside(root, systemCodexHome)) {
    throw new Error(`Refusing to use an artifact path inside system Codex home: ${root}`);
  }
  return root;
}
