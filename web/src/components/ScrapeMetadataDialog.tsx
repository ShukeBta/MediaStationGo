import { useEffect, useState } from 'react'
import { Folder, FolderInput, Sparkles, X, CheckCircle2, AlertCircle, RefreshCw } from 'lucide-react'
import toast from 'react-hot-toast'

import { libraryAPI } from '../api/library'
import { toolsAPI, type OrganizeResultSummary } from '../api/tools'
import type { Library } from '../types'
import { EpisodeArtworkToggle } from './EpisodeArtworkToggle'

interface ScrapeMetadataDialogProps {
  open: boolean
  library: Library | null
  scrapeEpisodeArtwork: boolean
  onScrapeEpisodeArtworkChange: (checked: boolean) => void
  onClose: () => void
  onCompleted?: () => void | Promise<void>
}

type ScrapeTargetMode = 'current' | 'custom'
type ScrapeActionType = 'organize_and_scrape' | 'scrape_only'

export function ScrapeMetadataDialog({
  open,
  library,
  scrapeEpisodeArtwork,
  onScrapeEpisodeArtworkChange,
  onClose,
  onCompleted,
}: ScrapeMetadataDialogProps) {
  const [targetMode, setTargetMode] = useState<ScrapeTargetMode>('current')
  const [actionType, setActionType] = useState<ScrapeActionType>('organize_and_scrape')
  const [destPath, setDestPath] = useState('')
  const [transferMode, setTransferMode] = useState('hardlink')
  const [refreshMatched, setRefreshMatched] = useState(true)

  const [availableLibraries, setAvailableLibraries] = useState<Library[]>([])
  const [busy, setBusy] = useState(false)
  const [previewBusy, setPreviewBusy] = useState(false)
  const [previewResult, setPreviewResult] = useState<OrganizeResultSummary | null>(null)

  useEffect(() => {
    if (!open) return
    setTargetMode('current')
    setActionType('organize_and_scrape')
    setDestPath('')
    setTransferMode('hardlink')
    setRefreshMatched(true)
    setPreviewResult(null)

    // Load available libraries for quick destination selection
    libraryAPI
      .list({ includeHidden: true })
      .then((libs) => setAvailableLibraries(libs.filter((l) => l.id !== library?.id)))
      .catch(() => undefined)
  }, [open, library])

  if (!open || !library) return null

  const isCloud = (library.path || '').toLowerCase().startsWith('cloud://')

  const effectiveDestPath = targetMode === 'custom' ? destPath.trim() : library.path
  const isOrganize = targetMode === 'custom' || actionType === 'organize_and_scrape'

  const handlePreview = async () => {
    if (previewBusy || busy) return
    if (targetMode === 'custom' && !destPath.trim()) {
      toast.error('请输入指定目标文件夹路径')
      return
    }

    setPreviewBusy(true)
    setPreviewResult(null)
    try {
      const result = await toolsAPI.organizeLibrary(library.id, {
        dest_path: effectiveDestPath,
        transfer_mode: transferMode,
        dry_run: true,
      })
      setPreviewResult(result)
      toast.success('已生成整理预览')
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { error?: string } } })?.response?.data?.error || '预览失败'
      toast.error(msg)
    } finally {
      setPreviewBusy(false)
    }
  }

  const handleExecute = async () => {
    if (busy || previewBusy) return

    if (targetMode === 'custom' && !destPath.trim()) {
      toast.error('请输入指定目标文件夹路径')
      return
    }

    setBusy(true)
    try {
      if (!isOrganize) {
        // Pure metadata scrape
        await libraryAPI.scrape(library.id, {
          episode_images: scrapeEpisodeArtwork,
          refresh_matched: refreshMatched,
        })
        toast.success('刮削任务已加入后台队列')
      } else {
        // Organize and scrape
        const result = await toolsAPI.organizeLibrary(library.id, {
          dest_path: effectiveDestPath,
          transfer_mode: transferMode,
          scan_after: true,
          scrape_after: true,
        })
        const organized = result.organized ?? 0
        const replaced = result.replaced ?? 0
        const reclassified = result.reclassified ?? 0
        const skipped = result.skipped ?? 0
        toast.success(`整理与刮削已启动：新增 ${organized} · 替换 ${replaced} · 纠偏 ${reclassified} · 跳过 ${skipped}`)
      }

      await onCompleted?.()
      onClose()
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { error?: string } } })?.response?.data?.error || '操作失败'
      toast.error(msg)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-ink-900/40 px-4 py-6 backdrop-blur-sm">
      <div className="flex max-h-[90vh] w-full max-w-2xl flex-col overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-2xl">
        {/* Header */}
        <div className="flex items-start justify-between gap-4 border-b border-gray-200 px-6 py-4">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-brand-50 text-brand-600">
              <Sparkles size={20} />
            </div>
            <div>
              <h2 className="font-display text-xl font-bold text-gray-900">刮削元数据与整理</h2>
              <p className="mt-0.5 text-xs text-gray-500">
                对媒体库「<span className="font-semibold text-gray-700">{library.name}</span>」执行刮削与文件整理
              </p>
            </div>
          </div>
          <button onClick={onClose} className="btn-ghost h-9 w-9 p-0 text-gray-400 hover:text-gray-600" aria-label="关闭">
            <X size={18} />
          </button>
        </div>

        {/* Content Body */}
        <div className="flex-1 overflow-y-auto p-6 space-y-5">
          {isCloud && (
            <div className="flex items-start gap-2 rounded-xl border border-amber-200 bg-amber-50 p-3 text-xs font-medium text-amber-800">
              <AlertCircle size={16} className="shrink-0 mt-0.5 text-amber-600" />
              <span>当前为云盘媒体库，文件整理将受限。推荐仅使用元数据刮削，或在外部存储中配置云盘挂载/转移。</span>
            </div>
          )}

          {/* Mode Selection Cards */}
          <div className="space-y-3">
            <label className="text-xs font-bold uppercase tracking-wider text-gray-400">选择刮削整理目标位置</label>
            <div className="grid gap-3 sm:grid-cols-2">
              {/* Option 1: Current Folder */}
              <div
                onClick={() => setTargetMode('current')}
                className={`relative flex cursor-pointer flex-col justify-between rounded-xl border p-4 transition-all ${
                  targetMode === 'current'
                    ? 'border-brand-500 bg-brand-50/40 ring-2 ring-brand-500/20'
                    : 'border-gray-200 bg-white hover:border-gray-300'
                }`}
              >
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-2.5">
                    <Folder size={18} className={targetMode === 'current' ? 'text-brand-600' : 'text-gray-500'} />
                    <span className="font-bold text-sm text-gray-900">媒体当前文件夹</span>
                  </div>
                  {targetMode === 'current' && <CheckCircle2 size={16} className="text-brand-600" />}
                </div>
                <p className="mt-2 text-xs text-gray-500">
                  直接在当前媒体库目录内进行刮削整理，无需迁移文件。
                </p>
                <div className="mt-3 truncate rounded-lg bg-gray-100/80 px-2.5 py-1.5 font-mono text-[11px] text-gray-600" title={library.path}>
                  {library.path}
                </div>
              </div>

              {/* Option 2: Custom Specified Folder */}
              <div
                onClick={() => setTargetMode('custom')}
                className={`relative flex cursor-pointer flex-col justify-between rounded-xl border p-4 transition-all ${
                  targetMode === 'custom'
                    ? 'border-brand-500 bg-brand-50/40 ring-2 ring-brand-500/20'
                    : 'border-gray-200 bg-white hover:border-gray-300'
                }`}
              >
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-2.5">
                    <FolderInput size={18} className={targetMode === 'custom' ? 'text-brand-600' : 'text-gray-500'} />
                    <span className="font-bold text-sm text-gray-900">整理到指定文件夹</span>
                  </div>
                  {targetMode === 'custom' && <CheckCircle2 size={16} className="text-brand-600" />}
                </div>
                <p className="mt-2 text-xs text-gray-500">
                  将媒体规范整理转移至指定目标目录，并自动扫描入库刮削。
                </p>
                <div className="mt-3 truncate rounded-lg bg-gray-100/80 px-2.5 py-1.5 font-mono text-[11px] text-gray-600">
                  {destPath.trim() || '需自定义目标路径…'}
                </div>
              </div>
            </div>
          </div>

          {/* Sub-options for Current Folder Mode */}
          {targetMode === 'current' && (
            <div className="rounded-xl border border-gray-200 bg-gray-50/50 p-4 space-y-3">
              <label className="text-xs font-bold text-gray-600">当前文件夹执行方式</label>
              <div className="grid gap-2 sm:grid-cols-2">
                <label className={`flex cursor-pointer items-center gap-2.5 rounded-lg border p-2.5 text-xs font-semibold transition ${actionType === 'organize_and_scrape' ? 'border-brand-400 bg-brand-50 text-brand-700' : 'border-gray-200 bg-white text-gray-700'}`}>
                  <input
                    type="radio"
                    name="actionType"
                    checked={actionType === 'organize_and_scrape'}
                    onChange={() => setActionType('organize_and_scrape')}
                    className="text-brand-600"
                  />
                  <span>整理规范命名并刮削</span>
                </label>
                <label className={`flex cursor-pointer items-center gap-2.5 rounded-lg border p-2.5 text-xs font-semibold transition ${actionType === 'scrape_only' ? 'border-brand-400 bg-brand-50 text-brand-700' : 'border-gray-200 bg-white text-gray-700'}`}>
                  <input
                    type="radio"
                    name="actionType"
                    checked={actionType === 'scrape_only'}
                    onChange={() => setActionType('scrape_only')}
                    className="text-brand-600"
                  />
                  <span>仅原地刮削元数据（不重命名）</span>
                </label>
              </div>
            </div>
          )}

          {/* Custom Destination Input (when targetMode === 'custom') */}
          {targetMode === 'custom' && (
            <div className="rounded-xl border border-gray-200 bg-gray-50/50 p-4 space-y-3">
              <div>
                <label className="mb-1.5 block text-xs font-bold text-gray-700">
                  指定目标文件夹路径 <span className="text-red-500">*</span>
                </label>
                <div className="relative">
                  <input
                    type="text"
                    value={destPath}
                    onChange={(e) => setDestPath(e.target.value)}
                    placeholder="如 /media/organized 或 D:\Media\Movies"
                    className="h-10 w-full rounded-xl border border-gray-200 bg-white px-3 text-sm font-mono text-gray-800 outline-none focus:border-brand-500 focus:ring-1 focus:ring-brand-500"
                  />
                </div>
              </div>

              {availableLibraries.length > 0 && (
                <div>
                  <span className="mb-1 block text-[11px] font-medium text-gray-500">快捷填入已有媒体库路径：</span>
                  <div className="flex flex-wrap gap-1.5">
                    {availableLibraries.map((lib) => (
                      <button
                        key={lib.id}
                        type="button"
                        onClick={() => setDestPath(lib.path)}
                        className="rounded-lg border border-gray-200 bg-white px-2.5 py-1 text-[11px] font-medium text-gray-600 hover:border-brand-300 hover:bg-brand-50 hover:text-brand-700 transition"
                      >
                        {lib.name} ({lib.path})
                      </button>
                    ))}
                  </div>
                </div>
              )}
            </div>
          )}

          {/* Transfer Mode & Advanced Scrape Options */}
          <div className="grid gap-4 sm:grid-cols-2">
            {isOrganize && (
              <label>
                <span className="mb-1 block text-xs font-bold text-gray-600">转移方式</span>
                <select
                  value={transferMode}
                  onChange={(e) => setTransferMode(e.target.value)}
                  className="h-10 w-full rounded-xl border border-gray-200 bg-white px-3 text-xs font-semibold text-gray-700 outline-none focus:border-brand-300"
                >
                  <option value="hardlink">硬链接（Hardlink - 推荐，不占双份空间）</option>
                  <option value="move">移动（Move - 转移源文件）</option>
                  <option value="copy">复制（Copy - 占用双倍空间）</option>
                  <option value="symlink">软链接（Symlink）</option>
                </select>
              </label>
            )}

            <div className={`space-y-2.5 ${!isOrganize ? 'sm:col-span-2' : ''}`}>
              <span className="block text-xs font-bold text-gray-600">刮削偏好设置</span>
              <div className="flex flex-wrap gap-2">
                <EpisodeArtworkToggle
                  checked={scrapeEpisodeArtwork}
                  onChange={onScrapeEpisodeArtworkChange}
                  title="关闭后仍会获取主海报和每集文字元数据，只跳过每集图片"
                  className="h-10 text-xs"
                />
                <label className="flex h-10 items-center gap-2 rounded-xl border border-gray-200 bg-white px-3 text-xs font-semibold text-gray-700 cursor-pointer hover:border-gray-300">
                  <input
                    type="checkbox"
                    checked={refreshMatched}
                    onChange={(e) => setRefreshMatched(e.target.checked)}
                    className="h-4 w-4 rounded border-gray-300 text-brand-600 focus:ring-brand-500"
                  />
                  <span>覆盖/刷新已有元数据</span>
                </label>
              </div>
            </div>
          </div>

          {/* Preview Results */}
          {previewResult && (
            <div className="rounded-xl border border-brand-200 bg-brand-50/50 p-4 text-xs space-y-1.5">
              <div className="font-bold text-brand-900 flex items-center gap-1.5">
                <CheckCircle2 size={14} className="text-brand-600" />
                <span>整理预览结果：</span>
              </div>
              <div className="text-gray-700 flex flex-wrap gap-x-4 gap-y-1">
                <span>待新增：<strong className="text-green-600">{previewResult.organized ?? 0}</strong></span>
                <span>替换/洗版：<strong>{previewResult.replaced ?? 0}</strong></span>
                <span>纠偏：<strong>{previewResult.reclassified ?? 0}</strong></span>
                <span>已在目标/跳过：<strong>{previewResult.skipped ?? 0}</strong></span>
              </div>
              {previewResult.errors && previewResult.errors.length > 0 && (
                <div className="mt-2 text-red-600">
                  <span className="font-semibold">部分异常 ({previewResult.errors.length})：</span>
                  <ul className="list-disc list-inside mt-0.5 space-y-0.5 font-mono text-[11px]">
                    {previewResult.errors.slice(0, 3).map((err, i) => (
                      <li key={i} className="truncate">{err}</li>
                    ))}
                  </ul>
                </div>
              )}
            </div>
          )}
        </div>

        {/* Footer Actions */}
        <div className="flex items-center justify-between border-t border-gray-200 px-6 py-4 bg-gray-50">
          <div>
            {isOrganize && (
              <button
                type="button"
                onClick={handlePreview}
                disabled={previewBusy || busy || isCloud}
                className="btn-outline px-4 text-xs h-9"
              >
                {previewBusy ? '预览生成中…' : '预览整理路径'}
              </button>
            )}
          </div>
          <div className="flex items-center gap-2.5">
            <button type="button" onClick={onClose} className="btn-outline px-4 text-xs h-9">
              取消
            </button>
            <button
              type="button"
              onClick={handleExecute}
              disabled={busy || previewBusy || (isCloud && targetMode === 'custom')}
              className="btn-primary px-5 text-xs h-9 gap-1.5"
            >
              {busy ? <RefreshCw size={14} className="animate-spin" /> : <Sparkles size={14} />}
              <span>{busy ? '任务提交中…' : isOrganize ? '开始刮削整理' : '开始刮削元数据'}</span>
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
