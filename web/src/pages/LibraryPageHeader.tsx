import { ArrowLeft, GitMerge, Globe, RefreshCw, SlidersHorizontal, WandSparkles } from 'lucide-react'
import { Link } from 'react-router-dom'

import type { Library } from '../types'
import { libraryDisplayPath } from './libraryDisplayModel'
import '../styles/media.css'

type LibraryPageHeaderProps = {
  library: Library | null
  itemCount: number
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
    <header className="library-page-heading">
      <div className="library-page-heading-copy">
        <Link to="/libraries" className="library-breadcrumb"><ArrowLeft size={13} />全部媒体库</Link>
        <h1>
          {library?.name ?? '媒体库'}
          <span className="library-heading-count">{itemCount.toLocaleString()}</span>
        </h1>
        {library && <p className="library-heading-path" title={library.path}>{displayPath}</p>}
        {scanProgress && <p className="collection-status" role="status">{scanProgress}</p>}
      </div>
      <div className="library-page-heading-actions">
        <button
          type="button"
          className="btn-primary"
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
            <RefreshCw size={14} className={scanning ? 'animate-spin' : ''} />
            {scanning ? '扫描中…' : '立即扫描'}
          </button>
          <details className="collection-maintenance">
          <summary><SlidersHorizontal size={15} />管理</summary>
          <div className="collection-maintenance-panel">
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
          </div>
          </details>
          </>
        )}
      </div>
    </header>
  )
}
