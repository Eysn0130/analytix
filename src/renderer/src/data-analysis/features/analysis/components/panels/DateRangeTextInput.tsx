import {
  ChangeEvent,
  KeyboardEvent,
  MouseEvent as ReactMouseEvent,
  MutableRefObject,
  Ref,
  useCallback,
  useEffect,
  useRef,
  useState
} from "react";

interface DateRangeTextInputProps {
  ariaLabel: string;
  className?: string;
  id?: string;
  inputRef?: Ref<HTMLInputElement>;
  max?: string;
  min?: string;
  name: string;
  onCommit: (value: string) => void;
  value: string;
}

const FULL_DATE_PATTERN = /^\d{4}-\d{2}-\d{2}$/;

function parseIsoDay(text: string): Date | null {
  const trimmed = String(text || "").trim();
  const match = trimmed.match(/^(\d{4})-(\d{2})-(\d{2})$/);
  if (!match) {
    return null;
  }
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  if (!Number.isFinite(year) || !Number.isFinite(month) || !Number.isFinite(day)) {
    return null;
  }
  const next = new Date(year, month - 1, day);
  if (next.getFullYear() !== year || next.getMonth() !== month - 1 || next.getDate() !== day) {
    return null;
  }
  return next;
}

function formatDigitsAsDateDraft(digits: string): string {
  const compact = String(digits || "").replace(/\D/g, "").slice(0, 8);
  if (!compact) {
    return "";
  }
  if (compact.length <= 4) {
    return compact;
  }
  if (compact.length <= 6) {
    return `${compact.slice(0, 4)}-${compact.slice(4)}`;
  }
  return `${compact.slice(0, 4)}-${compact.slice(4, 6)}-${compact.slice(6, 8)}`;
}

function normalizeDraft(text: string): string {
  const compact = String(text || "").replace(/\s+/g, "");
  if (!compact) {
    return "";
  }
  const filtered = compact.replace(/[^\d-]/g, "");
  if (!filtered.includes("-")) {
    return formatDigitsAsDateDraft(filtered);
  }
  return filtered.slice(0, 10);
}

function clampDate(value: string, min?: string, max?: string): string {
  let next = value;
  if (min && next < min) {
    next = min;
  }
  if (max && next > max) {
    next = max;
  }
  return next;
}

function normalizeCommittedValue(draft: string, min?: string, max?: string): string | null {
  const trimmed = String(draft || "").trim();
  if (!trimmed) {
    return "";
  }
  const candidate = FULL_DATE_PATTERN.test(trimmed) ? trimmed : formatDigitsAsDateDraft(trimmed);
  if (!FULL_DATE_PATTERN.test(candidate) || !parseIsoDay(candidate)) {
    return null;
  }
  return clampDate(candidate, min, max);
}

function resolveSegmentRange(position: number): { start: number; end: number } {
  if (position <= 4) {
    return { start: 0, end: 4 };
  }
  if (position <= 7) {
    return { start: 5, end: 7 };
  }
  return { start: 8, end: 10 };
}

function isSegmentSelection(start: number, end: number): boolean {
  return (start === 0 && end === 4) || (start === 5 && end === 7) || (start === 8 && end === 10);
}

function moveSegmentRange(currentStart: number, direction: -1 | 1): { start: number; end: number } {
  if (direction < 0) {
    if (currentStart <= 0) {
      return { start: 0, end: 4 };
    }
    if (currentStart <= 5) {
      return { start: 0, end: 4 };
    }
    return { start: 5, end: 7 };
  }
  if (currentStart < 5) {
    return { start: 5, end: 7 };
  }
  if (currentStart < 8) {
    return { start: 8, end: 10 };
  }
  return { start: 8, end: 10 };
}

function assignInputRef(ref: Ref<HTMLInputElement> | undefined, value: HTMLInputElement | null): void {
  if (!ref) {
    return;
  }
  if (typeof ref === "function") {
    ref(value);
    return;
  }
  (ref as MutableRefObject<HTMLInputElement | null>).current = value;
}

