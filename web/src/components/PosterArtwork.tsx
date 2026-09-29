import { useState } from 'react'
import { Film } from 'lucide-react'
import { imageURL } from '../api/client'
import '../styles/media.css'

type PosterArtworkProps = {
  title: string
  poster?: string
  version?: string
  year?: number
  maxWidth?: number
}

/** One consistent, legible cover treatment for missing and failed artwork. */
export function PosterArtwork({ title, poster, version, year, maxWidth = 360 }: PosterArtworkProps) {
  const src = imageURL(poster || '', version, { maxWidth, quality: maxWidth > 360 ? 88 : 80 })
  const [failedSrc, setFailedSrc] = useState('')
  const [landscapeSrc, setLandscapeSrc] = useState('')
  const letters = Array.from(title.trim().replace(/^[\s\p{P}]+/u, '')).slice(0, 2).join('') || '映'
  const hue = Array.from(title).reduce((value, letter) => value + (letter.codePointAt(0) ?? 0), 0) % 4
  const hasPoster = Boolean(poster && failedSrc !== src)

  return (
    <div className={`poster-artwork poster-palette-${hue}`}>
      {hasPoster ? (
        <>
          {landscapeSrc === src && <img src={src} alt="" aria-hidden="true" className="poster-artwork-blur" loading="lazy" referrerPolicy="no-referrer" />}
          <img
            src={src}
            alt={title}
            loading="lazy"
            decoding="async"
            className={`poster-artwork-image ${landscapeSrc === src ? 'is-landscape' : ''}`}
            referrerPolicy="no-referrer"
            onLoad={(event) => setLandscapeSrc(event.currentTarget.naturalWidth > event.currentTarget.naturalHeight ? src : '')}
            onError={() => setFailedSrc(src)}
          />
        </>
      ) : (
        <div className="poster-artwork-fallback" role="img" aria-label={`${title} · 暂无海报`}>
          <span className="poster-artwork-frame" aria-hidden="true" />
          <div className="poster-artwork-edition" aria-hidden="true"><Film size={13} strokeWidth={1.4} /><span>MEDIA / ARCHIVE</span></div>
          <span className="poster-artwork-monogram" aria-hidden="true">{letters}</span>
          <div className="poster-artwork-credit" aria-hidden="true"><span>{title}</span><span>{year && year > 0 ? year : 'COLLECTION'}</span></div>
        </div>
      )}
    </div>
  )
}
