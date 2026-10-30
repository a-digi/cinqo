import { useEffect, useState } from 'react'
import { fetchSkills, addSkill, removeSkill } from '../../api'
import { PlusIcon, SparkIcon, XIcon } from '../../Shared/Icons/icons'
import { inputClass } from '../../Shared/inputClass'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { plural } from '../CompaniesPage/companyStats'
import { PAGE_ACCENT, accentFor } from '../JobsPage/jobsTheme'
import { LetterIndex } from './LetterIndex'
import { filterByLetter, letterIndex } from './skillRules'
import { PersonaSwitcher } from '../Persona/PersonaSwitcher/PersonaSwitcher'

// Its own page as of step 7 (moved out of ProfilePage). "Edit" for a
// skill is a rename, done as remove-old-then-add-new — a skill is a
// bare deduplicated string with no separate id an in-place update
// could target, so this is the simplest correct way to express it.
// Step 9 added the persona switcher — skills are per-persona as of
// step 8. See
// plan/ai/tools/career/step-07-dedicated-skills-and-experience-pages.md
// and plan/ai/tools/career/step-09-persona-frontend.md. Header card and a tinted
// card of coloured chips, with an A-Z index above them as its overview.
export function SkillsPage() {
  const [personaId, setPersonaId] = useState<string | null>(null)
  const [skills, setSkills] = useState<string[]>([])
  // The letter the index is filtering by, or null for all skills.
  const [activeLetter, setActiveLetter] = useState<string | null>(null)
  const [newSkill, setNewSkill] = useState('')
  const [editing, setEditing] = useState<string | null>(null)
  const [editValue, setEditValue] = useState('')
  const [error, setError] = useState('')

  function load(forPersonaId: string) {
    setError('')
    fetchSkills(forPersonaId)
      .then(setSkills)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
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
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  function handleRemove(skill: string) {
    if (!personaId) return
    setError('')
    removeSkill(personaId, skill)
      .then(() => {
        setSkills((prev) => prev.filter((s) => s !== skill))
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
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
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  const index = letterIndex(skills)
  // A letter whose skills are all gone (removed or renamed) stops filtering.
  const letter = activeLetter !== null && index.some((l) => l.letter === activeLetter && l.count > 0) ? activeLetter : null
  const visibleSkills = letter === null ? skills : filterByLetter(skills, letter)

  return (
    <div className="career-v-tilt career-a-glow career-t-wave max-w-7xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<SparkIcon />}
        title="Skills"
        description="The skills on this persona. The AI reads them when matching you against job postings."
        pill={personaId ? { text: plural(skills.length, 'skill', 'skills') } : undefined}
      />

      <PersonaSwitcher personaId={personaId} onChange={setPersonaId} />

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      {personaId && (
        <section
          className="career-enter career-delay-1 rounded-3xl border p-5 shadow-sm"
          style={{ borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }}
        >
          {skills.length > 0 && <LetterIndex index={index} active={letter} total={skills.length} onPick={setActiveLetter} />}
          {skills.length === 0 && <p className="mb-3 text-sm text-gray-500">No skills yet. Add the first one below.</p>}
          <div className="mb-4 flex flex-wrap gap-2">
            {visibleSkills.map((skill, idx) => {
              const accent = accentFor(skill)
              return editing === skill ? (
                <span
                  key={skill}
                  className="flex items-center gap-1.5 rounded-full border bg-white px-2 py-1"
                  style={{ borderColor: accent.to }}
                >
                  <input
                    // Deliberate: entering edit mode should focus the input
                    // immediately, the same convention most inline-rename UIs use.
                    // eslint-disable-next-line jsx-a11y/no-autofocus
                    autoFocus
                    value={editValue}
                    onChange={(e) => {
                      setEditValue(e.target.value)
                    }}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') saveEdit(skill)
                      if (e.key === 'Escape') cancelEdit()
                    }}
                    className="w-28 rounded border border-gray-300 px-1.5 py-0.5 text-xs text-gray-900 focus:border-gray-500 focus:outline-none"
                  />
                  <button
                    type="button"
                    aria-label="Save"
                    onClick={() => {
                      saveEdit(skill)
                    }}
                    className="text-xs font-medium text-gray-700 hover:text-gray-900"
                  >
                    Save
                  </button>
                  <button type="button" aria-label="Cancel" onClick={cancelEdit} className="text-gray-400 hover:text-red-700">
                    <XIcon />
                  </button>
                </span>
              ) : (
                <span
                  key={skill}
                  className="career-chip flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-medium"
                  style={{ background: accent.soft, color: accent.text, '--i': idx } as React.CSSProperties}
                >
                  <span aria-hidden="true" className="h-2 w-2 rounded-full" style={{ background: accent.to }} />
                  <button
                    type="button"
                    onClick={() => {
                      startEdit(skill)
                    }}
                    className="hover:underline"
                  >
                    {skill}
                  </button>
                  <button
                    type="button"
                    aria-label={`Remove ${skill}`}
                    onClick={() => {
                      handleRemove(skill)
                    }}
                    className="opacity-60 hover:text-red-700 hover:opacity-100"
                  >
                    <XIcon />
                  </button>
                </span>
              )
            })}
          </div>
          <div className="flex gap-2">
            <input
              value={newSkill}
              onChange={(e) => {
                setNewSkill(e.target.value)
              }}
              onKeyDown={(e) => {
                if (e.key === 'Enter') handleAdd()
              }}
              placeholder="Add a skill"
              className={`flex-1 ${inputClass}`}
            />
            <button
              type="button"
              onClick={handleAdd}
              className="flex items-center gap-1 rounded-md bg-gray-800 px-3.5 py-1.5 text-sm font-medium text-white transition-colors hover:bg-gray-900"
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
