import type {
  HTMLAttributes,
  ReactElement,
  ReactNode
} from 'react'
import { forwardRef } from 'react'

type WorkbenchShellProps = HTMLAttributes<HTMLDivElement> & {
  children: ReactNode
}

export const WorkbenchShell = forwardRef<HTMLDivElement, WorkbenchShellProps>(
  ({ children, className = '', ...props }, ref) => (
    <div
      ref={ref}
      className={`ds-workbench-shell ds-no-drag flex h-full min-h-0 w-full min-w-0 bg-ds-main ${className}`}
      {...props}
    >
      {children}
    </div>
  )
)

WorkbenchShell.displayName = 'WorkbenchShell'

export function WorkbenchStage({
  children,
  className = '',
  ...props
}: HTMLAttributes<HTMLElement> & { children: ReactNode }): ReactElement {
  return (
    <main
      className={`ds-no-drag ds-stage-surface relative flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden ${className}`}
      {...props}
    >
      {children}
    </main>
  )
}
