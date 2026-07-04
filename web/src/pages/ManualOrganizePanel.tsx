import type { Library } from '../types'

type PreviewItem = {
  source: string
  target?: string
  action: string
  reason?: string
}

type ManualOrganizePanelProps = {
  organizeSource: string
  selectedCount: number
  localLibraries: Library[]
  organizeLibraryID: string
  organizeDestPath: string
  organizeMediaType: string
  organizeTransferMode: string
  manualMoveKeepsSeeding: boolean
  scanAfter: boolean
  scrapeAfter: boolean
  organizeReady: boolean
  organizeBusy: string
  previewItems: PreviewItem[]
  onClearSelected: () => void
  onLibraryChange: (value: string) => void
  onDestPathChange: (value: string) => void
  onMediaTypeChange: (value: string) => void
  onTransferModeChange: (value: string) => void
  onScanAfterChange: (value: boolean) => void
  onScrapeAfterChange: (value: boolean) => void
  onPreview: () => void
  onRun: () => void
}

export function ManualOrganizePanel({
  organizeSource,
  selectedCount,
  localLibraries,
  organizeLibraryID,
  organizeDestPath,
  organizeMediaType,
  organizeTransferMode,
  manualMoveKeepsSeeding,
  scanAfter,
  scrapeAfter,
  organizeReady,
  organizeBusy,
  previewItems,
  onClearSelected,
  onLibraryChange,
  onDestPathChange,
  onMediaTypeChange,
  onTransferModeChange,
  onScanAfterChange,
  onScrapeAfterChange,
  onPreview,
  onRun,
}: ManualOrganizePanelProps) {
  return (
    <section className="glass-panel space-y-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="font-display text-lg font-semibold text-ink-600">手动整理入库</h2>
          <p className="text-xs text-sand-500">来源优先使用选中项；未选中时使用当前目录。</p>
        </div>
        <div className="max-w-xl truncate rounded-xl border border-gray-200 bg-gray-50 px-3 py-2 font-mono text-xs text-ink-100" title={organizeSource}>
          来源：{organizeSource || '未选择'}
        </div>
      </div>
      {selectedCount > 0 && (
        <div className="flex flex-wrap items-center gap-2 rounded-xl border border-primary-400/20 bg-primary-400/5 px-3 py-2 text-xs text-ink-100">
          <span>已选择 {selectedCount} 个项目用于整理。</span>
          <button type="button" className="font-semibold text-brand-500 hover:text-brand-700" onClick={onClearSelected}>
            清空选择
          </button>
        </div>
      )}

      <div className="grid gap-3 lg:grid-cols-[1.2fr_1fr_150px_150px]">
        <label className="space-y-1">
          <span className="text-xs text-ink-50">目标媒体库 / 存储</span>
          <select
            className="input-base w-full"
            value={organizeLibraryID}
            onChange={(event) => onLibraryChange(event.target.value)}
          >
            <option value="">手动填写目的路径</option>
            {localLibraries.map((library) => (
              <option key={library.id} value={library.id}>
                {library.name}（{library.type}）— {library.path}
              </option>
            ))}
          </select>
          <span className="text-[11px] text-sand-500">
            手动整理只写入本地可写媒体库；网盘请到“外部存储”中挂载、扫描或转存。
          </span>
        </label>
        <label className="space-y-1">
          <span className="text-xs text-ink-50">目的路径</span>
          <input
            className="input-base w-full"
            placeholder="例如 F:\\media\\电影 或 /media/电影"
            value={organizeDestPath}
            onChange={(event) => onDestPathChange(event.target.value)}
          />
        </label>
        <label className="space-y-1">
          <span className="text-xs text-ink-50">类型</span>
          <select className="input-base w-full" value={organizeMediaType} onChange={(event) => onMediaTypeChange(event.target.value)}>
            <option value="auto">自动识别</option>
            <option value="movie">电影</option>
            <option value="tv">剧集</option>
            <option value="anime">动漫</option>
            <option value="variety">综艺</option>
            <option value="adult">成人</option>
          </select>
        </label>
        <label className="space-y-1">
          <span className="text-xs text-ink-50">整理方式</span>
          <select className="input-base w-full" value={organizeTransferMode} onChange={(event) => onTransferModeChange(event.target.value)}>
            <option value="hardlink">硬链接</option>
            <option value="move">移动（关闭保种才会移动）</option>
            <option value="copy">复制</option>
            <option value="symlink">软链接</option>
          </select>
        </label>
      </div>

      {manualMoveKeepsSeeding && (
        <div className="rounded-xl border border-orange-300 bg-orange-50 px-3 py-2 text-xs text-orange-700">
          “保种”已开启，选择“移动”时后端会改用硬链接以保留下载源。要执行真正移动，请先在上方自动整理设置里关闭“保种”并保存。
        </div>
      )}

      <div className="flex flex-wrap items-center gap-2">
        <label className="flex items-center gap-2 rounded-lg border border-gray-200 bg-gray-50 px-2 py-1 text-xs text-ink-100">
          <input type="checkbox" checked={scanAfter} onChange={(event) => onScanAfterChange(event.target.checked)} />
          整理后扫描入库
        </label>
        <label className="flex items-center gap-2 rounded-lg border border-gray-200 bg-gray-50 px-2 py-1 text-xs text-ink-100">
          <input
            type="checkbox"
            checked={scanAfter && scrapeAfter}
            disabled={!scanAfter}
            onChange={(event) => onScrapeAfterChange(event.target.checked)}
          />
          整理后自动刮削
        </label>
        <button
          type="button"
          className="neon-button !border-primary-400/30 !bg-white !text-brand-500"
          disabled={!organizeReady || organizeBusy !== ''}
          onClick={onPreview}
        >
          {organizeBusy === 'preview' ? '预览中…' : '预览整理'}
        </button>
        <button
          type="button"
          className="neon-button"
          disabled={!organizeReady || organizeBusy !== ''}
          onClick={onRun}
        >
          {organizeBusy === 'run' ? '整理中…' : '开始整理入库'}
        </button>
      </div>

      {previewItems.length > 0 && <ManualOrganizePreviewTable items={previewItems} />}
    </section>
  )
}

function ManualOrganizePreviewTable({ items }: { items: PreviewItem[] }) {
  return (
    <div className="max-h-72 overflow-auto rounded-xl border border-gray-200 bg-white/70">
      <table className="w-full text-left text-xs">
        <thead className="sticky top-0 bg-white text-sand-500">
          <tr>
            <th className="px-3 py-2">动作</th>
            <th>来源</th>
            <th>目标</th>
            <th>原因</th>
          </tr>
        </thead>
        <tbody>
          {items.map((item, index) => (
            <tr key={`${item.source}-${index}`} className="border-t border-gray-200 align-top">
              <td className="px-3 py-2 font-semibold text-brand-500">{item.action}</td>
              <td className="max-w-xs truncate py-2 font-mono text-ink-100" title={item.source}>{item.source}</td>
              <td className="max-w-xs truncate py-2 font-mono text-ink-100" title={item.target}>{item.target || '—'}</td>
              <td className="py-2 text-sand-500">{item.reason || '—'}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
