import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

test('发现模块排序使用 Reorder 原生拖拽并避开页面级锁', async () => {
  const source = await readFile(new URL('./DiscoverSectionPickerModal.tsx', import.meta.url), 'utf8')

  assert.match(source, /<Reorder\.Item[\s\S]*?dragListener=\{!disabled\}[\s\S]*?dragPropagation\s+dragMomentum=\{false\}/)
  assert.doesNotMatch(source, /dragListener=\{false\}/)
  assert.doesNotMatch(source, /useDragControls|dragControls\.start/)
})
