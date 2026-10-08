import type { SubscriptionRuleDefaults } from '../api/subscriptions'
import type { SubscriptionFormValues } from './subscriptionFormModel'

export const subscriptionRuleFieldKeys = [
  'pollIntervalMinutes', 'resolution', 'quality', 'effects', 'releaseGroups', 'excludeWords',
  'minSeeders', 'maxSeeders', 'minSizeGB', 'maxSizeGB', 'freeOnly', 'washEnabled', 'washPriority',
] as const

export type SubscriptionRuleFormValues = Pick<SubscriptionFormValues, typeof subscriptionRuleFieldKeys[number]>

export function subscriptionRuleFormValues(rules: SubscriptionRuleDefaults): SubscriptionRuleFormValues {
  return {
    pollIntervalMinutes: String(rules.poll_interval_minutes),
    resolution: rules.resolution,
    quality: rules.quality,
    effects: rules.effects,
    releaseGroups: rules.release_groups,
    excludeWords: rules.exclude_words,
    minSeeders: rules.min_seeders > 0 ? String(rules.min_seeders) : '',
    maxSeeders: rules.max_seeders > 0 ? String(rules.max_seeders) : '',
    minSizeGB: rules.min_size_gb > 0 ? String(rules.min_size_gb) : '',
    maxSizeGB: rules.max_size_gb > 0 ? String(rules.max_size_gb) : '',
    freeOnly: rules.free_only,
    washEnabled: rules.wash_enabled,
    washPriority: rules.wash_priority,
  }
}

// Only rule fields participate. Identity, search terms, season and destination
// stay local to the form; late network responses preserve explicit edits.
export function applySubscriptionRuleDefaults(
  values: SubscriptionFormValues,
  rules: SubscriptionRuleDefaults | null,
  edited: ReadonlySet<keyof SubscriptionFormValues> = new Set(),
): SubscriptionFormValues {
  if (!rules) return { ...values }
  const changes = subscriptionRuleFormValues(rules)
  return { ...values, ...Object.fromEntries(Object.entries(changes).filter(([key]) => !edited.has(key as keyof SubscriptionFormValues))) }
}

export function subscriptionRuleDefaultsInput(values: SubscriptionRuleFormValues): SubscriptionRuleDefaults {
  const rules = {
    poll_interval_minutes: ruleNumber(values.pollIntervalMinutes, '扫描频率', true),
    resolution: values.resolution,
    quality: values.quality,
    effects: values.effects.trim(),
    release_groups: values.releaseGroups.trim(),
    exclude_words: values.excludeWords.trim(),
    min_seeders: ruleNumber(values.minSeeders, '最少做种数', true),
    max_seeders: ruleNumber(values.maxSeeders, '最多做种数', true),
    min_size_gb: ruleNumber(values.minSizeGB, '最小体积'),
    max_size_gb: ruleNumber(values.maxSizeGB, '最大体积'),
    free_only: values.freeOnly,
    wash_enabled: values.washEnabled,
    wash_priority: values.washPriority,
  }
  if (rules.poll_interval_minutes < 5 || rules.poll_interval_minutes > 1440) throw new Error('扫描频率必须为 5–1440 分钟')
  if (rules.max_seeders > 0 && rules.max_seeders < rules.min_seeders) throw new Error('最多做种数不能少于最少做种数')
  if (rules.max_size_gb > 0 && rules.max_size_gb < rules.min_size_gb) throw new Error('最大体积不能小于最小体积')
  return rules
}

function ruleNumber(value: string, label: string, integer = false): number {
  const parsed = Number(value)
  if (!Number.isFinite(parsed) || parsed < 0 || (integer && !Number.isInteger(parsed))) {
    throw new Error(`${label}必须为非负${integer ? '整数' : '数值'}`)
  }
  return parsed
}
