import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { motion, useReducedMotion } from 'framer-motion'
import { ArrowRight, ArrowUpRight, Film, FolderOpen, Library as LibraryIcon, Music, PlayCircle, RefreshCw, Tv } from 'lucide-react'

import { imageURL } from '../api/client'
import { MediaCard } from '../components/MediaCard'
import { MediaProbeBackfillButton } from '../components/MediaProbeBackfillButton'
import { artworkScore, seriesCardLink, type SeriesCard } from '../utils/groupSeries'
import { mediaPrimaryArtworkURL } from '../utils/mediaArtwork'
import { libraryDisplayPath } from './libraryDisplayModel'
import { mediaTime, type LibraryPreview } from './librariesPageModel'
import '../styles/media.css'

const TYPE_ICONS: Record<string, ReactNode> = {
  movie: <Film size={18} />, tv: <Tv size={18} />, anime: <PlayCircle size={18} />,
  variety: <Tv size={18} />, music: <Music size={18} />, adult: <Film size={18} />,
}

const TYPE_LABELS: Record<string, string> = {
  movie: '电影', tv: '剧集', anime: '动漫', variety: '综艺', music: '音乐', adult: '成人',
}

export function LibrariesHeader({
  previewCount, total, repairMsg, repairing, onRepairRescrape,
}: {
  previewCount: number
  total: number
  repairMsg: string
  repairing: boolean
  onRepairRescrape: () => void
}) {
  return (
    <header className="collection-page-header">
      <div className="collection-heading">
        <p className="media-eyebrow">YOUR PERSONAL COLLECTION</p>
        <h1>值得珍藏的，<br className="sm:hidden" />都在这里<span className="collection-heading-dot">.</span></h1>
        <p className="collection-description">媒体库 · {previewCount} 个目录，{total.toLocaleString()} 个条目。为每一次观影，保留期待。</p>
      </div>
      <div className="collection-header-tools">
        <Link to="/admin" className="btn-outline">管理媒体库<ArrowUpRight size={15} /></Link>
        <details className="collection-maintenance">
          <summary>维护工具</summary>
          <div className="collection-maintenance-panel">
            <MediaProbeBackfillButton />
            <button type="button" onClick={onRepairRescrape} disabled={repairing} className="btn-outline disabled:cursor-not-allowed disabled:opacity-60" title="从媒体路径回填缺失/错误的外部 ID，再批量重刮整库">
              <RefreshCw size={14} className={repairing ? 'animate-spin' : ''} />
              {repairing ? '正在启动…' : '全库修复+重刮'}
            </button>
          </div>
        </details>
      </div>
      {repairMsg && <p className="collection-status" role="status">{repairMsg}</p>}
    </header>
  )
}

export function LibrariesEmptyState() {
  return (
    <div className="collection-empty-state">
      <div className="collection-empty-icon"><LibraryIcon size={30} strokeWidth={1.3} /></div>
      <p className="media-eyebrow">MAKE ROOM FOR GREAT STORIES</p>
      <h2>你的私人影院，从这里开始</h2>
      <p>添加一个媒体目录，收藏的电影与剧集便有了归处。</p>
      <Link to="/admin" className="btn-primary">添加媒体库<ArrowRight size={16} /></Link>
    </div>
  )
}

export function LibrariesContent({ previews }: { previews: LibraryPreview[] }) {
  const reducedMotion = useReducedMotion()
  return (
    <>
      <section className="collection-directory" aria-label="媒体库入口">
        <div className="collection-section-label"><span>全部媒体库</span><span>{String(previews.length).padStart(2, '0')} COLLECTIONS</span></div>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {previews.map((preview, index) => (
            <motion.div className="min-w-0" key={preview.library.id} initial={reducedMotion ? false : { opacity: 0, y: 16 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: .5, delay: Math.min(index, 6) * .045 }}>
              <LibraryEntryCard preview={preview} index={index} />
            </motion.div>
          ))}
        </div>
      </section>
      <div className="collection-shelves">
        {previews.map((preview) => <LibraryShelf key={preview.library.id} preview={preview} />)}
      </div>
    </>
  )
}

function LibraryEntryCard({ preview, index }: { preview: LibraryPreview; index: number }) {
  const library = preview.library
  const artwork = library.cover_url ? [{ src: library.cover_url, version: library.updated_at }] : libraryArtworkItems(preview.cards)
  const displayPath = libraryDisplayPath(library.path)

  return (
    <Link to={`/library/${library.id}`} className={`collection-entry collection-tone-${index % 4}`}>
      <div className="collection-entry-art" aria-hidden="true">
        <span className="collection-entry-orbit" />
        <div className="collection-entry-emblem">{TYPE_ICONS[library.type] ?? <FolderOpen size={34} />}</div>
        {artwork.length > 0 && <div className={`collection-entry-covers ${artwork.length === 1 ? 'single-cover' : ''}`}>
          {artwork.slice(0, 3).map(({ src, version }, artworkIndex) => (
            <img key={`${src}-${artworkIndex}`} src={imageURL(src, version, { maxWidth: 320, quality: 80 })} alt="" loading="lazy" referrerPolicy="no-referrer" onError={(event) => { event.currentTarget.style.visibility = 'hidden' }} />
          ))}
        </div>}
        <span className="collection-entry-number">{String(index + 1).padStart(2, '0')}</span>
        <span className="collection-entry-arrow"><ArrowUpRight size={18} /></span>
      </div>
      <div className="collection-entry-body">
        <div className="collection-entry-caption"><span>{TYPE_LABELS[library.type] ?? library.type}</span><span>{preview.total.toLocaleString()} 个条目</span></div>
        <h2>{library.name}</h2>
        <p title={library.path}>{displayPath}</p>
      </div>
    </Link>
  )
}

function LibraryShelf({ preview }: { preview: LibraryPreview }) {
  const library = preview.library
  const cards = preview.cards.slice(0, 10)
  const displayPath = libraryDisplayPath(library.path)
  return (
    <section className="collection-shelf">
      <div className="collection-shelf-heading">
        <div className="min-w-0">
          <p className="media-eyebrow">{TYPE_LABELS[library.type] ?? library.type} / RECENTLY ADDED</p>
          <h2>{library.name}</h2>
          <p className="collection-shelf-meta"><span title={library.path}>{displayPath}</span> · {preview.total.toLocaleString()} 个条目 · 最新 {cards.length} 部</p>
        </div>
        <Link to={`/library/${library.id}`} className="collection-view-all">浏览全部<ArrowRight size={15} /></Link>
      </div>
      {cards.length > 0 ? (
        <div className="collection-shelf-scroll">
          {cards.map((card) => <div key={card.key} className="collection-shelf-item"><MediaCard media={card.rep} count={card.count} linkTo={seriesCardLink(card)} /></div>)}
        </div>
      ) : (
        <div className="collection-shelf-empty"><FolderOpen size={22} strokeWidth={1.4} /><p>该目录暂无可展示内容，扫描媒体库后会出现在这里。</p></div>
      )}
    </section>
  )
}

function libraryArtworkItems(cards: SeriesCard[]): Array<{ src: string; version?: string }> {
  return [...cards]
    .sort((a, b) => artworkScore(b.rep) - artworkScore(a.rep) || mediaTime(b.rep) - mediaTime(a.rep))
    .map((card) => ({ src: mediaPrimaryArtworkURL(card.rep), version: card.rep.updated_at }))
    .filter((item) => Boolean(item.src))
    .slice(0, 4)
}
