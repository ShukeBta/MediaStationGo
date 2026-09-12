export const AI_PROTOCOLS = [
  { value: 'openai', label: 'OpenAI Chat Completions（兼容 DeepSeek / Qwen）', base: 'https://api.openai.com/v1' },
  { value: 'responses', label: 'OpenAI Responses', base: 'https://api.openai.com/v1' },
  { value: 'anthropic', label: 'Anthropic Messages', base: 'https://api.anthropic.com/v1' },
  { value: 'gemini', label: 'Gemini GenerateContent', base: 'https://generativelanguage.googleapis.com/v1beta' },
  { value: 'ollama', label: 'Ollama 本地接口', base: 'http://localhost:11434' },
]

export function isAIProvider(provider: string): boolean {
  return ['openai', 'deepseek', 'qwen', 'anthropic', 'gemini', 'ollama'].includes(provider)
}

export function readAIOptions(extra: string | undefined, provider = 'openai'): Record<string, unknown> & { protocol: string; model: string } {
  let options: Record<string, unknown> = {}
  try {
    const parsed: unknown = JSON.parse(extra || '{}')
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) options = parsed as Record<string, unknown>
  } catch { /* Older configurations may not have AI options. */ }
  const source = typeof options.provider === 'string' ? options.provider : provider
  const fallback = ['anthropic', 'gemini', 'ollama'].includes(source) ? source : 'openai'
  return {
    ...options,
    protocol: typeof options.protocol === 'string' ? options.protocol : fallback,
    model: typeof options.model === 'string' ? options.model : '',
  }
}

export function aiUsesKeylessLocalService(provider: string, extra: string | undefined): boolean {
  return isAIProvider(provider) && readAIOptions(extra, provider).protocol === 'ollama'
}
