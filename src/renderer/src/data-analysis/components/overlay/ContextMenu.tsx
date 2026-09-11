import { useEffect, useMemo, useRef, type KeyboardEvent as ReactKeyboardEvent, type PointerEvent, type ReactNode } from "react";

export interface ContextMenuActionItem {
  id: string;
  type?: "item";
  icon?: ReactNode;
  label: string;
  nativeIcon?: string;
  nativeLabel?: string;
  nativeTooltip?: string;
  shortcut?: string;
  danger?: boolean;
  disabled?: boolean;
  submenu?: ContextMenuItem[];
  onSelect?: () => void;
}

export interface ContextMenuSeparatorItem {
  id: string;
  type: "separator";
}

export type ContextMenuItem = ContextMenuActionItem | ContextMenuSeparatorItem;

interface ContextMenuProps {
  open: boolean;
  anchor: { x: number; y: number } | null;
  items: ContextMenuItem[];
  onClose: () => void;
  className?: string;
  closeOnScroll?: boolean;
  deferScrollCloseUntilStable?: boolean;
  preferNativeBridge?: boolean;
}

type NativeContextMenuTemplateItem =
  | { id: string; type: "separator"; label: string }
  | {
      id: string;
      type?: "item";
      label: string;
      icon?: string;
      enabled: boolean;
      toolTip?: string;
      submenu?: NativeContextMenuTemplateItem[];
    };

function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value));
}

function getNativeContextMenuBridge():
  | ((template: NativeContextMenuTemplateItem[]) => Promise<{ id: string | null } | null | undefined>)
  | null {
  if (typeof window === "undefined") {
    return null;
  }
  const showContextMenu = window.electronBridge?.showContextMenu;
  return typeof showContextMenu === "function"
    ? (showContextMenu as (
        template: NativeContextMenuTemplateItem[],
      ) => Promise<{ id: string | null } | null | undefined>)
    : null;
}

function toNativeContextMenuTemplate(items: ContextMenuItem[]): NativeContextMenuTemplateItem[] {
  return items.map((item) => {
    if (item.type === "separator") {
      return {
        id: item.id,
        type: "separator",
        label: ""
      };
    }
    return {
      id: item.id,
      type: item.type,
      label: item.nativeLabel ?? item.label,
      icon: item.nativeIcon,
      enabled: item.disabled !== true,
      toolTip: item.nativeTooltip,
      submenu: item.submenu ? toNativeContextMenuTemplate(item.submenu) : undefined
    };
  });
}

function findContextMenuActionItem(
  items: ContextMenuItem[],
  selectedId: string,
): ContextMenuActionItem | null {
  for (const item of items) {
    if (item.type === "separator") {
      continue;
    }
    if (item.id === selectedId) {
      return item;
    }
    const submenuMatch = item.submenu
      ? findContextMenuActionItem(item.submenu, selectedId)
      : null;
    if (submenuMatch) {
      return submenuMatch;
    }
  }
  return null;
}

