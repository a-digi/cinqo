import { useEffect, useState } from 'react'
import { createRecruiter, fetchCompanies, type Company } from '../../api'
import { PlusIcon, UsersIcon } from '../../Shared/Icons/icons'
import { inputClass } from '../../Shared/inputClass'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { COMPANIES_PATH, RECRUITERS_PATH } from '../../paths'
import { Dropdown } from '../Dropdown/Dropdown'
import { PAGE_ACCENT } from '../JobsPage/jobsTheme'

// The page behind the Recruiters list's "New recruiter" button. A recruiter must
// belong to an existing company; ?companyId= pre-selects one (set when the list
// is filtered by a company). Menu-less, like the other create/detail pages.
export function NewRecruiterPage() {
  const [companies, setCompanies] = useState<Company[]>([])
  const [companyId, setCompanyId] = useState(() => new URLSearchParams(window.location.search).get('companyId') ?? '')
  const [firstName, setFirstName] = useState('')
  const [lastName, setLastName] = useState('')
  const [email, setEmail] = useState('')
  const [error, setError] = useState('')
  const [isLoaded, setIsLoaded] = useState(false)
  const [isSaving, setIsSaving] = useState(false)

  useEffect(() => {
    fetchCompanies()
      .then(setCompanies)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setIsLoaded(true)
      })
  }, [])

  function backToList() {
    const suffix = companyId ? `?companyId=${encodeURIComponent(companyId)}` : ''
    window.__cinqoToolBridge.navigate(`${RECRUITERS_PATH}${suffix}`)
  }

  function handleCreate() {
    const first = firstName.trim()
    const last = lastName.trim()
    if (!companyId) {
      setError('Company is required')
      return
    }
    if (!first && !last) {
      setError('First or last name is required')
      return
    }
    setError('')
    setIsSaving(true)
    createRecruiter(companyId, first, last, email.trim())
      .then(backToList)
      .catch((err: unknown) => {
        setIsSaving(false)
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  return (
    <div className="max-w-3xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<UsersIcon />}
        title="New recruiter"
        description="Add a recruiter contact. It is tied to a company, and that cannot be changed later."
      />
      {isLoaded && companies.length === 0 ? (
        <div
          className="rounded-3xl border p-6 text-sm text-gray-700"
          style={{ borderColor: PAGE_ACCENT.soft, background: PAGE_ACCENT.soft }}
        >
          No companies yet.{' '}
          <a
            href={COMPANIES_PATH}
            onClick={(e) => {
              e.preventDefault()
              window.__cinqoToolBridge.navigate(COMPANIES_PATH)
            }}
            className="font-medium underline"
          >
            Create one first
          </a>
          .
        </div>
      ) : (
        <form
          onSubmit={(e) => {
            e.preventDefault()
            handleCreate()
          }}
          className="rounded-3xl border p-5 shadow-sm"
          style={{ borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }}
        >
          <div className="mb-4">
            <Dropdown
              label="Company"
              placeholder="Select a company"
              options={companies.map((c) => ({ value: c.id, label: c.name }))}
              value={companyId}
              onChange={setCompanyId}
            />
          </div>
          <div className="mb-4 grid gap-4 sm:grid-cols-2">
            <div>
              <label className="mb-1 block text-xs font-medium text-gray-600" htmlFor="recruiter-first">
                First name
              </label>
              <input
                id="recruiter-first"
                value={firstName}
                onChange={(e) => {
                  setFirstName(e.target.value)
                }}
                placeholder="First name"
                className={inputClass}
              />
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-gray-600" htmlFor="recruiter-last">
                Last name
              </label>
              <input
                id="recruiter-last"
                value={lastName}
                onChange={(e) => {
                  setLastName(e.target.value)
                }}
                placeholder="Last name"
                className={inputClass}
              />
            </div>
          </div>
          <label className="mb-1 block text-xs font-medium text-gray-600" htmlFor="recruiter-email">
            Email
          </label>
          <input
            id="recruiter-email"
            value={email}
            onChange={(e) => {
              setEmail(e.target.value)
            }}
            placeholder="Email"
            className={`mb-4 ${inputClass}`}
          />
          <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>
          <div className="mt-3 flex gap-2">
            <button
              type="submit"
              disabled={isSaving}
              className="flex items-center gap-1 rounded-md bg-gray-900 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800 disabled:opacity-50"
            >
              <PlusIcon />
              Create recruiter
            </button>
            <button
              type="button"
              onClick={backToList}
              className="rounded-md border border-gray-200 px-3.5 py-2 text-sm text-gray-700 hover:bg-gray-50"
            >
              Cancel
            </button>
          </div>
        </form>
      )}
    </div>
  )
}
