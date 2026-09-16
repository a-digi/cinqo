import { useEffect, useState } from 'react'
import { fetchSkills, addSkill, removeSkill } from '../../api'
import { PlusIcon, XIcon } from '../../icons'
import { PersonaSwitcher } from '../PersonaSwitcher/PersonaSwitcher'

// Its own page as of step 7 (moved out of ProfilePage). "Edit" for a
// skill is a rename, done as remove-old-then-add-new — a skill is a
// bare deduplicated string with no separate id an in-place update
// could target, so this is the simplest correct way to express it.
// Step 9 added the persona switcher — skills are per-persona as of
// step 8. See
// plan/ai/tools/career/step-07-dedicated-skills-and-experience-pages.md
// and plan/ai/tools/career/step-09-persona-frontend.md.
export function SkillsPage() {
  const [personaId, setPersonaId] = useState<string | null>(null)
  const [skills, setSkills] = useState<string[]>([])
  const [newSkill, setNewSkill] = useState('')
  const [editing, setEditing] = useState<string | null>(null)
  const [editValue, setEditValue] = useState('')
  const [error, setError] = useState('')

  function load(forPersonaId: string) {
    setError('')
    fetchSkills(forPersonaId)
      .then(setSkills)
      .catch((err: Error) => setError(err.message))
  }

  useEffect(() => {
    if (personaId) load(personaId)
  }, [personaId])

  function handleAdd() {
    if (!personaId) return
    const skill = newSkill.trim()
    if (!skill) return
    setError('')
    addSkill(personaId, skill)
      .then((result) => {
        setSkills(result)
        setNewSkill('')
      })
      .catch((err: Error) => setError(err.message))
  }

  function handleRemove(skill: string) {
    if (!personaId) return
    setError('')
    removeSkill(personaId, skill)
      .then(() => setSkills((prev) => prev.filter((s) => s !== skill)))
      .catch((err: Error) => setError(err.message))
  }

  function startEdit(skill: string) {
    setEditing(skill)
    setEditValue(skill)
  }

  function cancelEdit() {
    setEditing(null)
    setEditValue('')
  }

  function saveEdit(oldSkill: string) {
    if (!personaId) return
    const newValue = editValue.trim()
    if (!newValue || newValue === oldSkill) {
      cancelEdit()
      return
    }
    setError('')
    addSkill(personaId, newValue)
      .then(() => removeSkill(personaId, oldSkill))
      .then(() => {
        setSkills((prev) => prev.map((s) => (s === oldSkill ? newValue : s)))
        cancelEdit()
      })
      .catch((err: Error) => setError(err.message))
  }

  return (
    <div className="max-w-2xl p-6 font-sans text-gray-900">
      <h1 className="mb-1.5 text-xl font-semibold">Skills</h1>
      <p className="mb-5 text-sm text-gray-500">
        Skills on this persona's own career profile — what the AI reads when matching you against
        job postings.
      </p>

      <PersonaSwitcher personaId={personaId} onChange={setPersonaId} />

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      {personaId && (
        <section className="rounded-md border border-gray-200 p-4 shadow-sm">
          <div className="mb-3 flex flex-wrap gap-2">
            {skills.map((skill) =>
              editing === skill ? (
                <span key={skill} className="flex items-center gap-1.5 rounded-full bg-gray-100 px-2 py-1">
                  <input
                    autoFocus
                    value={editValue}
                    onChange={(e) => setEditValue(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') saveEdit(skill)
                      if (e.key === 'Escape') cancelEdit()
                    }}
                    className="w-28 rounded border border-gray-300 px-1.5 py-0.5 text-xs text-gray-900 focus:border-gray-500 focus:outline-none"
                  />
                  <button
                    type="button"
                    aria-label="Save"
                    onClick={() => saveEdit(skill)}
                    className="text-xs font-medium text-gray-700 hover:text-gray-900"
                  >
                    Save
                  </button>
                  <button
                    type="button"
                    aria-label="Cancel"
                    onClick={cancelEdit}
                    className="text-gray-400 hover:text-red-700"
                  >
                    <XIcon />
                  </button>
                </span>
              ) : (
                <span
                  key={skill}
                  className="flex items-center gap-1.5 rounded-full bg-gray-100 px-3 py-1 text-xs text-gray-700"
                >
                  <button type="button" onClick={() => startEdit(skill)} className="hover:underline">
                    {skill}
                  </button>
                  <button
                    type="button"
                    aria-label={`Remove ${skill}`}
                    onClick={() => handleRemove(skill)}
                    className="text-gray-400 hover:text-red-700"
                  >
                    <XIcon />
                  </button>
                </span>
              ),
            )}
          </div>
          <div className="flex gap-2">
            <input
              value={newSkill}
              onChange={(e) => setNewSkill(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleAdd()}
              placeholder="Add a skill"
              className="flex-1 rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
            />
            <button
              type="button"
              onClick={handleAdd}
              className="flex items-center gap-1 rounded-md border border-gray-200 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-50"
            >
              <PlusIcon />
              Add
            </button>
          </div>
        </section>
      )}
    </div>
  )
}
