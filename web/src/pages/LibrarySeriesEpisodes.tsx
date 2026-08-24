import { Link } from 'react-router-dom'
import { ChevronRight, Play } from 'lucide-react'

import { imageURL } from '../api/client'
import { ExternalPlayerButton } from '../components/ExternalPlayerButton'
import type { Media } from '../types'
import { seriesTitleFromPath } from '../utils/groupSeries'
import { mediaBackdropArtworkURL } from '../utils/mediaArtwork'
import { formatSize } from './libraryPageModel'

type SeasonGroup = {
  season: number
  episodes: Media[]
}

type LibrarySeriesEpisodesProps = {
  loading: boolean
  selectedEpisodes: SeasonGroup[]
  selectedSeason: number | null
  visibleEpisodes: Media[]
  playbackFrom: string
  onSeasonChange: (season: number) => void
}

export function LibrarySeriesEpisodes({
  loading,
  selectedEpisodes,
  selectedSeason,
  visibleEpisodes,
  playbackFrom,
  onSeasonChange,
}: LibrarySeriesEpisodesProps) {
  if (loading) {
    return (
      <div className="rounded-2xl bg-white/75 p-6 text-center text-sm text-sand-500 shadow-soft">
        正在加载剧集…
      </div>
    )
  }

  const displaySeason = selectedSeason ?? selectedEpisodes[0]?.season ?? 1

  return (
    <>
      <div className="flex flex-wrap items-center gap-2">
        {selectedEpisodes.map(({ season, episodes }) => (
          <button
            key={season}
            onClick={() => onSeasonChange(season)}
            className={
              'rounded-xl border px-4 py-2 text-sm font-semibold transition ' +
              (selectedSeason === season
                ? 'border-brand-300 bg-brand-50 text-brand-700'
                : 'border-sand-200 bg-white text-ink-100 hover:border-brand-200 hover:text-brand-600')
            }
          >
            {season === 0 ? '特别篇' : `第 ${season} 季`} · {episodes.length} 集
          </button>
        ))}
      </div>

      <div>
        <h3 className="mb-3 font-display text-lg font-semibold text-ink-600">
          {displaySeason === 0 ? '特别篇' : `第 ${displaySeason} 季`}
        </h3>
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
          {visibleEpisodes.map((ep) => (
            <div
              key={ep.id}
              className="group flex items-center gap-3 rounded-xl border border-sand-200 bg-white p-3 shadow-card transition-all hover:border-brand-300 hover:shadow-card-hover"
            >
              <Link to={`/media/${ep.id}`} className="flex min-w-0 flex-1 items-center gap-3">
                <div className="flex h-10 w-10 shrink-0 items-center justify-center overflow-hidden rounded-xl bg-brand-50 text-brand-600 font-semibold text-sm">
                  {mediaBackdropArtworkURL(ep) ? (
                    <img
                      src={imageURL(mediaBackdropArtworkURL(ep), ep.updated_at, { maxWidth: 96, quality: 78 })}
                      alt=""
                      loading="lazy"
                      decoding="async"
                      className="h-full w-full object-cover"
                      referrerPolicy="no-referrer"
                    />
                  ) : (
                    ep.episode_num || '—'
                  )}
                </div>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium text-ink-600">
                    {episodeDisplayTitle(ep, visibleEpisodes)}
                  </p>
                  <p className="text-xs text-sand-500">
                    {ep.duration_sec > 0
                      ? `${Math.floor(ep.duration_sec / 60)} 分钟`
                      : formatSize(ep.size_bytes)}
                  </p>
                </div>
                <ChevronRight size={15} className="shrink-0 text-sand-400 transition-colors group-hover:text-brand-500" />
              </Link>
              <div className="flex shrink-0 flex-col gap-1">
                <Link
                  to={`/play/${ep.id}`}
                  state={{ from: playbackFrom }}
                  aria-label={`直接播放${episodeDisplayTitle(ep, visibleEpisodes)}`}
                  title="直接播放"
                  className="inline-flex items-center justify-center gap-1 rounded-lg border border-brand-300/50 bg-brand-50 px-2 py-1 text-xs font-semibold text-brand-600 transition hover:border-brand-400 hover:bg-brand-100"
                >
                  <Play size={13} fill="currentColor" />
                  播放
                </Link>
                <ExternalPlayerButton mediaId={ep.id} label="外部" compact />
              </div>
            </div>
          ))}
        </div>
      </div>
    </>
  )
}

function episodeDisplayTitle(ep: Media, siblings: Media[]): string {
  const title = ep.episode_title?.trim()
  if (title && !looksLikeSeriesTitle(ep, title, siblings)) {
    return title
  }

  const mediaTitle = ep.title?.trim()
  if (mediaTitle && !looksLikeSeriesTitle(ep, mediaTitle, siblings)) {
    return mediaTitle
  }

  return ep.episode_num > 0 ? `第 ${ep.episode_num} 集` : mediaTitle || title || '未命名'
}

function looksLikeSeriesTitle(ep: Media, title: string, siblings: Media[]): boolean {
  const normalized = normalizeEpisodeTitle(title)
  if (!normalized) return true
  if (ep.original_name && normalizeEpisodeTitle(ep.original_name) === normalized) return true
  const pathTitle = seriesTitleFromPath(ep.path)
  if (pathTitle && normalizeEpisodeTitle(pathTitle) === normalized) return true

  const siblingTitles = new Set(
    siblings
      .map((item) => normalizeEpisodeTitle(item.title))
      .filter(Boolean),
  )
  return siblingTitles.size === 1 && siblingTitles.has(normalized) && siblings.length > 1
}

function normalizeEpisodeTitle(value?: string): string {
  return (value ?? '')
    .toLowerCase()
    .replace(/\s*\((?:19|20)\d{2}\)\s*/g, ' ')
    .replace(/\s*\{(?:tmdb|tmdbid|douban|bangumi|bgm|thetvdb|tvdb)[\s:=#-]*[a-z0-9_-]+\}\s*/g, ' ')
    .replace(/[\s._-]+/g, ' ')
    .trim()
}
