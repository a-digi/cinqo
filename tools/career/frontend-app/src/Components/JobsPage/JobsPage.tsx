import { useEffect, useRef, useState } from 'react'
import {
  fetchJobs,
  fetchJob,
  removeJob,
  fetchCompanies,
  fetchPortals,
  fetchJobLocations,
  linkJobToCompany,
  fetchProfiles,
  updateJobMatch,
  updateJobCv,
  unlinkJobCv,
  mediaDownloadUrl,
  fetchCvGenerations,
  type Job,
  type Company,
  type Portal,
  type Profile,
  type CvGeneration,
} from '../../api'
import { useAiChoice } from '../../Cinqo/Platform/useAiChoice'
import { createConversation, sendMessage, awaitTurnCompletion, fetchTurnStatus } from '../../Cinqo/Conversation/conversation'
import { CoreApiError } from '../../Cinqo/Http/client'
import { buildJobMatchMessage, jobMatchConversationTitle } from '../../JobMatch/buildJobMatchMessage'
import { buildGenerateCvMessage, generateCvConversationTitle } from '../../CvPdf/buildGenerateCvMessage'
import { Dropdown } from '../Dropdown/Dropdown'
import { Pagination } from '../Pagination/Pagination'
import { Modal } from '../../Shared/Modal/Modal'
import { MatchScoreBar } from '../../Shared/MatchScoreBar/MatchScoreBar'
import { ActionMenu, type ActionMenuItem } from '../../Shared/ActionMenu/ActionMenu'
import {
  BriefcaseIcon,
  FilterIcon,
  XIcon,
  EyeIcon,
  ExternalLinkIcon,
  TrashIcon,
  MatchIcon,
  RobotIcon,
  PDFIcon,
  PlusIcon,
} from '../../Shared/Icons/icons'
import { AddJobModal } from './AddJobModal'
import { companyCell } from './companyCell'
import { VIEW_COMPANY_PATH, VIEW_PORTAL_PATH } from '../../paths'
import { PageHeaderCard } from '../../Shared/PageHeader/PageHeaderCard'
import { PAGE_ACCENT, accentFor, initialOf } from './jobsTheme'
import { JobStatsStrip } from './JobStatsStrip'
import { SelectCvModal } from './SelectCvModal'
import { AddToBoardModal } from '../JobBoard/AddToBoardModal'
import { plural } from '../CompaniesPage/companyStats'
import { ConfirmationModal } from '../../Shared/ConfirmationModal/ConfirmationModal'
import {
  REMOVE_CONFIRMATION,
  builderPathForJob,
  choiceLabels,
  cvChangeDisabledReason,
  replaceConfirmation,
  showRemove,
  sourceNote,
} from './cvActions'
import { canOpenOriginal, detailsState, isManualJob, platformLabel } from './manualJob'
import { Typewriter } from '../../Shared/Typewriter/Typewriter'
import { truncate } from '../../Shared/Text/transform'
import { PageTips } from '../../Shared/PageTips/PageTips'

const PAGE_SIZE = 50
const SEARCH_DEBOUNCE_MS = 300
const MAX_LOCATION_DISPLAY_LENGTH = 50
const JOB_DETAILS_PATH = '/tools/career/job-details'

