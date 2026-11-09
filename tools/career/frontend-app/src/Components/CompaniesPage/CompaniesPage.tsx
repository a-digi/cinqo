import { useEffect, useMemo, useRef, useState } from 'react'
import { fetchCompaniesPage, updateCompany, removeCompany, type Company, type CompanyOverview } from '../../api'
import { BuildingIcon, PlusIcon } from '../../Shared/Icons/icons'
import { inputClass } from '../../Shared/inputClass'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { Modal } from '../../Shared/Modal/Modal'
import { Reviews } from '../../Shared/Reviews/Reviews'
import { Stars } from '../../Shared/Reviews/Stars'
import { REVIEW_TYPE_COMPANY } from '../../Shared/Reviews/reviewTypes'
import { SummaryRail } from '../../Shared/SummaryRail/SummaryRail'
import { ACCENTS, PAGE_ACCENT, accentFor, initialOf } from '../JobsPage/jobsTheme'
import { percentOrNull, plural } from './companyStats'
import { Pagination } from '../Pagination/Pagination'
import { useAddressSearch } from '../../Shared/AddressState/useAddressSearch'
import { writeAddress, type AddressMode } from '../../Shared/AddressState/writeAddress'
import {
  MAX_COMPANIES_LIMIT,
  MIN_COMPANIES_LIMIT,
  buildCompaniesSearch,
  isCanonicalCompaniesSearch,
  limitFromText,
  normalizeCompaniesState,
  offsetForPage,
  parseCompaniesUrl,
  type CompaniesListState,
} from './companiesUrlState'
import { clampPage } from '../../Shared/AddressState/paging'

import { COMPANIES_PATH, JOBS_PATH, NEW_COMPANY_PATH, RECRUITERS_PATH, VIEW_COMPANY_PATH } from '../../paths'
import { PageTips } from '../../Shared/PageTips/PageTips'

