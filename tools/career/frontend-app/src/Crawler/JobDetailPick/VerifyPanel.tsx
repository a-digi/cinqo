import { useEffect, useRef, useState } from 'react'
import {
  PickError,
  savePickChoices,
  testPickChoices,
  type Choices,
  type PickCandidates,
  type PickField,
  type PickTestJob,
} from './jobDetailPick'
import {
  changedCount,
  checkedJobs,
  choicesKey,
  chars,
  describePickFailure,
  FIELD_NAMES,
  recrawlNote,
  saveGate,
  shortValue,
  splitFlags,
  valueRows,
  verificationState,
  type Verification,
} from './jobDetailPickRules'

const buttonClass =
  'rounded-md border border-blue-300 bg-white px-2.5 py-1 text-xs font-medium text-blue-800 hover:bg-blue-100 disabled:cursor-not-allowed disabled:opacity-50'
const primaryClass =
  'rounded-md bg-blue-700 px-2.5 py-1 text-xs font-medium text-white hover:bg-blue-800 disabled:cursor-not-allowed disabled:opacity-50'

function errorText(err: unknown): string {
  if (err instanceof PickError) return describePickFailure(err.message, err.advice)
  return err instanceof Error ? err.message : 'Something went wrong.'
}

function JobCheck({ job, chosen }: { job: PickTestJob; chosen: PickField[] }) {
  const [full, setFull] = useState(false)
  if (job.error) {
    return (
      <div className="rounded-md border border-red-200 bg-red-50 p-2">
        <div className="font-medium">“{job.title}”</div>
        <p className="text-red-700">This page could not be read: {job.error}</p>
      </div>
    )
  }
  const rows = valueRows(job, chosen)
  const description = job.values.description
  return (
    <div className="rounded-md border border-gray-200 bg-white p-2" data-testid="verify-job">
      <div className="font-medium">“{job.title}”</div>
      <table className="mt-1 w-full text-left">
        <tbody>
          {rows.map((r) => (
            <tr key={r.field} className="align-top">
              <td className="w-24 pr-2 text-gray-600">{FIELD_NAMES[r.field]}</td>
              <td className="pr-2">{r.value === '' ? <span className="text-amber-900">(empty)</span> : r.value}</td>
              <td className="text-gray-500">{r.note}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {job.report?.truncated && (
        <p className="text-amber-900">
          The description was cut at its limit: {chars(job.report.chars)} of {chars(job.report.fullLength)} kept.
        </p>
      )}
      {job.diff && (
        <p className="text-gray-700">
          Compared with the description saved now:{' '}
          {job.storedChars === 0
            ? 'there is none yet.'
            : job.diff.unchanged
              ? 'the same.'
              : `${job.diff.added} lines added, ${job.diff.removed} removed, ${job.diff.kept} kept.`}
        </p>
      )}
      {description !== '' && (
        <div className="mt-1">
          <button
            type="button"
            className={buttonClass}
            onClick={() => {
              setFull((f) => !f)
            }}
          >
            {full ? 'Hide the description' : 'Read the description'}
          </button>
          {full && (
            <pre className="mt-1 max-h-56 overflow-auto whitespace-pre-wrap rounded-md bg-gray-900 p-2 font-mono text-[11px] text-gray-100">
              {description.slice(0, 6000)}
            </pre>
          )}
        </div>
      )}
    </div>
  )
}

// Checking the picks on sample jobs and saving them (plan 186, step 8). The check is the REAL extraction on up to three saved jobs of the
// link; its flags say what looks off. Saving needs a check that is still about the picks on the screen, no blocking flag, and the person's
// tick when there are warnings (or when the link has no saved jobs to check on). Nothing is recrawled by saving.
export function VerifyPanel({
  linkId,
  page,
  choices,
  previewValid,
  onSaved,
}: {
  linkId: string
  page: PickCandidates
  choices: Choices
  previewValid: boolean
  onSaved: (yaml: string) => void
}) {
  const [verification, setVerification] = useState<Verification | null>(null)
  const [checking, setChecking] = useState(false)
  const [checkError, setCheckError] = useState('')
  const [confirmed, setConfirmed] = useState(false)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState('')
  const [saved, setSaved] = useState('')
  const seq = useRef(0)
  const key = choicesKey(choices)
  const state = verificationState(verification, key)
  const chosen: PickField[] = choices.fields.map((f) => f.label)
  const jobIds = page.samples.map((s) => s.jobId)
  const hasSamples = jobIds.length > 0
  const gate = saveGate({ previewValid, verification: state, test: verification?.test ?? null, hasSamples, confirmed, saving })
  const groups = verification ? splitFlags(verification.test.flags) : null

  // A change of the picks ends an earlier tick and an earlier "saved" note.
  useEffect(() => {
    setConfirmed(false)
    setSaved('')
  }, [key])

  function check() {
    const n = ++seq.current
    setChecking(true)
    setCheckError('')
    setSaved('')
    testPickChoices(linkId, choices, jobIds)
      .then((t) => {
        if (n !== seq.current) return
        setVerification({ key, test: t })
        setConfirmed(false)
      })
      .catch((err: unknown) => {
        if (n !== seq.current) return
        setCheckError(errorText(err))
      })
      .finally(() => {
        if (n === seq.current) setChecking(false)
      })
  }

  function save() {
    setSaving(true)
    setSaveError('')
    savePickChoices(linkId, choices)
      .then((p) => {
        const checked = verification ? checkedJobs(verification.test).length : 0
        const changed = verification ? changedCount(verification.test) : 0
        setSaved(`Saved. ${recrawlNote(checked, changed, page.jobCount)}`)
        onSaved(p.yaml ?? '')
      })
      .catch((err: unknown) => {
        setSaveError(errorText(err))
      })
      .finally(() => {
        setSaving(false)
      })
  }

  const hasStored = page.current.yaml !== ''
  // The tick is asked for only when it can matter: warnings on a fresh check, or a link with no jobs to check on.
  const needsTick = hasSamples
    ? state === 'fresh' && groups !== null && groups.blocking.length === 0 && groups.warnings.length > 0
    : previewValid

  return (
    <div className="rounded-md border border-blue-200 bg-white p-2" data-testid="job-detail-verify">
      <div className="font-medium">Check the picks on sample jobs</div>
      <p className="mt-1 text-gray-600">
        {hasSamples
          ? `The picks are tried on ${jobIds.length} saved job${jobIds.length === 1 ? '' : 's'} of this link (their pages are read one after the other, so it takes a little while). Nothing is saved by checking.`
          : 'This link has no saved jobs yet, so the picks cannot be checked on a job.'}
      </p>
      <div className="mt-2 flex flex-wrap items-center gap-2">
        <button type="button" className={primaryClass} disabled={!hasSamples || !previewValid || checking} onClick={check}>
          {checking
            ? 'Reading the pages…'
            : verification
              ? 'Check again'
              : `Check on ${jobIds.length} job${jobIds.length === 1 ? '' : 's'}`}
        </button>
        {state === 'stale' && <span className="text-amber-900">The picks changed since this check.</span>}
      </div>
      {checkError && (
        <p role="alert" className="mt-1 text-red-700">
          {checkError}
        </p>
      )}
      {verification && (
        <div className={`mt-2 space-y-2 ${state === 'stale' ? 'opacity-60' : ''}`}>
          {verification.test.jobs.map((j) => (
            <JobCheck key={j.jobId} job={j} chosen={chosen} />
          ))}
          {groups && (
            <div className="space-y-1">
              {groups.blocking.map((f) => (
                <p key={`${f.code}|${f.field ?? ''}`} role="alert" className="rounded-md border border-red-300 bg-red-50 p-2 text-red-800">
                  {f.message}
                </p>
              ))}
              {groups.warnings.map((f) => (
                <p key={`${f.code}|${f.field ?? ''}`} className="rounded-md border border-amber-300 bg-amber-50 p-2 text-amber-900">
                  {f.field ? `${FIELD_NAMES[f.field]}: ` : ''}
                  {f.message}
                </p>
              ))}
              {groups.info.map((f) => (
                <p key={`${f.code}|${f.field ?? ''}`} className="text-gray-600">
                  {f.field ? `${FIELD_NAMES[f.field]}: ` : ''}
                  {f.message}
                </p>
              ))}
              {groups.blocking.length + groups.warnings.length + groups.info.length === 0 && (
                <p className="text-green-800">Nothing looks off on these jobs.</p>
              )}
            </div>
          )}
          <p className="text-gray-700">
            {recrawlNote(checkedJobs(verification.test).length, changedCount(verification.test), page.jobCount)}
          </p>
        </div>
      )}

      <div className="mt-3 border-t border-blue-100 pt-2">
        {hasStored && (
          <p className="mb-1 text-gray-700">
            Saving replaces the job-detail instructions this link has now
            {page.current.lossy.length > 0 ? ' (and what this screen cannot show of them)' : ''}.
          </p>
        )}
        {needsTick && (
          <label className="flex items-center gap-2">
            <input
              type="checkbox"
              checked={confirmed}
              onChange={(e) => {
                setConfirmed(e.target.checked)
              }}
            />
            <span>{hasSamples ? 'I looked at the warnings and these picks are fine' : 'Save without checking on a job'}</span>
          </label>
        )}
        <div className="mt-2 flex flex-wrap items-center gap-2">
          <button type="button" className={primaryClass} disabled={!gate.ok} onClick={save}>
            {saving ? 'Saving…' : 'Save these instructions'}
          </button>
          {!gate.ok && !saving && <span className="text-gray-600">{shortValue(gate.reason, 160)}</span>}
        </div>
        {saveError && (
          <p role="alert" className="mt-1 text-red-700">
            {saveError}
          </p>
        )}
        {saved && <p className="mt-1 text-green-800">{saved}</p>}
      </div>
    </div>
  )
}
