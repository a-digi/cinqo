import { DEFAULT_PAGE_SIZES, pageWindow, rangeText, totalPages } from './paginationRules'

// The pagination bar under a table (plan 259): "Showing 21–40 of 87", Previous, the page numbers, Next, and the number of rows per page. It only reads and
// reports: the page and the size live with the caller (the address of the page), so a reload, a link or the Back button keep the place.
export function Pagination({
  page,
  pageSize,
  total,
  onPageChange,
  onPageSizeChange,
  pageSizes = DEFAULT_PAGE_SIZES,
  label = 'Pagination',
}: {
  page: number
  pageSize: number
  total: number
  onPageChange: (page: number) => void
  onPageSizeChange?: (size: number) => void
  pageSizes?: readonly number[]
  label?: string
}) {
  const last = totalPages(total, pageSize)
  const base = 'inline-flex h-8 min-w-8 items-center justify-center rounded-md px-2 text-xs font-medium transition-colors'
  return (
    <nav aria-label={label} className="flex flex-wrap items-center justify-between gap-3 text-xs text-slate-600">
      <p data-testid="pagination-range" aria-live="polite">
        {rangeText(page, pageSize, total)}
      </p>
      <div className="flex flex-wrap items-center gap-3">
        {onPageSizeChange && (
          <label className="flex items-center gap-1.5">
            Rows per page
            <select
              value={pageSize}
              onChange={(e) => {
                onPageSizeChange(Number(e.target.value))
              }}
              className="rounded-md border border-slate-200 bg-white px-1.5 py-1 text-xs text-slate-800"
            >
              {pageSizes.map((s) => (
                <option key={s} value={s}>
                  {s}
                </option>
              ))}
            </select>
          </label>
        )}
        <ul className="flex items-center gap-1">
          <li>
            <button
              type="button"
              disabled={page <= 1}
              onClick={() => {
                onPageChange(page - 1)
              }}
              className={`${base} border border-slate-200 bg-white text-slate-700 hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-40`}
            >
              Previous
            </button>
          </li>
          {pageWindow(page, last).map((item, i) => (
            <li key={item === 'gap' ? `gap-${String(i)}` : item}>
              {item === 'gap' ? (
                <span aria-hidden="true" className="px-1 text-slate-400">
                  …
                </span>
              ) : (
                <button
                  type="button"
                  aria-label={`Page ${String(item)}`}
                  aria-current={item === page ? 'page' : undefined}
                  onClick={() => {
                    if (item !== page) onPageChange(item)
                  }}
                  className={`${base} ${item === page ? 'bg-slate-800 text-white' : 'border border-slate-200 bg-white text-slate-700 hover:bg-slate-50'}`}
                >
                  {item}
                </button>
              )}
            </li>
          ))}
          <li>
            <button
              type="button"
              disabled={page >= last}
              onClick={() => {
                onPageChange(page + 1)
              }}
              className={`${base} border border-slate-200 bg-white text-slate-700 hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-40`}
            >
              Next
            </button>
          </li>
        </ul>
      </div>
    </nav>
  )
}
