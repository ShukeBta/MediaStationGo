import { Loader2, Pencil, Send, Trash2 } from 'lucide-react'

import type { NotifyChannel } from '../types'
import { channelSummary, eventSummary, TYPE_LABELS } from './notifyChannelsModel'

type NotifyChannelCardProps = {
  channel: NotifyChannel
  onTest: () => void
  testing?: boolean
  onEdit: () => void
  onDelete: () => void
}

export function NotifyChannelCard({
  channel,
  onTest,
  testing,
  onEdit,
  onDelete,
}: NotifyChannelCardProps) {
  const summary = channelSummary(channel)
  return (
    <div className="glass-panel flex items-center justify-between gap-3">
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <span className="font-medium text-ink-600">{channel.name}</span>
          <span className="rounded-lg border border-gray-200 bg-gray-50 px-2 py-0.5 text-xs text-ink-50">
            {TYPE_LABELS[channel.type] ?? channel.type}
          </span>
          {!channel.enabled && (
            <span className="rounded-lg bg-sand-500/30 px-2 py-0.5 text-xs text-ink-100">已禁用</span>
          )}
        </div>
        <div className="mt-1 truncate text-xs text-ink-50">{summary}</div>
        <div className="mt-1 truncate text-xs text-sand-500">{eventSummary(channel.events)}</div>
      </div>
      <div className="flex shrink-0 gap-2">
        <button
          onClick={onTest}
          disabled={testing}
          className="rounded-lg border border-gray-200 px-2 py-1 text-xs text-ink-100 hover:border-primary-400/40 hover:text-brand-500"
        >
          {testing ? <Loader2 size={12} className="inline animate-spin" /> : <Send size={12} className="inline" />} 测试
        </button>
        <button
          onClick={onEdit}
          className="rounded-lg border border-gray-200 px-2 py-1 text-xs text-ink-100 hover:border-primary-400/40 hover:text-brand-500"
        >
          <Pencil size={12} className="inline" /> 编辑
        </button>
        <button
          onClick={onDelete}
          className="rounded-lg border border-red-400/40 px-2 py-1 text-xs text-red-400 hover:bg-red-400/10"
        >
          <Trash2 size={12} className="inline" /> 删除
        </button>
      </div>
    </div>
  )
}
