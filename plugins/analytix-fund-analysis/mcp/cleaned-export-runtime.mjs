import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { dataOf, objectOf, text } from "./runtime-normalizers.mjs";
import { resolveDataAnalysisCasesRoot } from "./case-project-context.mjs";
import { abortError, throwIfAborted } from "./abort-runtime.mjs";

export const CLEANED_EXPORT_RUNTIME_VERSION = "0.15.71-cleaned-export-runtime";

const DEFAULT_TIMEOUT_MS = 110_000;
const MAX_TIMEOUT_MS = 110_000;
const POLL_INTERVAL_MS = 2_000;
const TERMINAL_JOB_STATUSES = new Set(["succeeded", "failed", "canceled"]);
const ALLOWED_EXPORT_TABLES = new Set([
  "fc_coercive_measure",
  "fc_person_contact",
  "fc_person_address",
  "fc_person",
  "fc_sub_account",
  "fc_account",
  "fc_transaction",
  "fc_task_fail",
  "fc_task_success"
]);

function sleep(ms, signal) {
  throwIfAborted(signal);
  return new Promise((resolve, reject) => {
    let settled = false;
    const finish = (operation) => {
      if (settled) return;
      settled = true;
      signal?.removeEventListener("abort", onAbort);
      operation();
    };
    const timer = setTimeout(() => finish(resolve), ms);
    const onAbort = () => {
      clearTimeout(timer);
      finish(() => reject(abortError(signal)));
    };
    signal?.addEventListener("abort", onAbort, { once: true });
    if (signal?.aborted) onAbort();
  });
}

function isSameOrInside(childPath, parentPath) {
  const child = path.resolve(childPath);
  const parent = path.resolve(parentPath);
  const relative = path.relative(parent, child);
  return relative === "" || (!relative.startsWith("..") && !path.isAbsolute(relative));
}

function xlsxArchiveStructurePresent(filePath, sizeBytes) {
  const tailSize = Math.min(sizeBytes, 65_557);
  const tail = Buffer.alloc(tailSize);
  const descriptor = fs.openSync(filePath, "r");
  let bytesRead = 0;
  try {
    bytesRead = fs.readSync(descriptor, tail, 0, tail.length, sizeBytes - tailSize);
  } finally {
    fs.closeSync(descriptor);
  }
  const bytes = tail.subarray(0, bytesRead);
  let endOffset = -1;
  for (let index = bytes.length - 22; index >= 0; index -= 1) {
    if (bytes.readUInt32LE(index) === 0x06054b50) {
      endOffset = index;
      break;
    }
  }
  if (endOffset < 0) return false;
  const entryCount = bytes.readUInt16LE(endOffset + 10);
  const directorySize = bytes.readUInt32LE(endOffset + 12);
  const directoryOffset = bytes.readUInt32LE(endOffset + 16);
  if (!entryCount || !directorySize || directoryOffset + directorySize > sizeBytes || directorySize > 32 * 1024 * 1024) {
    return false;
  }
  const directory = Buffer.alloc(directorySize);
  const directoryDescriptor = fs.openSync(filePath, "r");
  try {
    const directoryBytesRead = fs.readSync(directoryDescriptor, directory, 0, directory.length, directoryOffset);
    if (directoryBytesRead !== directory.length) return false;
  } finally {
    fs.closeSync(directoryDescriptor);
  }
  const entries = new Set();
  let cursor = 0;
  for (let count = 0; count < entryCount && cursor + 46 <= directory.length; count += 1) {
    if (directory.readUInt32LE(cursor) !== 0x02014b50) return false;
    const nameLength = directory.readUInt16LE(cursor + 28);
    const extraLength = directory.readUInt16LE(cursor + 30);
    const commentLength = directory.readUInt16LE(cursor + 32);
    const nextCursor = cursor + 46 + nameLength + extraLength + commentLength;
    if (!nameLength || nextCursor > directory.length) return false;
    entries.add(directory.toString("utf8", cursor + 46, cursor + 46 + nameLength));
    cursor = nextCursor;
  }
  return entries.has("[Content_Types].xml")
    && entries.has("_rels/.rels")
    && entries.has("xl/workbook.xml")
    && [...entries].some((entry) => /^xl\/worksheets\/sheet[^/]*\.xml$/u.test(entry));
}

