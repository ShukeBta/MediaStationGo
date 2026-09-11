import { ArrowLeft, MessageSquare, MessageSquareOff, RefreshCw, Sparkles } from 'lucide-react'

import type { DanmakuStatus } from '../player/useDanmaku'
import type { PlayerMode } from './playerPageModel'

type PlayerTopBarProps = {
  directOnly: boolean
  mode: PlayerMode
  danmakuEnabled: boolean
  danmakuStatus: DanmakuStatus
  onToggleDanmaku: () => void
  onBack: () => void
  onToggleMode: () => void
}

// 弹幕按钮要如实反映状态：加载中/无弹幕/未匹配/服务不可用 都不该显示成"已开启且正常"。
function danmakuHint(status: DanmakuStatus): string {
  switch (status) {
    case 'loading':
      return '弹幕加载中'
    case 'empty':
      return '这一集没有弹幕'
    case 'unmatched':
      return '未匹配到弹幕，可在详情页手动匹配'
    case 'unavailable':
      return '弹幕服务不可用'
    case 'error':
      return '弹幕加载失败'
    default:
      return ''
  }
}

export function PlayerTopBar({
  directOnly,
  mode,
  danmakuEnabled,
  danmakuStatus,
  onToggleDanmaku,
  onBack,
  onToggleMode,
}: PlayerTopBarProps) {
  const hint = danmakuHint(danmakuStatus)
  const hasDanmaku = danmakuStatus === 'ready'
  return (
    <div className="pointer-events-none absolute inset-x-0 top-0 z-20 flex items-center justify-between p-4 sm:p-6">
      <button
        onClick={onBack}
        className="pointer-events-auto flex items-center gap-2 rounded-full border border-white/15 bg-black/70 px-4 py-2 text-sm font-medium text-white shadow-xl backdrop-blur transition hover:bg-black/85"
      >
        <ArrowLeft size={16} /> 返回
      </button>

      <div className="pointer-events-auto flex items-center gap-2">
        <button
          onClick={onToggleDanmaku}
          title={hint || (danmakuEnabled ? '关闭弹幕' : '开启弹幕')}
          className={`flex items-center gap-2 rounded-full border px-4 py-2 text-sm font-medium shadow-xl backdrop-blur transition ${
            danmakuEnabled && hasDanmaku
              ? 'border-white/15 bg-black/70 text-white hover:bg-black/85'
              : 'border-white/10 bg-black/60 text-white/60 hover:bg-black/75'
          }`}
        >
          {danmakuEnabled ? <MessageSquare size={14} /> : <MessageSquareOff size={14} />}
          弹幕
          {hint ? <span className="text-xs text-white/50">· {hint}</span> : null}
        </button>

        {directOnly ? (
          <span
            className="flex items-center gap-2 rounded-full border border-white/15 bg-black/70 px-4 py-2 text-sm font-medium text-white shadow-xl backdrop-blur"
            title="宿主机不转码，由客户端本地解码直连"
          >
            <Sparkles size={14} /> 客户端直连解码
          </span>
        ) : (
          <button
            onClick={onToggleMode}
            className="flex items-center gap-2 rounded-full border border-white/15 bg-black/70 px-4 py-2 text-sm font-medium text-white shadow-xl backdrop-blur transition hover:bg-black/85"
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
    </div>
  )
}
