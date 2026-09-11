import type { CleaningProcessNode } from "../components/CleaningProcessFlow";
import type {
  CleaningJobDTO,
  CleaningStepDetailDTO,
  CleaningStepSummaryDTO
} from "../../../services/cleaning/api";
import type { CaseDetailDTO } from "../../cases/api";
import { STEP_CATALOG } from "./constants";
import type { CleaningFileStats } from "./file-stats";
import { formatCount, formatDateTime, formatNodeCount } from "./formatters";
import { canonicalNonnegativeCount, reconcileEquivalentCounts } from "./known-metrics";
import { getCleaningFileStage, getKindClass } from "./status";
import type {
  CleaningAvailability,
  CleaningBoardStatus,
  ExportState,
  StepCatalogItem
} from "./types";

export interface CleaningExportReadinessViewModel {
  ready: boolean;
  pendingCount: number;
  buttonTitle: string;
  hint: string;
}

export interface CleaningStepViewModel extends StepCatalogItem {
  affectedRows: number | null;
  affectedRowsLabel: string;
  hit: boolean;
  indexLabel: string;
  kindClassName: string;
  stateClassName: string;
  stateLabel: string;
}

export interface CleaningStepImpactViewModel {
  txnStepAffected: number | null;
  accountStepAffected: number | null;
  stepHits: number | null;
  stepInsightMeta: string;
}

export interface CleaningStepDetailFooterViewModel {
  label: string;
  loadedCount: number | null;
  loadError: boolean;
  totalCount: number | null;
  partial: boolean;
}

export type CleaningSummaryCardKey = "files" | "scope" | "transaction" | "account" | "output";
export type CleaningSummaryCardIcon = "file" | "scope" | "transaction" | "account" | "output";

export interface CleaningSummaryCardViewModel {
  key: CleaningSummaryCardKey;
  icon: CleaningSummaryCardIcon;
  className: string;
  label: string;
  value: string;
  meta: string;
}

const FUNDS_FILE_KINDS = ["fc_account", "fc_transaction", "fc_sub_account", "fc_coercive_measure"];
const PERSON_FILE_KINDS = ["fc_person", "fc_person_address", "fc_person_contact"];

function countFilesByKind(fileStats: CleaningFileStats, kinds: string[]): number {
  return kinds.reduce((sum, kind) => sum + (fileStats.byKind[kind]?.length || 0), 0);
}

function formatFileKindCount(fileStats: CleaningFileStats, kind: string): string {
  return formatNodeCount(countFilesByKind(fileStats, [kind]));
}

function formatImportedRowsByKind(fileStats: CleaningFileStats, kind: string): string {
  return formatNodeCount(fileStats.importedRowsByKind[kind] ?? null);
}

function getVisibleRowCount(
  caseRowCount: number | null | undefined,
  importedRowCount: number | null | undefined
): number | null {
  return reconcileEquivalentCounts(
    canonicalNonnegativeCount(caseRowCount),
    canonicalNonnegativeCount(importedRowCount)
  );
}

