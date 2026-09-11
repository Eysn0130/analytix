import { ReactNode, useCallback, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { Dialog } from "../overlay/Dialog";

export type ConfirmTone = "default" | "danger";

export interface ConfirmFact {
  label: string;
  value: string;
}

export interface ConfirmEventDetail {
  id?: string;
  tone?: ConfirmTone;
  title: string;
  subtitle?: string;
  description?: string;
  facts?: ConfirmFact[];
  requireAcknowledgement?: boolean;
  acknowledgementLabel?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  width?: number;
  resolve?: (value: boolean) => void;
}

interface ConfirmMessage {
  id: string;
  tone: ConfirmTone;
  title: string;
  subtitle?: string;
  description?: string;
  facts: ConfirmFact[];
  requireAcknowledgement: boolean;
  acknowledgementLabel?: string;
  confirmLabel: string;
  cancelLabel: string;
  width: number;
}

interface ConfirmQueueItem extends ConfirmMessage {
  resolve: (value: boolean) => void;
}

declare global {
  interface WindowEventMap {
    "analytix:confirm": CustomEvent<ConfirmEventDetail>;
  }
}

const CONFIRM_EVENT_NAME = "analytix:confirm";
const CONFIRM_DEFAULT_WIDTH = 520;

function createConfirmId(): string {
  return `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}

function splitSummaryClauses(text: string): string[] {
  const normalized = String(text || "").trim();
  if (!normalized) {
    return [];
  }

  const primaryClauses = normalized
    .split(/[。！？!?；;]/u)
    .map((item) => item.trim())
    .filter(Boolean);
  if (primaryClauses.length > 1) {
    return primaryClauses;
  }

  const secondaryClauses = normalized
    .split(/[，,]/u)
    .map((item) => item.trim())
    .filter(Boolean);

  return secondaryClauses.length > 1 ? secondaryClauses : primaryClauses;
}

function normalizeConfirmFacts(detail: ConfirmEventDetail, tone: ConfirmTone): ConfirmFact[] {
  const explicitFacts = Array.isArray(detail.facts)
    ? detail.facts
        .map((item) => ({
          label: String(item?.label || "").trim(),
          value: String(item?.value || "").trim()
        }))
        .filter((item) => item.label && item.value)
        .slice(0, 3)
    : [];

  if (explicitFacts.length) {
    return explicitFacts;
  }

  const impact = String(detail.subtitle || "").trim() || "当前选中的对象";
  const description = String(detail.description || "").trim();
  const clauses = splitSummaryClauses(description);

  if (tone !== "danger") {
    const facts: ConfirmFact[] = [];
    if (impact) {
      facts.push({
        label: "当前对象",
        value: impact
      });
    }
    if (clauses[0]) {
      facts.push({
        label: "执行结果",
        value: clauses[0]
      });
    }
    if (clauses[1]) {
      facts.push({
        label: "后续处理",
        value: clauses.slice(1).join("，")
      });
    }
    return facts.slice(0, 3);
  }

  const immediate = clauses[0] || "确认后系统会立即执行本次变更。";
  const irreversible = clauses.slice(1).join("，") || "执行后如需恢复，请通过后续操作重新处理。";

  return [
    {
      label: "影响对象",
      value: impact
    },
    {
      label: "立即生效",
      value: immediate
    },
    {
      label: "不可撤回",
      value: irreversible
    }
  ];
}

function normalizeConfirmDetail(detail: ConfirmEventDetail): ConfirmMessage {
  const title = String(detail.title || "").trim() || "请确认操作";
  const subtitle = String(detail.subtitle || "").trim();
  const description = String(detail.description || "").trim();
  const tone = detail.tone === "danger" ? "danger" : "default";
  const facts = normalizeConfirmFacts(detail, tone);
  const requireAcknowledgement =
    typeof detail.requireAcknowledgement === "boolean" ? detail.requireAcknowledgement : tone === "danger";
  const acknowledgementLabel = String(
    detail.acknowledgementLabel ||
      (tone === "danger" ? "我已确认本次变更会立即生效，并理解其影响结果。" : "")
  ).trim();
  const confirmLabel = String(detail.confirmLabel || (tone === "danger" ? "确认继续" : "确认")).trim() || "确认";
  const cancelLabel = String(detail.cancelLabel || "取消").trim() || "取消";
  const width =
    typeof detail.width === "number" && Number.isFinite(detail.width)
      ? Math.max(420, Math.round(detail.width))
      : CONFIRM_DEFAULT_WIDTH;

  return {
    id: detail.id || createConfirmId(),
    tone,
    title,
    subtitle: subtitle || undefined,
    description: description || undefined,
    facts,
    requireAcknowledgement,
    acknowledgementLabel: acknowledgementLabel || undefined,
    confirmLabel,
    cancelLabel,
    width
  };
}

function buildNativeConfirmMessage(detail: ConfirmMessage): string {
  const facts = detail.facts.map((item) => `${item.label}：${item.value}`);
  return [detail.title, detail.subtitle, ...facts, detail.description].filter(Boolean).join("\n\n");
}

export function emitConfirm(detail: ConfirmEventDetail): Promise<boolean> {
  const normalized = normalizeConfirmDetail(detail);
  if (typeof window === "undefined") {
    return Promise.resolve(false);
  }

  return new Promise((resolve) => {
    const event = new CustomEvent<ConfirmEventDetail>(CONFIRM_EVENT_NAME, {
      detail: {
        ...normalized,
        resolve
      },
      cancelable: true
    });
    window.dispatchEvent(event);
    if (!event.defaultPrevented) {
      resolve(window.confirm(buildNativeConfirmMessage(normalized)));
    }
  });
}

export function ConfirmProvider({ children }: { children: ReactNode }): JSX.Element {
  const [queue, setQueue] = useState<ConfirmQueueItem[]>([]);
  const [acknowledged, setAcknowledged] = useState(false);
  const queueRef = useRef<ConfirmQueueItem[]>([]);
  const cancelButtonRef = useRef<HTMLButtonElement | null>(null);

  useEffect(() => {
    queueRef.current = queue;
  }, [queue]);

  const settle = useCallback((value: boolean) => {
    const current = queueRef.current[0];
    if (!current) {
      return;
    }
    setAcknowledged(false);
    current.resolve(value);
    setQueue((prev) => prev.slice(1));
  }, []);

  useEffect(() => {
    const onConfirm = (event: Event): void => {
      const customEvent = event as CustomEvent<ConfirmEventDetail>;
      const resolve = customEvent.detail?.resolve;
      if (typeof resolve !== "function") {
        return;
      }
      customEvent.preventDefault();
      const normalized = normalizeConfirmDetail(customEvent.detail);
      setQueue((prev) => [
        ...prev,
        {
          ...normalized,
          resolve
        }
      ]);
    };

    window.addEventListener(CONFIRM_EVENT_NAME, onConfirm as EventListener);
    return () => {
      window.removeEventListener(CONFIRM_EVENT_NAME, onConfirm as EventListener);
      queueRef.current.forEach((item) => item.resolve(false));
      queueRef.current = [];
    };
  }, []);

  const active = queue[0] ?? null;

  useEffect(() => {
    if (!active) {
      return;
    }
    setAcknowledged(false);
    const timer = window.setTimeout(() => {
      cancelButtonRef.current?.focus();
    }, 24);
    return () => {
      window.clearTimeout(timer);
    };
  }, [active]);

  const isConfirmDisabled = Boolean(active?.tone === "danger" && active.requireAcknowledgement && !acknowledged);

  return (
    <>
      {children}
      {active && typeof document !== "undefined"
        ? createPortal(
            <Dialog
              open
              title={active.title}
              subtitle={active.tone === "danger" ? "高风险操作，请先确认影响范围" : "请确认后继续"}
              width={active.width}
              className="ui-confirm-dialog"
              maskClassName="ui-confirm-mask"
              showCloseButton={false}
              footer={
                <>
                  <button type="button" className="btn" onClick={() => settle(false)} ref={cancelButtonRef}>
                    {active.cancelLabel}
                  </button>
                  <button
                    type="button"
                    className={`btn ui-confirm-submit${active.tone === "danger" ? " is-danger" : ""}`}
                    onClick={() => settle(true)}
                    disabled={isConfirmDisabled}
                  >
                    {active.confirmLabel}
                  </button>
                </>
              }
              onClose={() => settle(false)}
            >
              <div className={`ui-confirm-panel ui-confirm-panel-${active.tone}`}>
                {active.tone === "danger" ? (
                  <>
                    {active.facts.length ? (
                      <dl className="ui-confirm-facts">
                        {active.facts.map((item) => (
                          <div className="ui-confirm-fact" key={`${active.id}-${item.label}`}>
                            <dt>{item.label}</dt>
                            <dd>{item.value}</dd>
                          </div>
                        ))}
                      </dl>
                    ) : null}
                    {active.requireAcknowledgement && active.acknowledgementLabel ? (
                      <label className="ui-confirm-ack">
                        <input
                          type="checkbox"
                          checked={acknowledged}
                          onChange={(event) => setAcknowledged(event.target.checked)}
                        />
                        <span>{active.acknowledgementLabel}</span>
                      </label>
                    ) : null}
                  </>
                ) : active.facts.length ? (
                  <dl className="ui-confirm-facts ui-confirm-facts-default">
                    {active.facts.map((item) => (
                      <div className="ui-confirm-fact" key={`${active.id}-${item.label}`}>
                        <dt>{item.label}</dt>
                        <dd>{item.value}</dd>
                      </div>
                    ))}
                  </dl>
                ) : (
                  <div className="ui-confirm-copy">
                    {active.subtitle ? <strong className="ui-confirm-subtitle">{active.subtitle}</strong> : null}
                    {active.description ? <p className="ui-confirm-description">{active.description}</p> : null}
                  </div>
                )}
              </div>
            </Dialog>,
            document.body
          )
        : null}
    </>
  );
}
