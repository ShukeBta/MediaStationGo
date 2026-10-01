import assert from 'node:assert/strict'
import test from 'node:test'

import { changeSubscriptionFormMode, defaultSubscriptionFormValues, subscriptionFormFeed } from './subscriptionFormModel.ts'

const cloud = {
  ...defaultSubscriptionFormValues,
  deliveryMode: 'resource_import', sourceMode: 'rss',
  name: '测试剧集', filter: 'Example Show (2026)',
  feed: 'resource-import://default?alias=测试剧集&alias=Example+Show&aliases=Example+Show%7CAnother+Title',
  libraryID: 'cloud-library', libraryRootID: 'cloud-root', seasonNumber: '2',
}

test('converting a cloud subscription to PT preserves its search titles without a cloud destination or filter', () => {
  const values = changeSubscriptionFormMode(cloud, 'download', 'pt')
  const feed = new URL(subscriptionFormFeed(values))
  assert.equal(values.deliveryMode, 'download')
  assert.equal(values.sourceMode, 'pt')
  assert.equal(values.searchKeyword, 'Example Show (2026)')
  assert.equal(values.filter, '')
  assert.equal(values.libraryID, '')
  assert.equal(values.libraryRootID, '')
  assert.equal(values.seasonNumber, '2')
  assert.equal(feed.protocol, 'site-search:')
  assert.equal(feed.searchParams.get('keyword'), 'Example Show (2026)')
  assert.deepEqual(feed.searchParams.getAll('alias'), ['测试剧集', 'Example Show', 'Another Title'])
  assert.equal(cloud.filter, 'Example Show (2026)')
})

test('cloud conversion falls back to the work name and discards stale PT state', () => {
  const values = changeSubscriptionFormMode({ ...cloud, filter: '', searchKeyword: 'Stale Show', feed: 'broken' }, 'download', 'pt')
  assert.equal(values.searchKeyword, cloud.name)
  assert.equal(new URL(values.feed).searchParams.get('keyword'), cloud.name)
})

test('converting cloud to RSS does not turn its title into a regular expression or keep its feed', () => {
  const values = changeSubscriptionFormMode(cloud, 'download', 'rss')
  assert.equal(values.feed, '')
  assert.equal(values.searchKeyword, '')
  assert.equal(values.filter, '')
})

test('reselecting the active cloud mode retains its existing aliases', () => {
  const values = changeSubscriptionFormMode(cloud, 'resource_import', 'pt')
  assert.equal(values.feed, cloud.feed)
  assert.equal(values.filter, cloud.filter)
})

test('PT to cloud round trip preserves primary and alternate search titles without PT-only scope', () => {
  const pt = {
    ...defaultSubscriptionFormValues,
    name: '测试剧集', searchKeyword: 'Example Show', filter: 'Another Title', savePath: '/downloads/tv',
    feed: 'site-search://resources?keyword=Example+Show&site_id=private-site&category=7&alias=Original+Title',
  }
  const cloudValues = changeSubscriptionFormMode(pt, 'resource_import', 'pt')
  assert.equal(cloudValues.filter, 'Example Show')
  assert.equal(cloudValues.searchKeyword, '')
  assert.equal(new URL(cloudValues.feed).searchParams.get('site_id'), null)
  const restored = changeSubscriptionFormMode(cloudValues, 'download', 'pt')
  assert.equal(restored.searchKeyword, 'Example Show')
  assert.equal(restored.filter, '')
  assert.equal(restored.savePath, '/downloads/tv')
  assert.deepEqual(new URL(restored.feed).searchParams.getAll('alias'), ['测试剧集', 'Another Title', 'Original Title'])
})
