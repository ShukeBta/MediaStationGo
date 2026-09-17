import { ChevronDown, ChevronUp, Filter, RotateCcw, Search } from 'lucide-react'
import { type FormEvent, type ReactNode, useEffect, useMemo, useState } from 'react'

type Facet = { name: string; count: number }
type ChipOption = { name: string; label: string; title?: string }

export type LibraryFilterValues = {
  query: string
  sort: string
  category: string
  genre: string
  year: string
  adultType: string
}

type FilterKey = keyof LibraryFilterValues

const sortOptions = [
  { name: '', label: '最近入库' },
  { name: 'rating', label: '评分最高' },
  { name: 'year', label: '年份最新' },
  { name: 'title', label: '标题排序' },
]

export function LibraryFilterBar({
  values,
  categories,
  genres,
  years,
  adultTypes,
  onChange,
  onReset,
}: {
  values: LibraryFilterValues
  categories: Facet[]
  genres: Facet[]
  years: Facet[]
  adultTypes: Facet[]
  onChange: (key: FilterKey, value: string) => void
  onReset: () => void
}) {
  const [expanded, setExpanded] = useState(true)
  const [queryDraft, setQueryDraft] = useState(values.query)

  useEffect(() => setQueryDraft(values.query), [values.query])
  const yearOptions = useMemo(() => buildYearOptions(years), [years])
  const visibleYearOptions = useMemo(() => {
    if (!values.year || yearOptions.some((option) => option.name === values.year)) return yearOptions
    return [{ name: values.year, label: values.year }, ...yearOptions]
  }, [values.year, yearOptions])

  const activeSummary = useMemo(() => {
    const selected: string[] = []
    if (values.query) selected.push(`“${values.query}”`)
    if (values.sort) selected.push(sortOptions.find((option) => option.name === values.sort)?.label ?? values.sort)
    if (values.category) selected.push(values.category)
    if (values.genre) selected.push(values.genre)
    if (values.year) selected.push(visibleYearOptions.find((option) => option.name === values.year)?.label ?? values.year)
    if (values.adultType) selected.push(values.adultType)
    return selected
  }, [values, visibleYearOptions])
  const hasActiveFilters = activeSummary.length > 0

  const submitSearch = (event: FormEvent) => {
    event.preventDefault()
    onChange('query', queryDraft.trim())
  }

  return (
    <section className="rounded-2xl border border-gray-200 bg-white p-3 shadow-sm sm:p-4" aria-label="媒体库筛选">
      <div className="flex items-center gap-2">
        <button
          type="button"
          className="flex h-11 min-w-0 flex-1 items-center gap-2 rounded-xl border border-gray-200 bg-gray-50/50 px-3 text-left text-sm font-semibold text-gray-700 transition hover:border-gray-300 hover:bg-white"
          aria-expanded={expanded}
          onClick={() => setExpanded((value) => !value)}
        >
          <Filter size={16} className="shrink-0 text-brand-600" />
          <span className="shrink-0">筛选</span>
          <span className="min-w-0 flex-1 truncate text-xs font-normal text-gray-500">
            {hasActiveFilters ? activeSummary.join(' · ') : '全部内容'}
          </span>
          {expanded ? <ChevronUp size={16} className="shrink-0" /> : <ChevronDown size={16} className="shrink-0" />}
        </button>
        <button
          type="button"
          className="btn-ghost h-11 shrink-0 px-3 disabled:cursor-not-allowed disabled:opacity-40"
          disabled={!hasActiveFilters}
          title="重置全部筛选"
          onClick={onReset}
        >
          <RotateCcw size={15} />
          <span className="hidden sm:inline">重置</span>
        </button>
      </div>

      {expanded && (
        <div className="mt-3 divide-y divide-gray-200 border-t border-gray-200">
          <FilterRow label="搜索">
            <form className="flex w-full max-w-xl gap-2" onSubmit={submitSearch}>
              <label className="relative min-w-0 flex-1">
                <span className="sr-only">搜索媒体名称</span>
                <Search size={16} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
                <input
                  type="search"
                  className="input-base h-10 w-full py-2 pl-9 pr-3"
                  placeholder="搜索标题或原名，回车提交"
                  value={queryDraft}
                  onChange={(event) => setQueryDraft(event.target.value)}
                />
              </label>
              <button
                type="submit"
                className="inline-flex h-10 items-center justify-center rounded-xl bg-brand-500 px-4 py-2 text-sm font-bold text-brand-950 transition hover:bg-brand-600 active:scale-[0.98]"
              >
                搜索
              </button>
            </form>
          </FilterRow>

          <FilterRow label="排序">
            <ChipGroup value={values.sort} options={sortOptions} onChange={(value) => onChange('sort', value)} />
          </FilterRow>

          {categories.length > 0 && (
            <FilterRow label="分类">
              <FacetChips value={values.category} options={categories} onChange={(value) => onChange('category', value)} />
            </FilterRow>
          )}

          {adultTypes.length > 0 && (
            <FilterRow label="类型">
              <FacetChips value={values.adultType} options={adultTypes} onChange={(value) => onChange('adultType', value)} />
            </FilterRow>
          )}

          {genres.length > 0 && (
            <FilterRow label="风格">
              <FacetChips value={values.genre} options={genres} onChange={(value) => onChange('genre', value)} />
            </FilterRow>
          )}

          {years.length > 0 && (
            <FilterRow label="年份">
              <ChipGroup
                value={values.year}
                options={[{ name: '', label: '全部' }, ...visibleYearOptions]}
                onChange={(value) => onChange('year', value)}
              />
            </FilterRow>
          )}
        </div>
      )}
    </section>
  )
}

function FilterRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-2 py-3 sm:flex-row sm:items-start">
      <div className="w-16 shrink-0 pt-1.5 text-xs font-semibold text-gray-500">{label}</div>
      <div className="min-w-0 flex-1">{children}</div>
    </div>
  )
}

function FacetChips({
  value,
  options,
  labelFor = (name) => name,
  onChange,
}: {
  value: string
  options: Facet[]
  labelFor?: (name: string) => string
  onChange: (value: string) => void
}) {
  return (
    <ChipGroup
      value={value}
      options={[{ name: '', label: '全部' }, ...options.map((option) => ({
        name: option.name,
        label: labelFor(option.name),
        title: `${option.count} 部作品`,
      }))]}
      onChange={onChange}
    />
  )
}

function ChipGroup({
  value,
  options,
  onChange,
}: {
  value: string
  options: ChipOption[]
  onChange: (value: string) => void
}) {
  return (
    <div className="flex flex-wrap gap-1.5">
      {options.map((option) => {
        const selected = option.name === value
        return (
          <button
            key={option.name || '__all__'}
            type="button"
            aria-pressed={selected}
            title={option.title}
            className={selected
              ? 'rounded-lg border border-brand-500 bg-brand-500 px-3 py-1.5 text-xs font-semibold text-brand-950 shadow-sm'
              : 'rounded-lg border border-gray-200 bg-gray-50/70 px-3 py-1.5 text-xs font-semibold text-gray-600 transition hover:border-brand-300 hover:bg-white hover:text-gray-900'}
            onClick={() => onChange(option.name)}
          >
            {option.label}
          </button>
        )
      })}
    </div>
  )
}

function buildYearOptions(facets: Facet[]): ChipOption[] {
  const currentYear = new Date().getFullYear()
  const recentStart = currentYear - 10
  const years = facets
    .map((facet) => ({ year: Number(facet.name), count: facet.count }))
    .filter((facet) => Number.isInteger(facet.year) && facet.year > 0)
  const options: ChipOption[] = years
    .filter((facet) => facet.year >= recentStart)
    .sort((left, right) => right.year - left.year)
    .map((facet) => ({ name: String(facet.year), label: String(facet.year), title: `${facet.count} 部作品` }))

  const firstDecade = Math.floor((recentStart - 1) / 10) * 10
  for (let index = 0; index < 3; index += 1) {
    const start = firstDecade - index * 10
    const end = index === 0 ? recentStart - 1 : start + 9
    const count = years.filter((facet) => facet.year >= start && facet.year <= end).reduce((sum, facet) => sum + facet.count, 0)
    if (count > 0) {
      options.push({ name: `${start}-${end}`, label: `${start}年代`, title: `${count} 部作品` })
    }
  }

  const earlierThan = firstDecade - 20
  const earlierCount = years.filter((facet) => facet.year < earlierThan).reduce((sum, facet) => sum + facet.count, 0)
  if (earlierCount > 0) {
    options.push({ name: `before-${earlierThan}`, label: '更早', title: `${earlierCount} 部作品` })
  }
  return options
}
