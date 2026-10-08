import assert from 'node:assert/strict'
import test from 'node:test'

import { defaultSubscriptionFormValues } from './subscriptionFormModel.ts'
import { applySubscriptionRuleDefaults, subscriptionRuleDefaultsInput, subscriptionRuleFormValues } from './subscriptionDefaultRulesModel.ts'

const rules = {
  poll_interval_minutes: 30, resolution: '1080p', quality: 'web-dl', effects: 'HDR',
  release_groups: 'GroupA', exclude_words: 'CAM,TS', min_seeders: 5, max_seeders: 100,
  min_size_gb: 0.5, max_size_gb: 20, free_only: true, wash_enabled: true, wash_priority: 'quality',
}

test('新建订阅套用所有默认规则，作品、季集和目录仍来自当前表单', () => {
  const draft = { ...defaultSubscriptionFormValues, name: '测试剧', searchKeyword: 'Original Show', feed: 'site-search://resources?alias=Test', libraryID: 'library', libraryRootID: 'root', savePath: '/downloads/tv', seasonNumber: '2', totalEpisodes: '12' }
  const result = applySubscriptionRuleDefaults(draft, rules)
  assert.deepEqual(subscriptionRuleDefaultsInput(result), rules)
  for (const key of ['name', 'searchKeyword', 'feed', 'libraryID', 'libraryRootID', 'savePath', 'seasonNumber', 'totalEpisodes']) assert.equal(result[key], draft[key])
  assert.equal(draft.resolution, 'best')
})

test('默认值晚到或重新保存时保留用户已编辑的 false、0 和空串', () => {
  const draft = { ...defaultSubscriptionFormValues, resolution: '2160p', quality: '', excludeWords: '', minSeeders: '0', freeOnly: false, washEnabled: false }
  const edited = new Set(['resolution', 'quality', 'excludeWords', 'minSeeders', 'freeOnly', 'washEnabled'])
  const result = applySubscriptionRuleDefaults(draft, rules, edited)
  for (const key of edited) assert.equal(result[key], draft[key])
  assert.equal(result.pollIntervalMinutes, '30')
  assert.equal(result.effects, 'HDR')
  const explicitApply = applySubscriptionRuleDefaults(result, rules)
  assert.equal(explicitApply.freeOnly, true)
  assert.equal(explicitApply.minSeeders, '5')
})

test('取消编辑后的新建表单使用当前默认值，加载失败不改写表单', () => {
  assert.deepEqual(applySubscriptionRuleDefaults(defaultSubscriptionFormValues, null), defaultSubscriptionFormValues)
  const latest = { ...rules, poll_interval_minutes: 60, resolution: '720p', free_only: false }
  const reset = applySubscriptionRuleDefaults(defaultSubscriptionFormValues, latest)
  assert.deepEqual(subscriptionRuleDefaultsInput(reset), latest)
})

test('保存默认规则保留显式关闭和无限制值，并拒绝不合法范围', () => {
  const values = subscriptionRuleFormValues(rules)
  const input = subscriptionRuleDefaultsInput({ ...values, quality: '', excludeWords: '', minSeeders: '', maxSeeders: '0', minSizeGB: '', maxSizeGB: '', freeOnly: false, washEnabled: false })
  assert.equal(input.quality, '')
  assert.equal(input.exclude_words, '')
  assert.equal(input.min_seeders, 0)
  assert.equal(input.max_seeders, 0)
  assert.equal(input.max_size_gb, 0)
  assert.equal(input.free_only, false)
  assert.equal(input.wash_enabled, false)
  for (const invalid of [
    { pollIntervalMinutes: '0' }, { pollIntervalMinutes: '1441' }, { pollIntervalMinutes: '5.5' },
    { minSeeders: '-1' }, { minSeeders: '1.5' }, { minSizeGB: 'nan' },
    { minSeeders: '10', maxSeeders: '5' }, { minSizeGB: '10', maxSizeGB: '5' },
  ]) assert.throws(() => subscriptionRuleDefaultsInput({ ...values, ...invalid }))
})
