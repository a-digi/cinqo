import { useEffect, useMemo, useRef, useState } from 'react'
import { updatePortalLink, type LinkPagination, type LinkRule, type PortalLink } from '../../api'
import { WindowCheckPrompt } from '../WindowCheckPrompt'
import {
  fetchLinkCandidates,
  learnLinkPagination,
  learnLinkPattern,
  LinkCrawlError,
  WINDOW_CHECK_CODE,
  previewLinkRule,
  saveLinkRules,
  testLinkRule,
  type PageLinks,
  type PaginationProposal,
  type Proposal,
  type RulePreview,
  type RuleTest,
} from '../linkCrawl'
import {
  DEFAULT_MAX_PAGES,
  describeFailure,
  describePagination,
  filterLinks,
  hiddenSitesNotice,
  jobHostConfirmText,
  linkLabel,
  MAX_PAGES_LIMIT,
  NO_MARKS,
  parseMaxPages,
  saveBlockedReason,
  shortUrl,
  stopReasonText,
  toggleMark,
  type Marks,
} from '../linkCrawlRules'

import { InstructionSection } from '../InstructionSection'
import { buttonClass, cardClass, inputClass, primaryClass } from '../instructionStyles'

const SHOWN_LINKS = 200
const PREVIEW_ITEMS = 15

function errorText(err: unknown): string {
  if (err instanceof LinkCrawlError) return describeFailure(err.message, err.advice)
  return err instanceof Error ? err.message : 'Something went wrong.'
}

function siteHostOf(url: string): string {
  try {
    return new URL(url).host.toLowerCase()
  } catch {
    return ''
  }
}

