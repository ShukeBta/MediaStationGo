import { useEffect, useState } from 'react'
import toast from 'react-hot-toast'

import { recognitionWordsAPI, type RecognitionWordsConfig, type RecognitionWordsTestResult } from '../api/recognitionWords'

const DEFAULT_CONFIG: RecognitionWordsConfig = {
  enabled: true,
  local_text: '',
  shared_urls: [
    'https://raw.githubusercontent.com/Putarku/MoviePilot-Help/main/Words/general.txt',
    'https://raw.githubusercontent.com/Putarku/MoviePilot-Help/main/Words/TV.txt',
    'https://raw.githubusercontent.com/Putarku/MoviePilot-Help/main/Words/anime.txt',
  ],
  rule_count: 0,
}

export function useRecognitionWordsPanel() {
  const [config, setConfig] = useState<RecognitionWordsConfig>(DEFAULT_CONFIG)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [syncing, setSyncing] = useState(false)
  const [testing, setTesting] = useState(false)
  const [testInput, setTestInput] = useState('')
  const [testResult, setTestResult] = useState<RecognitionWordsTestResult | null>(null)

  const refresh = async () => {
    setLoading(true)
    try {
      setConfig(await recognitionWordsAPI.get())
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    refresh().catch(() => undefined)
  }, [])

  const updateEnabled = (enabled: boolean) => {
    setConfig((prev) => ({ ...prev, enabled }))
  }

  const updateSharedURLs = (value: string) => {
    setConfig((prev) => ({
      ...prev,
      shared_urls: value.split(/\r?\n/).map((item) => item.trim()).filter(Boolean),
    }))
  }

  const updateLocalText = (localText: string) => {
    setConfig((prev) => ({ ...prev, local_text: localText }))
  }

  const save = async () => {
    setSaving(true)
    try {
      const saved = await recognitionWordsAPI.save(config)
      setConfig(saved)
      toast.success('识别词配置已保存')
    } catch (error: unknown) {
      toast.error(apiErrorMessage(error, '保存失败'))
    } finally {
      setSaving(false)
    }
  }

  const sync = async () => {
    setSyncing(true)
    try {
      const synced = await recognitionWordsAPI.sync()
      setConfig(synced)
      toast.success(`已同步 ${synced.rule_count} 条识别词`)
    } catch (error: unknown) {
      toast.error(apiErrorMessage(error, '同步失败'))
    } finally {
      setSyncing(false)
    }
  }

  const test = async () => {
    if (!testInput.trim()) return
    setTesting(true)
    try {
      setTestResult(await recognitionWordsAPI.test(testInput))
    } catch (error: unknown) {
      toast.error(apiErrorMessage(error, '测试失败'))
    } finally {
      setTesting(false)
    }
  }

  return {
    config,
    loading,
    saving,
    syncing,
    testing,
    testInput,
    testResult,
    save,
    setTestInput,
    sync,
    test,
    updateEnabled,
    updateLocalText,
    updateSharedURLs,
  }
}

function apiErrorMessage(error: unknown, fallback: string): string {
  return (error as { response?: { data?: { error?: string } } })?.response?.data?.error ?? fallback
}
