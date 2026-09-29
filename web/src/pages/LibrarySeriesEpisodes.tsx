import { Link } from 'react-router-dom'
import { ChevronRight, Play } from 'lucide-react'

import { imageURL } from '../api/client'
import { ExternalPlayerButton } from '../components/ExternalPlayerButton'
import type { Media } from '../types'
import { seriesTitleFromPath } from '../utils/groupSeries'
import { mediaBackdropArtworkURL } from '../utils/mediaArtwork'
import { formatSize } from './libraryPageModel'
import '../styles/media.css'

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
      <div className="collection-shelf-empty" role="status">
        正在加载剧集…
      </div>
    )
  }

  const displaySeason = selectedSeason ?? selectedEpisodes[0]?.season ?? 1

  return (
    <>
      <div className="series-season-tabs" aria-label="选择季度">
        {selectedEpisodes.map(({ season, episodes }) => (
          <button
            key={season}
            type="button"
            onClick={() => onSeasonChange(season)}
            aria-pressed={displaySeason === season}
            className={`series-season-tab ${displaySeason === season ? 'is-active' : ''}`}
          >
            {season === 0 ? '特别篇' : `第 ${season} 季`} · {episodes.length} 集
          </button>
        ))}
      </div>

      <div>
        <h3 className="series-episodes-heading">
          {displaySeason === 0 ? '特别篇' : `第 ${displaySeason} 季`}
        </h3>
        <div className="series-episodes-grid">
          {visibleEpisodes.map((ep) => (
            <div
              key={ep.id}
              className="series-episode-card"
            >
              <Link to={`/media/${ep.id}`} className="series-episode-content">
                <div className="series-episode-thumb">
                  <span>{ep.episode_num ? `${ep.episode_num}${ep.episode_end_num && ep.episode_end_num > ep.episode_num ? `–${ep.episode_end_num}` : ''}${ep.episode_part_num ? `·${ep.episode_part_num}` : ''}` : '—'}</span>
                  {mediaBackdropArtworkURL(ep) ? (
                    <img
                      src={imageURL(mediaBackdropArtworkURL(ep), ep.updated_at, { maxWidth: 96, quality: 78 })}
                      alt=""
                      loading="lazy"
                      decoding="async"
                      className="absolute inset-0 h-full w-full object-cover"
                      referrerPolicy="no-referrer"
                      onError={(event) => { event.currentTarget.style.visibility = 'hidden' }}
                    />
                  ) : null}
                </div>
                <div className="min-w-0 flex-1">
                  <p className="series-episode-title">
                    {episodeDisplayTitle(ep, visibleEpisodes)}
                  </p>
                  <p className="series-episode-meta">
                    {ep.duration_sec > 0
                      ? `${Math.floor(ep.duration_sec / 60)} 分钟`
                      : formatSize(ep.size_bytes)}
                  </p>
                </div>
                <ChevronRight size={14} className="series-episode-chevron" />
              </Link>
              <div className="series-episode-actions">
                <Link
                  to={`/play/${ep.id}`}
                  state={{ from: playbackFrom }}
                  aria-label={`直接播放${episodeDisplayTitle(ep, visibleEpisodes)}`}
                  title="直接播放"
                  className="series-episode-play"
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
