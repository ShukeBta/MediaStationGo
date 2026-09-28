import assert from 'node:assert/strict'
import { Buffer } from 'node:buffer'
import { readFileSync } from 'node:fs'
import { setImmediate } from 'node:timers'
import { URL } from 'node:url'
import test from 'node:test'
import ts from 'typescript'

const { Event, EventTarget } = globalThis

function loadModule(path, dependencies, globals = {}) {
  const source = readFileSync(new URL(path, import.meta.url), 'utf8')
  const code = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS } }).outputText
  const exports = {}
  new Function('require', 'exports', ...Object.keys(globals), code)((name) => {
    if (name in dependencies) return dependencies[name]
    throw new Error(`Unexpected dependency ${name}`)
  }, exports, ...Object.values(globals))
  return exports
}

function harness() {
  const jwt = (expires, id) => `header.${Buffer.from(JSON.stringify({ exp: expires, id })).toString('base64url')}.signature`
  const future = Math.floor(Date.now() / 1000) + 3600
  const state = { token: jwt(future, 'first'), refreshCount: 0, logout() {}, async tokenRefresh() {
    state.refreshCount++
    state.token = jwt(future, 'refreshed')
    return true
  } }
  const api = { interceptors: { request: { use() {} }, response: { use() {} } } }
  const client = loadModule('../api/client.ts', {
    axios: { default: { create: () => api } },
    '../stores/auth': { useAuthStore: { getState: () => state } },
    '../stores/playProfile': { getActivePlayProfileId: () => 'family', getActivePlayProfilePinToken: () => 'pin proof' },
  })
  let tick, cleared = false
  const document = new EventTarget()
  const player = loadModule('./hlsSession.ts', { '../api/client': client }, {
    window: { location: { origin: 'https://media.example' } }, document,
    setInterval: (fn) => { tick = fn; return 123 },
    clearInterval: (id) => { assert.equal(id, 123); cleared = true },
  })
  const expire = () => { state.token = jwt(Math.floor(Date.now() / 1000) - 1, 'expired') }
  return { player, client, state, expire, tick: () => tick(), cleared: () => cleared, document }
}

test('HLS playlist uses rotating browser cookie while preserving the selected profile', () => {
  const { client } = harness()
  const url = new URL(client.hlsURL('movie'), 'https://media.example')
  assert.equal(url.searchParams.has('token'), false)
  assert.equal(url.searchParams.get('profile_id'), 'family')
  assert.equal(url.searchParams.get('profile_pin_token'), 'pin proof')
})

test('each HLS segment refreshes expired credentials and drops stale playlist query tokens', async () => {
  const { player, state, expire } = harness()
  const requests = []
  function xhr() {
    const request = { headers: {}, open(method, url) { this.url = new URL(url); requests.push(this) }, setRequestHeader(name, value) { this.headers[name] = value } }
    return request
  }
  const raw = '/api/hls/movie/seg_00001.ts?token=original&profile_id=family&profile_pin_token=proof'
  await player.setupHlsXHR(xhr(), raw)
  const firstToken = state.token
  expire()
  await player.setupHlsXHR(xhr(), raw)
  assert.equal(state.refreshCount, 1)
  assert.equal(requests[0].headers.Authorization, `Bearer ${firstToken}`)
  assert.equal(requests[1].headers.Authorization, `Bearer ${state.token}`)
  assert.equal(requests[1].url.searchParams.has('token'), false)
  assert.equal(requests[1].url.searchParams.get('profile_id'), 'family')
  assert.equal(requests[1].url.searchParams.get('profile_pin_token'), 'proof')
  assert.equal(requests[1].withCredentials, true)
})

test('native HLS refreshes while paused and stops refreshing on teardown', async () => {
  const h = harness()
  const video = new EventTarget()
  const stop = h.player.startHlsTokenRefresh(video)
  h.expire()
  h.tick()
  await new Promise(setImmediate)
  assert.equal(h.state.refreshCount, 1)
  h.expire()
  video.dispatchEvent(new Event('play'))
  await new Promise(setImmediate)
  assert.equal(h.state.refreshCount, 2)
  stop()
  assert.equal(h.cleared(), true)
  h.expire()
  video.dispatchEvent(new Event('play'))
  h.document.dispatchEvent(new Event('visibilitychange'))
  await new Promise(setImmediate)
  assert.equal(h.state.refreshCount, 2)
})

test('HLS does not forward a fresh Bearer token to a remote playlist target', async () => {
  const { player, state, expire } = harness()
  expire()
  await assert.rejects(player.setupHlsXHR({}, 'https://other.example/segment.ts'), /unexpected HLS/)
  assert.equal(state.refreshCount, 0)
})
