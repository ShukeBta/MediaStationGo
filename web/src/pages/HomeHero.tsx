import { Link } from 'react-router-dom'
import { ArrowRight, ArrowUpRight, Play, Sparkles, Star } from 'lucide-react'
import { imageURL } from '../api/client'
import type { SeriesCard } from '../utils/groupSeries'
import { seriesCardLink } from '../utils/groupSeries'
import { mediaPosterURL } from '../utils/mediaArtwork'

export function HomeHero({ card, canPlay, canDiscover, onShuffle, shuffling, shuffleError }: { card: SeriesCard | null; canPlay: boolean; canDiscover: boolean; onShuffle: () => void; shuffling: boolean; shuffleError: string }) {
  const media = card?.rep
  const poster = mediaPosterURL(media)
  return <section className={`home-hero${poster ? ' home-hero-with-poster' : ''}`} aria-label={media ? '片库随心看' : '欢迎来到私人影院'}>
    {media && poster && <div className="home-hero-poster-stage"><img key={`poster-${media.id}-${poster}`} className="home-hero-poster" src={imageURL(poster, media.updated_at, { maxWidth: 640, quality: 90 })} alt={`${media.display_title || media.title}海报`} onError={(e) => { e.currentTarget.style.visibility = 'hidden' }} referrerPolicy="no-referrer" /></div>}
    <div className="home-hero-content">
      <div className="home-hero-badge"><span className="home-hero-live-dot" />{media ? <><Sparkles size={12} />片库随心看</> : 'THE ART OF WATCHING'}<span className="home-hero-badge-rule" />{media ? 'FROM YOUR LIBRARY' : '光影 · 私享'}</div>
      {media ? <h2 className="home-hero-title">{media.display_title || media.title}</h2> : <h2>把世界调成<br /><span>你喜欢的频道。</span></h2>}
      <p>{media?.overview || (media ? '一部值得留出时间的好作品，已收录在你的私人片库。' : '从一帧风景，到一场冒险。收藏热爱，在这里发现下一段难忘的故事。')}</p>
      {media && <div className="home-hero-meta">{media.rating > 0 && <span className="home-hero-rating"><Star size={13} fill="currentColor" />{media.rating.toFixed(1)}</span>}{media.year > 0 && <span>{media.year}</span>}{media.video_codec && <span className="home-hero-format">{media.video_codec}</span>}</div>}
      <div className="home-hero-actions">
        {canPlay && <Link className="home-hero-primary" to={card ? seriesCardLink(card) : '/libraries'}><Play size={15} fill="currentColor" />{card ? '探索这部作品' : '进入我的片库'}<ArrowRight size={15} /></Link>}
        {canDiscover && <Link className="home-hero-secondary" to="/discover">发现更多<ArrowUpRight size={16} /></Link>}
        {canPlay && media && <button type="button" className="home-hero-secondary" onClick={onShuffle} disabled={shuffling} aria-busy={shuffling}>{shuffling ? '正在选片…' : '换一部看看'}<Sparkles size={15} /></button>}
      </div>
      {shuffleError && <p role="status">{shuffleError}</p>}
    </div>
    <div className="home-hero-bottom"><span>MEDIASTATION<span className="home-hero-wordmark-dot">GO</span></span><span className="home-hero-bottom-line" /><span>{media ? '精选推荐' : '专属于你的观影空间'}</span></div>
  </section>
}
