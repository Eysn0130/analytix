import { emitToast, type ToastEventDetail, type ToastTone } from "./ToastCenter";

type ToastOptions = Pick<
  ToastEventDetail,
  | "durationMs"
  | "dismissible"
  | "dedupeKey"
  | "actionLabel"
  | "actionAriaLabel"
  | "actionHref"
  | "onAction"
  | "closeOnAction"
>;

export interface ToastCopy extends Pick<ToastEventDetail, "actionLabel" | "actionAriaLabel" | "actionHref" | "onAction" | "closeOnAction"> {
  tone: ToastTone;
  title: string;
  detail?: string;
  impact?: string;
  nextStep?: string;
}

const DEFAULT_DETAIL_MAX_LENGTH = 60;
const DETAIL_IMPACT_MAX_LENGTH = 34;
const DETAIL_NEXT_STEP_MAX_LENGTH = 18;
const DEFAULT_NEXT_STEP_BY_TONE: Record<ToastTone, string> = {
  info: "可继续查看当前页面",
  running: "可先处理其他任务",
  success: "可继续核对结果",
  warning: "请先复核后再继续",
  error: "请按提示修正后重试"
};
const NEXT_STEP_MARKERS = [
  "请",
  "可",
  "建议",
  "进入",
  "前往",
  "查看",
  "打开",
  "返回",
  "稍后",
  "重新",
  "继续",
  "联系",
  "改用",
  "切换",
  "等待"
];

function normalizeText(value: unknown): string {
  return String(value || "")
    .replace(/\s+/g, " ")
    .trim();
}

function trimTrailingPunctuation(value: string): string {
  return value.replace(/[。！？.!?；;，,、]+$/u, "").trim();
}

function ensureSentence(value: string): string {
  const text = trimTrailingPunctuation(normalizeText(value));
  return text ? `${text}。` : "";
}

function looksLikeNextStep(value: string): boolean {
  const text = trimTrailingPunctuation(normalizeText(value));
  if (!text) {
    return false;
  }
  return NEXT_STEP_MARKERS.some((marker) => text.startsWith(marker));
}

function splitDetailByPunctuation(detail: string): string[] {
  return detail
    .split(/[。！？!?；;]+/u)
    .flatMap((segment) => segment.split(/，/u))
    .map((segment) => trimTrailingPunctuation(segment))
    .filter(Boolean);
}

function splitToastNarrative(detail: string): { impact?: string; nextStep?: string } {
  const normalized = trimTrailingPunctuation(normalizeText(detail));
  if (!normalized) {
    return {};
  }

  const segments = splitDetailByPunctuation(normalized);
  if (!segments.length) {
    return {};
  }

  if (segments.length === 1) {
    const [single] = segments;
    if (!single) {
      return {};
    }
    return looksLikeNextStep(single) ? { nextStep: single } : { impact: single };
  }

  const tail = segments[segments.length - 1];
  if (tail && looksLikeNextStep(tail)) {
    const impact = segments.slice(0, -1).join("，");
    return {
      impact: impact || undefined,
      nextStep: tail
    };
  }

  return {
    impact: segments.join("，")
  };
}

function buildActionNextStep(actionLabel?: string): string {
  const label = trimTrailingPunctuation(normalizeText(actionLabel));
  if (!label) {
    return "";
  }
  return label.startsWith("请") ? label : `可${label}`;
}

function compactToastClause(value: string, maxLength: number): string {
  const normalized = trimTrailingPunctuation(normalizeText(value));
  if (!normalized) {
    return "";
  }

  if (/[\\/]/u.test(normalized) || normalized.includes(".../")) {
    return compactToastPath(normalized, maxLength);
  }

  return compactToastText(normalized, maxLength);
}

