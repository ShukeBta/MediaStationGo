import { Link } from 'react-router-dom'

import { AIAssistantHeader } from './AIAssistantHeader'
import { AIAssistantRecommendationsSection } from './AIAssistantRecommendationsSection'
import { AIAssistantSearchSection } from './AIAssistantSearchSection'
import { useAIAssistantPage } from './useAIAssistantPage'

// AIAssistantPage exposes the two AI helpers backed by the Go server:
//   - smart search: parses a natural-language query into a SearchIntent +
//     a list of matching local media items.
//   - recommendations: returns a list of recommended titles based on the
//     current user's recent watch history.
//
// The Vue version had a full chat surface; the Go backend has no chat or
// operation-execute endpoints, so we render the same two capabilities as
// a focused two-panel screen.
export function AIAssistantPage() {
  const assistant = useAIAssistantPage()

  return (
    <div className="space-y-6">
      <AIAssistantHeader status={assistant.status} />

      <AIAssistantSearchSection
        query={assistant.query}
        searching={assistant.searching}
        intent={assistant.intent}
        items={assistant.items}
        localCards={assistant.localCards}
        externalItems={assistant.externalItems}
        onSearch={assistant.onSearch}
        setQuery={assistant.setQuery}
      />

      <AIAssistantRecommendationsSection
        recs={assistant.recs}
        recommending={assistant.recommending}
        onRecommend={assistant.onRecommend}
      />

      {/* Decorative footer (mirrors the Vue page hint that AI runs locally). */}
      {!assistant.status?.enabled && (
        <p className="text-xs text-sand-500">
          提示: 当前未配置外部 AI Provider,系统将使用本地规则引擎解析查询。
          管理员可在 <Link to="/admin?tab=api" className="text-brand-500">API 配置</Link>{' '}
          中接入 OpenAI / DeepSeek 等服务以获得更好效果。
        </p>
      )}
    </div>
  )
}
