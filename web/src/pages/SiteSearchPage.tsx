import { useSearchParams } from 'react-router-dom'
import { Search } from 'lucide-react'
import { PTResourceSearchPanel } from './PTResourceSearchPanel'

export function SiteSearchPage() {
  const [params] = useSearchParams()
  const query = params.get('q') || ''
  return <div className="space-y-6">
    <header className="flex items-center gap-3">
      <Search className="h-6 w-6 text-brand-500" />
      <div><h1 className="font-display text-3xl font-bold text-ink-600">PT 站点搜索</h1><p className="text-sm text-ink-50">跨站搜索、直接下载和持续订阅。</p></div>
    </header>
    <div className="glass-panel"><PTResourceSearchPanel key={query} initialQuery={query} /></div>
  </div>
}
