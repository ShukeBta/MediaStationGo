import { Link } from 'react-router-dom'
import { Loader2, Search, Wand2 } from 'lucide-react'

type AIAssistantRecommendationsSectionProps = {
  recs: string[] | null
  recommending: boolean
  onRecommend: () => void
}

export function AIAssistantRecommendationsSection({
  recs,
  recommending,
  onRecommend,
}: AIAssistantRecommendationsSectionProps) {
  return (
    <section className="glass-panel space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="font-display text-lg font-semibold text-ink-600">为你推荐</h2>
        <button onClick={onRecommend} disabled={recommending} className="neon-button">
          {recommending ? <Loader2 size={16} className="animate-spin" /> : <Wand2 size={16} />}
          生成推荐
        </button>
      </div>
      <p className="text-xs text-sand-500">推荐基于你的最近观看历史。点击标题在媒体库中查找。</p>

      {recs && recs.length > 0 && (
        <ul className="grid gap-2 sm:grid-cols-2">
          {recs.map((title, index) => (
            <li key={index}>
              <Link
                to={`/search?q=${encodeURIComponent(title)}`}
                className="flex items-center justify-between rounded-xl border border-gray-200 bg-gray-50 px-3 py-2 text-sm text-ink-200 hover:border-primary-400/40 hover:text-brand-500"
              >
                <span className="truncate">{title}</span>
                <Search size={14} className="shrink-0 opacity-60" />
              </Link>
            </li>
          ))}
        </ul>
      )}

      {recs && recs.length === 0 && (
        <p className="text-sm text-ink-50">还没有推荐结果 — 先去看几部片子,我再给你挑。</p>
      )}
    </section>
  )
}
