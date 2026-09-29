import { Link } from 'react-router-dom'
import { ArrowLeft, BellPlus, BellRing, CircleArrowUp, CirclePlus, Database, FileText, FolderInput, Pencil, Play, Search, Settings2, Sparkles, Star, Trash2 } from 'lucide-react'

import { ExternalPlayerButton } from '../components/ExternalPlayerButton'
import { PosterArtwork } from '../components/PosterArtwork'
import type { Media } from '../types'
import { seriesTitle, type SeriesCard } from '../utils/groupSeries'
import { mediaPosterURL } from '../utils/mediaArtwork'
import { LibrarySeriesSubtitleSearch } from './LibrarySeriesSubtitleSearch'
import { MediaDetailBackdrop } from './MediaDetailArtwork'
import '../styles/media.css'

type LibrarySeriesDetailHeaderProps = {
  series: SeriesCard
  visibleEpisodes: Media[]
  allEpisodes: Media[]
  playbackFrom: string
  isAdmin: boolean
  seriesToolBusy: string
  onBack: () => void
  onSmartScrape: () => void
  onManualScrape: () => void
  onMetadataEdit: () => void
  onProbe: () => void
  onNFO: () => void
  onOrganize: () => void
  onSoftDelete: () => void
  onUpgrade: () => void
  canReplenish: boolean
  onReplenish: () => void
  canFollow: boolean
  onFollow: () => void
  autoFollow: boolean
}

export function LibrarySeriesDetailHeader({
  series,
  visibleEpisodes,
  allEpisodes,
  playbackFrom,
  isAdmin,
  seriesToolBusy,
  onBack,
  onSmartScrape,
  onManualScrape,
  onMetadataEdit,
  onProbe,
  onNFO,
  onOrganize,
  onSoftDelete,
  onUpgrade,
  canReplenish,
  onReplenish,
  canFollow,
  onFollow,
  autoFollow,
}: LibrarySeriesDetailHeaderProps) {
  const firstEpisode = firstPlayableEpisode(visibleEpisodes.length > 0 ? visibleEpisodes : allEpisodes)
  const poster = mediaPosterURL(series.rep)

  return (
    <section className="series-detail-hero">
      <MediaDetailBackdrop media={series.rep} />
      <div className="series-detail-navigation">
        <button type="button" onClick={onBack} className="media-detail-back-button">
          <ArrowLeft size={16} />
          返回列表
        </button>
        {autoFollow && (
          <span className="series-detail-follow">
            <BellRing size={13} />
            自动追更中
          </span>
        )}
      </div>

      <div className="series-detail-main">
        <div className="series-detail-poster">
          <PosterArtwork title={seriesTitle(series.rep)} poster={poster} version={series.rep.updated_at} year={series.rep.year} maxWidth={480} />
        </div>
        <div className="series-detail-copy">
          <div className="media-detail-heading">
            <p className="media-eyebrow">SERIES / 一集接一集的好故事</p>
            <h2>{seriesTitle(series.rep)}</h2>
            <div className="media-detail-facts">
              {series.rep.rating > 0 && <span className="media-detail-rating"><Star size={13} fill="currentColor" />{series.rep.rating.toFixed(1)}</span>}
              {series.rep.year > 0 && <span>{series.rep.year}</span>}
              <span>共 {series.count} 集</span>
            </div>
          </div>
          <p className="series-detail-overview">
            {series.rep.overview || '暂无简介'}
          </p>

          {firstEpisode && (
            <div className="media-detail-playback-actions">
              <Link to={`/play/${firstEpisode.id}`} state={{ from: playbackFrom }} className="btn-primary media-detail-primary-play">
                <Play size={16} fill="currentColor" />
                从第一集开始播放
              </Link>
              <ExternalPlayerButton mediaId={firstEpisode.id} label="外部播放器播放" />
              {isAdmin && canReplenish && (
                <button onClick={onReplenish} disabled={!!seriesToolBusy} className="btn-outline gap-2">
                  <CirclePlus size={16} />
                  补集
                </button>
              )}
              {isAdmin && canFollow && (
                <button onClick={onFollow} disabled={!!seriesToolBusy} className="btn-outline gap-2">
                  <BellPlus size={16} />
                  配置自动追更
                </button>
              )}
            </div>
          )}

          {isAdmin && allEpisodes.length > 0 && (
            <details className="series-detail-management">
              <summary><Settings2 size={14} />管理这部剧集</summary>
              <div className="flex flex-wrap gap-2">
                <button onClick={onSmartScrape} disabled={!!seriesToolBusy} className="btn-outline px-3.5 py-2 text-xs gap-1.5">
                  <Sparkles size={13} />
                  <span>{seriesToolBusy === 'scrape' ? '刮削中…' : '整剧智能刮削'}</span>
                </button>
                <button onClick={onManualScrape} disabled={!!seriesToolBusy} className="btn-outline px-3.5 py-2 text-xs gap-1.5">
                  <Search size={13} />
                  <span>手动匹配整剧</span>
                </button>
                <LibrarySeriesSubtitleSearch title={seriesTitle(series.rep)} episodes={allEpisodes} />
                <button onClick={onMetadataEdit} disabled={!!seriesToolBusy} className="btn-outline px-3.5 py-2 text-xs gap-1.5">
                  <Pencil size={13} />
                  <span>编辑元数据</span>
                </button>
                <button onClick={onProbe} disabled={!!seriesToolBusy} className="btn-outline px-3.5 py-2 text-xs gap-1.5">
                  <Database size={13} />
                  <span>{seriesToolBusy === 'probe' ? '探测中…' : '探测媒体轨'}</span>
                </button>
                <button onClick={onNFO} disabled={!!seriesToolBusy} className="btn-outline px-3.5 py-2 text-xs gap-1.5">
                  <FileText size={13} />
                  <span>{seriesToolBusy === 'nfo' ? '写出中…' : '写出本地 NFO'}</span>
                </button>
                <button onClick={onOrganize} disabled={!!seriesToolBusy} className="btn-outline px-3.5 py-2 text-xs gap-1.5">
                  <FolderInput size={13} />
                  <span>{seriesToolBusy === 'organize' ? '整理中…' : '整理当前合集'}</span>
                </button>
                <button onClick={onUpgrade} disabled={!!seriesToolBusy} className="btn-outline px-3.5 py-2 text-xs gap-1.5">
                  <CircleArrowUp size={13} />
                  <span>整剧升级片源</span>
                </button>
                <button onClick={onSoftDelete} disabled={!!seriesToolBusy} className="btn-outline px-3.5 py-2 text-xs gap-1.5 !border-red-100 !text-red-500 hover:!border-red-200 hover:!bg-red-50">
                  <Trash2 size={13} />
                  <span>{seriesToolBusy === 'delete' ? '处理中…' : '移入回收站'}</span>
                </button>
              </div>
            </details>
          )}
        </div>
      </div>
    </section>
  )
}

function firstPlayableEpisode(episodes: Media[]): Media | null {
  const sorted = [...episodes]
  sorted.sort((a, b) =>
    (a.season_num || 0) - (b.season_num || 0)
    || (a.episode_num || 0) - (b.episode_num || 0),
  )
  return sorted[0] ?? null
}
