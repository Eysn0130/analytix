import { FormEvent, ReactNode, useCallback, useEffect, useId, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { Dialog } from "../overlay/Dialog";

export type PromptLayout = "auto" | "compact" | "panel";

export interface PromptFieldDetail {
  key: string;
  label: string;
  placeholder?: string;
  defaultValue?: string;
  description?: string;
  autoComplete?: string;
  selectOnFocus?: boolean;
  type?: "text" | "search" | "password";
}

export type PromptValues = Record<string, string>;

export interface PromptEventDetail {
  id?: string;
  title: string;
  subtitle?: string;
  description?: string;
  layout?: PromptLayout;
  fields?: PromptFieldDetail[];
  confirmLabel?: string;
  cancelLabel?: string;
  width?: number;
  resolve?: (value: PromptValues | null) => void;
}

interface PromptMessage {
  id: string;
  title: string;
  subtitle?: string;
  description?: string;
  layout: Exclude<PromptLayout, "auto">;
  fields: PromptFieldDetail[];
  confirmLabel: string;
  cancelLabel: string;
  width: number;
}

interface PromptQueueItem extends PromptMessage {
  resolve: (value: PromptValues | null) => void;
}

declare global {
  interface WindowEventMap {
    "analytix:prompt": CustomEvent<PromptEventDetail>;
  }
}

const PROMPT_EVENT_NAME = "analytix:prompt";
const PROMPT_DEFAULT_WIDTH = 560;

function createPromptId(): string {
  return `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}

function normalizeFields(fields: PromptEventDetail["fields"]): PromptFieldDetail[] {
  const list = Array.isArray(fields) ? fields : [];
  if (!list.length) {
    return [
      {
        key: "value",
        label: "输入内容",
        defaultValue: "",
        type: "text"
      }
    ];
  }

  return list.map((field, index) => ({
    key: String(field?.key || `field_${index + 1}`).trim() || `field_${index + 1}`,
    label: String(field?.label || `字段 ${index + 1}`).trim() || `字段 ${index + 1}`,
    placeholder: String(field?.placeholder || "").trim() || undefined,
    defaultValue: String(field?.defaultValue || ""),
    description: String(field?.description || "").trim() || undefined,
    autoComplete: String(field?.autoComplete || "").trim() || undefined,
    selectOnFocus: field?.selectOnFocus === true,
    type: field?.type === "password" || field?.type === "search" ? field.type : "text"
  }));
}

function resolvePromptLayout(
  requestedLayout: PromptEventDetail["layout"],
  fields: PromptFieldDetail[]
): Exclude<PromptLayout, "auto"> {
  if (requestedLayout === "compact" || requestedLayout === "panel") {
    return requestedLayout;
  }
  return fields.length > 1 ? "panel" : "compact";
}

function normalizePromptDetail(detail: PromptEventDetail): PromptMessage {
  const fields = normalizeFields(detail.fields);
  const title = String(detail.title || "").trim() || "请输入信息";
  const subtitle = String(detail.subtitle || "").trim();
  const description = String(detail.description || "").trim();
  const confirmLabel = String(detail.confirmLabel || "确认提交").trim() || "确认提交";
  const cancelLabel = String(detail.cancelLabel || "取消").trim() || "取消";
  const width =
    typeof detail.width === "number" && Number.isFinite(detail.width)
      ? Math.max(460, Math.round(detail.width))
      : PROMPT_DEFAULT_WIDTH;

  return {
    id: detail.id || createPromptId(),
    title,
    subtitle: subtitle || undefined,
    description: description || undefined,
    layout: resolvePromptLayout(detail.layout, fields),
    fields,
    confirmLabel,
    cancelLabel,
    width
  };
}

function buildNativePromptMessage(detail: PromptMessage, field: PromptFieldDetail): string {
  return [detail.title, detail.subtitle, detail.description, field.label, field.description].filter(Boolean).join("\n\n");
}

async function fallbackPrompt(detail: PromptMessage): Promise<PromptValues | null> {
  if (typeof window === "undefined") {
    return null;
  }

  const values: PromptValues = {};
  for (const field of detail.fields) {
    const next = window.prompt(buildNativePromptMessage(detail, field), field.defaultValue || "");
    if (next === null) {
      return null;
    }
    values[field.key] = next;
  }
  return values;
}

function collectCompactPromptNotes(message: PromptMessage | null): string[] {
  if (!message || message.layout !== "compact" || message.fields.length !== 1) {
    return [];
  }

  const notes = [
    message.description && message.subtitle ? message.subtitle : "",
    message.fields[0]?.description
  ]
    .map((item) => String(item || "").trim())
    .filter(Boolean);

  return Array.from(new Set(notes));
}

export function emitPrompt(detail: PromptEventDetail): Promise<PromptValues | null> {
  const normalized = normalizePromptDetail(detail);
  if (typeof window === "undefined") {
    return Promise.resolve(null);
  }

  return new Promise((resolve) => {
    const event = new CustomEvent<PromptEventDetail>(PROMPT_EVENT_NAME, {
      detail: {
        ...normalized,
        resolve
      },
      cancelable: true
    });
    window.dispatchEvent(event);
    if (!event.defaultPrevented) {
      void fallbackPrompt(normalized).then(resolve);
    }
  });
}

function buildInitialValues(fields: PromptFieldDetail[]): PromptValues {
  return fields.reduce<PromptValues>((acc, field) => {
    acc[field.key] = String(field.defaultValue || "");
    return acc;
  }, {});
}

export function PromptProvider({ children }: { children: ReactNode }): JSX.Element {
  const [queue, setQueue] = useState<PromptQueueItem[]>([]);
  const [values, setValues] = useState<PromptValues>({});
  const [detailsExpanded, setDetailsExpanded] = useState(false);
  const queueRef = useRef<PromptQueueItem[]>([]);
  const fieldRefs = useRef<Map<string, HTMLInputElement>>(new Map());
  const formId = useId();

  useEffect(() => {
    queueRef.current = queue;
  }, [queue]);

  const settle = useCallback((nextValue: PromptValues | null) => {
    const current = queueRef.current[0];
    if (!current) {
      return;
    }
    current.resolve(nextValue);
    setQueue((prev) => prev.slice(1));
  }, []);

  useEffect(() => {
    const onPrompt = (event: Event): void => {
      const customEvent = event as CustomEvent<PromptEventDetail>;
      const resolve = customEvent.detail?.resolve;
      if (typeof resolve !== "function") {
        return;
      }
      customEvent.preventDefault();
      const normalized = normalizePromptDetail(customEvent.detail);
      setQueue((prev) => [
        ...prev,
        {
          ...normalized,
          resolve
        }
      ]);
    };

    window.addEventListener(PROMPT_EVENT_NAME, onPrompt as EventListener);
    return () => {
      window.removeEventListener(PROMPT_EVENT_NAME, onPrompt as EventListener);
      queueRef.current.forEach((item) => item.resolve(null));
      queueRef.current = [];
    };
  }, []);

  const active = queue[0] ?? null;

  useEffect(() => {
    if (!active) {
      setValues({});
      setDetailsExpanded(false);
      fieldRefs.current.clear();
      return;
    }
    setValues(buildInitialValues(active.fields));
    setDetailsExpanded(false);
    fieldRefs.current.clear();
  }, [active]);

  useEffect(() => {
    if (!active) {
      return;
    }
    const firstField = active.fields[0];
    const timer = window.setTimeout(() => {
      const input = fieldRefs.current.get(firstField.key);
      if (!input) {
        return;
      }
      input.focus();
      if (firstField.selectOnFocus) {
        input.select();
      }
    }, 24);
    return () => {
      window.clearTimeout(timer);
    };
  }, [active]);

  const onSubmit = useCallback(
    (event: FormEvent<HTMLFormElement>) => {
      event.preventDefault();
      settle(active ? { ...values } : null);
    },
    [active, settle, values]
  );

  const promptSubtitle = active
    ? active.layout === "panel"
      ? active.subtitle || "请补充后继续"
      : active.description || active.subtitle || "填写后继续"
    : undefined;
  const promptSummary = active?.layout === "panel" ? active.description : undefined;
  const compactPromptNotes = collectCompactPromptNotes(active);

  return (
    <>
      {children}
      {active && typeof document !== "undefined"
        ? createPortal(
            <Dialog
              open
              title={active.title}
              subtitle={promptSubtitle}
              width={active.width}
              className="ui-prompt-dialog"
              maskClassName="ui-prompt-mask"
              showCloseButton={false}
              footer={
                <>
                  <button type="button" className="btn" onClick={() => settle(null)}>
                    {active.cancelLabel}
                  </button>
                  <button type="submit" form={`${formId}-${active.id}`} className="btn ui-prompt-submit">
                    {active.confirmLabel}
                  </button>
                </>
              }
              onClose={() => settle(null)}
            >
              <form id={`${formId}-${active.id}`} className="ui-prompt-form" onSubmit={onSubmit}>
                {promptSummary ? (
                  <div className="ui-prompt-summary">
                    <p>{promptSummary}</p>
                  </div>
                ) : null}
                <div className="ui-prompt-fields">
                  {active.fields.map((field) => (
                    <label key={field.key} className="ui-prompt-field">
                      <span>{field.label}</span>
                      <input
                        ref={(node) => {
                          if (!node) {
                            fieldRefs.current.delete(field.key);
                            return;
                          }
                          fieldRefs.current.set(field.key, node);
                        }}
                        type={field.type || "text"}
                        className="ui-prompt-input"
                        value={values[field.key] ?? ""}
                        placeholder={field.placeholder}
                        autoComplete={field.autoComplete}
                        onChange={(event) => {
                          const nextValue = event.target.value;
                          setValues((prev) => ({
                            ...prev,
                            [field.key]: nextValue
                          }));
                        }}
                      />
                      {active.layout === "panel" && field.description ? <small>{field.description}</small> : null}
                    </label>
                  ))}
                </div>
                {active.layout === "compact" && compactPromptNotes.length ? (
                  <div className="ui-prompt-help">
                    <button
                      type="button"
                      className="ui-prompt-help-toggle"
                      aria-expanded={detailsExpanded}
                      onClick={() => setDetailsExpanded((prev) => !prev)}
                    >
                      {detailsExpanded ? "收起说明" : "查看更多"}
                    </button>
                    {detailsExpanded ? (
                      <div className="ui-prompt-help-panel">
                        {compactPromptNotes.map((note) => (
                          <p key={note}>{note}</p>
                        ))}
                      </div>
                    ) : null}
                  </div>
                ) : null}
              </form>
            </Dialog>,
            document.body
          )
        : null}
    </>
  );
}
