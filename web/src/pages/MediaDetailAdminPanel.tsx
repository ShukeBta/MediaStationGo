import { useState } from 'react'
import { ArrowRightLeft, Database, FileText, FolderInput, Image, LoaderCircle, Pencil, Search, Sparkles, Trash2 } from 'lucide-react'

import type { Media } from '../types'

type MediaDetailAdminPanelProps = {
  media: Media
  onSmartScrape: () => void
  onManualScrape: () => void
  onMetadataEdit: () => void
  onOrganize: () => void
  onMoveLibrary: () => void
  onProbe: () => void | Promise<void>
  onGenerateArtwork: () => void
  onExportNFO: () => void
  onSoftDelete: () => void
}

export function MediaDetailAdminPanel({
  media,
  onSmartScrape,
  onManualScrape,
  onMetadataEdit,
  onOrganize,
  onMoveLibrary,
  onProbe,
  onGenerateArtwork,
  onExportNFO,
  onSoftDelete,
}: MediaDetailAdminPanelProps) {
  const [probing, setProbing] = useState(false)

  const handleProbe = async () => {
    if (probing) return
    setProbing(true)
    try {
      await onProbe()
    } finally {
      setProbing(false)
    }
  }

  return (
    <div className="rounded-2xl border border-gray-200 bg-gray-50/50 p-5 space-y-3">
      <p className="text-[10px] font-bold uppercase tracking-[0.2em] text-[#c9954a]">系统后台高级控制面板</p>
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
        <button onClick={onMoveLibrary} className="btn-outline py-2 px-3.5 text-xs gap-1.5 border-gray-200 hover:border-brand-500/50 hover:bg-brand-50">
          <ArrowRightLeft size={13} className="text-[#c9954a]" />
          <span>移动媒体库</span>
        </button>
        <button
          type="button"
          onClick={() => void handleProbe()}
          disabled={probing}
          className="btn-outline py-2 px-3.5 text-xs gap-1.5 border-gray-200 hover:border-brand-500/50 hover:bg-brand-50 disabled:cursor-wait disabled:opacity-70"
        >
          {probing
            ? <LoaderCircle size={13} className="animate-spin text-[#c9954a]" />
            : <Database size={13} className="text-gray-600" />}
          <span>{probing ? '探测中…' : '探测媒体轨 (ffprobe)'}</span>
        </button>
        {canGenerateArtwork(media) && (
          <button onClick={onGenerateArtwork} className="btn-outline py-2 px-3.5 text-xs gap-1.5 border-gray-200 hover:border-brand-500/50 hover:bg-brand-50">
            <Image size={13} className="text-gray-600" />
            <span>按时间生成预览图</span>
          </button>
        )}
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

function canGenerateArtwork(media: Media): boolean {
  return media.season_num === 0 && media.episode_num === 0 && !media.poster_url?.trim() && !media.backdrop_url?.trim()
}