export function ContextMenu({
  open,
  anchor,
  items,
  onClose,
  className,
  closeOnScroll = true,
  deferScrollCloseUntilStable = false,
  preferNativeBridge = false,
}: ContextMenuProps): JSX.Element | null {
  const menuRef = useRef<HTMLDivElement | null>(null);
  const nativeMenuOpenRef = useRef(false);
  const latestItemsRef = useRef(items);
  const latestOnCloseRef = useRef(onClose);
  const nativeContextMenuBridge = preferNativeBridge ? getNativeContextMenuBridge() : null;

  useEffect(() => {
    latestItemsRef.current = items;
    latestOnCloseRef.current = onClose;
  }, [items, onClose]);

  useEffect(() => {
    if (!open || !anchor || !preferNativeBridge || nativeMenuOpenRef.current) {
      if (!open) {
        nativeMenuOpenRef.current = false;
      }
      return;
    }

    const showContextMenu = getNativeContextMenuBridge();
    if (!showContextMenu) {
      return;
    }

    let cancelled = false;
    nativeMenuOpenRef.current = true;
    void showContextMenu(toNativeContextMenuTemplate(latestItemsRef.current))
      .then((result) => {
        if (cancelled) {
          return;
        }
        const selectedId = typeof result?.id === "string" ? result.id : "";
        const selected = selectedId ? findContextMenuActionItem(latestItemsRef.current, selectedId) : null;
        if (selected && !selected.disabled) {
          selected.onSelect?.();
        }
      })
      .catch(() => undefined)
      .finally(() => {
        nativeMenuOpenRef.current = false;
        if (!cancelled) {
          latestOnCloseRef.current();
        }
      });

    return () => {
      cancelled = true;
      nativeMenuOpenRef.current = false;
    };
  }, [anchor, open, preferNativeBridge]);

  useEffect(() => {
    if (!open || nativeContextMenuBridge) {
      return;
    }

    let closeOnScrollEnabled = closeOnScroll && !deferScrollCloseUntilStable;
    let nestedScrollEnableFrame: number | null = null;
    const scrollEnableFrame = closeOnScroll && deferScrollCloseUntilStable
      ? window.requestAnimationFrame(() => {
          nestedScrollEnableFrame = window.requestAnimationFrame(() => {
            closeOnScrollEnabled = true;
          });
        })
      : null;

    const onPointerDown = (): void => {
      onClose();
    };

    const onKeyDown = (event: globalThis.KeyboardEvent): void => {
      if (event.key === "Escape") {
        onClose();
      }
    };

    const onScroll = (): void => {
      if (!closeOnScroll || !closeOnScrollEnabled) {
        return;
      }
      onClose();
    };

    window.addEventListener("pointerdown", onPointerDown);
    window.addEventListener("keydown", onKeyDown);
    window.addEventListener("scroll", onScroll, true);

    return () => {
      if (scrollEnableFrame != null) {
        window.cancelAnimationFrame(scrollEnableFrame);
      }
      if (nestedScrollEnableFrame != null) {
        window.cancelAnimationFrame(nestedScrollEnableFrame);
      }
      window.removeEventListener("pointerdown", onPointerDown);
      window.removeEventListener("keydown", onKeyDown);
      window.removeEventListener("scroll", onScroll, true);
    };
  }, [closeOnScroll, deferScrollCloseUntilStable, nativeContextMenuBridge, open, onClose]);

  useEffect(() => {
    if (!open || nativeContextMenuBridge) {
      return;
    }
    const frame = window.requestAnimationFrame(() => {
      menuRef.current?.querySelector<HTMLButtonElement>("[data-context-menu-item]:not(:disabled)")?.focus();
    });
    return () => window.cancelAnimationFrame(frame);
  }, [nativeContextMenuBridge, open, items]);

  const position = useMemo(() => {
    if (!anchor) {
      return { left: 0, top: 0 };
    }

    const estimatedWidth = 224;
    const estimatedHeight = Math.max(132, items.length * 38 + 12);
    const padding = 12;

    return {
      left: clamp(anchor.x, padding, window.innerWidth - estimatedWidth - padding),
      top: clamp(anchor.y, padding, window.innerHeight - estimatedHeight - padding)
    };
  }, [anchor, items.length]);

  function focusMenuItem(direction: "first" | "last" | "next" | "previous"): void {
    const buttons = Array.from(
      menuRef.current?.querySelectorAll<HTMLButtonElement>("[data-context-menu-item]:not(:disabled)") ?? []
    );
    if (!buttons.length) {
      return;
    }
    const currentIndex = buttons.indexOf(document.activeElement as HTMLButtonElement);
    const nextIndex =
      direction === "first"
        ? 0
        : direction === "last"
          ? buttons.length - 1
          : direction === "next"
            ? currentIndex < 0
              ? 0
              : (currentIndex + 1) % buttons.length
            : currentIndex < 0
              ? buttons.length - 1
              : (currentIndex - 1 + buttons.length) % buttons.length;
    buttons[nextIndex]?.focus();
  }

  function handleMenuKeyDown(event: ReactKeyboardEvent<HTMLDivElement>): void {
    switch (event.key) {
      case "ArrowDown":
        event.preventDefault();
        focusMenuItem("next");
        break;
      case "ArrowUp":
        event.preventDefault();
        focusMenuItem("previous");
        break;
      case "Home":
        event.preventDefault();
        focusMenuItem("first");
        break;
      case "End":
        event.preventDefault();
        focusMenuItem("last");
        break;
      case "Escape":
        event.preventDefault();
        onClose();
        break;
    }
  }

  if (!open || !anchor || nativeContextMenuBridge) {
    return null;
  }

  return (
    <div
      ref={menuRef}
      className={["ui-context-menu", className].filter(Boolean).join(" ")}
      style={position}
      onKeyDown={handleMenuKeyDown}
      onPointerDownCapture={(event: PointerEvent<HTMLDivElement>) => {
        event.stopPropagation();
        event.nativeEvent.stopImmediatePropagation();
      }}
      role="menu"
    >
      {items.map((item) => (
        item.type === "separator" ? (
          <div key={item.id} className="ui-context-menu-separator" role="separator" />
        ) : (
          <button
            key={item.id}
            type="button"
            className={`ui-context-menu-item${item.danger ? " danger" : ""}`}
            data-context-menu-item
            role="menuitem"
            disabled={item.disabled}
            onClick={() => {
              if (!item.disabled) {
                item.onSelect?.();
              }
              onClose();
            }}
          >
            <span className="ui-context-menu-item-main">
              {item.icon ? <span className="ui-context-menu-item-icon">{item.icon}</span> : null}
              <span>{item.label}</span>
            </span>
            {item.shortcut ? <span className="code">{item.shortcut}</span> : null}
          </button>
        )
      ))}
    </div>
  );
}
