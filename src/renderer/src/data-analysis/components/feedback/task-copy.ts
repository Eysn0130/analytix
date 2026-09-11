type TaskCopyPhase = "running" | "success";

function normalizeText(value: unknown, fallback = ""): string {
  const text = String(value || "").trim();
  return text || fallback;
}

export function defaultTaskTitle(phase: TaskCopyPhase): string {
  return phase === "success" ? "当前任务已完成" : "正在处理当前任务";
}

export function taskPhaseChip(phase: TaskCopyPhase, blocking: boolean): string {
  if (phase === "success") {
    return "完成";
  }
  return blocking ? "进行中" : "后台执行";
}

export function taskHelperText(phase: TaskCopyPhase, blocking: boolean): string {
  if (phase === "success") {
    return "本次操作已完成，当前工作区状态已经同步。";
  }
  if (blocking) {
    return "系统正在执行当前操作，界面暂时锁定以避免重复提交。";
  }
  return "当前任务正在后台执行，你可以继续浏览当前工作区。";
}

export function normalizeTaskTitle(value: unknown, phase: TaskCopyPhase): string {
  return normalizeText(value, defaultTaskTitle(phase));
}

export function resolveTaskTitle(value: unknown, fallback: unknown, phase: TaskCopyPhase): string {
  const direct = normalizeText(value);
  if (direct) {
    return direct;
  }
  const fallbackText = normalizeText(fallback);
  return fallbackText || defaultTaskTitle(phase);
}
