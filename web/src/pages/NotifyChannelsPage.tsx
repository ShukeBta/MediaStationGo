import { useEffect, useState } from 'react'
import { Bell, Loader2, Play, Plus, Square } from 'lucide-react'
import toast from 'react-hot-toast'

import { notifyChannelsAPI } from '../api/notify_channels'
import { confirmAction } from '../components/confirmAction'
import type { NotifyChannel } from '../types'
import { NotifyChannelCard } from './NotifyChannelCard'
import { NotifyChannelFormModal } from './NotifyChannelFormModal'

// NotifyChannelsPage replaces the Vue NotifyTab. Operators can register
// multiple Telegram bots / Bark devices / WeChat keys / Webhooks and
// fire a test notification on demand.
export function NotifyChannelsPage() {
  const [channels, setChannels] = useState<NotifyChannel[]>([])
  const [loading, setLoading] = useState(true)
  const [editing, setEditing] = useState<NotifyChannel | null>(null)
  const [showForm, setShowForm] = useState(false)
  const [testingID, setTestingID] = useState<string | null>(null)
  const [pollingBusy, setPollingBusy] = useState<'start' | 'stop' | null>(null)

  const refresh = async () => {
    setLoading(true)
    try {
      setChannels(await notifyChannelsAPI.list())
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    refresh().catch(() => undefined)
  }, [])

  const onTest = async (id: string) => {
    setTestingID(id)
    try {
      await notifyChannelsAPI.test(id)
      toast.success('测试消息已发送')
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '发送失败'
      toast.error(msg)
    } finally {
      setTestingID(null)
    }
  }

  const onStartPolling = async () => {
    setPollingBusy('start')
    try {
      const res = await notifyChannelsAPI.startTelegramPolling()
      if ((res.started ?? 0) > 0) {
        toast.success(`已启动 ${res.started} 个 Telegram Bot 轮询`)
      } else if ((res.already_running ?? 0) > 0) {
        toast.success('Telegram Bot 轮询已在运行')
      } else {
        toast.error(res.errors?.[0] ?? '没有可启动的 Telegram Bot 渠道')
      }
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '启动轮询失败'
      toast.error(msg)
    } finally {
      setPollingBusy(null)
    }
  }

  const onStopPolling = async () => {
    setPollingBusy('stop')
    try {
      const res = await notifyChannelsAPI.stopTelegramPolling()
      toast.success(res.stopped > 0 ? `已停止 ${res.stopped} 个 Telegram Bot 轮询` : '当前没有运行中的轮询')
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '停止轮询失败'
      toast.error(msg)
    } finally {
      setPollingBusy(null)
    }
  }

  const onDelete = async (ch: NotifyChannel) => {
    if (!(await confirmAction({ title: '删除通知渠道', message: `确定删除「${ch.name}」?`, confirmText: '删除' }))) return
    try {
      await notifyChannelsAPI.remove(ch.id)
      toast.success('已删除')
      await refresh()
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '删除失败'
      toast.error(msg)
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-amber-400/10 text-amber-300">
            <Bell size={20} />
          </div>
          <div>
            <h1 className="font-display text-3xl font-bold text-ink-600">通知渠道</h1>
            <p className="text-sm text-ink-50">
              配置 Telegram / Bark / 企业微信 / Webhook 多通道推送
            </p>
          </div>
        </div>
        <div className="flex flex-wrap justify-end gap-2">
          <button
            onClick={onStartPolling}
            disabled={pollingBusy !== null}
            className="rounded-xl border border-gray-200 px-3 py-2 text-sm text-ink-100 hover:border-primary-400/40 hover:text-brand-500 disabled:opacity-50"
            title="启动 Telegram Bot 长轮询"
          >
            {pollingBusy === 'start' ? <Loader2 size={16} className="inline animate-spin" /> : <Play size={16} className="inline" />}
            {' '}启动 Bot 轮询
          </button>
          <button
            onClick={onStopPolling}
            disabled={pollingBusy !== null}
            className="rounded-xl border border-gray-200 px-3 py-2 text-sm text-ink-100 hover:border-primary-400/40 hover:text-brand-500 disabled:opacity-50"
            title="停止本地长轮询。"
          >
            {pollingBusy === 'stop' ? <Loader2 size={16} className="inline animate-spin" /> : <Square size={16} className="inline" />}
            {' '}停止轮询
          </button>
          <button
            onClick={() => {
              setEditing(null)
              setShowForm(true)
            }}
            className="neon-button"
          >
            <Plus size={16} /> 添加渠道
          </button>
        </div>
      </div>

      {loading && (
        <div className="flex justify-center py-12 text-ink-50">
          <Loader2 className="animate-spin" />
        </div>
      )}

      {!loading && channels.length === 0 && (
        <div className="glass-panel py-12 text-center text-ink-50">暂无通知渠道</div>
      )}

      {!loading && channels.length > 0 && (
        <div className="space-y-3">
          {channels.map((ch) => (
            <NotifyChannelCard
              key={ch.id}
              channel={ch}
              onTest={() => onTest(ch.id)}
              testing={testingID === ch.id}
              onEdit={() => {
                setEditing(ch)
                setShowForm(true)
              }}
              onDelete={() => onDelete(ch)}
            />
          ))}
        </div>
      )}

      {showForm && (
        <NotifyChannelFormModal
          editing={editing}
          onClose={() => setShowForm(false)}
          onSaved={async () => {
            setShowForm(false)
            await refresh()
          }}
        />
      )}
    </div>
  )
}
