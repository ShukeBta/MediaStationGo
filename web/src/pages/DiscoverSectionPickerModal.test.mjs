import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

test('发现模块排序不依赖 Framer Motion 页面级拖拽锁', async () => {
  const source = await readFile(new URL('./DiscoverSectionPickerModal.tsx', import.meta.url), 'utf8')

  assert.match(source, /dragListener=\{false\}/)
  assert.match(source, /dragControls=\{dragControls\}\s+\/\/[^\n]+\s+dragPropagation\s+dragMomentum=\{false\}/)
})
