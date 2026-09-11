import { Component, type ErrorInfo, type ReactNode } from 'react'
import i18n from '../i18n'

type Props = {
  children: ReactNode
}

type State = {
  hasError: boolean
}

export class AppErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false }

  static getDerivedStateFromError(_error: Error): State {
    return { hasError: true }
  }

  override componentDidCatch(_error: Error, _info: ErrorInfo): void {
    if (typeof window !== 'undefined' && typeof window.analytix?.logs?.error === 'function') {
      void window.analytix.logs.error('renderer', 'Uncaught render error').catch(() => undefined)
    }
  }

  private handleReload = (): void => {
    window.location.reload()
  }

  override render(): ReactNode {
    if (!this.state.hasError) return this.props.children

    return (
      <div className="flex h-full min-h-0 flex-col items-center justify-center bg-ds-main px-6">
        <div className="w-full max-w-md rounded-2xl border border-amber-200/80 bg-amber-50/90 p-6 text-center shadow-[0_14px_32px_rgba(20,47,95,0.08)] dark:border-amber-800/60 dark:bg-amber-950/35">
          <h2 className="text-[16px] font-semibold text-amber-900 dark:text-amber-100">
            {i18n.t('appErrorTitle')}
          </h2>
          <button
            type="button"
            onClick={this.handleReload}
            className="mt-4 rounded-full bg-amber-900/10 px-5 py-2 text-[13px] font-medium text-amber-900 transition hover:bg-amber-900/20 dark:bg-amber-100/10 dark:text-amber-100 dark:hover:bg-amber-100/20"
          >
            {i18n.t('appErrorReload')}
          </button>
        </div>
      </div>
    )
  }
}