export function DateRangeTextInput(props: DateRangeTextInputProps): JSX.Element {
  const { ariaLabel, className = "", id, inputRef: externalInputRef, max, min, name, onCommit, value } = props;
  const inputRef = useRef<HTMLInputElement | null>(null);
  const [draft, setDraft] = useState(String(value || ""));

  const bindInputRef = useCallback(
    (node: HTMLInputElement | null): void => {
      inputRef.current = node;
      assignInputRef(externalInputRef, node);
    },
    [externalInputRef]
  );

  useEffect(() => {
    if (document.activeElement !== inputRef.current) {
      setDraft(String(value || ""));
    }
  }, [value]);

  const selectSegmentAtCaret = useCallback((input: HTMLInputElement): void => {
    if (!FULL_DATE_PATTERN.test(input.value)) {
      return;
    }
    const start = input.selectionStart ?? 0;
    const end = input.selectionEnd ?? 0;
    if (start !== end) {
      return;
    }
    const range = resolveSegmentRange(start);
    input.setSelectionRange(range.start, range.end);
  }, []);

  const syncSegmentSelection = useCallback(
    (input: HTMLInputElement): void => {
      selectSegmentAtCaret(input);
      window.requestAnimationFrame(() => {
        if (document.activeElement !== input) {
          return;
        }
        selectSegmentAtCaret(input);
      });
    },
    [selectSegmentAtCaret]
  );

  const commitDraft = useCallback(
    (nextDraft: string): void => {
      const normalized = normalizeCommittedValue(nextDraft, min, max);
      if (normalized == null) {
        setDraft(String(value || ""));
        return;
      }
      setDraft(normalized);
      if (normalized !== value) {
        onCommit(normalized);
      }
    },
    [max, min, onCommit, value]
  );

  const handleChange = useCallback(
    (event: ChangeEvent<HTMLInputElement>): void => {
      const nextDraft = normalizeDraft(event.target.value);
      setDraft(nextDraft);
      if (!nextDraft) {
        if (value) {
          onCommit("");
        }
        return;
      }
      const normalized = normalizeCommittedValue(nextDraft, min, max);
      if (normalized == null) {
        return;
      }
      setDraft(normalized);
      if (normalized !== value) {
        onCommit(normalized);
      }
    },
    [max, min, onCommit, value]
  );

  const handleBlur = useCallback((): void => {
    commitDraft(draft);
  }, [commitDraft, draft]);

  const handleMouseUp = useCallback(
    (event: ReactMouseEvent<HTMLInputElement>): void => {
      const input = event.currentTarget;
      syncSegmentSelection(input);
    },
    [syncSegmentSelection]
  );

  const handleKeyDown = useCallback(
    (event: KeyboardEvent<HTMLInputElement>): void => {
      const input = event.currentTarget;
      const selectionStart = input.selectionStart ?? 0;
      const selectionEnd = input.selectionEnd ?? 0;
      const canSegmentSelect = FULL_DATE_PATTERN.test(draft);

      if ((event.key === "Backspace" || event.key === "Delete") && canSegmentSelect) {
        if ((selectionStart === 0 && selectionEnd === draft.length) || isSegmentSelection(selectionStart, selectionEnd)) {
          event.preventDefault();
          setDraft("");
          if (value) {
            onCommit("");
          }
          window.requestAnimationFrame(() => {
            input.setSelectionRange(0, 0);
          });
          return;
        }
      }

      if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
        if (!canSegmentSelect) {
          return;
        }
        if (selectionStart !== selectionEnd && !isSegmentSelection(selectionStart, selectionEnd)) {
          return;
        }
        event.preventDefault();
        const range = moveSegmentRange(selectionStart, event.key === "ArrowLeft" ? -1 : 1);
        input.setSelectionRange(range.start, range.end);
        return;
      }

      if (event.key === "Enter") {
        event.preventDefault();
        commitDraft(draft);
        input.blur();
        return;
      }

      if (event.key === "Escape") {
        event.preventDefault();
        setDraft(String(value || ""));
        window.requestAnimationFrame(() => {
          if (FULL_DATE_PATTERN.test(String(value || ""))) {
            const range = resolveSegmentRange(selectionStart);
            input.setSelectionRange(range.start, range.end);
          }
        });
      }
    },
    [commitDraft, draft, onCommit, value]
  );

  return (
    <input
      id={id}
      ref={bindInputRef}
      aria-label={ariaLabel}
      autoComplete="off"
      className={className}
      data-max={max || undefined}
      data-min={min || undefined}
      inputMode="numeric"
      name={name}
      spellCheck={false}
      type="text"
      value={draft}
      onBlur={handleBlur}
      onChange={handleChange}
      onFocus={(event) => syncSegmentSelection(event.currentTarget)}
      onKeyDown={handleKeyDown}
      onMouseUp={handleMouseUp}
    />
  );
}
