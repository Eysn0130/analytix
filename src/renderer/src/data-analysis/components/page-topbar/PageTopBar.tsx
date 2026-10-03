import { Sun, Moon, Bell } from "../../../design/AnalytixUiIcons";
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
      <Sun aria-hidden="true" focusable="false" />
    );
  }

  return (
    <Moon aria-hidden="true" focusable="false" />
  );
}

function BellIcon(): JSX.Element {
  return (
    <Bell aria-hidden="true" focusable="false" />
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
