import type { DanmakuImportFormat, DanmakuImportSummary } from '../api/danmaku'

// 详情页「导入本地弹幕文件」的纯逻辑：格式推断与结果说明。
// 单独放在 model 里，便于用 node --test 直接验证（组件文件只导出组件，保持 fast refresh 可用）。

/** 与后端 service.DanmakuMaxImportBytes / DANMAKU_IMPORT_MAX_BYTES 保持一致。 */
export const DANMAKU_IMPORT_MAX_BYTES = 8 * 1024 * 1024

/** 按文件扩展名推断格式；判断不出来就交给服务端按内容识别（auto）。 */
export function importFormatFromFilename(name: string): DanmakuImportFormat {
  const lower = String(name || '').toLowerCase()
  if (lower.endsWith('.xml')) return 'bilibili-xml'
  if (lower.endsWith('.json')) return 'dandanplay-json'
  return 'auto'
}

/** 导入成功后的一句话说明：过滤/丢弃/采样都必须如实说出来，不能只报成功条数。 */
export function describeImportSummary(summary: DanmakuImportSummary): string {
  const notes = [
    summary.filtered ? `屏蔽过滤 ${summary.filtered} 条` : '',
    summary.dropped_modes ? `丢弃 ${summary.dropped_modes} 条不支持的模式` : '',
    summary.skipped ? `跳过 ${summary.skipped} 条无效弹幕` : '',
    summary.truncated ? `按密度上限采样到 ${summary.count} 条` : '',
  ].filter(Boolean)
  const format = summary.format ? `（${summary.format}）` : ''
  return `已导入 ${summary.count} 条弹幕${format}${notes.length ? `：${notes.join('，')}` : ''}`
}
