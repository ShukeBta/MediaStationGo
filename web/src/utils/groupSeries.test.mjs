import assert from 'node:assert/strict'
import test from 'node:test'

import { groupSeries, seriesTitleFromPath } from './groupSeries.ts'

const seriesDirectory =
  'cloud://openlist/115/动漫/[Maho.sub&VCB-Studio] Aki Sora Yume no Naka [Hi10p_1080p]'

assert.equal(
  seriesTitleFromPath('cloud://openlist/115/动漫/吞噬星空 (2020) [tmdbid-101172]/Season 1/HDR/Tunshi Xingkong - 150 (2160p HQ).mkv'),
  '吞噬星空',
)

test('SPs 特典目录与正片目录聚合为同一部剧', () => {
  const episodePath = `${seriesDirectory}/[Maho.sub&VCB-Studio] Aki Sora Yume no Naka [01][Hi10p_1080p][x264_flac].mkv`
  const specialPath = `${seriesDirectory}/SPs/[Maho.sub&VCB-Studio] Aki Sora Yume no Naka [NCED][Hi10p_1080p][x264_flac].mkv`

  assert.notEqual(seriesTitleFromPath(episodePath), '')
  assert.equal(seriesTitleFromPath(specialPath), seriesTitleFromPath(episodePath))

  const cards = groupSeries([
    {
      id: 'episode-1',
      library_id: 'anime-library',
      title: 'Aki Sora OVA',
      path: episodePath,
      season_num: 1,
      episode_num: 1,
      tmdb_id: 1281913,
    },
    {
      id: 'special-nced',
      library_id: 'anime-library',
      title: 'Aki Sora OVA',
      path: specialPath,
      season_num: 1,
      episode_num: 2,
      tmdb_id: 1281913,
    },
  ])

  assert.equal(cards.length, 1)
  assert.equal(cards[0]?.count, 2)
})

test('已匹配的不同季目录按刮削后的整剧标题合并', () => {
  const seasonTwoPath =
    'cloud://openlist/115/剧集/模范出租车2 [import-29430b23083f]/' +
    '【高清剧集网 www.BTHDTV.com】模范出租车2[全16集][简繁字幕].Taxi.Driver.S02.1080p.SBS.WEB-DL.AAC2.0.H.264-BlackTV/' +
    'Taxi.Driver.S02E01.1080p.SBS.WEB-DL.AAC2.0.H.264-BlackTV.mkv'
  const seasonThreePath =
    'cloud://openlist/115/剧集/模范出租车3/Taxi.Driver.S03E01.1080p.WEB-DL.AAC2.0.H.264-BlackTV.mkv'

  assert.notEqual(seriesTitleFromPath(seasonTwoPath), seriesTitleFromPath(seasonThreePath))

  const cards = groupSeries([
    {
      id: 'taxi-driver-s02e01',
      library_id: 'tv-library',
      title: '模范出租车',
      original_name: '모범택시',
      path: seasonTwoPath,
      season_num: 2,
      episode_num: 1,
      tmdb_id: 119769,
      scrape_status: 'matched',
    },
    {
      id: 'taxi-driver-s03e01',
      library_id: 'tv-library',
      title: '模范出租车',
      original_name: '모범택시',
      path: seasonThreePath,
      season_num: 3,
      episode_num: 1,
      tmdb_id: 119769,
      scrape_status: 'matched',
    },
  ])

  assert.equal(cards.length, 1)
  assert.equal(cards[0]?.count, 2)

  const pendingCards = groupSeries([
    {
      id: 'taxi-driver-pending-s02e01',
      library_id: 'tv-library',
      title: '模范出租车',
      path: seasonTwoPath,
      season_num: 2,
      episode_num: 1,
      tmdb_id: 119769,
      scrape_status: 'pending',
    },
    {
      id: 'taxi-driver-pending-s03e01',
      library_id: 'tv-library',
      title: '模范出租车',
      path: seasonThreePath,
      season_num: 3,
      episode_num: 1,
      tmdb_id: 119769,
      scrape_status: 'pending',
    },
  ])
  assert.equal(pendingCards.length, 2)
})

test('OpenList 单集发布包目录不会把同一部剧拆成多张卡片', () => {
  const flatReleaseFiles = [
    'Alien - Earth (2025) - S01E01 - Neverland [DSNP WEBDL-1080p][EAC3 5.1][h264]-Kitsune.mkv',
    'Alien - Earth (2025) - S01E02 - Mr. October [DSNP WEBDL-1080p][EAC3 5.1][h264]-FLUX.mkv',
  ]
  const releaseHosts = ['TTHDTT', 'TTHDTT', 'BTHDTV', 'BBEGGE', 'TTHDTT', 'BBHDTV']
  const paths = [
    ...flatReleaseFiles.map((file) => `cloud://openlist/115/剧集/${file}/${file}`),
    ...releaseHosts.map((host, index) => {
      const episode = index + 3
      const episodeCode = String(episode).padStart(2, '0')
      const directory =
        `【高清剧集网发布 www.${host}.com】异形：地球.第一季[第${episodeCode}集][简繁英字幕].` +
        'Alien.Earth.S01.2025.2160p.DSNP.WEB-DL.DDP5.1.HDR.H.265-ColorTV'
      const file =
        `Alien.Earth.S01E${episodeCode}.2025.2160p.DSNP.WEB-DL.DDP5.1.HDR.H.265-ColorTV.mkv`
      return `cloud://openlist/115/剧集/${directory}/${file}`
    }),
  ]

  for (const path of paths) assert.equal(seriesTitleFromPath(path), '')

  const cards = groupSeries(
    paths.map((path, index) => ({
      id: `alien-earth-${index + 1}`,
      library_id: 'tv-library',
      title: '异形：地球',
      original_name: 'Alien: Earth',
      path,
      season_num: 1,
      episode_num: index + 1,
      tmdb_id: 157239,
    })),
  )

  assert.equal(cards.length, 1)
  assert.equal(cards[0]?.count, 8)
})

test('可清洗的 Season 0 特殊集目录继续归入正片剧名', () => {
  const cards = groupSeries([
    {
      id: 'example-main',
      library_id: 'tv-library',
      title: 'Example Show',
      path: 'cloud://openlist/115/剧集/Example Show/Season 01/Example.Show.S01E01.mkv',
      season_num: 1,
      episode_num: 1,
      tmdb_id: 1001,
    },
    {
      id: 'example-special',
      library_id: 'tv-library',
      title: 'Example Show',
      path: 'cloud://openlist/115/剧集/Example Show S00E01/Example.Show.S00E01.mkv',
      season_num: 0,
      episode_num: 1,
      tmdb_id: 1002,
    },
  ])

  assert.equal(cards.length, 1)
  assert.equal(cards[0]?.count, 2)
})
