import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

test('发现模块排序同时支持手柄与整行原生拖拽', async () => {
  const source = await readFile(new URL('./DiscoverSectionPickerModal.tsx', import.meta.url), 'utf8')

  assert.match(source, /import \{ Reorder, useDragControls \} from 'framer-motion'/)
  assert.match(source, /<Reorder\.Item[\s\S]*?dragListener=\{!disabled\}[\s\S]*?dragControls=\{dragControls\}[\s\S]*?dragPropagation\s+dragMomentum=\{false\}/)
  assert.match(source, /onPointerDown=\{\(event\) => \{\s*if \(!disabled\) dragControls\.start\(event\)\s*\}\}/)
  assert.doesNotMatch(source, /dragListener=\{false\}/)
})
