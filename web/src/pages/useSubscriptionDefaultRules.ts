import { useCallback, useEffect, useRef, useState } from 'react'

import { subscriptionsAPI, type SubscriptionRuleDefaults } from '../api/subscriptions'

export function useSubscriptionDefaultRules() {
  const [rules, setRules] = useState<SubscriptionRuleDefaults | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const sequence = useRef(0)

  const reload = useCallback(async () => {
    const request = ++sequence.current
    setLoading(true)
    setError('')
    try {
      const result = await subscriptionsAPI.defaultRules()
      if (sequence.current === request) setRules(result)
    } catch (requestError) {
      if (sequence.current === request) setError(defaultRulesError(requestError, '默认订阅规则加载失败'))
    } finally {
      if (sequence.current === request) setLoading(false)
    }
  }, [])

  useEffect(() => {
    void reload()
    return () => { sequence.current += 1 }
  }, [reload])

  const save = async (input: SubscriptionRuleDefaults) => {
    const result = await subscriptionsAPI.saveDefaultRules(input)
    sequence.current += 1
    setRules(result)
    setError('')
    setLoading(false)
    return result
  }

  return { rules, loading, error, reload, save }
}

export function defaultRulesError(error: unknown, fallback: string): string {
  return (error as { response?: { data?: { error?: string } } })?.response?.data?.error
    || (error instanceof Error ? error.message : '') || fallback
}
