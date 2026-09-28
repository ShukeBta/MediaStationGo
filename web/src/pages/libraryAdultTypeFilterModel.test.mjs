import assert from 'node:assert/strict'
import test from 'node:test'

import { buildAdultTypeFacets, mediaHasAdultType } from './libraryAdultTypeFilterModel.ts'

test('成人库按固定顺序统计 AV 与 FC2 并支持筛选', () => {
  const rows = [
    { adult_type: 'FC2' },
    { adult_type: 'AV' },
    { adult_type: 'av' },
    {},
  ]

  assert.deepEqual(buildAdultTypeFacets(rows), [
    { name: 'AV', count: 2 },
    { name: 'FC2', count: 1 },
  ])
  assert.equal(mediaHasAdultType(rows[0], 'FC2'), true)
  assert.equal(mediaHasAdultType(rows[1], 'FC2'), false)
  assert.equal(mediaHasAdultType(rows[1], ''), true)
})
