import { FormEvent, useEffect, useState } from 'react'
import { Loader2, Save, SettingsIcon } from 'lucide-react'
import toast from 'react-hot-toast'

import { adminAPI } from '../api/admin'
import { libraryAPI } from '../api/library'
import type { Library, Setting } from '../types'
import { SettingRow } from './SettingsRow'
import { ALL_KEYS, GROUPS } from './settingsGroups'
import { SystemUpdatePanel } from './SystemUpdatePanel'
import { RecognitionWordsPanel } from './RecognitionWordsPanel'

export function SettingsPage() {
  const [activeGroup, setActiveGroup] = useState(GROUPS[0].key)
  const [values, setValues] = useState<Record<string, string>>({})
  const [dirty, setDirty] = useState<Set<string>>(new Set())
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [libraries, setLibraries] = useState<Library[]>([])

  const refresh = async () => {
    setLoading(true)
    try {
      const [all, libs] = await Promise.all([
        adminAPI.listSettings(),
        libraryAPI.list({ includeHidden: true }).catch(() => [] as Library[]),
      ])
      const idx: Record<string, string> = {}
      for (const s of all as Setting[]) {
        if (ALL_KEYS.has(s.key)) idx[s.key] = s.value
      }
      setValues(idx)
      setLibraries(libs as Library[])
      setDirty(new Set())
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    refresh().catch(() => undefined)
  }, [])

  const onChange = (key: string, value: string) => {
    setValues((v) => ({ ...v, [key]: value }))
    setDirty((d) => new Set(d).add(key))
  }

  const onSave = async (e: FormEvent) => {
    e.preventDefault()
    if (dirty.size === 0) return
    setSaving(true)
    try {
      // Backend exposes a single-key updater; loop through dirty keys.
      for (const key of dirty) {
        await adminAPI.updateSetting(key, values[key] ?? '')
      }
      toast.success(`已保存 ${dirty.size} 项配置`)
      setDirty(new Set())
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '保存失败'
      toast.error(msg)
    } finally {
      setSaving(false)
    }
  }

  const group = GROUPS.find((g) => g.key === activeGroup)!

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-sand-300/40 text-ink-100">
          <SettingsIcon size={20} />
        </div>
        <div>
          <h1 className="font-display text-3xl font-bold text-ink-600">系统设置</h1>
          <p className="text-sm text-ink-50">
            按分组编辑转码 / 网盘转存 / Adult 等关键配置
          </p>
        </div>
      </div>

      <div className="flex gap-2 overflow-x-auto border-b border-gray-200">
        {GROUPS.map((g) => (
          <button
            key={g.key}
            onClick={() => setActiveGroup(g.key)}
            className={
              'border-b-2 px-4 py-2 text-sm whitespace-nowrap transition ' +
              (activeGroup === g.key
                ? 'border-primary-400 text-brand-500'
                : 'border-transparent text-ink-50 hover:text-white')
            }
          >
            {g.label}
          </button>
        ))}
      </div>

      {loading && (
        <div className="flex justify-center py-12 text-ink-50">
          <Loader2 className="animate-spin" />
        </div>
      )}

      {!loading && (
        <div className="space-y-4">
          {group.key === 'system-update' && <SystemUpdatePanel />}
          {group.key === 'recognition-words' && <RecognitionWordsPanel />}
          {group.items.length > 0 && (
            <form onSubmit={onSave} className="glass-panel space-y-4">
              {group.description && <p className="text-xs text-sand-500">{group.description}</p>}
              {group.items.map((it) => (
                <SettingRow
                  key={it.key}
                  def={it}
                  value={values[it.key] ?? it.defaultValue ?? ''}
                  onChange={(v) => onChange(it.key, v)}
                  libraries={libraries}
                />
              ))}
              <div className="flex items-center justify-between pt-2">
                <span className="text-xs text-sand-500">
                  {dirty.size > 0 ? `有 ${dirty.size} 项未保存` : '所有更改已保存'}
                </span>
                <button
                  type="submit"
                  disabled={saving || dirty.size === 0}
                  className="neon-button disabled:opacity-50"
                >
                  {saving ? <Loader2 size={16} className="animate-spin" /> : <Save size={16} />}
                  保存
                </button>
              </div>
            </form>
          )}
        </div>
      )}
    </div>
  )
}
