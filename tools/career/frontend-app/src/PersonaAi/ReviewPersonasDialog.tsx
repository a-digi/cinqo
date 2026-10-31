import { useState } from 'react'
import { applyPersonaAiRun, type PersonaAiRun, type ProposedPersona, type ProposedRole } from '../api'
import { inputClass } from '../Shared/inputClass'
import { Modal } from '../Shared/Modal/Modal'
import { XIcon } from '../Shared/Icons/icons'
import { MAX_SHORT } from './personaProposal'
import { accepted, addSkill, createLabel, draftsFrom, duplicateHint, removeAt, reviewProblem, type Draft } from './reviewRules'

// The review of a ready run's proposal (step 112): every proposed persona can be left out or
// edited (name, description, details, skills, experience), a duplicate is hinted, and what is kept
// is created in one go. Nothing exists until Create is pressed.
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
  const problem = reviewProblem(drafts)

  function edit(index: number, change: (p: ProposedPersona) => ProposedPersona) {
    setDrafts((list) => list.map((d, i) => (i === index ? { ...d, persona: change(d.persona) } : d)))
  }

  function create() {
    setError('')
    setCreating(true)
    applyPersonaAiRun(run.id, accepted(drafts))
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
    <Modal open title="Review the proposed personas" onClose={creating ? () => undefined : onClose} size="wide">
      {run.proposal?.note && <p className="mb-4 rounded-lg bg-purple-50 p-3 text-sm text-purple-900">{run.proposal.note}</p>}
      <p className="mb-4 text-sm text-gray-600">
        Leave out what you do not want and fix what is not right. Nothing is created until you press the button below.
      </p>

      <div className="space-y-5">
        {drafts.map((d, i) => {
          const hint = duplicateHint(d.persona, existing)
          return (
            <section
              key={i}
              aria-label={`Persona ${i + 1}`}
              className={`rounded-xl border p-4 ${d.include ? 'border-gray-200 bg-white' : 'border-gray-100 bg-gray-50 opacity-70'}`}
            >
              <label className="mb-3 flex items-center gap-2 text-sm font-medium text-gray-800">
                <input
                  type="checkbox"
                  checked={d.include}
                  onChange={(e) => {
                    setDrafts((list) => list.map((x, j) => (j === i ? { ...x, include: e.target.checked } : x)))
                  }}
                  className="h-4 w-4 rounded border-gray-300"
                />
                Create this persona
              </label>
              {hint && <p className="mb-3 rounded-md bg-amber-50 px-3 py-2 text-xs text-amber-900">{hint}</p>}

              <fieldset disabled={!d.include} className="space-y-3">
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
                        className="inline-flex items-center gap-1 rounded-full bg-gray-100 px-2.5 py-1 text-xs text-gray-800"
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

      {(error || problem) && <p className="mt-4 text-sm text-red-700">{error || problem}</p>}
      <div className="mt-5 flex justify-end gap-2">
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
          onClick={create}
          disabled={creating || problem !== null}
          className="rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white hover:bg-gray-800 disabled:opacity-50"
        >
          {creating ? 'Creating…' : createLabel(drafts)}
        </button>
      </div>
    </Modal>
  )
}

function RoleFields({ role, onChange, onRemove }: { role: ProposedRole; onChange: (r: ProposedRole) => void; onRemove: () => void }) {
  const field = (key: keyof ProposedRole, label: string) => (
    <label className="block text-xs font-medium text-gray-500">
      {label}
      <input
        value={role[key]}
        onChange={(e) => {
          onChange({ ...role, [key]: e.target.value })
        }}
        className={`mt-1 ${inputClass}`}
      />
    </label>
  )
  return (
    <div className="space-y-2">
      <div className="grid gap-2 sm:grid-cols-2">
        {field('company', 'Company')}
        {field('title', 'Title')}
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
