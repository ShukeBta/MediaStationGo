type SearchStatusPanelsProps = {
  loading: boolean
  error: string
  showIdle: boolean
  showEmpty: boolean
}

export function SearchStatusPanels({
  loading,
  error,
  showIdle,
  showEmpty,
}: SearchStatusPanelsProps) {
  return (
    <>
      {loading && (
        <div className="flex items-center gap-2 py-8 text-ink-50">
          <span className="inline-block h-4 w-4 animate-spin rounded-full border-2 border-primary-400 border-t-transparent" />
          搜索中…
        </div>
      )}

      {error && (
        <div className="glass-panel !border-red-400/30 p-4 text-sm text-red-400">{error}</div>
      )}

      {showIdle && (
        <div className="glass-panel flex flex-col items-center gap-2 p-10 text-center">
          <p className="text-lg text-ink-100">输入关键词开始搜索</p>
          <p className="text-sm text-sand-500">支持电影、电视剧、动漫等媒体内容的快速搜索</p>
        </div>
      )}

      {showEmpty && (
        <div className="glass-panel flex flex-col items-center gap-2 p-10 text-center">
          <p className="text-lg text-ink-100">未找到匹配的媒体</p>
          <p className="text-sm text-sand-500">尝试其他关键词，或者添加媒体库后执行扫描</p>
        </div>
      )}
    </>
  )
}
