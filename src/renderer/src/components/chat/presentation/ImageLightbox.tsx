import { ImagePreviewLightbox } from '../ImagePreviewLightbox'
// The existing owner retains zoom, download, modal focus and Escape behavior.
export interface ImageLightboxLabels { close?: string; download?: string }
export function ImageLightbox({ src, alt, onClose }: { src: string; alt: string; labels?: ImageLightboxLabels; onClose: () => void }) {
  return <ImagePreviewLightbox open src={src} alt={alt} downloadHref={src} onClose={onClose} />
}
