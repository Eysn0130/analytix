import { File, Image } from '../../../design/AnalytixUiIcons'
import { DshGlobeOutlineMedium } from './DshToolIcons'
import { isWriteImageFilePath } from '@shared/write-text-file'
export function classifyLinkPath(path: string): 'image' | 'file' { return isWriteImageFilePath(path) ? 'image' : 'file' }
export function LinkIconMedium({ kind, className }: { kind: string; href?: string; className?: string }) {
  if (kind === 'url') return <DshGlobeOutlineMedium className={className} />
  const Icon = kind === 'image' ? Image : File
  return <Icon className={className} aria-hidden="true" />
}
