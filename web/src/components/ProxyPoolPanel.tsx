import { useState, useEffect } from 'react'
import { Activity, Save } from 'lucide-react'
import toast from 'react-hot-toast'
import { apiConfigsAPI, type ProxyPoolConfig, type ProxyPoolInput, type ProxyPoolItem } from '../api/api_configs'
import { confirmAction } from './confirmAction'
export function ProxyPoolPanel() {
  const [items, setItems] = useState<ProxyPoolItem[]>([])
  const [value, setValue] = useState('')
  const [config, setConfig] = useState<ProxyPoolConfig>({
    proxy_pool_type: 'normal',
    has_resin_proxy_token: false,
  })
  const [resinProxyURL, setResinProxyURL] = useState('')
  const [resinProxyToken, setResinProxyToken] = useState('')
  const [resinAccount, setResinAccount] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [checking, setChecking] = useState(false)
  const [loadError, setLoadError] = useState('')

  const refresh = async () => {
    setLoading(true)
    setLoadError('')
    try {
      const [loaded, loadedConfig] = await Promise.all([
        apiConfigsAPI.listProxyPool(),
        apiConfigsAPI.getProxyPoolConfig(),
      ])
      setItems(loaded)
      setValue(proxyPoolText(loaded))
      setConfig(loadedConfig)
      setResinProxyURL(loadedConfig.resin_proxy_url ?? '')
      setResinAccount(loadedConfig.resin_account ?? '')
      setResinProxyToken('')
    } catch (err: unknown) {
      setLoadError(apiErrorMessage(err, '代理池加载失败'))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void refresh()
  }, [])

  const save = async () => {
    setSaving(true)
    try {
      const patch = {
        proxy_pool_type: config.proxy_pool_type,
        resin_proxy_url: resinProxyURL,
        resin_account: resinAccount,
        ...(resinProxyToken.trim() ? { resin_proxy_token: resinProxyToken.trim() } : {}),
      }
      const savedConfig = await apiConfigsAPI.updateProxyPoolConfig(patch)
      setConfig(savedConfig)
      setResinProxyURL(savedConfig.resin_proxy_url ?? '')
      setResinAccount(savedConfig.resin_account ?? '')
      setResinProxyToken('')
      if (config.proxy_pool_type === 'normal') {
        const retained = new Set<number>()
        const input: ProxyPoolInput[] = value
          .split(/\r?\n/)
          .map((line) => line.trim())
          .filter((line) => line && line !== '!')
          .map((line) => {
            const replace = line.startsWith('!')
            const url = replace ? line.slice(1).trim() : line
            if (replace) return { url }
            const index = items.findIndex((item, itemIndex) => (
              !retained.has(itemIndex) && item.display_url === url
            ))
            if (index < 0) return { url }
            retained.add(index)
            return { id: items[index].id }
          })
        const saved = await apiConfigsAPI.replaceProxyPool(input)
        setItems(saved)
        setValue(proxyPoolText(saved))
      }
      toast.success('代理池已保存')
    } catch (err: unknown) {
      toast.error(apiErrorMessage(err, '代理池保存失败'))
    } finally {
      setSaving(false)
    }
  }

  const checkAndCleanup = async () => {
    setChecking(true)
    try {
      const result = await apiConfigsAPI.checkProxyPool()
      const summary = `共 ${result.total} 个：可用 ${result.available}，不可用 ${result.unavailable}，无法判定 ${result.inconclusive}`
      if (result.unavailable === 0) {
        toast.success(`检测完成，${summary}`)
        return
      }
      const dirtyWarning = value === proxyPoolText(items)
        ? ''
        : ' 当前文本框有未保存修改，清理成功后将以服务端结果覆盖。'
      const confirmed = await confirmAction({
        title: '清理不可用代理',
        message: `检测完成，${summary}。将永久删除 ${result.unavailable} 个确定不可用代理，此操作不可恢复。${dirtyWarning}`,
        confirmText: '确认清理',
      })
      if (!confirmed) return
      if (!result.cleanup_token) throw new Error('missing proxy cleanup token')
      const cleaned = await apiConfigsAPI.cleanupProxyPool(result.cleanup_token)
      setItems(cleaned.items)
      setValue(proxyPoolText(cleaned.items))
      toast.success(`已剔除 ${cleaned.removed} 个不可用代理`)
    } catch (err: unknown) {
      toast.error(apiErrorMessage(err, '代理池检测失败'))
    } finally {
      setChecking(false)
    }
  }

  return (
    <div className="glass-panel p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <p className="font-medium text-ink-600">代理池</p>
          <p className="text-xs text-sand-500">
            {config.proxy_pool_type === 'normal'
              ? '每行一个代理；使用首个节点，失败后恢复直连。空行会被忽略。'
              : 'Resin 通过 URL 反向代理统一调度内部节点。'}
          </p>
        </div>
        <div className="flex items-center gap-2">
          {config.proxy_pool_type === 'normal' && (
            <button
              type="button"
              onClick={() => void checkAndCleanup()}
              disabled={loading || saving || checking || Boolean(loadError) || items.length === 0}
              className="btn-outline !px-3 !py-1.5 !text-xs"
            >
              <Activity size={12} /> {checking ? '检测中…' : '检测不可用代理'}
            </button>
          )}
          <button
            type="button"
            onClick={() => void save()}
            disabled={loading || saving || checking || Boolean(loadError) || (
              config.proxy_pool_type === 'resin'
              && (!resinProxyURL.trim() || (!config.has_resin_proxy_token && !resinProxyToken.trim()))
            )}
            className="neon-button !px-3 !py-1.5 !text-xs"
          >
            <Save size={12} /> 保存
          </button>
        </div>
      </div>

      {loading && <p className="py-5 text-center text-sm text-sand-500">加载中…</p>}
      {!loading && loadError && (
        <div className="mt-3 flex items-center justify-between gap-3 text-sm text-red-700">
          <span className="break-words">{loadError}</span>
          <button type="button" onClick={() => void refresh()} className="shrink-0 font-medium hover:text-red-900">
            重试
          </button>
        </div>
      )}
      {!loading && !loadError && (
        <div className="mt-3 space-y-3">
          <label className="block text-xs text-ink-50">
            代理类型
            <select
              className="input-base mt-1 w-full"
              value={config.proxy_pool_type}
              onChange={(event) => setConfig({ ...config, proxy_pool_type: event.target.value as 'normal' | 'resin' })}
            >
              <option value="normal">普通代理池</option>
              <option value="resin">Resin 代理池</option>
            </select>
          </label>
          {config.proxy_pool_type === 'normal' ? (
            <label className="block text-xs text-ink-50">
              代理地址
              <textarea
                className="input-base mt-1 min-h-40 resize-y font-mono text-xs"
                placeholder={'http://user:pass@proxy.example:8080\nsocks5://proxy.example:1080'}
                value={value}
                onChange={(event) => setValue(event.target.value)}
                disabled={checking}
                autoComplete="off"
                spellCheck={false}
              />
              <span className="mt-1 block text-sand-500">
                已保存 {items.length} 个节点，其中 {items.filter((item) => item.has_auth).length} 个含认证；未改动的脱敏行会保留原认证。
                需要强制替换或移除认证时，在该行开头加 !。
              </span>
            </label>
          ) : (
            <div className="grid gap-3 md:grid-cols-2">
              <label className="text-xs text-ink-50">
                Resin 实例地址
                <input
                  className="input-base mt-1"
                  type="url"
                  placeholder="https://resin.example.com"
                  value={resinProxyURL}
                  onChange={(event) => setResinProxyURL(event.target.value)}
                />
              </label>
              <label className="text-xs text-ink-50">
                RESIN_PROXY_TOKEN
                <input
                  className="input-base mt-1"
                  type="password"
                  placeholder={config.has_resin_proxy_token ? '•••••••••••• (留空保留原值)' : '输入代理 Token'}
                  value={resinProxyToken}
                  onChange={(event) => setResinProxyToken(event.target.value)}
                />
              </label>
              <label className="text-xs text-ink-50 md:col-span-2">
                粘性标识（可选）
                <input
                  className="input-base mt-1"
                  maxLength={128}
                  placeholder="留空时由 Resin 随机调度"
                  value={resinAccount}
                  onChange={(event) => setResinAccount(event.target.value)}
                />
              </label>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function proxyPoolText(items: ProxyPoolItem[]): string {
  return items.map((item) => item.display_url).join('\n')
}

function apiErrorMessage(err: unknown, fallback: string): string {
  return (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? fallback
}