// The link crawler of one portal link (plan 185, step 5): choose the job links of the link's page, see the partial URL the tool
// worked out, check what it finds, and save the rule. Shown in the "Job list rules (Katana)" modal that "Job list" opens while Katana is the
// selected engine (plan 190); the engine, not a stored choice, decides that Crawl now and auto-discovery use the rule.
export function LinkCrawlPanel({ link, onReload }: { link: PortalLink; onReload: () => void }) {
  const siteHost = useMemo(() => siteHostOf(link.url), [link.url])
  const [open, setOpen] = useState(false)
  const [page, setPage] = useState<PageLinks | null>(null)
  const [loading, setLoading] = useState(false)
  const [query, setQuery] = useState('')
  const [sameSiteOnly, setSameSiteOnly] = useState(true)
  const [marks, setMarks] = useState<Marks>(NO_MARKS)
  const [proposal, setProposal] = useState<Proposal | null>(null)
  const [pattern, setPattern] = useState('')
  const [excludes, setExcludes] = useState<string[]>([])
  const [preview, setPreview] = useState<RulePreview | null>(null)
  const [previewError, setPreviewError] = useState('')
  const [confirmed, setConfirmed] = useState(false)
  const [pagesProposal, setPagesProposal] = useState<PaginationProposal | null>(null)
  const [pagination, setPagination] = useState<LinkPagination | undefined>(undefined)
  const [paginationError, setPaginationError] = useState('')
  const [maxPagesText, setMaxPagesText] = useState(String(DEFAULT_MAX_PAGES))
  const [test, setTest] = useState<RuleTest | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [note, setNote] = useState('')
  const [confirmHost, setConfirmHost] = useState('')
  // plan 198: a browser window opened for a check; the person is asked whether it was cleared
  const [windowCheck, setWindowCheck] = useState(false)
  const learnSeq = useRef(0)
  const previewSeq = useRef(0)
  const pagesSeq = useRef(0)

  // The most pages a crawl loads, as typed (null = not a number from 1 to 50).
  const maxPages = parseMaxPages(maxPagesText)
  const effectivePagination: LinkPagination | undefined = useMemo(
    () => (pagination && maxPages ? { ...pagination, maxPages } : pagination),
    [pagination, maxPages],
  )
  const maxPagesError = pagination && maxPages === null ? `The most pages: a number from 1 to ${MAX_PAGES_LIMIT}.` : ''
  const rule: LinkRule = useMemo(
    () => ({
      host: siteHost,
      include: [pattern.trim()],
      exclude: excludes,
      ...(proposal?.rule.jobHost && proposal.rule.jobHost === link.jobHost ? { jobHost: proposal.rule.jobHost } : {}),
      ...(effectivePagination ? { pagination: effectivePagination } : {}),
    }),
    [siteHost, pattern, excludes, effectivePagination, proposal, link.jobHost],
  )

  // The picks changed: ask for the proposed partial URL (debounced; a late answer for older picks is dropped).
  useEffect(() => {
    if (!page || marks.picked.length === 0) {
      setProposal(null)
      return
    }
    const seq = ++learnSeq.current
    const timer = setTimeout(() => {
      learnLinkPattern(link.id, marks.picked, marks.notJob, page.links)
        .then((p) => {
          if (seq !== learnSeq.current) return
          setError('')
          setProposal(p)
          setPattern(p.pattern)
          setExcludes(p.exclude ?? [])
          setConfirmed(false)
          setTest(null)
          setNote('')
        })
        .catch((err: unknown) => {
          if (seq !== learnSeq.current) return
          setProposal(null)
          setError(errorText(err))
        })
    }, 250)
    return () => {
      clearTimeout(timer)
    }
  }, [marks, page, link.id])

  // The next-page links changed: ask how the pages follow each other (debounced; a late answer for older picks is dropped).
  useEffect(() => {
    if (!page || marks.pages.length === 0) {
      setPagesProposal(null)
      setPagination(undefined)
      setPaginationError('')
      return
    }
    const seq = ++pagesSeq.current
    const timer = setTimeout(() => {
      learnLinkPagination(link.id, marks.pages, page.links, page.url)
        .then((p) => {
          if (seq !== pagesSeq.current) return
          setPaginationError('')
          setPagesProposal(p)
          setPagination(p.pagination)
          setConfirmed(false)
          setTest(null)
        })
        .catch((err: unknown) => {
          if (seq !== pagesSeq.current) return
          setPagesProposal(null)
          setPagination(undefined)
          setPaginationError(errorText(err))
        })
    }, 250)
    return () => {
      clearTimeout(timer)
    }
  }, [marks.pages, page, link.id])

  // The pattern changed (proposed or edited): check what it finds among this page's links.
  useEffect(() => {
    if (!page || pattern.trim() === '') {
      setPreview(null)
      setPreviewError('')
      return
    }
    const seq = ++previewSeq.current
    const timer = setTimeout(() => {
      previewLinkRule(link.id, rule, page.links)
        .then((p) => {
          if (seq !== previewSeq.current) return
          setPreview(p)
          setPreviewError('')
        })
        .catch((err: unknown) => {
          if (seq !== previewSeq.current) return
          setPreview(null)
          setPreviewError(errorText(err))
        })
    }, 300)
    return () => {
      clearTimeout(timer)
    }
  }, [page, rule, pattern, link.id])

  const shown = useMemo(
    () => (page ? filterLinks(page.links, query, sameSiteOnly, siteHost, link.jobHost) : []),
    [page, query, sameSiteOnly, siteHost, link.jobHost],
  )
  // plan 203: which links of other sites are hidden, and when the page's postings are on another site
  const notice = useMemo(
    () => (page ? hiddenSitesNotice(page.site, page.otherHosts, siteHost, sameSiteOnly) : null),
    [page, siteHost, sameSiteOnly],
  )
  const warnHost = page?.otherHosts?.find((h) => !h.allowed)?.host ?? ''
  const jobUrls = useMemo(() => new Set(preview?.items.map((i) => i.url) ?? []), [preview])

  function loadLinks() {
    setLoading(true)
    setError('')
    setNote('')
    setWindowCheck(false)
    fetchLinkCandidates(link.id)
      .then((p) => {
        setPage(p)
        setMarks(NO_MARKS)
        setPagesProposal(null)
        setPagination(undefined)
        setPaginationError('')
        setProposal(null)
        setPattern('')
        setExcludes([])
        setTest(null)
      })
      .catch((err: unknown) => {
        if (err instanceof LinkCrawlError && err.code === WINDOW_CHECK_CODE) {
          setWindowCheck(true) // the question below replaces the error text
          return
        }
        setError(errorText(err))
      })
      .finally(() => {
        setLoading(false)
      })
  }

  // Confirm or remove the link's job site (plan 203); the person decides, the AI never does. A rule made for the old site is dropped by
  // the backend, so the screen starts again.
  function setJobHost(host: string) {
    setBusy(true)
    setError('')
    updatePortalLink(link.id, { jobHost: host })
      .then(() => {
        setConfirmHost('')
        setPage(null)
        setMarks(NO_MARKS)
        setProposal(null)
        setPattern('')
        setNote(host === '' ? 'The job site is removed.' : `${host} is now this link's job site. Load the links again to pick its jobs.`)
        onReload()
      })
      .catch((err: unknown) => {
        setError(errorText(err))
      })
      .finally(() => {
        setBusy(false)
      })
  }

  function runTest() {
    setBusy(true)
    setError('')
    testLinkRule(link.id, rule)
      .then(setTest)
      .catch((err: unknown) => {
        setError(errorText(err))
      })
      .finally(() => {
        setBusy(false)
      })
  }

  function save() {
    setBusy(true)
    setError('')
    saveLinkRules(link.id, rule)
      .then(() => {
        setNote('The rule is saved.')
        onReload()
      })
      .catch((err: unknown) => {
        setError(errorText(err))
      })
      .finally(() => {
        setBusy(false)
      })
  }

  function removeRule() {
    setBusy(true)
    setError('')
    saveLinkRules(link.id, null)
      .then(() => {
        setNote('The rule is removed.')
        onReload()
      })
      .catch((err: unknown) => {
        setError(errorText(err))
      })
      .finally(() => {
        setBusy(false)
      })
  }

  const blocked = saveBlockedReason({
    pattern,
    preview: preview ? { matched: preview.matched, needsConfirm: preview.needsConfirm } : null,
    previewError,
    proposalNeedsConfirm: proposal?.needsConfirm ?? false,
    paginationError: paginationError || maxPagesError,
    paginationNeedsConfirm: pagesProposal?.needsConfirm ?? false,
    confirmed,
    busy,
  })
  const needsConfirm = (proposal?.needsConfirm ?? false) || (preview?.needsConfirm ?? false) || (pagesProposal?.needsConfirm ?? false)
  const notes = [...(proposal?.notes ?? []), ...(preview?.notes ?? []), ...(pagesProposal?.notes ?? [])]

  return (
    <div className="text-sm text-gray-800" data-testid="link-crawl-panel">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0">
          <h3 className="text-base font-semibold text-gray-900">Link crawler</h3>
          <p className="mt-0.5 text-sm text-gray-500">Find the jobs of this page by the address of their links, without AI.</p>
        </div>
        <button
          type="button"
          className={primaryClass}
          onClick={() => {
            setOpen((o) => !o)
          }}
        >
          {open ? 'Close' : link.linkRules ? 'Change the rule' : 'Choose the job links'}
        </button>
      </div>

      {link.jobHost !== '' && (
        <p className="mt-4 flex flex-wrap items-center gap-2 text-gray-800" data-testid="job-host-line">
          <span>
            Job links also on <strong>{link.jobHost}</strong> (confirmed by you).
          </span>
          <button
            type="button"
            className={buttonClass}
            disabled={busy}
            onClick={() => {
              setJobHost('')
            }}
          >
            Remove
          </button>
        </p>
      )}

      {link.linkRules && (
        <div className={`mt-4 ${cardClass} text-gray-800`} data-testid="link-crawl-saved-rule">
          <div className="font-semibold text-gray-900">Saved rule for {link.linkRules.host}</div>
          <ul className="mt-1 list-disc pl-5 font-mono">
            {link.linkRules.include.map((p) => (
              <li key={`i-${p}`}>{p}</li>
            ))}
            {(link.linkRules.exclude ?? []).map((p) => (
              <li key={`e-${p}`} className="text-red-800">
                not: {p}
              </li>
            ))}
          </ul>
          <p className="mt-1">{describePagination(link.linkRules.pagination)}</p>
          <p className="mt-1 text-gray-600">
            Crawl now and auto-discovery use this rule while Katana is the selected engine. With Rod selected the link is crawled with its
            AI instructions instead.
          </p>
          <button type="button" className={`${buttonClass} mt-1`} disabled={busy} onClick={removeRule}>
            Remove the rule
          </button>
        </div>
      )}

      {note && (
        <p role="status" className="mt-4 text-green-700">
          {note}
        </p>
      )}
      {error && (
        <p role="alert" className="mt-4 text-red-700">
          {error}
        </p>
      )}

      {open && windowCheck && (
        <WindowCheckPrompt
          className="mt-3"
          portalLinkId={link.id}
          onCleared={loadLinks}
          onCancel={() => {
            setWindowCheck(false)
          }}
        />
      )}

      {open && (
        <div className="mt-8">
          <InstructionSection
            number={1}
            title="Choose the job links"
            description="Load the links of the page, then tick two or three that are jobs."
          >
            <div className="flex flex-wrap items-center gap-2">
              <button type="button" className={primaryClass} disabled={loading} onClick={loadLinks}>
                {loading ? 'Loading the links…' : page ? 'Load the links again' : 'Load the links of this page'}
              </button>
              {page && (
                <span className="text-gray-700">
                  {page.links.length} links found on “{page.title || page.finalUrl}”
                  {page.truncated ? ' (the list was cut at its limit)' : ''}
                  {page.droppedPrivate > 0 ? `, ${page.droppedPrivate} to private addresses hidden` : ''}
                </span>
              )}
            </div>

            {page && (
              <>
                {notice && (
                  <p
                    role="status"
                    data-testid="hidden-sites-notice"
                    className={`rounded-md border px-2 py-1.5 ${notice.tone === 'warn' ? 'border-amber-300 bg-amber-50 text-amber-900' : 'border-gray-200 bg-gray-50 text-gray-700'}`}
                  >
                    {notice.text}
                  </p>
                )}
                {notice?.tone === 'warn' && warnHost !== '' && (
                  <div className="rounded-md border border-amber-300 bg-white p-2 text-amber-900" data-testid="job-host-confirm">
                    {confirmHost === '' ? (
                      <button
                        type="button"
                        className={buttonClass}
                        disabled={busy}
                        onClick={() => {
                          setConfirmHost(warnHost)
                        }}
                      >
                        Use {warnHost} for the job links…
                      </button>
                    ) : (
                      <>
                        <p>{jobHostConfirmText(confirmHost, siteHost)}</p>
                        <div className="mt-2 flex gap-2">
                          <button
                            type="button"
                            className={primaryClass}
                            disabled={busy}
                            onClick={() => {
                              setJobHost(confirmHost)
                            }}
                          >
                            Yes, confirm {confirmHost}
                          </button>
                          <button
                            type="button"
                            className={buttonClass}
                            disabled={busy}
                            onClick={() => {
                              setConfirmHost('')
                            }}
                          >
                            Cancel
                          </button>
                        </div>
                      </>
                    )}
                  </div>
                )}
                <p className="text-gray-700">
                  Tick the links that are <strong>jobs</strong>; two or three are enough. Mark a link that looks like a job but is not one
                  with <em>not a job</em>, and the link to the second (and third) page of the list with <em>next page</em>.
                </p>
                <div className="flex flex-wrap items-center gap-3">
                  <input
                    type="search"
                    value={query}
                    onChange={(e) => {
                      setQuery(e.target.value)
                    }}
                    placeholder="Filter by address or text"
                    aria-label="Filter the links"
                    className={`w-64 ${inputClass}`}
                  />
                  <label className="flex items-center gap-1">
                    <input
                      type="checkbox"
                      checked={sameSiteOnly}
                      onChange={(e) => {
                        setSameSiteOnly(e.target.checked)
                      }}
                    />
                    Only this site ({siteHost}
                    {link.jobHost !== '' ? ` and ${link.jobHost}` : ''})
                  </label>
                  <span className="text-gray-600">
                    {marks.picked.length} job link(s) ticked, {marks.notJob.length} marked not a job, {marks.pages.length} next page link(s)
                  </span>
                </div>

                <ul className="max-h-96 overflow-auto rounded-xl border border-gray-200 bg-white" aria-label="Links of the page">
                  {shown.slice(0, SHOWN_LINKS).map((l) => {
                    const isPicked = marks.picked.includes(l.url)
                    const isNot = marks.notJob.includes(l.url)
                    const isPage = marks.pages.includes(l.url)
                    return (
                      <li
                        key={l.url}
                        className={`flex items-start gap-2 border-b border-gray-100 px-2 py-1 last:border-b-0 ${isPicked ? 'bg-green-50' : isNot ? 'bg-red-50' : isPage ? 'bg-gray-100' : ''}`}
                      >
                        <input
                          type="checkbox"
                          checked={isPicked}
                          aria-label={`Job link ${l.url}`}
                          onChange={() => {
                            setMarks((m) => toggleMark(m, l.url, 'picked'))
                          }}
                          className="mt-0.5"
                        />
                        <div className="min-w-0 flex-1">
                          <div className="truncate font-medium text-gray-900">
                            {linkLabel(l) || <span className="text-gray-400">(no text)</span>}
                          </div>
                          <div className="truncate font-mono text-gray-600">{shortUrl(l.url, siteHost)}</div>
                        </div>
                        {jobUrls.has(l.url) && <span className="shrink-0 rounded bg-green-100 px-1.5 py-0.5 text-green-800">job</span>}
                        <button
                          type="button"
                          className={`shrink-0 rounded border px-1.5 py-0.5 ${isNot ? 'border-red-300 bg-red-100 text-red-800' : 'border-gray-200 text-gray-600 hover:bg-gray-100'}`}
                          onClick={() => {
                            setMarks((m) => toggleMark(m, l.url, 'notJob'))
                          }}
                        >
                          {isNot ? 'not a job ✓' : 'not a job'}
                        </button>
                        <button
                          type="button"
                          className={`shrink-0 rounded border px-1.5 py-0.5 ${isPage ? 'border-gray-900 bg-gray-900 text-white' : 'border-gray-200 text-gray-600 hover:bg-gray-100'}`}
                          onClick={() => {
                            setMarks((m) => toggleMark(m, l.url, 'pages'))
                          }}
                        >
                          {isPage ? 'next page ✓' : 'next page'}
                        </button>
                      </li>
                    )
                  })}
                  {shown.length === 0 && <li className="px-2 py-2 text-gray-500">No link matches the filter.</li>}
                  {shown.length > SHOWN_LINKS && (
                    <li className="px-2 py-1 text-gray-500">{shown.length - SHOWN_LINKS} more: narrow the filter to see them.</li>
                  )}
                </ul>
              </>
            )}
          </InstructionSection>

          {page && marks.pages.length > 0 && (
            <InstructionSection number={2} title="The next pages" description="How the crawl moves on from the first page of the list.">
              <div className="space-y-3" data-testid="link-crawl-pagination">
                {pagesProposal && (
                  <>
                    <p>{describePagination(effectivePagination)}</p>
                    <ul className="font-mono text-gray-600">
                      {pagesProposal.nextUrls.slice(0, 3).map((u) => (
                        <li key={u} className="truncate">
                          {shortUrl(u, siteHost)}
                        </li>
                      ))}
                    </ul>
                  </>
                )}
                {paginationError && (
                  <p role="alert" className="text-red-700">
                    {paginationError}
                  </p>
                )}
                <label className="flex items-center gap-2">
                  The most pages one crawl loads
                  <input
                    type="text"
                    inputMode="numeric"
                    value={maxPagesText}
                    onChange={(e) => {
                      setMaxPagesText(e.target.value)
                      setTest(null)
                    }}
                    aria-label="The most pages"
                    className={`w-20 ${inputClass}`}
                  />
                  <span className="text-gray-600">(1 to {MAX_PAGES_LIMIT})</span>
                </label>
                {maxPagesError && (
                  <p role="alert" className="text-red-700">
                    {maxPagesError}
                  </p>
                )}
                <button
                  type="button"
                  className={buttonClass}
                  onClick={() => {
                    setMarks((m) => ({ ...m, pages: [] }))
                  }}
                >
                  Only the first page
                </button>
              </div>
            </InstructionSection>
          )}

          {page && marks.picked.length > 0 && (
            <>
              <InstructionSection
                number={3}
                title="The partial URL of the jobs"
                description="The address pattern the job links share, and the links to leave out."
              >
                <label className="block">
                  <span className="sr-only">Pattern</span>
                  <input
                    type="text"
                    value={pattern}
                    onChange={(e) => {
                      setPattern(e.target.value)
                      setConfirmed(false)
                      setTest(null)
                    }}
                    spellCheck={false}
                    className={`w-full font-mono ${inputClass}`}
                  />
                </label>
                <p className="text-gray-600">
                  <code>*</code> stands for any text inside one part of the address, <code>#</code> for a number (an id such as 8195, but
                  not a word such as types), <code>**</code> for anything below it. The scheme and a trailing / do not matter.
                </p>
                {excludes.length > 0 && (
                  <div>
                    <div className="font-semibold text-gray-900">Not these</div>
                    <ul className="mt-1 space-y-1">
                      {excludes.map((p) => (
                        <li key={p} className="flex items-center gap-2 font-mono text-red-800">
                          {p}
                          <button
                            type="button"
                            className={buttonClass}
                            onClick={() => {
                              setExcludes((ex) => ex.filter((x) => x !== p))
                            }}
                          >
                            Remove
                          </button>
                        </li>
                      ))}
                    </ul>
                  </div>
                )}
              </InstructionSection>
              <InstructionSection
                number={4}
                title="Check and save"
                description="See what the rule finds on this page, test it on the page, then save it."
              >
                {preview && (
                  <div data-testid="link-crawl-preview">
                    <div>
                      The rule finds <strong>{preview.items.length}</strong> job(s) on this page
                      {preview.skippedNoTitle > 0 ? `; ${preview.skippedNoTitle} matching link(s) have no text and are left out` : ''}.
                    </div>
                    <ul className="mt-1 max-h-40 overflow-auto">
                      {preview.items.slice(0, PREVIEW_ITEMS).map((i) => (
                        <li key={i.url} className="truncate">
                          <span className="font-medium">{i.title}</span>{' '}
                          <span className="font-mono text-gray-500">{shortUrl(i.url, siteHost)}</span>
                        </li>
                      ))}
                    </ul>
                    {preview.items.length > PREVIEW_ITEMS && (
                      <div className="text-gray-500">…and {preview.items.length - PREVIEW_ITEMS} more.</div>
                    )}
                  </div>
                )}
                {previewError && (
                  <p role="alert" className="text-red-700">
                    {previewError}
                  </p>
                )}
                {notes.length > 0 && (
                  <ul className="list-disc pl-5 text-amber-900">
                    {notes.map((n) => (
                      <li key={n}>{n}</li>
                    ))}
                  </ul>
                )}
                {proposal && (proposal.excludedStillMatched?.length ?? 0) > 0 && (
                  <p className="text-amber-900">
                    Still matched although you marked them as not a job: {proposal.excludedStillMatched?.join(', ')}
                  </p>
                )}
                {needsConfirm && (
                  <label className="flex items-center gap-2 text-amber-900">
                    <input
                      type="checkbox"
                      checked={confirmed}
                      onChange={(e) => {
                        setConfirmed(e.target.checked)
                      }}
                    />
                    I checked the matches: they are all jobs.
                  </label>
                )}
                <div className="flex flex-wrap items-center gap-2">
                  <button
                    type="button"
                    className={buttonClass}
                    disabled={busy || pattern.trim() === '' || previewError !== ''}
                    onClick={runTest}
                  >
                    Test on the page now
                  </button>
                  <button type="button" className={primaryClass} disabled={blocked !== ''} onClick={save} title={blocked}>
                    Save the rule
                  </button>
                  {blocked !== '' && <span className="text-gray-600">{blocked}</span>}
                </div>
                {test && (
                  <div data-testid="link-crawl-test" className="rounded-md border border-green-200 bg-green-50 p-2">
                    <div>
                      Loaded {test.pages.length} page(s) just now: <strong>{test.items.length}</strong> job(s) would be created;{' '}
                      {stopReasonText(test.stopReason, test.stopDetail)}. Nothing was saved.
                    </div>
                    <ul className="mt-1">
                      {test.pages.map((p, i) => (
                        <li key={p.url} className="truncate">
                          Page {i + 1}: {p.newJobs} new job link(s) — “{p.title !== '' ? p.title : p.finalUrl}”
                        </li>
                      ))}
                    </ul>
                    {test.stopReason === 'max_pages' && (
                      <div className="text-gray-600">A test loads two pages at most; a crawl loads up to the limit above.</div>
                    )}
                  </div>
                )}
              </InstructionSection>
            </>
          )}
        </div>
      )}
    </div>
  )
}
