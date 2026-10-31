import { useState } from 'react'
import { applyPersonaAiRun, type PersonaAiRun, type ProposedPersona, type ProposedRole } from '../api'
import { ConfirmationModal } from '../Shared/ConfirmationModal/ConfirmationModal'
import { inputClass } from '../Shared/inputClass'
import { Modal } from '../Shared/Modal/Modal'
import { XIcon } from '../Shared/Icons/icons'
import { MAX_SHORT } from './personaProposal'
import { PersonaPreview } from './PersonaPreview'
import { isNewCompany, isNewSkill, isNewTitle, newItems, newItemsText } from './knownData'
import { useKnownData } from './useKnownData'
import {
  accepted,
  addSkill,
  continueLabel,
  countsLabel,
  decide,
  declineAll,
  declinedNames,
  draftsFrom,
  duplicateHint,
  finalPersona,
  previewSummary,
  removeAt,
  reviewProblem,
  saveLabel,
  toDecisions,
  type Draft,
} from './reviewRules'

// The review of a ready run's proposal (steps 112 and 113): the user decides on EVERY proposed
// persona, Accept or Decline, and nothing is pre-accepted. An accepted or undecided persona can be
// edited (name, description, details, skills, experience); a declined one is collapsed. A duplicate
// is hinted. Saving needs a decision on every persona and creates the accepted ones in one go; the
// run is then closed. Before anything is saved there is a preview: the accepted personas exactly as
// they will be saved, and the declined ones by name; Save is only on that screen. Nothing exists
// until Save is pressed.
export function ReviewPersonasDialog({
  run,
  existing,
  onClose,
  onCreated,
}: {
  run: PersonaAiRun
  existing: { id: string; name: string }[]
  onClose: () => void
  onCreated: (created: { id: string; name: string }[]) => void
}) {
  const [drafts, setDrafts] = useState<Draft[]>(() => draftsFrom(run.proposal ?? { personas: [], note: '' }))
  const [skillText, setSkillText] = useState<Record<number, string>>({})
  const [error, setError] = useState('')
  const [creating, setCreating] = useState(false)
  const [decliningAll, setDecliningAll] = useState(false)
  // 'decide' (accept, decline, edit) then 'preview' (read-only, and the only place Save is).
  const [step, setStep] = useState<'decide' | 'preview'>('decide')
  const problem = reviewProblem(drafts)
  // The user's existing skills, employers and titles, to mark what the AI added that is not in them
  // (step 113). A hint only: nothing is marked while it loads, when it cannot be read, or when there
  // is nothing to compare with.
  const knownState = useKnownData(existing.map((e) => e.id))
  const known = knownState.status === 'ready' && knownState.comparable ? knownState.known : null
  const compareNote =
    knownState.status === 'loading'
      ? 'Checking the proposal against your existing data…'
      : knownState.status === 'failed'
        ? 'Your existing data could not be read, so nothing is marked as new.'
        : knownState.comparable
          ? 'Amber items are not in your existing skills, employers or job titles: check them before you accept.'
          : 'You have no skills or experience yet to compare with, so nothing is marked.'

  function edit(index: number, change: (p: ProposedPersona) => ProposedPersona) {
    setDrafts((list) => list.map((d, i) => (i === index ? { ...d, persona: change(d.persona) } : d)))
  }

  function save(decisions: ReturnType<typeof toDecisions>) {
    if (decisions === null) return
    setError('')
    setCreating(true)
    applyPersonaAiRun(run.id, decisions)
      .then(onCreated)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setCreating(false)
      })
  }

  const detail = (index: number, field: keyof ProposedPersona['details'], label: string) => (
    <label className="block text-xs font-medium text-gray-500">
      {label}
      <input
        value={String(drafts[index].persona.details[field])}
        onChange={(e) => {
          edit(index, (p) => ({
            ...p,
            details: { ...p.details, [field]: field === 'minSalary' ? Math.max(0, Number(e.target.value) || 0) : e.target.value },
          }))
        }}
        type={field === 'minSalary' ? 'number' : 'text'}
        className={`mt-1 ${inputClass}`}
      />
    </label>
  )

  return (
    <Modal
      open
      title={step === 'decide' ? 'Review the proposed personas' : 'Preview before saving'}
      onClose={creating ? () => undefined : onClose}
      size="wide"
    >
      {step === 'decide' && (
        <>
          {run.proposal?.note && <p className="mb-4 rounded-lg bg-purple-50 p-3 text-sm text-purple-900">{run.proposal.note}</p>}
          <p className="mb-3 text-xs text-gray-500" aria-live="polite">
            {compareNote}
          </p>
          <p className="mb-4 text-sm text-gray-600">
            Decide on every persona: accept the ones you want (fix what is not right first) and decline the others. Nothing is created until
            you have looked at the preview and pressed Save.
          </p>

          <div className="space-y-5">
            {drafts.map((d, i) => {
              const hint = duplicateHint(d.persona, existing)
              return (
                <section
                  key={i}
                  aria-label={`Persona ${i + 1}`}
                  data-decision={d.decision}
                  className={`rounded-xl border p-4 ${
                    d.decision === 'accepted'
                      ? 'border-green-300 bg-green-50/40'
                      : d.decision === 'declined'
                        ? 'border-gray-100 bg-gray-50'
                        : 'border-gray-200 bg-white'
                  }`}
                >
                  <div className="mb-3 flex flex-wrap items-center gap-2">
                    <span className={`text-sm font-semibold ${d.decision === 'declined' ? 'text-gray-400 line-through' : 'text-gray-900'}`}>
                      {d.persona.name || '(no name)'}
                    </span>
                    <span
                      className={`rounded-full px-2 py-0.5 text-[11px] font-semibold ${
                        d.decision === 'accepted'
                          ? 'bg-green-100 text-green-800'
                          : d.decision === 'declined'
                            ? 'bg-rose-100 text-rose-800'
                            : 'bg-gray-100 text-gray-600'
                      }`}
                    >
                      {d.decision === 'accepted' ? 'Accepted' : d.decision === 'declined' ? 'Declined' : 'Undecided'}
                    </span>
                    <span className="ml-auto flex gap-2">
                      <button
                        type="button"
                        aria-pressed={d.decision === 'accepted'}
                        onClick={() => {
                          setDrafts((list) => decide(list, i, 'accepted'))
                        }}
                        className={`rounded-full border px-3 py-1 text-xs font-medium ${
                          d.decision === 'accepted'
                            ? 'border-green-600 bg-green-600 text-white'
                            : 'border-green-300 text-green-800 hover:bg-green-50'
                        }`}
                      >
                        Accept
                      </button>
                      <button
                        type="button"
                        aria-pressed={d.decision === 'declined'}
                        onClick={() => {
                          setDrafts((list) => decide(list, i, 'declined'))
                        }}
                        className={`rounded-full border px-3 py-1 text-xs font-medium ${
                          d.decision === 'declined'
                            ? 'border-rose-600 bg-rose-600 text-white'
                            : 'border-rose-300 text-rose-800 hover:bg-rose-50'
                        }`}
                      >
                        Decline
                      </button>
                    </span>
                  </div>
                  {d.decision === 'declined' && (
                    <p className="text-xs text-gray-500">Declined: this persona will not be created. Press Decline again to undo.</p>
                  )}
                  {hint && d.decision !== 'declined' && (
                    <p className="mb-3 rounded-md bg-amber-50 px-3 py-2 text-xs text-amber-900">{hint}</p>
                  )}
                  {known && d.decision !== 'declined' && newItemsText(newItems(known, d.persona)) !== null && (
                    <p className="mb-3 rounded-md bg-amber-50 px-3 py-2 text-xs text-amber-900">
                      {newItemsText(newItems(known, d.persona))}
                    </p>
                  )}

                  <fieldset hidden={d.decision === 'declined'} className="space-y-3">
                    <div className="grid gap-3 sm:grid-cols-2">
                      <label className="block text-xs font-medium text-gray-500">
                        Name
                        <input
                          value={d.persona.name}
                          onChange={(e) => {
                            edit(i, (p) => ({ ...p, name: e.target.value }))
                          }}
                          className={`mt-1 ${inputClass}`}
                        />
                      </label>
                      <label className="block text-xs font-medium text-gray-500">
                        Description
                        <input
                          value={d.persona.description}
                          onChange={(e) => {
                            edit(i, (p) => ({ ...p, description: e.target.value }))
                          }}
                          className={`mt-1 ${inputClass}`}
                        />
                      </label>
                      {detail(i, 'headline', 'Headline')}
                      {detail(i, 'location', 'Location')}
                      {detail(i, 'desiredTitles', 'Desired titles')}
                      {detail(i, 'desiredLocations', 'Desired locations')}
                      {detail(i, 'minSalary', 'Minimum salary')}
                    </div>
                    <label className="block text-xs font-medium text-gray-500">
                      Summary
                      <textarea
                        value={d.persona.details.summary}
                        onChange={(e) => {
                          edit(i, (p) => ({ ...p, details: { ...p.details, summary: e.target.value } }))
                        }}
                        rows={3}
                        className={`mt-1 ${inputClass}`}
                      />
                    </label>

                    <div>
                      <p className="mb-1.5 text-xs font-medium text-gray-500">Skills</p>
                      <div className="flex flex-wrap gap-1.5">
                        {d.persona.skills.map((skill, s) => (
                          <span
                            key={skill}
                            title={known && isNewSkill(known, skill) ? 'Not in your data' : undefined}
                            className={`inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-xs ${
                              known && isNewSkill(known, skill)
                                ? 'bg-amber-100 text-amber-900 ring-1 ring-amber-300'
                                : 'bg-gray-100 text-gray-800'
                            }`}
                          >
                            {skill}
                            <button
                              type="button"
                              aria-label={`Remove ${skill}`}
                              onClick={() => {
                                edit(i, (p) => ({ ...p, skills: removeAt(p.skills, s) }))
                              }}
                              className="text-gray-400 hover:text-red-700"
                            >
                              <XIcon />
                            </button>
                          </span>
                        ))}
                        {d.persona.skills.length === 0 && <span className="text-xs text-gray-400">No skills.</span>}
                      </div>
                      <div className="mt-2 flex gap-2">
                        <input
                          value={skillText[i] ?? ''}
                          onChange={(e) => {
                            setSkillText((t) => ({ ...t, [i]: e.target.value }))
                          }}
                          onKeyDown={(e) => {
                            if (e.key === 'Enter') {
                              e.preventDefault()
                              edit(i, (p) => ({ ...p, skills: addSkill(p.skills, skillText[i] ?? '') }))
                              setSkillText((t) => ({ ...t, [i]: '' }))
                            }
                          }}
                          maxLength={MAX_SHORT}
                          placeholder="Add a skill"
                          aria-label={`Add a skill to persona ${i + 1}`}
                          className={`max-w-xs ${inputClass}`}
                        />
                        <button
                          type="button"
                          onClick={() => {
                            edit(i, (p) => ({ ...p, skills: addSkill(p.skills, skillText[i] ?? '') }))
                            setSkillText((t) => ({ ...t, [i]: '' }))
                          }}
                          className="rounded-md border border-gray-200 px-3 py-1.5 text-xs text-gray-700 hover:bg-gray-50"
                        >
                          Add
                        </button>
                      </div>
                    </div>

                    {d.persona.experience.length > 0 && (
                      <div>
                        <p className="mb-1.5 text-xs font-medium text-gray-500">Experience</p>
                        <ul className="space-y-2">
                          {d.persona.experience.map((role, r) => (
                            <li key={r} className="rounded-lg border border-gray-100 p-3">
                              <RoleFields
                                newCompany={known !== null && isNewCompany(known, role.company)}
                                newTitle={known !== null && isNewTitle(known, role.title)}
                                role={role}
                                onChange={(next) => {
                                  edit(i, (p) => ({ ...p, experience: p.experience.map((x, k) => (k === r ? next : x)) }))
                                }}
                                onRemove={() => {
                                  edit(i, (p) => ({ ...p, experience: removeAt(p.experience, r) }))
                                }}
                              />
                            </li>
                          ))}
                        </ul>
                      </div>
                    )}
                  </fieldset>
                </section>
              )
            })}
          </div>

          <p className="mt-5 text-sm font-medium text-gray-700" aria-live="polite">
            {countsLabel(drafts)}
          </p>
          {(error !== '' || problem !== null) && <p className="mt-2 text-sm text-red-700">{error !== '' ? error : problem}</p>}
          <div className="mt-4 flex flex-wrap items-center justify-end gap-2">
            <button
              type="button"
              onClick={() => {
                setDecliningAll(true)
              }}
              disabled={creating}
              className="mr-auto rounded-md border border-rose-200 px-3.5 py-2 text-sm font-medium text-rose-700 hover:bg-rose-50 disabled:opacity-50"
            >
              Decline all
            </button>
            <button
              type="button"
              onClick={onClose}
              disabled={creating}
              className="rounded-md border border-gray-200 px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50"
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={() => {
                setError('')
                setStep('preview')
              }}
              disabled={creating || problem !== null}
              className="rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white hover:bg-gray-800 disabled:opacity-50"
            >
              {continueLabel(drafts)}
            </button>
          </div>
        </>
      )}

      {step === 'preview' && (
        <>
          <p className="mb-1 text-sm font-semibold text-gray-900" aria-live="polite">
            {previewSummary(drafts)}
          </p>
          <p className="mb-4 text-sm text-gray-600">
            This is exactly what will be saved. Nothing exists yet: press Save to create it, or go back to change anything.
          </p>
          <div className="space-y-4">
            {accepted(drafts).map((persona, i) => (
              <PersonaPreview key={i} persona={finalPersona(persona)} known={known} />
            ))}
          </div>
          {declinedNames(drafts).length > 0 && (
            <section
              aria-label="Declined personas"
              className="mt-4 rounded-xl border border-rose-100 bg-rose-50/50 p-3 text-sm text-rose-900"
            >
              <p className="mb-1 text-xs font-semibold uppercase tracking-wide text-rose-700">Declined, will not be created</p>
              <ul className="list-inside list-disc">
                {declinedNames(drafts).map((n, i) => (
                  <li key={i}>{n}</li>
                ))}
              </ul>
            </section>
          )}
          {error !== '' && <p className="mt-3 text-sm text-red-700">{error}</p>}
          <div className="mt-5 flex justify-end gap-2">
            <button
              type="button"
              onClick={() => {
                setError('')
                setStep('decide')
              }}
              disabled={creating}
              className="mr-auto rounded-md border border-gray-200 px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50"
            >
              Back
            </button>
            <button
              type="button"
              onClick={onClose}
              disabled={creating}
              className="rounded-md border border-gray-200 px-3.5 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50"
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={() => {
                save(toDecisions(drafts))
              }}
              disabled={creating || problem !== null}
              className="rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white hover:bg-gray-800 disabled:opacity-50"
            >
              {creating ? 'Saving…' : saveLabel(drafts)}
            </button>
          </div>
        </>
      )}

      <ConfirmationModal
        open={decliningAll}
        title="Decline all"
        message="Decline every proposed persona? Nothing will be created and this run is closed."
        confirmLabel="Decline all"
        variant="danger"
        onConfirm={() => {
          setDecliningAll(false)
          save(toDecisions(declineAll(drafts)))
        }}
        onCancel={() => {
          setDecliningAll(false)
        }}
      />
    </Modal>
  )
}

