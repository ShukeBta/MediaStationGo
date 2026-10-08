import assert from 'node:assert/strict'
import test from 'node:test'

import { defaultSections, defaultSectionDefs, discoverCardMetaText, discoverCardSecondaryText, discoverSourceLabel, discoverRowsStorageKey, fd2PPVSortOptions, initialDiscoverSelection, markDiscoverDefaultsReviewed, orderSelectedSections, readCachedDiscoverRows, shouldUpgradeDiscoverDefaults, writeCachedDiscoverRow } from './discoverPageModel.ts'

const sections = [
  { key: 'first', label: '第一模块', provider: 'test' },
  { key: 'second', label: '第二模块', provider: 'test' },
  { key: 'third', label: '第三模块', provider: 'test' },
]

test('默认发现独立展示华语热门、最新、待映推荐，同时保留动漫和综艺', () => {
  assert.deepEqual(defaultSections.slice(0, 8), ['tmdb_chinese_movie', 'tmdb_chinese_latest_movie', 'tmdb_chinese_upcoming_movie', 'tmdb_chinese_tv', 'tmdb_chinese_latest_tv', 'tmdb_chinese_upcoming_tv', 'tmdb_chinese_anime', 'tmdb_chinese_variety'])
  assert.deepEqual(orderSelectedSections(defaultSections, defaultSectionDefs), defaultSections)
  assert.deepEqual(orderSelectedSections(['douban_hot_movie'], defaultSectionDefs), ['douban_hot_movie'])
  const labels = Object.fromEntries(defaultSectionDefs.map((section) => [section.key, section.label]))
  assert.equal(labels.tmdb_chinese_movie, '华语热门电影')
  assert.equal(labels.tmdb_chinese_latest_movie, '华语最新电影')
  assert.equal(labels.tmdb_chinese_upcoming_movie, '华语待上映电影')
  assert.equal(labels.tmdb_chinese_tv, '华语热门剧集')
  assert.equal(labels.tmdb_chinese_latest_tv, '华语最新剧集')
  assert.equal(labels.tmdb_chinese_upcoming_tv, '华语待播剧集')
})

const legacyDefaultSections = defaultSections.filter((key) => !key.startsWith('tmdb_chinese_'))

test('旧版自动保存的默认模块升级为华语推荐优先', () => {
  const result = initialDiscoverSelection({ configured: true, selected_sections: legacyDefaultSections }, defaultSectionDefs, true)
  assert.deepEqual(result, { selected: defaultSections, shouldSave: true })
  assert.deepEqual(initialDiscoverSelection({ configured: false, selected_sections: [] }, defaultSectionDefs, false), { selected: defaultSections, shouldSave: true })
})

test('此前只有国产电影和剧集的默认选择补上华语动漫和综艺', () => {
  const previousDefaults = ['tmdb_chinese_movie', 'tmdb_chinese_tv', ...legacyDefaultSections]
  assert.deepEqual(initialDiscoverSelection({ configured: true, selected_sections: previousDefaults }, defaultSectionDefs, true), { selected: defaultSections, shouldSave: true })
})

test('国产默认迁移不会覆盖自定义、空列表或已调整的模块顺序', () => {
  for (const selected of [['douban_hot_movie'], [], [...legacyDefaultSections].reverse(), defaultSections]) {
    assert.deepEqual(initialDiscoverSelection({ configured: true, selected_sections: selected }, defaultSectionDefs, true), { selected, shouldSave: false })
  }
  const oldServerSections = defaultSectionDefs.filter((section) => !section.key.startsWith('tmdb_chinese_'))
  assert.deepEqual(initialDiscoverSelection({ configured: true, selected_sections: legacyDefaultSections }, oldServerSections, true), { selected: legacyDefaultSections, shouldSave: false })
})

test('服务端已迁移账号不受浏览器迁移标记影响，删除新栏目不会再次开启', () => {
  for (const selected of [['tmdb_chinese_movie', 'tmdb_chinese_tv'], legacyDefaultSections, []]) {
    assert.deepEqual(initialDiscoverSelection({ configured: true, selected_sections: selected, sections_version: 1 }, defaultSectionDefs, true), { selected, shouldSave: false })
  }
})

