import { useEffect, useState } from 'react'
import { fetchJobPersonaMatches, recalculateJobPersonaMatches, type JobPersonaMatches } from '../../api'
import { AppLink } from '../../Shared/AppLink/AppLink'
import { scoreChipClasses } from '../../Shared/MatchScoreBar/matchRules'
import { viewPersonaPathFor } from '../../paths'
import { UserIcon } from '../../Shared/Icons/icons'
import { InfoCard } from './InfoCard'

// The "Best persona" card on a job's page (step 141): which persona fits the job best, with its score, and how the other
// personas of every profile score, best first. The scores are made by themselves when a job is found; "Recalculate" scores
// the job again now (after a persona was changed, say). A persona with nothing to compare (no skills and no title) has no
// score, which is not the same as 0%. See plan/ai/tools/career/step-141-persona-job-match.md.
// reloadKey is bumped by the page when the job's description was refreshed (step 148): the scores are made again from the new text, so
// the card asks for them again, keeping what it shows until the new ones arrive.
export function PersonaMatchesCard({ jobId, reloadKey = 0 }: { jobId: string; reloadKey?: number }) {
  const [data, setData] = useState<JobPersonaMatches | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    setData((previous) => (reloadKey === 0 ? null : previous))
    setError('')
    fetchJobPersonaMatches(jobId)
      .then(setData)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }, [jobId, reloadKey])

  function recalculate() {
    setError('')
    setBusy(true)
    recalculateJobPersonaMatches(jobId)
      .then(setData)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setBusy(false)
      })
  }

  const best = data?.matches.find((m) => m.personaId === data.best)
  const others = data?.matches.filter((m) => m.personaId !== data.best) ?? []
  return (
    <InfoCard look="persona" icon={<UserIcon className="h-4 w-4" />} title="Best target role">
      {error !== '' && (
        <p role="alert" className="mb-3 text-xs text-red-700">
          {error}
        </p>
      )}
      {data === null && error === '' && <p className="text-xs text-gray-400">Loading…</p>}
      {data !== null && best === undefined && (
        <p className="text-xs text-gray-500">
          No target role has a score for this job yet. Scores are made when a job is found; “Recalculate” makes them now. A target role with
          no skills and no title to compare has none.
        </p>
      )}
      {best !== undefined && (
        <div className="rounded-xl border border-white bg-white/70 p-3">
          <p className="text-[11px] font-medium uppercase tracking-wide text-gray-400">Fits this job best</p>
          <div className="mt-1 flex items-center justify-between gap-2">
            <div className="min-w-0">
              <AppLink
                href={viewPersonaPathFor(best.personaId)}
                className="block break-words text-sm font-semibold text-blue-700 hover:underline"
              >
                {best.personaName}
              </AppLink>
              <p className="break-words text-xs text-gray-500">{best.profileName}</p>
            </div>
            <span className={`shrink-0 rounded-full px-2.5 py-1 text-sm font-semibold ${scoreChipClasses(best.score)}`}>{best.score}%</span>
          </div>
        </div>
      )}
      {others.length > 0 && (
        <ul aria-label="Other target roles" className="mt-3 space-y-1.5">
          {others.map((m) => (
            <li key={m.personaId} className="flex items-start justify-between gap-2 text-xs">
              <span className="min-w-0 break-words text-gray-600">
                <AppLink href={viewPersonaPathFor(m.personaId)} className="block hover:underline">
                  {m.personaName}
                </AppLink>
                <span className="block text-[11px] text-gray-400">{m.profileName}</span>
              </span>
              <span className={`shrink-0 rounded-full px-1.5 py-0.5 font-semibold ${scoreChipClasses(m.score)}`}>{m.score}%</span>
            </li>
          ))}
        </ul>
      )}
      <button
        type="button"
        disabled={busy}
        onClick={recalculate}
        className="mt-4 rounded-md border border-gray-300 bg-white/70 px-3 py-1.5 text-xs font-medium text-gray-700 transition-colors hover:bg-white disabled:opacity-50"
      >
        {busy ? 'Scoring…' : 'Recalculate'}
      </button>
    </InfoCard>
  )
}
