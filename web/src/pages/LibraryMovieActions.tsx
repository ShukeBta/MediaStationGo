import { Database, FileText, Search, Sparkles, Trash2 } from 'lucide-react'

import type { Media } from '../types'

type LibraryMovieActionsProps = {
  media: Media
  busy: boolean
  onSmartScrape: (media: Media) => void
  onManualScrape: (media: Media) => void
  onProbe: (media: Media) => void
  onNFO: (media: Media) => void
  onSoftDelete: (media: Media) => void
}

export function LibraryMovieActions({
  media,
  busy,
  onSmartScrape,
  onManualScrape,
  onProbe,
  onNFO,
  onSoftDelete,
}: LibraryMovieActionsProps) {
  const buttonClass = 'flex h-8 w-8 items-center justify-center rounded-lg border border-white/70 bg-white/90 text-gray-700 shadow-sm backdrop-blur transition hover:bg-brand-50 hover:text-brand-600 disabled:opacity-50'

  return (
    <>
      <button title="智能刮削" disabled={busy} onClick={() => onSmartScrape(media)} className={buttonClass}>
        <Sparkles size={13} />
      </button>
      <button title="手动匹配刮削" disabled={busy} onClick={() => onManualScrape(media)} className={buttonClass}>
        <Search size={13} />
      </button>
      <button title="探测媒体轨" disabled={busy} onClick={() => onProbe(media)} className={buttonClass}>
        <Database size={13} />
      </button>
      <button title="写出本地 NFO" disabled={busy} onClick={() => onNFO(media)} className={buttonClass}>
        <FileText size={13} />
      </button>
      <button title="移入回收站" disabled={busy} onClick={() => onSoftDelete(media)} className={`${buttonClass} hover:!bg-red-50 hover:!text-red-500`}>
        <Trash2 size={13} />
      </button>
    </>
  )
}
