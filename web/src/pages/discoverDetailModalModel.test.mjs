import assert from 'node:assert/strict'
import test from 'node:test'

import {
  catalogItemProviderID,
  discoverItemPeople,
  discoverItemValues,
  discoverPerformerItem,
  discoverReleaseStatus,
  mergeDiscoverDetail,
  supportsAdultMovieDetail,
  supportsCatalogItemDetail,
} from './discoverDetailDataModel.ts'

test('成人作品详情只对可信 JavDB 或 FD2PPV 作品引用启用', () => {
  assert.equal(supportsAdultMovieDetail({
    title: '测试',
    media_type: 'adult',
    source: 'javdb',
    provider_id: 'QNRVYG',
    original_name: 'MIZD-534',
  }), true)
  assert.equal(supportsAdultMovieDetail({
    title: 'FC2 测试',
    media_type: 'adult',
    source: 'fd2ppv',
    provider_id: '3780016',
    original_name: 'FC2-PPV-3780016',
  }), true)
  assert.equal(supportsAdultMovieDetail({
    title: '测试',
    media_type: 'adult',
    source: 'javbus',
    provider_id: 'QNRVYG',
    original_name: 'MIZD-534',
  }), false)
})

test('普通作品详情对有明确类型和 ID 的 TMDb 与豆瓣项启用', () => {
  assert.equal(supportsCatalogItemDetail({ title: '电影', source: 'tmdb', media_type: 'movie', tmdb_id: 42 }), true)
  assert.equal(supportsCatalogItemDetail({ title: '剧集', source: 'tmdb', media_type: 'tv', tmdb_id: 43 }), true)
  assert.equal(supportsCatalogItemDetail({ title: '豆瓣作品', source: 'douban', media_type: 'movie', douban_id: '10485526' }), true)
  assert.equal(supportsCatalogItemDetail({ title: '缺少 ID', source: 'douban', media_type: 'movie' }), false)
  assert.equal(catalogItemProviderID({ title: '豆瓣作品', source: 'douban', douban_id: '10485526' }), '10485526')
  assert.equal(catalogItemProviderID({ title: 'TMDb 作品', source: 'tmdb', tmdb_id: 42 }), 42)
})

test('详情合并保留列表已有字段并补充时长片商女优', () => {
  const merged = mergeDiscoverDetail(
    { title: '列表标题', release_date: '2026-08-04', in_library: true, media_id: 'media-1' },
    {
      title: '详情标题',
      duration_minutes: 240,
      maker: 'MOODYZ',
      preview_images: ['https://img.example/sample-1.jpg', 'https://img.example/sample-2.jpg'],
      people: [{ name: '石川澪', source: 'javdb', source_id: 'QV0p9' }],
      directors: ['导演 A'],
      writers: ['编剧 A'],
      aliases: ['别名 A'],
      countries: ['日本'],
      languages: ['日语'],
    },
  )
  assert.equal(merged.title, '详情标题')
  assert.equal(merged.release_date, '2026-08-04')
  assert.equal(merged.duration_minutes, 240)
  assert.equal(merged.maker, 'MOODYZ')
  assert.deepEqual(merged.preview_images, ['https://img.example/sample-1.jpg', 'https://img.example/sample-2.jpg'])
  assert.equal(merged.in_library, true)
  assert.equal(merged.media_id, 'media-1')
  assert.equal(discoverItemPeople(merged)[0].name, '石川澪')
  assert.deepEqual(merged.directors, ['导演 A'])
  assert.deepEqual(merged.writers, ['编剧 A'])
  assert.deepEqual(merged.aliases, ['别名 A'])
  assert.deepEqual(merged.countries, ['日本'])
  assert.deepEqual(merged.languages, ['日语'])
})

test('类别和演员字段兼容接口数组与本地逗号字符串', () => {
  assert.deepEqual(discoverItemValues(['單體作品', '口交', '口交']), ['單體作品', '口交'])
  assert.deepEqual(discoverItemValues('演员 A, 演员 B'), ['演员 A', '演员 B'])
})

test('发行日期明确区分未发行和已发行', () => {
  const now = new Date(2026, 6, 18)
  assert.equal(discoverReleaseStatus('2026-08-04', now), 'upcoming')
  assert.equal(discoverReleaseStatus('2026-07-18', now), 'released')
  assert.equal(discoverReleaseStatus('未知', now), '')
})

test('女优资料可转换为现有女优详情入口', () => {
  const item = discoverPerformerItem({
    name: '石川澪',
    source: 'javdb',
    source_id: 'QV0p9',
    profile_url: 'https://javdb.com/actors/QV0p9',
  })
  assert.equal(item.media_type, 'person')
  assert.equal(item.provider_id, 'QV0p9')
  assert.equal(item.title, '石川澪')
})
