export function apiErrorMessage(err: unknown, fallback: string): string {
  const data = (err as { response?: { data?: { error?: string; message?: string } } })?.response?.data
  if (data?.error) return data.error
  if (data?.message) return data.message
  if ((err as { code?: string })?.code === 'ECONNABORTED') return '请求超时，请检查服务或网络'
  return fallback
}
