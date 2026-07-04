import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { motion } from 'framer-motion'
import { ArrowRight, Film, FolderOpen, Library as LibraryIcon, Music, PlayCircle, RefreshCw, Tv } from 'lucide-react'

import { imageURL } from '../api/client'
import { EpisodeArtworkToggle } from '../components/EpisodeArtworkToggle'
import { MediaCard } from '../components/MediaCard'
import { artworkScore, seriesCardLink, type SeriesCard } from '../utils/groupSeries'
import { libraryDisplayPath } from './libraryDisplayModel'
import { mediaTime, type LibraryPreview } from './librariesPageModel'

const TYPE_ICONS: Record<string, ReactNode> = {
  movie: <Film size={18} />,
  tv: <Tv size={18} />,
  anime: <PlayCircle size={18} />,
  variety: <Tv size={18} />,
  music: <Music size={18} />,
  adult: <Film size={18} />,
}

const TYPE_LABELS: Record<string, string> = {
  movie: '电影',
  tv: '剧集',
  anime: '动漫',
  variety: '综艺',
  music: '音乐',
  adult: '成人',
}

export function LibrariesHeader({
  previewCount,
  total,
  repairMsg,
  repairEpisodeArtwork,
  repairing,
  onRepairEpisodeArtworkChange,
  onRepairRescrape,
}: {
  previewCount: number
  total: number
  repairMsg: string
  repairEpisodeArtwork: boolean
  repairing: boolean
  onRepairEpisodeArtworkChange: (value: boolean) => void
  onRepairRescrape: () => void
}) {
  return (
    <div className="flex flex-wrap items-end justify-between gap-4">
      <div>
        <h1 className="font-display text-3xl font-bold text-ink-600">媒体库</h1>
        <p className="mt-1 text-sm text-ink-50">
          共 {previewCount} 个目录 · {total.toLocaleString()} 个条目。每个目录直接展示最新入库内容。
        </p>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        {repairMsg && <span className="text-xs text-ink-50">{repairMsg}</span>}
        <EpisodeArtworkToggle
          checked={repairEpisodeArtwork}
          onChange={onRepairEpisodeArtworkChange}
          title="关闭后仍会获取主海报和每集文字元数据，只跳过每集图片"
          className="h-10"
        />
        <button
          type="button"
          onClick={onRepairRescrape}
          disabled={repairing}
          className="btn-outline disabled:cursor-not-allowed disabled:opacity-60"
          title="从媒体路径回填缺失/错误的外部 ID，再批量重刮整库"
        >
          <RefreshCw size={14} className={repairing ? 'animate-spin' : ''} />
          {repairing ? '正在启动…' : '全库修复+重刮'}
        </button>
        <Link to="/admin" className="btn-outline">
          管理媒体库
          <ArrowRight size={14} />
        </Link>
      </div>
    </div>
  )
}

export function LibrariesEmptyState() {
  return (
    <div className="flex flex-col items-center justify-center rounded-3xl border border-dashed border-sand-200 bg-white py-24 text-center">
      <LibraryIcon className="mb-4 h-12 w-12 text-gray-400" />
      <p className="text-sm text-ink-50">暂无媒体库，请到管理后台添加目录。</p>
    </div>
  )
}

export function LibrariesContent({ previews }: { previews: LibraryPreview[] }) {
  return (
    <>
      <section className="space-y-4">
        <div>
          <h2 className="font-display text-2xl font-bold text-ink-600">媒体库入口</h2>
          <p className="text-sm text-ink-50">按目录进入完整媒体库；下方每个目录也会直接展示最新内容。</p>
        </div>
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {previews.map((preview, index) => (
            <motion.div
              key={preview.library.id}
              initial={{ opacity: 0, y: 12 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ delay: index * 0.03 }}
            >
              <LibraryEntryCard preview={preview} />
            </motion.div>
          ))}
        </div>
      </section>

      <section className="space-y-6">
        {previews.map((preview, index) => (
          <motion.div
            key={preview.library.id}
            initial={{ opacity: 0, y: 12 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: index * 0.03 }}
          >
            <LibraryShelf preview={preview} />
          </motion.div>
        ))}
      </section>
    </>
  )
}

