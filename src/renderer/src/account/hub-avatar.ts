import { DEFAULT_HUB_BASE_URL, type HubAvatarStyle } from '@shared/hub-account'

export function avatarDisplayUrl(value: string | undefined): string {
  const url = value?.trim() ?? ''
  if (!url) return ''
  if (/^(?:https?:|data:|blob:)/iu.test(url)) return url
  if (url.startsWith('/')) return `${DEFAULT_HUB_BASE_URL}${url}`
  return ''
}

export function shouldShowHubPhotoAvatar(style: HubAvatarStyle | undefined, url: string | undefined): boolean {
  return style === 'photo' && Boolean(url?.trim())
}
