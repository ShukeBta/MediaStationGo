import assert from 'node:assert/strict'
import test from 'node:test'

import { buildActorFacets, isActorFacetName, librarySupportsActorFilter } from './libraryActorFilterModel.ts'

test('演员筛选排除有码无码等分类标签', () => {
  const facets = buildActorFacets([
    { actors: '石川澪, 有码, 无码, Censored' },
    { actors: '石川澪, 七沢みあ, 有码 无码' },
  ])

  assert.deepEqual(facets, [
    { name: '七沢みあ', count: 1 },
    { name: '石川澪', count: 2 },
  ])
  assert.equal(isActorFacetName('无码女优'), false)
  assert.equal(isActorFacetName('Actor Name'), true)
})

test('仅成人媒体库启用演员筛选', () => {
  assert.equal(librarySupportsActorFilter('adult'), true)
  assert.equal(librarySupportsActorFilter('movie'), false)
  assert.equal(librarySupportsActorFilter('tv'), false)
  assert.equal(librarySupportsActorFilter('anime'), false)
  assert.equal(librarySupportsActorFilter(undefined), false)
})
