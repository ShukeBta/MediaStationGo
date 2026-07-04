import type { FormEvent } from 'react'
import { useState } from 'react'
import { Loader2 } from 'lucide-react'
import toast from 'react-hot-toast'

import {
  notifyChannelsAPI,
  type NotifyChannelInput,
} from '../api/notify_channels'
import type { NotifyChannel } from '../types'
import {
  EMPTY_CONFIG,
  EVENT_ALL,
  EVENT_NONE,
  EVENT_OPTIONS,
  type EventMode,
  initialEventMode,
  normalizeInitialConfig,
} from './notifyChannelsModel'
import { Field } from './NotifyChannelFormField'
import { NotifyChannelEventFields } from './NotifyChannelEventFields'
import {
  BarkFields,
  EmailFields,
  TelegramFields,
  WebhookFields,
  WechatFields,
} from './NotifyChannelProviderFields'

type NotifyChannelFormModalProps = {
  editing: NotifyChannel | null
  onClose: () => void
  onSaved: () => void | Promise<void>
}

export function NotifyChannelFormModal({
  editing,
  onClose,
  onSaved,
}: NotifyChannelFormModalProps) {
  const [name, setName] = useState(editing?.name ?? '')
  const [type, setType] = useState<NotifyChannel['type']>(
    editing?.type ?? 'telegram',
  )
  const [config, setConfig] = useState<Record<string, string>>(
    normalizeInitialConfig(editing?.type ?? 'telegram', editing?.config ?? {}),
  )
  const [events, setEvents] = useState<string[]>(editing?.events ?? [])
  const [eventMode, setEventMode] = useState<EventMode>(initialEventMode(editing?.events))
  const [enabled, setEnabled] = useState(editing?.enabled ?? true)
  const [saving, setSaving] = useState(false)

  const onTypeChange = (t: NotifyChannel['type']) => {
    setType(t)
    setConfig({ ...EMPTY_CONFIG[t] })
  }

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault()
    if (type === 'telegram') {
      if (!String(config.bot_token ?? '').trim()) {
        toast.error('请填写 Telegram Bot Token')
        return
      }
      if (!String(config.admin_user_ids ?? '').trim()) {
        toast.error('请填写管理员 Telegram ID')
        return
      }
    }
    const selectedEvents = events.filter((event) => EVENT_OPTIONS.some((item) => item.value === event))
    if (eventMode === 'custom' && selectedEvents.length === 0) {
      toast.error('请至少选择一个推送事件，或选择关闭全部推送事件')
      return
    }
    setSaving(true)
    try {
      const cleanedConfig = Object.fromEntries(
        Object.entries(config).map(([key, value]) => [key, String(value ?? '').trim()]),
      )
      delete cleanedConfig.chat_id
      const input: NotifyChannelInput = {
        name: name.trim(),
        type: type,
        config: cleanedConfig,
        events: eventMode === 'all' ? [EVENT_ALL] : eventMode === 'none' ? [EVENT_NONE] : selectedEvents,
        enabled,
      }
      if (editing) {
        await notifyChannelsAPI.update(editing.id, input)
      } else {
        await notifyChannelsAPI.create(input)
      }
      toast.success('已保存')
      await onSaved()
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '保存失败'
      toast.error(msg)
    } finally {
      setSaving(false)
    }
  }

  const updateConfig = (k: string, v: string) =>
    setConfig((c) => ({ ...c, [k]: v }))

  const toggleEvent = (event: string) => {
    setEvents((current) =>
      current.includes(event)
        ? current.filter((item) => item !== event)
        : [...current.filter((item) => item !== EVENT_ALL && item !== EVENT_NONE), event],
    )
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm">
      <div className="glass-panel w-full max-w-lg max-h-[90vh] overflow-y-auto">
        <h2 className="mb-4 font-display text-xl font-semibold text-ink-600">
          {editing ? '编辑通知渠道' : '添加通知渠道'}
        </h2>
        <form onSubmit={onSubmit} className="space-y-4">
          <Field label="名称">
            <input
              required
              className="input-base"
              placeholder="如: Telegram 通知"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </Field>

          <Field label="渠道类型">
            <select
              className="input-base"
              value={type}
              onChange={(e) => onTypeChange(e.target.value as NotifyChannel['type'])}
            >
              <option value="telegram">Telegram</option>
              <option value="wechat">企业微信 / Server酱</option>
              <option value="bark">Bark (iOS)</option>
              <option value="webhook">Webhook</option>
              <option value="email">Email (SMTP)</option>
            </select>
          </Field>

          {type === 'telegram' && <TelegramFields config={config} updateConfig={updateConfig} />}
          {type === 'wechat' && <WechatFields config={config} updateConfig={updateConfig} />}
          {type === 'bark' && <BarkFields config={config} updateConfig={updateConfig} />}
          {type === 'webhook' && <WebhookFields config={config} updateConfig={updateConfig} />}
          {type === 'email' && <EmailFields config={config} updateConfig={updateConfig} />}

          <NotifyChannelEventFields
            events={events}
            eventMode={eventMode}
            setEventMode={setEventMode}
            toggleEvent={toggleEvent}
          />

          <label className="flex cursor-pointer items-center gap-2 text-sm text-ink-100">
            <input
              type="checkbox"
              className="h-4 w-4 accent-primary-400"
              checked={enabled}
              onChange={(e) => setEnabled(e.target.checked)}
            />
            启用
          </label>

          <div className="flex justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={onClose}
              className="rounded-lg border border-gray-200 px-4 py-2 text-sm text-ink-100 hover:bg-gray-50"
            >
              取消
            </button>
            <button type="submit" disabled={saving} className="neon-button">
              {saving && <Loader2 size={16} className="animate-spin" />} 保存
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
