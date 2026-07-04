import type { FormEvent } from 'react'
import { useState } from 'react'
import toast from 'react-hot-toast'

import { strmAPI, type RepairSTRMResult } from '../api/strm'
import { currentOrigin } from './strmPageModel'
import { apiErrorMessage, isHTTPURL } from './strmPageUtils'

export function useStrmRepairForm() {
  const [baseURL, setBaseURL] = useState(currentOrigin())
  const [outputDir, setOutputDir] = useState('data/strm/tree')
  const [refreshLibrary, setRefreshLibrary] = useState(true)
  const [scrapeAfter, setScrapeAfter] = useState(false)
  const [runningMode, setRunningMode] = useState<'repair' | 'preview' | null>(null)
  const [result, setResult] = useState<RepairSTRMResult | null>(null)

  const runRepair = async (dryRun: boolean) => {
    const trimmedBaseURL = baseURL.trim()
    if (!outputDir.trim()) {
      toast.error('请填写 STRM 输出目录')
      return
    }
    if (trimmedBaseURL && !isHTTPURL(trimmedBaseURL)) {
      toast.error('域名必须以 http:// 或 https:// 开头')
      return
    }
    setRunningMode(dryRun ? 'preview' : 'repair')
    try {
      const next = await strmAPI.repair({
        output_dir: outputDir.trim(),
        base_url: trimmedBaseURL.replace(/\/+$/, '') || undefined,
        dry_run: dryRun,
        refresh_library: refreshLibrary,
        scrape_after: !dryRun && refreshLibrary && scrapeAfter,
      })
      setResult(next)
      if (dryRun) {
        toast.success(`预检完成：可修复 ${next.previewed ?? 0} 个 · 跳过 ${next.skipped}`)
      } else {
        toast.success(`修复完成：修复 ${next.repaired} 个 · 跳过 ${next.skipped}`)
      }
    } catch (err: unknown) {
      toast.error(apiErrorMessage(err, dryRun ? 'STRM 修复预检失败' : 'STRM 修复失败'))
    } finally {
      setRunningMode(null)
    }
  }

  const onRepair = async (event: FormEvent) => {
    event.preventDefault()
    await runRepair(false)
  }

  const onPreview = async () => {
    await runRepair(true)
  }

  const setRefreshLibraryEnabled = (value: boolean) => {
    setRefreshLibrary(value)
    if (!value) setScrapeAfter(false)
  }

  return {
    baseURL,
    outputDir,
    repairing: runningMode !== null,
    refreshLibrary,
    scrapeAfter,
    result,
    runningMode,
    onPreview,
    onRepair,
    setBaseURL,
    setOutputDir,
    setRefreshLibrary: setRefreshLibraryEnabled,
    setScrapeAfter,
  }
}