test('迁移只执行一次并按用户区分，用户之后移除国产模块不会被重新加入', (t) => {
  mockLocalStorage(t)
  assert.equal(shouldUpgradeDiscoverDefaults('user-one'), true)
  markDiscoverDefaultsReviewed('user-one', defaultSectionDefs)
  assert.equal(shouldUpgradeDiscoverDefaults('user-one'), false)
  assert.equal(shouldUpgradeDiscoverDefaults('user-two'), true)
  assert.deepEqual(initialDiscoverSelection({ configured: true, selected_sections: legacyDefaultSections }, defaultSectionDefs, shouldUpgradeDiscoverDefaults('user-one')), { selected: legacyDefaultSections, shouldSave: false })
})

test('无法持久保存迁移状态时保留现有选择', (t) => {
  const previousWindow = globalThis.window
  globalThis.window = { localStorage: { getItem() { throw new Error('storage unavailable') } } }
  t.after(() => { globalThis.window = previousWindow })
  assert.equal(shouldUpgradeDiscoverDefaults('user-one'), false)
})

test('推荐过滤规则更新后忽略旧缓存并只读取新版本结果', (t) => {
  const storage = mockLocalStorage(t)
  storage.set(discoverRowsStorageKey, JSON.stringify({ version: 5, saved_at: Date.now(), rows: { tmdb_chinese_tv: { page: 1, has_next: true, items: [{ title: '旧版未过滤作品' }] } } }))
  assert.deepEqual(readCachedDiscoverRows(['tmdb_chinese_tv']), { rows: {}, rowCanNext: {} })
  writeCachedDiscoverRow('tmdb_chinese_tv', 1, [{ title: '正常国产剧' }], false)
  assert.deepEqual(readCachedDiscoverRows(['tmdb_chinese_tv']), { rows: { tmdb_chinese_tv: [{ title: '正常国产剧' }] }, rowCanNext: { tmdb_chinese_tv: false } })
})

test('最新待映栏目不复用跨日缓存，其他栏目的刷新不会延长其日期范围', (t) => {
  const storage = mockLocalStorage(t)
  for (const key of ['tmdb_chinese_latest_movie', 'tmdb_chinese_latest_tv', 'tmdb_chinese_upcoming_movie', 'tmdb_chinese_upcoming_tv']) {
    writeCachedDiscoverRow(key, 1, [{ title: '旧日期范围的作品' }], false)
  }
  const cached = JSON.parse(storage.get(discoverRowsStorageKey))
  for (const row of Object.values(cached.rows)) row.saved_at -= 24 * 60 * 60 * 1000
  storage.set(discoverRowsStorageKey, JSON.stringify(cached))
  writeCachedDiscoverRow('tmdb_chinese_movie', 1, [{ title: '热门电影' }], false)
  const selected = ['tmdb_chinese_movie', ...Object.keys(cached.rows)]
  assert.deepEqual(readCachedDiscoverRows(selected), { rows: { tmdb_chinese_movie: [{ title: '热门电影' }] }, rowCanNext: { tmdb_chinese_movie: false } })
  writeCachedDiscoverRow('tmdb_chinese_upcoming_movie', 1, [{ title: '待映电影' }], true)
  assert.deepEqual(readCachedDiscoverRows(['tmdb_chinese_upcoming_movie']), { rows: { tmdb_chinese_upcoming_movie: [{ title: '待映电影' }] }, rowCanNext: { tmdb_chinese_upcoming_movie: true } })
})

function mockLocalStorage(t) {
  const previousWindow = globalThis.window
  const storage = new Map()
  globalThis.window = { localStorage: { getItem: (key) => storage.get(key) ?? null, setItem: (key, value) => storage.set(key, value) } }
  t.after(() => { globalThis.window = previousWindow })
  return storage
}

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
