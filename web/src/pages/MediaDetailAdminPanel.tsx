import { Database, FileText, FolderInput, Pencil, Search, Sparkles, Trash2 } from 'lucide-react'

import { EpisodeArtworkToggle } from '../components/EpisodeArtworkToggle'
import type { Media } from '../types'

type MediaDetailAdminPanelProps = {
  media: Media
  scrapeEpisodeArtwork: boolean
  onScrapeEpisodeArtworkChange: (checked: boolean) => void
  onSmartScrape: () => void
  onManualScrape: () => void
  onMetadataEdit: () => void
  onOrganize: () => void
  onProbe: () => void
  onExportNFO: () => void
  onSoftDelete: () => void
}

export function MediaDetailAdminPanel({
  media,
  scrapeEpisodeArtwork,
  onScrapeEpisodeArtworkChange,
  onSmartScrape,
  onManualScrape,
  onMetadataEdit,
  onOrganize,
  onProbe,
  onExportNFO,
  onSoftDelete,
}: MediaDetailAdminPanelProps) {
  return (
    <div className="rounded-2xl border border-gray-200 bg-gray-50/50 p-5 space-y-3">
      <p className="text-[10px] font-bold uppercase tracking-[0.2em] text-[#c9954a]">系统后台高级控制面板</p>
      {isEpisodeArtworkTarget(media) && (
        <EpisodeArtworkToggle
          checked={scrapeEpisodeArtwork}
          onChange={onScrapeEpisodeArtworkChange}
          title="关闭后仍会获取每集简介、评分和时长，只跳过单集图片"
          className="h-10"
        />
      )}
      <div className="flex flex-wrap gap-2">
        <button onClick={onSmartScrape} className="btn-outline py-2 px-3.5 text-xs gap-1.5 border-gray-200 hover:border-brand-500/50 hover:bg-brand-50">
          <Sparkles size={13} className="text-[#c9954a]" />
          <span>智能刮削 (TMDB)</span>
        </button>
        <button onClick={onManualScrape} className="btn-outline py-2 px-3.5 text-xs gap-1.5 border-gray-200 hover:border-brand-500/50 hover:bg-brand-50">
          <Search size={13} className="text-[#c9954a]" />
          <span>手动匹配刮削</span>
        </button>
        <button onClick={onMetadataEdit} className="btn-outline py-2 px-3.5 text-xs gap-1.5 border-gray-200 hover:border-brand-500/50 hover:bg-brand-50">
          <Pencil size={13} className="text-gray-600" />
          <span>编辑元数据</span>
        </button>
        <button onClick={onOrganize} className="btn-outline py-2 px-3.5 text-xs gap-1.5 border-gray-200 hover:border-brand-500/50 hover:bg-brand-50">
          <FolderInput size={13} className="text-[#c9954a]" />
          <span>整理入库</span>
        </button>
        <button onClick={onProbe} className="btn-outline py-2 px-3.5 text-xs gap-1.5 border-gray-200 hover:border-brand-500/50 hover:bg-brand-50">
          <Database size={13} className="text-gray-600" />
          <span>探测媒体轨 (ffprobe)</span>
        </button>
        <button onClick={onExportNFO} className="btn-outline py-2 px-3.5 text-xs gap-1.5 border-gray-200 hover:border-brand-500/50 hover:bg-brand-50">
          <FileText size={13} />
          <span>写出本地 NFO 属性</span>
        </button>
        <button
          onClick={onSoftDelete}
          className="btn-outline py-2 px-3.5 text-xs gap-1.5 !border-red-100 !text-red-500 hover:!bg-red-50 hover:!border-red-200"
        >
          <Trash2 size={13} />
          <span>移入回收站</span>
        </button>
      </div>
    </div>
  )
}

function isEpisodeArtworkTarget(media: Media): boolean {
  return media.season_num > 0 || media.episode_num > 0
}
