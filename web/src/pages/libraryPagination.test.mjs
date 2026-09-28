import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { URL } from 'node:url'
import test from 'node:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import * as jsx from 'react/jsx-runtime'
import ts from 'typescript'

function loadComponent(filename) {
  const source = readFileSync(new URL(filename, import.meta.url), 'utf8')
  const code = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX } }).outputText
  const exports = {}
  new Function('require', 'exports', code)((name) => {
    if (name === 'react/jsx-runtime') return jsx
    if (name === 'lucide-react') return { Film: () => null, GitMerge: () => null, Globe: () => null, WandSparkles: () => null }
    if (name === '../components/MediaCard') return { MediaCard: ({ media }) => createElement('article', { 'data-card': media.id }) }
    if (name === '../components/Pagination') return { Pagination: ({ page, totalPages }) => createElement('nav', {}, `${page}/${totalPages}`) }
    if (name === './libraryDisplayModel') return { libraryDisplayPath: (path) => path }
    throw new Error(`Unexpected dependency ${name}`)
  }, exports)
  return exports
}

test('首屏标题渲染总数177，不再出现48/177补载提示', () => {
  const { LibraryPageHeader } = loadComponent('./LibraryPageHeader.tsx')
  const html = renderToStaticMarkup(createElement(LibraryPageHeader, {
    library: { name: '剧集', type: 'tv', path: '/media/tv' }, itemCount: 177,
    scanProgress: '', isAdmin: false,
  }))
  assert.match(html, /\(177\)/)
  assert.doesNotMatch(html, /继续加载|48/)
})

test('第二页直接渲染返回的48张，不再对当前页做二次slice', () => {
  const { LibraryMediaSections } = loadComponent('./LibraryMediaSections.tsx')
  const props = {
    isSeries: true, items: [], selectedSeries: null, loading: false, page: 2, total: 177,
    seriesCards: Array.from({ length: 48 }, (_, i) => ({ key: `key${i}`, rep: { id: `media${i}` }, linkMedia: { id: `media${i}` } })),
  }
  const html = renderToStaticMarkup(createElement(LibraryMediaSections, props))
  assert.equal(html.match(/data-card=/g)?.length, 48)
  assert.match(html, />2\/4<\/nav>/)
  const loading = renderToStaticMarkup(createElement(LibraryMediaSections, { ...props, loading: true }))
  assert.match(loading, /加载当前页/)
  assert.doesNotMatch(loading, /data-card=|暂无|没有符合/)
  assert.equal(renderToStaticMarkup(createElement(LibraryMediaSections, { ...props, selectedSeries: props.seriesCards[0] })), '')
})
