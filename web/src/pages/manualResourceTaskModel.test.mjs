import assert from 'node:assert/strict'
import test from 'node:test'

import {
  manualResourcePreviewSelection,
  manualResourceTypeLabel,
} from './manualResourceTaskModel.ts'

test('手动任务预览只接受一个候选和一个可用目录', () => {
  const selection = manualResourcePreviewSelection({
    results: [{ index: 0, title: 'Sintel', resource_type: 'magnet' }],
    roots: [{ id: 'root-a', enabled: true }],
  })
  assert.equal(selection.candidate.title, 'Sintel')
  assert.equal(selection.root.id, 'root-a')

  assert.throws(
    () => manualResourcePreviewSelection({ results: [], roots: [{ id: 'root-a' }] }),
    /解析结果无效/,
  )
  assert.throws(
    () => manualResourcePreviewSelection({
      results: [{ index: 0, title: 'Sintel' }],
      roots: [{ id: 'root-a' }, { id: 'root-b' }],
    }),
    /只能有一个/,
  )
})

test('手动任务类型使用明确中文标签', () => {
  assert.equal(manualResourceTypeLabel({ index: 0, title: 'A', resource_type: '115_share' }), '115 分享')
  assert.equal(manualResourceTypeLabel({ index: 0, title: 'B', resource_type: 'magnet' }), '磁链')
})
