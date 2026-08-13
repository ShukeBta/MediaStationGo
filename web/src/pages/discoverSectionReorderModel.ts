export function pointerReorderIndex(pointerY: number, itemCenters: number[]): number {
  if (itemCenters.length === 0) return -1
  return itemCenters.filter((center) => pointerY >= center).length
}

export function moveDiscoverSection(selected: string[], key: string, targetIndex: number): string[] {
  const sourceIndex = selected.indexOf(key)
  if (sourceIndex === -1 || targetIndex < 0) return selected

  const boundedTargetIndex = Math.min(targetIndex, selected.length - 1)
  if (sourceIndex === boundedTargetIndex) return selected

  const next = [...selected]
  const [moved] = next.splice(sourceIndex, 1)
  next.splice(boundedTargetIndex, 0, moved)
  return next
}
