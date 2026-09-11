import assert from 'node:assert/strict'
import test from 'node:test'

import {
  DANMAKU_IMPORT_MAX_BYTES,
  describeImportSummary,
  importFormatFromFilename,
} from './mediaDetailDanmakuModel.ts'

test('按扩展名推断弹幕文件格式，认不出来就交给服务端判断', () => {
  assert.equal(importFormatFromFilename('12345.xml'), 'bilibili-xml')
  assert.equal(importFormatFromFilename('DANMAKU.XML'), 'bilibili-xml')
  assert.equal(importFormatFromFilename('episode-1.json'), 'dandanplay-json')
  assert.equal(importFormatFromFilename('danmaku.JSON'), 'dandanplay-json')
  assert.equal(importFormatFromFilename('danmaku.txt'), 'auto')
  assert.equal(importFormatFromFilename(''), 'auto')
})

test('导入说明如实包含过滤、丢弃模式、跳过与采样', () => {
  assert.equal(
    describeImportSummary({
      source: 'local',
      format: 'bilibili-xml',
      count: 120,
      total: 200,
      filtered: 0,
      dropped_modes: 0,
      skipped: 0,
      truncated: false,
    }),
    '已导入 120 条弹幕（bilibili-xml）',
  )
  const message = describeImportSummary({
    source: 'local',
    format: 'dandanplay-json',
    count: 40,
    total: 200,
    filtered: 3,
    dropped_modes: 5,
    skipped: 2,
    truncated: true,
  })
  assert.match(message, /已导入 40 条弹幕（dandanplay-json）/)
  assert.match(message, /屏蔽过滤 3 条/)
  assert.match(message, /丢弃 5 条不支持的模式/)
  assert.match(message, /跳过 2 条无效弹幕/)
  assert.match(message, /按密度上限采样到 40 条/)
})

test('前端的上限与服务端的 8MiB 保持一致', () => {
  assert.equal(DANMAKU_IMPORT_MAX_BYTES, 8 * 1024 * 1024)
})
