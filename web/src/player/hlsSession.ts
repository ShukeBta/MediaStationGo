import { ensureAccessToken } from '../api/client'

// Native HLS cannot customize individual requests. Keep its HttpOnly access
// cookie fresh while the player is mounted, including long pauses, and when a
// suspended tab resumes. Refresh responses update the cookie server-side.
export function startHlsTokenRefresh(video: HTMLVideoElement): () => void {
  const refresh = () => { void ensureAccessToken(90).catch(() => undefined) }
  const timer = setInterval(refresh, 15_000)
  video.addEventListener('play', refresh)
  video.addEventListener('seeking', refresh)
  document.addEventListener('visibilitychange', refresh)
  refresh()
  return () => {
    clearInterval(timer)
    video.removeEventListener('play', refresh)
    video.removeEventListener('seeking', refresh)
    document.removeEventListener('visibilitychange', refresh)
  }
}

// hls.js invokes xhrSetup for playlists and every segment. Refresh before
// opening the request, and discard tokens inherited from an older playlist.
export async function setupHlsXHR(xhr: XMLHttpRequest, raw: string): Promise<void> {
  const url = new URL(raw, window.location.origin)
  if (url.origin !== window.location.origin || !url.pathname.startsWith('/api/hls/')) {
    throw new Error('unexpected HLS request target')
  }
  const token = await ensureAccessToken(90)
  url.searchParams.delete('token')
  xhr.open('GET', url.toString(), true)
  xhr.withCredentials = true
  if (token) xhr.setRequestHeader('Authorization', `Bearer ${token}`)
}
