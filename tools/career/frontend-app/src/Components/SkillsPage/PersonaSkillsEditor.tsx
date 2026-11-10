import { useEffect, useState } from 'react'
import { addSkill, fetchPersonaSkillsView, removeSkill, setPersonaSkillIds, type PersonaSkillsView } from '../../api'
import { PlusIcon, XIcon } from '../../Shared/Icons/icons'
import { inputClass } from '../../Shared/inputClass'
import { SKILLS_PATH } from '../../paths'
import { PAGE_ACCENT, accentFor } from '../JobsPage/jobsTheme'
import { buildProfileSearch, skillTextOk } from '../ProfileLibrary/libraryRules'
import { toggleId, usesText } from '../ProfileLibrary/personaUseRules'

// One persona's skills (plan 173): the skills of the profile's library it USES (a checklist: each is chosen or not), and the skills that
// belong only to this persona (its own: add, rename, remove). The two are marked apart. onCount tells the page around it how many
// skills the persona shows (for its header). The library itself is edited on the Skills page.
export function PersonaSkillsEditor({
  personaId,
  profileId,
  onCount,
}: {
  personaId: string
  profileId: string
  onCount?: (count: number) => void
}) {
  const [view, setView] = useState<PersonaSkillsView | null>(null)
  const [newSkill, setNewSkill] = useState('')
  const [editing, setEditing] = useState<string | null>(null)
  const [editValue, setEditValue] = useState('')
  const [error, setError] = useState('')

  function load() {
    setError('')
    fetchPersonaSkillsView(personaId)
      .then(setView)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [personaId])

  function fail(err: unknown) {
    setError(err instanceof Error ? err.message : String(err))
  }

  function toggle(skillId: string, on: boolean) {
    if (!view) return
    setError('')
    setPersonaSkillIds(personaId, toggleId(view.used, skillId, on))
      .then(setView)
      .catch(fail)
  }

  function handleAdd() {
    if (!skillTextOk(newSkill)) return
    setError('')
    addSkill(personaId, newSkill.trim())
      .then(() => {
        setNewSkill('')
        load()
      })
      .catch(fail)
  }

  function handleRemove(skill: string) {
    setError('')
    removeSkill(personaId, skill).then(load).catch(fail)
  }

  function saveEdit(oldSkill: string) {
    const text = editValue.trim()
    if (!skillTextOk(text) || text === oldSkill) {
      setEditing(null)
      return
    }
    setError('')
    addSkill(personaId, text)
      .then(() => removeSkill(personaId, oldSkill))
      .then(() => {
        setEditing(null)
        load()
      })
      .catch(fail)
  }

  useEffect(() => {
    if (view) onCount?.(view.used.length + view.own.length)
  }, [view, onCount])

  const cardStyle = { borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }

  return (
    <>
      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      <section className="career-enter career-delay-1 mb-5 rounded-3xl border p-5 shadow-sm" style={cardStyle}>
        <div className="mb-3 flex flex-wrap items-baseline justify-between gap-2">
          <h2 className="text-sm font-semibold text-gray-800">From the profile’s library</h2>
          {view && <span className="text-xs text-gray-500">{usesText(view.used.length, view.library.length, 'skill', 'skills')}</span>}
        </div>
        {view?.library.length === 0 && (
          <p className="text-sm text-gray-500">
            The profile has no skills yet.{' '}
            <a
              href={`${SKILLS_PATH}${buildProfileSearch(profileId)}`}
              onClick={(e) => {
                e.preventDefault()
                window.__cinqoToolBridge.navigate(`${SKILLS_PATH}${buildProfileSearch(profileId)}`)
              }}
              className="font-medium text-blue-700 hover:underline"
            >
              Add them to the library
            </a>{' '}
            and choose here which ones this target role uses.
          </p>
        )}
        <div className="flex flex-wrap gap-2">
          {view?.library.map((skill) => {
            const accent = accentFor(skill.skill)
            const on = view.used.includes(skill.id)
            return (
              <button
                key={skill.id}
                type="button"
                role="checkbox"
                aria-checked={on}
                title={on ? 'Used by this target role: click to stop using it' : 'Not used by this target role: click to use it'}
                onClick={() => {
                  toggle(skill.id, !on)
                }}
                className="flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs font-medium transition-colors"
                style={
                  on
                    ? { background: accent.soft, color: accent.text, borderColor: accent.to }
                    : { background: '#fff', color: '#6b7280', borderColor: '#e5e7eb' }
                }
              >
                <span
                  aria-hidden="true"
                  className="inline-flex h-3.5 w-3.5 items-center justify-center rounded-sm border text-[10px]"
                  style={{ borderColor: on ? accent.to : '#d1d5db', background: on ? accent.to : '#fff', color: '#fff' }}
                >
                  {on ? '✓' : ''}
                </span>
                {skill.skill}
              </button>
            )
          })}
        </div>
      </section>

      <section className="career-enter career-delay-2 rounded-3xl border p-5 shadow-sm" style={cardStyle}>
        <div className="mb-3 flex flex-wrap items-baseline justify-between gap-2">
          <h2 className="text-sm font-semibold text-gray-800">Only on this target role</h2>
          {view && (
            <span className="text-xs text-gray-500">
              {view.own.length === 1 ? '1 skill of its own' : `${String(view.own.length)} skills of its own`}
            </span>
          )}
        </div>
        {view?.own.length === 0 && (
          <p className="mb-3 text-sm text-gray-500">
            No skills of its own yet. Add one below if this role needs a skill the profile does not list.
          </p>
        )}
        <div className="mb-4 flex flex-wrap gap-2">
          {view?.own.map((item, idx) => {
            const accent = accentFor(item.skill)
            return editing === item.skill ? (
              <span
                key={item.id}
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
                    if (e.key === 'Enter') saveEdit(item.skill)
                    if (e.key === 'Escape') setEditing(null)
                  }}
                  className="w-28 rounded border border-gray-300 px-1.5 py-0.5 text-xs text-gray-900 focus:border-gray-500 focus:outline-none"
                />
                <button
                  type="button"
                  aria-label="Save"
                  onClick={() => {
                    saveEdit(item.skill)
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
                key={item.id}
                className="career-chip flex items-center gap-1.5 rounded-full border border-dashed px-3 py-1 text-xs font-medium"
                style={{ background: accent.soft, color: accent.text, borderColor: accent.to, '--i': idx } as React.CSSProperties}
              >
                <button
                  type="button"
                  onClick={() => {
                    setEditing(item.skill)
                    setEditValue(item.skill)
                  }}
                  className="hover:underline"
                >
                  {item.skill}
                </button>
                <button
                  type="button"
                  aria-label={`Remove ${item.skill}`}
                  onClick={() => {
                    handleRemove(item.skill)
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
            placeholder="Add a skill to this target role only"
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
    </>
  )
}