// Jobs are populated by the AI's own crawling workflow (step 4); step 97
// added "Add job" for one a person found themselves (see AddJobModal).
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
  // companyIds (multi-select, step 83) — seeded from every ?companyId=
  // occurrence in the URL via getAll, so both a single legacy link
  // (CompaniesPage's own "N linked jobs") and a future multi-value link
  // arrive pre-filtered identically.
  const [companyIds, setCompanyIds] = useState<string[]>(() => new URLSearchParams(window.location.search).getAll('companyId'))
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
    return params.has('companyId') || params.has('portalId')
  })

  // "Job Match" (step XX) own state — profiles to choose from, the
  // shared AI-platform/model choice (the saved Settings choice, via useAiChoice
  // PortalsPage.tsx already uses), and matchPickerJob: the job a
  // profile is currently being picked for (non-null only while 2+
  // profiles exist and the picker Modal is open — a single profile is
  // auto-chosen without ever setting this).
  const [profiles, setProfiles] = useState<Profile[]>([])
  // The AI platform and model are no longer chosen on this page: they are one
  // saved setting for the whole tool (Settings). Job Match and Generate CV use it.
  const ai = useAiChoice()
  const selectedPlatformId = ai.platform?.id ?? null
  const [matchPickerJob, setMatchPickerJob] = useState<Job | null>(null)

  // "Add job" (step 97) — the modal for a job added by hand.
  const [addOpen, setAddOpen] = useState(false)

  // "Generate CV PDF" (step XX) own state — cvPickerJob mirrors
  // matchPickerJob exactly: the job a profile is currently being
  // picked for (non-null only while 2+ profiles exist and the picker
  // Modal is open).
  const [cvPickerJob, setCvPickerJob] = useState<Job | null>(null)

  // "Generate manually" / "Select existing" / "Remove CV" (step 98): the job whose
  // "Select existing" modal is open, and the jobs awaiting a confirmation before
  // the CV Builder opens (replacing a CV) or the CV is removed.
  const [selectCvJob, setSelectCvJob] = useState<Job | null>(null)
  const [pendingManualCvJob, setPendingManualCvJob] = useState<Job | null>(null)
  const [pendingRemoveCvJob, setPendingRemoveCvJob] = useState<Job | null>(null)
  // The job being added to a profile's job board (step 126).
  const [addToBoardJob, setAddToBoardJob] = useState<Job | null>(null)

  // CV generation audit trail (step 5) — historyJob is the job whose
  // history modal is currently open (mirrors matchPickerJob/cvPickerJob's
  // own "non-null only while its own Modal is open" shape). Fetched
  // lazily, only when the modal actually opens for a given job — not
  // eagerly for every job in the list. See
  // plan/ai/career/cv-generation-audit/step-05-jobs-page-ui.md.
  const [historyJob, setHistoryJob] = useState<Job | null>(null)
  const [cvGenerations, setCvGenerations] = useState<CvGeneration[]>([])
  const [historyLoading, setHistoryLoading] = useState(false)
  const [historyError, setHistoryError] = useState('')

  function openCvHistory(job: Job) {
    setHistoryJob(job)
    setCvGenerations([])
    setHistoryError('')
    setHistoryLoading(true)
    fetchCvGenerations(job.id)
      .then(setCvGenerations)
      .catch((err: unknown) => {
        setHistoryError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        setHistoryLoading(false)
      })
  }

  function load(companyIdsOverride?: string[], portalIdOverride?: string, pageOverride?: number, locationOverride?: string) {
    setError('')
    const effectivePage = pageOverride ?? page
    fetchJobs(
      query,
      locationOverride ?? location,
      companyIdsOverride ?? companyIds,
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
    load(companyIds, portalId, 1)
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

  function handleCompanyFilterChange(ids: string[]) {
    setCompanyIds(ids)
    setPage(1)
    load(ids, undefined, 1)
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
  // live. err is the failure from awaitTurnCompletion, if any.
  //
  // undefined does NOT mean the match itself finished — only that the
  // ORCHESTRATOR's own turn did. Under this codebase's own "reply
  // returns early" architecture (api/src/conversation/subagent.go), a
  // turn is marked complete the moment the orchestrator's own model
  // call finishes, even if a sub-agent it spawned (e.g. to crawl a
  // missing description) is still running — and separately, the model
  // may simply end its own turn without ever getting around to calling
  // save_job_match. Either way, job_matches would still be sitting at
  // 'matching' with nothing left watching it, so the real, server-side
  // outcome is checked directly here rather than assumed from the turn
  // alone; a turn that ended without producing a completed match is
  // recorded as failed, matching this tool's own established
  // "reconcile/reap what never finished" philosophy elsewhere
  // (crawl_runs' own stale-run reaper) rather than leaving an
  // indefinite, silently-stuck "Matching…" state.
  function finishMatch(jobId: string, err?: unknown) {
    if (err) {
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
      return
    }
    void fetchJob(jobId)
      .then((job) => {
        if (job.matchStatus === 'matching') {
          return updateJobMatch(jobId, {
            status: 'failed',
            error: 'The AI finished without completing the match — try again.',
          })
        }
      })
      .catch((recordErr: unknown) => {
        console.error('failed to verify job match outcome', jobId, recordErr)
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

  // finishCvGeneration is the shared "a Generate CV PDF conversation
  // just ended" handling, mirroring finishMatch above exactly — used
  // by both a freshly-started generation (startCvGeneration, below)
  // and a resumed one (resumeCvWatch). The AI's own save_cv_document MCP
  // tool call, together with the platform's own conversation
  // orchestrator (which promotes the rendered PDF into permanent Media
  // storage in-process before that call even reaches this tool's own
  // backend — see cv_pdf.go's own top doc comment), already settles
  // cvStatus at 'completed' by the time a turn finishes successfully —
  // this function never touches Media or any tool's own proxy route
  // itself, it only re-verifies the real, server-side outcome (a turn
  // can end without the model ever calling save_cv_document) rather than
  // assuming success from the turn alone.
  function finishCvGeneration(jobId: string, err?: unknown) {
    if (err) {
      const text =
        err instanceof CoreApiError && (err.status === 401 || err.status === 403)
          ? 'Ask an admin to grant you access to AI conversations.'
          : err instanceof Error
            ? err.message
            : 'CV generation failed.'
      void updateJobCv(jobId, { status: 'failed', error: text })
        .catch((recordErr: unknown) => {
          console.error('failed to record cv generation failure', jobId, recordErr)
        })
        .then(() => {
          load()
        })
      return
    }
    void fetchJob(jobId)
      .then((job) => {
        if (job.cvStatus === 'generating') {
          return updateJobCv(jobId, {
            status: 'failed',
            error: 'The AI finished without producing a CV — try again.',
          })
        }
      })
      .catch((recordErr: unknown) => {
        console.error('failed to verify cv generation outcome', jobId, recordErr)
      })
      .then(() => {
        load()
      })
  }

  // resumeCvWatch picks a still-in-flight CV generation back up after a
  // reload — mirrors resumeMatchWatch exactly.
  async function resumeCvWatch(jobId: string, conversationId: string) {
    let status
    try {
      status = await fetchTurnStatus(conversationId)
    } catch {
      status = 'running' as const
    }
    if (status !== 'running') {
      awaitTurnCompletion(conversationId).then(
        () => {
          finishCvGeneration(jobId)
        },
        (err: unknown) => {
          finishCvGeneration(jobId, err)
        },
      )
      return
    }
    awaitTurnCompletion(conversationId).then(
      () => {
        finishCvGeneration(jobId)
      },
      (err: unknown) => {
        finishCvGeneration(jobId, err)
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
      if (job.matchStatus !== 'matching' || resumedMatchesRef.current.has(job.id)) continue
      resumedMatchesRef.current.add(job.id)
      if (job.matchConversationId) {
        void resumeMatchWatch(job.id, job.matchConversationId)
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [jobs])

  // resumedCvRef mirrors resumedMatchesRef exactly.
  const resumedCvRef = useRef<Set<string>>(new Set())
  useEffect(() => {
    for (const job of jobs) {
      if (job.cvStatus !== 'generating' || resumedCvRef.current.has(job.id)) continue
      resumedCvRef.current.add(job.id)
      if (job.cvConversationId) {
        void resumeCvWatch(job.id, job.cvConversationId)
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
    if (!ai.platform) return
    const model = ai.model ?? undefined

    createConversation({ title: jobMatchConversationTitle(job), platformId: ai.platform.id, model, hidden: true })
      .then((conversation) => {
        window.__cinqoToolBridge.openConversation(conversation.id)
        // The initial optimistic update below (fired synchronously,
        // before this network round trip even lands) has no
        // conversation id to set yet — without ALSO updating it here,
        // the "Matching…" indicator's own click handler
        // (handleCheckMatchProgress) would keep checking a stale/empty
        // matchConversationId and silently do nothing until the next
        // full reload re-fetches the real value from the server. This
        // is what makes it clickable immediately instead.
        setJobs((prev) => prev.map((j) => (j.id === job.id ? { ...j, matchConversationId: conversation.id } : j)))
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

  // startCvGeneration mirrors startMatch's own shape (opened
  // immediately, conversation id persisted server-side via updateJobCv
  // BEFORE the actual instruction message is sent), with one deliberate
  // difference: NOT hidden, per your own instruction — this conversation
  // should show up in the normal /conversations list, for auditability.
  // `hidden` is set once, at creation, with no toggle path anywhere in
  // this codebase — conversations created before this change stay
  // hidden forever; only new ones from here on show up in the list. See
  // plan/ai/career/cv-generation-audit/step-04-unhide-conversation.md.
  function startCvGeneration(job: Job, profileId: string) {
    if (!ai.platform) return
    const model = ai.model ?? undefined

    createConversation({ title: generateCvConversationTitle(job), platformId: ai.platform.id, model, hidden: false })
      .then((conversation) => {
        window.__cinqoToolBridge.openConversation(conversation.id)
        setJobs((prev) => prev.map((j) => (j.id === job.id ? { ...j, cvConversationId: conversation.id } : j)))
        return updateJobCv(job.id, { profileId, status: 'generating', conversationId: conversation.id })
          .catch(() => {
            // Best-effort — a failure here only costs reload-resilience
            // for this one attempt, not the attempt itself.
          })
          .then(() => sendMessage(conversation.id, buildGenerateCvMessage(job, profileId)))
      })
      .then(
        () => {
          finishCvGeneration(job.id)
        },
        (err: unknown) => {
          finishCvGeneration(job.id, err)
        },
      )
    setJobs((prev) =>
      prev.map((j) => (j.id === job.id ? { ...j, cvStatus: 'generating', cvError: undefined, cvMediaFileId: undefined } : j)),
    )
  }

  function handleGenerateCvClick(job: Job) {
    if (job.cvStatus === 'generating' || !selectedPlatformId) return
    if (profiles.length === 0) return
    if (profiles.length === 1) {
      startCvGeneration(job, profiles[0].id)
      return
    }
    setCvPickerJob(job)
  }

  // "Generate manually" opens the CV Builder for this job; replacing a CV the job
  // already has asks first.
  function handleManualCvClick(job: Job) {
    if (replaceConfirmation(job, 'manual')) {
      setPendingManualCvJob(job)
      return
    }
    window.__cinqoToolBridge.navigate(builderPathForJob(job.id))
  }

  function confirmRemoveCv() {
    if (!pendingRemoveCvJob) return
    const job = pendingRemoveCvJob
    setPendingRemoveCvJob(null)
    setError('')
    unlinkJobCv(job.id)
      .then(() => {
        load()
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : String(err))
      })
  }

  function handleCheckCvProgress(job: Job) {
    if (job.cvConversationId) {
      window.__cinqoToolBridge.openConversation(job.cvConversationId)
    }
  }

  // buildJobActionItems assembles one row's own ActionMenu entries —
  // the exact same five actions (Job Match, Generate/Download CV, Open
  // original posting, View details, Delete) and the exact same
  // enabled/disabled/tooltip/color logic the old inline icon row used,
  // just expressed as data for the shared ActionMenu component
  // (Shared/ActionMenu/ActionMenu.tsx) to render instead of JSX
  // per-icon. hasDescription/failed/neverCrawled are recomputed here
  // (not threaded in from the row map below) so this function stays a
  // plain "job in, menu items out" mapping any other caller could
  // reuse without also having to reach into the row-rendering loop.
  function buildJobActionItems(job: Job): ActionMenuItem[] {
    const details = detailsState(job) // a manual job's details are always available (step 97)
    const failed = details === 'failed'
    const neverCrawled = details === 'not-crawled'

    const items: ActionMenuItem[] = []

    if (job.matchStatus === 'matching') {
      items.push({
        key: 'match',
        label: 'Watch AI matching…',
        icon: <RobotIcon />,
        onClick: () => {
          handleCheckMatchProgress(job)
        },
        title: "Open the chat window to watch the AI work on this job's own match",
      })
    } else {
      items.push({
        key: 'match',
        label: 'Job Match',
        icon: <MatchIcon />,
        onClick: () => {
          handleMatchClick(job)
        },
        disabled: !selectedPlatformId || profiles.length === 0,
        title: !selectedPlatformId
          ? 'No AI platform configured — add one in Cinqo first (see Career ▸ Settings)'
          : profiles.length === 0
            ? 'Create a profile first'
            : 'Assess how well this job fits a profile',
      })
    }

    items.push({
      key: 'board',
      label: 'Add to job board',
      icon: <BriefcaseIcon className="h-3.5 w-3.5" />,
      onClick: () => {
        setAddToBoardJob(job)
      },
      disabled: profiles.length === 0,
      title: profiles.length === 0 ? 'Create a profile first' : 'Put this job on a profile’s job board',
    })

    // A single "CV" parent entry with no onClick/href of its own — the
    // row itself is purely a toggle for its own children below, which
    // vary by job.cvStatus (the in-progress watch link, the ready
    // download/regenerate pair, or the not-yet-generated/failed
    // generate link) plus the always-present generation-history link.
    // Unified from what used to be up to two separate top-level entries
    // (a bare 'cv' leaf/group plus a standalone 'cv-history' leaf) into
    // one consistent "CV" entry point regardless of state.
    const cvChildren: ActionMenuItem[] = []
    // The two choices that do not use the AI (step 98), and removing the CV; all
    // three are blocked only while an AI generation is running.
    const choice = choiceLabels(job)
    const changeBlocked = cvChangeDisabledReason(job)
    const manualItem: ActionMenuItem = {
      key: 'cv-manual',
      label: choice.manual,
      icon: <PlusIcon />,
      onClick: () => {
        handleManualCvClick(job)
      },
      disabled: changeBlocked !== undefined,
      title: changeBlocked ?? 'Build this job’s CV yourself in the CV Builder',
    }
    const existingItem: ActionMenuItem = {
      key: 'cv-existing',
      label: choice.existing,
      icon: <PDFIcon className="h-3.5 w-3.5" />,
      onClick: () => {
        setSelectCvJob(job)
      },
      disabled: changeBlocked !== undefined,
      title: changeBlocked ?? 'Attach a CV you already made to this job',
    }
    let cvVariant: ActionMenuItem['variant'] = 'default'
    let cvTitle = 'Generate a CV PDF tailored to this job'

    if (job.cvStatus === 'generating') {
      cvTitle = 'AI is generating this CV — click to watch'
      cvChildren.push({
        key: 'cv-watch',
        label: 'Watch AI generating CV…',
        icon: <RobotIcon />,
        onClick: () => {
          handleCheckCvProgress(job)
        },
        title: 'Open the chat window to watch the AI generate this CV',
      })
      cvChildren.push(manualItem, existingItem)
    } else if (job.cvStatus === 'completed' && job.cvMediaFileId) {
      cvVariant = 'success'
      cvTitle = 'CV ready — download it, or have the AI try again'
      cvChildren.push(
        {
          key: 'cv-download',
          label: 'Download CV',
          icon: <PDFIcon className="h-3.5 w-3.5" />,
          href: mediaDownloadUrl(job.cvMediaFileId),
          target: '_blank',
          rel: 'noreferrer',
          title: sourceNote(job.cvSource) ? `Download the CV — ${sourceNote(job.cvSource).toLowerCase()}` : 'Download the CV',
          variant: 'success',
        },
        {
          key: 'cv-regenerate',
          label: choice.ai,
          icon: <RobotIcon />,
          onClick: () => {
            handleGenerateCvClick(job)
          },
          disabled: !selectedPlatformId || profiles.length === 0,
          title: !selectedPlatformId
            ? 'No AI platform configured — add one in Cinqo first (see Career ▸ Settings)'
            : profiles.length === 0
              ? 'Create a profile first'
              : 'Not happy with this CV? Have the AI build a new one.',
        },
        manualItem,
        existingItem,
      )
      if (showRemove(job)) {
        cvChildren.push({
          key: 'cv-remove',
          label: 'Remove CV from this job',
          icon: <TrashIcon />,
          onClick: () => {
            setPendingRemoveCvJob(job)
          },
          title: 'Remove the link to this CV; the CV itself stays in your CV list',
          variant: 'danger',
        })
      }
    } else {
      if (job.cvStatus === 'failed') cvVariant = 'danger'
      cvTitle = !selectedPlatformId
        ? 'No AI platform configured — add one in Cinqo first (see Career ▸ Settings)'
        : profiles.length === 0
          ? 'Create a profile first'
          : job.cvStatus === 'failed'
            ? (job.cvError ?? 'Failed to generate CV — click to retry')
            : 'Generate a CV PDF tailored to this job'
      cvChildren.push({
        key: 'cv-generate',
        label: choice.ai,
        icon: <PDFIcon className="h-3.5 w-3.5" />,
        onClick: () => {
          handleGenerateCvClick(job)
        },
        disabled: !selectedPlatformId || profiles.length === 0,
        title: cvTitle,
        variant: job.cvStatus === 'failed' ? 'danger' : 'default',
      })
      cvChildren.push(manualItem, existingItem)
    }

    if (job.cvStatus) {
      cvChildren.push({
        key: 'cv-history',
        label: 'View CV generation history',
        icon: <RobotIcon />,
        onClick: () => {
          openCvHistory(job)
        },
        title: 'Every AI conversation that has ever generated a CV for this job — for auditing',
      })
    }

    items.push({
      key: 'cv',
      label: 'CV',
      icon: <PDFIcon className="h-3.5 w-3.5" />,
      title: cvTitle,
      variant: cvVariant,
      children: cvChildren,
    })

    // A job added by hand without a link has nothing to open (step 97).
    if (canOpenOriginal(job)) {
      items.push({
        key: 'original',
        label: 'Open original posting',
        icon: <ExternalLinkIcon />,
        href: job.sourceUrl,
        target: '_blank',
        rel: 'noreferrer',
      })
    }

    items.push({
      key: 'details',
      label: 'View details',
      icon: <EyeIcon />,
      onClick: () => {
        handleViewDetails(job.id)
      },
      disabled: failed,
      title: failed ? 'Failed to crawl' : neverCrawled ? 'Not crawled yet' : 'View job details',
    })

    items.push({
      key: 'delete',
      label: 'Delete',
      icon: <TrashIcon />,
      onClick: () => {
        handleRemove(job.id)
      },
      title: 'Delete this job',
      variant: 'danger',
    })

    return items
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
  for (const id of companyIds)
    activeFilters.push({
      key: `company-${id}`,
      label: `Company: ${companyNames[id] ?? '…'}`,
      onClear: () => {
        handleCompanyFilterChange(companyIds.filter((c) => c !== id))
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
    <div className="career-a-sheen career-t-underline max-w-7xl p-6 font-sans text-gray-900">
      <div className="min-w-0">
        <PageHeaderCard
          icon={<BriefcaseIcon />}
          title="Saved Jobs"
          description="Job postings the AI has crawled and saved on your behalf, and any you added yourself."
          pill={{ text: `${total} ${total === 1 ? 'job' : 'jobs'}` }}
        />
        {/* Two columns: the jobs on the left (the point of the page), the statistics cards
            on the right. Below the large breakpoint the cards drop under the list. */}
        <div className="flex flex-col gap-5 lg:flex-row lg:items-start">
          <div className="career-enter career-delay-1 min-w-0 flex-1">
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
                    multiple
                    placeholder="Any company"
                    options={companies.map((c) => ({ value: c.id, label: c.name }))}
                    value={companyIds}
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
              <button
                type="button"
                onClick={() => {
                  setAddOpen(true)
                }}
                className="flex items-center gap-1.5 rounded-md bg-gray-800 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-900"
              >
                <PlusIcon />
                Add job
              </button>
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

            <div
              className="relative overflow-hidden rounded-3xl border shadow-sm"
              style={{ borderColor: PAGE_ACCENT.soft, background: `linear-gradient(170deg, #ffffff 55%, ${PAGE_ACCENT.soft} 100%)` }}
            >
              <div
                className="pointer-events-none absolute -bottom-12 -right-10 h-40 w-40 rounded-full border-[16px] opacity-10"
                style={{ borderColor: PAGE_ACCENT.from }}
              />
              <div className="relative overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr>
                      <th className="bg-blue-50/70 p-3 text-left text-xs font-semibold uppercase tracking-wide text-blue-700">Title</th>
                      <th className="bg-blue-50/70 p-3 text-left text-xs font-semibold uppercase tracking-wide text-blue-700">Company</th>
                      <th className="bg-blue-50/70 p-3 text-left text-xs font-semibold uppercase tracking-wide text-blue-700">Platform</th>
                      <th className="bg-blue-50/70 p-3 text-left text-xs font-semibold uppercase tracking-wide text-blue-700">Posted</th>
                      <th className="bg-blue-50/70 p-3 text-left text-xs font-semibold uppercase tracking-wide text-blue-700">Match</th>
                      <th className="bg-blue-50/70 p-3 text-right text-xs font-semibold uppercase tracking-wide text-blue-700">Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {jobs.map((job) => {
                      const cell = companyCell(job, companyNames, VIEW_COMPANY_PATH)
                      return (
                        <tr key={job.id} className="transition-colors last:[&>td]:border-b-0 hover:bg-blue-50/50">
                          <td className="border-b border-blue-50 p-3 text-gray-900">
                            <div className="flex items-center gap-3">
                              <span
                                className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-sm font-bold text-white shadow-sm"
                                style={{ background: accentFor(job.company || job.title).to }}
                                aria-hidden="true"
                              >
                                {initialOf(job.company || job.title)}
                              </span>
                              <span className="font-medium">{job.title}</span>
                              {job.boardProfileCount > 0 && (
                                <a
                                  href={`${JOB_DETAILS_PATH}?id=${encodeURIComponent(job.id)}`}
                                  onClick={(e) => {
                                    e.preventDefault()
                                    window.__cinqoToolBridge.navigate(`${JOB_DETAILS_PATH}?id=${encodeURIComponent(job.id)}`)
                                  }}
                                  title="Profiles whose job board has this job (open the job to see the status on each)"
                                  className="rounded-full bg-blue-100 px-2 py-0.5 text-xs font-medium text-blue-800 hover:underline"
                                >
                                  On {plural(job.boardProfileCount, 'board', 'boards')}
                                </a>
                              )}
                              {isManualJob(job) && (
                                <span
                                  title="You added this job yourself"
                                  className="rounded-full bg-violet-100 px-2 py-0.5 text-xs font-medium text-violet-800"
                                >
                                  Manual
                                </span>
                              )}
                            </div>
                          </td>
                          <td className="border-b border-blue-50 p-3">
                            {cell.linkTo ? (
                              <a
                                href={cell.linkTo}
                                onClick={(e) => {
                                  e.preventDefault()
                                  window.__cinqoToolBridge.navigate(cell.linkTo ?? '')
                                }}
                                title="Open this company's page"
                                className="font-medium text-blue-700 hover:underline focus:outline-none focus-visible:underline"
                              >
                                {cell.text}
                              </a>
                            ) : (
                              cell.text
                            )}
                            {cell.linkTo && (
                              <div className="mt-0.5 flex items-center gap-1.5 text-xs text-gray-400">
                                {cell.linkedName !== null && <>linked to {cell.linkedName}</>}
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
                          <td className="border-b border-blue-50 p-3">
                            {platformLabel(job) === '—' ? (
                              <span className="text-gray-300">—</span>
                            ) : job.portalId ? (
                              <a
                                href={`${VIEW_PORTAL_PATH}?id=${encodeURIComponent(job.portalId)}`}
                                onClick={(e) => {
                                  e.preventDefault()
                                  window.__cinqoToolBridge.navigate(`${VIEW_PORTAL_PATH}?id=${encodeURIComponent(job.portalId ?? '')}`)
                                }}
                                title="Open this portal's page"
                                className="inline-flex rounded-full px-2.5 py-0.5 text-xs font-medium hover:underline"
                                style={{ background: accentFor(platformLabel(job)).soft, color: accentFor(platformLabel(job)).text }}
                              >
                                {platformLabel(job)}
                              </a>
                            ) : (
                              <span
                                className="inline-flex rounded-full px-2.5 py-0.5 text-xs font-medium"
                                style={{ background: accentFor(platformLabel(job)).soft, color: accentFor(platformLabel(job)).text }}
                              >
                                {platformLabel(job)}
                              </span>
                            )}
                          </td>
                          <td className="border-b border-blue-50 p-3">
                            {job.postedAt ? (
                              <span className="rounded-full bg-gray-100 px-2.5 py-0.5 text-xs text-gray-600">{job.postedAt}</span>
                            ) : null}
                          </td>
                          <td className="border-b border-blue-50 p-3">
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
                              <MatchScoreBar score={job.matchScore} skills={job.matchedSkills} jobTitle={job.title} />
                            ) : (
                              <span className="text-xs text-gray-300">—</span>
                            )}
                          </td>
                          <td className="border-b border-blue-50 p-3 text-right">
                            <ActionMenu triggerLabel={`Actions for ${job.title}`} items={buildJobActionItems(job)} />
                          </td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
              </div>
            </div>

            <Pagination page={page} pageSize={PAGE_SIZE} total={total} onPageChange={handlePageChange} />
          </div>

          {/* Refetched when a job is added, deleted or finishes matching / its CV: the page's
              own total, and how many jobs on this page have a completed match or CV. */}
          <aside className="career-enter career-delay-2 w-full shrink-0 career-rail-sticky lg:w-72">
            <JobStatsStrip
              refreshKey={`${total}:${jobs.filter((j) => j.matchStatus === 'completed').length}:${jobs.filter((j) => j.cvStatus === 'completed').length}`}
            />
            <div className="mt-8">
              <PageTips page="jobs" orientation="vertical" />
            </div>
          </aside>
        </div>
      </div>

      <AddJobModal
        open={addOpen}
        onClose={() => {
          setAddOpen(false)
        }}
        onAdded={() => {
          setAddOpen(false)
          setPage(1)
          load(undefined, undefined, 1)
        }}
        onOpenExisting={handleViewDetails}
      />
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
                if (matchPickerJob) {
                  startMatch(matchPickerJob, profile.id)
                }
                setMatchPickerJob(null)
              }}
              className="rounded-md border border-gray-200 px-3 py-2 text-left text-sm text-gray-900 hover:bg-gray-50"
            >
              {profile.firstName} {profile.lastName}
            </button>
          ))}
        </div>
      </Modal>

      <Modal
        open={cvPickerJob !== null}
        title="Choose a profile to build the CV for"
        onClose={() => {
          setCvPickerJob(null)
        }}
      >
        <div className="flex flex-col gap-1.5">
          {profiles.map((profile) => (
            <button
              key={profile.id}
              type="button"
              onClick={() => {
                if (cvPickerJob) {
                  startCvGeneration(cvPickerJob, profile.id)
                }
                setCvPickerJob(null)
              }}
              className="rounded-md border border-gray-200 px-3 py-2 text-left text-sm text-gray-900 hover:bg-gray-50"
            >
              {profile.firstName} {profile.lastName}
            </button>
          ))}
        </div>
      </Modal>

      <AddToBoardModal
        job={addToBoardJob}
        onClose={() => {
          setAddToBoardJob(null)
        }}
        onAdded={(count) => {
          if (addToBoardJob) {
            const id = addToBoardJob.id
            setJobs((prev) => prev.map((j) => (j.id === id ? { ...j, boardProfileCount: j.boardProfileCount + count } : j)))
          }
        }}
      />

      <SelectCvModal
        job={selectCvJob}
        onClose={() => {
          setSelectCvJob(null)
        }}
        onLinked={load}
      />

      <ConfirmationModal
        open={pendingManualCvJob !== null}
        title={pendingManualCvJob ? (replaceConfirmation(pendingManualCvJob, 'manual')?.title ?? '') : ''}
        message={pendingManualCvJob ? (replaceConfirmation(pendingManualCvJob, 'manual')?.message ?? '') : ''}
        confirmLabel={pendingManualCvJob ? replaceConfirmation(pendingManualCvJob, 'manual')?.confirmLabel : undefined}
        onConfirm={() => {
          const job = pendingManualCvJob
          setPendingManualCvJob(null)
          if (job) window.__cinqoToolBridge.navigate(builderPathForJob(job.id))
        }}
        onCancel={() => {
          setPendingManualCvJob(null)
        }}
      />

      <ConfirmationModal
        open={pendingRemoveCvJob !== null}
        title={REMOVE_CONFIRMATION.title}
        message={REMOVE_CONFIRMATION.message}
        confirmLabel={REMOVE_CONFIRMATION.confirmLabel}
        variant="danger"
        onConfirm={confirmRemoveCv}
        onCancel={() => {
          setPendingRemoveCvJob(null)
        }}
      />

      <Modal
        open={historyJob !== null}
        title={historyJob ? `CV generation history — ${historyJob.title}` : 'CV generation history'}
        onClose={() => {
          setHistoryJob(null)
        }}
      >
        {historyLoading && <p className="text-sm text-gray-500">Loading…</p>}
        {historyError && <p className="text-sm text-red-700">{historyError}</p>}
        {!historyLoading && !historyError && cvGenerations.length === 0 && (
          <p className="text-sm text-gray-400">No generation attempts recorded yet.</p>
        )}
        {!historyLoading && !historyError && cvGenerations.length > 0 && (
          <ul className="flex max-h-96 flex-col gap-2 overflow-y-auto">
            {cvGenerations.map((generation) => (
              <li key={generation.id} className="rounded-md border border-gray-200 p-3">
                <div className="flex items-center justify-between gap-2">
                  <span className="text-sm font-medium text-gray-900">{new Date(generation.startedAt).toLocaleString()}</span>
                  <span
                    className={`shrink-0 text-xs font-medium ${
                      generation.status === 'completed'
                        ? 'text-green-700'
                        : generation.status === 'failed'
                          ? 'text-red-700'
                          : 'text-gray-500'
                    }`}
                  >
                    {generation.status}
                  </span>
                </div>
                {generation.error && <p className="mt-1 text-xs text-red-600">{generation.error}</p>}
                <div className="mt-2 flex flex-wrap gap-x-3 gap-y-1 text-xs">
                  <button
                    type="button"
                    onClick={() => {
                      window.__cinqoToolBridge.openConversation(generation.conversationId)
                    }}
                    className="text-gray-700 underline hover:text-gray-900"
                  >
                    Open conversation
                  </button>
                  <a
                    href={`/conversations/${generation.conversationId}/logs`}
                    target="_blank"
                    rel="noreferrer"
                    className="text-gray-700 underline hover:text-gray-900"
                  >
                    View raw logs
                  </a>
                  {generation.mediaFileId && (
                    <a
                      href={mediaDownloadUrl(generation.mediaFileId)}
                      target="_blank"
                      rel="noreferrer"
                      className="text-gray-700 underline hover:text-gray-900"
                    >
                      Download PDF
                    </a>
                  )}
                </div>
              </li>
            ))}
          </ul>
        )}
      </Modal>
    </div>
  )
}
