import type { ManualMatchResult } from '../../api'
import { confidenceText, dealbreakerText, modeText, pointsShare, pointsText, relevanceText, tierLabel } from './matchRules'

// A small chip saying which engine made a match: "AI" or "Manual" (nothing for a job without one).
export function EngineBadge({ label }: { label: string }) {
  if (label === '') return null
  const manual = label === 'Manual'
  return (
    <span
      title={manual ? 'Made by the manual engine, without AI' : 'Made by an AI conversation'}
      className={`rounded-full px-2 py-0.5 text-[10px] font-semibold ${manual ? 'bg-teal-50 text-teal-700' : 'bg-violet-50 text-violet-700'}`}
    >
      {label}
    </span>
  )
}

// "How this score was made" (step 129): the best-fit persona, each component's points (or why it was not
// available), how much evidence the score rests on, and what the persona is missing.
export function MatchBreakdown({ result }: { result: ManualMatchResult }) {
  return (
    <div className="space-y-3 text-xs text-gray-600">
      <p>
        Best fit: <span className="font-medium text-gray-800">{result.personaName}</span>. {modeText(result)}.
      </p>
      <ul className="space-y-1.5">
        {result.components.map((c) => (
          <li key={c.key} title={c.note}>
            <div className="flex items-baseline justify-between gap-2">
              <span className="font-medium text-gray-700">{c.label}</span>
              <span className={c.evaluable ? 'text-gray-700' : 'text-gray-400'}>{pointsText(c)}</span>
            </div>
            <div className="mt-0.5 h-1 overflow-hidden rounded-full bg-gray-100">
              <div className="h-full rounded-full bg-blue-400" style={{ width: `${String(pointsShare(c) * 100)}%` }} />
            </div>
            <p className="mt-0.5 text-[11px] text-gray-400">{c.note}</p>
          </li>
        ))}
      </ul>
      <p className={result.lowConfidence ? 'font-medium text-amber-700' : 'text-gray-500'}>
        {confidenceText(result.coverage, result.lowConfidence)}.
      </p>
      {relevanceText(result.relevanceScale) !== '' && <p className="font-medium text-amber-700">{relevanceText(result.relevanceScale)}</p>}
      {dealbreakerText(result.dealbreakerScale) !== '' && (
        <p className="font-medium text-amber-700">{dealbreakerText(result.dealbreakerScale)}</p>
      )}
      {result.missing.length > 0 && (
        <div>
          <p className="mb-1 font-medium text-gray-700">Missing requirements</p>
          <ul className="space-y-1">
            {result.missing.map((m) => (
              <li key={m.term} className="flex items-start gap-2">
                <span className="shrink-0 rounded-full bg-red-50 px-2 py-0.5 text-[10px] font-semibold text-red-700">
                  {tierLabel(m.tier)}
                </span>
                <span className="text-gray-700">{m.term}</span>
              </li>
            ))}
          </ul>
        </div>
      )}
      {result.gaps.length > 0 && (
        <div>
          <p className="mb-1 font-medium text-gray-700">In the job, not in this target role</p>
          <div className="flex flex-wrap gap-1.5">
            {result.gaps.map((g) => (
              <span key={g} className="rounded-full border border-gray-200 bg-gray-50 px-2 py-0.5 text-[11px] text-gray-600">
                {g}
              </span>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
