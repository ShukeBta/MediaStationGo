import { useState } from 'react'
import { Link } from 'react-router-dom'
import { ArrowDown, ArrowRight, ArrowUpRight, Check, Clock3, Film, LibraryBig, Play, Plus, RefreshCw, Tv2 } from 'lucide-react'
import { imageURL } from '../api/client'
import { MediaCard } from '../components/MediaCard'
import type { HistoryItem } from '../api/playback'
import type { Library, Media } from '../types'
import type { SeriesCard } from '../utils/groupSeries'
import { seriesCardLink } from '../utils/groupSeries'
import { mediaBackdropURL, mediaPosterURL } from '../utils/mediaArtwork'
import { filterRecentCards, playbackProgress, type RecentFilter } from './homePageModel'

export function HomeLoadingState() {
  return <div className="home-loading" role="status" aria-label="正在加载首页">
    <span className="cinema-eyebrow">YOUR PRIVATE CINEMA</span>
    <div className="home-skeleton home-skeleton-heading" />
    <div className="home-skeleton home-skeleton-hero" />
    <div className="home-skeleton-grid">{[0, 1, 2, 3, 4, 5].map((i) => <div key={i} className="home-skeleton" />)}</div>
    <span className="sr-only">首页内容准备中…</span>
  </div>
}

export function HomeEmptyState({ canManage, hasLibraries }: { canManage: boolean; hasLibraries: boolean }) {
  return <section className="home-empty">
    <div className="home-empty-icon"><LibraryBig size={28} strokeWidth={1.3} /></div>
    <div><span className="cinema-eyebrow">THE FIRST FRAME</span>
      <h2>{hasLibraries ? '好故事，正在等待入场。' : '你的私人影院，从这里开始。'}</h2>
      <p>{canManage ? (hasLibraries ? '扫描媒体目录，收藏的电影与剧集就会出现在这里。' : '添加第一个媒体库，让每一部收藏都有自己的位置。') : '这里还没有可观看的内容，请联系管理员添加媒体库或调整访问权限。'}</p>
    </div>
    {canManage && <Link to="/admin" className="btn-outline"><Plus size={16} />{hasLibraries ? '管理媒体库' : '添加媒体库'}</Link>}
  </section>
}

export function HomeLoadError({ message, onRetry }: { message: string; onRetry: () => void }) {
  return <div className="home-error" role="alert">
    <div className="home-empty-icon"><Film size={30} strokeWidth={1.3} /></div>
    <span className="cinema-eyebrow">LET’S TRY AGAIN</span><h1>首页内容暂时未能加载</h1>
    <p>再试一次，让好故事继续。</p><details><summary>查看错误详情</summary><p>{message}</p></details>
    <button type="button" onClick={onRetry} className="btn-primary"><RefreshCw size={16} />重新加载</button>
  </div>
}

export function HomeWelcome({ name, libraryCount, canPlay }: { name: string; libraryCount: number; canPlay: boolean }) {
  return <div className="home-welcome">
    <div><span className="cinema-eyebrow">YOUR PRIVATE CINEMA</span><h1>好戏，随时开场<span className="home-title-dot">.</span></h1>
      <p>欢迎回来，{name}。把时间留给喜欢的故事。</p>
    </div>
    {canPlay && <Link to="/libraries" className="home-library-count"><span className="home-count-icon"><LibraryBig size={20} strokeWidth={1.5} /></span>
      <span><strong>{libraryCount.toString().padStart(2, '0')}</strong><small>个媒体库</small></span><ArrowUpRight size={17} />
    </Link>}
  </div>
}

export function HomeLibraryShortcuts({ libraries }: { libraries: Library[] }) {
  if (libraries.length === 0) return null
  return <nav className="home-library-shortcuts" aria-label="快速进入媒体库">
    {libraries.slice(0, 4).map((lib) => {
      const Icon = ['tv', 'anime', 'variety'].includes(lib.type) ? Tv2 : Film
      const subtitle: Record<string, string> = { movie: 'FILM COLLECTION', tv: 'SERIES COLLECTION', anime: 'ANIMATION', adult: 'PRIVATE COLLECTION', variety: 'VARIETY SHOWS', music: 'MUSIC COLLECTION' }
      return <Link key={lib.id} to={`/library/${lib.id}`} className="home-library-shortcut">
        <span className="home-library-shortcut-icon"><Icon size={20} strokeWidth={1.4} /></span>
        <span className="home-library-shortcut-label"><small>{subtitle[lib.type] || 'YOUR COLLECTION'}</small><strong>{lib.name}</strong></span>
        <ArrowUpRight size={15} className="home-shortcut-arrow" />
      </Link>
    })}
    <Link to="/libraries" className="home-shortcuts-more" aria-label="查看全部媒体库"><ArrowRight size={18} /></Link>
  </nav>
}

