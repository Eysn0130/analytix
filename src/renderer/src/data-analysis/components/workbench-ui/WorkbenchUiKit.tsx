import {
  ButtonHTMLAttributes,
  CSSProperties,
  HTMLAttributes,
  InputHTMLAttributes,
  KeyboardEvent,
  ReactNode,
  useLayoutEffect,
  useRef
} from "react";
import "./workbench-ui-kit.css";

export const WORKBENCH_UI_TOKEN_NAMES = {
  text: "--workbench-ui-text",
  textMuted: "--workbench-ui-text-muted",
  surface: "--workbench-ui-surface",
  surfaceMuted: "--workbench-ui-surface-muted",
  border: "--workbench-ui-border",
  accentStart: "--workbench-ui-accent-start",
  accentEnd: "--workbench-ui-accent-end",
  tableHeadBg: "--workbench-ui-table-head-bg"
} as const;

type WorkbenchButtonTone = "neutral" | "accent" | "ghost" | "danger";
type WorkbenchButtonSize = "md" | "sm";
type WorkbenchMetricTone = "positive" | "negative" | "neutral";
type WorkbenchMetricCardElement = "article" | "button" | "div";

function cx(...tokens: Array<string | false | null | undefined>): string {
  return tokens.filter(Boolean).join(" ");
}

export interface WorkbenchButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  tone?: WorkbenchButtonTone;
  size?: WorkbenchButtonSize;
  badge?: ReactNode;
  block?: boolean;
  loading?: boolean;
}

export function WorkbenchButton({
  tone = "neutral",
  size = "md",
  badge,
  block = false,
  loading = false,
  className,
  children,
  disabled,
  ...props
}: WorkbenchButtonProps): JSX.Element {
  return (
    <button
      className={cx(
        "workbench-ui-btn",
        `workbench-ui-btn--${tone}`,
        `workbench-ui-btn--${size}`,
        Boolean(badge) && "workbench-ui-btn--with-badge",
        block && "workbench-ui-btn--block",
        loading && "workbench-ui-btn--loading",
        tone === "accent" && "primary",
        tone === "ghost" && "ghost",
        tone === "danger" && "danger",
        className
      )}
      disabled={disabled || loading}
      aria-busy={loading || undefined}
      {...props}
    >
      {loading ? (
        <span className="workbench-ui-btn__spinner" aria-hidden="true">
          <span />
        </span>
      ) : null}
      <span className="workbench-ui-btn__content">{children}</span>
      {!loading && badge ? <span className="workbench-ui-btn__badge">{badge}</span> : null}
    </button>
  );
}

export interface WorkbenchToolbarButtonProps extends Omit<WorkbenchButtonProps, "size"> {}

export function WorkbenchToolbarButton({
  className,
  ...props
}: WorkbenchToolbarButtonProps): JSX.Element {
  return (
    <WorkbenchButton
      size="sm"
      className={cx("workbench-ui-toolbar-btn", className)}
      {...props}
    />
  );
}

export interface WorkbenchSegmentedOption<ValueT extends string> {
  value: ValueT;
  label: ReactNode;
  disabled?: boolean;
}

export interface WorkbenchSegmentedControlProps<ValueT extends string> {
  value: ValueT | null | undefined;
  options: Array<WorkbenchSegmentedOption<ValueT>>;
  onChange: (value: ValueT) => void;
  ariaLabel: string;
  className?: string;
  optionClassName?: string;
  indicatorClassName?: string;
}

