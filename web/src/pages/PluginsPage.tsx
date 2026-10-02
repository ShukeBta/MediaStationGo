import { useCallback, useEffect, useRef, useState } from 'react'
import { Loader2, Puzzle, RefreshCw } from 'lucide-react'
import toast from 'react-hot-toast'

import { pluginsAPI, type PluginInfo, type PluginUpdate } from '../api/plugins'
import { PluginCard } from './PluginCard'

function errorMessage(error: unknown): string {
  const message = (error as { response?: { data?: { error?: unknown } } })?.response?.data?.error
  return typeof message === 'string' ? message : '请求失败，请检查网络后重试。'
}

export function PluginsPage() {
  const [plugins, setPlugins] = useState<PluginInfo[]>([])
  const [loaded, setLoaded] = useState(false)
  const [pending, setPending] = useState('refresh')
  const [error, setError] = useState('')
  const lock = useRef(false)

  const perform = useCallback(async (key: string, operation: () => Promise<void>) => {
    if (lock.current) return false
    lock.current = true
    setPending(key)
    setError('')
    try {
      await operation()
      return true
    } catch (err) {
      setError(errorMessage(err))
      return false
    } finally {
      lock.current = false
      setPending('')
    }
  }, [])

  const refresh = useCallback(() => perform('refresh', async () => {
    setPlugins(await pluginsAPI.list())
    setLoaded(true)
  }), [perform])

  useEffect(() => { void refresh() }, [refresh])

  const update = (id: string, input: PluginUpdate) => perform(`${id}:${input.config ? 'save' : 'toggle'}`, async () => {
    const updated = await pluginsAPI.update(id, input)
    setPlugins((current) => current.map((plugin) => plugin.id === id ? updated : plugin))
    toast.success(input.config ? '插件配置已保存' : updated.enabled ? '插件已启用' : '插件已停用')
  })

  const run = async (id: string) => {
    await perform(`${id}:run`, async () => {
      const result = await pluginsAPI.run(id)
      setPlugins((current) => current.map((plugin) => plugin.id === id
        ? { ...plugin, last_run: result, last_error: '', status: 'ready' } : plugin))
      toast.success('插件运行完成')
    })
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="space-y-2">
          <h1 className="flex items-center gap-3 font-display text-3xl font-bold text-ink-600"><Puzzle />插件管理</h1>
          <p className="max-w-2xl text-sm text-ink-100">管理内置扩展的启停、配置和运行记录。当前仅支持随系统提供的内置插件，运行记录保留至服务重启。</p>
        </div>
        <button type="button" className="btn-ghost" disabled={Boolean(pending)} onClick={() => void refresh()}>
          <RefreshCw size={16} className={pending === 'refresh' ? 'animate-spin' : ''} />刷新
        </button>
      </div>
      {error && (
        <div role="alert" className="card flex flex-wrap items-center justify-between gap-3 p-4">
          <p className="text-sm text-red-600">{error}</p>
          <button type="button" className="btn-ghost" disabled={Boolean(pending)} onClick={() => void refresh()}>重新加载</button>
        </div>
      )}
      {!loaded && pending && <p role="status" className="flex items-center gap-2 text-ink-100"><Loader2 className="animate-spin" size={18} />正在加载插件…</p>}
      {loaded && plugins.length === 0 && <p className="card p-6 text-ink-100">当前没有可用的内置插件。</p>}
      {plugins.map((plugin) => <PluginCard key={plugin.id} plugin={plugin} busy={Boolean(pending)}
        pending={pending} onUpdate={update} onRun={run} />)}
    </div>
  )
}
