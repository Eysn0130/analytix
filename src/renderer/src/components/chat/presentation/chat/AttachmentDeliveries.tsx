import type { ReactNode } from 'react'
import type { ToolBlock } from '../../../../agent/types'
import { GeneratedFilesPanel } from '../../message-timeline-bubbles'
import type { PresentCall } from './presented-files'
import styles from './ChatView.module.css'

/** Existing schema validation, protected attachments and native save/open owner. */
export function AttachmentDeliveries({ blocks, children, present }: { blocks: ToolBlock[]; children?: ReactNode; present?: { threadId: string | null; calls: PresentCall[] } }) {
  return <div className={styles.deliveries}><GeneratedFilesPanel blocks={blocks} present={present} />{children}</div>
}
