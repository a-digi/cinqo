import { useState } from 'react'
import type { ProposedPersona, ProposedRole } from '../api'
import { inputClass } from '../Shared/inputClass'
import { XIcon } from '../Shared/Icons/icons'
import { isNewCompany, isNewSkill, isNewTitle, type KnownData } from './knownData'
import { MAX_SHORT } from './personaProposal'
import { addSkill, removeAt } from './reviewRules'

// The form to fix a suggested persona before accepting it: name, description, details, skills and
// experience. Items that are not in the user's own data are marked amber.
export function PersonaEditForm({
  persona,
  known,
  onChange,
}: {
  persona: ProposedPersona
  known: KnownData | null
  onChange: (change: (p: ProposedPersona) => ProposedPersona) => void
}) {
  const [skillText, setSkillText] = useState('')

  const detail = (field: keyof ProposedPersona['details'], label: string) => (
    <label className="block text-xs font-medium text-gray-500">
      {label}
      <input
        value={String(persona.details[field])}
        onChange={(e) => {
          onChange((p) => ({
            ...p,
            details: { ...p.details, [field]: field === 'minSalary' ? Math.max(0, Number(e.target.value) || 0) : e.target.value },
          }))
        }}
        type={field === 'minSalary' ? 'number' : 'text'}
        className={`mt-1 ${inputClass}`}
      />
    </label>
  )

  function add() {
    onChange((p) => ({ ...p, skills: addSkill(p.skills, skillText) }))
    setSkillText('')
  }

  return (
    <div className="space-y-3">
      <div className="grid gap-3 sm:grid-cols-2">
        <label className="block text-xs font-medium text-gray-500">
          Name
          <input
            value={persona.name}
            onChange={(e) => {
              onChange((p) => ({ ...p, name: e.target.value }))
            }}
            className={`mt-1 ${inputClass}`}
          />
        </label>
        <label className="block text-xs font-medium text-gray-500">
          Description
          <input
            value={persona.description}
            onChange={(e) => {
              onChange((p) => ({ ...p, description: e.target.value }))
            }}
            className={`mt-1 ${inputClass}`}
          />
        </label>
        {detail('headline', 'Headline')}
        {detail('location', 'Location')}
        {detail('desiredTitles', 'Desired titles')}
        {detail('desiredLocations', 'Desired locations')}
        {detail('minSalary', 'Minimum salary')}
      </div>
      <label className="block text-xs font-medium text-gray-500">
        Summary
        <textarea
          value={persona.details.summary}
          onChange={(e) => {
            onChange((p) => ({ ...p, details: { ...p.details, summary: e.target.value } }))
          }}
          rows={3}
          className={`mt-1 ${inputClass}`}
        />
      </label>

      <div>
        <p className="mb-1.5 text-xs font-medium text-gray-500">Skills</p>
        <div className="flex flex-wrap gap-1.5">
          {persona.skills.map((skill, s) => (
            <span
              key={skill}
              title={known && isNewSkill(known, skill) ? 'Not in your data' : undefined}
              className={`inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-xs ${
                known && isNewSkill(known, skill) ? 'bg-amber-100 text-amber-900 ring-1 ring-amber-300' : 'bg-gray-100 text-gray-800'
              }`}
            >
              {skill}
              <button
                type="button"
                aria-label={`Remove ${skill}`}
                onClick={() => {
                  onChange((p) => ({ ...p, skills: removeAt(p.skills, s) }))
                }}
                className="text-gray-400 hover:text-red-700"
              >
                <XIcon />
              </button>
            </span>
          ))}
          {persona.skills.length === 0 && <span className="text-xs text-gray-400">No skills.</span>}
        </div>
        <div className="mt-2 flex gap-2">
          <input
            value={skillText}
            onChange={(e) => {
              setSkillText(e.target.value)
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                add()
              }
            }}
            maxLength={MAX_SHORT}
            placeholder="Add a skill"
            aria-label="Add a skill"
            className={`max-w-xs ${inputClass}`}
          />
          <button
            type="button"
            onClick={add}
            className="rounded-md border border-gray-200 px-3 py-1.5 text-xs text-gray-700 hover:bg-gray-50"
          >
            Add
          </button>
        </div>
      </div>

      {persona.experience.length > 0 && (
        <div>
          <p className="mb-1.5 text-xs font-medium text-gray-500">Experience</p>
          <ul className="space-y-2">
            {persona.experience.map((role, r) => (
              <li key={r} className="rounded-lg border border-gray-100 p-3">
                <RoleFields
                  newCompany={known !== null && isNewCompany(known, role.company)}
                  newTitle={known !== null && isNewTitle(known, role.title)}
                  role={role}
                  onChange={(next) => {
                    onChange((p) => ({ ...p, experience: p.experience.map((x, k) => (k === r ? next : x)) }))
                  }}
                  onRemove={() => {
                    onChange((p) => ({ ...p, experience: removeAt(p.experience, r) }))
                  }}
                />
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
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