export function buildCleanedExportReadinessViewModel(
  cleaningAvailability: CleaningAvailability,
  currentJob: CleaningJobDTO | null,
  latestSucceededJob: CleaningJobDTO | null = null
): CleaningExportReadinessViewModel {
  let pendingCount = 0;
  cleaningAvailability.scopeFiles.forEach((item) => {
    if (getCleaningFileStage(item) !== "done") {
      pendingCount += 1;
    }
  });
  const hasVerifiedTransactionData = cleaningAvailability.hasTransactionData === true;
  const transactionDataUnknown = cleaningAvailability.hasTransactionData === null;
  const scopeReady =
    hasVerifiedTransactionData &&
    cleaningAvailability.scopeFiles.length > 0 &&
    pendingCount === 0;
  const succeededOutputReady =
    Boolean(latestSucceededJob) &&
    hasVerifiedTransactionData &&
    cleaningAvailability.scopeRows !== null &&
    cleaningAvailability.scopeRows > 0 &&
    pendingCount === 0;
  const ready = !currentJob && (scopeReady || succeededOutputReady);

  let buttonTitle = "请先完成数据清洗，再导出已清洗数据";
  if (currentJob) {
    buttonTitle = "当前清洗任务仍在执行，请等待清洗完成后再导出已清洗数据";
  } else if (ready) {
    buttonTitle = "导出当前案件已完成清洗的数据";
  } else if (transactionDataUnknown) {
    buttonTitle = "当前案件交易流水数量尚未验证，暂不能导出已清洗数据";
  } else if (cleaningAvailability.hasTransactionData === false) {
    buttonTitle = "当前已验证清洗范围为 0 行（不代表全案无交易），暂不能导出";
  } else if (pendingCount > 0) {
    buttonTitle = `当前还有 ${pendingCount} 个清洗范围文件未完成，暂不能导出已清洗数据`;
  } else if (latestSucceededJob && cleaningAvailability.scopeRows !== null && cleaningAvailability.scopeRows > 0) {
    buttonTitle = "清洗结果已存在，正在同步清洗范围状态";
  } else if (cleaningAvailability.scopeFiles.length === 0) {
    buttonTitle = "当前清洗范围为空，暂不能导出已清洗数据";
  }

  let hint = "请先完成数据清洗，再导出已清洗数据。";
  if (currentJob) {
    hint = "当前清洗或重新清洗正在执行，已清洗数据导出会在任务完成后恢复可用。";
  } else if (ready) {
    hint = "已清洗数据导出已就绪，将按当前清洗范围导出最新结果。";
  } else if (transactionDataUnknown) {
    hint = "交易流水数量尚未通过完整数据边界验证，请刷新或补齐数据后再导出。";
  } else if (cleaningAvailability.hasTransactionData === false) {
    hint = "当前已验证清洗范围为 0 行；该结果不等于全案不存在交易，请核对导入范围。";
  } else if (pendingCount > 0) {
    hint = `已清洗数据导出需等待清洗完成；当前还有 ${pendingCount} 个范围文件未完成。`;
  } else if (latestSucceededJob && cleaningAvailability.scopeRows !== null && cleaningAvailability.scopeRows > 0) {
    hint = "清洗结果已存在，正在同步清洗范围状态；如需导出请稍后刷新。";
  } else if (cleaningAvailability.scopeFiles.length === 0) {
    hint = "当前清洗范围为空，请先补齐可清洗数据后再导出。";
  }

  return {
    ready,
    pendingCount,
    buttonTitle,
    hint
  };
}

export function buildCleaningSummaryCards({
  accountStepAffected,
  caseDetail,
  cleanedOutputReady,
  cleaningAvailability,
  currentJob,
  fileStats,
  latestSucceededJob,
  txnStepAffected
}: {
  accountStepAffected: number | null;
  caseDetail: CaseDetailDTO | null;
  cleanedOutputReady: boolean;
  cleaningAvailability: CleaningAvailability;
  currentJob: CleaningJobDTO | null;
  fileStats: CleaningFileStats;
  latestSucceededJob: CleaningJobDTO | null;
  txnStepAffected: number | null;
}): CleaningSummaryCardViewModel[] {
  const transactionRows = getVisibleRowCount(caseDetail?.stats.tx, fileStats.transactionImportedRows);
  const accountRows = getVisibleRowCount(caseDetail?.stats.accounts, fileStats.accountImportedRows);
  const outputRows = latestSucceededJob?.cleaned_rows ?? null;
  return [
    {
      key: "files",
      icon: "file",
      className: "is-files",
      label: "清洗文件",
      value: formatCount(fileStats.all.length),
      meta: `${fileStats.done.length} 个已完成清洗映射`
    },
    {
      key: "scope",
      icon: "scope",
      className: "is-pending",
      label: "清洗范围",
      value: formatCount(cleaningAvailability.scopeRows),
      meta: currentJob ? `${currentJob.progress}% 执行中` : `${formatCount(cleaningAvailability.scopeFiles.length)} 个范围文件`
    },
    {
      key: "transaction",
      icon: "transaction",
      className: "is-transaction",
      label: "交易流水",
      value: formatCount(transactionRows),
      meta: txnStepAffected === null ? "交易步骤影响行数待证据验证" : `交易步骤影响 ${formatCount(txnStepAffected)} 行`
    },
    {
      key: "account",
      icon: "account",
      className: "is-account",
      label: "账户信息",
      value: formatCount(accountRows),
      meta: accountStepAffected === null ? "账户步骤影响行数待证据验证" : `账户步骤影响 ${formatCount(accountStepAffected)} 行`
    },
    {
      key: "output",
      icon: "output",
      className: "is-output",
      label: "最新成品",
      value: formatCount(outputRows),
      meta: latestSucceededJob
        ? `完成于 ${formatDateTime(latestSucceededJob.updated_at)}，行数待证据验证`
        : cleanedOutputReady
        ? "已清洗数据就绪"
        : "尚无成功任务"
    }
  ];
}

export function buildCleaningStepViewModels(stepSummaries: CleaningStepSummaryDTO[]): CleaningStepViewModel[] {
  const stepSummaryById = new Map<number, CleaningStepSummaryDTO>();
  stepSummaries.forEach((item) => {
    stepSummaryById.set(item.step, item);
  });
  return STEP_CATALOG.map((item) => {
    const summary = stepSummaryById.get(item.step);
    const affectedRows = summary?.affected_rows ?? null;
    const hit = false;
    const kind = summary?.kind || item.kind;
    return {
      ...item,
      kind,
      description: summary?.description || item.description,
      affectedRows,
      affectedRowsLabel: "--",
      hit,
      indexLabel: `STEP ${item.step}`,
      kindClassName: getKindClass(kind),
      stateClassName: hit ? "is-hit" : "",
      stateLabel: "待证据"
    };
  });
}

