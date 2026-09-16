import { useEffect, useRef, useState } from 'react'
import {
  fetchJobs,
  removeJob,
  fetchCompanies,
  fetchPortals,
  fetchJobLocations,
  linkJobToCompany,
  type Job,
  type Company,
  type Portal,
} from '../../api'
import { Dropdown } from '../Dropdown/Dropdown'
import { Pagination } from '../Pagination/Pagination'
import { FilterIcon, XIcon } from '../../Shared/Icons/icons'

const PAGE_SIZE = 50
const SEARCH_DEBOUNCE_MS = 300

// No "add job" affordance — jobs are populated by the AI's own
// crawling workflow (step 4), never hand-entered here. See this
// step's own open question 2.
//
// Step 15 added an optional companyId filter — read once from the
// URL on mount (so CompaniesPage's own "N linked jobs" link arrives
// pre-filtered) and also selectable via a Dropdown once companies
// exist. See plan/ai/tools/career/step-15-companies-frontend.md.
//
// Step 53 added an optional portalId filter (same URL-seeded +
// Dropdown pattern as companyId) and a Platform column showing which
// portal (LinkedIn, Indeed, etc.) a job was found via. See
// plan/ai/tools/career/step-53-jobs-page-platform-column-filter-pagination.md.
//
// Step 55 turned the query search into live autocomplete — no Search
// button, no <form> submit; typing debounces SEARCH_DEBOUNCE_MS before
// reloading page 1. See plan/ai/tools/career/step-55-jobs-page-
// location-dropdown-active-filters-live-search.md.
export function JobsPage() {
  const [query, setQuery] = useState('')
  const [location, setLocation] = useState('')
  const [companyId, setCompanyId] = useState(() => new URLSearchParams(window.location.search).get('companyId') ?? '')
  const [portalId, setPortalId] = useState(() => new URLSearchParams(window.location.search).get('portalId') ?? '')
  const [companies, setCompanies] = useState<Company[]>([])
  const [portals, setPortals] = useState<Portal[]>([])
  const [locations, setLocations] = useState<string[]>([])
  const [jobs, setJobs] = useState<Job[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [error, setError] = useState('')
  // Pre-expanded whenever a filter arrives pre-set via the URL (e.g.
  // CompaniesPage's own "N linked jobs" link) — a filter silently
  // applied behind a collapsed panel would be confusing.
  const [showFilters, setShowFilters] = useState(() => {
    const params = new URLSearchParams(window.location.search)
    return params.get('companyId') !== null || params.get('portalId') !== null
  })

  function load(companyIdOverride?: string, portalIdOverride?: string, pageOverride?: number, locationOverride?: string) {
    setError('')
    const effectivePage = pageOverride ?? page
    fetchJobs(
      query,
      locationOverride ?? location,
      companyIdOverride ?? companyId,
      portalIdOverride ?? portalId,
      PAGE_SIZE,
      (effectivePage - 1) * PAGE_SIZE,
    )
      .then((result) => {
        setJobs(result.jobs)
        setTotal(result.total)
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  useEffect(() => {
    load(companyId, portalId, 1)
    fetchCompanies()
      .then(setCompanies)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
    fetchPortals()
      .then(setPortals)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
    fetchJobLocations()
      .then(setLocations)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Live autocomplete: skip the very first render (the mount effect
  // above already loads page 1 once) so typing is the only thing that
  // triggers this debounce, not the initial empty-string query.
  const isFirstQueryRender = useRef(true)
  useEffect(() => {
    if (isFirstQueryRender.current) {
      isFirstQueryRender.current = false
      return
    }
    const timeoutId = window.setTimeout(() => {
      setPage(1)
      load(undefined, undefined, 1)
    }, SEARCH_DEBOUNCE_MS)
    return () => {
      window.clearTimeout(timeoutId)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query])

  function handleCompanyFilterChange(id: string) {
    setCompanyId(id)
    setPage(1)
    load(id, undefined, 1)
  }

  function handlePortalFilterChange(id: string) {
    setPortalId(id)
    setPage(1)
    load(undefined, id, 1)
  }

  function handleLocationFilterChange(loc: string) {
    setLocation(loc)
    setPage(1)
    load(undefined, undefined, 1, loc)
  }

  function handlePageChange(nextPage: number) {
    setPage(nextPage)
    load(undefined, undefined, nextPage)
  }

  function handleRemove(id: string) {
    setError('')
    removeJob(id)
      .then(() => {
        load()
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  function handleUnlink(id: string) {
    setError('')
    linkJobToCompany(id, '')
      .then(() => {
        load()
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  const companyNames: Record<string, string> = Object.fromEntries(companies.map((c) => [c.id, c.name]))
  const portalNames: Record<string, string> = Object.fromEntries(portals.map((p) => [p.id, p.name]))

  const activeFilters: { key: string; label: string; onClear: () => void }[] = []
  if (location)
    activeFilters.push({
      key: 'location',
      label: `Location: ${location}`,
      onClear: () => {
        handleLocationFilterChange('')
      },
    })
  if (companyId)
    activeFilters.push({
      key: 'company',
      label: `Company: ${companyNames[companyId] ?? '…'}`,
      onClear: () => {
        handleCompanyFilterChange('')
      },
    })
  if (portalId)
    activeFilters.push({
      key: 'portal',
      label: `Platform: ${portalNames[portalId] ?? '…'}`,
      onClear: () => {
        handlePortalFilterChange('')
      },
    })

  return (
    <div className="max-w-4xl p-6 font-sans text-gray-900">
      <h1 className="mb-1.5 text-xl font-semibold">Saved Jobs</h1>
      <p className="mb-5 text-sm text-gray-500">Job postings the AI has crawled and saved on your behalf.</p>

      {activeFilters.length > 0 && (
        <div className="mb-2 flex flex-wrap gap-1.5">
          {activeFilters.map((filter) => (
            <span
              key={filter.key}
              className="flex items-center gap-1 rounded-full border border-gray-200 bg-gray-100 py-1 pl-2.5 pr-1.5 text-xs text-gray-700"
            >
              {filter.label}
              <button
                type="button"
                onClick={filter.onClear}
                aria-label={`Clear ${filter.label}`}
                className="text-gray-400 hover:text-gray-700"
              >
                <XIcon />
              </button>
            </span>
          ))}
        </div>
      )}

      <div className="mb-4 flex flex-wrap items-center gap-2">
        <input
          value={query}
          onChange={(e) => {
            setQuery(e.target.value)
          }}
          placeholder="Search title, company, description…"
          className="flex-1 min-w-[180px] rounded-md border border-gray-300 px-2.5 py-1.5 text-sm text-gray-900 focus:border-gray-500 focus:outline-none"
        />
        <button
          type="button"
          aria-expanded={showFilters}
          onClick={() => {
            setShowFilters((prev) => !prev)
          }}
          className="flex items-center gap-1.5 rounded-md border border-gray-300 px-3.5 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50"
        >
          <FilterIcon />
          Filters
        </button>
        {showFilters && (
          <>
            {locations.length > 0 && (
              <Dropdown
                placeholder="Any location"
                options={[{ value: '', label: 'Any location' }, ...locations.map((loc) => ({ value: loc, label: loc }))]}
                value={location}
                onChange={handleLocationFilterChange}
                searchable
              />
            )}
            {companies.length > 0 && (
              <Dropdown
                placeholder="Any company"
                options={[{ value: '', label: 'Any company' }, ...companies.map((c) => ({ value: c.id, label: c.name }))]}
                value={companyId}
                onChange={handleCompanyFilterChange}
                searchable
              />
            )}
            {portals.length > 0 && (
              <Dropdown
                placeholder="Any platform"
                options={[{ value: '', label: 'Any platform' }, ...portals.map((p) => ({ value: p.id, label: p.name }))]}
                value={portalId}
                onChange={handlePortalFilterChange}
                searchable
              />
            )}
          </>
        )}
      </div>

      <div className="min-h-[1.2em] text-sm text-red-700">{error}</div>

      <div className="overflow-hidden rounded-md border border-gray-200 shadow-sm">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr>
                <th className="border-b border-gray-200 bg-gray-50 p-3 text-left font-medium text-gray-500">Title</th>
                <th className="border-b border-gray-200 bg-gray-50 p-3 text-left font-medium text-gray-500">Company</th>
                <th className="border-b border-gray-200 bg-gray-50 p-3 text-left font-medium text-gray-500">Platform</th>
                <th className="border-b border-gray-200 bg-gray-50 p-3 text-left font-medium text-gray-500">Posted</th>
                <th className="border-b border-gray-200 bg-gray-50 p-3"></th>
              </tr>
            </thead>
            <tbody>
              {jobs.map((job) => (
                <tr key={job.id} className="last:[&>td]:border-b-0 hover:bg-gray-50">
                  <td className="border-b border-gray-200 p-3">
                    <a
                      href={job.sourceUrl}
                      target="_blank"
                      rel="noreferrer"
                      className="text-gray-900 underline decoration-gray-300 hover:decoration-gray-600"
                    >
                      {job.title}
                    </a>
                  </td>
                  <td className="border-b border-gray-200 p-3">
                    {job.company}
                    {job.companyId && (
                      <div className="mt-0.5 flex items-center gap-1.5 text-xs text-gray-400">
                        linked to {companyNames[job.companyId] ?? '…'}
                        <button
                          type="button"
                          onClick={() => {
                            handleUnlink(job.id)
                          }}
                          className="text-gray-400 underline hover:text-red-700"
                        >
                          unlink
                        </button>
                      </div>
                    )}
                  </td>
                  <td className="border-b border-gray-200 p-3">{job.portalName ?? '—'}</td>
                  <td className="border-b border-gray-200 p-3">{job.postedAt}</td>
                  <td className="border-b border-gray-200 p-3">
                    <button
                      type="button"
                      onClick={() => {
                        handleRemove(job.id)
                      }}
                      className="rounded-md border border-gray-200 px-3 py-1 text-red-700 transition-colors hover:bg-red-50"
                    >
                      Remove
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      <Pagination page={page} pageSize={PAGE_SIZE} total={total} onPageChange={handlePageChange} />
    </div>
  )
}
