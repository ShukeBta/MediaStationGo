import type { RefObject } from 'react'

import type { DanmakuPayload } from '../api/danmaku'
import type { DanmakuStatus } from './useDanmaku'

// 弹幕覆盖层：只负责挂载引擎容器 + 如实显示加载状态。
// 引擎自己往 containerRef 里插节点，React 不再碰它。

type DanmakuOverlayProps = {
  containerRef: RefObject<HTMLDivElement>
  status: DanmakuStatus
  message: string
  payload: DanmakuPayload | null
  onRetry: () => void
}

function noticeFor(status: DanmakuStatus, message: string) {
  const suffix = message ? `：${message}` : ''
  switch (status) {
    case 'unmatched':
      return { text: `未匹配到弹幕${suffix}`, tone: 'warning' as const }
    case 'unavailable':
      return { text: `弹幕服务不可用${suffix}`, tone: 'warning' as const }
    case 'error':
      return { text: `弹幕加载失败${suffix}`, tone: 'danger' as const }
    case 'empty':
      return { text: '这一集没有弹幕', tone: 'muted' as const }
    default:
      return null
  }
}

export function DanmakuOverlay({ containerRef, status, message, payload, onRetry }: DanmakuOverlayProps) {
  const notice = noticeFor(status, message)
  const title = payload ? `${payload.anime_title ?? ''} ${payload.episode_title ?? ''}`.trim() : ''
  return (
    <>
      <div
        ref={containerRef}
        className="pointer-events-none absolute inset-0 z-10 overflow-hidden"
        data-danmaku-status={status}
      />
      {notice ? (
        <div className="pointer-events-none absolute left-1/2 top-20 z-20 flex -translate-x-1/2 items-center gap-3">
          <span
            className={`pointer-events-auto rounded-full border px-4 py-2 text-xs shadow-xl backdrop-blur ${
              notice.tone === 'danger'
                ? 'border-red-400/30 bg-red-950/70 text-red-100'
                : notice.tone === 'warning'
                  ? 'border-amber-300/30 bg-amber-950/70 text-amber-100'
                  : 'border-white/15 bg-black/70 text-white/80'
            }`}
            title={title || undefined}
          >
            {notice.text}
          </span>
          {notice.tone !== 'muted' ? (
            <button
              onClick={onRetry}
              className="pointer-events-auto rounded-full border border-white/15 bg-black/70 px-3 py-2 text-xs text-white shadow-xl backdrop-blur transition hover:bg-black/85"
            >
              重试
            </button>
          ) : null}
        </div>
      ) : null}
    </>
  )
}
