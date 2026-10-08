import assert from 'node:assert/strict'
import test from 'node:test'
import { ptDownloadInput, ptMetadataForQuery, ptSubscriptionInput, ptSubscriptionResultMessage } from './ptResourceModel.ts'
import { defaultSubscriptionFormValues, subscriptionFormFeed, subscriptionSearchKeyword } from './subscriptionFormModel.ts'

test('PT 下载保留站点及种子标识，由站点接口处理认证链接', () => {
  const payload = ptDownloadInput({ site_id: 'site-a', site_name: '站点 A', id: '42', title: 'Show S02E03', torrent_url: '/details/42', download_url: '', category: 'tv', size: 1, seeders: 2, leechers: 0, free: false }, { media_type: 'tv', poster_url: 'poster' })
  assert.equal(payload.site_id, 'site-a')
  assert.equal(payload.id, '42')
  assert.equal(payload.download_url, undefined)
  assert.equal(payload.torrent_url, '/details/42')
  assert.equal(payload.source_category, 'tv')
  assert.equal(payload.media_type, 'tv')
})

test('作品追更保留作品名称与季集范围，搜索关键词可独立调整', () => {
  const payload = ptSubscriptionInput('  Original Show ', { title: '中文剧名', original_name: 'Original Show', media_type: 'tv', year: 2026 }, '2', '12')
  assert.equal(payload.keyword, 'Original Show')
  assert.equal(payload.name, '中文剧名')
  assert.equal(payload.season_number, 2)
  assert.equal(payload.total_episodes, 12)
  assert.equal(Object.hasOwn(payload, 'poll_interval_minutes'), false)
  assert.equal(Object.hasOwn(payload, 'resolution'), false)
  const movie = ptSubscriptionInput('Film', { media_type: 'movie' }, '2', '12')
  assert.equal(movie.season_number, undefined)
  assert.equal(movie.total_episodes, undefined)
})

test('订阅已保存但首次运行失败时不展示创建失败或零结果成功', () => {
  assert.match(ptSubscriptionResultMessage({ queued: 0, run_error: '下载器不可用' }), /订阅已保存，首次执行失败：下载器不可用/)
  assert.match(ptSubscriptionResultMessage({ queued: 2 }), /2 个下载/)
  assert.match(ptSubscriptionResultMessage({ queued: 0 }), /按计划继续搜索/)
})

test('全剧总集数不能作为所选季的完成上限', () => {
  const metadata = { title: '多季剧集', media_type: 'tv', total_episodes: 100 }
  assert.equal(ptSubscriptionInput('多季剧集', metadata, '2', '').total_episodes, 0)
  assert.equal(ptSubscriptionInput('多季剧集', metadata, '2', '12').total_episodes, 12)
})

test('订阅表单默认生成 PT 关键词源，不需要用户填写协议或媒体库', () => {
  const values = { ...defaultSubscriptionFormValues, name: '剧名 & 新篇' }
  assert.equal(values.deliveryMode, 'download')
  assert.equal(values.sourceMode, 'pt')
  const feed = new URL(subscriptionFormFeed(values))
  assert.equal(feed.protocol, 'site-search:')
  assert.equal(feed.hostname, 'resources')
  assert.equal(feed.searchParams.get('keyword'), '剧名 & 新篇')
  assert.equal(feed.searchParams.has('library_id'), false)
})

test('编辑 PT 关键词保留已有站点分类和别名约束', () => {
  const original = 'site-search://resources?keyword=Old&site_id=site-b&category=tv&alias=Original&alias=Alias'
  const feed = new URL(subscriptionFormFeed({ ...defaultSubscriptionFormValues, feed: original, searchKeyword: '新关键词' }))
  assert.equal(subscriptionSearchKeyword(feed.toString()), '新关键词')
  assert.equal(feed.searchParams.get('site_id'), 'site-b')
  assert.equal(feed.searchParams.get('category'), 'tv')
  assert.deepEqual(feed.searchParams.getAll('alias'), ['Original', 'Alias'])
})

test('RSS 和原有网盘订阅地址保持原值', () => {
  for (const values of [
    { ...defaultSubscriptionFormValues, sourceMode: 'rss', feed: 'https://site.test/rss?token=example' },
    { ...defaultSubscriptionFormValues, deliveryMode: 'resource_import', feed: 'resource-import://default?alias=Title' },
  ]) assert.equal(subscriptionFormFeed(values), values.feed)
})

test('PT 下载和追更传递本地保存目录，空值由下载器默认配置接管', () => {
  const item = { site_id: 'site-a', id: '42', title: 'Test S01E01' }
  assert.equal(ptDownloadInput(item, {}, ' /downloads/local ').save_path, '/downloads/local')
  assert.equal(ptSubscriptionInput('Test', {}, '1', '', ' /downloads/local ').save_path, '/downloads/local')
  assert.equal(ptDownloadInput(item, {}, '  ').save_path, undefined)
  assert.equal(ptSubscriptionInput('Test', {}, '1', '', '').save_path, undefined)
})

test('改搜其他作品时订阅名称和别名不沿用之前打开的作品', () => {
  const metadata = { title: '原作品', original_name: 'Original', year: 2026, media_type: 'tv', poster_url: 'old-poster' }
  assert.equal(ptMetadataForQuery(' original ', 'Original 2026', metadata), metadata)
  assert.equal(ptMetadataForQuery('Original 2026', 'Original 2026', metadata), metadata)
  const active = ptMetadataForQuery('另一作品', 'Original 2026', metadata)
  const payload = ptSubscriptionInput('另一作品', active, '1', '')
  assert.equal(payload.name, '另一作品')
  assert.equal(payload.original_title, undefined)
  assert.equal(payload.poster_url, undefined)
  assert.equal(payload.year, undefined)
})
