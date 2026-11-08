import { useEffect, useState } from 'react'
import {
  fetchJobRequirements,
  fetchRequirementSuggestions,
  saveJobRequirements,
  type JobRequirement,
  type JobRequirementInput,
} from '../../api'
import { CheckCircleIcon, PlusIcon, SparkleIcon, TrashIcon } from '../../Shared/Icons/icons'
import { CARD_LOOKS } from './cardLooks'
import { Frame, IconBadge } from './InfoCard'
import { inputClass } from '../../Shared/inputClass'
import {
  MAX_REQUIREMENTS,
  MAX_REQUIREMENT_LENGTH,
  TIERS,
  addedTo,
  freshSuggestions,
  groupedByTier,
  requirementProblem,
  without,
  withTier,
} from './requirementRules'

// What the job asks for, written by hand (step 129): each requirement is a term or a short sentence with a tier.
// The manual job match (Jobs page, Job Match, Manual) scores a persona against these when a job has any, and says
// which it is missing; without any it reads the job's text. Suggest from the job text offers the job's distinctive
// words to accept with one click (a word count, never an AI call). Every change saves the whole list at once. See
// plan/ai/tools/career/step-129-manual-job-match.md.
export function JobRequirements({ jobId }: { jobId: string }) {
  const [list, setList] = useState<JobRequirement[] | null>(null)
  const [term, setTerm] = useState('')
  const [tier, setTier] = useState(2)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [suggestions, setSuggestions] = useState<string[] | null>(null)

  useEffect(() => {
    setList(null)
    setError('')
    setSuggestions(null)
    fetchJobRequirements(jobId)
      .then(setList)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }, [jobId])

  // Saves the next list and shows what the server stored.
  function save(next: JobRequirementInput[], then?: () => void) {
    setBusy(true)
    setError('')
    saveJobRequirements(jobId, next)
      .then((saved) => {
        setList(saved)
        then?.()
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setBusy(false)
      })
  }

  function add(newTerm: string, newTier: number, then?: () => void) {
    if (!list) return
    const problem = requirementProblem(list, newTerm)
    if (problem !== '') {
      setError(problem)
      return
    }
    save(addedTo(list, newTerm, newTier), then)
  }

  function suggest() {
    setError('')
    fetchRequirementSuggestions(jobId)
      .then(setSuggestions)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  const groups = list ? groupedByTier(list) : []
  const fresh = list && suggestions ? freshSuggestions(suggestions, list) : []

  return (
    <Frame look="requirements" label="Requirements" className="mt-6">
      <div className="mb-1 flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2.5">
          <IconBadge look="requirements">
            <CheckCircleIcon className="h-4 w-4" />
          </IconBadge>
          <h2 className="text-sm font-semibold" style={{ color: CARD_LOOKS.requirements.accent.text }}>
            Requirements{list && list.length > 0 ? ` (${String(list.length)})` : ''}
          </h2>
        </div>
        <button
          type="button"
          onClick={suggest}
          disabled={list === null || busy}
          className="flex items-center gap-1.5 rounded-md border border-gray-200 px-3 py-1.5 text-xs font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-40"
        >
          <SparkleIcon />
          Suggest from the job text
        </button>
      </div>
      <p className="mb-3 text-xs text-gray-500">
        What this job asks for, with how much it matters. The manual job match (Jobs ▸ Job Match ▸ Manual) checks a persona against these
        and lists what is missing; without any it reads the job text instead. Run it again after a change.
      </p>

      {suggestions !== null && (
        <div className="mb-4 rounded-xl border border-white bg-white/70 p-3">
          <div className="mb-2 flex items-center justify-between gap-2">
            <p className="text-xs font-medium text-gray-700">
              Words this job stresses. Click one to add it as {TIERS.find((t) => t.value === tier)?.label.toLowerCase()}.
            </p>
            <button
              type="button"
              onClick={() => {
                setSuggestions(null)
              }}
              className="text-xs text-gray-500 underline hover:text-gray-700"
            >
              Close
            </button>
          </div>
          {fresh.length === 0 ? (
            <p className="text-xs text-gray-400">No more suggestions.</p>
          ) : (
            <div className="flex flex-wrap gap-1.5">
              {fresh.map((s) => (
                <button
                  key={s}
                  type="button"
                  disabled={busy}
                  onClick={() => {
                    add(s, tier)
                  }}
                  className="flex items-center gap-1 rounded-full border border-gray-200 bg-white px-2.5 py-1 text-xs text-gray-700 hover:bg-blue-50 disabled:opacity-40"
                >
                  <PlusIcon />
                  {s}
                </button>
              ))}
            </div>
          )}
        </div>
      )}

      {list === null ? (
        error === '' && <p className="text-xs text-gray-500">Loading…</p>
      ) : list.length === 0 ? (
        <p className="mb-3 text-xs text-gray-400">No requirements yet. Add the first one below.</p>
      ) : (
        <div className="mb-4 space-y-3">
          {groups.map((g) => (
            <div key={g.tier}>
              <p className="mb-1 text-[11px] font-semibold uppercase tracking-wide text-gray-500">{g.label}</p>
              <ul className="space-y-1">
                {g.items.map((r) => (
                  <li key={r.id} className="flex items-center gap-2 rounded-md border border-gray-100 px-3 py-1.5">
                    <span className="min-w-0 flex-1 break-words text-sm text-gray-800">{r.term}</span>
                    <select
                      aria-label={`Importance of ${r.term}`}
                      value={r.tier}
                      disabled={busy}
                      onChange={(e) => {
                        save(withTier(list, r.id, Number(e.target.value)))
                      }}
                      className="rounded-md border border-gray-200 px-1.5 py-1 text-xs text-gray-700"
                    >
                      {TIERS.map((t) => (
                        <option key={t.value} value={t.value}>
                          {t.label}
                        </option>
                      ))}
                    </select>
                    <button
                      type="button"
                      aria-label={`Remove ${r.term}`}
                      disabled={busy}
                      onClick={() => {
                        save(without(list, r.id))
                      }}
                      className="rounded-md p-1.5 text-gray-400 hover:bg-red-50 hover:text-red-600 disabled:opacity-40"
                    >
                      <TrashIcon />
                    </button>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      )}

      <form
        className="flex flex-wrap items-start gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          add(term, tier, () => {
            setTerm('')
          })
        }}
      >
        <input
          aria-label="New requirement"
          value={term}
          maxLength={MAX_REQUIREMENT_LENGTH}
          disabled={list === null || busy}
          onChange={(e) => {
            setTerm(e.target.value)
            setError('')
          }}
          placeholder="For example Go, or Abgeschlossene Ausbildung als Erzieher"
          className={`${inputClass} min-w-48 flex-1`}
        />
        <select
          aria-label="Importance of the new requirement"
          value={tier}
          onChange={(e) => {
            setTier(Number(e.target.value))
          }}
          className="rounded-md border border-gray-300 px-2 py-1.5 text-sm text-gray-700"
        >
          {TIERS.map((t) => (
            <option key={t.value} value={t.value}>
              {t.label}
            </option>
          ))}
        </select>
        <button
          type="submit"
          disabled={list === null || busy}
          className="flex items-center gap-1.5 rounded-md bg-gray-800 px-3.5 py-1.5 text-sm font-medium text-white hover:bg-gray-900 disabled:opacity-40"
        >
          <PlusIcon />
          Add
        </button>
      </form>
      <div className="mt-1 flex items-center justify-between gap-2">
        <p role="alert" className="min-h-[1.2em] text-xs text-red-700">
          {error}
        </p>
        <p className="text-[11px] text-gray-400">
          {list ? list.length : 0} of {MAX_REQUIREMENTS}
        </p>
      </div>
    </Frame>
  )
}
