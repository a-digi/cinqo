import { useEffect, useRef, useState } from 'react'
import {
  fetchJobs,
  removeJob,
  fetchCompanies,
  fetchPortals,
  fetchJobLocations,
  linkJobToCompany,
  fetchProfiles,
  updateJobMatch,
  type Job,
  type Company,
  type Portal,
  type Profile,
} from '../../api'
import { fetchPlatforms, fetchPlatformKeys, type Platform } from '../../Cinqo/Platform/platformRepository'
import { createConversation, sendMessage, awaitTurnCompletion, fetchTurnStatus } from '../../Cinqo/Conversation/conversation'
import { CoreApiError } from '../../Cinqo/Http/client'
import { CrawlPlatformPicker } from '../../Crawler/CrawlPlatformPicker/CrawlPlatformPicker'
import { buildJobMatchMessage, jobMatchConversationTitle } from '../../JobMatch/buildJobMatchMessage'
import { Dropdown } from '../Dropdown/Dropdown'
import { Pagination } from '../Pagination/Pagination'
import { Modal } from '../../Shared/Modal/Modal'
import { MatchScoreBar } from '../../Shared/MatchScoreBar/MatchScoreBar'
import { FilterIcon, XIcon, EyeIcon, ExternalLinkIcon, TrashIcon, MatchIcon, RobotIcon } from '../../Shared/Icons/icons'
import { Typewriter } from '../../Shared/Typewriter/Typewriter'
import { truncate } from '../../Shared/Text/transform'

