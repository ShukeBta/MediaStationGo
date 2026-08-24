import { GitMerge, Globe, WandSparkles } from 'lucide-react'

import type { Library } from '../types'
import { libraryDisplayPath } from './libraryDisplayModel'

type LibraryPageHeaderProps = {
  library: Library | null
  itemCount: number
  loadingAllText: string
  scanProgress: string
  isAdmin: boolean
  scanning: boolean
  scraping: boolean
  repairing: boolean
  canCleanTitles: boolean
  canManageAggregation: boolean
  onScan: () => void
  onScrape: () => void
  onRepairRescrape: () => void
  onCleanTitles: () => void
  onManageAggregation: () => void
  onResourceSearch: () => void
}

export function LibraryPageHeader({
  library,
  itemCount,
  loadingAllText,
  scanProgress,
  isAdmin,
  scanning,
  scraping,
  repairing,
  canCleanTitles,
  canManageAggregation,
  onScan,
  onScrape,
  onRepairRescrape,
  onCleanTitles,
  onManageAggregation,
  onResourceSearch,
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
      <div className="flex flex-wrap items-center justify-end gap-2">
        <button
          type="button"
          className="btn-outline h-10 px-3"
          title="查找资源"
          aria-label="查找资源"
          onClick={onResourceSearch}
        >
          <Globe size={18} />
          <span>查找资源</span>
        </button>
        {isAdmin && (
          <>
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
          {canCleanTitles && (
            <button type="button" onClick={onCleanTitles} className="btn-outline">
              <WandSparkles size={16} />
              AI 清洗标题
            </button>
          )}
          {canManageAggregation && (
            <button type="button" onClick={onManageAggregation} className="btn-outline">
              <GitMerge size={16} />
              手动聚合
            </button>
          )}
          </>
        )}
      </div>
    </div>
  )
}
