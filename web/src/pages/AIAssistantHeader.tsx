import { Sparkles } from 'lucide-react'

export type AIAssistantStatus = {
  enabled: boolean
  provider: string
  model: string
}

type AIAssistantHeaderProps = {
  status: AIAssistantStatus | null
}

export function AIAssistantHeader({ status }: AIAssistantHeaderProps) {
  return (
    <div className="flex flex-wrap items-end justify-between gap-3">
      <div className="flex items-center gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-gradient-to-br from-primary-400 to-purple-500">
          <Sparkles className="h-5 w-5 text-ink-600" />
        </div>
        <div>
          <h1 className="font-display text-3xl font-bold text-ink-600">AI 助手</h1>
          <p className="text-sm text-ink-50">自然语言搜索 · 基于观影历史的智能推荐</p>
        </div>
      </div>
      {status && (
        <div className="text-xs text-ink-50">
          <span
            className={
              'mr-2 inline-block h-2 w-2 rounded-full ' +
              (status.enabled ? 'bg-emerald-400' : 'bg-sand-500/30')
            }
          />
          {status.enabled
            ? `已连接 · ${status.provider}${status.model ? ' / ' + status.model : ''}`
            : '未配置 AI 服务,使用本地规则解析'}
        </div>
      )}
    </div>
  )
}
