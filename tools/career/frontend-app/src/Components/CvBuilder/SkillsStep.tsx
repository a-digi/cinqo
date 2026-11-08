import { useState } from 'react'
import { PlusIcon, XIcon } from '../../Shared/Icons/icons'
import type { CvStepProps } from './PersonalDetailsStep'

// Wizard step 3. See PersonalDetailsStep's own doc comment for the
// shared "local CvData only" contract every step here follows.
export function SkillsStep({ data, onChange }: CvStepProps) {
  const [newSkill, setNewSkill] = useState('')

  const addSkill = () => {
    const skill = newSkill.trim()
    if (!skill) return
    onChange({ ...data, skills: [...data.skills, skill] })
    setNewSkill('')
  }

  const removeSkill = (index: number) => {
    onChange({ ...data, skills: data.skills.filter((_, i) => i !== index) })
  }

  return (
    <div>
      <div className="mb-3 flex flex-wrap gap-1.5">
        {data.skills.map((skill, i) => (
          <span key={`${skill}-${i}`} className="inline-flex items-center gap-1 rounded-full bg-gray-100 px-2.5 py-1 text-xs text-gray-800">
            {skill}
            <button
              type="button"
              onClick={() => {
                removeSkill(i)
              }}
              className="text-gray-400 hover:text-gray-700"
              aria-label={`Remove ${skill}`}
            >
              <XIcon />
            </button>
          </span>
        ))}
        {data.skills.length === 0 && <p className="text-xs text-gray-400">No skills yet.</p>}
      </div>
      <div className="flex gap-1.5">
        <input
          type="text"
          value={newSkill}
          onChange={(e) => {
            setNewSkill(e.target.value)
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault()
              addSkill()
            }
          }}
          placeholder="Add a skill…"
          className="w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
        />
        <button
          type="button"
          onClick={addSkill}
          className="shrink-0 rounded-md bg-gray-900 px-2.5 py-1.5 text-white hover:bg-gray-800"
          aria-label="Add skill"
        >
          <PlusIcon />
        </button>
      </div>
    </div>
  )
}