const PAGE_SIZE = 50
const SEARCH_DEBOUNCE_MS = 300
const MAX_LOCATION_DISPLAY_LENGTH = 50
const JOB_DETAILS_PATH = '/tools/career/job-details'

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

  // "Job Match" (step XX) own state — profiles to choose from, the
  // shared AI-platform/model choice (same CrawlPlatformPicker
  // PortalsPage.tsx already uses), and matchPickerJob: the job a
  // profile is currently being picked for (non-null only while 2+
  // profiles exist and the picker Modal is open — a single profile is
  // auto-chosen without ever setting this).
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [platforms, setPlatforms] = useState<Platform[]>([])
  const [selectedPlatformId, setSelectedPlatformId] = useState<string | null>(null)
  const [selectedModel, setSelectedModel] = useState<string | null>(null)
  const [matchPickerJob, setMatchPickerJob] = useState<Job | null>(null)

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
    fetchProfiles()
      .then(setProfiles)
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
    // Same "prefer whichever platform already has a key" preference
    // PortalsPage.tsx's own identical effect uses — a key-list load
    // failure degrades to plain "pick the first platform" rather than
    // blocking selection. See
    // plan/ai/tools/career/step-61-portals-prefer-platform-with-key.md.
    Promise.all([fetchPlatforms(), fetchPlatformKeys().catch(() => [])])
      .then(([list, keys]) => {
        setPlatforms(list)
        if (list.length > 0) {
          const withKey = list.find((p) => keys.some((k) => k.platform === p.id))
          const chosen = withKey ?? list[0]
          setSelectedPlatformId(chosen.id)
          setSelectedModel(chosen.models.length > 0 ? chosen.models[0] : null)
        }
      })
      .catch(() => {
        // Left empty (no platforms) — the Job Match icon already
        // disables itself when platforms.length === 0.
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

  function handleViewDetails(id: string) {
    window.__cinqoToolBridge.navigate(`${JOB_DETAILS_PATH}?id=${encodeURIComponent(id)}`)
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

  // finishMatch is the shared "a Job Match conversation just ended"
  // handling (mirrors CrawlPanel.tsx's own finishGenerateInstructions)
  // — used by both a freshly-started match (startMatch, below) and a
  // resumed one (resumeMatchWatch), so a match that finishes while
  // this tab was away is handled identically to one that finishes
  // live. err is the failure from awaitTurnCompletion, if any;
  // undefined means the turn itself completed successfully — NOT that
  // a score was necessarily saved: save_job_match is the AI's own
  // tool call, made (or not) entirely at its own discretion during
  // that turn, so a "successful" turn that never called it simply
  // leaves matchStatus stuck at 'matching' after reload, same as any
  // other silently-incomplete AI task in this tool.
  function finishMatch(jobId: string, err?: unknown) {
    if (!err) {
      load()
      return
    }
    const text =
      err instanceof CoreApiError && (err.status === 401 || err.status === 403)
        ? 'Ask an admin to grant you access to AI conversations.'
        : err instanceof Error
          ? err.message
          : 'Job match failed.'
    void updateJobMatch(jobId, { status: 'failed', error: text })
      .catch((recordErr: unknown) => {
        console.error('failed to record job match failure', jobId, recordErr)
      })
      .then(() => {
        load()
      })
  }

  // resumeMatchWatch picks a still-in-flight match back up after a
  // reload — job.matchConversationId is the durable trace startMatch
  // (below) already wrote. Mirrors CrawlPanel.tsx's own
  // resumeGenerateInstructions: a status-check failure (network error,
  // not "no turn found") is treated as "still running," optimistic,
  // rather than silently reverting a job that may still be mid-match.
  async function resumeMatchWatch(jobId: string, conversationId: string) {
    let status
    try {
      status = await fetchTurnStatus(conversationId)
    } catch {
      status = 'running' as const
    }
    if (status !== 'running') {
      awaitTurnCompletion(conversationId).then(
        () => {
          finishMatch(jobId)
        },
        (err: unknown) => {
          finishMatch(jobId, err)
        },
      )
      return
    }
    awaitTurnCompletion(conversationId).then(
      () => {
        finishMatch(jobId)
      },
      (err: unknown) => {
        finishMatch(jobId, err)
      },
    )
  }

  // resumedMatchesRef tracks which jobs' own matches this instance has
  // already started (re)watching — a Set, not a single boolean (every
  // other resume-on-reload effect in this tool guards, e.g.
  // CrawlPanel.tsx's own resumedRef), since this is a LIST page with
  // many independently-matching rows, not one entity's own single
  // panel. Once a job id is added it's never removed, so a later
  // unrelated reload (e.g. a filter change) never reopens a watch this
  // instance already has in flight.
  const resumedMatchesRef = useRef<Set<string>>(new Set())
  useEffect(() => {
    for (const job of jobs) {
      if (job.matchStatus === 'matching' && job.matchConversationId && !resumedMatchesRef.current.has(job.id)) {
        resumedMatchesRef.current.add(job.id)
        void resumeMatchWatch(job.id, job.matchConversationId)
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [jobs])

  // startMatch creates a hidden conversation the same way
  // CrawlPanel.tsx's own handleGenerateInstructionsWithAI does, opens
  // the floating chat widget immediately, and persists the
  // conversation id server-side (updateJobMatch) BEFORE sending the
  // actual instruction message — so even a page reload landing in the
  // brief window between those two calls still has something to
  // resume from.
  function startMatch(job: Job, profileId: string) {
    if (!selectedPlatformId) return
    const platform = platforms.find((p) => p.id === selectedPlatformId)
    const model = platform && platform.models.length > 0 ? (selectedModel ?? platform.models[0]) : undefined

    createConversation({ title: jobMatchConversationTitle(job), platformId: selectedPlatformId, model, hidden: true })
      .then((conversation) => {
        window.__cinqoToolBridge.openConversation(conversation.id)
        return updateJobMatch(job.id, { profileId, status: 'matching', conversationId: conversation.id })
          .catch(() => {
            // Best-effort — a failure here only costs reload-resilience
            // for this one attempt, not the attempt itself.
          })
          .then(() => sendMessage(conversation.id, buildJobMatchMessage(job, profileId)))
      })
      .then(
        () => {
          finishMatch(job.id)
        },
        (err: unknown) => {
          finishMatch(job.id, err)
        },
      )
    // Reflect "matching" immediately rather than waiting for the round
    // trip above to land — load() after the round trip already
    // reconciles with the server's own real state regardless.
    setJobs((prev) =>
      prev.map((j) => (j.id === job.id ? { ...j, matchStatus: 'matching', matchScore: undefined, matchError: undefined } : j)),
    )
  }

  function handleMatchClick(job: Job) {
    if (job.matchStatus === 'matching' || !selectedPlatformId) return
    if (profiles.length === 0) return
    if (profiles.length === 1) {
      startMatch(job, profiles[0].id)
      return
    }
    setMatchPickerJob(job)
  }

  function handleCheckMatchProgress(job: Job) {
    if (job.matchConversationId) {
      window.__cinqoToolBridge.openConversation(job.matchConversationId)
    }
  }

  const companyNames: Record<string, string> = Object.fromEntries(companies.map((c) => [c.id, c.name]))
  const portalNames: Record<string, string> = Object.fromEntries(portals.map((p) => [p.id, p.name]))

  const activeFilters: { key: string; label: string; onClear: () => void }[] = []
  if (location)
    activeFilters.push({
      key: 'location',
      label: `Location: ${truncate(location, MAX_LOCATION_DISPLAY_LENGTH)}`,
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
    <div className="max-w-5xl p-6 font-sans text-gray-900">
      <h1 className="mb-1.5 text-xl font-semibold">Saved Jobs</h1>
      <p className="mb-5 text-sm text-gray-500">Job postings the AI has crawled and saved on your behalf.</p>

      <CrawlPlatformPicker
        platforms={platforms}
        selectedPlatformId={selectedPlatformId}
        selectedModel={selectedModel}
        onSelectPlatform={(id) => {
          setSelectedPlatformId(id)
          const next = platforms.find((p) => p.id === id)
          setSelectedModel(next && next.models.length > 0 ? next.models[0] : null)
        }}
        onSelectModel={setSelectedModel}
      />

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

      {showFilters && (
        <div className="mb-2 flex flex-wrap items-center gap-2">
          {locations.length > 0 && (
            <Dropdown
              placeholder="Any location"
              options={[
                { value: '', label: 'Any location' },
                ...locations.map((loc) => ({ value: loc, label: truncate(loc, MAX_LOCATION_DISPLAY_LENGTH) })),
              ]}
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
                <th className="border-b border-gray-200 bg-gray-50 p-3 text-left font-medium text-gray-500">Match</th>
                <th className="border-b border-gray-200 bg-gray-50 p-3 text-right font-medium text-gray-500">Actions</th>
              </tr>
            </thead>
            <tbody>
              {jobs.map((job) => {
                // hasDescription/failed/neverCrawled (step XX) drive the
                // Eye icon's own three visual states — see
                // detailCrawlStatus's own doc comment (api.ts) for why
                // this is a SEPARATE signal from the job's own crawledAt
                // (which every job has, set unconditionally by the
                // listing crawl, and says nothing about whether the
                // job's own DETAIL page was ever separately crawled).
                const hasDescription = job.description.trim() !== ''
                const failed = !hasDescription && job.detailCrawlStatus === 'failed'
                const neverCrawled = !hasDescription && !job.detailCrawlStatus
                return (
                  <tr key={job.id} className="last:[&>td]:border-b-0 hover:bg-gray-50">
                    <td className="border-b border-gray-200 p-3 text-gray-900">{job.title}</td>
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
                      {job.matchStatus === 'matching' ? (
                        <button
                          type="button"
                          onClick={() => {
                            handleCheckMatchProgress(job)
                          }}
                          title="Open the chat window to watch the AI work on this job's own match"
                          className="inline-flex items-center gap-1 text-xs text-gray-500 underline hover:text-gray-700"
                        >
                          <span className="inline-flex items-center gap-1 [animation:robot-bob_1.6s_ease-in-out_infinite]">
                            <RobotIcon />
                            <Typewriter text="Matching…" />
                          </span>
                        </button>
                      ) : job.matchStatus === 'failed' ? (
                        <span className="text-xs text-red-700" title={job.matchError ?? 'Job match failed'}>
                          Failed
                        </span>
                      ) : job.matchStatus === 'completed' && job.matchScore !== undefined ? (
                        <MatchScoreBar score={job.matchScore} />
                      ) : (
                        <span className="text-xs text-gray-300">—</span>
                      )}
                    </td>
                    <td className="border-b border-gray-200 p-3">
                      <div className="flex items-center justify-end gap-1">
                        {job.matchStatus !== 'matching' && (
                          <button
                            type="button"
                            onClick={() => {
                              handleMatchClick(job)
                            }}
                            disabled={!selectedPlatformId || profiles.length === 0}
                            title={
                              !selectedPlatformId
                                ? 'No AI platform configured — add one on the Platforms page first'
                                : profiles.length === 0
                                  ? 'Create a profile first'
                                  : 'Job Match — assess how well this job fits a profile'
                            }
                            className="rounded-md p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900 disabled:cursor-not-allowed disabled:opacity-40"
                          >
                            <MatchIcon />
                          </button>
                        )}
                        <a
                          href={job.sourceUrl}
                          target="_blank"
                          rel="noreferrer"
                          title="Open the original posting"
                          className="rounded-md p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900"
                        >
                          <ExternalLinkIcon />
                        </a>
                        {failed ? (
                          <button
                            type="button"
                            disabled
                            title="Failed to crawl"
                            className="cursor-not-allowed rounded-md p-1.5 text-red-400"
                          >
                            <EyeIcon />
                          </button>
                        ) : (
                          <button
                            type="button"
                            onClick={() => {
                              handleViewDetails(job.id)
                            }}
                            title={neverCrawled ? 'Not crawled yet' : 'View job details'}
                            className={
                              neverCrawled
                                ? 'rounded-md p-1.5 text-yellow-500 transition-colors hover:bg-yellow-50'
                                : 'rounded-md p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900'
                            }
                          >
                            <EyeIcon />
                          </button>
                        )}
                        <button
                          type="button"
                          onClick={() => {
                            handleRemove(job.id)
                          }}
                          title="Delete this job"
                          className="rounded-md p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-700"
                        >
                          <TrashIcon />
                        </button>
                      </div>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      </div>

      <Pagination page={page} pageSize={PAGE_SIZE} total={total} onPageChange={handlePageChange} />

      <Modal
        open={matchPickerJob !== null}
        title="Choose a profile to match against"
        onClose={() => {
          setMatchPickerJob(null)
        }}
      >
        <div className="flex flex-col gap-1.5">
          {profiles.map((profile) => (
            <button
              key={profile.id}
              type="button"
              onClick={() => {
                if (matchPickerJob) startMatch(matchPickerJob, profile.id)
                setMatchPickerJob(null)
              }}
              className="rounded-md border border-gray-200 px-3 py-2 text-left text-sm text-gray-900 hover:bg-gray-50"
            >
              {profile.firstName} {profile.lastName}
            </button>
          ))}
        </div>
      </Modal>
    </div>
  )
}
