import assert from 'node:assert/strict'
import test from 'node:test'

import {
  subscriptionImportResultLabel,
  subscriptionImportTimeLabel,
  subscriptionSeriesDetailHref,
} from './subscriptionPageModel.ts'

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

test('自动入库明细将底层状态转换为业务文案', () => {
  assert.equal(subscriptionImportResultLabel('imported', 'completed'), '已入库')
  assert.equal(subscriptionImportResultLabel('', 'queued'), '排队中')
  assert.equal(subscriptionImportResultLabel('', 'completed_with_warning'), '已完成（有告警）')
  assert.equal(subscriptionImportResultLabel('', 'unexpected_status'), '状态未知')
})

test('自动入库明细只使用任务最终时间作为入库时间', () => {
  const finishedAt = '2026-09-12T07:15:23Z'
  assert.equal(subscriptionImportTimeLabel({
    id: 'job-1',
    attempt: 1,
    outcome: 'imported',
    status: 'completed',
    created_at: '2026-09-12T06:00:00Z',
    updated_at: '2026-09-12T08:00:00Z',
    finished_at: finishedAt,
  }), `入库时间：${new Date(finishedAt).toLocaleString()}`)

  assert.equal(subscriptionImportTimeLabel({
    id: 'job-2',
    attempt: 1,
    outcome: 'imported',
    status: 'completed',
    created_at: '2026-09-12T06:00:00Z',
    updated_at: '2026-09-12T08:00:00Z',
  }), '入库时间：记录缺失')
})
