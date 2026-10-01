import { ExternalResults } from './SearchExternalResults'
import { SearchHeader } from './SearchHeader'
import { SearchInputBar } from './SearchInputBar'
import { SearchLocalResults } from './SearchLocalResults'
import { SearchPeopleResults } from './SearchPeopleResults'
import { Link } from 'react-router-dom'
import { SearchStatusPanels } from './SearchStatusPanels'
import { useSearchPage } from './useSearchPage'
import { usePermission } from '../hooks/usePermission'

export function SearchPage() {
  const canUseAI = usePermission('can_use_ai')
  const canViewDiscover = usePermission('can_view_discover')
  const canSearchSites = usePermission('can_manage_sites')
  const search = useSearchPage({ canUseAI, canViewDiscover })

  return (
    <div className="space-y-6">
      <SearchHeader
        aiOn={search.aiOn}
        aiAvailable={search.aiAvailable}
        canUseAI={canUseAI}
        onToggleAI={() => search.setAiOn((on) => !on)}
      />

      <SearchInputBar
        aiOn={search.aiOn}
        query={search.q}
        onQueryChange={search.setQ}
        onClear={search.clearQuery}
        onAISubmit={search.onAISubmit}
      />

      <Link to="/people" className="inline-block text-sm text-brand-600">浏览人物资料</Link>
      {canSearchSites && <Link to={`/site-search?q=${encodeURIComponent(search.q.trim())}`} className="ml-4 inline-block text-sm font-semibold text-brand-600">直接搜索 PT 资源</Link>}
      <SearchPeopleResults query={search.q} />

      {search.intent && (
        <div className="glass-panel !p-3 text-xs text-ink-100">
          AI 解析:
          <span className="ml-2 font-mono text-brand-500">{JSON.stringify(search.intent)}</span>
        </div>
      )}

      <SearchStatusPanels
        loading={search.loading}
        error={search.error}
        showIdle={search.showIdle}
        showEmpty={search.showEmpty}
      />

      <SearchLocalResults
        localCards={search.localCards}
        itemCount={search.itemCount}
        searchTotal={search.searchTotal}
        loading={search.loading}
        loadingMore={search.loadingMore}
        hasMore={search.hasMore}
        onLoadMore={() => void search.loadMore()}
      />

      {search.externalItems.length > 0 && (
        <ExternalResults
          items={search.externalItems}
        />
      )}
      {search.externalLoading && <p className="text-sm text-sand-500">正在查找外部影视资料…</p>}
      {search.externalError && <p className="text-sm text-amber-600">部分外部资料暂不可用：{search.externalError}</p>}
    </div>
  )
}
