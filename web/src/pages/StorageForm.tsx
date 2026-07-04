import { FormEvent, useEffect, useMemo, useState } from 'react'
import { Loader2 } from 'lucide-react'
import toast from 'react-hot-toast'

import {
  storageAPI,
  type StorageType,
} from '../api/storage_config'
import { confirmAction } from '../components/confirmAction'
import { CloudBrowser } from './CloudBrowser'
import { QRLoginPanel } from './QRLoginPanel'
import { StorageFormActions } from './StorageFormActions'
import { StorageTransferSettings } from './StorageTransferSettings'
import { StorageUploadPanel } from './StorageUploadPanel'
import {
  FIELD_DEFS,
  TRANSFER_SUPPORTED_TYPES,
  TYPE_LABEL,
  isCloud,
} from './storageConfigModel'

export function StorageForm({ type }: { type: StorageType }) {
  const fields = useMemo(() => FIELD_DEFS[type], [type])
  const transferSupported = TRANSFER_SUPPORTED_TYPES.has(type)
  const [config, setConfig] = useState<Record<string, string>>({})
  const [enabled, setEnabled] = useState(true)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [loggingOut, setLoggingOut] = useState(false)
  const [browserVersion, setBrowserVersion] = useState(0)

  const refresh = async () => {
    setLoading(true)
    try {
      const r = await storageAPI.get(type)
      const next: Record<string, string> = {}
      for (const [key, value] of Object.entries(r.config ?? {})) {
        next[key] = value === '********' ? '' : String(value ?? '')
      }
      for (const f of fields) {
        const v = r.config?.[f.key]
        // List() redacts secrets to "********"; show empty so the user
        // doesn't accidentally save the placeholder.
        next[f.key] = v === '********' ? '' : v ?? ''
      }
      setConfig(next)
      setEnabled(r.enabled ?? true)
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
      setBrowserVersion((version) => version + 1)
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
      if (!r.ok) {
        toast.error(r.error ?? '连接失败')
        return
      }
      if (isCloud(type)) {
        await storageAPI.save(type, config, true)
        setEnabled(true)
        toast.success('连接成功，已保存并启用')
        await refresh()
        setBrowserVersion((version) => version + 1)
        return
      }
      toast.success('连接成功')
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '测试失败'
      toast.error(msg)
    } finally {
      setTesting(false)
    }
  }

  const onLogout = async () => {
    const ok = await confirmAction({
      title: '退出云盘登录',
      message: `将清空「${TYPE_LABEL[type] ?? type}」保存的 Cookie / Token / 密码并停用该外部存储，同时移除本项目中的对应网盘挂载和媒体记录；不会删除网盘里的真实文件。`,
      confirmText: '退出登录',
    })
    if (!ok) return
    setLoggingOut(true)
    try {
      await storageAPI.logout(type)
      toast.success('已退出云盘登录、停用该存储并清理本项目挂载')
      await refresh()
      setBrowserVersion((version) => version + 1)
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '退出登录失败'
      toast.error(msg)
    } finally {
      setLoggingOut(false)
    }
  }

  if (loading) {
    return (
      <div className="flex justify-center py-12 text-ink-50">
        <Loader2 className="animate-spin" />
      </div>
    )
  }

  const transferEnabled = (config.transfer_enabled ?? '').toLowerCase() === 'true'
  const transferMode = config.transfer_mode === 'move' ? 'move' : 'copy'

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
      {type === 'cloud115' && (
        <QRLoginPanel
          type={type}
          onCookie={(c) => setConfig((cfg) => ({ ...cfg, cookie: c }))}
        />
      )}
      <label className="flex items-center gap-2 text-sm text-ink-100">
        <input
          type="checkbox"
          className="h-4 w-4 accent-primary-400"
          checked={enabled}
          onChange={(e) => setEnabled(e.target.checked)}
        />
        启用
      </label>
      {transferSupported && (
        <StorageTransferSettings
          transferEnabled={transferEnabled}
          transferMode={transferMode}
          onTransferEnabledChange={(checked) => {
            setConfig((current) => ({ ...current, transfer_enabled: checked ? 'true' : 'false' }))
          }}
          onTransferModeChange={(mode) => {
            setConfig((current) => ({ ...current, transfer_mode: mode }))
          }}
        />
      )}
      <StorageFormActions
        showLogout={isCloud(type)}
        loggingOut={loggingOut}
        testing={testing}
        saving={saving}
        onLogout={onLogout}
        onTest={onTest}
      />
      <StorageUploadPanel
        type={type}
        transferEnabled={transferEnabled}
        transferMode={transferMode}
      />
      {isCloud(type) && <CloudBrowser key={`${type}-${browserVersion}-${enabled ? 'on' : 'off'}`} type={type} enabled={enabled} />}
    </form>
  )
}
