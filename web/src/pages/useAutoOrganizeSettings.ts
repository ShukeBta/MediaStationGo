import { useCallback, useEffect, useMemo, useState } from 'react'
import toast from 'react-hot-toast'

import { adminAPI } from '../api/admin'
import { schedulerAPI } from '../api/scheduler'
import {
  AUTO_ORGANIZE_DEFAULTS,
  AUTO_ORGANIZE_KEYS,
  type AutoOrganizeConfig,
  type AutoOrganizeTab,
  mergeAutoOrganizeSettings,
  settingOn,
} from './autoOrganizeModel'

type UseAutoOrganizeSettingsOptions = {
  onScrapeAfterChange: (enabled: boolean) => void
}

export function useAutoOrganizeSettings({ onScrapeAfterChange }: UseAutoOrganizeSettingsOptions) {
  const [config, setConfig] = useState<AutoOrganizeConfig>(AUTO_ORGANIZE_DEFAULTS)
  const [dirty, setDirty] = useState(false)
  const [saving, setSaving] = useState(false)
  const [running, setRunning] = useState(false)
  const [loading, setLoading] = useState(true)
  const [activeTab, setActiveTab] = useState<AutoOrganizeTab>('basic')

  const refresh = useCallback(() => {
    setLoading(true)
    adminAPI
      .listSettings()
      .then((rows) => {
        const nextConfig = mergeAutoOrganizeSettings(rows)
        setConfig(nextConfig)
        onScrapeAfterChange(settingOn(nextConfig.scrapeAfter))
        setDirty(false)
      })
      .catch(() => undefined)
      .finally(() => setLoading(false))
  }, [onScrapeAfterChange])

  useEffect(() => {
    refresh()
  }, [refresh])

  const changeConfig = useCallback((key: keyof AutoOrganizeConfig, value: string) => {
    setConfig((current) => ({ ...current, [key]: value }))
    if (key === 'scrapeAfter') onScrapeAfterChange(settingOn(value))
    setDirty(true)
  }, [onScrapeAfterChange])

  const save = useCallback(async (): Promise<boolean> => {
    setSaving(true)
    try {
      for (const key of Object.keys(AUTO_ORGANIZE_KEYS) as Array<keyof AutoOrganizeConfig>) {
        await adminAPI.updateSetting(AUTO_ORGANIZE_KEYS[key], config[key] ?? '')
      }
      setDirty(false)
      toast.success('整理入库设置已保存')
      return true
    } catch (err: unknown) {
      toast.error((err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '保存整理入库设置失败')
      return false
    } finally {
      setSaving(false)
    }
  }, [config])

  const runNow = useCallback(async () => {
    if (dirty) {
      const saved = await save()
      if (!saved) return
    }
    setRunning(true)
    try {
      await schedulerAPI.run('organize_source')
      toast.success('已触发自动整理任务，请稍后刷新媒体库查看入库结果')
    } catch (err: unknown) {
      toast.error((err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '触发自动整理失败')
    } finally {
      setRunning(false)
    }
  }, [dirty, save])

  const moveKeepsSeeding = useMemo(
    () => config.transferMode === 'move' && settingOn(config.keepSeeding),
    [config.keepSeeding, config.transferMode],
  )

  return {
    config,
    dirty,
    saving,
    running,
    loading,
    activeTab,
    moveKeepsSeeding,
    refresh,
    save,
    runNow,
    setActiveTab,
    changeConfig,
  }
}
