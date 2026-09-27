import { ExternalResults } from './SearchExternalResults'
import { SearchHeader } from './SearchHeader'
import { SearchInputBar } from './SearchInputBar'
import { SearchLocalResults } from './SearchLocalResults'
import { SearchStatusPanels } from './SearchStatusPanels'
import { useSearchPage } from './useSearchPage'
import { usePermission } from '../hooks/usePermission'

export function SearchPage() {
  const canUseAI = usePermission('can_use_ai')
  const search = useSearchPage({ canUseAI })

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
    </div>
  )
}
