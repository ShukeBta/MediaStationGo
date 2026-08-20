import { useState } from 'react'
import toast from 'react-hot-toast'
import { X } from 'lucide-react'

import { toolsAPI } from '../api/tools'
import { libraryAPI } from '../api/library'
import type { BackgroundTask, TaskItem } from '../api/tasks'

const kindLabels: Record<string, string> = {
  organize: '整理 / 重命名',
  scan: '入库扫描',
  scrape: '刮削',
}

// ItemRetryDialog lets the operator manually re-handle a single failed item.
// It pre-fills the values captured from the failed task so retrying is
// reproducible, while still allowing overrides before re-running.
export function ItemRetryDialog({
  task,
  item,
  onClose,
}: {
  task: BackgroundTask
  item: TaskItem
  onClose: () => void
}) {
  const [sourcePath, setSourcePath] = useState(item.source || task.source_path || '')
  const [destPath, setDestPath] = useState(item.dest_path || task.dest_path || '')
  const [transferMode, setTransferMode] = useState('hardlink')
  const [scanAfter, setScanAfter] = useState(true)
  const [scrapeAfter, setScrapeAfter] = useState(true)
  const [busy, setBusy] = useState(false)

  const run = async () => {
    setBusy(true)
    try {
      if (item.kind === 'organize') {
        if (!sourcePath.trim()) {
          toast.error('整理需要来源路径')
          return
        }
        await toolsAPI.organizeDirectory({
          source_path: sourcePath.trim(),
          dest_path: destPath.trim() || undefined,
          transfer_mode: transferMode,
          scan_after: scanAfter,
          scrape_after: scanAfter && scrapeAfter,
        })
        toast.success(`已重新整理：${item.name}`)
      } else if (item.kind === 'scan' && item.library_id) {
        await libraryAPI.scan(item.library_id)
        toast.success(`已重新入库：${item.name}`)
      } else if (item.kind === 'scrape' && item.library_id) {
        await libraryAPI.scrape(item.library_id, {
          episode_artwork: true,
          refresh_matched: true,
        })
        toast.success(`已重新刮削：${item.name}`)
      } else {
        toast.error(`无法处理该类型条目（${item.kind}）`)
        return
      }
      onClose()
    } catch (err: unknown) {
      toast.error((err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '处理失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" onClick={onClose}>
      <div
        className="glass-card-elevated w-full max-w-lg space-y-4 rounded-2xl bg-white p-5 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-start justify-between">
          <div>
            <h3 className="font-display text-lg font-semibold text-ink-600">手动处理失败条目</h3>
            <p className="text-xs text-sand-500">
              {item.name} · {kindLabels[item.kind] ?? item.kind}
            </p>
          </div>
          <button
            type="button"
            className="rounded-lg p-1 text-sand-500 hover:bg-gray-100"
            onClick={onClose}
            aria-label="关闭"
          >
            <X size={18} />
          </button>
        </div>

        {item.error && (
          <div className="rounded-lg border border-red-200 bg-red-50 p-2 text-xs text-red-600">{item.error}</div>
        )}

        <div className="space-y-3">
          {item.kind === 'organize' && (
            <>
              <label className="block text-sm">
                <span className="mb-1 block text-xs font-medium text-sand-500">来源路径</span>
                <input
                  className="w-full rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm text-ink-600 outline-none focus:border-primary-400"
                  value={sourcePath}
                  onChange={(e) => setSourcePath(e.target.value)}
                />
              </label>
              <label className="block text-sm">
                <span className="mb-1 block text-xs font-medium text-sand-500">目标路径（留空则用默认）</span>
                <input
                  className="w-full rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm text-ink-600 outline-none focus:border-primary-400"
                  value={destPath}
                  onChange={(e) => setDestPath(e.target.value)}
                />
              </label>
              <label className="block text-sm">
                <span className="mb-1 block text-xs font-medium text-sand-500">转移方式</span>
                <select
                  className="w-full rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm text-ink-600 outline-none focus:border-primary-400"
                  value={transferMode}
                  onChange={(e) => setTransferMode(e.target.value)}
                >
                  <option value="hardlink">硬链接</option>
                  <option value="move">移动</option>
                  <option value="copy">复制</option>
                </select>
              </label>
              <div className="flex items-center gap-4">
                <label className="flex items-center gap-1.5 text-sm text-ink-600">
                  <input type="checkbox" checked={scanAfter} onChange={(e) => setScanAfter(e.target.checked)} />
                  入库
                </label>
                <label className="flex items-center gap-1.5 text-sm text-ink-600">
                  <input
                    type="checkbox"
                    checked={scrapeAfter}
                    disabled={!scanAfter}
                    onChange={(e) => setScrapeAfter(e.target.checked)}
                  />
                  刮削
                </label>
              </div>
            </>
          )}

          {item.kind === 'scan' && (
            <p className="text-sm text-sand-600">
              将重新扫描媒体库「{item.name}」以入库。完成后会自动触发刮削（如已启用）。
            </p>
          )}

          {item.kind === 'scrape' && (
            <p className="text-sm text-sand-600">
              将重新刮削媒体库「{item.name}」。会重试上次未匹配的条目并刷新元数据。
            </p>
          )}
        </div>

        <div className="flex justify-end gap-2 pt-1">
          <button type="button" className="btn-ghost" onClick={onClose} disabled={busy}>
            取消
          </button>
          <button type="button" className="btn-primary" onClick={() => void run()} disabled={busy}>
            {busy ? '处理中…' : '开始处理'}
          </button>
        </div>
      </div>
    </div>
  )
}
