import {
  compactToastText,
  toToastErrorDetail,
  type ToastCopy,
} from "../../../components/feedback/toast-copy";

export type StatsToastKind = "info" | "ok" | "warn" | "danger";

export function resolveStatsToast(
  message: string,
  kind: StatsToastKind,
): ToastCopy {
  const text = String(message || "").trim();

  if (kind === "danger") {
    return {
      tone: "error",
      title: "统计分析执行失败",
      detail: toToastErrorDetail(text),
    };
  }

  if (kind === "ok") {
    return resolveStatsSuccessToast(text);
  }

  return resolveStatsInfoToast(text, kind);
}

function resolveStatsSuccessToast(text: string): ToastCopy {
  if (text.startsWith("已创建导出任务：")) {
    const jobId = text.replace("已创建导出任务：", "").trim();
    return {
      tone: "running",
      title: "统计导出任务已入列",
      detail: jobId
        ? `任务 ${jobId} 已进入后台队列，可继续查看当前工作台。`
        : "统计导出任务已进入后台队列。",
    };
  }
  if (text === "导出完成") {
    return {
      tone: "success",
      title: "统计数据已导出",
      detail: "结果文件已写入导出目录。",
    };
  }
  if (text === "已标记待调单") {
    return {
      tone: "success",
      title: "已标记待调单",
      detail: "当前统计行已加入待调单。",
    };
  }
  if (text === "已取消待调单") {
    return {
      tone: "success",
      title: "已取消待调单",
      detail: "当前统计行已移出待调单。",
    };
  }
  if (text.startsWith("更新完成：")) {
    return {
      tone: "success",
      title: "对象信息已更新",
      detail: compactToastText(text.replace("更新完成：", "")),
    };
  }
  if (text.startsWith("删除完成：")) {
    return {
      tone: "success",
      title: "对象数据已删除",
      detail: compactToastText(text.replace("删除完成：", "")),
    };
  }
  return {
    tone: "success",
    title: compactToastText(text, 24) || "操作已完成",
    detail: undefined,
  };
}

function resolveStatsInfoToast(text: string, kind: StatsToastKind): ToastCopy {
  if (text.includes("请先选择案件")) {
    return {
      tone: "info",
      title: "先打开案件",
      detail: "进入统计分析前，请先在案件页打开一个案件。",
    };
  }
  if (text.includes("未选择统计行")) {
    return {
      tone: "info",
      title: "先选择统计行",
      detail: "请先选中一行统计结果，再执行当前操作。",
    };
  }
  if (text.includes("缺少可用对象，无法生成图谱")) {
    return {
      tone: "info",
      title: "当前没有可用于生成图谱的对象",
      detail: "请先在左侧选择对象，或补全统计数据后再试。",
    };
  }
  if (text.includes("请先在左侧选择对象")) {
    return {
      tone: "info",
      title: "先选择左侧对象",
      detail: "请先在左侧树中选择对象后，再执行当前操作。",
    };
  }
  if (text.includes("请先选择对象后执行批量操作")) {
    return {
      tone: "info",
      title: "先选择对象",
      detail: "请先选择至少一个对象后，再执行批量操作。",
    };
  }
  if (text.includes("当前无可导出统计数据")) {
    return {
      tone: "info",
      title: "当前没有可导出的统计数据",
      detail: "请先生成统计结果后，再执行导出。",
    };
  }
  if (text.includes("已取消导出")) {
    return {
      tone: "info",
      title: "已取消导出",
      detail: "系统未创建新的统计导出任务。",
    };
  }
  if (text.includes("请先取消该父级下的子级勾选，再收起父级")) {
    return {
      tone: "warning",
      title: "当前存在待处理勾选项",
      detail: "请先取消该父级下的子级勾选，再收起父级。",
    };
  }

  return {
    tone: kind === "warn" ? "warning" : "info",
    title: kind === "warn" ? "统计分析待复核" : compactToastText(text, 24) || "系统提示",
    detail: kind === "warn" ? compactToastText(text) : undefined,
  };
}
