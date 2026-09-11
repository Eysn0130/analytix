import {
  ReactNode,
  useCallback,
  useEffect,
  useRef,
  useState
} from "react";
import { createPortal } from "react-dom";
import { BlueAlertLottie } from "./BlueAlertLottie";
import { FailAlertLottie } from "./FailAlertLottie";
import { InsiderLoadingLottie } from "./InsiderLoadingLottie";
import { SuccessCheckedLottie } from "./SuccessCheckedLottie";
import { WarningTekkisLottie } from "./WarningTekkisLottie";

export type ToastTone = "info" | "running" | "success" | "warning" | "error";

export interface ToastEventDetail {
  id?: string;
  tone?: ToastTone;
  title: string;
  detail?: string;
  durationMs?: number;
  dismissible?: boolean;
  dedupeKey?: string;
  actionLabel?: string;
  actionAriaLabel?: string;
  actionHref?: string;
  onAction?: () => void;
  closeOnAction?: boolean;
}

interface ToastMessage {
  id: string;
  tone: ToastTone;
  title: string;
  detail?: string;
  dismissible: boolean;
  duplicateCount: number;
  closing: boolean;
  createdAt: number;
  dedupeKey: string;
  actionLabel?: string;
  actionAriaLabel?: string;
  actionHref?: string;
  onAction?: () => void;
  closeOnAction: boolean;
}

interface ToastRuntimeMeta {
  durationMs: number;
  remainingMs: number;
  startedAt: number;
}

declare global {
  interface WindowEventMap {
    "analytix:toast": CustomEvent<ToastEventDetail>;
    "analytix:toast-dismiss": CustomEvent<{ id?: string }>;
  }
}

const TOAST_EVENT_NAME = "analytix:toast";
const TOAST_DISMISS_EVENT_NAME = "analytix:toast-dismiss";
const TOAST_EXIT_MS = 160;
const TOAST_MAX_COUNT = 4;
const TOAST_DEDUPE_WINDOW_MS = 1200;
const TOAST_DEFAULT_DURATION_MS: Record<ToastTone, number> = {
  info: 3400,
  running: 3800,
  success: 3200,
  warning: 5200,
  error: 6400
};

