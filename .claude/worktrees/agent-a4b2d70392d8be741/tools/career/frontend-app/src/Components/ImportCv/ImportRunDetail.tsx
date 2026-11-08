import type { CVImportRun } from '../../api'
import type { CVImportProposal } from './parseProposal'
import type { InsertResult, ItemResult } from './insertProposal'
import { experienceLabel } from './experienceLabel'

// 'skipped' covers both "the AI proposed it but the user unchecked it"
// and "never attempted" alike — a stored InsertResult can't
// distinguish those two (only checked+attempted items ever appear in
// it), and the distinction isn't worth persisting a third state for.
type RunStatus = 'inserted' | 'failed' | 'skipped'

function runStatusFor(result: ItemResult | undefined): RunStatus {
  if (!result) return 'skipped'
  return result.ok ? 'inserted' : 'failed'
}

const STATUS_LABEL: Record<RunStatus, string> = { inserted: 'Inserted', failed: 'Failed', skipped: 'Skipped' }
const STATUS_CLASSES: Record<RunStatus, string> = {
  inserted: 'bg-green-100 text-green-700',
  failed: 'bg-red-100 text-red-700',
  skipped: 'bg-gray-100 text-gray-500',
}

function StatusBadge({ result }: { result: ItemResult | undefined }) {
  const status = runStatusFor(result)
  return (
    <span className={`shrink-0 rounded-full px-2 py-0.5 text-xs font-medium ${STATUS_CLASSES[status]}`}>
      {STATUS_LABEL[status]}
      {status === 'failed' && result?.error ? `: ${result.error}` : ''}
    </span>
  )
}

function Row({ label, result }: { label: string; result: ItemResult | undefined }) {
  return (
    <li className="flex items-center justify-between gap-3 py-1.5 text-sm">
      <span className="truncate text-gray-700">{label}</span>
      <StatusBadge result={result} />
    </li>
  )
}

// ImportRunDetail is a deliberately plain, checkbox-free read-only
// rendering of one past cv_import_runs entry — "the object with the
// AI's own suggestions and what the user saved," per the feature ask.
// Not a reuse of ProposalRow (the live review checklist's own
// component): that component's whole point is an interactive
// checkbox, which would misleadingly imply this view can still change
// anything. See plan/ai/media/step-05-career-history.md.
export function ImportRunDetail({ run, onClose }: { run: CVImportRun; onClose: () => void }) {
  const proposal = run.aiProposal as CVImportProposal
  const summary = run.saveSummary as InsertResult | null

  const skillResult = (index: number) => summary?.skills.find((s) => s.index === index)
  const experienceResult = (index: number) => summary?.experience.find((e) => e.index === index)

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <p className="text-sm text-gray-500">Reviewed “{run.originalFilename}”</p>
          <p className="text-xs text-gray-400">{new Date(run.createdAt).toLocaleString()}</p>
        </div>
        <button
          type="button"
          onClick={onClose}
          className="rounded-md border border-gray-200 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50"
        >
          Close
        </button>
      </div>

      {!summary && <p className="text-sm text-gray-400">Nothing was saved from this analysis.</p>}

      <section className="rounded-md border border-gray-200 p-4 shadow-sm">
        <h2 className="mb-2 text-base font-semibold text-gray-900">Profile</h2>
        <ul>
          <Row label={proposal.profile.name || '(no name proposed)'} result={summary?.profile} />
        </ul>
      </section>

      <section className="rounded-md border border-gray-200 p-4 shadow-sm">
        <h2 className="mb-2 text-base font-semibold text-gray-900">Personas</h2>
        <ul>
          <Row
            label={[proposal.persona.name, proposal.persona.title].filter(Boolean).join(' — ') || '(no persona proposed)'}
            result={summary?.persona}
          />
        </ul>
      </section>

      <section className="rounded-md border border-gray-200 p-4 shadow-sm">
        <h2 className="mb-2 text-base font-semibold text-gray-900">Personal Details</h2>
        <ul>
          <Row label={proposal.personalDetails.headline || '(no headline proposed)'} result={summary?.personalDetails} />
        </ul>
      </section>

      <section className="rounded-md border border-gray-200 p-4 shadow-sm">
        <h2 className="mb-2 text-base font-semibold text-gray-900">Skills</h2>
        {proposal.skills.length === 0 && <p className="text-sm text-gray-400">No skills proposed.</p>}
        <ul className="divide-y divide-gray-100">
          {proposal.skills.map((skill, i) => (
            <Row key={i} label={skill.value} result={skillResult(i)} />
          ))}
        </ul>
      </section>

      <section className="rounded-md border border-gray-200 p-4 shadow-sm">
        <h2 className="mb-2 text-base font-semibold text-gray-900">Experience</h2>
        {proposal.experience.length === 0 && <p className="text-sm text-gray-400">No experience proposed.</p>}
        <ul className="divide-y divide-gray-100">
          {proposal.experience.map((exp, i) => (
            <Row key={i} label={experienceLabel(exp)} result={experienceResult(i)} />
          ))}
        </ul>
      </section>
    </div>
  )
}
