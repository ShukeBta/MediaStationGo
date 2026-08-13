import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

test('发现模块排序由整行统一启动拖拽并覆盖手柄区域', async () => {
  const source = await readFile(new URL('./DiscoverSectionPickerModal.tsx', import.meta.url), 'utf8')

  assert.match(source, /import \{ Reorder, useDragControls \} from 'framer-motion'/)
  assert.match(source, /<Reorder\.Item[\s\S]*?dragListener=\{false\}[\s\S]*?dragControls=\{dragControls\}[\s\S]*?onPointerDown=\{\(event\) => \{\s*if \(!disabled\) dragControls\.start\(event\)\s*\}\}[\s\S]*?style=\{\{ touchAction: 'none' \}\}/)
  assert.equal(source.match(/onPointerDown=/g)?.length, 1)
  assert.match(source, /className="flex cursor-grab items-center[^"]*active:cursor-grabbing"/)
})