function createToastId(): string {
  return `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}

function normalizeToastDetail(detail: ToastEventDetail): Required<Pick<ToastEventDetail, "title">> &
  Omit<ToastEventDetail, "title"> & {
    id: string;
    tone: ToastTone;
    durationMs: number;
    dismissible: boolean;
    dedupeKey: string;
    actionLabel?: string;
    actionAriaLabel?: string;
    actionHref?: string;
    onAction?: () => void;
    closeOnAction: boolean;
  } {
  const title = String(detail.title || "").trim() || "系统提示";
  const tone = detail.tone ?? "info";
  const detailText = String(detail.detail || "").trim();
  const actionLabel = String(detail.actionLabel || "").trim();
  const actionAriaLabel = String(detail.actionAriaLabel || "").trim();
  const actionHref = String(detail.actionHref || "").trim();
  const onAction = typeof detail.onAction === "function" ? detail.onAction : undefined;
  const hasAction = Boolean(actionLabel && (actionHref || onAction));
  const durationMs =
    typeof detail.durationMs === "number" && Number.isFinite(detail.durationMs)
      ? Math.max(0, Math.round(detail.durationMs))
      : TOAST_DEFAULT_DURATION_MS[tone];

  return {
    ...detail,
    id: detail.id || createToastId(),
    title,
    detail: detailText || undefined,
    tone,
    durationMs,
    dismissible: detail.dismissible !== false,
    dedupeKey: String(detail.dedupeKey || `${tone}|${title}|${detailText}`).trim(),
    actionLabel: hasAction ? actionLabel : undefined,
    actionAriaLabel: hasAction ? actionAriaLabel : undefined,
    actionHref: hasAction ? actionHref || undefined : undefined,
    onAction: hasAction ? onAction : undefined,
    closeOnAction: detail.closeOnAction !== false
  };
}

function toastTonePriority(tone: ToastTone): number {
  if (tone === "error") {
    return 4;
  }
  if (tone === "warning") {
    return 3;
  }
  if (tone === "running") {
    return 2;
  }
  if (tone === "info") {
    return 1;
  }
  return 0;
}

function resolveOverflowToast(
  toasts: ToastMessage[],
  nextTone: ToastTone,
): { dropIncoming: boolean; toastId?: string } {
  const visible = toasts.filter((item) => !item.closing);
  if (visible.length < TOAST_MAX_COUNT) {
    return { dropIncoming: false };
  }

  const weakest = [...visible].sort((left, right) => {
    const priorityDelta = toastTonePriority(left.tone) - toastTonePriority(right.tone);
    if (priorityDelta !== 0) {
      return priorityDelta;
    }
    return left.createdAt - right.createdAt;
  })[0];

  if (!weakest) {
    return { dropIncoming: false };
  }

  if (toastTonePriority(nextTone) < toastTonePriority(weakest.tone)) {
    return { dropIncoming: true };
  }

  return { dropIncoming: false, toastId: weakest.id };
}

function ToastGlyph({ tone }: { tone: ToastTone }): JSX.Element {
  if (tone === "running") {
    return <InsiderLoadingLottie variant="toast" />;
  }
  if (tone === "success") {
    return <SuccessCheckedLottie variant="toast" />;
  }
  if (tone === "warning") {
    return <WarningTekkisLottie variant="toast" />;
  }
  if (tone === "error") {
    return <FailAlertLottie variant="toast" />;
  }
  return <BlueAlertLottie variant="toast" />;
}

function ToastCenter({
  toasts,
  onDismiss,
  onAction,
  onPause,
  onResume
}: {
  toasts: ToastMessage[];
  onDismiss: (id: string) => void;
  onAction: (id: string) => void;
  onPause: (id: string) => void;
  onResume: (id: string) => void;
}): JSX.Element {
  if (!toasts.length) {
    return <></>;
  }

  return (
    <aside className="ui-toast-center" aria-live="polite" aria-relevant="additions text">
      {toasts.map((toast) => (
        <article
          key={toast.id}
          className={`ui-toast ui-toast-${toast.tone}${toast.closing ? " is-closing" : ""}`}
          role={toast.tone === "error" || toast.tone === "warning" ? "alert" : "status"}
          onMouseEnter={() => onPause(toast.id)}
          onMouseLeave={() => onResume(toast.id)}
          onFocusCapture={() => onPause(toast.id)}
          onBlurCapture={() => onResume(toast.id)}
          aria-atomic="true"
        >
          <div className="ui-toast-icon" aria-hidden="true">
            <ToastGlyph tone={toast.tone} />
          </div>
          <div className="ui-toast-content">
            <div className="ui-toast-title-row">
              <strong title={toast.title}>{toast.title}</strong>
              {toast.duplicateCount > 1 ? <span className="ui-toast-repeat">x{toast.duplicateCount}</span> : null}
            </div>
            {toast.detail ? <p title={toast.detail}>{toast.detail}</p> : null}
            {toast.actionLabel ? (
              <div className="ui-toast-actions">
                <button
                  type="button"
                  className="ui-toast-action"
                  onClick={() => onAction(toast.id)}
                  aria-label={toast.actionAriaLabel || toast.actionLabel}
                >
                  <span>{toast.actionLabel}</span>
                  <svg viewBox="0 0 24 24" aria-hidden="true">
                    <path d="M8 12h8" />
                    <path d="m12 8 4 4-4 4" />
                  </svg>
                </button>
              </div>
            ) : null}
          </div>
          {toast.dismissible ? (
            <button
              type="button"
              className="ui-toast-close"
              onClick={() => onDismiss(toast.id)}
              aria-label="关闭提示"
            >
              <svg viewBox="0 0 24 24" aria-hidden="true">
                <path d="M7 7 17 17" />
                <path d="M17 7 7 17" />
              </svg>
            </button>
          ) : null}
        </article>
      ))}
    </aside>
  );
}

export function emitToast(detail: ToastEventDetail): string {
  const normalized = normalizeToastDetail(detail);
  if (typeof window !== "undefined") {
    window.dispatchEvent(
      new CustomEvent<ToastEventDetail>(TOAST_EVENT_NAME, {
        detail: normalized,
        cancelable: true
      })
    );
  }
  return normalized.id;
}

export function dismissToast(id: string): void {
  const toastId = String(id || "").trim();
  if (!toastId || typeof window === "undefined") {
    return;
  }
  window.dispatchEvent(
    new CustomEvent<{ id?: string }>(TOAST_DISMISS_EVENT_NAME, {
      detail: { id: toastId },
      cancelable: true
    })
  );
}

export function ToastProvider({ children }: { children: ReactNode }): JSX.Element {
  const [toasts, setToasts] = useState<ToastMessage[]>([]);
  const toastsRef = useRef<ToastMessage[]>([]);
  const dismissalTimersRef = useRef<Map<string, number>>(new Map());
  const removalTimersRef = useRef<Map<string, number>>(new Map());
  const runtimeRef = useRef<Map<string, ToastRuntimeMeta>>(new Map());

  useEffect(() => {
    toastsRef.current = toasts;
  }, [toasts]);

  const clearDismissTimer = useCallback((id: string) => {
    const timer = dismissalTimersRef.current.get(id);
    if (timer !== undefined) {
      window.clearTimeout(timer);
      dismissalTimersRef.current.delete(id);
    }
  }, []);

  const clearRemovalTimer = useCallback((id: string) => {
    const timer = removalTimersRef.current.get(id);
    if (timer !== undefined) {
      window.clearTimeout(timer);
      removalTimersRef.current.delete(id);
    }
  }, []);

  const disposeToastRuntime = useCallback(
    (id: string) => {
      clearDismissTimer(id);
      clearRemovalTimer(id);
      runtimeRef.current.delete(id);
    },
    [clearDismissTimer, clearRemovalTimer]
  );

  const removeToast = useCallback(
    (id: string) => {
      disposeToastRuntime(id);
      setToasts((prev) => prev.filter((item) => item.id !== id));
    },
    [disposeToastRuntime]
  );

  const dismiss = useCallback(
    (id: string) => {
      const current = toastsRef.current.find((item) => item.id === id);
      if (!current || current.closing) {
        return;
      }
      clearDismissTimer(id);
      clearRemovalTimer(id);
      setToasts((prev) => prev.map((item) => (item.id === id ? { ...item, closing: true } : item)));
      const removalTimer = window.setTimeout(() => {
        removeToast(id);
      }, TOAST_EXIT_MS);
      removalTimersRef.current.set(id, removalTimer);
    },
    [clearDismissTimer, clearRemovalTimer, removeToast]
  );

  const handleAction = useCallback(
    (id: string) => {
      const current = toastsRef.current.find((item) => item.id === id);
      if (!current) {
        return;
      }

      try {
        if (current.onAction) {
          current.onAction();
        } else if (current.actionHref) {
          window.location.assign(current.actionHref);
        }
        if (current.closeOnAction) {
          dismiss(id);
        }
      } catch {
        console.error("[toast action failed]");
      }
    },
    [dismiss]
  );

  const scheduleDismiss = useCallback(
    (id: string, delayMs: number) => {
      clearDismissTimer(id);
      if (delayMs <= 0) {
        return;
      }
      runtimeRef.current.set(id, {
        durationMs: delayMs,
        remainingMs: delayMs,
        startedAt: Date.now()
      });
      const timer = window.setTimeout(() => {
        dismiss(id);
      }, delayMs);
      dismissalTimersRef.current.set(id, timer);
    },
    [clearDismissTimer, dismiss]
  );

  const pauseDismiss = useCallback(
    (id: string) => {
      const meta = runtimeRef.current.get(id);
      if (!meta) {
        return;
      }
      clearDismissTimer(id);
      const elapsed = Date.now() - meta.startedAt;
      meta.remainingMs = Math.max(180, meta.remainingMs - elapsed);
      runtimeRef.current.set(id, meta);
    },
    [clearDismissTimer]
  );

  const resumeDismiss = useCallback(
    (id: string) => {
      const meta = runtimeRef.current.get(id);
      const toast = toastsRef.current.find((item) => item.id === id);
      if (!meta || !toast || toast.closing || meta.remainingMs <= 0) {
        return;
      }
      meta.startedAt = Date.now();
      runtimeRef.current.set(id, meta);
      clearDismissTimer(id);
      const timer = window.setTimeout(() => {
        dismiss(id);
      }, meta.remainingMs);
      dismissalTimersRef.current.set(id, timer);
    },
    [clearDismissTimer, dismiss]
  );

  const notify = useCallback(
    (detail: ToastEventDetail) => {
      const normalized = normalizeToastDetail(detail);
      const now = Date.now();
      const existingById = toastsRef.current.find(
        (item) => !item.closing && item.id === normalized.id
      );
      const existingByDedupe = existingById
        ? undefined
        : toastsRef.current.find(
            (item) =>
              !item.closing &&
              item.dedupeKey === normalized.dedupeKey &&
              now - item.createdAt <= TOAST_DEDUPE_WINDOW_MS
          );
      const existing = existingById ?? existingByDedupe;
      const isIdentityUpdate = Boolean(existingById);

      if (existing) {
        clearRemovalTimer(existing.id);
        setToasts((prev) =>
          prev.map((item) =>
            item.id === existing.id
              ? {
                  ...item,
                  tone: normalized.tone,
                  title: normalized.title,
                  detail: normalized.detail,
                  dismissible: normalized.dismissible,
                  dedupeKey: normalized.dedupeKey,
                  actionLabel: normalized.actionLabel,
                  actionAriaLabel: normalized.actionAriaLabel,
                  actionHref: normalized.actionHref,
                  onAction: normalized.onAction,
                  closeOnAction: normalized.closeOnAction,
                  duplicateCount: isIdentityUpdate
                    ? item.duplicateCount
                    : item.duplicateCount + 1,
                  createdAt: now,
                  closing: false
                }
              : item
          )
        );
        if (normalized.durationMs > 0) {
          scheduleDismiss(existing.id, normalized.durationMs);
        } else {
          clearDismissTimer(existing.id);
          runtimeRef.current.delete(existing.id);
        }
        return existing.id;
      }

      const overflow = resolveOverflowToast(toastsRef.current, normalized.tone);
      if (overflow.dropIncoming) {
        return normalized.id;
      }

      if (overflow.toastId) {
        disposeToastRuntime(overflow.toastId);
      }

      const next: ToastMessage = {
        id: normalized.id,
        tone: normalized.tone,
        title: normalized.title,
        detail: normalized.detail,
        dismissible: normalized.dismissible,
        duplicateCount: 1,
        closing: false,
        createdAt: now,
        dedupeKey: normalized.dedupeKey,
        actionLabel: normalized.actionLabel,
        actionAriaLabel: normalized.actionAriaLabel,
        actionHref: normalized.actionHref,
        onAction: normalized.onAction,
        closeOnAction: normalized.closeOnAction
      };

      setToasts((prev) => [
        next,
        ...prev.filter((item) => item.id !== normalized.id && item.id !== overflow.toastId)
      ]);
      if (normalized.durationMs > 0) {
        scheduleDismiss(normalized.id, normalized.durationMs);
      }
      return normalized.id;
    },
    [clearDismissTimer, clearRemovalTimer, disposeToastRuntime, scheduleDismiss]
  );

  useEffect(() => {
    const handleToastEvent = (event: WindowEventMap[typeof TOAST_EVENT_NAME]): void => {
      event.preventDefault();
      notify(event.detail);
    };
    const handleToastDismissEvent = (event: WindowEventMap[typeof TOAST_DISMISS_EVENT_NAME]): void => {
      const id = String(event.detail?.id || "").trim();
      if (!id) {
        return;
      }
      event.preventDefault();
      dismiss(id);
    };

    window.addEventListener(TOAST_EVENT_NAME, handleToastEvent);
    window.addEventListener(TOAST_DISMISS_EVENT_NAME, handleToastDismissEvent);
    return () => {
      window.removeEventListener(TOAST_EVENT_NAME, handleToastEvent);
      window.removeEventListener(TOAST_DISMISS_EVENT_NAME, handleToastDismissEvent);
    };
  }, [dismiss, notify]);

  useEffect(() => {
    const dismissalTimers = dismissalTimersRef.current;
    const removalTimers = removalTimersRef.current;
    const runtime = runtimeRef.current;
    return () => {
      dismissalTimers.forEach((timer) => window.clearTimeout(timer));
      dismissalTimers.clear();
      removalTimers.forEach((timer) => window.clearTimeout(timer));
      removalTimers.clear();
      runtime.clear();
    };
  }, []);

  const toastLayer = typeof document === "undefined"
    ? null
    : createPortal(
        <ToastCenter
          toasts={toasts}
          onDismiss={dismiss}
          onAction={handleAction}
          onPause={pauseDismiss}
          onResume={resumeDismiss}
        />,
        document.body
      );

  return (
    <>
      {children}
      {toastLayer}
    </>
  );
}
