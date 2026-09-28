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
  return exports
}

test('媒体库筛选默认收起，仅展示当前筛选摘要', () => {
  const { LibraryFilterBar } = loadFilterPanel()
  const currentYear = new Date().getFullYear()
  const html = renderToStaticMarkup(React.createElement(LibraryFilterBar, {
    values: { query: '', sort: '', category: '', genre: '', year: '', adultType: '' },
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
    onChange: () => {},
    onReset: () => {},
  }))
  assert.match(html, /aria-expanded="false"/)
  assert.match(html, /全部内容/)
  assert.doesNotMatch(html, /动画电影|AV|FC2|年代|更早|<select|演员|语言|华语/)
})
