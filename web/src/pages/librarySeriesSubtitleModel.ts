import type { SubtitleSearchCandidate } from '../api/subtitles'
import type { Media } from '../types'

export type SeasonSubtitleStrategy = 'downloads' | 'uploader'

export type SeasonSubtitleSelection = {
  candidateIDs: Record<string, string>
  uploader: string
}

export function episodeKey(season: number, episode: number): string {
  return `S${String(season).padStart(2, '0')}E${String(episode).padStart(2, '0')}`
}

export function candidatesForEpisode(
  candidates: SubtitleSearchCandidate[],
  mediaID: string,
): SubtitleSearchCandidate[] {
  return candidates
    .filter((candidate) => candidate.media_id === mediaID)
    .sort(compareSeasonSubtitleCandidates)
}

export function selectSeasonSubtitleCandidates(
  episodes: Media[],
  candidates: SubtitleSearchCandidate[],
  strategy: SeasonSubtitleStrategy,
): SeasonSubtitleSelection {
  const byMedia = new Map(
    episodes.map((episode) => [episode.id, candidatesForEpisode(candidates, episode.id)]),
  )
  const uploader = strategy === 'uploader' ? preferredSeasonUploader(byMedia) : ''
  const candidateIDs: Record<string, string> = {}
  for (const episode of episodes) {
    const available = byMedia.get(episode.id) ?? []
    const selected = uploader
      ? available.find((candidate) => candidate.uploader === uploader) ?? available[0]
      : available[0]
    if (selected) candidateIDs[episode.id] = selected.candidate_id
  }
  return { candidateIDs, uploader }
}

function preferredSeasonUploader(byMedia: Map<string, SubtitleSearchCandidate[]>): string {
  const stats = new Map<string, { coverage: number; downloads: number; likes: number }>()
  for (const candidates of byMedia.values()) {
    const bestByUploader = new Map<string, SubtitleSearchCandidate>()
    for (const candidate of candidates) {
      const uploader = candidate.uploader.trim()
      if (!uploader || bestByUploader.has(uploader)) continue
      bestByUploader.set(uploader, candidate)
    }
    for (const [uploader, candidate] of bestByUploader) {
      const current = stats.get(uploader) ?? { coverage: 0, downloads: 0, likes: 0 }
      current.coverage += 1
      current.downloads += candidate.download_count || 0
      current.likes += candidate.like_count || 0
      stats.set(uploader, current)
    }
  }
  const ranked = Array.from(stats.entries())
    .filter(([, value]) => value.coverage >= 2)
    .sort(([nameA, a], [nameB, b]) =>
      b.coverage - a.coverage
      || b.downloads - a.downloads
      || b.likes - a.likes
      || nameA.localeCompare(nameB, 'zh-CN'),
    )
  return ranked[0]?.[0] ?? ''
}

function compareSeasonSubtitleCandidates(a: SubtitleSearchCandidate, b: SubtitleSearchCandidate): number {
  return (
    (b.download_count || 0) - (a.download_count || 0)
    || (b.like_count || 0) - (a.like_count || 0)
    || a.rank - b.rank
    || a.candidate_id.localeCompare(b.candidate_id)
  )
}
