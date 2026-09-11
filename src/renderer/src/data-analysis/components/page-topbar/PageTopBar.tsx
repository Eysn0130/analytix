import type { ReactNode } from "react";
import { useAppStore } from "../../store/app-store";
import "./page-topbar.css";

export interface PageTopBarTab {
  key: string;
  label: string;
  active?: boolean;
  onClick?: () => void;
  disabled?: boolean;
}

export interface PageTopBarNotification {
  active?: boolean;
  badge?: string;
  ariaLabel: string;
  title: string;
  onClick: () => void;
}

export interface PageTopBarTool {
  key: string;
  icon: ReactNode;
  ariaLabel: string;
  title: string;
  onClick: () => void;
  disabled?: boolean;
  tone?: "default" | "alert";
  badge?: string;
  className?: string;
}

interface PageTopBarProps {
  className?: string;
  tabs?: PageTopBarTab[];
  tabsAriaLabel?: string;
  notification?: PageTopBarNotification | null;
  extraTools?: PageTopBarTool[];
  middleContent?: ReactNode;
  middleClassName?: string;
  toolsContent?: ReactNode;
  hideThemeToggle?: boolean;
}

function ThemeIcon({ dark }: { dark: boolean }): JSX.Element {
  if (dark) {
    return (
      <svg viewBox="0 0 20 20" fill="none" focusable="false">
        <circle cx="10" cy="10" r="3.15" stroke="currentColor" strokeWidth="1.5" />
        <path d="M10 2.8V4.3" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
        <path d="M10 15.7V17.2" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
        <path d="M2.8 10H4.3" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
        <path d="M15.7 10H17.2" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
        <path d="M4.95 4.95L6 6" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
        <path d="M14 14L15.05 15.05" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
        <path d="M14 6L15.05 4.95" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
        <path d="M4.95 15.05L6 14" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
      </svg>
    );
  }

  return (
    <svg viewBox="0 0 20 20" fill="none" focusable="false">
      <path
        d="M12.7 3.1A6.8 6.8 0 1 0 16.9 14.6A7.1 7.1 0 0 1 12.7 3.1Z"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinejoin="round"
      />
    </svg>
  );
}

function BellIcon(): JSX.Element {
  return (
    <svg viewBox="0 0 20 20" fill="none" focusable="false">
      <path
        d="M10 3.3C7.93 3.3 6.25 4.98 6.25 7.05V8.86C6.25 9.45 6.05 10.03 5.69 10.49L4.86 11.57C4.3 12.29 4.81 13.35 5.73 13.35H14.27C15.19 13.35 15.7 12.29 15.14 11.57L14.31 10.49C13.95 10.03 13.75 9.45 13.75 8.86V7.05C13.75 4.98 12.07 3.3 10 3.3Z"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinejoin="round"
      />
      <path
        d="M8.35 15.15C8.63 15.9 9.25 16.35 10 16.35C10.75 16.35 11.37 15.9 11.65 15.15"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
      />
    </svg>
  );
}

function renderToolButton(tool: PageTopBarTool): JSX.Element {
  const className = [
    "page-topbar__tool",
    tool.tone === "alert" ? "is-alert" : "",
    tool.className ?? ""
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <button
      key={tool.key}
      type="button"
      className={className}
      aria-label={tool.ariaLabel}
      title={tool.title}
      onClick={tool.onClick}
      disabled={tool.disabled}
    >
      {tool.icon}
      {tool.badge ? (
        <span className="page-topbar__badge" aria-hidden="true">
          {tool.badge}
        </span>
      ) : null}
    </button>
  );
}

export function PageTopBar(props: PageTopBarProps): JSX.Element {
  const {
    className = "",
    tabs = [],
    tabsAriaLabel = "页面切换",
    notification,
    extraTools = [],
    middleContent,
    middleClassName = "",
    toolsContent,
    hideThemeToggle = false
  } = props;
  const {
    state: { resolvedTheme },
    actions
  } = useAppStore();

  const toggleThemeLabel = resolvedTheme === "dark" ? "切换为浅色主题" : "切换为深色主题";
  const hasTabs = tabs.length > 0;
  const hasInteractiveTabs = tabs.some((tab) => typeof tab.onClick === "function");
  const hasTools = Boolean(toolsContent) || !hideThemeToggle || extraTools.length > 0 || Boolean(notification);
  const rootClassName = ["page-topbar", className].filter(Boolean).join(" ");

  return (
    <header className={rootClassName}>
      {hasTabs ? (
        <nav className="page-topbar__tabs" aria-label={tabsAriaLabel} role={hasInteractiveTabs ? "tablist" : undefined}>
          {tabs.map((tab) => {
            const interactive = typeof tab.onClick === "function";
            const tabClassName = [
              "page-topbar__tab",
              tab.active ? "is-active" : "",
              !interactive ? "is-static" : ""
            ]
              .filter(Boolean)
              .join(" ");

            if (!interactive) {
              return (
                <div key={tab.key} className={tabClassName} role={hasInteractiveTabs ? "presentation" : undefined} aria-current={tab.active ? "page" : undefined}>
                  {tab.label}
                </div>
              );
            }

            return (
              <button
                key={tab.key}
                type="button"
                role="tab"
                aria-selected={Boolean(tab.active)}
                className={tabClassName}
                onClick={tab.onClick}
                disabled={tab.disabled}
              >
                {tab.label}
              </button>
            );
          })}
        </nav>
      ) : null}

      {middleContent ? <div className={["page-topbar__middle", middleClassName].filter(Boolean).join(" ")}>{middleContent}</div> : null}

      <div className="page-topbar__drag-region" aria-hidden="true" />

      {hasTools ? (
        <div className="page-topbar__tools" role="group" aria-label="页面工具">
          {toolsContent}

          {!hideThemeToggle ? (
            <button
              type="button"
              className="page-topbar__tool"
              aria-label={toggleThemeLabel}
              title={toggleThemeLabel}
              onClick={() => actions.setThemeMode(resolvedTheme === "dark" ? "light" : "dark")}
            >
              <ThemeIcon dark={resolvedTheme === "dark"} />
            </button>
          ) : null}

          {extraTools.map((tool) => renderToolButton(tool))}

          {notification ? (
            <button
              type="button"
              className={`page-topbar__tool${notification.active ? " is-alert" : ""}`}
              aria-label={notification.ariaLabel}
              title={notification.title}
              onClick={notification.onClick}
            >
              <BellIcon />
              {notification.badge ? (
                <span className="page-topbar__badge" aria-hidden="true">
                  {notification.badge}
                </span>
              ) : null}
            </button>
          ) : null}
        </div>
      ) : null}
    </header>
  );
}
