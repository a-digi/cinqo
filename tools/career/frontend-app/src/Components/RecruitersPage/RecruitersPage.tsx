import { useEffect, useState } from 'react'
import {
  fetchRecruiters,
  createRecruiter,
  updateRecruiter,
  removeRecruiter,
  fetchCompanies,
  type Recruiter,
  type Company,
} from '../../api'
import { Dropdown } from '../Dropdown/Dropdown'
import { PlusIcon } from '../../icons'

const COMPANIES_PATH = '/tools/career/companies'

// Recruiters are forced to be mapped to an existing Company, never
// implicit — the same pattern step 8/10 already established for
// Persona->Profile. No company reassignment on edit, matching the
// backend's own update_recruiter. See
// plan/ai/tools/career/step-17-recruiters-frontend.md.
export function RecruitersPage() {
  const [companies, setCompanies] = useState<Company[]>([])
  const [recruiters, setRecruiters] = useState<Recruiter[]>([])
  const [filterCompanyId, setFilterCompanyId] = useState(
    () => new URLSearchParams(window.location.search).get('companyId') ?? '',
  )
  const [newCompanyId, setNewCompanyId] = useState('')
  const [newFirstName, setNewFirstName] = useState('')
  const [newLastName, setNewLastName] = useState('')
  const [newEmail, setNewEmail] = useState('')
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editFirstName, setEditFirstName] = useState('')
  const [editLastName, setEditLastName] = useState('')
  const [editEmail, setEditEmail] = useState('')
  const [error, setError] = useState('')

  function load(companyIdOverride?: string) {
    setError('')
    fetchRecruiters(companyIdOverride ?? filterCompanyId)
      .then(setRecruiters)
      .catch((err: Error) => setError(err.message))
  }

  useEffect(() => {
    load()
    fetchCompanies()
      .then(setCompanies)
      .catch((err: Error) => setError(err.message))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  function handleFilterChange(id: string) {
    setFilterCompanyId(id)
    load(id)
  }

  function handleCreate() {
    const firstName = newFirstName.trim()
    const lastName = newLastName.trim()
    const email = newEmail.trim()
    if (!newCompanyId) {
      setError('Company is required')
      return
    }
    if (!firstName && !lastName) {
      setError('First or last name is required')
      return
    }
    setError('')
    createRecruiter(newCompanyId, firstName, lastName, email)
      .then(() => {
        setNewFirstName('')
        setNewLastName('')
        setNewEmail('')
        load()
      })
      .catch((err: Error) => setError(err.message))
  }

  function startEdit(r: Recruiter) {
    setEditingId(r.id)
    setEditFirstName(r.firstName)
    setEditLastName(r.lastName)
    setEditEmail(r.email)
  }

  function cancelEdit() {
    setEditingId(null)
    setEditFirstName('')
    setEditLastName('')
    setEditEmail('')
  }

  function saveEdit(id: string) {
    setError('')
    updateRecruiter(id, { firstName: editFirstName.trim(), lastName: editLastName.trim(), email: editEmail.trim() })
      .then((result) => {
        setRecruiters(result)
        cancelEdit()
      })
      .catch((err: Error) => setError(err.message))
  }

  function handleDelete(id: string) {
    setError('')
    removeRecruiter(id)
      .then(() => setRecruiters((prev) => prev.filter((r) => r.id !== id)))
      .catch((err: Error) => setError(err.message))
  }

  function navigateToCompanies(e: React.MouseEvent) {
    e.preventDefault()
    window.__cinqoToolBridge.navigate(COMPANIES_PATH)
  }

  const companyNames: Record<string, string> = Object.fromEntries(companies.map((c) => [c.id, c.name]))

  return (
    <div className="max-w-2xl p-6 font-sans text-gray-900">
      <h1 className="mb-1.5 text-xl font-semibold">Recruiters</h1>
      <p className="mb-5 text-sm text-gray-500">
        Recruiter contacts, each tied to an existing company. A recruiter's own company cannot be
        changed after creation — delete and recreate instead.
      </p>

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      {companies.length > 0 && (
        <div className="mb-4">
          <Dropdown
            label="Filter by company"
            placeholder="All companies"
            options={[{ value: '', label: 'All companies' }, ...companies.map((c) => ({ value: c.id, label: c.name }))]}
            value={filterCompanyId}
            onChange={handleFilterChange}
          />
        </div>
      )}

      <div className="mb-4 space-y-3">
        {recruiters.map((r) => (
          <div key={r.id} className="rounded-md border border-gray-200 p-3">
            {editingId === r.id ? (
              <div>
                <input
                  autoFocus
                  value={editFirstName}
                  onChange={(e) => setEditFirstName(e.target.value)}
                  placeholder="First name"
                  className="mb-2 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
                />
                <input
                  value={editLastName}
                  onChange={(e) => setEditLastName(e.target.value)}
                  placeholder="Last name"
                  className="mb-2 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
                />
                <input
                  value={editEmail}
                  onChange={(e) => setEditEmail(e.target.value)}
                  placeholder="Email"
                  className="mb-2 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
                />
                <div className="flex gap-2">
                  <button
                    type="button"
                    onClick={() => saveEdit(r.id)}
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
                  <div className="text-sm font-medium">
                    {r.firstName || r.lastName ? `${r.firstName} ${r.lastName}`.trim() : '(unnamed)'}
                  </div>
                  {r.email && <div className="mt-0.5 text-xs text-gray-500">{r.email}</div>}
                  <div className="mt-0.5 text-xs text-gray-400">{companyNames[r.companyId] ?? '…'}</div>
                </div>
                <div className="flex shrink-0 gap-2">
                  <button
                    type="button"
                    onClick={() => startEdit(r)}
                    className="rounded-md border border-gray-200 px-2.5 py-1 text-xs text-gray-700 hover:bg-gray-50"
                  >
                    Edit
                  </button>
                  <button
                    type="button"
                    onClick={() => handleDelete(r.id)}
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

      {companies.length === 0 ? (
        <div className="rounded-md border border-gray-200 bg-gray-50 p-3 text-sm text-gray-700">
          No companies yet.{' '}
          <a href={COMPANIES_PATH} onClick={navigateToCompanies} className="font-medium underline">
            Create one first
          </a>
          .
        </div>
      ) : (
        <section className="rounded-md border border-gray-200 p-4 shadow-sm">
          <h2 className="mb-3 text-sm font-semibold">New recruiter</h2>
          <div className="mb-2">
            <Dropdown
              label="Company"
              placeholder="Select a company"
              options={companies.map((c) => ({ value: c.id, label: c.name }))}
              value={newCompanyId}
              onChange={setNewCompanyId}
            />
          </div>
          <input
            value={newFirstName}
            onChange={(e) => setNewFirstName(e.target.value)}
            placeholder="First name"
            className="mb-2 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
          />
          <input
            value={newLastName}
            onChange={(e) => setNewLastName(e.target.value)}
            placeholder="Last name"
            className="mb-2 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
          />
          <input
            value={newEmail}
            onChange={(e) => setNewEmail(e.target.value)}
            placeholder="Email"
            className="mb-2 w-full rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
          />
          <button
            type="button"
            onClick={handleCreate}
            className="flex items-center gap-1 rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800"
          >
            <PlusIcon />
            Create recruiter
          </button>
        </section>
      )}
    </div>
  )
}
