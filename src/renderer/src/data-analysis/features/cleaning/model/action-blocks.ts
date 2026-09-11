export type CleaningActionBlockTone = "info" | "error";

export interface CleaningActionBlockViewModel {
  detail: string;
  message: string;
  title: string;
  tone: CleaningActionBlockTone;
}

export function buildOpenCaseRequiredActionBlock(kind: "cleaning" | "export"): CleaningActionBlockViewModel {
  return {
    detail: kind === "cleaning" ? "进入清洗工作台前，请先在案件页打开一个案件。" : "进入导出前，请先在案件页打开一个案件。",
    message: "请先在案件页打开一个案件。",
    title: "先打开案件",
    tone: "info"
  };
}

export function buildRunningCleaningJobActionBlock(jobId: string): CleaningActionBlockViewModel {
  return {
    detail: `任务 ${jobId} 正在运行，请等待完成后再发起新的清洗。`,
    message: `当前已有清洗任务 ${jobId} 正在执行，请等待完成后再试。`,
    title: "已有清洗任务执行中",
    tone: "info"
  };
}

export function buildNoCleanableDataActionBlock(): CleaningActionBlockViewModel {
  return {
    detail: "当前已验证清洗范围为 0 行，已阻止启动；该结果不代表全案不存在交易流水。",
    message: "当前可清洗范围为 0 行，请核对导入范围后重试。",
    title: "当前清洗范围为 0",
    tone: "info"
  };
}

export function buildUnverifiedCleanableDataActionBlock(): CleaningActionBlockViewModel {
  return {
    detail: "交易流水数量尚未通过完整数据边界验证，请刷新案件数据或补齐缺失统计后重试。",
    message: "清洗范围尚未验证，当前已阻止启动清洗。",
    title: "清洗范围待验证",
    tone: "info"
  };
}

export function buildExportBusyActionBlock(): CleaningActionBlockViewModel {
  return {
    detail: "请等待当前导出完成后，再发起新的导出。",
    message: "当前已有导出任务在执行，请等待当前导出完成后再试。",
    title: "已有导出任务执行中",
    tone: "info"
  };
}

export function buildCleaningActiveExportActionBlock(): CleaningActionBlockViewModel {
  return {
    detail: "请等待当前清洗或重新清洗完成后，再导出已清洗数据。",
    message: "当前清洗任务仍在执行，请等待清洗完成后再导出已清洗数据。",
    title: "清洗执行中",
    tone: "info"
  };
}

export function buildCleanedExportNotReadyActionBlock(): CleaningActionBlockViewModel {
  return {
    detail: "当前案件还没有完成清洗，或存在待重新清洗的数据，暂不能导出已清洗数据。",
    message: "请先完成数据清洗，再导出已清洗数据。",
    title: "已清洗数据尚未就绪",
    tone: "info"
  };
}