function RoleFields({
  role,
  onChange,
  onRemove,
  newCompany,
  newTitle,
}: {
  role: ProposedRole
  onChange: (r: ProposedRole) => void
  onRemove: () => void
  // Not in the user's own data (step 113): a hint, shown next to the label.
  newCompany: boolean
  newTitle: boolean
}) {
  const field = (key: keyof ProposedRole, label: string, flagged = false) => (
    <label className="block text-xs font-medium text-gray-500">
      {label}
      {flagged && (
        <span className="ml-1.5 rounded bg-amber-100 px-1.5 py-0.5 text-[10px] font-medium text-amber-800">not in your data</span>
      )}
      <input
        value={role[key]}
        onChange={(e) => {
          onChange({ ...role, [key]: e.target.value })
        }}
        className={`mt-1 ${inputClass} ${flagged ? 'border-amber-400' : ''}`}
      />
    </label>
  )
  return (
    <div className="space-y-2">
      <div className="grid gap-2 sm:grid-cols-2">
        {field('company', 'Company', newCompany)}
        {field('title', 'Title', newTitle)}
        {field('startDate', 'Start')}
        {field('endDate', 'End')}
      </div>
      <label className="block text-xs font-medium text-gray-500">
        Description
        <textarea
          value={role.description}
          onChange={(e) => {
            onChange({ ...role, description: e.target.value })
          }}
          rows={2}
          className={`mt-1 ${inputClass}`}
        />
      </label>
      <button type="button" onClick={onRemove} className="text-xs text-red-700 hover:underline">
        Remove this entry
      </button>
    </div>
  )
}
