import { useEffect, useState } from 'react'
import { fetchRecruiters, updateRecruiter, removeRecruiter, fetchCompanies, type Recruiter, type Company } from '../../api'
import { Dropdown } from '../Dropdown/Dropdown'
import { PlusIcon, UsersIcon } from '../../Shared/Icons/icons'
import { inputClass } from '../../Shared/inputClass'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { SummaryRail } from '../../Shared/SummaryRail/SummaryRail'
import { ACCENTS, PAGE_ACCENT, accentFor, initialOf } from '../JobsPage/jobsTheme'
import { percentOrNull, plural } from '../CompaniesPage/companyStats'
import { COMPANIES_PATH, NEW_RECRUITER_PATH } from '../../paths'
import { recruiterName, summarizeRecruiters } from './recruiterStats'

// Recruiters are forced to be mapped to an existing Company, never
// implicit — the same pattern step 8/10 already established for
// Persona->Profile. No company reassignment on edit, matching the
// backend's own update_recruiter. The page has the same layout as Portals and
// Companies: header card, an overview on the left, the recruiters on the right,
// "New recruiter" opens its own page. See
// plan/ai/tools/career/step-17-recruiters-frontend.md.
export function RecruitersPage() {
  const [companies, setCompanies] = useState<Company[]>([])
  const [recruiters, setRecruiters] = useState<Recruiter[]>([])
  const [filterCompanyId, setFilterCompanyId] = useState(() => new URLSearchParams(window.location.search).get('companyId') ?? '')
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editFirstName, setEditFirstName] = useState('')
  const [editLastName, setEditLastName] = useState('')
  const [editEmail, setEditEmail] = useState('')
  const [error, setError] = useState('')

  function load(companyIdOverride?: string) {
    setError('')
    fetchRecruiters(companyIdOverride ?? filterCompanyId)
      .then(setRecruiters)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  useEffect(() => {
    load()
    fetchCompanies()
      .then(setCompanies)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  function handleFilterChange(id: string) {
    setFilterCompanyId(id)
    load(id)
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
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  function handleDelete(id: string) {
    setError('')
    removeRecruiter(id)
      .then(() => {
        setRecruiters((prev) => prev.filter((r) => r.id !== id))
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  function navigateToCompanies(e: React.MouseEvent) {
    e.preventDefault()
    window.__cinqoToolBridge.navigate(COMPANIES_PATH)
  }

  const companyNames: Record<string, string> = Object.fromEntries(companies.map((c) => [c.id, c.name]))
  const summary = summarizeRecruiters(recruiters)
  const withEmailPercent = percentOrNull(summary.withEmail, summary.recruiters)
  const filtered = filterCompanyId !== ''

  return (
    <div className="max-w-7xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<UsersIcon />}
        title="Recruiters"
        description="Recruiter contacts, each tied to an existing company. A recruiter's company cannot be changed after creation: delete and recreate instead."
        pill={{ text: plural(summary.recruiters, 'recruiter', 'recruiters') }}
      />

      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <div className="order-1 min-w-0 flex-1 lg:order-2">
          <div className="mb-3 flex items-end gap-3">
            {companies.length > 0 ? (
              <Dropdown
                label="Filter by company"
                placeholder="All companies"
                options={[{ value: '', label: 'All companies' }, ...companies.map((c) => ({ value: c.id, label: c.name }))]}
                value={filterCompanyId}
                onChange={handleFilterChange}
              />
            ) : (
              <span />
            )}
          </div>

          <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

          {companies.length === 0 && (
            <div
              className="rounded-3xl border p-6 text-sm text-gray-700"
              style={{ borderColor: PAGE_ACCENT.soft, background: PAGE_ACCENT.soft }}
            >
              No companies yet.{' '}
              <a href={COMPANIES_PATH} onClick={navigateToCompanies} className="font-medium underline">
                Create one first
              </a>
              .
            </div>
          )}

          {companies.length > 0 && recruiters.length === 0 && !error && (
            <div
              className="rounded-3xl border p-8 text-center text-sm text-gray-500"
              style={{ borderColor: PAGE_ACCENT.soft, background: PAGE_ACCENT.soft }}
            >
              {filtered ? 'This company has no recruiters yet.' : 'No recruiters yet. Add the first one with “New recruiter”.'}
            </div>
          )}

          <div className="career-stagger space-y-3">
            {recruiters.map((r) => {
              const name = recruiterName(r)
              const companyName = companyNames[r.companyId] ?? '…'
              const companyAccent = accentFor(companyName)
              return (
                <div
                  key={r.id}
                  className="rounded-3xl border p-4 shadow-sm transition-shadow hover:shadow-md"
                  style={{ borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }}
                >
                  {editingId === r.id ? (
                    <div>
                      <input
                        // Deliberate: entering edit mode should focus the input
                        // immediately, the same convention most inline-rename UIs use.
                        // eslint-disable-next-line jsx-a11y/no-autofocus
                        autoFocus
                        value={editFirstName}
                        onChange={(e) => {
                          setEditFirstName(e.target.value)
                        }}
                        placeholder="First name"
                        className={`mb-2 ${inputClass}`}
                      />
                      <input
                        value={editLastName}
                        onChange={(e) => {
                          setEditLastName(e.target.value)
                        }}
                        placeholder="Last name"
                        className={`mb-2 ${inputClass}`}
                      />
                      <input
                        value={editEmail}
                        onChange={(e) => {
                          setEditEmail(e.target.value)
                        }}
                        placeholder="Email"
                        className={`mb-2 ${inputClass}`}
                      />
                      <div className="flex gap-2">
                        <button
                          type="button"
                          onClick={() => {
                            saveEdit(r.id)
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
                        style={{ background: accentFor(name).to }}
                        aria-hidden="true"
                      >
                        {initialOf(name)}
                      </span>
                      <div className="min-w-0 flex-1">
                        <div className="text-sm font-semibold">{name}</div>
                        {r.email && <div className="mt-0.5 truncate text-xs text-gray-500">{r.email}</div>}
                        <span
                          className="mt-2 inline-flex rounded-full px-2.5 py-0.5 text-xs font-medium"
                          style={{ background: companyAccent.soft, color: companyAccent.text }}
                        >
                          {companyName}
                        </span>
                      </div>
                      <div className="flex shrink-0 gap-2">
                        <button
                          type="button"
                          onClick={() => {
                            startEdit(r)
                          }}
                          className="rounded-full border border-blue-200 px-3 py-1 text-xs text-blue-700 hover:bg-blue-50"
                        >
                          Edit
                        </button>
                        <button
                          type="button"
                          onClick={() => {
                            handleDelete(r.id)
                          }}
                          className="rounded-full border border-red-200 px-3 py-1 text-xs text-red-700 hover:bg-red-50"
                        >
                          Delete
                        </button>
                      </div>
                    </div>
                  )}
                </div>
              )
            })}
          </div>
        </div>

        <aside className="order-2 w-full shrink-0 lg:order-1 lg:sticky lg:top-6 lg:w-56">
          {companies.length > 0 && (
            <button
              type="button"
              onClick={() => {
                const suffix = filtered ? `?companyId=${encodeURIComponent(filterCompanyId)}` : ''
                window.__cinqoToolBridge.navigate(`${NEW_RECRUITER_PATH}${suffix}`)
              }}
              className="mb-4 flex w-full items-center justify-center gap-1.5 rounded-md bg-gray-800 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-900"
            >
              <PlusIcon />
              New recruiter
            </button>
          )}
          <SummaryRail
            title={filtered ? 'Overview (filtered)' : 'Overview'}
            segments={[
              { value: summary.withEmail, color: '#16a34a' },
              { value: summary.withoutEmail, color: '#cbd5e1' },
            ]}
            centerValue={withEmailPercent === null ? '–' : `${withEmailPercent}%`}
            centerLabel="have an email"
            rows={[
              { label: 'Recruiters', value: String(summary.recruiters), color: ACCENTS[0].to },
              { label: 'With email', value: String(summary.withEmail), color: '#16a34a' },
              { label: 'Without email', value: String(summary.withoutEmail), color: ACCENTS[2].to, emphasis: summary.withoutEmail > 0 },
              { label: 'Companies', value: String(summary.companies), color: ACCENTS[4].to },
            ]}
          />
        </aside>
      </div>
    </div>
  )
}
