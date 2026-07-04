export function apiErrorMessage(err: unknown, fallback: string): string {
  return (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? fallback
}

export function isHTTPURL(value: string): boolean {
  return /^https?:\/\//i.test(value)
}
