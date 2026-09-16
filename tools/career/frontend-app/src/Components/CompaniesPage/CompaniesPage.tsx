import { useEffect, useState } from 'react'
import { fetchCompanies, createCompany, updateCompany, removeCompany, type Company } from '../../api'
import { PlusIcon } from '../../icons'

const JOBS_PATH = '/tools/career/jobs'
const RECRUITERS_PATH = '/tools/career/recruiters'

// Companies are tool-wide, not persona/profile-scoped — same reasoning
// JobsPage itself already documents, since a company is about the job
// market, not the job seeker's own identity. No switcher needed here.
// Step 17 added recruiterCount alongside jobCount, linking to
// RecruitersPage the same one-way ?companyId= pre-filter way. See
// plan/ai/tools/career/step-15-companies-frontend.md and
// plan/ai/tools/career/step-17-recruiters-frontend.md.
export function CompaniesPage() {
  const [companies, setCompanies] = useState<Company[]>([])
  const [newName, setNewName] = useState('')
  const [newDescription, setNewDescription] = useState('')
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editName, setEditName] = useState('')
  const [editDescription, setEditDescription] = useState('')
  const [error, setError] = useState('')

  function load() {
    setError('')
    fetchCompanies()
      .then(setCompanies)
      .catch((err: Error) => setError(err.message))
  }

  useEffect(() => {
    load()
  }, [])

  function handleCreate() {
    const name = newName.trim()
    if (!name) {
      setError('Name is required')
      return
    }
    setError('')
    createCompany(name, newDescription.trim())
      .then((result) => {
        setCompanies(result)
        setNewName('')
        setNewDescription('')
      })
      .catch((err: Error) => setError(err.message))
  }

  function startEdit(c: Company) {
    setEditingId(c.id)
    setEditName(c.name)
    setEditDescription(c.description)
  }

  function cancelEdit() {
    setEditingId(null)
    setEditName('')
    setEditDescription('')
  }

  function saveEdit(id: string) {
    setError('')
    updateCompany(id, { name: editName.trim(), description: editDescription.trim() })
      .then((result) => {
        setCompanies(result)
        cancelEdit()
      })
      .catch((err: Error) => setError(err.message))
  }

  function handleDelete(id: string) {
    setError('')
    removeCompany(id)
      .then(() => setCompanies((prev) => prev.filter((c) => c.id !== id)))
      .catch((err: Error) => setError(err.message))
  }

  function navigateToJobs(e: React.MouseEvent, companyId: string) {
    e.preventDefault()
    window.__cinqoToolBridge.navigate(`${JOBS_PATH}?companyId=${encodeURIComponent(companyId)}`)
  }

  function navigateToRecruiters(e: React.MouseEvent, companyId: string) {
    e.preventDefault()
    window.__cinqoToolBridge.navigate(`${RECRUITERS_PATH}?companyId=${encodeURIComponent(companyId)}`)
  }

  return (
    <div className="max-w-2xl p-6 font-sans text-gray-900">
      <h1 className="mb-1.5 text-xl font-semibold">Companies</h1>
      <p className="mb-5 text-sm text-gray-500">
        A curated directory of companies, separate from the free-text company name on each crawled
        job. Link a job to a company from the Jobs page. Deleting a company un-links its jobs — it
        never deletes them.
      </p>

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      <div className="mb-4 space-y-3">
        {companies.map((c) => (
          <div key={c.id} className="rounded-md border border-gray-200 p-3">
            {editingId === c.id ? (
              <div>
                <input
                  autoFocus
                  value={editName}
                  onChange={(e) => setEditName(e.target.value)}
                  placeholder="Name"
                  className="mb-2 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
                />
                <textarea
                  value={editDescription}
                  onChange={(e) => setEditDescription(e.target.value)}
                  placeholder="Description"
                  rows={2}
                  className="mb-2 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
                />
                <div className="flex gap-2">
                  <button
                    type="button"
                    onClick={() => saveEdit(c.id)}
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
              <div className="flex items-start justify-between gap-3">
                <div>
                  <div className="text-sm font-medium">{c.name}</div>
                  {c.description && <div className="mt-0.5 text-xs text-gray-500">{c.description}</div>}
                  <div className="mt-1 flex gap-3">
                    <a
                      href={`${JOBS_PATH}?companyId=${encodeURIComponent(c.id)}`}
                      onClick={(e) => navigateToJobs(e, c.id)}
                      className="text-xs text-gray-500 underline hover:text-gray-700"
                    >
                      {c.jobCount} linked job{c.jobCount === 1 ? '' : 's'}
                    </a>
                    <a
                      href={`${RECRUITERS_PATH}?companyId=${encodeURIComponent(c.id)}`}
                      onClick={(e) => navigateToRecruiters(e, c.id)}
                      className="text-xs text-gray-500 underline hover:text-gray-700"
                    >
                      {c.recruiterCount} recruiter{c.recruiterCount === 1 ? '' : 's'}
                    </a>
                  </div>
                </div>
                <div className="flex shrink-0 gap-2">
                  <button
                    type="button"
                    onClick={() => startEdit(c)}
                    className="rounded-md border border-gray-200 px-2.5 py-1 text-xs text-gray-700 hover:bg-gray-50"
                  >
                    Edit
                  </button>
                  <button
                    type="button"
                    onClick={() => handleDelete(c.id)}
                    className="rounded-md border border-gray-200 px-2.5 py-1 text-xs text-red-700 hover:bg-red-50"
                  >
                    Delete
                  </button>
                </div>
              </div>
            )}
          </div>
        ))}
      </div>

      <section className="rounded-md border border-gray-200 p-4 shadow-sm">
        <h2 className="mb-3 text-sm font-semibold">New company</h2>
        <input
          value={newName}
          onChange={(e) => setNewName(e.target.value)}
          placeholder="Name"
          className="mb-2 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
        />
        <textarea
          value={newDescription}
          onChange={(e) => setNewDescription(e.target.value)}
          placeholder="Description"
          rows={2}
          className="mb-2 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
        />
        <button
          type="button"
          onClick={handleCreate}
          className="flex items-center gap-1 rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800"
        >
          <PlusIcon />
          Create company
        </button>
      </section>
    </div>
  )
}
