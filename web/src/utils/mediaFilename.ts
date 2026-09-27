import type { Media } from '../types'

type MediaFilenameSource = Pick<Media, 'id' | 'path' | 'display_title' | 'title' | 'original_name'>

export function mediaFilename(media: MediaFilenameSource): string {
  const normalizedPath = media.path?.trim().replace(/\\/g, '/') ?? ''
  const parts = normalizedPath.split('/').filter(Boolean)
  return parts[parts.length - 1]
    || media.display_title?.trim()
    || media.title?.trim()
    || media.original_name?.trim()
    || media.id
    || '未命名片源'
}

export function compareMediaFilename(left: MediaFilenameSource, right: MediaFilenameSource): number {
  return mediaFilename(left).localeCompare(mediaFilename(right), 'zh-CN', {
    numeric: true,
    sensitivity: 'base',
  })
}
