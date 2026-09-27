import assert from 'node:assert/strict'
import test from 'node:test'

import {
  splitSubtitleASRTasks,
  subtitleASRResultSummary,
  subtitleASRStageLabel,
} from './subtitleASRTaskModel.ts'

const baseTask = {
  id: 'task-1',
  owner_id: 'admin',
  media_id: 'media-1',
  source_language: 'ja',
  stage: 'queued',
  progress_current: 0,
  progress_total: 0,
  result: null,
  error: '',
  created_at: 1,
  updated_at: 1,
  started_at: 0,
  completed_at: 0,
  attempt_count: 0,
  media_available: true,
}

test('字幕生产任务按运行中和已结束分区', () => {
  const tasks = [
    { ...baseTask, id: 'queued', status: 'queued' },
    { ...baseTask, id: 'running', status: 'running' },
    { ...baseTask, id: 'completed', status: 'completed' },
    { ...baseTask, id: 'failed', status: 'failed' },
  ]

  const grouped = splitSubtitleASRTasks(tasks)

  assert.deepEqual(grouped.active.map((task) => task.id), ['queued', 'running'])
  assert.deepEqual(grouped.recent.map((task) => task.id), ['completed', 'failed'])
})

test('成功与失败任务都提供明确结果摘要', () => {
  assert.equal(subtitleASRStageLabel('transcribing'), 'SenseVoice 正在识别')
  assert.equal(subtitleASRResultSummary({
    ...baseTask,
    status: 'completed',
    stage: 'completed',
    result: { filename: 'movie.zh-CN.srt', source: 'sensevoice-qwen', language: 'zh-CN', segment_count: 42, duration: 125 },
  }), 'movie.zh-CN.srt · 42 个分段 · 2:05')
  assert.equal(subtitleASRResultSummary({
    ...baseTask,
    status: 'failed',
    stage: 'failed',
    error: 'ASR 服务不可用',
  }), 'ASR 服务不可用')
})
