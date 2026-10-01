import assert from 'node:assert/strict'
import test from 'node:test'

import { libraryFollowSubscriptionDraft } from './libraryFollowSubscriptionModel.ts'
import { subscriptionFormFeed } from './subscriptionFormModel.ts'

const input = {
  title: '测试剧集',
  feed: 'site-search://search?keyword=测试剧集&alias=Example+Show&alias=Example+Show+2026',
  mediaType: 'tv',
  selectedSeason: 2,
  representativeSeason: 1,
  episodes: [{ season_num: 1 }, { season_num: 2 }],
}

test('library follow-up opens PT downloads for the selected season and retains alternate search titles', () => {
  const draft = libraryFollowSubscriptionDraft(input)
  const feed = new URL(subscriptionFormFeed(draft))
  assert.equal(draft.deliveryMode, 'download')
  assert.equal(draft.sourceMode, 'pt')
  assert.equal(draft.name, '测试剧集')
  assert.equal(draft.seasonNumber, '2')
  assert.equal(draft.totalEpisodes, '')
  assert.equal(feed.protocol, 'site-search:')
  assert.equal(feed.searchParams.get('keyword'), '测试剧集')
  assert.deepEqual(feed.searchParams.getAll('alias'), ['Example Show', 'Example Show 2026'])
  assert.equal(draft.libraryID, '')
  assert.equal(draft.libraryRootID, '')
  assert.equal(draft.savePath, '')
})

test('unscraped library media can start a local subscription without a root or episode number', () => {
  const draft = libraryFollowSubscriptionDraft({ ...input, selectedSeason: null, representativeSeason: 0, episodes: [] })
  assert.equal(draft.deliveryMode, 'download')
  assert.equal(draft.seasonNumber, '1')
  assert.equal(draft.libraryRootID, '')
})

test('a known season is retained when no season tab is selected', () => {
  const draft = libraryFollowSubscriptionDraft({ ...input, selectedSeason: null, representativeSeason: 0, episodes: [{ season_num: 3 }] })
  assert.equal(draft.seasonNumber, '3')
})
