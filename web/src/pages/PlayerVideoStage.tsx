import type { RefObject } from 'react'

import { subtitlesAPI, type SubtitleTrack } from '../api/subtitles'
import type { Media } from '../types'

type PlayerVideoStageProps = {
  media: Media | null
  playerError: string
  subs: SubtitleTrack[]
  videoRef: RefObject<HTMLVideoElement>
  onVideoError: () => void
}

export function PlayerVideoStage({
  media,
  playerError,
  subs,
  videoRef,
  onVideoError,
}: PlayerVideoStageProps) {
  return (
    <div className="flex flex-1 items-center justify-center">
      {media ? (
        <video
          ref={videoRef}
          controls
          autoPlay
          playsInline
          className="max-h-screen w-full max-w-[1600px] bg-black"
          onError={onVideoError}
        >
          {subs.map((track, index) => (
            <track
              key={track.path}
              kind="subtitles"
              src={subtitlesAPI.url(media.id, track.path)}
              srcLang={track.lang}
              label={track.label || track.lang}
              default={index === 0}
            />
          ))}
        </video>
      ) : (
        <p className="text-sand-500">加载中…</p>
      )}
      {playerError ? (
        <div className="absolute bottom-20 left-1/2 w-[min(92vw,720px)] -translate-x-1/2 rounded-2xl border border-white/15 bg-black/75 px-5 py-4 text-sm text-white shadow-2xl backdrop-blur">
          {playerError}
        </div>
      ) : null}
    </div>
  )
}
