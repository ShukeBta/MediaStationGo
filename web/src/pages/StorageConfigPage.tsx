import { FormEvent, useEffect, useMemo, useState } from 'react'
import { Cloud, Loader2, Save, Send } from 'lucide-react'
import toast from 'react-hot-toast'

import { storageAPI, type StorageType } from '../api/storage_config'

// StorageConfigPage manages the Alist / S3 / WebDAV adapters used by
// the import / playback / STRM subsystems. Mirrors the Vue UI's
// `admin/storage/*` tabs in a tabbed React surface.
export function StorageConfigPage() {
  const [active, setActive] = useState<StorageType>('alist')
  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-blue-400/10 text-blue-300">
          <Cloud size={20} />
        </div>
        <div>
          <h1 className="font-display text-3xl font-bold text-ink-600">外部存储</h1>
          <p className="text-sm text-ink-50">
            配置 Alist / S3 / WebDAV 后端,支持密码加密存储 + 在线测试
          </p>
        </div>
      </div>

      <div className="flex gap-2 border-b border-gray-200">
        {(['alist', 'webdav', 's3'] as StorageType[]).map((t) => (
          <button
            key={t}
            onClick={() => setActive(t)}
            className={
              'border-b-2 px-4 py-2 text-sm uppercase ' +
              (active === t
                ? 'border-primary-400 text-brand-500'
                : 'border-transparent text-ink-50 hover:text-white')
            }
          >
            {t}
          </button>
        ))}
      </div>

      <StorageForm key={active} type={active} />
    </div>
  )
}

const FIELD_DEFS: Record<StorageType, { key: string; label: string; secret?: boolean; placeholder?: string }[]> = {
  alist: [
    { key: 'server', label: 'Server URL', placeholder: 'https://alist.example.com' },
    { key: 'token', label: 'Token', secret: true },
  ],
  webdav: [
    { key: 'url', label: 'URL', placeholder: 'https://example.com/dav/' },
    { key: 'username', label: '用户名' },
    { key: 'password', label: '密码', secret: true },
  ],
  s3: [
    { key: 'endpoint', label: 'Endpoint', placeholder: 'https://s3.amazonaws.com' },
    { key: 'region', label: 'Region', placeholder: 'us-east-1' },
    { key: 'bucket', label: 'Bucket' },
    { key: 'access_key', label: 'Access Key', secret: true },
    { key: 'secret_key', label: 'Secret Key', secret: true },
    { key: 'force_path_style', label: 'force_path_style (true/false)' },
  ],
}

function StorageForm({ type }: { type: StorageType }) {
  const fields = useMemo(() => FIELD_DEFS[type], [type])
  const [config, setConfig] = useState<Record<string, string>>({})
  const [enabled, setEnabled] = useState(true)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)

  const refresh = async () => {
    setLoading(true)
    try {
      const r = await storageAPI.get(type)
      const next: Record<string, string> = {}
      for (const f of fields) {
        const v = r.config?.[f.key]
        // List() redacts secrets to "********"; show empty so the user
        // doesn't accidentally save the placeholder.
        next[f.key] = v === '********' ? '' : v ?? ''
      }
      setConfig(next)
      setEnabled(r.enabled)
    } catch {
      const next: Record<string, string> = {}
      for (const f of fields) next[f.key] = ''
      setConfig(next)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    refresh().catch(() => undefined)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [type])

  const onSave = async (e: FormEvent) => {
    e.preventDefault()
    setSaving(true)
    try {
      await storageAPI.save(type, config, enabled)
      toast.success('已保存')
      await refresh()
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '保存失败'
      toast.error(msg)
    } finally {
      setSaving(false)
    }
  }

  const onTest = async () => {
    setTesting(true)
    try {
      const r = await storageAPI.test(type, config)
      if (r.ok) toast.success('连接成功')
      else toast.error(r.error ?? '连接失败')
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '测试失败'
      toast.error(msg)
    } finally {
      setTesting(false)
    }
  }

  if (loading) {
    return (
      <div className="flex justify-center py-12 text-ink-50">
        <Loader2 className="animate-spin" />
      </div>
    )
  }

  return (
    <form onSubmit={onSave} className="glass-panel space-y-4">
      {fields.map((f) => (
        <label key={f.key} className="block">
          <span className="mb-1 block text-sm text-ink-100">
            {f.label}
            <span className="ml-2 font-mono text-[10px] text-gray-500">{f.key}</span>
          </span>
          <input
            type={f.secret ? 'password' : 'text'}
            className="input-base"
            placeholder={f.placeholder}
            value={config[f.key] ?? ''}
            onChange={(e) => setConfig((c) => ({ ...c, [f.key]: e.target.value }))}
          />
        </label>
      ))}
      <label className="flex items-center gap-2 text-sm text-ink-100">
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
          onClick={onTest}
          disabled={testing}
          className="rounded-lg border border-gray-200 px-4 py-2 text-sm text-ink-100 hover:bg-gray-50"
        >
          {testing ? <Loader2 size={14} className="inline animate-spin" /> : <Send size={14} className="inline" />}
          {' '}测试
        </button>
        <button type="submit" disabled={saving} className="neon-button">
          {saving ? <Loader2 size={16} className="animate-spin" /> : <Save size={16} />}
          保存
        </button>
      </div>
    </form>
  )
}
