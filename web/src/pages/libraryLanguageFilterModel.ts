const languageAliases: Record<string, string> = {
  cn: 'zh',
  'zh-cn': 'zh',
  'zh-tw': 'zh',
  'zh-hans': 'zh',
  'zh-hant': 'zh',
  jp: 'ja',
  kr: 'ko',
}

const preferredLanguageLabels: Record<string, string> = {
  zh: '华语',
  yue: '粤语',
}

type DisplayNamesConstructor = new (
  locales: string | string[],
  options: { type: 'language' },
) => { of(code: string): string | undefined }

const DisplayNames = (Intl as typeof Intl & { DisplayNames?: DisplayNamesConstructor }).DisplayNames
const chineseLanguageNames = DisplayNames ? new DisplayNames(['zh-CN'], { type: 'language' }) : undefined

export function languageLabel(value: string): string {
  const raw = value.trim()
  if (!raw || /[\u3400-\u9fff]/u.test(raw)) return raw

  const normalized = raw.toLowerCase()
  const code = languageAliases[normalized] ?? normalized
  const preferred = preferredLanguageLabels[code]
  if (preferred) return preferred

  try {
    const localized = chineseLanguageNames?.of(code)
    if (localized && localized.toLowerCase() !== code) return localized
  } catch (error) {
    if (!(error instanceof RangeError)) throw error
  }
  return `其他语言（${raw.toUpperCase()}）`
}
