import { useEffect, useState } from 'react'
import { fetchPersonas, createPersona, updatePersona, deletePersona, type Persona } from '../../../api'
import { PlusIcon } from '../../../Shared/Icons/icons'
import { ProfileSwitcher } from '../../ProfileSwitcher/ProfileSwitcher'

// Full persona CRUD — since step 10, every persona belongs to a
// profile (job seeker), so this page now scopes its own list and its
// own create form to whichever profile is currently selected via
// ProfileSwitcher, the same "forced to be mapped" enforcement one
// level up from step 8's own personaId requirement. See
// plan/ai/tools/career/step-09-persona-frontend.md and
// plan/ai/tools/career/step-11-job-seeker-profile-frontend.md.
export function PersonasPage() {
  const [profileId, setProfileId] = useState<string | null>(null)
  const [personas, setPersonas] = useState<Persona[]>([])
  const [newName, setNewName] = useState('')
  const [newDescription, setNewDescription] = useState('')
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editName, setEditName] = useState('')
  const [editDescription, setEditDescription] = useState('')
  const [error, setError] = useState('')

  function load(forProfileId: string) {
    setError('')
    fetchPersonas(forProfileId)
      .then(setPersonas)
      .catch((err: Error) => setError(err.message))
  }

  useEffect(() => {
    if (profileId) load(profileId)
  }, [profileId])

  function handleCreate() {
    if (!profileId) return
    const name = newName.trim()
    if (!name) {
      setError('Name is required')
      return
    }
    setError('')
    createPersona(profileId, name, newDescription.trim() || undefined)
      .then(() => {
        load(profileId)
        setNewName('')
        setNewDescription('')
      })
      .catch((err: Error) => setError(err.message))
  }

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
      .catch((err: Error) => setError(err.message))
  }

  function handleDelete(id: string) {
    setError('')
    deletePersona(id)
      .then(() => setPersonas((prev) => prev.filter((p) => p.id !== id)))
      .catch((err: Error) => setError(err.message))
  }

  return (
    <div className="max-w-2xl p-6 font-sans text-gray-900">
      <h1 className="mb-1.5 text-xl font-semibold">Personas</h1>
      <p className="mb-5 text-sm text-gray-500">
        Each persona owns its own details, skills, and experience — use separate personas for
        different roles this profile is pursuing (e.g. "Backend Engineer", "Freelance
        Consultant"). Deleting a persona permanently deletes its own details, every skill, and
        every experience entry — there's no separate confirmation step.
      </p>

      <ProfileSwitcher profileId={profileId} onChange={setProfileId} />

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      {profileId && (
        <>
          <div className="mb-4 space-y-2">
            {personas.map((p) =>
              editingId === p.id ? (
                <div key={p.id} className="rounded-md border border-gray-200 p-3">
                  <input
                    autoFocus
                    value={editName}
                    onChange={(e) => setEditName(e.target.value)}
                    placeholder="Name"
                    className="mb-2 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
                  />
                  <input
                    value={editDescription}
                    onChange={(e) => setEditDescription(e.target.value)}
                    placeholder="Description"
                    className="mb-2 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
                  />
                  <div className="flex gap-2">
                    <button
                      type="button"
                      onClick={() => saveEdit(p.id)}
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
                <div key={p.id} className="flex items-start justify-between gap-3 rounded-md border border-gray-200 p-3">
                  <div>
                    <div className="text-sm font-medium">{p.name}</div>
                    {p.description && <div className="text-xs text-gray-500">{p.description}</div>}
                    <div className="text-xs text-gray-400">Created {p.createdAt}</div>
                  </div>
                  <div className="flex shrink-0 gap-2">
                    <button
                      type="button"
                      onClick={() => startEdit(p)}
                      className="rounded-md border border-gray-200 px-2.5 py-1 text-xs text-gray-700 hover:bg-gray-50"
                    >
                      Edit
                    </button>
                    <button
                      type="button"
                      onClick={() => handleDelete(p.id)}
                      className="rounded-md border border-gray-200 px-2.5 py-1 text-xs text-red-700 hover:bg-red-50"
                    >
                      Delete
                    </button>
                  </div>
                </div>
              ),
            )}
          </div>

          <section className="rounded-md border border-gray-200 p-4 shadow-sm">
            <h2 className="mb-3 text-sm font-semibold">New persona</h2>
            <input
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              placeholder={'Name, e.g. "Backend Engineer"'}
              className="mb-2 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
            />
            <input
              value={newDescription}
              onChange={(e) => setNewDescription(e.target.value)}
              placeholder="Description (optional)"
              className="mb-2 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
            />
            <button
              type="button"
              onClick={handleCreate}
              className="flex items-center gap-1 rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800"
            >
              <PlusIcon />
              Create persona
            </button>
          </section>
        </>
      )}
    </div>
  )
}
