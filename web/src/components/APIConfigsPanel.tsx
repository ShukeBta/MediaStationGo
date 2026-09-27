import { FormEvent, useEffect, useState } from 'react'
import toast from 'react-hot-toast'
import { Eye, KeyRound, LoaderCircle, Pencil, RefreshCw, Save, Trash2, X } from 'lucide-react'

import { apiConfigsAPI, type APIConfig } from '../api/api_configs'
import { confirmAction } from './confirmAction'
import { AIConfigFields } from './AIConfigFields'
import { aiUsesKeylessLocalService, isAIProvider } from './aiConfigModel'

// Compact inline-editable provider table for use inside AdminPage's "外部API" tab.
export function APIConfigsPanel() {
  const [items, setItems] = useState<APIConfig[]>([])
  const [loading, setLoading] = useState(true)
  const [editing, setEditing] = useState<string | null>(null)

  const refresh = () =>
    apiConfigsAPI
      .list()
      .then(setItems)
      .finally(() => setLoading(false))

  useEffect(() => {
    refresh().catch(() => undefined)
  }, [])

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <KeyRound className="h-5 w-5 text-brand-500" />
        <div>
          <p className="font-display text-lg font-semibold text-ink-600">外部 API 配置</p>
          <p className="text-xs text-ink-50">
            TMDb / Bangumi / TheTVDB / Fanart / OpenAI / Douban / Adult / FD2PPV 密钥与源管理
            · AES-GCM 加密存储
          </p>
        </div>
      </div>

      {loading && (
        <p className="py-6 text-center text-sm text-sand-500">加载中…</p>
      )}

      {!loading && (
        <div className="glass-panel overflow-hidden">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-gray-200 text-xs uppercase tracking-wider text-sand-500">
              <tr>
                <th className="px-4 py-3">服务</th>
                <th className="px-4 py-3">密钥</th>
                <th className="px-4 py-3">状态</th>
                <th className="px-4 py-3 text-right">操作</th>
              </tr>
            </thead>
            <tbody>
              {items.map((item) =>
                editing === item.provider ? (
                  <EditingRow
                    key={item.id}
                    item={item}
                    onCancel={() => setEditing(null)}
                    onSaved={() => {
                      setEditing(null)
                      refresh()
                    }}
                  />
                ) : (
                  <tr
                    key={item.id}
                    className="border-t border-gray-200 transition hover:bg-gray-50"
                  >
                    <td className="px-4 py-3">
                      <p className="font-medium text-ink-600">{isAIProvider(item.provider) ? 'AI / 大语言模型' : item.provider}</p>
                      {item.description && (
                        <p className="text-xs text-sand-500">{item.description}</p>
                      )}
                      {isAICompatibleProvider(item.provider) && item.model && (
                        <p className="mt-1 text-xs text-brand-500">模型：{item.model}</p>
                      )}
                    </td>
                    <td className="px-4 py-3 font-mono text-xs">
                      {apiConfigConfigured(item) ? (
                        <span className="text-brand-500">{item.masked_key || (aiUsesKeylessLocalService(item.provider, item.extra) ? '无需密钥' : '')}</span>
                      ) : (
                        <span className="text-gray-500">未配置</span>
                      )}
                      {item.provider === 'adult' && apiConfigSourceCount(item) > 0 ? (
                        <span className="text-brand-500">{apiConfigSourceCount(item)} 个源</span>
                      ) : null}
                    </td>
                    <td className="px-4 py-3">
                      {apiConfigConfigured(item) ? (
                        <span className="inline-flex items-center gap-1 rounded-full bg-emerald-400/10 px-2 py-0.5 text-xs text-emerald-400">
                          已配置
                        </span>
                      ) : (
                        <span className="inline-flex items-center gap-1 rounded-full bg-sand-300/40 px-2 py-0.5 text-xs text-sand-500">
                          未配置
                        </span>
                      )}
                    </td>
                    <td className="px-4 py-3 text-right">
                      <div className="flex items-center justify-end gap-1">
                        <button
                          onClick={() => setEditing(item.provider)}
                          className="rounded-lg p-1.5 text-ink-50 transition hover:bg-gray-50 hover:text-white"
                          title="编辑"
                        >
                          <Pencil size={14} />
                        </button>
                        <button
                          onClick={() => {
                            toast(
                              `已配置 ${item.has_key ? '✓' : '✗'} 密钥 (在线测试请用对应功能页面)`,
                            )
                          }}
                          className="rounded-lg p-1.5 text-ink-50 transition hover:bg-gray-50 hover:text-ink-200"
                          title="查看状态"
                        >
                          <Eye size={14} />
                        </button>
                        {item.has_key && (
                          <button
                            onClick={async () => {
                              if (!(await confirmAction({ title: '清除 API Key', message: `确定清除 ${item.provider} 的 API Key?`, confirmText: '清除' }))) return
                              await apiConfigsAPI.remove(item.provider)
                              toast.success('已清除')
                              refresh()
                            }}
                            className="rounded-lg p-1.5 text-ink-50 transition hover:bg-red-400/10 hover:text-red-400"
                            title="清除密钥"
                          >
                            <Trash2 size={14} />
                          </button>
                        )}
                      </div>
                    </td>
                  </tr>
                ),
              )}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

function EditingRow({
  item,
  onCancel,
  onSaved,
}: {
  item: APIConfig
  onCancel: () => void
  onSaved: () => void
}) {
  const [apiKey, setAPIKey] = useState('')
  const [baseURL, setBaseURL] = useState(item.base_url ?? '')
  const [extra, setExtra] = useState(item.extra ?? '')
  const [model, setModel] = useState(item.model ?? '')
  const [models, setModels] = useState<Array<{ id: string; owned_by?: string }>>([])
  const [enabled, setEnabled] = useState(item.enabled)
  const [saving, setSaving] = useState(false)
  const [detecting, setDetecting] = useState(false)
  const isAdult = item.provider === 'adult'
  const isAI = isAIProvider(item.provider)
  const isFD2PPV = item.provider === 'fd2ppv'
  const isAICompatible = isAICompatibleProvider(item.provider)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (isAICompatible && !model.trim()) {
      toast.error('请先探测或填写模型')
      return
    }
    setSaving(true)
    try {
      const patch: Record<string, unknown> = { base_url: baseURL, enabled }
      if (isAdult || isAI) patch.extra = extra
      if (isFD2PPV) patch.extra = extra.trim()
      if (isAICompatible && model.trim()) patch.model = model.trim()
      if (apiKey.trim()) patch.api_key = apiKey.trim()
      await apiConfigsAPI.update(item.provider, patch)
      toast.success(`${item.provider} 已保存`)
      onSaved()
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '保存失败'
      toast.error(msg)
    } finally {
      setSaving(false)
    }
  }

  const discoverModels = async () => {
    setDetecting(true)
    try {
      const items = await apiConfigsAPI.discoverModels(item.provider, {
        base_url: baseURL.trim(),
        ...(apiKey.trim() ? { api_key: apiKey.trim() } : {}),
      })
      setModels(items)
      setModel((current) => preferredDetectedModel(current, baseURL, items))
      toast.success(`探测到 ${items.length} 个模型`)
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '模型探测失败'
      toast.error(msg)
    } finally {
      setDetecting(false)
    }
  }

  return (
    <tr className="border-t border-gray-200 bg-primary-400/5">
      <td colSpan={4} className="px-4 py-3">
        <form onSubmit={submit} className="flex flex-wrap items-end gap-3">
          <span className="text-sm font-medium text-ink-600">{isAI ? 'AI / 大语言模型' : item.provider}</span>
          {isAI && <AIConfigFields provider={item.provider} extra={extra} onExtraChange={setExtra} baseURL={baseURL} onBaseURLChange={setBaseURL} />}
          {!isAdult && (
            <label className="flex-1 text-xs text-ink-50">
              {isFD2PPV ? '密码' : 'API Key'}
              <input
                className="input-base mt-1"
                type="password"
                placeholder={item.has_key ? '•••••••••••• (留空保留原值)' : isFD2PPV ? '输入 FD2PPV 密码' : '输入密钥'}
                value={apiKey}
                onChange={(e) => setAPIKey(e.target.value)}
              />
            </label>
          )}
          {!isFD2PPV && (
            <label className="flex-1 text-xs text-ink-50">
              {isAdult ? '主源 URL' : 'Base URL'}
              <input
                className="input-base mt-1"
                placeholder={isAdult ? 'https://javdb.com' : isAI ? 'https://api.openai.com/v1' : 'https://api.themoviedb.org/3'}
                value={baseURL}
                onChange={(e) => setBaseURL(e.target.value)}
              />
            </label>
          )}
          {isAdult && (
            <label className="min-w-64 flex-1 text-xs text-ink-50">
              备用源 URL
              <textarea
                className="input-base mt-1 min-h-20 resize-y"
                placeholder={'https://javbus.sbs\nhttps://www.javbus.com'}
                value={extra}
                onChange={(e) => setExtra(e.target.value)}
              />
            </label>
          )}
          {isFD2PPV && (
            <label className="flex-1 text-xs text-ink-50">
              账号
              <input
                className="input-base mt-1"
                autoComplete="username"
                placeholder="输入 FD2PPV 账号"
                value={extra}
                onChange={(e) => setExtra(e.target.value)}
              />
            </label>
          )}
          {isAICompatible && (
            <label className="min-w-64 flex-1 text-xs text-ink-50">
              模型
              <div className="mt-1 flex gap-2">
                {models.length > 0 ? (
                  <select className="input-base min-w-0 flex-1" value={model} onChange={(e) => setModel(e.target.value)}>
                    {models.map((candidate) => (
                      <option key={candidate.id} value={candidate.id}>
                        {candidate.id}{candidate.owned_by ? ` · ${candidate.owned_by}` : ''}
                      </option>
                    ))}
                  </select>
                ) : (
                  <input
                    className="input-base min-w-0 flex-1"
                    placeholder="先探测，或手动填写模型 ID"
                    value={model}
                    onChange={(e) => setModel(e.target.value)}
                  />
                )}
                <button
                  type="button"
                  onClick={() => void discoverModels()}
                  disabled={detecting || !baseURL.trim()}
                  className="btn-outline shrink-0 px-3"
                >
                  {detecting ? <LoaderCircle size={14} className="animate-spin" /> : <RefreshCw size={14} />}
                  探测模型
                </button>
              </div>
            </label>
          )}
          <label className="flex items-center gap-2 text-xs text-ink-50">
            <input
              type="checkbox"
              checked={enabled}
              onChange={(e) => setEnabled(e.target.checked)}
            />
            启用
          </label>
          <button type="submit" disabled={saving} className="neon-button !px-3 !py-1.5 !text-xs">
            <Save size={12} /> 保存
          </button>
          <button
            type="button"
            onClick={onCancel}
            className="rounded-lg border border-sand-400/30 px-2 py-1.5 text-xs text-ink-50 hover:text-white"
          >
            <X size={12} />
          </button>
        </form>
      </td>
    </tr>
  )
}

function preferredDetectedModel(
  current: string,
  baseURL: string,
  items: Array<{ id: string }>,
): string {
  if (items.some((item) => item.id === current)) return current
  const ids = items.map((item) => item.id)
  if (baseURL.toLowerCase().includes('deepseek')) {
    const deepseekChat = ids.find((id) => id.toLowerCase() === 'deepseek-chat')
    if (deepseekChat) return deepseekChat
  }
  return ids.find((id) => id.toLowerCase().includes('flash')) ?? ids[0] ?? current
}

function apiConfigConfigured(item: APIConfig): boolean {
  if (aiUsesKeylessLocalService(item.provider, item.extra)) return true
  if (item.provider === 'adult') {
    return Boolean(item.base_url?.trim() || item.extra?.trim())
  }
  if (item.provider === 'fd2ppv') {
    return Boolean(item.has_key && item.extra?.trim())
  }
  return item.has_key
}

function apiConfigSourceCount(item: APIConfig): number {
  if (item.provider !== 'adult') return 0
  return [item.base_url, item.extra]
    .join('\n')
    .split(/[\s,;]+/)
    .map((value) => value.trim())
    .filter(Boolean).length
}

function isAICompatibleProvider(provider: string): boolean {
  return ['openai', 'deepseek', 'siliconflow'].includes(provider)
}
