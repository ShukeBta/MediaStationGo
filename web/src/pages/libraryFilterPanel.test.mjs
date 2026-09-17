import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { URL } from 'node:url'
import test from 'node:test'
import * as React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import * as jsx from 'react/jsx-runtime'
import ts from 'typescript'

function loadFilterPanel() {
  const modelSource = readFileSync(new URL('./libraryLanguageFilterModel.ts', import.meta.url), 'utf8')
  const modelCode = ts.transpileModule(modelSource, {
    compilerOptions: { module: ts.ModuleKind.CommonJS },
  }).outputText
  const modelExports = {}
  new Function('exports', modelCode)(modelExports)

  const source = readFileSync(new URL('./LibraryActorFilter.tsx', import.meta.url), 'utf8')
  const code = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX },
  }).outputText
  const exports = {}
  new Function('require', 'exports', code)((name) => {
    if (name === 'react') return React
    if (name === 'react/jsx-runtime') return jsx
    if (name === './libraryLanguageFilterModel') return modelExports
    if (name === 'lucide-react') {
      return Object.fromEntries(['ChevronDown', 'ChevronUp', 'Filter', 'RotateCcw', 'Search'].map((key) => [key, () => null]))
    }
    throw new Error(`Unexpected dependency ${name}`)
  }, exports)
  return { ...exports, ...modelExports }
}

test('原下拉筛选整合为按钮组，且不展示演员筛选', () => {
  const { LibraryFilterBar } = loadFilterPanel()
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

test('语言筛选统一显示中文名称', () => {
  const { LibraryFilterBar, languageLabel } = loadFilterPanel()
  const codes = ['af', 'ar', 'bg', 'bo', 'cs', 'el', 'fa', 'gd', 'he', 'id', 'km', 'my', 'ta', 'uk', 'xh', 'zu']
  const html = renderToStaticMarkup(React.createElement(LibraryFilterBar, {
    values: { query: '', sort: '', category: '', genre: '', year: '', language: '', adultType: '' },
    categories: [],
    adultTypes: [],
    genres: [],
    years: [],
    languages: codes.map((name) => ({ name, count: 1 })),
    onChange: () => {},
    onReset: () => {},
  }))

  assert.equal(languageLabel('cn'), '华语')
  assert.equal(languageLabel('JP'), '日语')
  assert.match(html, /南非荷兰语/)
  assert.match(html, /阿拉伯语/)
  assert.match(html, /希腊语/)
  assert.match(html, /波斯语/)
  assert.match(html, /乌克兰语/)
  for (const code of codes) assert.doesNotMatch(html, new RegExp(`>${code.toUpperCase()}<`))
})
