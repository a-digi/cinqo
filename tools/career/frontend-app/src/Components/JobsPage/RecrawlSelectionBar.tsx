import { PAGE_ACCENT } from './jobsTheme'
import { recrawlButtonLabel, selectionText, type SelectedJob } from './recrawlRules'

// The bar above the list while jobs are selected (step 148): how many, how many can be recrawled and how many were added by hand,
// with the Recrawl button and Clear. busy is true while a recrawl is being started or is running.
export function RecrawlSelectionBar({
  selected,
  busy,
  onRecrawl,
  onClear,
}: {
  selected: readonly SelectedJob[]
  busy: boolean
  onRecrawl: () => void
  onClear: () => void
}) {
  if (selected.length === 0) return null
  const canCount = selected.filter((j) => j.canRecrawl).length
  const disabledReason = busy ? 'A recrawl is already running.' : canCount === 0 ? 'None of the selected jobs can be recrawled.' : ''
  return (
    <div
      className="mb-3 flex flex-wrap items-center justify-between gap-2 rounded-xl border px-3 py-2 text-sm"
      style={{ borderColor: PAGE_ACCENT.from, background: PAGE_ACCENT.soft, color: PAGE_ACCENT.text }}
    >
      <span className="font-medium">{selectionText(selected)}</span>
      <span className="flex items-center gap-2">
        <button
          type="button"
          disabled={disabledReason !== ''}
          title={disabledReason === '' ? 'Crawl the pages of the selected jobs again for fresh descriptions' : disabledReason}
          onClick={onRecrawl}
          className="rounded-md bg-gray-800 px-3 py-1.5 text-xs font-medium text-white hover:bg-gray-900 disabled:opacity-50"
        >
          {recrawlButtonLabel(selected.length)}
        </button>
        <button type="button" onClick={onClear} className="text-xs underline hover:opacity-80">
          Clear
        </button>
      </span>
    </div>
  )
}
