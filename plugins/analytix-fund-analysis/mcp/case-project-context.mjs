import crypto from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

export const CASE_PROJECT_CONTEXT_VERSION = "0.16.17-case-project-context";

const CASE_PROJECT_RELATIVE_PATH = path.join(".analytix", "case-project.json");
const CASE_ID_PATTERN = /^[A-Za-z0-9_-]{4,80}$/u;
const SHA256_PATTERN = /^[a-f0-9]{64}$/u;
const MAX_CASE_BINDING_BYTES = 1024 * 1024;
const CASE_PROJECT_KEYS = new Set(["version", "workspaceRoot", "caseId", "source", "updatedAt"]);

function text(value) {
  return String(value == null ? "" : value).trim();
}

function objectOf(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function isSameOrInside(childPath, parentPath) {
  const child = path.resolve(childPath);
  const parent = path.resolve(parentPath);
  const relative = path.relative(parent, child);
  return relative === "" || (!!relative && !relative.startsWith("..") && !path.isAbsolute(relative));
}

function realPath(value) {
  try {
    return fs.realpathSync(path.resolve(value));
  } catch {
    return "";
  }
}

function runtimeContext(args = {}) {
  const source = objectOf(args);
  return objectOf(source._analytix || source.__analytix || source.analytix_runtime_context);
}

function workspaceCandidates(args = {}, env = process.env) {
  const context = runtimeContext(args);
  return [
    context.workspaceRealPath,
    env.ANALYTIX_CASE_PROJECT_ROOT,
    env.ANALYTIX_WORKSPACE_ROOT
  ].map(text).filter(Boolean);
}

function configCandidates(args = {}, env = process.env) {
  return [
    env.ANALYTIX_CASE_PROJECT_CONFIG
  ].map(text).filter(Boolean);
}

function findCaseProjectConfigFromWorkspace(workspaceRoot) {
  const workspaceRealPath = realPath(workspaceRoot);
  if (!workspaceRealPath) return "";
  const candidate = path.join(workspaceRealPath, CASE_PROJECT_RELATIVE_PATH);
  return fs.existsSync(candidate) ? candidate : "";
}

function readCaseProjectDocument(configPath) {
  try {
    const body = fs.readFileSync(configPath);
    if (!body.length || body.length > MAX_CASE_BINDING_BYTES) return null;
    const parsed = JSON.parse(body.toString("utf8"));
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return null;
    if (Object.keys(parsed).some((key) => !CASE_PROJECT_KEYS.has(key))) return null;
    return { body, parsed };
  } catch {
    return null;
  }
}

function normalizeCaseProjectBinding(document, configPath, expectedWorkspace = "") {
  if (!document) return null;
  const source = objectOf(document.parsed);
  const caseId = text(source.caseId);
  if (source.version !== 1 || !CASE_ID_PATTERN.test(caseId) || text(source.source) !== "analytix-data-analysis") return null;
  const workspaceRoot = realPath(source.workspaceRoot);
  const expectedWorkspaceRealPath = expectedWorkspace ? realPath(expectedWorkspace) : "";
  const configRealPath = realPath(configPath);
  if (!workspaceRoot || !configRealPath || (expectedWorkspace && !expectedWorkspaceRealPath)
    || (expectedWorkspaceRealPath && workspaceRoot !== expectedWorkspaceRealPath)
    || !isSameOrInside(configRealPath, workspaceRoot)) {
    return null;
  }
  const bindingSha256 = crypto.createHash("sha256").update(document.body).digest("hex");
  const caseBindingHash = crypto.createHash("sha256")
    .update(["case-binding-v1", workspaceRoot, caseId, bindingSha256].join("\u0000"))
    .digest("hex");
  return {
    case_id: caseId,
    caseId,
    workspace_root: workspaceRoot,
    workspaceRoot,
    case_project_config: configRealPath,
    binding_sha256: bindingSha256,
    case_binding_hash: caseBindingHash,
    source: "case-project"
  };
}

export function resolveCaseProjectContext(args = {}, env = process.env) {
  const tried = [];
  const context = runtimeContext(args);
  const expectedWorkspace = text(context.workspaceRealPath);
  for (const candidate of configCandidates(args, env)) {
    const configPath = path.resolve(candidate);
    tried.push(configPath);
    const binding = normalizeCaseProjectBinding(readCaseProjectDocument(configPath), configPath, expectedWorkspace);
    if (binding) return { ...binding, tried };
  }
  for (const workspaceRoot of workspaceCandidates(args, env)) {
    const configPath = findCaseProjectConfigFromWorkspace(workspaceRoot);
    if (!configPath) continue;
    tried.push(configPath);
    const binding = normalizeCaseProjectBinding(readCaseProjectDocument(configPath), configPath, workspaceRoot);
    if (binding) return { ...binding, tried };
  }
  return null;
}

function defaultDataAnalysisHomeCandidates(env = process.env) {
  const home = text(env.HOME) || os.homedir();
  const candidates = [
    env.ANALYTIX_DATA_ANALYSIS_DIR,
    env.ANALYTIX_DATA_ANALYSIS_HOME
  ].map(text).filter(Boolean);
  if (process.platform === "darwin" && home) {
    candidates.push(path.join(home, "Library", "Application Support", "analytix", "data-analysis"));
  }
  const localAppData = text(env.LOCALAPPDATA);
  const appData = text(env.APPDATA);
  if (localAppData) candidates.push(path.join(localAppData, "analytix", "data-analysis"));
  if (appData) candidates.push(path.join(appData, "analytix", "data-analysis"));
  if (home) {
    candidates.push(path.join(home, ".analytix", "data-analysis"));
    candidates.push(path.join(home, ".config", "analytix", "data-analysis"));
  }
  return [...new Set(candidates.map((candidate) => path.resolve(candidate)))];
}

export function resolveDataAnalysisCasesRoot(env = process.env, caseId = "") {
  const explicitRoot = text(env.ANALYTIX_DATA_ANALYSIS_CASES_ROOT);
  if (explicitRoot) return path.resolve(explicitRoot);
  const candidates = defaultDataAnalysisHomeCandidates(env).map((root) => path.join(root, "cases"));
  const normalizedCaseId = text(caseId);
  if (normalizedCaseId) {
    const existing = candidates.find((root) => fs.existsSync(path.join(root, normalizedCaseId, "case.duckdb")));
    if (existing) return existing;
  }
  return candidates[0] || path.resolve("cases");
}

export function resolveCaseDuckdbPath(caseId, env = process.env) {
  const normalizedCaseId = text(caseId);
  if (!CASE_ID_PATTERN.test(normalizedCaseId)) {
    throw new Error("Invalid analytix case id for project-scoped DuckDB access.");
  }
  const root = resolveDataAnalysisCasesRoot(env, normalizedCaseId);
  const dbPath = path.resolve(root, normalizedCaseId, "case.duckdb");
  if (!isSameOrInside(dbPath, root)) {
    throw new Error("Resolved analytix case DuckDB path escaped data-analysis cases root.");
  }
  return dbPath;
}

export function stripAnalytixRuntimeContext(args = {}) {
  const next = { ...objectOf(args) };
  delete next._analytix;
  delete next.__analytix;
  delete next.analytix_runtime_context;
  return next;
}

export function explicitCaseIdFromArgs(args = {}) {
  const source = objectOf(args);
  return text(source.case_id || source.caseId);
}

export function assertExplicitCaseMatchesProject(args = {}, projectContext = null) {
  const context = runtimeContext(args);
  const explicitCaseId = explicitCaseIdFromArgs(args);
  const runtimeCaseId = text(context.caseId);
  const runtimeWorkspaceRealPath = text(context.workspaceRealPath);
  const runtimeCaseBindingHash = text(context.caseBindingHash);
  const mismatch = projectContext && (
    (explicitCaseId && explicitCaseId !== projectContext.case_id)
    || (runtimeCaseId && runtimeCaseId !== projectContext.case_id)
    || (runtimeWorkspaceRealPath && realPath(runtimeWorkspaceRealPath) !== projectContext.workspace_root)
    || (runtimeCaseBindingHash && (!SHA256_PATTERN.test(runtimeCaseBindingHash) || runtimeCaseBindingHash !== projectContext.case_binding_hash))
  );
  if (explicitCaseId && runtimeCaseId && explicitCaseId !== runtimeCaseId) {
    const error = new Error(`Explicit case_id ${explicitCaseId} does not match frozen host case ${runtimeCaseId}.`);
    error.code = "INVALID_CASE_SOURCE";
    error.caseId = explicitCaseId;
    error.expectedCaseId = runtimeCaseId;
    error.stage = "frozen_case_scope_guard";
    throw error;
  }
  if (!mismatch) return explicitCaseId || runtimeCaseId || "";
  {
    const observedCaseId = explicitCaseId || runtimeCaseId;
    const error = new Error(`Frozen or explicit case ${observedCaseId} does not match analytix case project ${projectContext.case_id}.`);
    error.code = "INVALID_CASE_SOURCE";
    error.caseId = observedCaseId;
    error.expectedCaseId = projectContext.case_id;
    error.stage = "case_project_scope_guard";
    error.details = {
      explicit_case_id: explicitCaseId,
      frozen_case_id: runtimeCaseId,
      project_case_id: projectContext.case_id,
      workspace_root: projectContext.workspace_root,
      case_project_config: projectContext.case_project_config
    };
    throw error;
  }
}
