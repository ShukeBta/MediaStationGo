import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

import { moveDiscoverSection, pointerReorderIndex } from './discoverSectionReorderModel.ts'

test('指针越过其他行中点后才切换目标位置', () => {
  const otherItemCenters = [175, 225]

  assert.equal(pointerReorderIndex(100, otherItemCenters), 0)
  assert.equal(pointerReorderIndex(174, otherItemCenters), 0)
  assert.equal(pointerReorderIndex(175, otherItemCenters), 1)
  assert.equal(pointerReorderIndex(224, otherItemCenters), 1)
  assert.equal(pointerReorderIndex(225, otherItemCenters), 2)
  assert.equal(pointerReorderIndex(260, otherItemCenters), 2)
  assert.equal(pointerReorderIndex(100, []), -1)
})

test('原生指针拖动按目标索引移动指定模块', () => {
  const selected = ['first', 'second', 'third']

  assert.deepEqual(moveDiscoverSection(selected, 'first', 2), ['second', 'third', 'first'])
  assert.deepEqual(moveDiscoverSection(selected, 'third', 0), ['third', 'first', 'second'])
  assert.deepEqual(moveDiscoverSection(selected, 'second', 99), ['first', 'third', 'second'])
  assert.equal(moveDiscoverSection(selected, 'second', 1), selected)
  assert.equal(moveDiscoverSection(selected, 'missing', 0), selected)
  assert.equal(moveDiscoverSection(selected, 'first', -1), selected)
})

test('排序列表使用原生指针捕获并覆盖整行和手柄区域', async () => {
  const source = await readFile(new URL('./DiscoverSectionPickerModal.tsx', import.meta.url), 'utf8')

  assert.doesNotMatch(source, /framer-motion/)
  assert.match(source, /event\.currentTarget\.setPointerCapture\(event\.pointerId\)/)
  assert.match(source, /onPointerMove=/)
  assert.match(source, /onPointerCancel=/)
  assert.match(source, /onLostPointerCapture=/)
  assert.match(source, /data-discover-section-key=\{section\.key\}/)
  assert.match(source, /item !== event\.currentTarget/)
  assert.match(source, /'flex touch-none select-none items-center/)
})
