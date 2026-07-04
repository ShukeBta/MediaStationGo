import { ArrowUp, FolderPlus } from 'lucide-react'

interface CloudBrowserToolbarProps {
  stack: { id: string; name: string }[]
  mountMediaType: string
  mounting: boolean
  batchMounting: boolean
  loading: boolean
  hasDirectories: boolean
  onGoTo: (index: number) => void
  onGoUp: () => void
  onCreateFolder: () => void
  onMediaTypeChange: (value: string) => void
  onMountCurrent: () => void
  onMountVisibleDirectories: () => void
}

export function CloudBrowserToolbar({
  stack,
  mountMediaType,
  mounting,
  batchMounting,
  loading,
  hasDirectories,
  onGoTo,
  onGoUp,
  onCreateFolder,
  onMediaTypeChange,
  onMountCurrent,
  onMountVisibleDirectories,
}: CloudBrowserToolbarProps) {
  return (
    <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
      <div className="flex flex-wrap items-center gap-1 text-xs text-[var(--app-muted)]">
        <span className="text-[var(--app-subtle)]">网盘资源:</span>
        {stack.map((item, index) => (
          <span key={index}>
            <button type="button" className="hover:text-brand-500" onClick={() => onGoTo(index)}>
              {item.name}
            </button>
            {index < stack.length - 1 && <span className="mx-1">/</span>}
          </span>
        ))}
      </div>
      <p className="basis-full text-xs text-[var(--app-muted)]">
        挂载后不会复制网盘文件；后台会递归读取该目录里的子文件夹和媒体文件，扫描到的影片会自动加入对应媒体库。小目录通常几十秒，大目录取决于网盘接口速度。
        如果已有同名同类型媒体库，会在首页和 Emby/SenPlayer 中自动归并显示。
      </p>
      <div className="flex flex-wrap items-center gap-2">
        <button
          type="button"
          className="inline-flex items-center gap-1 rounded border border-[var(--app-border)] px-2 py-0.5 text-xs text-[var(--app-subtle)] hover:bg-[var(--app-hover)] hover:text-[var(--app-text)] disabled:cursor-not-allowed disabled:opacity-50"
          disabled={loading || stack.length <= 1}
          onClick={onGoUp}
        >
          <ArrowUp size={14} />
          返回上级
        </button>
        <button
          type="button"
          className="inline-flex items-center gap-1 rounded border border-[var(--app-border)] px-2 py-0.5 text-xs text-[var(--app-subtle)] hover:bg-[var(--app-hover)] hover:text-[var(--app-text)] disabled:cursor-not-allowed disabled:opacity-50"
          disabled={loading}
          onClick={onCreateFolder}
        >
          <FolderPlus size={14} />
          新建文件夹
        </button>
        <select
          className="rounded border border-[var(--app-border)] bg-[var(--app-panel)] px-2 py-0.5 text-xs text-[var(--app-subtle)]"
          value={mountMediaType}
          onChange={(event) => onMediaTypeChange(event.target.value)}
        >
          <option value="auto">自动识别</option>
          <option value="movie">电影</option>
          <option value="tv">剧集</option>
          <option value="anime">动漫</option>
          <option value="variety">综艺</option>
          <option value="adult">成人</option>
        </select>
        <button
          type="button"
          className="rounded border border-brand-400/40 px-2 py-0.5 text-xs text-brand-500 hover:bg-brand-400/10 disabled:opacity-50"
          disabled={mounting || batchMounting || loading}
          onClick={onMountCurrent}
        >
          {mounting ? '挂载中…' : '挂载当前目录并归并到媒体库'}
        </button>
        <button
          type="button"
          className="rounded border border-blue-300/70 px-2 py-0.5 text-xs text-blue-500 hover:bg-blue-500/10 disabled:opacity-50"
          disabled={mounting || batchMounting || loading || !hasDirectories}
          onClick={onMountVisibleDirectories}
        >
          {batchMounting ? '批量挂载中…' : '一键挂载当前目录下所有文件夹'}
        </button>
      </div>
    </div>
  )
}
