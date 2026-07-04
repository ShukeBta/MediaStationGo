import type { Library } from '../types'
import { EpisodeArtworkToggle } from '../components/EpisodeArtworkToggle'
import { libraryDisplayPath } from './libraryDisplayModel'

type LibraryPageHeaderProps = {
  library: Library | null
  itemCount: number
  loadingAllText: string
  scanProgress: string
  isAdmin: boolean
  scrapeEpisodeArtwork: boolean
  scanning: boolean
  scraping: boolean
  repairing: boolean
  onScrapeEpisodeArtworkChange: (checked: boolean) => void
  onScan: () => void
  onScrape: () => void
  onRepairRescrape: () => void
}

export function LibraryPageHeader({
  library,
  itemCount,
  loadingAllText,
  scanProgress,
  isAdmin,
  scrapeEpisodeArtwork,
  scanning,
  scraping,
  repairing,
  onScrapeEpisodeArtworkChange,
  onScan,
  onScrape,
  onRepairRescrape,
}: LibraryPageHeaderProps) {
  const displayPath = library ? libraryDisplayPath(library.path) : ''

  return (
    <div className="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1 className="font-display text-3xl font-bold text-ink-600">
          {library?.name ?? '媒体库'}
          <span className="text-sand-500"> ({itemCount})</span>
        </h1>
        {library && <p className="text-sm text-ink-50" title={library.path}>{library.type} · {displayPath}</p>}
        {loadingAllText && <p className="mt-1 text-xs text-sand-500">{loadingAllText}</p>}
        {scanProgress && <p className="mt-1 text-xs text-brand-500">{scanProgress}</p>}
      </div>
      {isAdmin && (
        <div className="flex flex-wrap items-center gap-2">
          <EpisodeArtworkToggle
            checked={scrapeEpisodeArtwork}
            onChange={onScrapeEpisodeArtworkChange}
            title="关闭后仍会获取主海报和每集文字元数据，只跳过每集图片"
            className="h-10"
          />
          <button onClick={onScan} disabled={scanning} className="btn-outline">
            {scanning ? '扫描中…' : '立即扫描'}
          </button>
          <button onClick={onScrape} disabled={scraping} className="btn-outline">
            {scraping ? '刮削中…' : '刮削元数据'}
          </button>
          <button
            onClick={onRepairRescrape}
            disabled={repairing}
            className="btn-outline"
            title="回填本库占位符外部 ID 并重刮，修正空 ID / 拆集问题"
          >
            {repairing ? '修复中…' : '修复+重刮本库'}
          </button>
        </div>
      )}
    </div>
  )
}
