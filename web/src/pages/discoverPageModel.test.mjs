import assert from 'node:assert/strict'
import test from 'node:test'

import { discoverCardMetaText, discoverCardSecondaryText, discoverSourceLabel, fd2PPVSortOptions, orderSelectedSections } from './discoverPageModel.ts'

const sections = [
  { key: 'first', label: '第一模块', provider: 'test' },
  { key: 'second', label: '第二模块', provider: 'test' },
  { key: 'third', label: '第三模块', provider: 'test' },
]

test('发现模块保留用户排序并过滤无效或重复模块', () => {
  assert.deepEqual(orderSelectedSections(['third', 'first'], sections), ['third', 'first'])
  assert.deepEqual(
    orderSelectedSections(['third', 'missing', 'first', 'third', 'second'], sections),
    ['third', 'first', 'second'],
  )
})

test('普通作品卡片补充原名或明确的来源评分', () => {
  assert.equal(discoverCardSecondaryText({ title: '中文名', original_name: 'Original Name', media_type: 'anime' }), 'Original Name')
  assert.equal(discoverCardSecondaryText({ title: '豆瓣作品', source: 'douban', media_type: 'movie', rating: 8.6 }), '豆瓣评分 8.6')
  assert.equal(discoverCardSecondaryText({ title: '成人作品', media_type: 'adult', rating: 4.8 }), '')
})

test('发现卡片优先展示完整发行日期，没有日期时保留年份', () => {
  assert.equal(discoverCardMetaText({ title: '作品', media_type: 'adult', release_date: '2026-08-04', year: 2026 }), '成人作品 · 2026-08-04')
  assert.equal(discoverCardMetaText({ title: '作品', media_type: 'movie', year: 2026 }), '电影 · 2026')
})

test('FC2 来源和五种排序条件使用面向用户的固定文案', () => {
  assert.equal(discoverSourceLabel('fd2ppv'), 'FC2')
  assert.deepEqual(
    fd2PPVSortOptions.map((option) => option.value),
    ['release', 'views', 'likes', 'favorites', 'comments'],
  )
})
