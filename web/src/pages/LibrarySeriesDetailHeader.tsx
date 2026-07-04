import { Link } from 'react-router-dom'
import { ArrowLeft, Database, FileText, Film, FolderInput, Pencil, Play, Search, Sparkles, Trash2 } from 'lucide-react'

import { imageURL } from '../api/client'
import { ExternalPlayerButton } from '../components/ExternalPlayerButton'
import type { Media } from '../types'
import { seriesTitle, type SeriesCard } from '../utils/groupSeries'

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
}: LibrarySeriesDetailHeaderProps) {
  const firstEpisode = firstPlayableEpisode(visibleEpisodes.length > 0 ? visibleEpisodes : allEpisodes)

  return (
    <>
      <div className="flex items-center gap-4">
        <button onClick={onBack} className="btn-ghost gap-2">
          <ArrowLeft size={16} />
          返回列表
        </button>
        <h2 className="truncate font-display text-2xl font-bold text-ink-600">
          {seriesTitle(series.rep)}
        </h2>
        <span className="text-sm text-sand-500">共 {series.count} 集</span>
      </div>

      <div className="flex flex-col gap-6 sm:flex-row">
        <div className="w-40 shrink-0 overflow-hidden rounded-xl bg-sand-200 shadow-card">
          {series.rep.poster_url ? (
            <img
              src={imageURL(series.rep.poster_url, series.rep.updated_at)}
              alt={series.rep.title}
              className="aspect-[2/3] w-full object-cover"
              referrerPolicy="no-referrer"
            />
          ) : (
            <div className="flex aspect-[2/3] items-center justify-center text-gray-500">
              <Film size={40} />
            </div>
          )}
        </div>
        <div className="flex-1 space-y-3">
          <p className="text-sm leading-relaxed text-ink-50">
            {series.rep.overview || '暂无简介'}
          </p>

          {firstEpisode && (
            <div className="flex flex-wrap gap-2">
              <Link to={`/play/${firstEpisode.id}`} state={{ from: playbackFrom }} className="btn-primary inline-flex">
                <Play size={16} fill="currentColor" />
                从第一集开始播放
              </Link>
              <ExternalPlayerButton mediaId={firstEpisode.id} label="外部播放器播放" />
            </div>
          )}

          {isAdmin && allEpisodes.length > 0 && (
            <div className="rounded-2xl border border-sand-200 bg-white/80 p-4 shadow-sm">
              <p className="mb-3 text-[10px] font-bold uppercase tracking-[0.2em] text-[#c9954a]">系统后台高级控制面板</p>
              <div className="flex flex-wrap gap-2">
                <button onClick={onSmartScrape} disabled={!!seriesToolBusy} className="btn-outline px-3.5 py-2 text-xs gap-1.5">
                  <Sparkles size={13} className="text-[#c9954a]" />
                  <span>{seriesToolBusy === 'scrape' ? '刮削中…' : '整剧智能刮削'}</span>
                </button>
                <button onClick={onManualScrape} disabled={!!seriesToolBusy} className="btn-outline px-3.5 py-2 text-xs gap-1.5">
                  <Search size={13} className="text-[#c9954a]" />
                  <span>手动匹配整剧</span>
                </button>
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
                <button onClick={onSoftDelete} disabled={!!seriesToolBusy} className="btn-outline px-3.5 py-2 text-xs gap-1.5 !border-red-100 !text-red-500 hover:!border-red-200 hover:!bg-red-50">
                  <Trash2 size={13} />
                  <span>{seriesToolBusy === 'delete' ? '处理中…' : '移入回收站'}</span>
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </>
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
