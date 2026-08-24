import assert from 'node:assert/strict'
import test from 'node:test'

import { selectSeasonSubtitleCandidates } from './librarySeriesSubtitleModel.ts'

const episodes = [
  { id: 'ep-1' },
  { id: 'ep-2' },
  { id: 'ep-3' },
]

function candidate(mediaID, candidateID, uploader, downloads, likes = 0) {
  return {
    media_id: mediaID,
    candidate_id: candidateID,
    uploader,
    download_count: downloads,
    like_count: likes,
    rank: 1,
  }
}

test('默认逐集选择下载量最高的字幕', () => {
  const result = selectSeasonSubtitleCandidates(episodes, [
    candidate('ep-1', 'a-low', '甲', 10),
    candidate('ep-1', 'a-high', '乙', 20),
    candidate('ep-2', 'b-high', '甲', 30),
  ], 'downloads')

  assert.deepEqual(result, {
    candidateIDs: { 'ep-1': 'a-high', 'ep-2': 'b-high' },
    uploader: '',
  })
})

test('同一上传人优先覆盖最多集，缺失集回退最高下载量', () => {
  const result = selectSeasonSubtitleCandidates(episodes, [
    candidate('ep-1', 'a-common', '统一上传人', 10),
    candidate('ep-1', 'a-high', '其他', 100),
    candidate('ep-2', 'b-common', '统一上传人', 20),
    candidate('ep-2', 'b-high', '其他二', 200),
    candidate('ep-3', 'c-high', '仅本集', 300),
  ], 'uploader')

  assert.deepEqual(result, {
    candidateIDs: {
      'ep-1': 'a-common',
      'ep-2': 'b-common',
      'ep-3': 'c-high',
    },
    uploader: '统一上传人',
  })
})
