import assert from 'node:assert/strict'
import test from 'node:test'

import { buildCategoryFacets, mediaHasCategory } from './libraryCategoryFilterModel.ts'

test('媒体库自动分类按固定顺序统计并筛选', () => {
  const rows = [
    { auto_category: '欧美电影' },
    { auto_category: '华语电影' },
    { auto_category: '欧美电影' },
    { auto_category: '' },
  ]

  assert.deepEqual(buildCategoryFacets(rows), [
    { name: '华语电影', count: 1 },
    { name: '欧美电影', count: 2 },
  ])
  assert.equal(mediaHasCategory(rows[0], '欧美电影'), true)
  assert.equal(mediaHasCategory(rows[1], '欧美电影'), false)
})
