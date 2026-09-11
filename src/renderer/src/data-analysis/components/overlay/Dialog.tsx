import { ReactNode, useEffect } from "react";

interface DialogProps {
  open: boolean;
  title: string;
  subtitle?: string;
  headerExtra?: ReactNode;
  headerContentClassName?: string;
  width?: number;
  className?: string;
  maskClassName?: string;
  children: ReactNode;
  footer?: ReactNode;
  bodyContentClassName?: string;
  footerContentClassName?: string;
  closeLabel?: string;
  closeAriaLabel?: string;
  showCloseButton?: boolean;
  onClose: () => void;
}

export function Dialog({
  open,
  title,
  subtitle,
  headerExtra,
  headerContentClassName,
  width = 560,
  className,
  maskClassName,
  children,
  footer,
  bodyContentClassName,
  footerContentClassName,
  closeLabel = "关闭",
  closeAriaLabel,
  showCloseButton = true,
  onClose
}: DialogProps): JSX.Element | null {
  useEffect(() => {
    if (!open) {
      return;
    }

    const onKeyDown = (event: KeyboardEvent): void => {
      if (event.key === "Escape") {
        onClose();
      }
    };

    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [open, onClose]);

  if (!open) {
    return null;
  }

  const headerContent = (
    <>
      <div className="stack-gap-sm">
        <h3>{title}</h3>
        {subtitle ? <p>{subtitle}</p> : null}
      </div>
      <div className="ui-dialog-header__actions">
        {headerExtra}
        {showCloseButton ? (
          <button type="button" className="btn btn-xs" onClick={onClose} aria-label={closeAriaLabel || closeLabel}>
            {closeLabel}
          </button>
        ) : null}
      </div>
    </>
  );

  const bodyContent = bodyContentClassName ? (
    <div className={bodyContentClassName}>{children}</div>
  ) : (
    children
  );

  const footerContent = footer && footerContentClassName ? (
    <div className={footerContentClassName}>{footer}</div>
  ) : footer;

  return (
    <div
      className={`ui-dialog-mask${maskClassName ? ` ${maskClassName}` : ""}`}
      onMouseDown={onClose}
      role="presentation"
    >
      <section
        className={`ui-dialog${className ? ` ${className}` : ""}`}
        style={{ width: `${Math.max(360, Math.min(width, window.innerWidth - 32))}px` }}
        onMouseDown={(event) => event.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-label={title}
      >
        <header className="ui-dialog-header">
          {headerContentClassName ? (
            <div className={headerContentClassName}>{headerContent}</div>
          ) : (
            headerContent
          )}
        </header>

        <div className="ui-dialog-body">{bodyContent}</div>

        {footer ? <footer className="ui-dialog-footer">{footerContent}</footer> : null}
      </section>
    </div>
  );
}
