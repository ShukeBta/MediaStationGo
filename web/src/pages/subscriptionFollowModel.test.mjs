import assert from 'node:assert/strict'
import test from 'node:test'

import { followedSeriesKeys } from './subscriptionFollowModel.ts'

const library = { id: 'local-library', type: 'tv' }
const media = { title: '测试剧集', original_name: 'Example Show', season_num: 2, year: 2026, path: '/media/测试剧集/Season 2/S02E01.mkv' }
const card = { key: 'series-one', rep: media, linkMedia: media, count: 1 }
const pt = {
  enabled: true, delivery_mode: 'download', name: '我的追更',
  feed_url: 'site-search://search?keyword=Example+Show', season_number: 2,
}

test('PT follow badge recognizes the resource keyword without requiring a cloud library root', () => {
  assert.deepEqual([...followedSeriesKeys(library, [card], [pt])], ['series-one'])
})

test('PT follow badge respects enabled state, season, year and whole titles', () => {
  for (const changed of [
    { enabled: false },
    { season_number: 1 },
    { year: 2020 },
    { feed_url: 'site-search://search?keyword=Example' },
    { feed_url: 'https://example.test/rss' },
  ]) {
    assert.deepEqual([...followedSeriesKeys(library, [card], [{ ...pt, ...changed }])], [], JSON.stringify(changed))
  }
})

test('PT follow badge can use a translated search alias', () => {
  const subscription = { ...pt, feed_url: 'site-search://search?keyword=Alternate&alias=Example+Show' }
  assert.deepEqual([...followedSeriesKeys(library, [card], [subscription])], ['series-one'])
})

test('changing the season tab changes the badge for that series without changing other cards', () => {
  const firstSeason = { ...media, season_num: 1 }
  const cards = [
    { ...card, rep: firstSeason, linkMedia: firstSeason },
    { ...card, key: 'second-card', rep: firstSeason, linkMedia: firstSeason },
  ]
  assert.deepEqual([...followedSeriesKeys(library, cards, [pt], { key: card.key, season: 2 })], [card.key])
  assert.deepEqual([...followedSeriesKeys(library, cards, [pt], { key: card.key, season: 1 })], [])
})

test('cloud subscriptions retain the library and root checks', () => {
  const cloud = { ...pt, name: '测试剧集', feed_url: 'resource-import://default', delivery_mode: 'resource_import', library_id: library.id, library_root_id: 'cloud-root' }
  assert.deepEqual([...followedSeriesKeys(library, [card], [cloud])], [])
  const cloudMedia = { ...media, library_root_id: 'cloud-root' }
  const cloudCard = { ...card, rep: cloudMedia, linkMedia: cloudMedia }
  assert.deepEqual([...followedSeriesKeys(library, [cloudCard], [cloud])], ['series-one'])
  assert.deepEqual([...followedSeriesKeys(library, [cloudCard], [{ ...cloud, library_id: 'other-library' }])], [])
})
