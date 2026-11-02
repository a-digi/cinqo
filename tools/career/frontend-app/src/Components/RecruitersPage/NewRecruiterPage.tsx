import { useEffect, useState } from 'react'
import { createRecruiter, fetchCompanies, type Company } from '../../api'
import { PlusIcon, UsersIcon } from '../../Shared/Icons/icons'
import { inputClass } from '../../Shared/inputClass'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { companiesError, initialCompanyIds, listFilterAfterCreate } from './recruiterCompaniesRules'
import { COMPANIES_PATH, RECRUITERS_PATH } from '../../paths'
import { Dropdown } from '../Dropdown/Dropdown'
import { PAGE_ACCENT } from '../JobsPage/jobsTheme'

// The page behind the Recruiters list's "New recruiter" button. A recruiter works for
// one or more existing companies (at least one is asked for here); ?companyId=
// pre-selects one (set when the list is filtered by a company). Menu-less, like
// the other create/detail pages.
export function NewRecruiterPage() {
  const [companies, setCompanies] = useState<Company[]>([])
  const [companyIds, setCompanyIds] = useState<string[]>(() => initialCompanyIds(window.location.search))
  const [cameFrom] = useState(() => new URLSearchParams(window.location.search).get('companyId') ?? '')
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

  // Cancel goes back to the list it came from; after creating, to the one company's list when only one was chosen.
  function goToList(filter: string) {
    const suffix = filter ? `?companyId=${encodeURIComponent(filter)}` : ''
    window.__cinqoToolBridge.navigate(`${RECRUITERS_PATH}${suffix}`)
  }

  function handleCreate() {
    const first = firstName.trim()
    const last = lastName.trim()
    const missing = companiesError(companyIds)
    if (missing) {
      setError(missing)
      return
    }
    if (!first && !last) {
      setError('First or last name is required')
      return
    }
    setError('')
    setIsSaving(true)
    createRecruiter(companyIds, first, last, email.trim())
      .then(() => {
        goToList(listFilterAfterCreate(companyIds))
      })
      .catch((err: unknown) => {
        setIsSaving(false)
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  return (
    <div className="career-v-blur career-a-float career-t-wave max-w-3xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<UsersIcon />}
        title="New recruiter"
        description="Add a recruiter contact and the companies they work for. A recruiter can work for several, and the companies can be changed later."
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
          className="career-enter career-delay-1 rounded-3xl border p-5 shadow-sm"
          style={{ borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }}
        >
          <div className="mb-4">
            <Dropdown
              multiple
              searchable
              label="Companies"
              placeholder="Select one or more companies"
              options={companies.map((c) => ({ value: c.id, label: c.name }))}
              value={companyIds}
              onChange={setCompanyIds}
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
              onClick={() => {
                goToList(cameFrom)
              }}
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