export function resolveAnalytixAppDataRoot(env = process.env) {
  const source = objectOf(env);
  const explicit = text(source.ANALYTIX_DATA_ANALYSIS_DIR || source.ANALYTIX_DATA_ANALYSIS_HOME);
  if (explicit) return path.resolve(explicit);
  const casesRoot = resolveDataAnalysisCasesRoot(env);
  if (text(casesRoot)) return path.dirname(casesRoot);
  const home = text(source.HOME) || os.homedir();
  return path.resolve(home, ".analytix", "data-analysis");
}

function allowedExportRoots(env = process.env, options = {}) {
  const extraRoots = Array.isArray(options.allowedRoots) ? options.allowedRoots : [];
  return [
    resolveAnalytixAppDataRoot(env),
    resolveDataAnalysisCasesRoot(env),
    ...extraRoots
  ].map(text).filter(Boolean).map((root) => path.resolve(root));
}

export function assertAnalytixOwnedExportPath(outputPath, env = process.env, options = {}) {
  const candidate = path.resolve(text(outputPath) || ".");
  const roots = allowedExportRoots(env, options);
  if (!roots.some((root) => isSameOrInside(candidate, root))) {
    throw new Error(`cleaned export output escaped Analytix-owned app data: ${candidate}`);
  }
  return candidate;
}

function inspectExportFile(filePath) {
  const stat = fs.statSync(filePath);
  const extension = path.extname(filePath).toLowerCase();
  const head = Buffer.alloc(Math.min(4096, Math.max(4, stat.size)));
  const descriptor = fs.openSync(filePath, "r");
  let bytesRead = 0;
  try {
    bytesRead = fs.readSync(descriptor, head, 0, head.length, 0);
  } finally {
    fs.closeSync(descriptor);
  }
  const bytes = head.subarray(0, bytesRead);
  const csvHeader = extension === ".csv"
    ? bytes.toString("utf8").replace(/^\uFEFF/u, "").split(/\r?\n/u)[0].trim()
    : "";
  const signatureOk = extension === ".xlsx"
    ? bytes.length >= 4 && bytes[0] === 0x50 && bytes[1] === 0x4b
    : extension === ".csv"
      ? Boolean(csvHeader)
      : false;
  const workbookStructurePresent = extension === ".xlsx" && signatureOk
    ? xlsxArchiveStructurePresent(filePath, stat.size)
    : false;
  const readableStructure = extension === ".xlsx" ? workbookStructurePresent : signatureOk;
  return {
    file_name: path.basename(filePath),
    format: extension.replace(/^\./u, ""),
    size_bytes: stat.size,
    inspection_status: stat.isFile() && stat.size > 0 && readableStructure ? "passed" : "failed",
    header_present: extension === ".csv" ? Boolean(csvHeader) : undefined,
    workbook_signature_present: extension === ".xlsx" ? signatureOk : undefined,
    workbook_structure_present: extension === ".xlsx" ? workbookStructurePresent : undefined
  };
}

export function inspectCleanedExportDirectory(outputPath, env = process.env, options = {}) {
  const exportDir = assertAnalytixOwnedExportPath(outputPath, env, options);
  const stat = fs.statSync(exportDir);
  if (!stat.isDirectory()) {
    throw new Error("cleaned export output is not a directory");
  }
  const files = fs.readdirSync(exportDir, { withFileTypes: true })
    .filter((entry) => entry.isFile() && /\.(?:csv|xlsx)$/iu.test(entry.name))
    .map((entry) => inspectExportFile(path.join(exportDir, entry.name)));
  if (!files.length) {
    throw new Error("cleaned export completed without readable CSV/XLSX files");
  }
  if (files.some((file) => file.inspection_status !== "passed")) {
    throw new Error("cleaned export contains an empty or unreadable CSV/XLSX file");
  }
  return {
    inspection_status: "passed",
    file_count: files.length,
    files
  };
}

function normalizedTables(value) {
  if (value === undefined || (Array.isArray(value) && value.length === 0)) {
    return ["fc_transaction"];
  }
  if (!Array.isArray(value)) {
    throw new Error("tables must be an array of approved cleaned table names");
  }
  const selected = value.map(text);
  const invalid = selected.filter((item) => !item || !ALLOWED_EXPORT_TABLES.has(item));
  if (invalid.length) {
    throw new Error(`unapproved cleaned export tables: ${invalid.join(", ")}`);
  }
  return [...new Set(selected)];
}

