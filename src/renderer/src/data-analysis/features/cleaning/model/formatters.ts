import { canonicalNonnegativeCount } from "./known-metrics";

export function formatDateTime(value: string | null | undefined): string {
  if (!value) {
    return "--";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false
  }).format(date);
}

export function formatCount(value: number | null | undefined): string {
  const count = canonicalNonnegativeCount(value);
  if (count === null) {
    return "--";
  }
  return new Intl.NumberFormat("zh-CN").format(count);
}

export function formatNodeCount(value: number | null | undefined): string {
  const count = canonicalNonnegativeCount(value);
  if (count === null) {
    return "--";
  }
  if (count < 10000) {
    return new Intl.NumberFormat("zh-CN").format(count);
  }
  const compact = (count / 10000).toFixed(1).replace(/\.0$/, "");
  return `${compact}万`;
}

export function formatDurationMs(value: number | null | undefined): string {
  const duration = canonicalNonnegativeCount(value);
  if (duration === null) {
    return "--";
  }
  if (duration < 1000) {
    return `${duration}ms`;
  }
  return `${(duration / 1000).toFixed(duration >= 10_000 ? 0 : 1)}s`;
}
