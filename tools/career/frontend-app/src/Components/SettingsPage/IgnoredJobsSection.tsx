import { useEffect, useState } from 'react'
import { fetchIgnoredJobs } from '../../api'
import { InfoBox } from '../../Shared/InfoBox/InfoBox'
import { ignoredDetail, ignoredEmptyText, ignoredHeadline, ignoredReason, ignoredWhen, type IgnoredList } from './crawlerRules'

function errorText(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

// Settings → Job crawler → "Kept out" (step 150): the latest jobs the rules refused, so nothing disappears silently. A job listed
// here is not remembered as deleted: the next crawl looks at it again, so a changed rule or profile can let it in.
export function IgnoredJobsSection({ ignoreEnabled }: { ignoreEnabled: boolean }) {
  const [list, setList] = useState<IgnoredList | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  function load() {
    setLoading(true)
    fetchIgnoredJobs()
      .then((l) => {
        setList(l)
        setError('')
      })
      .catch((err: unknown) => {
        setError(errorText(err))
      })
      .finally(() => {
        setLoading(false)
      })
  }

  useEffect(load, [])

  return (
    <section className="career-enter career-delay-2 mt-5 rounded-3xl border border-gray-200 p-6 shadow-sm">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">Kept out</h2>
          <p className="text-sm text-gray-500">The latest jobs the rules above kept out of the job list (the newest 200 are kept).</p>
        </div>
        <button
          type="button"
          disabled={loading}
          onClick={load}
          className="rounded-full border border-gray-300 px-4 py-1.5 text-sm font-medium text-gray-800 hover:bg-gray-50 disabled:opacity-50"
        >
          {loading ? 'Loading…' : 'Refresh'}
        </button>
      </div>

      {error !== '' && (
        <div className="mt-3">
          <InfoBox variant="error" label="Could not load" message={error} />
        </div>
      )}
      {list !== null && (
        <>
          <p className="mt-3 text-sm text-gray-700">{list.total === 0 ? ignoredEmptyText(ignoreEnabled) : ignoredHeadline(list)}</p>
          {list.items.length > 0 && (
            <ul className="mt-3 divide-y divide-gray-100">
              {list.items.map((j) => (
                <li key={j.id} className="py-3 text-sm">
                  <div className="flex flex-wrap items-baseline justify-between gap-x-3">
                    <a href={j.sourceUrl} target="_blank" rel="noreferrer" className="font-medium text-gray-900 hover:underline">
                      {j.title}
                    </a>
                    <span className="text-xs text-gray-500">{ignoredWhen(j)}</span>
                  </div>
                  {ignoredDetail(j) !== '' && <p className="text-xs text-gray-500">{ignoredDetail(j)}</p>}
                  <p className="mt-0.5 text-xs text-gray-600">{ignoredReason(j)}</p>
                </li>
              ))}
            </ul>
          )}
        </>
      )}
    </section>
  )
}
