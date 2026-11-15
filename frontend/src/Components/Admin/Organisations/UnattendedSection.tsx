import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { fetchBackgroundOverview, type BackgroundOverview } from '../../../api/organisations'
import { ApiError } from '../../../api/client'
import { LoadingSpinner } from '../../../Shared/Components/Loading/LoadingSpinner'
import { approvalText, DEFAULT_PERIOD, KILL_SWITCH_TEXT, orgLabel, periodText, PERIODS, totals } from './unattendedRules'

const TOOLS_PATH = '/admin/tools'

// What the operator sees of unattended tool use: the kill switch, the tools that ask to be used without a signed-in person, and what they used per day and
// organisation. `reloadKey` changes when an organisation's switch is changed, so the numbers are read again.
export function UnattendedSection({ reloadKey }: { reloadKey: number }) {
  const [days, setDays] = useState<number>(DEFAULT_PERIOD)
  const [data, setData] = useState<BackgroundOverview | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    let current = true
    fetchBackgroundOverview(days)
      .then((o) => {
        if (!current) return
        setData(o)
        setError('')
      })
      .catch((err: unknown) => {
        if (current) setError(err instanceof ApiError ? err.message : 'Failed to load the unattended use.')
      })
    return () => {
      current = false
    }
  }, [days, reloadKey])

  const sum = data ? totals(data.usage) : { calls: 0, denied: 0 }
  return (
    <section aria-label="Unattended use" className="max-w-4xl space-y-3 rounded-lg border border-gray-200 p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-sm font-semibold text-gray-900">Unattended use of tools</h2>
        <div role="group" aria-label="Period" className="flex gap-1.5">
          {PERIODS.map((d) => (
            <button
              key={d}
              type="button"
              aria-pressed={d === days}
              className={`rounded px-2.5 py-1 text-xs font-medium ${d === days ? 'bg-gray-900 text-white' : 'border border-gray-300 text-gray-700'}`}
              onClick={() => {
                setDays(d)
              }}
            >
              {`${String(d)} days`}
            </button>
          ))}
        </div>
      </div>
      <p className="text-xs text-gray-500">
        A tool can work without a signed-in person only when an administrator approved its routes on the{' '}
        <Link to={TOOLS_PATH} className="underline">
          Tools page
        </Link>{' '}
        and the organisation&apos;s switch above is on.
      </p>
      {error !== '' && (
        <p role="alert" className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700">
          {error}
        </p>
      )}
      {data === null && error === '' && <LoadingSpinner label="Loading…" />}
      {data?.killSwitch && (
        <p role="status" className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800">
          {KILL_SWITCH_TEXT}
        </p>
      )}
      {data && (
        <>
          <div>
            <h3 className="text-xs font-semibold uppercase tracking-wide text-gray-400">Tools that ask</h3>
            {data.tools.length === 0 ? (
              <p className="mt-1 text-sm text-gray-500">No tool asks to be used without a signed-in person.</p>
            ) : (
              <ul className="mt-1 space-y-1">
                {data.tools.map((t) => (
                  <li key={t.slug} className="flex items-baseline justify-between gap-3 text-sm">
                    <span className="text-gray-900">{t.name}</span>
                    <span className="text-xs text-gray-600">{approvalText(t)}</span>
                  </li>
                ))}
              </ul>
            )}
          </div>
          <div>
            <h3 className="text-xs font-semibold uppercase tracking-wide text-gray-400">{`Calls in ${periodText(data.days)}`}</h3>
            {data.usage.length === 0 ? (
              <p className="mt-1 text-sm text-gray-500">Nothing was called without a signed-in person.</p>
            ) : (
              <table className="mt-1 w-full text-left text-sm">
                <thead>
                  <tr className="text-xs uppercase text-gray-400">
                    <th className="py-1 font-medium">Day</th>
                    <th className="py-1 font-medium">Tool</th>
                    <th className="py-1 font-medium">Organisation</th>
                    <th className="py-1 text-right font-medium">Calls</th>
                    <th className="py-1 text-right font-medium">Refused</th>
                  </tr>
                </thead>
                <tbody>
                  {data.usage.map((u) => (
                    <tr key={`${u.day}|${u.toolSlug}|${u.orgId}`} className="border-t border-gray-100">
                      <td className="py-1 tabular-nums text-gray-600">{u.day}</td>
                      <td className="py-1 text-gray-900">{u.toolName}</td>
                      <td className="py-1 text-gray-700">{orgLabel(u)}</td>
                      <td className="py-1 text-right tabular-nums">{u.calls}</td>
                      <td className="py-1 text-right tabular-nums text-gray-600">{u.denied}</td>
                    </tr>
                  ))}
                  <tr className="border-t border-gray-300 font-medium">
                    <td className="py-1" colSpan={3}>
                      Total
                    </td>
                    <td className="py-1 text-right tabular-nums">{sum.calls}</td>
                    <td className="py-1 text-right tabular-nums">{sum.denied}</td>
                  </tr>
                </tbody>
              </table>
            )}
          </div>
        </>
      )}
    </section>
  )
}
