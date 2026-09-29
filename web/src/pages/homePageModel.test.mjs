import test from 'node:test'
import assert from 'node:assert/strict'
import { filterRecentCards, playbackProgress, loadRandomHero } from './homePageModel.ts'

test('random hero samples the entire collection and excludes previous page', async () => {
  const calls = []
  const load = async page => { calls.push(page); return { total: 100, items: [{ key: String(page) }] } }
  assert.equal((await loadRandomHero(load, 0, () => .99)).item.key, '100')
  assert.deepEqual(calls, [1, 100])
  assert.equal((await loadRandomHero(load, 1, () => 0)).item.key, '2')
  assert.equal((await loadRandomHero(load, 100, () => .999)).item.key, '99')
})

test('random hero handles empty, single-item and concurrently shrinking collections', async () => {
  assert.deepEqual(await loadRandomHero(async () => ({ total: 0, items: [] })), { item: null, page: 0 })
  const only = { key: 'only' }
  assert.deepEqual(await loadRandomHero(async () => ({ total: 1, items: [only] }), 1), { item: only, page: 1 })
  assert.deepEqual(await loadRandomHero(async page => ({ total: 3, items: page === 1 ? [only] : [] }), 0, () => .9), { item: only, page: 1 })
  await assert.rejects(loadRandomHero(async () => { throw new Error('offline') }), /offline/)
})

test('recent filters use the display library and preserve the original order', () => {
  const libraries = [{ id: 'movies', type: 'movie' }, { id: 'shows', type: 'tv' }, { id: 'animation', type: 'anime' }]
  const cards = [
    { key: 'one', rep: { library_id: 'movies' } },
    { key: 'two', rep: { library_id: 'cloud-source', display_library_id: 'shows' } },
    { key: 'three', rep: { library_id: 'animation' } },
    { key: 'four', rep: { library_id: 'movies', display_library_id: 'unavailable' } },
  ]
  assert.equal(filterRecentCards(cards, libraries, 'all'), cards)
  assert.deepEqual(filterRecentCards(cards, libraries, 'movie').map(c => c.key), ['one', 'four'])
  assert.deepEqual(filterRecentCards(cards, libraries, 'series').map(c => c.key), ['two'])
  assert.deepEqual(filterRecentCards(cards, libraries, 'anime').map(c => c.key), ['three'])
  assert.equal(cards.length, 4)
})

test('playback progress handles missing duration and clamps stale playback positions', () => {
  assert.equal(playbackProgress(50, 200), 25)
  assert.equal(playbackProgress(300, 200), 100)
  assert.equal(playbackProgress(-10, 100), 0)
  assert.equal(playbackProgress(20, 0), 0)
  assert.equal(playbackProgress(Infinity, 100), 0)
  assert.equal(playbackProgress(20, NaN), 0)
})
