import fs from "node:fs";
import path from "node:path";
import {
  dataOf,
  text
} from "./runtime-normalizers.mjs";
import {
  assertExplicitCaseMatchesProject,
  explicitCaseIdFromArgs,
  resolveCaseDuckdbPath,
  resolveCaseProjectContext,
  stripAnalytixRuntimeContext
} from "./case-project-context.mjs";
import {
  executeLocalDuckdbWorkbenchSkill,
  supportsLocalDuckdbWorkbenchSkill
} from "./duckdb-workbench-runtime.mjs";
import { throwIfAborted } from "./abort-runtime.mjs";

export const BACKEND_API_CLIENT_VERSION = "0.16.1";
export const DEFAULT_BACKEND_BASE_URL = "http://127.0.0.1:18731";
const ACTIVE_CASE_API_SOURCE = "active-case-api";

export function queryString(params) {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(params || {})) {
    if (value === undefined || value === null || value === "") {
      continue;
    }
    query.set(key, String(value));
  }
  return query.toString();
}

export function normalizeBaseUrl(value, fallback = DEFAULT_BACKEND_BASE_URL) {
  return text(value).replace(/\/+$/u, "") || fallback;
}

export class CaseSourceError extends Error {
  constructor({
    code = "CASE_SOURCE_BLOCKER",
    message = "",
    caseId = "",
    stage = "",
    details = {},
    cause = null
  } = {}) {
    super(message || "Analytix case source is not available.");
    this.name = "CaseSourceError";
    this.code = code;
    this.caseId = text(caseId);
    this.stage = text(stage);
    this.details = details && typeof details === "object" ? details : {};
    if (cause) this.cause = cause;
  }
}

function localFallbackCaseId(args = {}, env = {}) {
  try {
    const projectContext = resolveCaseProjectContext(args, env);
    if (!projectContext) return "";
    assertExplicitCaseMatchesProject(args, projectContext);
    return projectContext.case_id;
  } catch {
    return "";
  }
}

function shouldUseLocalDuckdbFallback(error, env = {}, args = {}) {
  const message = text(error?.message || error);
  const code = backendErrorCode(error);
  if (/^(1|true|yes)$/iu.test(text(env.ANALYTIX_FUNDS_DISABLE_LOCAL_DUCKDB_WORKBENCH))) {
    return false;
  }
  if (code === "INVALID_ARGUMENT" && /Structured SQL parser is unavailable|controlled case SQL fails closed/iu.test(message)) {
    return true;
  }
  const hasExplicitLocalCase = Boolean(localFallbackCaseId(args, env));
  return hasExplicitLocalCase && (
    /fetch failed|ECONNREFUSED|ECONNRESET|ETIMEDOUT|backend case API|case project|Failed to fetch|network error/iu.test(message) ||
    ["SKILL_NOT_FOUND", "NOT_IMPLEMENTED", "CAPABILITY_UNAVAILABLE"].includes(code)
  );
}

export function isCaseSourceError(error) {
  return error instanceof CaseSourceError || text(error?.name) === "CaseSourceError";
}

function backendErrorCode(error) {
  const payload = error?.payload;
  if (!payload || typeof payload !== "object") return "";
  const source = payload.error && typeof payload.error === "object" ? payload.error : {};
  return text(source.code);
}

function localProjectCaseRecord(projectContext, env = {}) {
  const caseId = text(projectContext?.case_id);
  const workspaceRoot = text(projectContext?.workspace_root);
  let duckdbPath = "";
  let duckdbReady = false;
  try {
    duckdbPath = resolveCaseDuckdbPath(caseId, env);
    duckdbReady = fs.existsSync(duckdbPath);
  } catch {
    duckdbPath = "";
  }
  const workspaceName = workspaceRoot ? path.basename(workspaceRoot) : "";
  return {
    id: caseId,
    case_id: caseId,
    caseId,
    name: workspaceName || caseId,
    case_name: workspaceName || caseId,
    status: duckdbReady ? "ready" : "duckdb_missing",
    source: "case-project",
    workspace_root: workspaceRoot,
    case_project_config: text(projectContext?.case_project_config),
    duckdb_ready: duckdbReady,
    duckdb_path: duckdbPath
  };
}

