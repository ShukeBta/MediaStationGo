import { Play } from 'lucide-react'
import { Link } from 'react-router-dom'

import { imageURL } from '../api/client'
import { PosterArtwork } from '../components/PosterArtwork'
import type { Media } from '../types'
import { mediaBackdropArtworkURL, mediaPosterURL } from '../utils/mediaArtwork'
import '../styles/media.css'

type MediaDetailArtworkProps = { media: Media }

export function MediaDetailBackdrop({ media }: MediaDetailArtworkProps) {
  const backdrop = mediaBackdropArtworkURL(media)
  return (
    <div className="media-detail-backdrop" aria-hidden="true">
      {backdrop && <img src={imageURL(backdrop, media.updated_at, { maxWidth: 1920, quality: 86 })} alt="" decoding="async" referrerPolicy="no-referrer" onError={(event) => { event.currentTarget.style.visibility = 'hidden' }} />}
      <div className="media-detail-backdrop-veil" />
    </div>
  )
}

export function MediaDetailPoster({ media }: MediaDetailArtworkProps) {
  const title = media.display_title?.trim() || media.title
  return (
    <div className="media-detail-poster-column">
      <Link to={`/play/${media.id}`} className="media-detail-poster" aria-label={`播放 ${title}`}>
        <PosterArtwork title={title} poster={mediaPosterURL(media)} version={media.updated_at} year={media.year} maxWidth={640} />
        <span className="media-detail-poster-play" aria-hidden="true"><span className="media-card-play-circle"><Play size={24} fill="currentColor" /></span></span>
      </Link>
      <p className="media-detail-poster-footnote">YOUR PRIVATE SCREENING</p>
    </div>
  )
}
