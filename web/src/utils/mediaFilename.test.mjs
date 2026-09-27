import assert from 'node:assert/strict'
import test from 'node:test'

import { compareMediaFilename, mediaFilename } from './mediaFilename.ts'

test('普通用户响应缺少存储路径时使用可见标题作为片源名称', () => {
  assert.equal(mediaFilename({ id: 'version-1', title: 'MOC-012' }), 'MOC-012')
})

test('无路径的多版本作品仍可稳定排序', () => {
  const versions = [
    { id: 'version-2', title: 'MOC-012 版本 2' },
    { id: 'version-1', title: 'MOC-012 版本 1' },
  ]

  versions.sort(compareMediaFilename)

  assert.deepEqual(versions.map((version) => version.id), ['version-1', 'version-2'])
})

test('管理员响应包含路径时仍优先显示文件名', () => {
  assert.equal(
    mediaFilename({ id: 'version-1', title: 'MOC-012', path: 'cloud:\\adult\\MOC-012.2160p.mkv' }),
    'MOC-012.2160p.mkv',
  )
})
