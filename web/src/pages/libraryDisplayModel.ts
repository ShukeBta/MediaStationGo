import { cloudLibraryLabel, cloudLibraryProvider, TYPE_LABEL } from './storageConfigModel'

export function libraryDisplayPath(path: string): string {
  const provider = cloudLibraryProvider(path)
  if (!provider) return path
  const providerLabel = TYPE_LABEL[provider] || provider
  return `${providerLabel} / ${cloudLibraryLabel(path)}`
}
