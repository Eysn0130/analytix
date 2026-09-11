export type EmbeddedFlowCallback = (value: unknown) => void;

export function text(value: unknown): string {
  return String(value == null ? "" : value).trim();
}

export function asObject(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : {};
}

export function asNullableObject(value: unknown): Record<string, unknown> | null {
  return value && typeof value === "object" && !Array.isArray(value)
    ? cloneJson(value as Record<string, unknown>, {})
    : null;
}

export function asArray(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

export function parseJson<T>(raw: unknown, fallback: T): T {
  try {
    if (!raw) return fallback;
    return JSON.parse(String(raw)) as T;
  } catch {
    return fallback;
  }
}

export function uniqueStrings(values: unknown): string[] {
  const source = Array.isArray(values) ? values : typeof values === "string" ? [values] : [];
  const next = new Set<string>();
  source.forEach((item) => {
    const value = text(item);
    if (value) next.add(value);
  });
  return Array.from(next);
}

export function toFiniteNumber(value: unknown, fallback = 0): number {
  const next = Number(value);
  return Number.isFinite(next) ? next : fallback;
}

export function clampInt(value: unknown, fallback: number, minValue: number, maxValue: number): number {
  const next = Math.round(toFiniteNumber(value, fallback));
  return Math.max(minValue, Math.min(maxValue, next));
}

export function cloneJson<T>(value: T, fallback: T): T {
  try {
    return JSON.parse(JSON.stringify(value)) as T;
  } catch {
    return fallback;
  }
}

export function mapDirection(value: unknown): "in" | "out" | "both" {
  const next = text(value).toLowerCase();
  if (next === "in" || next === "out" || next === "both") return next;
  if (next === "all") return "both";
  return "both";
}

export function invoke(callback: EmbeddedFlowCallback | undefined, value: unknown): void {
  if (typeof callback !== "function") return;
  window.setTimeout(() => {
    try {
      callback(value);
    } catch {
      // noop
    }
  }, 0);
}