export function WorkbenchSegmentedControl<ValueT extends string>({
  value,
  options,
  onChange,
  ariaLabel,
  className,
  optionClassName,
  indicatorClassName
}: WorkbenchSegmentedControlProps<ValueT>): JSX.Element {
  const indicatorRef = useRef<HTMLSpanElement | null>(null);
  const optionRefs = useRef<Array<HTMLButtonElement | null>>([]);
  const isFirstPaintRef = useRef(true);
  const prevIndexRef = useRef(0);
  const optionCount = Math.max(options.length, 1);
  const rawActiveIndex = options.findIndex((option) => option.value === value);
  const activeIndex = Math.max(0, rawActiveIndex);
  const hasActiveOption = rawActiveIndex >= 0;
  const firstEnabledIndex = options.findIndex((option) => !option.disabled);
  const focusableIndex = rawActiveIndex >= 0 && !options[rawActiveIndex]?.disabled ? rawActiveIndex : Math.max(firstEnabledIndex, 0);
  const style = {
    gridTemplateColumns: `repeat(${optionCount}, minmax(0, 1fr))`,
    "--workbench-ui-segment-count": String(optionCount),
    "--workbench-ui-active-index": String(activeIndex),
    "--workbench-ui-indicator-opacity": hasActiveOption ? "1" : "0"
  } as CSSProperties;

  const findNextEnabledIndex = (startIndex: number, direction: 1 | -1): number => {
    if (!options.length) {
      return startIndex;
    }
    let candidate = startIndex;
    for (let checked = 0; checked < options.length; checked += 1) {
      candidate = (candidate + direction + options.length) % options.length;
      if (!options[candidate]?.disabled) {
        return candidate;
      }
    }
    return startIndex;
  };

  const focusOption = (index: number): void => {
    optionRefs.current[index]?.focus();
  };

  const selectOptionAt = (index: number): void => {
    const nextOption = options[index];
    if (!nextOption || nextOption.disabled) {
      return;
    }
    onChange(nextOption.value);
    focusOption(index);
  };

  const onOptionKeyDown = (event: KeyboardEvent<HTMLButtonElement>, index: number): void => {
    if (!options.length) {
      return;
    }
    if (event.key === "ArrowRight" || event.key === "ArrowDown") {
      event.preventDefault();
      selectOptionAt(findNextEnabledIndex(index, 1));
      return;
    }
    if (event.key === "ArrowLeft" || event.key === "ArrowUp") {
      event.preventDefault();
      selectOptionAt(findNextEnabledIndex(index, -1));
      return;
    }
    if (event.key === "Home") {
      event.preventDefault();
      if (firstEnabledIndex >= 0) {
        selectOptionAt(firstEnabledIndex);
      }
      return;
    }
    if (event.key === "End") {
      event.preventDefault();
      for (let candidate = options.length - 1; candidate >= 0; candidate -= 1) {
        if (!options[candidate]?.disabled) {
          selectOptionAt(candidate);
          break;
        }
      }
    }
  };

  useLayoutEffect(() => {
    const indicator = indicatorRef.current;
    const previousIndex = prevIndexRef.current;
    prevIndexRef.current = activeIndex;

    if (!indicator || !hasActiveOption) {
      return;
    }
    if (isFirstPaintRef.current) {
      isFirstPaintRef.current = false;
      return;
    }
    if (previousIndex === activeIndex || typeof indicator.animate !== "function") {
      return;
    }
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
      return;
    }

    const indicatorWidth = indicator.getBoundingClientRect().width;
    if (!indicatorWidth) {
      return;
    }

    const from = previousIndex * indicatorWidth;
    const to = activeIndex * indicatorWidth;
    const direction = to >= from ? 1 : -1;
    const overshoot = Math.min(16, Math.max(8, indicatorWidth * 0.14)) * direction;
    const recoil = Math.min(7, Math.max(3, indicatorWidth * 0.06)) * direction;

    const animation = indicator.animate(
      [
        {
          transform: `translate3d(${from}px, 0, 0) scaleX(0.985)`
        },
        {
          offset: 0.62,
          transform: `translate3d(${to + overshoot}px, 0, 0) scaleX(1.028)`
        },
        {
          offset: 0.8,
          transform: `translate3d(${to - recoil}px, 0, 0) scaleX(0.996)`
        },
        {
          transform: `translate3d(${to}px, 0, 0) scaleX(1)`
        }
      ],
      {
        duration: 520,
        easing: "cubic-bezier(0.22, 1, 0.36, 1)",
        fill: "none"
      }
    );

    return () => {
      animation.cancel();
    };
  }, [activeIndex, hasActiveOption]);

  return (
    <div className={cx("workbench-ui-segmented", className)} style={style} role="radiogroup" aria-label={ariaLabel} aria-orientation="horizontal">
      <span ref={indicatorRef} className={cx("workbench-ui-segmented__indicator", indicatorClassName)} aria-hidden="true" />
      {options.map((option, index) => {
        const active = option.value === value;
        return (
          <button
            key={option.value}
            ref={(node) => {
              optionRefs.current[index] = node;
            }}
            className={cx(
              "workbench-ui-segmented__option",
              active && "workbench-ui-segmented__option--active",
              active && "active",
              optionClassName
            )}
            role="radio"
            type="button"
            aria-checked={active}
            tabIndex={index === focusableIndex ? 0 : -1}
            disabled={option.disabled}
            onKeyDown={(event) => onOptionKeyDown(event, index)}
            onClick={() => onChange(option.value)}
          >
            {option.label}
          </button>
        );
      })}
    </div>
  );
}

