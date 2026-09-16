import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { URL } from 'node:url'
import test from 'node:test'
import * as React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import * as jsx from 'react/jsx-runtime'
import ts from 'typescript'

function loadFilterPanel() {
  const source = readFileSync(new URL('./LibraryActorFilter.tsx', import.meta.url), 'utf8')
  const code = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX },
  }).outputText
  const exports = {}
  new Function('require', 'exports', code)((name) => {
    if (name === 'react') return React
    if (name === 'react/jsx-runtime') return jsx
    if (name === 'lucide-react') {
      return Object.fromEntries(['ChevronDown', 'ChevronUp', 'Filter', 'RotateCcw', 'Search'].map((key) => [key, () => null]))
    }
    throw new Error(`Unexpected dependency ${name}`)
  }, exports)
  return exports.LibraryFilterBar
}

test('原下拉筛选整合为按钮组，且不展示演员筛选', () => {
  const LibraryFilterBar = loadFilterPanel()
  const currentYear = new Date().getFullYear()
  const html = renderToStaticMarkup(React.createElement(LibraryFilterBar, {
    values: { query: '', sort: '', category: '', genre: '', year: '', language: '', adultType: '' },
    categories: [{ name: '动画电影', count: 12 }],
    adultTypes: [{ name: 'AV', count: 8 }, { name: 'FC2', count: 4 }],
    genres: [{ name: '动画', count: 12 }],
    years: [
      { name: String(currentYear), count: 5 },
      { name: String(currentYear - 11), count: 3 },
      { name: String(currentYear - 21), count: 2 },
      { name: String(currentYear - 31), count: 1 },
      { name: String(currentYear - 50), count: 1 },
    ],
    languages: [{ name: 'zh', count: 7 }],
    onChange: () => {},
    onReset: () => {},
  }))
  assert.match(html, /动画电影/)
  assert.match(html, /AV/)
  assert.match(html, /FC2/)
  assert.match(html, /华语/)
  assert.match(html, /年代/)
  assert.match(html, /更早/)
  assert.doesNotMatch(html, /<select|演员/)
})
