import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { URL } from 'node:url'
import test from 'node:test'
import ts from 'typescript'

// Run the actual hook with deterministic effects and deferred HTTP responses.
// No production server, cloud storage, or extra test dependency is involved.
function harness() {
  const slots = []
  const requests = []
  let cursor = 0
  let dirty = true
  let pending = []
  let result
  let getCount = 0
  let props = { id: 'library', selected: null, options: { page: 1 } }
  const changed = (old, next) => !old || old.length !== next.length || next.some((v, i) => !Object.is(v, old[i]))
  const react = {
    useState(initial) {
      const i = cursor++
      if (!(i in slots)) slots[i] = typeof initial === 'function' ? initial() : initial
      return [slots[i], (value) => {
        const next = typeof value === 'function' ? value(slots[i]) : value
        if (!Object.is(slots[i], next)) { slots[i] = next; dirty = true }
      }]
    },
    useRef(initial) {
      const i = cursor++
      if (!(i in slots)) slots[i] = { current: initial }
      return slots[i]
    },
    useCallback(callback, deps) {
      const i = cursor++
      if (changed(slots[i]?.deps, deps)) slots[i] = { deps, callback }
      return slots[i].callback
    },
    useEffect(effect, deps) {
      const i = cursor++
      if (changed(slots[i]?.deps, deps)) pending.push(() => {
        slots[i]?.cleanup?.()
        slots[i] = { deps, cleanup: effect() }
      })
    },
  }
  const api = {
    get: async (id) => { getCount++; return { id, type: 'tv' } },
    browse: (id, options, signal) => new Promise((resolve, reject) => requests.push({ id, options, signal, resolve, reject })),
    listSeriesEpisodes: async () => ({ items: [] }),
  }
  const source = readFileSync(new URL('./useLibraryData.ts', import.meta.url), 'utf8')
  const code = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
  const exports = {}
  new Function('require', 'exports', code)((name) => {
    if (name === 'react') return react
    if (name === '../api/library') return { libraryAPI: api }
    if (name === 'react-hot-toast') return { default: { error: () => {} } }
    throw new Error(`Unexpected dependency ${name}`)
  }, exports)
  const flush = async () => {
    for (let i = 0; i < 12; i++) {
      await Promise.resolve()
      if (dirty) {
        dirty = false; cursor = 0; pending = []
        result = exports.useLibraryData(props.id, props.selected, props.options)
        for (const effect of pending) effect()
      }
    }
    assert.equal(dirty, false, 'hook must settle without a render loop')
  }
  return {
    requests, flush,
    get result() { return result },
    get getCount() { return getCount },
    setOptions(options) { props = { ...props, options }; dirty = true },
    unmount() { for (const slot of slots) slot?.cleanup?.() },
  }
}

function pageResponse(page, facets = false) {
  return {
    page, page_size: 48, total: 177, is_series: true, items: [],
    series_cards: Array.from({ length: page === 4 ? 33 : 48 }, (_, i) => ({ key: `page${page}-${i}` })),
    ...(facets ? { facets: { categories: [{ name: '欧美剧', count: 177 }], actors: [], adult_types: [] } } : {}),
  }
}

test('首屏只请求48张，总数177；空闲时不自动补齐后续页', async () => {
  const h = harness()
  await h.flush()
  assert.equal(h.requests.length, 1)
  assert.equal(h.requests[0].options.page, 1)
  assert.equal(h.requests[0].options.facets, 1)
  h.requests[0].resolve(pageResponse(1, true))
  await h.flush()
  assert.equal(h.result.total, 177)
  assert.equal(h.result.seriesCards.length, 48)
  assert.equal(h.result.loadingPage, false)
  await h.flush()
  assert.equal(h.requests.length, 1)
  h.setOptions({ page: 2 })
  await h.flush()
  assert.equal(h.requests.length, 2)
  assert.equal(h.requests[1].options.facets, 0)
  assert.equal(h.result.total, 177)
  assert.equal(h.result.loadingPage, true)
  h.requests[1].resolve(pageResponse(2))
  await h.flush()
  assert.equal(h.result.page, 2)
  assert.equal(h.result.facets.categories[0].count, 177)
  h.unmount()
})

test('快速翻页取消旧请求，即使旧响应迟到也不能覆盖新页', async () => {
  const h = harness()
  await h.flush()
  h.requests[0].resolve(pageResponse(1, true))
  await h.flush()
  h.setOptions({ page: 2 }); await h.flush()
  h.setOptions({ page: 3 }); await h.flush()
  assert.equal(h.requests[1].signal.aborted, true)
  h.requests[2].resolve(pageResponse(3)); await h.flush()
  h.requests[1].resolve(pageResponse(2)); await h.flush()
  assert.equal(h.result.page, 3)
  assert.equal(h.result.seriesCards[0].key, 'page3-0')
  h.unmount()
})

test('末页修正和新入库定位同步URL后不重复请求、不出现二次过渡', async () => {
  for (const options of [{ page: 99 }, { page: 1, focus_media: 'new-media' }]) {
    const h = harness()
    h.setOptions(options); await h.flush()
    h.requests[0].resolve(pageResponse(4, true)); await h.flush()
    h.setOptions({ page: 4 }); await h.flush()
    assert.equal(h.requests.length, 1)
    assert.equal(h.result.page, 4)
    assert.equal(h.result.loadingPage, false)
    h.unmount()
  }
})

test('筛选传递到服务端；失败显式报错，刷新只发一次新请求并更新统计', async () => {
  const h = harness()
  await h.flush()
  h.requests[0].resolve(pageResponse(1, true)); await h.flush()
  h.setOptions({ page: 1, category: '日韩剧' }); await h.flush()
  assert.equal(h.requests[1].options.category, '日韩剧')
  h.requests[1].reject(new Error('offline')); await h.flush()
  assert.notEqual(h.result.error, '')
  h.result.reloadCurrentLibrary(); await h.flush()
  assert.equal(h.requests.length, 3)
  assert.equal(h.requests[2].options.facets, 1)
  assert.equal(h.getCount, 2)
  h.requests[2].resolve({ ...pageResponse(1, true), total: 0, series_cards: [] }); await h.flush()
  assert.equal(h.result.error, '')
  assert.equal(h.result.total, 0)
  h.unmount()
})
