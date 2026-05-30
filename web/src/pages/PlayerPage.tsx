import { useEffect, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import type Hls from 'hls.js'
import { ArrowLeft, RefreshCw, Sparkles } from 'lucide-react'
import toast from 'react-hot-toast'

import { mediaAPI } from '../api/library'
import { api, hlsURL, streamURL } from '../api/client'
import { playbackAPI } from '../api/playback'
import { subtitlesAPI, type SubtitleTrack } from '../api/subtitles'
import { systemAPI } from '../api/system'
import type { Media } from '../types'

type Mode = 'direct' | 'hls'

// Fullscreen, dark-themed video page.
//
//   ?mode=hls       force HLS even when direct play would work
//   ?mode=direct    force direct play (default for browser-friendly codecs)
//
// We pick a sensible default based on the source codec: H.264 + AAC in
// MP4 / WebM containers play directly; everything else (HEVC, MKV, AV1,
// AC3 audio, …) gets routed through ffmpeg → HLS.
//
// External subtitles next to the source file are auto-discovered and
// attached as <track> elements.
export function PlayerPage() {
  const { id = '' } = useParams()
  const [params, setParams] = useSearchParams()
  const navigate = useNavigate()

  const ref = useRef<HTMLVideoElement>(null)
  const hlsRef = useRef<Hls | null>(null)
  const lastSentRef = useRef(0)

  const [media, setMedia] = useState<Media | null>(null)
  const [mode, setMode] = useState<Mode>('direct')
  const [subs, setSubs] = useState<SubtitleTrack[]>([])
  const [hlsUnavailable, setHlsUnavailable] = useState(false)
  const [playerError, setPlayerError] = useState('')
  // 「客户端直连解码」模式：宿主机不转码，播放器强制 direct play、隐藏 HLS 切换。
  const [directOnly, setDirectOnly] = useState(false)

  // 读取宿主机的「直连解码」开关。开启时全程 direct play，不走 HLS。
  useEffect(() => {
    systemAPI
      .info()
      .then((info) => setDirectOnly(Boolean(info.direct_play_only)))
      .catch(() => setDirectOnly(false))
  }, [])

  // Load metadata and pick a default mode.
  useEffect(() => {
    if (!id) return
    mediaAPI.get(id).then((m) => {
      setMedia(m)
      const forced = params.get('mode') as Mode | null
      const auto = pickMode(m)
      // 直连解码模式下忽略 ?mode=hls 与自动判定，始终 direct play。
      setMode(directOnly ? 'direct' : (forced ?? auto))
      setPlayerError('')
    })
    subtitlesAPI
      .list(id)
      .then((tracks) => setSubs(tracks ?? []))
      .catch(() => setSubs([]))
  }, [id, params, directOnly])

  // Wire up the actual <video> element when we know the mode.
  useEffect(() => {
    if (!media || !ref.current) return
    teardownHls()

    const video = ref.current
    if (mode === 'hls') {
      const url = hlsURL(media.id)
      void import('hls.js').then(({ default: HlsCtor }) => {
        if (HlsCtor.isSupported()) {
          const hls = new HlsCtor({ enableWorker: true, lowLatencyMode: false })
          hls.loadSource(url)
          hls.attachMedia(video)
          hls.on(HlsCtor.Events.ERROR, (_, data) => {
            if (data.fatal) {
              setHlsUnavailable(true)
              setPlayerError('HLS 转码不可用，正在尝试直接播放原始文件。若出现有画面无声音，通常是 MKV/AC3/EAC3 音轨需要配置本机 ffmpeg 转码为 AAC。')
              toast.error('HLS 转码失败，尝试切换到直接播放')
              setMode('direct')
              params.set('mode', 'direct')
              setParams(params, { replace: true })
            }
          })
          hlsRef.current = hls
        } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
          video.src = url
        } else {
          setHlsUnavailable(true)
          setPlayerError('当前浏览器不支持 HLS，正在尝试直接播放。')
          toast.error('当前浏览器不支持 HLS，降级到直接播放')
          setMode('direct')
        }
        void video.play().catch(() => undefined)
      }).catch(() => {
        setHlsUnavailable(true)
        setPlayerError('HLS 播放组件加载失败，正在尝试直接播放。')
        setMode('direct')
      })
    } else {
      video.src = streamURL(media.id)
      if (hlsUnavailable && needsTranscodeForBrowser(media)) {
        setPlayerError('当前正在直连播放原始文件；此封装或音轨浏览器兼容性有限，可能只有画面没有声音。请配置本机 ffmpeg 后切回 HLS 转码播放。')
      }
      void video.play().catch(() => undefined)
    }
    return () => teardownHls(media.id, mode === 'hls')
  }, [hlsUnavailable, media, mode, params, setParams])

  // Persist resume position every 10 seconds while playing.
  useEffect(() => {
    if (!media || !ref.current) return
    const video = ref.current
    const handler = () => {
      const now = Date.now()
      if (now - lastSentRef.current < 10_000) return
      lastSentRef.current = now
      const positionMs = Math.floor(video.currentTime * 1000)
      const durationMs = Math.floor((video.duration || 0) * 1000)
      if (positionMs > 0) {
        playbackAPI.recordProgress(media.id, positionMs, durationMs).catch(() => undefined)
      }
    }
    video.addEventListener('timeupdate', handler)
    video.addEventListener('pause', handler)
    return () => {
      video.removeEventListener('timeupdate', handler)
      video.removeEventListener('pause', handler)
    }
  }, [media])

  // ESC = back.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        const target = media?.id || id
        navigate(target ? `/media/${target}` : '/', { replace: true })
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [id, media?.id, navigate])

  const teardownHls = (mediaId?: string, stopServer = false) => {
    if (hlsRef.current) {
      hlsRef.current.destroy()
      hlsRef.current = null
    }
    if (stopServer && mediaId) {
      api.delete(`/hls/${encodeURIComponent(mediaId)}`).catch(() => undefined)
    }
  }

  const goBack = () => {
    const target = media?.id || id
    navigate(target ? `/media/${target}` : '/', { replace: true })
  }

  return (
    <div className="relative -m-6 flex min-h-screen flex-col overflow-hidden bg-black md:-m-8">
      <div className="pointer-events-none absolute inset-x-0 top-0 z-20 flex items-center justify-between p-4 sm:p-6">
        <button
          onClick={goBack}
          className="pointer-events-auto flex items-center gap-2 rounded-full border border-white/15 bg-black/70 px-4 py-2 text-sm font-medium text-white shadow-xl backdrop-blur transition hover:bg-black/85"
        >
          <ArrowLeft size={16} /> 返回
        </button>

        {directOnly ? (
          <span
            className="pointer-events-auto flex items-center gap-2 rounded-full border border-white/15 bg-black/70 px-4 py-2 text-sm font-medium text-white shadow-xl backdrop-blur"
            title="宿主机不转码，由客户端本地解码直连"
          >
            <Sparkles size={14} /> 客户端直连解码
          </span>
        ) : (
          <button
            onClick={() => {
              const next = mode === 'hls' ? 'direct' : 'hls'
              setMode(next)
              params.set('mode', next)
              setParams(params, { replace: true })
            }}
            className="pointer-events-auto flex items-center gap-2 rounded-full border border-white/15 bg-black/70 px-4 py-2 text-sm font-medium text-white shadow-xl backdrop-blur transition hover:bg-black/85"
            title="切换播放模式"
          >
            {mode === 'hls' ? (
              <>
                <RefreshCw size={14} /> HLS 转码中
              </>
            ) : (
              <>
                <Sparkles size={14} /> 直接播放
              </>
            )}
          </button>
        )}
      </div>

      <div className="flex flex-1 items-center justify-center">
        {media ? (
          <video
            ref={ref}
            controls
            autoPlay
            playsInline
            className="max-h-screen w-full max-w-[1600px] bg-black"
            onError={() => {
              // 浏览器对 <video src> 的错误描述非常有限，把详细原因
              // 转给开发者控制台 + 一条 toast；常见原因是 codec 不支持。
              if (mode === 'direct') {
                if (directOnly) {
                  setPlayerError('直接播放失败。当前为「客户端直连解码」模式，宿主机不转码；请使用支持该编码/封装的播放器（如 Infuse / VLC / Emby 客户端）播放，或关闭直连解码模式。')
                  toast.error('直接播放失败（客户端直连解码模式）')
                } else if (hlsUnavailable) {
                  setPlayerError('直接播放失败，且 HLS 转码不可用。请检查文件是否存在，或配置本机 ffmpeg 后使用 HLS 转码播放。')
                  toast.error('直接播放失败，HLS 转码不可用')
                } else {
                  toast.error('直接播放失败，切换到 HLS 转码')
                  setMode('hls')
                  params.set('mode', 'hls')
                  setParams(params, { replace: true })
                }
              } else {
                setPlayerError('视频播放失败，请检查文件是否存在，或确认 ffmpeg 已正确配置。')
                toast.error('视频播放失败，请检查文件是否存在')
              }
            }}
          >
            {subs.map((t, i) => (
              <track
                key={t.path}
                kind="subtitles"
                src={subtitlesAPI.url(media.id, t.path)}
                srcLang={t.lang}
                label={t.label || t.lang}
                default={i === 0}
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
    </div>
  )
}

const directContainers = ['mp4', 'webm', 'm4v']
const directVideoCodecs = ['h264', 'avc', 'avc1']
const directAudioCodecs = ['aac', 'mp3', 'opus']

function pickMode(m: Media): Mode {
  return needsTranscodeForBrowser(m) ? 'hls' : 'direct'
}

function needsTranscodeForBrowser(m: Media): boolean {
  const c = (m.container ?? '').toLowerCase()
  const v = (m.video_codec ?? '').toLowerCase()
  const a = (m.audio_codec ?? '').toLowerCase()
  const containerOK = directContainers.some((x) => c.includes(x))
  const videoOK = !v || directVideoCodecs.some((x) => v.includes(x))
  const audioOK = !a || directAudioCodecs.some((x) => a.includes(x))
  return !(containerOK && videoOK && audioOK)
}
