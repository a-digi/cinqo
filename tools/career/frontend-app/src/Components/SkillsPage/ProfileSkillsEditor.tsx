import { useEffect, useState } from 'react'
import { addProfileSkill, fetchProfileSkills, removeProfileSkill, renameProfileSkill, type ProfileSkill } from '../../api'
import { PlusIcon, XIcon } from '../../Shared/Icons/icons'
import { inputClass } from '../../Shared/inputClass'
import { PAGE_ACCENT, accentFor } from '../JobsPage/jobsTheme'
import { removeWarning, skillTextOk, usedByText } from '../ProfileLibrary/libraryRules'
import { LetterIndex } from './LetterIndex'
import { filterByLetter, letterIndex } from './skillRules'

// The profile's library of skills (plan 173): global to the profile, shared by its personas; each persona uses some of them. The A-Z
// index, the coloured chips (click a chip to rename it, the x removes it; a skill personas use asks first), and the field to add one.
// onCount tells the page around it how many skills there are (for its header).
export function ProfileSkillsEditor({ profileId, onCount }: { profileId: string; onCount?: (count: number) => void }) {
  const [skills, setSkills] = useState<ProfileSkill[]>([])
  const [activeLetter, setActiveLetter] = useState<string | null>(null)
  const [newSkill, setNewSkill] = useState('')
  const [editing, setEditing] = useState<string | null>(null)
  const [editValue, setEditValue] = useState('')
  // The skill whose removal waits for a yes because personas use it.
  const [confirming, setConfirming] = useState<string | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    setError('')
    setConfirming(null)
    fetchProfileSkills(profileId)
      .then(setSkills)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }, [profileId])

  function fail(err: unknown) {
    setError(err instanceof Error ? err.message : String(err))
  }

  function handleAdd() {
    if (!skillTextOk(newSkill)) return
    setError('')
    addProfileSkill(profileId, newSkill.trim())
      .then((list) => {
        setSkills(list)
        setNewSkill('')
      })
      .catch(fail)
  }

  function doRemove(skill: ProfileSkill) {
    setError('')
    setConfirming(null)
    removeProfileSkill(skill.id)
      .then(() => {
        setSkills((prev) => prev.filter((s) => s.id !== skill.id))
      })
      .catch(fail)
  }

  function askRemove(skill: ProfileSkill) {
    if (removeWarning(skill.usedBy) === null) doRemove(skill)
    else setConfirming(skill.id)
  }

  function saveEdit(skill: ProfileSkill) {
    const text = editValue.trim()
    if (!skillTextOk(text) || text === skill.skill) {
      setEditing(null)
      return
    }
    setError('')
    renameProfileSkill(skill.id, text)
      .then((list) => {
        setSkills(list)
        setEditing(null)
      })
      .catch(fail)
  }

  const names = skills.map((s) => s.skill)
  const index = letterIndex(names)
  const letter = activeLetter !== null && index.some((l) => l.letter === activeLetter && l.count > 0) ? activeLetter : null
  const visibleNames = letter === null ? names : filterByLetter(names, letter)
  const visible = skills.filter((s) => visibleNames.includes(s.skill))
  const pending = skills.find((s) => s.id === confirming)

  useEffect(() => {
    onCount?.(skills.length)
  }, [skills.length, onCount])

  return (
    <>
      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>
      <section
        className="career-enter career-delay-1 rounded-3xl border p-5 shadow-sm"
        style={{ borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }}
      >
        {skills.length > 0 && <LetterIndex index={index} active={letter} total={skills.length} onPick={setActiveLetter} />}
        {skills.length === 0 && <p className="mb-3 text-sm text-gray-500">No skills in the library yet. Add the first one below.</p>}
        {pending && (
          <div
            role="alert"
            className="mb-3 flex flex-wrap items-center gap-2 rounded-2xl border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-900"
          >
            <span className="min-w-0 flex-1">
              <strong>{pending.skill}</strong>: {removeWarning(pending.usedBy)}
            </span>
            <button
              type="button"
              onClick={() => {
                doRemove(pending)
              }}
              className="rounded-md bg-red-700 px-2.5 py-1 font-medium text-white hover:bg-red-800"
            >
              Remove
            </button>
            <button
              type="button"
              onClick={() => {
                setConfirming(null)
              }}
              className="rounded-md border border-gray-300 bg-white px-2.5 py-1 text-gray-700 hover:bg-gray-50"
            >
              Keep
            </button>
          </div>
        )}
        <div className="mb-4 flex flex-wrap gap-2">
          {visible.map((skill, idx) => {
            const accent = accentFor(skill.skill)
            return editing === skill.id ? (
              <span
                key={skill.id}
                className="flex items-center gap-1.5 rounded-full border bg-white px-2 py-1"
                style={{ borderColor: accent.to }}
              >
                <input
                  // Deliberate: entering edit mode should focus the input immediately, like the other inline renames.
                  // eslint-disable-next-line jsx-a11y/no-autofocus
                  autoFocus
                  value={editValue}
                  onChange={(e) => {
                    setEditValue(e.target.value)
                  }}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') saveEdit(skill)
                    if (e.key === 'Escape') setEditing(null)
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
                <button
                  type="button"
                  aria-label="Cancel"
                  onClick={() => {
                    setEditing(null)
                  }}
                  className="text-gray-400 hover:text-red-700"
                >
                  <XIcon />
                </button>
              </span>
            ) : (
              <span
                key={skill.id}
                title={usedByText(skill.usedBy)}
                className="career-chip flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-medium"
                style={{ background: accent.soft, color: accent.text, '--i': idx } as React.CSSProperties}
              >
                <span aria-hidden="true" className="h-2 w-2 rounded-full" style={{ background: accent.to }} />
                <button
                  type="button"
                  onClick={() => {
                    setEditing(skill.id)
                    setEditValue(skill.skill)
                  }}
                  className="hover:underline"
                >
                  {skill.skill}
                </button>
                {skill.usedBy > 0 && <span className="rounded-full bg-white/60 px-1.5 text-[10px]">{skill.usedBy}</span>}
                <button
                  type="button"
                  aria-label={`Remove ${skill.skill}`}
                  onClick={() => {
                    askRemove(skill)
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
            placeholder="Add a skill to the library"
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
        <p className="mt-3 text-xs text-gray-500">
          The number on a skill is how many target roles use it. Each target role chooses its skills from this library.
        </p>
      </section>
    </>
  )
}
