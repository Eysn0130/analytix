export function abortError(signal, fallback = "operation cancelled") {
  if (signal?.reason instanceof Error) return signal.reason;
  const message = String(signal?.reason == null ? "" : signal.reason).trim() || fallback;
  const error = new Error(message);
  error.name = "AbortError";
  error.code = "ABORT_ERR";
  return error;
}

export function throwIfAborted(signal, fallback) {
  if (signal?.aborted) throw abortError(signal, fallback);
}
