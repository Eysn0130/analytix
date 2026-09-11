import type { MouseEvent, PointerEvent, ReactElement } from 'react'
import { ArrowLeft, Plus } from 'lucide-react'
import { SidebarTitlebarToggleButton } from '../sidebar/SidebarPrimitives'
import { ShellGuiUpdateAction, shellGuiUpdateActionMode } from './ShellGuiUpdateAction'
import { shellToolbarIconButtonClass, ToolbarTooltip } from './ShellToolbar'

type ShellNavigationControlsProps = {
  leftSidebarCollapsed: boolean
  sidebarLabel: string
  backLabel: string
  forwardLabel: string
  newChatLabel: string
  canGoBack: boolean
  canGoForward: boolean
  newChatDisabled: boolean
  onToggleSidebar: () => void
  onBack: () => void
  onForward: () => void
  onNewChat: () => void
}

function stopShellNavigationEvent(event: MouseEvent<HTMLDivElement> | PointerEvent<HTMLDivElement>): void {
  event.stopPropagation()
}

export function ShellNavigationControls({
  leftSidebarCollapsed,
  sidebarLabel,
  backLabel,
  forwardLabel,
  newChatLabel,
  canGoBack,
  canGoForward,
  newChatDisabled,
  onToggleSidebar,
  onBack,
  onForward,
  onNewChat
}: ShellNavigationControlsProps): ReactElement {
  return (
    <div
      className="ds-shell-navigation-controls ds-no-drag"
      data-left-sidebar-collapsed={leftSidebarCollapsed ? 'true' : 'false'}
      onPointerDown={stopShellNavigationEvent}
      onMouseDown={stopShellNavigationEvent}
      onClick={stopShellNavigationEvent}
    >
      <ToolbarTooltip label={sidebarLabel}>
        <SidebarTitlebarToggleButton
          onClick={onToggleSidebar}
          title={sidebarLabel}
          ariaLabel={sidebarLabel}
          collapsed={leftSidebarCollapsed}
          className="ds-toolbar-icon-button ds-shell-sidebar-toggle"
          cursorSpotlight={false}
        />
      </ToolbarTooltip>
      <ToolbarTooltip label={backLabel}>
        <button
          type="button"
          onClick={onBack}
          disabled={!canGoBack}
          className={shellToolbarIconButtonClass(false, 'ds-shell-navigation-button')}
          aria-label={backLabel}
        >
          <ArrowLeft className="ds-toolbar-icon-svg" strokeWidth={1.85} />
        </button>
      </ToolbarTooltip>
      <ToolbarTooltip label={forwardLabel}>
        <button
          type="button"
          onClick={onForward}
          disabled={!canGoForward}
          className={shellToolbarIconButtonClass(false, 'ds-shell-navigation-button')}
          aria-label={forwardLabel}
        >
          <ArrowLeft className="ds-toolbar-icon-svg ds-shell-forward-icon" strokeWidth={1.85} />
        </button>
      </ToolbarTooltip>
      <ShellGuiUpdateAction mode={shellGuiUpdateActionMode(leftSidebarCollapsed)} />
      <ToolbarTooltip label={newChatLabel} hidden={!leftSidebarCollapsed}>
        <button
          type="button"
          onClick={onNewChat}
          disabled={!leftSidebarCollapsed || newChatDisabled}
          tabIndex={leftSidebarCollapsed ? undefined : -1}
          className={shellToolbarIconButtonClass(false, 'ds-collapsed-new-chat-button')}
          aria-label={newChatLabel}
          aria-hidden={leftSidebarCollapsed ? undefined : true}
          data-collapsed={leftSidebarCollapsed ? 'true' : 'false'}
        >
          <Plus className="ds-toolbar-icon-svg" strokeWidth={1.9} />
        </button>
      </ToolbarTooltip>
    </div>
  )
}