export function ContinueWatchingSection({ history }: { history: HistoryItem[] }) {
  return <section>
    <div className="cinema-section-heading"><div><span className="cinema-eyebrow">PICK UP WHERE YOU LEFT OFF</span><h2>故事，接着看</h2></div>
      <Link to="/history" className="cinema-text-link">观看历史<ArrowRight size={14} /></Link></div>
    <div className="home-continue-grid">{history.slice(0, 4).map((h) => h.media && <ContinueCard key={h.id} media={h.media} progress={playbackProgress(h.position_ms, h.duration_ms)} />)}</div>
  </section>
}

const recentFilters: { key: RecentFilter; label: string }[] = [{ key: 'all', label: '全部' }, { key: 'movie', label: '电影' }, { key: 'series', label: '剧集' }, { key: 'anime', label: '动漫' }]

export function RecentMediaSection({ recentCards, libraries }: { recentCards: SeriesCard[]; libraries: Library[] }) {
  const [filter, setFilter] = useState<RecentFilter>('all')
  const cards = filterRecentCards(recentCards, libraries, filter)
  return <section id="recent-media" className="home-recent">
    <div className="cinema-section-heading"><div><span className="cinema-eyebrow">NEW IN YOUR COLLECTION</span><h2>最近入库 <span className="home-section-count">{recentCards.length}</span></h2></div>
      <Link to="/poster-wall" className="cinema-text-link">打开海报墙<ArrowUpRight size={14} /></Link></div>
    <div className="home-recent-toolbar"><div className="home-filter-tabs" role="group" aria-label="筛选最近入库类型">
      {recentFilters.map((item) => <button type="button" key={item.key} onClick={() => setFilter(item.key)} aria-pressed={filter === item.key} className={filter === item.key ? 'is-active' : ''}>{filter === item.key && <Check size={12} />}{item.label}</button>)}
    </div><span className="home-sort-label"><ArrowDown size={12} />按入库时间</span></div>
    {cards.length > 0 ? <div className="home-poster-grid" key={filter}>{cards.map((card, i) => <div className="home-poster-entry" key={card.key} style={{ animationDelay: `${Math.min(i, 8) * 35}ms` }}><MediaCard media={card.rep} count={card.count} linkTo={seriesCardLink(card)} /></div>)}</div>
      : <div className="home-filter-empty"><Film size={24} strokeWidth={1.3} /><p>近期还没有添加{recentFilters.find((item) => item.key === filter)?.label}内容</p><button type="button" onClick={() => setFilter('all')} className="cinema-text-link">查看全部<ArrowRight size={14} /></button></div>}
  </section>
}

function ContinueCard({ media, progress }: { media: Media; progress: number }) {
  const artwork = mediaBackdropURL(media) || mediaPosterURL(media)
  const [failed, setFailed] = useState(false)
  return <Link to={`/play/${media.id}`} className="home-continue-card">
    <div className="home-continue-visual">
      {artwork && !failed ? <img src={imageURL(artwork, media.updated_at, { maxWidth: 640, quality: 80 })} alt="" loading="lazy" decoding="async" referrerPolicy="no-referrer" onError={() => setFailed(true)} /> : <span className="home-continue-fallback">{media.title.slice(0, 2)}</span>}
      <span className="home-continue-play"><Play size={20} fill="currentColor" /></span><span className="home-continue-progress"><span style={{ width: `${progress}%` }} /></span>
    </div>
    <div className="home-continue-info"><h3>{media.display_title || media.title}</h3><span><Clock3 size={11} />已观看 {progress}%</span></div>
  </Link>
}