export interface WorkbenchModeCardProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  active?: boolean;
}

export function WorkbenchModeCard({ active = false, className, children, ...props }: WorkbenchModeCardProps): JSX.Element {
  return (
    <button className={cx("workbench-ui-mode-card", active && "workbench-ui-mode-card--active", className)} {...props}>
      {children}
    </button>
  );
}

export interface WorkbenchMetricStripProps extends HTMLAttributes<HTMLDivElement> {
  columns?: number;
  tabletColumns?: number;
}

export function WorkbenchMetricStrip({
  columns = 4,
  tabletColumns = Math.min(columns, 2),
  className,
  style,
  children,
  ...props
}: WorkbenchMetricStripProps): JSX.Element {
  const mergedStyle = {
    ...style,
    "--workbench-ui-metric-columns": String(Math.max(columns, 1)),
    "--workbench-ui-metric-columns-tablet": String(Math.max(Math.min(columns, tabletColumns), 1))
  } as CSSProperties;

  return (
    <div className={cx("workbench-ui-metric-strip", className)} style={mergedStyle} {...props}>
      {children}
    </div>
  );
}

export interface WorkbenchMetricCardProps extends HTMLAttributes<HTMLElement> {
  as?: WorkbenchMetricCardElement;
  active?: boolean;
  buttonType?: ButtonHTMLAttributes<HTMLButtonElement>["type"];
  icon?: ReactNode;
  label: ReactNode;
  secondaryLabel?: ReactNode;
  value: ReactNode;
  description?: ReactNode;
  metaAccent?: ReactNode;
  metaTail?: ReactNode;
  trendTone?: WorkbenchMetricTone;
}

export function WorkbenchMetricCard({
  as = "article",
  active = false,
  buttonType = "button",
  icon,
  label,
  secondaryLabel,
  value,
  description,
  metaAccent,
  metaTail,
  trendTone = "neutral",
  className,
  children,
  ...props
}: WorkbenchMetricCardProps): JSX.Element {
  const Component = as;
  const elementProps = {
    ...props,
    className: cx(
      "workbench-ui-metric-card",
      as === "button" && "workbench-ui-metric-card--interactive",
      active && "workbench-ui-metric-card--active",
      className
    )
  } as Record<string, unknown>;

  if (Component === "button") {
    elementProps.type = buttonType;
  }

  const componentProps = elementProps as HTMLAttributes<HTMLElement> & ButtonHTMLAttributes<HTMLButtonElement>;

  return (
    <Component {...componentProps}>
      <div className="workbench-ui-metric-card__head">
        {icon ? <span className="workbench-ui-metric-card__icon" aria-hidden="true">{icon}</span> : null}
        <span className="workbench-ui-metric-card__label-group">
          <span className="workbench-ui-metric-card__label">{label}</span>
          {secondaryLabel ? (
            <span className="workbench-ui-metric-card__secondary">{secondaryLabel}</span>
          ) : null}
        </span>
      </div>
      <strong className="workbench-ui-metric-card__value">{value}</strong>
      {description ? <p className="workbench-ui-metric-card__description">{description}</p> : null}
      {(metaAccent || metaTail) ? (
        <span className={cx("workbench-ui-metric-card__meta", `is-${trendTone}`)}>
          {metaAccent ? (
            <span className="workbench-ui-metric-card__meta-accent">{metaAccent}</span>
          ) : null}
          {metaTail ? (
            <span className="workbench-ui-metric-card__meta-tail">{metaTail}</span>
          ) : null}
        </span>
      ) : null}
      {children}
    </Component>
  );
}

