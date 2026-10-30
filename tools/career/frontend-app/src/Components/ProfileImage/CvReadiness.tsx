import { PAGE_ACCENT } from '../JobsPage/jobsTheme'
import { readyCount, type ReadinessItem } from './readinessRules'

// The Photo page's overview: a checklist on the right of the photo card saying what a
// CV built from this profile still needs, each open item with a button to the page that
// fixes it. "status" is the state of the extra read of the profile's first persona.
export function CvReadiness({
  items,
  status,
  personaName,
  onFix,
}: {
  items: ReadinessItem[]
  status: 'loading' | 'ready' | 'error'
  personaName: string
  onFix: (item: ReadinessItem) => void
}) {
  const done = readyCount(items)
  return (
    <section className="career-enter career-delay-2 rounded-3xl border bg-white p-4 shadow-sm" style={{ borderColor: PAGE_ACCENT.soft }}>
      <div className="flex items-baseline justify-between">
        <h2 className="text-sm font-semibold text-gray-800">CV readiness</h2>
        {status !== 'loading' && (
          <span className="text-xs text-gray-500">
            {done} of {items.length} ready
          </span>
        )}
      </div>
      {status === 'loading' && <p className="mt-3 animate-pulse text-xs text-gray-400">Checking…</p>}
      {status !== 'loading' && (
        <>
          <ul className="mt-3 space-y-2.5">
            {items.map((item) => (
              <li key={item.key} className="flex items-start gap-2.5">
                <span
                  aria-hidden="true"
                  className={`mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full border-2 text-[11px] font-bold ${
                    item.done ? 'border-green-600 bg-green-600 text-white' : 'border-gray-300 bg-white text-transparent'
                  }`}
                >
                  ✓
                </span>
                <div className="min-w-0 flex-1">
                  <div className={`text-sm ${item.done ? 'text-gray-600' : 'font-medium text-gray-900'}`}>{item.label}</div>
                  <div className="text-xs text-gray-500">{item.note}</div>
                </div>
                {!item.done && item.fix !== null && (
                  <button
                    type="button"
                    onClick={() => {
                      onFix(item)
                    }}
                    className="shrink-0 rounded-full border border-blue-200 px-2.5 py-0.5 text-xs text-blue-700 hover:bg-blue-50"
                  >
                    Fix
                  </button>
                )}
              </li>
            ))}
          </ul>
          {personaName !== '' && <p className="mt-3 text-xs text-gray-400">Checked on the persona “{personaName}”.</p>}
          {status === 'error' && <p className="mt-3 text-xs text-amber-700">Could not read the persona, so its items are unchecked.</p>}
        </>
      )}
    </section>
  )
}
