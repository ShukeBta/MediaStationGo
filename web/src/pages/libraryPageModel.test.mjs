import assert from 'node:assert/strict'
import test from 'node:test'

import { seriesReplenishmentTargets } from './libraryPageModel.ts'

test('补集目标按当前季的实际目录分组，并去重同一集的多版本', () => {
  const targets = seriesReplenishmentTargets([
    { id: 's5-e1', season_num: 5, episode_num: 1, path: 'cloud://openlist/115/剧集/路西法/Season 5/S05E01.mkv' },
    { id: 'main-e2', season_num: 6, episode_num: 2, path: 'cloud://openlist/115/剧集/路西法 (2016)/Season 6/S06E02.mkv' },
    { id: 'main-e2-alt', season_num: 6, episode_num: 2, path: 'cloud://openlist/115/剧集/路西法 (2016)/Season 6/S06E02-alt.mkv' },
    { id: 'main-e3', season_num: 6, episode_num: 3, path: 'cloud://openlist/115/剧集/路西法 (2016)/Season 6/S06E03.mkv' },
    { id: 'release-e1', season_num: 6, episode_num: 1, path: 'cloud://openlist/115/剧集/Lucifer.S06.2160p/S06E01.mkv' },
    { id: 'release-e2', season_num: 6, episode_num: 2, path: 'cloud://openlist/115/剧集/Lucifer.S06.2160p/S06E02.mkv' },
  ], 6)

  assert.equal(targets.length, 2)
  const main = targets.find((target) => target.sourceLabel === '路西法 (2016)')
  const release = targets.find((target) => target.sourceLabel === 'Lucifer.S06.2160p')
  assert.equal(main?.episodeCount, 2)
  assert.equal(main?.media.id, 'main-e2')
  assert.equal(release?.episodeCount, 2)
  assert.equal(release?.media.id, 'release-e1')
})