export function WorkbenchTableShell({ className, children, ...props }: HTMLAttributes<HTMLElement>): JSX.Element {
  return (
    <section className={cx("workbench-ui-table-shell", className)} {...props}>
      {children}
    </section>
  );
}

export function WorkbenchTableToolbar({ className, children, ...props }: HTMLAttributes<HTMLDivElement>): JSX.Element {
  return (
    <div className={cx("workbench-ui-table-toolbar", className)} {...props}>
      {children}
    </div>
  );
}

export function WorkbenchTableBody({ className, children, ...props }: HTMLAttributes<HTMLDivElement>): JSX.Element {
  return (
    <div className={cx("workbench-ui-table-body", className)} {...props}>
      {children}
    </div>
  );
}

export function WorkbenchTableActions({ className, children, ...props }: HTMLAttributes<HTMLDivElement>): JSX.Element {
  return (
    <div className={cx("workbench-ui-table-actions", className)} {...props}>
      {children}
    </div>
  );
}

export function WorkbenchTableFooter({ className, children, ...props }: HTMLAttributes<HTMLDivElement>): JSX.Element {
  return (
    <div className={cx("workbench-ui-table-footer", className)} {...props}>
      {children}
    </div>
  );
}

export interface WorkbenchSearchFieldProps extends Omit<InputHTMLAttributes<HTMLInputElement>, "className" | "onChange" | "value"> {
  value: string;
  onChange: (value: string) => void;
  className?: string;
  inputClassName?: string;
  clearClassName?: string;
  clearLabel?: string;
  wrapperRole?: HTMLAttributes<HTMLDivElement>["role"];
}

export function WorkbenchSearchField({
  value,
  onChange,
  className,
  inputClassName,
  clearClassName,
  clearLabel = "清空搜索",
  wrapperRole,
  disabled,
  ...props
}: WorkbenchSearchFieldProps): JSX.Element {
  return (
    <div className={cx("workbench-ui-search-field", className)} role={wrapperRole}>
      <input
        className={cx("workbench-ui-search-field__input", inputClassName)}
        value={value}
        disabled={disabled}
        onChange={(event) => onChange(event.target.value)}
        {...props}
      />
      <button
        className={cx("workbench-ui-search-field__clear", clearClassName)}
        title={clearLabel}
        type="button"
        aria-label={clearLabel}
        disabled={disabled || !value}
        onClick={() => onChange("")}
      >
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" aria-hidden="true">
          <path d="M7 7L17 17" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
          <path d="M17 7L7 17" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
        </svg>
      </button>
    </div>
  );
}

export interface WorkbenchToolbarSearchFieldProps extends WorkbenchSearchFieldProps {}

export function WorkbenchToolbarSearchField({
  className,
  inputClassName,
  clearClassName,
  ...props
}: WorkbenchToolbarSearchFieldProps): JSX.Element {
  return (
    <WorkbenchSearchField
      className={cx("workbench-ui-toolbar-search-field", className)}
      inputClassName={cx("workbench-ui-toolbar-search-field__input", inputClassName)}
      clearClassName={cx("workbench-ui-toolbar-search-field__clear", clearClassName)}
      {...props}
    />
  );
}