const SEARCH_DEBOUNCE_MS = 300
const NO_OVERVIEW: CompanyOverview = { companies: 0, withJobs: 0, withRecruiters: 0, jobs: 0, recruiters: 0 }

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
  // What the list shows (step 137): the page, the search and how many per page, exactly as the ADDRESS says them. A control
  // writes the address, the address announces itself and this follows (the same scheme as the Jobs list), so Back, Forward,
  // a reload and a copied link all show the same list.
  const address = useAddressSearch(COMPANIES_PATH)
  const view: CompaniesListState = useMemo(() => parseCompaniesUrl(address.search), [address.search])
  const { page, limit } = view
  // The text of the search box while it is being typed (it reaches the address after SEARCH_DEBOUNCE_MS) and of the per-page
  // box (it reaches the address on Enter or when the box loses focus, and only when it is a number from 1 to 200).
  // The company whose reviews window is open, and a counter that makes the list's small stars ask again once it is closed.
  const [ratingFor, setRatingFor] = useState<Company | null>(null)
  const [starsVersion, setStarsVersion] = useState(0)
  const [queryInput, setQueryInput] = useState(view.query)
  const [limitInput, setLimitInput] = useState(String(view.limit))
  const limitFocused = useRef(false)
  const [companies, setCompanies] = useState<Company[]>([])
  const [total, setTotal] = useState(0)
  const [overview, setOverview] = useState<CompanyOverview>(NO_OVERVIEW)
  const [loaded, setLoaded] = useState(false)
  const [reloadTick, setReloadTick] = useState(0)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editName, setEditName] = useState('')
  const [editDescription, setEditDescription] = useState('')
  const [error, setError] = useState('')

  // Changes the address: the change of one thing keeps everything else the address says. A page or limit change pushes an
  // entry (Back returns to the earlier list); the search replaces the entry (typing must not leave an entry per pause).
  function go(patch: Partial<CompaniesListState>, mode: AddressMode = 'push') {
    writeAddress(window, COMPANIES_PATH, buildCompaniesSearch({ ...parseCompaniesUrl(window.location.search), ...patch }), mode)
  }

  // Loads the page the address names; the latest request wins, so a slow answer never replaces a newer one.
  const latestRequest = useRef(0)
  const viewKey = buildCompaniesSearch(view)
  useEffect(() => {
    setError('')
    const request = ++latestRequest.current
    fetchCompaniesPage(view.query, view.limit, offsetForPage(view.page, view.limit))
      .then((result) => {
        if (request !== latestRequest.current) return
        setCompanies(result.companies)
        setTotal(result.total)
        setOverview(result.summary)
        setLoaded(true)
        // A page past the last one (an address typed by hand, companies deleted since) becomes the last page.
        const last = clampPage(view.page, result.total, view.limit)
        if (last !== view.page) go({ page: last }, 'replace')
      })
      .catch((err: unknown) => {
        if (request !== latestRequest.current) return
        setError(err instanceof Error ? err.message : String(err))
        setLoaded(true)
      })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [viewKey, reloadTick])

  function reload() {
    setReloadTick((t) => t + 1)
  }

  // The address changed (a link, Back or Forward, or this page): the boxes show what it says (the search box keeps what it has
  // when that means the same, so a space typed at the end is not taken away; the per-page box is left alone while it has the
  // focus), and an address that is not exactly the canonical one (`?p=1`, `limit=25`, a foreign parameter, a bad limit) is
  // rewritten as the canonical one, replacing the entry.
  useEffect(() => {
    setQueryInput((current) => (current.trim() === view.query ? current : view.query))
    if (!limitFocused.current) setLimitInput(String(view.limit))
    if (!isCanonicalCompaniesSearch(window.location.search)) {
      go({}, 'replace')
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [address.tick])

  // Live search: after SEARCH_DEBOUNCE_MS without typing, the text becomes the address's search and the list goes back to
  // page 1 (replacing the entry).
  useEffect(() => {
    const timeoutId = window.setTimeout(() => {
      const wanted = normalizeCompaniesState({ page: 1, query: queryInput, limit: MIN_COMPANIES_LIMIT }).query
      if (parseCompaniesUrl(window.location.search).query !== wanted) go({ query: wanted, page: 1 }, 'replace')
    }, SEARCH_DEBOUNCE_MS)
    return () => {
      window.clearTimeout(timeoutId)
    }
  }, [queryInput])

  // The per-page box: a number from 1 to 200 is applied (back to page 1); anything else puts the last good value back.
  function commitLimit() {
    const n = limitFromText(limitInput)
    if (n === null) {
      setLimitInput(String(limit))
      return
    }
    setLimitInput(String(n))
    if (n !== limit) go({ limit: n, page: 1 })
  }

  function clearSearch() {
    setQueryInput('')
    go({ query: '', page: 1 }, 'replace')
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
      .then(() => {
        cancelEdit()
        reload()
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  function handleDelete(id: string) {
    setError('')
    removeCompany(id)
      .then(() => {
        reload()
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

  const withJobsPercent = percentOrNull(overview.withJobs, overview.companies)

  return (
    <div className="career-v-right career-a-sheen career-t-wave max-w-7xl p-6 font-sans text-gray-900">
      <PageHeaderCard
        icon={<BuildingIcon />}
        title="Companies"
        description="A directory of companies, separate from the free-text name on each crawled job. Deleting a company un-links its jobs; it never deletes them."
        // The pill links on to the Recruiters page, the other list tied to companies.
        pill={{ text: plural(overview.companies, 'company', 'companies'), href: RECRUITERS_PATH }}
      />
      <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
        <div className="order-1 min-w-0 flex-1 lg:order-2">
          <p className="mb-3 text-xs text-gray-500">Link a job to a company from the Jobs page.</p>

          <div className="mb-3 flex flex-wrap items-center gap-3">
            <input
              type="search"
              value={queryInput}
              onChange={(e) => {
                setQueryInput(e.target.value)
              }}
              placeholder="Search name or description…"
              aria-label="Search companies"
              className="min-w-[180px] flex-1 rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
            />
            <label className="flex items-center gap-2 text-sm text-gray-600">
              Show
              <input
                type="text"
                inputMode="numeric"
                value={limitInput}
                onChange={(e) => {
                  setLimitInput(e.target.value)
                }}
                onFocus={() => {
                  limitFocused.current = true
                }}
                onBlur={() => {
                  limitFocused.current = false
                  commitLimit()
                }}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') commitLimit()
                }}
                aria-label={`Companies per page, ${String(MIN_COMPANIES_LIMIT)} to ${String(MAX_COMPANIES_LIMIT)}`}
                title={`How many companies to show per page (${String(MIN_COMPANIES_LIMIT)} to ${String(MAX_COMPANIES_LIMIT)})`}
                className="w-16 rounded-md border border-gray-300 px-2 py-1.5 text-center text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
              />
              per page
            </label>
          </div>

          <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

          {loaded && total === 0 && !error && view.query === '' && (
            <div
              className="career-wiggle rounded-3xl border p-8 text-center text-sm text-gray-500"
              style={{ borderColor: PAGE_ACCENT.soft, background: PAGE_ACCENT.soft }}
            >
              No companies yet. Create the first one with “New company”.
            </div>
          )}

          {loaded && total === 0 && !error && view.query !== '' && (
            <div
              className="rounded-3xl border p-8 text-center text-sm text-gray-500"
              style={{ borderColor: PAGE_ACCENT.soft, background: PAGE_ACCENT.soft }}
            >
              No company matches “{view.query}”.{' '}
              <button type="button" onClick={clearSearch} className="font-medium text-gray-700 underline hover:text-gray-900">
                Clear the search
              </button>
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
                        <div className="mt-1 flex items-center gap-2">
                          <Stars key={`${c.id}-${starsVersion}`} entityType={REVIEW_TYPE_COMPANY} entityId={c.id} />
                          <button
                            type="button"
                            onClick={() => {
                              setRatingFor(c)
                            }}
                            className="rounded-md border border-amber-200 bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-800 hover:bg-amber-100"
                          >
                            Rate
                          </button>
                        </div>
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

          {total > 0 && (
            <Pagination
              page={page}
              pageSize={limit}
              total={total}
              onPageChange={(next) => {
                go({ page: next })
              }}
            />
          )}
        </div>

        <aside className="order-2 w-full shrink-0 lg:order-1 career-rail-sticky lg:w-72">
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
              { value: overview.withJobs, color: '#16a34a' },
              { value: overview.companies - overview.withJobs, color: '#cbd5e1' },
            ]}
            centerValue={withJobsPercent === null ? '–' : `${withJobsPercent}%`}
            centerLabel="have jobs"
            rows={[
              { label: 'Companies', value: String(overview.companies), color: ACCENTS[0].to },
              { label: 'With jobs', value: String(overview.withJobs), color: '#16a34a' },
              { label: 'Linked jobs', value: String(overview.jobs), color: ACCENTS[6].to },
              { label: 'Recruiters', value: String(overview.recruiters), color: ACCENTS[4].to },
            ]}
          />
          <div className="mt-8">
            <PageTips page="companies" orientation="vertical" />
          </div>
        </aside>
      </div>

      <Modal
        open={ratingFor !== null}
        title={ratingFor ? `Rate ${ratingFor.name}` : 'Rate'}
        onClose={() => {
          setRatingFor(null)
          setStarsVersion((v) => v + 1) // the list's stars ask again: the rating may have changed
        }}
      >
        {ratingFor && <Reviews entityType={REVIEW_TYPE_COMPANY} entityId={ratingFor.id} inRail />}
      </Modal>
    </div>
  )
}
