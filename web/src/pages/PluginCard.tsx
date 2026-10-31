import { useState } from 'react'
import { Play, Power, Save } from 'lucide-react'

import type { PluginInfo, PluginRunResult, PluginUpdate } from '../api/plugins'

const statusLabels: Record<PluginInfo['status'], string> = {
  disabled: '已停用', ready: '可运行', running: '运行中', error: '运行异常',
}

function RunResult({ result }: { result: PluginRunResult }) {
  return (
    <section className="space-y-3 rounded-xl bg-gray-50 p-4" aria-label="上次运行结果">
      <h3 className="font-medium text-ink-600">上次运行结果</h3>
      <p className="text-sm text-ink-100">{result.summary}</p>
      <dl className="grid grid-cols-2 gap-3 sm:grid-cols-3">
        {result.metrics.map((metric, index) => (
          <div key={`${metric.label}-${index}`}>
            <dt className="text-xs text-ink-100">{metric.label}</dt>
            <dd className="mt-1 text-xl font-semibold text-ink-600">{metric.value.toLocaleString('zh-CN')}</dd>
          </div>
        ))}
      </dl>
      <p className="text-xs text-ink-100">
        完成于 {new Date(result.completed_at).toLocaleString('zh-CN')} · 耗时 {result.duration_ms.toLocaleString('zh-CN')} 毫秒
      </p>
    </section>
  )
}

export function PluginCard({ plugin, busy, pending, onUpdate, onRun }: {
  plugin: PluginInfo
  busy: boolean
  pending: string
  onUpdate: (id: string, input: PluginUpdate) => Promise<boolean>
  onRun: (id: string) => Promise<void>
}) {
  // Only edited keys are retained, so refreshes preserve drafts while other fields stay current.
  const [draft, setDraft] = useState<Record<string, unknown>>({})
  const config = { ...plugin.config, ...draft }
  const dirty = plugin.config_fields.some((field) =>
    (config[field.key] ?? field.default) !== (plugin.config[field.key] ?? field.default))
  const disabled = busy || plugin.status === 'running'
  const save = async () => {
    if (await onUpdate(plugin.id, { config })) setDraft({})
  }

  return (
    <article className="card space-y-5 p-5 sm:p-6" aria-labelledby={`plugin-${plugin.id}`} aria-busy={busy}>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <h2 id={`plugin-${plugin.id}`} className="text-xl font-semibold text-ink-600">{plugin.name}</h2>
            <span className="rounded-full bg-primary-400/10 px-2 py-1 text-xs text-brand-500">内置插件</span>
            <span className="rounded-full bg-gray-100 px-2 py-1 text-xs text-ink-100">{statusLabels[plugin.status]}</span>
          </div>
          <p className="text-sm text-ink-100">{plugin.description}</p>
          <p className="text-xs text-ink-100">版本 {plugin.version} · {plugin.author}</p>
        </div>
        <button type="button" className="btn-ghost" disabled={disabled}
          onClick={() => void onUpdate(plugin.id, { enabled: !plugin.enabled })}>
          <Power size={16} />{pending === `${plugin.id}:toggle` ? '处理中…' : plugin.enabled ? '停用插件' : '启用插件'}
        </button>
      </div>
      {plugin.config_fields.length > 0 && (
        <fieldset disabled={disabled} className="space-y-3">
          <legend className="mb-3 font-medium text-ink-600">插件配置</legend>
          {plugin.config_fields.map((field) => (
            <label key={field.key} className="flex cursor-pointer items-start gap-3 text-sm text-ink-600">
              <input type="checkbox" className="mt-1 h-4 w-4 accent-[var(--app-brand-text)]"
                checked={Boolean(config[field.key] ?? field.default)}
                onChange={(event) => setDraft((previous) => ({ ...previous, [field.key]: event.target.checked }))} />
              <span>{field.label}<span className="mt-1 block text-xs text-ink-100">{field.description}</span></span>
            </label>
          ))}
          <div className="flex flex-wrap items-center gap-3">
            <button type="button" className="btn-ghost" disabled={disabled || !dirty} onClick={() => void save()}>
              <Save size={16} />{pending === `${plugin.id}:save` ? '保存中…' : '保存配置'}
            </button>
            {dirty && <span className="text-xs text-ink-100">有未保存的修改，请保存后运行。</span>}
          </div>
        </fieldset>
      )}
      <button type="button" className="btn-primary" disabled={disabled || !plugin.enabled || dirty}
        onClick={() => void onRun(plugin.id)}>
        <Play size={16} />{pending === `${plugin.id}:run` ? '运行中…' : '立即运行'}
      </button>
      {!plugin.enabled && <p className="text-sm text-ink-100">启用插件后即可手动运行。</p>}
      {plugin.last_error && <p role="alert" className="text-sm text-red-600">上次运行错误：{plugin.last_error}</p>}
      {plugin.last_run ? <RunResult result={plugin.last_run} /> : <p className="text-sm text-ink-100">暂无运行记录。</p>}
    </article>
  )
}
