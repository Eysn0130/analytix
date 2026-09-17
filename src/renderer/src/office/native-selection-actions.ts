import type { ResolvedWriteQuickAction } from '../write/quick-actions'

export type NativeSelectionAction = {
  id: string
  label: string
  kind: 'local' | 'discussion' | 'proposal'
  enabled: boolean
  run: () => void | Promise<unknown>
}

/** The same definitions drive the local action strip, native menu and keyboard entry. */
export function nativeSelectionActions(options: {
  quickActions: ResolvedWriteQuickAction[]
  hasSelection: boolean
  editable: boolean
  busy: boolean
  canSubmit: boolean
  labels: {copy:string;quote:string}
  copy: () => void | Promise<unknown>
  quote: () => void | Promise<unknown>
  task: (action: ResolvedWriteQuickAction) => void | Promise<unknown>
}): NativeSelectionAction[] {
  const available = options.hasSelection && !options.busy
  return [
    {id:'copy',label:options.labels.copy,kind:'local',enabled:available,run:options.copy},
    {id:'quote',label:options.labels.quote,kind:'discussion',enabled:available,run:options.quote},
    ...options.quickActions.map((action, index) => ({
      id:`task:${index}`,label:action.label,kind:action.mode === 'edit' ? 'proposal' as const : 'discussion' as const,
      enabled:available && options.canSubmit && (action.mode !== 'edit' || options.editable),run:() => options.task(action)
    }))
  ]
}
