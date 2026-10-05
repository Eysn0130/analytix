import { File, Image, ExternalLink as Link } from '../../../design/AnalytixUiIcons'
import { isWriteImageFilePath } from '@shared/write-text-file'
export function classifyLinkPath(path: string): 'image' | 'file' { return isWriteImageFilePath(path) ? 'image' : 'file' }
export function LinkIconMedium({ kind, className }: { kind: string; href?: string; className?: string }) {
  const Icon = kind === 'url' ? Link : kind === 'image' ? Image : File
  return <Icon className={className} aria-hidden="true" />
}
