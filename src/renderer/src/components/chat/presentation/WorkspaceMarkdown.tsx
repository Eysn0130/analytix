import { Fragment, useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { isWriteImageFilePath } from '@shared/write-text-file'
import { findFileReferences, type FileReferenceTarget } from '../../../lib/file-references'
import { useValidatedFileReference } from '../../../lib/file-reference-validation'
import { previewWorkspaceFile } from '../../../lib/workspace-file-preview'
import { openWorkspacePathInEditor } from '../../../lib/open-workspace-path'
import { useChatStore } from '../../../store/chat-store'
import { HoverCard } from './HoverCard'
import { ImagePreview } from './ImagePreview'
import { ImagePreviewLightbox } from '../ImagePreviewLightbox'
import { LinkIconMedium, classifyLinkPath } from './LinkIcon'
import css from './markdown/MarkdownText.module.css'

// Only the existing host image reader may produce local raster data URLs.
const RASTER_DATA_URL = /^data:image\/(?:png|jpeg|webp|gif|bmp|avif|x-icon|vnd\.microsoft\.icon);base64,[A-Za-z0-9+/=\r\n]+$/
function useWorkspaceImage(target: FileReferenceTarget | null): string | undefined {
  const workspaceRoot = useChatStore(s => s.workspaceRoot)
  const validation = useValidatedFileReference(target, workspaceRoot)
  const key = `${workspaceRoot}\u0000${target?.path ?? ''}`
  const [image, setImage] = useState<{ key: string; src: string } | null>(null)
  useEffect(() => {
    if (!target || validation.status !== 'valid' || !isWriteImageFilePath(validation.path)) return
    let canceled = false
    const read = window.analytix?.files?.readImage
    if (typeof read !== 'function') return
    void read({ path: validation.path, workspaceRoot }).then(result => {
      if (!canceled && result.ok && RASTER_DATA_URL.test(result.dataUrl)) setImage({ key, src: result.dataUrl })
    }).catch(() => undefined)
    return () => { canceled = true }
  }, [key, target, validation.status, validation.status === 'valid' ? validation.path : '', workspaceRoot])
  return image?.key === key ? image.src : undefined
}

export function WorkspaceFileLink({ target: input, children, glyph = true }: {
  target: FileReferenceTarget; children: ReactNode; glyph?: boolean
}) {
  const target = useMemo(() => input, [input.path, input.line, input.column])
  const workspaceRoot = useChatStore(s => s.workspaceRoot)
  const validation = useValidatedFileReference(target, workspaceRoot)
  const imageTarget = useMemo(() => isWriteImageFilePath(target.path) ? target : null, [target])
  const src = useWorkspaceImage(imageTarget)
  const { t } = useTranslation('common')
  if (validation.status !== 'valid') return <>{children}</>
  const resolved = { ...target, path: validation.path }
  const anchor = <button type="button" className={`${css.fileMention} ${css.fileLink} ds-file-reference-link`}
    title={resolved.path} onClick={() => previewWorkspaceFile({ ...resolved, workspaceRoot })}
    onDoubleClick={() => { void openWorkspacePathInEditor(resolved, workspaceRoot).catch(() => undefined) }}>
    {glyph && <LinkIconMedium kind={classifyLinkPath(resolved.path)} className={css.linkIcon} />}{children}
  </button>
  return src ? <HoverCard inline anchor={anchor} content={<>
    <ImagePreview src={src} alt={resolved.path.split(/[\\/]/).pop() ?? ''} loadingLabel={t('loading')} failedLabel={t('imagePreviewFailed')} />
    <span className={css.previewName}>{resolved.path.split(/[\\/]/).pop()}</span>
  </>} /> : anchor
}

export function TextFileReferences({ text }: { text: string }) {
  const matches = findFileReferences(text)
  const children: ReactNode[] = []
  let offset = 0
  for (const match of matches) {
    children.push(text.slice(offset, match.start), <WorkspaceFileLink key={`${match.start}:${match.target.path}`} target={match.target}>{match.text}</WorkspaceFileLink>)
    offset = match.end
  }
  children.push(text.slice(offset))
  return <>{children}</>
}

export function InlineFileMention({ value }: { value: string }) {
  const matches = findFileReferences(value)
  const match = matches.length === 1 && matches[0].start === 0 && matches[0].end === value.length ? matches[0] : undefined
  return <code>{match ? <WorkspaceFileLink target={match.target}>{value}</WorkspaceFileLink> : value}</code>
}

export function WorkspaceMarkdownImage({ destination, alt, local, inLink }: {
  destination: string; alt: string; local?: FileReferenceTarget; inLink: boolean
}) {
  const target = useMemo(() => local ?? null, [local?.path, local?.line])
  const localSrc = useWorkspaceImage(target)
  const remoteSrc = useMemo(() => {
    try { const url = new URL(destination); return /^https?:$/.test(url.protocol) ? destination : undefined } catch { return undefined }
  }, [destination])
  const src = localSrc ?? remoteSrc
  const [failed, setFailed] = useState(false)
  const [open, setOpen] = useState(false)
  const close = useCallback(() => setOpen(false), [])
  const { t } = useTranslation('common')
  useEffect(() => { setFailed(false); setOpen(false) }, [src])
  if (!src || failed) return <span className={css.imageAlt}>{alt || destination}</span>
  const image = <img className={css.image} src={src} alt={alt} onError={() => setFailed(true)} loading="lazy" decoding="async" referrerPolicy="no-referrer" />
  if (inLink) return image
  return <Fragment>
    <button type="button" className={css.imageButton} aria-label={t('imagePreviewOpen', { name: alt || destination })} onClick={() => setOpen(true)}>{image}</button>
    {open && <ImagePreviewLightbox open src={src} alt={alt} downloadHref={localSrc} downloadDisabled={!localSrc} onClose={close} />}
  </Fragment>
}
