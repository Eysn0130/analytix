export const UNKNOWN_CASE_FACT_LABEL = "--";

export function readFiniteCaseFactNumber(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) ? value : null;
}

export function readNonNegativeCaseFactNumber(value: unknown): number | null {
  const number = readFiniteCaseFactNumber(value);
  return number != null && number >= 0 ? number : null;
}

export function readNonNegativeCaseFactInteger(value: unknown): number | null {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0 ? value : null;
}

export function readCaseFactBoolean(value: unknown): boolean | null {
  return typeof value === "boolean" ? value : null;
}

export function formatCaseFactDecimal(value: unknown): string {
  const number = readFiniteCaseFactNumber(value);
  if (number == null) {
    return UNKNOWN_CASE_FACT_LABEL;
  }
  return number.toLocaleString("zh-CN", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
}

export function formatCaseFactMoney(value: unknown): string {
  const formatted = formatCaseFactDecimal(value);
  return formatted === UNKNOWN_CASE_FACT_LABEL ? formatted : `¥${formatted}`;
}

export function formatSignedCaseFactMoney(value: unknown): string {
  const amount = readFiniteCaseFactNumber(value);
  if (amount == null) {
    return UNKNOWN_CASE_FACT_LABEL;
  }
  if (amount === 0) {
    return formatCaseFactMoney(0);
  }
  return `${amount > 0 ? "+" : "-"}${formatCaseFactMoney(Math.abs(amount))}`;
}

export function formatCaseFactCount(value: unknown): string {
  const count = readNonNegativeCaseFactInteger(value);
  return count == null ? UNKNOWN_CASE_FACT_LABEL : count.toLocaleString("zh-CN");
}

export function formatCaseFactPercentage(value: unknown): string {
  const percentage = readFiniteCaseFactNumber(value);
  if (percentage == null || percentage < 0 || percentage > 100) {
    return UNKNOWN_CASE_FACT_LABEL;
  }
  return `${percentage.toFixed(0)}%`;
}
