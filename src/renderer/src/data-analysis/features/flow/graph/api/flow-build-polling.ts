const FLOW_BUILD_POLL_OFFLINE_INITIAL_MS = 180;
const FLOW_BUILD_POLL_OFFLINE_ACTIVE_MS = 320;
const FLOW_BUILD_POLL_OFFLINE_STEADY_MS = 520;
const FLOW_BUILD_POLL_WS_INITIAL_MS = 1500;
const FLOW_BUILD_POLL_WS_STEADY_MS = 4500;

const FLOW_BUILD_CANCEL_CODES = new Set(["JOB_CANCELED", "FLOW_BUILD_CANCELED"]);

export const FLOW_BUILD_TIMEOUT_MS = 120_000;

function text(value: unknown): string {
  return String(value == null ? "" : value).trim();
}

function asObject(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : {};
}

export function getFlowBuildPollDelayMs({ wsReady, pollCount }: { wsReady: boolean; pollCount: number }): number {
  if (wsReady) {
    return pollCount <= 0 ? FLOW_BUILD_POLL_WS_INITIAL_MS : FLOW_BUILD_POLL_WS_STEADY_MS;
  }
  if (pollCount <= 0) {
    return FLOW_BUILD_POLL_OFFLINE_INITIAL_MS;
  }
  if (pollCount < 4) {
    return FLOW_BUILD_POLL_OFFLINE_ACTIVE_MS;
  }
  return FLOW_BUILD_POLL_OFFLINE_STEADY_MS;
}

export function isFlowBuildCanceled(source: unknown): boolean {
  const payload = asObject(source);
  const code = text(payload.code || payload.error_code || payload.errorCode);
  const message = text(payload.message || payload.error || payload.detail);
  return FLOW_BUILD_CANCEL_CODES.has(code) || /cancel/i.test(message);
}

export function toFlowBuildErrorMessage(source: unknown, fallback: string): string {
  const payload = asObject(source);
  const direct = text(payload.error || payload.message || payload.detail);
  return direct || fallback;
}
