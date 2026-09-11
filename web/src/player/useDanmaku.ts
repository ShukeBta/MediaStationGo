import Danmaku from 'danmaku'
import { useCallback, useEffect, useRef, useState } from 'react'
import type { RefObject } from 'react'

import { danmakuAPI, describeDanmakuError, type DanmakuPayload } from '../api/danmaku'

// 服务端已完成匹配/去重/屏蔽/密度采样/时间轴修正，这里只做三件事：
//   1. 取数据（并区分"服务不可用"与"这一集没弹幕"）
//   2. 把弹幕交给 danmaku 引擎，让它自己跟着 <video> 走
//   3. 暴露开关/透明度/字号/速度/偏移这些运行时设置
//
// 关键约束：**绝不让 React 接管弹幕 DOM** —— 引擎实例放在 ref 里，
// 用 setState 驱动弹幕会直接把播放页拖垮。

export type DanmakuStatus = 'idle' | 'loading' | 'ready' | 'empty' | 'unmatched' | 'unavailable' | 'error'

export type DanmakuSettings = {
  enabled: boolean
  opacity: number
  fontSize: number
  speed: number
  offsetSeconds: number
}

export type DanmakuController = {
  containerRef: RefObject<HTMLDivElement>
  status: DanmakuStatus
  message: string
  payload: DanmakuPayload | null
  reload: () => void
}

export const DEFAULT_DANMAKU_SETTINGS: DanmakuSettings = {
  enabled: true,
  opacity: 0.85,
  fontSize: 25,
  speed: 144,
  offsetSeconds: 0,
}

// 弹弹play/B站 的模式 -> danmaku 引擎的模式。服务端已过滤掉 7/8/9。
function engineMode(mode: number): 'rtl' | 'ltr' | 'top' | 'bottom' {
  switch (mode) {
    case 4:
      return 'bottom'
    case 5:
      return 'top'
    case 6:
      return 'ltr'
    default:
      return 'rtl'
  }
}

function toEngineComments(payload: DanmakuPayload, fontSize: number) {
  return payload.comments.map((comment) => ({
    text: comment.m,
    time: comment.time,
    mode: engineMode(comment.mode),
    style: {
      color: `#${(comment.color || 0xffffff).toString(16).padStart(6, '0')}`,
      fontSize: `${fontSize}px`,
    },
  }))
}

export function useDanmaku(
  mediaId: string,
  videoRef: RefObject<HTMLVideoElement>,
  settings: DanmakuSettings,
): DanmakuController {
  const containerRef = useRef<HTMLDivElement>(null)
  const instanceRef = useRef<Danmaku | null>(null)
  const [status, setStatus] = useState<DanmakuStatus>('idle')
  const [message, setMessage] = useState('')
  const [payload, setPayload] = useState<DanmakuPayload | null>(null)
  const [reloadToken, setReloadToken] = useState(0)

  const reload = useCallback(() => setReloadToken((value) => value + 1), [])

  // 拉取弹幕。服务端把"没开弹幕"和"这集没匹配到"分成 503/404 两种状态，这里如实呈现。
  useEffect(() => {
    if (!mediaId) {
      setStatus('idle')
      setPayload(null)
      return
    }
    let cancelled = false
    setStatus('loading')
    setMessage('')
    danmakuAPI
      .fetch(mediaId, { offsetSeconds: settings.offsetSeconds || undefined })
      .then((data) => {
        if (cancelled) return
        setPayload(data)
        if (!data.comments.length) {
          setStatus('empty')
          setMessage('这一集没有弹幕')
          return
        }
        setStatus('ready')
      })
      .catch((error: unknown) => {
        if (cancelled) return
        const described = describeDanmakuError(error)
        setPayload(null)
        setStatus(described.kind === 'unmatched' ? 'unmatched' : described.kind === 'unavailable' ? 'unavailable' : 'error')
        setMessage(described.message)
      })
    return () => {
      cancelled = true
    }
    // offsetSeconds 变化由下面的实例重建处理，不在这里重新请求整集数据。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mediaId, reloadToken])

  // 创建/销毁引擎实例。字号与倍速变化需要重建，其余设置是运行时生效。
  useEffect(() => {
    const container = containerRef.current
    const media = videoRef.current
    if (!payload || !container || !media || !payload.comments.length) {
      return
    }
    const instance = new Danmaku({
      container,
      media,
      engine: 'canvas',
      speed: settings.speed,
      comments: toEngineComments(payload, settings.fontSize),
    })
    instanceRef.current = instance
    return () => {
      instance.destroy()
      instanceRef.current = null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [payload, settings.fontSize, settings.speed])

  // 开关、透明度：运行时生效，不重建实例。
  useEffect(() => {
    const instance = instanceRef.current
    if (!instance) return
    if (settings.enabled) {
      instance.show()
    } else {
      instance.hide()
    }
  }, [settings.enabled, status])

  useEffect(() => {
    const container = containerRef.current
    if (container) {
      container.style.opacity = String(settings.opacity)
    }
  }, [settings.opacity, status])

  // 容器尺寸变化（全屏、窗口缩放）必须通知引擎重排。
  useEffect(() => {
    const container = containerRef.current
    if (!container) return
    const observer = new ResizeObserver(() => instanceRef.current?.resize())
    observer.observe(container)
    return () => observer.disconnect()
  }, [])

  return { containerRef, status, message, payload, reload }
}
