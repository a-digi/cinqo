import { useEffect, useState } from 'react'
import { fetchPersonas, updatePersona, deletePersona, type Persona } from '../../../api'
import { LayersIcon, PlusIcon } from '../../../Shared/Icons/icons'
import { inputClass } from '../../../Shared/inputClass'
import { PageHeaderCard } from '../../../Shared/PageHeader/PageHeaderCard'
import { NEW_PERSONA_PATH } from '../../../paths'
import { plural } from '../../CompaniesPage/companyStats'
import { PAGE_ACCENT, accentFor, initialOf } from '../../JobsPage/jobsTheme'
import { ProfileSwitcher } from '../../ProfileSwitcher/ProfileSwitcher'
import { NeedsAttention } from './NeedsAttention'
import { summarizePersonas, undescribedPersonas } from './personaStats'

// Full persona CRUD — since step 10, every persona belongs to a
// profile (job seeker), so this page now scopes its own list and its
// own create form to whichever profile is currently selected via
// ProfileSwitcher, the same "forced to be mapped" enforcement one
// level up from step 8's own personaId requirement. See
// plan/ai/tools/career/step-09-persona-frontend.md and
// plan/ai/tools/career/step-11-job-seeker-profile-frontend.md. Same layout as
// Companies, but mirrored: header card, the personas on the left and the "needs
// attention" panel on the right;
// "New persona" opens its own page.
export function PersonasPage() {
  const [profileId, setProfileId] = useState<string | null>(null)
  const [personas, setPersonas] = useState<Persona[]>([])
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editName, setEditName] = useState('')
  const [editDescription, setEditDescription] = useState('')
  const [error, setError] = useState('')

  function load(forProfileId: string) {
    setError('')
    fetchPersonas(forProfileId)
      .then(setPersonas)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  useEffect(() => {
    if (profileId) load(profileId)
  }, [profileId])

  function startEdit(p: Persona) {
    setEditingId(p.id)
    setEditName(p.name)
    setEditDescription(p.description ?? '')
  }

  function cancelEdit() {
    setEditingId(null)
    setEditName('')
    setEditDescription('')
  }

  function saveEdit(id: string) {
    const name = editName.trim()
    if (!name) {
      setError('Name is required')
      return
    }
    setError('')
    updatePersona(id, { name, description: editDescription.trim() })
      .then(() => {
        if (profileId) load(profileId)
        cancelEdit()
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  function handleDelete(id: string) {
    setError('')
    deletePersona(id)
      .then(() => {
        setPersonas((prev) => prev.filter((p) => p.id !== id))
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  const summary = summarizePersonas(personas)
  const undescribed = undescribedPersonas(personas)

  // The panel's "Add one": opens that persona's inline edit and brings it into view.
  function editFromPanel(id: string) {
    const persona = personas.find((p) => p.id === id)
    if (!persona) return
    startEdit(persona)
    requestAnimationFrame(() => {
      document.getElementById(`persona-${id}`)?.scrollIntoView({ behavior: 'smooth', block: 'center' })
    })
  }

  return (
    <div className="career-v-flip career-a-sheen career-t-wave max-w-7xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<LayersIcon />}
        title="Personas"
        description={
          'One persona per role a profile is pursuing, e.g. "Backend Engineer". Each has its own summary, skills and experience.'
        }
        pill={{ text: plural(summary.personas, 'persona', 'personas') }}
      />

      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <div className="min-w-0 flex-1">
          <ProfileSwitcher profileId={profileId} onChange={setProfileId} />

          {profileId && (
            <div className="mb-3 flex items-center justify-start gap-3">
              <button
                type="button"
                onClick={() => {
                  window.__cinqoToolBridge.navigate(`${NEW_PERSONA_PATH}?profileId=${encodeURIComponent(profileId)}`)
                }}
                className="flex shrink-0 items-center gap-1.5 rounded-md bg-gray-800 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-900"
              >
                <PlusIcon />
                New persona
              </button>
              <p className="min-w-0 flex-1 text-xs text-gray-500">
                Deleting a persona permanently deletes its details, every skill and every experience entry, with no further confirmation.
              </p>
            </div>
          )}

          <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

          {profileId && personas.length === 0 && !error && (
            <div
              className="career-wiggle rounded-3xl border p-8 text-center text-sm text-gray-500"
              style={{ borderColor: PAGE_ACCENT.soft, background: PAGE_ACCENT.soft }}
            >
              This profile has no personas yet. Create the first one with “New persona”.
            </div>
          )}

          {profileId && (
            <div className="career-stagger space-y-3">
              {personas.map((p) => (
                <div
                  key={p.id}
                  id={`persona-${p.id}`}
                  className="rounded-3xl border p-4 shadow-sm transition-shadow hover:shadow-md"
                  style={{ borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }}
                >
                  {editingId === p.id ? (
                    <div>
                      <input
                        // Deliberate: entering edit mode should focus the input
                        // immediately, the same convention most inline-rename UIs use.
                        // eslint-disable-next-line jsx-a11y/no-autofocus
                        autoFocus
                        value={editName}
                        onChange={(e) => {
                          setEditName(e.target.value)
                        }}
                        placeholder="Name"
                        className={`mb-2 ${inputClass}`}
                      />
                      <input
                        value={editDescription}
                        onChange={(e) => {
                          setEditDescription(e.target.value)
                        }}
                        placeholder="Description"
                        className={`mb-2 ${inputClass}`}
                      />
                      <div className="flex gap-2">
                        <button
                          type="button"
                          onClick={() => {
                            saveEdit(p.id)
                          }}
                          className="rounded-md bg-gray-900 px-3 py-1 text-xs font-medium text-white hover:bg-gray-800"
                        >
                          Save
                        </button>
                        <button
                          type="button"
                          onClick={cancelEdit}
                          className="rounded-md border border-gray-200 px-3 py-1 text-xs text-gray-700 hover:bg-gray-50"
                        >
                          Cancel
                        </button>
                      </div>
                    </div>
                  ) : (
                    <div className="flex items-start gap-3">
                      <span
                        className="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-base font-bold text-white shadow-sm"
                        style={{ background: accentFor(p.name).to }}
                        aria-hidden="true"
                      >
                        {initialOf(p.name)}
                      </span>
                      <div className="min-w-0 flex-1">
                        <div className="text-sm font-semibold">{p.name}</div>
                        {p.description && <div className="mt-0.5 text-xs text-gray-500">{p.description}</div>}
                        <div className="mt-1 text-xs text-gray-400">Created {formatDate(p.createdAt)}</div>
                      </div>
                      <div className="flex shrink-0 gap-2">
                        <button
                          type="button"
                          onClick={() => {
                            startEdit(p)
                          }}
                          className="rounded-full border border-blue-200 px-3 py-1 text-xs text-blue-700 hover:bg-blue-50"
                        >
                          Edit
                        </button>
                        <button
                          type="button"
                          onClick={() => {
                            handleDelete(p.id)
                          }}
                          className="rounded-full border border-red-200 px-3 py-1 text-xs text-red-700 hover:bg-red-50"
                        >
                          Delete
                        </button>
                      </div>
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>

        {profileId && (
          <aside className="w-full shrink-0 lg:sticky lg:top-6 lg:w-72">
            <NeedsAttention total={summary.personas} undescribed={undescribed} onEdit={editFromPanel} />
          </aside>
        )}
      </div>
    </div>
  )
}

// The created time as a local date, or as given when it is not one.
function formatDate(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleDateString()
}