export function buildCleaningStepImpactViewModel(enrichedSteps: CleaningStepViewModel[]): CleaningStepImpactViewModel {
  return {
    accountStepAffected: null,
    stepHits: null,
    txnStepAffected: null,
    stepInsightMeta: `${formatCount(enrichedSteps.length)} 个步骤的案件命中数待宿主证据验证`
  };
}

export function buildCleaningStepDetailFooterViewModel(
  detailData: CleaningStepDetailDTO | null,
  detailError = ""
): CleaningStepDetailFooterViewModel | null {
  if (!detailData) {
    return null;
  }
  return {
    label: detailError.trim() || "原始明细需同案证据回执和受控披露授权，普通界面不发布",
    loadedCount: null,
    loadError: Boolean(detailError.trim()),
    totalCount: null,
    partial: false
  };
}

export function getCleaningBoardStatus(
  currentJob: CleaningJobDTO | null,
  latestJob: CleaningJobDTO | null,
  latestSucceededJob: CleaningJobDTO | null,
  cleanedOutputReady = false
): CleaningBoardStatus {
  if (currentJob) {
    return "running";
  }
  if (latestJob?.status === "failed" || latestJob?.status === "canceled") {
    return "failed";
  }
  if (latestSucceededJob || cleanedOutputReady) {
    return "done";
  }
  return "idle";
}

export function getCleaningBoardStatusLabel({
  currentJob,
  latestJob,
  latestSucceededJob,
  cleanedOutputReady = false,
  contextLoading,
  jobsLoading
}: {
  currentJob: CleaningJobDTO | null;
  latestJob: CleaningJobDTO | null;
  latestSucceededJob: CleaningJobDTO | null;
  cleanedOutputReady?: boolean;
  contextLoading: boolean;
  jobsLoading: boolean;
}): string {
  if (currentJob) {
    return `清洗执行中 · ${currentJob.progress}%`;
  }
  if (latestJob?.status === "failed") {
    return "最近一次执行失败";
  }
  if (latestJob?.status === "canceled") {
    return "最近一次执行已取消";
  }
  if (latestSucceededJob) {
    return `最近成功于 ${formatDateTime(latestSucceededJob.updated_at)}`;
  }
  if (cleanedOutputReady) {
    return "清洗结果已就绪";
  }
  if (contextLoading || jobsLoading) {
    return "正在同步上下文";
  }
  return "等待开始清洗";
}

