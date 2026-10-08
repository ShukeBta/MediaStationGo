import { FormEvent, useState } from 'react'
import { Loader2, Save } from 'lucide-react'
import toast from 'react-hot-toast'

import type { SubscriptionRuleDefaults } from '../api/subscriptions'
import { SubscriptionRuleFields } from './SubscriptionRuleFields'
import { subscriptionRuleDefaultsInput, subscriptionRuleFormValues, type SubscriptionRuleFormValues } from './subscriptionDefaultRulesModel'
import { defaultRulesError } from './useSubscriptionDefaultRules'
import type { SubscriptionFormValues } from './subscriptionFormModel'

export function SubscriptionDefaultRulesPanel({ rules, onSave, onClose }: {
  rules: SubscriptionRuleDefaults
  onSave: (rules: SubscriptionRuleDefaults) => Promise<SubscriptionRuleDefaults>
  onClose: () => void
}) {
  const [values, setValues] = useState(() => subscriptionRuleFormValues(rules))
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const update = <K extends keyof SubscriptionRuleFormValues>(key: K, value: SubscriptionFormValues[K]) => {
    setValues((current) => ({ ...current, [key]: value }))
  }
  const save = async (event: FormEvent) => {
    event.preventDefault()
    if (saving) return
    setError('')
    setSaving(true)
    try {
      const saved = await onSave(subscriptionRuleDefaultsInput(values))
      setValues(subscriptionRuleFormValues(saved))
      toast.success('默认订阅规则已保存')
    } catch (requestError) {
      setError(defaultRulesError(requestError, '默认订阅规则保存失败'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <form onSubmit={save} className="glass-panel space-y-4 border-primary-300" aria-labelledby="subscription-default-rules-title">
      <div>
        <h2 id="subscription-default-rules-title" className="text-lg font-semibold text-ink-600">默认订阅规则</h2>
        <p className="mt-1 text-xs text-sand-500">用于之后新建的 PT、RSS 和网盘追更。网盘追更仅使用扫描频率、分辨率、质量、特效、发布组和排除词；已有订阅可在编辑时单独套用。</p>
      </div>
      <SubscriptionRuleFields values={values} onChange={update} disabled={saving} />
      {error && <p role="alert" className="text-sm text-red-600">{error}</p>}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-xs text-sand-500">数量或体积留空、填写 0 均表示不限。</p>
        <div className="flex gap-2">
          <button type="button" className="btn-outline" onClick={onClose} disabled={saving}>收起</button>
          <button type="submit" className="btn-primary gap-2" disabled={saving}>
            {saving ? <Loader2 size={15} className="animate-spin" /> : <Save size={15} />}
            {saving ? '保存中…' : '保存默认规则'}
          </button>
        </div>
      </div>
    </form>
  )
}
