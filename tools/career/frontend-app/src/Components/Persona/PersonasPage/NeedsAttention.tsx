import { PAGE_ACCENT, accentFor, initialOf } from '../../JobsPage/jobsTheme'

interface Item {
  id: string
  name: string
}

// The Personas page's overview: a panel on the right that lists the personas still
// without a description, each with an Edit shortcut, under a small "N of M described"
// bar. When every persona is described it says so instead.
export function NeedsAttention({ total, undescribed, onEdit }: { total: number; undescribed: Item[]; onEdit: (id: string) => void }) {
  const described = total - undescribed.length
  const percent = total === 0 ? 0 : Math.round((described / total) * 100)
  const allDone = total > 0 && undescribed.length === 0
  return (
    <section className="career-enter career-delay-2 rounded-3xl border bg-white p-4 shadow-sm" style={{ borderColor: PAGE_ACCENT.soft }}>
      <div className="flex items-baseline justify-between">
        <h2 className="text-sm font-semibold text-gray-800">Needs attention</h2>
        <span className="text-xs text-gray-500">
          {described} of {total} described
        </span>
      </div>
      <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-gray-100" role="presentation">
        <div className="h-full rounded-full" style={{ width: `${percent}%`, background: allDone ? '#16a34a' : PAGE_ACCENT.to }} />
      </div>

      {total === 0 && <p className="mt-3 text-xs text-gray-500">Nothing to check yet.</p>}
      {allDone && (
        <p className="mt-3 flex items-center gap-2 rounded-xl bg-green-50 px-3 py-2 text-xs font-medium text-green-700">
          <span aria-hidden="true">✓</span> Everyone has a description
        </p>
      )}
      {undescribed.length > 0 && (
        <>
          <p className="mt-3 text-xs text-gray-500">A description tells you (and the AI) what each persona is for.</p>
          <ul className="mt-2 space-y-1.5">
            {undescribed.map((p) => (
              <li key={p.id} className="flex items-center gap-2 rounded-xl bg-amber-50 px-2.5 py-1.5">
                <span
                  className="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-xs font-bold text-white"
                  style={{ background: accentFor(p.name).to }}
                  aria-hidden="true"
                >
                  {initialOf(p.name)}
                </span>
                <span className="min-w-0 flex-1 truncate text-sm text-amber-900">{p.name}</span>
                <button
                  type="button"
                  onClick={() => {
                    onEdit(p.id)
                  }}
                  className="rounded-full border border-amber-300 bg-white px-2.5 py-0.5 text-xs font-medium text-amber-800 hover:bg-amber-100"
                >
                  Add one
                </button>
              </li>
            ))}
          </ul>
        </>
      )}
    </section>
  )
}