function buildToastFormulaDetail(
  tone: ToastTone,
  detail?: string,
  impact?: string,
  nextStep?: string,
  actionLabel?: string
): string | undefined {
  const parsed = detail ? splitToastNarrative(detail) : {};
  const resolvedImpact = trimTrailingPunctuation(impact || parsed.impact || detail || "");
  const resolvedNextStep = trimTrailingPunctuation(
    nextStep || parsed.nextStep || buildActionNextStep(actionLabel) || DEFAULT_NEXT_STEP_BY_TONE[tone]
  );

  if (!resolvedImpact && !resolvedNextStep) {
    return undefined;
  }

  if (!resolvedImpact) {
    return ensureSentence(compactToastClause(resolvedNextStep, DEFAULT_DETAIL_MAX_LENGTH));
  }

  if (!resolvedNextStep) {
    return ensureSentence(compactToastClause(resolvedImpact, DEFAULT_DETAIL_MAX_LENGTH));
  }

  const impactText = compactToastClause(resolvedImpact, DETAIL_IMPACT_MAX_LENGTH);
  const nextStepText = compactToastClause(resolvedNextStep, DETAIL_NEXT_STEP_MAX_LENGTH);
  return `${ensureSentence(impactText)} ${ensureSentence(nextStepText)}`;
}

export function compactToastText(value: unknown, maxLength = DEFAULT_DETAIL_MAX_LENGTH): string {
  const text = normalizeText(value);
  if (!text) {
    return "";
  }
  if (text.length <= maxLength) {
    return text;
  }
  return `${text.slice(0, Math.max(0, maxLength - 1)).trimEnd()}…`;
}

export function compactToastPath(value: unknown, maxLength = DEFAULT_DETAIL_MAX_LENGTH): string {
  const text = normalizeText(value);
  if (!text) {
    return "";
  }
  if (text.length <= maxLength) {
    return text;
  }

  const segments = text.split(/[\\/]+/u).filter(Boolean);
  if (segments.length >= 2) {
    return `.../${segments.slice(-2).join("/")}`;
  }
  return compactToastText(text, maxLength);
}

export function toToastErrorDetail(error: unknown, fallback = "请稍后重试。"): string {
  const raw = normalizeText(
    error instanceof Error
      ? error.message
      : typeof error === "string"
        ? error
        : fallback
  );

  const cleaned = raw
    .replace(/^[A-Z][A-Z0-9_]+:\s*/u, "")
    .replace(/^Error:\s*/u, "")
    .trim();

  const compacted = compactToastText(cleaned || fallback);
  const narrative = splitToastNarrative(compacted);
  const fallbackImpact = "当前指令未完成";
  const resolvedImpact =
    narrative.impact ||
    (!looksLikeNextStep(compacted) ? trimTrailingPunctuation(compacted) : fallbackImpact);
  const resolvedNextStep =
    narrative.nextStep ||
    (looksLikeNextStep(compacted) ? trimTrailingPunctuation(compacted) : "请按提示修正后重试");

  return buildToastFormulaDetail("error", undefined, resolvedImpact, resolvedNextStep) || ensureSentence(fallbackImpact);
}

export function normalizeToastCopy<T extends ToastCopy>(copy: T): T {
  const title = String(copy.title || "").trim() || "系统提示";
  const actionLabel = normalizeText(copy.actionLabel);
  const actionAriaLabel = normalizeText(copy.actionAriaLabel);
  const detail = buildToastFormulaDetail(copy.tone, copy.detail, copy.impact, copy.nextStep, actionLabel);

  return {
    ...copy,
    title,
    detail,
    impact: undefined,
    nextStep: undefined,
    actionLabel: actionLabel || undefined,
    actionAriaLabel: actionAriaLabel || undefined
  };
}

export function buildNotificationCenterToast(reasons: readonly string[]): ToastCopy {
  if (!reasons.length) {
    return {
      tone: "info",
      title: "当前无异常提示",
      impact: "系统未检测到需要立即处理的异常信号",
      nextStep: "可继续查看当前页面"
    };
  }

  const [firstReason = "存在待处理提醒"] = reasons;
  const normalizedReason = trimTrailingPunctuation(compactToastText(firstReason, 44)) || "存在待处理提醒";
  const impact =
    reasons.length > 1
      ? `${normalizedReason} 等 ${reasons.length} 项仍在待排查范围内`
      : `${normalizedReason}仍在待排查范围内`;

  return {
    tone: "warning",
    title: "发现待处置信号",
    impact,
    nextStep: "请进入通知中心继续排查"
  };
}

export function showToast(copy: ToastCopy, options?: ToastOptions): string {
  return emitToast(normalizeToastCopy({ ...copy, ...(options ?? {}) }));
}
