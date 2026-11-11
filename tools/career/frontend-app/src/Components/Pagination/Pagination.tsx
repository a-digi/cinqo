import { pageWindow } from './pageWindow'

export interface PaginationProps {
  page: number
  pageSize: number
  total: number
  onPageChange: (page: number) => void
  // plan 250, step 2: page buttons between Prev and Next ("1 … 4 [5] 6 … 9"). Off by default, so the lists that use the plain pager do not change.
  numbered?: boolean
}

// Shared Prev/Next pager — plain page-number paging (no page-size
// picker, no jump-to-page input) since no list page in this app needs
// more than that yet. Controlled: all state (current page) lives in
// the caller, this component only renders it and reports the next
// page via onPageChange. See
// plan/ai/tools/career/step-53-jobs-page-platform-column-filter-pagination.md.
export function Pagination({ page, pageSize, total, onPageChange, numbered = false }: PaginationProps) {
  const pageCount = Math.max(1, Math.ceil(total / pageSize))
  const rangeStart = total === 0 ? 0 : (page - 1) * pageSize + 1
  const rangeEnd = Math.min(page * pageSize, total)

  return (
    <div className="mt-3 flex items-center justify-between gap-3 text-sm text-gray-500">
      <span>
        Showing {rangeStart}–{rangeEnd} of {total}
      </span>
      <div className="flex items-center gap-2">
        <button
          type="button"
          disabled={page <= 1}
          onClick={() => {
            onPageChange(page - 1)
          }}
          className="rounded-md border border-gray-300 px-2.5 py-1 text-gray-700 transition-colors hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:bg-transparent"
        >
          Prev
        </button>
        {numbered ? (
          <>
            <span className="sr-only" aria-live="polite">
              Page {page} of {pageCount}
            </span>
            <ol className="flex items-center gap-1" aria-label="Pages">
              {pageWindow(page, pageCount).map((slot, i) =>
                slot === 'gap' ? (
                  <li key={`gap-${String(i)}`} aria-hidden="true" className="px-1 text-gray-400">
                    …
                  </li>
                ) : (
                  <li key={slot}>
                    <button
                      type="button"
                      aria-label={`Page ${String(slot)}`}
                      aria-current={slot === page ? 'page' : undefined}
                      onClick={() => {
                        if (slot !== page) onPageChange(slot)
                      }}
                      className={`min-w-8 rounded-md border px-2 py-1 transition-colors ${
                        slot === page ? 'border-gray-900 bg-gray-900 text-white' : 'border-gray-300 text-gray-700 hover:bg-gray-50'
                      }`}
                    >
                      {slot}
                    </button>
                  </li>
                ),
              )}
            </ol>
          </>
        ) : (
          <span className="text-gray-400">
            Page {page} of {pageCount}
          </span>
        )}
        <button
          type="button"
          disabled={page >= pageCount}
          onClick={() => {
            onPageChange(page + 1)
          }}
          className="rounded-md border border-gray-300 px-2.5 py-1 text-gray-700 transition-colors hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:bg-transparent"
        >
          Next
        </button>
      </div>
    </div>
  )
}
