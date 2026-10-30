import { useEffect, useState } from 'react'
import { fetchCompanies, updateCompany, removeCompany, type Company } from '../../api'
import { BuildingIcon, PlusIcon } from '../../Shared/Icons/icons'
import { inputClass } from '../../Shared/inputClass'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { SummaryRail } from '../../Shared/SummaryRail/SummaryRail'
import { ACCENTS, PAGE_ACCENT, accentFor, initialOf } from '../JobsPage/jobsTheme'
import { percentOrNull, plural, summarizeCompanies } from './companyStats'

import { JOBS_PATH, NEW_COMPANY_PATH, RECRUITERS_PATH, VIEW_COMPANY_PATH } from '../../paths'

// Companies are tool-wide, not persona/profile-scoped — same reasoning
// JobsPage itself already documents, since a company is about the job
// market, not the job seeker's own identity. No switcher needed here.
// Step 17 added recruiterCount alongside jobCount, linking to
// RecruitersPage the same one-way ?companyId= pre-filter way. The page
// has the same layout as Portals and Jobs: header card, an overview on
// the left, the companies on the right; "New company" opens its own page. See
// plan/ai/tools/career/step-15-companies-frontend.md and
// plan/ai/tools/career/step-17-recruiters-frontend.md.
export function CompaniesPage() {
  const [companies, setCompanies] = useState<Company[]>([])
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editName, setEditName] = useState('')
  const [editDescription, setEditDescription] = useState('')
  const [error, setError] = useState('')

  function load() {
    setError('')
    fetchCompanies()
      .then(setCompanies)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  useEffect(() => {
    load()
  }, [])

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
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  function handleDelete(id: string) {
    setError('')
    removeCompany(id)
      .then(() => {
        setCompanies((prev) => prev.filter((c) => c.id !== id))
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  function navigateToJobs(e: React.MouseEvent, companyId: string) {
    e.preventDefault()
    window.__cinqoToolBridge.navigate(`${JOBS_PATH}?companyId=${encodeURIComponent(companyId)}`)
  }

  function navigateToRecruiters(e: React.MouseEvent, companyId: string) {
    e.preventDefault()
    window.__cinqoToolBridge.navigate(`${RECRUITERS_PATH}?companyId=${encodeURIComponent(companyId)}`)
  }

  const summary = summarizeCompanies(companies)
  const withJobsPercent = percentOrNull(summary.withJobs, summary.companies)

  return (
    <div className="career-v-right max-w-7xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<BuildingIcon />}
        title="Companies"
        description="A directory of companies, separate from the free-text name on each crawled job. Deleting a company un-links its jobs; it never deletes them."
        pill={{ text: plural(summary.companies, 'company', 'companies') }}
      />

      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <div className="order-1 min-w-0 flex-1 lg:order-2">
          <p className="mb-3 text-xs text-gray-500">Link a job to a company from the Jobs page.</p>

          <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

          {companies.length === 0 && !error && (
            <div
              className="rounded-3xl border p-8 text-center text-sm text-gray-500"
              style={{ borderColor: PAGE_ACCENT.soft, background: PAGE_ACCENT.soft }}
            >
              No companies yet. Create the first one with “New company”.
            </div>
          )}

          <div className="career-stagger space-y-3">
            {companies.map((c) => {
              const accent = accentFor(c.name)
              return (
                <div
                  key={c.id}
                  className="rounded-3xl border p-4 shadow-sm transition-shadow hover:shadow-md"
                  style={{ borderColor: PAGE_ACCENT.soft, background: `linear-gradient(160deg, #ffffff 45%, ${PAGE_ACCENT.soft} 100%)` }}
                >
                  {editingId === c.id ? (
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
                      <textarea
                        value={editDescription}
                        onChange={(e) => {
                          setEditDescription(e.target.value)
                        }}
                        placeholder="Description"
                        rows={2}
                        className={`mb-2 ${inputClass}`}
                      />
                      <div className="flex gap-2">
                        <button
                          type="button"
                          onClick={() => {
                            saveEdit(c.id)
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
                        style={{ background: accent.to }}
                        aria-hidden="true"
                      >
                        {initialOf(c.name)}
                      </span>
                      <div className="min-w-0 flex-1">
                        <a
                          href={`${VIEW_COMPANY_PATH}?id=${encodeURIComponent(c.id)}`}
                          onClick={(e) => {
                            e.preventDefault()
                            window.__cinqoToolBridge.navigate(`${VIEW_COMPANY_PATH}?id=${encodeURIComponent(c.id)}`)
                          }}
                          className="text-sm font-semibold hover:underline"
                        >
                          {c.name}
                        </a>
                        {c.description && <div className="mt-0.5 text-xs text-gray-500">{c.description}</div>}
                        <div className="mt-2 flex flex-wrap gap-2">
                          <a
                            href={`${JOBS_PATH}?companyId=${encodeURIComponent(c.id)}`}
                            onClick={(e) => {
                              navigateToJobs(e, c.id)
                            }}
                            className="rounded-full px-2.5 py-0.5 text-xs font-medium hover:underline"
                            style={{ background: PAGE_ACCENT.soft, color: PAGE_ACCENT.text }}
                          >
                            {plural(c.jobCount, 'linked job', 'linked jobs')}
                          </a>
                          <a
                            href={`${RECRUITERS_PATH}?companyId=${encodeURIComponent(c.id)}`}
                            onClick={(e) => {
                              navigateToRecruiters(e, c.id)
                            }}
                            className="rounded-full px-2.5 py-0.5 text-xs font-medium hover:underline"
                            style={{ background: ACCENTS[4].soft, color: ACCENTS[4].text }}
                          >
                            {plural(c.recruiterCount, 'recruiter', 'recruiters')}
                          </a>
                        </div>
                      </div>
                      <div className="flex shrink-0 gap-2">
                        <button
                          type="button"
                          onClick={() => {
                            window.__cinqoToolBridge.navigate(`${VIEW_COMPANY_PATH}?id=${encodeURIComponent(c.id)}`)
                          }}
                          className="rounded-full border border-gray-200 px-3 py-1 text-xs text-gray-700 hover:bg-gray-50"
                        >
                          View
                        </button>
                        <button
                          type="button"
                          onClick={() => {
                            startEdit(c)
                          }}
                          className="rounded-full border border-blue-200 px-3 py-1 text-xs text-blue-700 hover:bg-blue-50"
                        >
                          Edit
                        </button>
                        <button
                          type="button"
                          onClick={() => {
                            handleDelete(c.id)
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
          <button
            type="button"
            onClick={() => {
              window.__cinqoToolBridge.navigate(NEW_COMPANY_PATH)
            }}
            className="mb-4 flex w-full items-center justify-center gap-1.5 rounded-md bg-gray-800 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-900"
          >
            <PlusIcon />
            New company
          </button>
          <SummaryRail
            title="Overview"
            segments={[
              { value: summary.withJobs, color: '#16a34a' },
              { value: summary.companies - summary.withJobs, color: '#cbd5e1' },
            ]}
            centerValue={withJobsPercent === null ? '–' : `${withJobsPercent}%`}
            centerLabel="have jobs"
            rows={[
              { label: 'Companies', value: String(summary.companies), color: ACCENTS[0].to },
              { label: 'With jobs', value: String(summary.withJobs), color: '#16a34a' },
              { label: 'Linked jobs', value: String(summary.jobs), color: ACCENTS[6].to },
              { label: 'Recruiters', value: String(summary.recruiters), color: ACCENTS[4].to },
            ]}
          />
        </aside>
      </div>
    </div>
  )
}
