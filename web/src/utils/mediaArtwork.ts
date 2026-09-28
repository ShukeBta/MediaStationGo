import type { Media } from '../types'

type MediaArtwork = Pick<
  Media,
  'poster_url' | 'backdrop_url' | 'generated_poster_url' | 'generated_backdrop_url'
>

export function mediaPosterURL(media?: MediaArtwork | null): string {
  return firstArtwork(media?.poster_url, media?.generated_poster_url)
}

export function mediaBackdropURL(media?: MediaArtwork | null): string {
  return firstArtwork(media?.backdrop_url, media?.generated_backdrop_url)
}

export function mediaPrimaryArtworkURL(media?: MediaArtwork | null): string {
  return mediaPosterURL(media) || mediaBackdropURL(media)
}

export function mediaBackdropArtworkURL(media?: MediaArtwork | null): string {
  return mediaBackdropURL(media) || mediaPosterURL(media)
}

export function hasMediaArtwork(media?: MediaArtwork | null): boolean {
  return Boolean(mediaPrimaryArtworkURL(media))
}

function firstArtwork(...values: Array<string | undefined>): string {
  for (const value of values) {
    const normalized = value?.trim()
    if (normalized) return normalized
  }
  return ''
}
