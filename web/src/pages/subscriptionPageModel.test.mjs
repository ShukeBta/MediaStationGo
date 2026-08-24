import assert from 'node:assert/strict'
import test from 'node:test'

import { subscriptionSeriesDetailHref } from './subscriptionPageModel.ts'

test('订阅详情链接使用后端权威 series key', () => {
  const subscription = {
    library_id: 'e1333358-17ff-4b90-82f0-663cec26c0df',
    series_key: 'series:4790edb7',
    media: {
      library_id: 'e1333358-17ff-4b90-82f0-663cec26c0df',
      path: 'cloud://openlist/115/动漫/吞噬星空 (2020) [tmdbid-101172]/Season 1/HDR/吞噬星空.S01E01.mkv',
      title: '吞噬星空',
      season_num: 1,
      episode_num: 1,
    },
  }

  assert.equal(
    subscriptionSeriesDetailHref(subscription),
    '/library/e1333358-17ff-4b90-82f0-663cec26c0df?series=series%3A4790edb7',
  )
})

test('没有权威 series key 时不根据媒体路径猜算链接', () => {
  assert.equal(subscriptionSeriesDetailHref({
    library_id: 'anime-library',
    media: {
      library_id: 'anime-library',
      path: '/动漫/Example/Season 1/HDR/Example.S01E01.mkv',
      title: 'Example',
      season_num: 1,
      episode_num: 1,
    },
  }), '')
})
