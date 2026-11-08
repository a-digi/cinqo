import { Dropdown } from '../../../Shared/Components/Dropdown/Dropdown'
import type { SortKey } from './aiLogsRules'

const SORT_OPTIONS: { value: SortKey; label: string }[] = [
  { value: 'newest', label: 'Newest first' },
  { value: 'oldest', label: 'Oldest first' },
  { value: 'files', label: 'Most files' },
  { value: 'title', label: 'Title A–Z' },
]

// Search, sort and the result count above the table. The text and the sort live in the page.
export function AiLogsToolbar({
  query,
  onQueryChange,
  sortKey,
  onSortChange,
  matching,
  total,
}: {
  query: string
  onQueryChange: (query: string) => void
  sortKey: SortKey
  onSortChange: (key: SortKey) => void
  matching: number
  total: number
}) {
  const filtered = matching !== total
  return (
    <div className="flex flex-wrap items-center gap-3">
      <div className="relative min-w-56 flex-1 sm:max-w-sm">
        <svg
          viewBox="0 0 20 20"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.6"
          className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400"
          aria-hidden="true"
        >
          <circle cx="9" cy="9" r="5.5" />
          <path d="m13.5 13.5 3 3" strokeLinecap="round" />
        </svg>
        <input
          type="search"
          value={query}
          onChange={(e) => {
            onQueryChange(e.target.value)
          }}
          placeholder="Search title, id or folder…"
          aria-label="Search AI logs"
          className="w-full rounded-lg border border-gray-300 bg-white py-2 pl-9 pr-3 text-sm focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-200"
        />
      </div>
      <div className="w-44">
        <Dropdown options={SORT_OPTIONS} value={sortKey} onChange={onSortChange} />
      </div>
      <p className="ml-auto text-sm text-gray-500" aria-live="polite">
        {filtered ? `${matching} of ${total} conversations` : `${total} ${total === 1 ? 'conversation' : 'conversations'}`}
      </p>
    </div>
  )
}
