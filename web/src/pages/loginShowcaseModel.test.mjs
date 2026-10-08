import assert from 'node:assert/strict'
import test from 'node:test'
import { loginShowcasePosters, uniqueLoginShowcaseItems } from './loginShowcaseModel.ts'

test('poster fan keeps the selected work in front and never repeats cards to fill empty slots', () => {
  for (const count of [0, 1, 2, 5, 6, 8]) {
    const items = Array.from({ length: count }, (_, i) => ({ title: `Film ${i}`, year: 2026, artwork_url: `/api/auth/showcase/artwork/${i}` }))
    for (const index of [0, 1, 10]) {
      const posters = loginShowcasePosters(items, index)
      assert.equal(posters.length, Math.min(count, 7))
      assert.equal(new Set(posters).size, posters.length)
      if (count) assert.equal(posters[Math.floor(posters.length / 2)], items[index % count])
    }
  }
})

test('episode rows do not repeat the same work, while remakes from different years remain distinct', () => {
  const rows = [
    { title: 'Example', year: 2020, artwork_url: '/poster/episode1' },
    { title: ' Example ', year: 2020, artwork_url: '/poster/episode2' },
    { title: 'Example', year: 2026, artwork_url: '/poster/remake' },
    { title: 'No poster', year: 2026, artwork_url: '' },
  ]
  assert.deepEqual(uniqueLoginShowcaseItems(rows), [rows[0], rows[2]])
})
