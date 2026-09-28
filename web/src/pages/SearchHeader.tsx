import { Sparkles } from 'lucide-react'

type SearchHeaderProps = {
  aiOn: boolean
  aiAvailable: boolean
  canUseAI: boolean
  onToggleAI: () => void
}

export function SearchHeader({ aiOn, aiAvailable, canUseAI, onToggleAI }: SearchHeaderProps) {
  return (
    <header className="flex items-center justify-between">
      <h1 className="font-display text-3xl font-bold text-ink-600">搜索</h1>
      {canUseAI && (
        <button
          className={
            'neon-button !px-3 !py-1 !text-xs ' +
            (aiOn ? '!border-accent-400 !bg-accent-400/20 !text-accent-400' : '')
          }
          onClick={onToggleAI}
          disabled={!aiAvailable}
          title={aiAvailable ? '启用 AI 智能搜索' : 'AI 智能搜索当前不可用'}
        >
          <Sparkles size={12} /> {aiOn ? '智能搜索已开启' : '智能搜索'}
        </button>
      )}
    </header>
  )
}
