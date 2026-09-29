import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { BellRing, Layers, Play, Star } from 'lucide-react'
import type { Media } from '../types'
import { mediaPosterURL } from '../utils/mediaArtwork'
import { PosterArtwork } from './PosterArtwork'
import '../styles/media.css'

export const MediaCard = ({
  media, progress, count, rating, linkTo, onClick, actions, autoFollow = false,
}: {
  media: Media
  progress?: number
  count?: number
  rating?: number
  linkTo?: string
  onClick?: () => void
  actions?: ReactNode
  autoFollow?: boolean
}) => {
  const href = linkTo ?? `/media/${media.id}`
  const title = media.display_title?.trim() || media.title
  const displayRating = rating ?? media.rating
  const versionCount = media.versions?.length ?? 0
  const partCount = media.parts?.length ?? 0
  const badge = count !== undefined && count > 1 ? `${count} 集`
    : count === undefined && partCount > 1 ? `${partCount} 片段`
    : count === undefined && versionCount > 1 ? `${versionCount} 版本` : ''

  const card = (
    <>
      <div className="media-card-poster">
        <PosterArtwork title={title} poster={mediaPosterURL(media)} version={media.updated_at} year={media.year} />
        <div className="media-card-shade" aria-hidden="true" />
        <div className="media-card-topline">
          {displayRating > 0 && (
            <span className="media-card-badge media-card-rating" aria-label={`评分 ${displayRating.toFixed(1)}`}>
              <Star size={10} fill="currentColor" /> {displayRating.toFixed(1)}
            </span>
          )}
          {badge && <span className="media-card-badge media-card-count"><Layers size={11} />{badge}</span>}
        </div>
        <div className="media-card-play" aria-hidden="true">
          <span className="media-card-play-circle"><Play size={20} fill="currentColor" strokeWidth={1.5} /></span>
          <span>{onClick || (count && count > 1) ? '查看内容' : '探索影片'}</span>
        </div>
        <div className="media-card-bottomline">
          {autoFollow && <span className="media-card-badge media-card-follow"><BellRing size={10} />自动追更</span>}
          {(media.douban_rating ?? 0) > 0 && <span className="media-card-badge media-card-douban">豆 {media.douban_rating!.toFixed(1)}</span>}
        </div>
        {progress !== undefined && progress > 0 && progress < 1 && (
          <div className="media-card-progress" role="progressbar" aria-label="观看进度" aria-valuenow={Math.round(progress * 100)} aria-valuemin={0} aria-valuemax={100}>
            <span style={{ width: `${Math.round(progress * 100)}%` }} />
          </div>
        )}
      </div>
      <div className="media-card-caption">
        <p className="media-card-title" title={title}>{title}</p>
        <div className="media-card-meta">
          <span>{media.year > 0 ? media.year : '私人片库'}</span>
          <span className="media-card-formats">
            {media.adult_type && <span>{media.adult_type}</span>}
            {media.video_codec && <span>{media.video_codec}</span>}
          </span>
        </div>
      </div>
    </>
  )

  return (
    <div className="media-card group">
      {onClick ? (
        <button type="button" onClick={onClick} className="media-card-link" aria-label={`查看 ${title}`}>{card}</button>
      ) : (
        <Link to={href} className="media-card-link" aria-label={`查看 ${title}`}>{card}</Link>
      )}
      {actions && <div className="media-card-actions">{actions}</div>}
    </div>
  )
}
