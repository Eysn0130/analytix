import { createPortal } from 'react-dom'
import { useEffect, type ButtonHTMLAttributes, type ReactElement, type ReactNode, type Ref } from 'react'
import { ChevronDown, Expand, Info, Minimize2, MoreHorizontal, PanelRightClose } from '../../design/AnalytixUiIcons'
import { Tooltip } from '../chat/presentation/Tooltip'
import { useAnchoredPanel } from '../chat/presentation/useAnchoredPanel'
import { useTranslation } from 'react-i18next'
import styles from './workspace-viewer-header.module.css'

/** Presentation callbacks only; the workspace keeps its existing layout state. */
export type WorkspaceViewerControls = {
  focused: boolean
  onToggleFocus: () => void
  onCollapse: () => void
}

export function WorkspaceViewerIconButton({ label, children, className = '', tone, compact, ...props }:
  ButtonHTMLAttributes<HTMLButtonElement> & { label: string; children: ReactNode; tone?: 'commit'; compact?: boolean; ref?: Ref<HTMLButtonElement> }): ReactElement {
  return <Tooltip label={label} side="bottom" portal>
    <button {...props} type={props.type ?? 'button'} aria-label={label} className={`${styles.action} ${className}`}
      data-tone={tone} data-compact={compact || undefined}>{children}</button>
  </Tooltip>
}

/** A local menu whose document identity and command authority stay with its caller. */
export function WorkspaceViewerMenu({ label, title, ownerKey, enabled = true, children, triggerRef }:
  { label: string; title?: string; ownerKey?: string; enabled?: boolean; children: ReactNode; triggerRef?: Ref<HTMLButtonElement> }): ReactElement {
  const menu = useAnchoredPanel<HTMLSpanElement>({ enabled, ownerKey })
  useEffect(() => {
    if (!menu.open || document.activeElement !== menu.triggerRef.current) return
    menu.panelRef.current?.querySelector<HTMLElement>('button:not(:disabled),select:not(:disabled),input:not(:disabled),textarea:not(:disabled),[contenteditable="true"],[tabindex="0"]')?.focus()
  }, [menu.open, menu.panelRef, menu.triggerRef])
  const assignTrigger = (element: HTMLButtonElement | null): void => {
    menu.triggerRef.current = element
    if (typeof triggerRef === 'function') triggerRef(element)
    else if (triggerRef) triggerRef.current = element
  }
  return <span ref={menu.rootRef} className={styles.menuAnchor}>
    <WorkspaceViewerIconButton ref={assignTrigger} label={label} disabled={!enabled}
      className={title ? styles.titleButton : ''} aria-haspopup="menu" aria-expanded={menu.open} aria-controls={menu.id}
      onClick={() => menu.setOpen(!menu.open)}>
      {title ? <><span>{title}</span><ChevronDown aria-hidden="true" /></> : <MoreHorizontal aria-hidden="true" />}
    </WorkspaceViewerIconButton>
    {menu.open ? createPortal(<div ref={menu.panelRef} id={menu.id} role="menu" className={styles.menu} style={menu.style}
      onClick={event => {
        if (event.target instanceof Element && event.target.closest('button[role="menuitem"]:not(:disabled)')) menu.setOpen(false)
      }} onKeyDown={event => {
        if (event.nativeEvent.isComposing) return
        if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); menu.setOpen(false); menu.triggerRef.current?.focus(); return }
        if (event.target instanceof Element && event.target.closest('select,input,textarea,[contenteditable]:not([contenteditable="false"])')) return
        if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
        const buttons = [...event.currentTarget.querySelectorAll<HTMLButtonElement>('button:not(:disabled),select:not(:disabled),input:not(:disabled)')]
        if (!buttons.length) return
        event.preventDefault()
        const index = buttons.indexOf(document.activeElement as HTMLButtonElement)
        const next = event.key === 'Home' ? 0 : event.key === 'End' ? buttons.length - 1
          : (index + (event.key === 'ArrowUp' ? -1 : 1) + buttons.length) % buttons.length
        buttons[next].focus()
      }}>{children}</div>, document.body) : null}
  </span>
}

/** Replaces a viewer's existing operation row; it never owns an editor or a tab. */
export function WorkspaceViewerHeader({ title, titleControl, leading, actions, status, statusRole = 'status', viewerControls, className = '' }:
  { title: string; titleControl?: ReactNode; leading?: ReactNode; actions?: ReactNode; status?: string; statusRole?: 'status' | 'alert'; viewerControls?: WorkspaceViewerControls; className?: string }): ReactElement {
  const { t } = useTranslation('common')
  return <header className={`${styles.header} ${className}`} data-workspace-viewer-header data-viewer-controls={!!viewerControls || undefined}>
    {leading ? <div className={styles.actions}>{leading}</div> : null}
    <div className={styles.identity}>{titleControl ?? <span className={styles.title} title={title}>{title}</span>}</div>
    {status ? <Tooltip label={status} side="bottom" portal><span className={styles.status} tabIndex={0} role={statusRole}>
      <Info aria-hidden="true" /><span className={styles.srOnly}>{status}</span>
    </span></Tooltip> : null}
    {actions ? <div className={styles.actionViewport}>{actions}</div> : null}
      {viewerControls ? <div className={styles.actions}>
        <WorkspaceViewerIconButton compact label={t(viewerControls.focused ? 'workbenchDock' : 'workbenchFocus')}
          aria-pressed={viewerControls.focused} onClick={viewerControls.onToggleFocus}>
          {viewerControls.focused ? <Minimize2 aria-hidden="true" /> : <Expand aria-hidden="true" />}
        </WorkspaceViewerIconButton>
        <WorkspaceViewerIconButton compact id="workbench-workspace-collapse" label={t('workbenchCollapse')}
          aria-controls="workbench-right-workspace" aria-expanded={true} onClick={viewerControls.onCollapse}><PanelRightClose aria-hidden="true" /></WorkspaceViewerIconButton>
      </div> : null}
  </header>
}
