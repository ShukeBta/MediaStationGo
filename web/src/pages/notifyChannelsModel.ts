import type { NotifyChannel } from '../types'

export const TYPE_LABELS: Record<NotifyChannel['type'], string> = {
  telegram: 'Telegram',
  wechat: '企业微信',
  bark: 'Bark',
  webhook: 'Webhook',
  email: 'Email',
}

export const EVENT_OPTIONS = [
  { value: 'subscription_hit', label: '订阅命中新资源' },
  { value: 'download_complete', label: '下载任务完成' },
  { value: 'library_ingest', label: '入库完成' },
  { value: 'scrape_failed', label: '刮削失败告警' },
  { value: 'system_alert', label: '系统异常通知' },
]

export const EVENT_ALL = '__all__'
export const EVENT_NONE = '__none__'

export type EventMode = 'all' | 'custom' | 'none'

export const EMPTY_CONFIG: Record<NotifyChannel['type'], Record<string, string>> = {
  telegram: {
    bot_token: '',
    admin_user_ids: '',
    group_chat_id: '',
    channel_chat_id: '',
    api_base_url: '',
    proxy_url: '',
  },
  wechat: { sendkey: '' },
  bark: { device_key: '', server: '' },
  webhook: { url: '', method: 'POST', headers: '', body_template: '' },
  email: { smtp_host: '', smtp_port: '465', username: '', password: '', from: '', to: '', tls: 'true' },
}

export function initialEventMode(events: string[] | undefined): EventMode {
  if (events?.includes(EVENT_NONE)) return 'none'
  if (events?.includes(EVENT_ALL) || !events || events.length === 0) return 'all'
  return 'custom'
}

export function eventSummary(events: string[] | undefined): string {
  if (events?.includes(EVENT_NONE)) return '事件：不推送'
  if (events?.includes(EVENT_ALL)) return '事件：全部'
  if (!events || events.length === 0) return '事件：全部'
  const labels = events.map((event) => EVENT_OPTIONS.find((item) => item.value === event)?.label ?? event)
  return `事件：${labels.join('、')}`
}

export function channelSummary(ch: NotifyChannel): string {
  const cfg = ch.config ?? {}
  switch (ch.type) {
    case 'telegram':
      return `Bot ${cfg.bot_token ? '已配置' : '未配置'} → 管理员 ${cfg.admin_user_ids ?? '-'} · 群组 ${cfg.group_chat_id ?? '-'} · 频道 ${cfg.channel_chat_id ?? '-'}`
    case 'wechat':
      return `SendKey ${String(cfg.sendkey ?? '').slice(0, 10)}…`
    case 'bark':
      return `Device ${String(cfg.device_key ?? '').slice(0, 10)}…`
    case 'webhook':
      return `${cfg.method ?? 'POST'} ${cfg.url ?? ''}`
    case 'email':
      return `SMTP ${cfg.smtp_host ?? '-'}:${cfg.smtp_port ?? '-'} → ${cfg.to ?? '-'}`
    default:
      return ''
  }
}

export function normalizeInitialConfig(
  type: NotifyChannel['type'],
  raw: Record<string, unknown>,
): Record<string, string> {
  const base = { ...EMPTY_CONFIG[type] }
  for (const [key, value] of Object.entries(raw ?? {})) {
    base[key] = String(value ?? '')
  }
  if (type === 'telegram' && !base.group_chat_id && !base.channel_chat_id && base.chat_id?.startsWith('-')) {
    base.group_chat_id = base.chat_id
  }
  if (type === 'telegram' && !base.admin_user_ids && base.chat_id && !base.chat_id.startsWith('-')) {
    base.admin_user_ids = base.chat_id
  }
  delete base.chat_id
  return base
}