function LibraryEntryCard({ preview }: { preview: LibraryPreview }) {
  const library = preview.library
  const artwork = libraryArtworkItems(preview.cards)
  const displayPath = libraryDisplayPath(library.path)

  return (
    <Link
      to={`/library/${library.id}`}
      className="group flex overflow-hidden rounded-3xl border border-sand-200 bg-white p-3 shadow-card transition-all hover:-translate-y-0.5 hover:border-brand-200 hover:shadow-card-hover"
    >
      <div className="grid h-24 w-36 shrink-0 grid-cols-2 gap-1 overflow-hidden rounded-2xl bg-[linear-gradient(135deg,#fff7ed,#f8fafc)]">
        {artwork.length > 0 ? (
          artwork.map(({ src, version }, index) => (
            <img
              key={`${src}-${index}`}
              src={imageURL(src, version)}
              alt=""
              loading="lazy"
              referrerPolicy="no-referrer"
              className="h-full w-full object-cover"
              onError={(event) => { event.currentTarget.style.visibility = 'hidden' }}
            />
          ))
        ) : (
          <div className="col-span-2 flex h-full items-center justify-center text-brand-500">
            {TYPE_ICONS[library.type] ?? <FolderOpen size={34} />}
          </div>
        )}
      </div>
      <div className="flex min-w-0 flex-1 flex-col justify-between px-4 py-1">
        <div>
          <div className="mb-1 inline-flex rounded-full bg-sand-100 px-2 py-0.5 text-[10px] font-bold text-sand-600">
            {TYPE_LABELS[library.type] ?? library.type}
          </div>
          <h2 className="truncate font-display text-xl font-black text-ink-600 group-hover:text-brand-600">
            {library.name}
          </h2>
          <p className="mt-1 line-clamp-1 break-all text-xs text-ink-50" title={library.path}>
            {displayPath}
          </p>
        </div>
        <div className="flex items-center justify-between text-xs font-bold">
          <span className="text-sand-600">{preview.total.toLocaleString()} 个条目</span>
          <span className="text-brand-600">浏览全部</span>
        </div>
      </div>
    </Link>
  )
}

function LibraryShelf({ preview }: { preview: LibraryPreview }) {
  const library = preview.library
  const cards = preview.cards.slice(0, 10)
  const displayPath = libraryDisplayPath(library.path)

  return (
    <section className="rounded-[1.7rem] border border-sand-200 bg-white/75 p-4 shadow-card">
      <div className="mb-4 flex flex-wrap items-end justify-between gap-3">
        <div className="min-w-0">
          <div className="mb-1 inline-flex items-center gap-2 rounded-full bg-brand-50 px-2.5 py-1 text-[11px] font-bold text-brand-700">
            {TYPE_ICONS[library.type] ?? <LibraryIcon size={14} />}
            {TYPE_LABELS[library.type] ?? library.type}
          </div>
          <h2 className="truncate font-display text-2xl font-black text-ink-600">{library.name}</h2>
          <p className="mt-1 line-clamp-1 break-all text-xs text-ink-50">
            <span title={library.path}>{displayPath}</span> · {preview.total.toLocaleString()} 个条目 · 最新 {cards.length} 部
          </p>
        </div>
        <Link to={`/library/${library.id}`} className="btn-outline shrink-0">
          浏览全部
          <ArrowRight size={14} />
        </Link>
      </div>

      {cards.length > 0 ? (
        <div className="flex gap-4 overflow-x-auto pb-2 pr-1">
          {cards.map((card) => (
            <div key={card.key} className="w-[9.5rem] shrink-0 lg:w-[10rem] 2xl:w-[10.5rem]">
              <MediaCard
                media={card.rep}
                count={card.count}
                linkTo={seriesCardLink(card)}
              />
            </div>
          ))}
        </div>
      ) : (
        <div className="rounded-2xl border border-dashed border-sand-200 bg-white px-6 py-10 text-center text-sm text-ink-50">
          该目录暂无可展示内容，扫描媒体库后会出现在这里。
        </div>
      )}
    </section>
  )
}

function libraryArtworkItems(cards: SeriesCard[]): Array<{ src: string; version?: string }> {
  return [...cards]
    .sort((a, b) => artworkScore(b.rep) - artworkScore(a.rep) || mediaTime(b.rep) - mediaTime(a.rep))
    .map((card) => ({
      src: card.rep.poster_url || card.rep.backdrop_url || '',
      version: card.rep.updated_at,
    }))
    .filter((item) => Boolean(item.src))
    .slice(0, 4)
}