export function buildCleaningProcessNodes({
  fileStats,
  caseDetail,
  caseName,
  cleanedOutputReady,
  currentJob,
  latestSucceededJob,
  cleaningAvailability,
  orderedJobCount,
  wsStatus
}: {
  fileStats: CleaningFileStats;
  caseDetail: CaseDetailDTO | null;
  caseName: string;
  cleanedOutputReady: boolean;
  currentJob: CleaningJobDTO | null;
  latestSucceededJob: CleaningJobDTO | null;
  cleaningAvailability: CleaningAvailability;
  orderedJobCount: number;
  wsStatus: string;
}): CleaningProcessNode[] {
  const totalFiles = formatNodeCount(fileStats.all.length);
  const fundsFiles = formatNodeCount(countFilesByKind(fileStats, FUNDS_FILE_KINDS));
  const personFiles = formatNodeCount(countFilesByKind(fileStats, PERSON_FILE_KINDS));
  const totalRows = formatNodeCount(fileStats.importedRows);
  const accountRows = getVisibleRowCount(caseDetail?.stats.accounts, fileStats.accountImportedRows);
  const transactionRows = getVisibleRowCount(caseDetail?.stats.tx, fileStats.transactionImportedRows);
  const personRowCount = getVisibleRowCount(caseDetail?.stats.persons, fileStats.importedRowsByKind.fc_person);
  const personRows = formatNodeCount(personRowCount);
  return [
    {
      id: "case",
      kind: "compact",
      eyebrow: "案件",
      title: caseName || "未命名案件",
      status: currentJob ? "清洗中" : cleanedOutputReady || latestSucceededJob ? "已清洗" : "待清洗",
      accent: "#5b77ff",
      metrics: [
        { label: "文件", value: totalFiles },
        { label: "范围", value: formatNodeCount(cleaningAvailability.scopeRows) }
      ]
    },
    {
      id: "funds",
      kind: "compact",
      eyebrow: "库",
      title: "资金数据库",
      caption: "fc_{account,transaction,sub_account,coercive_measure}",
      status: wsStatus === "open" ? "主链路" : "同步中",
      accent: "#5f7fff",
      metrics: [
        { label: "表", value: "4" },
        { label: "文件", value: fundsFiles }
      ]
    },
    {
      id: "people",
      kind: "compact",
      eyebrow: "库",
      title: "人员数据库",
      caption: "fc_person{,_address,_contact}",
      status: personRowCount === null ? "待验证" : personRowCount > 0 ? "已接入" : "待接入",
      accent: "#00a7c4",
      metrics: [
        { label: "表", value: "3" },
        { label: "文件", value: personFiles }
      ]
    },
    {
      id: "materials",
      kind: "compact",
      eyebrow: "库",
      title: "案件资料库",
      caption: "import_file_log",
      status: orderedJobCount ? "联动中" : "待接入",
      accent: "#ff9a57",
      metrics: [
        { label: "任务", value: formatNodeCount(caseDetail?.stats.tasks) },
        { label: "文件", value: totalFiles }
      ]
    },
    {
      id: "table-account",
      kind: "table",
      eyebrow: "",
      title: "账户信息",
      caption: "fc_account",
      status: "资金",
      accent: "#7b90ff",
      metrics: [
        { label: "文件", value: formatFileKindCount(fileStats, "fc_account") },
        { label: "行", value: formatNodeCount(accountRows) }
      ]
    },
    {
      id: "table-transaction",
      kind: "table",
      eyebrow: "",
      title: "交易明细",
      caption: "fc_transaction",
      status: "资金",
      accent: "#7b90ff",
      metrics: [
        { label: "文件", value: formatFileKindCount(fileStats, "fc_transaction") },
        { label: "行", value: formatNodeCount(transactionRows) }
      ]
    },
    {
      id: "table-sub-account",
      kind: "table",
      eyebrow: "",
      title: "关联子账户",
      caption: "fc_sub_account",
      status: "资金",
      accent: "#7b90ff",
      metrics: [
        { label: "文件", value: formatFileKindCount(fileStats, "fc_sub_account") },
        { label: "行", value: formatImportedRowsByKind(fileStats, "fc_sub_account") }
      ]
    },
    {
      id: "table-coercive",
      kind: "table",
      eyebrow: "",
      title: "强制措施",
      caption: "fc_coercive_measure",
      status: "资金",
      accent: "#7b90ff",
      metrics: [
        { label: "文件", value: formatFileKindCount(fileStats, "fc_coercive_measure") },
        { label: "行", value: formatImportedRowsByKind(fileStats, "fc_coercive_measure") }
      ]
    },
    {
      id: "table-person",
      kind: "table",
      eyebrow: "",
      title: "人员信息",
      caption: "fc_person",
      status: "人员",
      accent: "#2acfb4",
      metrics: [
        { label: "文件", value: formatFileKindCount(fileStats, "fc_person") },
        { label: "行", value: personRows }
      ]
    },
    {
      id: "table-address",
      kind: "table",
      eyebrow: "",
      title: "住址信息",
      caption: "fc_person_address",
      status: "人员",
      accent: "#2acfb4",
      metrics: [
        { label: "文件", value: formatFileKindCount(fileStats, "fc_person_address") },
        { label: "行", value: formatImportedRowsByKind(fileStats, "fc_person_address") }
      ]
    },
    {
      id: "table-contact",
      kind: "table",
      eyebrow: "",
      title: "联系方式",
      caption: "fc_person_contact",
      status: "人员",
      accent: "#2acfb4",
      metrics: [
        { label: "文件", value: formatFileKindCount(fileStats, "fc_person_contact") },
        { label: "行", value: formatImportedRowsByKind(fileStats, "fc_person_contact") }
      ]
    },
    {
      id: "raw",
      kind: "compact",
      eyebrow: "库",
      title: "原始库",
      caption: "fc_*_raw",
      status: "7表",
      accent: "#5f7fff",
      metrics: [
        { label: "表", value: "7" },
        { label: "行", value: totalRows }
      ]
    },
    {
      id: "clean",
      kind: "compact",
      eyebrow: "库",
      title: "清洗库",
      caption: "fc_*_norm",
      status: currentJob ? `${currentJob.progress}%` : latestSucceededJob || cleanedOutputReady ? "完成" : "待生成",
      accent: "#2acfb4",
      metrics: [
        { label: "表", value: "7" },
        { label: "状态", value: latestSucceededJob || cleanedOutputReady ? "就绪" : "待清洗" }
      ]
    }
  ];
}

export function shouldShowCleaningExportStrip(exportState: ExportState): boolean {
  return exportState.status !== "idle" || Boolean(exportState.jobId) || Boolean(exportState.outputPath);
}
