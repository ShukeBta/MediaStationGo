// Package service — direct-play / HLS streaming.
//
// StreamService exposes two flavours of playback:
//
//   - Direct play: the original file is served with HTTP Range support.
//     Works for browser-friendly containers (mp4 / webm / m4v), no ffmpeg
//     involved, zero CPU overhead.
//   - HLS: when the client opts in (or the source codec / container is
//     not browser-friendly), the TranscoderService runs ffmpeg in the
//     background and we serve the resulting .m3u8 + .ts files directly.
//
// The HTTP layer decides which mode to use based on the request path:
//
//	GET /api/stream/:id              → direct play
//	GET /api/hls/:id/index.m3u8      → HLS playlist
//	GET /api/hls/:id/seg_NNNNN.ts    → HLS segment
package service