export function createBackendApiClient(options = {}) {
  const baseUrl = normalizeBaseUrl(options.baseUrl || DEFAULT_BACKEND_BASE_URL);
  const apiToken = text(options.apiToken);
  const env = options.env || {};

  function apiUrl(pathname) {
    const pathnameText = text(pathname);
    const path = pathnameText.startsWith("/") ? pathnameText : `/${pathnameText}`;
    if (baseUrl.endsWith("/api/v1")) {
      return `${baseUrl}${path}`;
    }
    if (baseUrl.endsWith("/api")) {
      return `${baseUrl}/v1${path}`;
    }
    return `${baseUrl}/api/v1${path}`;
  }

  function headers(extra = {}) {
    const output = {
      accept: "application/json",
      ...extra
    };
    if (apiToken) {
      output.authorization = `Bearer ${apiToken}`;
    }
    return output;
  }

  async function httpJson(pathname, fetchOptions = {}) {
    const response = await fetch(apiUrl(pathname), {
      ...fetchOptions,
      headers: headers(fetchOptions.headers || {})
    });
    const bodyText = await response.text();
    let payload = null;
    if (bodyText) {
      try {
        payload = JSON.parse(bodyText);
      } catch (error) {
        payload = { raw: bodyText };
      }
    }
    if (!response.ok) {
      const message =
        payload?.message ||
        (payload?.error && typeof payload.error === "object" ? payload.error.message : "") ||
        (typeof payload?.error === "string" ? payload.error : "") ||
        `Analytix API request failed: HTTP ${response.status}`;
      const error = new Error(message);
      error.status = response.status;
      error.payload = payload;
      throw error;
    }
    return payload || {};
  }

  async function resolveCase(args = {}, { signal } = {}) {
    throwIfAborted(signal);
    const projectContext = resolveCaseProjectContext(args, env);
    const explicitCaseId = explicitCaseIdFromArgs(args);
    try {
      assertExplicitCaseMatchesProject(args, projectContext);
    } catch (error) {
      throw new CaseSourceError({
        code: "INVALID_CASE_SOURCE",
        message: text(error?.message || error),
        caseId: explicitCaseId,
        stage: "case_project_scope_guard",
        details: error?.details || {},
        cause: error
      });
    }

    if (!projectContext) {
      throw new CaseSourceError({
        code: "CASE_SOURCE_BLOCKER",
        message: "Unable to resolve analytix case project context. Open the tool from an analytix case project workspace.",
        stage: "case_project_context",
        details: {
          required_binding: ".analytix/case-project.json",
          runtime_context: "_analytix.workspaceRealPath"
        }
      });
    }

    const caseId = projectContext.case_id;
    const localCase = localProjectCaseRecord(projectContext, env);
    try {
      const detail = await httpJson(`/cases/${encodeURIComponent(caseId)}`, { signal });
      throwIfAborted(signal);
      return {
        case_id: caseId,
        source: projectContext.source,
        backend_case_api: ACTIVE_CASE_API_SOURCE,
        case: {
          ...localCase,
          ...(detail.data || detail || {}),
          source: "case-project",
          duckdb_ready: localCase.duckdb_ready,
          duckdb_path: localCase.duckdb_path
        },
        case_project: projectContext
      };
    } catch (error) {
      throwIfAborted(signal);
      const message = error instanceof Error ? error.message : String(error || "");
      if (localCase.duckdb_ready) {
        return {
          case_id: caseId,
          source: projectContext.source,
          case: {
            ...localCase,
            backend_status: "unavailable",
            backend_message: message
          },
          case_project: projectContext,
          backend_case_api: ACTIVE_CASE_API_SOURCE,
          backend_validation_error: {
            status: Number(error?.status || 0) || undefined,
            code: backendErrorCode(error) || undefined,
            message
          }
        };
      }
      if (Number(error?.status || 0) === 404 || backendErrorCode(error) === "CASE_NOT_FOUND") {
        throw new CaseSourceError({
          code: "INVALID_CASE_SOURCE",
          message: `Analytix case project case_id does not exist in backend: ${caseId}`,
          caseId,
          stage: "case_project_lookup",
          details: {
            backend_case_api: ACTIVE_CASE_API_SOURCE,
            backend_status: Number(error?.status || 0) || undefined,
            backend_error_code: backendErrorCode(error) || undefined,
            backend_message: message,
            workspace_root: projectContext.workspace_root,
            case_project_config: projectContext.case_project_config
          },
          cause: error
        });
      }
      throw new CaseSourceError({
        code: "CASE_SOURCE_BLOCKER",
        message: `Unable to validate analytix case project via backend case API: ${message}`,
        caseId,
        stage: "case_project_lookup",
        details: {
          backend_case_api: ACTIVE_CASE_API_SOURCE,
          backend_status: Number(error?.status || 0) || undefined,
          backend_error_code: backendErrorCode(error) || undefined,
          backend_message: message,
          workspace_root: projectContext.workspace_root,
          case_project_config: projectContext.case_project_config
        },
        cause: error
      });
    }
  }

  async function executeSkill(skillId, args = {}, { signal } = {}) {
    throwIfAborted(signal);
    let resolved;
    try {
      resolved = await resolveCase(args, { signal });
    } catch (error) {
      throwIfAborted(signal);
      if (supportsLocalDuckdbWorkbenchSkill(skillId) && shouldUseLocalDuckdbFallback(error, env, args)) {
        const caseId = localFallbackCaseId(args, env);
        return {
          data: await executeLocalDuckdbWorkbenchSkill(skillId, {
            ...args,
            case_id: caseId
          }, { env, signal })
        };
      }
      throw error;
    }
    const input = {
      ...stripAnalytixRuntimeContext(args),
      case_id: resolved.case_id
    };
    const localInput = {
      ...args,
      case_id: resolved.case_id
    };
    if (supportsLocalDuckdbWorkbenchSkill(skillId) && resolved.case?.duckdb_ready === true) {
      return {
        data: await executeLocalDuckdbWorkbenchSkill(skillId, localInput, { env, signal })
      };
    }
    if (skillId === "get_account_stats") {
      if (!Array.isArray(input.account_keys) || input.account_keys.length === 0) {
        const aliases = [
          input.account_key,
          input.accountKey,
          input.account,
          input.card_no,
          input.cardNo,
          input.acct_no,
          input.acctNo
        ].map(text).filter(Boolean);
        if (aliases.length > 0) {
          input.account_keys = aliases;
        }
      }
      delete input.account_key;
      delete input.accountKey;
      delete input.account;
      delete input.card_no;
      delete input.cardNo;
      delete input.acct_no;
      delete input.acctNo;
      input.date_start = text(input.date_start);
      input.date_end = text(input.date_end);
      input.direction_mode = text(input.direction_mode) || "both";
      input.success_filter = text(input.success_filter) || "all";
      input.cash_filter = text(input.cash_filter) || "all";
      input.metric_mode = text(input.metric_mode) || "full";
    }
    if (skillId === "run_full_case_analysis" && input.write_report !== true) {
      input.write_report = false;
    }
    try {
      const response = await httpJson(`/skills/${encodeURIComponent(skillId)}:execute`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ input }),
        signal
      });
      throwIfAborted(signal);
      return response;
    } catch (error) {
      throwIfAborted(signal);
      if (skillId !== "run_full_case_analysis" && supportsLocalDuckdbWorkbenchSkill(skillId) && shouldUseLocalDuckdbFallback(error, env, localInput)) {
        return {
          data: await executeLocalDuckdbWorkbenchSkill(skillId, localInput, { env, signal })
        };
      }
      throw error;
    }
  }

  async function httpPostData(pathname, input, { signal } = {}) {
    return dataOf(
      await httpJson(pathname, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(input || {}),
        signal
      })
    );
  }

  return {
    baseUrl,
    executeSkill,
    httpJson,
    httpPostData,
    queryString,
    resolveCase
  };
}
