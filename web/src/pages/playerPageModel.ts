import type { Media } from '../types'

export type PlayerMode = 'direct' | 'hls'

const directContainers = ['mp4', 'webm', 'm4v']
const directVideoCodecs = ['h264', 'avc', 'avc1']
const directAudioCodecs = ['aac', 'mp3', 'opus']

export function pickPlayerMode(media: Media): PlayerMode {
  return needsTranscodeForBrowser(media) ? 'hls' : 'direct'
}

export function needsTranscodeForBrowser(media: Media): boolean {
  const container = (media.container ?? '').toLowerCase()
  const videoCodec = (media.video_codec ?? '').toLowerCase()
  const audioCodec = (media.audio_codec ?? '').toLowerCase()
  const containerOK = directContainers.some((item) => container.includes(item))
  const videoOK = !videoCodec || directVideoCodecs.some((item) => videoCodec.includes(item))
  const audioOK = !audioCodec || directAudioCodecs.some((item) => audioCodec.includes(item))
  return !(containerOK && videoOK && audioOK)
}
