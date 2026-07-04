import { Link } from 'react-router-dom'
import { ArrowRight } from 'lucide-react'

type ShortcutItem = {
  to: string
  title: string
  description: string
  badge?: string
  group?: string
}

type ManagementShortcutsProps = {
  title: string
  description?: string
  items: ShortcutItem[]
}

export function ManagementShortcuts({ title, description, items }: ManagementShortcutsProps) {
  const groups = groupShortcutItems(items)
  const hasNamedGroups = groups.some((group) => group.label)

  return (
    <section className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3 border-b border-[var(--app-border)] pb-3">
        <div>
          <h2 className="font-display text-lg font-bold text-[var(--app-text)]">{title}</h2>
          {description && <p className="mt-1 text-sm text-[var(--app-muted)]">{description}</p>}
        </div>
      </div>
      {hasNamedGroups ? (
        <div className="grid gap-5 lg:grid-cols-2">
          {groups.map((group) => (
            <div key={group.label || 'default'} className="space-y-3">
              {group.label && <h3 className="text-xs font-bold uppercase tracking-wider text-[var(--app-brand-text)]">{group.label}</h3>}
              <ShortcutGrid items={group.items} />
            </div>
          ))}
        </div>
      ) : (
        <ShortcutGrid items={items} />
      )}
    </section>
  )
}

function groupShortcutItems(items: ShortcutItem[]) {
  const groups: { label: string; items: ShortcutItem[] }[] = []
  items.forEach((item) => {
    const label = item.group ?? ''
    const group = groups.find((candidate) => candidate.label === label)
    if (group) {
      group.items.push(item)
      return
    }
    groups.push({ label, items: [item] })
  })
  return groups
}

function ShortcutGrid({ items }: { items: ShortcutItem[] }) {
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      {items.map((item) => (
        <ShortcutCard key={item.to} item={item} />
      ))}
    </div>
  )
}

function ShortcutCard({ item }: { item: ShortcutItem }) {
  return (
    <Link
      to={item.to}
      className="group rounded-lg border border-[var(--app-border)] bg-[var(--app-panel)] p-4 shadow-sm transition hover:-translate-y-0.5 hover:border-brand-500/50 hover:bg-[var(--app-panel-soft)] hover:shadow-md"
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <h3 className="truncate text-sm font-bold text-[var(--app-text)]">{item.title}</h3>
            {item.badge && (
              <span className="shrink-0 rounded-full border border-[var(--app-brand-border)] bg-[var(--app-brand-soft)] px-2 py-0.5 text-[10px] font-bold text-[var(--app-brand-text)]">
                {item.badge}
              </span>
            )}
          </div>
          <p className="mt-2 line-clamp-2 text-xs leading-5 text-[var(--app-muted)]">{item.description}</p>
        </div>
        <ArrowRight
          size={16}
          className="mt-0.5 shrink-0 text-[var(--app-brand-text)] transition group-hover:translate-x-0.5 group-hover:text-brand-500"
        />
      </div>
    </Link>
  )
}
