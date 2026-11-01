import { useEffect, useState } from 'react'
import { applyProfileMatchRules, ApplyRulesError, type ApplyRulesSummary } from '../../api'
import { AppLink } from '../../Shared/AppLink/AppLink'
import { jobBoardPathFor } from '../../paths'
import { plural } from '../CompaniesPage/companyStats'
import { details, failureText, headline } from './applyText'

type View =
  | { kind: 'idle' }
  | { kind: 'working'; dryRun: boolean }
  | { kind: 'preview'; summary: ApplyRulesSummary }
  | { kind: 'done'; summary: ApplyRulesSummary }
  | { kind: 'failed'; text: string }

// "Apply to existing jobs" on a profile's settings page (step 143). The rules above only act on jobs found from now on; this runs
// them over the jobs that are already there. It is two steps: Preview counts what would happen and writes nothing, then "Add …"
// does it. A card that is on the board is never moved; the first rule of the list that fits a job decides. A change to the rules
// drops an old preview (it would no longer be what the button does). See plan/ai/tools/career/step-143-apply-rules-to-existing-jobs.md.
export function ApplyPanel({
  profileId,
  rulesKey,
  hasRules,
  disabled,
}: {
  profileId: string
  rulesKey: string
  hasRules: boolean
  disabled: boolean
}) {
  const [view, setView] = useState<View>({ kind: 'idle' })

  useEffect(() => {
    setView({ kind: 'idle' })
  }, [rulesKey, profileId])

  function run(dryRun: boolean) {
    setView({ kind: 'working', dryRun })
    applyProfileMatchRules(profileId, dryRun)
      .then((summary) => {
        setView(dryRun ? { kind: 'preview', summary } : { kind: 'done', summary })
      })
      .catch((err: unknown) => {
        if (err instanceof ApplyRulesError) setView({ kind: 'failed', text: failureText(err.message, err.summary) })
        else setView({ kind: 'failed', text: err instanceof Error ? err.message : String(err) })
      })
  }

  const working = view.kind === 'working'
  const summary = view.kind === 'preview' || view.kind === 'done' ? view.summary : null
  return (
    <section aria-label="Apply to existing jobs" className="mt-6 rounded-2xl border border-gray-100 bg-white/70 p-4">
      <h2 className="mb-1 text-sm font-semibold text-gray-700">Apply to existing jobs</h2>
      <p className="mb-3 text-sm text-gray-600">
        The rules above act on jobs found from now on. Here you can apply them to the jobs that are already there too: the first rule that
        fits a job decides its status. A job that is already on this board is never moved.{' '}
        <strong>Jobs you took off the board earlier come back</strong> if they fit a rule, so look at the preview first.
      </p>

      <div className="flex flex-wrap items-center gap-2">
        <button
          type="button"
          disabled={!hasRules || disabled || working}
          onClick={() => {
            run(true)
          }}
          className="rounded-md border border-gray-300 px-3.5 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 disabled:opacity-50"
        >
          {view.kind === 'working' && view.dryRun ? 'Checking…' : 'Preview'}
        </button>
        {view.kind === 'preview' && view.summary.total > 0 && (
          <button
            type="button"
            disabled={disabled || working}
            onClick={() => {
              run(false)
            }}
            className="rounded-md bg-gray-800 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-900 disabled:opacity-50"
          >
            {`Add ${plural(view.summary.total, 'job', 'jobs')} to the board`}
          </button>
        )}
        {view.kind === 'working' && !view.dryRun && <span className="text-sm text-gray-500">Adding…</span>}
      </div>
      {!hasRules && <p className="mt-2 text-xs text-gray-500">Add a rule first.</p>}

      <div aria-live="polite">
        {summary !== null && (
          <div className="mt-3 rounded-xl border border-gray-100 bg-gray-50 p-3 text-sm text-gray-800">
            <p className="font-medium">{headline(summary, view.kind === 'preview')}</p>
            <p className="mt-1 text-xs text-gray-500">{details(summary)}</p>
            {view.kind === 'done' && summary.total > 0 && (
              <p className="mt-2 text-xs">
                <AppLink href={jobBoardPathFor(profileId)} className="font-medium text-blue-700 hover:underline">
                  Open the job board
                </AppLink>
              </p>
            )}
          </div>
        )}
        {view.kind === 'failed' && (
          <p role="alert" className="mt-3 text-sm text-red-700">
            {view.text}
          </p>
        )}
      </div>
    </section>
  )
}