function normalizedOutputName(value) {
  return text(value)
    .replace(/[\\/:*?"<>|\r\n]/gu, "_")
    .replace(/\s+/gu, " ")
    .slice(0, 120);
}

function normalizedTargetDir(args = {}) {
  return text(args.target_dir || args.targetDir || args.output_dir || args.outputDir);
}

function blockedPayload({
  caseId = "",
  code,
  reason,
  job = {},
  nextActions = []
}) {
  const jobData = objectOf(job);
  return {
    tool: "export_cleaned_case_data",
    skill_id: "export_cleaned_case_data",
    case_id: caseId || undefined,
    status: "blocked",
    error_code: code,
    answer_card: {
      card_type: "cleaned_export_delivery_card",
      intent: "cleaned_detail_export",
      fact_source: "analytix_cleaned_export_workflow",
      source_scope: ["当前案件清洗后数据"],
      warnings: [reason]
    },
    key_facts: {
      cleaned_export: {
        job_status: text(jobData.status) || "not_started",
        progress: Number(jobData.progress || 0),
        delivery_status: "blocked",
        inspection_status: "not_passed",
        reason
      }
    },
    warnings: [reason],
    next_actions: nextActions
  };
}

function pendingPayload({ caseId, job, selectedTables, exportFormat }) {
  const jobData = objectOf(job);
  return {
    tool: "export_cleaned_case_data",
    skill_id: "export_cleaned_case_data",
    case_id: caseId,
    status: "pending",
    answer_card: {
      card_type: "cleaned_export_delivery_card",
      intent: "cleaned_detail_export",
      fact_source: "analytix_cleaned_export_workflow",
      source_scope: ["当前案件清洗后数据"]
    },
    key_facts: {
      cleaned_export: {
        job_id: text(jobData.job_id),
        job_status: text(jobData.status) || "running",
        progress: Number(jobData.progress || 0),
        export_format: exportFormat,
        selected_tables: selectedTables,
        delivery_status: "pending",
        inspection_status: "not_started"
      }
    },
    warnings: ["清洗明细导出任务仍在执行，尚未形成可交付文件。"],
    next_actions: ["使用同一任务编号继续检查导出状态；文件读取检查通过前不得宣称已交付。"],
    export_job: jobData
  };
}

function deliveredPayload({ caseId, job, selectedTables, exportFormat, inspection }) {
  const jobData = objectOf(job);
  return {
    tool: "export_cleaned_case_data",
    skill_id: "export_cleaned_case_data",
    case_id: caseId,
    status: "ok",
    answer_card: {
      card_type: "cleaned_export_delivery_card",
      intent: "cleaned_detail_export",
      fact_source: "analytix_cleaned_export_workflow",
      source_scope: ["当前案件清洗后数据"],
      facts: [{
        export_format: exportFormat,
        selected_tables: selectedTables,
        file_count: inspection.file_count,
        files: inspection.files
      }]
    },
    key_facts: {
      cleaned_export: {
        job_status: text(jobData.status),
        progress: Number(jobData.progress || 0),
        export_format: exportFormat,
        selected_tables: selectedTables,
        delivery_status: "delivered",
        delivery_location: "Analytix 当前案件 exports 目录",
        inspection_status: inspection.inspection_status,
        file_count: inspection.file_count,
        files: inspection.files
      }
    },
    artifact_provenance: {
      output_path: text(jobData.output_path),
      analytix_owned: true,
      inspection_status: inspection.inspection_status
    },
    next_actions: ["后续复核意见或研判列应另建证据表，不得改写清洗导出原字段。"]
  };
}

export function createCleanedExportRuntime({
  resolveCase,
  httpJson,
  httpPostData,
  env = process.env,
  sleepFn = sleep,
  pollIntervalMs = POLL_INTERVAL_MS
}) {
  async function pollJob(jobId, timeoutMs, signal) {
    throwIfAborted(signal);
    const deadline = Date.now() + timeoutMs;
    let job = dataOf(await httpJson(`/export/jobs/${encodeURIComponent(jobId)}`, { signal }));
    while (!TERMINAL_JOB_STATUSES.has(text(job.status).toLowerCase()) && Date.now() < deadline) {
      await sleepFn(pollIntervalMs, signal);
      throwIfAborted(signal);
      job = dataOf(await httpJson(`/export/jobs/${encodeURIComponent(jobId)}`, { signal }));
    }
    return job;
  }

  async function exportCleanedCaseData(args = {}, { signal } = {}) {
    throwIfAborted(signal);
    if (!text(args.purpose)) {
      return blockedPayload({
        code: "CLEANED_EXPORT_PURPOSE_REQUIRED",
        reason: "创建或继续清洗明细导出前，必须说明本次交付用途。"
      });
    }
    const resolved = await resolveCase(args, { signal });
    const caseId = resolved.case_id;
    const exportFormat = text(args.export_format).toLowerCase() === "csv" ? "csv" : "xlsx";
    const targetDir = normalizedTargetDir(args);
    let selectedTables;
    try {
      selectedTables = normalizedTables(args.tables);
    } catch (error) {
      return blockedPayload({
        caseId,
        code: "CLEANED_EXPORT_TABLES_INVALID",
        reason: `清洗明细导出未创建：${text(error?.message || error)}`
      });
    }
    const timeoutValue = Number(args.timeout_ms || DEFAULT_TIMEOUT_MS);
    const timeoutMs = Math.max(1_000, Math.min(Number.isFinite(timeoutValue) ? timeoutValue : DEFAULT_TIMEOUT_MS, MAX_TIMEOUT_MS));
    let job = {};

    try {
      if (text(args.job_id)) {
        job = dataOf(await httpJson(`/export/jobs/${encodeURIComponent(text(args.job_id))}`, { signal }));
        if (text(job.case_id) !== caseId) {
          return blockedPayload({
            caseId,
            code: "CLEANED_EXPORT_CASE_MISMATCH",
            reason: "该导出任务不属于当前案件，不能继续读取或交付。"
          });
        }
      } else {
        if (args.confirm_export !== true) {
          return blockedPayload({
            caseId,
            code: "CLEANED_EXPORT_CONFIRMATION_REQUIRED",
            reason: "创建清洗明细导出前需要明确确认导出操作。",
            nextActions: ["用户明确要求导出后，以 confirm_export=true 创建当前案件清洗明细任务。"]
          });
        }
        job = await httpPostData("/export/cleaned", {
          case_id: caseId,
          export_format: exportFormat,
          filters: { tables: selectedTables },
          output_name: normalizedOutputName(args.output_name),
          target_dir: targetDir || null
        }, { signal });
      }

      if (args.wait_for_completion === false) {
        return pendingPayload({ caseId, job, selectedTables, exportFormat });
      }
      job = await pollJob(text(job.job_id), timeoutMs, signal);
    } catch (error) {
      throwIfAborted(signal);
      const backendError = objectOf(objectOf(error?.payload).error);
      const backendCode = text(backendError.code);
      const reason = text(backendError.message || error?.message || error);
      return blockedPayload({
        caseId,
        code: backendCode || "CLEANED_EXPORT_BACKEND_UNAVAILABLE",
        reason: `清洗明细导出未创建或无法继续检查：${reason || "导出服务不可用"}`,
        nextActions: backendCode === "DATASET_NOT_READY"
          ? ["先完成当前案件清洗任务，再重新创建清洗明细导出。"]
          : ["恢复当前案件导出服务后，按同一用途重试；不得把失败任务说成已交付。"]
      });
    }
    const status = text(job.status).toLowerCase();
    if (!TERMINAL_JOB_STATUSES.has(status)) {
      return pendingPayload({ caseId, job, selectedTables, exportFormat });
    }
    if (status !== "succeeded") {
      return blockedPayload({
        caseId,
        code: "CLEANED_EXPORT_JOB_FAILED",
        reason: text(job.error) || `清洗明细导出任务状态为 ${status}，未形成可交付文件。`,
        job,
        nextActions: ["先恢复清洗数据集或导出服务，再重新创建清洗明细导出任务。"]
      });
    }

    try {
      throwIfAborted(signal);
      const inspection = inspectCleanedExportDirectory(text(job.output_path), env, {
        allowedRoots: targetDir ? [targetDir] : []
      });
      return deliveredPayload({ caseId, job, selectedTables, exportFormat, inspection });
    } catch (error) {
      throwIfAborted(signal);
      return blockedPayload({
        caseId,
        code: "CLEANED_EXPORT_INSPECTION_FAILED",
        reason: `清洗明细导出文件读取检查未通过：${text(error?.message || error)}`,
        job,
        nextActions: ["重新检查当前案件 exports 目录中的导出文件；读取检查通过前不得宣称已交付。"]
      });
    }
  }

  return { exportCleanedCaseData };
}
