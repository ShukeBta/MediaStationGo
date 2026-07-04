import { useEffect, useMemo, useState } from 'react'
import toast from 'react-hot-toast'

import { toolsAPI } from '../api/tools'
import { confirmAction } from '../components/confirmAction'
import type { Library } from '../types'
import {
  formatScanSummary,
  formatScrapeSummary,
  summarizeOrganizeResults,
  type OrganizePreviewItem,
} from './fileManagerModel'

type UseManualOrganizeOptions = {
  currentDir: string
  localLibraries: Library[]
  selectedPath?: string
  selectedPaths: string[]
  scrapeAfter: boolean
  keepSeeding: boolean
  onScrapeAfterChange: (value: boolean) => void
  onClearSelected: () => void
  refresh: () => void
}

export function useManualOrganize({
  currentDir,
  localLibraries,
  selectedPath,
  selectedPaths,
  scrapeAfter,
  keepSeeding,
  onScrapeAfterChange,
  onClearSelected,
  refresh,
}: UseManualOrganizeOptions) {
  const [organizeLibraryID, setOrganizeLibraryID] = useState('')
  const [organizeDestPath, setOrganizeDestPath] = useState('')
  const [organizeTransferMode, setOrganizeTransferMode] = useState('hardlink')
  const [organizeMediaType, setOrganizeMediaType] = useState('auto')
  const [scanAfter, setScanAfter] = useState(true)
  const [organizeBusy, setOrganizeBusy] = useState('')
  const [previewItems, setPreviewItems] = useState<OrganizePreviewItem[]>([])

  useEffect(() => {
    if (!organizeLibraryID) return
    const lib = localLibraries.find((item) => item.id === organizeLibraryID)
    if (!lib) {
      setOrganizeLibraryID('')
      return
    }
    setOrganizeDestPath(lib.path)
    setOrganizeMediaType(lib.type || 'auto')
  }, [localLibraries, organizeLibraryID])

  const organizeSources = useMemo(
    () => selectedPaths.length > 0 ? selectedPaths : [selectedPath || currentDir].filter(Boolean),
    [currentDir, selectedPath, selectedPaths],
  )
  const organizeSource = organizeSources.length === 1 ? organizeSources[0] : `${organizeSources.length} 个已选项目`
  const organizeReady = organizeSources.length > 0 && Boolean(organizeDestPath.trim())
  const manualMoveKeepsSeeding = organizeTransferMode === 'move' && keepSeeding

  const runManualOrganize = async (dryRun: boolean) => {
    if (!organizeReady) {
      toast.error('请选择来源文件/文件夹，并设置目标媒体库或目的路径')
      return
    }
    if (!dryRun) {
      const ok = await confirmAction({
        title: '确认整理入库',
        message: `来源：${organizeSources.join('\n')}\n目标：${organizeDestPath}\n方式：${organizeTransferMode}${scanAfter ? '\n整理完成后会扫描入库。' : ''}${scanAfter && scrapeAfter ? '\n扫描后会自动刮削。' : ''}`,
        confirmText: '开始整理',
      })
      if (!ok) return
    }
    setOrganizeBusy(dryRun ? 'preview' : 'run')
    try {
      const results = []
      for (const sourcePath of organizeSources) {
        results.push(await toolsAPI.organizeDirectory({
          source_path: sourcePath,
          dest_path: organizeDestPath.trim(),
          transfer_mode: organizeTransferMode,
          media_type: organizeMediaType === 'auto' ? undefined : organizeMediaType,
          scan_after: !dryRun && scanAfter,
          scrape_after: !dryRun && scanAfter && scrapeAfter,
          library_id: !dryRun && scanAfter && organizeLibraryID ? organizeLibraryID : undefined,
          dry_run: dryRun,
        }))
      }
      const summary = summarizeOrganizeResults(results)
      setPreviewItems(summary.preview)
      if (summary.total === 0) {
        toast(`未发现可整理视频：${organizeSource}`, {
          icon: '!',
          duration: 6000,
        })
        return
      }
      if (dryRun) {
        toast.success(`预览完成：新增 ${summary.organized} · 替换 ${summary.replaced} · 纠偏 ${summary.reclassified} · 跳过 ${summary.skipped}`)
        return
      }
      const scanText = scanAfter ? formatScanSummary(summary.scans) : ''
      const scrapeText = scanAfter && scrapeAfter ? formatScrapeSummary(summary.scrapes) : ''
      toast.success(`整理完成：新增 ${summary.organized} · 替换 ${summary.replaced} · 纠偏 ${summary.reclassified} · 跳过 ${summary.skipped}${scanText}${scrapeText}`)
      onClearSelected()
      refresh()
    } catch (err: unknown) {
      toast.error((err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '整理失败')
    } finally {
      setOrganizeBusy('')
    }
  }

  return {
    organizeLibraryID,
    organizeDestPath,
    organizeTransferMode,
    organizeMediaType,
    scanAfter,
    organizeBusy,
    previewItems,
    organizeSource,
    organizeReady,
    manualMoveKeepsSeeding,
    setOrganizeLibraryID,
    setOrganizeDestPath,
    setOrganizeTransferMode,
    setOrganizeMediaType,
    setScanAfter,
    onScrapeAfterChange,
    preview: () => runManualOrganize(true),
    run: () => runManualOrganize(false),
  }
}
